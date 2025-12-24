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
