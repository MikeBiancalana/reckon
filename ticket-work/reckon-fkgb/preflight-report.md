# Preflight Check Report: reckon-fkgb

Status: PASS WITH WARNINGS

Scope: `git diff origin/main -- internal/` (7 files, +1601/-24): `internal/cli/todo_edit.go` (new), `internal/cli/todo.go`, `internal/cli/today.go`, `internal/node/insert.go`, and their `_test.go` files, plus `internal/node/AGENTS.md` (docs).

## Automated Checks

| Check | Result |
|---|---|
| `gofmt -l internal cmd` | PASS, no output (exit 0) |
| `go vet ./...` | PASS, no output (exit 0) |
| `go test ./...` | PASS, all 19 packages `ok`; re-run with `-count=1` shows no `FAIL`/`panic` |
| `go test -tags integration ./tests/` | PASS, `ok github.com/MikeBiancalana/reckon/tests` |
| `go test -cover ./internal/node ./internal/cli` | node 94.3%, cli 74.9% |
| `go build -o <scratchpad>/rk ./cmd/rk` | PASS, exit 0 |

Per-function coverage of changed code (from `-coverprofile`):

| Function | Coverage |
|---|---|
| `runTodoEditE` (`todo_edit.go:60`) | 89.7% |
| `validateTodoEditFlags` (`todo_edit.go:183`) | 100% |
| `checkTodoDepends` (`todo_edit.go:222`) | 85.7% |
| `reconcileAfterEdit` (`todo_edit.go:248`) | 88.9% |
| `UpsertField` (`node/insert.go:59`) | 100% |
| `RemoveField` (`node/insert.go:101`) | 91.7% |
| `commitSplice` (`node/insert.go:85`) | 80.0% |
| `resolveDurableTodoRef` (`today.go:456`) | 91.7% |
| `loadDurableTodoAt` (`todo.go:923`) | 81.8% |
| `findDurableTodoByRefOrAlias` (`todo.go:944`) | 84.6% |

## Manual Checks

### Error Handling
- [x] Errors wrapped with context on the main paths (e.g. `todo_edit.go:88-94`, `todo_edit.go:139-141`, `insert.go` `RemoveField`/`commitSplice` wrap with method name).
- [ ] WARNING: three bare `return err` sites in changed non-test code:
  - `internal/cli/todo_edit.go:85` - `output.ModeFromFlags` error, unwrapped.
  - `internal/cli/todo_edit.go:169` - `output.Print` error, unwrapped.
  - `internal/cli/todo_edit.go:225` - `checkTodoDepends` returns `lookupNodeByRefOrAlias` error unwrapped; caller at `:140` adds `todo edit: --depends:` prefix, so context is present at the command level.
- [x] No ignored errors found in added lines (`_, err :=` scan clean).

### Resource Cleanup
- [x] Index handle: `ix` closed via `defer` at `todo_edit.go:101-106`; `reconcileAfterEdit` opens and `defer`s `Close` at `todo_edit.go:254`.
- [x] No `sql.Rows` or transactions added. `checkTodoDepends` uses `QueryRow`/`Scan` (`todo_edit.go:229-236`), which releases the connection.

### CLI Patterns (subsystem: cli)
- [x] `--quiet` respected: pretty output gated at `todo_edit.go:167` (same pattern as `todo.go:384`, `todo.go:701`); reconcile warning gated at `todo_edit.go:175`.
- [x] Errors returned, no `os.Exit` in added code.
- [x] Input validated before any write (`validateTodoEditFlags`, `todo_edit.go:183`); "nothing to change" guard at `todo_edit.go:70-72`.

### TUI Patterns
- [x] N/A. No TUI files changed.

### Test Coverage
- [x] Every new non-test function has direct tests or is exercised via `TestTodoEdit_*` / `TestUpsertField_*` / `TestRemoveField_*` (ran through `runTodoEditE`).

### Common Mistakes
- [x] No new `TODO`/`FIXME` lines.
- [x] No commented-out code in added lines.
- [x] No `fmt.Print`/`log.Print`/`panic` in added non-test code.
- [x] Hardcoded paths: none added.

## Issues Found

### Critical (Must Fix)
None.

### Warning (Should Fix)
1. `internal/cli/todo_edit.go:85`, `:169`, `:225` - bare `return err` (see Error Handling above). Non-blocking; wrapping `:85` and `:169` with `todo edit:` would match the file's convention.

### Info (Nice to Have)
1. `internal/cli/todo_edit.go:225` - `checkTodoDepends` could wrap its own lookup error; currently relies on the caller's prefix.

## Summary

**Status:** PASS WITH WARNINGS (1 warning item, 3 sites; threshold is < 3 warnings).

**Next Steps:**
- Proceed to code review. Optionally wrap the three bare returns before review.
- Do not stage or commit this report per orchestrator instruction (report file written only).
