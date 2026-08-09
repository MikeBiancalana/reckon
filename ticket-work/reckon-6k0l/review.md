# Code Review: reckon-6k0l — `rk todo list` interactive mini-TUI

**Verdict: APPROVE**

The implementation delivers every load-bearing design decision, faithfully mirrors the
`checklist_run.go`/`checklist_runner.go` reference with only the justified deviations, and is
backed by tests that check behavior rather than restating names. All `go vet` and `go test`
pass for `internal/cli` and `internal/tui/components`. The three preflight error-wrapping
warnings are cosmetic and their suggested fix would *regress* the messages (see §Error handling);
no code change is required.

## Certification of the 7 load-bearing design decisions

| # | Decision | Verdict | Evidence |
|---|---|---|---|
| 1 | Dispatch never reaches `RunPrompt` on any non-TTY/flagged path (byte-identical guarantee) | CONFIRMED | `todoListWantsTUI` (todo_browse.go:19-32) returns false on `!isInteractive()`, on any of `all/state/durable/ephemeral` `Changed`, or `jsonFlag/ndjsonFlag`. The classic body (todo.go:445-491) is textually unchanged; the branch (todo.go:441-443) is the only insertion. Only bare+TTY enters `runTodoBrowse`. |
| 2 | Local-session refresh (no mid-session re-query) is safe because ephemeral line-index `Ref`s never drift | CONFIRMED | `flipChecklistLine` (todo.go:998-1013) mutates one byte in place, never removing/reordering lines. `splitChecklistLines` (read) and `checklistMarkRe` (write) both index over *all* checkbox lines in file order, so the two agree. Even the recurring pile-up path is safe: `addEphemeralTodo` (todo.go:383-432) appends at EOF, so a mid-session materialization cannot shift an already-captured `Ref`. |
| 3 | Recurring (`repeat:`) todos: `doneDurableTodo`→`doneRecurringTodo` returns non-error with `state:"open"`; closure still removes from session and does not misread it as failure | CONFIRMED | `doneRecurringTodo` (todo.go:777-857) returns `(res, nil)` with `State:"open"`. `makeMarkDoneFunc` (todo_browse.go:136-149) discards the result value, checks only `err`, and removes on any non-error — preventing the double-advance hazard. Test `TestMakeMarkDoneFunc_RecurringAdvancesWithoutDoneState` (todo_test.go) asserts `scheduled` advanced, `state` stayed `open`, item removed. |
| 4 | Durable vs. ephemeral dispatch keyed on `Kind`/`Ref`, never raw position | CONFIRMED | `makeMarkDoneFunc` dispatches on `it.Kind` (todo_browse.go:139-143); `pos` only indexes the local session slice to fetch the item, and the *captured* `Ref` (ULID or line index) drives the write. No position-based file lookup for durable; ephemeral's line-index `Ref` is the stable captured value, not the slice position. `TestTodoBrowser_HeterogeneousCursorToRefDispatch` proves the ephemeral row's own `Kind`/`Ref` reaches the callback. |
| 5 | Mid-session `MarkDoneFunc` errors surface via `Err()` → wrapped error, no swallow/panic | CONFIRMED | `Update` stores `r.err` and breaks (todo_browser.go:102-104); `Done()` reports `canceled` when `r.err != nil` (todo_browser.go:69) so the host quits with `ok=false` and `RunPrompt` returns `(zero,false,nil)`; `runTodoBrowse` then wraps `browser.Err()` (todo_browse.go:53-55). `TestTodoBrowser_MidSessionError` asserts `RunPrompt` err is nil while `Err()` carries the cause. |
| 6 | Preflight's 3 unwrapped-error warnings | COSMETIC — do not apply the suggested fix | See §Error handling below. |
| 7 | No bubbletea closure-capture bug in `Update` | CONFIRMED | `Update` (todo_browser.go:79-117) returns `(r, nil)` — no `tea.Cmd` closures, no goroutines, so the REVIEW_PATTERNS async-capture class cannot occur. `View`'s `range` uses `item` synchronously. `makeMarkDoneFunc`'s captured `session` is mutated only under bubbletea's single-goroutine `Update`, and it returns a fresh copy each call so component and closure never alias the same backing array after the first action. |

## AC conformance (checked against acceptance-criteria.md, not the plan's paraphrase)

- §1.7 component decoupling: `todo_browser.go` imports only `fmt`, `strings`, `bubbletea` — no `node`/`index`. `Kind`/`Ref` are documented opaque. PASS.
- §1.8 no bespoke isatty in `todo.go`: dispatch reuses the shared `isInteractive()` seam only to *select the branch* (required because non-TTY must fall through, not error); the guard remains `PromptGuard`-via-`RunPrompt`. No `os.Stat` added. PASS.
- §2 flag set: `--all/--state/--durable/--ephemeral/--json/--ndjson` gate; `--quiet/--vault/--date/--log-*` deliberately do not (AC §2 lines 76-77). Implementation matches exactly. PASS.
- §2 refresh strategy (B): adopted, with the missed-pile-up live-visibility gap documented at todo_browse.go:130-133. PASS.
- §4 empty-list-opens-TUI: AC §4 (lines 216-217) specifies opening the browser rendering `(no items)`; implementation and `TestTodoBrowser_EmptyListNoOps` conform — this is spec, not an open UX question. PASS.
- §4 regression: the four protected non-TTY tests (todo_test.go) are untouched and pass (`isInteractive()` reports false under `go test`). PASS.

## Findings

### Error handling — preflight warnings are cosmetic; the recommended fix regresses (category: error-wrapping)

The three flagged sites return errors that **already carry a `todo list:` prefix**:

- todo_browse.go:42-44 (`buildTodoItems`) — every error it can return is pre-wrapped: `index.Open`→`"todo list: open index"` (:69), `Reconcile`→`"todo list: reconcile index"` (:74), or the list-fn errors below.
- todo_browse.go:77-79 (`listDurableTodos`) — that function wraps internally, e.g. `"todo list: query durable nodes: %w"` (todo.go:500).
- todo_browse.go:81-83 (`listEphemeralTodos`) — wraps internally, e.g. `"todo list: query ephemeral container: %w"` (todo.go:591).

Applying the preflight's suggested `fmt.Errorf("todo list: %w", err)` / `fmt.Errorf("todo list: list todos: %w", err)` would produce redundant prefixes like `todo list: list todos: todo list: query durable nodes: …`. **Recommendation: leave as-is.** The current code carries full context without duplication. No action required.

### Testing — end-to-end TUI dispatch coverage gap [INFERRED, acceptable]

No test drives `isInteractive=true` + no flags + no `--no-input` all the way through a live
`tea.Program`, because that requires a PTY. Mitigations already in place: the dispatch→`runTodoBrowse`→`RunPrompt`→`PromptGuard` wiring *is* exercised end-to-end via `TestTodoList_Interactive_NoInputErrors` (short-circuited at the guard before a Program opens); `buildTodoItems`, `makeMarkDoneFunc`, and the component (via `RunPrompt` with scripted input) are all unit-covered. This matches the checklist precedent and the plan's "no test-only program-options global" stance. Not a defect.

## Positive observations

- Remove-on-action (todo_browse.go:147) is a cleaner resolution of AC §2's [OPEN] recurring-visual question than the AC's tentative "no visual Done flip" — it collapses the open-todo semantics, the recurring double-advance hazard, and the one-directional-completion constraint into one rule, and is explained inline (todo_browse.go:121-133).
- `View` builds lines via a slice + `strings.Join` (todo_browser.go:124-152), correctly avoiding the phantom-blank-line pitfall from REVIEW_PATTERNS.
- Test identity assertions record `(position, kind, ref)` per call (`fakeMarkDoneCall`), so heterogeneous-dispatch and ref-stability tests prove the *intended row* was hit, not merely "a" row.
- `buildTodoItems` closes the index before the interactive session (defer at todo_browse.go:71), so no SQLite handle is held across the TUI — the write path takes `vaultDir` directly.

## Questions for consideration (non-blocking)

- Bare `rk todo list` + `--quiet` on a real TTY opens the TUI (`--quiet` is intentionally not in the gate set, per AC §2). This is spec-compliant and `--quiet` has never affected classic `todo list` output, so behavior is unchanged for the only case that mattered (scripts, which are non-TTY). Flagging only for visibility; no change recommended.
