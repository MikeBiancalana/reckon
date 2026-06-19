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
4. What does cross-tool composition look like concretely — a query language? a
   `reckon` meta-command that resolves links and renders views? saved views?
5. What is the interchange format when tools DO need to talk (JSON lines? plain
   text records?).

## Parking lot / notes

- Existing reckon already uses plain markdown as source of truth + SQLite for
  query/aggregation (see README). That existing split (text = truth, DB = index)
  is itself a hint: text substrate + derived query layer. Worth reconciling with
  the redesign rather than discarding.
