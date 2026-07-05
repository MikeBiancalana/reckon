// Package node — LogParser, the group-file parser for the log tool (v1-T4).
//
// A "log-day" file (frontmatter `type: log-day`) is a group file: one day's
// preamble/frontmatter followed by N `## HH:MM · author` entry blocks (see
// (*Node).SplitEntries in node.go). LogParser.Parse splits such a file into
// the day node plus one log-entry node per block; any other file (Type !=
// "log-day") passes through unchanged, exactly like MarkdownParser, so
// LogParser is safe to use as the vault-wide default parser (index.Open).
//
// FORMAT COUPLING: entryHeaderFieldsRe/extractEntryID below are markdown-ATX-
// header-specific, mirroring node.go's own FORMAT COUPLING note for
// SplitEntries. RenderLogEntry is the one shared format definition the writer
// (internal/cli/add.go) and this parser both use, so writer and reader can
// never drift apart on the entry byte-format.
package node

import (
	"bytes"
	"regexp"
	"strings"
)

// entryHeaderFieldsRe parses one log-entry header line's HH:MM, optional kind
// word, and optional "· author" suffix:
//
//	## HH:MM · author         (no kind word)
//	## HH:MM kind · author    (kind word present)
//
// The kind character class excludes the "·" separator itself so a header
// with no kind word never mismatches "·" as the kind (this is what makes the
// two header shapes unambiguous under Go's regexp engine without relying on
// any particular backtracking behavior).
var entryHeaderFieldsRe = regexp.MustCompile(`^## (\d{1,2}:\d{2})(?: ([^\s·]+))?(?: · (.+?))?\s*$`)

// LogParser splits a "log-day" group file into a day node plus one
// "log-entry" node per `## ` block; any other file passes through as a
// single unchanged node (Type-driven dispatch, matching MarkdownParser).
type LogParser struct{}

// Parse implements Parser.
func (LogParser) Parse(raw []byte, loc Loc) ([]*Node, error) {
	day, err := ParseAt(raw, loc)
	if err != nil {
		return nil, err
	}
	if day.Type != "log-day" {
		return []*Node{day}, nil
	}

	dayDate := logDayDate(day, loc)

	entries := day.SplitEntries()
	nodes := make([]*Node, 0, len(entries)+1)
	nodes = append(nodes, day)

	for _, e := range entries {
		entry := buildLogEntry(e, day.Raw, dayDate, loc)
		nodes = append(nodes, entry)
		if entry.ULID != "" {
			day.Links = append(day.Links, Link{Rel: "contains", To: entry.ULID})
		}
	}
	return nodes, nil
}

// Serialize implements Parser. Byte-preserving: entries are sub-views of the
// day file's Raw and are never serialized back to their own files (plan.md
// "Files to modify" — LogParser.Serialize).
func (LogParser) Serialize(n *Node) ([]byte, error) {
	return n.Serialize(), nil
}

var _ Parser = LogParser{}

// logDayDate returns the day file's date string: the day node's first alias
// if present, else derived from the "log/<date>.md" Loc.File.
func logDayDate(day *Node, loc Loc) string {
	if len(day.Aliases) > 0 {
		return day.Aliases[0]
	}
	base := loc.File
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	return strings.TrimSuffix(base, ".md")
}

// buildLogEntry derives one log-entry *Node from a SplitEntries block. raw is
// the day node's full Raw bytes (e.Span indexes into it).
func buildLogEntry(e Entry, raw []byte, dayDate string, loc Loc) *Node {
	block := raw[e.Span.Start:e.Span.End]

	rest := block[len(e.Header):]
	rest = bytes.TrimPrefix(rest, []byte("\r\n"))
	rest = bytes.TrimPrefix(rest, []byte("\n"))

	ulid, body := extractEntryID(rest)
	didTarget, body := extractEntryDid(body)

	hhmm, kind, author := "", "", ""
	if m := entryHeaderFieldsRe.FindStringSubmatch(e.Header); m != nil {
		hhmm, kind, author = m[1], m[2], m[3]
	}

	// entryTime is left empty (rather than the malformed "<dayDate>T:00Z")
	// when the header doesn't match entryHeaderFieldsRe -- e.g. a
	// hand-authored/synced day file with a non-timestamp "## Section"
	// heading (SplitEntries treats any "^## " line as an entry boundary;
	// C2, reckon-uv09 review).
	entryTime := ""
	if hhmm != "" {
		entryTime = dayDate + "T" + hhmm + ":00Z"
	}

	n := &Node{
		Raw:    block,
		Loc:    loc,
		Type:   "log-entry",
		ULID:   ulid,
		Time:   entryTime,
		Author: author,
		// Body is trimmed (plan.md: "Body = entry lines minus header and
		// id::, trimmed"; M2, reckon-uv09 review) -- the untrimmed
		// remainder otherwise retains the inter-entry blank line/trailing
		// newline from the source block.
		Body: strings.TrimSpace(string(body)),
	}
	if kind != "" {
		n.Props = map[string]string{"kind": kind}
	}
	if didTarget != "" {
		n.Links = append(n.Links, Link{Rel: "did", To: didTarget})
	}
	return n
}

// extractEntryID reports whether rest's first line is an inline `id:: <ULID>`
// marker (plan.md Decision 3): if so, the ULID is returned and the line is
// dropped from the returned body; otherwise ulid is "" and body is rest
// unchanged.
func extractEntryID(rest []byte) (ulid string, body []byte) {
	const prefix = "id:: "
	if !bytes.HasPrefix(rest, []byte(prefix)) {
		return "", rest
	}
	line := rest[len(prefix):]
	after := []byte(nil)
	if nl := bytes.IndexByte(line, '\n'); nl >= 0 {
		after = line[nl+1:]
		line = line[:nl]
	}
	line = bytes.TrimSuffix(line, []byte("\r"))
	return string(line), after
}

// extractEntryDid mirrors extractEntryID for the v1-T6 recurrence audit
// marker: if rest's first line is an inline `did:: <rule-ULID>` marker, the
// target ULID/alias is returned and the line is dropped from the returned
// body; otherwise target is "" and body is rest unchanged. Called after
// extractEntryID so the fixed line order (id:: then did::) peels in the same
// order RenderLogEntryWithDid writes them; a hand-authored entry with did::
// but no id:: still parses (did:: is always checked as rest's current first
// line, whatever that is).
func extractEntryDid(rest []byte) (target string, body []byte) {
	const prefix = "did:: "
	if !bytes.HasPrefix(rest, []byte(prefix)) {
		return "", rest
	}
	line := rest[len(prefix):]
	after := []byte(nil)
	if nl := bytes.IndexByte(line, '\n'); nl >= 0 {
		after = line[nl+1:]
		line = line[:nl]
	}
	line = bytes.TrimSuffix(line, []byte("\r"))
	return string(line), after
}

// RenderLogEntry returns the exact entry block for one log entry, the single
// shared format definition the writer (internal/cli/add.go) and this parser
// both use.
func RenderLogEntry(hhmm, author, ulid, body string) string {
	return "## " + hhmm + " · " + author + "\n" + "id:: " + ulid + "\n" + body + "\n"
}

// RenderLogEntryWithDid is RenderLogEntry plus a `did:: <rule-ULID>` marker
// line immediately after id:: (v1-T6 recurrence audit entry, plan.md "Exact
// `did::` Marker Syntax"). buildLogEntry's extractEntryDid turns the did::
// line into a Link{Rel:"did", To:didTarget} on the parsed entry node; the
// line is otherwise ordinary inert body bytes to the core parser, exactly
// like id::.
func RenderLogEntryWithDid(hhmm, author, ulid, didTarget, body string) string {
	return "## " + hhmm + " · " + author + "\n" + "id:: " + ulid + "\n" + "did:: " + didTarget + "\n" + body + "\n"
}
