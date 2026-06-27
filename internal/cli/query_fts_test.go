// Package cli — TDD red tests for reckon-a4eh: sanctioned FTS5 MATCH surface.
//
// The sanctioned surface is the PUBLIC fts5 vtable `fts_search(id, body)`:
//
//	SELECT id FROM fts_search WHERE fts_search MATCH 'term'
//
// Results carry an `id` column so they flow through the canonical-node NDJSON
// path, identical to `SELECT id FROM nodes WHERE ...`.
//
// These tests are RED against the pre-implementation tree: `validateReadOnlySQL`
// rejects any MATCH ("not supported in v1") and no `fts_search` object exists.
// They go GREEN once `_fts` is promoted to the public `fts_search` vtable and the
// matchRe guard is removed.
//
// Standard harness: setupQueryVault → t.Cleanup(resetCLIFlags) → writeTestNode →
// buildIndex → runQuery (see query_test.go).

package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/MikeBiancalana/reckon/internal/node"
)

// FTS-1: basic MATCH returns matching nodes as canonical NDJSON.
func TestQueryFTS1_BasicMatch(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	writeTestNode(t, vault, "a.md", node.Mint(), "note", "hello world")
	writeTestNode(t, vault, "b.md", node.Mint(), "note", "hello again")
	writeTestNode(t, vault, "c.md", node.Mint(), "note", "no match here")
	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault, "SELECT id FROM fts_search WHERE fts_search MATCH 'hello'")
	if err != nil {
		t.Fatalf("FTS-1: unexpected error: %v\nstderr: %s", err, stderr)
	}
	if n := countNDJSONLines(stdout); n != 2 {
		t.Fatalf("FTS-1: expected 2 results, got %d\nstdout: %s", n, stdout)
	}
	envs, perr := node.ReadNDJSON(strings.NewReader(stdout))
	if perr != nil {
		t.Fatalf("FTS-1: ReadNDJSON: %v\nstdout: %s", perr, stdout)
	}
	for i, e := range envs {
		if !strings.Contains(e.Body, "hello") {
			t.Errorf("FTS-1: env[%d] body %q does not contain 'hello'", i, e.Body)
		}
	}
}

// FTS-2: no matches → exit 0, empty stdout/stderr (not an error).
func TestQueryFTS2_NoMatches(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	writeTestNode(t, vault, "a.md", node.Mint(), "note", "hello world")
	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault, "SELECT id FROM fts_search WHERE fts_search MATCH 'xyzzy_nonexistent'")
	if err != nil {
		t.Fatalf("FTS-2: expected no error, got: %v\nstderr: %s", err, stderr)
	}
	if stdout != "" {
		t.Errorf("FTS-2: expected empty stdout, got: %q", stdout)
	}
	if stderr != "" {
		t.Errorf("FTS-2: expected empty stderr, got: %q", stderr)
	}
}

// FTS-3: MATCH composes with --limit.
func TestQueryFTS3_Limit(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	for i := 0; i < 5; i++ {
		writeTestNode(t, vault, "n"+string(rune('0'+i))+".md", node.Mint(), "note", "common content")
	}
	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault, "SELECT id FROM fts_search WHERE fts_search MATCH 'common'", "--limit", "2")
	if err != nil {
		t.Fatalf("FTS-3: %v\nstderr: %s", err, stderr)
	}
	if n := countNDJSONLines(stdout); n != 2 {
		t.Errorf("FTS-3: expected 2 lines with --limit 2, got %d\nstdout: %s", n, stdout)
	}
}

// FTS-4: MATCH composes with --fields (canonical envelope key restriction).
func TestQueryFTS4_Fields(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	writeTestNode(t, vault, "a.md", node.Mint(), "note", "hello world")
	buildIndex(t, vault)

	stdout, _, err := runQuery(t, vault, "SELECT id FROM fts_search WHERE fts_search MATCH 'hello'", "--fields", "type")
	if err != nil {
		t.Fatalf("FTS-4: %v", err)
	}
	rows := parseNDJSONMaps(t, stdout)
	if len(rows) == 0 {
		t.Fatal("FTS-4: expected at least 1 result")
	}
	for i, row := range rows {
		if _, has := row["body"]; has {
			t.Errorf("FTS-4: row[%d] has unrequested 'body'", i)
		}
		if _, has := row["type"]; !has {
			t.Errorf("FTS-4: row[%d] missing requested 'type'", i)
		}
	}
}

// FTS-5: MATCH composes with --raw (no envelope reconstruction; no 'v' key).
func TestQueryFTS5_Raw(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	writeTestNode(t, vault, "a.md", node.Mint(), "note", "hello world")
	buildIndex(t, vault)

	stdout, _, err := runQuery(t, vault, "SELECT id, body FROM fts_search WHERE fts_search MATCH 'hello'", "--raw")
	if err != nil {
		t.Fatalf("FTS-5: %v", err)
	}
	rows := parseNDJSONMaps(t, stdout)
	if len(rows) == 0 {
		t.Fatal("FTS-5: expected at least 1 result")
	}
	for i, row := range rows {
		if _, has := row["v"]; has {
			t.Errorf("FTS-5: row[%d] has envelope key 'v' in --raw mode", i)
		}
		if _, has := row["id"]; !has {
			t.Errorf("FTS-5: row[%d] missing 'id'", i)
		}
	}
}

// FTS-7: an invalid FTS5 query surfaces a non-empty error, not silent empty.
func TestQueryFTS7_SyntaxError(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	writeTestNode(t, vault, "a.md", node.Mint(), "note", "hello")
	buildIndex(t, vault)

	stdout, _, err := runQuery(t, vault, "SELECT id FROM fts_search WHERE fts_search MATCH 'AND'")
	if err == nil {
		t.Fatal("FTS-7: expected error for invalid FTS5 query 'AND', got nil")
	}
	if stdout != "" {
		t.Errorf("FTS-7: expected empty stdout, got: %q", stdout)
	}
}

// FTS-9: FTS5 prefix operator passes through unescaped.
func TestQueryFTS9_Prefix(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	writeTestNode(t, vault, "a.md", node.Mint(), "note", "hello world")
	writeTestNode(t, vault, "b.md", node.Mint(), "note", "no match")
	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault, "SELECT id FROM fts_search WHERE fts_search MATCH 'hell*'")
	if err != nil {
		t.Fatalf("FTS-9: %v\nstderr: %s", err, stderr)
	}
	if n := countNDJSONLines(stdout); n != 1 {
		t.Errorf("FTS-9: expected 1 result for 'hell*', got %d\nstdout: %s", n, stdout)
	}
}

// FTS-15: --json with MATCH emits a single JSON array.
func TestQueryFTS15_JSON(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	writeTestNode(t, vault, "a.md", node.Mint(), "note", "hello world")
	writeTestNode(t, vault, "b.md", node.Mint(), "note", "hello again")
	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault, "--json", "SELECT id FROM fts_search WHERE fts_search MATCH 'hello'")
	if err != nil {
		t.Fatalf("FTS-15: %v\nstderr: %s", err, stderr)
	}
	var arr []json.RawMessage
	if uerr := json.Unmarshal([]byte(stdout), &arr); uerr != nil {
		t.Fatalf("FTS-15: stdout is not a JSON array: %v\nstdout: %s", uerr, stdout)
	}
	if len(arr) != 2 {
		t.Errorf("FTS-15: expected 2 array elements, got %d\nstdout: %s", len(arr), stdout)
	}
}

// FTS-bm25: the bm25() ranking auxiliary function works against the public vtable.
func TestQueryFTSBM25Order(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	writeTestNode(t, vault, "a.md", node.Mint(), "note", "hello world")
	writeTestNode(t, vault, "b.md", node.Mint(), "note", "hello hello hello")
	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault,
		"SELECT id FROM fts_search WHERE fts_search MATCH 'hello' ORDER BY bm25(fts_search)")
	if err != nil {
		t.Fatalf("FTS-bm25: %v\nstderr: %s", err, stderr)
	}
	if n := countNDJSONLines(stdout); n != 2 {
		t.Errorf("FTS-bm25: expected 2 ranked results, got %d\nstdout: %s", n, stdout)
	}
}

// FTS-private: the underlying private vtable name stays unreachable from user SQL.
func TestQueryFTSPrivateBlocked(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	writeTestNode(t, vault, "a.md", node.Mint(), "note", "hello")
	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault, "SELECT * FROM _fts WHERE _fts MATCH 'hello'")
	if err == nil {
		t.Fatal("FTS-private: expected rejection of private _fts access, got nil")
	}
	if stdout != "" {
		t.Errorf("FTS-private: expected empty stdout, got: %q", stdout)
	}
	combined := strings.ToLower(stdout + stderr + err.Error())
	if !strings.Contains(combined, "private") {
		t.Errorf("FTS-private: expected 'private' in error, got: %v", err)
	}
}
