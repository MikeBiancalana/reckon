# Codebase analysis — reckon-liml (v1-T7: `rk today`, split-actuator agenda)

Read-only research for planning. Worktree: `/home/chadd/repos/reckon/.worktrees/reckon-liml`.

## 0. TL;DR for the planner

All three dependencies (T3 `rk query`, T4 log tool/`rk add`, T5 `rk todo`) are merged
and present, and every primitive T7 needs — span-local writes, index views, did-linked
log entries — already exists and has a direct precedent to clone. But there are **four
real, undecided gaps** nothing upstream resolves, all load-bearing for planning:

1. **Command-name collision.** `internal/cli/today.go` already registers a live,
   working `rk today` command — the *legacy* "dump today's journal to stdout" verb
   (`journalService`-backed, `--format json/tsv/csv`). T7's agenda must claim the same
   verb name. This is not a hypothetical; it's a real file that must be edited/replaced.
   See §3.
2. **The scheduling vocabulary T7 needs doesn't exist yet.** `rk todo`'s node schema
   today has exactly `state` (`open`/`done`), `scheduled`, `deadline`, `repeat`,
   `depends`. There is **no `pinned`/today-pin prop, no `priority` prop, and no
   `in-progress`/`cancelled` state value** anywhere in code. T7 must invent all of
   these (prop names, value shapes) — nothing to reuse, only conventions to follow
   (lowercase scalar props via `InsertField`/`SetField`, mirroring `scheduled`/`deadline`).
   See §5.
3. **Completion→did-linked-log-entry exists today only on the *recurring* todo path**
   (`doneRecurringTodo` in `todo.go`), not on a plain `rk todo done`. T7's design intent
   ("completion emits a linked log-entry by default") means either generalizing that
   machinery into the plain-completion path too, or duplicating it in `rk today`'s own
   "do" handler. See §6.
4. **No interactive/TUI machinery is used by any v1 (node/index-based) verb.**
   Bubbletea is a live dependency and heavily used, but *only* by the legacy
   `journal.Task`/`models.Note`-bound TUI (`rk tui` and its CLI helper pickers). Every
   T3–T6/T8 verb (`todo`, `add`, `query`, `index`, `note`) is a plain, non-interactive
   cobra command. The ticket's "in-list keys" language reads like an interactive
   experience, but the Lean v1 Scope section of the design doc and every real
   precedent point toward CLI plumbing first (verbs a TUI or agent could drive later).
   This is the single biggest scope call left for planning. See §8.

Everything else below is direct, working precedent: span-local todo writes
(`todo.go`), did-linked log-entry creation (`add.go` + `node.RenderLogEntryWithDid` +
`LogParser`), the stable index views + SQL query pattern (`query.go`, `schema.go`),
and the `ix.Reconcile()`-after-write pattern already proven end-to-end in
`rk todo add` → `rk todo list`.

---

## 1. Ticket recap and dependency status

`bd show reckon-liml`:

> Query overdue + scheduled-today + today-pinned (+ reminders later); in-list relative
> scheduling keys (t/d/D/p) and do keys (x/i/c) actuating NATIVE nodes via span-local
> writes; completion emits a linked log-entry (did->task). SPLIT actuator:
> externally-fed work tickets are read-only with an open-in-source jump (feed stubbed
> in v1). Done when: the agenda surfaces the correct set; actuation writes through to
> files and reindexes; completion logs a did entry; native rows actuate while stubbed
> external rows are read-only; tested.

Depends on (all ✓ merged): `reckon-p0zs` (T3 `rk query`), `reckon-qiua` (T5 `rk todo`),
`reckon-uv09` (T4 log tool/`rk add`). Not a dependency, but adjacent and **not yet
started**: `reckon-s6oh` (T9, migration of DB-primary data to text-truth) — this means
the legacy `journalService`/`journal.Task`/`models.Note` DB-primary subsystem T7 will
be colliding with (today.go, the legacy TUI) is *still live* and won't be retired by
T9 before T7 lands.

Governing design section: `docs/design/composable-redesign.md` lines 759–796
("Proposal — `rk today`: a live agenda...") and lines 991–1062 (scheduling/recurrence
model), plus the 2026-06-22 amendments at lines 1164–1213 (Lean v1 scope, split
actuator). Also `docs/design/composable-redesign-assessment.md:34,41,55` and
`docs/design/composable-redesign-rebuttal.md:185,212` (origin of "split actuator").

---

## 2. What the design doc actually specifies (verbatim, so nothing is lost in paraphrase)

From `docs/design/composable-redesign.md:759-796`:

> - **Surface** — `rk today` queries overdue + scheduled-today + today-pinned + today's
>   reminders. The agent optionally pre-fills a proposed plan (orders the list, suggests
>   do-dates) → kills blank-list triage paralysis while the user stays in control.
> - **Schedule** — in-list relative keys: `t`=today-pin · `d`=defer
>   (tomorrow/next-week/pick) · `D`=deadline · `p`=priority; writes straight to the task
>   file. **Model = org's:** a `scheduled` do-date + a hard `deadline` + a today-pin.
>   No bucket taxonomy to learn — "buckets" are just views over do-dates.
> - **Do** — `x`=done · `i`=in-progress · `c`=cancel. Completion **emits a `log-entry`
>   node linked `did`→task** (via the promotion machinery), closing the loop into the
>   journal and giving time-tracking for free. Default on, toggleable.
>
> **Why it fits:** porcelain over {graph-query read} + {todo-tool write}, in-process in
> the multi-call `rk`; **reuses current reckon's existing TUI task list**. CLI verbs sit
> *underneath* as the plumbing the TUI and the agent both call — same verbs, two
> surfaces.

Sub-decisions confirmed (line 788-795): scheduling = org `scheduled`+`deadline`+
today-pin, no bucket taxonomy (Today/Later/Someday are *computed views*); agent
planning = propose-and-confirm (`[a]ccept/[e]dit/[r]eject`, never silent mutation) —
**note: reminders and agent auto-plan are explicitly listed as "+ reminders later" /
deferred pieces in the ticket text**, so the ticket's own scope is narrower than the
full design proposal (no reminders, agent pre-fill is optional/deferred, not required
for "Done when").

**Split actuator** (2026-06-22 amendment, `composable-redesign.md:1189-1191`):

> `rk today` = split actuator: native nodes actuated in-list; external work tickets
> **read-only + jump** (driving a ticket done *from the agenda* would be the reconcile
> engine the owner refused).

**Lean v1 scope** (`composable-redesign.md:1198-1213`) lists `rk today` (split
actuator) as IN; it does **not** separately list a TUI/interactive layer as an
in-scope deliverable — the "reuses current TUI" line above is from the *original*
2026-06-21 proposal, predating the 2026-06-22 amendments and the Lean v1 Scope cut.
See §8 for why this matters.

---

## 3. Command-name collision: `rk today` already exists (CRITICAL, must be resolved in planning)

`internal/cli/today.go` (67 lines, full file) registers `todayCmd` at
`RootCmd.AddCommand(todayCmd)` (`root.go:120`). Today it does this:

```go
var todayCmd = &cobra.Command{
    Use:   "today",
    Short: "Output today's journal to stdout",
    ...
    RunE: func(cmd *cobra.Command, args []string) error {
        ...
        content, err := journalService.GetJournalContent(today)
        fmt.Print(content)
        return nil
    },
}
```

It supports `--format json/tsv/csv` and reads via the legacy DB-primary
`journalService` (`internal/journal.Service`), operating on `~/.reckon/journal/*.md`
(a *different* tree from the vault's `log/*.md`). It has **zero relationship** to
`internal/node`/`internal/index`/`cfg.VaultDir`.

**Evidence the new agenda is meant to claim this exact name, not a new one:**
`docs/design/km-architecture-proposal.md:413`: *"`/startup` reads `rk today` (T7) +
`rk query` saved views instead of parsing Logseq journals"* — i.e. the design's own
mental model is that `rk today` **becomes** the agenda; the journal-dump use case is
retired, not renamed elsewhere.

**No dedicated test file exists for `today.go`'s current behavior** (`find` for
`today_test.go` — none). The only place the string `"today"` is asserted against is
`internal/cli/root_help_test.go:35`, which just checks that a verb named `"today"` is
registered in the help tree — it does not assert anything about *what* `rk today`
does. So replacing the command's `RunE` entirely is test-safe, but is a real
user-facing behavior change (loses `--format`, loses the "pipe journal to an LLM"
use case) that planning should call out explicitly, not silently drop.

Options for planning to choose between (none pre-decided anywhere):
- Replace `todayCmd`'s `RunE` outright with the new agenda; drop/relocate the
  journal-dump behavior (matches the design doc's naming intent most directly).
- Keep the old behavior behind a flag (e.g. `rk today --journal`) and make the agenda
  the new default `RunE`.
- Something else — but *some* explicit decision is required; this is not a "just add
  a new file" ticket the way T4/T5/T8 were.

Related-but-distinct existing verb: `rk schedule` (`internal/cli/schedule.go`) is a
**different, legacy, unrelated** "daily schedule items" resource (time-blocked
agenda items, `journalService`-backed). Not a naming collision with T7's in-list
scheduling keys, but worth knowing it exists so the two aren't confused.

---

## 4. Node/index substrate T7 reads and writes (all shipped, direct precedent)

### 4.1 `internal/node` — the byte-preserving node package

`Node` struct (`internal/node/node.go:73-92`):
```go
type Node struct {
    Raw []byte
    ULID, Type, Time, Author, Body string
    Aliases   []string
    Props     map[string]string
    Fragments []Fragment
    Links     []Link
    Loc       Loc
    // unexported: frontmatter, fmOrder, fieldSpans, blockSpans, bodySpan
}
```

Write primitives (all span-local, re-parse-after-splice; never full-file regen):
- `(*Node) SetField(key, value string) error` — edits an **existing** scalar
  frontmatter key in place (`node.go:153-169`). This is what `rk todo done` uses to
  flip `state`, and what the recurrence path uses to advance `scheduled`. **T7's `t`/
  `d`/`D`/`p` keys will use this for `scheduled`/`deadline` once those props already
  exist on a todo, and `InsertField` the first time a prop like `pinned`/`priority`
  doesn't exist yet.**
- `(*Node) InsertField(key, value string) error` (`internal/node/insert.go`) — adds a
  **new** scalar key that doesn't exist yet, inserted as the first line inside the
  frontmatter fence (or creates a fresh fence if none exists). This is the primitive
  T7 needs the *first* time it sets `pinned`/`priority` on a todo that never had that
  key (`HasField(key)` is the trichotomy check — `SetField` if present, `InsertField`
  if absent).
- `(*Node) HasField(key string) bool` — the presence check to route between the two.
- `Render()`/`NewNode()` (`render.go`) — create-path only (not needed for T7's
  actuation of *existing* todo files, only relevant if T7 ever creates new nodes,
  e.g. the did-linked log entry).

### 4.2 `internal/index` — stable views, read/write handle

Public API (`internal/index/index.go`):
```go
func Open(cfg *config.Config) (*Index, error)              // LogParser{} default
func OpenWithParser(cfg *config.Config, p node.Parser) (*Index, error)
func (ix *Index) Close() error
func (ix *Index) DB() *sql.DB
func (ix *Index) Meta(key string) (string, error)
func DBPath(cfg *config.Config) (string, error)             // read-only DSN construction (T3 only)
```
Freshness (`internal/index/reconcile.go`):
```go
func (ix *Index) Rebuild() (Stats, error)     // full rebuild (rk index)
func (ix *Index) Reconcile() (Stats, error)   // lazy, hash-authoritative, incremental
```

Stable public views (`internal/index/schema.go:81-90`, also documented in
`internal/index/AGENTS.md`):

| View | Columns |
|------|---------|
| `nodes` | `id, ulid, type, time, author, body, loc` |
| `edges` | `src, rel, dst, dst_key, from_frag, to_frag` |
| `node_props` | `id, key, value` |
| `aliases` | `alias, id` |
| `fts` / `fts_search` | `id, body` (`fts_search` supports `MATCH`) |

`id` = inline ULID when present, else surrogate `file:<relpath>`. `dst_key` is
NULL for a dangling (unresolved) edge.

**Architecturally important for T7:** `rk query` (T3) opens the index **read-only**
and **never reconciles** (`query.go:125-127` errors if the index doesn't exist yet;
no `Reconcile()` call anywhere in `query.go`). `rk todo` opens it **read-write** via
plain `index.Open(cfg)` and calls `ix.Reconcile()` itself before listing
(`todo.go:454-462`, in `runTodoListE`). **T7 should follow the `todo.go` pattern, not
the `query.go` pattern**: it needs write access anyway (to write through
todo files after an in-list actuation) and needs to reindex after every write — so it
should hold one `*index.Index` handle open via `index.Open(cfg)` for its whole run,
querying `ix.DB()` directly with hand-written SQL against the views above (exactly
like `todo.go`'s `listDurableTodos`/`loadTodoProps`/`loadDependsOn`), not shell out to
`rk query` as a subprocess.

**Reindex-after-write:** "actuation writes through to files and reindexes" (the
ticket's own acceptance language) maps directly onto calling `ix.Reconcile()` after
each write, the same way `runTodoListE` calls it before each read. Since a single
`rk today` invocation should show a self-consistent list, list once, reconcile once
up front (as `todo.go` does), and reconcile again after any write this same
invocation performs (single-key actuation from the plan, `d`efer, `x`done, etc.) so a
subsequent read in the same session/list reflects it.

### 4.3 How dates/scheduling are represented today (exact fields, formats)

From `todo.go`'s `addDurableTodo`/`todoListItem` and `recur.go`:

- `state` prop: string, currently only `"open"` / `"done"` used anywhere in code.
  **No `"in-progress"`/`"cancelled"` value exists** — grepped the whole repo, zero
  hits outside comments/unrelated contexts (checklist "in-progress" run status is a
  *different*, unrelated concept in `internal/checklist`). T7's `i`/`c` do-keys need
  new state values invented and threaded through wherever `state` is read/filtered
  (`rk todo list`'s default `state != "open"` skip logic, `todo.go:519`, would need to
  learn about the new values too if `rk todo list`/`rk today` are meant to stay
  consistent).
- `scheduled` / `deadline` props: **date-only strings, `YYYY-MM-DD`, UTC, no
  time-of-day** — parsed via `parseSchedDate` (`recur.go:97-103`,
  `time.ParseInLocation("2006-01-02", s, time.UTC)`), never `time.Parse`'s local
  default (this is the exact anti-pattern flagged in `docs/REVIEW_PATTERNS.md`'s
  "Time and Timezone Handling" section — see §9). **Reuse `parseSchedDate` directly**
  for T7's own overdue/scheduled-today comparisons rather than re-implementing date
  parsing.
- `repeat` prop: org repeater cookie (`+Nd`/`++Nd`/`.+Nd`), parsed by `parseRepeat`
  (`recur.go:63-93`). Not itself in scope for T7 beyond "a recurring todo's *current*
  occurrence is what `rk today` surfaces as an actionable row" (design doc line 1021)
  — i.e. T7 reads the same `scheduled`/`repeat` props T6 already writes; no new
  recurrence logic needed.
- `pinned` (today-pin) / `priority`: **do not exist anywhere in the codebase.** No
  prop name, no value convention (boolean? date? priority letter/number?) is
  established by any shipped code. Only the design prose exists (`t`=today-pin,
  `p`=priority — see §2). This is 100% new ground for T7's plan to define. Likely
  shape, by analogy with `scheduled`/`deadline`/`repeat` (all lowercase lone-word
  props, plain scalar values): `pinned: "true"` (or a date) and something like
  `priority: "A"`/`priority: "1"` — but the exact convention is undecided and should
  be a named decision in plan.md, the same way T5's plan.md had D1-D10 decisions and
  T6 had explicit repeater-family decisions.
- `depends` — a *typed edge* (`Link{Rel: "depends-on"}`), not a prop; unrelated to
  scheduling but shows up in `todoListItem`. Not directly relevant to T7 unless the
  agenda also wants to surface/respect dependency blocking (not mentioned in the
  ticket; likely out of scope).

**"Overdue" has no existing definition in code** — it's pure design prose
("`rk today` queries overdue + scheduled-today + today-pinned"). The natural
reading (scheduled or deadline date < today) needs to be made concrete in planning;
org-mode's convention (which this design explicitly imitates) is usually "scheduled
< today OR deadline <= today," but nothing in this repo settles it.

---

## 5. Span-local write + completion/linking patterns to clone

### 5.1 Resolving a todo ref (ULID or alias) to a file — clone verbatim

`todo.go`'s durable-todo resolution chain (`doneDurableTodo`, lines 640-695) is
exactly the shape T7's actuation needs (all same-package `cli` helpers, directly
callable from a new file):
```go
func loadDurableTodoAt(path string) (*node.Node, string, error)               // todo.go:795
func findDurableTodoByRefOrAlias(todosDir, ref string) (*node.Node, string, error) // todo.go:816
func relTodoPath(vaultDir, path string) string                                // todo.go:873
func resolveAuthor(flag string) string                                        // todo.go:246
func writeFileAtomic(path string, data []byte) error                          // adopt.go:250
```
Fast path: `todos/<ref>.md` by ULID; fallback: glob `todos/*.md`, parse each,
match ULID or alias. CRLF files are refused (`reckon-vj55` convention, repeated in
every file-reading helper in this package — must be replicated in any new
file-reading code T7 adds).

### 5.2 Scheduling keys (`t`/`d`/`D`/`p`) — span-local prop writes

Mechanically identical to how `doneRecurringTodo` advances `scheduled`
(`todo.go:749`, `n.SetField("scheduled", nextStr)` then
`writeFileAtomic(foundPath, n.Serialize())`). For a prop that doesn't exist yet on a
given todo (first-ever pin/priority set), use `n.InsertField(key, value)` instead
(§4.1). Concrete recipe per key, using existing primitives:
- `t` (today-pin): `HasField("pinned")` → `SetField`/`InsertField("pinned", ...)`.
- `d` (defer): compute a new `scheduled` date (tomorrow/next-week/arbitrary — the
  design text implies a *sub-choice*, not a single deterministic transform — see
  open question in §8) → `SetField("scheduled", newDate)` (todos always have
  `scheduled` already per `addDurableTodo`'s optional-but-typical usage; if absent,
  `InsertField`).
- `D` (deadline): same shape against `deadline`.
- `p` (priority): `HasField("priority")` → `SetField`/`InsertField`.

All four: `writeFileAtomic(path, n.Serialize())` after the field edit, then
`ix.Reconcile()` (§4.2) so the in-process agenda state (and any subsequent read)
reflects the change.

### 5.3 Do keys (`x`/`i`/`c`) — state transition + completion logging

`x` (done) on a **plain** (non-recurring) todo today = `doneDurableTodo`'s
non-recurring branch (`todo.go:679-694`): idempotent skip if already `"done"`,
else `n.SetField("state", "done")` + `writeFileAtomic`. **This path does NOT
currently write a did-linked log entry** — only the recurrence branch
(`doneRecurringTodo`) does. Per the design's "completion emits a linked log-entry
(did->task) by default, toggleable," T7's `x` key needs that log-entry write
unconditionally (or by default+toggle), which today's plain `rk todo done` doesn't
do. Two implementation options, both viable, a real decision for planning:
  (a) Extend `todo.go`'s non-recurring completion branch (and thus `rk todo done`
      itself) to always emit the did-entry — changes `rk todo done`'s existing
      behavior/tests (`todo_test.go` pins current `todoDoneResult` shapes).
  (b) Implement `rk today`'s own completion handler that calls the same
      `SetField("state","done")`+write, then separately calls `appendDidLogEntry`
      itself — leaves `rk todo done` untouched, duplicates a few lines.

Either way, the **exact machinery to call** is already shipped and requires zero new
node-package work:
```go
// internal/cli/add.go:201
func appendDidLogEntry(logDir, day, hhmm, author, body, didTarget string) (logAddResult, error)
```
This writes (or appends to) `log/<day>.md` via
`node.RenderLogEntryWithDid(hhmm, author, id, didTarget, body)`
(`internal/node/logparser.go:192-194`), which embeds a `did:: <target-ULID>` marker
line right after `id::`. On reindex, `LogParser.buildLogEntry` → `extractEntryDid`
(`logparser.go:164-177`) turns that marker into `Link{Rel: "did", To: didTarget}` on
the parsed **log-entry** node (not the day node) — i.e. the `did`→task edge appears
in the `edges` view as `(src=<log-entry-id>, rel='did', dst=<task-ULID>)` automatically
on the next reconcile, no extra edge-writing code needed. This is precisely "a
log-entry node linked did→task" from the design prose — already built, just not yet
wired to the plain-completion path.

`i` (in-progress) / `c` (cancel): **no existing state-value or code path at all** —
net-new. Likely shape: `SetField("state", "in-progress")` /
`SetField("state", "cancelled")`, mirroring the `"done"` transition, but whether `c`
also emits a did-entry (semantically it's not "did" the task, it's "gave up on" it)
is undecided by the design doc and should be a named decision.

### 5.4 `rk query`/`rk note show`'s edge-query pattern (for surfacing did-links, backlinks)

If T7 wants to show/verify a task's linked log entries (or an agenda row's other
edges), the pattern is `note_v1.go`'s `loadNoteForwardLinks`/`loadNoteBacklinks`
(`note_v1.go:444-483`): `SELECT ... FROM edges WHERE src = ?` /
`SELECT ... FROM edges WHERE dst_key = ?` against `ix.DB()`. Same shape as
`todo.go`'s `loadDependsOn` (`todo.go:561-571`, `SELECT dst FROM edges WHERE src = ?
AND rel = 'depends-on'`).

---

## 6. Query surface: building the agenda's SELECT

`rk query`'s validated read-only path (`query.go`) is the **CLI-facing** read
surface for agents/humans running ad hoc SQL — not necessarily what `rk today`
should call internally (see §4.2: `rk today` needs its own read-write `*index.Index`
handle, and can just run SQL directly against `ix.DB()` the way `todo.go` does).
Still, `query.go` is the best reference for:
- **What SQL shape works against the views** — e.g. `todo.go:488`,
  `SELECT id, body FROM nodes WHERE type = 'todo'`, then per-row
  `SELECT key, value FROM node_props WHERE id = ?` to assemble props
  (`loadTodoProps`, `todo.go:541-559`). T7's agenda query is the same shape filtered
  further: `type = 'todo' AND EXISTS (props: state='open') AND (scheduled <= today OR
  deadline <= today OR pinned prop present)`. Since props are EAV-shaped
  (`node_props(id,key,value)`, one row per key), an agenda SELECT will likely need
  either a self-join/subquery per prop (`WHERE id IN (SELECT id FROM node_props WHERE
  key='scheduled' AND value <= ?)`) or a post-filter in Go after loading each
  candidate's props (the `todo.go` `listDurableTodos` approach: load candidate
  IDs+bodies broadly, then `loadTodoProps` per row and filter in Go). The latter is
  simpler and is the existing precedent; SQL-side date comparison on a
  `YYYY-MM-DD` string column also works correctly (lexical order == chronological
  order for that format) if a planner prefers to push the filter into SQL.
- **Saved views** (`--view NAME` → `<vault>/.reckon/views/<name>.sql`,
  `query.go:205-222`) — no saved view files exist anywhere in this repo (not even a
  sample); a hypothetical "daily" view named in the design doc (line 818) was never
  created. T7 does not need to create or depend on one; it's simplest to hand-roll
  its own SQL in Go (again, exactly `todo.go`'s pattern).

---

## 7. SPLIT actuator: native vs. externally-fed work tickets

**Zero existing code or model for this.** Grepped the whole repo (`jira`,
`work-ticket`, `external`, `worklog`) — no hits outside design docs and the unrelated
`rk-<verb>` external-dispatch mechanism (`dispatch.go`, about finding `rk-foo`
binaries on `$PATH`, nothing to do with work-ticket data). The "feed stubbed in v1"
language in the ticket means: **no real Jira/GitHub integration exists or is being
built**, but the split-actuator *behavior* (some agenda rows are read-only + carry a
jump-to-source action; native rows are read/write) must still be implemented and
tested against some fixture.

Nothing in the codebase defines how an "externally-fed" node would be marked. Likely,
undecided shape for planning to settle: a distinct `type` (e.g. `type: work-ticket`)
and/or a `source`/`external` prop, plus a `url`/`source-url` prop the "jump" action
opens (`xdg-open`/`open` or just prints the URL — no existing "open URL" helper
anywhere in the codebase; would be new). Since the feed is stubbed, tests will need
to hand-write one or two fixture nodes (via `writeTestNode`, see §10) carrying
whatever marker convention is chosen, and assert (a) they appear in the agenda listing,
(b) `t`/`d`/`D`/`p`/`x`/`i`/`c` actuation on such a row is rejected/no-ops, and (c) a
"jump" action produces the expected reference (URL or file path) without writing to
the ticket's own file.

---

## 8. Interactive/TUI machinery — what exists, and the central scope question

### 8.1 What's live and what it's bound to

`go.mod` already depends on `charmbracelet/bubbletea` `v1.3.10`,
`charmbracelet/bubbles` `v0.21.1`, `charmbracelet/lipgloss` `v1.1.0`. Usage is
**entirely confined to the legacy DB-primary subsystem**:

- `internal/tui/` (`model.go`, `handlers.go`, `commands.go`, `layout.go`,
  `keyboard.go`) — the full-screen `rk tui` journal+task TUI, built around
  `journal.Task` (`internal/journal/models.go:106`, DB rows) and driven by
  `journalTaskService`/`journalService`/`notesService` (all legacy, `root.go:29-33`).
  Registered as `tuiCmd` (`stubs.go`, `Use: "tui"`, `requiresDB: true`).
- `internal/tui/components/task_list.go`, `task_picker.go` — bound to
  `journal.Task` (has `Status TaskStatus`, `ScheduledDate *string`,
  `DeadlineDate *string` — a parallel, DB-primary scheduling model, structurally
  similar in spirit to but a completely separate type from the node-based
  `todo.go`'s `Props["scheduled"]`/`["deadline"]`). Already implements overdue/
  due-today/due-soon color-coding (`overdueStyle`/`dueTodayStyle`/`dueSoonStyle`,
  `task_list.go:14-30`) — useful as a **visual-design reference** if T7 does build a
  presentation layer, but the underlying type cannot be reused as-is (wrong data
  model).
- `internal/cli/task_picker_helper.go`, `note_picker_helper.go` — thin
  `tea.Program` wrappers (`PickTask`/`PickNote`) around those components, called from
  the legacy `task.go`/`notes.go` commands.
- `internal/cli/log.go`, `internal/cli/notes.go` — legacy `journalService`/
  `notesService`-backed commands that happen to also launch small bubbletea
  sub-programs (a multiline text editor for `rk log add` with no args, a note-edit
  form) — again, legacy DB-primary side, not node/index-based.

**None of T3/T4/T5/T6/T8** (`query.go`, `add.go`, `todo.go`, `recur.go`, `note_v1.go`)
import `internal/tui` or `internal/tui/components` at all, and none launch a
`tea.Program`. Every one is a synchronous cobra `RunE` reading/writing
`internal/node`+`internal/index` and printing via `internal/output`. This is a
striking, consistent pattern across every prior v1 ticket.

### 8.2 The scope question

The ticket's own language — "in-list relative scheduling keys," "single keys do,"
"one surface" — describes an org-agenda interactive experience, and the *original*
2026-06-21 design proposal explicitly says "reuses current reckon's existing TUI task
list." But:
- That TUI is bound to the wrong (legacy, DB-primary) data model — "reusing" it
  literally would mean writing a `journal.Task` adapter over node/index data, or
  porting the visual/interaction patterns (not the code) into a fresh
  `bubbles/list`-based component, following `note_picker.go`'s pattern (§8.1) as the
  template: `list.Item` implementation + `list.ItemDelegate` + a thin
  `tea.Program` wrapper + a `PickX`-style helper in `internal/cli`.
- The **Lean v1 Scope** section (§2 above, `composable-redesign.md:1198-1213`),
  written *after* the original proposal, lists `rk today` (split actuator) as the
  v1 deliverable without separately calling out a TUI as in-scope — consistent with
  every other v1 verb shipping as plain CLI first ("CLI verbs sit underneath as the
  plumbing the TUI and the agent both call — same verbs, two surfaces," design doc
  line 782-786 — i.e. the plumbing is the required layer; the TUI is a second,
  separable surface).
- The design's own defer-key semantics ("`d`=defer (tomorrow/next-week/pick)")
  implies a sub-choice UI for at least one key, which is easiest to express as a
  flag/argument in a non-interactive verb (`rk today defer <ref> tomorrow|next-week|
  <date>`) and harder to express cleanly as a single bare keypress without some kind
  of follow-up prompt.
- "Tested" (the ticket's own bar) is far cheaper to satisfy for a set of
  non-interactive CLI verbs (`RootCmd.SetArgs`+`Execute()`, the pattern every
  existing `_test.go` in this package uses) than for a full bubbletea interactive
  loop (`internal/tui/AGENTS.md`'s own testing section: "test the model via
  simulated `Update()` calls, don't test rendering").

**Recommendation for planning to explicitly confirm, not assume:** build `rk today`
(and its scheduling/do actuation) as plain, scriptable CLI verbs first — an agenda
*list* command plus per-row actuation verbs/flags (mirroring `rk todo add/list/done`'s
shape) — satisfying "Done when" without an interactive loop. If an actual
single-keystroke interactive list is still wanted for v1 (not just plumbing), treat
it as a thin presentation layer on top of the same verbs, built fresh against
`bubbles/list` + node/index data (following `note_picker.go`'s structure), never
reusing the legacy `journal.Task`-bound components directly.

---

## 9. Known pitfalls (`docs/REVIEW_PATTERNS.md`) directly relevant to this work

- **UTC-vs-local date comparison** (`REVIEW_PATTERNS.md` "Time and Timezone
  Handling," two duplicate entries in the file): `time.Parse("2006-01-02", s)`
  returns UTC midnight; comparing it against a *locally*-formatted "today" silently
  shifts the effective day for non-UTC-0 hosts. `recur.go` already threads this
  correctly (`parseSchedDate` always UTC via `ParseInLocation(..., time.UTC)`,
  `todoNow()` always `time.Now().UTC()`) — **T7's overdue/scheduled-today comparison
  must use the same UTC-consistent clock**, not `time.Now()`/`time.Now().Format(...)`
  mixed with a UTC-parsed stored date. Reuse `parseSchedDate` and a UTC "today"
  directly rather than re-deriving date logic.
- **TUI list selection by ID, not index, across a re-sort/reload**
  (`REVIEW_PATTERNS.md` "TUI List Components," reckon-obed) — only relevant if
  planning chooses to build an interactive list (§8.2): after any actuation reloads
  the agenda rows, restore cursor position by node ID, not by numeric index.
- **Scroll offset not clamped on list shrink** (same section) — same caveat, only
  relevant if an interactive list is built.
- **Async closure capture** (`internal/tui/AGENTS.md`, `REVIEW_PATTERNS.md` "TUI-
  Specific Patterns") — only relevant if a `tea.Cmd` async command is used; capture
  values before the closure, never read `m.field` inside a returned `func() tea.Msg`.
- **Unwrapped/ignored errors, `os.Exit()` in commands, ignoring `--quiet`** — the
  general CLI conventions (`REVIEW_PATTERNS.md` "CLI-Specific Patterns"), already
  consistently followed by `todo.go`/`add.go`/`query.go`; T7's new code should match
  (`fmt.Errorf(..., %w, err)` everywhere; `if !(mode == output.Pretty && quietFlag)`
  gating status-line prints, exactly as in every `runXxxE` reviewed above).
- **CRLF rejection** — not in REVIEW_PATTERNS.md but a hard, repeated convention in
  every file-reading helper this ticket will touch (`todo.go`, `add.go`): any raw
  file read must check `bytes.Contains(raw, []byte("\r\n"))` and refuse
  (`reckon-vj55`). Any new file-reading code in T7 must replicate this.
- **Dead code after refactoring** (`REVIEW_PATTERNS.md`) — directly relevant to §3:
  if `today.go`'s legacy behavior is dropped/replaced, check for now-orphaned helpers
  (`todayFormatFlag`, `formatJournalsJSON`/`formatJournalTSV`/`formatJournalsCSV` in
  `format.go` — verify whether those are used elsewhere, e.g. by `week.go`, before
  deleting).

---

## 10. Testing conventions to reuse

`internal/cli/query_test.go` header comments document the harness (same package,
directly reusable, no import needed):
```go
func setupQueryVault(t *testing.T) (vault, cache string)   // temp vault+cache, t.Setenv RECKON_CACHE
func writeTestNode(t *testing.T, vault, filename, id, typ, body string, extraFM ...string)
func resetCLIFlags()                                        // clears package-global flag vars + RootCmd IO
func buildIndex(t *testing.T, vault string)                 // runs `rk index --vault` via RootCmd
func runQuery(t *testing.T, vault string, args ...string) (stdout, stderr string, err error)
```
`internal/cli/adopt_test.go`: `mustWriteFile`, `mustReadFile`, `mustMkdirAll`,
`isValidULID` generic helpers. `internal/cli/todo_test.go`/`todo_recur_test.go` are
the closest sibling precedent (same package, new create/read/write CLI family over
node+index) — including the `todoNow`/`mintTodoULID` seam pattern (package-level
`var` function pointers overridable in tests for deterministic ULID/clock control,
`todo.go:25-33`) that T7's own date-sensitive logic ("what is overdue right now")
will likely need too, to pin an "as-of" date in tests rather than depending on real
wall-clock time.

All existing v1 command tests drive the command through `RootCmd.SetArgs([]string{...,
"--vault", vault})` + `RootCmd.Execute()`, never call `RunE` functions directly. Any
new `today_test.go`/`agenda_test.go` should follow that shape and register
`t.Cleanup(resetCLIFlags)` per the same-binary global-flag-leak caveat documented in
`query_test.go`'s `resetCLIFlags` comment.

---

## 11. AGENTS.md files read (subsystems touched)

- **`internal/node/AGENTS.md`** — byte-preservation invariant; `SetField`/
  `InsertField`/`HasField` trichotomy; `SetAliases` (not directly relevant to T7);
  the `LogParser`/`id::`/`did::` marker mechanism in full (§5.3 above is a direct
  application of this doc's "Group files: LogParser and the `id::` marker" section);
  round-trip fuzz-gate discipline (`TestRoundTripIdentity`/`FuzzRoundTripIdentity`
  must still pass — not expected to be touched by T7 since it only adds new scalar
  props/state values via existing primitives, but any new parsing logic would need
  to be checked against this corpus).
- **`internal/index/AGENTS.md`** — stable views table (reproduced in §4.2); freshness
  model (lazy reconcile-on-read + explicit rebuild, hash-authoritative); confirms
  `Open`'s default parser is `LogParser{}` (log-day-aware) so a plain `index.Open(cfg)`
  call already sees log entries and their `did` edges with zero extra wiring.
- **`internal/cli/AGENTS.md`** — largely describes the *legacy* v0 command style
  (package-global `journalService`/`taskService`/`notesService`, "Verb Alignment"
  reckon-89hp goal of `add/list/show/edit/delete`). Useful for generic Cobra/flag/
  `--quiet`/error-wrapping conventions, but the actual pattern to imitate for T7 is
  the *v1* precedent in `todo.go`/`add.go`/`query.go`/`note_v1.go` (not literally
  documented in this file, but a consistent live pattern across all four).
- **`internal/tui/AGENTS.md`** — Elm-architecture conventions (Model/Update/View),
  async closure-capture pitfall, component-nil-check pitfall, "test the model, not
  the rendering." Only directly relevant if planning decides to build an interactive
  presentation layer (§8.2); the *architecture* described (package `components`,
  message-passing between components and Model) is the template to follow for any
  new `bubbles/list`-based agenda view, per the `note_picker.go` precedent.

No `internal/index`/`internal/node`-specific entries exist in
`docs/REVIEW_PATTERNS.md` beyond the generic Go/CLI/TUI/date patterns already listed
in §9 — nothing node/index/parser-specific has been logged there from prior tickets.

---

## 12. Open questions for planning (concrete, numbered)

1. **`rk today` name collision** (§3): replace `today.go`'s legacy journal-dump
   `RunE` outright, gate it behind a flag, or something else? No test asserts the old
   behavior, but it's a real, currently-working user-facing feature.
2. **`pinned`/today-pin prop:** exact name and value shape (boolean string? a date?
   distinct from `scheduled`?). Also: does `t` (today-pin) *set* `scheduled` to
   today, or set an independent `pinned` flag that's orthogonal to `scheduled`? The
   design text lists "scheduled-today" and "today-pinned" as two *separate* source
   buckets in the same query, implying today-pin is not merely "scheduled==today" —
   needs an explicit decision.
3. **`priority` prop:** exact name, value domain (letter A/B/C? numeric? open
   string?) — zero precedent anywhere.
4. **New `state` values for `i`/`c`:** exact strings (`"in-progress"`/`"cancelled"`
   or other spelling), and whether `rk todo list`'s existing default filter
   (`state != "open"` is hidden unless `--all`) needs to change so these new states
   are still visible/hidden sensibly.
5. **"Overdue" definition:** `scheduled < today`, `deadline <= today`, or an OR of
   both? No code or doc settles this precisely.
6. **Completion→did-log-entry wiring** (§5.3): extend `rk todo done`'s plain
   (non-recurring) path to always emit a did-linked entry (changing existing
   `rk todo done` behavior/tests), or implement it fresh inside `rk today`'s own
   completion handler, leaving `rk todo done` untouched? The design says "default
   on, toggleable" — a `--no-log`-style flag is implied either way.
7. **Does `c` (cancel) also emit a did-entry?** Design text only says "completion"
   emits one; cancellation isn't obviously "completion." Undecided.
8. **`d` (defer) sub-choice:** "tomorrow/next-week/pick" implies more than a single
   deterministic transform. As a non-interactive verb this is naturally
   `rk today defer <ref> <target>` with `target` = `tomorrow`|`next-week`|an explicit
   date; as an interactive key it needs some kind of in-list sub-prompt. Depends on
   §13/8.2's scope call.
9. **Interactive TUI vs. plain CLI plumbing for v1** (§8.2, the biggest call):
   explicit decision needed, with the evidence above (Lean v1 Scope's own listing,
   the fact zero prior v1 verb is interactive, and the "tested" bar) leaning toward
   CLI-plumbing-first.
10. **SPLIT actuator fixture convention** (§7): what marks a node as
    "externally-fed" (`type: work-ticket`? a `source`/`external` prop? both?), and
    what field carries the "open in source" jump target (a `url` prop?). Entirely
    new; needs a decision before tests can be written.
11. **Reindex granularity:** reconcile once at agenda-load and once after each
    actuation (mirroring `todo.go`'s `list`), or something cheaper/different? No
    per-file fast-reindex primitive exists (§4.2) — every `Reconcile()` call is a
    full vault walk with an mtime/hash fast-path, not a single-file update.
