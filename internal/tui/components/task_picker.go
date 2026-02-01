package components

import (
	"fmt"
	"io"
	"strings"

	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	taskPickerBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("39")).
				Padding(1, 2)

	taskPickerTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("39"))

	taskPickerItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))

	taskPickerSelectedItemStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("39")).
					Bold(true)

	taskPickerDescStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245"))

	taskPickerHelpStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240"))
)

// TaskPickerSelectMsg is sent when a task is selected
type TaskPickerSelectMsg struct {
	TaskID string
}

// TaskPickerCancelMsg is sent when the task picker is cancelled
type TaskPickerCancelMsg struct{}

// taskPickerItem implements list.Item for the task picker
type taskPickerItem struct {
	task journal.Task
}

func (i taskPickerItem) FilterValue() string {
	return i.task.Text
}

func (i taskPickerItem) Title() string {
	return i.task.Text
}

func (i taskPickerItem) Description() string {
	var parts []string

	// Add tags if present
	if len(i.task.Tags) > 0 {
		parts = append(parts, strings.Join(i.task.Tags, ", "))
	}

	// Add created date
	if !i.task.CreatedAt.IsZero() {
		parts = append(parts, "Created: "+i.task.CreatedAt.Format("2006-01-02"))
	}

	// Add schedule if present
	if i.task.ScheduledDate != nil {
		parts = append(parts, "Scheduled: "+*i.task.ScheduledDate)
	}

	// Add deadline if present
	if i.task.DeadlineDate != nil {
		parts = append(parts, "Deadline: "+*i.task.DeadlineDate)
	}

	return strings.Join(parts, " | ")
}

// taskPickerDelegate handles rendering of task picker items
type taskPickerDelegate struct{}

func (d taskPickerDelegate) Height() int  { return 2 }
func (d taskPickerDelegate) Spacing() int { return 1 }
func (d taskPickerDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return nil
}

func (d taskPickerDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	item, ok := listItem.(taskPickerItem)
	if !ok {
		return
	}

	title := item.Title()
	desc := item.Description()

	isSelected := index == m.Index()

	// Render title
	titleStyle := taskPickerItemStyle
	if isSelected {
		titleStyle = taskPickerSelectedItemStyle
		title = "> " + title
	} else {
		title = "  " + title
	}

	fmt.Fprint(w, titleStyle.Render(title))
	if desc != "" {
		fmt.Fprint(w, "\n  "+taskPickerDescStyle.Render(desc))
	}
}

// TaskPicker is a reusable fuzzy finder for selecting tasks
type TaskPicker struct {
	list         list.Model
	title        string
	visible      bool
	tasks        []journal.Task
	selectedTask *journal.Task
	width        int
}

// NewTaskPicker creates a new task picker component
func NewTaskPicker(title string) *TaskPicker {
	delegate := taskPickerDelegate{}
	l := list.New([]list.Item{}, delegate, 0, 0)
	l.Title = title
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.Styles.Title = taskPickerTitleStyle
	l.SetShowHelp(false)

	return &TaskPicker{
		list:    l,
		title:   title,
		visible: false,
		width:   80,
	}
}

// Show displays the task picker with the given tasks
func (tp *TaskPicker) Show(tasks []journal.Task) {
	tp.visible = true
	tp.tasks = tasks
	tp.selectedTask = nil

	// Convert tasks to list items
	items := make([]list.Item, len(tasks))
	for i, task := range tasks {
		items[i] = taskPickerItem{task: task}
	}

	tp.list.SetItems(items)
	tp.list.ResetFilter()
}

// Hide hides the task picker
func (tp *TaskPicker) Hide() {
	tp.visible = false
	tp.selectedTask = nil
}

// IsVisible returns whether the task picker is visible
func (tp *TaskPicker) IsVisible() bool {
	return tp.visible
}

// GetSelectedTaskID returns the ID of the selected task, or empty string if none
func (tp *TaskPicker) GetSelectedTaskID() string {
	if tp.selectedTask == nil {
		return ""
	}
	return tp.selectedTask.ID
}

// SetWidth sets the width of the task picker
func (tp *TaskPicker) SetWidth(width int) {
	tp.width = width
	listWidth := width - 10
	listHeight := 15
	if listWidth < 40 {
		listWidth = 40
	}
	tp.list.SetSize(listWidth, listHeight)
}

// Update handles Bubble Tea messages
func (tp *TaskPicker) Update(msg tea.Msg) (*TaskPicker, tea.Cmd) {
	if !tp.visible {
		return tp, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc:
			tp.Hide()
			return tp, func() tea.Msg {
				return TaskPickerCancelMsg{}
			}

		case tea.KeyEnter:
			// Get selected item
			selectedItem := tp.list.SelectedItem()
			if selectedItem == nil {
				return tp, nil
			}

			item, ok := selectedItem.(taskPickerItem)
			if !ok {
				return tp, nil
			}

			tp.selectedTask = &item.task
			tp.Hide()

			return tp, func() tea.Msg {
				return TaskPickerSelectMsg{
					TaskID: item.task.ID,
				}
			}
		}
	}

	// Update the list
	var cmd tea.Cmd
	tp.list, cmd = tp.list.Update(msg)

	return tp, cmd
}

// View renders the task picker
func (tp *TaskPicker) View() string {
	if !tp.visible {
		return ""
	}

	var content strings.Builder

	// List view
	content.WriteString(tp.list.View())
	content.WriteString("\n\n")

	// Help text
	helpText := "ENTER: select  ESC: cancel  /: filter"
	content.WriteString(taskPickerHelpStyle.Render(helpText))

	// Wrap in box
	return taskPickerBoxStyle.Render(content.String())
}
