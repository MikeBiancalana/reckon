# Acceptance Criteria — reckon-ar9m (v1-T6: recurrence via stored `scheduled` cursor, org-style)

Phase 1 (AC extraction), read-only. No implementation code was read for this
document; only ticket text, sibling-ticket AC/plan/summary docs, and design
docs (which are documentation of intent, not implementation) were consulted.

Ticket text (verbatim):

> Recurring todos without log-derived fragility. repeat: org repeaters (+Nd
> fixed, ++Nd skip-to-future, .+Nd from-completion); on completion advance the
> rule node scheduled: prop via span-local edit AND write a did log entry as
> audit; surface the current occurrence; pile-up materializes an ephemeral
> instance. Cursor lives in text (frontmatter), so it survives index rebuild.
> Done when: a recurring todo advances correctly for each repeater family on
> completion; a did entry is written; blowing away+rebuilding the index
> preserves the cursor; tested.

Grounding sources: `bd show reckon-ar9m` / `reckon-qiua` / `reckon-uv09`;
`docs/design/composable-redesign.md` ("Recurrence (A#3b/c)" §, its
2026-06-22 amendment, the "hooks" §, the `rk today` / "complete-as-logging"
§, the A#3d timezone/floating-date §); `docs/design/composable-redesign-
rebuttal.md` (§C4, the definitive "store the cursor, don't derive it"
rationale); `ticket-work/reckon-qiua/{acceptance-criteria,plan,summary}.md`
(T5's durable/ephemeral todo data model — `state`/`scheduled`/`deadline`/
`depends` props, `SetField` span-local edit mechanics, ephemeral = no stable
address); `ticket-work/reckon-uv09/{acceptance-criteria,summary}.md` (T4's
`rk add` log-capture primitive, day-group-file shape, `resolveAuthor`
provenance, freshness/index-reconcile contract); `internal/node/AGENTS.md`
(byte-preservation invariant, generic scalar-prop handling, the `LogParser`/
`id::`-marker mechanism and its *current* lack of any per-entry typed-Link
capability beyond the day's synthetic `contains` edge); `internal/index/
AGENTS.md` (edges/nodes/node_props view shapes).

---

## Open questions for the planner (do not guess past these)

Collected here for visibility; each is also raised inline below. None of
these should be silently resolved by whoever plans/implements next — they
materially change which code paths need tests.

1. **Pile-up trigger mechanism.** The system is explicitly "invocation-driven,
   not ambient — no daemon" (design doc's hooks §). So *when* does pile-up
   detection actually run: as a side effect of `rk todo done`'s advance
   computation (recommended default — see §2.4), or as a side effect of a
   read path (`rk todo list`, an index reconcile) noticing a rule is more
   than one interval overdue even with no completion happening? The ticket's
   own "Done when" sentence bundles this with on-completion behavior, but the
   design doc's own phrasing ("a new occurrence comes due while the prior is
   still open") doesn't strictly require a completion action to occur first.
2. **Did-entry linking mechanism.** "Audit" implies traceability back to
   *which* rule/occurrence completed — but `internal/node/AGENTS.md`
   confirms log-entry sub-nodes currently carry only `ULID`/`Time`/`Author`/
   `Body`/`Props["kind"]`; there is no existing per-entry typed-`Link`
   mechanism (the day's `contains` edge is the *only* structural edge
   `LogParser` synthesizes today, and it points day→entry, not entry→task).
   Building a real `did`→task edge (mirroring `rk today`'s stated future
   convention) is new log-parser work this ticket may or may not be
   expected to do. See §2.3.
3. **How many ephemeral instances does one pile-up event materialize** — one
   per missed cycle, or exactly one representing "there is a backlog"? The
   ticket's own phrasing ("materializes **an** ephemeral instance," singular)
   leans toward the latter. See §2.4/EC-8.
4. **Does `rk todo done` gain a `--repeat`-completion-date override** (for
   deterministically testing "late"/"early" completion in automated tests),
   or do tests simulate lateness purely by fixture-authoring a past/future
   `scheduled:` value and running `done` at real wall-clock now? Recommended:
   the latter (no new flag) — simpler, and consistent with T5's `done` not
   accepting a backdate flag today. See §2.2.
5. **Is there any escape hatch to permanently stop a recurring rule** (delete
   `repeat:`, a `--stop-repeat` flag, `rk todo done --final`)? Not mentioned
   by the ticket; likely out of scope, but flagged since "recurring forever
   with no exit" is a real, foreseeable user need. See §5.
6. **`deadline:` repeaters.** The design doc's own recurrence section says
   repeaters apply to "`scheduled`/`deadline`" generically, but the ticket's
   "Done when" line only names `scheduled:` as the thing advanced. Recommend
   treating `deadline:` repeater-advance as out of scope for this ticket
   (§5) unless the planner decides otherwise.
7. **Repeater unit granularity.** The ticket's own examples (`+Nd`, `++Nd`,
   `.+Nd`) are day-only. Org itself also supports `w`/`m`/`y`. Recommend
   scoping this ticket to `d` (days) only, as literally specified, treating
   other units as a documented, deliberate non-goal rather than a bug if
   unsupported — see §5.

---

## 1. Explicit acceptance criteria

Directly from the ticket's "Done when" sentence, decomposed into
independently verifiable items:

- **AC-1 (repeater families advance correctly on completion).** For a
  durable todo carrying both a `repeat:` prop (one of `+Nd`, `++Nd`, `.+Nd`)
  and a `scheduled:` prop, completing it (`rk todo done <ref>`, per T5's
  existing verb) computes a new `scheduled:` date per that family's specific
  algorithm (§2.1) and writes it back via a **span-local edit** (`SetField`
  on the `scheduled:` key only — no other byte in the file changes).
- **AC-2 (did entry written as audit).** Completing a recurring todo (one
  with `repeat:` set) additionally writes a new log entry — via the existing
  log-capture primitive (`rk add`, T4) — recording that this occurrence was
  completed, with non-empty author/provenance (reusing T4's guarantee that
  every log entry carries `author`).
- **AC-3 (current occurrence is surfaced).** After the advance, the
  recurring todo continues to appear in the ordinary "open items" view
  (`rk todo list` with no `--all` flag, per T5) showing its *new* `scheduled:`
  value — i.e., completing one cycle does not remove the rule from the
  actionable list the way a normal one-off `done` removes a todo (see §2.2,
  the `state`-must-stay-non-terminal implicit requirement this forces).
- **AC-4 (pile-up materializes an ephemeral instance).** When completion
  detects that more than one repeater interval has elapsed since the last
  `scheduled:` cursor (i.e., occurrences "piled up" while the rule sat
  uncompleted), a concrete ephemeral todo instance (per T5's ephemeral
  shape — a container-file checkbox line, no stable address) is created to
  represent the missed/outstanding occurrence(s), so they are not silently
  discarded by the cursor simply jumping forward.
- **AC-5 (cursor survives index rebuild).** After a recurring todo's
  `scheduled:` cursor has been advanced at least once, deleting the index
  database entirely and rebuilding it from scratch (`rk index` full rebuild,
  or equivalent) reproduces the exact same `scheduled:` (and `repeat:`)
  values when queried — because the cursor lives in vault text
  (frontmatter), never in the index, and rebuilding is a pure read/derive
  operation with zero writes back to text.
- **AC-6 (backward compatible — no `repeat:` means no new behavior).** A
  durable todo with no `repeat:` prop at all behaves under `rk todo done`
  exactly as specified by T5: `state` flips to `done`, no `scheduled:`
  mutation, no log entry emitted. Recurrence logic must be a strict
  additive branch, never a default path change for existing todos.
- **AC-7 (tested).** AC-1..AC-6 are each covered by automated tests,
  following the `internal/cli` harness conventions established by T5/T4
  (`setupQueryVault`/`writeTestNode`/`resetCLIFlags`/`buildIndex`/`runTodo`-
  style helpers), including at minimum one test per repeater family, the
  did-entry scenario, the index-rebuild scenario, and the pile-up scenario
  (see §4).

---

## 2. Implicit requirements

### 2.1 Org repeater semantics — precise algorithm per family

The ticket names three families with org's own shorthand labels
(`+Nd` fixed, `++Nd` skip-to-future, `.+Nd` from-completion). These map
directly onto org-mode's own documented repeater algorithms (this is what
"org-style" cues, and the ticket's own labels already pin the semantics —
this is not a novel design, just a faithful port). Let:

- `old` = the rule's current `scheduled:` date (the cursor before this
  completion).
- `today` = the calendar date on which `rk todo done` is invoked (a plain
  date, since `scheduled` is **date-only/floating**, per the design doc's
  A#3d timezone section — no time-of-day component).
- `N` = the repeater's numeric day-interval (a positive integer parsed out
  of the `repeat:` string).

| Family | New `scheduled` | Behavior in words |
|---|---|---|
| `+Nd` (fixed) | `old + N` (single hop, unconditional) | Anchored to the *last scheduled date*, oblivious to when you actually complete it. If you're more than one interval late, the result can still be in the past (overdue) — this is intentional org behavior, not a bug; it requires completing again to keep walking forward and "catch up." |
| `++Nd` (skip-to-future) | `old + k·N`, smallest positive integer `k` such that the result is strictly after `today` | Skips over any fully-elapsed past cycles automatically; never lands in the past. Identical to `+Nd`'s result whenever at most one interval has elapsed (the common case). |
| `.+Nd` (from-completion) | `today + N` | Ignores `old` entirely — always exactly `N` days after the day you actually completed it. Completing early or late both shift the *next* occurrence relative to `today`, not to the original schedule. |

This table is the concrete spec a test-writer needs; it should be treated as
settled (it's a direct, unambiguous reading of the ticket's own family
labels plus standard org-mode semantics), not an open design question.

### 2.2 Two separate frontmatter props, not one compound org timestamp

Org-mode embeds the repeater *inside* the same timestamp as the date
(`SCHEDULED: <2026-07-08 +7d>`). The ticket text instead names `repeat:` and
`scheduled:` as **two distinct frontmatter keys** ("`repeat:` org repeaters
(...); on completion advance the rule node `scheduled:` prop"). This is the
right reading for this codebase: `internal/node`'s frontmatter model is
`map[string]string`, one scalar value per key, each with its own
`fieldSpans` entry (`internal/node/AGENTS.md`) — a compound value would
need bespoke parsing the generic node package doesn't have and shouldn't
gain just for this. Concretely:

```
scheduled: 2026-07-08
repeat: +7d
```

**No `internal/node` (core package) changes are required.** `repeat:` is a
plain scalar string — it lands in `Props["repeat"]` for free via the exact
same generic mechanism `scheduled`/`deadline` already use (confirmed by
`internal/node/AGENTS.md`'s "Scalar values: one `key: value` per line" and
"per-tool parser's job" doctrine). All new logic (parsing the repeater
spec, computing the new date, deciding when to fire the did-log-write and
pile-up-materialize side effects) lives entirely in the CLI todo-tool layer
(`rk todo done`'s handler), exactly mirroring how T5's `done` already calls
`SetField("state", "done")`.

### 2.3 `state:` must not become permanently terminal for a recurring rule

T5's `rk todo done` sets `state: done` and (per its own default list
behavior) todos in that state are filtered out of `rk todo list`'s default
view. If T6 reused that exact behavior unmodified for a recurring rule, the
rule would vanish from the actionable list after its very first
completion — directly contradicting AC-3 ("surface the current
occurrence"), which requires the rule to remain visible, showing its next
due date, indefinitely across cycles. **Therefore:** completing a todo that
carries `repeat:` must leave `state` in a non-terminal condition (simplest:
never touch `state` at all during a recurrence-branch completion, since org
itself resets a repeating entry back to its "not done" keyword — the
`scheduled:` advance *is* the completion signal for a recurring rule, not
`state:done`). This is a load-bearing implicit requirement, not a detail —
get this wrong and AC-3 cannot pass even if the date math is correct.

### 2.4 Pile-up: precise trigger and materialization shape (recommended default)

Per the design doc ("An occurrence materializes into a real ephemeral task
instance ... lazily, on need: ... (b) piles up (a new occurrence comes due
while the prior is still open → both coexist)") and its rebuttal companion
("overdue/stacked cycles materialize into instance nodes ... each with its
own state"). Since the system has no daemon (invocation-driven only, per
the hooks §), detection can only happen when some command actually runs.

**Recommended default (needs planner confirmation, open question #1):**
detect pile-up as a side effect of the `rk todo done` advance computation
itself — specifically, when the number of intervals that have elapsed
between `old` and `today` is **more than one** (i.e., for `+Nd`, when
`old + N` is still `<= today`; for `++Nd`, when `k > 1` was needed; `.+Nd`
has no natural pile-up concept since it never looks backward — flag whether
pile-up even applies to `.+Nd` at all, see EC-7), materialize **exactly one**
ephemeral instance (open question #3; recommended default: one per
completion event, not one per skipped cycle) representing the
missed/outstanding occurrence, in addition to advancing the rule's own
cursor to the new (current/future) date.

**Recommended shape of the materialized instance** (per T5's ephemeral data
model — a checkbox line in the shared ephemeral container, no stable
ULID/address by design): body text that identifies which rule it came from
and what was missed (e.g. embedding the rule's alias/ULID as plain text or
a `[[wikilink]]` reference in the checkbox line), since ephemeral items
have no frontmatter and thus no `depends`-style structured ref-prop
available to them (T5 confirmed this: ephemeral items are "not applicable"
for `depends-on` edges). Traceability here is necessarily a text
convention, not a queryable typed edge — flag this limitation explicitly
rather than silently assuming a structured link exists.

### 2.5 Did-log entry: content and linking (open question #2)

`internal/node/AGENTS.md` confirms the *current* log-entry mechanism
(`LogParser`, `id:: <ULID>` marker, T4) gives every entry `ULID`/`Time`/
`Author`/`Body`/`Props["kind"]`, and the **only** structural edge it
synthesizes is the day node's `contains`→entry edge — there is no existing
per-entry typed-`Link` capability an entry could use to point back at a
completed task (the design doc's own `did`→task edge is stated as `rk
today`'s (T7's) future "completion machinery," not something T4 already
built). Concretely, this ticket's "did log entry as audit" needs one of:

  (a) **Body-text convention only** — the entry's body includes a
      `[[rule-alias-or-ULID]]` reference plus a short note ("completed,
      advanced to `<new-date>`"), relying on the generic body-wikilink
      mechanism (`rel = "references"`) already shipped for ordinary body
      links. Weakest structurally, but requires zero `LogParser` changes.
  (b) **A new inline marker**, analogous to `id::`, e.g. a `did:: <rule-
      ULID>` body line within the entry block, parsed by an extended
      `LogParser`/`buildLogEntry` into a real `Link{Rel: "did", To:
      <rule-ULID>}` — matches the design's stated `did`→task edge
      vocabulary and makes the audit trail queryable via `rk query`, but
      is new parser work.
  (c) Body text only, no structured reference at all (weakest — "audit"
      arguably requires at least (a)'s minimal traceability, so this is
      not recommended).

**Recommended default:** (a), since it needs no `internal/node`/`LogParser`
changes and satisfies "audit" at a basic (human/FTS-searchable, and
generically-linked) level; (b) is the more architecturally faithful choice
if the planner wants the `did`→task edge to already exist before T7 needs
it. Either way, **every** did-entry must carry non-empty `author` (reusing
`resolveAuthor`, T4's existing helper) — this part is not open.

### 2.6 Recurrence is durable-only

Per the design doc: "A recurring rule is a **durable node** that projects
occurrences." T5's ephemeral todos have no stable address/ID and cannot
host a hand-editable, `SetField`-advanceable cursor. **`repeat:`/recurrence
is only meaningful on a durable todo.** An ephemeral item with `repeat:` set
(if that were even possible, given ephemeral items have no frontmatter at
all — they're plain checkbox lines) is not a real scenario; if a durable
todo's `--ephemeral` sibling flag and `--repeat` were both passed at
`add`-time, that should be a usage error, not silently accepted (see EC-1).

### 2.7 Verb surface — `rk todo done` is extended, not replaced

Nothing in the ticket names a new verb. The natural reading is that T5's
existing `rk todo done <ref>` command gains a conditional branch: if the
resolved todo's frontmatter has a `repeat:` prop, take the recurrence path
(§2.1-§2.5); otherwise, behave exactly as T5 specified (AC-6). No new CLI
command is implied by the ticket text.

---

## 3. Edge cases to handle

| # | Edge case | Expected behavior (grounded / recommended) |
|---|---|---|
| EC-1 | `repeat:` set on an **ephemeral** todo (or `--ephemeral --repeat ...` passed together at `add`-time) | Reject as a usage error at write time — recurrence is durable-only (§2.6); do not silently accept and then have `done` ignore it. |
| EC-2 | `repeat:` present but `scheduled:` missing entirely | Error, non-zero exit, clear message ("cannot advance a recurrence cursor with no `scheduled:` date set") — never invent a default date or silently no-op. |
| EC-3 | `scheduled:` present but not a valid `YYYY-MM-DD` date (hand-edited garbage, e.g. `"TBD"`, empty string) | Error, non-zero exit, clear message — never crash, never write a corrupted/partial date back. |
| EC-4 | `repeat:` malformed (not one of `+Nd`/`++Nd`/`.+Nd` — e.g. `"weekly"`, `"3d"` with no sigil, `"+0d"`/negative N, a `w`/`m`/`y` unit if out of scope per open question #7) | Error, non-zero exit, clear message identifying the unrecognized repeater syntax — never silently fall back to plain `done` behavior (that would silently drop the recurrence, a worse failure mode than a loud error). |
| EC-5 | No `repeat:` prop at all (the ordinary, non-recurring todo case) | Zero behavior change from T5: `state → done`, no `scheduled:` touch, no log entry (AC-6). This is the single most important non-regression check. |
| EC-6 | **On-time completion** (`today == old`) for each of the three families | `+Nd`/`++Nd`: `new = old + N` (both families agree in this case, `k=1`). `.+Nd`: `new = today + N = old + N` (all three families agree in the exact-on-time case — a useful sanity check that the three algorithms only diverge once timing is off). |
| EC-7 | **Early completion** (`today < old`) | `+Nd`/`++Nd`: `new = old + N` (unaffected by early completion — org's fixed/skip-to-future families only ever look at `old`, not `today`, when no cycle has been missed). `.+Nd`: `new = today + N`, which is **earlier** than `old + N` — completing early pulls the *next* occurrence earlier too. This is the family's defining, distinctly-testable behavior. Whether "early completion" can also trigger pile-up is nonsensical (nothing has piled up if you're early) — pile-up detection must never fire when `today <= old`. |
| EC-8 | **Late completion, single missed interval** (`old < today <= old + N`) | All three families still produce a result `> today` with `k=1`/no skip needed; no pile-up (only one cycle's worth of lateness, not stacked). |
| EC-9 | **Late completion, multiple missed intervals** (`today > old + N`, i.e. two or more full cycles have elapsed since the cursor) | `+Nd`: `new = old + N`, which is `<= today` — **still overdue after advancing**, by design (org's documented "catch-up" quirk: you may need to complete again to walk the cursor forward). `++Nd`: `new = old + k·N` for the smallest `k` landing strictly after `today` — never overdue. `.+Nd`: `new = today + N` — never overdue (anchored to actual completion day). This is the scenario most naturally correlated with pile-up (§2.4) — recommended: materialize one ephemeral instance here regardless of family, since real cycles were genuinely missed in all three cases, even though only `+Nd`'s *cursor itself* remains overdue afterward. |
| EC-10 | Blowing away and rebuilding the index **multiple times in a row**, with no intervening completion | Every rebuild must reproduce byte-identical `scheduled:`/`repeat:` values — reconcile/rebuild is a pure read/derive pass with zero writes to vault text; this must hold not just once but under repeated (idempotent) rebuilds. |
| EC-11 | Index rebuild **in between** two completions (advance once, blow away + rebuild the index, then advance again) | The second advance must read the correct (post-first-advance) `scheduled:` value off disk — proving the cursor genuinely lives in text, not cached/derived state that the rebuild could have silently reset. |
| EC-12 | A recurring todo with hand-added, unmodeled extra frontmatter/body content (Obsidian-authored fields, code fences) | Byte-preservation must hold exactly as it does for T5's plain `done`: only the `scheduled:` field's span (and, if applicable, `state:`'s span — see §2.3, ideally untouched) changes; every other byte is untouched. |
| EC-13 | Completing an **already up-to-date** recurring todo a second time on the very same day (`today == old`, called twice) | First call advances as normal (EC-6). What happens on the immediate second call is genuinely ambiguous — is it treated as a fresh on-time completion of the *new* cursor (advances again), or should it be idempotent/no-op like T5's already-done check? The ticket doesn't address double-completion; flag as open, recommend: since a recurring rule's `state` never becomes `done` (§2.3), there is no natural "already completed" idempotency signal available the way there was for T5's plain todos — treat every `done` call on a recurring rule as a fresh completion event (advances again) unless the planner decides an explicit idempotency guard is needed. |
| EC-14 | `--depends`/other T5 props coexisting with `repeat:` on the same durable todo | Should coexist without interference — `depends-on` edges and the recurrence cursor are orthogonal features on the same node type; no special-casing needed, but worth one test confirming they don't clobber each other's frontmatter spans. |

---

## 4. Test scenarios (Given/When/Then)

### AC-1 / EC-6-9 — repeater family advance math (one scenario per family × timing, minimum)

**TS-1** (`+Nd`, on-time)
Given a durable todo with `scheduled: 2026-07-05`, `repeat: +7d`,
When `rk todo done <ref>` is run on `2026-07-05` (today == scheduled),
Then the file's `scheduled:` becomes `2026-07-12` (`old + 7`) via a
span-local edit, and no other byte in the file changes.

**TS-2** (`+Nd`, late by multiple intervals — the "still overdue" quirk)
Given a durable todo with `scheduled: 2026-06-01`, `repeat: +7d`,
When `rk todo done <ref>` is run on `2026-07-05` (34 days late, ≈5 intervals
elapsed),
Then `scheduled:` becomes `2026-06-08` (`old + 7`, a single hop) — still in
the past relative to `today` — demonstrating the fixed family's documented
non-catch-up behavior, and a pile-up ephemeral instance is materialized
(EC-9).

**TS-3** (`++Nd`, late by multiple intervals — skip-to-future)
Given a durable todo with `scheduled: 2026-06-01`, `repeat: ++7d`,
When `rk todo done <ref>` is run on `2026-07-05`,
Then `scheduled:` becomes the smallest `2026-06-01 + 7k` strictly after
`2026-07-05` (`2026-07-06`), never landing in the past, and a pile-up
ephemeral instance is materialized (EC-9).

**TS-4** (`.+Nd`, from-completion, both early and late)
Given a durable todo with `scheduled: 2026-07-10`, `repeat: .+3d`,
When `rk todo done <ref>` is run on `2026-07-05` (5 days early),
Then `scheduled:` becomes `2026-07-08` (`today + 3`, ignoring the original
`2026-07-10` entirely) — and separately, given the same rule reset to
`scheduled: 2026-07-01` and run on `2026-07-20` (late), `scheduled:` becomes
`2026-07-23` (`today + 3`), never referencing the old date in either case.

**TS-5** (all three families agree on-time — sanity cross-check)
Given three otherwise-identical durable todos differing only in `repeat:`
(`+7d`, `++7d`, `.+7d`), all with `scheduled:` equal to today,
When each is completed today,
Then all three produce `scheduled: today + 7` — confirming the families
only diverge once timing drifts from exact on-time.

### AC-2 / AC-6 — did entry written as audit

**TS-6** (did entry appears on recurring completion)
Given a durable recurring todo (`repeat: +7d`, `scheduled:` today),
When `rk todo done <ref>` is run,
Then a new log entry (via the `rk add`/T4 primitive) appears in the current
day's group file, with non-empty `author`, and its body (and/or a
structured link, per §2.5's chosen mechanism) identifies the completed
rule — verified by `rk query` after an index pass returning a `log-entry`
row traceable back to the rule's ULID/alias.

**TS-7** (no did entry for a non-recurring todo — negative control, AC-6)
Given an ordinary durable todo with no `repeat:` prop,
When `rk todo done <ref>` is run,
Then `state` becomes `done` (T5 behavior, unchanged) and **no** new log
entry is written anywhere — confirming the did-log-write is strictly
additive to the recurrence branch, not a general "every completion logs"
behavior.

### AC-3 — current occurrence surfaced

**TS-8** (rule stays visible in default `rk todo list` after completion)
Given a durable recurring todo, open, `scheduled:` today,
When `rk todo done <ref>` is run, then `rk todo list` (default, no
`--all`) is run,
Then the rule still appears in the output (state did not become a
permanently filtered-out `done`), showing the newly-advanced `scheduled:`
date, not the old one.

### AC-4 — pile-up materializes an ephemeral instance

**TS-9** (pile-up on multi-interval-late completion)
Given a durable recurring todo (`repeat: ++7d`, `scheduled:` 5 weeks in the
past),
When `rk todo done <ref>` is run today,
Then, in addition to the rule's own `scheduled:` advancing to the next
future date, exactly one new ephemeral todo item appears (`rk todo list
--ephemeral`) referencing the rule and noting a missed occurrence — and
running `done` again immediately after (now on-time, no further lateness)
produces no additional ephemeral instance.

**TS-10** (no pile-up on a single-interval-late or on-time/early completion)
Given the on-time (TS-1), early (TS-4 early leg), and single-interval-late
(EC-8) scenarios,
When each is completed,
Then no ephemeral instance is created in any of them — pile-up is strictly
a multiple-missed-interval phenomenon (or, per §2.4's open question, absent
entirely for `.+Nd`).

### AC-5 — index rebuild preserves the cursor

**TS-11** (blow away and rebuild preserves an already-advanced cursor)
Given a recurring todo that has been completed once (`scheduled:` already
advanced from its original value),
When the index database file is deleted entirely and `rk index` (full
rebuild) is run, then `rk query "SELECT value FROM node_props WHERE key='scheduled' AND id=<rule>"`
is executed,
Then the returned value equals the post-advance `scheduled:` date exactly —
proving the cursor was read from text, not reconstructed from any
now-deleted index state.

**TS-12** (repeated rebuilds are idempotent, no drift)
Given the same setup as TS-11,
When the index is rebuilt three times in a row with no intervening
completion,
Then all three rebuilds report the identical `scheduled:` value — rebuilding
never itself mutates vault text.

**TS-13** (rebuild between two completions doesn't lose the intermediate state)
Given a recurring todo,
When it is completed once (advance #1), the index is deleted and rebuilt,
and it is completed a second time (advance #2),
Then advance #2's new `scheduled:` value is computed from advance #1's
result (confirmed via the file's actual bytes), not from any stale or
reset value — the index rebuild in between had zero effect on the
text-truth cursor.

### AC-6 — backward compatibility

**TS-14** (plain todo, no `repeat:`, completely unaffected)
Given a durable todo created via T5's `rk todo add` with no `--repeat`
equivalent ever set,
When `rk todo done <ref>` is run,
Then behavior is byte-for-byte identical to T5's own `TS-3.1` (state → done,
only that span changes) — no recurrence code path is exercised.

### Malformed-input edge cases (EC-2/3/4)

**TS-15** (`repeat:` with no `scheduled:`)
Given a durable todo with `repeat: +7d` but no `scheduled:` key at all,
When `rk todo done <ref>` is run,
Then the command exits non-zero with a clear error; the file is not
modified at all.

**TS-16** (malformed `repeat:` syntax)
Given a durable todo with `repeat: weekly` (not a valid org-repeater token),
When `rk todo done <ref>` is run,
Then the command exits non-zero with a clear "unrecognized repeater syntax"
error; the file is not modified at all.

**TS-17** (malformed `scheduled:` value)
Given a durable todo with `repeat: +7d`, `scheduled: not-a-date`,
When `rk todo done <ref>` is run,
Then the command exits non-zero with a clear error; the file is not
modified at all.

---

## 5. Explicitly out of scope

- **`rk today` / the agenda surface** (`reckon-liml`, v1-T7) — in-list
  scheduling keys, the propose-and-confirm agent planner, the *general*
  `did`→task edge convention for **all** todo completions (not just
  recurring ones). T6 only needs recurring todos to remain queryable/
  listable via T5's existing `rk todo list`; it does not need to build the
  live agenda TUI or the general completion-emits-a-log-entry default for
  ordinary (non-recurring) todos.
- **Reminders and the `on-reminder-due` hook** (design doc's A#3a /
  "hooks" §) — a `remind:` prop, standalone `reminder` nodes, delivery via
  external cron/`notify-send`. Entirely separate design area from
  recurrence; no `remind:` handling is in scope here.
- **`deadline:` repeater advance** (open question #6) — the ticket's own
  "Done when" clause only names `scheduled:`; recommend treating any
  `repeat:` interaction with `deadline:` as untouched/out of scope unless
  the planner decides otherwise.
- **Non-day repeater units** (`w`/`m`/`y`) — the ticket's own examples are
  day-granularity only (`+Nd`/`++Nd`/`.+Nd`); treat broader unit support as
  a deliberate non-goal for this ticket, not a bug.
- **iCal `RRULE` escape hatch** — mentioned by the design doc as an
  "optional" complex-rule fallback; not part of this ticket's "Done when"
  clause at all.
- **Permanently stopping/cancelling a recurring rule** (open question #5) —
  no escape-hatch verb/flag is requested by the ticket; not building one
  should not block sign-off, but is worth flagging as a foreseeable gap for
  a future ticket.
- **Checklists' "always-materialize" recurrence variant** — the design doc
  mentions checklists as the always-materialize end of the same spectrum,
  but no checklist tool exists yet (not part of any ticket in this
  backlog's current scope); irrelevant to T6.
- **Migration of legacy DB-primary recurrence/scheduling data** —
  `reckon-s6oh` (v1-T9). T6 operates only on the new text-primary todo node
  type from T5; it does not touch any old `internal/service`-backed
  recurring-task data.
- **MCP/agent-surface porcelain** — `reckon-cxx1` (v1-T10, fast-follow).
  No MCP wiring for recurrence in this ticket.
- **Structured `did`→task typed edge, if the planner picks option (a) in
  §2.5** — body-text-only audit linking is an accepted, explicitly-flagged
  reduction in scope relative to the design doc's stated future `did`→task
  edge convention; upgrading to a real typed edge (option (b)) can be
  deferred to whenever `rk today` (T7) needs it, if T6 doesn't build it
  first.
