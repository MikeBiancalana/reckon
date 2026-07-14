package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/output"
	"github.com/MikeBiancalana/reckon/internal/textmigrate"
	"github.com/spf13/cobra"
)

// importCmd runs the one-shot legacy-data importer: it reads gen-1 task
// files, SQLite-backed notes/checklists, and legacy journal day files, and
// writes vault-native canonical node files that carry the old ids as
// aliases. It opens the legacy store itself and never touches the default
// service DB, so it does not require the root command's normal DB init.
var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Migrate legacy SQLite-primary data into vault-native canonical nodes",
	Long: "Read gen-1 task files, SQLite-backed notes/checklists, and legacy journal " +
		"day files, and write vault-native canonical node files under todos/, notes/, " +
		"checklists/, and log/. Each migrated node's old id becomes an alias, so " +
		"pre-migration references keep resolving through the index. Re-running is " +
		"idempotent: a source record already migrated is skipped, not duplicated.",
	SilenceUsage: true,
	Args:         cobra.NoArgs,
	RunE:         runImportE,
}

var (
	importDryRunFlag bool
	importVerifyFlag bool
	importSourceFlag string
)

func init() {
	f := importCmd.Flags()
	f.BoolVar(&importDryRunFlag, "dry-run", false, "Preview the migration; write no files, touch no index/cache")
	f.BoolVar(&importVerifyFlag, "verify", false, "Verify a completed migration against a rebuilt index instead of migrating")
	f.StringVar(&importSourceFlag, "source", "", "Legacy data root (default: the legacy data directory)")
}

// resetImportFlags restores import flag variables to their defaults and
// clears their pflag Changed state, mirroring the other v1 commands' reset
// helpers.
func resetImportFlags(cmd *cobra.Command) {
	importDryRunFlag = false
	importVerifyFlag = false
	importSourceFlag = ""
	for _, name := range []string{"dry-run", "verify", "source"} {
		if fl := cmd.Flags().Lookup(name); fl != nil {
			fl.Changed = false
		}
	}
}

// importResult is the structured summary of one `rk import` run, printed
// via internal/output once the importer itself is implemented.
type importResult struct {
	Tasks              importTypeResult `json:"tasks"`
	Notes              importTypeResult `json:"notes"`
	ChecklistTemplates importTypeResult `json:"checklist_templates"`
	ChecklistRuns      importTypeResult `json:"checklist_runs"`
	JournalDays        importTypeResult `json:"journal_days"`

	FoldedTaskNotes   int `json:"folded_task_notes,omitempty"`
	RewrittenTaskRefs int `json:"rewritten_task_refs,omitempty"`
}

type importTypeResult struct {
	Created []importRecordEntry  `json:"created"`
	Skipped []importSkippedEntry `json:"skipped"`
	Errored []importErroredEntry `json:"errored"`
}

// Pretty renders a human-readable summary line for one run, followed by one
// indented line per created/skipped/errored record across every type.
func (r importResult) Pretty() string {
	var b strings.Builder
	created := len(r.Tasks.Created) + len(r.Notes.Created) + len(r.ChecklistTemplates.Created) +
		len(r.ChecklistRuns.Created) + len(r.JournalDays.Created)
	skipped := len(r.Tasks.Skipped) + len(r.Notes.Skipped) + len(r.ChecklistTemplates.Skipped) +
		len(r.ChecklistRuns.Skipped) + len(r.JournalDays.Skipped)
	errored := len(r.Tasks.Errored) + len(r.Notes.Errored) + len(r.ChecklistTemplates.Errored) +
		len(r.ChecklistRuns.Errored) + len(r.JournalDays.Errored)
	fmt.Fprintf(&b, "import: %d created, %d skipped, %d errored", created, skipped, errored)
	if r.FoldedTaskNotes > 0 {
		fmt.Fprintf(&b, "\n  folded %d task note(s) into todo bodies", r.FoldedTaskNotes)
	}
	if r.RewrittenTaskRefs > 0 {
		fmt.Fprintf(&b, "\n  rewrote %d [task:xid] reference(s) to [[xid]]", r.RewrittenTaskRefs)
	}
	byType := []struct {
		name string
		tr   importTypeResult
	}{
		{"task", r.Tasks}, {"note", r.Notes}, {"checklist template", r.ChecklistTemplates},
		{"checklist run", r.ChecklistRuns}, {"journal day", r.JournalDays},
	}
	for _, grp := range byType {
		name, tr := grp.name, grp.tr
		for _, c := range tr.Created {
			fmt.Fprintf(&b, "\n  created %s %s -> %s (id %s)", name, c.SourceID, c.Path, c.ULID)
		}
		for _, s := range tr.Skipped {
			fmt.Fprintf(&b, "\n  skipped %s %s (%s)", name, s.SourceID, s.Reason)
		}
		for _, e := range tr.Errored {
			fmt.Fprintf(&b, "\n  error %s %s: %s", name, e.SourceID, e.Error)
		}
	}
	return b.String()
}

type importRecordEntry struct {
	SourceID string `json:"source_id"`
	Path     string `json:"path"`
	ULID     string `json:"ulid"`
}

type importSkippedEntry struct {
	SourceID string `json:"source_id"`
	Reason   string `json:"reason"`
}

type importErroredEntry struct {
	SourceID string `json:"source_id"`
	Error    string `json:"error"`
}

// importVerifyResult is the structured summary of one `rk import --verify`
// run, printed via internal/output once the importer itself is implemented.
type importVerifyResult struct {
	Counts             map[string]int `json:"counts"`
	UnexpectedWarnings []string       `json:"unexpected_warnings,omitempty"`
	UnresolvedAliases  []string       `json:"unresolved_aliases,omitempty"`
	OK                 bool           `json:"ok"`
}

// Pretty renders a human-readable verify summary: overall OK/FAILED, the
// per-type counts, and one indented line per unexpected warning or
// unresolved alias.
func (r importVerifyResult) Pretty() string {
	var b strings.Builder
	status := "OK"
	if !r.OK {
		status = "FAILED"
	}
	fmt.Fprintf(&b, "import verify: %s", status)
	types := make([]string, 0, len(r.Counts))
	for typ := range r.Counts {
		types = append(types, typ)
	}
	sort.Strings(types)
	for _, typ := range types {
		fmt.Fprintf(&b, "\n  %s: %d", typ, r.Counts[typ])
	}
	for _, w := range r.UnexpectedWarnings {
		fmt.Fprintf(&b, "\n  unexpected warning: %s", w)
	}
	for _, a := range r.UnresolvedAliases {
		fmt.Fprintf(&b, "\n  unresolved alias: %s", a)
	}
	return b.String()
}

func runImportE(cmd *cobra.Command, args []string) error {
	defer resetImportFlags(cmd)

	if importVerifyFlag && importDryRunFlag {
		return fmt.Errorf("import: --dry-run and --verify are mutually exclusive")
	}

	mode, err := output.ModeFromFlags(jsonFlag, ndjsonFlag)
	if err != nil {
		return err
	}

	cfg, err := config.LoadWithOverrides(vaultFlag, "")
	if err != nil {
		return fmt.Errorf("import: load config: %w", err)
	}

	source := importSourceFlag
	if source == "" {
		source, err = config.DataDir()
		if err != nil {
			return fmt.Errorf("import: resolve source dir: %w", err)
		}
	}

	imp := &textmigrate.Importer{Source: source, Dest: cfg.VaultDir, DryRun: importDryRunFlag}

	if importVerifyFlag {
		vr, err := imp.Verify()
		if err != nil {
			return fmt.Errorf("import: verify: %w", err)
		}
		res := toImportVerifyResult(vr)
		if !(mode == output.Pretty && quietFlag) {
			if err := output.New(cmd.OutOrStdout(), mode).Print(res); err != nil {
				return err
			}
		}
		if !res.OK {
			return fmt.Errorf("import: verify found %d unexpected warning(s) and %d unresolved alias(es)",
				len(res.UnexpectedWarnings), len(res.UnresolvedAliases))
		}
		return nil
	}

	report, err := imp.Run()
	if err != nil {
		return fmt.Errorf("import: %w", err)
	}
	res := toImportResult(report)

	if !(mode == output.Pretty && quietFlag) {
		if err := output.New(cmd.OutOrStdout(), mode).Print(res); err != nil {
			return err
		}
	}

	errored := len(res.Tasks.Errored) + len(res.Notes.Errored) + len(res.ChecklistTemplates.Errored) +
		len(res.ChecklistRuns.Errored) + len(res.JournalDays.Errored)
	if errored > 0 {
		return fmt.Errorf("import: %d record(s) failed to migrate", errored)
	}
	return nil
}

// toImportResult maps a textmigrate.Report onto the CLI's JSON-tagged result
// shape.
func toImportResult(r *textmigrate.Report) importResult {
	return importResult{
		Tasks:              toImportTypeResult(r.Tasks),
		Notes:              toImportTypeResult(r.Notes),
		ChecklistTemplates: toImportTypeResult(r.ChecklistTemplates),
		ChecklistRuns:      toImportTypeResult(r.ChecklistRuns),
		JournalDays:        toImportTypeResult(r.JournalDays),
		FoldedTaskNotes:    r.FoldedTaskNotes,
		RewrittenTaskRefs:  r.RewrittenTaskRefs,
	}
}

func toImportTypeResult(tr textmigrate.TypeResult) importTypeResult {
	created := make([]importRecordEntry, 0, len(tr.Created))
	for _, c := range tr.Created {
		created = append(created, importRecordEntry{SourceID: c.SourceID, Path: c.Path, ULID: c.ULID})
	}
	skipped := make([]importSkippedEntry, 0, len(tr.Skipped))
	for _, s := range tr.Skipped {
		skipped = append(skipped, importSkippedEntry{SourceID: s.SourceID, Reason: s.Reason})
	}
	errored := make([]importErroredEntry, 0, len(tr.Errored))
	for _, e := range tr.Errored {
		errored = append(errored, importErroredEntry{SourceID: e.SourceID, Error: e.Error})
	}
	return importTypeResult{Created: created, Skipped: skipped, Errored: errored}
}

// toImportVerifyResult maps a textmigrate.VerifyResult onto the CLI's
// JSON-tagged result shape (index.Warning values are rendered to strings so
// the CLI output has no dependency on internal/index's own JSON shape).
func toImportVerifyResult(vr *textmigrate.VerifyResult) importVerifyResult {
	warnings := make([]string, 0, len(vr.UnexpectedWarnings))
	for _, w := range vr.UnexpectedWarnings {
		if w.Kind == "duplicate_ulid" {
			warnings = append(warnings, fmt.Sprintf("duplicate_ulid %s (%s)", w.ULID, strings.Join(w.Files, ", ")))
		} else {
			warnings = append(warnings, fmt.Sprintf("alias_collision %s (%s)", w.Alias, strings.Join(w.Files, ", ")))
		}
	}
	unresolved := append([]string(nil), vr.UnresolvedAliases...)
	sort.Strings(unresolved)
	return importVerifyResult{
		Counts:             vr.Counts,
		UnexpectedWarnings: warnings,
		UnresolvedAliases:  unresolved,
		OK:                 vr.OK,
	}
}
