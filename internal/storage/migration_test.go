package storage

import (
	"database/sql"
	"os"
	"testing"
)

func TestDateColumnsMigration(t *testing.T) {
	// Create a temporary database
	tmpfile, err := os.CreateTemp("", "test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	// Initialize database (which runs migrations)
	db, err := NewDatabase(tmpfile.Name())
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	// Verify scheduled_date column exists
	rows, err := db.DB().Query("PRAGMA table_info(tasks)")
	if err != nil {
		t.Fatalf("failed to get table info: %v", err)
	}
	defer rows.Close()

	var foundScheduled, foundDeadline bool
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			t.Fatalf("failed to scan column info: %v", err)
		}
		if name == "scheduled_date" && ctype == "TEXT" {
			foundScheduled = true
		}
		if name == "deadline_date" && ctype == "TEXT" {
			foundDeadline = true
		}
	}

	if !foundScheduled {
		t.Error("scheduled_date column not found in tasks table")
	}
	if !foundDeadline {
		t.Error("deadline_date column not found in tasks table")
	}

	// Verify indices exist
	indexRows, err := db.DB().Query("SELECT name FROM sqlite_master WHERE type='index' AND tbl_name='tasks'")
	if err != nil {
		t.Fatalf("failed to query indices: %v", err)
	}
	defer indexRows.Close()

	var foundScheduledIndex, foundDeadlineIndex bool
	for indexRows.Next() {
		var name string
		if err := indexRows.Scan(&name); err != nil {
			t.Fatalf("failed to scan index name: %v", err)
		}
		if name == "idx_tasks_scheduled" {
			foundScheduledIndex = true
		}
		if name == "idx_tasks_deadline" {
			foundDeadlineIndex = true
		}
	}

	if !foundScheduledIndex {
		t.Error("idx_tasks_scheduled index not found")
	}
	if !foundDeadlineIndex {
		t.Error("idx_tasks_deadline index not found")
	}

	// Test inserting a task with dates in ISO 8601 format
	_, err = db.DB().Exec(`
		INSERT INTO tasks (id, text, status, tags, position, created_at, scheduled_date, deadline_date)
		VALUES ('test-id', 'test task', 'open', NULL, 1, 1234567890, '2026-01-15', '2026-01-31')
	`)
	if err != nil {
		t.Fatalf("failed to insert task with dates: %v", err)
	}

	// Verify the dates were stored correctly
	var scheduledDate, deadlineDate sql.NullString
	err = db.DB().QueryRow("SELECT scheduled_date, deadline_date FROM tasks WHERE id = 'test-id'").Scan(&scheduledDate, &deadlineDate)
	if err != nil {
		t.Fatalf("failed to query task: %v", err)
	}

	if !scheduledDate.Valid || scheduledDate.String != "2026-01-15" {
		t.Errorf("expected scheduled_date to be '2026-01-15', got %v", scheduledDate)
	}
	if !deadlineDate.Valid || deadlineDate.String != "2026-01-31" {
		t.Errorf("expected deadline_date to be '2026-01-31', got %v", deadlineDate)
	}

	// Test that NULL values work for existing tasks
	_, err = db.DB().Exec(`
		INSERT INTO tasks (id, text, status, tags, position, created_at, scheduled_date, deadline_date)
		VALUES ('test-id-2', 'task without dates', 'open', NULL, 2, 1234567890, NULL, NULL)
	`)
	if err != nil {
		t.Fatalf("failed to insert task with NULL dates: %v", err)
	}

	var scheduledDate2, deadlineDate2 sql.NullString
	err = db.DB().QueryRow("SELECT scheduled_date, deadline_date FROM tasks WHERE id = 'test-id-2'").Scan(&scheduledDate2, &deadlineDate2)
	if err != nil {
		t.Fatalf("failed to query task: %v", err)
	}

	if scheduledDate2.Valid {
		t.Errorf("expected scheduled_date to be NULL, got %v", scheduledDate2)
	}
	if deadlineDate2.Valid {
		t.Errorf("expected deadline_date to be NULL, got %v", deadlineDate2)
	}
}

func TestDateColumnsMigration_ExistingDatabase(t *testing.T) {
	// Create a temporary database
	tmpfile, err := os.CreateTemp("", "test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	// Create a database with old schema (without date columns)
	rawDB, err := sql.Open("sqlite", tmpfile.Name())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	// Create tasks table without date columns
	_, err = rawDB.Exec(`
		CREATE TABLE tasks (
			id TEXT PRIMARY KEY,
			text TEXT NOT NULL,
			status TEXT NOT NULL,
			tags TEXT,
			position INTEGER NOT NULL,
			created_at INTEGER NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("failed to create old schema: %v", err)
	}

	// Insert a task
	_, err = rawDB.Exec(`
		INSERT INTO tasks (id, text, status, tags, position, created_at)
		VALUES ('old-task', 'existing task', 'open', NULL, 1, 1234567890)
	`)
	if err != nil {
		t.Fatalf("failed to insert task: %v", err)
	}
	rawDB.Close()

	// Now open with NewDatabase which should run migrations
	db, err := NewDatabase(tmpfile.Name())
	if err != nil {
		t.Fatalf("failed to open database with migrations: %v", err)
	}
	defer db.Close()

	// Verify columns were added
	rows, err := db.DB().Query("PRAGMA table_info(tasks)")
	if err != nil {
		t.Fatalf("failed to get table info: %v", err)
	}
	defer rows.Close()

	var foundScheduled, foundDeadline bool
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			t.Fatalf("failed to scan column info: %v", err)
		}
		if name == "scheduled_date" {
			foundScheduled = true
		}
		if name == "deadline_date" {
			foundDeadline = true
		}
	}

	if !foundScheduled {
		t.Error("scheduled_date column not added during migration")
	}
	if !foundDeadline {
		t.Error("deadline_date column not added during migration")
	}

	// Verify existing task has NULL for new columns
	var scheduledDate, deadlineDate sql.NullString
	err = db.DB().QueryRow("SELECT scheduled_date, deadline_date FROM tasks WHERE id = 'old-task'").Scan(&scheduledDate, &deadlineDate)
	if err != nil {
		t.Fatalf("failed to query old task: %v", err)
	}

	if scheduledDate.Valid {
		t.Errorf("expected existing task scheduled_date to be NULL, got %v", scheduledDate)
	}
	if deadlineDate.Valid {
		t.Errorf("expected existing task deadline_date to be NULL, got %v", deadlineDate)
	}
}
