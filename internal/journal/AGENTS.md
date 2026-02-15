# Journal Subsystem Guide

## Overview

The journal subsystem handles parsing, writing, and managing daily journal files and tasks. It's the core domain logic layer.

**Key files:**
- `models.go` - Domain models (Journal, Intention, Task, LogEntry, etc.)
- `parser.go` - Markdown parsing (file → models)
- `writer.go` - Markdown writing (models → file)
- `service.go` - Business logic (orchestrates repo + file operations)
- `repository.go` - Database CRUD for journals
- `task_repository.go` - Database CRUD for tasks
- `task_parser.go` - Task-specific parsing
- `task_writer.go` - Task-specific writing

## Domain Model

### Core Concepts

**Intention** - A daily goal (1-3 per day max)
- Lives in daily journal only
- Statuses: `open`, `done`, `carried` (rolled to next day)
- Lightweight, disposable (not for multi-day work)

**Task** - Multi-day work item with its own log history
- Has schedule date, deadline, tags, notes
- Statuses: `open`, `done`
- Persisted to SQLite (global task list)
- Logged to both task context AND daily journal (with `[task:id]`)

**Log Entry** - Timestamped activity record
- Types: `log` (default), `meeting`, `break`
- Can link to tasks via task_id field
- Supports duration tracking (duration_minutes)
- Lives in daily journal

**Zettelkasten Note** - Knowledge card with wiki-style links
- Slug-based IDs (human-readable)
- Supports tags, backlinks
- Stored in date-based directory hierarchy
- Separate from daily journal

**Win** - Daily accomplishment to celebrate
- Simple text entries
- Lives in daily journal only

**Schedule Item** - Time-boxed events for the day
- Has start time and optional duration
- Lives in daily journal only

### Status Values

**Intention statuses:**
- `open` - Not yet done
- `done` - Completed
- `carried` - Rolled to next day (shown with `[>]`)

**Task statuses:**
- `open` - Active work
- `done` - Completed

**Log entry types:**
- `log` - Regular work log (default)
- `meeting` - Meeting time tracked
- `break` - Break time tracked

## File Format

### Daily Journal (`~/.reckon/journal/YYYY-MM-DD.md`)

```markdown
---
date: 2025-01-15
---

## Intentions

- [ ] Review PR #1234
- [x] Finish API refactor
- [>] Read requirements doc (carried from 2025-01-14)

## Schedule

- 09:00-10:00 Morning focus time
- 14:00-15:00 Team meeting

## Log

- 09:00 Started day, reviewing priorities
- 09:30 [task:abc123] Working on API refactor
- 10:30 [meeting:standup] 15m - Daily sync
- 12:00 [break] 45m - Lunch
  - Had good conversation with Bob about architecture
- 14:00 General research on OAuth flows

## Wins

- Shipped authentication fix
- Had productive 1:1 with manager
```

**Parsing rules:**
- Intentions: `- [ ]` = open, `- [x]` = done, `- [>]` = carried
- Schedule: `HH:MM-HH:MM Description` or `HH:MM Description`
- Log entries: `- HH:MM Content` (optional `[task:id]`, `[meeting:name]`, `[break]`)
- Duration: `30m`, `2h`, or `1h30m` at end of log entry
- Notes on entries: Indented with `  - ` under the entry
- Wins: Unordered list items

### Task File (Not currently used - tasks in SQLite)

**Note:** Tasks are currently stored in SQLite only. The original plan had separate task files, but this hasn't been implemented. Tasks live in the database.

### Zettelkasten Note (`~/.reckon/notes/YYYY/YYYY-MM/YYYY-MM-DD-slug.md`)

```markdown
---
title: Git interactive rebase workflow
created: 2025-01-12T10:30:00Z
updated: 2025-01-12T10:30:00Z
tags: git, workflow
---

To squash the last 3 commits:

    git rebase -i HEAD~3

Change `pick` to `squash` (or `s`) for commits to combine.

## See Also

- [[git-bisect-workflow]]
- [[git-cherry-pick]]
```

**Format:**
- YAML frontmatter with metadata
- Markdown content
- Wiki links: `[[slug]]` format (slug, not title)

## Parsing

### Parser Architecture

```
Markdown File → goldmark AST → Walker → Domain Models → Return
```

**Key functions:**
- `ParseJournal(content string, date string) (*Journal, error)` - Main entry point
- Section parsers: `parseIntentions()`, `parseLogs()`, `parseWins()`, etc.
- Helper parsers: `parseLogEntry()`, `parseDuration()`, `parseTaskRef()`

### Parsing Patterns

**State machine for sections:**
```go
switch {
case strings.HasPrefix(line, "## Intentions"):
    section = "intentions"
case strings.HasPrefix(line, "## Log"):
    section = "log"
// ... etc
}
```

**Regex for structured data:**
```go
// Log entry: "- 09:30 [task:abc123] Working on feature"
logPattern := regexp.MustCompile(`^-\s+(\d{2}:\d{2})\s+(.*)$`)

// Duration: "30m" or "1h30m"
durationPattern := regexp.MustCompile(`(\d+h)?(\d+m)?$`)
```

**Position tracking:**
Items maintain their position in the source file for stable sorting.

## Writing

### Writer Architecture

```
Domain Models → Markdown String Builder → Write to File
```

**Key functions:**
- `WriteJournal(journal *Journal, filePath string) error` - Main entry point
- Section writers: `writeIntentions()`, `writeLogs()`, `writeWins()`
- Formatters: `formatLogEntry()`, `formatDuration()`

### Writing Patterns

**Preserve order with position field:**
```go
// Sort by position before writing
sort.Slice(intentions, func(i, j int) bool {
    return intentions[i].Position < intentions[j].Position
})
```

**Atomic writes:**
```go
// Write to temp file, then rename (atomic on Unix)
tmpFile := filePath + ".tmp"
os.WriteFile(tmpFile, content, 0644)
os.Rename(tmpFile, filePath)
```

**YAML frontmatter:**
```go
fmt.Fprintf(w, "---\n")
fmt.Fprintf(w, "date: %s\n", journal.Date)
fmt.Fprintf(w, "---\n\n")
```

## Service Layer

### Responsibilities

- **Orchestration:** Coordinates between repository (DB) and file store
- **Business logic:** Intention carry-over, task linking, validation
- **Transaction management:** Ensures DB + file stay in sync

### Pattern: DB + File Sync

```go
func (s *Service) AddWin(journal *Journal, text string) error {
    // 1. Create domain model
    win := NewWin(text, nextPosition)
    journal.Wins = append(journal.Wins, *win)

    // 2. Write to file (source of truth)
    if err := s.writer.WriteJournal(journal); err != nil {
        return fmt.Errorf("failed to write journal: %w", err)
    }

    // 3. Update database (derived index)
    if err := s.repo.SaveJournal(journal); err != nil {
        return fmt.Errorf("failed to save to db: %w", err)
    }

    return nil
}
```

**Principle:** Files are source of truth, DB is rebuildable index.

## Common Tasks

### Add a New Field to Intention

1. Add to `models.go`:
   ```go
   type Intention struct {
       // ... existing fields
       YourField string `json:"your_field"`
   }
   ```

2. Update parser in `parser.go` to extract field
3. Update writer in `writer.go` to format field
4. Update repository SQL in `repository.go` to store field
5. Create migration in `internal/db/migrations/`
6. Add tests in `parser_test.go` and `writer_test.go`

### Add a New Section to Journal

1. Add slice to Journal model in `models.go`
2. Add parser function in `parser.go`
3. Add writer function in `writer.go`
4. Add to `ParseJournal()` section switch
5. Add to `WriteJournal()` output
6. Update repository to persist (if needed)

### Change Log Entry Format

**CAUTION:** Changing format breaks existing files!

1. Update parser regex in `parser.go`
2. Update writer format in `writer.go`
3. Add migration logic or version detection
4. Test with real journal files (use backups!)
5. Consider backward compatibility

## Testing

### Parser Tests

Use golden files for comprehensive testing:

```go
func TestParseJournal(t *testing.T) {
    content := readTestFile(t, "testdata/journal.md")
    journal, err := ParseJournal(content, "2025-01-15")

    // Test parsed values
    assert.NoError(t, err)
    assert.Equal(t, 3, len(journal.Intentions))
    assert.Equal(t, "open", journal.Intentions[0].Status)
}
```

### Roundtrip Tests

Ensure parse → write → parse produces same result:

```go
func TestRoundtrip(t *testing.T) {
    original := createTestJournal()

    // Write
    content := WriteJournalToString(original)

    // Parse
    parsed, err := ParseJournal(content, original.Date)

    // Compare
    assert.Equal(t, original, parsed)
}
```

### Test Files

- `testdata/*.md` - Sample journal files
- `*_test.go` - Test files next to implementation
- `*_example_test.go` - Executable examples (also used as tests)

## Common Pitfalls

### 1. Off-by-One Position Errors

Positions are 0-indexed internally but may display as 1-indexed:
```go
// ❌ Don't mix indexing
position := len(items)  // 0-indexed
display := position     // Shows wrong number to user

// ✅ Be explicit
position := len(items)  // 0-indexed for array access
display := position + 1 // 1-indexed for user display
```

### 2. Time Zone Handling

Always use UTC internally, convert for display:
```go
// ❌ Local time can cause date boundary issues
now := time.Now()

// ✅ Truncate and use UTC for date calculations
today := time.Now().UTC().Truncate(24 * time.Hour)
```

### 3. Missing Error Wrapping

```go
// ❌ Lost context
return err

// ✅ Add context
return fmt.Errorf("failed to parse intention on line %d: %w", lineNum, err)
```

### 4. Not Handling Empty Files

```go
// ❌ Crashes on empty file
lines := strings.Split(content, "\n")
firstLine := lines[0]  // Panic if empty

// ✅ Check length
if len(lines) == 0 {
    return &Journal{Date: date}, nil
}
```

## Database Schema

See `/internal/storage/AGENTS.md` for detailed schema documentation.

**Key tables:**
- `journals` - Journal metadata
- `intentions` - Daily intentions
- `log_entries` - Timestamped logs
- `tasks` - Multi-day tasks
- `notes` - Zettelkasten notes

## Migration Strategy

**Adding new fields:**
1. Create migration in `internal/db/migrations/NNNNN_description.sql`
2. Update models
3. Update repository queries
4. Test migration on sample database

**Changing format:**
1. Add version field to frontmatter
2. Support both old and new formats in parser
3. Write always uses new format
4. Provide migration script if needed

## Resources

- **goldmark docs:** https://github.com/yuin/goldmark
- **Markdown spec:** https://commonmark.org/
- **File format examples:** `testdata/*.md`
- **Parser tests:** `parser_test.go`
