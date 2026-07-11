package textmigrate

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/MikeBiancalana/reckon/internal/node"
	"github.com/stretchr/testify/require"
)

// Given every kind of migrated node (a todo, a note, a checklist template,
// a checklist run, and a log day), when each is re-parsed via node.Parse,
// then Serialize is byte-identical to the file on disk -- the node
// package's own round-trip invariant holds on importer-generated files
// exactly as it does on hand-authored ones.
func TestImporter_RoundTrip_ByteIdenticalAfterReparse(t *testing.T) {
	source := newFixtureSource(t)
	dest, _ := newFixtureDest(t)
	db := openFixtureDB(t, source)

	tk := fixtureTask("Buy milk", "get 2%", time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC))
	writeFixtureTaskFile(t, source, "task1.md", tk)

	writeFixtureNote(t, source, db, "Grocery Plan", "grocery-plan", "grocery-plan.md", []string{"errands"},
		time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC), "Weekly grocery plan with a [[wikilink]].\n")

	fixtureChecklist(t, db, "morning", []string{"Check email", "Review calendar"})

	writeFixtureJournalDay(t, source, &journal.Journal{
		Date: "2026-01-07",
		LogEntries: []journal.LogEntry{
			{Timestamp: time.Date(2026, 1, 7, 9, 0, 0, 0, time.UTC), Content: "Started work", Position: 0},
		},
	})

	imp := &Importer{Source: source, Dest: dest}
	report, err := imp.Run()
	require.NoError(t, err)
	require.Equal(t, 5, totalCreated(report))
	require.Equal(t, 0, totalErrored(report))

	var paths []string
	for _, c := range report.Tasks.Created {
		paths = append(paths, c.Path)
	}
	for _, c := range report.Notes.Created {
		paths = append(paths, c.Path)
	}
	for _, c := range report.ChecklistTemplates.Created {
		paths = append(paths, c.Path)
	}
	for _, c := range report.ChecklistRuns.Created {
		paths = append(paths, c.Path)
	}
	for _, c := range report.JournalDays.Created {
		paths = append(paths, c.Path)
	}
	require.Len(t, paths, 5)

	for _, p := range paths {
		raw, err := os.ReadFile(filepath.Join(dest, p))
		require.NoErrorf(t, err, "read %s", p)
		n, err := node.Parse(raw)
		require.NoErrorf(t, err, "parse %s", p)
		require.Equalf(t, raw, n.Serialize(), "%s is not byte-identical after reparse", p)
	}
}

// Given a migrated task's typed fields, when compared to the source
// record, then every field the target todo schema supports matches
// (state, scheduled, deadline, aliases); fields with no target on the new
// schema (legacy task notes) appear in the run's reported summary rather
// than being silently dropped.
func TestImporter_TypedFieldsMatchSource_DroppedFieldsInSummary(t *testing.T) {
	source := newFixtureSource(t)
	dest, _ := newFixtureDest(t)

	sched := "2026-02-01"
	tk := fixtureTask("Renew passport", "Visit the DMV", time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC))
	tk.ScheduledDate = &sched
	tk.Tags = []string{"errands", "urgent"}
	tk.Notes = []journal.TaskNote{{ID: "note-1", Text: "Bring photo ID", Position: 0}}
	writeFixtureTaskFile(t, source, "task1.md", tk)

	imp := &Importer{Source: source, Dest: dest}
	report, err := imp.Run()
	require.NoError(t, err)
	require.Len(t, report.Tasks.Created, 1)

	n := mustParseVaultFile(t, dest, report.Tasks.Created[0].Path)
	require.Equal(t, "open", n.Props["state"])
	require.Equal(t, sched, n.Props["scheduled"])
	require.Contains(t, n.Aliases, tk.ID)
	require.Contains(t, n.Props["tags"], "errands")
	require.Contains(t, n.Props["tags"], "urgent")

	// The legacy task note has no field on the new todo schema; it must
	// still be visible (folded into the body) and counted, not silently
	// dropped.
	require.Equal(t, 1, report.FoldedTaskNotes)
	require.Contains(t, n.Body, "Bring photo ID")
}
