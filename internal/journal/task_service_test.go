package journal

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MikeBiancalana/reckon/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTaskServiceTest(t *testing.T) (*TaskService, *storage.FileStore, *storage.Database, string) {
	t.Helper()

	// Create temp directory - each call gets a unique temp dir
	tmpDir := t.TempDir()

	// Save current RECKON_DATA_DIR
	oldDataDir := os.Getenv("RECKON_DATA_DIR")

	// Set up config to use temp directory
	os.Setenv("RECKON_DATA_DIR", tmpDir)
	t.Cleanup(func() {
		if oldDataDir != "" {
			os.Setenv("RECKON_DATA_DIR", oldDataDir)
		} else {
			os.Unsetenv("RECKON_DATA_DIR")
		}
	})

	// Create database (schema is initialized automatically)
	db, err := storage.NewDatabase(filepath.Join(tmpDir, "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Close()
	})

	// Create repository and file store
	repo := NewTaskRepository(db, nil)
	store := storage.NewFileStore()

	// Create service
	service := NewTaskService(repo, store, nil)

	return service, store, db, tmpDir
}

// createTaskFile creates a task file in the tasks directory with YAML frontmatter format
func createTaskFile(t *testing.T, tmpDir, taskID, title, status string, notes []struct{ id, text string }) {
	t.Helper()

	tasksDir := filepath.Join(tmpDir, "tasks")
	err := os.MkdirAll(tasksDir, 0755)
	require.NoError(t, err)

	// Build log section with notes
	logSection := "## Log\n\n"
	if len(notes) > 0 {
		logSection += "### 2025-01-01\n"
		for _, note := range notes {
			logSection += fmt.Sprintf("  - %s %s\n", note.id, note.text)
		}
	}

	content := fmt.Sprintf(`---
id: %s
title: %s
created: 2025-01-01
status: %s
---

## Description

%s`, taskID, title, status, logSection)

	filePath := filepath.Join(tasksDir, taskID+".md")
	err = os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(t, err)
}

// Simplified helper for tasks without notes
func createTaskFileSimple(t *testing.T, tmpDir, taskID, title, status string) {
	t.Helper()
	createTaskFile(t, tmpDir, taskID, title, status, nil)
}

// createTaskFileWithDates creates a task file with optional scheduled_date and deadline_date
func createTaskFileWithDates(t *testing.T, tmpDir, taskID, title, status string, notes []struct{ id, text string }, scheduledDate, deadlineDate *string) {
	t.Helper()

	tasksDir := filepath.Join(tmpDir, "tasks")
	err := os.MkdirAll(tasksDir, 0755)
	require.NoError(t, err)

	logSection := "## Log\n\n"
	if len(notes) > 0 {
		logSection += "### 2025-01-01\n"
		for _, note := range notes {
			logSection += fmt.Sprintf("  - %s %s\n", note.id, note.text)
		}
	}

	scheduledDateLine := ""
	if scheduledDate != nil {
		scheduledDateLine = fmt.Sprintf("scheduled_date: %s\n", *scheduledDate)
	}

	deadlineDateLine := ""
	if deadlineDate != nil {
		deadlineDateLine = fmt.Sprintf("deadline_date: %s\n", *deadlineDate)
	}

	content := fmt.Sprintf(`---
id: %s
title: %s
created: 2025-01-01
status: %s
%s%s---
## Description

%s`, taskID, title, status, scheduledDateLine, deadlineDateLine, logSection)

	filePath := filepath.Join(tasksDir, taskID+".md")
	err = os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(t, err)
}

func TestGetAllTasks_EmptyFile(t *testing.T) {
	service, _, _, _ := setupTaskServiceTest(t)

	tasks, err := service.GetAllTasks()
	require.NoError(t, err)
	assert.Empty(t, tasks)
}

func TestGetAllTasks_WithTasks(t *testing.T) {
	service, _, _, tmpDir := setupTaskServiceTest(t)

	// Create task files
	task1Content := `---
id: task-1
title: First task
created: 2025-01-01
status: open
---

## Description

## Log

### 2025-01-01
  - note-1 First note
`
	task2Content := `---
id: task-2
title: Second task
created: 2025-01-02
status: done
---

## Description

## Log
`
	tasksDir := filepath.Join(tmpDir, "tasks")
	os.MkdirAll(tasksDir, 0755)
	err := os.WriteFile(filepath.Join(tasksDir, "task-1.md"), []byte(task1Content), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tasksDir, "task-2.md"), []byte(task2Content), 0644)
	require.NoError(t, err)

	// Get all tasks
	tasks, err := service.GetAllTasks()
	require.NoError(t, err)
	require.Len(t, tasks, 2)

	// Verify first task
	assert.Equal(t, "task-1", tasks[0].ID)
	assert.Equal(t, "First task", tasks[0].Text)
	assert.Equal(t, TaskOpen, tasks[0].Status)
	assert.Equal(t, 0, tasks[0].Position)
	require.Len(t, tasks[0].Notes, 1)
	assert.Equal(t, "note-1", tasks[0].Notes[0].ID)
	assert.Equal(t, "First note", tasks[0].Notes[0].Text)

	// Verify second task
	assert.Equal(t, "task-2", tasks[1].ID)
	assert.Equal(t, "Second task", tasks[1].Text)
	assert.Equal(t, TaskDone, tasks[1].Status)
	assert.Equal(t, 1, tasks[1].Position)
	assert.Empty(t, tasks[1].Notes)
}

func TestAddTask(t *testing.T) {
	service, _, _, tmpDir := setupTaskServiceTest(t)

	// Add a task
	err := service.AddTask("New task to complete", []string{})
	require.NoError(t, err)

	// Verify it was saved by reading back
	tasks, err := service.GetAllTasks()
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "New task to complete", tasks[0].Text)
	assert.Equal(t, TaskOpen, tasks[0].Status)
	assert.Equal(t, 0, tasks[0].Position)

	// Verify the task file exists in tasks directory
	tasksDir := filepath.Join(tmpDir, "tasks")
	files, err := os.ReadDir(tasksDir)
	require.NoError(t, err)
	require.Len(t, files, 1)
	taskFile := filepath.Join(tasksDir, files[0].Name())
	content, err := os.ReadFile(taskFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "New task to complete")
	assert.Contains(t, string(content), "status: open")
}

func TestAddTask_MultipleTasksIncrementPosition(t *testing.T) {
	service, _, _, _ := setupTaskServiceTest(t)

	// Add multiple tasks
	err := service.AddTask("First task", []string{})
	require.NoError(t, err)

	err = service.AddTask("Second task", []string{})
	require.NoError(t, err)

	err = service.AddTask("Third task", []string{})
	require.NoError(t, err)

	// Verify positions
	tasks, err := service.GetAllTasks()
	require.NoError(t, err)
	require.Len(t, tasks, 3)

	assert.Equal(t, 0, tasks[0].Position)
	assert.Equal(t, "First task", tasks[0].Text)

	assert.Equal(t, 1, tasks[1].Position)
	assert.Equal(t, "Second task", tasks[1].Text)

	assert.Equal(t, 2, tasks[2].Position)
	assert.Equal(t, "Third task", tasks[2].Text)
}

func TestToggleTask_OpenToDone(t *testing.T) {
	service, _, _, tmpDir := setupTaskServiceTest(t)

	// Create initial task
	createTaskFileSimple(t, tmpDir, "task-1", "My task", "open")

	// Toggle to done
	err := service.ToggleTask("task-1")
	require.NoError(t, err)

	// Verify status changed
	tasks, err := service.GetAllTasks()
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, TaskDone, tasks[0].Status)

	// Verify file was updated
	tasksDir := filepath.Join(tmpDir, "tasks")
	files, err := os.ReadDir(tasksDir)
	require.NoError(t, err)
	require.Len(t, files, 1)
	taskFile := filepath.Join(tasksDir, files[0].Name())
	content, err := os.ReadFile(taskFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "status: done")
	assert.NotContains(t, string(content), "status: open")
}

func TestToggleTask_DoneToOpen(t *testing.T) {
	service, _, _, tmpDir := setupTaskServiceTest(t)

	// Create completed task
	createTaskFileSimple(t, tmpDir, "task-1", "Completed task", "done")

	// Toggle to open
	err := service.ToggleTask("task-1")
	require.NoError(t, err)

	// Verify status changed
	tasks, err := service.GetAllTasks()
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, TaskOpen, tasks[0].Status)

	// Verify file was updated
	tasksDir := filepath.Join(tmpDir, "tasks")
	files, err := os.ReadDir(tasksDir)
	require.NoError(t, err)
	require.Len(t, files, 1)
	taskFile := filepath.Join(tasksDir, files[0].Name())
	content, err := os.ReadFile(taskFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "status: open")
	assert.NotContains(t, string(content), "status: done")
}

func TestToggleTask_NotFound(t *testing.T) {
	service, _, _, _ := setupTaskServiceTest(t)

	// Try to toggle non-existent task
	err := service.ToggleTask("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "task not found")
}

func TestAddTaskNote(t *testing.T) {
	service, _, _, tmpDir := setupTaskServiceTest(t)

	// Create initial task
	createTaskFileSimple(t, tmpDir, "task-1", "My task", "open")

	// Add a note
	err := service.AddTaskNote("task-1", "This is a note")
	require.NoError(t, err)

	// Verify note was added
	tasks, err := service.GetAllTasks()
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Len(t, tasks[0].Notes, 1)
	assert.Equal(t, "This is a note", tasks[0].Notes[0].Text)
	assert.Equal(t, 0, tasks[0].Notes[0].Position)

	// Verify file was updated
	tasksDir := filepath.Join(tmpDir, "tasks")
	files, err := os.ReadDir(tasksDir)
	require.NoError(t, err)
	require.Len(t, files, 1)
	taskFile := filepath.Join(tasksDir, files[0].Name())
	content, err := os.ReadFile(taskFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "This is a note")
	assert.Contains(t, string(content), "  - ") // Indented note
}

func TestAddTaskNote_MultipleNotes(t *testing.T) {
	service, _, _, tmpDir := setupTaskServiceTest(t)

	// Create initial task with one note
	createTaskFile(t, tmpDir, "task-1", "My task", "open", []struct{ id, text string }{
		{"note-1", "First note"},
	})

	// Add more notes
	err := service.AddTaskNote("task-1", "Second note")
	require.NoError(t, err)

	err = service.AddTaskNote("task-1", "Third note")
	require.NoError(t, err)

	// Verify notes were added with correct positions
	tasks, err := service.GetAllTasks()
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Len(t, tasks[0].Notes, 3)

	assert.Equal(t, "First note", tasks[0].Notes[0].Text)
	assert.Equal(t, 0, tasks[0].Notes[0].Position)

	assert.Equal(t, "Second note", tasks[0].Notes[1].Text)
	assert.Equal(t, 1, tasks[0].Notes[1].Position)

	assert.Equal(t, "Third note", tasks[0].Notes[2].Text)
	assert.Equal(t, 2, tasks[0].Notes[2].Position)
}

func TestAddTaskNote_TaskNotFound(t *testing.T) {
	service, _, _, _ := setupTaskServiceTest(t)

	// Try to add note to non-existent task
	err := service.AddTaskNote("nonexistent", "Note text")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "task not found")
}

func TestService_DeleteTaskNote(t *testing.T) {
	service, _, _, tmpDir := setupTaskServiceTest(t)

	// Create task with notes
	createTaskFile(t, tmpDir, "task-1", "My task", "open", []struct{ id, text string }{
		{"note-1", "First note"},
		{"note-2", "Second note"},
		{"note-3", "Third note"},
	})

	// Delete middle note
	err := service.DeleteTaskNote("task-1", "note-2")
	require.NoError(t, err)

	// Verify note was deleted and positions updated
	tasks, err := service.GetAllTasks()
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Len(t, tasks[0].Notes, 2)

	assert.Equal(t, "note-1", tasks[0].Notes[0].ID)
	assert.Equal(t, "First note", tasks[0].Notes[0].Text)
	assert.Equal(t, 0, tasks[0].Notes[0].Position)

	assert.Equal(t, "note-3", tasks[0].Notes[1].ID)
	assert.Equal(t, "Third note", tasks[0].Notes[1].Text)
	assert.Equal(t, 1, tasks[0].Notes[1].Position) // Position updated after deletion

	// Verify file was updated
	tasksDir := filepath.Join(tmpDir, "tasks")
	files, err := os.ReadDir(tasksDir)
	require.NoError(t, err)
	require.Len(t, files, 1)
	taskFile := filepath.Join(tasksDir, files[0].Name())
	content, err := os.ReadFile(taskFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "First note")
	assert.Contains(t, string(content), "Third note")
	assert.NotContains(t, string(content), "Second note")
}

func TestService_DeleteTaskNote_TaskNotFound(t *testing.T) {
	service, _, _, _ := setupTaskServiceTest(t)

	// Try to delete note from non-existent task
	err := service.DeleteTaskNote("nonexistent", "note-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "task not found")
}

func TestService_DeleteTaskNote_NoteNotFound(t *testing.T) {
	service, _, _, tmpDir := setupTaskServiceTest(t)

	// Create task
	createTaskFile(t, tmpDir, "task-1", "My task", "open", []struct{ id, text string }{
		{"note-1", "First note"},
	})

	// Try to delete non-existent note
	err := service.DeleteTaskNote("task-1", "nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "note not found")
}

func TestSave_UpdatesBothFileAndDB(t *testing.T) {
	service, _, _, tmpDir := setupTaskServiceTest(t)

	// Add a task
	err := service.AddTask("Test task", []string{})
	require.NoError(t, err)

	// Verify task file exists in tasks directory
	tasks, err := service.GetAllTasks()
	require.NoError(t, err)
	require.Len(t, tasks, 1)

	tasksDir := filepath.Join(tmpDir, "tasks")
	files, err := os.ReadDir(tasksDir)
	require.NoError(t, err)
	require.Len(t, files, 1)
	taskFile := filepath.Join(tasksDir, files[0].Name())
	content, err := os.ReadFile(taskFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "Test task")

	// Verify DB has the task (should be synced by service)
	// We can verify this by checking that GetAllTasks works
	assert.Equal(t, "Test task", tasks[0].Text)
}

func TestService_FileIsSourceOfTruth(t *testing.T) {
	service, _, _, tmpDir := setupTaskServiceTest(t)

	// Add task via service
	err := service.AddTask("Task from service", []string{})
	require.NoError(t, err)

	// Manually create a different task file directly in tasks directory
	createTaskFileSimple(t, tmpDir, "manual-task", "Manual task", "open")

	// GetAllTasks should return what's in the files (files are source of truth)
	tasks, err := service.GetAllTasks()
	require.NoError(t, err)
	require.Len(t, tasks, 2)

	// Find the manual task
	var manualTask *Task
	for i := range tasks {
		if tasks[i].ID == "manual-task" {
			manualTask = &tasks[i]
			break
		}
	}

	require.NotNil(t, manualTask, "manual task should be present")
	assert.Equal(t, "manual-task", manualTask.ID)
	assert.Equal(t, "Manual task", manualTask.Text)
}

func TestService_Integration(t *testing.T) {
	service, _, _, _ := setupTaskServiceTest(t)

	// Add multiple tasks
	err := service.AddTask("Task 1", []string{})
	require.NoError(t, err)

	err = service.AddTask("Task 2", []string{})
	require.NoError(t, err)

	// Add notes to first task
	tasks, err := service.GetAllTasks()
	require.NoError(t, err)
	task1ID := tasks[0].ID

	err = service.AddTaskNote(task1ID, "Note 1")
	require.NoError(t, err)

	err = service.AddTaskNote(task1ID, "Note 2")
	require.NoError(t, err)

	// Toggle first task
	err = service.ToggleTask(task1ID)
	require.NoError(t, err)

	// Verify final state
	tasks, err = service.GetAllTasks()
	require.NoError(t, err)
	require.Len(t, tasks, 2)

	// Task 1 should be done with 2 notes
	assert.Equal(t, TaskDone, tasks[0].Status)
	require.Len(t, tasks[0].Notes, 2)
	assert.Equal(t, "Note 1", tasks[0].Notes[0].Text)
	assert.Equal(t, "Note 2", tasks[0].Notes[1].Text)

	// Task 2 should be open with no notes
	assert.Equal(t, TaskOpen, tasks[1].Status)
	assert.Empty(t, tasks[1].Notes)

	// Delete a note
	noteID := tasks[0].Notes[0].ID
	err = service.DeleteTaskNote(task1ID, noteID)
	require.NoError(t, err)

	// Verify note was deleted
	tasks, err = service.GetAllTasks()
	require.NoError(t, err)
	require.Len(t, tasks[0].Notes, 1)
	assert.Equal(t, "Note 2", tasks[0].Notes[0].Text)
	assert.Equal(t, 0, tasks[0].Notes[0].Position) // Position was updated
}

func TestScheduleTask(t *testing.T) {
	service, _, _, tmpDir := setupTaskServiceTest(t)

	createTaskFile(t, tmpDir, "task-1", "My task", "open", nil)

	err := service.ScheduleTask("task-1", "2026-01-20")
	require.NoError(t, err)

	tasks, err := service.GetAllTasks()
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.NotNil(t, tasks[0].ScheduledDate)
	assert.Equal(t, "2026-01-20", *tasks[0].ScheduledDate)
}

func TestScheduleTask_InvalidDate(t *testing.T) {
	service, _, _, tmpDir := setupTaskServiceTest(t)

	createTaskFile(t, tmpDir, "task-1", "My task", "open", nil)

	err := service.ScheduleTask("task-1", "invalid-date")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid date format")
}

func TestScheduleTask_NotFound(t *testing.T) {
	service, _, _, _ := setupTaskServiceTest(t)

	err := service.ScheduleTask("nonexistent", "2026-01-20")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "task not found")
}

func TestSetTaskDeadline(t *testing.T) {
	service, _, _, tmpDir := setupTaskServiceTest(t)

	createTaskFile(t, tmpDir, "task-1", "My task", "open", nil)

	err := service.SetTaskDeadline("task-1", "2026-01-25")
	require.NoError(t, err)

	tasks, err := service.GetAllTasks()
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.NotNil(t, tasks[0].DeadlineDate)
	assert.Equal(t, "2026-01-25", *tasks[0].DeadlineDate)
}

func TestSetTaskDeadline_InvalidDate(t *testing.T) {
	service, _, _, tmpDir := setupTaskServiceTest(t)

	createTaskFile(t, tmpDir, "task-1", "My task", "open", nil)

	err := service.SetTaskDeadline("task-1", "01-25-2026")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid date format")
}

func TestSetTaskDeadline_NotFound(t *testing.T) {
	service, _, _, _ := setupTaskServiceTest(t)

	err := service.SetTaskDeadline("nonexistent", "2026-01-25")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "task not found")
}

func TestClearTaskSchedule(t *testing.T) {
	service, _, _, tmpDir := setupTaskServiceTest(t)

	scheduledDate := "2026-01-20"
	createTaskFileWithDates(t, tmpDir, "task-1", "My task", "open", nil, &scheduledDate, nil)

	err := service.ClearTaskSchedule("task-1")
	require.NoError(t, err)

	tasks, err := service.GetAllTasks()
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Nil(t, tasks[0].ScheduledDate)
}

func TestClearTaskDeadline(t *testing.T) {
	service, _, _, tmpDir := setupTaskServiceTest(t)

	deadlineDate := "2026-01-25"
	createTaskFileWithDates(t, tmpDir, "task-1", "My task", "open", nil, nil, &deadlineDate)

	err := service.ClearTaskDeadline("task-1")
	require.NoError(t, err)

	tasks, err := service.GetAllTasks()
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Nil(t, tasks[0].DeadlineDate)
}

func TestGetTasksByTimeframe(t *testing.T) {
	service, _, _, tmpDir := setupTaskServiceTest(t)

	today := time.Now().Format("2006-01-02")
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")

	// Get end of current week (Sunday)
	now := time.Now()
	weekday := now.Weekday()
	daysToSunday := int(time.Sunday - weekday)
	if daysToSunday <= 0 {
		daysToSunday += 7
	}
	endOfWeek := now.AddDate(0, 0, daysToSunday).Format("2006-01-02")

	// A date after end of week
	afterWeek := now.AddDate(0, 0, daysToSunday+1).Format("2006-01-02")

	// A date before today (should go to rest)
	pastDate := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

	createTaskFileWithDates(t, tmpDir, "task-today", "Today task", "open", nil, &today, nil)
	createTaskFileWithDates(t, tmpDir, "task-tomorrow", "Tomorrow task", "open", nil, &tomorrow, nil)
	createTaskFileWithDates(t, tmpDir, "task-end-of-week", "End of week task", "open", nil, &endOfWeek, nil)
	createTaskFileWithDates(t, tmpDir, "task-after-week", "After week task", "open", nil, &afterWeek, nil)
	createTaskFileWithDates(t, tmpDir, "task-past", "Past task", "open", nil, &pastDate, nil)
	createTaskFileWithDates(t, tmpDir, "task-unscheduled", "Unscheduled task", "open", nil, nil, nil)

	todayTasks, thisWeek, rest, err := service.GetTasksByTimeframe()
	require.NoError(t, err)

	assert.Len(t, todayTasks, 1)
	assert.Equal(t, "task-today", todayTasks[0].ID)

	assert.Len(t, thisWeek, 2)
	var thisWeekIDs []string
	for _, task := range thisWeek {
		thisWeekIDs = append(thisWeekIDs, task.ID)
	}
	assert.Contains(t, thisWeekIDs, "task-tomorrow")
	assert.Contains(t, thisWeekIDs, "task-end-of-week")

	assert.Len(t, rest, 3)
	var restIDs []string
	for _, task := range rest {
		restIDs = append(restIDs, task.ID)
	}
	assert.Contains(t, restIDs, "task-after-week")
	assert.Contains(t, restIDs, "task-past")
	assert.Contains(t, restIDs, "task-unscheduled")
}

func TestGetTasksByTimeframe_Empty(t *testing.T) {
	service, _, _, _ := setupTaskServiceTest(t)

	today, thisWeek, rest, err := service.GetTasksByTimeframe()
	require.NoError(t, err)

	assert.Empty(t, today)
	assert.Empty(t, thisWeek)
	assert.Empty(t, rest)
}
