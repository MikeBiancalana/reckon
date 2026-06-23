package node

import (
	"bytes"
	"strings"
	"testing"
)

// --- Adversarial corpus: real-shaped Obsidian / Logseq / agent input ---------
// Carried verbatim from the gating spike (internal/spike/roundtrip). These are
// the contract: any change to the parser must keep them green.

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

const weirdEdges = "---\n" +
	"id: 01J9Z3K7Q2W8XR4M6N0V5BYHEG\n" +
	"type: note\n" +
	"x-custom-plugin-field: keep me verbatim\n" +
	"state: open   \n" +
	"---\n" +
	"Body with no trailing newline."

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

// AC1 — THE GATE: serialize(parse(f)) == f, byte-for-byte, across the corpus.
func TestRoundTripIdentity(t *testing.T) {
	for name, src := range roundtripCorpus {
		t.Run(name, func(t *testing.T) {
			n, err := Parse([]byte(src))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if got := n.Serialize(); !bytes.Equal(got, []byte(src)) {
				t.Fatalf("round-trip not byte-identical\n--- want ---\n%q\n--- got ---\n%q", src, got)
			}
		})
	}
}

// AC8 — structured view: code-fence content inert; real links captured with
// fragment/label parsed; rel routing (body -> references).
func TestStructuredViewIgnoresFences(t *testing.T) {
	n, err := Parse([]byte(noteObsidian))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	byTarget := map[string]Link{}
	for _, l := range n.Links {
		byTarget[l.To] = l
	}
	for _, inert := range []string{"notalink", "alsonotalink"} {
		if _, found := byTarget[inert]; found {
			t.Errorf("fenced %q leaked into links", inert)
		}
	}
	for _, want := range []string{"grocery-plan", "index", "01JABC", "other-note", "quoted-link"} {
		l, found := byTarget[want]
		if !found {
			t.Errorf("missing expected link %q (got %v)", want, linkTargets(n.Links))
			continue
		}
		if l.Rel != "references" {
			t.Errorf("body link %q rel = %q, want references", want, l.Rel)
		}
	}
	if l := byTarget["grocery-plan"]; l.To != "grocery-plan" {
		t.Errorf("label/target not parsed: %+v", l)
	}
	if l := byTarget["index"]; l.ToFrag != "Heading" {
		t.Errorf("heading fragment not parsed: %+v", l)
	}
	if l := byTarget["01JABC"]; l.ToFrag != "para3" {
		t.Errorf("block fragment not parsed: %+v", l)
	}
	if !hasFragment(n.Fragments, "para3") {
		t.Errorf("block anchor ^para3 not captured: %+v", n.Fragments)
	}
}

// AC6/7 — canonical typed view: reserved keys mapped, props exclude reserved +
// ref-valued, ref-valued prop routed to a typed link.
func TestCanonicalView(t *testing.T) {
	n, err := Parse([]byte(todoItem))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if n.ULID != "01J9Z3K7Q2W8XR4M6N0V5BYHEE" {
		t.Errorf("ULID = %q", n.ULID)
	}
	if n.Type != "todo" {
		t.Errorf("Type = %q", n.Type)
	}
	if n.Author != "mike" {
		t.Errorf("Author = %q", n.Author)
	}
	// props: non-reserved scalars only; reserved + ref-valued excluded.
	for _, reserved := range []string{"id", "type", "author", "depends"} {
		if _, ok := n.Props[reserved]; ok {
			t.Errorf("props leaked %q: %v", reserved, n.Props)
		}
	}
	if n.Props["state"] != "open" || n.Props["scheduled"] != "2026-06-22" || n.Props["deadline"] != "2026-06-25" {
		t.Errorf("props wrong: %v", n.Props)
	}
	// ref-valued prop -> typed link with rel = prop key.
	var dep *Link
	for i := range n.Links {
		if n.Links[i].Rel == "depends" {
			dep = &n.Links[i]
		}
	}
	if dep == nil || dep.To != "01J9Z2QH8M" {
		t.Errorf("depends link wrong: %+v (links=%v)", dep, n.Links)
	}
}

// AC6 — aliases parse from both scalar and [list] forms.
func TestAliasParsing(t *testing.T) {
	list, err := Parse([]byte(noteObsidian))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(list.Aliases) != 2 || list.Aliases[0] != "my-note" || list.Aliases[1] != "second-alias" {
		t.Errorf("list aliases = %v", list.Aliases)
	}
	scalar, err := Parse([]byte(logDay))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(scalar.Aliases) != 1 || scalar.Aliases[0] != "2026-06-22" {
		t.Errorf("scalar aliases = %v", scalar.Aliases)
	}
}

// AC4 — THE BET: a span-local field edit changes ONLY that field's value bytes.
func TestSpanLocalEditIsSurgical(t *testing.T) {
	n, err := Parse([]byte(todoItem))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := n.SetField("state", "done"); err != nil {
		t.Fatalf("SetField: %v", err)
	}
	want := strings.Replace(todoItem, "state: open", "state: done", 1)
	if got := string(n.Serialize()); got != want {
		t.Fatalf("edit not surgical\n--- want ---\n%q\n--- got ---\n%q", want, got)
	}
	if n.Props["state"] != "done" {
		t.Errorf("view not updated: %q", n.Props["state"])
	}
	if n.Props["scheduled"] != "2026-06-22" {
		t.Errorf("sibling disturbed: %v", n.Props)
	}
}

// SetField on a missing/non-scalar key errors (spike scope contract).
func TestSetFieldMissingKey(t *testing.T) {
	n, err := Parse([]byte(todoItem))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := n.SetField("nonexistent", "x"); err == nil {
		t.Fatal("expected error setting a missing field")
	}
}

// Editing one entry of a group file leaves its siblings byte-identical.
func TestGroupFileEditOneEntry(t *testing.T) {
	n, err := Parse([]byte(logDay))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	entries := n.SplitEntries()
	if len(entries) != 3 {
		t.Fatalf("want 3 entries, got %d", len(entries))
	}
	newMiddle := "## 10:17 win · mike\nHardened the parser AND the writer.\n\n"
	out, err := n.ReplaceEntryBody(entries[1], newMiddle)
	if err != nil {
		t.Fatalf("ReplaceEntryBody: %v", err)
	}
	n2, err := Parse(out)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	entries2 := n2.SplitEntries()
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

// MarkdownParser implements the per-tool Parser pair and round-trips.
func TestMarkdownParserRoundTrip(t *testing.T) {
	p := MarkdownParser{}
	nodes, err := p.Parse([]byte(todoItem), Loc{File: "todos/x.md"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("want 1 node, got %d", len(nodes))
	}
	if nodes[0].Loc.File != "todos/x.md" {
		t.Errorf("loc not set: %+v", nodes[0].Loc)
	}
	out, err := p.Serialize(nodes[0])
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	if !bytes.Equal(out, []byte(todoItem)) {
		t.Errorf("parser round-trip not byte-identical:\n%q", out)
	}
}

// AC2 — fuzz gate: for ANY input Parse accepts, Serialize is byte-identical, and
// a span edit of any present scalar re-parses and stays byte-identical bar the
// spliced value.
func FuzzRoundTripIdentity(f *testing.F) {
	for _, src := range roundtripCorpus {
		f.Add([]byte(src))
	}
	f.Add([]byte(conflicted))
	f.Add([]byte(""))
	f.Add([]byte("---\nid: x\n"))
	f.Add([]byte("no frontmatter at all\njust body\n"))

	f.Fuzz(func(t *testing.T, raw []byte) {
		n, err := Parse(raw)
		if err != nil {
			return // refused — acceptable
		}
		if got := n.Serialize(); !bytes.Equal(got, raw) {
			t.Fatalf("fuzz round-trip not byte-identical\ninput: %q\ngot:   %q", raw, got)
		}
		for k := range n.fieldSpans {
			m, _ := Parse(raw)
			before := string(m.Serialize())
			if err := m.SetField(k, "EDITED"); err != nil {
				continue
			}
			after := string(m.Serialize())
			if before == after && m.Props[k] != "EDITED" {
				t.Fatalf("edit of %q did not take and did not change bytes", k)
			}
			break
		}
	})
}

// --- helpers -----------------------------------------------------------------

func blockBytes(raw []byte, e Entry) string { return string(raw[e.Span.Start:e.Span.End]) }

func hasFragment(fs []Fragment, id string) bool {
	for _, fr := range fs {
		if fr.ID == id {
			return true
		}
	}
	return false
}

func linkTargets(ls []Link) []string {
	var out []string
	for _, l := range ls {
		out = append(out, l.To)
	}
	return out
}
