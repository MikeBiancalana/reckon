package components

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	textPromptBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("39")).
				Padding(1, 2)

	textPromptTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("39"))

	textPromptErrorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("196")).
				Italic(true)

	textPromptHelpStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240"))
)

// TextPromptSubmitMsg is sent when a value is confirmed with Enter.
type TextPromptSubmitMsg struct {
	Value string
}

// TextPromptCancelMsg is sent when the prompt is cancelled with Esc.
type TextPromptCancelMsg struct{}

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

// Init satisfies Prompt[string]. Priming (Show) already happened before a
// TextPrompt is handed to RunPrompt/Wizard, so there is nothing to do here.
func (tp *TextPrompt) Init() tea.Cmd { return nil }

// Result returns the submitted text. Only meaningful once Done() reports
// finished.
func (tp *TextPrompt) Result() string { return tp.result }

// Done reports whether Update has reached a terminal state.
func (tp *TextPrompt) Done() (finished, canceled bool) { return tp.submitted, tp.canceled }

// Update handles Bubble Tea messages. Mirrors DatePicker.Update's Esc/Enter
// shape: Esc cancels unconditionally; Enter blocks (sets an error, stays
// non-terminal) only when required and the input is blank after trimming,
// otherwise submits the raw (untrimmed) value -- trimming, where wanted, is
// the caller's job (e.g. wizardAddBody), not this component's.
func (tp *TextPrompt) Update(msg tea.Msg) (Prompt[string], tea.Cmd) {
	if !tp.visible {
		return tp, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc:
			tp.Hide()
			tp.canceled = true
			return tp, func() tea.Msg {
				return TextPromptCancelMsg{}
			}

		case tea.KeyEnter:
			value := tp.textInput.Value()
			if tp.required && strings.TrimSpace(value) == "" {
				tp.error = "This field is required"
				return tp, nil
			}

			tp.result = value
			tp.submitted = true
			tp.Hide()
			return tp, func() tea.Msg {
				return TextPromptSubmitMsg{Value: value}
			}
		}
	}

	var cmd tea.Cmd
	tp.textInput, cmd = tp.textInput.Update(msg)
	if tp.textInput.Value() != "" {
		tp.error = ""
	}
	return tp, cmd
}

// View renders the prompt.
func (tp *TextPrompt) View() string {
	if !tp.visible {
		return ""
	}

	var content string
	content += textPromptTitleStyle.Render(tp.title) + "\n\n"
	content += tp.textInput.View() + "\n"

	if tp.error != "" {
		content += textPromptErrorStyle.Render("✗ "+tp.error) + "\n"
	} else {
		content += "\n"
	}

	content += "\n"
	content += textPromptHelpStyle.Render("ESC: cancel  ENTER: confirm")

	return textPromptBoxStyle.Render(content)
}
