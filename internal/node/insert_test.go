package node

// TDD red tests for reckon-9bfx: `(*Node) InsertField(key, value string) error`
// — the span-safe frontmatter-insert primitive `rk adopt` uses to stamp a
// minted `id:` into an id-less file. InsertField does not exist yet
// (internal/node/insert.go is not yet created); every test below calls it
// directly, so this file does not compile until that method is implemented.
// That non-compilation IS the expected red state for this gate (see plan.md
// D1/D2, acceptance-criteria.md §3 "malformed or unparseable frontmatter").
//
// Design under test (plan.md D2 — "Detection trichotomy for where to insert"):
//   - key already present as a scalar -> error (symmetric to SetField's
//     missing-key error; TestSetFieldMissingKey in node_test.go).
//   - a terminated frontmatter block exists (bodySpan.Start > 0) -> splice the
//     new "key: value\n" line in as the FIRST line inside the fence (matches
//     Render's canonical key order, render.go:36-38); every other byte
//     unchanged.
//   - raw starts with the exact 4-byte fence "---\n" but no closing "\n---" is
//     found (unterminated) -> refuse outright, never silently treat as "no
//     block" (would nest the user's attempted frontmatter into the body).
//   - raw does not start with "---\n" at all -> prepend a fresh
//     "---\nkey: value\n---\n" block ahead of the untouched original bytes.

import (
	"bytes"
	"strings"
	"testing"
)

// Scenario (acceptance-criteria.md §4, "stamps a missing id into an existing
// frontmatter block"): a file has a frontmatter block but no `id:` key. The
// new line must land as the first line inside the fence, every other byte
// must be untouched, and reparsing must show the new field.
func TestInsertField_ExistingBlockNoIDKey(t *testing.T) {
	src := []byte("---\ntype: note\n---\nbody text\n")
	n, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if n.ULID != "" {
		t.Fatalf("fixture unexpectedly already has an id: %q", n.ULID)
	}

	const ulid = "01J9Z3K7Q2W8XR4M6N0V5BYHED"
	if err := n.InsertField("id", ulid); err != nil {
		t.Fatalf("InsertField: %v", err)
	}

	want := "---\nid: " + ulid + "\ntype: note\n---\nbody text\n"
	if got := string(n.Serialize()); got != want {
		t.Fatalf("insert not surgical\n--- want ---\n%q\n--- got ---\n%q", want, got)
	}

	// The id: line must be the FIRST line inside the fence.
	lines := strings.Split(string(n.Serialize()), "\n")
	if len(lines) < 2 || lines[0] != "---" || lines[1] != "id: "+ulid {
		t.Fatalf("id: not the first line inside the fence: %q", n.Serialize())
	}

	// Every other byte (the "type: note" line, both "---" delimiters, and
	// the body) must be unchanged and present verbatim after the new line.
	if !strings.HasSuffix(string(n.Serialize()), "type: note\n---\nbody text\n") {
		t.Fatalf("original bytes disturbed: %q", n.Serialize())
	}

	// Reparsing (both the mutated n itself, and a fresh Parse of its bytes)
	// must show the new field and the untouched pre-existing view.
	if n.ULID != ulid {
		t.Errorf("n.ULID after InsertField = %q, want %q", n.ULID, ulid)
	}
	if n.Type != "note" {
		t.Errorf("n.Type after InsertField = %q, want note", n.Type)
	}
	if n.Body != "body text\n" {
		t.Errorf("n.Body after InsertField = %q, want %q", n.Body, "body text\n")
	}

	reparsed, err := Parse(n.Serialize())
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	if reparsed.ULID != ulid || reparsed.Type != "note" || reparsed.Body != "body text\n" {
		t.Fatalf("reparse did not show the new field: %+v", reparsed)
	}
}

// Scenario (acceptance-criteria.md §4, "inserts a whole frontmatter block
// into a file with none"): a body-only file (no `---` block at all) gets a
// fresh `---\nid: <v>\n---\n` block prepended, and the original bytes follow
// byte-for-byte, unchanged.
func TestInsertField_NoFrontmatterBlock(t *testing.T) {
	src := []byte("just some text\n")
	n, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if n.ULID != "" {
		t.Fatalf("fixture unexpectedly already has an id: %q", n.ULID)
	}

	const ulid = "01J9Z3K7Q2W8XR4M6N0V5BYHEE"
	if err := n.InsertField("id", ulid); err != nil {
		t.Fatalf("InsertField: %v", err)
	}

	want := "---\nid: " + ulid + "\n---\n" + string(src)
	if got := string(n.Serialize()); got != want {
		t.Fatalf("insert not surgical\n--- want ---\n%q\n--- got ---\n%q", want, got)
	}
	if n.ULID != ulid {
		t.Errorf("n.ULID = %q, want %q", n.ULID, ulid)
	}
	if n.Body != string(src) {
		t.Errorf("n.Body = %q, want %q (original body untouched)", n.Body, src)
	}
}

// Refuses (non-nil error) on an unterminated frontmatter block: the raw bytes
// start with the exact fence "---\n" but no closing "\n---" is found anywhere.
// Per plan.md D2, this must NOT be silently treated as "no block" (that would
// nest the user's attempted frontmatter into the body) — it is a hard refusal,
// and the node must be left completely untouched.
func TestInsertField_RefusesUnterminatedBlock(t *testing.T) {
	src := []byte("---\ntype: note\nno closing fence anywhere in this file\n")
	n, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if n.ULID != "" {
		t.Fatalf("fixture unexpectedly already has an id: %q", n.ULID)
	}
	orig := append([]byte(nil), n.Serialize()...)

	if err := n.InsertField("id", "01J9Z3K7Q2W8XR4M6N0V5BYHEF"); err == nil {
		t.Fatal("expected an error inserting into an unterminated frontmatter block")
	}

	if got := n.Serialize(); !bytes.Equal(got, orig) {
		t.Fatalf("node mutated despite refusal\n--- want ---\n%q\n--- got ---\n%q", orig, got)
	}
}

// Refuses (non-nil error) when the key already exists — symmetric to
// SetField's missing-key error (TestSetFieldMissingKey, node_test.go:223):
// SetField requires the key to already exist; InsertField requires it not to.
// Existing content must be untouched by the refused call.
func TestInsertField_RefusesExistingKey(t *testing.T) {
	n, err := Parse([]byte(todoItem)) // todoItem (node_test.go) already has id:
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if n.ULID == "" {
		t.Fatal("fixture expected to already have an id")
	}
	orig := append([]byte(nil), n.Serialize()...)

	if err := n.InsertField("id", "01J9Z3K7Q2W8XR4M6N0V5BYHEG"); err == nil {
		t.Fatal("expected an error inserting a key that already exists")
	}

	if got := n.Serialize(); !bytes.Equal(got, orig) {
		t.Fatalf("node mutated despite refusal\n--- want ---\n%q\n--- got ---\n%q", orig, got)
	}
	if n.ULID != "01J9Z3K7Q2W8XR4M6N0V5BYHEE" {
		t.Errorf("existing id value disturbed: %q", n.ULID)
	}
}

// A body that itself starts with dash characters (e.g. an alternate-style
// markdown thematic break) but does NOT match the exact 4-byte frontmatter
// fence signature "---\n" must be treated as "no frontmatter block at all"
// (a fresh block prepended), not misdetected as an unterminated fence (which
// would wrongly refuse). This pins the fence detector to the exact byte
// signature, not a loose "starts with dashes" heuristic.
func TestInsertField_LeadingDashesNotFrontmatterFence(t *testing.T) {
	// raw[3] is 'N', not '\n' — bytes.HasPrefix(raw, "---\n") is false, so
	// this must NOT be classified as an (unterminated) fence attempt.
	src := []byte("---Not a frontmatter fence, just dashes.\nMore body text.\n")
	n, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if n.ULID != "" {
		t.Fatalf("fixture unexpectedly already has an id: %q", n.ULID)
	}

	const ulid = "01J9Z3K7Q2W8XR4M6N0V5BYHEH"
	if err := n.InsertField("id", ulid); err != nil {
		t.Fatalf("InsertField: %v", err)
	}

	want := "---\nid: " + ulid + "\n---\n" + string(src)
	if got := string(n.Serialize()); got != want {
		t.Fatalf("insert not surgical\n--- want ---\n%q\n--- got ---\n%q", want, got)
	}
	if n.Body != string(src) {
		t.Errorf("n.Body = %q, want %q (original body untouched)", n.Body, src)
	}
}

// Round-trip (mirrors render_test.go's TestCreateRoundTrip / sameView
// discipline): after InsertField, both Serialize() and Render() output
// re-parse to an equivalent node — same typed view, stable across repeated
// Serialize/Render calls.
func TestInsertField_RoundTrip(t *testing.T) {
	src := []byte("---\ntype: todo\nstate: open\n---\nDo the thing.\n")
	n, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	const ulid = "01J9Z3K7Q2W8XR4M6N0V5BYHEJ"
	if err := n.InsertField("id", ulid); err != nil {
		t.Fatalf("InsertField: %v", err)
	}

	// parse(serialize(n)) reproduces n's typed view.
	reparsed, err := Parse(n.Serialize())
	if err != nil {
		t.Fatalf("parse(serialize): %v", err)
	}
	sameView(t, "parse(serialize) after InsertField", reparsed, n)
	if got := reparsed.Serialize(); !bytes.Equal(got, n.Serialize()) {
		t.Fatalf("serialize not stable across reparse\n--- want ---\n%q\n--- got ---\n%q", n.Serialize(), got)
	}

	// parse(render(n)) also reproduces n's typed view (the create-path
	// primitive, render.go, must agree with the edit path on the same node).
	rendered := n.Render()
	reparsedFromRender, err := Parse(rendered)
	if err != nil {
		t.Fatalf("parse(render): %v", err)
	}
	sameView(t, "parse(render) after InsertField", reparsedFromRender, n)
}

// ─────────────────────────────────────────────────────────────────────────────
// UpsertField / RemoveField
// ─────────────────────────────────────────────────────────────────────────────

// mustParseStr parses src or fails the test.
func mustParseStr(t *testing.T, src string) *Node {
	t.Helper()
	n, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return n
}

// requireNamedError fails unless err is non-nil and carries both the method
// prefix and the key, so a bare "not implemented" cannot satisfy a refusal test.
func requireNamedError(t *testing.T, err error, method, key string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected an error, got nil", method)
	}
	if !strings.Contains(err.Error(), method+":") || !strings.Contains(err.Error(), key) {
		t.Fatalf("error %q must start with %q and name key %q", err, method+":", key)
	}
}

func TestUpsertField_ScalarSplicesValueOnly(t *testing.T) {
	src := "---\nid: X\ntype: todo\ndeadline: 2026-07-01\nstate: open\n---\nbody\n"
	n := mustParseStr(t, src)
	if err := n.UpsertField("deadline", "2026-08-01"); err != nil {
		t.Fatalf("UpsertField: %v", err)
	}
	want := strings.Replace(src, "2026-07-01", "2026-08-01", 1)
	if got := string(n.Serialize()); got != want {
		t.Fatalf("not value-local\n--- want ---\n%q\n--- got ---\n%q", want, got)
	}
	if n.Props["deadline"] != "2026-08-01" {
		t.Errorf("Props[deadline] = %q", n.Props["deadline"])
	}
}

func TestUpsertField_BareColonGetsSpace(t *testing.T) {
	cases := map[string]string{
		"no_space":       "---\ntype: todo\ndeadline:\nstate: open\n---\nb\n",
		"trailing_space": "---\ntype: todo\ndeadline: \nstate: open\n---\nb\n",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			n := mustParseStr(t, src)
			if err := n.UpsertField("deadline", "2026-08-01"); err != nil {
				t.Fatalf("UpsertField: %v", err)
			}
			want := "---\ntype: todo\ndeadline: 2026-08-01\nstate: open\n---\nb\n"
			if got := string(n.Serialize()); got != want {
				t.Fatalf("want %q, got %q", want, got)
			}
			if n.Props["deadline"] != "2026-08-01" {
				t.Errorf("Props[deadline] = %q", n.Props["deadline"])
			}
		})
	}
}

func TestUpsertField_BlockListCollapsesToOneLine(t *testing.T) {
	src := "---\nid: X\ntype: todo\ndepends-on:\n  - \"[[A]]\"\n  - \"[[B]]\"\nstate: open\n---\nbody\n"
	n := mustParseStr(t, src)
	if err := n.UpsertField("depends-on", `"[[C]]"`); err != nil {
		t.Fatalf("UpsertField: %v", err)
	}
	want := "---\nid: X\ntype: todo\ndepends-on: \"[[C]]\"\nstate: open\n---\nbody\n"
	if got := string(n.Serialize()); got != want {
		t.Fatalf("block not collapsed surgically\n--- want ---\n%q\n--- got ---\n%q", want, got)
	}
	if len(n.Links) != 1 || n.Links[0].Rel != "depends-on" || n.Links[0].To != "C" {
		t.Errorf("Links = %+v, want single depends-on -> C", n.Links)
	}
	re := mustParseStr(t, string(n.Serialize()))
	sameView(t, "reparse after collapse", re, n)
}

func TestUpsertField_AbsentKeyEqualsInsertField(t *testing.T) {
	src := "---\nid: X\ntype: todo\n---\nbody\n"
	a := mustParseStr(t, src)
	b := mustParseStr(t, src)
	if err := a.UpsertField("priority", "B"); err != nil {
		t.Fatalf("UpsertField: %v", err)
	}
	if err := b.InsertField("priority", "B"); err != nil {
		t.Fatalf("InsertField: %v", err)
	}
	if !bytes.Equal(a.Serialize(), b.Serialize()) {
		t.Fatalf("Upsert on absent key differs from InsertField\n%q\n%q", a.Serialize(), b.Serialize())
	}
}

func TestUpsertField_RefusesScalarPlusBlockDuplicate(t *testing.T) {
	src := "---\ntype: todo\ntags: x\ntags:\n  - a\n---\nb\n"
	n := mustParseStr(t, src)
	err := n.UpsertField("tags", "y")
	requireNamedError(t, err, "UpsertField", "tags")
	if string(n.Raw) != src {
		t.Fatalf("Raw mutated despite refusal: %q", n.Raw)
	}
}

// Guard: parse/serialize identity over the shared corpus must keep
// holding while the new field primitives exist.
func TestUpsertField_GuardRoundTripCorpus(t *testing.T) {
	for name, src := range roundtripCorpus {
		t.Run(name, func(t *testing.T) {
			n, err := Parse([]byte(src))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if got := string(n.Serialize()); got != src {
				t.Fatalf("round-trip not byte-identical\n--- want ---\n%q\n--- got ---\n%q", src, got)
			}
		})
	}
}

func TestRemoveField_ScalarLineGone(t *testing.T) {
	cases := map[string]string{
		"plain":            "---\nid: X\ntype: todo\ndeadline: 2026-07-01\nstate: open\n---\nbody\n",
		"trailing_comment": "---\nid: X\ntype: todo\ndeadline: 2026-07-01 # soft\nstate: open\n---\nbody\n",
	}
	want := "---\nid: X\ntype: todo\nstate: open\n---\nbody\n"
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			n := mustParseStr(t, src)
			if err := n.RemoveField("deadline"); err != nil {
				t.Fatalf("RemoveField: %v", err)
			}
			if got := string(n.Serialize()); got != want {
				t.Fatalf("want %q, got %q", want, got)
			}
			if n.HasField("deadline") {
				t.Error("HasField(deadline) still true")
			}
			if _, ok := n.Props["deadline"]; ok {
				t.Error("Props still has deadline")
			}
		})
	}
}

func TestRemoveField_BlockListGone(t *testing.T) {
	src := "---\nid: X\ntype: todo\ntags:\n  - a\n  - b\nstate: open\n---\nbody\n"
	n := mustParseStr(t, src)
	if err := n.RemoveField("tags"); err != nil {
		t.Fatalf("RemoveField: %v", err)
	}
	want := "---\nid: X\ntype: todo\nstate: open\n---\nbody\n"
	if got := string(n.Serialize()); got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
	if _, ok := n.Props["tags"]; ok {
		t.Error("Props still has tags")
	}
}

func TestRemoveField_AbsentKeyIsNoOp(t *testing.T) {
	src := "---\nid: X\ntype: todo\n---\nbody\n"
	n := mustParseStr(t, src)
	if err := n.RemoveField("deadline"); err != nil {
		t.Fatalf("RemoveField on absent key: %v", err)
	}
	if string(n.Raw) != src {
		t.Fatalf("Raw changed on no-op: %q", n.Raw)
	}
}

func TestRemoveField_DuplicatesAllRemoved(t *testing.T) {
	cases := map[string]string{
		"scalar_scalar": "---\nid: X\ntype: todo\ndeadline: 2026-01-01\nstate: open\ndeadline: 2026-02-02\n---\nbody\n",
		"scalar_block":  "---\nid: X\ntype: todo\ndeadline: 2026-01-01\nstate: open\ndeadline:\n  - a\n---\nbody\n",
		"block_scalar":  "---\nid: X\ntype: todo\ndeadline:\n  - a\nstate: open\ndeadline: 2026-02-02\n---\nbody\n",
	}
	want := "---\nid: X\ntype: todo\nstate: open\n---\nbody\n"
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			n := mustParseStr(t, src)
			if err := n.RemoveField("deadline"); err != nil {
				t.Fatalf("RemoveField: %v", err)
			}
			if got := string(n.Serialize()); got != want {
				t.Fatalf("want %q, got %q", want, got)
			}
			re := mustParseStr(t, string(n.Serialize()))
			if re.HasField("deadline") {
				t.Error("reparse still shows deadline")
			}
			if _, ok := re.Props["deadline"]; ok {
				t.Error("reparse Props still has deadline")
			}
		})
	}
}

func TestRemoveField_RefusesEmptyingFrontmatter(t *testing.T) {
	src := "---\ntype: todo\n---\nbody\n"
	n := mustParseStr(t, src)
	err := n.RemoveField("type")
	requireNamedError(t, err, "RemoveField", "type")
	if !strings.Contains(strings.ToLower(err.Error()), "frontmatter") {
		t.Errorf("error %q should explain that frontmatter would be left empty", err)
	}
	if string(n.Raw) != src {
		t.Fatalf("Raw mutated despite refusal: %q", n.Raw)
	}
}

func TestRemoveField_RefValuedLinkGone(t *testing.T) {
	cases := map[string]string{
		"scalar":      "---\nid: X\ntype: todo\ndepends-on: \"[[A]]\"\nstate: open\n---\nbody\n",
		"multi":       "---\nid: X\ntype: todo\ndepends-on: \"[[A]]\", \"[[B]]\"\nstate: open\n---\nbody\n",
		"block_list":  "---\nid: X\ntype: todo\ndepends-on:\n  - \"[[A]]\"\n  - \"[[B]]\"\nstate: open\n---\nbody\n",
		"unquoted_ln": "---\nid: X\ntype: todo\ndepends-on: [[A]]\nstate: open\n---\nbody\n",
	}
	want := "---\nid: X\ntype: todo\nstate: open\n---\nbody\n"
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			n := mustParseStr(t, src)
			if len(n.Links) == 0 {
				t.Fatal("fixture has no links")
			}
			if err := n.RemoveField("depends-on"); err != nil {
				t.Fatalf("RemoveField: %v", err)
			}
			if got := string(n.Serialize()); got != want {
				t.Fatalf("want %q, got %q", want, got)
			}
			if len(n.Links) != 0 {
				t.Errorf("Links = %+v, want none", n.Links)
			}
		})
	}
}

func TestUpsertRemoveField_RefuseContinuationShapes(t *testing.T) {
	cases := []struct {
		name, src, key string
	}{
		{"unindented_list", "---\ntype: todo\ndepends-on:\n- \"[[a]]\"\n- \"[[b]]\"\n---\nb\n", "depends-on"},
		{"comment_in_block", "---\ntype: todo\ndepends-on:\n  - \"[[a]]\"\n  # keep\n  - \"[[b]]\"\n---\nb\n", "depends-on"},
		{"nested_map", "---\ntype: todo\ndeadline:\n  soft: 2026-01-01\n---\nb\n", "deadline"},
		{"folded_scalar", "---\ntype: todo\ndeadline: >\n  2026-01-01\n---\nb\n", "deadline"},
		{"blank_in_block", "---\ntype: todo\ndepends-on:\n  - \"[[a]]\"\n\n  - \"[[b]]\"\n---\nb\n", "depends-on"},
		{"orphan_after_scalar", "---\ntype: todo\ntags:\nk: 1\n  - orphan\n---\nb\n", "k"},
		{"col0_comment_then_indented_item", "---\ntype: todo\ndepends-on:\n  - \"[[tgt]]\"\n# keep\n  - \"[[other]]\"\n---\nb\n", "depends-on"},
		{"col0_comment_then_col0_item", "---\ntype: todo\ndepends-on:\n# keep\n- \"[[tgt]]\"\n---\nb\n", "depends-on"},
		{"col0_comment_then_nested_map", "---\ntype: todo\ndeadline:\n# c\n  soft: 2026-01-01\n---\nb\n", "deadline"},
		{"flow_list_multiline", "---\ntype: todo\ndepends-on: [\n\"[[tgt]]\",\n\"[[other]]\"\n]\n---\nb\n", "depends-on"},
		{"flow_list_continued", "---\ntype: todo\ndepends-on: [\"[[tgt]]\",\n\"[[other]]\"]\n---\nb\n", "depends-on"},
		{"dq_scalar_multiline", "---\ntype: todo\ndeadline: \"2026-01-01\nsoft\"\n---\nb\n", "deadline"},
		{"sq_scalar_multiline", "---\ntype: todo\npriority: 'A\nB'\n---\nb\n", "priority"},
		{"stray_doc_end", "---\ntype: todo\nk: 1\n...\nj: 2\n---\nb\n", "k"},
	}
	for _, c := range cases {
		t.Run(c.name+"/upsert", func(t *testing.T) {
			n := mustParseStr(t, c.src)
			requireNamedError(t, n.UpsertField(c.key, "v"), "UpsertField", c.key)
			if string(n.Raw) != c.src {
				t.Fatalf("Raw mutated despite refusal: %q", n.Raw)
			}
		})
		t.Run(c.name+"/remove", func(t *testing.T) {
			n := mustParseStr(t, c.src)
			requireNamedError(t, n.RemoveField(c.key), "RemoveField", c.key)
			if string(n.Raw) != c.src {
				t.Fatalf("Raw mutated despite refusal: %q", n.Raw)
			}
		})
	}
}

func TestUpsertRemoveField_AcceptsSiblingShapes(t *testing.T) {
	cases := []struct{ name, tail string }{
		{"sibling_after_blank", "\nk: v\n"},
		{"col0_comment_then_sibling", "# note\nk: v\n"},
		{"indented_comment_then_sibling", "  # note\nk: v\n"},
		{"indented_comment_before_fence", "  # note\n"},
		{"spaced_key", "Due Date: 1\n"},
		{"dotted_key", "my.key: 1\n"},
		{"no_space_after_colon", "due:2026-08-01\n"},
		{"tab_after_colon", "k:\tv\n"},
		{"bare_key", "k:\n"},
		{"last_key_before_fence", ""},
		{"blank_before_fence", "\n"},
	}
	for _, c := range cases {
		src := "---\ntype: todo\ndeadline: 2026-01-01\n" + c.tail + "---\nb\n"
		t.Run(c.name+"/upsert", func(t *testing.T) {
			n := mustParseStr(t, src)
			if err := n.UpsertField("deadline", "2026-02-02"); err != nil {
				t.Fatalf("UpsertField refused legitimate shape: %v", err)
			}
			want := strings.Replace(src, "deadline: 2026-01-01", "deadline: 2026-02-02", 1)
			if string(n.Raw) != want {
				t.Fatalf("Raw = %q, want %q", n.Raw, want)
			}
		})
		t.Run(c.name+"/remove", func(t *testing.T) {
			n := mustParseStr(t, src)
			if err := n.RemoveField("deadline"); err != nil {
				t.Fatalf("RemoveField refused legitimate shape: %v", err)
			}
			want := strings.Replace(src, "deadline: 2026-01-01\n", "", 1)
			if string(n.Raw) != want {
				t.Fatalf("Raw = %q, want %q", n.Raw, want)
			}
		})
	}
}

func TestRemoveField_RefusesWhenBodyWouldChange(t *testing.T) {
	src := "---\nk: 1\n---\nintro\n---\nbar\n"
	n := mustParseStr(t, src)
	requireNamedError(t, n.RemoveField("k"), "RemoveField", "k")
	if string(n.Raw) != src {
		t.Fatalf("Raw mutated despite refusal: %q", n.Raw)
	}
}

func TestUpsertField_RefusesValueThatDoesNotReadBack(t *testing.T) {
	for name, value := range map[string]string{
		"extra_key":     "x\nstate: done",
		"fence_inject":  "x\n---\ninjected",
		"trailing_ws":   "x ",
		"empty_to_list": "",
	} {
		t.Run(name, func(t *testing.T) {
			src := "---\ntype: todo\nk: old\n---\nb\n"
			if name == "empty_to_list" {
				src = "---\ntype: todo\nk: old\n  - item\n---\nb\n"
			}
			n := mustParseStr(t, src)
			requireNamedError(t, n.UpsertField("k", value), "UpsertField", "k")
			if string(n.Raw) != src {
				t.Fatalf("Raw mutated despite refusal: %q", n.Raw)
			}
		})
	}
	t.Run("insert_path", func(t *testing.T) {
		src := "---\ntype: todo\n---\nb\n"
		n := mustParseStr(t, src)
		requireNamedError(t, n.UpsertField("k", "x\nstate: done"), "UpsertField", "k")
		if string(n.Raw) != src {
			t.Fatalf("Raw mutated despite refusal: %q", n.Raw)
		}
	})
}
