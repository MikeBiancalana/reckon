package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

// TestIndexCommandDuplicateULIDWarningJSON (reckon-5b44, AC scenario 11): a
// vault with two files sharing a ULID must surface a non-empty duplicate_ulid
// warning in `rk index --json` output, with exit code 0 (warn, not fail).
func TestIndexCommandDuplicateULIDWarningJSON(t *testing.T) {
	root := t.TempDir()
	vault := filepath.Join(root, "vault")
	cache := filepath.Join(root, "cache")
	if err := os.MkdirAll(vault, 0o755); err != nil {
		t.Fatalf("mkdir vault: %v", err)
	}
	dupID := "01HZZZZZZZZZZZZZZZZZZZZZZB"
	noteA := "---\nid: " + dupID + "\ntype: note\n---\nbody a\n"
	noteB := "---\nid: " + dupID + "\ntype: note\n---\nbody b\n"
	if err := os.WriteFile(filepath.Join(vault, "a.md"), []byte(noteA), 0o644); err != nil {
		t.Fatalf("write a.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vault, "b.md"), []byte(noteB), 0o644); err != nil {
		t.Fatalf("write b.md: %v", err)
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

	// warn-only: exit code must stay 0 even though the vault has a collision.
	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("rk index: %v", err)
	}

	var res indexResult
	if err := json.Unmarshal(buf.Bytes(), &res); err != nil {
		t.Fatalf("decode json output %q: %v", buf.String(), err)
	}
	if len(res.Warnings) == 0 {
		t.Fatalf("Warnings empty, want a duplicate_ulid entry (output: %s)", buf.String())
	}
	var found bool
	for _, w := range res.Warnings {
		if w.Kind != "duplicate_ulid" || w.ULID != dupID {
			continue
		}
		found = true
		if got := strings.Join(w.Files, ","); got != "a.md,b.md" {
			t.Errorf("duplicate_ulid Files = %q, want %q", got, "a.md,b.md")
		}
	}
	if !found {
		t.Errorf("no duplicate_ulid warning for %s found in %+v", dupID, res.Warnings)
	}
}

// TestIndexCommandAliasCollisionPretty (reckon-5b44, AC scenario 12): a vault
// with an alias collision must communicate it in the default/pretty output,
// not only in --json.
func TestIndexCommandAliasCollisionPretty(t *testing.T) {
	root := t.TempDir()
	vault := filepath.Join(root, "vault")
	cache := filepath.Join(root, "cache")
	if err := os.MkdirAll(vault, 0o755); err != nil {
		t.Fatalf("mkdir vault: %v", err)
	}
	noteA := "---\nid: 01HZZZZZZZZZZZZZZZZZZZZZZC\ntype: note\naliases: shared\n---\nbody a\n"
	noteB := "---\nid: 01HZZZZZZZZZZZZZZZZZZZZZZD\ntype: note\naliases: shared\n---\nbody b\n"
	if err := os.WriteFile(filepath.Join(vault, "a.md"), []byte(noteA), 0o644); err != nil {
		t.Fatalf("write a.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vault, "b.md"), []byte(noteB), 0o644); err != nil {
		t.Fatalf("write b.md: %v", err)
	}
	t.Setenv("RECKON_CACHE", cache)

	var buf bytes.Buffer
	RootCmd.SetOut(&buf)
	RootCmd.SetErr(&buf)
	RootCmd.SetArgs([]string{"index", "--vault", vault})
	t.Cleanup(func() {
		RootCmd.SetArgs(nil)
		RootCmd.SetOut(nil)
		RootCmd.SetErr(nil)
		vaultFlag = ""
	})

	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("rk index: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "shared") {
		t.Errorf("pretty output does not mention the colliding alias %q: %q", "shared", out)
	}
}

// TestIndexCommandCleanVaultWarningsEmpty (reckon-5b44, AC scenario 13): a
// clean single-note vault must report a present-but-empty warnings field,
// leaving the pre-existing Nodes/Aliases/VaultID assertions unaffected.
func TestIndexCommandCleanVaultWarningsEmpty(t *testing.T) {
	root := t.TempDir()
	vault := filepath.Join(root, "vault")
	cache := filepath.Join(root, "cache")
	if err := os.MkdirAll(vault, 0o755); err != nil {
		t.Fatalf("mkdir vault: %v", err)
	}
	note := "---\nid: 01HZZZZZZZZZZZZZZZZZZZZZZE\ntype: note\naliases: alpha\n---\nhello body\n"
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

	if !strings.Contains(buf.String(), `"warnings"`) {
		t.Fatalf("json output missing warnings field: %s", buf.String())
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
	if len(res.Warnings) != 0 {
		t.Errorf("Warnings = %+v, want empty for a clean vault", res.Warnings)
	}
}
