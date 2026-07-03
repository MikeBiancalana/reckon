# Implementation Plan: Interactive TUI for Checklist Runs (reckon-yk1i)

## Summary of approach

Add a compact, inline (non-alt-screen) Bubble Tea model in `internal/cli/` that drives a single active checklist run, plus two new cobra commands (`rk cl run`, `rk cl abandon`) and one new service method (`checklist.Service.AbandonRun`). The design mirrors the existing inline-TUI helpers (`task_picker_helper.go`, `note_picker_helper.go`, `log.go`): a small unexported model with `Init/Update/View`, a plain launcher function that calls `tea.NewProgram(m)` (no `WithAltScreen`), and a cobra `RunE` that branches on the final model's terminal state.

Two deliberate structural choices make the work testable without ever spinning up Bubble Tea:
1. The resume-vs-start decision is extracted into a plain helper function (`resolveChecklistRun`) that a headless test can exercise directly.
2. The model holds an injected `*checklist.Service`, so model tests can construct a real temp-SQLite service (same fixture style as `service_test.go`) and assert that toggles actually persist and auto-complete.

All service mutations are called **synchronously inside `Update`** (not wrapped in `tea.Cmd` closures), which sidesteps the closure-capture pitfall (18 occurrences in REVIEW_PATTERNS.md) and matches the "simple, one screen" spirit of the ticket. DB calls are local SQLite operations, so synchronous is fine.

## Files to modify (with reasons)

**`internal/checklist/service.go`** — add `AbandonRun(nameOrID string) (*Run, error)` immediately after `ResetRun` (~line 213). It mirrors `ResetRun`'s abandon half (lines 198–206) but stops before creating a new run, and errors when no active run exists. `ResetRun` is left byte-for-byte unchanged (AC #14, guarded by existing `TestResetRun`/`TestResetRunNoActive`).

**`internal/checklist/service_test.go`** — add `TestAbandonRun`, `TestAbandonRunAlreadyAbandoned`, `TestAbandonRunCompleted`, `TestAbandonRunTemplateNotFound`, using the existing `setupChecklistTestService(t)` fixture (real temp SQLite, no mocks).

**`internal/cli/checklist_run_helper.go`** (NEW) — the inline TUI. Contains:
- `checklistRunModel` struct: `service *checklist.Service`, `run *checklist.Run`, `cursor int`, `completed bool`, `abandoned bool`, `canceled bool`, `err error`.
- `Init/Update/View` methods.
- `resolveChecklistRun(svc *checklist.Service, nameOrID string) (run *checklist.Run, resumed bool, err error)` — the resume-or-start decision (headless-testable, does not touch Bubble Tea).
- `runChecklistTUI(svc *checklist.Service, run *checklist.Run) (checklistRunModel, error)` — the launcher.
- Package-level lipgloss styles (e.g. `checklistHintStyle`, a cursor/selected style), following `logHintStyle` at `log.go:13`.

**`internal/cli/checklist_run_helper_test.go`** (NEW) — model + helper tests following `log_test.go`'s direct-construction style (`m.Update(msg)` then type-assert the returned model; never `tea.NewProgram`). Uses a locally built temp `*checklist.Service` so toggles hit a real DB.

**`internal/cli/checklist.go`** — add `checklistRunCmd()` and `checklistAbandonCmd()`, register both in `GetChecklistCommand()` alongside the existing `cmd.AddCommand(...)` block (lines 45–49). `checklistAbandonCmd` is modeled directly on `checklistResetCmd` (lines 310–333) but calls `AbandonRun` and prints an abandon confirmation; `checklistRunCmd` calls `resolveChecklistRun` then `runChecklistTUI` and branches on the returned state.

**`internal/cli/checklist_test.go`** (NEW) — CLI-command tests. Sets the package global `checklistService` to a temp service, then invokes the command functions' `RunE` (or `.Execute()` with `SetArgs`). Covers the `abandon` command's quiet/non-quiet/error paths and the `resolveChecklistRun` resume-vs-start scenarios. (No existing checklist CLI test file exists today; this is new but uses only exported constructors.)

No changes to `internal/checklist/model.go` (`RunStatusAbandoned` already exists, model.go:54), `internal/checklist/repository.go` (`UpdateRunStatus`/`GetActiveRunByTemplate` already exist), `internal/cli/root.go` (global `checklistService` already wired at root.go:172), or `cmd/rk/main.go` (all errors already map to exit 1).

## Design decisions (with alternatives considered)

### How `rk cl run` decides resume-vs-start
`GetActiveRun` (service.go:129–143) returns an **error** — not `(nil, nil)` — when no active run exists, and the same "not found" error when the template is missing. So a nil-check is wrong. The chosen approach is a small helper:

```
run, err := svc.GetActiveRun(nameOrID)   // active run exists -> resume
if err == nil { return run, true, nil }
run, err = svc.StartRun(nameOrID)        // no active run -> start fresh
if err != nil { return nil, false, err } // StartRun re-runs GetTemplate, so a
return run, false, nil                    // missing template still fails fast
```

This satisfies: resume existing (AC #2), start fresh when none/completed/abandoned (AC #3, #26, and "run after abandon starts fresh"), and fail-fast on missing template (StartRun re-validates via `GetTemplate`, so the "checklist template %q not found" error surfaces before the TUI is drawn).

- **Alternative considered — inline in `RunE`**: same logic but not extracted. Rejected because extracting it makes the resume/start AC scenarios unit-testable without launching Bubble Tea (which can't run headless).
- **Alternative considered — a `Service.ResumeOrStartRun` method** using the private `GetActiveRunByTemplate` to distinguish "template missing" vs "no active run" vs "DB error" precisely. Cleaner error precision, but it's scope creep beyond the ticket (which only names `AbandonRun`) and pushes resume/start policy into the service. Kept as a fallback if reviewers want the decision in the service layer. The minor imperfection of the chosen approach: a transient DB error inside `GetActiveRun` leads to a `StartRun` attempt, but `StartRun` hits the same DB and returns its own error, so net behavior stays correct.

### How the TUI signals outcome back to the CLI (for exit messaging)
The final model exposes four mutually-relevant fields the `RunE` reads: `completed`, `abandoned`, `canceled` (q/Esc/Ctrl+C), and `err`. The launcher returns `(checklistRunModel, error)`; the outer `error` is only for `p.Run()` infra failures and the guarded type assertion (`result, ok := finalModel.(checklistRunModel)` per the bare-assertion pitfall). Service-call failures during `Update` land on `result.err`. `RunE` logic: return the infra error, then return `result.err`, otherwise branch — `abandoned` prints "✗ Abandoned %q" unless `--quiet`; `completed` prints nothing extra (the TUI owns the completion frame); `canceled` prints nothing (run left active).

- **`View()` on exit**: unlike `logMultilineModel.View()` (which blanks on every terminal state because the caller prints next), the completion message is TUI-owned per the AC's "final frame on exit" note. So `View()` returns the full checklist frame with a "✓ Complete!" footer when `completed` (it stays in scrollback), but returns `""` when `abandoned`/`canceled`/`err` (clean exit, CLI handles messaging). `View()` builds a `[]string` of non-empty lines and `strings.Join`s once, avoiding the "unconditional newline join" pitfall; the empty-checklist case renders a "(no items)" line rather than crashing.
- **Alternative considered — a dedicated outcome struct** (`checklistRunResult{completed, abandoned, err}`) instead of returning the model. Marginally cleaner API, but returning the model matches the codebase's same-package field-access style and keeps the launcher trivial.

### Where `AbandonRun` sits relative to `ResetRun`
Placed directly after `ResetRun` in `service.go` so readers compare the two "abandon" operations side by side. It is an **independent** method, not a refactor of `ResetRun`.

- **Signature: `(*Run, error)`, not `error`.** This deviates from the codebase-analysis suggestion of `error`-only, but is required by AC scenario "under `--quiet`, the abandoned run's ID is printed" — the CLI needs the run's ID. Returning the just-abandoned run serves the CLI (quiet prints `run.ID`) and keeps a single lookup (no TOCTOU double fetch). Error semantics mirror `GetActiveRun`: no active run → `"no active run for %q (use 'start' to begin)"`; missing template → the `GetTemplate` "not found" error; every wrapped with `%w`.
- **Alternative considered — extract a shared `abandonActiveRun(tpl)` helper used by both `ResetRun` and `AbandonRun`** for DRY. Rejected to keep the diff minimal and avoid any risk of regressing `ResetRun` (AC #14); the duplicated ~4-line block is cheap insurance.

### Whether the CLI abandon command and TUI 'a' key share one code path
Yes — both call `checklist.Service.AbandonRun`. The `rk cl abandon <template>` `RunE` calls `AbandonRun(args[0])`; the TUI's `a` handler calls `AbandonRun(m.run.TemplateID)`. Because `StartRun` enforces at most one active run per template, "the active run for this template" is exactly `m.run`, so both entry points resolve to and abandon the same run with identical error/success semantics (AC "single abandon code path", "abandon via CLI vs TUI must be behaviorally identical"). Passing `m.run.TemplateID` (resolved via `GetTemplate`'s name-then-ID fallback) is robust. If the run was completed/abandoned out-of-band before `a` is pressed, `AbandonRun` returns "no active run"; the TUI stores it in `m.err` and quits (concurrency reconciliation is explicitly out of scope).

## Test scenarios (mapped to files)

### `internal/checklist/service_test.go` (real temp SQLite fixture)
- **Abandon a fresh active run** → `TestAbandonRun`: create tpl, start, check one item, `AbandonRun` returns no error + run with `RunStatusAbandoned`; assert `ListRuns(false)` is empty and `GetActiveRun` now errors, and no new run was created.
- **Abandon an already-abandoned run** → `TestAbandonRunAlreadyAbandoned`: abandon, then abandon again → second call errors.
- **Abandon an already-completed run** → `TestAbandonRunCompleted`: check all items (auto-completes), `AbandonRun` errors and status stays `RunStatusCompleted`.
- **Abandon a nonexistent template** → `TestAbandonRunTemplateNotFound`: `AbandonRun("ghost")` → "not found" error.
- **ResetRun regression (active run / no active run)** → already covered by existing `TestResetRun` / `TestResetRunNoActive`; must keep passing unchanged (no new tests, verify no regression).

### `internal/cli/checklist_run_helper_test.go` (direct model.Update, real temp service)
- **Navigate down j/Down** → seed model (3 items, cursor 0), send `tea.KeyMsg` for `j` then for `Down`; assert cursor advances and clamps at last item.
- **Navigate up k/Up** → cursor 2, send `k` then `Up`; assert cursor decrements and clamps at 0.
- **Toggle unchecked item on** → cursor on unchecked, send space (and separately enter); assert `GetRunStatus` shows item checked, run still active.
- **Toggle checked item off** → checked item with others unchecked, toggle; assert item unchecked and persisted.
- **Auto-complete on last item** → all-but-one checked, toggle last → assert `result.completed == true`, service shows `RunStatusCompleted`, and the returned cmd is `tea.Quit`.
- **Abandon via `a`** → press `a` → `result.abandoned == true`, service shows `RunStatusAbandoned`, `ListRuns(false)` empty (no new run).
- **Quit via `q` / `Esc` / `Ctrl+C`** → three tests; each → `result.canceled == true`, service still shows `RunStatusActive` with item state unchanged, `abandoned == false`.
- **View states** → `completed` View contains the template name, checkbox marks, and "✓ Complete!" (non-empty, persists); `canceled`/`abandoned`/`err` View returns `""`; active View is non-empty and contains name + `[ ]`/`[x]` marks + hint line.
- **Empty checklist (0 items)** → model with a run whose `Items == []`: View renders "(no items)" and does not crash; space/enter and j/k are no-ops; only `a`/`q` exit. (Guards the `allChecked([]) == false` never-completes case.)

### `internal/cli/checklist_test.go` (set global `checklistService` to a temp service)
- **Abandon an active run via CLI** → seed active run, invoke `checklistAbandonCmd().RunE`; non-quiet asserts confirmation printed and run abandoned; quiet asserts stdout equals `run.ID`. (Requires capturing `os.Stdout` since commands use `fmt.Printf`, not `cmd.OutOrStdout`, note in Risks.)
- **Abandon with no active run** → command returns an error (exit non-zero).
- **Abandon a nonexistent template** → command returns "not found" error.
- **`resolveChecklistRun` — resume** → start a run, `resolveChecklistRun` → `resumed == true`, same run ID (covers AC #2, "run resumes active run", headlessly).
- **`resolveChecklistRun` — start fresh after abandon** → abandon, then `resolveChecklistRun` → `resumed == false`, brand-new active run, all items unchecked, different ID (covers "run after abandon starts fresh").
- **`resolveChecklistRun` — after completion** → complete a run, `resolveChecklistRun` → new active run (not an error).
- **`resolveChecklistRun` — missing template** → returns "not found" error before any TUI.

Note: `rk cl run` end-to-end cannot be exercised in tests because it launches `tea.NewProgram` (no headless mode, consistent with how `PickTask`/`runLogMultilineEditor` are not run in tests). Its behavior is covered by testing `resolveChecklistRun` + the model separately.

## Known risks or ambiguities

- **`AbandonRun` return type deviates from the ticket/analysis text.** The ticket says "`AbandonRun(nameOrID)`" and the analysis proposed `error`-only, but the AC's quiet-mode-prints-ID scenario forces returning the run. Plan uses `(*Run, error)`. Flag for reviewer sign-off; the alternative (CLI fetches active run for its ID before abandoning) is uglier and racier.
- **Bubble Tea final-frame rendering.** The completion-message-persists-in-scrollback behavior depends on the inline program rendering `View()` one last time on `tea.Quit`. This is the expected behavior but should be confirmed by running the binary during implementation (the `/verify` or `/run` step), since it's the one behavior not covered by headless model tests.
- **CLI stdout capture in tests.** Checklist commands print via `fmt.Printf`/`fmt.Println` to the process `os.Stdout` (see `checklistResetCmd`), not `cmd.OutOrStdout()`. Asserting quiet-mode ID output requires swapping `os.Stdout` in the test. If that's deemed too invasive, downgrade the CLI test to assert only error/no-error (exit behavior) and rely on service-layer coverage for the ID value.
- **Package-global `checklistService` in tests.** Setting the global from `checklist_test.go` works but introduces shared mutable state across tests in package `cli`; tests must set it in each test (or a helper) and not rely on ordering. This matches the acknowledged "package globals are an anti-pattern" note in `internal/cli/AGENTS.md`; no DI refactor is in scope here.
- **`resolveChecklistRun` swallows the `GetActiveRun` error.** It conflates "no active run", "template missing", and transient DB errors, relying on `StartRun` to re-surface the real error. Correct in practice (StartRun re-validates), but a reviewer preferring precise errors may push for the `Service.ResumeOrStartRun` alternative.
- **`a` abandon targets "the template's active run," not a captured run object.** Relies on the single-active-run-per-template invariant (enforced by `StartRun`). Correct today; if that invariant ever changes, the TUI's abandon could target a different run than displayed. Acceptable given current constraints and the out-of-scope concurrency note.
- **Keybinding coverage.** Ensure `ctrl+c` is special-cased in addition to `q`/`Esc` (all three → canceled, run stays active), matching `taskPickerModel`/`logMultilineModel`; `ctrl+c` must not be treated as abandon.

### Critical Files for Implementation
- internal/cli/checklist_run_helper.go (new — TUI model, `resolveChecklistRun`, launcher)
- internal/checklist/service.go (add `AbandonRun`)
- internal/cli/checklist.go (add `run` and `abandon` commands, register them)
- internal/cli/log.go (reference pattern for inline model + launcher + `View`)
- internal/checklist/service_test.go (reference fixture + add `AbandonRun` tests)
