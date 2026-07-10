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

// blockAliases (AC b): Obsidian Properties-panel-shaped block-style `aliases`
// list with a flow-style `tags` sibling in the same frontmatter block, LF
// line endings.
const blockAliases = `---
id: 01J9Z3K7Q2W8XR4M6N0V5BYHEI
type: note
aliases:
  - project-x
  - proj-x
tags: [x, y]
---
# Project X
`

// crlfNote (AC c): an entire file using \r\n line endings throughout —
// frontmatter delimiters, field lines, and body.
const crlfNote = "---\r\n" +
	"id: 01J9Z3K7Q2W8XR4M6N0V5BYHEJ\r\n" +
	"type: note\r\n" +
	"aliases: [x]\r\n" +
	"---\r\n" +
	"# Title\r\n" +
	"\r\n" +
	"Body text with a [[link]].\r\n"

// inlineCodeNote (AC d): a single-backtick inline code span containing a
// wikilink-shaped string, plus a real wikilink elsewhere in the body.
const inlineCodeNote = "---\n" +
	"id: 01J9Z3K7Q2W8XR4M6N0V5BYHEK\n" +
	"type: note\n" +
	"---\n" +
	"Use the `[[not-a-link]]` syntax to link notes.\n" +
	"\n" +
	"See [[real-target]] for the real thing.\n"

// multiTargetDepends (AC e): a ref-valued frontmatter prop with two
// comma-separated wikilink targets on one scalar line.
const multiTargetDepends = `---
id: 01J9Z3K7Q2W8XR4M6N0V5BYHEL
type: todo
depends: [[A]], [[B]]
---
Body text.
`

// logDayWithIDs (T4, internal/node/logparser.go): a log-day group file whose
// entries each carry an inline `id:: <ULID>` line -- the per-entry-identity
// marker the log tool's group parser (LogParser, logparser_test.go) reads.
// Included in roundtripCorpus to prove an id:: line is inert to the CORE
// parser (parseFrontmatter/extractBody have no `::` handling at all -- see
// node.go's FORMAT COUPLING doc comment) and survives byte-for-byte, exactly
// like any other body content, through TestRoundTripIdentity and
// FuzzRoundTripIdentity below.
const logDayWithIDs = `---
id: 01J9Z3K7Q2W8XR4M6N0V5BYHEQ
type: log-day
aliases: 2026-07-05
---
# 2026-07-05

## 08:38 progress · mike
id:: 01J9Z3K7Q2W8XR4M6N0V5BYHER
Built the round-trip spike. Linked [[reckon-redesign]].

## 10:17 win · mike
id:: 01J9Z3K7Q2W8XR4M6N0V5BYHES
Hardened the parser.
`

// logDayWithDid (v1-T6, internal/node/logparser.go): a log-day group file
// whose single entry carries BOTH an `id:: <ULID>` line and a `did::
// <ULID>` line immediately after it -- the recurrence audit marker
// (logparser_test.go's TestLogParser_ParsesDidMarkerIntoLink /
// TestLogParser_DidRoundTripIdentity) that LogParser turns into a
// Link{Rel:"did"} edge on the entry node. Included in roundtripCorpus,
// mirroring logDayWithIDs immediately above, to prove a did:: line is
// exactly as inert to the CORE parser as an id:: line (parseFrontmatter/
// extractBody have no `::` handling at all) and survives byte-for-byte
// through TestRoundTripIdentity and FuzzRoundTripIdentity below.
const logDayWithDid = `---
id: 01J9Z3K7Q2W8XR4M6N0V5BYHFG
type: log-day
aliases: 2026-07-05
---
# 2026-07-05

## 09:15 · mike
id:: 01J9Z3K7Q2W8XR4M6N0V5BYHFH
did:: 01J9Z3K7Q2W8XR4M6N0V5BYHFI
completed recurring todo 01J9Z3K7Q2W8XR4M6N0V5BYHFI (repeat +7d); advanced scheduled 2026-07-05 → 2026-07-12
`

var roundtripCorpus = map[string]string{
	"noteObsidian":       noteObsidian,
	"todoItem":           todoItem,
	"logDay":             logDay,
	"logDayWithIDs":      logDayWithIDs,
	"logDayWithDid":      logDayWithDid,
	"weirdEdges":         weirdEdges,
	"blockAliases":       blockAliases,
	"crlfNote":           crlfNote,
	"inlineCodeNote":     inlineCodeNote,
	"multiTargetDepends": multiTargetDepends,
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
	f.Add([]byte(crlfNote))     // CRLF-throughout seed
	f.Add([]byte(blockAliases)) // block-scalar YAML list seed

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

// TestBlockScalarAliases (AC b): Obsidian-shaped block-style `aliases:` list
// must parse to the same typed result as the flow form, without disturbing a
// sibling flow-style key in the same frontmatter block.
//
// Today's bug: the bare `aliases:` line matches fmScalarRe as an empty-value
// scalar, and the following `  - item` continuation lines don't match
// fmScalarRe at all (no leading key char) — so they're silently skipped and
// deriveView calls parseAliases("") -> nil. n.Aliases comes back empty, no
// error, indistinguishable from "no aliases specified."
func TestBlockScalarAliases(t *testing.T) {
	t.Run("multi_item_block_list_with_sibling_flow_key", func(t *testing.T) {
		n, err := Parse([]byte(blockAliases))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if len(n.Aliases) != 2 || n.Aliases[0] != "project-x" || n.Aliases[1] != "proj-x" {
			t.Errorf("block aliases = %v, want [project-x proj-x]", n.Aliases)
		}
		if n.Props["tags"] != "[x, y]" {
			t.Errorf("sibling flow key disturbed: Props[tags] = %q", n.Props["tags"])
		}
	})

	t.Run("single_item_block_list", func(t *testing.T) {
		src := "---\n" +
			"id: 01J9Z3K7Q2W8XR4M6N0V5BYHEM\n" +
			"type: note\n" +
			"aliases:\n" +
			"  - solo-alias\n" +
			"---\n" +
			"Body.\n"
		n, err := Parse([]byte(src))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if len(n.Aliases) != 1 || n.Aliases[0] != "solo-alias" {
			t.Errorf("single-item block alias = %v, want [solo-alias]", n.Aliases)
		}
	})

	// Regression: aliases: immediately followed by another key with no `- `
	// continuation line must NOT start erroring or fabricating entries — this
	// case already yields nil today and must keep doing so.
	t.Run("empty_block_list_stays_nil", func(t *testing.T) {
		src := "---\n" +
			"id: 01J9Z3K7Q2W8XR4M6N0V5BYHEN\n" +
			"type: note\n" +
			"aliases:\n" +
			"tags: zettel\n" +
			"---\n" +
			"Body.\n"
		n, err := Parse([]byte(src))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if len(n.Aliases) != 0 {
			t.Errorf("empty `aliases:` = %v, want nil/empty (regression)", n.Aliases)
		}
		if n.Props["tags"] != "zettel" {
			t.Errorf("sibling key after empty aliases: disturbed: %q", n.Props["tags"])
		}
	})
}

// TestCRLFFrontmatter (AC c): a whole-file CRLF note must populate typed
// fields exactly like the LF equivalent, not silently degrade to "entire file
// becomes body."
//
// Today's bug: parseFrontmatter requires bytes.HasPrefix(raw, "---\n"); a CRLF
// file's first four bytes are '-','-','-','\r', so the prefix check fails
// outright, bodyStart stays 0, and every typed field (ULID/Type/Author/
// Aliases/Props) comes back zero with no error.
func TestCRLFFrontmatter(t *testing.T) {
	t.Run("whole_file_crlf_populates_typed_fields", func(t *testing.T) {
		n, err := Parse([]byte(crlfNote))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if n.ULID != "01J9Z3K7Q2W8XR4M6N0V5BYHEJ" {
			t.Errorf("ULID = %q, want populated", n.ULID)
		}
		if n.Type != "note" {
			t.Errorf("Type = %q, want %q", n.Type, "note")
		}
		if len(n.Aliases) != 1 || n.Aliases[0] != "x" {
			t.Errorf("Aliases = %v, want [x]", n.Aliases)
		}
		for _, v := range []string{n.ULID, n.Type, n.Author, n.Time} {
			if strings.HasSuffix(v, "\r") {
				t.Errorf("captured field value has a stray trailing \\r: %q", v)
			}
		}
		var found bool
		for _, l := range n.Links {
			if l.To == "link" {
				found = true
			}
		}
		if !found {
			t.Errorf("body [[link]] did not produce a link: %v", n.Links)
		}
	})

	// Regression: CRLF *inside the body only* (clean-LF frontmatter) already
	// works today — must not break.
	t.Run("crlf_only_in_body_regression", func(t *testing.T) {
		src := "---\n" +
			"id: 01J9Z3K7Q2W8XR4M6N0V5BYHEO\n" +
			"type: note\n" +
			"---\n" +
			"Line one.\r\n" +
			"Line two with a [[crlf-link]].\r\n"
		n, err := Parse([]byte(src))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if n.ULID != "01J9Z3K7Q2W8XR4M6N0V5BYHEO" || n.Type != "note" {
			t.Errorf("LF frontmatter with CRLF body failed to parse: ULID=%q Type=%q", n.ULID, n.Type)
		}
		wantBody := "Line one.\r\nLine two with a [[crlf-link]].\r\n"
		if n.Body != wantBody {
			t.Errorf("CRLF body bytes not preserved verbatim:\n got %q\n want %q", n.Body, wantBody)
		}
	})

	// After parsing a CRLF note, SetField on an existing scalar field must
	// still splice a valid Raw offset (spans computed during CRLF-aware
	// scanning must remain correct byte offsets into Raw).
	t.Run("setfield_after_crlf_parse_stays_surgical", func(t *testing.T) {
		n, err := Parse([]byte(crlfNote))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if err := n.SetField("type", "task"); err != nil {
			t.Fatalf("SetField on a CRLF-parsed node: %v", err)
		}
		want := strings.Replace(crlfNote, "type: note\r\n", "type: task\r\n", 1)
		if got := string(n.Serialize()); got != want {
			t.Fatalf("CRLF SetField not surgical\n--- want ---\n%q\n--- got ---\n%q", want, got)
		}
		if n.Type != "task" {
			t.Errorf("view not updated after CRLF SetField: %q", n.Type)
		}
	})
}

// TestInlineCodeInert (AC d): a wikilink-shaped string inside inline code
// (single- or double-backtick span) must not produce a link — no escape
// hatch for this one, per the ticket.
//
// Today's bug: extractBody only tracks fenced (``` / ~~~) blocks via inFence;
// it has no notion of an inline backtick span and runs wikilinkRe against the
// raw line text unconditionally.
func TestInlineCodeInert(t *testing.T) {
	t.Run("single_backtick_span_is_inert", func(t *testing.T) {
		body := "Use the `[[target]]` syntax to link notes.\n"
		n, err := Parse([]byte(body))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		for _, l := range n.Links {
			if l.To == "target" {
				t.Errorf("inline code `[[target]]` leaked into links: %+v", n.Links)
			}
		}
	})

	t.Run("double_backtick_span_with_nested_backtick_is_inert", func(t *testing.T) {
		body := "See ``a [[nested-target]] with ` backtick`` here.\n"
		n, err := Parse([]byte(body))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		for _, l := range n.Links {
			if l.To == "nested-target" {
				t.Errorf("double-backtick span (with a nested single backtick) leaked into links: %+v", n.Links)
			}
		}
	})

	t.Run("real_link_adjacent_to_code_span_still_linkified", func(t *testing.T) {
		body := "See `[[masked]]` and [[real-link]] right after.\n"
		n, err := Parse([]byte(body))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		var foundReal bool
		for _, l := range n.Links {
			if l.To == "masked" {
				t.Errorf("code span leaked into links: %+v", n.Links)
			}
			if l.To == "real-link" {
				foundReal = true
			}
		}
		if !foundReal {
			t.Errorf("adjacent real link not linkified: %v", n.Links)
		}
	})

	t.Run("unterminated_backtick_does_not_blind_rest_of_line", func(t *testing.T) {
		body := "A stray ` backtick with no close and a [[stray-real-link]] here.\n"
		n, err := Parse([]byte(body))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		var found bool
		for _, l := range n.Links {
			if l.To == "stray-real-link" {
				found = true
			}
		}
		if !found {
			t.Errorf("real link after an unterminated backtick not linkified: %v", n.Links)
		}
	})

	// Regression: fenced code blocks must stay inert (already passing today).
	t.Run("fenced_code_block_regression", func(t *testing.T) {
		n, err := Parse([]byte(noteObsidian))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		for _, inert := range []string{"notalink", "alsonotalink"} {
			for _, l := range n.Links {
				if l.To == inert {
					t.Errorf("fenced %q leaked into links", inert)
				}
			}
		}
	})
}

// TestMultiTargetRefProp (AC e): a ref-valued frontmatter prop with several
// comma-separated wikilink targets on one scalar line must parse into one
// Link per target (same Rel), and a mixed valid/invalid value must never
// fabricate a garbage Link.
//
// Today's bug: refValRe = ^\[\[(.+?)\]\]$ is anchored to the whole trimmed
// value. For "[[A]], [[B]]" the only way \]\]$ can match is against the
// string's final "]]", forcing the lazy capture group to swallow everything
// in between — producing one corrupted Link{To: "A]], [[B"} instead of two
// clean links (or a clean drop).
func TestMultiTargetRefProp(t *testing.T) {
	depends := func(t *testing.T, val string) *Node {
		t.Helper()
		src := "---\n" +
			"id: 01J9Z3K7Q2W8XR4M6N0V5BYHEP\n" +
			"type: todo\n" +
			"depends: " + val + "\n" +
			"---\n" +
			"Body.\n"
		n, err := Parse([]byte(src))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		return n
	}
	dependsLinks := func(n *Node) []Link {
		var out []Link
		for _, l := range n.Links {
			if l.Rel == "depends" {
				out = append(out, l)
			}
		}
		return out
	}

	t.Run("two_targets", func(t *testing.T) {
		links := dependsLinks(depends(t, "[[A]], [[B]]"))
		if len(links) != 2 {
			t.Fatalf("want 2 depends links, got %d: %v", len(links), links)
		}
		if links[0].To != "A" || links[1].To != "B" {
			t.Errorf("targets wrong: %+v", links)
		}
	})

	t.Run("three_targets", func(t *testing.T) {
		links := dependsLinks(depends(t, "[[A]], [[B]], [[C]]"))
		if len(links) != 3 {
			t.Fatalf("want 3 depends links, got %d: %v", len(links), links)
		}
		for i, want := range []string{"A", "B", "C"} {
			if links[i].To != want {
				t.Errorf("target[%d] = %q, want %q (links=%v)", i, links[i].To, want, links)
			}
		}
	})

	t.Run("comma_spacing_variants_agree", func(t *testing.T) {
		variants := []string{"[[A]],[[B]]", "[[A]], [[B]]", "[[A]] ,  [[B]]"}
		for _, v := range variants {
			links := dependsLinks(depends(t, v))
			if len(links) != 2 || links[0].To != "A" || links[1].To != "B" {
				t.Errorf("variant %q produced wrong links: %v", v, links)
			}
		}
	})

	t.Run("per_token_modifiers", func(t *testing.T) {
		links := dependsLinks(depends(t, "[[A#heading]], [[B|label]]"))
		if len(links) != 2 {
			t.Fatalf("want 2 depends links, got %d: %v", len(links), links)
		}
		if links[0].To != "A" || links[0].ToFrag != "heading" {
			t.Errorf("first target wrong: %+v", links[0])
		}
		if links[1].To != "B" || links[1].ToFrag != "" {
			t.Errorf("second target wrong: %+v", links[1])
		}
	})

	// Regression: single-target ref props must keep producing exactly one
	// link, unchanged.
	t.Run("single_target_regression", func(t *testing.T) {
		links := dependsLinks(depends(t, "[[X]]"))
		if len(links) != 1 || links[0].To != "X" {
			t.Fatalf("single-target regression broke: %v", links)
		}
	})

	// The mandatory floor regardless of parse strategy: a mixed valid/invalid
	// value must never fabricate a garbage Link.
	t.Run("mixed_valid_invalid_no_garbage_link", func(t *testing.T) {
		n := depends(t, "[[A]], not-a-link")
		links := dependsLinks(n)
		if len(links) != 0 {
			t.Errorf("expected no depends links for a mixed valid/invalid value, got %v", links)
		}
		for _, l := range n.Links {
			if strings.Contains(l.To, "]]") || strings.Contains(l.To, ",") {
				t.Errorf("garbage link fabricated: %+v", l)
			}
		}
		if _, ok := n.Props["depends"]; !ok {
			t.Errorf("mixed value should fall through to Props, got Props=%v", n.Props)
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

// --- TestSetAliases_* (reckon-ih5g, v1-T8) -----------------------------------
//
// SetAliases is the one new node-package primitive this ticket adds: it must
// upsert `aliases` across all three real-world shapes (flow scalar, bare
// scalar, absent) plus the one shape today's InsertField/SetField pair
// silently corrupts -- Obsidian's block-style indented list (verified
// separately, ticket-work/reckon-ih5g/acceptance-criteria.md §2.4). Today
const aliasFlowScalar = `---
id: 01J9Z3K7Q2W8XR4M6N0V5BYHFM
type: note
aliases: [old-title]
---
Body text.
`

const aliasBareScalar = `---
id: 01J9Z3K7Q2W8XR4M6N0V5BYHFN
type: note
aliases: old-title
---
Body text.
`

const aliasAbsent = `---
id: 01J9Z3K7Q2W8XR4M6N0V5BYHFP
type: note
---
Body text.
`

// blockAliasesAtEOF: the block list is the LAST frontmatter key, so the
// block's span ends exactly at the closing delimiter (End == closeAbs) --
// locks in the EOF span math for SetAliases' whole-block splice.
const blockAliasesAtEOF = `---
id: 01J9Z3K7Q2W8XR4M6N0V5BYHFQ
type: note
tags: [x, y]
aliases:
  - project-x
  - proj-x
---
# Project X
`

// blockAliasesCRLF: an entire file using \r\n line endings with a block-style
// aliases: list -- exercises SetAliases' splice against CRLF-recorded spans.
const blockAliasesCRLF = "---\r\n" +
	"id: 01J9Z3K7Q2W8XR4M6N0V5BYHFR\r\n" +
	"type: note\r\n" +
	"aliases:\r\n" +
	"  - project-x\r\n" +
	"tags: [x, y]\r\n" +
	"---\r\n" +
	"# Project X\r\n"

func TestSetAliases_BlockListCollapsesToFlow(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		newList []string
	}{
		{"flow_scalar", aliasFlowScalar, []string{"old-title", "new-title"}},
		{"bare_scalar", aliasBareScalar, []string{"old-title", "new-title"}},
		{"absent", aliasAbsent, []string{"new-title"}},
		// blockAliases (declared above, in the roundtripCorpus section):
		// Obsidian Properties-panel block-style `aliases:` list with a
		// sibling flow-style `tags` key in the same frontmatter block -- the
		// verified corruption repro (acceptance-criteria.md §2.4).
		{"block_list", blockAliases, []string{"project-x", "proj-x", "new-title"}},
		{"block_list_at_eof", blockAliasesAtEOF, []string{"project-x", "proj-x", "new-title"}},
		{"block_list_crlf", blockAliasesCRLF, []string{"project-x", "new-title"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n, err := Parse([]byte(tc.src))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if err := n.SetAliases(tc.newList); err != nil {
				t.Fatalf("SetAliases: %v", err)
			}

			got := n.Serialize()
			if !strings.Contains(string(got), "aliases: [") {
				t.Errorf("expected a canonical flow aliases line (aliases: [...]), got:\n%q", got)
			}
			if strings.Count(string(got), "aliases:") > 1 {
				t.Errorf("duplicate aliases: key present, got:\n%q", got)
			}
			if !sameStringSlice(n.Aliases, tc.newList) {
				t.Errorf("Aliases = %v, want %v", n.Aliases, tc.newList)
			}

			// A sibling flow key in the same frontmatter block must survive
			// untouched (after the block in block_list, before it in
			// block_list_at_eof).
			if (tc.name == "block_list" || tc.name == "block_list_at_eof") && !strings.Contains(string(got), "tags: [x, y]") {
				t.Errorf("sibling `tags: [x, y]` disturbed by SetAliases, got:\n%q", got)
			}
			if tc.name == "block_list_crlf" && !strings.Contains(string(got), "tags: [x, y]\r\n") {
				t.Errorf("CRLF sibling `tags: [x, y]` disturbed by SetAliases, got:\n%q", got)
			}

			// The edit must stay round-trip-stable: re-parsing the edited
			// bytes must reproduce them exactly (the byte-preservation
			// invariant extended past this mutation).
			reparsed, err := Parse(got)
			if err != nil {
				t.Fatalf("re-parse after SetAliases: %v", err)
			}
			if !bytes.Equal(reparsed.Serialize(), got) {
				t.Errorf("post-SetAliases round trip not stable:\n--- got ---\n%q\n--- reparsed.Serialize() ---\n%q", got, reparsed.Serialize())
			}
		})
	}

	// Confirm no roundtripCorpus regression: SetAliases must not be
	// implemented by mutating shared parser state that the corpus depends
	// on. TestRoundTripIdentity (above) already gates this on every `go
	// test`; re-checking it inline here (not as a separate t.Run, so it
	// contributes to this same test's pass/fail rather than reporting its
	// own independent PASS line) ties the two gates together explicitly for
	// this ticket's reviewers.
	for name, src := range roundtripCorpus {
		n, err := Parse([]byte(src))
		if err != nil {
			t.Fatalf("roundtripCorpus[%s]: Parse: %v", name, err)
		}
		if got := n.Serialize(); !bytes.Equal(got, []byte(src)) {
			t.Fatalf("roundtripCorpus[%s]: round-trip not byte-identical after SetAliases lands\n--- want ---\n%q\n--- got ---\n%q", name, src, got)
		}
	}
}

// sameStringSlice reports whether a and b contain the same elements in the
// same order (a small local helper so this file needn't import "reflect").
func sameStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
