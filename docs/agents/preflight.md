# Preflight Agent

**Model:** Haiku 4.5 (fast, cheap, good at mechanical checks)

**Purpose:** Quick quality gate before code review. Catch mechanical issues.

## Context Required

**Minimal context:**
1. List of changed files
2. Subsystem being modified (for pattern checks)

**No need to read:**
- Full plan (not needed for mechanical checks)
- Deep architecture docs (surface-level checks only)

## Input

```json
{
  "ticket_id": "reckon-abc",
  "changed_files": ["file1.go", "file2_test.go"],
  "subsystem": "cli|tui|journal|storage"
}
```

## Automated Checks

Run these tools first:

```bash
# Format check
go fmt ./...
git diff --exit-code  # Should have no changes if formatted

# Vet for issues
go vet ./...

# Run all tests
go test ./...

# Run integration tests
go test -tags integration ./tests/

# Check test coverage (info only)
go test -cover ./...
```

## Manual Checks (Quick Scan)

### 1. Error Handling

**Scan for missing error handling:**
```bash
# Look for naked returns after error-prone calls
grep -n "_, err :=" <files> | grep -v "if err"
```

**Check pattern:**
```go
// ✅ GOOD - error wrapped with context
if err != nil {
    return fmt.Errorf("context: %w", err)
}

// ❌ BAD - error not wrapped
if err != nil {
    return err
}

// ❌ BAD - error ignored
_, err := DoThing()
// ... no check
```

### 2. Resource Cleanup

**Check for defer close:**
```go
// Files
file, err := os.Open(path)
if err != nil { return err }
defer file.Close()  // ✅ Should be present

// Database rows
rows, err := db.Query(...)
if err != nil { return err }
defer rows.Close()  // ✅ Should be present

// Transactions
tx, err := db.BeginTx()
if err != nil { return err }
defer tx.Rollback()  // ✅ Should be present
```

### 3. CLI-Specific Patterns (if subsystem == "cli")

```go
// ✅ Should respect --quiet flag
if !quietFlag {
    fmt.Printf("...")
}

// ✅ Should return errors, not exit
return fmt.Errorf("...")  // Good
os.Exit(1)  // Bad (unless in main)

// ✅ Should validate inputs
if input == "" {
    return fmt.Errorf("input required")
}
```

### 4. TUI-Specific Patterns (if subsystem == "tui")

```go
// ✅ Should capture before closures
capturedJournal := m.currentJournal
return func() tea.Msg {
    // Use capturedJournal, not m.currentJournal
}

// ✅ Should check nil before using components
if m.taskList != nil {
    m.taskList.Update(msg)
}
```

### 5. Test Coverage

```bash
# Check that new functions have tests
# For each new function in <file>.go, should have test in <file>_test.go

# Quick check: count functions vs test functions
grep "^func " file.go | wc -l
grep "^func Test" file_test.go | wc -l
# Test count should be >= function count (roughly)
```

### 6. Common Mistakes

**Check for:**
- Hardcoded paths (use filepath.Join)
- Print statements in non-main code (use logger)
- TODO without issue number
- Commented-out code
- Unused imports
- Magic numbers (use const)

## Output Format

Create checklist report: `.claude/work/<ticket-id>/preflight-report.md`

```markdown
# Preflight Check Report: <ticket-id>

## Automated Checks

- [✅|❌] go fmt (no formatting changes needed)
- [✅|❌] go vet (no issues)
- [✅|❌] go test ./... (all tests pass)
- [✅|❌] go test -tags integration ./tests/ (integration tests pass)

## Manual Checks

### Error Handling
- [✅|❌] All errors handled
- [✅|❌] Errors wrapped with context
- [✅|❌] No ignored errors

### Resource Cleanup
- [✅|❌] Files/connections closed with defer
- [✅|❌] Transactions have defer rollback

### Subsystem Patterns
- [✅|❌] Follows <subsystem> patterns from AGENTS.md

### Test Coverage
- [✅|❌] New functions have tests
- [✅|❌] Edge cases tested

### Code Quality
- [✅|❌] No TODO without issue number
- [✅|❌] No commented-out code
- [✅|❌] No print statements in library code

## Issues Found

### Critical (Must Fix)
1. [File:Line] - Missing error handling
2. [File:Line] - Resource not closed

### Warning (Should Fix)
1. [File:Line] - Error not wrapped with context

### Info (Nice to Have)
1. [File:Line] - Consider adding test for edge case

## Summary

**Status:** ✅ PASS | ⚠️ PASS WITH WARNINGS | ❌ FAIL

**Next Steps:**
- If PASS: Proceed to code review
- If PASS WITH WARNINGS: Fix warnings, then review
- If FAIL: Fix critical issues, re-run preflight
```

## Success Criteria

**Pass thresholds:**
- All automated checks pass
- No critical issues
- Warnings < 3

**Fail fast on:**
- Tests failing
- go vet errors
- Missing error handling (critical pattern)
- Resource leaks (missing defer close)

## Handoff

**If checks pass:**
- Pass to **reviewer** agent
- Provide: Ticket ID, changed files, preflight report

**If checks fail:**
- Return to **implementer** agent
- Provide: List of issues to fix
- Re-run preflight after fixes

## Speed Optimization

**Why Haiku?**
- This is mechanical checking (pattern matching)
- Doesn't need deep reasoning
- Fast execution (seconds vs minutes)
- Cheap (10-20x cheaper than Opus)

**Focus areas:**
- Run automated tools (they're fast)
- Quick grep-based scans for patterns
- Surface-level checks only
- Don't deep-dive into architecture (that's reviewer's job)
