package textmigrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/MikeBiancalana/reckon/internal/node"
	"github.com/stretchr/testify/require"
)

// Given legacy journal day files, when the importer runs then one
// log/<date>.md file exists per day as a valid log-day node, with every
// legacy "## Log" entry reproduced as a log-entry sub-node carrying the
// same content.
func TestImporter_JournalDays_LogEntriesReproduced(t *testing.T) {
	source := newFixtureSource(t)
	dest, _ := newFixtureDest(t)

	writeFixtureJournalDay(t, source, &journal.Journal{
		Date: "2026-01-07",
		LogEntries: []journal.LogEntry{
			{Timestamp: time.Date(2026, 1, 7, 9, 0, 0, 0, time.UTC), Content: "Started work", Position: 0},
			{Timestamp: time.Date(2026, 1, 7, 10, 30, 0, 0, time.UTC), Content: "Reviewed PRs", Position: 1},
		},
	})
	writeFixtureJournalDay(t, source, &journal.Journal{
		Date: "2026-01-08",
		LogEntries: []journal.LogEntry{
			{Timestamp: time.Date(2026, 1, 8, 11, 0, 0, 0, time.UTC), Content: "Shipped the release", Position: 0},
		},
	})

	imp := &Importer{Source: source, Dest: dest}
	report, err := imp.Run()
	require.NoError(t, err)
	require.Len(t, report.JournalDays.Created, 2)

	byID := map[string]RecordOutcome{}
	for _, c := range report.JournalDays.Created {
		byID[c.SourceID] = c
	}

	raw := readVaultFile(t, dest, byID["2026-01-07"].Path)
	nodes, err := node.LogParser{}.Parse(raw, node.Loc{File: byID["2026-01-07"].Path})
	require.NoError(t, err)
	require.Equal(t, "log-day", nodes[0].Type)
	require.Len(t, nodes, 3, "one log-day node plus one log-entry node per legacy entry")
	var bodies []string
	for _, n := range nodes[1:] {
		bodies = append(bodies, n.Body)
	}
	require.Contains(t, strings.Join(bodies, "\n"), "Started work")
	require.Contains(t, strings.Join(bodies, "\n"), "Reviewed PRs")

	raw2 := readVaultFile(t, dest, byID["2026-01-08"].Path)
	nodes2, err := node.LogParser{}.Parse(raw2, node.Loc{File: byID["2026-01-08"].Path})
	require.NoError(t, err)
	require.Len(t, nodes2, 2)
	require.Contains(t, nodes2[1].Body, "Shipped the release")
}

// Given an empty legacy journal day file (frontmatter only, no Log
// entries), when migrated it produces a valid zero-entry log-day node, not
// an error that aborts the run.
func TestImporter_JournalEmptyDay_ZeroEntryLogDay(t *testing.T) {
	source := newFixtureSource(t)
	dest, _ := newFixtureDest(t)

	writeFixtureJournalDay(t, source, &journal.Journal{Date: "2026-01-09"})

	imp := &Importer{Source: source, Dest: dest}
	report, err := imp.Run()
	require.NoError(t, err)
	require.Len(t, report.JournalDays.Created, 1)
	require.Empty(t, report.JournalDays.Errored)

	n := mustParseVaultFile(t, dest, report.JournalDays.Created[0].Path)
	require.Equal(t, "log-day", n.Type)
	require.Empty(t, n.SplitEntries())
}

// Given a legacy journal day file with malformed content the importer
// cannot parse (a CRLF file, matching the same guard the rest of the
// vault-writing tools apply), when migrated that day is reported as an
// error naming the day's date, and the run continues with the remaining
// days.
func TestImporter_JournalMalformedDayReportedContinues(t *testing.T) {
	source := newFixtureSource(t)
	dest, _ := newFixtureDest(t)

	writeFixtureJournalDay(t, source, &journal.Journal{
		Date: "2026-01-10",
		LogEntries: []journal.LogEntry{
			{Timestamp: time.Date(2026, 1, 10, 9, 0, 0, 0, time.UTC), Content: "Good day", Position: 0},
		},
	})
	writeFixtureJournalDayRaw(t, source, "2026-01-11", "---\r\ndate: 2026-01-11\r\n---\r\n\r\n## Log\r\n\r\n- 09:00 broken\r\n")

	imp := &Importer{Source: source, Dest: dest}
	report, err := imp.Run()
	require.NoError(t, err)

	require.Len(t, report.JournalDays.Created, 1)
	require.Equal(t, "2026-01-10", report.JournalDays.Created[0].SourceID)
	require.Len(t, report.JournalDays.Errored, 1)
	require.Equal(t, "2026-01-11", report.JournalDays.Errored[0].SourceID)
	require.NotEmpty(t, report.JournalDays.Errored[0].Error)
}

// Given a legacy journal day with Intentions/Wins/Schedule entries (which
// have no direct analog in the flat log-entry model), when migrated those
// entries are preserved losslessly as a preamble block ahead of the day's
// log entries, and the counts are visible in the run's reported summary.
func TestImporter_JournalPreambleSectionsPreservedAndCounted(t *testing.T) {
	source := newFixtureSource(t)
	dest, _ := newFixtureDest(t)

	writeFixtureJournalDay(t, source, &journal.Journal{
		Date: "2026-01-12",
		Intentions: []journal.Intention{
			{Text: "Review PRs", Status: journal.IntentionOpen, Position: 0},
			{Text: "Ship the release", Status: journal.IntentionDone, Position: 1},
		},
		Wins: []journal.Win{
			{Text: "Fixed the flaky test", Position: 0},
		},
		ScheduleItems: []journal.ScheduleItem{
			{Time: time.Date(2026, 1, 12, 14, 0, 0, 0, time.UTC), Content: "Client meeting", Position: 0},
		},
		LogEntries: []journal.LogEntry{
			{Timestamp: time.Date(2026, 1, 12, 9, 0, 0, 0, time.UTC), Content: "Started work", Position: 0},
		},
	})

	imp := &Importer{Source: source, Dest: dest}
	report, err := imp.Run()
	require.NoError(t, err)
	require.Len(t, report.JournalDays.Created, 1)

	require.Equal(t, 2, report.JournalPreamble.Intentions)
	require.Equal(t, 1, report.JournalPreamble.Wins)
	require.Equal(t, 1, report.JournalPreamble.Schedule)

	n := mustParseVaultFile(t, dest, report.JournalDays.Created[0].Path)
	require.Contains(t, n.Body, "### Intentions")
	require.Contains(t, n.Body, "Review PRs")
	require.Contains(t, n.Body, "### Wins")
	require.Contains(t, n.Body, "Fixed the flaky test")
	require.Contains(t, n.Body, "### Schedule")
	require.Contains(t, n.Body, "Client meeting")

	// H3 (### ), never H2 (## ), so LogParser's entry-splitter (which only
	// matches "^## ") does not mis-split the preamble into phantom entries.
	require.NotContains(t, n.Body, "\n## Intentions")
	require.Len(t, n.SplitEntries(), 1, "the preamble must not be split into extra log entries")
}

// Given a legacy "## Log" entry whose content contains an inline
// [task:<xid>] marker, when migrated that marker is rewritten to a
// [[<xid>]] wikilink so the index derives a reference edge to the migrated
// todo, and the rewrite is counted in the run's summary.
func TestImporter_JournalTaskRefRewrittenToWikilink(t *testing.T) {
	source := newFixtureSource(t)
	dest, _ := newFixtureDest(t)

	writeFixtureJournalDay(t, source, &journal.Journal{
		Date: "2026-01-13",
		LogEntries: []journal.LogEntry{
			{Timestamp: time.Date(2026, 1, 13, 10, 0, 0, 0, time.UTC), Content: "[task:legacy-abc123] Working on the feature", Position: 0},
		},
	})

	imp := &Importer{Source: source, Dest: dest}
	report, err := imp.Run()
	require.NoError(t, err)
	require.Len(t, report.JournalDays.Created, 1)
	require.Equal(t, 1, report.RewrittenTaskRefs)

	n := mustParseVaultFile(t, dest, report.JournalDays.Created[0].Path)
	require.Contains(t, n.Body, "[[legacy-abc123]]")
	require.NotContains(t, n.Body, "[task:legacy-abc123]")
}

// readVaultFile reads raw bytes of a migrated file at dest/relPath, for
// callers that need node.LogParser's multi-node split rather than the
// single-node node.Parse mustParseVaultFile uses.
func readVaultFile(t *testing.T, dest, relPath string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dest, relPath))
	require.NoError(t, err)
	return raw
}
