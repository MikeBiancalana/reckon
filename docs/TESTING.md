# Testing Strategy

## Philosophy

**Test behavior, not implementation.** Tests should verify that the system does the right thing, not how it does it.

**Key principles:**
- Tests should be fast and reliable
- Tests should be independent (no shared state)
- Test one thing at a time
- Prefer many small tests over few large tests
- Integration tests validate multi-module workflows

## Test Pyramid

```
        /\
       /  \      5% - End-to-End (Full "day's work" validation)
      /----\
     /      \    25% - Integration (Multi-module workflows)
    /--------\
   /          \  70% - Unit (Isolated component testing)
  /____________\
```

**Unit Tests (70%)** - Fast, isolated, mock dependencies
- Parser/writer correctness
- Service logic with mocked repositories
- Model validation
- Utility functions

**Integration Tests (25%)** - Real database, multiple modules
- Input → parsing → storage → retrieval workflows
- File I/O + database sync
- Service layer with real repository

**End-to-End Tests (5%)** - Full system validation
- "Day's work" simulation (create logs, tasks, notes, verify connections)
- TUI visual/UX validation
- CLI command chains

## Unit Tests

### What We Test

**Location:** `*_test.go` files next to implementation

**Coverage areas:**
- **Parsers:** Markdown → models (use golden files in `testdata/`)
- **Writers:** Models → markdown (roundtrip tests)
- **Services:** Business logic with mocked repositories
- **Models:** Validation, methods, state transitions

**What we DON'T test:**
- Actual file I/O (use mocks/in-memory)
- External dependencies (git, network)
- Bubble Tea rendering (test model updates only)

### Example Unit Test

```go
func TestParseIntention(t *testing.T) {
    content := "- [ ] Review PR #123"
    intention, err := parseIntention(content, 0)

    assert.NoError(t, err)
    assert.Equal(t, "Review PR #123", intention.Text)
    assert.Equal(t, "open", intention.Status)
}
```

### Table-Driven Tests

Use for testing multiple cases:

```go
func TestParseDuration(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected int
        wantErr  bool
    }{
        {"minutes only", "30m", 30, false},
        {"hours only", "2h", 120, false},
        {"hours and minutes", "1h30m", 90, false},
        {"invalid format", "30", 0, true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := parseDuration(tt.input)
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
                assert.Equal(t, tt.expected, result)
            }
        })
    }
}
```

### Roundtrip Tests

Ensure parse → write → parse produces same result:

```go
func TestJournalRoundtrip(t *testing.T) {
    original := createTestJournal()

    // Write
    content, err := WriteJournalToString(original)
    require.NoError(t, err)

    // Parse
    parsed, err := ParseJournal(content, original.Date)
    require.NoError(t, err)

    // Compare
    assert.Equal(t, original, parsed)
}
```

## Integration Tests

### What We Test

**Location:** `tests/integration_test.go` or `*_integration_test.go`

**Coverage areas:**
- **End-to-end workflows:** User action → storage → retrieval
- **Multi-module coordination:** Parser + Service + Repository + File I/O
- **Data persistence:** Write to DB, read back, verify correctness
- **Migration scenarios:** Old data format → new format

**Build tag:** Use `//go:build integration` to separate from unit tests

### Example Integration Test

Test a complete workflow across multiple modules:

```go
//go:build integration

package tests

func TestIntentionWorkflow(t *testing.T) {
    // Setup
    db := setupTestDB(t)
    fileStore := setupTestFileStore(t)
    service := journal.NewService(db, fileStore)

    // 1. Create intention via service
    journal := service.GetJournal("2025-01-15")
    err := service.AddIntention(journal, "Complete task X")
    require.NoError(t, err)

    // 2. Verify written to file
    content := fileStore.ReadJournalFile("2025-01-15")
    assert.Contains(t, content, "[ ] Complete task X")

    // 3. Parse file back
    parsed, err := ParseJournal(content, "2025-01-15")
    require.NoError(t, err)

    // 4. Verify in database
    fromDB, err := db.GetJournal("2025-01-15")
    require.NoError(t, err)
    assert.Equal(t, parsed.Intentions, fromDB.Intentions)
}
```

### Common Integration Scenarios

**Multi-module workflows to test:**
- Create task → Log time to it → Verify task.Notes updated
- Create note → Link to another note → Verify backlinks
- Edit intention in file → Rebuild DB → Verify sync
- Carry intention forward → Verify carried status + new journal entry
- Add log entry with task reference → Verify task shows in log history
- Create schedule item with duration → Parse → Store → Retrieve

### Running Integration Tests

```bash
# Run only integration tests
go test -tags integration ./tests/

# Run with verbose output
go test -tags integration -v ./tests/

# Run specific integration test
go test -tags integration -run TestIntentionWorkflow ./tests/
```

## TUI Visual & UX Testing

### Tool Stack

**xterm.js + gotty + Playwright MCP**

- **gotty:** Serves TUI over WebSocket (makes terminal accessible via browser)
- **xterm.js:** Renders terminal in browser (client-side terminal emulator)
- **Playwright MCP:** Automates browser interactions and captures screenshots

### Setup

1. Install gotty: `go install github.com/yudai/gotty@latest`
2. Start TUI via gotty: `gotty -w rk`
3. Open browser to gotty URL (default: http://localhost:8080)
4. Use Playwright MCP to interact with xterm.js terminal

### What to Test

**Visual regression:**
- Layout rendering (40-40-18 columns visible)
- Color schemes and styling (task colors, done strikethrough)
- Border rendering and alignment
- Long text truncation/wrapping

**Interaction testing:**
- Keyboard navigation (tab, j/k, arrows)
- Component focus states (highlighted sections)
- Help text visibility (toggle with '?')
- Error message display
- Modal dialogs (confirmation prompts)

**UX validation:**
- Response time (subjective feel)
- Smooth transitions
- Clear visual feedback for actions
- Intuitive keybindings

### Example Test Scenario

```javascript
// Playwright test via MCP
test('Task list shows completed tasks with strikethrough', async ({ page }) => {
    // 1. Navigate to gotty URL
    await page.goto('http://localhost:8080');

    // 2. Wait for terminal to load
    await page.waitForSelector('.xterm');

    // 3. Navigate to tasks section (tab key)
    await page.keyboard.press('Tab');
    await page.keyboard.press('Tab'); // Navigate to tasks

    // 4. Create task (press 't')
    await page.keyboard.press('t');
    await page.keyboard.type('Test task for visual check');
    await page.keyboard.press('Enter');

    // 5. Mark task done (space key)
    await page.keyboard.press('Space');

    // 6. Screenshot and verify strikethrough styling
    const screenshot = await page.screenshot();
    // Visual comparison with baseline screenshot

    // 7. Verify task text contains strikethrough
    const terminalText = await page.textContent('.xterm-rows');
    expect(terminalText).toContain('Test task for visual check');
});
```

### When to Use TUI Testing

**Use for:**
- Visual regression testing (compare screenshots before/after changes)
- UX flow validation (multi-step interactions)
- Accessibility checks (focus indicators, contrast ratios)
- Cross-terminal compatibility (different terminal emulators)

**Don't use for:**
- Regular CI/CD (too slow, complex setup)
- Unit-level testing (use model tests instead)
- Automated test suites (manual or semi-automated only)

### Notes

- This is primarily for **manual validation** and **visual QA**
- Not part of regular CI pipeline (too complex/slow)
- Useful for major UX changes or redesigns
- Can capture baseline screenshots for regression testing

## End-to-End "Day's Work" Validation

### Concept

Simulate a full day of productivity work and verify all connections work correctly.

### Status

🎂 **Icing on the cake** - Nice to have, not implemented yet.

This test would live in `tests/e2e_test.go` and serve as both validation and living documentation of how the system should work.

### Example Scenario

```go
//go:build e2e

package tests

func TestFullDayWorkflow(t *testing.T) {
    day := "2025-01-15"

    // Morning: Set intentions
    service.AddIntention(journal, "Review PRs")
    service.AddIntention(journal, "Work on feature X")

    // Create task for multi-day work
    task := taskService.CreateTask("Implement auth system")
    taskService.AddTag(task, "backend")

    // Log work throughout day
    service.AddLog(journal, "09:00", "Started reviewing PRs")
    service.AddLog(journal, "10:30", fmt.Sprintf("[task:%s] Working on auth", task.ID))
    service.AddLog(journal, "12:00", "[break] Lunch")

    // Create zettelkasten note
    note := notesService.CreateNote("OAuth Flow Patterns", []string{"auth", "backend"})

    // Link note to task (future feature)
    // taskService.LinkNote(task.ID, note.Slug)

    // Afternoon: More work
    service.AddLog(journal, "14:00", fmt.Sprintf("[task:%s] Implemented token refresh", task.ID))
    taskService.AddNote(task, "Need to handle edge case: expired refresh token")

    // End of day: Mark intention done, add win
    service.ToggleIntention(journal, intentionID)
    service.AddWin(journal, "Completed PR reviews, made good progress on auth")

    // ========================================
    // VERIFY EVERYTHING CONNECTED PROPERLY
    // ========================================

    // 1. Journal file has all entries
    content := readJournalFile(day)
    assert.Contains(t, content, "Review PRs")
    assert.Contains(t, content, "[break] Lunch")
    assert.Contains(t, content, "Completed PR reviews")

    // 2. Task has notes
    loadedTask := taskService.GetTask(task.ID)
    assert.Len(t, loadedTask.Notes, 1)
    assert.Contains(t, loadedTask.Notes[0].Text, "expired refresh token")

    // 3. Task appears in correct log entries
    logs := service.GetLogEntries(day)
    taskLogs := filterByTaskID(logs, task.ID)
    assert.Len(t, taskLogs, 2)
    assert.Contains(t, taskLogs[0].Content, "Working on auth")
    assert.Contains(t, taskLogs[1].Content, "Implemented token refresh")

    // 4. Note is findable by tag
    authNotes := notesService.GetNotesByTag("auth")
    assert.Contains(t, authNotes, note)

    // 5. Intention was marked done
    intentions := service.GetIntentions(day)
    reviewIntention := findIntentionByText(intentions, "Review PRs")
    assert.Equal(t, "done", reviewIntention.Status)

    // 6. Database and files are in sync
    dbJournal := repo.GetJournal(day)
    fileJournal := parseJournalFile(day)
    assert.Equal(t, dbJournal, fileJournal)

    // 7. Time tracking is accurate
    totalTime := calculateTotalTime(logs)
    assert.Greater(t, totalTime, 0, "Should have logged time")

    // 8. All IDs are valid and resolvable
    assert.NotEmpty(t, task.ID)
    assert.NotEmpty(t, note.ID)
    for _, log := range logs {
        assert.NotEmpty(t, log.ID)
    }
}
```

### Value

**Why this test matters:**
- Catches integration bugs missed by unit tests
- Validates real-world usage patterns
- Ensures data consistency across all modules
- Documents expected system behavior
- Verifies all connections (tasks↔logs, notes↔tasks, etc.)

**When to implement:**
- After core features are stable
- Before major refactoring
- As living documentation of system behavior
- When adding complex cross-module features

## Running Tests

### Quick Reference

```bash
# All tests (unit only, default)
go test ./...

# Specific package
go test ./internal/journal/...

# Single test by name
go test -run TestParseIntention ./internal/journal/

# With coverage report
go test -cover ./...

# Detailed coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out  # View in browser

# Integration tests only
go test -tags integration ./tests/

# Verbose output (show test names as they run)
go test -v ./...

# Run tests in parallel (4 workers)
go test -p 4 ./...

# Run specific test file
go test ./internal/journal/parser_test.go

# Run until failure (stress test)
go test -count=100 ./internal/journal/

# Run with race detection
go test -race ./...

# Benchmark tests
go test -bench=. ./...
```

### Test Organization

```
reckon/
├── internal/
│   ├── journal/
│   │   ├── parser.go
│   │   ├── parser_test.go        # Unit tests
│   │   └── testdata/             # Golden files
│   │       ├── journal-basic.md
│   │       └── journal-with-tasks.md
│   ├── tui/
│   │   ├── model.go
│   │   └── model_test.go         # TUI model tests
│   └── storage/
│       ├── database.go
│       └── database_test.go
└── tests/
    ├── integration_test.go        # Integration tests
    └── e2e_test.go               # End-to-end tests (future)
```

## Coverage Goals

### Target Coverage by Subsystem

| Subsystem | Target | Rationale |
|-----------|--------|-----------|
| **Parsers/Writers** | >90% | Critical for data correctness |
| **Service layer** | >80% | Business logic must be solid |
| **Repository** | >70% | Mostly CRUD, but important |
| **TUI models** | >60% | Harder to test, focus on critical paths |
| **CLI commands** | >60% | User-facing, test happy paths |
| **Overall project** | >70% | Good balance of coverage vs effort |

### Coverage is a Guide, Not a Goal

Don't test for coverage's sake. Focus on:
- **Critical paths** - Data flow, user actions that matter
- **Edge cases** - Empty input, invalid data, boundary conditions
- **Error handling** - What happens when things fail
- **Regression prevention** - Test bugs once fixed

**Good coverage:**
```go
// Tests actual user scenarios
func TestParseJournal_RealWorldFile(t *testing.T) { ... }
func TestParseJournal_EmptyFile(t *testing.T) { ... }
func TestParseJournal_InvalidDate(t *testing.T) { ... }
```

**Bad coverage chasing:**
```go
// Just testing getters for coverage numbers
func TestIntention_GetID(t *testing.T) {
    i := Intention{ID: "123"}
    assert.Equal(t, "123", i.GetID())
}
```

## Test Data

### Golden Files

Use `testdata/` directories for expected inputs/outputs:

```
internal/journal/testdata/
├── journal-basic.md          # Simple journal file
├── journal-with-tasks.md     # Journal with task references
├── journal-empty.md          # Empty journal
├── intention-formats.md      # Various intention formats
└── expected-output.md        # Expected parsed result
```

**Loading golden files:**
```go
func TestParseJournal_Basic(t *testing.T) {
    content := readTestFile(t, "testdata/journal-basic.md")
    journal, err := ParseJournal(content, "2025-01-15")

    assert.NoError(t, err)
    assert.Equal(t, 3, len(journal.Intentions))
}

func readTestFile(t *testing.T, path string) string {
    content, err := os.ReadFile(path)
    require.NoError(t, err, "Failed to read test file")
    return string(content)
}
```

### In-Memory Database

Use `:memory:` for fast, isolated database tests:

```go
func setupTestDB(t *testing.T) *storage.Database {
    db, err := storage.NewDatabase(":memory:")
    require.NoError(t, err, "Failed to create test database")

    t.Cleanup(func() {
        db.Close()
    })

    return db
}
```

**Benefits:**
- No disk I/O (fast)
- Isolated (no shared state between tests)
- Clean (fresh database for each test)
- No cleanup needed (memory-only)

### Test Helpers

Create reusable setup/teardown functions:

```go
// Create test journal with standard data
func createTestJournal(date string) *journal.Journal {
    return &journal.Journal{
        Date: date,
        Intentions: []journal.Intention{
            {ID: "int1", Text: "Test intention 1", Status: "open"},
            {ID: "int2", Text: "Test intention 2", Status: "done"},
        },
        Log: []journal.LogEntry{
            {ID: "log1", Timestamp: "09:00", Content: "Started work"},
        },
    }
}

// Setup full test environment
func setupTestEnvironment(t *testing.T) (*journal.Service, *storage.Database) {
    db := setupTestDB(t)
    fileStore := &mockFileStore{}
    service := journal.NewService(db, fileStore)
    return service, db
}
```

## Writing Good Tests

### DO

✅ **Test one thing at a time**
```go
// Good - tests parsing only
func TestParseIntention_OpenStatus(t *testing.T) {
    result, _ := parseIntention("- [ ] Task")
    assert.Equal(t, "open", result.Status)
}
```

✅ **Use descriptive test names**
```go
// Pattern: TestFunction_Scenario_ExpectedBehavior
func TestParseIntention_WithCarriedStatus_SetsCarriedFrom(t *testing.T)
func TestAddLog_WithTaskReference_UpdatesTaskHistory(t *testing.T)
```

✅ **Use table-driven tests for multiple cases**
```go
func TestParseDuration(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected int
    }{
        {"30 minutes", "30m", 30},
        {"2 hours", "2h", 120},
        {"1.5 hours", "1h30m", 90},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, _ := parseDuration(tt.input)
            assert.Equal(t, tt.expected, result)
        })
    }
}
```

✅ **Check both success and error paths**
```go
func TestParseDate(t *testing.T) {
    // Success case
    date, err := parseDate("2025-01-15")
    assert.NoError(t, err)
    assert.Equal(t, "2025-01-15", date)

    // Error case
    _, err = parseDate("invalid")
    assert.Error(t, err)
}
```

✅ **Clean up resources**
```go
func TestWithTempFile(t *testing.T) {
    tmpFile, _ := os.CreateTemp("", "test-*.md")
    t.Cleanup(func() {
        os.Remove(tmpFile.Name())
    })
    // Test with file
}
```

### DON'T

❌ **Don't test implementation details**
```go
// Bad - tests internal state
func TestParser_InternalBufferSize(t *testing.T) {
    p := NewParser()
    assert.Equal(t, 4096, p.bufferSize) // Who cares?
}

// Good - tests behavior
func TestParser_ParsesLargeFile(t *testing.T) {
    content := strings.Repeat("line\n", 10000)
    result, err := Parse(content)
    assert.NoError(t, err)
    assert.Len(t, result, 10000)
}
```

❌ **Don't share state between tests**
```go
// Bad - shared state
var globalJournal *Journal

func TestA(t *testing.T) {
    globalJournal = &Journal{Date: "2025-01-15"}
}

func TestB(t *testing.T) {
    // Depends on TestA running first!
    assert.Equal(t, "2025-01-15", globalJournal.Date)
}

// Good - independent tests
func TestA(t *testing.T) {
    journal := &Journal{Date: "2025-01-15"}
    // Test with journal
}

func TestB(t *testing.T) {
    journal := &Journal{Date: "2025-01-16"}
    // Test with fresh journal
}
```

❌ **Don't make tests depend on order**
```go
// Bad - order-dependent (tests run in parallel!)
func TestCreate(t *testing.T) {
    createItem("item1")
}

func TestList(t *testing.T) {
    items := listItems()
    assert.Contains(t, items, "item1") // May fail if TestCreate hasn't run!
}

// Good - self-contained
func TestList(t *testing.T) {
    createItem("item1") // Setup within test
    items := listItems()
    assert.Contains(t, items, "item1")
}
```

❌ **Don't mock everything**
```go
// Bad - over-mocking makes test brittle
mockParser := &MockParser{}
mockWriter := &MockWriter{}
mockDB := &MockDB{}
mockFileStore := &MockFileStore{}
service := NewService(mockParser, mockWriter, mockDB, mockFileStore)

// Good - use real collaborators when simple
db := setupTestDB(t) // Real in-memory DB
parser := NewParser() // Real parser
service := NewService(parser, db)
```

❌ **Don't write timing-dependent tests**
```go
// Bad - flaky test
go doSomethingAsync()
time.Sleep(100 * time.Millisecond) // Hope it's done!
assert.True(t, done)

// Good - use channels or synchronization
done := make(chan bool)
go func() {
    doSomething()
    done <- true
}()
select {
case <-done:
    // Success
case <-time.After(1 * time.Second):
    t.Fatal("Timed out")
}
```

## CI/CD Integration

### Current State

**Local only:** Tests run manually before pushing

```bash
# Developer workflow
go test ./...              # Run all tests
go test -cover ./...       # Check coverage
git commit && git push     # Manual quality gate
```

### Future: GitHub Actions

**Proposed workflow:**

```yaml
name: Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Run unit tests
        run: go test -v ./...

      - name: Run integration tests
        run: go test -tags integration -v ./tests/

      - name: Generate coverage
        run: go test -coverprofile=coverage.out ./...

      - name: Upload coverage
        uses: codecov/codecov-action@v3
        with:
          files: ./coverage.out

      - name: Check coverage threshold
        run: |
          coverage=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//')
          if (( $(echo "$coverage < 70" | bc -l) )); then
            echo "Coverage $coverage% is below 70% threshold"
            exit 1
          fi
```

**Benefits:**
- Automated testing on every push/PR
- Coverage tracking over time
- Fail PRs if tests fail or coverage drops
- Visible test status in GitHub

## Summary

**Test at the right level:**
- Unit tests for logic and correctness
- Integration tests for multi-module workflows
- TUI tests for visual/UX validation
- E2E tests for full system behavior

**Focus on value:**
- Test critical paths and edge cases
- Don't chase coverage percentages
- Write tests that catch real bugs
- Make tests maintainable and clear

**Keep tests fast:**
- Use in-memory databases
- Mock external dependencies
- Run tests in parallel
- Optimize slow tests

**Make tests reliable:**
- No shared state
- No timing dependencies
- No order dependencies
- Clean up resources

For more details on testing specific subsystems, see:
- **TUI testing:** `internal/tui/AGENTS.md`
- **Parser testing:** `internal/journal/AGENTS.md`
- **Database testing:** `internal/storage/AGENTS.md`
