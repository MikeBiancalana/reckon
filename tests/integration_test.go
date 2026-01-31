//go:build integration

package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/MikeBiancalana/reckon/internal/storage"
)

func TestJournalCRUD(t *testing.T) {
	// Create temporary directory for test
	tmpDir, err := os.MkdirTemp("", "reckon-integration-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Set up test environment
	oldDataDir := os.Getenv("RECKON_DATA_DIR")
	defer func() {
		os.Setenv("RECKON_DATA_DIR", oldDataDir)
	}()

	testDataDir := filepath.Join(tmpDir, "data")
	os.Setenv("RECKON_DATA_DIR", testDataDir)

	// Initialize database
	dbPath, err := config.DatabasePath()
	if err != nil {
		t.Fatalf("Failed to get database path: %v", err)
	}

	db, err := storage.NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Create services
	journalRepo := journal.NewRepository(db)
	fileStore := storage.NewFileStore()
	journalSvc := journal.NewService(journalRepo, fileStore, nil)

	// Test creating and retrieving a journal
	testDate := "2023-12-25"
	j, err := journalSvc.GetByDate(testDate)
	if err != nil {
		t.Fatalf("Failed to get journal: %v", err)
	}

	// Add an intention
	err = journalSvc.AddIntention(j, "Test integration intention")
	if err != nil {
		t.Fatalf("Failed to add intention: %v", err)
	}

	// Reload and verify
	j, err = journalSvc.GetByDate(testDate)
	if err != nil {
		t.Fatalf("Failed to reload journal: %v", err)
	}

	if len(j.Intentions) != 1 {
		t.Errorf("Expected 1 intention, got %d", len(j.Intentions))
	}

	if j.Intentions[0].Text != "Test integration intention" {
		t.Errorf("Expected intention text 'Test integration intention', got '%s'", j.Intentions[0].Text)
	}
}

func TestFileSystemIntegration(t *testing.T) {
	// Create temporary directory for test
	tmpDir, err := os.MkdirTemp("", "reckon-integration-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Set up test environment
	oldDataDir := os.Getenv("RECKON_DATA_DIR")
	defer func() {
		os.Setenv("RECKON_DATA_DIR", oldDataDir)
	}()

	os.Setenv("RECKON_DATA_DIR", tmpDir)

	// Initialize services
	fileStore := storage.NewFileStore()

	// Test writing and reading a journal file
	testDate := "2023-12-25"
	testContent := `---
date: 2023-12-25
---

## Intentions

- [ ] Test intention

## Wins

- Test win

## Log

- 10:00 Test log entry`

	// Write the file
	err = fileStore.WriteJournalFile(testDate, testContent)
	if err != nil {
		t.Fatalf("Failed to write journal: %v", err)
	}

	// Read it back
	readContent, _, err := fileStore.ReadJournalFile(testDate)
	if err != nil {
		t.Fatalf("Failed to read journal: %v", err)
	}

	if readContent != testContent {
		t.Errorf("Read content doesn't match written content")
	}

	// Test file modification time
	_, fileInfo, err := fileStore.ReadJournalFile(testDate)
	if err != nil {
		t.Fatalf("Failed to get file info: %v", err)
	}

	if time.Since(fileInfo.LastModified) > time.Minute {
		t.Errorf("Modification time seems incorrect")
	}
}

func TestBackwardCompatibility(t *testing.T) {
	// Create temporary directory for test
	tmpDir, err := os.MkdirTemp("", "reckon-integration-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Set up test environment
	oldDataDir := os.Getenv("RECKON_DATA_DIR")
	defer func() {
		os.Setenv("RECKON_DATA_DIR", oldDataDir)
	}()

	testDataDir := filepath.Join(tmpDir, "data")
	os.Setenv("RECKON_DATA_DIR", testDataDir)

	// Initialize services
	dbPath, err := config.DatabasePath()
	if err != nil {
		t.Fatalf("Failed to get database path: %v", err)
	}

	db, err := storage.NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	journalRepo := journal.NewRepository(db)
	fileStore := storage.NewFileStore()
	journalSvc := journal.NewService(journalRepo, fileStore, nil)

	// Create an "old format" journal file (without Schedule section)
	oldJournalContent := `---
date: 2023-12-01
---

## Intentions

- [ ] Complete project proposal
- [x] Review team feedback
- [>] Schedule client meeting

## Wins

- Successfully deployed the new feature
- Received positive feedback from users
- Improved response time by 20%

## Log

- 09:00 Started work on bug fixes
- 10:30 [meeting:standup] Daily standup meeting
- 11:00 Fixed critical authentication bug
- 12:00 [break] Lunch break
- 13:00 Code review session
- 15:00 [task:deploy] Deployed to production
- 16:00 Documentation updates
- 17:00 End of day wrap-up`

	testDate := "2023-12-01"
	err = fileStore.WriteJournalFile(testDate, oldJournalContent)
	if err != nil {
		t.Fatalf("Failed to write old format journal: %v", err)
	}

	// Test that we can load and parse the old format journal
	j, err := journalSvc.GetByDate(testDate)
	if err != nil {
		t.Fatalf("Failed to load old format journal: %v", err)
	}

	// Verify basic content is parsed correctly
	if len(j.Intentions) != 3 {
		t.Errorf("Expected 3 intentions, got %d", len(j.Intentions))
	}

	if len(j.Wins) != 3 {
		t.Errorf("Expected 3 wins, got %d", len(j.Wins))
	}

	if len(j.LogEntries) != 8 {
		t.Errorf("Expected 8 log entries, got %d", len(j.LogEntries))
	}

	// Verify ScheduleItems is empty (since old format doesn't have it)
	if len(j.ScheduleItems) != 0 {
		t.Errorf("Expected 0 schedule items for old format, got %d", len(j.ScheduleItems))
	}

	// Test that we can still add new content to old format journals
	err = journalSvc.AddWin(j, "Additional win added to old journal")
	if err != nil {
		t.Fatalf("Failed to add win to old format journal: %v", err)
	}

	// Reload and verify
	j, err = journalSvc.GetByDate(testDate)
	if err != nil {
		t.Fatalf("Failed to reload journal after adding content: %v", err)
	}

	if len(j.Wins) != 4 {
		t.Errorf("Expected 4 wins after adding one, got %d", len(j.Wins))
	}

	if j.Wins[3].Text != "Additional win added to old journal" {
		t.Errorf("Expected added win text to match, got '%s'", j.Wins[3].Text)
	}
}

func TestScheduleAdditionToOldJournals(t *testing.T) {
	// Create temporary directory for test
	tmpDir, err := os.MkdirTemp("", "reckon-integration-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Set up test environment
	oldDataDir := os.Getenv("RECKON_DATA_DIR")
	defer func() {
		os.Setenv("RECKON_DATA_DIR", oldDataDir)
	}()

	testDataDir := filepath.Join(tmpDir, "data")
	os.Setenv("RECKON_DATA_DIR", testDataDir)

	// Initialize services
	dbPath, err := config.DatabasePath()
	if err != nil {
		t.Fatalf("Failed to get database path: %v", err)
	}

	db, err := storage.NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	journalRepo := journal.NewRepository(db)
	fileStore := storage.NewFileStore()
	journalSvc := journal.NewService(journalRepo, fileStore, nil)

	// Create an old format journal
	oldJournalContent := `---
date: 2023-12-02
---

## Intentions

- [ ] Finish documentation

## Wins

- Completed code review

## Log

- 09:00 Started documentation`

	testDate := "2023-12-02"
	err = fileStore.WriteJournalFile(testDate, oldJournalContent)
	if err != nil {
		t.Fatalf("Failed to write old format journal: %v", err)
	}

	// Load the journal
	j, err := journalSvc.GetByDate(testDate)
	if err != nil {
		t.Fatalf("Failed to load old format journal: %v", err)
	}

	// Add a schedule item (this should work even on old journals)
	err = journalSvc.AddScheduleItem(j, "10:00", "Team meeting")
	if err != nil {
		t.Fatalf("Failed to add schedule item to old journal: %v", err)
	}

	// Reload and verify
	j, err = journalSvc.GetByDate(testDate)
	if err != nil {
		t.Fatalf("Failed to reload journal after adding schedule: %v", err)
	}

	if len(j.ScheduleItems) != 1 {
		t.Errorf("Expected 1 schedule item, got %d", len(j.ScheduleItems))
	}

	if j.ScheduleItems[0].Content != "Team meeting" {
		t.Errorf("Expected schedule item content 'Team meeting', got '%s'", j.ScheduleItems[0].Content)
	}

	// Verify the time was parsed
	expectedTime, _ := time.Parse("2006-01-02 15:04", "2023-12-02 10:00")
	if !j.ScheduleItems[0].Time.Equal(expectedTime) {
		t.Errorf("Expected time %v, got %v", expectedTime, j.ScheduleItems[0].Time)
	}

	// Verify the file now contains the Schedule section
	content, _, err := fileStore.ReadJournalFile(testDate)
	if err != nil {
		t.Fatalf("Failed to read updated journal file: %v", err)
	}

	if !strings.Contains(content, "## Schedule") {
		t.Errorf("Expected journal file to contain '## Schedule' section")
	}

	if !strings.Contains(content, "- 10:00 Team meeting") {
		t.Errorf("Expected journal file to contain the schedule item")
	}
}

func TestJournalTaskServiceIntegration(t *testing.T) {
	// Create temporary directory for test
	tmpDir, err := os.MkdirTemp("", "reckon-integration-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Set up test environment
	oldDataDir := os.Getenv("RECKON_DATA_DIR")
	defer func() {
		os.Setenv("RECKON_DATA_DIR", oldDataDir)
	}()

	testDataDir := filepath.Join(tmpDir, "data")
	os.Setenv("RECKON_DATA_DIR", testDataDir)

	// Initialize services
	dbPath, err := config.DatabasePath()
	if err != nil {
		t.Fatalf("Failed to get database path: %v", err)
	}

	db, err := storage.NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	taskRepo := journal.NewTaskRepository(db, nil)
	fileStore := storage.NewFileStore()
	journalTaskSvc := journal.NewTaskService(taskRepo, fileStore, nil)

	// Create individual task files
	task1Content := `---
id: test-task-1
title: Test task 1
created: 2024-01-01
status: active
---

## Description

Test task 1 description

## Log

### 2024-01-01
  - note-1-task1 First note
`

	task2Content := `---
id: test-task-2
title: Test task 2 completed
created: 2024-01-01
status: done
---

## Description

Test task 2 description
`

	task3Content := `---
id: test-task-3
title: Test task 3 with notes
created: 2024-01-01
status: active
---

## Description

Test task 3 description

## Log

### 2024-01-01
  - note-1-task3 First note
  - note-2-task3 Second note
`

	// Get tasks directory
	tasksDir, err := config.TasksDir()
	if err != nil {
		t.Fatalf("Failed to get tasks directory: %v", err)
	}

	// Write task files
	err = os.WriteFile(filepath.Join(tasksDir, "test-task-1.md"), []byte(task1Content), 0644)
	if err != nil {
		t.Fatalf("Failed to write task1 file: %v", err)
	}

	err = os.WriteFile(filepath.Join(tasksDir, "test-task-2.md"), []byte(task2Content), 0644)
	if err != nil {
		t.Fatalf("Failed to write task2 file: %v", err)
	}

	err = os.WriteFile(filepath.Join(tasksDir, "test-task-3.md"), []byte(task3Content), 0644)
	if err != nil {
		t.Fatalf("Failed to write task3 file: %v", err)
	}

	// Test that GetAllTasks works
	tasks, err := journalTaskSvc.GetAllTasks()
	if err != nil {
		t.Fatalf("Failed to get all tasks: %v", err)
	}

	if len(tasks) != 3 {
		t.Errorf("Expected 3 tasks, got %d", len(tasks))
	}

	// Verify first task
	if tasks[0].ID != "test-task-1" {
		t.Errorf("Expected task ID 'test-task-1', got '%s'", tasks[0].ID)
	}
	if tasks[0].Text != "Test task 1" {
		t.Errorf("Expected task text 'Test task 1', got '%s'", tasks[0].Text)
	}
	if tasks[0].Status != journal.TaskOpen {
		t.Errorf("Expected task status Open, got %v", tasks[0].Status)
	}

	// Verify second task (completed)
	if tasks[1].Status != journal.TaskDone {
		t.Errorf("Expected second task to be Done, got %v", tasks[1].Status)
	}

	// Verify third task has notes
	if len(tasks[2].Notes) != 2 {
		t.Errorf("Expected 2 notes on third task, got %d", len(tasks[2].Notes))
	}

	// Test adding a new task
	_, err = journalTaskSvc.AddTask("New integration test task", []string{})
	if err != nil {
		t.Fatalf("Failed to add task: %v", err)
	}

	// Verify task was added
	tasks, err = journalTaskSvc.GetAllTasks()
	if err != nil {
		t.Fatalf("Failed to get tasks after adding: %v", err)
	}

	if len(tasks) != 4 {
		t.Errorf("Expected 4 tasks after adding one, got %d", len(tasks))
	}

	// Find the new task (don't assume order)
	found := false
	var newTask journal.Task
	for _, task := range tasks {
		if task.Text == "New integration test task" {
			newTask = task
			found = true
			break
		}
	}

	if !found {
		t.Errorf("New task not found")
	} else if newTask.Status != journal.TaskOpen {
		t.Errorf("Expected new task to be Open, got %v", newTask.Status)
	}

	// Verify task was added
	tasks, err = journalTaskSvc.GetAllTasks()
	if err != nil {
		t.Fatalf("Failed to get tasks after adding: %v", err)
	}

	if len(tasks) != 4 {
		t.Errorf("Expected 4 tasks after adding one, got %d", len(tasks))
	}

}

func TestDatabaseMigration(t *testing.T) {
	// Create temporary directory for test
	tmpDir, err := os.MkdirTemp("", "reckon-integration-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Set up test environment
	oldDataDir := os.Getenv("RECKON_DATA_DIR")
	defer func() {
		os.Setenv("RECKON_DATA_DIR", oldDataDir)
	}()

	testDataDir := filepath.Join(tmpDir, "data")
	os.Setenv("RECKON_DATA_DIR", testDataDir)

	// Initialize database - this should create tables automatically
	dbPath, err := config.DatabasePath()
	if err != nil {
		t.Fatalf("Failed to get database path: %v", err)
	}

	db, err := storage.NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Check that the database file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Errorf("Expected database file to be created")
	}

	// Try to query the database to ensure tables exist
	// Check journals table
	var count int
	err = db.DB().QueryRow("SELECT COUNT(*) FROM journals").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query journals table: %v", err)
	}

}

func TestEdgeCases(t *testing.T) {
	// Create temporary directory for test
	tmpDir, err := os.MkdirTemp("", "reckon-integration-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Set up test environment
	oldDataDir := os.Getenv("RECKON_DATA_DIR")
	defer func() {
		os.Setenv("RECKON_DATA_DIR", oldDataDir)
	}()

	testDataDir := filepath.Join(tmpDir, "data")
	os.Setenv("RECKON_DATA_DIR", testDataDir)

	// Initialize services
	dbPath, err := config.DatabasePath()
	if err != nil {
		t.Fatalf("Failed to get database path: %v", err)
	}

	db, err := storage.NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	journalRepo := journal.NewRepository(db)
	fileStore := storage.NewFileStore()
	journalSvc := journal.NewService(journalRepo, fileStore, nil)

	// Test journal with very long content
	longText := strings.Repeat("This is a very long line of text that should test the parser's ability to handle extended content. ", 19) + "This is a very long line of text that should test the parser's ability to handle extended content."
	testDate := "2023-12-03"

	j, err := journalSvc.GetByDate(testDate)
	if err != nil {
		t.Fatalf("Failed to get journal: %v", err)
	}

	err = journalSvc.AddIntention(j, longText)
	if err != nil {
		t.Fatalf("Failed to add long intention: %v", err)
	}

	// Reload and verify
	j, err = journalSvc.GetByDate(testDate)
	if err != nil {
		t.Fatalf("Failed to reload journal: %v", err)
	}

	if len(j.Intentions) != 1 {
		t.Errorf("Expected 1 intention, got %d", len(j.Intentions))
	}

	if j.Intentions[0].Text != longText {
		t.Errorf("Long text was not preserved correctly. Expected length %d, got length %d", len(longText), len(j.Intentions[0].Text))
		t.Logf("Expected: %q", longText)
		t.Logf("Got: %q", j.Intentions[0].Text)
	}

	// Test journal with empty sections
	emptyJournalContent := `---
date: 2023-12-04
---

## Intentions

## Wins

## Log
`

	testDate2 := "2023-12-04"
	err = fileStore.WriteJournalFile(testDate2, emptyJournalContent)
	if err != nil {
		t.Fatalf("Failed to write empty journal: %v", err)
	}

	j2, err := journalSvc.GetByDate(testDate2)
	if err != nil {
		t.Fatalf("Failed to load empty journal: %v", err)
	}

	// Should have empty slices, not nil
	if j2.Intentions == nil {
		t.Errorf("Intentions should be initialized as empty slice")
	}

	if j2.Wins == nil {
		t.Errorf("Wins should be initialized as empty slice")
	}

	if j2.LogEntries == nil {
		t.Errorf("LogEntries should be initialized as empty slice")
	}

	if j2.ScheduleItems == nil {
		t.Errorf("ScheduleItems should be initialized as empty slice")
	}
}

func TestCLICommands(t *testing.T) {
	// Create temporary directory for test
	tmpDir, err := os.MkdirTemp("", "reckon-integration-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Set up test environment
	oldDataDir := os.Getenv("RECKON_DATA_DIR")
	defer func() {
		os.Setenv("RECKON_DATA_DIR", oldDataDir)
	}()

	testDataDir := filepath.Join(tmpDir, "data")
	os.Setenv("RECKON_DATA_DIR", testDataDir)

	// Test task creation via CLI
	// Since we can't easily run the binary in test, let's test the service directly
	// But for integration, we can test that the CLI parsing works by testing the commands

	// For now, just test that we can initialize services (already tested above)
	// In a real scenario, we'd use exec.Command to run the binary
}

func TestLogNotePersistence(t *testing.T) {
	// This test verifies the fix for the bug where log notes and task notes
	// were not being written to markdown files due to closure capture issues.
	// The bug was that Go closures capture variables by reference, not by value.
	// When the Enter key handler reset m.noteLogEntryID before the async function
	// ran, the closure saw the empty value.

	// Create temporary directory for test
	tmpDir, err := os.MkdirTemp("", "reckon-integration-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Set up test environment
	oldDataDir := os.Getenv("RECKON_DATA_DIR")
	defer func() {
		os.Setenv("RECKON_DATA_DIR", oldDataDir)
	}()

	testDataDir := filepath.Join(tmpDir, "data")
	os.Setenv("RECKON_DATA_DIR", testDataDir)

	// Initialize services
	dbPath, err := config.DatabasePath()
	if err != nil {
		t.Fatalf("Failed to get database path: %v", err)
	}

	db, err := storage.NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	journalRepo := journal.NewRepository(db)
	fileStore := storage.NewFileStore()
	journalSvc := journal.NewService(journalRepo, fileStore, nil)

	// Create a journal with a log entry
	testDate := "2024-06-15"
	j, err := journalSvc.GetByDate(testDate)
	if err != nil {
		t.Fatalf("Failed to get journal: %v", err)
	}

	// Add a log entry
	err = journalSvc.AppendLog(j, "Test log entry for note persistence")
	if err != nil {
		t.Fatalf("Failed to add log entry: %v", err)
	}

	// Reload to get the entry ID
	j, err = journalSvc.GetByDate(testDate)
	if err != nil {
		t.Fatalf("Failed to reload journal: %v", err)
	}

	if len(j.LogEntries) != 1 {
		t.Fatalf("Expected 1 log entry, got %d", len(j.LogEntries))
	}

	logEntryID := j.LogEntries[0].ID

	// Now simulate what the TUI does: add a log note
	// This is the operation that was failing due to closure capture bug
	err = journalSvc.AddLogNote(j, logEntryID, "This is a test note")
	if err != nil {
		t.Fatalf("Failed to add log note: %v", err)
	}

	// Verify the note was added in memory
	if len(j.LogEntries[0].Notes) != 1 {
		t.Fatalf("Expected 1 note in memory, got %d", len(j.LogEntries[0].Notes))
	}

	// Verify the note text
	if j.LogEntries[0].Notes[0].Text != "This is a test note" {
		t.Errorf("Expected note text 'This is a test note', got '%s'", j.LogEntries[0].Notes[0].Text)
	}

	// Reload from disk and verify the note persists
	j, err = journalSvc.GetByDate(testDate)
	if err != nil {
		t.Fatalf("Failed to reload journal after adding note: %v", err)
	}

	if len(j.LogEntries) != 1 {
		t.Fatalf("Expected 1 log entry after reload, got %d", len(j.LogEntries))
	}

	if len(j.LogEntries[0].Notes) != 1 {
		t.Fatalf("Expected 1 note after reload, got %d", len(j.LogEntries[0].Notes))
	}

	if j.LogEntries[0].Notes[0].Text != "This is a test note" {
		t.Errorf("Note text not preserved after reload. Expected 'This is a test note', got '%s'", j.LogEntries[0].Notes[0].Text)
	}

	// Verify the markdown file contains the note
	content, _, err := fileStore.ReadJournalFile(testDate)
	if err != nil {
		t.Fatalf("Failed to read journal file: %v", err)
	}

	if !strings.Contains(content, "This is a test note") {
		t.Errorf("Journal file should contain the note text. Content:\n%s", content)
	}

	// Verify the note format in the file (IDs are no longer in markdown)
	expectedNoteFormat := "  - This is a test note"
	if !strings.Contains(content, expectedNoteFormat) {
		t.Errorf("Journal file should contain properly formatted note. Expected format:\n%s\n\nActual content:\n%s", expectedNoteFormat, content)
	}

	// Add another note to test multiple notes
	err = journalSvc.AddLogNote(j, logEntryID, "Second test note")
	if err != nil {
		t.Fatalf("Failed to add second log note: %v", err)
	}

	// Reload and verify
	j, err = journalSvc.GetByDate(testDate)
	if err != nil {
		t.Fatalf("Failed to reload journal after second note: %v", err)
	}

	if len(j.LogEntries[0].Notes) != 2 {
		t.Fatalf("Expected 2 notes after adding second, got %d", len(j.LogEntries[0].Notes))
	}

	// Verify markdown file has both notes
	content, _, err = fileStore.ReadJournalFile(testDate)
	if err != nil {
		t.Fatalf("Failed to read journal file: %v", err)
	}

	if !strings.Contains(content, "This is a test note") {
		t.Errorf("Journal file should contain first note")
	}

	if !strings.Contains(content, "Second test note") {
		t.Errorf("Journal file should contain second note")
	}
}
