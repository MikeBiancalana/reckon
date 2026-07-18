# Implementation Plan: reckon-fnqs.8 — Persistent multi-pane `rk tui` over index + verbs

## Summary

Rebuild `rk tui` as a porcelain: a persistent 4-pane bubbletea app (agenda, todos, log, notes) that reads exclusively through the SQLite index's public views and writes exclusively by calling the existing unexported CLI verb functions, reconciling after every mutation. Because the verbs (`addDurableTodo`, `dispatchTodayAct`/`act*`, `appendLogEntry`, and a new `createNote`) are unexported in `internal/cli`, all new orchestration code (model/layout/keyboard/panes/read-helpers) lives as new files in `package cli`. The four presentation widgets that panes actually consume are decoupled from `internal/journal`; the orphaned ones are deleted — both routes yield zero `internal/journal`/`internal/service` imports under `internal/tui` (test scenario 11). No file watcher (fsnotify is gone); freshness is pull-based via `ix.Reconcile()`.

## Files to create

| File (all `internal/cli/`) | Contents |
|---|---|
| `tui.go` | `tuiCmd` cobra command (mirrors pre-deletion `stubs.go` shape, codebase-analysis §6: `buildLoggerConfig(true)` → `index.Open(cfg)` → `tea.NewProgram(model, tea.WithAltScreen())`, `defer ix.Close()`), plus `newTUIModel(ix, cfg)` constructor. No `requiresDB` annotation. |
| `tui_model.go` | `tuiModel` struct (holds `*index.Index`, `cfg`, `vaultDir`, the 4 pane wrappers, focused-pane enum, mode flags, `width`/`height`); `Init()` batches the 4 initial load cmds; `Update()` flat `switch msg.(type)` dispatcher; `View()` (modal-state branch → layout renderer). Async result msg types: `agendaLoadedMsg`, `todosLoadedMsg`, `logLoadedMsg`, `notesListLoadedMsg`, `notesLinksLoadedMsg`, `mutationDoneMsg`, `errMsg`. Every load `tea.Cmd` snapshots captured vars (§8 async-closure pitfall). |
| `tui_layout.go` | `calcPaneDims(w, h) paneDims` — reimplemented fresh for 4 panes; clamps negative dims to 0 (edge case: resize). `WindowSizeMsg` handler propagates via each pane's `SetSize`. |
| `tui_keyboard.go` | Priority chain: sub-flow-input-active > text-entry-active > focused-pane-normal > global (Tab focus-cycle across the 4 fixed panes, quit, help). Hosts the agenda actuator sub-flow state machine (below). |
| `tui_panes.go` | 4 pane wrappers: **agenda** (hand-rolled list over `[]agendaItem`), **todos** (hand-rolled subject-only list over `[]todoListItem`), **log** (wraps decoupled `components.LogView`), **notes** (composite of `components.NotePicker` + `components.NotesPane`). |
| `tui_read.go` | New index read helpers: `loadLogEntries(db)`, `listNotes(db)`, `loadNotesPaneLinks(db, noteID)`. Reuses `buildAgenda`, `listDurableTodos`, `listEphemeralTodos`, `loadTodoProps`, `loadDependsOn` verbatim. |

Split is for size; merging `tui_layout`/`tui_keyboard` into `tui_model.go` is acceptable.

New test files: `internal/cli/tui_test.go` (model/layout/keyboard/verb-reuse/durability), `internal/tui/no_journal_import_test.go` (new `package tui`, scenario 11).

## Files to modify

| File | Change | Reason |
|---|---|---|
| `internal/cli/root.go:106` | Add `RootCmd.AddCommand(tuiCmd)` in `init()` | Register 9th command. |
| `internal/cli/note_v1.go:228` | Extract `createNote(notesDir, params) (noteCreateResult, error)` from `runNoteCreateE`; `RunE` calls it | Decision 2 needs a callable note-creation verb; avoids duplicating ~40 lines (slug/collision/dir-validation/`node.NewNode`/`writeFileAtomic`). |
| `internal/tui/components/task_list.go` | Replace `journal.Task` with local `type DateInfo struct { ScheduledDate, DeadlineDate *string }`; `FormatDateInfo`/`GetDateStyle` take `DateInfo` | Decouple (Decision 1); helpers only ever touch those two `*string` fields. |
| `internal/tui/components/task_list_test.go` | Update fixtures to `DateInfo` | Import-set member (scenario 11 greps `_test.go` too). |
| `internal/tui/components/log_view.go` | Replace `journal.LogEntry` with local `type LogEntryRow struct { ID string; Timestamp time.Time; Content string; EntryType string }`; **drop** the `Notes`/`LogNote` machinery (`findLogNoteText`, note rows in `buildLogItems`, `LogNoteAddMsg`/`DeleteMsg`, `n`/`d` keys, `SelectedLogNote`, `IsSelectedItemNote`, note-collapse) | Decouple; log entries are flat `log-entry` nodes with no nested-notes source or verb in v1. |
| `internal/tui/components/task_picker.go` (+`task_picker_test.go`, `task_picker_example_test.go`) | Replace `journal.Task` with the same local `DateInfo`-style row type as `task_list.go` (or a shared `TaskRow`) | **[CORRECTED — was "delete"]** fnqs.6's own scope ("Retarget the pickers at index rows instead of journal.Task") names this file explicitly. Deleting it here forces fnqs.6 to recreate it from scratch; decoupling now satisfies the same zero-import goal at no extra cost and leaves fnqs.6 net work unchanged. Not one of the 4 panes, but keep it wired-but-unused (e.g. reserved for a future "link todo" flow) rather than delete — a decoupled-but-currently-uncalled component is not the "dead code after refactoring" pitrap; a *deleted-then-recreated* one is pure waste. |
| `internal/tui/components/note_picker.go` | Add `SetHeight(h int)` (constructor currently does `list.New(items, delegate, 0, 0)` and only exposes `SetWidth` — height is never set, so the internal `bubbles/list.Model` renders at height 0 today). Add an `embedded bool` mode that suppresses `notePickerBoxStyle`'s own border/padding when true | **[NEW — advisor-flagged]** `NotePicker` as built is modal-popup-shaped (fixed self-bordered box, no height API), not persistent-pane-shaped. Mounting it as one of 4 on-screen panes needs real height control, and its own border would double up against the pane layout's own frame. Required before the notes-pane composition (Decision 2) can render correctly, not optional polish. |

## Files to delete (orphaned; removal satisfies scenario 11)

Confirmed zero external references: `wins_view.go`, `intention_list.go`, and `schedule_view.go`. (`task_picker.go` moved to the decouple table above — see correction.)

- `wins_view.go`, `intention_list.go`: not among the 4 panes, no caller, nothing else wants them. AC §2.1 explicitly blesses delete-or-decouple for these; delete is the lower-cost choice since nothing downstream (fnqs.6/fnqs.10) names them specifically.
- `schedule_view.go`: **[DECISION — deviates from Decision 1's literal "decouple 6", meets its goal]** it models only `time+content` rows with j/k; the agenda pane needs `State`/`ReadOnly`/`Priority`/`Pinned` + 7 actuator keys, so it hand-rolls its renderer instead. Decoupling schedule_view but leaving it unused would itself be the "dead code after refactoring" pitfall (§8). Deleting it yields the same zero-journal-import result. Owner may veto in favor of decouple-and-adapt.

## Design decisions

### Decouple vs delete, per component
Tie fate to consumption: `task_list.go` (agenda date column) and `log_view.go` (log pane base) are **decoupled**; the other four are **deleted**. All six end journal-free.

### Data loading (reads — index-only)
Each pane loads via a `tea.Cmd` closure capturing `*sql.DB`:
- **Agenda**: `buildAgenda(db, todoNow().Format("2006-01-02"))` verbatim → `[]agendaItem` (already carries `ReadOnly`, `Title`, `State`, `Priority`, dates). Per-row malformed-date rows are skipped with a warning inside `buildAgenda` (edge case handled upstream).
- **Todos**: `listDurableTodos(db, false, "")` + `listEphemeralTodos(db, false)` → subject-only display uses `todoListItem.Title` (AC#4).
- **Log**: new `loadLogEntries(db)` = `SELECT id, time, body, title FROM nodes WHERE type='log-entry' ORDER BY time DESC`, mapped to `components.LogEntryRow` (`Timestamp` parsed from `time`, `Content` from title/body). Log entries are distinct index nodes (`internal/node/logparser.go:118`), not raw day-file text (scenario 2).
- **Notes list**: new `listNotes(db)` = `SELECT id, ulid, title FROM nodes WHERE type='note'`, slug from first alias (`aliases` view) or `loc` stem → `[]*models.Note{Title, Slug}` for `NotePicker`.
- **Notes links**: new `loadNotesPaneLinks(db, noteID)` wraps `loadNoteForwardLinks`/`loadNoteBacklinks`, building `[]components.LinkDisplayItem`. **Backlinks must set `NoteLink.SourceNote`** — `notes_pane.go:231,455` dereferences `link.NoteLink.SourceNote.Slug`; resolve each `src` id → `models.Note{Slug,Title}` or the pane panics. Outgoing sets `TargetSlug`/`TargetNoteID`/`DisplayText`(title)/`IsResolved`.

`models.Note`/`models.NoteLink` are NOT banned by the strict check (only `internal/journal`/`internal/service`), so `note_picker.go`/`notes_pane.go` need no decoupling.

### Mutation flow (writes — verbs only)
Every mutation: focused pane emits an action → `tea.Cmd` calls the verb (captured `vaultDir`) → on success runs `ix.Reconcile()` (eager, non-fatal per `today.go:367-378`) → emits `mutationDoneMsg` → model fires that pane's reload cmd → pane re-selects previously-focused row **by ID, not index** (§8 selection-identity pitfall). The TUI never calls `writeFileAtomic` directly.

- Add todo: text-entry sub-flow → `addDurableTodo(todosDir, author, body, sched, deadline, "", "")` (scenario 5).
- Add log: text-entry sub-flow → `appendLogEntry(logDir, day, hhmm, author, body)` (scenario 7).
- Create note: sub-flow → new `createNote(...)` (Decision 2).

### Agenda actuator sub-flow (AC §2.11, scenarios 6, 8)
1. **Read-only guard first**: if the selected `agendaItem.ReadOnly` is true, reject with the exact string `"is read-only (external work ticket); use rk today open instead"` and touch no file. **Gate on the loaded row's `ReadOnly` flag — do NOT rely on the verb path**: `runTodayActE`'s guard (`today.go:348-354`) lives in the cobra handler, not in `dispatchTodayAct`; calling `dispatchTodayAct` on a work-ticket ref instead falls through to `loadNativeTodoForEdit` and returns the wrong `"no todo found"` error (scenario 8 asserts the exact read-only string).
2. **No-arg keys** `t`/`x`/`i`/`c`: immediately `dispatchTodayAct(vaultDir, ref, key, "", logDid)`. **`logDid` defaults `true`** (today.go:570-571: CLI's `--no-log` flag defaults off, i.e. logging is on by default) — this is the design doc's "complete-as-logging" behavior (§ design rationale: `x` emits a `log-entry` linked `did`→task by default, toggleable), the central point of the agenda pane existing at all. Do not silently drop it behind an ambiguous `noLog` placeholder; pass `logDid=true` explicitly for `x`, and surface a toggle (not required for v1, but the default must match the CLI's).
3. **Arg keys** `d`/`D`/`p`: open an input sub-flow, then `dispatchTodayAct(vaultDir, ref, key, arg, logDid)`:
   - `d`/`D`: `components.DatePicker` → emit **`YYYY-MM-DD`**. The verbs' `resolveDeferDate`/`parseSchedDate` accept that literal, so emitting a concrete date sidesteps the CLI-vs-TUI parser-syntax divergence entirely — **`components.ParseRelativeDate` is deliberately not used for actuator args** (open item resolved). `ParseRelativeDate` stays in `date_parser.go` unused but journal-free (not newly-introduced dead code).
   - `p`: tiny A/B/C prompt.
4. Recurring `x`: `completeDurableTodoNode`'s recurrence branch advances `scheduled` and keeps `state:open`; the pane must render the reloaded row's actual `state` (not force `done`) — edge case + scenario 6.

### Notes-pane composition (Decision 2)
Composite wrapper owns focus at the top level and holds a `mode` (browse | inspect):
- **browse** (default): `NotePicker` visible+active (call `Show(listNotes(...))` at load; it renders its own boxed list within the pane region). Keys route to `NotePicker`.
- On `NotePickerSelectMsg{NoteSlug}`: resolve slug→id, fire `loadNotesPaneLinks` cmd, `NotesPane.SetLoading(id, true)`; on `notesLinksLoadedMsg` call `NotesPane.UpdateLinks(...)`, switch `mode=inspect`. Keys route to `NotesPane`.
- **inspect**: `NotesPane` active. `Esc` → back to browse (re-`Show` picker). `NotesPane`'s `LinkSelectedMsg{NoteID}` (enter on a link) → load that note's links, stay in inspect (graph navigation).
- Focus-cycle (global Tab) treats the whole composite as one of the 4 panes; the composite's internal mode is independent of top-level focus. Key-conflict note: `NotePicker` uses `/` (filter) and `Esc` (cancel, no-op in browse); `NotesPane` uses `j/k/g/G/enter/space` — none collide with global Tab/quit.

### Package placement
All orchestration in `package cli` (verbs are unexported; exporting the whole verb surface for a revived `internal/tui` root package is a larger, unscoped refactor). `internal/tui/components` stays a separately-imported presentation package (no cycle: `cli` imports `components`, not vice-versa).

### Clock consistency (§8)
Agenda due/overdue **truth** comes from `buildAgenda`'s UTC-clocked query (`todoNow()`). `task_list.go`'s `FormatDateInfo`/`GetDateStyle` (local-clock) are used **display-only** for the friendly date string/color, never as a second due/overdue source — avoids non-UTC midnight-boundary disagreement.

## Test scenarios (map to files/functions)

| # (AC §4) | Given/when/then (one line) | Test location |
|---|---|---|
| 1 | N durable + M ephemeral → todos pane lists open items, subject-only | `tui_test.go::TestTodosPaneLoad` (asserts `Title`, no body) |
| 2 | 3-entry day file → log pane shows 3 `log-entry` rows | `tui_test.go::TestLoadLogEntries` (via `loadLogEntries`) |
| 3 | note with 2 fwd + 1 backlink → counts match; backlink `SourceNote` populated (no panic) | `tui_test.go::TestLoadNotesPaneLinks` + browse→inspect via `TestNotesComposition` |
| 4 | todo scheduled-today + work-ticket deadline-today → both appear, ticket flagged read-only, matches `buildAgenda` | `tui_test.go::TestAgendaPaneLoad` |
| 5 | submit "buy milk" → `addDurableTodo` called, `todos/<ULID>.md` exists `state:open`, appears after Reconcile | `tui_test.go::TestAddTodoFlow` |
| 6 | press `x` → `completeDurableTodoNode`; plain→`done`; `repeat:`→`scheduled` advances, `state:open`; **a linked `log-entry` node is created (`logDid=true` default)** | `tui_test.go::TestAgendaActDone` + `TestAgendaActDoneRecurring` + `TestAgendaActDoneEmitsLogEntry` |
| 7 | submit log entry → `appendLogEntry`, byte-identical block | `tui_test.go::TestAddLogFlow` |
| 8 | actuator key on work-ticket row → exact read-only error, file untouched | `tui_test.go::TestAgendaReadOnlyGuard` |
| 9 | create+complete+log, exit, `rk index` → all 3 reflected, no warnings | `tui_test.go::TestDurabilityAfterRebuild` |
| 10 | `rk todo list --json` (fresh process) after exit → lists TUI-created todo | `tui_test.go::TestMutationVisibleToFreshCLI` |
| 11 | grep `internal/tui/**/*.go` imports → zero `journal`/`service` | `internal/tui/no_journal_import_test.go::TestNoJournalOrServiceImports` (filepath.Walk + `go/parser` over import specs) |
| 12 | 120x40 → `WindowSizeMsg{80,24}` → 4 panes resize, no negative dims/panic | `tui_test.go::TestResizeClampsDimensions` |
| 13 | Tab from todos → focus advances through 4 fixed panes, stable order | `tui_test.go::TestFocusCycle` |

Edge-case tests: empty-vault friendly empty states per pane; malformed agenda date skipped-with-warning; long title truncation (existing `log_view` truncation retained); CRLF verb-refusal surfaced not worked around. Component tests use direct `Update()` calls (repo convention; `teatest` is not a dependency). Scenarios 5-10 are plain in-package Go tests that call verbs directly and read back via files / `index.Rebuild` / `runQuery`.

## Pipeline note: skeleton pass required before test-writing

**[ADDED post-plan, advisor-flagged]** This ticket is greenfield + refactor, not "add a test for existing behavior." A red-state gate (`go test -run '^$' ./...` compiling every `_test.go`) will fail on a test file that references `newTUIModel`/`tuiModel`/`loadLogEntries`/`LogEntryRow`/`DateInfo`/`createNote`/etc. before those symbols exist — and the standard test-writer step is forbidden from creating non-test files, so it cannot fix that itself. Before test-writing: the orchestrator (not a model-generation step — this is mechanical, signatures only) writes empty skeletons for every new type/function this plan names (zero-value/error returns, no logic) so the package compiles. Test-writer then writes real tests against those skeletons, which fail on assertions (true red state), not on compilation.

## Known risks / ambiguities

- **[DECISION, owner-vetoable]** `schedule_view.go` deleted rather than decoupled — deviates from Decision 1's literal "decouple 6" but meets its stated goal (zero journal imports) and avoids introducing dead code. If the owner wants it decoupled-and-used, the agenda pane must instead adapt it, which requires adding `State`/`ReadOnly`/`Priority`/actuator support it currently lacks.
- **`no_journal_import_test.go` package**: `internal/tui/` root currently has no `.go` file; the test introduces `package tui` there. Confirm no tooling assumes that dir stays package-less.
- **Non-TTY `rk tui`**: out of scope (fnqs.6). Default bubbletea behavior (hang/opaque error) accepted; no guard added.
- **Live external edits**: not detected until the next mutation-triggered `Reconcile()` (no watcher). Optional `tea.Tick` poll is not required by Done-when; recommend omitting for v1.
- **`summary_view.go`**: left unwired; its transitive `internal/time → internal/journal` import is outside `internal/tui` and outside scenario 11's direct-import grep — do not touch.
- **`ParseRelativeDate` divergence**: resolved by emitting `YYYY-MM-DD` from the date picker; the richer TUI parser is not wired to actuator/creation args, keeping CLI/TUI accepted-syntax identical.

### Critical Files for Implementation
- internal/cli/today.go (agenda read `buildAgenda`, actuators `act*`/`dispatchTodayAct`, read-only guard pattern at `runTodayActE:348-354`)
- internal/cli/todo.go (todos read helpers `listDurableTodos`/`loadTodoProps`/`loadDependsOn`, `addDurableTodo`, `completeDurableTodoNode`)
- internal/cli/note_v1.go (`createNote` extraction target; note link/list read helpers)
- internal/cli/root.go (command registration at :99-106; `buildLoggerConfig`)
- internal/tui/components/log_view.go (representative decouple target; notes composition peers `note_picker.go`/`notes_pane.go` live alongside)
