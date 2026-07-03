# Implementation Plan: reckon-9bfx — ULID mint policy + `rk adopt` for id-less files

## Summary of approach

Three deliverables in the "Done when" clause, resolved as follows:

1. **Policy recorded in the design doc** — already satisfied. Invariant #7 (`docs/design/composable-redesign.md:501-508`) states the exact policy, and invariant #6 (`:495-500`) records "create mints via `node.NewNode`". This ticket's obligation is to *verify* the text is present and still accurate against shipped behavior (`nodeKey` in `internal/index/reconcile.go:254-264` keys id-less files on `file:<relpath>`; confirmed). Optionally append a one-line note to #7 that `rk adopt` is now implemented. No prose to author from scratch.

2. **`rk adopt [path...]`** — the core new deliverable. A new builtin v1 Cobra command (mirroring `internal/cli/index.go`) that walks the given paths, and for each id-less markdown file stamps a freshly minted ULID into `id:` via a **new `node.InsertField` primitive** (span-safe frontmatter insert). It never opens the index; the index promotes the file from surrogate key to ULID key on its next normal reconcile.

3. **"Mint-on-create is wired in the tool create paths"** — resolved as **option (a), with the one nuance that makes it non-vacuous** (see below).

### Resolution of the mint-on-create scoping tension

I adopt **(a)**, justified concretely rather than as a hand-wave:

- The shared mint-on-create primitive `node.NewNode` (mints via `node.Mint`) + `node.Render` **already exists and is already gate-tested** by the closed H1 ticket reckon-s0ix (`internal/node/render_test.go`: `TestNewNodeMintsAndRoundTrips`, `TestCreateRoundTrip`). It mints correctly whenever called.
- There are **no v1 node-based create paths in the tree to wire into**: `addCmd`/`todoCmd` are `errNotImplemented` stubs (`internal/cli/stubs.go:39-59`); `rk log`/`rk note`/`rk task` are still the legacy xid-based `internal/journal` system, which never touches `internal/node`. A repo grep confirms `node.NewNode`/`.Render()` are called nowhere outside `internal/node` and its tests.
- Wiring into `rk note` **cannot** be this ticket's job: reckon-ih5g (rk note) literally *depends on* reckon-9bfx — that's a dependency cycle if we tried. Likewise reckon-qiua (rk todo) and reckon-uv09 (rk log) are separate open tickets that will build those commands.
- Therefore the only node-authoring path this ticket actually introduces is **`rk adopt` itself**, and the meaningful, in-tree obligation for done-criterion 3 is: **`rk adopt`'s own id-stamping must go through the same mint function `node.NewNode` uses (`node.Mint`)**, so adopted files are format-indistinguishable from tool-created ones.

Concrete actions for (3):
- `rk adopt` mints via `node.Mint()` (never a re-implemented/truncated ULID).
- Cite the existing `render_test.go` gates as the regression proof that `NewNode`/`Render` is the one mint-on-create primitive; optionally add a lightweight guard test asserting no *second* "build frontmatter from scratch" path exists besides `node.Render` and `adopt`'s insert (AC §4's "no competing create-path" scenario).
- **Explicitly descope** wiring into `rk add`/`rk todo`/`rk log`/`rk note` as a deliberate decision (recorded here and in the plan), since those are downstream tickets whose AC will require them to call `node.NewNode`/`Render`. Invariants #6/#7 already record that contract.

This is the resolution the two prior research agents converged on; ratifying it and pinning the non-vacuous part (adopt mints via `node.Mint`) so the criterion is genuinely met rather than dismissed.

---

## Files to create / modify

### Create

- **`internal/node/insert.go`** — the new span-safe insert primitive `(*Node) InsertField(key, value string) error`. Lives in `internal/node` (not the CLI) because it needs `n.bodySpan.Start` and the `---\n` fence geometry, both unexported; it mirrors `SetField`'s splice-then-reparse discipline and the package's encapsulation invariant ("nothing outside `node` touches `fieldSpans`/`bodySpan`"). Kept in a sibling file (rather than appended to `node.go`) for review isolation.
- **`internal/node/insert_test.go`** — unit tests for `InsertField` (existing-block insert, no-block prepend, unterminated-fence refusal, key-already-present refusal, surgical byte-preservation, round-trip stability). Models on `render_test.go`'s `sameView` helper and `node_test.go`'s `TestSpanLocalEditIsSurgical`.
- **`internal/cli/adopt.go`** — `adoptCmd` (Cobra command, `Annotations{"requiresDB":"false"}`, `SilenceUsage:true`, `Args: cobra.MinimumNArgs(1)`), the RunE pipeline (path resolution + vault containment, directory walk, per-file processing), the `adoptResult` structured-output type with a `Pretty()` method, and a local `writeFileAtomic` helper. Modeled on `internal/cli/index.go` (config load, `output.New`, `--quiet` gating).
- **`internal/cli/adopt_test.go`** — CLI-level tests driving the real `RootCmd.Execute()` with a temp vault + temp cache, modeled on `internal/cli/index_test.go:14-59`.

### Modify

- **`internal/cli/root.go`** — register the command: add `RootCmd.AddCommand(adoptCmd)` in `init()` alongside `queryCmd`/`indexCmd`.
- **`internal/index/reconcile.go`** — export the ignore predicates so `rk adopt`'s directory walk uses the *exact* rules the index uses. Add thin exported wrappers `func Indexable(name string) bool { return indexable(name) }` and `func ShouldSkipDir(name string) bool { return shouldSkipDir(name) }` (leaves existing lowercase call sites untouched). Alternative considered: replicate the two predicates inside `adopt.go` — rejected as a drift risk.
- **`docs/design/composable-redesign.md`** *(optional, low priority)* — verify invariant #7 wording; optionally append a clause noting `rk adopt` is now the implemented command.

### Explicitly NOT modified

- `internal/node/node.go` `SetField` and its `TestSetFieldMissingKey` — unchanged; `SetField`'s "existing scalar keys only" contract stays intact. `InsertField` fills the adjacent gap, it does not weaken `SetField`.
- `internal/index/reconcile.go` `nodeKey`/schema — no keying change (surrogate→ULID promotion already works once the file has an `id:`).
- The legacy v0 commands and `internal/journal`.

---

## Design decisions (with alternatives)

### D1 — Where the span-safe insert lives: method on `*Node`, not a free function on raw bytes

`(*Node) InsertField(key, value string) error`. It needs `n.bodySpan.Start` (unexported; set >0 only when a *terminated* frontmatter block was recognized) to distinguish "block exists" from "no block", and it mirrors `SetField`'s splice + `ParseAt` re-parse so the typed view/spans stay consistent. A free function on raw bytes would duplicate `parseFrontmatter`'s fence scan and couldn't update a `Node`'s view — rejected.

### D2 — Detection trichotomy for "where to insert"

```go
func (n *Node) InsertField(key, value string) error {
    if _, exists := n.fieldSpans[key]; exists {
        return fmt.Errorf("InsertField: key %q already present (use SetField)", key)
    }
    raw := n.Raw
    var out []byte
    switch {
    case n.bodySpan.Start > 0: // terminated frontmatter block exists
        line := key + ": " + value + "\n"
        out = append(out, raw[:4]...)   // opening "---\n"
        out = append(out, line...)      // id inserted as FIRST line inside fence
        out = append(out, raw[4:]...)
    case bytes.HasPrefix(raw, []byte("---\n")): // unterminated
        return fmt.Errorf("InsertField: unterminated frontmatter block; refusing to insert")
    default: // no frontmatter block at all
        out = append(out, ("---\n" + key + ": " + value + "\n---\n")...)
        out = append(out, raw...)
    }
    reparsed, err := ParseAt(out, n.Loc)
    if err != nil {
        return fmt.Errorf("InsertField: re-parse after splice failed: %w", err)
    }
    *n = *reparsed
    return nil
}
```

- **`id:` inserted as the first line inside the fence** — matches `Render`'s canonical key order (`render.go:36-38`), minimal predictable diff.
- **"No frontmatter block" detected via `bodySpan.Start`**.
- **Unterminated frontmatter is refused**, not silently treated as "no block" — prepending a fresh block would nest the user's attempted frontmatter into the body.

### D3 — Mint in the command, insert is generic

`rk adopt` computes `id := node.Mint()` then calls `n.InsertField("id", id)`. Keeps `InsertField` format-generic while the command owns mint policy, using the *same* `node.Mint` that `node.NewNode` uses.

### D4 — Write strategy: atomic temp-file + rename (deliberately introducing the convention here)

No existing atomic-write convention in the codebase (everyone calls `os.WriteFile` directly). This is the first tool to mutate hand-authored truth files, where a half-written frontmatter block corrupts a real user note. Decision: `writeFileAtomic(path, data)` — `os.CreateTemp(dir, ".adopt-*.tmp")` in the same directory, write, chmod to original file's mode, close, `os.Rename` over original, `os.Remove(tmp)` on any error.

### D5 — Idempotency / already-id'd = no-op (not error)

A file with a non-empty `id:` is skipped as byte-identical no-op, reported as `skipped`. Safe re-runs, mixed-batch friendliness. No `--force`/re-mint mode (out of scope; would break inbound links).

### D6 — Error handling is per-file; batch continues; non-zero exit if any failed

Each path processed independently; file-level errors collected into `adoptResult.Errored`, don't stop others. RunE emits structured result, then returns summarizing error so exit code reflects failure. `SilenceUsage: true`.

### D7 — Path policy: vault-scoped + `.md`-only for explicit files + index ignore-rules for directories

- **Vault containment (reject out-of-vault):** resolve each arg to absolute, `filepath.Rel(cfg.VaultDir, abs)`; reject if `..`-prefixed. Mirrors cache-inside-vault guard in `config.LoadWithOverrides:151-157`.
- **Explicit file arg** must end in `.md`, else error.
- **Directory arg** → `filepath.WalkDir` honoring `index.ShouldSkipDir`/`index.Indexable` (same rules as `reconcile.go:594-616`).

### D8 — CRLF files: refuse (skip with error)

CRLF files parse as body-only today (known M4 gap, reckon-vj55). Command pre-checks `bytes.Contains(raw, "\r\n")` and refuses with explicit error, avoiding mixed line endings.

### D9 — Output shape

`adoptResult{Adopted []{Path,ULID}, Skipped []{Path,Reason}, Errored []{Path,Error}}` printed via `output.New`. Follows `index.go`'s rule: `--quiet` suppresses Pretty status line; `--json`/`--ndjson` always emit. `requiresDB=false`, resolves vault via `config.LoadWithOverrides`, never `index.Open`.

---

## Test scenarios (traceable to AC document)

### `internal/node/insert_test.go`
- Insert `id` into existing block → first line inside fence; all other bytes identical; reparse yields expected ULID; Serialize round-trip stable.
- Insert into file with no frontmatter → result begins `---\nid: <v>\n---\n` + original bytes.
- Refuses unterminated `---\n` block.
- Refuses when key already present.
- Body starting with `---` → prepended block still parses correctly.

### `internal/cli/adopt_test.go`
- Stamps missing id into existing block; other bytes unchanged.
- Inserts whole block into file with none.
- No-op on already-id'd file: byte-identical, exit 0, reported skipped.
- Idempotent: adopt twice → second run byte-identical, same ULID.
- Mixed batch (some id-less, some id'd): id-less get distinct ULIDs, others untouched.
- Directory arg walks recursively; `.obsidian/`, `.git/`, dotfiles, non-md files, already-id'd files left untouched.
- Explicit non-`.md` file → error, unmodified.
- Conflict-marker file → per-file error, unmodified.
- CRLF file → per-file error (reckon-vj55), unmodified.
- Unterminated-frontmatter file → per-file error, unmodified.
- Adopt never touches index: no cache/db created; subsequent `rk index` reflects new ULID.
- Minted id is valid ULID indistinguishable from `node.Mint` output.
- Path outside vault → error.
- `--json`/`--ndjson`/`--quiet` output behavior.

### Regression / guard (mint-on-create — resolution of criterion 3)
- Cite existing `TestNewNodeMintsAndRoundTrips` and `TestCreateRoundTrip` (`internal/node/render_test.go`) as proof `node.NewNode`/`Render` mints correctly.
- *(Optional)* guard test asserting only two "author frontmatter from scratch" paths are `node.Render` and adopt's insert.

---

## Implementation steps (sequencing)

1. Add exported `Indexable`/`ShouldSkipDir` wrappers in `internal/index/reconcile.go`.
2. Implement `node.InsertField` (`internal/node/insert.go`) + `insert_test.go`.
3. Implement `adoptCmd` (`internal/cli/adopt.go`).
4. Register in `root.go` `init()`.
5. Write `adopt_test.go` covering all scenarios above.
6. Verify invariant #7 in design doc; optional one-line note.
7. Cite `render_test.go` gates for criterion 3; optionally add guard test.

---

## Known risks / ambiguities

- **Concurrency:** two `rk adopt` processes racing on the same id-less file could each mint (two ULIDs; atomic rename makes one win — no corruption, redundant mint). Full cross-process locking out of scope.
- **adopt vs. concurrent reconcile:** narrow race, index already tolerates external edits (git/Syncthing).
- **CRLF / unterminated-frontmatter are policy refusals**, not graceful handling — belongs to reckon-vj55 (parser scope) to fix properly.
- **Exporting index predicates** couples `internal/cli` → `internal/index` further (already imports it, so no new dependency direction).
- **`id:` first-line ordering** could cosmetically reorder a block an Obsidian plugin expects in specific order — low risk, matches canonical `Render` order.
- **Criterion-3 interpretation** is the one non-literal part of "Done when"; resolution is explicit here (not a silent drop), pinning the real obligation: adopt mints via `node.Mint`.

---

## Critical files for implementation

- `internal/node/node.go` — SetField splice pattern + `bodySpan`/`parseFrontmatter` geometry
- `internal/cli/index.go` — v1 command template
- `internal/cli/root.go` — command registration
- `internal/index/reconcile.go` — ignore predicates to export; `nodeKey` surrogate-key contract
- `internal/node/render.go` — `node.NewNode`/`Render`/`Mint`
