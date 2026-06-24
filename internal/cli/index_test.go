package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestIndexCommandRebuilds (reckon-pb82): `rk index` against a corpus dir must
// build the index and report a structured summary in --json mode.
func TestIndexCommandRebuilds(t *testing.T) {
	root := t.TempDir()
	vault := filepath.Join(root, "vault")
	cache := filepath.Join(root, "cache")
	if err := os.MkdirAll(vault, 0o755); err != nil {
		t.Fatalf("mkdir vault: %v", err)
	}
	note := "---\nid: 01HZZZZZZZZZZZZZZZZZZZZZZA\ntype: note\naliases: alpha\n---\nhello body\n"
	if err := os.WriteFile(filepath.Join(vault, "a.md"), []byte(note), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	t.Setenv("RECKON_CACHE", cache)

	var buf bytes.Buffer
	RootCmd.SetOut(&buf)
	RootCmd.SetErr(&buf)
	RootCmd.SetArgs([]string{"index", "--json", "--vault", vault})
	t.Cleanup(func() {
		RootCmd.SetArgs(nil)
		RootCmd.SetOut(nil)
		RootCmd.SetErr(nil)
		vaultFlag = ""
		jsonFlag = false
	})

	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("rk index: %v", err)
	}

	var res indexResult
	if err := json.Unmarshal(buf.Bytes(), &res); err != nil {
		t.Fatalf("decode json output %q: %v", buf.String(), err)
	}
	if res.Nodes != 1 {
		t.Errorf("nodes = %d, want 1", res.Nodes)
	}
	if res.Aliases != 1 {
		t.Errorf("aliases = %d, want 1", res.Aliases)
	}
	if res.VaultID == "" {
		t.Errorf("vault_id empty in output")
	}

	// The index db must live under the cache dir, never inside the vault.
	if _, err := os.Stat(filepath.Join(cache, res.VaultID, "index.db")); err != nil {
		t.Errorf("index.db not found under cache: %v", err)
	}
}
