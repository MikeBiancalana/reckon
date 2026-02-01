package cli

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/MikeBiancalana/reckon/internal/tui/components"
)

// taskPickerModel is a simple Bubble Tea model for the task picker
type taskPickerModel struct {
	picker   *components.TaskPicker
	taskID   string
	canceled bool
}

func (m taskPickerModel) Init() tea.Cmd {
	return nil
}

func (m taskPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Allow ctrl+c to quit
		if msg.String() == "ctrl+c" {
			m.canceled = true
			return m, tea.Quit
		}

	case components.TaskPickerSelectMsg:
		// User selected a task
		m.taskID = msg.TaskID
		return m, tea.Quit

	case components.TaskPickerCancelMsg:
		// User cancelled with ESC
		m.canceled = true
		return m, tea.Quit
	}

	// Update the picker
	var cmd tea.Cmd
	m.picker, cmd = m.picker.Update(msg)
	return m, cmd
}

func (m taskPickerModel) View() string {
	return m.picker.View()
}

// PickTask launches an interactive task picker and returns the selected task ID.
// Returns the task ID, whether it was canceled, and any error.
// This is a helper function for use in CLI commands that need task selection.
func PickTask(tasks []journal.Task, title string) (taskID string, canceled bool, err error) {
	if len(tasks) == 0 {
		return "", false, fmt.Errorf("no tasks available")
	}

	// Create picker
	picker := components.NewTaskPicker(title)
	picker.Show(tasks)

	// Create model
	m := taskPickerModel{
		picker: picker,
	}

	// Run the program
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return "", false, fmt.Errorf("failed to run task picker: %w", err)
	}

	// Extract result
	result := finalModel.(taskPickerModel)
	return result.taskID, result.canceled, nil
}

// PickOpenTask is a convenience function that filters tasks to only open tasks
// and launches the picker. This is commonly used in commands.
func PickOpenTask(allTasks []journal.Task, title string) (taskID string, canceled bool, err error) {
	// Filter to only open tasks
	openTasks := make([]journal.Task, 0)
	for _, t := range allTasks {
		if t.Status == journal.TaskOpen {
			openTasks = append(openTasks, t)
		}
	}

	if len(openTasks) == 0 {
		return "", false, fmt.Errorf("no open tasks found")
	}

	return PickTask(openTasks, title)
}
