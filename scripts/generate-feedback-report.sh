#!/bin/bash
# Generate weekly feedback loop report
# Usage: ./scripts/generate-feedback-report.sh

set -euo pipefail

REPORT_DATE=$(date -I)
REPORT_FILE=".claude/feedback/$REPORT_DATE.md"

echo "📊 Generating Feedback Loop Report for $REPORT_DATE"
echo ""

# Create feedback directory if needed
mkdir -p .claude/feedback
mkdir -p .claude/metrics

# Count reviews this week
REVIEW_COUNT=$(find .claude/work/*/review.md -mtime -7 2>/dev/null | wc -l)

# Start report
cat > "$REPORT_FILE" <<EOF
# Feedback Loop Report: $REPORT_DATE

## Reviews This Week: $REVIEW_COUNT

EOF

# Run pattern extraction and append to report
echo "## Pattern Frequency" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"

./scripts/extract-review-patterns.sh >> /tmp/patterns.txt 2>&1 || true
sed -n '/## Pattern Frequency/,/## Next Actions/p' /tmp/patterns.txt | \
    grep -v "^## " >> "$REPORT_FILE"

echo "" >> "$REPORT_FILE"
echo "## Actions Recommended" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"

sed -n '/## Next Actions/,/## Files to Update/p' /tmp/patterns.txt | \
    grep -v "^## " | grep -v "^$" >> "$REPORT_FILE"

# Calculate success metrics if we have metric files
if [ -d ".claude/metrics" ] && [ "$(ls -A .claude/metrics/*.json 2>/dev/null | wc -l)" -gt 0 ]; then
    echo "" >> "$REPORT_FILE"
    echo "## Success Metrics" >> "$REPORT_FILE"
    echo "" >> "$REPORT_FILE"

    # Count verdicts
    approve_count=$(grep -h "APPROVE" .claude/metrics/review-*.json 2>/dev/null | grep -v "APPROVE_WITH_CHANGES" | wc -l || echo "0")
    approve_with_changes=$(grep -h "APPROVE_WITH_CHANGES" .claude/metrics/review-*.json 2>/dev/null | wc -l || echo "0")
    request_changes=$(grep -h "REQUEST_CHANGES" .claude/metrics/review-*.json 2>/dev/null | wc -l || echo "0")
    total=$((approve_count + approve_with_changes + request_changes))

    if [ $total -gt 0 ]; then
        approve_rate=$(( (approve_count * 100) / total ))
        echo "- **First-review approve rate:** ${approve_rate}%"
    fi

    # Pattern trends
    echo "- **Patterns resolved:** (To be tracked manually)"
    echo "- **New patterns emerging:** (To be tracked manually)"

    echo "" >> "$REPORT_FILE"
fi

# Recommendations section
cat >> "$REPORT_FILE" <<EOF

## Recommendations

Based on pattern frequency above:

1. **High priority:** Patterns with 6+ occurrences need preflight checks
2. **Medium priority:** Patterns with 3-5 occurrences need guide updates
3. **Monitor:** Patterns with 1-2 occurrences (watch for recurrence)

## Next Steps

- [ ] Update docs/REVIEW_PATTERNS.md frequency counts
- [ ] Add high-frequency patterns to relevant guides
- [ ] Create preflight checks for very common patterns
- [ ] Review metrics trends

---

Generated: $REPORT_DATE
By: scripts/generate-feedback-report.sh
EOF

echo "✅ Report generated: $REPORT_FILE"
echo ""
cat "$REPORT_FILE"
