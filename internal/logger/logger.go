package logger

import (
	"log/slog"
	"os"
)

var debugLogger *slog.Logger
var isDebugEnabled bool

func init() {
	// Check if RECKON_DEBUG environment variable is set
	debugEnv := os.Getenv("RECKON_DEBUG")
	isDebugEnabled = debugEnv == "1" || debugEnv == "true"

	if isDebugEnabled {
		// Create a debug logger that writes to stderr with DEBUG level
		debugLogger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
	} else {
		// Create a no-op logger that discards everything
		debugLogger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelError + 1, // Level higher than ERROR to discard everything
		}))
	}
}

// Debug logs a debug-level message with optional key-value pairs
func Debug(msg string, args ...any) {
	if isDebugEnabled {
		debugLogger.Debug(msg, args...)
	}
}

// IsDebugEnabled returns whether debug logging is enabled
func IsDebugEnabled() bool {
	return isDebugEnabled
}
