package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/MikeBiancalana/reckon/internal/logger"
	"github.com/MikeBiancalana/reckon/internal/storage"
	"github.com/MikeBiancalana/reckon/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

// Exit codes follow standard UNIX conventions:
//   - 0: Success
//   - 1: General error (database failure, I/O error)
//   - 2: Usage error (invalid flags, missing arguments)
//   - 3: Not found (resource does not exist)
const (
	ExitCodeSuccess    = 0
	ExitCodeGeneralErr = 1
	ExitCodeUsageErr   = 2
	ExitCodeNotFound   = 3
)

var (
	service            *journal.Service
	journalTaskService *journal.TaskService
	dateFlag           string
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

	// Add global flags
	RootCmd.Flags().StringVar(&dateFlag, "date", "", "Date to operate on in YYYY-MM-DD format")

	// Add subcommands
	RootCmd.AddCommand(GetLogCommand())
	RootCmd.AddCommand(todayCmd)
	RootCmd.AddCommand(weekCmd)
	RootCmd.AddCommand(rebuildCmd)
	RootCmd.AddCommand(GetReviewCommand())
	RootCmd.AddCommand(GetScheduleCommand())
	RootCmd.AddCommand(GetTaskCommand())
	RootCmd.AddCommand(GetWinCommand())
}

// initService initializes the journal service
func initService() {
	dbPath, err := config.DatabasePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting database path: %v\n", err)
		os.Exit(ExitCodeGeneralErr)
	}

	db, err := storage.NewDatabase(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		os.Exit(ExitCodeGeneralErr)
	}

	log := logger.GetLogger()
	repo := journal.NewRepository(db, log)
	fileStore := storage.NewFileStore()
	service = journal.NewService(repo, fileStore, log)

	journalTaskRepo := journal.NewTaskRepository(db, log)
	journalTaskService = journal.NewTaskService(journalTaskRepo, fileStore, log)
}

// getEffectiveDate returns the date to operate on, either from --date flag or today
func getEffectiveDate() (string, error) {
	if dateFlag != "" {
		if _, err := time.Parse("2006-01-02", dateFlag); err != nil {
			return "", fmt.Errorf("invalid date format: %s (expected YYYY-MM-DD)", dateFlag)
		}
		return dateFlag, nil
	}
	return time.Now().Format("2006-01-02"), nil
}

// Execute runs the root command
func Execute() error {
	return RootCmd.Execute()
}
