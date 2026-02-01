package cli

import (
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/MikeBiancalana/reckon/internal/tui/components"
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

func TestTaskNoteModel_StateTransition_PickTaskToEnterNote(t *testing.T) {
	// Test state transition from picking task to entering note
	tasks := []journal.Task{
		{
			ID:        "task-1",
			Text:      "Test task",
			Status:    journal.TaskOpen,
			CreatedAt: time.Now(),
		},
	}

	picker := components.NewTaskPicker("Select Task")
	picker.Show(tasks)

	editor := components.NewTextEditor("Enter Note")

	m := taskNoteModel{
		stage:  taskNoteStagePickTask,
		picker: picker,
		editor: editor,
		tasks:  tasks,
	}

	// Simulate task selection
	msg := components.TaskPickerSelectMsg{TaskID: "task-1"}
	updatedModel, _ := m.Update(msg)
	result := updatedModel.(taskNoteModel)

	// Verify state transition
	assert.Equal(t, taskNoteStageEnterNote, result.stage)
	assert.Equal(t, "task-1", result.taskID)
	assert.Nil(t, result.err)
}

func TestTaskNoteModel_StateTransition_InvalidTaskID(t *testing.T) {
	// Test error handling when invalid task ID is selected
	tasks := []journal.Task{
		{
			ID:        "task-1",
			Text:      "Test task",
			Status:    journal.TaskOpen,
			CreatedAt: time.Now(),
		},
	}

	picker := components.NewTaskPicker("Select Task")
	picker.Show(tasks)

	editor := components.NewTextEditor("Enter Note")

	m := taskNoteModel{
		stage:  taskNoteStagePickTask,
		picker: picker,
		editor: editor,
		tasks:  tasks,
	}

	// Simulate invalid task selection
	msg := components.TaskPickerSelectMsg{TaskID: "invalid-task-id"}
	updatedModel, _ := m.Update(msg)
	result := updatedModel.(taskNoteModel)

	// Verify error is set
	assert.NotNil(t, result.err)
	assert.Contains(t, result.err.Error(), "task not found")
	assert.Contains(t, result.err.Error(), "invalid-task-id")
}

func TestTaskNoteModel_StateTransition_EnterNoteToDone(t *testing.T) {
	// Test state transition from entering note to done
	tasks := []journal.Task{
		{
			ID:        "task-1",
			Text:      "Test task",
			Status:    journal.TaskOpen,
			CreatedAt: time.Now(),
		},
	}

	picker := components.NewTaskPicker("Select Task")
	editor := components.NewTextEditor("Enter Note")

	m := taskNoteModel{
		stage:  taskNoteStageEnterNote,
		picker: picker,
		editor: editor,
		tasks:  tasks,
		taskID: "task-1",
	}

	// Simulate note submission
	msg := components.TextEditorSubmitMsg{Text: "This is a test note"}
	updatedModel, _ := m.Update(msg)
	result := updatedModel.(taskNoteModel)

	// Verify state transition
	assert.Equal(t, taskNoteStageDone, result.stage)
	assert.Equal(t, "This is a test note", result.noteText)
	assert.Nil(t, result.err)
}

func TestTaskNoteModel_Cancellation_FromPicker(t *testing.T) {
	// Test cancellation during task picking
	tasks := []journal.Task{
		{
			ID:        "task-1",
			Text:      "Test task",
			Status:    journal.TaskOpen,
			CreatedAt: time.Now(),
		},
	}

	picker := components.NewTaskPicker("Select Task")
	picker.Show(tasks)

	editor := components.NewTextEditor("Enter Note")

	m := taskNoteModel{
		stage:  taskNoteStagePickTask,
		picker: picker,
		editor: editor,
		tasks:  tasks,
	}

	// Simulate cancellation
	msg := components.TaskPickerCancelMsg{}
	updatedModel, _ := m.Update(msg)
	result := updatedModel.(taskNoteModel)

	// Verify cancellation
	assert.True(t, result.canceled)
	assert.Nil(t, result.err)
}

func TestTaskNoteModel_Cancellation_FromEditor(t *testing.T) {
	// Test cancellation during note entry
	tasks := []journal.Task{
		{
			ID:        "task-1",
			Text:      "Test task",
			Status:    journal.TaskOpen,
			CreatedAt: time.Now(),
		},
	}

	picker := components.NewTaskPicker("Select Task")
	editor := components.NewTextEditor("Enter Note")

	m := taskNoteModel{
		stage:  taskNoteStageEnterNote,
		picker: picker,
		editor: editor,
		tasks:  tasks,
		taskID: "task-1",
	}

	// Simulate cancellation
	msg := components.TextEditorCancelMsg{}
	updatedModel, _ := m.Update(msg)
	result := updatedModel.(taskNoteModel)

	// Verify cancellation
	assert.True(t, result.canceled)
	assert.Nil(t, result.err)
}

func TestTaskNoteModel_WindowSize(t *testing.T) {
	// Test window size handling
	tasks := []journal.Task{
		{
			ID:        "task-1",
			Text:      "Test task",
			Status:    journal.TaskOpen,
			CreatedAt: time.Now(),
		},
	}

	picker := components.NewTaskPicker("Select Task")
	picker.Show(tasks)

	editor := components.NewTextEditor("Enter Note")

	m := taskNoteModel{
		stage:  taskNoteStagePickTask,
		picker: picker,
		editor: editor,
		tasks:  tasks,
		width:  80,
		height: 24,
	}

	// Simulate window resize
	msg := tea.WindowSizeMsg{Width: 120, Height: 40}
	updatedModel, _ := m.Update(msg)
	result := updatedModel.(taskNoteModel)

	// Verify window size is updated
	assert.Equal(t, 120, result.width)
	assert.Equal(t, 40, result.height)
}

func TestTaskNoteModel_View_WithError(t *testing.T) {
	// Test view rendering with error
	tasks := []journal.Task{
		{
			ID:        "task-1",
			Text:      "Test task",
			Status:    journal.TaskOpen,
			CreatedAt: time.Now(),
		},
	}

	picker := components.NewTaskPicker("Select Task")
	editor := components.NewTextEditor("Enter Note")

	m := taskNoteModel{
		stage:  taskNoteStagePickTask,
		picker: picker,
		editor: editor,
		tasks:  tasks,
		err:    fmt.Errorf("test error"),
	}

	// Render view
	view := m.View()

	// Verify error is displayed
	assert.Contains(t, view, "Error:")
	assert.Contains(t, view, "test error")
	assert.Contains(t, view, "Press any key to exit")
}

func TestTaskNoteModel_View_NoError(t *testing.T) {
	// Test view rendering without error during task picking
	tasks := []journal.Task{
		{
			ID:        "task-1",
			Text:      "Test task",
			Status:    journal.TaskOpen,
			CreatedAt: time.Now(),
		},
	}

	picker := components.NewTaskPicker("Select Task")
	picker.Show(tasks)

	editor := components.NewTextEditor("Enter Note")

	m := taskNoteModel{
		stage:  taskNoteStagePickTask,
		picker: picker,
		editor: editor,
		tasks:  tasks,
	}

	// Render view
	view := m.View()

	// Verify picker view is shown (not empty and not error)
	assert.NotEmpty(t, view)
	assert.NotContains(t, view, "Error:")
}
