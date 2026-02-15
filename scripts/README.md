# Workflow Scripts

Helper scripts for the agent workflow pipeline and feedback loop.

## Feedback Loop Scripts

### extract-review-patterns.sh

Extract patterns from code reviews for tracking and analysis.

**Usage:**
```bash
# Analyze all reviews from past week
./scripts/extract-review-patterns.sh

# Analyze specific ticket
./scripts/extract-review-patterns.sh reckon-abc
```

**Output:**
- Pattern frequency counts
- Action recommendations (when to add to guides/preflight)
- List of affected tickets

**When to run:**
- After completing a code review
- Weekly on Monday morning (team ritual)
- Before updating docs/REVIEW_PATTERNS.md

### generate-feedback-report.sh

Generate comprehensive weekly feedback loop report.

**Usage:**
```bash
./scripts/generate-feedback-report.sh
```

**Output:** `.claude/feedback/YYYY-MM-DD.md` containing:
- Pattern frequency analysis
- Success metrics (approval rates, trends)
- Recommended actions
- Files to update

**When to run:**
- Weekly (Monday morning recommended)
- End of sprint
- Before quarterly planning

## Workflow

### Weekly Feedback Loop Cadence

**Monday morning (15 minutes):**
```bash
# 1. Generate report
./scripts/generate-feedback-report.sh

# 2. Review output
cat .claude/feedback/$(date -I).md

# 3. Take actions based on recommendations
#    - Update REVIEW_PATTERNS.md frequency counts
#    - Add patterns to guides if thresholds met
```

**After each review:**
```bash
# Quick pattern check for just-completed review
./scripts/extract-review-patterns.sh reckon-abc
```

## Pattern Thresholds

| Frequency | Status | Action |
|-----------|--------|--------|
| 0 | ✅ No occurrences | None |
| 1-2 | 🟡 Watch | Document in REVIEW_PATTERNS.md |
| 3-5 | 🔴 Common | Add to subsystem AGENTS.md warnings |
| 6-10 | 🔴🔴 Very common | Add to preflight manual checks |
| 11+ | 🔴🔴🔴 Critical | Automate in preflight |

## Integration

These scripts are used by:
- `/work-ticket-pipeline` skill (Phase 8: Feedback Loop)
- `docs/agents/orchestrator.md` (Phase 7: Feedback Loop)
- Manual workflow review process

## Files Created/Updated

Scripts create and maintain:
- `.claude/metrics/review-<ticket-id>.json` - Per-review metrics
- `.claude/feedback/YYYY-MM-DD.md` - Weekly reports
- `.claude/work/<ticket-id>/pattern-frequency.txt` - Per-ticket pattern counts

Scripts help update:
- `docs/REVIEW_PATTERNS.md` - Pattern library
- `internal/*/AGENTS.md` - Subsystem guides
- `docs/agents/preflight.md` - Quality checks
- `docs/agents/implementer.md` - Critical patterns

## Future Enhancements

Possible additions:
- `update-guide.sh` - Automate adding patterns to guides
- `create-preflight-check.sh` - Generate preflight check code from pattern
- `trend-analysis.sh` - Compare metrics across time periods
- `pattern-miner.sh` - ML-based pattern extraction from review text

## Notes

**Why bash scripts?**
- Simple, portable, easy to understand
- Can be run manually or integrated into workflows
- No dependencies beyond standard Unix tools
- Easy for humans to modify and extend

**Why track patterns?**
This is the learning system that makes the workflow improve over time. Without pattern tracking, we'd catch the same issues repeatedly. With it, common mistakes become impossible through proactive prevention.
