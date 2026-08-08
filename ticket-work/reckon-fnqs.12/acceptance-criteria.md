# Acceptance Criteria: reckon-fnqs.12 — `rk checklist run` interactive mini-TUI

Sources: `bd show reckon-fnqs.12/.11/.6/.7/reckon-fnqs/reckon-yk1i`; `internal/checklist/{model,service}.go`;
`internal/cli/checklist.go` (fnqs.11, closed); `internal/tui/components/{prompt,wizard,task_picker,note_picker}.go`
(fnqs.6, closed); `internal/cli/interactive.go` (TTY guard wiring); git history of the deleted v0 TUI
(`git show 9a4e4ed:internal/cli/checklist_run_helper*.go`, commit message trailer, PR #144).

## 1. Explicit acceptance criteria

1. Bare `rk checklist run <template>` with stdin and stdout both attached to a real TTY opens an interactive
   mini-TUI.
2. The mini-TUI resumes the template's active run if one exists (`Service.GetActiveRun`); otherwise it starts a
   fresh run (`Service.StartRun`) — same resolution order `runChecklistStartE` (checklist.go:410-437) already
   uses.
3. The mini-TUI lists the run's items with checkboxes; the user can move a selection cursor and toggle an
   item's checked state.
4. The mini-TUI shows overall completion state (progress / all-items-checked).
5. Checking an item off in the TUI persists via the same `checklist.Service` calls fnqs.11's verbs use
   (`Service.CheckItem`) — not a second, parallel persistence path.
6. Quitting the TUI persists whatever was checked during the session (see §2 on timing — nothing is lost or
   rolled back).
7. Invoking on a non-TTY (piped/redirected stdin or stdout) errors instead of hanging.
8. Invoking with `--no-input` errors instead of opening the TUI, even on a real TTY.
9. The TUI is built via `components.Prompt[T]` / `components.RunPrompt[T]` (optionally `Wizard`, though a
   single-screen checklist has no reason to chain steps) — no new hand-rolled `tea.NewProgram` call outside
   that host, and no bespoke isatty check in `internal/cli/checklist.go` (the shared `promptGuard` wired at
   `internal/cli/interactive.go:39-41` covers it for free).
10. The new component (`ChecklistRunner` or equivalent) does not reference `*checklist.Run` / `*checklist.RunItem`
    in its own type signature; it operates over a small display type, e.g.
    `ChecklistItem{Text string; Checked bool}`.
11. `internal/cli/checklist.go` (or a new sibling file, e.g. `checklist_run.go`) owns the mapping between
    `checklist.Service`'s real types and `ChecklistItem` in both directions — `internal/tui/components` must not
    import `internal/checklist`.

## 2. Implicit requirements [INFERRED]

- **Persistence timing is per-toggle, not batched at quit.** The deleted v0 model
  (`checklistRunModel.Update`, space/enter case) called `service.CheckItem(m.run.ID, m.cursor)` synchronously on
  every toggle keypress, immediately followed by `GetRunStatus` to detect auto-completion. `Service.CheckItem`
  itself is not transactional/batchable (`internal/checklist/service.go:147-184`) — it writes through and
  re-checks completion on every call. Given "checking items off *and* quitting* persists" (ticket wording) and
  this exact historical precedent, the natural design is: each toggle keypress fires the mapped `CheckItem` call
  immediately; "quitting persists" then just means nothing further needs to happen at exit, because everything
  toggled so far is already durable. [HIGH CONFIDENCE — grounded in git history, not just wording.]
- **Consequence for the "no `*checklist.Run`/`*RunItem` dependency" constraint**: since the component can't
  hold a `*checklist.Service` either (that would defeat the same decoupling goal — a `Service` method takes/
  returns the real domain types), live per-toggle persistence needs the verb layer to inject a callback (a
  plain func value, e.g. `func(position int) ([]ChecklistItem, bool /*completed*/, error)`) into the component
  at construction. The component calls this closure on toggle and re-renders from its return value; the CLI
  layer's closure is the only place holding `*checklist.Service`/`*checklist.Run`. **[OPEN]** — this is a new
  shape relative to all five existing `Prompt[T]` components (Form/TextEditor/DatePicker/TaskPicker/NotePicker
  are all pure collect-then-submit with zero side effects mid-flow); `ChecklistRunner` would be the first
  side-effecting-during-interaction component. Two viable alternatives exist and should be resolved during
  planning, not left ambiguous in the implementation:
  - (A) Live persistence via injected callback (matches v0's behavior and `CheckItem`'s semantics exactly;
    crash-safe — a `kill -9` mid-session loses nothing already toggled).
  - (B) Pure collect-then-submit: component only mutates local `[]ChecklistItem` state; `Result()` returns the
    final slice; the verb layer diffs against the initially-loaded state after `Done()` and calls `CheckItem`
    once per changed position. Matches the existing components' pattern more closely but risks losing all
    toggles if the process dies before quit, and can't auto-detect completion mid-session (see next point).
  - Recommend (A): it is what "the same Service calls fnqs.11 wires" persisting *live* most plausibly means, and
    only (A) supports auto-complete-and-exit deterministically without an extra full-slice diff step.
- **Auto-complete-and-exit** [INFERRED, carried over from yk1i, not restated verbatim in fnqs.12's text]: v0
  auto-quit the TUI the moment the last item was checked (`Service.CheckItem`'s side effect flips
  `RunStatusCompleted`), showing a completion message first. fnqs.12's scope text only says "see completion,"
  which is satisfiable either by auto-exit (v0's behavior) or by staying open with a visible "complete" banner
  until the user manually quits. **[OPEN]** — recommend keeping v0's auto-exit behavior for continuity, but flag
  it explicitly since the ticket doesn't pin it down.
- **`run` has no non-interactive mode of its own.** Re-reading "Done when" literally: non-tty/`--no-input`
  *errors*, full stop — it does not say `run` falls back to `start`-like flag-driven behavior the way fnqs.7's
  `todo add` does (bare → wizard, flagged → today's non-interactive path, *same verb* either way). Checklist's
  non-interactive surface already exists as separate verbs (`start`/`check`/`status`/`reset`/`abandon`,
  fnqs.11, shipped). So `checklist run` is TUI-only; the guard's job is simply refusal, not "equivalent
  flag-driven behavior on this same command." **[INFERRED, medium-high confidence]** — worth confirming with
  the ticket owner before implementation, since it changes whether `run` needs its own flag surface at all.
- **Command is new, name is `run`**, distinct from fnqs.11's already-shipped `start` (non-interactive
  resume-or-start, no TUI). Both will coexist; `start` is unchanged by this ticket.
- **Resume/fresh-start resolution reuses fnqs.11's exact logic** (`GetActiveRun` else `StartRun`), not a
  divergent third implementation — ideally by factoring `runChecklistStartE`'s inline resolve step into a
  shared helper both verbs call, though that's an implementation choice, not an acceptance criterion.
- **`--json`/`--ndjson` don't gate the guard.** `promptGuard()` (interactive.go:32-37) only checks
  `noInputFlag || !isInteractive()`. `rk checklist run <template> --json` on a real TTY would still open the
  TUI (an odd combination, but consistent with how the shared guard already behaves and will behave for every
  future wizard verb) — not a bug to fix in this ticket.

## 3. Edge cases to handle

- **Empty checklist (template has 0 items).** v0 handled this explicitly: view renders `(no items)`; toggle keys
  (space/enter) and navigation keys (j/k/arrows) are no-ops that neither error nor crash; quit still exits
  cleanly. Mirror this.
- **All items already checked at TUI-open time.** Cannot occur as a *resumed active* run — `Service.CheckItem`
  auto-transitions a run to `RunStatusCompleted` the moment its last item is checked, and `GetActiveRun` only
  ever returns runs in `RunStatusActive`. So "resume an active run that's already 100% checked" is not a
  reachable state; no special-case needed beyond correct auto-complete-and-exit handling (previous section).
- **Template not found.** `GetTemplate`/`GetActiveRun`/`StartRun` all surface
  `checklist template %q not found`. `run` must propagate this and exit non-zero without ever opening the TUI
  — same message fnqs.11's verbs already produce, no new wording.
- **No active run + fresh start.** Standard path; new run created via `StartRun`, TUI opens on it with every
  item unchecked and cursor at 0.
- **User quits mid-session without finishing** (q / Esc / Ctrl+C — v0 treated all three as cancel). Run stays
  `RunStatusActive` in storage; every item toggled during the session remains checked (already persisted per
  §2); no abandon side effect fires.
- **Terminal resize mid-session.** v0's model never handled `tea.WindowSizeMsg` (no case for it in `Update`);
  this is an inline, non-alt-screen program, so resize is left to normal terminal reflow. [INFERRED — no special
  handling required; note it explicitly rather than silently skip it, since fnqs.6's `TaskPicker`/`NotePicker`
  *do* take an externally-driven `SetWidth`, unlike this precedent.]
- **Very long item text.** No wrapping/truncation logic existed in v0 (plain string concatenation into the
  view, no `lipgloss.Width` constraint). Natural terminal line-wrap applies. [INFERRED — acceptable unless the
  ticket owner wants otherwise; flag, don't silently assume more polish was wanted.]
- **Toggling an item after the run has already auto-completed within the same session** (only reachable if
  auto-exit-on-completion is *not* implemented, see §2's [OPEN] item — e.g., user unchecks the item that just
  completed the run). `CheckItem` has no active-status guard on direct-by-ID calls and never reverts
  `RunStatusCompleted` back to `RunStatusActive` on an uncheck. This is inherited `Service` behavior, not a new
  bug to fix here — but it means the two [OPEN] items above (auto-exit; live-vs-batch persistence) interact:
  skipping auto-exit reintroduces this quirk into the TUI's reachable state space.
- **Concurrent modification** — another process (e.g. `rk checklist check` from a script) mutates the same
  run while the TUI is open in a different terminal. Not handled by v0 either (no file-watch/poll/lock
  mechanism); the TUI's in-memory item state goes stale until its own next toggle round-trips through
  `GetRunStatus`. Out of scope for this ticket; note as a known limitation, don't attempt to fix.
- **`--no-input` passed together with an actual TTY.** Guard must still fire (flag wins over an interactive
  terminal) — `promptGuard()`'s `noInputFlag ||` ordering already guarantees this.
- **Only one of stdin/stdout is a TTY** (e.g., `rk checklist run x | cat`, or `rk checklist run x < /dev/null`
  in a terminal). `isInteractive()` requires *both* to be char devices — either alone being non-tty is enough
  to trigger the guard.

## 4. Test scenarios (given/when/then)

### Interactive TUI path

- Given a template with 3 items and no active run, when `rk checklist run <template>` runs on a TTY, then a
  fresh run starts and the TUI opens with all items unchecked and the cursor on item 0.
- Given a template with an existing active, partially-checked run, when `rk checklist run <template>` runs on
  a TTY, then the TUI opens showing that run's existing state (resumed) — it does not start a second run.
- Given the TUI is open with cursor at item 0 of 3, when the user presses `j` (or Down) three times, then the
  cursor advances to item 1, then item 2, then clamps at item 2 (no wraparound, no error) on the third press.
- Given the TUI is open with cursor at the last item, when the user presses `k` (or Up) repeatedly, then the
  cursor clamps at item 0.
- Given the TUI is open with the cursor on an unchecked item, when the user presses Space (or Enter), then that
  item shows checked in the view and `Service.GetRunStatus` (queried out-of-band, simulating a concurrent
  `rk checklist status`) reports it checked immediately — not only after quitting.
- Given the TUI is open with the cursor on an already-checked item, when the user presses Space, then the item
  becomes unchecked again and its `CheckedAt` clears.
- Given a run with exactly one unchecked item remaining, when the user checks that last item, then the run's
  status becomes `RunStatusCompleted` in storage, and the TUI's behavior matches whatever §2's [OPEN]
  auto-exit decision resolves to (either: shows a completion message and exits automatically; or: shows a
  completion indicator and waits for manual quit) — pin this down before writing the test, not after.
- Given the TUI is open with some items checked, when the user quits via `q`, `Esc`, or `Ctrl+C`, then the TUI
  exits, the run remains `RunStatusActive`, and every item checked during the session is still checked in
  storage afterward (three separate test cases, one per key, mirroring v0's `TestChecklistRunModelQuit{Q,Esc,CtrlC}`).
- Given a template with 0 items, when the TUI opens, then it renders a "no items" state; pressing toggle keys,
  navigation keys is a no-op (no crash, no error, no service call); quitting still exits cleanly.
- Given a template name that doesn't exist, when `rk checklist run <template>` is invoked on a TTY, then it
  errors with the same "checklist template %q not found" message fnqs.11's verbs use, and no TUI is ever drawn.

### Non-interactive / flagged path (fnqs.11 parity — regression coverage, not new behavior)

- Given a template with no active run, when `rk checklist start <template>` runs non-interactively (unchanged
  from fnqs.11), then it creates the same kind of run `run`'s TUI would have resolved to — confirming both
  entry points share one resolution path, not two.
- Given items were checked via the TUI in a prior session, when a script later runs
  `rk checklist status <template> --json`, then the reported checked/unchecked state and item positions match
  exactly what the TUI last displayed — confirming both paths write through the same
  `Service`/`Repository`, with no divergent state.
- Given an active run, when `rk checklist check <template> <position>` is invoked directly (never touching
  `run`), then it continues to behave exactly as fnqs.11 shipped it — `run`'s addition must not alter
  check/start/status/reset/abandon's existing flags or output.

### TTY-guard error path

- Given stdout is piped to a file (or stdin redirected from `/dev/null`), when `rk checklist run <template>` is
  invoked, then it returns a non-zero exit with an error naming `--no-input`/needing an interactive terminal,
  within a bounded time (no hang) — assert via a timeout-bounded test harness, same pattern as
  `prompt_test.go`'s `runPromptForTest`.
- Given a real TTY, when `rk checklist run <template> --no-input` is invoked, then it still errors the same way
  (the flag overrides an actually-interactive terminal).
- Given the guard fires, when the error surfaces, then no `tea.Program` is ever started (assert no TUI escape
  sequences / rendering reached the output stream — `tea.WithOutput(io.Discard)` plus asserting on the returned
  error, mirroring `TestRunPrompt_GuardBlocksBeforeProgram`).
- Given `run`'s implementation goes through `components.RunPrompt`, when the guard is exercised, then it is
  `components.PromptGuard` (the shared hook wired once in `internal/cli/interactive.go`) doing the check — not
  a second, checklist-specific isatty check hand-rolled in `checklist.go`.

## 5. Explicitly out of scope

- **Vault-native persistence rewrite** (reckon-fnqs.13, P3, open, deferred by design). fnqs.12 ships on top of
  the existing `storage.Database`-backed `Service`/`Repository` as-is. Per fnqs.13's own text, the TUI's
  rendering/interaction logic must not need to change when that rewrite lands — only the load/save calls behind
  the `ChecklistItem` mapping. This is *why* the "no `*checklist.Run`/`*RunItem` dependency" constraint exists
  in this ticket; violating it here creates exactly the rework fnqs.13 is trying to avoid.
- **`rk index --rebuild` not preserving checklist state** — already an accepted, documented limitation from
  fnqs.11 (`checklistCmd.Long`); unchanged and not re-litigated by this ticket.
- **Composable board / pluggable panes** (reckon-fnqs.10, P3, open). Mounting `ChecklistRunner` as a pane
  inside the persistent top-level TUI (`rk tui`, fnqs.8) alongside agenda/todos/notes is a separate, later
  ticket. fnqs.12 only wires the standalone `rk checklist run` verb.
- **Wizard-chained checklist *creation*** (an interactive `rk checklist create` flow analogous to fnqs.7's
  `rk todo add` wizard). fnqs.7 (P2, still open) covers bare-vs-flagged wizards for todo/note/add, not
  checklist; fnqs.12 is scoped only to the `run` verb over an existing template.
- **New flags on `create`/`list`/`start`/`check`/`status`/`reset`/`abandon`.** Those verbs are already
  closed/shipped by fnqs.11; out of scope here beyond possibly factoring their shared resolve-run logic into a
  helper `run` also calls.
- **Multi-template/run picker inside the TUI** (e.g., choosing which template's run to open from a list).
  Scope is a single `<template>` positional argument per invocation, matching every fnqs.11 verb's existing
  convention.
- **Editing template item text or reordering items from within the TUI.** `Service.AddTemplateItem`/
  `RemoveTemplateItem` exist but have no CLI verb from fnqs.11 either; the mini-TUI is a run-*execution*
  surface, not a template editor.
- **In-TUI abandon keybinding** (`a`, present in deleted v0). Not listed in fnqs.12's scope bullets (only
  move/select/toggle/see-completion are); `Service.AbandonRun` and `rk checklist abandon` already exist and
  cover the non-interactive path. Treat as optional stretch, not a Done-when requirement — flagged under §2 as
  [OPEN], not assumed.
