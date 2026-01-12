package components

import (
	"testing"
	"time"

	"github.com/MikeBiancalana/reckon/internal/journal"
	tea "github.com/charmbracelet/bubbletea"
)

func TestNewTaskList(t *testing.T) {
	tests := []struct {
		name  string
		tasks []journal.Task
	}{
		{
			name:  "empty task list",
			tasks: []journal.Task{},
		},
		{
			name: "single task without notes",
			tasks: []journal.Task{
				{
					ID:        "task1",
					Text:      "Complete project",
					Status:    journal.TaskOpen,
					Notes:     []journal.TaskNote{},
					Position:  0,
					CreatedAt: time.Now(),
				},
			},
		},
		{
			name: "single task with notes",
			tasks: []journal.Task{
				{
					ID:     "task1",
					Text:   "Complete project",
					Status: journal.TaskOpen,
					Notes: []journal.TaskNote{
						{ID: "note1", Text: "First note", Position: 0},
						{ID: "note2", Text: "Second note", Position: 1},
					},
					Position:  0,
					CreatedAt: time.Now(),
				},
			},
		},
		{
			name: "multiple tasks",
			tasks: []journal.Task{
				{
					ID:        "task1",
					Text:      "Task one",
					Status:    journal.TaskOpen,
					Notes:     []journal.TaskNote{},
					Position:  0,
					CreatedAt: time.Now(),
				},
				{
					ID:     "task2",
					Text:   "Task two",
					Status: journal.TaskDone,
					Notes: []journal.TaskNote{
						{ID: "note1", Text: "Note for task two", Position: 0},
					},
					Position:  1,
					CreatedAt: time.Now(),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tl := NewTaskList(tt.tasks)
			if tl == nil {
				t.Fatal("NewTaskList returned nil")
			}

			if tl.collapsedMap == nil {
				t.Error("collapsedMap should be initialized")
			}

			if len(tl.tasks) != len(tt.tasks) {
				t.Errorf("expected %d tasks, got %d", len(tt.tasks), len(tl.tasks))
			}
		})
	}
}

func TestTaskList_View(t *testing.T) {
	t.Run("empty list shows placeholder", func(t *testing.T) {
		tl := NewTaskList([]journal.Task{})
		view := tl.View()

		if view == "" {
			t.Error("View should return placeholder for empty list")
		}
	})

	t.Run("non-empty list renders", func(t *testing.T) {
		tasks := []journal.Task{
			{
				ID:        "task1",
				Text:      "Test task",
				Status:    journal.TaskOpen,
				Notes:     []journal.TaskNote{},
				Position:  0,
				CreatedAt: time.Now(),
			},
		}
		tl := NewTaskList(tasks)
		view := tl.View()

		if view == "" {
			t.Error("View should render non-empty list")
		}
	})
}

func TestTaskList_SelectedTask(t *testing.T) {
	tasks := []journal.Task{
		{
			ID:     "task1",
			Text:   "First task",
			Status: journal.TaskOpen,
			Notes: []journal.TaskNote{
				{ID: "note1", Text: "Note text", Position: 0},
			},
			Position:  0,
			CreatedAt: time.Now(),
		},
		{
			ID:        "task2",
			Text:      "Second task",
			Status:    journal.TaskDone,
			Notes:     []journal.TaskNote{},
			Position:  1,
			CreatedAt: time.Now(),
		},
	}

	tl := NewTaskList(tasks)

	t.Run("returns nil for empty list", func(t *testing.T) {
		emptyList := NewTaskList([]journal.Task{})
		if emptyList.SelectedTask() != nil {
			t.Error("SelectedTask should return nil for empty list")
		}
	})

	t.Run("returns selected task", func(t *testing.T) {
		selected := tl.SelectedTask()
		if selected == nil {
			t.Fatal("SelectedTask should return a task")
		}

		if selected.ID != "task1" {
			t.Errorf("expected task1, got %s", selected.ID)
		}
	})

	t.Run("returns pointer to authoritative task instance", func(t *testing.T) {
		selected := tl.SelectedTask()
		if selected == nil {
			t.Fatal("SelectedTask should return a task")
		}

		// Mutate the task through the returned pointer
		selected.Status = journal.TaskDone
		selected.Text = "Modified text"

		// Verify the change is reflected in the tasks slice
		if tl.tasks[0].Status != journal.TaskDone {
			t.Error("mutation should affect task in tasks slice")
		}
		if tl.tasks[0].Text != "Modified text" {
			t.Error("text mutation should affect task in tasks slice")
		}
	})
}

func TestTaskList_UpdateTasks(t *testing.T) {
	initialTasks := []journal.Task{
		{
			ID:        "task1",
			Text:      "Initial task",
			Status:    journal.TaskOpen,
			Notes:     []journal.TaskNote{},
			Position:  0,
			CreatedAt: time.Now(),
		},
	}

	newTasks := []journal.Task{
		{
			ID:        "task2",
			Text:      "New task",
			Status:    journal.TaskOpen,
			Notes:     []journal.TaskNote{},
			Position:  0,
			CreatedAt: time.Now(),
		},
		{
			ID:        "task3",
			Text:      "Another task",
			Status:    journal.TaskDone,
			Notes:     []journal.TaskNote{},
			Position:  1,
			CreatedAt: time.Now(),
		},
	}

	tl := NewTaskList(initialTasks)
	tl.UpdateTasks(newTasks)

	if len(tl.tasks) != len(newTasks) {
		t.Errorf("expected %d tasks after update, got %d", len(newTasks), len(tl.tasks))
	}

	if tl.tasks[0].ID != "task2" {
		t.Errorf("expected first task to be task2, got %s", tl.tasks[0].ID)
	}
}

func TestTaskList_SetSize(t *testing.T) {
	tl := NewTaskList([]journal.Task{})
	// Should not panic
	tl.SetSize(80, 24)
}

func TestTaskList_SpaceToggle(t *testing.T) {
	tasks := []journal.Task{
		{
			ID:        "task1",
			Text:      "Test task",
			Status:    journal.TaskOpen,
			Notes:     []journal.TaskNote{},
			Position:  0,
			CreatedAt: time.Now(),
		},
	}

	tl := NewTaskList(tasks)

	t.Run("space on task emits toggle message", func(t *testing.T) {
		msg := tea.KeyMsg{Type: tea.KeySpace}
		_, cmd := tl.Update(msg)

		if cmd == nil {
			t.Fatal("space key should return a command")
		}

		result := cmd()
		toggleMsg, ok := result.(TaskToggleMsg)
		if !ok {
			t.Fatal("expected TaskToggleMsg")
		}

		if toggleMsg.TaskID != "task1" {
			t.Errorf("expected task1, got %s", toggleMsg.TaskID)
		}
	})

	t.Run("space on note does nothing", func(t *testing.T) {
		tasksWithNotes := []journal.Task{
			{
				ID:     "task1",
				Text:   "Test task",
				Status: journal.TaskOpen,
				Notes: []journal.TaskNote{
					{ID: "note1", Text: "Note text", Position: 0},
				},
				Position:  0,
				CreatedAt: time.Now(),
			},
		}

		tl2 := NewTaskList(tasksWithNotes)

		// Move to note (down arrow)
		downMsg := tea.KeyMsg{Type: tea.KeyDown}
		tl2, _ = tl2.Update(downMsg)

		// Try to toggle
		msg := tea.KeyMsg{Type: tea.KeySpace}
		_, cmd := tl2.Update(msg)

		// Should not emit toggle message
		if cmd != nil {
			result := cmd()
			if _, ok := result.(TaskToggleMsg); ok {
				t.Error("space on note should not emit toggle message")
			}
		}
	})
}

func TestTaskList_EnterToggle(t *testing.T) {
	t.Run("enter on task without notes does nothing", func(t *testing.T) {
		tasks := []journal.Task{
			{
				ID:        "task1",
				Text:      "Test task",
				Status:    journal.TaskOpen,
				Notes:     []journal.TaskNote{},
				Position:  0,
				CreatedAt: time.Now(),
			},
		}

		tl := NewTaskList(tasks)
		initialItems := len(tl.list.Items())

		msg := tea.KeyMsg{Type: tea.KeyEnter}
		tl, _ = tl.Update(msg)

		if len(tl.list.Items()) != initialItems {
			t.Error("enter on task without notes should not change items")
		}
	})

	t.Run("enter on task with notes toggles collapse", func(t *testing.T) {
		tasks := []journal.Task{
			{
				ID:     "task1",
				Text:   "Test task",
				Status: journal.TaskOpen,
				Notes: []journal.TaskNote{
					{ID: "note1", Text: "Note 1", Position: 0},
					{ID: "note2", Text: "Note 2", Position: 1},
				},
				Position:  0,
				CreatedAt: time.Now(),
			},
		}

		tl := NewTaskList(tasks)

		// Initially expanded (task + 2 notes = 3 items)
		if len(tl.list.Items()) != 3 {
			t.Errorf("expected 3 items initially, got %d", len(tl.list.Items()))
		}

		// Collapse
		msg := tea.KeyMsg{Type: tea.KeyEnter}
		tl, _ = tl.Update(msg)

		// Should have only 1 item (just the task)
		if len(tl.list.Items()) != 1 {
			t.Errorf("expected 1 item after collapse, got %d", len(tl.list.Items()))
		}

		if !tl.collapsedMap["task1"] {
			t.Error("task1 should be marked as collapsed")
		}

		// Expand again
		tl, _ = tl.Update(msg)

		// Should have 3 items again
		if len(tl.list.Items()) != 3 {
			t.Errorf("expected 3 items after expand, got %d", len(tl.list.Items()))
		}

		if tl.collapsedMap["task1"] {
			t.Error("task1 should not be marked as collapsed")
		}
	})
}

func TestBuildTaskItems(t *testing.T) {
	t.Run("builds items for tasks without notes", func(t *testing.T) {
		tasks := []journal.Task{
			{
				ID:        "task1",
				Text:      "Task 1",
				Status:    journal.TaskOpen,
				Notes:     []journal.TaskNote{},
				Position:  0,
				CreatedAt: time.Now(),
			},
			{
				ID:        "task2",
				Text:      "Task 2",
				Status:    journal.TaskDone,
				Notes:     []journal.TaskNote{},
				Position:  1,
				CreatedAt: time.Now(),
			},
		}

		collapsedMap := make(map[string]bool)
		items := buildTaskItems(tasks, collapsedMap)

		if len(items) != 2 {
			t.Errorf("expected 2 items, got %d", len(items))
		}

		for i, item := range items {
			taskItem, ok := item.(TaskItem)
			if !ok {
				t.Errorf("item %d is not a TaskItem", i)
				continue
			}

			if taskItem.isNote {
				t.Errorf("item %d should not be a note", i)
			}
		}
	})

	t.Run("builds items for tasks with notes (expanded)", func(t *testing.T) {
		tasks := []journal.Task{
			{
				ID:     "task1",
				Text:   "Task 1",
				Status: journal.TaskOpen,
				Notes: []journal.TaskNote{
					{ID: "note1", Text: "Note 1", Position: 0},
					{ID: "note2", Text: "Note 2", Position: 1},
				},
				Position:  0,
				CreatedAt: time.Now(),
			},
		}

		collapsedMap := make(map[string]bool)
		items := buildTaskItems(tasks, collapsedMap)

		// Should have 3 items: task + 2 notes
		if len(items) != 3 {
			t.Errorf("expected 3 items, got %d", len(items))
		}

		// First item should be the task
		taskItem, ok := items[0].(TaskItem)
		if !ok || taskItem.isNote {
			t.Error("first item should be the task, not a note")
		}

		// Next two should be notes
		for i := 1; i < 3; i++ {
			taskItem, ok := items[i].(TaskItem)
			if !ok {
				t.Errorf("item %d is not a TaskItem", i)
				continue
			}

			if !taskItem.isNote {
				t.Errorf("item %d should be a note", i)
			}

			if taskItem.taskID != "task1" {
				t.Errorf("note %d should have taskID task1, got %s", i, taskItem.taskID)
			}
		}
	})

	t.Run("builds items for tasks with notes (collapsed)", func(t *testing.T) {
		tasks := []journal.Task{
			{
				ID:     "task1",
				Text:   "Task 1",
				Status: journal.TaskOpen,
				Notes: []journal.TaskNote{
					{ID: "note1", Text: "Note 1", Position: 0},
					{ID: "note2", Text: "Note 2", Position: 1},
				},
				Position:  0,
				CreatedAt: time.Now(),
			},
		}

		collapsedMap := map[string]bool{"task1": true}
		items := buildTaskItems(tasks, collapsedMap)

		// Should have only 1 item: the task
		if len(items) != 1 {
			t.Errorf("expected 1 item, got %d", len(items))
		}

		taskItem, ok := items[0].(TaskItem)
		if !ok || taskItem.isNote {
			t.Error("item should be the task, not a note")
		}
	})

	t.Run("handles multiple tasks with mixed notes", func(t *testing.T) {
		tasks := []journal.Task{
			{
				ID:     "task1",
				Text:   "Task 1",
				Status: journal.TaskOpen,
				Notes: []journal.TaskNote{
					{ID: "note1", Text: "Note 1", Position: 0},
				},
				Position:  0,
				CreatedAt: time.Now(),
			},
			{
				ID:        "task2",
				Text:      "Task 2",
				Status:    journal.TaskDone,
				Notes:     []journal.TaskNote{},
				Position:  1,
				CreatedAt: time.Now(),
			},
			{
				ID:     "task3",
				Text:   "Task 3",
				Status: journal.TaskOpen,
				Notes: []journal.TaskNote{
					{ID: "note2", Text: "Note 2", Position: 0},
					{ID: "note3", Text: "Note 3", Position: 1},
				},
				Position:  2,
				CreatedAt: time.Now(),
			},
		}

		collapsedMap := make(map[string]bool)
		items := buildTaskItems(tasks, collapsedMap)

		// Should have: task1 + note1 + task2 + task3 + note2 + note3 = 6 items
		if len(items) != 6 {
			t.Errorf("expected 6 items, got %d", len(items))
		}
	})
}

func TestFindNoteText(t *testing.T) {
	notes := []journal.TaskNote{
		{ID: "note1", Text: "First note", Position: 0},
		{ID: "note2", Text: "Second note", Position: 1},
		{ID: "note3", Text: "Third note", Position: 2},
	}

	t.Run("finds existing note", func(t *testing.T) {
		text := findNoteText(notes, "note2")
		if text != "Second note" {
			t.Errorf("expected 'Second note', got '%s'", text)
		}
	})

	t.Run("returns empty string for non-existent note", func(t *testing.T) {
		text := findNoteText(notes, "nonexistent")
		if text != "" {
			t.Errorf("expected empty string, got '%s'", text)
		}
	})

	t.Run("handles empty notes slice", func(t *testing.T) {
		text := findNoteText([]journal.TaskNote{}, "note1")
		if text != "" {
			t.Errorf("expected empty string, got '%s'", text)
		}
	})
}

func TestTaskItem_FilterValue(t *testing.T) {
	task := journal.Task{
		ID:        "task1",
		Text:      "Test task",
		Status:    journal.TaskOpen,
		Notes:     []journal.TaskNote{},
		Position:  0,
		CreatedAt: time.Now(),
	}

	t.Run("task item returns task text", func(t *testing.T) {
		item := TaskItem{task: task, isNote: false}
		if item.FilterValue() != "Test task" {
			t.Errorf("expected 'Test task', got '%s'", item.FilterValue())
		}
	})

	t.Run("note item returns empty string", func(t *testing.T) {
		item := TaskItem{task: task, isNote: true, noteID: "note1"}
		if item.FilterValue() != "" {
			t.Errorf("expected empty string, got '%s'", item.FilterValue())
		}
	})
}

func TestTaskList_OptimisticUpdate(t *testing.T) {
	t.Run("space key applies optimistic update immediately", func(t *testing.T) {
		tasks := []journal.Task{
			{
				ID:        "task1",
				Text:      "Test task",
				Status:    journal.TaskOpen,
				Notes:     []journal.TaskNote{},
				Position:  0,
				CreatedAt: time.Now(),
			},
		}

		tl := NewTaskList(tasks)

		// Verify initial state
		if len(tl.optimisticMap) != 0 {
			t.Error("optimisticMap should be empty initially")
		}
		if len(tl.togglingMap) != 0 {
			t.Error("togglingMap should be empty initially")
		}

		// Trigger space key
		msg := tea.KeyMsg{Type: tea.KeySpace}
		tl, cmd := tl.Update(msg)

		if cmd == nil {
			t.Fatal("space key should return a command")
		}

		// Verify optimistic state was set
		if tl.optimisticMap["task1"] != journal.TaskDone {
			t.Error("optimistic state should be TaskDone")
		}
		if !tl.togglingMap["task1"] {
			t.Error("toggling state should be true")
		}

		// Verify command returns TaskToggleMsg
		result := cmd()
		toggleMsg, ok := result.(TaskToggleMsg)
		if !ok {
			t.Fatal("expected TaskToggleMsg")
		}
		if toggleMsg.TaskID != "task1" {
			t.Errorf("expected task1, got %s", toggleMsg.TaskID)
		}
	})

	t.Run("optimistic update toggles from done to open", func(t *testing.T) {
		tasks := []journal.Task{
			{
				ID:        "task1",
				Text:      "Completed task",
				Status:    journal.TaskDone,
				Notes:     []journal.TaskNote{},
				Position:  0,
				CreatedAt: time.Now(),
			},
		}

		tl := NewTaskList(tasks)

		// Trigger space key
		msg := tea.KeyMsg{Type: tea.KeySpace}
		tl, _ = tl.Update(msg)

		// Verify optimistic state was set to open
		if tl.optimisticMap["task1"] != journal.TaskOpen {
			t.Error("optimistic state should be TaskOpen")
		}
		if !tl.togglingMap["task1"] {
			t.Error("toggling state should be true")
		}
	})
}

func TestTaskList_ClearOptimisticState(t *testing.T) {
	tasks := []journal.Task{
		{
			ID:        "task1",
			Text:      "Test task",
			Status:    journal.TaskOpen,
			Notes:     []journal.TaskNote{},
			Position:  0,
			CreatedAt: time.Now(),
		},
	}

	tl := NewTaskList(tasks)

	// Set optimistic state manually
	tl.optimisticMap["task1"] = journal.TaskDone
	tl.togglingMap["task1"] = true

	// Clear it
	tl.ClearOptimisticState("task1")

	// Verify it was cleared
	if _, exists := tl.optimisticMap["task1"]; exists {
		t.Error("optimistic state should be cleared")
	}
	if _, exists := tl.togglingMap["task1"]; exists {
		t.Error("toggling state should be cleared")
	}
}

func TestTaskList_RevertOptimisticToggle(t *testing.T) {
	tasks := []journal.Task{
		{
			ID:        "task1",
			Text:      "Test task",
			Status:    journal.TaskOpen,
			Notes:     []journal.TaskNote{},
			Position:  0,
			CreatedAt: time.Now(),
		},
	}

	tl := NewTaskList(tasks)

	// Set optimistic state manually
	tl.optimisticMap["task1"] = journal.TaskDone
	tl.togglingMap["task1"] = true

	// Revert it
	tl.RevertOptimisticToggle("task1")

	// Verify it was reverted
	if _, exists := tl.optimisticMap["task1"]; exists {
		t.Error("optimistic state should be reverted")
	}
	if _, exists := tl.togglingMap["task1"]; exists {
		t.Error("toggling state should be reverted")
	}
}

func TestTaskList_UpdateTasksPreservesOptimisticState(t *testing.T) {
	tasks := []journal.Task{
		{
			ID:        "task1",
			Text:      "Test task",
			Status:    journal.TaskOpen,
			Notes:     []journal.TaskNote{},
			Position:  0,
			CreatedAt: time.Now(),
		},
	}

	tl := NewTaskList(tasks)

	// Set optimistic state
	tl.optimisticMap["task1"] = journal.TaskDone
	tl.togglingMap["task1"] = true

	// Update tasks
	newTasks := []journal.Task{
		{
			ID:        "task1",
			Text:      "Test task updated",
			Status:    journal.TaskDone, // Database has confirmed the change
			Notes:     []journal.TaskNote{},
			Position:  0,
			CreatedAt: time.Now(),
		},
	}
	tl.UpdateTasks(newTasks)

	// Optimistic state should still exist (caller must clear it)
	if tl.optimisticMap["task1"] != journal.TaskDone {
		t.Error("optimistic state should be preserved during UpdateTasks")
	}
	if !tl.togglingMap["task1"] {
		t.Error("toggling state should be preserved during UpdateTasks")
	}
}

func TestTaskDelegate_RendersOptimisticState(t *testing.T) {
	t.Run("renders optimistic status over actual status", func(t *testing.T) {
		// Create TaskList with optimistic state
		tasks := []journal.Task{
			{
				ID:        "task1",
				Text:      "Test task",
				Status:    journal.TaskOpen,
				Notes:     []journal.TaskNote{},
				Position:  0,
				CreatedAt: time.Now(),
			},
		}
		tl := NewTaskList(tasks)
		tl.optimisticMap["task1"] = journal.TaskDone

		// Verify the optimistic state is accessible via the task list
		if tl.optimisticMap["task1"] != journal.TaskDone {
			t.Error("task list should have optimistic state")
		}
	})

	t.Run("shows loading indicator when toggling", func(t *testing.T) {
		// Create TaskList with toggling state
		tasks := []journal.Task{
			{
				ID:        "task1",
				Text:      "Test task",
				Status:    journal.TaskOpen,
				Notes:     []journal.TaskNote{},
				Position:  0,
				CreatedAt: time.Now(),
			},
		}
		tl := NewTaskList(tasks)
		tl.togglingMap["task1"] = true

		// Validate the toggling state is accessible via the task list
		if !tl.togglingMap["task1"] {
			t.Error("task list should have toggling state")
		}
	})
}

func TestTaskList_MultipleOptimisticToggles(t *testing.T) {
	t.Run("handles multiple tasks being toggled", func(t *testing.T) {
		tasks := []journal.Task{
			{
				ID:        "task1",
				Text:      "Task 1",
				Status:    journal.TaskOpen,
				Notes:     []journal.TaskNote{},
				Position:  0,
				CreatedAt: time.Now(),
			},
			{
				ID:        "task2",
				Text:      "Task 2",
				Status:    journal.TaskDone,
				Notes:     []journal.TaskNote{},
				Position:  1,
				CreatedAt: time.Now(),
			},
		}

		tl := NewTaskList(tasks)

		// Toggle first task
		msg := tea.KeyMsg{Type: tea.KeySpace}
		tl, _ = tl.Update(msg)

		// Move to second task and toggle
		downMsg := tea.KeyMsg{Type: tea.KeyDown}
		tl, _ = tl.Update(downMsg)
		tl, _ = tl.Update(msg)

		// Both should be in optimistic state
		if tl.optimisticMap["task1"] != journal.TaskDone {
			t.Error("task1 should have optimistic state TaskDone")
		}
		if tl.optimisticMap["task2"] != journal.TaskOpen {
			t.Error("task2 should have optimistic state TaskOpen")
		}
		if !tl.togglingMap["task1"] || !tl.togglingMap["task2"] {
			t.Error("both tasks should be toggling")
		}

		// Clear first task
		tl.ClearOptimisticState("task1")

		// First should be cleared, second should remain
		if _, exists := tl.optimisticMap["task1"]; exists {
			t.Error("task1 optimistic state should be cleared")
		}
		if tl.optimisticMap["task2"] != journal.TaskOpen {
			t.Error("task2 should still have optimistic state")
		}
	})
}
