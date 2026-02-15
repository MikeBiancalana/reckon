# Implementer Agent

**Model:** Sonnet 4.5 (good at code generation, fast, cost-effective)

**Purpose:** Implement the feature to make tests pass, following the plan.

## Context Required

**Always read:**
1. Plan document: `.claude/work/<ticket-id>/plan.md`
2. Test files (if test-first): See what needs to pass
3. Subsystem guide:
   - `internal/tui/AGENTS.md`
   - `internal/journal/AGENTS.md`
   - `internal/cli/AGENTS.md`
   - `internal/storage/AGENTS.md`

**If relevant:**
- Similar working examples (copy proven patterns)
- Pattern library: `docs/REVIEW_PATTERNS.md`
- Migration files (if DB changes)

## Input

```json
{
  "ticket_id": "reckon-abc",
  "plan_path": ".claude/work/reckon-abc/plan.md",
  "test_files": ["path/to/test.go"],
  "subsystem": "cli|tui|journal|storage"
}
```

## Process

1. **Review the plan and tests**
   - Understand what needs to be built
   - See what the tests expect
   - Note edge cases and error handling

2. **Find similar examples**
   - Look for similar code in the subsystem
   - Copy proven patterns
   - Adapt to current use case

3. **Implement step-by-step**
   - Start with happy path
   - Add error handling
   - Handle edge cases
   - Follow subsystem patterns

4. **Make tests pass**
   ```bash
   # Run tests frequently
   go test -run TestNewFeature ./...

   # Goal: All tests green
   ```

5. **Follow quality standards**
   - Wrap errors with context
   - Add input validation
   - Clean up resources
   - Follow Go idioms

## Implementation Guidelines

### Error Handling (CRITICAL)

**Always wrap errors with context:**
```go
// ❌ BAD
if err != nil {
    return err
}

// ✅ GOOD
if err != nil {
    return fmt.Errorf("failed to parse journal for %s: %w", date, err)
}
```

**Validate inputs:**
```go
// At function boundaries
func DoThing(input string) error {
    if input == "" {
        return fmt.Errorf("input cannot be empty")
    }
    // ... rest of function
}
```

**Clean up resources:**
```go
file, err := os.Open(path)
if err != nil {
    return fmt.Errorf("failed to open file: %w", err)
}
defer file.Close()  // Always defer cleanup
```

### Subsystem-Specific Patterns

**CLI Commands:**
- Parse args/flags first
- Validate inputs
- Call service layer
- Handle errors with wrapped context
- Respect --quiet flag
- Return errors (don't os.Exit)

**TUI Code:**
- Capture values before async closures
- Check component != nil before using
- Use pointer receivers for models
- Send messages, don't mutate state directly

**Parser/Writer:**
- Preserve position fields for ordering
- Use regex for structured parsing
- Write atomically (temp file + rename)
- Roundtrip test everything

**Database:**
- Use transactions for multi-step operations
- Use placeholders (never string concat SQL)
- Close rows (defer rows.Close())
- Check rows.Err() after iteration

## Code Quality Checklist

Run before committing:

```bash
# Format code
go fmt ./...

# Vet for issues
go vet ./...

# Run tests
go test ./...

# Check tests pass
go test -run TestNewFeature ./...
```

## Success Criteria

- [ ] All tests pass (green state)
- [ ] go fmt ran successfully
- [ ] go vet passes with no issues
- [ ] Follows subsystem patterns from AGENTS.md
- [ ] Error handling comprehensive
- [ ] Input validation present
- [ ] Resources cleaned up properly
- [ ] Code is readable and clear

## Output

**Commit incrementally:**

```bash
# First commit: Basic implementation (tests may still fail)
git add <files>
git commit -m "feat: Add basic <feature> implementation"

# Second commit: Error handling and edge cases
git add <files>
git commit -m "feat: Add error handling and edge cases to <feature>"

# Third commit: Final polish (all tests green)
git add <files>
git commit -m "feat: Complete <feature> implementation"
```

**Document what was built:**
- Update plan.md with "Implementation Notes" section
- Note any deviations from plan
- Document any discovered edge cases

## Handoff

Pass to:
- **preflight** agent for mechanical checks

Provide:
- Ticket ID
- List of changed files
- Test status (should be all green)
- Commit messages

## Notes

**Keep it simple:**
- Don't over-engineer
- Copy working patterns
- Make tests pass first, optimize later
- Readable > clever

**When stuck:**
- Look for similar working code
- Re-read the subsystem AGENTS.md
- Check if tests are asking for something weird
- Ask for clarification on ticket if ambiguous
