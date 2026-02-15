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

func TestTaskList_UpdateTasks_CursorRestoration(t *testing.T) {
	t.Run("preserves cursor on same task after update", func(t *testing.T) {
		initialTasks := []journal.Task{
			{ID: "task1", Text: "Task 1", Status: journal.TaskOpen, Position: 0, CreatedAt: time.Now()},
			{ID: "task2", Text: "Task 2", Status: journal.TaskOpen, Position: 1, CreatedAt: time.Now()},
			{ID: "task3", Text: "Task 3", Status: journal.TaskOpen, Position: 2, CreatedAt: time.Now()},
		}

		tl := NewTaskList(initialTasks)

		// Select task2 (index 1)
		tl.list.Select(1)

		// Verify task2 is selected
		selectedItem := tl.list.SelectedItem()
		if taskItem, ok := selectedItem.(TaskItem); ok {
			if taskItem.task.ID != "task2" {
				t.Fatalf("expected task2 to be selected, got %s", taskItem.task.ID)
			}
		}

		// Update with modified tasks (task2 text changed)
		updatedTasks := []journal.Task{
			{ID: "task1", Text: "Task 1", Status: journal.TaskOpen, Position: 0, CreatedAt: time.Now()},
			{ID: "task2", Text: "Task 2 MODIFIED", Status: journal.TaskOpen, Position: 1, CreatedAt: time.Now()},
			{ID: "task3", Text: "Task 3", Status: journal.TaskOpen, Position: 2, CreatedAt: time.Now()},
		}
		tl.UpdateTasks(updatedTasks)

		// Verify cursor stayed on task2
		selectedItem = tl.list.SelectedItem()
		if taskItem, ok := selectedItem.(TaskItem); ok {
			if taskItem.task.ID != "task2" {
				t.Errorf("expected cursor to remain on task2, got %s", taskItem.task.ID)
			}
		} else {
			t.Error("expected selected item to be a TaskItem")
		}

		// Verify lastSelectedTaskID is updated correctly
		if tl.lastSelectedTaskID != "task2" {
			t.Errorf("expected lastSelectedTaskID to be task2, got %s", tl.lastSelectedTaskID)
		}
	})

	t.Run("handles task removal - lastSelectedTaskID updates to cursor position", func(t *testing.T) {
		initialTasks := []journal.Task{
			{ID: "task1", Text: "Task 1", Status: journal.TaskOpen, Position: 0, CreatedAt: time.Now()},
			{ID: "task2", Text: "Task 2", Status: journal.TaskOpen, Position: 1, CreatedAt: time.Now()},
			{ID: "task3", Text: "Task 3", Status: journal.TaskOpen, Position: 2, CreatedAt: time.Now()},
		}

		tl := NewTaskList(initialTasks)

		// Select task2
		tl.list.Select(1)
		tl.lastSelectedTaskID = "task2" // Simulate it was tracked

		// Update with task2 removed
		updatedTasks := []journal.Task{
			{ID: "task1", Text: "Task 1", Status: journal.TaskOpen, Position: 0, CreatedAt: time.Now()},
			{ID: "task3", Text: "Task 3", Status: journal.TaskOpen, Position: 1, CreatedAt: time.Now()},
		}
		tl.UpdateTasks(updatedTasks)

		// When task2 is removed, cursor will be on some valid task
		// Verify lastSelectedTaskID matches whatever task is now selected
		selectedItem := tl.list.SelectedItem()
		if selectedItem != nil {
			if taskItem, ok := selectedItem.(TaskItem); ok && !taskItem.isNote {
				if tl.lastSelectedTaskID != taskItem.task.ID {
					t.Errorf("expected lastSelectedTaskID to match cursor position %s, got %s", taskItem.task.ID, tl.lastSelectedTaskID)
				}
			}
		} else {
			// If no tasks, lastSelectedTaskID should be empty
			if tl.lastSelectedTaskID != "" {
				t.Errorf("expected lastSelectedTaskID to be empty when no tasks, got %s", tl.lastSelectedTaskID)
			}
		}
	})

	t.Run("handles empty task list", func(t *testing.T) {
		initialTasks := []journal.Task{
			{ID: "task1", Text: "Task 1", Status: journal.TaskOpen, Position: 0, CreatedAt: time.Now()},
		}

		tl := NewTaskList(initialTasks)
		tl.list.Select(0)

		// Update to empty list
		tl.UpdateTasks([]journal.Task{})

		// Verify lastSelectedTaskID is cleared
		if tl.lastSelectedTaskID != "" {
			t.Errorf("expected lastSelectedTaskID to be empty after clearing tasks, got %s", tl.lastSelectedTaskID)
		}
	})

	t.Run("preserves cursor with notes present", func(t *testing.T) {
		initialTasks := []journal.Task{
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
			{ID: "task2", Text: "Task 2", Status: journal.TaskOpen, Position: 1, CreatedAt: time.Now()},
		}

		tl := NewTaskList(initialTasks)

		// Select task2 (should be at index 2: task1, note1, task2)
		tl.list.Select(2)

		// Update tasks (task1 adds another note)
		updatedTasks := []journal.Task{
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
			{ID: "task2", Text: "Task 2", Status: journal.TaskOpen, Position: 1, CreatedAt: time.Now()},
		}
		tl.UpdateTasks(updatedTasks)

		// Verify cursor stayed on task2 (now at index 3: task1, note1, note2, task2)
		selectedItem := tl.list.SelectedItem()
		if taskItem, ok := selectedItem.(TaskItem); ok {
			if taskItem.isNote {
				t.Error("expected selected item to be a task, not a note")
			}
			if taskItem.task.ID != "task2" {
				t.Errorf("expected cursor to remain on task2, got %s", taskItem.task.ID)
			}
		} else {
			t.Error("expected selected item to be a TaskItem")
		}

		// Verify lastSelectedTaskID
		if tl.lastSelectedTaskID != "task2" {
			t.Errorf("expected lastSelectedTaskID to be task2, got %s", tl.lastSelectedTaskID)
		}
	})
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
		tl.focused = true
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
		tl.focused = true
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
		tl.focused = true

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

func TestParseDate(t *testing.T) {
	t.Run("parses valid date", func(t *testing.T) {
		dateStr := "2026-01-20"
		result, ok := parseDate(&dateStr)
		if !ok {
			t.Error("expected parse to succeed")
		}
		expected := time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC)
		if !result.Equal(expected) {
			t.Errorf("expected %v, got %v", expected, result)
		}
	})

	t.Run("returns false for nil", func(t *testing.T) {
		_, ok := parseDate(nil)
		if ok {
			t.Error("expected parse to fail for nil")
		}
	})

	t.Run("returns false for empty string", func(t *testing.T) {
		dateStr := ""
		_, ok := parseDate(&dateStr)
		if ok {
			t.Error("expected parse to fail for empty string")
		}
	})

	t.Run("returns false for invalid format", func(t *testing.T) {
		dateStr := "01-20-2026"
		_, ok := parseDate(&dateStr)
		if ok {
			t.Error("expected parse to fail for invalid format")
		}
	})
}

func TestGroupTasksByTime(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	lastWeek := time.Now().AddDate(0, 0, -7).Format("2006-01-02")

	t.Run("empty task list", func(t *testing.T) {
		result := GroupTasksByTime([]journal.Task{})
		if len(result.Today) != 0 || len(result.ThisWeek) != 0 || len(result.AllTasks) != 0 {
			t.Error("expected empty result")
		}
	})

	t.Run("unscheduled tasks go to AllTasks", func(t *testing.T) {
		tasks := []journal.Task{
			{ID: "task1", Text: "Unscheduled task", Status: journal.TaskOpen},
		}
		result := GroupTasksByTime(tasks)
		if len(result.AllTasks) != 1 {
			t.Errorf("expected 1 task in AllTasks, got %d", len(result.AllTasks))
		}
		if len(result.Today) != 0 || len(result.ThisWeek) != 0 {
			t.Error("expected no tasks in Today or ThisWeek")
		}
	})

	t.Run("scheduled for today goes to Today", func(t *testing.T) {
		tasks := []journal.Task{
			{ID: "task1", Text: "Today's task", Status: journal.TaskOpen, ScheduledDate: &today},
		}
		result := GroupTasksByTime(tasks)
		if len(result.Today) != 1 {
			t.Errorf("expected 1 task in Today, got %d", len(result.Today))
		}
	})

	t.Run("deadline today goes to Today", func(t *testing.T) {
		tasks := []journal.Task{
			{ID: "task1", Text: "Due today", Status: journal.TaskOpen, DeadlineDate: &today},
		}
		result := GroupTasksByTime(tasks)
		if len(result.Today) != 1 {
			t.Errorf("expected 1 task in Today, got %d", len(result.Today))
		}
	})

	t.Run("overdue deadline goes to Today", func(t *testing.T) {
		tasks := []journal.Task{
			{ID: "task1", Text: "Overdue task", Status: journal.TaskOpen, DeadlineDate: &lastWeek},
		}
		result := GroupTasksByTime(tasks)
		if len(result.Today) != 1 {
			t.Errorf("expected 1 task in Today, got %d", len(result.Today))
		}
	})

	t.Run("scheduled for this week goes to ThisWeek", func(t *testing.T) {
		daysUntilFriday := (time.Friday - time.Now().Weekday() + 7) % 7
		if daysUntilFriday == 0 {
			daysUntilFriday = 7
		}
		thisFriday := time.Now().AddDate(0, 0, int(daysUntilFriday)).Format("2006-01-02")
		tasks := []journal.Task{
			{ID: "task1", Text: "This week's task", Status: journal.TaskOpen, ScheduledDate: &thisFriday},
		}
		result := GroupTasksByTime(tasks)
		if len(result.ThisWeek) != 1 {
			t.Errorf("expected 1 task in ThisWeek, got %d", len(result.ThisWeek))
		}
	})

	t.Run("done tasks are excluded", func(t *testing.T) {
		tasks := []journal.Task{
			{ID: "task1", Text: "Done task", Status: journal.TaskDone, ScheduledDate: &today},
		}
		result := GroupTasksByTime(tasks)
		if len(result.Today) != 0 {
			t.Errorf("expected 0 tasks in Today, got %d", len(result.Today))
		}
	})

	t.Run("future tasks go to AllTasks", func(t *testing.T) {
		futureDate := time.Now().AddDate(0, 0, 30).Format("2006-01-02")
		tasks := []journal.Task{
			{ID: "task1", Text: "Future task", Status: journal.TaskOpen, ScheduledDate: &futureDate},
		}
		result := GroupTasksByTime(tasks)
		if len(result.AllTasks) != 1 {
			t.Errorf("expected 1 task in AllTasks, got %d", len(result.AllTasks))
		}
	})

	t.Run("tasks with both scheduled and deadline", func(t *testing.T) {
		tasks := []journal.Task{
			{ID: "task1", Text: "Task with both dates", Status: journal.TaskOpen, ScheduledDate: &today, DeadlineDate: &tomorrow},
		}
		result := GroupTasksByTime(tasks)
		if len(result.Today) != 1 {
			t.Errorf("expected 1 task in Today, got %d", len(result.Today))
		}
	})
}

func TestTimeGroupedTaskCount(t *testing.T) {
	t.Run("TodayCount counts only open tasks", func(t *testing.T) {
		today := time.Now().Format("2006-01-02")
		tasks := []journal.Task{
			{ID: "task1", Text: "Open task", Status: journal.TaskOpen, ScheduledDate: &today},
			{ID: "task2", Text: "Done task", Status: journal.TaskDone, ScheduledDate: &today},
		}
		grouped := GroupTasksByTime(tasks)
		if grouped.TodayCount() != 1 {
			t.Errorf("expected TodayCount to be 1, got %d", grouped.TodayCount())
		}
	})

	t.Run("ThisWeekCount counts only open tasks", func(t *testing.T) {
		daysUntilFriday := (time.Friday - time.Now().Weekday() + 7) % 7
		if daysUntilFriday == 0 {
			daysUntilFriday = 7
		}
		thisFriday := time.Now().AddDate(0, 0, int(daysUntilFriday)).Format("2006-01-02")
		tasks := []journal.Task{
			{ID: "task1", Text: "Open task", Status: journal.TaskOpen, ScheduledDate: &thisFriday},
			{ID: "task2", Text: "Done task", Status: journal.TaskDone, ScheduledDate: &thisFriday},
		}
		grouped := GroupTasksByTime(tasks)
		if grouped.ThisWeekCount() != 1 {
			t.Errorf("expected ThisWeekCount to be 1, got %d", grouped.ThisWeekCount())
		}
	})

	t.Run("AllTasksCount counts only open tasks", func(t *testing.T) {
		tasks := []journal.Task{
			{ID: "task1", Text: "Open task", Status: journal.TaskOpen},
			{ID: "task2", Text: "Done task", Status: journal.TaskDone},
		}
		grouped := GroupTasksByTime(tasks)
		if grouped.AllTasksCount() != 1 {
			t.Errorf("expected AllTasksCount to be 1, got %d", grouped.AllTasksCount())
		}
	})
}

func TestTimeGroupedTaskList_Navigation(t *testing.T) {
	t.Run("j at end of section wraps to next section", func(t *testing.T) {
		today := time.Now().Format("2006-01-02")
		nextWeek := time.Now().AddDate(0, 0, 5).Format("2006-01-02")
		tasks := []journal.Task{
			{ID: "task1", Text: "Today task 1", Status: journal.TaskOpen, ScheduledDate: &today},
			{ID: "task2", Text: "Today task 2", Status: journal.TaskOpen, ScheduledDate: &today},
			{ID: "task3", Text: "Week task", Status: journal.TaskOpen, ScheduledDate: &nextWeek},
		}

		tgl := NewTimeGroupedTaskList(tasks)
		tgl.focused = true

		if tgl.sectionIndex != 0 {
			t.Errorf("expected initial sectionIndex 0, got %d", tgl.sectionIndex)
		}

		// Select last item in TODAY section
		tgl.list.Select(1)

		// Press j to move down (should wrap to THIS WEEK)
		msg := tea.KeyMsg{Type: tea.KeyDown}
		tgl, _ = tgl.Update(msg)

		if tgl.sectionIndex != 1 {
			t.Errorf("expected sectionIndex 1 after wrapping, got %d", tgl.sectionIndex)
		}
		if tgl.list.Index() != 0 {
			t.Errorf("expected index 0 in new section, got %d", tgl.list.Index())
		}
	})

	t.Run("k at start of section wraps to previous section", func(t *testing.T) {
		today := time.Now().Format("2006-01-02")
		nextWeek := time.Now().AddDate(0, 0, 5).Format("2006-01-02")
		tasks := []journal.Task{
			{ID: "task1", Text: "Today task", Status: journal.TaskOpen, ScheduledDate: &today},
			{ID: "task2", Text: "Week task 1", Status: journal.TaskOpen, ScheduledDate: &nextWeek},
			{ID: "task3", Text: "Week task 2", Status: journal.TaskOpen, ScheduledDate: &nextWeek},
		}

		tgl := NewTimeGroupedTaskList(tasks)
		tgl.focused = true

		// Start in TODAY (first non-empty section)
		if tgl.sectionIndex != 0 {
			t.Errorf("expected initial sectionIndex 0, got %d", tgl.sectionIndex)
		}

		// Move to THIS WEEK section first
		tgl.sectionIndex = 1
		tgl.updateListForSection()

		// Press k to move up (should wrap to TODAY)
		msg := tea.KeyMsg{Type: tea.KeyUp}
		tgl, _ = tgl.Update(msg)

		if tgl.sectionIndex != 0 {
			t.Errorf("expected sectionIndex 0 after wrapping, got %d", tgl.sectionIndex)
		}
		if tgl.list.Index() != 0 {
			t.Errorf("expected index 0 in new section, got %d", tgl.list.Index())
		}
	})

	t.Run("j at end of ALL TASKS wraps to TODAY", func(t *testing.T) {
		today := time.Now().Format("2006-01-02")
		futureDate := time.Now().AddDate(0, 0, 30).Format("2006-01-02")
		tasks := []journal.Task{
			{ID: "task1", Text: "Today task", Status: journal.TaskOpen, ScheduledDate: &today},
			{ID: "task2", Text: "Future task 1", Status: journal.TaskOpen, ScheduledDate: &futureDate},
			{ID: "task3", Text: "Future task 2", Status: journal.TaskOpen, ScheduledDate: &futureDate},
		}

		tgl := NewTimeGroupedTaskList(tasks)
		tgl.focused = true

		// Move to ALL TASKS and select last item
		tgl.sectionIndex = 2
		tgl.updateListForSection()
		tgl.list.Select(1)

		// Press j to wrap to TODAY
		msg := tea.KeyMsg{Type: tea.KeyDown}
		tgl, _ = tgl.Update(msg)

		if tgl.sectionIndex != 0 {
			t.Errorf("expected sectionIndex 0 after wrapping, got %d", tgl.sectionIndex)
		}
	})

	t.Run("k at start of TODAY wraps to ALL TASKS", func(t *testing.T) {
		today := time.Now().Format("2006-01-02")
		futureDate := time.Now().AddDate(0, 0, 30).Format("2006-01-02")
		tasks := []journal.Task{
			{ID: "task1", Text: "Today task", Status: journal.TaskOpen, ScheduledDate: &today},
			{ID: "task2", Text: "Future task", Status: journal.TaskOpen, ScheduledDate: &futureDate},
		}

		tgl := NewTimeGroupedTaskList(tasks)
		tgl.focused = true

		// Press k at start of TODAY (should wrap to ALL TASKS)
		msg := tea.KeyMsg{Type: tea.KeyUp}
		tgl, _ = tgl.Update(msg)

		if tgl.sectionIndex != 2 {
			t.Errorf("expected sectionIndex 2 after wrapping, got %d", tgl.sectionIndex)
		}
	})

	t.Run("empty section is skipped", func(t *testing.T) {
		today := time.Now().Format("2006-01-02")
		tasks := []journal.Task{
			{ID: "task1", Text: "Today task", Status: journal.TaskOpen, ScheduledDate: &today},
			// THIS WEEK is empty
			{ID: "task2", Text: "All tasks", Status: journal.TaskOpen},
		}

		tgl := NewTimeGroupedTaskList(tasks)
		tgl.focused = true

		// Should skip empty THIS WEEK and go directly to ALL TASKS
		if tgl.sectionIndex != 0 {
			t.Errorf("expected initial sectionIndex 0, got %d", tgl.sectionIndex)
		}

		tgl.list.Select(0)
		msg := tea.KeyMsg{Type: tea.KeyDown}
		tgl, _ = tgl.Update(msg)

		// Should wrap from TODAY to ALL TASKS (skipping empty THIS WEEK)
		if tgl.sectionIndex != 2 {
			t.Errorf("expected sectionIndex 2 (skipping empty section), got %d", tgl.sectionIndex)
		}
	})
}

func TestTimeGroupedTaskList_SelectedTask(t *testing.T) {
	t.Run("returns nil for empty list", func(t *testing.T) {
		tgl := NewTimeGroupedTaskList([]journal.Task{})
		if tgl.SelectedTask() != nil {
			t.Error("SelectedTask should return nil for empty list")
		}
	})

	t.Run("returns selected task", func(t *testing.T) {
		today := time.Now().Format("2006-01-02")
		tasks := []journal.Task{
			{ID: "task1", Text: "Today task", Status: journal.TaskOpen, ScheduledDate: &today},
		}
		tgl := NewTimeGroupedTaskList(tasks)

		selected := tgl.SelectedTask()
		if selected == nil {
			t.Fatal("SelectedTask should return a task")
		}
		if selected.ID != "task1" {
			t.Errorf("expected task1, got %s", selected.ID)
		}
	})
}
