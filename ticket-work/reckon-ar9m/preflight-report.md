# Preflight Check Report: reckon-ar9m

**Ticket:** v1-T6: recurrence for todos  
**Date:** 2026-07-05  
**Status:** PASS

---

## Automated Checks

### Code Formatting
- ✅ `go fmt ./...` — No formatting changes needed (clean)

### Static Analysis
- ✅ `go vet ./...` — No issues found

### Test Suite
- ✅ `go test ./...` — All tests pass
  - `internal/cli`: PASS (cached)
  - `internal/node`: PASS (cached)
  - All other packages: PASS

### Test Coverage
- ✅ `internal/cli`: **36.4%** coverage
  - New/modified code well-tested: recur_test.go (8 test functions), todo_recur_test.go (20 test functions)
- ✅ `internal/node`: **93.6%** coverage (high, as expected for parser-heavy code)

---

## Manual Checks

### Error Handling
- ✅ **All errors wrapped with context**
  - `parseRepeat()` (recur.go:66, 74): Malformed cookie errors wrapped
  - `parseSchedDate()` (recur.go:100): Date parsing errors wrapped with context
  - `doneRecurringTodo()` (todo.go:709-737): Validation errors wrapped, repeater/date parse errors provide context
  - `appendDidLogEntry()` (add.go:202-210): Header validation and write errors wrapped
  - `doneRecurringTodo()` calls to appendDidLogEntry (todo.go:753-756): Error wrapped

### Resource Cleanup
- ✅ **Proper defer usage**
  - `listDurableTodos()` (todo.go:503): `rows.Close()` called explicitly before nested queries (correct pattern for avoiding lock issues)
  - `listEphemeralTodos()` (todo.go:571-593): `QueryRow()` used (self-closing), no defer needed
  - Database connections: `index.Close()` with defer at todo.go:455

### CLI Patterns
- ✅ **Respects --quiet flag**
  - `runTodoAddE()` (todo.go:314): Checks `!(mode == output.Pretty && quietFlag)` before output
  - `runTodoDoneE()` (todo.go:626): Same quiet flag pattern respected

- ✅ **No hardcoded paths**
  - All paths use `filepath.Join()` (todo.go:299, 330, 385, 638, 640, 749, 761, 830)
  - Vault-relative paths normalized with `filepath.ToSlash()` (todo.go:861, 863)

- ✅ **No direct print statements**
  - All output routed through `output.New()` helper

- ✅ **Input validation**
  - repeat validation via `parseRepeat()` before writes (todo.go:284-286)
  - scheduled validation via `parseSchedDate()` before writes (todo.go:718-721)
  - Both validations occur *before* any file mutations (EC-2/3/4 guarantee)

### UTC/Timezone Handling (docs/REVIEW_PATTERNS.md compliance)
- ✅ **Explicit UTC throughout**
  - `todoNow` seam defined as `func() time.Time { return time.Now().UTC() }` (todo.go:33)
  - `addDurableTodo()` uses `time.Now().UTC()` (todo.go:340)
  - `addEphemeralTodo()` uses `time.Now().UTC()` (todo.go:391)
  - `parseSchedDate()` uses `time.ParseInLocation("2006-01-02", s, time.UTC)` (recur.go:98)
  - `doneRecurringTodo()` constructs today via `todoNow()` (UTC) and `parseSchedDate()` (recur.go:723-728)

- ✅ **No local-time anti-patterns**
  - No bare `time.Now()` without `.UTC()` 
  - No `.Local()` calls
  - No `Truncate()` for date arithmetic
  - All date arithmetic via `time.AddDate()` on UTC dates (recur.go:124, 130, 132, 138)
  - `daysBetween()` uses integer division on UTC durations (recur.go:111)

### Test Coverage Quality
- ✅ **Unit tests (recur_test.go)**
  - `TestParseRepeat_ValidFamiliesAndUnits`: Covers +Nd, ++Nd, .+Nd, week-to-day normalization
  - `TestParseRepeat_Rejects`: Comprehensive rejection cases (EC-4: zero, negative, wrong units, malformed)
  - `TestParseSchedDate_Rejects`: Invalid date formats, plus positive control (EC-3)
  - `TestAdvanceSchedule_Fixed`: TS-1/TS-2/EC-7/EC-8 repeater math
  - `TestAdvanceSchedule_Skip`: TS-3 future-skip behavior, boundary conditions
  - `TestAdvanceSchedule_FromCompletion`: TS-4 completion-anchored behavior
  - `TestAdvanceSchedule_AllAgreeOnTime`: TS-5/EC-6 on-time parity across families
  - `TestAdvanceSchedule_MissedCount`: EC-8/EC-9 pile-up boundary (7-day vs 8-day)

- ✅ **Integration tests (todo_recur_test.go)**
  - `TestTodoDone_Recurring_Fixed_Advances`: TS-1 end-to-end fixed-family advance
  - `TestTodoDone_Recurring_Fixed_LateOverdue_PilesUp`: TS-2/EC-9 multi-late + pile-up
  - `TestTodoDone_Recurring_Skip_SkipsToFuture_PilesUp`: TS-3 skip-to-future
  - `TestTodoDone_Recurring_FromCompletion_EarlyAndLate`: TS-4 completion-anchored (early/late legs)
  - `TestTodoDone_Recurring_DidEntryWritten`: TS-6 did:: audit entry + link
  - `TestTodoDone_NonRecurring_NoDidEntry`: TS-7/AC-6 backward compatibility (non-recurring todos unaffected)
  - `TestTodoDone_Recurring_StaysInDefaultList`: TS-8 State stays "open"
  - `TestTodoDone_Recurring_PileUpMaterializesOne`: TS-9 exactly one ephemeral instance per completion
  - `TestTodoDone_Recurring_NoPileUpWhenNotLate`: TS-10 on-time/early/single-late no pile-up
  - `TestTodoDone_Recurring_CursorSurvivesRebuild`: TS-11 text-authority over index
  - `TestTodoDone_Recurring_RepeatedRebuildsIdempotent`: TS-12/EC-10 rebuild idempotency
  - `TestTodoDone_Recurring_RebuildBetweenCompletions`: TS-13/EC-11 cursor computed from on-disk state
  - `TestTodoDone_NonRecurring_Unchanged`: TS-14/EC-5 byte-for-byte plain-done parity
  - `TestTodoDone_Recurring_MissingScheduled_Errors`: TS-15/EC-2 error on missing scheduled
  - `TestTodoDone_Recurring_MalformedRepeat_Errors`: TS-16/EC-4 error on malformed cookie
  - `TestTodoDone_Recurring_MalformedScheduled_Errors`: TS-17/EC-3 error on bad date format
  - `TestTodoDone_Recurring_DoubleCompletionAdvancesAgain`: EC-13 no idempotency for recurring rules
  - `TestTodoDone_Recurring_ByteExtraContentPreserved`: EC-12 span-local edit only touches scheduled:
  - `TestTodoDone_Recurring_CoexistsWithDepends`: EC-14 orthogonality with depends-on edges
  - `TestTodoAdd_Repeat_RejectsEphemeralAndRequiresScheduled`: Add-time validation (EC-1/2/4)

### Code Quality Patterns
- ✅ **No TODO without issue number** — None found
- ✅ **No commented-out code** — None found
- ✅ **No magic numbers** — Constants defined (repeatFixed/repeatSkip/repeatFromCompletion enum at recur.go:40-44)
- ✅ **Byte-preservation invariant** — `doneRecurringTodo()` uses `SetField("scheduled", nextStr)` (todo.go:733) for surgical span-local edit
- ✅ **Documentation** — Package headers explain design, constants documented, function comments justify non-obvious choices (e.g., datesBetween division on line 111)

### Architecture
- ✅ **Module isolation**
  - Pure repeater arithmetic in `recur.go` (no IO, no vault/index dependency)
  - Seams for testing: `todoNow` variable (todo.go:33), `mintTodoULID` variable (todo.go:25)
  - CLI integration via `doneRecurringTodo()` in todo.go
  - Log entry audit via `appendDidLogEntry()` in add.go (reused from T4)

- ✅ **LogParser extensions**
  - `extractEntryDid()` mirrors `extractEntryID()` (logparser.go:164-177)
  - `RenderLogEntryWithDid()` complements `RenderLogEntry()` (logparser.go:192-194)
  - `did:: <rule-ULID>` marker documented in AGENTS.md (node/AGENTS.md:194-217)
  - `Link{Rel: "did", To: <rule-ULID>}` synthesized during parse (logparser.go:132)

- ✅ **Durable-only enforcement**
  - `--repeat` rejected with `--ephemeral` (todo.go:277-278)
  - `--repeat` requires `--scheduled` (todo.go:280-282)
  - Recurring rule's state stays "open" on completion (todo.go:741 sets State: "open", not "done")

- ✅ **Pile-up materialization**
  - Exactly one instance iff `missed > 0` (todo.go:760-772)
  - Text format: `[[<rule-ULID>]] missed <N> occurrence(s) (was due <old>, repeat <cookie>)` (todo.go:766)
  - References rule by ULID or first alias (todo.go:762-765)

---

## Summary

**Status:** ✅ **PASS**

All automated checks pass, all manual pattern checks pass, test coverage is comprehensive with both unit and integration tests covering normal flow, edge cases (EC-1 through EC-14), and backward compatibility. Error handling is consistent throughout with wrapped errors providing context. UTC/timezone handling is explicit and correct. Resource cleanup is proper. Code follows the codebase's established patterns for byte-preservation, seams for testing, and surgical field edits.

No issues found. Ready for code review.
