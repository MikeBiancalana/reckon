package index

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/node"
)

// testVault creates an isolated vault dir + a sibling cache dir and returns a
// Config pointing at them. The cache is deliberately NOT inside the vault.
func testVault(t *testing.T) (*config.Config, string) {
	t.Helper()
	root := t.TempDir()
	vault := filepath.Join(root, "vault")
	cache := filepath.Join(root, "cache")
	if err := os.MkdirAll(vault, 0o755); err != nil {
		t.Fatalf("mkdir vault: %v", err)
	}
	cfg, err := config.LoadWithOverrides(vault, cache)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	return cfg, vault
}

// writeFile writes content to relpath within the vault.
func writeFile(t *testing.T, vault, relpath, content string) {
	t.Helper()
	full := filepath.Join(vault, relpath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", relpath, err)
	}
}

// noteFile builds a minimal note file body with the given frontmatter lines.
func noteFile(id, body string, fmLines ...string) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("id: " + id + "\n")
	b.WriteString("type: note\n")
	for _, l := range fmLines {
		b.WriteString(l + "\n")
	}
	b.WriteString("---\n")
	b.WriteString(body + "\n")
	return b.String()
}

func count(t *testing.T, ix *Index, query string, args ...any) int {
	t.Helper()
	var n int
	if err := ix.DB().QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return n
}

func TestRebuildPopulatesViews(t *testing.T) {
	cfg, vault := testVault(t)
	idA, idB := node.Mint(), node.Mint()
	writeFile(t, vault, "a/note-a.md", noteFile(idA,
		fmt.Sprintf("Body of A links to [[%s]] and [[ghost]].", idB),
		"aliases: alpha",
		"status: open",
		fmt.Sprintf("depends: \"[[%s]]\"", idB)))
	writeFile(t, vault, "note-b.md", noteFile(idB, "Body of B.", "aliases: [beta, bravo]"))

	ix, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer ix.Close()
	if err := ix.Rebuild(); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	if got := count(t, ix, "SELECT count(*) FROM nodes"); got != 2 {
		t.Errorf("nodes = %d, want 2", got)
	}
	if got := count(t, ix, "SELECT count(*) FROM aliases"); got != 3 {
		t.Errorf("aliases = %d, want 3 (alpha,beta,bravo)", got)
	}
	// Scalar frontmatter -> node_props; the ref-valued `depends` becomes an edge, not a prop.
	if got := count(t, ix, "SELECT count(*) FROM node_props WHERE id=? AND key='status' AND value='open'", idA); got != 1 {
		t.Errorf("scalar prop status=open missing for A")
	}
	if got := count(t, ix, "SELECT count(*) FROM node_props WHERE id=? AND key='depends'", idA); got != 0 {
		t.Errorf("ref prop depends leaked into node_props (should be an edge)")
	}
	if got := count(t, ix, "SELECT count(*) FROM fts WHERE id=?", idB); got != 1 {
		t.Errorf("fts rows for B = %d, want 1", got)
	}

	// depends frontmatter ref -> typed edge rel=depends, resolved to B.
	if got := count(t, ix,
		"SELECT count(*) FROM edges WHERE src=? AND rel='depends' AND dst=? AND dst_key=?",
		idA, idB, idB); got != 1 {
		t.Errorf("typed depends edge A->B not found/resolved")
	}
	// body [[idB]] -> references edge resolved to B.
	if got := count(t, ix,
		"SELECT count(*) FROM edges WHERE src=? AND rel='references' AND dst=? AND dst_key=?",
		idA, idB, idB); got != 1 {
		t.Errorf("references edge A->B not found/resolved")
	}
	// body [[ghost]] -> dangling references edge, dst_key NULL.
	if got := count(t, ix,
		"SELECT count(*) FROM edges WHERE src=? AND dst='ghost' AND dst_key IS NULL", idA); got != 1 {
		t.Errorf("dangling ghost edge not found")
	}
}

// dumpAll returns a deterministic, sorted textual snapshot of the graph content.
func dumpAll(t *testing.T, ix *Index) string {
	t.Helper()
	var lines []string
	add := func(q string) {
		rows, err := ix.DB().Query(q)
		if err != nil {
			t.Fatalf("dump query %q: %v", q, err)
		}
		defer rows.Close()
		cols, _ := rows.Columns()
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				t.Fatalf("scan: %v", err)
			}
			lines = append(lines, fmt.Sprintf("%v", vals))
		}
	}
	add("SELECT id,ulid,type,time,author,body,loc FROM nodes")
	add("SELECT src,rel,dst,dst_key,from_frag,to_frag FROM edges")
	add("SELECT id,key,value FROM node_props")
	add("SELECT alias,id FROM aliases")
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func TestRebuildDeterministic(t *testing.T) {
	cfg, vault := testVault(t)
	idA, idB, idC := node.Mint(), node.Mint(), node.Mint()
	writeFile(t, vault, "a.md", noteFile(idA, fmt.Sprintf("see [[%s]]", idB), "aliases: a1"))
	writeFile(t, vault, "sub/b.md", noteFile(idB, "b body", "k: v"))
	writeFile(t, vault, "sub/deep/c.md", noteFile(idC, fmt.Sprintf("c -> [[%s]]", idA)))

	ix, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer ix.Close()

	if err := ix.Rebuild(); err != nil {
		t.Fatalf("rebuild1: %v", err)
	}
	d1 := dumpAll(t, ix)
	if err := ix.Rebuild(); err != nil {
		t.Fatalf("rebuild2: %v", err)
	}
	d2 := dumpAll(t, ix)
	if d1 != d2 {
		t.Errorf("rebuild not deterministic:\n--- first ---\n%s\n--- second ---\n%s", d1, d2)
	}
}

func TestReconcileAddEditDelete(t *testing.T) {
	cfg, vault := testVault(t)
	idA := node.Mint()
	writeFile(t, vault, "a.md", noteFile(idA, "original body"))

	ix, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer ix.Close()
	if err := ix.Rebuild(); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	// ADD
	idB := node.Mint()
	writeFile(t, vault, "b.md", noteFile(idB, "new file"))
	if _, err := ix.Reconcile(); err != nil {
		t.Fatalf("reconcile add: %v", err)
	}
	if got := count(t, ix, "SELECT count(*) FROM nodes"); got != 2 {
		t.Fatalf("after add nodes = %d, want 2", got)
	}

	// EDIT
	writeFile(t, vault, "a.md", noteFile(idA, "edited body"))
	if _, err := ix.Reconcile(); err != nil {
		t.Fatalf("reconcile edit: %v", err)
	}
	var body string
	if err := ix.DB().QueryRow("SELECT body FROM nodes WHERE id=?", idA).Scan(&body); err != nil {
		t.Fatalf("scan body: %v", err)
	}
	if !strings.Contains(body, "edited body") {
		t.Errorf("body not updated after edit: %q", body)
	}

	// DELETE
	if err := os.Remove(filepath.Join(vault, "b.md")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := ix.Reconcile(); err != nil {
		t.Fatalf("reconcile delete: %v", err)
	}
	if got := count(t, ix, "SELECT count(*) FROM nodes WHERE id=?", idB); got != 0 {
		t.Errorf("deleted node still present")
	}
}

func TestReconcileRenameKeyedByULID(t *testing.T) {
	cfg, vault := testVault(t)
	idA, idB := node.Mint(), node.Mint()
	// A links to B; B will be renamed. Backlink must survive, loc must update.
	writeFile(t, vault, "a.md", noteFile(idA, fmt.Sprintf("ref [[%s]]", idB)))
	writeFile(t, vault, "b.md", noteFile(idB, "b body"))

	ix, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer ix.Close()
	if err := ix.Rebuild(); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	// rename b.md -> archive/b-renamed.md (same ULID)
	if err := os.MkdirAll(filepath.Join(vault, "archive"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	old := filepath.Join(vault, "b.md")
	newp := filepath.Join(vault, "archive", "b-renamed.md")
	if err := os.Rename(old, newp); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if _, err := ix.Reconcile(); err != nil {
		t.Fatalf("reconcile rename: %v", err)
	}

	// node B still present, keyed by ULID, loc updated
	var loc string
	if err := ix.DB().QueryRow("SELECT loc FROM nodes WHERE id=?", idB).Scan(&loc); err != nil {
		t.Fatalf("B missing after rename: %v", err)
	}
	if !strings.Contains(loc, "b-renamed.md") {
		t.Errorf("loc not updated after rename: %q", loc)
	}
	// backlink A->B still resolves
	if got := count(t, ix, "SELECT count(*) FROM edges WHERE src=? AND dst_key=?", idA, idB); got != 1 {
		t.Errorf("backlink A->B not preserved after rename")
	}
	if got := count(t, ix, "SELECT count(*) FROM nodes"); got != 2 {
		t.Errorf("node count = %d after rename, want 2", got)
	}
}

func TestReconcileMtimeFastPath(t *testing.T) {
	cfg, vault := testVault(t)
	writeFile(t, vault, "a.md", noteFile(node.Mint(), "body"))

	ix, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer ix.Close()
	if err := ix.Rebuild(); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	st, err := ix.Reconcile()
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if st.Reparsed != 0 {
		t.Errorf("Reparsed = %d on unchanged vault, want 0 (mtime fast-path)", st.Reparsed)
	}
}

func TestSchemaVersionAutoRebuild(t *testing.T) {
	cfg, vault := testVault(t)
	writeFile(t, vault, "a.md", noteFile(node.Mint(), "body"))

	ix, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := ix.Rebuild(); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	// Force a stale schema_version and a marker row.
	if _, err := ix.DB().Exec("UPDATE _index_meta SET value='0' WHERE key='schema_version'"); err != nil {
		t.Fatalf("set stale version: %v", err)
	}
	if _, err := ix.DB().Exec("INSERT INTO _nodes(node_key,loc_file,hash,mtime) VALUES('stale-marker','x','x',0)"); err != nil {
		t.Fatalf("insert marker: %v", err)
	}
	ix.Close()

	// Reopen: stale schema_version must trigger a full rebuild (marker gone, version current).
	ix2, err := Open(cfg)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer ix2.Close()
	if got := count(t, ix2, "SELECT count(*) FROM _nodes WHERE node_key='stale-marker'"); got != 0 {
		t.Errorf("auto-rebuild did not occur (stale marker survived)")
	}
	v, err := ix2.Meta("schema_version")
	if err != nil {
		t.Fatalf("meta: %v", err)
	}
	if v != fmt.Sprintf("%d", SchemaVersion) {
		t.Errorf("schema_version = %q after auto-rebuild, want %d", v, SchemaVersion)
	}
}

func TestIgnoreGlobsAndConflictMarkers(t *testing.T) {
	cfg, vault := testVault(t)
	good := node.Mint()
	writeFile(t, vault, "good.md", noteFile(good, "fine"))
	// ignored locations
	writeFile(t, vault, ".obsidian/config.md", noteFile(node.Mint(), "obsidian"))
	writeFile(t, vault, ".git/x.md", noteFile(node.Mint(), "git"))
	writeFile(t, vault, "note.sync-conflict-1.md", noteFile(node.Mint(), "conflict copy"))
	// conflict-marker file: parser refuses, reconcile must not crash
	writeFile(t, vault, "bad.md", "---\nid: "+node.Mint()+"\n---\n<<<<<<< HEAD\nx\n=======\ny\n>>>>>>> other\n")

	ix, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer ix.Close()
	if err := ix.Rebuild(); err != nil {
		t.Fatalf("rebuild must tolerate malformed files: %v", err)
	}
	if got := count(t, ix, "SELECT count(*) FROM nodes"); got != 1 {
		t.Errorf("nodes = %d, want 1 (only good.md indexed)", got)
	}
	if got := count(t, ix, "SELECT count(*) FROM nodes WHERE id=?", good); got != 1 {
		t.Errorf("good.md not indexed")
	}
}

func TestCacheInsideVaultRejected(t *testing.T) {
	root := t.TempDir()
	vault := filepath.Join(root, "vault")
	if err := os.MkdirAll(vault, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// cache inside vault -> config guard rejects
	if _, err := config.LoadWithOverrides(vault, filepath.Join(vault, "cache")); err == nil {
		t.Fatalf("expected config to reject cache inside vault")
	}
}

func TestDBPathUnderCache(t *testing.T) {
	cfg, _ := testVault(t)
	p, err := DBPath(cfg)
	if err != nil {
		t.Fatalf("DBPath: %v", err)
	}
	if !strings.HasPrefix(p, cfg.CacheDir) {
		t.Errorf("DBPath %q not under cache %q", p, cfg.CacheDir)
	}
	if !strings.HasSuffix(p, "index.db") {
		t.Errorf("DBPath %q does not end in index.db", p)
	}
}
