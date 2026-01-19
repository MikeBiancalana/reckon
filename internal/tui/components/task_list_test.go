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

func TestTimeGroupedTasks_IsEmpty(t *testing.T) {
	t.Run("empty grouped tasks is empty", func(t *testing.T) {
		grouped := TimeGroupedTasks{}
		if !grouped.IsEmpty() {
			t.Error("empty grouped tasks should be empty")
		}
	})

	t.Run("grouped tasks with tasks is not empty", func(t *testing.T) {
		today := []journal.Task{
			{ID: "t1", Text: "Task 1", Status: journal.TaskOpen},
		}
		grouped := TimeGroupedTasks{Today: today}
		if grouped.IsEmpty() {
			t.Error("grouped tasks with tasks should not be empty")
		}
	})
}

func TestTimeGroupedTasks_CountMethods(t *testing.T) {
	t.Run("TodayCount counts only open tasks", func(t *testing.T) {
		tasks := []journal.Task{
			{ID: "t1", Text: "Open", Status: journal.TaskOpen},
			{ID: "t2", Text: "Done", Status: journal.TaskDone},
			{ID: "t3", Text: "Also Open", Status: journal.TaskOpen},
		}
		grouped := TimeGroupedTasks{Today: tasks}
		if grouped.TodayCount() != 2 {
			t.Errorf("expected 2 open tasks, got %d", grouped.TodayCount())
		}
	})

	t.Run("ThisWeekCount counts only open tasks", func(t *testing.T) {
		tasks := []journal.Task{
			{ID: "t1", Text: "Open", Status: journal.TaskOpen},
			{ID: "t2", Text: "Done", Status: journal.TaskDone},
		}
		grouped := TimeGroupedTasks{ThisWeek: tasks}
		if grouped.ThisWeekCount() != 1 {
			t.Errorf("expected 1 open task, got %d", grouped.ThisWeekCount())
		}
	})

	t.Run("AllTasksCount counts only open tasks", func(t *testing.T) {
		tasks := []journal.Task{
			{ID: "t1", Text: "Open", Status: journal.TaskOpen},
			{ID: "t2", Text: "Done", Status: journal.TaskDone},
			{ID: "t3", Text: "Also Open", Status: journal.TaskOpen},
			{ID: "t4", Text: "Done Too", Status: journal.TaskDone},
		}
		grouped := TimeGroupedTasks{AllTasks: tasks}
		if grouped.AllTasksCount() != 2 {
			t.Errorf("expected 2 open tasks, got %d", grouped.AllTasksCount())
		}
	})
}

func TestGroupTasksByTime(t *testing.T) {
	today := time.Now()
	todayStr := today.Format("2006-01-02")
	tomorrow := today.AddDate(0, 0, 1)
	tomorrowStr := tomorrow.Format("2006-01-02")
	yesterday := today.AddDate(0, 0, -1)
	yesterdayStr := yesterday.Format("2006-01-02")

	nextWeek := today.AddDate(0, 0, 7)
	nextWeekStr := nextWeek.Format("2006-01-02")
	nextMonth := today.AddDate(0, 1, 0)
	nextMonthStr := nextMonth.Format("2006-01-02")

	t.Run("scheduled for today goes to Today", func(t *testing.T) {
		tasks := []journal.Task{
			{ID: "t1", Text: "Today Task", Status: journal.TaskOpen, ScheduledDate: &todayStr},
		}
		grouped := GroupTasksByTime(tasks)

		if len(grouped.Today) != 1 {
			t.Errorf("expected 1 task in Today, got %d", len(grouped.Today))
		}
		if len(grouped.ThisWeek) != 0 {
			t.Errorf("expected 0 tasks in ThisWeek, got %d", len(grouped.ThisWeek))
		}
		if len(grouped.AllTasks) != 0 {
			t.Errorf("expected 0 tasks in AllTasks, got %d", len(grouped.AllTasks))
		}
	})

	t.Run("deadline today goes to Today", func(t *testing.T) {
		tasks := []journal.Task{
			{ID: "t1", Text: "Due Today", Status: journal.TaskOpen, DeadlineDate: &todayStr},
		}
		grouped := GroupTasksByTime(tasks)

		if len(grouped.Today) != 1 {
			t.Errorf("expected 1 task in Today, got %d", len(grouped.Today))
		}
	})

	t.Run("overdue deadline goes to Today", func(t *testing.T) {
		tasks := []journal.Task{
			{ID: "t1", Text: "Overdue", Status: journal.TaskOpen, DeadlineDate: &yesterdayStr},
		}
		grouped := GroupTasksByTime(tasks)

		if len(grouped.Today) != 1 {
			t.Errorf("expected 1 overdue task in Today, got %d", len(grouped.Today))
		}
	})

	t.Run("scheduled for this week goes to ThisWeek", func(t *testing.T) {
		futureThisWeek := today.AddDate(0, 0, 3)
		futureThisWeekStr := futureThisWeek.Format("2006-01-02")
		tasks := []journal.Task{
			{ID: "t1", Text: "This Week Task", Status: journal.TaskOpen, ScheduledDate: &futureThisWeekStr},
		}
		grouped := GroupTasksByTime(tasks)

		if len(grouped.Today) != 0 {
			t.Errorf("expected 0 tasks in Today, got %d", len(grouped.Today))
		}
		if len(grouped.ThisWeek) != 1 {
			t.Errorf("expected 1 task in ThisWeek, got %d", len(grouped.ThisWeek))
		}
		if len(grouped.AllTasks) != 0 {
			t.Errorf("expected 0 tasks in AllTasks, got %d", len(grouped.AllTasks))
		}
	})

	t.Run("deadline this week goes to ThisWeek", func(t *testing.T) {
		futureThisWeek := today.AddDate(0, 0, 4)
		futureThisWeekStr := futureThisWeek.Format("2006-01-02")
		tasks := []journal.Task{
			{ID: "t1", Text: "Due This Week", Status: journal.TaskOpen, DeadlineDate: &futureThisWeekStr},
		}
		grouped := GroupTasksByTime(tasks)

		if len(grouped.ThisWeek) != 1 {
			t.Errorf("expected 1 task in ThisWeek, got %d", len(grouped.ThisWeek))
		}
	})

	t.Run("scheduled for next week goes to AllTasks", func(t *testing.T) {
		tasks := []journal.Task{
			{ID: "t1", Text: "Next Week", Status: journal.TaskOpen, ScheduledDate: &nextWeekStr},
		}
		grouped := GroupTasksByTime(tasks)

		if len(grouped.AllTasks) != 1 {
			t.Errorf("expected 1 task in AllTasks, got %d", len(grouped.AllTasks))
		}
	})

	t.Run("unscheduled tasks go to AllTasks", func(t *testing.T) {
		tasks := []journal.Task{
			{ID: "t1", Text: "Unscheduled", Status: journal.TaskOpen},
		}
		grouped := GroupTasksByTime(tasks)

		if len(grouped.AllTasks) != 1 {
			t.Errorf("expected 1 task in AllTasks, got %d", len(grouped.AllTasks))
		}
	})

	t.Run("done tasks are excluded from all groups", func(t *testing.T) {
		tasks := []journal.Task{
			{ID: "t1", Text: "Done Today", Status: journal.TaskDone, ScheduledDate: &todayStr},
			{ID: "t2", Text: "Done This Week", Status: journal.TaskDone, DeadlineDate: &tomorrowStr},
			{ID: "t3", Text: "Done AllTasks", Status: journal.TaskDone, ScheduledDate: &nextMonthStr},
		}
		grouped := GroupTasksByTime(tasks)

		if len(grouped.Today) != 0 {
			t.Errorf("expected 0 tasks in Today, got %d", len(grouped.Today))
		}
		if len(grouped.ThisWeek) != 0 {
			t.Errorf("expected 0 tasks in ThisWeek, got %d", len(grouped.ThisWeek))
		}
		if len(grouped.AllTasks) != 0 {
			t.Errorf("expected 0 tasks in AllTasks, got %d", len(grouped.AllTasks))
		}
	})

	t.Run("mixed tasks are grouped correctly", func(t *testing.T) {
		futureThisWeek := today.AddDate(0, 0, 3)
		futureThisWeekStr := futureThisWeek.Format("2006-01-02")
		tasks := []journal.Task{
			{ID: "t1", Text: "Today", Status: journal.TaskOpen, ScheduledDate: &todayStr},
			{ID: "t2", Text: "This Week", Status: journal.TaskOpen, ScheduledDate: &futureThisWeekStr},
			{ID: "t3", Text: "All Tasks", Status: journal.TaskOpen, ScheduledDate: &nextMonthStr},
			{ID: "t4", Text: "Unscheduled", Status: journal.TaskOpen},
		}
		grouped := GroupTasksByTime(tasks)

		if len(grouped.Today) != 1 {
			t.Errorf("expected 1 task in Today, got %d", len(grouped.Today))
		}
		if len(grouped.ThisWeek) != 1 {
			t.Errorf("expected 1 task in ThisWeek, got %d", len(grouped.ThisWeek))
		}
		if len(grouped.AllTasks) != 2 {
			t.Errorf("expected 2 tasks in AllTasks, got %d", len(grouped.AllTasks))
		}
	})

	t.Run("both scheduled and deadline set uses whichever applies first", func(t *testing.T) {
		futureThisWeek := today.AddDate(0, 0, 3)
		futureThisWeekStr := futureThisWeek.Format("2006-01-02")
		tasks := []journal.Task{
			{ID: "t1", Text: "Both Today", Status: journal.TaskOpen, ScheduledDate: &todayStr, DeadlineDate: &tomorrowStr},
			{ID: "t2", Text: "Both This Week", Status: journal.TaskOpen, ScheduledDate: &futureThisWeekStr, DeadlineDate: &nextMonthStr},
		}
		grouped := GroupTasksByTime(tasks)

		if len(grouped.Today) != 1 {
			t.Errorf("expected 1 task in Today, got %d", len(grouped.Today))
		}
		if len(grouped.ThisWeek) != 1 {
			t.Errorf("expected 1 task in ThisWeek, got %d", len(grouped.ThisWeek))
		}
	})

	t.Run("empty tasks returns empty groups", func(t *testing.T) {
		grouped := GroupTasksByTime([]journal.Task{})

		if !grouped.IsEmpty() {
			t.Error("empty tasks should result in empty groups")
		}
	})
}

func TestNewTimeGroupedTaskList(t *testing.T) {
	today := time.Now()
	todayStr := today.Format("2006-01-02")

	t.Run("creates with tasks grouped", func(t *testing.T) {
		tasks := []journal.Task{
			{ID: "t1", Text: "Today", Status: journal.TaskOpen, ScheduledDate: &todayStr},
		}
		tgl := NewTimeGroupedTaskList(tasks)

		if tgl == nil {
			t.Fatal("NewTimeGroupedTaskList returned nil")
		}

		if tgl.groupedTasks.TodayCount() != 1 {
			t.Errorf("expected 1 task in Today, got %d", tgl.groupedTasks.TodayCount())
		}
	})

	t.Run("handles empty tasks", func(t *testing.T) {
		tgl := NewTimeGroupedTaskList([]journal.Task{})

		if tgl == nil {
			t.Fatal("NewTimeGroupedTaskList returned nil")
		}

		if !tgl.groupedTasks.IsEmpty() {
			t.Error("empty tasks should result in empty groups")
		}
	})
}
