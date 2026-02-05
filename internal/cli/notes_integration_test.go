package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MikeBiancalana/reckon/internal/config"
	notessvc "github.com/MikeBiancalana/reckon/internal/service"
	"github.com/MikeBiancalana/reckon/internal/storage"
)

func TestNoteCreationWithWikiLinks(t *testing.T) {
	// Setup test environment
	tmpDir := t.TempDir()
	os.Setenv("RECKON_DATA_DIR", tmpDir)
	defer os.Unsetenv("RECKON_DATA_DIR")

	// Initialize database and service
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := storage.NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer db.Close()

	repo := notessvc.NewNotesRepository(db)
	notesService = notessvc.NewNotesService(repo)

	// Create first note (target for links)
	err = createNoteWithContent("Target Note", []string{"target"}, "This is the target note.")
	if err != nil {
		t.Fatalf("Failed to create target note: %v", err)
	}

	// Create second note with wiki links
	contentWithLinks := `This note references [[target-note]] which exists.

It also references [[non-existent-note]] which doesn't exist yet.

And here's another reference to [[target-note|the same note with custom text]].`

	err = createNoteWithContent("Source Note", []string{"source"}, contentWithLinks)
	if err != nil {
		t.Fatalf("Failed to create source note: %v", err)
	}

	// Get the source note
	sourceNote, err := notesService.GetNoteBySlug("source-note")
	if err != nil {
		t.Fatalf("Failed to get source note: %v", err)
	}

	// Verify links were extracted
	links, err := notesService.GetLinksBySourceNote(sourceNote.ID)
	if err != nil {
		t.Fatalf("Failed to get links: %v", err)
	}

	// Should have 2 unique links (target-note appears twice but should be deduplicated, plus non-existent-note)
	if len(links) != 2 {
		t.Errorf("Expected 2 unique links, got %d", len(links))
	}

	// Check that target-note link is resolved (has target_note_id)
	var targetLinkResolved bool
	var nonExistentLinkUnresolved bool

	for _, link := range links {
		if link.TargetSlug == "target-note" {
			if link.TargetNoteID != "" {
				targetLinkResolved = true
			}
		}
		if link.TargetSlug == "non-existent-note" {
			if link.TargetNoteID == "" {
				nonExistentLinkUnresolved = true
			}
		}
	}

	if !targetLinkResolved {
		t.Error("Expected link to 'target-note' to be resolved with target_note_id")
	}
	if !nonExistentLinkUnresolved {
		t.Error("Expected link to 'non-existent-note' to be unresolved (no target_note_id)")
	}

	// Now create the non-existent note and verify orphaned backlinks are resolved
	err = createNoteWithContent("Non Existent Note", []string{"new"}, "This note now exists!")
	if err != nil {
		t.Fatalf("Failed to create non-existent note: %v", err)
	}

	// Re-fetch the links
	links, err = notesService.GetLinksBySourceNote(sourceNote.ID)
	if err != nil {
		t.Fatalf("Failed to get links after creating target: %v", err)
	}

	// Now both links should be resolved
	var allResolved = true
	for _, link := range links {
		if link.TargetNoteID == "" {
			allResolved = false
			t.Errorf("Link to '%s' is still unresolved after creating target note", link.TargetSlug)
		}
	}

	if allResolved {
		t.Log("All links successfully resolved after creating target notes")
	}
}

func TestQuietFlag(t *testing.T) {
	// Setup test environment
	tmpDir := t.TempDir()
	os.Setenv("RECKON_DATA_DIR", tmpDir)
	defer os.Unsetenv("RECKON_DATA_DIR")

	// Initialize database and service
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := storage.NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer db.Close()

	repo := notessvc.NewNotesRepository(db)
	notesService = notessvc.NewNotesService(repo)

	// Set quiet flag
	quietFlag = true
	defer func() { quietFlag = false }()

	// Create note - should not produce output
	err = createNoteWithContent("Quiet Note", []string{}, "Quiet content")
	if err != nil {
		t.Fatalf("Failed to create quiet note: %v", err)
	}

	// Verify note was created
	note, err := notesService.GetNoteBySlug("quiet-note")
	if err != nil {
		t.Fatalf("Failed to get quiet note: %v", err)
	}
	if note == nil {
		t.Fatal("Quiet note was not created")
	}
}

func TestNoteFileStructure(t *testing.T) {
	// Setup test environment
	tmpDir := t.TempDir()
	os.Setenv("RECKON_DATA_DIR", tmpDir)
	defer os.Unsetenv("RECKON_DATA_DIR")

	// Initialize database and service
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := storage.NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer db.Close()

	repo := notessvc.NewNotesRepository(db)
	notesService = notessvc.NewNotesService(repo)

	// Create note
	err = createNoteWithContent("Structure Test", []string{"tag1", "tag2"}, "Test content")
	if err != nil {
		t.Fatalf("Failed to create note: %v", err)
	}

	// Verify directory structure
	notesDir, _ := config.NotesDir()

	// Check year directory exists
	yearPath := filepath.Join(notesDir, "2026")
	if _, err := os.Stat(yearPath); os.IsNotExist(err) {
		t.Errorf("Year directory not created: %s", yearPath)
	}

	// Check year-month directory exists
	monthPath := filepath.Join(notesDir, "2026", "2026-02")
	if _, err := os.Stat(monthPath); os.IsNotExist(err) {
		t.Errorf("Month directory not created: %s", monthPath)
	}

	// Check note file exists with correct name pattern
	note, _ := notesService.GetNoteBySlug("structure-test")
	fullPath := filepath.Join(notesDir, note.FilePath)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		t.Errorf("Note file not created: %s", fullPath)
	}

	// Verify file name matches pattern: yyyy-mm-dd-slug.md
	expectedFilename := "2026-02-01-structure-test.md"
	if filepath.Base(note.FilePath) != expectedFilename {
		t.Errorf("Expected filename '%s', got '%s'", expectedFilename, filepath.Base(note.FilePath))
	}
}
