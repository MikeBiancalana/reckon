package integration

import (
	"os"
	"path/filepath"
	"testing"
)

// TestTempDir creates a temporary directory for integration tests
func TestTempDir(t *testing.T) string {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "reckon-integration-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Clean up temp dir after test
	t.Cleanup(func() {
		os.RemoveAll(tempDir)
	})

	return tempDir
}

// SetupTestEnvironment creates a test environment with mock config and data
func SetupTestEnvironment(t *testing.T) (string, string) {
	t.Helper()

	tempDir := TestTempDir(t)

	// Create config directory
	configDir := filepath.Join(tempDir, ".reckon")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	// Create journal directory
	journalDir := filepath.Join(tempDir, "journals")
	if err := os.MkdirAll(journalDir, 0755); err != nil {
		t.Fatalf("Failed to create journal dir: %v", err)
	}

	// Set environment variables for test
	os.Setenv("RECKON_HOME", configDir)
	os.Setenv("RECKON_JOURNAL_DIR", journalDir)

	// Cleanup environment
	t.Cleanup(func() {
		os.Unsetenv("RECKON_HOME")
		os.Unsetenv("RECKON_JOURNAL_DIR")
	})

	return configDir, journalDir
}

// CreateTestJournalFile creates a test journal file with sample content
func CreateTestJournalFile(t *testing.T, journalDir, date, content string) string {
	t.Helper()

	// Ensure directory exists
	if err := os.MkdirAll(journalDir, 0755); err != nil {
		t.Fatalf("Failed to create journal directory: %v", err)
	}

	journalPath := filepath.Join(journalDir, date+".md")
	if err := os.WriteFile(journalPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test journal file: %v", err)
	}

	return journalPath
}
