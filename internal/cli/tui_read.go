package cli

import (
	"database/sql"

	"github.com/MikeBiancalana/reckon/internal/models"
	"github.com/MikeBiancalana/reckon/internal/tui/components"
)

// loadLogEntries loads every index `log-entry` node (internal/node/
// logparser.go:118) as a components.LogEntryRow, newest first. Log entries
// are distinct index nodes, not raw day-file text (plan.md scenario 2).
func loadLogEntries(db *sql.DB) ([]components.LogEntryRow, error) {
	// TODO(reckon-fnqs.8): implement
	return nil, nil
}

// listNotes loads every index `note` node as a *models.Note (Title/Slug),
// for components.NotePicker's browse-mode list.
func listNotes(db *sql.DB) ([]*models.Note, error) {
	// TODO(reckon-fnqs.8): implement
	return nil, nil
}

// loadNotesPaneLinks loads noteID's outgoing links and backlinks (via
// loadNoteForwardLinks/loadNoteBacklinks, note_v1.go) as
// components.LinkDisplayItem rows for the notes pane's inspect mode.
// Backlinks must resolve NoteLink.SourceNote (notes_pane.go:231,455
// dereferences it) or the pane panics.
func loadNotesPaneLinks(db *sql.DB, noteID string) (outgoing, backlinks []components.LinkDisplayItem, err error) {
	// TODO(reckon-fnqs.8): implement
	return nil, nil, nil
}
