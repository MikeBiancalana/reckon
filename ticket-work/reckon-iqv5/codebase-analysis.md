# Codebase Analysis: reckon-iqv5 — Add time-based task grouping in TUI

## Files Most Likely to Be Modified

Critical:
- internal/tui/model.go — renderTaskList() function; add groupTasksByUrgency()
- internal/tui/task_sort.go — existing SortTasksByPriority/taskBucket maps to the 4 sections
- internal/tui/components/task_list.go — task styling helpers (overdueStyle, dueTodayStyle etc.)
- internal/tui/keyboard.go — new key handlers for section collapse/expand

Secondary:
- internal/tui/handlers.go — task loading/state (already has ID-based selection restoration)

## Existing Patterns

A. Task Bucketing (task_sort.go): SortTasksByPriority() groups into 4 buckets exactly:
   Bucket 0 → OVERDUE (red), Bucket 1 → TODAY (yellow), Bucket 2 → THIS WEEK, Bucket 3 → BACKLOG

B. Collapse/Expand: LogView already has collapsedMap map[string]bool, indicators /,  Enter key toggle

C. Lipgloss Styles: overdueStyle (red #196), dueTodayStyle (yellow #226) already defined

D. ID-Based Selection Restoration: handlers.go:82-99 already implemented

## Key Types

type Task struct {
    ID            string
    Text          string
    Status        TaskStatus
    ScheduledDate *string  // "YYYY-MM-DD" or nil
    DeadlineDate  *string  // "YYYY-MM-DD" or nil
    CreatedAt     time.Time
}

func SortTasksByPriority(tasks []journal.Task, now time.Time) []journal.Task
func taskBucket(task journal.Task, today, weekStart, weekEnd time.Time) int

type Model struct {
    tasks            []journal.Task
    selectedIndex    int
    taskScrollOffset int
}

## Proposed New Structure

type TaskGroup struct {
    Name        string
    Tasks       []journal.Task
    Bucket      int
    IsCollapsed bool
}

Add to Model:
    taskGroups     []TaskGroup
    groupCollapsed map[string]bool  // "BACKLOG" → true by default

## Known Pitfalls

1. Timezone in date parsing — use now.Location() not UTC
2. Scroll offset clamping — clamp after any group change
3. Selection by index — preserve by task ID (already handled in handlers.go)
4. Phantom newlines — use strings.Join with conditionals
5. Nil collapsedMap — check before access
