# Implementation Plan: reckon-s6oh (v1-T9 — migrate DB-primary data to text-truth)

## Summary

Build a one-shot importer + verifier that reads legacy reckon data (gen-1 tasks, SQLite notes, SQLite checklists, journal day files) and emits vault-native canonical nodes (`todos/`, `notes/`, `checklists/`, `log/`) that T1–T8 tooling already understands. Every migrated node is built via the established `node.NewNode → set fields → Render → Parse → writeFileAtomic` recipe (`internal/cli/todo.go:341-367`, `note_v1.go:320-344`), so the round-trip keystone (AC2) holds by construction. The old xid becomes a `Node.Aliases` entry so pre-migration references resolve through the index's existing alias path (`internal/index/reconcile.go:375`) with zero new resolver code (AC3). Legacy tooling (`rk task`/`rk notes`/`rk checklist`/`rk log`) is left running unmodified (§5 out-of-scope).

## Files to create / modify

New package `internal/textmigrate/` (name chosen to not collide with the unrelated `internal/migrate`, see D-Name):

| File | Responsibility |
|---|---|
| `migrate.go` | `Importer` struct (source data dir, legacy DB path, dest vault dir, `DryRun bool`), orchestration, `Report`/per-type result types, xid→newULID map build, idempotency alias-set. |
| `tasks.go` | gen-1 task → `todos/<ULID>.md`. |
| `notes.go` | SQLite note + legacy note file → `notes/<slug>.md`. |
| `checklists.go` | checklist template/run → `checklists/templates\|runs/<ULID>.md`. |
| `journal.go` | legacy journal day file → `log/<date>.md`. |
| `verify.go` | index-backed verify pass (`index.Open`→`Rebuild`, count + warning + alias-resolution asserts). |
| `writer.go` | shared `writeFileAtomic` (copy the exact helper from `internal/cli/adopt.go:250-284`) + CRLF guard, so the package has no `internal/cli` dependency. |
| `*_test.go` (per converter) + `migrate_test.go` | fixture-DB + fixture-file end-to-end (mirrors `internal/cli/adopt_test.go` real-`Execute()` style and `internal/spike/roundtrip/roundtrip_test.go`). |

New CLI + registration:
- `internal/cli/import.go` — `importCmd` (`Use: "import"`), flags `--dry-run`, `--verify`, `--source` (defaults to `config.DataDir()`); `Annotations{"requiresDB":"false"}` + `SilenceUsage:true`; `importResult` with per-type Created/Skipped/Errored/folded-count slices + `Pretty()`, printed via `output.ModeFromFlags`/`output.New` gated by `!(mode==output.Pretty && quietFlag)` (`adopt.go:92-121`).
- `internal/cli/root.go:135` — `RootCmd.AddCommand(importCmd)`.

Read-only call targets (no changes): `internal/node/{node,render,logparser,ulid}.go`; `internal/index` (`Open`, `Rebuild`, `Stats`/`Warning`); `internal/config` (`DataDir`/`DatabasePath`/`TasksDir`/`NotesDir`/`JournalDir` source, `LoadWithOverrides` dest); `internal/storage.NewDatabase`; `internal/journal.TaskService.GetAllTasks`/`ParseJournal`; `internal/service.NewNotesRepository(...).GetAllNotes`; `internal/checklist.NewRepository(...).ListTemplates/ListRuns`.

## Design decisions

**D-Name (open Q3 — command/package collision).** Package `internal/textmigrate`; command `rk import` (single verb, `--dry-run`/`--verify` flags, matching the `adopt`/`index` single-verb-with-flags shape). Rejected `rk migrate <sub>` — `rk migrate run/status/rollback` already exist for the unrelated xid-filename→slug migration (`internal/cli/migrate.go:14-27,160-163`); reusing the verb collides. `rk import` is unregistered today (`root.go:117-135`) and reads as "import my legacy data."

**D-Config-bridge (open Q4).** Source = `config.DatabasePath()` opened via `storage.NewDatabase` + `config.{Tasks,Notes,Journal}Dir()`; honors the `RECKON_DATA_DIR` override so tests can point the whole legacy family at a fixture dir. Dest = `config.LoadWithOverrides(vaultFlag,"")` → `Config.VaultDir` (honors `--vault`/`RECKON_VAULT`). The `Importer` takes both roots as explicit fields (no global singletons) so tests inject fixtures directly; the CLI wires `--source` (default `config.DataDir()`) → source, `cfg.VaultDir` → dest, `cfg` → verify's `index.Open`. `requiresDB:false` because the command opens the *legacy* DB itself and must not trigger `root.go`'s global `initServiceE` against the default DB.

**D-Tasks.** Source gen-1 tasks via `journal.TaskService.GetAllTasks()` (file-primary reader the live `rk task` uses, `task_service.go:149`), NOT a `tasks` SQLite read — resolves the doc contradiction (AGENTS.md:111 is stale; `task_writer.go`/`GetAllTasks` prove file-primacy). Emit `todos/<ULID>.md`, `type:todo`, `Props{state}` (`open`/`done` from `Task.Status`), `scheduled`/`deadline` from `Task.ScheduledDate`/`DeadlineDate` iff set, body = `Task.Text` (+ `Description`). `Aliases:[oldXid]`. `Task.Tags` preserved as a `tags: [a, b]` prop (inert to T5, lossless, round-trips as a non-ref Props value per `node.go:382-392`). No `depends-on`: gen-1 `Task` has no dependency field, so scenario 1's depends edge is vacuously satisfied.

**D-TaskNotes (open Q3 / EC-3).** `Task.Notes` folded into the todo body as a trailing `### Notes` list; per-task folded count reported in the summary. Rejected drop (loses user content) and a bespoke sub-node type (over-build). Silent drop is explicitly disallowed by AC1.

**D-Notes.** Source `service.NewNotesRepository(db).GetAllNotes()` (DB = metadata authority). Body = read `filepath.Join(NotesDir, note.FilePath)` and strip its legacy frontmatter (keep body only). Emit `notes/<slug>.md`, `type:note`, `Props{title, tags?}`, `Aliases:[oldXid, oldSlug]` (IR2 — both must resolve). New filename slug goes through the same `slugCollision` check `note_v1.go:651` uses with deterministic suffix disambiguation (EC-2); the *old* slug remains an alias regardless. `note_links` rows are NOT migrated — the index re-derives `references` edges from body `[[wikilinks]]` (acceptance-criteria §2). EC-9: DB row whose `file_path` is missing on disk → per-row error/skip, run continues.

**D-Checklists (open Q1 — no prior schema; DECISION: in scope, minimal schema).** Kept in scope (ticket names them; schema is cheap via the same recipe; descoping breaks AC1 counts). Define, per the already-agreed 1-file-per-template/run granularity (`composable-redesign.md:313-314`) under the `checklists/` placeholder (`km-architecture-proposal.md:154`):

| Node | Path | type | Props | Body | Aliases | Links |
|---|---|---|---|---|---|---|
| Template | `checklists/templates/<ULID>.md` | `checklist-template` | `name` | `- item` lines (position order) | `[oldXid]` | — |
| Run | `checklists/runs/<ULID>.md` | `checklist-run` | `status`, `started`, `completed?` | `- [x]`/`- [ ] item` lines | `[oldXid]` | `instance-of: [[<template-oldXid>]]` |

Items are body lines, not per-item nodes (no per-item ULID minting — minimal). Run→template link uses the template's **old xid**, which resolves via the template node's alias (no cross-record ULID map needed). Counts: templates 1:1, runs 1:1; items are sub-records surfaced in the summary. EC-6: zero-item checklist renders with no item lines, no error. Flag to owner: no v1 consumer reads these yet — they index as generic nodes only.

**D-Journal (open Q2 / EC-4).** Only `## Log` entries become typed `log-entry` sub-nodes — exactly the ticket's stated mapping. Build the day file the way `add.go` does: `node.NewNode("log-day", …)` with `Aliases:[date]`, one `node.RenderLogEntry(hhmm, author, MintAt(ts), content)` block per `## Log` entry (`add.go:184-237`, `logparser.go:182`). Log-entry ULIDs mint fresh (legacy entries have position IDs, not xids — no alias to carry). `Intentions`/`Wins`/`Schedule` are preserved **losslessly as an H3 (`### Intentions`, etc.) preamble block before the first `## HH:MM` entry**, with per-section counts reported. Rationale: they have no v1 consumer and no timestamp; forcing them into timed log-entries fabricates times, and mapping to todos is speculative (risks double-migration if a real intentions tool later ships). H3 (not `## `) is required so `SplitEntries`/`LogParser` (`node.go:585` matches `^## ` only) does not mis-split them into phantom empty-time log-entries. Rejected: intentions→todo / wins→kinded-entry (speculative, no consumer); drop-with-count (lossy when preservation is free). EC-5 empty day → valid zero-entry `log-day`. EC-8/scenario 20: a day section that `ParseJournal` can't handle is reported per-day (date + reason), run continues.

**D-TaskRef-rewrite (AC3 for tasks).** Within the journal format rewrite, recognized inline `[task:<xid>]` markers in `## Log` content are converted to `[[<xid>]]` so the index derives a `references` edge resolving to the migrated todo via its xid alias (scenario 5). This is canonicalization of the migrated content itself, not rewriting an external referencer, so it stays within IR1's intent; rewritten-ref count is reported. Conservative alternative (leave `[task:xid]` verbatim, non-resolving) noted and rejected as AC3-incomplete for the task-reference form.

**D-MintAt (IR3).** All nodes mint via `node.MintAt(sourceCreatedAt)` (not `Mint()`), so ULID lexical order reflects authoring history. `Task.CreatedAt`/`Note.CreatedAt`/`Template.CreatedAt`/`Run.StartedAt`/entry timestamp all feed `MintAt`. `node.Time` = the same instant formatted RFC3339.

**D-Time (REVIEW_PATTERNS:929, reckon-gcuu).** One conversion rule, tested explicitly: legacy `time.Time` values (`Task.CreatedAt` is UTC-midnight from `time.Parse("2006-01-02",…)`, `task_service.go:200`) → `t.UTC().Format(time.RFC3339)`. Log entry `time` composed as `date+"T"+hhmm+":00Z"` (matches `add.go:221`), keeping both halves on one clock.

**D-Idempotency (open Q5 / IR6).** Skip-if-alias-present, no ledger. Before writing each type, scan the target subdir, parse existing nodes, build the set of aliases already present; a source record whose old xid is already an alias is reported skipped (scenarios 13, 14). For logs the marker is the day file: an existing `log/<date>.md` (from a prior run or `rk add`) is skipped and reported (avoids duplicating entries). Rejected a separate state ledger (second source of truth that can drift from the files, which are the durable marker).

**Error strategy (REVIEW_PATTERNS:21, :175).** Per-record failures `%w`-wrap with the source id and accumulate into the result (`res.addErrored`, `adopt.go:70-72`); the run processes every record and returns one summarizing `fmt.Errorf` only at the end. No `os.Exit` in the package. CRLF guard before parsing any legacy `.md` (`adopt.go:205-208`); conflict-marker files fail per-item, not crash (`node.go:119-123`).

**Dry-run / verify.** `--dry-run` walks every source and produces the same create/skip/error plan a real run would, writing zero files and never opening the index/CacheDir (EC-11, scenarios 10, 12). `--verify` opens `index.Open(cfg)`, `Rebuild()`, and asserts: (a) `Stats.Warnings` has no unexpected `duplicate_ulid`/`alias_collision` (IR5, `reconcile.go:34-40`); (b) per-`type` row counts equal source counts (AC1); (c) each migrated old id resolves via `SELECT id FROM aliases WHERE alias=?`. Reuses `rk index`'s count-query template (`index.go:97-110`).

## Test scenarios

Implement the acceptance-criteria §6 given/when/then list verbatim — do not re-derive:
- Counts & basic import: scenarios 1–4 (tasks→todos w/ state+scheduled+deadline; notes→`notes/<slug>.md` w/ title+aliases+body; checklists 1:1; journal days→`log-day` w/ `## Log`→`log-entry`).
- Alias resolution (AC3): 5 (task xid via `_aliases`), 6 (inbound `[[old-slug]]` resolves), 7 (old+new slug both alias & resolve).
- Round-trip (AC2): 8 (`Serialize()` byte-identical after reparse), 9 (typed fields match source; EC-3/EC-4 drops appear in a reported summary).
- Dry-run/verify (AC4): 10 (no files written, plan matches), 11 (verify counts + zero unexpected warnings), 12 (no CacheDir/index touched on dry-run).
- Idempotency/partial (IR6/EC-10): 13 (full re-run is a no-op), 14 (partial re-run completes remainder, verify reports full counts).
- Edge cases: 15 (task_notes fold/drop visible in summary), 16 (empty day → zero-entry log-day), 17 (missing note file → error/skip, continue), 18 (zero-item checklist), 19 (colliding aliases disambiguated or surfaced, never silently resolved), 20 (malformed journal section reported per-day, continue).

Plus REVIEW_PATTERNS-mandated edge tests: empty legacy store (zero rows, all types), a row with a missing/malformed field, and a cross-type same-string xid collision (EC-1) surfaced by verify.

## Known risks / ambiguities

- **[OPEN→owner] Checklist consumer.** The invented `checklist-template`/`checklist-run` schema (D-Checklists) has no v1 tool reading it yet — files migrate and index as generic nodes but are not otherwise consumable until a future `rk checklist` ticket. Flag the chosen `type`/props to the owner so the eventual tool matches.
- **[INFERRED] Task source contradiction.** Resolved in favor of file-primary (D-Tasks). If a deployment somehow has DB task rows without corresponding `tasks/*.md` files, `GetAllTasks` (and thus the importer) won't see them; verify's count is taken against the same file-based reader, so AC1 stays internally consistent but is blind to any DB-only orphans. Low risk given `task_writer.go` writes a file on every task create.
- **AC3 scope for `[meeting:name]`/`[break]`.** D-TaskRef-rewrite only rewrites `[task:xid]`; `[meeting:]`/`[break]` markers have no migration target and stay verbatim text (documented, not silently changed).
- **Global flat alias namespace.** Thousands of migrated xids share one namespace (`composable-redesign.md:963`); the index flags but does not error on collisions (`reconcile.go:389-480`). Verify must surface these; a real collision blocks "done."
- **Log entry re-mint on partial-day re-run.** Idempotency is per-day-file, not per-entry; a day file that was half-written then deleted mid-run would re-mint all its entries' ULIDs on re-run. Acceptable because a day is written atomically (`writeFileAtomic`) as one file — it is either fully present (skipped) or absent (fully rewritten), never half.

### Critical Files for Implementation
- internal/cli/adopt.go (structured-result + `writeFileAtomic` + CRLF-guard template)
- internal/cli/todo.go (durable-todo create recipe; target todo shape)
- internal/cli/add.go (log-day create/append + entry-time composition)
- internal/node/render.go (the `NewNode`/`Render` create primitive every converter uses)
- internal/index/reconcile.go (verify pass: `Stats`/`Warning`, alias resolution)
