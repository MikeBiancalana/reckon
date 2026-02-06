package service

import (
	"fmt"
	"os"

	"github.com/MikeBiancalana/reckon/internal/logger"
	"github.com/MikeBiancalana/reckon/internal/models"
	"github.com/MikeBiancalana/reckon/internal/parser"
)

// NotesService handles business logic for notes and wiki links.
type NotesService struct {
	repo *NotesRepository
}

// NewNotesService creates a new notes service.
func NewNotesService(repo *NotesRepository) *NotesService {
	return &NotesService{repo: repo}
}

// SaveNote saves a note to the database.
func (s *NotesService) SaveNote(note *models.Note) error {
	return s.repo.SaveNote(note)
}

// GetNoteByID retrieves a note by its ID.
func (s *NotesService) GetNoteByID(id string) (*models.Note, error) {
	return s.repo.GetNoteByID(id)
}

// GetNoteBySlug retrieves a note by its slug.
func (s *NotesService) GetNoteBySlug(slug string) (*models.Note, error) {
	return s.repo.GetNoteBySlug(slug)
}

// UpdateNoteLinks extracts wiki links from a note's content and updates the database.
// This should be called after creating or updating a note.
//
// The function:
// 1. Reads the note's markdown file
// 2. Extracts all wiki-style links ([[slug]] or [[slug|text]])
// 3. Deletes old wiki links for this note
// 4. Creates new link records with resolved target_note_id when possible
// 5. Commits everything in a transaction
func (s *NotesService) UpdateNoteLinks(note *models.Note) error {
	logger.Info("UpdateNoteLinks", "note_id", note.ID, "slug", note.Slug, "file_path", note.FilePath)

	// Read the note's content from file
	content, err := os.ReadFile(note.FilePath)
	if err != nil {
		logger.Error("UpdateNoteLinks", "error", err, "note_id", note.ID, "file_path", note.FilePath)
		return fmt.Errorf("failed to read note file: %w", err)
	}

	// Extract wiki links
	wikiLinks := parser.ExtractWikiLinks(string(content))
	logger.Debug("UpdateNoteLinks", "note_id", note.ID, "links_found", len(wikiLinks))

	// Begin transaction
	tx, err := s.repo.db.BeginTx()
	if err != nil {
		logger.Error("UpdateNoteLinks", "error", err, "note_id", note.ID, "operation", "begin_transaction")
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete existing wiki links for this note
	if err := s.repo.DeleteNoteLinks(tx, note.ID, models.LinkTypeReference); err != nil {
		logger.Error("UpdateNoteLinks", "error", err, "note_id", note.ID, "operation", "delete_old_links")
		return fmt.Errorf("failed to delete old links: %w", err)
	}

	// Create new link records
	for _, wikiLink := range wikiLinks {
		// Try to resolve the target note
		targetNote, err := s.repo.GetNoteBySlug(wikiLink.TargetSlug)
		if err != nil {
			logger.Error("UpdateNoteLinks", "error", err, "note_id", note.ID, "target_slug", wikiLink.TargetSlug)
			return fmt.Errorf("failed to look up target note: %w", err)
		}

		// Create the link
		link := models.NewNoteLink(note.ID, wikiLink.TargetSlug, models.LinkTypeReference)

		// Set target_note_id if the target exists
		if targetNote != nil {
			link.UpdateTargetNoteID(targetNote.ID)
			logger.Debug("UpdateNoteLinks", "note_id", note.ID, "target_slug", wikiLink.TargetSlug, "target_note_id", targetNote.ID, "resolved", true)
		} else {
			logger.Debug("UpdateNoteLinks", "note_id", note.ID, "target_slug", wikiLink.TargetSlug, "resolved", false)
		}

		// Save the link
		if err := s.repo.SaveNoteLink(tx, link); err != nil {
			logger.Error("UpdateNoteLinks", "error", err, "note_id", note.ID, "link_id", link.ID)
			return fmt.Errorf("failed to save link: %w", err)
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		logger.Error("UpdateNoteLinks", "error", err, "note_id", note.ID, "operation", "commit_transaction")
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	logger.Info("UpdateNoteLinks", "note_id", note.ID, "links_created", len(wikiLinks))
	return nil
}

// ResolveOrphanedBacklinks finds all links with NULL target_note_id where the target now exists,
// and updates them to point to the correct note.
//
// This should be called after creating a new note to connect any existing links that pointed
// to this note's slug before it existed.
func (s *NotesService) ResolveOrphanedBacklinks(note *models.Note) error {
	logger.Info("ResolveOrphanedBacklinks", "note_id", note.ID, "slug", note.Slug)

	// Get all orphaned links that point to this slug
	orphanedLinks, err := s.repo.GetOrphanedLinks()
	if err != nil {
		logger.Error("ResolveOrphanedBacklinks", "error", err, "note_id", note.ID)
		return fmt.Errorf("failed to get orphaned links: %w", err)
	}

	resolvedCount := 0
	for _, link := range orphanedLinks {
		// Check if this link points to the newly created note
		if link.TargetSlug == note.Slug {
			// Update the link to point to this note
			if err := s.repo.UpdateNoteLinkTargetID(link.ID, note.ID); err != nil {
				logger.Error("ResolveOrphanedBacklinks", "error", err, "link_id", link.ID, "note_id", note.ID)
				return fmt.Errorf("failed to update link: %w", err)
			}
			resolvedCount++
			logger.Debug("ResolveOrphanedBacklinks", "link_id", link.ID, "note_id", note.ID, "source_note_id", link.SourceNoteID)
		}
	}

	logger.Info("ResolveOrphanedBacklinks", "note_id", note.ID, "resolved_count", resolvedCount)
	return nil
}

// GetLinksBySourceNote retrieves all links from a source note.
func (s *NotesService) GetLinksBySourceNote(sourceNoteID string) ([]models.NoteLink, error) {
	return s.repo.GetLinksBySourceNote(sourceNoteID)
}

// GetAllNotes retrieves all notes from the database.
func (s *NotesService) GetAllNotes() ([]*models.Note, error) {
	return s.repo.GetAllNotes()
}
