package cli

import (
	"fmt"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/index"
	"github.com/MikeBiancalana/reckon/internal/logger"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

// tuiCmd launches the persistent 4-pane `rk tui` porcelain: reads exclusively
// through the SQLite index's public views and writes exclusively by calling
// the existing unexported CLI verb functions (addDurableTodo,
// dispatchTodayAct, appendLogEntry, createNote), reconciling after every
// mutation (plan.md, reckon-fnqs.8). Mirrors the pre-deletion (reckon-fnqs.1)
// stubs.go shape but opens its own index directly rather than depending on
// PersistentPreRunE-initialized package-level services — hence no
// requiresDB annotation.
var tuiCmd = &cobra.Command{
	Use:          "tui",
	Short:        "Launch the interactive terminal UI",
	Long:         "Launch the full-screen terminal user interface: a persistent 4-pane porcelain (agenda, todos, log, notes) over the vault index.",
	SilenceUsage: true,
	Args:         cobra.NoArgs,
	RunE:         runTUIE,
}

func runTUIE(cmd *cobra.Command, args []string) error {
	// Reconfigure the logger for TUI mode: the alt-screen suppresses
	// interleaved log lines (mirrors the retired stubs.go behavior).
	if err := logger.InitializeWithConfig(buildLoggerConfig(true)); err != nil {
		return fmt.Errorf("tui: initialize logger: %w", err)
	}

	cfg, err := config.LoadWithOverrides(vaultFlag, "")
	if err != nil {
		return fmt.Errorf("tui: load config: %w", err)
	}

	ix, err := index.Open(cfg)
	if err != nil {
		return fmt.Errorf("tui: open index: %w", err)
	}
	defer ix.Close()

	if _, err := ix.Reconcile(); err != nil {
		return fmt.Errorf("tui: reconcile index: %w", err)
	}

	model := newTUIModel(ix, cfg)
	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

// newTUIModel constructs the top-level model and its 4 pane wrappers.
func newTUIModel(ix *index.Index, cfg *config.Config) *tuiModel {
	return &tuiModel{
		ix:       ix,
		cfg:      cfg,
		vaultDir: cfg.VaultDir,
		agenda:   newAgendaPane(),
		todos:    newTodosPane(),
		log:      newLogPane(),
		notes:    newNotesPane(),
	}
}
