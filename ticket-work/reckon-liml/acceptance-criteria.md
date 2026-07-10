# Acceptance Criteria — reckon-liml (v1-T7: `rk today` — split-actuator agenda)

Phase 1 (AC extraction), read-only. No Phase 0 `codebase-analysis.md` exists yet for this
ticket (only `ticket-work/reckon-liml/state.json`); grounding below comes directly from the
proposal doc and from reading the shipped T3/T4/T5/T6 implementations this ticket builds on.

**Grounding sources:**
- Ticket: `bd show reckon-liml` (description + "Done when" clause below).
- Proposal (source of truth for scope/semantics): `docs/design/composable-redesign.md` —
  "Design: query → schedule → do UX" (`rk today` proposal, lines 759–796), its sub-decisions
  (788–796), "Design: scheduling, reminders & recurrence" (991–1062), the round-trip/span-local
  GATING note (563–586), "Design: index freshness & rebuild" (893–937), the 2026-06-22
  amendments' split-actuator decision (1189–1191), and "Lean v1 scope" (1198–1213).
- `docs/design/composable-redesign-rebuttal.md` lines 100–126 (split-actuator confirmation:
  "read-only with an 'open in Jira/GH' jump… Confirmed acceptable — done-from-agenda never
  reaches a real work ticket").
- `docs/design/composable-redesign-assessment.md` lines 34, 41 (the risk that named split
  actuator as a decision point).
- `docs/design/km-architecture-proposal.md` line 152 (T7 still open), line 413 (`/startup`
  will read `rk today` once it lands).
- Shipped code this ticket sits directly on top of: `internal/cli/todo.go` (`rk todo`
  add/list/done, durable+ephemeral, span-local `SetField`, the `repeat:` recurrence branch
  `doneRecurringTodo` that already writes a linked `did::` log entry on completion),
  `internal/cli/add.go` (`appendLogEntry`/`appendDidLogEntry`/`writeLogEntryBlock` — the
  exact machinery a `did`-linked completion log entry would reuse), `internal/cli/query.go`
  (`rk query`, read-only SQL over the stable views), `internal/index/schema.go` (public views
  `nodes`/`edges`/`node_props`/`aliases`/`fts`/`fts_search` an agenda SELECT would run
  against), `internal/index/reconcile.go` (`Index.Reconcile()`), `internal/cli/recur.go`
  (`parseSchedDate` — strict `YYYY-MM-DD`, UTC, date-only), and the **pre-existing, unrelated**
  `internal/cli/today.go` / `internal/cli/schedule.go` (legacy journal-based `rk today`/
  `rk schedule` commands, already wired into `RootCmd` at `root.go:120,124` — see EC-11).

---

## 1. Explicit acceptance criteria (verbatim-grounded, numbered)

From the ticket's "Done when" clause: *"the agenda surfaces the correct set; actuation writes
through to files and reindexes; completion logs a did entry; native rows actuate while stubbed
external rows are read-only; tested."* Split into independently verifiable items, each cross-
referenced to the proposal text that gives it its precise shape:

- **AC-1 (correct surfaced set).** `rk today` surfaces exactly the union of: overdue items
  (`scheduled` or `deadline` date < today), items `scheduled` for today, and items carrying a
  today-pin — no more, no less. [PROPOSAL, line 764: "`rk today` queries overdue +
  scheduled-today + today-pinned + today's reminders"] **Reminders are explicitly excluded
  from this ticket's scope** — see §5 — so the *implemented* set for reckon-liml is overdue +
  scheduled-today + today-pinned only; the ticket's own description already narrows the
  proposal's four-part surface to three parts via "(+ reminders later)".

- **AC-2 (in-list relative scheduling actuates natively, writes through).** The schedule keys
  `t`/`d`/`D`/`p` mutate a native row's frontmatter **in place, in the agenda**, and the write
  lands on disk as a real file mutation (not a queued/staged change) — "writes straight to the
  task file" [PROPOSAL, line 769]. Reindexing then makes the mutation visible to `rk query`
  and to the next `rk today` load. [Ticket: "actuation writes through to files and reindexes"]

- **AC-3 (in-list do-keys actuate natively, writes through).** The do keys `x`/`i`/`c` mutate a
  native row's state in place with the same write-through-and-reindex guarantee as AC-2.
  [PROPOSAL, line 773: "`x`=done · `i`=in-progress · `c`=cancel"]

- **AC-4 (completion logs a linked `did` entry).** Completing a native row (the `x` key) emits
  a `log-entry` node **linked `did`→task** — two nodes joined by an edge, not a merge into the
  task or a rewrite of the task's own body — closing the loop into the journal/log and giving
  time-tracking for free. [PROPOSAL, lines 773–776, 794–795: "Completion = emits a linked
  `log-entry` (`did`→task) by default, toggleable. Two nodes + one edge — preserves the hard
  task/log separation."] This is default-on and must be toggleable per the proposal, though the
  ticket's "Done when" only requires the default-on behavior to be demonstrated.

- **AC-5 (native vs. external — split actuator).** Rows sourced from native reckon nodes
  (durable/ephemeral todos) are actuatable via AC-2/AC-3's keys. Rows representing externally-
  fed work tickets (Jira/GitHub-style) are **read-only in the agenda** — none of `t`/`d`/`D`/
  `p`/`x`/`i`/`c` may mutate them — and instead expose an **open-in-source jump** action.
  [PROPOSAL/amendment, lines 1189–1191: "native nodes actuated in-list; external work tickets
  read-only + jump (driving a ticket done *from the agenda* would be the reconcile engine the
  owner refused)"; rebuttal doc lines 100–102, 125–126: "read-only with an 'open in Jira/GH'
  jump, fed by an `rk-jira`/`rk-gh` [extension]… Confirmed acceptable — done-from-agenda never
  reaches a real work ticket."] The **feed itself is stubbed in v1** — see §5 — so this AC's
  real-world test surface in this ticket is: (a) the agenda's actuator dispatch correctly
  rejects/no-ops mutation attempts against a row whose `kind`/`source` marks it external, and
  (b) the jump action is present and points at *something* resolvable, exercised against a
  synthetic/stub external row since no live `rk-jira`/`rk-gh` producer exists to generate one.

- **AC-6 (tested).** AC-1..AC-5 are each covered by automated tests, following the
  `internal/cli` harness conventions already established by the T3/T5/T6 test suites
  (`query_test.go`, `todo_test.go`, `todo_recur_test.go`: `setupQueryVault`/`writeTestNode`-
  style fixtures, `resetCLIFlags`, `buildIndex`, a `runToday`-shaped helper of the same form as
  `runQuery`/`runTodo`).

---

## 2. Implicit requirements — inferred/pinned-down semantics

The ticket text names the keys and verbs; the proposal gives partial semantics; several
concrete behaviors are **not stated anywhere** and must be inferred or flagged as genuinely
open. Each item below is marked **[PROPOSAL]** (explicit in the design doc), **[TICKET]**
(explicit in the ticket text), or **[INFERRED]** (this document's extraction, not settled by
either source — Phase 2/planning must confirm or override).

### 2.1 Relative scheduling keys (`t`/`d`/`D`/`p`)

| Key | Meaning | Grounding | Concrete effect |
|---|---|---|---|
| `t` | today-pin | [PROPOSAL, 768] "`t`=today-pin" | **[INFERRED]** Sets some pin marker (e.g. a boolean-ish `pinned: today` frontmatter prop, or a `scheduled:` set to today's date — the proposal doesn't say which) on the native node so the row surfaces under "today-pinned" on future agenda loads even if not `scheduled`/overdue. **No `pin`/`priority` prop exists anywhere in the current codebase** (`internal/node`, `internal/cli`, `internal/index/schema.go` were grepped — zero hits) — this ticket introduces the concept from scratch. **[OPEN]**: exact prop name/representation. |
| `d` | defer | [PROPOSAL, 768] "`d`=defer (tomorrow/next-week/pick)" | **[INFERRED]** Sets/advances the node's `scheduled:` (do-date) prop — not `deadline:` — to tomorrow, next-week, or a user-picked date, offered as an in-list sub-choice (the "pick" option implies a secondary prompt, breaking the "≤1 keystroke" claim at line 762 for that one sub-path — likely 1 keystroke + a follow-up date entry only when "pick" is chosen). Uses the same `YYYY-MM-DD` date format and UTC date-only semantics as `todo.go`'s existing `--scheduled` flag and `recur.go`'s `parseSchedDate`. |
| `D` | deadline | [PROPOSAL, 768–771] "`D`=deadline"; "Model = org's: a `scheduled` do-date + a hard `deadline`… + a today-pin" | **[INFERRED]** Sets/updates the node's `deadline:` prop, reusing the existing `deadline` prop already supported by `rk todo add --deadline` (`todo.go:120`) and read back by `rk todo list` (`todo.go:169`) — this key is not new plumbing, just a new *writer* of an existing field. |
| `p` | priority | [PROPOSAL, 768] "`p`=priority" | **[OPEN, INFERRED]** The proposal names the key but never specifies a priority *scale* (org uses `[#A]`/`[#B]`/`[#C]`; nothing here pins one) or a prop name. No `priority` prop exists anywhere in the codebase today. This is the least-specified of the four keys — Phase 2 must pick a concrete scale/prop before implementation, and note it explicitly rather than silently inventing one that later needs a breaking migration. |

**Cross-cutting for all four keys — [PROPOSAL, GATING, lines 572–586]:** every write must be
**span-local** — rewrite only the one frontmatter field being changed (`SetField`, mirroring
`todo.go`'s `n.SetField("state", "done")` pattern at line 685) — and must **byte-preserve**
every other span of the file (unmodeled frontmatter, body, blockquotes, code fences). Full-file
regeneration is explicitly called out as "the one failure the whole design exists to prevent."

### 2.2 Do keys (`x`/`i`/`c`)

| Key | Meaning | Grounding | Concrete effect |
|---|---|---|---|
| `x` | done | [PROPOSAL, 773] | Sets `state: done` via span-local `SetField` — the exact mechanism `todo.go`'s `doneDurableTodo` already implements (line 685) — **and** additionally triggers the AC-4 linked-`did`-entry write. If the target node carries a `repeat:` prop, `x` should take the **existing recurrence branch** (`doneRecurringTodo`, `todo.go:697-790`) rather than reimplement cursor-advance logic: that function already (a) advances `scheduled:` instead of flipping `state`, (b) writes a `did::`-linked log entry via `appendDidLogEntry` (`add.go:201`), and (c) materializes a pile-up instance when occurrences were missed. **[INFERRED]**: `rk today`'s `x` should call into `rk todo done`'s existing logic (durable and ephemeral) rather than duplicate it — consistent with the proposal's "CLI verbs sit *underneath* as the plumbing the TUI and the agent both call — same verbs, two surfaces" (lines 782–785). |
| `i` | in-progress | [PROPOSAL, 773] | **[OPEN, INFERRED]** Sets `state: in-progress` (or similar). **No such state value exists today** — `internal/cli/todo.go` only ever writes/reads `state: open` / `state: done` (lines 344, 514, 679, 685, 692). Introducing `in-progress` is new: the default `rk todo list` filter (`!all && state != "open"` at line 519) would need to decide whether `in-progress` counts as "open" for default-list purposes — not specified anywhere, must be confirmed in Phase 2. |
| `c` | cancel | [PROPOSAL, 773] | **[OPEN, INFERRED]** Sets `state: cancelled` (or `canceled`/`cancel` — spelling unpinned). Also entirely new; same default-list-filter question as `i` applies (does a cancelled item still show under `--all`? presumably yes, but unstated). |

**Cross-cutting for `x`/`i`/`c` (AC-4 detail) — [PROPOSAL, 794–795]:** "Two nodes + one edge" —
the linked log entry is a **separate `log-entry` node** (day-group file, per the shipped `rk
add`/`appendLogEntry`/`appendDidLogEntry` machinery in `add.go`) joined to the task by a `did`
edge, exactly mirroring the `did::` marker syntax `doneRecurringTodo` already emits
(`node.RenderLogEntryWithDid`, `add.go:209`). **[INFERRED]**: only `x` (done) triggers the
linked-log-entry write by default — the proposal's "Completion emits…" language (773) reads as
specific to *completion*, not to `i`/`c`; whether `i` (in-progress, a state *transition*, not a
completion) or `c` (cancel) also log is **[OPEN]**, and a reasonable default is "no" for both
(neither is "did" the task, in the org sense) pending Phase 2 confirmation. Only `x` is
unambiguously required to log, per AC-4's own grounding.

### 2.3 Span-local writes (why this matters beyond §2.1/2.2)

**[PROPOSAL, GATING, lines 572–586]** directly: "A tool rewrites **only the spans it owns**
… and byte-preserves all unmodeled prose. Do *not* regenerate the whole file." This is not
agenda-specific — it's the same invariant `rk todo`/`rk add`/`rk index` already honor — but it
is worth stating explicitly for `rk today` because the agenda is the first surface where a
**single user keystroke** triggers a write to a file the agenda itself did not just create
(unlike `todo add`, which always creates fresh). Any implementation that re-renders the whole
node (`Render()`+full overwrite) instead of `SetField`-then-serialize risks silently discarding
hand-authored content (Obsidian extra frontmatter, prose, code fences) sitting in the same file
— exactly the failure class the round-trip fuzz gate (`internal/spike/roundtrip/`,
`spike-roundtrip-verdict.md`) was built to catch.

### 2.4 Reminders — explicitly deferred, not merely unimplemented

**[TICKET]**, literally: "Query overdue + scheduled-today + today-pinned **(+ reminders
later)**" — the parenthetical explicitly pushes reminders out of this ticket, independent of
the proposal's own reminder design (which exists — lines 999–1007 — but is gated behind the
`on-reminder-due` hook seam, itself the *only* hook realized in the "Lean v1 scope," lines
1198–1213). This ticket's agenda surface (AC-1) is overdue + scheduled-today + today-pinned
only; no reminder rows, no `remind:` prop reads, no `on-reminder-due` wiring.

---

## 3. Edge cases to handle

| # | Edge case | Expected behavior (grounded) |
|---|---|---|
| EC-1 | Empty agenda (no overdue, nothing scheduled today, nothing pinned) | Clean "nothing due" result — exits 0, `--json` emits an empty items array (mirrors `todo list`'s `{"items": []}` convention, `todo.go:464`), pretty mode prints a short "no items" line, never an error. |
| EC-2 | A task is both overdue (past `scheduled`/`deadline`) **and** carries a today-pin | **Dedupe required.** The task must appear **exactly once** in the agenda, not once per matching criterion — AC-1's "correct set" reading is a *set* (union), not a *multiset*. Recommend: query by `id`/`node_key` with `DISTINCT` or a `UNION` (not `UNION ALL`) across the three sub-queries (overdue / scheduled-today / today-pinned), or equivalently a single query with an `OR` across the three conditions grouped by node. Row-level metadata (e.g. "why is this here" — overdue vs. pinned vs. both) is a **[INFERRED, nice-to-have]** display concern, not required by any AC. |
| EC-3 | A task is scheduled today **and** also today-pinned | Same dedupe as EC-2 — appears once. |
| EC-4 | Actuation attempted on a stubbed-external row (`t`/`d`/`D`/`p`/`x`/`i`/`c` pressed on a non-native row) | Must be **rejected, not silently ignored and not crash-worthy**: the agenda gives visible feedback (e.g. a status-line message) that the row is read-only, and no file write of any kind occurs against it. At the CLI-verb layer (if actuation is also exposed as a non-interactive verb, e.g. `rk today act <ref> <key>`), this should be a clear non-zero-exit error, distinguishable from AC-2/AC-3's success case — mirrors the "no silent no-op" convention `todo.go`'s `doneDurableTodo` already uses for a not-found ref (line 660). |
| EC-5 | Midnight / date-boundary rollover while the agenda is open (a long-lived session, or a row computed against "today" at load time vs. actuation time) | **[INFERRED, no direct proposal text].** "Today" must be computed once per load/action against a single consistent clock (per `recur.go`'s existing UTC-date-only discipline, `todo.go`'s `todoNow` seam pattern, line 33) — an agenda open across local midnight should not silently reclassify an item mid-session in a way that corrupts an in-flight write; recommend recomputing "today" fresh at actuation time (not reusing a stale load-time value) since the write itself is what matters for correctness, and a stale display is cosmetic. Should reuse the same UTC-vs-local convention already locked in for `scheduled`/`deadline` (date-only, floating, per proposal line 1052) to avoid the exact class of bug `add.go`'s `effectiveLogDate` comment (lines 137–157) documents was already found and fixed once in this codebase for a mixed-clock composition bug. |
| EC-6 | Missing `scheduled`/`deadline` on a candidate node (most todos, in fact — both are optional per `todo.go:119-120`) | Must not crash or misclassify: absence of `scheduled` simply means the node is not eligible for the "overdue" or "scheduled-today" buckets (only "today-pinned", if separately pinned, could still surface it) — same "absent, not empty-string" prop convention already established (`todo.go`'s `if scheduled != ""` gating at line 345). |
| EC-7 | Invalid/malformed `scheduled` or `deadline` value on a node (hand-edited to something not `YYYY-MM-DD`, e.g. free text, a partial date, or a different format) | Must not crash the whole agenda load. Per `recur.go`'s existing `parseSchedDate` (strict `time.ParseInLocation("2006-01-02", …)`), a malformed date should cause that **one row** to be skipped/flagged (with a visible warning, not a silent drop) rather than aborting `rk today` for every task — mirrors the design's general "parser tolerates/skips and logs, never crashes" posture for malformed input (index freshness section, line 934: "Malformed files… parser tolerates/skips and logs, never crashes the reconcile"). |
| EC-8 | Concurrent file edits between index read and actuation write (another process/editor changes the file on disk between when the agenda loaded it and when a keystroke writes to it) | **[INFERRED, no direct proposal text — nearest analog is the index-freshness design's general posture].** The write path should not blindly trust agenda-load-time state: at minimum, re-read-and-reparse the target file immediately before the span-local `SetField`/write (as `todo.go`'s `doneDurableTodo` already does — it re-resolves and re-parses the file fresh on every invocation rather than trusting cached state, lines 640-661) so the mutation is applied against current on-disk bytes, not a stale in-memory snapshot. A full transactional conflict-detection scheme (hash-compare-and-swap) is **not** implied by anything in the proposal and would be scope creep; "always re-read before writing" is the minimum bar already met by the pattern this ticket reuses. |
| EC-9 | Actuation succeeds on disk but the subsequent reindex/write-through step fails (partial failure) | Per the index-freshness design's "write-through (optimization): reckon tools update the index inline on write → instant agenda; the next reconcile then finds nothing to do. **Not required for correctness**" (lines 908–910) — the **file write is authoritative**; if the inline index update fails or is skipped, the *next* lazy reconcile-on-read (triggered automatically before the next `rk today`/`rk query` load, per lines 904–907) self-heals. The agenda must not roll back or fail the user-visible action just because the optimization step had trouble — mirrors `doneRecurringTodo`'s own documented ordering: "the cursor advance is the authoritative write and happens first… the already-advanced cursor is not rolled back" (`todo.go:706-711`). |
| EC-10 | A recurring todo (`repeat:` prop) shows up in the agenda and gets `x`'d | Must route through the existing `doneRecurringTodo` cursor-advance path (§2.2), not the plain state-flip path — the row's `state` stays `open`/unchanged (per `todoDoneResult`'s doc comment, `todo.go:203-208`: "State stays 'open' … the cursor advance is the completion signal, not a state flip") and the row's `scheduled:` advances instead, potentially still leaving it in the agenda's "scheduled-today" or "overdue" bucket on next load if the new date qualifies. |
| EC-11 | Name collision: a **legacy** `rk today` command already exists (`internal/cli/today.go`, wired at `root.go:120`) that dumps the current day's **journal** content — an entirely different, DB/journal-backed feature predating the node-based redesign | **[INFERRED, real implementation risk, not addressed by any doc found].** The proposal's `rk today` (this ticket) and the shipped legacy `rk today` are two different commands with the same name. Someone must decide: replace the legacy command outright, rename the legacy one, or namespace the new agenda differently (`rk agenda`?) before this ticket can wire a command named `today` into `RootCmd` without a collision. This is a scope/naming risk worth flagging loudly in Phase 2, not something this AC-extraction phase can resolve. |
| EC-12 | Ephemeral todos (no stable address, container-line items) appearing in the agenda | Must be actuatable too — the proposal's "native nodes actuated in-list" (line 1189) doesn't distinguish durable from ephemeral, and `rk todo done --ephemeral <index>` already exists as the underlying verb (`todo.go:839`). Scheduling keys (`t`/`d`/`D`/`p`) are murkier: `rk todo add --ephemeral` explicitly rejects `--scheduled`/`--deadline` today (`todo.go:277-279`, "durable-only") — so whether an ephemeral row can even be scheduled/deadlined/pinned from the agenda is **[OPEN]**; a defensible default is "ephemeral rows support only the do-keys (`x`/`i`/`c`), not the schedule-keys," consistent with the existing durable-only restriction, but this needs Phase 2 confirmation rather than silent invention. |

---

## 4. Test scenarios (Given/When/Then), mapped to acceptance criteria

### AC-1 — correct surfaced set

**TS-1.1** (overdue surfaces)
Given a durable todo with `scheduled: <yesterday's date>` and `state: open`,
When `rk today` is run,
Then that todo appears in the agenda output.

**TS-1.2** (scheduled-today surfaces)
Given a durable todo with `scheduled: <today's date>`,
When `rk today` is run,
Then that todo appears in the agenda output.

**TS-1.3** (today-pinned surfaces)
Given a durable todo with no `scheduled`/`deadline` at all, but carrying whatever pin
representation Phase 2 settles on (§2.1),
When `rk today` is run,
Then that todo appears in the agenda output.

**TS-1.4** (future-scheduled excluded)
Given a durable todo with `scheduled: <tomorrow's date>` and no pin,
When `rk today` is run,
Then that todo does **not** appear in the agenda output.

**TS-1.5** (done items excluded)
Given a durable todo with `scheduled: <today's date>` but `state: done`,
When `rk today` is run,
Then that todo does not appear (mirrors `rk todo list`'s default open-only filter).

**TS-1.6** (dedupe — EC-2)
Given a durable todo with `scheduled: <yesterday>` (overdue) **and** a today-pin set,
When `rk today` is run,
Then that todo appears exactly once in the output, not twice.

**TS-1.7** (empty agenda — EC-1)
Given a vault with zero todos (or all todos future-scheduled/done/unpinned),
When `rk today` is run,
Then the command exits 0 with an empty result set, not an error.

### AC-2 — in-list schedule-key actuation writes through

**TS-2.1** (`d` defer writes `scheduled:` span-locally)
Given an overdue native todo file with hand-added extra frontmatter and body prose,
When the agenda's `d`→tomorrow action is invoked against that row,
Then the file's `scheduled:` line is updated to tomorrow's date, every other byte (extra
frontmatter, body, prose) is unchanged, and a subsequent `rk query`/`rk today` load reflects
the new date (reindexed).

**TS-2.2** (`D` sets deadline)
Given a native todo with no `deadline:` prop,
When the agenda's `D` action with a supplied date is invoked,
Then the file gains a `deadline:` prop with that value, span-locally.

**TS-2.3** (`t` today-pin)
Given a native todo not currently pinned,
When the agenda's `t` action is invoked,
Then the file's pin representation (per Phase 2's chosen mechanism) is set, and the todo
subsequently appears in `rk today` even without matching overdue/scheduled-today.

**TS-2.4** (`p` priority)
Given a native todo with no priority set,
When the agenda's `p` action is invoked with a value,
Then the file gains the chosen priority representation, span-locally (exact assertion depends
on Phase 2's pinned scale/prop name).

**TS-2.5** (write-through visible without manual reindex)
Given a fresh `d`/`D`/`t`/`p` actuation just performed via the agenda,
When `rk query "SELECT …"` (or the next `rk today` load) is run immediately after, with no
manual `rk index` step in between,
Then the mutation is visible — satisfying "actuation writes through to files and reindexes."

### AC-3 — in-list do-key actuation writes through

**TS-3.1** (`x` on a plain native todo)
Given a native todo with `state: open`,
When the agenda's `x` action is invoked,
Then the file's `state:` becomes `done` via a span-local edit (byte-identical elsewhere), and
the row drops out of the next `rk today` load (no longer open).

**TS-3.2** (`i` in-progress)
Given a native todo with `state: open`,
When the agenda's `i` action is invoked,
Then the file's `state:` becomes the chosen in-progress value (Phase 2 to confirm exact
string), span-locally, and the row's continued presence/absence in the default agenda view
matches whatever default-list-filter decision Phase 2 makes (§2.2).

**TS-3.3** (`c` cancel)
Given a native todo with `state: open`,
When the agenda's `c` action is invoked,
Then the file's `state:` becomes the chosen cancelled value, span-locally.

**TS-3.4** (`x` on an ephemeral row — EC-12)
Given an ephemeral todo checkbox item, unchecked,
When the agenda's `x` action is invoked against that row,
Then the checkbox flips `- [ ]` → `- [x]` via the existing single-byte splice mechanism
(`flipChecklistLine`, `todo.go:931`), reusing `rk todo done --ephemeral` under the hood.

### AC-4 — completion logs a linked `did` entry

**TS-4.1** (`x` emits a linked log entry)
Given a native todo with `state: open`,
When the agenda's `x` action is invoked,
Then, in addition to TS-3.1's state flip, a new `log-entry` node is appended to today's log
day file (`log/<date>.md`) carrying a `did::`-style marker referencing the completed todo's
ULID, and `rk query` on `edges` shows a `rel='did'` edge from the new log entry to the todo's
node (mirroring the existing recurring-todo `did::` mechanism in `doneRecurringTodo`/
`appendDidLogEntry`).

**TS-4.2** (recurring todo `x` reuses the T6 path, not a duplicate)
Given a native todo with a `repeat:` prop and a `scheduled:` cursor,
When the agenda's `x` action is invoked,
Then the behavior matches `rk todo done`'s existing recurrence branch exactly: `state` stays
`open`, `scheduled:` advances per the repeater arithmetic, a `did`-linked log entry is written,
and (if intervals were missed) an ephemeral catch-up instance is materialized — i.e. `rk
today`'s `x` does not reimplement this logic with different behavior.

**TS-4.3** (toggle off — proposal's "toggleable" clause)
Given the linked-log-entry behavior is toggled off (mechanism TBD by Phase 2 — flag/config),
When the agenda's `x` action is invoked,
Then the todo's `state` still becomes `done`, but no `log-entry` node/`did` edge is created.

**TS-4.4** (`i`/`c` do not log by default — §2.2 inference)
Given a native todo with `state: open`,
When the agenda's `i` or `c` action is invoked,
Then no `log-entry`/`did` edge is created (only `x` triggers AC-4's logging, per the
default inferred in §2.2, pending Phase 2 confirmation).

### AC-5 — split actuator (native vs. external)

**TS-5.1** (external row is unactuatable)
Given an agenda that includes one native todo row and one synthetic/stub external-work-ticket
row (constructed directly as a test fixture, since no live `rk-jira`/`rk-gh` feed exists),
When any of `t`/`d`/`D`/`p`/`x`/`i`/`c` is invoked against the external row,
Then no file anywhere in the vault is modified as a result, and the agenda surfaces a clear
"read-only" indication rather than silently doing nothing or crashing.

**TS-5.2** (external row exposes a jump)
Given the same external-work-ticket fixture,
When the agenda's "open in source" action is invoked against that row,
Then the action resolves to *something* identifying the external source (a URL/reference
field carried on the stub row) rather than erroring — exact resolution mechanism (open browser?
print URL? per Phase 2) is not pinned by the proposal, only that the affordance exists and is
distinct from the native actuation keys.

**TS-5.3** (native row unaffected by the split)
Given the mixed agenda from TS-5.1,
When `x` is invoked against the *native* row specifically,
Then it actuates normally (per TS-3.1), confirming the split is per-row, not global.

### AC-6 — tested

**TS-6.1** (harness reuse)
Given the existing `internal/cli` test harness conventions (`setupQueryVault`/
`writeTestNode`-equivalent fixtures, `resetCLIFlags`, `buildIndex`, per `query_test.go`/
`todo_test.go`/`todo_recur_test.go`),
When a `runToday(t, vault, args...) (stdout, stderr string, err error)` helper is added in the
same shape as `runQuery`/`runTodo`,
Then all TS-1.x .. TS-5.x scenarios above are expressible as table-driven or per-scenario tests
using that helper, with no bespoke one-off scaffolding required.

---

## 5. Explicitly OUT of scope for v1

- **Reminders** — the ticket's own text defers this: "overdue + scheduled-today +
  today-pinned **(+ reminders later)**." No reminder rows in the agenda, no `remind:` prop
  reads, no `on-reminder-due` hook wiring. [TICKET + PROPOSAL, hooks section: "Realize now:
  `on-reminder-due`… Defer all others until a concrete use appears" — but even the realized
  `on-reminder-due` hook is about *delivery*, not agenda surfacing, and reminders as an agenda
  *source* are the deferred piece here.]
- **A live external work-ticket feed** (`rk-jira`/`rk-gh`). [PROPOSAL, lines 1186-1188, 1211:
  "Work task model… lives in `rk-jira`/`rk-gh` extensions"; "Lean v1 scope… DEFERRED… work
  feeders `rk-jira`/`rk-gh`"] This ticket's split-actuator behavior (AC-5) must be built and
  tested against a **synthetic/stub** external row shape, since the real feeder does not exist
  yet — "feed stubbed in v1" per the ticket text itself.
- **Round-trip-to-source actuation for external tickets** (marking a Jira ticket done *from*
  the agenda, syncing state back to the external system). [PROPOSAL/amendment, lines 1189-1191:
  "driving a ticket done *from the agenda* would be the reconcile engine the owner refused."]
  Explicitly, permanently out of scope, not just deferred — this is a design decision, not a
  sequencing one.
- **Agent-assisted daily planning / propose-and-confirm plan pre-fill.** [PROPOSAL, lines
  765-767, 792-793: "The agent optionally pre-fills a proposed plan… Agent daily planning =
  propose-and-confirm (`[a]ccept/[e]dit/[r]eject`)."] This is part of the full `rk today`
  vision but is an *agent porcelain* layered on top of the deterministic agenda surface/
  actuator this ticket builds — the ticket's "Done when" clause (surfacing, actuation,
  logging, split-actuator, tested) makes no mention of agent pre-fill, and it depends on
  agent-porcelain infrastructure (`docs/design/composable-redesign.md`'s "AI as a first-class
  participant" section) not gated by this ticket's dependency list.
- **Read-time synthesis / `rk-brief`.** [PROPOSAL, lines 1183-1185, 1210: "Read-time synthesis
  is NOT core. Reserve an `rk-brief` extension seam"] Not part of the agenda surface itself.
- **Recurrence mechanics themselves** (repeater parsing/advance arithmetic, pile-up
  materialization) — already shipped in T6 (`reckon-ar9m`, closed) and reused, not reimplemented,
  by this ticket per §2.2/EC-10.
- **The checklist tool, multi-level fragments, hooks beyond `on-reminder-due`, the
  `--ref/--pop/--cp` disposition zoo** — all explicitly named in the "Lean v1 scope" deferred
  list (lines 1210-1213), none of them touch `rk today`.
- **MCP/agent-surface wiring for `rk today`** — `reckon-cxx1` (v1-T10, fast-follow), which
  depends on this ticket per `bd show reckon-p0zs`'s BLOCKS list; this ticket only needs
  `rk today` to be agent-ergonomic at the CLI/verb level (structured output), not MCP-wired.
- **Migrating/removing the legacy DB-journal-backed `rk today`/`rk schedule`/`rk task`
  commands** (`internal/cli/today.go`, `schedule.go`, `task.go`, `journal` package) — that is
  `reckon-s6oh` (v1-T9)'s job. This ticket must, however, **resolve the naming collision** with
  the legacy `today.go` command (EC-11) at least at the command-wiring level, even though the
  broader legacy-data migration is out of scope.
