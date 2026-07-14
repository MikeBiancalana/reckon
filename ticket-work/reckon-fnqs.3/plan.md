# Implementation Plan: reckon-fnqs.3 — Subject/body node convention (multi-line bodies, first line displayed)

## Amendment (post-review, pre-push)

D1 below is **superseded**. Ticket author clarified at the PR gate: the ticket's "or any node"
phrasing overstated intent — the convention was meant for tasks (todo), not notes (already has
`props['title']`) or a future checklist/run type (frontmatter prop is the better fit there too,
same reasoning as note). Changed `insertNode` (`internal/index/reconcile.go:270`) to gate
`deriveTitle` on `n.Type == "todo"`; every other type gets `title = ""`. Added
`TestReconcileTitleTodoOnly` (`internal/index/index_test.go`) as the negative-case regression.
Updated `internal/index/AGENTS.md`, `docs/design/composable-redesign.md`, root `AGENTS.md`,
`internal/node/AGENTS.md` to match. `deriveTitle` itself (`internal/index/title.go`) is unchanged
— still a pure, type-agnostic string function; the scoping lives entirely at the one call site.

## Summary

A durable node's body follows the git-commit shape: the first non-empty line is the subject/title, an optional blank line, then the remainder is the body. This ticket makes list-shaped surfaces (`rk todo list`, `rk today`) render only that first line while `--json` keeps the full body plus a new `title` field, and it exposes `title` as a selectable column on the `nodes` query view. The node layer (`internal/node`) is left completely unchanged — round-trip byte-identity is already structurally guaranteed by `node.Parse`/`Serialize`. The work is: (1) a pure Go title-derivation function computed at reconcile and stored as a physical `_nodes.title` column, (2) the `nodes` view exposing it, (3) a `SchemaVersion` bump to force rebuild, (4) two CLI result structs gaining a `Title` field and their `Pretty()` renderers switching from body to title, and (5) documentation of the convention.

## Design decisions

### D1 — Title scope: uniform, type-agnostic derivation for ALL node rows (not todo-only) — DECIDED

`_nodes.title` is populated for every node row via one unconditional derivation in `insertNode`, regardless of `type`.

- **Rationale:** The ticket says "`_nodes`" (not "`_nodes` where `type='todo'`") and "A durable todo (**or any node**)". Scoping the column to todo-only would force an `if n.Type == "todo"` branch inside `insertNode` — more complexity, against the text. The todo/today examples in the ticket describe the *render* surfaces (which are legitimately type-specific), not the column.
- **Collision with `note`'s `props['title']`:** `internal/cli/note_v1.go:324` stores an explicit, editable `props["title"]` frontmatter label that is a *different concept* from a body-derived title (note bodies default to `""`, so `nodes.title` for a note is usually empty). We do **not** consult `_props` during title computation and do **not** special-case `note`. The two coexist: `nodes.title` = derived first-body-line (index column); `_props['title']` / `node_props` = note's explicit label (unchanged). This divergence is documented in the spec so a future reader of a `note` row's `title` column isn't surprised.
- **Alternative rejected:** derive `title` only for `type='todo'`, or prefer `props['title']` when present. Rejected: contradicts the "any node" wording and adds per-type coupling for no ticket-required benefit (the only consumers reading `title` are todo/agenda surfaces).

### D2 — `rk query`: raw-path only, no canonical-path extension — DECIDED

The DoD "`rk query` can select `title` without string surgery" is satisfied purely by the `nodes` view gaining the column; **no `query.go` code change**. `SELECT title FROM nodes` (no `id` → `rawObjects`) and `--raw "SELECT id, title FROM nodes"` both pass `title` through verbatim.

- **Rationale:** The AC's own two query test scenarios only exercise `rawObjects`. `node.Envelope` must not gain a `Title` field (ticket asserts the node layer is unchanged), so the canonical path cannot carry title without a map-injection hack.
- **Known limitation (decided, not missed):** the natural `rk query "SELECT id, title FROM nodes"` (with `id` → canonical mode → `reconstructEnvelopeObject`, query.go:420) silently drops `title`. This is consistent with canonical mode *already* discarding every non-envelope column the caller selects — it is not a new wart. If a future ticket wants uniformity, the one-line fix is a map-injection in `reconstructEnvelopeObject`/`canonicalObjects` (mirroring how `obj["id"]` is injected at query.go:411), **never** an `Envelope` field. Out of scope here.
- **No query pretty renderer:** `query.go:103-105` unconditionally remaps `Pretty → NDJSON`. There is no `queryResult.Pretty()` to modify; the ticket's "query pretty output" phrasing does not map onto a real surface. We build nothing here.

### D3 — Derivation helper lives in `internal/index`, not `internal/node` — DECIDED (deviates from codebase-analysis.md)

Add an unexported `deriveTitle(body string) string` to the `index` package (new file `internal/index/title.go`), called from `insertNode`.

- **Rationale:** The AC doc is explicit that `internal/node` is out of scope and the node layer is unchanged; the ticket frames title as "computed at reconcile" — an index-layer concern. Co-locating with the sole consumer (`insertNode`) honors that boundary. It stays pure and unit-testable because `internal/index/index_test.go` is `package index` (unexported access works).
- **Explicit deviation:** `codebase-analysis.md` suggested a `node.Title(body string) string` helper in `internal/node`. We reject that to avoid broadening the node package's surface against the AC's stated boundary, and because a `(*Node).Title()` method was specifically flagged as contradicting the ticket's framing. Not pre-exported for fnqs.5 (YAGNI — one-word promotion if ever needed).

### D4 — Derivation algorithm

`deriveTitle(body)`: iterate lines split on `\n`; return the first line that is non-whitespace after `strings.TrimSpace` (which also strips a trailing `\r` for CRLF bodies); return `""` if no such line exists. First non-empty line only — no blank-line separator required, no truncation/length cap.

### D5 — `Title` JSON tag: `omitempty` on both structs — DECIDED

`todoListItem.Title` and `agendaItem.Title` both use `` `json:"title,omitempty"` ``.

- **Rationale:** Durable todos always have a non-empty body (CLI guard at todo.go:273), so `title` is present on every durable JSON row → AC7 ("body and title both present") holds for all tested cases. Ephemeral rows (which have no multi-line body) stay clean with no empty `title` key, matching the existing durable-only-field convention (`Scheduled`/`Deadline`/etc. are all `omitempty`). Reconciles the minor inconsistency in codebase-analysis.md (which suggested no-omitempty for todo). Ephemeral pretty-print keeps rendering `Body` (todo.go:193) unchanged.

### D6 — `title` appended LAST in the `nodes` view

`SELECT node_key AS id, ulid, type, time, author, body, loc_file AS loc, title FROM _nodes`. All current consumers use named columns, so position is irrelevant; appending last matches AC3's stated ordering and disturbs nothing.

## Files to modify

### Production code

| File:line | Change | Reason |
|---|---|---|
| `internal/index/title.go` (**new**) | Add unexported `deriveTitle(body string) string` (first non-empty line, `TrimSpace`'d). | D3/D4. Pure, table-testable. |
| `internal/index/schema.go:28-38` | Add `title TEXT NOT NULL DEFAULT ''` to `CREATE TABLE _nodes` (after `body`). | Physical column per AC2/AC (computed-in-Go, not a view SQL expression). |
| `internal/index/schema.go:81-82` | `nodes` view: append `, title` → `SELECT node_key AS id, ulid, type, time, author, body, loc_file AS loc, title FROM _nodes`. | AC3 — expose on the query contract. D6. |
| `internal/index/schema.go:10` | Bump `SchemaVersion` `2 → 3`; update the doc comment to note v3 adds the derived `title` column. | No migrations; version bump forces `Open`→`Rebuild`. `BuilderVersion` unchanged. |
| `internal/index/reconcile.go:269-275` (`insertNode`) | Compute `title := deriveTitle(n.Body)`; add `title` to the `INSERT OR REPLACE INTO _nodes(...)` column list + one more `?` placeholder + pass `title`. Column order: `node_key,ulid,type,time,author,body,title,loc_file,hash,mtime`. | AC2 — computed at reconcile; runs for both `Reconcile` and `Rebuild` (both funnel through `indexFile`→`insertNode`). Wrap no new error path (Exec error already wrapped). |
| `internal/cli/todo.go:159-173` (`todoListItem`) | Add `Title string ` `` `json:"title,omitempty"` `` after `Body`. | D5. |
| `internal/cli/todo.go:196` (`todoListResult.Pretty()`, durable branch) | Render `it.Title` instead of `it.Body`. Ephemeral branch (line 193) unchanged. | AC4. |
| `internal/cli/todo.go:488` + `526-536` (`listDurableTodos`) | `SELECT id, body FROM nodes` → `SELECT id, body, title FROM nodes`; extend the `row` struct + `Scan`; set `Title: r.title` on the item. Keep `Body: strings.TrimSpace(r.body)`. | AC7 — durable list carries both. |
| `internal/cli/today.go:96-109` (`agendaItem`) | Add `Title string ` `` `json:"title,omitempty"` `` after `Body`. | D5. |
| `internal/cli/today.go:128` (`agendaResult.Pretty()`) | Render `it.Title` instead of `it.Body`. | AC5. |
| `internal/cli/today.go:231` + `293-306` (`buildAgenda`) | `SELECT id, type, body, loc FROM nodes` → add `title`; extend the `candidate` struct + `Scan`; set `Title: c.title`. Keep `Body: strings.TrimSpace(c.body)`. | AC7 — agenda carries both. |

`listEphemeralTodos` (todo.go:573) and `reconstructEnvelopeObject`/`canonicalObjects` (query.go:401-462): **unchanged** (D2, D5). `internal/node/*`: **unchanged** (out of scope).

### Documentation (AC1) — one authoritative home per fact, others are pointers

| File | Change |
|---|---|
| `docs/design/composable-redesign.md:469` (`### Fields`, `body` row) | Document the subject/body convention: first non-empty body line = subject/title; optional blank line; remainder = body. Add a note that the **index** derives a `title` from this (downstream, not a parser behavior). Reconcile the `props` row (line 471, `note: ...title`): explicitly distinguish `nodes.title` (derived index column) from `props['title']` (note's explicit editable label). Do **not** add a new numbered spec invariant (§Invariants) — that would wrongly imply `node.Parse` enforces it. This is the authoritative home for the *convention*. |
| `internal/index/AGENTS.md:27` (query-contract table, `nodes` row) | Update columns to `id, ulid, type, time, author, body, loc, title`. Add a short subsection defining the derived `title` column (first non-empty body line, `TrimSpace`'d, computed in `insertNode`; uniform across all types; diverges from `note`'s `props['title']`). Authoritative home for the *column*. |
| `internal/node/AGENTS.md` | One-line pointer: the subject/body convention is a downstream index derivation over `Body`, NOT a parser behavior (`node.Parse` does not split subject/body). Prevents a future reader assuming the parser handles it. |
| `AGENTS.md` (root, Domain Concepts, ~line 16-19) | One-line note on the git-commit subject/body shape, pointing to composable-redesign.md. (The ticket literally names "AGENTS.md" — root is the referent.) |

### Test-file touchpoints (pinned contracts / snapshots)

| File | Change |
|---|---|
| `internal/cli/todo_test.go` (header "Pinned contract" block, ~line 30-45) | Add the `Title string ` `` `json:"title,omitempty"` `` line to the pinned `todoListItem` shape so the hand-maintained contract stays truthful. |
| `internal/cli/today_test.go` (header "Pinned contract" block) | Add `Title` to the pinned `agendaItem` shape. |
| `internal/index/index_test.go:180` (`dumpAll`) | Add `title` to `SELECT id,ulid,type,time,author,body,loc FROM nodes` so `TestRebuildDeterministic` also snapshots the new column (free regression coverage; not required, recommended). |

## Test scenarios (translated from acceptance-criteria.md §4 — not invented)

### Unit — title derivation (`internal/index/title_test.go`, **new**, `package index`)
- **`TestDeriveTitle`** — table-driven over acceptance-criteria.md §3 Edge Cases: empty `""`→`""`; whitespace-only `"   \n\n  "`→`""`; `"Buy milk"`→`"Buy milk"`; `"Buy milk\n"`→`"Buy milk"`; git-commit shape `"Ship it.\n\nDetails.\n"`→`"Ship it."`; no separator `"Ship it.\nDetails.\n"`→`"Ship it."`; leading blanks `"\n\nShip it.\n\nDetails.\n"`→`"Ship it."`; multiple inner blanks →`"Ship it."`; surrounding whitespace `"  Ship it.  \n\n..."`→`"Ship it."`; CRLF `"Ship it.\r\n\r\nDetails.\r\n"`→`"Ship it."` (no trailing `\r`); 5-line blockquote/fence fixture→`"Ship the report."`.

### Integration — title wired into reconcile (`internal/index/index_test.go`)
- **`TestReconcilePopulatesTitle`** — write a `todos/<ULID>.md` fixture with a multi-line body; `Rebuild`/`Reconcile`; assert `SELECT title FROM nodes WHERE id=?` equals the first non-empty line. (A passing `TestDeriveTitle` does not prove the wiring — this is the AC2 integration assertion.)
- **Schema rebuild:** existing **`TestSchemaVersionAutoRebuild`** (index_test.go:330) already covers the version-bump→rebuild mechanism and compares against the live `SchemaVersion` constant, so it needs no edit. (AC "post-rebuild rows have title populated" is covered by `TestReconcilePopulatesTitle` via the rebuild path.)

### CLI — pretty/JSON surfaces (`internal/cli/todo_test.go`, `internal/cli/today_test.go`)
Fixtures written **directly to disk** (`mustWriteFile`/`writeTodoFixture` pattern), NOT via `rk todo add` (entry UX is fnqs.5). 5-line body `"Ship it.\n\nline2\nline3\nline4\n"`.
- **`TestTodoList_Pretty_RendersTitleOnly`** — `rk todo list` (pretty) output contains `"Ship it."` and does NOT contain `"line2"`/`"line3"`/`"line4"`.
- **`TestToday_Pretty_RendersTitleOnly`** — scheduled-today todo with the same body; `rk today` pretty output shows only the subject (same assertion shape).
- **`TestTodoList_JSON_TitleAndBody`** — `rk todo list --json`: item JSON has `"title":"Ship it."` AND `"body"` containing all 5 lines (outer-`TrimSpace`'d as today). Same for **`TestToday_JSON_TitleAndBody`** (`agendaItem`).
- **`TestTodoList_Ephemeral_Unaffected`** — `todos/inbox.md` checklist; ephemeral rows render exactly as before, no `Title` behavior asserted for `Kind=="ephemeral"`.

### Round-trip byte-identity (`internal/cli/todo_test.go`)
- **`TestTodoList_MultiLineBody_RoundTripByteIdentical`** — write a 5-line-body fixture directly; capture original bytes; run `rk todo list` (which reconciles); assert the file's bytes on disk are byte-for-byte unchanged (same style as `TestTodoDone_DurableBytePreservation`). AC8(b) — a regression guard that nothing routed through `Render`/`SetField`/`Raw`.

### `rk query` (`internal/cli/query_test.go`)
- **`TestQuery_SelectTitle_NoStringSurgery`** — `rk query "SELECT title FROM nodes WHERE type='todo'"` (no `id` → `rawObjects`): emitted row's `title` == `"Ship it."`, no client-side body splitting.
- **`TestQuery_Raw_IdAndTitle`** — `rk query --raw "SELECT id, title FROM nodes WHERE type='todo'"`: row has both `id` and `title`.

## Implementation sequence

1. `internal/index/title.go` + `title_test.go` (`TestDeriveTitle`) — pure, no dependencies.
2. `schema.go` (column + view + `SchemaVersion` bump) and `reconcile.go` (`insertNode`) — wire derivation into the index; add `TestReconcilePopulatesTitle`; update `dumpAll`.
3. `todo.go` + `today.go` struct fields, `Pretty()` renderers, and SELECT/scan population; update the two pinned-contract test headers.
4. CLI + query tests (pretty, JSON, round-trip, query).
5. Documentation (composable-redesign.md, index/AGENTS.md, node/AGENTS.md, root AGENTS.md).
6. Full `go test ./...` — the schema bump rebuilds every test vault; confirm no snapshot/contract test regressed.

## Known risks and ambiguities

- **Canonical `rk query` gap (accepted, D2):** `SELECT id, title FROM nodes` (id present → canonical mode) silently omits `title`. Decided out of scope, consistent with canonical mode already ignoring non-envelope selected columns. Documented here so a reviewer reads it as *decided*, not *missed*. Follow-up (if ever wanted): map-inject in `canonicalObjects`/`reconstructEnvelopeObject`, never an `Envelope` field.
- **Schema bump = full rebuild for every existing vault** (`internal/index/AGENTS.md:62-66`). Expected and correct (index is derived/disposable); changes the next `rk index` run's cost, not correctness. Worth a PR note.
- **Note `title` divergence:** a `note` row's `nodes.title` (body-derived, usually `""`) will differ from its `props['title']` (explicit label). Intentional per D1; the divergence is documented in the spec, not silently absorbed.
- **`_nodes.body` carries the trailing `\n`** that `addDurableTodo` appends (todo.go:341, `body+"\n"`). `deriveTitle` must skip leading blank lines and take the first *non-empty* line — not `strings.SplitN(body,"\n",2)[0]` (which returns `""` for a body starting with a blank line). Covered by the `TestDeriveTitle` blank-first-line and CRLF rows.
- **Pinned-contract test headers are hand-maintained doc comments**, not generated — they must be edited in the same PR as the struct change or they become false (no test auto-fails on drift, per codebase-analysis: no `reflect.DeepEqual`/`JSONEq` exact-key-set assertions exist).
- **`omitempty` on `todoListItem.Title`:** if the pinned-contract test is later hardened to assert an exact key set on an empty-body durable row, `title` would be absent — acceptable per D5 (all tested durable rows have non-empty bodies), but flagged so the implementer doesn't get surprised.

### Critical Files for Implementation
- internal/index/schema.go
- internal/index/reconcile.go
- internal/cli/todo.go
- internal/cli/today.go
- docs/design/composable-redesign.md
