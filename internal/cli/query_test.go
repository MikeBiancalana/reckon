// Package cli — TDD red tests for v1-T3 `rk query` (reckon-p0zs).
//
// Every test here is intentionally FAILING against the current stub in
// internal/cli/stubs.go (which returns "not yet implemented" for every query
// invocation and registers no local flags). When the real query.go is wired up
// the tests will go green.
//
// Frontmatter → props mapping (verified from internal/node/node.go and
// internal/index/index_test.go):
//   - Scalar frontmatter fields not in reservedKeys {id,type,time,author,aliases}
//     are stored in node_props, EXCEPT ref-valued fields like `depends: "[[X]]"`
//     which become typed edges (not props).
//   - Confirmed: `status: open` → node_props(id,'status','open')
//     (see TestRebuildPopulatesViews in internal/index/index_test.go).
//
// Harness note: RootCmd is a shared singleton. Each test must:
//   1. Register t.Cleanup(resetCLIFlags) immediately after setupQueryVault.
//   2. Call resetCLIFlags() between two Execute calls within the same test.
//
// sqlite driver: registered transitively via internal/cli/index.go →
// internal/index/index.go → _ "modernc.org/sqlite". No extra blank import needed.
//
// TODO(impl): add TestValidateReadOnlySQL once validateReadOnlySQL is exported/
// accessible. Drive it through RootCmd until then (see TestQueryTS2, TS3, TS14,
// TS15). The unit-test function should live in query_test.go once the production
// symbol exists; a direct table-driven call to validateReadOnlySQL(s) would verify
// the accept/reject/false-positive cases in plan.md "Layer 2 — statement".

package cli

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/index"
	"github.com/MikeBiancalana/reckon/internal/node"
)

// ─────────────────────────────────────────────────────────────────────────────
// Harness helpers
// ─────────────────────────────────────────────────────────────────────────────

// setupQueryVault creates a temp vault + sibling cache dir, sets RECKON_CACHE in
// the test environment, and returns (vault, cache) absolute paths.
func setupQueryVault(t *testing.T) (vault, cache string) {
	t.Helper()
	root := t.TempDir()
	vault = filepath.Join(root, "vault")
	cache = filepath.Join(root, "cache")
	if err := os.MkdirAll(vault, 0o755); err != nil {
		t.Fatalf("mkdir vault: %v", err)
	}
	// t.Setenv restores automatically at test cleanup.
	t.Setenv("RECKON_CACHE", cache)
	return vault, cache
}

// writeTestNode writes one markdown node file to the vault with the given id, type,
// and body. Extra frontmatter lines (e.g. "status: open") may be appended.
func writeTestNode(t *testing.T, vault, filename, id, typ, body string, extraFM ...string) {
	t.Helper()
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString("id: " + id + "\n")
	sb.WriteString("type: " + typ + "\n")
	for _, line := range extraFM {
		sb.WriteString(line + "\n")
	}
	sb.WriteString("---\n")
	sb.WriteString(body + "\n")
	full := filepath.Join(vault, filename)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir parent for %s: %v", filename, err)
	}
	if err := os.WriteFile(full, []byte(sb.String()), 0o644); err != nil {
		t.Fatalf("write %s: %v", filename, err)
	}
}

// resetCLIFlags resets all CLI package-global flag variables to defaults and
// clears the RootCmd IO writers/args. Call between Execute invocations AND in
// t.Cleanup.
func resetCLIFlags() {
	vaultFlag = ""
	jsonFlag = false
	ndjsonFlag = false
	quietFlag = false
	RootCmd.SetArgs(nil)
	RootCmd.SetOut(nil)
	RootCmd.SetErr(nil)
}

// buildIndex runs `rk index --vault <vault>` through RootCmd and fatals on any
// error. RECKON_CACHE must already be set via t.Setenv. Resets CLI state after.
func buildIndex(t *testing.T, vault string) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	RootCmd.SetOut(&outBuf)
	RootCmd.SetErr(&errBuf)
	RootCmd.SetArgs([]string{"index", "--vault", vault})
	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("buildIndex: rk index failed: %v\nstdout: %s\nstderr: %s",
			err, outBuf.String(), errBuf.String())
	}
	resetCLIFlags()
}

// runQuery executes `rk query --vault <vault> [args...]` through RootCmd and
// returns (stdout, stderr, error). The caller must call resetCLIFlags() if
// running another Execute in the same test.
func runQuery(t *testing.T, vault string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	RootCmd.SetOut(&outBuf)
	RootCmd.SetErr(&errBuf)
	RootCmd.SetArgs(append([]string{"query", "--vault", vault}, args...))
	err = RootCmd.Execute()
	return outBuf.String(), errBuf.String(), err
}

// countNDJSONLines counts non-empty lines in s.
func countNDJSONLines(s string) int {
	n := 0
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}

// parseNDJSONMaps decodes each non-empty line of s as map[string]any.
func parseNDJSONMaps(t *testing.T, s string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for i, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("line %d not valid JSON: %v\nline: %s", i+1, err, line)
		}
		out = append(out, m)
	}
	return out
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-1 (AC-1): Correct node results from corpus
// Red: stub returns errNotImplemented → err != nil → t.Fatalf fires.
// ─────────────────────────────────────────────────────────────────────────────

func TestQueryTS1_CorrectNodes(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	// 3 task nodes + 1 note
	writeTestNode(t, vault, "t1.md", node.Mint(), "task", "task body one")
	writeTestNode(t, vault, "t2.md", node.Mint(), "task", "task body two")
	writeTestNode(t, vault, "t3.md", node.Mint(), "task", "task body three")
	writeTestNode(t, vault, "n1.md", node.Mint(), "note", "note body")
	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault, "SELECT id FROM nodes WHERE type='task'")
	if err != nil {
		t.Fatalf("TS-1: expected no error, got: %v\nstderr: %s", err, stderr)
	}

	lines := countNDJSONLines(stdout)
	if lines != 3 {
		t.Errorf("TS-1: expected 3 NDJSON lines for type=task, got %d\nstdout: %s", lines, stdout)
	}

	// Each line must decode to an envelope with type="task"
	envs, parseErr := node.ReadNDJSON(strings.NewReader(stdout))
	if parseErr != nil {
		t.Fatalf("TS-1: ReadNDJSON: %v\nstdout: %s", parseErr, stdout)
	}
	for i, e := range envs {
		if e.Type != "task" {
			t.Errorf("TS-1: envelope[%d]: type=%q, want task", i, e.Type)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-2 (AC-2): Non-SELECT rejected — INSERT
// Red: stub returns errNotImplemented; combined lacks "select"/"only" → Errorf.
// ─────────────────────────────────────────────────────────────────────────────

func TestQueryTS2_InsertRejected(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	writeTestNode(t, vault, "a.md", node.Mint(), "note", "body")
	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault,
		"INSERT INTO nodes VALUES ('x','x','x','x','x','x','x')")
	if err == nil {
		t.Fatal("TS-2: expected error for INSERT, got nil")
	}
	if stdout != "" {
		t.Errorf("TS-2: stdout must be empty on rejection, got: %q", stdout)
	}

	combined := strings.ToLower(stdout + stderr + err.Error())
	// After implementation: "only SELECT statements are allowed" or similar.
	// Red state: "not yet implemented" has none of these → Errorf fires.
	if !strings.Contains(combined, "select") && !strings.Contains(combined, "only") {
		t.Errorf("TS-2: expected SELECT rejection message, got stdout=%q stderr=%q err=%v",
			stdout, stderr, err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-3 (AC-2): Non-SELECT rejected — DROP VIEW
// Red: stub returns errNotImplemented; combined lacks "select"/"not allowed" → Errorf.
// ─────────────────────────────────────────────────────────────────────────────

func TestQueryTS3_DropRejected(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	writeTestNode(t, vault, "a.md", node.Mint(), "note", "body")
	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault, "DROP VIEW nodes")
	if err == nil {
		t.Fatal("TS-3: expected error for DROP, got nil")
	}
	if stdout != "" {
		t.Errorf("TS-3: stdout must be empty on rejection, got: %q", stdout)
	}

	combined := strings.ToLower(stdout + stderr + err.Error())
	if !strings.Contains(combined, "select") && !strings.Contains(combined, "not allowed") {
		t.Errorf("TS-3: expected rejection message, got stdout=%q stderr=%q err=%v",
			stdout, stderr, err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-4 (AC-3 + AC-4): NDJSON output is parseable via node.ReadNDJSON
// Red: stub returns errNotImplemented → err != nil → t.Fatalf fires.
// ─────────────────────────────────────────────────────────────────────────────

func TestQueryTS4_NDJSONParseable(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	writeTestNode(t, vault, "a.md", node.Mint(), "note", "first note")
	writeTestNode(t, vault, "b.md", node.Mint(), "note", "second note")
	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault, "SELECT * FROM nodes")
	if err != nil {
		t.Fatalf("TS-4: expected no error, got: %v\nstderr: %s", err, stderr)
	}

	envs, parseErr := node.ReadNDJSON(strings.NewReader(stdout))
	if parseErr != nil {
		t.Fatalf("TS-4: ReadNDJSON failed: %v\nstdout: %s", parseErr, stdout)
	}
	if len(envs) != 2 {
		t.Errorf("TS-4: expected 2 envelopes, got %d", len(envs))
	}

	// Each line must be independently valid JSON
	for i, line := range strings.Split(strings.TrimRight(stdout, "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !json.Valid([]byte(line)) {
			t.Errorf("TS-4: line %d not valid JSON: %q", i+1, line)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-5 (AC-4): Canonical envelope fields present for a specific node
// Red: stub returns errNotImplemented → err != nil → t.Fatalf fires.
// ─────────────────────────────────────────────────────────────────────────────

func TestQueryTS5_EnvelopeFields(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	knownID := node.Mint()
	// status is a non-reserved scalar frontmatter key → stored in node_props
	// (confirmed in index_test.go:TestRebuildPopulatesViews)
	writeTestNode(t, vault, "known.md", knownID, "task", "known body here",
		"status: open")
	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault,
		fmt.Sprintf("SELECT * FROM nodes WHERE id='%s'", knownID))
	if err != nil {
		t.Fatalf("TS-5: expected no error, got: %v\nstderr: %s", err, stderr)
	}

	lines := countNDJSONLines(stdout)
	if lines != 1 {
		t.Fatalf("TS-5: expected 1 NDJSON line, got %d\nstdout: %s", lines, stdout)
	}

	// Take the first non-empty line
	var firstLine string
	for _, l := range strings.Split(stdout, "\n") {
		if strings.TrimSpace(l) != "" {
			firstLine = l
			break
		}
	}

	env, parseErr := node.Unmarshal([]byte(firstLine))
	if parseErr != nil {
		t.Fatalf("TS-5: Unmarshal: %v", parseErr)
	}

	if env.V != node.EnvelopeVersion {
		t.Errorf("TS-5: V = %d, want %d", env.V, node.EnvelopeVersion)
	}
	if env.ULID != knownID {
		t.Errorf("TS-5: ULID = %q, want %q", env.ULID, knownID)
	}
	if env.Type != "task" {
		t.Errorf("TS-5: Type = %q, want task", env.Type)
	}
	if !strings.Contains(env.Body, "known body") {
		t.Errorf("TS-5: Body = %q, want to contain 'known body'", env.Body)
	}
	// Props: status=open should be present (non-reserved scalar → node_props)
	if env.Props == nil || env.Props["status"] != "open" {
		t.Errorf("TS-5: Props[status] = %q, want 'open'; all props: %v",
			env.Props["status"], env.Props)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-6 (AC-5): Raw-rows mode for aggregates (--raw flag)
// Red: --raw is unknown → cobra "unknown flag: --raw" → err != nil → t.Fatalf.
// ─────────────────────────────────────────────────────────────────────────────

func TestQueryTS6_RawAggregate(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	writeTestNode(t, vault, "t1.md", node.Mint(), "task", "task one")
	writeTestNode(t, vault, "t2.md", node.Mint(), "task", "task two")
	writeTestNode(t, vault, "n1.md", node.Mint(), "note", "note one")
	buildIndex(t, vault)

	// --raw is unknown in red state; cobra rejects with "unknown flag: --raw"
	stdout, stderr, err := runQuery(t, vault,
		"--raw",
		"SELECT type, COUNT(*) AS n FROM nodes GROUP BY type",
	)
	if err != nil {
		t.Fatalf("TS-6: --raw aggregate failed: %v\nstderr: %s", err, stderr)
	}

	rows := parseNDJSONMaps(t, stdout)
	if len(rows) == 0 {
		t.Fatalf("TS-6: expected at least 1 raw row, got 0\nstdout: %s", stdout)
	}
	for i, row := range rows {
		if _, has := row["type"]; !has {
			t.Errorf("TS-6: row[%d] missing key 'type': %v", i, row)
		}
		if _, has := row["n"]; !has {
			t.Errorf("TS-6: row[%d] missing key 'n': %v", i, row)
		}
		// Raw mode must not inject envelope versioning
		if _, hasV := row["v"]; hasV {
			t.Errorf("TS-6: row[%d] has envelope key 'v' in --raw mode: %v", i, row)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-7 (AC-6): Named saved view executes (--view flag)
// Red: --view is unknown → cobra rejects → errView != nil → t.Fatalf fires.
// ─────────────────────────────────────────────────────────────────────────────

func TestQueryTS7_SavedView(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	writeTestNode(t, vault, "t1.md", node.Mint(), "task", "task one")
	writeTestNode(t, vault, "n1.md", node.Mint(), "note", "note one")
	buildIndex(t, vault)

	// Write view file at VaultDir/.reckon/views/<name>.sql (per plan.md)
	viewsDir := filepath.Join(vault, ".reckon", "views")
	if err := os.MkdirAll(viewsDir, 0o755); err != nil {
		t.Fatalf("TS-7: mkdir views: %v", err)
	}
	viewSQL := "SELECT id FROM nodes WHERE type='task'"
	if err := os.WriteFile(filepath.Join(viewsDir, "tasks.sql"), []byte(viewSQL), 0o644); err != nil {
		t.Fatalf("TS-7: write view file: %v", err)
	}

	stdoutView, _, errView := runQuery(t, vault, "--view", "tasks")
	if errView != nil {
		t.Fatalf("TS-7: --view tasks failed: %v", errView)
	}
	resetCLIFlags()

	// Compare with running the SQL directly
	stdoutSQL, _, errSQL := runQuery(t, vault, viewSQL)
	if errSQL != nil {
		t.Fatalf("TS-7: direct SQL failed: %v", errSQL)
	}

	if stdoutView != stdoutSQL {
		t.Errorf("TS-7: --view output differs from inline SQL\nview:   %q\ninline: %q",
			stdoutView, stdoutSQL)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-8 (EC-6): Missing named view returns error mentioning the name
// Red: --view unknown → cobra error lacks "ghost" → Errorf fires.
// ─────────────────────────────────────────────────────────────────────────────

func TestQueryTS8_MissingView(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	writeTestNode(t, vault, "a.md", node.Mint(), "note", "body")
	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault, "--view", "ghost")
	if err == nil {
		t.Fatal("TS-8: expected error for missing view 'ghost', got nil")
	}
	if stdout != "" {
		t.Errorf("TS-8: expected empty stdout on missing view, got: %q", stdout)
	}

	combined := stdout + stderr + err.Error()
	if !strings.Contains(combined, "ghost") {
		t.Errorf("TS-8: expected error to mention 'ghost', got:\nstdout=%q\nstderr=%q\nerr=%v",
			stdout, stderr, err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-9 (AC-8): --lang sql is equivalent to default
// Red: --lang unknown → cobra rejects first call → errLang != nil → t.Fatalf.
// ─────────────────────────────────────────────────────────────────────────────

func TestQueryTS9_LangSQL(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	writeTestNode(t, vault, "a.md", node.Mint(), "note", "body a")
	buildIndex(t, vault)

	stdoutLang, _, errLang := runQuery(t, vault, "--lang", "sql", "SELECT id FROM nodes")
	if errLang != nil {
		t.Fatalf("TS-9: --lang sql failed: %v", errLang)
	}
	resetCLIFlags()

	stdoutDefault, _, errDefault := runQuery(t, vault, "SELECT id FROM nodes")
	if errDefault != nil {
		t.Fatalf("TS-9: default lang failed: %v", errDefault)
	}

	if stdoutLang != stdoutDefault {
		t.Errorf("TS-9: --lang sql output differs from default:\nlang:    %q\ndefault: %q",
			stdoutLang, stdoutDefault)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-10 (AC-8 / EC-14): --lang <unsupported> exits with error
// Red: --lang unknown → cobra rejects → err contains "unknown flag" not "unsupported"/
//      "python". Errorf fires on the message check.
// ─────────────────────────────────────────────────────────────────────────────

func TestQueryTS10_LangUnsupported(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	writeTestNode(t, vault, "a.md", node.Mint(), "note", "body")
	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault, "--lang", "python", "some query")
	if err == nil {
		t.Fatal("TS-10: expected error for --lang python, got nil")
	}
	if stdout != "" {
		t.Errorf("TS-10: expected empty stdout, got: %q", stdout)
	}

	combined := strings.ToLower(stdout + stderr + err.Error())
	// After implementation: "unsupported language: python" (plan.md AC-8).
	// Red state: "unknown flag: --lang" → "unknown" present but not "unsupported"/"python".
	if !strings.Contains(combined, "unsupported") && !strings.Contains(combined, "python") {
		t.Errorf("TS-10: expected 'unsupported'/'python' in error:\nstdout=%q\nstderr=%q\nerr=%v",
			stdout, stderr, err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-11 (AC-9 + AC-10): Connection-level read-only enforced — direct DB test
//
// This test opens the index db directly using the same file: URI mechanism that
// openReadOnlyIndex() will use (plan.md "Layer 1 — connection"). It does NOT go
// through RootCmd, so it can PASS in red state — that is intentional: the SQLite
// infrastructure must be correct before the command wraps it.
// ─────────────────────────────────────────────────────────────────────────────

func TestQueryTS11_ReadOnlyConnection(t *testing.T) {
	vault, cache := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	writeTestNode(t, vault, "a.md", node.Mint(), "note", "body")
	buildIndex(t, vault)

	cfg, err := config.LoadWithOverrides(vault, cache)
	if err != nil {
		t.Fatalf("TS-11: LoadWithOverrides: %v", err)
	}
	dbPath, err := index.DBPath(cfg)
	if err != nil {
		t.Fatalf("TS-11: DBPath: %v", err)
	}

	// Build file: URI with mode=ro — the exact form the plan requires
	// (plain-path ?mode=ro is silently ignored by modernc.org/sqlite; see plan.md).
	absPath, err := filepath.Abs(dbPath)
	if err != nil {
		t.Fatalf("TS-11: filepath.Abs: %v", err)
	}
	u := url.URL{Scheme: "file", Path: absPath}
	u.RawQuery = "mode=ro"

	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		t.Fatalf("TS-11: sql.Open read-only: %v", err)
	}
	defer db.Close()

	_, execErr := db.Exec("INSERT INTO _nodes(node_key,loc_file,hash,mtime) VALUES('ts11-ro','x','x',0)")
	if execErr == nil {
		t.Fatal("TS-11: expected readonly error from INSERT on read-only connection, got nil")
	}
	if !strings.Contains(strings.ToLower(execErr.Error()), "readonly") {
		t.Errorf("TS-11: expected 'readonly' in error, got: %v", execErr)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-12 (EC-1): Empty result set → exit 0, empty stdout, empty stderr
// Red: stub returns errNotImplemented → err != nil → t.Fatalf fires.
// ─────────────────────────────────────────────────────────────────────────────

func TestQueryTS12_EmptyResult(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	writeTestNode(t, vault, "a.md", node.Mint(), "note", "body")
	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault, "SELECT * FROM nodes WHERE type='phantom'")
	if err != nil {
		t.Fatalf("TS-12: expected no error for empty result, got: %v\nstderr: %s", err, stderr)
	}
	if stdout != "" {
		t.Errorf("TS-12: expected empty stdout for empty result, got: %q", stdout)
	}
	if stderr != "" {
		t.Errorf("TS-12: expected empty stderr for empty result, got: %q", stderr)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-13 (EC-2): Syntactically invalid SQL → non-zero exit, error on stderr
// Red sub-test 1: combined lacks "select" (stub says "not yet implemented") → Errorf.
// Red sub-test 2: combined lacks "syntax"/"near"/"sql" → Errorf.
// ─────────────────────────────────────────────────────────────────────────────

func TestQueryTS13_InvalidSQL(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	writeTestNode(t, vault, "a.md", node.Mint(), "note", "body")
	buildIndex(t, vault)

	t.Run("bad_leading_keyword", func(t *testing.T) {
		// Validator rejects: first token "SELEKT" ≠ SELECT/WITH.
		stdout, stderr, err := runQuery(t, vault, "SELEKT * FORM nodes")
		if err == nil {
			t.Fatal("TS-13a: expected error for SELEKT, got nil")
		}
		if stdout != "" {
			t.Errorf("TS-13a: expected empty stdout, got: %q", stdout)
		}
		combined := strings.ToLower(stdout + stderr + err.Error())
		// After implementation: "only SELECT statements are allowed" contains "select".
		// Red: "not yet implemented" lacks "select" → Errorf.
		if !strings.Contains(combined, "select") {
			t.Errorf("TS-13a: expected SELECT validation error, got stdout=%q stderr=%q err=%v",
				stdout, stderr, err)
		}
	})

	t.Run("valid_prefix_bad_syntax", func(t *testing.T) {
		resetCLIFlags()
		// Validator passes (leading SELECT), SQLite returns syntax error.
		stdout, stderr, err := runQuery(t, vault, "SELECT * FORM nodes")
		if err == nil {
			t.Fatal("TS-13b: expected syntax error for 'SELECT * FORM nodes', got nil")
		}
		if stdout != "" {
			t.Errorf("TS-13b: expected empty stdout, got: %q", stdout)
		}
		combined := strings.ToLower(stdout + stderr + err.Error())
		// After implementation: SQLite "near 'nodes': syntax error" or similar.
		// Red: "not yet implemented" lacks these → Errorf.
		if !strings.Contains(combined, "syntax") && !strings.Contains(combined, "near") &&
			!strings.Contains(combined, "sql") {
			t.Errorf("TS-13b: expected SQL syntax error, got stdout=%q stderr=%q err=%v",
				stdout, stderr, err)
		}
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-14 (EC-3): Multi-statement input rejected
// Red: combined lacks "single"/"statement"/"select" → Errorf fires.
// ─────────────────────────────────────────────────────────────────────────────

func TestQueryTS14_MultiStatement(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	writeTestNode(t, vault, "a.md", node.Mint(), "note", "body")
	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault, "SELECT 1; DELETE FROM nodes")
	if err == nil {
		t.Fatal("TS-14: expected error for multi-statement, got nil")
	}
	if stdout != "" {
		t.Errorf("TS-14: expected empty stdout, got: %q", stdout)
	}

	combined := strings.ToLower(stdout + stderr + err.Error())
	// After implementation: "only a single SELECT statement is allowed" (plan.md IR-2).
	// Red: "not yet implemented" lacks "single"/"statement"/"select" → Errorf.
	if !strings.Contains(combined, "single") && !strings.Contains(combined, "statement") &&
		!strings.Contains(combined, "select") {
		t.Errorf("TS-14: expected multi-statement rejection, got stdout=%q stderr=%q err=%v",
			stdout, stderr, err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-15 (EC-4): Write-CTE rejected
// Red: combined lacks "delete"/"write"/"not allowed" → Errorf fires.
// ─────────────────────────────────────────────────────────────────────────────

func TestQueryTS15_WriteCTE(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	writeTestNode(t, vault, "a.md", node.Mint(), "note", "body")
	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault,
		"WITH x AS (DELETE FROM _nodes RETURNING *) SELECT * FROM x")
	if err == nil {
		t.Fatal("TS-15: expected error for write-CTE, got nil")
	}
	if stdout != "" {
		t.Errorf("TS-15: expected empty stdout on write-CTE rejection, got: %q", stdout)
	}

	combined := strings.ToLower(stdout + stderr + err.Error())
	// After implementation: "write/DDL statements are not allowed" or "_nodes" private table
	// rejection. Red: "not yet implemented" lacks these → Errorf.
	if !strings.Contains(combined, "delete") && !strings.Contains(combined, "write") &&
		!strings.Contains(combined, "not allowed") && !strings.Contains(combined, "private") {
		t.Errorf("TS-15: expected write-CTE rejection, got stdout=%q stderr=%q err=%v",
			stdout, stderr, err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-16 (EC-7): --limit 0 returns zero rows, exit 0
// Red: --limit unknown → cobra error → err != nil → t.Fatalf fires.
// ─────────────────────────────────────────────────────────────────────────────

func TestQueryTS16_LimitZero(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	for i := 0; i < 5; i++ {
		writeTestNode(t, vault, fmt.Sprintf("n%d.md", i), node.Mint(), "note",
			fmt.Sprintf("body %d", i))
	}
	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault, "--limit", "0", "SELECT * FROM nodes")
	if err != nil {
		t.Fatalf("TS-16: --limit 0 must succeed, got: %v\nstderr: %s", err, stderr)
	}
	if stdout != "" {
		t.Errorf("TS-16: expected empty stdout for --limit 0, got: %q", stdout)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-17 (EC-8): --limit -1 is a usage error
// Red: --limit unknown → cobra "unknown flag: --limit" error; the message
//      contains "limit" but NOT "negative"/"invalid"/"must be" → Errorf fires.
// ─────────────────────────────────────────────────────────────────────────────

func TestQueryTS17_LimitNegative(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	writeTestNode(t, vault, "a.md", node.Mint(), "note", "body")
	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault, "--limit", "-1", "SELECT * FROM nodes")
	if err == nil {
		t.Fatal("TS-17: expected error for --limit -1, got nil")
	}
	if stdout != "" {
		t.Errorf("TS-17: expected empty stdout, got: %q", stdout)
	}

	combined := strings.ToLower(stdout + stderr + err.Error())
	if !strings.Contains(combined, "limit") {
		t.Errorf("TS-17: expected 'limit' in error, got stdout=%q stderr=%q err=%v",
			stdout, stderr, err)
	}
	// After implementation: "limit must be >= 0" / "invalid" / "non-negative".
	// Red: cobra only says "unknown flag: --limit" → no "negative"/"invalid"/"must be".
	if !strings.Contains(combined, "negative") && !strings.Contains(combined, "invalid") &&
		!strings.Contains(combined, "must be") {
		t.Errorf("TS-17: expected 'negative'/'invalid'/'must be' in error (not just flag name), got err=%v",
			err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-18 (AC-11): --fields limits envelope keys; unknown field → warning, omitted
// Red: --fields unknown → cobra rejects → err != nil → t.Fatalf fires.
// ─────────────────────────────────────────────────────────────────────────────

func TestQueryTS18_Fields(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	writeTestNode(t, vault, "a.md", node.Mint(), "task", "task body a")
	writeTestNode(t, vault, "b.md", node.Mint(), "task", "task body b")
	buildIndex(t, vault)

	t.Run("requested_fields_only", func(t *testing.T) {
		// --fields ulid,type → each output line: {v, ulid, type} only.
		// "id" is accepted per plan note (injected identity key).
		stdout, _, err := runQuery(t, vault, "--fields", "ulid,type", "SELECT * FROM nodes")
		if err != nil {
			t.Fatalf("TS-18a: expected no error, got: %v", err)
		}

		rows := parseNDJSONMaps(t, stdout)
		if len(rows) == 0 {
			t.Fatalf("TS-18a: expected rows, got none\nstdout: %s", stdout)
		}

		// Keys allowed: "v" (always retained per plan), "ulid" (requested),
		// "type" (requested), "id" (plan note: injected identity — allowed).
		allowed := map[string]bool{"v": true, "ulid": true, "type": true, "id": true}
		for i, row := range rows {
			for k := range row {
				if !allowed[k] {
					t.Errorf("TS-18a: row[%d] has unrequested key %q", i, k)
				}
			}
			if _, has := row["ulid"]; !has {
				t.Errorf("TS-18a: row[%d] missing requested key 'ulid'", i)
			}
			if _, has := row["type"]; !has {
				t.Errorf("TS-18a: row[%d] missing requested key 'type'", i)
			}
			if _, has := row["body"]; has {
				t.Errorf("TS-18a: row[%d] has unrequested key 'body'", i)
			}
			if _, has := row["author"]; has {
				t.Errorf("TS-18a: row[%d] has unrequested key 'author'", i)
			}
		}
	})

	t.Run("bogus_field_omitted", func(t *testing.T) {
		resetCLIFlags()
		// --fields ulid,bogus → "bogus" absent; query still succeeds (EC-9).
		stdout, _, err := runQuery(t, vault, "--fields", "ulid,bogus", "SELECT * FROM nodes")
		if err != nil {
			t.Fatalf("TS-18b: expected no error with unknown field, got: %v", err)
		}
		rows := parseNDJSONMaps(t, stdout)
		for i, row := range rows {
			if _, hasBogus := row["bogus"]; hasBogus {
				t.Errorf("TS-18b: row[%d] 'bogus' present, expected omission", i)
			}
			// "ulid" must still be present
			if _, hasULID := row["ulid"]; !hasULID {
				t.Errorf("TS-18b: row[%d] missing 'ulid'", i)
			}
		}
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-19 (AC-12): --limit N caps output rows
// Red: --limit unknown → cobra rejects → err != nil → t.Fatalf fires.
// ─────────────────────────────────────────────────────────────────────────────

func TestQueryTS19_Limit(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	for i := 0; i < 10; i++ {
		writeTestNode(t, vault, fmt.Sprintf("n%02d.md", i), node.Mint(), "note",
			fmt.Sprintf("body %d", i))
	}
	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault, "--limit", "3", "SELECT * FROM nodes")
	if err != nil {
		t.Fatalf("TS-19: expected no error for --limit 3, got: %v\nstderr: %s", err, stderr)
	}

	lines := countNDJSONLines(stdout)
	if lines != 3 {
		t.Errorf("TS-19: expected exactly 3 lines with --limit 3, got %d\nstdout: %s",
			lines, stdout)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-20 (EC-10): Missing / unbuilt index → clear error mentioning "index"
// Red: combined lacks "index not found"/"rk index" → Errorf fires.
//      ("not yet implemented" does not contain "index".)
// ─────────────────────────────────────────────────────────────────────────────

func TestQueryTS20_MissingIndex(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	// Intentionally do NOT call buildIndex → no index.db exists.

	stdout, stderr, err := runQuery(t, vault, "SELECT * FROM nodes")
	if err == nil {
		t.Fatal("TS-20: expected error when index not built, got nil")
	}
	if stdout != "" {
		t.Errorf("TS-20: expected empty stdout, got: %q", stdout)
	}

	combined := strings.ToLower(stdout + stderr + err.Error())
	// After implementation: "index not found" / "run 'rk index'" (plan.md IR-5).
	// Red: "not yet implemented" has no "index" → Errorf.
	if !strings.Contains(combined, "index") {
		t.Errorf("TS-20: expected 'index' in error message, got stdout=%q stderr=%q err=%v",
			stdout, stderr, err)
	}
	// Bonus: actionable hint to run rk index
	if !strings.Contains(combined, "rk index") && !strings.Contains(combined, "run") {
		t.Logf("TS-20: note: error should suggest running 'rk index'; got: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-21 (EC-11): FTS MATCH → error propagated (not silently empty)
// Red: combined lacks "match"/"fts"/"not supported"/"context" → Errorf fires.
// ─────────────────────────────────────────────────────────────────────────────

func TestQueryTS21_FTSMatch(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	writeTestNode(t, vault, "a.md", node.Mint(), "note", "hello world")
	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault, "SELECT id FROM fts WHERE fts MATCH 'hello'")

	// Must NOT silently return empty output with no error
	if err == nil && stdout == "" && stderr == "" {
		t.Fatal("TS-21: MATCH on fts view returned no error and no output — must produce an error")
	}

	var errStr string
	if err != nil {
		errStr = err.Error()
	}
	combined := strings.ToLower(stdout + stderr + errStr)
	// After implementation: plan.md rejects MATCH with "full-text MATCH is not supported
	// in v1" OR forwards SQLite's "unable to use function MATCH in the requested context".
	// Red: "not yet implemented" lacks all of these → Errorf.
	if !strings.Contains(combined, "match") && !strings.Contains(combined, "fts") &&
		!strings.Contains(combined, "not supported") && !strings.Contains(combined, "context") {
		t.Errorf("TS-21: expected MATCH-specific error message, got stdout=%q stderr=%q err=%v",
			stdout, stderr, err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-22 (EC-12): Broken pipe handled gracefully — Execute returns nil
// brokenPipeWriter allows 1 successful Write, then returns io.ErrClosedPipe.
// Red: stub returns errNotImplemented immediately → Execute returns non-nil → Errorf.
// ─────────────────────────────────────────────────────────────────────────────

// brokenPipeWriter simulates a downstream consumer closing the pipe after the
// first write. Used to verify isBrokenPipe handling in the query command.
type brokenPipeWriter struct {
	n int // count of Write calls
}

func (w *brokenPipeWriter) Write(p []byte) (int, error) {
	w.n++
	if w.n > 1 {
		return 0, io.ErrClosedPipe
	}
	return len(p), nil
}

func TestQueryTS22_BrokenPipe(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	// 10 nodes → multiple NDJSON writes expected after implementation
	for i := 0; i < 10; i++ {
		writeTestNode(t, vault, fmt.Sprintf("n%02d.md", i), node.Mint(), "note",
			fmt.Sprintf("body %d", i))
	}
	buildIndex(t, vault)

	var errBuf bytes.Buffer
	bpw := &brokenPipeWriter{}
	RootCmd.SetOut(bpw)
	RootCmd.SetErr(&errBuf)
	RootCmd.SetArgs([]string{"query", "--vault", vault, "SELECT * FROM nodes"})

	execErr := RootCmd.Execute()

	// Broken pipe must be handled gracefully: Execute returns nil.
	if execErr != nil {
		t.Errorf("TS-22: broken pipe: expected Execute() to return nil, got: %v", execErr)
	}
	// No "broken pipe" text on stderr
	if strings.Contains(strings.ToLower(errBuf.String()), "broken pipe") {
		t.Errorf("TS-22: stderr must not mention 'broken pipe', got: %q", errBuf.String())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// EC-13: --json and --ndjson together → mutually exclusive error
// Note: this is enforced by root.go PersistentPreRunE and will PASS in red state
// (existing functionality). Included for completeness and regression coverage.
// ─────────────────────────────────────────────────────────────────────────────

func TestQueryEC13_JSONAndNDJSON(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	writeTestNode(t, vault, "a.md", node.Mint(), "note", "body")
	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault, "--json", "--ndjson", "SELECT * FROM nodes")
	if err == nil {
		t.Fatal("EC-13: expected error for --json --ndjson, got nil")
	}
	if stdout != "" {
		t.Errorf("EC-13: expected empty stdout, got: %q", stdout)
	}

	combined := strings.ToLower(stdout + stderr + err.Error())
	if !strings.Contains(combined, "exclusive") {
		t.Errorf("EC-13: expected 'exclusive' in error, got stdout=%q stderr=%q err=%v",
			stdout, stderr, err)
	}
}
