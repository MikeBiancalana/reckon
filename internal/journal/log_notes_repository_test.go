package journal

import (
	"testing"
	"time"
)

// TestSaveLogNotes_Success tests saving notes for a log entry
func TestSaveLogNotes_Success(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewRepository(db, nil)

	// Create a journal with a log entry
	journal := &Journal{
		Date:         "2024-01-15",
		FilePath:     "/test/2024-01-15.md",
		LastModified: time.Now(),
		LogEntries: []LogEntry{
			{
				ID:        "entry-1",
				Timestamp: time.Now(),
				Content:   "Test entry",
				EntryType: EntryTypeLog,
				Position:  0,
				Notes: []LogNote{
					{ID: "note-1", Text: "First note", Position: 0},
					{ID: "note-2", Text: "Second note", Position: 1},
					{ID: "note-3", Text: "Third note", Position: 2},
				},
			},
		},
	}

	err := repo.SaveJournal(journal)
	if err != nil {
		t.Fatalf("SaveJournal failed: %v", err)
	}

	// Retrieve notes
	notes, err := repo.GetLogNotes("entry-1")
	if err != nil {
		t.Fatalf("GetLogNotes failed: %v", err)
	}

	if len(notes) != 3 {
		t.Fatalf("Expected 3 notes, got %d", len(notes))
	}

	// Verify note content
	expectedNotes := []struct {
		ID   string
		Text string
		Pos  int
	}{
		{"note-1", "First note", 0},
		{"note-2", "Second note", 1},
		{"note-3", "Third note", 2},
	}

	for i, expected := range expectedNotes {
		if notes[i].ID != expected.ID {
			t.Errorf("Note %d: expected ID %s, got %s", i, expected.ID, notes[i].ID)
		}
		if notes[i].Text != expected.Text {
			t.Errorf("Note %d: expected text %s, got %s", i, expected.Text, notes[i].Text)
		}
		if notes[i].Position != expected.Pos {
			t.Errorf("Note %d: expected position %d, got %d", i, expected.Pos, notes[i].Position)
		}
	}
}

// TestSaveLogNotes_EmptyNotes tests saving a log entry with no notes
func TestSaveLogNotes_EmptyNotes(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewRepository(db, nil)

	journal := &Journal{
		Date:         "2024-01-15",
		FilePath:     "/test/2024-01-15.md",
		LastModified: time.Now(),
		LogEntries: []LogEntry{
			{
				ID:        "entry-1",
				Timestamp: time.Now(),
				Content:   "Test entry",
				EntryType: EntryTypeLog,
				Position:  0,
				Notes:     []LogNote{},
			},
		},
	}

	err := repo.SaveJournal(journal)
	if err != nil {
		t.Fatalf("SaveJournal failed: %v", err)
	}

	notes, err := repo.GetLogNotes("entry-1")
	if err != nil {
		t.Fatalf("GetLogNotes failed: %v", err)
	}

	if len(notes) != 0 {
		t.Errorf("Expected 0 notes, got %d", len(notes))
	}
}

// TestSaveLogNotes_Update tests updating notes for an existing log entry
func TestSaveLogNotes_Update(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewRepository(db, nil)

	// Create initial journal with notes
	journal := &Journal{
		Date:         "2024-01-15",
		FilePath:     "/test/2024-01-15.md",
		LastModified: time.Now(),
		LogEntries: []LogEntry{
			{
				ID:        "entry-1",
				Timestamp: time.Now(),
				Content:   "Test entry",
				EntryType: EntryTypeLog,
				Position:  0,
				Notes: []LogNote{
					{ID: "note-1", Text: "First note", Position: 0},
					{ID: "note-2", Text: "Second note", Position: 1},
				},
			},
		},
	}

	err := repo.SaveJournal(journal)
	if err != nil {
		t.Fatalf("Initial SaveJournal failed: %v", err)
	}

	// Update with different notes
	journal.LogEntries[0].Notes = []LogNote{
		{ID: "note-3", Text: "New note", Position: 0},
	}

	err = repo.SaveJournal(journal)
	if err != nil {
		t.Fatalf("Update SaveJournal failed: %v", err)
	}

	// Verify only new note exists
	notes, err := repo.GetLogNotes("entry-1")
	if err != nil {
		t.Fatalf("GetLogNotes failed: %v", err)
	}

	if len(notes) != 1 {
		t.Fatalf("Expected 1 note, got %d", len(notes))
	}

	if notes[0].ID != "note-3" {
		t.Errorf("Expected note ID note-3, got %s", notes[0].ID)
	}
	if notes[0].Text != "New note" {
		t.Errorf("Expected text 'New note', got %s", notes[0].Text)
	}
}

// TestGetLogNotes_NonexistentEntry tests getting notes for a nonexistent entry
func TestGetLogNotes_NonexistentEntry(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewRepository(db, nil)

	notes, err := repo.GetLogNotes("nonexistent-entry")
	if err != nil {
		t.Fatalf("GetLogNotes failed: %v", err)
	}

	if len(notes) != 0 {
		t.Errorf("Expected 0 notes for nonexistent entry, got %d", len(notes))
	}
}

// TestGetJournalByDate_LoadsNotes tests that GetJournalByDate loads notes correctly
func TestGetJournalByDate_LoadsNotes(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewRepository(db, nil)

	// Create a journal with multiple log entries, each with notes
	journal := &Journal{
		Date:         "2024-01-15",
		FilePath:     "/test/2024-01-15.md",
		LastModified: time.Now(),
		LogEntries: []LogEntry{
			{
				ID:        "entry-1",
				Timestamp: time.Now(),
				Content:   "First entry",
				EntryType: EntryTypeLog,
				Position:  0,
				Notes: []LogNote{
					{ID: "note-1", Text: "First note", Position: 0},
					{ID: "note-2", Text: "Second note", Position: 1},
				},
			},
			{
				ID:        "entry-2",
				Timestamp: time.Now().Add(1 * time.Hour),
				Content:   "Second entry",
				EntryType: EntryTypeLog,
				Position:  1,
				Notes: []LogNote{
					{ID: "note-3", Text: "Third note", Position: 0},
				},
			},
			{
				ID:        "entry-3",
				Timestamp: time.Now().Add(2 * time.Hour),
				Content:   "Third entry",
				EntryType: EntryTypeLog,
				Position:  2,
				Notes:     []LogNote{}, // No notes
			},
		},
	}

	err := repo.SaveJournal(journal)
	if err != nil {
		t.Fatalf("SaveJournal failed: %v", err)
	}

	// Retrieve journal
	retrieved, err := repo.GetJournalByDate("2024-01-15")
	if err != nil {
		t.Fatalf("GetJournalByDate failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Expected journal to be retrieved")
	}

	if len(retrieved.LogEntries) != 3 {
		t.Fatalf("Expected 3 log entries, got %d", len(retrieved.LogEntries))
	}

	// Verify first entry has 2 notes
	if len(retrieved.LogEntries[0].Notes) != 2 {
		t.Errorf("Entry 0: expected 2 notes, got %d", len(retrieved.LogEntries[0].Notes))
	}

	// Verify second entry has 1 note
	if len(retrieved.LogEntries[1].Notes) != 1 {
		t.Errorf("Entry 1: expected 1 note, got %d", len(retrieved.LogEntries[1].Notes))
	}

	// Verify third entry has 0 notes
	if len(retrieved.LogEntries[2].Notes) != 0 {
		t.Errorf("Entry 2: expected 0 notes, got %d", len(retrieved.LogEntries[2].Notes))
	}

	// Verify note content
	if retrieved.LogEntries[0].Notes[0].Text != "First note" {
		t.Errorf("Entry 0, Note 0: expected 'First note', got '%s'", retrieved.LogEntries[0].Notes[0].Text)
	}
	if retrieved.LogEntries[0].Notes[1].Text != "Second note" {
		t.Errorf("Entry 0, Note 1: expected 'Second note', got '%s'", retrieved.LogEntries[0].Notes[1].Text)
	}
	if retrieved.LogEntries[1].Notes[0].Text != "Third note" {
		t.Errorf("Entry 1, Note 0: expected 'Third note', got '%s'", retrieved.LogEntries[1].Notes[0].Text)
	}
}

// TestDeleteJournal_CascadeDeletesNotes tests that deleting a journal deletes its notes
func TestDeleteJournal_CascadeDeletesNotes(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewRepository(db, nil)

	// Create a journal with notes
	journal := &Journal{
		Date:         "2024-01-15",
		FilePath:     "/test/2024-01-15.md",
		LastModified: time.Now(),
		LogEntries: []LogEntry{
			{
				ID:        "entry-1",
				Timestamp: time.Now(),
				Content:   "Test entry",
				EntryType: EntryTypeLog,
				Position:  0,
				Notes: []LogNote{
					{ID: "note-1", Text: "First note", Position: 0},
					{ID: "note-2", Text: "Second note", Position: 1},
				},
			},
		},
	}

	err := repo.SaveJournal(journal)
	if err != nil {
		t.Fatalf("SaveJournal failed: %v", err)
	}

	// Verify notes exist
	notes, err := repo.GetLogNotes("entry-1")
	if err != nil {
		t.Fatalf("GetLogNotes failed: %v", err)
	}
	if len(notes) != 2 {
		t.Fatalf("Expected 2 notes before deletion, got %d", len(notes))
	}

	// Delete journal
	err = repo.DeleteJournal("2024-01-15")
	if err != nil {
		t.Fatalf("DeleteJournal failed: %v", err)
	}

	// Verify notes are deleted
	notes, err = repo.GetLogNotes("entry-1")
	if err != nil {
		t.Fatalf("GetLogNotes failed after deletion: %v", err)
	}
	if len(notes) != 0 {
		t.Errorf("Expected 0 notes after journal deletion, got %d", len(notes))
	}
}

// TestClearAllData_DeletesNotes tests that ClearAllData deletes log notes
func TestClearAllData_DeletesNotes(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewRepository(db, nil)

	// Create journals with notes
	journal1 := &Journal{
		Date:         "2024-01-15",
		FilePath:     "/test/2024-01-15.md",
		LastModified: time.Now(),
		LogEntries: []LogEntry{
			{
				ID:        "entry-1",
				Timestamp: time.Now(),
				Content:   "Test entry",
				EntryType: EntryTypeLog,
				Position:  0,
				Notes: []LogNote{
					{ID: "note-1", Text: "First note", Position: 0},
				},
			},
		},
	}

	journal2 := &Journal{
		Date:         "2024-01-16",
		FilePath:     "/test/2024-01-16.md",
		LastModified: time.Now(),
		LogEntries: []LogEntry{
			{
				ID:        "entry-2",
				Timestamp: time.Now(),
				Content:   "Another entry",
				EntryType: EntryTypeLog,
				Position:  0,
				Notes: []LogNote{
					{ID: "note-2", Text: "Second note", Position: 0},
				},
			},
		},
	}

	err := repo.SaveJournal(journal1)
	if err != nil {
		t.Fatalf("SaveJournal 1 failed: %v", err)
	}

	err = repo.SaveJournal(journal2)
	if err != nil {
		t.Fatalf("SaveJournal 2 failed: %v", err)
	}

	// Clear all data
	err = repo.ClearAllData()
	if err != nil {
		t.Fatalf("ClearAllData failed: %v", err)
	}

	// Verify notes are deleted
	notes1, err := repo.GetLogNotes("entry-1")
	if err != nil {
		t.Fatalf("GetLogNotes 1 failed: %v", err)
	}
	if len(notes1) != 0 {
		t.Errorf("Expected 0 notes for entry-1, got %d", len(notes1))
	}

	notes2, err := repo.GetLogNotes("entry-2")
	if err != nil {
		t.Fatalf("GetLogNotes 2 failed: %v", err)
	}
	if len(notes2) != 0 {
		t.Errorf("Expected 0 notes for entry-2, got %d", len(notes2))
	}
}
