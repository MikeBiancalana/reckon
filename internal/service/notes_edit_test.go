package service

import (
	"testing"
	"time"

	"github.com/MikeBiancalana/reckon/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAllNotes(t *testing.T) {
	service, _, tempDir := setupNotesTestService(t)
	defer cleanupNotesTestService(t, tempDir)

	// Create multiple notes
	note1 := models.NewNote("Note 1", "note-1", "2024/2024-01/2024-01-01-note-1.md", []string{"tag1"})
	note2 := models.NewNote("Note 2", "note-2", "2024/2024-01/2024-01-02-note-2.md", []string{"tag2"})
	note3 := models.NewNote("Note 3", "note-3", "2024/2024-01/2024-01-03-note-3.md", []string{"tag1", "tag2"})

	require.NoError(t, service.SaveNote(note1))
	require.NoError(t, service.SaveNote(note2))
	require.NoError(t, service.SaveNote(note3))

	// Get all notes
	notes, err := service.GetAllNotes()
	require.NoError(t, err)
	require.Len(t, notes, 3)

	// Verify notes are returned (order may vary due to ORDER BY created_at DESC)
	slugs := make(map[string]bool)
	for _, n := range notes {
		slugs[n.Slug] = true
	}

	assert.True(t, slugs["note-1"])
	assert.True(t, slugs["note-2"])
	assert.True(t, slugs["note-3"])
}

func TestGetAllNotes_Empty(t *testing.T) {
	service, _, tempDir := setupNotesTestService(t)
	defer cleanupNotesTestService(t, tempDir)

	// Get all notes when database is empty
	notes, err := service.GetAllNotes()
	require.NoError(t, err)
	assert.Len(t, notes, 0)
}

func TestUpdateNoteTimestamp(t *testing.T) {
	service, _, tempDir := setupNotesTestService(t)
	defer cleanupNotesTestService(t, tempDir)

	// Create a note with a timestamp in the past
	note := models.NewNote("Test Note", "test-note", "2024/2024-01/2024-01-01-test-note.md", []string{"tag1"})
	pastTime := time.Now().Add(-1 * time.Hour)
	note.CreatedAt = pastTime
	note.UpdatedAt = pastTime

	require.NoError(t, service.SaveNote(note))

	// Update timestamp
	err := service.UpdateNoteTimestamp(note)
	require.NoError(t, err)

	// Verify the in-memory note was updated
	assert.True(t, note.UpdatedAt.After(pastTime))
	assert.Equal(t, pastTime.Unix(), note.CreatedAt.Unix())

	// Verify the database was updated
	retrieved, err := service.GetNoteByID(note.ID)
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	assert.True(t, retrieved.UpdatedAt.After(pastTime))
	assert.Equal(t, pastTime.Unix(), retrieved.CreatedAt.Unix())
}

func TestUpdateNoteTimestamp_NonExistent(t *testing.T) {
	service, _, tempDir := setupNotesTestService(t)
	defer cleanupNotesTestService(t, tempDir)

	// Try to update timestamp for a non-existent note
	note := models.NewNote("Ghost Note", "ghost-note", "path/to/ghost.md", nil)

	// This should not error - SQLite allows UPDATE with no matches
	err := service.UpdateNoteTimestamp(note)
	require.NoError(t, err)
}

func TestGetAllNotes_PreservesMetadata(t *testing.T) {
	service, _, tempDir := setupNotesTestService(t)
	defer cleanupNotesTestService(t, tempDir)

	// Create a note with specific metadata
	now := time.Now()
	note := models.NewNote("Test Note", "test-note", "2024/2024-01/2024-01-01-test-note.md", []string{"tag1", "tag2"})
	note.CreatedAt = now
	note.UpdatedAt = now.Add(1 * time.Hour)

	require.NoError(t, service.SaveNote(note))

	// Get all notes
	notes, err := service.GetAllNotes()
	require.NoError(t, err)
	require.Len(t, notes, 1)

	retrieved := notes[0]
	assert.Equal(t, note.ID, retrieved.ID)
	assert.Equal(t, note.Title, retrieved.Title)
	assert.Equal(t, note.Slug, retrieved.Slug)
	assert.Equal(t, note.FilePath, retrieved.FilePath)
	assert.Equal(t, note.Tags, retrieved.Tags)
	assert.Equal(t, now.Unix(), retrieved.CreatedAt.Unix())
	assert.Equal(t, now.Add(1*time.Hour).Unix(), retrieved.UpdatedAt.Unix())
}

func TestGetAllNotes_WithNoTags(t *testing.T) {
	service, _, tempDir := setupNotesTestService(t)
	defer cleanupNotesTestService(t, tempDir)

	// Create a note with no tags
	note := models.NewNote("No Tags Note", "no-tags", "2024/2024-01/2024-01-01-no-tags.md", nil)
	require.NoError(t, service.SaveNote(note))

	// Get all notes
	notes, err := service.GetAllNotes()
	require.NoError(t, err)
	require.Len(t, notes, 1)

	assert.Equal(t, "no-tags", notes[0].Slug)
	assert.Nil(t, notes[0].Tags)
}
