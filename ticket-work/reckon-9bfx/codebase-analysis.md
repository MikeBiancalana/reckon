# Codebase analysis for reckon-9bfx

Ticket: "Review M2: ULID mint policy + rk adopt for id-less (Obsidian) files"
Worktree: `/home/chadd/repos/reckon/.worktrees/reckon-9bfx` (branch `reckon-9bfx`)

Done-when, from `bd show reckon-9bfx`:
1. the policy is recorded in the design doc
2. `rk adopt [path...]` stamps `id:` into id-less files via span-safe write (frontmatter insert), and is tested
3. mint-on-create is wired in the tool create paths

---

## 1. Where `rk adopt` would live / command registration / create paths

**Single binary, cobra subcommands — not a separate `rk-adopt` binary.**

- `cmd/rk/main.go` is the only binary (`cmd/` has exactly one subdir, `cmd/rk`).
- `internal/cli/dispatch.go` (`maybeDispatch`, `findExternal`) execs an external
  `rk-<verb>` off `PATH` **only as a fallback** when cobra's `RootCmd.Find` can't
  resolve the verb (see `dispatch.go:107-131`). All real v1 commands
  (`query`, `index`, `add`, `todo`) are registered as builtin cobra commands in
  `internal/cli/root.go`'s `init()` (lines ~120-134). `rk adopt` should follow
  this pattern: a new `internal/cli/adopt.go` (or a case in an existing file)
  defining `var adoptCmd = &cobra.Command{...}`, registered via
  `RootCmd.AddCommand(adoptCmd)` in `root.go`'s `init()` alongside `queryCmd`/`indexCmd`.

**Best template: `internal/cli/index.go` and `internal/cli/query.go`.** These are
the two *v1* (node/index-era) commands, as opposed to the *v0* legacy commands
(`note.go`, `task.go`, `log.go`, `notes.go`, `checklist.go`, `week.go`, `win.go`,
`today.go`, `schedule.go`, `review.go` — all backed by `internal/journal`/
`internal/service`/`internal/storage`, xid-keyed, DB-primary). `rk adopt` is a v1
tool operating on vault text + index, so it should mirror `index.go`'s shape, not
the legacy `AGENTS.md` "Add a New Subcommand" scaffold (which reflects the older
DB/service-global pattern):

```go
// internal/cli/index.go:16-21 (pattern to copy)
var indexCmd = &cobra.Command{
    Use:   "index",
    Short: "Build or rebuild the vault index",
    Annotations: map[string]string{"requiresDB": "false"},
    RunE: func(cmd *cobra.Command, args []string) error {
        mode, err := output.ModeFromFlags(jsonFlag, ndjsonFlag)
        cfg, err := config.LoadWithOverrides(vaultFlag, "")
        ix, err := index.Open(cfg)
        defer ix.Close()
        ...
        return output.New(cmd.OutOrStdout(), mode).Print(res)
    },
}
```
`rk adopt` needs `Annotations: {"requiresDB": "false"}` too (it's a vault-text
op, not a legacy-DB op) and should resolve the vault via
`config.LoadWithOverrides(vaultFlag, "")` exactly like `index.go:28`.

Flag-variable convention: `query.go` shows the idiom for command-local flags —
package-level vars, bound in `init()`, reset via a deferred `resetQueryFlags`-style
helper so `RootCmd`'s shared singleton stays clean across repeated `Execute()`
calls in tests (`query.go:22-28`, `65-75`).

**Tool create paths — there currently are none wired to `node.NewNode`/`Render`.**
This is the key finding for done-criterion 3:

```
grep -rn "node\.NewNode\|node\.Mint\|\.Render()" --include="*.go" . \
  | grep -v "_test.go\|internal/node/"
# => no results
```

`node.NewNode` (`internal/node/render.go:96-104`) and `node.Mint`
(`internal/node/ulid.go:20`) are called nowhere outside `internal/node` itself
(and its tests). The v1 create-path stubs that exist —
`addCmd` and `todoCmd` in `internal/cli/stubs.go:39-59` — are literal
`errNotImplemented` stubs ("v1 stub — not yet implemented"); they don't call
`node.NewNode` because they don't do anything yet. `rk note`/`rk log`/`rk task`
(T4/T5/T8 in the design's numbering) are still the legacy `internal/journal`
xid-based system (`journal/models.go` uses `xid.New().String()` at 7+ call
sites, `journal/task_parser.go`, `journal/task_service.go:267` — none of this
touches `internal/node`).

**Concretely, "mint-on-create is wired in the tool create paths" has nothing to
wire into on this branch today** — T4 (log)/T5 (todo)/T8 (note) haven't been
rebuilt on the node/index foundation yet (T8 = `reckon-ih5g`, which is one of
this ticket's *blockers*, i.e. scheduled to start after this ticket). The
foundation-review disposition (`docs/design/foundation-review-2026-06-24.md:60-62`)
already closed the primitive-level H1 work separately (`reckon-s0ix`, closed) —
`node.Render`/`node.NewNode` exist and are gate-tested
(`internal/node/render_test.go`). What's left for *this* ticket's "wired in
the tool create paths" is best read as: **the one artifact-creation path that
does exist on this branch is `rk adopt` itself** (it turns an id-less file into
one with a minted `id:`) — so make sure `rk adopt`'s own implementation goes
through `node.NewNode`/`node.Mint` correctly, and leave a forward note (or a
tracked follow-up) that T4/T5/T8, when built, must call `node.NewNode`/`Render`
for their create paths rather than re-deriving IDs. Worth confirming this
reading with the ticket owner/plan phase since it's the one part of "Done when"
that isn't literally actionable against current code.

---

## 2. "Span-safe write (frontmatter insert)" — no existing primitive; this is new work

`internal/node/node.go`'s `SetField` (`node.go:122-150`) is a span-local edit,
but it is **explicitly scoped to existing scalar keys only**:

```go
// SetField applies a span-local edit to an existing scalar frontmatter field...
// The key must already exist as a scalar; adding a missing key (inserting a
// frontmatter line) is a separate concern, out of scope for the byte-preservation
// keystone.
func (n *Node) SetField(key, value string) error {
    span, ok := n.fieldSpans[key]
    if !ok {
        return fmt.Errorf("SetField: no scalar span for %q (existing scalar keys only)", key)
    }
    ...
}
```

The **gating spike** this was productionized from documents the same
out-of-scope boundary even more explicitly —
`internal/spike/roundtrip/roundtrip.go:113-115`:
```go
// Spike scope: the key must already exist as a scalar. Adding a missing key is a
// separate concern (insert a line into the frontmatter block) and is out of
// scope for proving the byte-preservation bet.
```

So **"insert a missing key into frontmatter" has never been implemented** — not
in the spike, not in the productionized `node` package. `rk adopt` needs a new
primitive for this, e.g. `(*Node).InsertField(key, value string) error` (name
TBD in planning) that:

1. **Frontmatter block already exists, key absent** (the common Obsidian case —
   file has `---\n...\n---\n` with `type`/`tags`/etc. but no `id:`). Needs to
   splice a new `id: <ULID>\n` line into the block. `parseFrontmatter`
   (`node.go:158-197`) computes `closeAbs` (byte offset of the closing `---`
   line) locally but **does not store it on `Node`** — only per-field
   `fieldSpans` and the final `bodySpan.Start` survive. Implementing insert
   needs either (a) extending `Node`/`parseFrontmatter` to also record a
   frontmatter-block-end span, or (b) recomputing the insertion point by
   re-scanning Raw the same way `parseFrontmatter` does. Byte-preservation
   discipline requires this be a **surgical splice + re-parse**, exactly like
   `SetField` does at `node.go:139-149` (build `out` as prefix + new bytes +
   suffix, call `ParseAt(out, n.Loc)`, replace `*n`).
   - Ordering: `Render` emits `id` first among frontmatter keys
     (`render.go:37-39`), but `rk adopt` is editing a *hand-authored* file, not
     rendering fresh — inserting `id:` as the **first line inside the fence**
     (right after `---\n`) is the natural choice to match `Render`'s canonical
     order and keeps the diff minimal/predictable, but planning should confirm
     this against what feels least surprising for an Obsidian-edited file.
2. **No frontmatter block at all** (`raw` doesn't start with `---\n` —
   `parseFrontmatter` returns `bodyStart = 0` at `node.go:159-160`). Needs to
   **prepend** a whole new block: `"---\nid: <ULID>\n---\n"` + original bytes.
   This is a pure prefix-insert, no existing span to touch — simpler case but
   still needs a round-trip check (parse the result, confirm `Body` is
   unchanged and `ULID` reads back correctly).

Both cases must satisfy the same invariant the rest of `node` enforces:
`Serialize()`/re-parse round-trip stability, and must refuse on conflict-marker
files (`conflictMarkers` check already centralized in `ParseAt`,
`node.go:107-112`, so re-parsing after the splice gets this for free — but the
*pre*-edit read should probably also refuse outright rather than silently
overwrite a conflicted file).

There is no other span-tracking/frontmatter-editing utility anywhere else in
the repo (`internal/parser` is unrelated — it's link-extraction/security for the
*legacy* v0 notes system, `internal/parser/links.go`, not frontmatter).

---

## 3. "id-less files key on `file:<relpath>`" — already implemented (prior ticket), confirmed

This is exactly what the index does today. `internal/index/reconcile.go:254-264`:

```go
// nodeKey is the node's stable index identity: its inline ULID when present, else
// a path-derived surrogate (rename-stability is a ULID property by design).
func nodeKey(n *node.Node, rel string, seen map[string]int) string {
    if n.ULID != "" {
        return n.ULID
    }
    base := "file:" + rel
    seen[base]++
    if seen[base] == 1 {
        return base
    }
    return fmt.Sprintf("%s#%d", base, seen[base]-1)
}
```

`internal/index/schema.go:22-24` documents the same contract at the schema
level ("Identity: node_key is the inline ULID when present, else a surrogate
`file:<relpath>`"), and the public `nodes` view exposes this as `id`:
`schema.go:81-82`: `SELECT node_key AS id, ulid, type, time, author, body,
loc_file AS loc FROM _nodes;`. So an id-less file shows up in `rk query`
results with `id = "file:notes/foo.md"` and empty `ulid`. Renaming that file
changes its surrogate key (old key swept next reconcile, any inbound edges to
it go dangling) — this is the "not rename-stable until a tool touches it"
behavior the ticket description names, and it's already live/tested
(`internal/index/AGENTS.md`: "Rename is free: nodes are keyed by ULID... 
ULID-less files rename as delete+add"). `rk adopt` stamping an `id:` into the
file is precisely the "tool touches it" event that promotes a surrogate-keyed
node to ULID-keyed (rename-stable) on the next reconcile — no index-side
change needed for that promotion to work; it's a natural consequence of
`nodeKey` once the file has an `id:`.

---

## 4. Design doc — the policy is *already recorded*; nothing to add there

`docs/design/composable-redesign.md` — the canonical, actively-maintained
design doc (1523 lines; "Node ID scheme", "Canonical node (spec)", "Decision
log" etc.) — already has this as **Invariant #7** under "### Invariants"
(the numbered list following "### Fields" in the "Canonical node (spec)"
section), lines 501-508:

```
7. **Index never mutates truth files (ULID mint policy, 2026-06-24).** Tools that
   *create* nodes mint a ULID; the indexer never writes into source files, so a
   hand/Obsidian-authored id-less file keys on `file:<relpath>` and is not
   rename-stable until a tool touches it. An explicit, opt-in **`rk adopt`**
   stamps ULIDs into id-less files (a user-initiated mutation), giving
   Obsidian-authored files a path to first-class without the index ever writing
   truth. (Review finding M2.)
```

Invariant #6 (immediately above it, lines ~493-500) is the sibling H1 record
("Create renders through one primitive... `node.NewNode` mints the ULID on
create. (Closes review finding H1.)").

This was added by the **foundation review's disposition itself**
(`docs/design/foundation-review-2026-06-24.md:63-65`):
> "M2 — policy recorded (design invariant #7): index never mutates truth;
> create mints; `rk adopt` stamps id-less Obsidian files opt-in. Ticket
> `reckon-9bfx` (before T8/T9)."

**Implication for planning:** done-criterion 1 ("the policy is recorded in the
design doc") appears to already be satisfied by prior work (probably done in
the same session that wrote the disposition, before this ticket was filed to
track the *implementation*). Worth a quick sanity check in the plan step
(e.g., does invariant #7's wording need a follow-up note once `rk adopt`
actually ships, such as cross-referencing the new command's flags/behavior) but
there is no missing prose to author from scratch. `grep -rn "adopt"
docs/design/*.md` shows only this one substantive mention plus unrelated
"adopt Obsidian's wikilink surface" / "adopt hledger" prose elsewhere in the
same file (format-adoption, not `rk adopt` the command).

No other design doc mentions "H1"/"M2" or ULID minting — checked all of
`docs/design/*.md` and `docs/*.md`.

---

## 5. Test patterns to model `rk adopt` tests on

**CLI command test pattern** — `internal/cli/index_test.go:14-59`
(`TestIndexCommandRebuilds`) is the closest analog: builds a temp vault dir with
a hand-written `.md` file (including literal frontmatter with `id:`), sets
`RECKON_CACHE` env, drives the command through the real `RootCmd` (not a
bespoke harness):

```go
root := t.TempDir()
vault := filepath.Join(root, "vault")
cache := filepath.Join(root, "cache")
os.MkdirAll(vault, 0o755)
os.WriteFile(filepath.Join(vault, "a.md"), []byte(note), 0o644)
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
if err := RootCmd.Execute(); err != nil { t.Fatalf(...) }
```
This pattern — real `RootCmd.Execute()`, temp vault + temp cache, explicit
cleanup of the package-global flag vars in `t.Cleanup` — is what `rk adopt`'s
CLI-level tests should copy. For `rk adopt` specifically, the test would then
`os.ReadFile` the adopted file afterward and assert the `id:` line was
inserted and everything else byte-identical (or re-`node.Parse` it and check
`.ULID != ""` plus body/other-props unchanged).

**External-dispatch/PATH-lookup pattern** (only relevant if `rk adopt` ever
became an external binary, which it should NOT — see §1) is in
`internal/cli/dispatch_test.go:14-30` (`TestFindExternal_Found`) — not needed
here but confirms builtins are the right shape.

**`node` package unit-test pattern** — `internal/node/render_test.go`,
specifically `TestCreateRoundTrip` (lines 33-79) and `TestNewNodeMintsAndRoundTrips`
(line 81) — is the template for testing a new `InsertField`-style primitive:
build/parse a `Node`, apply the edit, re-`Parse` the output, assert (a) the
targeted field changed, (b) every other typed field/prop/link/body is
byte-identical via a `sameView`-style helper (`render_test.go:11-28`), and (c)
`Serialize()`/`Render()` round-trip stability post-edit
(`bytes.Equal(parsed.Serialize(), rendered)`). Also directly relevant:
`internal/node/node_test.go:202-230` — `TestSpanLocalEditIsSurgical` (asserts
`SetField` touches *only* the target span, nothing else byte-shifts) and
`TestSetFieldMissingKey` (asserts the *current* missing-key behavior, i.e. an
error) — the new insert primitive is precisely filling the gap
`TestSetFieldMissingKey` currently documents as out of scope, so that test's
docstring ("spike scope contract") may need a companion `TestInsertField...`
test rather than a change to `SetField` itself.

**ULID minting tests** — `internal/node/ulid_test.go` (format, time-sortable,
concurrent-safe, monotonic) is unaffected by this ticket; `rk adopt` should
just call `node.Mint()` (or build via `node.NewNode`-adjacent helpers) and
inherit these guarantees rather than re-implementing ID generation.

---

## 6. Pitfalls from `docs/REVIEW_PATTERNS.md`

Generic (apply to any new CLI command):
- **No `os.Exit` in library/command code** — return wrapped errors, let
  `main.go`'s single `os.Exit(1)` handle it (`REVIEW_PATTERNS.md:175-201`,
  "very common" frequency historically).
- **Respect `--quiet`** — any success/status line `rk adopt` prints (e.g. "adopted
  3 files, skipped 1 already-ULID'd") must be gated on `!quietFlag`
  (`REVIEW_PATTERNS.md:204-228`); structured `--json`/`--ndjson` output should
  still always emit (see `index.go:53-58`'s comment: "In pretty mode --quiet
  suppresses the status line; structured output is the requested data, so
  --json/--ndjson always emit").
- **Wrap every error with context** (`fmt.Errorf("adopt: ...: %w", err)`) —
  the single most frequent finding historically (`REVIEW_PATTERNS.md:19-49`).
- **Validate args via cobra `Args:`**, don't index into `args[0]` unchecked
  (`REVIEW_PATTERNS.md:403-413`).
- **Tests that don't test what they claim** / **missing edge-case tests**
  (`REVIEW_PATTERNS.md:232-296`) — worth deliberately covering: file with no
  frontmatter at all, file with frontmatter but no closing fence (malformed —
  `parseFrontmatter` returns `bodyStart=0` i.e. treats whole file as body, so
  `rk adopt` must not "insert into" something that isn't actually a
  frontmatter block), file that already has `id:` (should be a no-op / skipped,
  not double-inserted), and a file containing conflict markers (must refuse,
  matching `ParseAt`'s existing refusal at `node.go:107-112`).

Nothing in `REVIEW_PATTERNS.md` documents an existing "atomic file write"
convention (no `os.Rename`-after-tempfile pattern found anywhere in the repo —
`internal/storage/filesystem.go`, `internal/cli/notes.go`,
`internal/migrate/log_files.go` all call `os.WriteFile` directly). `rk adopt`
writing directly to vault files with `os.WriteFile` would match existing
precedent, but since this ticket is explicitly about *mutating hand-authored
truth files* (a first for this codebase — the index's whole design principle
is "never mutates truth files"), planning should decide deliberately whether
`rk adopt` needs write-safety beyond existing precedent (e.g. write-to-temp +
rename for crash-safety, since a half-written frontmatter block is worse than
a half-written legacy DB row). This isn't a documented pitfall but is a gap
worth flagging given the stakes are different from every other file-write in
the codebase.

Also relevant but not a "pitfall" per se: `internal/index/lock_unix.go`'s
reconcile-writer flock (`index.lock` in the cache dir) guards
`Rebuild`/`Reconcile` against each other, not against vault-file writers.
`rk adopt` writing to a vault file while `rk index`/a lazy reconcile is
mid-walk on another process is a real (if narrow) race; nothing in the index
package currently coordinates with external vault-file writers, so this is
either accepted as out-of-scope (git/Syncthing already have to tolerate
concurrent external edits) or worth a one-line note in the plan.

---

## Summary of concrete file/line references

| Concern | File | Lines |
|---|---|---|
| Command registration | `internal/cli/root.go` | `init()` ~120-134 |
| External-dispatch fallback (NOT the model to use) | `internal/cli/dispatch.go` | 107-131 |
| v1 command pattern to copy | `internal/cli/index.go` | 16-59 |
| v1 command pattern (flags/reset idiom) | `internal/cli/query.go` | 22-75 |
| v1 stub commands (no create path wired) | `internal/cli/stubs.go` | 39-59 |
| Legacy xid-based create paths (NOT node-based) | `internal/journal/models.go` | 47,57,80,99,122,143,164,181 |
| `node.NewNode` | `internal/node/render.go` | 96-104 |
| `node.Mint` / `MintAt` | `internal/node/ulid.go` | 20, 23-32 |
| `Node.Render` | `internal/node/render.go` | 31-95 |
| `Node.SetField` (existing-key-only span edit) | `internal/node/node.go` | 122-150 |
| `parseFrontmatter` (closing-fence offset, not stored) | `internal/node/node.go` | 158-197 |
| Spike's explicit "insert missing key: out of scope" note | `internal/spike/roundtrip/roundtrip.go` | 108-115 |
| `nodeKey` (`file:<relpath>` surrogate) | `internal/index/reconcile.go` | 254-264 |
| Schema doc of `node_key`/`id` identity contract | `internal/index/schema.go` | 22-24, 81-82 |
| Design invariant #7 (policy already recorded) | `docs/design/composable-redesign.md` | 501-508 |
| Design invariant #6 (H1, sibling) | `docs/design/composable-redesign.md` | 493-500 |
| Disposition entry naming this ticket | `docs/design/foundation-review-2026-06-24.md` | 60-65 |
| CLI test pattern to copy | `internal/cli/index_test.go` | 14-59 |
| `node` package create/round-trip test pattern | `internal/node/render_test.go` | 11-137 |
| `SetField` surgical-edit + missing-key tests | `internal/node/node_test.go` | 202-230 |
| Vault walk / ignore-glob conventions | `internal/index/reconcile.go` | 133-162, 599-616 |
| Config vault/cache resolution | `internal/config/config.go` | 108-163 |
| `--quiet` / error-wrapping / os.Exit pitfalls | `docs/REVIEW_PATTERNS.md` | 19-49, 173-228, 403-413 |
