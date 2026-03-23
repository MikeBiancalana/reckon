package cli

import (
	"testing"

	"github.com/MikeBiancalana/reckon/internal/tui/components"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func newTestLogMultilineModel() logMultilineModel {
	editor := components.NewTextEditor("Add Log Entry")
	editor.SetSize(80, 15)
	return logMultilineModel{
		editor: editor,
		width:  80,
		height: 24,
	}
}

func TestLogMultilineModel_Structure(t *testing.T) {
	m := newTestLogMultilineModel()
	assert.NotNil(t, m.editor)
	assert.Empty(t, m.message)
	assert.False(t, m.canceled)
	assert.Equal(t, 80, m.width)
	assert.Equal(t, 24, m.height)
}

func TestLogMultilineModel_SubmitTransition(t *testing.T) {
	m := newTestLogMultilineModel()
	msg := components.TextEditorSubmitMsg{Text: "line1\nline2\nline3"}
	updatedModel, _ := m.Update(msg)
	result := updatedModel.(logMultilineModel)

	assert.Equal(t, "line1 line2 line3", result.message)
	assert.False(t, result.canceled)
}

func TestLogMultilineModel_SubmitTrimsWhitespace(t *testing.T) {
	m := newTestLogMultilineModel()
	msg := components.TextEditorSubmitMsg{Text: "  hello world  \n  "}
	updatedModel, _ := m.Update(msg)
	result := updatedModel.(logMultilineModel)

	assert.Equal(t, "hello world", result.message)
	assert.False(t, result.canceled)
}

func TestLogMultilineModel_CancelFromEditor(t *testing.T) {
	m := newTestLogMultilineModel()
	msg := components.TextEditorCancelMsg{}
	updatedModel, _ := m.Update(msg)
	result := updatedModel.(logMultilineModel)

	assert.True(t, result.canceled)
	assert.Empty(t, result.message)
}

func TestLogMultilineModel_CtrlCCancel(t *testing.T) {
	m := newTestLogMultilineModel()
	msg := tea.KeyMsg{Type: tea.KeyCtrlC}
	updatedModel, _ := m.Update(msg)
	result := updatedModel.(logMultilineModel)

	assert.True(t, result.canceled)
	assert.Empty(t, result.message)
}

func TestLogMultilineModel_WindowResize(t *testing.T) {
	m := newTestLogMultilineModel()
	msg := tea.WindowSizeMsg{Width: 120, Height: 40}
	updatedModel, _ := m.Update(msg)
	result := updatedModel.(logMultilineModel)

	assert.Equal(t, 120, result.width)
	assert.Equal(t, 40, result.height)
}

func TestLogMultilineModel_View_WhenDone(t *testing.T) {
	m := newTestLogMultilineModel()
	m.message = "some entry"
	view := m.View()
	assert.Empty(t, view)
}

func TestLogMultilineModel_View_WhenCancelled(t *testing.T) {
	m := newTestLogMultilineModel()
	m.canceled = true
	view := m.View()
	assert.Empty(t, view)
}

func TestLogMultilineModel_View_WhenActive(t *testing.T) {
	m := newTestLogMultilineModel()
	// Editor must be shown to have visible content
	m.editor.Show() //nolint
	view := m.View()
	assert.NotEmpty(t, view)
}

func TestJoinLogLines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"single line", "hello world", "hello world"},
		{"two lines", "line1\nline2", "line1 line2"},
		{"three lines", "line1\nline2\nline3", "line1 line2 line3"},
		{"trailing newline", "hello\n", "hello"},
		{"leading newline", "\nhello", "hello"},
		{"whitespace only", "   \n  ", ""},
		{"meeting syntax", "[meeting:standup]\ndiscussed progress", "[meeting:standup] discussed progress"},
		{"task ref", "worked on [task:abc-123]\nfixed the bug", "worked on [task:abc-123] fixed the bug"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := joinLogLines(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
