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

1. What is the atom, per tool? (file / file-per-group / line / event) — and is a
   mixed model acceptable, or does mixing break the "common substrate" promise?
2. Does history/undo matter enough to pay for an event log (B), or is git history
   enough?
3. Is linking the *core* feature (everything-is-a-node) or just notes-link-notes?
4. ~~What does cross-tool composition look like concretely?~~ **Resolved
   2026-06-19:** graph-query over a shared property-graph index; views = saved
   queries; agent authors them. See Decision log.
5. ~~Interchange format when tools talk?~~ **Mostly moot 2026-06-19:** the shared
   index *is* the integration; tools don't pipe to each other. If a same-type
   pipe case ever arises, decide format then (likely JSON lines).

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

## Parking lot / notes

- Existing reckon already uses plain markdown as source of truth + SQLite for
  query/aggregation (see README). That existing split (text = truth, DB = index)
  is itself a hint: text substrate + derived query layer. Worth reconciling with
  the redesign rather than discarding.
