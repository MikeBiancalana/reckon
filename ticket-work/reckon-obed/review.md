# Code Review: reckon-obed -- Ghost-selection bug fix

**Reviewer:** Claude (Opus 4.6)
**Date:** 2026-03-28

## Verdict: APPROVE WITH CHANGES

The simplification is well-motivated and well-executed. Replacing the `list.Model` wrapper with plain `[]journal.Task` + `selectedIndex` eliminates the root cause of the ghost-selection bug (cursor/view desync in the Bubble Tea list component). The new code is dramatically simpler, the sort logic is clean and well-tested, and all tests pass. Two issues need attention before merge.

---

## Critical Issues

### 1. `handleTasksLoaded` does not preserve selection identity across reloads

**File:** `internal/tui/handlers.go`, line 81

```go
m.tasks = SortTasksByPriority(msg.tasks, stdtime.Now())
m.selectedIndex = clampIndex(m.selectedIndex, len(m.tasks))
```

When tasks are reloaded (after toggle, add, note, schedule, deadline), the sort order may change -- a toggled task gets filtered out, a newly scheduled task moves buckets, etc. `clampIndex` preserves the *numeric* index but not the *identity* of the selected task. This means:

- If you toggle the 3rd task done, it disappears from the list, and the cursor silently jumps to whatever task is now at index 2. This is acceptable but worth noting.
- If you schedule a task and it moves from bucket 3 to bucket 1, the cursor stays at the old numeric position, pointing at a different task. The user loses track of what they just acted on.

**Recommendation:** Before sorting, capture the selected task's ID. After sorting, scan for that ID and update `selectedIndex` to its new position. Fall back to `clampIndex` if not found (task was deleted or filtered). This is a small change:

```go
var selectedID string
if m.selectedIndex >= 0 && m.selectedIndex < len(m.tasks) {
    selectedID = m.tasks[m.selectedIndex].ID
}
m.tasks = SortTasksByPriority(msg.tasks, stdtime.Now())
m.selectedIndex = clampIndex(m.selectedIndex, len(m.tasks))
if selectedID != "" {
    for i, t := range m.tasks {
        if t.ID == selectedID {
            m.selectedIndex = i
            break
        }
    }
}
```

**Severity:** Medium-high. This directly affects the user experience for the primary use case (acting on tasks). Without this fix, the PR trades one selection-desync bug for a milder but still jarring one.

### 2. `taskScrollOffset` is never reset when task list changes

**File:** `internal/tui/model.go`, `renderTaskList` (line 577) and `handlers.go` (line 77)

When `handleTasksLoaded` replaces `m.tasks`, the `taskScrollOffset` is not reset or adjusted. If the user was scrolled down in a long list and then tasks are reloaded with fewer items, the offset could leave the viewport pointing past the end of the list. The scroll-follow-cursor logic in `renderTaskList` will correct this *if* `selectedIndex` is in range, but if `selectedIndex` is -1 (empty list), the scroll offset is never corrected and the render loop silently produces zero lines (not a crash, but a blank pane that recovers only when the user navigates).

**Recommendation:** In `handleTasksLoaded`, after setting `selectedIndex`, also clamp `taskScrollOffset`:

```go
if m.taskScrollOffset > len(m.tasks)-1 {
    m.taskScrollOffset = max(0, len(m.tasks)-1)
}
```

**Severity:** Low-medium. Edge case (requires having scrolled down, then tasks shrink). No crash, just a momentarily blank pane.

---

## Non-Critical Suggestions

### 1. Week boundary calculation uses Monday-relative arithmetic that may confuse on Sunday

**File:** `internal/tui/task_sort.go`, lines 23-28

```go
weekday := now.Weekday()
if weekday == stdtime.Sunday {
    weekday = 7
}
weekStart := today.AddDate(0, 0, -int(weekday-stdtime.Monday))
weekEnd := weekStart.AddDate(0, 0, 7)
```

The Sunday special-casing (mapping it to 7) combined with the `weekday - Monday` arithmetic works correctly, but it is subtle. A short comment explaining that Go's `time.Sunday == 0` and the code maps it to 7 so Monday-based arithmetic works would help future readers. The test suite covers a Monday fixture but does not test a Sunday or Saturday boundary.

**Suggestion:** Add a test case with `now` set to a Sunday to validate the week boundary.

### 2. Duplicate `parseDate` / `parseTaskDate` functions

**Files:** `internal/tui/task_sort.go:98` (`parseTaskDate`) and `internal/tui/components/task_list.go:121` (`parseDate`)

These are identical implementations in two packages. This is a minor DRY violation. Consider extracting to a shared location (e.g., `internal/journal` or a `dateutil` package) if the duplication grows, but it is acceptable as-is for two call sites.

### 3. Duplicate style definitions

**File:** `internal/tui/model.go` lines 594-599 re-declare `taskNormalStyle` and `taskDoneStyle` that are already defined in `internal/tui/components/task_list.go` lines 12-16 as `taskStyle` and `taskDoneStyle`. Consider reusing the exported component styles rather than duplicating them inline.

### 4. `renderDetailArea` note truncation loses header

**File:** `internal/tui/model.go`, lines 663-665

```go
if len(lines) > height {
    lines = lines[:height]
}
```

The header "NOTES" is `lines[0]`. If `height == 1`, the user sees only the header and no notes. If `height == 0` (impossible given the `detailHeight >= 3` guard in `renderNewLayout`), lines would be empty. This is fine given the guard, but worth noting that the header consumes one of the `height` lines, so effective note capacity is `height - 1`.

### 5. `handleDelete` does not support task deletion from the Tasks section

**File:** `internal/tui/keyboard.go`, lines 352-364

The `handleDelete` function only handles `SectionLogs`. There is no `case SectionTasks:` branch. If the user presses `d` while focused on the Tasks section, nothing happens. This appears to be a pre-existing limitation, not a regression from this PR, but worth noting since the `confirmItemType` handling in `deleteItem()` (commands.go:158) does support `"task"` deletion.

### 6. Duplicate comment on line 522-523 of model.go

**File:** `internal/tui/model.go`, lines 522-523

```go
// New journal task messages
// New journal task messages
```

Trivial copy-paste duplication in a comment.

### 7. `handleTaskScheduled` / `handleTaskDeadlineSet` re-parse an already-formatted date

**File:** `internal/tui/handlers.go`, lines 214-217 and 224-227

```go
parsedDate, _ := components.ParseRelativeDate(msg.date)
friendlyDate := parsedDate.Format("Jan 2")
```

`msg.date` is already in `"2006-01-02"` format (set in `handleDatePickerKeys`). Passing a `YYYY-MM-DD` string to `ParseRelativeDate` (which handles relative strings like "tomorrow") works but is semantically misleading. A plain `time.Parse("2006-01-02", msg.date)` would be clearer and safer (avoids relying on `ParseRelativeDate` accepting absolute dates). The `_` also silently ignores parse errors.

---

## Positive Observations

- **Excellent simplification.** Removing ~800 lines of `task_list.go` component wrapper and replacing it with ~90 lines of direct rendering is a textbook KISS improvement. The old `list.Model` abstraction was the source of the bug, and removing it entirely rather than patching it is the right call.

- **Clean sort implementation.** `SortTasksByPriority` with explicit bucket assignment and `sort.SliceStable` is clear, correct, and testable. The bucket function reads like a specification.

- **Good test coverage for the sort logic.** 12 test cases covering empty input, done-task exclusion, all four buckets, cross-bucket ordering, stability, and the deadline+schedule interaction. Well structured.

- **Proper async closure capture.** The `handleSpaceKey` method correctly captures `task.ID` before the closure, following the documented pattern. The extensive documentation of this pattern in `model.go` is valuable.

- **Bounds checking is thorough.** `selectedTask()` guards against out-of-bounds access. `clampIndex` is well-tested with edge cases including empty lists and negative indices.

---

## Summary

This is a clean, well-motivated simplification that eliminates a real bug by removing its root cause. The code is readable, the sort logic is well-tested, and the architecture is simpler. The main gap is that selection identity is not preserved across task reloads (critical issue #1), which can cause a disorienting selection jump after any task mutation. The scroll offset issue (#2) is minor but easy to fix alongside #1. After addressing those two items, this is ready to merge.
