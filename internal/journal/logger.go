package journal

import "log/slog"

// DefaultLogger returns a non-nil logger, defaulting to slog.Default()
func DefaultLogger(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return slog.Default()
	}
	return logger
}
