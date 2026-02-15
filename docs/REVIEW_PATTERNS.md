# Review Patterns Library

**Purpose:** Capture recurring patterns (good and bad) discovered during code review to improve future work.

This is a **learning system** - patterns found during reviews feed back into preflight checks and implementation guides.

## How This Works

1. **Reviewer finds issue** → Documents in review.md
2. **Pattern extraction** → Issue categorized and tracked
3. **Frequency tracking** → Count occurrences (2-3x = add to guides)
4. **Feedback to system:**
   - Common issues → Add to preflight checks
   - Good patterns → Add to subsystem AGENTS.md examples
   - Anti-patterns → Add to subsystem AGENTS.md warnings

## Pattern Categories

### Error Handling

#### ❌ Anti-Pattern: Unwrapped Errors
**Issue:** Returning errors without context makes debugging impossible.

```go
// BAD
if err != nil {
    return err
}
```

**Fix:**
```go
// GOOD
if err != nil {
    return fmt.Errorf("failed to parse journal for %s: %w", date, err)
}
```

**Frequency:** 🔴🔴🔴🔴🔴 (Very common - 15 occurrences)

**Preflight check:** `grep -n "return err$" *.go`

**Added to guides:**
- ✅ internal/cli/AGENTS.md - Error handling section
- ✅ internal/journal/AGENTS.md - Service layer section
- ✅ docs/agents/implementer.md - Critical patterns

---

#### ❌ Anti-Pattern: Ignored Errors
**Issue:** Calling error-returning functions without checking the error.

```go
// BAD
db.Query(...)
// ... continue without checking err
```

**Fix:**
```go
// GOOD
rows, err := db.Query(...)
if err != nil {
    return fmt.Errorf("query failed: %w", err)
}
defer rows.Close()
```

**Frequency:** 🔴🔴🔴 (Common - 8 occurrences)

**Preflight check:** `grep -n "_, err :=" *.go | grep -v "if err"`

**Added to guides:**
- ✅ internal/storage/AGENTS.md - Query patterns section
- ⚠️ TODO: Add to preflight.md automated checks

---

### Resource Management

#### ❌ Anti-Pattern: Missing defer close
**Issue:** Files, database connections, or transactions not closed, causing resource leaks.

```go
// BAD
file, err := os.Open(path)
if err != nil {
    return err
}
// ... use file ...
// Missing file.Close()
```

**Fix:**
```go
// GOOD
file, err := os.Open(path)
if err != nil {
    return fmt.Errorf("failed to open %s: %w", path, err)
}
defer file.Close()
```

**Frequency:** 🔴🔴🔴🔴 (Common - 12 occurrences)

**Preflight check:** Look for `os.Open`, `db.Query`, `db.BeginTx` without corresponding `defer close/Close()`

**Added to guides:**
- ✅ internal/storage/AGENTS.md - Connection patterns
- ✅ internal/journal/AGENTS.md - File operations
- ⚠️ TODO: Add automated check to preflight.md

---

### TUI-Specific Patterns

#### ❌ Anti-Pattern: Closure Capture Bug
**Issue:** Capturing loop variables or model fields directly in async closures leads to race conditions.

```go
// BAD - m.currentJournal can change before closure runs
return func() tea.Msg {
    journal := m.currentJournal  // Race condition!
    return loadTasks(journal)
}
```

**Fix:**
```go
// GOOD - Capture value immediately
capturedJournal := m.currentJournal
return func() tea.Msg {
    return loadTasks(capturedJournal)
}
```

**Frequency:** 🔴🔴🔴🔴🔴🔴 (Very common - 18 occurrences)

**Preflight check:** Manual review of tea.Cmd functions for closure patterns

**Added to guides:**
- ✅ internal/tui/AGENTS.md - Critical async pattern (prominent warning)
- ✅ docs/agents/implementer.md - TUI-specific patterns

---

#### ❌ Anti-Pattern: Nil Component Access
**Issue:** Accessing optional components without nil checks causes panics.

```go
// BAD
m.taskList.Update(msg)  // Panic if taskList is nil!
```

**Fix:**
```go
// GOOD
if m.taskList != nil {
    m.taskList.Update(msg)
}
```

**Frequency:** 🔴🔴🔴 (Common - 7 occurrences)

**Preflight check:** Look for component access patterns without nil checks

**Added to guides:**
- ✅ internal/tui/AGENTS.md - Component lifecycle section
- ⚠️ TODO: Add example of proper component initialization

---

### CLI-Specific Patterns

#### ❌ Anti-Pattern: os.Exit() in Library Code
**Issue:** Using os.Exit() in commands prevents proper error handling and testing.

```go
// BAD
if err != nil {
    fmt.Fprintf(os.Stderr, "Error: %v\n", err)
    os.Exit(1)
}
```

**Fix:**
```go
// GOOD
if err != nil {
    return fmt.Errorf("operation failed: %w", err)
}
```

**Frequency:** 🔴🔴 (Occasional - 4 occurrences)

**Preflight check:** `grep -n "os.Exit" cmd/*.go`

**Added to guides:**
- ✅ internal/cli/AGENTS.md - Error handling section
- ✅ docs/agents/implementer.md - CLI patterns

---

#### ❌ Anti-Pattern: Ignoring --quiet Flag
**Issue:** Printing output even when user requested quiet mode.

```go
// BAD
fmt.Printf("Processing...\n")
```

**Fix:**
```go
// GOOD
if !quietFlag {
    fmt.Printf("Processing...\n")
}
```

**Frequency:** 🔴🔴 (Occasional - 5 occurrences)

**Preflight check:** Look for fmt.Print* in CLI commands without quiet flag check

**Added to guides:**
- ✅ internal/cli/AGENTS.md - Output guidelines
- ⚠️ TODO: Add test pattern for --quiet flag

---

### Testing Anti-Patterns

#### ❌ Anti-Pattern: Tests Don't Test What They Claim
**Issue:** Test name says one thing, test checks something else.

```go
// BAD - Name says "handles empty input" but doesn't test that
func TestParseTask_HandlesEmptyInput(t *testing.T) {
    result := ParseTask("valid input")
    assert.NotNil(t, result)
}
```

**Fix:**
```go
// GOOD - Test matches name
func TestParseTask_HandlesEmptyInput(t *testing.T) {
    _, err := ParseTask("")
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "empty input")
}
```

**Frequency:** 🔴🔴🔴 (Common - 9 occurrences)

**Preflight check:** Manual review during code review

**Added to guides:**
- ✅ docs/TESTING.md - Writing good tests section
- ⚠️ TODO: Add examples to subsystem guides

---

#### ❌ Anti-Pattern: Missing Edge Case Tests
**Issue:** Only testing happy path, missing error conditions.

**Common missing tests:**
- Empty string inputs
- Nil pointers
- Invalid formats
- Boundary conditions (first/last day of year)
- Concurrent access

**Fix:** Use table-driven tests with comprehensive cases:
```go
tests := []struct {
    name    string
    input   string
    want    Result
    wantErr bool
}{
    {name: "happy path", input: "valid", want: Result{}, wantErr: false},
    {name: "empty input", input: "", want: Result{}, wantErr: true},
    {name: "invalid format", input: "bad", want: Result{}, wantErr: true},
    {name: "nil handling", input: "", want: Result{}, wantErr: true},
}
```

**Frequency:** 🔴🔴🔴🔴 (Common - 11 occurrences)

**Added to guides:**
- ✅ docs/TESTING.md - Edge case examples
- ⚠️ TODO: Add checklist to docs/agents/test-writer.md

---

## Good Patterns to Replicate

### ✅ Table-Driven Tests
**Pattern:** Comprehensive test coverage with clear test cases.

```go
func TestParseDate(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    time.Time
        wantErr bool
    }{
        {name: "ISO format", input: "2024-01-15", want: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)},
        {name: "short format", input: "1/15", want: time.Date(time.Now().Year(), 1, 15, 0, 0, 0, 0, time.UTC)},
        {name: "invalid", input: "not-a-date", wantErr: true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := ParseDate(tt.input)
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
                assert.Equal(t, tt.want, got)
            }
        })
    }
}
```

**Benefits:**
- Clear test cases
- Easy to add new cases
- Good coverage

**Frequency:** ✅✅✅✅✅ (Used consistently - good!)

---

### ✅ Service Layer Pattern
**Pattern:** Separate business logic from CLI/TUI code.

```go
// Service handles business logic
type TaskService struct {
    store *storage.Store
}

func (s *TaskService) CreateTask(desc string, due time.Time) (*Task, error) {
    // Validation, business rules, persistence
}

// CLI just calls service
func createTaskCmd(cmd *cobra.Command, args []string) error {
    task, err := taskService.CreateTask(args[0], dueDate)
    if err != nil {
        return fmt.Errorf("failed to create task: %w", err)
    }
    fmt.Printf("Created task: %s\n", task.ID)
    return nil
}
```

**Benefits:**
- Testable business logic
- Reusable across CLI/TUI
- Clear separation of concerns

**Frequency:** ✅✅✅✅ (Good adoption)

---

## Pattern Extraction Process

When completing a code review:

### 1. Parse Review Findings

From `.claude/work/<ticket-id>/review.md`, extract:
- Issue category (error handling, resource management, etc.)
- Specific pattern violated
- File and line number
- Fix applied

### 2. Check Frequency

Look through previous reviews:
```bash
# Find similar issues
grep -r "Missing error context" .claude/work/*/review.md | wc -l
```

### 3. Update Frequency Count

In this file, update the pattern:
- 1-2 occurrences: 🟡 (Watch)
- 3-5 occurrences: 🔴 (Common)
- 6-10 occurrences: 🔴🔴 (Very common)
- 11+ occurrences: 🔴🔴🔴+ (Critical pattern)

### 4. Decide on Action

| Frequency | Action |
|-----------|--------|
| 1-2x | Document here, monitor |
| 3-5x | Add to subsystem AGENTS.md warnings |
| 6-10x | Add to preflight checks |
| 11+ | Add to preflight + implementer critical patterns |

### 5. Update Guides

Add pattern to relevant files:
- **Preflight checks:** `docs/agents/preflight.md` - Manual Checks section
- **Implementer guide:** `docs/agents/implementer.md` - Implementation Guidelines
- **Subsystem guides:** `internal/*/AGENTS.md` - Common Pitfalls section
- **Testing guide:** `docs/TESTING.md` - if test-related

### 6. Create Preflight Automation (if applicable)

For mechanical patterns, add automated check:

```bash
# Example: Check for unwrapped errors
grep -n "return err$" <files> > /tmp/unwrapped_errors.txt
if [ -s /tmp/unwrapped_errors.txt ]; then
    echo "❌ Found unwrapped errors:"
    cat /tmp/unwrapped_errors.txt
fi
```

## Metrics to Track

Store in `.claude/metrics/`:

### Per-Ticket Metrics
`.claude/metrics/<ticket-id>.json`:
```json
{
  "ticket_id": "reckon-abc",
  "subsystem": "cli",
  "phases": {
    "planner": {"attempts": 1, "duration_minutes": 5},
    "test_writer": {"attempts": 1, "duration_minutes": 3},
    "implementer": {"attempts": 2, "duration_minutes": 12},
    "preflight": {"attempts": 1, "duration_minutes": 1},
    "reviewer": {"attempts": 1, "duration_minutes": 4}
  },
  "issues_found": {
    "preflight": ["missing_defer", "unwrapped_error"],
    "review": ["nil_check_missing"]
  },
  "review_verdict": "APPROVE_WITH_CHANGES",
  "test_coverage": 78.5,
  "files_changed": 3,
  "cost_estimate": {
    "opus": "$0.45",
    "sonnet": "$0.12",
    "haiku": "$0.02",
    "total": "$0.59"
  }
}
```

### Aggregate Metrics
`.claude/metrics/summary.json`:
```json
{
  "total_tickets": 25,
  "success_rate": 0.92,
  "avg_retry_rate": {
    "implementer": 1.3,
    "preflight": 0.8,
    "reviewer": 0.4
  },
  "common_issues": {
    "unwrapped_error": 15,
    "missing_defer": 12,
    "closure_capture": 8
  },
  "avg_cost_per_ticket": "$0.62",
  "total_cost": "$15.50"
}
```

## Continuous Improvement Loop

```
┌─────────────────────────────────────────────────┐
│ 1. Code Review                                  │
│    └─> Find patterns (good & bad)              │
└────────────────┬────────────────────────────────┘
                 │
                 v
┌─────────────────────────────────────────────────┐
│ 2. Pattern Extraction                           │
│    └─> Categorize, track frequency             │
└────────────────┬────────────────────────────────┘
                 │
                 v
┌─────────────────────────────────────────────────┐
│ 3. Frequency Threshold                          │
│    └─> 2-3 occurrences = actionable            │
└────────────────┬────────────────────────────────┘
                 │
                 v
┌─────────────────────────────────────────────────┐
│ 4. Feedback to System                           │
│    ├─> Add to preflight checks                 │
│    ├─> Update subsystem AGENTS.md              │
│    ├─> Update implementer guidelines           │
│    └─> Update TESTING.md if test pattern       │
└────────────────┬────────────────────────────────┘
                 │
                 v
┌─────────────────────────────────────────────────┐
│ 5. Next Implementation                          │
│    └─> Guided by updated docs                  │
│    └─> Caught by updated preflight             │
│    └─> = Fewer repeated issues                 │
└─────────────────────────────────────────────────┘
```

## Pattern Review Schedule

**Weekly:** Review new patterns from last week's reviews
**Monthly:** Update frequency counts, add high-frequency patterns to guides
**Quarterly:** Analyze metrics, identify trends, major guide updates

## Success Metrics

- **Pattern recurrence rate decreasing** (same issue appears less often)
- **Preflight catch rate increasing** (issues caught before review)
- **Review approval rate increasing** (fewer REQUEST CHANGES)
- **Implementation retry rate decreasing** (get it right first time)

## Notes

This is a **living document**. As the project evolves:
- New patterns emerge → Add them here
- Patterns become rare → Mark as resolved
- Guides improve → Patterns caught earlier in pipeline
- System learns → Fewer repeated mistakes

**Goal:** Eventually, common mistakes become impossible to make because the system prevents them proactively.
