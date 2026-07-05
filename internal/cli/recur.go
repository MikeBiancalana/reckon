// Package cli — org-style repeater arithmetic for v1-T6 recurring durable
// todos (reckon-ar9m). Pure, greenfield module: no IO, no vault/index
// dependency, deterministic given an injected `today` so it is directly
// unit-testable (recur_test.go) independent of the CLI integration harness
// (todo_recur_test.go) that drives the same math through `rk todo done`.
//
// Repeater families (org-mode vocabulary, plan.md "Exact Repeater
// Arithmetic"):
//   - `+Nd` (repeatFixed)        — next = old + N, ignoring today entirely.
//     May land on/before today (the fixed family's documented non-catch-up
//     quirk: repeated completion is how a user "walks" it forward).
//   - `++Nd` (repeatSkip)        — next = smallest old + k*N (k>=1) strictly
//     after today. Never lands in the past or exactly on today.
//   - `.+Nd` (repeatFromCompletion) — next = today + N, ignoring old
//     entirely. Always strictly in the future (N >= 1).
//
// Pile-up (family-independent): missed = daysBetween(old,today)/N when that
// quotient is positive (i.e. more than one full interval has elapsed since
// old), else 0. The caller (doneRecurringTodo, todo.go) materializes exactly
// one ephemeral catch-up instance iff missed > 0.
//
// All dates are UTC, date-only (no time-of-day component): parsed via
// time.ParseInLocation("2006-01-02", s, time.UTC) and compared/added with
// time.Time.AddDate, never time.Parse's local-default or Truncate-based
// arithmetic (docs/REVIEW_PATTERNS.md's UTC-vs-local anti-pattern guard —
// mixing clocks between the stored "old" and the computed "today" would
// silently shift the effective calendar day for non-UTC hosts).
package cli

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// repeatKind identifies which org-style repeater family a cookie belongs to.
type repeatKind int

const (
	repeatFixed          repeatKind = iota // +Nd
	repeatSkip                             // ++Nd
	repeatFromCompletion                   // .+Nd
)

// repeatSpec is a parsed, normalized repeater cookie.
type repeatSpec struct {
	kind repeatKind
	days int    // normalized to days: d -> N, w -> 7N
	raw  string // original cookie, for error messages
}

// repeatCookieRe matches one org-style repeater cookie: an optional-doubled
// sigil, a positive (no leading zero) integer, and a d/w unit. Order matters
// in the alternation: "++" and ".+" must be tried before bare "+", or the
// bare-"+" branch would greedily consume the first "+" of "++N..." and leave
// a dangling second "+" that fails to match the required digit class.
var repeatCookieRe = regexp.MustCompile(`^(\+\+|\.\+|\+)([1-9][0-9]*)([dw])$`)

// parseRepeat validates and normalizes a repeat: prop value (EC-4: rejects
// "weekly", "3d", "+0d", "-1d", "+7m"/"+7y" (unit out of scope), "", and any
// cookie with leading/trailing garbage or whitespace).
func parseRepeat(s string) (repeatSpec, error) {
	m := repeatCookieRe.FindStringSubmatch(s)
	if m == nil {
		return repeatSpec{}, fmt.Errorf("malformed repeat cookie %q (want +Nd, ++Nd, or .+Nd; units d/w only)", s)
	}
	sigil, numStr, unit := m[1], m[2], m[3]

	n, err := strconv.Atoi(numStr)
	if err != nil {
		// Unreachable given the regex's digit class, but handled rather than
		// ignored per the codebase's no-silent-fallback convention.
		return repeatSpec{}, fmt.Errorf("malformed repeat cookie %q: %w", s, err)
	}

	days := n
	if unit == "w" {
		days = n * 7
	}

	var kind repeatKind
	switch sigil {
	case "+":
		kind = repeatFixed
	case "++":
		kind = repeatSkip
	case ".+":
		kind = repeatFromCompletion
	}

	return repeatSpec{kind: kind, days: days, raw: s}, nil
}

// parseSchedDate parses a scheduled: prop value as a UTC, date-only
// time.Time (EC-3: rejects anything not exactly "YYYY-MM-DD").
func parseSchedDate(s string) (time.Time, error) {
	t, err := time.ParseInLocation("2006-01-02", s, time.UTC)
	if err != nil {
		return time.Time{}, fmt.Errorf("malformed scheduled date %q (want YYYY-MM-DD): %w", s, err)
	}
	return t, nil
}

// daysBetween returns the whole number of UTC days from `from` to `to`
// (to - from), which may be negative if to precedes from. Both operands are
// expected to be UTC, date-only (midnight) time.Time values, so the
// difference is always an exact multiple of 24h -- integer duration division
// is used instead of float Hours() math to avoid any rounding ambiguity.
func daysBetween(from, to time.Time) int {
	return int(to.Sub(from) / (24 * time.Hour))
}

// advanceSchedule computes the next scheduled date for a repeater family
// given the current cursor (old) and the completion date (today), plus how
// many intervals were missed (family-independent pile-up trigger: strictly
// more than one interval elapsed since old).
func advanceSchedule(old, today time.Time, spec repeatSpec) (next time.Time, missed int) {
	switch spec.kind {
	case repeatFixed:
		// Single unconditional hop off old; ignores today entirely. May
		// remain <= today (overdue) -- intentional org "catch-up" behavior:
		// completing again keeps walking it forward one interval at a time.
		next = old.AddDate(0, 0, spec.days)

	case repeatSkip:
		// Smallest old + k*days (k >= 1) that lands strictly after today --
		// never overdue, never exactly on today (documented boundary
		// resolution, plan.md "Known Risks").
		next = old.AddDate(0, 0, spec.days)
		for !next.After(today) {
			next = next.AddDate(0, 0, spec.days)
		}

	case repeatFromCompletion:
		// Always anchored to the actual completion day, ignoring old
		// entirely -- always strictly future since days >= 1.
		next = today.AddDate(0, 0, spec.days)
	}

	if d := daysBetween(old, today); d > spec.days {
		missed = d / spec.days
	}
	return next, missed
}
