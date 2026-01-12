package components

import (
	"testing"
	"time"
)

func TestParseRelativeDate(t *testing.T) {
	// Use a fixed "now" for testing to make tests deterministic
	fixedNow := time.Date(2025, 1, 12, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name        string
		input       string
		expectedDay int
		expectedErr bool
		description string
	}{
		{
			name:        "today shorthand",
			input:       "t",
			expectedDay: 12,
			expectedErr: false,
			description: "Should parse 't' as today",
		},
		{
			name:        "today full",
			input:       "today",
			expectedDay: 12,
			expectedErr: false,
			description: "Should parse 'today' as today",
		},
		{
			name:        "tomorrow shorthand",
			input:       "tm",
			expectedDay: 13,
			expectedErr: false,
			description: "Should parse 'tm' as tomorrow",
		},
		{
			name:        "tomorrow full",
			input:       "tomorrow",
			expectedDay: 13,
			expectedErr: false,
			description: "Should parse 'tomorrow' as tomorrow",
		},
		{
			name:        "plus 3 days",
			input:       "+3d",
			expectedDay: 15,
			expectedErr: false,
			description: "Should parse '+3d' as 3 days from now",
		},
		{
			name:        "plus 1 day",
			input:       "+1d",
			expectedDay: 13,
			expectedErr: false,
			description: "Should parse '+1d' as 1 day from now",
		},
		{
			name:        "plus 2 weeks",
			input:       "+2w",
			expectedDay: 26,
			expectedErr: false,
			description: "Should parse '+2w' as 2 weeks from now",
		},
		{
			name:        "plus 1 week",
			input:       "+1w",
			expectedDay: 19,
			expectedErr: false,
			description: "Should parse '+1w' as 1 week from now",
		},
		{
			name:        "next monday",
			input:       "mon",
			expectedDay: 13, // Jan 12, 2025 is Sunday, next Monday is Jan 13
			expectedErr: false,
			description: "Should parse 'mon' as next Monday",
		},
		{
			name:        "next tuesday",
			input:       "tue",
			expectedDay: 14,
			expectedErr: false,
			description: "Should parse 'tue' as next Tuesday",
		},
		{
			name:        "next wednesday",
			input:       "wed",
			expectedDay: 15,
			expectedErr: false,
			description: "Should parse 'wed' as next Wednesday",
		},
		{
			name:        "next thursday",
			input:       "thu",
			expectedDay: 16,
			expectedErr: false,
			description: "Should parse 'thu' as next Thursday",
		},
		{
			name:        "next friday",
			input:       "fri",
			expectedDay: 17,
			expectedErr: false,
			description: "Should parse 'fri' as next Friday",
		},
		{
			name:        "next saturday",
			input:       "sat",
			expectedDay: 18,
			expectedErr: false,
			description: "Should parse 'sat' as next Saturday",
		},
		{
			name:        "next sunday",
			input:       "sun",
			expectedDay: 19, // Jan 12 is Sunday, next Sunday is Jan 19
			expectedErr: false,
			description: "Should parse 'sun' as next Sunday",
		},
		{
			name:        "absolute date YYYY-MM-DD",
			input:       "2025-01-15",
			expectedDay: 15,
			expectedErr: false,
			description: "Should parse absolute date '2025-01-15'",
		},
		{
			name:        "invalid format",
			input:       "invalid",
			expectedErr: true,
			description: "Should return error for invalid format",
		},
		{
			name:        "invalid weekday",
			input:       "xyz",
			expectedErr: true,
			description: "Should return error for invalid weekday",
		},
		{
			name:        "empty string",
			input:       "",
			expectedErr: true,
			description: "Should return error for empty string",
		},
		{
			name:        "invalid date - Feb 30",
			input:       "2025-02-30",
			expectedErr: true,
			description: "Should reject Feb 30 as invalid",
		},
		{
			name:        "invalid date - month 13",
			input:       "2025-13-01",
			expectedErr: true,
			description: "Should reject month 13",
		},
		{
			name:        "invalid date - day 32",
			input:       "2025-01-32",
			expectedErr: true,
			description: "Should reject day 32",
		},
		{
			name:        "invalid date - Feb 29 non-leap year",
			input:       "2025-02-29",
			expectedErr: true,
			description: "Should reject Feb 29 in non-leap year",
		},
		{
			name:        "past date - yesterday",
			input:       "2025-01-11",
			expectedErr: true,
			description: "Should reject past dates",
		},
		{
			name:        "past date - last year",
			input:       "2024-12-31",
			expectedErr: true,
			description: "Should reject dates from last year",
		},
		{
			name:        "+0d edge case",
			input:       "+0d",
			expectedErr: true,
			description: "Should reject +0d and suggest 't' or 'today'",
		},
		{
			name:        "+0w edge case",
			input:       "+0w",
			expectedErr: true,
			description: "Should reject +0w and suggest 't' or 'today'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseRelativeDateWithNow(tt.input, fixedNow)

			if tt.expectedErr {
				if err == nil {
					t.Errorf("Expected error for input %q, but got none", tt.input)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error for input %q: %v", tt.input, err)
				return
			}

			if result.Day() != tt.expectedDay {
				t.Errorf("Expected day %d for input %q, got %d", tt.expectedDay, tt.input, result.Day())
			}
		})
	}
}

func TestFormatDate(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Time
		expected string
	}{
		{
			name:     "standard date",
			input:    time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
			expected: "2025-01-15",
		},
		{
			name:     "single digit month",
			input:    time.Date(2025, 3, 5, 0, 0, 0, 0, time.UTC),
			expected: "2025-03-05",
		},
		{
			name:     "end of year",
			input:    time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC),
			expected: "2025-12-31",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatDate(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestNextWeekday(t *testing.T) {
	// Fixed "now" is Sunday, Jan 12, 2025
	fixedNow := time.Date(2025, 1, 12, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name        string
		weekday     string
		expectedDay int
		expectedErr bool
	}{
		{
			name:        "monday",
			weekday:     "mon",
			expectedDay: 13,
			expectedErr: false,
		},
		{
			name:        "tuesday",
			weekday:     "tue",
			expectedDay: 14,
			expectedErr: false,
		},
		{
			name:        "wednesday",
			weekday:     "wed",
			expectedDay: 15,
			expectedErr: false,
		},
		{
			name:        "thursday",
			weekday:     "thu",
			expectedDay: 16,
			expectedErr: false,
		},
		{
			name:        "friday",
			weekday:     "fri",
			expectedDay: 17,
			expectedErr: false,
		},
		{
			name:        "saturday",
			weekday:     "sat",
			expectedDay: 18,
			expectedErr: false,
		},
		{
			name:        "sunday",
			weekday:     "sun",
			expectedDay: 19, // next Sunday, not today
			expectedErr: false,
		},
		{
			name:        "invalid weekday",
			weekday:     "xyz",
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := nextWeekdayWithNow(tt.weekday, fixedNow)

			if tt.expectedErr {
				if err == nil {
					t.Errorf("Expected error for weekday %q, but got none", tt.weekday)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error for weekday %q: %v", tt.weekday, err)
				return
			}

			if result.Day() != tt.expectedDay {
				t.Errorf("Expected day %d for weekday %q, got %d", tt.expectedDay, tt.weekday, result.Day())
			}
		})
	}
}

func TestParseDateBoundaryConditions(t *testing.T) {
	// Use a fixed "now" for testing - Jan 12, 2025 (Sunday)
	fixedNow := time.Date(2025, 1, 12, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name        string
		input       string
		expectedDay int
		expectedErr bool
		description string
	}{
		{
			name:        "year boundary - Dec 31 to Jan 1",
			input:       "2025-12-31",
			expectedDay: 31,
			expectedErr: false,
			description: "Should handle year boundary dates",
		},
		{
			name:        "large value - +1000d",
			input:       "+1000d",
			expectedErr: false,
			description: "Should handle large day values",
		},
		{
			name:        "large value - +52w",
			input:       "+52w",
			expectedErr: false,
			description: "Should handle large week values",
		},
		{
			name:        "leap year - 2024-02-29 (past)",
			input:       "2024-02-29",
			expectedErr: true,
			description: "Should reject Feb 29 2024 as past date",
		},
		{
			name:        "leap year - 2028-02-29 (future)",
			input:       "2028-02-29",
			expectedDay: 29,
			expectedErr: false,
			description: "Should accept Feb 29 2028 as valid leap year date",
		},
		{
			name:        "month boundary - Jan 31",
			input:       "2025-01-31",
			expectedDay: 31,
			expectedErr: false,
			description: "Should handle month boundary",
		},
		{
			name:        "invalid month - 00",
			input:       "2025-00-15",
			expectedErr: true,
			description: "Should reject month 00",
		},
		{
			name:        "invalid day - 00",
			input:       "2025-01-00",
			expectedErr: true,
			description: "Should reject day 00",
		},
		{
			name:        "April 31 (invalid)",
			input:       "2025-04-31",
			expectedErr: true,
			description: "Should reject April 31",
		},
		{
			name:        "June 31 (invalid)",
			input:       "2025-06-31",
			expectedErr: true,
			description: "Should reject June 31",
		},
		{
			name:        "September 31 (invalid)",
			input:       "2025-09-31",
			expectedErr: true,
			description: "Should reject September 31",
		},
		{
			name:        "November 31 (invalid)",
			input:       "2025-11-31",
			expectedErr: true,
			description: "Should reject November 31",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseRelativeDateWithNow(tt.input, fixedNow)

			if tt.expectedErr {
				if err == nil {
					t.Errorf("Expected error for input %q, but got none", tt.input)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error for input %q: %v", tt.input, err)
				return
			}

			if tt.expectedDay > 0 && result.Day() != tt.expectedDay {
				t.Errorf("Expected day %d for input %q, got %d", tt.expectedDay, tt.input, result.Day())
			}
		})
	}
}

func TestGetDateDescription(t *testing.T) {
	fixedNow := time.Date(2025, 1, 12, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name     string
		date     time.Time
		expected string
	}{
		{
			name:     "today",
			date:     fixedNow,
			expected: "today",
		},
		{
			name:     "tomorrow",
			date:     fixedNow.AddDate(0, 0, 1),
			expected: "tomorrow",
		},
		{
			name:     "3 days from now",
			date:     fixedNow.AddDate(0, 0, 3),
			expected: "Wednesday", // Jan 12 + 3 = Jan 15 (Wednesday)
		},
		{
			name:     "1 day from now",
			date:     fixedNow.AddDate(0, 0, 1),
			expected: "tomorrow",
		},
		{
			name:     "1 week from now",
			date:     fixedNow.AddDate(0, 0, 7),
			expected: "in 1 week",
		},
		{
			name:     "2 weeks from now",
			date:     fixedNow.AddDate(0, 0, 14),
			expected: "in 2 weeks",
		},
		{
			name:     "3 weeks from now",
			date:     fixedNow.AddDate(0, 0, 21),
			expected: "in 3 weeks",
		},
		{
			name:     "next monday",
			date:     time.Date(2025, 1, 13, 0, 0, 0, 0, time.UTC),
			expected: "tomorrow", // Jan 13 is tomorrow from Jan 12
		},
		{
			name:     "next friday",
			date:     time.Date(2025, 1, 17, 0, 0, 0, 0, time.UTC),
			expected: "Friday",
		},
		{
			name:     "28 days away - show formatted date",
			date:     fixedNow.AddDate(0, 0, 28),
			expected: "Feb 9, 2025",
		},
		{
			name:     "30 days away - show formatted date",
			date:     fixedNow.AddDate(0, 0, 30),
			expected: "Feb 11, 2025",
		},
		{
			name:     "60 days away - show formatted date",
			date:     fixedNow.AddDate(0, 0, 60),
			expected: "Mar 13, 2025",
		},
		{
			name:     "100 days away - show formatted date",
			date:     fixedNow.AddDate(0, 0, 100),
			expected: "Apr 22, 2025",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getDateDescriptionWithNow(tt.date, fixedNow)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}
