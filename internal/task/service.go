package task

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/journal"
)

// Service handles task business logic
type Service struct {
	repo       *Repository
	journalSvc *journal.Service
}

// NewService creates a new task service
func NewService(repo *Repository, journalSvc *journal.Service) *Service {
	return &Service{
		repo:       repo,
		journalSvc: journalSvc,
	}
}

// Create creates a new task
func (s *Service) Create(title string, tags []string) (*Task, error) {
	task := NewTask(title, tags)

	// Set file path
	tasksDir, err := config.TasksDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get tasks directory: %w", err)
	}
	task.FilePath = filepath.Join(tasksDir, task.ID+".md")

	// Save to filesystem and database
	if err := s.save(task); err != nil {
		return nil, err
	}

	return task, nil
}

// GetByID retrieves a task by ID
func (s *Service) GetByID(id string) (*Task, error) {
	// Get metadata from database
	task, err := s.repo.GetTaskByID(id)
	if err != nil {
		return nil, err
	}

	// Read full content from file
	content, err := os.ReadFile(task.FilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read task file: %w", err)
	}

	// Parse the file
	fullTask, err := ParseTask(string(content), task.FilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse task file: %w", err)
	}

	return fullTask, nil
}

// List retrieves tasks filtered by status and tags
func (s *Service) List(status *Status, tags []string) ([]Task, error) {
	return s.repo.ListTasks(status, tags)
}

// UpdateStatus updates a task's status
func (s *Service) UpdateStatus(id string, status Status) error {
	task, err := s.GetByID(id)
	if err != nil {
		return err
	}

	task.SetStatus(status)
	return s.save(task)
}

// Update updates a task's editable fields
func (s *Service) Update(id string, title *string, description *string, tags []string) error {
	task, err := s.GetByID(id)
	if err != nil {
		return err
	}

	if title != nil {
		task.Title = *title
	}
	if description != nil {
		task.Description = *description
	}
	if tags != nil {
		task.Tags = tags
	}

	return s.save(task)
}

// FindStaleTasks finds tasks that haven't been updated in the specified number of days
func (s *Service) FindStaleTasks(days int) ([]Task, error) {
	return s.repo.FindStaleTasks(days)
}

// AppendLog adds a log entry to a task
// This is the dual-write method - writes to both task file and today's journal
func (s *Service) AppendLog(taskID string, content string) error {
	// Get the task
	task, err := s.GetByID(taskID)
	if err != nil {
		return err
	}

	// Create log entry
	timestamp := time.Now()
	entry := NewLogEntry(timestamp, content)
	task.AppendLog(*entry)

	// Save task (write to file and database)
	if err := s.save(task); err != nil {
		return fmt.Errorf("failed to save task: %w", err)
	}

	// Also append to today's journal with [task:id] prefix
	today, err := s.journalSvc.GetToday()
	if err != nil {
		return fmt.Errorf("failed to get today's journal: %w", err)
	}

	journalContent := fmt.Sprintf("[task:%s] %s", taskID, content)
	if err := s.journalSvc.AppendLog(today, journalContent); err != nil {
		return fmt.Errorf("failed to append to journal: %w", err)
	}

	return nil
}

// Delete deletes a task
func (s *Service) Delete(id string) error {
	task, err := s.repo.GetTaskByID(id)
	if err != nil {
		return err
	}

	// Delete file
	if err := os.Remove(task.FilePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete task file: %w", err)
	}

	// Delete from database
	return s.repo.DeleteTask(id)
}

// save saves a task to both filesystem and database
func (s *Service) save(task *Task) error {
	// Serialize to markdown
	content := WriteTask(task)

	// Write to filesystem
	if err := os.WriteFile(task.FilePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write task file: %w", err)
	}

	// Save to database
	if err := s.repo.SaveTask(task); err != nil {
		return fmt.Errorf("failed to save task to database: %w", err)
	}

	return nil
}
