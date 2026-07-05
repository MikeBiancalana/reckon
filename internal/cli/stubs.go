package cli

import (
	"fmt"

	"github.com/MikeBiancalana/reckon/internal/logger"
	"github.com/MikeBiancalana/reckon/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

// tuiCmd launches the interactive TUI. It requires a live database (requiresDB=true)
// so PersistentPreRunE will initialise services before RunE executes.
var tuiCmd = &cobra.Command{
	Use:         "tui",
	Short:       "Launch the interactive terminal UI",
	Long:        "Launch the full-screen terminal user interface for journaling and task management.",
	Annotations: map[string]string{"requiresDB": "true"},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Reconfigure logger for TUI mode (alt-screen suppresses log lines)
		cfg := buildLoggerConfig(true)
		if err := logger.InitializeWithConfig(cfg); err != nil {
			return fmt.Errorf("failed to initialize logger for TUI mode: %w", err)
		}

		model := tui.NewModel(journalService)
		if journalTaskService != nil {
			model.SetJournalTaskService(journalTaskService)
		}
		if notesService != nil {
			model.SetNotesService(notesService)
		}
		p := tea.NewProgram(model, tea.WithAltScreen())
		_, err := p.Run()
		return err
	},
}

// addCmd is a v1 stub for the "rk add" command family.
var addCmd = &cobra.Command{
	Use:         "add",
	Short:       "Add a new node to the vault",
	Long:        "Add a new node (fact, task, event, …) to the reckon vault. (v1 stub — not yet implemented)",
	Annotations: map[string]string{"requiresDB": "false"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("add: %w", errNotImplemented)
	},
}

// errNotImplemented is the sentinel returned by v1 stub commands.
var errNotImplemented = fmt.Errorf("not yet implemented (v1-T0 stub)")
