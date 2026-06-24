package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// prettyRecord implements fmt.Stringer for T-6 pretty-mode tests.
// Using a local type avoids importing external packages and keeps tests hermetic.
type prettyRecord struct {
	Title string
}

func (r prettyRecord) String() string {
	return "Record: " + r.Title
}

// testRecord is a plain struct used for JSON and NDJSON output tests (T-7, T-8, T-9).
type testRecord struct {
	ID    int    `json:"id"`
	Value string `json:"value"`
}

// TestPrint_Pretty (T-6 / AC-3): Pretty mode must emit human-readable text via
// fmt.Stringer. Output must NOT start with '{' or '['.
func TestPrint_Pretty(t *testing.T) {
	var buf bytes.Buffer
	w := New(&buf, Pretty)
	if err := w.Print(prettyRecord{Title: "hello"}); err != nil {
		t.Fatalf("Print (Pretty): %v", err)
	}
	out := buf.String()
	if len(out) == 0 {
		t.Fatal("Pretty mode: expected non-empty output")
	}
	trimmed := strings.TrimSpace(out)
	if len(trimmed) == 0 {
		t.Fatal("Pretty mode: output is all whitespace")
	}
	first := trimmed[0]
	if first == '{' || first == '[' {
		t.Errorf("Pretty mode output starts with %q — expected human-readable text, not JSON\noutput: %s", first, out)
	}
}

// TestPrint_JSON (T-7 / AC-3 / EC-5): JSON mode Print for a single record must emit
// a single valid JSON object (starting with '{'), parseable by json.Unmarshal, and
// NOT an array.
func TestPrint_JSON(t *testing.T) {
	var buf bytes.Buffer
	w := New(&buf, JSON)
	rec := testRecord{ID: 1, Value: "alpha"}
	if err := w.Print(rec); err != nil {
		t.Fatalf("Print (JSON): %v", err)
	}
	out := strings.TrimSpace(buf.String())
	if len(out) == 0 {
		t.Fatal("JSON mode: expected non-empty output")
	}
	if out[0] != '{' {
		t.Errorf("JSON single Print: first byte = %q, want '{'", out[0])
	}
	if out[0] == '[' {
		t.Error("JSON single Print must not emit an array (EC-5)")
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Errorf("json.Unmarshal failed: %v\noutput: %s", err, out)
	}
}

// TestPrintAll_NDJSON_Multiple (T-8 / AC-3 / EC-5): NDJSON PrintAll of 3 records
// must emit exactly 3 newline-terminated lines, each a valid JSON object, and no
// line may start with '['.
func TestPrintAll_NDJSON_Multiple(t *testing.T) {
	var buf bytes.Buffer
	w := New(&buf, NDJSON)
	recs := []any{
		testRecord{ID: 1, Value: "a"},
		testRecord{ID: 2, Value: "b"},
		testRecord{ID: 3, Value: "c"},
	}
	if err := w.PrintAll(recs); err != nil {
		t.Fatalf("PrintAll (NDJSON, 3 records): %v", err)
	}
	raw := buf.String()
	lines := strings.Split(strings.TrimRight(raw, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("NDJSON 3-record PrintAll: got %d lines, want 3\noutput: %q", len(lines), raw)
	}
	for i, line := range lines {
		if line == "" {
			t.Errorf("line %d is empty", i)
			continue
		}
		if line[0] == '[' {
			t.Errorf("line %d starts with '[' — must be JSON object not array: %q", i, line)
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Errorf("line %d: json.Unmarshal failed: %v\nline: %q", i, err, line)
		}
	}
}

// TestPrintAll_NDJSON_Single (T-9 / EC-5): NDJSON PrintAll of exactly 1 record must
// emit exactly 1 newline-terminated JSON line — never a one-element array.
func TestPrintAll_NDJSON_Single(t *testing.T) {
	var buf bytes.Buffer
	w := New(&buf, NDJSON)
	if err := w.PrintAll([]any{testRecord{ID: 42, Value: "only"}}); err != nil {
		t.Fatalf("PrintAll (NDJSON, 1 record): %v", err)
	}
	raw := buf.String()
	if !strings.HasSuffix(raw, "\n") {
		t.Errorf("NDJSON line must end with newline; got %q", raw)
	}
	line := strings.TrimRight(raw, "\n")
	if strings.Contains(line, "\n") {
		t.Errorf("NDJSON single-record PrintAll produced multiple lines: %q", raw)
	}
	if len(line) == 0 {
		t.Fatal("expected non-empty NDJSON line")
	}
	if line[0] == '[' {
		t.Errorf("NDJSON single record must not be wrapped in an array (EC-5): %q", line)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(line), &obj); err != nil {
		t.Errorf("json.Unmarshal failed: %v\nline: %q", err, line)
	}
}

// TestPrintAll_JSONArray: JSON mode PrintAll must emit a single top-level JSON array
// '[...]' containing all records (as opposed to NDJSON which emits one line each).
func TestPrintAll_JSONArray(t *testing.T) {
	var buf bytes.Buffer
	w := New(&buf, JSON)
	recs := []any{
		testRecord{ID: 1, Value: "x"},
		testRecord{ID: 2, Value: "y"},
	}
	if err := w.PrintAll(recs); err != nil {
		t.Fatalf("PrintAll (JSON, 2 records): %v", err)
	}
	out := strings.TrimSpace(buf.String())
	if len(out) == 0 {
		t.Fatal("JSON PrintAll: expected non-empty output")
	}
	if out[0] != '[' {
		t.Errorf("JSON mode PrintAll: first byte = %q, want '['", out[0])
	}
	if out[len(out)-1] != ']' {
		t.Errorf("JSON mode PrintAll: last byte = %q, want ']'", out[len(out)-1])
	}
	var arr []map[string]any
	if err := json.Unmarshal([]byte(out), &arr); err != nil {
		t.Errorf("json.Unmarshal as array failed: %v\noutput: %s", err, out)
	}
	if len(arr) != 2 {
		t.Errorf("JSON array length = %d, want 2", len(arr))
	}
}

// TestModeFromFlags_MutualExclusion: ModeFromFlags must return an error when both
// --json and --ndjson are set (mutually exclusive). Other combinations must succeed
// with the expected Mode value.
func TestModeFromFlags_MutualExclusion(t *testing.T) {
	tests := []struct {
		name    string
		json    bool
		ndjson  bool
		want    Mode
		wantErr bool
	}{
		// both flags → error (AC-3 mutual exclusion)
		{"both flags set", true, true, 0, true},
		// json only → JSON mode
		{"json only", true, false, JSON, false},
		// ndjson only → NDJSON mode
		{"ndjson only", false, true, NDJSON, false},
		// neither → Pretty mode (default)
		{"neither flag", false, false, Pretty, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ModeFromFlags(tt.json, tt.ndjson)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ModeFromFlags(%v, %v): expected error, got nil (mode=%v)", tt.json, tt.ndjson, got)
				}
			} else {
				if err != nil {
					t.Errorf("ModeFromFlags(%v, %v): unexpected error: %v", tt.json, tt.ndjson, err)
					return
				}
				if got != tt.want {
					t.Errorf("ModeFromFlags(%v, %v) = %v, want %v", tt.json, tt.ndjson, got, tt.want)
				}
			}
		})
	}
}
