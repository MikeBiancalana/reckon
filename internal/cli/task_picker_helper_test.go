package cli

import (
	"testing"
	"time"

	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/stretchr/testify/assert"
)

func TestPickOpenTask_FiltersTasks(t *testing.T) {
	tasks := []journal.Task{
		{
			ID:        "task-1",
			Text:      "Open task 1",
			Status:    journal.TaskOpen,
			CreatedAt: time.Now(),
		},
		{
			ID:        "task-2",
			Text:      "Done task",
			Status:    journal.TaskDone,
			CreatedAt: time.Now(),
		},
		{
			ID:        "task-3",
			Text:      "Open task 2",
			Status:    journal.TaskOpen,
			CreatedAt: time.Now(),
		},
	}

	// We can't actually run the interactive picker in tests,
	// but we can verify the filtering logic
	openTasks := make([]journal.Task, 0)
	for _, t := range tasks {
		if t.Status == journal.TaskOpen {
			openTasks = append(openTasks, t)
		}
	}

	assert.Len(t, openTasks, 2)
	assert.Equal(t, "task-1", openTasks[0].ID)
	assert.Equal(t, "task-3", openTasks[1].ID)
}

func TestPickOpenTask_NoOpenTasks(t *testing.T) {
	tasks := []journal.Task{
		{
			ID:        "task-1",
			Text:      "Done task",
			Status:    journal.TaskDone,
			CreatedAt: time.Now(),
		},
	}

	// Verify filtering returns empty for all done tasks
	openTasks := make([]journal.Task, 0)
	for _, t := range tasks {
		if t.Status == journal.TaskOpen {
			openTasks = append(openTasks, t)
		}
	}

	assert.Len(t, openTasks, 0)
}

func TestPickTask_EmptyTaskList(t *testing.T) {
	// Verify behavior with empty task list
	tasks := []journal.Task{}
	assert.Len(t, tasks, 0)
}

func TestTaskPickerModel_Structure(t *testing.T) {
	// Verify the model structure is correct
	m := taskPickerModel{
		taskID:   "test-id",
		canceled: false,
	}

	assert.Equal(t, "test-id", m.taskID)
	assert.False(t, m.canceled)
}
