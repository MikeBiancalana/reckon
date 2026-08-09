# Plan: reckon-6k0l — `rk todo list` interactive mini-TUI

## Summary of approach

Overload the existing `rk todo list` verb so that a **bare invocation on a real TTY** opens a single-screen bubbletea browser hosted by fnqs.6's `components.RunPrompt`, mirroring fnqs.12's `ChecklistRunner`/`checklist_run.go` structure. The browser lists open todos, moves a cursor, and marks the selected item done through an injected callback that calls the **existing** `doneDurableTodo`/`doneEphemeralTodo` verb functions — no parallel write path. Every other invocation (non-TTY, or any output-shaping flag present) runs today's classic list body **byte-for-byte unchanged**.

Two new units plus a minimal branch:

1. **`internal/tui/components/todo_browser.go`** — a `TodoBrowser` implementing `Prompt[[]TodoItem]`. Holds a `[]TodoItem` display slice, `cursor int`, terminal flags, an injected `MarkDoneFunc`, and a mid-session `err`. Never imports `internal/index`/`internal/node`. Structurally a near-copy of `ChecklistRunner` with three deltas (§Component).
2. **`internal/cli/todo_browse.go`** — `runTodoBrowse` (config/index/reconcile/build/RunPrompt), `buildTodoItems` (index rows → `[]TodoItem`), `makeMarkDoneFunc` (the dispatch closure). Sibling file mirroring `checklist_run.go`'s convention, keeping `todo.go`'s classic path untouched so byte-identical output is trivially verifiable.
3. **`internal/cli/todo.go`** — `runTodoListE` gains one early dispatch branch (§Dispatch); classic body otherwise unchanged.

Everything else here — data sources, ref namespaces, write-function signatures, the `Prompt[T]`/`RunPrompt`/`PromptGuard` contract — is documented in `ticket-work/reckon-6k0l/codebase-analysis.md` §§2–5 and `acceptance-criteria.md` §§2–3; this plan references those rather than restating them.

## Settled design decision: dispatch (not open for re-litigation)

Bare `rk todo list` on a **real TTY** opens the mini-TUI. Every other invocation — non-TTY (any invocation), or any recognized output-shaping flag present — produces **today's exact classic-path output, byte-identical, with `RunPrompt`/`PromptGuard` never invoked**. This matches fnqs.7's stated bare-vs-args convention (`todo list` has real non-TTY script/agent callers to preserve), not fnqs.12's always-guarded model (`checklist run` was brand-new with no non-TTY callers). It preserves the four existing non-TTY tests (`todo_test.go:733,757,790,892`) unmodified, since `isInteractive()` already reports false under `go test`.

The one sub-case the fork's framing didn't explicitly adjudicate — **`--no-input` on a real TTY** — resolves to **error**, stated as a decision, not [OPEN]:

| invocation | result |
|---|---|
| bare, real TTY | mini-TUI |
| bare, real TTY, `--no-input` | guard error (via `PromptGuard`) |
| bare, non-TTY (± `--no-input`) | classic text output (no error) |
| any output-shaping flag, any TTY | classic text output |

Rationale: `--no-input`'s registered help (`root.go:99`) is "*Never prompt interactively; error instead of showing a TUI prompt*"; the ticket Done-when says "non-tty or `--no-input` errors per the TTY guard"; and mirroring checklist.go's guard mechanism *requires* reaching `RunPrompt` for the guard to fire. "Recognized flag" in the fork resolution = the output-selector set (`--all/--state/--durable/--ephemeral/--json/--ndjson`); `--no-input` is a prompt-suppression flag, not an output selector, so it is deliberately excluded from the classic-route set. A bare-but-for-`--no-input` TTY call therefore enters the TUI branch, where `RunPrompt`'s `PromptGuard` (`noInputFlag || !isInteractive()`) errors. The competing "byte-identical to today" pull was written to protect non-TTY scripts (rows 3–4, where the outcome is identical either way); it does not make `--no-input` a permanent no-op on `todo list`. *Rejected alternative:* add `--no-input` to the classic-route set (never errors) — contradicts the flag's documented purpose and the Done-when.

## Files to create / modify

| File | Action | Reason |
|---|---|---|
| `internal/tui/components/todo_browser.go` | **create** | `TodoItem` type, `MarkDoneFunc` type, `TodoBrowser` component implementing `Prompt[[]TodoItem]`. Near-copy of `checklist_runner.go` (constructor + `Show` prime + `Init/Update/View/Result/Done/Err`). |
| `internal/tui/components/todo_browser_test.go` | **create** | Behavioral tests driving the browser through `RunPrompt` with scripted keystrokes + a fake `MarkDoneFunc`, reusing `runPromptForTest`/`singleByteReader` from the package's existing test files. |
| `internal/cli/todo_browse.go` | **create** | `runTodoBrowse`, `buildTodoItems`, `makeMarkDoneFunc`. Sibling file (not appended to `todo.go`) for cohesion, mirroring `checklist_run.go`. |
| `internal/cli/todo.go` | **modify** | Add the dispatch branch at the top of `runTodoListE` (§Dispatch). Classic body unchanged. |
| `internal/cli/todo_test.go` | **modify (add funcs)** | CLI-layer tests: dispatch matrix (stub `isInteractive`), `buildTodoItems` mapping, `makeMarkDoneFunc` persistence against a temp vault, ephemeral-ref stability. Reuses `runTodo`/`writeEphemeralContainer` helpers already in the file. |

## Dispatch wiring (`runTodoListE`, `todo.go:438`)

Add at the very top, after the `defer resetTodoFlags(cmd)` (which still fires on either branch):

```
if todoListWantsTUI(cmd) {
    return runTodoBrowse(cmd)
}
// ... existing classic body unchanged ...
```

`todoListWantsTUI(cmd)` returns `isInteractive() && !anyChanged`, where `anyChanged` is true if any of `cmd.Flags().Changed("all"|"state"|"durable"|"ephemeral")` **or** the persistent `jsonFlag`/`ndjsonFlag` is set. It does **not** consult `noInputFlag` (see decision matrix — `--no-input` on a TTY reaches the guard).

**Preempting the AC §1.8 "no bespoke isatty check" review flag:** the dispatch reuses the **shared** `isInteractive()` seam (`interactive.go:16`, a stubbable package var) purely to *select the branch*. This is not the forbidden thing. The forbidden thing is a hand-rolled `os.Stat(os.Stdin)` in `todo.go` duplicating the guard's device logic. An explicit branch is structurally unavoidable here because non-TTY must **fall through** to classic output, which a guard-that-errors cannot do — this is exactly the shape checklist didn't need (no non-TTY callers). The guard itself (for `--no-input` and as redundant non-TTY defense) remains `PromptGuard`-via-`RunPrompt`, unchanged from checklist.go.

`runTodoBrowse` does not re-`defer resetTodoFlags` — the caller's defer covers it.

## `runTodoBrowse` skeleton (`todo_browse.go`)

```
func runTodoBrowse(cmd *cobra.Command) error {
    cfg, err := config.LoadWithOverrides(vaultFlag, "")   // same as classic
    if err != nil { return fmt.Errorf("todo list: load config: %w", err) }

    items, err := buildTodoItems(cfg)                     // open+reconcile+list, then CLOSE index
    if err != nil { return err }

    browser := components.NewTodoBrowser("Todos")
    browser.Show(items, makeMarkDoneFunc(cfg.VaultDir, items))

    if _, _, err := components.RunPrompt[[]components.TodoItem](browser); err != nil {
        return fmt.Errorf("todo list: %w", err)           // guard/Program error → surface
    }
    if e := browser.Err(); e != nil {                     // mid-session mark-done error
        return fmt.Errorf("todo list: %w", e)
    }
    return nil                                            // ok=false (user quit) is not an error
}
```

**Close the index inside `buildTodoItems`, before the interactive session** (advisor point 4): the write path (`doneDurableTodo`/`doneEphemeralTodo`) takes `vaultDir` and bypasses the index, and refresh strategy (B) never re-queries, so the index handle is needed only to build the initial slice. `buildTodoItems` opens, `Reconcile()`s, runs the existing `listDurableTodos`/`listEphemeralTodos`, maps to `[]TodoItem`, then closes — no SQLite handle held across the session.

## Component: `TodoBrowser` (`todo_browser.go`)

Delta from `ChecklistRunner` is small — three changes:

**Display type & callback:**
```
type TodoItem struct {
    Kind  string // "durable" | "ephemeral" — dispatch discriminator, opaque to the component
    Ref   string // durable ULID, or ephemeral 1-based line index as a string — opaque to the component
    Title string // render text (never empty; fallbacks applied at build time)
    Done  bool   // rendered checkbox state
}
type MarkDoneFunc func(position int) (remaining []TodoItem, err error)
```
A single unified type flows both ways: the component renders `Title`/`Done` and carries `Kind`/`Ref` opaquely (never interpreting them, satisfying the "no dependence on node/index internals" constraint); the CLI closure dispatches on `Kind`/`Ref`. This eliminates the parallel per-position dispatch slice that `acceptance-criteria.md` §2 warned about (and its drift risk, AC §3 "Mixed-kind list ordering stability") — the ticket's illustrative `{ID, Title, Done}` already blesses carrying identity in the display type.

**One-directional, shrinking callback (not a toggle):** named `MarkDoneFunc`, not `ToggleFunc`, and it drops checklist's `completed bool` return (no aggregate "browsing complete" signal — AC §2). On `space`/`enter` with a non-empty list, `Update` captures `pos := r.cursor`, calls `r.onMarkDone(pos)`; on error sets `r.err`; on success adopts the returned (shrunk) slice and **clamps** `if r.cursor > len(r.items)-1 { r.cursor = len(r.items)-1 }` (and guards `len==0`). Removal is always at the cursor position (you can only act on the item under the cursor) — there is no re-sort — so index-clamp lands on the sensible next item and needs no ID-based re-location (the `REVIEW_PATTERNS` "Index-Only Selection Identity After Re-Sort" case does not apply). There is no scroll/viewport (renders all rows, natural terminal scroll, matching `ChecklistRunner`), so the "Scroll Offset Not Clamped" pitfall also does not apply.

**`T` and `Done()`:** `T = []TodoItem` (symmetry with the other components; lets tests assert `Result()`). `Done()` returns `finished=false` **always**, `canceled = r.canceled || r.err != nil`. `finished` never fires — there is no terminal "done browsing" state — so `RunPrompt` always returns `ok=false` and the CLI prints nothing on exit; every mark-done already persisted per-keypress. `Err()` (not part of `Prompt[T]`) surfaces a mid-session callback error, read by the CLI after `RunPrompt` returns, exactly as `ChecklistRunner.Err()`.

Unchanged from `ChecklistRunner`: `NewTodoBrowser(title)` + `Show(items, onMarkDone)` prime/reset; `q`/`esc`/`ctrl+c` → `canceled` (Ctrl+C handled in `Update`, never falling through to bubbletea's kill path — the no-error-on-quit contract); `j`/`k`/`up`/`down` clamp; empty-list `(no items)` render; `[]string` + `strings.Join` view build (no phantom-blank-line pitfall). View row: cursor prefix + `[x]`/`[ ]` (from `Done`) + `Title`. No `WindowSizeMsg`/wrapping (AC §3, accepted).

## `makeMarkDoneFunc` (dispatch closure, `todo_browse.go`)

Captures `vaultDir` and its **own mutable copy** of `[]TodoItem` (the authoritative session slice; the component adopts whatever the closure returns, so the two stay in lockstep after the first action):

```
func makeMarkDoneFunc(vaultDir string, items []TodoItem) components.MarkDoneFunc {
    session := append([]TodoItem(nil), items...)
    return func(pos int) ([]TodoItem, error) {
        it := session[pos]
        var err error
        if it.Kind == "ephemeral" {
            _, err = doneEphemeralTodo(vaultDir, it.Ref)   // Ref is the 1-based line index string
        } else {
            _, err = doneDurableTodo(vaultDir, it.Ref)     // Ref is the ULID
        }
        if err != nil { return nil, err }
        session = append(session[:pos], session[pos+1:]...) // remove the acted item
        return append([]TodoItem(nil), session...), nil
    }
}
```

Remove-on-action (rather than flip-`Done`-in-place) is deliberate and covers three cases at once: (1) matches "list open todos"; (2) prevents the **recurring double-advance hazard** — a `repeat:` todo never flips to `Done`, so leaving it visible would let a second `space` call `doneRecurringTodo` again and advance `scheduled` twice; (3) one-directional completion means a done item has no further useful interaction. `doneDurableTodo` internally routes recurring items through `doneRecurringTodo` (state stays `"open"`, `scheduled` advances) — the closure treats every non-error result uniformly as "remove from session." Recurring representation: **the item simply disappears from this session's view** and reappears (correctly, with advanced `scheduled`) on the next `rk todo list`. The `todoDoneResult` return value is discarded here; a transient "advanced to <date>" footer is stretch-only, not built (AC §2 [OPEN], deferred).

**Refresh strategy: (B) local mutation, no mid-session re-query** — per `acceptance-criteria.md` §2 and `codebase-analysis.md` §3. Load-bearing justification (advisor point 3): `flipChecklistLine` (`todo.go:991`) flips a **single byte in place** and never removes a line, so the 1-based file-order index of every *other* ephemeral checkbox stays valid for the whole session; durable ULIDs are immutable. Captured `Ref`s therefore never drift, making local removal correct, not merely cheap. Documented gap: a recurring completion that materializes a new `todos/inbox.md` pile-up item (`todo.go:838`) won't appear live — it surfaces on the next `rk todo list`.

## `buildTodoItems` mapping (`todo_browse.go`)

Reuses the existing `listDurableTodos(db, false, "")` + `listEphemeralTodos(db, false)` (open-only, matching classic default), concatenated durable-then-ephemeral. Per row:
- **durable:** `Kind="durable"`, `Ref=row.ID`, `Done = (row.State=="done")` (always false here since open-only), `Title` = fallback chain `row.Title → row.Body → row.ID` (AC §3: `deriveTitle` returns `""` for a blank body; a row must never render blank).
- **ephemeral:** `Kind="ephemeral"`, `Ref = strconv.Itoa(row.Line)`, `Done = row.Checked`, `Title` = `row.Body`, falling back to a literal placeholder (e.g. `"(empty item)"`) when the checkbox text is empty (`splitChecklistLines` capture group 2 can be `""`).

## Test scenarios

Component tests (`todo_browser_test.go`) drive `RunPrompt` directly with scripted keys + a fake `MarkDoneFunc` (guard is nil in the components package, so `RunPrompt` runs the Program under `WithInput`/`WithOutput`). CLI tests (`todo_test.go`) verify wiring without driving a full Program through `RootCmd.Execute` (following the reference plan's "no test-only program-options global" stance).

**From `acceptance-criteria.md` §4 (cite by reference; do not restate):** all "Interactive TUI path" and "TTY-guard / dispatch-boundary path" given/when/thens apply. The four "Non-interactive / flagged path" regression tests (`todo_test.go:733,757,790,892`) must pass **unmodified** — the hard consequence of the dispatch decision.

**Highest-value scenarios that have no `ChecklistRunner` coverage (new-vs-reference; make explicit):**
- **List shrinks on mark-done + cursor clamps:** 3 open items, cursor on the last, `space` → item removed, `Result()` length 2, cursor clamped to index 1 (mirrors AC §3 "Cursor clamp after a toggle shrinks the list").
- **Last open item toggled → clean empty transition:** 1 item, `space` → view renders `(no items)`, no index-out-of-range, quit exits cleanly.
- **Heterogeneous cursor-to-ref wiring:** a list mixing durable + ephemeral rows; navigate onto an **ephemeral** row and `space` → assert the ephemeral branch fires with the correct **line-index** `Ref` (not the durable branch), proving `Kind`/`Ref` dispatch, not position-guessing. The fake `MarkDoneFunc` records `(Kind, Ref)` per call, mirroring `fakeToggler.calls`.
- **Ephemeral-ref stability across two sequential toggles** (advisor point 3): a container where an earlier line is already checked; toggle two different unchecked ephemeral items in sequence → assert each hits its intended file line (proving captured `Ref`s don't shift after the first write).

**Standard component scenarios** (mirror `checklist_runner_test.go`): conformance `var _ Prompt[[]TodoItem]`; fresh navigate-then-act asserts the acted position; cursor clamp down/up; empty-list no-ops; mid-session error → `RunPrompt` err is nil, `browser.Err()` is the fake's error, `ok=false`; the three quit keys (`q`/`esc`/`ctrl+c`) → `ok=false`, no error, callback never fired; empty-`Title` row renders a non-blank fallback.

**CLI-layer scenarios** (`todo_test.go`, stub the `isInteractive` package var):
- Dispatch matrix: `isInteractive=false` + no flags → classic output (the four protected tests already cover this); `isInteractive=true` + `--no-input` → guard error naming `--no-input`/terminal, no TUI escapes on stdout; `isInteractive=true` + `--json` → classic JSON, no TUI; each output-shaping flag with `isInteractive=true` → classic path.
- `buildTodoItems`: durable/ephemeral mapping, `Ref` values, order (durable-then-ephemeral), and all three empty-`Title` fallbacks.
- `makeMarkDoneFunc` persistence against a temp vault: a durable `Ref` flips `state:` to `done` on disk; an ephemeral `Ref` flips `[ ]`→`[x]` in `inbox.md`; a `repeat:` durable advances `scheduled:` and leaves `state:` `"open"` (assert via file read or fresh `--json`), with the item removed from the returned session slice.

## Known risks / ambiguities remaining

- **Recurring pile-up not live-visible** (strategy B): a completion materializing a new `inbox.md` item won't appear until the TUI is reopened. Accepted; documented in `makeMarkDoneFunc`.
- **Concurrent external modification** (another `rk todo done` while the TUI is open): in-memory session goes stale; not handled, same accepted limitation as fnqs.12 (AC §3).
- **No transient "advanced to <date>" feedback on recurring completion**: the item silently vanishes for the session. Acceptable representation; richer feedback is stretch-only (AC §2 [OPEN]).
- **"Recently-done" view** (ticket's "optionally"): not built — open-only default matches the Scope's first sentence and today's list default (AC §5). No residual [OPEN] on the dispatch or guard behavior — the matrix above is settled.

### Critical Files for Implementation
- /home/chadd/repos/reckon/.worktrees/reckon-6k0l/internal/tui/components/todo_browser.go (new — `TodoItem`, `MarkDoneFunc`, `TodoBrowser`)
- /home/chadd/repos/reckon/.worktrees/reckon-6k0l/internal/cli/todo_browse.go (new — `runTodoBrowse`, `buildTodoItems`, `makeMarkDoneFunc`)
- /home/chadd/repos/reckon/.worktrees/reckon-6k0l/internal/cli/todo.go (modify — dispatch branch in `runTodoListE`; write functions `doneDurableTodo`:647 / `doneEphemeralTodo`:902 reused)
- /home/chadd/repos/reckon/.worktrees/reckon-6k0l/internal/tui/components/checklist_runner.go (reference — structural template)
- /home/chadd/repos/reckon/.worktrees/reckon-6k0l/internal/cli/interactive.go (reference — `isInteractive` seam + `PromptGuard` wiring)
