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
    tags TEXT,
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

-- Log notes table
CREATE TABLE IF NOT EXISTS log_notes (
    id TEXT PRIMARY KEY,
    log_entry_id TEXT NOT NULL,
    text TEXT NOT NULL,
    position INTEGER NOT NULL,
    FOREIGN KEY (log_entry_id) REFERENCES log_entries(id) ON DELETE CASCADE
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
CREATE INDEX IF NOT EXISTS idx_log_notes_entry ON log_notes(log_entry_id);
CREATE INDEX IF NOT EXISTS idx_schedule_items_date ON schedule_items(journal_date);
CREATE INDEX IF NOT EXISTS idx_schedule_items_time ON schedule_items(time);


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

	// Run migrations for existing databases
	if err := runMigrations(db); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return &Database{db: db}, nil
}

// runMigrations applies schema migrations for existing databases
func runMigrations(db *sql.DB) error {
	// Migration: Add tags column to tasks table if missing
	if err := addColumnIfMissing(db, "tasks", "tags", "TEXT"); err != nil {
		return err
	}

	// Migration: Fix tasks table PRIMARY KEY if missing
	if err := fixTasksPrimaryKey(db); err != nil {
		return err
	}

	return nil
}

// fixTasksPrimaryKey ensures the tasks table has a PRIMARY KEY on id column
func fixTasksPrimaryKey(db *sql.DB) error {
	// Check if id column is a primary key
	rows, err := db.Query("PRAGMA table_info(tasks)")
	if err != nil {
		return fmt.Errorf("failed to get tasks table info: %w", err)
	}
	defer rows.Close()

	hasPrimaryKey := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return fmt.Errorf("failed to scan column info: %w", err)
		}
		if name == "id" && pk == 1 {
			hasPrimaryKey = true
			break
		}
	}

	// If PRIMARY KEY is missing, recreate the table
	if !hasPrimaryKey {
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}
		defer tx.Rollback()

		// Create new table with proper schema
		_, err = tx.Exec(`
			CREATE TABLE tasks_new (
				id TEXT PRIMARY KEY,
				text TEXT NOT NULL,
				status TEXT NOT NULL,
				tags TEXT,
				position INTEGER NOT NULL,
				created_at INTEGER NOT NULL
			)
		`)
		if err != nil {
			return fmt.Errorf("failed to create tasks_new table: %w", err)
		}

		// Copy data from old table
		_, err = tx.Exec("INSERT INTO tasks_new SELECT id, text, status, tags, position, created_at FROM tasks")
		if err != nil {
			return fmt.Errorf("failed to copy data to tasks_new: %w", err)
		}

		// Drop old table
		_, err = tx.Exec("DROP TABLE tasks")
		if err != nil {
			return fmt.Errorf("failed to drop old tasks table: %w", err)
		}

		// Rename new table
		_, err = tx.Exec("ALTER TABLE tasks_new RENAME TO tasks")
		if err != nil {
			return fmt.Errorf("failed to rename tasks_new: %w", err)
		}

		// Recreate indices
		_, err = tx.Exec("CREATE INDEX idx_tasks_status ON tasks(status)")
		if err != nil {
			return fmt.Errorf("failed to create status index: %w", err)
		}

		_, err = tx.Exec("CREATE INDEX idx_tasks_position ON tasks(position)")
		if err != nil {
			return fmt.Errorf("failed to create position index: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit transaction: %w", err)
		}
	}

	return nil
}

// addColumnIfMissing adds a column to a table if it doesn't exist
func addColumnIfMissing(db *sql.DB, table, column, colType string) error {
	// Check if column exists using PRAGMA table_info
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return fmt.Errorf("failed to get table info for %s: %w", table, err)
	}
	defer rows.Close()

	columnExists := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return fmt.Errorf("failed to scan column info: %w", err)
		}
		if name == column {
			columnExists = true
			break
		}
	}

	if !columnExists {
		_, err := db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, colType))
		if err != nil {
			return fmt.Errorf("failed to add column %s to %s: %w", column, table, err)
		}
	}

	return nil
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
