package journal

import (
	"testing"
	"time"

	"github.com/MikeBiancalana/reckon/internal/storage"
)

func setupTestDB(t *testing.T) *storage.Database {
	t.Helper()
	db, err := storage.NewDatabase(":memory:")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	return db
}

func TestNewTaskRepository(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewTaskRepository(db)
	if repo == nil {
		t.Fatal("expected non-nil repository")
	}
	if repo.db == nil {
		t.Fatal("expected repository to have database")
	}
}

func TestSaveTask_NewTask(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewTaskRepository(db)

	task := &Task{
		ID:        "task-123",
		Text:      "Test task",
		Status:    TaskOpen,
		Position:  0,
		CreatedAt: time.Now(),
		Notes: []TaskNote{
			{ID: "note-1", Text: "First note", Position: 0},
			{ID: "note-2", Text: "Second note", Position: 1},
		},
	}

	err := repo.SaveTask(task)
	if err != nil {
		t.Fatalf("failed to save task: %v", err)
	}

	// Verify task was saved
	loaded, err := repo.GetTaskByID("task-123")
	if err != nil {
		t.Fatalf("failed to load task: %v", err)
	}

	if loaded.ID != task.ID {
		t.Errorf("expected ID %s, got %s", task.ID, loaded.ID)
	}
	if loaded.Text != task.Text {
		t.Errorf("expected text %s, got %s", task.Text, loaded.Text)
	}
	if loaded.Status != task.Status {
		t.Errorf("expected status %s, got %s", task.Status, loaded.Status)
	}
	if len(loaded.Notes) != 2 {
		t.Fatalf("expected 2 notes, got %d", len(loaded.Notes))
	}
	if loaded.Notes[0].Text != "First note" {
		t.Errorf("expected first note text 'First note', got %s", loaded.Notes[0].Text)
	}
	if loaded.Notes[1].Text != "Second note" {
		t.Errorf("expected second note text 'Second note', got %s", loaded.Notes[1].Text)
	}
}

func TestSaveTask_UpdateExisting(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewTaskRepository(db)

	// Save initial task
	task := &Task{
		ID:        "task-123",
		Text:      "Original text",
		Status:    TaskOpen,
		Position:  0,
		CreatedAt: time.Now(),
		Notes: []TaskNote{
			{ID: "note-1", Text: "Original note", Position: 0},
		},
	}
	err := repo.SaveTask(task)
	if err != nil {
		t.Fatalf("failed to save initial task: %v", err)
	}

	// Update task
	task.Text = "Updated text"
	task.Status = TaskDone
	task.Notes = []TaskNote{
		{ID: "note-2", Text: "New note", Position: 0},
		{ID: "note-3", Text: "Another note", Position: 1},
	}
	err = repo.SaveTask(task)
	if err != nil {
		t.Fatalf("failed to update task: %v", err)
	}

	// Verify updates
	loaded, err := repo.GetTaskByID("task-123")
	if err != nil {
		t.Fatalf("failed to load task: %v", err)
	}

	if loaded.Text != "Updated text" {
		t.Errorf("expected updated text, got %s", loaded.Text)
	}
	if loaded.Status != TaskDone {
		t.Errorf("expected status done, got %s", loaded.Status)
	}
	if len(loaded.Notes) != 2 {
		t.Fatalf("expected 2 notes after update, got %d", len(loaded.Notes))
	}
}

func TestSaveTasks_BulkOperation(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewTaskRepository(db)

	tasks := []Task{
		{
			ID:        "task-1",
			Text:      "First task",
			Status:    TaskOpen,
			Position:  0,
			CreatedAt: time.Now(),
			Notes:     []TaskNote{{ID: "note-1", Text: "Note 1", Position: 0}},
		},
		{
			ID:        "task-2",
			Text:      "Second task",
			Status:    TaskDone,
			Position:  1,
			CreatedAt: time.Now(),
			Notes:     []TaskNote{{ID: "note-2", Text: "Note 2", Position: 0}},
		},
	}

	err := repo.SaveTasks(tasks)
	if err != nil {
		t.Fatalf("failed to save tasks: %v", err)
	}

	// Verify all tasks were saved
	allTasks, err := repo.GetAllTasks()
	if err != nil {
		t.Fatalf("failed to get all tasks: %v", err)
	}

	if len(allTasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(allTasks))
	}
}

func TestGetAllTasks(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewTaskRepository(db)

	// Save multiple tasks
	task1 := &Task{
		ID:        "task-1",
		Text:      "First task",
		Status:    TaskOpen,
		Position:  0,
		CreatedAt: time.Now(),
		Notes: []TaskNote{
			{ID: "note-1", Text: "Note 1", Position: 0},
		},
	}
	task2 := &Task{
		ID:        "task-2",
		Text:      "Second task",
		Status:    TaskDone,
		Position:  1,
		CreatedAt: time.Now(),
		Notes: []TaskNote{
			{ID: "note-2", Text: "Note 2", Position: 0},
			{ID: "note-3", Text: "Note 3", Position: 1},
		},
	}

	if err := repo.SaveTask(task1); err != nil {
		t.Fatalf("failed to save task1: %v", err)
	}
	if err := repo.SaveTask(task2); err != nil {
		t.Fatalf("failed to save task2: %v", err)
	}

	// Get all tasks
	tasks, err := repo.GetAllTasks()
	if err != nil {
		t.Fatalf("failed to get all tasks: %v", err)
	}

	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}

	// Verify tasks are sorted by position
	if tasks[0].Position != 0 {
		t.Errorf("expected first task position 0, got %d", tasks[0].Position)
	}
	if tasks[1].Position != 1 {
		t.Errorf("expected second task position 1, got %d", tasks[1].Position)
	}

	// Verify notes are loaded
	if len(tasks[0].Notes) != 1 {
		t.Errorf("expected task 1 to have 1 note, got %d", len(tasks[0].Notes))
	}
	if len(tasks[1].Notes) != 2 {
		t.Errorf("expected task 2 to have 2 notes, got %d", len(tasks[1].Notes))
	}

	// Verify notes are sorted by position
	if tasks[1].Notes[0].Position != 0 {
		t.Errorf("expected first note position 0, got %d", tasks[1].Notes[0].Position)
	}
	if tasks[1].Notes[1].Position != 1 {
		t.Errorf("expected second note position 1, got %d", tasks[1].Notes[1].Position)
	}
}

func TestGetTaskByID_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewTaskRepository(db)

	_, err := repo.GetTaskByID("nonexistent")
	if err == nil {
		t.Fatal("expected error when getting nonexistent task")
	}
}

func TestDeleteTask(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewTaskRepository(db)

	// Save a task with notes
	task := &Task{
		ID:        "task-123",
		Text:      "Test task",
		Status:    TaskOpen,
		Position:  0,
		CreatedAt: time.Now(),
		Notes: []TaskNote{
			{ID: "note-1", Text: "Note 1", Position: 0},
			{ID: "note-2", Text: "Note 2", Position: 1},
		},
	}
	err := repo.SaveTask(task)
	if err != nil {
		t.Fatalf("failed to save task: %v", err)
	}

	// Delete the task
	err = repo.DeleteTask("task-123")
	if err != nil {
		t.Fatalf("failed to delete task: %v", err)
	}

	// Verify task is gone
	_, err = repo.GetTaskByID("task-123")
	if err == nil {
		t.Fatal("expected error when getting deleted task")
	}

	// Verify notes are also deleted (CASCADE)
	var noteCount int
	err = db.DB().QueryRow("SELECT COUNT(*) FROM task_notes WHERE task_id = ?", "task-123").Scan(&noteCount)
	if err != nil {
		t.Fatalf("failed to count notes: %v", err)
	}
	if noteCount != 0 {
		t.Errorf("expected 0 notes after task deletion, got %d", noteCount)
	}
}

func TestDeleteTaskNote(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewTaskRepository(db)

	// Save a task with notes
	task := &Task{
		ID:        "task-123",
		Text:      "Test task",
		Status:    TaskOpen,
		Position:  0,
		CreatedAt: time.Now(),
		Notes: []TaskNote{
			{ID: "note-1", Text: "Note 1", Position: 0},
			{ID: "note-2", Text: "Note 2", Position: 1},
		},
	}
	err := repo.SaveTask(task)
	if err != nil {
		t.Fatalf("failed to save task: %v", err)
	}

	// Delete one note
	err = repo.DeleteTaskNote("note-1")
	if err != nil {
		t.Fatalf("failed to delete note: %v", err)
	}

	// Verify only one note remains
	loaded, err := repo.GetTaskByID("task-123")
	if err != nil {
		t.Fatalf("failed to load task: %v", err)
	}

	if len(loaded.Notes) != 1 {
		t.Fatalf("expected 1 note after deletion, got %d", len(loaded.Notes))
	}
	if loaded.Notes[0].ID != "note-2" {
		t.Errorf("expected remaining note to be note-2, got %s", loaded.Notes[0].ID)
	}
}

func TestSaveTask_EmptyNotes(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewTaskRepository(db)

	task := &Task{
		ID:        "task-123",
		Text:      "Test task",
		Status:    TaskOpen,
		Position:  0,
		CreatedAt: time.Now(),
		Notes:     []TaskNote{},
	}

	err := repo.SaveTask(task)
	if err != nil {
		t.Fatalf("failed to save task with empty notes: %v", err)
	}

	loaded, err := repo.GetTaskByID("task-123")
	if err != nil {
		t.Fatalf("failed to load task: %v", err)
	}

	if len(loaded.Notes) != 0 {
		t.Errorf("expected 0 notes, got %d", len(loaded.Notes))
	}
}

func TestGetAllTasks_Empty(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewTaskRepository(db)

	tasks, err := repo.GetAllTasks()
	if err != nil {
		t.Fatalf("failed to get all tasks: %v", err)
	}

	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestSaveTask_NotesSortedByPosition(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewTaskRepository(db)

	task := &Task{
		ID:        "task-123",
		Text:      "Test task",
		Status:    TaskOpen,
		Position:  0,
		CreatedAt: time.Now(),
		Notes: []TaskNote{
			{ID: "note-3", Text: "Third", Position: 2},
			{ID: "note-1", Text: "First", Position: 0},
			{ID: "note-2", Text: "Second", Position: 1},
		},
	}

	err := repo.SaveTask(task)
	if err != nil {
		t.Fatalf("failed to save task: %v", err)
	}

	loaded, err := repo.GetTaskByID("task-123")
	if err != nil {
		t.Fatalf("failed to load task: %v", err)
	}

	// Verify notes are sorted by position
	if loaded.Notes[0].Text != "First" {
		t.Errorf("expected first note text 'First', got %s", loaded.Notes[0].Text)
	}
	if loaded.Notes[1].Text != "Second" {
		t.Errorf("expected second note text 'Second', got %s", loaded.Notes[1].Text)
	}
	if loaded.Notes[2].Text != "Third" {
		t.Errorf("expected third note text 'Third', got %s", loaded.Notes[2].Text)
	}
}
