#!/bin/bash
# Extract patterns from code reviews for feedback loop
# Usage: ./scripts/extract-review-patterns.sh [ticket-id]
#   If ticket-id provided: analyze that review
#   If no args: analyze all reviews from past week

set -euo pipefail

# Common patterns to track
PATTERNS=(
    "unwrapped error"
    "missing defer"
    "closure capture"
    "nil check"
    "missing validation"
    "os.Exit"
    "quiet flag"
    "missing test"
    "edge case"
    "hardcoded"
)

# Colors
RED='\033[0;31m'
YELLOW='\033[1;33m'
GREEN='\033[0;32m'
NC='\033[0m' # No Color

echo "🔍 Review Pattern Extractor"
echo "============================"
echo ""

# Determine which reviews to analyze
if [ $# -eq 1 ]; then
    # Single ticket
    TICKET_ID="$1"
    REVIEW_FILES=("ticket-work/$TICKET_ID/review.md")
    echo "📄 Analyzing: $TICKET_ID"
else
    # All reviews from past week
    REVIEW_FILES=($(find ticket-work/*/review.md -mtime -7 2>/dev/null || true))
    if [ ${#REVIEW_FILES[@]} -eq 0 ]; then
        echo "❌ No reviews found from past week"
        exit 1
    fi
    echo "📅 Analyzing reviews from past 7 days: ${#REVIEW_FILES[@]} files"
fi

echo ""

# Extract patterns from each review
declare -A pattern_counts
declare -A pattern_tickets

for review_file in "${REVIEW_FILES[@]}"; do
    if [ ! -f "$review_file" ]; then
        continue
    fi

    ticket_id=$(basename $(dirname "$review_file"))

    # Count patterns in this review
    for pattern in "${PATTERNS[@]}"; do
        count=$(grep -ic "$pattern" "$review_file" || echo "0")
        if [ "$count" -gt 0 ]; then
            # Increment total count
            pattern_counts["$pattern"]=$((${pattern_counts["$pattern"]:-0} + count))
            # Track which tickets
            pattern_tickets["$pattern"]+="$ticket_id "
        fi
    done
done

# Display results
echo "## Pattern Frequency"
echo ""

for pattern in "${PATTERNS[@]}"; do
    count=${pattern_counts["$pattern"]:-0}
    tickets=${pattern_tickets["$pattern"]:-}

    # Determine status
    if [ $count -eq 0 ]; then
        status="✅ No occurrences"
        color=$GREEN
    elif [ $count -le 2 ]; then
        status="🟡 Watch ($count)"
        color=$YELLOW
    elif [ $count -le 5 ]; then
        status="🔴 Common ($count) - Add to guides"
        color=$RED
    elif [ $count -le 10 ]; then
        status="🔴🔴 Very common ($count) - Add to preflight"
        color=$RED
    else
        status="🔴🔴🔴 Critical ($count) - Automate!"
        color=$RED
    fi

    printf "${color}%-25s ${status}${NC}\n" "$pattern:"

    if [ $count -gt 0 ] && [ $count -le 10 ]; then
        echo "  Tickets: $tickets"
    fi
done

echo ""
echo "## Next Actions"
echo ""

# Suggest actions based on frequency
action_needed=false
for pattern in "${PATTERNS[@]}"; do
    count=${pattern_counts["$pattern"]:-0}

    if [ $count -ge 3 ] && [ $count -lt 6 ]; then
        echo "⚠️  '$pattern' ($count) → Add to subsystem AGENTS.md"
        action_needed=true
    elif [ $count -ge 6 ] && [ $count -lt 11 ]; then
        echo "🚨 '$pattern' ($count) → Add to preflight manual checks"
        action_needed=true
    elif [ $count -ge 11 ]; then
        echo "🔥 '$pattern' ($count) → AUTOMATE in preflight!"
        action_needed=true
    fi
done

if [ "$action_needed" = false ]; then
    echo "✅ No patterns hit action thresholds"
fi

echo ""
echo "## Files to Update"
echo ""
echo "- docs/REVIEW_PATTERNS.md (update frequency counts)"

for pattern in "${PATTERNS[@]}"; do
    count=${pattern_counts["$pattern"]:-0}
    if [ $count -ge 3 ]; then
        echo "- internal/*/AGENTS.md (add '$pattern' warning)"
    fi
    if [ $count -ge 6 ]; then
        echo "- docs/agents/preflight.md (add '$pattern' check)"
    fi
done

echo ""
echo "📖 See docs/agents/feedback-loop.md for detailed process"
