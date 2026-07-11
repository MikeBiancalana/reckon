// Package textmigrate is a one-shot importer that reads legacy
// reckon data (gen-1 task files, SQLite-backed notes/checklists, and legacy
// journal day files) and emits vault-native canonical node files that the
// node/index tooling understands. Old xids are preserved as node aliases so
// pre-migration references keep resolving through the index's alias path.
//
// Every record is converted via the node.NewNode -> set fields -> Render ->
// Parse -> writeFileAtomic recipe, and re-running the importer over its own
// output is idempotent: a source record whose old id is already an alias in
// the destination vault is skipped, not duplicated.
package textmigrate

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MikeBiancalana/reckon/internal/node"
)

// legacyAuthor is the author stamped on every migrated node: none of the
// legacy sources record who authored a task/note/checklist/journal entry, so
// migrated content is attributed to the migration itself rather than
// fabricating an author.
const legacyAuthor = "legacy-import"

// Importer performs (or, with DryRun, previews) the migration.
type Importer struct {
	// Source is the legacy data root: Source/tasks, Source/notes,
	// Source/journal, and Source/reckon.db all live under it.
	Source string
	// Dest is the destination vault root.
	Dest string
	// DryRun computes the same create/skip/error plan a real run would,
	// without writing any file or touching the destination's index/cache.
	DryRun bool
}

// RecordOutcome is one migrated (or, under DryRun, would-migrate) node.
type RecordOutcome struct {
	SourceID string // the legacy record's id (xid, or a journal day's date)
	Path     string // vault-relative destination path
	ULID     string // the minted node id
}

// SkippedOutcome is one source record left alone because the destination
// vault already carries its old id as an alias.
type SkippedOutcome struct {
	SourceID string
	Reason   string
}

// ErroredOutcome is one source record the importer could not convert.
type ErroredOutcome struct {
	SourceID string
	Error    string
}

// TypeResult is the created/skipped/errored outcome set for one source
// record type.
type TypeResult struct {
	Created []RecordOutcome
	Skipped []SkippedOutcome
	Errored []ErroredOutcome
}

func (r *TypeResult) addCreated(sourceID, path, ulid string) {
	r.Created = append(r.Created, RecordOutcome{SourceID: sourceID, Path: path, ULID: ulid})
}

func (r *TypeResult) addSkipped(sourceID, reason string) {
	r.Skipped = append(r.Skipped, SkippedOutcome{SourceID: sourceID, Reason: reason})
}

func (r *TypeResult) addErrored(sourceID string, err error) {
	r.Errored = append(r.Errored, ErroredOutcome{SourceID: sourceID, Error: err.Error()})
}

// JournalPreambleCounts tallies legacy journal sections that have no
// direct log-entry analog and are instead preserved as a log-day's
// preamble block, summed across every migrated day.
type JournalPreambleCounts struct {
	Intentions int
	Wins       int
	Schedule   int
}

// Report is the structured summary of one Importer.Run.
type Report struct {
	Tasks              TypeResult
	Notes              TypeResult
	ChecklistTemplates TypeResult
	ChecklistRuns      TypeResult
	JournalDays        TypeResult

	// FoldedTaskNotes counts legacy task notes folded into a migrated
	// todo's body (the new todo schema has no separate notes field).
	FoldedTaskNotes int
	// RewrittenTaskRefs counts inline [task:<xid>] markers rewritten to
	// [[<xid>]] wikilinks within migrated journal log-entry bodies.
	RewrittenTaskRefs int
	// JournalPreamble sums JournalPreambleCounts across every migrated day.
	JournalPreamble JournalPreambleCounts
}

// Run performs the migration (or, under DryRun, previews it) and returns a
// Report describing every record's outcome. Every source is processed even
// if another source errors partway through (error strategy: accumulate
// per-record failures into the report, never abort the run).
func (imp *Importer) Run() (*Report, error) {
	report := &Report{}

	if err := imp.runTasks(report); err != nil {
		return nil, fmt.Errorf("textmigrate: tasks: %w", err)
	}
	if err := imp.runNotes(report); err != nil {
		return nil, fmt.Errorf("textmigrate: notes: %w", err)
	}
	if err := imp.runChecklists(report); err != nil {
		return nil, fmt.Errorf("textmigrate: checklists: %w", err)
	}
	if err := imp.runJournal(report); err != nil {
		return nil, fmt.Errorf("textmigrate: journal: %w", err)
	}

	return report, nil
}

// renderAndParse is the shared tail of the NewNode -> set fields -> Render ->
// Parse -> writeFileAtomic recipe every converter follows: it turns a
// freshly-built node into the byte-preserving, round-trip-stable form that is
// actually written to disk.
func renderAndParse(n *node.Node) (*node.Node, error) {
	parsed, err := node.Parse(n.Render())
	if err != nil {
		return nil, fmt.Errorf("render/parse: %w", err)
	}
	return parsed, nil
}

// scanAliases collects every alias already present on markdown files
// directly under dir (non-recursive), for the idempotency check: a source
// record whose old id is already one of these aliases has already been
// migrated. A missing directory is not an error (nothing
// migrated yet). Unparsable/CRLF files are skipped rather than aborting the
// scan, matching adopt.go/note_v1.go's tolerant-scan policy.
func scanAliases(dir string) (map[string]bool, error) {
	aliases := map[string]bool{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return aliases, nil
		}
		return nil, fmt.Errorf("scan %s: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		raw, err := os.ReadFile(path)
		if err != nil || bytes.Contains(raw, []byte("\r\n")) {
			continue
		}
		n, err := node.Parse(raw)
		if err != nil {
			continue
		}
		for _, a := range n.Aliases {
			aliases[a] = true
		}
	}
	return aliases, nil
}

// overrideDataDir temporarily points RECKON_DATA_DIR at dir so
// journal.TaskService.GetAllTasks() -- which resolves its directory via the
// global config.TasksDir() rather than an injectable parameter -- reads the
// given legacy root instead of whatever the process's ambient legacy data
// dir happens to be. The returned func restores the previous value (or
// unsets it if it was unset).
func overrideDataDir(dir string) func() {
	prev, had := os.LookupEnv("RECKON_DATA_DIR")
	os.Setenv("RECKON_DATA_DIR", dir)
	return func() {
		if had {
			os.Setenv("RECKON_DATA_DIR", prev)
		} else {
			os.Unsetenv("RECKON_DATA_DIR")
		}
	}
}
