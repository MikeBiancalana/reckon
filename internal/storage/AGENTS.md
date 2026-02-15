# Storage Subsystem Guide

## Overview

The storage layer provides database access via SQLite. The database is a **derived index** built from markdown files—it can always be rebuilt.

**Key files:**
- `database.go` - Database connection, initialization, transactions
- `database_test.go` - Database tests
- `notes.go` - Notes repository methods
- `notes_test.go` - Notes tests
- `filesystem.go` - File system operations (reading/writing markdown)
- `migration_test.go` - Migration tests
- `../db/migrations/*.sql` - SQL migration files

## Architecture Principle

**Files are source of truth, SQLite is a rebuildable index.**

```
Markdown Files (source of truth)
         ↓
    Parse & Index
         ↓
SQLite Database (fast queries, derived data)
```

This means:
- If database gets corrupted, run `rk rebuild`
- Files can be edited directly and synced
- Database schema can be changed and rebuilt from files

## Database Schema

### Core Tables

**journals** - Daily journal metadata
```sql
CREATE TABLE journals (
    date TEXT PRIMARY KEY,           -- YYYY-MM-DD
    file_path TEXT NOT NULL,         -- Relative path to .md file
    last_modified INTEGER            -- Unix timestamp
);
```

**intentions** - Daily goals
```sql
CREATE TABLE intentions (
    id TEXT PRIMARY KEY,             -- XID
    journal_date TEXT NOT NULL,      -- YYYY-MM-DD
    text TEXT NOT NULL,
    status TEXT NOT NULL,            -- 'open', 'done', 'carried'
    carried_from TEXT,               -- YYYY-MM-DD if carried
    position INTEGER NOT NULL,       -- Order in file
    FOREIGN KEY (journal_date) REFERENCES journals(date)
);
```

**log_entries** - Timestamped activity logs
```sql
CREATE TABLE log_entries (
    id TEXT PRIMARY KEY,             -- XID
    journal_date TEXT NOT NULL,      -- YYYY-MM-DD
    timestamp TEXT NOT NULL,         -- HH:MM or ISO timestamp
    content TEXT NOT NULL,
    task_id TEXT,                    -- Optional task reference
    entry_type TEXT NOT NULL,        -- 'log', 'meeting', 'break'
    duration_minutes INTEGER,        -- Optional duration
    position INTEGER NOT NULL,
    FOREIGN KEY (journal_date) REFERENCES journals(date)
);
```

**log_notes** - Notes attached to log entries
```sql
CREATE TABLE log_notes (
    id TEXT PRIMARY KEY,             -- XID
    log_entry_id TEXT NOT NULL,      -- Parent log entry
    text TEXT NOT NULL,
    position INTEGER NOT NULL,
    FOREIGN KEY (log_entry_id) REFERENCES log_entries(id)
);
```

**tasks** - Multi-day work items
```sql
CREATE TABLE tasks (
    id TEXT PRIMARY KEY,             -- XID
    text TEXT NOT NULL,
    status TEXT NOT NULL,            -- 'open', 'done'
    tags TEXT,                       -- JSON array of tags
    position INTEGER NOT NULL,
    created_at INTEGER NOT NULL,     -- Unix timestamp
    scheduled_date TEXT,             -- YYYY-MM-DD (optional)
    deadline_date TEXT               -- YYYY-MM-DD (optional)
);
```

**task_notes** - Notes attached to tasks
```sql
CREATE TABLE task_notes (
    id TEXT PRIMARY KEY,             -- XID
    task_id TEXT NOT NULL,
    text TEXT NOT NULL,
    position INTEGER NOT NULL,
    FOREIGN KEY (task_id) REFERENCES tasks(id)
);
```

**wins** - Daily accomplishments
```sql
CREATE TABLE wins (
    id TEXT PRIMARY KEY,             -- XID
    journal_date TEXT NOT NULL,
    text TEXT NOT NULL,
    position INTEGER NOT NULL,
    FOREIGN KEY (journal_date) REFERENCES journals(date)
);
```

**schedule_items** - Daily schedule
```sql
CREATE TABLE schedule_items (
    id TEXT PRIMARY KEY,             -- XID
    journal_date TEXT NOT NULL,
    time TEXT NOT NULL,              -- HH:MM
    description TEXT NOT NULL,
    duration_minutes INTEGER,        -- Optional
    position INTEGER NOT NULL,
    FOREIGN KEY (journal_date) REFERENCES journals(date)
);
```

**notes** - Zettelkasten notes
```sql
CREATE TABLE notes (
    id TEXT PRIMARY KEY,             -- XID
    title TEXT NOT NULL,
    slug TEXT UNIQUE NOT NULL,       -- URL-safe identifier
    file_path TEXT NOT NULL,         -- Relative path from notes dir
    tags TEXT,                       -- Comma-separated
    created_at INTEGER NOT NULL,     -- Unix timestamp
    updated_at INTEGER NOT NULL      -- Unix timestamp
);
```

**note_links** - Wiki-style links between notes
```sql
CREATE TABLE note_links (
    id TEXT PRIMARY KEY,             -- XID
    source_note_id TEXT NOT NULL,    -- Linking note
    target_slug TEXT NOT NULL,       -- Target note slug
    target_note_id TEXT,             -- Resolved note ID (nullable if orphaned)
    link_type TEXT NOT NULL,         -- 'reference', 'parent', 'child', 'related'
    created_at INTEGER NOT NULL,
    FOREIGN KEY (source_note_id) REFERENCES notes(id)
);
```

### Indexes

```sql
CREATE INDEX idx_intentions_journal ON intentions(journal_date);
CREATE INDEX idx_log_entries_journal ON log_entries(journal_date);
CREATE INDEX idx_log_entries_task ON log_entries(task_id);
CREATE INDEX idx_wins_journal ON wins(journal_date);
CREATE INDEX idx_schedule_journal ON schedule_items(journal_date);
CREATE INDEX idx_tasks_status ON tasks(status);
CREATE INDEX idx_notes_slug ON notes(slug);
CREATE INDEX idx_note_links_source ON note_links(source_note_id);
CREATE INDEX idx_note_links_target ON note_links(target_slug);
```

## Migrations

### Migration Files

Located in `/internal/db/migrations/`:
```
00001_initial_schema.sql
00002_add_task_dates.sql
00003_add_schedule_items.sql
00004_add_log_notes.sql
00005_add_task_notes.sql
00006_create_notes_tables.sql
```

**Naming convention:** `NNNNN_description.sql`
- `NNNNN` - Sequential number (00001, 00002, etc.)
- `description` - Brief description with underscores

### Migration Format

```sql
-- Migration: Add scheduled_date to tasks
-- Created: 2025-01-10

-- Up migration
ALTER TABLE tasks ADD COLUMN scheduled_date TEXT;
ALTER TABLE tasks ADD COLUMN deadline_date TEXT;

CREATE INDEX idx_tasks_schedule ON tasks(scheduled_date);
CREATE INDEX idx_tasks_deadline ON tasks(deadline_date);

-- Rollback not currently supported, but document here:
-- DROP INDEX idx_tasks_deadline;
-- DROP INDEX idx_tasks_schedule;
-- ALTER TABLE tasks DROP COLUMN deadline_date;
-- ALTER TABLE tasks DROP COLUMN scheduled_date;
```

### Running Migrations

Migrations run automatically on database initialization:
```go
db, err := storage.NewDatabase(dbPath)
// Migrations run here
```

**Manual migration** (if needed):
```bash
sqlite3 ~/.reckon/reckon.db < internal/db/migrations/00007_new_migration.sql
```

### Creating a New Migration

1. **Create file:** `/internal/db/migrations/NNNNN_description.sql`
2. **Write SQL:**
   ```sql
   -- Migration: Description
   -- Created: YYYY-MM-DD

   CREATE TABLE new_table (...);
   CREATE INDEX idx_new ON new_table(column);
   ```
3. **Update models** in `/internal/journal/models.go`
4. **Update repository** to use new schema
5. **Test migration** on sample database
6. **Add test** in `migration_test.go`

## Database Connection

### Initialization

```go
db, err := storage.NewDatabase(dbPath)
if err != nil {
    return fmt.Errorf("failed to open database: %w", err)
}
defer db.Close()
```

**Connection pool settings:**
```go
db.SetMaxOpenConns(1)  // SQLite is single-writer
db.SetMaxIdleConns(1)
```

### Transactions

```go
tx, err := db.BeginTx()
if err != nil {
    return fmt.Errorf("failed to begin transaction: %w", err)
}
defer tx.Rollback()  // Rollback if not committed

// Do work...
_, err = tx.Exec("INSERT INTO ...")
if err != nil {
    return err  // Deferred rollback happens
}

// Commit
if err := tx.Commit(); err != nil {
    return fmt.Errorf("failed to commit: %w", err)
}
```

**Pattern:** Always use `defer tx.Rollback()` for safety.

### Query Patterns

**Single row:**
```go
var task Task
err := db.QueryRow("SELECT id, text FROM tasks WHERE id = ?", taskID).
    Scan(&task.ID, &task.Text)
if err == sql.ErrNoRows {
    return nil, fmt.Errorf("task not found: %s", taskID)
}
```

**Multiple rows:**
```go
rows, err := db.Query("SELECT id, text FROM tasks WHERE status = ?", "open")
if err != nil {
    return nil, err
}
defer rows.Close()

var tasks []Task
for rows.Next() {
    var task Task
    if err := rows.Scan(&task.ID, &task.Text); err != nil {
        return nil, err
    }
    tasks = append(tasks, task)
}
if err := rows.Err(); err != nil {
    return nil, err
}
```

**Exec (insert/update/delete):**
```go
result, err := db.Exec(
    "INSERT INTO tasks (id, text, status) VALUES (?, ?, ?)",
    task.ID, task.Text, task.Status,
)
if err != nil {
    return fmt.Errorf("failed to insert task: %w", err)
}
```

## JSON Encoding

### Tags as JSON

Tasks and notes store tags as JSON arrays:

**Writing:**
```go
import "encoding/json"

tagsJSON, err := json.Marshal(task.Tags)
if err != nil {
    return fmt.Errorf("failed to marshal tags: %w", err)
}

db.Exec("INSERT INTO tasks (..., tags) VALUES (..., ?)", string(tagsJSON))
```

**Reading:**
```go
var tagsJSON string
err := row.Scan(&task.ID, &task.Text, &tagsJSON)

if tagsJSON != "" {
    if err := json.Unmarshal([]byte(tagsJSON), &task.Tags); err != nil {
        return fmt.Errorf("failed to unmarshal tags: %w", err)
    }
}
```

## File System Operations

### Reading Journal Files

```go
content, err := os.ReadFile(filePath)
if err != nil {
    if os.IsNotExist(err) {
        return "", nil  // File doesn't exist yet (new day)
    }
    return "", fmt.Errorf("failed to read file: %w", err)
}
return string(content), nil
```

### Writing Journal Files

**Atomic write pattern:**
```go
// Write to temp file
tmpPath := filePath + ".tmp"
if err := os.WriteFile(tmpPath, []byte(content), 0644); err != nil {
    return fmt.Errorf("failed to write temp file: %w", err)
}

// Atomic rename (on Unix systems)
if err := os.Rename(tmpPath, filePath); err != nil {
    os.Remove(tmpPath)  // Clean up temp file
    return fmt.Errorf("failed to rename file: %w", err)
}
```

### Directory Structure

```go
// Ensure directory exists
dirPath := filepath.Dir(filePath)
if err := os.MkdirAll(dirPath, 0755); err != nil {
    return fmt.Errorf("failed to create directory: %w", err)
}
```

## Common Tasks

### Add a New Table

1. **Create migration:**
   ```sql
   CREATE TABLE new_items (
       id TEXT PRIMARY KEY,
       text TEXT NOT NULL,
       created_at INTEGER NOT NULL
   );

   CREATE INDEX idx_new_items_created ON new_items(created_at);
   ```

2. **Add model** in `/internal/journal/models.go`:
   ```go
   type NewItem struct {
       ID        string
       Text      string
       CreatedAt time.Time
   }
   ```

3. **Add repository methods** in `database.go`:
   ```go
   func (db *Database) SaveNewItem(item *NewItem) error { ... }
   func (db *Database) GetNewItems() ([]NewItem, error) { ... }
   ```

4. **Add tests** in `database_test.go`

### Add an Index

```sql
CREATE INDEX idx_table_column ON table_name(column_name);
```

**When to index:**
- Columns used in WHERE clauses
- Foreign key columns
- Columns used in ORDER BY
- Columns used in JOINs

**When NOT to index:**
- Small tables (< 1000 rows)
- Columns with low cardinality (few unique values)
- Columns that are rarely queried

### Query Optimization

**Use EXPLAIN QUERY PLAN:**
```bash
sqlite3 ~/.reckon/reckon.db
> EXPLAIN QUERY PLAN SELECT * FROM tasks WHERE status = 'open';
```

**Look for:**
- "SCAN TABLE" without index (bad for large tables)
- "SEARCH TABLE ... USING INDEX" (good)

## Testing

### In-Memory Database for Tests

```go
func setupTestDB(t *testing.T) *storage.Database {
    db, err := storage.NewDatabase(":memory:")
    if err != nil {
        t.Fatalf("Failed to create test database: %v", err)
    }
    t.Cleanup(func() { db.Close() })
    return db
}
```

**Benefits:**
- Fast (no disk I/O)
- Isolated (no shared state)
- Clean (fresh for each test)

### Test Fixtures

```go
func createTestJournal(t *testing.T, db *storage.Database) *journal.Journal {
    j := &journal.Journal{
        Date: "2025-01-15",
        Intentions: []journal.Intention{
            {ID: "int1", Text: "Test intention", Status: "open"},
        },
    }
    repo := journal.NewRepository(db)
    if err := repo.SaveJournal(j); err != nil {
        t.Fatalf("Failed to save test journal: %v", err)
    }
    return j
}
```

## Common Pitfalls

### 1. SQL Injection (Use Placeholders!)

```go
// ❌ NEVER do this - SQL injection vulnerability
query := fmt.Sprintf("SELECT * FROM tasks WHERE id = '%s'", taskID)
db.Query(query)

// ✅ Always use placeholders
db.Query("SELECT * FROM tasks WHERE id = ?", taskID)
```

### 2. Forgetting to Close Rows

```go
// ❌ Leaks connections
rows, _ := db.Query("SELECT ...")
for rows.Next() { ... }
// Forgot defer rows.Close()

// ✅ Always defer Close
rows, err := db.Query("SELECT ...")
if err != nil {
    return err
}
defer rows.Close()  // CRITICAL
for rows.Next() { ... }
```

### 3. Not Checking rows.Err()

```go
// ❌ Misses errors during iteration
for rows.Next() {
    rows.Scan(&item)
    items = append(items, item)
}
return items  // May have incomplete data

// ✅ Check for errors
for rows.Next() {
    rows.Scan(&item)
    items = append(items, item)
}
if err := rows.Err(); err != nil {
    return nil, err
}
return items
```

### 4. Concurrent Writes

SQLite is single-writer. Don't try concurrent writes:
```go
// ❌ Will cause "database is locked" errors
go db.Exec("INSERT ...")
go db.Exec("INSERT ...")

// ✅ Serialize writes or use transactions
tx, _ := db.BeginTx()
tx.Exec("INSERT ...")
tx.Exec("INSERT ...")
tx.Commit()
```

## Rebuild Database

The `rk rebuild` command recreates the database from markdown files:

1. Delete existing database
2. Create fresh database with schema
3. Scan all journal files
4. Parse each file
5. Insert into database

**When to rebuild:**
- Database corruption
- Schema changes during development
- Testing with fresh state
- After manual file edits

## Resources

- **SQLite docs:** https://www.sqlite.org/docs.html
- **modernc SQLite (Go driver):** https://pkg.go.dev/modernc.org/sqlite
- **Database schema:** See tables above
- **Migration examples:** `/internal/db/migrations/*.sql`
