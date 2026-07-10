# Implementation Plan: v1-T7 `rk today` (split-actuator agenda) — reckon-liml

## Summary

`rk today` becomes a live org-agenda over the node/index substrate: it surfaces the union of overdue + scheduled-today + today-pinned native todos (plus stubbed external work-ticket rows), and exposes deterministic, scriptable actuation verbs that write straight through to the task files and reindex. Every primitive this needs already ships — span-local `SetField`/`InsertField` writes (`internal/node`), `index.Open`+`Reconcile` write-then-reindex (`internal/index`), `doneRecurringTodo`/`appendDidLogEntry` `did`→task linking, and the SQL-over-stable-views read pattern from `todo.go`. T7 is porcelain that composes these; it invents no new storage machinery.

The single hard structural problem is that a legacy, journal-backed `rk today` already occupies the verb name (`internal/cli/today.go`, wired at `root.go:120`), and it is DB-service-backed (`requiresDB` true) — incompatible with being a subcommand parent of a node/index (`requiresDB=false`) command. I resolve this by **replacing** `todayCmd`'s `RunE` with the agenda and dropping the journal-dump behavior at the command-wiring level (the `journal` package itself is untouched — that removal is T9's job). The verb name stays registered, so `root_help_test.go` still passes.

For the interaction model (decision C): the proposal does **not** decisively settle whether v1 ships an interactive TUI. The original 2026-06-21 proposal says "reuses current reckon's existing TUI task list," but the later 2026-06-22 "Lean v1 scope" cut lists `rk today` (split actuator) as IN without separately calling out a TUI, every shipped v1 verb (`todo`/`add`/`query`/`note`/`index`) is plain non-interactive cobra, and the proposal's own "CLI verbs sit underneath as the plumbing the TUI and the agent both call — same verbs, two surfaces" frames the plumbing as the required layer and the TUI as a separable second surface. **I recommend the lean interpretation: ship plain, scriptable CLI verbs only — no bubbletea loop in v1 — and flag this for user sign-off.** Concretely: `rk today` (the agenda list) + `rk today act <ref> <key> [arg]` (the split-actuator: the ticket's `t/d/D/p/x/i/c` keys, one dispatch that enforces native-vs-external) + `rk today open <ref>` (the external jump). A future TUI is a thin presentation layer calling the same `act` dispatch.

All actuation goes through the shipped span-local primitives; completion (`x`) reuses `doneDurableTodo`'s recurrence branch and `appendDidLogEntry` verbatim, refactored so both `rk today` and `rk todo done` share one completion function that takes an explicit "emit did-entry" flag.

## Files to modify

- **`internal/cli/today.go`** — *major rewrite.* Remove the legacy journal-dump `RunE`, the `--format` flag (`todayFormatFlag`), and the `internal/journal` import. Replace with the agenda family: `todayCmd` gains an agenda `RunE` and `Annotations: {"requiresDB":"false"}`; add child commands `todayActCmd` and `todayOpenCmd`; add result types (`agendaResult`/`agendaItem`, `todayActResult`, `todayOpenResult`) with `Pretty()` methods (mirroring `todoListResult`/`todoDoneResult`); add helpers `buildAgenda`, the agenda predicate/candidate loader, the ref→node resolver + split-actuator classifier, the four schedule-key writers, and `resetTodayFlags`. Register children in this file's `init()`.
- **`internal/cli/todo.go`** — *moderate.* Extract the plain-completion body of `doneDurableTodo` into a shared `completeDurableTodoNode(vaultDir string, n *node.Node, foundPath, ref string, logDid bool) (todoDoneResult, error)` that both `rk todo done` (calls with `logDid=false`, preserving current behavior) and `rk today act x` (calls with `logDid=!--no-log`, default true) use. The recurrence branch already logs; leave it. Optional one-line consistency fix: teach `listDurableTodos`' default filter to treat `in-progress` as visible (see decision B4).
- **`internal/cli/format.go`** — *minor.* Remove the now-orphaned `formatJournalTSV` (grep-confirmed: only `today.go` calls it). Keep `formatJournalsJSON`/`formatJournalsCSV` (still used by `week.go`) and `parseFormat` (6 other callers). No other change.
- **`internal/cli/today_test.go`** — *NEW.* Add the `runToday(t, vault, args...) (stdout, stderr string, err error)` helper (same shape as `runQuery`/`runTodo`) plus all TS-1.x…TS-6.x tests, reusing `setupQueryVault`/`writeTestNode`/`resetCLIFlags`/`buildIndex` from `query_test.go` and the `todoNow` clock seam / `pinTodoNow` pattern from `todo_recur_test.go`.
- **`internal/cli/root.go`** — *likely untouched.* `RootCmd.AddCommand(todayCmd)` already exists (line 120); `act`/`open` are registered under `todayCmd` in `today.go`'s `init()`. Verify only.
- **`internal/cli/root_help_test.go`** — *no change, verify.* Still asserts `today` is registered (line 35); replacing `RunE` keeps that true.

No changes to `internal/index` or `internal/node`: the new frontmatter keys (`pinned`, `priority`, `source`, `source-url`) are plain scalars that flow into the `node_props` view automatically (confirmed: `parseRefValues` only promotes `[[wikilink]]` values to edges, so a bare URL stays a scalar prop), and `type: work-ticket` flows into `nodes.type`. No schema/view/DDL work.

## Design decisions

### A. Command-name collision — REPLACE the legacy `rk today`

**Decision:** Replace `todayCmd`'s `RunE` with the agenda; annotate `todayCmd` `requiresDB=false`; delete the journal-dump code path (RunE, `--format` flag, orphaned `formatJournalTSV`). The `internal/journal` package/service is **not** touched (still used by `week`/`schedule`/`task`/`tui`); only `rk today`'s *use* of it is removed — exactly the "resolve the naming collision at the command-wiring level" mandate that acceptance-criteria §5 assigns to this ticket (the broader legacy-data retirement is T9/`reckon-s6oh`).

**Why:** `docs/design/km-architecture-proposal.md:413` states the design's own model — `/startup` reads the new `rk today` (T7) — i.e. the agenda *becomes* `rk today`; the journal dump is retired, not relocated. Grep confirms no test asserts the legacy output (only `root_help_test.go:35` checks the verb is registered, which stays true).

**Alternatives considered:**
- *Relocate the journal dump to a `rk today journal` subcommand.* Rejected: the `requiresDB` annotation walks ancestors (`root.go:94-99`), so a `requiresDB=false` agenda parent would starve a `journalService`-backed child (service never initialized → `journalService == nil` error). Making it work would require the child to lazily self-init the service — net-new plumbing for a feature slated for T9 removal.
- *Gate the dump behind `rk today --journal`.* Same `requiresDB` conflict (the flag path needs the service the annotation skips).
- *Name the agenda `rk agenda`, leave legacy `today` intact.* Rejected: contradicts the design's explicit naming intent and leaves a stale command the design says to retire.

**Flag for sign-off:** this is a real user-facing behavior change — `rk today` no longer dumps the journal / no `--format`. The "pipe a day to an LLM" use case is partially covered by `rk week --format`; a dedicated `rk journal` verb can be reintroduced by T9 if still wanted.

### B. Missing vocabulary — new props/state strings, all plain scalars in `node_props`

Consistent with T5's existing lowercase lone-word scalar props (`state`/`scheduled`/`deadline`/`repeat`), all new fields are node frontmatter scalars written via the `HasField` trichotomy (`SetField` if present, else `InsertField`) and read back from the `node_props` view. No index columns, no views.

**B1. `pinned` = a date (`YYYY-MM-DD`), surfaced when `pinned == today`.** The `t` key sets `pinned` to today's UTC date (`todoNow()`). *Why a date, not a boolean:* the proposal lists "scheduled-today" and "today-pinned" as two *separate* source buckets (design line 764), so a pin is not merely `scheduled==today`; a dated pin (a) reuses the exact `parseSchedDate` format, (b) auto-expires — it matches only `== today`, never triggers "overdue" pressure the next day, matching the soft "today-pin" semantics, and (c) is a genuinely distinct query predicate from `scheduled` (`<= today`). *Alternative:* `pinned: true` boolean — rejected: never expires (needs a separate unpin action), and "today-pin" reads as a single-day nudge. **Flag for sign-off** (genuinely underspecified).

**B2. `priority` = a single letter `A`/`B`/`C`, validated.** The `p` key sets it; the `act` handler rejects any value outside `{A,B,C}`. *Why:* the design's gold standard is org-agenda (line 756), and org priorities are `[#A]/[#B]/[#C]`; a validated closed set avoids the unbounded value space acceptance-criteria §2.1 explicitly warns against ("pick a concrete scale … rather than silently inventing one that later needs a breaking migration"). *Alternative:* numeric `1-9` — rejected for org-consistency. **Flag for sign-off.**

**B3. New `state` values `in-progress` (`i`) and `cancelled` (`c`).** Set via `SetField("state", …)`, mirroring the `done` transition. These are terminal-ish transitions on durable native rows only.

**B4. Default-filter treatment of the new states.** The **agenda** predicate treats a native row as actionable when `state ∈ {open, in-progress}` (excludes `done`, `cancelled`). *Recommended one-line consistency fix in `todo.go`:* change `listDurableTodos`' default hide-filter from `state != "open"` to also keep `in-progress` visible, so an in-progress item does not vanish from `rk todo list` default while showing in `rk today`. `done`/`cancelled` stay hidden by default (both terminal). *Alternative:* leave `rk todo list` untouched (in-progress hidden there by default) — rejected as a latent UX inconsistency, but noted because it touches a T5 command + `todo_test.go`. **Flag: this modifies T5 behavior/tests minimally.**

**B5. "Overdue" definition (open Q5).** Effective agenda predicate per node: `(scheduled present AND scheduled <= today) OR (deadline present AND deadline <= today) OR (pinned present AND pinned == today)`, ANDed (for native rows) with `state ∈ {open,in-progress}`. This is AC-1's "overdue (scheduled or deadline < today) + scheduled-today + today-pinned," with the one deliberate superset that a `deadline == today` also surfaces (a "due today" item is canonically actionable; no TS contradicts it — TS-1.4 only tests future-*scheduled* exclusion). All comparisons use `parseSchedDate` (strict `YYYY-MM-DD`, UTC) against a single `todoNow()`-derived UTC "today", per `recur.go` discipline (avoids the UTC-vs-local bug class in `REVIEW_PATTERNS.md`). Lexical `<=` on `YYYY-MM-DD` strings is chronological, but I compute in Go after `loadTodoProps` (the `todo.go` `listDurableTodos` precedent) so malformed dates (EC-7) are caught per-row.

### C. Interaction model — CLI plumbing only (no TUI in v1) — NEEDS SIGN-OFF

**Decision:** v1 ships three plain cobra verbs, no bubbletea loop:
- `rk today` → the agenda list (AC-1). Structured via `--json`/`--ndjson`; empty agenda prints "today: nothing due" / `{"items":[]}` and exits 0 (EC-1).
- `rk today act <ref> <key> [arg]` → the split actuator (AC-2/3/4/5a). `<key>` accepts the ticket's single letters *or* readable aliases: `t|pin`, `d|defer <tomorrow|next-week|YYYY-MM-DD>`, `D|deadline <YYYY-MM-DD>`, `p|priority <A|B|C>`, `x|done` (respects `--no-log`), `i|start`, `c|cancel`. One `RunE` switch = one place the native-vs-external guard lives.
- `rk today open <ref>` → the external jump (AC-5b): prints the row's `source-url` (structured). Shelling to `$BROWSER`/`xdg-open` is a trivial, deferred enhancement behind a future `--open` flag; print is the tested, side-effect-free behavior.

**Why this is the proposal's lean reading, and why C is genuinely unsettled:** the 2026-06-21 proposal's "reuses current TUI task list" predates the 2026-06-22 "Lean v1 scope" cut, which lists `rk today` (split actuator) as IN but does **not** list a TUI as an in-scope deliverable; the proposal's "same verbs, two surfaces" makes the CLI verbs the required plumbing and the TUI a separable surface; zero prior v1 verb is interactive; and the ticket's "tested" bar is far cheaper for `RootCmd.SetArgs`+`Execute` verbs than a simulated-`Update()` TUI harness. A single `act` dispatch also maps 1:1 to the ticket's "in-list keys" mental model and gives the split-actuator guard exactly one home.

**Alternatives considered:** (a) *Full/minimal bubbletea list* — rejected as gold-plating for v1: the only reusable TUI code is bound to the legacy `journal.Task` model (wrong data model), so it would be net-new `bubbles/list` work, and nothing in Lean v1 requires it. (b) *Eight named subcommands* (`pin`/`defer`/`deadline`/`priority`/`done`/`start`/`cancel`/`open`) — more discoverable but larger surface; rejected for the reviewer's "minimal net-new surface." **Because the proposal does not decisively settle C, this recommendation needs user sign-off; if an interactive list is wanted for v1, it should be a thin layer over the same `act` dispatch, built fresh against `bubbles/list` following `note_picker.go`'s structure — not the legacy components.**

### D. External-row stub — `type: work-ticket` + `source`/`source-url` props

**Decision:** an external row is a node with `type: work-ticket` and scalar props `source: <jira|gh|…>` and `source-url: <url>` (plus optional `scheduled`/`deadline`/`state` so it can surface). The split-actuator dispatch keys on `type`: `todo` (and, if ever surfaced, `todo-ephemeral`) → native/actuatable; `work-ticket` → external/read-only + jump. No feeder is built (the feed is stubbed, per acceptance-criteria §5); tests hand-write `work-ticket` fixtures via `writeTestNode(vault, "work/ext1.md", ulid, "work-ticket", body, "source: jira", "source-url: https://…", "scheduled: <today>")`.

**Behavior:**
- The agenda candidate query becomes `type IN ('todo','work-ticket')`; external rows surface via the same date predicate, marked in the item struct with `Source`/`ReadOnly`/`SourceURL` so a caller (or future TUI/agent) can render the read-only indication (TS-5.1).
- `rk today act <ref> <key>` on a `work-ticket` row → returns a clear non-zero "row is read-only (external work ticket); use `rk today open`" error, **no file write of any kind** (EC-4/TS-5.1). This dispatch guard is the "split actuator."
- `rk today open <ref>` on a `work-ticket` row → resolves and prints `source-url` (TS-5.2); on a native row → error (jump is external-only).

**Why:** `type`-based dispatch reuses the existing `nodes.type` column with zero schema work and cleanly separates the two actuator halves; `source-url` as a scalar prop needs no edge machinery (confirmed via `parseRefValues`). **Alternatives considered:** a `source`/`external` boolean prop *on a `todo`* — rejected: overloads the todo type and complicates the native resolver; a distinct type is cleaner and matches the rebuttal's "read-only view-nodes written into the store" language (rebuttal lines 101-103). **Flag for sign-off:** exact marker names (`work-ticket`, `source`, `source-url`) are net-new conventions.

### Relevant open questions resolved

- **Q6 completion→did wiring:** refactor to a shared `completeDurableTodoNode(…, logDid bool)`; `rk today act x` defaults `logDid=true` (AC-4), `--no-log` disables it (AC-4 "toggleable"); `rk todo done` calls with `logDid=false` (preserves T5). Reuses `appendDidLogEntry` verbatim. *Alternative — always log from `rk todo done` — rejected* to avoid changing T5's default contract (and to avoid log spam on bulk closes); a one-line default flip is available if the owner wants the fuller design reading. **Flag.**
- **Q7 does `c` (and `i`) log?** No. AC-4/§2.2: only `x` (completion) emits a `did` entry (neither `start` nor `cancel` is "did" the task). TS-4.4 asserts this.
- **Q8 defer sub-choice:** as a non-interactive verb this is the explicit arg `rk today act <ref> d <tomorrow|next-week|YYYY-MM-DD>` — no in-list sub-prompt needed, dissolving the "≤1 keystroke" tension for CLI.
- **Q10 external convention:** decided in D.
- **Q11 reindex granularity:** `rk today` (list) opens the index RW and `Reconcile()`s once before reading (the `todo.go` list precedent). `rk today act`/`open` `Reconcile()` once up front (to resolve external rows + current state) and, for `act` writes, `Reconcile()` **again after the write** (eager write-through) so even a subsequent read-only `rk query` — which never reconciles — sees the change, directly satisfying "actuation writes through to files and reindexes" and TS-2.5's `rk query` variant. Every `Reconcile()` is a mtime/hash-fast-path vault walk (cheap).
- **EC-12 ephemeral in the agenda:** **descoped for v1.** Ephemeral todos carry no `scheduled`/`deadline`/`pinned` (durable-only, `todo.go:277`), so they can never match AC-1's predicate — including them would require an "unscheduled inbox" section AC-1 doesn't describe. Ephemeral actuation stays with the existing `rk todo done --ephemeral <index>`. TS-3.4 is therefore descoped (or reframed as a direct `rk todo done --ephemeral` test). **Flag for sign-off** (lean reading of the [INFERRED] EC-12).

### Cross-cutting conventions (from `REVIEW_PATTERNS.md` / package precedent)

Span-local writes only (`SetField`/`InsertField` + `writeFileAtomic`, never `Render()`+overwrite of an existing file — AC-2/§2.3); re-read + re-parse the target file immediately before writing (EC-8, as `doneDurableTodo` already does); CRLF rejection on every raw read (`reckon-vj55`); malformed date → skip that field + warn to stderr, never abort the load (EC-7); file write authoritative, index update non-fatal/self-healing (EC-9); `fmt.Errorf(…%w…)` wrapping and `!(mode==Pretty && quietFlag)` status-line gating throughout.

## Test scenarios

All in package `internal/cli`, file `today_test.go`, driven through `RootCmd.SetArgs(...)`+`Execute()` via a `runToday` helper, with `t.Cleanup(resetCLIFlags)` and the `todoNow` clock seam pinned for deterministic "today."

**AC-1 (surfaced set) — `internal/cli/today_test.go`:**
- `TestToday_SurfacesOverdue` (TS-1.1: `scheduled=yesterday`, open → present)
- `TestToday_SurfacesScheduledToday` (TS-1.2)
- `TestToday_SurfacesPinned` (TS-1.3: no dates, `pinned=today` → present)
- `TestToday_ExcludesFutureScheduled` (TS-1.4: `scheduled=tomorrow` → absent)
- `TestToday_ExcludesDone` (TS-1.5: `scheduled=today`, `state=done` → absent)
- `TestToday_DedupesOverdueAndPinned` (TS-1.6/EC-2: overdue AND pinned → exactly once)
- `TestToday_EmptyAgenda` (TS-1.7/EC-1: exit 0, `{"items":[]}`)
- `TestToday_SkipsMalformedDateRow` (EC-7: bad `scheduled` → row skipped + stderr warning, others still listed)

**AC-2 (schedule keys write through) — `today_test.go`:**
- `TestTodayAct_DeferWritesScheduledSpanLocal` (TS-2.1: `act <ref> d tomorrow`; assert `scheduled` line changed, extra frontmatter + body byte-identical, re-listed date reflects change)
- `TestTodayAct_DeadlineInsertsDeadline` (TS-2.2: `act <ref> D <date>` on a todo with no `deadline` → `InsertField` path)
- `TestTodayAct_PinSetsPinnedAndSurfaces` (TS-2.3: `act <ref> t` → `pinned=today`, subsequently in `rk today`)
- `TestTodayAct_PrioritySetsLetter` (TS-2.4: `act <ref> p B`; and `act <ref> p Z` → validation error)
- `TestTodayAct_WriteThroughVisibleToQuery` (TS-2.5: after `act`, `rk query` with no manual `rk index` sees the mutation — exercises the eager post-write reconcile)

**AC-3 (do keys write through) — `today_test.go`:**
- `TestTodayAct_DoneFlipsStateSpanLocal` (TS-3.1: `act <ref> x` → `state=done`, byte-identical elsewhere, drops from next load)
- `TestTodayAct_StartSetsInProgress` (TS-3.2: `state=in-progress`; stays in agenda per B4/B5)
- `TestTodayAct_CancelSetsCancelled` (TS-3.3: `state=cancelled`; drops from agenda)
- (TS-3.4 ephemeral — **descoped per EC-12 decision**; if retained, `TestTodoDone_EphemeralFlipsCheckbox` lives in `todo_test.go` against the existing verb)

**AC-4 (completion logs `did`) — `today_test.go`:**
- `TestTodayAct_DoneEmitsDidEntry` (TS-4.1: new `log-entry` in `log/<today>.md`, `edges` shows `rel='did'` from entry → todo ULID)
- `TestTodayAct_DoneOnRecurringReusesT6Path` (TS-4.2: `repeat` prop → `state` stays open, `scheduled` advances, `did` entry written — asserts identical to `rk todo done` recurrence)
- `TestTodayAct_DoneNoLogSuppressesEntry` (TS-4.3: `act <ref> x --no-log` → state done, no `log-entry`/`did` edge)
- `TestTodayAct_StartAndCancelDoNotLog` (TS-4.4: `i`/`c` create no `did` edge)

**AC-5 (split actuator) — `today_test.go`:**
- `TestTodayAct_ExternalRowRejected` (TS-5.1: each of `t/d/D/p/x/i/c` on a `work-ticket` fixture → non-zero read-only error, no file in vault modified — assert via mtime/bytes)
- `TestTodayOpen_ExternalRowExposesURL` (TS-5.2: `rk today open <ext-ref>` prints `source-url`)
- `TestTodayAct_NativeRowUnaffectedBySplit` (TS-5.3: mixed agenda, `x` on the native row actuates normally)
- `TestToday_ListMarksExternalReadOnly` (external row appears with `ReadOnly`/`Source` set in `--json`)

**AC-6 (tested) — harness:** `TestToday_RunHarness` implicit; `runToday(t, vault, args...)` added mirroring `runQuery`. Plus `TestToday_LegacyJournalDumpReplaced` (bare `rk today` no longer dumps journal content) and confirm `root_help_test.go` still green (verb registered).

## Known risks or ambiguities

- **Decision C is not settled by the proposal.** The recommended CLI-only interpretation (and the `act`-dispatch shape) needs explicit user sign-off; a later TUI is additive over the same verbs.
- **Legacy-behavior removal is user-visible** (decision A). `rk today --format`/journal dump goes away now, ahead of T9. Called out, not silent.
- **Modifying T5 surfaces** (decisions B4, Q6): the `listDurableTodos` filter tweak and the `doneDurableTodo`→`completeDurableTodoNode` refactor touch `todo.go` and may shift a few `todo_test.go`/`todo_recur_test.go` expectations. The refactor must keep `rk todo done`'s existing output byte-for-byte (default `logDid=false`); the recurrence branch is unchanged.
- **New conventions need sign-off:** `pinned`-as-date (B1), `priority` `A/B/C` (B2), `state` spellings `in-progress`/`cancelled` (B3), and the `work-ticket`/`source`/`source-url` marker (D) are all net-new and, once written into files, are effectively a schema commitment (changing them later is a migration).
- **`deadline == today` is a deliberate superset** of AC-1's literal "deadline < today" (B5). No TS contradicts it, but confirm the owner wants due-today deadlines to surface.
- **Multi-file, non-atomic completion** (`x`): the state/scheduled write and the `did`-entry write span two files and cannot be atomic — the existing `doneRecurringTodo` ordering/partial-failure contract (file write authoritative, log-entry failure surfaced but not rolled back) is inherited as-is.
- **Cross-directory external resolution:** `work-ticket` fixtures may live outside `todos/`, so `act`/`open` classify/resolve external refs via the index (`nodes`/`aliases` views), while native durable refs reuse `findDurableTodoByRefOrAlias` (glob `todos/`) for identical resolution to `rk todo done`. Keep the two resolution paths clearly separated.

## Out of scope (explicit, per acceptance-criteria §5)

- **Reminders** as an agenda source (ticket: "+ reminders later"); no `remind:` reads, no `on-reminder-due` wiring.
- **A live external feed** (`rk-jira`/`rk-gh`); only the synthetic `work-ticket` stub + split-actuator behavior.
- **Round-trip-to-source actuation** for external tickets (permanently out — "the reconcile engine the owner refused").
- **Agent-assisted plan pre-fill / propose-and-confirm** (`[a]ccept/[e]dit/[r]eject`); porcelain layered on top later.
- **Read-time synthesis / `rk-brief`.**
- **Recurrence mechanics** (parsing/advance/pile-up) — reused from T6, not reimplemented.
- **Interactive bubbletea TUI** (decision C recommendation).
- **Ephemeral rows in the agenda** (EC-12 decision) — stay with `rk todo done --ephemeral`.
- **MCP/agent-surface wiring** (`reckon-cxx1`/T10); T7 only needs CLI-level structured output.
- **Removing the legacy `journal` package/`schedule`/`task` commands** (T9/`reckon-s6oh`); T7 only frees the `today` verb name.
- **No placeholder TODOs / stub RunE** anywhere in the shipped code.
