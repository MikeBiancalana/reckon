package cli

import (
	"strings"
	"testing"
)

// TestValidateReadOnlySQL covers the statement-level read-only guard directly
// (the integration tests in query_test.go exercise it black-box; this pins the
// accept/reject/false-positive matrix from plan.md "Layer 2 — statement").
func TestValidateReadOnlySQL(t *testing.T) {
	accept := []string{
		"SELECT * FROM nodes",
		"select id from nodes where type='task'",
		"  SELECT id FROM nodes  ",
		"SELECT * FROM nodes;", // single trailing semicolon allowed
		"WITH x AS (SELECT id FROM nodes) SELECT * FROM x",
		"with cte as (select * from edges) select * from cte",
		"-- a leading comment\nSELECT 1",
		"/* block */ SELECT id FROM nodes",
		"SELECT replace(body,'a','b') FROM nodes",      // replace() is a function, not a write
		"SELECT id FROM nodes WHERE body LIKE 'a_b'",   // underscore inside a literal
		"SELECT id FROM node_props WHERE key='status'", // underscore mid-identifier (public view)
		"SELECT dst_key FROM edges",                    // underscore mid-identifier column
	}
	for _, q := range accept {
		if err := validateReadOnlySQL(q); err != nil {
			t.Errorf("expected accept, got reject for %q: %v", q, err)
		}
	}

	reject := []struct {
		name, sql, wantSubstr string
	}{
		{"empty", "   ", "empty"},
		{"comment only", "-- nothing", "empty"},
		{"insert", "INSERT INTO nodes VALUES (1)", "select"},
		{"update", "UPDATE nodes SET type='x'", "select"},
		{"delete", "DELETE FROM nodes", "select"},
		{"drop", "DROP VIEW nodes", "select"},
		{"create", "CREATE TABLE t(x)", "select"},
		{"alter", "ALTER TABLE t ADD COLUMN x", "select"},
		{"pragma", "PRAGMA writable_schema=1", "select"},
		{"vacuum", "VACUUM", "select"},
		{"attach", "ATTACH DATABASE 'x' AS y", "select"},
		{"multi statement", "SELECT 1; DELETE FROM nodes", "single"},
		{"multi select", "SELECT 1; SELECT 2", "single"},
		{"write cte", "WITH x AS (DELETE FROM _nodes RETURNING *) SELECT * FROM x", "write"},
		{"private table", "SELECT * FROM _nodes", "private"},
		{"private fts", "SELECT * FROM _fts", "private"},
		{"fts match", "SELECT id FROM fts WHERE fts MATCH 'hello'", "match"},
		{"bad leading keyword", "SELEKT * FROM nodes", "select"},
	}
	for _, tc := range reject {
		t.Run(tc.name, func(t *testing.T) {
			err := validateReadOnlySQL(tc.sql)
			if err == nil {
				t.Fatalf("expected reject for %q, got nil", tc.sql)
			}
			if !strings.Contains(strings.ToLower(err.Error()), tc.wantSubstr) {
				t.Errorf("error for %q = %q, want substring %q", tc.sql, err.Error(), tc.wantSubstr)
			}
		})
	}
}

// TestParseFields verifies --fields splitting and trimming.
func TestParseFields(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"ulid,type", []string{"ulid", "type"}},
		{" ulid , type ", []string{"ulid", "type"}},
		{"ulid,,type,", []string{"ulid", "type"}},
	}
	for _, c := range cases {
		got := parseFields(c.in)
		if len(got) != len(c.want) {
			t.Errorf("parseFields(%q) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("parseFields(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}
