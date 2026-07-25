# reckon-fnqs.11 — Codebase Analysis: `rk checklist` CLI verb surface

## 1. `internal/checklist` Service interface

All methods on `*Service` (`internal/checklist/service.go`), backed by `*Repository` (`internal/checklist/repository.go`):

| Method | Signature | Errors |
|---|---|---|
| `NewService` | `NewService(repo *Repository) *Service` (service.go:14) | — |
| `CreateTemplate` | `CreateTemplate(name string, items []string) (*Template, error)` (service.go:19) | empty name; duplicate name (`%q already exists`); wrapped repo error |
| `GetTemplate` | `GetTemplate(nameOrID string) (*Template, error)` (service.go:43) | tries name then ID; `%q not found` |
| `ListTemplates` | `ListTemplates() ([]*Template, error)` (service.go:58) | repo error only |
| `DeleteTemplate` | `DeleteTemplate(nameOrID string) error` (service.go:63) | not required by ticket; exists |
| `AddTemplateItem` | `AddTemplateItem(nameOrID, text string) error` (service.go:72) | not required by ticket; exists |
| `RemoveTemplateItem` | `RemoveTemplateItem(nameOrID string, position int) error` (service.go:86) — **0-based** position | `position %d out of range`; not required by ticket but exists |
| `StartRun` | `StartRun(nameOrID string) (*Run, error)` (service.go:107) | template not found; `"an active run already exists for %q (use 'reset' to start fresh)"` if one is active |
| `GetActiveRun` | `GetActiveRun(nameOrID string) (*Run, error)` (service.go:129) | `"no active run for %q (use 'start' to begin)"` if none |
| `CheckItem` | `CheckItem(runID string, position int) error` (service.go:147) — **0-based**, takes a **run ID**, not a template name | position out of range; auto-completes run when all items checked |
| `GetRunStatus` | `GetRunStatus(runID string) (*Run, error)` (service.go:187) | thin wrapper over `repo.GetRunByID` |
| `ResetRun` | `ResetRun(nameOrID string) (*Run, error)` (service.go:192) | abandons existing active run (if any), starts fresh |
| `AbandonRun` | `AbandonRun(nameOrID string) (*Run, error)` (service.go:218) | `"no active run for %q"` if none active (also true for already-completed/abandoned) |
| `ListRuns` | `ListRuns(includeCompleted bool) ([]*Run, error)` (service.go:240) | `includeCompleted=false` → active only |

All errors are `fmt.Errorf("...: %w", err)`-wrapped strings, no sentinel/typed errors — callers match on substring if needed (none of the recovered v0 code did).

**`check <template> <position>` requires two service calls**, not one: `GetActiveRun(template)` → take `.ID` → `CheckItem(run.ID, pos0based)`. There is no single "check by template name" method.

## 2. storage.Database wiring

- `checklist.NewRepository(db *storage.Database) *Repository` (repository.go:17) — needs a live `*storage.Database`.
- `storage.NewDatabase(path string) (*Database, error)` (internal/storage/database.go:188) opens/pings/creates schema/runs migrations. The `checklist_templates`/`checklist_template_items`/`checklist_runs`/`checklist_run_items` tables are already baked into the schema constant (database.go:115-153) and indexed (database.go:156-159) — **no schema/migration work needed**.
- Path resolution: `config.DatabasePath()` (internal/config/config.go:88) returns `~/.reckon/reckon.db`, honoring `RECKON_DATA_DIR` env override (config.go:18-38, primarily for tests) — this is the exact path the pre-purge `initServiceE` used for the shared `db` that fed `journalService`/`checklistService` (recovered `root.go` @ 83ca4ef^, "failed to get database path" → `storage.NewDatabase(dbPath)` → `checklist.NewRepository(db)`).
- **Currently zero live callers of `config.DatabasePath()` or `storage.NewDatabase()` in `internal/cli`** (only `internal/textmigrate/{notes,checklists,verify}.go`, the legacy-import tool, call `storage.NewDatabase` directly) — confirms the domain layer is fully orphaned as the ticket states.

## 3. Existing `rk <noun> <verb>` conventions (post-purge, v1)

Reference files: `internal/cli/root.go`, `todo.go`, `note.go`/`note_v1.go`, `index.go`, `output/output.go`.

- **Registration**: top-level commands are package-level `cobra.Command` vars, added once in `root.go`'s `init()` via `RootCmd.AddCommand(...)` (root.go:101-109: `GetNoteCommand()`, `todayCmd`, `addCmd`, `todoCmd`, `queryCmd`, `indexCmd`, `adoptCmd`, `migrateCmd`, `tuiCmd`). `note.go` uses a `GetNoteCommand()` indirection only because it's split across `note.go`/`note_v1.go`; `todo.go`/`index.go` register their parent var directly. **Recommend the direct-var pattern** (`checklistCmd`, add to root.go's list) — simpler, matches the majority.
- **Subcommand files**: one file per noun (`todo.go`, `note_v1.go`, `index.go`), each with its own `init()` calling `parentCmd.AddCommand(childCmd1, childCmd2, ...)` and its own flag vars/`resetXFlags` helper.
- **`--json`/`--ndjson`**: global persistent flags (root.go:96-97, `jsonFlag`/`ndjsonFlag`), mutually exclusive (enforced twice: root.go:84-86 in `PersistentPreRunE`, and again per-command via `output.ModeFromFlags(jsonFlag, ndjsonFlag)`, e.g. todo.go:298, index.go:22). Every command constructs `mode, err := output.ModeFromFlags(...)` then calls `output.New(cmd.OutOrStdout(), mode).Print(res)` where `res` is a small struct with json tags implementing `Pretty() string` (output/output.go:26-77). **This is the convention to follow** — not v0's bare `fmt.Printf`/`os.Stdout`.
- **`--quiet`**: pretty-mode-only suppression, pattern is `if !(mode == output.Pretty && quietFlag) { ...Print... }` (todo.go:323, index.go:54-56).
- **Repeatable flags**: `cf.StringArrayVar(&noteTagFlag, "tag", nil, "Tag (repeatable)")` (note_v1.go:125-126) is the established idiom for `--item`/`--tag`-style repeatable flags. **No existing precedent for a `--items-file` bulk-file-input flag anywhere in the repo** (grepped `internal/cli`, `internal/textmigrate`) — this will be net-new; plain `StringVar` + `os.ReadFile` + line-split-and-trim (skip blank lines) is consistent with how other commands read files (`add.go:223`, `note_v1.go:632`, `query.go:210` all do bare `os.ReadFile`).
- **Error handling**: always `return fmt.Errorf("<verb>: ...: %w", err)`, never `os.Exit` — enforced by `docs/REVIEW_PATTERNS.md` (CLI-Specific Patterns, "os.Exit() in Library Code") and `internal/cli/AGENTS.md`. Cobra prints to stderr and `cmd/rk/main.go:10-12` maps any non-nil error to `os.Exit(1)` uniformly — the `ExitCode{Success,GeneralErr,UsageErr,NotFound}` constants (root.go:17-22) are declared but **unused by any command today** ([OPEN] — don't invent new exit-code plumbing for this ticket, no precedent to match).
- **DB/service handle**: no global services survive in `internal/cli` today (the anti-pattern of package-level `journalService`/`checklistService` + `initServiceE()` was deliberately deleted in the fnqs.1 purge — see §2). The one still-active DB-backed command, `rk index`, opens its store per-invocation inside `RunE` and defers `Close()` (index.go:32-36: `ix, err := index.Open(cfg); defer ix.Close()`). **Recommend the same shape for checklist**: a small unexported helper (e.g. `openChecklistService() (*checklist.Service, *storage.Database, error)`) called at the top of each `RunE`, `defer db.Close()`, using `config.DatabasePath()` — not a revived global/`initServiceE()`.
- **Test seam**: `t.Setenv("RECKON_DATA_DIR", t.TempDir())` is the standard safety net wherever a test's command path reaches `config.DatabasePath()`/`storage.NewDatabase` (used in `migrate_legacy_test.go:103,200,224,241`, `today_test.go:9-16`). New checklist tests should do the same rather than reintroduce the old `setupChecklistCLITestService` pattern of directly poking a package-global.

## 4. Deleted v0 verb layer (recovered) — reference, not template

Deleted in `83ca4ef` ("Retire the v0 DB-primary verb surface (reckon-fnqs.1)", #158). Recovered via `git show 83ca4ef^:internal/cli/checklist.go` and `...checklist_run_helper.go`.

**v0 surface** (`GetChecklistCommand()`, aliased `cl`): `checklist template {list,add,show,delete}`, `checklist template item {add,remove}`, `checklist run <template>` (inline Bubble Tea TUI), `checklist start`, `checklist check <position> --template <name>`, `checklist status [template]` (no-arg → lists all active runs via `ListRuns(false)`), `checklist reset <template>`, `checklist abandon <template>`, `checklist history` (→ `ListRuns(true)`).

Useful as-is: error-message wording (`"failed to %s: %w"`), `printRunStatus(run)` rendering (`%-20s [n/m]` + `[ ]`/`[x]` lines + "✓ Complete!"), and `resolveChecklistRun(svc, nameOrID) (run, resumed, err)` (checklist_run_helper.go, start-or-resume: `GetActiveRun` else `StartRun`) — directly reusable for the ticket's "start (or resume) a run" requirement.

**Must diverge from v0, per this ticket's scope**, in three ways:
1. **Flatter verb surface.** Ticket wants `rk checklist {create,list,start,check,status,reset,abandon}` directly under `checklist`, not v0's nested `template`/`template item`/`run`/`history` tree. `create <name> --item ... [--items-file ...]` replaces `template add <name> [item...]`.
2. **`--json` via `internal/output`,** not v0's bare `fmt.Printf`/`os.Stdout` (§3) — v0 predates/ignored that package.
3. **`check <template> <position>` is positional**, not v0's `check <position> --template <name>`.

The deleted inline TUI (`checklistRunModel`, Bubble Tea, checklist_run_helper.go) is reckon-yk1i/#144's scope — **out of scope here**, it's the follow-up ticket reckon-fnqs.12 ("interactive mini-TUI on the Prompt/Wizard layer"), which this ticket blocks. Do not resurrect `checklistRunModel` or a `run` verb.

## 5. `ListRuns` coverage gap [OPEN]

Ticket's "Done when" requires every one of `{CreateTemplate, StartRun, CheckItem, GetActiveRun, ResetRun, AbandonRun, ListRuns, ListTemplates}` reachable via a verb, but the enumerated verb list (`create/list/start/check/status/reset/abandon`) maps only to:

| Verb | Method(s) |
|---|---|
| create | CreateTemplate |
| list | ListTemplates |
| start | StartRun (+ GetActiveRun for resume-detection) |
| check | GetActiveRun + CheckItem |
| status | GetActiveRun (single-template case) |
| reset | ResetRun |
| abandon | AbandonRun |

`ListRuns` has **no assigned verb**. v0 covered it two ways: `status` with no args (`ListRuns(false)`, all active runs) and a separate `history` verb (`ListRuns(true)`). Implementer must pick one (e.g., `rk checklist status` with no template arg → active runs; add `--all`/`history` for completed+abandoned) or the Done-when bullet is unsatisfied.

## 6. Docs / pitfalls

- `internal/cli/AGENTS.md` is **stale**: describes pre-purge files (`task.go`, `notes.go`, `log.go`, `schedule.go`, `win.go`) that no longer exist, and the "Package-level globals" anti-pattern section describes exactly the `initServiceE()`/`checklistService` pattern deleted in fnqs.1. Don't follow its "Service Initialization" example; follow §3 above instead. [OPEN] whether to fix this doc is in scope — ticket doesn't ask for it.
- `internal/storage/AGENTS.md` is also stale (v0-era: claims "SQLite is a rebuildable index" from markdown — true for the now-defunct v0 tables, false for checklist, which has no text source of truth at all).
- `docs/REVIEW_PATTERNS.md` CLI-relevant pitfalls to actively avoid: unwrapped errors (`return err` alone), ignoring `--quiet`, `os.Exit()` inside command code (all documented in §3 above with file:line precedent to copy instead).
- `AGENTS.md` (repo root) confirms "checklist-run TUI" was already shipped once (line 32, now deleted) and lists the DB-primary→text-truth migration as still-open future work (T9, reckon-s6oh) — this ticket is explicitly *not* that migration.

## 7. `rk index --rebuild` constraint — verified, and a wording caveat [OPEN]

Verified facts: `rk index` (`internal/cli/index.go:16-58`) opens only `index.Open(cfg)`, whose backing file is `cfg.CacheDir/<vault-id>/index.db` (`internal/index/index.go:57,79,84-85`) — a disjoint SQLite file from `config.DatabasePath()` (`~/.reckon/reckon.db`), and `internal/index` has **zero imports of `internal/storage`** (confirmed by grep). So under the natural wiring (checklist DB opened via `config.DatabasePath()`, §2/§3), `rk index` **cannot** and does not touch checklist data — this is the correct answer to task item 6's "confirm it does not touch this DB."

Note there is no literal `--rebuild` flag — `rk index` always performs a full `ix.Rebuild()` unconditionally (no flag gating it); the ticket's `rk index --rebuild` phrasing doesn't match the actual CLI surface (just `rk index`).

The ticket's Done-when phrasing ("`rk checklist status` after a fresh `rk index --rebuild` visibly reports the run as gone") reads as a literal deletion-on-rebuild claim, which the verified mechanism contradicts. Resolve this as a **documentation** requirement, not a code requirement: surface in `--help`/command output that checklist data lives only in `~/.reckon/reckon.db`, is not part of the git-synced vault, has no text source of truth, is never rebuilt/derived by `rk index`, and has no backup/portability guarantee — i.e., it's orthogonal to (not managed by) the index/vault rebuild story, which is itself enough to explain why a fresh environment/new machine "loses" it. Do **not** add code that deletes checklist data as part of index rebuild — that would be actively harmful and contradicts the ticket's own "reuse Service/Repository as-is" scope.

## 8. Files to create / modify

| File | Action | Notes |
|---|---|---|
| `internal/cli/checklist.go` | **create** | `checklistCmd` + 7 subcommands (create/list/start/check/status/reset/abandon [+ history or --all]), `output`-package Result types w/ `Pretty()`, `openChecklistService()` helper, `resolveChecklistRun` (adapted from recovered helper, §4) |
| `internal/cli/checklist_test.go` | **create** | Use `t.Setenv("RECKON_DATA_DIR", t.TempDir())` (§3), not the old package-global-poking harness |
| `internal/cli/root.go` | **modify** | Add `checklistCmd` to the `RootCmd.AddCommand(...)` list (root.go:101-109) |
| `internal/checklist/*` | none | Service/Repository/model already complete and tested; no changes needed |
| `go.mod` | none | cobra, xid, modernc.org/sqlite, testify already present; no bubbletea/lipgloss needed (TUI is fnqs.12's scope) |
| `internal/cli/AGENTS.md`, `internal/storage/AGENTS.md` | none required | stale but out of ticket scope (§6); optional cleanup |
