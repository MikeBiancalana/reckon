# Acceptance Criteria: Interactive TUI for Checklist Runs (reckon-yk1i)

## 1. Explicit Acceptance Criteria

1. A new CLI command `rk cl run <template>` is added and becomes the primary human-facing entry point for working a checklist.
2. `rk cl run <template>`: if an active run already exists for the template, the TUI resumes that run (loads its current item/checked state).
3. `rk cl run <template>`: if no active run exists for the template, a new run is started (equivalent to `StartRun`) and the TUI opens on it.
4. The TUI is a Bubbletea program launched with `tea.NewProgram()` **without** `tea.WithAltScreen()` — inline, not full-screen — following the same pattern as `task_picker_helper.go` and `log.go`.
5. The TUI model is a standalone type living in `internal/cli/` (not `internal/tui/`), consistent with `taskPickerModel` / `logMultilineModel`.
6. The TUI shows checklist items with checkboxes inline in the terminal.
7. Arrow keys (Up/Down) and `j`/`k` navigate between items.
8. `space` or `enter` toggles the checked state of the currently selected item.
9. Auto-complete detection: when all items become checked, the TUI shows a completion message and exits on its own (no extra keypress required).
10. Pressing `a` abandons the current run: marks it abandoned and exits the TUI. It must **not** start a new run afterward.
11. Pressing `q` or `Esc` quits the TUI while leaving the run active/untouched for later resumption via `rk cl run` again.
12. Rendering is compact: checklist name, items with checkboxes, and one hint line for keybindings — nothing more elaborate.
13. `checklist.Service` gets a new method `AbandonRun(nameOrID)` that marks the active run for a template as abandoned **without** starting a new run.
14. `ResetRun` is unchanged in behavior: it still abandons any existing active run and starts a fresh one (no regression).
15. A new CLI command `rk cl abandon <template>` is added, calling `AbandonRun` for non-TUI use.
16. `rk cl start` remains as-is: the non-interactive entry point for LLM agents (no output/behavior changes).

## 2. Implicit Requirements

- **"Resume" semantics**: resuming means loading the existing `Run` (via the same lookup `GetActiveRun`/`GetActiveRunByTemplate` uses) with its current per-item `Checked` state intact — not restarting or clearing progress. Toggling in the TUI must persist through the same `CheckItem(run.ID, position)` path the CLI's `cl check` uses, so TUI and CLI stay consistent.
- **Template not found**: `rk cl run <template>` and `rk cl abandon <template>` must fail fast with the same "checklist template %q not found" error `GetTemplate` already produces, before any TUI is drawn or service mutation happens.
- **Run already completed, re-running `rk cl run`**: `GetActiveRunByTemplate` only ever returns rows with `status = 'active'`, so a completed run is invisible to it. Re-running `rk cl run <template>` after completion is therefore **not an error** — it transparently starts a brand-new run, identical to calling `start` when nothing is active.
- **Exit codes**: `cmd/rk/main.go` currently maps *any* non-nil error from `cli.Execute()` to `os.Exit(1)`. The `ExitCodeNotFound`/`ExitCodeUsageErr` constants in `internal/cli/root.go` exist but are not wired to distinct exit paths anywhere in the checklist CLI today. New commands (`run`, `abandon`) should follow the existing convention — return a wrapped error from `RunE` — rather than introduce new fine-grained exit codes.
- **Non-interactive / non-TTY behavior**: no `isatty`/`term.IsTerminal` check exists anywhere in this codebase today — `rk log add` (no args) and the task picker both call `tea.NewProgram()` unconditionally. `rk cl run` should follow the same precedent: no new TTY-detection gate. If stdin/stdout isn't a real terminal, `p.Run()` itself will surface an error, which propagates as the command's error (exit 1), same as the existing inline TUIs.
- **`--quiet` interaction**: `quietFlag` currently gates only *post-action confirmation text* (e.g. `checklistStartCmd`, `checklistResetCmd` print `✓ ...` unless quiet, or print a bare ID when quiet). Since the TUI's own view *is* the interactive output, `--quiet` should not (and cannot meaningfully) suppress TUI rendering; it should only affect any confirmation the CLI prints after the TUI exits (e.g. after abandon/complete), mirroring `checklistResetCmd`'s existing `!quietFlag` / quiet-prints-ID split.
- **AbandonRun error semantics**: if no active run exists for the template, `AbandonRun` should return an error rather than silently no-op — mirroring `GetActiveRun`'s existing "no active run for %q (use 'start' to begin)" pattern. This keeps `AbandonRun` symmetric with `StartRun` (errors if a run *does* exist) and `GetActiveRun` (errors if it *doesn't*).
- **Single abandon code path**: the TUI's `a` key and the CLI's `rk cl abandon` must both call `checklist.Service.AbandonRun` so error/success semantics are identical regardless of entry point.
- **Invariant preserved**: because `GetActiveRunByTemplate` filters to `status='active'`, the TUI can only ever load an active run — never a completed or abandoned one. This must continue to hold; there is no code path where the TUI edits a finished run's items.
- **Return-value convention**: the new TUI helper should follow `PickTask`'s `(result, canceled/outcome, err)` convention — expose enough state on the final model (e.g. completed/abandoned/quit) for the calling `RunE` to decide what to print, the same way `taskPickerModel`/`logMultilineModel` do.
- **Final frame on exit**: because there's no alt-screen, whatever `View()` last rendered stays in scrollback. Unlike `logMultilineModel.View()` (which blanks itself because the caller prints its own confirmation next), the completion message ("show completion message and exit") should be visible in the TUI's own final rendered frame, since the ticket calls that out as TUI-owned behavior, not a follow-up CLI print.
- **Ctrl+C**: should behave like `q`/Esc (quit, leave run active), matching the "ctrl+c" special-case already present in both `taskPickerModel` and `logMultilineModel` — it must not be treated as abandon.

## 3. Edge Cases

- **Empty template (0 items)**: `NewRun` yields `Items == []`; `allChecked([]RunItem{})` returns `false` in the current implementation, so an empty checklist can *never* auto-complete. The TUI must render something sane (e.g. "(no items)"), not crash on navigation with nothing to select, treat space/enter as a no-op, and rely on `a`/`q` as the only way out.
- **Template not found**: both `rk cl run` and `rk cl abandon` fail before touching the service/TUI (see Implicit Requirements).
- **Run already abandoned**: a second abandon attempt (via CLI or defensively via TUI) finds no active run (`GetActiveRunByTemplate` filters it out) → `AbandonRun` returns an error, it does not silently succeed twice.
- **Run already completed**: same as above — a completed run isn't "active," so `AbandonRun` on it errors as "no active run," it does not un-complete or re-abandon it.
- **Run already completed, user runs `rk cl run` again**: not an error — starts a fresh run (see Implicit Requirements).
- **Abandon via CLI vs abandon via TUI**: must be behaviorally identical (same underlying `AbandonRun` call, same error text on failure).
- **Toggling the last unchecked item**: the checked→completed transition happens synchronously inside the service call that processes the toggle. The TUI must detect `RunStatusCompleted` in that same Update() cycle and exit immediately — there is no subsequent keypress in which the user could uncheck the just-completed run through the TUI (no race window to close).
- **No active runs when the TUI is invoked**: cannot happen by construction — `rk cl run` always either resumes an active run or calls `StartRun` first, so the TUI model always initializes with a real, active `Run`.
- **Terminal too narrow / no `WindowSizeMsg` yet**: inline Bubbletea still delivers `tea.WindowSizeMsg`; the TUI should not crash before the first size message arrives, but no new minimum-width handling is being invented beyond what `task_picker_helper.go`/`log.go` already do.
- **Concurrent checklist items**: not applicable — there is a single selection cursor over a single run, and all input is serialized through Bubbletea's Update() loop; no locking concerns beyond what `CheckItem`'s single service call already provides.
- **Two terminals touching the same run** (e.g. `rk cl check` from another shell while the TUI is open): out of scope to reconcile live — the TUI holds its own in-memory copy of the run and only re-syncs on its own key presses, not via polling. See Out of Scope.
- **Nothing to "save" on quit**: every toggle already persists immediately via `CheckItem`, so `q`/Esc requires no flush/save step and cannot lose in-progress state.
- **Argument style for `rk cl abandon`**: it takes a positional `<template>` argument like `reset`/`start`, not a `--template` flag (that flag pattern is specific to `cl check`/`cl status`'s "which active run" disambiguation) — the two styles must not be conflated.

## 4. Test Scenarios (Given/When/Then)

### TUI keybindings

**Scenario: Navigate down with j/Down**
Given a resumed run with 3 items and the cursor on item 1
When the user presses `j` (or Down)
Then the cursor moves to item 2

**Scenario: Navigate up with k/Up**
Given a resumed run with 3 items and the cursor on item 2
When the user presses `k` (or Up)
Then the cursor moves to item 1

**Scenario: Toggle an unchecked item on**
Given the cursor is on an unchecked item
When the user presses `space` (or `enter`)
Then the item becomes checked and the change is persisted via `CheckItem`
And the run remains active (not all items are checked yet)

**Scenario: Toggle a checked item off**
Given the cursor is on a checked item and other items remain unchecked
When the user presses `space` (or `enter`)
Then the item becomes unchecked and the change is persisted

**Scenario: Auto-complete on last item**
Given all items except one are checked
When the user toggles the last unchecked item on
Then the service marks the run `RunStatusCompleted`
And the TUI immediately renders a completion message and exits
And no further keypress is read for that run

**Scenario: Abandon via `a`**
Given a run is active with some items checked
When the user presses `a`
Then `AbandonRun` is called for that run
And the run's status becomes `abandoned`
And the TUI exits without starting a new run

**Scenario: Quit via `q`**
Given a run is active with some items checked
When the user presses `q`
Then the TUI exits
And the run remains `active` with its current item state unchanged

**Scenario: Quit via Esc**
Given a run is active
When the user presses `Esc`
Then behavior is identical to pressing `q`

**Scenario: Quit via Ctrl+C**
Given a run is active
When the user presses `ctrl+c`
Then the TUI exits and the run remains active (same as `q`), not abandoned

### AbandonRun service method

**Scenario: Abandon a fresh active run**
Given a template with an active run that has some (not all) items checked
When `AbandonRun(templateName)` is called
Then it returns no error
And the run's status becomes `RunStatusAbandoned`
And no new run is created
And a subsequent `GetActiveRun(templateName)` returns a "no active run" error

**Scenario: Abandon an already-abandoned run**
Given a template whose only run was already abandoned
When `AbandonRun(templateName)` is called again
Then it returns an error (no active run to abandon)

**Scenario: Abandon an already-completed run**
Given a template whose only run is `RunStatusCompleted`
When `AbandonRun(templateName)` is called
Then it returns an error (no active run to abandon) and the run's status is unchanged

**Scenario: Abandon a nonexistent template**
Given no template named "ghost" exists
When `AbandonRun("ghost")` is called
Then it returns a "template not found" error

### ResetRun regression (must still abandon + start fresh)

**Scenario: Reset with an active in-progress run**
Given a template with an active run that has one of two items checked
When `ResetRun(templateName)` is called
Then the old run's status becomes `RunStatusAbandoned`
And a brand-new active run is returned with all items unchecked
And the new run's ID differs from the old run's ID

**Scenario: Reset with no active run**
Given a template with no active run
When `ResetRun(templateName)` is called
Then no error is returned
And a new active run is started

### `rk cl abandon <template>` CLI command

**Scenario: Abandon an active run via CLI**
Given a template has an active run
When the user runs `rk cl abandon <template>`
Then the command exits 0
And the run is marked abandoned
And (unless `--quiet`) a confirmation is printed; under `--quiet`, the abandoned run's ID is printed instead

**Scenario: Abandon with no active run**
Given a template has no active run (never started, or already abandoned/completed)
When the user runs `rk cl abandon <template>`
Then the command returns an error and exits non-zero

**Scenario: Abandon a nonexistent template**
Given no template named "ghost" exists
When the user runs `rk cl abandon ghost`
Then the command returns a "template not found" error and exits non-zero

**Scenario: `rk cl run` after abandon starts fresh**
Given a template's active run was just abandoned via `rk cl abandon`
When the user runs `rk cl run <template>`
Then a brand-new run is started (not the abandoned one) and the TUI opens on it with all items unchecked

## 5. Out of Scope

- Multi-checklist browsing/switching inside the TUI (no in-TUI template picker/list view) — the template is chosen via the CLI argument before launch.
- Editing checklist item text (add/remove/rename items) from the TUI — remains `rk cl template item add/remove`.
- Template creation/editing from the TUI — remains `rk cl template add`/`show`/`delete`.
- Any full-screen/alt-screen behavior, mouse support, scrollable viewports for long checklists, or persistent dashboards.
- Changes to `rk cl start`'s non-interactive behavior or output — it stays exactly as-is for LLM/agent use.
- Changes to `rk cl check`, `rk cl status`, or `rk cl history` behavior or output.
- New `isatty`/TTY-detection gating before launching the TUI (matches existing precedent — no such gating exists elsewhere in this CLI today).
- Wiring up fine-grained exit codes (`ExitCodeNotFound`, etc.) for checklist commands; `main.go` continues to treat all errors as exit 1.
- Real-time refresh or conflict detection if the same run is modified concurrently from another terminal or a non-interactive CLI call while the TUI is open.
- "Un-abandoning" a run — once abandoned (via TUI or CLI), the only way forward is `rk cl start`/`run` starting a new run.
- Configurable keybindings, themes/colors, or accessibility features beyond what existing inline TUIs (`task_picker_helper.go`, `log.go`) already provide.
- Database schema changes — `RunStatusAbandoned` already exists as a `Run.Status` value and `Repository.UpdateRunStatus` already supports transitioning to it; no migration work is needed.
