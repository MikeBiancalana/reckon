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
