package components

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	textEditorBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("39")).
				Padding(1, 2)

	textEditorTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("39"))

	textEditorHelpStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240"))
)

// TextEditorSubmitMsg is sent when the editor content is submitted
type TextEditorSubmitMsg struct {
	Text string
}

// TextEditorCancelMsg is sent when the editor is cancelled
type TextEditorCancelMsg struct{}

// TextEditor is a multiline text editor component
type TextEditor struct {
	textarea textarea.Model
	title    string
	visible  bool
	width    int
	height   int
}

// NewTextEditor creates a new text editor component
func NewTextEditor(title string) *TextEditor {
	ta := textarea.New()
	ta.Placeholder = "Type your text here..."
	ta.ShowLineNumbers = false
	ta.CharLimit = 0 // No character limit

	return &TextEditor{
		textarea: ta,
		title:    title,
		visible:  false,
		width:    80,
		height:   10,
	}
}

// Show displays the text editor
func (te *TextEditor) Show() tea.Cmd {
	te.visible = true
	te.textarea.SetValue("")
	return te.textarea.Focus()
}

// Hide hides the text editor
func (te *TextEditor) Hide() {
	te.visible = false
	te.textarea.Blur()
}

// IsVisible returns whether the text editor is visible
func (te *TextEditor) IsVisible() bool {
	return te.visible
}

// SetSize sets the width and height of the text editor
func (te *TextEditor) SetSize(width, height int) {
	te.width = width
	te.height = height

	// Account for borders and padding
	taWidth := width - 10
	taHeight := height - 8

	if taWidth < 40 {
		taWidth = 40
	}
	if taHeight < 3 {
		taHeight = 3
	}

	te.textarea.SetWidth(taWidth)
	te.textarea.SetHeight(taHeight)
}

// GetText returns the current text content
func (te *TextEditor) GetText() string {
	return te.textarea.Value()
}

// SetText sets the text content
func (te *TextEditor) SetText(text string) {
	te.textarea.SetValue(text)
}

// Update handles Bubble Tea messages
func (te *TextEditor) Update(msg tea.Msg) (*TextEditor, tea.Cmd) {
	if !te.visible {
		return te, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc:
			te.Hide()
			return te, func() tea.Msg {
				return TextEditorCancelMsg{}
			}

		case tea.KeyCtrlD:
			// Ctrl+D submits the text
			text := strings.TrimSpace(te.textarea.Value())
			te.Hide()
			return te, func() tea.Msg {
				return TextEditorSubmitMsg{Text: text}
			}
		}
	}

	// Update textarea
	var cmd tea.Cmd
	te.textarea, cmd = te.textarea.Update(msg)
	return te, cmd
}

// View renders the text editor
func (te *TextEditor) View() string {
	if !te.visible {
		return ""
	}

	var content strings.Builder

	// Title
	content.WriteString(textEditorTitleStyle.Render(te.title))
	content.WriteString("\n\n")

	// Textarea
	content.WriteString(te.textarea.View())
	content.WriteString("\n\n")

	// Help text
	helpText := "CTRL+D: submit  ESC: cancel"
	content.WriteString(textEditorHelpStyle.Render(helpText))

	// Wrap in box
	return textEditorBoxStyle.Render(content.String())
}
