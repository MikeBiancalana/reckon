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

import "fmt"

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
// Report describing every record's outcome.
func (imp *Importer) Run() (*Report, error) {
	return nil, fmt.Errorf("textmigrate: Importer.Run not implemented")
}
