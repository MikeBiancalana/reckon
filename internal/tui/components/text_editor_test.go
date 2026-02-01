package components

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestNewTextEditor(t *testing.T) {
	editor := NewTextEditor("Test Editor")
	assert.NotNil(t, editor)
	assert.Equal(t, "Test Editor", editor.title)
	assert.False(t, editor.IsVisible())
}

func TestTextEditorShowHide(t *testing.T) {
	editor := NewTextEditor("Test Editor")

	// Initially hidden
	assert.False(t, editor.IsVisible())

	// Show
	editor.Show()
	assert.True(t, editor.IsVisible())

	// Hide
	editor.Hide()
	assert.False(t, editor.IsVisible())
}

func TestTextEditorSetGetText(t *testing.T) {
	editor := NewTextEditor("Test Editor")

	// Set text
	testText := "This is a test note\nWith multiple lines"
	editor.SetText(testText)

	// Get text
	assert.Equal(t, testText, editor.GetText())
}

func TestTextEditorSetSize(t *testing.T) {
	editor := NewTextEditor("Test Editor")

	// Set size
	editor.SetSize(100, 20)
	assert.Equal(t, 100, editor.width)
	assert.Equal(t, 20, editor.height)
}

func TestTextEditorSubmit(t *testing.T) {
	editor := NewTextEditor("Test Editor")
	editor.Show()
	editor.SetText("Test note content")

	// Simulate Ctrl+D (submit)
	msg := tea.KeyMsg{Type: tea.KeyCtrlD}
	updatedEditor, cmd := editor.Update(msg)

	// Should hide after submit
	assert.False(t, updatedEditor.IsVisible())

	// Should return submit message
	assert.NotNil(t, cmd)
	result := cmd()
	submitMsg, ok := result.(TextEditorSubmitMsg)
	assert.True(t, ok)
	assert.Equal(t, "Test note content", submitMsg.Text)
}

func TestTextEditorCancel(t *testing.T) {
	editor := NewTextEditor("Test Editor")
	editor.Show()
	editor.SetText("Some content")

	// Simulate ESC (cancel)
	msg := tea.KeyMsg{Type: tea.KeyEsc}
	updatedEditor, cmd := editor.Update(msg)

	// Should hide after cancel
	assert.False(t, updatedEditor.IsVisible())

	// Should return cancel message
	assert.NotNil(t, cmd)
	result := cmd()
	_, ok := result.(TextEditorCancelMsg)
	assert.True(t, ok)
}

func TestTextEditorView(t *testing.T) {
	editor := NewTextEditor("Test Editor")

	// Hidden editor should return empty view
	assert.Equal(t, "", editor.View())

	// Visible editor should return non-empty view
	editor.Show()
	view := editor.View()
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "Test Editor")
	assert.Contains(t, view, "CTRL+D: submit")
	assert.Contains(t, view, "ESC: cancel")
}

func TestTextEditorUpdateWhenHidden(t *testing.T) {
	editor := NewTextEditor("Test Editor")

	// Updates should be ignored when hidden
	msg := tea.KeyMsg{Type: tea.KeyCtrlD}
	updatedEditor, cmd := editor.Update(msg)

	assert.False(t, updatedEditor.IsVisible())
	assert.Nil(t, cmd)
}
