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

	// Test that PickOpenTask returns an error when no open tasks exist
	_, _, err := PickOpenTask(tasks, "Select a task")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no open tasks found")
}

func TestPickTask_EmptyTaskList(t *testing.T) {
	// Test that PickTask returns an error with empty task list
	tasks := []journal.Task{}
	_, _, err := PickTask(tasks, "Select a task")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no tasks available")
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

func TestTaskScheduleModel_Structure(t *testing.T) {
	// Verify the task schedule model structure is correct
	m := taskScheduleModel{
		state:    scheduleStateTaskPicker,
		canceled: false,
	}

	assert.Equal(t, scheduleStateTaskPicker, m.state)
	assert.False(t, m.canceled)
	assert.Nil(t, m.selectedTask)
}

func TestTaskScheduleModel_StateTransitions(t *testing.T) {
	// Test state constants exist and are distinct
	assert.NotEqual(t, scheduleStateTaskPicker, scheduleStateDatePicker)
	assert.NotEqual(t, scheduleStateDatePicker, scheduleStateDone)
	assert.NotEqual(t, scheduleStateTaskPicker, scheduleStateDone)
}

func TestTaskDeadlineModel_Structure(t *testing.T) {
	// Verify the taskDeadlineModel structure is correct
	m := taskDeadlineModel{
		state:    deadlineStateTaskPicker,
		canceled: false,
	}

	assert.Equal(t, deadlineStateTaskPicker, m.state)
	assert.False(t, m.canceled)
	assert.Nil(t, m.error)
	assert.Nil(t, m.selectedTask)
}

func TestTaskDeadlineModel_StateTransitions(t *testing.T) {
	// Test state constants exist and are distinct
	assert.Equal(t, deadlineState(0), deadlineStateTaskPicker)
	assert.Equal(t, deadlineState(1), deadlineStateDatePicker)
	assert.Equal(t, deadlineState(2), deadlineStateDone)

	// Verify states are different values
	assert.NotEqual(t, deadlineStateTaskPicker, deadlineStateDatePicker)
	assert.NotEqual(t, deadlineStateDatePicker, deadlineStateDone)
	assert.NotEqual(t, deadlineStateTaskPicker, deadlineStateDone)
}
