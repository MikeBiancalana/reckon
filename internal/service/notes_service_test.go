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

	filePath := createTestNoteFile(t, notesDir, "my-note.md", content)

	// Create the source note
	note := models.NewNote("My Note", "my-note", filePath, nil)
	err := service.SaveNote(note)
	require.NoError(t, err)

	// Update links
	err = service.UpdateNoteLinks(note)
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

	filePath := createTestNoteFile(t, notesDir, "source-note.md", content)

	// Create the source note
	sourceNote := models.NewNote("Source Note", "source-note", filePath, nil)
	err = service.SaveNote(sourceNote)
	require.NoError(t, err)

	// Update links
	err = service.UpdateNoteLinks(sourceNote)
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

	filePath := createTestNoteFile(t, notesDir, "code-note.md", content)

	// Create the source note
	note := models.NewNote("Code Note", "code-note", filePath, nil)
	err := service.SaveNote(note)
	require.NoError(t, err)

	// Update links
	err = service.UpdateNoteLinks(note)
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

	filePath := createTestNoteFile(t, notesDir, "note.md", initialContent)

	// Create and save note
	note := models.NewNote("Note", "note", filePath, nil)
	err := service.SaveNote(note)
	require.NoError(t, err)

	// Update links
	err = service.UpdateNoteLinks(note)
	require.NoError(t, err)

	// Verify initial links
	links, err := service.GetLinksBySourceNote(note.ID)
	require.NoError(t, err)
	assert.Len(t, links, 2)

	// Update the file content
	updatedContent := `# Note

Now links to [[link-three]] and [[link-four]].`

	err = os.WriteFile(filePath, []byte(updatedContent), 0644)
	require.NoError(t, err)

	// Update note metadata
	note.UpdatedAt = time.Now()
	err = service.SaveNote(note)
	require.NoError(t, err)

	// Update links again
	err = service.UpdateNoteLinks(note)
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

	sourceFilePath := createTestNoteFile(t, notesDir, "source-note.md", sourceContent)

	sourceNote := models.NewNote("Source Note", "source-note", sourceFilePath, nil)
	err := service.SaveNote(sourceNote)
	require.NoError(t, err)

	// Update links (target doesn't exist, so target_note_id will be NULL)
	err = service.UpdateNoteLinks(sourceNote)
	require.NoError(t, err)

	// Verify link has NULL target_note_id
	links, err := service.GetLinksBySourceNote(sourceNote.ID)
	require.NoError(t, err)
	assert.Len(t, links, 1)
	assert.Equal(t, "target-note", links[0].TargetSlug)
	assert.Empty(t, links[0].TargetNoteID)

	// Now create the target note
	targetFilePath := createTestNoteFile(t, notesDir, "target-note.md", "# Target Note\n\nContent here.")

	targetNote := models.NewNote("Target Note", "target-note", targetFilePath, nil)
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
		filePath := createTestNoteFile(t, notesDir, filename, content)

		sourceNote := models.NewNote("Source Note "+string(rune('0'+i)), "source-note-"+string(rune('0'+i)), filePath, nil)
		err := service.SaveNote(sourceNote)
		require.NoError(t, err)

		err = service.UpdateNoteLinks(sourceNote)
		require.NoError(t, err)
	}

	// Create the target note
	targetFilePath := createTestNoteFile(t, notesDir, "shared-target.md", "# Shared Target\n\nContent.")

	targetNote := models.NewNote("Shared Target", "shared-target", targetFilePath, nil)
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
	filePath := createTestNoteFile(t, notesDir, "empty-note.md", "")

	note := models.NewNote("Empty Note", "empty-note", filePath, nil)
	err := service.SaveNote(note)
	require.NoError(t, err)

	// Update links (should succeed with no links)
	err = service.UpdateNoteLinks(note)
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

	filePath := createTestNoteFile(t, notesDir, "note.md", content)

	note := models.NewNote("Note", "note", filePath, nil)
	err := service.SaveNote(note)
	require.NoError(t, err)

	// Update links
	err = service.UpdateNoteLinks(note)
	require.NoError(t, err)

	// Verify duplicates were deduplicated
	links, err := service.GetLinksBySourceNote(note.ID)
	require.NoError(t, err)
	assert.Len(t, links, 1)
	assert.Equal(t, "same-note", links[0].TargetSlug)
}

func TestGetAllNotes_Empty(t *testing.T) {
	service, _, tempDir := setupNotesTestService(t)
	defer cleanupNotesTestService(t, tempDir)

	notes, err := service.GetAllNotes()
	require.NoError(t, err)
	assert.Empty(t, notes)
}

func TestGetAllNotes(t *testing.T) {
	service, _, tempDir := setupNotesTestService(t)
	defer cleanupNotesTestService(t, tempDir)

	// Create multiple notes with different timestamps
	note1 := models.NewNote("First Note", "first-note", "/path/to/first.md", []string{"tag1", "tag2"})
	time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	note2 := models.NewNote("Second Note", "second-note", "/path/to/second.md", []string{"tag2", "tag3"})
	time.Sleep(10 * time.Millisecond)
	note3 := models.NewNote("Third Note", "third-note", "/path/to/third.md", []string{"tag1"})

	err := service.SaveNote(note1)
	require.NoError(t, err)
	err = service.SaveNote(note2)
	require.NoError(t, err)
	err = service.SaveNote(note3)
	require.NoError(t, err)

	// Get all notes
	notes, err := service.GetAllNotes()
	require.NoError(t, err)
	assert.Len(t, notes, 3)

	// Verify all three notes are present (repository doesn't enforce order)
	slugs := make([]string, len(notes))
	for i, note := range notes {
		slugs[i] = note.Slug
	}
	assert.ElementsMatch(t, []string{"first-note", "second-note", "third-note"}, slugs)

	// Verify all fields are populated correctly
	for _, note := range notes {
		assert.NotEmpty(t, note.ID)
		assert.NotEmpty(t, note.Title)
		assert.NotEmpty(t, note.Slug)
		assert.NotEmpty(t, note.FilePath)
		assert.NotZero(t, note.CreatedAt)
		assert.NotZero(t, note.UpdatedAt)
	}
}

func TestGetAllNotes_WithTags(t *testing.T) {
	service, _, tempDir := setupNotesTestService(t)
	defer cleanupNotesTestService(t, tempDir)

	// Create notes with various tags
	note1 := models.NewNote("Note 1", "note-1", "/path/to/note1.md", []string{"golang", "testing"})
	note2 := models.NewNote("Note 2", "note-2", "/path/to/note2.md", []string{"python", "django"})
	note3 := models.NewNote("Note 3", "note-3", "/path/to/note3.md", []string{"golang", "python"})
	note4 := models.NewNote("Note 4", "note-4", "/path/to/note4.md", nil)

	for _, note := range []*models.Note{note1, note2, note3, note4} {
		err := service.SaveNote(note)
		require.NoError(t, err)
	}

	// Get all notes
	notes, err := service.GetAllNotes()
	require.NoError(t, err)
	assert.Len(t, notes, 4)

	// Verify tags are correctly retrieved
	tagCounts := make(map[string]int)
	for _, note := range notes {
		for _, tag := range note.Tags {
			tagCounts[tag]++
		}
	}

	assert.Equal(t, 2, tagCounts["golang"])
	assert.Equal(t, 2, tagCounts["python"])
	assert.Equal(t, 1, tagCounts["testing"])
	assert.Equal(t, 1, tagCounts["django"])
}
