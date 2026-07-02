// Package node is the canonical-node keystone for the composable reckon redesign.
//
// One representation, four consumers (parser, promotion, indexer, resolver). A
// Node is a BYTE-PRESERVING view of one markdown file: Raw is authoritative and
// Serialize returns it verbatim, so parse(serialize(parse(f))) == f holds by
// construction. The canonical typed fields (ulid/type/time/author/body/aliases/
// props/fragments/links/loc) are a derived projection over Raw; edits are surgical
// span splices, never a regenerate-from-model (the lossy anti-pattern the design
// exists to prevent).
//
// Link routing (spec invariant 3): a body [[ref]] becomes a `references` edge; a
// ref-valued frontmatter prop `K: [[X]]` becomes a typed edge with rel = the prop
// key (the generic core's rule — per-type rel vocab, e.g. depends->depends-on, is
// a per-tool parser's job) and is dropped from props. Resolution of alias/ULID
// targets is the index's job, not the parser's.
//
// Productionized from the gating spike internal/spike/roundtrip (PASSED:
// round-trip identity, surgical span edits, group-file sibling preservation,
// ~396k-exec fuzz, 0 failures).
//
// FORMAT COUPLING: parseFrontmatter/deriveView/extractBody/SplitEntries below
// are markdown-syntax-specific (frontmatter, [[wikilinks]], fenced code, ATX
// `## ` headers), and Render (render.go) emits markdown. None of that is
// currently expressed by the Parser interface (parser.go) — a second file
// format (e.g. org-mode) would need parsing+creation+group-splitting to become
// per-format concerns on that interface. Today's encapsulation (nothing
// outside this package touches fieldSpans/bodySpan/frontmatter) is what keeps
// that a contained, deferrable change rather than a rewrite.
package node

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

// conflictMarkers are git / Syncthing artifacts. A real file with these is
// malformed; the parser refuses gracefully (never panic, never silently "fix").
var conflictMarkers = []string{"<<<<<<< ", "=======\n", ">>>>>>> "}

// reservedKeys are frontmatter keys promoted to typed Node fields, so they are
// never surfaced again in Props.
var reservedKeys = map[string]bool{
	"id": true, "type": true, "time": true, "author": true, "aliases": true,
}

// Span is a byte range [Start,End) within a Node's Raw bytes.
type Span struct{ Start, End int }

// Link is a forward typed edge derived from the body or a ref-valued prop. `To`
// may be a ULID or an alias — resolution is the index's job.
type Link struct {
	Rel      string `json:"rel"`
	To       string `json:"to"`
	FromFrag string `json:"from_frag,omitempty"`
	ToFrag   string `json:"to_frag,omitempty"`
}

// Fragment is a node-local sub-anchor (a ^block id), unique within the node.
type Fragment struct {
	ID     string `json:"id"`
	Anchor string `json:"anchor,omitempty"`
}

// Loc is the parser-derived source location (envelope-only).
type Loc struct {
	File string `json:"file"`
}

// Node is a byte-preserving view of one markdown file (or a sub-node of a group
// file). Raw is authoritative; the typed fields are derived on Parse.
type Node struct {
	Raw []byte

	ULID      string
	Type      string
	Time      string
	Author    string
	Body      string
	Aliases   []string
	Props     map[string]string
	Fragments []Fragment
	Links     []Link
	Loc       Loc

	frontmatter map[string]string
	fmOrder     []string
	fieldSpans  map[string]Span
	bodySpan    Span
}

var (
	// A scalar frontmatter line: key, the gap after the colon, then the value.
	// Captures so we can compute the value's exact byte span. Single-line scalar
	// values only (no block scalars, no nested maps) — matches the proven spike.
	fmScalarRe = regexp.MustCompile(`^([A-Za-z0-9_-]+):([ \t]*)(.*?)([ \t]*)$`)

	wikilinkRe    = regexp.MustCompile(`\[\[([^\]]+)\]\]`)
	blockAnchorRe = regexp.MustCompile(`\s\^([A-Za-z0-9][A-Za-z0-9-]*)\s*$`)
	fenceRe       = regexp.MustCompile("^(```|~~~)")
	refValRe      = regexp.MustCompile(`^\[\[(.+?)\]\]$`)
)

// Parse builds a byte-preserving Node from raw file bytes (no location).
func Parse(raw []byte) (*Node, error) { return ParseAt(raw, Loc{}) }

// ParseAt is Parse with a source location recorded in Loc.
func ParseAt(raw []byte, loc Loc) (*Node, error) {
	for _, m := range conflictMarkers {
		if bytes.Contains(raw, []byte(m)) {
			return nil, fmt.Errorf("refusing to parse: file contains conflict marker %q", strings.TrimSpace(m))
		}
	}

	n := &Node{
		Raw:         raw,
		Loc:         loc,
		frontmatter: map[string]string{},
		fieldSpans:  map[string]Span{},
	}

	bodyStart := parseFrontmatter(n, raw)
	n.bodySpan = Span{Start: bodyStart, End: len(raw)}
	n.Body = string(raw[bodyStart:])
	deriveView(n)
	extractBody(n, raw, bodyStart)
	return n, nil
}

// Serialize returns the authoritative bytes. For an unedited node this is the
// original input verbatim — which is what makes the round-trip lossless.
func (n *Node) Serialize() []byte { return n.Raw }

// SetField applies a span-local edit to an existing scalar frontmatter field: it
// splices the new value into the recorded byte span, leaves every other byte
// untouched, and re-parses from the spliced bytes so the view and spans stay
// consistent (offsets recomputed, never stale).
//
// The key must already exist as a scalar; adding a missing key (inserting a
// frontmatter line) is a separate concern, out of scope for the byte-preservation
// keystone.
func (n *Node) SetField(key, value string) error {
	span, ok := n.fieldSpans[key]
	if !ok {
		return fmt.Errorf("SetField: no scalar span for %q (existing scalar keys only)", key)
	}
	out := make([]byte, 0, len(n.Raw)-(span.End-span.Start)+len(value))
	out = append(out, n.Raw[:span.Start]...)
	out = append(out, value...)
	out = append(out, n.Raw[span.End:]...)

	reparsed, err := ParseAt(out, n.Loc)
	if err != nil {
		return fmt.Errorf("SetField: re-parse after splice failed: %w", err)
	}
	*n = *reparsed
	return nil
}

// parseFrontmatter locates a leading `---\n ... \n---\n` block, records the byte
// span of each scalar value (in source order), and returns the body's byte
// offset. No frontmatter -> body starts at 0.
func parseFrontmatter(n *Node, raw []byte) (bodyStart int) {
	if !bytes.HasPrefix(raw, []byte("---\n")) {
		return 0
	}
	rest := raw[4:]
	idx := bytes.Index(rest, []byte("\n---"))
	if idx < 0 {
		return 0 // unterminated frontmatter -> whole file is body
	}
	closeAbs := 4 + idx + 1 // byte offset of the closing "---"
	afterClose := closeAbs + 3
	if afterClose < len(raw) && raw[afterClose] != '\n' {
		return 0
	}

	pos := 4
	for pos < closeAbs {
		nl := bytes.IndexByte(raw[pos:closeAbs], '\n')
		var lineEnd int
		if nl < 0 {
			lineEnd = closeAbs
		} else {
			lineEnd = pos + nl
		}
		line := string(raw[pos:lineEnd])
		if m := fmScalarRe.FindStringSubmatchIndex(line); m != nil {
			key := line[m[2]:m[3]]
			valStart := pos + m[6]
			valEnd := pos + m[7]
			if _, seen := n.frontmatter[key]; !seen {
				n.fmOrder = append(n.fmOrder, key)
			}
			n.frontmatter[key] = string(raw[valStart:valEnd])
			n.fieldSpans[key] = Span{Start: valStart, End: valEnd}
		}
		if nl < 0 {
			break
		}
		pos = lineEnd + 1
	}

	if afterClose < len(raw) {
		return afterClose + 1 // skip the newline after closing ---
	}
	return len(raw)
}

// deriveView projects the raw frontmatter into the canonical typed fields:
// reserved keys -> typed fields; ref-valued props -> typed links (rel = key);
// remaining scalars -> Props. Empty collections stay nil so the envelope's
// omitempty round-trips.
func deriveView(n *Node) {
	n.ULID = n.frontmatter["id"]
	n.Type = n.frontmatter["type"]
	n.Time = n.frontmatter["time"]
	n.Author = n.frontmatter["author"]
	n.Aliases = parseAliases(n.frontmatter["aliases"])

	for _, k := range n.fmOrder {
		if reservedKeys[k] {
			continue
		}
		val := n.frontmatter[k]
		if to, frag, ok := parseRefValue(val); ok {
			n.Links = append(n.Links, Link{Rel: k, To: to, ToFrag: frag})
			continue
		}
		if n.Props == nil {
			n.Props = map[string]string{}
		}
		n.Props[k] = val
	}
}

// extractBody appends body-derived links (rel=references) and block-anchor
// fragments, treating fenced code blocks as inert (so [[notalink]] / #nottag
// inside a fence is correctly ignored — a correctness requirement for the index).
func extractBody(n *Node, raw []byte, bodyStart int) {
	body := raw[bodyStart:]
	inFence := false
	for _, lineB := range bytes.Split(body, []byte("\n")) {
		line := string(lineB)
		if fenceRe.MatchString(strings.TrimSpace(line)) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		for _, lm := range wikilinkRe.FindAllStringSubmatch(line, -1) {
			n.Links = append(n.Links, parseBodyLink(lm[1]))
		}
		if bm := blockAnchorRe.FindStringSubmatch(line); bm != nil {
			n.Fragments = append(n.Fragments, Fragment{ID: bm[1]})
		}
	}
}

// parseAliases reads the `aliases` frontmatter value in scalar (`2026-06-22`) or
// list (`[a, b]`) form.
func parseAliases(v string) []string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	if strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]") {
		var out []string
		for _, p := range strings.Split(v[1:len(v)-1], ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		return out
	}
	return []string{v}
}

// parseRefValue reports whether a frontmatter value is a wikilink reference
// (optionally double-quoted), returning the resolved target and any fragment.
func parseRefValue(raw string) (to, frag string, ok bool) {
	s := strings.TrimSpace(raw)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	m := refValRe.FindStringSubmatch(s)
	if m == nil {
		return "", "", false
	}
	to, frag = splitRef(m[1])
	return to, frag, true
}

// parseBodyLink turns a wikilink body `target#frag|label` into a references edge.
func parseBodyLink(inner string) Link {
	to, frag := splitRef(inner)
	return Link{Rel: "references", To: to, ToFrag: frag}
}

// splitRef strips an optional |label and #fragment (or #^block) from a wikilink
// inner, returning the bare target and the fragment.
func splitRef(inner string) (to, frag string) {
	if i := strings.Index(inner, "|"); i >= 0 {
		inner = inner[:i]
	}
	if i := strings.Index(inner, "#"); i >= 0 {
		frag = strings.TrimPrefix(inner[i+1:], "^")
		inner = inner[:i]
	}
	return inner, frag
}

// --- Group files (e.g. a log day): split into entry sub-nodes ----------------
//
// FORMAT COUPLING: this section is markdown-ATX-header-specific (`## `) and is
// a *Node method, not a Parser interface method — group-splitting isn't
// currently pluggable per format at all (unlike Parse/Serialize). A second
// format needing group files would need an equivalent hung off its own
// per-format parser instead.

// Entry is one `## ...` block within a group file, with the byte span of its
// whole block (header + body) within the file's Raw.
type Entry struct {
	Header string
	Span   Span
}

var entryHeaderRe = regexp.MustCompile(`(?m)^## .*$`)

// SplitEntries returns the entry blocks of a group file. Each entry runs from its
// `## ` header to just before the next header (or EOF). Content before the first
// header (frontmatter + preamble) is not an entry.
func (n *Node) SplitEntries() []Entry {
	raw := n.Raw
	locs := entryHeaderRe.FindAllIndex(raw[n.bodySpan.Start:], -1)
	var entries []Entry
	for i, loc := range locs {
		start := n.bodySpan.Start + loc[0]
		end := len(raw)
		if i+1 < len(locs) {
			end = n.bodySpan.Start + locs[i+1][0]
		}
		hdrEnd := n.bodySpan.Start + loc[1]
		entries = append(entries, Entry{
			Header: string(raw[start:hdrEnd]),
			Span:   Span{Start: start, End: end},
		})
	}
	return entries
}

// ReplaceEntryBody splices new bytes over an entry's whole block, leaving its
// siblings byte-identical. Returns the new file bytes (the caller re-parses).
func (n *Node) ReplaceEntryBody(e Entry, newBlock string) ([]byte, error) {
	if e.Span.Start < 0 || e.Span.End > len(n.Raw) || e.Span.Start > e.Span.End {
		return nil, fmt.Errorf("ReplaceEntryBody: span out of range")
	}
	out := make([]byte, 0, len(n.Raw))
	out = append(out, n.Raw[:e.Span.Start]...)
	out = append(out, newBlock...)
	out = append(out, n.Raw[e.Span.End:]...)
	return out, nil
}
