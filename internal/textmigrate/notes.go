package textmigrate

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MikeBiancalana/reckon/internal/models"
	"github.com/MikeBiancalana/reckon/internal/node"
	notessvc "github.com/MikeBiancalana/reckon/internal/service"
	"github.com/MikeBiancalana/reckon/internal/storage"
)

// convertNote builds the note node.Node for one legacy DB note row plus its
// already-read legacy body content. existingSlugs is consulted for
// filename-collision disambiguation; the returned slug is the new
// (possibly disambiguated) filename slug. The note's old slug is always
// retained as an alias regardless of disambiguation.
func convertNote(dbNote *models.Note, body string, existingSlugs map[string]bool) (n *node.Node, newSlug string, err error) {
	newSlug = disambiguateSlug(dbNote.Slug, existingSlugs)

	aliases := []string{newSlug}
	seen := map[string]bool{newSlug: true}
	for _, a := range []string{dbNote.ID, dbNote.Slug} {
		if !seen[a] {
			seen[a] = true
			aliases = append(aliases, a)
		}
	}

	n = node.NewNode("note", legacyAuthor, body)
	n.ULID = node.MintAt(dbNote.CreatedAt)
	n.Time = dbNote.CreatedAt.UTC().Format(time.RFC3339)
	n.Aliases = aliases

	props := map[string]string{"title": dbNote.Title}
	if len(dbNote.Tags) > 0 {
		props["tags"] = "[" + strings.Join(dbNote.Tags, ", ") + "]"
	}
	n.Props = props

	return n, newSlug, nil
}

// disambiguateSlug returns candidate unchanged if it is not already claimed
// in existingSlugs, else appends a deterministic "-2", "-3", ... suffix
// until it finds a free one (EC-2), mirroring note_v1.go's slugCollision
// disambiguation.
func disambiguateSlug(candidate string, existingSlugs map[string]bool) string {
	if !existingSlugs[candidate] {
		return candidate
	}
	for i := 2; ; i++ {
		c := fmt.Sprintf("%s-%d", candidate, i)
		if !existingSlugs[c] {
			return c
		}
	}
}

// scanNoteSlugs recursively collects every candidate slug already claimed
// under notesDir: each file's own basename (minus .md) and every alias on
// each parsed file. This single set drives both filename-collision
// disambiguation (EC-2) and the idempotency skip check (a source note's old
// xid already present as an alias means it was already migrated).
// Unparsable/CRLF files are skipped rather than aborting the scan, and a
// missing directory yields an empty set (nothing migrated yet).
func scanNoteSlugs(notesDir string) (map[string]bool, error) {
	slugs := map[string]bool{}
	walkErr := filepath.WalkDir(notesDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		slugs[strings.TrimSuffix(filepath.Base(path), ".md")] = true

		raw, err := os.ReadFile(path)
		if err != nil || bytes.Contains(raw, []byte("\r\n")) {
			return nil
		}
		n, err := node.Parse(raw)
		if err != nil {
			return nil
		}
		for _, a := range n.Aliases {
			slugs[a] = true
		}
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("scan %s: %w", notesDir, walkErr)
	}
	return slugs, nil
}

// runNotes migrates every legacy note DB row (metadata authority) paired
// with its on-disk legacy body file into notes/<slug>.md.
func (imp *Importer) runNotes(report *Report) error {
	db, err := storage.NewDatabase(filepath.Join(imp.Source, "reckon.db"))
	if err != nil {
		return fmt.Errorf("open legacy db: %w", err)
	}
	defer db.Close()

	dbNotes, err := notessvc.NewNotesRepository(db).GetAllNotes()
	if err != nil {
		return fmt.Errorf("read legacy notes: %w", err)
	}

	notesDir := filepath.Join(imp.Dest, "notes")
	existingSlugs, err := scanNoteSlugs(notesDir)
	if err != nil {
		return fmt.Errorf("scan existing notes: %w", err)
	}

	for _, dbNote := range dbNotes {
		if existingSlugs[dbNote.ID] {
			report.Notes.addSkipped(dbNote.ID, "already migrated (alias present)")
			continue
		}

		legacyPath := filepath.Join(imp.Source, "notes", dbNote.FilePath)
		raw, err := os.ReadFile(legacyPath)
		if err != nil {
			report.Notes.addErrored(dbNote.ID, fmt.Errorf("read legacy body %s: %w", dbNote.FilePath, err))
			continue
		}
		if bytes.Contains(raw, []byte("\r\n")) {
			report.Notes.addErrored(dbNote.ID, fmt.Errorf("legacy body %s: CRLF line endings are not supported", dbNote.FilePath))
			continue
		}
		legacy, err := node.Parse(raw)
		if err != nil {
			report.Notes.addErrored(dbNote.ID, fmt.Errorf("parse legacy body %s: %w", dbNote.FilePath, err))
			continue
		}

		built, newSlug, err := convertNote(dbNote, legacy.Body, existingSlugs)
		if err != nil {
			report.Notes.addErrored(dbNote.ID, err)
			continue
		}

		parsed, err := renderAndParse(built)
		if err != nil {
			report.Notes.addErrored(dbNote.ID, fmt.Errorf("note %s: %w", dbNote.ID, err))
			continue
		}

		relPath := "notes/" + newSlug + ".md"
		if imp.DryRun {
			report.Notes.addCreated(dbNote.ID, relPath, parsed.ULID)
			continue
		}

		if err := os.MkdirAll(notesDir, 0o755); err != nil {
			report.Notes.addErrored(dbNote.ID, fmt.Errorf("create notes dir: %w", err))
			continue
		}
		if err := writeFileAtomic(filepath.Join(notesDir, newSlug+".md"), parsed.Serialize()); err != nil {
			report.Notes.addErrored(dbNote.ID, fmt.Errorf("write %s: %w", relPath, err))
			continue
		}

		report.Notes.addCreated(dbNote.ID, relPath, parsed.ULID)
		existingSlugs[newSlug] = true
		existingSlugs[dbNote.ID] = true
		existingSlugs[dbNote.Slug] = true
	}

	return nil
}
