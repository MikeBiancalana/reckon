package textmigrate

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/MikeBiancalana/reckon/internal/models"
	notessvc "github.com/MikeBiancalana/reckon/internal/service"
	"github.com/stretchr/testify/require"
)

// Given a completed migration, when verify runs then it reports per-type
// counts matching the source record counts and zero unexpected
// duplicate_ulid/alias_collision warnings from a fresh index rebuild.
func TestImporter_Verify_CountsAndZeroUnexpectedWarnings(t *testing.T) {
	source := newFixtureSource(t)
	dest, _ := newFixtureDest(t)
	db := openFixtureDB(t, source)

	tk1 := fixtureTask("Buy milk", "", time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC))
	tk2 := fixtureTask("Ship the release", "", time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC))
	writeFixtureTaskFile(t, source, "task1.md", tk1)
	writeFixtureTaskFile(t, source, "task2.md", tk2)

	writeFixtureNote(t, source, db, "Grocery Plan", "grocery-plan", "grocery-plan.md", nil,
		time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC), "Weekly grocery plan.\n")

	fixtureChecklist(t, db, "morning", []string{"Check email"})

	writeFixtureJournalDay(t, source, &journal.Journal{
		Date: "2026-01-07",
		LogEntries: []journal.LogEntry{
			{Timestamp: time.Date(2026, 1, 7, 9, 0, 0, 0, time.UTC), Content: "Started work", Position: 0},
		},
	})

	imp := &Importer{Source: source, Dest: dest}
	report, err := imp.Run()
	require.NoError(t, err)
	require.Equal(t, 0, totalErrored(report))

	vr, err := imp.Verify()
	require.NoError(t, err)
	require.True(t, vr.OK)
	require.Empty(t, vr.UnexpectedWarnings)
	require.Empty(t, vr.UnresolvedAliases)
	require.Equal(t, 2, vr.Counts["todo"])
	require.Equal(t, 1, vr.Counts["note"])
	require.Equal(t, 1, vr.Counts["checklist-template"])
	require.Equal(t, 1, vr.Counts["checklist-run"])
	require.Equal(t, 1, vr.Counts["log-day"])
	require.Equal(t, 1, vr.Counts["log-entry"])
}

// Given two source records (a note filename collision) whose aliases would
// otherwise collide, when migrated the collision is deterministically
// disambiguated -- never silently resolved to one arbitrary winner -- and
// verify reports no unexpected warning for it, since the importer already
// resolved it up front.
func TestImporter_Verify_NoteCollisionDisambiguatedNotAWarning(t *testing.T) {
	source := newFixtureSource(t)
	dest, _ := newFixtureDest(t)
	db := openFixtureDB(t, source)

	writeSeedNode(t, dest, "notes/weekly-review.md", "note", []string{"weekly-review"},
		map[string]string{"title": "Weekly Review (native)"}, "Native note body.\n",
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	writeFixtureNote(t, source, db, "Weekly Review", "weekly-review", "weekly-review.md", nil,
		time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC), "Legacy weekly review body.\n")

	imp := &Importer{Source: source, Dest: dest}
	_, err := imp.Run()
	require.NoError(t, err)

	vr, err := imp.Verify()
	require.NoError(t, err)
	require.Empty(t, vr.UnexpectedWarnings)
}

// Given a task and a note that happen to share the exact same legacy id
// string (not enforced by any uniqueness constraint across the two legacy
// tables), when migrated each keeps its own alias, and verify's index
// rebuild surfaces the resulting alias collision as an unexpected warning
// rather than silently resolving a reference to one arbitrary winner.
func TestImporter_Verify_CrossTypeXIDCollisionSurfacedAsWarning(t *testing.T) {
	source := newFixtureSource(t)
	dest, _ := newFixtureDest(t)
	db := openFixtureDB(t, source)

	const sharedID = "shared-legacy-id-123"

	tk := journal.Task{
		ID:        sharedID,
		Text:      "Buy milk",
		Status:    journal.TaskOpen,
		CreatedAt: time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
	}
	writeFixtureTaskFile(t, source, "task1.md", &tk)

	note := models.NewNote("Grocery Plan", "grocery-plan", "grocery-plan.md", nil)
	note.ID = sharedID
	note.CreatedAt = time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC)
	note.UpdatedAt = note.CreatedAt
	require.NoError(t, notessvc.NewNotesRepository(db).SaveNote(note))
	notePath := filepath.Join(source, "notes", "grocery-plan.md")
	require.NoError(t, os.WriteFile(notePath, []byte("---\ntitle: Grocery Plan\n---\n\nWeekly grocery plan.\n"), 0o644))

	imp := &Importer{Source: source, Dest: dest}
	report, err := imp.Run()
	require.NoError(t, err)
	require.Equal(t, 0, totalErrored(report), "both records still migrate; the collision is a verify-time concern, not an import-time failure")

	vr, err := imp.Verify()
	require.NoError(t, err)
	require.False(t, vr.OK, "a cross-type alias collision must never be silently resolved")
	require.NotEmpty(t, vr.UnexpectedWarnings)
}
