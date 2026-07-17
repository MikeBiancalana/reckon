//go:build acceptance

// Package acceptance exercises the real rk binary end-to-end against
// throwaway vaults, covering the daily-driver workflows the composable v1 is
// meant to serve: capture, todo lifecycle (incl. recurrence and the
// complete→log loop), notes with linking, index generation, and the query
// surface. These are system tests: every assertion is made against either
// the bytes rk leaves on disk or the JSON it emits — never against internal
// packages — so they hold no matter how the internals are refactored.
//
// Run:
//
//	go test -tags acceptance ./tests/acceptance/
//
// TestMain builds rk once from the enclosing module; each test gets a fresh
// vault and cache dir, so tests are independent and parallel-safe.
package acceptance

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var rkBin string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "rk-acceptance-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "acceptance: mktemp:", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmp)

	rkBin = filepath.Join(tmp, "rk")
	build := exec.Command("go", "build", "-o", rkBin, "../../cmd/rk")
	build.Stdout = os.Stderr
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "acceptance: build rk:", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// vault is one throwaway RECKON_VAULT + RECKON_CACHE pair.
type vault struct {
	dir   string
	cache string
}

func newVault(t *testing.T) *vault {
	t.Helper()
	return &vault{dir: t.TempDir(), cache: t.TempDir()}
}

// run executes rk with the vault's environment and fails the test on a
// non-zero exit. It returns stdout only; rk's operational log lines go to
// stderr and are surfaced only on failure.
func (v *vault) run(t *testing.T, args ...string) string {
	t.Helper()
	stdout, stderr, err := v.exec(args...)
	if err != nil {
		t.Fatalf("rk %s: %v\nstderr:\n%s", strings.Join(args, " "), err, stderr)
	}
	return stdout
}

// runErr executes rk and returns the outcome without failing the test; use
// it to assert rejection paths.
func (v *vault) runErr(args ...string) (stdout, stderr string, err error) {
	return v.exec(args...)
}

func (v *vault) exec(args ...string) (stdout, stderr string, err error) {
	cmd := exec.Command(rkBin, args...)
	cmd.Env = append(os.Environ(),
		"RECKON_VAULT="+v.dir,
		"RECKON_CACHE="+v.cache,
		"RECKON_AUTHOR=acceptance",
	)
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err = cmd.Run()
	return out.String(), errb.String(), err
}

// execEnv is like exec but layers extraEnv on top of the vault's base
// environment, for tests that need LOG_LEVEL/RECKON_DEBUG or similar.
func (v *vault) execEnv(extraEnv []string, args ...string) (stdout, stderr string, err error) {
	cmd := exec.Command(rkBin, args...)
	cmd.Env = append(os.Environ(),
		"RECKON_VAULT="+v.dir,
		"RECKON_CACHE="+v.cache,
		"RECKON_AUTHOR=acceptance",
	)
	cmd.Env = append(cmd.Env, extraEnv...)
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err = cmd.Run()
	return out.String(), errb.String(), err
}

// runJSON executes rk with --json appended and unmarshals stdout.
func (v *vault) runJSON(t *testing.T, args ...string) map[string]any {
	t.Helper()
	out := v.run(t, append(args, "--json")...)
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("rk %s: unmarshal %q: %v", strings.Join(args, " "), out, err)
	}
	return m
}

func (v *vault) readFile(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(v.dir, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

func today() string { return time.Now().UTC().Format("2006-01-02") }

func str(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

// ─────────────────────────────────────────────────────────────────────────────
// Capture — the front door (v1-T4)
// ─────────────────────────────────────────────────────────────────────────────

// A day's captures land in one log/<date>.md group file: timestamped,
// authored, backfillable with --at, each entry carrying its own inline ULID.
func TestCapture_DayFileAuthoredAndBackfilled(t *testing.T) {
	t.Parallel()
	v := newVault(t)

	v.run(t, "add", "Morning standup", "--at", "08:30", "-q")
	v.run(t, "add", "Deep work block", "-q")

	day := v.readFile(t, "log/"+today()+".md")
	for _, want := range []string{
		"type: log-day",
		"## 08:30 · acceptance", // backfilled time + provenance
		"Morning standup",
		"Deep work block",
		"id:: ", // every entry has an inline ULID
	} {
		if !strings.Contains(day, want) {
			t.Errorf("day file missing %q:\n%s", want, day)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Todos — real lifecycle (v1 todo + T6 recurrence)
// ─────────────────────────────────────────────────────────────────────────────

// A durable todo closes IN PLACE: done flips state in the todo's own file.
// No carry, no journal-entry-as-closure.
func TestTodo_OpenToDoneClosesInPlace(t *testing.T) {
	t.Parallel()
	v := newVault(t)

	created := v.runJSON(t, "todo", "add", "Ship the acceptance suite")
	id := str(created, "id")
	if id == "" {
		t.Fatalf("todo add returned no id: %v", created)
	}

	done := v.runJSON(t, "todo", "done", id)
	if got := str(done, "state"); got != "done" {
		t.Errorf("todo done state = %q, want done", got)
	}
	file := v.readFile(t, "todos/"+id+".md")
	if !strings.Contains(file, "state: done") {
		t.Errorf("todo file not closed in place:\n%s", file)
	}

	// Default list hides done items; --all shows them.
	if lst := v.run(t, "todo", "list", "--json"); strings.Contains(lst, id) {
		t.Errorf("default todo list still shows done todo %s", id)
	}
	if lst := v.run(t, "todo", "list", "--all", "--json"); !strings.Contains(lst, id) {
		t.Errorf("todo list --all does not show done todo %s", id)
	}
}

// Completing a recurring todo advances the stored scheduled cursor
// (org-style), leaves the rule node open, and emits a `did` audit entry into
// the day log — the complete→log loop.
func TestTodo_RecurrenceAdvancesCursorAndLogsDid(t *testing.T) {
	t.Parallel()
	v := newVault(t)

	sched := today()
	created := v.runJSON(t, "todo", "add", "Water plants",
		"--scheduled", sched, "--repeat", "+1w")
	id := str(created, "id")

	done := v.runJSON(t, "todo", "done", id)
	if rec, _ := done["recurred"].(bool); !rec {
		t.Fatalf("expected recurred=true, got %v", done)
	}
	next := str(done, "scheduled")
	if next == sched || next == "" {
		t.Errorf("scheduled cursor did not advance: %q -> %q", sched, next)
	}

	file := v.readFile(t, "todos/"+id+".md")
	if !strings.Contains(file, "state: open") {
		t.Errorf("recurring todo should stay open after done:\n%s", file)
	}
	if !strings.Contains(file, "scheduled: "+next) {
		t.Errorf("cursor %q not persisted in file:\n%s", next, file)
	}

	// The audit entry references the todo by ULID via a did:: edge line.
	if p := str(done, "did_entry_path"); p == "" {
		t.Errorf("no did audit entry emitted: %v", done)
	} else if !strings.Contains(v.readFile(t, p), "did:: "+id) {
		t.Errorf("did entry in %s carries no did:: %s edge", p, id)
	}
}

// Ephemeral todos live in a group file with no stable address and close by
// index — the ephemeral/durable split.
func TestTodo_EphemeralInboxLifecycle(t *testing.T) {
	t.Parallel()
	v := newVault(t)

	v.run(t, "todo", "add", "Throwaway reminder", "--ephemeral", "-q")
	lst := v.run(t, "todo", "list", "--ephemeral", "--json")
	if !strings.Contains(lst, "Throwaway reminder") {
		t.Fatalf("ephemeral item not listed: %s", lst)
	}
	v.run(t, "todo", "done", "1", "--ephemeral", "-q")
	lst = v.run(t, "todo", "list", "--ephemeral", "--json")
	if strings.Contains(lst, `"Throwaway reminder"`) && !strings.Contains(lst, `"checked":true`) {
		t.Errorf("ephemeral item not checked after done: %s", lst)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Notes — the knowledge pillar (v1-T8)
// ─────────────────────────────────────────────────────────────────────────────

// A created note honors the settled T8 conventions: slug filename, the slug
// self-minted into aliases:, title/description/stage props, and NO
// timestamp/updated field (git is the versioning backplane).
func TestNote_CreateConventions(t *testing.T) {
	t.Parallel()
	v := newVault(t)

	v.run(t, "note", "create", "PAS Entity Model",
		"--description", "How PAS entities map across vendors.",
		"--stage", "seedling", "-q")

	file := v.readFile(t, "notes/pas-entity-model.md")
	for _, want := range []string{
		"type: note",
		"aliases: [pas-entity-model]",
		"title: PAS Entity Model",
		"description: How PAS entities map across vendors.",
		"stage: seedling",
	} {
		if !strings.Contains(file, want) {
			t.Errorf("note missing %q:\n%s", want, file)
		}
	}
	for _, banned := range []string{"\ntimestamp:", "\nupdated:"} {
		if strings.Contains(file, banned) {
			t.Errorf("note carries forbidden field %q:\n%s", banned, file)
		}
	}

	// Stage is a closed CLI-layer enum.
	if _, stderr, err := v.runErr("note", "create", "Bad Stage", "--stage", "ripe"); err == nil {
		t.Errorf("invalid --stage accepted; stderr: %s", stderr)
	}
}

// A [[wikilink]] to a not-yet-written note is stored as an unresolved edge,
// resolves the moment the target is created (with no edit to the source
// file), and yields an index-derived backlink on the target.
func TestNote_DanglingLinkResolvesAndBacklinks(t *testing.T) {
	t.Parallel()
	v := newVault(t)

	v.run(t, "note", "create", "PAS Entity Model",
		"--body", "Relates to [[lease-lifecycle]], not yet written.", "-q")

	show := v.runJSON(t, "note", "show", "pas-entity-model")
	if n := resolvedLinks(show); n != 0 {
		t.Fatalf("expected 0 resolved links before target exists, got %d", n)
	}

	srcBefore := v.readFile(t, "notes/pas-entity-model.md")
	v.run(t, "note", "create", "Lease Lifecycle", "-q")

	show = v.runJSON(t, "note", "show", "pas-entity-model")
	if n := resolvedLinks(show); n != 1 {
		t.Fatalf("expected 1 resolved link after target created, got %d: %v", n, show)
	}
	if srcAfter := v.readFile(t, "notes/pas-entity-model.md"); srcAfter != srcBefore {
		t.Errorf("source file was edited during link resolution (must be index-only)")
	}

	target := v.runJSON(t, "note", "show", "lease-lifecycle")
	if bl, _ := target["backlinks"].([]any); len(bl) != 1 {
		t.Errorf("expected 1 backlink on target, got %v", target["backlinks"])
	}
}

func resolvedLinks(show map[string]any) int {
	links, _ := show["forward_links"].([]any)
	n := 0
	for _, l := range links {
		if m, ok := l.(map[string]any); ok && str(m, "dst_key") != "" {
			n++
		}
	}
	return n
}

// Rename moves the file to the new slug and retains the old slug as an alias
// redirect: links written against the old name keep resolving.
func TestNote_RenameRetainsRedirect(t *testing.T) {
	t.Parallel()
	v := newVault(t)

	v.run(t, "note", "create", "PAS Entity Model", "-q")
	v.run(t, "note", "rename", "pas-entity-model", "PAS Entity Map", "-q")

	file := v.readFile(t, "notes/pas-entity-map.md")
	if !strings.Contains(file, "pas-entity-model") {
		t.Errorf("old slug not retained as alias:\n%s", file)
	}
	if _, err := os.Stat(filepath.Join(v.dir, "notes/pas-entity-model.md")); !os.IsNotExist(err) {
		t.Errorf("old file still present after rename")
	}
	show := v.runJSON(t, "note", "show", "pas-entity-model") // old name must still resolve
	if got := str(show, "title"); got != "PAS Entity Map" {
		t.Errorf("old slug resolves to title %q, want renamed note", got)
	}
}

// rk note index generates marker-bearing, deterministic catalogs and never
// touches a hand-authored index.md.
func TestNote_IndexDeterministicAndOwnershipSafe(t *testing.T) {
	t.Parallel()
	v := newVault(t)

	v.run(t, "note", "create", "Beta Note", "--description", "Second.", "-q")
	v.run(t, "note", "create", "Alpha Note", "--description", "First.", "-q")

	v.run(t, "note", "index", "-q")
	first := v.readFile(t, "notes/index.md")
	if !strings.HasPrefix(first, "<!-- generated by rk note index") {
		t.Fatalf("generated index missing ownership marker:\n%s", first)
	}
	v.run(t, "note", "index", "-q")
	if second := v.readFile(t, "notes/index.md"); second != first {
		t.Errorf("index generation not deterministic across runs")
	}
	if !strings.Contains(first, "[Alpha Note](alpha-note.md) — First.") {
		t.Errorf("index entry malformed:\n%s", first)
	}

	// Hand-authored index.md (no marker) in a subdir is skipped, not clobbered.
	sub := filepath.Join(v.dir, "notes", "area")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	hand := "# My own catalog\n"
	if err := os.WriteFile(filepath.Join(sub, "index.md"), []byte(hand), 0o644); err != nil {
		t.Fatal(err)
	}
	v.run(t, "note", "create", "Area Note", "--dir", "area", "-q")
	v.run(t, "note", "index", "-q")
	if got := v.readFile(t, "notes/area/index.md"); got != hand {
		t.Errorf("hand-authored index.md was clobbered:\n%s", got)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Query — the agent read surface (v1-T3)
// ─────────────────────────────────────────────────────────────────────────────

// After a day's mixed work, rk query sees every type in one graph; writes
// through the query surface are rejected.
func TestQuery_CrossTypeGraphReadOnly(t *testing.T) {
	t.Parallel()
	v := newVault(t)

	v.run(t, "add", "Captured a thought", "-q")
	v.run(t, "todo", "add", "A durable task", "-q")
	v.run(t, "note", "create", "A Note", "-q")
	v.run(t, "index", "-q")

	out := v.run(t, "query",
		"SELECT type, COUNT(*) AS c FROM nodes GROUP BY type ORDER BY type", "--raw")
	for _, typ := range []string{"log-day", "log-entry", "note", "todo"} {
		if !strings.Contains(out, typ) {
			t.Errorf("query result missing type %q:\n%s", typ, out)
		}
	}

	if _, stderr, err := v.runErr("query", "DELETE FROM nodes"); err == nil {
		t.Errorf("non-SELECT accepted by rk query; stderr: %s", stderr)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Today — the agenda surface (v1-T7)
// ─────────────────────────────────────────────────────────────────────────────

// The agenda surfaces exactly the actionable set — overdue and scheduled-today
// — and excludes future, unscheduled, and already-done todos.
func TestToday_AgendaSurfacesActionableOnly(t *testing.T) {
	t.Parallel()
	v := newVault(t)

	overdue := str(v.runJSON(t, "todo", "add", "Overdue item", "--scheduled", "2026-01-02"), "id")
	todayItem := str(v.runJSON(t, "todo", "add", "Today item", "--scheduled", today()), "id")
	future := str(v.runJSON(t, "todo", "add", "Future item", "--scheduled", "2030-01-01"), "id")
	unscheduled := str(v.runJSON(t, "todo", "add", "Unscheduled item"), "id")
	doneToday := str(v.runJSON(t, "todo", "add", "Done item", "--scheduled", today()), "id")
	v.run(t, "todo", "done", doneToday, "-q")

	out := v.run(t, "today", "--json")
	for id, want := range map[string]bool{
		overdue: true, todayItem: true,
		future: false, unscheduled: false, doneToday: false,
	} {
		if got := strings.Contains(out, id); got != want {
			t.Errorf("today agenda contains %s = %v, want %v:\n%s", id, got, want, out)
		}
	}
}

// The agenda is an actuator for native todos: `rk today act <ref> x` completes
// the todo in place and (by default) writes the did:: audit entry — the same
// complete→log loop as `rk todo done`, driven from the agenda surface.
func TestToday_ActCompletesInPlace(t *testing.T) {
	t.Parallel()
	v := newVault(t)

	id := str(v.runJSON(t, "todo", "add", "Actuate me", "--scheduled", today()), "id")
	v.run(t, "today", "act", id, "x", "-q")

	file := v.readFile(t, "todos/"+id+".md")
	if !strings.Contains(file, "state: done") {
		t.Errorf("today act x did not close the todo in place:\n%s", file)
	}
	if day := v.readFile(t, "log/"+today()+".md"); !strings.Contains(day, "did:: "+id) {
		t.Errorf("today act x wrote no did:: audit entry for %s:\n%s", id, day)
	}
	if out := v.run(t, "today", "--json"); strings.Contains(out, id) {
		t.Errorf("completed todo still on the agenda:\n%s", out)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// The daily driver — one vault, one day, every pillar
// ─────────────────────────────────────────────────────────────────────────────

// The composed workflow the system exists for: capture through the day, work
// a task to done, grow a note that links the work, and read it all back
// through one graph. Asserts the cross-type seams the per-pillar tests can't.
func TestDailyDriver_EndToEnd(t *testing.T) {
	t.Parallel()
	v := newVault(t)

	// Morning: capture + plan.
	v.run(t, "add", "Standup: picked up the mapper bug", "--at", "09:00", "-q")
	created := v.runJSON(t, "todo", "add", "Fix the mapper bug")
	todoID := str(created, "id")

	// During: knowledge grows out of the work, linking the todo by ULID.
	v.run(t, "note", "create", "Mapper Bug RCA",
		"--description", "Root cause of the mapper bug.",
		"--stage", "seedling",
		"--body", "Work tracked in [["+todoID+"]].", "-q")

	// Evening: close the loop.
	v.run(t, "todo", "done", todoID, "-q")
	v.run(t, "add", "Wrapped: mapper bug fixed, RCA note started", "-q")

	// The note's ULID link resolved to the todo — cross-type edge.
	show := v.runJSON(t, "note", "show", "mapper-bug-rca")
	if n := resolvedLinks(show); n != 1 {
		t.Errorf("note→todo ULID link unresolved: %v", show)
	}

	// The whole day is one queryable graph: the note→todo edge is visible.
	v.run(t, "index", "-q")
	edges := v.run(t, "query",
		"SELECT src, dst_key FROM edges WHERE dst_key = '"+todoID+"'", "--raw")
	if !strings.Contains(edges, todoID) {
		t.Errorf("note→todo edge not queryable:\n%s", edges)
	}

	// And the day file tells the story with provenance.
	day := v.readFile(t, "log/"+today()+".md")
	for _, want := range []string{"Standup", "Wrapped", "· acceptance"} {
		if !strings.Contains(day, want) {
			t.Errorf("day file missing %q:\n%s", want, day)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// --quiet — logger suppression
// ─────────────────────────────────────────────────────────────────────────────

// --quiet raises the effective log floor: a clean run leaves stderr fully
// empty, not just missing the one banner string.
func TestQuiet_SuppressesInitLog(t *testing.T) {
	t.Parallel()
	v := newVault(t)

	_, stderr, err := v.exec("--quiet", "todo", "add", "quiet suppresses init log")
	if err != nil {
		t.Fatalf("rk --quiet todo add: %v\nstderr:\n%s", err, stderr)
	}
	if strings.Contains(stderr, "reckon initialized") {
		t.Errorf("stderr contains init log line under --quiet:\n%s", stderr)
	}
	if stderr != "" {
		t.Errorf("stderr not empty under --quiet:\n%q", stderr)
	}
}

// Baseline: with no flags, the default floor is WARN — INFO-level noise
// (including the init banner) is suppressed even without --quiet.
func TestBaseline_SuppressesInitLog(t *testing.T) {
	t.Parallel()
	v := newVault(t)

	_, stderr, err := v.exec("todo", "add", "baseline suppresses init log")
	if err != nil {
		t.Fatalf("rk todo add: %v\nstderr:\n%s", err, stderr)
	}
	if strings.Contains(stderr, `msg="reckon initialized"`) {
		t.Errorf("stderr contains init log line under default flags:\n%s", stderr)
	}
}

// The default WARN floor is opt-out, not a removal: an explicit
// --log-level INFO still surfaces the banner.
func TestLogLevelInfo_ShowsInitLog(t *testing.T) {
	t.Parallel()
	v := newVault(t)

	_, stderr, err := v.exec("--log-level", "INFO", "todo", "add", "explicit info shows init log")
	if err != nil {
		t.Fatalf("rk --log-level INFO todo add: %v\nstderr:\n%s", err, stderr)
	}
	if !strings.Contains(stderr, `msg="reckon initialized"`) {
		t.Errorf("stderr missing init log line under --log-level INFO:\n%s", stderr)
	}
}

// An explicit --log-level always outranks --quiet: --quiet only raises the
// default floor, it never clamps an explicit user request.
func TestQuiet_ExplicitLogLevelWins(t *testing.T) {
	t.Parallel()
	v := newVault(t)

	_, stderr, err := v.exec("--quiet", "--log-level", "DEBUG", "todo", "add", "explicit level wins")
	if err != nil {
		t.Fatalf("rk --quiet --log-level DEBUG todo add: %v\nstderr:\n%s", err, stderr)
	}
	if !strings.Contains(stderr, "reckon initialized") {
		t.Errorf("explicit --log-level DEBUG did not override --quiet:\n%s", stderr)
	}
}

// --log-level WARN alone (no --quiet) already silences the init line —
// regression baseline for pre-existing behavior.
func TestLogLevelWarn_SuppressesInitLog(t *testing.T) {
	t.Parallel()
	v := newVault(t)

	_, stderr, err := v.exec("--log-level", "WARN", "todo", "add", "log level warn alone")
	if err != nil {
		t.Fatalf("rk --log-level WARN todo add: %v\nstderr:\n%s", err, stderr)
	}
	if strings.Contains(stderr, "reckon initialized") {
		t.Errorf("stderr contains init log line under --log-level WARN:\n%s", stderr)
	}
}

// --quiet and --log-level WARN together are redundant, not conflicting.
func TestQuiet_RedundantWithWarn(t *testing.T) {
	t.Parallel()
	v := newVault(t)

	_, stderr, err := v.exec("--quiet", "--log-level", "WARN", "todo", "add", "quiet redundant with warn")
	if err != nil {
		t.Fatalf("rk --quiet --log-level WARN todo add: %v\nstderr:\n%s", err, stderr)
	}
	if strings.Contains(stderr, "reckon initialized") {
		t.Errorf("stderr contains init log line under --quiet --log-level WARN:\n%s", stderr)
	}
}

// --quiet is orthogonal to output-mode flags: JSON/NDJSON on stdout stays
// intact and parseable (AC3), independent of the logger-suppression fix.
func TestQuiet_JSONStdoutIntact(t *testing.T) {
	t.Parallel()

	t.Run("json", func(t *testing.T) {
		t.Parallel()
		v := newVault(t)
		stdout, stderr, err := v.exec("--quiet", "--json", "todo", "add", "quiet json stdout intact")
		if err != nil {
			t.Fatalf("rk --quiet --json todo add: %v\nstderr:\n%s", err, stderr)
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(stdout), &m); err != nil {
			t.Errorf("stdout not valid JSON under --quiet --json: %v\nstdout:\n%s", err, stdout)
		}
	})

	t.Run("ndjson", func(t *testing.T) {
		t.Parallel()
		v := newVault(t)
		stdout, stderr, err := v.exec("--quiet", "--ndjson", "todo", "add", "quiet ndjson stdout intact")
		if err != nil {
			t.Fatalf("rk --quiet --ndjson todo add: %v\nstderr:\n%s", err, stderr)
		}
		for _, line := range strings.Split(strings.TrimRight(stdout, "\n"), "\n") {
			if line == "" {
				continue
			}
			var m map[string]any
			if err := json.Unmarshal([]byte(line), &m); err != nil {
				t.Errorf("stdout line not valid NDJSON under --quiet --ndjson: %v\nline:\n%s", err, line)
			}
		}
	})
}

// --quiet outranks LOG_LEVEL/RECKON_DEBUG env vars when no explicit
// --log-level flag is given: flag beats env, the same precedence order
// --log-level itself already uses against those env vars.
func TestQuiet_EnvLogLevelLoses(t *testing.T) {
	t.Parallel()

	t.Run("LOG_LEVEL", func(t *testing.T) {
		t.Parallel()
		v := newVault(t)
		_, stderr, err := v.execEnv([]string{"LOG_LEVEL=DEBUG"},
			"--quiet", "todo", "add", "env log level loses to quiet")
		if err != nil {
			t.Fatalf("rk --quiet todo add (LOG_LEVEL=DEBUG): %v\nstderr:\n%s", err, stderr)
		}
		if strings.Contains(stderr, "reckon initialized") {
			t.Errorf("stderr contains init log line under --quiet with LOG_LEVEL=DEBUG:\n%s", stderr)
		}
	})

	t.Run("RECKON_DEBUG", func(t *testing.T) {
		t.Parallel()
		v := newVault(t)
		_, stderr, err := v.execEnv([]string{"RECKON_DEBUG=1"},
			"--quiet", "todo", "add", "reckon debug env loses to quiet")
		if err != nil {
			t.Fatalf("rk --quiet todo add (RECKON_DEBUG=1): %v\nstderr:\n%s", err, stderr)
		}
		if strings.Contains(stderr, "reckon initialized") {
			t.Errorf("stderr contains init log line under --quiet with RECKON_DEBUG=1:\n%s", stderr)
		}
	})
}
