package node

// STUB — implementation lands in Phase 4.

// Parser is the per-tool keystone pair. Each tool ships one; the core stays
// generic and calls it. Parse splits a file into its nodes; Serialize is the
// inverse, producing inline text to write into the tool's store.
type Parser interface {
	Parse(raw []byte, loc Loc) ([]*Node, error)
	Serialize(n *Node) ([]byte, error)
}

// MarkdownParser is the default file-per-item parser (frontmatter + markdown
// body). Group-file sub-node splitting with per-entry ULIDs is a per-tool concern
// (the log tool, T4); this default returns a single node per file.
type MarkdownParser struct{}

// Parse implements Parser.
func (MarkdownParser) Parse(raw []byte, loc Loc) ([]*Node, error) { return nil, nil }

// Serialize implements Parser.
func (MarkdownParser) Serialize(n *Node) ([]byte, error) { return nil, nil }

var _ Parser = MarkdownParser{}
