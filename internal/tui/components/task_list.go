package components

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	taskStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39"))

	taskDoneStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Strikethrough(true)

	overdueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	dueTodayStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("226"))

	dueSoonStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214"))

	scheduledStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	dateInfoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))
)

// TaskToggleMsg is sent when a task's status is toggled
type TaskToggleMsg struct {
	TaskID string
}

// TaskNoteDeleteMsg is sent when a task note should be deleted
type TaskNoteDeleteMsg struct {
	TaskID string
	NoteID string
}

// DateInfo carries the two schedule-related date fields task_list's date
// formatting/styling helpers need. It decouples this package from
// internal/journal (Decision 1, reckon-fnqs.8): FormatDateInfo/GetDateStyle
// only ever touch these two *string fields, never anything else on
// journal.Task.
type DateInfo struct {
	ScheduledDate *string
	DeadlineDate  *string
}

// FormatDateInfo returns a formatted date string for a task's schedule/deadline dates.
func FormatDateInfo(info DateInfo) string { return formatDateInfo(info) }

func localToday() time.Time {
	now := time.Now()
	y, m, d := now.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, now.Location())
}

func formatDateInfo(info DateInfo) string {
	today := localToday()
	var parts []string

	if scheduledDate, ok := parseDate(info.ScheduledDate); ok {
		dateStr := formatFriendlyDate(scheduledDate, today)
		parts = append(parts, "📅 "+dateStr)
	}

	if deadlineDate, ok := parseDate(info.DeadlineDate); ok {
		dateStr := formatFriendlyDate(deadlineDate, today)
		daysUntil := int(deadlineDate.Sub(today).Hours() / 24)

		if daysUntil < 0 {
			parts = append(parts, "🔴 overdue (due "+dateStr+")")
		} else if daysUntil == 0 {
			parts = append(parts, "due today 🟡")
		} else if daysUntil <= 2 {
			parts = append(parts, "due "+dateStr+" 🟡")
		} else {
			parts = append(parts, "due "+dateStr)
		}
	}

	return strings.Join(parts, "  ")
}

func formatFriendlyDate(t time.Time, today time.Time) string {
	diff := t.Sub(today)

	switch {
	case diff == 0:
		return "today"
	case diff == 24*time.Hour:
		return "tomorrow"
	case t.Year() == today.Year():
		return t.Format("Jan 2")
	default:
		return t.Format("Jan 2, 2006")
	}
}

// GetDateStyle returns the appropriate lipgloss style for a task based on its urgency.
func GetDateStyle(info DateInfo) lipgloss.Style { return getDateStyle(info) }

func getDateStyle(info DateInfo) lipgloss.Style {
	today := localToday()

	if deadlineDate, ok := parseDate(info.DeadlineDate); ok {
		daysUntil := int(deadlineDate.Sub(today).Hours() / 24)

		if daysUntil < 0 {
			return overdueStyle
		}
		if daysUntil == 0 {
			return dueTodayStyle
		}
		if daysUntil <= 2 {
			return dueSoonStyle
		}
	}

	if scheduledDate, ok := parseDate(info.ScheduledDate); ok {
		daysUntil := int(scheduledDate.Sub(today).Hours() / 24)
		if daysUntil >= 0 && daysUntil <= 7 {
			return scheduledStyle
		}
	}

	return dateInfoStyle
}

func parseDate(dateStr *string) (time.Time, bool) {
	if dateStr == nil || *dateStr == "" {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation("2006-01-02", *dateStr, time.Local)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
