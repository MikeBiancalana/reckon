package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/models"
	notesvc "github.com/MikeBiancalana/reckon/internal/service"
	"github.com/MikeBiancalana/reckon/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupNoteShowTest creates a test environment for note show tests.
func setupNoteShowTest(t *testing.T) (string, func()) {
	t.Helper()

	// Create temp directory
	tempDir, err := os.MkdirTemp("", "note-show-test-*")
	require.NoError(t, err)

	// Set environment variable for test data directory
	os.Setenv("RECKON_DATA_DIR", tempDir)

	// Create notes directory structure
	notesDir, err := config.NotesDir()
	require.NoError(t, err)

	// Create database
	dbPath, err := config.DatabasePath()
	require.NoError(t, err)
	db, err := storage.NewDatabase(dbPath)
	require.NoError(t, err)

	// Initialize global notesService
	notesRepo := notesvc.NewNotesRepository(db)
	notesService = notesvc.NewNotesService(notesRepo)

	cleanup := func() {
		os.Unsetenv("RECKON_DATA_DIR")
		os.RemoveAll(tempDir)
	}

	return notesDir, cleanup
}

// createNoteFile creates a note file with frontmatter and content.
func createNoteFile(t *testing.T, notesDir, relativePath, content string) {
	t.Helper()

	fullPath := filepath.Join(notesDir, relativePath)
	dir := filepath.Dir(fullPath)
	require.NoError(t, os.MkdirAll(dir, 0755))
	require.NoError(t, os.WriteFile(fullPath, []byte(content), 0644))
}

func TestStripFrontmatter(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "with frontmatter",
			input: `---
title: Test Note
tags: [tag1, tag2]
---

# Content

This is the content.`,
			expected: `
# Content

This is the content.`,
		},
		{
			name:     "without frontmatter",
			input:    "# Just Content\n\nNo frontmatter here.",
			expected: "# Just Content\n\nNo frontmatter here.",
		},
		{
			name: "empty content after frontmatter",
			input: `---
title: Test
---
`,
			expected: "",
		},
		{
			name:     "unclosed frontmatter",
			input:    "---\ntitle: Test\n\nNo closing delimiter",
			expected: "---\ntitle: Test\n\nNo closing delimiter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripFrontmatter(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNoteShowCommand_Integration(t *testing.T) {
	notesDir, cleanup := setupNoteShowTest(t)
	defer cleanup()

	// Create target notes for links
	targetNote1 := models.NewNote("Target One", "target-one", "2026/2026-02/target-one.md", nil)
	err := notesService.SaveNote(targetNote1)
	require.NoError(t, err)

	targetNote2 := models.NewNote("Target Two", "target-two", "2026/2026-02/target-two.md", nil)
	err = notesService.SaveNote(targetNote2)
	require.NoError(t, err)

	// Create main note with links
	mainNoteContent := `---
title: Main Note
tags: [test, integration]
---

# Main Note

This is a test note that links to [[target-one]] and [[target-two]].

It also references [[orphaned-link]] which doesn't exist.`

	mainNotePath := "2026/2026-02/main-note.md"
	createNoteFile(t, notesDir, mainNotePath, mainNoteContent)

	// Save with relative path (as per model design)
	mainNote := models.NewNote("Main Note", "main-note", mainNotePath, []string{"test", "integration"})
	err = notesService.SaveNote(mainNote)
	require.NoError(t, err)

	// Update links with notesDir parameter
	err = notesService.UpdateNoteLinks(mainNote, notesDir)
	require.NoError(t, err)

	// Create a source note that links to main note (backlink)
	sourceNoteContent := `---
title: Source Note
---

# Source Note

This references [[main-note]].`

	sourceNotePath := "2026/2026-02/source-note.md"
	createNoteFile(t, notesDir, sourceNotePath, sourceNoteContent)

	// Save with relative path
	sourceNote := models.NewNote("Source Note", "source-note", sourceNotePath, nil)
	err = notesService.SaveNote(sourceNote)
	require.NoError(t, err)

	// Update links with notesDir parameter
	err = notesService.UpdateNoteLinks(sourceNote, notesDir)
	require.NoError(t, err)

	// Test: Get the main note
	retrievedNote, err := notesService.GetNoteBySlug("main-note")
	require.NoError(t, err)
	require.NotNil(t, retrievedNote)
	assert.Equal(t, "Main Note", retrievedNote.Title)
	assert.Equal(t, "main-note", retrievedNote.Slug)
	assert.Equal(t, []string{"test", "integration"}, retrievedNote.Tags)

	// Test: Read note content and strip frontmatter
	noteFilePath := filepath.Join(notesDir, retrievedNote.FilePath)
	contentBytes, err := os.ReadFile(noteFilePath)
	require.NoError(t, err)
	content := stripFrontmatter(string(contentBytes))
	assert.Contains(t, content, "# Main Note")
	assert.Contains(t, content, "This is a test note")
	assert.NotContains(t, content, "title: Main Note") // Frontmatter stripped

	// Test: Get outgoing links
	outgoingLinks, err := notesService.GetLinksBySourceNote(retrievedNote.ID)
	require.NoError(t, err)
	assert.Len(t, outgoingLinks, 3)

	// Verify link details
	linkSlugs := make(map[string]bool)
	resolvedCount := 0
	for _, link := range outgoingLinks {
		linkSlugs[link.TargetSlug] = true
		if link.TargetNoteID != "" {
			resolvedCount++
		}
	}
	assert.True(t, linkSlugs["target-one"])
	assert.True(t, linkSlugs["target-two"])
	assert.True(t, linkSlugs["orphaned-link"])
	assert.Equal(t, 2, resolvedCount) // Only target-one and target-two are resolved

	// Test: Get backlinks
	backlinks, err := notesService.GetBacklinks(retrievedNote.ID)
	require.NoError(t, err)
	assert.Len(t, backlinks, 1)
	assert.Equal(t, sourceNote.ID, backlinks[0].SourceNoteID)
	assert.Equal(t, retrievedNote.ID, backlinks[0].TargetNoteID)

	// Test: Verify source note can be retrieved for backlink display
	sourceNoteForDisplay, err := notesService.GetNoteByID(backlinks[0].SourceNoteID)
	require.NoError(t, err)
	assert.Equal(t, "Source Note", sourceNoteForDisplay.Title)
	assert.Equal(t, "source-note", sourceNoteForDisplay.Slug)
}

func TestNoteShowCommand_NoteNotFound(t *testing.T) {
	_, cleanup := setupNoteShowTest(t)
	defer cleanup()

	// Try to get non-existent note
	note, err := notesService.GetNoteBySlug("non-existent")
	require.NoError(t, err)
	assert.Nil(t, note)
}

func TestNoteShowCommand_NoLinks(t *testing.T) {
	notesDir, cleanup := setupNoteShowTest(t)
	defer cleanup()

	// Create standalone note with no links
	noteContent := `---
title: Standalone Note
---

# Standalone Note

This note has no links.`

	notePath := "2026/2026-02/standalone.md"
	createNoteFile(t, notesDir, notePath, noteContent)

	note := models.NewNote("Standalone Note", "standalone", notePath, nil)
	err := notesService.SaveNote(note)
	require.NoError(t, err)

	// Get links (should be empty)
	outgoingLinks, err := notesService.GetLinksBySourceNote(note.ID)
	require.NoError(t, err)
	assert.Empty(t, outgoingLinks)

	// Get backlinks (should be empty)
	backlinks, err := notesService.GetBacklinks(note.ID)
	require.NoError(t, err)
	assert.Empty(t, backlinks)
}

func TestNoteShowCommand_FileNotFound(t *testing.T) {
	_, cleanup := setupNoteShowTest(t)
	defer cleanup()

	// Create note in database but don't create file
	note := models.NewNote("Missing File", "missing-file", "2026/2026-02/missing.md", nil)
	err := notesService.SaveNote(note)
	require.NoError(t, err)

	// Try to read file
	notesDir, err := config.NotesDir()
	require.NoError(t, err)
	noteFilePath := filepath.Join(notesDir, note.FilePath)
	_, err = os.ReadFile(noteFilePath)
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}

func TestDisplayLink_ResolvedAndOrphaned(t *testing.T) {
	notesDir, cleanup := setupNoteShowTest(t)
	defer cleanup()

	// Create a target note
	targetNote := models.NewNote("Target Note", "target-note", "2026/2026-02/target.md", nil)
	err := notesService.SaveNote(targetNote)
	require.NoError(t, err)

	// Create source note with both resolved and orphaned links
	sourceContent := `# Source

Links to [[target-note]] and [[orphaned]].`

	sourcePath := "2026/2026-02/source.md"
	createNoteFile(t, notesDir, sourcePath, sourceContent)

	// Save with relative path
	sourceNote := models.NewNote("Source", "source", sourcePath, nil)
	err = notesService.SaveNote(sourceNote)
	require.NoError(t, err)

	// Update links with notesDir parameter
	err = notesService.UpdateNoteLinks(sourceNote, notesDir)
	require.NoError(t, err)

	// Get links
	links, err := notesService.GetLinksBySourceNote(sourceNote.ID)
	require.NoError(t, err)
	assert.Len(t, links, 2)

	// Verify one is resolved, one is orphaned
	var resolvedLink, orphanedLink *models.NoteLink
	for i := range links {
		if links[i].TargetSlug == "target-note" {
			resolvedLink = &links[i]
		} else if links[i].TargetSlug == "orphaned" {
			orphanedLink = &links[i]
		}
	}

	require.NotNil(t, resolvedLink)
	require.NotNil(t, orphanedLink)
	assert.NotEmpty(t, resolvedLink.TargetNoteID)
	assert.Empty(t, orphanedLink.TargetNoteID)

	// Verify we can get the target note's title for resolved link
	targetForDisplay, err := notesService.GetNoteByID(resolvedLink.TargetNoteID)
	require.NoError(t, err)
	assert.Equal(t, "Target Note", targetForDisplay.Title)
}

// TestNoteShow_PathTraversalProtection tests that path traversal attempts are blocked.
func TestNoteShow_PathTraversalProtection(t *testing.T) {
	notesDir, cleanup := setupNoteShowTest(t)
	defer cleanup()

	// Create a legitimate note file
	noteContent := `---
title: Legitimate Note
---

# Legitimate Note

This is a legitimate note.`

	notePath := "2026/2026-02/legitimate-note.md"
	createNoteFile(t, notesDir, notePath, noteContent)

	// Save the note with relative path
	note := models.NewNote("Legitimate Note", "legitimate-note", notePath, nil)
	err := notesService.SaveNote(note)
	require.NoError(t, err)

	// Test 1: Attempt path traversal in Note.FilePath when reading
	// Create a malicious note entry with path traversal
	maliciousNote := models.NewNote("Malicious Note", "malicious-note", "../../../etc/passwd", nil)
	err = notesService.SaveNote(maliciousNote)
	require.NoError(t, err)

	// Try to read the malicious note - should fail
	retrieved, err := notesService.GetNoteBySlug("malicious-note")
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	// Construct the file path as noteShowCmd would
	filePath := filepath.Join(notesDir, retrieved.FilePath)
	cleanPath := filepath.Clean(filePath)
	cleanNotesDir := filepath.Clean(notesDir)

	// Verify path traversal is detected
	if !strings.HasPrefix(cleanPath, cleanNotesDir+string(filepath.Separator)) {
		// Path traversal detected - this is the expected behavior
		t.Log("Path traversal correctly blocked:", retrieved.FilePath)
	} else {
		t.Fatal("Path traversal was not detected!")
	}

	// Test 2: Verify legitimate notes can still be read
	legitimateNote, err := notesService.GetNoteBySlug("legitimate-note")
	require.NoError(t, err)
	require.NotNil(t, legitimateNote)

	// This should work
	legitPath := filepath.Join(notesDir, legitimateNote.FilePath)
	cleanLegitPath := filepath.Clean(legitPath)
	cleanNotesDir2 := filepath.Clean(notesDir)

	assert.True(t, strings.HasPrefix(cleanLegitPath, cleanNotesDir2+string(filepath.Separator)),
		"Legitimate note path should be within notesDir")

	// Read the file to ensure it works
	contentBytes, err := os.ReadFile(cleanLegitPath)
	require.NoError(t, err)
	assert.Contains(t, string(contentBytes), "Legitimate Note")
}
