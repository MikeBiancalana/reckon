package components

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewTextEntryBar(t *testing.T) {
	teb := NewTextEntryBar()

	if teb == nil {
		t.Fatal("Expected NewTextEntryBar to return non-nil value")
	}

	if teb.mode != ModeInactive {
		t.Errorf("Expected initial mode to be ModeInactive, got %q", teb.mode)
	}

	if teb.IsFocused() {
		t.Error("Expected initial state to be unfocused")
	}

	if teb.GetValue() != "" {
		t.Error("Expected initial value to be empty")
	}
}

func TestEntryModeConstants(t *testing.T) {
	tests := []struct {
		mode     EntryMode
		expected string
	}{
		{ModeInactive, ""},
		{ModeTask, "task"},
		{ModeIntention, "intention"},
		{ModeWin, "win"},
		{ModeLog, "log"},
	}

	for _, tt := range tests {
		if string(tt.mode) != tt.expected {
			t.Errorf("Expected mode %q to equal %q", tt.mode, tt.expected)
		}
	}
}

func TestSetMode(t *testing.T) {
	teb := NewTextEntryBar()

	modes := []EntryMode{ModeTask, ModeIntention, ModeWin, ModeLog, ModeInactive}

	for _, mode := range modes {
		teb.SetMode(mode)
		if teb.mode != mode {
			t.Errorf("Expected mode to be %q, got %q", mode, teb.mode)
		}
	}
}

func TestSetWidth(t *testing.T) {
	teb := NewTextEntryBar()

	widths := []int{80, 120, 160, 200}

	for _, width := range widths {
		teb.SetWidth(width)
		if teb.width != width {
			t.Errorf("Expected width to be %d, got %d", width, teb.width)
		}
	}
}

func TestFocusAndBlur(t *testing.T) {
	teb := NewTextEntryBar()

	// Initially unfocused
	if teb.IsFocused() {
		t.Error("Expected initial state to be unfocused")
	}

	// Focus
	teb.Focus()
	if !teb.IsFocused() {
		t.Error("Expected state to be focused after Focus()")
	}

	// Blur
	teb.Blur()
	if teb.IsFocused() {
		t.Error("Expected state to be unfocused after Blur()")
	}
}

func TestGetValueAndClear(t *testing.T) {
	teb := NewTextEntryBar()
	teb.SetMode(ModeTask)
	teb.Focus()

	// Simulate typing
	teb.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test task")})

	value := teb.GetValue()
	if value != "test task" {
		t.Errorf("Expected value to be 'test task', got %q", value)
	}

	// Clear
	teb.Clear()
	value = teb.GetValue()
	if value != "" {
		t.Errorf("Expected value to be empty after Clear(), got %q", value)
	}
}

func TestViewInactiveState(t *testing.T) {
	teb := NewTextEntryBar()
	teb.SetWidth(80)

	view := teb.View()

	// Should contain the inactive prompt
	expectedPrompt := "Press t (task), i (intention), w (win), L (log), or n (note) to add entry"
	if !strings.Contains(view, expectedPrompt) {
		t.Errorf("Expected inactive view to contain %q, got: %s", expectedPrompt, view)
	}

	// Should contain >
	if !strings.Contains(view, ">") {
		t.Errorf("Expected view to contain '>', got: %s", view)
	}
}

func TestViewActiveState(t *testing.T) {
	tests := []struct {
		mode   EntryMode
		prompt string
	}{
		{ModeTask, "Add task (#tag1 #tag2): "},
		{ModeIntention, "Add intention: "},
		{ModeWin, "Add win: "},
		{ModeLog, "Add log entry: "},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			teb := NewTextEntryBar()
			teb.SetWidth(80)
			teb.SetMode(tt.mode)
			teb.Focus()

			view := teb.View()

			// Should contain the mode-specific prompt
			if !strings.Contains(view, tt.prompt) {
				t.Errorf("Expected view to contain prompt %q, got: %s", tt.prompt, view)
			}

			// Should contain >
			if !strings.Contains(view, ">") {
				t.Errorf("Expected view to contain '>', got: %s", view)
			}
		})
	}
}

func TestViewAlwaysVisible(t *testing.T) {
	// Test that view always returns non-empty string in all states
	teb := NewTextEntryBar()
	teb.SetWidth(80)

	states := []struct {
		name    string
		mode    EntryMode
		focused bool
	}{
		{"inactive unfocused", ModeInactive, false},
		{"task focused", ModeTask, true},
		{"task unfocused", ModeTask, false},
		{"intention focused", ModeIntention, true},
		{"win focused", ModeWin, true},
		{"log focused", ModeLog, true},
	}

	for _, state := range states {
		t.Run(state.name, func(t *testing.T) {
			teb.SetMode(state.mode)
			if state.focused {
				teb.Focus()
			} else {
				teb.Blur()
			}

			view := teb.View()
			if view == "" {
				t.Error("Expected View() to always return non-empty string")
			}
		})
	}
}

func TestUpdateDelegatesWhenFocused(t *testing.T) {
	teb := NewTextEntryBar()
	teb.SetMode(ModeTask)
	teb.Focus()

	// Simulate typing
	updatedTeb, _ := teb.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hello")})

	value := updatedTeb.GetValue()
	if value != "hello" {
		t.Errorf("Expected value to be 'hello', got %q", value)
	}
}

func TestUpdateReturnsNewInstance(t *testing.T) {
	teb := NewTextEntryBar()
	teb.SetMode(ModeTask)
	teb.Focus()

	updatedTeb, _ := teb.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test")})

	// Should return a pointer to the same struct (or a new one)
	if updatedTeb == nil {
		t.Error("Expected Update to return non-nil TextEntryBar")
	}
}

func TestPromptByMode(t *testing.T) {
	tests := []struct {
		mode           EntryMode
		expectedPrompt string
	}{
		{ModeInactive, "Press t (task), i (intention), w (win), L (log), or n (note) to add entry"},
		{ModeTask, "Add task (#tag1 #tag2): "},
		{ModeIntention, "Add intention: "},
		{ModeWin, "Add win: "},
		{ModeLog, "Add log entry: "},
		{ModeNote, "Add note: "},
	}

	teb := NewTextEntryBar()
	teb.SetWidth(80)

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			teb.SetMode(tt.mode)
			if tt.mode != ModeInactive {
				teb.Focus()
			}

			view := teb.View()

			if !strings.Contains(view, tt.expectedPrompt) {
				t.Errorf("Expected view to contain %q for mode %q, got: %s",
					tt.expectedPrompt, tt.mode, view)
			}
		})
	}
}

func TestClearResetsInputValue(t *testing.T) {
	teb := NewTextEntryBar()
	teb.SetMode(ModeTask)
	teb.Focus()

	// Add some text
	teb.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("some text")})

	if teb.GetValue() == "" {
		t.Error("Expected value to be non-empty before Clear()")
	}

	// Clear
	teb.Clear()

	if teb.GetValue() != "" {
		t.Errorf("Expected value to be empty after Clear(), got %q", teb.GetValue())
	}
}

func TestActiveVsInactiveStyle(t *testing.T) {
	teb := NewTextEntryBar()
	teb.SetWidth(80)

	// Inactive view
	teb.SetMode(ModeInactive)
	teb.Blur()
	inactiveView := teb.View()

	// Active view
	teb.SetMode(ModeTask)
	teb.Focus()
	activeView := teb.View()

	// Views should be different (different styling)
	if inactiveView == activeView {
		t.Error("Expected inactive and active views to be different")
	}

	// Both should be non-empty
	if inactiveView == "" || activeView == "" {
		t.Error("Expected both views to be non-empty")
	}
}

func TestWidthAdjustment(t *testing.T) {
	teb := NewTextEntryBar()

	// Test various widths
	widths := []int{60, 80, 120, 160, 200}

	for _, width := range widths {
		teb.SetWidth(width)
		view := teb.View()

		// View should be non-empty for all widths
		if view == "" {
			t.Errorf("Expected non-empty view for width %d", width)
		}
	}
}

func TestFocusReturnsCommand(t *testing.T) {
	teb := NewTextEntryBar()

	cmd := teb.Focus()

	// Focus should return a command (may be nil, but that's ok)
	// We just verify it doesn't panic and returns something
	_ = cmd
}

func TestBlurOnEscape(t *testing.T) {
	teb := NewTextEntryBar()
	teb.SetMode(ModeTask)
	teb.Focus()

	if !teb.IsFocused() {
		t.Fatal("Expected component to be focused before ESC test")
	}

	// Press ESC
	teb.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if teb.IsFocused() {
		t.Error("Expected component to be blurred after ESC key")
	}
}
