# Code Review: reckon-liml

Scope reviewed: `internal/cli/today.go` (full rewrite), `internal/cli/todo.go`
(`completeDurableTodoNode` extraction + B4 filter), `internal/cli/format.go`
(orphan removal), `internal/cli/today_test.go` (new), `internal/cli/todo_test.go`
(one appended test), against `origin/main`. Cross-checked against the shipped
`doneDurableTodo`/`doneRecurringTodo`, `internal/node` `SetField`/`InsertField`
semantics, `internal/index` schema/reconcile, and the approved plan +
acceptance-criteria. All signed-off decisions (A/B/C/Q6/B4/B5) were reviewed
as-implemented, not relitigated.

## Functional Review

### Issues

1. **[Major]** Post-write reconcile failure is fatal, violating EC-9 (`internal/cli/today.go:356-358`, `runTodayActE`).
   After `dispatchTodayAct` has already written the mutation to disk (the
   authoritative write), the eager write-through reconcile is:
   ```go
   if _, err := ix.Reconcile(); err != nil {
       return fmt.Errorf("today act: reconcile index after write: %w", err)
   }
   ```
   On error this returns before `res` is printed, so the command exits non-zero
   with no success output — even though the file mutation persisted. EC-9 and
   the plan's own cross-cutting conventions are explicit that the file write is
   authoritative and the index update is a non-fatal, self-healing optimization:
   *"The agenda must not roll back or fail the user-visible action just because
   the optimization step had trouble."* The initial `Reconcile()` at the top of
   the handler would already have caught a persistent index problem, so this
   branch fires mainly on a transient failure.
   Failure scenario: a script runs `rk today act <recurring-ref> x`; the cursor
   advances + a `did` entry is written; the post-write reconcile hits a transient
   fault and the command exits non-zero; the script retries; the retry's initial
   reconcile now succeeds and `dispatchTodayAct` **advances the recurrence cursor
   a second time** (recurring completion is non-idempotent, unlike the plain
   state flip) and writes a second `did` entry. Also, because `rk query` never
   reconciles on its own, a swallowed-but-here-surfaced reconcile failure leaves
   `rk query` stale with no success signal to the user.
   Suggested fix: print `res` first (or after), then attempt the reconcile and on
   failure emit a `!quietFlag`-gated warning to `cmd.ErrOrStderr()` and return
   `nil` (exit 0). Mirror the `doneRecurringTodo` posture already documented in
   `todo.go` — the authoritative write is done; the follow-on step is surfaced
   but never rolls back or fails the action.

2. **[Minor]** External `work-ticket` rows are subjected to the native state filter, deviating from plan B5 (`internal/cli/today.go:254-257`, `buildAgenda`).
   The predicate `if state != "open" && state != "in-progress" { continue }`
   runs for *every* candidate, but plan B5 scopes the `state ∈ {open,in-progress}`
   AND to *native rows only* ("ANDed (for native rows) with state ∈ …"). External
   rows are meant to surface purely on the date predicate + read-only marking.
   Failure scenario: a real (post-stub) feeder emits a work-ticket with a foreign
   state vocabulary (`state: in-review`, `To Do`, or no `state` at all) scheduled
   today — it is silently dropped from the agenda despite matching the date
   predicate, contradicting AC-5's "external rows surface via the same date
   predicate." Latent in v1 because every stub fixture uses `state: open`, but it
   will bite when `rk-jira`/`rk-gh` land. Suggested fix: gate the state filter on
   `c.typ == "todo"` (external rows bypass it), or normalize/relax external states.

3. **[Minor]** `--no-log` is silently ignored for recurring completions (`internal/cli/today.go:84` flag help; `todo.go:686` recurrence branch).
   `act x --no-log` routes through `completeDurableTodoNode` → `doneRecurringTodo`
   for a `repeat:` row, which always writes a `did` entry regardless of `logDid`.
   This is plan-sanctioned (Q6: "the recurrence branch already logs; leave it"),
   so it is not a defect, but the flag's help text — *"Suppress the did-entry log
   write when completing (x/done)"* — promises unconditional suppression. A user
   who runs `act x --no-log` on a recurring todo still gets a log entry with no
   feedback. Suggested fix: document the caveat in the flag help (e.g. "…except
   recurring completions, which always log an audit entry"). No test currently
   exercises `--no-log` on a recurring row, so this asymmetry is untested.

4. **[Minor]** `todayActResult` drops the idempotent-skip signal (`internal/cli/today.go:557-567`, `actDone`).
   `completeDurableTodoNode` returns `Skipped: true` when a todo is already
   `done`, but `actDone` copies only a subset of `todoDoneResult` into
   `todayActResult`, which has no `Skipped` field. So `rk today act <ref> x` on an
   already-completed todo reports `State: done` indistinguishably from a real
   completion (and writes no `did` entry, correctly). Minor observability gap for
   scripted callers; consider surfacing a `skipped` field for parity with
   `rk todo done`.

### Notes (informational, not blocking)

- **Split-actuator safety is genuinely robust (positive).** Even setting aside
  the index classify guard (`runTodayActE:341-347`), the *write* path
  (`loadNativeTodoForEdit:427-448`) independently refuses any node whose
  `Type != "todo"` and only ever walks `todos/`. A `work-ticket` can therefore
  never be written on a stale index, an unresolvable ref, or an alias collision —
  the worst case is a "no todo found" error or a spurious read-only rejection, not
  an external write. Two independent guards; this is exactly the discipline the
  ticket demanded.
- **Alias-collision determinism** (`lookupNodeByRefOrAlias:672`): the alias join
  has no `ORDER BY`/`LIMIT`, but `_aliases.alias` de-dups on reconcile (INSERT OR
  REPLACE) and collisions raise an `alias_collision` warning, so within a given
  index state resolution is deterministic. Combined with the write-path type
  filter above, a native/external alias collision is safe (at worst a native
  actuation is wrongly rejected as read-only). No change required.
- **Path traversal**: `ref` flows unsanitized into
  `filepath.Join(todosDir, ref+".md")` (`loadNativeTodoForEdit:430`), so a `../`
  ref could resolve/write outside `todos/` — but only to a file that is already a
  valid `type: todo` node, and this is inherited verbatim from the shipped
  `doneDurableTodo`. For a local single-user tool this is acceptable and
  pre-existing; noting only for completeness.

## Design Review

The rewrite is a clean, idiomatic port that composes the existing substrate
rather than inventing storage machinery, exactly as the plan promised. Command
shape (`today` / `today act` / `today open`), the `requiresDB=false`
annotations, the `resetTodayFlags` mirror of `resetTodoFlags`, the
`ModeFromFlags` + `!(mode==Pretty && quietFlag)` output gating, and the
`fmt.Errorf(…%w…)` wrapping are all consistent with `todo.go`/`query.go`
siblings. The result types carry a pinned-JSON-contract header comment matched
by the test file — good forward-compatibility hygiene.

The `completeDurableTodoNode` extraction is the highest-risk change and it is
done correctly. With `logDid=false` the shared body returns the identical
`todoDoneResult{Kind:"durable", …, State:"done", Skipped:false}` immediately
after the atomic write — byte-for-byte the pre-refactor `doneDurableTodo`
behavior (verified against `origin/main`). The recurrence branch, the
idempotent-skip branch, and the error paths (`set state`/`write`) are untouched
and still delegate to `doneRecurringTodo`. `rk todo done` remains a
zero-behavior-change caller. The B4 one-liner (`state != "in-progress"` added to
the default hide-filter) is minimal and correctly scoped.

Span-local write discipline is honored throughout: every actuator uses
`setOrInsertField` (the `HasField` trichotomy) + `writeFileAtomic(path,
n.Serialize())`, never `Render()`+overwrite. `SetField`/`InsertField` re-parse
from spliced bytes, and the node handed to each actuator is re-read fresh from
disk at actuation time (`loadNativeTodoForEdit`), so EC-8's "re-read before
write" is satisfied and the index snapshot is used only for classification.
Date handling is uniformly UTC: `todoNow()` is `time.Now().UTC()`,
`todayStr := todoNow().Format(...)` formats that UTC instant, and every compare
goes through `parseSchedDate` (`ParseInLocation(..., time.UTC)`). This sidesteps
the mixed UTC-parse/local-format bug class documented in `REVIEW_PATTERNS.md`
(reckon-gcuu) — the format is on a UTC value, not local. Dedupe (EC-2/3) is
structural: one candidate row per node, a single `matched` bool OR'd across the
three date predicates, appended once. Malformed dates (EC-7) skip the offending
field with a per-row warning and still surface the row on a sibling match.
Empty agenda returns a non-nil `[]agendaItem{}` → `{"items":[]}` and a
"nothing due" pretty line at exit 0. The orphaned `formatJournalTSV` removal is
clean and the `journal` package is correctly left untouched (decision A).

Minor architectural observations (no action needed): `buildAgenda` loads props
per-candidate (`loadTodoProps` in a loop) and loads props for *every* todo+
work-ticket before filtering by state — an N+1 pattern, but it is the exact
`listDurableTodos` precedent and fine at vault scale on local SQLite. The
candidate query has no `ORDER BY`, so agenda output order is
SQLite-storage-order; no AC requires ordering and tests don't depend on it, but
a stable `ORDER BY` would make output more predictable. `agendaItem.Path`
(index `loc`) and `todayActResult.Path` (`relTodoPath`) derive the same
vault-relative path from different sources — harmless.

## Test Assessment

Coverage is strong and genuinely black-box: every scenario drives
`RootCmd.SetArgs`+`Execute` and asserts on stdout/stderr/exit code, raw file
bytes, and `rk query`-observed index SQL. AC-1..AC-6 are each covered, including
the hard ones — byte-identity of span-local writes against fixtures carrying
extra frontmatter, a blockquote, and a fenced code block (TS-2.1/3.1);
whole-vault snapshot equality after every one of the seven keys is rejected on
an external row (TS-5.1); write-through visible to `rk query` with no manual
`rk index` (TS-2.5); recurrence reuse asserting the exact `+7d` advance +
`did` edge (TS-4.2); and a false-green guard against the legacy journal RunE.
The `--no-log` suppression test asserts the `log/` dir is never created — a
clean negative. The B4 test in `todo_test.go` is correctly targeted.

Gaps worth noting (none blocking):
- No test drives `act` by **alias** (all `act` tests use the ULID `id`), so the
  `findDurableTodoByRefOrAlias` path inside `loadNativeTodoForEdit` is only
  covered transitively via `rk todo done` tests.
- No test for `act x --no-log` on a **recurring** row — which would surface the
  Issue-3 asymmetry (recurrence always logs).
- No test for the EC-9 post-write-reconcile-failure path (Issue 1) — hard
  without fault injection, but the untested branch is precisely the one that
  deviates from the contract.
- No coverage of `normalizeActKey` rejecting an unknown key, `rk today open` on
  a native row erroring, or `--ndjson` mode on the agenda. All are minor.

## Verdict

**Verdict: REQUEST CHANGES**

The implementation is high quality, faithful to the approved plan, and its
split-actuator guard is doubly safe. One Major issue blocks: the post-write
reconcile is fatal (`today.go:356-358`), directly violating the explicit EC-9
"file write authoritative, index update non-fatal" contract and creating a
real (if narrow) double-advance-on-retry hazard for recurring completions. It
is a ~3-line fix (warn-and-continue instead of return). Address that, and
ideally the plan-B5 external state-filter deviation (Issue 2); the remaining
Minors are optional polish.

## Re-review (post-b32347e)

Fix commit `b32347e` touches `internal/cli/today.go`, `internal/cli/todo.go`,
and `internal/cli/today_test.go` (+4 pin tests). `go build ./...` clean;
`go test ./internal/cli/` green (all prior + 4 new named tests pass). All four
original findings are genuinely fixed — not papered over — and no behavior
drift was introduced for `rk todo done`.

### Per-issue verification

1. **[Major] EC-9 reconcile-fatal — FIXED.** `runTodayActE`
   (`today.go:357-380`) now sequences: `dispatchTodayAct` (authoritative
   write) → print `res` → eager `ix.Reconcile()`. A reconcile failure is
   downgraded to a `!quietFlag`-gated `cmd.ErrOrStderr()` warning and the
   handler returns `nil` (exit 0). This is exactly the suggested posture:
   `res` is emitted before the optimization step, the already-successful file
   write is never rolled back or failed, and the double-advance-on-retry
   hazard for recurring completions is closed. Warning routes to stderr so it
   cannot corrupt `--json` stdout.

2. **[Minor] External state filter — FIXED.** `buildAgenda`
   (`today.go:263`) now gates the `state ∈ {open,in-progress}` predicate on
   `c.typ == "todo"`; the candidate query (`today.go:231`) still selects both
   `todo` and `work-ticket`, so an external row with a foreign/terminal state
   surfaces purely on the date predicate + `ReadOnly:true`, per plan B5.
   Pinned by `TestToday_ExternalRowWithForeignStateSurfaces` (work-ticket with
   `state: done` scheduled today must appear).

3. **[Minor] `--no-log` on recurring — FIXED, no `rk todo done` drift.**
   `logDid` is threaded into `doneRecurringTodo` (`todo.go:769`), gating only
   the did-entry block (`todo.go:818-832`) while the cursor advance / pile-up
   materialization always run. `completeDurableTodoNode` gained a separate
   `recurLogDid` param (`todo.go:689`) forwarded to `doneRecurringTodo`
   (`todo.go:698`). Both call sites updated: `doneDurableTodo` passes
   `recurLogDid=true` (`todo.go:663`) so `rk todo done` still logs
   unconditionally on recurrence (Q6, byte-for-byte unchanged); `actDone`
   passes `logDid, logDid` (`today.go:584`) so `rk today act x --no-log`
   suppresses uniformly. Grep confirms these are the only callers. Pinned by
   `TestTodayAct_DoneNoLogSuppressesEntryOnRecurring` (no `log/` dir; cursor
   advances; state stays open) and `TestTodayAct_DoneOnRecurringStillLogsWithoutNoLog`
   (log day file created without the flag).

4. **[Minor] Skipped signal — FIXED.** `todayActResult` gains
   `Skipped bool json:"skipped"` (`today.go:146`), `Pretty()` handles it first
   (`today.go:154`), and `actDone` propagates `res.Skipped` (`today.go:584`).
   The JSON tag carries no `omitempty`, matching `todoDoneResult.Skipped`
   (`todo.go:215`) exactly — true struct/JSON/Pretty parity. Pinned by
   `TestTodayAct_DoneSkippedSignalInJSON` (asserts JSON `skipped=true` +
   `state=done`, and Pretty text contains "skip").

### New findings

1. **[Informational, non-blocking]** The Issue-1 fix reorders print-before-reconcile,
   so if `output.Print(res)` itself errors (realistically only a closed/broken
   stdout, e.g. SIGPIPE) the handler returns that error and the eager
   reconcile is skipped, whereas pre-fix the reconcile ran first. This is
   benign: the file write is authoritative and the index self-heals at the
   next mutation's front-door `Reconcile()`, and a dead consumer moots the
   eager-reconcile benefit anyway. No change required; noting for completeness.

The 4 new pin tests are substantive (assert real file bytes, `log/` absence,
decoded JSON fields, and Pretty text) rather than vacuous, and each targets the
exact behavior its issue concerned. No JSON-contract-shape test enumerates
`todayActResult`'s field set, so the added `skipped` field breaks nothing.

**Verdict: APPROVE**

All four findings from the first pass are resolved with correct, plan-faithful
fixes and matching pin tests; both signature-change call sites are updated,
`rk todo done` recurrence behavior is preserved, and the reconcile reorder
introduces no material regression. The lone new observation is informational.
