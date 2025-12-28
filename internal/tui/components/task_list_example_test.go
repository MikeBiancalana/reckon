package components_test

import (
	"fmt"
	"time"

	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/MikeBiancalana/reckon/internal/tui/components"
)

// Example demonstrates basic usage of TaskList component
func ExampleTaskList() {
	// Create some sample tasks
	tasks := []journal.Task{
		{
			ID:     "task1",
			Text:   "Complete project documentation",
			Status: journal.TaskOpen,
			Notes: []journal.TaskNote{
				{ID: "note1", Text: "Include API examples", Position: 0},
				{ID: "note2", Text: "Add diagrams", Position: 1},
			},
			Position:  0,
			CreatedAt: time.Now(),
		},
		{
			ID:        "task2",
			Text:      "Review pull requests",
			Status:    journal.TaskDone,
			Notes:     []journal.TaskNote{},
			Position:  1,
			CreatedAt: time.Now(),
		},
		{
			ID:     "task3",
			Text:   "Update dependencies",
			Status: journal.TaskOpen,
			Notes: []journal.TaskNote{
				{ID: "note3", Text: "Check for security vulnerabilities", Position: 0},
			},
			Position:  2,
			CreatedAt: time.Now(),
		},
	}

	// Create the task list component
	taskList := components.NewTaskList(tasks)

	// Set size
	taskList.SetSize(80, 24)

	// Get selected task
	selected := taskList.SelectedTask()
	if selected != nil {
		fmt.Printf("Selected task: %s\n", selected.Text)
	}

	// Output: Selected task: Complete project documentation
}

// Example demonstrates updating tasks in the TaskList
func ExampleTaskList_UpdateTasks() {
	// Start with empty task list
	taskList := components.NewTaskList([]journal.Task{})

	// Add new tasks
	newTasks := []journal.Task{
		{
			ID:        "task1",
			Text:      "New task",
			Status:    journal.TaskOpen,
			Notes:     []journal.TaskNote{},
			Position:  0,
			CreatedAt: time.Now(),
		},
	}

	taskList.UpdateTasks(newTasks)

	selected := taskList.SelectedTask()
	if selected != nil {
		fmt.Printf("Updated task: %s\n", selected.Text)
	}

	// Output: Updated task: New task
}
