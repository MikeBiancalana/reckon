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
)

// TaskToggleMsg is sent when a task's status is toggled
type TaskToggleMsg struct {
	TaskID string
}

// TaskToggleErrorMsg is sent when a task toggle operation fails
type TaskToggleErrorMsg struct {
	TaskID string
	Error  error
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
	taskList *TaskList // Reference to parent TaskList for state access
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
		// Determine status to display (optimistic or actual)
		displayStatus := item.task.Status
		if optimisticStatus, hasOptimistic := d.taskList.optimisticMap[item.task.ID]; hasOptimistic {
			displayStatus = optimisticStatus
		}

		// Render task with checkbox
		checkbox := "[ ]"
		style = taskStyle
		if displayStatus == journal.TaskDone {
			checkbox = "[x]"
			style = taskDoneStyle
		}

		// Add expand/collapse indicator if task has notes
		indicator := ""
		if len(item.task.Notes) > 0 {
			if d.taskList.collapsedMap[item.task.ID] {
				indicator = "▶ "
			} else {
				indicator = "▼ "
			}
		}

		// Add loading indicator at the end if currently toggling
		loadingIndicator := ""
		if d.taskList.togglingMap[item.task.ID] {
			loadingIndicator = " ⋯"
		}

		text = fmt.Sprintf("%s%s %s%s", indicator, checkbox, item.task.Text, loadingIndicator)
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
	list          list.Model
	collapsedMap  map[string]bool
	tasks         []journal.Task // keep track of original tasks for state management
	optimisticMap map[string]journal.TaskStatus // taskID -> optimistic status
	togglingMap   map[string]bool        // taskID -> isToggling
}

// NewTaskList creates a new task list component
func NewTaskList(tasks []journal.Task) *TaskList {
	collapsedMap := make(map[string]bool)
	optimisticMap := make(map[string]journal.TaskStatus)
	togglingMap := make(map[string]bool)

	tl := &TaskList{
		collapsedMap:  collapsedMap,
		tasks:         tasks,
		optimisticMap: optimisticMap,
		togglingMap:   togglingMap,
	}

	items := buildTaskItems(tasks, collapsedMap)
	delegate := TaskDelegate{taskList: tl}
	l := list.New(items, delegate, 0, 0)
	l.Title = "Tasks"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = taskListTitleStyle

	tl.list = l
	return tl
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
			// Toggle task status with optimistic update
			selectedItem := tl.list.SelectedItem()
			if selectedItem != nil {
				taskItem, ok := selectedItem.(TaskItem)
				if ok && !taskItem.isNote {
					// Guard against re-toggling tasks already being toggled
					if tl.togglingMap[taskItem.task.ID] {
						return tl, nil
					}

					// Apply optimistic update immediately
					newStatus := journal.TaskOpen
					if taskItem.task.Status == journal.TaskOpen {
						newStatus = journal.TaskDone
					}
					tl.optimisticMap[taskItem.task.ID] = newStatus
					tl.togglingMap[taskItem.task.ID] = true

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

// ClearOptimisticState clears the optimistic state for a task after successful toggle
func (tl *TaskList) ClearOptimisticState(taskID string) {
	delete(tl.optimisticMap, taskID)
	delete(tl.togglingMap, taskID)
}

// RevertOptimisticToggle reverts an optimistic toggle when the operation fails
func (tl *TaskList) RevertOptimisticToggle(taskID string) {
	delete(tl.optimisticMap, taskID)
	delete(tl.togglingMap, taskID)
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

	// Clean up optimistic state for tasks that no longer exist
	taskIDSet := make(map[string]bool)
	for _, task := range tasks {
		taskIDSet[task.ID] = true
	}
	for taskID := range tl.optimisticMap {
		if !taskIDSet[taskID] {
			delete(tl.optimisticMap, taskID)
		}
	}
	for taskID := range tl.togglingMap {
		if !taskIDSet[taskID] {
			delete(tl.togglingMap, taskID)
		}
	}

	tl.tasks = tasks
	items := buildTaskItems(tasks, tl.collapsedMap)
	tl.list.SetItems(items)

	// Restore cursor to the previously selected task
	if selectedTaskID != "" {
		for i, item := range items {
			if taskItem, ok := item.(TaskItem); ok && taskItem.task.ID == selectedTaskID {
				tl.list.Select(i)
				break
			}
		}
	}
}
