# TUI Subsystem Guide

## Overview

The TUI is built with [Bubble Tea](https://github.com/charmbracelet/bubbletea), using the Elm architecture (Model-View-Update pattern).

**Key files:**
- `model.go` - Main TUI model and state
- `handlers.go` - Key event handlers
- `commands.go` - Async command functions
- `view.go` - View rendering
- `components/` - Reusable UI components (task list, log view, etc.)

## Architecture

### Model-View-Update Pattern

```
User Input → Update() → Model State Change → View() → Render
                ↓
         Cmd (async) → Msg → Update()
```

**Key types:**
- `Model` - Holds all TUI state (current date, journal, components)
- `tea.Msg` - Messages passed to Update (key presses, async results)
- `tea.Cmd` - Async commands that return messages

### Layout Structure (40-40-18)

```
┌─────────────────┬─────────────────┬──────────┐
│   Left (40%)    │  Center (40%)   │Right(18%)│
│                 │                 │          │
│   Log View      │   Task List     │ Schedule │
│                 │   (grouped by   │ Intents  │
│                 │    time)        │  Wins    │
│                 │                 │          │
└─────────────────┴─────────────────┴──────────┘
```

## Critical Patterns

### 1. Async Closure Capture (CRITICAL!)

**The Problem:**
Go closures capture variables by REFERENCE. When creating async commands (`tea.Cmd`), you must capture values BEFORE the closure.

**❌ WRONG - Bug waiting to happen:**
```go
func (m *Model) submitTextEntry() tea.Cmd {
    inputText := m.textEntryBar.GetValue()
    return func() tea.Msg {
        // BUG: m.currentJournal may have changed by the time this runs!
        err := m.service.AddWin(m.currentJournal, inputText)
        return errMsg{err}
    }
}
```

**✅ CORRECT - Capture value before closure:**
```go
func (m *Model) submitTextEntry() tea.Cmd {
    inputText := m.textEntryBar.GetValue()
    capturedJournal := m.currentJournal  // Capture BEFORE closure
    return func() tea.Msg {
        // Safe: capturedJournal is the value at capture time
        err := m.service.AddWin(capturedJournal, inputText)
        return errMsg{err}
    }
}
```

**When to capture:**
- Model fields (m.currentDate, m.currentJournal, etc.)
- Component state
- Service references (if they could be nil)

**See also:** `/docs/ASYNC_PATTERNS.md` for comprehensive documentation

### 2. Component Communication

Components communicate via messages:

```go
// Component sends message
return tl, func() tea.Msg {
    return TaskToggleMsg{TaskID: taskItem.task.ID}
}

// Model handles message
case TaskToggleMsg:
    // Update state
    // Trigger async command if needed
```

**Pattern:** Components emit messages, Model handles them and updates state.

### 3. State Synchronization

The Model maintains the canonical state. Components reflect that state.

**When state changes:**
```go
// 1. Update model state
m.currentJournal = updatedJournal

// 2. Update components to reflect new state
m.logView.UpdateLogs(updatedJournal.Log)
m.intentionList.UpdateIntentions(updatedJournal.Intentions)

// 3. Return updated model
return m, cmd
```

**Anti-pattern:** Don't let components hold canonical state that Model also needs.

### 4. Focus Management

Only the focused section receives keyboard input:

```go
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        if m.focusedSection == SectionTasks {
            m.taskList, cmd = m.taskList.Update(msg)
            return m, cmd
        }
        // ... other sections
    }
}
```

Each component checks `focused` flag before handling keys.

## Common Tasks

### Add a New Section

1. Add enum to `model.go`:
   ```go
   const (
       SectionIntentions Section = iota
       SectionWins
       SectionLogs
       SectionTasks
       SectionSchedule
       SectionYourNew  // Add here
       SectionCount
   )
   ```

2. Add component to Model:
   ```go
   type Model struct {
       // ... existing fields
       yourNewComponent *components.YourComponent
   }
   ```

3. Handle focus switching in `Update()`
4. Render in `View()`

### Add a New Keybinding

1. Update `handlers.go`:
   ```go
   case "x":  // Your new key
       if m.focusedSection == SectionTasks {
           return m.handleYourAction()
       }
   ```

2. Update help text in `view.go`
3. Test all sections to avoid conflicts

### Add a New Message Type

1. Define in relevant component file:
   ```go
   type YourMsg struct {
       Field string
   }
   ```

2. Send from component:
   ```go
   return component, func() tea.Msg {
       return YourMsg{Field: "value"}
   }
   ```

3. Handle in `model.go` Update:
   ```go
   case YourMsg:
       return m.handleYourMsg(msg)
   ```

## Testing TUI Components

**Test the model, not the rendering:**

```go
func TestModelUpdate(t *testing.T) {
    m := NewModel(service, taskService, watcher)

    // Simulate key press
    msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")}
    updatedModel, cmd := m.Update(msg)

    // Check state change
    if updatedModel.taskCreationMode != true {
        t.Error("Expected task creation mode to be enabled")
    }
}
```

**Don't test Bubble Tea rendering** - it's integration-level, slow, and fragile.

## Common Pitfalls

### 1. Forgetting to Return Updated Model

```go
// ❌ WRONG
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    m.someField = "changed"  // This changes a COPY
    return m, nil            // Returns original
}

// ✅ CORRECT - Use pointer receiver or return modified copy
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    m.someField = "changed"  // Modifies in place
    return m, nil
}
```

### 2. Not Checking Component Nil

```go
// ❌ Can panic if component not initialized
m.taskList.Update(msg)

// ✅ Check first
if m.taskList != nil {
    m.taskList.Update(msg)
}
```

### 3. Mutating Shared Slices

```go
// ❌ Appending to shared slice
tasks := m.taskList.GetTasks()
tasks = append(tasks, newTask)  // Doesn't update component

// ✅ Update through component method
m.taskList.UpdateTasks(append(tasks, newTask))
```

## Performance Considerations

- **Avoid expensive operations in View()** - it's called frequently
- **Cache formatted strings** if computing them is expensive
- **Use lipgloss Style caching** - create styles once, reuse
- **Debounce rapid updates** (e.g., file watcher) using timers

## Resources

- **Bubble Tea docs:** https://github.com/charmbracelet/bubbletea
- **Lipgloss (styling):** https://github.com/charmbracelet/lipgloss
- **Async patterns:** `/docs/ASYNC_PATTERNS.md`
- **TUI examples:** `internal/tui/components/*_test.go`
