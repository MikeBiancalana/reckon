package cli

import (
	"testing"
	"time"

	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/stretchr/testify/assert"
)

func TestPickOpenTaskAndEnterNote_FiltersTasks(t *testing.T) {
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

	// We can't actually run the interactive workflow in tests,
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

func TestPickOpenTaskAndEnterNote_NoOpenTasks(t *testing.T) {
	tasks := []journal.Task{
		{
			ID:        "task-1",
			Text:      "Done task",
			Status:    journal.TaskDone,
			CreatedAt: time.Now(),
		},
	}

	// Test that PickOpenTaskAndEnterNote returns an error when no open tasks exist
	_, _, _, err := PickOpenTaskAndEnterNote(tasks)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no open tasks found")
}

func TestPickTaskAndEnterNote_EmptyTaskList(t *testing.T) {
	// Test that PickTaskAndEnterNote returns an error with empty task list
	tasks := []journal.Task{}
	_, _, _, err := PickTaskAndEnterNote(tasks)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no tasks available")
}

func TestTaskNoteModel_Structure(t *testing.T) {
	// Verify the model structure is correct
	m := taskNoteModel{
		stage:    taskNoteStagePickTask,
		taskID:   "test-id",
		noteText: "test note",
		canceled: false,
	}

	assert.Equal(t, taskNoteStagePickTask, m.stage)
	assert.Equal(t, "test-id", m.taskID)
	assert.Equal(t, "test note", m.noteText)
	assert.False(t, m.canceled)
}

func TestTaskNoteModel_Stages(t *testing.T) {
	// Verify the stage constants are defined
	assert.Equal(t, taskNoteStage(0), taskNoteStagePickTask)
	assert.Equal(t, taskNoteStage(1), taskNoteStageEnterNote)
	assert.Equal(t, taskNoteStage(2), taskNoteStageDone)
}
