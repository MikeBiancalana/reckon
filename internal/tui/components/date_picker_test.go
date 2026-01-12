package components

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewDatePicker(t *testing.T) {
	dp := NewDatePicker("Schedule Task")

	if dp == nil {
		t.Fatal("Expected NewDatePicker to return non-nil value")
	}

	if dp.visible {
		t.Error("Expected initial state to be hidden")
	}

	if dp.title != "Schedule Task" {
		t.Errorf("Expected title to be 'Schedule Task', got %q", dp.title)
	}

	if dp.GetValue() != "" {
		t.Error("Expected initial value to be empty")
	}
}

func TestDatePickerShowHide(t *testing.T) {
	dp := NewDatePicker("Test")

	// Initially hidden
	if dp.IsVisible() {
		t.Error("Expected date picker to be hidden initially")
	}

	// Show
	dp.Show()
	if !dp.IsVisible() {
		t.Error("Expected date picker to be visible after Show()")
	}

	// Hide
	dp.Hide()
	if dp.IsVisible() {
		t.Error("Expected date picker to be hidden after Hide()")
	}
}

func TestDatePickerShowClearsState(t *testing.T) {
	dp := NewDatePicker("Test")

	// Set some state
	dp.textInput.SetValue("test")
	dp.error = "some error"
	dp.preview = "some preview"

	// Show should clear state
	dp.Show()

	if dp.GetValue() != "" {
		t.Error("Expected Show() to clear input value")
	}

	if dp.error != "" {
		t.Error("Expected Show() to clear error")
	}

	if dp.preview != "" {
		t.Error("Expected Show() to clear preview")
	}
}

func TestDatePickerGetValue(t *testing.T) {
	dp := NewDatePicker("Test")
	dp.Show()

	// Simulate typing
	dp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2025-01-15")})

	value := dp.GetValue()
	if value != "2025-01-15" {
		t.Errorf("Expected value to be '2025-01-15', got %q", value)
	}
}

func TestDatePickerSetWidth(t *testing.T) {
	dp := NewDatePicker("Test")

	dp.SetWidth(80)
	if dp.width != 80 {
		t.Errorf("Expected width to be 80, got %d", dp.width)
	}
}

func TestDatePickerUpdateEsc(t *testing.T) {
	dp := NewDatePicker("Test")
	dp.Show()

	if !dp.IsVisible() {
		t.Fatal("Expected date picker to be visible")
	}

	// Press ESC
	dp.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if dp.IsVisible() {
		t.Error("Expected date picker to be hidden after ESC")
	}
}

func TestDatePickerUpdateEnterValid(t *testing.T) {
	dp := NewDatePicker("Test")
	dp.Show()

	// Type valid date
	dp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2025-01-15")})

	// Press Enter
	dp.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should have no error
	if dp.error != "" {
		t.Errorf("Expected no error for valid date, got: %s", dp.error)
	}
}

func TestDatePickerUpdateEnterInvalid(t *testing.T) {
	dp := NewDatePicker("Test")
	dp.Show()

	// Type invalid date
	dp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("invalid")})

	// Press Enter
	dp.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should have error
	if dp.error == "" {
		t.Error("Expected error for invalid date")
	}
}

func TestDatePickerUpdateEnterEmpty(t *testing.T) {
	dp := NewDatePicker("Test")
	dp.Show()

	// Press Enter without typing anything
	dp.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should have error
	if dp.error == "" {
		t.Error("Expected error for empty input")
	}

	if !strings.Contains(dp.error, "enter a date") {
		t.Errorf("Expected error message about entering a date, got: %s", dp.error)
	}
}

func TestDatePickerPreviewAbsoluteDate(t *testing.T) {
	dp := NewDatePicker("Test")
	dp.Show()

	// Type absolute date
	dp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2025-01-15")})

	// Should have preview
	if dp.preview == "" {
		t.Error("Expected preview for valid date")
	}

	if !strings.Contains(dp.preview, "2025-01-15") {
		t.Errorf("Expected preview to contain date '2025-01-15', got: %s", dp.preview)
	}
}

func TestDatePickerPreviewRelativeDate(t *testing.T) {
	dp := NewDatePicker("Test")
	dp.Show()

	// Type relative date
	dp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("tomorrow")})

	// Should have preview
	if dp.preview == "" {
		t.Error("Expected preview for valid relative date")
	}

	if !strings.Contains(dp.preview, "tomorrow") {
		t.Errorf("Expected preview to contain 'tomorrow', got: %s", dp.preview)
	}
}

func TestDatePickerPreviewInvalidDate(t *testing.T) {
	dp := NewDatePicker("Test")
	dp.Show()

	// Type invalid date
	dp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("invalid")})

	// Should have error, no preview
	if dp.error == "" {
		t.Error("Expected error for invalid date")
	}

	if dp.preview != "" {
		t.Error("Expected no preview for invalid date")
	}
}

func TestDatePickerViewWhenHidden(t *testing.T) {
	dp := NewDatePicker("Test")

	view := dp.View()

	if view != "" {
		t.Error("Expected empty view when hidden")
	}
}

func TestDatePickerViewWhenVisible(t *testing.T) {
	dp := NewDatePicker("Test")
	dp.Show()

	view := dp.View()

	if view == "" {
		t.Error("Expected non-empty view when visible")
	}

	// Should contain title
	if !strings.Contains(view, "Test") {
		t.Error("Expected view to contain title")
	}

	// Should contain help text
	if !strings.Contains(view, "ESC") || !strings.Contains(view, "ENTER") {
		t.Error("Expected view to contain help text")
	}
}

func TestDatePickerViewWithPreview(t *testing.T) {
	dp := NewDatePicker("Test")
	dp.Show()

	// Type valid date
	dp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("tomorrow")})

	view := dp.View()

	// Should contain preview arrow
	if !strings.Contains(view, "→") {
		t.Error("Expected view to contain preview arrow")
	}
}

func TestDatePickerViewWithError(t *testing.T) {
	dp := NewDatePicker("Test")
	dp.Show()

	// Type invalid date
	dp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("invalid")})

	view := dp.View()

	// Should contain error indicator
	if !strings.Contains(view, "✗") {
		t.Error("Expected view to contain error indicator")
	}
}

func TestDatePickerParsedDate(t *testing.T) {
	dp := NewDatePicker("Test")
	dp.Show()

	// Type valid absolute date
	dp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2025-01-15")})

	date, err := dp.ParsedDate()

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	if date.Year() != 2025 || date.Month() != time.January || date.Day() != 15 {
		t.Errorf("Expected date 2025-01-15, got %v", date)
	}
}

func TestDatePickerParsedDateEmpty(t *testing.T) {
	dp := NewDatePicker("Test")
	dp.Show()

	// Don't type anything
	date, err := dp.ParsedDate()

	if err != nil {
		t.Errorf("Expected no error for empty input, got: %v", err)
	}

	if !date.IsZero() {
		t.Error("Expected zero time for empty input")
	}
}

func TestDatePickerParsedDateInvalid(t *testing.T) {
	dp := NewDatePicker("Test")
	dp.Show()

	// Type invalid date
	dp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("invalid")})

	_, err := dp.ParsedDate()

	if err == nil {
		t.Error("Expected error for invalid date")
	}
}

func TestDatePickerUpdateWhenHidden(t *testing.T) {
	dp := NewDatePicker("Test")

	// Should not process updates when hidden
	dp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test")})

	if dp.GetValue() != "" {
		t.Error("Expected no input to be processed when hidden")
	}
}

func TestDatePickerMultipleShowHideCycles(t *testing.T) {
	dp := NewDatePicker("Test")

	// Cycle 1
	dp.Show()
	dp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2025-01-15")})
	if dp.GetValue() != "2025-01-15" {
		t.Error("Expected input to be stored")
	}
	dp.Hide()

	// Cycle 2 - should be cleared
	dp.Show()
	if dp.GetValue() != "" {
		t.Error("Expected input to be cleared after Show()")
	}
}

func TestDatePickerUpdatePreview(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectError   bool
		expectPreview bool
	}{
		{
			name:          "empty input",
			input:         "",
			expectError:   false,
			expectPreview: false,
		},
		{
			name:          "valid absolute date",
			input:         "2025-01-15",
			expectError:   false,
			expectPreview: true,
		},
		{
			name:          "valid relative date",
			input:         "tomorrow",
			expectError:   false,
			expectPreview: true,
		},
		{
			name:          "invalid date",
			input:         "invalid",
			expectError:   true,
			expectPreview: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dp := NewDatePicker("Test")
			dp.Show()

			if tt.input != "" {
				dp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.input)})
			}

			if tt.expectError && dp.error == "" {
				t.Error("Expected error but got none")
			}

			if !tt.expectError && dp.error != "" {
				t.Errorf("Expected no error but got: %s", dp.error)
			}

			if tt.expectPreview && dp.preview == "" {
				t.Error("Expected preview but got none")
			}

			if !tt.expectPreview && dp.preview != "" {
				t.Errorf("Expected no preview but got: %s", dp.preview)
			}
		})
	}
}
