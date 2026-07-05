# Implementation Plan: v1-T5 — `rk todo` durable + ephemeral (reckon-qiua)

## Summary of approach

Add a real `rk todo` command family (`add`, `list`, `done`) in a new `internal/cli/todo.go`, graduating the existing `todoCmd` stub the same mechanical way `queryCmd`/`indexCmd` graduated. The command is a truth-file tool built on `internal/node`, structurally cloned from `internal/cli/adopt.go` (same config resolution, same `output.Writer` result pattern, same `writeFileAtomic`, same `Annotations{"requiresDB":"false"}`), with `list` reading through the `internal/index` SQLite views (design principle: per-tool list commands are "thin sugar over `rk query`").

Two on-disk shapes, both handled by the existing `node.MarkdownParser{}` (one node per file), so **no change to `index.Open` / no custom `node.Parser` is required**:

- **Durable todo** — `todos/<ULID>.md`, one file = one node, `type: todo`. Created via the `NewNode`/`Render`/`Parse` recipe; completed via a span-local `SetField("state","done")`. Dependency authored as a literal `depends-on: "[[target]]"` frontmatter key, so the generic parser emits a `depends-on` edge with zero custom code.
- **Ephemeral todo** — a single shared container file `todos/inbox.md`, `type: todo-ephemeral`. The **container** is a node (addressable, indexable, queryable by type); the **items** are plain markdown task-list lines (`- [ ] text`) in the body and get **no ULID / no node identity**, honoring "ephemeral = no stable address." Add appends a checkbox line; `done` flips `- [ ]`→`- [x]` via a one-byte surgical splice.

`add` is the first verb in the repo that creates brand-new files, so it owns new `os.MkdirAll(todos/)` and no-clobber logic that `adopt.go` never needed. `list` is the first CLI caller of `Index.Reconcile()`, wired so a fresh `add` is visible to the very next `list` with no manual `rk index` step.

## Files to modify

- **`internal/cli/todo.go`** — NEW. The real `var todoCmd` plus `todoAddCmd`, `todoListCmd`, `todoDoneCmd` subcommands, their flag registration, the three result structs (`todoAddResult`, `todoListResult`, `todoDoneResult`) with `Pretty()` methods, an `resolveAuthor()` helper, and the durable/ephemeral read-write helpers. Reuses the package-private `writeFileAtomic` from `adopt.go` verbatim (same package, zero duplication).
- **`internal/cli/stubs.go`** — EDIT. Delete the `todoCmd` stub block (lines 50–59). Leaving it produces a duplicate `var todoCmd` compile error once `todo.go` defines the real one. `errNotImplemented` (line 62) stays — `addCmd` still uses it. `tuiCmd`/`addCmd` untouched.
- **`internal/cli/root.go`** — NO EDIT expected. `RootCmd.AddCommand(todoCmd)` at line 132 already exists and resolves to the new definition once the stub is gone. (Confirm build; the subcommand `AddCommand`s live inside `todo.go`'s `init()`, like `query.go`.)
- **`internal/cli/todo_test.go`** — NEW (written first, TDD-red). Reuses `setupQueryVault`, `writeTestNode`, `resetCLIFlags`, `buildIndex` from `query_test.go`; adds a `runTodo(t, vault, args...) (stdout, stderr string, err error)` helper mirroring `runQuery`. Pins the result-struct JSON shapes in a header comment exactly as `adopt_test.go` does.

No changes to `internal/node`, `internal/index`, `internal/config`, or `internal/output`.

## Design decisions

Each resolves an AC-doc `[OPEN]` (or a prompt-mandated judgment call); alternatives and rationale below.

### D1 — Durable on-disk shape: `todos/<ULID>.md`, `type: todo`

Layout (frontmatter key order is fixed by `render.go`: id, type, time, author, then **props sorted alphabetically**, then typed links sorted by rel):

```
---
id: 01J...
type: todo
time: 2026-07-04T12:00:00Z
author: mike
deadline: 2026-07-15
scheduled: 2026-07-10
state: open
depends-on: "[[01JA...]]"
---
Buy milk
```

- Filename = the node's own ULID (`todos/<ULID>.md`) — rename-stable identity is a ULID property; matches the design's "filename = ULID" note and the scope table's "1 file per todo."
- `state`/`scheduled`/`deadline` are plain scalar props (omitted entirely when unset — `render.go` only emits props present in the map, so no empty-string keys, satisfying EC-3). `state: open` is always present on create.
- Created strictly via the documented recipe: `NewNode("todo",author,body)` → set `Time`, `Props`, `Links` → `Render()` → `Parse(raw)` → write `reparsed.Serialize()`. The `Parse` step is mandatory before the file is ever `SetField`-edited (render.go doc contract).
- **Alternative rejected:** minting a stable per-item ID some other way, or embedding type in the ID — the design explicitly says "do NOT bake type into the ID." Filename-as-ULID is the established convention.

### D2 — Ephemeral on-disk shape: single container `todos/inbox.md`, `type: todo-ephemeral`, items are checkbox body lines

```
---
id: 01J...
type: todo-ephemeral
time: 2026-07-04T12:00:00Z
author: mike
---
# Inbox

- [ ] call dentist
- [x] buy milk
```

- One shared container (not per-day). The container is a node with its own ULID/type (the "file/day only" addressable atom); individual `- [ ] text` lines are **not** nodes and get no ULID — this is the literal encoding of "individual tasks NOT addressed / ephemeral = no ID by design."
- First `add --ephemeral` builds the file via `NewNode("todo-ephemeral",author,"# Inbox\n\n- [ ] <text>\n")` → `Render` → `Parse` → write. Subsequent `add --ephemeral` reads the file and appends `- [ ] <text>\n` at EOF (EOF append only touches the body, never the frontmatter; the file still round-trips as plain markdown).
- `scheduled`/`deadline`/`depends` are **not supported for ephemeral** in this ticket (durable-only), per AC-doc §2.1 — deferred to `rk today`/recurrence. `add --ephemeral` with those flags is a usage error.
- **Alternative rejected:** per-day `todos/inbox-YYYY-MM-DD.md`. It multiplies containers and makes `done --ephemeral <n>` ambiguous about *which* container to index into, for no benefit this ticket needs; day-bucketing is `rk today` territory. Single default container keeps positional addressing unambiguous.
- **Alternative rejected:** giving each item its own ULID via a group-file parser (the T4 log-tool model). That would make every item addressable — directly contradicting the design. `SplitEntries`/`ReplaceEntryBody` are ATX-`## `-header primitives and don't fit checkbox lines anyway; we use a checkbox-line splice instead (D5).

### D3 — Discriminator = `type` value; queryable at both index and porcelain layers

Durable = `type: todo`; ephemeral container = `type: todo-ephemeral`. Requires zero schema/parser change — directly expressible against the existing public `nodes` view (`WHERE type='todo'` vs `type='todo-ephemeral'`), satisfying AC-6. `rk todo list --durable`/`--ephemeral` are the porcelain filters over the same distinction.

- **Alternative rejected:** a `durable: false` boolean prop on one shared `type: todo`. A distinct `type` is cleaner to query, needs no prop convention, and reads naturally in `rk query`. The prompt and AC-doc both lean `type`.

### D4 — `list` reads through the index (Open + Reconcile), not a filesystem walk

`todoListCmd` does `cfg := LoadWithOverrides(vaultFlag,"")` → `ix := index.Open(cfg)` → `ix.Reconcile()` → SQL over the public views, then closes. This is the first CLI wiring of `Index.Reconcile()`.

- Justification against the design principle ("per-tool list commands are thin sugar over `rk query`"): querying the same index engine is exactly "thin sugar," where a raw filesystem walk would fork a second, drifting read path.
- **Freshness (hard requirement, AC-1/TS-2.1):** unlike raw `rk query` (a power-user surface that errors with "run `rk index` first" when the DB is absent), `rk todo list` is porcelain and must satisfy create→list with no manual index step. `index.Open` auto-rebuilds a missing/stale index (via `ensureSchema`→`Rebuild`); the explicit `Reconcile()` then picks up any file written since the last pass. So `add` (a pure truth-file write) followed immediately by `list` reflects the new todo. This deliberately opens the index **read-write** (Reconcile mutates), diverging from `rk query`'s read-only open — a conscious porcelain-vs-power-user distinction.
- Durable rows: `type='todo'`; pull `state`/`scheduled`/`deadline` from `node_props` and `depends-on` from `edges` for each id. Ephemeral: `type='todo-ephemeral'`; read the container `body` and split it into checkbox lines, emitting one list record per line (the container is one node, so its whole body is already indexed — no extra file read).
- Default = open only (durable `state='open'`; ephemeral unchecked). `--all` includes done/checked; `--state <s>` filters durable exact state; `--durable`/`--ephemeral` restrict kind.
- Missing vault dir (TS-2.5) / empty vault (TS-2.4): `reconcileTx`'s `WalkDir` already tolerates `os.IsNotExist` → empty index → zero rows, exit 0. `Open` still succeeds (it only MkdirAll's the cache dir, never the vault).
- **Alternative considered:** pure filesystem glob of `todos/*.md` (sidesteps freshness entirely). Rejected as the primary path because it ignores the index the design says list should ride on; but it is the natural fallback if index coupling proves fragile (noted in risks).

### D5 — `done` targeting: durable via `SetField`, ephemeral via 1-based line index + checkbox flip; filesystem-only (never touches the index)

`done` is a truth-file mutation modeled on `adopt.go` (Annotations `requiresDB=false`; never opens the index — the index picks up the changed `state` on the next `list`/`index` via hash reconcile).

- **Durable:** `rk todo done <ref>`. Resolution: try `todos/<ref>.md` directly (the ULID fast-path, since `add` always names by ULID); if absent, walk `todos/*.md`, parse each, match `n.ULID==ref` or `ref ∈ n.Aliases` (alias support, TS-3.2). On match: if `n.Props["state"]=="done"` → idempotent **skipped** result, **no rewrite** (EC-1/TS-3.3); else `n.SetField("state","done")` (the field already exists as a scalar from create, so `SetField`, not `InsertField` — mirrors `adopt.go`'s `SetField("id",id)`), then `writeFileAtomic(reparsed.Serialize())`. Non-existent ref → error, non-zero exit (TS-3.4), distinct from the skip case. Read path applies the same CRLF guard as `adopt.go` (refuse, cite reckon-vj55).
- **Ephemeral:** `rk todo done --ephemeral <n>`, `<n>` a **1-based index over all checkbox lines in file order** in `todos/inbox.md`. `list --ephemeral` surfaces this stable `index` per item so the user knows what to pass. Locate line `n`; if already `- [x]` → idempotent skip; else flip the single space inside `[ ]` to `x` (a one-byte surgical replacement — `[ ]` and `[x]` are both 3 bytes, so every other byte, including sibling item lines, is byte-identical), then `writeFileAtomic`. Index out of range / missing container → error, non-zero exit.
  - Choosing "index over all lines in file order" (not "nth unchecked") is a refinement of the AC-doc suggestion: it's positionally stable as items complete and maps 1:1 to what `list` prints. The handle is transient (valid only for the current view), never stored, and not resolvable by the index — satisfying "no stable address."
- **Alternative rejected for ephemeral:** substring/text match on item body. Ambiguous with duplicate texts and awkward to script; positional index is deterministic and pairs with `list`'s output.

### D6 — `depends` authored literally as `depends-on:`; dangling targets accepted

`--depends <target>` writes `n.Links = []node.Link{{Rel:"depends-on", To:target}}`, which `render.go` emits as `depends-on: "[[target]]"`. On parse, `deriveView` routes it to `Links` with `Rel` = the literal key `"depends-on"` (the generic core never renames keys; hyphen is legal in the key regex), so the index gets a `depends-on` edge with **zero custom parser code**. The ref-valued prop routes to `edges`, never `node_props` (TS-5.2). **No existence validation at write time** — per the design's dangling-link semantics, an unresolvable target is written verbatim and becomes a dangling edge (`dst=target`, `dst_key IS NULL` after `resolveEdges`), auto-resolving on a later reindex once the target exists (EC-2/TS-5.3/TS-5.4). Single dependency only (frontmatter is `map[string]string`, one value per key) — multiple deps are explicitly out of scope.

- **Alternative rejected:** key `depends:` + a thin custom `Parser` that renames `depends`→`depends-on` post-parse. That reintroduces the single-global-`MarkdownParser` friction (D7) for no gain; authoring the canonical key directly is strictly simpler.

### D7 — `index.Open`'s single global `MarkdownParser{}` is sufficient; do not touch it

Both shapes are one-node-per-file (durable = the todo; ephemeral = the container). Nothing needs group-splitting into multiple `_nodes` rows, so the hardcoded `node.MarkdownParser{}` in `index.Open` already handles them. This ticket does **not** modify `index.Open`/`OpenWithParser`. (The single-global-parser limitation remains a real, shared blocker for the still-open T4 log tool, but is genuinely a non-issue here — flag it there, don't work around it here.)

### D8 — Author source: `--author` flag → `$RECKON_AUTHOR` → `$USER` → `"local"`

`NewNode` requires an author string and no existing helper derives one. Add a small `resolveAuthor(flag string) string` with that precedence, always returning non-empty (so `author:` is always emitted for provenance). `--author` gives tests a deterministic value; the env/USER/local chain gives a sane default the ticket doesn't otherwise specify.

- **Alternative rejected:** git-config lookup — heavier, adds a dependency, and the ticket never mentions provenance; the env/OS-user fallback is adequate for v1.

### D9 — Directory creation + no-clobber on `add`

Before the durable write, `os.MkdirAll(filepath.Join(cfg.VaultDir,"todos"),0o755)` (creates the vault root transitively) — `config.LoadWithOverrides` is pure and `writeFileAtomic` doesn't MkdirAll, so `add` (first verb to create new files) owns this (EC-6/TS-1.5). Then **no-clobber:** `os.Stat(path)`; if it exists, error rather than overwrite (EC-5) — `writeFileAtomic`'s `os.Rename` would otherwise silently clobber, and it's only ever been used for in-place edits of known-existing files. Ephemeral is exempt from no-clobber (an existing container is the expected append target).

### D10 — Flags, output, and flag-reset hygiene

- `add <body>`: `--ephemeral`, `--scheduled`, `--deadline`, `--depends`, `--author` (+ global `--vault`/`--json`/`--ndjson`/`--quiet`).
- `list`: `--all`, `--state`, `--durable`, `--ephemeral`.
- `done <ref>`: `--ephemeral` (positional becomes the 1-based index when set).
- Output mirrors `adopt.go`: a `Pretty()` status line suppressed under `mode==Pretty && quietFlag`; `--json`/`--ndjson` always emit structured results. Result structs use `omitempty` json tags so durable-only fields don't appear on ephemeral records and vice versa.
- Each subcommand's `RunE` uses `defer resetTodoFlags(cmd)` to clear its local flag vars + pflag `Changed` state, exactly like `query.go`'s `resetQueryFlags` — keeping the shared `resetCLIFlags` harness untouched.

## Test scenarios

Written TDD-red first in `internal/cli/todo_test.go` (compiles against not-yet-defined result structs, pinned in a header comment; `runTodo` helper added). Finalized from the AC-doc's ~25 scenarios.

**Create (AC-1)**
- **T-1.1** durable happy path: `add "Buy milk"` on a vault with no `todos/` → creates `todos/<ULID>.md` with `type: todo`, `state: open`, a valid 26-char ULID, body "Buy milk"; exit 0; JSON result carries `kind:"durable"`, `path`, `id`, `state:"open"`.
- **T-1.2** durable with props: `add "Ship v1" --scheduled 2026-07-10 --deadline 2026-07-15` → file contains scalar `scheduled: 2026-07-10` and `deadline: 2026-07-15` (as props, not links); omitted props produce no empty keys.
- **T-1.3** durable with live dep: pre-existing durable `A`; `add "Blocked" --depends A` → `node.Parse` reports `Link{Rel:"depends-on",To:"A"}` and **no** `Props["depends-on"]`.
- **T-1.4** ephemeral happy path: `add "call dentist" --ephemeral` on fresh vault → `todos/inbox.md` created, `type: todo-ephemeral`, body has an unchecked `- [ ] call dentist`; second `add --ephemeral "buy milk"` appends a second unchecked line, container ULID unchanged, first line byte-identical.
- **T-1.5** fresh-vault dir creation: `--vault <nonexistent-path>` `add "first"` → vault + `todos/` created, file written, no "no such file" error.
- **T-1.6** create-path round-trip: read the file T-1.1 wrote → `node.Parse(raw).Serialize()` byte-equals on-disk bytes.
- **T-1.7** no-clobber (EC-5): pre-create a stray file at the exact target path a deterministic ULID would map to (or assert via a forced-collision seam) → `add` errors, does not overwrite. (If forcing a ULID collision is impractical, assert the `os.Stat`-before-write guard via a stray `todos/<ULID>.md` fixture and a `done`-independent unit on the write helper.)
- **T-1.8** ephemeral rejects date flags: `add "x" --ephemeral --scheduled 2026-07-10` → usage error (durable-only props).

**List (AC-2, AC-6)**
- **T-2.1** freshness: `add "Task A"` then `list` (no manual `index`) → output includes "Task A", `state:"open"`.
- **T-2.2** kind distinction: one durable + one ephemeral item → `list` shows both tagged `durable`/`ephemeral`; `list --durable` shows only the durable; `list --ephemeral` shows only the ephemeral (also covers AC-6 at porcelain layer).
- **T-2.3** default hides done: one open + one `state: done` durable → `list` shows only open; `list --all` shows both.
- **T-2.4** empty vault: fresh vault, no todos → exit 0, `--json` emits empty items array, no error.
- **T-2.5** missing vault dir: `list --vault <missing>` → exit 0, empty result, no crash.
- **T-2.6** ephemeral list carries a stable 1-based `index` and `checked` flag per item; `--all` includes checked items.

**Done (AC-3)**
- **T-3.1** durable happy path + byte preservation: durable file with `state: open`, an extra hand-added frontmatter key, and a code-fenced body → `done <ULID>` flips only the `state` span to `done`; every other byte identical (diff shows only the state value).
- **T-3.2** durable via alias: file with `aliases: [buy-milk]` → `done buy-milk` resolves to the same file, `state` becomes `done`.
- **T-3.3** idempotent already-done (EC-1): `done <ULID>` on a `state: done` file → exit 0, result reports "skipped/already done", file bytes unchanged (no rewrite).
- **T-3.4** non-existent ref: `done nonexistent-alias` → non-zero exit, "not found" error, distinct from T-3.3.
- **T-3.5** ephemeral done: container with two unchecked items → `done --ephemeral 1` flips item 1 `- [ ]`→`- [x]`; item 2 byte-identical; no ULID/id introduced anywhere in the container; second `done --ephemeral 1` is an idempotent skip.
- **T-3.6** done→list reflects completion: after T-3.1, `list` (Open+Reconcile) no longer shows the item by default but shows it under `--all` with `state:done`.

**Round-trip (AC-4)**
- **T-4.1** parse→serialize identity on any tool-written durable todo (with/without optional props/deps).
- **T-4.2** render round-trip stability: node built via `add`'s construction path → `serialize(parse(render(n))) == render(n)`.
- **T-4.3** edit-path stability (EC-8): durable with unmodeled extra frontmatter/body → after `done`, `parse(serialize(parse(file)))` equals the post-edit file.

**depends-on edges (AC-5)**
- **T-5.1** edge appears after indexing: durable `B` created `--depends A` (A exists); after `rk index`, `rk query "SELECT * FROM edges WHERE rel='depends-on'"` returns one row, `dst` resolves to A, `dst_key` non-NULL.
- **T-5.2** edge not in node_props: `SELECT * FROM node_props WHERE key='depends-on'` → zero rows.
- **T-5.3** dangling dep (EC-2): `--depends nonexistent-ulid`; after rebuild, an edge with `rel='depends-on'`, `dst='nonexistent-ulid'`, `dst_key IS NULL`; no indexing error.
- **T-5.4** dangling later resolves: create the missing target, reindex → the edge's `dst_key` becomes populated (may be framed as reuse of existing index-package dangling-link coverage).

**Queryable distinction (AC-6)**
- **T-6.1** index-level: after creating one durable + one ephemeral, `rk query "SELECT type,count(*) FROM nodes WHERE type IN ('todo','todo-ephemeral') GROUP BY type"` separates the counts correctly.

**Harness / plumbing (AC-7)**
- **T-7.1** `runTodo` helper drives all scenarios via the shared `RootCmd` singleton with `resetCLIFlags`/`resetTodoFlags` between Execute calls.
- **T-7.2** `--quiet` suppresses the pretty status line on `add`/`done` (mirrors adopt/index) without affecting the write.
- **T-7.3** `done` never creates the cache/index dir (mirrors `TestAdoptCmd_NeverTouchesIndex`): run `done` in a vault with no index; assert the cache dir is not created.

## Known risks / ambiguities remaining after these decisions

1. **`list` opens the index read-write.** Reconcile mutates and takes `ix.lock()`. For the single-user local tool this is fine and matches the design's lazy-reconcile-on-read intent, but it is a deliberate divergence from `rk query`'s read-only contract; if concurrent-index concerns ever arise, the fallback is the filesystem-glob list path (D4 alternative). Flagged, not blocking.
2. **Ephemeral `done` positional-index stability across concurrent edits.** The 1-based file-order index is stable within a single view but shifts if items are *added/removed* (not merely completed) between a `list` and a `done`. Acceptable for a transient, un-stored handle by design; documented so users pair `list`→`done` in one step.
3. **No-clobber test ergonomics (T-1.7).** Forcing a genuine ULID collision is impractical; the test likely asserts the `os.Stat`-before-write guard via a pre-planted stray file or a small seam. The behavioral guarantee (never overwrite) is firm; the test mechanism is the soft spot.
4. **`time:` frontmatter value is non-deterministic.** Tests must assert on presence/parse-ability, never exact value. Round-trip tests read-back-and-compare, so they're unaffected.
5. **Ephemeral checkbox regex scope.** The plan matches `- [ ]`/`- [x]` task-list lines (optionally `* `); unusual indentation or non-standard markers are treated as non-items (not listed, not completable). Out of scope to generalize; acceptable for tool-written content since `add` controls the format.
6. **`rk index`-then-list double reconcile.** On a just-built index, `list`'s `Open`+`Reconcile` does an extra (cheap, mostly no-op) pass. Negligible cost; noted for completeness.

### Critical Files for Implementation
- internal/cli/todo.go (new)
- internal/cli/todo_test.go (new)
- internal/cli/stubs.go (delete `todoCmd` stub)
- internal/cli/adopt.go (precedent + reused `writeFileAtomic`)
- internal/node/render.go (create-path recipe)
- internal/index/reconcile.go (`Reconcile` wiring for `list`)
