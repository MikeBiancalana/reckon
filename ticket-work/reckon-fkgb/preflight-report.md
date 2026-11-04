Status: PASS

# Preflight Check Report: reckon-fkgb

Scope: `git diff origin/main -- internal/` at HEAD `5a55c37` (7 files: `internal/cli/todo_edit.go` (A), `internal/cli/todo_edit_test.go` (A), `internal/cli/todo.go` (M), `internal/cli/today.go` (M), `internal/node/insert.go` (M), `internal/node/insert_test.go` (M), `internal/node/AGENTS.md` (M)). No `internal/tui` files changed. This run supersedes the earlier report at HEAD `e85c7d0`.

## Automated Checks

| Check | Result |
|---|---|
| `gofmt -l internal cmd` | PASS, no output |
| `go vet ./...` | PASS, no output, exit 0 |
| `go test -count=1 ./...` | PASS, 16 packages `ok`, 3 with no test files (`cmd/rk`, `internal/perf`, `internal/time`), exit 0, no FAIL |
| `go test -tags integration -count=1 ./tests/` | PASS, `ok github.com/MikeBiancalana/reckon/tests`, exit 0 |
| `go test -cover ./internal/node ./internal/cli` | PASS, node 94.2%, cli 75.0%, exit 0 |
| `go build -o <scratchpad>/rk ./cmd/rk` | PASS, exit 0 |

Per-function coverage of changed code (`go test -coverprofile` + `go tool cover -func`):

| Function | File:Line | Coverage |
|---|---|---|
| `runTodoEditE` | `internal/cli/todo_edit.go:60` | 92.9% |
| `validateTodoEditFlags` | `internal/cli/todo_edit.go:187` | 100.0% |
| `checkTodoDepends` | `internal/cli/todo_edit.go:226` | 85.7% |
| `reconcileAfterEdit` | `internal/cli/todo_edit.go:252` | 88.9% |
| `resolveDurableTodoRef` | `internal/cli/today.go:456` | 92.3% |
| `resolveSchedDateArg` | `internal/cli/today.go:520` | 100.0% |
| `validatePriority` | `internal/cli/today.go:536` | 100.0% |
| `loadDurableTodoAt` | `internal/cli/todo.go:923` | 81.8% |
| `findDurableTodoByRefOrAlias` | `internal/cli/todo.go:944` | 84.6% |
| `InsertField` | `internal/node/insert.go:24` | 94.1% |
| `UpsertField` | `internal/node/insert.go:62` | 96.3% |
| `commitUpsert` | `internal/node/insert.go:102` | 85.7% |
| `lineAfter` | `internal/node/insert.go:116` | 75.0% |
| `checkNoContinuation` | `internal/node/insert.go:132` | 90.9% |
| `looksLikeSiblingKey` | `internal/node/insert.go:153` | 100.0% |
| `RemoveField` | `internal/node/insert.go:164` | 96.0% |

New tests: 64 `func Test*` added under `internal/` (16 in `internal/node/insert_test.go`, 48 in `internal/cli/todo_edit_test.go`).

## Manual Checks

### Error Handling
- [✅] Errors wrapped with context in new code. Examples: `todo_edit.go:92`, `:118`, `:121`, `:135`, `:149`, `:166`, `:179`; `insert.go:46`, `:105`, `:184`.
- [✅] No ignored errors. `insert.go` and `todo_edit.go` have no `_, _ :=` or discarded error returns.
- [✅] Bare `return err` sites in new code. Each one either passes through an already-prefixed error or has context added by its caller, so none lack context:

| Site | Returned error | Context source |
|---|---|---|
| `internal/cli/todo_edit.go:83` | `validateTodoEditFlags` | already prefixed `todo edit:` |
| `internal/cli/todo_edit.go:88` | `output.ModeFromFlags` | already prefixed `output:` (`internal/output/output.go:34`) |
| `internal/cli/todo_edit.go:97` | `resolveDurableTodoRef` | verb-prefixed (`todo edit: ...`) |
| `internal/cli/todo_edit.go:229` | `lookupNodeByRefOrAlias` (via `checkTodoDepends`) | caller wraps at `todo_edit.go:121` `todo edit: --depends: %w` |
| `internal/cli/todo_edit.go:262` | `ix.Reconcile()` (via `reconcileAfterEdit`) | caller wraps at `todo_edit.go:174` `todo edit: warning: reconcile index after write: %v` |

  `internal/node/insert.go` has no bare `return err` sites.

### Resource Cleanup
- [✅] `defer ix.Close()` after the lazy open, with `ix` captured by the closure: `todo_edit.go:104-109`.
- [✅] `defer opened.Close()` for the standalone index in `reconcileAfterEdit`: `todo_edit.go:257-258`.
- [✅] Flag reset via `defer resetTodoFlags(cmd)`: `todo_edit.go:61`.
- [✅] No file handles, rows or transactions are opened in the new code. `checkTodoDepends` uses `QueryRow.Scan`, which closes its own rows. `todo.go:559` (`listDurableTodos`) pre-exists and is not in the diff.
- [✅] Node edits make no file I/O (`insert.go` works on in-memory bytes). The only write is `writeFileAtomic` at `todo_edit.go:165`.

### Subsystem Patterns (cli)
- [✅] `--quiet` respected. Reconcile warning suppressed when quiet: `todo_edit.go:173`. Result print suppressed in Pretty+quiet: `todo_edit.go:177`. JSON/NDJSON still print under `--quiet`, matching existing verbs (`todo.go:384`, `today.go:362`).
- [✅] Returns errors, no `os.Exit` in new code. Verified by grep of added lines.
- [✅] Inputs validated before any write. `validateTodoEditFlags` at `todo_edit.go:82`; `--depends` cycle and target checks run before `writeFileAtomic` (`todo_edit.go:113-122`, `:164-167`).

### Subsystem Patterns (tui)
- [➖] Not applicable. No `internal/tui` files are in the diff, and no Bubble Tea closures were added.

### Test Coverage
- [✅] Every new function has at least one test (coverage above). Lowest: `lineAfter` 75.0%, `commitUpsert` 85.7%.
- [✅] Edge cases covered by name, e.g. `TestTodoEdit_RejectsInvalidDate`, `TestTodoEdit_DependsRejectsDirectCycle`, `TestTodoEdit_RefusesCRLFFile`, `TestTodoEdit_DependsRejectsInvalidUTF8`, `TestTodoEdit_RefDoesNotEscapeTodosDir`, `TestUpsertField_RefusesScalarPlusBlockDuplicate`, `TestRemoveField_RefusesEmptyingFrontmatter`, `TestUpsertField_RefusesValueThatDoesNotReadBack`.

### Code Quality
- [✅] No TODO/FIXME in added non-test code.
- [✅] No commented-out code in added lines.
- [✅] No `fmt.Print*` or `log` calls added to library code.
- [✅] No hard-coded paths. Paths use `filepath.Join`.
- [✅] No provenance refs (plan/review/ticket IDs) in comments added by this branch. `insert.go:14` (`plan.md D2`) and the other `plan.md` refs in `todo.go` (lines 262, 296, 393, 445, 824, 830, 1037) pre-exist on `origin/main` and are not in the diff.

## Issues Found

### Critical (Must Fix)
None.

### Warning (Should Fix)
None blocking.

### Info (Nice to Have)
1. `internal/node/insert.go:59-60` - the `UpsertField` doc comment has an inconsistent line wrap (one line runs longer than the surrounding lines). Cosmetic only; `gofmt` does not touch comments.
2. `internal/cli/todo_edit.go:252-265` - `reconcileAfterEdit` returns the `ix.Reconcile()` error unwrapped. Its only caller adds context (`todo_edit.go:174`), so this is acceptable, but a wrap inside the helper would make it self-describing.
3. `internal/cli/todo.go:477`, `:932`, `:982` - the `(reckon-vj55)` reference in error strings is a message, not a code comment. These strings are pre-existing, and `:932` only changed its verb argument. Not a provenance violation.

## Summary

**Status:** PASS

All automated checks pass with exit 0: gofmt clean, vet clean, unit tests (16 packages), integration tests, coverage (node 94.2%, cli 75.0%), and the rk build. Manual pattern checks found no missing error context, no leaked resources, no `--quiet` violations, and no provenance refs or commented-out code in the branch's additions. TUI checks do not apply. Only info-level notes remain.

**Next Steps:**
- PASS: proceed to code review.
