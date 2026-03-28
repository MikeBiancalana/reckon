package components

import (
	"strings"
	"time"

	"github.com/MikeBiancalana/reckon/internal/journal"
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

// FormatDateInfo returns a formatted date string for a task's schedule/deadline dates.
func FormatDateInfo(task journal.Task) string { return formatDateInfo(task) }

func formatDateInfo(task journal.Task) string {
	today := time.Now().Truncate(24 * time.Hour)
	var parts []string

	if scheduledDate, ok := parseDate(task.ScheduledDate); ok {
		dateStr := formatFriendlyDate(scheduledDate, today)
		parts = append(parts, "📅 "+dateStr)
	}

	if deadlineDate, ok := parseDate(task.DeadlineDate); ok {
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
func GetDateStyle(task journal.Task) lipgloss.Style { return getDateStyle(task) }

func getDateStyle(task journal.Task) lipgloss.Style {
	today := time.Now().Truncate(24 * time.Hour)

	if deadlineDate, ok := parseDate(task.DeadlineDate); ok {
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

	if scheduledDate, ok := parseDate(task.ScheduledDate); ok {
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
	t, err := time.Parse("2006-01-02", *dateStr)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
