package textmigrate

import (
	"fmt"
	"testing"
	"time"

	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/stretchr/testify/require"
)

// Given a fixture legacy store with a mix of open/done tasks, some with a
// scheduled date and/or a deadline, when the importer runs then one
// todos/<ULID>.md file exists per task, each with a state matching the
// source status and scheduled/deadline present iff the source had them.
// Gen-1 tasks carry no dependency field, so no migrated todo carries a
// depends-on edge.
func TestImporter_Tasks_StateScheduledDeadline(t *testing.T) {
	source := newFixtureSource(t)
	dest, _ := newFixtureDest(t)

	openPlain := fixtureTask("Buy milk", "", time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC))

	sched := "2026-02-01"
	openScheduled := fixtureTask("Renew passport", "", time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC))
	openScheduled.ScheduledDate = &sched

	deadline := "2026-03-01"
	doneWithDeadline := fixtureTask("File taxes", "", time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC))
	doneWithDeadline.Status = journal.TaskDone
	doneWithDeadline.DeadlineDate = &deadline

	for i, tk := range []*journal.Task{openPlain, openScheduled, doneWithDeadline} {
		writeFixtureTaskFile(t, source, fmt.Sprintf("task%d.md", i+1), tk)
	}

	imp := &Importer{Source: source, Dest: dest}
	report, err := imp.Run()
	require.NoError(t, err)
	require.Len(t, report.Tasks.Created, 3)
	require.Empty(t, report.Tasks.Errored)

	byID := map[string]RecordOutcome{}
	for _, c := range report.Tasks.Created {
		byID[c.SourceID] = c
	}

	n1 := mustParseVaultFile(t, dest, byID[openPlain.ID].Path)
	require.Equal(t, "todo", n1.Type)
	require.Equal(t, "open", n1.Props["state"])
	require.Empty(t, n1.Props["scheduled"])
	require.Empty(t, n1.Props["deadline"])
	require.Contains(t, n1.Aliases, openPlain.ID)
	for _, l := range n1.Links {
		require.NotEqual(t, "depends-on", l.Rel, "gen-1 tasks have no dependency field")
	}

	n2 := mustParseVaultFile(t, dest, byID[openScheduled.ID].Path)
	require.Equal(t, "open", n2.Props["state"])
	require.Equal(t, sched, n2.Props["scheduled"])
	require.Empty(t, n2.Props["deadline"])

	n3 := mustParseVaultFile(t, dest, byID[doneWithDeadline.ID].Path)
	require.Equal(t, "done", n3.Props["state"])
	require.Equal(t, deadline, n3.Props["deadline"])
	require.Empty(t, n3.Props["scheduled"])
}

// Given a migrated todo whose source task had a legacy xid, when the index
// resolves a reference to that xid, it resolves to the migrated todo's ULID
// via the aliases table -- not through any importer-side text rewrite.
func TestImporter_TaskXID_ResolvesViaIndexAlias(t *testing.T) {
	source := newFixtureSource(t)
	dest, _ := newFixtureDest(t)

	tk := fixtureTask("Renew passport", "", time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC))
	writeFixtureTaskFile(t, source, "task1.md", tk)

	imp := &Importer{Source: source, Dest: dest}
	report, err := imp.Run()
	require.NoError(t, err)
	require.Len(t, report.Tasks.Created, 1)

	vr, err := imp.Verify()
	require.NoError(t, err)
	require.NotContains(t, vr.UnresolvedAliases, tk.ID)
	require.Equal(t, 1, vr.Counts["todo"])
}

// Given a task with attached legacy notes, when migrated the notes are
// folded into the todo's body (the new todo schema has no separate notes
// field) and the fold is visible in the run's reported summary, not a
// silent drop.
func TestImporter_Tasks_NotesFoldedIntoBodyAndSummary(t *testing.T) {
	source := newFixtureSource(t)
	dest, _ := newFixtureDest(t)

	tk := fixtureTask("Plan offsite", "Coordinate the team offsite.", time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC))
	tk.Notes = []journal.TaskNote{
		{ID: "note-1", Text: "Booked the venue", Position: 0},
		{ID: "note-2", Text: "Sent invites", Position: 1},
	}
	writeFixtureTaskFile(t, source, "task1.md", tk)

	imp := &Importer{Source: source, Dest: dest}
	report, err := imp.Run()
	require.NoError(t, err)
	require.Len(t, report.Tasks.Created, 1)
	require.Equal(t, 2, report.FoldedTaskNotes)

	n := mustParseVaultFile(t, dest, report.Tasks.Created[0].Path)
	require.Contains(t, n.Body, "Booked the venue")
	require.Contains(t, n.Body, "Sent invites")
}

// Given a task record whose scheduled date is not a valid calendar date
// (hand-edited or corrupt legacy data), the importer reports a per-record
// error and continues migrating the remaining tasks rather than aborting
// the run.
func TestConvertTask_MalformedScheduledDate_ReturnsError(t *testing.T) {
	bad := "not-a-date"
	tk := journal.Task{
		ID:            "legacy-bad-task",
		Text:          "Broken task",
		Status:        journal.TaskOpen,
		CreatedAt:     time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
		ScheduledDate: &bad,
	}

	_, _, err := convertTask(tk)
	require.Error(t, err)

	// Contrast case: a well-formed task with a valid scheduled date must
	// NOT error, so this test actually discriminates malformed input from
	// the stub's blanket "not implemented" failure.
	valid := "2026-02-01"
	tk.ScheduledDate = &valid
	n, _, err := convertTask(tk)
	require.NoError(t, err)
	require.Equal(t, valid, n.Props["scheduled"])
}

// Given a fixture legacy store with one malformed task alongside a
// well-formed one, the importer reports the malformed record as errored
// (naming its source id) and still migrates the well-formed one.
func TestImporter_Tasks_MalformedFieldReportedContinues(t *testing.T) {
	source := newFixtureSource(t)
	dest, _ := newFixtureDest(t)

	good := fixtureTask("Buy milk", "", time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC))
	writeFixtureTaskFile(t, source, "task-good.md", good)

	bad := fixtureTask("Broken task", "", time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC))
	badDate := "not-a-date"
	bad.ScheduledDate = &badDate
	writeFixtureTaskFile(t, source, "task-bad.md", bad)

	imp := &Importer{Source: source, Dest: dest}
	report, err := imp.Run()
	require.NoError(t, err)

	require.Len(t, report.Tasks.Created, 1)
	require.Equal(t, good.ID, report.Tasks.Created[0].SourceID)
	require.Len(t, report.Tasks.Errored, 1)
	require.Equal(t, bad.ID, report.Tasks.Errored[0].SourceID)
	require.NotEmpty(t, report.Tasks.Errored[0].Error)
}
