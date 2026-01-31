# Form Component Integration Guide

Quick reference for integrating the form component into task/log/note creation commands.

## Quick Start

### 1. Add Form to Your Model

```go
import "github.com/MikeBiancalana/reckon/internal/tui/components"

type Model struct {
    // ... existing fields
    taskForm *components.Form
}

func initialModel() Model {
    // Create task form
    taskForm := components.NewForm("Create Task")
    taskForm.AddField(components.FormField{
        Label:       "Task Name",
        Key:         "name",
        Type:        components.FieldTypeText,
        Required:    true,
        Placeholder: "What needs to be done?",
    }).AddField(components.FormField{
        Label:       "Tags",
        Key:         "tags",
        Type:        components.FieldTypeText,
        Required:    false,
        Placeholder: "#work #urgent",
    }).AddField(components.FormField{
        Label:       "Due Date",
        Key:         "due_date",
        Type:        components.FieldTypeDate,
        Required:    false,
        Placeholder: "t, tm, +3d, mon",
    })

    return Model{
        taskForm: taskForm,
    }
}
```

### 2. Handle Keyboard Input

```go
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    var cmd tea.Cmd

    switch msg := msg.(type) {
    case tea.KeyMsg:
        // Show form when user presses 't' for task
        if msg.String() == "t" && !m.taskForm.IsVisible() {
            return m, m.taskForm.Show()
        }

    case components.FormSubmitMsg:
        return m.handleTaskFormSubmit(msg)

    case components.FormCancelMsg:
        m.taskForm.Hide()
        return m, nil
    }

    // Update form
    m.taskForm, cmd = m.taskForm.Update(msg)
    return m, cmd
}
```

### 3. Process Form Submission

```go
func (m Model) handleTaskFormSubmit(msg components.FormSubmitMsg) (tea.Model, tea.Cmd) {
    values := msg.Result.Values

    taskName := values["name"]
    tags := values["tags"]
    dueDateStr := values["due_date"]

    // Parse date if provided
    var dueDate time.Time
    if dueDateStr != "" {
        date, err := m.taskForm.ParsedDateValue("due_date")
        if err == nil {
            dueDate = date
        }
    }

    // Create the task
    capturedJournal := m.currentJournal
    return m, func() tea.Msg {
        err := m.service.AddTask(capturedJournal, taskName, tags, dueDate)
        if err != nil {
            return errMsg{err}
        }
        return taskCreatedMsg{}
    }
}
```

### 4. Render the Form

```go
func (m Model) View() string {
    // If form is visible, show it
    if m.taskForm.IsVisible() {
        return m.taskForm.View()
    }

    // Otherwise show normal UI
    return m.mainView()
}
```

## Use Cases

### Task Creation Form

```go
taskForm := components.NewForm("Create Task")
taskForm.AddField(components.FormField{
    Label:       "Task Name",
    Key:         "name",
    Type:        components.FieldTypeText,
    Required:    true,
    Placeholder: "What needs to be done?",
}).AddField(components.FormField{
    Label:       "Tags",
    Key:         "tags",
    Type:        components.FieldTypeText,
    Required:    false,
    Placeholder: "#tag1 #tag2",
}).AddField(components.FormField{
    Label:       "Due Date",
    Key:         "due_date",
    Type:        components.FieldTypeDate,
    Required:    false,
    Placeholder: "t, tm, +3d, mon, 2026-12-31",
})
```

### Log Entry Form

```go
logForm := components.NewForm("Add Log Entry")
logForm.AddField(components.FormField{
    Label:       "Content",
    Key:         "content",
    Type:        components.FieldTypeText,
    Required:    true,
    Placeholder: "What happened?",
}).AddField(components.FormField{
    Label:       "Tags",
    Key:         "tags",
    Type:        components.FieldTypeText,
    Required:    false,
    Placeholder: "#meeting #planning",
})
```

### Note Creation Form

```go
noteForm := components.NewForm("Create Note")
noteForm.AddField(components.FormField{
    Label:       "Title",
    Key:         "title",
    Type:        components.FieldTypeText,
    Required:    true,
    Placeholder: "Note title",
}).AddField(components.FormField{
    Label:       "Content",
    Key:         "content",
    Type:        components.FieldTypeText,
    Required:    true,
    Placeholder: "Note content",
}).AddField(components.FormField{
    Label:       "Tags",
    Key:         "tags",
    Type:        components.FieldTypeText,
    Required:    false,
    Placeholder: "#ideas #reference",
})
```

## Message Flow

1. User presses trigger key (e.g., 't' for task)
2. Form.Show() is called, returns focus command
3. Form becomes visible and focused on first field
4. User fills in fields (Tab/Shift+Tab to navigate)
5. User submits (Enter) or cancels (ESC)
6. Form validates and sends FormSubmitMsg or FormCancelMsg
7. Parent model handles the message
8. Form.Hide() is called

## Best Practices

### Error Handling

```go
case components.FormSubmitMsg:
    values := msg.Result.Values

    // Validate business logic
    if containsInvalidChars(values["name"]) {
        // Show error to user
        return m, func() tea.Msg {
            return errMsg{fmt.Errorf("invalid characters in name")}
        }
    }

    // Process the form
    m.taskForm.Hide()
    return m, m.createTask(values)
```

### Multiple Forms

```go
type Model struct {
    taskForm *components.Form
    logForm  *components.Form
    noteForm *components.Form
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    var cmd tea.Cmd

    switch msg := msg.(type) {
    case tea.KeyMsg:
        switch msg.String() {
        case "t":
            return m, m.taskForm.Show()
        case "l":
            return m, m.logForm.Show()
        case "n":
            return m, m.noteForm.Show()
        }

    case components.FormSubmitMsg:
        // Determine which form was submitted
        if m.taskForm.IsVisible() {
            return m.handleTaskSubmit(msg)
        } else if m.logForm.IsVisible() {
            return m.handleLogSubmit(msg)
        } else if m.noteForm.IsVisible() {
            return m.handleNoteSubmit(msg)
        }
    }

    // Update all forms
    m.taskForm, cmd = m.taskForm.Update(msg)
    m.logForm, _ = m.logForm.Update(msg)
    m.noteForm, _ = m.noteForm.Update(msg)

    return m, cmd
}
```

### Async Closure Capture

When creating async commands, always capture model values before the closure:

```go
// CORRECT
func (m Model) handleTaskFormSubmit(msg components.FormSubmitMsg) (tea.Model, tea.Cmd) {
    values := msg.Result.Values
    capturedJournal := m.currentJournal  // Capture before closure

    return m, func() tea.Msg {
        err := m.service.AddTask(capturedJournal, values["name"])
        return taskCreatedMsg{err: err}
    }
}

// WRONG - m.currentJournal may change before closure runs
func (m Model) handleTaskFormSubmit(msg components.FormSubmitMsg) (tea.Model, tea.Cmd) {
    values := msg.Result.Values

    return m, func() tea.Msg {
        err := m.service.AddTask(m.currentJournal, values["name"])  // Bug!
        return taskCreatedMsg{err: err}
    }
}
```

### Form Width

Set form width to match your UI:

```go
func (m Model) handleWindowSize(msg tea.WindowSizeMsg) Model {
    m.width = msg.Width
    m.height = msg.Height

    // Update form width
    m.taskForm.SetWidth(m.width - 4)  // Leave some margin

    return m
}
```

## Testing

Test form integration in your model:

```go
func TestModel_TaskFormSubmit(t *testing.T) {
    m := initialModel()

    // Show form
    m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRune, Runes: []rune{'t'}})

    // Fill in values
    m.taskForm.SetValues(map[string]string{
        "name": "Test task",
        "due_date": "tm",
    })

    // Submit
    m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

    // Verify form submitted
    require.NotNil(t, cmd)
    msg := cmd()
    _, ok := msg.(components.FormSubmitMsg)
    assert.True(t, ok)
}
```

## Migration from TextEntryBar

If you're replacing TextEntryBar with Form:

### Before (TextEntryBar)

```go
// Single field only
textEntry.SetMode(components.ModeTask)
textEntry.Focus()

// On submit
value := textEntry.GetValue()
service.AddTask(journal, value)
```

### After (Form)

```go
// Multiple fields with validation
taskForm.Show()

// On submit
case components.FormSubmitMsg:
    values := msg.Result.Values
    service.AddTask(journal, values["name"], values["tags"])
```

## Tips

1. Always call Hide() after handling FormSubmitMsg or FormCancelMsg
2. Use ParsedDateValue() for date fields instead of parsing manually
3. Custom validators run after built-in validation
4. Tab navigation wraps around (first ↔ last field)
5. Form clears values on Show() for clean state
6. Set form width on window resize for responsive UI
7. Use descriptive field keys - they're used in the result map

## See Also

- FORM_README.md - Complete component documentation
- form_example_test.go - Usage examples
- form_test.go - Test examples
