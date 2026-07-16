# Acceptance Criteria — reckon-fnqs.1 (Retire the v0 DB-primary verb surface)

This is a deletion ticket: no new behavior, no failing tests to write first. Acceptance
criteria are assertions about **absence** — mechanically checkable via `go build`, `go vet`,
`grep`, and `RootCmd.Commands()`. Grounded in `bd show reckon-fnqs.1` plus direct reading of
`internal/cli/root.go`, every v0 command file, `internal/tui/`, `docs/TESTING.md`, and a build
of the current binary (`rk --help`, `rk note --help`) for ground truth.

## 0. Ground truth (verified, not inferred)

**Currently registered top-level commands** (`rk --help`, built from this worktree today): add,
adopt, checklist, completion*, help*, import, index, log, migrate, note, notes, query, rebuild,
review, schedule, summary, task, today, todo, tui, week, win. (*cobra built-ins, always present.)

**Surviving set (8 + 2 built-ins)** — matches the ticket's own v1 enumeration exactly:
add, adopt, import, index, note, query, today, todo.

**Dying set — 12 top-level verbs** (`note` itself survives; only 2 of its children die,
tracked separately) — the ticket's description names 10: log, notes, week, rebuild, review,
schedule, task, win, checklist, tui — plus `note`'s children `new`/`list`. **Two more top-level
verbs die but are absent from the ticket's own enumeration** — found by auditing every
`RootCmd.AddCommand` call site, not just `root.go`'s explicit block:
- `migrate` — self-registers via its own `init()` at `internal/cli/migrate.go:163`, not through
  `root.go`'s list. It *is* named in the ticket's Scope bullet ("...internal/cli/migrate.go"),
  just not in the verb-name sentence. Uses `config.DatabasePath()` directly (2 of the ticket's
  "3 non-test call sites").
- `summary` — self-registers via its own `init()` at `internal/cli/summary.go:117`. **Not named
  anywhere in the ticket.** RunE calls `journalService.GetByDate(...)` (`summary.go:36`),
  identical DB dependency to the named v0 set. No `requiresDB` annotation, so it silently opens
  the legacy DB today exactly like `log`/`task`/`week`/etc. Must die with the rest — flagged here
  because AC-1's exact-set check is the only thing that would catch a missed deletion of it.

**Two corrections to premises implied by the ticket/assignment:**
- `internal/tui/components/` has **zero** import of the parent `internal/tui` package — verified
  with `grep -rn 'MikeBiancalana/reckon/internal/tui"' internal/tui/components/*.go` (no hits).
  Its `*_example_test.go` files are `package components_test` importing only
  `internal/tui/components` itself. The assignment's premise that components/ tests import the
  dying parent is false; components/ already compiles independently and needs no changes.
- The `--date` persistent flag (`root.go:108`) and `getEffectiveDate()` (`root.go:180-188`) are
  **not** v0-only. `internal/cli/add.go:159-162` (`effectiveLogDate`) calls `getEffectiveDate()`
  on the explicit-flag path, and its own comment (`add.go:154-157`) documents this is
  intentional. Both must survive untouched.

## 1. Explicit acceptance criteria

- **AC-1 (exact top-level verb set).** `RootCmd.Commands()` names contain all 8 of
  `{add, adopt, import, index, note, query, today, todo}` **and none** of the 12 dying verbs
  `{log, notes, week, rebuild, review, schedule, task, win, checklist, tui, migrate, summary}`.
  **Do not** assert an exact-set snapshot including cobra's `help`/`completion`: verified
  empirically that a cold `RootCmd.Commands()` (before any `Execute()` call) returns only the 8
  explicitly-registered verbs — `help`/`completion` are added lazily inside `Execute()`
  (`InitDefaultHelpCmd`/`InitDefaultCompletionCmd`). Since `RootCmd` is a shared package global
  across the test binary, whether they're present depends on whether an earlier test in the file
  already called `Execute()` — an exact-set check including them is order-dependent and flaky.
  The two-directional containment check above is immune to that and is the real point: it makes
  `migrate`/`summary` absence explicit, not just implicit in a snapshot an implementer might
  "fix" by deleting entries from.
- **AC-2 (`note` parent keeps only v1 children).** `rk note --help` lists exactly
  `{create, index, rename, show}`. `new` and `list` are gone. `noteCmd` (the parent, `note.go:21`)
  and `GetNoteCommand()` (`note.go:220`) still exist — `note_v1.go:139`'s `init()` calls
  `noteCmd.AddCommand(noteCreateCmd, ...)` and depends on `noteCmd` being declared.
- **AC-3 (no non-test `config.DatabasePath()` caller).** `grep -rn 'DatabasePath()' --include=*.go .`
  outside `_test.go` files and `tests/` returns zero results. Before: 3 sites
  (`root.go:153`, `migrate.go:37`, `migrate.go:90`). After: `internal/config/config.go:89`'s
  `DatabasePath()` function itself is **not** deleted (out of scope; still called by
  `tests/integration_test.go`, a surviving library-level test — see EC-6) but has no remaining
  caller in `internal/cli` or any other non-test package.
- **AC-4 (`requiresDB` machinery gone).** `grep -rn 'requiresDB' --include=*.go .` returns zero
  results: no `Annotations` map key, no ancestor-walk in `PersistentPreRunE`, no `initServiceE`
  function or call site.
- **AC-5 (`initServiceE` and its 4 service globals gone).** `initServiceE` (`root.go:150-177`)
  is deleted. Package vars `journalService`, `journalTaskService`, `notesService`,
  `checklistService` (`root.go:30-33`) are deleted — they have zero remaining readers once every
  v0 file that used them is gone (verified: every non-test reader is one of the files in §2's
  kill list).
- **AC-6 (`PersistentPreRunE` survives, trimmed not gutted).** The json/ndjson mutual-exclusion
  check and the `initLoggerE()` call (`root.go:84-91`) remain; only the `requiresDB` ancestor
  walk + conditional `initServiceE()` call (`root.go:93-104`) is removed. `buildLoggerConfig`,
  `initLoggerE`, `getEffectiveDate` all remain unchanged.
- **AC-7 (`go build ./...` and `go vet ./...` clean).** Both exit 0 from repo root after deletion
  — this is the mechanical proof that no surviving file references a deleted identifier.
- **AC-8 (suite green, v0 tests deleted not skipped).** `go test ./...` exits 0. This covers the
  default (untagged) suite only — `tests/integration_test.go` (`//go:build integration`) and
  `tests/acceptance/acceptance_test.go` (`//go:build acceptance`) are excluded from it by their
  build tags; see AC-6's edit to `PersistentPreRunE` and T12 for why the acceptance-tagged suite
  is also required green. `grep -rn 't.Skip(' internal/cli/*.go` returns exactly the same 2
  pre-existing hits as today (`add_test.go:662`, `gitattributes_test.go:73` — both unrelated to
  v0, environment/wall-clock skips) — i.e. the count does not increase. Every test file listed in
  §2 as "delete" is gone from the tree (`git status` / `ls` shows them absent), not
  present-with-skipped-tests.
- **AC-8b (acceptance-tagged suite green).** `go test -tags acceptance ./tests/acceptance/`
  exits 0 — see T12. This is the strongest available regression guard on the
  `PersistentPreRunE` edit (AC-6), since it drives the real compiled binary through the surviving
  v1 verbs and the default suite in AC-8 never compiles this file.
- **AC-9 (`internal/tui` non-components subtree gone, components/ untouched).**
  `internal/tui/{commands,handlers,keyboard,layout,model,task_sort}.go` and their `_test.go`
  siblings plus `AGENTS.md` are deleted. `internal/tui/components/**` is unchanged (file count
  and content identical to before this ticket).
- **AC-10 (`rk import` still works end-to-end).** `import_test.go` passes unmodified;
  `internal/cli/import.go` (requiresDB=false, imports only `config`/`output`/`textmigrate`) is
  untouched. This is the fnqs.2 dependency the ticket explicitly protects.

## 2. File-level kill list (verified via import-graph audit, not just the ticket's prose)

**Delete whole file** (non-test):
`log.go`, `notes.go`, `note_picker_helper.go`, `week.go`, `rebuild.go`, `review.go`,
`schedule.go`, `task.go`, `task_note_helper.go`, `task_picker_helper.go`, `win.go`,
`checklist.go`, `checklist_run_helper.go`, `stubs.go`, `migrate.go`, `summary.go`, `format.go`.

`format.go` is not named in the ticket but every one of its functions (`parseFormat`,
`formatTasksJSON/TSV/CSV`, `formatScheduleItems*`, `formatWins*`, `formatJournals*`,
`formatNotesJSON/CSV`) has callers **only** in the dying files above (`note.go`'s `noteListCmd`,
`schedule.go`, `task.go`, `week.go`, `win.go` — verified by grepping every call site). v1 code
uses the unrelated `internal/output` package instead (`output.ModeFromFlags`/`output.New(...)`).

**Delete whole file** (test): `log_test.go`, `note_test.go`, `notes_test.go`,
`notes_edit_test.go`, `notes_integration_test.go`, `note_show_test.go`,
`note_picker_helper_test.go`, `task_note_helper_test.go`, `task_picker_helper_test.go`,
`checklist_test.go`, `checklist_run_helper_test.go`. (11 files. No test file exists today for
`week`/`rebuild`/`review`/`schedule`/`task`/`win`/`migrate`/`summary`/`tui` themselves — nothing
to delete there.)

**Edit, not delete:**
- `root.go` — imports (`checklist`, `journal`, `service`, `storage`, `config`), package var
  block, `PersistentPreRunE` body, `initServiceE`, `RootCmd.AddCommand` block. **Trap:** the
  existing `// v0 subcommands` / `// v1 subcommands` comments (`root.go:116,129`) are stale and
  actively misleading — `todayCmd` sits in the "v0" block but survives; `tuiCmd` sits in the "v1"
  block but dies. Do not use those comments as a deletion guide; use the verb sets in §0.
- `note.go` — delete `noteNewCmd`, `noteListCmd`, and their `init()` registrations
  (`note.go:210-217`); keep `noteCmd` var and `GetNoteCommand()`. Update `noteCmd.Long`
  (`note.go:24`, currently `"...create, list, and delete notes."` — stale: `list` is gone,
  `delete` never existed as a subcommand) to describe the surviving `create/show/rename/index`.
  Prune now-unused imports (`models`, `sort`, `text/tabwriter`, `os`, `golang.org/x/term`) —
  check each against what remains.

**Whole subtree delete:** `internal/tui/` minus `internal/tui/components/` (§AC-9).

**[INFERRED, non-blocking]** `internal/migrate/` (7 files: `backup.go`, `database.go`,
`log_files.go`, `migrate.go`, `migrate_test.go`, `task_files.go`, `validate.go`) becomes fully
orphaned once `internal/cli/migrate.go` is deleted — it was the package's only importer. Same for
`internal/sync/watcher.go`, orphaned once `internal/tui/model.go` (its only importer) is deleted.
Neither is named in the ticket, and no AC above gates on their removal — they compile fine as
dead code. Delete them for cleanliness if convenient; leaving them does not fail this ticket.

## 3. Implicit requirements

- **`note` parent must not lose v1 children.** Covered by AC-2. The risk is deleting the whole
  `note.go` file by pattern-matching "note.go = v0" — it hosts the shared `noteCmd` var that
  `note_v1.go` depends on.
- **Helper-function provenance checked, not assumed.** `resolveAuthor`, `relTodoPath`,
  `containsString` — all three used by `note_v1.go` — are defined in `todo.go` (lines 247, 931,
  939), a v1 survivor. Verified directly; no v1-silently-depends-on-v0-helper case exists for
  these three. `slug.go` (`slugify`, `validateSlug`, `writeFileAtomic` via `adopt.go`) is
  similarly v1-shared and untouched by this ticket.
- **`recur.go` / `doneRecurringTodo` independence.** `recur.go`'s pure repeater math is consumed
  only by `todo.go:doneRecurringTodo` (v1). No coupling to `task.go`'s dying recurrence code, if
  any existed.
- **No survivor test hard-codes a dying verb name as a CLI-dispatch fixture.** Checked
  `dispatch_test.go` (uses synthetic `"foo"`/`"nosuchcommand"`, not real verbs) and grepped every
  surviving test file for the dying verb strings — all hits are false positives (`"log"` and
  `"notes"` as **vault directory names** in `add_test.go`/`today_test.go`/`todo_recur_test.go`/
  `adopt_test.go`/`note_v1_test.go`; `"task"` as a **node type string** in `query_test.go`). None
  reference the CLI verbs. No survivor test needs editing for this reason.
- **`tests/integration_test.go` is out of this ticket's blast radius.** It calls
  `config.DatabasePath()` 6 times, but it's a `//go:build integration`-gated test (not run by
  default `go test ./...`) exercising `internal/journal`/`internal/storage` directly — the
  surviving legacy-read libraries — never through the `cli` package or any v0 command. AC-3's
  "non-test" carve-out already covers it; no change required here, and this ticket's scope
  boundary (journal/storage survive) makes its continued existence correct, not a leftover.

## 4. Edge cases

- **EC-1 (`--date` flag and `getEffectiveDate` survive).** Already covered in §0 — `add.go`
  depends on both. Do not remove them while trimming `PersistentPreRunE`.
- **EC-2 (`buildLoggerConfig` loses one caller, keeps another).** `stubs.go:21` (`tuiCmd`, dying)
  and `root.go:142` (`initLoggerE`, surviving) are its only two call sites. The function itself
  is not dead code after `stubs.go` is deleted.
- **EC-3 (dispatch behavior for now-unregistered verbs is a non-issue, not a regression).**
  Once `rk log`/`rk task`/etc. are no longer cobra commands, `maybeDispatch` (`dispatch.go:103`)
  will look for an external `rk-log`/`rk-task` binary on `PATH` before falling through to
  cobra's "unknown command". This is existing, verb-agnostic dispatch logic (no hard-coded verb
  list) — expected behavior, not something to special-case or guard against.
- **EC-4 (vestigial `requiresDB` annotations on survivors).** `today`/`add`/`todo`/`query`/
  `index`/`adopt`/`import` and the 4 `note_v1.go` commands all carry
  `Annotations: map[string]string{"requiresDB": "false"}`. Once the ancestor-walk that reads
  this annotation is deleted (AC-4), these literals become inert data nobody reads. Stripping
  them is optional cleanup, not gated by any AC — leaving them is harmless.
- **EC-5 (stale `RootCmd.Long`).** `root.go:75`'s `Long` text ("...combining daily journaling,
  task management, and knowledge base") is generic enough to leave as-is; not gated by any AC,
  noted only so it isn't mistaken for a required edit.
- **EC-6 (`config.DatabasePath()` the function is not deleted).** Explicit per AC-3 — it has
  exactly one remaining caller after this ticket (`tests/integration_test.go`), which is
  in-scope-surviving per the ticket's own boundary.
- **EC-7 (`today_test.go`'s stale header comment).** `today_test.go:1-33` narrates a pre-T7
  "red state" where `todayCmd` still lacked `requiresDB=false` (it now has it, confirmed at
  `today.go:56`). The comment is now historically inaccurate but harmless — not part of this
  ticket's scope to fix, flagged only so it isn't mistaken for something this ticket broke.

## 5. Test scenarios (given/when/then)

`internal/cli/root_help_test.go` **must be updated** — it is the single highest-value test in
this ticket, and today it asserts presence only (`TestRootHelp_ListsSubcommands` checks that
`{add, log, todo, note, query, today, index}` appear as substrings, `"log"` included). Change it
to assert the exact set from AC-1.

| # | Given | When | Then |
|---|---|---|---|
| T1 | fresh `RootCmd` (no prior `Execute()` in the same test binary — see AC-1) | reading `RootCmd.Commands()` names | contains all 8 survivors; contains none of the 12 dying verbs (replaces `TestRootHelp_ListsSubcommands`'s substring loop; keep the existing `"Usage:"` and `--help` exit-0 checks) |
| T2 | fresh `RootCmd` | `rk note --help` | output lists exactly `create, index, rename, show`; does not contain `new` or `list` as command names |
| T3 | repo root | `grep -rn 'DatabasePath()' --include=*.go . \| grep -v _test.go \| grep -v tests/` | zero output |
| T4 | repo root | `grep -rn requiresDB --include=*.go .` | zero output |
| T5 | repo root | `go build ./...` | exit 0 |
| T6 | repo root | `go vet ./...` | exit 0 |
| T7 | repo root | `go test ./...` | exit 0 |
| T8 | repo root | `grep -rn 't.Skip(' internal/cli/*.go \| wc -l` | equals 2 (unchanged from baseline) |
| T9 | repo root | `ls internal/cli/{log,notes,week,rebuild,review,schedule,task,win,checklist,stubs,migrate,summary,format}.go 2>&1` | every path reports "No such file" |
| T10 | repo root | `ls internal/tui/components/` vs. this ticket's pre-change listing | identical file set (regression guard for the components/ independence finding in §0) |
| T11 | built `rk`, fresh vault | `rk import` against fixture legacy data (existing `import_test.go` scenarios) | unchanged pass/fail behavior — proves fnqs.2's dependency is intact |
| T12 | repo root | `go test -tags acceptance ./tests/acceptance/` | passes. This suite (`tests/acceptance/acceptance_test.go`, repo convention since #156) rebuilds the real `rk` binary and drives the surviving v1 verbs end-to-end through `RootCmd.Execute()` — the strongest available guard that trimming `PersistentPreRunE` (AC-6) didn't regress real-binary startup, since default `go test ./...` excludes both this suite and `tests/integration_test.go` (`//go:build acceptance` / `//go:build integration` respectively) |

`TestAddHelp` and `TestRootHelp_MissingVaultOK` in `root_help_test.go` need no logic change
(neither references a dying verb), but should be re-read once T1's rewrite lands, since they
share the file.

## 6. Explicitly out of scope

- Moving `internal/textmigrate` off `rk import` (reckon-fnqs.2 — this ticket blocks it, doesn't
  do it).
- Rebuilding the TUI (later ticket; `internal/tui/components/` is kept exactly so that rebuild
  has presentation components to reuse).
- The composable-prompt layer (reckon-fnqs.6).
- Backlog triage / `current_state.md` rewrite (reckon-fnqs.9).
- Deleting `internal/journal`, `internal/service`, `internal/checklist`, `internal/storage`,
  `internal/db`, or `internal/textmigrate` — all survive verbatim per the ticket's scope
  boundary; only their `internal/cli` **callers** in the v0 verb set die.
- Trimming write-only methods off `journal.Service`/`checklist.Service` now that their only
  remaining consumers are legacy-read (textmigrate) and library-level tests
  (`tests/integration_test.go`) — plausible future cleanup, not this ticket.
- `internal/migrate` and `internal/sync` orphan-package deletion — see §2's [INFERRED,
  non-blocking] note; convenient but not gated.
