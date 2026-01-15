package journal

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MikeBiancalana/reckon/internal/storage"
)

// setupLogNotesTestService creates a test service with in-memory database and temp directory
func setupLogNotesTestService(t *testing.T) (*Service, string) {
	t.Helper()

	// Create temp directory for test files
	tempDir, err := os.MkdirTemp("", "journal-lognotes-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	// Set up config to point to our test directory
	origDataDir := os.Getenv("RECKON_DATA_DIR")
	os.Setenv("RECKON_DATA_DIR", tempDir)
	t.Cleanup(func() {
		if origDataDir == "" {
			os.Unsetenv("RECKON_DATA_DIR")
		} else {
			os.Setenv("RECKON_DATA_DIR", origDataDir)
		}
	})

	// Create database
	dbPath := filepath.Join(tempDir, "test.db")
	db, err := storage.NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}

	// Create file store
	fileStore := storage.NewFileStore()

	// Create service
	repo := NewRepository(db)
	service := NewService(repo, fileStore)

	return service, tempDir
}

// cleanupLogNotesTestService cleans up test resources
func cleanupLogNotesTestService(t *testing.T, tempDir string) {
	t.Helper()
	os.RemoveAll(tempDir)
}

// TestAddLogNote_Success tests adding a note to a log entry
func TestAddLogNote_Success(t *testing.T) {
	service, tempDir := setupLogNotesTestService(t)
	defer cleanupLogNotesTestService(t, tempDir)

	// Create a journal with a log entry
	journal := NewJournal("2024-01-15")
	entry := NewLogEntry(time.Now(), "Test entry", EntryTypeLog, 0)
	journal.LogEntries = append(journal.LogEntries, *entry)

	// Add a note
	err := service.AddLogNote(journal, entry.ID, "First note")
	if err != nil {
		t.Fatalf("AddLogNote failed: %v", err)
	}

	// Verify note was added
	if len(journal.LogEntries[0].Notes) != 1 {
		t.Fatalf("Expected 1 note, got %d", len(journal.LogEntries[0].Notes))
	}

	note := journal.LogEntries[0].Notes[0]
	if note.Text != "First note" {
		t.Errorf("Expected note text 'First note', got '%s'", note.Text)
	}
	if note.Position != 0 {
		t.Errorf("Expected note position 0, got %d", note.Position)
	}
	if note.ID == "" {
		t.Error("Expected note to have generated ID")
	}

	// Verify journal was saved and can be retrieved
	retrieved, err := service.GetByDate("2024-01-15")
	if err != nil {
		t.Fatalf("GetByDate failed: %v", err)
	}

	if len(retrieved.LogEntries) != 1 {
		t.Fatalf("Expected 1 log entry, got %d", len(retrieved.LogEntries))
	}

	if len(retrieved.LogEntries[0].Notes) != 1 {
		t.Fatalf("Expected 1 note in retrieved journal, got %d", len(retrieved.LogEntries[0].Notes))
	}

	if retrieved.LogEntries[0].Notes[0].Text != "First note" {
		t.Errorf("Retrieved note text mismatch: expected 'First note', got '%s'", retrieved.LogEntries[0].Notes[0].Text)
	}
}

// TestAddLogNote_MultipleNotes tests adding multiple notes to a log entry
func TestAddLogNote_MultipleNotes(t *testing.T) {
	service, tempDir := setupLogNotesTestService(t)
	defer cleanupLogNotesTestService(t, tempDir)

	journal := NewJournal("2024-01-15")
	entry := NewLogEntry(time.Now(), "Test entry", EntryTypeLog, 0)
	journal.LogEntries = append(journal.LogEntries, *entry)

	// Add multiple notes
	noteTexts := []string{"First note", "Second note", "Third note"}
	for _, text := range noteTexts {
		err := service.AddLogNote(journal, entry.ID, text)
		if err != nil {
			t.Fatalf("AddLogNote failed: %v", err)
		}
	}

	// Verify all notes were added
	if len(journal.LogEntries[0].Notes) != 3 {
		t.Fatalf("Expected 3 notes, got %d", len(journal.LogEntries[0].Notes))
	}

	// Verify note content and positions
	for i, expectedText := range noteTexts {
		note := journal.LogEntries[0].Notes[i]
		if note.Text != expectedText {
			t.Errorf("Note %d: expected text '%s', got '%s'", i, expectedText, note.Text)
		}
		if note.Position != i {
			t.Errorf("Note %d: expected position %d, got %d", i, i, note.Position)
		}
	}
}

// TestAddLogNote_InvalidLogEntryID tests adding a note with invalid log entry ID
func TestAddLogNote_InvalidLogEntryID(t *testing.T) {
	service, tempDir := setupLogNotesTestService(t)
	defer cleanupLogNotesTestService(t, tempDir)

	journal := NewJournal("2024-01-15")

	err := service.AddLogNote(journal, "nonexistent-id", "Test note")
	if err == nil {
		t.Fatal("Expected error when adding note to nonexistent log entry")
	}

	expectedError := "log entry not found: nonexistent-id"
	if err.Error() != expectedError {
		t.Errorf("Expected error '%s', got '%s'", expectedError, err.Error())
	}
}

// TestUpdateLogNote_Success tests updating a note
func TestUpdateLogNote_Success(t *testing.T) {
	service, tempDir := setupLogNotesTestService(t)
	defer cleanupLogNotesTestService(t, tempDir)

	journal := NewJournal("2024-01-15")
	entry := NewLogEntry(time.Now(), "Test entry", EntryTypeLog, 0)
	journal.LogEntries = append(journal.LogEntries, *entry)

	// Add a note
	err := service.AddLogNote(journal, entry.ID, "Original text")
	if err != nil {
		t.Fatalf("AddLogNote failed: %v", err)
	}

	noteID := journal.LogEntries[0].Notes[0].ID

	// Update the note
	err = service.UpdateLogNote(journal, entry.ID, noteID, "Updated text")
	if err != nil {
		t.Fatalf("UpdateLogNote failed: %v", err)
	}

	// Verify note was updated
	if len(journal.LogEntries[0].Notes) != 1 {
		t.Fatalf("Expected 1 note, got %d", len(journal.LogEntries[0].Notes))
	}

	note := journal.LogEntries[0].Notes[0]
	if note.Text != "Updated text" {
		t.Errorf("Expected updated text 'Updated text', got '%s'", note.Text)
	}
	if note.ID != noteID {
		t.Errorf("Note ID should remain unchanged, expected '%s', got '%s'", noteID, note.ID)
	}

	// Verify update was persisted
	retrieved, err := service.GetByDate("2024-01-15")
	if err != nil {
		t.Fatalf("GetByDate failed: %v", err)
	}

	if retrieved.LogEntries[0].Notes[0].Text != "Updated text" {
		t.Errorf("Retrieved note text mismatch: expected 'Updated text', got '%s'", retrieved.LogEntries[0].Notes[0].Text)
	}
}

// TestUpdateLogNote_InvalidIDs tests updating a note with invalid IDs
func TestUpdateLogNote_InvalidIDs(t *testing.T) {
	service, tempDir := setupLogNotesTestService(t)
	defer cleanupLogNotesTestService(t, tempDir)

	journal := NewJournal("2024-01-15")
	entry := NewLogEntry(time.Now(), "Test entry", EntryTypeLog, 0)
	journal.LogEntries = append(journal.LogEntries, *entry)

	// Add a note
	err := service.AddLogNote(journal, entry.ID, "Test note")
	if err != nil {
		t.Fatalf("AddLogNote failed: %v", err)
	}

	noteID := journal.LogEntries[0].Notes[0].ID

	// Test invalid log entry ID
	err = service.UpdateLogNote(journal, "invalid-entry-id", noteID, "New text")
	if err == nil {
		t.Fatal("Expected error when updating note with invalid log entry ID")
	}

	// Test invalid note ID
	err = service.UpdateLogNote(journal, entry.ID, "invalid-note-id", "New text")
	if err == nil {
		t.Fatal("Expected error when updating note with invalid note ID")
	}
}

// TestDeleteLogNote_Success tests deleting a note
func TestDeleteLogNote_Success(t *testing.T) {
	service, tempDir := setupLogNotesTestService(t)
	defer cleanupLogNotesTestService(t, tempDir)

	journal := NewJournal("2024-01-15")
	entry := NewLogEntry(time.Now(), "Test entry", EntryTypeLog, 0)
	journal.LogEntries = append(journal.LogEntries, *entry)

	// Add multiple notes
	err := service.AddLogNote(journal, entry.ID, "First note")
	if err != nil {
		t.Fatalf("AddLogNote 1 failed: %v", err)
	}
	err = service.AddLogNote(journal, entry.ID, "Second note")
	if err != nil {
		t.Fatalf("AddLogNote 2 failed: %v", err)
	}
	err = service.AddLogNote(journal, entry.ID, "Third note")
	if err != nil {
		t.Fatalf("AddLogNote 3 failed: %v", err)
	}

	secondNoteID := journal.LogEntries[0].Notes[1].ID

	// Delete the second note
	err = service.DeleteLogNote(journal, entry.ID, secondNoteID)
	if err != nil {
		t.Fatalf("DeleteLogNote failed: %v", err)
	}

	// Verify note was deleted and positions were re-indexed
	notes := journal.LogEntries[0].Notes
	if len(notes) != 2 {
		t.Fatalf("Expected 2 notes after deletion, got %d", len(notes))
	}

	if notes[0].Text != "First note" {
		t.Errorf("Note 0: expected 'First note', got '%s'", notes[0].Text)
	}
	if notes[0].Position != 0 {
		t.Errorf("Note 0: expected position 0, got %d", notes[0].Position)
	}

	if notes[1].Text != "Third note" {
		t.Errorf("Note 1: expected 'Third note', got '%s'", notes[1].Text)
	}
	if notes[1].Position != 1 {
		t.Errorf("Note 1: expected position 1, got %d", notes[1].Position)
	}

	// Verify deletion was persisted
	retrieved, err := service.GetByDate("2024-01-15")
	if err != nil {
		t.Fatalf("GetByDate failed: %v", err)
	}

	if len(retrieved.LogEntries[0].Notes) != 2 {
		t.Fatalf("Expected 2 notes in retrieved journal, got %d", len(retrieved.LogEntries[0].Notes))
	}
}

// TestDeleteLogNote_LastNote tests deleting the only note
func TestDeleteLogNote_LastNote(t *testing.T) {
	service, tempDir := setupLogNotesTestService(t)
	defer cleanupLogNotesTestService(t, tempDir)

	journal := NewJournal("2024-01-15")
	entry := NewLogEntry(time.Now(), "Test entry", EntryTypeLog, 0)
	journal.LogEntries = append(journal.LogEntries, *entry)

	// Add one note
	err := service.AddLogNote(journal, entry.ID, "Only note")
	if err != nil {
		t.Fatalf("AddLogNote failed: %v", err)
	}

	noteID := journal.LogEntries[0].Notes[0].ID

	// Delete the note
	err = service.DeleteLogNote(journal, entry.ID, noteID)
	if err != nil {
		t.Fatalf("DeleteLogNote failed: %v", err)
	}

	// Verify no notes remain
	if len(journal.LogEntries[0].Notes) != 0 {
		t.Errorf("Expected 0 notes after deletion, got %d", len(journal.LogEntries[0].Notes))
	}

	// Verify deletion was persisted
	retrieved, err := service.GetByDate("2024-01-15")
	if err != nil {
		t.Fatalf("GetByDate failed: %v", err)
	}

	if len(retrieved.LogEntries[0].Notes) != 0 {
		t.Errorf("Expected 0 notes in retrieved journal, got %d", len(retrieved.LogEntries[0].Notes))
	}
}

// TestDeleteLogNote_InvalidIDs tests deleting a note with invalid IDs
func TestDeleteLogNote_InvalidIDs(t *testing.T) {
	service, tempDir := setupLogNotesTestService(t)
	defer cleanupLogNotesTestService(t, tempDir)

	journal := NewJournal("2024-01-15")
	entry := NewLogEntry(time.Now(), "Test entry", EntryTypeLog, 0)
	journal.LogEntries = append(journal.LogEntries, *entry)

	// Add a note
	err := service.AddLogNote(journal, entry.ID, "Test note")
	if err != nil {
		t.Fatalf("AddLogNote failed: %v", err)
	}

	noteID := journal.LogEntries[0].Notes[0].ID

	// Test invalid log entry ID
	err = service.DeleteLogNote(journal, "invalid-entry-id", noteID)
	if err == nil {
		t.Fatal("Expected error when deleting note with invalid log entry ID")
	}

	// Test invalid note ID
	err = service.DeleteLogNote(journal, entry.ID, "invalid-note-id")
	if err == nil {
		t.Fatal("Expected error when deleting note with invalid note ID")
	}

	// Verify original note still exists
	if len(journal.LogEntries[0].Notes) != 1 {
		t.Errorf("Expected 1 note to remain, got %d", len(journal.LogEntries[0].Notes))
	}
}

// TestLogNotes_Integration tests complete workflow of add/update/delete
func TestLogNotes_Integration(t *testing.T) {
	service, tempDir := setupLogNotesTestService(t)
	defer cleanupLogNotesTestService(t, tempDir)

	journal := NewJournal("2024-01-15")
	entry := NewLogEntry(time.Now(), "Integration test entry", EntryTypeLog, 0)
	journal.LogEntries = append(journal.LogEntries, *entry)

	// Add three notes
	for i := 1; i <= 3; i++ {
		err := service.AddLogNote(journal, entry.ID, "Note "+string(rune('0'+i)))
		if err != nil {
			t.Fatalf("AddLogNote %d failed: %v", i, err)
		}
	}

	if len(journal.LogEntries[0].Notes) != 3 {
		t.Fatalf("Expected 3 notes, got %d", len(journal.LogEntries[0].Notes))
	}

	// Update second note
	secondNoteID := journal.LogEntries[0].Notes[1].ID
	err := service.UpdateLogNote(journal, entry.ID, secondNoteID, "Updated note 2")
	if err != nil {
		t.Fatalf("UpdateLogNote failed: %v", err)
	}

	if journal.LogEntries[0].Notes[1].Text != "Updated note 2" {
		t.Errorf("Expected updated text, got '%s'", journal.LogEntries[0].Notes[1].Text)
	}

	// Delete first note
	firstNoteID := journal.LogEntries[0].Notes[0].ID
	err = service.DeleteLogNote(journal, entry.ID, firstNoteID)
	if err != nil {
		t.Fatalf("DeleteLogNote failed: %v", err)
	}

	// Verify final state
	notes := journal.LogEntries[0].Notes
	if len(notes) != 2 {
		t.Fatalf("Expected 2 notes, got %d", len(notes))
	}

	if notes[0].Text != "Updated note 2" {
		t.Errorf("Note 0: expected 'Updated note 2', got '%s'", notes[0].Text)
	}
	if notes[1].Text != "Note 3" {
		t.Errorf("Note 1: expected 'Note 3', got '%s'", notes[1].Text)
	}

	// Verify positions are correct
	if notes[0].Position != 0 || notes[1].Position != 1 {
		t.Error("Note positions not properly re-indexed")
	}

	// Verify persistence
	retrieved, err := service.GetByDate("2024-01-15")
	if err != nil {
		t.Fatalf("GetByDate failed: %v", err)
	}

	retrievedNotes := retrieved.LogEntries[0].Notes
	if len(retrievedNotes) != 2 {
		t.Fatalf("Expected 2 notes in retrieved journal, got %d", len(retrievedNotes))
	}

	if retrievedNotes[0].Text != "Updated note 2" || retrievedNotes[1].Text != "Note 3" {
		t.Error("Retrieved notes do not match expected state")
	}
}

// TestLogNotes_RoundTripWithMarkdown tests that notes survive markdown round-trip
func TestLogNotes_RoundTripWithMarkdown(t *testing.T) {
	service, tempDir := setupLogNotesTestService(t)
	defer cleanupLogNotesTestService(t, tempDir)

	// Create journal with notes
	journal := NewJournal("2024-01-15")
	entry := NewLogEntry(time.Now(), "Test entry", EntryTypeLog, 0)
	journal.LogEntries = append(journal.LogEntries, *entry)

	err := service.AddLogNote(journal, entry.ID, "First note with special chars: [brackets] & \"quotes\"")
	if err != nil {
		t.Fatalf("AddLogNote 1 failed: %v", err)
	}

	err = service.AddLogNote(journal, entry.ID, "Second note with newline content")
	if err != nil {
		t.Fatalf("AddLogNote 2 failed: %v", err)
	}

	// Retrieve the journal again to test round-trip
	retrieved, err := service.GetByDate("2024-01-15")
	if err != nil {
		t.Fatalf("Failed to retrieve journal: %v", err)
	}

	// Verify notes are preserved
	if len(retrieved.LogEntries) != 1 {
		t.Fatalf("Expected 1 log entry, got %d", len(retrieved.LogEntries))
	}

	if len(retrieved.LogEntries[0].Notes) != 2 {
		t.Fatalf("Expected 2 notes, got %d", len(retrieved.LogEntries[0].Notes))
	}

	expectedTexts := []string{
		"First note with special chars: [brackets] & \"quotes\"",
		"Second note with newline content",
	}

	for i, expectedText := range expectedTexts {
		if retrieved.LogEntries[0].Notes[i].Text != expectedText {
			t.Errorf("Note %d: expected '%s', got '%s'", i, expectedText, retrieved.LogEntries[0].Notes[i].Text)
		}
	}
}
