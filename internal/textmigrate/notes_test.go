package textmigrate

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Given a fixture DB with several notes, each backed by a real on-disk
// legacy file, when the importer runs then one notes/<slug>.md file exists
// per note, with title and aliases matching the DB row and body matching
// the legacy file's body content.
func TestImporter_Notes_TitleAliasesBody(t *testing.T) {
	source := newFixtureSource(t)
	dest, _ := newFixtureDest(t)
	db := openFixtureDB(t, source)

	note1 := writeFixtureNote(t, source, db, "Grocery Plan", "grocery-plan", "grocery-plan.md", []string{"errands"},
		time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC), "Weekly grocery plan.\n")
	note2 := writeFixtureNote(t, source, db, "OAuth Flow Patterns", "oauth-flow-patterns", "oauth-flow-patterns.md", nil,
		time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC), "Notes on refresh tokens.\n")

	imp := &Importer{Source: source, Dest: dest}
	report, err := imp.Run()
	require.NoError(t, err)
	require.Len(t, report.Notes.Created, 2)

	byID := map[string]RecordOutcome{}
	for _, c := range report.Notes.Created {
		byID[c.SourceID] = c
	}

	n1 := mustParseVaultFile(t, dest, byID[note1.ID].Path)
	require.Equal(t, "note", n1.Type)
	require.Equal(t, "Grocery Plan", n1.Props["title"])
	require.Contains(t, n1.Aliases, note1.ID)
	require.Contains(t, n1.Aliases, "grocery-plan")
	require.Contains(t, n1.Body, "Weekly grocery plan.")

	n2 := mustParseVaultFile(t, dest, byID[note2.ID].Path)
	require.Equal(t, "OAuth Flow Patterns", n2.Props["title"])
	require.Contains(t, n2.Body, "Notes on refresh tokens.")
}

// Given a migrated note whose old slug was known, and a different on-disk
// note file containing a literal wikilink to that old slug, when the index
// reconciles then that edge resolves to the migrated note (not dangling),
// proving old inbound links survive the migration.
func TestImporter_Notes_OldSlugInboundWikilinkResolves(t *testing.T) {
	source := newFixtureSource(t)
	dest, _ := newFixtureDest(t)
	db := openFixtureDB(t, source)

	writeFixtureNote(t, source, db, "Git Rebase", "git-rebase", "git-rebase.md", nil,
		time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC), "How to rebase cleanly.\n")

	imp := &Importer{Source: source, Dest: dest}
	report, err := imp.Run()
	require.NoError(t, err)
	require.Len(t, report.Notes.Created, 1)

	// A separate, independently-authored vault note links to the old slug.
	writeSeedNode(t, dest, "notes/other-note.md", "note", nil, map[string]string{"title": "Other Note"},
		"See [[git-rebase]] for the workflow.\n", time.Date(2026, 1, 8, 0, 0, 0, 0, time.UTC))

	vr, err := imp.Verify()
	require.NoError(t, err)
	require.True(t, vr.OK)
	require.NotContains(t, vr.UnresolvedAliases, "git-rebase")
}

// Given a note whose new filename slug differs from its old slug after
// collision disambiguation, when the index reconciles then both the old and
// new slug aliases are present and both resolve.
func TestImporter_Notes_SlugCollisionDisambiguatedBothAliasesResolve(t *testing.T) {
	source := newFixtureSource(t)
	dest, _ := newFixtureDest(t)
	db := openFixtureDB(t, source)

	// A note already exists in the destination vault claiming the slug
	// "weekly-review" before the legacy note of the same slug is migrated.
	writeSeedNode(t, dest, "notes/weekly-review.md", "note", []string{"weekly-review"},
		map[string]string{"title": "Weekly Review (native)"}, "Native note body.\n",
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	legacy := writeFixtureNote(t, source, db, "Weekly Review", "weekly-review", "weekly-review.md", nil,
		time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC), "Legacy weekly review body.\n")

	imp := &Importer{Source: source, Dest: dest}
	report, err := imp.Run()
	require.NoError(t, err)
	require.Len(t, report.Notes.Created, 1)

	migrated := mustParseVaultFile(t, dest, report.Notes.Created[0].Path)
	require.NotEqual(t, "notes/weekly-review.md", report.Notes.Created[0].Path, "disambiguated filename must not collide")
	require.Contains(t, migrated.Aliases, "weekly-review", "old slug is retained as an alias regardless of the new filename")
	require.Contains(t, migrated.Aliases, legacy.ID)

	vr, err := imp.Verify()
	require.NoError(t, err)
	require.Empty(t, vr.UnexpectedWarnings, "a disambiguated collision must not surface as an alias_collision warning")
}

// Given a note DB row whose file_path points at a file that no longer
// exists on disk, when migrated that record is reported as an error naming
// the source id, and the run continues with the remaining notes.
func TestImporter_Notes_MissingFileReportedErrorContinues(t *testing.T) {
	source := newFixtureSource(t)
	dest, _ := newFixtureDest(t)
	db := openFixtureDB(t, source)

	good := writeFixtureNote(t, source, db, "Grocery Plan", "grocery-plan", "grocery-plan.md", nil,
		time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC), "Weekly grocery plan.\n")

	// DB row present, but no file written under source/notes/ -- simulates
	// DB/file drift.
	missing := writeFixtureNote(t, source, db, "Ghost Note", "ghost-note", "ghost-note.md", nil,
		time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC), "unused")
	require.NoError(t, os.Remove(filepath.Join(source, "notes", "ghost-note.md")))

	imp := &Importer{Source: source, Dest: dest}
	report, err := imp.Run()
	require.NoError(t, err)

	require.Len(t, report.Notes.Created, 1)
	require.Equal(t, good.ID, report.Notes.Created[0].SourceID)
	require.Len(t, report.Notes.Errored, 1)
	require.Equal(t, missing.ID, report.Notes.Errored[0].SourceID)
	require.NotEmpty(t, report.Notes.Errored[0].Error)
}
