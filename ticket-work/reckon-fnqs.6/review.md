# Code Review: reckon-fnqs.6 — Composable prompt layer

**Verdict: APPROVE WITH CHANGES**

One required change (a live, trivially-fixable display regression in `rk tui`), plus
non-blocking notes. Build, `go vet`, and the full test suite are green; the architecture is
clean and the design forks resolved in the plan are faithfully realized in the code.

---

## Summary

The `Prompt[T]` interface, generic `RunPrompt[T]` host, type-erased `Wizard`, and TTY guard are
well-constructed and correct. The five components satisfy `Prompt[T]` directly via their own
`Update` methods (Fork A option (a)), the two pickers converge on a shared `IndexRow`
(Fork B), and exactly one `tea.NewProgram` drives everything. The four flagged risk areas
(ESC-back re-mount, nil guard hook, `Update` return-type threading, dead date-picker msgs) all
hold up. The single blocking issue is the pre-flagged `notesToRows` `CreatedAt` handling, which
is a real regression, not a non-issue.

---

## Critical Issues (must fix)

### C1 — `notesToRows` unconditionally emits a garbage `created` date into every `rk tui` note row

`internal/cli/tui_model.go:132` formats `n.CreatedAt` into `Props["created"]` with no zero
guard. This is the item deferred to review, and it **is** a real, live regression:

- `listNotes` → `loadNoteDisplay` (`internal/cli/tui_read.go`) constructs
  `&models.Note{ID, Title, Slug}` and **never sets `CreatedAt`**. So in the `rk tui` notes
  browse path, every note has a zero `time.Time`.
- A zero `time.Time.Format("2006-01-02")` yields the non-empty string `"0001-01-01"` (verified).
- The new `notePickerItem.Description()` (`note_picker.go:74`) appends `created` whenever the
  string is non-empty — so **every note row in the notes pane now renders
  `| created: 0001-01-01`**.
- The pre-migration `NotePicker.Description()` guarded this exact case with
  `if !i.note.CreatedAt.IsZero()` (confirmed against `origin/main`), so it displayed nothing.

This directly breaks the plan's own guardrail ("zero behavior change to the one working
porcelain", plan.md:11,167). It is cosmetic (no crash, no data effect), but it is visible on
every note in the primary porcelain and the fix is one line.

**Fix:** restore parity in `notesToRows` — only set the `created` key when
`!n.CreatedAt.IsZero()`. (`tags` already degrades correctly: `strings.Join(nil, ", ")` is `""`,
which `Description()` skips — only `created` needs the guard.)

**Why it slipped preflight:** `note_picker_test.go`'s scenario-14 test only ever puts `slug` in
`Props`; it never exercises the `created`/`tags` folding. The fix should ship with a regression
test that passes a row with a zero/absent `created` and asserts the rendered description
contains no `0001-01-01`.

---

## Recommendations (non-blocking)

### R1 — Wizard/RunPrompt structurally drop each step's priming `tea.Cmd` (forward-looking)

The transition machinery correctly *calls* the newly-mounted step's `Init()` on advance/back
(`wizard.go:128,142`) — but for all 5 components `Init()` returns `nil`, and the real priming
lives in `Show()`, whose returned `tea.Cmd` (e.g. `textInput.Focus()`'s blink) is discarded by
every factory. `Step[T]`'s signature (`func(prior map[string]any) Prompt[T]`, wizard.go:51) gives
the factory no channel to propagate a cmd, so the "call Init() on transition" logic is currently
a no-op.

- **Not a bug today:** focus *state* is set synchronously inside `Show()→Focus()` (the passing
  wizard tests type into and submit fields, proving input reaches them); only the cursor-blink
  animation is lost, and no live caller routes through Wizard/RunPrompt yet.
- **Why note it:** it contradicts the plan's stated `Init()` rationale (plan.md:87) and is a trap
  for fnqs.7/fnqs.10 — any future component doing real async work in its `Show` cmd (initial
  load, etc.) will have it silently dropped. Consider having `Step[T]` factories return
  `(Prompt[T], tea.Cmd)`, or documenting that priming must be synchronous.

### R2 — `RunPrompt` host does not check `Done()` after `Init()`

`runPromptHost.Init` (`prompt.go:50`) only forwards `p.Init()`; it never checks `Done()`. A
Prompt that is already terminal at mount time hangs the program until the first message arrives.
Reachable only via (a) an empty `Wizard` (0 steps → `finished=true` at `Init`) or (b) reusing a
completed component without an intervening `Show()`. Neither is a real use case, but a cheap
guard (check `Done()` in the host `Init` and return `tea.Quit` if terminal) closes the latent
hang.

### R3 — Guard message's suggested remedy is misleading

`promptGuard` (`interactive.go:34`) returns "…pass `--no-input` or run from an interactive
terminal." On a non-TTY, *passing `--no-input` produces the same error* — it is not a remedy.
The acceptance criteria only require the message to name `--no-input`, which it does, and the
verb-specific flag alternative is fnqs.7's job, so this is a wording nit. When fnqs.7 wires real
flags, revisit the phrasing (the true remedy for a non-TTY is "provide the value via flags").

### R4 — `Show()`-as-reset-point is a documented but sharp contract

Moving selection-reset from `Hide()` to `Show()` (note_picker.go:196, task_picker.go:182) is
sound: every live caller re-`Show()`s before reuse, and `Update` early-returns while `!visible`.
But a completed picker handed straight to `RunPrompt` without `Show()` would report
`finished=true` on the first `Update` and return a **stale** `Result()`. The `Hide()` comment
documents the contract; no change required, just flagging the edge for future callers.

---

## Flagged areas — confirmed sound

- **Wizard ESC-back re-mount (wizard.go):** correct. Back-nav decrements the index and re-mounts
  the prior step from its factory, re-priming from the unchanged shared map
  (`TestWizard_EscMidFlowStepsBackKeepsResult` proves the round-trip). No goroutine/cmd leak:
  bubbletea `Cmd`s are inert `func() tea.Msg` closures; the abandoned step's discarded
  submit/cancel cmd simply never executes. The shared map never retains a result for an index
  beyond the current step, so no staleness.
- **Nil `PromptGuard` hook:** the accepted risk still looks reasonable. It is auto-wired in
  `interactive.go`'s `init()`, so any binary linking `internal/cli` (i.e. the real `rk`) is
  always guarded; the footgun only bites a hypothetical future entry point that hosts prompts
  without importing `cli`. The guard runs before `tea.NewProgram` (prompt.go:79), and
  `Wizard.Run` inherits it through the single `RunPrompt` call.
- **`Update` return-type change:** threaded correctly through both live sites — notes picker
  (tui_keyboard.go:183-184) and datePicker (tui_keyboard.go:291-292) — each with a panic-safe
  `.(*Concrete)` assertion (the concrete `Update` always returns the same concrete pointer).
  `TaskPicker` has zero live callers; `log.view`/`links`/`textEntry` are unconverted components
  whose signatures are untouched. Runtime behavior of `rk tui` is preserved (the added
  `canceled` flag is inert — nothing reads the pickers' `Done()`).
- **`DatePickerSubmitMsg`/`DatePickerCancelMsg` dead for the live caller:** confirmed.
  `handleDateSubFlowKey` intercepts Esc/Enter itself (tui_keyboard.go:274-290) and only delegates
  other keys to `dp.Update`; grep finds no handler for either msg anywhere in `internal/cli`. The
  new emissions are exercised only by the component tests and the host.
- **Hide()/Show() reset move:** no stale-selection bug for live callers (see R4).

---

## Dimension notes

- **Correctness:** one real regression (C1); everything else holds. All 5 components set their
  terminal flag synchronously inside `Update`, so the host detects `Done()` on the same tick —
  the generic host has no per-component special-casing to get wrong.
- **Architecture:** clean. Type-erased `wizardStep` is the idiomatic way to chain heterogeneous
  generics under one `T`; a single `tea.NewProgram` drives components and wizard alike. `IndexRow`
  in `components` keeps the layer self-contained (no `components → index` edge). Stringly-typed
  `Props` is an accepted, documented trade-off.
- **Testing:** strong. The timeout-bounded goroutine harness (`runPromptForTest`) is a nice
  defense against a mis-scripted keystroke hanging `go test`. Gap: the display-parity path that
  C1 broke is untested (drove the regression past preflight); add it with the fix. Runtime
  coverage is representative (Form + DatePicker), which the acceptance criteria explicitly permit.
- **Maintainability:** comments explain design intent without ticket/plan provenance (house
  rule respected). R1's dropped-cmd seam is the main future-facing sharp edge.
- **Error handling:** `RunPrompt` returns/​wraps the `tea.Program` error and propagates the guard
  error before opening a program; date parse errors surface as inline component state.
- **Performance:** negligible — `notesToRows` allocates one small map per note on an
  already-small list.
- **Security:** no injection/secret surface. `PromptGuard` is an unsynchronized mutable package
  global mutated by tests, but no test runs in parallel, so no data race.

---

## Positive observations

- Both design forks (A: direct interface satisfaction; B: shared `IndexRow`) are implemented
  exactly as the plan resolved them, and the compile-time conformance block in `prompt_test.go`
  locks Fork A in place.
- The DatePicker `result`-capture-before-`Hide()` ordering (date_picker.go:150-154) is a
  genuinely subtle correctness point handled correctly and clearly commented.
- The guard seam (`isInteractive` var + `PromptGuard` hook wired via `init()`) mirrors the
  existing `todoNow` precedent and keeps the check inside `RunPrompt`, so it covers every future
  caller for free without `components` importing `cli`.
