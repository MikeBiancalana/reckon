package journal

import (
	"testing"
	"time"
)

func TestParseJournal(t *testing.T) {
	markdown := `---
date: 2023-12-01
---

## Intentions

- [ ] Test intention 1
- [x] Completed intention
- [>] Carried intention (carried from 2023-11-30)

## Wins

- Completed a task
- Fixed a bug

## Log

- 09:00 Started work
- 10:30 [meeting:standup] 30m Meeting notes
- 11:00 [task:abc123] Worked on feature
- 12:00 [break] 45m Lunch break`

	filePath := "/test/journal/2023-12-01.md"
	lastModified := time.Now()

	journal, err := ParseJournal(markdown, filePath, lastModified)
	if err != nil {
		t.Fatalf("ParseJournal failed: %v", err)
	}

	if journal.Date != "2023-12-01" {
		t.Errorf("Expected date 2023-12-01, got %s", journal.Date)
	}
	if journal.FilePath != filePath {
		t.Errorf("Expected filePath %s, got %s", filePath, journal.FilePath)
	}

	// Test intentions
	if len(journal.Intentions) != 3 {
		t.Errorf("Expected 3 intentions, got %d", len(journal.Intentions))
	}
	if journal.Intentions[0].Status != IntentionOpen {
		t.Errorf("Expected first intention open, got %s", journal.Intentions[0].Status)
	}
	if journal.Intentions[1].Status != IntentionDone {
		t.Errorf("Expected second intention done, got %s", journal.Intentions[1].Status)
	}
	if journal.Intentions[2].Status != IntentionCarried {
		t.Errorf("Expected third intention carried, got %s", journal.Intentions[2].Status)
	}
	if journal.Intentions[2].CarriedFrom != "2023-11-30" {
		t.Errorf("Expected carried from 2023-11-30, got %s", journal.Intentions[2].CarriedFrom)
	}

	// Test wins
	if len(journal.Wins) != 2 {
		t.Errorf("Expected 2 wins, got %d", len(journal.Wins))
	}
	if journal.Wins[0].Text != "Completed a task" {
		t.Errorf("Expected win text 'Completed a task', got '%s'", journal.Wins[0].Text)
	}

	// Test log entries
	if len(journal.LogEntries) != 4 {
		t.Errorf("Expected 4 log entries, got %d", len(journal.LogEntries))
	}
	if journal.LogEntries[1].EntryType != EntryTypeMeeting {
		t.Errorf("Expected second entry type meeting, got %s", journal.LogEntries[1].EntryType)
	}
	if journal.LogEntries[1].DurationMinutes != 30 {
		t.Errorf("Expected duration 30, got %d", journal.LogEntries[1].DurationMinutes)
	}
	if journal.LogEntries[2].TaskID != "abc123" {
		t.Errorf("Expected task ID abc123, got %s", journal.LogEntries[2].TaskID)
	}
	if journal.LogEntries[3].EntryType != EntryTypeBreak {
		t.Errorf("Expected fourth entry type break, got %s", journal.LogEntries[3].EntryType)
	}
}

func TestParseIntention(t *testing.T) {
	tests := []struct {
		line     string
		expected *Intention
	}{
		{"- [ ] Open intention", &Intention{Status: IntentionOpen, Text: "Open intention"}},
		{"- [x] Done intention", &Intention{Status: IntentionDone, Text: "Done intention"}},
		{"- [>] Carried (carried from 2023-11-30)", &Intention{Status: IntentionCarried, Text: "Carried", CarriedFrom: "2023-11-30"}},
		{"- Invalid line", nil},
	}

	for _, test := range tests {
		result := parseIntention(test.line, 0)
		if test.expected == nil {
			if result != nil {
				t.Errorf("Expected nil for line '%s', got %+v", test.line, result)
			}
			continue
		}
		if result == nil {
			t.Errorf("Expected intention for line '%s', got nil", test.line)
			continue
		}
		if result.Status != test.expected.Status {
			t.Errorf("Expected status %s, got %s", test.expected.Status, result.Status)
		}
		if result.Text != test.expected.Text {
			t.Errorf("Expected text '%s', got '%s'", test.expected.Text, result.Text)
		}
		if result.CarriedFrom != test.expected.CarriedFrom {
			t.Errorf("Expected carriedFrom '%s', got '%s'", test.expected.CarriedFrom, result.CarriedFrom)
		}
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		content  string
		expected int
	}{
		{"Meeting 30m", 30},
		{"Work 2h", 120},
		{"Break 1h30m", 90},
		{"No duration", 0},
	}

	for _, test := range tests {
		result := parseDuration(test.content)
		if result != test.expected {
			t.Errorf("Expected duration %d for '%s', got %d", test.expected, test.content, result)
		}
	}
}

func TestParseScheduleItem(t *testing.T) {
	date := "2023-12-01"
	tests := []struct {
		name         string
		line         string
		expectNil    bool
		expectTime   string // HH:MM format, empty if no time
		expectContent string
	}{
		{
			name:         "schedule item with time",
			line:         "- 09:00 Morning standup",
			expectNil:    false,
			expectTime:   "09:00",
			expectContent: "Morning standup",
		},
		{
			name:         "schedule item with time and complex content",
			line:         "- 14:00 Client meeting",
			expectNil:    false,
			expectTime:   "14:00",
			expectContent: "Client meeting",
		},
		{
			name:         "schedule item without time",
			line:         "- Review PR",
			expectNil:    false,
			expectTime:   "",
			expectContent: "Review PR",
		},
		{
			name:         "schedule item without time - complex",
			line:         "- Review PR (no time)",
			expectNil:    false,
			expectTime:   "",
			expectContent: "Review PR (no time)",
		},
		{
			name:      "invalid line - no bullet",
			line:      "09:00 Not a list item",
			expectNil: true,
		},
		{
			name:      "invalid line - empty bullet",
			line:      "- ",
			expectNil: true,
		},
		{
			name:      "invalid line - whitespace only bullet",
			line:      "-    ",
			expectNil: true,
		},
		{
			name:          "malformed time - treated as content",
			line:          "- 25:00 Invalid hour",
			expectNil:     false,
			expectTime:    "",
			expectContent: "25:00 Invalid hour",
		},
		{
			name:          "malformed time - invalid minutes - treated as content",
			line:          "- 10:99 Invalid minutes",
			expectNil:     false,
			expectTime:    "",
			expectContent: "10:99 Invalid minutes",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := parseScheduleItem(test.line, date, 0)

			if test.expectNil {
				if result != nil {
					t.Errorf("Expected nil for line '%s', got %+v", test.line, result)
				}
				return
			}

			if result == nil {
				t.Fatalf("Expected schedule item for line '%s', got nil", test.line)
			}

			if result.Content != test.expectContent {
				t.Errorf("Expected content '%s', got '%s'", test.expectContent, result.Content)
			}

			if test.expectTime == "" {
				// Expect zero time
				if !result.Time.IsZero() {
					t.Errorf("Expected zero time for line '%s', got %v", test.line, result.Time)
				}
			} else {
				// Check time matches
				expectedTime, _ := parseTime(date, test.expectTime)
				if !result.Time.Equal(expectedTime) {
					t.Errorf("Expected time %v, got %v", expectedTime, result.Time)
				}
			}
		})
	}
}

func TestParseJournalWithSchedule(t *testing.T) {
	markdown := `---
date: 2023-12-01
---

## Schedule

- 09:00 Morning standup
- 14:00 Client meeting
- Review PR (no time)

## Intentions

- [ ] Test intention 1

## Log

- 09:00 Started work`

	filePath := "/test/journal/2023-12-01.md"
	lastModified := time.Now()

	journal, err := ParseJournal(markdown, filePath, lastModified)
	if err != nil {
		t.Fatalf("ParseJournal failed: %v", err)
	}

	// Test schedule items
	if len(journal.ScheduleItems) != 3 {
		t.Errorf("Expected 3 schedule items, got %d", len(journal.ScheduleItems))
	}

	if len(journal.ScheduleItems) > 0 {
		// First item with time
		if journal.ScheduleItems[0].Content != "Morning standup" {
			t.Errorf("Expected first item content 'Morning standup', got '%s'", journal.ScheduleItems[0].Content)
		}
		expectedTime, _ := parseTime("2023-12-01", "09:00")
		if !journal.ScheduleItems[0].Time.Equal(expectedTime) {
			t.Errorf("Expected first item time %v, got %v", expectedTime, journal.ScheduleItems[0].Time)
		}

		// Third item without time
		if len(journal.ScheduleItems) > 2 {
			if journal.ScheduleItems[2].Content != "Review PR (no time)" {
				t.Errorf("Expected third item content 'Review PR (no time)', got '%s'", journal.ScheduleItems[2].Content)
			}
			if !journal.ScheduleItems[2].Time.IsZero() {
				t.Errorf("Expected third item to have zero time, got %v", journal.ScheduleItems[2].Time)
			}
		}
	}
}

func TestParseJournalWithoutSchedule(t *testing.T) {
	markdown := `---
date: 2023-12-01
---

## Intentions

- [ ] Test intention 1

## Log

- 09:00 Started work`

	filePath := "/test/journal/2023-12-01.md"
	lastModified := time.Now()

	journal, err := ParseJournal(markdown, filePath, lastModified)
	if err != nil {
		t.Fatalf("ParseJournal failed: %v", err)
	}

	// Should have empty schedule items, not nil
	if journal.ScheduleItems == nil {
		t.Error("Expected ScheduleItems to be initialized, got nil")
	}
	if len(journal.ScheduleItems) != 0 {
		t.Errorf("Expected 0 schedule items, got %d", len(journal.ScheduleItems))
	}
}

func TestParseJournalEmptySchedule(t *testing.T) {
	markdown := `---
date: 2023-12-01
---

## Schedule

## Intentions

- [ ] Test intention 1`

	filePath := "/test/journal/2023-12-01.md"
	lastModified := time.Now()

	journal, err := ParseJournal(markdown, filePath, lastModified)
	if err != nil {
		t.Fatalf("ParseJournal failed: %v", err)
	}

	// Empty schedule section should result in 0 items
	if len(journal.ScheduleItems) != 0 {
		t.Errorf("Expected 0 schedule items for empty section, got %d", len(journal.ScheduleItems))
	}
}
