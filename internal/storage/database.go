package storage

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

const schema = `
-- Journals table
CREATE TABLE IF NOT EXISTS journals (
    date TEXT PRIMARY KEY,
    file_path TEXT NOT NULL,
    last_modified INTEGER NOT NULL
);

-- Intentions table
CREATE TABLE IF NOT EXISTS intentions (
    id TEXT PRIMARY KEY,
    journal_date TEXT NOT NULL,
    text TEXT NOT NULL,
    status TEXT NOT NULL,
    carried_from TEXT,
    position INTEGER NOT NULL,
    FOREIGN KEY (journal_date) REFERENCES journals(date) ON DELETE CASCADE
);

-- Log entries table
CREATE TABLE IF NOT EXISTS log_entries (
    id TEXT PRIMARY KEY,
    journal_date TEXT NOT NULL,
    timestamp TEXT NOT NULL,
    content TEXT NOT NULL,
    task_id TEXT,
    entry_type TEXT NOT NULL,
    duration_minutes INTEGER,
    position INTEGER NOT NULL,
    FOREIGN KEY (journal_date) REFERENCES journals(date) ON DELETE CASCADE
);

-- Wins table
CREATE TABLE IF NOT EXISTS wins (
    id TEXT PRIMARY KEY,
    journal_date TEXT NOT NULL,
    text TEXT NOT NULL,
    position INTEGER NOT NULL,
    FOREIGN KEY (journal_date) REFERENCES journals(date) ON DELETE CASCADE
);

-- Tasks table (global tasks)
CREATE TABLE IF NOT EXISTS tasks (
    id TEXT PRIMARY KEY,
    text TEXT NOT NULL,
    status TEXT NOT NULL,
    position INTEGER NOT NULL,
    created_at INTEGER NOT NULL
);

-- Task notes table
CREATE TABLE IF NOT EXISTS task_notes (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL,
    text TEXT NOT NULL,
    position INTEGER NOT NULL,
    FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
);

-- Schedule items table (per-journal)
CREATE TABLE IF NOT EXISTS schedule_items (
    id TEXT PRIMARY KEY,
    journal_date TEXT NOT NULL,
    time TEXT NOT NULL,
    content TEXT NOT NULL,
    position INTEGER NOT NULL,
    FOREIGN KEY (journal_date) REFERENCES journals(date) ON DELETE CASCADE
);

-- Phase 2: Task Management tables
-- Multi-day tasks (separate from journal tasks)
CREATE TABLE IF NOT EXISTS phase2_tasks (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    status TEXT NOT NULL,
    created TEXT NOT NULL,
    file_path TEXT NOT NULL
);

-- Task tags
CREATE TABLE IF NOT EXISTS phase2_task_tags (
    task_id TEXT NOT NULL,
    tag TEXT NOT NULL,
    PRIMARY KEY (task_id, tag),
    FOREIGN KEY (task_id) REFERENCES phase2_tasks(id) ON DELETE CASCADE
);

-- Task log entries
CREATE TABLE IF NOT EXISTS phase2_task_log_entries (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL,
    date TEXT NOT NULL,
    timestamp TEXT NOT NULL,
    content TEXT NOT NULL,
    FOREIGN KEY (task_id) REFERENCES phase2_tasks(id) ON DELETE CASCADE
);

-- Indices for faster queries
CREATE INDEX IF NOT EXISTS idx_intentions_date ON intentions(journal_date);
CREATE INDEX IF NOT EXISTS idx_intentions_status ON intentions(status);
CREATE INDEX IF NOT EXISTS idx_log_entries_date ON log_entries(journal_date);
CREATE INDEX IF NOT EXISTS idx_log_entries_type ON log_entries(entry_type);
CREATE INDEX IF NOT EXISTS idx_log_entries_task ON log_entries(task_id);
CREATE INDEX IF NOT EXISTS idx_wins_date ON wins(journal_date);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_position ON tasks(position);
CREATE INDEX IF NOT EXISTS idx_task_notes_task ON task_notes(task_id);
CREATE INDEX IF NOT EXISTS idx_schedule_items_date ON schedule_items(journal_date);
CREATE INDEX IF NOT EXISTS idx_schedule_items_time ON schedule_items(time);

-- Phase 2 task indices
CREATE INDEX IF NOT EXISTS idx_phase2_tasks_status ON phase2_tasks(status);
CREATE INDEX IF NOT EXISTS idx_phase2_task_tags_tag ON phase2_task_tags(tag);
CREATE INDEX IF NOT EXISTS idx_phase2_task_log_date ON phase2_task_log_entries(date);
CREATE INDEX IF NOT EXISTS idx_phase2_task_log_task ON phase2_task_log_entries(task_id);
`

// Database wraps a SQL database connection
type Database struct {
	db *sql.DB
}

// NewDatabase creates a new database connection and initializes the schema
func NewDatabase(path string) (*Database, error) {
	// Open database with WAL mode for better concurrent access
	db, err := sql.Open("sqlite", path+"?_journal=WAL&_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Enable foreign keys (required for CASCADE to work)
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Initialize schema
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return &Database{db: db}, nil
}

// Close closes the database connection
func (d *Database) Close() error {
	return d.db.Close()
}

// DB returns the underlying database connection
func (d *Database) DB() *sql.DB {
	return d.db
}

// BeginTx starts a new transaction
func (d *Database) BeginTx() (*sql.Tx, error) {
	return d.db.Begin()
}
