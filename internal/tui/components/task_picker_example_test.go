package components_test

import (
	"fmt"

	"github.com/MikeBiancalana/reckon/internal/tui/components"
	tea "github.com/charmbracelet/bubbletea"
)

// Example demonstrates how to use the TaskPicker component in a Bubble Tea program
func Example() {
	// Create some sample tasks
	tasks := []components.TaskRow{
		{ID: "task-1", Title: "Write comprehensive tests"},
		{ID: "task-2", Title: "Implement fuzzy search"},
		{ID: "task-3", Title: "Review pull requests"},
	}

	// Create a model that uses the TaskPicker
	type model struct {
		picker       *components.TaskPicker
		selectedTask string
	}

	// Initialize the model
	m := model{
		picker: components.NewTaskPicker("Select a task"),
	}

	// Show the picker with tasks
	m.picker.Show(tasks)

	// In a real application, you would run this with tea.NewProgram(m)
	// For demonstration purposes, we'll just show the pattern

	fmt.Println("TaskPicker initialized successfully")
	// Output: TaskPicker initialized successfully
}

// ExampleTaskPicker_integration demonstrates integration patterns
func ExampleTaskPicker_integration() {
	// Create a picker
	picker := components.NewTaskPicker("Choose a task to schedule")

	// In a real Bubble Tea model, you would:
	// 1. Include the picker in your model struct
	// 2. Call picker.Show(tasks) to display it
	// 3. Handle TaskPickerSelectMsg and TaskPickerCancelMsg in Update()
	// 4. Call picker.View() in your View() function

	// Example message handling:
	handleMessage := func(msg tea.Msg) string {
		switch msg := msg.(type) {
		case components.TaskPickerSelectMsg:
			return fmt.Sprintf("Selected task: %s", msg.TaskID)
		case components.TaskPickerCancelMsg:
			return "Cancelled"
		}
		return ""
	}

	_ = handleMessage
	fmt.Println("TaskPicker ready:", picker != nil)
	// Output: TaskPicker ready: true
}

// ExampleTaskPicker_withScheduleCommand shows how to use TaskPicker in a schedule command
func ExampleTaskPicker_withScheduleCommand() {
	// This example shows how the TaskPicker can be used in the schedule command

	type scheduleModel struct {
		picker       *components.TaskPicker
		selectedTask string
		dateStr      string
	}

	// Step 1: Show task picker
	m := scheduleModel{
		picker: components.NewTaskPicker("Select task to schedule"),
	}

	// Load tasks (in real code, this would come from the task service)
	tasks := []components.TaskRow{
		{ID: "task-1", Title: "Important task"},
	}

	m.picker.Show(tasks)

	// Step 2: Handle selection
	handleSelection := func(msg tea.Msg) {
		switch msg := msg.(type) {
		case components.TaskPickerSelectMsg:
			// Use the selected task ID to schedule it
			taskID := msg.TaskID
			fmt.Printf("Scheduling task: %s\n", taskID)
		}
	}

	_ = handleSelection
	fmt.Println("Schedule command example ready")
	// Output: Schedule command example ready
}
