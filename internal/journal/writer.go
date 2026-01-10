package journal

import (
	"fmt"
	"sort"
	"strings"
)

// WriteJournal serializes a Journal object to markdown format
func WriteJournal(j *Journal) string {
	var sb strings.Builder

	// Write frontmatter
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("date: %s\n", j.Date))
	sb.WriteString("---\n\n")

	// Write Schedule section
	sb.WriteString("## Schedule\n\n")
	if len(j.ScheduleItems) > 0 {
		sortedSchedule := make([]ScheduleItem, len(j.ScheduleItems))
		copy(sortedSchedule, j.ScheduleItems)
		sort.Slice(sortedSchedule, func(i, j int) bool {
			return sortedSchedule[i].Position < sortedSchedule[j].Position
		})

		for _, item := range sortedSchedule {
			if !item.Time.IsZero() {
				timeStr := item.Time.Format("15:04")
				sb.WriteString(fmt.Sprintf("- %s %s\n", timeStr, item.Content))
			} else {
				sb.WriteString(fmt.Sprintf("- %s\n", item.Content))
			}
		}
	}
	sb.WriteString("\n")

	// Write Intentions section
	sb.WriteString("## Intentions\n\n")
	if len(j.Intentions) > 0 {
		// Sort by position
		sortedIntentions := make([]Intention, len(j.Intentions))
		copy(sortedIntentions, j.Intentions)
		sort.Slice(sortedIntentions, func(i, j int) bool {
			return sortedIntentions[i].Position < sortedIntentions[j].Position
		})

		for _, intention := range sortedIntentions {
			marker := "[ ]"
			switch intention.Status {
			case IntentionDone:
				marker = "[x]"
			case IntentionCarried:
				marker = "[>]"
			}

			text := intention.Text
			if intention.CarriedFrom != "" {
				text = fmt.Sprintf("%s (carried from %s)", text, intention.CarriedFrom)
			}

			sb.WriteString(fmt.Sprintf("- %s %s\n", marker, text))
		}
	}
	sb.WriteString("\n")

	// Write Wins section
	sb.WriteString("## Wins\n\n")
	if len(j.Wins) > 0 {
		// Sort by position
		sortedWins := make([]Win, len(j.Wins))
		copy(sortedWins, j.Wins)
		sort.Slice(sortedWins, func(i, j int) bool {
			return sortedWins[i].Position < sortedWins[j].Position
		})

		for _, win := range sortedWins {
			sb.WriteString(fmt.Sprintf("- %s\n", win.Text))
		}
	}
	sb.WriteString("\n")

	// Write Log section
	sb.WriteString("## Log\n\n")
	if len(j.LogEntries) > 0 {
		// Sort by position (which should correspond to timestamp order)
		sortedEntries := make([]LogEntry, len(j.LogEntries))
		copy(sortedEntries, j.LogEntries)
		sort.Slice(sortedEntries, func(i, j int) bool {
			return sortedEntries[i].Position < sortedEntries[j].Position
		})

		for _, entry := range sortedEntries {
			timeStr := entry.Timestamp.Format("15:04")
			content := entry.Content

			// Add duration if present
			if entry.DurationMinutes > 0 {
				duration := formatDuration(entry.DurationMinutes)
				// Check if duration is already in content
				if !strings.Contains(content, duration) {
					content = fmt.Sprintf("%s %s", content, duration)
				}
			}

			sb.WriteString(fmt.Sprintf("- %s %s\n", timeStr, content))

			// Write notes if present
			if len(entry.Notes) > 0 {
				// Sort notes by position
				sortedNotes := make([]LogNote, len(entry.Notes))
				copy(sortedNotes, entry.Notes)
				sort.Slice(sortedNotes, func(i, j int) bool {
					return sortedNotes[i].Position < sortedNotes[j].Position
				})

				for _, note := range sortedNotes {
					sb.WriteString(fmt.Sprintf("  - %s %s\n", note.ID, note.Text))
				}
			}
		}
	}

	return sb.String()
}

// formatDuration converts minutes to a human-readable duration string
func formatDuration(minutes int) string {
	if minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	}

	hours := minutes / 60
	mins := minutes % 60

	if mins == 0 {
		return fmt.Sprintf("%dh", hours)
	}

	return fmt.Sprintf("%dh%dm", hours, mins)
}
