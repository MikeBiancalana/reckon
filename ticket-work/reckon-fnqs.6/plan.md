# Implementation Plan: reckon-fnqs.6 — Composable prompt layer (Prompt interface + RunPrompt host + Wizard + TTY guard)

## Summary of approach

Build a composable prompt layer in `internal/tui/components/`: a generic `Prompt[T]` interface, a single generic `RunPrompt[T]` host that owns the only `tea.NewProgram` call, a `Wizard` that chains heterogeneous prompts, and a TTY guard wired from `internal/cli/`. This is greenfield host-building — the three helper files the ticket names were deleted in `83ca4ef` with zero live callers (codebase-analysis.md:44-52), so there is nothing to migrate, only new construction plus minimal additive changes to the 5 components.

**Fork A → resolved: option (a), each component's own `Update` satisfies `Prompt[T]` directly.** Decisive reason: acceptance-criteria.md §4 scenario 1 requires `var _ Prompt[FormResult] = (*Form)(nil)` to compile *per component* — that holds verbatim only under (a); option (b) makes an adapter satisfy `Prompt`, forcing scenario 1 to be rewritten. A written acceptance test satisfied as-authored by one option and requiring a rewrite of the other is the tie-breaker. Secondary: (a) lets each component set its completion flag synchronously at submit/cancel decision time, versus (b) catching the component's own emitted msg on a tea round-trip. This changes each component's `Update` return type from its concrete pointer to `Prompt[T]` — the ripple lands on exactly 2 production call sites plus component tests (enumerated below). This deliberately touches `Update`, a method `rk tui` calls directly, which the integration-risk note (codebase-analysis.md:289-299) warns against; the note's real concern is *orphaning* `rk tui`, not the mechanical touch — callers are updated in lockstep so `rk tui` keeps working, and Fork A option (a) explicitly sanctions this ripple.

**Fork B → resolved: retarget BOTH pickers onto one shared `components.IndexRow{ID, Title, Type string; Props map[string]string}`.** Decisive reasons: scenario 14 tests `NotePicker.Show` *with the new type*, and scenario 15 requires the two pickers to converge on one row type (or document why not). TaskPicker's `TaskRow` has zero live callers (task_picker.go:46-50 comment: "wired-but-unused"), so its migration is ripple-free. NotePicker's `Show([]*models.Note)` (note_picker.go:174) has 2 live callers (tui_keyboard.go:192, tui_model.go:174); those get an inline `models.Note → IndexRow` map that preserves title + slug. Scenario 14 explicitly excuses tag/created fidelity ("fields that survive: title, an id-equivalent"), which is the cover to fold notes' extra fields into `Props` without parity. This removes the last legacy-shaped domain type from the picker layer — the ticket's core intent.

**DatePicker submit signal (required infra, §2.4).** DatePicker emits nothing today: on valid Enter it returns `(dp, nil)` (date_picker.go:118-119). Add `DatePickerSubmitMsg{Date time.Time}` and `DatePickerCancelMsg{}`, mirroring the other 4 components' Esc-cancel pattern. In `Update`: Esc → `Hide()` + emit `DatePickerCancelMsg`; valid Enter → store parsed date, set `submitted`, `Hide()`, emit `DatePickerSubmitMsg`; empty/invalid Enter → set error, no msg (unchanged). Safety: the live caller `handleDateSubFlowKey` (tui_keyboard.go:273-294) intercepts Esc and Enter *itself* and only delegates non-Esc/Enter keys to `dp.Update` (line 291-292), so the new Esc/Enter emissions are dead code for `rk tui` — zero behavior change to the one working porcelain.

**Guardrail — additive, never replacement.** `Done()`, the completion flags, and `Result() T` are added *alongside* every existing `*SubmitMsg`/`*CancelMsg` emission and every existing getter (`GetValues`, `GetText`, `GetSelectedTaskID`, `GetSelectedNoteSlug`, `ParsedDate`). NotePicker keeps `NotePickerSelectMsg{NoteSlug}` + `GetSelectedNoteSlug()` after the IndexRow retarget so `rk tui`'s selection path is untouched. An implementer must not delete msgs "because `Done()` replaces them."

## Prompt[T] interface shape

```go
// internal/tui/components/prompt.go (new)
type Prompt[T any] interface {
    Init() tea.Cmd
    Update(tea.Msg) (Prompt[T], tea.Cmd)
    View() string
    Result() T
    Done() (finished, canceled bool)
}
```

`Done()` is beyond the ticket's 4-method sketch ("Init/Update/View + Result() T") but is required: Go generics cannot type-switch on each component's distinct `*SelectMsg`/`*SubmitMsg` to detect completion generically (codebase-analysis.md:11-14, 130-132). `RunPrompt`'s host calls `Update`, then `Done()`; on `finished` it captures `Result()` and quits, on `canceled` it quits with the zero value. `Result()` carries no cancellation signal — it is only read once `Done()` reports `finished` (acceptance-criteria.md:79).

Per-component realization (all additive):

| Component | `T` | `Result()` source | `Done()` finished when | new state added |
|---|---|---|---|---|
| Form | `FormResult` | `GetValues()` | existing `submitted` flag | `canceled bool` |
| TextEditor | `string` | `GetText()` | Ctrl+D submit | `submitted`, `canceled bool` |
| DatePicker | `time.Time` | new `result` field | valid Enter | `submitted`, `canceled bool`, `result time.Time` |
| TaskPicker | `string` | `GetSelectedTaskID()` | `selectedTask != nil` | `canceled bool` |
| NotePicker | `string` | `GetSelectedNoteSlug()` | `selectedNote != nil` | `canceled bool` |

`FormResult` (form.go:32-35) already exists; `Form.Result()` returns `FormResult{Values: f.GetValues()}`. [Alternative considered: `T = map[string]string` to match scenario 4's "matching Form.GetValues()" literally; `FormResult` chosen as the semantic result type — scenario 4's assertion compares against `GetValues()` either way.]

## RunPrompt[T] host

```go
func RunPrompt[T any](p Prompt[T], opts ...tea.ProgramOption) (result T, ok bool, err error)
```

**I/O seam (required for Phase 3 testability — advisor-flagged, settled here so the test-writer doesn't invent an ad-hoc one).** With no options, `tea.NewProgram(host).Run()` reads real `os.Stdin`; under `go test` there's no TTY and no way to inject a keystroke, so any test that needs the Program to actually run and receive input (Form Enter→values, DatePicker Esc→zero, every Wizard flow) would hang or error setting raw mode on a non-TTY — the exact hang this ticket exists to prevent, resurfacing in the test path. `opts ...tea.ProgramOption` lets tests pass `tea.WithInput(bytes.NewReader(...))` + `tea.WithOutput(io.Discard)`; production callers pass nothing. `Wizard.Run` threads the same `opts` through its single `RunPrompt` call — one seam covers both.

Shape mirrors the deleted `PickTask`/`PickNote` 3-tuple `(value, canceled bool, err error)` (acceptance-criteria.md:32) — cancellation as a distinct bool, not folded into `T` or `error`. Behavior:

1. Call the injected guard hook first (below); if it errors, return `(zero, false, err)` before touching `tea.NewProgram`.
2. Wrap `p` in a tiny internal `tea.Model` whose `Update` delegates to `p.Update`, then checks `p.Done()`: `finished` → `tea.Quit` (capture `Result()`); `canceled` → `tea.Quit` (leave zero, `ok=false`).
3. `tea.NewProgram(host, opts...).Run()` — no `tea.WithAltScreen()` by default (matches deleted helpers, codebase-analysis.md:116-118), but test `opts` can add `WithInput`/`WithOutput`. Type-assert final model, return.

`Wizard` opens no Program of its own — `Wizard.Run` is `RunPrompt[map[string]any](w, opts...)`, so the same test seam drives scenarios 7-10 without a second code path to seam.

## Wizard: shared-result-map + ESC-back

The Wizard is itself a `Prompt[map[string]any]` — `Wizard.Run()` is just `RunPrompt[map[string]any](w)`, which is why exactly one `tea.NewProgram` drives everything (scenario 6). Its `Result()` returns the shared map; its `Done()` reports overall success (last step finished) / abort (step-0 cancel).

Steps are heterogeneous `Prompt[T]` with different `T`, so they cannot be stored as `[]Prompt[T]` for one `T`. They are registered as **factories** producing a non-generic erased step:

```go
type wizardStep interface {          // non-generic, type-erased
    Init() tea.Cmd
    Update(tea.Msg) (wizardStep, tea.Cmd)
    View() string
    Done() (finished, canceled bool)
    resultAny() any
    key() string
}
// Generic wrapper turning a typed Prompt[T] into an erased step:
func Step[T any](key string, factory func(prior map[string]any) Prompt[T]) StepFactory
```

Factory closures inject earlier results at construction time because `tea.Model`'s `Init()` takes no arguments; every component already primes state via `Show(...)`, not `Init()` (acceptance-criteria.md §2.5). Closure-capture rule (REVIEW_PATTERNS.md:117-144): the factory captures the shared map by value at mount time, not by reading `w.results` inside a deferred `tea.Cmd`.

**ESC-back mechanism (explicit — this is the keybinding-conflict pitfall flagged at codebase-analysis.md:256-261).** No new key, no double-ESC convention. Reuse each step's own cancel signal: after delegating to the active step's `Update`, the Wizard reads that step's `Done()`:

- `finished` → store `resultAny()` under `key()` in the shared map, advance; if it was the last step, Wizard's own `Done()` becomes `finished`.
- `canceled` at **step 0** → whole flow aborts: Wizard's `Done()` becomes `canceled`, no result committed (scenario 9).
- `canceled` at **step >0** → decrement the step index and re-mount the prior step from its factory, which re-primes from the *unchanged* shared map so the earlier value is still shown (scenarios 8, 10). The map is never cleared on back-navigation.

Nil-component guard (REVIEW_PATTERNS.md:147-161): the active-step pointer is nil-checked before `Update`/`View` in case a factory returns nil.

**Step-transition `Init()` (advisor-flagged — easy to miss, bites once 7-10 are runnable).** bubbletea calls `Init()` once at Program start, on the *outer* Wizard model; it never re-fires automatically when the Wizard swaps in a new inner step on advance/back. The Wizard's own `Update` must call the newly-mounted step's `Init()` itself and return its `tea.Cmd` alongside the transition — otherwise a step that relies on `Init` for a cursor blink or initial load silently never gets it.

## TTY guard

- **Seam** (testability, §2.6): `var isInteractive = func() bool { ... }` package-level in `internal/cli` (mirrors `todoNow` at todo.go:33). Overridable in tests since `os.Stat` can't be faked.
- **Detection:** stdlib `os.ModeCharDevice` on **both** stdin and stdout — the guard fires if *either* is not a char device. Stdin is the fatal one (bubbletea's read loop hangs on piped input); stdout matters for garbled render. Matches the documented house pattern (internal/cli/AGENTS.md:231-238, which checks stdout only — extended to stdin per the §4 open question). [Alternative: direct `github.com/mattn/go-isatty` (already indirect at go.mod:34); stdlib chosen to match the documented pattern with zero new import surface. The seam makes the underlying impl swappable, so this is low-stakes.]
- **Flag:** `noInputFlag bool` in root.go:24-32 var block; `RootCmd.PersistentFlags().BoolVar(&noInputFlag, "no-input", false, "Never prompt interactively; error instead of showing a TUI prompt")` at root.go:91-97.
- **Policy + placement:** a `func promptGuard() error` in `internal/cli` returns a usage error naming `--no-input` when `noInputFlag || !isInteractive()`. It is injected into the components layer via a package-level hook so the check lives *inside* `RunPrompt`/`Wizard` (covers every future fnqs.7/fnqs.10 caller automatically, §2.7) without `internal/tui/components` importing `internal/cli`:

```go
// components (prompt.go): nil default = allow, so component unit tests need no setup
var PromptGuard func() error
// cli wires it once at process entry:
components.PromptGuard = promptGuard
```

Nil-by-default means a future fnqs.10 pane host that forgets to wire it is *unguarded* — flag this as the cost of the hook approach (see risks). The guard returns a plain `error` whose message names `--no-input`; wiring a distinct `ExitCodeUsageErr = 2` process exit is out of scope (§2.8 — that constant is dead code today, cmd/rk/main.go exits 1 on any error).

## Index-row replacement type

`components.IndexRow{ID, Title, Type string; Props map[string]string}` lives in `internal/tui/components` (in prompt.go or a new indexrow.go). [Alternative: `internal/index` as a public query type — rejected to avoid a new `components → index` dependency; callers in `internal/cli` already assemble bespoke rows from `nodes`/`node_props` SQL, so they map into `IndexRow` the same way, keeping components self-contained. The import test bans only `internal/journal`/`internal/service`, not `internal/models`/`internal/index`, so neither placement trips it — see no_journal_import_test.go:29-32.] Both pickers switch their item structs and `Show(...)` params to `[]IndexRow`. TaskPicker's `Description()` reads `Props["scheduled"]`/`Props["deadline"]` where it read `TaskRow.DateInfo` fields; NotePicker's reads `Props["slug"]` etc.

## Files to modify

New files:
- `internal/tui/components/prompt.go` — `Prompt[T]` interface, `IndexRow`, `RunPrompt[T]`, `PromptGuard` hook.
- `internal/tui/components/wizard.go` — `Wizard`, `wizardStep`, `StepFactory`, `Step[T]` erasure helper.
- `internal/cli/interactive.go` — `isInteractive` seam, `noInputFlag`-aware `promptGuard()`, and the `components.PromptGuard = promptGuard` wiring (called from an init/setup path).

Modified components (all additive except the `Update` return-type change and the two `Show` signatures):
- `internal/tui/components/date_picker.go` — add `Init`; change `Update` return to `Prompt[time.Time]`; add `DatePickerSubmitMsg`/`DatePickerCancelMsg`, Esc-cancel + valid-Enter-submit emission, `submitted`/`canceled`/`result` state, `Result()`, `Done()`.
- `internal/tui/components/form.go` — add `Init`; `Update` → `Prompt[FormResult]`; add `canceled` flag, `Result()`, `Done()`.
- `internal/tui/components/text_editor.go` — add `Init`; `Update` → `Prompt[string]`; add `submitted`/`canceled`, `Result()`, `Done()`.
- `internal/tui/components/task_picker.go` — add `Init`; `Update` → `Prompt[string]`; add `canceled`, `Result()`, `Done()`; migrate `TaskRow` → `IndexRow` (item struct + `Show([]IndexRow)` + `Description()` from `Props`).
- `internal/tui/components/note_picker.go` — add `Init`; `Update` → `Prompt[string]`; add `canceled`, `Result()`, `Done()`; migrate `Show([]*models.Note)` → `Show([]IndexRow)`, item struct to `IndexRow`, drop the `internal/models` import; keep `NotePickerSelectMsg`/`GetSelectedNoteSlug`/`SetEmbedded`/`IsFiltering`/`SetHeight`.

Live `rk tui` call sites (lockstep updates — Fork A + Fork B ripple):
- `internal/cli/tui_keyboard.go:184` — `m.notes.picker.Update(msg)` returns `Prompt[string]`; assert back: `p, c := m.notes.picker.Update(msg); m.notes.picker = p.(*components.NotePicker)`.
- `internal/cli/tui_keyboard.go:292` — same assertion for `m.datePicker.Update(msg)` → `*components.DatePicker`.
- `internal/cli/tui_keyboard.go:192` — `m.notes.picker.Show(m.notes.notes)`: wrap with a new `notesToRows([]*models.Note) []components.IndexRow` helper.
- `internal/cli/tui_model.go:174` — `m.notes.picker.Show(msg.notes)`: same `notesToRows` wrap. `m.notes.notes` stays `[]*models.Note` (tui_model.go:107) for the inspect pane (notes_pane.go), which is untouched.

CLI flag:
- `internal/cli/root.go` — add `noInputFlag` var (root.go:24-32) and its `PersistentFlags().BoolVar` registration (root.go:91-97). The `promptGuard` policy is *not* added to `PersistentPreRunE` (that runs for every verb; §2.7).

## Test scenarios (mapped to files / function names)

`internal/tui/components/prompt_test.go` (new):
- Scenario 1 → `TestPrompt_ConformanceCompiles`: five `var _ Prompt[T] = (*Component)(nil)` assertions (Form/TextEditor/DatePicker/TaskPicker/NotePicker).
- Scenario 4 → `TestRunPrompt_FormSubmitReturnsValues`: drive via `RunPrompt(form, tea.WithInput(bytes.NewReader(enterKeySeq)), tea.WithOutput(io.Discard))`; primed Form, Enter → `(GetValues match, true, nil)`.
- Scenario 5 → `TestRunPrompt_EscReturnsZeroNotOK` (reframed from "Result() never invoked", which needs a spy the concrete type doesn't have, to a behavioral assertion): DatePicker driven via the same `WithInput`/`WithOutput` seam, Esc → `(zero, false, nil)`.
- Scenario 6 → reframed from a source-grep ("tea.NewProgram appears once") to the real invariant: **Wizard opens no Program of its own** — covered by scenarios 7-10 actually running end-to-end through the `WithInput`/`WithOutput` seam and producing correct multi-step results; no separate test needed once those pass.
- Guard-at-host: `TestRunPrompt_GuardBlocksBeforeProgram`: set `PromptGuard` to return an error; assert `RunPrompt` returns it and never blocks.

`internal/tui/components/date_picker_test.go` (extend; reuse `futureDate()` helper at date_picker_test.go:11-16):
- Scenario 2 → `TestDatePicker_UpdateEnterSelfSignalsSubmit`.
- Scenario 3 → `TestDatePicker_UpdateEscSelfSignalsCancel`.

`internal/tui/components/wizard_test.go` (new):
- Scenario 7 → `TestWizard_ThreeStepsCollectAllResults` (DatePicker → Form → TextEditor).
- Scenario 8 → `TestWizard_EscMidFlowStepsBackKeepsResult`.
- Scenario 9 → `TestWizard_EscAtStep0AbortsFlow`.
- Scenario 10 → `TestWizard_FactoryStepSeesPriorResult`.

`internal/tui/components/note_picker_test.go` (new — no test file exists today, codebase-analysis.md:280-283):
- Scenario 14 → `TestNotePicker_ShowWithIndexRowSelects` (title display + selection parity for surviving fields).

`internal/cli/interactive_test.go` (new):
- Scenario 11 → `TestPromptGuard_NonTTYErrors`: stub `isInteractive` → false; `promptGuard()` returns an error naming `--no-input`.
- Scenario 12 → `TestPromptGuard_NoInputFlagBeatsTTY`: `isInteractive` → true, `noInputFlag = true`; same error.
- Scenario 13 → `TestPromptGuard_NotInvokedForNonPromptingVerb`: a verb path that never calls `RunPrompt` completes with no flag-attributable error (falls out of guard placement, §2.7 — assert `promptGuard` is not on `PersistentPreRunE`).
- Scenario 15 → asserted structurally by both pickers sharing `components.IndexRow`; document convergence in `TestNotePicker_ShowWithIndexRowSelects` or a comment (both pickers now take `[]IndexRow`).

Enumerated test churn from the Fork A `Update` return-type change (mechanical `got, _ := comp.Update(msg); got.(*Concrete).Method()`): `date_picker_test.go`, `form_test.go`, `task_picker_test.go`, `text_editor_test.go`, and `internal/cli/tui_test.go` (NotePicker's only current coverage).

## Known risks and ambiguities

- **Fork A test churn is broad but mechanical.** Every existing component-test site doing `x, cmd := x.Update(msg)` then calling a concrete method breaks and needs a `.(*Concrete)` assertion. Bounded and known (files listed above); no logic change, but a place for copy-paste slips.
- **Guard hook is nil-by-default global state.** A future fnqs.10 pane host that forgets `components.PromptGuard = ...` runs *unguarded* (silently allows prompts on a non-TTY). Mitigation is a process-entry wiring convention in `internal/cli`; flagged so fnqs.10 knows to wire it too. [Alternative: pass an explicit guard into `RunPrompt` — rejected because §2.7 wants the check automatic so callers can't forget; the hook is the automatic version, at the cost of this global.]
- **`IndexRow.Props` typing for TaskPicker dates.** `TaskRow.DateInfo` used typed `*string` fields (task_picker.go:74-81); moving to `Props["scheduled"]`/`["deadline"]` is stringly-typed. Behavior-preserving for display, but callers assembling `IndexRow` must populate those keys consistently. No live TaskPicker caller exists to break, so this is deferred to fnqs.7's wiring. [INFERRED] key names (`scheduled`/`deadline`/`slug`) are implementer's choice — not dictated by any existing schema constant.
- **NotePicker Props parity loss is sanctioned, not accidental.** Tags/created are folded into `Props` without formatting parity; scenario 14 explicitly requires only title + an id-equivalent. Confirm no other `rk tui` path depends on NotePicker's richer `Description()` output during implementation (the inspect pane uses `NotesPane`/`models.Note` directly, not the picker's Description).
- **`FORM_README.md` / `INTEGRATION_GUIDE.md`** in `internal/tui/components/` describe deleted v0 flows (codebase-analysis.md:101-105). [OPEN] whether to update/delete — not in scope, low cost, left to implementer's discretion.
- **`ExitCodeUsageErr = 2`** stays dead code (§2.8); the guard returns a message-only error. Wiring a real exit-2 path through `cmd/rk/main.go` is explicitly out of scope.

### Critical Files for Implementation
- internal/tui/components/date_picker.go
- internal/tui/components/note_picker.go
- internal/tui/components/task_picker.go
- internal/cli/tui_keyboard.go
- internal/cli/root.go
