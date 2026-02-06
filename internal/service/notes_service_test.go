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
	filePath := createTestNoteFile(t, notesDir, "my-note.md", content)
	absFilePath := createTestNoteFile(t, notesDir, "my-note.md", content)

	// Create the source note with relative path
	note := models.NewNote("My Note", "my-note", "my-note.md", nil)
	err := service.SaveNote(note)
	// Create the source note
	note := models.NewNote("My Note", "my-note", filePath, nil)
	err := service.SaveNote(note)
	// Create the source note with relative path
	relFilePath, err := filepath.Rel(notesDir, absFilePath)
	require.NoError(t, err)
	note := models.NewNote("My Note", "my-note", relFilePath, nil)
	err = service.SaveNote(note)
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
	filePath := createTestNoteFile(t, notesDir, "source-note.md", content)
	absFilePath := createTestNoteFile(t, notesDir, "source-note.md", content)

	// Create the source note with relative path
	sourceNote := models.NewNote("Source Note", "source-note", "source-note.md", nil)
	// Create the source note
	sourceNote := models.NewNote("Source Note", "source-note", filePath, nil)
	// Create the source note with relative path
	relFilePath, err := filepath.Rel(notesDir, absFilePath)
	require.NoError(t, err)
	sourceNote := models.NewNote("Source Note", "source-note", relFilePath, nil)
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
	filePath := createTestNoteFile(t, notesDir, "code-note.md", content)
	absFilePath := createTestNoteFile(t, notesDir, "code-note.md", content)

	// Create the source note with relative path
	note := models.NewNote("Code Note", "code-note", "code-note.md", nil)
	err := service.SaveNote(note)
	// Create the source note
	note := models.NewNote("Code Note", "code-note", filePath, nil)
	err := service.SaveNote(note)
	// Create the source note with relative path
	relFilePath, err := filepath.Rel(notesDir, absFilePath)
	require.NoError(t, err)
	note := models.NewNote("Code Note", "code-note", relFilePath, nil)
	err = service.SaveNote(note)
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
	filePath := createTestNoteFile(t, notesDir, "note.md", initialContent)
	absFilePath := createTestNoteFile(t, notesDir, "note.md", initialContent)

	// Create and save note with relative path
	note := models.NewNote("Note", "note", "note.md", nil)
	err := service.SaveNote(note)
	// Create and save note
	note := models.NewNote("Note", "note", filePath, nil)
	err := service.SaveNote(note)
	// Create and save note with relative path
	relFilePath, err := filepath.Rel(notesDir, absFilePath)
	require.NoError(t, err)
	note := models.NewNote("Note", "note", relFilePath, nil)
	err = service.SaveNote(note)
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
	err = os.WriteFile(filePath, []byte(updatedContent), 0644)
	err = os.WriteFile(absFilePath, []byte(updatedContent), 0644)
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
	sourceFilePath := createTestNoteFile(t, notesDir, "source-note.md", sourceContent)
	sourceAbsPath := createTestNoteFile(t, notesDir, "source-note.md", sourceContent)

	sourceNote := models.NewNote("Source Note", "source-note", "source-note.md", nil)
	err := service.SaveNote(sourceNote)
	sourceNote := models.NewNote("Source Note", "source-note", sourceFilePath, nil)
	err := service.SaveNote(sourceNote)
	sourceRelPath, err := filepath.Rel(notesDir, sourceAbsPath)
	require.NoError(t, err)
	sourceNote := models.NewNote("Source Note", "source-note", sourceRelPath, nil)
	err = service.SaveNote(sourceNote)
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
	targetFilePath := createTestNoteFile(t, notesDir, "target-note.md", "# Target Note\n\nContent here.")
	targetAbsPath := createTestNoteFile(t, notesDir, "target-note.md", "# Target Note\n\nContent here.")

	targetNote := models.NewNote("Target Note", "target-note", "target-note.md", nil)
	targetNote := models.NewNote("Target Note", "target-note", targetFilePath, nil)
	targetRelPath, err := filepath.Rel(notesDir, targetAbsPath)
	require.NoError(t, err)
	targetNote := models.NewNote("Target Note", "target-note", targetRelPath, nil)
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
		filePath := createTestNoteFile(t, notesDir, filename, content)
		absPath := createTestNoteFile(t, notesDir, filename, content)

		sourceNote := models.NewNote("Source Note "+string(rune('0'+i)), "source-note-"+string(rune('0'+i)), filename, nil)
		err := service.SaveNote(sourceNote)
		sourceNote := models.NewNote("Source Note "+string(rune('0'+i)), "source-note-"+string(rune('0'+i)), filePath, nil)
		err := service.SaveNote(sourceNote)
		relPath, err := filepath.Rel(notesDir, absPath)
		require.NoError(t, err)
		sourceNote := models.NewNote("Source Note "+string(rune('0'+i)), "source-note-"+string(rune('0'+i)), relPath, nil)
		err = service.SaveNote(sourceNote)
		require.NoError(t, err)

		err = service.UpdateNoteLinks(sourceNote, notesDir)
		require.NoError(t, err)
	}

	// Create the target note
	createTestNoteFile(t, notesDir, "shared-target.md", "# Shared Target\n\nContent.")
	targetFilePath := createTestNoteFile(t, notesDir, "shared-target.md", "# Shared Target\n\nContent.")
	targetAbsPath := createTestNoteFile(t, notesDir, "shared-target.md", "# Shared Target\n\nContent.")

	targetNote := models.NewNote("Shared Target", "shared-target", "shared-target.md", nil)
	err := service.SaveNote(targetNote)
	targetNote := models.NewNote("Shared Target", "shared-target", targetFilePath, nil)
	err := service.SaveNote(targetNote)
	targetRelPath, err := filepath.Rel(notesDir, targetAbsPath)
	require.NoError(t, err)
	targetNote := models.NewNote("Shared Target", "shared-target", targetRelPath, nil)
	err = service.SaveNote(targetNote)
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
	filePath := createTestNoteFile(t, notesDir, "empty-note.md", "")
	absPath := createTestNoteFile(t, notesDir, "empty-note.md", "")

	note := models.NewNote("Empty Note", "empty-note", "empty-note.md", nil)
	err := service.SaveNote(note)
	note := models.NewNote("Empty Note", "empty-note", filePath, nil)
	err := service.SaveNote(note)
	relPath, err := filepath.Rel(notesDir, absPath)
	require.NoError(t, err)
	note := models.NewNote("Empty Note", "empty-note", relPath, nil)
	err = service.SaveNote(note)
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
	filePath := createTestNoteFile(t, notesDir, "note.md", content)
	absPath := createTestNoteFile(t, notesDir, "note.md", content)

	note := models.NewNote("Note", "note", "note.md", nil)
	err := service.SaveNote(note)
	note := models.NewNote("Note", "note", filePath, nil)
	err := service.SaveNote(note)
	relPath, err := filepath.Rel(notesDir, absPath)
	require.NoError(t, err)
	note := models.NewNote("Note", "note", relPath, nil)
	err = service.SaveNote(note)
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

func TestGetBacklinks(t *testing.T) {
	service, _, tempDir := setupNotesTestService(t)
	defer cleanupNotesTestService(t, tempDir)

	notesDir := filepath.Join(tempDir, "notes")

	// Create a target note
	targetNote := models.NewNote("Target Note", "target-note", "/path/to/target.md", nil)
	err := service.SaveNote(targetNote)
	require.NoError(t, err)

	// Create multiple source notes that link to the target
	sourceNote1Content := `# Source One

This links to [[target-note]].`
	sourceNote1AbsPath := createTestNoteFile(t, notesDir, "source-one.md", sourceNote1Content)
	sourceNote1RelPath, err := filepath.Rel(notesDir, sourceNote1AbsPath)
	require.NoError(t, err)
	sourceNote1 := models.NewNote("Source One", "source-one", sourceNote1RelPath, nil)
	err = service.SaveNote(sourceNote1)
	require.NoError(t, err)
	err = service.UpdateNoteLinks(sourceNote1, notesDir)
	require.NoError(t, err)

	sourceNote2Content := `# Source Two

Also references [[target-note]].`
	sourceNote2AbsPath := createTestNoteFile(t, notesDir, "source-two.md", sourceNote2Content)
	sourceNote2RelPath, err := filepath.Rel(notesDir, sourceNote2AbsPath)
	require.NoError(t, err)
	sourceNote2 := models.NewNote("Source Two", "source-two", sourceNote2RelPath, nil)
	err = service.SaveNote(sourceNote2)
	require.NoError(t, err)
	err = service.UpdateNoteLinks(sourceNote2, notesDir)
	require.NoError(t, err)

	// Get backlinks for the target note
	backlinks, err := service.GetBacklinks(targetNote.ID)
	require.NoError(t, err)
	assert.Len(t, backlinks, 2)

	// Verify backlinks point to the correct source notes
	sourceIDs := make([]string, len(backlinks))
	for i, backlink := range backlinks {
		sourceIDs[i] = backlink.SourceNoteID
		assert.Equal(t, targetNote.ID, backlink.TargetNoteID)
		assert.Equal(t, "target-note", backlink.TargetSlug)
	}

	assert.Contains(t, sourceIDs, sourceNote1.ID)
	assert.Contains(t, sourceIDs, sourceNote2.ID)
}

func TestGetBacklinks_NoBacklinks(t *testing.T) {
	service, _, tempDir := setupNotesTestService(t)
	defer cleanupNotesTestService(t, tempDir)

	// Create a note with no backlinks
	note := models.NewNote("Lonely Note", "lonely-note", "/path/to/lonely.md", nil)
	err := service.SaveNote(note)
	require.NoError(t, err)

	// Get backlinks (should be empty)
	backlinks, err := service.GetBacklinks(note.ID)
	require.NoError(t, err)
	assert.Empty(t, backlinks)
}

func TestGetBacklinks_OrphanedLinksNotIncluded(t *testing.T) {
	service, _, tempDir := setupNotesTestService(t)
	defer cleanupNotesTestService(t, tempDir)

	notesDir := filepath.Join(tempDir, "notes")

	// Create a target note
	targetNote := models.NewNote("Target Note", "target-note", "/path/to/target.md", nil)
	err := service.SaveNote(targetNote)
	require.NoError(t, err)

	// Create source note with link to target
	sourceContent := `# Source

Links to [[target-note]].`
	sourceAbsPath := createTestNoteFile(t, notesDir, "source.md", sourceContent)
	sourceRelPath, err := filepath.Rel(notesDir, sourceAbsPath)
	require.NoError(t, err)
	sourceNote := models.NewNote("Source", "source", sourceRelPath, nil)
	err = service.SaveNote(sourceNote)
	require.NoError(t, err)
	err = service.UpdateNoteLinks(sourceNote, notesDir)
	require.NoError(t, err)

	// Create orphaned note with link to non-existent note
	orphanContent := `# Orphan

Links to [[non-existent]].`
	orphanAbsPath := createTestNoteFile(t, notesDir, "orphan.md", orphanContent)
	orphanRelPath, err := filepath.Rel(notesDir, orphanAbsPath)
	require.NoError(t, err)
	orphanNote := models.NewNote("Orphan", "orphan", orphanRelPath, nil)
	err = service.SaveNote(orphanNote)
	require.NoError(t, err)
	err = service.UpdateNoteLinks(orphanNote, notesDir)
	require.NoError(t, err)

	// Get backlinks for target - should only include resolved links
	backlinks, err := service.GetBacklinks(targetNote.ID)
	require.NoError(t, err)
	assert.Len(t, backlinks, 1)
	assert.Equal(t, sourceNote.ID, backlinks[0].SourceNoteID)
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
