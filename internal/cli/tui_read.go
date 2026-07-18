package cli

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/MikeBiancalana/reckon/internal/models"
	"github.com/MikeBiancalana/reckon/internal/tui/components"
)

// loadLogEntries loads every index `log-entry` node (internal/node/
// logparser.go:118) as a components.LogEntryRow, newest first. Log entries
// are distinct index nodes, not raw day-file text (plan.md scenario 2).
func loadLogEntries(db *sql.DB) ([]components.LogEntryRow, error) {
	rows, err := db.Query("SELECT id, time, body FROM nodes WHERE type = 'log-entry' ORDER BY time DESC")
	if err != nil {
		return nil, fmt.Errorf("tui: query log entries: %w", err)
	}
	type row struct{ id, time, body string }
	var candidates []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.time, &r.body); err != nil {
			rows.Close()
			return nil, fmt.Errorf("tui: scan log entry: %w", err)
		}
		candidates = append(candidates, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("tui: iterate log entries: %w", err)
	}
	rows.Close()

	entries := []components.LogEntryRow{}
	for _, r := range candidates {
		// entryTime may be "" for a malformed/hand-authored header
		// (logparser.go's buildLogEntry) -- treat that as the zero
		// time.Time rather than erroring the whole load.
		var ts time.Time
		if r.time != "" {
			ts, _ = time.Parse(time.RFC3339, r.time)
		}
		props, err := loadTodoProps(db, r.id)
		if err != nil {
			return nil, err
		}
		kind := props["kind"]
		if kind == "" {
			kind = "note"
		}
		entries = append(entries, components.LogEntryRow{
			ID:        r.id,
			Timestamp: ts,
			Content:   strings.TrimSpace(r.body),
			EntryType: kind,
		})
	}
	return entries, nil
}

// listNotes loads every index `note` node as a *models.Note (Title/Slug),
// for components.NotePicker's browse-mode list.
func listNotes(db *sql.DB) ([]*models.Note, error) {
	rows, err := db.Query("SELECT id FROM nodes WHERE type = 'note'")
	if err != nil {
		return nil, fmt.Errorf("tui: query notes: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("tui: scan note: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("tui: iterate notes: %w", err)
	}
	rows.Close()

	notes := []*models.Note{}
	for _, id := range ids {
		n, err := loadNoteDisplay(db, id)
		if err != nil {
			return nil, err
		}
		if n != nil {
			notes = append(notes, n)
		}
	}
	return notes, nil
}

// loadNoteDisplay resolves id to a *models.Note{ID,Title,Slug} for display
// (picker rows, link endpoints): slug from the file's loc stem (always the
// note's current filename, so it stays correct across a rename without a
// second alias-table query), title from the explicit `title` frontmatter
// prop (notes carry no body-derived title -- internal/index/reconcile.go
// only derives one for type=="todo") falling back to slug when absent.
// Returns (nil, nil) if id doesn't resolve to any node (a dangling edge
// target), which callers treat as "unresolved", not an error.
func loadNoteDisplay(db *sql.DB, id string) (*models.Note, error) {
	var loc string
	err := db.QueryRow("SELECT loc FROM nodes WHERE id = ?", id).Scan(&loc)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("tui: load note %q: %w", id, err)
	}
	props, err := loadTodoProps(db, id)
	if err != nil {
		return nil, err
	}
	slug := strings.TrimSuffix(filepath.Base(loc), ".md")
	title := props["title"]
	if title == "" {
		title = slug
	}
	return &models.Note{ID: id, Title: title, Slug: slug}, nil
}

// resolveNoteIDBySlug resolves a note's slug (its self-minted first alias,
// per createNote) to its index node id, for the notes-pane composite's
// browse->inspect transition (components.NotePickerSelectMsg carries only
// the slug). Returns "" (no error) if slug matches nothing.
func resolveNoteIDBySlug(db *sql.DB, slug string) (string, error) {
	var id string
	err := db.QueryRow("SELECT id FROM aliases WHERE alias = ? LIMIT 1", slug).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("tui: resolve note slug %q: %w", slug, err)
	}
	return id, nil
}

// loadNotesPaneLinks loads noteID's outgoing links and backlinks (via
// loadNoteForwardLinks/loadNoteBacklinks, note_v1.go) as
// components.LinkDisplayItem rows for the notes pane's inspect mode.
// Backlinks must resolve NoteLink.SourceNote (notes_pane.go:231,455
// dereferences it) or the pane panics.
func loadNotesPaneLinks(db *sql.DB, noteID string) (outgoing, backlinks []components.LinkDisplayItem, err error) {
	fwd, err := loadNoteForwardLinks(db, noteID)
	if err != nil {
		return nil, nil, err
	}
	outgoing = []components.LinkDisplayItem{}
	for _, l := range fwd {
		item := components.LinkDisplayItem{
			NoteLink: models.NoteLink{TargetSlug: l.Dst, TargetNoteID: l.DstKey},
		}
		if l.DstKey != "" {
			target, terr := loadNoteDisplay(db, l.DstKey)
			if terr != nil {
				return nil, nil, terr
			}
			if target != nil {
				item.NoteLink.TargetNote = target
				item.DisplayText = target.Title
				item.IsResolved = true
			}
		}
		if item.DisplayText == "" {
			item.DisplayText = l.Dst
		}
		outgoing = append(outgoing, item)
	}

	bls, err := loadNoteBacklinks(db, noteID)
	if err != nil {
		return nil, nil, err
	}
	backlinks = []components.LinkDisplayItem{}
	for _, l := range bls {
		item := components.LinkDisplayItem{
			NoteLink: models.NoteLink{SourceNoteID: l.Src},
		}
		src, serr := loadNoteDisplay(db, l.Src)
		if serr != nil {
			return nil, nil, serr
		}
		if src != nil {
			item.NoteLink.SourceNote = src
			item.DisplayText = src.Title
			item.IsResolved = true
		} else {
			item.DisplayText = l.Src
		}
		backlinks = append(backlinks, item)
	}
	return outgoing, backlinks, nil
}
