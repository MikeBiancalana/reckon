package node

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// EnvelopeVersion is the current NDJSON envelope schema version.
const EnvelopeVersion = 1

// Envelope is the parser's normalized NDJSON output: every inline fact plus
// parser-derived fields (loc, hash, mtime, v). A superset of the inline facts and
// the interchange format between tools. It serializes to exactly one physical
// line — a body's internal newlines are JSON-escaped to \n, so no object ever
// contains a literal newline byte.
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
func (n *Node) Envelope() Envelope {
	return Envelope{
		V:         EnvelopeVersion,
		ULID:      n.ULID,
		Type:      n.Type,
		Time:      n.Time,
		Author:    n.Author,
		Body:      n.Body,
		Aliases:   n.Aliases,
		Props:     n.Props,
		Fragments: n.Fragments,
		Links:     n.Links,
		Loc:       n.Loc,
	}
}

// Marshal encodes the envelope as compact one-line JSON (no trailing newline).
func (e Envelope) Marshal() ([]byte, error) {
	data, err := json.Marshal(e)
	if err != nil {
		return nil, fmt.Errorf("envelope marshal: %w", err)
	}
	return data, nil
}

// Unmarshal decodes one envelope from a single NDJSON line.
func Unmarshal(data []byte) (Envelope, error) {
	var e Envelope
	if err := json.Unmarshal(data, &e); err != nil {
		return Envelope{}, fmt.Errorf("envelope unmarshal: %w", err)
	}
	return e, nil
}

// WriteNDJSON writes envelopes as newline-delimited JSON, one node per line.
func WriteNDJSON(w io.Writer, envs ...Envelope) error {
	bw := bufio.NewWriter(w)
	for _, e := range envs {
		data, err := e.Marshal()
		if err != nil {
			return err
		}
		if bytes.ContainsRune(data, '\n') {
			return fmt.Errorf("WriteNDJSON: encoded envelope contains a literal newline")
		}
		bw.Write(data)
		bw.WriteByte('\n')
	}
	return bw.Flush()
}

// ReadNDJSON reads newline-delimited JSON envelopes from r. Blank lines are
// skipped; any malformed line is an error.
func ReadNDJSON(r io.Reader) ([]Envelope, error) {
	var out []Envelope
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		e, err := Unmarshal(line)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("ReadNDJSON: %w", err)
	}
	return out, nil
}
