package time

import (
	"strconv"
	"time"

	"github.com/MikeBiancalana/reckon/internal/journal"
)

type Category string

const (
	CategoryMeeting Category = "meetings"
	CategoryBreak   Category = "breaks"
	CategoryTask    Category = "tasks"
	CategoryLog     Category = "untracked"
)

const maxTaskDurationMinutes = 10 * 60 // 10 hours max

type TimeSummary struct {
	Meetings     int // minutes
	Breaks       int // minutes
	Tasks        int // minutes
	Untracked    int // minutes
	TotalTracked int // minutes
}

func (s *TimeSummary) Add(other TimeSummary) {
	s.Meetings += other.Meetings
	s.Breaks += other.Breaks
	s.Tasks += other.Tasks
	s.Untracked += other.Untracked
	s.TotalTracked += other.TotalTracked
}

func (s *TimeSummary) MeetingsFormatted() string {
	return formatDuration(s.Meetings)
}

func (s *TimeSummary) BreaksFormatted() string {
	return formatDuration(s.Breaks)
}

func (s *TimeSummary) TasksFormatted() string {
	return formatDuration(s.Tasks)
}

func (s *TimeSummary) UntrackedFormatted() string {
	return formatDuration(s.Untracked)
}

func (s *TimeSummary) TotalTrackedFormatted() string {
	return formatDuration(s.TotalTracked)
}

func formatDuration(minutes int) string {
	if minutes < 0 {
		minutes = 0
	}
	hours := minutes / 60
	mins := minutes % 60
	if hours > 0 {
		return formatDurationHours(hours, mins)
	}
	return formatDurationMins(mins)
}

func formatDurationHours(hours, minutes int) string {
	if minutes == 0 {
		return formatDurationMins(hours * 60)
	}
	return formatDurationMins(hours*60 + minutes)
}

func formatDurationMins(minutes int) string {
	if minutes < 60 {
		return formatSingularOrPlural(minutes, "minute")
	}
	hours := minutes / 60
	mins := minutes % 60
	if mins == 0 {
		return formatSingularOrPlural(hours, "hour")
	}
	return formatSingularOrPlural(hours, "hour") + " " + formatSingularOrPlural(mins, "minute")
}

func formatSingularOrPlural(count int, unit string) string {
	if count == 1 {
		return "1 " + unit
	}
	return strconv.Itoa(count) + " " + unit + "s"
}

func CalculateDaySummary(journal *journal.Journal) TimeSummary {
	summary := TimeSummary{}

	if journal == nil || len(journal.LogEntries) == 0 {
		return summary
	}

	for i, entry := range journal.LogEntries {
		duration := entry.DurationMinutes

		if duration > 0 {
			switch entry.EntryType {
			case "meeting":
				summary.Meetings += duration
			case "break":
				summary.Breaks += duration
			default:
				if entry.TaskID != "" {
					summary.Tasks += duration
				} else {
					summary.Untracked += duration
				}
			}
			summary.TotalTracked += duration
		} else if i > 0 {
			prevEntry := journal.LogEntries[i-1]
			if prevEntry.TaskID != "" && entry.TaskID == "" {
				elapsed := int(entry.Timestamp.Sub(prevEntry.Timestamp).Minutes())
				if elapsed > 0 && elapsed < maxTaskDurationMinutes {
					summary.Tasks += elapsed
					summary.TotalTracked += elapsed
				}
			}
		}
	}

	return summary
}

func CalculateWeekSummary(journals map[string]*journal.Journal) TimeSummary {
	var summary TimeSummary
	for _, j := range journals {
		summary.Add(CalculateDaySummary(j))
	}
	return summary
}

func GetTaskDuration(journal *journal.Journal, taskID string) int {
	total := 0
	for i, entry := range journal.LogEntries {
		if entry.TaskID == taskID {
			if entry.DurationMinutes > 0 {
				total += entry.DurationMinutes
			} else if i < len(journal.LogEntries)-1 {
				nextEntry := journal.LogEntries[i+1]
				if nextEntry.TaskID == "" && nextEntry.Timestamp.After(entry.Timestamp) {
					elapsed := int(nextEntry.Timestamp.Sub(entry.Timestamp).Minutes())
					if elapsed > 0 && elapsed < maxTaskDurationMinutes {
						total += elapsed
					}
				}
			}
		}
	}
	return total
}

func IsWithinDay(entryTime, startOfDay, endOfDay time.Time) bool {
	return entryTime.After(startOfDay) && entryTime.Before(endOfDay)
}
