package journal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/storage"
	"github.com/rs/xid"
	"gopkg.in/yaml.v3"
)

// TaskFrontmatter represents the YAML frontmatter in task files
type TaskFrontmatter struct {
	ID      string   `yaml:"id"`
	Title   string   `yaml:"title"`
	Created string   `yaml:"created"`
	Status  string   `yaml:"status"`
	Tags    []string `yaml:"tags,omitempty"`
}

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

// GetAllTasks loads tasks from individual task files (source of truth)
// The files are the authoritative source; DB is just an index/cache
func (s *TaskService) GetAllTasks() ([]Task, error) {
	// Get tasks directory
	tasksDir, err := config.TasksDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get tasks directory: %w", err)
	}

	// Read all .md files from tasks directory
	files, err := os.ReadDir(tasksDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Task{}, nil
		}
		return nil, fmt.Errorf("failed to read tasks directory: %w", err)
	}

	var tasks []Task
	position := 0

	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".md") || file.IsDir() {
			continue
		}

		filePath := filepath.Join(tasksDir, file.Name())
		content, err := os.ReadFile(filePath)
		if err != nil {
			continue // Skip files that can't be read
		}

		// Parse file
		frontmatter, notes, err := parseTaskFile(string(content))
		if err != nil {
			continue // Skip invalid files
		}

		// Create task
		status := TaskOpen
		if frontmatter.Status == "done" {
			status = TaskDone
		}

		createdAt := time.Now()
		if frontmatter.Created != "" {
			if parsed, err := time.Parse("2006-01-02", frontmatter.Created); err == nil {
				createdAt = parsed
			}
		}

		task := Task{
			ID:        frontmatter.ID,
			Text:      frontmatter.Title,
			Status:    status,
			Tags:      frontmatter.Tags,
			Notes:     notes,
			Position:  position,
			CreatedAt: createdAt,
		}

		tasks = append(tasks, task)
		position++
	}

	return tasks, nil
}

// parseTaskFile extracts and parses the YAML frontmatter and notes from task content
func parseTaskFile(content string) (*TaskFrontmatter, []TaskNote, error) {
	parts := strings.Split(content, "---")
	if len(parts) < 3 {
		return nil, nil, fmt.Errorf("invalid frontmatter format")
	}

	var fm TaskFrontmatter
	if err := yaml.Unmarshal([]byte(parts[1]), &fm); err != nil {
		return nil, nil, err
	}

	body := parts[2]
	notes := parseNotesFromBody(body)

	return &fm, notes, nil
}

// parseNotesFromBody parses notes from the task file body
func parseNotesFromBody(body string) []TaskNote {
	lines := strings.Split(body, "\n")
	var notes []TaskNote
	inLog := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "## Log" {
			inLog = true
			continue
		}
		if inLog && strings.HasPrefix(trimmed, "### ") {
			// date line, skip
			continue
		}
		if inLog && strings.HasPrefix(line, "  - ") {
			noteText := strings.TrimPrefix(line, "  - ")
			id, text := extractID(noteText)
			if id == "" {
				id = xid.New().String()
			}
			note := TaskNote{
				ID:       id,
				Text:     text,
				Position: len(notes),
			}
			notes = append(notes, note)
		}
	}

	return notes
}

// AddTask creates a new task and persists it
func (s *TaskService) AddTask(text string, tags []string) error {
	// Load all existing tasks
	tasks, err := s.GetAllTasks()
	if err != nil {
		return fmt.Errorf("failed to load tasks: %w", err)
	}

	// Create new task with position at end
	newTask := NewTask(text, tags, len(tasks))
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

	// Find and toggle the task
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

	// Find the task and add the note
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

// UpdateTask updates a task's title and/or tags
func (s *TaskService) UpdateTask(taskID string, title string, tags []string) error {
	// Load all tasks
	tasks, err := s.GetAllTasks()
	if err != nil {
		return fmt.Errorf("failed to load tasks: %w", err)
	}

	// Find and update the task
	found := false
	for i := range tasks {
		if tasks[i].ID == taskID {
			if title != "" {
				tasks[i].Text = title
			}
			// TODO: Add support for tags in Task struct when implemented
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("task not found: %s", taskID)
	}

	// Save changes
	if err := s.save(tasks); err != nil {
		return fmt.Errorf("failed to update task: %w", err)
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

	// Find the task and remove the note
	taskFound := false
	noteFound := false
	for i := range tasks {
		if tasks[i].ID == taskID {
			taskFound = true
			// Find and remove the note
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

// save persists tasks to both individual files and database
// Files are source of truth, DB is index/cache for querying
func (s *TaskService) save(tasks []Task) error {
	// Get tasks directory
	tasksDir, err := config.TasksDir()
	if err != nil {
		return fmt.Errorf("failed to get tasks directory: %w", err)
	}

	// Write each task to individual file
	for _, task := range tasks {
		content := writeTaskFile(task)
		filePath := filepath.Join(tasksDir, task.ID+".md")
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write task file %s: %w", filePath, err)
		}
	}

	// Write to DB for indexing/querying
	if err := s.repo.SaveTasks(tasks); err != nil {
		return fmt.Errorf("failed to save tasks to database: %w", err)
	}

	return nil
}

// writeTaskFile serializes a task to markdown format with frontmatter
func writeTaskFile(task Task) string {
	status := "open"
	if task.Status == TaskDone {
		status = "done"
	}

	frontmatter := TaskFrontmatter{
		ID:      task.ID,
		Title:   task.Text,
		Created: task.CreatedAt.Format("2006-01-02"),
		Status:  status,
		Tags:    task.Tags,
	}

	yamlData, _ := yaml.Marshal(frontmatter)

	logSection := "## Log\n\n"
	if len(task.Notes) > 0 {
		logSection += fmt.Sprintf("### %s\n", task.CreatedAt.Format("2006-01-02"))
		for _, note := range task.Notes {
			logSection += fmt.Sprintf("  - %s %s\n", note.ID, note.Text)
		}
	}

	return fmt.Sprintf("---\n%s---\n\n## Description\n\n%s", string(yamlData), logSection)
}
