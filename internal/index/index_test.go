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

// filterWarnings returns the warnings of the given kind ("duplicate_ulid" or
// "alias_collision"), for assertions that don't want to care about ordering
// between kinds.
func filterWarnings(ws []Warning, kind string) []Warning {
	var out []Warning
	for _, w := range ws {
		if w.Kind == kind {
			out = append(out, w)
		}
	}
	return out
}

// multiNodeParser is a stub Parser that always returns a fixed set of nodes for
// any file's raw bytes, regardless of content. It exists so a test can put two
// nodes sharing a ULID inside a single file without needing a real multi-entry
// (group-file) parser implementation.
type multiNodeParser struct{ nodes []*node.Node }

func (p multiNodeParser) Parse(raw []byte, loc node.Loc) ([]*node.Node, error) {
	out := make([]*node.Node, len(p.nodes))
	for i, n := range p.nodes {
		clone := *n
		clone.Loc = loc
		out[i] = &clone
	}
	return out, nil
}

func (p multiNodeParser) Serialize(n *node.Node) ([]byte, error) {
	return n.Serialize(), nil
}

var _ node.Parser = multiNodeParser{}

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
	if _, err := ix.Rebuild(); err != nil {
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
	add("SELECT id,ulid,type,time,author,body,loc,title FROM nodes")
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

	if _, err := ix.Rebuild(); err != nil {
		t.Fatalf("rebuild1: %v", err)
	}
	d1 := dumpAll(t, ix)
	if _, err := ix.Rebuild(); err != nil {
		t.Fatalf("rebuild2: %v", err)
	}
	d2 := dumpAll(t, ix)
	if d1 != d2 {
		t.Errorf("rebuild not deterministic:\n--- first ---\n%s\n--- second ---\n%s", d1, d2)
	}
}

// TestReconcilePopulatesTitle covers AC2 (reckon-fnqs.3): the nodes.title
// column must be populated at reconcile/rebuild time (insertNode) from each
// node's first non-empty body line. A passing TestDeriveTitle (title_test.go)
// only proves the derivation function is correct in isolation; this is the
// integration assertion that it is actually wired into the index write path.
func TestReconcilePopulatesTitle(t *testing.T) {
	cfg, vault := testVault(t)
	id := node.Mint()
	body := "Ship it.\n\nline2\nline3\nline4\n"
	src := "---\nid: " + id + "\ntype: todo\nstate: open\n---\n" + body
	writeFile(t, vault, "todos/"+id+".md", src)

	ix, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer ix.Close()
	if _, err := ix.Rebuild(); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	var title string
	if err := ix.DB().QueryRow("SELECT title FROM nodes WHERE id=?", id).Scan(&title); err != nil {
		t.Fatalf("scan title: %v", err)
	}
	if title != "Ship it." {
		t.Errorf("title = %q, want %q", title, "Ship it.")
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
	if _, err := ix.Rebuild(); err != nil {
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
	if _, err := ix.Rebuild(); err != nil {
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
	if _, err := ix.Rebuild(); err != nil {
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
	if _, err := ix.Rebuild(); err != nil {
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
	if _, err := ix.Rebuild(); err != nil {
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

// --- M3/M4: duplicate-ULID + alias-collision warnings (reckon-5b44) ---------

// TestReconcileDuplicateULIDWarns covers AC scenario 1: two files sharing a
// non-empty ULID produce exactly one duplicate-ULID warning naming both files.
func TestReconcileDuplicateULIDWarns(t *testing.T) {
	cfg, vault := testVault(t)
	dupID := node.Mint()
	writeFile(t, vault, "a.md", noteFile(dupID, "body a"))
	writeFile(t, vault, "b.md", noteFile(dupID, "body b"))

	ix, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer ix.Close()

	st, err := ix.Rebuild()
	if err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	dupWarnings := filterWarnings(st.Warnings, "duplicate_ulid")
	if len(dupWarnings) != 1 {
		t.Fatalf("duplicate_ulid warnings = %d, want 1 (got %+v)", len(dupWarnings), st.Warnings)
	}
	w := dupWarnings[0]
	if w.ULID != dupID {
		t.Errorf("warning ULID = %q, want %q", w.ULID, dupID)
	}
	if got := strings.Join(w.Files, ","); got != "a.md,b.md" {
		t.Errorf("warning Files = %q, want %q", got, "a.md,b.md")
	}
}

// TestReconcileDuplicateULIDThreeFiles covers AC scenario 2: 3+ files sharing
// one ULID produce a single warning naming all of them, not pairwise warnings.
func TestReconcileDuplicateULIDThreeFiles(t *testing.T) {
	cfg, vault := testVault(t)
	dupID := node.Mint()
	writeFile(t, vault, "a.md", noteFile(dupID, "body a"))
	writeFile(t, vault, "b.md", noteFile(dupID, "body b"))
	writeFile(t, vault, "c.md", noteFile(dupID, "body c"))

	ix, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer ix.Close()

	st, err := ix.Rebuild()
	if err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	dupWarnings := filterWarnings(st.Warnings, "duplicate_ulid")
	if len(dupWarnings) != 1 {
		t.Fatalf("duplicate_ulid warnings = %d, want 1 (got %+v)", len(dupWarnings), st.Warnings)
	}
	if got := strings.Join(dupWarnings[0].Files, ","); got != "a.md,b.md,c.md" {
		t.Errorf("warning Files = %q, want %q", got, "a.md,b.md,c.md")
	}
}

// TestReconcileNoDuplicateWarningForEmptyULID covers AC scenario 3: files with
// no id: (empty-string ULID, surrogate file:-keyed) must never be reported as a
// duplicate, no matter how many of them exist.
func TestReconcileNoDuplicateWarningForEmptyULID(t *testing.T) {
	cfg, vault := testVault(t)
	writeFile(t, vault, "a.md", "---\ntype: note\n---\nbody a\n")
	writeFile(t, vault, "b.md", "---\ntype: note\n---\nbody b\n")

	ix, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer ix.Close()

	st, err := ix.Rebuild()
	if err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	if got := len(filterWarnings(st.Warnings, "duplicate_ulid")); got != 0 {
		t.Errorf("duplicate_ulid warnings = %d, want 0 for empty-ULID surrogate-keyed files (got %+v)", got, st.Warnings)
	}
}

// TestReconcileDuplicateULIDDetectedOnMtimeFastPath covers AC scenario 4: a
// duplicate-ULID pair already indexed must still be reported when a later
// Reconcile() takes the mtime fast-path for both files (no reparse).
func TestReconcileDuplicateULIDDetectedOnMtimeFastPath(t *testing.T) {
	cfg, vault := testVault(t)
	dupID := node.Mint()
	writeFile(t, vault, "a.md", noteFile(dupID, "body a"))
	writeFile(t, vault, "b.md", noteFile(dupID, "body b"))

	ix, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer ix.Close()
	if _, err := ix.Rebuild(); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	// Neither file touched: Reconcile should take the mtime fast-path for both,
	// yet the duplicate-ULID warning must still be reported every pass.
	st, err := ix.Reconcile()
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if st.Reparsed != 0 {
		t.Fatalf("Reparsed = %d, want 0 (mtime fast-path)", st.Reparsed)
	}
	if got := len(filterWarnings(st.Warnings, "duplicate_ulid")); got != 1 {
		t.Errorf("duplicate_ulid warnings after fast-path reconcile = %d, want 1 (got %+v)", got, st.Warnings)
	}
}

// TestReconcileDuplicateULIDResolvesOnDelete covers AC scenario 5: once one of
// the colliding files is deleted, the next Reconcile() reports zero
// duplicate-ULID warnings and the survivor is indexed normally.
func TestReconcileDuplicateULIDResolvesOnDelete(t *testing.T) {
	cfg, vault := testVault(t)
	dupID := node.Mint()
	writeFile(t, vault, "a.md", noteFile(dupID, "body a"))
	writeFile(t, vault, "b.md", noteFile(dupID, "body b"))

	ix, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer ix.Close()
	st, err := ix.Rebuild()
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if got := len(filterWarnings(st.Warnings, "duplicate_ulid")); got != 1 {
		t.Fatalf("precondition: duplicate_ulid warnings = %d, want 1", got)
	}

	if err := os.Remove(filepath.Join(vault, "b.md")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	st, err = ix.Reconcile()
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if got := len(filterWarnings(st.Warnings, "duplicate_ulid")); got != 0 {
		t.Errorf("duplicate_ulid warnings after delete = %d, want 0 (got %+v)", got, st.Warnings)
	}
	if got := count(t, ix, "SELECT count(*) FROM nodes WHERE id=?", dupID); got != 1 {
		t.Errorf("survivor node count for id=%s = %d, want 1", dupID, got)
	}
}

// TestReconcileDuplicateULIDWithinFile covers AC scenario 6: a single file
// whose parser yields two nodes sharing a non-empty ULID (e.g. two hand-authored
// frontmatter blocks) must warn, not crash or silently drop a node.
func TestReconcileDuplicateULIDWithinFile(t *testing.T) {
	cfg, vault := testVault(t)
	dupID := node.Mint()
	writeFile(t, vault, "both.md", "---\nid: "+dupID+"\ntype: note\n---\ntwo nodes, one file\n")

	parser := multiNodeParser{nodes: []*node.Node{
		{ULID: dupID, Type: "note", Body: "first node body\n"},
		{ULID: dupID, Type: "note", Body: "second node body\n"},
	}}

	ix, err := OpenWithParser(cfg, parser)
	if err != nil {
		t.Fatalf("OpenWithParser: %v", err)
	}
	defer ix.Close()

	st, err := ix.Rebuild()
	if err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	dupWarnings := filterWarnings(st.Warnings, "duplicate_ulid")
	if len(dupWarnings) != 1 {
		t.Fatalf("duplicate_ulid warnings = %d, want 1 (got %+v)", len(dupWarnings), st.Warnings)
	}
	if got := strings.Join(dupWarnings[0].Files, ","); got != "both.md" {
		t.Errorf("warning Files = %q, want %q", got, "both.md")
	}
}

// TestReconcileAliasCollisionWarns covers AC scenario 7: two different nodes
// declaring the same alias produce one alias-collision warning naming both
// node_keys/files, and resolveEdges' existing lowest-node_key tiebreak still
// resolves a reference to that alias deterministically.
func TestReconcileAliasCollisionWarns(t *testing.T) {
	cfg, vault := testVault(t)
	idA, idB := node.Mint(), node.Mint()
	writeFile(t, vault, "a.md", noteFile(idA, "body a", "aliases: shared"))
	writeFile(t, vault, "b.md", noteFile(idB, "body b", "aliases: shared"))

	ix, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer ix.Close()

	st, err := ix.Rebuild()
	if err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	aliasWarnings := filterWarnings(st.Warnings, "alias_collision")
	if len(aliasWarnings) != 1 {
		t.Fatalf("alias_collision warnings = %d, want 1 (got %+v)", len(aliasWarnings), st.Warnings)
	}
	w := aliasWarnings[0]
	if w.Alias != "shared" {
		t.Errorf("warning Alias = %q, want %q", w.Alias, "shared")
	}
	wantKeys := []string{idA, idB}
	sort.Strings(wantKeys)
	if got := strings.Join(w.NodeKeys, ","); got != strings.Join(wantKeys, ",") {
		t.Errorf("warning NodeKeys = %v, want %v", w.NodeKeys, wantKeys)
	}
	if got := strings.Join(w.Files, ","); got != "a.md,b.md" {
		t.Errorf("warning Files = %q, want %q", got, "a.md,b.md")
	}

	// resolveEdges' existing lowest-node_key tiebreak must still resolve a
	// reference to the colliding alias deterministically (unchanged behavior).
	writeFile(t, vault, "c.md", noteFile(node.Mint(), "see [[shared]]"))
	if _, err := ix.Reconcile(); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	want := idA
	if idB < idA {
		want = idB
	}
	if got := count(t, ix, "SELECT count(*) FROM edges WHERE dst='shared' AND dst_key=?", want); got != 1 {
		t.Errorf("edge to shared alias did not resolve to lowest node_key %s", want)
	}
}

// TestReconcileAliasCollisionThreeNodes covers AC scenario 8: an alias shared
// by 3+ nodes produces a single warning listing all of them, not pairwise.
func TestReconcileAliasCollisionThreeNodes(t *testing.T) {
	cfg, vault := testVault(t)
	idA, idB, idC := node.Mint(), node.Mint(), node.Mint()
	writeFile(t, vault, "a.md", noteFile(idA, "body a", "aliases: shared"))
	writeFile(t, vault, "b.md", noteFile(idB, "body b", "aliases: shared"))
	writeFile(t, vault, "c.md", noteFile(idC, "body c", "aliases: shared"))

	ix, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer ix.Close()

	st, err := ix.Rebuild()
	if err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	aliasWarnings := filterWarnings(st.Warnings, "alias_collision")
	if len(aliasWarnings) != 1 {
		t.Fatalf("alias_collision warnings = %d, want 1 (got %+v)", len(aliasWarnings), st.Warnings)
	}
	w := aliasWarnings[0]
	if len(w.NodeKeys) != 3 {
		t.Errorf("warning NodeKeys = %v, want 3 entries", w.NodeKeys)
	}
	if got := strings.Join(w.Files, ","); got != "a.md,b.md,c.md" {
		t.Errorf("warning Files = %q, want %q", got, "a.md,b.md,c.md")
	}
}

// TestReconcileAliasCollisionResolvesOnEdit covers AC scenario 9: once one of
// the colliding files is edited to remove the alias, the next Reconcile()
// reports zero alias-collision warnings for it.
func TestReconcileAliasCollisionResolvesOnEdit(t *testing.T) {
	cfg, vault := testVault(t)
	idA, idB := node.Mint(), node.Mint()
	writeFile(t, vault, "a.md", noteFile(idA, "body a", "aliases: shared"))
	writeFile(t, vault, "b.md", noteFile(idB, "body b", "aliases: shared"))

	ix, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer ix.Close()
	st, err := ix.Rebuild()
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if got := len(filterWarnings(st.Warnings, "alias_collision")); got != 1 {
		t.Fatalf("precondition: alias_collision warnings = %d, want 1", got)
	}

	writeFile(t, vault, "b.md", noteFile(idB, "body b edited"))
	st, err = ix.Reconcile()
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if got := len(filterWarnings(st.Warnings, "alias_collision")); got != 0 {
		t.Errorf("alias_collision warnings after edit = %d, want 0 (got %+v)", got, st.Warnings)
	}
}

// TestReconcileDuplicateULIDAndAliasCollisionIndependent covers AC scenario 10
// (cross-cutting): a node that is part of both a ULID collision and an alias
// collision must produce both warnings, each correctly scoped, with neither
// swallowing the other.
func TestReconcileDuplicateULIDAndAliasCollisionIndependent(t *testing.T) {
	cfg, vault := testVault(t)
	dupID, idC := node.Mint(), node.Mint()
	// a.md and b.md collide on ULID. Both declare the same alias so it survives
	// on node_key=dupID regardless of which file the walk visits last; that
	// alias separately collides with c.md's alias.
	writeFile(t, vault, "a.md", noteFile(dupID, "body a", "aliases: sharedAlias"))
	writeFile(t, vault, "b.md", noteFile(dupID, "body b", "aliases: sharedAlias"))
	writeFile(t, vault, "c.md", noteFile(idC, "body c", "aliases: sharedAlias"))

	ix, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer ix.Close()

	st, err := ix.Rebuild()
	if err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	dupWarnings := filterWarnings(st.Warnings, "duplicate_ulid")
	if len(dupWarnings) != 1 {
		t.Fatalf("duplicate_ulid warnings = %d, want 1 (got %+v)", len(dupWarnings), st.Warnings)
	}
	if got := strings.Join(dupWarnings[0].Files, ","); got != "a.md,b.md" {
		t.Errorf("duplicate_ulid Files = %q, want %q", got, "a.md,b.md")
	}

	aliasWarnings := filterWarnings(st.Warnings, "alias_collision")
	if len(aliasWarnings) != 1 {
		t.Fatalf("alias_collision warnings = %d, want 1 (got %+v)", len(aliasWarnings), st.Warnings)
	}
	w := aliasWarnings[0]
	if w.Alias != "sharedAlias" {
		t.Errorf("alias_collision Alias = %q, want %q", w.Alias, "sharedAlias")
	}
	// Exactly two node_keys: the collapsed a/b pair (dupID) and idC -- the ULID
	// collision must not leak a and b in as two separate alias-collision entries.
	if len(w.NodeKeys) != 2 {
		t.Errorf("alias_collision NodeKeys = %v, want 2 entries", w.NodeKeys)
	}
	// node_key=dupID's surviving _nodes row is loc_file=b.md (lexically last of
	// a.md/b.md in the walk), so the alias-collision file list is b.md + c.md.
	if got := strings.Join(w.Files, ","); got != "b.md,c.md" {
		t.Errorf("alias_collision Files = %q, want %q", got, "b.md,c.md")
	}
}

// TestReconcileDuplicateULIDSurvivesRename covers the recommended edge case
// (AC section 3): renaming one of the colliding files changes its path but not
// its ULID, so the collision must still be reported -- rename alone doesn't fix it.
func TestReconcileDuplicateULIDSurvivesRename(t *testing.T) {
	cfg, vault := testVault(t)
	dupID := node.Mint()
	writeFile(t, vault, "a.md", noteFile(dupID, "body a"))
	writeFile(t, vault, "b.md", noteFile(dupID, "body b"))

	ix, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer ix.Close()
	st, err := ix.Rebuild()
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if got := len(filterWarnings(st.Warnings, "duplicate_ulid")); got != 1 {
		t.Fatalf("precondition: duplicate_ulid warnings = %d, want 1", got)
	}

	if err := os.Rename(filepath.Join(vault, "b.md"), filepath.Join(vault, "b-renamed.md")); err != nil {
		t.Fatalf("rename: %v", err)
	}
	st, err = ix.Reconcile()
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	dupWarnings := filterWarnings(st.Warnings, "duplicate_ulid")
	if len(dupWarnings) != 1 {
		t.Errorf("duplicate_ulid warnings after rename = %d, want 1 (rename alone doesn't resolve a collision)", len(dupWarnings))
	} else if got := strings.Join(dupWarnings[0].Files, ","); got != "a.md,b-renamed.md" {
		t.Errorf("duplicate_ulid Files after rename = %q, want %q", got, "a.md,b-renamed.md")
	}
}
