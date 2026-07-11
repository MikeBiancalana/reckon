// TDD red tests for the internal/textmigrate importer. internal/textmigrate
// does not exist as a working implementation yet -- migrate.go, tasks.go,
// notes.go, checklists.go, journal.go, verify.go, and writer.go currently
// hold compilation-only stubs (every exported function returns a "not
// implemented" error or zero value). Every test below exercises the real
// Importer type against fixture legacy data and IS EXPECTED TO FAIL until
// those stubs are replaced with working conversions; that failure is this
// package's red state.
//
// Fixture style: a temp "legacy data root" (tasks/, notes/, journal/
// subdirectories plus a SQLite reckon.db for notes and checklists) paired
// with a temp, empty "vault" destination -- mirroring the fixture-DB +
// fixture-file pattern internal/cli/adopt_test.go and
// internal/spike/roundtrip/roundtrip_test.go use for other packages'
// end-to-end tests, but scoped to the legacy -> vault shape this package
// converts between.
package textmigrate

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MikeBiancalana/reckon/internal/checklist"
	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/MikeBiancalana/reckon/internal/models"
	"github.com/MikeBiancalana/reckon/internal/node"
	notessvc "github.com/MikeBiancalana/reckon/internal/service"
	"github.com/MikeBiancalana/reckon/internal/storage"
	"github.com/stretchr/testify/require"
)

// ─────────────────────────────────────────────────────────────────────────────
// Fixture harness (shared across this package's *_test.go files)
// ─────────────────────────────────────────────────────────────────────────────

// newFixtureSource creates a temp legacy data root with empty tasks/,
// notes/, and journal/ subdirectories. The legacy SQLite store is opened
// separately via openFixtureDB (notes and checklists only; tasks and
// journal are always file-primary).
func newFixtureSource(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, sub := range []string{"tasks", "notes", "journal"} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, sub), 0o755))
	}
	return root
}

// newFixtureDest creates a temp, empty vault dir and a sibling cache dir,
// and points RECKON_CACHE at the cache dir (auto-restored by the testing
// framework) so a later Importer.Verify resolves a hermetic, per-test cache
// location instead of a real user cache dir.
func newFixtureDest(t *testing.T) (dest, cache string) {
	t.Helper()
	root := t.TempDir()
	dest = filepath.Join(root, "vault")
	cache = filepath.Join(root, "cache")
	require.NoError(t, os.MkdirAll(dest, 0o755))
	t.Setenv("RECKON_CACHE", cache)
	return dest, cache
}

// openFixtureDB opens (creating and schema-initializing if needed) the
// legacy SQLite DB at source/reckon.db.
func openFixtureDB(t *testing.T, source string) *storage.Database {
	t.Helper()
	db, err := storage.NewDatabase(filepath.Join(source, "reckon.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

// fixtureTask mints a legacy Task (real xid, via journal.NewTask) with
// CreatedAt overridden to createdAt so callers get deterministic authoring
// order across a fixture set.
func fixtureTask(text, description string, createdAt time.Time) *journal.Task {
	tk := journal.NewTask(text, description, nil, 0)
	tk.CreatedAt = createdAt
	return tk
}

// writeFixtureTaskFile hand-renders one legacy gen-1 task file matching the
// exact frontmatter keys internal/journal's real reader
// (TaskFrontmatter/parseTaskFile) expects -- id, title, created, status,
// tags, scheduled_date, deadline_date -- and writes it under
// source/tasks/<filename>. filename may be any *.md basename; GetAllTasks
// scans the directory regardless of name.
func writeFixtureTaskFile(t *testing.T, source, filename string, tk *journal.Task) {
	t.Helper()
	var b []byte
	add := func(s string) { b = append(b, []byte(s)...) }

	add("---\n")
	add("id: " + tk.ID + "\n")
	add("title: " + tk.Text + "\n")
	add("created: " + tk.CreatedAt.Format("2006-01-02") + "\n")
	status := "open"
	if tk.Status == journal.TaskDone {
		status = "done"
	}
	add("status: " + status + "\n")
	if len(tk.Tags) > 0 {
		add("tags:\n")
		for _, tag := range tk.Tags {
			add("  - " + tag + "\n")
		}
	}
	if tk.ScheduledDate != nil {
		add("scheduled_date: " + *tk.ScheduledDate + "\n")
	}
	if tk.DeadlineDate != nil {
		add("deadline_date: " + *tk.DeadlineDate + "\n")
	}
	add("---\n\n## Description\n\n" + tk.Description + "\n\n")
	if len(tk.Notes) > 0 {
		add("## Log\n\n")
		add("### " + tk.CreatedAt.Format("2006-01-02") + "\n")
		for _, note := range tk.Notes {
			add("  - " + note.ID + " " + note.Text + "\n")
		}
	}

	path := filepath.Join(source, "tasks", filename)
	require.NoError(t, os.WriteFile(path, b, 0o644))
}

// writeFixtureNote registers a legacy note DB row (metadata authority) and
// writes its paired legacy body file under source/notes/<relPath>, mirroring
// the real split between notes.slug/title/tags (DB) and note body content
// (file).
func writeFixtureNote(t *testing.T, source string, db *storage.Database, title, slug, relPath string, tags []string, createdAt time.Time, legacyBody string) *models.Note {
	t.Helper()
	n := models.NewNote(title, slug, relPath, tags)
	n.CreatedAt = createdAt
	n.UpdatedAt = createdAt
	repo := notessvc.NewNotesRepository(db)
	require.NoError(t, repo.SaveNote(n))

	content := "---\ntitle: " + title + "\n---\n\n" + legacyBody
	full := filepath.Join(source, "notes", relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	return n
}

// fixtureChecklist creates a legacy checklist template (with items) and one
// active run of it via the real checklist.Service, so both rows and every
// derived id (xid) are exactly what production code would produce.
func fixtureChecklist(t *testing.T, db *storage.Database, name string, items []string) (*checklist.Template, *checklist.Run) {
	t.Helper()
	repo := checklist.NewRepository(db)
	svc := checklist.NewService(repo)
	tpl, err := svc.CreateTemplate(name, items)
	require.NoError(t, err)
	run, err := svc.StartRun(name)
	require.NoError(t, err)
	return tpl, run
}

// writeFixtureJournalDay renders j via the real production journal-day
// writer (internal/journal.WriteJournal, also used by journal.Service's
// SaveJournal) and writes it under source/journal/<date>.md.
func writeFixtureJournalDay(t *testing.T, source string, j *journal.Journal) {
	t.Helper()
	content := journal.WriteJournal(j)
	path := filepath.Join(source, "journal", j.Date+".md")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// writeFixtureJournalDayRaw writes an arbitrary raw string as a legacy day
// file, for fixtures that need to deviate from WriteJournal's shape (e.g. a
// CRLF file).
func writeFixtureJournalDayRaw(t *testing.T, source, date, raw string) {
	t.Helper()
	path := filepath.Join(source, "journal", date+".md")
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o644))
}

// mustParseVaultFile reads and parses a migrated file at dest/relPath.
func mustParseVaultFile(t *testing.T, dest, relPath string) *node.Node {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dest, relPath))
	require.NoError(t, err)
	n, err := node.Parse(raw)
	require.NoError(t, err)
	return n
}

// writeSeedNode writes a fully-formed node file directly to the vault (via
// node.Render, bypassing the importer entirely), simulating output a prior
// migration run -- or any other tool using the standard create recipe --
// already produced. Used to pre-seed "partial prior run" fixtures.
func writeSeedNode(t *testing.T, dest, relPath, typ string, aliases []string, props map[string]string, body string, mintedAt time.Time) *node.Node {
	t.Helper()
	n := node.NewNode(typ, "legacy-import", body)
	n.ULID = node.MintAt(mintedAt)
	n.Time = mintedAt.UTC().Format(time.RFC3339)
	n.Aliases = aliases
	n.Props = props
	rendered := n.Render()
	parsed, err := node.Parse(rendered)
	require.NoError(t, err)
	full := filepath.Join(dest, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, parsed.Serialize(), 0o644))
	return parsed
}

func totalCreated(r *Report) int {
	return len(r.Tasks.Created) + len(r.Notes.Created) + len(r.ChecklistTemplates.Created) +
		len(r.ChecklistRuns.Created) + len(r.JournalDays.Created)
}

func totalSkipped(r *Report) int {
	return len(r.Tasks.Skipped) + len(r.Notes.Skipped) + len(r.ChecklistTemplates.Skipped) +
		len(r.ChecklistRuns.Skipped) + len(r.JournalDays.Skipped)
}

func totalErrored(r *Report) int {
	return len(r.Tasks.Errored) + len(r.Notes.Errored) + len(r.ChecklistTemplates.Errored) +
		len(r.ChecklistRuns.Errored) + len(r.JournalDays.Errored)
}

// ─────────────────────────────────────────────────────────────────────────────
// Orchestration: dry-run, idempotency, empty store
// ─────────────────────────────────────────────────────────────────────────────

// Given a fixture legacy store with one record of every type, a --dry-run
// import reports the same create plan a real run would but writes nothing:
// no todos/notes/checklists/log directory is created under the vault.
func TestImporter_DryRun_WritesNoFiles(t *testing.T) {
	source := newFixtureSource(t)
	dest, _ := newFixtureDest(t)

	tk := fixtureTask("Buy milk", "", time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC))
	writeFixtureTaskFile(t, source, "task1.md", tk)

	db := openFixtureDB(t, source)
	writeFixtureNote(t, source, db, "Grocery Plan", "grocery-plan", "grocery-plan.md", nil,
		time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC), "Plan for the week.\n")

	fixtureChecklist(t, db, "morning", []string{"Check email"})

	writeFixtureJournalDay(t, source, &journal.Journal{
		Date: "2026-01-07",
		LogEntries: []journal.LogEntry{
			{Timestamp: time.Date(2026, 1, 7, 9, 0, 0, 0, time.UTC), Content: "Started work", Position: 0},
		},
	})

	imp := &Importer{Source: source, Dest: dest, DryRun: true}
	report, err := imp.Run()
	require.NoError(t, err)

	require.Len(t, report.Tasks.Created, 1)
	require.Len(t, report.Notes.Created, 1)
	require.Len(t, report.ChecklistTemplates.Created, 1)
	require.Len(t, report.ChecklistRuns.Created, 1)
	require.Len(t, report.JournalDays.Created, 1)
	require.Equal(t, 0, totalErrored(report))

	for _, dir := range []string{"todos", "notes", "checklists", "log"} {
		_, err := os.Stat(filepath.Join(dest, dir))
		require.Truef(t, os.IsNotExist(err), "dry-run created %s", dir)
	}
}

// Given a --dry-run invocation, the destination's index cache directory is
// never created or touched.
func TestImporter_DryRun_DoesNotTouchCacheOrIndex(t *testing.T) {
	source := newFixtureSource(t)
	dest, cache := newFixtureDest(t)

	tk := fixtureTask("Buy milk", "", time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC))
	writeFixtureTaskFile(t, source, "task1.md", tk)

	imp := &Importer{Source: source, Dest: dest, DryRun: true}
	_, err := imp.Run()
	require.NoError(t, err)

	_, statErr := os.Stat(cache)
	require.Truef(t, os.IsNotExist(statErr), "dry-run created the cache dir %q", cache)
}

// Given a migration that already fully completed, running the importer a
// second time over the same source and destination is a no-op: every record
// is reported skipped, nothing new is created, and no already-written
// file's bytes change.
func TestImporter_FullReRun_NoOp(t *testing.T) {
	source := newFixtureSource(t)
	dest, _ := newFixtureDest(t)

	tk1 := fixtureTask("Buy milk", "", time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC))
	tk2 := fixtureTask("Ship the release", "", time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC))
	writeFixtureTaskFile(t, source, "task1.md", tk1)
	writeFixtureTaskFile(t, source, "task2.md", tk2)

	db := openFixtureDB(t, source)
	writeFixtureNote(t, source, db, "Grocery Plan", "grocery-plan", "grocery-plan.md", nil,
		time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC), "Plan for the week.\n")

	imp := &Importer{Source: source, Dest: dest}
	first, err := imp.Run()
	require.NoError(t, err)
	require.Equal(t, 3, totalCreated(first))
	require.Equal(t, 0, totalErrored(first))

	beforeTask := mustParseVaultFile(t, dest, first.Tasks.Created[0].Path)

	second := &Importer{Source: source, Dest: dest}
	report, err := second.Run()
	require.NoError(t, err)

	require.Equal(t, 0, totalCreated(report), "second run should create nothing")
	require.Equal(t, 3, totalSkipped(report), "second run should skip every already-migrated record")
	require.Equal(t, 0, totalErrored(report))

	afterTask := mustParseVaultFile(t, dest, first.Tasks.Created[0].Path)
	require.Equal(t, beforeTask.ULID, afterTask.ULID, "re-run must not re-mint an already-migrated node's ULID")
	require.Equal(t, beforeTask.Aliases, afterTask.Aliases)
	require.Equal(t, string(beforeTask.Serialize()), string(afterTask.Serialize()), "re-run must not touch an already-migrated file's bytes")
}

// Given a migration interrupted after writing some but not all records
// (simulated by pre-seeding the vault with one already-migrated task before
// the source has two), re-running the importer completes the remaining
// record without duplicating the pre-seeded one, and a subsequent verify
// pass reports the full source counts.
func TestImporter_PartialReRun_CompletesRemainder(t *testing.T) {
	source := newFixtureSource(t)
	dest, _ := newFixtureDest(t)

	tk1 := fixtureTask("Buy milk", "", time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC))
	tk2 := fixtureTask("Ship the release", "", time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC))
	writeFixtureTaskFile(t, source, "task1.md", tk1)
	writeFixtureTaskFile(t, source, "task2.md", tk2)

	// Simulate an interrupted prior run: tk1 already migrated, tk2 was never
	// reached.
	writeSeedNode(t, dest, "todos/01J9Z3K7Q2W8XR4M6N0V5BYHED.md", "todo",
		[]string{tk1.ID}, map[string]string{"state": "open"}, tk1.Text+"\n", tk1.CreatedAt)

	imp := &Importer{Source: source, Dest: dest}
	report, err := imp.Run()
	require.NoError(t, err)

	require.Len(t, report.Tasks.Skipped, 1)
	require.Equal(t, tk1.ID, report.Tasks.Skipped[0].SourceID)
	require.Len(t, report.Tasks.Created, 1)
	require.Equal(t, tk2.ID, report.Tasks.Created[0].SourceID)

	vr, err := imp.Verify()
	require.NoError(t, err)
	require.Equal(t, 2, vr.Counts["todo"])
	require.Empty(t, vr.UnexpectedWarnings)
	require.True(t, vr.OK)
}

// Given a fixture legacy store with zero records of every type (empty
// tasks/notes/journal directories and an empty, schema-initialized
// database), the importer produces zero created/skipped/errored records
// across the board rather than erroring.
func TestImporter_EmptyLegacyStore_ZeroCounts(t *testing.T) {
	source := newFixtureSource(t)
	dest, _ := newFixtureDest(t)
	openFixtureDB(t, source) // schema-initialized, zero rows

	imp := &Importer{Source: source, Dest: dest}
	report, err := imp.Run()
	require.NoError(t, err)

	require.Equal(t, 0, totalCreated(report))
	require.Equal(t, 0, totalSkipped(report))
	require.Equal(t, 0, totalErrored(report))

	vr, err := imp.Verify()
	require.NoError(t, err)
	require.True(t, vr.OK)
	for _, typ := range []string{"todo", "note", "checklist-template", "checklist-run", "log-day"} {
		require.Equalf(t, 0, vr.Counts[typ], "type %q", typ)
	}
}
