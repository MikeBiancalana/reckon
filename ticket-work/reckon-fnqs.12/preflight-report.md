# Preflight Check Report: reckon-fnqs.12

## Summary

**Status:** ⚠️ **PASS WITH WARNINGS**

This ticket adds a checklist runner TUI component and CLI command for interactive checklist runs. Automated checks pass completely; one error-handling inconsistency and missing explicit tests warrant review.

---

## Automated Checks

### Format Check
- ✅ **go fmt ./...**: No formatting issues (exit 0)

### Linter
- ✅ **go vet ./...**: No linting issues (exit 0)

### Test Suite
- ✅ **go test ./...**: All tests pass (cached)
- ✅ **go test -cover ./...**: All packages pass
  - `internal/cli`: 74.8% coverage
  - `internal/tui/components`: 76.6% coverage

### Affected Packages
No test failures. All related packages verify.

---

## Manual Checks

### 1. Error Handling

#### Line 30 (checklist_run.go)
```go
_, svc, db, err := setupChecklistRun()
if err != nil {
    return err
}
```
- **Status**: ✅ CONSISTENT
- **Rationale**: `setupChecklistRun()` wraps errors internally with "checklist:" context. This pattern is used consistently across all checklist commands (create, list, start, check, status, reset, abandon). Returning unwrapped is intentional and correct.

#### Line 36 (checklist_run.go)
```go
run, _, err := resolveChecklistRun(svc, name)
if err != nil {
    return fmt.Errorf("checklist run: %w", err)
}
```
- **Status**: ✅ CORRECT
- **Rationale**: `resolveChecklistRun()` returns raw errors. Wrapping with "checklist run:" context is correct.

#### Line 43 (checklist_run.go)
```go
if _, _, err := components.RunPrompt[[]components.ChecklistItem](runner); err != nil {
    return err
}
```
- **Status**: ⚠️ **WARNING** — Should wrap error
- **Issue**: RunPrompt returns errors from `PromptGuard()` or `tea.Program.Run()` without context wrapping. These are not wrapped by RunPrompt itself, so the caller should wrap them for consistency.
- **Pattern Mismatch**: Line 36 (above) wraps `resolveChecklistRun()` errors; line 43 should similarly wrap RunPrompt errors with "checklist run:" prefix.
- **Recommendation**: Change to:
  ```go
  if _, _, err := components.RunPrompt[[]components.ChecklistItem](runner); err != nil {
      return fmt.Errorf("checklist run: %w", err)
  }
  ```

#### Line 46 (checklist_run.go)
```go
if e := runner.Err(); e != nil {
    return fmt.Errorf("checklist run: %w", e)
}
```
- **Status**: ✅ CORRECT
- **Rationale**: Runner's internal toggle error is properly wrapped with "checklist run:" context.

#### Summary
- ✅ No ignored errors
- ✅ Errors from wrapped helpers returned appropriately
- ⚠️ RunPrompt error (line 43) should be wrapped for consistency with line 36

### 2. Resource Cleanup

#### checklist_run.go
- ✅ **Line 24**: `defer resetChecklistFlags(cmd)` — Flags reset at function start
- ✅ **Line 32**: `defer db.Close()` — Database connection properly closed
  - Defer is correctly placed after successful `setupChecklistRun()`, following standard resource cleanup pattern

#### checklist_runner.go
- ✅ No resource acquisition (TUI component is stateless; lifecycle managed by RunPrompt host)

### 3. TUI-Specific Patterns

#### Variable Capture in Closures (checklist_run.go, lines 64-76)
```go
func makeToggleFunc(svc *checklist.Service, runID string) components.ToggleFunc {
    return func(position int) ([]components.ChecklistItem, bool, error) {
        updated, err := checkAndRefetchRun(svc, runID, position)
        // ...
    }
}
```
- **Status**: ✅ CORRECT
- **Analysis**:
  - `svc` captured by value (parameter)
  - `runID` captured by value (parameter, immutable string)
  - Comment explicitly documents: "closes over svc and the immutable run ID (not the mutable *Run) so there is no staleness across the session's toggles"
  - No reference capture of mutable state ✓

#### checklist_runner.go Update Method
- ✅ No closures created; `onToggle` is a callback field set via `Show()`, not captured in a closure
- ✅ Safe pointer receiver usage in `Update()` method

### 4. CLI-Specific Patterns

#### checklist_run Command
- ✅ **TUI-only command**: Correctly documented in docstring (line 11-14)
- ✅ **Guard enforcement**: "TUI-only: non-TTY and --no-input are refused by the shared components.PromptGuard"
- ℹ️ **--quiet flag**: Not applicable (interactive TUI; suppressing an interactive prompt is not meaningful)
- ✅ **Returns errors, not exits**: All error paths return `fmt.Errorf(...)`, no `os.Exit()`
- ✅ **Input validation**: Template name passed through `resolveChecklistRun()` which validates via `svc.GetTemplate(name)`

### 5. Test Coverage

**Correction (orchestrator):** the automated pass below did not find
`internal/tui/components/checklist_runner_test.go` (271 lines, 12 test
functions) or the new functions added to `internal/cli/checklist_test.go` (7
test functions) — both exist and were written in Phase 3 (TDD red state)
before this implementation. `ChecklistRunner.Update()`/`View()` (cursor
clamping, toggle, quit keys, empty-checklist, completion) and
`runItemsToChecklistItems`/`makeToggleFunc` are directly unit-tested, not
just incidentally covered. See ticket-work/reckon-fnqs.12/state.json's
`new_test_names` for the full list. No test-coverage gap exists; the
finding below is void.

#### Package Coverage
- `internal/tui/components`: 76.6% coverage
- `internal/cli`: 74.8% coverage

**Status**: ✅ **NO GAP** — explicit unit tests exist for both new files (see correction above).

### 6. Code Quality

#### Common Mistakes
- ✅ No hardcoded paths (uses function parameters)
- ✅ No print/log statements in library code (checklist_runner.go uses only `fmt.Sprintf`)
- ✅ No untracked TODOs (no TODO/FIXME comments without issue numbers)
- ✅ No commented-out code
- ✅ No unused imports (verified via `go vet`)
- ✅ No magic numbers (all literals have semantic meaning)

#### Code Style
- ✅ Comments are clear and explain intent (e.g., line 64-67 closure comment, line 18-23 type comments)
- ✅ Function organization is logical (public types/functions first, helpers after)
- ✅ Variable naming is clear and idiomatic

---

## Issues Found

### Critical (Must Fix)
*None*

### Warnings (Should Fix)
1. **File: internal/cli/checklist_run.go, Line 43**
   - **Issue**: RunPrompt error returned unwrapped
   - **Current**: `return err`
   - **Expected**: `return fmt.Errorf("checklist run: %w", err)`
   - **Rationale**: Inconsistent with error wrapping pattern on line 36; other helpers' errors are wrapped for context

### Info (Nice to Have)
*None — see correction in §5 above; the original "no test files" finding here was a false positive (the automated pass missed pre-existing checklist_runner_test.go and checklist_test.go additions).*

---

## Subsystem Patterns Summary

### CLI Checklist Pattern (internal/cli/checklist.go)
✅ All new code follows established patterns:
- Error wrapping with "checklist <verb>:" context (mostly ✓, one inconsistency on line 43)
- Flag reset via defer in RunE functions
- Database resource cleanup with defer
- Output mode handling via setupChecklistRun()
- --quiet flag respect (N/A for TUI-only command)

### TUI Components Pattern (internal/tui/components/)
✅ All new code follows established patterns:
- Prompt[T] interface implementation
- Closure captures by value, not reference
- Stateless component design (state reset via Show())
- Safe pointer receiver methods
- Clear separation of model and view logic

---

## Next Steps

**Recommended before handoff to code review:**
1. Fix line 43 to wrap RunPrompt error (1-line change) — deferred to reviewer's judgment, not auto-fixed here (preflight is mechanical-fix-only).

Proceed to code review; note the error-wrapping inconsistency in review comments.

---

## Checklist Summary

| Check | Result | Notes |
|-------|--------|-------|
| go fmt | ✅ | No changes needed |
| go vet | ✅ | No issues |
| go test ./... | ✅ | All pass |
| go test -cover ./... | ✅ | 74.8% CLI, 76.6% components |
| Error handling | ⚠️ | Line 43 should wrap |
| Resource cleanup | ✅ | defer patterns correct |
| TUI patterns (closures) | ✅ | Variable capture safe |
| CLI patterns | ✅ | Follows conventions |
| Test coverage | ✅ | Explicit unit tests exist (see §5 correction) |
| Code quality | ✅ | No issues found |
| **Overall** | **⚠️ PASS WITH WARNINGS** | Fix line 43 (deferred to reviewer) |
