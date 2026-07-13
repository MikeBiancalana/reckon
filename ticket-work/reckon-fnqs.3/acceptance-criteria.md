# Acceptance Criteria: reckon-fnqs.3 — Subject/body node convention

## Grounding facts

- **Write path (unaffected by this ticket):** `internal/cli/todo.go:341`
  `addDurableTodo` joins argv into one line before calling `node.NewNode`.
  Multi-line body entry via `rk todo add`/`rk add` is fnqs.5's job. This
  ticket's fixtures write `todos/<ULID>.md` directly (`mustWriteFile` /
  `writeTodoFixture`, the pattern already used throughout `todo_test.go` /
  `today_test.go`), not via the CLI add path.
- **Read paths needing title:** `todo.go:488` (`listDurableTodos`, `SELECT
  id, body FROM nodes`) and `todo.go:535` (`strings.TrimSpace(r.body)` into
  `todoListItem.Body`); `today.go:231` (`buildAgenda`, `SELECT id, type,
  body, loc FROM nodes`) and `today.go:305` (same trim into
  `agendaItem.Body`). Both must additionally select and render `title`.
- **Index physical layer:** `_nodes` is created at `internal/index/schema.go:28`
  (columns: `node_key, ulid, type, time, author, body, loc_file, hash,
  mtime`); the `nodes` view projects it at `schema.go:81-82`. Rows are
  written in `internal/index/reconcile.go:269` (`insertNode`, one `INSERT OR
  REPLACE INTO _nodes(...)` per parsed `node.Node`). The ticket's "computed
  at reconcile" + "column to `_nodes`" phrasing means a **physical column**
  populated in `insertNode`, not a view-only SQL expression (e.g. not
  `substr(body, 1, instr(body,char(10))-1)` in the `nodes` view DDL) — a
  computed-in-Go value survives odd bodies (CRLF, no trailing newline, blank
  first lines) without fragile SQL string logic.
- **Schema is unversioned/no-migrations:** `schema.go:10`, `SchemaVersion =
  2`. Adding a column requires bumping this constant so `Open` triggers a
  full rebuild (`internal/index/index.go:106-118`); the regression test is
  `TestSchemaVersionAutoRebuild` (`index_test.go:330`).
- **`node.Envelope`** (`internal/node/envelope.go:19`) has no `Title` field
  and must not gain one — the ticket asserts the node layer is unchanged, and
  title is a downstream (index-layer) derivation over `Body`, not something
  `Parse`/`deriveView` computes. If planning is tempted to add a
  `(*Node).Title()` method, that contradicts the ticket's own framing; flag
  as a deviation rather than adopting it silently. **[Reading confirmed, not
  a live risk]** — nothing in the described scope requires touching
  `internal/node`.
- **`rk query`'s canonical-vs-raw split** (`internal/cli/query.go:159`):
  `canonical := !raw && hasColumn(cols, "id")`. A `SELECT` that omits `id`
  (or passes `--raw`) goes through `rawObjects` (`query.go:509`), which
  passes every selected column through verbatim — `title` works there with
  **zero `query.go` changes**, purely from the `nodes` view gaining the
  column. A `SELECT` that includes `id` goes through `canonicalObjects` →
  `reconstructEnvelopeObject` (`query.go:420-422`, `SELECT ulid, type, time,
  author, body, loc FROM nodes WHERE id = ?`), which does **not** currently
  fetch or emit `title` regardless of what the original SQL selected.
- **`rk query` has no pretty renderer.** `query.go:102-105` remaps `Pretty ->
  NDJSON` unconditionally ("Query results are data, not a status line").
  The ticket's "todo list / today / query pretty output render title only"
  wording does not map onto a real surface for `query` — there is no
  `queryResult.Pretty()` to modify and none should be added. `title`
  becomes available to `rk query` purely as a selectable NDJSON/JSON column
  (AC6); do not build a query-specific pretty renderer to satisfy this
  ticket.
- **`dumpAll` (`index_test.go:158-184`) explicitly enumerates `nodes` view
  columns** (`SELECT id,ulid,type,time,author,body,loc FROM nodes`,
  line 180) as a deterministic-rebuild snapshot, used by the two
  `dumpAll(t, ix)` calls around line 204/208 (rebuild-is-deterministic
  test). It is not `SELECT *`, so adding `title` to the view does not break
  it as-is — but it also means that test gets no free regression coverage
  of the new column unless `title` is added to its column list. Flag as a
  test-file touchpoint, parallel to the pinned-contract comments below.
- **`note` type already has an unrelated `title` prop.** `internal/cli/note_v1.go:324`
  sets `props["title"] = title` from a separate CLI argument; note body
  defaults to `""` (`note_v1.go:135,262`, `--body` flag, empty unless
  passed). `docs/design/composable-redesign.md:471` documents this as
  intentional: `props | ... | per-type open bag (todo: state,due; note:
  tags,title; ...)`. A body-derived `nodes.title` for a `note` row will
  usually be empty and is a **different concept** from `_props['title']`.
  `_nodes` is not type-scoped (todo, todo-ephemeral, log-day, log-entry,
  checklist-template, checklist-run, note, and future work-ticket rows all
  land in it), and the ticket's Scope bullet says "`_nodes`," not "`_nodes`
  where `type='todo'`" — so the default reading is a uniform, type-agnostic
  derivation. Flagged as [OPEN] below, not resolved by fiat.

## 1. Explicit acceptance criteria

Derived from the ticket's Scope + Done-when clauses.

1. The subject/body convention — first non-empty body line is the subject
   (title), a blank line, then the remainder is body — is documented in a
   node/index spec doc and in AGENTS.md (see "Doc location," Implicit
   Requirements).
2. `_nodes` gains a physical `title` column, computed in `insertNode`
   (`reconcile.go:269`) from each node's `Body` at reconcile time (both
   `Reconcile` and `Rebuild`, since both funnel through `indexFile` →
   `insertNode`).
3. The `nodes` view (`schema.go:81-82`) exposes `title` alongside the
   existing columns (`id, ulid, type, time, author, body, loc, title`).
4. `rk todo list` pretty output renders `title` only (not the full body) per
   item; `todo.go:181` `todoListResult.Pretty()`.
5. `rk today` pretty output renders `title` only per item; `today.go:117`
   `agendaResult.Pretty()`.
6. `rk query` can select `title` directly via SQL — `SELECT title FROM
   nodes ...` (no `id` column, or `--raw`) — with no post-processing/string
   splitting, purely from AC2/AC3. (Canonical/id-included mode surfacing
   `title` too is an optional enhancement — see Implicit Requirements.)
   Note: `query` has no pretty renderer to update (`query.go:102-105`
   remaps `Pretty -> NDJSON` unconditionally) — the ticket's "query pretty
   output" phrasing doesn't correspond to a real surface; this AC is fully
   satisfied by the column existing and being selectable, nothing more.
7. `--json` output for `rk todo list` and `rk today` keeps the full
   (untrimmed-of-internal-newlines) `body` field **and** adds the new
   `title` field — both present, not title-replaces-body.
8. A todo file with a 5-line body: (a) `rk todo list` (and `rk today`, if
   the todo also matches the agenda predicate) renders exactly one line
   showing only the subject; (b) the file's bytes are unchanged after the
   reconcile that computed its title (round-trip byte-identity — see
   Implicit Requirements, this is a structural guarantee to verify, not new
   behavior to build).

## 2. Implicit requirements

- **Title derivation, precisely:** skip leading blank/whitespace-only
  lines; the first line with non-whitespace content, trimmed of surrounding
  whitespace, is the title. No blank-line separator required — a
  single-paragraph body with no separator still yields a title (its first
  line), same as a body that does have the git-commit blank-line shape (the
  separator affects nothing about *derivation*, only about what a human
  reading the raw file perceives as "body vs. subject").
- **CRLF:** `Body` is `raw[bodyStart:]` verbatim (`node.go:135`) and never
  strips `\r` (`internal/node/AGENTS.md`, "no `\r` byte is ever stripped
  from `Raw`"). Splitting the first line on `\n` alone leaves a trailing
  `\r` on the title for a CRLF body — the derivation must trim it (or use
  `TrimSpace`, which already strips `\r`) so `title` never carries a stray
  `\r`.
- **Empty / whitespace-only body → `title == ""`.** No error, no fallback
  to another field (not `type`, not `ulid`, not a props value).
- **No truncation.** The convention is "first line," not "first N
  characters" — a very long single line is not shortened. (Not stated by
  the ticket; flagged so an implementer doesn't invent a length cap nobody
  asked for.)
- **Round-trip byte-identity is already structurally guaranteed** by
  `node.Parse`/`Serialize` being untouched (`Raw` is authoritative, never
  regenerated) — this ticket adds a pure read-side derivation over `Body`
  and a new read-only index column; nothing writes to vault files. AC8(b)
  is a regression/confirmation test, not new functionality — it fails only
  if some part of this change is wired through `Render`/`SetField` or
  otherwise touches `Raw`, which it should not.
- **Ephemeral todo items don't need `Title`.** `todoListItem.Kind ==
  "ephemeral"` rows are individual `- [ ] text` checklist lines already
  split one-per-line by `splitChecklistLines` (`todo.go:584`,
  `listEphemeralTodos`) — each `it.text` is inherently single-line; there is
  no multi-line body to split. Leave `Title` empty/unset for ephemeral rows
  (mirroring how `Scheduled`/`Deadline`/etc. are already durable-only,
  `omitempty`-tagged fields on the shared `todoListItem` struct), and keep
  ephemeral pretty-print rendering `Body` as today (`todo.go:193`).
- **Struct changes touch pinned contracts.** `todoListItem` and
  `agendaItem` gaining a `Title` field changes JSON shape pinned in
  `todo_test.go`'s header comment (~line 19, "Pinned contract... do not
  change field names/tags without updating that pin") and the analogous
  header in `today_test.go`. Both header comments must be updated alongside
  the struct change, in the same PR.
- **`title` in `--json` is additive, not a body replacement.** The existing
  `strings.TrimSpace(r.body)` / `strings.TrimSpace(c.body)` assignments into
  the JSON `Body` field (`todo.go:535`, `today.go:305`) can stay as-is —
  round-trip byte-identity concerns the vault file's `Raw` bytes, not this
  JSON field, so trimming the JSON `body` was already fine and remains fine.
- **[OPEN] Index-wide vs. todo-scoped derivation.** Ticket text names
  `_nodes` generically; default reading (stated above) is a uniform
  algorithm applied to every row regardless of `type`, including `note`
  (where it will diverge from the existing `props['title']`) and
  `log-day`/`log-entry`/`checklist-*` (where a "first body line" is
  mechanically well-defined but not obviously meaningful). Nothing in scope
  requires reading `_props` during title computation — do not special-case
  `note`'s `props['title']` unless planning explicitly decides to. Flag this
  divergence in the doc (AC1) regardless of which way it's resolved, so a
  future reader of a `note` row's `title` column isn't surprised it doesn't
  match the note's real title.
- **[OPEN, low priority] Canonical `rk query` mode.** Whether
  `reconstructEnvelopeObject` (`query.go:420`) should also fetch and inject
  `title` (mirroring how `obj["id"]` is injected at `query.go:411`) so
  `rk query --fields title "SELECT id FROM nodes"` works too. Not required
  by the literal "Done when" (AC6 above is satisfiable without it); flag as
  an optional enhancement, not a blocking requirement. If added, it stays a
  map-injection in `query.go`, not a `node.Envelope` field.
- **[OPEN] Doc location for AC1.** The ticket says "the node spec +
  AGENTS.md." Two candidates for "node spec," and a real tension on
  AGENTS.md: `docs/design/composable-redesign.md` tabulates node fields
  (~lines 469-475) and is the closest thing to a living node spec.
  `internal/node/AGENTS.md` is the package's own convention doc but this
  convention is computed in the **index** layer, not `internal/node` —
  `internal/index/AGENTS.md`'s "query contract" table (lines 25-32, the
  `nodes` view column list) is arguably the more accurate home for
  documenting a new `title` column. Likely both AGENTS.md files want a
  pointer (index/AGENTS.md: full definition + column; node/AGENTS.md: a
  cross-reference noting the convention is a downstream derivation over
  `Body`, not a parser behavior). Not resolved here.

## 3. Edge cases

| Case | Expected `title` |
|---|---|
| Empty body (`""`) | `""` |
| Whitespace-only body (`"   \n\n  "`) | `""` |
| Single-line body, no trailing newline (`"Buy milk"`) | `"Buy milk"` |
| Single-line body with trailing newline (`"Buy milk\n"`) | `"Buy milk"` |
| Subject + blank line + body (git-commit shape): `"Ship it.\n\nDetails here.\n"` | `"Ship it."` |
| Subject with **no** blank-line separator: `"Ship it.\nDetails here.\n"` | `"Ship it."` (derivation doesn't require the separator; see Implicit Requirements) |
| Leading blank line(s) before subject: `"\n\nShip it.\n\nDetails.\n"` | `"Ship it."` (leading blanks skipped) |
| Multiple blank lines between subject and body: `"Ship it.\n\n\n\nDetails.\n"` | `"Ship it."` (unaffected — only the first line matters) |
| Leading/trailing whitespace on the subject line: `"  Ship it.  \n\nDetails.\n"` | `"Ship it."` (trimmed) |
| CRLF body: `"Ship it.\r\n\r\nDetails.\r\n"` | `"Ship it."` (no trailing `\r`) |
| 5-line body (AC8 fixture), e.g. existing `today_test.go:486` shape (`"Ship the report.\n\n> keep this blockquote\n\n\`\`\`text\nfenced content\n\`\`\`\n"`) | `"Ship the report."` |
| `note` type, empty body (typical — see Grounding facts) | `""`, distinct from `props['title']` — see [OPEN] above |
| `log-day` / `todo-ephemeral` container body (first line is normally a `# Heading`) | Whatever the literal first non-empty line is (e.g. `"# Inbox"` or `"# 2026-07-13"`) — mechanically consistent, not specially suppressed |

## 4. Test scenarios (Given/When/Then)

**Node body round-trip (byte-identical), file-level**
Given a `todos/<ULID>.md` fixture written directly with a 5-line body
(subject + blank + 3 body lines, extra frontmatter preserved, matching the
existing `writeTodoFixture`/`mustWriteFile` pattern in `todo_test.go`/`today_test.go`)
When `rk index` (or `rk todo list`, which reconciles) runs against it
Then the file's bytes on disk are unchanged from the fixture's original bytes
(byte-for-byte comparison, same style as `TestTodoDone_DurableBytePreservation`)

**Title derivation function (unit-level, wherever it's implemented — index package)**
Given each body string in the Edge Cases table above
When the title derivation function runs
Then it returns the expected title in that row

**`todo list` pretty output**
Given a durable todo with a 5-line body ("Ship it.\n\nline2\nline3\nline4\n")
When `rk todo list` runs in pretty mode (no `--json`/`--ndjson`)
Then the item's rendered line contains `"Ship it."` and does not contain
`"line2"`, `"line3"`, or `"line4"`

**`today` pretty output**
Given a scheduled-today todo with the same 5-line body
When `rk today` runs in pretty mode
Then the item's rendered line contains only the subject, same assertion
shape as the `todo list` scenario

**`--json` output carries both fields**
Given the same 5-line-body todo
When `rk todo list --json` runs
Then the item's JSON object has `"title": "Ship it."` and `"body"` containing
the full multi-line text (all 5 lines, `strings.TrimSpace`-outer-trimmed as
today)
And the same holds for `rk today --json`'s `agendaItem`

**`rk query` selects title without string surgery**
Given the same fixture, indexed
When `rk query "SELECT title FROM nodes WHERE type='todo'"` runs (no `id`
column, so the non-canonical/raw-objects path is used per `query.go:159`)
Then the emitted row's `title` field is `"Ship it."`, with no client-side
splitting of `body` required

**`rk query --raw` also works (equivalent path)**
Given the same fixture
When `rk query --raw "SELECT id, title FROM nodes WHERE type='todo'"` runs
Then the emitted row has both `id` and `title` fields (raw mode never
invokes `reconstructEnvelopeObject`, so `title` passes through regardless of
canonical-mode support)

**Schema version bump triggers rebuild**
Given an existing index built under the pre-change `SchemaVersion`
When `Open` runs against it post-change
Then a full rebuild occurs (existing `TestSchemaVersionAutoRebuild` pattern)
and post-rebuild rows have `title` populated

**Ephemeral items are unaffected**
Given `todos/inbox.md` with checklist items
When `rk todo list` runs
Then ephemeral rows render exactly as before (their single-line `Body`,
no new `Title` behavior expected/asserted for `Kind == "ephemeral"`)

## 5. Out of scope

- **`rk todo show` / `rk todo edit`** — fnqs.4. This ticket does not add a
  show/edit surface that displays or edits the full body distinctly from
  the title; it only changes how existing list-shaped surfaces (`todo
  list`, `today`, `query`) render an already-existing body.
- **Multi-line body entry UX for `rk todo add` / `rk add`** — fnqs.5. The
  Scope/Done-when text for *this* ticket is satisfiable entirely with
  fixtures written directly to disk (see Grounding facts and Test
  Scenarios); no change to `addDurableTodo` (`todo.go:341`) or its argv-join
  behavior is required or expected here. If AC8's test needs a body with
  embedded newlines and the harness's `runTodo`/`runToday` test helpers
  already pass such strings straight through to `node.NewNode` today, that
  is incidental existing behavior, not a fnqs.3 deliverable — do not expand
  `todo add` flags/parsing as part of this ticket.
- **TUI** — fnqs.8. No TUI rendering changes.
- **`internal/node` package changes.** `node.Parse`, `deriveView`,
  `extractBody`, `Render`, and `node.Envelope` are all unchanged (see
  Grounding facts). `[Reading confirmed, not a live risk]` — nothing in the
  described scope needs a node-layer edit; if implementation finds a reason
  to touch `internal/node`, that's a deviation from the ticket's explicit
  claim and should be called out, not silently absorbed.
- **`_props['title']` consultation/precedence for `note` rows** — explicitly
  not required (see [OPEN] in Implicit Requirements); default behavior is a
  uniform body-derived column with no per-type special-casing.
- **`work-ticket` type** — no current writer constructs one
  (`node.NewNode("work-ticket", ...)` doesn't exist in the codebase today,
  only referenced as a future-feed stub in `today.go`); title derivation
  applies mechanically the same as any other type if/when such rows exist,
  but there's no fixture to test against today.
- **Performance.** No stated performance requirement; a per-row Go-side
  string scan at reconcile time is acceptable.
