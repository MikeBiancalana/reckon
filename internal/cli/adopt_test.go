// TDD red tests for reckon-9bfx: `rk adopt [path...]` (internal/cli/adopt.go,
// not yet created — adoptCmd is not registered on RootCmd yet). These tests
// drive the real RootCmd.Execute() dispatch (cobra resolves the "adopt"
// subcommand by name, so this file compiles even before adopt.go exists), but
// every scenario below decodes the structured JSON summary into `adoptResult`
// — a package-level type adopt.go must define. Until adopt.go exists,
// `adoptResult` is undefined and this file does not compile; that IS the
// expected red state for this gate.
//
// Harness: reuses setupQueryVault/resetCLIFlags from query_test.go (same
// package, same temp-vault/temp-cache/RootCmd-singleton harness rk query's
// tests already established) rather than re-deriving index_test.go's
// original, more repetitive per-test setup.
//
// Contract this file pins down for the implementation (plan.md "Files to
// create" — `adoptResult{Adopted []{Path,ULID}, Skipped []{Path,Reason},
// Errored []{Path,Error}}`):
//
//	type adoptResult struct {
//	    Adopted []struct {
//	        Path string `json:"path"`
//	        ULID string `json:"ulid"`
//	    } `json:"adopted"`
//	    Skipped []struct {
//	        Path   string `json:"path"`
//	        Reason string `json:"reason"`
//	    } `json:"skipped"`
//	    Errored []struct {
//	        Path  string `json:"path"`
//	        Error string `json:"error"`
//	    } `json:"errored"`
//	}
//
// Paths are reported vault-relative (mirroring index.Warning.Files' convention
// in internal/index/reconcile.go, e.g. "a.md", "notes/a.md"), not as the
// absolute path a caller may have passed on the command line — this keeps
// output stable across machines/temp dirs.
package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MikeBiancalana/reckon/internal/node"
)

// ─────────────────────────────────────────────────────────────────────────────
// Harness helpers (adopt-specific; vault/cache/flag-reset plumbing is shared
// with query_test.go via setupQueryVault/resetCLIFlags).
// ─────────────────────────────────────────────────────────────────────────────

// runAdopt executes `rk adopt --vault <vault> [args...]` through RootCmd and
// returns stdout plus the command's error. Mirrors runQuery/buildIndex in
// query_test.go. The caller must call resetCLIFlags() before another Execute
// within the same test (t.Cleanup(resetCLIFlags) covers end-of-test).
func runAdopt(t *testing.T, vault string, args ...string) (stdout string, err error) {
	t.Helper()
	var buf bytes.Buffer
	RootCmd.SetOut(&buf)
	RootCmd.SetErr(&buf)
	RootCmd.SetArgs(append([]string{"adopt", "--vault", vault}, args...))
	err = RootCmd.Execute()
	return buf.String(), err
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent of %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

// isValidULID mirrors internal/node/ulid_test.go's isCrockford (unexported,
// package node) — same 26-char Crockford base32 alphabet — reused here (this
// package cannot see node's unexported helper) to assert `rk adopt`'s minted
// id is indistinguishable in format from node.Mint's output.
const crockfordAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

func isValidULID(s string) bool {
	if len(s) != 26 {
		return false
	}
	for _, r := range s {
		if !strings.ContainsRune(crockfordAlphabet, r) {
			return false
		}
	}
	return true
}

// ─────────────────────────────────────────────────────────────────────────────
// Scenarios (acceptance-criteria.md §4 / plan.md "Test scenarios")
// ─────────────────────────────────────────────────────────────────────────────

// Given a file with an existing frontmatter block and no `id:` key, `rk
// adopt` inserts a new `id:` line inside the fence and leaves every other
// byte unchanged.
func TestAdoptCmd_StampsMissingIDIntoExistingBlock(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	target := filepath.Join(vault, "note.md")
	mustWriteFile(t, target, "---\ntype: note\n---\nbody text\n")

	out, err := runAdopt(t, vault, "--json", target)
	if err != nil {
		t.Fatalf("rk adopt: %v (output: %s)", err, out)
	}

	got := mustReadFile(t, target)
	if !strings.HasPrefix(got, "---\nid: ") {
		t.Fatalf("id: not stamped as the first line inside the fence: %q", got)
	}
	if !strings.HasSuffix(got, "type: note\n---\nbody text\n") {
		t.Fatalf("original bytes disturbed: %q", got)
	}

	n, err := node.Parse([]byte(got))
	if err != nil {
		t.Fatalf("re-parse adopted file: %v", err)
	}
	if n.ULID == "" {
		t.Error("ULID empty after adopt")
	}
	if n.Type != "note" {
		t.Errorf("Type = %q, want note", n.Type)
	}
	if n.Body != "body text\n" {
		t.Errorf("Body = %q, want %q", n.Body, "body text\n")
	}

	var res adoptResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("decode json output %q: %v", out, err)
	}
	if len(res.Adopted) != 1 || res.Adopted[0].Path != "note.md" {
		t.Fatalf("Adopted = %+v, want one entry for note.md", res.Adopted)
	}
	if res.Adopted[0].ULID != n.ULID {
		t.Errorf("reported ULID %q != file's ULID %q", res.Adopted[0].ULID, n.ULID)
	}
}

// Given a file with no frontmatter block at all, `rk adopt` prepends a whole
// `---\nid: <ULID>\n---\n` block ahead of the original content, which follows
// byte-for-byte unchanged.
func TestAdoptCmd_InsertsBlockIntoFileWithNone(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	target := filepath.Join(vault, "plain.md")
	const src = "just some text\n"
	mustWriteFile(t, target, src)

	out, err := runAdopt(t, vault, target)
	if err != nil {
		t.Fatalf("rk adopt: %v (output: %s)", err, out)
	}

	got := mustReadFile(t, target)
	if !strings.HasSuffix(got, src) {
		t.Fatalf("original body not preserved byte-for-byte: %q", got)
	}
	if !strings.HasPrefix(got, "---\nid: ") {
		t.Fatalf("new block not prepended: %q", got)
	}

	n, err := node.Parse([]byte(got))
	if err != nil {
		t.Fatalf("re-parse adopted file: %v", err)
	}
	if n.ULID == "" {
		t.Error("ULID empty after adopt")
	}
	if n.Body != src {
		t.Errorf("Body = %q, want %q", n.Body, src)
	}
}

// Given a file that already has an `id:`, `rk adopt` is a byte-identical
// no-op that exits 0.
func TestAdoptCmd_NoOpOnAlreadyIDdFile(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	target := filepath.Join(vault, "a.md")
	const src = "---\nid: 01J9Z3K7Q2W8XR4M6N0V5BYHED\ntype: note\n---\nbody\n"
	mustWriteFile(t, target, src)

	if _, err := runAdopt(t, vault, target); err != nil {
		t.Fatalf("rk adopt: %v", err)
	}

	if got := mustReadFile(t, target); got != src {
		t.Fatalf("already-id'd file mutated\n--- want ---\n%q\n--- got ---\n%q", src, got)
	}
}

// Running `rk adopt` twice in a row on the same id-less file mints exactly
// once: the second run sees the id already present and is a byte-identical
// no-op (same ULID, no re-mint).
func TestAdoptCmd_IdempotentAcrossTwoRuns(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	target := filepath.Join(vault, "b.md")
	mustWriteFile(t, target, "---\ntype: note\n---\nfirst run stamps this.\n")

	if _, err := runAdopt(t, vault, target); err != nil {
		t.Fatalf("first rk adopt: %v", err)
	}
	afterFirst := mustReadFile(t, target)
	n1, err := node.Parse([]byte(afterFirst))
	if err != nil || n1.ULID == "" {
		t.Fatalf("first adopt did not stamp an id: %v (content %q)", err, afterFirst)
	}
	resetCLIFlags()

	if _, err := runAdopt(t, vault, target); err != nil {
		t.Fatalf("second rk adopt: %v", err)
	}
	afterSecond := mustReadFile(t, target)
	if afterFirst != afterSecond {
		t.Fatalf("second adopt not a byte-identical no-op\n--- after 1st ---\n%q\n--- after 2nd ---\n%q", afterFirst, afterSecond)
	}
}

// A mixed batch (some id-less, some already id'd) processes each file
// independently: id-less files get distinct, freshly minted ids; the
// already-id'd file is untouched; no error from it blocks the others.
func TestAdoptCmd_MixedBatchDistinctULIDs(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	xPath := filepath.Join(vault, "x.md")
	yPath := filepath.Join(vault, "y.md")
	zPath := filepath.Join(vault, "z.md")
	ySrc := "---\nid: " + node.Mint() + "\ntype: note\n---\nalready adopted\n"
	mustWriteFile(t, xPath, "---\ntype: note\n---\nx body\n")
	mustWriteFile(t, yPath, ySrc)
	mustWriteFile(t, zPath, "---\ntype: note\n---\nz body\n")

	out, err := runAdopt(t, vault, "--json", xPath, yPath, zPath)
	if err != nil {
		t.Fatalf("rk adopt: %v (output: %s)", err, out)
	}

	if got := mustReadFile(t, yPath); got != ySrc {
		t.Fatalf("y.md (already id'd) mutated: %q", got)
	}
	xNode, err := node.Parse([]byte(mustReadFile(t, xPath)))
	if err != nil || xNode.ULID == "" {
		t.Fatalf("x.md not stamped: %v", err)
	}
	zNode, err := node.Parse([]byte(mustReadFile(t, zPath)))
	if err != nil || zNode.ULID == "" {
		t.Fatalf("z.md not stamped: %v", err)
	}
	if xNode.ULID == zNode.ULID {
		t.Errorf("x.md and z.md minted the same ULID: %q", xNode.ULID)
	}

	var res adoptResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("decode json output %q: %v", out, err)
	}
	if len(res.Adopted) != 2 {
		t.Fatalf("Adopted = %+v, want 2 entries", res.Adopted)
	}
	if len(res.Skipped) != 1 || res.Skipped[0].Path != "y.md" {
		t.Fatalf("Skipped = %+v, want one entry for y.md", res.Skipped)
	}
	if len(res.Errored) != 0 {
		t.Errorf("Errored = %+v, want none", res.Errored)
	}
}

// A directory argument walks recursively, honoring the same ignore rules the
// index uses: .obsidian/, .git/, and non-.md files are left untouched; an
// already-id'd file under the tree is left untouched; only the id-less .md
// file is stamped.
func TestAdoptCmd_DirectoryWalkSkipsIgnoredPaths(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	mustMkdirAll(t, filepath.Join(vault, "notes"))
	mustMkdirAll(t, filepath.Join(vault, ".obsidian"))
	mustMkdirAll(t, filepath.Join(vault, ".git"))

	aSrc := "---\ntype: note\n---\nnotes/a.md is id-less\n"
	bSrc := "---\nid: " + node.Mint() + "\ntype: note\n---\nnotes/b.md already id'd\n"
	obsidianSrc := "---\ntype: note\n---\nshould never be touched\n"
	gitSrc := "---\ntype: note\n---\nshould never be touched either\n"
	readmeSrc := "not markdown, plain text\n"

	mustWriteFile(t, filepath.Join(vault, "notes", "a.md"), aSrc)
	mustWriteFile(t, filepath.Join(vault, "notes", "b.md"), bSrc)
	mustWriteFile(t, filepath.Join(vault, ".obsidian", "config.md"), obsidianSrc)
	mustWriteFile(t, filepath.Join(vault, ".git", "x.md"), gitSrc)
	mustWriteFile(t, filepath.Join(vault, "notes", "readme.txt"), readmeSrc)

	if _, err := runAdopt(t, vault, vault); err != nil {
		t.Fatalf("rk adopt: %v", err)
	}

	aGot := mustReadFile(t, filepath.Join(vault, "notes", "a.md"))
	if aGot == aSrc {
		t.Error("notes/a.md was not stamped")
	}
	if n, err := node.Parse([]byte(aGot)); err != nil || n.ULID == "" {
		t.Fatalf("notes/a.md not adopted: %v %q", err, aGot)
	}

	for _, tc := range []struct{ path, want string }{
		{filepath.Join(vault, "notes", "b.md"), bSrc},
		{filepath.Join(vault, ".obsidian", "config.md"), obsidianSrc},
		{filepath.Join(vault, ".git", "x.md"), gitSrc},
		{filepath.Join(vault, "notes", "readme.txt"), readmeSrc},
	} {
		if got := mustReadFile(t, tc.path); got != tc.want {
			t.Errorf("%s was modified, want untouched\n--- want ---\n%q\n--- got ---\n%q", tc.path, tc.want, got)
		}
	}
}

// An explicit non-.md file argument is rejected (destructive-if-wrong: adopt
// must not inject a YAML-ish block into an arbitrary file), and the file is
// left unmodified.
func TestAdoptCmd_ExplicitNonMarkdownFileErrors(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	target := filepath.Join(vault, "data.json")
	const src = `{"key": "value"}` + "\n"
	mustWriteFile(t, target, src)

	if _, err := runAdopt(t, vault, target); err == nil {
		t.Fatal("expected an error adopting a non-.md file")
	}

	if got := mustReadFile(t, target); got != src {
		t.Fatalf("data.json was modified\n--- want ---\n%q\n--- got ---\n%q", src, got)
	}
}

// A git/Syncthing conflict-marker file surfaces the parser's refusal
// (node.Parse explicitly refuses these) as a per-file error, and is left
// unmodified.
func TestAdoptCmd_ConflictMarkerFileErrors(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	target := filepath.Join(vault, "conflict.md")
	const src = "<<<<<<< HEAD\nmine\n=======\ntheirs\n>>>>>>> other\n"
	mustWriteFile(t, target, src)

	if _, err := runAdopt(t, vault, target); err == nil {
		t.Fatal("expected an error adopting a conflict-marker file")
	}

	if got := mustReadFile(t, target); got != src {
		t.Fatalf("conflict.md was modified\n--- want ---\n%q\n--- got ---\n%q", src, got)
	}
}

// A CRLF file is a per-file error (known parser-scope gap, tracked separately
// as reckon-vj55 — plan.md D8), not silently mishandled or mixed-line-ending
// corrupted; the file is left unmodified.
func TestAdoptCmd_CRLFFileErrors(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	target := filepath.Join(vault, "crlf.md")
	const src = "---\r\ntype: note\r\n---\r\nbody with CRLF endings\r\n"
	mustWriteFile(t, target, src)

	if _, err := runAdopt(t, vault, target); err == nil {
		t.Fatal("expected an error adopting a CRLF file (reckon-vj55 known limitation)")
	}

	if got := mustReadFile(t, target); got != src {
		t.Fatalf("crlf.md was modified\n--- want ---\n%q\n--- got ---\n%q", src, got)
	}
}

// An unterminated frontmatter block (opens "---\n" but never closes) is a
// per-file error, not silently treated as "no frontmatter" (which would nest
// the user's attempted block into the body) — mirrors
// node.InsertField's refusal (insert_test.go). The file is left unmodified.
func TestAdoptCmd_UnterminatedFrontmatterErrors(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	target := filepath.Join(vault, "unterminated.md")
	const src = "---\ntype: note\nthis frontmatter block never closes\n"
	mustWriteFile(t, target, src)

	if _, err := runAdopt(t, vault, target); err == nil {
		t.Fatal("expected an error adopting an unterminated-frontmatter file")
	}

	if got := mustReadFile(t, target); got != src {
		t.Fatalf("unterminated.md was modified\n--- want ---\n%q\n--- got ---\n%q", src, got)
	}
}

// `rk adopt` never opens, reads, or writes the SQLite index: running it in a
// vault with no cache/index directory yet on disk must not create one. The
// index only picks up the new ULID on the next normal `rk index`/reconcile.
func TestAdoptCmd_NeverTouchesIndex(t *testing.T) {
	vault, cache := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	target := filepath.Join(vault, "fresh.md")
	mustWriteFile(t, target, "---\ntype: note\n---\nno index yet\n")

	if _, err := runAdopt(t, vault, target); err != nil {
		t.Fatalf("rk adopt: %v", err)
	}

	if _, err := os.Stat(cache); err == nil {
		t.Errorf("rk adopt created the cache dir %q; it must never touch the index", cache)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat cache dir: %v", err)
	}
}

// The minted id is a syntactically valid ULID (26-char Crockford base32),
// indistinguishable in format from node.Mint's own output.
func TestAdoptCmd_MintedULIDIsValidFormat(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	target := filepath.Join(vault, "mint.md")
	mustWriteFile(t, target, "---\ntype: note\n---\nmint me\n")

	out, err := runAdopt(t, vault, "--json", target)
	if err != nil {
		t.Fatalf("rk adopt: %v (output: %s)", err, out)
	}

	var res adoptResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("decode json output %q: %v", out, err)
	}
	if len(res.Adopted) != 1 {
		t.Fatalf("Adopted = %+v, want 1 entry", res.Adopted)
	}
	if !isValidULID(res.Adopted[0].ULID) {
		t.Errorf("minted id %q is not a valid 26-char Crockford ULID", res.Adopted[0].ULID)
	}
}

// A path outside the vault root is rejected (plan.md D7: rk adopt is
// vault-scoped, mirroring config.LoadWithOverrides's cache-inside-vault
// containment guard), and the file is left unmodified.
func TestAdoptCmd_PathOutsideVaultErrors(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	outside := t.TempDir() // a sibling temp dir, guaranteed not under vault
	target := filepath.Join(outside, "file.md")
	const src = "---\ntype: note\n---\noutside the vault\n"
	mustWriteFile(t, target, src)

	if _, err := runAdopt(t, vault, target); err == nil {
		t.Fatal("expected an error adopting a path outside the vault root")
	}

	if got := mustReadFile(t, target); got != src {
		t.Fatalf("out-of-vault file was modified\n--- want ---\n%q\n--- got ---\n%q", src, got)
	}
}

// --quiet suppresses the pretty status line (matching indexCmd's
// mode==Pretty && quietFlag convention, internal/cli/index.go) without
// affecting the underlying write.
func TestAdoptCmd_QuietSuppressesPrettyStatusLine(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	target := filepath.Join(vault, "quiet.md")
	mustWriteFile(t, target, "---\ntype: note\n---\nquiet please\n")

	out, err := runAdopt(t, vault, "--quiet", target)
	if err != nil {
		t.Fatalf("rk adopt: %v", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("--quiet did not suppress the pretty status line: %q", out)
	}

	n, err := node.Parse([]byte(mustReadFile(t, target)))
	if err != nil || n.ULID == "" {
		t.Fatalf("file not adopted despite --quiet: %v", err)
	}
}

// Default (pretty) output mentions the adopted file, mirroring
// TestIndexCommandAliasCollisionPretty's style of asserting on a meaningful
// substring of the human-readable status line.
func TestAdoptCmd_PrettyOutputMentionsAdoptedFile(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	target := filepath.Join(vault, "pretty.md")
	mustWriteFile(t, target, "---\ntype: note\n---\npretty output\n")

	out, err := runAdopt(t, vault, target)
	if err != nil {
		t.Fatalf("rk adopt: %v", err)
	}
	if !strings.Contains(out, "pretty.md") {
		t.Errorf("pretty output does not mention the adopted file %q: %q", "pretty.md", out)
	}
}
