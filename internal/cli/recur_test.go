// Package cli — TDD red tests for v1-T6 recurrence repeater arithmetic
// (reckon-ar9m). internal/cli/recur.go does not exist yet; this file
// references repeatKind, repeatSpec, parseRepeat, parseSchedDate,
// daysBetween, and advanceSchedule -- types/functions the implementation
// phase must define -- so the whole `cli` package fails to COMPILE right
// now. That is the expected TDD-red state at this stage ("the package does
// not build", not "tests run and fail"), mirroring todo_test.go's and
// add_test.go's own header convention for the immediately-preceding
// reckon-qiua/reckon-uv09 tickets.
//
// Precedent / harness reuse: none needed here -- this file is pure,
// table-driven unit tests against plan.md's new internal/cli/recur.go
// module. No vault/index/CLI harness is exercised; see todo_recur_test.go
// for the CLI-integration-level scenarios that DO reuse setupQueryVault /
// runTodo / buildIndex / runQuery (query_test.go) and mustWriteFile /
// mustReadFile / isValidULID (adopt_test.go).
//
// ─────────────────────────────────────────────────────────────────────────
// Pinned contract (plan.md "Create: internal/cli/recur.go"):
// ─────────────────────────────────────────────────────────────────────────
//
//	type repeatKind int
//	const ( repeatFixed repeatKind = iota; repeatSkip; repeatFromCompletion ) // +Nd, ++Nd, .+Nd
//
//	type repeatSpec struct {
//	    kind repeatKind
//	    days int    // normalized to days: d->N, w->7N
//	    raw  string // original cookie, for error messages
//	}
//
//	var repeatCookieRe = regexp.MustCompile(`^(\+\+|\.\+|\+)([1-9][0-9]*)([dw])$`)
//
//	func parseRepeat(s string) (repeatSpec, error)
//	func parseSchedDate(s string) (time.Time, error)
//	func daysBetween(from, to time.Time) int
//	func advanceSchedule(old, today time.Time, spec repeatSpec) (next time.Time, missed int)
//
// Exact arithmetic per family (plan.md "Exact Repeater Arithmetic"):
//   - `+Nd` (repeatFixed):        next = old.AddDate(0,0,days)                     -- ignores today entirely.
//   - `++Nd` (repeatSkip):        next = smallest old+k*days, k>=1, strictly > today.
//   - `.+Nd` (repeatFromCompletion): next = today.AddDate(0,0,days)                -- ignores old entirely.
//
// Pile-up (family-independent): missed = daysBetween(old,today) > days ?
// daysBetween(old,today)/days : 0 (integer division, floor).
//
// All dates are UTC, date-only (no time-of-day component); tests build
// fixture dates with the local mustUTCDate helper below (deliberately NOT
// routed through parseSchedDate itself, so a fixture-construction bug can
// never mask a parseSchedDate bug it would otherwise be tested against).
package cli

import (
	"testing"
	"time"
)

// mustUTCDate parses "2006-01-02" into a UTC time.Time, fataling on error.
// Independent of parseSchedDate (the function under test elsewhere in this
// file) so fixture construction never shares a bug with the code being
// verified.
func mustUTCDate(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.ParseInLocation("2006-01-02", s, time.UTC)
	if err != nil {
		t.Fatalf("mustUTCDate(%q): %v", s, err)
	}
	return tm
}

// ─────────────────────────────────────────────────────────────────────────
// TestParseRepeat_ValidFamiliesAndUnits
// ─────────────────────────────────────────────────────────────────────────

func TestParseRepeat_ValidFamiliesAndUnits(t *testing.T) {
	tests := []struct {
		name     string
		cookie   string
		wantKind repeatKind
		wantDays int
	}{
		{"fixed_days", "+7d", repeatFixed, 7},
		{"skip_days", "++7d", repeatSkip, 7},
		{"from_completion_days", ".+3d", repeatFromCompletion, 3},
		{"fixed_weeks_normalizes_to_days", "+2w", repeatFixed, 14},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := parseRepeat(tt.cookie)
			if err != nil {
				t.Fatalf("parseRepeat(%q): unexpected error: %v", tt.cookie, err)
			}
			if spec.kind != tt.wantKind {
				t.Errorf("kind = %v, want %v", spec.kind, tt.wantKind)
			}
			if spec.days != tt.wantDays {
				t.Errorf("days = %d, want %d", spec.days, tt.wantDays)
			}
			if spec.raw != tt.cookie {
				t.Errorf("raw = %q, want %q (original cookie preserved for error messages)", spec.raw, tt.cookie)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────
// TestParseRepeat_Rejects (EC-4)
// ─────────────────────────────────────────────────────────────────────────

func TestParseRepeat_Rejects(t *testing.T) {
	bad := []string{
		"weekly", // not org-repeater syntax at all
		"3d",     // no sigil
		"+0d",    // zero interval
		"-1d",    // negative
		"+7m",    // month unit, out of scope (FIRM: d/w only)
		"+7y",    // year unit, out of scope
		"",       // empty
		"++",     // sigil with no number/unit
		".+d",    // missing numeric interval
		"+d",     // missing numeric interval
		"+07d",   // leading zero
		"+7",     // missing unit
		"+7dd",   // trailing garbage
		" +7d",   // leading whitespace
		"+7d ",   // trailing whitespace
	}
	for _, cookie := range bad {
		t.Run(cookie, func(t *testing.T) {
			if _, err := parseRepeat(cookie); err == nil {
				t.Errorf("parseRepeat(%q): expected an error, got nil", cookie)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────
// TestParseSchedDate_Rejects (EC-3)
// ─────────────────────────────────────────────────────────────────────────

func TestParseSchedDate_Rejects(t *testing.T) {
	bad := []string{
		"not-a-date",
		"TBD",
		"",
		"2026-13-40", // invalid month/day
		"07-05-2026", // wrong field order/format
		"2026/07/05", // wrong separator
	}
	for _, s := range bad {
		t.Run(s, func(t *testing.T) {
			if _, err := parseSchedDate(s); err == nil {
				t.Errorf("parseSchedDate(%q): expected an error, got nil", s)
			}
		})
	}

	t.Run("valid_date_accepted", func(t *testing.T) {
		got, err := parseSchedDate("2026-07-05")
		if err != nil {
			t.Fatalf("parseSchedDate(valid): unexpected error: %v", err)
		}
		want := mustUTCDate(t, "2026-07-05")
		if !got.Equal(want) {
			t.Errorf("parseSchedDate(\"2026-07-05\") = %v, want %v", got, want)
		}
	})
}

// ─────────────────────────────────────────────────────────────────────────
// TestAdvanceSchedule_Fixed (+Nd)
// ─────────────────────────────────────────────────────────────────────────

func TestAdvanceSchedule_Fixed(t *testing.T) {
	spec7d := repeatSpec{kind: repeatFixed, days: 7, raw: "+7d"}

	tests := []struct {
		name       string
		old, today string
		wantNext   string
		wantMissed int
	}{
		// TS-1: on-time.
		{"TS-1_on_time", "2026-07-05", "2026-07-05", "2026-07-12", 0},
		// TS-2: late by multiple intervals (34 days) -- still overdue, the
		// fixed family's documented non-catch-up quirk.
		{"TS-2_multi_late_still_overdue", "2026-06-01", "2026-07-05", "2026-06-08", 4},
		// EC-7: early completion -- unaffected by today, no pile-up.
		{"EC-7_early", "2026-07-10", "2026-07-05", "2026-07-17", 0},
		// EC-8: single-late, interior (old < today < old+N).
		{"EC-8_single_late_interior", "2026-07-01", "2026-07-04", "2026-07-08", 0},
		// EC-8: single-late, exact boundary (today == old+N) -- still no
		// pile-up; fixed family lands exactly on today (re-due today).
		{"EC-8_single_late_boundary", "2026-07-01", "2026-07-08", "2026-07-08", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			old := mustUTCDate(t, tt.old)
			today := mustUTCDate(t, tt.today)
			next, missed := advanceSchedule(old, today, spec7d)
			wantNext := mustUTCDate(t, tt.wantNext)
			if !next.Equal(wantNext) {
				t.Errorf("next = %v, want %v", next.Format("2006-01-02"), wantNext.Format("2006-01-02"))
			}
			if missed != tt.wantMissed {
				t.Errorf("missed = %d, want %d", missed, tt.wantMissed)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────
// TestAdvanceSchedule_Skip (++Nd)
// ─────────────────────────────────────────────────────────────────────────

func TestAdvanceSchedule_Skip(t *testing.T) {
	spec7d := repeatSpec{kind: repeatSkip, days: 7, raw: "++7d"}

	tests := []struct {
		name       string
		old, today string
		wantNext   string
		wantMissed int
	}{
		// TS-3: late by multiple intervals -- skips straight to the smallest
		// future occurrence, never landing in the past.
		{"TS-3_multi_late_skips_to_future", "2026-06-01", "2026-07-05", "2026-07-06", 4},
		// on-time: k=1 agrees with the fixed family.
		{"on_time", "2026-07-05", "2026-07-05", "2026-07-12", 0},
		// exact boundary (today == old+N): must NOT land exactly on today
		// (strict >), so it skips one further cycle to old+2N; still no
		// pile-up (documented boundary resolution, FIRM).
		{"boundary_today_equals_old_plus_N", "2026-07-01", "2026-07-08", "2026-07-15", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			old := mustUTCDate(t, tt.old)
			today := mustUTCDate(t, tt.today)
			next, missed := advanceSchedule(old, today, spec7d)
			wantNext := mustUTCDate(t, tt.wantNext)
			if !next.Equal(wantNext) {
				t.Errorf("next = %v, want %v", next.Format("2006-01-02"), wantNext.Format("2006-01-02"))
			}
			if next.After(today) == false {
				t.Errorf("next = %v must be strictly after today = %v for the skip-to-future family", next, today)
			}
			if missed != tt.wantMissed {
				t.Errorf("missed = %d, want %d", missed, tt.wantMissed)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────
// TestAdvanceSchedule_FromCompletion (.+Nd)
// ─────────────────────────────────────────────────────────────────────────

func TestAdvanceSchedule_FromCompletion(t *testing.T) {
	tests := []struct {
		name       string
		days       int
		old, today string
		wantNext   string
		wantMissed int
	}{
		// TS-4 early leg: ignores old entirely, pulls the next occurrence
		// earlier than old+N would have been.
		{"TS-4_early", 3, "2026-07-10", "2026-07-05", "2026-07-08", 0},
		// TS-4 late leg: ignores old entirely, always anchored to today.
		{"TS-4_late", 3, "2026-07-01", "2026-07-20", "2026-07-23", 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := repeatSpec{kind: repeatFromCompletion, days: tt.days, raw: ".+3d"}
			old := mustUTCDate(t, tt.old)
			today := mustUTCDate(t, tt.today)
			next, missed := advanceSchedule(old, today, spec)
			wantNext := mustUTCDate(t, tt.wantNext)
			if !next.Equal(wantNext) {
				t.Errorf("next = %v, want %v", next.Format("2006-01-02"), wantNext.Format("2006-01-02"))
			}
			if missed != tt.wantMissed {
				t.Errorf("missed = %d, want %d", missed, tt.wantMissed)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────
// TestAdvanceSchedule_AllAgreeOnTime (TS-5 / EC-6)
// ─────────────────────────────────────────────────────────────────────────

func TestAdvanceSchedule_AllAgreeOnTime(t *testing.T) {
	old := mustUTCDate(t, "2026-07-05")
	today := mustUTCDate(t, "2026-07-05")
	want := mustUTCDate(t, "2026-07-12")

	specs := []struct {
		name string
		spec repeatSpec
	}{
		{"fixed", repeatSpec{kind: repeatFixed, days: 7, raw: "+7d"}},
		{"skip", repeatSpec{kind: repeatSkip, days: 7, raw: "++7d"}},
		{"from_completion", repeatSpec{kind: repeatFromCompletion, days: 7, raw: ".+7d"}},
	}
	for _, tt := range specs {
		t.Run(tt.name, func(t *testing.T) {
			next, missed := advanceSchedule(old, today, tt.spec)
			if !next.Equal(want) {
				t.Errorf("next = %v, want %v (all three families must agree exactly on-time)", next.Format("2006-01-02"), want.Format("2006-01-02"))
			}
			if missed != 0 {
				t.Errorf("missed = %d, want 0", missed)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────
// TestAdvanceSchedule_MissedCount (EC-8 boundary vs EC-9)
// ─────────────────────────────────────────────────────────────────────────

func TestAdvanceSchedule_MissedCount(t *testing.T) {
	spec7d := repeatSpec{kind: repeatFixed, days: 7, raw: "+7d"}
	old := mustUTCDate(t, "2026-07-01")

	t.Run("EC-8_boundary_today_equals_old_plus_N_missed_zero", func(t *testing.T) {
		today := mustUTCDate(t, "2026-07-08") // exactly old+7
		_, missed := advanceSchedule(old, today, spec7d)
		if missed != 0 {
			t.Errorf("missed = %d, want 0 (exact one-interval boundary is not pile-up)", missed)
		}
	})

	t.Run("EC-9_one_day_past_boundary_missed_one", func(t *testing.T) {
		today := mustUTCDate(t, "2026-07-09") // old+7+1
		_, missed := advanceSchedule(old, today, spec7d)
		if missed != 1 {
			t.Errorf("missed = %d, want 1", missed)
		}
	})

	t.Run("daysBetween_directly", func(t *testing.T) {
		if got := daysBetween(old, mustUTCDate(t, "2026-07-08")); got != 7 {
			t.Errorf("daysBetween(old, old+7) = %d, want 7", got)
		}
		if got := daysBetween(old, mustUTCDate(t, "2026-06-24")); got != -7 {
			t.Errorf("daysBetween(old, old-7) = %d, want -7 (may be negative)", got)
		}
		if got := daysBetween(old, old); got != 0 {
			t.Errorf("daysBetween(old, old) = %d, want 0", got)
		}
	})
}
