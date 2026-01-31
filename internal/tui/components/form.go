package components

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// FormFieldType represents the type of form field
type FormFieldType int

const (
	FieldTypeText FormFieldType = iota
	FieldTypeDate
)

// FormField represents a single field in a form
type FormField struct {
	Label       string
	Key         string
	Type        FormFieldType
	Required    bool
	Placeholder string
	Validator   func(string) error
	textInput   textinput.Model
}

// FormResult represents the result of a submitted form
type FormResult struct {
	Values map[string]string
}

// FormSubmitMsg is sent when the form is successfully submitted
type FormSubmitMsg struct {
	Result FormResult
}

// FormCancelMsg is sent when the form is cancelled
type FormCancelMsg struct{}

var (
	formBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("39")).
			Padding(1, 2)

	formTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39"))

	formLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	formLabelFocusedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("39")).
				Bold(true)

	formErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Italic(true)

	formHelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	formDatePreviewStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("40")).
				Italic(true)
)

// Form is a reusable multi-field form component
type Form struct {
	title       string
	fields      []FormField
	focusIndex  int
	visible     bool
	errors      map[string]string
	width       int
	submitted   bool
	datePreview string // Preview for date fields
}

// NewForm creates a new form with the given title
func NewForm(title string) *Form {
	return &Form{
		title:      title,
		fields:     make([]FormField, 0),
		focusIndex: 0,
		visible:    false,
		errors:     make(map[string]string),
		width:      60,
		submitted:  false,
	}
}

// AddField adds a field to the form
func (f *Form) AddField(field FormField) *Form {
	// Create text input for the field
	ti := textinput.New()
	ti.Placeholder = field.Placeholder
	ti.CharLimit = 500
	ti.Width = 50

	field.textInput = ti
	f.fields = append(f.fields, field)
	return f
}

// Show displays the form and focuses the first field
func (f *Form) Show() tea.Cmd {
	f.visible = true
	f.submitted = false
	f.errors = make(map[string]string)
	f.datePreview = ""

	// Clear all field values
	for i := range f.fields {
		f.fields[i].textInput.SetValue("")
	}

	// Focus first field
	f.focusIndex = 0
	if len(f.fields) > 0 {
		return f.fields[0].textInput.Focus()
	}

	return nil
}

// Hide hides the form
func (f *Form) Hide() {
	f.visible = false
	f.submitted = false
	f.datePreview = ""
	for i := range f.fields {
		f.fields[i].textInput.Blur()
	}
}

// IsVisible returns whether the form is visible
func (f *Form) IsVisible() bool {
	return f.visible
}

// SetWidth sets the width of the form
func (f *Form) SetWidth(width int) {
	f.width = width
	// Update field widths
	fieldWidth := width - 20 // Account for borders and padding
	if fieldWidth < 20 {
		fieldWidth = 20
	}
	for i := range f.fields {
		f.fields[i].textInput.Width = fieldWidth
	}
}

// GetValues returns the current form values
func (f *Form) GetValues() map[string]string {
	values := make(map[string]string)
	for _, field := range f.fields {
		values[field.Key] = field.textInput.Value()
	}
	return values
}

// SetValues sets form field values
func (f *Form) SetValues(values map[string]string) {
	for i := range f.fields {
		if val, ok := values[f.fields[i].Key]; ok {
			f.fields[i].textInput.SetValue(val)
		}
	}
	// Update date preview if focused field is a date
	if f.focusIndex >= 0 && f.focusIndex < len(f.fields) {
		f.updateDatePreview()
	}
}

// Update handles Bubble Tea messages
func (f *Form) Update(msg tea.Msg) (*Form, tea.Cmd) {
	if !f.visible {
		return f, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc:
			f.Hide()
			return f, func() tea.Msg {
				return FormCancelMsg{}
			}

		case tea.KeyEnter:
			return f.handleSubmit()

		case tea.KeyTab, tea.KeyShiftTab:
			return f.handleTabNavigation(msg.Type == tea.KeyShiftTab)
		}
	}

	// Update the currently focused field
	if f.focusIndex >= 0 && f.focusIndex < len(f.fields) {
		var cmd tea.Cmd
		f.fields[f.focusIndex].textInput, cmd = f.fields[f.focusIndex].textInput.Update(msg)

		// Update date preview if this is a date field
		if f.fields[f.focusIndex].Type == FieldTypeDate {
			f.updateDatePreview()
		}

		return f, cmd
	}

	return f, nil
}

// handleTabNavigation handles Tab/Shift+Tab navigation between fields
func (f *Form) handleTabNavigation(reverse bool) (*Form, tea.Cmd) {
	if len(f.fields) == 0 {
		return f, nil
	}

	// Blur current field
	f.fields[f.focusIndex].textInput.Blur()

	// Move focus
	if reverse {
		f.focusIndex--
		if f.focusIndex < 0 {
			f.focusIndex = len(f.fields) - 1
		}
	} else {
		f.focusIndex++
		if f.focusIndex >= len(f.fields) {
			f.focusIndex = 0
		}
	}

	// Focus new field and update date preview
	f.updateDatePreview()
	return f, f.fields[f.focusIndex].textInput.Focus()
}

// handleSubmit validates and submits the form
func (f *Form) handleSubmit() (*Form, tea.Cmd) {
	// Validate all fields
	f.errors = make(map[string]string)
	valid := true

	for i, field := range f.fields {
		value := field.textInput.Value()

		// Check required fields
		if field.Required && strings.TrimSpace(value) == "" {
			f.errors[field.Key] = fmt.Sprintf("%s is required", field.Label)
			valid = false
			continue
		}

		// Skip validation for empty optional fields
		if !field.Required && strings.TrimSpace(value) == "" {
			continue
		}

		// Validate date fields
		if field.Type == FieldTypeDate {
			_, err := ParseRelativeDate(value)
			if err != nil {
				f.errors[field.Key] = err.Error()
				valid = false
				continue
			}
		}

		// Run custom validator if provided
		if field.Validator != nil {
			if err := field.Validator(value); err != nil {
				f.errors[field.Key] = err.Error()
				valid = false
				continue
			}
		}

		// Update the field in the slice
		f.fields[i] = field
	}

	if !valid {
		return f, nil
	}

	// Form is valid - submit it
	f.submitted = true
	result := FormResult{
		Values: f.GetValues(),
	}

	return f, func() tea.Msg {
		return FormSubmitMsg{Result: result}
	}
}

// updateDatePreview updates the date preview for the current field if it's a date field
func (f *Form) updateDatePreview() {
	f.datePreview = ""

	if f.focusIndex < 0 || f.focusIndex >= len(f.fields) {
		return
	}

	field := f.fields[f.focusIndex]
	if field.Type != FieldTypeDate {
		return
	}

	value := field.textInput.Value()
	if value == "" {
		return
	}

	date, err := ParseRelativeDate(value)
	if err != nil {
		return
	}

	description := GetDateDescription(date)
	f.datePreview = FormatDate(date) + " (" + description + ")"
}

// View renders the form
func (f *Form) View() string {
	if !f.visible {
		return ""
	}

	var content strings.Builder

	// Title
	content.WriteString(formTitleStyle.Render(f.title))
	content.WriteString("\n\n")

	// Fields
	for i, field := range f.fields {
		focused := i == f.focusIndex

		// Label
		labelStyle := formLabelStyle
		if focused {
			labelStyle = formLabelFocusedStyle
		}

		label := field.Label
		if field.Required {
			label += " *"
		}
		content.WriteString(labelStyle.Render(label))
		content.WriteString("\n")

		// Input
		content.WriteString(field.textInput.View())
		content.WriteString("\n")

		// Date preview for focused date field
		if focused && field.Type == FieldTypeDate && f.datePreview != "" {
			content.WriteString(formDatePreviewStyle.Render("→ " + f.datePreview))
			content.WriteString("\n")
		}

		// Error message
		if err, hasErr := f.errors[field.Key]; hasErr {
			content.WriteString(formErrorStyle.Render("✗ " + err))
			content.WriteString("\n")
		}

		// Add spacing between fields
		if i < len(f.fields)-1 {
			content.WriteString("\n")
		}
	}

	// Help text
	content.WriteString("\n")
	content.WriteString(formHelpStyle.Render("TAB: next field  SHIFT+TAB: previous field  ENTER: submit  ESC: cancel"))

	// Wrap in box
	return formBoxStyle.Render(content.String())
}

// ParsedDateValue returns the parsed date for a date field
func (f *Form) ParsedDateValue(key string) (time.Time, error) {
	for _, field := range f.fields {
		if field.Key == key && field.Type == FieldTypeDate {
			value := field.textInput.Value()
			if value == "" {
				return time.Time{}, nil
			}
			return ParseRelativeDate(value)
		}
	}
	return time.Time{}, fmt.Errorf("field %s not found or not a date field", key)
}
