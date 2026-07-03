# Acceptance Criteria: reckon-obed
## Simplify TUI task list: replace list.Model with []Task + selectedIndex

---

## 1. Explicit Acceptance Criteria

1. **TaskList component removed** — All refs to `TaskList`, `TimeGroupedTaskList`, `list.Model`, `TaskItem`, `TaskDelegate`, `buildTaskItems` are deleted.

2. **Model has new fields** — `tasks []journal.Task` (open, priority-sorted) and `selectedIndex int` (cursor into displayed list).

3. **Priority sort implemented** — `SortTasksByPriority()` with:
   - Bucket 0: overdue deadline
   - Bucket 1: due today OR scheduled today/past
   - Bucket 2: due/scheduled this week
   - Bucket 3: everything else (by creation date)
   - Done tasks filtered out; stable sort within buckets.

4. **Notes in detail area** — Notes no longer interleave in the task list; shown in a fixed area below the list for the selected task.

5. **New render methods** — `renderTaskList(width, height)` and `renderDetailArea(width, height)` replace `renderTasksWithDetailPane`.

6. **Navigation works** — j/k moves `selectedIndex` through tasks only (no notes in navigation path).

7. **All kept operations work** — add task, edit task, toggle done (archive), add note, tags, schedule/deadline dates, date picker modal, date urgency indicators.

8. **Removed features gone** — clear date submenu (c), delete task from TUI (d), collapsible notes in task list, detail pane positioning logic, notes pane stub, `SectionNotes`.

9. **Tests updated** — `model_selection_test.go` deleted; `task_list_test.go` rewritten for surviving helpers; new tests for `SortTasksByPriority`, `renderTaskList`, `renderDetailArea`.

10. **No ghost-selection** — completing tasks, adding notes, reloading tasks never leaves `selectedIndex` pointing at an invisible item.

11. **`go build ./...` passes** at end of every implementation step.

12. **`go test ./...` passes** at completion.

---

## 2. Implicit Requirements

1. Existing `journal.Task` structs unchanged — structural change only.
2. `selectedIndex` clamped to valid range after any task list mutation.
3. Priority sort is stable — equal-priority tasks keep relative creation-date order.
4. Detail area updates immediately when `selectedIndex` changes.
5. No `list.Model` management anywhere in the new implementation.
6. All async closures capture `selectedIndex`/task ID by value at creation time (not reference).

---

## 3. Edge Cases

1. **Empty task list** — `selectedIndex = -1`; detail area shows placeholder; no crash on navigation.
2. **Down at last task** — `selectedIndex` stays at `len(tasks)-1`.
3. **Up at first task** — `selectedIndex` stays at 0.
4. **Task archived (current selection)** — `selectedIndex` clamped to `min(selectedIndex, len(tasks)-1)`, or -1 if empty.
5. **Task archived (earlier in list)** — `selectedIndex` decremented by 1 to track same visual row.
6. **Task added** — sort re-run; `selectedIndex` updated to track previously selected task by ID.
7. **Task's deadline changed** — may change bucket; `selectedIndex` tracks edited task.
8. **All tasks archived** — empty state, no crash.
9. **No notes** — detail area shows placeholder, no panic indexing empty `Notes` slice.
10. **Large task list** — scroll offset computed and applied (not just computed).

---

## 4. Test Scenarios (Given/When/Then)

### SortTasksByPriority

**S1: Overdue deadline → bucket 0**
- Given: task with DeadlineDate = yesterday
- When: SortTasksByPriority called
- Then: task appears before all non-overdue tasks

**S2: Due today → bucket 1**
- Given: task with DeadlineDate = today
- When: SortTasksByPriority called
- Then: task appears after overdue, before this-week tasks

**S3: Scheduled past → bucket 1**
- Given: task with ScheduledDate = 3 days ago, no deadline
- When: SortTasksByPriority called
- Then: task in bucket 1 (overdue scheduled)

**S4: Done tasks filtered**
- Given: 2 open tasks, 1 done task
- When: SortTasksByPriority called
- Then: result has 2 tasks only

**S5: Stable sort within bucket**
- Given: 3 bucket-3 tasks created at t1, t2, t3
- When: SortTasksByPriority called
- Then: order is t1, t2, t3

### renderTaskList

**S6: Empty list**
- Given: tasks = []
- When: renderTaskList called
- Then: returns non-empty string with "no tasks" message, no panic

**S7: Selection highlight**
- Given: 3 tasks, selectedIndex = 1
- When: renderTaskList called
- Then: second task line has selection styling, others do not

**S8: Scroll offset**
- Given: 20 tasks, height = 5, selectedIndex = 15
- When: renderTaskList called
- Then: selected task is visible in rendered output (scroll applied)

### renderDetailArea

**S9: No notes**
- Given: selected task has Notes = []
- When: renderDetailArea called
- Then: shows placeholder or empty state, no panic

**S10: Notes display**
- Given: selected task has 2 notes
- When: renderDetailArea called
- Then: both note texts appear in output

### Navigation

**S11: j at bottom**
- Given: 3 tasks, selectedIndex = 2
- When: 'j' pressed
- Then: selectedIndex still = 2

**S12: k at top**
- Given: 3 tasks, selectedIndex = 0
- When: 'k' pressed
- Then: selectedIndex still = 0

**S13: Archive selected**
- Given: 3 tasks, selectedIndex = 1
- When: space/toggle on task 1
- Then: tasks has 2 items, selectedIndex = 1 (now points to old task 2)

---

## 5. Out of Scope

- Task note deletion from detail area (follow-on ticket)
- Scroll indicator / scroll bar
- Task filtering or search
- Notes/wiki pane (separate initiative)
- New task fields or persistence changes
- Multi-selection
- Undo/redo
- Drag-and-drop reordering
- Performance tuning beyond removing list.Model
