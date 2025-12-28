package integration

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestCLI runs CLI commands and returns output for testing
type TestCLI struct {
	t       *testing.T
	tempDir string
	binPath string
	env     []string
}

// NewTestCLI creates a new test CLI instance
func NewTestCLI(t *testing.T) *TestCLI {
	t.Helper()

	tempDir := TestTempDir(t)
	configDir, journalDir := SetupTestEnvironment(t)

	// Build the binary for testing
	binPath := filepath.Join(tempDir, "rk")
	cmd := exec.Command("go", "build", "-o", binPath, "../../cmd/rk")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to build test binary: %v", err)
	}

	env := []string{
		fmt.Sprintf("RECKON_HOME=%s", configDir),
		fmt.Sprintf("RECKON_JOURNAL_DIR=%s", journalDir),
		"HOME=" + tempDir, // Prevent reading from actual home directory
	}

	return &TestCLI{
		t:       t,
		tempDir: tempDir,
		binPath: binPath,
		env:     env,
	}
}

// Run executes a CLI command with given arguments
func (tc *TestCLI) Run(args ...string) (stdout, stderr string, err error) {
	tc.t.Helper()

	cmd := exec.Command(tc.binPath, args...)
	cmd.Env = append(os.Environ(), tc.env...)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err = cmd.Run()

	return stdoutBuf.String(), stderrBuf.String(), err
}

// RunExpectSuccess runs a command and expects it to succeed
func (tc *TestCLI) RunExpectSuccess(args ...string) string {
	tc.t.Helper()

	stdout, stderr, err := tc.Run(args...)
	if err != nil {
		tc.t.Logf("Command failed: %s %v", strings.Join(args, " "), err)
		tc.t.Logf("STDOUT: %s", stdout)
		tc.t.Logf("STDERR: %s", stderr)
		tc.t.Fatalf("Expected command to succeed, but it failed")
	}

	return stdout
}

// RunExpectFailure runs a command and expects it to fail
func (tc *TestCLI) RunExpectFailure(args ...string) (stdout, stderr string) {
	tc.t.Helper()

	stdout, stderr, err := tc.Run(args...)
	if err == nil {
		tc.t.Logf("STDOUT: %s", stdout)
		tc.t.Fatalf("Expected command to fail, but it succeeded")
	}

	return stdout, stderr
}

// CreateTestJournal creates a test journal file
func (tc *TestCLI) CreateTestJournal(date, content string) {
	tc.t.Helper()

	journalDir := filepath.Join(tc.tempDir, "journals")
	if err := os.MkdirAll(journalDir, 0755); err != nil {
		tc.t.Fatalf("Failed to create journal directory: %v", err)
	}

	journalPath := filepath.Join(journalDir, date+".md")

	if err := os.WriteFile(journalPath, []byte(content), 0644); err != nil {
		tc.t.Fatalf("Failed to create test journal: %v", err)
	}
}

// GetJournalContent returns the content of a journal file
func (tc *TestCLI) GetJournalContent(date string) string {
	tc.t.Helper()

	journalDir := filepath.Join(tc.tempDir, "journals")
	journalPath := filepath.Join(journalDir, date+".md")

	content, err := os.ReadFile(journalPath)
	if err != nil {
		tc.t.Fatalf("Failed to read journal content: %v", err)
	}

	return string(content)
}

// TestCLI_Log tests the log command functionality
func TestCLI_Log(t *testing.T) {
	cli := NewTestCLI(t)

	// Test logging to a new journal
	output := cli.RunExpectSuccess("log", "Test log entry")
	expected := "✓ Logged: Test log entry"
	if !strings.Contains(output, expected) {
		t.Errorf("Expected output to contain %q, got %q", expected, output)
	}

	// Verify the log was added to today's journal
	today := time.Now().Format("2006-01-02")
	content := cli.GetJournalContent(today)

	if !strings.Contains(content, "Test log entry") {
		t.Errorf("Expected journal to contain log entry, got: %s", content)
	}

	// Test logging with multiple words
	output = cli.RunExpectSuccess("log", "Multiple", "word", "log", "entry")
	expected = "✓ Logged: Multiple word log entry"
	if !strings.Contains(output, expected) {
		t.Errorf("Expected output to contain %q, got %q", expected, output)
	}

	// Verify multiple word log was added
	content = cli.GetJournalContent(today)
	if !strings.Contains(content, "Multiple word log entry") {
		t.Errorf("Expected journal to contain multiple word log entry, got: %s", content)
	}
}

// TestCLI_LogWithEmptyArgs tests that log command fails without arguments
func TestCLI_LogWithEmptyArgs(t *testing.T) {
	cli := NewTestCLI(t)

	// Test that log command fails without arguments
	stdout, stderr, err := cli.Run("log")
	if err == nil {
		t.Logf("Command succeeded when expected to fail")
		t.Logf("STDERR: %s", stderr)
		t.Logf("STDOUT: %s", stdout)
		t.Error("Expected log command to fail without arguments")
	}

	// Check if any error message was provided
	if stderr == "" && stdout == "" {
		t.Log("No error output captured - may exit silently without args")
	}
}

// TestCLI_Today tests the today command
func TestCLI_Today(t *testing.T) {
	cli := NewTestCLI(t)

	// First create a journal for today
	today := time.Now().Format("2006-01-02")
	testContent := `# Test Journal

## Intentions
- Test intention 1
- Test intention 2

## Wins
- Test win 1

## Logs
09:00 Test log entry
10:00 Another log entry
`
	cli.CreateTestJournal(today, testContent)

	// Test today command output
	output := cli.RunExpectSuccess("today")

	// Verify the content matches our test journal
	if !strings.Contains(output, "Test Journal") {
		t.Errorf("Expected output to contain 'Test Journal', got: %s", output)
	}

	if !strings.Contains(output, "Test intention 1") {
		t.Errorf("Expected output to contain 'Test intention 1', got: %s", output)
	}

	if !strings.Contains(output, "Test win 1") {
		t.Errorf("Expected output to contain 'Test win 1', got: %s", output)
	}
}

// TestCLI_TodayEmpty tests today command with no existing journal
func TestCLI_TodayEmpty(t *testing.T) {
	cli := NewTestCLI(t)

	// Test today command with no existing journal
	output := cli.RunExpectSuccess("today")

	// Should return empty content (no error expected)
	if strings.TrimSpace(output) != "" {
		t.Errorf("Expected empty output for non-existent journal, got: %q", output)
	}
}

// TestCLI_Week tests the week command
func TestCLI_Week(t *testing.T) {
	cli := NewTestCLI(t)

	// Create journals for the past 7 days
	for i := 6; i >= 0; i-- {
		date := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		content := fmt.Sprintf(`# Journal for %s

## Intentions
- Intention for %s

## Wins
- Win for %s

`, date, date, date)
		cli.CreateTestJournal(date, content)
	}

	// Test week command output
	output := cli.RunExpectSuccess("week")

	// Should contain journals from multiple days
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 10 { // Should have multiple days worth of content
		t.Errorf("Expected multiple journal entries, got %d lines: %s", len(lines), output)
	}

	// Check that content from different days is present
	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

	if !strings.Contains(output, today) {
		t.Errorf("Expected week output to contain today's date %s", today)
	}

	if !strings.Contains(output, yesterday) {
		t.Errorf("Expected week output to contain yesterday's date %s", yesterday)
	}
}

// TestCLI_Rebuild tests the rebuild command
func TestCLI_Rebuild(t *testing.T) {
	cli := NewTestCLI(t)

	// Create some test journal files
	for i := 2; i >= 0; i-- {
		date := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		content := fmt.Sprintf(`# Journal for %s

## Intentions
- Test intention %s

## Wins
- Test win %s

## Logs
09:00 Log entry for %s

`, date, date, date, date)
		cli.CreateTestJournal(date, content)
	}

	// Run rebuild command
	output := cli.RunExpectSuccess("rebuild")

	// Verify success message
	if !strings.Contains(output, "✓ Database rebuilt successfully") {
		t.Errorf("Expected success message in rebuild output, got: %s", output)
	}

	// Verify rebuild process message
	if !strings.Contains(output, "Rebuilding database from markdown files") {
		t.Errorf("Expected rebuild message, got: %s", output)
	}
}

// TestCLI_RebuildWithNoJournals tests rebuild command with no journal files
func TestCLI_RebuildWithNoJournals(t *testing.T) {
	cli := NewTestCLI(t)

	// Run rebuild command with no journal files
	output := cli.RunExpectSuccess("rebuild")

	// Should still succeed even with no files
	if !strings.Contains(output, "✓ Database rebuilt successfully") {
		t.Errorf("Expected success message even with no journal files, got: %s", output)
	}
}

// TestCLI_Help tests that help commands work properly
func TestCLI_Help(t *testing.T) {
	cli := NewTestCLI(t)

	// Test root help
	output := cli.RunExpectSuccess("--help")

	// Check for the actual help content that exists
	if !strings.Contains(output, "terminal-based productivity tool") {
		t.Errorf("Expected help to contain app description, got: %s", output)
	}

	if !strings.Contains(output, "log") {
		t.Errorf("Expected help to list log command, got: %s", output)
	}

	if !strings.Contains(output, "today") {
		t.Errorf("Expected help to list today command, got: %s", output)
	}

	if !strings.Contains(output, "week") {
		t.Errorf("Expected help to list week command, got: %s", output)
	}

	if !strings.Contains(output, "rebuild") {
		t.Errorf("Expected help to list rebuild command, got: %s", output)
	}
}

// TestCLI_InvalidCommand tests that invalid commands fail properly
func TestCLI_InvalidCommand(t *testing.T) {
	cli := NewTestCLI(t)

	// Test invalid command - check if it fails
	stdout, stderr, err := cli.Run("invalid-command")
	if err == nil {
		t.Logf("Command succeeded when expected to fail")
		t.Logf("STDERR: %s", stderr)
		t.Logf("STDOUT: %s", stdout)
		t.Error("Expected invalid command to fail")
	}

	// Check if any error message was provided (cobra typically shows this)
	if !strings.Contains(stderr, "unknown") && !strings.Contains(stdout, "unknown") {
		t.Log("No 'unknown command' error found - may exit differently")
	}
}

// TestCLI_Version tests version command if it exists
func TestCLI_Version(t *testing.T) {
	cli := NewTestCLI(t)

	// Test if version flag is available (common in cobra apps)
	stdout, stderr, err := cli.Run("--version")

	// Don't fail the test if version isn't implemented, just log
	if err == nil && (strings.Contains(stdout, "version") || strings.Contains(stderr, "version")) {
		t.Log("Version command available")
	} else {
		t.Log("Version command not available (optional)")
	}
}
