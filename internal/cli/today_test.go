// Package cli — TDD red tests for v1-T7 `rk today` (split-actuator agenda,
// reckon-liml).
//
// internal/cli/today.go currently ships the LEGACY, DB-journal-backed `rk
// today` (dumps the current day's journal to stdout via fmt.Print, bypassing
// cmd.OutOrStdout() entirely; RunE lives at today.go — see plan.md decision
// A). Critically, `today.go`'s todayCmd carries NO `Annotations:
// {"requiresDB":"false"}` yet, so root.go's PersistentPreRunE walks the
// ancestor chain (root.go:94-99), finds no requiresDB=false anywhere, and
// calls initServiceE() — which eagerly opens/migrates a REAL sqlite file at
// config.DatabasePath() (~/.reckon/reckon.db by default; only
// $RECKON_DATA_DIR redirects it, internal/config/config.go:18-38). Every
// runToday() call below sets RECKON_DATA_DIR to a fresh t.TempDir() as a
// safety net so these tests can never touch a developer's real ~/.reckon,
// regardless of whether the command they invoke resolves to the legacy
// RunE (today, red state) or the new agenda/act/open dispatch (green state,
// once today.go gains requiresDB=false and this becomes a no-op).
//
// This file does NOT reference any not-yet-existing Go identifier
// (buildAgenda, agendaItem, agendaResult, todayActResult, todayOpenResult,
// resetTodayFlags, etc. all remain undefined until today.go is rewritten).
// Every scenario drives RootCmd.SetArgs + Execute and asserts on
// stdout/stderr/exit code, raw file bytes, and `rk query`-observed index SQL
// — exactly the black-box surface query_test.go/todo_test.go/
// todo_recur_test.go already use. Because `today` (the verb) already exists
// as a registered cobra command, `rk today`/`rk today act ...`/`rk today
// open ...` all resolve through RootCmd today (act/open aren't registered
// subcommands yet, so cobra falls through to todayCmd's own legacy RunE with
// ["act", ref, key, ...] / ["open", ref] as plain positional args — a
// no-op from the new agenda's point of view, but NOT a compile error and NOT
// an "unknown command" cobra error). That is the correct TDD-red state at
// this gate: the package builds cleanly, but every scenario below fails at
// runtime against the wrong (legacy) behavior.
//
// ─────────────────────────────────────────────────────────────────────────
// Pinned contract for the implementer (plan.md "Files to modify" —
// internal/cli/today.go — plus acceptance-criteria.md §2/§4). These types do
// NOT exist yet; this comment is the contract this file's generic
// map[string]any decoding below assumes once they do (mirrors
// todo_recur_test.go's "Pinned contract" header convention):
//
//	// agendaItem is one row of `rk today`'s agenda output (native or
//	// external/work-ticket). JSON keys chosen to match the existing
//	// snake_case convention for compound names (did_entry_id, source_url).
//	type agendaItem struct {
//	    ID        string `json:"id"`
//	    Type      string `json:"type"`                 // "todo" | "work-ticket"
//	    Path      string `json:"path,omitempty"`
//	    State     string `json:"state,omitempty"`
//	    Scheduled string `json:"scheduled,omitempty"`
//	    Deadline  string `json:"deadline,omitempty"`
//	    Pinned    string `json:"pinned,omitempty"`
//	    Priority  string `json:"priority,omitempty"`
//	    Source    string `json:"source,omitempty"`      // external only, e.g. "jira"
//	    SourceURL string `json:"source_url,omitempty"`  // external only
//	    ReadOnly  bool   `json:"read_only,omitempty"`   // true for external/work-ticket rows
//	    Body      string `json:"body,omitempty"`
//	    Title     string `json:"title,omitempty"`       // derived first non-empty body line (reckon-fnqs.3)
//	}
//
//	// agendaResult wraps `rk today`'s items so --json emits a single object
//	// ({"items": []} on empty), mirroring todoListResult.
//	type agendaResult struct {
//	    Items []agendaItem `json:"items"`
//	}
//
//	// todayActResult is the structured summary of one `rk today act` run,
//	// mirroring todoDoneResult's shape (the "x" key reuses
//	// completeDurableTodoNode, which is doneDurableTodo/doneRecurringTodo's
//	// shared body per plan.md Q6, so the recurrence fields mirror
//	// todoDoneResult verbatim).
//	type todayActResult struct {
//	    Ref          string `json:"ref"`
//	    Key          string `json:"key"`
//	    ID           string `json:"id,omitempty"`
//	    Path         string `json:"path,omitempty"`
//	    State        string `json:"state,omitempty"`
//	    Scheduled    string `json:"scheduled,omitempty"`
//	    Deadline     string `json:"deadline,omitempty"`
//	    Pinned       string `json:"pinned,omitempty"`
//	    Priority     string `json:"priority,omitempty"`
//	    Recurred     bool   `json:"recurred,omitempty"`
//	    Repeat       string `json:"repeat,omitempty"`
//	    DidEntryID   string `json:"did_entry_id,omitempty"`
//	    DidEntryPath string `json:"did_entry_path,omitempty"`
//	}
//
//	// todayOpenResult is `rk today open <ref>`'s structured summary.
//	type todayOpenResult struct {
//	    Ref       string `json:"ref"`
//	    SourceURL string `json:"source_url"`
//	}
//
// Precedent / harness reuse (do not redefine these — they already live in
// this package): setupQueryVault, writeTestNode, resetCLIFlags, buildIndex,
// runQuery, parseNDJSONMaps (query_test.go); mustWriteFile, mustReadFile,
// isValidULID (adopt_test.go); mustDecodeJSON (todo_test.go); pinTodoNow,
// writeRecurringTodo, inboxPath, parseLogDayFile (todo_recur_test.go);
// dayLogPath, dayLogRelPath, utcToday (add_test.go).
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MikeBiancalana/reckon/internal/node"
)

// ─────────────────────────────────────────────────────────────────────────────
// Harness helpers
// ─────────────────────────────────────────────────────────────────────────────

// runToday executes `rk today <args...> --vault <vault>` through RootCmd and
// returns (stdout, stderr, error), mirroring runQuery (query_test.go) and
// runTodo (todo_test.go). It also isolates the legacy DB-journal path (see
// file header) via RECKON_DATA_DIR so no invocation here can ever touch a
// real ~/.reckon on the host running the tests. The caller must call
// resetCLIFlags() before another Execute within the same test
// (t.Cleanup(resetCLIFlags) covers end-of-test).
func runToday(t *testing.T, vault string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	t.Setenv("RECKON_DATA_DIR", filepath.Join(t.TempDir(), "reckon-data"))
	var outBuf, errBuf bytes.Buffer
	RootCmd.SetOut(&outBuf)
	RootCmd.SetErr(&errBuf)
	RootCmd.SetArgs(append([]string{"today", "--vault", vault}, args...))
	err = RootCmd.Execute()
	return outBuf.String(), errBuf.String(), err
}

// decodeAgendaItems decodes stdout as the {"items": [...]} envelope
// agendaResult is pinned to (see file header), without referencing the
// not-yet-existing agendaResult/agendaItem Go types directly: each item
// comes back as a generic map[string]any so field-presence assertions work
// regardless of exact struct tags once today.go is rewritten.
func decodeAgendaItems(t *testing.T, stdout string) []map[string]any {
	t.Helper()
	var env struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("decode agenda envelope: %v\nstdout: %q", err, stdout)
	}
	return env.Items
}

// dayOffset returns date shifted by days (positive or negative), preserving
// the strict YYYY-MM-DD / UTC / date-only discipline parseSchedDate and the
// todoNow seam both use (recur.go).
func dayOffset(t *testing.T, date string, days int) string {
	t.Helper()
	tm, err := time.ParseInLocation("2006-01-02", date, time.UTC)
	if err != nil {
		t.Fatalf("dayOffset: parse %q: %v", date, err)
	}
	return tm.AddDate(0, 0, days).Format("2006-01-02")
}

// todoFixtureSrc renders the exact byte-for-byte source of a durable native
// todo (id/type/state[/scheduled] + optional extra frontmatter + body),
// hand-authored so byte-preservation assertions have a known-exact "before"
// string to diff against — mirrors todo_recur_test.go's recurringTodoSrc,
// but for the general (non-recurring) case this file needs. scheduled=""
// omits the scheduled: line entirely (EC-6: absent, not empty-string).
func todoFixtureSrc(id, state, scheduled, body string, extraFM ...string) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString("id: " + id + "\n")
	sb.WriteString("type: todo\n")
	sb.WriteString("state: " + state + "\n")
	if scheduled != "" {
		sb.WriteString("scheduled: " + scheduled + "\n")
	}
	for _, line := range extraFM {
		sb.WriteString(line + "\n")
	}
	sb.WriteString("---\n")
	sb.WriteString(body + "\n")
	return sb.String()
}

// writeTodoFixture writes todoFixtureSrc's output to todos/<id>.md and
// returns (absolute path, source string), mirroring
// todo_recur_test.go's writeRecurringTodo.
func writeTodoFixture(t *testing.T, vault, id, state, scheduled, body string, extraFM ...string) (path, src string) {
	t.Helper()
	src = todoFixtureSrc(id, state, scheduled, body, extraFM...)
	path = filepath.Join(vault, "todos", id+".md")
	mustWriteFile(t, path, src)
	return path, src
}

// snapshotVaultFiles reads every regular file under vault into a
// path(relative)->content map, for later byte-identity comparison (EC-4/
// TS-5.1: an external-row actuation rejection must leave every vault file
// byte-identical).
func snapshotVaultFiles(t *testing.T, vault string) map[string]string {
	t.Helper()
	snap := map[string]string{}
	err := filepath.Walk(vault, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(vault, path)
		if relErr != nil {
			return relErr
		}
		snap[rel] = mustReadFile(t, path)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshotVaultFiles: walk %s: %v", vault, err)
	}
	return snap
}

// vaultSnapshotsEqual reports whether two snapshotVaultFiles results are
// identical (same file set, same bytes).
func vaultSnapshotsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

// ─────────────────────────────────────────────────────────────────────────────
// AC-1 (correct surfaced set) — TS-1.1..TS-1.7 / EC-1/EC-2/EC-3/EC-6/EC-7
// ─────────────────────────────────────────────────────────────────────────────

// TestToday_SurfacesOverdue (TS-1.1): an open durable todo scheduled
// yesterday is overdue and must surface.
func TestToday_SurfacesOverdue(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-10")
	today := "2026-07-10"
	yesterday := dayOffset(t, today, -1)

	id := node.Mint()
	writeTodoFixture(t, vault, id, "open", yesterday, "Overdue task.")

	stdout, stderr, err := runToday(t, vault, "--json")
	if err != nil {
		t.Fatalf("rk today --json: %v\nstderr: %s", err, stderr)
	}
	items := decodeAgendaItems(t, stdout)
	found := false
	for _, it := range items {
		if it["id"] == id {
			found = true
		}
	}
	if !found {
		t.Errorf("overdue item %q missing from agenda: %v", id, items)
	}
}

// TestToday_SurfacesScheduledToday (TS-1.2): a todo scheduled exactly today
// must surface.
func TestToday_SurfacesScheduledToday(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-10")
	today := "2026-07-10"

	id := node.Mint()
	writeTodoFixture(t, vault, id, "open", today, "Scheduled for today.")

	stdout, stderr, err := runToday(t, vault, "--json")
	if err != nil {
		t.Fatalf("rk today --json: %v\nstderr: %s", err, stderr)
	}
	items := decodeAgendaItems(t, stdout)
	found := false
	for _, it := range items {
		if it["id"] == id {
			found = true
		}
	}
	if !found {
		t.Errorf("scheduled-today item %q missing from agenda: %v", id, items)
	}
}

// TestToday_SurfacesPinned (TS-1.3): a todo with no scheduled/deadline at
// all, but pinned=today, must still surface.
func TestToday_SurfacesPinned(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-10")
	today := "2026-07-10"

	id := node.Mint()
	writeTodoFixture(t, vault, id, "open", "", "Pinned for today.", "pinned: "+today)

	stdout, stderr, err := runToday(t, vault, "--json")
	if err != nil {
		t.Fatalf("rk today --json: %v\nstderr: %s", err, stderr)
	}
	items := decodeAgendaItems(t, stdout)
	found := false
	for _, it := range items {
		if it["id"] == id {
			found = true
		}
	}
	if !found {
		t.Errorf("pinned item %q missing from agenda: %v", id, items)
	}
}

// TestToday_ExcludesFutureScheduled (TS-1.4): a todo scheduled tomorrow,
// with no pin, must NOT surface.
func TestToday_ExcludesFutureScheduled(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-10")
	today := "2026-07-10"
	tomorrow := dayOffset(t, today, 1)

	id := node.Mint()
	writeTodoFixture(t, vault, id, "open", tomorrow, "Scheduled for tomorrow.")

	stdout, stderr, err := runToday(t, vault, "--json")
	if err != nil {
		t.Fatalf("rk today --json: %v\nstderr: %s", err, stderr)
	}
	items := decodeAgendaItems(t, stdout)
	for _, it := range items {
		if it["id"] == id {
			t.Errorf("future-scheduled item %q must not surface: %v", id, items)
		}
	}
}

// TestToday_ExcludesDone (TS-1.5): a todo scheduled today but already done
// must NOT surface (mirrors rk todo list's default open-only filter).
func TestToday_ExcludesDone(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-10")
	today := "2026-07-10"

	id := node.Mint()
	writeTodoFixture(t, vault, id, "done", today, "Already done.")

	stdout, stderr, err := runToday(t, vault, "--json")
	if err != nil {
		t.Fatalf("rk today --json: %v\nstderr: %s", err, stderr)
	}
	items := decodeAgendaItems(t, stdout)
	for _, it := range items {
		if it["id"] == id {
			t.Errorf("done item %q must not surface: %v", id, items)
		}
	}
}

// TestToday_DedupesOverdueAndPinned (TS-1.6/EC-2): a todo that is BOTH
// overdue and today-pinned must appear exactly once, not twice.
func TestToday_DedupesOverdueAndPinned(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-10")
	today := "2026-07-10"
	yesterday := dayOffset(t, today, -1)

	id := node.Mint()
	writeTodoFixture(t, vault, id, "open", yesterday, "Overdue and pinned.", "pinned: "+today)

	stdout, stderr, err := runToday(t, vault, "--json")
	if err != nil {
		t.Fatalf("rk today --json: %v\nstderr: %s", err, stderr)
	}
	items := decodeAgendaItems(t, stdout)
	count := 0
	for _, it := range items {
		if it["id"] == id {
			count++
		}
	}
	if count != 1 {
		t.Errorf("overdue+pinned item %q must appear exactly once, got %d: %v", id, count, items)
	}
}

// TestToday_EmptyAgenda (TS-1.7/EC-1): a vault with zero qualifying todos
// exits 0 with an empty result set in both --json and pretty mode, never an
// error.
func TestToday_EmptyAgenda(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-10")

	stdout, stderr, err := runToday(t, vault, "--json")
	if err != nil {
		t.Fatalf("rk today --json on empty vault: %v\nstderr: %s", err, stderr)
	}
	items := decodeAgendaItems(t, stdout)
	if len(items) != 0 {
		t.Errorf("want 0 items for an empty vault, got %d: %v", len(items), items)
	}

	resetCLIFlags()
	prettyOut, prettyErr, err2 := runToday(t, vault)
	if err2 != nil {
		t.Fatalf("rk today (pretty) on empty vault: %v\nstderr: %s", err2, prettyErr)
	}
	if strings.TrimSpace(prettyOut) == "" {
		t.Errorf("pretty mode: expected a short 'nothing due'-style line, got empty output (stderr=%q)", prettyErr)
	}
}

// TestToday_SkipsMalformedDateRow (EC-7): a row with an unparseable
// scheduled: value is skipped (with a stderr warning), while a sibling valid
// row still loads — one bad row must never abort the whole agenda.
func TestToday_SkipsMalformedDateRow(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-10")
	today := "2026-07-10"
	yesterday := dayOffset(t, today, -1)

	badID := node.Mint()
	writeTestNode(t, vault, "todos/"+badID+".md", badID, "todo", "Malformed date todo.",
		"state: open", "scheduled: not-a-date")

	goodID := node.Mint()
	writeTodoFixture(t, vault, goodID, "open", yesterday, "Valid overdue todo.")

	stdout, stderr, err := runToday(t, vault, "--json")
	if err != nil {
		t.Fatalf("rk today --json: %v\nstderr: %s", err, stderr)
	}
	items := decodeAgendaItems(t, stdout)
	var sawGood, sawBad bool
	for _, it := range items {
		if it["id"] == goodID {
			sawGood = true
		}
		if it["id"] == badID {
			sawBad = true
		}
	}
	if !sawGood {
		t.Errorf("valid overdue item %q missing from agenda when a sibling row has a malformed date: %v", goodID, items)
	}
	if sawBad {
		t.Errorf("malformed-date item %q must be skipped, not surfaced: %v", badID, items)
	}
	if strings.TrimSpace(stderr) == "" {
		t.Errorf("expected a stderr warning about the malformed scheduled: value on %q, got empty stderr", badID)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// reckon-fnqs.3 — subject/body node convention: `rk today` renders only the
// derived title (not the full body) in pretty mode, while --json carries
// both title and body.
//
// RED-state note: TestToday_JSON_TitleAndBody reuses decodeAgendaItems (this
// file's existing map[string]any decoder) rather than referencing a Title
// field on the pinned agendaItem struct directly -- that field does not
// exist until today.go is updated (Phase 4), and a direct struct reference
// would fail to compile, taking the whole cli package down with it.
// Decoding generically localizes the red state to a runtime assertion
// failure (missing "title" key today), matching this ticket's "tests must
// compile but must fail" rule.
// ─────────────────────────────────────────────────────────────────────────────

// TestToday_Pretty_RendersTitleOnly (AC5): a scheduled-today todo with a
// 5-line body renders only its first line ("Ship it.") in pretty `rk today`
// output -- not the rest of the body. RED today: agendaResult.Pretty()
// (today.go:128) still interpolates it.Body in full.
func TestToday_Pretty_RendersTitleOnly(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-10")
	today := "2026-07-10"

	id := node.Mint()
	writeTodoFixture(t, vault, id, "open", today, "Ship it.\n\nline2\nline3\nline4\n")

	out, stderr, err := runToday(t, vault)
	if err != nil {
		t.Fatalf("rk today: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(out, "Ship it.") {
		t.Errorf("pretty output missing title %q: %q", "Ship it.", out)
	}
	for _, unwanted := range []string{"line2", "line3", "line4"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("pretty output leaked body line %q, want title-only rendering: %q", unwanted, out)
		}
	}
}

// TestToday_JSON_TitleAndBody (AC7): `rk today --json` carries BOTH the
// derived title and the full multi-line body on the same agenda item --
// title is additive, not a body replacement. RED today: no "title" key
// exists in the JSON output at all, so the decoded title is always "".
func TestToday_JSON_TitleAndBody(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-10")
	today := "2026-07-10"

	id := node.Mint()
	writeTodoFixture(t, vault, id, "open", today, "Ship it.\n\nline2\nline3\nline4\n")

	stdout, stderr, err := runToday(t, vault, "--json")
	if err != nil {
		t.Fatalf("rk today --json: %v\nstderr: %s", err, stderr)
	}
	items := decodeAgendaItems(t, stdout)

	var found map[string]any
	for _, it := range items {
		if it["id"] == id {
			found = it
		}
	}
	if found == nil {
		t.Fatalf("item %q missing from agenda JSON: %v", id, items)
	}
	if title, _ := found["title"].(string); title != "Ship it." {
		t.Errorf("title = %v, want %q (item=%v)", found["title"], "Ship it.", found)
	}
	body, _ := found["body"].(string)
	for _, want := range []string{"Ship it.", "line2", "line3", "line4"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q, got %q", want, body)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// AC-2 (schedule keys write through) — TS-2.1..TS-2.5
// ─────────────────────────────────────────────────────────────────────────────

// TestTodayAct_DeferWritesScheduledSpanLocal (TS-2.1): `act <ref> d
// tomorrow` on an overdue native todo with hand-added extra frontmatter and
// body prose updates only the scheduled: line, byte-preserves everything
// else, and the change is visible on a subsequent rk query.
func TestTodayAct_DeferWritesScheduledSpanLocal(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-10")
	today := "2026-07-10"
	yesterday := dayOffset(t, today, -1)
	tomorrow := dayOffset(t, today, 1)

	id := node.Mint()
	body := "Ship the report.\n\n> keep this blockquote\n\n```text\nfenced content\n```\n"
	path, src := writeTodoFixture(t, vault, id, "open", yesterday, body, "x-obsidian-extra: keep-me")

	_, stderr, err := runToday(t, vault, "act", id, "d", "tomorrow")
	if err != nil {
		t.Fatalf("rk today act d tomorrow: %v\nstderr: %s", err, stderr)
	}

	want := strings.Replace(src, "scheduled: "+yesterday, "scheduled: "+tomorrow, 1)
	got := mustReadFile(t, path)
	if got != want {
		t.Fatalf("defer not span-local\n--- want ---\n%q\n--- got ---\n%q", want, got)
	}

	resetCLIFlags()
	stdoutQ, stderrQ, errQ := runQuery(t, vault, fmt.Sprintf(
		"SELECT value FROM node_props WHERE id='%s' AND key='scheduled'", id))
	if errQ != nil {
		t.Fatalf("rk query scheduled: %v\nstderr: %s", errQ, stderrQ)
	}
	rows := parseNDJSONMaps(t, stdoutQ)
	if len(rows) != 1 || rows[0]["value"] != tomorrow {
		t.Fatalf("scheduled not reindexed after defer, got rows=%v, want value=%q", rows, tomorrow)
	}
}

// TestTodayAct_DeadlineInsertsDeadline (TS-2.2): `act <ref> D <date>` on a
// todo with no pre-existing deadline: prop inserts a fresh deadline: line
// (InsertField path — the new line lands FIRST inside the frontmatter
// fence, per internal/node/insert.go).
func TestTodayAct_DeadlineInsertsDeadline(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-10")
	today := "2026-07-10"
	deadline := dayOffset(t, today, 5)

	id := node.Mint()
	path, src := writeTodoFixture(t, vault, id, "open", "", "Ship v1.")

	_, stderr, err := runToday(t, vault, "act", id, "D", deadline)
	if err != nil {
		t.Fatalf("rk today act D %s: %v\nstderr: %s", deadline, err, stderr)
	}

	want := "---\ndeadline: " + deadline + "\n" + strings.TrimPrefix(src, "---\n")
	got := mustReadFile(t, path)
	if got != want {
		t.Fatalf("deadline insert not span-local\n--- want ---\n%q\n--- got ---\n%q", want, got)
	}
}

// TestTodayAct_PinSetsPinnedAndSurfaces (TS-2.3): `act <ref> t` on an
// unpinned native todo sets pinned: to today's date, and the todo
// subsequently appears in `rk today` even with no scheduled/deadline match.
func TestTodayAct_PinSetsPinnedAndSurfaces(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-10")
	today := "2026-07-10"

	id := node.Mint()
	path, src := writeTodoFixture(t, vault, id, "open", "", "Someday maybe.")

	_, stderr, err := runToday(t, vault, "act", id, "t")
	if err != nil {
		t.Fatalf("rk today act t: %v\nstderr: %s", err, stderr)
	}

	want := "---\npinned: " + today + "\n" + strings.TrimPrefix(src, "---\n")
	got := mustReadFile(t, path)
	if got != want {
		t.Fatalf("pin not span-local\n--- want ---\n%q\n--- got ---\n%q", want, got)
	}

	resetCLIFlags()
	stdout, stderr2, err2 := runToday(t, vault, "--json")
	if err2 != nil {
		t.Fatalf("rk today --json after pin: %v\nstderr: %s", err2, stderr2)
	}
	items := decodeAgendaItems(t, stdout)
	found := false
	for _, it := range items {
		if it["id"] == id {
			found = true
		}
	}
	if !found {
		t.Errorf("pinned item %q not present in agenda after act t: %v", id, items)
	}
}

// TestTodayAct_PrioritySetsLetter (TS-2.4): `act <ref> p B` sets a validated
// priority letter span-locally; `act <ref> p Z` is rejected and leaves the
// file untouched.
func TestTodayAct_PrioritySetsLetter(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-10")
	today := "2026-07-10"

	t.Run("valid_letter_sets_priority", func(t *testing.T) {
		resetCLIFlags()
		id := node.Mint()
		path, src := writeTodoFixture(t, vault, id, "open", today, "Ship the report.")

		_, stderr, err := runToday(t, vault, "act", id, "p", "B")
		if err != nil {
			t.Fatalf("rk today act p B: %v\nstderr: %s", err, stderr)
		}

		want := "---\npriority: B\n" + strings.TrimPrefix(src, "---\n")
		got := mustReadFile(t, path)
		if got != want {
			t.Fatalf("priority insert not span-local\n--- want ---\n%q\n--- got ---\n%q", want, got)
		}
	})

	t.Run("invalid_letter_rejected", func(t *testing.T) {
		resetCLIFlags()
		id := node.Mint()
		path, src := writeTodoFixture(t, vault, id, "open", today, "Ship the report.")

		_, stderr, err := runToday(t, vault, "act", id, "p", "Z")
		if err == nil {
			t.Fatal("expected a validation error for priority Z, got nil")
		}
		// Must be specifically about the invalid priority value, not an
		// unrelated failure (see TestTodayAct_ExternalRowRejected's comment
		// on the same false-green risk from the still-registered legacy
		// journal-dump RunE).
		msg := strings.ToLower(err.Error() + " " + stderr)
		if !strings.Contains(msg, "priority") {
			t.Errorf("expected a priority-specific validation error, got err=%v stderr=%q", err, stderr)
		}
		got := mustReadFile(t, path)
		if got != src {
			t.Fatalf("file modified despite priority validation error\n--- want (untouched) ---\n%q\n--- got ---\n%q", src, got)
		}
	})
}

// TestTodayAct_WriteThroughVisibleToQuery (TS-2.5): a fresh act mutation is
// visible to a subsequent `rk query`, with no manual `rk index` step in
// between (exercises the eager post-write reconcile, plan.md Q11).
func TestTodayAct_WriteThroughVisibleToQuery(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-10")
	today := "2026-07-10"

	id := node.Mint()
	writeTodoFixture(t, vault, id, "open", today, "Ship it.")

	_, stderr, err := runToday(t, vault, "act", id, "p", "A")
	if err != nil {
		t.Fatalf("rk today act p A: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()

	// Deliberately no buildIndex call here — TS-2.5 is specifically testing
	// that no manual `rk index` step is required.
	stdout, stderrQ, errQ := runQuery(t, vault, fmt.Sprintf(
		"SELECT value FROM node_props WHERE id='%s' AND key='priority'", id))
	if errQ != nil {
		t.Fatalf("rk query priority (no manual rk index): %v\nstderr: %s", errQ, stderrQ)
	}
	rows := parseNDJSONMaps(t, stdout)
	if len(rows) != 1 || rows[0]["value"] != "A" {
		t.Fatalf("priority not visible to rk query without a manual rk index, got rows=%v", rows)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// AC-3 (do keys write through) — TS-3.1..TS-3.3 (TS-3.4 ephemeral descoped
// per plan.md EC-12 decision)
// ─────────────────────────────────────────────────────────────────────────────

// TestTodayAct_DoneFlipsStateSpanLocal (TS-3.1): `act <ref> x` flips
// state->done via a span-local edit and the row drops from the next agenda
// load.
func TestTodayAct_DoneFlipsStateSpanLocal(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-10")
	today := "2026-07-10"

	id := node.Mint()
	path, src := writeTodoFixture(t, vault, id, "open", today, "Ship the report.\n\n> notes here\n", "x-obsidian-extra: keep-me")

	_, stderr, err := runToday(t, vault, "act", id, "x", "--no-log")
	if err != nil {
		t.Fatalf("rk today act x --no-log: %v\nstderr: %s", err, stderr)
	}

	want := strings.Replace(src, "state: open", "state: done", 1)
	got := mustReadFile(t, path)
	if got != want {
		t.Fatalf("done not span-local\n--- want ---\n%q\n--- got ---\n%q", want, got)
	}

	resetCLIFlags()
	stdoutList, stderrList, errList := runToday(t, vault, "--json")
	if errList != nil {
		t.Fatalf("rk today --json after done: %v\nstderr: %s", errList, stderrList)
	}
	for _, it := range decodeAgendaItems(t, stdoutList) {
		if it["id"] == id {
			t.Errorf("done item %q still present in agenda: %v", id, it)
		}
	}
}

// TestTodayAct_StartSetsInProgress (TS-3.2): `act <ref> i` sets
// state->in-progress span-locally, and (per B4/B5) the row stays visible in
// the default agenda since its scheduled-today match still holds.
func TestTodayAct_StartSetsInProgress(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-10")
	today := "2026-07-10"

	id := node.Mint()
	path, src := writeTodoFixture(t, vault, id, "open", today, "Ship the report.")

	_, stderr, err := runToday(t, vault, "act", id, "i")
	if err != nil {
		t.Fatalf("rk today act i: %v\nstderr: %s", err, stderr)
	}

	want := strings.Replace(src, "state: open", "state: in-progress", 1)
	got := mustReadFile(t, path)
	if got != want {
		t.Fatalf("start not span-local\n--- want ---\n%q\n--- got ---\n%q", want, got)
	}

	resetCLIFlags()
	stdoutList, stderrList, errList := runToday(t, vault, "--json")
	if errList != nil {
		t.Fatalf("rk today --json after start: %v\nstderr: %s", errList, stderrList)
	}
	found := false
	for _, it := range decodeAgendaItems(t, stdoutList) {
		if it["id"] == id {
			found = true
		}
	}
	if !found {
		t.Errorf("in-progress item %q dropped from agenda, want still present per B4/B5", id)
	}
}

// TestTodayAct_CancelSetsCancelled (TS-3.3): `act <ref> c` sets
// state->cancelled span-locally, and the row drops from the default agenda
// (cancelled is terminal-ish, excluded by B5's state predicate).
func TestTodayAct_CancelSetsCancelled(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-10")
	today := "2026-07-10"

	id := node.Mint()
	path, src := writeTodoFixture(t, vault, id, "open", today, "Ship the report.")

	_, stderr, err := runToday(t, vault, "act", id, "c")
	if err != nil {
		t.Fatalf("rk today act c: %v\nstderr: %s", err, stderr)
	}

	want := strings.Replace(src, "state: open", "state: cancelled", 1)
	got := mustReadFile(t, path)
	if got != want {
		t.Fatalf("cancel not span-local\n--- want ---\n%q\n--- got ---\n%q", want, got)
	}

	resetCLIFlags()
	stdoutList, stderrList, errList := runToday(t, vault, "--json")
	if errList != nil {
		t.Fatalf("rk today --json after cancel: %v\nstderr: %s", errList, stderrList)
	}
	for _, it := range decodeAgendaItems(t, stdoutList) {
		if it["id"] == id {
			t.Errorf("cancelled item %q still present in agenda: %v", id, it)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// AC-4 (completion logs a did entry) — TS-4.1..TS-4.4
// ─────────────────────────────────────────────────────────────────────────────

// TestTodayAct_DoneEmitsDidEntry (TS-4.1): completing a native row via `act
// x` (default log-on) writes a linked log-entry node into today's log day
// file, and `rk query` on edges shows a rel='did' edge from the entry to the
// completed todo.
func TestTodayAct_DoneEmitsDidEntry(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-10")
	today := "2026-07-10"

	id := node.Mint()
	writeTodoFixture(t, vault, id, "open", today, "Ship the report.")

	_, stderr, err := runToday(t, vault, "act", id, "x")
	if err != nil {
		t.Fatalf("rk today act x: %v\nstderr: %s", err, stderr)
	}

	if _, statErr := os.Stat(dayLogPath(vault, today)); statErr != nil {
		t.Fatalf("log day file not created: %v", statErr)
	}
	entries := parseLogDayFile(t, vault, today)[1:]
	if len(entries) != 1 {
		t.Fatalf("want exactly 1 log entry, got %d", len(entries))
	}
	var foundDid bool
	for _, l := range entries[0].Links {
		if l.Rel == "did" && l.To == id {
			foundDid = true
		}
	}
	if !foundDid {
		t.Errorf("log entry missing Link{Rel:did, To:%s}: %+v", id, entries[0].Links)
	}

	resetCLIFlags()
	buildIndex(t, vault)
	stdoutQ, stderrQ, errQ := runQuery(t, vault, "SELECT src, dst FROM edges WHERE rel='did'")
	if errQ != nil {
		t.Fatalf("rk query did edges: %v\nstderr: %s", errQ, stderrQ)
	}
	rows := parseNDJSONMaps(t, stdoutQ)
	if len(rows) != 1 {
		t.Fatalf("want exactly 1 did edge, got %d: %v", len(rows), rows)
	}
	if rows[0]["dst"] != id {
		t.Errorf("edge dst = %v, want %q", rows[0]["dst"], id)
	}
}

// TestTodayAct_DoneOnRecurringReusesT6Path (TS-4.2): `act x` on a repeat:
// row behaves exactly like `rk todo done`'s existing recurrence branch —
// state stays open, scheduled advances, and a did-linked log entry is
// written. This proves rk today's x does not reimplement T6's cursor-advance
// logic with different behavior.
func TestTodayAct_DoneOnRecurringReusesT6Path(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-05")

	id := node.Mint()
	path, src := writeRecurringTodo(t, vault, id, "2026-07-05", "+7d", "Water plants")

	_, stderr, err := runToday(t, vault, "act", id, "x")
	if err != nil {
		t.Fatalf("rk today act x (recurring): %v\nstderr: %s", err, stderr)
	}

	want := strings.Replace(src, "scheduled: 2026-07-05", "scheduled: 2026-07-12", 1)
	got := mustReadFile(t, path)
	if got != want {
		t.Fatalf("recurring advance not span-local via rk today act x\n--- want ---\n%q\n--- got ---\n%q", want, got)
	}
	if !strings.Contains(got, "state: open") {
		t.Error("recurring rule's state must stay open, not flip to done")
	}

	if _, statErr := os.Stat(dayLogPath(vault, "2026-07-05")); statErr != nil {
		t.Fatalf("did-linked log day file not created: %v", statErr)
	}
	entries := parseLogDayFile(t, vault, "2026-07-05")[1:]
	if len(entries) != 1 {
		t.Fatalf("want exactly 1 log entry, got %d", len(entries))
	}
	var foundDid bool
	for _, l := range entries[0].Links {
		if l.Rel == "did" && l.To == id {
			foundDid = true
		}
	}
	if !foundDid {
		t.Errorf("recurring completion via rk today act x missing did-linked log entry: %+v", entries[0].Links)
	}
}

// TestTodayAct_DoneNoLogSuppressesEntry (TS-4.3): `act x --no-log` still
// flips state->done, but writes no log-entry/did edge (proposal's
// "toggleable" clause).
func TestTodayAct_DoneNoLogSuppressesEntry(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-10")
	today := "2026-07-10"

	id := node.Mint()
	path, src := writeTodoFixture(t, vault, id, "open", today, "Ship the report.")

	_, stderr, err := runToday(t, vault, "act", id, "x", "--no-log")
	if err != nil {
		t.Fatalf("rk today act x --no-log: %v\nstderr: %s", err, stderr)
	}

	want := strings.Replace(src, "state: open", "state: done", 1)
	got := mustReadFile(t, path)
	if got != want {
		t.Fatalf("state flip not span-local under --no-log\n--- want ---\n%q\n--- got ---\n%q", want, got)
	}

	if _, statErr := os.Stat(filepath.Join(vault, "log")); !os.IsNotExist(statErr) {
		t.Errorf("log/ dir must not be created under --no-log, stat err = %v", statErr)
	}
}

// TestTodayAct_StartAndCancelDoNotLog (TS-4.4): neither `i` nor `c` is a
// completion, so neither writes a log-entry/did edge (only `x` logs by
// default).
func TestTodayAct_StartAndCancelDoNotLog(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-10")
	today := "2026-07-10"

	startID := node.Mint()
	writeTodoFixture(t, vault, startID, "open", today, "Start me.")
	if _, stderr, err := runToday(t, vault, "act", startID, "i"); err != nil {
		t.Fatalf("rk today act i: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()

	cancelID := node.Mint()
	writeTodoFixture(t, vault, cancelID, "open", today, "Cancel me.")
	if _, stderr, err := runToday(t, vault, "act", cancelID, "c"); err != nil {
		t.Fatalf("rk today act c: %v\nstderr: %s", err, stderr)
	}

	if _, statErr := os.Stat(filepath.Join(vault, "log")); !os.IsNotExist(statErr) {
		t.Errorf("log/ dir must not be created by i or c, stat err = %v", statErr)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// AC-5 (split actuator: native vs. external) — TS-5.1..TS-5.3 + read-only
// marker on list
// ─────────────────────────────────────────────────────────────────────────────

// TestTodayAct_ExternalRowRejected (TS-5.1/EC-4): every actuation key
// against a synthetic work-ticket row is rejected with a non-zero error, and
// leaves EVERY file in the vault byte-identical (not just the target file).
func TestTodayAct_ExternalRowRejected(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-10")
	today := "2026-07-10"
	tomorrow := dayOffset(t, today, 1)

	extID := node.Mint()
	writeTestNode(t, vault, "work/"+extID+".md", extID, "work-ticket", "Fix the external ticket.",
		"state: open", "scheduled: "+today, "source: jira", "source-url: https://example.atlassian.net/browse/TICKET-1")

	attempts := [][]string{
		{"act", extID, "t"},
		{"act", extID, "d", tomorrow},
		{"act", extID, "D", tomorrow},
		{"act", extID, "p", "A"},
		{"act", extID, "x"},
		{"act", extID, "i"},
		{"act", extID, "c"},
	}
	for _, args := range attempts {
		key := args[2]
		t.Run(key, func(t *testing.T) {
			resetCLIFlags()
			before := snapshotVaultFiles(t, vault)
			_, stderr, err := runToday(t, vault, args...)
			if err == nil {
				t.Fatalf("expected a read-only rejection error for %v, got nil (stderr=%q)", args, stderr)
			}
			// The error must specifically say the row is read-only/external
			// (plan.md D: "row is read-only (external work ticket); use `rk
			// today open`") -- not just any unrelated failure. This also
			// guards against a false-green: today's still-registered legacy
			// journal-dump RunE happens to return SOME non-nil error too
			// (no journal exists for the sandboxed test date), which would
			// otherwise satisfy a bare "err != nil" check for the wrong
			// reason.
			msg := strings.ToLower(err.Error() + " " + stderr)
			if !strings.Contains(msg, "read-only") && !strings.Contains(msg, "read only") &&
				!strings.Contains(msg, "external") && !strings.Contains(msg, "work-ticket") &&
				!strings.Contains(msg, "work ticket") {
				t.Errorf("expected a read-only/external rejection message for %v, got err=%v stderr=%q", args, err, stderr)
			}
			after := snapshotVaultFiles(t, vault)
			if !vaultSnapshotsEqual(before, after) {
				t.Errorf("vault files mutated by a rejected external actuation %v\nbefore=%v\nafter=%v", args, before, after)
			}
		})
	}
}

// TestTodayOpen_ExternalRowExposesURL (TS-5.2): `rk today open <ref>` on a
// work-ticket row resolves to its source-url.
func TestTodayOpen_ExternalRowExposesURL(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-10")

	extID := node.Mint()
	url := "https://example.atlassian.net/browse/TICKET-42"
	writeTestNode(t, vault, "work/"+extID+".md", extID, "work-ticket", "Fix the external ticket.",
		"state: open", "source: jira", "source-url: "+url)

	stdout, stderr, err := runToday(t, vault, "open", extID, "--json")
	if err != nil {
		t.Fatalf("rk today open: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, url) {
		t.Errorf("rk today open output does not contain source URL %q, got: %q", url, stdout)
	}
}

// TestTodayAct_NativeRowUnaffectedBySplit (TS-5.3): in a mixed agenda (one
// native row + one external row), `x` on the native row actuates normally
// and the external row is left untouched — the split is per-row, not
// global.
func TestTodayAct_NativeRowUnaffectedBySplit(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-10")
	today := "2026-07-10"

	extID := node.Mint()
	extPath := filepath.Join(vault, "work", extID+".md")
	writeTestNode(t, vault, "work/"+extID+".md", extID, "work-ticket", "External ticket.",
		"state: open", "scheduled: "+today, "source: jira", "source-url: https://example.com/T-1")

	nativeID := node.Mint()
	nativePath, nativeSrc := writeTodoFixture(t, vault, nativeID, "open", today, "Ship it.")

	_, stderr, err := runToday(t, vault, "act", nativeID, "x", "--no-log")
	if err != nil {
		t.Fatalf("rk today act x (native, amid mixed agenda): %v\nstderr: %s", err, stderr)
	}

	want := strings.Replace(nativeSrc, "state: open", "state: done", 1)
	got := mustReadFile(t, nativePath)
	if got != want {
		t.Fatalf("native row not actuated span-locally amid a mixed agenda\n--- want ---\n%q\n--- got ---\n%q", want, got)
	}

	extRaw := mustReadFile(t, extPath)
	if !strings.Contains(extRaw, "state: open") {
		t.Errorf("external row's state unexpectedly changed by an actuation targeting a different (native) row: %q", extRaw)
	}
}

// TestToday_ListMarksExternalReadOnly: the agenda's --json output marks an
// external/work-ticket row with a read-only indication and its source, so a
// caller (or future TUI/agent) can render it distinctly.
func TestToday_ListMarksExternalReadOnly(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-10")
	today := "2026-07-10"

	extID := node.Mint()
	url := "https://example.com/TICKET-9"
	writeTestNode(t, vault, "work/"+extID+".md", extID, "work-ticket", "External ticket.",
		"state: open", "scheduled: "+today, "source: jira", "source-url: "+url)

	stdout, stderr, err := runToday(t, vault, "--json")
	if err != nil {
		t.Fatalf("rk today --json: %v\nstderr: %s", err, stderr)
	}
	items := decodeAgendaItems(t, stdout)
	var found map[string]any
	for _, it := range items {
		if it["id"] == extID {
			found = it
		}
	}
	if found == nil {
		t.Fatalf("external row %q missing from agenda: %v", extID, items)
	}
	if ro, _ := found["read_only"].(bool); !ro {
		t.Errorf("external row missing read_only=true: %v", found)
	}
	if src, _ := found["source"].(string); src != "jira" {
		t.Errorf("external row source = %v, want \"jira\": %v", found["source"], found)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// AC-6 (tested) / decision A — legacy journal dump replaced
// ─────────────────────────────────────────────────────────────────────────────

// TestToday_LegacyJournalDumpReplaced: bare `rk today` no longer dumps
// journal content — it emits the agenda envelope (via cmd.OutOrStdout(), the
// captured RootCmd writer), not the legacy today.go RunE's direct
// fmt.Print(journalContent) to the real process stdout.
func TestToday_LegacyJournalDumpReplaced(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-10")

	stdout, stderr, err := runToday(t, vault, "--json")
	if err != nil {
		t.Fatalf("rk today --json: %v\nstderr: %s", err, stderr)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Fatal("rk today --json produced no output on the captured RootCmd writer -- still bypassing cmd.OutOrStdout() via the legacy fmt.Print(journal content) path")
	}
	items := decodeAgendaItems(t, stdout)
	if len(items) != 0 {
		t.Errorf("expected an empty agenda for a vault with no todos, got %d items: %v", len(items), items)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Review-fix pins (reckon-liml review iteration): B5 external-state scope,
// --no-log on a recurring completion, and the Skipped signal on
// todayActResult. See ticket-work/reckon-liml/review.md issues 2/3/4.
// ─────────────────────────────────────────────────────────────────────────────

// TestToday_ExternalRowWithForeignStateSurfaces (review issue 2): plan.md B5
// scopes the state ∈ {open,in-progress} filter to NATIVE rows only. An
// external work-ticket row carrying a foreign/terminal state string (here
// "done", standing in for any non-native vocabulary a real feeder might
// emit) must still surface on the date predicate alone -- it must NOT be
// silently dropped the way a native todo with state=done would be.
func TestToday_ExternalRowWithForeignStateSurfaces(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-10")
	today := "2026-07-10"

	extID := node.Mint()
	writeTestNode(t, vault, "work/"+extID+".md", extID, "work-ticket", "External ticket with a foreign state.",
		"state: done", "scheduled: "+today, "source: jira", "source-url: https://example.com/T-2")

	stdout, stderr, err := runToday(t, vault, "--json")
	if err != nil {
		t.Fatalf("rk today --json: %v\nstderr: %s", err, stderr)
	}
	items := decodeAgendaItems(t, stdout)
	var found bool
	for _, it := range items {
		if it["id"] == extID {
			found = true
		}
	}
	if !found {
		t.Errorf("external row %q with foreign state %q must surface via the date predicate alone (plan.md B5): %v", extID, "done", items)
	}
}

// TestTodayAct_DoneNoLogSuppressesEntryOnRecurring (review issue 3): `act x
// --no-log` on a recurring (repeat:) row must suppress the did-entry log
// write, exactly like it already does for a plain (non-recurring) row
// (TestTodayAct_DoneNoLogSuppressesEntry) -- the flag's help text promises
// unconditional suppression. The T6 recurrence mechanics themselves (state
// stays open, scheduled advances by the repeater) must be unaffected.
func TestTodayAct_DoneNoLogSuppressesEntryOnRecurring(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-05")

	id := node.Mint()
	path, src := writeRecurringTodo(t, vault, id, "2026-07-05", "+7d", "Water plants")

	_, stderr, err := runToday(t, vault, "act", id, "x", "--no-log")
	if err != nil {
		t.Fatalf("rk today act x --no-log (recurring): %v\nstderr: %s", err, stderr)
	}

	want := strings.Replace(src, "scheduled: 2026-07-05", "scheduled: 2026-07-12", 1)
	got := mustReadFile(t, path)
	if got != want {
		t.Fatalf("recurring cursor advance under --no-log must still be span-local per T6\n--- want ---\n%q\n--- got ---\n%q", want, got)
	}
	if !strings.Contains(got, "state: open") {
		t.Error("recurring rule's state must stay open under --no-log, not flip to done")
	}

	if _, statErr := os.Stat(filepath.Join(vault, "log")); !os.IsNotExist(statErr) {
		t.Errorf("log/ dir must not be created by a recurring completion under --no-log, stat err = %v", statErr)
	}
}

// TestTodayAct_DoneOnRecurringStillLogsWithoutNoLog (review issue 3
// regression guard): without --no-log, `rk today act x` on a recurring row
// must still log unconditionally -- confirming the fix did not flip the
// default and did not touch `rk todo done`'s own always-logs recurrence
// contract (plan.md Q6). Duplicates the intent of
// TestTodayAct_DoneOnRecurringReusesT6Path but named to sit next to the new
// --no-log-on-recurring pin above for reviewer clarity.
func TestTodayAct_DoneOnRecurringStillLogsWithoutNoLog(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-05")

	id := node.Mint()
	writeRecurringTodo(t, vault, id, "2026-07-05", "+7d", "Water plants")

	_, stderr, err := runToday(t, vault, "act", id, "x")
	if err != nil {
		t.Fatalf("rk today act x (recurring, default logging): %v\nstderr: %s", err, stderr)
	}
	if _, statErr := os.Stat(dayLogPath(vault, "2026-07-05")); statErr != nil {
		t.Fatalf("did-linked log day file not created by default (no --no-log): %v", statErr)
	}
}

// TestTodayAct_DoneSkippedSignalInJSON (review issue 4): `act x` on an
// already-done todo is an idempotent no-op (completeDurableTodoNode's
// Skipped:true branch); todayActResult must propagate that signal (JSON
// "skipped" field + Pretty text), mirroring todoDoneResult's existing
// shape, instead of reporting State:"done" indistinguishably from a real
// completion.
func TestTodayAct_DoneSkippedSignalInJSON(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-10")
	today := "2026-07-10"

	id := node.Mint()
	writeTodoFixture(t, vault, id, "done", today, "Already done.")

	stdout, stderr, err := runToday(t, vault, "act", id, "x", "--json")
	if err != nil {
		t.Fatalf("rk today act x on an already-done todo: %v\nstderr: %s", err, stderr)
	}
	var res struct {
		Skipped bool   `json:"skipped"`
		State   string `json:"state"`
	}
	mustDecodeJSON(t, stdout, &res)
	if !res.Skipped {
		t.Errorf("todayActResult JSON missing skipped=true for an already-done todo: %q", stdout)
	}
	if res.State != "done" {
		t.Errorf("state = %q, want \"done\"", res.State)
	}

	resetCLIFlags()
	prettyOut, prettyErr, err2 := runToday(t, vault, "act", id, "x")
	if err2 != nil {
		t.Fatalf("rk today act x (pretty) on an already-done todo: %v\nstderr: %s", err2, prettyErr)
	}
	if !strings.Contains(strings.ToLower(prettyOut), "skip") {
		t.Errorf("pretty output should indicate the idempotent skip, got: %q", prettyOut)
	}
}
