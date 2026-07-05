# Code Review: reckon-ar9m (v1-T6 — org-style recurring todos)

**Verdict: APPROVE WITH CHANGES**

Reviewer: code-reviewer (Opus 4.8)
Date: 2026-07-05
Scope: `git diff origin/main` — `internal/cli/recur.go` (new), `internal/cli/todo.go`,
`internal/cli/add.go`, `internal/node/logparser.go`, `internal/node/AGENTS.md`, and the
three new/extended test files.

## Summary

This is high-quality, plan-faithful work. The repeater arithmetic is correct across all
three families and every boundary I re-derived independently (results below); the `did::`
marker round-trips byte-for-byte and projects into the index as `rel='did'` without
colliding with or shadowing `id::`; the `state: open` never-flipped decision integrates
cleanly because no other code path consumes the new `type: todo` nodes; and the tests are
substantive and non-tautological (absolute expected dates constructed independently of the
code under test, byte-exact preservation diffs, real index-query assertions). The findings
below are all **Minor** — no Critical or Major defects. Two of them are concrete
consistency gaps the review brief specifically asked about (add-time vs done-time
validation, and post-write validation ordering); they are worth fixing before merge but
none blocks the acceptance criteria.

### Verification performed

- **Re-derived the repeater math independently** (standalone Go program, not the project's
  tests) for `+Nd`/`++Nd`/`.+Nd` across on-time, early, single-late, multi-late, and both
  exact-boundary cases. Every result matched the implementation and the pinned test
  expectations: TS-1→`07-12`/m0, TS-2→`06-08`/m4, TS-3→`07-06`/m4, TS-4early→`07-08`/m0,
  TS-4late→`07-23`/m6, `++` boundary (`today==old+N`)→`old+2N`/m0, `++` exact-2N→`old+3N`/m2,
  EC-7 early `+`→`old+N`/m0.
- **Confirmed the `did` edge lands in the index as claimed**: `insertNode`
  (reconcile.go:289–295) writes every `n.Links` entry to `_edges` with
  `src_key = <entry-ULID>`, `rel = "did"`, `dst = <rule-ULID>`; the public `edges` view
  (schema.go:83–84) exposes it as `SELECT src, dst ... WHERE rel='did'`. `SchemaVersion`
  stays 2 — no schema change, as planned.
- **Confirmed no marker collision**: `extractEntryID` matches prefix `"id:: "`,
  `extractEntryDid` matches `"did:: "`; anchored `HasPrefix` checks mean `"did:: X"` never
  trips the `id::` branch and vice-versa. Fixed peel order (`id::` then `did::`) matches the
  renderer.
- **Confirmed the never-flip-state decision is safe end-to-end**: grepped the whole tree —
  the only consumer of `type = 'todo'` nodes is `listDurableTodos` (todo.go:485/511–518),
  which the new tests cover. The TUI (`internal/tui/*`) and `internal/journal`/`task.go`
  code operate on the *legacy* DB-primary task model, not these text-primary nodes, so a
  perpetually-open recurring rule cannot double-count or mis-render anywhere else. This is
  explicitly out-of-scope migration territory (AC §5, reckon-s6oh).

---

## Correctness

### Issues

1. **[Minor] A `repeat:` value that derives to a Link instead of a scalar Prop silently
   falls through to plain `state→done`, contradicting EC-4's no-silent-fallback intent.**
   The recurrence branch is gated on `n.Props["repeat"]` (todo.go:668). `Props` is only
   populated for scalar frontmatter whose value is *not* ref-shaped; `deriveView`
   (node.go:331–340) routes any value containing `[[…]]` into `n.Links` instead. So a
   hand-edited `repeat: [[weekly]]` (or any `[[…]]`-bearing repeat value) leaves
   `n.Props["repeat"]` empty, the branch is not taken, and the rule is quietly completed as
   a one-shot `state: done` — the exact "silently drop the recurrence" failure mode EC-4
   calls out as worse than a loud error. This is only reachable by an exotic hand-edit
   (`rk todo add --repeat` can't create it — parseRepeat guards that path), so severity is
   Minor, but the fix is cheap: branch on frontmatter presence (`n.HasField("repeat")`) or
   additionally scan `n.Links` for `Rel == "repeat"`, then run the existing `parseRepeat`
   validation so a present-but-malformed repeater always errors rather than being ignored.

2. **[Minor / observation] The `missed` count is generous at exact interval multiples.**
   `advanceSchedule` (recur.go:141–143) computes `missed = daysBetween(old,today)/days` when
   `> days`. At `today == old + k·N` the quotient is exactly `k`, which counts the occurrence
   due *today* as missed even though it is the one being completed (for `+`/`.+`) or
   deliberately skipped as "handled by this completion" (for `++`). Re-derivation:
   `old=2026-07-01, ++7d, today=2026-07-15` → `missed=2` although only `07-08` was truly
   skipped. This is informational text only (`"missed N occurrence(s)"`), matches the plan's
   explicitly-defined formula, and drives no AC or test failure — flagging for awareness, not
   as a defect. If a tighter count is ever wanted, `(daysBetween-1)/days` would exclude the
   boundary occurrence.

### Positive

- The three families, the pile-up trigger (`daysBetween > days`, strict), the negative-day
  early-completion guard (negative `daysBetween` never yields a positive `missed`), and the
  UTC-only clock (parse + `AddDate`, never `time.Parse`/`Truncate`) are all correct. The
  REVIEW_PATTERNS "UTC-parse vs local-format" trap is avoided: `today` is derived from a UTC
  `todoNow()` and re-parsed via `parseSchedDate` in UTC, so both operands share one clock.
- The `++` exact-boundary resolution (skip to `old+2N` at `today==old+N`) is faithful to
  org-mode's own strictly-future semantics; the plan flagged it for sign-off and it is
  **covered at the unit level** (`TestAdvanceSchedule_Skip/boundary_today_equals_old_plus_N`
  and `TestAdvanceSchedule_MissedCount`). I concur with the resolution.

---

## Error Handling

### Issues

1. **[Minor] `--author` validation runs after the authoritative cursor write, violating the
   plan's own "validate pre-write" guarantee.** `doneRecurringTodo` advances and writes the
   cursor first (todo.go:733–738), then calls `appendDidLogEntry`, whose `embeddedHeaderRe`
   author guard (add.go:202–204) is the *first* place a `"## "`-bearing `--author` is
   rejected. Result: `rk todo done <rule> --author $'x\n## y'` advances the `scheduled`
   cursor, writes the file, and *then* returns an error with the cursor already moved. The
   plan is careful to make EC-2/3/4 validation pure and pre-write ("guarantees EC-2/3/4 file
   not modified"); the author check is the one validation left on the far side of the
   authoritative write. This couples badly with finding-below and with EC-13: because a
   recurring rule has no idempotency signal, a caller that retries on non-zero exit (a common
   script pattern) would **double-advance** the cursor. Fix: hoist author resolution +
   `embeddedHeaderRe` validation to the top of `doneRecurringTodo`, before `SetField`
   (validate `resolveAuthor(todoAuthorFlag)` up front). Cheap and removes the only
   deterministic post-write failure mode.

2. **[Minor] Add-time validation checks the repeat cookie but not its required `scheduled`
   anchor's format, so a rule that always fails at done-time can be created through the
   supported path.** `runTodoAddE` (todo.go:280–287) requires `--scheduled` to be present and
   `parseRepeat`-validates the cookie, but never `parseSchedDate`-validates the date. So
   `rk todo add --scheduled TBD --repeat +7d` succeeds and persists a recurring rule that
   errors on *every* `rk todo done` (EC-3 at done-time). This is the exact
   "passes add-time, breaks done-time" asymmetry the brief asked about. It parallels how the
   plan already pre-empts EC-4 at add-time; do the same for EC-3 by adding
   `parseSchedDate(scheduled)` to the `if repeat != ""` block. (General non-recurring
   `--scheduled` format validation remains out of scope — this is only about the case where a
   valid date is load-bearing for the repeater.)

### Positive

- Cross-file non-atomicity (cursor → did entry → pile-up, three separate atomic writes) is
  handled as well as it reasonably can be without a transaction: the authoritative cursor is
  written first via `writeFileAtomic` (temp-file+rename, never half-written), later failures
  are wrapped and surfaced (todo.go:751/755/769) and the partial `res` is discarded by
  `runTodoDoneE` so the user never sees misleading success output. The residual risk — cursor
  advanced without its audit entry on a crash between writes — is genuinely benign (the cursor
  is the source of truth; the log is audit) and is explicitly documented in both the function
  doc comment and plan "Known Risks". I agree this is acceptable; addressing finding #1 above
  shrinks the *deterministic* slice of that risk window to essentially nothing.
- All errors are wrapped with `%w` and context; no bare `return err`.

---

## Testing

### Issues

1. **[Minor] No integration test exercises appending a `did` entry into a *pre-existing*
   `log/<day>.md` file.** `TestTodoDone_Recurring_DidEntryWritten` only covers the
   create-new-day-file branch of `writeLogEntryBlock`. The append-at-EOF branch is shared with
   `appendLogEntry` (covered by T4's add_test.go), so risk is low, but a recurring `done` on a
   day that already has a real `rk add` entry is a realistic path left un-asserted end-to-end.

2. **[Minor] The done-time `--author` `embeddedHeaderRe` rejection (add.go:202–204) is
   untested**, and consequently so is the post-write partial-failure it can produce
   (Error-Handling finding #1). A test that pins `todoNow`, runs `done --author $'x\n## y'`,
   asserts a non-zero error, *and* asserts the cursor did **not** advance would both cover the
   guard and lock in the recommended pre-write fix.

3. **[Minor] No test covers the exact-2×-interval `missed` count** (Correctness finding #2).
   Not required by any AC, but a single case would document the intended count semantics.

### Positive

- Tests are genuinely assertive, not tautological. `recur_test.go` builds fixture dates with
  a `mustUTCDate` helper *deliberately independent of* `parseSchedDate` (the function under
  test), so a fixture bug cannot mask a parser bug. The advance tests assert absolute
  expected dates and exact `missed` counts. The CLI tests assert byte-exact span-locality via
  `strings.Replace(src, "scheduled: OLD", "scheduled: NEW", 1)` diffs, real
  `SELECT ... FROM edges WHERE rel='did'` results after a real `buildIndex`, and the
  negative controls (no `log/` dir on a non-recurring done; no `inbox.md` when nothing piled
  up; inbox byte-unchanged on the second non-late completion). This is exactly the
  standard the REVIEW_PATTERNS library holds up.
- The ambiguous boundary the plan flagged (`++` at `today==old+N`) is covered.
- The TDD-red header conventions and pinned-contract blocks are consistent with T4/T5.

---

## Architecture

### Issues

1. **[Minor] `extractEntryDid` (logparser.go:164–177) is a near-verbatim copy of
   `extractEntryID` (141–154)** — identical body, only the prefix constant differs. A shared
   `extractMarkerLine(rest []byte, prefix string) (val string, body []byte)` would remove the
   duplication. This is a judgment call (the current form keeps each marker's doc comment
   local and the duplication is ~10 lines); acceptable as-is, but worth a shared helper if a
   third marker ever appears. Low priority.

### Positive

- `recur.go` is a clean, pure, IO-free module with an injected `today` — correctly placed as
  a `cli` sibling rather than a premature new package (YAGNI), and independently table-testable.
- The `todoNow` seam mirrors the existing `mintTodoULID` seam precisely; the `SetField`
  span-local edit and the `RenderLogEntry`/`RenderLogEntryWithDid` writer/parser pairing follow
  established patterns.
- The `writeLogEntryBlock` extraction is a genuine DRY win that keeps `appendLogEntry`'s
  signature (and thus T4's add_test.go) untouched — exactly as the plan specified.
- `did::` marker choice (option b) is well-justified: a body wikilink would be inert for
  log-entry sub-nodes (`buildLogEntry` never runs `extractBody`), so option (b) is the only
  route to a queryable audit edge, at ~15 lines and zero core-parser/fuzz-gate exposure.

---

## Maintainability

### Positive (no issues)

- Comments are exemplary: each non-obvious decision cites the plan section and AC/EC it
  satisfies, `daysBetween`'s integer-division rationale is documented, and the never-flip-state
  invariant is called out at every relevant site. AGENTS.md is updated with the new marker's
  fixed position and edge derivation.
- No dead code, no magic numbers (enum constants), no commented-out blocks.
- Minor note (not an issue): `todoAuthorFlag` is a package-global shared between `add` and
  `done`; this is the pre-existing reset-on-defer pattern and works, but is worth keeping in
  mind if a future command needs a differently-defaulted author.

---

## Performance

### Positive (no issues)

- `advanceSchedule`'s `++` loop iterates once per elapsed interval
  (`for !next.After(today)`). Worst realistic case — a `++1d` rule idle for years — is a few
  thousand trivial `AddDate` calls (microseconds); not worth converting to closed-form
  arithmetic. `daysBetween` is O(1). `listDurableTodos`' additional `props["repeat"]` read is
  free (already-loaded map). No new queries, no N+1 introduced.

---

## Security

### Positive (no issues)

- No injection surface: SQL is parameterized (`?`), the repeat cookie is regex-validated,
  `scheduled` is date-parsed, and there is no shell/`eval`/template execution. The did-entry
  body and the `[[…]]` pile-up text are written to markdown and re-parsed, never executed.
- The pile-up text embeds `n.Aliases[0]` unsanitized into an inbox checkbox line, but aliases
  are single-line-by-construction (parsed one-per-frontmatter-line), so they cannot carry a
  newline to break out of the `- [ ]` line or forge a `## ` entry header. `id` is a ULID.
  Reviewed and clean.

---

## Recommended actions before merge (all Minor)

1. Hoist `--author` resolution + `embeddedHeaderRe` validation to the top of
   `doneRecurringTodo`, before the cursor `SetField`/write (Error-Handling #1).
2. Add `parseSchedDate(scheduled)` to the add-time `if repeat != ""` block so a recurring
   rule can't be created with a `scheduled` value that only fails at done-time
   (Error-Handling #2).
3. Gate the recurrence branch on frontmatter presence rather than `Props` membership so a
   `[[…]]`-shaped `repeat:` value errors loudly instead of silently completing as a one-shot
   (Correctness #1).
4. Optional: add the three small test cases named under Testing #1–#3.
