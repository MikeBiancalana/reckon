# Plan: reckon-fnqs.12 — `rk checklist run` interactive mini-TUI

## Summary of approach

Add a TUI-only `rk checklist run <template>` verb that, on a TTY, opens a single-screen bubbletea program hosted by fnqs.6's `components.RunPrompt` — the **first live caller of `RunPrompt` anywhere in `internal/cli`** (confirmed: `internal/cli/tui.go:54` and `migrate_legacy.go:217` are unrelated `.Run()` calls; zero `RunPrompt` callers today).

Two new units plus one small refactor:

1. **`internal/tui/components/checklist_runner.go`** — a `ChecklistRunner` implementing `Prompt[[]ChecklistItem]`. It holds only a display slice (`[]ChecklistItem{Text, Checked}`), a `cursor int`, terminal flags, and an injected `ToggleFunc` callback. It **never imports `internal/checklist`** (the hard constraint). Per-toggle it calls the callback and re-renders from its return; on auto-completion it flags finished and the host quits.
2. **`internal/cli/checklist_run.go`** — the cobra `run` verb. It owns the `checklist.Service`↔`ChecklistItem` mapping in both directions: a `runItemsToChecklistItems` converter (mirrors `notesToRows`, `tui_model.go:118-140`) and the `ToggleFunc` closure that captures `*checklist.Service` + `runID` and calls `CheckItem`→`GetRunStatus` (the exact pair `runChecklistCheckE` uses, `checklist.go:463-472`).
3. **Refactor** the resume-or-start resolution out of `runChecklistStartE` (`checklist.go:421-436`) into a shared `resolveChecklistRun` both verbs call.

The `--no-input`/non-TTY guard comes for free: `RunPrompt` invokes `components.PromptGuard` (wired once at `interactive.go:39-41`) before opening any `tea.Program` (`prompt.go:79-84`). No bespoke isatty check in `checklist.go`.

## Files to modify / create

| File | Action | Reason |
|---|---|---|
| `internal/tui/components/checklist_runner.go` | **create** | `ChecklistItem` type, `ToggleFunc` type, `ChecklistRunner` component implementing `Prompt[[]ChecklistItem]`. Mirrors `task_picker.go` structure (constructor + `Show` prime + `Init/Update/View/Result/Done`). |
| `internal/tui/components/checklist_runner_test.go` | **create** | Behavioral tests driving the runner through `RunPrompt` with scripted keystrokes + a fake `ToggleFunc` (mirrors `prompt_test.go`'s `runPromptForTest` + `WithInput/WithOutput` seam). |
| `internal/cli/checklist_run.go` | **create** | `checklistRunCmd`, `runChecklistRunE`, `runItemsToChecklistItems` converter, `makeToggleFunc` closure factory. Kept in a sibling file (not appended to the 547-line `checklist.go`) for cohesion; AC §1.11 explicitly permits `checklist_run.go`. |
| `internal/cli/checklist.go` | **modify** | (a) Add `checklistRunCmd` to the `init()` `AddCommand` block (`checklist.go:125-133`). (b) Extract `resolveChecklistRun` and call it from `runChecklistStartE`. |
| `internal/cli/checklist_test.go` | **modify** (add funcs) | CLI-layer tests: guard paths + template-not-found via `RootCmd.Execute`; `resolveChecklistRun` resume-vs-fresh; the `ToggleFunc` closure persisting via a real `Service`+temp DB. Reuses `setupChecklistEnv`/`runChecklist` helpers already in the file. |

The `run` verb registers with `Use: "run <template>"`, `Args: cobra.ExactArgs(1)`, `SilenceUsage: true`, matching the sibling verbs. It takes **no extra flags** (see decision 3).

## Design decisions

**1. Persistence timing — per-toggle live via injected callback (accepted).**
The component calls `ToggleFunc` synchronously in `Update` on space/enter; the CLI closure runs `CheckItem`→`GetRunStatus` and returns the refreshed `[]ChecklistItem` + a `completed` bool. This is the only shape that (a) reads "the same Service calls fnqs.11 wires" literally, (b) detects auto-completion live without the component re-deriving `allChecked` (which would require importing/duplicating domain logic), and (c) is crash-safe (a `kill -9` mid-session loses nothing already toggled).
*Alternative rejected:* batch-collect-then-submit (component mutates local state, CLI diffs at `Result()`). Matches the other five pure components more closely, but loses toggles on a mid-session crash and cannot auto-detect completion without a client-side `allChecked` — reintroducing exactly the domain coupling the ticket forbids.
*Note:* synchronous call (not a deferred `tea.Cmd` closure) sidesteps the closure-capture staleness pitfall (`REVIEW_PATTERNS.md:117-135`) entirely — there is no closure reading mutable model fields at call time; the DB is local SQLite, so the call is fast.

**2. Auto-complete-and-exit (accepted).**
When `ToggleFunc` returns `completed=true`, the runner sets an internal `completed` flag; `Done()` reports `finished=true`; the host (`prompt.go:58-64`) returns `tea.Quit`. The final `View()` frame renders a `✓ Complete!` banner (with a trailing newline so bubbletea's non-alt-screen renderer doesn't erase it on exit — the v0 trick). Matches deleted v0 continuity.
*Alternative rejected:* stay open with a completion banner until manual quit. AC §2 leaves this `[OPEN]`; auto-exit wins for v0 continuity and because it removes the "uncheck an already-completed item" reachable-state quirk (AC §3) from the TUI.

**3. No extra flags on `run` (accepted).**
`start`/`check`/`status`/`reset`/`abandon` (fnqs.11, shipped) already cover every non-interactive need; `run` is TUI-only. Non-TTY/`--no-input` errors via the shared guard — refusal, not a flag-driven fallback.
*Alternative rejected:* a `--check <pos>` non-interactive mode on `run`. No precedent, and it duplicates `checklist check`.

**4. In-TUI abandon keybinding — out of scope (accepted).**
`rk checklist abandon` is a separate shipped verb; fnqs.12's scope lists only move/select/toggle/see-completion. The v0 `a` binding is not resurrected.

**5. Guard ordering — resolve-first (decided; wart documented).**
`runChecklistRunE` resolves the run (`resolveChecklistRun`) *before* calling `RunPrompt`; the guard fires inside `RunPrompt`. This gives structural parity with all seven sibling verbs (setup → resolve → act).
*Known wart:* in the narrow case of **non-TTY + no active run**, `StartRun` writes a fresh run before the guard errors. This is functionally benign and self-healing: a just-created all-unchecked active run is state-identical to a later fresh start (only `StartedAt` differs), and the next proper TTY invocation resumes it indistinguishably. The resume path (active run exists) has zero side effect.
*Alternative (legitimate, not a criteria violation):* call the shared `components.PromptGuard()` once at the top of `RunE` before resolving — zero side effect. This is **not** the "second checklist-specific isatty check" AC §4 forbids (that means a hand-rolled `isCharDevice(os.Stdin)` in `checklist.go`); it is the same shared hook invoked early. Rejected only for structural parity and single-guard-site cleanliness; the implementer may swap to guard-first if the phantom-run wart is judged unacceptable in review.

## `ChecklistItem` shape and the `Prompt[T]` parameter

```go
// internal/tui/components/checklist_runner.go
type ChecklistItem struct {
    Text    string
    Checked bool
}
type ToggleFunc func(position int) (items []ChecklistItem, completed bool, err error)
```
Position is implicit (slice index) — safe because `GetRunItems` sorts `ORDER BY position ASC` and `NewRun` assigns `Position: i` (`model.go:71-78`), so `Items[i].Position == i` for any Service-fetched run; the cursor index maps 1:1 to `CheckItem`'s `position` with no translation.

**`T = []ChecklistItem`.** `Done()`'s `finished` fires only on auto-completion, so `Result()` returns the all-checked slice exactly when it is meaningful; on user quit `ok=false` and `Result()` is never read. Cost is nil, it is symmetric with the other components returning their collected value, satisfies the "`Result` returns the committed value" contract, and lets a component test assert `Result()` on completion.
*Alternative rejected:* `T = struct{}` / `bool` — makes `Result()` a dead method for no benefit; the CLI ignoring the value in production does not justify degrading the contract.

## `ChecklistRunner` signatures

| Member | Signature | Behavior |
|---|---|---|
| `NewChecklistRunner` | `func(title string) *ChecklistRunner` | Sets header title (the template name); no run state yet. |
| `Show` | `func(items []ChecklistItem, onToggle ToggleFunc)` | Prime/reset point (mirrors `TaskPicker.Show`, `task_picker.go:163`): sets `items`, `cursor=0`, `onToggle`, clears `completed/canceled/err`. |
| `Init` | `func() tea.Cmd` → `nil` | Priming already done in `Show`. |
| `Update` | `func(tea.Msg) (Prompt[[]ChecklistItem], tea.Cmd)` | See keybindings below. |
| `View` | `func() string` | `[]string`-join render (see below). |
| `Result` | `func() []ChecklistItem` | Returns current `items`. |
| `Done` | `func() (finished, canceled bool)` | `finished = m.completed`; `canceled = m.canceled || m.err != nil`. |
| `Err` | `func() error` | **Not part of `Prompt[T]`.** Exposes a mid-session `ToggleFunc` error; the CLI holds the concrete pointer and reads it after `RunPrompt` returns. |

Conformance asserted at package scope: `var _ Prompt[[]ChecklistItem] = (*ChecklistRunner)(nil)` (mirrors `prompt_test.go:20-26`).

## Toggle-callback wiring (CLI layer)

```go
// internal/cli/checklist_run.go
func runItemsToChecklistItems(items []checklist.RunItem) []components.ChecklistItem { /* value-copy Text/Checked */ }

func makeToggleFunc(svc *checklist.Service, runID string) components.ToggleFunc {
    return func(position int) ([]components.ChecklistItem, bool, error) {
        if err := svc.CheckItem(runID, position); err != nil {           // service.go:147
            return nil, false, err
        }
        updated, err := svc.GetRunStatus(runID)                          // re-fetch by ID, not GetActiveRun
        if err != nil {                                                  // (auto-complete makes GetActiveRun error)
            return nil, false, err
        }
        return runItemsToChecklistItems(updated.Items),
            updated.Status == checklist.RunStatusCompleted, nil
    }
}
```
The closure captures `svc` and the **immutable** `runID` string (not the mutable `*Run`) — no staleness. It replays `runChecklistCheckE`'s exact `CheckItem`→`GetRunStatus` sequence (`checklist.go:463-472`), so both verbs write through one path (no parallel persistence).

### `runChecklistRunE` skeleton

```go
func runChecklistRunE(cmd *cobra.Command, args []string) error {
    defer resetChecklistFlags(cmd)
    name := args[0]
    _, svc, db, err := setupChecklistRun()                 // checklist.go:158
    if err != nil { return err }
    defer db.Close()

    run, _, err := resolveChecklistRun(svc, name)          // GetTemplate → GetActiveRun else StartRun
    if err != nil { return fmt.Errorf("checklist run: %w", err) }   // template-not-found surfaces here, before any Program

    runner := components.NewChecklistRunner(run.TemplateName)
    runner.Show(runItemsToChecklistItems(run.Items), makeToggleFunc(svc, run.ID))

    _, _, err = components.RunPrompt[[]components.ChecklistItem](runner)   // guard fires here for non-tty/--no-input
    if err != nil { return err }                            // guard/Program error → surface
    if e := runner.Err(); e != nil {                        // mid-session CheckItem error
        return fmt.Errorf("checklist run: %w", e)
    }
    return nil                                              // ok=false (user quit) is NOT an error — print nothing
}
```

`resolveChecklistRun` (extracted from `checklist.go:421-436`) returns **raw** errors so each caller keeps its own prefix — `runChecklistStartE`'s existing `checklist start: %w` messages (asserted by its tests) are unchanged:
```go
func resolveChecklistRun(svc *checklist.Service, name string) (run *checklist.Run, resumed bool, err error) {
    if _, err := svc.GetTemplate(name); err != nil { return nil, false, err }
    run, err = svc.GetActiveRun(name)
    if err != nil {
        if run, err = svc.StartRun(name); err != nil { return nil, false, err }
        return run, false, nil
    }
    return run, true, nil
}
```

**Critical for the no-error-on-quit contract:** in raw mode bubbletea delivers Ctrl+C as `tea.KeyCtrlC`, not SIGINT. `Program.Run()` returns `nil` *only because* the component's `Update` handles Ctrl+C and sets `canceled` (the host then quits cleanly). If Ctrl+C fell through to a default kill path, `Run()` would return `ErrProgramKilled`, `RunPrompt` would propagate it, and `if err != nil { return err }` would print an error on a user quit — violating AC. So all three quit keys are handled explicitly in `Update` (below).

## Keybindings, clamping, empty-checklist, quit

`Update` (`tea.KeyMsg`), all guarded by `len(items)`:
- **`q` / `esc` / `ctrl+c`** → `m.canceled = true`, return `(m, nil)`. Host quits via `Done()`; `ok=false`; CLI prints nothing. (Ctrl+C handled here per the note above.)
- **`j` / `down`** → `if len(items) > 0 { m.cursor = min(m.cursor+1, len(items)-1) }`. No wraparound.
- **`k` / `up`** → `if len(items) > 0 { m.cursor = max(m.cursor-1, 0) }`.
- **`space` / `enter`** → if `len(items) == 0`, no-op (no Service call). Else capture `pos := m.cursor` into a local, call `items, completed, err := m.onToggle(pos)`; on `err` set `m.err`; else `m.items = items` and `m.completed = completed`. Return `(m, nil)` — the host quits on `completed`/`err` via `Done()` (no need to return `tea.Quit`, matching how the host drives `TaskPicker`).
- Any other key → no-op.

**Empty checklist** (`len(items)==0`): reachable only defensively — `create` requires ≥1 item, and `allChecked([])` returns `false` (`service.go:246-248`) so a 0-item run never auto-completes and stays resumable. `View` renders a `(no items)` line; navigation/toggle are no-ops; quit exits cleanly.

**View** builds a `[]string` and `strings.Join(lines, "\n")` to avoid the unconditional-newline-join phantom-blank-line pitfall (`REVIEW_PATTERNS.md:879-901`):
- Header: `TemplateName  [checked/total]` (progress derived from `items`).
- One line per item: cursor prefix (`> ` selected / `  `) + `[x]`/`[ ]` + text.
- Footer: **either** the completion banner `✓ Complete!\n` (when `m.completed`) **or** an error line (when `m.err != nil`) **or** the help line `j/k: move  space/enter: toggle  q: quit` — appended conditionally, never blank.

Terminal resize and long-text wrapping: no special handling (no `WindowSizeMsg` case, no `SetWidth`), matching v0's inline non-alt-screen program; natural terminal reflow applies (AC §3, accepted).

## Test scenarios (organized by file)

### `internal/tui/components/checklist_runner_test.go` — behavioral, via `RunPrompt` + fake `ToggleFunc`
The fake `ToggleFunc` records the position it was called with and mutates an in-memory `[]ChecklistItem`, so cursor→position wiring is asserted by *which* item flips.
- Conformance: `var _ Prompt[[]ChecklistItem] = (*ChecklistRunner)(nil)` compiles.
- Fresh run, 3 unchecked items, cursor at 0 (drive one navigation, assert toggled index).
- **Cursor clamp down:** `j j j` then `space` on 3 items → position **2** toggled (no wraparound).
- **Cursor clamp up:** `k k` then `space` → position **0** toggled.
- **Toggle on:** `space` on an unchecked item → fake records that position, item shows checked in `View()`.
- **Toggle off:** pre-checked item + `space` → position toggled back to unchecked.
- **Auto-complete:** last unchecked item + `space` where fake returns `completed=true` → `RunPrompt` returns `ok=true`, `Result()` is all-checked; `View()` contained `✓ Complete!` before quit.
- **Quit keys (3 cases):** `q`, `Esc` (`0x1b`), `Ctrl+C` (`0x03`) each → `ok=false`, no error, fake never called after quit. (Ctrl+C case specifically proves `Run()` returns `nil`.)
- **Empty checklist:** 0 items → `j`/`k`/`space` are no-ops (fake never called), quit exits cleanly, `View()` shows `(no items)`.
- **Mid-session error:** fake returns an error → `RunPrompt` returns `ok=false` with `err==nil`, and `runner.Err()` is the fake's error.

### `internal/cli/checklist_test.go` (add funcs) — no `tea.Program` driven
- **`resolveChecklistRun` fresh vs resume:** temp DB + template; first call → `resumed=false`, new active run; second call → `resumed=true`, same run ID (proves both entry points share one resolution path, AC §4).
- **`makeToggleFunc` persists live:** real `Service` + temp DB; call the closure with a position → assert `GetRunStatus` reports that item checked immediately, and `CheckedAt` set; toggling again clears it. Covers AC's "Space → Service reports it checked immediately" without driving a Program.
- **`makeToggleFunc` completion:** check the last remaining item via the closure → returns `completed=true` and `GetRunStatus` reports `RunStatusCompleted`.
- **`runItemsToChecklistItems`:** pure unit test (Text/Checked mapping, order preserved).
- **Guard — non-TTY** (`RootCmd.Execute`, no `isInteractive` stub → false in test env): `run <template>` returns a non-zero error naming `--no-input`/interactive terminal, no TUI escape sequences in captured stdout. This CLI test *is* the `RunE→RunPrompt` wiring proof (guard returns before any Program).
- **Guard — `--no-input`** on a stubbed `isInteractive=true`: still errors (flag wins).
- **Template-not-found:** `run nonexistent` → same `checklist template %q not found` message the siblings produce, no TUI drawn (resolution fails before `RunPrompt`).
- **fnqs.11 regression parity:** existing `start`/`check`/`status` tests continue to pass after the `resolveChecklistRun` extraction (run to confirm no message/behavior drift).

**No test-only program-options global.** The above covers every AC given/when/then without stubbing `isInteractive=true` to drive a real `tea.Program` inside `RootCmd.Execute` (fragile, and needs a `[]tea.ProgramOption` package global that exists only for tests). If a single true end-to-end is later wanted, add that seam and flag it as a test-only global — but it is not required for coverage.

## Known risks / ambiguities remaining

- **Completion-banner scrollback survival is manual-verify, not unit-tested.** It depends on bubbletea's final-frame render plus the trailing-newline trick; component tests use `WithOutput(io.Discard)` and assert `ok=true`/`Result()`, not raw escape output. Flag for the implementer to eyeball once in a real terminal.
- **Phantom-run wart** (decision 5): non-TTY + no active run creates a run before the guard errors. Benign/self-healing; the guard-first-via-shared-hook alternative eliminates it if review objects.
- **Concurrent modification** (another `rk checklist check` mutates the run while the TUI is open): the TUI's in-memory slice goes stale until its next toggle round-trips through `GetRunStatus`. Not handled by v0 either; out of scope, noted as a known limitation.
- **`--json`/`--ndjson` don't gate the guard** (`promptGuard` only checks `noInputFlag || !isInteractive()`): `run <t> --json` on a real TTY still opens the TUI. Consistent with the shared guard's behavior for all future wizard verbs; not a bug to fix here. `run` ignores output-mode flags entirely (its "output" is the TUI).

## Critical Files for Implementation
- `internal/tui/components/checklist_runner.go` (new — `ChecklistItem`, `ToggleFunc`, `ChecklistRunner`)
- `internal/cli/checklist_run.go` (new — `run` verb, converter, `makeToggleFunc`)
- `internal/cli/checklist.go` (modify — register cmd, extract `resolveChecklistRun`)
- `internal/tui/components/prompt.go` (reference — `Prompt[T]`/`RunPrompt`/`PromptGuard` contract)
- `internal/tui/components/task_picker.go` (reference — component structure to mirror)
