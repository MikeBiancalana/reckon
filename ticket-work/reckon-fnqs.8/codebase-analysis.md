# Codebase analysis — reckon-fnqs.8 (persistent multi-pane `rk tui`)

## 1. What PR #158 deleted under `internal/tui/` (non-components)

Commit `83ca4ef` (`internal/tui root`, `internal/db`, `internal/sync`, `internal/migrate`) deleted, all in `internal/tui/`:
`AGENTS.md`, `commands.go` (+test), `handlers.go` (+test), `keyboard.go`, `layout.go` (+tests), `model.go` (+test), `task_sort.go` (+test). `internal/tui/components/` was untouched.

The command registration (`tuiCmd`) lived in `internal/cli/stubs.go` (deleted in the same commit's leaf-file-deletion step) — not a dedicated `tui.go`. A new `internal/cli/tui.go` is the natural home for the replacement, registered in `root.go`'s `init()` alongside the 8 v1 survivors (`internal/cli/root.go:99-106`).

**Salvage verdict: little to none, by design.** `Model` (`internal/tui/model.go`, git-archived at `83ca4ef^`) is built entirely around `*journal.Service`, `*journal.TaskService`, `*service.NotesService`, `*sync.Watcher` — the legacy DB-journal engine the ticket forbids touching. Every field, every `handle*` method, every async `tea.Cmd` closure in `commands.go`/`handlers.go` reads/writes through those services. None of it can be reused as-is; the model has to be rewritten from scratch against the index + verb functions.

What **is** worth carrying forward as pattern (not code):
- `layout.go`'s `CalculatePaneDimensions(termWidth, termHeight) PaneDimensions` — pure function, no legacy deps, splits height into fixed bottom bars (text-entry 3, summary 1, status 1) and the remaining rows across panes. Reimplement fresh for a 4-pane (agenda/todos/log/notes) layout rather than porting the 2-pane struct.
- `model.go`'s dispatcher-style `Update()` (one `switch msg.(type)` routing to `handle*` methods in `handlers.go`/`keyboard.go`) — same shape is fine for the new model.
- `keyboard.go`'s priority order: date-picker mode > text-entry-focused > confirm-mode > normal-mode-per-focused-pane. Reuse this precedence, not the code.
- `AGENTS.md`'s async-closure-capture warning (see §8/pitfalls) is still exactly correct and still needed — bubbletea `tea.Cmd` closures still capture by reference.

## 2. SQLite index schema and query surface

Schema: `internal/index/schema.go`. Physical tables are private (`_nodes`, `_edges`, `_props`, `_aliases`, `_file_meta`, `_index_meta`, `fts_search` fts5 vtable); **the query contract is the public views** — `nodes(id, ulid, type, time, author, body, loc, title)`, `edges(src, rel, dst, dst_key, from_frag, to_frag)`, `node_props(id, key, value)`, `aliases(alias, id)`, `fts(id, body)`. `fts_search` itself is also public (needed for `MATCH`). Read only through these views, per the ticket's "index only" constraint.

`internal/index/index.go`: `Open(cfg)` / `OpenWithParser` opens (or rebuilds, if schema stale) `<cacheDir>/<vaultID>/index.db` with `?_journal=WAL&_timeout=5000` — WAL + busy-timeout means the TUI holding an open `*Index` concurrently with a separate `rk` CLI invocation (or the TUI's own verb-function file writes) is safe; no exclusive-lock conflict. `ix.DB() *sql.DB` exposes the raw handle for `Query`/`QueryRow`. `ix.Reconcile()` does the lazy hash-based freshness pass; `ix.Close()` releases it. No `Watch`/subscribe API exists — freshness is pull-based only (see §8, fsnotify is gone).

No pane-specific "list todos"/"list log entries" query functions live in `internal/index` itself — every existing reader (`today.go`, `todo.go`, `note_v1.go`) hand-rolls its SQL against the views, inline in `internal/cli`. The new TUI must do the same (or, better, factor shared read helpers — see §3 for the exact functions to reuse/copy).

## 3. Verb functions the CLI commands call (write path) and reusable read helpers

All in package `internal/cli` (unexported — the TUI model must live in `internal/cli` too, or these need selective exporting; no package boundary currently separates "CLI verbs" from "CLI command wiring"). None of them touch `internal/index` for writing — they write markdown files directly, matching the ticket's "index only for reads, verbs for writes" contract.

| Verb | Signature | File:line | Notes |
|---|---|---|---|
| Add durable todo | `addDurableTodo(todosDir, author, body, scheduled, deadline, depends, repeat string) (todoAddResult, error)` | `internal/cli/todo.go:328` | Mints ULID via `mintTodoULID` seam (`node.Mint`). |
| Add ephemeral todo | `addEphemeralTodo(todosDir, author, text string) (todoAddResult, error)` | `internal/cli/todo.go:384` | Appends checkbox line to `todos/inbox.md`. |
| Complete a durable todo (by loaded node) | `completeDurableTodoNode(vaultDir string, n *node.Node, foundPath, ref string, logDid, recurLogDid bool) (todoDoneResult, error)` | `internal/cli/todo.go:685` | Shared by `rk todo done` and `rk today act x`; handles both plain and recurring (repeat:) branches. |
| Resolve a todo file for editing | `loadNativeTodoForEdit(vaultDir, ref string) (*node.Node, string, error)` | `internal/cli/today.go:441` | ULID fast-path then `todos/*.md` walk; use this (not a hand-rolled walk) so the TUI never disagrees with the CLI on ref resolution. |
| Append plain log entry | `appendLogEntry(logDir, day, hhmm, author, body string) (logAddResult, error)` | `internal/cli/add.go:183` | Creates `log/<day>.md` or appends at EOF. |
| Append did-linked log entry | `appendDidLogEntry(logDir, day, hhmm, author, body, didTarget string) (logAddResult, error)` | `internal/cli/add.go:200` | Only used by the recurrence completion path. |
| Create note | inline in `runNoteCreateE` (`internal/cli/note_v1.go:228`) | — | **Not factored into a standalone verb function** — slug/collision/dir validation, `node.NewNode`, and `writeFileAtomic` are all inlined in the `RunE`. If the TUI needs "create note" as a callable verb, either extract the body into a `createNote(...)` function first (small refactor, keeps CLI/TUI behavior identical) or duplicate the ~40 lines of logic — extraction is the safer choice given the ticket's "never open a store the verbs don't own" intent applies equally to "never duplicate a verb's logic". |
| Today agenda actuators | `actPin`, `actDefer`, `actDeadline`, `actPriority`, `actDone`, `actStart`, `actCancel` (all `(vaultDir string, n *node.Node, foundPath, ref string, ...) (todayActResult, error)`) | `internal/cli/today.go:475-613` | These *are* the agenda pane's write path — see §4. |
| Load agenda | `buildAgenda(db *sql.DB, todayStr string) ([]agendaItem, []string, error)` | `internal/cli/today.go:223` | Direct reuse for the agenda pane's data load (index-only read). |
| Load todo props/depends | `loadTodoProps(db, id)`, `loadDependsOn(db, id)` | `internal/cli/todo.go:539,559` | Reuse for the todos pane's data load. |
| List durable/ephemeral todos | `listDurableTodos(db, all, stateFilter)`, `listEphemeralTodos(db, all)` | `internal/cli/todo.go:484,571` | Direct reuse for the todos pane. |
| Write file (all verbs funnel through this) | `writeFileAtomic(path string, data []byte) error` | `internal/cli/adopt.go:249` | Every verb's terminal write step; the TUI itself should never call this directly — always go through a verb, per the ticket's "never open a store the verbs don't own" rule. |

**Package-boundary consequence:** because these verbs are unexported and live in `internal/cli`, the new TUI model/layout/keyboard code must also live under `internal/cli` (e.g. `internal/cli/tui.go` + a new `internal/cli/tui_*.go` split, or a subpackage that `internal/cli` re-exports into) — it cannot be a clean `internal/tui` package calling into `internal/cli` verbs, since Go unexported identifiers aren't visible cross-package. `internal/tui/components` (pure presentation, no CLI coupling) stays a separate importable package as it already is; the *root* TUI package is naturally either merged into `internal/cli` or the specific verbs it needs get exported (`AddDurableTodo` etc.) — **this is a real design decision, not settled by existing code**. [OPEN] Given `internal/cli` already has 30+ files and cobra command boilerplate mixed with the verb bodies, the lower-friction path is putting the new TUI files directly in `internal/cli` (mirrors how `today.go`'s `dispatchTodayAct` already reuses `todo.go`'s internals in-package).

## 4. `rk today`'s split-actuator model (agenda pane source)

`internal/cli/today.go`. Three cobra commands: bare `today` (list), `today act <ref> <key> [arg]`, `today open <ref>`.

- **List** (`runTodayE` → `buildAgenda`, `today.go:173,223`): queries `nodes WHERE type IN ('todo','work-ticket')`, joins in props, applies predicate `(scheduled<=today) OR (deadline<=today) OR (pinned==today)` AND (native rows only) `state IN (open,in-progress)`. Native and external (`work-ticket`) rows share one `agendaItem` shape (`today.go:93-107`): `ID,Type,Path,State,Scheduled,Deadline,Pinned,Priority,Source,SourceURL,ReadOnly,Body,Title`.
- **Actuate** (`runTodayActE` → `dispatchTodayAct`, `today.go:314,407`): classifies the ref via the index first (`lookupNodeByRefOrAlias`, `today.go:681`) — a `work-ticket` row is rejected read-only before any file touch (the "split" in split-actuator). For native rows, resolves to a file (`loadNativeTodoForEdit`) and dispatches on a single-letter key: `t`=pin-today, `d`=defer(tomorrow|next-week|YYYY-MM-DD), `D`=deadline, `p`=priority(A/B/C), `x`=done (delegates to `completeDurableTodoNode`), `i`=start(in-progress), `c`=cancel. Each `act*` function mutates one frontmatter field via `setOrInsertField` (`today.go:467`) and writes with `writeFileAtomic`, then the caller does an **eager, non-fatal Reconcile** (`today.go:376`) so the just-written state is immediately visible to a following read.
- **Open** (`runTodayOpenE`, `today.go:619`): read-only, prints `source-url` for external rows.

For the agenda pane this is a near-complete blueprint: TUI keypresses map 1:1 onto the `t/d/D/p/x/i/c` actuator keys, calling the same `act*` functions (or `dispatchTodayAct` directly) rather than duplicating field-mutation logic. Reconcile-after-write is the same pattern the TUI needs after every mutation from any pane.

## 5. `internal/tui/components/*.go` public API and import audit

**Import audit result: the components package imports `internal/journal`, `internal/models`, `internal/logger`, `internal/time` — it does NOT import `internal/service`, `internal/sync`, `internal/db`, or the parent `internal/tui` package.** This still holds today (`go build ./...` succeeds; `internal/journal`/`internal/models` were never deleted by PR #158 — only their *consumers* in `internal/tui` root and 15 `internal/cli` v0 leaf files were). So the components package is safe to keep importing as-is: `journal.Task`, `journal.LogEntry`, `journal.Win`, `journal.Intention`, `journal.ScheduleItem`, `models.Note`, `models.NoteLink` are plain structs with no DB/service coupling — they're display-layer DTOs. The new TUI constructs these structs by hand from index query rows; it never calls `journal.Service`/`journal.TaskService`/`service.NotesService` methods.

| File | Type | Constructor | Data type consumed | Update/View | Notes |
|---|---|---|---|---|---|
| `task_list.go` | **not a widget** — helper funcs only | — | `journal.Task` | — | `FormatDateInfo(task)`, `GetDateStyle(task)`. No `TaskList` struct/Update/View exists; old `model.go` hand-rolled its own list rendering (`renderTaskList`/`renderDetailArea`) using these helpers. The todos pane needs the same hand-rolled approach — there is no ready-made list widget for todos. |
| `log_view.go` | `LogView` | `NewLogView(logEntries []journal.LogEntry) *LogView` | `journal.LogEntry` | `Update(tea.Msg)(*LogView,tea.Cmd)` / `View() string` | `SetSize`, `SetFocused`, `UpdateLogEntries`, `SelectedLogEntry()`, emits `LogNoteAddMsg`/`LogNoteDeleteMsg`. |
| `wins_view.go` | `WinsView` | `NewWinsView(wins []journal.Win) *WinsView` | `journal.Win` | same shape | Not one of the 4 target panes but available. |
| `intention_list.go` | `IntentionList` | `NewIntentionList(intentions []journal.Intention) *IntentionList` | `journal.Intention` | same shape | Not one of the 4 target panes. |
| `schedule_view.go` | `ScheduleView` | `NewScheduleView(items []journal.ScheduleItem) *ScheduleView` | `journal.ScheduleItem` | `View()`/`Update()`; `UpdateSchedule`, `SelectedItem()` | Candidate base for agenda pane rendering if reshaped, though agenda's native data shape is `agendaItem` (today.go), not `journal.ScheduleItem` — needs an adapter either way. |
| `notes_pane.go` | `NotesPane` | `NewNotesPane() *NotesPane` (no-arg; populate via `UpdateLinks`/`SetLoading`) | `[]LinkDisplayItem{NoteLink models.NoteLink, DisplayText, IsResolved}` | `Update`/`View`, `SetSize`, `SetFocused` | Shows **links of one selected note** (outgoing+backlinks), not a browsable note list. |
| `note_picker.go` | `NotePicker` | `NewNotePicker(title string) *NotePicker` | `*models.Note` (fuzzy-filterable list) | `Update`/`View` | This is the "list of notes" widget — closer fit for a notes pane than `notes_pane.go` if the requirement is "browse notes" rather than "inspect one note's graph." Emits `NotePickerSelectMsg{NoteSlug}` / `NotePickerCancelMsg`. |
| `task_picker.go` | `TaskPicker` | `NewTaskPicker(title string) *TaskPicker` | `journal.Task` (fuzzy-filterable) | `Update`/`View` | Modal-style picker, not persistent-pane shaped as-is but reusable for e.g. a "link todo" flow. |
| `summary_view.go` | `SummaryView` | `NewSummaryView() *SummaryView` | `*internal/time.TimeSummary{Meetings,Breaks,Tasks,Untracked,TotalTracked}` | `View()`; `SetSummary`, `SetWidth`, `Toggle`/`IsVisible`/`SetVisible` | Time-tracking summary bar; no index-backed source exists yet for this data — likely out of scope for v1 unless dropped or stubbed. |
| `status_bar.go` | `StatusBar` | `NewStatusBar() *StatusBar` | primitives (`SetDate`, `SetSection`, `SetInputMode`, `SetNoteSelected`) | `View()` only (no Update — stateless bar) | Directly reusable for the persistent status/help line. |
| `form.go` | `Form` | `NewForm(title string) *Form`, `.AddField(FormField)` | `FormField{Label,Key,Type(Text\|Date),Required,Placeholder,Validator}` → `FormResult{Values map[string]string}` | `Update`/`View`; emits `FormSubmitMsg{Result}` / `FormCancelMsg` | Generic, no journal/service coupling — the right vehicle for "add todo"/"add log entry" input inside the TUI, whose submit handler calls the verb functions from §3 directly (per ticket: "not routed through a shared Prompt interface yet"). |
| `date_picker.go` | `DatePicker` | `NewDatePicker(title string) *DatePicker` | primitive | `Update`/`View` | Reusable as-is for schedule/deadline entry. |
| `date_parser.go` | funcs only | — | strings | `ParseRelativeDate(input string) (time.Time, error)` (`t/tm/mon.../+3d/+2w/YYYY-MM-DD`) | Distinct from and richer than `internal/cli`'s `parseSchedDate`/`resolveDeferDate` (`recur.go:97`, `today.go:491`) which only handle `tomorrow`/`next-week`/literal date. Decide which parser the TUI's date entry uses — mixing both risks inconsistent accepted syntax between CLI and TUI for the same field. [OPEN] |
| `text_editor.go` | `TextEditor` | `NewTextEditor(title string) *TextEditor` | string | `Update`/`View`; `TextEditorSubmitMsg`/`TextEditorCancelMsg` | Multi-line body editor — useful for todo/note body entry. |
| `text_entry_bar.go` | `TextEntryBar` | `NewTextEntryBar() *TextEntryBar` | string | `Update`/`View` | Single-line capture bar, matches old model's bottom-bar pattern. |
| `collapsible.go` | helpers only | — | — | — | `CollapseIndicatorExpanded`/`Collapsed` glyphs used by `notes_pane.go`. |

## 6. Command registration entry point

`cmd/rk/main.go` just calls `cli.Execute()`. Command tree lives entirely in `internal/cli`. `RootCmd` and the 8 v1 survivors are registered in `internal/cli/root.go:99-106`:
```
GetNoteCommand(), todayCmd, addCmd, todoCmd, queryCmd, indexCmd, adoptCmd, migrateCmd
```
Add a 9th: `RootCmd.AddCommand(tuiCmd)` in that same `init()` block, with `tuiCmd` defined in a new `internal/cli/tui.go`. Old pattern to follow for the command body (`internal/cli/stubs.go` at `83ca4ef^`):
```go
var tuiCmd = &cobra.Command{
    Use: "tui", Short: "Launch the interactive terminal UI",
    RunE: func(cmd *cobra.Command, args []string) error {
        cfg := buildLoggerConfig(true) // isTUIMode=true — suppresses alt-screen log spam
        if err := logger.InitializeWithConfig(cfg); err != nil { ... }
        model := newTUIModel(...)      // NEW: constructed from index + vault config, not journalService
        p := tea.NewProgram(model, tea.WithAltScreen())
        _, err := p.Run()
        return err
    },
}
```
`buildLoggerConfig(isTUIMode bool)` (`root.go:35`) is unchanged and still exactly fits this call. The old `Annotations: map[string]string{"requiresDB": "true"}` no longer applies — that annotation mechanism was deleted along with `PersistentPreRunE`'s ancestor walk (PR #158 commit "Strip inert requiresDB annotations"); the new command needs no annotation, just an `index.Open(cfg)` call inside `RunE` (mirrors `today.go:186`).

`TestRootHelp_ListsSubcommands` (`internal/cli/root_help_test.go:21-46`) asserts exactly `{add, adopt, migrate, index, note, query, today, todo}` appear in `--help` output — adding `tui` doesn't break this substring-match test (it only checks presence of the 8, not absence of others), but consider adding `"tui"` to the asserted list for symmetry.

## 7. Root bubbletea model composition pattern (pre-deletion reference)

`Model` (archived `internal/tui/model.go` at `83ca4ef^`) shape to imitate structurally (not copy):
- Single struct holding all sub-component pointers + primitive UI state (focused section enum, mode flags: `helpMode`, `confirmMode`, `datePickerVisible`, etc.) + `width`/`height` from `tea.WindowSizeMsg`.
- `Init()` returns `tea.Batch` of initial load commands.
- `Update()` is a flat `switch msg.(type)` dispatch table to `handle*` methods, with `tea.KeyMsg` routed through a priority chain in `keyboard.go`: date-picker > text-entry-focused > confirm-mode > normal-mode (which itself branches on focused section).
- `View()` branches on top-level modal state first (terminal-too-small, confirm-mode, help-mode) before falling into the normal layout renderer, which sizes each pane from a `CalculatePaneDimensions`-style function and joins panes with `lipgloss.JoinHorizontal`/`JoinVertical`, ending with a `strings.Join` of non-empty bottom-bar parts (never unconditional `+"\n"+` concatenation — see §8).
- Message types are plain structs per async result (`journalLoadedMsg`, `tasksLoadedMsg`, `errMsg{err}`, etc.) — for the new model, the equivalents are index-query-result messages (e.g. `agendaLoadedMsg{items []agendaItem}`, `todosLoadedMsg`, `logLoadedMsg`, `notesLoadedMsg`) produced by `tea.Cmd` closures that call the §3/§4 read helpers against a captured `*index.Index`/`*sql.DB`.

No `internal/sync.Watcher` equivalent is available (see §8) — `Init()` should not attempt to start a file watcher; periodic/on-action `Reconcile()` is the only freshness mechanism now.

## 8. Pitfalls (docs/REVIEW_PATTERNS.md + repo-state findings)

| Pitfall | Where documented | Applies here because |
|---|---|---|
| **Async closure capture** — `tea.Cmd` closures capture model fields *by reference*; must snapshot into a local `captured*` var before returning the closure. | `docs/REVIEW_PATTERNS.md:117-145` (18 occurrences, most frequent TUI bug) | Every load-agenda/load-todos/load-log/load-notes `tea.Cmd` and every verb-call `tea.Cmd` needs this. |
| **Nil component access** — components can be nil during partial init; guard with `if m.pane != nil`. | `docs/REVIEW_PATTERNS.md:147-169` | 4-pane static layout still has an init-order window before all panes are populated. |
| **Missing keyboard handler for a section** — adding a focusable pane without adding its `Update`-routing case silently swallows its keys. | `docs/REVIEW_PATTERNS.md:651-693` | Exactly 4 panes to wire (agenda/todos/log/notes); checklist in that section is directly applicable. |
| **Key binding conflicts** (component wants a key already claimed globally, e.g. Tab). | `docs/REVIEW_PATTERNS.md:696-719` | The agenda pane's single-letter actuator keys (`t/d/D/p/x/i/c`) must not collide with pane-focus-cycling or global quit/help keys. |
| **Stale async data not rejected** — track a context ID and drop late-arriving updates that no longer match current selection. | `docs/REVIEW_PATTERNS.md:722-751` | Relevant if any pane loads per-selection detail (e.g. a "selected todo's notes") asynchronously. |
| **Computed scroll offset not applied in View()**. | `docs/REVIEW_PATTERNS.md:755-787` | Applies to any pane with more rows than height (todos/log especially). |
| **Unconditional `"\n"`-join of possibly-empty view sections** creates phantom blank lines / height overflow. | `docs/REVIEW_PATTERNS.md:879-901` | Bottom-bar assembly (status/summary/text-entry) must build a `[]string` and skip empties, per the `strings.Join(parts, "\n")` pattern already shown in old `model.go`'s `renderNewLayout`. |
| **Dead code after refactoring** — orphaned struct fields/switch cases/helpers once a feature path is removed. | `docs/REVIEW_PATTERNS.md:905-923` | Directly relevant to this ticket's own rebuild: don't carry over `Model` fields (e.g. `confirmItemType` values like `"intention"`/`"win"`) that have no corresponding pane in the new 4-pane scope. |
| **Index-only selection identity after list re-sort/reload** — preserve selection by ID, not numeric index. | `docs/REVIEW_PATTERNS.md:957-978` | Any pane reloaded after a verb-call mutation (which is every pane, since every mutation triggers a Reconcile+reload) must re-locate the previously-selected row by ID. |
| **`time.Parse` vs `time.ParseInLocation` for calendar-date comparisons** — UTC-parse + local-format "today" mismatches for non-UTC users. | `docs/REVIEW_PATTERNS.md:929-951`; also live in `task_list.go:49-53,131` (`localToday`, `parseDate` already use `ParseInLocation`/local) vs `internal/cli`'s `todoNow()` (`todo.go:33`, deliberately UTC) and `parseSchedDate` (`recur.go:97`, UTC) | The TUI pulls from **both** a UTC-clocked read path (agenda/todo query predicates, which compare against `todoNow()`/UTC) and a local-clocked *component* (`task_list.go` helpers use `time.Local`) — mixing them for "is this due today" display vs. "is this due today" query-filter can disagree near midnight in non-UTC timezones. Keep the pane's own "is it today" logic on the same clock the index query used (UTC), and treat `task_list.go`'s `FormatDateInfo`/`GetDateStyle` as **display-only** friendly-string helpers, not a second source of due/overdue truth. |
| **No file-watcher available.** `internal/sync` (fsnotify-based) was deleted in PR #158; `fsnotify` was `go mod tidy`'d out of `go.mod` entirely. | commit `83ca4ef` message; `go.mod` (no `fsnotify` line) | The old `Model.watcher`/`waitForFileChange` pattern has no replacement — do not attempt to port it. Live external edits to the vault are not detected until the TUI's own next Reconcile (verb-call-triggered, or optionally a `tea.Tick`-based periodic Reconcile if the ticket wants near-live external-edit pickup; not required by the "Done when" criteria, which only demand TUI-originated mutations survive `--rebuild`). |
| **Package boundary for verbs is unexported** (§3) — TUI code calling `addDurableTodo` etc. must live inside `internal/cli`. | n/a — direct code reading | Not a "pitfall" from docs, but a hard constraint the plan must account for; getting this wrong (e.g. starting a fresh `internal/tui` package that tries to import `internal/cli`) would require exporting the whole verb surface, a much bigger surgical change than the ticket implies. |

## Open questions for the plan phase

- [OPEN] Where does the new TUI's Go code physically live — merged into `internal/cli` (verbs stay unexported, TUI is just more files in that package) vs. exporting the verb surface so a revived `internal/tui` package can call in? §3 recommends the former as lower-friction.
- [OPEN] `rk note create`'s logic is inlined in `runNoteCreateE`, not a standalone verb function (§3) — extract first, or accept light duplication?
- [OPEN] Notes pane: `notes_pane.go` (single-note link graph) vs. `note_picker.go` (browsable note list) serve different jobs; ticket says "notes" pane without specifying which interaction — likely wants a browsable list (picker-shaped) with drill-down, i.e. both, composed.
- [OPEN] Date-entry parsing: `internal/tui/components/date_parser.go`'s `ParseRelativeDate` vs. `internal/cli`'s narrower `resolveDeferDate`/`parseSchedDate` — pick one so TUI-entered dates and CLI-entered dates accept the same syntax.
- [OPEN] `summary_view.go` (time-tracking summary) has no index-backed data source in the new world — likely drop from v1's persistent layout rather than force a fake source.
