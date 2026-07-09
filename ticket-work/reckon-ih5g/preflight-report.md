# Preflight Check Report: reckon-ih5g

## Status: PASS WITH WARNINGS

---

## Automated Checks

| Check | Result | Details |
|-------|--------|---------|
| `go fmt ./...` | ✅ PASS | No formatting changes needed |
| `go vet ./...` | ✅ PASS | No issues detected |
| `go test ./...` | ✅ PASS | All tests pass (cached results) |
| `go test -cover ./internal/cli/` | ✅ PASS | 40.6% coverage |
| `go test -cover ./internal/node/` | ✅ PASS | 94.1% coverage |

---

## Manual Checks

### Error Handling
- ✅ All returned errors from config, index, and node parsing are wrapped with context
- ✅ Error wrapping follows fmt.Errorf("context: %w", err) pattern
- ⚠️ **Inconsistency**: Some error returns bypass context wrapping (see findings)
- ✅ No ignored errors (no bare `_, err := ...` without checks)

### Resource Cleanup
- ✅ Index opened with `ix, err := index.Open(cfg)` followed by `defer ix.Close()` (lines 349, 353; 660, 664)
- ✅ Database rows closed with `defer rows.Close()` after Query calls (lines 414, 435)
- ⚠️ **Note**: Line 671-689 uses explicit `rows.Close()` instead of defer pattern
  - Safe (all paths covered) but inconsistent with defer pattern elsewhere
  - Intentional explicit cleanup design, not a defect

### CLI: --quiet Flag Respect
- ✅ PASS (lines 323, 401, 543, 737)
- All output paths check `if !(mode == output.Pretty && quietFlag)` before printing
- Pattern correctly suppresses pretty output only when both conditions met

### No os.Exit in CLI Lib Code
- ✅ PASS: grep found zero os.Exit calls in note_v1.go, slug.go, or referenced helpers
- All errors returned from functions for caller handling

### No Variable Capture Bugs in Closures
- ✅ PASS: `add := func(a string)` closure at line 509-515 correctly captures function params, not loop iterator

### Flag Reset Helper
- ✅ PASS: `resetNoteFlags(cmd)` helper present and properly called with `defer` at lines 212, 336, 455, 648
- Mirrors todo.go's `resetTodoFlags` pattern (documented in line 28 comment)
- Clears all note-related flag variables and pflag Changed state

### Test Coverage
- ✅ PASS: 17 comprehensive test scenarios in note_v1_test.go covering:
  - Create/show/rename/index commands (TS-1 through TS-17)
  - Slug collision detection and self-minting aliases
  - Forward links, backlinks, dangling link resolution
  - Block-list aliases handling (Obsidian compatibility)
  - Invalid input rejection (empty slug, reserved slug, duplicate slug)
- ✅ node_test.go: 94.1% coverage with adversarial corpus from gating spike

---

## Findings

### 1. Inconsistent Error Wrapping at Call Sites (WARNING)
**File:** `internal/cli/note_v1.go`  
**Lines:** 382, 386  
**Severity:** WARNING  
**Issue:** `loadNoteForwardLinks` and `loadNoteBacklinks` errors returned bare, inconsistent with similar calls at lines 374, 378 which wrap errors with context.

**Evidence:**
```go
// Lines 372-379: errors WRAPPED at call site
props, err := loadProps(db, id)
if err != nil {
    return fmt.Errorf("note show: %w", err)  // wrapped
}
aliases, err := loadAliases(db, id)
if err != nil {
    return fmt.Errorf("note show: %w", err)  // wrapped
}

// Lines 380-386: errors NOT wrapped at call site
forwardLinks, err := loadNoteForwardLinks(db, id)
if err != nil {
    return err  // NOT wrapped (inconsistent)
}
backlinks, err := loadNoteBacklinks(db, id)
if err != nil {
    return err  // NOT wrapped (inconsistent)
}
```

**Context:** Helper functions (loadProps, loadAliases, loadNoteForwardLinks, loadNoteBacklinks) already wrap their internal errors, so this is not a missing error. It's inconsistent treatment of similar operations in the same function.

**Recommendation:** For consistency, wrap at call site: `return fmt.Errorf("note show: %w", err)` to add caller context uniformly.

### 2. Bare Error Returns from Output Package (WARNING)
**File:** `internal/cli/note_v1.go`  
**Lines:** 246, 325, 466, 545  
**Severity:** WARNING  
**Issue:** Errors from `output.ModeFromFlags()` and `output.New(...).Print()` are returned without adding command context.

**Evidence:**
```go
// Line 244-246 (runNoteCreateE):
mode, err := output.ModeFromFlags(jsonFlag, ndjsonFlag)
if err != nil {
    return err  // no "note create:" context
}

// Line 323-325 (runNoteCreateE):
if err := output.New(cmd.OutOrStdout(), mode).Print(res); err != nil {
    return err  // no "note create:" context
}
```

**Context:** Similar pattern appears in runNoteShowE (line 340-341), runNoteRenameE (line 464-466), runNoteIndexE (line 650-652). All omit command context wrapper.

**Recommendation:** Wrap for uniform error reporting:
```go
mode, err := output.ModeFromFlags(jsonFlag, ndjsonFlag)
if err != nil {
    return fmt.Errorf("note create: %w", err)
}
```

### 3. Explicit Resource Cleanup Pattern (INFO)
**File:** `internal/cli/note_v1.go`  
**Lines:** 671-689  
**Severity:** INFO (not a defect)  
**Note:** The `runNoteIndexE` function uses explicit `rows.Close()` calls (lines 680, 686, 689) instead of the more conventional `defer rows.Close()` pattern used elsewhere (e.g., lines 414, 435).

**Context:** Pattern is safe and covers all error paths correctly. This appears intentional for explicit control over resource lifecycle within the loop logic.

**No action required** — this is a stylistic variance, not a bug.

---

## Coverage Summary

| Package | Coverage | Quality |
|---------|----------|---------|
| `internal/cli` | 40.6% | Adequate for CLI commands |
| `internal/node` | 94.1% | Excellent (gating spike corpus) |

All test files follow established patterns (resetCLIFlags, mustDecodeJSON helpers from todo_test.go/query_test.go/adopt_test.go).

---

## Next Steps

**Status:** ✅ Ready for code review  
**Blockers:** None (all warnings are fixable style/consistency issues, not correctness bugs)

Recommended before merge:
1. Consider adding command context to output package errors (low priority, consistency improvement)
2. Align error wrapping at call sites for loadNoteForwardLinks/loadNoteBacklinks calls (low priority, consistency)

Both are enhancements, not required fixes.

---

**Checked by:** Preflight Agent (Haiku 4.5)  
**Date:** 2026-07-09  
**Ticket:** reckon-ih5g  
**Branch:** main
