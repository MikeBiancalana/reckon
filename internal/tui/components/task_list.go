package components

import (
	"fmt"
	"strings"

	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	taskOpenStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	taskDoneStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Strikethrough(true)

	noteStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))

	expandedIcon  = "▼"
	collapsedIcon = "▶"
)

// TaskItem represents an item in the task list (task or note)
// Implements list.Item interface
type TaskItem struct {
	task     *journal.Task
	isNote   bool
	noteID   string
	taskID   string
	noteText string // Store note text directly
}

// FilterValue implements list.Item interface
func (t TaskItem) FilterValue() string {
	if t.isNote {
		return t.noteText
	}
	return t.task.Text
}

// TaskList is a Bubble Tea component for displaying tasks with collapsible notes
type TaskList struct {
	list         list.Model
	collapsedMap map[string]bool
}

// NewTaskList creates a new TaskList component
func NewTaskList(tasks []journal.Task) *TaskList {
	// Convert tasks to list items
	items := make([]list.Item, 0, len(tasks)*2) // Pre-allocate some space
	for i := range tasks {
		task := &tasks[i]
		items = append(items, TaskItem{
			task:   task,
			isNote: false,
			taskID: task.ID,
		})
	}

	// Create list with default delegate
	delegate := list.NewDefaultDelegate()
	l := list.New(items, delegate, 0, 0)

	return &TaskList{
		list:         l,
		collapsedMap: make(map[string]bool),
	}
}

// Update handles Bubble Tea messages
func (tl *TaskList) Update(msg tea.Msg) (*TaskList, tea.Cmd) {
	var cmd tea.Cmd
	tl.list, cmd = tl.list.Update(msg)
	return tl, cmd
}

// View renders the task list
func (tl *TaskList) View() string {
	items := tl.list.Items()

	var sb strings.Builder
	for i, item := range items {
		taskItem, ok := item.(TaskItem)
		if !ok {
			continue
		}

		// Highlight selected item
		line := tl.renderTaskItem(taskItem)
		if i == tl.list.Index() {
			line = lipgloss.NewStyle().
				Background(lipgloss.Color("240")).
				Foreground(lipgloss.Color("15")).
				Render(line)
		}

		sb.WriteString(line)
		sb.WriteString("\n")
	}

	return sb.String()
}

// renderTaskItem renders a single task item (task or note)
func (tl *TaskList) renderTaskItem(taskItem TaskItem) string {
	if taskItem.isNote {
		// Render note with indentation
		line := fmt.Sprintf("  - %s", taskItem.noteText)
		return noteStyle.Render(line)
	}

	// Render task with checkbox and expand/collapse indicator
	task := taskItem.task
	checkbox := "[ ]"
	if task.Status == journal.TaskDone {
		checkbox = "[x]"
	}

	indicator := expandedIcon
	if tl.collapsedMap[task.ID] {
		indicator = collapsedIcon
	}

	// Omit ID in display for cleaner UI
	line := fmt.Sprintf("%s %s %s", indicator, checkbox, task.Text)

	if task.Status == journal.TaskDone {
		return taskDoneStyle.Render(line)
	}
	return taskOpenStyle.Render(line)
}

// SetSize updates the dimensions of task list
func (tl *TaskList) SetSize(width, height int) {
	tl.list.SetSize(width, height)
}

// SelectedTask returns the currently selected task
func (tl *TaskList) SelectedTask() *journal.Task {
	selected := tl.list.SelectedItem()
	if selected == nil {
		return nil
	}

	taskItem, ok := selected.(TaskItem)
	if !ok || taskItem.isNote {
		return nil
	}

	return taskItem.task
}

// UpdateTasks updates the tasks in the list
func (tl *TaskList) UpdateTasks(tasks []journal.Task) {
	// Convert tasks to list items
	items := make([]list.Item, 0)
	for i := range tasks {
		task := &tasks[i]
		items = append(items, TaskItem{
			task:   task,
			isNote: false,
			taskID: task.ID,
		})

		// Add notes if task is expanded
		if !tl.collapsedMap[task.ID] {
			for j := range task.Notes {
				items = append(items, TaskItem{
					task:     task,
					isNote:   true,
					noteID:   task.Notes[j].ID,
					taskID:   task.ID,
					noteText: task.Notes[j].Text,
				})
			}
		}
	}

	tl.list.SetItems(items)
}

// ToggleCollapse toggles the collapsed state of a task
func (tl *TaskList) ToggleCollapse(taskID string) {
	if _, exists := tl.collapsedMap[taskID]; exists {
		delete(tl.collapsedMap, taskID)
	} else {
		tl.collapsedMap[taskID] = true
	}
	// Refresh list to show/hide notes
	// Note: This requires the parent to call UpdateTasks
}

// IsCollapsed returns whether a task is collapsed
func (tl *TaskList) IsCollapsed(taskID string) bool {
	collapsed, exists := tl.collapsedMap[taskID]
	return exists && collapsed
}
