package cli

import (
	"fmt"
	"os"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/MikeBiancalana/reckon/internal/logger"
	"github.com/MikeBiancalana/reckon/internal/storage"
	"github.com/MikeBiancalana/reckon/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var (
	service            *journal.Service
	journalTaskService *journal.TaskService
)

// RootCmd is the root command for the CLI
var RootCmd = &cobra.Command{
	Use:   "rk",
	Short: "Reckon - CLI Productivity System",
	Long:  `A terminal-based productivity tool combining daily journaling, task management, and knowledge base.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Default behavior: launch TUI
		model := tui.NewModel(service)
		if journalTaskService != nil {
			model.SetJournalTaskService(journalTaskService)
		}
		p := tea.NewProgram(model, tea.WithAltScreen())
		_, err := p.Run()
		return err
	},
}

func init() {
	// Initialize service
	cobra.OnInitialize(initService)

	// Add subcommands
	RootCmd.AddCommand(logCmd)
	RootCmd.AddCommand(todayCmd)
	RootCmd.AddCommand(weekCmd)
	RootCmd.AddCommand(rebuildCmd)
	RootCmd.AddCommand(GetReviewCommand())
	RootCmd.AddCommand(GetScheduleCommand())
	RootCmd.AddCommand(GetTaskCommand())
}

// initService initializes the journal service
func initService() {
	dbPath, err := config.DatabasePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting database path: %v\n", err)
		os.Exit(1)
	}

	db, err := storage.NewDatabase(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		os.Exit(1)
	}

	log := logger.GetLogger()
	repo := journal.NewRepository(db, log)
	fileStore := storage.NewFileStore()
	service = journal.NewService(repo, fileStore, log)

	journalTaskRepo := journal.NewTaskRepository(db, log)
	journalTaskService = journal.NewTaskService(journalTaskRepo, fileStore, log)
}

// Execute runs the root command
func Execute() error {
	return RootCmd.Execute()
}
