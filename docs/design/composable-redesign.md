# Reckon Redesign — Composable, UNIX-style System

> Status: **architecture complete; mechanics pending.** Living design record.
> Started: 2026-06-19. Last updated: 2026-06-21.

## Status & resuming (start here)

**What this is.** A total redesign of reckon from one monolithic app into a set
of UNIX-composable tools (log, tasks, notes, ephemeral tasks, checklists) over a
shared plain-text + git substrate with a derived property-graph index. The
**design phase is complete**; what remains is mechanics/implementation.

**Decided pillars:**

| Layer | Decision |
|---|---|
| Substrate | plain-text files (Obsidian-flavored markdown) + git; Syncthing for mobile |
| Atoms | `file` (storage) vs `node` (address); per-tool granularity |
| Identity | ULID (inline, type-agnostic) + `#frag` block IDs + human aliases |
| Read glue | graph-query over a property-graph index in SQLite (per-device, never synced) |
| Write glue | pipe + emit-side disposition (`--ref` / `--pop`) + generic `--import` |
| Keystone | the canonical node (per-tool `parse`/`serialize` pair) |
| Boundaries | Go packages + one multi-call `rk` binary + `rk-<name>` PATH extensions |
| Build call | warranted — current reckon is ~60% there; a refactor, not a rewrite |

**What's next (mechanics, all downhill from the canonical node):**
1. **Index schema** — SQLite `nodes` / `edges` / `fts` / `aliases`; per-device,
   regenerated from text, never synced.
2. **Per-tool specs** — each tool's props, file layout, `parse`/`serialize`, and
   the opinionated structure it imposes.
3. **Resolver** — alias→ULID, `[[ref]]`→file, `ULID#frag`→span (falls out of #1).
4. **`query → schedule → do` UX** — a *UX* requirement (Logseq's weak spot), not
   just data: how a queried task list becomes scheduled and actionable.
5. **Migration** — map current reckon (per-type tables, xid, `note_links`) onto
   the unified node + edges graph; salvage parser / storage / TUI.

See the **Remaining design punch list** section for the full breakdown (open
decisions vs. specs-to-write vs. deferred).

**How to resume / hand off.** Read this file top-to-bottom for the full
reasoning; the **Decision log** records every call with rationale, and **Open
questions** tracks settled vs pending. The design's **core is domain-agnostic** —
a fresh agent (e.g. with work context) can flesh out a new facet either as a new
tool/package or as an `rk-<name>` PATH extension, without touching the core, so
long as it speaks the canonical node format. A beads tracking issue (`bd ready`)
anchors the next steps.

## The realization

Reckon has tried to be one program doing many disparate things:

- personal log (journal)
- todo tracker
- zettelkasten (linked notes)
- checklists (templates → instantiated runs)
- ephemeral tasks (quick capture, short-lived)

Making one app do all of these well is hard, maybe undesirable. The UNIX
philosophy — smaller, composable pieces sharing a common design philosophy —
likely yields a better result.

Two things are needed:
1. A **common substrate** (UNIX uses plain files).
2. A **way to tie the pieces together**.

Each function becomes a discrete program focused on doing one thing well.

## What UNIX actually shares (sharpening)

`ls`, `grep`, `sort` do **not** share a data schema. They share only:

- **Stream format** — lines of text. An *interchange* format, not a storage format.
- **Address space** — filesystem paths. A common way to *name/address* things.

Lesson: the pieces do not need a common data model. They need a common
**interchange format** + a common **address space** for linking. Lighter than a
shared schema. Resist the urge to give every item type the same fields — that
rebuilds the monolith with extra steps.

## Two decisions that drive everything

1. **What is the atom?** (the substrate / unit of storage)
2. **How do the pieces tie together?** (integration mechanism)

## Decision 1 — substrate options

### A. File-per-item
Markdown + frontmatter, one file per item, directory tree, links via `[[id]]`.
- **Win:** grep / ripgrep / git / any editor work for free. Zero lock-in. Most UNIX.
- **Cost:** structured queries (deps, "todos due this week") require scan + parse
  every time. No transactions.

### B. Append-only event log
Everything is an event (created / done / logged / linked). Each tool projects
the log into its own view.
- **Win:** history / undo for free. The log *is* temporal — personal-log falls
  out naturally. Time-travel.
- **Cost:** not greppable. Needs a projection layer to see "current state." Heavier.

### C. Line-oriented per-type
`todo.txt`-style. Each tool owns its own flat file, format tuned to its data.
Shared ID + link syntax across files.
- **Win:** each format optimal. Still grep/awk friendly. Closest to literal UNIX.
- **Cost:** linking across heterogeneous files needs a resolver. No single store.

## The model to steal: git

git is a clean example of "composable tools + common substrate":

- **Substrate** = content-addressed object store.
- **Plumbing** = low-level commands (`hash-object`, `cat-file`).
- **Porcelain** = human commands (`commit`, `log`).
- One dispatcher (`git <verb>`), many small commands.

Mapping to reckon:
- `reckon log`, `reckon todo`, `reckon note`, `reckon check` = porcelain.
- shared store + ID/link resolver = plumbing.
- Each porcelain piece does one thing well, reads/writes the common store.

## The unifying primitive

Do **not** unify on schema. Unify on **ID + time + link**:

- Every item has a stable ID, a timestamp, a body.
- Every item can `[[link]]` to any other.

The link is the zettelkasten primitive — but it also ties *all* the pieces:
a todo → references a log entry → references a note → references a checklist run.
The web of links is the integration. Disparate tools, one graph. This is the
same answer UNIX gave with paths: a shared address space.

## Current leanings & open positions (2026-06-19)

- **git model: liked.** Meshes with an earlier idea of one component being a DB
  store, but the git model is cleaner. Strong candidate for the integration shape.
- **File-per-item (A): mostly liked, with a caveat.** The *atom granularity must
  vary per tool*. One file per ephemeral task is far too much. An org-style file
  per *group* of ephemeral tasks feels right. So: "file-per-item" generalizes to
  "each tool chooses its own atom"; some tools = file-per-item, some = file-per-group.
  Still workable. Interpenetration with the existing UNIX system & tools is desirable.
- **Piping / linear composition: unresolved.** Linear stdin→stdout feeding may be
  the wrong mental model here. Checklists don't process zettelkasten contents —
  there's no obvious linear transform between most pairs of tools. But pulling
  *various threads together into different unified views* of stored material IS
  wanted. That points toward a **query / aggregation over the link graph** model
  rather than linear pipes. (Note: git itself is not pipe-centric either — it's a
  store + commands that query it.)

## Key tension to resolve next

> **RESOLVED 2026-06-19:** graph-query chosen for read/view glue. See Decision
> log below. Section kept for the rationale.

**Linear pipe vs. graph query as the integration model.**

- Pipes = `A | B`: linear transform, one tool's output feeds the next. Great for
  homogeneous text streams. Weak when tools are heterogeneous (checklist ⇏ note).
- Graph query = many item types in one address space; a view/query layer pulls
  threads across types into a unified view. Matches "different unified views of
  what I have stored." Closer to the git "store + query" shape.

Likely answer: the substrate is a linked store; *composition happens via querying
the graph*, not via linear pipes — though pipes may still serve narrow same-type
cases.

## Open questions (to chew on)

1. ~~What is the atom, per tool? Is a mixed model acceptable?~~ **Resolved
   2026-06-19:** yes, mixed is fine once **file** (storage) and **node**
   (address) are split. Per-tool scope table + two granularity rules recorded in
   the "Atoms" section.
2. ~~History/undo worth an event log (B), or is git history enough?~~ **Resolved
   2026-06-19:** git history is enough. Event log killed; see Decision log.
3. ~~Is linking core (everything-is-a-node) or just notes-link-notes?~~ **Mostly
   resolved 2026-06-19:** nodes span all *durable* types (todo, note, log entry,
   checklist run), not notes-only — so linking is core. But addressability is
   gated by durability, so it's "everything durable is a node," not literally
   everything.
4. ~~What does cross-tool composition look like concretely?~~ **Resolved
   2026-06-19:** graph-query over a shared property-graph index; views = saved
   queries; agent authors them. See Decision log.
5. ~~Interchange format when tools talk?~~ **Resolved 2026-06-19:** the canonical
   node spec (NDJSON envelope) is the interchange format, and it equals the
   file→node parser's output. See "Canonical node (spec)".

## Integration shape — graph-query (worked detail)

Mechanics: every tool writes items (ID + time + fields + `[[links]]`) into the
store. An **indexer** scans the store — agnostic to which tool wrote what — and
builds one index. A **query layer** reads across the union. Each tool still owns
its own writes; query is read-only over everything.

Integration = shared index + queries. Not data flowing tool→tool. Data sitting
in one address space, views slicing across it.

**The view no single tool can produce (why this beats pipes):**

```
view "daily review" =
    log entries from today
  + todos due today
  + notes touched today
  + open checklist runs
```

Log/todo/note/check tools know nothing of each other. The query pulls them
together by time + link. That is the composition.

**Query-flavor sub-options** (kept for record; see Decision log — flavor is now
treated as a swappable projection, not a load-bearing choice):

```sql
-- 1. SQL over index (pragmatic; reuses existing markdown→SQLite)
SELECT * FROM items
WHERE type='todo' AND due < now()+7d
  AND id IN (SELECT src FROM links WHERE dst='note:reckon');
```
```
-- 2. Dataview-style (declarative filter + link-follow; simplest to type)
from type=todo where due < +7d and links-to(note:reckon)
```
```cypher
-- 3. Property-graph / Cypher (most traversal power; most build cost)
MATCH (t:todo)-->(n:note {tag:'reckon'}) WHERE t.due < date()+7 RETURN t
```

### Contender table — two axes (don't conflate)

Read/view glue vs. write/reactive glue are different axes. May want one of each.

| Shape | Glue type | What it is | Fit |
|---|---|---|---|
| Linear pipes | read | `A \| B`, text stream | weak — tools heterogeneous, no transform between checklist & note |
| **Graph query** | read | shared index, query across types | **strong — matches "unified views"** |
| Tag/namespace + filter | read | no explicit graph; cross-tool via shared tags (org-agenda model) | simpler/cheaper, less precise; good *complement* not replacement |
| Transclusion / embed | read | doc pulls fragments by ref (Roam, org `#+INCLUDE`) | pairs *with* graph-query; good for hand-authored views |
| Event bus / pub-sub | write | tools emit, others react | only if behavioral glue wanted: close todo → append log; finish checklist → spawn todos |
| Plan 9 / FUSE path-as-API | read | each tool exposes data as virtual files | maximally UNIX, niche, heavy |

## Atoms — storage vs. addressable

Two distinct concepts were both being called "atom." Split them, give each a
system term, and **retire "atom" from the vocabulary** (informal use only).

- **file** — the *storage atom*. One git-tracked, hand-editable file on disk.
  The unit of versioning, editing, and (coarse) change detection. If a tool ever
  needs a directory as its unit, name that case then; today every storage atom
  is a file.
- **node** — the *addressable atom*. One thing with a stable ID: a graph node, a
  `[[link]]` target, an individually-queryable record. Deliberately the same
  word the property-graph index uses — vocabulary stays consistent
  substrate → index.

**Relationship:** a file holds 1..N nodes. The indexer turns files into nodes
but does **not** itself know each tool's internal format — **each tool ships a
"split file → nodes" parser** (cf. git clean/smudge filters, `file(1)` magic).
The core indexer stays generic; mixed granularity needs no special-casing in core.

### Why this is sound — for / against (logged)

Most arguments *against* files-of-varying-size are really arguments against
`file = address`. Once file ≠ node, they evaporate.

**Against (files-as-atoms + varying content):**
1. Link granularity breaks — links target items; a 20-task file needs sub-file
   anchors; path-as-address dies, intra-file IDs get re-invented.
2. Indexer needs per-tool knowledge to split multi-item files → substrate no
   longer opaque to who wrote what.
3. Merge conflicts — many logical edits hit one file; coarse lock.
4. git history coarsens — "when did task 7 change" becomes line archaeology.
5. Event/change detection coarsens — "something in this file changed, go diff."
6. Two storage shapes = two mental models; "common substrate" frays.
7. Index staleness — touch one item, whole file re-parses / all its nodes dirty.

**For:**
1. Right-sized to data's nature — ephemeral = high-volume/low-value/short-lived;
   one-file-each = inode bloat, noisy git. Notes = durable/linked → file-per-item.
   Varying granularity honors real shape (the pro-UNIX reading).
2. Human/editor ergonomics — a day's throwaways in one file beats 50 files.
3. Sometimes the group IS the atom — a checklist run, a day's log.
4. Fewer files = faster, cleaner git, less commit noise.
5. Plain-text tools work on LINES, not files — "many items/file, one per line"
   is *more* UNIX-native for list data (todo.txt, ledger, org-mode).
6. Item-level addressing still cheap — inline IDs (`^id`); file is a container.

**What survives as real cost after the split:** against-#2 (per-tool parsers)
and against-#4/#5 (coarse git/event history for grouped tools). Both land
*exactly* on the data you'd group anyway — ephemeral, low-value, history doesn't
matter. The costs self-select away.

### Two rules that decide granularity everywhere

- **Addressability = durability.** A thing is a node iff you might link to it,
  query it alone, or want its history. Everything else is just content inside a
  file. Corollary: *ephemeral* literally means "gets no stable address."
- **Group vs. isolate.** Group into one file when items are high-volume /
  low-value / short-lived; isolate (file-per-item) when durable / linked /
  history-worthy.

### Per-tool scope

| Tool | file (storage atom) | node (addressable atom) | Notes |
|---|---|---|---|
| Journal / log | 1 file per day | the day + each entry (each a node, own ULID; date/time as alias) | appended through the day; link a date or a moment |
| Todo (durable) | 1 file per todo | the todo (1:1) | deps are edges → IDs mandatory; fine git history wanted |
| Note (zettel) | 1 file per note | the note (1:1); blocks optional via `^id` | densely linked; file-per-item is the right fit |
| Checklist — template | 1 file per template | the template | reusable definition; items usually not addressed |
| Checklist — run | 1 file per run | the run | items are lines inside; address one only if linked |
| Ephemeral task | 1 file per group (day / inbox) | the file/day only — individual tasks NOT addressed | ephemeral = no ID by design |

Spectrum: **file-per-item** (note, todo) ←→ **file-per-group** (ephemeral). The
**promotion path** (ephemeral → durable todo) is the bridge when a throwaway
turns out to matter and needs an address.

## Node ID scheme — prior art & leaning (decision pending)

> Status: **DECIDED 2026-06-19.** Canonical = ULID; human aliases allowed;
> block-level addressing = node-local fragment IDs appended to the node ULID
> (`ULID#frag`). Links inside items are a confirmed requirement.

### Prior art

| System | ID scheme | Stored as | Lesson |
|---|---|---|---|
| Luhmann Zettelkasten | Folgezettel `21/3d7a6` (encodes lineage) | — | structure-in-ID is brittle, position-coupled; abandoned digitally |
| Digital zettel (The Archive, Zettlr) | timestamp `20260619T1430` | inside note | sortable, decentralized, decoupled from title; minute-collisions → add seconds |
| Obsidian | filename = note ID; `^blockid` (short random) for blocks; `#Heading` | filename + inline `^` | **two levels** (note + block); path-as-ID is rename-fragile (app must rewrite links) |
| Roam / Logseq | block-level UUID `((uuid))` | DB / inline `id::` line | everything addressable at block grain; UUIDs unauthorable; Logseq keeps them inline in md |
| org-mode (org-id) | UUID in `:ID:` drawer; `:CUSTOM_ID:` human alias | property on node | ID as a *property*, decoupled from heading; opaque ID + optional human alias |
| TiddlyWiki | title = ID | — | title-as-identity → rename = identity change |
| git | content hash (SHA) | — | content-addressing → identity changes on edit; WRONG for mutable nodes, right for snapshots |
| beads (this repo) | `reckon-4u1c` = prefix + base32 token | DB | namespace prefix + short random; works, but namespace-in-ID couples to classification |
| ULID / UUIDv7 | 128-bit, time-sortable, unique | inline | timestamp-sortability + guaranteed uniqueness, decentralized |

### Lessons

1. **Decouple ID from title, path, AND content.** Store it *inside* the item as
   a property/anchor. Kills rename-fragility (Obsidian/TiddlyWiki) and
   edit-fragility (git-hash). org-id and timestamp-zettel get this right.
2. **Two levels.** node ID + block `^id`; links inside items = `[[nodeid#blockid]]`.
   Universal across Obsidian/Roam/Logseq/org. Well-trodden, surmountable.
3. **Decentralized minting** (you + agents + devices) → random or time+entropy,
   no central counter. ULID/UUIDv7 ideal.
4. **Time-sortable = free win** — chronological order straight from the ID.
5. **Opaque ID + optional human alias** — identity opaque/stable; layer a
   slug/`CUSTOM_ID` for readable, re-pointable links.
6. **Do NOT bake type into the ID** — the promotion path (ephemeral→todo, maybe
   todo→note) reclassifies, and a type/namespace prefix would break links on
   promotion. Store type as a property. (beads bakes the namespace in only
   because its issues don't migrate type; reckon's nodes do.)

### Decision (2026-06-19) — locked

- **Canonical node ID = ULID.** Sortable, unique, decentralized, plain-text,
  type-agnostic. The authoritative identity.
- **A file holds 1..N nodes, each with its own ULID, stored inline.** Two cases,
  not to be conflated:
  - *file-per-item* (note, todo): node ULID in frontmatter `id:`.
  - *group file* (journal day, ephemeral inbox): one ULID per contained node,
    marked inline per item (`id::`-style); the container is itself a node with
    its own ULID.
- **Block / fragment addressing = node-local sub-IDs appended to the node ULID**,
  URL-fragment style: `ULID#frag`. Different level, simpler scheme — the sub-ID
  need only be unique *within* its node, so it stays short. **Delimiter `#`
  consciously chosen** over `^ / :: | >` — see Decision log.
  - Sub-IDs are **stable tokens, never positional** (line/ordinal breaks on
    edit — the decouple-from-content lesson, recursed).
  - A block gets a sub-ID **only when it becomes a link target**
    (*addressability = durability*, recursing to block level). Unlinked prose
    carries no ID at any level.
- **Human aliases** allowed, non-authoritative: date for a journal day
  (`2026-06-19`), slug for a note (`my-note-slug`), time for an entry. Resolver
  maps alias → ULID. Links may be written `[[ULID]]`, `[[ULID#frag]]`,
  `[[alias]]`, or `[[alias#frag]]`.
- **Type = property, never in the ID** (the promotion path reclassifies).

The per-tool **file→node parser** is exactly what splits a file into its nodes
(assigning/reading each ULID) and exposes their fragments — this is where the
two cases above are handled, keeping the core indexer generic.

## Promotion path (write-side node moves)

The one inter-tool *action* (vs. read-side views). Promotion takes content from
one tool and lands it as a node in another (ephemeral→todo, todo→note).

**Mechanism = pipe (validated).** Promotion moves *homogeneous* data
(nodes → nodes) — exactly where pipes fit, and exactly where graph-query did
*not*. The two integration models partition cleanly:
- **graph-query** = read, heterogeneous (unified views across types).
- **pipe** = move, homogeneous (a stream of one payload type: a node).

Shape: `rk <src> --<disposition> <ID> | rk <dst> --import`.
- emit side (`rk <src>`) picks the **disposition** (what happens to the source).
- import side (`rk <dst> --import`) is generic: reads the NDJSON node stream,
  writes into its own store.

### Disposition — selectable per-operation, chosen by the emit verb

- `--ref <ID>` — emit a *new* node carrying a `derived-from:<ID>` edge; **source
  untouched**; import **mints a new ULID**; the source's backlink is surfaced by
  the index (typed edges + backlink index already decided — no write-back to
  source). Use for addressed→addressed (todo→note): preserves the graph.
- `--pop <ID>` — emit the node carrying its **existing ULID** (if any) +
  provenance; the **source tool deletes it from its own store**; import
  **preserves the ULID** (mints only if the source had none, e.g. ephemeral).
  Use for consume/move. **Inbound links survive** because the ULID is preserved
  and type is just a property — promotion-as-move is exactly why type stays out
  of the ID.
- (optional `--cp` — copy, no edge, no delete.)

### Invariants & wrinkles

- **Each tool writes only its own store.** `--pop` deletes from the source store
  (source owns it); `--import` writes the destination store (dest owns it); the
  cross-link is just an edge in the new node, reverse surfaced by the index. No
  tool reaches into another's store → tools stay opaque, pipe model stays sound.
- **Interchange = NDJSON**, one node/line: `ulid?`, `type`, `time`, `props{}`,
  `body`, `fragments[]`, `links[]`. **This equals the file→node parser's output
  serialized** — promotion-serialization and parser-output are two faces of one
  artifact (the canonical serialized node). Speccing promotion partly specs the
  keystone parser contract.
- **Atomicity.** A raw pipe is two processes; exit codes don't flow backward, so
  `--pop` (delete source) then a failed `--import` could lose the node. Two
  answers, pick per need: **git history is the undo** (fine for low-value /
  ephemeral sources; consistent with "git = history/undo"), or a transactional
  **dispatcher form** `rk promote <ID> --to=note --pop` (single process) when
  atomicity matters — same disposition flags.
- **Event bus stays deferred** — promotion needs none of it. YAGNI held.
- (Other mechanisms: drop-file / maildir is usually just `--import`'s backend —
  emit writes a node-file into the dest dir, the indexer picks it up.)

## Canonical node (spec)

The keystone artifact. One representation, four consumers:
1. **parser** — `file → [node]` (splits a file into nodes).
2. **promotion** — pipes nodes between tools (emit/import).
3. **indexer** — nodes → property graph (nodes, edges, FTS, alias map).
4. **resolver** — index → alias→ULID, ref→file, `ULID#frag`→span.

Spec it once; the plumbing collapses into it.

### Two faces

- **Inline (on disk)** — what's authored in the text file: frontmatter + body +
  inline markers. Human/tool/agent writes this. Source of truth.
- **Envelope (NDJSON)** — the parser's normalized output: every inline fact plus
  parser-derived fields (`loc`, `hash`, `mtime`, `v`). Flows through pipes and
  into the index. A superset of the inline facts.
  - *Newlines are safe.* JSON escapes a body's internal newlines as `\n`, so an
    object never contains a literal newline byte; every node — **any type, any
    content** — serializes to exactly one physical line. NDJSON is therefore a
    uniform streaming format across all node types. Only discipline: emit compact
    JSON via a real encoder, never hand-concatenated strings.

### Fields

| Field | Where | Req | Type | Meaning |
|---|---|---|---|---|
| `ulid` | inline + env | yes¹ | string(26) | canonical identity (Crockford base32). ¹absent only on unaddressed emit; import mints it |
| `type` | inline + env | yes | string | node type — a *property*, never in the ULID |
| `time` | inline + env | yes | RFC3339 | semantic timestamp (may differ from the ULID's mint time, e.g. backdated log) |
| `body` | inline + env | yes | string | text content (markdown) |
| `aliases` | inline + env | no | [string] | human handles resolving to this ULID; unique in the alias namespace; non-authoritative |
| `props` | env (from inline fields) | no | object | per-type open bag (todo: `state`,`due`; note: `tags`,`title`; run: `items`). Core indexes generically, does not schematize |
| `fragments` | inline markers + env | no | [{`id`,`anchor?`}] | node-local sub-anchors; `id` unique *within* node; emitted only when targeted |
| `links` | env (from body + ref-props) | no | [{`rel`,`to`,`from_frag?`,`to_frag?`}] | forward typed edges; `to` = ULID or alias (resolved later) |
| `loc` | **env only** | yes² | {`file`,…} | source file; parser-derived; not authored inline. ²required in the envelope |
| `hash` | env only | no | string | body hash (change detection/dedupe); derived |
| `mtime` | env only | no | RFC3339 | last modified (git/file); derived |
| `v` | env only | no | int | envelope schema version (forward-compat) |

### Invariants

1. **Forward + authored only.** A node stores forward, authored facts. *All*
   reverse/aggregate views (backlinks, "what links here", counts) are
   **index-derived and never stored.** No duplicated truth.
2. **Inline ULID is truth.** Filename may mirror the ULID (file-per-item) but the
   inline `id:` wins.
3. **`links` is the normalized edge set.** Two authored sources feed it:
   ref-valued **props** → typed edges (`depends: [[X]]` → `depends-on`); body
   **`[[...]]`** → generic `references` edges. Reserved rels: `references`,
   `derived-from` (promotion), plus per-type typed rels (`depends-on`,
   `contains`, `child-of`). Otherwise an open vocabulary.
4. **Resolution is the index's job, not the parser's.** Envelope `to`/`aliases`
   may be unresolved (alias or ULID); the resolver maps them.
5. **Type-agnostic ULID** → `--pop` move preserves identity across a type change;
   inbound links survive promotion.

### Examples

Inline — a todo at `todos/01J9Z3K7Q2W8XR4M6N0V5BYHED.md`:

```markdown
---
id: 01J9Z3K7Q2W8XR4M6N0V5BYHED
type: todo
time: 2026-06-19T09:14:03-05:00
aliases: [buy-milk]
state: open
due: 2026-06-22
depends: [[01J9Z2QH8M...]]
---
Buy milk for the week. See [[grocery-plan]] for brands.
```

Envelope (one NDJSON line; pretty-printed here):

```json
{
  "v": 1,
  "ulid": "01J9Z3K7Q2W8XR4M6N0V5BYHED",
  "type": "todo",
  "time": "2026-06-19T09:14:03-05:00",
  "aliases": ["buy-milk"],
  "props": { "state": "open", "due": "2026-06-22" },
  "body": "Buy milk for the week. See [[grocery-plan]] for brands.",
  "fragments": [],
  "links": [
    { "rel": "depends-on", "to": "01J9Z2QH8M..." },
    { "rel": "references", "to": "grocery-plan" }
  ],
  "loc": { "file": "todos/01J9Z3K7Q2W8XR4M6N0V5BYHED.md" }
}
```

Routing: `state`/`due` → `props`; `depends` (ref-valued prop) → a `depends-on`
edge; body `[[grocery-plan]]` → a `references` edge with an *alias* target the
resolver resolves later.

Group file — a log day `log/2026-06-19.md` parses to **N+1 nodes**: one
`log-day` node (alias `2026-06-19`) plus one `log-entry` node per timestamped
entry, each with its own ULID. The day carries `contains` edges to its entries.
One envelope line per node.

### Parser contract (the keystone, now concrete)

Each tool ships one pair; the core stays generic and calls it:

- `parse(file) -> []Node` — split a file into its nodes, fill
  `ulid/type/time/body/props/fragments/links/loc`.
- `serialize(node) -> inlineText` — the inverse, for writing into the store.

Everything else reduces to that pair:
- **emit** = read node(s) → `serialize` to NDJSON on stdout.
- **import** = read NDJSON from stdin → mint `ulid` if absent → `serialize` into
  the tool's own store.
- **index** = `parse` every file → upsert nodes/edges/FTS/aliases.
- **resolve** = query the index.

## Program boundaries & the `rk` dispatcher

Question: separate binaries vs. one multi-call binary. Resolved via a middle path.

**Reframe (from git's own history):** git *started* as separate executables
(`git-commit` etc. on PATH), then consolidated (~v1.6) into **one multi-call
binary** with builtins, keeping a PATH extension point (`git-<name>`).
Composability comes from the shared object format + command decomposition, **not**
the process boundary. So packaging is an engineering choice, orthogonal to UNIX
composability — and git's trajectory (separate → consolidated) is a cautionary
tale against starting separate.

### Decision — the via media

1. **Decompose into packages.** Each tool is a Go package exposing `Run(args)`
   plus its `parse(file)->[]node` / `serialize(node)->text` pair. Tools are
   libraries first.
2. **Boundary discipline.** Packages talk *only* via the canonical node + the
   index. No package reaches into another's internals. Recovers the
   separate-binaries virtue (enforced honesty) by convention instead of by
   process isolation.
3. **One multi-call binary `rk` by default.** A single `main` imports all
   packages and dispatches `rk <tool> …` git/busybox-style. One install, one
   version → no format skew/drift, shared keystone code, fast (no exec between
   subcommands — matters for the TUI), git-quality UX (`rk help`, completion,
   uniform flags).
4. **`rk-<name>` PATH extension point.** `rk foo` execs an external `rk-foo` when
   no builtin matches. Serves:
   - **environment-specific** modules — e.g. work-only subcommands that differ
     from personal use, added per-environment without bloating the core;
   - **experimental / heavyweight / exotic / polyglot** components.
   These live on PATH, or **graduate** into the main binary once proven — cheap,
   because an external tool already speaks the node format, so wrapping it as a
   package and compiling it in is mechanical. ("Earn their keep.")

### Packaging is deferrable / dual

Because tools are packages, the binary layout is a *build-time* choice: one
`main` importing all = multi-call; N tiny `main`s each wrapping one package =
separate binaries. Same code, either or both targets. **Commit now to the
contract (node format + boundary discipline), not the process count.**

## Relationship to existing systems & to current reckon

### To current reckon — closer than the README suggests

Code inspection (2026-06-19) shows the redesign's DNA already present:
- **Modular packages per domain** — `journal`, `checklist`, `note`, `time`,
  `tui`, `cli`, `storage`, `parser`, `service`. Already decomposed by domain.
- **Time-sortable decentralized inline ID** — `rs/xid` (12-byte, base32,
  time-ordered). ULID's role is already filled; xid→ULID is near-cosmetic.
- **A link graph with backlinks** — `note_links(source, target, target_slug)`.
  Zettelkasten linking already exists, slug-addressed.
- **SQLite index**, per-type tables (`tasks`, `notes`, `note_links`,
  `log_entries`, `intentions`, `wins`, `schedule_items`, `checklist_*`).
- **Slugs** ≈ the alias layer.

**Evolution of the DNA, revolution of the structure.** Delta:

| Redesign change | vs current |
|---|---|
| One unified node + one edges graph across all types | per-type tables; links only note↔note. Generalize `note_links` → universal typed edges |
| All types = plain-text-truth, index disposable | journal is markdown; tasks/notes/checklists appear SQLite-primary. Invert → text truth everywhere (biggest change; verify current truth) |
| Packages behind multi-call `rk` + PATH extensions | one program, no extension point |
| Promotion, block `#frag`, alias resolver, agent-first | none (note-level links only, human-first) |
| xid → ULID | trivial |

**Verdict: a refactor of an already-modular codebase ~60% of the way there, not a
greenfield rewrite.** Salvageable: parser, storage, TUI (→ a view over the
index), xid, `note_links` logic.

### To other systems — borrowed parts, unshipped whole

- **Closest scope/philosophy: org-mode + org-roam** — text; todo+agenda+journal+
  notes+checklists; `org-id`; org-roam keeps a **SQLite link cache** over
  markdown; **capture+refile = promotion**; agenda ≈ graph-query. Differs:
  Emacs-locked, Lisp, not CLI-composable, not agent-first.
- **Closest execution: `zk`** (markdown + SQLite index + link graph), **`nb`**
  (git-backed plain-text CLI multi-tool), **taskwarrior+timewarrior** (separate
  composable binaries; custom store, no graph).
- **Closest data model: Tana / Anytype / Notion** — typed nodes + relations +
  queried views. Differs: proprietary, GUI/cloud, not text/git/CLI.
- **Note-graph GUIs: Obsidian, Logseq** — markdown + links + block refs +
  plugins/queries; cover note+todo+journal in one app. Differ: GUI monolith,
  not pipeable/agent-oriented.

**Novelty verdict:** every factor is borrowed; the *intersection* isn't shipped:
> {plain-text+git truth} × {one typed-node property-graph across **all**
> domains} × {strict UNIX-composable CLI + node-pipe} × {**agent-first**} ×
> {type-agnostic ULID + promotion}

Nearest single thing — "org-roam as composable CLI for agents" — doesn't exist.

### Worth building? — validated by lived experience

User's PKM history maps directly onto the design's bets:

- **org-mode / org-roam** — loved; left due to **Emacs lock-in + no elisp**.
  Still uses **Orgzly** on phone for todos/reminders (great). → validates
  "org-roam data model without Emacs"; mobile plain-text works.
- **Obsidian** — liked the feel, but **too little structure out of the box** →
  decision paralysis (separate note or not? where does it go? how to organize?).
  Couldn't build a *system*. Still uses for long-form wiki notes. → **opinionated
  structure is a feature, not a bug** — the core anti-Obsidian bet.
- **Logseq** — heavy use; loved **append-to-daily-log then branch off to a note**
  (enough structure to feel progress). But **todo tracking falls short**: query
  view → **scheduling → doing is not smooth**; and **no division between
  ephemeral and durable** tasks/reminders (also true in Obsidian). → validates
  the ephemeral/durable split and a smooth query→schedule→do path as core gaps.
- **Sync** — **Syncthing** syncs Logseq + Orgzly + Obsidian files to phone now.
  nvim primary on desktop; **Obsidian as optional mobile GUI**. → plain-text +
  Syncthing is the proven substrate; Obsidian-flavored markdown is *actively
  wanted*, not merely a hedge.

### Derived requirements

1. **Opinionated structure out of the box** — typed tools with clear homes; no
   blank-canvas paralysis. The structure *is* the product (anti-Obsidian).
2. **Daily-log capture + branch-to-note as first-class** — Logseq's loved flow;
   maps onto journal (capture) + promotion (`--ref` log-entry → note, backlinked).
3. **Ephemeral vs durable split** — a felt gap; already core. Keep central.
4. **Smooth query → schedule → do** — Logseq's pain; the todo/schedule tools must
   close the gap from "queried list" to "scheduled & actionable." (current reckon
   already has `schedule_items` + scheduled/deadline — salvage.)
5. **Plain-text + Syncthing mobile; Obsidian-compatible on-disk format.** Only
   text syncs; the **SQLite index is per-device, derived, regenerated — never
   synced** (reinforces "index disposable"). reckon files coexist in a synced
   vault alongside Obsidian/Logseq.
6. **Hard separation between tasks and logs** — explicitly valued in current
   reckon and kept by the decomposition (separate tools/types). A log entry is
   not a task; promotion bridges them *explicitly* when needed. User-confirmed
   core value; the module split enforces it structurally.

## Design: `query → schedule → do` UX (proposed — punch list A#1)

> Status: **DECIDED 2026-06-21** (three sub-decisions confirmed below). The
> highest-leverage open design — the specific pain that pushed the user off Logseq.

**The loop:** capture → surface → triage/schedule → do → review. Capture
(log/ephemeral) and review (the log) are other tools; the gap is the middle three
and the *transitions* between them.

**Root cause of the Logseq friction:** the query is a **read-only list** (view ≠
actuator) and scheduling is **absolute/manual** — so every transition is a
surface-switch + hand-edit + re-query.

**Criteria:** fewest surface-switches (ideal: one) · fewest keystrokes per
transition · relative > absolute scheduling · view *is* the actuator ·
agent-assistable triage · CLI/composable underneath · closes the loop into the
log · keyboard-first.

**Candidates considered:** (A) pure CLI verbs — composable but per-command
friction, no live surface; (B) live agenda TUI; (C) calendar/time-block —
high-friction; (D) kanban — heavier for keyboard; (E) edit files directly — the
friction being fled. Gold standard = **org-agenda** (the user loved it):
aggregate + schedule + mark-done in one buffer, single keys, never leaving.

### Proposal — `rk today`: a live agenda, agent-primed, complete-as-logging

A porcelain **view-tool** over read-glue (graph-query) + writes to the todo tool.
Three motions, each ≤1 keystroke, one surface:

- **Surface** — `rk today` queries overdue + scheduled-today + today-pinned +
  today's reminders. The **agent optionally pre-fills a proposed plan** (orders
  the list, suggests do-dates) → kills blank-list triage paralysis while the user
  stays in control.
- **Schedule** — in-list relative keys: `t`=today-pin · `d`=defer
  (tomorrow/next-week/pick) · `D`=deadline · `p`=priority; writes straight to the
  task file. **Model = org's:** a `scheduled` do-date + a hard `deadline` (the
  distinction Logseq lacked) + a today-pin. No bucket taxonomy to learn —
  "buckets" are just views over do-dates.
- **Do** — `x`=done · `i`=in-progress · `c`=cancel. Completion **emits a
  `log-entry` node linked `did`→task** (via the promotion machinery — two linked
  nodes, not a merge, so the hard task/log separation holds), closing the loop
  into the journal and giving time-tracking for free. Default on, toggleable.

**Why lowest-friction:** one surface (no view→edit→re-query round-trips),
keyboard verbs, relative scheduling, agent removes triage paralysis, doing feeds
the log/review automatically.

**Why it fits:** porcelain over {graph-query read} + {todo-tool write}, in-process
in the multi-call `rk`; **reuses current reckon's existing TUI task list**. CLI
verbs (candidate A) sit *underneath* as the plumbing the TUI and the agent both
call — same verbs, two surfaces. Embrace & extend: org-agenda reborn outside
Emacs.

**Sub-decisions (confirmed 2026-06-21):**
- **Scheduling model** = org `scheduled` (do-date) + `deadline` (hard) +
  today-pin. No bucket taxonomy; Today / Later / Someday are *computed views*
  over do-dates.
- **Agent daily planning** = propose-and-confirm (`[a]ccept / [e]dit / [r]eject`).
  The agent never silently mutates the plan.
- **Completion** = emits a linked `log-entry` (`did`→task) by default,
  toggleable. Two nodes + one edge — preserves the hard task/log separation.

## Design: agent query surface — `rk query` (decided — punch list A#2)

> Status: **DECIDED 2026-06-21.**

Single read surface over the shared index. Per-tool list commands (`rk todo ls`)
are thin sugar over it; an MCP `query()` tool is a thin wrapper over it (same
engine, two surfaces).

Decisions:
- **Language = SQL.** Agents are already strong at SQL — beats inventing and
  maintaining a custom DSL, and gives recursive-CTE graph traversal for free.
- **Against a stable named-view layer, not physical tables.** Public views
  (`nodes`, `edges`, `node_props`, `aliases`, `fts`); physical tables stay
  private. Plumbing/porcelain for the index — refactor storage freely, keep the
  query contract stable, avoid schema-coupling breakage.
- **Read-only.** `rk query` opens the index read-only (`query_only` / `mode=ro`)
  and rejects non-SELECT. Writes go only through the tools + promotion. **Query
  reads, tools write.**
- **Output = canonical node NDJSON by default** (composes with the pipe:
  `rk query … | rk note import`; feeds the agenda + agent), plus a **raw-rows
  mode** for aggregates/projections.
- **Saved views = named, versioned text files** (`rk query --view daily`), shared
  by the agenda and the human; git'd + synced.
- **Language-agnostic transport** — `rk query --lang sql|…` defaulting to SQL, so
  Cypher/Datalog can slot in later behind the same surface without changing
  callers (honors "don't paint into a corner").

## Design: index freshness & rebuild (decided — punch list A#4)

> Status: **DECIDED 2026-06-21.**

Files change via many paths, several out-of-band (nvim, Obsidian, Syncthing, git)
that reckon can't observe directly. The index is disposable/per-device, so favor
robustness over minimal-work cleverness.

**Decision: lazy reconcile on read + write-through for reckon's own changes +
explicit `rk index` for full rebuild.** Watcher deferred.

- **Lazy on-read (correctness backstop):** before `rk query` / agenda load, a
  fast reconcile — stat-walk, reparse only changed/new files, drop deleted.
  Catches every source uniformly, no daemon. The *only* trigger that catches
  Syncthing (not git-mediated, and a watcher is off when the device sleeps).
- **Write-through (optimization):** reckon tools update the index inline on write
  → instant agenda; the next reconcile then finds nothing to do. Not required for
  correctness.
- **Explicit `rk index`:** full rebuild from text (recovery / schema migration).

**Change detection = hash-authoritative.** mtime is a fast-path to skip unchanged
files, but the content `hash` is the authority (mtime is untrustworthy across
git/Syncthing).

**Index state stored in SQLite (two levels):**
- **Per-file meta table** `(path, ulid-set, hash, mtime)` — *the* change-detection
  authority; reconcile compares each file's stored hash/mtime.
- **Global `index_meta`** (key/value): `schema_version`, `last_reconcile_at`,
  `last_full_rebuild_at`, `vault_id`, `builder_version` — for display, debounce,
  and migration, **not** correctness.
- `schema_version` drives **auto-rebuild**: when reckon's index schema > the
  stored version, do a full `rk index` automatically (no index migrations — it's
  derived).

**Edge cases decided:**
- **Rename/move = free** — reconcile keys nodes by inline **ULID**, just updates
  `loc`; reorganizing folders never breaks links.
- **Index location** — a local cache dir (`~/.cache/reckon/<vault-id>/index.db`),
  **never inside the vault, never synced** (a synced live SQLite file = corruption).
- **Ignore globs** — `.sync-conflict-*`, `.stversions/`, `.git/`, `.obsidian/`,
  `.reckon/`.
- **Malformed files** (git conflict markers, Syncthing conflict copies) — parser
  tolerates/skips and logs, never crashes the reconcile.
- **Concurrency/atomicity** — WAL mode + a single reconcile-writer lock; each
  reconcile in a transaction.

## Design: link syntax (A#5) & alias namespace (A#6) — decided

> Status: **DECIDED 2026-06-21.**

### Link syntax (A#5)

Obsidian-compat dictates the wikilink *surface*; reckon decides the rest.
- **Adopt Obsidian as-is:** `[[target]]`, `[[target|Label]]`, `[[target#Heading]]`,
  `[[target#^block]]`, `^block` anchor, `![[target]]` embed, `#tag`.
- **Typed edges live in frontmatter props** (`depends: "[[ULID]]"` → a
  `depends-on` edge). Anything inside `[[ ]]` is a link target to Obsidian, so an
  inline typed-link syntax would break Obsidian resolution. **Body `[[ ]]` =
  generic `references` edge.**
- **Alias-preferred links.** reckon aliases == Obsidian frontmatter `aliases:`, so
  `[[my-note]]` resolves in both; ULID (`[[01J…]]`) is the durable fallback
  (filename = ULID → Obsidian resolves it too).
- **Block fragments use Obsidian `#^frag`** — the earlier `#` delimiter decision
  stands; the block anchor carries `^` to be Obsidian-native. Headings link via
  `#Heading`.
- Tags (`#tag`) → a `tags` prop; embeds (`![[ ]]`) → a `references` edge + embed
  flag. Pass-through; Obsidian renders them.

### Alias namespace & dangling links (A#6)

- **Global / flat namespace** (vault-wide, matches Obsidian). The index **enforces
  uniqueness** (which Obsidian can't): a colliding alias mint is rejected /
  auto-disambiguated.
- **Auto + explicit:** each tool auto-mints a canonical alias (note slug from
  title, journal day = date); the user may add more. All non-authoritative over
  the ULID.
- **Rename = retain old as redirect.** On slug/alias change, the old alias is kept
  in frontmatter `aliases:` → both reckon and Obsidian still resolve `[[old]]`;
  **zero file churn** (no mass link rewrites — safe across Syncthing/git devices).
- **Dangling links allowed** — `[[not-yet-created]]` is a forward-reference,
  stored as an **unresolved edge** that **auto-resolves when the target is
  created** (reindex), and is **queryable** (`rk query` for "unresolved"). Matches
  Obsidian (allow + click-to-create).
- **Scope/charset:** aliases are **node-level only** (fragments addressed
  structurally); case-insensitive match, spaces allowed, forbid link-control chars
  (`# | [ ] ^`).
- **Late-binding (Obsidian parity).** reckon's alias = Obsidian's *frontmatter*
  `aliases:` (a real alternate name that resolves to the page), **not** the
  cosmetic `[[page|display]]` form (that has no graph meaning in either tool).
  Links are stored as the **authored alias text** and resolved **late** via the
  index (edges are recomputed from text on every reindex) — exactly like
  Obsidian's name lookup. **Rule: never rewrite `[[alias]]` → `[[ULID]]` in
  files** (early-binding would diverge from Obsidian and destroy readability).
  Deltas vs Obsidian, both benign: reckon enforces alias *uniqueness* (Obsidian
  permits ambiguity → reckon is a strict subset, always Obsidian-valid); a
  human-introduced duplicate alias from Obsidian must be **flagged gracefully** on
  reindex, not crash.

## Design: scheduling, reminders & recurrence (A#3) — decided

> Status: **DECIDED 2026-06-21.**

Builds on #1 (`scheduled` do-date + `deadline` + today-pin, org model). All
scheduling data lives in **frontmatter props**, plain-text, org-aligned, surfaced
in `rk today`.

### Reminders (A#3a)
- A reminder is a **time-trigger** that can **stand alone** (an ephemeral
  `reminder` node) **or attach** to a durable node (a `remind:` prop). Same firing
  mechanism, flexible carrier — fits the ephemeral/durable split.
- **Delivery is delegated (daemonless).** reckon stores + surfaces due reminders
  (queryable); actual notification is external — desktop cron/systemd/`at` calling
  `rk remind due → notify-send`; mobile = whatever app reads the files
  (Orgzly/Obsidian). This is the first concrete **hook**: `on-reminder-due` (see
  the Hooks section).

### Recurrence (A#3b/c)
- **Syntax = org repeaters** on `scheduled`/`deadline`: `+1w` (fixed), `++1w`
  (skip to next future), `.+3d` (N after completion). Compact, plain-text, the
  user already knows org. Optional iCal `RRULE` escape hatch for complex rules.
- **Advance model = virtual occurrences that materialize on need** — dissolves the
  advance-in-place vs. instance-generation dichotomy:
  - A recurring rule is a **durable node** that **projects** occurrences.
  - Occurrences are **virtual** — computed from `repeat:` + the **log's `did`
    entries as the done-cursor** (no single mutable date to hand-manage; the
    auto-logged completions from #1 *are* the history). `rk today` shows a virtual
    occurrence as an actionable row; completing it logs a `did` and the projection
    advances. Advance-in-place's tidiness for trivial chores (water plants stays
    virtual, zero clutter).
  - An occurrence **materializes** into a real **ephemeral task instance**
    (promotion: rule → instance) **lazily, on need**: when it (a) gets a
    note/subtask/link, (b) **piles up** (a new occurrence comes due while the prior
    is still open → both coexist), or (c) is pinned/kept. This is the "durable rule
    emits ephemeral instances" model, for substantive/stacking occurrences.
  - **Checklists always materialize a run** (decided earlier) = the
    always-materialize end of the same spectrum.
  - One mechanism, per-tool defaults. Cost: a projection function (expand
    `repeat:` using the log as cursor) + the lazy materialize trigger. **No
    generator daemon.**

### Timezones (A#3d)
- **Hybrid:** do-dates (`scheduled`/`deadline`) = **date-only, floating** (a
  calendar day); log/event timestamps (the node `time` field) = **RFC3339 with
  offset** (the real instant); **reminder times = wall-clock local** ("9am wherever
  I am"), optional pinned zone. Matches human intent + the org/iCal
  floating-vs-absolute split.

### Why this beats org's one-entry-with-drawer
The type decomposition splits what org crams into a single ever-expanding entry +
`:LOGBOOK:` drawer into first-class pieces: the **durable rule** (the recurring
task), the **ephemeral instance** (when scheduled/due), and the **log** (when
done). Each its own queryable node type — no monolithic drawer.

## Design: hooks (reserved extension seam)

> Status: **seam reserved; realize `on-reminder-due` now, defer the rest (YAGNI).**

The reactive glue deferred earlier (event bus) finds its **daemonless, git-style
form here**: user scripts that fire **synchronously on lifecycle events**,
configured in a local `.reckon/hooks/` dir (embrace & extend — exactly git's
pattern). Lighter than a pub/sub bus; no daemon. The deferred-bus note ("easy to
add later if writes stay observable") — this is that later form.

The pattern is already latent: **complete→log**, **promotion**,
**materialize-on-need**, and **write-through index** are all implicit hook points.

**Candidate events** (taxonomy, for when needs arise): `post-create` /
`post-update` / `pre`/`post-delete`; `post-complete`; `post-promote`;
`on-materialize`; `on-reminder-due`; `on-occurrence-due`; `pre`/`post-reconcile`;
`pre-write` (validation/reject).

**Realize now:** `on-reminder-due` — it's how reminder delivery is delegated
(`notify-send` etc.). Defer all others until a concrete use appears.

**Cautions (decided constraints):**
- **Local / per-device, not synced** (like `.git/hooks`). Good for *side effects*
  (notify, sync-out); **be wary of vault-mutating hooks** — they can fight
  Syncthing / diverge per device.
- **Invocation-driven, not ambient** — no daemon, so time-based hooks
  (`on-reminder-due`, `on-occurrence-due`) only fire when reckon runs; they need an
  external **cron tick** (`rk tick` / `rk remind due`).
- **Opt-in, visible, logged** — hooks make behavior non-local; keep them
  discoverable and traceable.

## Remaining design punch list

Split by needs-a-decision vs decided-needs-spec vs deferred. Group A is the real
remaining *design* work; B is downhill from the keystone; C needs no action now.

### A. Open design decisions (a call is needed)

1. ~~`query → schedule → do` UX~~ — **Decided 2026-06-21:** `rk today`, a live
   agenda surface (org-agenda reborn); three sub-decisions confirmed (org
   scheduling, propose-and-confirm agent, complete→linked-log default). See the
   "Design: query → schedule → do UX" section.
2. ~~Agent query surface~~ — **Decided 2026-06-21:** `rk query`, SQL over a
   stable named-view layer, read-only, node-NDJSON output (+ raw-rows mode),
   versioned saved views, `--lang` for future languages, MCP as a thin shim. See
   the "Design: agent query surface" section.
3. ~~Scheduling / reminders / recurrence model~~ — **Decided 2026-06-21:**
   reminders = standalone-or-attached trigger (delivery delegated); recurrence =
   org repeaters; advance = virtual occurrences materializing lazy-on-need;
   timezones = hybrid (floating dates / absolute events / local reminders). See
   "Design: scheduling, reminders & recurrence".
4. ~~Index freshness / rebuild model~~ — **Decided 2026-06-21:** lazy reconcile
   on read + write-through for reckon's own changes + explicit `rk index` full
   rebuild; hash-authoritative detection; per-file + `index_meta` state in
   SQLite; index in a local cache dir, never synced; watcher deferred. See
   "Design: index freshness & rebuild".
5. ~~Link syntax finalization~~ — **Decided 2026-06-21:** adopt Obsidian's
   wikilink surface; typed edges in frontmatter props (body `[[ ]]` = generic
   `references`); alias-preferred links, ULID fallback; block frags `#^frag`. See
   "Design: link syntax & alias namespace".
6. ~~Alias namespace + dangling-link semantics~~ — **Decided 2026-06-21:** global
   flat namespace, index-enforced uniqueness; rename = retain old alias as a
   redirect (zero churn); dangling links allowed, auto-resolve on creation,
   queryable. See "Design: link syntax & alias namespace".

### B. Specs to write (decided in principle, detail pending)

7. **Index schema** — `nodes`/`edges`/`fts`/`aliases` columns; props storage
   (JSON column vs EAV); fragment + backlink indexing.
8. **Per-tool specs** — one each: **log**, **todo**, **note**, **ephemeral**,
   **checklist** (template+run); plus decide **time-tracking** (separate tool, or
   fold into log?). Each = file layout + props + parse/serialize + the
   opinionated structure it imposes.
9. **Resolver spec** — alias→ULID, `[[ref]]`→file, `ULID#frag`→span (ties to
   #6/#7).
10. **Migration plan** — current reckon (per-type tables, xid, `note_links`,
    journal md) → unified node+edges; xid→ULID (preserve old IDs as aliases?);
    salvage parser/storage/TUI.
11. **Config / repo & vault layout** — where files live, where the per-device
    gitignored index lives, how tools locate store+index, coexistence with
    Obsidian/Logseq in a synced vault.

### C. Deferred / acknowledged (no action now)

12. Reactive event bus — **reframed as git-style hooks** (daemonless); seam
    reserved, `on-reminder-due` realized, rest deferred. See "Design: hooks".
13. Attachments / binary (base64-field or `loc`-reference escape hatch).
14. Budgeting module via hledger/format (future; embrace & extend).
15. Multi-level fragment nesting (only if needed).
16. `storage == interchange` alternative (considered; declined unless "one
    format" appeals).

**Critical path:** **all of Group A decided.** Remaining = Group B specs
(mechanical projection of the canonical node) + Group C deferred. Hooks reserved
as a seam (realize `on-reminder-due`, defer the rest). Tracking issue:
`reckon-53fu`.

## Principle: embrace & extend (reuse existing tools/formats)

A value that emerged across the design: **prefer reusing and interoperating with
established, widely-used programs and formats over building from scratch.** A
small effort spent on compatibility buys large, battle-tested functionality and
keeps the user's existing tools working.

Instances already in the design:
- **git** as the state-over-time / history / undo substrate (not a custom log).
- **Obsidian-flavored markdown** on disk → existing editors, mobile GUI, and the
  Syncthing workflow keep working for free.
- **SQLite** as the index engine; **ULID** as a standard ID.

Forward-looking: a budgeting / purchases module would heavily consider
**hledger** (or at least its plain-text-accounting format) rather than inventing
one. New modules should ask buy-vs-build first and lean toward adopting an
existing format/tool and extending it.

Framing: **embrace & extend — *not* extinguish.** Interoperate, don't capture;
the adopted formats stay usable by their original tools.

Why it's cheap here: each tool already owns a `parse`/`serialize` pair and plugs
in as a package or an `rk-<name>` PATH extension — so adopting an external format
= writing one parser, and adopting an external tool = wrapping it behind the node
contract.

## Decision log

### 2026-06-19 — Integration model = graph-query (read glue)
Composition happens via querying a shared index, not linear pipes. Rationale:
tools are heterogeneous (checklist ⇏ note), and the goal is unified cross-type
views. Pipes may still serve narrow same-type cases.

### 2026-06-19 — Query flavor: deferred; lock in the *index model*, not the dialect
Reframe driven by two user constraints: (a) an agentic layer (local or Claude)
will author queries, so human typing ergonomics are irrelevant; (b) must not
paint into a corner by excluding future functionality.

Key insight: the corner-painting risk lives in the **index data model**, not the
query dialect. Dialect is a swappable projection on top of a rich-enough model.

**Decision:** model the index as a **property graph** — nodes (stable id, type,
time, arbitrary key/value props, full-text body) + **typed, directional edges**
with a **backlink index** — persisted in SQLite (reuse existing
markdown-truth → SQLite-index split). Query dialect chosen later / multiple
allowed:
- SQLite **recursive CTEs already give arbitrary-depth graph traversal** — no
  separate graph DB needed to keep the traversal option open.
- Can later layer Cypher/Datalog, or expose an **MCP tool surface** for the
  agent, without changing storage.

**Anti-corner checklist (things to NOT do, each forecloses future queries):**
- untyped / implicit links (lose edge semantics)
- one-way links without a backlink index (can't cheaply ask "what links to me")
- structured fields flattened into prose (lose structured filtering)
- no stable IDs (can't reference durably)
- type encoded only by file location (hard to reclassify / query by type)

### 2026-06-19 — Reactive glue (event bus): deferred (YAGNI)
Not in the initial design; views are read-only. Confirmed cheap to add later —
**provided writes stay observable.** Cheap insurance to preserve the option
without building it now: keep created/modified timestamps on items, and/or build
the store on git (hooks can fire on commit) and/or make index builds
incremental. Don't make writes invisible; that's the only thing that would make
a future event bus expensive.

### 2026-06-19 — Atom terminology + per-tool scope
Split the overloaded "atom": **file** = storage atom, **node** = addressable
atom; "atom" retired as a system term. A file holds 1..N nodes; **each tool
ships its own file→node parser** so the core indexer stays generic. Granularity
governed by two rules: *addressability = durability*, and *group
high-volume/low-value/short-lived, isolate durable/linked/history-worthy.*
Per-tool scope table recorded in the "Atoms" section. Ephemeral tasks
deliberately get **no** stable address; promotion to a durable todo is the
keep-it bridge.

### 2026-06-19 — Event log (substrate option B): killed
The append-only event log is **out**. Property-graph-in-SQLite + git together
cover the history/time-travel needs that motivated it (git = history/undo;
SQLite index = current-state queries; timestamps + day-files = temporal log).
Not worth the cost (not greppable, projection layer, heavier). Substrate is
plain-text files (option A/C blend) + derived property-graph index.

### 2026-06-19 — Node ID scheme: research logged, leaning ULID + aliases (pending)
Block-level links inside items confirmed as a requirement. Prior art + lessons
recorded in the "Node ID scheme" section. Leaning: canonical ULID stored inline,
decoupled from title/path/content; block `^id` for intra-item anchors; human
aliases (date/slug) resolving to ULIDs; type kept as a property, never in the
ID (because the promotion path reclassifies). Locked later same day — see the
LOCKED entry below.

### 2026-06-19 — Node ID scheme: LOCKED
ULID = canonical node identity. A file holds 1..N nodes, each with its own ULID
inline (file-per-item → frontmatter `id:`; group file → one ULID per contained
node + the container is its own node). Block-level addressing = node-local
sub-IDs appended URL-fragment style (`ULID#frag`), unique only within the node,
**stable tokens not positional**, assigned **only when a block is linked**.
Human aliases (date/slug/time) allowed, non-authoritative; resolver maps
alias → ULID; links written as `[[ULID]]`, `[[ULID#frag]]`, `[[alias]]`,
`[[alias#frag]]`. Type stays a property. The per-tool file→node parser owns the
file-splitting and ULID assignment.

### 2026-06-19 — Fragment delimiter: `#` (consciously chosen)
Node→fragment delimiter = `#` (`ULID#frag`). Parsing is unambiguous for *any*
punctuation (ULIDs are punctuation-free Crockford base32), so the call is
ergonomics. `#` wins on web-fragment semantics (`resource#frag`), wikilink
precedent, and shell-safety in composite position (`a#b` is not a comment).
Rejected: `|` and `>` as shell metacharacters (`|` also collides with the
wikilink display-text delimiter); `/` as filesystem-path-conflating. A single,
non-repeatable boundary fits the flat 2-level (node + fragment) model.

### 2026-06-19 — Promotion path: pipe with emit-side selectable disposition
Inter-tool node move uses a pipe: `rk <src> --<disp> <ID> | rk <dst> --import`.
Disposition selected per-operation by the emit verb: `--ref` (non-destructive,
new ULID, `derived-from` edge, source backlink via index) or `--pop` (consume;
source tool deletes from its own store; existing ULID **preserved** — works
because type is a property, not in the ID, so inbound links survive). Generic
`--import` consumer. Invariant: each tool writes only its own store; cross-links
are edges, reverses via the index. Interchange = NDJSON node envelope = the
file→node parser's serialized output. Atomicity via git-history recovery (raw
pipe) or a transactional `rk promote` dispatcher verb. Confirms the integration
split: **graph-query = read/heterogeneous, pipe = move/homogeneous.** Event bus
stays deferred.

### 2026-06-19 — Canonical node spec'd (keystone)
One artifact serves parser output, promotion interchange, index input, and
resolver input. Two faces: **inline** (authored truth) and **NDJSON envelope**
(parser-normalized superset with derived `loc`/`hash`/`mtime`/`v`). Fields:
`ulid` (type-agnostic), `type` (a prop), `time` (semantic), `body`, `aliases`,
`props` (per-type open bag), `fragments` (node-local), `links` (forward typed
edges fed by ref-props + body `[[..]]`), plus env-only `loc`. Invariants: nodes
store **forward/authored facts only** — all reverse/aggregate is index-derived,
never stored; inline ULID is truth; resolution is the index's job. **Parser
contract** = `parse(file)->[]node` + `serialize(node)->text` per tool;
emit/import/index/resolve all reduce to that pair. Settles open-Q #5 (interchange
format) and the parser-contract thread. Judgment defaults (vetoable): `time`
semantic+required; envelope `v` version; open rel vocab w/ reserved set;
day→`contains`→entries; typed edges from props + generic from body (`[[rel:target]]`
syntax deferred).

### 2026-06-19 — Program boundaries: packages + one multi-call `rk` + PATH extensions
Resolved separate-vs-single via a middle path, informed by git's history
(separate execs → consolidated multi-call builtin + PATH extension). Each tool =
a Go package (`Run` + parse/serialize); packages talk **only** via node + index
(boundary discipline = enforced honesty by convention). Default ship = one
multi-call `rk` binary (no skew, shared core, fast for the TUI, good UX).
`rk-<name>` PATH extension point handles environment-specific (work vs personal),
experimental, heavyweight, exotic, or polyglot modules — which can live on PATH
or graduate into the core once proven. Packaging (one main vs many) is a
build-time choice deferred by the package structure; the committed thing is the
contract, not the process count.

### 2026-06-19 — Assessment: build is warranted; ~60% already in reckon
The redesign targets gaps the user has personally hit and not solved in org-roam
(Emacs lock-in), Obsidian (too generic → paralysis), or Logseq (weak
todo→schedule→do flow, no ephemeral/durable split). No existing system ships the
intersection (text+git × unified typed-node graph × UNIX-composable CLI ×
agent-first × ULID+promotion). Current reckon already has modular packages, xid
(≈ULID), `note_links` (a link graph), a SQLite index, and slugs — so this is a
refactor, not a greenfield rewrite. Build warranted.

### 2026-06-19 — On-disk format: Obsidian-flavored markdown; index never synced
Adopt Obsidian-compatible markdown (`[[links]]`, `^block`, frontmatter) so the
user's existing nvim-desktop + Obsidian-mobile + Syncthing workflow keeps working
and the parser rides a format real editors already render. Only plain-text files
sync (git + Syncthing); the SQLite index is per-device, regenerated from text,
**never synced** — consistent with the index being disposable/derived.

### 2026-06-21 — Principle: embrace & extend existing tools/formats
Captured an emergent cross-cutting value: prefer reusing/interoperating with
established programs and formats (git for history, Obsidian-flavored markdown,
SQLite, ULID) over bespoke builds; a future budgeting module would adopt hledger
or its plain-text format. Buy-vs-build leans toward **embrace & extend (not
extinguish)** — interoperate so adopted formats stay usable by their original
tools. Cheap here because each tool is a `parse`/`serialize` package or an
`rk-<name>` extension behind the node contract. See the "Principle: embrace &
extend" section. (Also noted: NDJSON escapes body newlines as `\n`, so every node
serializes to one physical line — a uniform stream format for all node types.)

### 2026-06-21 — query→schedule→do UX: proposed `rk today` live agenda
Proposed approach for punch-list A#1 via a structured design pass (decompose →
locate friction → criteria → candidates → evaluate). Root cause of the Logseq
pain: view ≠ actuator + absolute/manual scheduling. Proposal = `rk today`, an
org-agenda-style live agenda TUI (the model the user loved) that collapses
surface + schedule + do into one keyboard-driven surface: graph-query surfaces
tasks (agent optionally pre-plans), in-list relative keys schedule (org
`scheduled`+`deadline`+today-pin), single keys do, and completion emits a
`log-entry` node linked `did`→task (promotion machinery — preserves the hard
task/log separation while closing the loop into the journal). Porcelain over
{graph-query read}+{todo write}, in-process in multi-call `rk`, reusing the
existing TUI task list; CLI verbs are the shared plumbing for both TUI and agent.
Pending three sub-decisions (scheduling model, agent auto-plan aggressiveness,
complete→log default). See the "Design: query → schedule → do UX" section.

### 2026-06-21 — Agent query surface: `rk query`, SQL over stable views, read-only
A#2 decided. Single read porcelain `rk query` over the shared index. Language =
SQL (agents already fluent; recursive CTEs give traversal; no custom language to
build/maintain), executed against a **stable public view layer** (`nodes`,
`edges`, `node_props`, `aliases`, `fts`) — physical tables private, so storage
can be refactored without breaking queries. **Read-only** (query reads, tools
write). Output = node NDJSON by default (+ raw-rows mode). Saved views = named
versioned text files. Transport language-agnostic via `--lang` for future
Cypher/Datalog. MCP `query()` = thin wrapper. Per-tool list commands = sugar over
`rk query`.

### 2026-06-21 — query→schedule→do: sub-decisions confirmed
All three sub-decisions confirmed (recommended defaults): scheduling = org
`scheduled`+`deadline`+today-pin (buckets are computed views, no taxonomy); agent
planning = propose-and-confirm (never silent mutation); completion = linked
`log-entry` (`did`→task) by default, toggleable. A#1 fully decided.

### 2026-06-21 — Index freshness: lazy-on-read + write-through + explicit rebuild
A#4 decided. Lazy reconcile on read is the universal backstop (the only trigger
that catches Syncthing/Obsidian/nvim/git uniformly, no daemon): stat-walk,
reparse the delta, drop deleted. Write-through on reckon's own writes is an
optimization for an instant agenda. Explicit `rk index` does a full rebuild.
Change detection is hash-authoritative (mtime only a fast-path; untrustworthy
across git/Syncthing). Index state in SQLite = per-file `(path, ulid-set, hash,
mtime)` (correctness authority) + global `index_meta` (`schema_version`,
`last_reconcile_at`, `last_full_rebuild_at`, `vault_id`, `builder_version`) for
display/debounce/migration, not correctness; a `schema_version` bump triggers an
auto full-rebuild (no index migrations — derived). Rename/move free (nodes keyed
by inline ULID, `loc` updated). Index in a local cache dir, never in the vault,
never synced. Watcher deferred.

### 2026-06-21 — Link syntax (A#5): Obsidian surface + typed edges in frontmatter
A#5 decided. Adopt Obsidian's wikilink surface wholesale (`[[t]]`, `|` display,
`#Heading`, `#^block`, `![[ ]]`, `#tag`). Typed edges live in **frontmatter
props** (Obsidian owns the `[[ ]]` interior, so inline typed-link syntax would
break it); body `[[ ]]` = generic `references`. Links prefer aliases (== Obsidian
`aliases:`) with ULID as the durable fallback (filename=ULID). Block fragments use
Obsidian `#^frag` (the `#` delimiter stands; block anchor carries `^`). Tags →
`tags` prop, embeds → reference edge + flag.

### 2026-06-21 — Alias namespace & dangling links (A#6)
A#6 decided. Global/flat alias namespace (Obsidian-matching); index enforces
uniqueness. Auto canonical alias per tool + user-added; non-authoritative over the
ULID. Rename = retain old alias as a redirect in frontmatter `aliases:` → both
reckon and Obsidian keep resolving `[[old]]`, zero file churn (Syncthing-safe; no
mass rewrites). Dangling links allowed as forward-references — stored as
unresolved edges, auto-resolve on target creation, queryable. Aliases node-level
only; case-insensitive, spaces allowed, forbid `# | [ ] ^`.

### 2026-06-21 — Scheduling/reminders/recurrence (A#3) decided
Builds on #1's org `scheduled`/`deadline`/today-pin; all scheduling in frontmatter
props. Reminders = a time-trigger that stands alone (ephemeral `reminder` node) or
attaches (`remind:` prop); delivery delegated/daemonless (`on-reminder-due` hook).
Recurrence syntax = org repeaters (`+`/`++`/`.+`) + optional RRULE. Advance model =
**virtual occurrences that materialize lazily on need** (note/link, pile-up, or
pin), projected from `repeat:` using the log's `did` entries as the done-cursor;
checklists always materialize a run — one mechanism, per-tool defaults, no
generator daemon. Timezones = hybrid (floating date-only do-dates; absolute
RFC3339-with-offset event/log timestamps; wall-clock-local reminders). Conceptual
win: the type decomposition replaces org's one-entry-with-`:LOGBOOK:`-drawer with
first-class rule / ephemeral-instance / log nodes.

### 2026-06-21 — Hooks reserved as a daemonless extension seam
The deferred reactive event bus is reframed as **git-style hooks** — synchronous
user scripts on lifecycle events, configured in `.reckon/hooks/` (embrace &
extend). Already-latent hook points: complete→log, promotion, materialize-on-need,
write-through index. Decision: **reserve the seam, realize only `on-reminder-due`
now** (reminder delivery), defer the rest (YAGNI). Constraints: hooks are
local/per-device and not synced (good for side-effects, wary of vault-mutating
ones that fight Syncthing); invocation-driven, not ambient (time-based hooks need
a cron tick); opt-in, visible, logged.

## Parking lot / notes

- Existing reckon already uses plain markdown as source of truth + SQLite for
  query/aggregation (see README). That existing split (text = truth, DB = index)
  is itself a hint: text substrate + derived query layer. Worth reconciling with
  the redesign rather than discarding.
