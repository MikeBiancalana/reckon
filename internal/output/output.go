// Package output provides a unified Writer for pretty, JSON, and NDJSON output modes.
package output

import (
	"encoding/json"
	"fmt"
	"io"
)

// Mode selects how records are serialised by a Writer.
type Mode int

const (
	Pretty Mode = iota // human-readable text
	JSON               // single JSON object or array
	NDJSON             // newline-delimited JSON, one object per line
)

// Writer writes records to an io.Writer in a consistent output mode.
type Writer struct {
	w    io.Writer
	mode Mode
}

// New constructs a Writer that serialises to w using the given mode.
func New(w io.Writer, mode Mode) *Writer {
	return &Writer{w: w, mode: mode}
}

// ModeFromFlags derives a Mode from the --json / --ndjson flag values.
// Returns an error when both are true (mutually exclusive).
func ModeFromFlags(jsonFlag, ndjsonFlag bool) (Mode, error) {
	if jsonFlag && ndjsonFlag {
		return 0, fmt.Errorf("output: --json and --ndjson are mutually exclusive")
	}
	if jsonFlag {
		return JSON, nil
	}
	if ndjsonFlag {
		return NDJSON, nil
	}
	return Pretty, nil
}

// prettyPrinter is the optional rich-text rendering interface.
type prettyPrinter interface {
	Pretty() string
}

// Print writes a single record. JSON mode emits one object; NDJSON emits one
// compact line; Pretty prefers Pretty(), then fmt.Stringer, then fmt.Sprintf("%v").
func (wr *Writer) Print(v any) error {
	switch wr.mode {
	case JSON, NDJSON:
		data, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("output marshal: %w", err)
		}
		if _, err := fmt.Fprintf(wr.w, "%s\n", data); err != nil {
			return fmt.Errorf("output write: %w", err)
		}
		return nil
	default: // Pretty
		var s string
		switch t := v.(type) {
		case prettyPrinter:
			s = t.Pretty()
		case fmt.Stringer:
			s = t.String()
		default:
			s = fmt.Sprintf("%v", v)
		}
		if _, err := fmt.Fprintf(wr.w, "%s\n", s); err != nil {
			return fmt.Errorf("output write: %w", err)
		}
		return nil
	}
}

// PrintAll writes a slice of records. JSON mode marshals the whole slice as one
// array; NDJSON emits one compact JSON line per element; Pretty renders each
// element individually.
func (wr *Writer) PrintAll(vs []any) error {
	switch wr.mode {
	case JSON:
		data, err := json.Marshal(vs)
		if err != nil {
			return fmt.Errorf("output marshal array: %w", err)
		}
		if _, err := fmt.Fprintf(wr.w, "%s\n", data); err != nil {
			return fmt.Errorf("output write array: %w", err)
		}
		return nil
	default: // NDJSON and Pretty: one element at a time
		for _, v := range vs {
			if err := wr.Print(v); err != nil {
				return err
			}
		}
		return nil
	}
}
