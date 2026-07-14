package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/MikeBiancalana/reckon/internal/logger"
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
	dateFlag     string
	quietFlag    bool
	logFileFlag  string
	logLevelFlag string
	jsonFlag     bool
	ndjsonFlag   bool
	vaultFlag    string
)

// buildLoggerConfig creates a logger configuration from flags and environment variables.
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

// RootCmd is the root command for the CLI. Bare invocation (no subcommand) prints help.
var RootCmd = &cobra.Command{
	Use:   "rk",
	Short: "Reckon - CLI Productivity System",
	Long:  `A terminal-based productivity tool combining daily journaling, task management, and knowledge base.`,
	// No RunE: bare "rk" prints help (git-style).
}

func init() {
	// PersistentPreRunE replaces cobra.OnInitialize: it runs only for actual
	// commands (not --help), validating flag exclusivity before logger init.
	RootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		// Mutually exclusive output mode flags
		if jsonFlag && ndjsonFlag {
			return fmt.Errorf("--json and --ndjson are mutually exclusive")
		}

		return initLoggerE()
	}

	// Persistent flags — available to all subcommands
	RootCmd.PersistentFlags().StringVar(&dateFlag, "date", "", "Date to operate on in YYYY-MM-DD format")
	RootCmd.PersistentFlags().BoolVarP(&quietFlag, "quiet", "q", false, "Suppress non-essential output")
	RootCmd.PersistentFlags().StringVar(&logFileFlag, "log-file", "", "Path to log file (default: ~/.reckon/logs/reckon.log in TUI mode, stderr otherwise)")
	RootCmd.PersistentFlags().StringVar(&logLevelFlag, "log-level", "", "Log level: DEBUG, INFO, WARN, ERROR (default: INFO)")
	RootCmd.PersistentFlags().BoolVar(&jsonFlag, "json", false, "Output as JSON")
	RootCmd.PersistentFlags().BoolVar(&ndjsonFlag, "ndjson", false, "Output as newline-delimited JSON")
	RootCmd.PersistentFlags().StringVar(&vaultFlag, "vault", "", "Override vault directory (default: $RECKON_VAULT or ~/reckon)")

	RootCmd.AddCommand(GetNoteCommand())
	RootCmd.AddCommand(todayCmd)
	RootCmd.AddCommand(addCmd)
	RootCmd.AddCommand(todoCmd)
	RootCmd.AddCommand(queryCmd)
	RootCmd.AddCommand(indexCmd)
	RootCmd.AddCommand(adoptCmd)
	RootCmd.AddCommand(importCmd)
}

// initLoggerE initializes the logger with command-line flags.
// Returns a wrapped error instead of calling os.Exit (per REVIEW_PATTERNS).
func initLoggerE() error {
	cfg := buildLoggerConfig(false)
	if err := logger.InitializeWithConfig(cfg); err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	logger.Info("reckon initialized", "version", "dev", "log_file", logger.GetLogFile(), "log_level", cfg.Level)
	return nil
}

// getEffectiveDate returns the date to operate on, either from --date flag or today.
func getEffectiveDate() (string, error) {
	if dateFlag != "" {
		if _, err := time.Parse("2006-01-02", dateFlag); err != nil {
			return "", fmt.Errorf("invalid date format: %s (expected YYYY-MM-DD)", dateFlag)
		}
		return dateFlag, nil
	}
	return time.Now().Format("2006-01-02"), nil
}

// Execute runs the root command with git-style external dispatch.
// It attempts to dispatch to an external rk-<verb> binary before falling
// through to cobra's built-in command tree. Returns an error on failure;
// main should call os.Exit(1) on non-nil return.
func Execute() error {
	defer logger.Close()

	handled, err := maybeDispatch(os.Args[1:])
	if handled {
		if err != nil {
			fmt.Fprintf(os.Stderr, "rk: %v\n", err)
		}
		return err
	}

	return RootCmd.Execute()
}
