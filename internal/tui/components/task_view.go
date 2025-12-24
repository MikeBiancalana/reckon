package components

import (
	"fmt"
	"strings"

	"github.com/MikeBiancalana/reckon/internal/task"
	"github.com/charmbracelet/lipgloss"
)

var (
	taskTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("12"))

	taskMetaStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	taskTagStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("6"))

	taskSectionStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("11"))

	taskLogTimeStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("10"))

	taskLogContentStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("7"))

	taskEmptyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Italic(true)
)

// TaskView displays a task's details
type TaskView struct {
	task   *task.Task
	width  int
	height int
}

// NewTaskView creates a new task view
func NewTaskView(t *task.Task) *TaskView {
	return &TaskView{
		task: t,
	}
}

// SetTask updates the displayed task
func (v *TaskView) SetTask(t *task.Task) {
	v.task = t
}

// SetSize updates the view dimensions
func (v *TaskView) SetSize(width, height int) {
	v.width = width
	v.height = height
}

// View renders the task view
func (v *TaskView) View() string {
	if v.task == nil {
		return taskEmptyStyle.Render("No task selected\n\nPress Ctrl+T to select a task")
	}

	var sb strings.Builder

	// Title
	sb.WriteString(taskTitleStyle.Render(v.task.Title))
	sb.WriteString("\n")

	// Metadata
	meta := fmt.Sprintf("ID: %s | Status: %s | Created: %s",
		v.task.ID, v.task.Status, v.task.Created)
	sb.WriteString(taskMetaStyle.Render(meta))
	sb.WriteString("\n")

	// Tags
	if len(v.task.Tags) > 0 {
		tagsStr := fmt.Sprintf("Tags: %s", strings.Join(v.task.Tags, ", "))
		sb.WriteString(taskTagStyle.Render(tagsStr))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")

	// Description section
	sb.WriteString(taskSectionStyle.Render("Description"))
	sb.WriteString("\n")
	if v.task.Description != "" {
		sb.WriteString(v.task.Description)
	} else {
		sb.WriteString(taskEmptyStyle.Render("(no description)"))
	}
	sb.WriteString("\n\n")

	// Log section
	sb.WriteString(taskSectionStyle.Render("Log"))
	sb.WriteString("\n")

	if len(v.task.LogEntries) == 0 {
		sb.WriteString(taskEmptyStyle.Render("(no log entries)"))
	} else {
		// Group entries by date
		entryMap := make(map[string][]task.LogEntry)
		dates := make([]string, 0)

		for _, entry := range v.task.LogEntries {
			if _, exists := entryMap[entry.Date]; !exists {
				dates = append(dates, entry.Date)
				entryMap[entry.Date] = make([]task.LogEntry, 0)
			}
			entryMap[entry.Date] = append(entryMap[entry.Date], entry)
		}

		// Render entries by date (most recent first)
		for i := len(dates) - 1; i >= 0; i-- {
			date := dates[i]
			sb.WriteString("\n")
			sb.WriteString(taskMetaStyle.Render(date))
			sb.WriteString("\n")

			for _, entry := range entryMap[date] {
				timeStr := entry.Timestamp.Format("15:04")
				sb.WriteString(taskLogTimeStyle.Render(timeStr))
				sb.WriteString(" ")
				sb.WriteString(taskLogContentStyle.Render(entry.Content))
				sb.WriteString("\n")
			}
		}
	}

	return sb.String()
}
