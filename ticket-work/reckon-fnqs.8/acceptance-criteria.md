# Acceptance criteria — reckon-fnqs.8 (persistent multi-pane `rk tui`)

Builds on `ticket-work/reckon-fnqs.8/codebase-analysis.md` (verb-function table §3, component
import audit §5, command-registration pattern §6, pitfall table §8) — not restated here except
where this doc's conclusion diverges from it (see §2.1).

## 1. Explicit acceptance criteria

From `bd show reckon-fnqs.8` Scope + Done-when, numbered:

1. `rk tui` opens a persistent, multi-pane, full-screen layout on a TTY (bubbletea `tea.WithAltScreen()`).
2. Exactly 4 panes: **agenda**, **todos**, **log**, **notes**. No more, no fewer, for this ticket.
3. Agenda pane surfaces `rk today`'s split-actuator model: same predicate (overdue + scheduled-today
   + today-pinned, native `todo` + external `work-ticket`), same read-only guard on external rows,
   same actuator keys (`t/d/D/p/x/i/c`) mapped to `today.go`'s `act*` functions.
4. Todos pane shows subject line only (the node's derived `title` — first non-empty body line,
   `internal/index/schema.go:37`), per the subject/body convention (reckon-fnqs.3, closed).
5. All reads go through the SQLite index's public views only (`nodes`, `edges`, `node_props`,
   `aliases`, `fts`/`fts_search` — `internal/index/schema.go:84-93`). No direct file walking for
   pane data, no legacy operational DB.
6. All writes call the existing CLI verb functions (`addDurableTodo`, `addEphemeralTodo`,
   `completeDurableTodoNode`, `appendLogEntry`, the `act*` family, etc. — codebase-analysis.md §3)
   — never a hand-rolled write path, never `writeFileAtomic` called directly from TUI code.
7. Creation/edit flows inside the TUI call verb functions directly, not through a Prompt/Wizard
   abstraction (reckon-fnqs.6 is explicitly not a dependency).
8. Every TUI-driven mutation is observable as a vault file change.
9. Every TUI-driven mutation survives a full index rebuild — see §3 "`rk index --rebuild`" note.
10. No code path in `internal/tui` imports the journal/service packages — see §2.1, the central
    open question this doc surfaces.

## 2. Implicit requirements

| # | Requirement | Grounding |
|---|---|---|
| 2.1 | **Component decoupling from `internal/journal` — contested, resolves to strict.** See below. | `internal/tui/components/{task_list,log_view,schedule_view,wins_view,intention_list,task_picker}.go` |
| 2.2 | New orchestration code (the model/layout/keyboard equivalent) must live in **package `cli`** (e.g. `internal/cli/tui.go` + siblings), not a revived `internal/tui` root package. | Verb functions are unexported (`addDurableTodo` etc., codebase-analysis.md §3); `internal/tui` importing `internal/cli` while `internal/cli` imports `internal/tui/components` is fine (no cycle, verified: neither currently imports the other), but `internal/tui` calling unexported `cli` symbols is not possible cross-package. Exporting the whole verb surface instead is a much larger, unscoped refactor. |
| 2.3 | Hold **one long-lived `*index.Index`** for the TUI session (open at startup, `Close()` on exit via `defer`), not open/close per keystroke. | `internal/index/index.go:69` `Open`; the reconcile lock is acquired/released per-`Reconcile()` call, not held for the DB handle's lifetime (`internal/index/lock_unix.go:14-27`), so a long-held handle coexisting with a separate `rk` CLI invocation, or with the TUI's own verb-triggered file writes, is safe. |
| 2.4 | Every pane mutation must trigger `ix.Reconcile()` before the next render, mirroring `today.go`'s "eager, non-fatal reconcile after write" pattern. | `internal/cli/today.go:367-378` |
| 2.5 | No background file-watcher. `internal/sync` (fsnotify) was deleted in PR #158 and `fsnotify` dropped from `go.mod`. External vault edits are picked up only on the TUI's own next `Reconcile()` (mutation-triggered, or an optional `tea.Tick` poll — not required by Done-when). | commit `83ca4ef`; codebase-analysis.md §8 |
| 2.6 | Logger must be reconfigured for TUI mode before `tea.NewProgram(...).Run()`, or INFO-level log lines corrupt the alt-screen. `buildLoggerConfig(true)` already exists and is unchanged from the pre-deletion `rk tui`. | `internal/cli/root.go:34-68`; pre-deletion reference `git show 83ca4ef^:internal/cli/stubs.go` |
| 2.7 | Ref resolution inside the TUI (ULID/alias → file) must reuse the CLI's existing resolvers (`loadNativeTodoForEdit`, `findDurableTodoByRefOrAlias`, `lookupNodeByRefOrAlias`) rather than a second hand-rolled walk, so a ref never resolves differently in the TUI than in a bare CLI invocation. | `internal/cli/today.go:441,681`; `internal/cli/todo.go:870` |
| 2.8 | `rk note create` has no standalone verb function — its body is inlined in `runNoteCreateE`. Wiring a "create note" flow inside the TUI requires either extracting a `createNote(...)` helper first (preferred, keeps CLI/TUI behavior identical and satisfies "call the same verb functions") or accepting logic duplication (violates the spirit of req. 6). | `internal/cli/note_v1.go:228-353` |
| 2.9 | `task_list.go` is **not a renderable pane widget** — it's two formatting helpers (`FormatDateInfo`, `GetDateStyle`) operating on `journal.Task.ScheduledDate`/`DeadlineDate`. The todos pane's actual list/cursor/scroll rendering has no surviving component to wire in; it must be built fresh (the old rendering lived in the deleted `internal/tui/model.go`, salvageable only as a pattern via `git show 83ca4ef^:internal/tui/model.go`). | `internal/tui/components/task_list.go:36-127` |
| 2.10 | Terminal resize (`tea.WindowSizeMsg`) must recompute pane dimensions and propagate via each component's `SetSize(width, height)`. No surviving `CalculatePaneDimensions`-equivalent exists in the tree; reimplement for the 4-pane (not 2-pane) layout using the deleted `layout.go` as a pattern reference only. | `git show 83ca4ef^:internal/tui/layout.go` |
| 2.11 | Agenda actuator keys `d`(defer)/`D`(deadline)/`p`(priority) take an argument that `rk today act <ref> <key> <arg>` passes positionally on the command line (`tomorrow`/`next-week`/`YYYY-MM-DD` for defer, a literal date for deadline, `A`/`B`/`C` for priority). Inside a persistent TUI there is no positional arg — pressing `d`/`D`/`p` must open a small input sub-flow (text/date entry, e.g. via `date_picker.go`/`text_entry_bar.go`) that collects the value and *then* calls the same `act*` function with it. This is the concrete mechanic behind test scenario 6 (§4) and is implied but not spelled out by reqs 6-7. | `internal/cli/today.go:405-435` (`dispatchTodayAct`'s `key, arg` split) |
| 2.12 | **Notes pane interaction model is undefined by the ticket — decide before implementing.** See §2.1-adjacent open question below. | `internal/tui/components/notes_pane.go` vs `note_picker.go` |

### 2.1 The journal-import scope question (contested — read before implementing)

`internal/tui/components/{task_list,log_view,schedule_view,wins_view,intention_list,task_picker}.go`
(plus `task_list_test.go`, `task_picker_test.go`, `task_picker_example_test.go`) directly `import
"github.com/MikeBiancalana/reckon/internal/journal"` and expose `journal.Task` / `journal.LogEntry` /
`journal.ScheduleItem` / `journal.Win` / `journal.Intention` as their public constructor/method
parameter types.

**`codebase-analysis.md` §5 concludes these imports are "safe to keep... as-is"**, reasoning that
`journal.Task` etc. are plain DTO structs with no live DB/service coupling. **This doc reaches the
opposite conclusion for the acceptance criteria's own stated verification method:**

- The ticket asks for this Done-when line to be checked by "a static/lint-style check, not just
  manual review" (this doc's own task framing, and consistent with the ticket's general preference
  for automatable checks over prose review).
- A static/lint check operates on the **import graph** — it sees `import ".../internal/journal"`
  and flags it. It cannot semantically distinguish "this file uses `journal.Task` as an inert DTO"
  from "this file uses `journal.Service`." There is no cheap static check that implements the loose
  reading; only manual review can, which the criterion rules out.
- Plain reading also favors strict: "the journal/service packages" most naturally parses as "the
  `journal` package and the `service` package" (both named, both banned), not "the service-shaped
  parts of the journal package."

**Conclusion: treat strict (zero imports of `internal/journal` or `internal/service` anywhere under
`internal/tui/`) as the operative target.** This means the 6 files above need their `journal.*`
parameter types replaced with local/index-native structs (e.g. a small `todoRow`/`logEntryRow`
shape carrying just the fields each renderer touches — `formatDateInfo`/`getDateStyle` only ever
read `ScheduledDate`/`DeadlineDate *string`, so the replacement surface is narrow per file). Whether
`wins_view.go`/`intention_list.go`/`task_picker.go` (not part of the 4-pane set) get decoupled too,
or deleted as unused, is an implementation choice — either satisfies the strict check, since a
deleted file imports nothing.

**[OPEN]**: this is Mike's ticket; he may descope the Done-when bullet to the loose reading
explicitly if the refactor cost is unwanted. Flag the disagreement with `codebase-analysis.md`
rather than silently picking a side when planning starts.

`summary_view.go` imports `internal/time`, and `internal/time/summary.go` in turn imports
`internal/journal` — a *transitive*, not direct, dependency, and `internal/time` sits outside
`internal/tui` entirely. A direct-import grep over `internal/tui/**/*.go` (the plain-meaning,
cheaply-automatable check) will not flag this, and per §4 out-of-scope, `summary_view.go` has no
index-backed data source anyway and is not one of the 4 panes — leave it unwired.

### 2.2 The notes-pane interaction model (undefined — carried forward from codebase-analysis.md, not resolved here)

The ticket names a "notes" pane but does not specify what it shows. Two existing components fit
different, non-overlapping jobs, and neither alone is obviously "the" notes pane:

- `notes_pane.go` (`NotesPane`) renders **one already-selected note's** outgoing links + backlinks
  — a link-graph inspector, not a list. It has no "which note" selection mechanism of its own
  (`NewNotesPane()` takes no note; callers push data via `UpdateLinks(noteID, outgoing, backlinks)`).
- `note_picker.go` (`NotePicker`) renders a **fuzzy-filterable list of notes** (`[]*models.Note`) —
  browsing, not graph inspection.

A persistent notes pane plausibly needs both, composed (browse → select → inspect links), but
nothing in the ticket text commits to that composition, to which one is the v1 minimum, or to what
selects a note if only `notes_pane.go` is used. **[OPEN]**: resolve during planning — candidates are
(a) `note_picker.go` alone (browsable list, no link-graph drill-down, simplest), (b) `notes_pane.go`
alone (requires some other pane/keybinding to first choose a note ID, underspecified), or (c) both
composed (matches "persistent pane" framing best, more surface area to build). This doc does not
pick one; test scenario 3 below is written against reading (b)/(c) (link-graph) since that's the
component literally named "notes_pane," but flags the assumption rather than asserting it as settled.

## 3. Edge cases

| Case | Expected behavior | Grounding |
|---|---|---|
| Empty index (fresh vault, no todos/log/notes yet) | Each pane renders a friendly empty state (mirrors CLI's `"todo: no items"` / `"today: nothing due"` conventions), not a crash or blank flash. | `internal/cli/todo.go:179-180`, `today.go:117` |
| Terminal resize mid-session | All 4 panes reflow via `SetSize`; no panic on 0-width/negative dimensions (old `CalculatePaneDimensions` clamped to `>= 0` — reproduce that clamp). | `git show 83ca4ef^:internal/tui/layout.go` (`if ... < 0 { ... = 0 }`) |
| Non-TTY invocation (piped/redirected `rk tui`) | **Out of scope for this ticket.** Done-when only requires "`rk tui` opens a persistent multi-pane layout on a TTY" — it does not require a specific non-TTY error path. The TTY guard (`isatty` check + `--no-input`) is explicitly reckon-fnqs.6's scope. Whatever bubbletea's default non-TTY behavior is (likely hang or an opaque terminal error) is acceptable for v1; do not build a guard here. [OPEN, not required] |
| Pane with zero items after a filter (e.g. all todos done, `--all` not set) | Same empty-state rendering as the fresh-vault case, distinguishable in copy if easy, not required. | — |
| Very long body/title content in a pane row | Truncate to fit pane width; do not let one row expand pane height or scroll horizontally. Prior art: `eec3af8` fixed exactly this for the log pane. | commit `eec3af8` ("Truncate log entry text to prevent journal pane from expanding horizontally") |
| Malformed `scheduled`/`deadline`/`pinned` date on a candidate agenda row | Skip the row with a warning, do not abort the whole agenda load (mirrors `buildAgenda`'s per-row warning collection). | `internal/cli/today.go:266-286` |
| External `work-ticket` row selected in agenda pane | Read-only: actuator keys rejected/no-op with the same message `rk today act` gives (`"is read-only (external work ticket); use rk today open instead"`); `rk today open`'s browser-jump equivalent (or a no-op if out of scope) rather than an in-place edit. | `internal/cli/today.go:352-354` |
| Recurring todo (`repeat:` prop) marked done via agenda `x` | Cursor advances (`scheduled` field changes), `state` stays `open` — must not flip to `done` in the pane's rendering. | `internal/cli/todo.go:764-844` (`doneRecurringTodo`) |
| Concurrent external mutation (user runs `rk todo add` in another terminal while TUI is open) | Not required to be picked up live (no watcher, §2.5); must not corrupt the TUI's own next write — WAL + busy-timeout + per-Reconcile flock make this safe by construction, not something the TUI needs to special-case. | `internal/index/index.go` (WAL/busy-timeout), `lock_unix.go` |
| CRLF line endings in a target file (todo/log/note) | Verb functions already refuse these (`"CRLF line endings are not supported (reckon-vj55)"`); TUI must surface that error, not attempt to work around it. | `internal/cli/todo.go:857-859` etc. |

## 4. Test scenarios (given/when/then)

**Reads**

1. Given a vault with N durable todos (open/in-progress) and M ephemeral inbox items, when the
   todos pane loads, then it lists exactly the open items with subject-line-only display (no body),
   sourced via `listDurableTodos`/`listEphemeralTodos` reused verbatim (or via a thin index-query
   wrapper with byte-identical filtering).
2. Given a `log/<date>.md` day file with 3 entries, when the log pane loads, then it shows 3
   `log-entry`-typed rows (queried from the index's `nodes` view, `type = 'log-entry'`), not the
   raw day-file body.
3. **[Assumes the link-graph reading of §2.2 — not settled.]** Given a note with 2 forward links
   and 1 backlink, and assuming the notes pane resolves a "current note" (selection mechanism TBD
   per §2.2) and renders its link graph, when that note is loaded, then outgoing/backlink counts
   match `loadNoteForwardLinks`/`loadNoteBacklinks`'s query results. If planning instead picks the
   browsable-list reading, this scenario is replaced by: given N notes exist, when the notes pane
   loads, then it lists all `type = 'note'` rows (title, slug) matching `rk note show`'s candidate
   set, filterable/fuzzy-searchable.
4. Given one todo scheduled for today and one work-ticket row with `deadline` = today, when the
   agenda pane loads, then both appear, the work-ticket row flagged read-only, matching `buildAgenda`
   output exactly for the same vault state.

**Writes (verb function reuse)**

5. Given the todos pane focused, when the user submits "buy milk" via the add-todo flow, then
   `addDurableTodo` is called (not a hand-rolled write), a new `todos/<ULID>.md` file exists on
   disk with `state: open`, and the pane's next render includes it after a `Reconcile()`.
6. Given an agenda row for a native todo, when the user presses `x`, then `completeDurableTodoNode`
   runs (same function `rk today act <ref> x` calls), the file's `state` flips to `done`, and (if
   the row carries `repeat:`) the recurrence branch advances `scheduled` and leaves `state` = `open`.
7. Given the log pane focused, when the user submits a log entry, then `appendLogEntry` creates or
   appends to `log/<day>.md` exactly as `rk add` would (byte-identical entry block format).
8. Given an external `work-ticket` agenda row, when the user presses any actuator key, then the
   mutation is rejected with the same read-only error `rk today act` returns, and the file is
   untouched.

**Index durability**

9. Given a TUI session that creates a todo, completes it, and appends a log entry, when the TUI
   exits and `rk index` (bare — there is no `--rebuild` flag; `internal/cli/index.go:16-59` always
   performs a full `ix.Rebuild()`) runs, then the rebuilt index reflects all three mutations with no
   errors or warnings attributable to TUI-written files. [INFERRED: ticket text says "`rk index
   --rebuild`" but the actual command is bare `rk index`.]
10. Given the same session, when `rk todo list --json` (a fresh CLI invocation, separate process)
    runs after the TUI exits, then it lists the TUI-created todo — proving the mutation is a real
    vault file change, not TUI-internal state.

**Static import check**

11. Given the full `internal/tui/**/*.go` source tree, when a test greps every file's import block
    for `"github.com/MikeBiancalana/reckon/internal/journal"` or `.../internal/service`, then zero
    matches are found. (Direct-import check, per §2.1 — deliberately does not attempt to catch
    `summary_view.go`'s transitive `internal/time → internal/journal` path, which is outside
    `internal/tui` and outside this ticket's remit.) This should be a real `_test.go` (e.g.
    `internal/tui/no_journal_import_test.go`, reading `internal/tui/**/*.go` via `go/parser` or a
    line-anchored regex over source text), not a one-off manual `grep` — the ticket asks for exactly
    this to be automatable.

**Layout/keyboard**

12. Given a 4-pane layout at 120x40, when `tea.WindowSizeMsg{80,24}` arrives, then all 4 panes
    resize without panic and no pane reports negative width/height.
13. Given focus on the todos pane, when Tab (or the chosen focus-cycle key) is pressed, then focus
    moves to the next of the 4 fixed panes in a stable, defined order — not to a 5th/pluggable pane
    (that's fnqs.10).

## 5. Out of scope

| Item | Owner ticket | Note |
|---|---|---|
| Pluggable pane registry, arrange/rearrange panes at runtime | reckon-fnqs.10 | This ticket's 4 panes are static/fixed, hardcoded. |
| Focus routing beyond cycling among the 4 fixed panes | reckon-fnqs.10 | §4 scenario 13 is the ceiling of in-scope focus behavior. |
| Prompt/Wizard-driven creation flows (`components.Prompt` interface, `RunPrompt[T]`, `Wizard`) | reckon-fnqs.6 | This ticket's creation/edit flows call verb functions directly, same as v0's per-component helper models did — not blocked on fnqs.6, not a dependency. |
| TTY guard / `--no-input` flag / non-tty usage-error behavior | reckon-fnqs.6 | Explicitly fnqs.6's scope per its own "Done when." Non-TTY `rk tui` behavior is unspecified/unguarded here (§3). |
| `wins`/`intentions` panes (`wins_view.go`, `intention_list.go`) | — | Listed among "existing components" in the ticket's inventory but not one of the 4 named panes; not required to be wired. Their journal-import status still matters for §2.1's strict check if they remain in the tree. |
| Time-tracking summary bar (`summary_view.go`) | — | No index-backed data source exists for meetings/breaks/tasks/untracked minutes in the new architecture; not required. |
| Live external-edit detection (file watcher) | — | `internal/sync` deleted, `fsnotify` dropped from `go.mod`; not reintroduced by this ticket (§2.5). |
| Composable/shared Prompt implementation between TUI panes and bare-CLI wizards | reckon-fnqs.10 (depends on fnqs.6/fnqs.7) | fnqs.10's own "Done when" requires this; explicitly deferred past this ticket. |
