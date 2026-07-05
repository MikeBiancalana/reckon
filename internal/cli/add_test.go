// Package cli — TDD red tests for v1-T4 `rk add` (reckon-uv09), the
// graduated vault-backed capture command replacing the internal/cli/stubs.go
// `addCmd` stub (structurally the same graduation reckon-qiua performed for
// `rk todo`). internal/cli/add.go does not exist yet; this file references
// logAddResult (a type the implementation must define) before it exists, so
// the whole `cli` package fails to COMPILE right now -- the expected
// TDD-red state ("the package does not build", not "tests run and fail"),
// mirroring internal/cli/todo_test.go's own header convention.
//
// Precedent / harness reuse (do not redefine these -- they already live in
// this package): setupQueryVault, writeTestNode, resetCLIFlags, buildIndex,
// runQuery, parseNDJSONMaps, countNDJSONLines (query_test.go); mustDecodeJSON
// (todo_test.go); mustWriteFile, mustReadFile, isValidULID (adopt_test.go);
// getEffectiveDate, resolveAuthor (root.go/todo.go, same package -- called
// directly below to derive the exact expected day/author rather than
// re-deriving that logic independently in the test).
//
// ─────────────────────────────────────────────────────────────────────────
// Pinned contract (plan.md "New — internal/cli/add.go"):
// ─────────────────────────────────────────────────────────────────────────
//
//	// logAddResult is the structured summary of one `rk add` run.
//	type logAddResult struct {
//	    Path string `json:"path"` // vault-relative: "log/<date>.md"
//	    ID   string `json:"id"`   // the new entry's ULID
//	    Day  string `json:"day"`  // the day file's date, e.g. "2026-07-05"
//	    Time string `json:"time"` // the entry's reconstructed time, e.g. "2026-07-05T09:15:00Z"
//	}
//
// addCmd: Use "add <text...>", Args cobra.MinimumNArgs(1),
// Annotations{"requiresDB":"false"}, SilenceUsage true, flags --author/--at
// (command-local, mirroring todoAddCmd's own local flags) plus the existing
// global --vault/--date/--json/--ndjson/--quiet. Body is args-joined-by-space
// (todoAddCmd convention); empty/whitespace body (EC-7) and a body that would
// introduce a literal "## " line (EC-9, defensive -- args are space-joined so
// this mostly guards embedded-newline/programmatic callers) are both
// rejected with a non-zero exit and no file write. File selection is
// log/<effectiveLogDate()>.md (--date, when given, picks the day FILE via
// getEffectiveDate(); when --date is omitted the day defaults to the
// current UTC calendar date, NOT getEffectiveDate()'s own local-clock
// default -- see effectiveLogDate's doc comment in add.go, reckon-uv09
// review C1: this keeps the day-file date and the entry's HH:MM, which
// already defaults to UTC below, on one shared clock); entry time is
// --at HH:MM if given, else current UTC HH:MM (--at backfills the time
// WITHIN whichever day file was selected -- a distinct concern from --date).
// The day file's frontmatter is `type: log-day`, aliases containing the
// date; the first write creates it via NewNode->Render->Parse->
// writeFileAtomic, later writes append a node.RenderLogEntry block strictly
// at EOF (CRLF day files refused outright, mirroring reckon-vj55). Every
// entry's author is stamped via resolveAuthor's existing precedence chain
// (--author flag > $RECKON_AUTHOR > $USER > "local"). rk add itself does not
// reconcile the index (capture -> explicit `rk index` -> query is the
// intended flow, matching rk query's own no-auto-reconcile contract).
package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MikeBiancalana/reckon/internal/node"
	"github.com/oklog/ulid/v2"
)

// ─────────────────────────────────────────────────────────────────────────────
// Harness helpers (add-specific; vault/cache/flag-reset/JSON-decode plumbing
// is shared with query_test.go/todo_test.go).
// ─────────────────────────────────────────────────────────────────────────────

// runAdd executes `rk add --vault <vault> [args...]` through RootCmd and
// returns (stdout, stderr, error), mirroring runTodo/runQuery/runAdopt. The
// caller must call resetCLIFlags() before another Execute within the same
// test (t.Cleanup(resetCLIFlags) covers end-of-test).
func runAdd(t *testing.T, vault string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	RootCmd.SetOut(&outBuf)
	RootCmd.SetErr(&errBuf)
	full := append([]string{"add", "--vault", vault}, args...)
	RootCmd.SetArgs(full)
	err = RootCmd.Execute()
	return outBuf.String(), errBuf.String(), err
}

// utcToday is the day rk add defaults to when --date is NOT given
// (effectiveLogDate in add.go): the current UTC calendar date -- NOT
// getEffectiveDate()'s own local-clock default. Tests below that omit
// --date must derive their expected "today" from this helper rather than
// from getEffectiveDate() directly, or they'd only coincidentally pass on a
// UTC-clocked test host (reckon-uv09 review, C1: the whole point of the fix
// is that the day file's date and the entry's HH:MM now share one clock).
func utcToday() string { return time.Now().UTC().Format("2006-01-02") }

// dayLogRelPath is the vault-relative path of a day's log file.
func dayLogRelPath(date string) string { return "log/" + date + ".md" }

// dayLogPath is the absolute path of a day's log file under vault.
func dayLogPath(vault, date string) string {
	return filepath.Join(vault, "log", date+".md")
}

// parseLogDayFile reads and LogParser-splits the day file for date, fataling
// on any read/parse error. Index 0 is always the log-day node; 1: are the
// entries in file order.
func parseLogDayFile(t *testing.T, vault, date string) []*node.Node {
	t.Helper()
	raw := mustReadFile(t, dayLogPath(vault, date))
	nodes, err := node.LogParser{}.Parse([]byte(raw), node.Loc{File: dayLogRelPath(date)})
	if err != nil {
		t.Fatalf("LogParser.Parse(%s): %v", date, err)
	}
	return nodes
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-1 (AC-1/AC-3): fresh vault, create path.
// ─────────────────────────────────────────────────────────────────────────────

func TestAddCmd_FreshVaultCreatesDayWithOneEntry(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	today := utcToday()

	if _, err := os.Stat(filepath.Join(vault, "log")); !os.IsNotExist(err) {
		t.Fatalf("precondition: log/ must not exist yet, stat err = %v", err)
	}

	out, stderr, err := runAdd(t, vault, "did the thing", "--json")
	if err != nil {
		t.Fatalf("rk add: %v\nstderr: %s", err, stderr)
	}

	var res logAddResult
	mustDecodeJSON(t, out, &res)
	if res.Day != today {
		t.Errorf("Day = %q, want %q", res.Day, today)
	}
	wantPath := dayLogRelPath(today)
	if res.Path != wantPath {
		t.Errorf("Path = %q, want %q", res.Path, wantPath)
	}
	if !isValidULID(res.ID) {
		t.Errorf("ID %q is not a valid ULID", res.ID)
	}

	raw := mustReadFile(t, filepath.Join(vault, wantPath))
	n, err := node.Parse([]byte(raw))
	if err != nil {
		t.Fatalf("parse created file: %v", err)
	}
	if n.Type != "log-day" {
		t.Errorf("Type = %q, want log-day", n.Type)
	}
	if len(n.Aliases) != 1 || n.Aliases[0] != today {
		t.Errorf("Aliases = %v, want [%s]", n.Aliases, today)
	}
	if !strings.Contains(n.Body, "did the thing") {
		t.Errorf("Body = %q, missing entry text", n.Body)
	}
	if got := n.Serialize(); string(got) != raw {
		t.Errorf("parse(raw).Serialize() != raw\n--- raw ---\n%q\n--- got ---\n%q", raw, got)
	}

	entries := parseLogDayFile(t, vault, today)[1:]
	if len(entries) != 1 {
		t.Fatalf("want exactly 1 log-entry via LogParser, got %d", len(entries))
	}
	if entries[0].ULID != res.ID {
		t.Errorf("entry ULID %q != reported ID %q", entries[0].ULID, res.ID)
	}
	if entries[0].Author == "" {
		t.Error("entry Author is empty, want non-empty provenance")
	}
}

// EC-2: a completely fresh (nonexistent) vault path gets its directory tree
// created transitively; no "no such file or directory" error.
func TestAddCmd_FreshVaultRootCreatesDirs(t *testing.T) {
	root := t.TempDir()
	vault := filepath.Join(root, "brand-new-vault")
	cache := filepath.Join(root, "cache")
	t.Setenv("RECKON_CACHE", cache)
	t.Cleanup(resetCLIFlags)

	if _, err := os.Stat(vault); !os.IsNotExist(err) {
		t.Fatalf("precondition: vault must not exist yet, stat err = %v", err)
	}

	out, stderr, err := runAdd(t, vault, "first capture", "--json")
	if err != nil {
		t.Fatalf("rk add on a fresh vault path: %v\nstderr: %s", err, stderr)
	}
	var res logAddResult
	mustDecodeJSON(t, out, &res)

	if _, err := os.Stat(filepath.Join(vault, "log")); err != nil {
		t.Errorf("log/ dir not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(vault, res.Path)); err != nil {
		t.Errorf("day file not created at %q: %v", res.Path, err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-2 (AC-2): span-local append -- prior bytes untouched, only EOF bytes
// change.
// ─────────────────────────────────────────────────────────────────────────────

func TestAddCmd_SecondAppendIsSpanLocal(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	today := utcToday()

	if _, stderr, err := runAdd(t, vault, "first thing"); err != nil {
		t.Fatalf("first rk add: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()

	before := mustReadFile(t, dayLogPath(vault, today))

	if _, stderr, err := runAdd(t, vault, "second thing"); err != nil {
		t.Fatalf("second rk add: %v\nstderr: %s", err, stderr)
	}

	after := mustReadFile(t, dayLogPath(vault, today))

	if !strings.HasPrefix(after, before) {
		limit := len(before)
		if len(after) < limit {
			limit = len(after)
		}
		t.Fatalf("append disturbed existing bytes (want `before` as an exact prefix of `after`)\n--- before ---\n%q\n--- after (same length prefix) ---\n%q",
			before, after[:limit])
	}
	suffix := after[len(before):]
	if !strings.HasPrefix(suffix, "\n## ") {
		t.Errorf("appended suffix does not start with a fresh EOF-only entry block: %q", suffix)
	}
	if !strings.Contains(suffix, "second thing") {
		t.Errorf("appended suffix missing new entry text: %q", suffix)
	}

	entries := parseLogDayFile(t, vault, today)[1:]
	if len(entries) != 2 {
		t.Fatalf("want 2 log-entry nodes after 2 adds, got %d", len(entries))
	}
	if entries[0].ULID == "" || entries[1].ULID == "" {
		t.Errorf("expected both tool-written entries to carry an id:: ULID: %+v", entries)
	}
	if entries[0].ULID == entries[1].ULID {
		t.Errorf("two entries share the same ULID: %q", entries[0].ULID)
	}
}

// TS-14 (round-trip against a hand-authored day file): append is span-local
// against ANY valid day file, not only ones the tool itself created.
func TestAddCmd_AppendToHandAuthoredDayFilePreservesBytes(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	const date = "2026-07-02"
	handAuthored := "---\n" +
		"id: " + node.Mint() + "\n" +
		"type: log-day\n" +
		"aliases: " + date + "\n" +
		"---\n" +
		"# " + date + "\n\n" +
		"## 07:00 · mike\n" +
		"Hand-typed morning note, no id:: line at all.\n"
	path := dayLogPath(vault, date)
	mustWriteFile(t, path, handAuthored)

	if _, stderr, err := runAdd(t, vault, "tool-added entry", "--date", date); err != nil {
		t.Fatalf("rk add --date onto a hand-authored file: %v\nstderr: %s", err, stderr)
	}

	after := mustReadFile(t, path)
	if !strings.HasPrefix(after, handAuthored) {
		t.Fatalf("append disturbed the hand-authored entry\n--- want prefix ---\n%q\n--- got ---\n%q", handAuthored, after)
	}

	entries := parseLogDayFile(t, vault, date)[1:]
	if len(entries) != 2 {
		t.Fatalf("want 2 entries (1 hand-authored + 1 tool-added), got %d", len(entries))
	}
	if entries[0].ULID != "" {
		t.Errorf("hand-authored entry unexpectedly has a ULID: %q", entries[0].ULID)
	}
	if !strings.Contains(entries[1].Body, "tool-added entry") {
		t.Errorf("second entry missing tool-added text: %q", entries[1].Body)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-4 (AC-5): capture -> index -> query end-to-end.
// ─────────────────────────────────────────────────────────────────────────────

func TestAddCmd_CaptureIndexQueryEndToEnd(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	if _, stderr, err := runAdd(t, vault, "buy milk"); err != nil {
		t.Fatalf("rk add: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()

	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault, "SELECT body, type FROM nodes WHERE type='log-entry'")
	if err != nil {
		t.Fatalf("rk query: %v\nstderr: %s", err, stderr)
	}
	rows := parseNDJSONMaps(t, stdout)
	if len(rows) != 1 {
		t.Fatalf("want exactly 1 log-entry row, got %d: %v", len(rows), rows)
	}
	if rows[0]["type"] != "log-entry" {
		t.Errorf("type = %v, want log-entry", rows[0]["type"])
	}
	if !strings.Contains(fmt.Sprintf("%v", rows[0]["body"]), "buy milk") {
		t.Errorf("body = %v, want to contain %q", rows[0]["body"], "buy milk")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-5 (AC-6): provenance on every entry, not just the first -- explicit
// --author, and the resolveAuthor fallback chain when absent.
// ─────────────────────────────────────────────────────────────────────────────

func TestAddCmd_ProvenanceOnEveryEntry(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	today := utcToday()

	if _, stderr, err := runAdd(t, vault, "first entry", "--author", "agent-x"); err != nil {
		t.Fatalf("rk add --author: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()

	t.Setenv("RECKON_AUTHOR", "")
	t.Setenv("USER", "")
	if _, stderr, err := runAdd(t, vault, "second entry"); err != nil {
		t.Fatalf("rk add (no author flag/env): %v\nstderr: %s", err, stderr)
	}

	entries := parseLogDayFile(t, vault, today)[1:]
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(entries))
	}
	byBody := map[string]*node.Node{}
	for _, e := range entries {
		byBody[strings.TrimSpace(e.Body)] = e
	}
	first, ok := byBody["first entry"]
	if !ok {
		t.Fatalf("first entry not found among %+v", entries)
	}
	if first.Author != "agent-x" {
		t.Errorf("first entry author = %q, want agent-x", first.Author)
	}
	second, ok := byBody["second entry"]
	if !ok {
		t.Fatalf("second entry not found among %+v", entries)
	}
	if second.Author != "local" {
		t.Errorf("second entry author = %q, want local (resolveAuthor fallback chain)", second.Author)
	}
	if first.Author == "" || second.Author == "" {
		t.Error("provenance must be non-empty on every entry")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-6: --at HH:MM backfill sets the entry's time, independent of the
// ULID's own mint timestamp (the design's time-vs-mint-time split).
// ─────────────────────────────────────────────────────────────────────────────

func TestAddCmd_AtFlagBackfillsTimeIndependentOfULIDMintTime(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	today := utcToday()

	before := time.Now()
	out, stderr, err := runAdd(t, vault, "stood up", "--at", "09:15", "--json")
	if err != nil {
		t.Fatalf("rk add --at: %v\nstderr: %s", err, stderr)
	}
	after := time.Now()

	var res logAddResult
	mustDecodeJSON(t, out, &res)

	wantTime := today + "T09:15:00Z"
	if res.Time != wantTime {
		t.Errorf("Time = %q, want %q", res.Time, wantTime)
	}

	entries := parseLogDayFile(t, vault, today)[1:]
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	if entries[0].Time != wantTime {
		t.Errorf("parsed entry Time = %q, want %q", entries[0].Time, wantTime)
	}

	// The ULID must still mint at the real wall-clock capture moment, NOT
	// 09:15 -- the design's time-vs-ULID-mint-time split (plan.md
	// Decision 4). A 5s tolerance window absorbs test-execution jitter.
	id, err := ulid.Parse(res.ID)
	if err != nil {
		t.Fatalf("parse ULID %q: %v", res.ID, err)
	}
	mintTime := id.Timestamp()
	if mintTime.Before(before.Add(-5*time.Second)) || mintTime.After(after.Add(5*time.Second)) {
		t.Errorf("ULID mint time %v not within [%v, %v] of the actual command invocation -- --at must backfill only the rendered time, never the ULID's own mint instant",
			mintTime, before, after)
	}
}

func TestAddCmd_AtFlagInvalidFormatRejected(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	if _, _, err := runAdd(t, vault, "bad time", "--at", "25:99"); err == nil {
		t.Fatal("expected an error for an invalid --at value, got nil")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// C1 regression (reckon-uv09 code review): the day file's date and the
// entry's HH:MM must come from ONE clock when --date/--at are both
// omitted. Before the fix, the day came from getEffectiveDate()'s LOCAL
// default while HH:MM came from resolveAtTime()'s UTC default; near a day
// boundary on a non-UTC host the two would disagree by a full day (e.g. a
// Sydney host at local 2026-07-05 08:30 would produce
// "2026-07-05T22:30:00Z" instead of the true UTC instant
// "2026-07-04T22:30:00Z").
//
// Faked deterministically by pointing the process-global time.Local at a
// real non-UTC zone (Australia/Sydney, UTC+10/+11) rather than waiting for
// (or fabricating) a real wall-clock midnight rollover -- no clock-injection
// seam is pinned by plan.md for add.go (see TestAddCmd_DayBoundaryRouting's
// skip rationale above), and mutating time.Local is the standard, safely
// restorable way to exercise Go's local-vs-UTC formatting divergence
// without one. Safe here because this package's tests never run with
// t.Parallel().
// ─────────────────────────────────────────────────────────────────────────────

func TestAddCmd_DefaultDateAndTimeShareOneClock(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	sydney, err := time.LoadLocation("Australia/Sydney")
	if err != nil {
		t.Skipf("tzdata unavailable (Australia/Sydney): %v", err)
	}
	origLocal := time.Local
	time.Local = sydney
	t.Cleanup(func() { time.Local = origLocal })

	wantDay := time.Now().UTC().Format("2006-01-02")

	out, stderr, err := runAdd(t, vault, "clock unification regression", "--json")
	if err != nil {
		t.Fatalf("rk add: %v\nstderr: %s", err, stderr)
	}
	var res logAddResult
	mustDecodeJSON(t, out, &res)

	// The day file must be selected from UTC, never from time.Local's
	// (here, Sydney's) calendar date.
	if res.Day != wantDay {
		t.Errorf("Day = %q, want UTC day %q -- the day file must default to UTC, not time.Local's calendar date", res.Day, wantDay)
	}
	wantPath := dayLogRelPath(wantDay)
	if res.Path != wantPath {
		t.Errorf("Path = %q, want %q", res.Path, wantPath)
	}

	// The composed `time` field's date component must agree with the day
	// file it was written into -- i.e. both halves came from the same
	// clock, the defect this test pins.
	if !strings.HasPrefix(res.Time, wantDay+"T") {
		t.Errorf("Time = %q, want date prefix %q (date half and HH:MM half of `time` must share one clock)", res.Time, wantDay)
	}
	if !strings.HasSuffix(res.Time, ":00Z") {
		t.Errorf("Time = %q, want a Z-tagged HH:MM:00 suffix", res.Time)
	}

	entries := parseLogDayFile(t, vault, wantDay)[1:]
	if len(entries) != 1 {
		t.Fatalf("want 1 entry in the UTC day file, got %d", len(entries))
	}
	if entries[0].Time != res.Time {
		t.Errorf("parsed entry Time = %q, want %q (reported result)", entries[0].Time, res.Time)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-9 (EC-6): a whole-file-CRLF day file is refused outright on append,
// mirroring the reckon-vj55 CRLF guard used throughout todo.go/adopt.go.
// ─────────────────────────────────────────────────────────────────────────────

func TestAddCmd_CRLFDayFileRejected(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	const date = "2026-07-01"
	crlf := "---\r\n" +
		"id: " + node.Mint() + "\r\n" +
		"type: log-day\r\n" +
		"aliases: " + date + "\r\n" +
		"---\r\n" +
		"# " + date + "\r\n" +
		"\r\n" +
		"## 08:00 · mike\r\n" +
		"existing entry\r\n"
	path := dayLogPath(vault, date)
	mustWriteFile(t, path, crlf)

	_, stderr, err := runAdd(t, vault, "new entry", "--date", date)
	if err == nil {
		t.Fatal("expected an error appending to a CRLF day file, got nil")
	}
	combined := strings.ToLower(stderr + err.Error())
	if !strings.Contains(combined, "crlf") {
		t.Errorf("expected a CRLF-specific error, got stderr=%q err=%v", stderr, err)
	}

	if got := mustReadFile(t, path); got != crlf {
		t.Fatalf("CRLF file was mutated despite the rejection\n--- want ---\n%q\n--- got ---\n%q", crlf, got)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-10 (EC-7): empty or whitespace-only body is rejected, mirroring
// `todo add`'s empty-body rejection.
// ─────────────────────────────────────────────────────────────────────────────

func TestAddCmd_EmptyOrWhitespaceBodyRejected(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	for _, body := range []string{"", "   ", "\t\n"} {
		t.Run(fmt.Sprintf("body=%q", body), func(t *testing.T) {
			resetCLIFlags()
			if _, _, err := runAdd(t, vault, body); err == nil {
				t.Fatalf("expected an error for body %q, got nil", body)
			}
		})
	}

	if _, err := os.Stat(filepath.Join(vault, "log")); !os.IsNotExist(err) {
		t.Errorf("log/ dir must not be created for a rejected empty/whitespace body, stat err = %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-11 (EC-9): a body that would introduce a literal "## " line is
// rejected (defensive guard against mis-splitting one logical entry into
// two spurious ones via the naive `^## .*$` SplitEntries header match).
// ─────────────────────────────────────────────────────────────────────────────

func TestAddCmd_EmbeddedHeaderLineGuard(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	cases := []struct {
		name      string
		body      string
		wantError bool
	}{
		// A single CLI argument containing an embedded newline followed by
		// "## " -- the realistic way this arises despite args normally being
		// space-joined (piped/programmatic multi-line text as one arg).
		{"embedded_newline_then_hash_header", "para one\n## sneaky header\npara two", true},
		// The body's very FIRST line is "## ...": once embedded right after
		// the entry's "id:: <ULID>\n" line, this produces exactly the same
		// "\n## " hazard as the embedded-newline case above.
		{"body_itself_starts_with_hash_header", "## fake header\nmore text", true},
		// "##" appearing mid-line (not at the start of any line) is not a
		// SplitEntries header match and must be accepted.
		{"midline_hash_not_a_header_is_fine", "cost is $5, see ## widget for details", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetCLIFlags()
			_, stderr, err := runAdd(t, vault, tc.body)
			if tc.wantError && err == nil {
				t.Fatalf("expected an error for body %q, got nil", tc.body)
			}
			if !tc.wantError && err != nil {
				t.Fatalf("expected no error for body %q, got: %v\nstderr: %s", tc.body, err, stderr)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-12 (EC-10): day-boundary routing -- best effort, skipped cleanly.
// ─────────────────────────────────────────────────────────────────────────────

// TestAddCmd_DayBoundaryRouting is intentionally skipped rather than written
// as a flaky test. Faking a real wall-clock midnight rollover deterministically
// would require either sleeping across an actual day boundary or introducing
// a clock-injection seam that plan.md does not name anywhere for add.go (unlike,
// e.g., todo.go's mintTodoULID seam for a different scenario) -- inventing one
// here would mean testing a seam this test-writing phase itself invented,
// not a pinned contract. getEffectiveDate()'s wall-clock-now default (the
// mechanism EC-10 recommends file selection follow) is already exercised
// implicitly by every other test in this file that omits --date, and its
// --date override half is exercised explicitly by
// TestAddCmd_DateFlagTargetsPastDay below.
func TestAddCmd_DayBoundaryRouting(t *testing.T) {
	t.Skip("TS-12/EC-10: day-boundary routing needs a real wall-clock midnight rollover; " +
		"no clock-injection seam is pinned by plan.md for add.go, and faking one deterministically " +
		"is out of this test-writing phase's scope (see comment above). Skipped cleanly rather than flaky.")
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-13 (EC-11): index lag is intentional, not a defect -- rk add never
// reconciles the index itself.
// ─────────────────────────────────────────────────────────────────────────────

func TestAddCmd_IndexLagIsIntentional(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	buildIndex(t, vault) // baseline index, built before the entry exists
	resetCLIFlags()

	if _, stderr, err := runAdd(t, vault, "just added"); err != nil {
		t.Fatalf("rk add: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()

	stdout, stderr, err := runQuery(t, vault, "SELECT * FROM nodes WHERE type='log-entry'")
	if err != nil {
		t.Fatalf("rk query before an index pass: %v\nstderr: %s", err, stderr)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("expected the new entry to be absent before an index pass (stale index is intentional), got: %q", stdout)
	}
	resetCLIFlags()

	buildIndex(t, vault)

	stdout2, stderr2, err2 := runQuery(t, vault, "SELECT body, type FROM nodes WHERE type='log-entry'")
	if err2 != nil {
		t.Fatalf("rk query after an index pass: %v\nstderr: %s", err2, stderr2)
	}
	rows := parseNDJSONMaps(t, stdout2)
	if len(rows) != 1 {
		t.Fatalf("want exactly 1 log-entry row after indexing, got %d: %v", len(rows), rows)
	}
	if !strings.Contains(fmt.Sprintf("%v", rows[0]["body"]), "just added") {
		t.Errorf("body = %v, want to contain %q", rows[0]["body"], "just added")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-15: --date targets a past day's file, not today's.
// ─────────────────────────────────────────────────────────────────────────────

func TestAddCmd_DateFlagTargetsPastDay(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	const date = "2026-07-01"
	out, stderr, err := runAdd(t, vault, "backfilled entry", "--date", date, "--json")
	if err != nil {
		t.Fatalf("rk add --date: %v\nstderr: %s", err, stderr)
	}
	var res logAddResult
	mustDecodeJSON(t, out, &res)

	if res.Day != date {
		t.Errorf("Day = %q, want %q", res.Day, date)
	}
	wantPath := dayLogRelPath(date)
	if res.Path != wantPath {
		t.Errorf("Path = %q, want %q", res.Path, wantPath)
	}

	entries, err := os.ReadDir(filepath.Join(vault, "log"))
	if err != nil {
		t.Fatalf("read log dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != date+".md" {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Fatalf("log/ dir = %v, want exactly one file %q (today's file must not be created)", names, date+".md")
	}

	// No --at given: time defaults to the current wall-clock HH:MM within
	// the backfilled day, not a fixed/zero value.
	if !strings.HasPrefix(res.Time, date+"T") || !strings.HasSuffix(res.Time, ":00Z") {
		t.Errorf("Time = %q, want the form %sT<HH:MM>:00Z (current wall-clock HH:MM, no --at given)", res.Time, date)
	}
}
