package journal

import (
	"fmt"

	"github.com/MikeBiancalana/reckon/internal/storage"
)

// TaskService handles business logic for task management
type TaskService struct {
	repo  *TaskRepository
	store *storage.FileStore
}

// NewTaskService creates a new task service
func NewTaskService(repo *TaskRepository, store *storage.FileStore) *TaskService {
	return &TaskService{
		repo:  repo,
		store: store,
	}
}

// GetAllTasks loads tasks from file (source of truth)
// The file is the authoritative source; DB is just an index/cache
func (s *TaskService) GetAllTasks() ([]Task, error) {
	// Read tasks file
	content, info, err := s.store.ReadTasksFile()
	if err != nil {
		return nil, fmt.Errorf("failed to read tasks file: %w", err)
	}

	// If file doesn't exist, return empty list
	if !info.Exists {
		return []Task{}, nil
	}

	// Parse content
	tasks, err := ParseTasksFile(content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse tasks file: %w", err)
	}

	return tasks, nil
}

// AddTask creates a new task and persists it
func (s *TaskService) AddTask(text string) error {
	// Load all existing tasks
	tasks, err := s.GetAllTasks()
	if err != nil {
		return fmt.Errorf("failed to load tasks: %w", err)
	}

	// Create new task with position at end
	newTask := NewTask(text, len(tasks))
	tasks = append(tasks, *newTask)

	// Save to both file and DB
	if err := s.save(tasks); err != nil {
		return fmt.Errorf("failed to save task: %w", err)
	}

	return nil
}

// ToggleTask toggles a task's status between open and done
func (s *TaskService) ToggleTask(taskID string) error {
	// Load all tasks
	tasks, err := s.GetAllTasks()
	if err != nil {
		return fmt.Errorf("failed to load tasks: %w", err)
	}

	// Find and toggle task
	found := false
	for i := range tasks {
		if tasks[i].ID == taskID {
			if tasks[i].Status == TaskOpen {
				tasks[i].Status = TaskDone
			} else if tasks[i].Status == TaskDone {
				tasks[i].Status = TaskOpen
			}
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("task not found: %s", taskID)
	}

	// Save changes
	if err := s.save(tasks); err != nil {
		return fmt.Errorf("failed to save task: %w", err)
	}

	return nil
}

// AddTaskNote adds a note to a task
func (s *TaskService) AddTaskNote(taskID, noteText string) error {
	// Load all tasks
	tasks, err := s.GetAllTasks()
	if err != nil {
		return fmt.Errorf("failed to load tasks: %w", err)
	}

	// Find task and add note
	found := false
	for i := range tasks {
		if tasks[i].ID == taskID {
			notePosition := len(tasks[i].Notes)
			newNote := NewTaskNote(noteText, notePosition)
			tasks[i].Notes = append(tasks[i].Notes, *newNote)
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("task not found: %s", taskID)
	}

	// Save changes
	if err := s.save(tasks); err != nil {
		return fmt.Errorf("failed to save note: %w", err)
	}

	return nil
}

// DeleteTaskNote removes a note from a task
func (s *TaskService) DeleteTaskNote(taskID, noteID string) error {
	// Load all tasks
	tasks, err := s.GetAllTasks()
	if err != nil {
		return fmt.Errorf("failed to load tasks: %w", err)
	}

	// Find task and remove note
	taskFound := false
	noteFound := false
	for i := range tasks {
		if tasks[i].ID == taskID {
			taskFound = true
			// Find and remove note
			for j, note := range tasks[i].Notes {
				if note.ID == noteID {
					// Remove note from slice
					tasks[i].Notes = append(tasks[i].Notes[:j], tasks[i].Notes[j+1:]...)
					noteFound = true
					// Update positions for remaining notes
					for k := j; k < len(tasks[i].Notes); k++ {
						tasks[i].Notes[k].Position = k
					}
					break
				}
			}
			break
		}
	}

	if !taskFound {
		return fmt.Errorf("task not found: %s", taskID)
	}
	if !noteFound {
		return fmt.Errorf("note not found: %s", noteID)
	}

	// Save changes
	if err := s.save(tasks); err != nil {
		return fmt.Errorf("failed to delete note: %w", err)
	}

	return nil
}

// GetTaskByID retrieves a single task by ID using the repository
func (s *TaskService) GetTaskByID(id string) (*Task, error) {
	return s.repo.GetTaskByID(id)
}

// save persists tasks to both file and database
// File is source of truth, DB is index/cache for querying
func (s *TaskService) save(tasks []Task) error {
	// Serialize to markdown
	content := WriteTasksFile(tasks)

	// Write to file first (source of truth)
	if err := s.store.WriteTasksFile(content); err != nil {
		return fmt.Errorf("failed to write tasks file: %w", err)
	}

	// Write to DB for indexing/querying
	if err := s.repo.SaveTasks(tasks); err != nil {
		return fmt.Errorf("failed to save tasks to database: %w", err)
	}

	return nil
}
