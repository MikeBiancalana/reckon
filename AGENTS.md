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
