package node

// Parser is the per-tool keystone pair. Each tool ships one; the core stays
// generic and calls it. Parse splits a file into its nodes; Serialize is the
// inverse, producing inline text to write into the tool's store. Everything else
// (emit, import, index, resolve) reduces to this pair.
type Parser interface {
	Parse(raw []byte, loc Loc) ([]*Node, error)
	Serialize(n *Node) ([]byte, error)
}

// MarkdownParser is the default file-per-item parser (frontmatter + markdown
// body), returning a single node per file. Group-file sub-node splitting with
// per-entry ULIDs is a per-tool concern (the log tool, T4); see (*Node).SplitEntries
// for the building block.
type MarkdownParser struct{}

// Parse implements Parser: a file-per-item file is one node.
func (MarkdownParser) Parse(raw []byte, loc Loc) ([]*Node, error) {
	n, err := ParseAt(raw, loc)
	if err != nil {
		return nil, err
	}
	return []*Node{n}, nil
}

// Serialize implements Parser. Byte-preserving: it returns the node's
// authoritative bytes (generating inline text from a model-only node, e.g. an
// imported envelope, is a promotion concern handled by a per-tool writer).
func (MarkdownParser) Serialize(n *Node) ([]byte, error) {
	return n.Serialize(), nil
}

var _ Parser = MarkdownParser{}
