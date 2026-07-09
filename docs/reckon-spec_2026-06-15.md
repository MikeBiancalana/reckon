# Reckon: Technical Spec

> **SUPERSEDED (2026-06-19)** with its companion redesign doc — halted at NO BUILD. Live design:
> **[`design/composable-redesign.md`](design/composable-redesign.md)** · doc map:
> [`design/INDEX.md`](design/INDEX.md). Kept for rationale.

_Date: 2026-06-15 · Author: Godfrey · Status: SUPERSEDED (was PROPOSED) · Companion to `reckon-redesign_2026-06-15.md`_

Concrete substance for the locked design: DB-first, build-not-buy, projection stays an honest grep-able mirror. This spec covers the schema, capture API, projection format, reconciliation, ref-extraction, and a buildable Phase 0.

---

## 1. Storage truth model

| Store | Truth | Concurrency | Purpose |
|-------|-------|-------------|---------|
| `~/.reckon/reckon.db` | **operational truth** | high (15+ writers) | entries, tasks, refs/links, page index, FTS |
| `~/.reckon/archive/journal/YYYY-MM-DD.md` | **projection (OUTPUT)** | none — regenerated | durable, git-committed, grep-able human mirror of entries |
| `~/.reckon/pages/<path>/<slug>.md` | **files-as-truth (INPUT)** | low | knowledge pages, human/agent editable, real dir hierarchy |
| `~/.reckon/digests/` | cache | none | synthesized briefs, invalidated on new entry |

Rule of honesty: **journal markdown is OUTPUT only** — regenerable from DB, never hand-edited (hand-edits are overwritten on next projection). **`pages/` is INPUT** — files are truth, indexed into the DB. Two opposite directions, never conflated.

---

## 2. Schema (DDL)

```sql
-- ── SUBSTRATE: append-only, attributed, lossless ──────────────────────────
CREATE TABLE entries (
    id          TEXT PRIMARY KEY,   -- xid
    ts          INTEGER NOT NULL,   -- event time, unix epoch (honors AT= backfill)
    created_at  INTEGER NOT NULL,   -- wall-clock insert time (audit; differs on backfill)
    author      TEXT NOT NULL,      -- PROVENANCE: 'mike' | 'Curly' | 'Riker' | peer id
    channel     TEXT NOT NULL,      -- cli | agent | slack | tui | phone
    kind        TEXT NOT NULL,      -- log|progress|win|intend|decision|note|done|commit|carry|owed
    body        TEXT NOT NULL,      -- raw, verbose OK — never compressed at write
    supersedes  TEXT,               -- entry id this corrects (nullable); append-only correction
    thread      TEXT,               -- optional grouping id
    day         TEXT NOT NULL       -- 'YYYY-MM-DD' derived from ts (projection + day queries)
);
CREATE INDEX idx_entries_day    ON entries(day);
CREATE INDEX idx_entries_ts     ON entries(ts);
CREATE INDEX idx_entries_author ON entries(author);
CREATE INDEX idx_entries_kind   ON entries(kind);

CREATE VIRTUAL TABLE entries_fts USING fts5(body, content='entries', content_rowid='rowid');
-- INSERT trigger keeps FTS in sync; append-only means no UPDATE/DELETE churn.

-- ── REFS: raw typed references extracted from a source (may not resolve yet) ─
CREATE TABLE refs (
    id          TEXT PRIMARY KEY,
    src_kind    TEXT NOT NULL,      -- entry | task | page
    src_id      TEXT NOT NULL,
    ref_type    TEXT NOT NULL,      -- jira | pr | page | task | person
    ref_value   TEXT NOT NULL,      -- 'SNP-35314' | 'gh:rbp-api#5759' | 'PAS/PAS-Entities' | '@drew'
    resolved_id TEXT                -- id of resolved task/page once a resolver pass links it
);
CREATE INDEX idx_refs_src   ON refs(src_kind, src_id);
CREATE INDEX idx_refs_value ON refs(ref_type, ref_value);

-- ── TASKS: pillar A (hand-rolled, no third-party backend) ──────────────────
CREATE TABLE tasks (
    id            TEXT PRIMARY KEY,
    title         TEXT NOT NULL,
    kind          TEXT NOT NULL,    -- anchored | anchorless
    external_ref  TEXT,             -- 'SNP-35314' | 'gh:rbp-api#5759' | NULL for anchorless
    state         TEXT NOT NULL,    -- anchored: reconciled from SoT; anchorless: reckon-owned
    owner         TEXT,             -- REQUIRED when anchorless
    created_at    INTEGER NOT NULL,
    due           INTEGER,
    ttl_days      INTEGER,          -- anchorless staleness window
    reconciled_at INTEGER,          -- anchored only
    source_state  TEXT,             -- raw state pulled from SoT (drift display)
    closed_at     INTEGER
);
CREATE UNIQUE INDEX idx_tasks_extref ON tasks(external_ref) WHERE external_ref IS NOT NULL;
CREATE INDEX idx_tasks_kind_state ON tasks(kind, state);

-- ── PAGES: index over files-as-truth knowledge docs ────────────────────────
CREATE TABLE pages (
    slug       TEXT PRIMARY KEY,    -- 'PAS/PAS-Entities'  (= path minus ext)
    title      TEXT NOT NULL,
    path       TEXT NOT NULL,       -- 'pages/PAS/PAS-Entities.md'
    tags       TEXT,                -- json array
    updated_at INTEGER NOT NULL
);
CREATE VIRTUAL TABLE pages_fts USING fts5(slug, title, body, tags);

-- ── LINKS: resolved typed edge graph (derived from refs) ───────────────────
CREATE TABLE links (
    src_kind  TEXT NOT NULL, src_id TEXT NOT NULL,
    dst_kind  TEXT NOT NULL, dst_id TEXT NOT NULL,
    link_type TEXT NOT NULL,         -- mentions | relates | reconciles | owned-by
    PRIMARY KEY (src_kind, src_id, dst_kind, dst_id, link_type)
);
```

Two-stage linking (`refs` → `links`) mirrors how a `[[name]]` can point at something that doesn't exist yet: extraction is cheap and always succeeds; resolution runs later and only links what exists. Nothing is lost if a referenced page/task isn't created yet — the ref persists and resolves when the target appears.

---

## 3. Capture API (the front door — built first)

One append primitive. Sub-second. A single INSERT on a WAL DB — no file touch on the hot path (projection is async).

### CLI
```
rk add [flags] <body...>
  --kind     log            # log|progress|win|intend|decision|note|done|commit|carry|owed
  --at       HH:MM          # backfill ts to today HH:MM (or full RFC3339 for other days)
  --author   <name>         # override provenance
  --channel  cli            # override channel
  --thread   <id>           # group under a thread
  --supersedes <entry-id>   # correction
```

Muscle-memory aliases (thin wrappers that set `--kind`), preserving the `work_system.sh` verbs:
```
rk log "..."        rk progress "..."   rk win "..."     rk intend "..."
rk done "..."       rk commit "..."     rk carry "..."   rk owed "..."
```

### Author / provenance resolution (immutable once stamped)
```
--author flag  >  $RECKON_AUTHOR env  >  OS user ('mike')
```
Agents export `RECKON_AUTHOR=<persona>` (Curly, Riker, callimachus…). Satisfies the verified-provenance rule: no anonymous entries.

### Agent interface
- **Primary (dead-simple): shell out to the CLI.** `rk add --author Curly --kind progress "SNP-35314 endpoint built"`. No SDK to maintain; every agent already runs Bash.
- **Library**: a thin Go `capture` package for in-process callers.
- **HTTP append** (Phase 5): `POST /entries` for Slack/phone into the same code path.

### Backfill
`AT=HH:MM` semantics preserved from `work_system.sh` (`reference_work_system_at_override`): `ts` = backfilled, `created_at` = real insert time. Both retained — the projection orders by `ts`, the audit trail keeps `created_at`.

---

## 4. Markdown projection (guardrail-critical)

One file per day, `archive/journal/YYYY-MM-DD.md`, regenerated from `entries WHERE day = ?`. **Deterministic & idempotent**: same rows → byte-identical output. Sorted by `(ts, id)`.

```markdown
# 2026-06-15

## 08:38 progress · Curly
SNP-35314 manual-trigger endpoint built; exercises watermark bulk-pull/fan-out on
existing queue topology, non-destructive.
↳ refs: SNP-35314

## 10:17 win · mike
Hardened Riker Jira formatting — root-caused the literal-glyph bug.

## 11:19 progress · Godfrey
~~superseded by 12:04~~ → see entry c8x… 
```

Format rules (keep it honest & grep-able):
- Header line per entry: `## HH:MM <kind> · <author>` — greppable by time, kind, or author (`grep '· Curly'`, `grep ' win '`).
- Body verbatim below (multi-line preserved).
- Refs as a trailing `↳ refs:` line — greppable (`grep 'SNP-35314' archive/journal/*.md`).
- Superseded entries render struck-through with a pointer to the superseder — **complete and honest**, never silently dropped.
- No frontmatter noise, no IDs in the human flow (IDs live in DB); the file reads clean.

Regeneration:
- `rk project [--day YYYY-MM-DD | --all]` regenerates touched day files.
- A debounced background committer (or `rk project --commit` on cron / launchd) runs `git add -A && git commit` in `~/.reckon`. Projection is eventually-consistent with the DB; the DB is always authoritative.
- Hand-edits to journal projection files are **ignored and overwritten** (they are output). Pages are where editing happens.

---

## 5. Tasks & reconciliation (pillar A)

### Adapter interface
```go
type SourceAdapter interface {
    Kind() string                                  // "jira" | "github"
    Matches(externalRef string) bool               // claims 'SNP-…' or 'gh:…'
    Reconcile(ctx context.Context, ref string) (state string, sourceState string, err error)
}
```
- **Jira adapter** → Atlassian MCP / the riker domain. Maps Jira status → reckon state.
- **GitHub adapter** → `gh` CLI / the plimsoll lifecycle. Maps PR state → reckon state.

### State mapping (initial; tunable)
| reckon state | Jira | GitHub PR |
|---|---|---|
| `ready` | Ready for Dev | open + approved |
| `active` | In Progress | open + changes/CI pending |
| `blocked` | Blocked | open + failing checks |
| `done` | Done / Closed | merged |
| `draft` | — | draft |

Honors existing domain rules: **Ready-for-Dev dwell is not stale** (`feedback_jira_ready_for_dev_dwell`) — `ready` tasks are not flagged; **behind-main is not a blocker** (`feedback_behind_main_not_a_blocker`).

### Loop
- `rk reconcile [--task <id> | --all]`: for each anchored task, call the matching adapter, update `state`, `source_state`, `reconciled_at`; record drift if `state` changed.
- Runs on demand, on schedule (launchd/cron), and folded into brief generation.
- **Agents never hand-flip anchored state** — they create the task with an `external_ref`; reconcile owns state thereafter.

### Anchorless tasks
- No adapter. `owner` + `ttl_days` required. Staleness: `now - created_at > ttl_days` → surfaces in the checklist as "stale — confirm or fin." Never silently rots.

### Creation
```
rk task add "title" --ref SNP-35314          # anchored — state reconciled immediately
rk task add "Send Drew the lease doc" --owner mike --ttl 7d   # anchorless
```

---

## 6. Ref extraction & auto-linking (cross-cutting #4)

Runs at insert (and on `rebuild`). Extract → write `refs` rows. A resolver pass turns resolvable refs into `links`.

Grammar (conservative — avoid noise):
| ref_type | pattern | example |
|---|---|---|
| jira | `\b[A-Z]{2,}-\d+\b` | `SNP-35314` |
| pr | `\bgh:([\w.-]+)#(\d+)\b` or `\bPR\s*#(\d+)\b` | `gh:rbp-api#5759` |
| page | `\[\[([\w/-]+)\]\]` | `[[PAS/PAS-Entities]]` |
| person | `@([\w-]+)` | `@drew` |
| task | `task:([a-z0-9]+)` | `task:c8x…` |

Resolver: a `jira` ref resolves to the task mirroring that `external_ref` (creating the edge `entry —mentions→ task`); a `page` ref resolves to a `pages.slug` if it exists. Unresolved refs persist and resolve when the target appears. **No hand-typed links anywhere** — this is the `[[ ]]` value without the friction that made Mike never use it.

---

## 7. Read-time synthesis (pillar C)

- `rk brief [--day today]` → gather day's entries + open/at-risk tasks + reconciliation drift + stale anchorless → Lycurgus-as-reader (LLM pass) → markdown brief. Cached in `digests/`, invalidated when a new entry lands for that day.
- `rk recall <query>` → FTS over `entries_fts` + `pages_fts`, expanded via the `links` graph → synthesized answer. callimachus's retrieval runs here.
- Brief delivery reuses the David/Gilbert brief plumbing (Slack canvas). Mike's primary read surface; he never reads raw entries.

Lycurgus role flip: write-time compressor → **read-time digest generator** over lossless substrate.

---

## 8. Phase 0 — concrete first slice (buildable)

**Goal:** the front door + substrate, replacing the `work_system.sh` log pipeline end-to-end for one agent.

Deliverables:
- `internal/store` — DB-first: `entries` table, WAL, append fn, FTS + insert trigger.
- `rk add` + the kind-alias wrappers; author/channel resolution; `AT=` backfill.
- Ref extraction at insert (write `refs`; resolution deferred to a later phase).
- `rk project [--day|--all]` — deterministic markdown regeneration + optional `--commit`.
- `rk today` / `rk week` read from DB (not files).

Acceptance criteria:
- 15 concurrent `rk add` invocations from parallel shells → all land, none lost (WAL append).
- Every row has a non-null `author`.
- `rk project` twice on unchanged data → byte-identical files (deterministic).
- `grep SNP-35314 archive/journal/*.md` finds the ref line.
- One real agent (e.g. Curly) sets `RECKON_AUTHOR` and logs progress via `rk add` for a full session; projection is clean and readable.

Explicitly **not** in Phase 0: reconciliation, pages, the LLM synthesis brief, TUI. (Phases 1–5 per the redesign doc.)

---

## 9. Migration specifics

- **Pages** (Phase 3): walk `~/logseq/pages/*.md`; de-mangle `A___B.md` → `pages/A/B.md` (split on `___`, each segment a dir level, final segment the file); preserve frontmatter; index into `pages` + `pages_fts`. ~153 files.
- **Journals**: not imported. `~/logseq/journals/` archived read-only.
- **Anchored tasks**: not migrated — created with `external_ref` and rebuilt from source on first reconcile.
- **Anchorless personal commitments**: hand-migrate the few live ones (`rk task add … --owner … --ttl …`).

---

## 10. Deferred / assumed (pending Mike)

- **Schedule feature**: recommend drop → Google Calendar (Mike has the MCP). Not carried into the new schema above.
- **Rebuild scope**: keep stack (Go/Bubble Tea/Cobra/SQLite); harvest parser/writer (for projection + page parsing) and TUI components; rebuild storage/capture/task/sync. Middle path — not greenfield, not in-place patch.
