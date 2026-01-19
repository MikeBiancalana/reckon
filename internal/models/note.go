package models

import (
	"time"

	"github.com/rs/xid"
)

// Zettelkasten Notes Directory Structure
// The notes are stored in the following structure:
//
//	~/.reckon/notes/
//	├── yyyy/
//	│   ├── yyyy-mm/
//	│   │   ├── yyyy-mm-dd-slug.md
//	│   │   └── ...
//	│   └── ...
//	└── ...
//
// Each note is a markdown file stored in a date-based hierarchy.
// The file_path field in the Note model stores the relative path from
// the notes root directory (e.g., "2024/2024-01/2024-01-15-my-slug.md")
//
// The slug field is a URL-safe identifier derived from the note title
// and date, used for linking between notes.

type LinkType string

const (
	LinkTypeReference LinkType = "reference"
	LinkTypeParent    LinkType = "parent"
	LinkTypeChild     LinkType = "child"
	LinkTypeRelated   LinkType = "related"
)

type Note struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Slug      string     `json:"slug"`
	FilePath  string     `json:"file_path"`
	Tags      []string   `json:"tags"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	Links     []NoteLink `json:"links,omitempty"`
}

func NewNote(title, slug, filePath string, tags []string) *Note {
	now := time.Now()
	return &Note{
		ID:        xid.New().String(),
		Title:     title,
		Slug:      slug,
		FilePath:  filePath,
		Tags:      tags,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (n *Note) UpdateTitle(title string) {
	n.Title = title
	n.UpdatedAt = time.Now()
}

func (n *Note) AddTag(tag string) {
	for _, t := range n.Tags {
		if t == tag {
			return
		}
	}
	n.Tags = append(n.Tags, tag)
	n.UpdatedAt = time.Now()
}

func (n *Note) RemoveTag(tag string) {
	newTags := make([]string, 0, len(n.Tags))
	for _, t := range n.Tags {
		if t != tag {
			newTags = append(newTags, t)
		}
	}
	n.Tags = newTags
	n.UpdatedAt = time.Now()
}

type NoteLink struct {
	ID           string    `json:"id"`
	SourceNoteID string    `json:"source_note_id"`
	TargetSlug   string    `json:"target_slug"`
	TargetNoteID string    `json:"target_note_id,omitempty"`
	LinkType     LinkType  `json:"link_type"`
	CreatedAt    time.Time `json:"created_at"`
	SourceNote   *Note     `json:"source_note,omitempty"`
	TargetNote   *Note     `json:"target_note,omitempty"`
}

func NewNoteLink(sourceNoteID, targetSlug string, linkType LinkType) *NoteLink {
	return &NoteLink{
		ID:           xid.New().String(),
		SourceNoteID: sourceNoteID,
		TargetSlug:   targetSlug,
		LinkType:     linkType,
		CreatedAt:    time.Now(),
	}
}

func (n *NoteLink) UpdateTargetNoteID(noteID string) {
	n.TargetNoteID = noteID
}
