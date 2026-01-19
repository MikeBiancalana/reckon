package logger

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestInitializeWithConfig_TUIMode(t *testing.T) {

	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	homeDir := tmpDir

	// Override home directory for testing
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", homeDir)
	defer os.Setenv("HOME", originalHome)

	cfg := Config{
		Level:   "DEBUG",
		Format:  "text",
		File:    "", // Let it auto-generate
		TUIMode: true,
	}

	InitializeWithConfig(cfg)

	// Verify logger is initialized
	if logger == nil {
		t.Fatal("Logger should be initialized")
	}

	// Verify log level
	if logLevel != slog.LevelDebug {
		t.Errorf("Expected log level DEBUG, got %v", logLevel)
	}

	// Verify log file was set
	if logFile == "" {
		t.Error("Log file should be set in TUI mode")
	}

	expectedLogFile := filepath.Join(homeDir, ".reckon", "logs", "reckon.log")
	if logFile != expectedLogFile {
		t.Errorf("Expected log file %s, got %s", expectedLogFile, logFile)
	}

	// Verify TUI mode is set
	if !tuiMode {
		t.Error("TUI mode should be true")
	}

	// Verify log directory was created
	logDir := filepath.Join(homeDir, ".reckon", "logs")
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		t.Errorf("Log directory should be created: %s", logDir)
	}
}

func TestInitializeWithConfig_NonTUIMode(t *testing.T) {

	cfg := Config{
		Level:   "INFO",
		Format:  "json",
		File:    "",
		TUIMode: false,
	}

	InitializeWithConfig(cfg)

	// Verify logger is initialized
	if logger == nil {
		t.Fatal("Logger should be initialized")
	}

	// Verify log level
	if logLevel != slog.LevelInfo {
		t.Errorf("Expected log level INFO, got %v", logLevel)
	}

	// Verify log file is NOT set in non-TUI mode without explicit file
	if logFile != "" {
		t.Errorf("Log file should not be set in non-TUI mode, got %s", logFile)
	}

	// Verify TUI mode is false
	if tuiMode {
		t.Error("TUI mode should be false")
	}

	// Verify log format
	if logFormat != "json" {
		t.Errorf("Expected log format json, got %s", logFormat)
	}
}

func TestInitializeWithConfig_ExplicitLogFile(t *testing.T) {

	tmpDir := t.TempDir()
	customLogFile := filepath.Join(tmpDir, "custom.log")

	cfg := Config{
		Level:   "WARN",
		Format:  "text",
		File:    customLogFile,
		TUIMode: false,
	}

	InitializeWithConfig(cfg)

	// Verify logger is initialized
	if logger == nil {
		t.Fatal("Logger should be initialized")
	}

	// Verify log level
	if logLevel != slog.LevelWarn {
		t.Errorf("Expected log level WARN, got %v", logLevel)
	}

	// Verify explicit log file is used
	if logFile != customLogFile {
		t.Errorf("Expected log file %s, got %s", customLogFile, logFile)
	}

	// Verify log directory was created
	if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
		t.Errorf("Log directory should exist: %s", tmpDir)
	}
}

func TestLogLevelParsing(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"DEBUG", slog.LevelDebug},
		{"debug", slog.LevelDebug},
		{"INFO", slog.LevelInfo},
		{"info", slog.LevelInfo},
		{"WARN", slog.LevelWarn},
		{"warn", slog.LevelWarn},
		{"WARNING", slog.LevelWarn},
		{"ERROR", slog.LevelError},
		{"error", slog.LevelError},
		{"", slog.LevelInfo},        // default
		{"invalid", slog.LevelInfo}, // default
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			cfg := Config{
				Level:   tt.input,
				Format:  "text",
				File:    "",
				TUIMode: false,
			}

			InitializeWithConfig(cfg)

			if logLevel != tt.expected {
				t.Errorf("For input %q, expected level %v, got %v", tt.input, tt.expected, logLevel)
			}
		})
	}
}

func TestGetters(t *testing.T) {
	cfg := Config{
		Level:   "DEBUG",
		Format:  "json",
		File:    "/tmp/test.log",
		TUIMode: true,
	}

	InitializeWithConfig(cfg)

	if GetLevel() != slog.LevelDebug {
		t.Errorf("GetLevel() returned %v, expected %v", GetLevel(), slog.LevelDebug)
	}

	if GetFormat() != "json" {
		t.Errorf("GetFormat() returned %s, expected json", GetFormat())
	}

	if GetLogFile() != "/tmp/test.log" {
		t.Errorf("GetLogFile() returned %s, expected /tmp/test.log", GetLogFile())
	}

	if !IsTUIMode() {
		t.Error("IsTUIMode() returned false, expected true")
	}
}

func TestLoggingFunctions(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	cfg := Config{
		Level:   "DEBUG",
		Format:  "text",
		File:    logFile,
		TUIMode: false,
	}

	InitializeWithConfig(cfg)

	// Test logging functions don't panic
	Debug("test debug", "key", "value")
	Info("test info", "key", "value")
	Warn("test warn", "key", "value")
	Error("test error", "key", "value")

	// Verify log file was created and has content
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	contentStr := string(content)

	// Check that log messages are present
	if !strings.Contains(contentStr, "test debug") {
		t.Error("Log file should contain 'test debug'")
	}
	if !strings.Contains(contentStr, "test info") {
		t.Error("Log file should contain 'test info'")
	}
	if !strings.Contains(contentStr, "test warn") {
		t.Error("Log file should contain 'test warn'")
	}
	if !strings.Contains(contentStr, "test error") {
		t.Error("Log file should contain 'test error'")
	}
}

// TestConcurrentAccess verifies thread-safety of concurrent logging operations
func TestConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "concurrent.log")

	cfg := Config{
		Level:   "DEBUG",
		Format:  "text",
		File:    logFile,
		TUIMode: false,
	}

	if err := InitializeWithConfig(cfg); err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}

	// Run concurrent logging operations
	const numGoroutines = 50
	const numLogsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < numLogsPerGoroutine; j++ {
				Debug("concurrent debug", "goroutine", goroutineID, "iteration", j)
				Info("concurrent info", "goroutine", goroutineID, "iteration", j)
				Warn("concurrent warn", "goroutine", goroutineID, "iteration", j)
				Error("concurrent error", "goroutine", goroutineID, "iteration", j)
			}
		}(i)
	}

	wg.Wait()

	// Verify log file was created and has content
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if len(content) == 0 {
		t.Error("Log file should have content after concurrent writes")
	}

	// Verify we have the expected number of log entries (roughly)
	lines := strings.Split(string(content), "\n")
	expectedMinLines := numGoroutines * numLogsPerGoroutine * 4 // 4 log levels
	// Allow some margin for missing newlines or formatting
	if len(lines) < expectedMinLines/2 {
		t.Errorf("Expected at least %d log lines, got %d", expectedMinLines/2, len(lines))
	}
}

// TestConcurrentGetters verifies thread-safety of concurrent getter operations
func TestConcurrentGetters(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "getters.log")

	cfg := Config{
		Level:   "WARN",
		Format:  "json",
		File:    logFile,
		TUIMode: true,
	}

	if err := InitializeWithConfig(cfg); err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}

	const numGoroutines = 100
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Track any panics or inconsistent reads
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errors <- fmt.Errorf("goroutine %d panicked: %v", goroutineID, r)
				}
			}()

			// Call all getter functions concurrently
			_ = GetLogger()
			level := GetLevel()
			format := GetFormat()
			file := GetLogFile()
			tui := IsTUIMode()

			// Verify values are consistent
			if level != slog.LevelWarn {
				errors <- fmt.Errorf("goroutine %d: expected log level WARN, got %v", goroutineID, level)
			}
			if format != "json" {
				errors <- fmt.Errorf("goroutine %d: expected format json, got %s", goroutineID, format)
			}
			if file != logFile {
				errors <- fmt.Errorf("goroutine %d: expected log file %s, got %s", goroutineID, logFile, file)
			}
			if !tui {
				errors <- fmt.Errorf("goroutine %d: expected TUI mode true, got false", goroutineID)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for any errors
	for err := range errors {
		t.Error(err)
	}
}

// TestConcurrentInitialization verifies thread-safety during concurrent initialization
func TestConcurrentInitialization(t *testing.T) {
	tmpDir := t.TempDir()

	const numGoroutines = 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errors <- fmt.Errorf("goroutine %d panicked: %v", goroutineID, r)
				}
			}()

			cfg := Config{
				Level:   "INFO",
				Format:  "text",
				File:    filepath.Join(tmpDir, fmt.Sprintf("concurrent-%d.log", goroutineID)),
				TUIMode: false,
			}

			if err := InitializeWithConfig(cfg); err != nil {
				errors <- fmt.Errorf("goroutine %d: initialization failed: %v", goroutineID, err)
			}

			// Try to use the logger immediately
			Info("test from goroutine", "id", goroutineID)
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for any errors
	for err := range errors {
		t.Error(err)
	}
}

// TestTUIModeFailure verifies that TUI mode failures are explicit
func TestTUIModeFailure(t *testing.T) {
	// Create a config with TUI mode but no file (and make home dir lookup fail)
	// We'll use an invalid path that can't be created
	cfg := Config{
		Level:   "INFO",
		Format:  "text",
		File:    "/proc/invalid/path/that/cannot/be/created.log", // Invalid path
		TUIMode: true,
	}

	err := InitializeWithConfig(cfg)
	if err == nil {
		t.Error("Expected error for TUI mode with invalid log file path, got nil")
	}

	if !strings.Contains(err.Error(), "TUI mode requires file-based logging") {
		t.Errorf("Expected error message about TUI mode requirement, got: %v", err)
	}
}

// TestClose verifies that Close() properly closes the lumberjack logger
func TestClose(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "close-test.log")

	cfg := Config{
		Level:   "INFO",
		Format:  "text",
		File:    logFile,
		TUIMode: false,
	}

	if err := InitializeWithConfig(cfg); err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}

	// Write some logs
	Info("test log before close")

	// Close should not error
	if err := Close(); err != nil {
		t.Errorf("Close() returned error: %v", err)
	}

	// Verify log file exists
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		t.Error("Log file should exist after Close()")
	}

	// Multiple Close() calls should be safe
	if err := Close(); err != nil {
		t.Errorf("Second Close() returned error: %v", err)
	}
}

// TestReinitialize verifies that reinitialization works correctly
func TestReinitialize(t *testing.T) {
	// First initialization
	cfg1 := Config{Level: "INFO", Format: "text", File: "", TUIMode: false}
	if err := InitializeWithConfig(cfg1); err != nil {
		t.Fatalf("First init failed: %v", err)
	}

	if GetLevel() != slog.LevelInfo {
		t.Errorf("Expected INFO level, got %v", GetLevel())
	}

	// Reinitialize with different config
	cfg2 := Config{Level: "DEBUG", Format: "json", File: filepath.Join(t.TempDir(), "test.log"), TUIMode: false}
	if err := InitializeWithConfig(cfg2); err != nil {
		t.Fatalf("Reinitialization failed: %v", err)
	}

	// Verify new config is applied
	if GetLevel() != slog.LevelDebug {
		t.Errorf("Expected DEBUG level after reinit, got %v", GetLevel())
	}
	if GetFormat() != "json" {
		t.Errorf("Expected json format after reinit, got %s", GetFormat())
	}
}
