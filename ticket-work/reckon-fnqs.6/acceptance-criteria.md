# Acceptance criteria — reckon-fnqs.6 (composable prompt layer)

No `codebase-analysis.md` exists yet for this ticket; grounding facts are inlined below rather than
referenced out. Design basis: `docs/design/composable-redesign.md:873-878` — TUI/CLI/MCP are peer
porcelains over one verb surface; the TTY guard exists because an agent invoking a porcelain must
never hang (876-879).

## 1. Explicit acceptance criteria

From `bd show reckon-fnqs.6` Scope + Done-when, numbered:

1. `components.Prompt[T]` interface: `Init() tea.Cmd`, `Update(tea.Msg) (…, tea.Cmd)`, `View() string`,
   `Result() T`. Exact `Update` return type is an open spelling question — see §2.1, not cosmetic.
2. Every one of the 5 named components — `Form`, `TextEditor`, `DatePicker`, `TaskPicker`,
   `NotePicker` (`internal/tui/components/{form,text_editor,date_picker,task_picker,note_picker}.go`)
   — implements `Prompt[T]` for its own result type.
3. One generic `RunPrompt[T any](p Prompt[T]) (T, bool, error)`-shaped host replacing the
   per-component helper models. **No such helper models currently exist to replace** — see §2.2.
4. `Wizard` chains ≥3 `Prompt`s into one flow: shared result map, ESC steps backward between steps.
5. TTY guard: `isatty` check + `--no-input` persistent flag. Non-tty stdin or `--no-input`, at the
   moment a verb would call `RunPrompt`/`Wizard`, returns an error naming `--no-input` and the
   flag-driven alternative — never reaches `tea.NewProgram`.
6. `TaskPicker`/`NotePicker` retargeted at index rows instead of domain types. **`TaskPicker` is
   partially done already** (landed under fnqs.8, not this ticket) — see §2.3. `NotePicker` is not
   retargeted at all: `Show(notes []*models.Note)`, `internal/tui/components/note_picker.go:174`.

## 2. Implicit requirements

| # | Requirement | Grounding |
|---|---|---|
| 2.1 | **`Update`'s return type must change on every one of the 5 components — a Go language constraint, not a style choice.** Go requires exact method-signature match for interface satisfaction (no covariant returns). Today each component's `Update` returns its own concrete pointer (`(*Form, tea.Cmd)`, `(*DatePicker, tea.Cmd)`, etc.); for any of them to satisfy `Prompt[T]`'s `Update` method, the declared return type must become the interface (`Prompt[T]` or embedded `tea.Model`), not the concrete pointer. This is unavoidable regardless of the interface's exact spelling. | `form.go:184`, `date_picker.go:92`, `text_editor.go:107`, `task_picker.go:220`, `note_picker.go:249` (current concrete-typed `Update` signatures) |
| 2.2 | **Nothing currently calls the deleted per-component hosts — there are no live callers to migrate.** The ticket's Problem section cites `task_picker_helper.go`/`note_picker_helper.go`/`checklist_run_helper.go` as today's hand-rolled hosts; all three lived in `internal/cli/`, not `internal/tui/components/`, and were deleted wholesale by commit `83ca4ef` ("Retire the v0 DB-primary verb surface (reckon-fnqs.1) (#158)"). Zero references to `PickTask`/`PickNote`/`PickOpenTask`/`runChecklistTUI` remain anywhere in the tree (verified by grep). `RunPrompt`/`Wizard` is new construction, not a replacement of live code. **Reusable precedent for `RunPrompt`'s own return shape**: the deleted `PickTask`/`PickNote` (`git show 83ca4ef^:internal/cli/task_picker_helper.go`) returned `(value, canceled bool, err error)` — a 3-tuple, cancellation as a distinct bool, not folded into `T` or `error`. Recommend `RunPrompt[T]` keep this exact shape. | commit `83ca4ef`; `git show 83ca4ef^:internal/cli/{task_picker_helper,note_picker_helper,checklist_run_helper}.go` |
| 2.3 | **"index rows (id/title/type/props)" names no existing struct.** The repo has no canonical `IndexRow{ID,Title,Type,Props}` type — every consumer defines its own bespoke row shape: `agendaItem` (`today.go:93-105`), `todoListItem` (`todo.go:156-168`), `LogEntryRow` (`log_view.go:32-37`). `TaskPicker` was already decoupled from `journal.Task` into a local `TaskRow{ID, Title, DateInfo}` (`task_picker.go:51-55`) as a side effect of fnqs.8 (commit `057f7b0`), *before* this ticket started — the ticket's own Problem-section claim ("`TaskPicker.Show()` takes `[]journal.Task`") is stale. `TaskRow` has no `Type`/`Props` fields though, so it's not yet the "id/title/type/props" shape either. **[OPEN]**: mint one new minimal `components.IndexRow` shared by both pickers, or keep each picker's own narrow row (extend `TaskRow`, add an equivalent for notes) — ticket text doesn't commit to either. |  |
| 2.4 | **`DatePicker.Update` never self-signals completion or cancellation today — must be fixed for a generic host to work at all.** On `KeyEnter` with a valid date it returns `(dp, nil)` with no cmd/msg (`date_picker.go:103-119`, comment "Valid date - will be handled by parent" at line 118); the *current* caller (`tui_keyboard.go`'s `handleDateSubFlowKey`) intercepts `KeyEnter` itself and calls `m.datePicker.ParsedDate()` directly, never routing it through `dp.Update` at all. On `KeyEsc` it calls `Hide()` with no cancel signal (`date_picker.go:100-102`), unlike the other 4 components, which all emit an explicit `XCancelMsg` via `tea.Cmd` on Esc (`form.go:192-196`, `text_editor.go:115-119`, `task_picker.go:228-232`, `note_picker.go:257-261`). A generic `RunPrompt[T]` has no per-component special-casing, so `DatePicker.Update` must be changed to self-detect both cases the way the other 4 already do. | `date_picker.go:92-131` |
| 2.5 | **A later wizard step can't see an earlier step's result via `Init()`'s arguments — `tea.Model`-shaped `Init()` takes none, by convention every existing component already follows** (`Show(...)` primes state, not `Init()`). Resolution: `Wizard` steps must be registered as factories (`func(prior map[string]any) Prompt[T]`), constructed/configured from the shared result map immediately before mounting, not as pre-built `Prompt` values. This is close to forced by `tea.Model`'s fixed signature, not a style choice. | — |
| 2.6 | **TTY-check needs a testability seam** — `isatty.IsTerminal` reads a real fd; no way to fake a terminal in a unit test. Existing precedent for exactly this shape of seam: `var todoNow = func() time.Time { return time.Now().UTC() }` (`todo.go:33`), an overridable package-level func var. The guard should follow the same pattern (e.g. `var isInteractive = func() bool {...}`), not a bare inline call. `github.com/mattn/go-isatty` is already in `go.mod:34` (indirect, pulled transitively via bubbletea/termenv) — promote to direct once imported. | `todo.go:33`; `go.mod:34` |
| 2.7 | **Guard placement inside `RunPrompt`/`Wizard.Run` itself (not a blanket pre-flight check) is what makes "no-op when the verb doesn't prompt" true for free.** `root.go`'s `PersistentPreRunE` (`root.go:81-88`) runs for every command regardless of whether it prompts; gating there would need a per-command "does this verb prompt" marker, and no such mechanism exists — `tui.go:20`'s comment mentioning a "`requiresDB` annotation" is aspirational prose with no actual `cobra.Command.Annotations` usage anywhere in the tree (confirmed by grep). Putting the check inside `RunPrompt`/`Wizard` means it only fires at the moment a prompt would actually be shown, and automatically covers every future caller (fnqs.7) without each one remembering to add it. | `root.go:81-97`; `tui.go:20` |
| 2.8 | **"Usage error" has no distinct exit code in this codebase today.** `root.go:20` defines `ExitCodeUsageErr = 2` but it is dead code — zero references outside its own declaration. `cmd/rk/main.go:9-12` exits 1 on any non-nil error from `cli.Execute()`, full stop; there is no exit-code differentiation mechanism today. The ticket's "usage error naming the flags to pass" is satisfiable by returning a plain error whose *message* names `--no-input`; wiring a real exit-code-2 path through `main.go` is a larger, separate change than this ticket's literal Done-when requires. **[OPEN, not required]**: nice-to-have if cheap, not blocking. | `root.go:17-22`; `cmd/rk/main.go:9-12` |
| 2.9 | `checklist_run_helper.go`'s pattern (`internal/checklist` package) is not one of the 5 named components, and the package has zero current CLI callers (confirmed by grep — no command wires to `internal/checklist` post v0-cut). Its mention in the ticket's Problem section is motivating context only, not a retrofit target. | — |

### 2.1 continued — proposed `Prompt[T]` spelling (not settled) and which option actually triggers the ripple

```go
type Prompt[T any] interface {
    Init() tea.Cmd
    Update(tea.Msg) (Prompt[T], tea.Cmd) // or: tea.Model, if Prompt embeds it instead
    View() string
    Result() T
}
```

The signature-change ripple in §2.1 is real **only if a component's own concrete `Update` method is
made to satisfy this interface directly (option a below)**. It is not forced by the interface's mere
existence — an adapter path (option b) sidesteps it entirely. Two ways to resolve, **[OPEN]** for
Phase 3:

- (a) Change each component's own `Update` signature outright to return the interface type. Fixes
  needed at the 2 *production* call sites that reassign into a concrete-typed field —
  `tui_keyboard.go:184` (`m.notes.picker, cmd = m.notes.picker.Update(msg)`) and `tui_keyboard.go:292`
  (`m.datePicker, cmd = m.datePicker.Update(msg)`) — via a type assertion back to the concrete type
  (mirrors bubbletea's own top-level `Update` idiom). **Also touches each component's own tests** that
  do the same reassignment pattern (e.g. `date_picker_test.go`, `form_test.go`), not just the 2
  production sites — a wider blast radius than it first looks.
- (b) Leave each component's existing concrete-typed `Update` untouched (so `tui_keyboard.go:184`/`292`
  and all component tests are unaffected), and satisfy `Prompt[T]` with a small wrapper/adapter type
  per component, constructed only where `RunPrompt`/`Wizard` needs the interface value. This is the
  only clean sub-variant — a "differently-named method" doesn't work here, because the interface's
  method is literally named `Update` (ticket text: "Init/Update/View"), so nothing but an `Update`
  method can satisfy it; a same-component second method with a different name is not an option.

## 3. Edge cases

| Case | Expected behavior | Grounding |
|---|---|---|
| ESC at wizard step 0 (the first step) | Abort the whole flow — ticket's own stated convention, and consistent with every component's existing "ESC always cancels" behavior (once DatePicker is fixed per §2.4). | ticket text; §2.4 |
| ESC at wizard step >0 | Step back to the previous step; that step's (and every earlier step's) previously-entered result stays in the shared result map — not cleared. | ticket text ("threading a shared result map, ESC-back between steps") |
| `--no-input` passed to a verb that never calls `RunPrompt`/`Wizard` | No-op, no error — falls out of guard placement inside `RunPrompt` itself (§2.7), not a blanket flag check. | §2.7 |
| Piped/non-tty stdin on a verb that would prompt | Errors before ever calling `tea.NewProgram`; message names `--no-input`. No verb calls `RunPrompt` yet (fnqs.7 wires the first ones), so the verb-specific "flag-driven alternative" text is that ticket's job, not this one's — this ticket only needs the guard mechanism and *a* correctly-shaped message. | §5 |
| Prompt aborted mid-flight (ESC) | `Result()` is never called by `RunPrompt` in this path — cancellation is detected via each component's own `XCancelMsg` (mirroring the deleted `PickTask`/`PickNote` precedent, §2.2). `RunPrompt[T]` returns `(zero-value T, false, nil)`. `Result() T` itself carries no cancellation signal — it's only meaningful once `RunPrompt` already knows `Update` signaled successful submission. | §2.2 |
| Later wizard step needs an earlier step's result at construction time | Resolved via factory-closure injection (§2.5), not via `Init()` arguments. | §2.5 |
| Wizard with <3 steps | Not required to be prevented or special-cased — Done-when only requires demonstrating ≥3 chained; nothing bars a 1- or 2-step wizard from working the same way. | ticket text |

## 4. Test scenarios (given/when/then)

**Prompt interface**

1. Given each of `Form`/`TextEditor`/`DatePicker`/`TaskPicker`/`NotePicker`, when checked against
   `Prompt[T]` for its own result type, then each satisfies the interface at compile time (a
   `var _ Prompt[FormResult] = (*Form)(nil)`-style assertion compiles per component).
2. Given `DatePicker` after the §2.4 fix, when `Update` receives `KeyEnter` with a valid date typed,
   then it self-signals submission (returns a cmd/msg the host can detect) without the caller having
   pre-intercepted the key.
3. Given `DatePicker` after the §2.4 fix, when `Update` receives `KeyEsc`, then it self-signals
   cancellation the same way `Form`/`TextEditor`/`TaskPicker`/`NotePicker` already do.

**`RunPrompt` host**

Done-when's "every component is reachable standalone through one host" is checked in two tiers, not
left ambiguous: scenario 1 (compile-time `Prompt[T]` conformance, all 5) is the operative check for
"every component" — a generic host takes the same code path regardless of which `Prompt[T]` it's
given, so per-component runtime coverage isn't required to be exhaustive. Scenarios 4-6 below are
representative runtime spot-checks (2 of the 5), not full per-component runtime coverage; if Phase 3
wants stronger proof, add one runtime scenario per remaining component (`TextEditor`, `TaskPicker`,
`NotePicker`) in the same shape as 4/5.

4. Given a `Form` primed with all-required fields filled validly, when `RunPrompt` drives it and the
   user presses Enter, then `RunPrompt` returns `(result matching Form.GetValues(), true, nil)`.
5. Given any one `Prompt` (e.g. `DatePicker` post-fix), when the user presses Esc, then `RunPrompt`
   returns `(zero value, false, nil)` and `Result()` is never invoked.
6. Given the package hosting `RunPrompt`, when grepped for `tea.NewProgram`, then exactly one call
   site exists driving all 5 components — not one hand-rolled host model per component (the pattern
   this ticket replaces, per §2.2).

**Wizard**

7. Given a `Wizard` chaining `DatePicker` → `Form` → `TextEditor` (3 distinct prompts), when the user
   completes all 3 in order, then the Wizard's final shared result map holds all 3 steps' results
   under distinct keys and the Wizard reports overall success.
8. Given a `Wizard` on step 2 of 3 with step 1 already completed, when the user presses Esc, then the
   Wizard returns to step 1 with step 1's previously-entered value still present/re-shown, and the
   shared result map is unchanged.
9. Given a `Wizard` on step 0 (the first step), when the user presses Esc, then the whole flow
   aborts — the Wizard-level run reports not-ok, and no step's result is committed anywhere.
10. Given a `Wizard` where step 2 is registered as a factory closure over the shared result map, when
    step 1 completes and step 2 mounts, then step 2's initial state reflects step 1's just-submitted
    result (e.g. a pre-filled field).

**TTY guard**

11. Given stdin is not a TTY (the §2.6 seam stubbed to return false), when a verb reaches its
    `RunPrompt`/`Wizard` call, then it returns an error mentioning `--no-input` and `tea.NewProgram` is
    never invoked (assert via the seam call count / no blocking).
12. Given stdin IS a TTY but `--no-input` is set, when a verb reaches its `RunPrompt`/`Wizard` call,
    then the same usage error fires — the flag beats TTY detection.
13. Given `--no-input` is passed to a verb that never calls `RunPrompt`/`Wizard`, when the command
    runs, then it completes normally with no error attributable to the flag.

**Picker retargeting**

14. Given `NotePicker.Show` called with the new non-`*models.Note` row type (§2.3's chosen shape),
    when rendered and a row selected, then title display and selection behave identically to today
    for the fields that survive (title, an id-equivalent).
15. Given `TaskPicker`'s existing `TaskRow`, when compared against whatever shape §2.3 settles on for
    `NotePicker`, then confirm both pickers converge on one shared row type (or explicitly document why
    they don't) — Done-when says "the pickers" plural, not one.

## 5. Out of scope

| Item | Owner ticket | Note |
|---|---|---|
| Wiring `rk todo add` / `rk note create` / `rk add` to actually call `RunPrompt`/`Wizard` on a bare TTY invocation | reckon-fnqs.7 | This ticket ships the interface/host/guard only; no verb calls them yet, so the guard's error message can't yet name a verb-specific "flag-driven alternative" beyond `--no-input` itself. |
| Composable board / pluggable pane registry, panes calling the Wizard host | reckon-fnqs.10 | Depends on this ticket; not this ticket's job. |
| New mini-TUI components beyond retargeting the existing 5 | — | Not commissioned by this ticket's Scope. |
| `TextEntryBar` (`components/text_entry_bar.go`) | — | Live today (`tui.go:69`, embedded via `tui_keyboard.go`), structurally the closest existing analog, but **not** named among the ticket's 5 components — leave unconverted. |
| `internal/checklist` package / any checklist-run TUI | — | Orphaned, 0 CLI callers today (§2.9) — not a retrofit target despite being cited in the ticket's Problem section. |
| Wiring `ExitCodeUsageErr` through `cmd/rk/main.go` for a real usage-specific process exit code | — | Optional; not required by Done-when (§2.8). |
| Rebuilding `rk tui`'s existing hand-rolled sub-flow state machine (`tui_keyboard.go`'s `inputModeSubFlow`/`tuiSubFlowKind`, agenda `d`/`D`/`p` capture, pane `n` creation flows) on top of Prompt/Wizard | reckon-fnqs.10 | fnqs.8 shipped this deliberately *not* routed through Prompt/Wizard (its own ticket text says so); migrating it is fnqs.10's "single shared implementation" goal, not this one's. |
