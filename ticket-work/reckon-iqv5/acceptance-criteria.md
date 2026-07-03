# Acceptance Criteria: Add Time-Based Task Grouping in TUI (reckon-iqv5)

## 1. Explicit Acceptance Criteria

1. Task view is restructured into sections: OVERDUE (red), TODAY (yellow), THIS WEEK, BACKLOG
2. BACKLOG is auto-collapsed by default
3. Tasks within each section sorted by urgency
4. User can toggle BACKLOG expanded/collapsed state
5. Sections display in order: OVERDUE → TODAY → THIS WEEK → BACKLOG

## 2. Implicit Requirements

- Task re-grouping does not modify underlying task dates
- Cursor selection must track task identity (ID, not index) across re-sorts
- "Today" is calculated once at view-render time
- "This week" = within next 7 days from today

## 3. Edge Cases

- Task with null/missing due date → falls into BACKLOG
- All tasks in BACKLOG → BACKLOG should expand (not auto-collapse when it's the only content)
- Cursor in collapsed BACKLOG → move cursor to nearest visible task
- Scroll offset after reload → clamp to avoid rendering beyond list bounds

## 4. Test Scenarios

### Scenario 1: Basic Grouping
Given: Tasks due yesterday, today, this week, and next month
When: User views the task list
Then: Tasks are grouped into OVERDUE, TODAY, THIS WEEK, BACKLOG
And: BACKLOG is collapsed by default

### Scenario 2: Toggle BACKLOG Collapse
Given: BACKLOG is collapsed
When: User toggles BACKLOG
Then: BACKLOG expands showing all items

### Scenario 3: Missing Due Date
Given: A task with no due date set
When: Task list is rendered
Then: Task appears in BACKLOG section

### Scenario 4: All-BACKLOG Edge Case
Given: All tasks have no due date
When: User views task list
Then: BACKLOG is NOT auto-collapsed (only content)

## 5. Out of Scope

- Configurable section colors
- Persistent collapse state across sessions
- Animated transitions
- Search/filter within groups
