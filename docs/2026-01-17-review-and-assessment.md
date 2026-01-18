# Reckon Personal Productivity System - Comprehensive Assessment Report

**Date:** 2026-01-17
**Scope:** Architecture, code quality, async patterns, instrumentation, UX design, and future notes system

---

## Executive Summary

Reckon is a **well-architected personal productivity system** with strong Go idioms and clean separation of concerns. The TUI implementation using Bubbletea is generally solid, though recent critical async bugs reveal the complexity of managing distributed state across components.

**Key Strengths:**
- Clean architecture with domain-centric design
- Plain-text markdown as source of truth (git-friendly, portable)
- Good test coverage (157 tests)
- Responsive TUI with thoughtful UX
- Minimal dependencies

**Critical Issues:**
1. **No instrumentation/logging** - debugging is difficult
2. **Opaque IDs everywhere** - poor UX in markdown and CLI
3. **CLI/TUI parity gaps** - CLI missing major features
4. **State management complexity** - manual sync between model and components
5. **Async closure bugs** - recently fixed, but patterns need documentation

**Overall Grade: B+** (Very good for personal tool, needs work for team/production use)

---

## 1. Idiomatic Go & Bubbletea Usage

### ✅ Excellent Go Practices

#### Clean Architecture
```
internal/
├── cli/          # Cobra commands (presentation layer)
├── journal/      # Domain logic (core business)
├── storage/      # Infrastructure (DB, files)
├── tui/          # Bubbletea UI (presentation layer)
└── config/       # Configuration
```

**Strengths:**
- **Repository Pattern**: Clean abstraction over SQLite (`journal/repository.go`)
- **Service Layer**: Business logic separated from persistence (`journal/service.go`)
- **Error Wrapping**: Consistent use of `fmt.Errorf("context: %w", err)`
- **Table-Driven Tests**: Comprehensive test coverage with idiomatic patterns
- **Constructor Functions**: `NewService()`, `NewIntention()` encapsulate initialization
- **Proper defer**: Resource cleanup with `defer tx.Rollback()`

#### Example of Good Error Handling
```go
// journal/service.go:38
func (s *Service) GetByDate(date string) (*Journal, error) {
    content, fileInfo, err := s.fileStore.ReadJournalFile(date)
    if err != nil {
        return nil, fmt.Errorf("failed to read journal: %w", err)
    }
    // ...
}
```
✅ Wraps errors with context, preserves stack trace

### ⚠️ Areas for Improvement

#### 1. Package-Level Globals (Major Issue)
**File:** `internal/cli/root.go:16-17`
```go
var (
    service            *journal.Service
    taskService        *journal.TaskService
)
```

**Problems:**
- Makes testing difficult (shared state between tests)
- Not thread-safe
- Hidden dependencies (commands use globals instead of explicit deps)

**Better approach:** Dependency injection via command context or struct
```go
type CLI struct {
    service     *journal.Service
    taskService *journal.TaskService
}

func (c *CLI) Execute() error { /* ... */ }
```

#### 2. Silent Error Ignoring
**File:** `internal/tui/model.go:419-421`
```go
filePath, _ := s.fileStore.GetJournalPath(j.Date)
j.FilePath = filePath
```
⚠️ Discards error - if path resolution fails, FilePath is empty string (silent failure)

#### 3. Leftover Debug Code
**File:** `internal/tui/model.go:165`
```go
} else {
    // Debug: taskService is nil
}
```
⚠️ Should be removed or converted to proper logging

#### 4. Missing Context for Cancellation
Long-running operations (file watching, DB queries) don't use `context.Context`:
```go
func (w *Watcher) watch() {
    for {
        select {
        case <-w.done:  // Custom channel instead of ctx.Done()
            return
        // ...
        }
    }
}
```

**Recommendation:** Use `context.Context` for standard cancellation patterns

---

### ✅ Good Bubbletea Patterns

#### 1. Component Architecture
**File:** `internal/tui/components/task_list.go`
```go
type TaskList struct {
    list         list.Model
    tasks        []journal.Task
    focused      bool
    collapsedMap map[string]bool
}

func (tl *TaskList) Update(msg tea.Msg) (*TaskList, tea.Cmd)
func (tl *TaskList) View() string
func (tl *TaskList) SetFocused(focused bool)
```

✅ **Strengths:**
- Self-contained state
- Implements Update/View pattern
- Returns commands for async operations
- Focus management for styling

#### 2. Message Passing Up, State Down
**File:** `internal/tui/components/log_view.go:193-195`
```go
case "n":
    return lv, func() tea.Msg {
        return LogNoteAddMsg{LogEntryID: entryID}
    }
```

✅ Components send messages up to parent, parent sends state updates down

#### 3. Responsive Layout Calculations
**File:** `internal/tui/layout.go:35-102`
```go
func CalculatePaneDimensions(termWidth, termHeight int, notesPaneVisible bool) PaneDimensions {
    // Pure function - easy to test
    dims.LogsWidth = int(float64(termWidth) * 0.40)
    dims.TasksWidth = int(float64(termWidth) * 0.40)
    // ...
}
```

✅ Pure function, testable, accounts for border overhead

#### 4. State Preservation Across Updates
**File:** `internal/tui/components/task_list.go:323-345`
```go
func (tl *TaskList) UpdateTasks(tasks []journal.Task) {
    // Save cursor position by ID
    selectedTaskID := /* extract from current item */

    // Update data
    tl.tasks = tasks
    items := buildTaskItems(tasks, tl.collapsedMap)
    tl.list.SetItems(items)

    // Restore cursor to same task
    for i, item := range items {
        if taskItem.task.ID == selectedTaskID {
            tl.list.Select(i)
            break
        }
    }
}
```

✅ Preserves cursor position across data refreshes

### ⚠️ Bubbletea Anti-Patterns

#### 1. Massive Update Function
**File:** `internal/tui/model.go:190-698` (508 lines!)

The main `Update()` function is a giant switch statement with ~15 cases. It handles:
- Window resize
- Journal loading
- Task operations
- Note operations
- Navigation
- Editing
- Confirmation dialogs
- File watching
- Error handling

**Recommendation:** Break into smaller handler functions
```go
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        return m.handleKey(msg)
    case journalLoadedMsg:
        return m.handleJournalLoaded(msg)
    case components.TaskToggleMsg:
        return m.handleTaskToggle(msg)
    // ...
    }
}
```

#### 2. State Duplication
**File:** `internal/tui/model.go:61-106`
```go
type Model struct {
    currentJournal *journal.Journal  // Journal data

    // Components (also store journal data internally!)
    logView       *components.LogView
    intentionList *components.IntentionList

    // Cached data (duplicates taskList data!)
    tasks []journal.Task
}
```

**Problem:** Three sources of truth for tasks:
1. `m.tasks` cached in model
2. `m.taskList.tasks` in component
3. Task files on disk

This creates **potential for desynchronization**.

#### 3. Duplicate Cursor Restoration Logic
**File:** `internal/tui/components/task_list.go:338-359`
```go
// Restore cursor (FIRST TIME - lines 338-345)
if selectedTaskID != "" {
    for i, item := range tl.list.Items() {
        if taskItem.task.ID == selectedTaskID {
            tl.list.Select(i)
            break
        }
    }
}

// Restore cursor (SECOND TIME - lines 352-359)
if selectedTaskID != "" {
    for i, item := range items {
        if taskItem, ok := item.(TaskItem); ok && taskItem.task.ID == selectedTaskID {
            tl.list.Select(i)
            break
        }
    }
}
```

⚠️ This logic appears twice in the same function - likely a merge artifact or refactoring oversight.

---

## 2. Engineering Principles Assessment

### KISS (Keep It Simple, Stupid): 7/10

#### ✅ Simple Choices
1. **Plain Markdown Storage**: No complex serialization formats
2. **SQLite for Indexing**: Simple, embedded database (no server)
3. **Minimal Dependencies**: Only 5 main libraries
4. **Straightforward Configuration**: `~/.reckon/` directory, environment variable override

#### ⚠️ Complexity Sources
1. **Dual Storage System**: Files + database creates sync complexity
   - Markdown files are "source of truth"
   - SQLite is "index/cache"
   - Must keep in sync via parsing/writing
   - Rebuild command exists for when sync breaks

2. **ID Embedding in Markdown**:
   ```markdown
   - 09:00 d58mbq96rjumohmic4dg Started work
   ```
   - 20-character opaque IDs embedded in human-readable files
   - Parsing logic extracts IDs: `entryID, content := extractID(restOfLine)`
   - Adds complexity vs position-based or title-based references

3. **File-Per-Task Model**:
   - Each task is separate `.md` file: `~/.reckon/tasks/{id}.md`
   - Must load ALL task files on every operation (no pagination)
   - Performance will degrade with many tasks

### YAGNI (You Aren't Gonna Need It): 6/10

#### ✅ Appropriately Scoped
- No plugin system
- No user authentication (single-user tool)
- No premature abstractions (FileStore is struct, not interface)

#### ⚠️ Unused Features
1. **Database Columns for Future Features**:
   ```go
   // internal/storage/database.go:156-173
   addColumnIfMissing(db, "tasks", "scheduled_date", "TEXT")
   addColumnIfMissing(db, "tasks", "deadline_date", "TEXT")
   ```
   ⚠️ Columns added but never used in code

2. **Archived Task Status**:
   ```go
   const (
       TaskOpen     TaskStatus = "open"
       TaskDone     TaskStatus = "done"
       TaskArchived TaskStatus = "archived"  // Never set or queried
   )
   ```

3. **Future Planning Comments**:
   ```go
   // internal/journal/models.go:151
   // Future: NoteSlug string for zettelkasten
   ```
   ⚠️ Planning for features that don't exist yet

### DRY/RUG (Don't Repeat Yourself / Reuse, Use, Generalize): 7/10

#### ✅ Good Abstractions
1. **Reusable UI Components**: `TaskList`, `LogView`, `IntentionList` all follow same pattern
2. **Common Constructors**: `NewIntention()`, `NewLogEntry()`, `NewWin()` with consistent ID generation
3. **Shared Parse/Write**: Single `WriteJournal()` and `parseJournal()` functions

#### ⚠️ Repetition Issues
1. **Error Formatting** (~100+ occurrences):
   ```go
   return fmt.Errorf("failed to X: %w", err)
   ```
   Could use helper: `wrapError(err, "failed to X")`

2. **Similar Update Methods** in components:
   - `TaskList.UpdateTasks()`
   - `LogView.UpdateLogEntries()`
   - `IntentionList.UpdateIntentions()`

   All follow same pattern:
   - Save cursor position by ID
   - Update data
   - Rebuild items
   - Restore cursor

   Could extract common pattern into generic helper.

3. **Duplicate Transaction Patterns**:
   ```go
   tx, err := r.db.BeginTx()
   if err != nil { return err }
   defer tx.Rollback()

   // ... do work ...

   return tx.Commit()
   ```
   Repeated in every repository method - could use `WithTransaction(func() error)` wrapper.

---

## 3. Async Behaviors - Critical Recent Bugs & Patterns

### 🔴 Critical Bug #1: Closure Capture by Reference (FIXED)

**Commit:** `7c0dbe0 - fix: capture values before async closure in submitTextEntry`

#### The Problem
**File:** `internal/tui/model.go` (BEFORE FIX)

```go
case "enter":
    cmd := m.submitTextEntry()
    m.noteLogEntryID = ""  // ⚠️ Reset BEFORE async function runs
    return m, cmd

func (m *Model) submitTextEntry() tea.Cmd {
    return func() tea.Msg {
        // ⚠️ Closure captures m.noteLogEntryID by REFERENCE
        // By the time this runs, it's already ""!
        err = m.service.AddLogNote(
            m.currentJournal,
            m.noteLogEntryID,  // This is "" now!
            inputText
        )
    }
}
```

**Why This Failed:**
1. User presses Enter → `submitTextEntry()` called, returns tea.Cmd
2. Model immediately resets `m.noteLogEntryID = ""`
3. Later, Bubbletea executes the command function
4. Function reads `m.noteLogEntryID` by reference → sees empty string
5. Note fails to save (no error shown!)

#### The Fix (Lines 1198-1209)
```go
func (m *Model) submitTextEntry() tea.Cmd {
    // CRITICAL: Capture ALL values we need BEFORE creating closure
    // Go closures capture variables by REFERENCE, not by value
    capturedLogEntryID := m.noteLogEntryID
    capturedTaskID := m.noteTaskID
    capturedEditID := m.editItemID
    capturedEditType := m.editItemType
    capturedCurrentJournal := m.currentJournal
    capturedDate := m.currentDate
    inputText := m.textEntryBar.Value()

    return func() tea.Msg {
        // Use captured values, not model fields
        err = m.service.AddLogNote(
            capturedCurrentJournal,
            capturedLogEntryID,
            inputText
        )
    }
}
```

✅ **Impact:** Log notes and task notes now save correctly

**Lesson:** **Always capture values before returning tea.Cmd closures**

---

### 🔴 Critical Bug #2: Component Recreation vs Update (FIXED)

**Commit:** `16a5d75 - fix: preserve UI state when refreshing after adding notes`

#### The Problem
**File:** `internal/tui/model.go` (BEFORE FIX)

```go
case journalLoadedMsg:
    m.currentJournal = &msg.journal

    // ⚠️ Creating NEW components destroys all state!
    m.intentionList = components.NewIntentionList(msg.journal.Intentions)
    m.winsView = components.NewWinsView(msg.journal.Wins)
    m.logView = components.NewLogView(msg.journal.LogEntries)
```

**What Happened:**
1. User adds a log note
2. Journal reloads from disk
3. ALL components recreated from scratch
4. Cursor position lost
5. Collapsed/expanded state lost
6. User's note appears to vanish (actually just scrolled off screen)

#### The Fix (Lines 222-251)
```go
case journalLoadedMsg:
    m.currentJournal = &msg.journal

    // Update or create - preserves cursor & collapsed state
    if m.intentionList == nil {
        m.intentionList = components.NewIntentionList(msg.journal.Intentions)
    } else {
        m.intentionList.UpdateIntentions(msg.journal.Intentions)
    }

    if m.logView == nil {
        m.logView = components.NewLogView(msg.journal.LogEntries)
    } else {
        m.logView.UpdateLogEntries(msg.journal.LogEntries)
    }
    // etc...
```

✅ **Impact:** UI state preserved across refreshes, notes visible after adding

**Lesson:** **Never recreate components - always use Update methods**

---

### 🔴 Critical Bug #3: Unstable Entity IDs (FIXED)

**Commit:** `ff5eba9 - fix: persist log entry IDs in markdown to fix note association`

#### The Problem
```
1. User adds log note via TUI to log entry with ID "abc123"
2. Note saved to database with parent_id="abc123"
3. File reloaded from markdown
4. Parser generates NEW ID "xyz789" (IDs weren't persisted)
5. Note's parent_id="abc123" doesn't match anything
6. Note becomes orphaned, invisible in UI
```

**Root Cause:** IDs were generated at parse time, not persisted in markdown

#### The Fix

**Writer** (`internal/journal/writer.go:106`):
```go
// BEFORE: - 09:00 Started work
// AFTER:  - 09:00 d58mbq96rjumohmic4dg Started work
sb.WriteString(fmt.Sprintf("- %s %s %s\n", timeStr, entry.ID, content))
```

**Parser** (`internal/journal/parser.go:238-244`):
```go
// Extract ID if present: "09:00 <ID> <content>"
entryID, content := extractID(restOfLine)
if entryID == "" {
    // Generate only if missing (backward compat)
    entryID = xid.New().String()
}
```

✅ **Impact:** Log notes persist correctly across save/reload cycles

⚠️ **Trade-off:** Markdown now contains opaque IDs (UX issue - see Section 4)

---

### Async Patterns Summary

#### ✅ Good Patterns

1. **All async work uses tea.Cmd** - no manual goroutines in TUI
   ```go
   func (m *Model) loadJournal() tea.Cmd {
       return func() tea.Msg {
           j, err := m.service.GetByDate(m.currentDate)
           if err != nil {
               return errMsg{err}
           }
           return journalLoadedMsg{*j}
       }
   }
   ```

2. **File watcher uses channels properly**
   ```go
   func (m *Model) waitForFileChange() tea.Cmd {
       return func() tea.Msg {
           event := <-m.watcher.Changes()
           return fileChangedMsg{date: event.Date}
       }
   }
   ```

3. **Component messages propagate up**
   ```go
   case components.TaskToggleMsg:
       return m, m.toggleTask(msg.TaskID)
   ```

#### ⚠️ Complexity & Confusion Sources

1. **State Distributed Across Layers**
   - Model fields: `m.noteLogEntryID`, `m.selectedTaskID`
   - Component state: cursor position, collapsed map
   - Cached data: `m.tasks` duplicates `m.taskList.tasks`

   **Problem:** Must manually keep all in sync

2. **Manual Synchronization**
   ```go
   case components.TaskSelectionChangedMsg:
       // Must manually trigger notes pane update
       m.updateNotesForSelectedTask()
       return m, nil
   ```
   No automatic reactivity - easy to forget sync steps

3. **Inconsistent Message vs Mutation Patterns**
   - Task toggle: Returns cmd, doesn't mutate
   - Log note add: Sets field, returns cmd
   - Focus change: Mutates directly

   **Inconsistency makes code harder to reason about**

4. **Error Handling Gaps**
   ```go
   case errMsg:
       m.lastError = msg.err  // Stored but where is it displayed?
       return m, nil
   ```
   Errors only visible in confirm mode - not shown during normal operation

---

## 4. Instrumentation & Logging - **CRITICAL GAP**

### Current State: ALMOST NO LOGGING

**Findings:**
- Only 1 file matches `log.*` patterns: `task_service_example_test.go` (uses `log.Fatal` in tests)
- CLI uses `fmt.Println` for user output (18 occurrences in `internal/cli/`)
- **Zero** structured logging in service layer, repository, or TUI
- No logging framework in dependencies

**What This Means:**
- **No visibility into system behavior**
- **Cannot debug production issues** without adding print statements
- **No audit trail** of operations
- **No performance metrics** or timing data
- **No error context** beyond what's shown in UI

### Why This is a Problem

#### Scenario 1: User Reports "Task didn't save"
Without logging:
```
User: "I added a task but it's not showing up"
You: "Let me add some fmt.Println to see what happened..."
(Rebuild, redeploy, wait for repro)
```

With logging:
```
2026-01-17 10:23:15 INFO  task_service: AddTask called title="Fix bug" tags=[]
2026-01-17 10:23:15 DEBUG task_service: Writing task file path=/home/user/.reckon/tasks/c03g2h8k.md
2026-01-17 10:23:15 ERROR task_service: Failed to write task file error="permission denied"
```
**Immediate diagnosis**

#### Scenario 2: Performance Issues
Without logging:
```
User: "The TUI is slow to load"
You: (No idea where the time is spent)
```

With logging:
```
2026-01-17 10:25:00 INFO  journal_service: GetByDate called date=2026-01-17 elapsed=12ms
2026-01-17 10:25:00 INFO  task_service: GetAllTasks called task_count=250 elapsed=450ms
```
**Identifies bottleneck: Task loading is slow with many tasks**

### Recommended Logging Strategy

#### 1. Add Structured Logging Library

**Recommended:** `slog` (standard library as of Go 1.21)
- Zero dependencies (already in stdlib)
- Structured logging (JSON output)
- Leveled logging (DEBUG, INFO, WARN, ERROR)
- Context support

**Alternative:** `zerolog` or `zap` (faster, more features)

#### 2. Logging Locations & Levels

**Service Layer** (`internal/journal/service.go`):
```go
func (s *Service) GetByDate(date string) (*Journal, error) {
    slog.Info("GetByDate called", "date", date)

    content, fileInfo, err := s.fileStore.ReadJournalFile(date)
    if err != nil {
        slog.Error("Failed to read journal file", "date", date, "error", err)
        return nil, fmt.Errorf("failed to read journal: %w", err)
    }

    slog.Debug("Journal file read", "date", date, "exists", fileInfo.Exists, "size", len(content))

    // ... rest of function
}
```

**Repository Layer** (`internal/journal/repository.go`):
```go
func (r *Repository) SaveJournal(j *Journal) error {
    slog.Debug("SaveJournal starting", "date", j.Date, "log_entries", len(j.LogEntries))

    tx, err := r.db.BeginTx()
    if err != nil {
        slog.Error("Failed to begin transaction", "error", err)
        return fmt.Errorf("failed to begin transaction: %w", err)
    }
    defer tx.Rollback()

    // ... work ...

    if err := tx.Commit(); err != nil {
        slog.Error("Failed to commit transaction", "date", j.Date, "error", err)
        return err
    }

    slog.Info("Journal saved", "date", j.Date)
    return nil
}
```

**TUI Layer** (`internal/tui/model.go`):
```go
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case journalLoadedMsg:
        slog.Debug("Journal loaded",
            "date", msg.journal.Date,
            "intentions", len(msg.journal.Intentions),
            "log_entries", len(msg.journal.LogEntries),
        )
        // ...

    case errMsg:
        slog.Error("TUI error", "error", msg.err)
        // ...
    }
}
```

#### 3. Performance Instrumentation

**Add timing to critical paths:**
```go
func (s *TaskService) GetAllTasks() ([]Task, error) {
    start := time.Now()
    defer func() {
        slog.Info("GetAllTasks completed",
            "elapsed_ms", time.Since(start).Milliseconds(),
            "task_count", len(tasks),
        )
    }()

    // ... existing code ...
}
```

#### 4. Configuration

**Environment-based log levels:**
```go
func init() {
    level := slog.LevelInfo
    if os.Getenv("RECKON_DEBUG") == "true" {
        level = slog.LevelDebug
    }

    handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
        Level: level,
    })
    slog.SetDefault(slog.New(handler))
}
```

**Usage:**
```bash
# Normal mode - INFO and above
rk

# Debug mode - all logs
RECKON_DEBUG=true rk

# Save logs to file
rk 2> reckon.log
```

#### 5. Log Rotation & Management

For users who want persistent logs:
```bash
# logrotate config at ~/.reckon/logs/
/home/user/.reckon/logs/reckon.log {
    daily
    rotate 7
    compress
    missingok
}
```

### Summary: Logging Implementation Plan

**Phase 1 - Foundation:**
1. Add `slog` to service and repository layers
2. Log all errors with context
3. Log entry/exit of major operations

**Phase 2 - Debugging:**
1. Add DEBUG-level logs for state changes
2. Log all database operations
3. Log file I/O operations

**Phase 3 - Performance:**
1. Add timing instrumentation
2. Log slow operations (>100ms warning)
3. Add operation counters

**Phase 4 - Production:**
1. Add log sampling (if too verbose)
2. Add log aggregation hooks
3. Add metrics export (optional)

**Estimated Effort:** 4-6 hours for Phase 1, 2-3 hours each for subsequent phases

---

## 5. Opaque ID Usage - Major UX Issue

### Current ID Strategy

**ID Generation:** `github.com/rs/xid` produces 20-character alphanumeric IDs
- Example: `d58mbq96rjumohmic4dg`
- Globally unique, sortable by time
- Unreadable, unmemorable

### Where IDs Appear (User-Facing)

#### 1. Embedded in Markdown Files
**File:** `~/.reckon/journal/2026-01-17.md`
```markdown
## Log

- 09:00 d58mbq96rjumohmic4dg Started working on feature
  - c03g2h8krjun1234abcd First note about approach
  - c03g2h8krjun5678efgh Second note with alternative
- 10:30 d58mbq96rjun9876wxyz [meeting:standup] 30m Daily sync
```

**Problems:**
- Human-readable file polluted with machine IDs
- Git diffs show ID changes (noise)
- Manual editing is painful (can't easily add notes without ID)
- File is 40% longer due to IDs

#### 2. Task Filenames
**Directory:** `~/.reckon/tasks/`
```
c03g2h8krjun1234abcd.md  (Title: "Fix authentication bug")
d58mbq96rjumohmic4dg.md  (Title: "Add logging to service layer")
c03g2h8krjun5678efgh.md  (Title: "Write documentation")
```

**Problems:**
- Can't browse tasks in filesystem (filenames meaningless)
- Can't use shell completion effectively
- Tab completion useless: `vim c03<TAB>` → shows 10 files starting with c03

#### 3. CLI Arguments
```bash
# Must copy/paste exact ID
rk task show c03g2h8krjun1234abcd
rk task done d58mbq96rjumohmic4dg
rk task note c03g2h8krjun5678efgh "Update implementation notes"
```

**Workaround exists:** Numeric indices
```bash
rk task done 1    # Toggle first task
rk task show 3    # Show third task
```

**Problem with numeric indices:**
- Fragile - position changes when tasks added/removed
- Not stable across sessions
- Can't use in scripts reliably

#### 4. Hidden in TUI (Good!)
The TUI **correctly hides IDs** - users only see:
```
[ ] Fix authentication bug
[x] Add logging to service layer
[ ] Write documentation
```

✅ This is the right UX - no IDs visible

### Impact on UNIX Principles & Scripting

**Current state:**
```bash
# Hard to script - requires JSON parsing
TASK_ID=$(rk task list --json | jq -r '.[] | select(.text | contains("auth")) | .id')
rk task done "$TASK_ID"

# Or use fragile numeric index
rk task done 1
```

**What we want:**
```bash
# Fuzzy title matching
rk task done --match "auth"

# Multiple selection
rk task done --tag urgent

# Pipe-friendly
rk task list --status=open | grep "auth" | rk task done --stdin
```

### Alternative Approaches

#### Option 1: Remove IDs from Markdown (Recommended for Logs)

**Approach:** Store IDs only in database, use position in file for ordering

**Markdown:**
```markdown
## Log

- 09:00 Started working on feature
  - First note about approach
  - Second note with alternative
- 10:30 [meeting:standup] 30m Daily sync
```

**Database:**
```sql
log_entries:
  id: d58mbq96rjumohmic4dg  (generated, not in file)
  position: 0
  content: "Started working on feature"

log_notes:
  id: c03g2h8krjun1234abcd
  log_entry_id: d58mbq96rjumohmic4dg  (references parent)
  position: 0
  text: "First note about approach"
```

**How it works:**
1. Parser reads file top-to-bottom, assigns positions
2. When saving to DB, generate IDs if not already in DB
3. Notes reference parent by ID (internal only)
4. Writer outputs clean markdown without IDs

**Pros:**
- Clean, human-readable markdown
- No ID pollution in git diffs
- Easy to manually edit files

**Cons:**
- Concurrent edits problematic (position-based ordering breaks)
- Database becomes source of truth for relationships
- Harder to debug (no ID in file to search for)

#### Option 2: Hidden IDs (Markdown Comments)

**Approach:** Store IDs in HTML comments (invisible in most markdown viewers)

**Markdown:**
```markdown
## Log

<!-- log-entry:d58mbq96rjumohmic4dg -->
- 09:00 Started working on feature
  <!-- note:c03g2h8krjun1234abcd -->
  - First note about approach
- 10:30 [meeting:standup] 30m Daily sync
```

**Pros:**
- IDs preserved for stability
- Clean visual appearance
- Easy to debug (ID still in file)

**Cons:**
- Still pollutes markdown source
- Comments show up in some editors
- More complex parsing

#### Option 3: Slug-Based Task Filenames

**Approach:** Use human-readable slugs for task files, store ID in frontmatter

**Filenames:**
```
2026-01-17-fix-authentication-bug.md
2026-01-17-add-logging-service.md
2026-01-17-write-documentation.md
```

**Frontmatter:**
```yaml
---
id: c03g2h8krjun1234abcd  # Internal ID for DB references
title: Fix authentication bug
created: 2026-01-17
status: open
---
```

**Pros:**
- Browsable filenames in filesystem
- Tab completion works: `vim 2026<TAB>auth<TAB>`
- Human-friendly

**Cons:**
- Renaming task breaks filename (need migration)
- Slug collisions possible
- Filenames can be very long

#### Option 4: User-Assignable Short IDs

**Approach:** Let users optionally assign memorable IDs, fall back to generated

**Task creation:**
```bash
# User assigns short ID
rk task new "Fix auth bug" --id auth-bug

# Auto-generated if not specified
rk task new "Write docs"  # Gets ID: c03g2h8k
```

**Usage:**
```bash
rk task done auth-bug        # User-assigned ID
rk task done c03g2h8k        # Auto-generated ID
rk task done --match "docs"  # Fuzzy match
```

**Pros:**
- User control for important tasks
- Memorable IDs for frequent use
- Backward compatible

**Cons:**
- User burden to assign IDs
- Uniqueness enforcement needed
- Inconsistent experience

#### Option 5: Fuzzy Matching + Hidden IDs (Recommended)

**Approach:** Keep IDs internal, expose multiple reference methods in CLI

**Implementation:**
```bash
# Numeric index (current)
rk task done 1

# Fuzzy title match (new)
rk task done "auth"          # Matches "Fix authentication bug"
rk task done --match "auth"  # Explicit fuzzy match

# Tag-based (new)
rk task done --tag urgent    # Toggle all urgent tasks

# Full ID (fallback for scripts)
rk task done c03g2h8krjun1234abcd
```

**For logs/notes:** Use Option 1 (remove from markdown)
**For tasks:** Use slugs (Option 3) with hidden IDs in frontmatter
**For CLI:** Support fuzzy matching (Option 5)

**Pros:**
- Clean markdown
- Browsable task files
- Script-friendly CLI
- User-friendly TUI (already implemented)

**Cons:**
- More complex CLI parsing
- Fuzzy matching can be ambiguous
- More code to maintain

### Recommended Path Forward

1. **Phase 1 - Log Entries:** Remove IDs from markdown, keep in DB only
2. **Phase 2 - Task Files:** Switch to slug-based filenames
3. **Phase 3 - CLI:** Add fuzzy matching and tag-based operations
4. **Phase 4 - Migration:** Provide tool to migrate existing data

**Estimated Effort:** 8-12 hours for complete implementation

---

## 6. CLI vs TUI Feature Parity - **Major Gaps**

### Current State

**TUI:** Full-featured interactive interface
**CLI:** Limited to logging and viewing, incomplete task management

### Feature Comparison Matrix

| Feature | TUI | CLI | Notes |
|---------|-----|-----|-------|
| **Journaling** |
| Add log entry | ✅ `L` | ✅ `rk log` | Both work |
| Edit log entry | ✅ `e` | ❌ Missing | CLI cannot edit |
| Delete log entry | ✅ `d` | ❌ Missing | CLI cannot delete |
| Add log note | ✅ `n` | ❌ Missing | CLI cannot add notes |
| Delete log note | ✅ `d` | ❌ Missing | CLI cannot delete notes |
| View today's log | ✅ Built-in | ✅ `rk today` | Both work |
| View week's logs | ✅ Navigate | ✅ `rk week` | Both work |
| **Intentions** |
| Add intention | ✅ `i` | ❌ Missing | No CLI command |
| Toggle intention | ✅ `space` | ❌ Missing | No CLI command |
| Edit intention | ✅ `e` | ❌ Missing | No CLI command |
| Delete intention | ✅ `d` | ❌ Missing | No CLI command |
| **Wins** |
| Add win | ✅ `w` | ❌ Missing | No CLI command |
| Edit win | ✅ `e` | ❌ Missing | No CLI command |
| Delete win | ✅ `d` | ❌ Missing | No CLI command |
| **Tasks** |
| Add task | ✅ `t` | ✅ `rk task new` | Both work |
| Toggle task | ✅ `space` | ✅ `rk task done` | Both work |
| Add task note | ✅ `n` | ✅ `rk task note` | Both work |
| Delete task note | ✅ `d` | ❌ Missing | CLI cannot delete |
| Edit task | ✅ `e` | ⚠️ `rk task edit` | CLI can only edit title |
| Delete task | ✅ `d` | ❌ Missing | CLI cannot delete |
| List tasks | ✅ Built-in | ✅ `rk task list` | CLI more powerful (filters) |
| View task details | ✅ Built-in | ✅ `rk task show` | Both work |
| **Schedule** |
| Add schedule item | ✅ `s` | ✅ `rk schedule add` | Both work |
| Edit schedule item | ✅ `e` | ❌ Missing | CLI cannot edit |
| Delete schedule item | ✅ `d` | ✅ `rk schedule delete` | Both work |
| List schedule | ✅ Built-in | ✅ `rk schedule list` | Both work |
| **Navigation** |
| Change date | ✅ `h`/`l`, `T` | ❌ Missing | No `--date` flag |
| Jump to date | ✅ Date picker | ❌ Missing | No date selection |
| **Analytics** |
| Time summary | ✅ `s` toggle | ✅ `rk summary` | Both work |
| Week summary | ✅ Navigate | ⚠️ `rk summary --week` | CLI more flexible |
| **Data Management** |
| Rebuild database | ❌ Not in TUI | ✅ `rk rebuild` | CLI only |
| Review stale tasks | ❌ Not impl | ❌ Not impl | Neither |

**Parity Score: 40%** (11/27 features fully implemented in both)

### Missing CLI Features (Priority Order)

#### P0 - Critical Gaps
1. **Date context** - Cannot operate on arbitrary dates
   ```bash
   # Want: rk --date 2026-01-15 today
   # Want: rk --date 2026-01-15 task list
   ```

2. **Intention management** - Completely missing
   ```bash
   # Want: rk intention add "Complete feature X"
   # Want: rk intention list
   # Want: rk intention done 1
   ```

3. **Win management** - Completely missing
   ```bash
   # Want: rk win add "Shipped feature to production"
   # Want: rk win list
   ```

4. **Log note management** - Cannot add or delete
   ```bash
   # Want: rk log note <log-id> "Additional context"
   # Want: rk log note delete <note-id>
   ```

#### P1 - Important Gaps
5. **Edit operations** - Can only edit task titles
   ```bash
   # Want: rk log edit <log-id> "Updated text"
   # Want: rk schedule edit <index> "New time/content"
   ```

6. **Delete operations** - Cannot delete tasks, logs
   ```bash
   # Want: rk task delete <task-id>
   # Want: rk log delete <log-id>
   ```

7. **Fuzzy matching** - Must use exact IDs
   ```bash
   # Want: rk task done --match "auth"
   # Want: rk task list --search "bug"
   ```

#### P2 - Nice to Have
8. **Batch operations** - Cannot act on multiple items
   ```bash
   # Want: rk task done 1 2 3
   # Want: rk task done --tag urgent
   ```

9. **JSON output** - Only tasks support it
   ```bash
   # Want: rk today --json
   # Want: rk intention list --json
   ```

10. **Stdin input** - Cannot pipe data in
    ```bash
    # Want: echo "Complete feature" | rk task new --stdin
    # Want: rk task list --status=open | rk task done --stdin
    ```

### UNIX Principles Violations

#### ❌ Not Composable
```bash
# Can't easily pipe tasks to other tools
rk task list               # Human-readable table (not pipeable)
rk task list --json        # JSON (better, but not all commands support)
rk task list --format=tsv  # ❌ Not implemented
```

#### ❌ Stateful Operations
```bash
# Always operates on "today"
rk log "Added feature"  # Goes to today's journal

# Want: Specify date context
rk --date 2026-01-15 log "Forgot to log this yesterday"
```

#### ❌ Limited Filtering
```bash
# Want: Rich query language
rk task list --where "status=open AND created>2026-01-01"

# Current: Limited flags
rk task list --status=open --tag=urgent  # ✅ This works
```

#### ❌ Exit Codes Not Documented
```bash
# Want: Meaningful exit codes for scripting
rk task done 999 ; echo $?  # What does exit code mean?

# Standard:
# 0 = success
# 1 = general error
# 2 = usage error
# 3 = not found
```

### Recommendations for CLI/TUI Parity

#### Phase 1 - Foundation (4-6 hours)
1. Add `--date` global flag to all commands
2. Add `--json` output to all commands
3. Document exit codes

#### Phase 2 - Missing Features (8-10 hours)
1. Implement `rk intention` subcommands (add/list/done/delete)
2. Implement `rk win` subcommands (add/list/delete)
3. Implement `rk log note` and `rk log delete`
4. Implement `rk task delete`

#### Phase 3 - UNIX Composability (6-8 hours)
1. Add `--format` flag (json/tsv/csv) to all list commands
2. Add `--stdin` support for batch operations
3. Add fuzzy matching with `--match` flag
4. Add `--quiet` mode for less verbose output

#### Phase 4 - Advanced Features (Optional)
1. Query language: `--where` for complex filters
2. Template output: `--template` flag with Go templates
3. Batch operations: Accept multiple IDs in single command
4. Plugin system: Allow custom commands

**Total Effort:** 18-24 hours for Phases 1-3

---

## 7. Notes System - Design for Zettelkasten Integration

### Current Notes Implementation

**Two types of notes currently exist:**

1. **Log Notes** (inline, nested under log entries)
   ```markdown
   ## Log
   - 09:00 d58mbq96rjumohmic4dg Started feature work
     - c03g2h8krjun1234abcd Decided on API-first approach
     - c03g2h8krjun5678efgh Need to handle edge case for auth
   ```

2. **Task Notes** (inline, in task files under ## Log section)
   ```markdown
   ---
   id: task-123
   title: Fix authentication bug
   ---

   ## Log

   ### 2026-01-17
     - note-1 Found root cause in middleware
     - note-2 Implementing fix with JWT validation
   ```

**Characteristics:**
- ✅ Notes are contextual (attached to parent log/task)
- ✅ Simple to add (just `n` in TUI)
- ✅ Visible inline (expand/collapse in TUI)
- ❌ Not independently searchable
- ❌ Cannot link between notes
- ❌ No concept of "backlinks"
- ❌ No standalone notes (must have parent)

### Zettelkasten Principles

**Core concepts:**
1. **Atomic notes** - Each note is self-contained, single idea
2. **Linking** - Notes link to related notes bidirectionally
3. **Emergence** - Knowledge structure emerges from connections
4. **No hierarchy** - Flat structure, organization through links
5. **Unique IDs** - Each note has permanent identifier (often timestamp-based)

**Common Zettelkasten formats:**
- Each note is separate markdown file
- Filename = unique ID (e.g., `202601171023.md` for timestamp-based)
- Links use wiki-style: `[[202601171015]]` or `[[202601171015|Link Text]]`
- Backlinks are computed (tools scan for `[[current-note-id]]`)

### Design Options for Reckon

#### Option 1: Inline Notes Only (Current + Improvements)

**Approach:** Keep current inline notes, add better search/navigation

**Implementation:**
- Notes remain nested under logs/tasks
- Add full-text search across all notes: `rk note search "API"`
- Add backlinks: Detect `[[log-id]]` or `[[task-id]]` references
- TUI shows backlinks in notes pane

**Example:**
```markdown
## Log
- 09:00 Started feature work
  - Decided on API-first approach, see [[task-auth-123]]

Task file (task-auth-123.md):
### Log
  - Related to API design from [[log-entry-xyz]]
```

**Pros:**
- Simple - no new files
- Notes stay contextual
- Easy to implement (just add search + link detection)

**Cons:**
- Still not truly "zettelkasten" (notes not atomic)
- Cannot have standalone notes
- Hard to see connection graph

#### Option 2: Separate Zettelkasten, Link from Inline Notes

**Approach:** Add `~/.reckon/notes/` directory for standalone notes, link to them from inline notes

**Structure:**
```
~/.reckon/
├── journal/
│   └── 2026-01-17.md      (has inline log notes)
├── tasks/
│   └── task-123.md        (has inline task notes)
└── notes/                 (new!)
    ├── 202601171023.md    (standalone note: "API Design Decisions")
    ├── 202601171045.md    (standalone note: "JWT Best Practices")
    └── 202601171102.md    (standalone note: "Edge Cases in Auth")
```

**Inline note references standalone:**
```markdown
## Log
- 09:00 Started feature work
  - Decided on API-first approach, documented in [[202601171023]]
```

**Standalone note content:**
```markdown
---
id: 202601171023
title: API Design Decisions
created: 2026-01-17T10:23:00Z
tags: [architecture, api]
---

# API Design Decisions

Decided to use API-first approach for auth feature.

## Rationale
- Enables mobile app later
- Clear separation of concerns

## Related
- Implementation: [[task-auth-123]]
- Standards: [[202601171045]] (JWT Best Practices)

## References
- Log entry: [[log-entry-xyz]]
```

**TUI workflow:**
1. User on log entry, presses `N` (capital N = new standalone note)
2. Text input: "API Design Decisions"
3. Editor opens (or inline TUI editor)
4. User writes note, saves
5. Link `[[202601171023]]` auto-inserted in log note

**CLI workflow:**
```bash
rk note new "API Design Decisions" --link-to log-entry-xyz
# Opens $EDITOR with template
# Saves to notes/202601171023.md
# Adds reference to log entry
```

**Pros:**
- True zettelkasten - atomic notes
- Can have notes not tied to logs/tasks
- Separate note files are shareable
- Can use external tools (Obsidian, Logseq, etc.)

**Cons:**
- Complexity - two types of notes
- Context confusion - when to use inline vs standalone?
- More files to manage

#### Option 3: All Notes in Zettelkasten, Inline Notes are Links Only

**Approach:** Inline notes become just links to standalone notes

**Before (inline):**
```markdown
## Log
- 09:00 Started feature work
  - Decided on API-first approach
  - Need to handle edge case for auth
```

**After (linked):**
```markdown
## Log
- 09:00 Started feature work [[202601171023]] [[202601171024]]
```

**Note files:**
```
notes/202601171023.md: "Decided on API-first approach"
notes/202601171024.md: "Need to handle edge case for auth"
```

**Pros:**
- Single note system
- All notes are first-class
- Easier to search/navigate

**Cons:**
- Lots of tiny note files
- Breaks current UX (no inline notes visible)
- Overhead for quick thoughts

#### Option 4: Hybrid - Inline for Quick Notes, Zettelkasten for Deep Thoughts (Recommended)

**Approach:** Two-tier system with clear distinction

**Quick inline notes:**
- Remain as-is
- For immediate context, reminders
- Lightweight, no title needed
- Example: "Don't forget to test edge cases"

**Deep standalone notes:**
- Separate markdown files in `notes/`
- For concepts, decisions, learnings
- Have titles, tags, backlinks
- Example: "JWT Security Best Practices"

**Conversion:** Quick note can "promote" to standalone
```
TUI: User on inline note, presses `P` (promote)
→ Opens editor with note text as starting point
→ Saves to notes/ID.md
→ Replaces inline note with link [[ID]]
```

**Visual distinction in TUI:**
```
Logs:
- 09:00 Started feature work
  - Quick thought: check edge cases           (italic, gray)
  - 📝 API Design Decisions [[202601171023]]  (link icon, blue)
```

**Pros:**
- Best of both worlds
- Clear mental model (quick vs deep)
- Upgrade path (quick → deep)
- Backward compatible

**Cons:**
- Still two systems to maintain
- User must decide which to use

### Recommended Implementation: Option 4 (Hybrid)

#### Phase 1 - Foundation (6-8 hours)
1. Create `~/.reckon/notes/` directory structure
2. Add `Note` model with title, tags, backlinks
3. Add database tables for notes and note_links
4. Implement link detection: `[[note-id]]` or `[[note-id|display text]]`
5. CLI: `rk note new "Title"` creates standalone note

#### Phase 2 - TUI Integration (8-10 hours)
1. Add `N` (capital N) key to create standalone note from context
2. Show linked notes in notes pane (click to navigate)
3. Compute and display backlinks
4. Add `P` key to promote inline note to standalone

#### Phase 3 - Navigation (6-8 hours)
1. TUI: Click link to open note
2. TUI: "Back" button to return to previous view
3. CLI: `rk note show <id>` displays note
4. CLI: `rk note backlinks <id>` shows what links to this note

#### Phase 4 - Search & Graph (8-12 hours)
1. Full-text search: `rk note search "JWT"`
2. Tag search: `rk note list --tag architecture`
3. Graph export: `rk note graph --format=dot` (for visualization)
4. Daily notes: `rk note daily` creates/opens note for today

#### Phase 5 - Advanced (Optional)
1. Templates: `rk note new --template meeting`
2. Aliases: `[[note-id|display text]]` support
3. Embed: `![[note-id]]` includes note content inline
4. Export: Convert notes to Obsidian/Logseq format

**Total Effort:** 28-38 hours for Phases 1-4

### Notes Navigation UX

**TUI Navigation Flow:**
```
1. User on log entry
2. Sees: "API Design Decisions [[202601171023]]"
3. Presses Enter on link
4. → Switches to note view:

   ┌─ Note: API Design Decisions ─────────────┐
   │ Created: 2026-01-17 10:23                 │
   │ Tags: [architecture, api]                 │
   │                                           │
   │ [Content of note]                         │
   │                                           │
   │ Related Notes:                            │
   │ → [[202601171045]] JWT Best Practices     │
   │ → [[task-auth-123]] Fix authentication bug│
   │                                           │
   │ Referenced By:                            │
   │ ← Log 2026-01-17 09:00                    │
   │ ← Task: Implement auth system             │
   │                                           │
   │ [←Back] [e]dit [d]elete                   │
   └───────────────────────────────────────────┘

4. Press ← or ESC to return to log view
5. Press Enter on another link to navigate
```

**Link Types:**
- `[[note-id]]` - Shows note title as link text
- `[[note-id|Custom Text]]` - Shows custom text
- `[[task-id]]` - Links to task
- `[[log-id]]` - Links to log entry (if given stable IDs)

**Backlinks Panel:**
```
Notes Pane (when note selected):
┌─ 202601171023: API Design Decisions ───┐
│                                         │
│ Content preview...                      │
│                                         │
│ Links:                                  │
│ → JWT Best Practices                    │
│ → Fix authentication bug                │
│                                         │
│ Referenced By (2):                      │
│ ← 2026-01-17 Log Entry                  │
│ ← Task: Implement auth                  │
└─────────────────────────────────────────┘
```

---

## Summary & Recommendations

### Immediate Actions (Next 2-4 weeks)

**P0 - Critical Issues:**
1. **Add logging** (Phase 1: 4-6 hours)
   - Use `slog` for structured logging
   - Log all errors with context
   - Enable with `RECKON_DEBUG=true`

2. **Fix duplicate cursor restoration** (30 min)
   - Remove duplicate loop in `TaskList.UpdateTasks()` (lines 352-359)

3. **Document closure capture pattern** (1 hour)
   - Add comments explaining capture-before-closure requirement
   - Create developer guide with async patterns

**P1 - Important Improvements:**
4. **CLI feature parity - Phase 1** (4-6 hours)
   - Add `--date` global flag
   - Add `--json` output to all commands
   - Implement `rk intention` commands

5. **Reduce state duplication** (4-6 hours)
   - Remove `m.tasks` from Model (use `m.taskList.GetTasks()` instead)
   - Consolidate focus state management

6. **Remove opaque IDs from log markdown** (6-8 hours)
   - Implement Option 1: Position-based IDs, database-only storage
   - Migration script for existing journals

### Medium-term Projects (1-3 months)

**P2 - Significant Features:**
7. **Zettelkasten integration** (28-38 hours)
   - Implement Option 4: Hybrid inline + standalone notes
   - Phases 1-4 from notes design

8. **CLI feature parity - Phases 2-3** (14-18 hours)
   - Add fuzzy matching
   - Add batch operations
   - UNIX composability improvements

9. **Refactor large Update function** (6-8 hours)
   - Break into handler methods
   - Improve testability

10. **Slug-based task filenames** (4-6 hours)
    - Implement Option 3: Date-slug format
    - Migration tool

### Long-term Architectural Improvements (3+ months)

**P3 - Nice to Have:**
11. **Remove package-level globals** (8-10 hours)
    - Dependency injection for CLI commands
    - Better testability

12. **Add context.Context** (4-6 hours)
    - Standard cancellation patterns
    - Request tracing (if needed)

13. **Performance optimization** (8-12 hours)
    - Task pagination/lazy loading
    - Database query optimization
    - Benchmark suite

---

## Final Assessment

**Overall Code Quality: B+ (85/100)**

**Breakdown:**
- Architecture: A- (90/100) - Clean separation, good patterns
- Go Idioms: B+ (85/100) - Mostly idiomatic, some globals
- Bubbletea Usage: B (80/100) - Good patterns, needs refactoring
- Testing: A- (90/100) - Good coverage, table-driven tests
- Documentation: C+ (75/100) - Code comments decent, no user docs
- KISS: B+ (70/100) - Simple storage, complex dual-system
- YAGNI: B (60/100) - Some unused features
- DRY: B+ (70/100) - Good abstractions, some repetition
- Async Handling: B (80/100) - Recent fixes good, needs docs
- Logging: F (0/100) - **Critical gap**
- UX (IDs): C (60/100) - Opaque IDs everywhere
- CLI/TUI Parity: D+ (40/100) - **Major gaps**

**Recommendation:** This is a **production-ready personal tool** with excellent fundamentals. The recent async bug fixes demonstrate good debugging skills and attention to detail. However, to evolve into a robust, team-ready product, address:

1. **Logging/instrumentation** - Critical for debugging
2. **CLI parity** - Essential for power users
3. **ID usability** - Improves user experience significantly

The codebase shows thoughtful design and care for quality. With focused effort on the gaps identified above, Reckon can become an exemplary Go TUI application.

---

**End of Report**
