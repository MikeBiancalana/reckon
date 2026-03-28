# Implementation Plan: reckon-obed
## Simplify TUI task list: replace list.Model with []Task + selectedIndex

---

## 1. Summary of Approach

The key insight is that `list.Model` from Bubble Tea introduces indirection and ghost-selection bugs because its internal cursor state diverges from what `renderTaskSection` actually renders. The fix is to replace the `list.Model`-backed `TaskList` component with two plain fields (`tasks []journal.Task` and `selectedIndex int`) directly on `Model`. This eliminates the entire class of bugs where the list.Model cursor lands on invisible note items or drifts out of sync with the rendered view.

The implementation follows a coexistence strategy: new fields and rendering functions are added alongside the old ones, then callers are swapped over, and finally the old code is deleted. Each step keeps `go build ./...` passing.

---

## 2. Files to Modify

**New files:**
- `internal/tui/task_sort.go` — `SortTasksByPriority` function (pure logic, no component dependency)
- `internal/tui/task_sort_test.go` — unit tests for priority sorting

**Modified files:**
- `internal/tui/model.go` — Add `tasks`/`selectedIndex`/`taskScrollOffset` fields; add `renderTaskList`/`renderDetailArea`/`selectedTask()`; remove `taskList`, `selectedTaskID`, `notesPaneVisible`, `detailPanePosition`, `clearDateMode`, `clearDateTargetTask`; remove `SectionNotes` (SectionCount → 2); remove `renderTasksWithDetailPane`, `renderDetailPane`, `renderTaskSection`, `DetailPanePosition`, `getTaskSection`, `calculateDetailPanePosition`, `updateNotesForSelectedTask`, `updateLinksForSelectedItem`; simplify `renderNewLayout`; remove notesPane from layout
- `internal/tui/keyboard.go` — Wire j/k to `selectedIndex`; remove `handleDeleteTask` (task deletion stays but reads `m.selectedTask()`); remove `handleClearDate`/`handleClearDateKeys`; remove `handleToggleNotesPane`/`N` keybinding; remove enter-to-expand for tasks; remove `c` keybinding; update `handleSpaceKey`, `handleAddNote`, `handleEditTask`, `handleScheduleTask`, `handleSetDeadline`; remove `SectionNotes` from `handleComponentKeys`
- `internal/tui/handlers.go` — Update `handleTasksLoaded` to set `m.tasks = SortTasksByPriority(msg.tasks)` and clamp `m.selectedIndex`; remove `handleTaskSelectionChanged`, `handleLinksLoaded`, `handleLinkSelected`
- `internal/tui/model.go` (Update function) — Remove `TaskSelectionChangedMsg`, `linksLoadedMsg`, `LinkSelectedMsg` cases
- `internal/tui/layout.go` — Remove `TaskSectionDimensions`, `CalculateTaskSectionDimensions`; simplify `CalculatePaneDimensions` (remove `notesPaneVisible` parameter)
- `internal/tui/commands.go` — Remove `loadLinksForNote`
- `internal/tui/components/task_list.go` — Delete `TaskList`, `TimeGroupedTaskList`, `TaskItem`, `TaskDelegate`, `buildTaskItems`, `GroupTasksByTime`, `TimeGroupedTasks`, `findNoteText`, `taskListTitleStyle`, `focusedTaskListTitleStyle`, `noteStyle`. **Keep**: `parseDate`, `formatDateInfo`, `formatFriendlyDate`, `getDateStyle`, `TaskToggleMsg`, `TaskNoteDeleteMsg`, style vars for dates (`overdueStyle`, `dueTodayStyle`, `dueSoonStyle`, `scheduledStyle`, `dateInfoStyle`, `taskStyle`, `taskDoneStyle`)
- `internal/tui/components/task_list_test.go` — Delete tests for removed types; keep tests for surviving helpers
- `internal/tui/components/task_list_example_test.go` — Delete entirely
- `internal/tui/model_selection_test.go` — Delete entirely
- `internal/tui/model_test.go` — Fix references to removed fields
- `internal/tui/layout_test.go` — Remove `CalculateTaskSectionDimensions` tests; update `CalculatePaneDimensions` signature
- `internal/tui/layout_example_test.go` — Update if references removed types

---

## 3. Design Decisions

**D1: Where to put `SortTasksByPriority`**
Chosen: New file `internal/tui/task_sort.go` in `tui` package. It is TUI-specific visual ordering logic, not domain logic. Keeps it separate from components/ which is being simplified.

**D2: Priority bucket scheme**
- Bucket 0: Overdue deadline (`deadline < today`)
- Bucket 1: Due/scheduled today (`deadline == today` OR `scheduled <= today`)
- Bucket 2: Due/scheduled this week (`deadline` or `scheduled` within current week)
- Bucket 3: Everything else
- Within each bucket: stable sort by `CreatedAt` ascending (oldest first)
- Done tasks filtered out before sorting

**D3: Remove `notesPane` entirely**
The ticket removes `SectionNotes` and the `N` keybinding. Leaving a dangling unused `notesPane` field is dead code. Clean removal is better.

**D4: Scroll offset for `renderTaskList`**
Add `taskScrollOffset int` to Model. The render function shows tasks from `taskScrollOffset` to `taskScrollOffset + visibleHeight`. Offset adjusts when `selectedIndex` moves outside the visible window.

**D5: `renderDetailArea` vertical split**
Simple bottom section of the tasks pane showing notes for `m.tasks[m.selectedIndex]`. If no notes, show "No notes" placeholder. Proportional split: top portion task list, bottom portion detail area.

**D6: Task note deletion via `d`**
The inline note deletion path (`IsSelectedItemNote()`) is removed since notes are no longer inline items. Task note deletion from the detail area is a follow-on ticket (explicitly out of scope). The `d` key in tasks section is removed per the ticket.

---

## 4. Test Scenarios (write BEFORE implementation — Phase 3)

### `internal/tui/task_sort_test.go`
1. Empty slice → empty slice
2. Single task → that task
3. Overdue deadline sorts before today's tasks
4. Today's deadline → bucket 1
5. Past scheduled date → bucket 1
6. This-week deadline → bucket 2
7. This-week scheduled → bucket 2
8. Unscheduled task → bucket 3
9. Within same bucket, older CreatedAt first (stable sort)
10. Done tasks excluded from result
11. Task with both schedule and deadline uses higher-priority bucket
12. Multiple tasks across all buckets — correct final order

### `internal/tui/model_test.go` (new tests)
1. `TestRenderTaskList_Empty` — renders "No tasks" message, no panic
2. `TestRenderTaskList_SelectedHighlighted` — selectedIndex task gets SelectedStyle
3. `TestRenderTaskList_ScrollOffset` — when selectedIndex beyond visible area, scroll adjusts
4. `TestRenderDetailArea_NoSelection` — empty tasks → "No task selected"
5. `TestRenderDetailArea_TaskWithNotes` — renders note texts for selected task
6. `TestRenderDetailArea_TaskWithoutNotes` — shows "No notes" placeholder
7. `TestSectionNavigation_TwoSections` — tab cycles Logs↔Tasks only (SectionCount=2)

---

## 5. Implementation Order

Each step must leave `go build ./...` passing.

### Step 1 — Add `SortTasksByPriority` with tests
Create `internal/tui/task_sort.go` and `internal/tui/task_sort_test.go`. Pure new code, nothing existing modified.

### Step 2 — Add new fields to Model (coexist with old)
Add `tasks []journal.Task`, `selectedIndex int`, `taskScrollOffset int` to Model struct. No callers yet.

### Step 3 — Implement render methods and `selectedTask()` helper (dead code)
Add `renderTaskList(width, height int) string`, `renderDetailArea(width, height int) string`, `selectedTask() *journal.Task` to `model.go`. Not called yet.

### Step 4 — Wire `handleTasksLoaded` to populate new fields
After existing `m.taskList.UpdateTasks(msg.tasks)`, also set:
```go
m.tasks = SortTasksByPriority(msg.tasks)
m.selectedIndex = clampIndex(m.selectedIndex, len(m.tasks))
```
Both old and new fields populated. Build passes.

### Step 5 — Swap rendering in `renderNewLayout`
Replace `m.renderTasksWithDetailPane()` with a vertical join of `m.renderTaskList(...)` and `m.renderDetailArea(...)`. Remove `notesPaneVisible` branching from `renderNewLayout`. Update `CalculatePaneDimensions` to remove `notesPaneVisible` param; update all callers. Old render functions still present but unused.

### Step 6 — Wire keyboard navigation to `selectedIndex`
- j/k in `handleComponentKeys` for `SectionTasks`: adjust `m.selectedIndex` and clamp (remove `m.taskList.Update(msg)`)
- `handleSpaceKey`: read `m.selectedTask()`, emit `TaskToggleMsg` directly
- `handleEnterKey` for `SectionTasks`: remove (enter does nothing in tasks now)
- `handleAddNote`: replace `m.taskList.SelectedTask()` with `m.selectedTask()`
- `handleEditTask`, `handleScheduleTask`, `handleSetDeadline`: same
- `handleDelete` for `SectionTasks`: simplify using `m.selectedTask()`; remove inline note deletion path
- Remove `handleDeleteTask` function
- Remove `handleClearDate`, `handleClearDateKeys`, `c` keybinding
- Remove `N` keybinding, `handleToggleNotesPane`

### Step 7 — Remove TaskSelectionChangedMsg and links plumbing
Remove from `Update()`: `TaskSelectionChangedMsg`, `linksLoadedMsg`, `LinkSelectedMsg` cases.
Remove: `handleTaskSelectionChanged`, `handleLinksLoaded`, `handleLinkSelected`, `updateNotesForSelectedTask`, `updateLinksForSelectedItem`, `loadLinksForNote`.

### Step 8 — Remove old rendering infrastructure
Delete: `renderTasksWithDetailPane`, `renderDetailPane`, `renderTaskSection`.
Delete from layout.go: `TaskSectionDimensions`, `CalculateTaskSectionDimensions`.
Delete from model.go: `DetailPanePosition`, `getTaskSection`, `calculateDetailPanePosition`.

### Step 9 — Remove SectionNotes and notes pane plumbing
- Remove `SectionNotes` from enum; `SectionCount` → 2
- Remove `SectionNameNotes`, its case in `sectionName()`
- Remove `SectionNotes` from `handleEnterKey` and `handleComponentKeys`
- Remove `notesPane` from Model struct and `NewModel`
- Remove `notesPane` from `renderNewLayout`
- Remove `selectedTaskID`, `notesPaneVisible`, `detailPanePosition` from Model struct
- Remove `clearDateMode`, `clearDateTargetTask` from Model struct
- Remove `clearDateMode` check from `View()`

### Step 10 — Delete TaskList component infrastructure
- Delete from `task_list.go`: `TaskList`, `NewTaskList`, all `TaskList` methods, `TimeGroupedTaskList`, all its methods, `TaskItem`, `TaskDelegate`, `buildTaskItems`, `GroupTasksByTime`, `TimeGroupedTasks`, `findNoteText`, `taskListTitleStyle`, `focusedTaskListTitleStyle`, `noteStyle`
- Remove `taskList` field from Model struct
- Remove `m.taskList` initialization from `handleJournalLoaded`

### Step 11 — Delete old tests, rewrite affected tests
- Delete `internal/tui/model_selection_test.go`
- Delete `internal/tui/components/task_list_example_test.go`
- Rewrite `internal/tui/components/task_list_test.go` for surviving helpers only
- Update `internal/tui/model_test.go`, `layout_test.go`, `layout_example_test.go`
- `go build ./...` and `go test ./...` pass

### Step 12 — Final cleanup
- `go vet ./...`
- Dead-code grep: `taskList`, `TaskList`, `notesPaneVisible`, `detailPanePosition`, `SectionNotes`, `clearDateMode`, `GroupTasksByTime`, `TimeGroupedTaskList`, `TaskItem`, `TaskDelegate`, `buildTaskItems`, `handleClearDate`, `handleToggleNotesPane`
- Fix any remaining references

---

## 6. Known Risks and Mitigations

| Risk | Mitigation |
|------|-----------|
| `CalculatePaneDimensions` callers in tests reference old signature | Update signature and callers in Step 5 simultaneously |
| `SortTasksByPriority` bucket boundaries differ from `GroupTasksByTime` | Copy identical date boundary logic; test edge cases |
| Section navigation modulo breaks with SectionCount change | `SectionCount` already used in modulo — changing to 2 automatically fixes it |
| `clearDateMode` check in `View()` removed but state field still referenced | Remove both the check and field together in Step 9 |
| Stale `m.taskList` reference after Step 10 | Step 10 removes the field — compiler catches any remaining refs |

---

## 7. Definition of Done

1. `go build ./...` passes with zero errors
2. `go test ./...` passes with zero failures
3. `go vet ./...` produces no warnings
4. No references to: `TaskList`, `TimeGroupedTaskList`, `TaskItem`, `TaskDelegate`, `buildTaskItems`, `GroupTasksByTime`, `notesPaneVisible`, `detailPanePosition`, `SectionNotes`, `clearDateMode`, `handleDeleteTask`, `handleClearDate`, `handleClearDateKeys`, `handleToggleNotesPane`, `TaskSelectionChangedMsg`, `linksLoadedMsg`, `handleLinksLoaded`, `handleLinkSelected`, `updateLinksForSelectedItem`, `loadLinksForNote`
5. Model struct has `tasks []journal.Task` and `selectedIndex int` (no `taskList`)
6. `Section` enum has exactly 2 values: `SectionLogs` (0), `SectionTasks` (1); `SectionCount` = 2
7. j/k moves `selectedIndex` with bounds clamping
8. Space toggles task at `selectedIndex`
9. `n`/`e`/`s`/`D` all read `m.selectedTask()` for their task context
10. `SortTasksByPriority` has comprehensive unit tests
11. `renderTaskList` and `renderDetailArea` have unit tests
12. Scroll works when `selectedIndex` is outside visible area
