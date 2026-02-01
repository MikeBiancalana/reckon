package components

import (
	"testing"
	"time"

	"github.com/MikeBiancalana/reckon/internal/journal"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestNewTaskPicker(t *testing.T) {
	picker := NewTaskPicker("Select a task")
	assert.NotNil(t, picker)
	assert.Equal(t, "Select a task", picker.title)
	assert.False(t, picker.visible)
	assert.Nil(t, picker.selectedTask)
}

func TestTaskPicker_Show(t *testing.T) {
	tasks := []journal.Task{
		{ID: "task-1", Text: "Write tests", Status: journal.TaskOpen},
		{ID: "task-2", Text: "Implement feature", Status: journal.TaskOpen},
	}

	picker := NewTaskPicker("Select a task")
	picker.Show(tasks)

	assert.True(t, picker.visible)
	assert.Len(t, picker.tasks, 2)
}

func TestTaskPicker_ShowWithEmptyTasks(t *testing.T) {
	picker := NewTaskPicker("Select a task")
	picker.Show([]journal.Task{})

	assert.True(t, picker.visible)
	assert.Len(t, picker.tasks, 0)
}

func TestTaskPicker_Hide(t *testing.T) {
	picker := NewTaskPicker("Select a task")
	picker.visible = true
	picker.Hide()

	assert.False(t, picker.visible)
	assert.Nil(t, picker.selectedTask)
}

func TestTaskPicker_IsVisible(t *testing.T) {
	picker := NewTaskPicker("Select a task")
	assert.False(t, picker.IsVisible())

	picker.visible = true
	assert.True(t, picker.IsVisible())
}

func TestTaskPicker_GetSelectedTaskID(t *testing.T) {
	picker := NewTaskPicker("Select a task")

	// No selection initially
	taskID := picker.GetSelectedTaskID()
	assert.Equal(t, "", taskID)

	// After selection
	selectedTask := &journal.Task{ID: "task-1", Text: "Test task"}
	picker.selectedTask = selectedTask
	taskID = picker.GetSelectedTaskID()
	assert.Equal(t, "task-1", taskID)
}

func TestTaskPicker_UpdateWithEscapeKey(t *testing.T) {
	tasks := []journal.Task{
		{ID: "task-1", Text: "Write tests", Status: journal.TaskOpen},
	}

	picker := NewTaskPicker("Select a task")
	picker.Show(tasks)

	msg := tea.KeyMsg{Type: tea.KeyEsc}
	_, cmd := picker.Update(msg)

	assert.False(t, picker.visible)
	assert.NotNil(t, cmd)

	// Execute the command to get the cancel message
	result := cmd()
	_, isCancel := result.(TaskPickerCancelMsg)
	assert.True(t, isCancel)
}

func TestTaskPicker_UpdateWithEnterKey(t *testing.T) {
	tasks := []journal.Task{
		{ID: "task-1", Text: "Write tests", Status: journal.TaskOpen},
		{ID: "task-2", Text: "Implement feature", Status: journal.TaskOpen},
	}

	picker := NewTaskPicker("Select a task")
	picker.Show(tasks)

	// Simulate selecting first item (list initialized with tasks)
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := picker.Update(msg)

	assert.NotNil(t, cmd)

	// Execute the command to get the select message
	result := cmd()
	selectMsg, isSelect := result.(TaskPickerSelectMsg)
	assert.True(t, isSelect)
	assert.Equal(t, "task-1", selectMsg.TaskID)
}

func TestTaskPicker_UpdateWhenNotVisible(t *testing.T) {
	picker := NewTaskPicker("Select a task")

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := picker.Update(msg)

	assert.False(t, picker.visible)
	assert.Nil(t, cmd)
}

func TestTaskPicker_FilterValue(t *testing.T) {
	task := journal.Task{ID: "task-1", Text: "Write comprehensive tests"}
	item := taskPickerItem{task: task}

	filterValue := item.FilterValue()
	assert.Equal(t, "Write comprehensive tests", filterValue)
}

func TestTaskPicker_ItemDescription(t *testing.T) {
	// Task with tags
	task1 := journal.Task{
		ID:        "task-1",
		Text:      "Write tests",
		Status:    journal.TaskOpen,
		Tags:      []string{"testing", "important"},
		CreatedAt: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
	}
	item1 := taskPickerItem{task: task1}
	desc1 := item1.Description()
	assert.Contains(t, desc1, "testing")
	assert.Contains(t, desc1, "important")

	// Task without tags
	task2 := journal.Task{
		ID:        "task-2",
		Text:      "No tags task",
		Status:    journal.TaskOpen,
		CreatedAt: time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC),
	}
	item2 := taskPickerItem{task: task2}
	desc2 := item2.Description()
	assert.NotEmpty(t, desc2)
}

func TestTaskPicker_FuzzyFiltering(t *testing.T) {
	tasks := []journal.Task{
		{ID: "task-1", Text: "Write comprehensive tests", Status: journal.TaskOpen},
		{ID: "task-2", Text: "Implement fuzzy search", Status: journal.TaskOpen},
		{ID: "task-3", Text: "Review pull request", Status: journal.TaskOpen},
	}

	picker := NewTaskPicker("Select a task")
	picker.Show(tasks)

	// The list component handles filtering internally
	// We just verify that all tasks are loaded
	assert.Len(t, picker.tasks, 3)
}

func TestTaskPicker_SetWidth(t *testing.T) {
	picker := NewTaskPicker("Select a task")
	picker.SetWidth(100)

	assert.Equal(t, 100, picker.width)
}

func TestTaskPicker_ViewWhenNotVisible(t *testing.T) {
	picker := NewTaskPicker("Select a task")
	view := picker.View()

	assert.Equal(t, "", view)
}

func TestTaskPicker_ViewWhenVisible(t *testing.T) {
	tasks := []journal.Task{
		{ID: "task-1", Text: "Write tests", Status: journal.TaskOpen},
	}

	picker := NewTaskPicker("Select a task")
	picker.Show(tasks)
	view := picker.View()

	assert.NotEmpty(t, view)
	assert.Contains(t, view, "Write tests")
}

func TestTaskPickerSelectMsg(t *testing.T) {
	msg := TaskPickerSelectMsg{TaskID: "task-123"}
	assert.Equal(t, "task-123", msg.TaskID)
}

func TestTaskPickerCancelMsg(t *testing.T) {
	msg := TaskPickerCancelMsg{}
	assert.NotNil(t, msg)
}
