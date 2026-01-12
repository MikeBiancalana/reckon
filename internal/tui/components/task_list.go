package components

import (
	"fmt"
	"io"

	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	taskStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39"))

	taskDoneStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Strikethrough(true)

	noteStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	taskListTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("205")).
				Bold(true)

	focusedTaskListTitleStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("11")).
					Bold(true)
)

// TaskToggleMsg is sent when a task's status is toggled
type TaskToggleMsg struct {
	TaskID string
}

// TaskItem represents an item in the task list (either a task or a note)
type TaskItem struct {
	task   journal.Task
	isNote bool
	noteID string
	taskID string // parent task ID for notes
}

func (t TaskItem) FilterValue() string {
	if t.isNote {
		return ""
	}
	return t.task.Text
}

// TaskDelegate handles rendering of task items
type TaskDelegate struct {
	collapsedMap map[string]bool // taskID -> isCollapsed
}

func (d TaskDelegate) Height() int                               { return 1 }
func (d TaskDelegate) Spacing() int                              { return 0 }
func (d TaskDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return nil }

func (d TaskDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	item, ok := listItem.(TaskItem)
	if !ok {
		return
	}

	var text string
	var style lipgloss.Style

	if item.isNote {
		// Render note with 2-space indent
		text = fmt.Sprintf("  - %s", findNoteText(item.task.Notes, item.noteID))
		style = noteStyle
	} else {
		// Render task with checkbox
		checkbox := "[ ]"
		style = taskStyle
		if item.task.Status == journal.TaskDone {
			checkbox = "[x]"
			style = taskDoneStyle
		}

		// Add expand/collapse indicator if task has notes
		indicator := ""
		if len(item.task.Notes) > 0 {
			if d.collapsedMap[item.task.ID] {
				indicator = "▶ "
			} else {
				indicator = "▼ "
			}
		}

		text = fmt.Sprintf("%s%s %s", indicator, checkbox, item.task.Text)
	}

	// Highlight selected item
	if index == m.Index() {
		text = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true).Render("▶ " + text)
	} else {
		text = style.Render(text)
	}

	fmt.Fprintf(w, "%s", text)
}

// findNoteText finds the text of a note by ID
func findNoteText(notes []journal.TaskNote, noteID string) string {
	for _, note := range notes {
		if note.ID == noteID {
			return note.Text
		}
	}
	return ""
}

// TaskList represents the tasks component
type TaskList struct {
	list         list.Model
	collapsedMap map[string]bool
	tasks        []journal.Task // keep track of original tasks for state management
	focused      bool
}

// NewTaskList creates a new task list component
func NewTaskList(tasks []journal.Task) *TaskList {
	collapsedMap := make(map[string]bool)
	items := buildTaskItems(tasks, collapsedMap)

	delegate := TaskDelegate{collapsedMap: collapsedMap}
	l := list.New(items, delegate, 0, 0)
	l.Title = "Tasks"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = taskListTitleStyle

	return &TaskList{
		list:         l,
		collapsedMap: collapsedMap,
		tasks:        tasks,
	}
}

// buildTaskItems converts tasks into list items, respecting collapsed state
func buildTaskItems(tasks []journal.Task, collapsedMap map[string]bool) []list.Item {
	items := make([]list.Item, 0)

	for _, task := range tasks {
		// Add the task itself
		items = append(items, TaskItem{
			task:   task,
			isNote: false,
		})

		// Add notes if task is not collapsed
		if !collapsedMap[task.ID] && len(task.Notes) > 0 {
			for _, note := range task.Notes {
				items = append(items, TaskItem{
					task:   task,
					isNote: true,
					noteID: note.ID,
					taskID: task.ID,
				})
			}
		}
	}

	return items
}

// Update handles messages for the task list
func (tl *TaskList) Update(msg tea.Msg) (*TaskList, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeySpace:
			// Toggle task status
			selectedItem := tl.list.SelectedItem()
			if selectedItem != nil {
				taskItem, ok := selectedItem.(TaskItem)
				if ok && !taskItem.isNote {
					// Return a message to toggle the task status
					return tl, func() tea.Msg {
						return TaskToggleMsg{TaskID: taskItem.task.ID}
					}
				}
			}
			return tl, nil

		case tea.KeyEnter:
			// Toggle expand/collapse
			selectedItem := tl.list.SelectedItem()
			if selectedItem != nil {
				taskItem, ok := selectedItem.(TaskItem)
				if ok && !taskItem.isNote && len(taskItem.task.Notes) > 0 {
					// Toggle collapsed state
					tl.collapsedMap[taskItem.task.ID] = !tl.collapsedMap[taskItem.task.ID]

					// Rebuild items with new collapsed state
					items := buildTaskItems(tl.tasks, tl.collapsedMap)
					tl.list.SetItems(items)

					// Update delegate with new collapsed map
					delegate := TaskDelegate{collapsedMap: tl.collapsedMap}
					tl.list.SetDelegate(delegate)

					// If collapsing and cursor was on a note, move back to task
					currentIndex := tl.list.Index()
					if currentIndex < len(items) {
						currentItem, ok := items[currentIndex].(TaskItem)
						if ok && currentItem.isNote && tl.collapsedMap[taskItem.task.ID] {
							// Find the task item index
							for i, item := range items {
								if ti, ok := item.(TaskItem); ok && !ti.isNote && ti.task.ID == taskItem.task.ID {
									tl.list.Select(i)
									break
								}
							}
						}
					}
				}
			}
			return tl, nil
		}
	}

	var cmd tea.Cmd
	tl.list, cmd = tl.list.Update(msg)
	return tl, cmd
}

// View renders the task list
func (tl *TaskList) View() string {
	if len(tl.list.Items()) == 0 {
		return "Tasks\n\nNo tasks yet - press t to add one"
	}
	return tl.list.View()
}

// SetSize sets the size of the list
func (tl *TaskList) SetSize(width, height int) {
	tl.list.SetSize(width, height)
}

// SetFocused sets whether this component is focused
func (tl *TaskList) SetFocused(focused bool) {
	tl.focused = focused
	if focused {
		tl.list.Styles.Title = focusedTaskListTitleStyle
	} else {
		tl.list.Styles.Title = taskListTitleStyle
	}
}

// SelectedTask returns the currently selected task
func (tl *TaskList) SelectedTask() *journal.Task {
	item := tl.list.SelectedItem()
	if item == nil {
		return nil
	}
	taskItem, ok := item.(TaskItem)
	if !ok {
		return nil
	}

	// Find and return the task from our tasks slice
	for i := range tl.tasks {
		if tl.tasks[i].ID == taskItem.task.ID {
			return &tl.tasks[i]
		}
	}
	return nil
}

// IsSelectedItemNote returns true if the currently selected item is a note
func (tl *TaskList) IsSelectedItemNote() bool {
	item := tl.list.SelectedItem()
	if item == nil {
		return false
	}
	taskItem, ok := item.(TaskItem)
	if !ok {
		return false
	}
	return taskItem.isNote
}

// UpdateTasks updates the list with new tasks
func (tl *TaskList) UpdateTasks(tasks []journal.Task) {
	// Preserve cursor position by finding the currently selected task ID
	selectedItem := tl.list.SelectedItem()
	var selectedTaskID string
	if selectedItem != nil {
		if taskItem, ok := selectedItem.(TaskItem); ok {
			selectedTaskID = taskItem.task.ID
		}
	}

	tl.tasks = tasks
	items := buildTaskItems(tasks, tl.collapsedMap)
	tl.list.SetItems(items)

	// Restore cursor to the previously selected task
	if selectedTaskID != "" {
		for i, item := range tl.list.Items() {
			if taskItem, ok := item.(TaskItem); ok && !taskItem.isNote && taskItem.task.ID == selectedTaskID {
				tl.list.Select(i)
				break
			}
		}
	}

	// Update delegate with current collapsed map
	delegate := TaskDelegate{collapsedMap: tl.collapsedMap}
	tl.list.SetDelegate(delegate)

	// Restore cursor to the same task if it still exists
	if selectedTaskID != "" {
		for i, item := range items {
			if taskItem, ok := item.(TaskItem); ok && taskItem.task.ID == selectedTaskID {
				tl.list.Select(i)
				break
			}
		}
	}
}
