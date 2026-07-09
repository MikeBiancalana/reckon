# Reckon: Unified Redesign

> **SUPERSEDED (2026-06-19).** Halted at the NO BUILD decision point (see bottom of this doc) —
> too work-coupled for a general personal tool. The live design is
> **[`design/composable-redesign.md`](design/composable-redesign.md)**; see
> [`design/INDEX.md`](design/INDEX.md) for the full doc map. Kept for rationale — its "portable
> core" analysis fed the composable design directly.

_Date: 2026-06-15 · Author: Godfrey (with Gilbert as logseq-overseer source) · Status: SUPERSEDED (was PROPOSED)_

## Thesis

Logseq conflates three different things into one markdown graph and serves none of them well. Mike uses ~20% of Logseq (timestamped append log + tag reconciliation) and pays the UI tax for 100% (outliner, backlinks, graph). It is decaying from both ends: the **write side** (capture friction, agents scattering TODOs) and the **read side** (raw journal is a wall of text optimized for agent append, unreadable for a human skim — so he reads a synthesized brief instead and stopped opening the journal).

Reckon-today is a sound *journal + global task* core, but it (a) has no knowledge-base pillar, (b) is CLI-only single-writer with files-as-truth — which breaks under the real workload of ~15 concurrent agent sessions — and (c) has no read-side synthesis. The redesign separates what Logseq conflated.

**Three pillars, four cross-cutting concerns.**

```
                 ┌─────────────────────────────────────────────┐
   CAPTURE  ───► │  ENTRY SUBSTRATE  (append-only, attributed)  │
  (front door)   └──────────────────┬──────────────────────────┘
                                     │  (auto-linking: typed refs extracted at index time)
        ┌────────────────────────────┼────────────────────────────┐
        ▼                            ▼                             ▼
  ┌───────────┐              ┌───────────────┐            ┌────────────────┐
  │  A. TASKS │              │  B. (substrate│            │  C. READ       │
  │  auto-    │◄────links────│   is pillar B)│────synth──►│  SURFACE       │
  │  reconcile│              │  agent log    │ (read-time)│  brief/recall  │
  └─────┬─────┘              └───────────────┘            └────────────────┘
        │                            ▲
        └────links──► ┌──────────────┴───┐
                      │  PAGES (knowledge)│  files-as-truth, real dir hierarchy
                      └───────────────────┘
```

(Pillar B *is* the substrate; the three "pillars" in Mike/Gilbert's framing are: A auto-reconciling task store, B clean agent append-substrate, C synthesized human read surface. Pages are a fourth durable store with a different concurrency profile.)

---

## Decisions locked (Mike, 2026-06-15)

| # | Decision | Value |
|---|----------|-------|
| Q1 | Cutover scope | Full unification, single store, phased. Agent pipeline rebuilt on reckon. |
| Q2 | Mike's interaction model | **Ambient-first.** Generated digest is the primary human surface; TUI is secondary. |
| Q3 | Readability split | Dense substrate + generated skim layer. Mike never reads raw agent log. |
| Q4 | Task model | **Auto-reconciling mirror.** Tasks are either (i) mirrors of an external SoT (Jira/GitHub) that reckon reconciles continuously — agents never hand-flip status — or (ii) anchorless personal items reckon owns with owner+TTL. `/startup` reconciliation becomes built-in, not a ritual. |
| Q5 | TODO metadata | Mandatory: owner, source, created, due/expiry, external link. Enforced at write. |
| Q6 | Page hierarchy | Real nested directories (path = hierarchy). No `A___B` glyph-encoded mangling. |
| Q7 | Pages audience | Agent-written/agent-read primary (callimachus); human retrieval via query + light browse. |
| Q8 | Migration | Pages imported (de-mangled). Journals archived **read-only**, not bulk-imported. Anchored tasks rebuild from source on first reconcile. Only **anchorless personal commitments** migrate by hand. |
| Q9 | Time-tracking | Dropped from core. Durations optional/freeform if ever needed. |

---

## Core model: the Entry

Everything written is an **entry** — an append-only, immutable, attributed event. This is the substrate (Pillar B).

```
Entry {
  id        xid              // stable, sortable
  ts        timestamp        // honors AT= backfill
  author    string           // PROVENANCE — agent name / peer id / "mike". Required.
  channel   enum             // cli | agent-api | slack | tui | phone
  kind      enum             // log | progress | win | intend | decision | note | done | ...
  body      text             // raw, verbose OK — LOSSLESS. No write-time compression.
  refs      []Ref            // AUTO-extracted: SNP-#####, PR #, page-slug, task-id, @person
  supersedes xid?            // corrections are new entries, never mutations
  thread    xid?             // optional grouping
}
```

Design consequences:
- **Append-only.** Nothing is mutated in place. A "correction" is a new entry that supersedes a prior one. This is what kills the "agent opened a TODO and never closed it" problem *at the substrate level* — state does not live in the log. The log is a record of what happened; task state lives in Pillar A and reconciles from source.
- **Lossless.** No write-time compression (see Cross-cutting #2). The verbose draft is preserved; presentation is a read-time concern.
- **Attributed.** `author` is mandatory. Satisfies Mike's standing rule that consequential writes need verified+attributed provenance. An anonymous append log cannot.

---

## Pillar A — Tasks (auto-reconciling)

A task is a first-class row, not a journal checkbox. Two kinds:

**Anchored** — has an `external_ref` (Jira key, GitHub PR). reckon **reconciles** state from that source on a schedule; agents and humans never hand-flip status. reckon stores the last-reconciled snapshot + any drift. The Jira/GitHub source is authoritative; reckon is an auto-updating mirror.

**Anchorless** — a personal item reckon owns directly (a promise to a person, no ticket). Mandatory `owner` + `created` + `ttl/expiry`. Surfaces when stale; never silently rots.

```
Task {
  id, title, kind(anchored|anchorless)
  external_ref  string?   // SNP-35314 | gh:rbp-api#5759  — null for anchorless
  state         string    // anchored: reconciled from source; anchorless: reckon-owned
  owner         string    // required for anchorless
  created, due, ttl
  reconciled_at timestamp  // anchored only
  links         []         // entries that reference it, pages it relates to (auto)
}
```

- **Reconciliation engine = built-in `/startup`.** Pluggable source adapters: Jira (the riker domain), GitHub (gh / plimsoll). Runs on schedule + on demand. This converts the morning reconciliation ritual into a system property.
- The task list is **authoritative-by-reconciliation**, not authoritative-by-fiat. That is the actual fix for the TODO wound — Logseq's task state was never trustworthy because it had no link to the real SoT.
- Logseq `TODO/DONE` keyword vs `#carry/#commit/#done` tag confusion **disappears**: there is one task model with one state machine; tags become `kind`/`refs`, not a parallel untracked TODO surface.

---

## Pillar B — substrate (the agent log)

The entry stream above. Stored in SQLite (operational truth — see Cross-cutting #3). Queryable, FTS-indexed, concurrency-safe append. This is what `work_system.sh` + section-aware markdown insertion is replaced by — and the section-awareness, the `## Log` insertion fragility, and the Lycurgus *write-gate latency* all go away.

---

## Pillar B′ — Pages (knowledge base) — the missing pillar

Durable knowledge docs (PAS entities, domain notes). reckon-today has none.

- **Files-as-truth here** — pages are durable, human-editable, git-friendly, and NOT a high-concurrency write target, so the file-as-truth model that breaks for the log is correct for pages.
- **Real directory hierarchy.** `pages/PAS/PAS-Entities.md`, not `PAS___PAS Entities.md`. Hierarchy = filesystem path. Kills the mangling Mike flagged.
- Frontmatter (title, tags, slug). **FTS5** for retrieval. Auto-linked to/from entries and tasks (Cross-cutting #4).
- callimachus owns CRUD + retrieval; Mike browses lightly.

---

## Pillar C — Read surface (synthesized)

- **Read-time synthesis** over the lossless substrate (Cross-cutting #2). Digests generated on demand, cached, invalidated on new entries.
- Surfaces:
  - **Daily brief** — the David/Gilbert brief pattern. Generated, delivered to Slack/canvas. Mike's primary read surface.
  - **Task checklist** — reconciled anchored tasks + anchorless-with-TTL. The thing Mike actually relies on (today's carry/commit list).
  - **Recall / query** — callimachus over FTS + entries + the auto-link graph.
  - **TUI** (optional, secondary per Q2) for live interaction.
- **Lycurgus flips role**: from a write-time compression gate (lossy — has bitten us when detail was dropped) into a **read-time digest generator** over lossless substrate.

---

## Cross-cutting concerns (sit under all pillars)

### 1. Capture (the front door — design FIRST)
The single biggest predictor of whether the substrate survives is capture latency. Half of why Logseq decayed was capture friction. Design the capture API before the storage model.
- **One append primitive**: `rk add` (CLI) / library call (agents) / HTTP. Sub-second. `body` + optional `kind`/`refs`; `author`+`ts`+`channel` auto-stamped. `AT=HH:MM` backfill preserved.
- **Every channel hits the same endpoint**: CLI, agent SDK, Slack, phone (later).
- Agents get a dead-simple append — **no section-awareness, no format discipline at write time** (synthesis handles presentation). Removes the `work_system.sh` insertion fragility and the Lycurgus write-gate latency in one move.

### 2. Synthesis timing — read-time (chosen: B)
Store raw substrate, synthesize at read time. Preserves everything; costs one LLM pass per digest (cheap now, and cached). Rejected: write-time compression (Lycurgus model) — lossy, and lossless substrate is worth more than the storage savings.

### 3. Concurrency + provenance
~15 agent sessions write simultaneously. Files-as-truth + last-writer-wins **breaks** here.
- **SQLite (WAL) is the append/write path.** Append-only = no update contention; WAL handles concurrent writers.
- **Every entry stamps `author`** — provenance is structural, not a nicety.
- Markdown files become a **rendered projection** (durable, git-committed archive), regenerated from the DB — not the concurrent-write target. **This inverts reckon's original "files as source of truth."** (Open decision — see below.)

### 4. Typed auto-linking (connective tissue)
The need Logseq's `[[ ]]` served — and that Mike refused to do by hand because manual linking is friction.
- At write/index time, extract typed refs from `body`: ticket keys (`SNP-#####`), PR numbers (`#5759`), page slugs, task ids, person handles (`@`).
- Build typed edges: entry→task, task→page, entry→page, entry→person. **Never hand-typed.**
- Powers recall, the task↔entry join, and the reconciliation linkage. Logseq's linking value without the friction that made Mike never use it.

---

## Storage layout (proposed)

```
~/.reckon/
  reckon.db              # OPERATIONAL TRUTH: entries, tasks, links, page-index, FTS5
  archive/journal/       # rendered daily markdown projection (git, durable, portable)
  pages/<path>/<slug>.md # knowledge pages — FILES-AS-TRUTH, real hierarchy
  digests/               # cached synthesized briefs
```

Two truth models, deliberately: **DB-as-truth for the high-concurrency log/tasks; files-as-truth for low-concurrency durable pages.** DB is rebuildable from the file projection + pages; the projection is regenerable from DB.

---

## What gets removed from reckon-today

- Dead `scheduled_date` / `deadline_date` columns (in DB, unreachable from Go).
- `TaskArchived` status that doesn't survive markdown round-trip.
- Duplicated/dead TUI code (`submitInput()` deprecated; duplicated verbose block in `task.go`).
- `Position` field on every type → replace with `ts` ordering + explicit order only where genuinely needed.
- Files-as-truth **for the journal** → DB (kept for pages).
- Section-aware markdown insertion / round-trip-fidelity machinery (the projection is write-only from DB, so no round-trip to preserve).
- **Schedule feature** — recommend drop, defer to Google Calendar (Mike has the MCP). _(Open decision.)_

---

## Phased plan

Each phase delivers standalone value. Sequencing rationale: capture first (or the substrate starves), then read (so Mike gets value immediately and can stop opening Logseq), then tasks (the reconciliation engine — the TODO fix), then pages (knowledge migration), then connective tissue + polish.

**Phase 0 — Foundations & the front door.** Entry substrate (append-only, attributed) + capture API (`rk add`, agent SDK) + DB-as-truth (WAL) + FTS + ref-extraction at index time. Provenance stamping. _Replaces the `work_system.sh` log pipeline on its own._

**Phase 1 — Read surface MVP.** Read-time synthesis → daily brief + recall query. Wire Lycurgus as digest generator. Cut callimachus *reads* over to the reckon substrate. _Mike stops needing the Logseq journal._

**Phase 2 — Tasks + reconciliation.** Anchored tasks with Jira/GitHub adapters (riker, plimsoll); anchorless tasks with owner+TTL. Built-in `/startup` reconcile. _The TODO-pain fix._

**Phase 3 — Pages.** Import 153 Logseq pages (de-mangle `A___B` → `A/B.md`). FTS retrieval. callimachus page CRUD on reckon.

**Phase 4 — Auto-link graph.** Typed edge graph queries (extraction already running since Phase 0). Powers recall depth + task/page joins.

**Phase 5 — Polish & decommission.** TUI (if wanted), Slack/phone capture channels, file projection/export hardening, retire the Logseq write pipeline; Logseq → read-only archive.

---

## Decisions resolved & guardrails (Mike, 2026-06-15)

1. **DB-first — LOCKED.** DB is operational truth; markdown is a durable, git-committed projection. Files-as-truth retired.
2. **Build, not buy — LOCKED.** No third-party task backend (taskwarrior/beads). The auto-reconciling-mirror model diverges from off-the-shelf tools; hand-roll the task pillar. (The `.beads` dir in this repo tracks reckon's own *dev* issues — unrelated to the personal task pillar.)
3. **Projection-honesty guardrail — LOCKED.** The markdown projection must stay a clean, grep-able, git-committed mirror, regenerable from the DB and never hand-edited. The day it stops being that, the plaintext virtues reckon exists for are lost. **Journal projection = OUTPUT; `pages/` = INPUT / files-as-truth — never confuse the two.**

## Still open (proceeding on recommendation unless vetoed)

- **Drop the Schedule feature**, defer to Google Calendar. Recommend drop.
- **Rebuild scope** — keep the stack (Go / Bubble Tea / Cobra / SQLite), harvest the parser/writer (projection + pages) and TUI components; rebuild storage + capture + task + sync. Not greenfield, not in-place patch. Recommend this middle path.

→ Concrete schema, capture API, projection format, reconciliation, and Phase 0 scope: **`reckon-spec_2026-06-15.md`**.

---

## Decision point (2026-06-15): retool vs greenfield — NO BUILD

Mike halted implementation before Phase 0. His reasoning, captured verbatim-in-spirit: the design that emerged is **very customized to how he works *here*** — auto-reconciling mirror of Jira/GitHub, the PAS pipeline, the agent fleet. But **reckon was conceived as a more general personal tool** he might use outside work too. The two have diverged, and he suspects **retooling reckon may be less fruitful than cutting fresh from whole cloth**.

This is a decision point, not a green light. The design thinking and spec stay valuable regardless of which way he goes. The open question — **retool reckon vs greenfield a work-specific system** — is Mike's to settle before any build.

To inform that call, the spec separates cleanly along exactly the axis he named:

**Portable core (general personal tool — reusable in reckon or anywhere):**
- Entry substrate: append-only, attributed, lossless.
- Capture-as-front-door (sub-second append, every channel one endpoint).
- Read-time synthesis over lossless substrate (digest, recall).
- DB-first + deterministic grep-able markdown projection.
- Anchorless tasks with owner + TTL.
- Typed auto-linking (refs → links), real-dir page hierarchy + FTS.

**Work-specific coupling (SN-job artifacts — NOT general):**
- The anchored auto-reconciling mirror against **Jira/GitHub** (pillar A's whole reconciliation engine).
- Agent-fleet provenance (15 concurrent personas, peer ids).
- Integration with Lycurgus / callimachus / riker / plimsoll and the David/Gilbert brief delivery.
- PAS domain context.

This split itself suggests an architecture if he ever wants both: **build the portable substrate once; layer the work-specific reconciliation/provenance/brief pieces as pluggable adapters.** Under that lens the substrate is reusable either way — which is the strongest argument *against* full greenfield, and the cleanest path *to* a general tool that also does the work job. But the call is Mike's. **Standing down on implementation.**
