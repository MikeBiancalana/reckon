# Agent Workflow Pipeline

**Purpose:** Structured workflow for implementing beads tickets using specialized agents with optimal model stratification.

## Why Specialized Agents?

**Problems solved:**
- Pattern blindness (repeated similar issues)
- Reactive code review (catching issues after implementation)
- Missing test-first discipline
- Suboptimal model usage (expensive models for mechanical checks)
- No feedback loop from review findings to future work

**Solution:**
Divide the workflow into specialized phases, each using the optimal model for that task.

## The Pipeline

```
1. Planner (Opus 4.6)
   ├─ Reads: ticket, AGENTS.md, subsystem guides
   ├─ Output: .claude/work/<ticket-id>/plan.md
   └─ Handoff: test-writer

2. Test Writer (Sonnet 4.5)
   ├─ Reads: plan, TESTING.md, subsystem test patterns
   ├─ Output: Test files (failing tests)
   └─ Handoff: implementer

3. Implementer (Sonnet 4.5)
   ├─ Reads: plan, tests, subsystem guides, examples
   ├─ Output: Implementation (make tests pass)
   └─ Handoff: preflight

4. Preflight (Haiku 4.5)
   ├─ Runs: go fmt, go vet, go test, pattern checks
   ├─ Output: .claude/work/<ticket-id>/preflight-report.md
   └─ Handoff: reviewer (if PASS) | implementer (if FAIL)

5. Reviewer (Opus 4.6)
   ├─ Reads: plan, preflight report, code, subsystem guides
   ├─ Output: .claude/work/<ticket-id>/review.md
   └─ Decision: APPROVE | APPROVE WITH CHANGES | REQUEST CHANGES
```

## Model Stratification

| Agent | Model | Why |
|-------|-------|-----|
| Planner | Opus 4.6 | Deep thinking, architecture decisions |
| Test Writer | Sonnet 4.5 | Code generation, fast, cost-effective |
| Implementer | Sonnet 4.5 | Code generation, pattern following |
| Preflight | Haiku 4.5 | Mechanical checks, 10-20x cheaper, fast |
| Reviewer | Opus 4.6 | Subtle bug detection, architecture critique |

**Cost optimization:**
- Use Opus only where deep reasoning is critical (planning, review)
- Use Haiku for mechanical pattern matching (preflight)
- Use Sonnet for bulk code generation (tests, implementation)

## Workflow Artifacts

All work for a ticket goes in `.claude/work/<ticket-id>/`:

```
.claude/work/reckon-abc/
├── plan.md                 # Design decisions, approach, test scenarios
├── preflight-report.md     # Automated + manual check results
└── review.md               # Deep code review with verdict
```

## Usage

### Manual Invocation

Call agents sequentially for a ticket:

```bash
# 1. Plan
claude-code --agent planner --input '{"ticket_id": "reckon-abc", "subsystem": "cli"}'

# 2. Write tests
claude-code --agent test-writer --input '{"ticket_id": "reckon-abc", "plan_path": ".claude/work/reckon-abc/plan.md"}'

# 3. Implement
claude-code --agent implementer --input '{"ticket_id": "reckon-abc", "plan_path": ".claude/work/reckon-abc/plan.md"}'

# 4. Preflight check
claude-code --agent preflight --input '{"ticket_id": "reckon-abc", "changed_files": ["file1.go", "file2_test.go"]}'

# 5. Review
claude-code --agent reviewer --input '{"ticket_id": "reckon-abc", "changed_files": ["file1.go", "file2_test.go"]}'
```

### Orchestrated (Future)

```bash
# Single command runs entire pipeline
claude-code --workflow ticket --input '{"ticket_id": "reckon-abc"}'
```

## Agent Specifications

See individual agent files for detailed specs:

- [planner.md](./planner.md) - Architecture & design
- [test-writer.md](./test-writer.md) - Test-first development
- [implementer.md](./implementer.md) - Implementation
- [preflight.md](./preflight.md) - Quality gates
- [reviewer.md](./reviewer.md) - Code review

## Success Metrics

**Before (manual workflow):**
- Repeated error handling issues
- Integration problems from isolated worktrees
- Reactive code review catching preventable issues
- Inconsistent test coverage
- Manual quality checks

**After (agent pipeline):**
- Pattern capture in preflight/reviewer → fewer repeats
- Test-first discipline → clearer requirements
- Preflight catches mechanical issues → reviewer focuses on architecture
- Model stratification → 50%+ cost reduction
- Documented artifacts → context for future work

## Feedback Loop

**Critical feature:** Patterns discovered during review feed back to:
- Preflight checks (add new mechanical patterns)
- Subsystem AGENTS.md guides (update patterns section)
- docs/REVIEW_PATTERNS.md (capture recurring issues)

This creates a learning system that gets better over time.

## Future Enhancements

1. **Orchestrator agent:** Chain agents automatically
2. **Pattern library:** docs/REVIEW_PATTERNS.md with known good/bad patterns
3. **Metrics:** Track pattern recurrence, review issues by category
4. **Skill integration:** `/work-ticket` command runs full pipeline
5. **Parallel planning:** For complex tickets, plan → split → parallel implementation
