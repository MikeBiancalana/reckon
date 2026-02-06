package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/MikeBiancalana/reckon/internal/logger"
	notessvc "github.com/MikeBiancalana/reckon/internal/service"
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
	journalService     *journal.Service
	journalTaskService *journal.TaskService
	notesService       *notessvc.NotesService
	dateFlag           string
	quietFlag          bool
	logFileFlag        string
	logLevelFlag       string
)

// buildLoggerConfig creates a logger configuration from flags and environment variables
func buildLoggerConfig(isTUIMode bool) logger.Config {
	cfg := logger.Config{
		Level:   logLevelFlag,
		Format:  os.Getenv("LOG_FORMAT"),
		File:    logFileFlag,
		TUIMode: isTUIMode,
	}

	if cfg.Level == "" {
		cfg.Level = os.Getenv("LOG_LEVEL")
		if cfg.Level == "" {
			debugVal := os.Getenv("RECKON_DEBUG")
			if debugVal == "1" || debugVal == "true" {
				cfg.Level = "DEBUG"
			} else {
				cfg.Level = "INFO"
			}
		}
	}

	if cfg.Format == "" {
		cfg.Format = "text"
	}

	return cfg
}

// RootCmd is the root command for the CLI
var RootCmd = &cobra.Command{
	Use:   "rk",
	Short: "Reckon - CLI Productivity System",
	Long:  `A terminal-based productivity tool combining daily journaling, task management, and knowledge base.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Reconfigure logger for TUI mode
		cfg := buildLoggerConfig(true)

		if err := logger.InitializeWithConfig(cfg); err != nil {
			return fmt.Errorf("failed to initialize logger for TUI mode: %w", err)
		}

		// Default behavior: launch TUI
		model := tui.NewModel(journalService)
		if journalTaskService != nil {
			model.SetJournalTaskService(journalTaskService)
		}
		p := tea.NewProgram(model, tea.WithAltScreen())
		_, err := p.Run()
		return err
	},
}

func init() {
	// Initialize logger and service
	cobra.OnInitialize(initLogger, initService)

	// Add global flags
	RootCmd.PersistentFlags().StringVar(&dateFlag, "date", "", "Date to operate on in YYYY-MM-DD format")
	RootCmd.PersistentFlags().BoolVarP(&quietFlag, "quiet", "q", false, "Suppress non-essential output")
	RootCmd.PersistentFlags().StringVar(&logFileFlag, "log-file", "", "Path to log file (default: ~/.reckon/logs/reckon.log in TUI mode, stderr otherwise)")
	RootCmd.PersistentFlags().StringVar(&logLevelFlag, "log-level", "", "Log level: DEBUG, INFO, WARN, ERROR (default: INFO)")

	// Add subcommands
	RootCmd.AddCommand(GetLogCommand())
	RootCmd.AddCommand(GetNoteCommand())
	RootCmd.AddCommand(GetNotesCommand())
	RootCmd.AddCommand(todayCmd)
	RootCmd.AddCommand(weekCmd)
	RootCmd.AddCommand(rebuildCmd)
	RootCmd.AddCommand(GetReviewCommand())
	RootCmd.AddCommand(GetScheduleCommand())
	RootCmd.AddCommand(GetTaskCommand())
	RootCmd.AddCommand(GetWinCommand())
}

// initLogger initializes the logger with command-line flags
// This is called via cobra.OnInitialize for non-TUI commands
func initLogger() {
	cfg := buildLoggerConfig(false)

	if err := logger.InitializeWithConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing logger: %v\n", err)
		os.Exit(ExitCodeGeneralErr)
	}

	// Log initialization for debugging
	logger.Info("reckon initialized", "version", "dev", "log_file", logger.GetLogFile(), "log_level", cfg.Level)
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

	// log := logger.GetLogger()
	repo := journal.NewRepository(db)
	fileStore := storage.NewFileStore()
	journalService = journal.NewService(repo, fileStore)

	journalTaskRepo := journal.NewTaskRepository(db)
	journalTaskService = journal.NewTaskService(journalTaskRepo, fileStore)

	// Initialize notes service
	notesRepo := notessvc.NewNotesRepository(db)
	notesService = notessvc.NewNotesService(notesRepo)
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
	defer logger.Close()
	return RootCmd.Execute()
}
