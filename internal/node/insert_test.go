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
