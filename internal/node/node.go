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
	// values only (no |/> block scalars, no nested maps) — matches the proven
	// spike. A bare `key:` (empty value) followed by indented `- item`
	// continuation lines is handled separately (see parseFrontmatter/
	// scanBlockList) by synthesizing an equivalent flow value before it ever
	// reaches this regex. See internal/node/AGENTS.md for the full supported
	// subset.
	fmScalarRe = regexp.MustCompile(`^([A-Za-z0-9_-]+):([ \t]*)(.*?)([ \t]*)$`)

	// blockItemRe matches one indented `- item` continuation line under a bare
	// `key:` frontmatter line (Obsidian Properties-panel block-style lists).
	blockItemRe = regexp.MustCompile(`^[ \t]+-[ \t]*(.*)$`)

	wikilinkRe    = regexp.MustCompile(`\[\[([^\]]+)\]\]`)
	blockAnchorRe = regexp.MustCompile(`\s\^([A-Za-z0-9][A-Za-z0-9-]*)\s*$`)
	fenceRe       = regexp.MustCompile("^(```|~~~)")
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

// HasField reports whether key has a recorded scalar span in the frontmatter
// block, independent of its value. This is the predicate to use when "key
// present but blank" (SetField is the right primitive) must be distinguished
// from "key absent entirely" (InsertField is the right primitive) — a
// derived typed field being its zero value (e.g. ULID == "") cannot make
// that distinction, since a literal blank `id: ` line also derives to "".
func (n *Node) HasField(key string) bool {
	_, ok := n.fieldSpans[key]
	return ok
}

// parseFrontmatter locates a leading `---\n ... \n---\n` (or CRLF-delimited)
// block, records the byte span of each scalar value (in source order), and
// returns the body's byte offset. No frontmatter -> body starts at 0.
//
// EOL handling is lenient and independently checked at each of three spots
// (opening delimiter, closing delimiter, per-line value extraction) rather
// than by normalizing the buffer, so every recorded Span stays a valid offset
// into the untouched Raw bytes (see SetField). See internal/node/AGENTS.md.
func parseFrontmatter(n *Node, raw []byte) (bodyStart int) {
	var headerLen int
	switch {
	case bytes.HasPrefix(raw, []byte("---\r\n")):
		headerLen = 5
	case bytes.HasPrefix(raw, []byte("---\n")):
		headerLen = 4
	default:
		return 0
	}
	rest := raw[headerLen:]
	idx := bytes.Index(rest, []byte("\n---"))
	if idx < 0 {
		return 0 // unterminated frontmatter -> whole file is body
	}
	closeAbs := headerLen + idx + 1 // byte offset of the closing "---"
	afterClose := closeAbs + 3

	// closeNLLen is the length of the line ending immediately after the
	// closing "---" (0 if the closing delimiter is the last thing in the
	// file). Accept either "\n" or "\r\n"; anything else means this isn't a
	// real closing delimiter (e.g. "---abc") and the whole file is body.
	closeNLLen := 0
	if afterClose < len(raw) {
		switch {
		case raw[afterClose] == '\n':
			closeNLLen = 1
		case raw[afterClose] == '\r' && afterClose+1 < len(raw) && raw[afterClose+1] == '\n':
			closeNLLen = 2
		default:
			return 0
		}
	}

	pos := headerLen
	for pos < closeAbs {
		nl := bytes.IndexByte(raw[pos:closeAbs], '\n')
		var lineEnd int
		if nl < 0 {
			lineEnd = closeAbs
		} else {
			lineEnd = pos + nl
		}
		// effEnd excludes a trailing '\r' from the substring handed to
		// fmScalarRe/blockItemRe, so captured values never carry a stray \r —
		// Raw itself is never touched, and valStart/valEnd (computed as pos +
		// match-index-into-this-substring) remain correct Raw offsets.
		effEnd := lineEnd
		if effEnd > pos && raw[effEnd-1] == '\r' {
			effEnd--
		}
		line := string(raw[pos:effEnd])
		if m := fmScalarRe.FindStringSubmatchIndex(line); m != nil {
			key := line[m[2]:m[3]]
			valStart := pos + m[6]
			valEnd := pos + m[7]
			if valStart == valEnd && nl >= 0 {
				// Empty scalar value: check for an indented `- item` block
				// list continuing on the following lines.
				if items, nextPos, found := scanBlockList(raw, lineEnd+1, closeAbs); found {
					if _, seen := n.frontmatter[key]; !seen {
						n.fmOrder = append(n.fmOrder, key)
					}
					n.frontmatter[key] = "[" + strings.Join(items, ", ") + "]"
					// No fieldSpan: this value has no single contiguous byte
					// span in Raw (it spans multiple lines), so SetField
					// correctly refuses to edit it.
					pos = nextPos
					continue
				}
			}
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
		return afterClose + closeNLLen
	}
	return len(raw)
}

// scanBlockList consumes indented `- item` continuation lines starting at pos
// (up to limit), returning the trimmed item text of each and the position
// immediately after the last consumed line. found is false (items nil, nextPos
// == pos) if the very first line isn't a continuation line — an empty block
// list (`aliases:` immediately followed by another key or the closing `---`)
// is not an error, just "no items".
func scanBlockList(raw []byte, pos, limit int) (items []string, nextPos int, found bool) {
	start := pos
	for pos < limit {
		nl := bytes.IndexByte(raw[pos:limit], '\n')
		var lineEnd int
		if nl < 0 {
			lineEnd = limit
		} else {
			lineEnd = pos + nl
		}
		effEnd := lineEnd
		if effEnd > pos && raw[effEnd-1] == '\r' {
			effEnd--
		}
		line := string(raw[pos:effEnd])
		m := blockItemRe.FindStringSubmatch(line)
		if m == nil {
			break
		}
		items = append(items, strings.TrimSpace(m[1]))
		if nl < 0 {
			pos = limit
			break
		}
		pos = lineEnd + 1
	}
	if len(items) == 0 {
		return nil, start, false
	}
	return items, pos, true
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
		if refs, ok := parseRefValues(val); ok {
			for _, r := range refs {
				n.Links = append(n.Links, Link{Rel: k, To: r.To, ToFrag: r.Frag})
			}
			continue
		}
		if n.Props == nil {
			n.Props = map[string]string{}
		}
		n.Props[k] = val
	}
}

// extractBody appends body-derived links (rel=references) and block-anchor
// fragments, treating fenced code blocks AND inline code spans as inert (so
// [[notalink]] / #nottag inside either is correctly ignored — a correctness
// requirement for the index). Indented (4-space) code blocks are not treated
// as code — an explicit non-goal, see internal/node/AGENTS.md.
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
		masked := maskInlineCode(line)
		for _, lm := range wikilinkRe.FindAllStringSubmatch(masked, -1) {
			n.Links = append(n.Links, parseBodyLink(lm[1]))
		}
		if bm := blockAnchorRe.FindStringSubmatch(masked); bm != nil {
			n.Fragments = append(n.Fragments, Fragment{ID: bm[1]})
		}
	}
}

// maskInlineCode replaces backtick-delimited inline code spans within a single
// line with equal-length spaces, so position-sensitive regexes (blockAnchorRe's
// end anchor) still work and non-code content elsewhere on the line is
// untouched. Backtick run lengths must match exactly to close a span (so a
// double-backtick span containing a lone nested backtick, e.g. "“a ` b“", is
// handled per CommonMark's backtick-fence rule). A run with no matching
// same-length closer before end of line is left as literal text — it must not
// blind real content (e.g. a real [[link]]) later on the line.
func maskInlineCode(line string) string {
	b := []byte(line)
	out := append([]byte(nil), b...) // full copy: untouched bytes pass through as-is
	i := 0
	for i < len(b) {
		if b[i] != '`' {
			i++
			continue
		}
		j := i
		for j < len(b) && b[j] == '`' {
			j++
		}
		runLen := j - i

		// Search for a same-length backtick run to close this span. A
		// differently-sized run in between is not a valid closer (per
		// CommonMark's backtick-fence rule) — skip past it and keep looking.
		k := j
		closeStart := -1
		for k < len(b) {
			if b[k] != '`' {
				k++
				continue
			}
			m := k
			for m < len(b) && b[m] == '`' {
				m++
			}
			if m-k == runLen {
				closeStart = k
				break
			}
			k = m
		}

		if closeStart < 0 {
			// Unterminated: leave literal (out already holds the original
			// bytes here) and resume scanning right after this run.
			i = j
			continue
		}

		spanEnd := closeStart + runLen
		for p := i; p < spanEnd; p++ {
			out[p] = ' '
		}
		i = spanEnd
	}
	return string(out)
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

// refTarget is one resolved wikilink target from a ref-valued frontmatter
// value (see parseRefValues).
type refTarget struct {
	To   string
	Frag string
}

// refListSeparators is the set of characters allowed to remain between/around
// [[wikilink]] tokens in a ref-valued frontmatter value for it to count as a
// clean multi-target ref list.
const refListSeparators = "[],\t \""

// parseRefValues reports whether a frontmatter value is a clean wikilink
// reference list: one or more [[target]] tokens (optionally the whole value
// double-quoted), with nothing but separator characters left once every
// matched token is removed. This is the guard against over-eager
// linkification: a value like `[[A]], not-a-link` returns ok=false rather
// than fabricating a garbage Link — the value falls through to Props instead.
// Single-target `depends: "[[X]]"` returns exactly one refTarget, unchanged
// from before this value was multi-target-aware.
func parseRefValues(raw string) (refs []refTarget, ok bool) {
	s := strings.TrimSpace(raw)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	matches := wikilinkRe.FindAllStringSubmatchIndex(s, -1)
	if len(matches) == 0 {
		return nil, false
	}

	var remainder strings.Builder
	last := 0
	for _, m := range matches {
		remainder.WriteString(s[last:m[0]])
		last = m[1]
	}
	remainder.WriteString(s[last:])
	for _, r := range remainder.String() {
		if !strings.ContainsRune(refListSeparators, r) {
			return nil, false
		}
	}

	for _, m := range matches {
		to, frag := splitRef(s[m[2]:m[3]])
		refs = append(refs, refTarget{To: to, Frag: frag})
	}
	return refs, true
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
