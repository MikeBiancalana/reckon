# Implementation Plan: reckon-iqv5 — Add Time-Based Task Grouping in TUI

## 1. Summary of Approach

Introduce a `TaskGroup` struct and `GroupTasksByTime` function that partitions the sorted task list (from `SortTasksByPriority`) into four named sections: OVERDUE, TODAY, THIS WEEK, BACKLOG. The `renderTaskList` method iterates over groups rather than a flat list, rendering section headers as non-selectable separator lines with collapse/expand support for BACKLOG. Cursor navigation (`j`/`k`) operates only on task rows via a `visibleTasks` flattened slice; `selectedIndex` maps 1:1 to a task in `visibleTasks`.

## 2. Files to Modify

### A. internal/tui/task_sort.go — Add grouping function
Add `TaskGroup` type and `GroupTasksByTime(tasks []journal.Task, now time.Time) []TaskGroup`.
Partitions pre-sorted tasks by bucket (0=OVERDUE, 1=TODAY, 2=THIS WEEK, 3=BACKLOG). Empty groups are omitted.

### B. internal/tui/model.go — Add group state + rewrite renderTaskList
New fields on Model:
    taskGroups       []TaskGroup
    groupCollapsed   map[string]bool   // "BACKLOG": true by default
    visibleTasks     []journal.Task    // flattened from non-collapsed groups
    visibleTaskIndex map[string]int    // task ID -> index in visibleTasks

Initialize in NewModel: groupCollapsed: map[string]bool{"BACKLOG": true}

New methods:
    rebuildVisibleTasks() — flatten taskGroups, respect groupCollapsed, handle all-BACKLOG edge case
    selectedTask() — updated to use visibleTasks
    toggleBacklogCollapse() — flip collapse, rebuild, restore cursor by ID

Rewrite renderTaskList:
    - Iterate taskGroups, emit styled header lines + task lines
    - Headers: "▼ OVERDUE (2)" / "▶ BACKLOG (5)" with group-specific lipgloss styles
    - Scroll logic accounts for header lines consuming viewport space

### C. internal/tui/handlers.go — Update handleTasksLoaded
After SortTasksByPriority, call GroupTasksByTime + rebuildVisibleTasks.
Restore cursor by ID in visibleTasks. Clamp scroll offset.

### D. internal/tui/keyboard.go — Add BACKLOG toggle key + fix nav
Add case "b": m.toggleBacklogCollapse() in SectionTasks handler.
Update j/k navigation to use len(m.visibleTasks).
Update help text to document 'b' key.

### E. internal/tui/components/task_list.go — Add section header styles
Add SectionHeaderOverdueStyle (red 196), SectionHeaderTodayStyle (yellow 226),
SectionHeaderThisWeekStyle (blue 39), SectionHeaderBacklogStyle (gray 240).
Add SectionHeaderStyle(groupName string) lipgloss.Style helper.

## 3. New Types/Structs

// In task_sort.go
type TaskGroup struct {
    Name   string         // "OVERDUE", "TODAY", "THIS WEEK", "BACKLOG"
    Bucket int            // 0-3
    Tasks  []journal.Task
}

// Internal render type (local to renderTaskList)
type renderLine struct {
    text      string
    isHeader  bool
    taskIndex int  // -1 for headers, index into visibleTasks for task lines
}

## 4. Design Decisions

D1: Cursor navigates only tasks (not section headers)
    - selectedIndex maps 1:1 to visibleTasks; no guard clauses needed in action handlers
    - Alternative: cursor on headers (like LogView). Rejected: only BACKLOG is collapsible, complicates all handlers.

D2: Key 'b' toggles BACKLOG collapse
    - Mnemonic for "Backlog", currently unused in Tasks section focus
    - Space already used for task toggle

D3: Keep calendar-week definition (not "next 7 days")
    - Existing taskBucket uses Monday-Sunday; changing would break existing tests
    - If product owner wants "next 7 days" change is isolated to taskBucket()

D4: rebuildVisibleTasks forces BACKLOG open when it's the only group
    - Checked each time rebuildVisibleTasks runs

D5: Timezone — fix parseTaskDate to use now.Location()
    - Per REVIEW_PATTERNS.md (reckon-gcuu): time.Parse returns UTC, causing bugs
    - Apply fix in task_sort.go; task_list.go is a follow-up

## 5. Test Scenarios (from AC)

1. Basic grouping: 4 tasks → 4 groups, correct names
2. Toggle BACKLOG: visibleTasks length changes on toggle
3. Missing due date → BACKLOG
4. All-BACKLOG: BACKLOG forced open
5. Cursor in collapsed BACKLOG → moves to last visible task
6. Scroll offset clamped after collapse
7. Empty groups omitted (only 2 groups returned when 2 buckets used)
8. Selection preserved by ID across reload

## 6. Known Risks

- "This week" definition: AC says "next 7 days" but code uses calendar-week. Plan keeps calendar-week.
- Scroll offset with mixed header/task lines: headers consume viewport space but aren't cursor targets. Trickiest rendering part.
- Timezone fix: changing parseTaskDate signature breaks callers within package. All callers in task_sort.go must be updated.

## 7. Implementation Order

1. Add TaskGroup type + GroupTasksByTime (task_sort.go) + tests
2. Add section header styles (components/task_list.go)
3. Add Model fields + rebuildVisibleTasks + selectedTask update (model.go)
4. Update handleTasksLoaded (handlers.go)
5. Rewrite renderTaskList (model.go)
6. Add toggleBacklogCollapse + wire 'b' key (keyboard.go + model.go)
7. Fix j/k navigation to use visibleTasks length (keyboard.go)
8. Write integration render tests
