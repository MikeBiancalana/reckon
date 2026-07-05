# Codebase Analysis — reckon-ar9m (v1-T6: recurrence via stored scheduled cursor, org-style)

Ticket: `bd show reckon-ar9m`. Depends on reckon-qiua (v1-T5, `rk todo`, merged, commit
`f55f623`/PR #148) and reckon-uv09 (v1-T4, `rk add`/log tool, merged, commit
`f3f44a1`/PR #149). Both are fully in the worktree already.

This report is research only — no code was changed.

---

## 0. Governing design doc (read this before planning)

`docs/design/composable-redesign.md`, section **"Design: scheduling, reminders &
recurrence (A#3) — decided"** (lines 991–1062), plus the **2026-06-22 amendment**
directly below it (lines 1036–1049), is the actual spec this ticket implements.
Key excerpts:

- **Syntax = org repeaters** on `scheduled`/`deadline`: `+Nd`/`+Nw` (fixed),
  `++Nd` (skip-to-future), `.+Nd` (N after completion date). Optional iCal
  `RRULE` escape hatch is explicitly out of scope for now.
- **Advance model**: "A recurring rule is a **durable node** that **projects**
  occurrences. The cursor is the rule node's own `scheduled:` prop... On
  completion the handler **advances `scheduled`** exactly like org... **and**
  writes a `did` log entry as **audit** (not as the source of truth)."
- **Amendment (2026-06-22, the load-bearing one for this ticket):** "the cursor
  lives in the rule's `scheduled:` prop; the log is audit (it *can* rebuild the
  cursor for recovery, but is never on the hot path)... state smeared across
  prose and derived is neither [deterministic nor hand-fixable]... the cursor
  lives in *text* and survives index-blow-away."
- **Materialization**: "An occurrence **materializes** into a real **ephemeral
  task instance** (promotion: rule → instance) **lazily, on need**: when it (a)
  gets a note/subtask/link, (b) **piles up** (a new occurrence comes due while
  the prior is still open → both coexist), or (c) is pinned/kept."
- **Timezones (A#3d):** do-dates (`scheduled`/`deadline`) are **date-only,
  floating** (a calendar day) — not RFC3339 instants. This matters for the
  repeater arithmetic (point 5 below).
- Elsewhere (line 773–776, the `rk today` design section): "Completion **emits
  a `log-entry` node linked `did`→task**... two linked nodes, not a merge, so
  the hard task/log separation holds." This is the general completion→log
  behavior this ticket specializes for recurring todos; it is **not yet
  implemented anywhere** — `rk todo done` today (T5) only flips `state` via
  `SetField`, no log entry is written (see §1).

There is **no existing "promotion machinery"** — grep of `internal/` for
`promot` only turns up doc comments describing the *concept*
(`internal/node/node.go:3`, `render.go:16`, `parser.go:29`); no reusable
`Promote()`/link-writing helper exists yet. Whatever `did`-link writing this
ticket needs will be hand-rolled in `internal/cli`, following the same
`NewNode → Render → Parse → writeFileAtomic` recipe already used everywhere
else, not by calling into some pre-built promotion API.

---

## 1. Todo tool (`internal/cli/todo.go`, v1-T5, reckon-qiua)

File: `/home/chadd/repos/reckon/.worktrees/reckon-ar9m/internal/cli/todo.go` (785 lines). Full contents read.

### Durable vs ephemeral

- **Durable todo** = one file `todos/<ULID>.md`, `type: todo`, frontmatter
  `state` (`open`|`done`), optional `scheduled`, `deadline` (plain scalar
  date strings), optional `depends-on` (a ref-valued prop → typed `Link`,
  `Rel: "depends-on"`). Created via `addDurableTodo` (todo.go:281-322):
  `id := mintTodoULID()` (seam over `node.Mint()`) → `node.NewNode("todo",
  author, body+"\n")` → set `n.Time`, `n.Props` (map with `state:"open"` +
  optional `scheduled`/`deadline`), `n.Links` (optional `depends-on`) →
  `n.Render()` → `node.Parse(rendered)` → `writeFileAtomic(path,
  parsed.Serialize())`.
- **Ephemeral todo** = one shared container `todos/inbox.md`, `type:
  todo-ephemeral`; individual items are `- [ ] text` checkbox lines in the
  body with **no ULID/node identity** (`addEphemeralTodo`, todo.go:334-373).
  Not relevant to durable recurring rules (recurrence is durable-only, ticket
  is about the rule *node*), but relevant as the analog for how "pile-up
  materializes an ephemeral instance" will likely be implemented — probably
  as either a second durable node or a container-style ephemeral entry; **no
  existing code does this "materialize an instance" step today** — it must be
  designed fresh for T6.

### Completion path — `doneDurableTodo` (todo.go:586-628)

```go
func doneDurableTodo(vaultDir, ref string) (todoDoneResult, error)
```
Resolution: fast path `todos/<ref>.md` (`loadDurableTodoAt`, todo.go:633-649),
else `findDurableTodoByRefOrAlias` walks `todos/*.md` matching `n.ULID == ref`
or `ref ∈ n.Aliases` (todo.go:654-673). If `n.Props["state"] == "done"` →
idempotent skip, **no rewrite**. Else:
```go
if err := n.SetField("state", "done"); err != nil { ... }
if err := writeFileAtomic(foundPath, n.Serialize()); err != nil { ... }
```
**This is the exact seam this ticket must extend**: for a todo whose
`scheduled` prop carries a repeater cookie, `done` must (a) advance
`scheduled` via a *second* `SetField` (or one combined splice) instead of/in
addition to setting `state: done`, and (b) write a `did` log entry linking
back to this todo's ULID. Non-recurring todos keep today's behavior
unchanged (`state: done`, no log entry — unless the ticket wants to unify all
completions to log a `did` entry; the design doc's `rk today` section implies
*all* completions eventually log `did`, but this ticket's literal scope is
"on completion of a **recurring** todo").

### Frontmatter/props representation

- `todoAddResult`/`todoListItem`/`todoDoneResult` (todo.go:132-203) are the
  pinned JSON-shaped result types (test file's header comment is the
  authoritative pin — see §8).
- Durable list reads `state`, `scheduled`, `deadline` out of `node_props`
  (`loadTodoProps`, todo.go:487-505) and `depends-on` out of `edges`
  (`loadDependsOn`, todo.go:507-517) — i.e. `rk todo list` is a **DB read**,
  not a text read, and expects the index to already reflect current text
  (`ix.Reconcile()` called first, todo.go:407-409). A new `repeat`-carrying
  `scheduled` value will show up in `node_props` automatically with zero
  index-schema change (it is a plain scalar prop already).
- `resolveAuthor` (todo.go:211-222): `--author` flag → `$RECKON_AUTHOR` →
  `$USER` → `"local"`. Reusable directly for any new recurrence-related
  command/flag needing an author.
- `writeFileAtomic` lives in `internal/cli/adopt.go:250-280` (temp file in
  same dir + `os.Rename`, mode preserved, temp cleaned up on any error). It's
  package-private (`cli`), already reused by `todo.go` and `add.go` — a T6
  handler in the same package gets it for free.

---

## 2. Log tool (`internal/cli/add.go` + `internal/node/logparser.go`, v1-T4, reckon-uv09)

**Important naming trap:** `internal/cli/log.go` (old file, `logCmd`, journal-
DB-backed) is **legacy** and unrelated to T4/T6. The actual "rk log tool" from
the ticket dependency is **`rk add`** (`internal/cli/add.go`, 227 lines,
command `Use: "add <text...>"`), which writes to `log/<date>.md` group files.
`add.go`'s own doc comment is explicit: "Legacy `rk log` (DB journal) is
untouched, owned by a separate ticket." Do not confuse the two.

### Writer: `internal/cli/add.go`

- `runAddE` → `effectiveLogDate()` (UTC calendar day unless `--date` given,
  add.go:158-163) + `resolveAtTime(addAtFlag)` (HH:MM, UTC default,
  add.go:169-178) → `appendLogEntry(logDir, day, hhmm, author, body)`
  (add.go:184-227).
- `appendLogEntry`: mints `id := node.Mint()`, builds
  `block := node.RenderLogEntry(hhmm, author, id, body)`. First write of the
  day creates `log/<day>.md` via `NewNode("log-day", "", "# "+day+"\n\n"+block)`
  → `Render` → `Parse` → `writeFileAtomic`; subsequent writes append `"\n" +
  block` strictly at EOF (byte-preserving append, no re-serialize of prior
  content), after a defensive `node.Parse(appended)` sanity check.
- `logAddResult{Path, ID, Day, Time}` is the pinned JSON result shape.
- `embeddedHeaderRe` (add.go:70) rejects any body/author containing a line
  starting with `## ` (would be mis-split as a new entry by `SplitEntries`).
  **Any T6-authored log-entry body (e.g. embedding a `did::`-style marker or
  free text) must pass through the same guard** if it reuses this writer path,
  or a parallel guard must be added if T6 writes log entries through a new
  helper.

### `did`→task link: does NOT exist yet

`RenderLogEntry(hhmm, author, ulid, body string) string` (logparser.go:155-157)
today emits exactly:
```
## HH:MM · author
id:: <ulid>
<body>
```
There is **no** `did::`-style marker and no mechanism for a log-entry sub-node
to carry a typed `Link` back to a task. Log-entry sub-nodes are produced by
`buildLogEntry` (logparser.go:90-131) from a raw `Span` inside the day file's
`Raw` — they have **no independent frontmatter block** of their own (they're
just a `## ` header + optional `id:: <ULID>` line + body text), so `SetField`/
`InsertField` (which operate on frontmatter scalar spans) do not apply to a
log-entry sub-node at all.

**This is the key open design question for T6's "write a did log entry"
requirement.** The existing precedent (`id::`, which is deliberately "inert to
the core parser" — `parseFrontmatter` only scans the `---...---` block, so an
`id::` line is just ordinary body text that `LogParser.buildLogEntry` special-
cases) strongly suggests the natural extension is a parallel `did:: <ULID>`
marker line, extracted by `extractEntryID`-like logic and surfaced as
`Link{Rel: "did", To: <task-ulid>}` on the log-entry node — mirroring how the
generic core already turns a ref-valued frontmatter prop into a typed edge
(`node.go` `deriveView`, "per-tool parser's job" for rel vocab per
`node.go`'s own doc comment, line 11-13). Concretely this would mean:

- Extend `node.RenderLogEntry` (or add a new function, e.g.
  `RenderLogEntryWithLink(hhmm, author, ulid, rel, target, body string)`) to
  emit a second marker line, e.g. `did:: <target-ulid>\n`, right after `id::`.
  This is the "Files to modify" surface in `internal/node/logparser.go`.
- Extend `buildLogEntry` to parse that second line into `entry.Links =
  append(entry.Links, Link{Rel: "did", To: target})`, dropping the line from
  `Body` the same way `extractEntryID` already drops `id::`.
- `internal/cli/todo.go`'s completion handler would then call something like
  `appendLogEntry(logDir, day, hhmm, author, didBody, todoULID)` (a new
  variant, since `add.go`'s own `appendLogEntry` doesn't take a link target
  today) — likely a small new shared helper in `internal/cli` (same package as
  `todo.go` and `add.go`, so no export needed) that both `rk add` and the new
  todo-completion path can call, OR the todo-completion path constructs the
  block directly with `node.RenderLogEntry`-plus-`did::` and appends at EOF
  itself, duplicating `add.go`'s day-file-creation logic (todo.go already
  duplicates `addDurableTodo`'s create-vs-append pattern from `add.go`'s
  `appendLogEntry`, so this would not be a new kind of duplication).
- Whichever route is chosen, the **day-file resolution clock bug class**
  from the T4 review (see review commit message: "C1... `getEffectiveDate()`
  defaults to LOCAL calendar date while `resolveAtTime("")` defaults to UTC
  wall-clock... mixing two clocks") is a proven, high-risk failure mode here
  too — `effectiveLogDate()` (add.go:158) is the fixed, UTC-consistent helper
  to reuse verbatim rather than re-deriving day/time resolution independently.

### `internal/node/AGENTS.md` "Group files" section (lines 149-215) is the
canonical doc for `LogParser`/`id::` conventions and must be updated in
parallel with any `did::` addition (mirrors how T4 added its own "Group
files: LogParser and the `id::` marker" section per that ticket's own M1
review fix).

---

## 3. Canonical node package (`internal/node`, v1-T1)

Files read in full: `node.go` (570 lines), `render.go` (129 lines), `parser.go`
(35 lines), `insert.go` (51 lines), `envelope.go` (109 lines), `logparser.go`
(157 lines), `ulid.go` (36 lines).

### `Envelope` (envelope.go:19-33)

```go
type Envelope struct {
    V         int               `json:"v"`
    ULID      string            `json:"ulid,omitempty"`
    Type      string            `json:"type"`
    Time      string            `json:"time"`
    Author    string            `json:"author"`
    Body      string            `json:"body"`
    Aliases   []string          `json:"aliases,omitempty"`
    Props     map[string]string `json:"props,omitempty"`
    Fragments []Fragment        `json:"fragments,omitempty"`
    Links     []Link            `json:"links,omitempty"`
    Loc       Loc               `json:"loc"`
    Hash      string            `json:"hash,omitempty"`
    Mtime     string            `json:"mtime,omitempty"`
}
```
`(*Node).Envelope()` projects a `Node` into this; not obviously needed by T6
unless a new NDJSON-emitting surface is added (unlikely for this ticket —
`rk todo`/`rk today` are DB/text readers, not NDJSON emitters, in the current
code).

### `Node` struct + byte-preserving edit primitives (node.go:71-178)

```go
type Node struct {
    Raw []byte
    ULID, Type, Time, Author, Body string
    Aliases   []string
    Props     map[string]string
    Fragments []Fragment
    Links     []Link
    Loc       Loc
    // unexported: frontmatter, fmOrder, fieldSpans, bodySpan
}

func Parse(raw []byte) (*Node, error)
func ParseAt(raw []byte, loc Loc) (*Node, error)
func (n *Node) Serialize() []byte                 // == Raw, verbatim
func (n *Node) SetField(key, value string) error  // span-local splice, key MUST already exist as scalar
func (n *Node) HasField(key string) bool
```

**`SetField` is the exact "span-local text edit" primitive the ticket
requires for advancing `scheduled:`.** Mechanism (node.go:151-167): looks up
the byte `Span` recorded for `key` in `fieldSpans` (populated during
`parseFrontmatter`, one contiguous span per scalar `key: value` line),
splices `value` into `Raw[span.Start:span.End]` leaving every other byte
untouched, then **re-parses the spliced bytes from scratch**
(`ParseAt(out, n.Loc)`) and overwrites `*n = *reparsed` — so spans/derived
fields are never stale after an edit. `todo.go`'s `doneDurableTodo` already
uses exactly this for `state` (`n.SetField("state", "done")`); the
recurrence handler would call `n.SetField("scheduled", nextValue)` the same
way. **Constraint:** `scheduled` must already exist as a scalar frontmatter
key (it does, whenever `--scheduled` was passed at `todo add` time) — `SetField`
errors on a missing key; `InsertField` (insert.go) is the counterpart for
adding a *new* key, needed only if a recurring rule's `scheduled` could be
absent at creation and inserted later (unlikely — a recurring rule needs
`scheduled` from creation to have anything to advance).

**Repeater cookie fits in the existing scalar-value span with zero parser
change.** `fmScalarRe` (node.go:102) is
`^([A-Za-z0-9_-]+):([ \t]*)(.*?)([ \t]*)$` — group 3 (the captured value span)
is lazy but anchored against a trailing-whitespace-stripping group 4, so an
**internal space is preserved**: `scheduled: 2026-07-10 +1w` parses with
`n.frontmatter["scheduled"] == "2026-07-10 +1w"` and `n.fieldSpans["scheduled"]`
covering the *entire* `"2026-07-10 +1w"` substring (verified against the
regex semantics; this is the org-style "date + repeater cookie in one field"
representation and it Just Works against the existing frontmatter scanner —
**no `node.go` changes needed to store the repeater syntax**, only new parsing
*logic* in the todo/recurrence handler to split `"2026-07-10 +1w"` into
(date, repeaterKind, N, unit) and to re-render the advanced value back into
that same one-line shape via `SetField`).

### `InsertField` (insert.go, 51 lines)

Adds a new scalar key that doesn't exist yet — trichotomy: key already
present → error (use `SetField`); terminated frontmatter block exists → splice
new `"key: value\n"` line as the FIRST line inside the fence; unterminated
fence → refuse; no frontmatter at all → prepend a fresh block. Relevant if
recurring-rule creation needs to add e.g. a `repeat:` marker prop distinct
from folding the cookie into `scheduled` itself (a design choice for the
planning phase — see open questions below).

### `Render`/`NewNode` (render.go)

`NewNode(typ, author, body string) *Node` mints a ULID and returns a
Raw-less node; `(*Node).Render() []byte` is the create-path inverse of
`deriveView`, emitting canonical frontmatter (`id, type, time, author,
aliases`, then `Props` sorted by key, then typed `Links` grouped by `Rel`
sorted by `rel,to`). Every existing create path (`addDurableTodo`,
`appendLogEntry`'s day-file branch) uses `NewNode → Render → Parse →
writeFileAtomic`; a new recurring-rule creation path (if `rk todo add` grows
a `--repeat` flag) would do the same.

### Group-file primitives (node.go:519-569) — `SplitEntries`/`ReplaceEntryBody`

```go
func (n *Node) SplitEntries() []Entry           // Entry{Header string; Span Span}
func (n *Node) ReplaceEntryBody(e Entry, newBlock string) ([]byte, error)
```
These are the ATX-`## `-header-specific group-file primitives `LogParser`
uses for `log/<date>.md`. Not directly usable for the recurring-rule
`scheduled:` cursor (that's a single-file frontmatter edit via `SetField`,
not a group-file entry edit) — but `ReplaceEntryBody` is the template for how
a `did::`-marker-bearing log entry block would need to be constructed/spliced
if T6 needs a variant of the day-file append path (see §2).

### `Link`/`Fragment` types (node.go:52-64)

```go
type Link struct {
    Rel, To string
    FromFrag, ToFrag string `json:"from_frag,omitempty" / "to_frag,omitempty"`
}
```
`Rel` is a free string — "per-type rel vocab... is a per-tool parser's job"
(node.go:11-13 doc comment). `depends-on` (T5) is the existing precedent for
a tool-invented `Rel`; `did` (T6) would be the same pattern, just sourced
from a log-entry's special marker line instead of a frontmatter ref-valued
prop (see §2's open question).

### `internal/node/AGENTS.md` (236 lines, read in full)

Documents the byte-preservation invariant, the supported frontmatter/markdown
subset, `Render`, and the "Group files: LogParser and the `id::` marker"
section (T4's own addition) in detail. **Conventions/pitfalls section
(lines 216-227)** to obey: any change to `parseFrontmatter`'s scanning loop
must be re-verified against `TestRoundTripIdentity`/`FuzzRoundTripIdentity` in
`node_test.go` (adversarial round-trip corpus) before anything else; a key
with no recorded span is read-only via `SetField` "by construction, not by a
special-cased error check" — i.e. don't add ad-hoc error branches, extend
`fieldSpans` population instead if a new key shape needs edit support.

---

## 4. Index package (`internal/index`, v1-T2) — confirms text is sole state authority

Files read in full: `index.go` (162 lgines), `reconcile.go` (626 lines),
`schema.go` (109 lines).

**Direct answer to the ticket's core constraint:** `Rebuild()` (reconcile.go:45-82)
does `DROP` + recreate every physical table (`dropDDL`/`schemaDDL`, schema.go),
then calls `reconcileTx` which does nothing but **walk the vault directory,
read+parse each `.md` file with `ix.parser` (`node.LogParser{}` by default,
index.go:69-71), and upsert rows derived purely from each file's parsed
`Node`** (`indexFile`/`insertNode`, reconcile.go:227-297). There is **no
secondary state store** anywhere else — no separate "recurrence cursor"
table, no per-tool sidecar file, nothing. The physical schema (schema.go:27-91)
is exactly `_nodes` (id/ulid/type/time/author/body/loc/hash/mtime), `_edges`,
`_props` (key/value per node), `_aliases`, `fts_search`, plus bookkeeping
(`_file_meta`, `_index_meta`) — **all of it reconstructed from scratch by
`Rebuild` from vault text alone.**

**Consequence for this ticket:** if the recurrence cursor (the current
`scheduled:` value/repeater state) lived *only* in the SQLite index — e.g. as
a DB-only column not reflected in any file's frontmatter — a `Rebuild()` would
silently reset every recurring todo's cursor to whatever the *last text
edit* encoded, discarding any index-only progress. This is precisely why the
design doc's 2026-06-22 amendment (§0) mandates the cursor live in the
`scheduled:` frontmatter prop of the rule's own `.md` file: `Rebuild`
re-derives `node_props.scheduled` from that exact text on every rebuild, so
"blowing away + rebuilding the index" is a correctness non-event as long as
the implementation **never** advances the cursor by writing only to the DB
(e.g. never do `UPDATE _props SET value=? WHERE key='scheduled'` without a
corresponding `n.SetField("scheduled", ...)` + `writeFileAtomic` first). This
also means the recurrence advance logic does **not** need to touch
`internal/index` at all — it's a pure `internal/node` + `internal/cli` text
edit, and the *next* `Reconcile()`/`Rebuild()` picks up the new `scheduled`
value automatically via the existing hash-authoritative change detection
(reconcile.go:160-181).

`internal/index/AGENTS.md` (100 lines, read in full) documents this same
"derived, disposable... no migrations" model and confirms `rk todo list`-style
callers should keep reading through the public views (`nodes`, `edges`,
`node_props`, `aliases`, `fts`/`fts_search`), never the private `_`-prefixed
tables.

---

## 5. Existing repeater/recurrence/org parsing code — none (confirmed greenfield)

Grepped `internal/` and `docs/` for `repeat|recur|org-style|org-mode` and for
literal `+1d`/`++`/`.+` patterns near "schedul"/"repeat"/"recur". Findings:

- **No implementation exists.** Zero hits in any `.go` file for repeater
  parsing, cursor advance, or org-date syntax.
- The only substantive hits are the design-doc prose already covered in §0
  (`docs/design/composable-redesign.md`,
  `docs/design/composable-redesign-assessment.md`,
  `docs/design/composable-redesign-rebuttal.md`) plus incidental unrelated
  matches (`strings.Repeat` in test helpers, "recurring issue" in
  process/skill docs, a stale `scheduled_date` DB column mentioned as dead
  code in the redesign doc).
- `composable-redesign-assessment.md:27` and `:43` are worth a glance for
  context/skepticism on the recurrence feature's cost/value trade-off (the
  assessment initially proposed *deferring* recurrence; the owner's rebuttal,
  `composable-redesign-rebuttal.md:132-141`, is what pinned it back into v1
  scope with the stored-cursor amendment) — background only, not load-bearing
  for implementation.
- This ticket is therefore **fully greenfield** for the repeater-parsing
  logic itself (parsing `+Nd`/`++Nd`/`.+Nd` cookies, computing the next date
  for each family). No prior art/partial implementation to reconcile with or
  extend — a new small parsing module (likely `internal/node` or a new
  `internal/todo`/`internal/recur`-style package, or simply free functions in
  `internal/cli/todo.go` mirroring how `todo.go` already hosts all of T5's
  logic in one file) is needed from scratch.

---

## 6. AGENTS.md conventions per subsystem

Existing `AGENTS.md` files (all read in full or in relevant part):
`/AGENTS.md` (root, 556 lines), `internal/cli/AGENTS.md` (469 lines),
`internal/node/AGENTS.md` (246 lines), `internal/index/AGENTS.md` (100 lines),
`internal/journal/AGENTS.md` (409 lines), `internal/storage/AGENTS.md`
(584 lines), `internal/tui/AGENTS.md` (not read — TUI is out of scope for
this ticket, no evidence `rk today`/TUI surfaces are touched by T6's stated
scope).

- **`internal/cli/AGENTS.md`** — describes the **legacy** Cobra command
  conventions (task/notes/log/schedule/win — the pre-redesign DB-backed
  commands) plus generic Cobra patterns (flag reset, `--quiet` handling,
  error wrapping, "return errors, don't `os.Exit`"). Its "Common Pitfalls"
  section (lines 376-421) lists: forgetting `--quiet`, printing errors to
  stdout instead of stderr, not validating args via Cobra's `Args`, and
  hardcoding dates instead of respecting `--date`. **This file has not been
  updated for the v1 redesign tools (T3/T4/T5) at all** — it still describes
  the old `taskService`/`journalService` globals pattern; `todo.go`/`add.go`
  don't follow it (they use the newer `config.LoadWithOverrides` +
  `index.Open` + `node.Parse` recipe instead). Treat this file's *generic*
  Cobra hygiene advice (flag reset, `--quiet`, error wrapping, stderr-vs-
  stdout) as still applicable; ignore its service-layer architecture
  guidance as stale for anything built on `internal/node`/`internal/index`.
- **`internal/node/AGENTS.md`** and **`internal/index/AGENTS.md`** — covered
  in §3/§4 above; both are current and were updated by T4 as part of that
  ticket's own review fixes (a documented convention: "Document new parser
  behavior in AGENTS.md" is itself a review-caught pattern, see §7). **T6
  should update `internal/node/AGENTS.md`'s "Group files" section if it adds
  a `did::` marker**, mirroring T4's own precedent.
- **`internal/journal/AGENTS.md`** / **`internal/storage/AGENTS.md`** — both
  describe the **pre-redesign, DB-backed** journal/task/storage subsystem
  (the legacy `internal/journal`, `internal/storage` packages used by
  `internal/cli/log.go`'s legacy `logCmd`, `internal/cli/task.go`, etc.).
  Not relevant to this ticket's implementation surface (which is entirely
  `internal/node` + `internal/index` + `internal/cli` for the new tools) —
  skimmed, no actionable content for T6 found. Do not confuse their patterns
  (repository/service layering, `*sql.DB` directly) with the new tools'
  simpler text-first recipe.
- **Root `/AGENTS.md`** (556 lines) — general repo-wide conventions (build/
  test commands, overall architecture map, links to the design docs in §0).
  Confirms `docs/design/composable-redesign.md` as the canonical design
  reference and lists the v1 ticket sequence (T1 keystone spike → T2 index →
  T3 query → T4 log → T5 todo → **T6 recurrence**, matching this ticket's own
  "depends on" list).

---

## 7. `docs/REVIEW_PATTERNS.md` — pitfalls relevant to this ticket

1,025-line file; read the relevant sections (Error Handling, Resource
Management, CLI-Specific Patterns, Testing Anti-Patterns, Time/Timezone
Handling, "Good Patterns to Replicate", Pattern Frequency Summary).

**Most relevant single entry — Time/Timezone Handling** (lines 927-953,
duplicated at 1001-1024, "discovered in reckon-gcuu"):
> `time.Parse("2006-01-02", dateStr)` returns UTC midnight. Comparing against
> `time.Now().Truncate(24*time.Hour)` (also UTC midnight) seems consistent,
> but `today.Format("2006-01-02")` formats in *local* time — producing the
> wrong calendar date in UTC-offset timezones... **For testable functions**
> that accept `now time.Time`, derive `loc` from `now.Location()` and pass it
> to any parse calls so tests can inject UTC and stay consistent.

This is **directly on point** for repeater arithmetic: `++Nd` ("skip to next
future occurrence") requires comparing a parsed `scheduled` date against
"today," and the design doc pins do-dates as **date-only, floating** (§0,
A#3d) — exactly the shape this anti-pattern warns about. T4's own commit
history (`f3f44a1`'s "C1" fix, §2) already hit a near-identical bug class
(UTC vs local clock mixing between `getEffectiveDate()` and
`resolveAtTime("")`) and had to add `effectiveLogDate()` specifically to fix
it. **T6's repeater-advance function should accept an explicit `now
time.Time` (or `today time.Time`) parameter** rather than calling
`time.Now()` internally, so tests can inject a fixed date deterministically
and so the "which clock" question has one obvious, reviewable answer.

**General CLI/error patterns also applicable** (Error Handling §19-77,
CLI-Specific §173-228): wrap every error with `fmt.Errorf("...: %w", err)`
context (matches `todo.go`/`add.go`'s existing style exactly — every error
already follows `"todo done: ...: %w"`-shaped messages); respect `--quiet`;
never `os.Exit`; validate args via Cobra's `Args`. All already the house
style in `todo.go`/`add.go` — continue it.

**Testing Anti-Patterns** (lines 230-293): table-driven tests covering edge
cases (empty input, boundary conditions — "first/last day of year" is called
out explicitly, relevant to date-rollover in repeater math, e.g. `+1d` from
Jan 31 or across a leap day) and tests that actually assert what their name
claims. **Good pattern to replicate**: table-driven tests (lines 298-334),
exemplary implementation plans with explicit design-decision/alternative/
rationale write-ups (lines 791-819 — this is the format `ticket-work/reckon-
qiua/plan.md`'s "Design decisions" section already follows, see §8), and
test-first/TDD-red development (lines 822-846 — also the pattern `todo_test.go`/
`add_test.go` already follow, see §8).

---

## 8. Files likely to be created/modified, and test conventions to follow

### Likely files to modify

- **`internal/cli/todo.go`** — `doneDurableTodo` (todo.go:586-628) is the
  primary hook: after the existing idempotent-skip check, branch on whether
  the todo's `scheduled` value carries a repeater cookie; if so, compute the
  next `scheduled` value (new recurrence-math helper, see below), call
  `n.SetField("scheduled", next)` (and/or `SetField("state", ...)` if the
  design keeps the rule node always `state: open` rather than marking it
  `done` — an open design question the planning phase must resolve: does a
  recurring rule's `state` ever become `"done"`, or does completing an
  occurrence only ever advance `scheduled` and leave `state: open`
  permanently?), then write the `did` log entry (see next bullet), then
  `writeFileAtomic`. Also likely needs a new `todo add --repeat <cookie>`
  flag (or folding the repeater into the existing `--scheduled` flag's
  accepted syntax) to let a rule be created with a repeater from the start —
  mirrors `todoScheduledFlag`'s existing flag-var pattern (todo.go:32-65).
- **`internal/node/logparser.go`** — extend `RenderLogEntry` (or add a
  sibling function) and `buildLogEntry`/`extractEntryID`-equivalent parsing
  to support a `did:: <ULID>` marker line producing `Link{Rel: "did", To:
  ulid}` on the log-entry node (see §2's detailed writeup). This is the
  piece most likely to need new test coverage in
  `internal/node/logparser_test.go` (464 lines today, T4's own test file —
  follow its existing structure: split-count assertions, distinct-ULID
  assertions, round-trip-through-`roundtripCorpus` assertions).
- **`internal/cli/add.go`** — possibly needs a variant of `appendLogEntry`
  (add.go:184-227) that accepts a `did`-target ULID, or `todo.go` grows its
  own small local helper that constructs the day-file append itself (either
  is consistent with existing duplication level between `todo.go` and
  `add.go`'s create-vs-append logic).
- **New recurrence-math code** — a new file is likely warranted (e.g.
  `internal/cli/recur.go`, or a new package `internal/recur`/`internal/todo`
  if the logic is judged substantial/reusable/independently-testable enough
  to not want it entangled in `internal/cli` — worth the planning agent's
  explicit call, since `internal/cli` currently hosts *all* T3/T4/T5 logic
  directly with no intermediate service layer, per the "per-tool logic lives
  in `internal/cli/<tool>.go`" precedent established by `todo.go`/`add.go`,
  but a pure date-arithmetic function with no CLI/IO dependencies is also a
  natural candidate for a standalone, more easily unit-tested package).
  Needs: a parser for the `scheduled`/`deadline` value's repeater suffix
  (`"2026-07-10 +1w"` / `"2026-07-10 ++1w"` / `"2026-07-10 .+3d"` →
  date + kind + N + unit), and an advance function per family:
  - `+Nd` (fixed/catch-up): next = old date + N (repeats every N units from
    the last scheduled date, regardless of when actually completed — can
    stay in the past if completed very late; "catch-up" per the design doc's
    own parenthetical).
  - `++Nd` (skip-to-future): next = old date + N, repeated until the result
    is `>= today` (skips missed cycles rather than piling them up one at a
    time).
  - `.+Nd` (from-completion): next = **today** (completion date) + N,
    ignoring the old scheduled date entirely.
  - Needs explicit unit support for at least `d` (days) and probably `w`
    (weeks)/`m` (months) — org supports `d/w/m/y`; the ticket text only
    literally requires `+Nd` shapes but the design doc's own example is
    `+1w`, so week (and likely month/year, for calendar-correct rollover —
    the "first/last day of year" edge case the REVIEW_PATTERNS.md testing
    section calls out) should be planned for.
- **`internal/node/AGENTS.md`** — doc update for the `did::` marker
  convention, mirroring T4's own "Group files" section addition.
- **`internal/cli/AGENTS.md`** — likely doc update, though as noted in §6 this
  file is already stale for the redesign tools; low priority.

### Test conventions to follow (from `todo_test.go`/`add_test.go`)

Both `internal/cli/todo_test.go` (1,324 lines) and `internal/cli/add_test.go`
(749 lines) were **written first, TDD-red**, with a distinctive header-comment
convention worth replicating exactly for T6's test file:

1. A comment block explaining the TDD-red state: "internal/cli/todo.go does
   not exist yet... every test below references types this file's
   implementation phase must define... so the whole `cli` package fails to
   COMPILE right now. That is the expected TDD-red state."
2. A "Precedent / harness reuse (do not redefine these — they already live in
   this package)" list naming exactly which existing test helpers to reuse:
   `setupQueryVault`, `writeTestNode`, `resetCLIFlags`, `buildIndex`,
   `runQuery`, `parseNDJSONMaps`, `countNDJSONLines` (from `query_test.go`);
   `mustWriteFile`, `mustReadFile`, `mustMkdirAll`, `isValidULID`,
   `crockfordAlphabet` (from `adopt_test.go`); `mustDecodeJSON` (from
   `todo_test.go` itself, reusable by a T6 test file in the same package).
   **T6's test file should add its own such list**, including
   `resolveAuthor`/`getEffectiveDate`/`effectiveLogDate` (root.go/todo.go/
   add.go) and any new recurrence-math helpers once they exist.
3. A **"Pinned contract"** comment block that reproduces the exact Go struct
   definitions (with field tags and comments) the implementation must match
   byte-for-byte — copy-pasted from that ticket's own `plan.md`. This is the
   mechanism that keeps the plan.md and the tests from drifting.
4. Test bodies are **not uniformly table-driven** — `todo_test.go`/
   `add_test.go` mix table-driven subtests for pure-function cases (date
   parsing, flag combinations) with individual `Test<Scenario>` functions for
   CLI-integration scenarios (each builds a temp vault dir via `t.TempDir()`,
   writes fixture files with `mustWriteFile`, runs the command via a
   `runTodo(t, vault, args...)`/`runAdd(...)`-style helper capturing
   stdout/stderr/err, then asserts on both the returned JSON result shape and
   the resulting file bytes on disk). **A T6 test file should follow the same
   shape**: table-driven for the pure repeater-math function(s), scenario
   functions for the full `rk todo done` → `scheduled` advances → `did` log
   entry appears → **index rebuild preserves the cursor** end-to-end
   assertions (the last one is the ticket's explicit acceptance criterion:
   "blowing away+rebuilding the index preserves the cursor" — test this
   literally, e.g. delete the cache dir / call `ix.Rebuild()` and re-read
   `node_props.scheduled` for the rule's ULID, asserting it still matches the
   advanced value from the text file, not some stale/reset value).
5. No `testdata/` directory is used anywhere in `internal/cli` for these new-
   style tests (that's a legacy-test-package convention, e.g. possibly used
   in `internal/journal`/`internal/checklist` — not confirmed in this
   research, not needed for T6 based on T4/T5 precedent). Fixtures are
   constructed inline via `t.TempDir()` + `mustWriteFile`/`writeTestNode`, not
   loaded from files on disk.
6. `internal/node/logparser_test.go` and `internal/node/node_test.go` are the
   parallel precedent for any `internal/node` changes (the `did::` marker
   parsing) — same TDD-red header convention, plus the mandatory
   `roundtripCorpus`/`TestRoundTripIdentity`/`FuzzRoundTripIdentity` gate any
   `parseFrontmatter`/`extractBody`/group-file-parsing change must pass (per
   `internal/node/AGENTS.md`'s "Conventions / pitfalls" section, §3 above).

---

## Summary of open design questions for the planning phase

1. **Where does the `did::` marker / `did` `Link` live**, and what's the exact
   text format? (§2 — no existing mechanism; natural extension of the
   `id::` precedent, but needs a concrete function signature decision in
   `internal/node/logparser.go`.)
2. **Does a recurring rule's `state` ever flip to `"done"`**, or does
   completing an occurrence only ever advance `scheduled` (leaving
   `state: open` forever, with "done" being purely a *log* fact via the
   `did` entry)? Affects `doneDurableTodo`'s branching logic directly.
3. **What repeater units are in scope** — the ticket text only literally
   specifies `+Nd`/`++Nd`/`.+Nd` (days), but the design doc's own example is
   `+1w` (weeks); decide whether weeks/months/years are in this ticket's
   scope or deferred.
4. **Where does "pile-up materializes an ephemeral instance" actually write
   to** — no existing code path does anything like this (§1); needs a fresh
   design decision (a second durable node? an ephemeral-container-style
   entry? something new?) with reference to how `todos/inbox.md`'s ephemeral
   container works today as the closest existing analog, and to the
   `internal/checklist` package's `Run`/`RunItem` model (DB-legacy, not
   node-based, but conceptually the closest "template → materialized
   instance" precedent in the codebase — `internal/checklist/model.go:57-94`).
5. **New package vs. existing `internal/cli/todo.go`** for the repeater-math
   logic (§8) — a call the planning agent should make explicitly given the
   codebase's current "no service layer, logic lives directly in
   `internal/cli/<tool>.go`" convention for the redesigned tools.
