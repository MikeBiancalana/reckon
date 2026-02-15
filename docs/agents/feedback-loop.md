# Feedback Loop Process

**Purpose:** Close the loop from code review findings back to implementation guides and preflight checks.

This document describes **how** to extract patterns from reviews and feed them back into the system for continuous improvement.

## Overview

The feedback loop is what makes this workflow a **learning system** rather than just a process:

```
Review Findings → Pattern Extraction → Frequency Tracking → Guide Updates → Preflight Automation → Fewer Issues
```

Without the feedback loop, we'll keep catching the same issues in review. With it, common issues get caught earlier (preflight) or prevented entirely (better guides).

## When to Run the Feedback Loop

**After each review:** Quick check (5 minutes)
- Did this review find any new patterns?
- Did it find patterns we've seen before?
- Update frequency counts

**Weekly:** Pattern mining (30 minutes)
- Review all reviews from the past week
- Extract common themes
- Update pattern library
- Identify candidates for guide updates

**Monthly:** System updates (2 hours)
- Add high-frequency patterns to guides
- Create new preflight checks
- Update metrics
- Analyze trends

## Step-by-Step Process

### 1. Parse Review Findings

After a code review completes, read `.claude/work/<ticket-id>/review.md`.

**Extract structured data:**

```bash
# Create pattern extraction helper
cat > /tmp/extract_patterns.sh <<'EOF'
#!/bin/bash
REVIEW_FILE="$1"
TICKET_ID=$(basename $(dirname "$REVIEW_FILE"))

# Extract issues section
sed -n '/## Functional Review/,/## Design Review/p' "$REVIEW_FILE" | \
  grep -E "^\d+\. \*\*\[Critical|Major|Minor\]" | \
  while read line; do
    severity=$(echo "$line" | grep -oP '\[\K(Critical|Major|Minor)')
    description=$(echo "$line" | sed 's/^.*\] //')
    echo "$TICKET_ID,$severity,$description"
  done
EOF
chmod +x /tmp/extract_patterns.sh

# Run on review file
/tmp/extract_patterns.sh .claude/work/reckon-abc/review.md
```

**Output example:**
```
reckon-abc,Major,Missing error context in journal parsing
reckon-abc,Critical,Resource leak - file not closed
reckon-abc,Minor,Variable name could be clearer
```

### 2. Categorize Issues

Map each issue to a category:

| Category | Keywords | Target Guide |
|----------|----------|--------------|
| Error Handling | "error", "wrap", "context" | All AGENTS.md + implementer.md |
| Resource Management | "close", "defer", "leak" | implementer.md + subsystem guides |
| TUI Patterns | "closure", "capture", "nil check", "component" | internal/tui/AGENTS.md |
| CLI Patterns | "os.Exit", "quiet flag", "validation" | internal/cli/AGENTS.md |
| Testing | "test", "coverage", "edge case" | TESTING.md + test-writer.md |
| Performance | "inefficient", "N+1", "cache" | subsystem guides |
| Security | "validation", "injection", "sanitize" | All guides |

### 3. Check Frequency

Look for similar issues in past reviews:

```bash
# Find similar issues across all reviews
PATTERN="Missing error context"
grep -r "$PATTERN" .claude/work/*/review.md | wc -l
```

**Update frequency in REVIEW_PATTERNS.md:**
- First occurrence: 🟡 Watch
- 2nd occurrence: 🟡🟡 Monitor
- 3-5 occurrences: 🔴 Common
- 6-10 occurrences: 🔴🔴 Very common
- 11+ occurrences: 🔴🔴🔴 Critical (must address)

### 4. Decide on Action

Based on frequency and severity:

```
┌──────────────────┬─────────────────────────────────────┐
│ Frequency        │ Action                              │
├──────────────────┼─────────────────────────────────────┤
│ 1x               │ Document in REVIEW_PATTERNS.md      │
├──────────────────┼─────────────────────────────────────┤
│ 2x               │ Add to pattern library with example │
├──────────────────┼─────────────────────────────────────┤
│ 3-5x (Common)    │ Add warning to subsystem AGENTS.md  │
├──────────────────┼─────────────────────────────────────┤
│ 6-10x (Very)     │ Add to preflight manual checks      │
├──────────────────┼─────────────────────────────────────┤
│ 11+ (Critical)   │ Automate in preflight + add to      │
│                  │ implementer.md critical section     │
└──────────────────┴─────────────────────────────────────┘
```

**Severity multiplier:**
- Critical issues: Act one level earlier (2x = add to guides)
- Minor issues: Act one level later (need more occurrences)

### 5. Update Pattern Library

Add or update entry in `docs/REVIEW_PATTERNS.md`:

```markdown
#### ❌ Anti-Pattern: [Pattern Name]
**Issue:** [What's wrong]

[Bad code example]

**Fix:**
[Good code example]

**Frequency:** [🟡/🔴/🔴🔴/🔴🔴🔴] ([count] occurrences)

**Tickets:** reckon-abc, reckon-def, reckon-xyz

**Preflight check:** [How to detect mechanically]

**Added to guides:**
- ✅ [guide path] - [section]
- ⚠️ TODO: [pending action]
```

### 6. Update Implementation Guides

When pattern hits 3+ occurrences, add to guides:

#### Update Subsystem AGENTS.md

Example: Adding to `internal/cli/AGENTS.md`

```markdown
## Common Pitfalls

### Missing Error Context (⚠️ Very Common)

**Issue:** Returning errors without wrapping them loses context.

**Bad:**
```go
if err != nil {
    return err  // Where did this error come from?
}
```

**Good:**
```go
if err != nil {
    return fmt.Errorf("failed to load journal for %s: %w", date, err)
}
```

**Why:** When errors bubble up through multiple layers, context helps debugging.

**See:** docs/REVIEW_PATTERNS.md - Error Handling section
```

#### Update Implementer Guide

Example: Adding to `docs/agents/implementer.md`

In the "Critical Patterns" section:

```markdown
### Error Handling (CRITICAL)

**⚠️ PATTERN #1: Always wrap errors** (Found in 15 reviews)

Every error must include context about what operation failed:

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

This pattern has been found in 15+ code reviews. Make it automatic.
```

### 7. Update Preflight Checks

When pattern hits 6+ occurrences, automate detection:

#### Add Manual Check

In `docs/agents/preflight.md`, add to "Manual Checks" section:

```markdown
### Error Handling

**Scan for missing error handling:**
```bash
# Look for naked returns after error-prone calls
grep -n "return err$" <files>
# Flag these for review - should be wrapped with context
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
```
```

#### Add Automated Check (if possible)

For truly mechanical checks, create a simple script:

```bash
# .claude/scripts/check-error-wrapping.sh
#!/bin/bash
FILES="$@"
FOUND=0

for file in $FILES; do
  # Find "return err" that aren't wrapped
  grep -n "return err$" "$file" | while read line; do
    echo "⚠️ Unwrapped error: $file:$line"
    FOUND=$((FOUND + 1))
  done
done

if [ $FOUND -gt 0 ]; then
  echo "Found $FOUND unwrapped errors"
  exit 1
fi
```

Reference in preflight.md:
```bash
# Run error wrapping check
.claude/scripts/check-error-wrapping.sh $(git diff --name-only)
```

### 8. Track Metrics

Store metrics in `.claude/metrics/`:

#### Per-Review Metrics

`.claude/metrics/review-<ticket-id>.json`:
```json
{
  "ticket_id": "reckon-abc",
  "date": "2024-02-15",
  "subsystem": "cli",
  "verdict": "APPROVE_WITH_CHANGES",
  "issues": [
    {
      "category": "error_handling",
      "severity": "major",
      "pattern": "unwrapped_error",
      "count": 3,
      "files": ["cmd/notes.go"]
    },
    {
      "category": "resource_management",
      "severity": "critical",
      "pattern": "missing_defer",
      "count": 1,
      "files": ["internal/journal/parser.go"]
    }
  ],
  "retry_count": 1,
  "phase_durations": {
    "planner": 5,
    "test_writer": 3,
    "implementer": 12,
    "preflight": 1,
    "reviewer": 4
  }
}
```

#### Aggregate Metrics

Update `.claude/metrics/summary.json` weekly:
```json
{
  "period": "2024-02-01 to 2024-02-15",
  "tickets_completed": 8,
  "patterns": {
    "unwrapped_error": {
      "count": 15,
      "trend": "decreasing",
      "last_seen": "reckon-abc",
      "status": "added_to_preflight"
    },
    "missing_defer": {
      "count": 12,
      "trend": "stable",
      "last_seen": "reckon-def",
      "status": "in_guides"
    },
    "closure_capture": {
      "count": 8,
      "trend": "decreasing",
      "last_seen": "reckon-xyz",
      "status": "in_guides_and_preflight"
    }
  },
  "success_metrics": {
    "first_review_approve_rate": 0.62,
    "avg_retry_per_phase": {
      "implementer": 1.3,
      "preflight": 0.6,
      "reviewer": 0.4
    },
    "preflight_catch_rate": 0.75
  }
}
```

### 9. Generate Feedback Report

Create `.claude/feedback/<date>.md`:

```markdown
# Feedback Loop Report: 2024-02-15

## Reviews This Week: 8

## New Patterns Discovered

### 1. Inconsistent date validation
- **Frequency:** 2 occurrences (new)
- **Action:** Monitoring
- **Tickets:** reckon-abc, reckon-def

### 2. Missing transaction rollback
- **Frequency:** 3 occurrences (threshold reached!)
- **Action:** Add to internal/storage/AGENTS.md
- **Tickets:** reckon-ghi, reckon-jkl, reckon-mno

## Patterns Updated

### Unwrapped errors
- **Previous:** 12 occurrences
- **This week:** +3 occurrences = 15 total
- **Status:** Already in preflight + guides
- **Trend:** ⬆️ Still occurring (preflight check not catching all cases)
- **Action:** Strengthen preflight check

### Closure capture bug
- **Previous:** 8 occurrences
- **This week:** 0 occurrences
- **Status:** In guides + preflight
- **Trend:** ✅ Resolved! Pattern eliminated
- **Action:** None needed, guides working

## Guide Updates Made

1. Added "Missing transaction rollback" to internal/storage/AGENTS.md
2. Updated preflight check for unwrapped errors (more comprehensive grep)
3. Added example to implementer.md for error context

## Success Metrics

- **Patterns resolved this week:** 1 (closure capture)
- **New patterns emerging:** 1 (date validation)
- **Preflight catch rate:** 75% (up from 68% last week)
- **First-review approve rate:** 62% (up from 54% last week)

## Recommendations

1. **High priority:** Improve error wrapping check in preflight
2. **Medium priority:** Monitor date validation pattern
3. **Celebrate:** Closure capture bug eliminated! 🎉

## Next Actions

- [ ] Update preflight error wrapping check
- [ ] Add transaction rollback to storage guide
- [ ] Monitor date validation for 3rd occurrence
```

## Automation Opportunities

Create helper scripts:

### Pattern Extraction Script

`.claude/scripts/extract-patterns.sh`:
```bash
#!/bin/bash
# Extract patterns from recent reviews

echo "# Pattern Extraction Report"
echo "Generated: $(date)"
echo ""

# Find all review files
for review in .claude/work/*/review.md; do
  ticket_id=$(basename $(dirname "$review"))

  # Extract issues
  echo "## $ticket_id"
  sed -n '/### Issues/,/## Design Review/p' "$review" | \
    grep -E "^\d+\." | \
    head -5  # Top 5 issues
  echo ""
done

# Pattern frequency
echo "## Pattern Frequency"
for pattern in "unwrapped error" "missing defer" "closure capture" "nil check"; do
  count=$(grep -ir "$pattern" .claude/work/*/review.md | wc -l)
  echo "- $pattern: $count occurrences"
done
```

### Guide Update Helper

`.claude/scripts/update-guide.sh`:
```bash
#!/bin/bash
# Helper to add pattern to guide

GUIDE="$1"
PATTERN_NAME="$2"
PATTERN_FILE="/tmp/pattern.md"

echo "Adding pattern '$PATTERN_NAME' to $GUIDE"
echo ""
echo "Enter pattern markdown (Ctrl-D when done):"
cat > "$PATTERN_FILE"

# Add to Common Pitfalls section
sed -i "/## Common Pitfalls/r $PATTERN_FILE" "$GUIDE"

echo "✓ Pattern added to $GUIDE"
echo "Review changes with: git diff $GUIDE"
```

## Integration with /work-ticket-pipeline

Enhance the skill to include feedback loop:

```markdown
### 8. Post-Review Feedback (New Phase)

After review completes:

1. Extract patterns from review.md
2. Check frequency in REVIEW_PATTERNS.md
3. Update frequency counts
4. If threshold reached (3+ occurrences):
   - Add to relevant guides
   - Add to preflight if 6+ occurrences
5. Store metrics
6. Generate feedback report if weekly threshold
```

## Success Criteria

The feedback loop is working when:

1. **Pattern recurrence decreases**
   - Same issue appears less often over time
   - Track in metrics: pattern frequency trending down

2. **Preflight catch rate increases**
   - More issues caught before review
   - Track in metrics: preflight issues / total issues

3. **First-review approval rate increases**
   - Fewer REQUEST CHANGES, more APPROVE
   - Track in metrics: verdict distribution

4. **Guide quality improves**
   - More real-world examples from actual reviews
   - Patterns section grows with proven issues

5. **System learns**
   - New patterns added to guides automatically
   - Common mistakes become impossible (prevented by preflight)

## Weekly Review Cadence

**Monday morning (15 minutes):**
- Run pattern extraction script
- Review last week's patterns
- Update frequency counts

**Monthly (1 hour):**
- Generate comprehensive metrics report
- Update REVIEW_PATTERNS.md with new high-frequency patterns
- Add patterns to guides (3+ occurrences)
- Create new preflight checks (6+ occurrences)
- Analyze trends

**Quarterly (2 hours):**
- Deep analysis of metrics
- Identify systemic issues
- Major guide updates
- Celebrate patterns eliminated
- Set goals for next quarter

## Tools Needed

Create in `.claude/scripts/`:
- `extract-patterns.sh` - Parse reviews for patterns
- `check-frequency.sh` - Count pattern occurrences
- `update-guide.sh` - Add pattern to guide
- `generate-metrics.sh` - Aggregate metrics
- `feedback-report.sh` - Generate weekly report

## Notes

**This is the most important part of the system.**

Without the feedback loop:
- We'll keep catching the same issues
- Reviews become repetitive
- No improvement over time

With the feedback loop:
- System learns from mistakes
- Common issues get caught earlier
- Eventually, common mistakes become impossible
- Guides stay relevant and practical

**The goal:** Make the most common mistakes impossible through proactive prevention rather than reactive detection.
