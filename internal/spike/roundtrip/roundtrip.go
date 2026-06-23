// Package roundtrip is a GATING design spike for the composable reckon redesign.
//
// It proves (or refutes) the load-bearing bet from the design's keystone:
// SPAN-LOCAL WRITE-BACK. Under files-as-truth, a tool must be able to rewrite
// ONLY the spans it owns (e.g. one frontmatter field, or one log entry's body)
// while byte-preserving every other byte of the file — including prose, code
// fences, blank lines, and content no tool models. If that holds, then
// parse(serialize(parse(f))) == f is true by construction, and a parser bug can
// never silently rewrite an authoritative file.
//
// The central insight under test: do NOT regenerate the file from a structured
// model (what the current journal writer does, and why it loses unmodeled
// content). Instead, keep the original bytes as authoritative, parse a STRUCTURED
// VIEW alongside them with byte-offset spans for the owned fields, and apply
// edits as surgical splices into those spans. Serializing an unedited node
// returns the original bytes verbatim.
//
// Durable identity is the ULID (in frontmatter / a ^frag anchor), NOT a byte
// offset. Offsets are ephemeral and recomputed on every parse — which is exactly
// what reckon does on its lazy reconcile-on-read — so the "does the span survive
// edits above/below it" worry is handled by re-parsing from current text, never
// by caching offsets across external edits.
//
// This is spike-quality: it handles the common cases needed to validate the bet
// (scalar frontmatter fields, a markdown body, log-day group files), and
// documents the boundaries it does not yet cover.
package roundtrip

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

// conflictMarkers are git / Syncthing artifacts. A real file with these is
// malformed; the parser must refuse gracefully (never panic, never silently
// "fix" it), per the design's malformed-file handling.
var conflictMarkers = []string{"<<<<<<< ", "=======\n", ">>>>>>> "}

// Span is a byte range [Start,End) within a Node's Raw bytes.
type Span struct {
	Start, End int
}

// Link is a wikilink extracted from the body: [[target#frag|label]].
type Link struct {
	Target   string
	Fragment string
	Label    string
}

// Node is a byte-preserving view of one markdown file (or one sub-node of a
// group file). Raw is authoritative; the structured fields are a derived view.
type Node struct {
	Raw []byte // authoritative bytes — Serialize returns this verbatim

	// Structured view (derived from Raw on Parse).
	Frontmatter map[string]string // scalar keys only (spike scope)
	Body        string
	Links       []Link
	Tags        []string
	Fragments   []string // ^block anchors that appear as link targets

	// Span bookkeeping for span-local edits.
	fieldSpans map[string]Span // frontmatter key -> byte span of its VALUE
	bodySpan   Span
}

var (
	// A scalar frontmatter line: key, the gap after the colon, then the value.
	// Captures so we can compute the value's exact byte span. Spike scope:
	// single-line scalar values only (no block scalars, no nested maps).
	fmScalarRe = regexp.MustCompile(`^([A-Za-z0-9_-]+):([ \t]*)(.*?)([ \t]*)$`)

	wikilinkRe   = regexp.MustCompile(`\[\[([^\]]+)\]\]`)
	tagRe        = regexp.MustCompile(`(^|[\s(])#([A-Za-z0-9][A-Za-z0-9_/-]*)`)
	blockAnchorRe = regexp.MustCompile(`\s\^([A-Za-z0-9][A-Za-z0-9-]*)\s*$`)
	fenceRe      = regexp.MustCompile("^(```|~~~)")
)

// Parse builds a byte-preserving Node from raw file bytes.
func Parse(raw []byte) (*Node, error) {
	for _, m := range conflictMarkers {
		if bytes.Contains(raw, []byte(m)) {
			return nil, fmt.Errorf("refusing to parse: file contains conflict marker %q", strings.TrimSpace(m))
		}
	}

	n := &Node{
		Raw:         raw,
		Frontmatter: map[string]string{},
		fieldSpans:  map[string]Span{},
	}

	bodyStart := parseFrontmatter(n, raw)
	n.bodySpan = Span{Start: bodyStart, End: len(raw)}
	n.Body = string(raw[bodyStart:])
	extractBody(n, raw, bodyStart)
	return n, nil
}

// Serialize returns the authoritative bytes. For an unedited node this is the
// original input verbatim — which is what makes the round-trip lossless.
func (n *Node) Serialize() []byte { return n.Raw }

// SetField applies a span-local edit to an existing scalar frontmatter field:
// it splices the new value into the recorded byte span and leaves every other
// byte untouched. The node is then re-parsed from the spliced bytes so the view
// and spans stay consistent (and offsets are recomputed, never stale).
//
// Spike scope: the key must already exist as a scalar. Adding a missing key is a
// separate concern (insert a line into the frontmatter block) and is out of
// scope for proving the byte-preservation bet.
func (n *Node) SetField(key, value string) error {
	span, ok := n.fieldSpans[key]
	if !ok {
		return fmt.Errorf("SetField: no scalar span for %q (spike handles existing scalar keys only)", key)
	}
	out := make([]byte, 0, len(n.Raw)-(span.End-span.Start)+len(value))
	out = append(out, n.Raw[:span.Start]...)
	out = append(out, value...)
	out = append(out, n.Raw[span.End:]...)

	reparsed, err := Parse(out)
	if err != nil {
		return fmt.Errorf("SetField: re-parse after splice failed: %w", err)
	}
	*n = *reparsed
	return nil
}

// parseFrontmatter locates a leading `---\n ... \n---\n` block, records the byte
// span of each scalar value, and returns the byte offset where the body begins.
// If there is no frontmatter, body starts at 0.
func parseFrontmatter(n *Node, raw []byte) (bodyStart int) {
	if !bytes.HasPrefix(raw, []byte("---\n")) {
		return 0
	}
	// Find the closing fence: a line that is exactly "---".
	rest := raw[4:]
	idx := bytes.Index(rest, []byte("\n---"))
	if idx < 0 {
		return 0 // unterminated frontmatter -> treat whole file as body
	}
	// Validate the closing fence line ends the line (newline or EOF after it).
	closeAbs := 4 + idx + 1 // byte offset of the closing "---"
	afterClose := closeAbs + 3
	if afterClose < len(raw) && raw[afterClose] != '\n' {
		return 0
	}

	// Walk frontmatter lines in [4, closeAbs) and record value spans.
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
			n.Frontmatter[key] = string(raw[valStart:valEnd])
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

// extractBody pulls links, tags, and block-anchor fragments from the body,
// treating fenced code blocks as inert (so `[[notalink]]` or `#nottag` inside a
// code fence is correctly ignored — a correctness requirement for the index).
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
			n.Links = append(n.Links, parseLink(lm[1]))
		}
		for _, tm := range tagRe.FindAllStringSubmatch(line, -1) {
			n.Tags = append(n.Tags, tm[2])
		}
		if bm := blockAnchorRe.FindStringSubmatch(line); bm != nil {
			n.Fragments = append(n.Fragments, bm[1])
		}
	}
}

func parseLink(inner string) Link {
	l := Link{}
	if i := strings.Index(inner, "|"); i >= 0 {
		l.Label = inner[i+1:]
		inner = inner[:i]
	}
	if i := strings.Index(inner, "#"); i >= 0 {
		l.Fragment = strings.TrimPrefix(inner[i+1:], "^")
		inner = inner[:i]
	}
	l.Target = inner
	return l
}

// --- Group files (log day): split into entry sub-nodes -----------------------

// Entry is one `## ...` block within a group file, with the byte span of its
// whole block (header + body) within the file's Raw.
type Entry struct {
	Header string
	Span   Span
}

var entryHeaderRe = regexp.MustCompile(`(?m)^## .*$`)

// SplitEntries returns the entry blocks of a group file (e.g. a log day). Each
// entry runs from its `## ` header to just before the next header (or EOF).
// Content before the first header (frontmatter + preamble) is not an entry.
func SplitEntries(n *Node) []Entry {
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

// ReplaceEntryBody splices new bytes over an entry's whole block. Used to prove
// that editing one entry leaves its siblings byte-identical.
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
