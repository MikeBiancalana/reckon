# Implementation Plan: v1-T6 — Recurrence via Stored `scheduled` Cursor (org-style)

## Summary

Add org-mode-style recurring todos to the existing durable-todo tool. A recurring rule is an ordinary durable todo (`todos/<ULID>.md`, `type: todo`) that carries two plain scalar frontmatter props: `scheduled: YYYY-MM-DD` (the cursor) and `repeat: <cookie>` (one of `+Nd`, `++Nd`, `.+Nd`; `d`/`w` units). On `rk todo done`, when the resolved todo carries a `repeat:` prop, the handler takes a new recurrence branch that: (1) validates and parses the repeater + current `scheduled` date; (2) computes the next date per the repeater family; (3) advances the cursor via a **span-local `SetField("scheduled", next)`** edit, leaving `state: open` untouched so the rule stays visible in the default list (the completion signal for a recurring rule is the cursor advance, never `state: done`); (4) writes a **`did` audit log entry** into `log/<today>.md` via a new `did:: <rule-ULID>` marker that the LogParser turns into a queryable `Link{Rel:"did"}` edge; and (5) when occurrences piled up (more than one interval elapsed), materializes **exactly one** ephemeral catch-up instance into `todos/inbox.md`. The cursor lives entirely in vault text, so `index.Rebuild()` (which drops and re-derives every table from text alone) reproduces it byte-for-byte. Non-recurring todos are completely unaffected: no `repeat:` prop means the exact T5 `state → done` path runs, no log entry, no math.

The repeater arithmetic is a new, pure, greenfield module (`internal/cli/recur.go`) taking an injectable `today` so it is deterministically unit-testable; the CLI path gets a `todoNow` test seam (mirroring the existing `mintTodoULID` seam) so integration tests can pin "today" and assert the acceptance-criteria's absolute expected dates.

---

## Files to Modify / Create

### Create: `internal/cli/recur.go` (new, pure repeater math — no IO)
Rationale: the codebase has no service layer; per-tool logic lives directly in `internal/cli/<tool>.go` (T3/T4/T5 precedent). The repeater math is pure and independently table-testable, so it belongs in the `cli` package as a sibling file, not a new `internal/recur` package (YAGNI — no external consumer today; T7 `rk today` can extract it later if it needs it). Contents:

```go
type repeatKind int
const ( repeatFixed repeatKind = iota; repeatSkip; repeatFromCompletion ) // +Nd, ++Nd, .+Nd

type repeatSpec struct {
    kind repeatKind
    days int    // normalized to days: d→N, w→7N
    raw  string // original cookie, for error messages
}

// Order matters: ++ and .+ must precede bare +. Dot is escaped (literal).
var repeatCookieRe = regexp.MustCompile(`^(\+\+|\.\+|\+)([1-9][0-9]*)([dw])$`)

func parseRepeat(s string) (repeatSpec, error)            // EC-4: rejects "weekly","3d","+0d","+7m", ""
func parseSchedDate(s string) (time.Time, error)          // time.ParseInLocation("2006-01-02", s, time.UTC); EC-3
func daysBetween(from, to time.Time) int                  // whole UTC days, to−from (may be negative)
func advanceSchedule(old, today time.Time, spec repeatSpec) (next time.Time, missed int)
```

`[1-9][0-9]*` in the regex rejects `+0d` and leading-zero forms; the unit class `[dw]` rejects `m`/`y` (out of scope) as a malformed-repeater error, never a silent fallback.

### Modify: `internal/cli/todo.go`
- **`doneDurableTodo` (lines 586–628)** — the primary seam. After node resolution (unchanged) and before the plain-done path, branch on the `repeat` prop:
  ```go
  if repeat, ok := n.Props["repeat"]; ok && strings.TrimSpace(repeat) != "" {
      return doneRecurringTodo(vaultDir, n, foundPath, ref, repeat)
  }
  // ... existing idempotent-skip check + SetField("state","done") + write (untouched)
  ```
  The existing `state == "done"` idempotent skip stays on the non-recurring path only; a recurring rule's `state` is always `open`, so it never trips.
- **New `doneRecurringTodo(vaultDir string, n *node.Node, foundPath, ref, repeatCookie string) (todoDoneResult, error)`** — the recurrence handler (validate → compute → advance cursor → write did entry → maybe materialize). Detailed under Data Flow.
- **New test seam** `var todoNow = func() time.Time { return time.Now().UTC() }` (mirrors `mintTodoULID`, line 25) so integration tests pin "today".
- **`--repeat` flag on `todoAddCmd`**: add `todoRepeatFlag string` to the flag-var block (lines 32–43), register `af.StringVar(&todoRepeatFlag, "repeat", "", ...)`, add `"repeat"` to `resetTodoFlags`'s reset loop and zero it. Plumb into `addDurableTodo` (new `repeat` param) → `props["repeat"] = repeat`. Validate at add-time: reject `--ephemeral --repeat` (EC-1, extend the existing guard at line 241), reject `--repeat` without `--scheduled` (a repeater with no anchor cannot advance; EC-2 pre-empted at creation), and `parseRepeat(repeat)` to reject malformed cookies early (EC-4 at add-time).
- **`--author` flag on `todoDoneCmd`**: register `df.StringVar(&todoAuthorFlag, "author", "", ...)` (the var and reset-loop entry already exist) so the did-entry author has provenance; `resolveAuthor(todoAuthorFlag)` still defaults correctly if unset.
- **`todoDoneResult` (lines 188–203)** — extend with omitempty recurrence fields (State becomes `"open"` on the recurrence path — honest, since state is never flipped):
  ```go
  Recurred     bool   `json:"recurred,omitempty"`
  Scheduled    string `json:"scheduled,omitempty"`     // newly-advanced date
  Repeat       string `json:"repeat,omitempty"`
  DidEntryID   string `json:"did_entry_id,omitempty"`
  DidEntryPath string `json:"did_entry_path,omitempty"`
  Missed       int    `json:"missed,omitempty"`
  Materialized string `json:"materialized,omitempty"`  // "todos/inbox.md#<line>"
  ```
  Update `Pretty()` and the pinned-contract comment (lines 127–129) accordingly.
- **`todoListItem` (lines 147–160) + `listDurableTodos` (loadTodoProps read)** — add `Repeat string json:"repeat,omitempty"` sourced from `props["repeat"]`, so `rk todo list` surfaces the recurring nature alongside the advanced `Scheduled` (supports AC-3). Additive; update the pin.

### Modify: `internal/node/logparser.go`
- **New `RenderLogEntryWithDid(hhmm, author, ulid, didTarget, body string) string`** — emits the entry with a second marker line right after `id::`:
  ```
  ## HH:MM · author
  id:: <entry-ULID>
  did:: <rule-ULID>
  <body>
  ```
  `RenderLogEntry` (lines 155–157) stays byte-identical (it is pinned by T4's `logparser_test.go` header and used by `rk add`'s non-linked path).
- **New `extractEntryDid(rest []byte) (target string, body []byte)`** — mirrors `extractEntryID` (lines 137–150) for prefix `"did:: "`.
- **`buildLogEntry` (lines 90–131)** — after `ulid, body := extractEntryID(rest)`, add `didTarget, body := extractEntryDid(body)`; if `didTarget != ""`, `n.Links = append(n.Links, Link{Rel: "did", To: didTarget})`. This is the whole audit-edge mechanism; it does **not** touch `parseFrontmatter`/`extractBody`/`deriveView`, so the core `TestRoundTripIdentity`/`FuzzRoundTripIdentity` gates are unaffected (LogParser.Serialize returns Raw verbatim; the `did::` line is inert bytes inside the day file).

### Modify: `internal/cli/add.go`
- **New `appendDidLogEntry(logDir, day, hhmm, author, body, didTarget string) (logAddResult, error)`** — same create-vs-append skeleton as `appendLogEntry` (lines 184–227) but building the block via `node.RenderLogEntryWithDid`. Recommend factoring the shared create-or-append tail into a small private helper both call, to avoid drift; do **not** change `appendLogEntry`'s existing signature (add_test.go may reference it and it is the `rk add` path). Apply the same `embeddedHeaderRe` guard (line 70) to the generated body/author defensively (the body is machine-generated but the author can be user-supplied).

### Modify: `internal/node/node_test.go` and `internal/node/logparser_test.go`
- Add a `logDayWithDid` fixture (an entry with `id::` then `did::`) to the `roundtripCorpus` (mirroring the existing `logDayWithIDs`) and did-marker parse tests (see Test Scenarios).

### Modify: `internal/node/AGENTS.md`
- Extend the "Group files: LogParser and the `id::` marker" section to document the new `did:: <ULID>` marker, its fixed position (immediately after `id::`), and that it derives to `Link{Rel:"did"}` (mirrors T4's own precedent of documenting new parser behavior). `internal/cli/AGENTS.md` is already stale for the redesign tools (per codebase-analysis §6) — low-priority optional note only.

---

## Design Decisions (firm resolutions)

### Gap 1 — Pile-up trigger point: **completion-time, not read-time.** (FIRM)
Pile-up is detected and materialized synchronously inside `doneRecurringTodo`, computed from the same interval arithmetic that advances the cursor (`advanceSchedule` returns `missed`). Trigger condition, **family-independent**: pile-up fires iff `daysBetween(old, today) > spec.days` (strictly more than one interval elapsed since the cursor). `missed = daysBetween(old, today) / spec.days`.

Alternatives rejected:
- **Read-time detection (in `rk todo list` / reconcile).** Rejected because it would make a read path mutate the vault (write an ephemeral instance), directly violating AC-5/EC-10's invariant that reconcile/rebuild is a pure read/derive pass with zero writes to text; it would also require per-rule dedup state to avoid re-materializing on every list, which has nowhere to live except… text (circular) or the disposable index (lost on rebuild). The system is explicitly daemonless/invocation-driven (design doc hooks §), and the design doc says advance/materialize happen "on completion." Completion-time is deterministic, single-write, and idempotent-free.

### Gap 2 — did-entry linking mechanism: **a real `did:: <rule-ULID>` marker → `Link{Rel:"did"}` edge** (option (b), FIRM)
Exact syntax and round-trip specified in its own section below. Chosen over the AC doc's "recommended default (a)" (body-text `[[wikilink]]` only) for a decisive structural reason the AC analysis did not have code access to verify: **log-entry sub-nodes are built by `buildLogEntry`, which never runs `extractBody`.** A `[[wikilink]]` in a log-entry body is therefore inert — it produces *no* edge at all (only FTS text), so option (a) would not yield a queryable audit link even at the "generic references" level the AC doc assumed. Option (b) is the only route to a queryable `did`→task edge, it is the design doc's own stated vocabulary, and its cost is ~15 lines in `logparser.go` following the exact inert-marker pattern `id::` already established (no core-parser change, no fuzz-gate exposure). The `did` edge lands in `_edges` automatically via `insertNode`'s existing `for _, l := range n.Links` loop (reconcile.go:289–295), queryable as `SELECT src, dst FROM edges WHERE rel='did'` with **zero index-schema change**.

Alternatives rejected:
- **(a) body wikilink** — inert for sub-nodes (no edge), as above.
- **(c) plain prose, no reference** — fails "audit / traceability."

### Gap 3 — pile-up instance count: **exactly one catch-up instance per completion event** (FIRM)
Regardless of how many intervals were missed, one ephemeral checkbox line is appended to `todos/inbox.md`, its text naming the missed count and the rule (e.g. `- [ ] [[<rule-ULID-or-alias>]] missed 4 occurrence(s) (was due 2026-06-01, repeat ++7d)`). Chosen because the ticket says "materializes **an** ephemeral instance" (singular); one-per-missed-cycle risks unbounded inbox spam when a rule sits untouched for months (a `++1d` rule idle a year would emit 365 lines); and per-cycle walking is already available to the user via `+Nd`'s catch-up semantics (repeat `done`). The single instance preserves the "occurrences were not silently discarded" guarantee (AC-4) at bounded cost.

Alternatives rejected:
- **One instance per missed cycle** — unbounded, spammy, and provides no information the single counted instance doesn't.

### State-transition question: **a recurring rule's `state` is never flipped; it stays `open`.** (FIRM)
On the recurrence branch, `doneRecurringTodo` does **not** call `SetField("state", …)`. The only frontmatter edit is `SetField("scheduled", next)`. This is load-bearing: T5's `rk todo list` default view filters out `state != "open"` (todo.go:466), so flipping to `done` would make the rule vanish after its first completion, making AC-3 ("surface the current occurrence") unsatisfiable. Org itself resets a repeating entry's keyword back to its not-done state on completion; here the cursor advance *is* the completion record, and the `did` log entry is the durable audit fact. Result JSON reports `State: "open"` on this path (honest).

Alternatives rejected:
- **Flip to `done` then rely on `--all`** — breaks the default actionable list; contradicts AC-3.
- **Introduce a third state (e.g. `recurring`)** — unnecessary new vocabulary; `open` already means "actionable," which a recurring rule always is.

Consequence for double-completion (EC-13): because `state` never becomes terminal, there is no idempotency signal, so each `rk todo done` on a recurring rule is a fresh completion and advances again. This is documented, intended behavior (matches the design's "completing it advances the stored cursor"), not a bug.

### Repeater units: **`d` (days) and `w` (weeks); reject `m`/`y`.** (FIRM)
`w` normalizes to `7·N` days — pure day arithmetic, zero calendar edge cases, and it is the governing design doc's own canonical example (`+1w`, line 1010). `m`/`y` are rejected with a clear malformed-repeater error because calendar-month/year normalization (e.g. Jan 31 + 1 month) is a semantics decision org handles specially, the ticket does not require it, and Go's `AddDate` normalization would silently surprise. This satisfies the governing design doc without incurring the month/year complexity the AC doc rightly flagged.

### Repeater representation: **two separate props (`scheduled:` + `repeat:`), not one org compound timestamp.** (FIRM, per AC §2.2)
`internal/node`'s frontmatter model is one scalar per key with one `fieldSpans` entry; a compound `<2026-07-08 +7d>` would need bespoke parsing the core doesn't have. Both props land in `Props` for free (`+7d` has no `[[`, so `parseRefValues` returns false → Props). `SetField("scheduled", …)` targets exactly the scheduled span; `repeat` is never edited. **No `internal/node` core changes** for storing the repeater.

### Location of math: `internal/cli/recur.go` (FIRM) — rationale under Files, above.

---

## Exact Repeater Arithmetic (per family)

Definitions: `old` = current `scheduled` date (UTC, date-only); `today` = completion date (UTC, date-only, floating — no time component); `days` = normalized interval (`d`→N, `w`→7N). All dates parsed/formatted via `ParseInLocation("2006-01-02", …, time.UTC)` / `.Format("2006-01-02")` so both operands share one clock (the REVIEW_PATTERNS time-bug guard; the injected `today` keeps this pure/testable).

| Family | `next` = | Rule |
|---|---|---|
| `+Nd` (fixed) | `old.AddDate(0,0,days)` | Single unconditional hop off `old`. May be `≤ today` (overdue) — intentional org "catch-up"; complete again to keep walking. Ignores `today`. |
| `++Nd` (skip-to-future) | smallest `old + k·days` with `k≥1` and result **strictly `> today`** | `next = old+days`; `while !next.After(today) { next = next.AddDate(0,0,days) }`. Never overdue. |
| `.+Nd` (from-completion) | `today.AddDate(0,0,days)` | Ignores `old` entirely. Always strictly future (days ≥ 1). |

Timing behavior (verified against all ACs):
- **On-time (`today == old`)** — all three converge on `old+days`: `+`→old+days; `++`→old+days (>today, k=1); `.+`→today+days=old+days. (EC-6 / TS-5.)
- **Early (`today < old`)** — `+`→old+days; `++`→old+days (k=1, since old>today already); `.+`→today+days (**earlier** than old+days, the family's defining behavior). No pile-up (daysBetween ≤ 0). (EC-7 / TS-4 early.)
- **Late, single interval (`old < today < old+days`)** — `+`→old+days (>today); `++`→old+days (k=1, >today); `.+`→today+days. No pile-up (daysBetween < days). (EC-8 interior / TS-1 when late.)
- **Late, multi-interval (`today > old+days`)** — pile-up fires. `+`→old+days (may be `≤ today`, overdue by design); `++`→smallest strictly-future; `.+`→today+days. (EC-9 / TS-2 / TS-3.)

Pile-up (family-independent): `missed = daysBetween(old,today) > days ? daysBetween(old,today)/days : 0`; materialize one instance iff `missed > 0`.

Worked checks: TS-1 (`old=2026-07-05,+7d,today=07-05`)→`07-12`, missed 0. TS-2 (`old=2026-06-01,+7d,today=07-05`, 34d)→`06-08` (overdue), missed `34/7=4`, materialize. TS-3 (`old=2026-06-01,++7d,today=07-05`)→loop to `07-06` (first `old+7k>07-05`), missed 4, materialize. TS-4 early (`old=07-10,.+3d,today=07-05`)→`07-08`, missed 0. TS-4 late (`old=07-01,.+3d,today=07-20`)→`07-23`, missed `19/3=6`.

Documented boundary resolution: at exactly `today == old+days`, `++` yields `old+2·days` (strict `> today`), which honors the AC doc's primary EC-8 invariant "result `> today`" over its parenthetical "`k=1`"; the occurrence due exactly today is treated as handled by this completion. `+` at that boundary lands exactly on `today` (re-due today) — intentional fixed-family behavior.

---

## Exact `did::` Marker Syntax and Round-trip

**Written block** (`RenderLogEntryWithDid`), for a recurring completion:
```
## 09:15 · mike
id:: 01J8ZK...ENTRY
did:: 01J8ZA...RULE
completed recurring todo 01J8ZA...RULE (repeat +7d); advanced scheduled 2026-07-05 → 2026-07-12
```
- Line 1: existing header (`## HH:MM · author`), byte-identical to T4.
- Line 2: existing `id:: <entry-ULID>` (the log-entry's own ULID, minted via `node.Mint()` in `appendDidLogEntry`).
- Line 3: **new** `did:: <rule-ULID>` — the target is the completed rule's ULID (`n.ULID`), so the derived edge resolves directly without an alias lookup.
- Line 4+: body (human/FTS-searchable audit text).

**Round-trip through `buildLogEntry`:**
1. `SplitEntries` yields the block (unchanged; `## ` boundary detection is untouched).
2. `rest` = block minus header. `ulid, body := extractEntryID(rest)` peels line 2 → `ULID = entry-ULID`, `body` now starts with `did:: …`.
3. **New:** `didTarget, body := extractEntryDid(body)` peels line 3 → `didTarget = rule-ULID`, `body` = the audit text; if `didTarget != ""`, append `Link{Rel:"did", To: didTarget}`.
4. `Body = strings.TrimSpace(body)` (unchanged trimming).
5. Fixed order is `id::` then `did::` (matching the renderer). `extractEntryDid` peels whichever of the two top lines is `did::`, so a hand-authored entry with `did::` and no `id::` still parses (entry gets a `did` link but no ULID, hence no edge source — a degenerate case, documented; reckon's own writer always emits `id::` first).

**Byte-preservation / gates:** `LogParser.Serialize` returns `n.Serialize()` = Raw verbatim, and the `did::` line is ordinary bytes inside the day file's Raw, so `parse(serialize(parse(f)))==f` holds by construction. A `logDayWithDid` fixture is added to `roundtripCorpus` to prove the marker survives `TestRoundTripIdentity`/`FuzzRoundTripIdentity` untouched (mirrors `logDayWithIDs`). No `parseFrontmatter`/`deriveView`/`extractBody` change, so the core round-trip corpus is not otherwise touched.

**Index projection:** `insertNode` writes each `n.Links` entry to `_edges` (reconcile.go:289–295). The did edge is `src_key = <entry-ULID>`, `rel = "did"`, `dst = <rule-ULID>`, queryable via the public `edges` view (`SELECT src, dst FROM edges WHERE rel='did'`). No schema change (SchemaVersion stays 2).

---

## Data Flow — `doneRecurringTodo`

1. **Validate (pure, pre-write — guarantees EC-2/3/4 "file not modified"):**
   - `spec, err := parseRepeat(repeatCookie)` → malformed cookie ⇒ error, no write (EC-4).
   - `schedStr, ok := n.Props["scheduled"]` (and `n.HasField("scheduled")`) → missing ⇒ error "cannot advance a recurrence cursor with no scheduled date" (EC-2).
   - `old, err := parseSchedDate(schedStr)` → unparseable ⇒ error (EC-3).
   - `today` = `parseSchedDate(effectiveLogDate-style day)` derived from `todoNow()` (UTC calendar day; test-pinnable).
2. **Compute (pure):** `next, missed := advanceSchedule(old, today, spec)`; `nextStr := next.Format("2006-01-02")`.
3. **Advance cursor (authoritative write):** `n.SetField("scheduled", nextStr)` → `writeFileAtomic(foundPath, n.Serialize())`. Only the `scheduled` value span changes (EC-12 byte-preservation; `state`, `depends-on`, body, extra fields untouched — EC-14).
4. **Write audit (best-effort-but-surfaced):** `author := resolveAuthor(todoAuthorFlag)`; `day, hhmm` from `todoNow()`; `body := fmt.Sprintf("completed recurring todo %s (repeat %s); advanced scheduled %s → %s", n.ULID, repeatCookie, schedStr, nextStr)`; `res := appendDidLogEntry(logDir, day, hhmm, author, body, n.ULID)`.
5. **Materialize pile-up (only if `missed > 0`):** `addEphemeralTodo(todosDir, author, fmt.Sprintf("[[%s]] missed %d occurrence(s) (was due %s, repeat %s)", ruleRef, missed, schedStr, repeatCookie))` where `ruleRef` = first alias if present else `n.ULID`.
6. **Return** `todoDoneResult{Kind:"durable", Ref:ref, Path:relPath, ID:n.ULID, State:"open", Recurred:true, Scheduled:nextStr, Repeat:repeatCookie, DidEntryID:res.ID, DidEntryPath:res.Path, Missed:missed, Materialized:…}`.

**Ordering / partial-failure:** cursor first (source of truth), then audit, then materialize. Writes span multiple files and cannot be atomic; if step 4 or 5 errors it is wrapped and returned (surfaced to the user), but the already-advanced cursor is not rolled back (rollback across atomic file writes isn't available, and the cursor is the authoritative state per the design's "log is audit, not the hot path"). Documented as a known, rare risk.

---

## Edge Cases → Behavior

- **EC-1** `--ephemeral --repeat` at add ⇒ usage error (extend runTodoAddE guard).
- **EC-2** `repeat` present, `scheduled` absent ⇒ done errors non-zero, file untouched.
- **EC-3** `scheduled` not `YYYY-MM-DD` ⇒ done errors non-zero, file untouched.
- **EC-4** malformed `repeat` (`weekly`, `3d`, `+0d`, `+7m`) ⇒ error at add-time and done-time; never a silent fall-through to plain `done`.
- **EC-5/AC-6** no `repeat` ⇒ exact T5 behavior (`state→done`, no log, no math).
- **EC-6** on-time ⇒ all families `old+days`.
- **EC-7** early ⇒ `.+` pulls earlier; no pile-up.
- **EC-8** single-late ⇒ all `> today`; no pile-up.
- **EC-9** multi-late ⇒ `+` may stay overdue, `++`/`.+` future; one instance materialized.
- **EC-10/11** repeated / interleaved rebuilds ⇒ cursor is text-derived, reproduced identically.
- **EC-12** hand-added extra frontmatter/body ⇒ only `scheduled` span changes.
- **EC-13** double-completion same day ⇒ advances again (no idempotency; state never terminal).
- **EC-14** `depends` + `repeat` coexist ⇒ orthogonal; both spans preserved.

---

## Test Scenarios (mapped to concrete files/functions)

Follow the T5/T4 harness conventions: TDD-red header comment, "precedent/harness reuse" list (`resetCLIFlags`, `buildIndex`, `mustWriteFile`, `mustReadFile`, `isValidULID`, `mustDecodeJSON`, plus new `advanceSchedule`/`parseRepeat`/`todoNow`), and a pinned-contract block reproducing the extended `todoDoneResult`/`todoListItem` structs and the `repeatSpec`/`RenderLogEntryWithDid` signatures.

### `internal/cli/recur_test.go` (pure, table-driven — uses AC absolute dates)
- `TestParseRepeat_ValidFamiliesAndUnits` — `+7d`,`++7d`,`.+3d`,`+2w` → correct kind/days.
- `TestParseRepeat_Rejects` (EC-4) — `weekly`,`3d`,`+0d`,`-1d`,`+7m`,`+7y`,``, `++`, `.+d`.
- `TestParseSchedDate_Rejects` (EC-3) — `not-a-date`,`TBD`,``, `2026-13-40`.
- `TestAdvanceSchedule_Fixed` — TS-1 (`07-05`+7d on-time→`07-12`, missed 0), TS-2 (`06-01`+7d @`07-05`→`06-08`, missed 4), EC-7 early, EC-8 single-late.
- `TestAdvanceSchedule_Skip` — TS-3 (`06-01`++7d @`07-05`→`07-06`, missed 4), on-time, boundary `today==old+7`→`old+14`.
- `TestAdvanceSchedule_FromCompletion` — TS-4 early (`07-10`.+3d @`07-05`→`07-08`) and late (`07-01`.+3d @`07-20`→`07-23`).
- `TestAdvanceSchedule_AllAgreeOnTime` — TS-5/EC-6 (`+7d`/`++7d`/`.+7d`, today==old → all `+7`).
- `TestAdvanceSchedule_MissedCount` — EC-8 boundary (`today==old+N`→missed 0) vs EC-9 (`today=old+N+1`→missed 1).

### `internal/cli/todo_recur_test.go` (CLI scenarios — `todoNow` pinned to a fixed date; `t.TempDir()` vault)
- `TestTodoDone_Recurring_Fixed_Advances` — TS-1: file `scheduled` → `07-12`, span-local (only that span changed), `State:"open"`, `Recurred:true`.
- `TestTodoDone_Recurring_Fixed_LateOverdue_PilesUp` — TS-2 + EC-9.
- `TestTodoDone_Recurring_Skip_SkipsToFuture_PilesUp` — TS-3.
- `TestTodoDone_Recurring_FromCompletion_EarlyAndLate` — TS-4.
- `TestTodoDone_Recurring_DidEntryWritten` — TS-6/AC-2: `log/<day>.md` gains an entry with non-empty author; after `buildIndex`, `SELECT src,dst FROM edges WHERE rel='did'` returns the entry→rule pair.
- `TestTodoDone_NonRecurring_NoDidEntry` — TS-7/AC-6: `state→done`, no `log/` file created.
- `TestTodoDone_Recurring_StaysInDefaultList` — TS-8/AC-3: after done, default `rk todo list` still shows the rule with the advanced `scheduled` (and `repeat`).
- `TestTodoDone_Recurring_PileUpMaterializesOne` — TS-9/AC-4: exactly one new `todos/inbox.md` line referencing the rule; a second immediately-on-time `done` adds none.
- `TestTodoDone_Recurring_NoPileUpWhenNotLate` — TS-10: on-time/early/single-late add no ephemeral line.
- `TestTodoDone_Recurring_CursorSurvivesRebuild` — TS-11/AC-5: advance once, delete index db + `ix.Rebuild()`, query `node_props` `scheduled` for the rule == advanced value.
- `TestTodoDone_Recurring_RepeatedRebuildsIdempotent` — TS-12/EC-10: three rebuilds, identical value.
- `TestTodoDone_Recurring_RebuildBetweenCompletions` — TS-13/EC-11: advance, rebuild, advance again computed off disk bytes.
- `TestTodoDone_NonRecurring_Unchanged` — TS-14/EC-5: byte-identical to T5 done.
- `TestTodoDone_Recurring_MissingScheduled_Errors` — TS-15/EC-2: non-zero, file unmodified.
- `TestTodoDone_Recurring_MalformedRepeat_Errors` — TS-16/EC-4.
- `TestTodoDone_Recurring_MalformedScheduled_Errors` — TS-17/EC-3.
- `TestTodoDone_Recurring_DoubleCompletionAdvancesAgain` — EC-13.
- `TestTodoDone_Recurring_ByteExtraContentPreserved` — EC-12.
- `TestTodoDone_Recurring_CoexistsWithDepends` — EC-14.
- `TestTodoAdd_Repeat_RejectsEphemeralAndRequiresScheduled` — EC-1 + add-time EC-2/EC-4.

### `internal/node/logparser_test.go` + `internal/node/node_test.go`
- `TestRenderLogEntryWithDid_Format` — exact 4-line block.
- `TestLogParser_ParsesDidMarkerIntoLink` — `did::` line → `Link{Rel:"did", To:ruleULID}` on the log-entry, and the line is dropped from `Body` (and `id::` still parses to the entry ULID).
- `TestLogParser_DidRoundTripIdentity` — `logDayWithDid` added to `roundtripCorpus`; asserted under `TestRoundTripIdentity`/`FuzzRoundTripIdentity`.

---

## Implementation Steps

1. `internal/cli/recur.go` + `internal/cli/recur_test.go` (pure math first; red → green).
2. `internal/node/logparser.go`: `RenderLogEntryWithDid`, `extractEntryDid`, `buildLogEntry` wiring; `logDayWithDid` fixture + logparser/node tests.
3. `internal/cli/add.go`: `appendDidLogEntry` (+ shared create/append helper).
4. `internal/cli/todo.go`: `todoNow` seam; `--repeat` flag + add-time validation + `addDurableTodo` plumbing; `--author` on done; extended `todoDoneResult`/`todoListItem`; `doneDurableTodo` branch + `doneRecurringTodo`.
5. `internal/cli/todo_recur_test.go`: full CLI scenarios incl. index-rebuild.
6. `internal/node/AGENTS.md`: document the `did::` marker.
7. Run `go test ./...` incl. the fuzz round-trip gate.

---

## Known Risks / Remaining Ambiguities

- **`++` exact-boundary semantics** (`today == old+days`): resolved to strict-future (`old+2·days`), which honors EC-8's "result > today" over its parenthetical "k=1". This is a deliberate, documented divergence from one AC parenthetical at a measure-zero boundary; flag for reviewer sign-off.
- **Cross-file non-atomicity**: cursor advance, did entry, and pile-up are three separate atomic file writes. A crash between steps can leave the cursor advanced without its audit entry. Accepted per the design ("log is audit, never the hot path"); the authoritative cursor is always consistent. No transaction spans them.
- **Pile-up applies to `.+Nd` too** (uniform `today > old+days` trigger). The AC doc floated "flag whether pile-up even applies to `.+Nd`"; resolved to *yes* (EC-9's own recommendation: real cycles were missed regardless of family). If the reviewer prefers `.+Nd` to never pile up, it is a one-line guard.
- **Ephemeral traceability is coarse**: the materialized instance carries a `[[rule]]` wikilink that yields only a *container-level* `references` edge (from `todos/inbox.md`), not a per-item typed edge — ephemeral items have no node identity by T5's design. Documented limitation, not a defect.
- **Author on `done`**: added a `--author` flag; if unset, `resolveAuthor("")` falls back to `$RECKON_AUTHOR`/`$USER`/`"local"` so the did entry always has non-empty provenance (AC-2), but automated/CI provenance may read `"local"`.
- **`w` unit inclusion** slightly exceeds the ticket's literal `Nd` examples; justified by the governing design doc's `+1w` example and zero calendar risk. `m`/`y` remain out of scope and error loudly.

### Critical Files for Implementation
- internal/cli/todo.go
- internal/cli/recur.go (new)
- internal/node/logparser.go
- internal/cli/add.go
- internal/cli/todo_recur_test.go (new)
