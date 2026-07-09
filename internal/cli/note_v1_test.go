// Package cli — TDD red tests for v1-T8 `rk note` + linking (reckon-ih5g).
//
// internal/cli/note_v1.go (noteCreateCmd/noteShowCmd/noteRenameCmd/
// noteIndexCmd) does not exist yet. Unlike reckon-uv09/reckon-qiua's own red
// test files (add_test.go/todo_test.go), which reference not-yet-defined
// package-level symbols (logAddResult, todoAddResult, ...) to force a BUILD
// failure, this file references NO such symbol: every scenario below drives
// the command tree by name only (`rk note create/show/rename/index` via
// RootCmd.Execute), so the `cli` package compiles today.
//
// The runtime-red mechanism is more subtle than a stub returning
// errNotImplemented, and is worth spelling out precisely (verified directly
// against this worktree's cobra v1.10.2 vendor, command.go:905-960, not
// assumed):
//
//   - `noteCmd` (note.go) is the existing LEGACY parent for `rk note`
//     (subcommands today: "new", "list"). It has no RunE of its own — it is
//     a pure container.
//   - `rk note create ...` / `show` / `rename` / `index` do not match any of
//     noteCmd's two existing subcommand names, so cobra's Find() stops
//     descending at noteCmd itself and hands it the remaining args
//     ("create", "PAS Entity Model", ...) as plain positional arguments.
//   - Because noteCmd itself is not Runnable() (no Run/RunE), cobra's
//     execute() returns flag.ErrHelp — BEFORE even reaching the
//     PersistentPreRunE chain (root.go's requiresDB walk never runs, so no
//     legacy DB init is attempted either) — and ExecuteC() converts
//     flag.ErrHelp into "print noteCmd's help text, return nil".
//   - Net effect: `RootCmd.Execute()` returns a NIL error and stdout holds
//     noteCmd's help page, NOT JSON and NOT a file write — for every one of
//     the four not-yet-existing subcommands, regardless of the args/flags
//     that follow, UNLESS those args include a flag string cobra doesn't
//     recognize on noteCmd itself (e.g. `--stage`), in which case
//     ParseFlags fails first with a real "unknown flag: --stage" error.
//
// Consequently every scenario below fails at runtime via one of two
// deterministic paths, never a trivial "any error will do" check:
//   - Happy-path scenarios decode the (actually-help-text) stdout as JSON via
//     the shared mustDecodeJSON helper (todo_test.go) — json.Unmarshal on a
//     human-readable help page reliably errors, so mustDecodeJSON's own
//     t.Fatalf fires.
//   - Rejection scenarios assert `err == nil` is itself the failure (cobra's
//     "unknown subcommand" path returns nil, not an error) — so `if err ==
//     nil { t.Fatal(...) }` fires immediately, exactly the same shape as
//     "expected an error, got nil" in every sibling *_test.go file, except
//     here it is guaranteed to trigger in red state instead of only
//     sometimes.
//   - The two scenarios that pass an as-yet-unregistered FLAG (TS-10's
//     `--stage`) do get a real non-nil cobra parse error ("unknown flag:
//     --stage"); those assert on a domain-specific substring (e.g.
//     "seedling"/"budding"/"evergreen") that is provably absent from cobra's
//     generic flag-parse message, so the substring check's t.Errorf fires.
//
// Some scenarios (TS-3/4/5/6/12/13) test already-shipped, type-agnostic
// index machinery (body-wikilink→references edges, ref-prop→typed edges,
// dangling-edge resolution — internal/node + internal/index, proven for
// `todo` type by TestTodoDependsOn_DanglingResolvesLater) applied to `note`
// nodes. To keep every scenario genuinely red rather than accidentally
// passing against already-shipped generic behavior, each such test relies on
// at least one note (typically the link TARGET) being created through the
// not-yet-existing `rk note create` — which is what must self-mint the
// note's own canonical alias into its `aliases:` frontmatter (plan.md's
// "load-bearing wiring" finding) for `[[slug]]` links elsewhere to ever
// resolve a `dst_key` at all. A hand-authored SOURCE/linking note
// (writeTestNode) is fine to use as supporting fixture — only the
// self-minted-alias side is what T8 actually adds.
//
// Precedent / harness reuse (do not redefine these — they already live in
// this package): setupQueryVault, writeTestNode, resetCLIFlags, buildIndex,
// runQuery, parseNDJSONMaps (query_test.go); mustDecodeJSON (todo_test.go);
// mustWriteFile, mustReadFile, isValidULID (adopt_test.go).
//
// ─────────────────────────────────────────────────────────────────────────
// Pinned contract this file exercises (plan.md "Create field conventions" /
// "Show output format" / "Rename command surface" / "index.md generation
// choice"), reproduced here for readers, NOT referenced as Go symbols:
// ─────────────────────────────────────────────────────────────────────────
//
//	rk note create <title> [--description D] [--stage S] [--tag T]...
//	                        [--alias A]... [--slug SLUG] [--dir SUBDIR]
//	                        [--body TEXT] [--type TYPE] [--author A]
//	  -> writes notes/<slug-or-override>.md (or notes/<dir>/<slug>.md);
//	     self-mints `aliases: [<slug>]`; JSON result includes at least
//	     "id" (the new ULID) and "path" (vault-relative), mirroring every
//	     other v1 create command's result shape (todoAddResult, logAddResult).
//
//	rk note show <ref> [--json]
//	  -> noteShowResult: id, type, title, description, stage, aliases, path,
//	     forward_links []{rel,dst,dst_key}, backlinks []{src,rel}.
//
//	rk note rename <ref> "<new title>" [--json]
//	  -> renames notes/<old-slug>.md -> notes/<new-slug>.md, id: unchanged,
//	     aliases: gains the new slug alongside the retained old slug (via
//	     the new internal/node SetAliases primitive for the block-list
//	     case).
//
//	rk note index [--json]
//	  -> (re)generates notes/index.md (or notes/<dir>/index.md per
//	     populated subdirectory): one markdown-link entry per note, sorted,
//	     deterministic byte-for-byte across repeated runs with no vault
//	     changes.
package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MikeBiancalana/reckon/internal/node"
)

// ─────────────────────────────────────────────────────────────────────────────
// Harness helper (note-specific; vault/cache/flag-reset/JSON-decode plumbing
// is shared with query_test.go/todo_test.go/adopt_test.go).
// ─────────────────────────────────────────────────────────────────────────────

// runNote executes `rk note --vault <vault> <args...>` through RootCmd and
// returns (stdout, stderr, error), mirroring runTodo/runAdd/runQuery. The
// caller must call resetCLIFlags() before another Execute within the same
// test (t.Cleanup(resetCLIFlags) covers end-of-test).
func runNote(t *testing.T, vault string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	RootCmd.SetOut(&outBuf)
	RootCmd.SetErr(&errBuf)
	full := append([]string{"note", "--vault", vault}, args...)
	RootCmd.SetArgs(full)
	err = RootCmd.Execute()
	return outBuf.String(), errBuf.String(), err
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-1 (AC-1/AC-2/AC-8): fresh create, slug filename, round-trip.
// ─────────────────────────────────────────────────────────────────────────────

func TestNoteCreate_SlugFilenameAndRoundTrip(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	out, stderr, err := runNote(t, vault, "create", "PAS Entity Model", "--json")
	if err != nil {
		t.Fatalf("rk note create: %v\nstderr: %s", err, stderr)
	}

	var res map[string]any
	mustDecodeJSON(t, out, &res)

	wantPath := filepath.Join(vault, "notes", "pas-entity-model.md")
	raw := mustReadFile(t, wantPath)

	n, err := node.Parse([]byte(raw))
	if err != nil {
		t.Fatalf("parse created file: %v", err)
	}
	if !isValidULID(n.ULID) {
		t.Errorf("ULID %q is not a valid ULID", n.ULID)
	}
	if n.Type != "note" {
		t.Errorf("Type = %q, want note", n.Type)
	}
	if n.Props["title"] != "PAS Entity Model" {
		t.Errorf("Props[title] = %q, want %q", n.Props["title"], "PAS Entity Model")
	}
	foundSelfAlias := false
	for _, a := range n.Aliases {
		if a == "pas-entity-model" {
			foundSelfAlias = true
		}
	}
	if !foundSelfAlias {
		t.Errorf("Aliases = %v, want to contain the self-minted slug %q", n.Aliases, "pas-entity-model")
	}
	if got := n.Serialize(); string(got) != raw {
		t.Errorf("parse(raw).Serialize() != raw\n--- raw ---\n%q\n--- got ---\n%q", raw, got)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-2 (AC-9): filename is never identity — an externally renamed file still
// resolves by its unchanged `id:` after reconcile.
// ─────────────────────────────────────────────────────────────────────────────

func TestNoteCreate_FilenameNotIdentity(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	out, stderr, err := runNote(t, vault, "create", "PAS Entity Model", "--json")
	if err != nil {
		t.Fatalf("rk note create: %v\nstderr: %s", err, stderr)
	}
	var res map[string]any
	mustDecodeJSON(t, out, &res)
	id, _ := res["id"].(string)
	resetCLIFlags()

	origPath := filepath.Join(vault, "notes", "pas-entity-model.md")
	renamedPath := filepath.Join(vault, "notes", "renamed-externally-not-via-tool.md")
	if err := os.Rename(origPath, renamedPath); err != nil {
		t.Fatalf("external (non-tool) rename: %v", err)
	}

	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault, fmt.Sprintf("SELECT id FROM nodes WHERE id='%s'", id))
	if err != nil {
		t.Fatalf("rk query: %v\nstderr: %s", err, stderr)
	}
	rows := parseNDJSONMaps(t, stdout)
	if len(rows) != 1 {
		t.Fatalf("expected the node to still resolve by its unchanged id after an external filename rename, got %d rows: %v", len(rows), rows)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-3 (AC-3): body [[wikilink]] -> a `references` edge, resolved via the
// target note's self-minted alias.
// ─────────────────────────────────────────────────────────────────────────────

func TestNoteLinks_BodyWikilinkReferencesEdge(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	// Note A: hand-authored fixture, body links to "note-b" (already-shipped
	// generic body-link mechanism -- not the thing this test is pinning).
	aID := node.Mint()
	writeTestNode(t, vault, "notes/note-a.md", aID, "note", "See [[note-b]] for details.")

	// Note B must be created via `rk note create`, which must self-mint the
	// slug "note-b" into its own aliases: line -- absent that, dst_key can
	// never resolve.
	out, stderr, err := runNote(t, vault, "create", "Note B", "--json")
	if err != nil {
		t.Fatalf("rk note create: %v\nstderr: %s", err, stderr)
	}
	var res map[string]any
	mustDecodeJSON(t, out, &res)
	resetCLIFlags()

	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault, fmt.Sprintf(
		"SELECT rel, dst, dst_key FROM edges WHERE rel='references' AND src='%s'", aID))
	if err != nil {
		t.Fatalf("rk query: %v\nstderr: %s", err, stderr)
	}
	rows := parseNDJSONMaps(t, stdout)
	if len(rows) != 1 {
		t.Fatalf("expected exactly 1 references edge, got %d: %v", len(rows), rows)
	}
	if rows[0]["dst_key"] == nil || rows[0]["dst_key"] == "" {
		t.Errorf("dst_key = %v, want resolved to note B's node_key (rk note create must self-mint its own alias)", rows[0]["dst_key"])
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-4 (AC-3): a ref-valued frontmatter prop -> a TYPED edge (rel = the prop
// key), not `references`.
// ─────────────────────────────────────────────────────────────────────────────

func TestNoteLinks_TypedEdgeFromRefProp(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	aID := node.Mint()
	writeTestNode(t, vault, "notes/note-a.md", aID, "note", "Note A body.", `depends: "[[note-b]]"`)

	out, stderr, err := runNote(t, vault, "create", "Note B", "--json")
	if err != nil {
		t.Fatalf("rk note create: %v\nstderr: %s", err, stderr)
	}
	var res map[string]any
	mustDecodeJSON(t, out, &res)
	resetCLIFlags()

	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault, fmt.Sprintf(
		"SELECT rel, dst_key FROM edges WHERE rel='depends' AND src='%s'", aID))
	if err != nil {
		t.Fatalf("rk query: %v\nstderr: %s", err, stderr)
	}
	rows := parseNDJSONMaps(t, stdout)
	if len(rows) != 1 {
		t.Fatalf("expected exactly 1 depends edge, got %d: %v", len(rows), rows)
	}
	if rows[0]["rel"] != "depends" {
		t.Errorf("rel = %v, want depends (not references)", rows[0]["rel"])
	}
	if rows[0]["dst_key"] == nil || rows[0]["dst_key"] == "" {
		t.Errorf("dst_key = %v, want resolved (rk note create must self-mint note B's alias)", rows[0]["dst_key"])
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-5 (AC-4): backlinks are index-derived only, never stored on the target
// note's own file.
// ─────────────────────────────────────────────────────────────────────────────

func TestNoteBacklinks_IndexDerivedNotStored(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	outB, stderrB, errB := runNote(t, vault, "create", "Note B", "--json")
	if errB != nil {
		t.Fatalf("rk note create Note B: %v\nstderr: %s", errB, stderrB)
	}
	var resB map[string]any
	mustDecodeJSON(t, outB, &resB)
	bID, _ := resB["id"].(string)
	resetCLIFlags()

	aID := node.Mint()
	writeTestNode(t, vault, "notes/note-a.md", aID, "note", "Linking to [[note-b]] here.")

	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault, fmt.Sprintf(
		"SELECT src FROM edges WHERE dst_key='%s'", bID))
	if err != nil {
		t.Fatalf("rk query backlinks: %v\nstderr: %s", err, stderr)
	}
	rows := parseNDJSONMaps(t, stdout)
	if len(rows) != 1 || rows[0]["src"] != aID {
		t.Fatalf("expected exactly 1 backlink from note A (%q), got %v", aID, rows)
	}

	bRaw := mustReadFile(t, filepath.Join(vault, "notes", "note-b.md"))
	if strings.Contains(bRaw, aID) {
		t.Errorf("note B's own on-disk file must contain no inbound-link data, but it contains A's ULID %q", aID)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-6 (AC-5): a dangling link resolves once its target is later created —
// mirrors TestTodoDependsOn_DanglingResolvesLater (todo_test.go:1149) for the
// `note`/`references` case.
// ─────────────────────────────────────────────────────────────────────────────

func TestNoteLink_DanglingResolvesLater(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	aID := node.Mint()
	writeTestNode(t, vault, "notes/note-a.md", aID, "note", "See [[not-yet-created]] soon.")

	buildIndex(t, vault)

	stdoutBefore, _, err := runQuery(t, vault,
		"SELECT dst_key FROM edges WHERE rel='references' AND dst='not-yet-created'")
	if err != nil {
		t.Fatalf("rk query before target exists: %v", err)
	}
	resetCLIFlags()
	rowsBefore := parseNDJSONMaps(t, stdoutBefore)
	if len(rowsBefore) != 1 || rowsBefore[0]["dst_key"] != nil {
		t.Fatalf("expected 1 dangling edge before the target note exists, got %v", rowsBefore)
	}

	out, stderr, err := runNote(t, vault, "create", "Not Yet Created", "--json")
	if err != nil {
		t.Fatalf("rk note create: %v\nstderr: %s", err, stderr)
	}
	var res map[string]any
	mustDecodeJSON(t, out, &res)
	resetCLIFlags()

	buildIndex(t, vault)

	stdoutAfter, _, err := runQuery(t, vault,
		"SELECT dst_key FROM edges WHERE rel='references' AND dst='not-yet-created'")
	if err != nil {
		t.Fatalf("rk query after target exists: %v", err)
	}
	rowsAfter := parseNDJSONMaps(t, stdoutAfter)
	if len(rowsAfter) != 1 || rowsAfter[0]["dst_key"] == nil {
		t.Fatalf("expected the edge to resolve once the target note exists (no edit to note A), got %v", rowsAfter)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-7 (AC-6): rename retains the old slug as an alias/redirect — flow-list
// aliases (the clean case).
// ─────────────────────────────────────────────────────────────────────────────

func TestNoteRename_RetainsOldAlias_FlowList(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	out, stderr, err := runNote(t, vault, "create", "Old Title", "--json")
	if err != nil {
		t.Fatalf("rk note create: %v\nstderr: %s", err, stderr)
	}
	var res map[string]any
	mustDecodeJSON(t, out, &res)
	resetCLIFlags()

	linkerID := node.Mint()
	writeTestNode(t, vault, "notes/linker.md", linkerID, "note", "See [[old-title]] for context.")

	renameOut, renameStderr, renameErr := runNote(t, vault, "rename", "old-title", "New Title", "--json")
	if renameErr != nil {
		t.Fatalf("rk note rename: %v\nstderr: %s", renameErr, renameStderr)
	}
	var renameRes map[string]any
	mustDecodeJSON(t, renameOut, &renameRes)
	resetCLIFlags()

	oldPath := filepath.Join(vault, "notes", "old-title.md")
	if _, statErr := os.Stat(oldPath); !os.IsNotExist(statErr) {
		t.Errorf("old file %q must no longer exist after rename, stat err = %v", oldPath, statErr)
	}
	newPath := filepath.Join(vault, "notes", "new-title.md")
	raw := mustReadFile(t, newPath)
	n, err := node.Parse([]byte(raw))
	if err != nil {
		t.Fatalf("parse renamed file: %v", err)
	}
	hasOld, hasNew := false, false
	for _, a := range n.Aliases {
		if a == "old-title" {
			hasOld = true
		}
		if a == "new-title" {
			hasNew = true
		}
	}
	if !hasOld || !hasNew {
		t.Errorf("Aliases = %v, want both old-title (retained redirect) and new-title (canonical)", n.Aliases)
	}

	buildIndex(t, vault)
	stdout, _, err := runQuery(t, vault,
		"SELECT dst_key FROM edges WHERE rel='references' AND dst='old-title'")
	if err != nil {
		t.Fatalf("rk query: %v", err)
	}
	rows := parseNDJSONMaps(t, stdout)
	if len(rows) != 1 || rows[0]["dst_key"] == nil {
		t.Errorf("[[old-title]] must still resolve after rename (zero edits to the linking note), got %v", rows)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-8 (AC-6/EC-13): rename retains the old alias — Obsidian block-list
// aliases (the verified corruption-repro gap that motivates the new
// internal/node SetAliases primitive).
// ─────────────────────────────────────────────────────────────────────────────

func TestNoteRename_RetainsOldAlias_BlockList(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	id := node.Mint()
	raw := "---\n" +
		"id: " + id + "\n" +
		"type: note\n" +
		"aliases:\n" +
		"  - old-title\n" +
		"---\n" +
		"# Old Title\n"
	path := filepath.Join(vault, "notes", "old-title.md")
	mustWriteFile(t, path, raw)

	out, stderr, err := runNote(t, vault, "rename", "old-title", "New Title", "--json")
	if err != nil {
		t.Fatalf("rk note rename: %v\nstderr: %s", err, stderr)
	}
	var res map[string]any
	mustDecodeJSON(t, out, &res)

	newPath := filepath.Join(vault, "notes", "new-title.md")
	newRaw := mustReadFile(t, newPath)

	if strings.Count(newRaw, "aliases:") > 1 {
		t.Fatalf("duplicate 'aliases:' key present (the InsertField/block-list corruption bug this ticket must fix), got:\n%s", newRaw)
	}
	if !strings.Contains(newRaw, "aliases: [") {
		t.Errorf("expected the block-list aliases to collapse to canonical flow form (aliases: [...]), got:\n%s", newRaw)
	}
	n, err := node.Parse([]byte(newRaw))
	if err != nil {
		t.Fatalf("parse renamed file: %v", err)
	}
	hasOld, hasNew := false, false
	for _, a := range n.Aliases {
		if a == "old-title" {
			hasOld = true
		}
		if a == "new-title" {
			hasNew = true
		}
	}
	if !hasOld || !hasNew {
		t.Errorf("Aliases = %v, want both old-title and new-title (SetAliases must handle the block-list shape)", n.Aliases)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-9 (AC-11/EC-4): no --description supplied is legal; description simply
// absent from Props.
// ─────────────────────────────────────────────────────────────────────────────

func TestNoteCreate_NoDescriptionSucceeds(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	out, stderr, err := runNote(t, vault, "create", "Some Title", "--json")
	if err != nil {
		t.Fatalf("rk note create: %v\nstderr: %s", err, stderr)
	}
	var res map[string]any
	mustDecodeJSON(t, out, &res)

	raw := mustReadFile(t, filepath.Join(vault, "notes", "some-title.md"))
	n, err := node.Parse([]byte(raw))
	if err != nil {
		t.Fatalf("parse created file: %v", err)
	}
	if v, has := n.Props["description"]; has {
		t.Errorf("Props[description] = %q present, want absent when --description is omitted", v)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-10 (AC-12/EC-5): an invalid --stage value is rejected at the CLI layer.
// ─────────────────────────────────────────────────────────────────────────────

func TestNoteCreate_InvalidStageRejected(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	_, stderr, err := runNote(t, vault, "create", "T", "--stage", "mature", "--json")
	if err == nil {
		t.Fatal("expected an error for an invalid --stage value, got nil")
	}
	combined := strings.ToLower(stderr + err.Error())
	if !strings.Contains(combined, "seedling") && !strings.Contains(combined, "budding") &&
		!strings.Contains(combined, "evergreen") {
		t.Errorf("expected the error to mention the valid stage enum (seedling|budding|evergreen), got stderr=%q err=%v", stderr, err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-11 (AC-13): the vault never stores an updated/timestamp field, even
// after a hand-edit outside the tool.
// ─────────────────────────────────────────────────────────────────────────────

func TestNoteCreate_NoTimestampField(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	out, stderr, err := runNote(t, vault, "create", "Timestamp Check", "--json")
	if err != nil {
		t.Fatalf("rk note create: %v\nstderr: %s", err, stderr)
	}
	var res map[string]any
	mustDecodeJSON(t, out, &res)

	path := filepath.Join(vault, "notes", "timestamp-check.md")
	raw := mustReadFile(t, path)

	// Simulate a hand-edit (content change, not via the tool).
	edited := raw + "\nHand-edited addendum, not via the tool.\n"
	mustWriteFile(t, path, edited)

	reraw := mustReadFile(t, path)
	n, err := node.Parse([]byte(reraw))
	if err != nil {
		t.Fatalf("re-parse hand-edited file: %v", err)
	}
	if v, has := n.Props["updated"]; has {
		t.Errorf("Props[updated] unexpectedly present: %q", v)
	}
	if v, has := n.Props["timestamp"]; has {
		t.Errorf("Props[timestamp] unexpectedly present: %q", v)
	}
	if n.Time == "" {
		t.Errorf("Time (the reserved `time:` field) must be present as the sole stored timestamp")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-12 (EC-6): [[link|label]] pipe syntax is stripped, resolving identically
// to an unlabeled [[link]].
// ─────────────────────────────────────────────────────────────────────────────

func TestNoteLinks_PipeSyntaxStripped(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	aID := node.Mint()
	writeTestNode(t, vault, "notes/note-a.md", aID, "note", "See [[note-b|Note B's Nicer Title]] for more.")

	out, stderr, err := runNote(t, vault, "create", "Note B", "--json")
	if err != nil {
		t.Fatalf("rk note create: %v\nstderr: %s", err, stderr)
	}
	var res map[string]any
	mustDecodeJSON(t, out, &res)
	resetCLIFlags()

	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault, fmt.Sprintf(
		"SELECT dst, dst_key FROM edges WHERE rel='references' AND src='%s'", aID))
	if err != nil {
		t.Fatalf("rk query: %v\nstderr: %s", err, stderr)
	}
	rows := parseNDJSONMaps(t, stdout)
	if len(rows) != 1 {
		t.Fatalf("expected exactly 1 references edge, got %d: %v", len(rows), rows)
	}
	if rows[0]["dst"] != "note-b" {
		t.Errorf("dst = %v, want %q (pipe label stripped)", rows[0]["dst"], "note-b")
	}
	if rows[0]["dst_key"] == nil || rows[0]["dst_key"] == "" {
		t.Errorf("dst_key = %v, want resolved", rows[0]["dst_key"])
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-13 (EC-7/AC-10): a raw [[ULID]] link resolves in reckon via _nodes.ulid,
// independent of the target's own aliases/filename (the documented
// reckon-resolves/Obsidian-dangles divergence).
// ─────────────────────────────────────────────────────────────────────────────

func TestNoteLinks_RawULIDResolvesInReckon(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	// Note B: created via the tool, filed under its slug. Its own aliases:
	// will NOT list its ULID (the tool self-mints only the slug) -- the
	// documented per-note escape hatch (AC-10) is opt-in, not automatic.
	out, stderr, err := runNote(t, vault, "create", "Note B", "--json")
	if err != nil {
		t.Fatalf("rk note create: %v\nstderr: %s", err, stderr)
	}
	var res map[string]any
	mustDecodeJSON(t, out, &res)
	bID, _ := res["id"].(string)
	resetCLIFlags()

	aID := node.Mint()
	writeTestNode(t, vault, "notes/note-a.md", aID, "note", "Raw link: [["+bID+"]].")

	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault, fmt.Sprintf(
		"SELECT dst_key FROM edges WHERE rel='references' AND src='%s' AND dst='%s'", aID, bID))
	if err != nil {
		t.Fatalf("rk query: %v\nstderr: %s", err, stderr)
	}
	rows := parseNDJSONMaps(t, stdout)
	if len(rows) != 1 {
		t.Fatalf("expected exactly 1 raw-ULID references edge, got %d: %v", len(rows), rows)
	}
	// Reckon's own resolver must match via _nodes.ulid regardless of B's
	// filename/aliases. NOTE (documented, expected divergence, not asserted
	// here since it's Obsidian-side UI behavior): the same [[ULID]] link
	// would show as broken inside Obsidian's own editor, since B's aliases:
	// never lists its own ULID.
	if rows[0]["dst_key"] != bID {
		t.Errorf("dst_key = %v, want %q (reckon resolves raw-ULID links via _nodes.ulid independent of aliases)", rows[0]["dst_key"], bID)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-14 (EC-9): an empty or invalid-slug title is rejected, no file written.
// ─────────────────────────────────────────────────────────────────────────────

func TestNoteCreate_EmptyOrInvalidSlugRejected(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	for _, title := range []string{"!!!", ""} {
		t.Run(fmt.Sprintf("title=%q", title), func(t *testing.T) {
			resetCLIFlags()
			_, stderr, err := runNote(t, vault, "create", title, "--json")
			if err == nil {
				t.Fatalf("expected an error for title %q (produces an empty/invalid slug), got nil", title)
			}
			combined := strings.ToLower(stderr + err.Error())
			if !strings.Contains(combined, "slug") {
				t.Errorf("expected a slug-specific error for title %q, got stderr=%q err=%v", title, stderr, err)
			}
		})
	}

	if entries, statErr := os.ReadDir(filepath.Join(vault, "notes")); statErr == nil && len(entries) != 0 {
		t.Errorf("no file should have been written for a rejected invalid-slug title, found: %v", entries)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-15 (EC-10): a title that slugifies to the reserved "index" is rejected.
// ─────────────────────────────────────────────────────────────────────────────

func TestNoteCreate_ReservedIndexSlugRejected(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	_, stderr, err := runNote(t, vault, "create", "Index", "--json")
	if err == nil {
		t.Fatal("expected an error for a title that slugifies to the reserved 'index', got nil")
	}
	combined := strings.ToLower(stderr + err.Error())
	if !strings.Contains(combined, "reserved") {
		t.Errorf("expected a reserved-slug-specific error, got stderr=%q err=%v", stderr, err)
	}

	if _, statErr := os.Stat(filepath.Join(vault, "notes", "index.md")); !os.IsNotExist(statErr) {
		t.Errorf("no notes/index.md should exist as a result of a rejected create, stat err = %v", statErr)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-16 (AC-14): `rk note index` generates a deterministic catalog.
// ─────────────────────────────────────────────────────────────────────────────

func TestNoteIndex_GeneratesDeterministicCatalog(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	out1, stderr1, err1 := runNote(t, vault, "create", "Alpha Note", "--description", "First note.", "--json")
	if err1 != nil {
		t.Fatalf("rk note create Alpha: %v\nstderr: %s", err1, stderr1)
	}
	var res1 map[string]any
	mustDecodeJSON(t, out1, &res1)
	resetCLIFlags()

	out2, stderr2, err2 := runNote(t, vault, "create", "Beta Note", "--description", "Second note.", "--json")
	if err2 != nil {
		t.Fatalf("rk note create Beta: %v\nstderr: %s", err2, stderr2)
	}
	var res2 map[string]any
	mustDecodeJSON(t, out2, &res2)
	resetCLIFlags()

	if _, _, err := runNote(t, vault, "index", "--json"); err != nil {
		t.Fatalf("rk note index: %v", err)
	}
	resetCLIFlags()

	indexPath := filepath.Join(vault, "notes", "index.md")
	first := mustReadFile(t, indexPath)
	if !strings.Contains(first, "Alpha Note") || !strings.Contains(first, "First note.") {
		t.Errorf("index.md missing the Alpha Note entry, got:\n%s", first)
	}
	if !strings.Contains(first, "Beta Note") || !strings.Contains(first, "Second note.") {
		t.Errorf("index.md missing the Beta Note entry, got:\n%s", first)
	}

	if _, _, err := runNote(t, vault, "index", "--json"); err != nil {
		t.Fatalf("rk note index (second run): %v", err)
	}
	second := mustReadFile(t, indexPath)
	if first != second {
		t.Errorf("rk note index is not deterministic across repeated runs with no vault changes:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TS-17 (EC-1): a duplicate title/slug is rejected; --slug is the escape
// hatch.
// ─────────────────────────────────────────────────────────────────────────────

func TestNoteCreate_DuplicateSlugRejected(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	out, stderr, err := runNote(t, vault, "create", "Duplicate Example", "--json")
	if err != nil {
		t.Fatalf("rk note create (first): %v\nstderr: %s", err, stderr)
	}
	var res map[string]any
	mustDecodeJSON(t, out, &res)
	resetCLIFlags()

	_, stderr2, err2 := runNote(t, vault, "create", "Duplicate Example", "--json")
	if err2 == nil {
		t.Fatal("expected the second create with a colliding slug to be rejected, got nil")
	}
	combined := strings.ToLower(stderr2 + err2.Error())
	if !strings.Contains(combined, "duplicate") && !strings.Contains(combined, "exists") && !strings.Contains(combined, "collis") {
		t.Errorf("expected a duplicate-slug-specific error, got stderr=%q err=%v", stderr2, err2)
	}
	resetCLIFlags()

	out3, stderr3, err3 := runNote(t, vault, "create", "Duplicate Example", "--slug", "duplicate-example-2", "--json")
	if err3 != nil {
		t.Fatalf("rk note create --slug override: %v\nstderr: %s", err3, stderr3)
	}
	var res3 map[string]any
	mustDecodeJSON(t, out3, &res3)
	if _, statErr := os.Stat(filepath.Join(vault, "notes", "duplicate-example-2.md")); statErr != nil {
		t.Errorf("expected the --slug override to create notes/duplicate-example-2.md: %v", statErr)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// EC-2: renaming to a title whose slug another note already owns is
// rejected.
// ─────────────────────────────────────────────────────────────────────────────

func TestNoteRename_NewSlugCollisionRejected(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	outA, stderrA, errA := runNote(t, vault, "create", "Note A", "--json")
	if errA != nil {
		t.Fatalf("rk note create A: %v\nstderr: %s", errA, stderrA)
	}
	var resA map[string]any
	mustDecodeJSON(t, outA, &resA)
	resetCLIFlags()

	outB, stderrB, errB := runNote(t, vault, "create", "Note B", "--json")
	if errB != nil {
		t.Fatalf("rk note create B: %v\nstderr: %s", errB, stderrB)
	}
	var resB map[string]any
	mustDecodeJSON(t, outB, &resB)
	resetCLIFlags()

	_, stderr, err := runNote(t, vault, "rename", "note-a", "Note B", "--json")
	if err == nil {
		t.Fatal("expected renaming to an already-claimed slug to be rejected, got nil")
	}
	combined := strings.ToLower(stderr + err.Error())
	if !strings.Contains(combined, "collis") && !strings.Contains(combined, "exists") && !strings.Contains(combined, "duplicate") {
		t.Errorf("expected a slug-collision-specific rename error, got stderr=%q err=%v", stderr, err)
	}

	if _, statErr := os.Stat(filepath.Join(vault, "notes", "note-a.md")); statErr != nil {
		t.Errorf("note-a.md should be untouched after a rejected rename: %v", statErr)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// rk note show: forward links and backlinks both surfaced (AC-3/AC-4).
// ─────────────────────────────────────────────────────────────────────────────

func TestNoteShow_ForwardAndBacklinks(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	outB, stderrB, errB := runNote(t, vault, "create", "Note B", "--json")
	if errB != nil {
		t.Fatalf("rk note create B: %v\nstderr: %s", errB, stderrB)
	}
	var resB map[string]any
	mustDecodeJSON(t, outB, &resB)
	bID, _ := resB["id"].(string)
	resetCLIFlags()

	outA, stderrA, errA := runNote(t, vault, "create", "Note A", "--body", "See [[note-b]].", "--json")
	if errA != nil {
		t.Fatalf("rk note create A: %v\nstderr: %s", errA, stderrA)
	}
	var resA map[string]any
	mustDecodeJSON(t, outA, &resA)
	aID, _ := resA["id"].(string)
	resetCLIFlags()

	buildIndex(t, vault)

	showOutB, showStderrB, showErrB := runNote(t, vault, "show", "note-b", "--json")
	if showErrB != nil {
		t.Fatalf("rk note show B: %v\nstderr: %s", showErrB, showStderrB)
	}
	var showResB struct {
		ID        string `json:"id"`
		Backlinks []struct {
			Src string `json:"src"`
			Rel string `json:"rel"`
		} `json:"backlinks"`
	}
	mustDecodeJSON(t, showOutB, &showResB)
	if showResB.ID != bID {
		t.Errorf("show B: ID = %q, want %q", showResB.ID, bID)
	}
	foundBacklink := false
	for _, bl := range showResB.Backlinks {
		if bl.Src == aID && bl.Rel == "references" {
			foundBacklink = true
		}
	}
	if !foundBacklink {
		t.Errorf("show B: backlinks = %+v, want a references backlink from A (%q)", showResB.Backlinks, aID)
	}
	resetCLIFlags()

	showOutA, showStderrA, showErrA := runNote(t, vault, "show", "note-a", "--json")
	if showErrA != nil {
		t.Fatalf("rk note show A: %v\nstderr: %s", showErrA, showStderrA)
	}
	var showResA struct {
		ForwardLinks []struct {
			Rel string `json:"rel"`
			Dst string `json:"dst"`
		} `json:"forward_links"`
	}
	mustDecodeJSON(t, showOutA, &showResA)
	foundForward := false
	for _, fl := range showResA.ForwardLinks {
		if fl.Dst == "note-b" && fl.Rel == "references" {
			foundForward = true
		}
	}
	if !foundForward {
		t.Errorf("show A: forward_links = %+v, want a references link to note-b", showResA.ForwardLinks)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// AC-7: create -> index -> query proven end to end (mirrors T4's own
// capture->index->query precedent).
// ─────────────────────────────────────────────────────────────────────────────

func TestNote_CreateIndexQuery_EndToEnd(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	outTarget, stderrTarget, errTarget := runNote(t, vault, "create", "Target Note", "--json")
	if errTarget != nil {
		t.Fatalf("rk note create target: %v\nstderr: %s", errTarget, stderrTarget)
	}
	var resTarget map[string]any
	mustDecodeJSON(t, outTarget, &resTarget)
	targetID, _ := resTarget["id"].(string)
	resetCLIFlags()

	outSource, stderrSource, errSource := runNote(t, vault, "create", "Source Note", "--body", "Linked: [[target-note]].", "--json")
	if errSource != nil {
		t.Fatalf("rk note create source: %v\nstderr: %s", errSource, stderrSource)
	}
	var resSource map[string]any
	mustDecodeJSON(t, outSource, &resSource)
	sourceID, _ := resSource["id"].(string)
	resetCLIFlags()

	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault, fmt.Sprintf(
		"SELECT src, dst_key FROM edges WHERE rel='references' AND src='%s'", sourceID))
	if err != nil {
		t.Fatalf("rk query: %v\nstderr: %s", err, stderr)
	}
	rows := parseNDJSONMaps(t, stdout)
	if len(rows) != 1 || rows[0]["dst_key"] != targetID {
		t.Fatalf("expected create->index->query to resolve the edge end to end, got %v (want dst_key=%q)", rows, targetID)
	}
	resetCLIFlags()

	showOut, showStderr, showErr := runNote(t, vault, "show", sourceID, "--json")
	if showErr != nil {
		t.Fatalf("rk note show: %v\nstderr: %s", showErr, showStderr)
	}
	var showRes map[string]any
	mustDecodeJSON(t, showOut, &showRes)
}
