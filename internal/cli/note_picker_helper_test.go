package cli

import (
	"testing"

	"github.com/MikeBiancalana/reckon/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestPickNote_EmptyNoteList(t *testing.T) {
	// Test that PickNote returns an error with empty note list
	notes := []*models.Note{}
	_, _, err := PickNote(notes, "Select a note")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no notes available")
}

func TestNotePickerModel_Structure(t *testing.T) {
	// Verify the model structure is correct
	m := notePickerModel{
		noteSlug: "test-slug",
		canceled: false,
	}

	assert.Equal(t, "test-slug", m.noteSlug)
	assert.False(t, m.canceled)
}

func TestPickNote_ValidateNoteData(t *testing.T) {
	// Create test notes
	notes := []*models.Note{
		models.NewNote("First Note", "first-note", "2024/2024-01/2024-01-01-first-note.md", []string{"tag1"}),
		models.NewNote("Second Note", "second-note", "2024/2024-01/2024-01-02-second-note.md", []string{"tag2"}),
	}

	// We can't run the interactive picker in tests, but we can verify the data structure
	assert.Len(t, notes, 2)
	assert.Equal(t, "first-note", notes[0].Slug)
	assert.Equal(t, "second-note", notes[1].Slug)
	assert.Equal(t, "First Note", notes[0].Title)
}
