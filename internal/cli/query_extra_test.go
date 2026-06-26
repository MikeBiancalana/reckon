package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/index"
	"github.com/MikeBiancalana/reckon/internal/node"
)

// TestQueryAliasesReconstructed (AC-4 / IR-12): a node's aliases must appear in
// the reconstructed canonical envelope. Closes a gap left by TS-5, which only
// covered props.
func TestQueryAliasesReconstructed(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	knownID := node.Mint()
	// aliases is a reserved frontmatter key → populates the aliases view
	// (confirmed in internal/index/index_test.go).
	writeTestNode(t, vault, "aliased.md", knownID, "note", "aliased body",
		"aliases: alpha")
	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault,
		"SELECT id FROM nodes WHERE id='"+knownID+"'")
	if err != nil {
		t.Fatalf("aliases: query failed: %v\nstderr: %s", err, stderr)
	}

	envs, perr := node.ReadNDJSON(strings.NewReader(stdout))
	if perr != nil {
		t.Fatalf("aliases: ReadNDJSON: %v\nstdout: %s", perr, stdout)
	}
	if len(envs) != 1 {
		t.Fatalf("aliases: expected 1 envelope, got %d", len(envs))
	}
	found := false
	for _, a := range envs[0].Aliases {
		if a == "alpha" {
			found = true
		}
	}
	if !found {
		t.Errorf("aliases: expected 'alpha' in Aliases, got %v", envs[0].Aliases)
	}
}

// TestQueryJSONArray (IR-10): --json emits a single JSON array of objects, not
// NDJSON. Complements EC-13 (which only covers the --json --ndjson conflict).
func TestQueryJSONArray(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	writeTestNode(t, vault, "a.md", node.Mint(), "note", "body a")
	writeTestNode(t, vault, "b.md", node.Mint(), "note", "body b")
	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault, "--json", "SELECT * FROM nodes")
	if err != nil {
		t.Fatalf("json: query failed: %v\nstderr: %s", err, stderr)
	}

	// The whole output must parse as one JSON array (NDJSON would not).
	var arr []map[string]any
	if e := json.Unmarshal([]byte(stdout), &arr); e != nil {
		t.Fatalf("json: output is not a single JSON array: %v\nstdout: %s", e, stdout)
	}
	if len(arr) != 2 {
		t.Errorf("json: expected 2 array elements, got %d", len(arr))
	}
}

// TestQueryJSONArrayEmpty (IR-10): an empty result in --json mode emits "[]",
// never "null".
func TestQueryJSONArrayEmpty(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	writeTestNode(t, vault, "a.md", node.Mint(), "note", "body")
	buildIndex(t, vault)

	stdout, _, err := runQuery(t, vault, "--json", "SELECT * FROM nodes WHERE type='ghost'")
	if err != nil {
		t.Fatalf("json empty: query failed: %v", err)
	}
	if strings.TrimSpace(stdout) != "[]" {
		t.Errorf("json empty: expected \"[]\", got %q", stdout)
	}
}

// TestOpenReadOnlyIndexRejectsWrites exercises the production openReadOnlyIndex
// helper directly (TS-11 reconstructs the URI by hand; this guards the real
// function so a regression in it is caught).
func TestOpenReadOnlyIndexRejectsWrites(t *testing.T) {
	vault, cache := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	writeTestNode(t, vault, "a.md", node.Mint(), "note", "body")
	buildIndex(t, vault)

	cfg, err := config.LoadWithOverrides(vault, cache)
	if err != nil {
		t.Fatalf("LoadWithOverrides: %v", err)
	}
	dbPath, err := index.DBPath(cfg)
	if err != nil {
		t.Fatalf("DBPath: %v", err)
	}

	db, err := openReadOnlyIndex(dbPath)
	if err != nil {
		t.Fatalf("openReadOnlyIndex: %v", err)
	}
	defer db.Close()

	if _, execErr := db.Exec("INSERT INTO _nodes(node_key,loc_file,hash,mtime) VALUES('ro','x','x',0)"); execErr == nil {
		t.Fatal("expected readonly error from INSERT, got nil")
	} else if !strings.Contains(strings.ToLower(execErr.Error()), "readonly") {
		t.Errorf("expected 'readonly' in error, got: %v", execErr)
	}
}

// TestResetQueryFlagsCoversAllLocalFlags guards against resetQueryFlags drifting
// out of sync with init(): after a run that sets every local flag, the deferred
// reset must return them all to defaults with Changed cleared.
func TestResetQueryFlagsCoversAllLocalFlags(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	writeTestNode(t, vault, "a.md", node.Mint(), "task", "body", "aliases: x")
	buildIndex(t, vault)

	// Run with every local flag set.
	var out, errb bytes.Buffer
	RootCmd.SetOut(&out)
	RootCmd.SetErr(&errb)
	RootCmd.SetArgs([]string{
		"query", "--vault", vault,
		"--raw", "--fields", "type", "--limit", "1", "--lang", "sql",
		"SELECT type FROM nodes",
	})
	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v\nstderr: %s", err, errb.String())
	}

	// After the deferred resetQueryFlags, all local flag vars are defaults...
	if queryRawFlag || queryFieldsFlag != "" || queryLimitFlag != 0 || queryViewFlag != "" || queryLangFlag != "sql" {
		t.Errorf("flag vars not reset: raw=%v fields=%q limit=%d view=%q lang=%q",
			queryRawFlag, queryFieldsFlag, queryLimitFlag, queryViewFlag, queryLangFlag)
	}
	// ...and pflag Changed is cleared for each local flag.
	for _, name := range []string{"view", "raw", "fields", "limit", "lang"} {
		if fl := queryCmd.Flags().Lookup(name); fl != nil && fl.Changed {
			t.Errorf("flag %q Changed still true after reset", name)
		}
	}
}
