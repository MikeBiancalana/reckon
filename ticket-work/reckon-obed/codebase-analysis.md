# Codebase Analysis: reckon-obed

## TUI Directory Structure

**Location:** `internal/tui/`

### Main Files:
- **model.go** (~26KB) — Core TUI state machine, implements Bubble Tea Model interface
- **handlers.go** — Keyboard & message handlers organized by event type
- **keyboard.go** — Keyboard input dispatch and routing
- **commands.go** — Async command generators (tea.Cmd pattern)
- **layout.go** — Pane dimension calculations
- **AGENTS.md** — Critical subsystem patterns

### Test Files:
- **model_test.go** — State transitions, message handling
- **model_selection_test.go** — Task selection rendering (to be DELETED per ticket)
- **handlers_test.go** — Handler logic
- **keyboard_test.go** — Keyboard routing
- **layout_test.go** — Pane dimension calculations

### Components Subdirectory:
- **task_list.go** (919 lines) — PRIMARY TARGET: TaskList wrapper around Bubble Tea's list.Model
- **task_list_test.go** — Tests for TaskList (to be rewritten)
- **task_list_example_test.go** — Example tests (to be deleted)
- intention_list.go, log_view.go, task_picker.go, wins_view.go, note_picker.go — Similar components (reference only)
- text_entry_bar.go, date_picker.go, form.go, notes_pane.go, status_bar.go, etc.

---

## Files Most Likely to Be Modified

### PRIMARY — internal/tui/components/task_list.go
- **Remove:** `list.Model`, `TaskItem`, `TaskDelegate`, `buildTaskItems`, `TimeGroupedTaskList`
- **Add:** `tasks []journal.Task`, `selectedIndex int`
- **Keep:** `parseDate`, `formatDateInfo`, `formatFriendlyDate`, `getDateStyle`, style vars, `TaskToggleMsg`
- Completely rewrite struct and methods

### PRIMARY — internal/tui/model.go
- **Add:** `tasks []journal.Task`, `selectedIndex int` fields to Model
- **Remove:** `taskList *components.TaskList` field, `selectedTaskID`, `notesPaneVisible`, `detailPanePosition`
- **Remove:** `renderTasksWithDetailPane`, `renderDetailPane`, `renderTaskSection`
- **Remove:** `DetailPanePosition`, `getTaskSection`, `calculateDetailPanePosition`, `TaskSectionDimensions`, `CalculateTaskSectionDimensions`
- **Add:** `renderTaskList(width, height)`, `renderDetailArea(width, height)`, `SortTasksByPriority()`
- **Delete:** `model_selection_test.go`

### PRIMARY — internal/tui/handlers.go
- Update `handleTasksLoaded` — populate `m.tasks` instead of `m.taskList.UpdateTasks()`
- **Remove:** `handleTaskSelectionChanged`, `updateNotesForSelectedTask`, `updateLinksForSelectedItem`
- **Remove:** `handleDeleteTask`, `handleClearDate`, `handleClearDateKeys`

### PRIMARY — internal/tui/keyboard.go
- **Remove:** `clearDateMode` handling, `c`/`d` keybindings for tasks, enter-to-expand for tasks
- **Remove:** `TaskSelectionChangedMsg` handling
- **Update:** j/k navigation to update `m.selectedIndex`
- **Update:** space/enter to use `m.tasks[m.selectedIndex]`
- **Update:** `handleAddNote`, `handleEditTask`, `handleScheduleTask`, `handleSetDeadline`

### SECONDARY — internal/tui/components/task_list_test.go
- Rewrite for surviving helpers (`parseDate`, `formatDateInfo`, etc.)
- **Delete:** `task_list_example_test.go`

---

## Key Types and Signatures

### journal.Task (DO NOT MODIFY)
```go
type Task struct {
    ID            string      `json:"id"`
    Text          string      `json:"text"`
    Status        TaskStatus  `json:"status"`  // "open" or "done"
    Tags          []string    `json:"tags"`
    Notes         []TaskNote  `json:"notes"`
    Position      int         `json:"position"`
    CreatedAt     time.Time   `json:"created_at"`
    ScheduledDate *string     `json:"scheduled_date,omitempty"` // YYYY-MM-DD
    DeadlineDate  *string     `json:"deadline_date,omitempty"`  // YYYY-MM-DD
}
```

### TaskList Public API (PRESERVE — callers in model.go/keyboard.go)
```go
func NewTaskList() *TaskList
func (tl *TaskList) Update(msg tea.Msg) (*TaskList, tea.Cmd)
func (tl *TaskList) View() string
func (tl *TaskList) SetSize(width, height int)
func (tl *TaskList) SetFocused(focused bool)
func (tl *TaskList) SelectedTask() *journal.Task
func (tl *TaskList) IsSelectedItemNote() bool
func (tl *TaskList) UpdateTasks(tasks []journal.Task)
func (tl *TaskList) GetTasks() []journal.Task
```

### Message Types (PRESERVE ALL)
- `TaskToggleMsg{TaskID string}`
- `TaskSelectionChangedMsg{TaskID string}` — being REMOVED from Model handling
- `TaskNoteDeleteMsg{TaskID, NoteID string}`

---

## Current Ghost-Selection Bug

The bug mechanism:
1. Notes are separate `list.Item`s interspersed between tasks
2. `renderTaskSection()` uses `m.taskList.SelectedTask()` — searches task array by ID
3. `TaskList.View()` is NEVER called in render path (rendering uses `GetTasks()` + manual loop)
4. Cursor position in `list.Model` and visual selection diverge whenever notes are collapsed/expanded

**Fix:** `selectedIndex int` pointing into `[]journal.Task` (no notes interspersed) = single source of truth.

---

## Critical Patterns from AGENTS.md

### Async Closure Capture (CRITICAL)
```go
// WRONG
return func() tea.Msg { return m.currentJournal } // reference — can change

// RIGHT
captured := m.currentJournal
return func() tea.Msg { return captured } // value — safe
```

### Component Communication
Components emit messages → Model handles them. Keep all existing message types.

### Focus Management
```go
m.taskList.SetFocused(m.focusedSection == SectionTasks)
```

---

## Known Pitfalls from docs/REVIEW_PATTERNS.md

1. **Dead code after refactoring** — grep for all references after deletion
2. **Nil pointer before check** — check `len(m.tasks) > 0` before indexing
3. **Stale async data** — verify context matches when async task data arrives
4. **Computed offset not applied** — actually use scroll offset in rendering

---

## SortTasksByPriority Buckets

Per ticket description:
- Bucket 0: overdue deadline
- Bucket 1: due today OR scheduled today/past
- Bucket 2: due/scheduled this week
- Bucket 3: everything else

Filter out done tasks. Stable sort by CreatedAt within buckets.
