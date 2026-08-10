package components

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// TextPrompt is a single-line Prompt[string], mirroring DatePicker's struct
// shape minus date parsing. Reused for todo-add's subject step, note-create's
// title step, and rk add's quick-capture prompt.
//
// Required gates empty-submit behavior: when true, Enter on an empty input
// must be blocked (mirrors assembleBody's "subject (first -m) must not be
// empty" rule); when false (rk add's quick-capture line), an empty submit
// must flow through to Done() finished=true with Result()=="" (no
// component-level block), letting the caller's own empty-body guard handle
// it downstream.
//
// STUB: Update/View are not yet implemented. Update currently returns tp
// unchanged with a nil cmd on every message, so tp never reaches a terminal
// state -- a keystroke-driven RunPrompt/Wizard.Run test against a live
// tea.Program will time out via runPromptForTest's bound rather than hang
// forever; a direct tp.Update(...) call in a non-Program test will just
// observe Done() staying (false, false) and Result() staying "".
type TextPrompt struct {
	textInput textinput.Model
	visible   bool
	title     string
	required  bool
	error     string

	submitted bool
	canceled  bool
	result    string
}

// NewTextPrompt creates a new single-line text prompt. required gates
// empty-submit behavior (see type doc).
func NewTextPrompt(title string, required bool) *TextPrompt {
	ti := textinput.New()
	ti.CharLimit = 500
	ti.Width = 50

	return &TextPrompt{
		textInput: ti,
		title:     title,
		required:  required,
	}
}

// Show displays the prompt and focuses the input.
func (tp *TextPrompt) Show() tea.Cmd {
	tp.visible = true
	tp.error = ""
	tp.submitted = false
	tp.canceled = false
	tp.result = ""
	tp.textInput.SetValue("")
	return tp.textInput.Focus()
}

// Hide hides the prompt.
func (tp *TextPrompt) Hide() {
	tp.visible = false
	tp.textInput.Blur()
}

// IsVisible returns whether the prompt is visible.
func (tp *TextPrompt) IsVisible() bool {
	return tp.visible
}

// SetValue pre-fills the input (e.g. a Wizard step factory re-priming from a
// prior result map entry on Esc-back).
func (tp *TextPrompt) SetValue(v string) {
	tp.textInput.SetValue(v)
}

// Init satisfies Prompt[string]. Priming (Show) already happened before a
// TextPrompt is handed to RunPrompt/Wizard, so there is nothing to do here.
func (tp *TextPrompt) Init() tea.Cmd { return nil }

// Result returns the submitted text. Only meaningful once Done() reports
// finished.
func (tp *TextPrompt) Result() string { return tp.result }

// Done reports whether Update has reached a terminal state.
func (tp *TextPrompt) Done() (finished, canceled bool) { return tp.submitted, tp.canceled }

// Update handles Bubble Tea messages.
//
// NOT YET IMPLEMENTED: the real body must mirror DatePicker.Update -- Esc
// cancels, Enter validates (blocking on empty input when required is true,
// else always submitting the trimmed value) -- but is stubbed here to
// compile without prematurely passing any behavioral test.
func (tp *TextPrompt) Update(msg tea.Msg) (Prompt[string], tea.Cmd) {
	return tp, nil
}

// View renders the prompt.
//
// NOT YET IMPLEMENTED.
func (tp *TextPrompt) View() string {
	return ""
}
