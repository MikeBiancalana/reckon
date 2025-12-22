# Reckon: CLI Productivity System

## Project Overview

A terminal-based productivity tool combining daily journaling, task management, and a zettelkasten-style knowledge base. Built in Go with a TUI interface.

> **Name**: "Reckon" — a reckoning of time, and for oneself. CLI shorthand: `rk`

### Core Principles

- **KISS**: Start minimal, add complexity only when needed
- **YAGNI**: Each phase delivers usable value; defer features until required
- **Plain text as source of truth**: Markdown files can be copied anywhere; SQLite is a derived, rebuildable index
- **Unix philosophy**: Do one (complex) thing well; integrate with external tools (LLMs, scripts) via simple interfaces

---

## Architecture

### Language & Framework

- **Go** with [Bubble Tea](https://github.com/charmbracelet/bubbletea) for TUI
- Single binary with both TUI and CLI subcommands
- Future extensibility: Go's HTTP stdlib enables a web view later; consider [Wails](https://wails.io/) or similar for potential desktop/mobile if needed

### Storage Model

```
~/.reckon/
├── journal/
│   └── 2025-01-15.md      # One file per day
├── tasks/
│   └── <uuid>.md          # One file per task (full history)
├── notes/
│   └── <slug>.md          # Zettelkasten cards
└── reckon.db              # SQLite index (rebuildable)
```

**Rebuild guarantee**: The SQLite database is always derivable from the Markdown files. A `flowlog rebuild` command scans all files and reconstructs the index. Files are the canonical source.

### File Formats

#### Daily Journal (`journal/2025-01-15.md`)

```markdown
---
date: 2025-01-15
---

## Intentions

- [ ] Review PR #1234 for Alice
- [ ] Read requirements doc for Project X
- [>] Finish API refactor (carried from 2025-01-14)

## Wins

- Shipped the authentication fix
- Had a good 1:1 with manager

## Log

- 09:00 Started day, coffee
- 09:15 [task:abc123] Picked up API refactor
- 10:30 [meeting:standup] 30m
- 11:00 [task:abc123] Back to refactor, found edge case
- 12:00 [break] 45m lunch
- 14:00 General research on OAuth flows
```

**Conventions**:
- `[ ]` = open intention, `[x]` = done, `[>]` = carried from previous day
- `[task:<id>]` links a log entry to a task
- `[meeting:<name>]` and `[break]` are time-tracked events with optional duration

#### Task File (`tasks/<uuid>.md`)

```markdown
---
id: abc123
title: API refactor for v2 endpoints
created: 2025-01-10
status: active
tags: [backend, api]
---

## Description

Refactor the v2 endpoints to use the new middleware pattern.

## Log

### 2025-01-10

- 14:00 Created task, outlined approach
- 15:30 Started work on /users endpoint

### 2025-01-14

- 09:00 Resumed, hit auth issue
- 11:00 Paused for meeting

### 2025-01-15

- 09:15 Picked back up
- 11:00 Found edge case in token refresh
```

**Task statuses**: `active`, `done`, `waiting`, `someday`

#### Zettelkasten Note (`notes/<slug>.md`)

```markdown
---
title: Git interactive rebase workflow
created: 2025-01-12
tags: [git, workflow]
---

To squash the last 3 commits:

    git rebase -i HEAD~3

Change `pick` to `squash` (or `s`) for commits to combine.

## See Also

- [[git-bisect-workflow]]
```

---

## Implementation Phases

### Phase 1: Daily Journal (MVP)

**Goal**: Replace current Logseq daily workflow with a functional TUI.

#### Features

1. **Journal view**: Display today's journal with three sections (Intentions, Wins, Log)
2. **Quick log append**: Single-pane text input that appends timestamped entries to Log section
3. **Intention management**: Add/toggle/carry intentions
4. **Day navigation**: View previous days (read-only for now)
5. **CLI commands** for script integration:
   - `flowlog log "message"` — append to today's log
   - `flowlog today` — output today's journal to stdout (for LLM summarization)
   - `flowlog week` — output last 7 days to stdout

#### Data Model (SQLite)

```sql
CREATE TABLE journals (
    date TEXT PRIMARY KEY,
    file_path TEXT NOT NULL,
    last_modified INTEGER
);

CREATE TABLE intentions (
    id TEXT PRIMARY KEY,
    journal_date TEXT,
    text TEXT,
    status TEXT, -- 'open', 'done', 'carried'
    carried_from TEXT,
    FOREIGN KEY (journal_date) REFERENCES journals(date)
);

CREATE TABLE log_entries (
    id TEXT PRIMARY KEY,
    journal_date TEXT,
    timestamp TEXT,
    content TEXT,
    task_id TEXT, -- nullable, for task-linked entries
    entry_type TEXT, -- 'log', 'meeting', 'break'
    duration_minutes INTEGER,
    FOREIGN KEY (journal_date) REFERENCES journals(date)
);
```

#### Implementation Tasks

1. Set up Go project structure with Bubble Tea
2. Define Markdown file format and parser (use goldmark or similar)
3. Implement file watcher to keep SQLite in sync
4. Build journal view component (read-only display)
5. Build log input component (append mode)
6. Build intention list component (add/toggle)
7. Implement carry-over logic for new day
8. Add CLI subcommands (`log`, `today`, `week`)
9. Add `rebuild` command to regenerate SQLite from files

---

### Phase 2: Task Management

**Goal**: Support multi-day tasks with their own log history.

#### Features

1. **Task creation**: From intention or standalone
2. **Task picker**: Fuzzy-searchable list to select active task
3. **Two-pane view**: Daily log on left, current task on right
4. **Task log append**: Entries go to both task file AND daily journal (with `[task:id]` prefix)
5. **Task status transitions**: active → done/waiting/someday
6. **Task review view**: List tasks by status, filter by tag

#### Data Model Additions

```sql
CREATE TABLE tasks (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    status TEXT DEFAULT 'active',
    created TEXT,
    file_path TEXT
);

CREATE TABLE task_tags (
    task_id TEXT,
    tag TEXT,
    PRIMARY KEY (task_id, tag)
);

CREATE TABLE task_log_entries (
    id TEXT PRIMARY KEY,
    task_id TEXT,
    date TEXT,
    timestamp TEXT,
    content TEXT,
    FOREIGN KEY (task_id) REFERENCES tasks(id)
);
```

#### Implementation Tasks

1. Define task file format and parser
2. Build task creation flow (from TUI and CLI)
3. Build task picker component with fuzzy search
4. Implement two-pane layout with tab switching
5. Implement dual-write logic (task file + journal)
6. Build task list/review view
7. Add CLI: `rk task new "title"`, `rk task list`, `rk task log <id> "message"`

---

### Phase 3: Time Tracking

**Goal**: Track meetings, breaks, and optionally task time.

#### Features

1. **Meeting/break entry**: `[meeting:standup] 30m` or `[break] 45m`
2. **Optional task timing**: Start/stop on tasks (stored as log entries with timestamps)
3. **Duration calculation**: Compute time spent per task/meeting for a given day
4. **Daily summary**: Show time breakdown at end of day

#### Data Model

Uses existing `log_entries` table with `entry_type` and `duration_minutes` fields.

#### Implementation Tasks

1. Extend log entry parser to extract duration
2. Build time summary component
3. Add optional start/stop commands for tasks
4. Add CLI: `rk summary` (outputs time breakdown for today)

---

### Phase 4: Zettelkasten Notes

**Goal**: Capture and search knowledge snippets.

#### Features

1. **Note creation**: Quick capture with title and tags
2. **Note search**: Full-text search via SQLite FTS5
3. **Tag filtering**: Browse notes by tag
4. **Link syntax**: `[[slug]]` for internal links (display only, no graph)

#### Data Model Additions

```sql
CREATE VIRTUAL TABLE notes_fts USING fts5(
    title,
    content,
    tags
);

CREATE TABLE notes (
    slug TEXT PRIMARY KEY,
    title TEXT,
    created TEXT,
    file_path TEXT
);

CREATE TABLE note_tags (
    slug TEXT,
    tag TEXT,
    PRIMARY KEY (slug, tag)
);
```

#### Implementation Tasks

1. Define note file format
2. Build note creation flow
3. Implement FTS5 indexing on rebuild
4. Build search view with fuzzy matching
5. Build tag browser
6. Add CLI: `rk note new "title"`, `rk note search "query"`

---

### Phase 5: Periodic Review

**Goal**: Surface stale tasks and prompt for triage.

#### Features

1. **Review mode**: TUI view showing tasks not touched in N days
2. **Quick triage**: Mark as done, defer to someday, add note, or delete
3. **Weekly review prompt**: Optional reminder on first launch of the week

#### Implementation Tasks

1. Query for stale tasks (no log entries in X days)
2. Build review TUI with batch actions
3. Implement weekly prompt logic (store last review date)
4. Add CLI: `rk review`

---

## CLI Command Summary

| Command | Description |
|---------|-------------|
| `rk` | Launch TUI |
| `rk log "msg"` | Append to today's journal log |
| `rk today` | Output today's journal to stdout |
| `rk week` | Output last 7 days to stdout |
| `rk task new "title"` | Create a new task |
| `rk task list` | List active tasks |
| `rk task log <id> "msg"` | Append to a task's log |
| `rk note new "title"` | Create a new note |
| `rk note search "q"` | Search notes |
| `rk summary` | Time breakdown for today |
| `rk review` | Launch review mode |
| `rk rebuild` | Regenerate SQLite from files |

---

## Non-Goals (For Now)

- Mobile app (defer until core is stable)
- Web view (defer; architecture supports it later)
- Bidirectional linking / graph visualization
- Sync (use scp/syncthing on the Markdown files)
- Logseq import (nice-to-have; can be a separate script)
- Encryption (use OS-level encryption if needed)

---

## Open Questions for Implementation

1. **File locking**: If CLI and TUI both write, need a strategy (SQLite handles its own locking; for files, consider advisory locks or single-writer model)
2. **Timestamp format**: ISO 8601 (`2025-01-15T09:00:00`) or simple `09:00`? Suggest simple for display, ISO in frontmatter.
3. **ID generation**: UUIDs for tasks? Or shorter nanoid-style slugs for ergonomics?
4. **Carry-over UX**: Automatic on new day, or prompt user to review yesterday's open items?

---

## Success Criteria

Phase 1 is complete when:
- [ ] User can launch TUI and see today's journal
- [ ] User can add intentions and toggle them done
- [ ] User can append log entries via TUI
- [ ] `rk log "msg"` works from terminal scripts
- [ ] `rk today` outputs the journal for LLM consumption
- [ ] Closing and reopening preserves all data
- [ ] `rk rebuild` regenerates the database from Markdown files
