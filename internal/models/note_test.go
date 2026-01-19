package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewNote(t *testing.T) {
	note := NewNote("Test Title", "test-slug", "/path/to/note.md", []string{"tag1", "tag2"})

	assert.NotEmpty(t, note.ID)
	assert.Equal(t, "Test Title", note.Title)
	assert.Equal(t, "test-slug", note.Slug)
	assert.Equal(t, "/path/to/note.md", note.FilePath)
	assert.Equal(t, []string{"tag1", "tag2"}, note.Tags)
	assert.False(t, note.CreatedAt.IsZero())
	assert.False(t, note.UpdatedAt.IsZero())
}

func TestNewNoteLink(t *testing.T) {
	link := NewNoteLink("source-id", "target-slug", LinkTypeReference)

	assert.NotEmpty(t, link.ID)
	assert.Equal(t, "source-id", link.SourceNoteID)
	assert.Equal(t, "target-slug", link.TargetSlug)
	assert.Equal(t, LinkTypeReference, link.LinkType)
	assert.False(t, link.CreatedAt.IsZero())
}

func TestNote_UpdateTitle(t *testing.T) {
	note := NewNote("Original Title", "test-slug", "/path/to/note.md", nil)
	originalUpdatedAt := note.UpdatedAt

	time.Sleep(time.Millisecond)
	note.UpdateTitle("New Title")

	assert.Equal(t, "New Title", note.Title)
	assert.True(t, note.UpdatedAt.After(originalUpdatedAt))
}

func TestNote_AddTag(t *testing.T) {
	note := NewNote("Test", "test", "/path", nil)

	note.AddTag("new-tag")
	assert.Contains(t, note.Tags, "new-tag")

	note.AddTag("new-tag")
	assert.Len(t, note.Tags, 1)
}

func TestNote_RemoveTag(t *testing.T) {
	note := NewNote("Test", "test", "/path", []string{"tag1", "tag2", "tag3"})

	note.RemoveTag("tag2")
	assert.NotContains(t, note.Tags, "tag2")
	assert.Contains(t, note.Tags, "tag1")
	assert.Contains(t, note.Tags, "tag3")
}

func TestNoteLink_UpdateTargetNoteID(t *testing.T) {
	link := NewNoteLink("source", "target", LinkTypeReference)

	assert.Empty(t, link.TargetNoteID)

	link.UpdateTargetNoteID("new-target-id")

	assert.Equal(t, "new-target-id", link.TargetNoteID)
}

func TestLinkType_Values(t *testing.T) {
	assert.Equal(t, LinkType("reference"), LinkTypeReference)
	assert.Equal(t, LinkType("parent"), LinkTypeParent)
	assert.Equal(t, LinkType("child"), LinkTypeChild)
	assert.Equal(t, LinkType("related"), LinkTypeRelated)
}
