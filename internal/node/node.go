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
// Productionized from the gating spike internal/spike/roundtrip (PASSED).
package node

// STUB — implementation lands in Phase 4. Signatures exist so tests compile and
// fail (TDD red).

// Span is a byte range [Start,End) within a Node's Raw bytes.
type Span struct{ Start, End int }

// Link is a forward typed edge derived from the body or a ref-valued prop.
type Link struct {
	Rel      string `json:"rel"`
	To       string `json:"to"`
	FromFrag string `json:"from_frag,omitempty"`
	ToFrag   string `json:"to_frag,omitempty"`
}

// Fragment is a node-local sub-anchor (^block id).
type Fragment struct {
	ID     string `json:"id"`
	Anchor string `json:"anchor,omitempty"`
}

// Loc is the parser-derived source location (envelope-only).
type Loc struct {
	File string `json:"file"`
}

// Node is a byte-preserving view of one markdown file (or sub-node of a group
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
	fieldSpans  map[string]Span
	bodySpan    Span
}

// Parse builds a byte-preserving Node from raw file bytes (no location).
func Parse(raw []byte) (*Node, error) { return ParseAt(raw, Loc{}) }

// ParseAt is Parse with a source location recorded in Loc.
func ParseAt(raw []byte, loc Loc) (*Node, error) { return &Node{Loc: loc}, nil }

// Serialize returns the authoritative bytes.
func (n *Node) Serialize() []byte { return nil }

// SetField applies a span-local edit to an existing scalar frontmatter field.
func (n *Node) SetField(key, value string) error { return nil }

// Entry is one `## ...` block within a group file.
type Entry struct {
	Header string
	Span   Span
}

// SplitEntries returns the entry blocks of a group file.
func (n *Node) SplitEntries() []Entry { return nil }

// ReplaceEntryBody splices new bytes over an entry's whole block.
func (n *Node) ReplaceEntryBody(e Entry, newBlock string) ([]byte, error) { return nil, nil }
