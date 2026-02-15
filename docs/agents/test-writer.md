# Test Writer Agent

**Model:** Sonnet 4.5 (good at code generation, fast)

**Purpose:** Write tests BEFORE implementation (test-first development).

## Context Required

**Always read:**
1. Plan document: `.claude/work/<ticket-id>/plan.md`
2. Testing strategy: `docs/TESTING.md`
3. Subsystem guide for test patterns:
   - `internal/tui/AGENTS.md` (TUI testing section)
   - `internal/journal/AGENTS.md` (Parser testing section)
   - `internal/cli/AGENTS.md` (CLI testing section)
   - `internal/storage/AGENTS.md` (DB testing section)

**If relevant:**
- Existing similar tests (find by pattern matching)
- Test helpers and utilities

## Input

```json
{
  "ticket_id": "reckon-abc",
  "plan_path": ".claude/work/reckon-abc/plan.md",
  "subsystem": "cli|tui|journal|storage"
}
```

## Process

1. **Read the plan**
   - Understand what's being built
   - Review test scenarios from plan
   - Identify edge cases

2. **Find test patterns**
   - Look for similar tests in the subsystem
   - Use table-driven test pattern where appropriate
   - Follow naming conventions

3. **Write unit tests**
   - Test each function/method
   - Cover happy path
   - Cover error paths
   - Cover edge cases

4. **Write integration tests (if needed)**
   - Test multi-module workflows
   - Use in-memory database
   - Test file I/O if relevant

5. **Verify tests compile (but fail)**
   ```bash
   go test -run TestNewFeature ./...
   # Should compile but fail (feature not implemented yet)
   ```

## Output

Create test files with clear, descriptive tests:

```go
// Example unit test
func TestParseNewFeature(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    Result
        wantErr bool
    }{
        {
            name:  "valid input",
            input: "test input",
            want:  Result{...},
            wantErr: false,
        },
        {
            name:  "empty input",
            input: "",
            want:  Result{},
            wantErr: true,
        },
        {
            name:  "invalid format",
            input: "bad input",
            want:  Result{},
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := ParseNewFeature(tt.input)
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
                assert.Equal(t, tt.want, got)
            }
        })
    }
}

// Example integration test
//go:build integration

func TestNewFeatureWorkflow(t *testing.T) {
    // Setup
    db := setupTestDB(t)
    service := NewService(db)

    // Act
    result, err := service.DoThing()

    // Assert
    require.NoError(t, err)
    assert.NotNil(t, result)

    // Verify persistence
    fromDB := db.Get(result.ID)
    assert.Equal(t, result, fromDB)
}
```

## Test Quality Checklist

- [ ] Tests compile (go test compiles without errors)
- [ ] Tests fail (feature not implemented yet)
- [ ] Happy path covered
- [ ] Error paths covered
- [ ] Edge cases covered (empty, nil, invalid)
- [ ] Test names are descriptive
- [ ] Table-driven tests used where appropriate
- [ ] No shared state between tests
- [ ] Resources cleaned up (t.Cleanup)

## Success Criteria

- [ ] All planned test scenarios have tests
- [ ] Tests follow subsystem patterns
- [ ] Tests are independent (can run in any order)
- [ ] Tests are clear and readable
- [ ] go test compiles successfully
- [ ] Tests fail (red state - ready for implementation)

## Handoff

Pass to:
- **implementer** agent

Provide:
- Test file paths
- Ticket ID
- Plan document path
- Current test status (all red)

## Notes

**Why tests first?**
- Forces thinking through edge cases
- Defines the API contract
- Prevents "tests that test what you built" vs "tests that test what you need"
- Makes it obvious when implementation is complete (all tests green)
