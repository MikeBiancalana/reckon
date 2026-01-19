// Package logger provides a thread-safe, globally accessible structured logger
// using slog with support for both file-based and stderr logging.
//
// Thread Safety:
//   - All public functions are thread-safe and can be called concurrently
//   - Initialization is protected by sync.Once to ensure it happens exactly once
//   - Configuration access is protected by RWMutex for concurrent reads
//   - The logger itself (slog.Logger) is inherently thread-safe
//
// Usage:
//   - Call Initialize() or InitializeWithConfig() before using the logger
//   - Use the package-level functions (Debug, Info, Warn, Error) for logging
//   - Call Close() when shutting down to flush buffered logs
package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	mu            sync.RWMutex
	logger        *slog.Logger
	logLevel      slog.Level
	logFormat     string
	logFile       string
	tuiMode       bool
	lumberjackLog *lumberjack.Logger
	initOnce      sync.Once
	closed        bool
)

// Config holds the logger configuration
type Config struct {
	Level   string // "DEBUG", "INFO", "WARN", "ERROR"
	Format  string // "text" or "json"
	File    string // Path to log file (empty means stderr, unless in TUI mode)
	TUIMode bool   // If true, automatically redirect to file
}

// ensureInitialized ensures the logger is initialized exactly once using sync.Once.
// This function is safe to call from multiple goroutines concurrently.
func ensureInitialized() {
	initOnce.Do(func() {
		cfg := Config{
			Level:   os.Getenv("LOG_LEVEL"),
			Format:  os.Getenv("LOG_FORMAT"),
			File:    "",
			TUIMode: false,
		}

		// Handle legacy RECKON_DEBUG env var
		if cfg.Level == "" {
			debugVal := os.Getenv("RECKON_DEBUG")
			if debugVal == "1" || debugVal == "true" {
				cfg.Level = "DEBUG"
			} else {
				cfg.Level = "INFO"
			}
		}

		if cfg.Format == "" {
			cfg.Format = "text"
		}

		mu.Lock()
		defer mu.Unlock()

		if err := initializeWithConfig(cfg); err != nil {
			// If initialization fails, fall back to stderr logging
			logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
			// Log the error to the fallback logger
			logger.Error("Failed to initialize logger with config, falling back to stderr",
				"error", err,
				"config", fmt.Sprintf("%+v", cfg))
		}
	})
}

// Initialize sets up the logger with default configuration (backward compatible).
// This function uses sync.Once to ensure initialization happens exactly once,
// even when called concurrently from multiple goroutines.
func Initialize() {
	ensureInitialized()
}

// InitializeWithConfig sets up the logger with a specific configuration.
// This allows CLI flags to override environment variables.
// Note: This marks initialization as complete, preventing ensureInitialized() from overwriting it.
func InitializeWithConfig(cfg Config) error {
	// Mark initOnce as done to prevent ensureInitialized() from running later
	var initErr error
	var didInit bool

	initOnce.Do(func() {
		didInit = true
		mu.Lock()
		defer mu.Unlock()

		// Reset closed flag when reinitializing
		closed = false

		initErr = initializeWithConfig(cfg)
	})

	// If initOnce already ran previously, update config directly
	if !didInit {
		mu.Lock()
		defer mu.Unlock()

		// Reset closed flag when reinitializing
		closed = false

		return initializeWithConfig(cfg)
	}

	return initErr
}

func initializeWithConfig(cfg Config) error {
	// Parse log level
	levelStr := cfg.Level
	if levelStr == "" {
		levelStr = "INFO"
	}

	switch strings.ToUpper(levelStr) {
	case "DEBUG":
		logLevel = slog.LevelDebug
	case "INFO":
		logLevel = slog.LevelInfo
	case "WARN", "WARNING":
		logLevel = slog.LevelWarn
	case "ERROR":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	// Parse log format
	logFormat = strings.ToLower(cfg.Format)
	if logFormat == "" {
		logFormat = "text"
	}

	// Determine output writer
	var writer io.Writer
	logFile = cfg.File
	tuiMode = cfg.TUIMode

	// If in TUI mode and no explicit log file, use default location
	if tuiMode && logFile == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("TUI mode requires file-based logging but cannot determine home directory: %w", err)
		}
		logDir := filepath.Join(homeDir, ".reckon", "logs")
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return fmt.Errorf("TUI mode requires file-based logging but cannot create log directory %s: %w", logDir, err)
		}
		logFile = filepath.Join(logDir, "reckon.log")
	}

	// Set up the writer
	if logFile != "" {
		// Ensure log directory exists
		logDir := filepath.Dir(logFile)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			if tuiMode {
				// In TUI mode, we MUST use file-based logging to prevent corruption
				return fmt.Errorf("TUI mode requires file-based logging but cannot create log directory %s: %w", logDir, err)
			}
			// In non-TUI mode, fall back to stderr if we can't create log directory
			writer = os.Stderr
			logFile = ""
		} else {
			// Use lumberjack for log rotation
			lumberjackLog = &lumberjack.Logger{
				Filename:   logFile,
				MaxSize:    10,   // megabytes
				MaxBackups: 3,    // keep 3 old log files
				MaxAge:     30,   // days
				Compress:   true, // compress old log files
			}
			writer = lumberjackLog
		}
	} else {
		if tuiMode {
			// TUI mode MUST have file-based logging
			return fmt.Errorf("TUI mode requires file-based logging but no log file was configured")
		}
		// Write to stderr (default)
		writer = os.Stderr
	}

	// Create handler
	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	if logFormat == "json" {
		handler = slog.NewJSONHandler(writer, opts)
	} else {
		handler = slog.NewTextHandler(writer, opts)
	}

	logger = slog.New(handler)
	return nil
}

func GetLogger() *slog.Logger {
	ensureInitialized()
	mu.RLock()
	defer mu.RUnlock()
	return logger
}

func GetLevel() slog.Level {
	ensureInitialized()
	mu.RLock()
	defer mu.RUnlock()
	return logLevel
}

func GetFormat() string {
	ensureInitialized()
	mu.RLock()
	defer mu.RUnlock()
	return logFormat
}

func GetLogFile() string {
	ensureInitialized()
	mu.RLock()
	defer mu.RUnlock()
	return logFile
}

func IsTUIMode() bool {
	ensureInitialized()
	mu.RLock()
	defer mu.RUnlock()
	return tuiMode
}

// Close closes the lumberjack logger if it exists, flushing any buffered log entries.
// This function is idempotent and can be called multiple times safely.
// After Close() is called, the logger remains usable but any file handle is released.
func Close() error {
	mu.Lock()
	defer mu.Unlock()

	// Return early if already closed
	if closed {
		return nil
	}

	var err error
	if lumberjackLog != nil {
		err = lumberjackLog.Close()
		lumberjackLog = nil
	}

	closed = true
	return err
}

func Debug(msg string, args ...any) {
	GetLogger().Debug(msg, args...)
}

func Info(msg string, args ...any) {
	GetLogger().Info(msg, args...)
}

func Warn(msg string, args ...any) {
	GetLogger().Warn(msg, args...)
}

func Error(msg string, args ...any) {
	GetLogger().Error(msg, args...)
}
