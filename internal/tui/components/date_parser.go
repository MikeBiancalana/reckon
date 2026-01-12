package components

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseRelativeDate parses relative date strings and returns a time.Time
// Supports:
// - "t" or "today" - today
// - "tm" or "tomorrow" - tomorrow
// - "mon", "tue", "wed", "thu", "fri", "sat", "sun" - next occurrence of weekday
// - "+3d" - 3 days from now
// - "+2w" - 2 weeks from now
// - "YYYY-MM-DD" - absolute date
func ParseRelativeDate(input string) (time.Time, error) {
	return parseRelativeDateWithNow(input, time.Now())
}

// parseRelativeDateWithNow is an internal function that accepts a "now" parameter for testing
func parseRelativeDateWithNow(input string, now time.Time) (time.Time, error) {
	input = strings.TrimSpace(strings.ToLower(input))

	if input == "" {
		return time.Time{}, fmt.Errorf("empty input")
	}

	// Check for absolute date (YYYY-MM-DD)
	if len(input) == 10 && input[4] == '-' && input[7] == '-' {
		parsed, err := time.Parse("2006-01-02", input)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid date format: %w", err)
		}
		// Convert to same timezone as "now" for consistency
		result := time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, now.Location())

		// Verify the date components match to catch invalid dates that normalize
		// (e.g., 2025-02-30 would normalize to 2025-03-02)
		if result.Year() != parsed.Year() || result.Month() != parsed.Month() || result.Day() != parsed.Day() {
			return time.Time{}, fmt.Errorf("invalid date: %s (normalized to %s)", input, result.Format("2006-01-02"))
		}

		// Reject past dates
		nowStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		if result.Before(nowStart) {
			return time.Time{}, fmt.Errorf("date cannot be in the past: %s", input)
		}

		return result, nil
	}

	// Handle "today" or "t"
	if input == "t" || input == "today" {
		return now, nil
	}

	// Handle "tomorrow" or "tm"
	if input == "tm" || input == "tomorrow" {
		return now.AddDate(0, 0, 1), nil
	}

	// Handle "+Nd" (N days from now)
	if strings.HasPrefix(input, "+") && strings.HasSuffix(input, "d") {
		daysStr := strings.TrimPrefix(strings.TrimSuffix(input, "d"), "+")
		days, err := strconv.Atoi(daysStr)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid days format: %w", err)
		}
		if days < 0 {
			return time.Time{}, fmt.Errorf("days must be positive")
		}
		if days == 0 {
			return time.Time{}, fmt.Errorf("use 't' or 'today' instead of '+0d'")
		}
		return now.AddDate(0, 0, days), nil
	}

	// Handle "+Nw" (N weeks from now)
	if strings.HasPrefix(input, "+") && strings.HasSuffix(input, "w") {
		weeksStr := strings.TrimPrefix(strings.TrimSuffix(input, "w"), "+")
		weeks, err := strconv.Atoi(weeksStr)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid weeks format: %w", err)
		}
		if weeks < 0 {
			return time.Time{}, fmt.Errorf("weeks must be positive")
		}
		if weeks == 0 {
			return time.Time{}, fmt.Errorf("use 't' or 'today' instead of '+0w'")
		}
		return now.AddDate(0, 0, weeks*7), nil
	}

	// Handle weekday shortcuts
	weekdays := []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}
	for _, wd := range weekdays {
		if input == wd {
			return nextWeekdayWithNow(wd, now)
		}
	}

	return time.Time{}, fmt.Errorf("invalid date format: %s", input)
}

// FormatDate formats a time.Time as YYYY-MM-DD
func FormatDate(t time.Time) string {
	return t.Format("2006-01-02")
}

// NextWeekday returns the next occurrence of the specified weekday
// weekday should be one of: mon, tue, wed, thu, fri, sat, sun
func NextWeekday(weekday string) (time.Time, error) {
	return nextWeekdayWithNow(weekday, time.Now())
}

// nextWeekdayWithNow is an internal function that accepts a "now" parameter for testing
func nextWeekdayWithNow(weekday string, now time.Time) (time.Time, error) {
	weekday = strings.ToLower(strings.TrimSpace(weekday))

	weekdayMap := map[string]time.Weekday{
		"mon": time.Monday,
		"tue": time.Tuesday,
		"wed": time.Wednesday,
		"thu": time.Thursday,
		"fri": time.Friday,
		"sat": time.Saturday,
		"sun": time.Sunday,
	}

	targetWeekday, ok := weekdayMap[weekday]
	if !ok {
		return time.Time{}, fmt.Errorf("invalid weekday: %s", weekday)
	}

	// Calculate days until target weekday
	currentWeekday := now.Weekday()
	daysUntil := int(targetWeekday - currentWeekday)

	// If target is today or in the past this week, go to next week
	if daysUntil <= 0 {
		daysUntil += 7
	}

	return now.AddDate(0, 0, daysUntil), nil
}

// GetDateDescription returns a human-readable description of the date
// relative to the current date (e.g., "today", "tomorrow", "in 3 days", "Monday")
func GetDateDescription(date time.Time) string {
	return getDateDescriptionWithNow(date, time.Now())
}

// getDateDescriptionWithNow is an internal function that accepts a "now" parameter for testing
func getDateDescriptionWithNow(date time.Time, now time.Time) string {
	// Normalize to start of day for comparison
	nowStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	dateStart := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())

	daysDiff := int(dateStart.Sub(nowStart).Hours() / 24)

	switch daysDiff {
	case 0:
		return "today"
	case 1:
		return "tomorrow"
	case 2, 3, 4, 5, 6:
		// For dates within the next week (but not today/tomorrow), show weekday
		return date.Weekday().String()
	}

	// Check for exact weeks
	if daysDiff%7 == 0 && daysDiff > 0 && daysDiff < 28 {
		weeks := daysDiff / 7
		if weeks == 1 {
			return "in 1 week"
		}
		return fmt.Sprintf("in %d weeks", weeks)
	}

	// Check if it's within a few weeks
	if daysDiff >= 7 && daysDiff < 28 {
		weeks := daysDiff / 7
		if weeks == 1 {
			return "in 1 week"
		}
		return fmt.Sprintf("in %d weeks", weeks)
	}

	// For dates 28+ days away, show the formatted date
	if daysDiff >= 28 {
		return date.Format("Jan 2, 2006")
	}

	// For anything else, just return the weekday
	return date.Weekday().String()
}
