# Agent Guidelines for Reckon

## What is Reckon?

Reckon is a **terminal-based productivity system** combining daily journaling, multi-day task management, time tracking, and zettelkasten-style knowledge capture. Built in Go with a Bubble Tea TUI and Cobra CLI.

**Core principle:** Plain-text markdown files are the source of truth. SQLite provides fast querying but is always rebuildable from markdown files.

**Key features:**
- Daily journal with intentions, logs, wins, and schedule
- Multi-day tasks with notes, deadlines, and time tracking
- Zettelkasten notes with wiki-style linking
- TUI for interactive management
- CLI for quick logging and scripting

## Domain Concepts (Read This Once)

**Intention** - A daily goal (1-3 per day max)
- Lives in daily journal only
- Statuses: `open`, `done`, `carried` (rolled to next day)
- Lightweight, for daily focus items

**Task** - Multi-day work item with its own history
- Has schedule date, deadline, tags, notes
- Statuses: `open`, `done`
- Tracked globally, can be scheduled/referenced in daily journals

**Log Entry** - Timestamped activity record
- Types: `log` (default), `meeting`, `break`
- Can link to tasks via `[task:id]`
- Supports duration tracking
- Lives in daily journal

**Zettelkasten Note** - Knowledge card with wiki-style links
- Slug-based IDs (human-readable, URL-safe)
- Supports tags, backlinks
- Stored in date-based hierarchy
- Searchable and interlinked

**Win** - Daily accomplishment to celebrate
- Simple text entries in daily journal

**Schedule Item** - Time-boxed event for the day
- Has start time and optional duration
- Lives in daily journal

## Current State (February 2026)

✅ **Implemented:**
- Daily journal (intentions, logs, wins, schedule)
- Multi-day task management with notes
- TUI with 40-40-18 layout (Log | Tasks | Schedule/Intentions/Wins)
- CLI commands for journaling, tasks, notes
- Zettelkasten notes (CLI: create, edit, show)
- Time tracking and summaries
- SQLite database with migrations

🚧 **In Progress:**
- Zettelkasten notes TUI (reckon-v89d)
- Notes search CLI (reckon-pfpb)
- CLI verb standardization (reckon-89hp)

⏸️  **Planned:**
- Periodic review system
- Enhanced logging/instrumentation
- Additional TUI editing capabilities

**See:** `bd stats` for current issue breakdown, `bd list --status=open` for active work

## Quick Start

### 1. Find Work
```bash
bd ready                   # Show unblocked issues
bd show <id>               # View issue details
bd update <id> --claim     # Claim issue atomically
```

### 2. Build and Test
```bash
go build -o rk ./cmd/rk    # Build binary
go test ./...              # Run all tests
go test -tags integration ./tests/  # Integration tests
```

### 3. Make Changes
- Read relevant subsystem docs (see below)
- Follow code style guidelines
- Write tests for new functionality

### 4. Complete Session
```bash
git add <files>            # Stage changes
bd sync                    # Sync beads changes
git commit -m "..."        # Commit with message
git push                   # Push to remote (MANDATORY)
```

**Work is NOT complete until `git push` succeeds.**

## Subsystem Documentation

For detailed context on specific parts of the codebase:

- **[TUI Guide](internal/tui/AGENTS.md)** - Bubble Tea patterns, async closures, component communication, state management
- **[Journal Guide](internal/journal/AGENTS.md)** - File format, parsing, writing, domain models, service layer
- **[CLI Guide](internal/cli/AGENTS.md)** - Cobra commands, flags, output formats, error handling
- **[Storage Guide](internal/storage/AGENTS.md)** - Database schema, migrations, queries, file operations

**When to read subsystem docs:**
- Working on TUI features → Read TUI Guide
- Changing journal format or parsing → Read Journal Guide
- Adding/modifying CLI commands → Read CLI Guide
- Schema changes or database work → Read Storage Guide

## Issue Tracking with Beads

This project uses **bd (beads) v0.43.0** for issue tracking.

**Essential commands:**
```bash
bd ready                           # Show unblocked issues
bd update <id> --claim             # Atomically claim issue
bd show <id>                       # View issue details
bd create "Title" --type task --priority 2
bd close <id>                      # Close issue
bd sync                            # Sync with git
bd dep add <a> <b>                 # A depends on B (B blocks A)
bd blocked                         # Show blocked issues
```

**For full workflow:** Run `bd prime` or see [docs/bd-usage.md](docs/bd-usage.md)

### Creating Well-Structured Issues

**Use ticket templates** for consistency and context:
- `.beads/templates/task.md` - Standard tasks/features
- `.beads/templates/bug.md` - Bug reports
- `.beads/templates/epic.md` - Large features

**Quick template usage:**
```bash
# Create issue
bd create "Issue title" --type task --priority 2

# Add description from template
cat .beads/templates/task.md
# Copy template, fill placeholders, set as description
bd update <id> --description="[paste filled template]"
```

**What makes a good issue:**
- Clear problem statement
- Proposed solution (if known)
- Files to modify listed
- Acceptance criteria checklist
- Relevant context and gotchas

See `.beads/templates/README.md` for detailed guidance.

## Testing Strategy

### Philosophy

**Test behavior, not implementation.** Tests should verify that the system does the right thing, not how it does it.

**Key principles:**
- Tests should be fast and reliable
- Tests should be independent (no shared state)
- Test one thing at a time
- Prefer many small tests over few large tests
- Integration tests validate multi-module workflows

### Test Pyramid

```
        /\
       /  \      5% - End-to-End (Full "day's work" validation)
      /----\
     /      \    25% - Integration (Multi-module workflows)
    /--------\
   /          \  70% - Unit (Isolated component testing)
  /____________\
```

**Unit Tests (70%)** - Fast, isolated, mock dependencies
- Parser/writer correctness
- Service logic with mocked repositories
- Model validation
- Utility functions

**Integration Tests (25%)** - Real database, multiple modules
- Input → parsing → storage → retrieval workflows
- File I/O + database sync
- Service layer with real repository

**End-to-End Tests (5%)** - Full system validation
- "Day's work" simulation (create logs, tasks, notes, verify connections)
- TUI visual/UX validation
- CLI command chains

### Unit Tests

**Location:** `*_test.go` files next to implementation

**What we test:**
- **Parsers:** Markdown → models (use golden files in `testdata/`)
- **Writers:** Models → markdown (roundtrip tests)
- **Services:** Business logic with mocked repositories
- **Models:** Validation, methods, state transitions

**What we DON'T test:**
- Actual file I/O (use mocks/in-memory)
- External dependencies (git, network)
- Bubble Tea rendering (test model updates only)

**Example:**
```go
func TestParseIntention(t *testing.T) {
    content := "- [ ] Review PR #123"
    intention, err := parseIntention(content, 0)

    assert.NoError(t, err)
    assert.Equal(t, "Review PR #123", intention.Text)
    assert.Equal(t, "open", intention.Status)
}
```

### Integration Tests

**Location:** `tests/integration_test.go` or `*_integration_test.go`

**What we test:**
- **End-to-end workflows:** User action → storage → retrieval
- **Multi-module coordination:** Parser + Service + Repository + File I/O
- **Data persistence:** Write to DB, read back, verify correctness
- **Migration scenarios:** Old data format → new format

**Tag:** Use build tag `//go:build integration` to separate from unit tests

**Example workflow tests:**
```go
// Test: Create intention → Save to file → Parse back → Verify in DB
func TestIntentionWorkflow(t *testing.T) {
    // 1. Create intention via service
    journal := service.GetJournal("2025-01-15")
    service.AddIntention(journal, "Complete task X")

    // 2. Verify written to file
    content := readJournalFile("2025-01-15")
    assert.Contains(t, content, "[ ] Complete task X")

    // 3. Parse file back
    parsed := parseJournal(content, "2025-01-15")

    // 4. Verify in database
    fromDB := repo.GetJournal("2025-01-15")
    assert.Equal(t, parsed.Intentions, fromDB.Intentions)
}
```

**Common integration scenarios:**
- Create task → Log time to it → Verify task.Notes updated
- Create note → Link to another note → Verify backlinks
- Edit intention in file → Rebuild DB → Verify sync
- Carry intention forward → Verify carried status + new journal entry

### TUI Visual & UX Testing

**Tool:** xterm.js + gotty + Playwright MCP

**Purpose:** Verify TUI rendering, interactions, and user experience

**Setup:**
1. **gotty:** Serves TUI over WebSocket
2. **xterm.js:** Renders terminal in browser
3. **Playwright MCP:** Automates browser interactions and captures screenshots

**What to test:**
- Layout rendering (40-40-18 columns visible)
- Color schemes and styling
- Keyboard navigation (tab, j/k, arrows)
- Component focus states
- Help text visibility
- Error message display
- Long text truncation/wrapping

**Example test scenario:**
```javascript
// Playwright test via MCP
test('Task list shows completed tasks with strikethrough', async () => {
    // 1. Launch TUI via gotty
    // 2. Navigate to tasks section (tab key)
    // 3. Create task and mark done (space key)
    // 4. Screenshot and verify strikethrough styling
    // 5. Verify task appears in done state
});
```

**When to use:**
- Visual regression testing (before/after screenshots)
- UX flow validation (multi-step interactions)
- Accessibility checks (focus indicators, contrast)
- Cross-terminal compatibility

**Note:** This is manual or semi-automated. Not part of regular CI (too slow/complex).

### End-to-End "Day's Work" Validation

**Concept:** Simulate a full day of productivity work and verify all connections.

**Scenario:**
```go
func TestFullDayWorkflow(t *testing.T) {
    day := "2025-01-15"

    // Morning: Set intentions
    service.AddIntention(journal, "Review PRs")
    service.AddIntention(journal, "Work on feature X")

    // Create task for multi-day work
    task := taskService.CreateTask("Implement auth system")
    taskService.AddTag(task, "backend")

    // Log work throughout day
    service.AddLog(journal, "09:00", "Started reviewing PRs")
    service.AddLog(journal, "10:30", fmt.Sprintf("[task:%s] Working on auth", task.ID))
    service.AddLog(journal, "12:00", "[break] Lunch")

    // Create zettelkasten note
    note := notesService.CreateNote("OAuth Flow Patterns", []string{"auth", "backend"})

    // Link note to task (future feature)
    // taskService.LinkNote(task.ID, note.Slug)

    // Afternoon: More work
    service.AddLog(journal, "14:00", fmt.Sprintf("[task:%s] Implemented token refresh", task.ID))
    taskService.AddNote(task, "Need to handle edge case: expired refresh token")

    // End of day: Mark intention done, add win
    service.ToggleIntention(journal, intentionID)
    service.AddWin(journal, "Completed PR reviews, made good progress on auth")

    // VERIFY EVERYTHING:
    // 1. Journal file has all entries
    content := readJournalFile(day)
    assert.Contains(t, content, "Review PRs")
    assert.Contains(t, content, "[break] Lunch")

    // 2. Task has notes
    loadedTask := taskService.GetTask(task.ID)
    assert.Len(t, loadedTask.Notes, 1)

    // 3. Task appears in log entries
    logs := service.GetLogEntries(day)
    taskLogs := filterByTaskID(logs, task.ID)
    assert.Len(t, taskLogs, 2)

    // 4. Note is findable by tag
    authNotes := notesService.GetNotesByTag("auth")
    assert.Contains(t, authNotes, note)

    // 5. Database and files are in sync
    dbJournal := repo.GetJournal(day)
    fileJournal := parseJournalFile(day)
    assert.Equal(t, dbJournal, fileJournal)
}
```

**Value:**
- Catches integration bugs missed by unit tests
- Validates real-world usage patterns
- Ensures data consistency across modules
- Documents expected system behavior

**Status:** 🎂 **Icing on the cake** - Nice to have, not implemented yet. This test would live in `tests/e2e_test.go` and serve as both validation and living documentation of how the system should work.

### Running Tests

```bash
# All tests (unit only)
go test ./...

# Specific package
go test ./internal/journal/...

# Single test
go test -run TestParseIntention ./internal/journal/

# With coverage
go test -cover ./...
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out  # View in browser

# Integration tests only
go test -tags integration ./tests/

# Verbose output
go test -v ./...

# Parallel execution (default)
go test -p 4 ./...

# Specific test file
go test ./internal/journal/parser_test.go

# Run until failure (stress test)
go test -count=100 ./internal/journal/
```

### Coverage Goals

**Target coverage by subsystem:**
- **Parsers/Writers:** >90% (critical for data correctness)
- **Service layer:** >80% (business logic must be solid)
- **Repository:** >70% (mostly CRUD, but important)
- **TUI models:** >60% (harder to test, focus on critical paths)
- **Overall project:** >70%

**Coverage is a guide, not a goal.** Don't test for coverage's sake. Focus on:
- Critical paths (data flow, user actions)
- Edge cases (empty input, invalid data)
- Error handling (what happens when things fail)

### Test Data

**Golden files:** Use `testdata/` directories for expected outputs
```
internal/journal/testdata/
├── journal-basic.md          # Simple journal file
├── journal-with-tasks.md     # Journal with task references
├── journal-empty.md          # Empty journal
└── intention-formats.md      # Various intention formats
```

**In-memory database:** Use `:memory:` for fast, isolated DB tests
```go
db, _ := storage.NewDatabase(":memory:")
```

**Test helpers:** Create reusable setup/teardown
```go
func setupTestDB(t *testing.T) *storage.Database {
    db, _ := storage.NewDatabase(":memory:")
    t.Cleanup(func() { db.Close() })
    return db
}
```

### Writing Good Tests

**DO:**
- ✅ Test one thing at a time
- ✅ Use descriptive test names: `TestParseIntention_WithCarriedStatus`
- ✅ Use table-driven tests for multiple cases
- ✅ Check both success and error paths
- ✅ Clean up resources (use `t.Cleanup()`)

**DON'T:**
- ❌ Test implementation details (internal state)
- ❌ Share state between tests (use fresh fixtures)
- ❌ Make tests depend on order (they run in parallel)
- ❌ Mock everything (test real collaborators when simple)
- ❌ Write tests that depend on timing (flaky tests)

### CI/CD Integration

**Currently:** Tests run locally before pushing

**Future:** GitHub Actions workflow
```yaml
- Run unit tests on every push
- Run integration tests on PRs
- Generate coverage reports
- Fail PR if coverage drops below threshold
```

## Code Style Guidelines

- **Formatting**: Use `go fmt` (standard Go formatting)
- **Imports**: stdlib → third-party → internal packages (blank line between groups)
- **Naming**: PascalCase for exported, camelCase for unexported
- **Packages**: lowercase, single word (journal, tui, cli, storage)
- **Errors**: Return errors, wrap with `fmt.Errorf("context: %w", err)`
- **Pointers**: Use for optional values and large structs to avoid copying
- **Types**: Strongly typed, avoid interface{}
- **Comments**: Document all exported functions/types
- **Enums**: Use iota for constants

## Git Workflow

### Standard Workflow (Single Agent)

**ALWAYS use short-lived feature branches.** Each ticket/issue gets its own branch.

**Starting work:**
```bash
git checkout main
git pull origin main
git checkout -b <issue-id>         # e.g., reckon-8ji
bd update <issue-id> --claim
# ... do work ...
```

**Completing work:**
```bash
git add <files>
git commit -m "feat: description"
bd close <issue-id>
bd sync
git push -u origin <issue-id>
gh pr create --base main
```

**Critical rules:**
- ONE branch per ticket/issue
- ALWAYS branch from `origin/main`
- Keep branches short-lived (hours to days, not weeks)
- Merge to main frequently via PRs
- Delete branches after merging

### Multi-Agent Workflow (Parallel Worktrees)

**When multiple agents work in parallel**, use git worktrees:

```bash
# From main repo
bd worktree create <issue-id>      # Creates worktree at ../<issue-id>/
cd ../<issue-id>/
bd update <issue-id> --claim
# ... work ...
git push -u origin <issue-id>
gh pr create --base main
```

**Benefits:**
- Each agent works in isolation (no checkout conflicts)
- Shared `.beads` database (all agents see same issue state)
- Atomic work claiming prevents duplicate work

**Cleanup:**
```bash
cd ../reckon
bd worktree remove <issue-id>      # After PR merges
```

**When to use worktrees:**
- Multiple agents working simultaneously
- Need to switch between issues without stashing
- Long-running work needs parallel development

**When NOT to use:**
- Single agent on one issue (use standard branches)
- Very short-lived changes (< 1 hour)

## Session Completion Checklist (MANDATORY)

Before ending a work session, **you MUST complete ALL steps**:

1. ✅ **File issues** for any remaining work or follow-ups
2. ✅ **Run quality gates** (if code changed):
   ```bash
   go test ./...              # Tests pass
   go vet ./...               # No lint errors
   go build -o rk ./cmd/rk    # Builds successfully
   ```
3. ✅ **Update issue status**:
   ```bash
   bd close <id> --reason="Completed in PR #XX"
   bd sync
   ```
4. ✅ **PUSH TO REMOTE** (MANDATORY):
   ```bash
   git add .
   git commit -m "..."
   bd sync
   git push
   git status                 # MUST show "up to date with origin"
   ```
5. ✅ **Clean up** worktrees if used
6. ✅ **Verify** all changes committed AND pushed
7. ✅ **Hand off** context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds

## Parallel Agent Coordination

**Atomic work claiming:**
```bash
bd ready                           # See available work
bd update <id> --claim             # Atomically claim (fails if already claimed)
```

The `--claim` flag:
- Sets assignee to you
- Sets status to `in_progress`
- Fails if already claimed (prevents duplicate work)
- Works across worktrees (shared database)

**Creating parallel work:**
```bash
bd create "Feature X" --type epic
bd create "Subtask 1" --type task --parent <epic-id>
bd create "Subtask 2" --type task --parent <epic-id>

# Multiple agents can now claim different subtasks
bd update <task-id> --claim        # Each agent claims their work
```

## Critical Gotchas (Must Know)

### 1. Async Closure Capture Bug (TUI)

**Problem:** Go closures capture by REFERENCE. In async commands, you must capture values BEFORE the closure.

```go
// ❌ WRONG - Bug waiting to happen
return func() tea.Msg {
    err := m.service.AddWin(m.currentJournal, text)  // m.currentJournal may have changed!
    return errMsg{err}
}

// ✅ CORRECT - Capture before closure
capturedJournal := m.currentJournal  // Capture value
return func() tea.Msg {
    err := m.service.AddWin(capturedJournal, text)  // Safe
    return errMsg{err}
}
```

**See:** `internal/tui/AGENTS.md` and `docs/ASYNC_PATTERNS.md` for full details

### 2. Don't Add Package-Level Globals

```go
// ❌ Don't add more of these (they exist in cli/root.go but are anti-pattern)
var service *journal.Service

// ✅ We're planning to refactor to dependency injection
```

### 3. Always Wrap Errors with Context

```go
// ❌ Lost context
return err

// ✅ Add context for debugging
return fmt.Errorf("failed to parse journal for %s: %w", date, err)
```

### 4. Files are Source of Truth

**Remember:** SQLite is a derived index. If database gets corrupted:
```bash
rk rebuild                         # Recreates database from markdown files
```

## Common Patterns

### Single Agent, Single Issue
```bash
git checkout -b reckon-abc origin/main
bd update reckon-abc --claim
# ... work ...
git add . && git commit -m "..."
bd close reckon-abc
bd sync && git push
gh pr create --base main
```

### Multi-Agent, Parallel Issues
```bash
# Agent 1
bd worktree create reckon-xyz
cd ../reckon-xyz
bd update reckon-xyz --claim
# ... work ...

# Agent 2 (simultaneously, different issue)
bd worktree create reckon-abc
cd ../reckon-abc
bd update reckon-abc --claim
# ... work ...
```

### Discovering New Work During Development
```bash
# While working, you find a bug
bd create "Fix validation bug" --type bug --priority 1
bd dep add reckon-current reckon-new  # New issue blocks current
bd update reckon-current --status blocked
bd update reckon-new --claim          # Switch to new issue
```

## Additional Resources

- **README.md** - User-facing product documentation
- **QUICKSTART.md** - Quick reference guide
- **docs/bd-usage.md** - Full beads workflow
- **docs/ASYNC_PATTERNS.md** - TUI async patterns (critical!)
- **docs/reckon-plan_2025-12-22.md** - Original architectural vision
- **docs/2026-01-17-review-and-assessment.md** - Technical review and architecture analysis
- **Subsystem guides** - See links above for TUI, Journal, CLI, Storage details

## Need Help?

- **Beads commands:** `bd help` or `bd <command> --help`
- **CLI commands:** `rk --help` or `rk <command> --help`
- **Unclear issue:** `bd show <id>` for details, or ask for clarification
- **Codebase questions:** Read relevant subsystem AGENTS.md file
- **Architecture questions:** See docs/ directory

---

**Remember:** This file is for orientation and workflow. For subsystem-specific details (TUI patterns, file formats, database schema, CLI conventions), see the linked guides above.
