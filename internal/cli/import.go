package cli

import (
	"fmt"

	"github.com/MikeBiancalana/reckon/internal/config"
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
	Annotations:  map[string]string{"requiresDB": "false"},
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

func runImportE(cmd *cobra.Command, args []string) error {
	defer resetImportFlags(cmd)

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
		if _, err := imp.Verify(); err != nil {
			return fmt.Errorf("import: verify: %w", err)
		}
		return nil
	}

	if _, err := imp.Run(); err != nil {
		return fmt.Errorf("import: %w", err)
	}
	return nil
}
