package components

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForm_NewForm(t *testing.T) {
	form := NewForm("Test Form")

	assert.NotNil(t, form)
	assert.Equal(t, "Test Form", form.title)
	assert.False(t, form.visible)
	assert.Equal(t, 0, len(form.fields))
	assert.Equal(t, 0, form.focusIndex)
}

func TestForm_AddField(t *testing.T) {
	form := NewForm("Test Form")

	field := FormField{
		Label:       "Name",
		Key:         "name",
		Type:        FieldTypeText,
		Required:    true,
		Placeholder: "Enter your name",
	}

	form.AddField(field)

	assert.Equal(t, 1, len(form.fields))
	assert.Equal(t, "Name", form.fields[0].Label)
	assert.Equal(t, "name", form.fields[0].Key)
	assert.True(t, form.fields[0].Required)
}

func TestForm_AddMultipleFields(t *testing.T) {
	form := NewForm("Test Form")

	form.AddField(FormField{
		Label:    "Name",
		Key:      "name",
		Type:     FieldTypeText,
		Required: true,
	}).AddField(FormField{
		Label:    "Email",
		Key:      "email",
		Type:     FieldTypeText,
		Required: false,
	}).AddField(FormField{
		Label:    "Due Date",
		Key:      "due_date",
		Type:     FieldTypeDate,
		Required: false,
	})

	assert.Equal(t, 3, len(form.fields))
	assert.Equal(t, "name", form.fields[0].Key)
	assert.Equal(t, "email", form.fields[1].Key)
	assert.Equal(t, "due_date", form.fields[2].Key)
}

func TestForm_ShowAndHide(t *testing.T) {
	form := NewForm("Test Form")
	form.AddField(FormField{
		Label: "Name",
		Key:   "name",
		Type:  FieldTypeText,
	})

	// Initially not visible
	assert.False(t, form.IsVisible())

	// Show form
	form.Show()
	assert.True(t, form.IsVisible())
	assert.Equal(t, 0, form.focusIndex)

	// Hide form
	form.Hide()
	assert.False(t, form.IsVisible())
}

func TestForm_GetAndSetValues(t *testing.T) {
	form := NewForm("Test Form")
	form.AddField(FormField{
		Label: "Name",
		Key:   "name",
		Type:  FieldTypeText,
	}).AddField(FormField{
		Label: "Email",
		Key:   "email",
		Type:  FieldTypeText,
	})

	// Set values
	values := map[string]string{
		"name":  "John Doe",
		"email": "john@example.com",
	}
	form.SetValues(values)

	// Get values
	gotValues := form.GetValues()
	assert.Equal(t, "John Doe", gotValues["name"])
	assert.Equal(t, "john@example.com", gotValues["email"])
}

func TestForm_TabNavigation(t *testing.T) {
	form := NewForm("Test Form")
	form.AddField(FormField{Label: "Field 1", Key: "field1", Type: FieldTypeText})
	form.AddField(FormField{Label: "Field 2", Key: "field2", Type: FieldTypeText})
	form.AddField(FormField{Label: "Field 3", Key: "field3", Type: FieldTypeText})
	form.Show()

	assert.Equal(t, 0, form.focusIndex)

	// Tab forward
	form, _ = form.Update(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, 1, form.focusIndex)

	form, _ = form.Update(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, 2, form.focusIndex)

	// Wrap around to first field
	form, _ = form.Update(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, 0, form.focusIndex)

	// Shift+Tab backward
	form, _ = form.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	assert.Equal(t, 2, form.focusIndex)

	form, _ = form.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	assert.Equal(t, 1, form.focusIndex)
}

func TestForm_EscapeKey(t *testing.T) {
	form := NewForm("Test Form")
	form.AddField(FormField{Label: "Name", Key: "name", Type: FieldTypeText})
	form.Show()

	assert.True(t, form.IsVisible())

	// Press ESC
	_, cmd := form.Update(tea.KeyMsg{Type: tea.KeyEsc})

	assert.False(t, form.IsVisible())

	// Check that FormCancelMsg is sent
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(FormCancelMsg)
	assert.True(t, ok, "Expected FormCancelMsg")
}

func TestForm_RequiredFieldValidation(t *testing.T) {
	form := NewForm("Test Form")
	form.AddField(FormField{
		Label:    "Name",
		Key:      "name",
		Type:     FieldTypeText,
		Required: true,
	}).AddField(FormField{
		Label:    "Email",
		Key:      "email",
		Type:     FieldTypeText,
		Required: false,
	})
	form.Show()

	// Submit with empty required field
	_, cmd := form.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should not submit
	assert.Nil(t, cmd)
	assert.NotEmpty(t, form.errors["name"])
	assert.Contains(t, form.errors["name"], "required")

	// Fill in required field
	form.fields[0].textInput.SetValue("John Doe")

	// Submit again
	_, cmd = form.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should submit successfully
	require.NotNil(t, cmd)
	msg := cmd()
	submitMsg, ok := msg.(FormSubmitMsg)
	assert.True(t, ok, "Expected FormSubmitMsg")
	assert.Equal(t, "John Doe", submitMsg.Result.Values["name"])
}

func TestForm_DateFieldValidation(t *testing.T) {
	form := NewForm("Test Form")
	form.AddField(FormField{
		Label:    "Due Date",
		Key:      "due_date",
		Type:     FieldTypeDate,
		Required: true,
	})
	form.Show()

	tests := []struct {
		name        string
		input       string
		shouldError bool
	}{
		{"valid today", "t", false},
		{"valid tomorrow", "tm", false},
		{"valid relative", "+3d", false},
		{"valid weekday", "mon", false},
		{"valid absolute", "2026-12-31", false},
		{"invalid empty", "", true},
		{"invalid format", "invalid", true},
		{"invalid date", "2026-13-45", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form.errors = make(map[string]string)
			form.fields[0].textInput.SetValue(tt.input)

			_, cmd := form.Update(tea.KeyMsg{Type: tea.KeyEnter})

			if tt.shouldError {
				assert.Nil(t, cmd, "Expected validation to fail")
				assert.NotEmpty(t, form.errors["due_date"], "Expected error message")
			} else {
				require.NotNil(t, cmd, "Expected validation to pass")
				msg := cmd()
				_, ok := msg.(FormSubmitMsg)
				assert.True(t, ok, "Expected FormSubmitMsg")
			}
		})
	}
}

func TestForm_OptionalDateField(t *testing.T) {
	form := NewForm("Test Form")
	form.AddField(FormField{
		Label:    "Name",
		Key:      "name",
		Type:     FieldTypeText,
		Required: true,
	}).AddField(FormField{
		Label:    "Due Date",
		Key:      "due_date",
		Type:     FieldTypeDate,
		Required: false,
	})
	form.Show()

	// Set only required field
	form.fields[0].textInput.SetValue("Test Task")

	// Submit with empty optional date field
	_, cmd := form.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should submit successfully
	require.NotNil(t, cmd)
	msg := cmd()
	submitMsg, ok := msg.(FormSubmitMsg)
	assert.True(t, ok)
	assert.Equal(t, "Test Task", submitMsg.Result.Values["name"])
	assert.Equal(t, "", submitMsg.Result.Values["due_date"])
}

func TestForm_CustomValidator(t *testing.T) {
	form := NewForm("Test Form")
	form.AddField(FormField{
		Label:    "Email",
		Key:      "email",
		Type:     FieldTypeText,
		Required: true,
		Validator: func(value string) error {
			if !strings.Contains(value, "@") {
				return fmt.Errorf("invalid email format")
			}
			return nil
		},
	})
	form.Show()

	// Test invalid email
	form.fields[0].textInput.SetValue("notanemail")
	_, cmd := form.Update(tea.KeyMsg{Type: tea.KeyEnter})

	assert.Nil(t, cmd)
	assert.Contains(t, form.errors["email"], "invalid email format")

	// Test valid email
	form.fields[0].textInput.SetValue("test@example.com")
	_, cmd = form.Update(tea.KeyMsg{Type: tea.KeyEnter})

	require.NotNil(t, cmd)
	msg := cmd()
	submitMsg, ok := msg.(FormSubmitMsg)
	assert.True(t, ok)
	assert.Equal(t, "test@example.com", submitMsg.Result.Values["email"])
}

func TestForm_View(t *testing.T) {
	form := NewForm("Create Task")
	form.AddField(FormField{
		Label:       "Task Name",
		Key:         "name",
		Type:        FieldTypeText,
		Required:    true,
		Placeholder: "Enter task name",
	}).AddField(FormField{
		Label:       "Due Date",
		Key:         "due_date",
		Type:        FieldTypeDate,
		Required:    false,
		Placeholder: "t, tm, +3d",
	})

	// Not visible
	view := form.View()
	assert.Empty(t, view)

	// Visible
	form.Show()
	view = form.View()
	assert.Contains(t, view, "Create Task")
	assert.Contains(t, view, "Task Name *")
	assert.Contains(t, view, "Due Date")
	assert.Contains(t, view, "TAB: next field")
	assert.Contains(t, view, "ESC: cancel")
}

func TestForm_ViewWithErrors(t *testing.T) {
	form := NewForm("Test Form")
	form.AddField(FormField{
		Label:    "Name",
		Key:      "name",
		Type:     FieldTypeText,
		Required: true,
	})
	form.Show()

	// Submit empty form to trigger validation error
	form.Update(tea.KeyMsg{Type: tea.KeyEnter})

	view := form.View()
	assert.Contains(t, view, "required")
	assert.Contains(t, view, "✗")
}

func TestForm_SetWidth(t *testing.T) {
	form := NewForm("Test Form")
	form.AddField(FormField{
		Label: "Name",
		Key:   "name",
		Type:  FieldTypeText,
	})

	form.SetWidth(100)
	assert.Equal(t, 100, form.width)
	// Field width should be adjusted
	assert.Equal(t, 80, form.fields[0].textInput.Width) // 100 - 20
}

func TestForm_ParsedDateValue(t *testing.T) {
	form := NewForm("Test Form")
	form.AddField(FormField{
		Label: "Due Date",
		Key:   "due_date",
		Type:  FieldTypeDate,
	})
	form.Show()

	// Set a valid date
	form.fields[0].textInput.SetValue("tm")

	date, err := form.ParsedDateValue("due_date")
	require.NoError(t, err)
	assert.False(t, date.IsZero())

	// Test invalid date
	form.fields[0].textInput.SetValue("invalid")
	_, err = form.ParsedDateValue("due_date")
	assert.Error(t, err)

	// Test empty value
	form.fields[0].textInput.SetValue("")
	date, err = form.ParsedDateValue("due_date")
	require.NoError(t, err)
	assert.True(t, date.IsZero())
}

func TestForm_ParsedDateValueNonExistentField(t *testing.T) {
	form := NewForm("Test Form")
	form.AddField(FormField{
		Label: "Name",
		Key:   "name",
		Type:  FieldTypeText,
	})

	_, err := form.ParsedDateValue("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestForm_DatePreviewUpdate(t *testing.T) {
	form := NewForm("Test Form")
	form.AddField(FormField{
		Label: "Due Date",
		Key:   "due_date",
		Type:  FieldTypeDate,
	})
	form.Show()

	// Initially no preview
	assert.Empty(t, form.datePreview)

	// Set a valid date value
	form.fields[0].textInput.SetValue("tm")
	form.updateDatePreview()

	// Should have preview
	assert.NotEmpty(t, form.datePreview)
	assert.Contains(t, form.datePreview, "tomorrow")

	// Set invalid date
	form.fields[0].textInput.SetValue("invalid")
	form.updateDatePreview()

	// Preview should be cleared
	assert.Empty(t, form.datePreview)
}

func TestForm_UpdateNotVisibleDoesNothing(t *testing.T) {
	form := NewForm("Test Form")
	form.AddField(FormField{
		Label: "Name",
		Key:   "name",
		Type:  FieldTypeText,
	})

	// Form not visible
	assert.False(t, form.IsVisible())

	// Update should do nothing
	updatedForm, cmd := form.Update(tea.KeyMsg{Type: tea.KeyEnter})

	assert.Nil(t, cmd)
	assert.False(t, updatedForm.IsVisible())
}

func TestForm_ClearValuesOnShow(t *testing.T) {
	form := NewForm("Test Form")
	form.AddField(FormField{
		Label: "Name",
		Key:   "name",
		Type:  FieldTypeText,
	})

	// Set a value
	form.fields[0].textInput.SetValue("Old Value")

	// Show should clear values
	form.Show()

	values := form.GetValues()
	assert.Empty(t, values["name"])
}

func TestForm_MultipleSubmissions(t *testing.T) {
	form := NewForm("Test Form")
	form.AddField(FormField{
		Label:    "Name",
		Key:      "name",
		Type:     FieldTypeText,
		Required: true,
	})

	// First submission
	form.Show()
	form.fields[0].textInput.SetValue("First")
	_, cmd := form.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)

	// Second show and submission
	form.Show()
	form.fields[0].textInput.SetValue("Second")
	_, cmd = form.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	submitMsg, ok := msg.(FormSubmitMsg)
	assert.True(t, ok)
	assert.Equal(t, "Second", submitMsg.Result.Values["name"])
}
