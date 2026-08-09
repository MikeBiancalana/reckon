# Codebase Analysis: reckon-6k0l — `rk todo list` interactive mini-TUI

Reference implementation: reckon-fnqs.12 (PR #165, merged), `internal/tui/components/checklist_runner.go` +
`internal/cli/checklist_run.go`. Read those two files plus `ticket-work/reckon-fnqs.12/{plan,review}.md`
directly — this doc only states where reckon-6k0l's shape must diverge and why.

## 1. The discriminator: this ticket overloads an existing verb, fnqs.12 did not

fnqs.12 added a brand-new verb (`checklist run`) with zero existing non-interactive callers, so it could
unconditionally attempt the TUI and let the guard reject non-TTY (`checklist_run.go:13-14,42`). reckon-6k0l
reuses `todo list` (`internal/cli/todo.go:96-102`), which today has real non-interactive consumers (scripts
piping `rk todo list`). There is **no existing bare-vs-flagged branching precedent in this codebase** to
copy: reckon-fnqs.7 (the ticket cited as prior art for "bare invocation gets a wizard") is still `OPEN`
(`bd show reckon-fnqs.7`), not merged — `rk todo add` has no wizard today. fnqs.12's separate-subcommand
model is the only shipped precedent, and it sidesteps this exact problem by not sharing a verb.

Required three-way split inside `runTodoListE` (`todo.go:438`):
- **flagged** (any of `--all/--state/--durable/--ephemeral` changed, registered `todo.go:121-125`) → today's
  exact code path, unchanged, regardless of TTY.
- **bare + TTY** → open the mini-TUI.
- **bare + non-TTY / `--no-input`** → error via the shared guard (not a silent fallback to text output).

**[OPEN, decide in plan]** Whether `--json`/`--ndjson` (global persistent flags, `root.go:96-97`) also count
as "flagged." If they don't, a bare non-interactive script with no other flags has **no way to get list
output without hitting the interactive guard's error** — `--json` becomes the forced non-interactive escape
hatch and should probably be added to the "any flag changed → old path" check alongside the four
list-specific flags. This is a consciously-accepted breaking change either way: `rk todo list | cat` in a
script that previously printed Pretty text will now error unless the script adds a flag. Own this in the
plan; fnqs.12's review doesn't have anything analogous because `checklist run` never had non-interactive
callers to break (fnqs.12's own accepted wart, `review.md` finding 1, is unrelated — a phantom DB row, not a
breaking output change).

## 2. Write path — call these, not a new one

`rk todo done <ref>` dispatches on `--ephemeral`:
- Durable: `doneDurableTodo(vaultDir, ref) (todoDoneResult, error)` — `todo.go:647`. Resolves `ref` via ULID
  fast path or alias walk, then calls `completeDurableTodoNode(vaultDir, n, foundPath, ref, false, true)`
  (`todo.go:670`) — the two trailing bools (`logDid=false, recurLogDid=true`) are **hardcoded by this call
  site**, so calling `doneDurableTodo` from the TUI toggle inherits `rk todo done`'s exact side effects
  (no did-log entry on a plain completion; recurring branch always logs) with no extra wiring.
- Ephemeral: `doneEphemeralTodo(vaultDir, ref) (todoDoneResult, error)` — `todo.go:902`. `ref` is a **1-based
  line-index string** (`strconv.Atoi`), not a ULID — a completely different ref namespace from durable.

**These are the two functions the toggle closure must call** — mirrors `makeToggleFunc` calling
`checkAndRefetchRun` (`checklist_run.go:68-76`, itself `checklist.go:486-491`). No new write path.

### Toggle is one-way, not a true toggle — the single biggest departure from ChecklistRunner

`checklist.Service.CheckItem` (`internal/checklist/service.go:147-184`) genuinely flips
`Checked` both directions every call (`newChecked := !item.Checked`, line 158) — that's what makes
`ChecklistRunner`'s space-bar semantics ("toggle") correct. Todo has **no such verb**: grep for
`reopen`/`undone`/state-back-to-`"open"` in `todo.go` finds nothing. `doneDurableTodo`/`doneEphemeralTodo`
only mark done, and are idempotent no-ops (`Skipped: true`, no error) if already done (`todo.go:706-710`,
`todo.go:924-926`). There is no way to un-mark an item from inside `rk todo done`'s verb layer, so the
mini-TUI's "toggle" key can only ever call "mark done" — pressing it on an already-done item is a harmless
no-op, not a reversal. Name the callback type accordingly (`MarkDoneFunc`, not `ToggleFunc`) or document the
asymmetry loudly if keeping the name for API-shape parity with `ChecklistRunner`.

A durable todo with a `repeat:` prop takes `doneRecurringTodo` (`todo.go:773`) instead of the plain
state-flip: it advances `scheduled` and **state stays `"open"` forever** (`todo.go:691` comment, `818`) — a
recurring item can never show `Done: true` after being "completed." The TUI must decide what visual feedback
a toggle on a recurring item gives (it will not check off); flag as **[OPEN]** for the plan.

`ChecklistRunner` auto-completes and exits when the callback reports `completed=true`
(`checklist_runner.go:67-69,106`). **TodoBrowser has no analogous terminal state** — marking the last open
item done doesn't "finish" a browsing session; `Done()` can only ever report `canceled=true` on a quit key,
`finished` should presumably never fire (or the component's `T` can be `struct{}`/unused). This is allowed by
the `Prompt[T]` contract (`finished`/`canceled` need not both be reachable) but is a real behavioral
difference from the reference to call out, not silently copy.

## 3. Data source: raw SQL against the index DB, not a query-layer abstraction

`rk todo list` does **not** go through any `internal/index` query API — it queries `ix.DB()` (the raw
`*sql.DB`) directly after `ix.Reconcile()` (`todo.go:460-469`). Two independent paths populate
`todoListResult.Items` (`todo.go:174`):

| Function | Source table(s) | Fields available |
|---|---|---|
| `listDurableTodos` (`todo.go:493`) | `nodes` (`id,body,title` WHERE `type='todo'`), `node_props` (`loadTodoProps`, `todo.go:548`), `edges` (`loadDependsOn`, `todo.go:568`, rel=`depends-on`) | ID, Body, Title, State (`props["state"]`), Scheduled, Deadline, Depends, Repeat |
| `listEphemeralTodos` (`todo.go:580`) | `nodes` (single row, `type='todo-ephemeral'`) body, split via `splitChecklistLines` (`todo.go:973`) | Container path, Line (stable 1-based, file order), Checked, Body text |

Existing display type `todoListItem` (`todo.go:156-170`) already carries every field a `TodoItem` would need
— `Kind`, `ID`/`Line`, `State`/`Checked`, `Body`, `Title`. **`TodoItem` needs more than the ticket's
illustrative `{ID, Title, Done}`**: durable and ephemeral use disjoint ref namespaces and disjoint write
functions, so the display type (or the CLI-layer mapping around it) must retain a `Kind` discriminator so
the toggle closure knows whether to call `doneDurableTodo` or `doneEphemeralTodo`. `[INFERRED]` shape:
`TodoItem{Kind, ID string /* ULID or line-index-as-string, used as the ref */, Title string, Done bool}`.

**Refresh-after-toggle is not the same problem checklist solved.** Checklist's `ToggleFunc` re-fetches via
one SQLite row (`GetRunStatus`, `service.go:187`) because runs live entirely in the operational DB. Todo's
writes go straight to vault files, bypassing the sqlite index — getting a fresh read requires either (a)
`ix.Reconcile()` again per toggle, which walks/hashes the whole vault (`reconcile.go:87`, cost scales with
vault size per keypress), or (b) patch just the touched item in-memory using the `todoDoneResult` the verb
call already returned (Path/State/Scheduled), skipping a full index round-trip. **[OPEN]**, but (b) is
clearly cheaper and matches "every mutation stays observable... survives `rk index --rebuild`" (ticket) since
the *file* write is authoritative either way — the in-memory patch is purely a display optimization, not a
correctness requirement.

## 4. List-shrink pitfalls unique to todo (ChecklistRunner never faced these)

`ChecklistRunner`'s cursor clamp (`checklist_runner.go:87-94`, `cursor < len(items)-1` / `cursor > 0`) only
ever operates on a **fixed-length** list — `CheckItem` flips `Checked` in place and never removes a row, so
`len(r.items)` never changes across a session. Todo's default list view excludes done items
(`todo.go:525`, `!all && state != "open" && state != "in-progress"` → skip). **If the mini-TUI mirrors that
default** (show open only), marking an item done removes it from the visible slice mid-session — a
genuinely new class of bug the reference component's tests never had to cover:

- `docs/REVIEW_PATTERNS.md:957-978` "Index-Only Selection Identity After Re-Sort": clamping cursor by
  numeric index after a shrink silently re-targets a different item; must track the previously-selected
  item's ID and re-locate it (or move to the next remaining item) instead of just `min(cursor, len-1)`.
- `docs/REVIEW_PATTERNS.md:980-997` "Scroll Offset Not Clamped on List Shrink": any scroll/viewport offset
  must be reclamped after the backing slice shrinks, or the render loop shows zero items despite items
  existing.

**[OPEN, decide in plan]** Whether the browser removes done items from view on toggle (triggers both
pitfalls above, needs new handling `ChecklistRunner` doesn't have) or keeps them visible with a checked mark
like `--all` (list stays fixed-length per session, and the existing `ChecklistRunner` clamp logic at
`checklist_runner.go:87-94` can be copied verbatim). The latter is far cheaper to get right; "optionally
recently-done" in the ticket's Scope reads as compatible with keeping done items visible by default within a
session even if the initial fetch excluded them.

## 5. `Prompt[T]` / `RunPrompt[T]` contract (unchanged from fnqs.6, use directly)

```go
// internal/tui/components/prompt.go:9-21
type Prompt[T any] interface {
    Init() tea.Cmd
    Update(tea.Msg) (Prompt[T], tea.Cmd)
    View() string
    Result() T   // meaningful only once Done() reports finished
    Done() (finished, canceled bool)
}
```
`RunPrompt[T any](p Prompt[T], opts ...tea.ProgramOption) (result T, ok bool, err error)` (`prompt.go:78-99`):
checks `components.PromptGuard` first (nil-safe, `prompt.go:39,79-84`), opens exactly one `tea.Program`,
returns `ok=true`+`Result()` on finish, `ok=false`+zero value on cancel, non-nil `err` only for
guard/Program-level failures (never for a mid-session domain error — see `ChecklistRunner.Err()` pattern,
`checklist_runner.go:75`, for how fnqs.12 kept a toggle failure out of `RunPrompt`'s own error return).

`Wizard`/`Step[T]` (`wizard.go`) chain **heterogeneous** multi-step flows into one `Prompt[map[string]any]`
— not applicable here. A todo browser is one screen (list + toggle), so it implements `Prompt[T]` directly
and is driven via a single `components.RunPrompt[T](browser)` call, exactly like
`checklist_run.go:42` — no `Wizard` involved.

### TTY guard — reuse verbatim, do not add a bespoke check

`components.PromptGuard` is wired once at `interactive.go:39-41` to `promptGuard` (`interactive.go:32-37`),
which checks `noInputFlag` (`root.go:32`, registered `root.go:99`) `|| !isInteractive()`
(`interactive.go:16-18`, both stdin and stdout must be a char device). `RunPrompt` invokes this
automatically before opening any `tea.Program` — `runTodoListE`'s bare branch needs **zero** isatty code of
its own, matching `checklist_run.go`'s comment at lines 13-14 ("TUI-only: non-TTY and --no-input are refused
by the shared components.PromptGuard, not a bespoke check here"). The only new code is the
flagged-vs-bare *branch selection itself* (§1), which happens before `RunPrompt` is even called — the guard
only fires once the bare path has already decided to attempt the TUI.

## 6. Reference component to mirror: `ChecklistRunner`, not `TaskPicker`/`NotePicker`

`ChecklistRunner` (`checklist_runner.go`, 156 lines) is the direct structural template: single-screen,
hand-rolled cursor (no `bubbles/list`), injected callback, `[]string` + `strings.Join` view build
(avoids the "Unconditional Newline Join with Optional Strings" pitfall, `REVIEW_PATTERNS.md:879-901` —
`ChecklistRunner`'s own `View()`, `checklist_runner.go:117-155`, is the positive example of this fix).
Copy: constructor + `Show(items, callback)` prime pattern (`checklist_runner.go:41-55`), `j/k/up/down` clamp,
`space/enter` → synchronous callback call capturing `pos := cursor` into a local before the call (avoids the
closure-capture-bug pattern, `REVIEW_PATTERNS.md:117-139`), `q/esc/ctrl+c` → `canceled=true` (never let
Ctrl+C fall through to bubbletea's default kill path, or `RunPrompt` returns a non-nil error on a plain user
quit — `checklist_runner.go:85`, `plan.md` "Critical for the no-error-on-quit contract").

`TaskPicker` (`task_picker.go`, 288 lines) and `NotePicker` (`note_picker.go`, 335 lines) are both
single-select pickers built on `bubbles/list` + `lipgloss` + fuzzy filtering — heavier machinery for a
different interaction (pick one and return, `Done()` fires on Enter) than toggle-in-place browsing. Worth a
glance only for the `IndexRow{ID, Title, Type, Props map[string]string}` display-type precedent
(`prompt.go:23-32`) the ticket's "display-type indirection" language refers to; not a structural template.

## 7. House conventions

No `internal/tui/AGENTS.md` exists. `internal/cli/AGENTS.md` (468 lines) predates the v1 rewrite — it
references `journal.Service`/`task.go`/`notes.go` naming not present in this codebase's actual CLI files;
treat its Cobra command examples as stale. Only its generic rules still apply and are already followed by
`todo.go`: return errors, never `os.Exit` (`AGENTS.md:167-179`); respect `--quiet`
(`AGENTS.md:378-388`, already done via `mode == output.Pretty && quietFlag` checks, `todo.go:323,636`); wrap
errors with an operation prefix (`AGENTS.md:187-194`, `todo.go` uses `"todo done: %w"` etc. throughout).

## 8. Open questions to resolve in plan.md (collected)

1. Does `--json`/`--ndjson` count as "flagged" for the bare/flagged split (§1)? Determines whether scripts
   have any non-interactive migration path.
2. Does the browser remove done items from view on toggle, or keep them visible checked (§4)? Determines
   whether the list-shrink pitfalls apply at all.
3. What visual feedback does toggling a recurring (`repeat:`) todo give, given it can never show `Done: true`
   (§2)?
4. Refresh strategy after a toggle: full `ix.Reconcile()` vs. in-memory patch from `todoDoneResult` (§3).
5. Command surface: does this stay inside `todo list`'s existing `RunE`, or is a separate helper file
   (`todo_browse.go`, mirroring `checklist_run.go`'s separate-file convention) preferred for cohesion even
   though it's not a separate cobra command?
