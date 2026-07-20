# reckon-fnqs.6 codebase analysis

## Open decisions (resolve before writing code)

1. **Prompt interface's Update signature vs. Go covariance.** `Update(tea.Msg)
   (Prompt[T], tea.Cmd)` on the interface can't be satisfied by
   `func (f *Form) Update(...) (*Form, tea.Cmd)` — Go has no covariant
   returns. Either (a) change every component's `Update` to literally return
   the interface type, or (b) keep each `Update(...) (*T, tea.Cmd)` as-is and
   give `RunPrompt[T]` a thin per-component adapter (functionally what the
   deleted helper models were, § 2) plus a uniform completion signal on the
   interface (a `Done() (finished, canceled bool)`-style method), since
   generics can't type-switch on each component's distinct `*SelectMsg`/
   `*CancelMsg` type to detect completion generically. (b) is less invasive.
2. **DatePicker has no submit path today** — needs new behavior, not
   extraction (§ 1).
3. **No "id/title/type/props" struct exists to reuse** — must be defined new;
   decide where it lives (`internal/index` vs. `internal/tui/components`)
   (§ 3).
4. **Should NotePicker's `[]*models.Note` input also be retargeted**, not just
   TaskPicker's? Ticket scope item 5 says "the pickers" (plural) and the
   parent epic (composable-redesign.md) is about decoupling from legacy
   domain types generally; the ticket's Problem section names only
   `journal.Task`/`task_picker.go`. `models.Note` isn't banned by the
   `internal/tui` import test (only `internal/journal`/`internal/service`
   are), so nothing forces the change, but leaving it means the "index rows"
   ticket ships one picker still on a legacy-shaped domain type. Needs an
   explicit yes/no, not a default.
5. **TTY guard: check stdin, stdout, or both?** The one documented example
   (internal/cli/AGENTS.md:236) checks stdout only; a prompt reading
   keystrokes cares about stdin too (§ 4).

## Ticket text is stale on two points — read this first

1. **task_picker.go already stopped taking `[]journal.Task`.** PR #162 (commit
   `057f7b0`, already in this worktree's history) retargeted it at a local
   `TaskRow{ID, Title, DateInfo}` (task_picker.go:51-58, `Show` at :173). Scope
   item 5 is *mostly* done for TaskPicker; what's still open is whether
   `TaskRow` itself should become the "index row" type or get replaced by one
   (see § Index row type below). `internal/tui/no_journal_import_test.go`
   statically fails the build if anything under `internal/tui/...` imports
   `internal/journal` or `internal/service` directly — this is the enforced
   guardrail, not a style preference.
2. **The three named helpers no longer exist at all.** `task_picker_helper.go`,
   `note_picker_helper.go`, and `checklist_run_helper.go` (plus
   `task_note_helper.go`) were deleted wholesale in commit `83ca4ef`
   ("Retire the v0 DB-primary verb surface", reckon-fnqs.1) because their
   *callers* (the v0 `rk task`/`rk notes`/`rk checklist` verbs) were deleted
   too. There is nothing left to "replace with RunPrompt" — this ticket is
   greenfield host-building, not a refactor of live call sites. Their old
   bodies are recovered below (§ Helper models) purely as a spec for what
   RunPrompt[T] needs to reproduce.
3. **No `rk checklist` verb exists in v1.** `internal/checklist/{model,repository,service}.go`
   (the domain) survived the v0 cut, but `internal/cli/checklist.go` (the verb)
   did not, and no v1 replacement has been registered (`internal/cli/root.go:99-107`
   lists the 9 live commands: note, today, add, todo, query, index, adopt,
   migrate, tui — no checklist). Scope item 1's Prompt list (form, text_editor,
   date_picker, task_picker, note_picker) doesn't include checklist_run either.
   Treat "checklist_run_helper.go" in the ticket's Problem section as historical
   color, not a target.

## 1. internal/tui/components/ inventory

Prompt-interface candidates (the 5 named in scope item 1):

| File | Exported type | Init? | Update sig | View | Result-shaped getter | Cancel signal |
|---|---|---|---|---|---|---|
| form.go | `Form` | none | `Update(tea.Msg) (*Form, tea.Cmd)` | yes | `GetValues() map[string]string` | `FormCancelMsg` (emitted, not stored) |
| text_editor.go | `TextEditor` | none | `Update(tea.Msg) (*TextEditor, tea.Cmd)` | yes | `GetText() string` | `TextEditorCancelMsg` |
| date_picker.go | `DatePicker` | none | `Update(tea.Msg) (*DatePicker, tea.Cmd)` | yes | `ParsedDate() (time.Time, error)` | **none** — see below |
| task_picker.go | `TaskPicker` | none | `Update(tea.Msg) (*TaskPicker, tea.Cmd)` | yes | `GetSelectedTaskID() string` | `TaskPickerCancelMsg` |
| note_picker.go | `NotePicker` | none | `Update(tea.Msg) (*NotePicker, tea.Cmd)` | yes | `GetSelectedNoteSlug() string` | `NotePickerCancelMsg` |

Every one of these five: no `Init()` method exists anywhere in the package
(`grep -rn "func.*Init()"` returns nothing) — Init() is a net-new method on
every component, not a rename. All use `Show()`/`Hide()`/`IsVisible()` for
visibility (a pattern the persistent `rk tui` still depends on directly — see
§ Integration risk). None currently satisfy `tea.Model` (Update returns the
concrete `*T`, not `tea.Model`); `RunPrompt[T]`'s adapter is exactly what has
to bridge that, same as the deleted helper models did per-component.

**DatePicker has no submit path today.** On Enter with a valid date it does
*nothing* — no Hide(), no Cmd, no message (date_picker.go:118-119, comment:
"Valid date - will be handled by parent"). The current sole caller
(`internal/cli/tui_keyboard.go:273-294`, `handleDateSubFlowKey`) special-cases
Enter itself: calls `m.datePicker.ParsedDate()`, checks the error/zero value,
and drives completion externally. `Prompt`-shaping DatePicker means adding a
real submit signal (a `DatePickerSubmitMsg`, or moving the Enter-handling
into `Update` itself) — this is new behavior, not extraction of existing
behavior. [OPEN] for whoever designs the interface.

Other files in the directory (not in scope — display/pane components, no
modal lifecycle, not touched by this ticket): `collapsible.go` (style consts
only), `date_parser.go` (pure functions: `ParseRelativeDate`, `FormatDate`,
etc. — shared by form/date_picker), `log_view.go` (`LogView`, embedded in
`rk tui`'s log pane), `notes_pane.go` (`NotesPane`, embedded inspect view),
`status_bar.go`, `summary_view.go`, `task_list.go` (holds `DateInfo`, the type
`TaskRow` embeds, plus pure date-formatting helpers), `text_entry_bar.go`
(`TextEntryBar`, embedded in `rk tui`'s create sub-flows).

**Doc files `FORM_README.md` and `INTEGRATION_GUIDE.md`** in this same
directory describe v0-era integration (task/log/note creation commands that
`83ca4ef` deleted). Stale; not authoritative for how RunPrompt/Wizard should
integrate. [OPEN] whether to update or delete them as part of this ticket —
not called out in scope, low cost either way.

## 2. Helper models (all deleted — recovered from git history for spec purposes)

`git show 83ca4ef^:internal/cli/<file>` recovers each. Shape common to all three:

- A private `<x>Model` struct wrapping one component pointer + result fields
  (`taskID string`, `canceled bool`, etc.), implementing bare `tea.Model`.
- `Update` special-cases `ctrl+c` → `canceled=true, tea.Quit`, and the
  component's own `*SelectMsg`/`*CancelMsg` → capture result / `canceled=true`,
  `tea.Quit`. Everything else delegates to the component's own `Update`.
- A public `Pick<X>(...) (result, canceled bool, err error)` function:
  validates non-empty input, constructs the component, calls `.Show(...)`,
  runs `tea.NewProgram(m).Run()` (no `tea.WithAltScreen()` — inline, not
  full-screen), type-asserts the final model, returns.
- `checklistRunModel` (checklist_run_helper.go) is the outlier: it does *not*
  wrap a `components.*` type at all — it's a standalone model driving
  `*checklist.Service` directly (cursor/check/abandon keys), because no
  `components.ChecklistRun` widget was ever built. Since no v1 checklist verb
  exists (see callout above), there's nothing for RunPrompt to generalize
  here yet.

What `RunPrompt[T]` needs to replicate generically: the `ctrl+c`-quits
behavior, the per-component submit/cancel message translation (component-
specific today — needs your Prompt interface to expose this uniformly,
e.g. via a `Done() (finished, canceled bool)`-style method, since Go generics
can't switch on `T`'s specific `*SelectMsg` type), and the "run own
`tea.NewProgram`, type-assert, return" pattern. None of the five components
do async I/O in `Update` (no `tea.Cmd` closures reading services), so the
closure-capture pitfall (§ 6) mostly doesn't apply *inside* the components —
it will apply inside whatever `Wizard` step-transition code you write if a
step's completion triggers a service call.

## 3. Index row type — nothing prebuilt matches "id/title/type/props"

No existing Go struct bundles exactly those four fields. What exists instead:

- The index's public `nodes` SQL view: `id, ulid, type, time, author, body,
  loc, title` (internal/index/schema.go:84-85). `title` is derived once at
  index-build time (`internal/index/title.go:deriveTitle`, first non-blank
  body line) and stored as a real column (schema v3).
- Props are a separate view, `node_props(id, key, value)` — one row per
  key, not a map. Every current caller loads them per-id via a local
  `loadTodoProps(db, id) (map[string]string, error)` helper
  (internal/cli/todo.go:548-561) and merges manually.
- Every existing "picker row" type is bespoke, built ad hoc from those two
  queries: `TaskRow{ID, Title, DateInfo}` (task_picker.go:51),
  `*models.Note{ID, Title, Slug, ...}` (internal/models/note.go — NotePicker
  still takes `[]*models.Note`, a legacy-shaped domain type, not banned by the
  import test since `internal/models` isn't in `bannedImports`, but the same
  smell the ticket is asking you to remove from TaskPicker),
  `LogEntryRow{ID, Timestamp, Content, EntryType}` (log_view.go:32),
  `LinkDisplayItem` (notes_pane.go:40). `internal/cli/tui_read.go` is the
  actual place these get assembled from raw `nodes`/`node_props` SQL today
  (`loadLogEntries`, `listNotes`, `loadNoteDisplay`).

[INFERRED] "index rows (id/title/type/props)" in the ticket describes the
`nodes` view's shape, not a Go type to go find — you'll be defining this
struct for the first time (candidates: put it in `internal/index` as a public
query-result type so `internal/tui/components` doesn't need a new dependency
on `internal/cli`'s scattered loaders, or keep it local to `components` as a
`components.Row{ID, Title, Type string; Props map[string]string}` and have
callers (currently in `internal/cli`) map `nodes`/`node_props` into it same as
they do for `TaskRow` today). Either way this is new code, not a find-and-reuse.

## 4. TTY guard — no code exists yet, but the dependency is already free

`grep -rn "isatty\|IsTerminal\|no-input\|NoInput"` across the whole repo
(excluding tests) returns **zero hits**. Nothing to extend.

- `internal/cli/AGENTS.md:231-238` documents the intended pattern verbatim
  (doc-only, never implemented):
  ```go
  func isTTY() bool {
      fileInfo, _ := os.Stdout.Stat()
      return (fileInfo.Mode() & os.ModeCharDevice) != 0
  }
  ```
  Note it checks **stdout**; for a prompt reading keystrokes you also care
  about **stdin** being a real tty (a script piping input while stdout is a
  terminal would still hang bubbletea's input reader). [OPEN] check both.
- `github.com/mattn/go-isatty v0.0.20` is **already in go.sum as an indirect
  dependency** (pulled in transitively via bubbletea → muesli/termenv). Using
  it directly costs nothing new to download; `go mod tidy` just re-labels it
  direct. `golang.org/x/term` was explicitly removed in `83ca4ef` ("go mod
  tidy: drop fsnotify and golang.org/x/term" — orphaned by the v0 tui
  deletion), so don't reach for that.
- Recommendation: stdlib `os.Stat().Mode()&os.ModeCharDevice` (matches the
  documented house pattern exactly, zero import surface, works cross-platform
  since `internal/index/lock_unix.go` / `lock_other.go` show this codebase
  does care about non-unix builds) over adding a direct `go-isatty` import.
  Either is defensible; don't add a new dependency when stdlib already covers it.
- The only current `tea.NewProgram` call in the entire repo is
  `internal/cli/tui.go:53` (`rk tui`) — reading the code, it has **no TTY
  guard today** (no isatty/stat check anywhere in tui.go's call path), so
  `rk tui < /dev/null` should hang on a real bubbletea read-loop.
  [INFERRED, not executed — this is also the ticket's own stated premise
  ("an agent invoking a verb that pops a mini-TUI hangs forever"), not
  independently verified here.] That's the one live prompting verb this
  ticket's guard must cover; every other mini-TUI verb (task schedule/note,
  checklist run, etc.) was deleted with v0 and doesn't exist to guard yet
  (they come back in reckon-fnqs.7, which depends on this ticket).

## 5. CLI flag conventions (cobra)

Persistent flags live as package-level vars in `internal/cli/root.go:24-32`,
registered in `init()` via `RootCmd.PersistentFlags().<Type>Var(&var, "name",
default, "help")` (root.go:91-97), validated together in
`RootCmd.PersistentPreRunE` (root.go:81-88, e.g. the `--json`/`--ndjson`
mutual-exclusion check). `--no-input` should follow this exact shape: a
`noInputFlag bool` beside `quietFlag`, `BoolVar(&noInputFlag, "no-input",
false, "...")`, with its TTY-conflict check added to `PersistentPreRunE` or to
`buildLoggerConfig`-adjacent guard code — whichever verb-entry seam you route
the "would this verb prompt" check through.

Subcommand-local flags reset themselves per-invocation via a
`reset<X>Flags(cmd)` function clearing both the var and the flag's `Changed`
state (root.go doesn't need this since persistent flags never get re-parsed
mid-test-run the same way, but `query.go:64-74` / `todo.go:57-73` show the
pattern for any new subcommand-scoped flag). Exit codes: `ExitCodeUsageErr =
2` is defined (root.go:20) but **never actually used** —
`cmd/rk/main.go` calls `os.Exit(1)` unconditionally on any `Execute()` error,
so today "usage error" and "general error" are indistinguishable at the
process level. Wiring a real exit-code distinction for the TTY guard's error
is out of scope unless you want to fix that pre-existing gap too — the ticket
just says "usage error", which today just means "a returned `error` with a
message telling the user what flag to pass," not a distinct exit code.

## 6. Known pitfalls (docs/REVIEW_PATTERNS.md, AGENTS.md)

- **Closure capture** (REVIEW_PATTERNS.md:117-144, AGENTS.md:308-325): async
  `tea.Cmd` closures must capture model fields by value before the closure,
  not read `m.field` inside it. Frequency-flagged as the most common TUI bug
  (18 occurrences). Relevant if `Wizard`'s step-advance logic returns a
  `tea.Cmd` that reads the shared result map — capture the map/step index
  before returning the closure.
- **Nil component access** (REVIEW_PATTERNS.md:147-161): guard any
  optional-component `.Update()`/`.View()` call with a nil check. Relevant if
  `Wizard` holds `[]Prompt[T]` and steps can be conditionally skipped.
- **Note:** `internal/tui/AGENTS.md`, referenced from the root `AGENTS.md:76`
  and `internal/cli/AGENTS.md:142,168`, **does not exist** — orphaned by the
  same `83ca4ef` deletion of `internal/tui`'s non-components files. The
  closure-capture and nil-component guidance above is the only surviving
  copy (in the root AGENTS.md / REVIEW_PATTERNS.md); don't go looking for a
  TUI-specific AGENTS.md, there isn't one.
- **Box-wrapper sizing** (REVIEW_PATTERNS.md:1029-1058): any bordered/padded
  component (all 5 prompt candidates use `lipgloss...Border(...).Padding(1,
  2)`) must feed its inner widget the content-area size (outer minus
  border/padding), not the raw outer target — `SetWidth` on Form/TaskPicker/
  NotePicker/TextEditor/DatePicker already do this subtraction inline; keep
  doing it the same way if `RunPrompt`/`Wizard` recompute sizes centrally.
- **Key binding conflicts** (REVIEW_PATTERNS.md:696-719): if `Wizard`'s
  ESC-back needs a key the underlying prompt already binds to something else
  (e.g. DatePicker's own ESC = cancel-whole-picker, not step-back), you need
  either a distinct key or a "first ESC = step's own cancel, second ESC
  within N ms = wizard back" convention — pick one and document it, doesn't
  currently exist as a pattern anywhere in this codebase.

## 7. Test conventions

- Unit tests: same package (`package components`), file
  `<component>_test.go`, test names `Test<Component>_<Behavior>` (e.g.
  `TestTaskPicker_UpdateWithEscapeKey`, `TestForm_AddField`). Mixed
  assertion style across files — `form_test.go`/`task_picker_test.go` use
  `testify/assert` (+ `require` where a nil-check gates further calls),
  `date_picker_test.go`/`text_editor_test.go` use bare `t.Error`/`t.Fatal`.
  No enforced house style; match whichever file you're extending.
- Example/integration tests: separate `package components_test` (external),
  file `<component>_example_test.go`, using Go's `Example`/`ExampleXxx`
  convention with `// Output:` comments, demonstrating intended usage
  patterns (constructing a host `model` struct, wiring `*SelectMsg` handling)
  without actually running `tea.NewProgram` (see task_picker_example_test.go
  — comments explicitly say "In a real application, you would run this with
  tea.NewProgram(m)" and never do). These are the closest thing to a spec for
  what a `RunPrompt`/`Wizard` example test should look like.
- **Gap:** `note_picker.go` has no dedicated test file at all (`note_picker_test.go`
  doesn't exist); its only test coverage is indirect, via
  `internal/cli/tui_test.go`. If you're retargeting NotePicker's input type,
  this is a real hole, not a style choice to match.
- `date_picker_test.go:11-16` has a `futureDate()` helper (returns
  `time.Now().AddDate(0,0,30)` formatted) specifically to avoid hardcoded
  date literals becoming time-bombs — reuse this helper (or the pattern) for
  any new DatePicker-adjacent Prompt test rather than a fixed date string.

## Integration risk: DatePicker and NotePicker are already live, embedded, non-modal

Unlike TaskPicker/Form/TextEditor (zero production callers today — genuinely
orphaned), DatePicker and NotePicker are mounted right now inside `rk tui`'s
single long-running `tea.Program`, hand-routed by direct field access in
`tui_keyboard.go`/`tui_model.go`/`tui_panes.go` (§ 1), including a
`SetEmbedded()` mode on NotePicker (note_picker.go:120-134) that skips its own
box border when mounted inline. **Add the Prompt-interface methods alongside
the existing `Show`/`Hide`/`ParsedDate`/etc. — don't remove or change the
signature of any method `rk tui` currently calls directly, or you break the
one working TUI porcelain in the repo.**
