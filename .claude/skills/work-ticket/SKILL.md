---
name: work-ticket
description: Work a beads ticket from claim to PR using a parallel-analysis, TDD implementation pipeline with model stratification and continuous feedback loop.
allowed-tools: Read, Grep, Glob, Edit, Write, Bash, Task, Agent
user-invocable: true
---

# Work Ticket Pipeline

Implements a beads ticket end-to-end: claim → worktree → parallel analysis → TDD loop → simplify → preflight → review → PR ready.

**Replaces:** `.opencode/command/work-ticket.md` and `.opencode/command/work-ticket-pipeline.md`

## Usage

```bash
# Interactive (picks a ready ticket or prompts)
/work-ticket

# Specific ticket
/work-ticket reckon-abc

# Headless (stops before git push for review)
claude -p "/work-ticket reckon-abc" \
  --allowedTools "Read,Grep,Glob,Edit,Write,Bash,Task,Agent"
```

> **Headless note:** The pipeline always pauses at the PR gate and prints a summary before pushing. In headless mode this is the natural exit point — review the summary, then `git push && gh pr create` manually if satisfied.

---

## Pipeline Overview

```
Phase 0: Claim ticket + worktree setup
Phase 1: ┌─ Codebase analysis agent ─┐  (PARALLEL)
         └─ AC extraction agent      ─┘
Phase 2: Planning (Opus 4.6) — consumes Phase 1 outputs
Phase 3: Test writing (Sonnet 4.6) — failing tests, TDD red state
Phase 4: TDD implementation loop (Sonnet 4.6) — max 10 iterations, stall detection
Phase 5: Simplify (Sonnet 4.6) — cut comment verbosity + complexity, best-effort
Phase 6: Preflight (Haiku 4.5) — go fmt, go vet, go test, pattern checks
Phase 7: Code review (Opus 4.6) — max 2 feedback loops
Phase 8: Feedback loop — pattern extraction, guide updates
Phase 9: Dry-run gate — summary + PR creation (user approves push)
```

### Model Stratification

| Phase | Model | Reason |
|-------|-------|--------|
| Analysis (parallel) | Sonnet 4.6 | Fast codebase reading |
| Planning | Opus 4.6 | Architecture decisions |
| Test writing | Sonnet 4.6 | Code generation |
| Implementation | Sonnet 4.6 | Code generation |
| Simplify | Sonnet 4.6 | Quality-only cleanup, no bug hunt |
| Preflight | Haiku 4.5 | Mechanical checks, 10-20x cheaper |
| Code review | Opus 4.6 | Subtle bug detection |

### Sub-agent Output Style

Sub-agents inherit none of the parent session's style hooks (caveman etc.) —
the prompt the orchestrator composes is the only style channel. Include this
block in EVERY sub-agent prompt (analysis, AC, planner, test-writer,
implementer, simplify, preflight, reviewer):

> **Output style.**
> - State decisions and facts, not your exploration journey ("I looked at X,
>   then Y" is noise; the conclusion plus a `file:line` pointer is signal).
> - Say each fact once, in the section where it's load-bearing; reference it
>   elsewhere instead of restating.
> - Point (`file:line`) instead of pasting code; quote only when the exact
>   wording is the finding (error strings, spec sentences under
>   interpretation).
> - Tables/lists for enumerable facts (fields, signatures, test names);
>   prose only for reasoning. One-line given/when/then.
> - Include only what the ticket/repo/upstream artifact doesn't already say.
> - Cut any paragraph whose removal wouldn't change what the next agent
>   does.
> - Mark uncertainty with `[INFERRED]`/`[OPEN]` tags, not hedging prose.
> - These docs usually fit in ~150-250 lines; justified overruns are fine,
>   but if you're far over you're probably narrating or duplicating.

For the implementer (and any agent writing Go), additionally:

> **Comment style.** Match the surrounding file's comment density. Comments
> state constraints and invariants the code can't show — never provenance:
> no plan.md decision labels, review-issue numbers, ticket IDs, phase names,
> or model names in code comments. That context lives in the pipeline
> artifacts and the PR, not the source.

(`ticket-work/**` is marked `linguist-generated` in `.gitattributes`, so
GitHub collapses pipeline artifacts in PR diffs — length there costs less
than it looks, but the rules above still apply: downstream agents read
these docs in full.)

---

## Phase 0: Claim Ticket + Worktree Setup

```bash
# Find ticket (use provided ID or show ready list)
bd ready
bd show <ticket-id>

# Handle a stale worktree left by a prior aborted run
if [ -d .worktrees/<ticket-id> ]; then
  git -C .worktrees/<ticket-id> log main..HEAD --oneline
  # Empty or clearly abandoned -> clean it up. Real unmerged work -> STOP and
  # escalate to the user; do not delete it silently.
  git worktree remove .worktrees/<ticket-id> --force
  git branch -D <ticket-id> 2>/dev/null || true
fi

# Always branch from an up-to-date origin/main, never the main checkout's
# current HEAD (which may be stale or mid-rebase).
git fetch origin
git worktree add .worktrees/<ticket-id> -b <ticket-id> origin/main

# Claim only AFTER the worktree exists. If `git worktree add` fails even
# after the stale-worktree cleanup above, do NOT claim -- report the error
# and stop.
bd update <ticket-id> --claim

# Initialize work directory INSIDE the worktree (git-tracked, travels with
# the branch/PR -- see Phase 2 for why this replaced .claude/work/).
mkdir -p .worktrees/<ticket-id>/ticket-work/<ticket-id>

# Determine subsystem from ticket description
# Options: cli, tui, journal, storage
# Ask user if ambiguous
```

**Validation:** Worktree exists at `.worktrees/<ticket-id>`, branched from `origin/main`, branch checked out cleanly, ticket shows `in_progress` in `bd show`.

All subsequent work happens inside the worktree directory. Artifacts live at `ticket-work/<ticket-id>/` relative to the worktree root (a plain, non-dot, git-tracked directory -- `.claude/` is gitignored and cannot hold anything that needs to be committed).

---

## Phase 1: Parallel Analysis (PARALLEL)

Spawn **two sub-agents simultaneously** using the Task tool:

### Agent A: Codebase Analysis
```
Task: Read the ticket for <ticket-id>. Then explore the codebase to:
1. Find files most likely to be modified (check subsystem: cli/tui/journal/storage)
2. Identify existing patterns to follow (similar features already implemented)
3. List interfaces, types, and function signatures that will be touched
4. Note any known pitfalls from docs/REVIEW_PATTERNS.md

Output: ticket-work/<ticket-id>/codebase-analysis.md
```

### Agent B: Acceptance Criteria Extraction
```
Task: Read the ticket for <ticket-id>. Extract and formalize:
1. Explicit acceptance criteria (numbered list)
2. Implicit requirements (inferred from description)
3. Edge cases to handle
4. Test scenarios (given/when/then format)
5. What is explicitly OUT of scope

Output: ticket-work/<ticket-id>/acceptance-criteria.md
```

**Wait for both agents.** Proceed only when both outputs exist and are non-empty.

---

## Phase 2: Planning (Opus 4.6)

Use Task tool with `subagent_type="Plan"`. The Plan subagent type has no Write
tool (by design -- a planner shouldn't be editing code), so it cannot produce
`plan.md` itself. Have it return the plan as its final message text; the
**orchestrator** (this skill has `Write` in its own `allowed-tools`) writes
that text to disk.

```
Context: Read:
- bd show <ticket-id>
- ticket-work/<ticket-id>/codebase-analysis.md
- ticket-work/<ticket-id>/acceptance-criteria.md
- docs/agents/planner.md

Produce a plan covering:
- Summary of approach
- Files to modify (with reasons)
- Design decisions (with alternatives considered)
- Test scenarios (directly from AC)
- Known risks or ambiguities

Return the full plan as your final message text -- you do not have a Write
tool, so you cannot save it to disk yourself.
```

**After the subagent returns:** orchestrator writes the returned plan text to `ticket-work/<ticket-id>/plan.md`.

**Validation:**
- `ticket-work/<ticket-id>/plan.md` exists and is non-empty
- Plan has all required sections
- No placeholder TODOs
- No design flaws obvious from context

**On failure:** Re-run planner with specific feedback. Max 2 attempts, then escalate. (This retry budget is for plan-quality failures -- writing the file is the orchestrator's job, not a retryable planner failure.)

**Commit:**
```bash
cd .worktrees/<ticket-id>
git add ticket-work/<ticket-id>/
git commit -m "docs: Add plan and analysis for <ticket-id>"
```

---

## Phase 3: Test Writing (Sonnet 4.6)

Use Task tool with `subagent_type="general-purpose"`, model=sonnet:

```
Context: Read:
- ticket-work/<ticket-id>/plan.md
- ticket-work/<ticket-id>/acceptance-criteria.md
- docs/TESTING.md
- internal/<subsystem>/AGENTS.md (test examples section)

Write comprehensive tests BEFORE any implementation:
- Unit tests for each acceptance criterion
- Integration tests for end-to-end scenarios
- Edge cases identified in AC extraction
- Tests MUST compile but MUST FAIL (feature not implemented yet)

Reference existing test files in the same package for style.
Do NOT create new documentation files. Do NOT modify non-test files.

Report the full list of new test function names you wrote in your final
message (e.g. TestFoo, TestBar) -- the orchestrator needs these by name for
the red-state gate below.
```

**Validation:** `go build ./...` never compiles `_test.go` files, so it proves nothing about
the tests just written. Force compilation of the test files directly, then
check for the specific named tests failing -- not just a nonzero exit code
(which looks identical whether the new tests correctly fail, a test file
doesn't compile, a pre-existing test flaked, or an unrelated package broke):

```bash
cd .worktrees/<ticket-id>

# 0 tests executed, but this forces full compilation of every _test.go file
# -- catches a bad import/syntax/undefined-symbol test file distinctly from
# a legitimately-failing one.
go test -run '^$' ./...

# Now check the specific new tests actually fail, by name.
go test ./... -run '<NewTest1|NewTest2|...>' -v 2>&1 | tee /tmp/red-state-<ticket-id>.txt
grep -E '^--- FAIL: (NewTest1|NewTest2|...)' /tmp/red-state-<ticket-id>.txt
# Every test name reported by the test-writer above must appear here.
```

Record the new test names in `state.json` (`new_test_names`) so Phase 4 can check them by name too.

**On compilation failure (the `-run '^$'` step fails):** Fix test compilation errors and retry. Max 2 attempts.

**On a named test missing from the FAIL output:** the test doesn't actually exercise unimplemented behavior (e.g. it was written to trivially pass, or targets the wrong symbol) -- treat this the same as a compilation failure: fix and retry, same 2-attempt budget.

**Commit:**
```bash
git add <test-files>
git commit -m "test: Write failing tests for <ticket-id>"
```

---

## Phase 4: TDD Implementation Loop (Sonnet 4.6)

Use Task tool with `subagent_type="general-purpose"`, model=sonnet.

**Max iterations: 10. Stall detection: 3 consecutive iterations with identical test output → escalate.**

```
Context: Read:
- ticket-work/<ticket-id>/plan.md
- All test files written in Phase 3
- internal/<subsystem>/AGENTS.md (implementation patterns)
- docs/REVIEW_PATTERNS.md (common pitfalls to avoid)
- Relevant existing implementation files (from codebase analysis)

Implement the feature to make tests pass:
1. Read the failing tests carefully — they define the required interface
2. Implement the minimal code to make tests pass (no gold-plating)
3. Run go test ./... after each logical chunk
4. Commit incremental progress with clear messages
5. Do NOT modify test files
6. Follow patterns from similar existing implementations
```

**After each attempt:** `go test` output embeds per-test/per-package timings
(`--- FAIL: TestX (0.01s)`, `ok pkg 0.42s`), so two consecutive runs are
almost never byte-identical even when genuinely stuck. Normalize before
comparing or hashing:
```bash
cd .worktrees/<ticket-id>

normalize_test_output() {
  sed -E 's/\([0-9]+\.[0-9]+s\)//g; s/[0-9]+(\.[0-9]+)?s$//g' | sort
}

go test ./... 2>&1 | normalize_test_output > /tmp/test-output-<attempt>.txt

# Compare to previous attempt for stall detection
diff /tmp/test-output-<prev>.txt /tmp/test-output-<attempt>.txt
```
Apply the same `normalize_test_output` before computing `state.json`'s `last_test_output_hash`.

**Stall detection:** If 3 consecutive normalized `go test` outputs are identical (same failures, same errors), the implementer is stuck. Escalate with the stuck output rather than continuing to loop.

**Success criterion:** the tests named in `state.json`'s `new_test_names` (from Phase 3) show `--- PASS:` lines, not just a bare exit code:
```bash
go test ./... -run '<NewTest1|NewTest2|...>' -v 2>&1 | grep -E '^--- PASS: (NewTest1|NewTest2|...)'
# Every named test must appear here. Then confirm no regressions elsewhere:
go test ./...          # exits 0
```
A bare exit-0 across the whole module is not sufficient on its own -- it's satisfied equally by an implementer who deleted the failing assertion instead of implementing the feature.

**Commit on success:**
```bash
git add <implementation-files>
git commit -m "feat: Implement <feature> for <ticket-id>"
```

**On max retries exceeded:** Escalate (see Error Handling).

---

## Phase 5: Simplify (Sonnet 4.6)

Best-effort quality pass on the Phase 4 diff before Preflight/Review see it —
review should judge the shape the code ships in, not the shape the TDD loop
happened to leave it in mid-iteration.

Use Task tool with `subagent_type="general-purpose"`, model=sonnet:

```
Context: Read:
- ticket-work/<ticket-id>/plan.md
- git diff origin/main (the implementation just written in Phase 4)
- internal/<subsystem>/AGENTS.md

Review the changed code (not the whole codebase) for:
- Reuse: duplicated logic that should call an existing helper, or a new
  helper worth extracting when the same shape appears 3+ times.
- Simplification: unnecessary abstraction, dead branches, over-parameterized
  functions for a single call site, premature interfaces.
- Efficiency: obviously wasteful allocations/loops introduced by the TDD
  loop's incremental patches -- not micro-optimization, only what's cheap
  and clearly better.
- Comment verbosity: comments restating what the code already says, stale
  comments left over from earlier iterations, comments narrating the TDD
  process itself. Delete these -- they're exploration residue, not
  documentation.
- Architecture altitude: did an iteration bolt something onto the wrong
  file/package under time pressure; does it belong one layer up or down.

Apply fixes directly. Quality only -- do NOT hunt for correctness bugs (Phase
7/Review's job) and do NOT change test-observable behavior. Do NOT modify
test files.

After every edit, run:
  go build ./... && go test ./...
If a simplification changes test output, revert that specific change --
this phase must be a no-op on behavior.
```

**Validation:** `go test ./...` exits 0 with the same set of passing tests as
immediately after Phase 4 (no regressions, no accidental deletions).

**Commit (only if something changed):**
```bash
cd .worktrees/<ticket-id>
if ! git diff --quiet; then
  git add -A
  git commit -m "refactor: Simplify implementation for <ticket-id>"
fi
```

**On test regression:** Revert the offending change, confirm back to green,
and proceed without it. This phase is optional polish, not gated -- do not
retry from scratch, do not escalate, do not block the pipeline on it.

---

## Phase 6: Preflight (Haiku 4.5)

Use Task tool with `subagent_type="general-purpose"`, model=haiku:

```
Context: Read docs/agents/preflight.md for the full checklist.

Run all checks and produce ticket-work/<ticket-id>/preflight-report.md.
```

**Mechanical checks:**
```bash
cd .worktrees/<ticket-id>

# Format (auto-fix -- go fmt itself is the only auto-fix Haiku performs, see below)
go fmt ./...
if ! git diff --quiet; then
  git add -A && git commit -m "fix: go fmt for <ticket-id>"
fi

# Vet
go vet ./...

# All tests
go test ./...

# Coverage
go test -cover ./...
```
(The old `git diff --exit-code || git add -A && git commit ...` one-liner is
a bash precedence trap: `A || B && C` is left-associative, i.e. `(A || B) &&
C`. On a clean diff, `git diff --exit-code` succeeds, short-circuiting `B`,
but `C` still runs -- `git commit` fires with nothing staged and errors. The
`if`-guarded form above only commits when `go fmt` actually changed something.)

**Manual pattern checks** (per `docs/agents/preflight.md`):
- All errors wrapped with context
- Resource cleanup with defer
- CLI: respect --quiet flag
- TUI: no variable capture in closures

**Verdict:** PASS / PASS WITH WARNINGS / FAIL

**On FAIL:** Report findings only in `preflight-report.md` -- do not write
non-fmt code changes. Inserting a missing `defer` (or any other manual-check
fix) isn't mechanical: correct placement requires understanding control flow
and resource lifetime, and Haiku is the least-capable model in the pipeline.
`go fmt` is the only auto-applied fix (it's a deterministic tool, not
codegen). Anything else routes back to Phase 4 (Sonnet, single iteration),
matching `docs/agents/preflight.md`'s existing Handoff section. Re-run
Simplify (Phase 5) then Preflight after the fix. Max 2 fix cycles, then
escalate.

---

## Phase 7: Code Review (Opus 4.6)

Use Task tool with `subagent_type="code-reviewer"`, model=opus:

```
Context: Read:
- ticket-work/<ticket-id>/plan.md
- ticket-work/<ticket-id>/preflight-report.md
- All changed files (git diff origin/main)
- internal/<subsystem>/AGENTS.md
- docs/REVIEW_PATTERNS.md

Review across 7 dimensions: correctness, architecture, testing, maintainability,
error handling, performance, security.

Output: ticket-work/<ticket-id>/review.md
Verdict must be one of: APPROVE / APPROVE WITH CHANGES / REQUEST CHANGES
```

**On REQUEST CHANGES:**
1. Extract required changes from review.md
2. Return to implementation (Phase 4 — single iteration, not full loop)
3. Re-run Simplify (Phase 5) + Preflight (Phase 6) + Review
4. Max 2 feedback loops, then escalate

**On APPROVE or APPROVE WITH CHANGES:** Proceed to Phase 8.

---

## Phase 8: Feedback Loop

Extract patterns from this review to improve future work.

**Pattern extraction:** a bare `grep -irl "$pattern" review.md` false-positives
on negations -- "Checked for unwrapped error handling — none found" still
matches `grep "unwrapped error"`. Scope the search to numbered `### Issues`
entries only (same convention `docs/agents/feedback-loop.md`'s
`extract_patterns.sh` already uses) -- those entries exist *only* when the
reviewer found a real problem, so a "checked, none found" sentence written
as prose can never live there:

```bash
# Check frequency of patterns found in this review.  The grep anchor
# (^[0-9]+\. \*\*\[severity\]) already restricts to numbered finding
# entries, so no section-range sed is needed — the reviewer output uses
# per-dimension headers (## Correctness, ## Architecture, ...) with no
# consistent pair of section boundaries to scope between.
for pattern in "unwrapped error" "missing defer" "closure capture" "nil check" "missing validation"; do
  count=$(
    for f in ticket-work/*/review.md; do
      grep -iE "^[0-9]+\. \*\*\[(Critical|Major|Minor)\].*${pattern}" "$f" \
        && echo "$f"
    done 2>/dev/null | sort -u | wc -l
  )
  echo "$pattern: $count" >> ticket-work/<ticket-id>/pattern-frequency.txt
done
```

This fixed 5-string vocabulary is a curated list, not a general-purpose
pattern miner -- a novel recurring issue never enters this loop no matter how
often it recurs. That's a known, accepted scope limit for this mechanism, not
a bug to fix here.

**Note (reckon-g96t, fixed):** the pattern scan above previously scoped
its grep through `sed -n '/## Functional Review/,/## Design Review/p'`,
which never matched the reviewer's actual output format.  That sed range
has been removed — the `^[0-9]+\. \*\*[severity]` anchor alone is
sufficient to restrict to numbered finding entries, and it works against
both the old `## Functional Review` convention and the current
per-dimension (`## Correctness`, `## Architecture`, ...) convention.
Verified against all existing `ticket-work/*/review.md` files and a
synthetic review containing known findings for each tracked pattern.

**Update actions (based on frequency thresholds):**
- 1-2 occurrences: Document in `docs/REVIEW_PATTERNS.md`
- 3-5 occurrences: Add to `internal/<subsystem>/AGENTS.md` Common Pitfalls
- 6-10 occurrences: Add to `docs/agents/preflight.md` manual checks
- 11+: Automate in preflight + add to `docs/agents/implementer.md` Critical section

**Idempotency:** crossing a threshold must trigger its doc-append at most
once, not on every subsequent ticket that also happens to hit the same
pattern. There's no separate ledger to go stale -- the target guide itself is
the source of truth:
```bash
if ! grep -qi "$pattern" "$TARGET_GUIDE"; then
  # append the pattern section to $TARGET_GUIDE
fi
```

**Constraint:** Only update EXISTING files. Do not create new markdown files at the repo root or anywhere else.

**Store metrics:**
```bash
mkdir -p .claude/metrics
cat > .claude/metrics/review-<ticket-id>.json << EOF
{
  "ticket_id": "<ticket-id>",
  "date": "$(date -I)",
  "subsystem": "<subsystem>",
  "verdict": "<APPROVE|APPROVE_WITH_CHANGES>",
  "retry_counts": {
    "implementer": <n>,
    "preflight": <n>,
    "reviewer": <n>
  }
}
EOF
```

---

## Phase 9: Dry-Run Gate (STOP BEFORE PUSH)

**Always pause here — even in headless mode.**

Create summary and present to user before any git push:

```bash
cat > ticket-work/<ticket-id>/summary.md << EOF
# Implementation Summary: <ticket-id>

## Status: READY FOR PUSH

## Review Verdict: <verdict>

## Changed Files:
$(cd .worktrees/<ticket-id> && git diff --name-only origin/main)

## Commits:
$(cd .worktrees/<ticket-id> && git log --oneline origin/main..HEAD)

## Test Results:
$(cd .worktrees/<ticket-id> && go test ./... 2>&1 | tail -5)

## Preflight: $(grep "Status:" ticket-work/<ticket-id>/preflight-report.md | head -1)
## Review: $(grep "Verdict:" ticket-work/<ticket-id>/review.md | head -1)

## Pattern Frequency:
$(cat ticket-work/<ticket-id>/pattern-frequency.txt)
EOF
```

**Print to user:**
```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  Pipeline complete for <ticket-id>
  Review: <verdict>
  Tests: PASS
  Preflight: <status>
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  Artifacts: ticket-work/<ticket-id>/
  Branch:    <ticket-id> (worktree: .worktrees/<ticket-id>)

  To push and create PR:
    cd .worktrees/<ticket-id>
    git push -u origin HEAD
    gh pr create --base main --title "..." --body "..."   # see below, NOT --fill

  Or to push now, type: yes
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

**If user confirms push:**

`gh pr create --fill` derives the title/body from git commit metadata. On any
branch with more than one commit (every ticket branch, since Phase 2/3/4/5/6
each commit separately), `--fill` does NOT synthesize a summary -- it sets the
title to the branch name (dashes turned to spaces, e.g. "reckon qiua") and the
body to a bare bullet list of every commit subject line. That is a real,
recurring defect observed in practice (reckon-qiua/PR #148 shipped exactly this
useless title+body), not a hypothetical -- never use `--fill` here. Build the
title/body explicitly from ticket + pipeline artifacts instead:

```bash
cd .worktrees/<ticket-id>

TICKET_TITLE=$(bd show <ticket-id> --json 2>/dev/null | jq -r '.[0].title // empty')
[ -z "$TICKET_TITLE" ] && TICKET_TITLE="<ticket-id>"

git push -u origin HEAD
gh pr create --base main \
  --title "$TICKET_TITLE (<ticket-id>)" \
  --body "$(cat <<EOF
## Summary

$(sed -n '/^## Summary/,/^## Files to modify/p' ticket-work/<ticket-id>/plan.md | sed '$d')

## Process

Plan → TDD-red tests → implementation → preflight → code review pipeline.
Review verdict: $(grep -m1 '^\*\*Verdict:' ticket-work/<ticket-id>/review.md || grep -m1 '^Verdict:' ticket-work/<ticket-id>/review.md)
Artifacts: ticket-work/<ticket-id>/ (plan.md, acceptance-criteria.md, review.md)

## Test plan

- [x] $(go test ./... 2>&1 | tail -1)
- [x] Preflight: $(grep -m1 -i 'status:' ticket-work/<ticket-id>/preflight-report.md)

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"

PR_NUM=$(gh pr view --json number -q .number)
bd update <ticket-id> --notes="PR #$PR_NUM: $(gh pr view --json url -q .url)"

echo "PR created. When merged:"
echo "  gh pr merge $PR_NUM --squash"
echo "  git worktree remove .worktrees/<ticket-id>"
echo "  git branch -D <ticket-id>"
echo "  bd close <ticket-id>"
```

---

## Error Handling & Escalation

### Retry Limits

| Phase | Max Retries | On Exceed |
|-------|-------------|-----------|
| Planner | 2 | Escalate to user |
| Test writer | 2 | Escalate to user |
| Implementer | 10 iterations | Escalate to user |
| Stall detection | 3 identical outputs | Escalate immediately |
| Simplify | N/A (best-effort) | Skip; never escalate, never blocks |
| Preflight | 2 fix cycles | Escalate to user |
| Reviewer | 2 feedback loops | Escalate to user |

### Escalation Format

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  ⚠ Pipeline escalation: <ticket-id>
  Stuck phase: <phase>
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Issue: <description of what's blocking>

Attempts made:
  1. <what was tried>
  2. <what was tried>

Artifacts:
  Plan:   ticket-work/<ticket-id>/plan.md
  Latest: <most recent error or output>

Recommendation: <specific action needed>

Options:
  1. Fix the issue and resume from <phase>
  2. Revise the plan
  3. Discuss approach
```

---

## State Tracking

Store in `ticket-work/<ticket-id>/state.json` (update after each phase):

```json
{
  "ticket_id": "<ticket-id>",
  "phase": "implementer",
  "worktree": ".worktrees/<ticket-id>",
  "subsystem": "cli",
  "attempts": {
    "planner": 1,
    "test_writer": 1,
    "implementer": 3,
    "simplify": 1,
    "preflight": 0,
    "reviewer": 0
  },
  "new_test_names": ["TestFoo", "TestBar"],
  "stall_count": 0,
  "last_test_output_hash": "<sha256 of normalized go test output (timings stripped, sorted)>",
  "status": "in_progress"
}
```

**Context recovery:** If a session is interrupted, read `state.json` to resume from the correct phase rather than starting over.

---

## See Also

- `docs/agents/planner.md` — Planner agent spec
- `docs/agents/test-writer.md` — Test writer spec
- `docs/agents/implementer.md` — Implementer spec
- `docs/agents/preflight.md` — Preflight checklist
- `docs/agents/reviewer.md` — Reviewer spec
- `docs/agents/feedback-loop.md` — Pattern extraction process
- `docs/REVIEW_PATTERNS.md` — Known anti-patterns with frequency tracking
- `docs/TESTING.md` — Test conventions
