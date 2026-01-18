package journal

import (
	"testing"
	"time"
)

// TestLogNotesRoundTrip tests parsing and writing log notes with various formats
func TestLogNotesRoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
	}{
		{
			name: "log entry with notes using ID format",
			markdown: `---
date: 2023-12-01
---

## Schedule

## Intentions

## Wins

## Log

- 09:00 Started work
  - note-1 First note on work
  - note-2 Second note on work
`,
		},
		{
			name: "log entry with note without ID",
			markdown: `---
date: 2023-12-01
---

## Schedule

## Intentions

## Wins

## Log

- 09:00 Started work
  - This is a note without explicit ID
`,
		},
		{
			name: "multiple log entries with mixed notes",
			markdown: `---
date: 2023-12-01
---

## Schedule

## Intentions

## Wins

## Log

- 09:00 Started work
  - note-1 First note
- 10:30 [meeting:standup] 30m Meeting notes
  - meeting-note Discussed project timeline
- 11:00 [task:abc123] Worked on feature
- 12:00 [break] 45m Lunch break
`,
		},
		{
			name: "log entry with empty notes section (no notes)",
			markdown: `---
date: 2023-12-01
---

## Schedule

## Intentions

## Wins

## Log

- 09:00 Started work
- 10:00 Continued work
`,
		},
		{
			name: "log entry with special characters in notes",
			markdown: `---
date: 2023-12-01
---

## Schedule

## Intentions

## Wins

## Log

- 09:00 Started work
  - note-1 This note has [brackets] and "quotes"
  - note-2 This note has special chars: @#$%
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := "/test/journal/2023-12-01.md"
			lastModified := time.Now()

			// First parse
			journal1, err := ParseJournal(tt.markdown, filePath, lastModified)
			if err != nil {
				t.Fatalf("First ParseJournal failed: %v", err)
			}

			// Write back to markdown
			output := WriteJournal(journal1)

			// Parse again
			journal2, err := ParseJournal(output, filePath, lastModified)
			if err != nil {
				t.Fatalf("Second ParseJournal failed: %v", err)
			}

			// Compare log entries count
			if len(journal1.LogEntries) != len(journal2.LogEntries) {
				t.Errorf("Log entry count mismatch: expected %d, got %d",
					len(journal1.LogEntries), len(journal2.LogEntries))
				return
			}

			// Compare each log entry's notes
			for i := 0; i < len(journal1.LogEntries); i++ {
				entry1 := journal1.LogEntries[i]
				entry2 := journal2.LogEntries[i]

				// Compare notes count
				if len(entry1.Notes) != len(entry2.Notes) {
					t.Errorf("Entry %d: note count mismatch: expected %d, got %d",
						i, len(entry1.Notes), len(entry2.Notes))
					continue
				}

				// Compare each note - text should always match
				for j := 0; j < len(entry1.Notes); j++ {
					note1 := entry1.Notes[j]
					note2 := entry2.Notes[j]

					if note1.Text != note2.Text {
						t.Errorf("Entry %d, Note %d: text mismatch:\nExpected: %q\nGot:      %q",
							i, j, note1.Text, note2.Text)
					}
				}
			}

			// For a better idempotency test: parse → write → parse → write
			// and compare the final two string outputs
			output2 := WriteJournal(journal2)
			if output != output2 {
				t.Errorf("Round-trip not idempotent:\nFirst write:\n%s\n\nSecond write:\n%s",
					output, output2)
			}
		})
	}
}

// TestLogNotesParsing tests specific parsing scenarios for log notes
func TestLogNotesParsing(t *testing.T) {
	tests := []struct {
		name          string
		markdown      string
		expectEntries int
		expectNotes   []int // notes per entry
		checkNote     func(t *testing.T, entry LogEntry, noteIdx int)
	}{
		{
			name: "parse note with explicit ID",
			markdown: `---
date: 2023-12-01
---

## Log

- 09:00 Started work
  - note-1 First note
`,
			expectEntries: 1,
			expectNotes:   []int{1},
			checkNote: func(t *testing.T, entry LogEntry, noteIdx int) {
				// IDs are now generated, not extracted from markdown
				// The "note-1" prefix becomes part of the text
				if entry.Notes[0].ID == "" {
					t.Error("Expected generated ID, got empty string")
				}
				if entry.Notes[0].Text != "note-1 First note" {
					t.Errorf("Expected note text 'note-1 First note', got '%s'", entry.Notes[0].Text)
				}
			},
		},
		{
			name: "parse note without ID generates ID",
			markdown: `---
date: 2023-12-01
---

## Log

- 09:00 Started work
  - This is a note without ID
`,
			expectEntries: 1,
			expectNotes:   []int{1},
			checkNote: func(t *testing.T, entry LogEntry, noteIdx int) {
				if entry.Notes[0].ID == "" {
					t.Error("Expected generated ID, got empty string")
				}
				if entry.Notes[0].Text != "This is a note without ID" {
					t.Errorf("Expected note text 'This is a note without ID', got '%s'", entry.Notes[0].Text)
				}
			},
		},
		{
			name: "parse multiple notes with mixed IDs",
			markdown: `---
date: 2023-12-01
---

## Log

- 09:00 Started work
  - note-1 First note
  - Second note without ID
  - note-3 Third note
`,
			expectEntries: 1,
			expectNotes:   []int{3},
			checkNote: func(t *testing.T, entry LogEntry, noteIdx int) {
				// Check all three notes
				if len(entry.Notes) != 3 {
					t.Fatalf("Expected 3 notes, got %d", len(entry.Notes))
				}
				// All IDs are now generated, the "ID prefix" becomes part of text
				if entry.Notes[0].ID == "" {
					t.Error("Expected generated ID for first note")
				}
				if entry.Notes[0].Text != "note-1 First note" {
					t.Errorf("Expected first note text 'note-1 First note', got '%s'", entry.Notes[0].Text)
				}
				if entry.Notes[1].ID == "" {
					t.Error("Expected generated ID for second note")
				}
				if entry.Notes[1].Text != "Second note without ID" {
					t.Errorf("Expected second note text 'Second note without ID', got '%s'", entry.Notes[1].Text)
				}
				if entry.Notes[2].ID == "" {
					t.Error("Expected generated ID for third note")
				}
				if entry.Notes[2].Text != "note-3 Third note" {
					t.Errorf("Expected third note text 'note-3 Third note', got '%s'", entry.Notes[2].Text)
				}
			},
		},
		{
			name: "parse notes across multiple log entries",
			markdown: `---
date: 2023-12-01
---

## Log

- 09:00 Started work
  - note-1 First log note
- 10:00 Continued work
  - note-2 Second log note
- 11:00 Finished work
`,
			expectEntries: 3,
			expectNotes:   []int{1, 1, 0},
			checkNote: func(t *testing.T, entry LogEntry, noteIdx int) {
				// This will be called for the first entry only
			},
		},
		{
			name: "parse note with 3+ space indentation",
			markdown: `---
date: 2023-12-01
---

## Log

- 09:00 Started work
   - note-1 Note with 3 spaces
`,
			expectEntries: 1,
			expectNotes:   []int{1},
			checkNote: func(t *testing.T, entry LogEntry, noteIdx int) {
				// Note: the "note-1" prefix is now part of the text
				if entry.Notes[0].Text != "note-1 Note with 3 spaces" {
					t.Errorf("Expected note text 'note-1 Note with 3 spaces', got '%s'", entry.Notes[0].Text)
				}
			},
		},
		{
			name: "parse note with tab indentation",
			markdown: `---
date: 2023-12-01
---

## Log

- 09:00 Started work
	- note-1 Note with tab
`,
			expectEntries: 1,
			expectNotes:   []int{1},
			checkNote: func(t *testing.T, entry LogEntry, noteIdx int) {
				// Note: the "note-1" prefix is now part of the text
				if entry.Notes[0].Text != "note-1 Note with tab" {
					t.Errorf("Expected note text 'note-1 Note with tab', got '%s'", entry.Notes[0].Text)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := "/test/journal/2023-12-01.md"
			lastModified := time.Now()

			journal, err := ParseJournal(tt.markdown, filePath, lastModified)
			if err != nil {
				t.Fatalf("ParseJournal failed: %v", err)
			}

			if len(journal.LogEntries) != tt.expectEntries {
				t.Fatalf("Expected %d log entries, got %d", tt.expectEntries, len(journal.LogEntries))
			}

			for i, expectNoteCount := range tt.expectNotes {
				if i >= len(journal.LogEntries) {
					break
				}
				entry := journal.LogEntries[i]
				if len(entry.Notes) != expectNoteCount {
					t.Errorf("Entry %d: expected %d notes, got %d", i, expectNoteCount, len(entry.Notes))
				}
			}

			if tt.checkNote != nil && len(journal.LogEntries) > 0 {
				tt.checkNote(t, journal.LogEntries[0], 0)
			}
		})
	}
}

// TestLogNotesPositionTracking tests that note positions are tracked correctly
func TestLogNotesPositionTracking(t *testing.T) {
	markdown := `---
date: 2023-12-01
---

## Log

- 09:00 Started work
  - note-1 First note
  - note-2 Second note
  - note-3 Third note
`

	filePath := "/test/journal/2023-12-01.md"
	lastModified := time.Now()

	journal, err := ParseJournal(markdown, filePath, lastModified)
	if err != nil {
		t.Fatalf("ParseJournal failed: %v", err)
	}

	if len(journal.LogEntries) != 1 {
		t.Fatalf("Expected 1 log entry, got %d", len(journal.LogEntries))
	}

	entry := journal.LogEntries[0]
	if len(entry.Notes) != 3 {
		t.Fatalf("Expected 3 notes, got %d", len(entry.Notes))
	}

	// Check positions are sequential
	for i, note := range entry.Notes {
		if note.Position != i {
			t.Errorf("Note %d: expected position %d, got %d", i, i, note.Position)
		}
	}
}

// TestLogNotesEdgeCases tests edge cases for log note parsing
func TestLogNotesEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		markdown    string
		expectError bool
		checkResult func(t *testing.T, journal *Journal)
	}{
		{
			name: "note before any log entry is ignored",
			markdown: `---
date: 2023-12-01
---

## Log

  - orphan-note This note has no parent log entry
- 09:00 Started work
`,
			expectError: false,
			checkResult: func(t *testing.T, journal *Journal) {
				if len(journal.LogEntries) != 1 {
					t.Errorf("Expected 1 log entry, got %d", len(journal.LogEntries))
				}
				if len(journal.LogEntries) > 0 && len(journal.LogEntries[0].Notes) != 0 {
					t.Errorf("Expected 0 notes, got %d", len(journal.LogEntries[0].Notes))
				}
			},
		},
		{
			name: "empty note line",
			markdown: `---
date: 2023-12-01
---

## Log

- 09:00 Started work
  -
`,
			expectError: false,
			checkResult: func(t *testing.T, journal *Journal) {
				// Empty notes should be ignored
				if len(journal.LogEntries) != 1 {
					t.Errorf("Expected 1 log entry, got %d", len(journal.LogEntries))
				}
			},
		},
		{
			name: "note with only ID is NOT skipped (ID becomes text)",
			markdown: `---
date: 2023-12-01
---

## Log

- 09:00 Started work
  - note-1
`,
			expectError: false,
			checkResult: func(t *testing.T, journal *Journal) {
				if len(journal.LogEntries) != 1 {
					t.Fatalf("Expected 1 log entry, got %d", len(journal.LogEntries))
				}
				// Notes with only "note-1" now have text "note-1" (ID is generated, prefix becomes text)
				// So they are NOT skipped
				if len(journal.LogEntries[0].Notes) != 1 {
					t.Errorf("Expected 1 note (note-1 becomes text), got %d", len(journal.LogEntries[0].Notes))
				}
			},
		},
		{
			name: "note with only whitespace after dash is skipped",
			markdown: `---
date: 2023-12-01
---

## Log

- 09:00 Started work
  -
`,
			expectError: false,
			checkResult: func(t *testing.T, journal *Journal) {
				if len(journal.LogEntries) != 1 {
					t.Fatalf("Expected 1 log entry, got %d", len(journal.LogEntries))
				}
				// Whitespace-only notes should be skipped
				if len(journal.LogEntries[0].Notes) != 0 {
					t.Errorf("Expected 0 notes (whitespace-only notes should be skipped), got %d", len(journal.LogEntries[0].Notes))
				}
			},
		},
		{
			name: "note with malformed ID (dash prefix)",
			markdown: `---
date: 2023-12-01
---

## Log

- 09:00 Started work
  - -note text here
`,
			expectError: false,
			checkResult: func(t *testing.T, journal *Journal) {
				if len(journal.LogEntries) != 1 {
					t.Fatalf("Expected 1 log entry, got %d", len(journal.LogEntries))
				}
				if len(journal.LogEntries[0].Notes) != 1 {
					t.Fatalf("Expected 1 note, got %d", len(journal.LogEntries[0].Notes))
				}
				note := journal.LogEntries[0].Notes[0]
				// Should treat entire string as text, not extract invalid ID
				if note.Text != "-note text here" {
					t.Errorf("Expected text '-note text here', got '%s'", note.Text)
				}
			},
		},
		{
			name: "note with malformed ID (dash suffix)",
			markdown: `---
date: 2023-12-01
---

## Log

- 09:00 Started work
  - note- text here
`,
			expectError: false,
			checkResult: func(t *testing.T, journal *Journal) {
				if len(journal.LogEntries) != 1 {
					t.Fatalf("Expected 1 log entry, got %d", len(journal.LogEntries))
				}
				if len(journal.LogEntries[0].Notes) != 1 {
					t.Fatalf("Expected 1 note, got %d", len(journal.LogEntries[0].Notes))
				}
				note := journal.LogEntries[0].Notes[0]
				// Should treat entire string as text, not extract invalid ID
				if note.Text != "note- text here" {
					t.Errorf("Expected text 'note- text here', got '%s'", note.Text)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := "/test/journal/2023-12-01.md"
			lastModified := time.Now()

			journal, err := ParseJournal(tt.markdown, filePath, lastModified)

			if tt.expectError && err == nil {
				t.Fatal("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if tt.checkResult != nil && journal != nil {
				tt.checkResult(t, journal)
			}
		})
	}
}
