Status: PASS

# Preflight Check Report: reckon-fkgb

Scope: `git diff origin/main -- internal/` (7 files, +1790/-27): `internal/cli/todo_edit.go` (new), `internal/cli/todo.go`, `internal/cli/today.go`, `internal/node/insert.go`, their `_test.go` files, and `internal/node/AGENTS.md` (docs). No TUI files changed. Re-run against current HEAD `e85c7d0`.

## Automated Checks

| Check | Result |
|---|---|
| `gofmt -l internal cmd` | PASS, no output |
| `go vet ./...` | PASS, no output, exit 0 |
| `go test -count=1 ./...` | PASS, 16 packages `ok`, 3 with no test files (`cmd/rk`, `internal/perf`, `internal/time`), no FAIL |
| `go test -tags integration -count=1 ./tests/` | PASS, `ok github.com/MikeBiancalana/reckon/tests` |
| `go test -count=1 -cover ./internal/node ./internal/cli` | PASS, node 94.2%, cli 75.0% |
| `go build -o /tmp/claude-1000/-home-chadd-repos-reckon/07058578-b04f-4517-9ef1-2699b825a095/scratchpad/rk ./cmd/rk` | PASS, exit 0 |

Per-function coverage of changed code (`go tool cover -func`, current line numbers):

| Function | File:Line | Coverage |
|---|---|---|
| `runTodoEditE` | `internal/cli/todo_edit.go:61` | 92.9% |
| `validateTodoEditFlags` | `internal/cli/todo_edit.go:188` | 100.0% |
| `checkTodoDepends` | `internal/cli/todo_edit.go:227` | 85.7% |
| `reconcileAfterEdit` | `internal/cli/todo_edit.go:253` | 88.9% |
| `resolveDurableTodoRef` | `internal/cli/today.go:456` | 92.3% |
| `resolveSchedDateArg` | `internal/cli/today.go:520` | 100.0% |
| `validatePriority` | `internal/cli/today.go:536` | 100.0% |
| `loadDurableTodoAt` | `internal/cli/todo.go:923` | 81.8% |
| `findDurableTodoByRefOrAlias` | `internal/cli/todo.go:944` | 84.6% |
| `InsertField` | `internal/node/insert.go:24` | 94.1% |
| `UpsertField` | `internal/node/insert.go:62` | 96.3% |
| `commitUpsert` | `internal/node/insert.go:102` | 85.7% |
| `lineAfter` | `internal/node/insert.go:116` | 75.0% |
| `checkNoContinuation` | `internal/node/insert.go:129` | 90.0% |
| `RemoveField` | `internal/node/insert.go:154` | 96.0% |

Note: the existing report at this path (before this run) cited stale line numbers and a `commitSplice` function that no longer exists. Its figures were not carried over.

## Manual Checks

### Error Handling

Added bare `return err` sites (non-test, added lines only):

| Location | Returns | Context source | Verdict |
|---|---|---|---|
| `internal/cli/todo_edit.go:84` | `validateTodoEditFlags` error | Callee errors already `todo edit: --<field>: ...` | OK |
| `internal/cli/todo_edit.go:89` | `output.ModeFromFlags` error | Reviewer ruling: peers match | OK, ruled acceptable |
| `internal/cli/todo_edit.go:98` | `resolveDurableTodoRef("todo edit", ...)` error | Callee prefixes with verb | OK |
| `internal/cli/todo_edit.go:230` | `lookupNodeByRefOrAlias` error in `checkTodoDepends` | Caller `todo_edit.go:121-122` wraps `todo edit: --depends:` | OK, ruled acceptable |
| `internal/cli/todo_edit.go:263` | `ix.Reconcile` error in `reconcileAfterEdit` | Caller `todo_edit.go:174-175` wraps as warning | OK |
| `internal/node/insert.go:70` | `checkNoContinuation` error | Message carries `UpsertField:` prefix | OK |
| `internal/node/insert.go:82` | `checkNoContinuation` error | Message carries `UpsertField:` prefix | OK |
| `internal/node/insert.go:92` | `trial.InsertField` error | Callee prefixes `InsertField:` | OK |
| `internal/node/insert.go:167` | `checkNoContinuation` error | Message carries `RemoveField:` prefix | OK |

Prior warning status:
- `todo_edit.go` print error: now wrapped at `internal/cli/todo_edit.go:180` (`todo edit: print result (file already written): %w`). Resolved.
- `ModeFromFlags` (`:89`) and `checkTodoDepends` (`:230`): unchanged, reviewer-ruled acceptable, not re-raised.

Other error-handling checks:
- [x] No ignored errors in added lines (`_, err :=` scan clean).
- [x] Main-path errors wrapped: `internal/cli/todo_edit.go:93, 101, 116-119, 122, 136, 150, 167, 180`.

### Resource Cleanup
- [x] `ix` closed by deferred closure: `internal/cli/todo_edit.go:105-110`.
- [x] `reconcileAfterEdit` opens and defers `Close`: `internal/cli/todo_edit.go:258-259`.
- [x] `defer resetTodoFlags(cmd)`: `internal/cli/todo_edit.go:62`.
- [x] Atomic write removes temp file on failure via defer: `internal/cli/adopt.go:249` (`writeFileAtomic`, reused by `todo_edit.go:166`).
- [x] No `sql.Rows` or transaction added. `checkTodoDepends` uses `QueryRow`/`Scan` (`todo_edit.go:239-241`).

### CLI Patterns (subsystem: cli)
- [x] `--quiet` respected: pretty result suppressed at `internal/cli/todo_edit.go:178`; reconcile warning suppressed at `:174`. Matches peers (`todo.go:384`, `todo.go:701`).
- [x] Errors returned, no `os.Exit` in added code.
- [x] Input validated before any write: `validateTodoEditFlags` (`todo_edit.go:83-85`); "nothing to change" guard (`todo_edit.go:80-82`).
- [x] Stderr output only for the reconcile warning (`todo_edit.go:175`).

### TUI Patterns
- [x] N/A. No `internal/tui` files changed. Async closure rule not applicable.

### Test Coverage
- [x] `todo_edit.go` functions exercised through command-level `TestTodoEdit_*` tests (`internal/cli/todo_edit_test.go`, 50+ tests).
- [x] `insert.go` helpers covered by `TestInsertField_*`, `TestUpsertField_*`, `TestRemoveField_*`, plus `TestUpsertRemoveField_RefuseContinuationShapes` (`internal/node/insert_test.go:476`) which hits `checkNoContinuation`.
- [x] No function is untested; `lineAfter` (75.0%) is the lowest-covered new helper.

### Common Mistakes
- [x] No new `TODO`/`FIXME` in added lines.
- [x] No commented-out code in added lines.
- [x] No `fmt.Print`/`log.Print`/`panic` in added non-test code.
- [x] No hardcoded paths in added lines.
- [x] `gofmt` clean on changed files.
- Info: `internal/cli/todo.go:932` (added line) carries a ticket ref `(reckon-vj55)` inside an error string. Not a comment, so the no-provenance-in-comments rule does not strictly apply. Pre-existing pattern in `todo.go:477, 922, 982`.

## Issues Found

### Critical (Must Fix)
None.

### Warning (Should Fix)
None.

### Info (Nice to Have)
1. `internal/node/insert.go:102` - `commitUpsert` at 85.7%, `lineAfter` at `:116` at 75.0%; both exercised only indirectly.
2. `internal/cli/todo.go:932` - error string carries ticket ref `reckon-vj55`; acceptable, matches adjacent lines.

## Summary

**Status:** PASS

All six automated checks pass. No critical issues. Zero open warnings: the reviewer-ruled bare returns are unchanged, and the print-error warning is resolved at `todo_edit.go:180`. The remaining bare returns (`todo_edit.go:84, 98, 263`; `insert.go:70, 82, 92, 167`) each pass through an already-prefixed or caller-wrapped error.

**Next Steps:**
- Proceed to code review.
- No code changes made. Only `ticket-work/reckon-fkgb/preflight-report.md` was written. Nothing staged, committed, or pushed. No `bd` commands run.
