package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MikeBiancalana/reckon/internal/models"
	"github.com/MikeBiancalana/reckon/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupNotesTestService creates a test service with in-memory database and temp directory.
func setupNotesTestService(t *testing.T) (*NotesService, *NotesRepository, string) {
	t.Helper()

	// Create temp directory for test files
	tempDir, err := os.MkdirTemp("", "notes-test-*")
	require.NoError(t, err)

	// Create notes subdirectory
	notesDir := filepath.Join(tempDir, "notes")
	require.NoError(t, os.MkdirAll(notesDir, 0755))

	// Create database
	dbPath := filepath.Join(tempDir, "test.db")
	db, err := storage.NewDatabase(dbPath)
	require.NoError(t, err)

	// Create service
	repo := NewNotesRepository(db)
	service := NewNotesService(repo)

	return service, repo, tempDir
}

// cleanupNotesTestService cleans up test resources.
func cleanupNotesTestService(t *testing.T, tempDir string) {
	t.Helper()
	os.RemoveAll(tempDir)
}

// createTestNoteFile creates a markdown file with the given content.
func createTestNoteFile(t *testing.T, dir, filename, content string) string {
	t.Helper()

	filePath := filepath.Join(dir, filename)
	err := os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(t, err)

	return filePath
}

func TestSaveNote(t *testing.T) {
	service, _, tempDir := setupNotesTestService(t)
	defer cleanupNotesTestService(t, tempDir)

	note := models.NewNote("Test Note", "test-note", "/path/to/note.md", []string{"tag1", "tag2"})

	err := service.SaveNote(note)
	require.NoError(t, err)

	// Retrieve the note
	retrieved, err := service.GetNoteByID(note.ID)
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	assert.Equal(t, note.ID, retrieved.ID)
	assert.Equal(t, note.Title, retrieved.Title)
	assert.Equal(t, note.Slug, retrieved.Slug)
	assert.Equal(t, note.FilePath, retrieved.FilePath)
	assert.Equal(t, note.Tags, retrieved.Tags)
}

func TestGetNoteBySlug(t *testing.T) {
	service, _, tempDir := setupNotesTestService(t)
	defer cleanupNotesTestService(t, tempDir)

	note := models.NewNote("Test Note", "test-note", "/path/to/note.md", []string{"tag1"})

	err := service.SaveNote(note)
	require.NoError(t, err)

	// Retrieve by slug
	retrieved, err := service.GetNoteBySlug("test-note")
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	assert.Equal(t, note.ID, retrieved.ID)
	assert.Equal(t, "test-note", retrieved.Slug)
}

func TestGetNoteBySlug_NotFound(t *testing.T) {
	service, _, tempDir := setupNotesTestService(t)
	defer cleanupNotesTestService(t, tempDir)

	retrieved, err := service.GetNoteBySlug("non-existent-slug")
	require.NoError(t, err)
	assert.Nil(t, retrieved)
}

func TestUpdateNoteLinks_SimpleLinks(t *testing.T) {
	service, _, tempDir := setupNotesTestService(t)
	defer cleanupNotesTestService(t, tempDir)

	notesDir := filepath.Join(tempDir, "notes")

	// Create a note file with wiki links
	content := `# My Note

This note references [[other-note]] and [[another-note|Another Note]].

See also [[third-note]].`

	createTestNoteFile(t, notesDir, "my-note.md", content)

	// Create the source note with relative path
	note := models.NewNote("My Note", "my-note", "my-note.md", nil)
	err := service.SaveNote(note)
	require.NoError(t, err)

	// Update links
	err = service.UpdateNoteLinks(note, notesDir)
	require.NoError(t, err)

	// Verify links were created
	links, err := service.GetLinksBySourceNote(note.ID)
	require.NoError(t, err)
	assert.Len(t, links, 3)

	// Verify link details
	targetSlugs := make([]string, len(links))
	for i, link := range links {
		targetSlugs[i] = link.TargetSlug
		assert.Equal(t, note.ID, link.SourceNoteID)
		assert.Equal(t, models.LinkTypeReference, link.LinkType)
		assert.Empty(t, link.TargetNoteID) // Target notes don't exist yet
	}

	assert.Contains(t, targetSlugs, "other-note")
	assert.Contains(t, targetSlugs, "another-note")
	assert.Contains(t, targetSlugs, "third-note")
}

func TestUpdateNoteLinks_WithExistingTargets(t *testing.T) {
	service, _, tempDir := setupNotesTestService(t)
	defer cleanupNotesTestService(t, tempDir)

	notesDir := filepath.Join(tempDir, "notes")

	// Create target notes first
	targetNote1 := models.NewNote("Target One", "target-one", "/path/to/target1.md", nil)
	err := service.SaveNote(targetNote1)
	require.NoError(t, err)

	targetNote2 := models.NewNote("Target Two", "target-two", "/path/to/target2.md", nil)
	err = service.SaveNote(targetNote2)
	require.NoError(t, err)

	// Create a note file with links to existing notes
	content := `# Source Note

Links to [[target-one]] and [[target-two]].

Also references [[non-existent-note]].`

	createTestNoteFile(t, notesDir, "source-note.md", content)

	// Create the source note with relative path
	sourceNote := models.NewNote("Source Note", "source-note", "source-note.md", nil)
	err = service.SaveNote(sourceNote)
	require.NoError(t, err)

	// Update links
	err = service.UpdateNoteLinks(sourceNote, notesDir)
	require.NoError(t, err)

	// Verify links
	links, err := service.GetLinksBySourceNote(sourceNote.ID)
	require.NoError(t, err)
	assert.Len(t, links, 3)

	// Check that existing targets have target_note_id set
	for _, link := range links {
		if link.TargetSlug == "target-one" {
			assert.Equal(t, targetNote1.ID, link.TargetNoteID)
		} else if link.TargetSlug == "target-two" {
			assert.Equal(t, targetNote2.ID, link.TargetNoteID)
		} else if link.TargetSlug == "non-existent-note" {
			assert.Empty(t, link.TargetNoteID)
		}
	}
}

func TestUpdateNoteLinks_ExcludesCodeBlocks(t *testing.T) {
	service, _, tempDir := setupNotesTestService(t)
	defer cleanupNotesTestService(t, tempDir)

	notesDir := filepath.Join(tempDir, "notes")

	// Create a note with links in code blocks
	content := "# Note with Code\n\n" +
		"Real link: [[real-link]]\n\n" +
		"```go\n// This [[code-link]] is in a code block\n```\n\n" +
		"Inline `[[inline-link]]` code.\n\n" +
		"Another real [[actual-link]] here."

	createTestNoteFile(t, notesDir, "code-note.md", content)

	// Create the source note with relative path
	note := models.NewNote("Code Note", "code-note", "code-note.md", nil)
	err := service.SaveNote(note)
	require.NoError(t, err)

	// Update links
	err = service.UpdateNoteLinks(note, notesDir)
	require.NoError(t, err)

	// Verify only non-code links were extracted
	links, err := service.GetLinksBySourceNote(note.ID)
	require.NoError(t, err)
	assert.Len(t, links, 2)

	targetSlugs := make([]string, len(links))
	for i, link := range links {
		targetSlugs[i] = link.TargetSlug
	}

	assert.Contains(t, targetSlugs, "real-link")
	assert.Contains(t, targetSlugs, "actual-link")
	assert.NotContains(t, targetSlugs, "code-link")
	assert.NotContains(t, targetSlugs, "inline-link")
}

func TestUpdateNoteLinks_UpdateExistingLinks(t *testing.T) {
	service, _, tempDir := setupNotesTestService(t)
	defer cleanupNotesTestService(t, tempDir)

	notesDir := filepath.Join(tempDir, "notes")

	// Create initial content
	initialContent := `# Note

Links to [[link-one]] and [[link-two]].`

	createTestNoteFile(t, notesDir, "note.md", initialContent)

	// Create and save note with relative path
	note := models.NewNote("Note", "note", "note.md", nil)
	err := service.SaveNote(note)
	require.NoError(t, err)

	// Update links
	err = service.UpdateNoteLinks(note, notesDir)
	require.NoError(t, err)

	// Verify initial links
	links, err := service.GetLinksBySourceNote(note.ID)
	require.NoError(t, err)
	assert.Len(t, links, 2)

	// Update the file content
	updatedContent := `# Note

Now links to [[link-three]] and [[link-four]].`

	notePath := filepath.Join(notesDir, "note.md")
	err = os.WriteFile(notePath, []byte(updatedContent), 0644)
	require.NoError(t, err)

	// Update note metadata
	note.UpdatedAt = time.Now()
	err = service.SaveNote(note)
	require.NoError(t, err)

	// Update links again
	err = service.UpdateNoteLinks(note, notesDir)
	require.NoError(t, err)

	// Verify old links were replaced
	links, err = service.GetLinksBySourceNote(note.ID)
	require.NoError(t, err)
	assert.Len(t, links, 2)

	targetSlugs := make([]string, len(links))
	for i, link := range links {
		targetSlugs[i] = link.TargetSlug
	}

	assert.Contains(t, targetSlugs, "link-three")
	assert.Contains(t, targetSlugs, "link-four")
	assert.NotContains(t, targetSlugs, "link-one")
	assert.NotContains(t, targetSlugs, "link-two")
}

func TestResolveOrphanedBacklinks(t *testing.T) {
	service, _, tempDir := setupNotesTestService(t)
	defer cleanupNotesTestService(t, tempDir)

	notesDir := filepath.Join(tempDir, "notes")

	// Create source note that links to a non-existent target
	sourceContent := `# Source Note

This links to [[target-note]] which doesn't exist yet.`

	createTestNoteFile(t, notesDir, "source-note.md", sourceContent)

	sourceNote := models.NewNote("Source Note", "source-note", "source-note.md", nil)
	err := service.SaveNote(sourceNote)
	require.NoError(t, err)

	// Update links (target doesn't exist, so target_note_id will be NULL)
	err = service.UpdateNoteLinks(sourceNote, notesDir)
	require.NoError(t, err)

	// Verify link has NULL target_note_id
	links, err := service.GetLinksBySourceNote(sourceNote.ID)
	require.NoError(t, err)
	assert.Len(t, links, 1)
	assert.Equal(t, "target-note", links[0].TargetSlug)
	assert.Empty(t, links[0].TargetNoteID)

	// Now create the target note
	createTestNoteFile(t, notesDir, "target-note.md", "# Target Note\n\nContent here.")

	targetNote := models.NewNote("Target Note", "target-note", "target-note.md", nil)
	err = service.SaveNote(targetNote)
	require.NoError(t, err)

	// Resolve orphaned backlinks
	err = service.ResolveOrphanedBacklinks(targetNote)
	require.NoError(t, err)

	// Verify link now has target_note_id set
	links, err = service.GetLinksBySourceNote(sourceNote.ID)
	require.NoError(t, err)
	assert.Len(t, links, 1)
	assert.Equal(t, "target-note", links[0].TargetSlug)
	assert.Equal(t, targetNote.ID, links[0].TargetNoteID)
}

func TestResolveOrphanedBacklinks_MultipleOrphans(t *testing.T) {
	service, _, tempDir := setupNotesTestService(t)
	defer cleanupNotesTestService(t, tempDir)

	notesDir := filepath.Join(tempDir, "notes")

	// Create multiple source notes that link to the same non-existent target
	for i := 1; i <= 3; i++ {
		content := `# Source Note

This links to [[shared-target]].`

		filename := "source-" + string(rune('0'+i)) + ".md"
		createTestNoteFile(t, notesDir, filename, content)

		sourceNote := models.NewNote("Source Note "+string(rune('0'+i)), "source-note-"+string(rune('0'+i)), filename, nil)
		err := service.SaveNote(sourceNote)
		require.NoError(t, err)

		err = service.UpdateNoteLinks(sourceNote, notesDir)
		require.NoError(t, err)
	}

	// Create the target note
	createTestNoteFile(t, notesDir, "shared-target.md", "# Shared Target\n\nContent.")

	targetNote := models.NewNote("Shared Target", "shared-target", "shared-target.md", nil)
	err := service.SaveNote(targetNote)
	require.NoError(t, err)

	// Resolve orphaned backlinks
	err = service.ResolveOrphanedBacklinks(targetNote)
	require.NoError(t, err)

	// Verify all orphaned links were resolved
	// We can't easily query by target, but we can verify each source has the target_note_id set
	for i := 1; i <= 3; i++ {
		sourceNote, err := service.GetNoteBySlug("source-note-" + string(rune('0'+i)))
		require.NoError(t, err)
		require.NotNil(t, sourceNote)

		links, err := service.GetLinksBySourceNote(sourceNote.ID)
		require.NoError(t, err)
		assert.Len(t, links, 1)
		assert.Equal(t, targetNote.ID, links[0].TargetNoteID)
	}
}

func TestUpdateNoteLinks_EmptyFile(t *testing.T) {
	service, _, tempDir := setupNotesTestService(t)
	defer cleanupNotesTestService(t, tempDir)

	notesDir := filepath.Join(tempDir, "notes")

	// Create empty note file
	createTestNoteFile(t, notesDir, "empty-note.md", "")

	note := models.NewNote("Empty Note", "empty-note", "empty-note.md", nil)
	err := service.SaveNote(note)
	require.NoError(t, err)

	// Update links (should succeed with no links)
	err = service.UpdateNoteLinks(note, notesDir)
	require.NoError(t, err)

	// Verify no links were created
	links, err := service.GetLinksBySourceNote(note.ID)
	require.NoError(t, err)
	assert.Empty(t, links)
}

func TestUpdateNoteLinks_DuplicateLinks(t *testing.T) {
	service, _, tempDir := setupNotesTestService(t)
	defer cleanupNotesTestService(t, tempDir)

	notesDir := filepath.Join(tempDir, "notes")

	// Create content with duplicate links
	content := `# Note

Multiple references to [[same-note]] and [[same-note]] again.

Also [[same-note|with display text]].`

	createTestNoteFile(t, notesDir, "note.md", content)

	note := models.NewNote("Note", "note", "note.md", nil)
	err := service.SaveNote(note)
	require.NoError(t, err)

	// Update links
	err = service.UpdateNoteLinks(note, notesDir)
	require.NoError(t, err)

	// Verify duplicates were deduplicated
	links, err := service.GetLinksBySourceNote(note.ID)
	require.NoError(t, err)
	assert.Len(t, links, 1)
	assert.Equal(t, "same-note", links[0].TargetSlug)
}
