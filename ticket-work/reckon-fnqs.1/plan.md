# Implementation Plan: reckon-fnqs.1 — Retire the v0 DB-primary verb surface

## Summary

Pure deletion. Remove the 12 v0 top-level verbs + `note`'s two v0 children, the four service globals and `initServiceE()` that opened the legacy operational DB, and the four packages that become orphaned (`internal/tui` root, `internal/db`, `internal/sync`, `internal/migrate`). `internal/journal`, `internal/service`, `internal/checklist`, `internal/storage`, `internal/tui/components` survive verbatim — `textmigrate` (→ `rk import`) reads them and moving it is reckon-fnqs.2.

**Order: leaf-first, not root-first.** Root-first (gut `root.go`'s globals to make the compiler enumerate dependents) floods the build with errors across 17 files you are about to delete anyway — noise, and it invites over-deletion into `note.go`/`summary.go` by error-chasing. Leaf-first localizes the error surface: once the dying `internal/cli` files are gone, the *only* compile errors are in `root.go` (undefined `GetLogCommand`, …) and `note.go`, which are exactly the two files that need editing. The forcing function is preserved and better aimed: removing `journalService` et al. after the leaves are gone makes the compiler name any reader you missed.

Unused package-level decls are legal Go, so intermediate states compile — that flexibility is what lets each step be independently green. There is no CI (`.github/workflows` does not exist) and no lint config, so green-at-each-step is hygiene, not a gate. **What the ACs check is final-state green.**

## Files to delete / modify

### internal/cli — delete whole (17 source + 11 test)
`log.go`, `notes.go`, `note_picker_helper.go`, `week.go`, `rebuild.go`, `review.go`, `schedule.go`, `task.go`, `task_note_helper.go`, `task_picker_helper.go`, `win.go`, `checklist.go`, `checklist_run_helper.go`, `stubs.go`, `migrate.go`, `summary.go`, `format.go`
`log_test.go`, `note_test.go`, `notes_test.go`, `notes_edit_test.go`, `notes_integration_test.go`, `note_show_test.go`, `note_picker_helper_test.go`, `task_note_helper_test.go`, `task_picker_helper_test.go`, `checklist_test.go`, `checklist_run_helper_test.go`

Re-verified independently: every reference to a symbol defined in these files (`parseFormat`, `formatNotes*`, `OutputFormat`, `PickNote`, `PickTask`, `stripFrontmatter`, `createNoteWithContent`, `journalService`, `journalTaskService`, `notesService`, `checklistService`) lives in another dying file or in `root.go`/`note.go` — the two EDIT files. Zero leakage into survivors. `migrate.go:163` and `summary.go:117` self-register outside `root.go`'s block; deleting the files is the only way to unregister them.

### internal/cli — edit
| File | Edit |
|---|---|
| `root.go` | drop imports `checklist`,`journal`,`service`,`storage`,`config`; drop the 4 service vars (30–33); delete `initServiceE()` (150–177); trim `PersistentPreRunE` to json/ndjson exclusivity + `initLoggerE()` (delete the `requiresDB` ancestor walk, 93–104); trim `AddCommand` to the 8 survivors. **Keep** `dateFlag`/`getEffectiveDate()` (`add.go:159-162` calls it), all other persistent flags, `buildLoggerConfig` (still called by `initLoggerE`), `Execute`/`maybeDispatch`. Ignore the stale `// v0` / `// v1` banner comments and delete them. |
| `note.go` | keep only `noteCmd` (21) and `GetNoteCommand()` (220). Delete `noteNewCmd`, `noteListCmd`, the three flag vars, and the whole `init()` (210–218). Rewrite `noteCmd.Long` (currently `"create, list, and delete notes"` — stale) to describe `create/show/rename/index`. Imports collapse to `cobra` alone (`fmt`,`os`,`sort`,`strings`,`text/tabwriter`,`models`,`golang.org/x/term` all die). |
| `add.go`, `adopt.go`, `import.go`, `index.go`, `note_v1.go` (×4), `query.go`, `todo.go` (×3), `today.go` (×3) | strip the now-inert `Annotations: map[string]string{"requiresDB": "false"}`. **Required, not optional** — see Decision 2. |
| `root_help_test.go` | rewrite `TestRootHelp_ListsSubcommands` — see Decision 3. |

### Whole packages — delete
`internal/tui/{commands,handlers,keyboard,layout,model,task_sort}.go` + their `_test.go` + `layout_example_test.go` + `internal/tui/AGENTS.md`. `internal/db/`, `internal/sync/`, `internal/migrate/`.
**`internal/tui/components/` untouched** — it imports only `internal/{time,journal,models,logger}` + itself, never the parent package. Zero changes needed.

### go.mod
`go mod tidy` after the cut. `fsnotify` (only `internal/sync`) and `golang.org/x/term` (only `note.go`/`task.go`, both dying) become unused direct requires. Build stays green without tidy; run it and re-verify `go build ./...`.

### Disagreements with the input artifacts (flagged, not silently resolved)
1. **acceptance-criteria.md §2 marks `internal/migrate`/`internal/sync` deletion "[INFERRED, non-blocking]"**; codebase-analysis.md and settled-constraint-5 say delete. **Siding with constraint-5: delete all three orphans** (`db`, `sync`, `migrate`).
2. **AC-4's literal grep is unsatisfiable alongside EC-7** — see Decision 2.
3. **codebase-analysis.md's root_help_test.go edit ("drop `"log"` from line 35") is insufficient** — it leaves a substring-matching test that cannot express absence. See Decision 3.

## Design decisions

### D1 — Do NOT cut into journal/service/checklist/storage service methods
Aggressive alternative: delete `journal.Service`, `checklist.Service`, `service.NotesService`, `storage.FileStore` now that only textmigrate consumes those packages. **Rejected.** (a) The ticket's scope boundary forbids it and fnqs.2 relocates textmigrate anyway, at which point the real reachability is known. (b) `tests/integration_test.go` (`//go:build integration`) exercises `journal`/`storage` directly and would break — a suite that is *supposed* to stay green here. (c) It multiplies the diff across package boundaries for zero AC benefit ("no non-test `config.DatabasePath()` caller" is satisfied without it).
**Instead:** run `/home/chadd/go/bin/deadcode ./...` *after* the cut and record findings in the ticket for fnqs.2. Expected false positives (do not act on them): `journal.Service`/`Repository`/`TaskRepository`, `checklist.Service`, `service.NotesService`, `storage.FileStore`. A finding pointing into `internal/cli` (should be empty) or into `internal/tui`/`db`/`sync`/`migrate` (should not exist) means the deletion is *incomplete* — that is the only actionable signal.

### D2 — Annotation stripping is REQUIRED; AC-4's grep must be scoped to non-test files
`requiresDB` appears in `add_test.go:31`, `today_test.go:8,9,17`, `note_v1_test.go:25` — **all comments, zero runtime reads** (verified: `grep -rn 'Annotations' internal/cli/*_test.go` returns only those two comment lines). So stripping the annotations from the 8 v1 source files is safe. But AC-4 as literally written (`grep -rn 'requiresDB' → 0`) cannot be satisfied without editing comments that EC-7 explicitly says to leave alone.
**Resolution: AC-4 is verified as `grep -rn 'requiresDB' --include='*.go' . | grep -v '_test.go'` → zero.** Since AC-4 gates, the stripping that codebase-analysis.md and EC-4 label "[RECOMMENDED]/optional" is **required**. Leaving inert annotations is also the exact shape `docs/REVIEW_PATTERNS.md:905` ("Dead Code After Refactoring") calls out.

### D3 — The help-surface test must iterate `RootCmd.Commands()`, never substring-match help text
Today's test substring-matches the rendered `--help` output. A naive port of that to absence assertions **is broken by construction** — dying verb names still occur as substrings of surviving output:

| Dying verb | Surviving text that contains it |
|---|---|
| `log` | `--log-file` / `--log-level` persistent flags (`root.go:110-111`) |
| `task` | `RootCmd.Long`: "…task management…" (`root.go:75`, EC-5 keeps it) |
| `notes` | `noteCmd.Short/Long` ("Manage standalone notes") |

`!strings.Contains(out, "log")` would false-fail forever.
**Assert over `RootCmd.Commands()` `.Name()` values, both directions:** all 8 survivors `{add, adopt, import, index, note, query, today, todo}` present; none of the 12 dying `{log, notes, week, rebuild, review, schedule, task, win, checklist, tui, migrate, summary}` present. Name-set iteration needs no `Execute()` call, so it is immune to cobra's lazy `help`/`completion` registration (AC-1's flakiness point — `RootCmd` is a package global, so an exact-set snapshot is order-dependent across tests in the same binary). Containment-both-ways is also what makes `migrate`/`summary` absence *explicit* rather than implicit in a snapshot someone can "fix" by editing the expected list. Keep the existing `"Usage:"` and `--help`-exits-0 assertions; `TestAddHelp` and `TestRootHelp_MissingVaultOK` need no logic change.

### D4 — Diff reviewability
The diff is ~5k lines and mostly file removals. Structure it so each commit is independently green and separately readable:
1. **Unregister** (behavior change, small, green): `root.go` `AddCommand` trim + `note.go` `init()` trim + delete `migrate.go`, `summary.go`. `rk help` is already correct here. The v0 command vars become unreferenced-but-legal decls.
2. **Delete leaves** (pure removal, green): the remaining 15 v0 source files + 11 test files.
3. **Delete the wiring** (green): `root.go` service globals, `initServiceE`, `PersistentPreRunE` walk, imports; `note.go` body + imports. *This is the compiler forcing-function step* — any missed reader from step 2 surfaces as an "undefined" error here.
4. **Delete orphaned packages** (green): `internal/tui` root, `internal/db`, `internal/sync`, `internal/migrate` + `go mod tidy`.
5. **Cleanup** (green): strip `requiresDB` annotations; rewrite `root_help_test.go`; doc updates.

Do not over-invest in the choreography — collapsing to fewer commits is acceptable; what is *not* acceptable is a final state that fails any check below.

## Test scenarios (from acceptance-criteria.md §5)

| # | Given | When | Then |
|---|---|---|---|
| T1 | `RootCmd` in the `cli` test binary | read `RootCmd.Commands()` names (no `Execute()` needed) | contains all 8 survivors; contains none of the 12 dying verbs (D3) |
| T2 | `RootCmd` | `rk note --help` | lists exactly `create, index, rename, show`; no `new`, no `list` **as command names** (assert over `noteCmd.Commands()` names, not help text, for the same substring reason as D3) |
| T3 | repo root | `grep -rn 'DatabasePath()' --include='*.go' . \| grep -v _test.go \| grep -v '^./tests/'` | zero. (Before: `root.go:153`, `migrate.go:37`, `migrate.go:90`. `config.DatabasePath()` the *function* is NOT deleted — `tests/integration_test.go` still calls it, legitimately.) |
| T4 | repo root | `grep -rn 'requiresDB' --include='*.go' . \| grep -v _test.go` | zero (D2) |
| T5/T6 | repo root | `go build ./...` ; `go vet ./...` | exit 0 — the mechanical proof no survivor references a deleted identifier |
| T7 | repo root | `go test ./...` | exit 0 |
| T8 | repo root | `grep -rn 't.Skip(' internal/cli/` | exactly 2 hits, unchanged (`add_test.go:662`, `gitattributes_test.go:73`) — proves deletion, not skipping |
| T9 | repo root | `ls internal/cli/{log,notes,week,rebuild,review,schedule,task,win,checklist,stubs,migrate,summary,format}.go` | all "No such file" |
| T10 | repo root | `ls internal/tui/components/` | identical file set to pre-change (regression guard on the components-independence finding) |
| T11 | built `rk` | `import_test.go` scenarios | unchanged pass — protects the fnqs.2 dependency |
| T12 | repo root | `go test -tags acceptance ./tests/acceptance/` | exit 0. **Required**: plain `go test ./...` never compiles this file (`//go:build acceptance`), and it is the only guard that drives the real binary through `PersistentPreRunE` after the trim. Verified its `"notes"` occurrence (`acceptance_test.go:365`) is a vault *directory*, not the dying verb — no edits needed. |
| T13 | repo root | `go test -tags integration ./tests/` | exit 0 — exercises the surviving `journal`/`storage` libs; cheap extra guard that D1's "don't touch them" held |
| T14 | post-deletion tree | `/home/chadd/go/bin/deadcode ./...` | findings recorded, not acted on (D1); any hit inside `internal/cli` or in a supposedly-deleted package = incomplete deletion |

## Known risks / ambiguities

- **v1 → v0 helper dependency: none.** `note_v1.go`'s six borrowed helpers all resolve to survivors: `resolveAuthor`/`relTodoPath`/`containsString` → `todo.go`; `writeFileAtomic` → `adopt.go`; `loadProps`/`loadAliases` → `query.go`. `today.go` never appears in a `journalService` grep — it is clean v1 despite sitting under the stale `// v0` banner in `root.go`.
- **`internal/sync` deletion breaks nothing.** Its sole importer is `internal/tui/model.go`, which dies. Its only external footprint is the `fsnotify` require in go.mod (→ `go mod tidy`).
- **`internal/models` / `internal/parser` survive.** `parser` is imported by `internal/service/notes_service.go`; `models` by service/textmigrate/node/v1 CLI. Both out of the blast radius. `internal/perf` likewise (imported by `internal/journal`).
- **`internal/spike/roundtrip` is pre-existing dead code** with zero real importers (every "internal/spike" hit elsewhere is a comment). Unrelated to the v0 DB surface — **do not touch**; it may show up in the post-deletion `deadcode` run.
- **`EC-3` dispatch behavior is expected, not a regression.** After deregistration, `rk task` makes `maybeDispatch` (`dispatch.go:103`) look for an external `rk-task` on `PATH` before cobra's "unknown command". Verb-agnostic logic, no hard-coded list; nothing to guard.
- **Doc staleness (recommended follow-up, not gated by any AC):** deleting `internal/tui/AGENTS.md` per AC-9 leaves a dangling reference at `docs/agents/planner.md:13`. Root `AGENTS.md` and `internal/cli/AGENTS.md` (its "Key files" list and "Current Verb Inconsistency" section) describe the v0 surface as current state; `README.md`/`QUICKSTART.md` likely document dying verbs. [OPEN] Either fix in step 5 or hand to reckon-fnqs.9 (backlog triage / `current_state.md` rewrite) — recommend a one-line note in the ticket either way.
- **[INFERRED] `go mod tidy` may touch `go.sum` broadly.** Keep it in its own commit so the review can skip it.
