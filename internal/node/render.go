package node

import (
	"bytes"
	"sort"
	"strconv"
	"strings"
)

// Render produces canonical markdown for a node built from typed fields (a
// CREATE or import, where there is no authoritative Raw to preserve). It is the
// inverse of deriveView+extractBody: typed fields -> text. It reads only the
// exported typed fields (never the parse-internal spans), so it works for a
// freshly-minted node.
//
// Render closes the H1 gap: every create/import/promotion path renders through
// ONE primitive, and the output is itself round-trip-stable —
// serialize(parse(render(n))) == render(n) and parse(render(n)) reproduces the
// node's typed view — so a created node's FIRST edit cannot misbehave. This is
// the create-path counterpart to the byte-preserving edit path (Serialize).
//
// Conventions (must mirror the parser):
//   - Frontmatter keys are emitted in a canonical order: id, type, time, author,
//     aliases, then props (sorted by key), then typed-edge links (rel != the
//     body-link rel "references"), sorted by rel,to.
//   - Aliases render as an inline list `[a, b]` (parseAliases reads that form).
//   - A typed-edge link renders as `rel: "[[to#frag]]"` — quoted so it is valid
//     YAML for Obsidian; the parser strips the quotes (parseRefValue).
//   - Body links (rel "references") live in the body text and are NOT re-emitted
//     as frontmatter.
//   - With no frontmatter fields at all, only the body is emitted.
func (n *Node) Render() []byte {
	var fm []string
	add := func(k, v string) { fm = append(fm, k+": "+v) }

	if n.ULID != "" {
		add("id", n.ULID)
	}
	if n.Type != "" {
		add("type", n.Type)
	}
	if n.Time != "" {
		add("time", n.Time)
	}
	if n.Author != "" {
		add("author", n.Author)
	}
	if len(n.Aliases) > 0 {
		add("aliases", "["+strings.Join(n.Aliases, ", ")+"]")
	}

	if len(n.Props) > 0 {
		keys := make([]string, 0, len(n.Props))
		for k := range n.Props {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			add(k, n.Props[k])
		}
	}

	typed := make([]Link, 0, len(n.Links))
	for _, l := range n.Links {
		if l.Rel == "references" {
			continue // body links stay in the body
		}
		typed = append(typed, l)
	}
	sort.Slice(typed, func(i, j int) bool {
		if typed[i].Rel != typed[j].Rel {
			return typed[i].Rel < typed[j].Rel
		}
		return typed[i].To < typed[j].To
	})
	// Group by Rel (typed is already sorted by Rel,To so same-Rel links are
	// contiguous). A rel with exactly one link keeps the exact current
	// single-line form; a rel with more than one link emits as ONE
	// comma-joined line so it round-trips through parseRefValues instead of
	// becoming duplicate frontmatter keys (which parseFrontmatter's
	// last-line-wins overwrite would collapse to one on reparse).
	quotedRef := func(l Link) string {
		ref := l.To
		if l.ToFrag != "" {
			ref += "#" + l.ToFrag
		}
		return strconv.Quote("[[" + ref + "]]")
	}
	for i := 0; i < len(typed); {
		j := i
		for j < len(typed) && typed[j].Rel == typed[i].Rel {
			j++
		}
		group := typed[i:j]
		if len(group) == 1 {
			add(group[0].Rel, quotedRef(group[0]))
		} else {
			parts := make([]string, len(group))
			for k, l := range group {
				parts[k] = quotedRef(l)
			}
			add(group[0].Rel, strings.Join(parts, ", "))
		}
		i = j
	}

	var out bytes.Buffer
	if len(fm) > 0 {
		out.WriteString("---\n")
		out.WriteString(strings.Join(fm, "\n"))
		out.WriteString("\n---\n")
	}
	out.WriteString(n.Body)
	return out.Bytes()
}

// NewNode builds a create-ready node from typed fields, minting a ULID when id
// is empty. The returned node has no authoritative Raw yet; call Render to
// produce canonical text to write, then Parse that text for subsequent edits.
func NewNode(typ, author, body string) *Node {
	return &Node{
		ULID:   Mint(),
		Type:   typ,
		Time:   "",
		Author: author,
		Body:   body,
	}
}
