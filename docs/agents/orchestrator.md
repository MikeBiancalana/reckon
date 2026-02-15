# Orchestrator Agent

**Model:** Sonnet 4.5 (good at coordination, cost-effective)

**Purpose:** Coordinate the specialized agent pipeline for ticket implementation.

## Overview

The orchestrator chains together the specialist agents (planner → test-writer → implementer → preflight → reviewer) to implement a beads ticket from start to finish.

**Key responsibilities:**
- Initialize work directory for ticket
- Call agents in correct sequence
- Pass context between agents
- Handle retry loops (preflight failures, review feedback)
- Capture and store artifacts
- Track workflow state

## Input

```json
{
  "ticket_id": "reckon-abc",
  "subsystem": "cli|tui|journal|storage",
  "approach": "test-first|code-first",
  "auto_fix": true  // Automatically fix preflight issues vs. return to user
}
```

## Process

### 1. Initialize Workspace

```bash
# Create work directory
mkdir -p .claude/work/<ticket-id>

# Get ticket details
bd show <ticket-id> > .claude/work/<ticket-id>/ticket.txt

# Determine subsystem if not provided
# Look at ticket description, affected files, labels
```

### 2. Planning Phase

**Call:** planner agent

**Input:**
```json
{
  "ticket_id": "reckon-abc",
  "ticket_description": "<from bd show>",
  "subsystem": "cli"
}
```

**Output:** `.claude/work/<ticket-id>/plan.md`

**Validation:**
- Plan file exists
- Has all required sections (Summary, Files to Modify, Design Decisions, Test Scenarios)
- No placeholders or TODOs

**On failure:** Re-run planner with feedback

### 3. Test Writing Phase (if approach == "test-first")

**Call:** test-writer agent

**Input:**
```json
{
  "ticket_id": "reckon-abc",
  "plan_path": ".claude/work/reckon-abc/plan.md",
  "subsystem": "cli"
}
```

**Output:** Test files (failing tests)

**Validation:**
```bash
# Tests compile
go test -run TestNewFeature ./... 2>&1 | grep -q "FAIL"
# Tests fail (not implemented yet)
```

**On failure:** Re-run test-writer with compilation errors

### 4. Implementation Phase

**Call:** implementer agent

**Input:**
```json
{
  "ticket_id": "reckon-abc",
  "plan_path": ".claude/work/reckon-abc/plan.md",
  "test_files": ["path/to/test.go"],
  "subsystem": "cli"
}
```

**Output:** Implementation code

**Validation:**
```bash
# Tests pass
go test -run TestNewFeature ./...
echo $?  # Should be 0
```

**On failure:**
- Check error messages
- Re-run implementer with test failures as context
- Max 3 retries before escalating

### 5. Preflight Phase

**Call:** preflight agent

**Input:**
```json
{
  "ticket_id": "reckon-abc",
  "changed_files": ["file1.go", "file2_test.go"],
  "subsystem": "cli"
}
```

**Output:** `.claude/work/<ticket-id>/preflight-report.md`

**Validation:**
- Status == PASS or PASS WITH WARNINGS
- No critical issues

**On FAIL:**
- If auto_fix == true:
  - Pass issues to implementer
  - Re-run preflight after fixes
  - Max 2 retry loops
- If auto_fix == false:
  - Return to user with preflight report

### 6. Review Phase

**Call:** reviewer agent

**Input:**
```json
{
  "ticket_id": "reckon-abc",
  "changed_files": ["file1.go", "file2_test.go"],
  "plan_path": ".claude/work/reckon-abc/plan.md",
  "preflight_report_path": ".claude/work/reckon-abc/preflight-report.md",
  "subsystem": "cli"
}
```

**Output:** `.claude/work/<ticket-id>/review.md`

**Validation:**
- Verdict in {APPROVE, APPROVE WITH CHANGES, REQUEST CHANGES}

**On REQUEST CHANGES:**
- Extract required changes
- Pass to implementer
- Re-run preflight + review
- Max 2 feedback loops before escalating to user

**On APPROVE or APPROVE WITH CHANGES:**
- Proceed to completion

### 7. Completion

**Success criteria:**
- All tests pass
- Preflight: PASS or PASS WITH WARNINGS
- Review: APPROVE or APPROVE WITH CHANGES

**Final actions:**
```bash
# Create summary
cat > .claude/work/<ticket-id>/summary.md <<EOF
# Implementation Summary: <ticket-id>

## Status: COMPLETE

## Artifacts:
- Plan: plan.md
- Preflight: preflight-report.md
- Review: review.md

## Changed Files:
$(git diff --name-only)

## Test Results:
$(go test ./...)

## Review Verdict:
$(grep "Verdict:" review.md)

## Ready for: Merge
EOF

# Update beads ticket
bd update <ticket-id> --notes="Implementation complete. See .claude/work/<ticket-id>/summary.md"
```

## Error Handling

### Retry Loops

**Implementer failures:**
- Max 3 attempts to make tests pass
- On exceed: Return to planner (design issue?)

**Preflight failures:**
- Max 2 fix attempts (if auto_fix)
- On exceed: Return to user with issues

**Review request changes:**
- Max 2 feedback loops
- On exceed: Return to user (needs discussion?)

### Escalation

When max retries exceeded:
```markdown
# Workflow Escalation: <ticket-id>

## Phase stuck: <implementer|preflight|reviewer>

## Issue:
<Description of blocking issue>

## Attempts:
1. [What was tried]
2. [What was tried]

## Artifacts:
- Plan: .claude/work/<ticket-id>/plan.md
- Latest error: <error message>
- Changed files: <list>

## Recommendation:
<Specific user action needed>
```

## Workflow State

Track state in `.claude/work/<ticket-id>/state.json`:

```json
{
  "ticket_id": "reckon-abc",
  "phase": "implementer",
  "attempts": {
    "planner": 1,
    "test_writer": 1,
    "implementer": 2,
    "preflight": 0,
    "reviewer": 0
  },
  "artifacts": {
    "plan": true,
    "tests": true,
    "preflight_report": false,
    "review": false
  },
  "changed_files": ["cmd/notes.go", "cmd/notes_test.go"],
  "status": "in_progress"
}
```

## Success Metrics

**Track for continuous improvement:**
- Average time per phase
- Retry rates by phase
- Common preflight issues
- Common review issues
- Overall success rate (APPROVE on first review)

**Store in:** `.claude/metrics/<ticket-id>.json`

## Output

**On success:**
```json
{
  "status": "complete",
  "ticket_id": "reckon-abc",
  "artifacts": {
    "plan": ".claude/work/reckon-abc/plan.md",
    "preflight": ".claude/work/reckon-abc/preflight-report.md",
    "review": ".claude/work/reckon-abc/review.md",
    "summary": ".claude/work/reckon-abc/summary.md"
  },
  "verdict": "APPROVE",
  "changed_files": ["file1.go", "file2_test.go"],
  "ready_for": "merge"
}
```

**On failure/escalation:**
```json
{
  "status": "escalated",
  "ticket_id": "reckon-abc",
  "stuck_phase": "implementer",
  "reason": "Max retry attempts exceeded",
  "artifacts": {
    "plan": ".claude/work/reckon-abc/plan.md",
    "error_log": ".claude/work/reckon-abc/errors.txt"
  },
  "recommendation": "Review test expectations - may be asking for wrong behavior"
}
```

## Integration with /work-ticket

Replace current skill implementation:

```go
// Current: Manual worktree + single agent
/work-ticket reckon-abc

// Future: Orchestrated pipeline
/work-ticket reckon-abc --pipeline
// → Creates worktree
// → Runs orchestrator
// → Returns summary
```

## Future Enhancements

1. **Parallel planning:** For epics, split into subtasks and run parallel pipelines
2. **Incremental commits:** Commit after each phase (plan → tests → impl → polish)
3. **Pattern mining:** Extract patterns from review findings → update preflight
4. **Cost tracking:** Log model usage and costs per ticket
5. **Quality trends:** Track metrics over time (test coverage, review issues, etc.)

## Notes

**Why Sonnet for orchestration?**
- Coordination doesn't need Opus-level reasoning
- Mostly mechanical task: call agents, check outputs, handle retries
- Cost-effective for the coordinator role
- Fast enough for interactive use

**When to use orchestrator:**
- Straightforward tickets (clear requirements)
- Automated workflow (CI/CD integration)
- Batch processing multiple tickets

**When NOT to use orchestrator:**
- Complex architectural decisions (need human input)
- Ambiguous requirements (need clarification)
- Tickets requiring multiple approaches (need exploration)
