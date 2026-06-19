# Reckon Redesign — Composable, UNIX-style System

> Status: **exploratory / iterating**. Living design record. Not a decision yet.
> Started: 2026-06-19

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

## Parking lot / notes

- Existing reckon already uses plain markdown as source of truth + SQLite for
  query/aggregation (see README). That existing split (text = truth, DB = index)
  is itself a hint: text substrate + derived query layer. Worth reconciling with
  the redesign rather than discarding.
