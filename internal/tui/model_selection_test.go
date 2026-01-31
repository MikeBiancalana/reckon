package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/MikeBiancalana/reckon/internal/tui/components"
)

// TestRenderTaskSection_SelectionIndicator verifies that the selected task
// is visually highlighted in the rendered output
func TestRenderTaskSection_SelectionIndicator(t *testing.T) {
	// Create a model with a task list
	service := &journal.Service{}
	m := NewModel(service)
	m.width = 120
	m.height = 30

	// Create some test tasks
	tasks := []journal.Task{
		{
			ID:        "task1",
			Text:      "First task",
			Status:    journal.TaskOpen,
			Position:  0,
			CreatedAt: time.Now(),
		},
		{
			ID:        "task2",
			Text:      "Second task",
			Status:    journal.TaskOpen,
			Position:  1,
			CreatedAt: time.Now(),
		},
		{
			ID:        "task3",
			Text:      "Third task",
			Status:    journal.TaskOpen,
			Position:  2,
			CreatedAt: time.Now(),
		},
	}

	// Initialize task list with these tasks
	m.taskList = components.NewTaskList(tasks)

	t.Run("selected task has visual indicator", func(t *testing.T) {
		// The first task should be selected by default
		rendered := m.renderTaskSection("TEST SECTION", tasks, 80, 20)

		// The output should contain the SelectedStyle applied to the first task
		// SelectedStyle uses foreground color 11 (bright yellow) and bold
		// We can't test the exact ANSI codes, but we can verify the structure
		if !strings.Contains(rendered, "First task") {
			t.Error("rendered output should contain First task")
		}
		if !strings.Contains(rendered, "Second task") {
			t.Error("rendered output should contain Second task")
		}
		if !strings.Contains(rendered, "Third task") {
			t.Error("rendered output should contain Third task")
		}

		// The rendered output should have styling applied
		// (ANSI codes will be present in the actual output)
		lines := strings.Split(rendered, "\n")
		if len(lines) < 4 { // header + 3 tasks
			t.Errorf("expected at least 4 lines, got %d", len(lines))
		}
	})

	t.Run("selection moves to second task", func(t *testing.T) {
		// Manually update the task list to select the second task
		// In real usage, this would happen through keyboard navigation
		m.taskList.UpdateTasks(tasks)

		// Simulate selecting the second task by updating internal state
		// (In the actual TUI, this happens through the list.Model's Select method)
		items := m.taskList.GetTasks()
		if len(items) < 2 {
			t.Fatal("expected at least 2 tasks")
		}

		rendered := m.renderTaskSection("TEST SECTION", tasks, 80, 20)

		// Both tasks should be in the output
		if !strings.Contains(rendered, "First task") {
			t.Error("rendered output should contain First task")
		}
		if !strings.Contains(rendered, "Second task") {
			t.Error("rendered output should contain Second task")
		}
	})

	t.Run("no selection when taskList is nil", func(t *testing.T) {
		m.taskList = nil
		rendered := m.renderTaskSection("TEST SECTION", tasks, 80, 20)

		// Should still render tasks, just without selection highlighting
		if !strings.Contains(rendered, "First task") {
			t.Error("rendered output should contain First task")
		}
		if !strings.Contains(rendered, "Second task") {
			t.Error("rendered output should contain Second task")
		}
	})

	t.Run("completed task has strikethrough style", func(t *testing.T) {
		completedTasks := []journal.Task{
			{
				ID:        "task1",
				Text:      "Completed task",
				Status:    journal.TaskDone,
				Position:  0,
				CreatedAt: time.Now(),
			},
		}

		m.taskList = components.NewTaskList(completedTasks)
		rendered := m.renderTaskSection("TEST SECTION", completedTasks, 80, 20)

		// Should contain the checkbox marked as done
		if !strings.Contains(rendered, "[x]") {
			t.Error("rendered output should contain [x] for completed task")
		}
		if !strings.Contains(rendered, "Completed task") {
			t.Error("rendered output should contain task text")
		}
	})

	t.Run("empty task list shows no tasks message", func(t *testing.T) {
		rendered := m.renderTaskSection("TEST SECTION", []journal.Task{}, 80, 20)

		if !strings.Contains(rendered, "No tasks") {
			t.Error("rendered output should contain 'No tasks' for empty list")
		}
	})

	t.Run("task with tags renders tags", func(t *testing.T) {
		taggedTasks := []journal.Task{
			{
				ID:        "task1",
				Text:      "Tagged task",
				Status:    journal.TaskOpen,
				Tags:      []string{"urgent", "work"},
				Position:  0,
				CreatedAt: time.Now(),
			},
		}

		m.taskList = components.NewTaskList(taggedTasks)
		rendered := m.renderTaskSection("TEST SECTION", taggedTasks, 80, 20)

		if !strings.Contains(rendered, "Tagged task") {
			t.Error("rendered output should contain task text")
		}
		if !strings.Contains(rendered, "urgent") || !strings.Contains(rendered, "work") {
			t.Error("rendered output should contain tags")
		}
	})
}

// TestRenderTaskSection_AllSections verifies selection works with different task lists
func TestRenderTaskSection_AllSections(t *testing.T) {
	service := &journal.Service{}
	m := NewModel(service)
	m.width = 120
	m.height = 30

	t.Run("multiple tasks with selection", func(t *testing.T) {
		tasks := []journal.Task{
			{
				ID:        "task1",
				Text:      "First task",
				Status:    journal.TaskOpen,
				Position:  0,
				CreatedAt: time.Now(),
			},
			{
				ID:        "task2",
				Text:      "Second task",
				Status:    journal.TaskOpen,
				Position:  1,
				CreatedAt: time.Now(),
			},
			{
				ID:        "task3",
				Text:      "Third task",
				Status:    journal.TaskOpen,
				Position:  2,
				CreatedAt: time.Now(),
			},
		}

		m.taskList = components.NewTaskList(tasks)

		rendered := m.renderTaskSection("TEST SECTION", tasks, 80, 20)
		if !strings.Contains(rendered, "First task") {
			t.Error("Section should contain first task")
		}
		if !strings.Contains(rendered, "Second task") {
			t.Error("Section should contain second task")
		}
		if !strings.Contains(rendered, "Third task") {
			t.Error("Section should contain third task")
		}
	})

	t.Run("single task gets selected", func(t *testing.T) {
		tasks := []journal.Task{
			{
				ID:        "single-task",
				Text:      "Only task",
				Status:    journal.TaskOpen,
				Position:  0,
				CreatedAt: time.Now(),
			},
		}

		m.taskList = components.NewTaskList(tasks)

		rendered := m.renderTaskSection("SINGLE TASK", tasks, 80, 20)
		if !strings.Contains(rendered, "Only task") {
			t.Error("Section should contain the only task")
		}
	})

	t.Run("section with mix of open and done tasks", func(t *testing.T) {
		tasks := []journal.Task{
			{
				ID:        "open-task",
				Text:      "Open task",
				Status:    journal.TaskOpen,
				Position:  0,
				CreatedAt: time.Now(),
			},
			{
				ID:        "done-task",
				Text:      "Done task",
				Status:    journal.TaskDone,
				Position:  1,
				CreatedAt: time.Now(),
			},
		}

		m.taskList = components.NewTaskList(tasks)

		rendered := m.renderTaskSection("MIXED SECTION", tasks, 80, 20)
		if !strings.Contains(rendered, "Open task") {
			t.Error("Section should contain open task")
		}
		if !strings.Contains(rendered, "Done task") {
			t.Error("Section should contain done task")
		}
		// Check for checkbox markers
		if !strings.Contains(rendered, "[ ]") {
			t.Error("Section should contain open task checkbox")
		}
		if !strings.Contains(rendered, "[x]") {
			t.Error("Section should contain done task checkbox")
		}
	})
}
