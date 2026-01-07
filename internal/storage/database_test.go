package storage

import (
	"testing"
)

func TestNewDatabase_SchemaInitialization(t *testing.T) {
	db, err := NewDatabase(":memory:")
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	// Verify log_notes table exists
	var tableName string
	err = db.DB().QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='log_notes'").Scan(&tableName)
	if err != nil {
		t.Fatalf("log_notes table does not exist: %v", err)
	}
	if tableName != "log_notes" {
		t.Errorf("expected table name 'log_notes', got %s", tableName)
	}
}

func TestLogNotes_ForeignKeyConstraint(t *testing.T) {
	db, err := NewDatabase(":memory:")
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	// Try to insert a log note with non-existent log_entry_id
	_, err = db.DB().Exec(`
		INSERT INTO log_notes (id, log_entry_id, text, position)
		VALUES ('note-1', 'nonexistent-entry', 'Test note', 0)
	`)

	if err == nil {
		t.Fatal("expected foreign key constraint violation, got nil error")
	}
}

func TestLogNotes_CascadeDelete(t *testing.T) {
	db, err := NewDatabase(":memory:")
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	// First create a journal
	_, err = db.DB().Exec(`
		INSERT INTO journals (date, file_path, last_modified)
		VALUES ('2024-01-01', '/test/path', 1234567890)
	`)
	if err != nil {
		t.Fatalf("failed to create journal: %v", err)
	}

	// Create a log entry
	_, err = db.DB().Exec(`
		INSERT INTO log_entries (id, journal_date, timestamp, content, entry_type, position)
		VALUES ('entry-1', '2024-01-01', '2024-01-01T10:00:00Z', 'Test entry', 'log', 0)
	`)
	if err != nil {
		t.Fatalf("failed to create log entry: %v", err)
	}

	// Create log notes associated with the entry
	_, err = db.DB().Exec(`
		INSERT INTO log_notes (id, log_entry_id, text, position)
		VALUES ('note-1', 'entry-1', 'First note', 0),
		       ('note-2', 'entry-1', 'Second note', 1)
	`)
	if err != nil {
		t.Fatalf("failed to create log notes: %v", err)
	}

	// Verify notes were created
	var noteCount int
	err = db.DB().QueryRow("SELECT COUNT(*) FROM log_notes WHERE log_entry_id = ?", "entry-1").Scan(&noteCount)
	if err != nil {
		t.Fatalf("failed to count notes: %v", err)
	}
	if noteCount != 2 {
		t.Errorf("expected 2 notes, got %d", noteCount)
	}

	// Delete the log entry
	_, err = db.DB().Exec("DELETE FROM log_entries WHERE id = ?", "entry-1")
	if err != nil {
		t.Fatalf("failed to delete log entry: %v", err)
	}

	// Verify notes were cascade deleted
	err = db.DB().QueryRow("SELECT COUNT(*) FROM log_notes WHERE log_entry_id = ?", "entry-1").Scan(&noteCount)
	if err != nil {
		t.Fatalf("failed to count notes after cascade delete: %v", err)
	}
	if noteCount != 0 {
		t.Errorf("expected 0 notes after cascade delete, got %d", noteCount)
	}
}

func TestLogNotes_Index(t *testing.T) {
	db, err := NewDatabase(":memory:")
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	// Verify idx_log_notes_entry index exists
	var indexName string
	err = db.DB().QueryRow(`
		SELECT name FROM sqlite_master
		WHERE type='index' AND name='idx_log_notes_entry'
	`).Scan(&indexName)
	if err != nil {
		t.Fatalf("idx_log_notes_entry index does not exist: %v", err)
	}
	if indexName != "idx_log_notes_entry" {
		t.Errorf("expected index name 'idx_log_notes_entry', got %s", indexName)
	}
}

func TestLogNotes_TableStructure(t *testing.T) {
	db, err := NewDatabase(":memory:")
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	// Query table structure
	rows, err := db.DB().Query("PRAGMA table_info(log_notes)")
	if err != nil {
		t.Fatalf("failed to query table info: %v", err)
	}
	defer rows.Close()

	columns := make(map[string]string)
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dfltValue interface{}

		err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk)
		if err != nil {
			t.Fatalf("failed to scan column info: %v", err)
		}
		columns[name] = colType
	}

	// Verify expected columns exist with correct types
	expectedColumns := map[string]string{
		"id":           "TEXT",
		"log_entry_id": "TEXT",
		"text":         "TEXT",
		"position":     "INTEGER",
	}

	for colName, expectedType := range expectedColumns {
		actualType, exists := columns[colName]
		if !exists {
			t.Errorf("expected column %s to exist", colName)
		} else if actualType != expectedType {
			t.Errorf("expected column %s to have type %s, got %s", colName, expectedType, actualType)
		}
	}
}
