package service

import (
	"testing"
	"time"

	"github.com/MikeBiancalana/reckon/internal/models"
	"github.com/MikeBiancalana/reckon/internal/storage"
)

// TestGetOutgoingLinksWithNotes_Integration tests the enriched outgoing links query
func TestGetOutgoingLinksWithNotes_Integration(t *testing.T) {
	// Setup
	db, cleanup := storage.NewTestDatabase(t)
	defer cleanup()

	repo := NewNotesRepository(db)

	// Create source note
	sourceNote := models.NewNote("Source Note", "source-note", "source-note.md", []string{})
	if err := repo.SaveNote(sourceNote); err != nil {
		t.Fatalf("failed to save source note: %v", err)
	}

	// Create target note
	targetNote := models.NewNote("Target Note", "target-note", "target-note.md", []string{})
	if err := repo.SaveNote(targetNote); err != nil {
		t.Fatalf("failed to save target note: %v", err)
	}

	// Create link from source to target
	tx, err := db.BeginTx()
	if err != nil {
		t.Fatalf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	link := models.NewNoteLink(sourceNote.ID, "target-note", models.LinkTypeReference)
	link.UpdateTargetNoteID(targetNote.ID)
	if err := repo.SaveNoteLink(tx, link); err != nil {
		t.Fatalf("failed to save link: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("failed to commit transaction: %v", err)
	}

	// Test: GetOutgoingLinksWithNotes should populate TargetNote
	links, err := repo.GetOutgoingLinksWithNotes(sourceNote.ID)
	if err != nil {
		t.Fatalf("GetOutgoingLinksWithNotes failed: %v", err)
	}

	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}

	if links[0].TargetNote == nil {
		t.Fatal("expected TargetNote to be populated, got nil")
	}

	if links[0].TargetNote.Title != "Target Note" {
		t.Errorf("expected target title 'Target Note', got '%s'", links[0].TargetNote.Title)
	}

	if links[0].TargetNote.Slug != "target-note" {
		t.Errorf("expected target slug 'target-note', got '%s'", links[0].TargetNote.Slug)
	}
}

// TestGetBacklinksWithNotes_Integration tests the enriched backlinks query
func TestGetBacklinksWithNotes_Integration(t *testing.T) {
	// Setup
	db, cleanup := storage.NewTestDatabase(t)
	defer cleanup()

	repo := NewNotesRepository(db)

	// Create source note
	sourceNote := models.NewNote("Source Note", "source-note", "source-note.md", []string{})
	if err := repo.SaveNote(sourceNote); err != nil {
		t.Fatalf("failed to save source note: %v", err)
	}

	// Create target note
	targetNote := models.NewNote("Target Note", "target-note", "target-note.md", []string{})
	if err := repo.SaveNote(targetNote); err != nil {
		t.Fatalf("failed to save target note: %v", err)
	}

	// Create link from source to target
	tx, err := db.BeginTx()
	if err != nil {
		t.Fatalf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	link := models.NewNoteLink(sourceNote.ID, "target-note", models.LinkTypeReference)
	link.UpdateTargetNoteID(targetNote.ID)
	if err := repo.SaveNoteLink(tx, link); err != nil {
		t.Fatalf("failed to save link: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("failed to commit transaction: %v", err)
	}

	// Test: GetBacklinksWithNotes should populate SourceNote
	backlinks, err := repo.GetBacklinksWithNotes(targetNote.ID)
	if err != nil {
		t.Fatalf("GetBacklinksWithNotes failed: %v", err)
	}

	if len(backlinks) != 1 {
		t.Fatalf("expected 1 backlink, got %d", len(backlinks))
	}

	if backlinks[0].SourceNote == nil {
		t.Fatal("expected SourceNote to be populated, got nil")
	}

	if backlinks[0].SourceNote.Title != "Source Note" {
		t.Errorf("expected source title 'Source Note', got '%s'", backlinks[0].SourceNote.Title)
	}

	if backlinks[0].SourceNote.Slug != "source-note" {
		t.Errorf("expected source slug 'source-note', got '%s'", backlinks[0].SourceNote.Slug)
	}
}

// TestGetOutgoingLinksWithNotes_UnresolvedLink tests handling of unresolved links
func TestGetOutgoingLinksWithNotes_UnresolvedLink(t *testing.T) {
	// Setup
	db, cleanup := storage.NewTestDatabase(t)
	defer cleanup()

	repo := NewNotesRepository(db)

	// Create source note
	sourceNote := models.NewNote("Source Note", "source-note", "source-note.md", []string{})
	if err := repo.SaveNote(sourceNote); err != nil {
		t.Fatalf("failed to save source note: %v", err)
	}

	// Create unresolved link (target note doesn't exist)
	tx, err := db.BeginTx()
	if err != nil {
		t.Fatalf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	link := models.NewNoteLink(sourceNote.ID, "nonexistent-note", models.LinkTypeReference)
	// Don't set TargetNoteID - it's unresolved
	if err := repo.SaveNoteLink(tx, link); err != nil {
		t.Fatalf("failed to save link: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("failed to commit transaction: %v", err)
	}

	// Test: GetOutgoingLinksWithNotes should return link with nil TargetNote
	links, err := repo.GetOutgoingLinksWithNotes(sourceNote.ID)
	if err != nil {
		t.Fatalf("GetOutgoingLinksWithNotes failed: %v", err)
	}

	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}

	if links[0].TargetNote != nil {
		t.Errorf("expected TargetNote to be nil for unresolved link, got %+v", links[0].TargetNote)
	}

	if links[0].TargetSlug != "nonexistent-note" {
		t.Errorf("expected target slug 'nonexistent-note', got '%s'", links[0].TargetSlug)
	}
}

// TestNotesService_GetOutgoingLinksWithNotes_Wrapper tests service wrapper
func TestNotesService_GetOutgoingLinksWithNotes_Wrapper(t *testing.T) {
	// Setup
	db, cleanup := storage.NewTestDatabase(t)
	defer cleanup()

	repo := NewNotesRepository(db)
	service := NewNotesService(repo)

	// Create notes
	sourceNote := models.NewNote("Source", "source", "source.md", []string{})
	if err := repo.SaveNote(sourceNote); err != nil {
		t.Fatalf("failed to save source note: %v", err)
	}

	targetNote := models.NewNote("Target", "target", "target.md", []string{})
	if err := repo.SaveNote(targetNote); err != nil {
		t.Fatalf("failed to save target note: %v", err)
	}

	// Create link
	tx, err := db.BeginTx()
	if err != nil {
		t.Fatalf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	link := models.NewNoteLink(sourceNote.ID, "target", models.LinkTypeReference)
	link.UpdateTargetNoteID(targetNote.ID)
	if err := repo.SaveNoteLink(tx, link); err != nil {
		t.Fatalf("failed to save link: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("failed to commit transaction: %v", err)
	}

	// Test: Service should delegate to repository
	links, err := service.GetOutgoingLinksWithNotes(sourceNote.ID)
	if err != nil {
		t.Fatalf("GetOutgoingLinksWithNotes failed: %v", err)
	}

	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}

	if links[0].TargetNote == nil {
		t.Fatal("expected TargetNote to be populated")
	}
}

// TestNotesService_GetBacklinksWithNotes_Wrapper tests service wrapper
func TestNotesService_GetBacklinksWithNotes_Wrapper(t *testing.T) {
	// Setup
	db, cleanup := storage.NewTestDatabase(t)
	defer cleanup()

	repo := NewNotesRepository(db)
	service := NewNotesService(repo)

	// Create notes
	sourceNote := models.NewNote("Source", "source", "source.md", []string{})
	if err := repo.SaveNote(sourceNote); err != nil {
		t.Fatalf("failed to save source note: %v", err)
	}

	targetNote := models.NewNote("Target", "target", "target.md", []string{})
	if err := repo.SaveNote(targetNote); err != nil {
		t.Fatalf("failed to save target note: %v", err)
	}

	// Create link
	tx, err := db.BeginTx()
	if err != nil {
		t.Fatalf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	link := models.NewNoteLink(sourceNote.ID, "target", models.LinkTypeReference)
	link.UpdateTargetNoteID(targetNote.ID)
	if err := repo.SaveNoteLink(tx, link); err != nil {
		t.Fatalf("failed to save link: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("failed to commit transaction: %v", err)
	}

	// Test: Service should delegate to repository
	backlinks, err := service.GetBacklinksWithNotes(targetNote.ID)
	if err != nil {
		t.Fatalf("GetBacklinksWithNotes failed: %v", err)
	}

	if len(backlinks) != 1 {
		t.Fatalf("expected 1 backlink, got %d", len(backlinks))
	}

	if backlinks[0].SourceNote == nil {
		t.Fatal("expected SourceNote to be populated")
	}
}
