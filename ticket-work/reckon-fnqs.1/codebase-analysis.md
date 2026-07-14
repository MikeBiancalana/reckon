# reckon-fnqs.1 — Deletion inventory: retire v0 DB-primary verb surface

All findings verified by grep/rg cross-reference sweep (every identifier defined in a
DELETE file was checked for references from every KEEP/EDIT file — zero hits found
beyond what's noted below). Three points below **correct or sharpen the ticket's own
stated premises** — read those before executing.

## internal/cli/ — file classification

| File | Verdict | Why |
|---|---|---|
| root.go | **EDIT** | strip v0 wiring; see "root.go edit" below |
| note.go | **EDIT** | drop `noteNewCmd`/`noteListCmd` + their `init()`; keep `noteCmd` var + `GetNoteCommand()` |
| root_help_test.go | **EDIT** | drop `"log"` from the expected-verbs list (line 35) — `rk log` is being deleted |
| task.go, notes.go, note_test.go, note_show_test.go, notes_edit_test.go, notes_integration_test.go, notes_test.go, note_picker_helper.go, note_picker_helper_test.go, format.go, log.go, log_test.go, checklist.go, checklist_run_helper.go, checklist_run_helper_test.go, checklist_test.go, schedule.go, week.go, win.go, review.go, rebuild.go, migrate.go, stubs.go, summary.go, task_note_helper.go, task_note_helper_test.go, task_picker_helper.go, task_picker_helper_test.go | **DELETE** | see per-file rationale below |
| add.go, adopt.go, index.go, import.go, query.go, today.go, todo.go, note_v1.go, slug.go, recur.go, dispatch.go, and all their `_test.go` | **KEEP unchanged** | v1, zero references to any DELETE-file symbol (verified) |
| add_test.go, adopt_test.go, import_test.go, index_test.go, query_test.go, query_extra_test.go, query_fts_test.go, query_validate_test.go, today_test.go, todo_test.go, todo_recur_test.go, recur_test.go, dispatch_test.go, note_v1_test.go, gitattributes_test.go | **KEEP unchanged** | see gitattributes_test.go note below |
| AGENTS.md | **STALE, not code** | documents v0 verb inconsistency as current state; flag for the implementer, not blocking |
| `root.go~` | **does not exist** | ticket names this as a trap; verified absent (`find . -name '*.go~'` → empty). No action. |

### DELETE rationale (grouped)

- **checklist.go / checklist_run_helper.go / checklist_test.go / checklist_run_helper_test.go** — `rk checklist`, uses `checklistService` (root.go global). No other caller of `resolveChecklistRun`/`runChecklistTUI`/`checklistRunModel` exists outside checklist.go.
- **log.go / log_test.go** — `rk log`, uses `journalService`.
- **notes.go (+ its 4 test files)** — `rk notes` (plural), uses `notesService`. Defines `stripFrontmatter`, `createNoteWithContent`, `openEditorForContent`, `displayNote/Link/Backlink`, `createNoteFromForm`, `launchNotesCreateForm` — all dead once this file dies; no external caller. `note_show_test.go`/`notes_integration_test.go`/`notes_edit_test.go`/`notes_test.go` all exercise these notes.go internals directly (`notesService`, `stripFrontmatter`, `createNoteWithContent`) — none call anything in note_v1.go. `notes_edit_test.go` duplicates notes.go's shell-injection-guard logic inline rather than calling it, but its own comments say "simulating notes.go logic" / "mirrors notes.go" — it's a test of dead functionality, not incidentally-reusable logic; delete.
- **schedule.go / week.go / win.go / review.go / rebuild.go** — each is a self-contained v0 verb (`rk schedule`, `rk week`, `rk win`, `rk review`, `rk rebuild`) using `journalService`/`journalTaskService`. No test files exist for these five today (pre-existing gap, not created by this deletion). `review.go`'s RunE bodies are already no-op stubs ("not yet supported") — still registers with default `requiresDB=true`, so it still opens the DB on invocation; dies anyway per the ticket's explicit verb list.
- **task.go / task_note_helper.go(+test) / task_picker_helper.go(+test)** — `rk task`, uses `journalTaskService`/`checklistService`(via helpers). `task_picker_helper.go`'s exported `PickTask`/`PickOpenTask` already have **zero callers anywhere in the repo** (confirmed by full-tree grep) — dead before this ticket; task.go uses `components.TaskPicker` instead for schedule/deadline flows, and `task_note_helper.go`'s `PickOpenTaskAndEnterNote` for the note flow. Both files die with task.go regardless.
- **format.go** — every one of its 17 functions (`formatTasksJSON/TSV/CSV`, `formatScheduleItems*`, `formatWins*`, `formatJournals*`, `formatNotesJSON/CSV`, plus `parseFormat`/`csvQuote`/`csvEscapeField`) is called only from task.go/schedule.go/win.go/week.go/note.go's dying `noteListCmd`. Verified zero calls from any v1 file. Whole file dies (not an edit — no surviving caller for any function in it).
- **migrate.go** — explicitly named in the ticket's Scope section. `rk migrate run/status/rollback`, the 2 of the ticket's 3 named `config.DatabasePath()` call sites (lines 37, 90). Self-registers `migrateCmd` via its own `init()` (not in root.go's `AddCommand` list) — no root.go touch needed beyond what's already required.
- **stubs.go** — `rk tui`, the 3rd `requiresDB=true` sink; explicitly returns in a later TUI-rebuild ticket per the ticket description.
- **summary.go — FORCED, not optional.** Not in the ticket's explicit v0 verb list, but it self-registers `summaryCmd` via its own `init()` (root.go's `AddCommand` block never mentions it — same self-registration pattern as migrate.go) and both `showTodaySummary()`/`showWeekSummary()` call `journalService.GetByDate` directly. Once root.go's edit removes the `journalService` package var (nothing else needs it — verified), **summary.go fails to compile**, independent of whether it's "in scope." It has no test file. Must delete.

### KEEP files verified clean (the cross-reference sweep)

Extracted every top-level `func`/`var` identifier defined across all 21 DELETE-candidate
files (102 funcs incl. bubbletea `Init`/`Update`/`View` methods, 46 cobra command vars)
and grepped each by name against every KEEP file (add/adopt/index/import/query/today/todo/
note_v1/slug/recur/dispatch/root + their tests). **Zero matches.** In particular, the six
CLI-internal helpers note_v1.go depends on but doesn't define itself all resolve to KEEP
files, not DELETE ones:

| Helper note_v1.go calls | Defined in |
|---|---|
| `resolveAuthor`, `relTodoPath`, `containsString` | todo.go (KEEP) |
| `writeFileAtomic` | adopt.go (KEEP) |
| `loadProps`, `loadAliases` | query.go (KEEP) |

The same sweep was repeated for every `type`/`const` defined in the 21 DELETE files
(`OutputFormat`/`FormatJSON`/`FormatTSV`/`FormatCSV` in format.go; the local type alias
`type Task = journal.Task` in task.go; the bubbletea model structs/state enums in each
helper file) — zero references from any KEEP file. In particular `OutputFormat` and its
three consts are used only by `parseFormat` and its v0 callers; no KEEP file switches on
or declares a variable of that type. `cmd/rk/main.go` was also checked: it calls only
`cli.Execute()`, no direct reference to any `Get*Command` func or the four root.go
service globals being deleted.

`getEffectiveDate()` and the `--date` flag (`dateFlag`) looked like v0-only machinery but
**survive**: `add.go:160` calls `getEffectiveDate()` directly (confirmed by grep, with
extensive comments in add.go/add_test.go explicitly contrasting it against UTC-anchored
logic). `vaultFlag`, `quietFlag`, `jsonFlag`, `ndjsonFlag` are likewise used across
add/adopt/import/note_v1/query/index/today/todo — all survive in root.go's persistent
flag block untouched.

**gitattributes_test.go** — tests that the repo-root `.gitattributes` file scopes
`merge=union` to `log/*.md` only. This is the vault-native `log/` directory convention
(a v1 concept, unrelated to `internal/cli/log.go`'s v0 command), and asserts file content
only — no dependency on any CLI command. KEEP unchanged.

## root.go edit — exact scope

**Imports to drop** (only used by the code below): `checklist`, `journal`,
`notessvc "internal/service"`, `storage`, `config` (only consumer was `initServiceE`).

**Package vars to drop**: `journalService`, `journalTaskService`, `notesService`,
`checklistService` — confirmed zero references anywhere in the surviving codebase once
initServiceE and the v0 files are gone.

**Function to drop whole**: `initServiceE()` (lines 150–177) — the last non-test caller
of `config.DatabasePath()` besides migrate.go's two (also dying).

**`PersistentPreRunE` to simplify** (lines 83–105): drop the "walk cmd and its ancestors
for `requiresDB=false`" loop and the `initServiceE()` call. Keep the json/ndjson
mutual-exclusivity check and the `initLoggerE()` call. After this edit every remaining
registered command is `requiresDB=false` already (add/adopt/index/import/query/todo×3/
today×3/note_v1×4) or has no annotation at all and no longer matters, since nothing reads
the annotation anymore.

**`AddCommand` block to trim**: remove `GetLogCommand()`, `GetNotesCommand()`, `weekCmd`,
`rebuildCmd`, `GetReviewCommand()`, `GetScheduleCommand()`, `GetTaskCommand()`,
`GetWinCommand()`, `GetChecklistCommand()`, `tuiCmd`. Keep `GetNoteCommand()`, `todayCmd`,
`addCmd`, `todoCmd`, `queryCmd`, `indexCmd`, `adoptCmd`, `importCmd`.

Note: root.go's own source comments ("// v0 subcommands (preserved verbatim)" /
"// v1 subcommands") **mis-group two of these** — `todayCmd` sits under the "v0" banner
despite being a true v1 command (no DB dependency, `requiresDB=false` on all its
children), and `tuiCmd` sits under "v1" despite being the DB-writing TUI that dies in
this ticket. Go by DB-usage/reachability, not the banner text, when editing.

**[RECOMMENDED, not required by "done when"]**: strip the now-inert
`Annotations: map[string]string{"requiresDB": "false"}` from add.go, adopt.go, index.go,
import.go, note_v1.go (×4), query.go, todo.go (×3), today.go (×3) — the annotation does
nothing once PersistentPreRunE stops reading it, and REVIEW_PATTERNS.md's own "Dead Code
After Refactoring" entry (line 905) calls this exact shape out ("switch cases / fields
that no longer have callers... compiler won't catch these"). Cheap, low-risk, but
optional — leaving it doesn't break "no code path opens the legacy operational DB."

## Whole-package classification

| Package | Verdict | Reason |
|---|---|---|
| internal/journal, internal/service, internal/checklist, internal/storage | **KEEP, untouched** | ticket scope boundary — textmigrate depends on them; moving is reckon-fnqs.2 |
| internal/db | **DELETE** — but **not for the reason the ticket hints** | ticket says "imported only by internal/tui/model.go" — **false**. Verified: zero importers anywhere in the tree, including tui/model.go. It's a pre-existing orphaned duplicate of internal/service's NotesRepository (its own `NoteRepository` type, unrelated to tui). Delete for being dead weight, independent of the tui deletion. |
| internal/sync | **DELETE** | sole importer is internal/tui/model.go (verified), which dies |
| internal/migrate | **DELETE** | sole importer is internal/cli/migrate.go (verified), which dies |
| internal/tui (root package: model.go, handlers.go, commands.go, keyboard.go, layout.go, task_sort.go + tests) | **DELETE** | sole external importer is internal/cli/stubs.go (verified — its own `layout_example_test.go` doesn't count), which dies |
| internal/tui/components | **KEEP, untouched** | see trap correction below |
| internal/parser | **KEEP** | imported by internal/service/notes_service.go (a surviving legacy-read lib), independent of any CLI file |
| internal/models | **KEEP** | broadly used (service, db, textmigrate, node, cli v1) |
| internal/perf | **KEEP** | imported by internal/journal/{task_service,task_repository,service}.go, which survives |
| internal/spike/roundtrip | **out of scope, leave alone** | zero real importers anywhere — every hit for "internal/spike" in node.go/node_test.go/textmigrate's test is a *comment*, not an import. Already-dead pre-existing code, unrelated to the v0 DB surface. Don't touch in this ticket; flag as a candidate for a separate cleanup ticket if wanted. |

## Trap correction: internal/tui/components example tests (ticket point 6)

**The ticket's stated trap does not hold in the current tree.** Verified by reading all
four files in full and grepping the whole `internal/tui/components/` directory for any
import of the parent `"github.com/MikeBiancalana/reckon/internal/tui"` package:
zero matches.

- `date_picker_example_test.go`, `form_example_test.go`, `text_editor_example_test.go`:
  `package components_test`, importing only `internal/tui/components` (itself — standard
  Go external-test-package idiom).
- `task_picker_example_test.go`: same, plus `internal/journal` (survives).

None import the dying parent `internal/tui` package. **components/ needs zero changes to
keep compiling** — no rewrite, no import surgery. Confirm this directly (`rg -n
'reckon/internal/tui"' internal/tui/components/`) rather than trusting the ticket text on
this point.

## Test files: DELETE vs KEEP

### DELETE (test of a deleted surface)
| File | Tests |
|---|---|
| checklist_test.go | 8 `Test*` funcs — `rk checklist` |
| checklist_run_helper_test.go | 16 funcs |
| log_test.go | 10 funcs — `rk log` |
| note_test.go | `TestNoteCommand_DeprecationNotice`, `TestNoteNewCommand_*` — asserts on `noteNewCmd` (dying); `noteCmd` assertions alone would still compile but the file as a whole references the dying var |
| note_show_test.go | `stripFrontmatter`, `notesService`-based show tests — tests notes.go internals, not note_v1.go's `runNoteShowE` |
| notes_edit_test.go | duplicates notes.go's editor-validation logic; no functional target once notes.go dies |
| notes_integration_test.go | `notesService`/`createNoteWithContent` |
| notes_test.go | `notesService`-based create/slug/path-traversal tests, all v0 |
| note_picker_helper_test.go | 3 funcs — `PickNote`, only caller was notes.go |
| task_note_helper_test.go | 13 funcs |
| task_picker_helper_test.go | 8 funcs — tests already-uncalled `PickTask`/`PickOpenTask` |

### KEEP unchanged
add_test.go, adopt_test.go, import_test.go, index_test.go, query_test.go,
query_extra_test.go, query_fts_test.go, query_validate_test.go, today_test.go,
todo_test.go, todo_recur_test.go, recur_test.go, dispatch_test.go, note_v1_test.go,
gitattributes_test.go — verified zero references to any DELETE-file symbol.

### EDIT
root_help_test.go — line 35's expected-verbs list contains `"log"`; must drop it once
`rk log` is deleted (the other six verbs — add, todo, note, query, today, index — all
survive).

## `deadcode` baseline and how to read it

Ran `/home/chadd/go/bin/deadcode ./... 2>&1 | head -50` on the **current, undeleted**
tree per the ticket's instruction. As expected it reports nothing useful about the v0
surface (root.go still registers everything, so it's all "live" from `main`). Not
reproduced here — not informative pre-deletion.

**Caveat for whoever runs `deadcode ./...` after the deletion lands**: internal/journal,
internal/checklist, internal/service, internal/storage are explicitly kept *whole* per
scope (their file-level dead weight is reckon-fnqs.2's job, not this ticket's). After
this deletion, `deadcode` will very likely flag as unreachable-from-main:
- `journal.Service` and all its methods, `journal.Repository`/`NewRepository`,
  `journal.TaskRepository`/`NewTaskRepository` — textmigrate only ever calls
  `journal.NewTaskService(nil, nil)` (yes, nil repo/store — `TaskService.GetAllTasks()`
  reads task files directly off disk via `config.TasksDir()`, never touching repo/store)
  plus the pure parsing/model symbols (`ParseJournal`, `Journal`, `LogEntry`,
  `Intention`, `Win`, `ScheduleItem`, `Task`, `TaskDone`).
- `checklist.Service`/`NewService` — textmigrate only uses `checklist.NewRepository` +
  `Template`/`Run` model types, never the service layer.
- `service.NotesService`/`NewNotesService` — textmigrate only uses
  `NewNotesRepository`.
- `storage.FileStore`/`NewFileStore` — textmigrate only uses `storage.NewDatabase`.

**These are false positives for this ticket.** Do not delete them; the ticket's scope
boundary explicitly forbids touching these four packages here. Only treat a `deadcode`
finding as actionable if it points into `internal/cli` (should be empty post-deletion) or
into `internal/tui`/`internal/db`/`internal/sync`/`internal/migrate` (should not exist
post-deletion, so a finding there would mean the deletion was incomplete).

## Pitfall from docs/REVIEW_PATTERNS.md

"Dead Code After Refactoring" (line 905, ticket reckon-pjjr): after removing a feature,
orphaned struct fields / unreachable switch cases / now-uncalled helpers routinely survive
because the compiler doesn't flag them. Directly applicable here to the
`requiresDB` annotations left on v1 commands (see root.go section above) and to
`internal/cli/AGENTS.md`, whose "Key files" list and "Current Verb Inconsistency" section
describe the v0 surface as current state — stale after this ticket, not code, but will
mislead the next reader if left as-is.
