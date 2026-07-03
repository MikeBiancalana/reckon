# Codebase Analysis: reckon-yk1i — Interactive TUI for checklist runs

## 1. Files Most Likely to Be Modified or Added

### PRIMARY — new file: `internal/cli/checklist_run_helper.go` (or similar name)
Not yet created. Per the ticket, this must live in `internal/cli/` (NOT `internal/tui/` or
`internal/tui/components/`) and follow the exact pattern of `task_picker_helper.go` /
`note_picker_helper.go` / `log.go`: a self-contained Bubble Tea model + a plain Go launcher
function, invoked from a cobra `RunE`. No new `internal/tui/components/*` widget is needed —
the ticket calls for "compact rendering" of a name + checkbox list + hint line, which is simple
enough to render directly with `strings.Builder`/`lipgloss` inside the model's `View()`, the
same way `logMultilineModel.View()` composes `editorView + "\n" + hint`.

### PRIMARY — `internal/cli/checklist.go` (376 lines, existing)
- Add `rk cl run <template>` command (the new human-friendly entry point that launches the TUI).
- Add `rk cl abandon <template>` command (non-TUI abandon path, item 3 of the ticket).
- Register both in `GetChecklistCommand()` (currently registers `templateCmd`, `checklistStartCmd()`,
  `checklistCheckCmd()`, `checklistStatusCmd()`, `checklistResetCmd()`, `checklistHistoryCmd()` at
  lines 44–49).
- Reuse `printRunStatus(run *checklist.Run)` (lines 378–399) for the abandon confirmation output,
  same as `checklistResetCmd` does at line 326.

### PRIMARY — `internal/checklist/service.go` (232 lines, existing)
- Add `AbandonRun(nameOrID string) error` — marks the active run abandoned WITHOUT starting a new
  one (item 2 of the ticket). `ResetRun` (lines 191–213) must continue to behave exactly as it does
  today (abandon + start fresh); it should NOT be changed in externally observable behavior, though
  it could optionally be refactored to call the new `AbandonRun` internally for DRY-ness (a judgment
  call for the planning phase — see "Design note" below).

### SECONDARY — `internal/checklist/service_test.go` (330 lines, existing)
- Add unit tests for `AbandonRun`: happy path (active run → abandoned, no new run created), no
  active run (should error, mirroring `GetActiveRun`'s `"no active run for %q"` error text), and a
  check that `ResetRun` behavior is unchanged (existing `TestResetRun` / `TestResetRunNoActive`
  already cover this and should keep passing).

### SECONDARY — `internal/cli/root.go` (existing, no functional change expected)
- `checklistService` is already a package global (line 33) wired up in `initServiceE()` (lines
  171–172: `checklistRepo := checklist.NewRepository(db); checklistService = checklist.NewService(checklistRepo)`).
  The new `run`/`abandon` commands can use this global exactly like the other checklist commands do
  — no wiring changes needed here.

### SECONDARY — new test file: `internal/cli/checklist_run_helper_test.go` (or matching the name chosen above)
- Follows the pattern of `internal/cli/log_test.go` / `internal/cli/task_picker_helper_test.go`:
  construct the model directly (no `tea.NewProgram`), feed it `tea.KeyMsg`/custom msgs via
  `Update()`, and assert on the returned model's fields and `View()` output.

No changes anticipated in `internal/checklist/model.go` or `internal/checklist/repository.go` —
`RunStatusAbandoned` and `UpdateRunStatus` already exist and are sufficient for `AbandonRun` (see
section 3).

---

## 2. Existing Inline TUI Patterns to Follow

Both `internal/cli/task_picker_helper.go` and `internal/cli/log.go` (and the twin
`internal/cli/note_picker_helper.go`) share one shape. This is the pattern the new checklist TUI
must replicate.

### Model struct
A small, unexported struct holding:
- The "business" state needed to render (e.g. `picker *components.TaskPicker` in
  `task_picker_helper.go:13`, or `editor *components.TextEditor` in `log.go:17`).
- A result field(s) set right before quitting (`taskID string` in `task_picker_helper.go:14`,
  `message string` in `log.go:18`).
- `canceled bool` — set to `true` whenever the user backs out (`ctrl+c`, Esc, cancel message).

For the checklist model this would look like (illustrative, not prescriptive):
```go
type checklistRunModel struct {
    service   *checklist.Service
    run       *checklist.Run
    cursor    int
    abandoned bool
    completed bool
    canceled  bool // true on q/Esc — quits but leaves the run active
    err       error
}
```

### `Init() tea.Cmd`
- `task_picker_helper.go:18-20` and `note_picker_helper.go:18-20`: return `nil` — no async init needed,
  the picker's data was already loaded synchronously before `tea.NewProgram` was constructed.
- `log.go:24-26`: returns `m.editor.Show()`, a `tea.Cmd` from the sub-component, when the component
  itself needs an init command.
- For the checklist model, `Init()` can safely return `nil` since the run is already loaded via the
  service before the model is constructed (mirrors the picker pattern, not the editor pattern).

### `Update(msg tea.Msg) (tea.Model, tea.Cmd)`
Common shape across all three files:
1. A `switch msg := msg.(type)` with a `case tea.KeyMsg:` branch that always special-cases
   `ctrl+c` first → sets `canceled = true`, returns `m, tea.Quit`
   (`task_picker_helper.go:24-29`, `log.go:30-34`).
2. Additional `case` branches for component-specific "done" messages
   (`components.TaskPickerSelectMsg` / `TaskPickerCancelMsg` in `task_picker_helper.go:31-39`;
   `components.TextEditorSubmitMsg` / `TextEditorCancelMsg` in `log.go:42-48`; also
   `tea.WindowSizeMsg` in `log.go:36-40` to propagate size to the sub-component).
3. Falls through to delegate the message into the wrapped sub-component and reassign it
   (`m.picker, cmd = m.picker.Update(msg)` at `task_picker_helper.go:43-45`; same shape in
   `note_picker_helper.go:43-45`).

For checklist, since there is no wrapped `components.*` sub-widget, `Update` will directly handle
`tea.KeyMsg` cases for `j`/`down`, `k`/`up` (move cursor), `space`/`enter` (toggle item via
`service.CheckItem`), `a` (call `service.AbandonRun`, set `abandoned = true`, `tea.Quit`), and
`q`/`esc` (set `canceled = true`, `tea.Quit`). After every mutating action it should re-fetch the
run via `service.GetRunStatus(run.ID)` (same re-fetch idiom the service itself uses internally at
`service.go:170-174`) so `Checked`/`Status` reflect DB state, then check
`run.Status == RunStatusCompleted` to trigger the "auto-complete detection" quit path.

### `View() string`
- `task_picker_helper.go:48-50` / `note_picker_helper.go:48-50`: trivially delegate to the
  sub-component's `View()`.
- `log.go:56-66` is the more instructive example for a **compact, hand-built** view: it guards on
  terminal states first (`if m.message != "" || m.canceled { return "" }` — i.e. render nothing once
  the program is about to quit, avoiding a flash of stale UI), then composes
  `editorView + "\n" + hint` where `hint` is a dim, static keybinding legend rendered with a
  package-level `lipgloss.NewStyle().Foreground(lipgloss.Color("240"))` (`log.go:13`,
  `logHintStyle`). **Note the "Unconditional Newline Join" pitfall from REVIEW_PATTERNS.md (see
  section 5)** — `log.go` gets away with unconditional `+ "\n" +` here only because `editorView` is
  never empty in the branch reached; the checklist view must build its lines with
  `strings.Join(nonEmptyParts, "\n")` or conditionally append the hint line, not blind
  concatenation, if any section can legitimately be empty (e.g. an empty checklist).

### How it's invoked from a cobra command
`log.go:74-92` (`runLogMultilineEditor`) and `task_picker_helper.go:55-82` (`PickTask`) are the
templates:
```go
func runXxx(...) (result T, canceled bool, err error) {
    m := xxxModel{ /* seeded state */ }
    p := tea.NewProgram(m)              // <-- NO tea.WithAltScreen()
    finalModel, err := p.Run()
    if err != nil {
        return zero, false, fmt.Errorf("...: %w", err)
    }
    result, ok := finalModel.(xxxModel)
    if !ok {
        return zero, false, fmt.Errorf("unexpected model type returned from picker")
    }
    return result.someField, result.canceled, nil
}
```
This is called synchronously from a cobra `RunE` (e.g. `log.go:106-146`, the `logAddCmd.RunE`),
which then branches on `canceled` before proceeding. Contrast with the **full-screen** TUI entry
point `tuiCmd` in `internal/cli/stubs.go:14-37`, which explicitly passes
`tea.NewProgram(model, tea.WithAltScreen())` — this is the pattern the ticket says NOT to use.

### Error/exit handling
- `p.Run()` errors are wrapped with `fmt.Errorf("...: %w", err)` — never `os.Exit`.
- The type assertion `finalModel.(xxxModel)` is always guarded with `, ok` and a descriptive error
  on failure (never a bare/panicking assertion) — this matches the "Bare type assertion (no ok
  check)" pitfall flagged in `docs/REVIEW_PATTERNS.md` (reckon-5yk8, line 867).
- Control returns to the calling cobra `RunE`, which decides what to print based on `quietFlag` and
  whether the result was canceled — the TUI itself never calls `os.Exit`.

---

## 3. `checklist.Service` — Current State, Data Model, Storage Layer

File: `internal/checklist/service.go` (232 lines). Struct at line 9:
```go
type Service struct {
    repo *Repository
}
func NewService(repo *Repository) *Service
```

### Current methods (all in `service.go`)
| Method | Lines | Behavior |
|---|---|---|
| `CreateTemplate(name string, items []string) (*Template, error)` | 19-40 | Validates non-empty name, rejects duplicate names, persists via `repo.SaveTemplate`. |
| `GetTemplate(nameOrID string) (*Template, error)` | 43-55 | Tries name lookup first, falls back to ID lookup. **This name-or-ID resolution pattern is reused everywhere** and should back `AbandonRun`'s parameter too. |
| `ListTemplates() ([]*Template, error)` | 58-60 | Delegates straight to repo. |
| `DeleteTemplate(nameOrID string) error` | 63-69 | Resolves template then deletes (cascades to runs via FK `ON DELETE CASCADE`). |
| `AddTemplateItem` / `RemoveTemplateItem` | 71-104 | Template item CRUD with position recompaction. |
| **`StartRun(nameOrID string) (*Run, error)`** | 106-126 | Resolves template, checks `repo.GetActiveRunByTemplate` — **errors if one already exists** (`"an active run already exists for %q (use 'reset' to start fresh)"`), else `NewRun(tpl)` + `repo.SaveRun`. |
| **`GetActiveRun(nameOrID string) (*Run, error)`** | 129-143 | Resolves template, fetches active run; **returns an error (not nil, nil) when none exists**: `"no active run for %q (use 'start' to begin)"`. Important for the new `rk cl run` command's "resume or start" logic — callers cannot just check for a nil run, they must catch/branch on this error. |
| **`CheckItem(runID string, position int) error`** | 145-184 | Toggles checked state at 0-based position, sets/clears `CheckedAt`, re-fetches the run and calls `allChecked(updated.Items)` (line 176, helper at 220-231) to auto-transition `RunStatusActive → RunStatusCompleted` via `repo.UpdateRunStatus(runID, RunStatusCompleted, &now)`. This exact "auto-complete on all-checked" logic is what the TUI needs to detect after each toggle to show its own completion message and exit. |
| `GetRunStatus(runID string) (*Run, error)` | 187-189 | Thin wrapper over `repo.GetRunByID`. |
| **`ResetRun(nameOrID string) (*Run, error)`** | 191-213 | Resolves template, if an active run exists calls `repo.UpdateRunStatus(existing.ID, RunStatusAbandoned, nil)` (abandon half), then unconditionally creates+saves a new run (start half). **This is exactly the "abandon" half that `AbandonRun` needs to extract/mirror**, minus the "start new run" second half. |
| `ListRuns(includeCompleted bool) ([]*Run, error)` | 216-218 | `repo.ListRuns(!includeCompleted)`. |
| `allChecked(items []RunItem) bool` | 220-231 | Private helper; returns `false` for an empty item slice (a template with zero items can never auto-complete). |

### Where `AbandonRun` fits
There is **no existing `AbandonRun` method** — confirmed via `grep -rn "AbandonRun" .` returning
nothing. The natural implementation mirrors `ResetRun`'s abandon half (lines 198-206) but stops
there:
```go
// AbandonRun marks the active run for a template as abandoned without starting a new one.
func (s *Service) AbandonRun(nameOrID string) error {
    tpl, err := s.GetTemplate(nameOrID)
    if err != nil {
        return err
    }
    existing, err := s.repo.GetActiveRunByTemplate(tpl.ID)
    if err != nil {
        return fmt.Errorf("failed to check for active run: %w", err)
    }
    if existing == nil {
        return fmt.Errorf("no active run for %q to abandon", tpl.Name)
    }
    if err := s.repo.UpdateRunStatus(existing.ID, RunStatusAbandoned, nil); err != nil {
        return fmt.Errorf("failed to abandon run: %w", err)
    }
    return nil
}
```
**Design note for the planning phase:** `ResetRun` could be refactored to call this new
`AbandonRun`-equivalent logic internally to avoid duplicating the "find + abandon active run"
block, but `AbandonRun` returns `error` (no run) while `ResetRun` needs the found run's ID only to
abandon it and doesn't need to return it — a small helper like
`s.abandonActiveRunIfExists(tpl *Template) error` could be shared by both. This is a nice-to-have
DRY cleanup, not a requirement — `ResetRun`'s existing test coverage (`TestResetRun`,
`TestResetRunNoActive` in `service_test.go:244-279`) must keep passing unchanged either way.

### "Abandoned" state already exists
`internal/checklist/model.go:48-55`:
```go
type RunStatus string
const (
    RunStatusActive    RunStatus = "active"
    RunStatusCompleted RunStatus = "completed"
    RunStatusAbandoned RunStatus = "abandoned"
)
```
No schema or model changes are needed — `RunStatusAbandoned` and the `UpdateRunStatus` repo method
that persists it already exist and are exercised today by `ResetRun`.

### Run / RunStatus data model
`internal/checklist/model.go`:
- `Template` (lines 10-16): `ID, Name, Items []TemplateItem, CreatedAt, UpdatedAt`.
- `TemplateItem` (30-36): `ID, TemplateID, Text, Position`.
- `Run` (57-66): `ID, TemplateID, TemplateName, Status RunStatus, Items []RunItem, StartedAt, CompletedAt *time.Time`.
- `NewRun(t *Template) *Run` (68-94): copies template items into fresh `RunItem`s with `Checked: false`, assigns `run.ID` first, then back-fills `RunID` on each item (line 91-93) — note the two-pass construction, relevant if the TUI ever needs to construct a `Run` client-side (it shouldn't; always go through the service).
- `RunItem` (96-105): `ID, RunID, TemplateItemID, Text, Position, Checked bool, CheckedAt *time.Time`.

### Storage/persistence layer
`internal/checklist/repository.go` (371 lines) wraps `*storage.Database` (`internal/storage/database.go`).
Schema (from `internal/storage/database.go` lines 116-159):
- `checklist_templates`, `checklist_template_items` (FK cascade delete),
  `checklist_runs`, `checklist_run_items` (FK cascade delete),
  indexes on `checklist_runs(template_id)`, `checklist_runs(status)`, `checklist_run_items(run_id)`.
- All repo writes that touch multiple tables use `r.db.BeginTx()` / `tx.Commit()` with
  `defer tx.Rollback()` immediately after begin (`SaveTemplate` at repository.go:22-44, `SaveRun` at
  176-219) — the safe-rollback-after-commit-is-a-noop idiom. `AbandonRun` only needs a single-row
  `UPDATE` via the already-existing `UpdateRunStatus` (repository.go:303-317), so it does **not**
  need a new transaction.
- Relevant existing repo methods for the new service method: `GetActiveRunByTemplate` (252-265,
  returns `(nil, nil)` — not an error — when no active run exists, unlike the service-level
  `GetActiveRun` which converts that into an error) and `UpdateRunStatus` (303-317).

---

## 4. Existing Checklist CLI Commands

File: `internal/cli/checklist.go`. Command tree built in `GetChecklistCommand()` (lines 15-52):
```
rk checklist (alias: rk cl)
├── template (alias: tpl)
│   ├── list
│   ├── add <name> [item...]
│   ├── show <name>
│   ├── delete <name>
│   └── item
│       ├── add <template> <text>
│       └── remove <template> <position>
├── start <template>
├── check <position> [--template=<name>]
├── status [template]
├── reset <template>
└── history
```

### Patterns common to every command
- Guard clause at the top of every `RunE`: `if checklistService == nil { return fmt.Errorf("checklist service not initialized") }`.
- Errors always wrapped: `return fmt.Errorf("failed to X: %w", err)`.
- Output respects `--quiet` (package-level `quietFlag` from `root.go:35`): non-quiet prints a
  `✓ ...` confirmation and/or `printRunStatus`; quiet mode prints only the machine-relevant ID
  (`fmt.Println(tpl.ID)` / `fmt.Println(run.ID)`), e.g. `checklistTemplateAddCmd` (lines 97-101),
  `checklistStartCmd` (217-221), `checklistResetCmd` (324-329).
- Position arguments are always **1-based on the CLI surface**, converted to 0-based before
  calling into the service (`pos1based, err := strconv.Atoi(...)`, then `pos1based-1`) — see
  `checklistTemplateItemRemoveCmd` (186-193) and `checklistCheckCmd` (237-243).
- `checklistCheckCmd` (227-270) uses a **command-local flag** `--template` bound to the
  package-level var `checklistTemplateFlag` (line 12), required because "check" only takes a
  position and needs the template to disambiguate which active run to hit
  (`cmd.Flags().StringVar(&checklistTemplateFlag, "template", "", "Checklist template name (required)")`,
  line 268). The new `rk cl abandon <template>` command should NOT need this flag pattern since
  it takes the template as a positional arg directly (like `reset`/`start`), but if the TUI launch
  command (`rk cl run <template>`) is added as a **sibling** function in the same file, be careful
  not to reuse `checklistTemplateFlag` for an unrelated purpose — it's a shared package-level var.
- `printRunStatus(run *checklist.Run)` (lines 378-399) is the canonical human-readable run
  renderer: `"%s  [%d/%d]\n"` header, then `"  %s %d. %s\n"` per item with `[ ]`/`[x]` marks, then
  a `"  ✓ Complete!"` footer if `run.Status == checklist.RunStatusCompleted`. The new TUI's compact
  view should use the same checkbox convention (`[ ]` / `[x]`) for visual consistency with the
  non-interactive commands, though it will render inline via `lipgloss` rather than raw `Printf`.

### `checklistResetCmd` (lines 310-333) — the closest existing analog to `abandon`
```go
Use:   "reset <template>",
Long:  "Abandons any in-progress run and starts a fresh one.",
Args:  cobra.ExactArgs(1),
RunE: func(cmd *cobra.Command, args []string) error {
    run, err := checklistService.ResetRun(args[0])
    if err != nil { return fmt.Errorf("failed to reset checklist: %w", err) }
    if !quietFlag {
        fmt.Printf("✓ Reset %q — starting fresh\n", args[0])
        printRunStatus(run)
    } else {
        fmt.Println(run.ID)
    }
    return nil
},
```
`checklistAbandonCmd` (new) should follow the same shape but call `checklistService.AbandonRun(args[0])`
(which returns `error`, no `*Run`), so its quiet-mode output can't print a new run ID — it likely
just prints nothing in quiet mode (nothing new was created) or echoes the template name.

### No existing `run` command
`grep -rn "cl run\|checklistRunCmd" .` returns nothing — `rk cl run <template>` is wholly new. Its
`RunE` needs to:
1. Guard `checklistService == nil`.
2. Try `checklistService.GetActiveRun(args[0])`; since that returns an **error** (not a nil run)
   when no active run exists (`service.go:139-141`), the "resume or start" branch must be
   `if err != nil { run, err = checklistService.StartRun(args[0]) }` rather than a nil check —
   worth flagging in the plan since it's easy to misread `GetActiveRun`'s contract.
3. Launch the new inline TUI model seeded with the resolved `run`.
4. After `p.Run()` returns, branch on the model's terminal state (`abandoned`/`completed`/`canceled`)
   for final quiet/non-quiet output, consistent with how `logAddCmd` branches on `canceled` at
   `log.go:116-119`.

---

## 5. Known Pitfalls from `docs/REVIEW_PATTERNS.md` Relevant to This Work

File: `docs/REVIEW_PATTERNS.md` (1026 lines). Most relevant sections for this ticket:

1. **Closure Capture Bug** (lines 117-145, frequency 🔴🔴🔴🔴🔴🔴 — 18 occurrences, the single
   most common TUI issue in this repo): any `tea.Cmd` closure that reads a model field must copy
   it into a local first. If the checklist TUI issues any async `tea.Cmd` (e.g. wrapping
   `service.CheckItem`/`service.AbandonRun` calls as commands rather than calling them synchronously
   inline in `Update`), capture `m.run.ID` / the position into locals before building the closure:
   ```go
   runID := m.run.ID
   pos := m.cursor
   return func() tea.Msg {
       err := m.service.CheckItem(runID, pos)
       return checkItemDoneMsg{err: err}
   }
   ```
   Given the DB calls are fast local SQLite operations, it may be simpler (and safer) to just call
   `service.CheckItem`/`service.AbandonRun` **synchronously inside `Update`** and skip `tea.Cmd`
   entirely (as `CheckItem` does with its own internal re-fetch) — this sidesteps the closure-capture
   class of bug altogether and matches the "compact/simple" spirit of the ticket. Worth calling out
   explicitly in the plan as the preferred approach.

2. **Nil Component Access** (lines 147-169, 🔴🔴🔴 7 occurrences): guard any optional pointer
   before dereferencing — relevant if the model holds `run *checklist.Run` and any code path could
   leave it nil (e.g. `StartRun` failing before the model is even constructed — in that case the
   cobra `RunE` should never construct the model at all, erroring out first, so this shouldn't be
   reachable, but worth a defensive check in `Update`/`View` regardless, per the pattern's guidance).

3. **Bare type assertion** (Pattern Frequency Summary, line 867, `reckon-5yk8`): always use
   `result, ok := finalModel.(checklistRunModel)` with an `ok` check and a descriptive error, not a
   bare `finalModel.(checklistRunModel)` — matches what `task_picker_helper.go:77-80` /
   `note_picker_helper.go:77-80` already do; keep doing it in the new helper.

4. **Ignoring `--quiet` Flag** (lines 204-227, 🔴🔴 5 occurrences): the `rk cl abandon` command
   (non-interactive path) must gate its confirmation `fmt.Printf` on `!quietFlag`, matching every
   other command in `checklist.go`. The interactive TUI itself is inherently non-quiet (it's a
   terminal UI), but any final summary line printed by the wrapping `RunE` *after* `p.Run()` returns
   should still respect `quietFlag`.

5. **Unwrapped Errors** (lines 21-47, 🔴🔴🔴🔴🔴 15 occurrences) and **Unconditional Newline Join**
   (lines 879-901, `reckon-qxem`): every error returned from the new `AbandonRun` service method and
   the new CLI commands must be wrapped with `fmt.Errorf("...: %w", err)`, matching every existing
   method in `service.go`/`checklist.go`. And the TUI's `View()` must not blindly
   `strings.Join`/`+"\n"+` optional sections (e.g., an error banner, a completion banner) — build a
   `[]string` of only the non-empty lines and join once, per the pattern's fix example. This is
   directly applicable since `log.go`'s own `View()` (the reference pattern) is only safe from this
   bug because its `editorView` is guaranteed non-empty in the reachable branch — the checklist
   view has more conditional sections (hint line, completion message, abandon confirmation) and
   needs to be more careful.

6. **Key Binding Conflicts** (lines 696-719): the ticket specifies `j`/`k`/arrows for nav,
   `space`/`enter` for toggle, `a` for abandon, `q`/`Esc` for quit — no conflicts with each other,
   but double check `ctrl+c` (used everywhere else in this codebase as a hard cancel) is still
   wired in addition to `q`/`Esc`, for consistency with `task_picker_helper.go`/`log.go`/`note_picker_helper.go`,
   which all special-case `ctrl+c` explicitly even when other cancel keys exist.

7. **Missing Edge Case Tests** (lines 263-293, 🔴🔴🔴🔴 11 occurrences — the top "critical" pattern
   per the frequency summary at line 854): make sure tests cover empty-template runs (0 items —
   note `allChecked` at `service.go:222-224` explicitly returns `false` for empty item slices, so a
   zero-item checklist can never auto-complete — the TUI should handle that gracefully, e.g. not
   claim "all done" for an empty list), abandon-with-no-active-run, and abandon-when-already-completed.

---

## 6. Test Conventions

### Service-layer tests — `internal/checklist/service_test.go` (330 lines)
- Uses `testify/assert` + `testify/require` (require for setup/fatal assertions, assert for
  value checks) — see imports at lines 8-10.
- `setupChecklistTestService(t *testing.T) *Service` (lines 13-22) is the shared fixture:
  creates a temp dir (`t.TempDir()`), opens a real SQLite DB at `filepath.Join(tempDir, "test.db")`
  via `storage.NewDatabase`, registers `t.Cleanup(func() { db.Close() })`, and wires up a real
  `Repository`/`Service` — **no mocking of the repository/DB layer; tests run against a real
  (temp-file) SQLite instance**. The new `AbandonRun` tests should use this exact fixture.
- One test function per behavior, named `Test<Method><Scenario>` (e.g. `TestResetRun`,
  `TestResetRunNoActive`, `TestCheckAllItemsCompletesRun`, `TestCheckItemUncheck`) — not
  table-driven for these service tests (table-driven style is reserved for pure functions per
  REVIEW_PATTERNS.md's "Good Pattern" section, e.g. date parsing). Follow the same one-test-per-
  scenario style for `AbandonRun`, mirroring `TestResetRun` (lines 244-266) /
  `TestResetRunNoActive` (lines 268-279) with `TestAbandonRun` / `TestAbandonRunNoActive` /
  possibly `TestAbandonRunThenStartFresh` (abandon then confirm `StartRun` succeeds again since no
  active run remains).

### CLI/TUI-layer tests — `internal/cli/task_picker_helper_test.go`, `internal/cli/log_test.go`
- Plain `testing` + `testify/assert` (no `require` needed since these are simpler unit checks).
- **Never actually runs `tea.NewProgram(...).Run()` in tests** — interactive programs can't be
  driven in a headless test the way this codebase tests other things; instead:
  - Construct the model struct directly with a test constructor helper (`newTestLogMultilineModel()`
    in `log_test.go:11-19`) and call `.Update(msg)` / `.View()` directly, asserting on the returned
    model's fields via a type-asserted `result := updatedModel.(logMultilineModel)`
    (`log_test.go:33-34` etc.) — this is the primary pattern to replicate for the new checklist
    model's tests (e.g. `TestChecklistRunModel_ToggleItem`, `TestChecklistRunModel_AbandonKey`,
    `TestChecklistRunModel_AutoCompleteOnAllChecked`, `TestChecklistRunModel_QuitKeepsRunActive`).
  - For pure helper functions not tied to Bubble Tea messages (e.g. `PickOpenTask`'s filtering
    logic), extract and test the underlying logic directly without going through the TUI at all
    (`TestPickOpenTask_FiltersTasks` in `task_picker_helper_test.go:11-45` re-implements the filter
    inline rather than invoking the interactive picker).
  - Struct-shape smoke tests like `TestTaskPickerModel_Structure` (`task_picker_helper_test.go:73-82`)
    just construct the model with literal field values and assert the fields round-trip — useful as
    a cheap first test for the new `checklistRunModel` struct too.
  - `View()` tests check for empty-string short-circuits on terminal states
    (`TestLogMultilineModel_View_WhenDone`, `TestLogMultilineModel_View_WhenCancelled` at
    `log_test.go:80-92`) and non-empty output when active
    (`TestLogMultilineModel_View_WhenActive`, lines 94-100) — the new model's `View()` should have
    equivalent tests for its abandoned/completed/canceled terminal states (matching the `log.go`
    `View()` guard `if m.message != "" || m.canceled { return "" }`, i.e. once the model is about to
    quit, decide explicitly whether `View()` should render a final message or blank).
  - Table-driven style IS used for pure string-transform helpers like `joinLogLines`
    (`TestJoinLogLines`, `log_test.go:102-124`) — if the new code has an analogous pure helper (e.g.
    a checkbox-line formatter), table-drive that test.

### Running tests
No `Makefile` target found; standard `go test ./...` (or scoped `go test ./internal/checklist/...`,
`go test ./internal/cli/...`) is the expected invocation based on repo conventions (module path
`github.com/MikeBiancalana/reckon`, Go 1.25.5 per `go.mod`).

---

## Summary of Concrete Additions Needed

1. **`internal/checklist/service.go`**: add `AbandonRun(nameOrID string) error` (~15 lines, mirrors
   `ResetRun`'s abandon half at lines 198-206).
2. **`internal/checklist/service_test.go`**: add `TestAbandonRun`, `TestAbandonRunNoActive` (and
   optionally a completed-run edge case).
3. **New file in `internal/cli/`** (e.g. `checklist_run_helper.go`): `checklistRunModel` Bubble Tea
   model (Init/Update/View) + `runChecklistTUI(run *checklist.Run) (finalState, error)` launcher
   using `tea.NewProgram(m)` — no `WithAltScreen()`.
4. **New test file**: `checklist_run_helper_test.go` mirroring `log_test.go`'s direct-model-
   construction style.
5. **`internal/cli/checklist.go`**:
   - New `checklistRunCmd()` registering `rk cl run <template>` (alias path via existing `cl` alias
     on the parent), wired into `GetChecklistCommand()` (add to the `cmd.AddCommand(...)` block at
     lines 45-49).
   - New `checklistAbandonCmd()` registering `rk cl abandon <template>`, same wiring location,
     modeled directly on `checklistResetCmd()` (lines 310-333) but calling the new
     `checklistService.AbandonRun` and adjusting output since no new run is created.
