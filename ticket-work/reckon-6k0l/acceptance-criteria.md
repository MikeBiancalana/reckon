# Acceptance Criteria: reckon-6k0l — `rk todo list` interactive mini-TUI

Sources: `bd show reckon-6k0l/reckon-fnqs.6/.7/.12/reckon-fnqs`; `internal/cli/todo.go`,
`internal/cli/todo_test.go`; `internal/cli/checklist_run.go`, `internal/cli/checklist.go`
(fnqs.12 precedent, closed, PR #165); `internal/tui/components/{prompt,checklist_runner}.go`
(fnqs.6/.12); `internal/cli/interactive.go`, `internal/cli/root.go` (TTY guard + flag wiring);
`internal/index/{title,reconcile}.go`. Reference doc `ticket-work/reckon-fnqs.12/acceptance-criteria.md`
gives the shape; content below is reckon-6k0l-specific, not copied.

**This ticket diverges from the fnqs.12 template more than it resembles it.** Checklist's `run`
is a brand-new, TUI-only command with a homogeneous, `Service`-backed item list. `rk todo list`
is an *existing* dual-purpose command whose bare, zero-flag, non-interactive form is today's
primary agent/script entry point, and its item list is a heterogeneous merge of two storage
kinds with two different completion verbs, one of which (recurring) doesn't have a "done" state
at all. Sections 2–5 below are load-bearing; do not skim past them expecting a checklist-run repeat.

## 1. Explicit acceptance criteria

Pulled from the ticket's Scope/Design constraint/Done-when (quoted there verbatim; not restated
here beyond the numbering needed for test-name traceability):

1. Bare `rk todo list` (no args — `todoListCmd` already takes `cobra.NoArgs` — and no flags) on a
   TTY opens an interactive mini-TUI.
2. The mini-TUI lists open todos (durable + ephemeral, matching today's default merge — see §2 for
   whether "recently-done" is included).
3. The user can move a selection cursor over the list and toggle an item's done state.
4. Any flag (`--all`, `--state`, `--durable`, `--ephemeral`, and per §2 probably `--json`/`--ndjson`)
   keeps behavior byte-identical to today's `rk todo list` output — no TUI code reached.
5. Toggling done in the TUI calls the same verb path `rk todo done <ref>` uses
   (`doneDurableTodo`/`doneEphemeralTodo`, todo.go:647,902) — not a second write path.
6. Non-TTY or `--no-input` errors per the shared guard instead of hanging (§2 pins down exactly
   which invocations this applies to).
7. The component (e.g. `TodoBrowser`) does not depend on `*node.Node` or index-row internals
   directly; it operates over a small display type the verb layer maps into, e.g.
   `TodoItem{ID string; Title string; Done bool}`.
8. Built via `components.Prompt[T]`/`components.RunPrompt[T]` — no hand-rolled `tea.NewProgram`,
   no bespoke isatty check in `internal/cli/todo.go` (the shared `promptGuard`,
   interactive.go:32–41, covers it for free, same as fnqs.12).

## 2. Implicit requirements [INFERRED]

**The load-bearing open question, everything else here follows from it:** does bare `rk todo
list` on a non-TTY (a) hit the guard and error, or (b) fall through to today's plain-text output?
**[OPEN — do not resolve by guessing, flag to the ticket owner or pick (b) and document the
choice inline at the call site.]** Evidence is genuinely split, not settled by precedent:
- Toward (a): the "Done when" clause pairs "non-tty" with "`--no-input`" under one guard clause,
  mirroring fnqs.12's `promptGuard()` (`noInputFlag || !isInteractive()`) verbatim; fnqs.12's own
  design constraint forbids a bespoke isatty check — but (b) requires exactly that (an early
  `isInteractive()` branch inside `runTodoListE` deciding whether to attempt `RunPrompt` at all),
  which is a new shape relative to `checklist run`'s single always-guarded path.
- Toward (b): four **existing, currently-passing** tests call bare `runTodo(t, vault, "list")`
  with zero flags and assert today's Pretty text output (todo_test.go:740, 772, 797, 899) — run
  inside `go test`, where `isInteractive()` already reports false (no real TTY). Interpretation
  (a) makes all four start failing (or requires rewriting them to stub `isInteractive`/add a
  flag, exactly as `checklist_test.go`'s `TestChecklistRun_GuardNonTTY` etc. already had to do for
  a *brand-new* command with no prior non-interactive contract). `rk todo list` bare already *is*
  the non-interactive contract for scripts/agents/CI; (a) is a breaking change fnqs.12 never had
  to make. The ticket's own parent framing ("non-interactive invocation stays byte-identical to
  today's output") reads more naturally as covering the *bare* case too, not just flagged calls.

  Recommendation if forced to pick: (b), with the TUI-attempt guarded only behind an explicit
  `isInteractive()` check made once in `runTodoListE` before ever calling `RunPrompt` — bare+TTY
  → attempt (subject to `RunPrompt`'s own guard as a second, redundant-but-harmless line of
  defense); bare+non-TTY → today's text path, no error; any flag → today's path unconditionally,
  regardless of TTY. This preserves all four existing tests unmodified and matches "byte-identical
  to today" literally. `--no-input` alone (no flags, real TTY) should still force the guard error
  (flag beats a real terminal, matching checklist's `TestChecklistRun_GuardNoInputFlag`).

**"Flagged" dispatch condition** [OPEN, downstream of the above]: which flags route to the
non-interactive path? `--all`/`--state`/`--durable`/`--ephemeral` (todoListCmd's own, todo.go:121–125)
certainly do. `--json`/`--ndjson` are root-persistent (root.go:88–89), shared by every command;
fnqs.12 explicitly left `--json` *not* gating `checklist run`'s guard (an accepted oddity, since
`run` has no text-output mode to fall back to). `todo list` is different: it already has a
meaningful `--json` output today, so a user/script passing `--json` almost certainly wants JSON,
not a TUI. Recommend `--json`/`--ndjson` also count as "flagged" here, unlike checklist run.
`--vault`/`--quiet`/`--date`/`--log-*` alone should not by themselves suppress the TUI (they don't
request a specific output shape). Check via `cmd.Flags().Changed(name)` on the specific flag set
above, not `cmd.Flags().NFlag()` (which would also count unrelated persistent flags).

**Heterogeneous list, no single verb dispatch by position.** `ChecklistRunner`'s `ToggleFunc(pos
int)` works because one `Service.CheckItem(runID, pos)` call resolves any position. `todo list`
merges two storage kinds (todo.go:472–485): durable (ULID → `doneDurableTodo`, todo.go:647) and
ephemeral (1-based line index → `doneEphemeralTodo`, todo.go:902) — two different verb functions,
two different ref shapes. The minimal `TodoItem{ID; Title; Done}` the ticket suggests cannot
carry which verb a given position needs. The CLI-layer closure (mirroring `makeToggleFunc`,
checklist_run.go:68–76) must hold a parallel per-position dispatch table (kind + ref) built at the
same time as the `[]TodoItem` slice, and regenerate both together on every refresh so positions
never drift apart. `ToggleFunc`'s `completed bool` (checklist_runner.go:24) has no todo analog —
there is no aggregate "run complete" signal; drop it from the todo equivalent's return shape.

**Recurring todos never become "done."** A durable todo with a `repeat:` prop takes
`doneRecurringTodo` (todo.go:773), not the plain state→done branch: it advances `scheduled`,
**leaves `state` as `"open"`** (todo.go: comment at 690–693, 817–819), optionally writes a did::
audit entry, and — if intervals were missed — materializes a *new* ephemeral todo in
`todos/inbox.md` (todo.go:838–850). In a `TodoItem{Done bool}` view, toggling a recurring item is
visually a no-op: `Done` never flips true, and the item stays in the "open" list under the same
title. This is a real UX gap, not a bug to paper over — flag it explicitly rather than silently
assuming `Done=true` after every successful toggle call. No requirement in the ticket text
resolves this; recommend the toggle path treats `todoDoneResult.Recurred==true` as "no visual
Done flip, optionally show a transient 'advanced to <date>' message," but this is [OPEN].

**"Toggle" is one-directional, not bidirectional like checklist's check/uncheck.** There is no
`rk todo undone`. `doneDurableTodo`/`doneEphemeralTodo` on an already-done item return
`Skipped: true` (todo.go:706–710, 924–926) — an idempotent no-op, not a reversal. If a
done/recently-done item is ever visible and toggled again, the correct behavior is "nothing
changes, no error, no double did-entry" — not an uncheck.

**Index staleness after toggle — a correctness landmine absent from checklist.** `rk todo list`
reads `todos` from the SQLite index after calling `ix.Reconcile()` (todo.go:466–468); `done*`
verbs write straight to markdown files (`writeFileAtomic`) and never touch the index. Checklist's
`Service` reads and writes the same DB-backed store, so `GetRunStatus` after `CheckItem` is
automatically fresh — that guarantee does **not** carry over. If the toggle callback naively
re-queries `listDurableTodos`/`listEphemeralTodos` against the already-open `*index.Index` without
calling `ix.Reconcile()` again first, it returns pre-toggle (stale) state. Two viable designs,
resolve during implementation:
  - (A) Re-run `ix.Reconcile()` + full re-list on every toggle keypress (correctness-complete,
    mirrors checklist's re-fetch-the-world pattern, surfaces newly-materialized pile-up items
    live) at the cost of a reconcile transaction (index/reconcile.go:87, takes a lock) per
    keystroke.
  - (B) Patch just the toggled item's `Done` field locally from the `todoDoneResult` the verb call
    already returns (todo.go:215–230), never touching the index again mid-session. Cheaper, but a
    recurring toggle's pile-up materialization (new inbox.md item) won't appear until the TUI is
    reopened.
  Recommend (B) for the common case, since per-item state is already known from the verb's own
  return value and a full reconcile per keystroke is needless I/O; note the missed-pile-up
  live-visibility gap explicitly if choosing it.

**List reshaping vs. cursor, and the "optionally recently-done" ambiguity are one decision, not
two.** The ticket's Scope says "list open (and optionally recently-done) todos" — optionally is
doing real work here; flag this explicitly per the task's own instruction, don't build it as a
hard requirement. The two paths have inverted cost:
  - Hide done items (matches today's default `--all`-less filter, todo.go:525): the visible slice
    *shrinks* after a successful toggle, the cursor must reclamp (mirrors `ChecklistRunner`'s
    up/down clamp, checklist_runner.go:88–94, but here length changes *during* the session, which
    checklist's fixed-size run never does), and the CLI's parallel dispatch slice (kind+ref) must
    be rebuilt in lockstep. More moving parts, but zero new display-type surface.
  - Show recently-done too (optional, per ticket text): list stays a stable size, toggling just
    flips a row's `Done` flag in place — this is the *cheaper*, closer-to-`ChecklistRunner` path,
    the inverse of what "optional" might suggest at first glance.
  Recommend hiding done items by default (matches "open todos" in the Scope's first sentence and
  today's list default) and treating "recently-done" as the stretch goal it's phrased as.

## 3. Edge cases to handle

- **Empty todo list** (no open durable or ephemeral items). Mirror `ChecklistRunner`'s `(no
  items)` render (checklist_runner.go:127–129); navigation/toggle keys are no-ops, no crash, no
  verb call, quit still exits cleanly.
- **List becomes empty mid-session** (last open item gets toggled done, and done items are
  hidden per §2). Must transition cleanly into the same empty-state render, not index out of range.
- **Cursor clamp after a toggle shrinks the list** (only if hide-done is chosen, §2): cursor at
  the last index of a 3-item list that just became a 2-item list must clamp to index 1, not stay
  at the stale index 2.
- **Todo with no title / malformed node.** `deriveTitle` (index/title.go:8–15) returns `""` when a
  durable todo's body has no non-blank line — legitimate for a hand-edited or migrated file, since
  `rk todo add`'s own validation (todo.go:279–281) already prevents this at creation time. An
  empty `Title` must not render a blank/invisible cursor row; fall back to something (ID, or a
  literal placeholder) rather than an empty string. Similarly an ephemeral checkbox line with no
  text after `- [ ] ` (`splitChecklistLines`' capture group 2 can be `""`, todo.go:977–987) needs
  the same fallback since ephemeral rows have no separate title field to prefer instead.
- **Toggling an item that fails mid-session** (file deleted/corrupted between list-time index read
  and toggle-time direct file re-read — `doneDurableTodo`'s fast path re-reads+re-parses the file
  fresh, todo.go:647–671, independent of the index). Mirror `ChecklistRunner`'s `r.err`/`Done()`
  canceled-with-error handling (checklist_runner.go:66–75, 100–104): surface the error, don't crash,
  don't silently swallow it.
- **Toggling a recurring todo** (see §2): does not flip `Done`; may materialize a new ephemeral
  item elsewhere in the vault that this session's view won't show without a full re-list (§2
  design choice (B)).
- **Toggling an already-done item** (only reachable if "recently-done" is shown, §2): idempotent
  skip, no error, no double did-entry (todo.go:706–710/924–926's `Skipped: true` path).
- **`--no-input` combined with a real TTY**: guard still fires (flag wins), same as
  `TestChecklistRun_GuardNoInputFlag`.
- **Only one of stdin/stdout is a TTY**: `isInteractive()` requires both (interactive.go:16–18);
  either alone non-tty triggers the guard/fallback per whichever of §2's two behaviors is chosen.
- **Terminal resize mid-session**: no special handling, same as `ChecklistRunner` precedent
  (no `tea.WindowSizeMsg` case) — note explicitly, don't silently omit.
- **Very long title text**: no wrapping/truncation, natural terminal line-wrap applies — same
  precedent as `ChecklistRunner`'s item text.
- **Concurrent modification** (another process/terminal runs `rk todo done <ref>` while the TUI is
  open): not handled — in-memory view goes stale until this session's own next toggle round-trips.
  Same accepted limitation checklist's TUI carries; don't attempt to fix here.
- **Mixed-kind list ordering stability**: durable items and ephemeral items are queried
  independently (todo.go:472–485) and appended durable-then-ephemeral; the CLI's per-position
  dispatch table (§2) must be built from that exact same concatenation order every refresh, or
  toggling position N after a partial refresh could call the wrong verb on the wrong ref.

## 4. Test scenarios (given/when/then)

### Interactive TUI path

- Given a vault with 2 open durable todos and 1 open ephemeral todo, when `rk todo list` runs bare
  on a TTY, then the TUI opens listing all 3 with the cursor on index 0.
- Given the TUI open with cursor at item 0 of 3, when the user presses `j`/Down three times, then
  the cursor advances to 1, then 2, then clamps at 2 on the third press (no wraparound).
- Given the TUI open with cursor at the last item, when the user presses `k`/Up repeatedly, then
  the cursor clamps at 0.
- Given the cursor is on an open durable (non-recurring) todo, when the user presses Space/Enter,
  then `doneDurableTodo` is called with that item's ULID, the file's `state:` prop flips to
  `done` on disk, and the view reflects it (checked, or removed, per §2's hide/show decision).
- Given the cursor is on an open ephemeral todo, when the user toggles it, then
  `doneEphemeralTodo` is called with that item's 1-based line index and the corresponding
  checkbox in `todos/inbox.md` flips to `[x]` on disk.
- Given the cursor is on a durable todo carrying a `repeat:` prop, when the user toggles it, then
  `doneRecurringTodo` runs (the file's `scheduled:` prop advances, `state:` stays `open`), and the
  item's `Done` does **not** flip true in the view — pin the exact visual treatment down per §2's
  [OPEN] item before writing this test, not after.
- Given a recurring todo with 2 fully-elapsed intervals, when the user toggles it, then a new
  ephemeral item is materialized in `todos/inbox.md` (todo.go:838–850) — assert its presence via a
  direct file read or fresh `rk todo list --json`, not necessarily via this session's live view
  (§2 design choice (B) does not require live visibility).
- Given the cursor is on an item and the toggle's underlying verb call errors (e.g. file removed
  out from under the session), when the user presses toggle, then the TUI surfaces the error
  (mirroring `ChecklistRunner`'s `Err()`/canceled-with-error) without crashing, and no further
  navigation is expected to recover mid-session.
- Given a todo whose derived title is empty (blank-body file), when it's listed in the TUI, then
  its row renders a non-empty fallback, not a blank/invisible line.
- Given a vault with zero open todos, when the TUI opens, then it renders a "no items" state;
  navigation/toggle keys are no-ops; quitting exits cleanly.
- Given the TUI open with some items toggled, when the user quits via `q`, `Esc`, or `Ctrl+C`,
  then the TUI exits and every toggle made during the session is already durable on disk (three
  cases, mirroring `TestChecklistRunner_QuitQ/QuitEsc/QuitCtrlC`).

### Non-interactive / flagged path (regression coverage — these 4 already exist and must keep passing)

- Given `TestTodoList_Pretty_RendersTitleOnly` (todo_test.go:740) calls bare `runTodo(t, vault,
  "list")` with zero flags in a non-TTY test process, when this ticket ships, then it still passes
  unmodified — resolving §2's [OPEN] guard question in favor of "bare+non-TTY falls through to
  text output" is a hard requirement of not breaking this test, not just a nicety.
- Same for `TestTodoList_Pretty_ShowsMetadata` (772), `TestTodoList_Pretty_NoMetadataOmitsAnnotations`
  (797), `TestTodoList_MultiLineBody_RoundTripByteIdentical` (899).
- Given any of `--all`/`--state`/`--durable`/`--ephemeral`/`--json`/`--ndjson` is passed, when
  `rk todo list` runs (TTY or not), then output is byte-identical to pre-ticket output — assert
  against the existing flagged tests (todo_test.go:493 onward) unmodified.
- Given items were toggled via the TUI in one session, when a later `rk todo list --json` runs,
  then the reported state matches what the TUI last displayed for non-recurring items (recurring
  items: `scheduled` advanced, `state` still `open` — assert the advance, not a `done` state).
- Given a durable todo is completed via `rk todo done <ref>` directly (never touching the TUI),
  when `rk todo list`'s TUI is later opened, then it reflects that completion (item absent or
  shown done per §2) — confirming both entry points write through one path with no divergence.

### TTY-guard / dispatch-boundary path

- Given stdout is piped or stdin is redirected, when `rk todo list` runs with **no flags**, then
  behavior matches whichever of §2's two resolutions was chosen — if (a): non-zero exit naming
  `--no-input`/needing a terminal, no hang, no TUI escape sequences reach stdout (mirroring
  `TestChecklistRun_GuardNonTTY`); if (b): today's plain-text output, unchanged, no error.
- Given a real TTY, when `rk todo list --no-input` runs (no other flags), then it errors the same
  way regardless of (a)/(b) above — the flag always wins over a real terminal (mirroring
  `TestChecklistRun_GuardNoInputFlag`).
- Given a real TTY, when `rk todo list --json` runs (no other flags), then it must NOT open the
  TUI — outputs JSON exactly as before (resolving §2's "does `--json` count as flagged" question:
  yes, unlike `checklist run`).
- Given the guard fires (assuming (a) is chosen for the bare+non-TTY case), when the error
  surfaces, then it comes from `components.PromptGuard`/the shared `promptGuard()`
  (interactive.go:32–41) — not a second, `todo.go`-local isatty check.

## 5. Explicitly out of scope

- **`rk todo add`'s create wizard (reckon-fnqs.7).** The ticket's Problem section references it as
  prior art ("todo add gets a create wizard from fnqs.7"), but **fnqs.7 is still OPEN, not
  shipped** (`bd show reckon-fnqs.7`) — `runTodoAddE` (todo.go:269) has no wizard/TTY branch
  today. reckon-6k0l depends only on fnqs.6 (closed), not fnqs.7. Do not build or assume any
  todo-add wizard exists or needs touching; this ticket is `list` only.
- **`rk todo show`/`rk todo edit`** (reckon-fnqs.4, P2, open) — a separate, unstarted ticket;
  no overlap expected but don't preempt its scope by adding show/edit affordances to the browser.
- **Composable board / pluggable panes** (reckon-fnqs.10, P3, open) — mounting a todo browser as a
  pane inside `rk tui` (fnqs.8, shipped) is later work; this ticket only wires the standalone
  `rk todo list` verb, same carve-out fnqs.12 made for checklist.
- **New flags on `add`/`done`.** Out of scope; those verbs are unchanged. `done`'s `--ephemeral`/
  `--author` flags are not otherwise reachable from the TUI (no way to pass a custom author from
  within the browser — falls back to `$RECKON_AUTHOR`/`$USER`/`"local"` same as `done` without
  `--author`, todo.go:252–263) — acceptable, not a gap this ticket needs to close.
- **Un-doing / unchecking a done item from the TUI.** No `rk todo undone` verb exists; per §2,
  "toggle" here is one-directional. Do not build a reversal that the CLI verb surface can't
  express.
- **Multi-vault / cross-vault browsing, filtering/search within the TUI, or sorting controls.**
  Not mentioned in Scope; the browser mirrors whatever `runTodoListE`'s existing default query
  already returns, in existing order.
- **Live re-indexing / file-watch for concurrent external edits.** Same accepted limitation as
  fnqs.12's checklist TUI; not handled, not required here.
- **Showing "recently-done" todos.** Per ticket text this is explicitly "(optionally)" — not a
  hard Done-when requirement. Flag before over-building it; §2 discusses why it's actually the
  *cheaper* of the two design paths despite reading as the stretch goal.
