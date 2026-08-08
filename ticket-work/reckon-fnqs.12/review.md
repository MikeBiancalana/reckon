# Code Review: reckon-fnqs.12 — `rk checklist run` interactive mini-TUI

**Verdict: APPROVE**

The implementation is correct, honors every hard ticket constraint, and is well tested. Two
minor findings follow; **neither gates merge** — both are optional. Verified `go test -count=1`
across `internal/tui/components` and `internal/cli` (all pass, including the Program-driven
Ctrl+C case).

---

## Scope reviewed

| File | Change |
|---|---|
| `internal/tui/components/checklist_runner.go` | new — `ChecklistItem`, `ToggleFunc`, `ChecklistRunner` (`Prompt[[]ChecklistItem]`) |
| `internal/cli/checklist_run.go` | new — `run` verb, `runItemsToChecklistItems`, `makeToggleFunc` |
| `internal/cli/checklist.go` | modified — register cmd; extract `resolveChecklistRun`, `checkAndRefetchRun` |
| `internal/tui/components/checklist_runner_test.go` | new — 12 behavioral tests via `RunPrompt` + fake toggler |
| `internal/cli/checklist_test.go` | +7 tests — resolve/toggle/convert/guard/not-found |

---

## Constraint & focus-area verification

| Focus area (from task) | Result | Evidence |
|---|---|---|
| `ChecklistRunner` avoids `internal/checklist` | ✅ | checklist_runner.go:3-8 imports only `fmt`, `strings`, `bubbletea` |
| Cursor clamp — no off-by-one / wraparound | ✅ | checklist_runner.go:87-94 (`cursor < len-1` / `cursor > 0`); tests :112-138 |
| Ctrl+C caught → `(ok=false, err=nil)` | ✅ | checklist_runner.go:85 handles `ctrl+c`; test :230 passes uncached (real evidence, not assumed) |
| `Done()` finished/canceled/error correct; no silent swallow | ✅ | See error-path table below |
| `checkAndRefetchRun` preserves `check` behavior/messages | ✅ | checklist.go:483-491; caller re-prefixes `checklist check:` (:474) — identical |
| `resolveChecklistRun` preserves `start` behavior/messages | ✅ | checklist.go:429-449; caller re-prefixes `checklist start:` (:424) — identical |
| Toggle closure staleness/race | ✅ | Captures immutable `runID` string (:68); `Update` is single-threaded, no `tea.Cmd` goroutine |
| Empty-checklist edge case | ✅ | checklist_runner.go:96,127; test :237 |
| "Run already completed" edge case | ✅ | Not reachable via resume (`GetActiveRun` returns only active); `makeToggleFunc` re-fetches by ID via `GetRunStatus`, so post-completion fetch still works (checklist.go:74) |

### Error-path enumeration (no path silently swallows an error)

| Outcome | `RunPrompt` returns | `runner.Err()` | CLI result |
|---|---|---|---|
| Guard blocks (non-TTY / `--no-input`) | `(_, false, guardErr)` | nil | returns `err` (surfaced) |
| `tea.Program.Run()` fails | `(_, false, err)` | nil | returns `err` (surfaced) |
| User quit (q/esc/ctrl+c) | `(_, false, nil)` | nil | returns nil — correct, quit is not an error |
| Mid-session toggle error | `(_, false, nil)` | non-nil | wrapped `checklist run: %w` (surfaced) |
| Auto-complete | `(items, true, nil)` | nil | returns nil |

`Done()` (checklist_runner.go:67-69) never reports finished+canceled together, satisfying the
`Prompt[T]` contract (prompt.go:18-20). The mid-session-error case reports `canceled=true` so
the host quits leaving the zero value, then the CLI reads `Err()` after return — the error is
carried, not lost.

---

## Findings

### 1. Phantom run created on non-TTY invocation with no active run — *optional*

**checklist_run.go:34-42.** `resolveChecklistRun` runs *before* `RunPrompt`, but the TTY guard
fires *inside* `RunPrompt`. So a non-TTY `rk checklist run <t>` with no existing active run calls
`StartRun` (writing a fresh run) and only *then* errors from the guard. AC #7 is still met (it
errors, does not hang), and the effect is benign/self-healing: the first non-TTY call creates the
run, later calls resume it, and a subsequent real-TTY run resumes it indistinguishably (only
`StartedAt` differs from a later fresh start). The plan (decision 5) flagged this as accepted.

- **Remedy if you want it gone:** add `if err := components.PromptGuard(); err != nil { return err }`
  at the top of `runChecklistRunE`, before `resolveChecklistRun`. This is **not** the AC #9-forbidden
  bespoke isatty check (that means a hand-rolled `isCharDevice` in checklist.go) — it is the same
  shared hook, invoked early.
- **Test gap either way:** `TestChecklistRun_GuardNonTTY` (checklist_test.go:995) asserts the error
  but not DB state, so it would not catch the phantom run. If the fix is applied, pair it with an
  assertion that no active run exists after the guarded call.

### 2. Unwrapped `RunPrompt` error — *cosmetic*

**checklist_run.go:42-44.** `return err` breaks this file's otherwise-uniform
`fmt.Errorf("checklist run: %w", ...)` wrapping (cf. :36, :46). Wrapping is safe for the guard
tests — `%w` preserves the `--no-input` substring they assert. Trivial one-line consistency nit.

### 3. Completion-banner scrollback survival — *manual-verify, [OPEN]*

The trailing-newline trick (checklist_runner.go:147) is meant to keep the `✓ Complete!` frame in
scrollback after bubbletea's final flush. Tests assert `View()` *content* contains the banner
(test :197) but cannot exercise terminal flush under `io.Discard`. Eyeball once in a real
terminal; out of unit-test reach, as the plan noted.

---

## Minor test-coverage observations (non-blocking)

- Arrow keys (`up`/`down`) and `enter` are handled identically to `k`/`j`/`space` via the
  `keyMsg.String()` switch but are only exercised through the `j`/`k`/`space` variants. Low risk.
- Mid-session error renders `error: %v` in a flashed final frame *and* cobra prints the wrapped
  error — slight double-display on that path. Cosmetic.

---

## Positive observations

- Hard decoupling constraint (AC #10/#11) cleanly met: the component speaks only `ChecklistItem`;
  the CLI owns both-direction conversion and is the sole holder of `*checklist.Service`.
- `View()` builds a `[]string` + `strings.Join` (checklist_runner.go:117-155), correctly avoiding
  the unconditional-newline phantom-blank-line pattern (REVIEW_PATTERNS.md:879-901).
- Both extractions (`checkAndRefetchRun`, `resolveChecklistRun`) genuinely unify `run` with the
  shipped `check`/`start` paths — one persistence path, no divergence (AC #5).
- Tests assert cursor→position wiring by *which* item flips (via `fakeToggler.calls`), not merely
  that *a* toggle happened; the `singleByteReader` seam correctly prevents bubbletea rune-coalescing
  from masking repeated-key clamp behavior.
