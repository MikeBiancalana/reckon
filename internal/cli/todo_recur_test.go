// Package cli — TDD red tests for v1-T6 `rk todo done` recurrence
// (reckon-ar9m): org-style repeaters (+Nd/++Nd/.+Nd) advance a durable
// todo's `scheduled:` cursor via a span-local edit, write a `did::`-linked
// audit log entry, and (on multi-interval lateness) materialize exactly one
// ephemeral catch-up instance into todos/inbox.md.
//
// This file references todoNow (a package-level `func() time.Time` var the
// plan requires todo.go to add, mirroring the existing mintTodoULID seam)
// and the extended todoDoneResult/todoListItem fields below -- none of
// which exist in today's internal/cli/todo.go. Combined with
// internal/cli/recur_test.go's references to the not-yet-created recur.go
// (repeatKind/repeatSpec/parseRepeat/parseSchedDate/advanceSchedule), the
// whole `cli` package fails to COMPILE right now. That is the expected
// TDD-red state at this stage ("the package does not build"), mirroring
// todo_test.go's/add_test.go's own header convention for the
// immediately-preceding reckon-qiua/reckon-uv09 tickets.
//
// Precedent / harness reuse (do not redefine these -- they already live in
// this package): setupQueryVault, writeTestNode, resetCLIFlags, buildIndex,
// runQuery, parseNDJSONMaps (query_test.go); mustWriteFile, mustReadFile,
// isValidULID (adopt_test.go); mustDecodeJSON, runTodo (todo_test.go);
// parseLogDayFile (add_test.go) -- reused here to inspect the did-linked
// log entry's Author/Links without hand-rolling a second LogParser-driving
// helper.
//
// ─────────────────────────────────────────────────────────────────────────
// Pinned contract (plan.md "Modify: internal/cli/todo.go" / "Data Flow —
// doneRecurringTodo" / "Exact `did::` Marker Syntax"):
// ─────────────────────────────────────────────────────────────────────────
//
//	// New test seam (mirrors mintTodoULID), todo.go:
//	var todoNow = func() time.Time { return time.Now().UTC() }
//
//	// todoDoneResult gains (all omitempty; State stays "open" on this path):
//	Recurred     bool   `json:"recurred,omitempty"`
//	Scheduled    string `json:"scheduled,omitempty"`     // newly-advanced date
//	Repeat       string `json:"repeat,omitempty"`
//	DidEntryID   string `json:"did_entry_id,omitempty"`
//	DidEntryPath string `json:"did_entry_path,omitempty"`
//	Missed       int    `json:"missed,omitempty"`
//	Materialized string `json:"materialized,omitempty"`  // "todos/inbox.md#<line>"
//
//	// todoListItem gains:
//	Repeat string `json:"repeat,omitempty"` // durable only, sourced from props["repeat"]
//
//	// internal/node/logparser.go gains:
//	func RenderLogEntryWithDid(hhmm, author, ulid, didTarget, body string) string
//	// emits: "## HH:MM · author\nid:: <entry-ULID>\ndid:: <rule-ULID>\n<body>\n"
//	// and buildLogEntry appends Link{Rel:"did", To:<rule-ULID>} to the entry node.
//
//	// todoAddCmd gains --repeat (rejects with --ephemeral; requires --scheduled;
//	// parseRepeat-validates at add time). todoDoneCmd gains --author.
//
// A recurring rule's `state` is NEVER flipped to "done" (FIRM, plan.md
// "State-transition question") -- the cursor advance IS the completion
// signal, so State stays "open" and the rule stays in the default list
// (AC-3). Pile-up materializes exactly ONE ephemeral instance per
// completion event regardless of how many intervals were missed (FIRM,
// Gap 3), with body text
// `[[<rule-ULID>]] missed <N> occurrence(s) (was due <old>, repeat <cookie>)`.
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MikeBiancalana/reckon/internal/node"
)

// ─────────────────────────────────────────────────────────────────────────────
// Harness helpers (recurrence-specific; vault/flag/JSON/log-parsing plumbing
// is shared with query_test.go/todo_test.go/add_test.go).
// ─────────────────────────────────────────────────────────────────────────────

// pinTodoNow overrides the todoNow seam to a fixed UTC calendar date (no
// time-of-day component -- scheduled/completion dates are floating/date-only
// per the design's A#3d section) for the duration of the current test, and
// restores the previous value via t.Cleanup. Mirrors TestTodoAdd_NoClobberExistingFile's
// mintTodoULID-override pattern (todo_test.go).
func pinTodoNow(t *testing.T, date string) {
	t.Helper()
	tm, err := time.ParseInLocation("2006-01-02", date, time.UTC)
	if err != nil {
		t.Fatalf("pinTodoNow: parse %q: %v", date, err)
	}
	prev := todoNow
	todoNow = func() time.Time { return tm }
	t.Cleanup(func() { todoNow = prev })
}

// recurringTodoSrc renders the exact byte-for-byte source of a durable todo
// carrying scheduled/repeat props, hand-authored (not routed through `rk
// todo add --repeat`) so byte-preservation assertions have a known-exact
// "before" string to diff against -- mirrors the manual-fixture style
// TestTodoDone_DurableBytePreservation (todo_test.go) uses for the
// non-recurring case. extraFM lines (if any) are inserted between repeat:
// and the closing frontmatter fence.
func recurringTodoSrc(id, scheduled, repeat, body string, extraFM ...string) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString("id: " + id + "\n")
	sb.WriteString("type: todo\n")
	sb.WriteString("state: open\n")
	sb.WriteString("scheduled: " + scheduled + "\n")
	sb.WriteString("repeat: " + repeat + "\n")
	for _, line := range extraFM {
		sb.WriteString(line + "\n")
	}
	sb.WriteString("---\n")
	sb.WriteString(body + "\n")
	return sb.String()
}

// writeRecurringTodo writes recurringTodoSrc's output to todos/<id>.md and
// returns (absolute path, source string).
func writeRecurringTodo(t *testing.T, vault, id, scheduled, repeat, body string, extraFM ...string) (path, src string) {
	t.Helper()
	src = recurringTodoSrc(id, scheduled, repeat, body, extraFM...)
	path = filepath.Join(vault, "todos", id+".md")
	mustWriteFile(t, path, src)
	return path, src
}

// inboxPath is the absolute path of the shared ephemeral container.
func inboxPath(vault string) string { return filepath.Join(vault, "todos", "inbox.md") }

// ─────────────────────────────────────────────────────────────────────────────
// AC-1 / TS-1..TS-5 — repeater family advance, driven end-to-end through
// `rk todo done`.
// ─────────────────────────────────────────────────────────────────────────────

// TestTodoDone_Recurring_Fixed_Advances (TS-1): on-time +Nd completion
// advances scheduled via a span-local edit; State stays "open"; Recurred true.
func TestTodoDone_Recurring_Fixed_Advances(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-05")

	id := node.Mint()
	path, src := writeRecurringTodo(t, vault, id, "2026-07-05", "+7d", "Water plants")

	out, stderr, err := runTodo(t, vault, "done", id, "--json")
	if err != nil {
		t.Fatalf("rk todo done: %v\nstderr: %s", err, stderr)
	}
	var res todoDoneResult
	mustDecodeJSON(t, out, &res)

	if res.Skipped {
		t.Error("Skipped = true, want a real completion")
	}
	if !res.Recurred {
		t.Error("Recurred = false, want true")
	}
	if res.State != "open" {
		t.Errorf("State = %q, want open (a recurring rule's state is never flipped)", res.State)
	}
	if res.Scheduled != "2026-07-12" {
		t.Errorf("Scheduled = %q, want 2026-07-12", res.Scheduled)
	}
	if res.Repeat != "+7d" {
		t.Errorf("Repeat = %q, want +7d", res.Repeat)
	}
	if res.Missed != 0 {
		t.Errorf("Missed = %d, want 0", res.Missed)
	}

	want := strings.Replace(src, "scheduled: 2026-07-05", "scheduled: 2026-07-12", 1)
	if got := mustReadFile(t, path); got != want {
		t.Fatalf("advance not span-local\n--- want ---\n%q\n--- got ---\n%q", want, got)
	}
}

// TestTodoDone_Recurring_Fixed_LateOverdue_PilesUp (TS-2 + EC-9): +Nd stays
// overdue after a single hop (the documented non-catch-up quirk), and
// multi-interval lateness materializes one pile-up instance.
func TestTodoDone_Recurring_Fixed_LateOverdue_PilesUp(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-05")

	id := node.Mint()
	writeRecurringTodo(t, vault, id, "2026-06-01", "+7d", "Water plants")

	out, stderr, err := runTodo(t, vault, "done", id, "--json")
	if err != nil {
		t.Fatalf("rk todo done: %v\nstderr: %s", err, stderr)
	}
	var res todoDoneResult
	mustDecodeJSON(t, out, &res)

	if res.Scheduled != "2026-06-08" {
		t.Errorf("Scheduled = %q, want 2026-06-08 (single hop, still overdue by design)", res.Scheduled)
	}
	if res.Missed != 4 {
		t.Errorf("Missed = %d, want 4", res.Missed)
	}
	if res.Materialized == "" {
		t.Error("Materialized is empty, want a pile-up instance reference")
	}

	inboxRaw := mustReadFile(t, inboxPath(vault))
	wantSubstr := fmt.Sprintf("[[%s]] missed %d occurrence(s) (was due %s, repeat %s)", id, 4, "2026-06-01", "+7d")
	if !strings.Contains(inboxRaw, wantSubstr) {
		t.Errorf("inbox.md missing pile-up line %q, got: %q", wantSubstr, inboxRaw)
	}
}

// TestTodoDone_Recurring_Skip_SkipsToFuture_PilesUp (TS-3): ++Nd skips
// straight to the smallest future occurrence, never landing in the past.
func TestTodoDone_Recurring_Skip_SkipsToFuture_PilesUp(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-05")

	id := node.Mint()
	writeRecurringTodo(t, vault, id, "2026-06-01", "++7d", "Water plants")

	out, stderr, err := runTodo(t, vault, "done", id, "--json")
	if err != nil {
		t.Fatalf("rk todo done: %v\nstderr: %s", err, stderr)
	}
	var res todoDoneResult
	mustDecodeJSON(t, out, &res)

	if res.Scheduled != "2026-07-06" {
		t.Errorf("Scheduled = %q, want 2026-07-06 (smallest old+7k strictly after today)", res.Scheduled)
	}
	if res.Missed != 4 {
		t.Errorf("Missed = %d, want 4", res.Missed)
	}
	if res.Materialized == "" {
		t.Error("Materialized is empty, want a pile-up instance reference")
	}
}

// TestTodoDone_Recurring_FromCompletion_EarlyAndLate (TS-4): .+Nd always
// anchors to the actual completion day, ignoring the old cursor entirely.
func TestTodoDone_Recurring_FromCompletion_EarlyAndLate(t *testing.T) {
	t.Run("early", func(t *testing.T) {
		vault, _ := setupQueryVault(t)
		t.Cleanup(resetCLIFlags)
		pinTodoNow(t, "2026-07-05")

		id := node.Mint()
		writeRecurringTodo(t, vault, id, "2026-07-10", ".+3d", "Review budget")

		out, stderr, err := runTodo(t, vault, "done", id, "--json")
		if err != nil {
			t.Fatalf("rk todo done: %v\nstderr: %s", err, stderr)
		}
		var res todoDoneResult
		mustDecodeJSON(t, out, &res)

		if res.Scheduled != "2026-07-08" {
			t.Errorf("Scheduled = %q, want 2026-07-08 (today+3, ignoring old=2026-07-10)", res.Scheduled)
		}
		if res.Missed != 0 {
			t.Errorf("Missed = %d, want 0 (early completion never piles up)", res.Missed)
		}
		if res.Materialized != "" {
			t.Errorf("Materialized = %q, want empty", res.Materialized)
		}
	})

	t.Run("late", func(t *testing.T) {
		vault, _ := setupQueryVault(t)
		t.Cleanup(resetCLIFlags)
		pinTodoNow(t, "2026-07-20")

		id := node.Mint()
		writeRecurringTodo(t, vault, id, "2026-07-01", ".+3d", "Review budget")

		out, stderr, err := runTodo(t, vault, "done", id, "--json")
		if err != nil {
			t.Fatalf("rk todo done: %v\nstderr: %s", err, stderr)
		}
		var res todoDoneResult
		mustDecodeJSON(t, out, &res)

		if res.Scheduled != "2026-07-23" {
			t.Errorf("Scheduled = %q, want 2026-07-23 (today+3, ignoring old=2026-07-01)", res.Scheduled)
		}
		if res.Missed != 6 {
			t.Errorf("Missed = %d, want 6 (19 days late / 3-day interval, floor)", res.Missed)
		}
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// AC-2 / AC-6 — did entry written as audit (TS-6/TS-7).
// ─────────────────────────────────────────────────────────────────────────────

// TestTodoDone_Recurring_DidEntryWritten (TS-6): completing a recurring
// rule additionally writes a did::-linked log entry, non-empty author,
// resolving to a real `did` edge after indexing.
func TestTodoDone_Recurring_DidEntryWritten(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-05")

	id := node.Mint()
	writeRecurringTodo(t, vault, id, "2026-07-05", "+7d", "Water plants")

	out, stderr, err := runTodo(t, vault, "done", id, "--author", "agent-x", "--json")
	if err != nil {
		t.Fatalf("rk todo done --author: %v\nstderr: %s", err, stderr)
	}
	var res todoDoneResult
	mustDecodeJSON(t, out, &res)

	if !isValidULID(res.DidEntryID) {
		t.Errorf("DidEntryID %q is not a valid ULID", res.DidEntryID)
	}
	wantLogPath := "log/2026-07-05.md"
	if res.DidEntryPath != wantLogPath {
		t.Errorf("DidEntryPath = %q, want %q", res.DidEntryPath, wantLogPath)
	}

	if _, err := os.Stat(filepath.Join(vault, "log", "2026-07-05.md")); err != nil {
		t.Fatalf("log day file not created: %v", err)
	}

	entries := parseLogDayFile(t, vault, "2026-07-05")[1:]
	if len(entries) != 1 {
		t.Fatalf("want exactly 1 log entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Author != "agent-x" {
		t.Errorf("entry Author = %q, want agent-x", e.Author)
	}
	if e.ULID != res.DidEntryID {
		t.Errorf("entry ULID %q != reported DidEntryID %q", e.ULID, res.DidEntryID)
	}
	var foundDid bool
	for _, l := range e.Links {
		if l.Rel == "did" && l.To == id {
			foundDid = true
		}
	}
	if !foundDid {
		t.Errorf("entry missing Link{Rel:did, To:%s}: %+v", id, e.Links)
	}

	buildIndex(t, vault)
	stdout, stderr, err := runQuery(t, vault, "SELECT src, dst FROM edges WHERE rel='did'")
	if err != nil {
		t.Fatalf("rk query did edges: %v\nstderr: %s", err, stderr)
	}
	rows := parseNDJSONMaps(t, stdout)
	if len(rows) != 1 {
		t.Fatalf("want exactly 1 did edge, got %d: %v", len(rows), rows)
	}
	if rows[0]["src"] != res.DidEntryID {
		t.Errorf("edge src = %v, want %q", rows[0]["src"], res.DidEntryID)
	}
	if rows[0]["dst"] != id {
		t.Errorf("edge dst = %v, want %q", rows[0]["dst"], id)
	}
}

// TestTodoDone_NonRecurring_NoDidEntry (TS-7/AC-6): the did-log-write is
// strictly additive to the recurrence branch -- a plain done never writes
// a log entry.
func TestTodoDone_NonRecurring_NoDidEntry(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	id := node.Mint()
	writeTestNode(t, vault, "todos/"+id+".md", id, "todo", "Ship v1", "state: open")

	if _, stderr, err := runTodo(t, vault, "done", id); err != nil {
		t.Fatalf("rk todo done: %v\nstderr: %s", err, stderr)
	}

	n, err := node.Parse([]byte(mustReadFile(t, filepath.Join(vault, "todos", id+".md"))))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if n.Props["state"] != "done" {
		t.Errorf("Props[state] = %q, want done", n.Props["state"])
	}

	if _, err := os.Stat(filepath.Join(vault, "log")); !os.IsNotExist(err) {
		t.Errorf("log/ dir must not be created for a non-recurring done, stat err = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// AC-3 — current occurrence surfaced (TS-8).
// ─────────────────────────────────────────────────────────────────────────────

// TestTodoDone_Recurring_StaysInDefaultList (TS-8): completing one cycle
// keeps the rule in the default (no --all) list, showing the new scheduled
// date and its repeat cookie.
func TestTodoDone_Recurring_StaysInDefaultList(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-05")

	id := node.Mint()
	writeRecurringTodo(t, vault, id, "2026-07-05", "+7d", "Water plants")

	if _, stderr, err := runTodo(t, vault, "done", id); err != nil {
		t.Fatalf("rk todo done: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()

	out, stderr, err := runTodo(t, vault, "list", "--json")
	if err != nil {
		t.Fatalf("rk todo list: %v\nstderr: %s", err, stderr)
	}
	var res todoListResult
	mustDecodeJSON(t, out, &res)

	var found *todoListItem
	for i := range res.Items {
		if res.Items[i].ID == id {
			found = &res.Items[i]
		}
	}
	if found == nil {
		t.Fatalf("recurring rule missing from default list: %+v", res.Items)
	}
	if found.State != "open" {
		t.Errorf("State = %q, want open", found.State)
	}
	if found.Scheduled != "2026-07-12" {
		t.Errorf("Scheduled = %q, want the advanced date 2026-07-12", found.Scheduled)
	}
	if found.Repeat != "+7d" {
		t.Errorf("Repeat = %q, want +7d", found.Repeat)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// AC-4 — pile-up materializes an ephemeral instance (TS-9/TS-10).
// ─────────────────────────────────────────────────────────────────────────────

// TestTodoDone_Recurring_PileUpMaterializesOne (TS-9): exactly one new
// ephemeral instance appears on a multi-interval-late completion; a second
// completion with no further lateness adds none.
func TestTodoDone_Recurring_PileUpMaterializesOne(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-05")

	id := node.Mint()
	writeRecurringTodo(t, vault, id, "2026-06-01", "++7d", "Water plants")

	out1, stderr, err := runTodo(t, vault, "done", id, "--json")
	if err != nil {
		t.Fatalf("first rk todo done: %v\nstderr: %s", err, stderr)
	}
	var res1 todoDoneResult
	mustDecodeJSON(t, out1, &res1)
	if res1.Missed != 4 || res1.Materialized == "" {
		t.Fatalf("first completion: want a pile-up materialization, got Missed=%d Materialized=%q", res1.Missed, res1.Materialized)
	}
	if res1.Scheduled != "2026-07-06" {
		t.Fatalf("first completion: Scheduled = %q, want 2026-07-06", res1.Scheduled)
	}

	before := mustReadFile(t, inboxPath(vault))
	resetCLIFlags()

	// Second completion: scheduled is now 2026-07-06 (future relative to
	// today=2026-07-05), so this is early/on-time -- no further lateness.
	out2, stderr, err := runTodo(t, vault, "done", id, "--json")
	if err != nil {
		t.Fatalf("second rk todo done: %v\nstderr: %s", err, stderr)
	}
	var res2 todoDoneResult
	mustDecodeJSON(t, out2, &res2)
	if res2.Missed != 0 {
		t.Errorf("second completion: Missed = %d, want 0", res2.Missed)
	}
	if res2.Materialized != "" {
		t.Errorf("second completion: Materialized = %q, want empty (no further pile-up)", res2.Materialized)
	}

	after := mustReadFile(t, inboxPath(vault))
	if after != before {
		t.Errorf("inbox.md changed on the second (non-late) completion\n--- before ---\n%q\n--- after ---\n%q", before, after)
	}
}

// TestTodoDone_Recurring_NoPileUpWhenNotLate (TS-10): on-time, early, and
// single-interval-late completions never create an ephemeral instance.
func TestTodoDone_Recurring_NoPileUpWhenNotLate(t *testing.T) {
	cases := []struct {
		name      string
		scheduled string
		repeat    string
		today     string
	}{
		{"on_time", "2026-07-05", "+7d", "2026-07-05"},
		{"early", "2026-07-10", ".+3d", "2026-07-05"},
		{"single_late", "2026-07-01", "+7d", "2026-07-05"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vault, _ := setupQueryVault(t)
			t.Cleanup(resetCLIFlags)
			pinTodoNow(t, tc.today)

			id := node.Mint()
			writeRecurringTodo(t, vault, id, tc.scheduled, tc.repeat, "Some recurring task")

			out, stderr, err := runTodo(t, vault, "done", id, "--json")
			if err != nil {
				t.Fatalf("rk todo done: %v\nstderr: %s", err, stderr)
			}
			var res todoDoneResult
			mustDecodeJSON(t, out, &res)
			if res.Missed != 0 {
				t.Errorf("Missed = %d, want 0", res.Missed)
			}
			if res.Materialized != "" {
				t.Errorf("Materialized = %q, want empty", res.Materialized)
			}
			if _, err := os.Stat(inboxPath(vault)); !os.IsNotExist(err) {
				t.Errorf("todos/inbox.md must not be created when nothing piled up, stat err = %v", err)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// AC-5 — cursor survives index rebuild (TS-11/TS-12/TS-13).
// ─────────────────────────────────────────────────────────────────────────────

// TestTodoDone_Recurring_CursorSurvivesRebuild (TS-11): blowing away and
// rebuilding the index reproduces the advanced scheduled value exactly,
// because it lives in vault text, never the index.
func TestTodoDone_Recurring_CursorSurvivesRebuild(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-05")

	id := node.Mint()
	writeRecurringTodo(t, vault, id, "2026-07-05", "+7d", "Water plants")

	if _, stderr, err := runTodo(t, vault, "done", id); err != nil {
		t.Fatalf("rk todo done: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()

	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault, fmt.Sprintf(
		"SELECT value FROM node_props WHERE key='scheduled' AND id='%s'", id))
	if err != nil {
		t.Fatalf("rk query scheduled: %v\nstderr: %s", err, stderr)
	}
	rows := parseNDJSONMaps(t, stdout)
	if len(rows) != 1 {
		t.Fatalf("want exactly 1 scheduled prop row, got %d: %v", len(rows), rows)
	}
	if rows[0]["value"] != "2026-07-12" {
		t.Errorf("scheduled value after rebuild = %v, want 2026-07-12", rows[0]["value"])
	}
}

// TestTodoDone_Recurring_RepeatedRebuildsIdempotent (TS-12/EC-10): three
// rebuilds in a row, with no intervening completion, report the identical
// scheduled value every time.
func TestTodoDone_Recurring_RepeatedRebuildsIdempotent(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-05")

	id := node.Mint()
	writeRecurringTodo(t, vault, id, "2026-07-05", "+7d", "Water plants")

	if _, stderr, err := runTodo(t, vault, "done", id); err != nil {
		t.Fatalf("rk todo done: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()

	for i := 0; i < 3; i++ {
		buildIndex(t, vault)
		stdout, stderr, err := runQuery(t, vault, fmt.Sprintf(
			"SELECT value FROM node_props WHERE key='scheduled' AND id='%s'", id))
		if err != nil {
			t.Fatalf("rebuild %d: rk query scheduled: %v\nstderr: %s", i, err, stderr)
		}
		rows := parseNDJSONMaps(t, stdout)
		if len(rows) != 1 || rows[0]["value"] != "2026-07-12" {
			t.Fatalf("rebuild %d: scheduled = %v, want exactly one row with 2026-07-12", i, rows)
		}
	}
}

// TestTodoDone_Recurring_RebuildBetweenCompletions (TS-13/EC-11): an index
// rebuild sandwiched between two completions has zero effect on the
// text-truth cursor -- advance #2 is computed off advance #1's on-disk
// result, not any cached/reset state.
func TestTodoDone_Recurring_RebuildBetweenCompletions(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-05")

	id := node.Mint()
	path, _ := writeRecurringTodo(t, vault, id, "2026-07-05", "+7d", "Water plants")

	if _, stderr, err := runTodo(t, vault, "done", id); err != nil {
		t.Fatalf("first rk todo done: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()

	n1, err := node.Parse([]byte(mustReadFile(t, path)))
	if err != nil || n1.Props["scheduled"] != "2026-07-12" {
		t.Fatalf("after advance #1: scheduled = %q, err=%v, want 2026-07-12", n1.Props["scheduled"], err)
	}

	buildIndex(t, vault)

	pinTodoNow(t, "2026-07-12")
	if _, stderr, err := runTodo(t, vault, "done", id); err != nil {
		t.Fatalf("second rk todo done: %v\nstderr: %s", err, stderr)
	}

	n2, err := node.Parse([]byte(mustReadFile(t, path)))
	if err != nil {
		t.Fatalf("parse after advance #2: %v", err)
	}
	if n2.Props["scheduled"] != "2026-07-19" {
		t.Errorf("after advance #2: scheduled = %q, want 2026-07-19 (computed from advance #1's on-disk result)", n2.Props["scheduled"])
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// AC-6 — backward compatibility (TS-14).
// ─────────────────────────────────────────────────────────────────────────────

// TestTodoDone_NonRecurring_Unchanged (TS-14/EC-5): a todo with no repeat:
// prop behaves byte-for-byte like T5's plain done -- no recurrence code
// path is exercised at all.
func TestTodoDone_NonRecurring_Unchanged(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	id := node.Mint()
	src := "---\n" +
		"id: " + id + "\n" +
		"type: todo\n" +
		"state: open\n" +
		"x-obsidian-extra: keep-me\n" +
		"---\n" +
		"Ship the thing.\n\n" +
		"```go\n" +
		"fmt.Println(\"hi\")\n" +
		"```\n"
	path := filepath.Join(vault, "todos", id+".md")
	mustWriteFile(t, path, src)

	out, stderr, err := runTodo(t, vault, "done", id, "--json")
	if err != nil {
		t.Fatalf("rk todo done: %v\nstderr: %s", err, stderr)
	}
	var res todoDoneResult
	mustDecodeJSON(t, out, &res)
	if res.Recurred {
		t.Error("Recurred = true for a todo with no repeat: prop")
	}
	if res.Scheduled != "" || res.Repeat != "" || res.DidEntryID != "" || res.Materialized != "" {
		t.Errorf("recurrence fields leaked on a non-recurring done: %+v", res)
	}

	want := strings.Replace(src, "state: open", "state: done", 1)
	if got := mustReadFile(t, path); got != want {
		t.Fatalf("non-recurring done disturbed bytes beyond the state field\n--- want ---\n%q\n--- got ---\n%q", want, got)
	}

	if _, err := os.Stat(filepath.Join(vault, "log")); !os.IsNotExist(err) {
		t.Errorf("log/ dir must not be created for a non-recurring done, stat err = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Malformed-input edge cases (EC-2/3/4) — TS-15/TS-16/TS-17.
// ─────────────────────────────────────────────────────────────────────────────

// TestTodoDone_Recurring_MissingScheduled_Errors (TS-15/EC-2): repeat: set
// but scheduled: entirely absent errors, file untouched.
func TestTodoDone_Recurring_MissingScheduled_Errors(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	id := node.Mint()
	src := "---\nid: " + id + "\ntype: todo\nstate: open\nrepeat: +7d\n---\nWater plants.\n"
	path := filepath.Join(vault, "todos", id+".md")
	mustWriteFile(t, path, src)

	if _, _, err := runTodo(t, vault, "done", id); err == nil {
		t.Fatal("expected an error when repeat: is set with no scheduled:, got nil")
	}
	if got := mustReadFile(t, path); got != src {
		t.Fatalf("file modified despite the error\n--- want ---\n%q\n--- got ---\n%q", src, got)
	}
}

// TestTodoDone_Recurring_MalformedRepeat_Errors (TS-16/EC-4): an
// unrecognized repeater cookie errors, file untouched.
func TestTodoDone_Recurring_MalformedRepeat_Errors(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	id := node.Mint()
	_, src := writeRecurringTodo(t, vault, id, "2026-07-05", "weekly", "Water plants")
	path := filepath.Join(vault, "todos", id+".md")

	if _, _, err := runTodo(t, vault, "done", id); err == nil {
		t.Fatal("expected an error for a malformed repeat: cookie, got nil")
	}
	if got := mustReadFile(t, path); got != src {
		t.Fatalf("file modified despite the error\n--- want ---\n%q\n--- got ---\n%q", src, got)
	}
}

// TestTodoDone_Recurring_MalformedScheduled_Errors (TS-17/EC-3): a
// non-YYYY-MM-DD scheduled: value errors, file untouched.
func TestTodoDone_Recurring_MalformedScheduled_Errors(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	id := node.Mint()
	_, src := writeRecurringTodo(t, vault, id, "not-a-date", "+7d", "Water plants")
	path := filepath.Join(vault, "todos", id+".md")

	if _, _, err := runTodo(t, vault, "done", id); err == nil {
		t.Fatal("expected an error for a malformed scheduled: value, got nil")
	}
	if got := mustReadFile(t, path); got != src {
		t.Fatalf("file modified despite the error\n--- want ---\n%q\n--- got ---\n%q", src, got)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// EC-13 — double completion (no idempotency signal for a recurring rule).
// ─────────────────────────────────────────────────────────────────────────────

// TestTodoDone_Recurring_DoubleCompletionAdvancesAgain (EC-13): since state
// never becomes terminal for a recurring rule, a second `done` call is a
// fresh completion event and advances again (no idempotent skip).
func TestTodoDone_Recurring_DoubleCompletionAdvancesAgain(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-05")

	id := node.Mint()
	path, _ := writeRecurringTodo(t, vault, id, "2026-07-05", "+7d", "Water plants")

	out1, stderr, err := runTodo(t, vault, "done", id, "--json")
	if err != nil {
		t.Fatalf("first rk todo done: %v\nstderr: %s", err, stderr)
	}
	var res1 todoDoneResult
	mustDecodeJSON(t, out1, &res1)
	if res1.Skipped {
		t.Error("first completion: Skipped = true, want false")
	}
	if res1.Scheduled != "2026-07-12" {
		t.Fatalf("first completion: Scheduled = %q, want 2026-07-12", res1.Scheduled)
	}
	resetCLIFlags()

	out2, stderr, err := runTodo(t, vault, "done", id, "--json")
	if err != nil {
		t.Fatalf("second rk todo done: %v\nstderr: %s", err, stderr)
	}
	var res2 todoDoneResult
	mustDecodeJSON(t, out2, &res2)
	if res2.Skipped {
		t.Error("second completion: Skipped = true, want false (no idempotency for a recurring rule)")
	}
	if !res2.Recurred {
		t.Error("second completion: Recurred = false, want true")
	}
	// today (2026-07-05) < old (2026-07-12): early completion, fixed family
	// unaffected by today -> old+7.
	if res2.Scheduled != "2026-07-19" {
		t.Errorf("second completion: Scheduled = %q, want 2026-07-19", res2.Scheduled)
	}

	n, err := node.Parse([]byte(mustReadFile(t, path)))
	if err != nil || n.Props["scheduled"] != "2026-07-19" {
		t.Errorf("file scheduled = %q, err=%v, want 2026-07-19", n.Props["scheduled"], err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// EC-12 — byte preservation with hand-added extra frontmatter/body.
// ─────────────────────────────────────────────────────────────────────────────

// TestTodoDone_Recurring_ByteExtraContentPreserved (EC-12): only the
// scheduled: span changes; hand-added extra frontmatter, blockquotes, and
// fenced code are untouched.
func TestTodoDone_Recurring_ByteExtraContentPreserved(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-05")

	id := node.Mint()
	body := "> a blockquote\n\n```text\nfenced\n```\n"
	path, src := writeRecurringTodo(t, vault, id, "2026-07-05", "+7d", body, "x-obsidian-extra: keep-me")

	if _, stderr, err := runTodo(t, vault, "done", id); err != nil {
		t.Fatalf("rk todo done: %v\nstderr: %s", err, stderr)
	}

	want := strings.Replace(src, "scheduled: 2026-07-05", "scheduled: 2026-07-12", 1)
	got := mustReadFile(t, path)
	if got != want {
		t.Fatalf("edit disturbed bytes beyond the scheduled field\n--- want ---\n%q\n--- got ---\n%q", want, got)
	}
	if !strings.Contains(got, "x-obsidian-extra: keep-me") {
		t.Error("hand-added extra frontmatter field lost")
	}
	if !strings.Contains(got, "state: open") {
		t.Error("state span disturbed -- must stay open for a recurring rule")
	}
	if !strings.Contains(got, "repeat: +7d") {
		t.Error("repeat span disturbed")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// EC-14 — coexists with --depends edges.
// ─────────────────────────────────────────────────────────────────────────────

// TestTodoDone_Recurring_CoexistsWithDepends (EC-14): depends-on edges and
// the recurrence cursor are orthogonal -- neither clobbers the other's
// frontmatter span.
func TestTodoDone_Recurring_CoexistsWithDepends(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-05")

	aID := node.Mint()
	writeTestNode(t, vault, "todos/"+aID+".md", aID, "todo", "Existing blocker", "state: open")

	id := node.Mint()
	path, src := writeRecurringTodo(t, vault, id, "2026-07-05", "+7d", "Blocked recurring task",
		"depends-on: [["+aID+"]]")

	if _, stderr, err := runTodo(t, vault, "done", id); err != nil {
		t.Fatalf("rk todo done: %v\nstderr: %s", err, stderr)
	}

	want := strings.Replace(src, "scheduled: 2026-07-05", "scheduled: 2026-07-12", 1)
	got := mustReadFile(t, path)
	if got != want {
		t.Fatalf("edit disturbed bytes beyond the scheduled field\n--- want ---\n%q\n--- got ---\n%q", want, got)
	}

	n, err := node.Parse([]byte(got))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var dep *node.Link
	for i := range n.Links {
		if n.Links[i].Rel == "depends-on" {
			dep = &n.Links[i]
		}
	}
	if dep == nil || dep.To != aID {
		t.Errorf("depends-on link missing/wrong after recurrence advance: %+v (links=%v)", dep, n.Links)
	}
	if n.Props["scheduled"] != "2026-07-12" {
		t.Errorf("scheduled = %q, want 2026-07-12", n.Props["scheduled"])
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// EC-1/EC-2/EC-4 at add-time.
// ─────────────────────────────────────────────────────────────────────────────

// TestTodoAdd_Repeat_RejectsEphemeralAndRequiresScheduled covers `rk todo
// add --repeat`'s add-time validation: rejected alongside --ephemeral
// (EC-1, recurrence is durable-only), rejected without --scheduled (EC-2,
// a repeater with no anchor cannot advance), rejected when malformed
// (EC-4), and accepted (positive control) when well-formed with --scheduled.
func TestTodoAdd_Repeat_RejectsEphemeralAndRequiresScheduled(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	t.Run("ephemeral_and_repeat_rejected", func(t *testing.T) {
		resetCLIFlags()
		if _, stderr, err := runTodo(t, vault, "add", "x", "--ephemeral", "--repeat", "+7d"); err == nil {
			t.Fatalf("expected a usage error for --ephemeral + --repeat, got nil (stderr=%q)", stderr)
		}
	})

	t.Run("repeat_without_scheduled_rejected", func(t *testing.T) {
		resetCLIFlags()
		if _, stderr, err := runTodo(t, vault, "add", "x", "--repeat", "+7d"); err == nil {
			t.Fatalf("expected a usage error for --repeat without --scheduled, got nil (stderr=%q)", stderr)
		}
	})

	t.Run("malformed_repeat_rejected", func(t *testing.T) {
		resetCLIFlags()
		if _, stderr, err := runTodo(t, vault, "add", "x", "--scheduled", "2026-07-05", "--repeat", "weekly"); err == nil {
			t.Fatalf("expected a usage error for a malformed --repeat cookie, got nil (stderr=%q)", stderr)
		}
	})

	t.Run("valid_repeat_accepted", func(t *testing.T) {
		resetCLIFlags()
		out, stderr, err := runTodo(t, vault, "add", "Water plants",
			"--scheduled", "2026-07-05", "--repeat", "+7d", "--json")
		if err != nil {
			t.Fatalf("rk todo add --scheduled --repeat: %v\nstderr: %s", err, stderr)
		}
		var res todoAddResult
		mustDecodeJSON(t, out, &res)
		if !isValidULID(res.ID) {
			t.Errorf("ID %q is not a valid ULID", res.ID)
		}

		n, err := node.Parse([]byte(mustReadFile(t, filepath.Join(vault, res.Path))))
		if err != nil {
			t.Fatalf("parse created file: %v", err)
		}
		if n.Props["repeat"] != "+7d" {
			t.Errorf("Props[repeat] = %q, want +7d", n.Props["repeat"])
		}
		if n.Props["scheduled"] != "2026-07-05" {
			t.Errorf("Props[scheduled] = %q, want 2026-07-05", n.Props["scheduled"])
		}
	})
}
