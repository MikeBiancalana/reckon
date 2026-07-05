# Acceptance Criteria — reckon-qiua (v1-T5: `rk todo` durable + ephemeral)

Phase 1 (AC extraction), read-only. Builds on `ticket-work/reckon-qiua/codebase-analysis.md`
(Phase 0) — read that first for the precedent hierarchy (`internal/cli/adopt.go` is the
template), exact `internal/node`/`internal/index` signatures, and the stub-graduation
mechanics (`todoCmd` currently lives as a stub in `internal/cli/stubs.go:50-59`, already
wired into `RootCmd` at `root.go:132`).

Grounding sources: `docs/design/composable-redesign.md` ("Atoms" §, per-tool scope table,
"addressability = durability" rule, dangling-link semantics, node-ID/link-syntax decisions),
`docs/design/code-walkthrough-foundation.md`, `internal/node/{node,render,parser,insert}.go`,
`internal/index/schema.go`, `internal/cli/adopt.go`, `internal/index/reconcile.go`,
`internal/config/config.go`, `internal/cli/{root,query,index}.go`, `internal/index/index_test.go`,
`internal/node/node_test.go` (`todoItem` fixture), and sibling ticket descriptions
(`reckon-ar9m`, `reckon-liml`, `reckon-s6oh`, `reckon-cxx1`).

---

## 1. Explicit acceptance criteria (verbatim-grounded, numbered)

Directly from the ticket's "Done when" clause, split into independently verifiable items:

- **AC-1 (create).** `rk todo add` creates a new todo item — a durable file-per-item
  node when durable, a group-file entry when ephemeral — and the write succeeds.
- **AC-2 (list).** `rk todo list` enumerates existing todo items (both durable and
  ephemeral) in a usable, scriptable form.
- **AC-3 (complete).** `rk todo done` marks an existing todo item complete via a
  **span-local edit** (not a full-file rewrite) to its `state` (durable) or its
  checkbox marker (ephemeral).
- **AC-4 (round-trip).** Todo files round-trip: `serialize(parse(f)) == f` for any
  todo file on disk (hand-edited or tool-written), and a tool-created todo's first
  render is itself round-trip-stable (`serialize(parse(render(n))) == render(n)`),
  per `internal/node`'s existing invariants #1 and #6.
- **AC-5 (depends-on edges indexed).** A `depends`-style frontmatter reference on a
  durable todo produces a `depends-on` edge that is visible in the index's `edges`
  view after indexing (`rk index` / reconcile).
- **AC-6 (ephemeral-vs-durable is queryable).** A consumer of the index (`rk query`)
  or of `rk todo list` can distinguish durable todos from ephemeral todos and filter
  by that distinction.
- **AC-7 (tested).** AC-1..AC-6 are each covered by automated tests in the `internal/cli`
  package (following the `adopt_test.go`/`query_test.go` harness conventions already
  established: `setupQueryVault`, `writeTestNode`, `resetCLIFlags`, `buildIndex`, and a
  new `runTodo` helper of the same shape as `runAdopt`/`runQuery`).

---

## 2. Implicit requirements — exact CLI verb shapes

The ticket text and design docs specify the *data model* precisely but leave the verb
surface (flags/args/output shape) unstated. Below is a concrete, grounded proposal for
each verb, built from the precedent in `adopt.go`/`query.go`/`index.go` and the design's
"agent-ergonomic verb" standard (structured `--json`/`--ndjson` beside pretty, `--vault`
override, `Annotations: {"requiresDB": "false"}`). Genuinely open decisions (not settled
by the ticket or design docs) are called out explicitly as **[OPEN]** — a test-writer can
still write concrete tests against the recommended default, but Phase 2 (planning) must
confirm or override each **[OPEN]** item before implementation locks.

### 2.1 Data model recap (grounds every verb below)

| Field | Durable todo | Ephemeral todo |
|---|---|---|
| Storage | `todos/<ULID>.md`, one file = one node | one shared container file (e.g. `todos/inbox.md`), body holds N items as plain text lines |
| Node identity | own ULID in frontmatter `id:`, `type: todo` | the **container** has an id/type (`type: todo-ephemeral` proposed); **individual items are not nodes** and get no ULID (design's "addressability = durability" / "ephemeral = no ID by design") |
| `state` | frontmatter prop, e.g. `state: open` / `state: done` | not a frontmatter prop; represented in-body as a markdown task checkbox (`- [ ] text` / `- [x] text`) |
| `scheduled`/`deadline` | optional frontmatter props | **[OPEN]** whether ephemeral items support these at all in v1; the ticket's per-item edge case ("ephemeral todo with no explicit due/state") implies they are optional, not implies they're supported — recommend: not supported for ephemeral in this ticket, deferred to `rk today`/recurrence tickets |
| `depends` → `depends-on` edge | frontmatter ref-valued prop, single target | not applicable (ephemeral items have no stable address to depend *on* or *from*) |
| Round-trip mechanism | `node.NewNode`/`Render` on create, `SetField` on `done` | `node.SplitEntries`/`ReplaceEntryBody`-style span-local text splice on the container's body (see §2.4) |

**Discriminator key naming [OPEN, but recommended]:** use the literal frontmatter key
`depends-on:` (not `depends:`) so `deriveView`'s `Link{Rel: k, ...}` naturally produces
an edge with `rel == "depends-on"` with zero extra parser code (per `node.go`'s doc:
"per-type rel vocab … is a per-tool parser's job" — the generic core never renames a key).
Note `internal/node/node_test.go`'s `todoItem` fixture uses `depends:` — that fixture is
a generic node-package round-trip exercise, not a pin on the real todo schema's key name.

**Durable-vs-ephemeral type discriminator [OPEN, but recommended]:** `type: todo` for
durable nodes, `type: todo-ephemeral` for the ephemeral container node. This requires zero
schema/parser changes — `SELECT * FROM nodes WHERE type = 'todo'` vs `type = 'todo-ephemeral'`
is directly expressible against the existing public `nodes` view (`internal/index/schema.go`).

**Single-dependency constraint (real, not a choice):** `internal/node`'s frontmatter model
is `map[string]string` — one value per key. `depends-on: "[[X]]"` supports exactly **one**
dependency target per todo in this ticket's scope. Multiple dependencies (`depends-on-2:`,
a list-valued ref, etc.) are **not supported** by the current node package and are out of
scope here (see §5) unless Phase 2 explicitly decides to extend the node model, which this
ticket should not do as a side effect.

### 2.2 `rk todo add`

```
rk todo add <body> [--scheduled YYYY-MM-DD] [--deadline YYYY-MM-DD]
            [--depends <ULID-or-alias>] [--ephemeral] [--vault <dir>] [--json|--ndjson]
```

- `<body>` — required positional, the todo's text (becomes `Body` on the node, or the
  checkbox line's text for ephemeral).
- `--scheduled`, `--deadline` — optional date props (durable only per §2.1); when
  omitted, the corresponding frontmatter key is **absent entirely** (not present with an
  empty string) — matches `render.go`'s `if n.Props["k"] != ""`-style emission via the
  `len(n.Props) > 0` / per-key presence pattern.
- `--depends <target>` — optional, single ref (ULID or alias); written verbatim as
  `depends-on: "[[<target>]]"`. **No existence validation at write time** — per the
  design's dangling-link semantics, an unresolvable target is allowed and becomes a
  dangling edge, resolved later if/when the target is created (see edge case EC-2).
- `--ephemeral` — boolean flag; **[OPEN, recommended default = durable]**. Absent this
  flag, `rk todo add` creates a durable file-per-item node (the deliberate, addressable
  default — consistent with promotion being the "cheap→durable" bridge, i.e. durable is
  the more deliberate act one opts *out* of, not into). `--ephemeral` appends a checkbox
  line to the shared ephemeral container instead.
- `--state` — **not exposed**; every new todo starts `state: open` (durable) / unchecked
  (ephemeral). No flag needed for v1.
- **Author** — `node.NewNode(typ, author, body)` requires an author string, but no
  existing code in `internal/cli` derives one (`codebase-analysis.md` §7 item 5).
  **[OPEN]**: source is undecided (flag? `$USER`? git config?). Recommend a placeholder
  default (e.g. `"local"` or `os.Getenv("USER")`) sufficient to unblock tests, flagged
  for Phase 2 to confirm — this should not block AC sign-off since the ticket doesn't
  mention author/provenance at all.
- Output: structured result (`todoAddResult`-style struct mirroring `adoptResult`), pretty
  line + `--json`/`--ndjson` machine form, containing at minimum: `path`, `id` (durable)
  or `container` + `line` (ephemeral), and the resolved `state`.
- Must ensure the containing directory exists before writing (see EC-6 below) — unlike
  `adopt.go`, which only ever edits files that already exist, `add` creates a **new**
  file/line, so `os.MkdirAll` on the target directory is a genuine new requirement, not
  something inherited for free from the existing precedent.
- Must not silently clobber an existing file at the computed path (see EC-5).

### 2.3 `rk todo list`

```
rk todo list [--all] [--state <state>] [--durable] [--ephemeral] [--vault <dir>] [--json|--ndjson]
```

- No required args. Default: show open items only (both kinds) — matches ordinary
  todo-list UX and avoids a firehose of completed history.
- `--all` — include completed (`state: done` / checked) items too.
- `--state <state>` — filter to an exact state value (`open`, `done`, …).
- `--durable` / `--ephemeral` — mutually-restrictive kind filters; default (neither
  flag) = both kinds, unioned. This is the concrete mechanism satisfying AC-6's
  "queryable" requirement at the CLI layer (the index-level mechanism is the `type`
  discriminator from §2.1; `rk query "SELECT * FROM nodes WHERE type='todo'"` is the
  underlying-index-level equivalent).
- Output per item: at minimum `id`/`alias` (durable) or `container`+`line` (ephemeral),
  `kind` (`durable`|`ephemeral`), `state`, `scheduled`, `deadline`, `depends` (durable
  only), `body`. Default pretty mode is one line per item; `--json`/`--ndjson` emit
  either the canonical node envelope (durable) or an equivalent flat record (ephemeral,
  which has no envelope since it isn't a node) — **[OPEN]**: exact schema of the
  ephemeral list record, since it's the one shape with no existing canonical-node
  precedent to copy; propose a minimal ad hoc struct (`container`, `line_no`, `text`,
  `checked bool`) rather than forcing it into the node envelope shape.
- **Freshness requirement [hard requirement, mechanism OPEN].** AC-1 requires
  create→list to work as an integrated flow without a separate manual `rk index` step
  (a fresh `rk todo add` must be visible to the very next `rk todo list`). Per
  `codebase-analysis.md` §7 item 4, `Index.Reconcile()` exists but has **zero callers**
  today, and `rk query` currently requires a pre-existing index built via `rk index`.
  `rk todo list`, if it reads through the index, **must** call `Reconcile()` (or an
  equivalent freshness pass) before querying — this is an observable behavioral
  requirement even though the specific code path is an implementation choice.
  Alternative: `rk todo list` could skip the index entirely and read directly off the
  filesystem (glob `todos/*.md` + parse each), sidestepping the freshness question
  altogether, at the cost of not using the index's query power. Either satisfies AC-1;
  Phase 2 must pick one, but a fresh add-then-list working correctly is non-negotiable.
- Must not error on a vault with zero todos (see EC-4) or a not-yet-existing vault
  directory (see EC-7) — both must produce a clean "no items" result, not a crash.

### 2.4 `rk todo done`

```
rk todo done <ref> [--vault <dir>] [--json|--ndjson]
```

- Durable: `<ref>` is a ULID or an alias, resolved the same way the index/resolver
  resolves any node reference. The command locates `todos/<ULID>.md`, calls
  `n.SetField("state", "done")` (span-local splice — the field already exists as a
  scalar from creation, so `SetField` — not `InsertField` — is the correct primitive,
  exactly mirroring `adopt.go`'s `SetField("id", id)` call), and writes the result back
  atomically. This is the literal mechanism behind AC-3 "state changes via span-local
  edits."
- Ephemeral: **[OPEN — the most under-specified corner of this ticket].** The design's
  own rule ("ephemeral = no stable address by design") is in tension with needing to
  target *one* specific line for completion. A stable, queryable, linkable ID would
  contradict "ephemeral"; some transient, non-persisted handle is needed instead.
  Recommended concrete shape: address by **1-based item index within the (default)
  ephemeral container**, e.g. `rk todo done --ephemeral 2` — a handle that is valid
  only for the lifetime of one `rk todo list --ephemeral` view, is never stored, and
  is not resolvable by the index (satisfying "no stable address"). The command then
  locates the 2nd unchecked checkbox line in the container's body and flips
  `- [ ]` → `- [x]` via a span-local splice over just that line (the same
  splice-and-reparse discipline as `SetField`/`InsertField`, applied to a body region
  instead of a frontmatter field — `node.ReplaceEntryBody`'s pattern is the closest
  existing primitive, though it is currently coupled to `## `-header entries and would
  need a checkbox-line variant). This needs explicit confirmation before test-writing
  locks in the exact addressing syntax.
- Idempotency on an already-done item: **[recommended: no-op, not an error]** — return
  a "skipped: already done" result, mirroring `adopt.go`'s `Skipped` bucket for
  "already has an id" rather than failing the command (see EC-1).
- Non-existent `<ref>`: error, non-zero exit, structured error message — no silent
  no-op (distinguishing "nothing to do" from "you asked for something that doesn't
  exist").

---

## 3. Edge cases to handle

| # | Edge case | Expected behavior (grounded) |
|---|---|---|
| EC-1 | `rk todo done` on an already-`done` durable todo | No-op / idempotent "already done" result, not an error (mirrors `adopt.go`'s skip-not-fail idiom for "already has an id"). `SetField` is never called a second time with the same value in a way that would corrupt the file; result is reported distinctly from a fresh completion. |
| EC-2 | `--depends <target>` where `<target>` is a ULID/alias that does not (yet) exist anywhere in the vault | **Allowed.** Per the design's dangling-link semantics ("Dangling links allowed … stored as an unresolved edge that auto-resolves when the target is created … queryable"), the todo is created successfully with the ref-valued prop written verbatim; after indexing, the edge row exists with `dst = <target>` and `dst_key IS NULL` (per `schema.go`'s comment: "dst_key is filled by the resolver pass (NULL = dangling)"). No error at `add` time. |
| EC-3 | Ephemeral todo created with no `--scheduled`/`--deadline`/state override | Must succeed with sensible defaults: an unchecked checkbox line, no due-date annotation. `rk todo list` must not crash or emit malformed output when scheduled/deadline are simply absent for an item (applies to durable too — omitted props must not appear as empty-string frontmatter keys per §2.2). |
| EC-4 | `rk todo list` against a vault with zero todos (fresh vault, or all filtered out) | Clean empty result: pretty mode prints a "no todos" style message (or nothing, consistent with `--quiet` conventions elsewhere), `--json` emits `{"items": []}` (or equivalent empty array), exit code 0 — never an error. |
| EC-5 | `rk todo add` computes a target path that already exists on disk (ULID collision — astronomically unlikely but must not be silently possible; more realistically, a manually pre-created stray file at that exact path) | Must **not** silently overwrite. Fail loudly (non-zero exit, clear error) rather than clobbering — this is a real gap relative to `adopt.go`'s `writeFileAtomic`, which is only ever invoked on paths that are already known to exist (in-place edits); `add`'s new-file write path needs its own existence check (e.g. `os.Stat` before `writeFileAtomic`, or open with an exclusive-create mode) since nothing upstream guarantees uniqueness at the filesystem layer. |
| EC-6 | Vault directory (or its `todos/` subdirectory) does not exist yet when `rk todo add` runs (fresh vault, first-ever todo) | `config.LoadWithOverrides` is **pure** — it resolves paths but creates no directories (confirmed in `internal/config/config.go`'s doc comment and by inspection). `writeFileAtomic` (`adopt.go`) does **not** `MkdirAll` its target directory either — it has never needed to, since `adopt` only ever writes into directories that already contain the file being edited. `rk todo add` is the **first** verb that must create a brand-new file in a possibly-nonexistent directory, so it must explicitly `os.MkdirAll(filepath.Join(vaultDir, "todos"), …)` (and transitively the vault root) before the atomic write. Must succeed cleanly on a completely fresh vault path with no prior `rk index`/`rk adopt` run. |
| EC-7 | `rk todo list` / `rk index` run against a not-yet-existing vault directory | Should not crash: `internal/index/reconcile.go` already tolerates a missing vault dir via `os.IsNotExist(walkErr)` (treated as zero files, not fatal) — `rk todo list` inherits this if it reads through the index; if it reads the filesystem directly instead (see §2.3 freshness alternative), it must apply the equivalent tolerance (missing `todos/` dir ⇒ empty list, not an error). |
| EC-8 | A todo file with unmodeled/hand-added frontmatter or body content (Obsidian-authored extra fields, blockquotes, code fences) goes through `rk todo done` | Byte-preservation must hold: only the `state` field's span changes; every other byte (comments, extra frontmatter keys, code fences, blockquotes) is untouched — the general `node.SetField` guarantee, but worth a todo-specific round-trip test since it's the concrete AC-4/AC-3 intersection. |
| EC-9 | `--depends` target is itself a durable todo that later gets `rk todo done`'d | Out of scope for *this* ticket to cascade/notify (no dependency-completion propagation logic requested by the ticket) — just confirming the edge persists and is queryable regardless of either endpoint's `state`. Don't build cascade logic; flag if tempted (scope creep). |

---

## 4. Test scenarios (Given/When/Then), mapped to acceptance criteria

### AC-1 — create

**TS-1.1** (durable create, happy path)
Given an initialized vault directory with no existing `todos/` subdirectory,
When `rk todo add "Buy milk"` is run,
Then a new file `todos/<ULID>.md` is created with `type: todo`, `state: open`, a freshly
minted ULID, the given body, and the command exits 0 with a structured result identifying
the new ULID/path.

**TS-1.2** (durable create with props)
Given an initialized vault,
When `rk todo add "Ship v1" --scheduled 2026-07-10 --deadline 2026-07-15` is run,
Then the created file's frontmatter contains `scheduled: 2026-07-10` and
`deadline: 2026-07-15` as plain scalar props (not links, not misrouted).

**TS-1.3** (durable create with a live dependency)
Given an existing durable todo with ULID `A`,
When `rk todo add "Blocked task" --depends A` is run,
Then the new todo's frontmatter contains a ref-valued `depends-on: "[[A]]"` line, and
`node.Parse` on the resulting file reports a `Link{Rel: "depends-on", To: "A"}` (not a
`Props["depends-on"]` entry — ref-valued props must route to `Links`, never `Props`).

**TS-1.4** (ephemeral create, happy path)
Given an initialized vault with no ephemeral container file yet,
When `rk todo add "call dentist" --ephemeral` is run,
Then the ephemeral container file is created (or appended to, if it already exists)
with a new unchecked checkbox line containing the given text, and the command exits 0.

**TS-1.5** (fresh vault, directory creation)
Given a vault path that does not exist on disk at all (no directory, no `.git`, nothing),
When `rk todo add "first item" --vault <fresh-path>` is run,
Then the vault directory and its `todos/` subdirectory are created as needed and the
todo file is written successfully — no "no such file or directory" error.

**TS-1.6** (create-path round-trip stability — AC-4 intersection)
Given a durable todo just created by `rk todo add`,
When its on-disk bytes are read and passed through `node.Parse` then `.Serialize()`,
Then the output equals the file's on-disk bytes exactly (round-trip identity holds on
the very first write, not just after a subsequent edit).

### AC-2 — list

**TS-2.1** (list sees a just-added durable todo — freshness requirement)
Given a fresh vault,
When `rk todo add "Task A"` is run immediately followed by `rk todo list` (no manual
`rk index` step in between),
Then `rk todo list`'s output includes "Task A" with `state: open` — freshness must not
require a separate indexing step from the user.

**TS-2.2** (list distinguishes durable vs ephemeral — also covers AC-6)
Given one durable todo (`type: todo`) and one ephemeral item in the container
(`type: todo-ephemeral` or equivalent),
When `rk todo list` is run with no filters,
Then both appear in the output, each tagged with its kind (`durable`/`ephemeral`) —
and `rk todo list --durable` shows only the first, `rk todo list --ephemeral` shows
only the second.

**TS-2.3** (default filters out done items)
Given one open durable todo and one `state: done` durable todo,
When `rk todo list` is run with no flags,
Then only the open todo appears; `rk todo list --all` shows both.

**TS-2.4** (empty vault)
Given a freshly initialized vault with zero todo files of any kind,
When `rk todo list` is run,
Then the command exits 0 and reports zero items (empty array in `--json`, a clean
"no todos" message in pretty mode) — not an error.

**TS-2.5** (list against a nonexistent vault directory)
Given a `--vault` path pointing at a directory that does not exist,
When `rk todo list --vault <missing-path>` is run,
Then the command exits 0 with an empty result (consistent with `reconcile.go`'s
existing `os.IsNotExist` tolerance) — not a crash or a confusing filesystem error.

### AC-3 — complete (span-local edit)

**TS-3.1** (durable done, happy path, byte-preservation)
Given a durable todo file with `state: open` plus hand-added extra frontmatter
(e.g. an Obsidian-only field) and a multi-line body with a code fence,
When `rk todo done <ULID>` is run,
Then the file's `state:` line becomes `state: done` and every other byte (the extra
frontmatter field, the body, the code fence) is byte-identical to before — verified by
diffing before/after with only the `state` span differing.

**TS-3.2** (durable done via alias)
Given a durable todo with an `aliases: [buy-milk]` entry,
When `rk todo done buy-milk` is run,
Then the same file is updated (alias resolves to the same ULID) and `state` becomes
`done`.

**TS-3.3** (already-done idempotency — EC-1)
Given a durable todo already at `state: done`,
When `rk todo done <ULID>` is run again,
Then the command exits 0, reports a "skipped: already done" style result (not an
error), and the file's bytes are unchanged (no spurious rewrite).

**TS-3.4** (nonexistent ref)
Given a vault with no todo matching `nonexistent-alias`,
When `rk todo done nonexistent-alias` is run,
Then the command exits non-zero with a clear "not found" error — distinct from the
idempotent-skip case in TS-3.3.

**TS-3.5** (ephemeral done)
Given an ephemeral container with two unchecked items,
When `rk todo done --ephemeral 1` (or the confirmed equivalent addressing syntax) is run,
Then the first item's checkbox flips from `- [ ]` to `- [x]` in place, the second
item's line is byte-identical to before, and no ULID/id is introduced anywhere in the
container file.

### AC-4 — round-trip

**TS-4.1** (parse→serialize identity on a tool-written durable todo)
Given any durable todo file written by `rk todo add` (with or without optional props/deps),
When the file is read and run through `node.Parse(raw).Serialize()`,
Then the output is byte-identical to the file's on-disk contents.

**TS-4.2** (render round-trip stability, per node package invariant #6)
Given a `node.Node` built via the same construction path `rk todo add` uses
(`node.NewNode("todo", author, body)` + `Props`/`Links` set + `.Render()`),
When the rendered bytes are parsed and re-rendered,
Then `serialize(parse(render(n))) == render(n)` — the create path never produces text
that changes shape on its very next read.

**TS-4.3** (edit path preserves unmodeled content, repeated from TS-3.1 as an explicit
round-trip assertion)
Given a durable todo with unmodeled extra frontmatter and body content,
When `rk todo done` performs its span-local edit,
Then `parse(serialize(parse(file)))` still equals the post-edit file (the edited file
is itself stable under a further round-trip — not just different from the original).

### AC-5 — depends-on edges in the index

**TS-5.1** (edge appears after indexing)
Given a durable todo `B` created with `--depends A` where `A` is an existing durable
todo,
When `rk index` (or the equivalent reconcile) is run and then
`rk query "SELECT * FROM edges WHERE src=... AND rel='depends-on'"` is executed,
Then exactly one row is returned with `dst` resolving to `A`'s ULID/alias and
`dst_key` populated (non-NULL, since `A` exists).

**TS-5.2** (edge does not leak into node_props)
Given the same setup as TS-5.1,
When `SELECT * FROM node_props WHERE id=<B> AND key='depends-on'` is queried,
Then zero rows are returned — the ref-valued prop must route to `edges`, never
`node_props` (mirrors the existing generic assertion in
`internal/index/index_test.go` for the `depends` key, applied to the todo tool's
actual `depends-on` key).

**TS-5.3** (dangling dependency — EC-2)
Given a durable todo created with `--depends nonexistent-ulid-or-alias`,
When the index is rebuilt,
Then an edge row exists with `rel='depends-on'`, `dst='nonexistent-ulid-or-alias'`, and
`dst_key IS NULL` — and no error/crash occurs during indexing.

**TS-5.4** (dangling dependency later resolves)
Given the dangling edge from TS-5.3,
When a new durable todo (or alias) matching `nonexistent-ulid-or-alias` is
subsequently created and the index is rebuilt again,
Then the previously-dangling edge's `dst_key` is now populated (auto-resolution on
reindex, per the design's dangling-link semantics) — this test may be framed as an
index-package-level regression check reused from existing dangling-link coverage
rather than a new todo-specific mechanism, since resolution is the index's job, not
the todo tool's.

### AC-6 — ephemeral-vs-durable is queryable

**TS-6.1** (query by type at the index level)
Given one durable todo and one ephemeral container node created via `rk todo add`,
When `rk query "SELECT type, count(*) FROM nodes WHERE type IN ('todo','todo-ephemeral') GROUP BY type"`
is run,
Then the counts correctly separate durable (`todo`) from ephemeral-container
(`todo-ephemeral`) rows.

**TS-6.2** (query at the CLI level, repeated from TS-2.2)
As TS-2.2: `rk todo list --durable` / `--ephemeral` filters correctly at the porcelain
layer, not just at the raw-SQL layer — confirms the distinction is queryable through
both the agent-facing `rk query` surface and the human-facing `rk todo list` sugar.

### AC-7 — tested

**TS-7.1** (harness reuse)
Given the existing `internal/cli` test harness (`setupQueryVault`, `writeTestNode`,
`resetCLIFlags`, `buildIndex`),
When a new `runTodo(t, vault, args...) (stdout, stderr string, err error)` helper is
added following the exact shape of `runAdopt`/`runQuery`,
Then all TS-1.x .. TS-6.x scenarios above are expressible as table-driven or per-scenario
tests using that helper, with no bespoke one-off test scaffolding required.

---

## 5. Out of scope (explicitly, per sibling tickets and ticket text)

- **Recurrence** (org repeaters `+Nd`/`++Nd`/`.+Nd`, stored `scheduled` cursor advance,
  `did` audit log entry on completion, pile-up materialization) — `reckon-ar9m` (v1-T6),
  which explicitly **depends on** this ticket. `rk todo done` in this ticket does not
  need to know about `repeat:` at all.
- **The agenda / `rk today`** (query overdue + scheduled-today + today-pinned, in-list
  relative scheduling keys `t`/`d`/`D`/`p`, do keys `x`/`i`/`c`, completion emitting a
  linked `log-entry` node, split actuator for external tickets) — `reckon-liml` (v1-T7).
  This ticket's `rk todo done` does **not** emit a linked log entry; that behavior
  belongs to `rk today`'s completion flow, not the base todo tool.
- **Migration of current DB-primary task data** (`tasks` SQLite table → text-truth
  nodes, xid→ULID aliasing, dry-run/verify mode) — `reckon-s6oh` (v1-T9). This ticket
  creates the *new* text-primary todo tool from scratch; it does not touch or migrate
  `internal/service`/legacy `task.go`'s existing SQLite-backed task data.
- **Hooks / reminders** (`on-reminder-due`, `remind:` prop, notification delivery) —
  reserved extension seam per the design doc's "Design: hooks" section, not part of
  this ticket. No `remind:` prop handling is required.
- **Note-linking beyond a simple depends-on edge** — generic body `[[wikilink]]`
  references (→ `references` edges) are already handled generically by
  `internal/node`'s `extractBody`; this ticket does not need to add any todo-specific
  body-link handling beyond what a plain markdown body already gets for free. No
  block-fragment (`#^frag`) linking work is required for todos specifically.
- **Any MCP/agent-surface work** — `reckon-cxx1` (v1-T10, fast-follow), which also
  depends on this ticket. `rk todo` only needs to be agent-ergonomic at the CLI/verb
  level (structured `--json`/`--ndjson` output, per the `bd` standard already
  established elsewhere in the codebase); no MCP server wiring is in scope here.
- **Cascading/notifying on dependency completion** (EC-9) — not requested by the
  ticket; a `depends-on` edge is purely a queryable graph fact in this ticket, with no
  behavioral side effects when the depended-on item completes.
- **Multiple dependencies per todo** — the current `internal/node` frontmatter model
  supports one ref-valued prop per key (§2.1); extending it to support N dependencies
  is out of scope for this ticket unless Phase 2 explicitly decides otherwise (and if
  so, that is a node-package change, not a todo-tool-local one).
- **Priority (`p` key/flag)** — mentioned in the `rk today` design section as a
  scheduling key, not in this ticket's prop list (`state`/`scheduled`/`deadline`/
  `depends` only). Do not add a `priority` prop speculatively.
