package node

import "io"

// STUB — implementation lands in Phase 4.

// EnvelopeVersion is the current NDJSON envelope schema version.
const EnvelopeVersion = 1

// Envelope is the parser's normalized NDJSON output: every inline fact plus
// parser-derived fields. A superset of the inline facts. Serializes to exactly
// one physical line (body newlines escape to \n).
type Envelope struct {
	V         int               `json:"v"`
	ULID      string            `json:"ulid,omitempty"`
	Type      string            `json:"type"`
	Time      string            `json:"time"`
	Author    string            `json:"author"`
	Body      string            `json:"body"`
	Aliases   []string          `json:"aliases,omitempty"`
	Props     map[string]string `json:"props,omitempty"`
	Fragments []Fragment        `json:"fragments,omitempty"`
	Links     []Link            `json:"links,omitempty"`
	Loc       Loc               `json:"loc"`
	Hash      string            `json:"hash,omitempty"`
	Mtime     string            `json:"mtime,omitempty"`
}

// Envelope projects the node into its NDJSON envelope form.
func (n *Node) Envelope() Envelope { return Envelope{} }

// Marshal encodes the envelope as compact one-line JSON (no trailing newline).
func (e Envelope) Marshal() ([]byte, error) { return nil, nil }

// Unmarshal decodes one envelope from a single NDJSON line.
func Unmarshal(data []byte) (Envelope, error) { return Envelope{}, nil }

// WriteNDJSON writes envelopes as newline-delimited JSON, one node per line.
func WriteNDJSON(w io.Writer, envs ...Envelope) error { return nil }

// ReadNDJSON reads newline-delimited JSON envelopes from r.
func ReadNDJSON(r io.Reader) ([]Envelope, error) { return nil, nil }
