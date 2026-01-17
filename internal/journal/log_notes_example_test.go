package journal

import (
	"fmt"
	"time"
)

// Example_logNotes demonstrates parsing and writing log notes in journal markdown
func Example_logNotes() {
	// Sample journal with log entries and nested note bullets
	markdown := `---
date: 2023-12-01
---

## Schedule

## Intentions

- [ ] Complete project documentation

## Wins

## Log

- 09:00 Started work on feature [task:xyz123]
  - note-1 Reviewed existing implementation
  - note-2 Identified key refactoring opportunities
- 10:30 [meeting:standup] 15m Daily standup meeting
  - meeting-note Discussed deployment timeline
  - meeting-note-2 Action items assigned to team
- 12:00 [break] 1h Lunch break
- 13:00 Resumed work on feature
  - Generated note without explicit ID
  - Another generated note
`

	// Parse the journal
	journal, err := ParseJournal(markdown, "/journals/2023-12-01.md", time.Now())
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Display parsed log entries and their notes
	fmt.Println("Parsed Log Entries:")
	for i, entry := range journal.LogEntries {
		fmt.Printf("Entry %d: %s - %s\n", i+1, entry.Timestamp.Format("15:04"), entry.Content)
		for j, note := range entry.Notes {
			// Mask generated IDs (20+ chars) as [...] for consistent output
			displayID := note.ID
			if len(note.ID) > 15 {
				displayID = "..."
			}
			fmt.Printf("  Note %d: [%s] %s\n", j+1, displayID, note.Text)
		}
	}

	fmt.Println("\nWritten Markdown:")
	// Write back to markdown
	output := WriteJournal(journal)

	// Show just the Log section, masking generated IDs
	lines := splitLines(output)
	inLog := false
	for _, line := range lines {
		if line == "## Log" {
			inLog = true
		}
		if inLog {
			// Mask generated IDs in log entry lines (format: "- HH:MM <ID> <content>")
			if len(line) > 2 && line[0:2] == "- " {
				// This is a log entry line
				parts := splitOnce(line[2:], " ") // Split after "- "
				if len(parts) == 2 {
					// parts[0] is the timestamp, parts[1] is "<ID> <content>"
					restParts := splitOnce(parts[1], " ") // Split "<ID> <content>"
					if len(restParts) == 2 && len(restParts[0]) > 15 {
						// Mask the ID
						line = "- " + parts[0] + " " + restParts[1]
					}
				}
			}
			// Mask generated IDs in note lines
			if len(line) > 4 && line[0:4] == "  - " {
				// This is a note line - check if it has a long ID
				parts := splitOnce(line[4:], " ")
				if len(parts) == 2 && len(parts[0]) > 15 {
					line = "  - ... " + parts[1]
				}
			}
			fmt.Println(line)
		}
	}

	// Output:
	// Parsed Log Entries:
	// Entry 1: 09:00 - Started work on feature [task:xyz123]
	//   Note 1: [note-1] Reviewed existing implementation
	//   Note 2: [note-2] Identified key refactoring opportunities
	// Entry 2: 10:30 - [meeting:standup] 15m Daily standup meeting
	//   Note 1: [meeting-note] Discussed deployment timeline
	//   Note 2: [meeting-note-2] Action items assigned to team
	// Entry 3: 12:00 - [break] 1h Lunch break
	// Entry 4: 13:00 - Resumed work on feature
	//   Note 1: [...] Generated note without explicit ID
	//   Note 2: [...] Another generated note
	//
	// Written Markdown:
	// ## Log
	//
	// - 09:00 Started work on feature [task:xyz123]
	//   - note-1 Reviewed existing implementation
	//   - note-2 Identified key refactoring opportunities
	// - 10:30 [meeting:standup] 15m Daily standup meeting
	//   - meeting-note Discussed deployment timeline
	//   - meeting-note-2 Action items assigned to team
	// - 12:00 [break] 1h Lunch break
	// - 13:00 Resumed work on feature
	//   - ... Generated note without explicit ID
	//   - ... Another generated note
}

func splitLines(s string) []string {
	result := []string{}
	start := 0
	for i, r := range s {
		if r == '\n' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		result = append(result, s[start:])
	}
	return result
}

func splitOnce(s string, sep string) []string {
	idx := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep[0] {
			idx = i
			break
		}
	}
	if idx == 0 && s[0] != sep[0] {
		return []string{s}
	}
	return []string{s[:idx], s[idx+1:]}
}
