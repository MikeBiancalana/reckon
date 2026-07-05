// Package node — TDD red tests for v1-T4's group-file log parser
// (internal/node/logparser.go, not yet created). LogParser and RenderLogEntry
// are referenced below before they exist, so this package fails to COMPILE
// right now -- the expected TDD-red state ("the package does not build",
// not "tests run and fail"), mirroring internal/cli/todo_test.go's own header
// convention for the immediately-preceding reckon-qiua ticket.
//
// ─────────────────────────────────────────────────────────────────────────
// Pinned contract (plan.md "New — internal/node/logparser.go"):
// ─────────────────────────────────────────────────────────────────────────
//
//	type LogParser struct{}
//	func (LogParser) Parse(raw []byte, loc Loc) ([]*Node, error)
//	func (LogParser) Serialize(n *Node) ([]byte, error)
//	func RenderLogEntry(hhmm, author, ulid, body string) string
//
// LogParser.Parse: ParseAt(raw, loc) once; if the parsed node's Type is not
// "log-day", return []*Node{day} unchanged (byte-identical to
// MarkdownParser's behavior); otherwise split via day.SplitEntries(),
// building one log-entry *Node per block:
//   - ULID from an `id:: <ULID>` line (the line is dropped from Body);
//     entries with no id:: line get ULID == "" (index-level surrogate
//     keying is a different package's concern, see reconcile.go).
//   - Type = "log-entry".
//   - Time reconstructed as "<dayDate>T<HH:MM>:00Z" -- dayDate from the day
//     node's first alias, else derived from the "log/<date>.md" Loc.File.
//   - Author from the header's "· author" suffix.
//   - Props["kind"] set only when the header carries an optional kind word
//     ("## HH:MM kind · author"); absent entirely when there is no kind word.
//   - The day node gets one Link{Rel: "contains", To: <entryULID>} appended
//     per ULID-bearing entry (zero contains links when no entry has an id::
//     line).
//
// RenderLogEntry(hhmm, author, ulid, body) returns exactly:
//
//	"## " + hhmm + " · " + author + "\n" + "id:: " + ulid + "\n" + body + "\n"
//
// Fixture reuse: `logDay` (node_test.go) has 3 kind-word headers and NO id::
// lines at all -- used below for split-count/kind-word/hand-authored-no-id
// scenarios. `logDayWithIDs` (added to node_test.go's roundtripCorpus
// alongside logDay, this same phase) carries id:: lines on both its entries
// -- used for ULID/contains-link/time-reconstruction scenarios, and to prove
// id:: lines survive TestRoundTripIdentity/FuzzRoundTripIdentity untouched.
// `todoItem` (node_test.go) is reused as a non-log-day fixture. `isCrockford`
// (ulid_test.go) is reused rather than redefining a ULID-shape check.
package node

import (
	"strings"
	"testing"
)

// ─────────────────────────────────────────────────────────────────────────
// N+1 split count (AC-4 / TS-3): a day file with 3 entries splits into
// exactly 4 nodes (1 log-day + 3 log-entry).
// ─────────────────────────────────────────────────────────────────────────

func TestLogParser_SplitsThreeEntriesIntoFourNodes(t *testing.T) {
	nodes, err := LogParser{}.Parse([]byte(logDay), Loc{File: "log/2026-06-22.md"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(nodes) != 4 {
		t.Fatalf("want 4 nodes (1 log-day + 3 log-entry), got %d", len(nodes))
	}
	if nodes[0].Type != "log-day" {
		t.Errorf("nodes[0].Type = %q, want log-day", nodes[0].Type)
	}
	for i, n := range nodes[1:] {
		if n.Type != "log-entry" {
			t.Errorf("nodes[%d].Type = %q, want log-entry", i+1, n.Type)
		}
		if n.Loc.File != "log/2026-06-22.md" {
			t.Errorf("nodes[%d].Loc.File = %q, want propagated from the day's Loc", i+1, n.Loc.File)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────
// Distinct, non-empty entry ULIDs read from id:: lines (AC-4 / TS-3),
// different from the day node's own ULID and from each other.
// ─────────────────────────────────────────────────────────────────────────

func TestLogParser_DistinctNonEmptyEntryULIDs(t *testing.T) {
	nodes, err := LogParser{}.Parse([]byte(logDayWithIDs), Loc{File: "log/2026-07-05.md"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(nodes) != 3 {
		t.Fatalf("want 3 nodes (1 log-day + 2 log-entry), got %d", len(nodes))
	}
	day := nodes[0]
	if day.ULID == "" {
		t.Fatal("day node ULID is empty; fixture has an id: frontmatter field")
	}
	seen := map[string]bool{}
	for i, e := range nodes[1:] {
		if e.ULID == "" {
			t.Errorf("entry %d: ULID empty, want the id:: value from the fixture", i)
			continue
		}
		if !isCrockford(e.ULID) {
			t.Errorf("entry %d: ULID %q not 26-char Crockford base32", i, e.ULID)
		}
		if e.ULID == day.ULID {
			t.Errorf("entry %d: ULID %q collides with the day node's own ULID", i, e.ULID)
		}
		if seen[e.ULID] {
			t.Errorf("entry %d: duplicate ULID %q across entries", i, e.ULID)
		}
		seen[e.ULID] = true
	}
	if len(seen) != 2 {
		t.Errorf("want 2 distinct entry ULIDs, got %d: %v", len(seen), seen)
	}
}

// ─────────────────────────────────────────────────────────────────────────
// Time reconstruction: Time becomes "<dayDate>T<HH:MM>:00Z", dayDate from
// the day node's first alias, else from the "log/<date>.md" Loc.File.
// ─────────────────────────────────────────────────────────────────────────

func TestLogParser_TimeReconstruction(t *testing.T) {
	t.Run("from_day_alias", func(t *testing.T) {
		nodes, err := LogParser{}.Parse([]byte(logDayWithIDs), Loc{File: "log/2026-07-05.md"})
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		wantByULID := map[string]string{
			"01J9Z3K7Q2W8XR4M6N0V5BYHER": "2026-07-05T08:38:00Z",
			"01J9Z3K7Q2W8XR4M6N0V5BYHES": "2026-07-05T10:17:00Z",
		}
		for _, e := range nodes[1:] {
			want, ok := wantByULID[e.ULID]
			if !ok {
				t.Fatalf("unexpected entry ULID %q (fixture drifted?)", e.ULID)
			}
			if e.Time != want {
				t.Errorf("entry %q: Time = %q, want %q", e.ULID, e.Time, want)
			}
		}
	})

	t.Run("from_loc_file_when_no_alias", func(t *testing.T) {
		src := "---\n" +
			"id: 01J9Z3K7Q2W8XR4M6N0V5BYHFA\n" +
			"type: log-day\n" +
			"---\n" +
			"# Untitled day\n\n" +
			"## 09:00 · mike\n" +
			"No aliases on this day node; date must come from Loc.File instead.\n"
		nodes, err := LogParser{}.Parse([]byte(src), Loc{File: "log/2026-07-01.md"})
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if len(nodes) != 2 {
			t.Fatalf("want 2 nodes, got %d", len(nodes))
		}
		want := "2026-07-01T09:00:00Z"
		if nodes[1].Time != want {
			t.Errorf("Time = %q, want %q (derived from Loc.File, no alias present)", nodes[1].Time, want)
		}
	})
}

// ─────────────────────────────────────────────────────────────────────────
// Kind-word tolerance: "## HH:MM kind · author" parses kind into
// Props["kind"]; a header without a kind word ("## HH:MM · author") still
// parses cleanly, with no Props["kind"] entry at all.
// ─────────────────────────────────────────────────────────────────────────

func TestLogParser_KindWordTolerance(t *testing.T) {
	t.Run("kind_word_present_becomes_props_kind", func(t *testing.T) {
		nodes, err := LogParser{}.Parse([]byte(logDay), Loc{File: "log/2026-06-22.md"})
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		// logDay's headers: "## 08:38 progress · mike", "## 10:17 win · mike",
		// "## 11:19 note · agent".
		wantKind := []string{"progress", "win", "note"}
		wantAuthor := []string{"mike", "mike", "agent"}
		for i, e := range nodes[1:] {
			if e.Props["kind"] != wantKind[i] {
				t.Errorf("entry %d: Props[kind] = %q, want %q", i, e.Props["kind"], wantKind[i])
			}
			if e.Author != wantAuthor[i] {
				t.Errorf("entry %d: Author = %q, want %q", i, e.Author, wantAuthor[i])
			}
		}
	})

	t.Run("no_kind_word_still_parses", func(t *testing.T) {
		src := "---\n" +
			"id: 01J9Z3K7Q2W8XR4M6N0V5BYHFB\n" +
			"type: log-day\n" +
			"aliases: 2026-07-05\n" +
			"---\n" +
			"# 2026-07-05\n\n" +
			"## 09:00 · mike\n" +
			"Entry with no kind word at all in its header.\n"
		nodes, err := LogParser{}.Parse([]byte(src), Loc{File: "log/2026-07-05.md"})
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if len(nodes) != 2 {
			t.Fatalf("want 2 nodes, got %d", len(nodes))
		}
		e := nodes[1]
		if e.Author != "mike" {
			t.Errorf("Author = %q, want mike", e.Author)
		}
		if e.Time != "2026-07-05T09:00:00Z" {
			t.Errorf("Time = %q, want 2026-07-05T09:00:00Z", e.Time)
		}
		if kind, has := e.Props["kind"]; has {
			t.Errorf("Props[kind] = %q, want absent entirely (no kind word in header)", kind)
		}
		if !strings.Contains(e.Body, "no kind word at all") {
			t.Errorf("Body = %q, missing expected entry text", e.Body)
		}
	})
}

// ─────────────────────────────────────────────────────────────────────────
// Hand-authored entries with no id:: line still split out as log-entry
// nodes (surrogate-keyed by file:path#N at the INDEX level, not here) --
// no crash, no fabricated ULID.
// ─────────────────────────────────────────────────────────────────────────

func TestLogParser_HandAuthoredEntriesWithoutIDStillSplit(t *testing.T) {
	nodes, err := LogParser{}.Parse([]byte(logDay), Loc{File: "log/2026-06-22.md"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(nodes) != 4 {
		t.Fatalf("want 4 nodes, got %d", len(nodes))
	}
	for i, e := range nodes[1:] {
		if e.Type != "log-entry" {
			t.Errorf("entry %d: Type = %q, want log-entry", i, e.Type)
		}
		if e.ULID != "" {
			t.Errorf("entry %d: ULID = %q, want empty (logDay fixture has no id:: lines)", i, e.ULID)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────
// `contains` links: the day node gets one Link{Rel:"contains", To:<ULID>}
// per ULID-bearing entry (Open-Q 7 / plan.md secondary call); zero when no
// entry carries an id:: line.
// ─────────────────────────────────────────────────────────────────────────

func TestLogParser_DayNodeGetsContainsLinksForIDBearingEntries(t *testing.T) {
	nodes, err := LogParser{}.Parse([]byte(logDayWithIDs), Loc{File: "log/2026-07-05.md"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	day := nodes[0]
	entryULIDs := map[string]bool{}
	for _, e := range nodes[1:] {
		entryULIDs[e.ULID] = true
	}
	var contains []Link
	for _, l := range day.Links {
		if l.Rel == "contains" {
			contains = append(contains, l)
		}
	}
	if len(contains) != len(entryULIDs) {
		t.Fatalf("want %d contains links (one per ID-bearing entry), got %d: %+v",
			len(entryULIDs), len(contains), contains)
	}
	for _, l := range contains {
		if !entryULIDs[l.To] {
			t.Errorf("contains link To=%q does not match any entry ULID", l.To)
		}
	}

	// Regression: logDay's entries carry no id:: lines at all -> zero
	// contains links, not a crash or a garbage edge to an empty target.
	nodesNoIDs, err := LogParser{}.Parse([]byte(logDay), Loc{File: "log/2026-06-22.md"})
	if err != nil {
		t.Fatalf("Parse (no-id fixture): %v", err)
	}
	for _, l := range nodesNoIDs[0].Links {
		if l.Rel == "contains" {
			t.Errorf("unexpected contains link on a day with no ID-bearing entries: %+v", l)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────
// A non-log-day node returns as a single node unchanged (MarkdownParser
// parity): dispatch is Type-driven, not path-driven.
// ─────────────────────────────────────────────────────────────────────────

func TestLogParser_NonLogDayReturnsSingleNodeUnchanged(t *testing.T) {
	nodes, err := LogParser{}.Parse([]byte(todoItem), Loc{File: "todos/x.md"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("want 1 node for a non-log-day file, got %d", len(nodes))
	}
	if nodes[0].Type != "todo" {
		t.Errorf("Type = %q, want todo (unchanged)", nodes[0].Type)
	}
	if nodes[0].Loc.File != "todos/x.md" {
		t.Errorf("Loc.File = %q, not propagated", nodes[0].Loc.File)
	}
	out, err := LogParser{}.Serialize(nodes[0])
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	if string(out) != todoItem {
		t.Errorf("Serialize output not byte-identical to input:\n%q", out)
	}
}

// Compile-time interface conformance, mirroring parser.go's own
// `var _ Parser = MarkdownParser{}` pattern.
var _ Parser = LogParser{}

// ─────────────────────────────────────────────────────────────────────────
// RenderLogEntry: the one shared format definition writer (internal/cli/
// add.go) and reader (LogParser) agree on.
// ─────────────────────────────────────────────────────────────────────────

func TestRenderLogEntry_ExactBlockFormat(t *testing.T) {
	got := RenderLogEntry("09:15", "mike", "01J9Z3K7Q2W8XR4M6N0V5BYHFC", "did the thing")
	want := "## 09:15 · mike\n" +
		"id:: 01J9Z3K7Q2W8XR4M6N0V5BYHFC\n" +
		"did the thing\n"
	if got != want {
		t.Fatalf("RenderLogEntry mismatch\n--- want ---\n%q\n--- got ---\n%q", want, got)
	}
}

// The rendered block, embedded in a day file and re-parsed through
// LogParser, must recover the exact hhmm/author/ulid/body it was rendered
// from -- writer and reader agreeing on one format, independent of the
// whole-fixture corpus test below.
func TestRenderLogEntry_RoundTripsThroughLogParser(t *testing.T) {
	ulid := "01J9Z3K7Q2W8XR4M6N0V5BYHFD"
	block := RenderLogEntry("14:30", "agent-x", ulid, "shipped the feature")
	src := "---\n" +
		"id: 01J9Z3K7Q2W8XR4M6N0V5BYHFE\n" +
		"type: log-day\n" +
		"aliases: 2026-07-05\n" +
		"---\n" +
		"# 2026-07-05\n\n" +
		block
	nodes, err := LogParser{}.Parse([]byte(src), Loc{File: "log/2026-07-05.md"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("want 2 nodes, got %d", len(nodes))
	}
	e := nodes[1]
	if e.ULID != ulid {
		t.Errorf("ULID = %q, want %q", e.ULID, ulid)
	}
	if e.Author != "agent-x" {
		t.Errorf("Author = %q, want agent-x", e.Author)
	}
	if e.Time != "2026-07-05T14:30:00Z" {
		t.Errorf("Time = %q, want 2026-07-05T14:30:00Z", e.Time)
	}
	if strings.TrimSpace(e.Body) != "shipped the feature" {
		t.Errorf("Body = %q, want %q", e.Body, "shipped the feature")
	}
}

// ─────────────────────────────────────────────────────────────────────────
// Round-trip: logDayWithIDs (node_test.go's roundtripCorpus) proves id::
// lines are inert to the CORE parser via TestRoundTripIdentity/
// FuzzRoundTripIdentity already. This test additionally proves LogParser
// itself agrees byte-for-byte with the day node's own Serialize on that
// same fixture (LogParser.Serialize is byte-preserving, never re-derived).
// ─────────────────────────────────────────────────────────────────────────

func TestLogParser_IDBearingFixtureRoundTrips(t *testing.T) {
	if _, ok := roundtripCorpus["logDayWithIDs"]; !ok {
		t.Fatal("logDayWithIDs missing from roundtripCorpus (node_test.go) -- add it alongside logDay")
	}
	nodes, err := LogParser{}.Parse([]byte(logDayWithIDs), Loc{File: "log/2026-07-05.md"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	day := nodes[0]
	out, err := LogParser{}.Serialize(day)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	if string(out) != logDayWithIDs {
		t.Fatalf("LogParser.Serialize(day) not byte-identical to source\n--- want ---\n%q\n--- got ---\n%q",
			logDayWithIDs, out)
	}
}
