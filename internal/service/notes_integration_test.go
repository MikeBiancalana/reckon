package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MikeBiancalana/reckon/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNotesIntegration_CompleteWorkflow demonstrates the complete workflow
// of creating notes with wiki links in a realistic scenario.
func TestNotesIntegration_CompleteWorkflow(t *testing.T) {
	service, _, tempDir := setupNotesTestService(t)
	defer cleanupNotesTestService(t, tempDir)

	notesDir := filepath.Join(tempDir, "notes")

	// Scenario: User creates three interconnected notes
	// Note 1 links to Note 2 and Note 3 (which don't exist yet)
	// Then we create Note 2 and Note 3
	// Finally verify all links are properly connected

	// Step 1: Create first note with forward references
	note1Content := `# Getting Started

This is an introduction to the project.

For technical details, see [[architecture]] and [[implementation]].

Also reference [[getting-started]] (self-reference).`

	createTestNoteFile(t, notesDir, "getting-started.md", note1Content)
	note1 := models.NewNote("Getting Started", "getting-started", "getting-started.md", []string{"intro"})
	note1Path := createTestNoteFile(t, notesDir, "getting-started.md", note1Content)
	note1 := models.NewNote("Getting Started", "getting-started", note1Path, []string{"intro"})
	note1AbsPath := createTestNoteFile(t, notesDir, "getting-started.md", note1Content)
	note1RelPath, err := filepath.Rel(notesDir, note1AbsPath)
	require.NoError(t, err)
	note1 := models.NewNote("Getting Started", "getting-started", note1RelPath, []string{"intro"})

	err = service.SaveNote(note1)
	require.NoError(t, err)

	err = service.UpdateNoteLinks(note1, notesDir)
	require.NoError(t, err)

	err = service.ResolveOrphanedBacklinks(note1)
	require.NoError(t, err)

	// Verify forward references were created with NULL target_note_id
	links, err := service.GetLinksBySourceNote(note1.ID)
	require.NoError(t, err)
	assert.Len(t, links, 3)

	for _, link := range links {
		if link.TargetSlug == "architecture" || link.TargetSlug == "implementation" {
			assert.Empty(t, link.TargetNoteID, "forward reference should have NULL target_note_id")
		} else if link.TargetSlug == "getting-started" {
			assert.Equal(t, note1.ID, link.TargetNoteID, "self-reference should resolve immediately")
		}
	}

	// Step 2: Create second note that links back to first note
	note2Content := `# Architecture

System architecture overview.

See [[getting-started]] for introduction.

Implementation details in [[implementation]].`

	createTestNoteFile(t, notesDir, "architecture.md", note2Content)
	note2 := models.NewNote("Architecture", "architecture", "architecture.md", []string{"technical"})
	note2Path := createTestNoteFile(t, notesDir, "architecture.md", note2Content)
	note2 := models.NewNote("Architecture", "architecture", note2Path, []string{"technical"})
	note2AbsPath := createTestNoteFile(t, notesDir, "architecture.md", note2Content)
	note2RelPath, err := filepath.Rel(notesDir, note2AbsPath)
	require.NoError(t, err)
	note2 := models.NewNote("Architecture", "architecture", note2RelPath, []string{"technical"})

	err = service.SaveNote(note2)
	require.NoError(t, err)

	err = service.UpdateNoteLinks(note2, notesDir)
	require.NoError(t, err)

	err = service.ResolveOrphanedBacklinks(note2)
	require.NoError(t, err)

	// Verify note1's link to architecture now has target_note_id
	links, err = service.GetLinksBySourceNote(note1.ID)
	require.NoError(t, err)

	for _, link := range links {
		if link.TargetSlug == "architecture" {
			assert.Equal(t, note2.ID, link.TargetNoteID, "forward reference should now be resolved")
		}
	}

	// Verify note2's links
	links2, err := service.GetLinksBySourceNote(note2.ID)
	require.NoError(t, err)
	assert.Len(t, links2, 2)

	for _, link := range links2 {
		if link.TargetSlug == "getting-started" {
			assert.Equal(t, note1.ID, link.TargetNoteID)
		} else if link.TargetSlug == "implementation" {
			assert.Empty(t, link.TargetNoteID, "still a forward reference")
		}
	}

	// Step 3: Create third note
	note3Content := `# Implementation

Implementation guide.

Prerequisites: [[architecture]] and [[getting-started]].`

	createTestNoteFile(t, notesDir, "implementation.md", note3Content)
	note3 := models.NewNote("Implementation", "implementation", "implementation.md", []string{"technical", "guide"})
	note3Path := createTestNoteFile(t, notesDir, "implementation.md", note3Content)
	note3 := models.NewNote("Implementation", "implementation", note3Path, []string{"technical", "guide"})
	note3AbsPath := createTestNoteFile(t, notesDir, "implementation.md", note3Content)
	note3RelPath, err := filepath.Rel(notesDir, note3AbsPath)
	require.NoError(t, err)
	note3 := models.NewNote("Implementation", "implementation", note3RelPath, []string{"technical", "guide"})

	err = service.SaveNote(note3)
	require.NoError(t, err)

	err = service.UpdateNoteLinks(note3, notesDir)
	require.NoError(t, err)

	err = service.ResolveOrphanedBacklinks(note3)
	require.NoError(t, err)

	// Step 4: Verify all links are now fully connected
	// Check note1's links
	links1, err := service.GetLinksBySourceNote(note1.ID)
	require.NoError(t, err)

	linkMap1 := make(map[string]string)
	for _, link := range links1 {
		linkMap1[link.TargetSlug] = link.TargetNoteID
	}

	assert.Equal(t, note2.ID, linkMap1["architecture"], "note1 -> architecture should be resolved")
	assert.Equal(t, note3.ID, linkMap1["implementation"], "note1 -> implementation should be resolved")
	assert.Equal(t, note1.ID, linkMap1["getting-started"], "note1 -> itself should be resolved")

	// Check note2's links
	links2, err = service.GetLinksBySourceNote(note2.ID)
	require.NoError(t, err)

	linkMap2 := make(map[string]string)
	for _, link := range links2 {
		linkMap2[link.TargetSlug] = link.TargetNoteID
	}

	assert.Equal(t, note1.ID, linkMap2["getting-started"], "note2 -> getting-started should be resolved")
	assert.Equal(t, note3.ID, linkMap2["implementation"], "note2 -> implementation should be resolved")

	// Check note3's links
	links3, err := service.GetLinksBySourceNote(note3.ID)
	require.NoError(t, err)

	linkMap3 := make(map[string]string)
	for _, link := range links3 {
		linkMap3[link.TargetSlug] = link.TargetNoteID
	}

	assert.Equal(t, note2.ID, linkMap3["architecture"], "note3 -> architecture should be resolved")
	assert.Equal(t, note1.ID, linkMap3["getting-started"], "note3 -> getting-started should be resolved")
}

// TestNotesIntegration_UpdateLinks demonstrates updating a note's content
// and properly syncing the changes to the database.
func TestNotesIntegration_UpdateLinks(t *testing.T) {
	service, _, tempDir := setupNotesTestService(t)
	defer cleanupNotesTestService(t, tempDir)

	notesDir := filepath.Join(tempDir, "notes")

	// Create initial note
	initialContent := `# Project Notes

Links to [[design]] and [[testing]].`

	createTestNoteFile(t, notesDir, "project.md", initialContent)
	note := models.NewNote("Project Notes", "project", "project.md", []string{"project"})
	notePath := createTestNoteFile(t, notesDir, "project.md", initialContent)
	note := models.NewNote("Project Notes", "project", notePath, []string{"project"})
	noteAbsPath := createTestNoteFile(t, notesDir, "project.md", initialContent)
	noteRelPath, err := filepath.Rel(notesDir, noteAbsPath)
	require.NoError(t, err)
	note := models.NewNote("Project Notes", "project", noteRelPath, []string{"project"})

	err = service.SaveNote(note)
	require.NoError(t, err)

	err = service.UpdateNoteLinks(note, notesDir)
	require.NoError(t, err)

	// Verify initial links
	links, err := service.GetLinksBySourceNote(note.ID)
	require.NoError(t, err)
	assert.Len(t, links, 2)

	initialSlugs := make([]string, len(links))
	for i, link := range links {
		initialSlugs[i] = link.TargetSlug
	}
	assert.Contains(t, initialSlugs, "design")
	assert.Contains(t, initialSlugs, "testing")

	// Update the note's content
	updatedContent := `# Project Notes

Updated to link to [[implementation]] and [[deployment]].

Removed old links.`

	projectPath := filepath.Join(notesDir, "project.md")
	err = os.WriteFile(projectPath, []byte(updatedContent), 0644)
	err = os.WriteFile(notePath, []byte(updatedContent), 0644)
	err = os.WriteFile(noteAbsPath, []byte(updatedContent), 0644)
	require.NoError(t, err)

	// Update links in database
	err = service.UpdateNoteLinks(note, notesDir)
	require.NoError(t, err)

	// Verify links were updated
	links, err = service.GetLinksBySourceNote(note.ID)
	require.NoError(t, err)
	assert.Len(t, links, 2)

	updatedSlugs := make([]string, len(links))
	for i, link := range links {
		updatedSlugs[i] = link.TargetSlug
	}
	assert.Contains(t, updatedSlugs, "implementation")
	assert.Contains(t, updatedSlugs, "deployment")
	assert.NotContains(t, updatedSlugs, "design")
	assert.NotContains(t, updatedSlugs, "testing")
}
