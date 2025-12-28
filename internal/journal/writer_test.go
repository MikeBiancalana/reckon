package journal

import (
	"strings"
	"testing"
	"time"
)

func TestWriteJournal_Schedule(t *testing.T) {
	tests := []struct {
		name     string
		journal  *Journal
		expected string
	}{
		{
			name: "schedule with times",
			journal: &Journal{
				Date: "2025-12-28",
				ScheduleItems: []ScheduleItem{
					{
						ID:       "1",
						Time:     mustParseTime(t, "2025-12-28 09:00"),
						Content:  "Morning standup",
						Position: 0,
					},
					{
						ID:       "2",
						Time:     mustParseTime(t, "2025-12-28 14:00"),
						Content:  "Client meeting",
						Position: 1,
					},
				},
				Intentions: []Intention{},
				Wins:       []Win{},
				LogEntries: []LogEntry{},
			},
			expected: `---
date: 2025-12-28
---

## Schedule

- 09:00 Morning standup
- 14:00 Client meeting

## Intentions


## Wins


## Log

`,
		},
		{
			name: "schedule with and without times",
			journal: &Journal{
				Date: "2025-12-28",
				ScheduleItems: []ScheduleItem{
					{
						ID:       "1",
						Time:     mustParseTime(t, "2025-12-28 09:00"),
						Content:  "Morning standup",
						Position: 0,
					},
					{
						ID:       "2",
						Time:     time.Time{}, // Zero time
						Content:  "Review PR",
						Position: 1,
					},
					{
						ID:       "3",
						Time:     mustParseTime(t, "2025-12-28 14:00"),
						Content:  "Client meeting",
						Position: 2,
					},
				},
				Intentions: []Intention{},
				Wins:       []Win{},
				LogEntries: []LogEntry{},
			},
			expected: `---
date: 2025-12-28
---

## Schedule

- 09:00 Morning standup
- Review PR
- 14:00 Client meeting

## Intentions


## Wins


## Log

`,
		},
		{
			name: "empty schedule",
			journal: &Journal{
				Date:          "2025-12-28",
				ScheduleItems: []ScheduleItem{},
				Intentions:    []Intention{},
				Wins:          []Win{},
				LogEntries:    []LogEntry{},
			},
			expected: `---
date: 2025-12-28
---

## Schedule


## Intentions


## Wins


## Log

`,
		},
		{
			name: "schedule sorted by position",
			journal: &Journal{
				Date: "2025-12-28",
				ScheduleItems: []ScheduleItem{
					{
						ID:       "3",
						Time:     mustParseTime(t, "2025-12-28 14:00"),
						Content:  "Third item",
						Position: 2,
					},
					{
						ID:       "1",
						Time:     mustParseTime(t, "2025-12-28 09:00"),
						Content:  "First item",
						Position: 0,
					},
					{
						ID:       "2",
						Time:     mustParseTime(t, "2025-12-28 10:00"),
						Content:  "Second item",
						Position: 1,
					},
				},
				Intentions: []Intention{},
				Wins:       []Win{},
				LogEntries: []LogEntry{},
			},
			expected: `---
date: 2025-12-28
---

## Schedule

- 09:00 First item
- 10:00 Second item
- 14:00 Third item

## Intentions


## Wins


## Log

`,
		},
		{
			name: "full journal with schedule",
			journal: &Journal{
				Date: "2025-12-28",
				ScheduleItems: []ScheduleItem{
					{
						ID:       "1",
						Time:     mustParseTime(t, "2025-12-28 09:00"),
						Content:  "Morning standup",
						Position: 0,
					},
				},
				Intentions: []Intention{
					{
						ID:       "int1",
						Text:     "Complete task",
						Status:   IntentionDone,
						Position: 0,
					},
				},
				Wins: []Win{
					{
						ID:       "win1",
						Text:     "Fixed bug",
						Position: 0,
					},
				},
				LogEntries: []LogEntry{
					{
						ID:        "log1",
						Timestamp: mustParseTime(t, "2025-12-28 10:00"),
						Content:   "Started work",
						EntryType: EntryTypeLog,
						Position:  0,
					},
				},
			},
			expected: `---
date: 2025-12-28
---

## Schedule

- 09:00 Morning standup

## Intentions

- [x] Complete task

## Wins

- Fixed bug

## Log

- 10:00 Started work
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := WriteJournal(tt.journal)
			if result != tt.expected {
				t.Errorf("WriteJournal() mismatch\nExpected:\n%s\nGot:\n%s", tt.expected, result)
				// Show line-by-line diff for easier debugging
				expectedLines := strings.Split(tt.expected, "\n")
				resultLines := strings.Split(result, "\n")
				maxLines := len(expectedLines)
				if len(resultLines) > maxLines {
					maxLines = len(resultLines)
				}
				for i := 0; i < maxLines; i++ {
					var expLine, resLine string
					if i < len(expectedLines) {
						expLine = expectedLines[i]
					}
					if i < len(resultLines) {
						resLine = resultLines[i]
					}
					if expLine != resLine {
						t.Errorf("Line %d differs:\nExpected: %q\nGot:      %q", i+1, expLine, resLine)
					}
				}
			}
		})
	}
}

// mustParseTime is a helper function to parse time strings in tests
func mustParseTime(t *testing.T, timeStr string) time.Time {
	t.Helper()
	parsedTime, err := time.Parse("2006-01-02 15:04", timeStr)
	if err != nil {
		t.Fatalf("Failed to parse time %s: %v", timeStr, err)
	}
	return parsedTime
}

func TestWriteJournal_ScheduleRoundTrip(t *testing.T) {
	// Test that we can parse a journal with schedule and write it back
	markdown := `---
date: 2023-12-01
---

## Schedule

- 09:00 Morning standup
- 14:00 Client meeting
- Review PR

## Intentions

- [ ] Test intention

## Wins

- Fixed bug

## Log

- 10:00 Started work
`

	filePath := "/test/journal/2023-12-01.md"
	lastModified := time.Now()

	// Parse the journal
	journal, err := ParseJournal(markdown, filePath, lastModified)
	if err != nil {
		t.Fatalf("ParseJournal failed: %v", err)
	}

	// Verify we parsed schedule correctly
	if len(journal.ScheduleItems) != 3 {
		t.Fatalf("Expected 3 schedule items, got %d", len(journal.ScheduleItems))
	}

	// Write it back
	output := WriteJournal(journal)

	// Parse it again to verify round-trip
	journal2, err := ParseJournal(output, filePath, lastModified)
	if err != nil {
		t.Fatalf("Second ParseJournal failed: %v", err)
	}

	// Verify schedule items match
	if len(journal2.ScheduleItems) != len(journal.ScheduleItems) {
		t.Errorf("Schedule item count mismatch after round-trip: expected %d, got %d",
			len(journal.ScheduleItems), len(journal2.ScheduleItems))
	}

	for i := 0; i < len(journal.ScheduleItems) && i < len(journal2.ScheduleItems); i++ {
		orig := journal.ScheduleItems[i]
		roundtrip := journal2.ScheduleItems[i]

		if orig.Content != roundtrip.Content {
			t.Errorf("Schedule item %d content mismatch: expected %q, got %q",
				i, orig.Content, roundtrip.Content)
		}

		// Compare times (allowing for zero times)
		if orig.Time.IsZero() != roundtrip.Time.IsZero() {
			t.Errorf("Schedule item %d time zero status mismatch: orig=%v, roundtrip=%v",
				i, orig.Time.IsZero(), roundtrip.Time.IsZero())
		}

		if !orig.Time.IsZero() && !roundtrip.Time.IsZero() {
			// Compare hours and minutes only since we format as HH:MM
			origHM := orig.Time.Format("15:04")
			roundtripHM := roundtrip.Time.Format("15:04")
			if origHM != roundtripHM {
				t.Errorf("Schedule item %d time mismatch: expected %s, got %s",
					i, origHM, roundtripHM)
			}
		}
	}
}
