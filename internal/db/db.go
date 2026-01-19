package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/MikeBiancalana/reckon/internal/models"
	"github.com/MikeBiancalana/reckon/internal/storage"
)

type NoteRepository struct {
	db *storage.Database
}

func NewNoteRepository(db *storage.Database) *NoteRepository {
	return &NoteRepository{db: db}
}

func (r *NoteRepository) CreateNote(note *models.Note) error {
	tags := strings.Join(note.Tags, ",")
	_, err := r.db.DB().Exec(`
		INSERT INTO notes (id, title, slug, file_path, created_at, updated_at, tags)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`,
		note.ID, note.Title, note.Slug, note.FilePath,
		note.CreatedAt.Unix(), note.UpdatedAt.Unix(), tags)
	if err != nil {
		return fmt.Errorf("failed to create note: %w", err)
	}
	return nil
}

func (r *NoteRepository) GetNoteByID(id string) (*models.Note, error) {
	var note models.Note
	var tags string
	var createdAt, updatedAt int64

	err := r.db.DB().QueryRow(`
		SELECT id, title, slug, file_path, created_at, updated_at, tags
		FROM notes WHERE id = ?
	`, id).Scan(&note.ID, &note.Title, &note.Slug, &note.FilePath,
		&createdAt, &updatedAt, &tags)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get note: %w", err)
	}

	note.CreatedAt = time.Unix(createdAt, 0)
	note.UpdatedAt = time.Unix(updatedAt, 0)
	if tags != "" {
		note.Tags = strings.Split(tags, ",")
	}
	return &note, nil
}

func (r *NoteRepository) GetNoteBySlug(slug string) (*models.Note, error) {
	var note models.Note
	var tags string
	var createdAt, updatedAt int64

	err := r.db.DB().QueryRow(`
		SELECT id, title, slug, file_path, created_at, updated_at, tags
		FROM notes WHERE slug = ?
	`, slug).Scan(&note.ID, &note.Title, &note.Slug, &note.FilePath,
		&createdAt, &updatedAt, &tags)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get note by slug: %w", err)
	}

	note.CreatedAt = time.Unix(createdAt, 0)
	note.UpdatedAt = time.Unix(updatedAt, 0)
	if tags != "" {
		note.Tags = strings.Split(tags, ",")
	}
	return &note, nil
}

func (r *NoteRepository) UpdateNote(note *models.Note) error {
	tags := strings.Join(note.Tags, ",")
	_, err := r.db.DB().Exec(`
		UPDATE notes SET title = ?, slug = ?, file_path = ?, tags = ?, updated_at = ?
		WHERE id = ?
	`,
		note.Title, note.Slug, note.FilePath, tags,
		time.Now().Unix(), note.ID)
	if err != nil {
		return fmt.Errorf("failed to update note: %w", err)
	}
	return nil
}

func (r *NoteRepository) DeleteNote(id string) error {
	_, err := r.db.DB().Exec("DELETE FROM notes WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete note: %w", err)
	}
	return nil
}

func (r *NoteRepository) ListNotes() ([]*models.Note, error) {
	rows, err := r.db.DB().Query(`
		SELECT id, title, slug, file_path, created_at, updated_at, tags
		FROM notes ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list notes: %w", err)
	}
	defer rows.Close()

	var notes []*models.Note
	for rows.Next() {
		var note models.Note
		var tags string
		var createdAt, updatedAt int64

		err := rows.Scan(&note.ID, &note.Title, &note.Slug, &note.FilePath,
			&createdAt, &updatedAt, &tags)
		if err != nil {
			return nil, fmt.Errorf("failed to scan note: %w", err)
		}

		note.CreatedAt = time.Unix(createdAt, 0)
		note.UpdatedAt = time.Unix(updatedAt, 0)
		if tags != "" {
			note.Tags = strings.Split(tags, ",")
		}
		notes = append(notes, &note)
	}
	return notes, nil
}

func (r *NoteRepository) CreateLink(link *models.NoteLink) error {
	_, err := r.db.DB().Exec(`
		INSERT INTO note_links (id, source_note_id, target_slug, target_note_id, link_type, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`,
		link.ID, link.SourceNoteID, link.TargetSlug, link.TargetNoteID,
		link.LinkType, link.CreatedAt.Unix())
	if err != nil {
		return fmt.Errorf("failed to create link: %w", err)
	}
	return nil
}

func (r *NoteRepository) GetLinksBySourceID(sourceNoteID string) ([]*models.NoteLink, error) {
	rows, err := r.db.DB().Query(`
		SELECT id, source_note_id, target_slug, target_note_id, link_type, created_at
		FROM note_links WHERE source_note_id = ?
	`, sourceNoteID)
	if err != nil {
		return nil, fmt.Errorf("failed to get links: %w", err)
	}
	defer rows.Close()

	var links []*models.NoteLink
	for rows.Next() {
		var link models.NoteLink
		var createdAt int64

		err := rows.Scan(&link.ID, &link.SourceNoteID, &link.TargetSlug,
			&link.TargetNoteID, &link.LinkType, &createdAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan link: %w", err)
		}

		link.CreatedAt = time.Unix(createdAt, 0)
		links = append(links, &link)
	}
	return links, nil
}

func (r *NoteRepository) GetBacklinks(targetNoteID string) ([]*models.NoteLink, error) {
	rows, err := r.db.DB().Query(`
		SELECT id, source_note_id, target_slug, target_note_id, link_type, created_at
		FROM note_links WHERE target_note_id = ?
	`, targetNoteID)
	if err != nil {
		return nil, fmt.Errorf("failed to get backlinks: %w", err)
	}
	defer rows.Close()

	var links []*models.NoteLink
	for rows.Next() {
		var link models.NoteLink
		var createdAt int64

		err := rows.Scan(&link.ID, &link.SourceNoteID, &link.TargetSlug,
			&link.TargetNoteID, &link.LinkType, &createdAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan backlink: %w", err)
		}

		link.CreatedAt = time.Unix(createdAt, 0)
		links = append(links, &link)
	}
	return links, nil
}

func (r *NoteRepository) DeleteLink(id string) error {
	_, err := r.db.DB().Exec("DELETE FROM note_links WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete link: %w", err)
	}
	return nil
}

func (r *NoteRepository) DeleteLinksBySourceID(sourceNoteID string) error {
	_, err := r.db.DB().Exec("DELETE FROM note_links WHERE source_note_id = ?", sourceNoteID)
	if err != nil {
		return fmt.Errorf("failed to delete links: %w", err)
	}
	return nil
}

func (r *NoteRepository) GetLink(sourceNoteID, targetSlug string) (*models.NoteLink, error) {
	var link models.NoteLink
	var createdAt int64

	err := r.db.DB().QueryRow(`
		SELECT id, source_note_id, target_slug, target_note_id, link_type, created_at
		FROM note_links WHERE source_note_id = ? AND target_slug = ?
	`, sourceNoteID, targetSlug).Scan(&link.ID, &link.SourceNoteID, &link.TargetSlug,
		&link.TargetNoteID, &link.LinkType, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get link: %w", err)
	}

	link.CreatedAt = time.Unix(createdAt, 0)
	return &link, nil
}

func (r *NoteRepository) UpdateLinkTargetNoteID(linkID, targetNoteID string) error {
	_, err := r.db.DB().Exec(`
		UPDATE note_links SET target_note_id = ? WHERE id = ?
	`, targetNoteID, linkID)
	if err != nil {
		return fmt.Errorf("failed to update link target: %w", err)
	}
	return nil
}
