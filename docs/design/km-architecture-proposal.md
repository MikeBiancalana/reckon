# Knowledge-Management Architecture — Research & Proposal

> Author: **Luhmann** (research peer, Fable model) · Date: 2026-07-09 · Status: PROPOSED
> Commissioned via Gilbert. Brief: `.km-research-brief.md`. Builds on Godfrey's assessments
> (`composable-redesign-assessment.md`, 2026-06-22; `reckon-redesign_2026-06-15.md`) rather than re-deriving them.
> **Amended 2026-07-09 (PM)** after review with Mike: Option A confirmed; timestamp semantics settled
> (§4.2); maturity stages + schema file adopted (§4.2.1); `rk lint` + OKF-lint CI (§4.3); agent-session
> capture via harness hooks (§4.5); `rk prime` discoverability verb (§4.8); cross-pollination survey (§1.4);
> sharing seam — circles-as-repos (§4.9, v2).

## Executive summary

**Recommendation: Option A — extend reckon.** Finish the composable-redesign v1 (T7/T8/T9), make the
notes pillar (v1-T8) **OKF-conformant by construction**, and run the Karpathy-brain operations
(ingest / query / lint) as agent porcelains over reckon's existing verb surface. Do **not** adopt OKF
wholesale (it has no task model — the spec explicitly declines workflow states), and do **not** run a
separate OKF/Karpathy store beside reckon (two link namespaces, two indexes, no cross-type graph).

The load-bearing finding: **reckon's canonical node format is already a structural superset of OKF.**
Both are "markdown + YAML frontmatter, exactly one required field (`type`), links that may dangle."
OKF conformance for reckon's notes costs a few conventions (recommended fields, an `index.md`
generator, an export view for wikilinks), not an architecture change. Meanwhile the three pain points
in the brief — type conflation, dead-tag litter, tasks that never close — are each *directly and
already* addressed by shipped reckon v1 machinery. The gap is not design; it is the three unfinished
v1 tasks plus the ritual rewiring and Logseq migration.

---

## Part 1 — Research findings

### 1.1 OKF: Google's Open Knowledge Format

Announced by Google Cloud on 2026-06-12; spec v0.1 lives at
[`GoogleCloudPlatform/knowledge-catalog/okf/SPEC.md`](https://github.com/GoogleCloudPlatform/knowledge-catalog/tree/main/okf).
Announcement: [Google Cloud blog](https://cloud.google.com/blog/products/data-analytics/how-the-open-knowledge-format-can-improve-data-sharing/).

**What it is.** "OKF v0.1 represents knowledge as a directory of markdown files with YAML
frontmatter, with a small set of agreed-upon conventions that let wikis written by different
producers be consumed by different agents without translation." A bundle is just a directory tree of
`.md` files — distributable as a git repo, tarball, or subdirectory. No SDK, runtime, or registry.

**Three principles (quoted from the spec/announcement):**
1. *Minimally opinionated* — "OKF requires exactly one thing of every concept: a type field."
2. *Producer/consumer independence* — who writes the knowledge is decoupled from who consumes it
   (human-authored → agent-consumed; pipeline-generated → human-browsed; LLM-synthesized → LLM-queried).
3. *Format, not platform* — not tied to any cloud, database, model provider, or agent framework.

**The typed-entity model (the part that matters here).**
- `type` is the **only required** frontmatter field. "Type values are **not** registered centrally.
  Producers SHOULD pick values that are descriptive and self-explanatory; consumers MUST tolerate
  unknown types gracefully." Example types from the sample bundles: `BigQuery Table`, `Metric`,
  `API Endpoint`, `Playbook`, `Runbook`.
- Recommended fields: `title`, `description` (one-sentence summary for indexes/search snippets),
  `resource` (URI of the underlying asset), `tags`, `timestamp` (ISO 8601 last-meaningful-change).
- **Arbitrary extra frontmatter keys are explicitly permitted**: consumers "SHOULD preserve unknown
  keys when round-tripping and SHOULD NOT reject documents with unrecognized fields."
- Reserved filenames: `index.md` (directory listing, progressive disclosure; no frontmatter except
  an optional bundle-root `okf_version`) and `log.md` (chronological update history; a convention,
  not a requirement).

**Linking.** Normal markdown links; bundle-root-absolute (`/tables/orders.md`) is the recommended
form. "A link from concept A to concept B asserts a *relationship*. The specific kind of relationship
… is conveyed by the surrounding prose, not by the link itself." Critically: **"Consumers MUST
tolerate broken links — a link whose target does not exist in the bundle is not malformed; it may
simply represent not-yet-written knowledge."**

**What OKF does NOT define — decisive for this proposal.** The spec defines no task management, no
workflow states, no approval processes, no lifecycle metadata beyond `timestamp`. `log.md` has a
`Deprecation` entry convention and that is the entire extent of state handling. **OKF is a knowledge
format, full stop. It cannot be the task store.**

Conformance bar (low by design): every non-reserved `.md` file has parseable YAML frontmatter with a
non-empty `type`; reserved files follow their structure. Everything else is soft guidance — consumers
must not reject bundles for unknown types, unknown keys, broken links, or missing `index.md`.

### 1.2 The "Karpathy brain" (LLM-wiki pattern)

Origin: Andrej Karpathy's April-2026 X post + [gist](https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f)
(16M+ views, 5k+ stars in days). Core claim: "Humans abandon wikis because the maintenance burden
grows faster than the value. LLMs don't get bored, don't forget to update a cross-reference, and can
touch 15 files in one pass." And: "The tedious part of maintaining a knowledge base is not the
reading or the thinking — it's the bookkeeping." Google explicitly frames OKF as the standardization
of this pattern ([Google Cloud Tech announcement](https://x.com/GoogleCloudTech/status/2067012903337664886);
analysis: [themenonlab](https://themenonlab.blog/blog/google-okf-open-knowledge-format-karpathy-llm-wiki-standard)).

**Architecture — three layers:**
- `raw/` — immutable curated sources. The LLM reads, never modifies.
- `wiki/` — LLM-generated, densely cross-linked markdown pages organized by type (summaries, entity
  pages, concept pages, syntheses), plus `wiki/index.md` (catalog with one-line summaries — the
  navigation layer that substitutes for RAG at ~100-source scale) and `wiki/log.md` (append-only
  operation record, parseable headings like `## [2026-04-02] ingest | Title`).
- The **schema file** (`CLAUDE.md` or equivalent) — page-type taxonomy, naming conventions,
  new-page-vs-edit rules, and the workflows. One practitioner: "The schema file is everything… it
  co-evolved into the single most important file in the repo."

**Three operations:**
- **Ingest** — read a new source, write/update a summary page, ripple updates across 10–15 entity
  and concept pages, update the index, append to the log.
- **Query** — answer from the wiki with citations; valuable answers get filed back as pages
  ("exploration compounds into knowledge").
- **Lint** — periodic health pass: contradictions, stale claims, orphan pages, missing
  cross-references. The gist names **drift** (silent staleness) as the primary failure mode at scale;
  lint is load-bearing, not optional.

No vector DB, no retrieval pipeline — long context + a maintained index. Jim's setup uses **OKF as
the storage layer** for exactly this: the `wiki/` pages are OKF-conformant typed concepts, which is
precisely what OKF was written to standardize. (The org's `understand-knowledge` skill likewise
targets "Karpathy-pattern LLM wiki knowledge base[s]" — the pattern is already in-house.)

### 1.2.1 Why neither source specifies a query backplane

Both omissions are deliberate, and the absence is load-bearing for the design below. OKF: principle 3
("format, not platform") — query is a *consumer* concern; a backplane in the spec would violate
producer/consumer independence. Karpathy: his answer to query *is* "no backplane" — the pattern's
thesis is that long context + a maintained `index.md` replaces retrieval infrastructure (fine at
~100 sources, degrades past that). Reckon's derived, disposable, never-synced SQLite property graph
is a strictly stronger consumer-side answer that stays OKF-legal: files remain the format, the index
is one consumer among many. Typed edges + backlinks + FTS also beat OKF's "relationship conveyed by
the surrounding prose" for machine query.

### 1.3 Reckon — current state (as of 2026-07-09)

Three design generations, each documented in-repo:

1. **2026-01-17 assessment** (`docs/2026-01-17-review-and-assessment.md`): original reckon =
   journal/TUI/time-tracking tool, grade B+, but DB-primary for tasks/notes and no knowledge pillar.
2. **Godfrey's work-coupled redesign** (2026-06-15, `reckon-redesign_2026-06-15.md` + spec):
   DB-first substrate, auto-reconciling Jira/GitHub task mirror, read-time synthesis. Mike halted it
   pre-build ("NO BUILD") because it over-fit his current job; reckon is meant to be a general
   personal tool.
3. **Composable redesign** (2026-06-19→23, `docs/design/composable-redesign.md` + Godfrey's
   assessment + rebuttal): the **live architecture**. UNIX-composable tools over plain-text + git;
   canonical node (ULID, `type`-as-property, frontmatter props, Obsidian-flavored wikilinks);
   disposable per-device SQLite property-graph index; `rk` multi-call binary; agent-ergonomic verbs
   (`bd` standard); model-free core. Godfrey's assessment surfaced three gating risks (round-trip
   keystone, journal write-path losslessness, corrected 20–35% reuse estimate); all were folded into
   the 2026-06-22 amendments and the round-trip spike **passed** (2026-06-23, ~396k fuzz executions,
   0 failures — `spike-roundtrip-verdict.md`).

**v1 build state** (beads + git history):

| Shipped | Task | What |
|---|---|---|
| ✅ | v1-T0 | `rk` dispatcher, config, vault/index layout, `merge=union` for `log/*.md` |
| ✅ | v1-T1 | canonical node package — byte-preserving parse/serialize, span-local edits, NDJSON envelope |
| ✅ | v1-T2 | SQLite property-graph index behind stable views; lazy reconcile; `rk index` |
| ✅ | v1-T3 | `rk query` — read-only SQL, NDJSON out, saved views (+ sanctioned FTS5 surface, #141) |
| ✅ | — | `rk todo add/list/done` — durable + ephemeral (#148) |
| ✅ | v1-T4 | `rk log` — day group files, provenance, `AT=` backfill, capture→index→query proven (#149) |
| ✅ | v1-T6 | recurrence via stored `scheduled` cursor, org repeaters (`+`/`++`/`.+`) (#150) |
| ✅ | — | `rk adopt` (ULID-stamp id-less files, #146); duplicate-ULID/alias-collision detection in reconcile (#142); checklist-run TUI (#144) |
| ⬜ | v1-T7 | `rk today` — split-actuator agenda |
| ⬜ | **v1-T8** | **note tool (`rk note`) + linking — the knowledge pillar** |
| ⬜ | v1-T9 | migration of current DB-primary data to text-truth |
| ⬜ | v1-T10 | MCP porcelain (fast-follow) |
| ⬜ | v1-T11 | `rk-brief` synthesis seam (deferred until lived need) |

### 1.4 Adjacent-idea survey (cross-pollination, 2026-07-09)

Swept for adjacent patterns worth borrowing, filtered against the simplicity constraint. Adopted:

- **Note maturity stages** ([Appleton](https://maggieappleton.com/evergreens), from
  [Matuschak's evergreen-notes](https://notes.andymatuschak.org/Evergreen_notes) lineage): one
  optional `stage: seedling|budding|evergreen` prop. Orthogonal to provenance (`author` = who/trust;
  `stage` = epistemic maturity). Outsized value in an agent-written wiki: consumers know settled vs.
  draft; lint prioritizes seedlings; queries filter to evergreen-only. → §4.2.1.
- **Evergreen writing discipline as schema-file content** (Matuschak: atomic, concept-oriented,
  densely linked, "titles are APIs"): zero code, pure agent-instruction. → §4.2.1.
- **OKF ecosystem tooling** ([tool index](https://okf.md/tools/),
  [survey](https://www.owox.com/blog/articles/okf-ecosystem-tools)): okf-lint/okf-conformance in CI
  over the export; `kiso` static-site publisher as a free read surface. → §4.3.

Noted, not adopted: **llms.txt** (fixed-address "read first" entry — reckon's generated root
`index.md` already is this; lesson: keep it curated-short); **Zep's counter-argument**
(["Markdown is not agent memory"](https://blog.getzep.com/markdown-is-not-agent-memory/) — markdown
breaks at scale/concurrency/temporal reasoning; reckon's hybrid answers each point: `merge=union` +
ULID-dedup, time-sortable ULIDs, SQLite graph instead of context-stuffing — the steelman that
validates the hybrid). Skipped deliberately: PARA, Johnny.Decimal, MemGPT-style memory layers —
duplicate reckon machinery or add taxonomy against the simplicity constraint.

---

## Part 2 — Fit analysis

### 2.1 Mike's pain points vs. reckon's design

| Pain (brief) | Logseq today | Reckon composable design | Status |
|---|---|---|---|
| **One bucket, three types** — journal/notes/tasks interspersed in daily files | All in `journals/YYYY_MM_DD.md`, papered over by queries | Separate typed tools over one substrate: `log/` day-files, `todos/` file-per-item, `notes/` file-per-note. Types are first-class (`type:` prop); "hard separation between tasks and logs" is a **user-confirmed core value** enforced by the module split | Log ✅, todos ✅, notes ⬜ (T8) |
| **Dead-tag litter** — `#tags` linking nowhere | `#carry`/`#done` etc. sprinkled, no page behind them | Tags → structured `tags` prop (queryable, not pseudo-links). Links resolve **late** via the index; dangling links are allowed as *forward references* but are **tracked, queryable (`rk query` for unresolved), and auto-resolve on target creation**. Alias uniqueness index-enforced; collisions flagged (#142) | ✅ shipped |
| **Tasks never close** — "done" = a new journal line; original `#carry` stays open forever | State smeared across prose; manual reconciliation | Task = a node with a real `state` prop; `rk todo done` closes it **in place** (span-local write). Recurrence advances a stored `scheduled` cursor org-style (T6). Ephemeral/durable split: throwaways get no address and expire; durables persist until closed. Completion emits a linked `did` log entry — the journal records the closure, but **state lives in the task, not the log** | ✅ shipped |
| **Plain text** (firm) | Yes, but state-in-prose | Yes, and *more honest*: files are truth, index is disposable/derived/never synced; byte-preserving round-trip is a **gated, fuzz-tested invariant** | ✅ shipped |
| **Repeats & checklists** | None real | Org repeaters + stored cursor (T6); checklist templates → materialized runs (TUI shipped, #144) | ✅ shipped |

The "carry" verb deserves a note: **carry disappears structurally.** Carrying was an artifact of
tasks living inside dated journal files — a task that outlives its page must be re-written onto the
next page. When a task is its own file with its own state, there is nothing to carry; `rk today`
surfaces open+scheduled+overdue automatically.

### 2.2 The reckon-node ↔ OKF congruence

Side by side, the two formats converge to a striking degree — independently (reckon's node was locked
2026-06-19; OKF published 2026-06-12; neither references the other):

| Dimension | OKF v0.1 | Reckon canonical node |
|---|---|---|
| Storage | Markdown + YAML frontmatter, directory tree | Same |
| Required fields | `type` (only) | `id` (ULID), `type`, `time` |
| Type taxonomy | Free-form, producer-chosen, consumers tolerate unknown | Same (`type` = property, open per-tool vocabulary) |
| Extra frontmatter | Explicitly permitted; must be preserved | `props` open bag, byte-preserving round-trip |
| Broken links | "MUST tolerate… may simply represent not-yet-written knowledge" | Dangling links = unresolved edges, auto-resolve, queryable |
| Index | `index.md` progressive-disclosure convention | SQLite property-graph (richer: typed edges, backlinks, FTS, aliases) |
| History | `log.md` convention; git recommended | git is the history substrate |
| Link syntax | Standard markdown links, `/`-rooted preferred | Obsidian wikilinks `[[alias]]`, alias→ULID late resolution |
| Identity | File path (implicit) | ULID inline (rename/move-proof) |
| Provenance | — | `author` field (required) |
| Task lifecycle | **None (out of scope by design)** | Full: state, done-in-place, recurrence cursor, ephemeral/durable, checklists |

Reading: **a reckon note is an OKF concept with extra keys** — and OKF consumers are *required* to
tolerate extra keys. The two real deltas are (a) link syntax (wikilinks vs. markdown links — OKF
consumers won't resolve `[[...]]`) and (b) reckon has no `index.md` generation. Both are export-layer
concerns, not model concerns. Conversely, reckon is *stronger* than OKF everywhere they overlap:
stable identity (ULID vs. path), typed edges + backlink index (vs. prose-conveyed relationships),
enforced alias uniqueness, provenance, and a fuzz-gated round-trip. OKF's spec-level tolerance of
broken links even vindicates a reckon decision Godfrey's panel already blessed: dangling links as
queryable forward-references rather than errors.

### 2.3 The Karpathy brain maps onto reckon's verb surface

| Karpathy-brain element | Reckon equivalent | State |
|---|---|---|
| `wiki/` typed pages | `notes/` (v1-T8), `type:` per page | ⬜ T8 |
| `raw/` immutable sources | `sources/` dir in vault — indexed read-only or ignored; `rk adopt` if promotion wanted | trivial addition |
| Schema file (`CLAUDE.md`) | The vault's agent instructions + per-tool opinionated structure (reckon's core anti-Obsidian bet: "the structure *is* the product") | exists conceptually; write it |
| `index.md` catalog | Superseded by the property-graph index + `rk query`; *generate* `index.md` for OKF interop | small T8 follow-on |
| `log.md` operation record | `rk log` day files — richer: provenance-stamped, ULID'd, queryable | ✅ |
| **Ingest** | Agent porcelain: read source → write/update notes via `rk note` → links auto-indexed | ⬜ needs T8 |
| **Query** | `rk query` (SQL over stable views, NDJSON, FTS + graph traversal) — this is RAG-over-your-own-substrate, already built | ✅ |
| **Lint** | Deterministic half shipped: reconcile detects duplicate ULIDs, alias collisions, unresolved edges are queryable. Judgment half (contradictions, staleness) = an LLM porcelain per the determinism-boundary law | ✅ / ⬜ porcelain |

The pattern's known failure mode — drift — is exactly what reckon's deterministic lint surface
already guards mechanically, leaving the LLM only the judgment calls. This is the composable-redesign
"determinism boundary" law applied to Jim's pattern, and it is a *better* Karpathy brain than a bare
OKF directory: the gist's lint pass has to rediscover orphans and broken links by reading files; reckon's
index already knows them.

---

## Part 3 — Options compared

### Option A — Extend reckon (recommended)

Finish v1 (T7 agenda, T8 notes, T9 migration); make T8's on-disk format OKF-conformant by
convention; add a thin OKF export; run Karpathy-brain ingest/lint as agent porcelains.

- **For:** Task lifecycle, recurrence, checklists, provenance, index, query surface — all shipped and
  tested. Notes land on the same substrate → one link namespace, one graph, cross-type edges
  (note↔task↔log) for free. OKF conformance is nearly free (see 2.2). Preserves Obsidian/Syncthing
  mobile flow. Honors every locked decision (plain text, files-as-truth, embrace-&-extend).
- **Against:** T7/T8/T9 are real work (though T8 is the smallest of the three pillars — file-per-note,
  the node package does the heavy lifting). Wikilinks need an export pass for strict OKF consumers.

### Option B — Adopt OKF wholesale (all three stores as OKF bundles)

- **For:** Maximum interop with Jim's tooling and future OKF consumers; one external standard.
- **Against — disqualifying:** OKF **has no task model** — no states, no lifecycle, no recurrence
  (§1.1). You would hand-roll `state:` frontmatter with no engine: no `rk todo done`, no stored-cursor
  recurrence, no ephemeral/durable split, no agenda — i.e., rebuild reckon's shipped machinery inside
  a format that deliberately excludes it. Path-as-identity (no ULID) reintroduces rename fragility the
  node design explicitly killed. No provenance field. A journal is also a poor fit (OKF's `log.md` is
  a *convention for wiki change history*, not a personal journal with provenance and backfill).
  This option mistakes a knowledge format for a system.

### Option C — OKF/Karpathy brain for notes + reckon for tasks + thin journal store

- **For:** Each type gets a purpose-built home; notes bundle is natively consumable by OKF tooling.
- **Against:** Two link namespaces — OKF markdown paths vs. reckon ULID/alias wikilinks — so
  **note↔task and note↔log edges break**, which forfeits the daily-review view and the
  log-capture→branch-to-note flow (the Logseq behavior Mike *loved*, per the composable doc's derived
  requirements). Two indexes, two lint surfaces, two sync stories. And since a reckon note can *be*
  OKF-conformant (2.2), the interop benefit is achievable inside Option A at a fraction of the
  seam-count. C is A with an unnecessary wall through the middle.

**Verdict: A.** B is disqualified on the task pillar; C pays permanent integration cost for a benefit
A gets with an export command.

---

## Part 4 — Recommended target architecture

### 4.1 Stores (three types, separately typed, one substrate)

```
vault/                          # plain text, git + Syncthing; the only synced truth
├── log/2026-07-09.md           # JOURNAL — day group file; N+1 nodes (day + entries);
│                               #   append via rk log; provenance + AT= backfill; merge=union
├── todos/01J….md               # TASKS — file-per-todo; state/scheduled/deadline/repeat props;
│                               #   ephemeral tasks in group files, no ULID, expire
├── checklists/…                # templates + materialized runs
├── notes/<topic>/01J… or slug.md   # NOTES — file-per-note, Zettelkasten; OKF-conformant
│   └── index.md                #   generated, OKF progressive-disclosure
├── sources/                    # raw layer (Karpathy): immutable, read-only
└── .gitattributes              # merge=union for log/*.md
~/.cache/reckon/<vault>/index.db    # derived property-graph; per-device; NEVER synced
```

Journal = timestamped append-only events. Tasks = stateful nodes that **close in place**. Notes =
durable linked knowledge. Different lifecycles, different write patterns, different directories —
same node format, same index, same query surface. This *is* the brief's "different things stored as
different things," with the cross-type link graph that made Logseq's one-bucket approach tempting in
the first place.

### 4.2 Notes pillar (v1-T8) — OKF-conformant by construction

Frontmatter convention for `rk note` (superset of OKF; every key OKF doesn't know is legal):

```markdown
---
id: 01J9Z3K7Q2W8XR4M6N0V5BYHED     # reckon identity (ULID)
type: concept                       # OKF required; free taxonomy: concept|entity|summary|
                                    #   decision|runbook|reference|memory…
title: PAS Entity Model             # OKF recommended
description: How PAS entities map across Propexo/AppFolio.   # OKF recommended — 1 sentence,
                                    #   mandatory-by-convention (feeds index.md + LLM snippets)
tags: [pas, integrations]
time: 2026-07-09T10:40:00-04:00     # reckon semantic timestamp (created/event; AT= backdatable)
author: mike                        # reckon provenance (who/trust)
stage: seedling                     # optional maturity: seedling|budding|evergreen (§4.2.1)
aliases: [pas-entities]
---
Body. Wikilinks: [[lease-lifecycle]], [[SNP-35314]]…
```

**Timestamp semantics (settled 2026-07-09).** The vault stores **no** `updated`/`timestamp` field.
Rationale: (a) last-modified is derivable — git is the versioning backplane, and the envelope/index
already carry `mtime`; storing it would duplicate truth (node invariant #1). (b) A stored updated-at
*will* lie — hand edits via nvim/Obsidian/phone won't bump it, and a wrong staleness signal is worse
than none (the dead-tag decay pathology, relocated). (c) Auto-bumping churns git/Syncthing and
dirties span-local write-back. OKF's `timestamp` ("last meaningful change") is derived **at export
time** from git last-commit (mtime fallback) — correct semantics, zero stored duplication. A
deliberate authored `reviewed:` field ("verified still true on X" — the honest staleness assertion,
bumped only by the lint pass) was considered and **deferred**: add later if lint proves the need.

#### 4.2.1 Maturity stages + the schema file

Two adoptions from the adjacent-idea survey (§1.4), both cheap, both aimed at the same problem —
an agent-written wiki needs epistemic signals and writing discipline or it becomes note soup:

- **`stage:` prop** — `seedling` (fresh synthesis, unverified) → `budding` (linked, partially
  verified) → `evergreen` (settled, load-bearing). Orthogonal to `author` provenance: *who wrote it*
  vs. *how settled it is*. Ingest porcelains mint at `seedling`; promotion is deliberate (human or
  lint-porcelain judgment). Queries and briefs can filter (`stage='evergreen'` for answering;
  `stage='seedling' ORDER BY time` for lint priority). One optional prop; absent = untracked, legal.
- **The vault schema file** — written alongside T8, zero code; Karpathy practitioners call it "the
  single most important file in the repo." Contents: the type taxonomy; the evergreen discipline
  (atomic notes, concept-oriented, densely linked, **titles are APIs** — a title states its claim);
  new-note-vs-edit rules; stage-transition rules; `description` mandatory-by-convention. It governs
  every ingest agent thereafter — the cheapest quality lever in the system, and it co-evolves.

Plus two small pieces:
- **`rk note index`** (or as part of reconcile): generate `notes/**/index.md` per OKF's
  progressive-disclosure convention from the property-graph index. Deterministic, derived, cheap.
- **`rk export --okf [dir]`**: emit a conformant bundle — rewrite `[[alias]]` → bundle-relative
  markdown links via the resolver, derive OKF `timestamp` from git last-commit, emit `index.md` +
  optional `log.md`, drop nothing (extra keys are legal). This is the interop door to Jim's tooling
  and any OKF consumer, and it is *pure projection* — the vault itself never bends to a consumer.
  **Run `okf-lint`/`okf-conformance` on the export in CI** ([ecosystem tools](https://okf.md/tools/))
  so conformance is guaranteed by machine, not care. `kiso` (bundle → static site) is a free
  browsable read surface over the same export, zero build.

### 4.3 Karpathy-brain porcelains (after T8)

Per the determinism-boundary law — deterministic verbs in core, judgment in porcelain:
- **`rk-ingest`** (agent porcelain): source in `sources/` → summary + entity/concept notes via
  `rk note`, ripple-updates existing notes, links auto-extracted and indexed. The schema file
  (vault-level agent instructions) governs taxonomy and new-page-vs-edit — Karpathy's "most important
  file," co-evolved.
- **`rk lint`** (deterministic core, mostly exists): one verb aggregating what reconcile already
  computes (#142) — unresolved edges, alias collisions, duplicate ULIDs, orphan notes (no inbound
  edges), stale seedlings (`stage='seedling'`, oldest first) — all index queries. **`rk-lint-deep`**
  (agent porcelain): contradictions, staleness, missing cross-refs — the judgment half, run
  periodically like Karpathy's lint pass; it reads `rk lint`'s output first (token economy).
- **Query** is `rk query` — shipped. Valuable answers file back as `type: synthesis` notes with
  `derived-from` edges (`author: <model>` — AI output re-enters as nodes, per the design).

### 4.4 True task closure (already shipped — for the record)

Open→done: `rk todo done` flips `state` in the task's own file (span-local, byte-preserving).
Repeats: org repeaters advance the stored `scheduled` cursor; missed cycles materialize as instances.
Checklists: template → run. Ephemeral: group-file, no address, expires. The journal *records*
closures (`did` edge) but never *holds* task state — the exact inversion of the Logseq failure.

### 4.5 Ritual & memory integration

| Today | Target |
|---|---|
| `work_system.sh log/progress/win/intend` | `rk log` with `kind`/tag prop (aliases preserve muscle memory; thin shim during transition) |
| `todo` / `commit` / `owed` | `rk todo add` (durable, owner prop) — real lifecycle |
| `carry` | **retired** — structurally unnecessary (see 2.1) |
| `fin` / `done` | `rk todo done` — closes in place + linked `did` log entry |
| `/startup` | reads `rk today` (T7) + `rk query` saved views instead of parsing Logseq journals. Work-ticket reconcile (Jira/GitHub) stays an **extension** (`rk-jira`/`rk-gh`, read-only+jump per the rebuttal) — not core |
| `/pa` (Gilbert), `/wrap-up`, `/weekly-retro` | same data via `rk query` (NDJSON, token-scoped) — agents stop grepping raw journals |
| `/log-day` | largely obsolete (capture is frictionless); residual = `rk log AT=` backfill |
| Lycurgus write-gate | retire on the log path — substrate is lossless; compression is read-time (`rk-brief` seam, T11, only if the flat-log read fails per Godfrey's test) |
| `MEMORY.md` auto-memory | **Phase-2 candidate**: memories are `type: memory` notes (one fact per file already matches file-per-note; `[[name]]` linking already matches). Bridge: keep `MEMORY.md` as a generated index over `rk query --view memories`. Defer until notes prove out — harness coupling, low pain today |
| `PA/observations` page | migrates with pages → `notes/PA/observations.md` |

**Record-as-you-go agent capture (added 2026-07-09).** Requirement: everything Mike does with an
agent is recorded as it happens. Two hook layers exist and must not be conflated: **reckon hooks**
(`.reckon/hooks/`, seam reserved in the design) fire on *reckon* lifecycle events and cannot see
agent activity; **Claude Code harness hooks** (settings.json) fire on *agent* activity — that is the
capture point. Wiring: SessionStart/SessionEnd/UserPromptSubmit/Stop hooks shell out to
`rk log --author <persona> …`. Everything needed on reckon's side is already shipped — `rk log`
(T4), mandatory provenance, `AT=` backfill, `merge=union` lossless concurrent appends — so this is
configuration, not reckon code. **Grain matters**: per-tool-call logging (PostToolUse) rebuilds the
wall-of-text; instead, deterministic boundary events via hooks (session open/close, prompt → intent
entry, turn markers + transcript path) + milestone entries via agent-instruction discipline
(`rk log` at decisions/completions), presentation at read time — the determinism-boundary law
applied to capture. Consequence to decide eyes-open: full-fleet capture makes the log a firehose,
which likely flips Godfrey's synthesis test (§4.7) toward building T11 `rk-brief`. Session
transcripts themselves are natural `sources/` citizens (immutable, high-volume, cite-backable).

### 4.6 Migration off Logseq (sequenced, reversible)

1. **Now → T7/T8 land:** keep Logseq writes; no cutover.
2. **T8 + pages import:** walk `~/logseq/pages/*.md`, de-mangle `A___B.md` → `notes/A/B.md`
   (decided Q6/Q8, 2026-06-15; ~153 files), preserve frontmatter, `rk adopt` to stamp ULIDs, index.
   Old tag names become aliases where they had real pages; bare dead tags simply don't migrate — the
   litter stays behind.
3. **Journals: archived read-only, not imported** (decided Q8). History stays greppable in place;
   optionally index later as `sources/` if recall wants it.
4. **Live tasks:** hand-migrate open `#carry`/`#commit`/`#owed` (the genuinely-alive handful) via
   `rk todo add`; everything closed stays in the archive.
5. **Ritual cutover:** shim `work_system.sh` verbs → `rk` (one week parallel-write if wanted), then
   repoint `/startup` `/pa` `/wrap-up` at `rk query`. Retire the shim.
6. **T9 (reckon's own DB-primary data → text-truth)** proceeds independently per its bead.
7. **Later:** `rk export --okf`, ingest/lint porcelains, MEMORY.md bridge, T10 MCP.

### 4.7 Risks / open questions

- ~~T8 format detail~~ — **settled 2026-07-09** (§4.2): reckon fields only in the vault; OKF
  `timestamp` derived at export; `reviewed:` deferred; `stage:` adopted.
- **OKF is v0.1, three weeks old.** Betting the *export* on it is cheap; betting the *store* would
  not be. Option A bets only the export. Watch adoption.
- **Wikilink/markdown-link dual life** — inside the vault, wikilinks win (Obsidian mobile, aliases);
  strict-OKF consumers get the export. If a live-consumer (not snapshot) need appears, revisit.
- **Synthesis** — unresolved from Godfrey's assessment (his honest test: render a real 15-agent day
  as a flat list; if Mike won't read it, T11 stops being deferred). Nothing in this proposal blocks
  either answer.

### 4.8 Agent discoverability — `rk prime` (added 2026-07-09, Mike's ask)

A first-class verb whose job is **direction and discoverability for agents**: the handle an agent
grasps first, from which it learns every other handle. This realizes `bd`-standard requirement #4
("self-describing / next-step hints — the agent learns the tool from the tool") as a concrete verb
instead of scattered `--help` text, and it is llms.txt's insight applied to a CLI: one fixed
entry point, curated-short, pointing at what matters.

- **`rk prime`** — emits a compact, token-budgeted orientation (target ≤ a few hundred lines):
  what reckon is (one paragraph); the store layout and node format (one cheat block); the verb map
  (verb → one-line purpose → canonical example); how to query (the stable views + 3–4 worked
  `rk query` examples); how to write (which verbs mutate what — and that `rk query` never does);
  next-step pointers (`rk prime --topic <t>` for depth).
- **`rk prime --topic node|query|todo|log|note|hooks`** — scoped deep-dives, so an agent pulls only
  the slice it needs (token economy, requirement #3).
- **`rk prime --json`** — machine-readable capability manifest: verbs, flags, output modes, view
  schemas. This is what the T10 MCP shim serves as its tool description, and what porcelains
  introspect — one source of truth for "what can rk do."
- **Implementation discipline**: content generated from Cobra command metadata + curated per-tool
  snippets, **compiled into the binary** — versioned with the code, so orientation can never drift
  from the verbs (the drift failure mode, §1.2, applied to docs). A stale `prime` is worse than none.
- Relationship to neighbors: the *vault* schema file (§4.2.1) teaches agents the **knowledge
  conventions** (taxonomy, discipline, stages); `rk prime` teaches the **tool surface**. Tool ships
  with the binary; conventions live with the vault. Don't merge them — they version independently.

### 4.9 Sharing seam — circles-as-repos (added 2026-07-09, Mike's ask; **v2, design-captured only**)

Requirement: share subsets of the knowledge base with other reckon users (e.g. Jim) so each party
contributes to a shared KB without exposing their private vault; imports mesh via ULIDs.

**Rejected shape: `visibility:` frontmatter prop + filtered export.** Considered first; fails on
three counts: prop-based access control in plain text is advisory and must fail closed (a forgotten
prop = a leak vector); export-time redaction and link-leak auditing become a policy engine; and
re-share updates land in distributed-sync territory (merge policy for partially-shared, locally-
edited nodes). Block-level visibility was rejected outright — atomic-note discipline (§4.2.1) makes
the note the natural sharing granule; to share half a note, split the note.

**Chosen shape: visibility = location; sync = git. Sharing becomes a property of the substrate,
not a feature.**

```
vault/                      # private — private by construction, never leaves
├── log/  todos/  notes/    # no visibility metadata anywhere
├── circles/
│   ├── team/               # ← a separate git repo; remote shared with the circle
│   │   ├── mike/           #   my contributions — I write only here
│   │   ├── jim/            #   Jim's — a read-only mirror for me
│   │   └── index.md        #   generated (OKF progressive disclosure)
│   └── family/             # another circle, another repo, another remote
```

Properties, each structural rather than enforced:
- **Private-by-default is free** — private is everywhere outside `circles/`; sharing is a physical,
  auditable act (`ls circles/team/mike/`). No per-note policy to forget.
- **Import/export verbs don't exist** — `git pull` is import, `git push` is share; lazy
  reconcile-on-read indexes pulled changes exactly as it catches Syncthing edits.
- **No merge conflicts, by construction** — per-author subdirs give disjoint write sets (a circle
  member's dir ≈ their remote branch). `author` provenance makes violations lintable.
- **History honesty** — a note shared by move enters the circle repo with fresh git history;
  pre-share private edit history *cannot* leak via `git log`. The prop/export model must work hard
  for this guarantee; here it is free.
- **ULIDs mesh the graph** — circle nodes index into the same property graph; cross-vault links and
  backlinks just work. The real collision hazard is **aliases**, not ULIDs — settled design below.
- **OKF interop rides free** — a circle repo is already nearly an OKF bundle (typed markdown +
  frontmatter + generated `index.md`); non-reckon consumers (a Karpathy brain, `kiso`) read it
  directly. The circle repo *is* the shared-KB artifact.

Verb surface (porcelain over git + already-planned machinery; start as an **`rk-circle` PATH
extension**, graduate into core when proven — the extension point exists for exactly this):
- `rk circle add <name> <remote>` — clone into `circles/`, ensure author subdir. Wraps git.
- `rk circle list|status|sync` — iterate circle repos; ahead/behind; pull --rebase + push. Wraps git.
- `rk share <id> --circle <c> [--move|--ref]` — sugar over the planned transactional `rk promote`:
  `--move` (ULID preserved; note lives shared thereafter) vs `--ref` (snapshot copy + `derived-from`;
  private original keeps evolving).
- One `rk lint` rule — edges from circle nodes → private nodes (link-leak; the *alias text* of a
  private target can itself leak information). Single index query.

**Alias namespacing (settled 2026-07-09, iterated with Mike).** Always-prefix, index-side only,
never on-collision: namespace-on-collision is history-dependent — adding a note could silently
rebind existing links, and a full rebuild could resolve differently than the incremental history
did, which threatens index disposability. Always-prefix is a pure function of the node — 
deterministic and rebuild-stable. Rules:
- **Namespace = the write-owner directory** (`team/jim::pas-entities`) — unique by construction
  (paths are unique) and *verifiable* by construction (members write only their own subdir; the
  dir is the trust boundary). NOT keyed on `author` — that field is self-asserted and non-unique
  (more than one Jim/Mike in the world, and it also carries agent personas); it narrows to
  provenance display + lint (foreign-author node in your subdir → flag), never resolution.
- **Self = per-circle config**, recorded at `rk circle add` (`circle.team.self = mike`): nodes in
  your own subdir index into the **root namespace** — so your private links keep resolving after a
  `--move` into a circle, and your agents' writes (author: Curly) in your subdir stay yours.
- **Scoped resolution, no fall-through**: an unqualified `[[alias]]` in a foreign file resolves in
  that writer's namespace; on miss it dangles — it must never silently bind to a private note
  (correctness + graph-pollution hazard). Cross-namespace links are explicit: `[[jim::alias]]`.
- **Shorthand via shortest-unique-prefix**: `[[jim::x]]` works while one `jim` dir exists across
  circles; two → **ambiguity is an error/dangling with a lint hint, never a guess**.
- Accepted cost: Obsidian's vault-global lookup diverges in the (previously undefined) ambiguous
  case, and qualified `[[jim::…]]` links show as dangling in Obsidian. Files stay Obsidian-valid.

Core changes required: indexer ignore-glob for nested `.git`, a derived `circle:` prop
materialized from `loc` (visibility-by-location stays queryable — the decision-log anti-corner is
type encoded *only* by location; a derivable storage/ownership fact is fine), and the alias
namespacing above. Everything else is porcelain.

**The complexity cliff, named:** reckon never manages identity, membership, invitations, or
permissions. Circle membership *is* repo access control — the host's problem (a private GitHub
repo's collaborator list). The day this feature grows an ACL, it has failed.

Costs, accepted eyes-open: a note cannot live in a topic hierarchy *and* a circle (the graph, not
the path, is the organization — links/tags/query are path-blind); multi-circle sharing = ref-copies
(mild duplication, rare); circle remotes need hosting; vault-level git must ignore `circles/`.

Status: **v2 seam.** First feature pulling reckon from single-user to multi-party — defer until a
lived consumer (an actual bundle exchange with Jim). v1 needs zero changes to keep the door open:
`circles/` is just a directory the indexer walks.

---

## Sources

- OKF spec: [SPEC.md — GoogleCloudPlatform/knowledge-catalog](https://github.com/GoogleCloudPlatform/knowledge-catalog/tree/main/okf) · [README](https://github.com/GoogleCloudPlatform/knowledge-catalog/tree/main/okf)
- OKF announcement: [Google Cloud blog, 2026-06](https://cloud.google.com/blog/products/data-analytics/how-the-open-knowledge-format-can-improve-data-sharing/) · [Google Cloud Tech on X](https://x.com/GoogleCloudTech/status/2067012903337664886)
- Karpathy LLM wiki: [gist](https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f) · OKF↔Karpathy framing: [themenonlab](https://themenonlab.blog/blog/google-okf-open-knowledge-format-karpathy-llm-wiki-standard) · pattern guides: [MindStudio](https://www.mindstudio.ai/blog/karpathy-llm-wiki-knowledge-base-pattern), [intelligentliving](https://www.intelligentliving.co/karpathy-llm-wiki-markdown-knowledge-base/)
- Cross-pollination (§1.4): [OKF ecosystem survey](https://www.owox.com/blog/articles/okf-ecosystem-tools) · [okf.md/tools](https://okf.md/tools/) · [Appleton, evergreens](https://maggieappleton.com/evergreens) · [garden history](https://maggieappleton.com/garden-history) · [Zep, "Markdown is not agent memory"](https://blog.getzep.com/markdown-is-not-agent-memory/) · [llms.txt spec](https://www.digitalapplied.com/blog/markdown-first-content-architecture-llms-txt-spec)
- In-repo: `docs/design/composable-redesign.md` (+ `-assessment.md`, `-rebuttal.md`), `docs/reckon-redesign_2026-06-15.md`, `docs/reckon-spec_2026-06-15.md`, `docs/design/spike-roundtrip-verdict.md`, `docs/2026-01-17-review-and-assessment.md`, `.beads/issues.jsonl` (v1 task state), git log through PR #150.
