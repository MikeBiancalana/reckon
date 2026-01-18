package logger

import (
	"log/slog"
	"os"
	"strings"
)

var (
	logger        *slog.Logger
	logLevel      slog.Level
	logFormat     string
	isInitialized bool
)

func init() {
	Initialize()
}

func Initialize() {
	if isInitialized {
		return
	}

	levelStr := os.Getenv("LOG_LEVEL")
	if levelStr == "" {
		levelStr = os.Getenv("RECKON_DEBUG")
		if levelStr == "1" || levelStr == "true" {
			levelStr = "DEBUG"
		} else {
			levelStr = "INFO"
		}
	}

	logFormat = os.Getenv("LOG_FORMAT")
	if logFormat == "" {
		logFormat = "text"
	}
	logFormat = strings.ToLower(logFormat)

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

	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	if logFormat == "json" {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}

	logger = slog.New(handler)
	isInitialized = true
}

func GetLogger() *slog.Logger {
	if !isInitialized {
		Initialize()
	}
	return logger
}

func GetLevel() slog.Level {
	if !isInitialized {
		Initialize()
	}
	return logLevel
}

func GetFormat() string {
	if !isInitialized {
		Initialize()
	}
	return logFormat
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
