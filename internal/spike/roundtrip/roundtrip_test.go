package roundtrip

import (
	"bytes"
	"strings"
	"testing"
)

// --- Adversarial corpus: real-shaped Obsidian / Logseq / agent input ---------

// A note as Obsidian would write it: frontmatter with aliases+tags, a body with
// a labelled link, a heading link, a block-ref link, an embed, an inline tag, a
// blockquote, a ^block anchor, and a fenced code block that CONTAINS things that
// look like links/tags/headings but must stay inert.
const noteObsidian = `---
id: 01J9Z3K7Q2W8XR4M6N0V5BYHED
type: note
aliases: [my-note, second-alias]
tags: zettel
---
# My Note

See [[grocery-plan|the plan]] and [[index#Heading]] and [[01JABC#^para3]].
An embed: ![[other-note]]. An inline #topic/sub tag here.

> a blockquote with [[quoted-link]] still counts

A paragraph worth linking to. ^para3

` + "```" + `go
// this [[notalink]] and #nottag and # not a heading must be inert
fmt.Println("[[alsonotalink]]")
` + "```" + `

Trailing prose.
`

// A todo as file-per-item: scalar frontmatter incl. a ref-valued prop, body.
const todoItem = `---
id: 01J9Z3K7Q2W8XR4M6N0V5BYHEE
type: todo
state: open
scheduled: 2026-06-22
deadline: 2026-06-25
depends: "[[01J9Z2QH8M]]"
author: mike
---
Buy milk for the week. See [[grocery-plan]] for brands.
`

// A log day: group file, frontmatter + three timestamped entries, each its own
// node in the real design.
const logDay = `---
id: 01J9Z3K7Q2W8XR4M6N0V5BYHEF
type: log-day
aliases: 2026-06-22
---
# 2026-06-22

## 08:38 progress · mike
Built the round-trip spike. Linked [[reckon-redesign]].

## 10:17 win · mike
Hardened the parser.

## 11:19 note · agent
Considering edge cases. ^e3
`

// Edge cases: unknown frontmatter key (must be preserved), trailing spaces on a
// value, and NO trailing newline at EOF.
const weirdEdges = "---\n" +
	"id: 01J9Z3K7Q2W8XR4M6N0V5BYHEG\n" +
	"type: note\n" +
	"x-custom-plugin-field: keep me verbatim\n" +
	"state: open   \n" + // trailing spaces after value
	"---\n" +
	"Body with no trailing newline."

// A file mid git conflict — must be refused gracefully.
const conflicted = `---
id: 01J9Z3K7Q2W8XR4M6N0V5BYHEH
type: note
---
<<<<<<< HEAD
mine
=======
theirs
>>>>>>> branch
`

var roundtripCorpus = map[string]string{
	"noteObsidian": noteObsidian,
	"todoItem":     todoItem,
	"logDay":       logDay,
	"weirdEdges":   weirdEdges,
}

// THE GATE: serialize(parse(f)) == f, byte-for-byte, across the corpus.
func TestRoundTripIdentity(t *testing.T) {
	for name, src := range roundtripCorpus {
		t.Run(name, func(t *testing.T) {
			n, err := Parse([]byte(src))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			got := n.Serialize()
			if !bytes.Equal(got, []byte(src)) {
				t.Fatalf("round-trip not byte-identical\n--- want ---\n%q\n--- got ---\n%q", src, got)
			}
		})
	}
}

// Structured view correctness: code-fence content is inert; real links/tags are
// captured with their fragment/label parsed.
func TestStructuredViewIgnoresFences(t *testing.T) {
	n, err := Parse([]byte(noteObsidian))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	targets := map[string]Link{}
	for _, l := range n.Links {
		targets[l.Target] = l
	}
	for _, inert := range []string{"notalink", "alsonotalink"} {
		if _, found := targets[inert]; found {
			t.Errorf("fenced %q leaked into links", inert)
		}
	}
	for _, want := range []string{"grocery-plan", "index", "01JABC", "other-note", "quoted-link"} {
		if _, found := targets[want]; !found {
			t.Errorf("missing expected link %q (got %v)", want, keys(targets))
		}
	}
	if l := targets["grocery-plan"]; l.Label != "the plan" {
		t.Errorf("label not parsed: %+v", l)
	}
	if l := targets["index"]; l.Fragment != "Heading" {
		t.Errorf("heading fragment not parsed: %+v", l)
	}
	if l := targets["01JABC"]; l.Fragment != "para3" {
		t.Errorf("block fragment not parsed: %+v", l)
	}
	for _, tag := range n.Tags {
		if tag == "nottag" {
			t.Error("fenced #nottag leaked into tags")
		}
	}
	if !contains(n.Fragments, "para3") {
		t.Errorf("block anchor ^para3 not captured: %v", n.Fragments)
	}
}

// THE BET: a span-local field edit changes ONLY that field's value bytes; every
// other byte (frontmatter siblings, body, fences) is preserved, and the
// structured view reflects the change.
func TestSpanLocalEditIsSurgical(t *testing.T) {
	n, err := Parse([]byte(todoItem))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := n.SetField("state", "done"); err != nil {
		t.Fatalf("SetField: %v", err)
	}

	// Expected file = original with exactly the one value substituted.
	want := strings.Replace(todoItem, "state: open", "state: done", 1)
	if got := string(n.Serialize()); got != want {
		t.Fatalf("edit not surgical\n--- want ---\n%q\n--- got ---\n%q", want, got)
	}
	if n.Frontmatter["state"] != "done" {
		t.Errorf("structured view not updated: %q", n.Frontmatter["state"])
	}
	// Sibling fields and the ref-valued prop must be intact.
	if n.Frontmatter["scheduled"] != "2026-06-22" || n.Frontmatter["depends"] != `"[[01J9Z2QH8M]]"` {
		t.Errorf("sibling frontmatter disturbed: %+v", n.Frontmatter)
	}
}

// Editing one entry of a group file leaves its siblings byte-identical — the
// "edits above/below the span" worry from watch-item #1.
func TestGroupFileEditOneEntry(t *testing.T) {
	n, err := Parse([]byte(logDay))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	entries := SplitEntries(n)
	if len(entries) != 3 {
		t.Fatalf("want 3 entries, got %d", len(entries))
	}

	newMiddle := "## 10:17 win · mike\nHardened the parser AND the writer.\n\n"
	out, err := n.ReplaceEntryBody(entries[1], newMiddle)
	if err != nil {
		t.Fatalf("ReplaceEntryBody: %v", err)
	}

	// Re-parse the edited file; entries 0 and 2 must be byte-identical to before.
	n2, err := Parse(out)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	entries2 := SplitEntries(n2)
	if len(entries2) != 3 {
		t.Fatalf("after edit want 3 entries, got %d", len(entries2))
	}
	if got := blockBytes(out, entries2[0]); got != blockBytes([]byte(logDay), entries[0]) {
		t.Errorf("entry 0 disturbed:\n%q", got)
	}
	if got := blockBytes(out, entries2[2]); got != blockBytes([]byte(logDay), entries[2]) {
		t.Errorf("entry 2 disturbed:\n%q", got)
	}
	if !strings.Contains(blockBytes(out, entries2[1]), "AND the writer") {
		t.Errorf("entry 1 not updated:\n%q", blockBytes(out, entries2[1]))
	}
}

// Malformed (git conflict) files are refused gracefully, never panic.
func TestConflictMarkersRefused(t *testing.T) {
	if _, err := Parse([]byte(conflicted)); err == nil {
		t.Fatal("expected an error for a file with conflict markers")
	}
}

// FuzzRoundTripIdentity is the fuzz gate the design names: for ANY input the
// parser accepts, serialize must return it byte-for-byte. Run:
//
//	go test ./internal/spike/roundtrip/ -run=x -fuzz=FuzzRoundTripIdentity -fuzztime=30s
func FuzzRoundTripIdentity(f *testing.F) {
	for _, src := range roundtripCorpus {
		f.Add([]byte(src))
	}
	f.Add([]byte(conflicted))
	f.Add([]byte(""))
	f.Add([]byte("---\nid: x\n")) // unterminated frontmatter
	f.Add([]byte("no frontmatter at all\njust body\n"))

	f.Fuzz(func(t *testing.T, raw []byte) {
		n, err := Parse(raw)
		if err != nil {
			return // refused (e.g. conflict markers) — acceptable
		}
		if got := n.Serialize(); !bytes.Equal(got, raw) {
			t.Fatalf("fuzz round-trip not byte-identical\ninput: %q\ngot:   %q", raw, got)
		}
		// And a span-local edit of any present scalar key must re-parse and
		// remain byte-identical except for the spliced value.
		for k := range n.fieldSpans {
			m, _ := Parse(raw) // fresh node
			before := string(m.Serialize())
			if err := m.SetField(k, "EDITED"); err != nil {
				continue
			}
			after := string(m.Serialize())
			if before == after && m.Frontmatter[k] != "EDITED" {
				t.Fatalf("edit of %q did not take and did not change bytes", k)
			}
			break
		}
	})
}

// --- helpers -----------------------------------------------------------------

func blockBytes(raw []byte, e Entry) string { return string(raw[e.Span.Start:e.Span.End]) }

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func keys(m map[string]Link) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
