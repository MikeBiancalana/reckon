package node

import (
	"bytes"
	"fmt"
)

// InsertField adds a new scalar frontmatter key that does not yet exist,
// splicing the "key: value\n" line into the raw bytes and re-parsing so the
// view and spans stay consistent. It is the insert counterpart to SetField
// (which requires the key to already exist): together they cover "the key is
// present" (SetField) and "the key is missing" (InsertField), never both.
//
// Detection trichotomy (plan.md D2):
//   - key already present as a scalar -> error (use SetField instead).
//   - a terminated frontmatter block exists (bodySpan.Start > 0) -> the new
//     line is spliced in as the FIRST line inside the fence (matches Render's
//     canonical key order), every other byte unchanged.
//   - raw starts with the exact 4-byte fence "---\n" but has no closing
//     "\n---" (unterminated) -> refuse outright; silently treating it as "no
//     block" would nest the user's attempted frontmatter into the body.
//   - raw does not start with "---\n" at all -> a fresh "---\nkey: value\n---\n"
//     block is prepended ahead of the untouched original bytes.
func (n *Node) InsertField(key, value string) error {
	if _, exists := n.fieldSpans[key]; exists {
		return fmt.Errorf("InsertField: key %q already present (use SetField)", key)
	}

	raw := n.Raw
	var out []byte
	switch {
	case n.bodySpan.Start > 0: // terminated frontmatter block exists
		line := key + ": " + value + "\n"
		out = append(out, raw[:4]...) // opening "---\n"
		out = append(out, line...)    // new key inserted as FIRST line inside fence
		out = append(out, raw[4:]...)
	case bytes.HasPrefix(raw, []byte("---\n")): // unterminated fence
		return fmt.Errorf("InsertField: unterminated frontmatter block; refusing to insert")
	default: // no frontmatter block at all
		out = append(out, []byte("---\n"+key+": "+value+"\n---\n")...)
		out = append(out, raw...)
	}

	reparsed, err := ParseAt(out, n.Loc)
	if err != nil {
		return fmt.Errorf("InsertField: re-parse after splice failed: %w", err)
	}
	*n = *reparsed
	return nil
}

// UpsertField sets key to value whatever shape the key currently has: it
// splices the value into an existing scalar line (adding the missing space
// after a bare "key:"), replaces a whole block-list span with a single
// "key: value" line, and delegates to InsertField when the key is absent.
// A key present as both a scalar and a block list is ambiguous, so the call
// returns an error and leaves the node unchanged. Every path re-parses before
// committing, and bytes outside the touched line(s) are preserved.
func (n *Node) UpsertField(key, value string) error {
	scalar, hasScalar := n.fieldSpans[key]
	block, hasBlock := n.blockSpans[key]
	switch {
	case hasScalar && hasBlock:
		return fmt.Errorf("UpsertField: key %q is present as both a scalar and a block list; resolve by hand", key)
	case hasScalar:
		var out []byte
		out = append(out, n.Raw[:scalar.Start]...)
		if scalar.Start == scalar.End && scalar.Start > 0 && n.Raw[scalar.Start-1] == ':' {
			out = append(out, ' ')
		}
		out = append(out, value...)
		out = append(out, n.Raw[scalar.End:]...)
		return n.commitSplice("UpsertField", out)
	case hasBlock:
		var out []byte
		out = append(out, n.Raw[:block.Start]...)
		out = append(out, key+": "+value+"\n"...)
		out = append(out, n.Raw[block.End:]...)
		return n.commitSplice("UpsertField", out)
	}
	return n.InsertField(key, value)
}

// commitSplice re-parses out and, only on success, replaces *n with the result.
func (n *Node) commitSplice(method string, out []byte) error {
	reparsed, err := ParseAt(out, n.Loc)
	if err != nil {
		return fmt.Errorf("%s: re-parse after splice failed: %w", method, err)
	}
	*n = *reparsed
	return nil
}

// RemoveField deletes every occurrence of key from the frontmatter: a scalar
// line (including its newline and any trailing comment) or a whole block-list
// span, repeating until no duplicate remains. An absent key is a no-op with a
// nil error. Removing the last key of a frontmatter block, which would leave
// an empty fence that re-parses as body, returns an error. Every path
// re-parses before committing, and bytes outside the removed lines are
// preserved.
func (n *Node) RemoveField(key string) error {
	cur := n
	for {
		var start, end int
		if span, ok := cur.fieldSpans[key]; ok {
			start = bytes.LastIndexByte(cur.Raw[:span.Start], '\n') + 1
			nl := bytes.IndexByte(cur.Raw[span.End:], '\n')
			if nl < 0 {
				return fmt.Errorf("RemoveField: key %q: unterminated line", key)
			}
			end = span.End + nl + 1
		} else if span, ok := cur.blockSpans[key]; ok {
			start, end = span.Start, span.End
		} else {
			break
		}
		out := make([]byte, 0, len(cur.Raw)-(end-start))
		out = append(out, cur.Raw[:start]...)
		out = append(out, cur.Raw[end:]...)
		next, err := ParseAt(out, n.Loc)
		if err != nil {
			return fmt.Errorf("RemoveField: re-parse after removing %q failed: %w", key, err)
		}
		if cur.bodySpan.Start > 0 && next.bodySpan.Start == 0 {
			return fmt.Errorf("RemoveField: removing %q would leave the frontmatter empty", key)
		}
		cur = next
	}
	if cur != n {
		*n = *cur
	}
	return nil
}
