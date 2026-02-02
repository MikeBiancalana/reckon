package service

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/MikeBiancalana/reckon/internal/logger"
	"github.com/MikeBiancalana/reckon/internal/models"
	"github.com/MikeBiancalana/reckon/internal/storage"
)

// NotesRepository handles database operations for notes and note links.
type NotesRepository struct {
	db *storage.Database
}

// NewNotesRepository creates a new notes repository.
func NewNotesRepository(db *storage.Database) *NotesRepository {
	return &NotesRepository{db: db}
}

// SaveNote saves a note to the database.
func (r *NotesRepository) SaveNote(note *models.Note) error {
	logger.Debug("SaveNote", "note_id", note.ID, "slug", note.Slug)

	tagsJSON := ""
	if len(note.Tags) > 0 {
		tagsJSON = strings.Join(note.Tags, ",")
	}

	_, err := r.db.DB().Exec(
		`INSERT INTO notes (id, title, slug, file_path, created_at, updated_at, tags)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   title = excluded.title,
		   slug = excluded.slug,
		   file_path = excluded.file_path,
		   updated_at = excluded.updated_at,
		   tags = excluded.tags`,
		note.ID, note.Title, note.Slug, note.FilePath,
		note.CreatedAt.Unix(), note.UpdatedAt.Unix(), tagsJSON,
	)
	if err != nil {
		logger.Error("SaveNote", "error", err, "note_id", note.ID)
		return fmt.Errorf("failed to save note: %w", err)
	}

	return nil
}

// GetNoteByID retrieves a note by its ID.
func (r *NotesRepository) GetNoteByID(id string) (*models.Note, error) {
	logger.Debug("GetNoteByID", "note_id", id)

	var note models.Note
	var tagsStr sql.NullString
	var createdUnix, updatedUnix int64

	err := r.db.DB().QueryRow(
		"SELECT id, title, slug, file_path, created_at, updated_at, tags FROM notes WHERE id = ?",
		id,
	).Scan(&note.ID, &note.Title, &note.Slug, &note.FilePath, &createdUnix, &updatedUnix, &tagsStr)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		logger.Error("GetNoteByID", "error", err, "note_id", id)
		return nil, fmt.Errorf("failed to get note: %w", err)
	}

	note.CreatedAt = time.Unix(createdUnix, 0)
	note.UpdatedAt = time.Unix(updatedUnix, 0)

	if tagsStr.Valid && tagsStr.String != "" {
		note.Tags = strings.Split(tagsStr.String, ",")
	}

	return &note, nil
}

// GetNoteBySlug retrieves a note by its slug.
func (r *NotesRepository) GetNoteBySlug(slug string) (*models.Note, error) {
	logger.Debug("GetNoteBySlug", "slug", slug)

	var note models.Note
	var tagsStr sql.NullString
	var createdUnix, updatedUnix int64

	err := r.db.DB().QueryRow(
		"SELECT id, title, slug, file_path, created_at, updated_at, tags FROM notes WHERE slug = ?",
		slug,
	).Scan(&note.ID, &note.Title, &note.Slug, &note.FilePath, &createdUnix, &updatedUnix, &tagsStr)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		logger.Error("GetNoteBySlug", "error", err, "slug", slug)
		return nil, fmt.Errorf("failed to get note by slug: %w", err)
	}

	note.CreatedAt = time.Unix(createdUnix, 0)
	note.UpdatedAt = time.Unix(updatedUnix, 0)

	if tagsStr.Valid && tagsStr.String != "" {
		note.Tags = strings.Split(tagsStr.String, ",")
	}

	return &note, nil
}

// DeleteNoteLinks deletes all links for a source note (used before re-inserting).
func (r *NotesRepository) DeleteNoteLinks(tx *sql.Tx, sourceNoteID string, linkType models.LinkType) error {
	logger.Debug("DeleteNoteLinks", "source_note_id", sourceNoteID, "link_type", linkType)

	_, err := tx.Exec(
		"DELETE FROM note_links WHERE source_note_id = ? AND link_type = ?",
		sourceNoteID, linkType,
	)
	if err != nil {
		logger.Error("DeleteNoteLinks", "error", err, "source_note_id", sourceNoteID)
		return fmt.Errorf("failed to delete note links: %w", err)
	}

	return nil
}

// SaveNoteLink saves a note link to the database.
func (r *NotesRepository) SaveNoteLink(tx *sql.Tx, link *models.NoteLink) error {
	logger.Debug("SaveNoteLink", "link_id", link.ID, "source_note_id", link.SourceNoteID, "target_slug", link.TargetSlug)

	targetNoteID := sql.NullString{}
	if link.TargetNoteID != "" {
		targetNoteID.Valid = true
		targetNoteID.String = link.TargetNoteID
	}

	_, err := tx.Exec(
		`INSERT INTO note_links (id, source_note_id, target_slug, target_note_id, link_type, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		link.ID, link.SourceNoteID, link.TargetSlug, targetNoteID, link.LinkType, link.CreatedAt.Unix(),
	)
	if err != nil {
		logger.Error("SaveNoteLink", "error", err, "link_id", link.ID)
		return fmt.Errorf("failed to save note link: %w", err)
	}

	return nil
}

// GetOrphanedLinks retrieves all links where target_note_id is NULL but the target slug exists.
func (r *NotesRepository) GetOrphanedLinks() ([]models.NoteLink, error) {
	logger.Debug("GetOrphanedLinks")

	rows, err := r.db.DB().Query(
		`SELECT nl.id, nl.source_note_id, nl.target_slug, nl.link_type, nl.created_at
		 FROM note_links nl
		 INNER JOIN notes n ON nl.target_slug = n.slug
		 WHERE nl.target_note_id IS NULL`,
	)
	if err != nil {
		logger.Error("GetOrphanedLinks", "error", err)
		return nil, fmt.Errorf("failed to query orphaned links: %w", err)
	}
	defer rows.Close()

	var links []models.NoteLink
	for rows.Next() {
		var link models.NoteLink
		var linkTypeStr string
		var createdUnix int64

		err := rows.Scan(&link.ID, &link.SourceNoteID, &link.TargetSlug, &linkTypeStr, &createdUnix)
		if err != nil {
			logger.Error("GetOrphanedLinks", "error", err)
			return nil, fmt.Errorf("failed to scan orphaned link: %w", err)
		}

		link.LinkType = models.LinkType(linkTypeStr)
		link.CreatedAt = time.Unix(createdUnix, 0)
		links = append(links, link)
	}

	return links, nil
}

// UpdateNoteLinkTargetID updates the target_note_id for a link.
func (r *NotesRepository) UpdateNoteLinkTargetID(linkID, targetNoteID string) error {
	logger.Debug("UpdateNoteLinkTargetID", "link_id", linkID, "target_note_id", targetNoteID)

	_, err := r.db.DB().Exec(
		"UPDATE note_links SET target_note_id = ? WHERE id = ?",
		targetNoteID, linkID,
	)
	if err != nil {
		logger.Error("UpdateNoteLinkTargetID", "error", err, "link_id", linkID)
		return fmt.Errorf("failed to update link target: %w", err)
	}

	return nil
}

// GetLinksBySourceNote retrieves all links from a source note.
func (r *NotesRepository) GetLinksBySourceNote(sourceNoteID string) ([]models.NoteLink, error) {
	logger.Debug("GetLinksBySourceNote", "source_note_id", sourceNoteID)

	rows, err := r.db.DB().Query(
		`SELECT id, source_note_id, target_slug, target_note_id, link_type, created_at
		 FROM note_links WHERE source_note_id = ?`,
		sourceNoteID,
	)
	if err != nil {
		logger.Error("GetLinksBySourceNote", "error", err, "source_note_id", sourceNoteID)
		return nil, fmt.Errorf("failed to query links: %w", err)
	}
	defer rows.Close()

	var links []models.NoteLink
	for rows.Next() {
		var link models.NoteLink
		var linkTypeStr string
		var createdUnix int64
		var targetNoteID sql.NullString

		err := rows.Scan(&link.ID, &link.SourceNoteID, &link.TargetSlug, &targetNoteID, &linkTypeStr, &createdUnix)
		if err != nil {
			logger.Error("GetLinksBySourceNote", "error", err)
			return nil, fmt.Errorf("failed to scan link: %w", err)
		}

		if targetNoteID.Valid {
			link.TargetNoteID = targetNoteID.String
		}
		link.LinkType = models.LinkType(linkTypeStr)
		link.CreatedAt = time.Unix(createdUnix, 0)
		links = append(links, link)
	}

	return links, nil
}

// GetAllNotes retrieves all notes from the database.
func (r *NotesRepository) GetAllNotes() ([]*models.Note, error) {
	logger.Debug("GetAllNotes")

	rows, err := r.db.DB().Query(
		`SELECT id, title, slug, file_path, created_at, updated_at, tags
		 FROM notes
		 ORDER BY created_at DESC`,
	)
	if err != nil {
		logger.Error("GetAllNotes", "error", err)
		return nil, fmt.Errorf("failed to query notes: %w", err)
	}
	defer rows.Close()

	var notes []*models.Note
	for rows.Next() {
		var note models.Note
		var tagsStr sql.NullString
		var createdUnix, updatedUnix int64

		err := rows.Scan(&note.ID, &note.Title, &note.Slug, &note.FilePath, &createdUnix, &updatedUnix, &tagsStr)
		if err != nil {
			logger.Error("GetAllNotes", "error", err)
			return nil, fmt.Errorf("failed to scan note: %w", err)
		}

		note.CreatedAt = time.Unix(createdUnix, 0)
		note.UpdatedAt = time.Unix(updatedUnix, 0)

		if tagsStr.Valid && tagsStr.String != "" {
			note.Tags = strings.Split(tagsStr.String, ",")
		}

		notes = append(notes, &note)
	}

	if err := rows.Err(); err != nil {
		logger.Error("GetAllNotes", "error", err)
		return nil, fmt.Errorf("failed to iterate notes: %w", err)
	}

	logger.Debug("GetAllNotes", "count", len(notes))
	return notes, nil
}

// UpdateNoteTimestamp updates the updated_at timestamp for a note.
func (r *NotesRepository) UpdateNoteTimestamp(noteID string, timestamp time.Time) error {
	logger.Debug("UpdateNoteTimestamp", "note_id", noteID, "timestamp", timestamp)

	_, err := r.db.DB().Exec(
		"UPDATE notes SET updated_at = ? WHERE id = ?",
		timestamp.Unix(), noteID,
	)
	if err != nil {
		logger.Error("UpdateNoteTimestamp", "error", err, "note_id", noteID)
		return fmt.Errorf("failed to update note timestamp: %w", err)
	}

	return nil
}
