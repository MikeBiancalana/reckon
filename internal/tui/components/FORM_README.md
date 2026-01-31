# Form Component

A reusable mini-TUI form component for the reckon project built with Bubble Tea.

## Features

- **Multi-field support**: Add multiple fields to a single form
- **Tab navigation**: Navigate between fields using Tab and Shift+Tab
- **Field validation**: Built-in validation for required fields and dates
- **Custom validators**: Add custom validation logic to any field
- **Date parsing**: Natural date parsing integration (t, tm, +3d, mon, 2026-12-31)
- **Live previews**: Date fields show live preview of parsed dates
- **Error messages**: Clear error messages for validation failures
- **Cancellation**: ESC key to cancel and close the form
- **Type safety**: Strongly typed field definitions and results

## Field Types

- **FieldTypeText**: Standard text input field
- **FieldTypeDate**: Date field with natural date parsing

## Basic Usage

```go
package main

import (
    "github.com/MikeBiancalana/reckon/internal/tui/components"
    tea "github.com/charmbracelet/bubbletea"
)

// Create a form
form := components.NewForm("Create Task")

// Add fields
form.AddField(components.FormField{
    Label:       "Task Name",
    Key:         "name",
    Type:        components.FieldTypeText,
    Required:    true,
    Placeholder: "Enter task name",
}).AddField(components.FormField{
    Label:       "Due Date",
    Key:         "due_date",
    Type:        components.FieldTypeDate,
    Required:    false,
    Placeholder: "t, tm, +3d, mon",
})

// Show the form
form.Show()
```

## Integration with Bubble Tea

```go
type Model struct {
    form *components.Form
}

func (m Model) Init() tea.Cmd {
    return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    var cmd tea.Cmd

    switch msg := msg.(type) {
    case tea.KeyMsg:
        switch msg.String() {
        case "n":
            // Show form when user presses 'n'
            return m, m.form.Show()
        }

    case components.FormSubmitMsg:
        // Handle successful form submission
        values := msg.Result.Values
        taskName := values["name"]
        dueDate := values["due_date"]

        // Process the form data
        // ...

        m.form.Hide()
        return m, nil

    case components.FormCancelMsg:
        // Handle form cancellation
        m.form.Hide()
        return m, nil
    }

    // Update form
    m.form, cmd = m.form.Update(msg)
    return m, cmd
}

func (m Model) View() string {
    return m.form.View()
}
```

## Field Definition

### FormField Structure

```go
type FormField struct {
    Label       string                // Display label for the field
    Key         string                // Unique key for the field (used in results)
    Type        FormFieldType         // Field type (Text or Date)
    Required    bool                  // Whether the field is required
    Placeholder string                // Placeholder text
    Validator   func(string) error    // Optional custom validator
}
```

### Required Fields

Mark fields as required:

```go
form.AddField(components.FormField{
    Label:    "Task Name",
    Key:      "name",
    Type:     components.FieldTypeText,
    Required: true,
})
```

Required fields will show an asterisk (*) in the label and prevent form submission if empty.

### Optional Fields

Optional fields can be left empty:

```go
form.AddField(components.FormField{
    Label:    "Notes",
    Key:      "notes",
    Type:     components.FieldTypeText,
    Required: false,
})
```

## Date Fields

Date fields support natural date parsing:

```go
form.AddField(components.FormField{
    Label:       "Due Date",
    Key:         "due_date",
    Type:        components.FieldTypeDate,
    Required:    false,
    Placeholder: "t, tm, +3d, mon, 2026-12-31",
})
```

### Supported Date Formats

- `t` or `today` - Today
- `tm` or `tomorrow` - Tomorrow
- `+3d` - 3 days from now
- `+2w` - 2 weeks from now
- `mon`, `tue`, `wed`, `thu`, `fri`, `sat`, `sun` - Next occurrence of weekday
- `2026-12-31` - Absolute date (YYYY-MM-DD)

### Date Preview

Date fields show a live preview of the parsed date as you type:

```
Due Date
tm
→ 2026-02-01 (tomorrow)
```

### Parsing Date Values

After form submission, parse date values:

```go
case components.FormSubmitMsg:
    // Get the parsed date
    date, err := form.ParsedDateValue("due_date")
    if err != nil {
        // Handle error
    }

    if !date.IsZero() {
        // Date was provided
        fmt.Printf("Due: %s\n", date.Format("2006-01-02"))
    }
```

## Custom Validation

Add custom validation logic to any field:

```go
form.AddField(components.FormField{
    Label:    "Email",
    Key:      "email",
    Type:     components.FieldTypeText,
    Required: true,
    Validator: func(value string) error {
        if !strings.Contains(value, "@") {
            return fmt.Errorf("invalid email format")
        }
        return nil
    },
})
```

Custom validators:
- Only run if the field has a value (or is required)
- Run after built-in validation (required, date parsing)
- Return an error to prevent submission
- Error messages are displayed below the field

## Keyboard Shortcuts

- **Tab**: Move to next field
- **Shift+Tab**: Move to previous field
- **Enter**: Submit form (if validation passes)
- **Esc**: Cancel and close form
- **Standard text editing**: All standard Bubble Tea textinput shortcuts

## Form Messages

### FormSubmitMsg

Sent when the form is successfully submitted:

```go
type FormSubmitMsg struct {
    Result FormResult
}

type FormResult struct {
    Values map[string]string
}
```

Access field values by key:

```go
case components.FormSubmitMsg:
    taskName := msg.Result.Values["name"]
    dueDate := msg.Result.Values["due_date"]
```

### FormCancelMsg

Sent when the form is cancelled (ESC key):

```go
case components.FormCancelMsg:
    // User cancelled the form
    m.form.Hide()
```

## Methods

### Creation

- `NewForm(title string) *Form` - Create a new form with a title

### Configuration

- `AddField(field FormField) *Form` - Add a field (chainable)
- `SetWidth(width int)` - Set form width
- `SetValues(values map[string]string)` - Set field values programmatically

### Display

- `Show() tea.Cmd` - Show the form and focus first field
- `Hide()` - Hide the form
- `IsVisible() bool` - Check if form is visible

### Data Access

- `GetValues() map[string]string` - Get all field values
- `ParsedDateValue(key string) (time.Time, error)` - Parse a date field value

### Bubble Tea Integration

- `Update(msg tea.Msg) (*Form, tea.Cmd)` - Handle messages
- `View() string` - Render the form

## Examples

### Task Creation Form

```go
form := components.NewForm("Create Task")

form.AddField(components.FormField{
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
```

### Log Entry Form

```go
form := components.NewForm("Add Log Entry")

form.AddField(components.FormField{
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
form := components.NewForm("Create Note")

form.AddField(components.FormField{
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

## Styling

The form uses the project's consistent styling:

- **Border**: Rounded border with color `39` (blue)
- **Title**: Bold blue text
- **Labels**: Gray text (`245`), blue and bold when focused (`39`)
- **Errors**: Red text (`196`) with italic style
- **Help text**: Dimmed gray (`240`)
- **Date preview**: Green text (`40`) with italic style

## Error Handling

The form displays validation errors inline:

```
Email *
test@
✗ invalid email format
```

Errors are cleared when:
- User modifies the field
- Form is resubmitted successfully
- Form is hidden/closed

## Best Practices

1. **Use descriptive labels**: Make it clear what each field is for
2. **Add helpful placeholders**: Show examples of valid input
3. **Mark required fields**: Use `Required: true` for mandatory fields
4. **Keep forms focused**: Don't add too many fields to a single form
5. **Use date fields for dates**: Take advantage of natural date parsing
6. **Handle both submit and cancel**: Always handle both `FormSubmitMsg` and `FormCancelMsg`
7. **Validate critical data**: Add custom validators for important fields
8. **Clear form state**: Call `Hide()` after handling submission/cancellation

## Testing

The form component includes comprehensive tests. See `form_test.go` for examples.

Run tests:

```bash
go test ./internal/tui/components -run TestForm -v
```

Run examples:

```bash
go test ./internal/tui/components -run ExampleForm -v
```

## Architecture

The form component follows Bubble Tea patterns:

- **Composable**: Forms contain multiple field components
- **Message-driven**: Uses Bubble Tea's message system for communication
- **Immutable updates**: Update returns a new form state
- **Command-based**: Actions return tea.Cmd for async operations
- **Self-contained**: All validation and state management is internal

## Future Enhancements

Potential future additions:

- Additional field types (textarea, select, checkbox)
- Field dependencies (show/hide based on other fields)
- Multi-step forms with pages
- Form persistence/autosave
- Async validation
- Custom field renderers
