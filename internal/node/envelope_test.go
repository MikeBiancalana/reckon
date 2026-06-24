package node

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

// AC3 — envelope marshal/unmarshal round-trips (deep-equal).
func TestEnvelopeRoundTrip(t *testing.T) {
	for name, src := range roundtripCorpus {
		t.Run(name, func(t *testing.T) {
			n, err := ParseAt([]byte(src), Loc{File: "x/" + name + ".md"})
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			env := n.Envelope()
			data, err := env.Marshal()
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			got, err := Unmarshal(data)
			if err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if !reflect.DeepEqual(got, env) {
				t.Fatalf("envelope round-trip mismatch\n want %+v\n got  %+v", env, got)
			}
		})
	}
}

// AC10 — every node serializes to exactly one physical line (body newlines are
// escaped, never literal).
func TestEnvelopeIsOneLine(t *testing.T) {
	n, err := ParseAt([]byte(noteObsidian), Loc{File: "notes/x.md"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	data, err := n.Envelope().Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if bytes.ContainsRune(data, '\n') {
		t.Fatalf("marshaled envelope contains a literal newline:\n%q", data)
	}
	if !strings.Contains(string(data), `\n`) {
		t.Errorf("expected body newlines escaped as \\n in: %s", data)
	}
}

// AC11 — envelope carries schema version and loc.
func TestEnvelopeMetadata(t *testing.T) {
	n, err := ParseAt([]byte(todoItem), Loc{File: "todos/t.md"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	env := n.Envelope()
	if env.V != EnvelopeVersion {
		t.Errorf("V = %d, want %d", env.V, EnvelopeVersion)
	}
	if env.Loc.File != "todos/t.md" {
		t.Errorf("Loc.File = %q", env.Loc.File)
	}
}

// AC7/8 — routing in the envelope: scalar props vs ref-prop link vs body link.
func TestEnvelopeRouting(t *testing.T) {
	n, err := ParseAt([]byte(todoItem), Loc{File: "todos/t.md"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	env := n.Envelope()
	if env.Props["state"] != "open" {
		t.Errorf("props.state = %q, want open", env.Props["state"])
	}
	if _, ok := env.Props["depends"]; ok {
		t.Errorf("ref-valued prop leaked into props: %v", env.Props)
	}
	rels := map[string]string{}
	for _, l := range env.Links {
		rels[l.Rel] = l.To
	}
	if rels["depends"] != "01J9Z2QH8M" {
		t.Errorf("depends edge = %q, want 01J9Z2QH8M (links=%v)", rels["depends"], env.Links)
	}
	if rels["references"] != "grocery-plan" {
		t.Errorf("references edge = %q, want grocery-plan (links=%v)", rels["references"], env.Links)
	}
}

// NDJSON stream: write N envelopes, read them back; one line per node.
func TestNDJSONStream(t *testing.T) {
	var envs []Envelope
	for _, src := range []string{noteObsidian, todoItem, logDay} {
		n, err := ParseAt([]byte(src), Loc{File: "x.md"})
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		envs = append(envs, n.Envelope())
	}
	var buf bytes.Buffer
	if err := WriteNDJSON(&buf, envs...); err != nil {
		t.Fatalf("WriteNDJSON: %v", err)
	}
	lines := strings.Count(strings.TrimRight(buf.String(), "\n"), "\n") + 1
	if lines != len(envs) {
		t.Errorf("want %d NDJSON lines, got %d:\n%s", len(envs), lines, buf.String())
	}
	got, err := ReadNDJSON(&buf)
	if err != nil {
		t.Fatalf("ReadNDJSON: %v", err)
	}
	if !reflect.DeepEqual(got, envs) {
		t.Fatalf("stream round-trip mismatch\n want %+v\n got  %+v", envs, got)
	}
}
