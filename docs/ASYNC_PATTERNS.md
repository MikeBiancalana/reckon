# Bubbletea Async Patterns in Reckon

This guide documents common async pitfalls, `tea.Cmd` best practices, and message passing guidelines for the Reckon TUI.

## Table of Contents

1. [The tea.Cmd Pattern](#the-teacmd-pattern)
2. [Closure Capture by Reference](#closure-capture-by-reference)
3. [Component Update Patterns](#component-update-patterns)
4. [Message Passing](#message-passing)
5. [File Watching](#file-watching)
6. [Common Pitfalls](#common-pitfalls)
7. [Testing Async Code](#testing-async-code)

---

## The tea.Cmd Pattern

Bubble Tea uses `tea.Cmd` (a `func() tea.Msg`) for async operations. Commands run asynchronously and deliver results via messages.

### Basic Pattern

```go
func (m *Model) loadJournal() tea.Cmd {
    return func() tea.Msg {
        // This runs asynchronously in the Bubbletea runtime
        j, err := m.service.GetByDate(m.currentDate)
        if err != nil {
            return errMsg{err}
        }
        return journalLoadedMsg{journal: *j}
    }
}
```

### Combining Commands

Use `tea.Batch` to run multiple commands concurrently:

```go
func (m *Model) Init() tea.Cmd {
    cmds := []tea.Cmd{m.loadJournal()}

    if m.taskService != nil {
        cmds = append(cmds, m.loadTasks())
    }

    if m.watcher != nil {
        if err := m.watcher.Start(); err == nil {
            cmds = append(cmds, m.waitForFileChange())
        }
    }

    return tea.Batch(cmds...)
}
```

### Component Update Pattern

Components return `(component, tea.Cmd)` from their Update method:

```go
func (tl *TaskList) Update(msg tea.Msg) (*TaskList, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        switch msg.String() {
        case "enter":
            return tl, func() tea.Msg {
                return TaskToggleMsg{TaskID: tl.selectedTask.ID}
            }
        }
    }
    var cmd tea.Cmd
    tl.list, cmd = tl.list.Update(msg)
    return tl, cmd
}
```

---

## Closure Capture by Reference

**This is the most common bug source in Bubble Tea code.** Go closures capture variables by **reference**, not by value.

### The Problem

```go
// WRONG - Buggy code!
case "enter":
    cmd := m.submitTextEntry()
    m.noteLogEntryID = ""  // Reset BEFORE async function runs!
    return m, cmd

func (m *Model) submitTextEntry() tea.Cmd {
    return func() tea.Msg {
        // BUG: Captures m.noteLogEntryID by reference
        // By the time this runs, it's already ""!
        err = m.service.AddLogNote(
            m.currentJournal,
            m.noteLogEntryID,  // Empty! Bug!
            inputText
        )
    }
}
```

### The Solution

**Capture all values BEFORE creating the closure:**

```go
// CORRECT - Capture values at closure creation time
capturedLogEntryID := m.noteLogEntryID
capturedTaskID := m.noteTaskID
capturedCurrentJournal := m.currentJournal

return func() tea.Msg {
    // Uses captured values - snapshots at closure creation time
    err = m.service.AddLogNote(
        capturedCurrentJournal,
        capturedLogEntryID,  // Correct value!
        inputText
    )
}
```

### Best Practices

1. **Capture all model values you need BEFORE returning the closure**
2. **Use descriptive variable names:** `capturedXxx` for clarity
3. **Function parameters are captured at the point of definition**
4. **Document the capture pattern** in code comments

### Pattern Used in Reckon

```go
// CRITICAL: Capture ALL values we need BEFORE creating the async function.
// Go closures capture variables by REFERENCE, not by value.
// If we don't capture them here, the values may be reset by the key handler
// before the async function runs.
capturedLogEntryID := m.noteLogEntryID
capturedTaskID := m.noteTaskID
capturedEditID := m.editItemID
capturedEditType := m.editItemType
capturedCurrentJournal := m.currentJournal
capturedMode := mode
capturedTaskService := m.taskService

return func() tea.Msg {
    switch capturedMode {
    case components.ModeLogNote:
        err = m.service.AddLogNote(capturedCurrentJournal, capturedLogEntryID, inputText)
    case components.ModeTaskNote:
        err = m.taskService.AddNote(capturedTaskID, inputText)
    }
    return err
}
```

---

## Component Update Patterns

### Update vs Recreate

**Never recreate components** - this destroys all UI state (cursor position, collapsed state, etc.).

```go
// WRONG - Destroys cursor position and collapsed state
case journalLoadedMsg:
    m.intentionList = components.NewIntentionList(msg.journal.Intentions)
    return m, nil

// CORRECT - Preserves UI state
case journalLoadedMsg:
    m.currentJournal = &msg.journal
    if m.intentionList == nil {
        m.intentionList = components.NewIntentionList(msg.journal.Intentions)
    } else {
        m.intentionList.UpdateIntentions(msg.journal.Intentions)
    }
    return m, nil
```

### Preserving Cursor Position

When updating lists, preserve the cursor position:

```go
func (tl *TaskList) UpdateTasks(tasks []journal.Task) {
    selectedItem := tl.list.SelectedItem()
    var selectedTaskID string
    if selectedItem != nil {
        if taskItem, ok := selectedItem.(TaskItem); ok {
            selectedTaskID = taskItem.task.ID
        }
    }

    tl.tasks = tasks
    items := buildTaskItems(tasks, tl.collapsedMap)
    tl.list.SetItems(items)

    // Restore cursor to the previously selected task
    if selectedTaskID != "" {
        for i, item := range items {
            if taskItem, ok := item.(TaskItem); ok && !taskItem.isNote && taskItem.task.ID == selectedTaskID {
                tl.list.Select(i)
                break
            }
        }
    }
}
```

### Focus Management

Track focus state for styling and input delegation:

```go
func (tl *TaskList) SetFocused(focused bool) {
    tl.focused = focused
    if focused {
        tl.list.Styles.Title = focusedTaskListTitleStyle
    } else {
        tl.list.Styles.Title = taskListTitleStyle
    }
}
```

---

## Message Passing

### Custom Message Types

Define messages for communication between components:

```go
// Internal messages (model.go)
type journalLoadedMsg struct {
    journal journal.Journal
}

type journalUpdatedMsg struct{}

type fileChangedMsg struct {
    date string
}

type errMsg struct {
    err error
}

// Component messages (components/ files)
type TaskToggleMsg struct {
    TaskID string
}

type TaskNoteDeleteMsg struct {
    TaskID string
    NoteID string
}
```

### Component-to-Parent Message Passing

Components send messages up to the parent model:

```go
// In a child component
case "n":
    return lv, func() tea.Msg {
        return LogNoteAddMsg{LogEntryID: entryID}
    }

// In the parent model
case components.LogNoteAddMsg:
    m.noteLogEntryID = msg.LogEntryID
    m.textEntryBar.SetMode(components.ModeLogNote)
    m.textEntryBar.Clear()
    return m, m.textEntryBar.Focus()
```

### Message Handler Pattern

Handle messages in the Update method:

```go
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case journalLoadedMsg:
        m.currentJournal = &msg.journal
        return m, m.intentionList.UpdateIntentions(msg.journal.Intentions)

    case errMsg:
        m.err = msg.err
        return m, nil

    case tea.KeyMsg:
        return m.handleKeyMsg(msg)
    }

    var cmd tea.Cmd
    m, cmd = m.updateComponents(msg)
    return m, cmd
}
```

---

## File Watching

### Channel-Based Pattern

The file watcher uses a channel combined with tea.Cmd:

```go
func (m *Model) waitForFileChange() tea.Cmd {
    if m.watcher == nil {
        return nil
    }

    return func() tea.Msg {
        event := <-m.watcher.Changes()
        return fileChangedMsg{date: event.Date}
    }
}
```

### Debouncing

Debounce file change events to avoid duplicate processing:

```go
func (w *Watcher) watch() {
    const debounceDelay = 100 * time.Millisecond

    for {
        select {
        case event, ok := <-w.watcher.Events:
            if !ok {
                return
            }

            if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
                if filepath.Ext(event.Name) == ".md" {
                    date := extractDate(event.Name)
                    w.pendingEvents[date] = true

                    if w.debounceTimer != nil {
                        w.debounceTimer.Stop()
                    }

                    w.debounceTimer = time.AfterFunc(debounceDelay, func() {
                        w.processPendingEvents()
                    })
                }
            }
        }
    }
}
```

---

## Common Pitfalls

### 1. State Mutation in Commands

**Problem:** Mutating state inside tea.Cmd closures causes race conditions.

**Solution:** State changes happen in `Update()`, not in commands:

```go
// WRONG - State mutation in closure
case "enter":
    return m, func() tea.Msg {
        m.textEntryBar.Clear()  // Mutation in async code!
        return submittedMsg{}
    }

// CORRECT - State mutation in Update()
case "enter":
    cmd := m.submitTextEntry()
    m.textEntryBar.Clear()  // Mutation in Update()
    return m, cmd
```

### 2. Forgetting Nil Checks

Always check for nil before using services or watchers:

```go
func (m *Model) loadTasks() tea.Cmd {
    if m.taskService == nil {
        return nil
    }
    return func() tea.Msg {
        tasks, err := m.taskService.List()
        if err != nil {
            return errMsg{err}
        }
        return tasksLoadedMsg{tasks}
    }
}
```

### 3. Ignoring Errors

Handle errors from async operations:

```go
return func() tea.Msg {
    if err := m.service.Save(journal); err != nil {
        return errMsg{err}
    }
    return journalUpdatedMsg{}
}
```

### 4. Blocking the Main Loop

Never perform long-running operations synchronously:

```go
// WRONG - Blocks the main loop!
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg.(type) {
    case someMsg:
        result := m.service.ExpensiveOperation()  // Blocks!
        return m, nil
    }
}

// CORRECT - Use tea.Cmd for expensive operations
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg.(type) {
    case someMsg:
        return m, func() tea.Msg {
            result := m.service.ExpensiveOperation()
            return expensiveOpDoneMsg{result}
        }
    }
}
```

---

## Testing Async Code

### Integration Tests

The integration tests verify the closure capture fix:

```go
func TestLogNotePersistence(t *testing.T) {
    // This test verifies the fix for the bug where log notes and task notes
    // were not being written to markdown files due to closure capture issues.
    // The bug was that Go closures capture variables by reference, not by value.
    // When the Enter key handler reset m.noteLogEntryID before the async function
    // ran, the closure saw the empty value.
}
```

### Test Pattern

```go
func TestAsyncOperation(t *testing.T) {
    // Given
    model := NewModelWithDeps(deps)

    // When - trigger async operation
    _, cmd := model.Update(asyncTriggerMsg{})

    // Then - command should not be nil
    require.NotNil(t, cmd)

    // When - execute the command
    msg := cmd()

    // Then - verify result
    assert.IsType(t, asyncDoneMsg{}, msg)
}
```

---

## Quick Reference

| Pattern | Do | Don't |
|---------|-----|-------|
| Closure capture | Capture values before creating closure | Reference model fields in closures |
| Component update | Update existing components | Recreate components |
| State mutation | Mutate state in Update() | Mutate state in commands |
| Error handling | Return error messages | Ignore async errors |
| Long operations | Use tea.Cmd | Block in Update() |

---

## Related Files

- `internal/tui/model.go` - Main model with async patterns and closure capture documentation
- `internal/tui/components/task_list.go` - Component with message passing patterns
- `internal/tui/components/log_view.go` - Component with log note message patterns
- `internal/sync/watcher.go` - File watching with channel-based notifications
- `internal/journal/service.go` - Service layer (synchronous, not thread-safe)
- `tests/integration_test.go` - Integration tests including async patterns
