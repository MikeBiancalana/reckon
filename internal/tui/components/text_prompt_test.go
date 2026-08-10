package components

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

// Compile-time conformance assertion: TextPrompt must satisfy Prompt[string]
// via its own Update method (mirrors prompt_test.go's var block for the
// other Prompt[T] components).
var _ Prompt[string] = (*TextPrompt)(nil)

// TestTextPrompt_ConformanceCompiles exists so the conformance assertion
// above has a runnable test function to report (mirrors
// TestPrompt_ConformanceCompiles, prompt_test.go).
func TestTextPrompt_ConformanceCompiles(t *testing.T) {}

func TestNewTextPrompt(t *testing.T) {
	tp := NewTextPrompt("Subject", true)
	assert.NotNil(t, tp)
	assert.Equal(t, "Subject", tp.title)
	assert.True(t, tp.required)
	assert.False(t, tp.visible)
}

func TestTextPrompt_ShowFocusesAndResetsState(t *testing.T) {
	tp := NewTextPrompt("Subject", true)
	cmd := tp.Show()

	assert.True(t, tp.visible)
	assert.Equal(t, "", tp.textInput.Value())
	// Show's returned cmd focuses the underlying textinput (mirrors
	// DatePicker.Show/Form.Show, which both return textInput.Focus()).
	assert.NotNil(t, cmd)
}

// TestTextPrompt_RequiredBlocksEmptySubmit: with Required=true, an Enter on
// an empty input must NOT reach a terminal state (finished stays
// false) -- mirrors DatePicker's "Please enter a date" block
// (date_picker.go:138-141) applied to plain text instead of a date. A
// second Enter after typing a non-empty value must then submit with that
// value.
func TestTextPrompt_RequiredBlocksEmptySubmit(t *testing.T) {
	tp := NewTextPrompt("Subject", true)
	tp.Show()

	tp.Update(tea.KeyMsg{Type: tea.KeyEnter})
	finished, canceled := tp.Done()
	if finished || canceled {
		t.Fatalf("Required=true, empty input, Enter: Done() = (%v, %v), want (false, false) -- blocked, not terminal", finished, canceled)
	}

	tp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Buy milk")})
	tp.Update(tea.KeyMsg{Type: tea.KeyEnter})
	finished, canceled = tp.Done()
	if !finished || canceled {
		t.Fatalf("Required=true, non-empty input, Enter: Done() = (%v, %v), want (true, false)", finished, canceled)
	}
	if got := tp.Result(); got != "Buy milk" {
		t.Errorf("Result() = %q, want %q", got, "Buy milk")
	}
}

// TestTextPrompt_NonRequiredAllowsEmptySubmit: with Required=false (rk
// add's quick-capture case), Enter on an empty input submits
// immediately with Result()=="" -- no component-level block, letting the
// caller's own empty-body guard (runAddE's existing check) handle it
// downstream.
func TestTextPrompt_NonRequiredAllowsEmptySubmit(t *testing.T) {
	tp := NewTextPrompt("Quick capture", false)
	tp.Show()

	tp.Update(tea.KeyMsg{Type: tea.KeyEnter})
	finished, canceled := tp.Done()
	if !finished || canceled {
		t.Fatalf("Required=false, empty input, Enter: Done() = (%v, %v), want (true, false)", finished, canceled)
	}
	if got := tp.Result(); got != "" {
		t.Errorf("Result() = %q, want empty", got)
	}
}

func TestTextPrompt_EnterSubmitsTypedValue(t *testing.T) {
	tp := NewTextPrompt("Title", true)
	tp.Show()

	tp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("PAS Entity Model")})
	_, cmd := tp.Update(tea.KeyMsg{Type: tea.KeyEnter})

	finished, canceled := tp.Done()
	if !finished || canceled {
		t.Fatalf("Done() = (%v, %v), want (true, false) after Enter with typed text", finished, canceled)
	}
	if got := tp.Result(); got != "PAS Entity Model" {
		t.Errorf("Result() = %q, want %q", got, "PAS Entity Model")
	}
	if cmd == nil {
		t.Error("expected a non-nil submit cmd (mirrors DatePickerSubmitMsg/TextEditorSubmitMsg's self-signal convention)")
	}
}

func TestTextPrompt_EscCancels(t *testing.T) {
	tp := NewTextPrompt("Subject", true)
	tp.Show()

	tp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("partial")})
	tp.Update(tea.KeyMsg{Type: tea.KeyEsc})

	finished, canceled := tp.Done()
	if finished || !canceled {
		t.Fatalf("Done() = (%v, %v), want (false, true) after Esc", finished, canceled)
	}
}
