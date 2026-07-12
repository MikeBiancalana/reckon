# Acceptance Criteria — reckon-s6oh (v1-T9: migrate DB-primary data to text-truth)

## 0. What this ticket is

A **one-shot importer + verifier**, not a live write-path cutover. It reads the
legacy SQLite-primary store (`~/.reckon/reckon.db` + `~/.reckon/{journal,tasks,notes}/`)
and the legacy journal markdown, and produces vault-native text-truth files
(`todos/`, `notes/`, `checklists/`, `log/`) that the T1–T8 tooling already
understands. Old `rk log`/`rk notes`/`rk task`/`rk checklist` (SQLite-backed)
keep running unmodified after this ticket lands — see §5.

Source of the "long pole, beside the keystone" framing:
`docs/design/composable-redesign-assessment.md:79` (watch-item #4). Confirmed
independent scope: `internal/cli/add.go:34` — `rk add`'s own doc comment says
it "does not touch the legacy DB-journal `rk log` (owned by a different
ticket, reckon-s6oh/T9)".

## 1. Explicit acceptance criteria (from the ticket's "Done when:")

1. **AC1 — Matching counts.** Running the importer against a COPY of real data
   produces a migrated node count that matches the source record count, per
   type (tasks, notes, checklist templates, checklist runs, journal
   days/entries) — see §2 for what "matching" means per source table given
   several tables have no 1:1 target concept.
2. **AC2 — Spot-checked round-trip.** A sample of migrated nodes, re-parsed via
   `internal/node.Parse`, reproduces the same typed fields the importer wrote
   (title/body/props/aliases/links) — i.e. the node package's own
   `parse(serialize(parse(f))) == f` byte-identity guarantee holds on
   importer-generated files exactly as it does on hand-authored ones
   (`internal/node/AGENTS.md` "byte-preservation invariant").
3. **AC3 — Old slug/id links resolve via alias.** A reference to a
   pre-migration identifier (old task/note `xid`, old note `slug`) still
   resolves post-migration, through the index's alias-resolution path
   (`internal/index/reconcile.go:375` `resolveEdges` — ULID first, then
   `_aliases`), not through any importer-side rewrite of the referencing
   text.
4. **AC4 — Dry-run + verify mode.** The importer supports a no-write
   preview (reports what it would create/skip/error, touches no files) and a
   post-run verify pass (checks AC1–AC3 against the already-written output).
5. **AC5 — Tested on sample data.** Covered by a fixture SQLite DB + fixture
   legacy journal/notes files, not only against production data.

## 2. Source → target mapping (the core design decision this ticket makes)

| Source (legacy) | Truth today | Target (vault) | Notes |
|---|---|---|---|
| `tasks` + `task_notes` (SQLite) | **DB-only** — no file at all (`internal/journal/AGENTS.md:111`: "Tasks are currently stored in SQLite only") | `todos/<ULID>.md` (durable todo, T5 shape) | 1:1 row→file. `task_notes` has no target field in the new todo schema (body-only; no `notes` concept in `internal/cli/todo.go`) — decide fold-into-body vs. drop; see EC-3. |
| `notes` + `note_links` (SQLite) **and** `~/.reckon/notes/YYYY/YYYY-MM/YYYY-MM-DD-slug.md` (file) | **Split** — DB is metadata authority (`notes_repository.go:34` `INSERT...ON CONFLICT DO UPDATE`); file holds body content | `notes/<slug>.md` (T8 shape: `id/type/aliases/title/description/stage`, slug-derived filename) | `note_links` is **always** `link_type="reference"`, produced only by re-parsing the file's own `[[wikilinks]]` (`internal/service/notes_service.go:108`) — `LinkTypeParent/Child/Related` are declared (`internal/models/note.go:31-33`) but never constructed anywhere in the codebase. **`note_links` rows do not need migrating** — the new index re-derives `references` edges from the migrated body text itself. |
| `checklist_templates`/`checklist_template_items`/`checklist_runs`/`checklist_run_items` (SQLite) | **DB-only** — `internal/checklist/repository.go` has zero `WriteFile` calls | `checklists/…` — **[OPEN] no target node shape exists.** No `rk checklist` v1 tool has shipped (deferred per `composable-redesign-assessment.md`'s lean-v1 list); `km-architecture-proposal.md:154` lists checklists only as a directory-tree placeholder ("templates + materialized runs"), with per-tool granularity decided in principle (`composable-redesign.md:313-314`: 1 file per template, 1 file per run) but no frontmatter schema, no `node.Parser`, no reserved `type` value. This ticket must invent one (e.g. `type: checklist-template` / `type: checklist-run`) using the same `NewNode`→`Render` recipe as todo/note, or explicitly scope checklists out and flag the gap. |
| `~/.reckon/journal/YYYY-MM-DD.md` (file, DB is a derived index per `internal/journal/AGENTS.md:248` "Files are source of truth, DB is rebuildable index" — the one legacy type that already claims file-primacy) | File-primary, old format | `log/<date>.md` (T4 `log-day`/`log-entry` shape, `internal/node/logparser.go`) | Format rewrite, not a DB read: old `## Intentions` / `## Schedule` / `## Log` / `## Wins` sections → new flat `## HH:MM · author` + `id::` entries (`node.RenderLogEntry`, `internal/cli/add.go:182`). Only `## Log` entries map cleanly (timestamp + content already fit). **Intentions/Wins/Schedule have no direct target in the log-entry model** — see EC-4. |

**Old→new directory/DB split** (confirms source paths): legacy = `~/.reckon/{journal,tasks,notes}/` +
`~/.reckon/reckon.db` (`internal/config/config.go:18-96`, `DataDir`/`JournalDir`/`TasksDir`/`NotesDir`/`DatabasePath`);
new = `Config.VaultDir` (`$HOME/reckon` by default, `internal/config/config.go:132`). These were
deliberately separated by reckon-lrw2 specifically so T9 wouldn't collide reading old data while
writing new — that ticket's close note says "Ties to T9 migration."

## 3. Implicit requirements

- **IR1 — xid→ULID alias mechanism is exactly the node package's existing `aliases:` field**, nothing
  bespoke. Every migrated node gets `aliases: [<old-xid>]` (todos, checklist rows) or
  `aliases: [<old-xid>, <slug>, ...]` (notes, which already key on slug — see IR2), written via
  `(*Node).SetAliases` or set directly on a fresh `NewNode` before first `Render` (mirrors
  `internal/cli/note_v1.go:310-322`'s `aliases := []string{slug}` pattern). Resolution is entirely
  the index's job (`resolveEdges`) — the importer never rewrites any text that references the old id.
- **IR2 — Notes already have a human alias (the slug) as their T8 filename.** A migrated note's
  aliases must include **both** the old xid *and* the old slug (filename is the *new*, possibly
  different, slug only if the old slug collides — see EC-2), so both a bare `[[old-slug]]` wikilink
  and a `[task:<xid>]`-style old reference keep resolving.
- **IR3 — ULID minting uses `node.Mint()`/`node.NewNode`, not a fresh crypto/rand call**, so migrated
  nodes sort with genuinely-authored nodes and pass the same monotonic/time-sortable contract
  (`internal/node/ulid.go`). Recommended: mint at the record's original `created_at`/`CreatedAt`
  timestamp via `node.MintAt(t)` rather than at migration-run time, so ULID lexical order still
  reflects authoring history instead of collapsing every migrated node into one import instant.
  **[INFERRED]** — not required by any ticket, but consistent with ULID's whole purpose and cheap to do.
- **IR4 — Every migrated file must satisfy the round-trip keystone.** Build via the established
  `NewNode → set fields → Render → Parse → writeFileAtomic` recipe (`internal/cli/todo.go:331-367`,
  `note_v1.go:320-344`, `internal/cli/adopt.go:250-284`'s `writeFileAtomic`), not hand-built strings —
  this is what makes AC2 hold by construction instead of by hope.
- **IR5 — Verify pass must exercise the real index**, not a parallel count mechanism: open (or build) an
  `internal/index.Index` over the migrated vault, `Rebuild()`, and assert `Stats.Warnings` contains no
  unexpected `duplicate_ulid`/`alias_collision` entries (`internal/index/reconcile.go:34-40`) — that
  warning machinery (reckon-5b44) is the system's designed detector for exactly the failure mode a bulk
  import risks (two migrated nodes claiming the same alias/ULID).
- **IR6 — Idempotency / resumability.** "Partial/interrupted migration runs" (ticket's own edge-case
  list) implies re-running the importer over already-migrated output must not duplicate or corrupt
  files. `writeFileAtomic`'s existing pattern is naturally re-run-safe for creates; the importer needs
  its own "already migrated" check per source record (e.g. skip if a vault file already carries the
  source's xid as an alias) — no existing primitive does this, it must be built. **[OPEN]** whether
  skip-if-alias-present is sufficient or a separate migration-state ledger is warranted.
- **IR7 — CRLF and conflict-marker guards already enforced by `node.Parse`/`adopt.go`'s CRLF refusal
  apply equally to migration output** — the importer must not produce CRLF files (matches `rk adopt`'s
  policy, `internal/cli/adopt.go:205-208`, "known parser-scope gap, reckon-vj55").

## 4. Edge cases

| # | Case | Handling expectation |
|---|---|---|
| EC-1 | Duplicate xid across source tables (e.g. a task and a note somehow share an id — not enforced by any SQLite `UNIQUE` across tables) | Each migrated node's alias is scoped by the old table's own PK space; a same-string collision across *types* is a real `alias_collision` the verify pass (IR5) must surface, not silently resolve. |
| EC-2 | Two old notes whose slugs collide with each other or with a T8-created note's slug (old `notes.slug` was `UNIQUE` within its own table only — no coordination with new `notes/` on disk) | New note filenames must go through the same `slugCollision` check `note_v1.go:651` uses, with a deterministic disambiguation (e.g. suffix) — the *old* slug still becomes an alias regardless of what the *new* filename ends up being. |
| EC-3 | `task_notes` (child notes on a task) — no target field on the new todo node | Decide: append to body as a rendered list, or drop with a reported count in the migration summary. Either is defensible; **silent drop with no count is not** (breaks AC1's "matching counts" framing at the sub-record level). |
| EC-4 | Old journal `## Intentions` / `## Wins` / `## Schedule` sections have no direct analog in the flat log-entry model | **[OPEN]**, not resolved by any dependency ticket. Candidates: render each as a plain log entry with a `kind` prop (log-entry `Props["kind"]` already exists per `internal/node/AGENTS.md:217`); or drop and count. Must be an explicit, documented decision, not an accidental omission. |
| EC-5 | Empty journal day (frontmatter + section headers, zero actual entries) | Should migrate to a `log-day` node with zero entries (or be skipped with a reported reason) — not error the whole run. |
| EC-6 | Checklist with zero items (empty `Items`) | `NewRun`/`NewTemplate` already tolerate this (`internal/checklist/model.go:71` `make([]RunItem, len(t.Items))` on an empty slice is a valid zero-length slice) — migrated file should render with no item lines, not error. |
| EC-7 | Note body containing `[[wiki links]]` to a note that itself failed to migrate (partial run, or a note referencing a since-deleted note whose DB row is gone but body text remains) | Becomes a dangling edge (`dst_key = NULL`) exactly like any other unresolved link — this is already a supported, queryable state (`internal/index/AGENTS.md:36`), not an importer failure. |
| EC-8 | Malformed / hand-edited legacy journal markdown that doesn't match the parser's regex assumptions (`internal/journal/parser.go`) | Old `ParseJournal` already has to tolerate this today; the importer inherits whatever it can/can't parse — anything it can't parse should be reported per-day, not abort the whole run. |
| EC-9 | Legacy note file missing entirely (DB row exists, `file_path` points at a file that's gone — DB/file drift is exactly the failure mode this whole redesign exists to end) | Report as an error/skip with the DB row's id; do not fabricate a body from nothing. |
| EC-10 | Interrupted mid-run (process killed after some files written, before verify) | Re-running must be safe (IR6) and a fresh `rk <migrate-verb> verify` must be able to assess partial output without assuming the whole source set was processed in one pass. |
| EC-11 | `--dry-run` must not require an index or touch the cache dir | Consistent with `rk adopt`'s `Annotations: {"requiresDB": "false"}` pattern (`internal/cli/adopt.go:32`) for the dry-run path; the verify-mode path legitimately does need the index. |

## 5. Out of scope

- **Cutting the SQLite-primary write path over.** `rk log`/`rk notes`/`rk task`/`rk checklist`
  (SQLite-backed, `internal/cli/root.go:117-127`) keep running unmodified; this ticket does not retire,
  redirect, or deprecate them. No dependency ticket or `v1-T*` entry assigns that cutover to anyone yet
  — **[OPEN]**, likely a future ticket.
- **Building a live `rk checklist` v1 text-truth tool.** Only the migration's own checklist *file
  shape* is this ticket's problem (§2); wiring a create/run CLI on top of it is explicitly deferred
  per `composable-redesign-assessment.md`'s lean-v1 list ("DEFERRED until a lived consumer appears:
  ... the checklist tool").
- **Logseq/Obsidian pages import.** That is T8's `rk adopt`-based migration
  (`km-architecture-proposal.md` §4.6, steps 2–4) — a different source (external Logseq vault) and
  different mechanism (`rk adopt` stamping ULIDs into already-text files), already shipped. Not
  reckon's own SQLite data.
- **Retiring the old `rk migrate` command.** `internal/cli/migrate.go` already exists under the verb
  `rk migrate` for an *unrelated, earlier* migration (opaque-ID → slug-based *task filenames*,
  reckon-2qh). T9's new importer needs its own verb/subcommand (e.g. `rk migrate text-truth` or a new
  top-level verb) — reusing bare `rk migrate run` would collide with that existing command's contract.
  **[OPEN]** naming choice, not specified by the ticket.
- **Archived-journal history beyond reckon's own DB-primary period.** `km-architecture-proposal.md`
  §4.6 point 3 keeps pre-existing Logseq journals "archived read-only, not imported" — out of scope
  here too by the same logic (this ticket only owns reckon's *own* journal files, already in the new
  `## Log`-section markdown format, not any older format).
- **Live reconciliation / two-way sync** between the legacy DB and the vault after migration. This is
  a one-shot import; the legacy DB is not kept in sync with post-migration vault edits.

## 6. Test scenarios (given/when/then)

**Counts & basic import**
1. Given a fixture SQLite DB with N tasks (mix of open/done, with/without schedule/deadline/depends),
   when the importer runs, then N `todos/<ULID>.md` files exist, each with `state` matching source
   `status`, `scheduled`/`deadline` present iff source had them, and a `depends-on` edge iff source
   `task_id` FK-equivalent existed.
2. Given a fixture DB with M notes each with a real on-disk legacy file, when the importer runs, then M
   `notes/<slug>.md` files exist with `title`/`aliases` matching DB, body matching the legacy file's
   body content.
3. Given a fixture DB with checklist templates/runs, when the importer runs, then the chosen
   checklist file shape (§2, EC-6) is produced 1:1 per template and per run.
4. Given K legacy journal day files, when the importer runs, then K `log/<date>.md` files exist as
   valid `log-day` nodes (`node.Type == "log-day"`), with `## Log` entries reproduced as `log-entry`
   nodes.

**Alias resolution (AC3)**
5. Given a migrated todo whose source task had `xid = "abc123"`, when `rk query`/index resolves an
   edge or reference to `"abc123"`, then it resolves to the migrated todo's ULID via `_aliases`
   (`internal/index/reconcile.go:375`).
6. Given a migrated note whose old slug was `"git-rebase"` and a *different* on-disk note file
   containing the literal wikilink `[[git-rebase]]`, when the index reconciles, then that edge's
   `dst_key` resolves to the migrated note (not NULL), proving old inbound links survive.
7. Given a note whose new slug (post-collision-disambiguation, EC-2) differs from its old slug, when
   the index reconciles, then both `aliases:` entries (old and new slug) are present and both resolve.

**Round-trip (AC2)**
8. Given any migrated node file, when re-`node.Parse`d, then `Serialize()` is byte-identical to the
   file on disk (the round-trip invariant holds on importer output, not just hand-authored input).
9. Given a migrated node's typed fields (title/props/aliases), when compared to the source DB row,
   then every field the target schema supports matches; fields with no target (EC-3, EC-4) appear in
   a reported "dropped/folded" summary, not silently.

**Dry-run / verify (AC4)**
10. Given a fixture DB, when the importer runs with `--dry-run`, then zero files are written under the
    target vault dir and the reported plan (create/skip/error per record) matches what a real run
    would do.
11. Given a completed real (non-dry-run) migration, when verify mode runs, then it reports the same
    counts AC1 requires and zero unexpected `duplicate_ulid`/`alias_collision` warnings from a fresh
    `Index.Rebuild()` (IR5).
12. Given a *dry-run* invocation, when it executes, then no `Config.CacheDir`/index files are created
    or modified (EC-11).

**Idempotency / partial runs (IR6, EC-10)**
13. Given a migration that already fully completed, when the importer is run a second time, then no
    duplicate files are created and no existing migrated file's ULID/aliases change (no-op on already-
    migrated records).
14. Given a migration interrupted after writing some but not all records (simulate: pre-seed the vault
    with a partial prior run's output), when the importer is re-run, then it completes the remaining
    records without duplicating the already-written ones, and a subsequent verify pass reports full
    counts.

**Edge cases**
15. Given a task with `task_notes` attached, when migrated, then the chosen EC-3 handling (fold or
    drop-with-count) is applied consistently and is visible in the migration summary output.
16. Given an empty journal day file (frontmatter only, no `## Log` entries), when migrated, then it
    produces a valid zero-entry `log-day` node (or is explicitly skipped-and-reported), not an error
    that aborts the run.
17. Given a note DB row whose `file_path` points at a file that no longer exists on disk, when
    migrated, then that record is reported as an error/skip (with the source id), and the run
    continues with the remaining records.
18. Given a checklist template with zero items, when migrated, then it produces a valid file with no
    item lines rather than erroring.
19. Given two source records (any type) that would produce colliding aliases (EC-1/EC-2), when
    migrated, then the collision is either deterministically disambiguated (notes' filename slug) or
    surfaced as a verify-pass warning (IR5) — never silently resolved to one arbitrary winner.
20. Given a legacy journal day file with an unparsable/malformed section, when migrated, then that
    day is reported per-day (id + reason) and the run continues rather than aborting entirely.

## 7. Open questions to resolve before/during implementation

- **[OPEN]** Checklist target node shape (frontmatter fields, `type` values) — no prior ticket defines
  this; §2 flags it as this ticket's own design decision to make.
- **[OPEN]** Journal `## Intentions`/`## Wins`/`## Schedule` → log-entry mapping (EC-4).
- **[OPEN]** `task_notes` target (EC-3): fold into body vs. drop-with-count.
- **[OPEN]** Command surface/verb name (avoid colliding with the existing unrelated `rk migrate`,
  §5).
- **[OPEN]** Idempotency mechanism (IR6): alias-presence check vs. a migration-state ledger.
