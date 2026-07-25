package components

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNotePicker_ShowWithIndexRowSelects (scenario 14): NotePicker.Show
// called with components.IndexRow (the Fork B shared row type — plan.md's
// "Index-row replacement type" section) instead of []*models.Note. Title
// display and selection behave identically to the pre-retarget
// *models.Note-based Show for "the fields that survive: title, an
// id-equivalent" (acceptance-criteria.md §4 scenario 14). Per plan.md line
// 107 ("NotePicker's reads Props["slug"] etc."), the id-equivalent lives in
// Props["slug"], not IndexRow.ID.
//
// Scenario 15 (both pickers converge on one shared row type) is asserted
// structurally by this test, not by a separate one: Show here takes
// []IndexRow, the same row type task_picker.go's Show(...) takes after its
// own Fork B retarget (task_picker_test.go's existing TaskRow-based tests
// are Phase 4's mechanical-churn job, not this file's — see plan.md's "Test
// scenarios" section).
func TestNotePicker_ShowWithIndexRowSelects(t *testing.T) {
	rows := []IndexRow{
		{
			ID:    "note-1",
			Title: "OAuth Flow Patterns",
			Type:  "note",
			Props: map[string]string{"slug": "oauth-flow-patterns"},
		},
		{
			ID:    "note-2",
			Title: "Second Note",
			Type:  "note",
			Props: map[string]string{"slug": "second-note"},
		},
	}

	np := NewNotePicker("Select a note")
	np.Show(rows)

	view := np.View()
	assert.Contains(t, view, "OAuth Flow Patterns")

	_, cmd := np.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	selectMsg, ok := msg.(NotePickerSelectMsg)
	require.True(t, ok, "expected NotePickerSelectMsg, got %T", msg)
	assert.Equal(t, "oauth-flow-patterns", selectMsg.NoteSlug)
	assert.Equal(t, "oauth-flow-patterns", np.GetSelectedNoteSlug())
}
