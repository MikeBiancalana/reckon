package journal

import (
	"fmt"
	"time"
)

// Example_logNotes demonstrates parsing and writing log notes in journal markdown
func Example_logNotes() {
	// Sample journal with log entries and nested note bullets
	// Note: IDs are no longer written to markdown - they're generated from position
	markdown := `---
date: 2023-12-01
---

## Schedule

## Intentions

- [ ] Complete project documentation

## Wins

## Log

- 09:00 Started work on feature [task:xyz123]
  - Reviewed existing implementation
  - Identified key refactoring opportunities
- 10:30 [meeting:standup] 15m Daily standup meeting
  - Discussed deployment timeline
  - Action items assigned to team
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
			// IDs are now position-based (date:entry_pos:note_pos)
			fmt.Printf("  Note %d: [%s] %s\n", j+1, note.ID, note.Text)
		}
	}

	fmt.Println("\nWritten Markdown:")
	// Write back to markdown - note that IDs are NOT included in output
	output := WriteJournal(journal)

	// Show just the Log section
	lines := splitLines(output)
	inLog := false
	for _, line := range lines {
		if line == "## Log" {
			inLog = true
		}
		if inLog {
			fmt.Println(line)
		}
	}

	// Output:
	// Parsed Log Entries:
	// Entry 1: 09:00 - Started work on feature [task:xyz123]
	//   Note 1: [2023-12-01:0:0] Reviewed existing implementation
	//   Note 2: [2023-12-01:0:1] Identified key refactoring opportunities
	// Entry 2: 10:30 - [meeting:standup] 15m Daily standup meeting
	//   Note 1: [2023-12-01:1:0] Discussed deployment timeline
	//   Note 2: [2023-12-01:1:1] Action items assigned to team
	// Entry 3: 12:00 - [break] 1h Lunch break
	// Entry 4: 13:00 - Resumed work on feature
	//   Note 1: [2023-12-01:3:0] Generated note without explicit ID
	//   Note 2: [2023-12-01:3:1] Another generated note
	//
	// Written Markdown:
	// ## Log
	//
	// - 09:00 Started work on feature [task:xyz123]
	//   - Reviewed existing implementation
	//   - Identified key refactoring opportunities
	// - 10:30 [meeting:standup] 15m Daily standup meeting
	//   - Discussed deployment timeline
	//   - Action items assigned to team
	// - 12:00 [break] 1h Lunch break
	// - 13:00 Resumed work on feature
	//   - Generated note without explicit ID
	//   - Another generated note
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
