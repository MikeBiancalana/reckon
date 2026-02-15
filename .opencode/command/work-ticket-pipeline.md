---
description: Work on a beads ticket using the specialized agent pipeline
agent: principal-engineer
---

# Agent Pipeline Workflow

**Purpose:** Implement a beads ticket using specialized agents with optimal model stratification (planner → test-writer → implementer → preflight → reviewer).

## Overview

This skill orchestrates the agent pipeline defined in `docs/agents/` to implement a ticket from planning through review. Each phase uses the optimal model and validates outputs before proceeding.

**See:** `docs/agents/README.md` for pipeline details.

## Process

### 1. Select and Claim Ticket

```bash
# Find ready tickets
bd ready

# Claim the ticket atomically
bd update <ticket-id> --status=in_progress
```

If user provided a ticket ID, use that. Otherwise, show `bd ready` output and ask which ticket to work on.

### 2. Initialize Workspace

```bash
# Create work directory
mkdir -p .claude/work/<ticket-id>

# Get ticket details
bd show <ticket-id>

# Determine subsystem from ticket
# Look at description, title, or ask user if unclear
# Options: cli, tui, journal, storage
```

### 3. Planning Phase

**Agent:** planner (Opus 4.6)

**Task:** Design the implementation approach.

Use the Task tool with subagent_type="Plan":
- Provide ticket details
- Specify subsystem
- Request comprehensive plan with design decisions, edge cases, test scenarios

**Output:** `.claude/work/<ticket-id>/plan.md`

**Validation:**
- Plan file exists and is complete
- Has all required sections (Summary, Files to Modify, Design Decisions, Test Scenarios)
- No obvious design flaws

**On failure:** Review plan and iterate if needed.

### 4. Test Writing Phase

**Agent:** test-writer (Sonnet 4.5)

**Task:** Write tests BEFORE implementation (test-first).

Use the Task tool with subagent_type="general-purpose":
- Read the plan document
- Read `docs/TESTING.md` for test patterns
- Read subsystem AGENTS.md for test examples
- Write comprehensive tests (unit + integration)

**Output:** Test files (should compile but fail - red state)

**Validation:**
```bash
# Tests compile
go test -run TestNewFeature ./...

# Tests fail (feature not implemented)
# Exit code should be non-zero
```

**On failure:** Fix compilation errors and retry.

### 5. Implementation Phase

**Agent:** implementer (Sonnet 4.5)

**Task:** Implement the feature to make tests pass.

Use the Task tool with subagent_type="general-purpose":
- Read plan and tests
- Read subsystem AGENTS.md for patterns
- Find similar working examples
- Implement step-by-step
- Make all tests pass

**Output:** Implementation code

**Validation:**
```bash
# All tests pass
go test ./...
# Exit code should be 0
```

**On failure:**
- Review test failures
- Re-read implementation guide
- Max 3 attempts, then escalate to user

**Commit incrementally:**
```bash
git add <files>
git commit -m "feat: Implement <feature> for <ticket-id>"
```

### 6. Preflight Check

**Agent:** preflight (Haiku 4.5 - fast, cheap)

**Task:** Run mechanical quality checks.

Use the Task tool with subagent_type="general-purpose" (or implement as direct commands):

```bash
# Format check
go fmt ./...
git diff --exit-code

# Vet for issues
go vet ./...

# Run all tests
go test ./...

# Check test coverage
go test -cover ./...
```

Manual pattern checks:
- Error handling (all errors wrapped with context?)
- Resource cleanup (defer close on files/db?)
- CLI patterns (respect --quiet flag?)
- TUI patterns (capture before closures?)

**Output:** `.claude/work/<ticket-id>/preflight-report.md`

**Validation:**
- Status is PASS or PASS WITH WARNINGS
- No critical issues

**On FAIL:**
- List specific issues
- Fix automatically if mechanical (formatting, missing defer)
- Re-run preflight
- Max 2 fix cycles, then escalate

**Commit fixes:**
```bash
git add <files>
git commit -m "fix: Address preflight issues for <ticket-id>"
```

### 7. Code Review

**Agent:** reviewer (Opus 4.6)

**Task:** Deep code review across 7 dimensions.

Use the Task tool with subagent_type="code-reviewer":
- Read plan, preflight report, implementation
- Review correctness, architecture, testing, maintainability
- Check against subsystem patterns
- Identify any subtle issues

**Output:** `.claude/work/<ticket-id>/review.md`

**Validation:**
- Verdict in {APPROVE, APPROVE WITH CHANGES, REQUEST CHANGES}

**On REQUEST CHANGES:**
- Extract required changes list
- Return to implementation phase
- Re-run preflight + review
- Max 2 feedback loops, then escalate to user for discussion

**On APPROVE or APPROVE WITH CHANGES:**
- Proceed to completion

### 8. Feedback Loop

**Purpose:** Extract patterns from review findings for continuous improvement.

**See:** `docs/agents/feedback-loop.md` for complete process.

**Actions:**

1. **Extract patterns from review:**
   ```bash
   # Parse review findings
   grep -A 3 "### Issues" .claude/work/<ticket-id>/review.md > /tmp/review-issues.txt
   ```

2. **Check pattern frequency:**
   ```bash
   # Count occurrences of common patterns across all reviews
   mkdir -p .claude/metrics

   for pattern in "unwrapped error" "missing defer" "closure capture" "nil check" "missing validation"; do
     count=$(grep -ir "$pattern" .claude/work/*/review.md 2>/dev/null | wc -l)
     echo "$pattern: $count" >> .claude/work/<ticket-id>/pattern-frequency.txt
   done
   ```

3. **Update pattern library:**
   - Open `docs/REVIEW_PATTERNS.md`
   - Update frequency counts for any patterns found
   - Add new patterns if discovered

4. **Check thresholds:**
   - **3+ occurrences:** Add warning to subsystem AGENTS.md
   - **6+ occurrences:** Add to preflight checks
   - **11+ occurrences:** Automate in preflight + critical section in implementer.md

5. **Store metrics:**
   ```bash
   cat > .claude/metrics/review-<ticket-id>.json <<EOF
   {
     "ticket_id": "<ticket-id>",
     "date": "$(date -I)",
     "subsystem": "<subsystem>",
     "verdict": "$(grep "Verdict:" .claude/work/<ticket-id>/review.md | cut -d: -f2 | tr -d ' ')",
     "patterns_found": $(cat .claude/work/<ticket-id>/pattern-frequency.txt | jq -R -s -c 'split("\n") | map(select(length > 0))'),
     "retry_counts": {
       "implementer": <actual-count>,
       "preflight": <actual-count>,
       "reviewer": <actual-count>
     }
   }
   EOF
   ```

**Outcome:** Pattern library updated, system learns from this review.

### 9. Completion

**Success criteria met:**
- ✅ All tests pass
- ✅ Preflight: PASS or PASS WITH WARNINGS
- ✅ Review: APPROVE or APPROVE WITH CHANGES
- ✅ Patterns extracted and tracked

**Final steps:**

```bash
# Create implementation summary
cat > .claude/work/<ticket-id>/summary.md <<EOF
# Implementation Summary: <ticket-id>

## Status: COMPLETE

## Artifacts:
- Plan: plan.md
- Preflight: preflight-report.md
- Review: review.md
- Pattern frequency: pattern-frequency.txt
- Metrics: ../metrics/review-<ticket-id>.json

## Changed Files:
$(git diff --name-only origin/main)

## Review Verdict:
$(grep "Verdict:" .claude/work/<ticket-id>/review.md)

## Patterns Tracked:
$(cat .claude/work/<ticket-id>/pattern-frequency.txt)

## Commits:
$(git log --oneline origin/main..HEAD)

## Learning:
$(grep -A 1 "Patterns for Future" .claude/work/<ticket-id>/review.md || echo "No new patterns documented")
EOF

# Push branch
git push -u origin HEAD

# Create PR
gh pr create --base main --fill

# Get PR number
PR_NUM=$(gh pr view --json number -q .number)

# Update ticket with PR link
bd update <ticket-id> --notes="PR #$PR_NUM: $(gh pr view --json url -q .url). Artifacts: .claude/work/<ticket-id>/"

# Inform user: ready to merge or wait for CI
echo "✓ Implementation complete!"
echo "✓ PR created: $(gh pr view --json url -q .url)"
echo ""
echo "📊 Artifacts in .claude/work/<ticket-id>/"
echo "   - Plan, preflight report, code review"
echo "   - Pattern frequency tracking"
echo "   - Metrics for analysis"
echo ""
echo "🔄 Feedback loop complete:"
cat .claude/work/<ticket-id>/pattern-frequency.txt
echo ""
echo "Next steps:"
echo "  - Review PR in GitHub"
echo "  - Wait for CI checks"
echo "  - Merge with: gh pr merge $PR_NUM --squash"
echo "  - Close ticket with: bd close <ticket-id>"
echo ""
echo "📈 Check docs/REVIEW_PATTERNS.md for pattern updates"
```

## Error Handling

### Retry Limits

- **Implementer:** Max 3 attempts to make tests pass
- **Preflight:** Max 2 fix cycles
- **Review:** Max 2 feedback loops

### Escalation

When max retries exceeded:

```markdown
# ⚠️ Workflow Escalation: <ticket-id>

## Phase stuck: <phase-name>

## Issue:
<Description of blocking issue>

## Attempts made:
1. [Attempt 1 details]
2. [Attempt 2 details]
3. [Attempt 3 details]

## Artifacts:
- Plan: .claude/work/<ticket-id>/plan.md
- Latest error: <error message>
- Changed files: <list>

## Recommendation:
<Specific user action needed>

Would you like to:
1. Manually fix the issue
2. Revise the plan
3. Discuss approach
```

## Workflow State Tracking

Store state in `.claude/work/<ticket-id>/state.json`:

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
  "status": "in_progress"
}
```

## Benefits vs Standard /work-ticket

**Standard workflow:**
- Single build agent does everything
- Reactive review (issues caught late)
- Expensive model for all tasks
- Pattern blindness (repeated issues)

**Pipeline workflow:**
- Specialized agents per phase
- Proactive checks (preflight catches issues before review)
- Optimal model per task (50%+ cost reduction)
- Pattern capture (learning system)
- Structured artifacts (clear audit trail)
- Test-first discipline (clearer requirements)

## Usage Examples

```bash
# Work on a specific ticket
/work-ticket-pipeline reckon-abc

# Let skill find ready ticket
/work-ticket-pipeline

# With explicit subsystem (skip detection)
/work-ticket-pipeline reckon-abc --subsystem=cli
```

## Notes

- **First time?** Review `docs/agents/README.md` to understand the pipeline
- **Artifacts:** All work saved in `.claude/work/<ticket-id>/` for context recovery
- **Iterative:** Each phase validates before proceeding
- **Learning:** Patterns discovered feed back to guides
- **Cost-effective:** Right model for each task (Opus only for planning/review)

## Future Enhancements

- Parallel work on complex tickets (split epic → parallel pipelines)
- Pattern mining (extract review findings → update preflight)
- Metrics tracking (costs, time per phase, common issues)
- Integration with worktrees (currently works in main branch)
