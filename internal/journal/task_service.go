package journal

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/perf"
	"github.com/MikeBiancalana/reckon/internal/storage"
	"github.com/rs/xid"
	"gopkg.in/yaml.v3"
)

// TaskFrontmatter represents the YAML frontmatter in task files
type TaskFrontmatter struct {
	ID            string   `yaml:"id"`
	Title         string   `yaml:"title"`
	Created       string   `yaml:"created"`
	Status        string   `yaml:"status"`
	Tags          []string `yaml:"tags,omitempty"`
	ScheduledDate *string  `yaml:"scheduled_date,omitempty"`
	DeadlineDate  *string  `yaml:"deadline_date,omitempty"`
}

// TaskService handles business logic for task management
type TaskService struct {
	repo   *TaskRepository
	store  *storage.FileStore
	logger *slog.Logger
}

// NewTaskService creates a new task service
func NewTaskService(repo *TaskRepository, store *storage.FileStore, logger *slog.Logger) *TaskService {
	return &TaskService{
		repo:   repo,
		store:  store,
		logger: DefaultLogger(logger),
	}
}

// slugRegex matches non-alphanumeric characters for removal in slugs
var slugRegex = regexp.MustCompile(`[^a-z0-9]+`)

func generateSlug(text string) string {
	slug := strings.ToLower(text)
	slug = slugRegex.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if len(slug) > 50 {
		slug = slug[:50]
		slug = strings.Trim(slug, "-")
	}
	if slug == "" {
		slug = "untitled"
	}
	return slug
}

func taskFilename(task Task) string {
	datePrefix := task.CreatedAt.Format("2006-01-02")
	slug := generateSlug(task.Text)
	return fmt.Sprintf("%s-%s.md", datePrefix, slug)
}

func taskFilePath(task Task) (string, error) {
	tasksDir, err := config.TasksDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(tasksDir, taskFilename(task)), nil
}

// MigrateTaskFilenames renames existing task files from {ID}.md to YYYY-MM-DD-slug.md format
func (s *TaskService) MigrateTaskFilenames() error {
	s.logger.Info("MigrateTaskFilenames", "operation", "start")

	tasksDir, err := config.TasksDir()
	if err != nil {
		s.logger.Error("MigrateTaskFilenames", "error", err, "operation", "get_tasks_dir")
		return fmt.Errorf("failed to get tasks directory: %w", err)
	}

	files, err := os.ReadDir(tasksDir)
	if err != nil {
		s.logger.Error("MigrateTaskFilenames", "error", err, "operation", "read_tasks_dir")
		return fmt.Errorf("failed to read tasks directory: %w", err)
	}

	migrated := 0
	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".md") || file.IsDir() {
			continue
		}

		// Skip files already in new format (YYYY-MM-DD-slug.md)
		if len(file.Name()) > 11 && file.Name()[4] == '-' && file.Name()[7] == '-' {
			continue
		}

		// Parse the file to get task info
		filePath := filepath.Join(tasksDir, file.Name())
		content, err := os.ReadFile(filePath)
		if err != nil {
			s.logger.Debug("MigrateTaskFilenames", "error", err, "file_path", filePath)
			continue
		}

		frontmatter, _, err := parseTaskFile(string(content))
		if err != nil {
			s.logger.Debug("MigrateTaskFilenames", "error", err, "file_path", filePath)
			continue
		}

		// Create a temporary task to generate the filename
		createdAt := time.Now()
		if frontmatter.Created != "" {
			if parsed, err := time.Parse("2006-01-02", frontmatter.Created); err == nil {
				createdAt = parsed
			}
		}

		tempTask := Task{
			ID:        frontmatter.ID,
			Text:      frontmatter.Title,
			CreatedAt: createdAt,
		}

		newFileName := taskFilename(tempTask)
		newFilePath := filepath.Join(tasksDir, newFileName)

		if file.Name() != newFileName {
			if err := os.Rename(filePath, newFilePath); err != nil {
				s.logger.Error("MigrateTaskFilenames", "error", err, "old_path", filePath, "new_path", newFilePath)
				continue
			}
			migrated++
			s.logger.Info("MigrateTaskFilenames", "migrated", file.Name(), "to", newFileName)
		}
	}

	s.logger.Info("MigrateTaskFilenames", "operation", "complete", "files_migrated", migrated)
	return nil
}

// GetAllTasks loads tasks from individual task files (source of truth)
// The files are the authoritative source; DB is just an index/cache
func (s *TaskService) GetAllTasks() ([]Task, error) {
	timer := perf.NewTimer("TaskService.GetAllTasks", s.logger, 100)
	defer timer.Stop()

	s.logger.Info("GetAllTasks", "operation", "start")

	// Get tasks directory
	tasksDir, err := config.TasksDir()
	if err != nil {
		s.logger.Error("GetAllTasks", "error", err, "operation", "get_tasks_dir")
		return nil, fmt.Errorf("failed to get tasks directory: %w", err)
	}

	// Read all .md files from tasks directory
	files, err := os.ReadDir(tasksDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Task{}, nil
		}
		s.logger.Error("GetAllTasks", "error", err, "operation", "read_tasks_dir")
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
			s.logger.Debug("GetAllTasks", "error", err, "file_path", filePath)
			continue // Skip files that can't be read
		}

		// Parse file
		frontmatter, notes, err := parseTaskFile(string(content))
		if err != nil {
			s.logger.Debug("GetAllTasks", "error", err, "file_path", filePath)
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
			ID:            frontmatter.ID,
			Text:          frontmatter.Title,
			Status:        status,
			Tags:          frontmatter.Tags,
			Notes:         notes,
			Position:      position,
			CreatedAt:     createdAt,
			ScheduledDate: frontmatter.ScheduledDate,
			DeadlineDate:  frontmatter.DeadlineDate,
		}

		tasks = append(tasks, task)
		position++
	}

	s.logger.Info("GetAllTasks", "operation", "complete", "total_tasks", len(tasks))
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

// validateTags validates and sanitizes tag input
func validateTags(tags []string) []string {
	validTags := make([]string, 0, len(tags))
	seen := make(map[string]bool)

	for _, tag := range tags {
		tag = strings.TrimSpace(strings.ToLower(tag))
		if tag != "" && !seen[tag] && len(tag) <= 50 { // reasonable length limit
			validTags = append(validTags, tag)
			seen[tag] = true
		}
	}
	return validTags
}

// AddTask creates a new task and persists it
func (s *TaskService) AddTask(text string, tags []string) error {
	s.logger.Debug("AddTask", "task_text", text, "tags", tags)

	// Validate and sanitize tags
	tags = validateTags(tags)

	// Load all existing tasks
	tasks, err := s.GetAllTasks()
	if err != nil {
		s.logger.Error("AddTask", "error", err, "operation", "load_tasks")
		return fmt.Errorf("failed to load tasks: %w", err)
	}

	// Create new task with position at end
	newTask := NewTask(text, tags, len(tasks))
	tasks = append(tasks, *newTask)

	// Save to both file and DB
	if err := s.save(tasks); err != nil {
		s.logger.Error("AddTask", "error", err, "task_id", newTask.ID)
		return fmt.Errorf("failed to save task: %w", err)
	}

	return nil
}

// ToggleTask toggles a task's status between open and done
func (s *TaskService) ToggleTask(taskID string) error {
	s.logger.Debug("ToggleTask", "task_id", taskID)

	// Load all tasks
	tasks, err := s.GetAllTasks()
	if err != nil {
		s.logger.Error("ToggleTask", "error", err, "operation", "load_tasks")
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
		err := fmt.Errorf("task not found: %s", taskID)
		s.logger.Error("ToggleTask", "error", err, "task_id", taskID)
		return err
	}

	// Save changes
	if err := s.save(tasks); err != nil {
		s.logger.Error("ToggleTask", "error", err, "task_id", taskID)
		return fmt.Errorf("failed to save task: %w", err)
	}

	return nil
}

// AddTaskNote adds a note to a task
func (s *TaskService) AddTaskNote(taskID, noteText string) error {
	s.logger.Debug("AddTaskNote", "task_id", taskID)

	// Load all tasks
	tasks, err := s.GetAllTasks()
	if err != nil {
		s.logger.Error("AddTaskNote", "error", err, "operation", "load_tasks")
		return fmt.Errorf("failed to load tasks: %w", err)
	}

	// Find the task and add the note
	found := false
	var noteID string
	for i := range tasks {
		if tasks[i].ID == taskID {
			notePosition := len(tasks[i].Notes)
			newNote := NewTaskNote(noteText, notePosition)
			tasks[i].Notes = append(tasks[i].Notes, *newNote)
			noteID = newNote.ID
			found = true
			break
		}
	}

	if !found {
		err := fmt.Errorf("task not found: %s", taskID)
		s.logger.Error("AddTaskNote", "error", err, "task_id", taskID)
		return err
	}

	// Save changes
	if err := s.save(tasks); err != nil {
		s.logger.Error("AddTaskNote", "error", err, "task_id", taskID, "note_id", noteID)
		return fmt.Errorf("failed to save note: %w", err)
	}

	return nil
}

// UpdateTask updates a task's title and/or tags
func (s *TaskService) UpdateTask(taskID string, title string, tags []string) error {
	s.logger.Debug("UpdateTask", "task_id", taskID, "title", title, "tags", tags)

	// Validate and sanitize tags
	tags = validateTags(tags)

	// Load all tasks
	tasks, err := s.GetAllTasks()
	if err != nil {
		s.logger.Error("UpdateTask", "error", err, "operation", "load_tasks")
		return fmt.Errorf("failed to load tasks: %w", err)
	}

	// Find and update the task
	found := false
	for i := range tasks {
		if tasks[i].ID == taskID {
			if title != "" {
				tasks[i].Text = title
			}
			tasks[i].Tags = tags
			found = true
			break
		}
	}

	if !found {
		err := fmt.Errorf("task not found: %s", taskID)
		s.logger.Error("UpdateTask", "error", err, "task_id", taskID)
		return err
	}

	// Save changes
	if err := s.save(tasks); err != nil {
		s.logger.Error("UpdateTask", "error", err, "task_id", taskID)
		return fmt.Errorf("failed to update task: %w", err)
	}

	return nil
}

// DeleteTaskNote removes a note from a task
func (s *TaskService) DeleteTaskNote(taskID, noteID string) error {
	s.logger.Debug("DeleteTaskNote", "task_id", taskID, "note_id", noteID)

	// Load all tasks
	tasks, err := s.GetAllTasks()
	if err != nil {
		s.logger.Error("DeleteTaskNote", "error", err, "operation", "load_tasks")
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
		err := fmt.Errorf("task not found: %s", taskID)
		s.logger.Error("DeleteTaskNote", "error", err, "task_id", taskID, "note_id", noteID)
		return err
	}
	if !noteFound {
		err := fmt.Errorf("note not found: %s", noteID)
		s.logger.Error("DeleteTaskNote", "error", err, "task_id", taskID, "note_id", noteID)
		return err
	}

	// Save changes
	if err := s.save(tasks); err != nil {
		s.logger.Error("DeleteTaskNote", "error", err, "task_id", taskID, "note_id", noteID)
		return fmt.Errorf("failed to delete note: %w", err)
	}

	return nil
}

// DeleteTask deletes a task and its associated file
func (s *TaskService) DeleteTask(taskID string) error {
	s.logger.Debug("DeleteTask", "task_id", taskID)

	// Load all tasks
	tasks, err := s.GetAllTasks()
	if err != nil {
		s.logger.Error("DeleteTask", "error", err, "operation", "load_tasks")
		return fmt.Errorf("failed to load tasks: %w", err)
	}

	// Find and remove the task
	found := false
	var deletedTask Task
	for i, t := range tasks {
		if t.ID == taskID {
			deletedTask = t
			// Remove from slice
			tasks = append(tasks[:i], tasks[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		err := fmt.Errorf("task not found: %s", taskID)
		s.logger.Error("DeleteTask", "error", err, "task_id", taskID)
		return err
	}

	// Delete the task file
	filePath, err := taskFilePath(deletedTask)
	if err != nil {
		s.logger.Error("DeleteTask", "error", err, "task_id", taskID)
		return fmt.Errorf("failed to get task file path: %w", err)
	}
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		s.logger.Error("DeleteTask", "error", err, "task_id", taskID, "file_path", filePath)
		return fmt.Errorf("failed to delete task file: %w", err)
	}

	// Save changes (updates DB and doesn't write the deleted task)
	if err := s.save(tasks); err != nil {
		s.logger.Error("DeleteTask", "error", err, "task_id", taskID)
		return fmt.Errorf("failed to save tasks after deletion: %w", err)
	}

	return nil
}

// save persists tasks to both individual files and database
// Files are source of truth, DB is index/cache for querying
func (s *TaskService) save(tasks []Task) error {
	s.logger.Info("save", "operation", "start", "task_count", len(tasks))

	// Get tasks directory
	tasksDir, err := config.TasksDir()
	if err != nil {
		s.logger.Error("save", "error", err, "operation", "get_tasks_dir")
		return fmt.Errorf("failed to get tasks directory: %w", err)
	}

	// Read existing files to detect renames
	existingFiles, err := os.ReadDir(tasksDir)
	if err != nil && !os.IsNotExist(err) {
		s.logger.Error("save", "error", err, "operation", "read_tasks_dir")
		return fmt.Errorf("failed to read tasks directory: %w", err)
	}

	// Build map of old filenames to detect renames
	oldFiles := make(map[string]string)
	for _, file := range existingFiles {
		if strings.HasSuffix(file.Name(), ".md") && !file.IsDir() {
			oldFiles[file.Name()] = filepath.Join(tasksDir, file.Name())
		}
	}

	// Write each task to individual file
	for _, task := range tasks {
		content := writeTaskFile(task)
		newFilePath, err := taskFilePath(task)
		if err != nil {
			s.logger.Error("save", "error", err, "task_id", task.ID)
			return fmt.Errorf("failed to get task file path: %w", err)
		}

		// Check if file needs to be renamed
		oldFileName := task.ID + ".md"
		if oldPath, exists := oldFiles[oldFileName]; exists {
			delete(oldFiles, oldFileName)
			if oldFileName != taskFilename(task) {
				if err := os.Rename(oldPath, newFilePath); err != nil {
					s.logger.Error("save", "error", err, "old_path", oldPath, "new_path", newFilePath)
					return fmt.Errorf("failed to rename task file: %w", err)
				}
			}
		}

		if err := os.WriteFile(newFilePath, []byte(content), 0644); err != nil {
			s.logger.Error("save", "error", err, "task_id", task.ID, "file_path", newFilePath)
			return fmt.Errorf("failed to write task file %s: %w", newFilePath, err)
		}
	}

	// Write to DB for indexing/querying
	if err := s.repo.SaveTasks(tasks); err != nil {
		s.logger.Error("save", "error", err, "operation", "save_to_db")
		return fmt.Errorf("failed to save tasks to database: %w", err)
	}

	s.logger.Info("save", "operation", "complete", "task_count", len(tasks))
	return nil
}

// writeTaskFile serializes a task to markdown format with frontmatter
func writeTaskFile(task Task) string {
	status := "open"
	if task.Status == TaskDone {
		status = "done"
	}

	frontmatter := TaskFrontmatter{
		ID:            task.ID,
		Title:         task.Text,
		Created:       task.CreatedAt.Format("2006-01-02"),
		Status:        status,
		Tags:          task.Tags,
		ScheduledDate: task.ScheduledDate,
		DeadlineDate:  task.DeadlineDate,
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

// validateDate validates a date string in YYYY-MM-DD format
func validateDate(date string) error {
	if date == "" {
		return nil // Empty is allowed for clearing
	}
	_, err := time.Parse("2006-01-02", date)
	if err != nil {
		return fmt.Errorf("invalid date format %q: expected YYYY-MM-DD", date)
	}
	return nil
}

// ScheduleTask sets the scheduled date for a task
// The date should be in YYYY-MM-DD format. Use empty string to clear the schedule.
func (s *TaskService) ScheduleTask(taskID string, date string) error {
	s.logger.Debug("ScheduleTask", "task_id", taskID, "date", date)

	if err := validateDate(date); err != nil {
		return err
	}

	return s.updateTaskDateField(taskID, "scheduled_date", date)
}

// SetTaskDeadline sets the deadline date for a task
// The date should be in YYYY-MM-DD format. Use empty string to clear the deadline.
func (s *TaskService) SetTaskDeadline(taskID string, date string) error {
	s.logger.Debug("SetTaskDeadline", "task_id", taskID, "date", date)

	if err := validateDate(date); err != nil {
		return err
	}

	return s.updateTaskDateField(taskID, "deadline_date", date)
}

// ClearTaskSchedule clears the scheduled date for a task, making it unscheduled.
func (s *TaskService) ClearTaskSchedule(taskID string) error {
	s.logger.Debug("ClearTaskSchedule", "task_id", taskID)
	return s.updateTaskDateField(taskID, "scheduled_date", "")
}

// ClearTaskDeadline clears the deadline date for a task, removing the deadline.
func (s *TaskService) ClearTaskDeadline(taskID string) error {
	s.logger.Debug("ClearTaskDeadline", "task_id", taskID)
	return s.updateTaskDateField(taskID, "deadline_date", "")
}

// updateTaskDateField is a helper to update a date field on a task
func (s *TaskService) updateTaskDateField(taskID string, field string, value string) error {
	// Load all tasks
	tasks, err := s.GetAllTasks()
	if err != nil {
		s.logger.Error("updateTaskDateField", "error", err, "task_id", taskID, "operation", "load_tasks")
		return fmt.Errorf("failed to load tasks: %w", err)
	}

	// Find and update the task
	found := false
	for i := range tasks {
		if tasks[i].ID == taskID {
			if field == "scheduled_date" {
				if value == "" {
					tasks[i].ScheduledDate = nil
				} else {
					tasks[i].ScheduledDate = &value
				}
			} else if field == "deadline_date" {
				if value == "" {
					tasks[i].DeadlineDate = nil
				} else {
					tasks[i].DeadlineDate = &value
				}
			}
			found = true
			break
		}
	}

	if !found {
		err := fmt.Errorf("task not found: %s", taskID)
		s.logger.Error("updateTaskDateField", "error", err, "task_id", taskID)
		return err
	}

	// Save changes
	if err := s.save(tasks); err != nil {
		s.logger.Error("updateTaskDateField", "error", err, "task_id", taskID, "field", field)
		return fmt.Errorf("failed to save task: %w", err)
	}

	return nil
}

// GetTasksByTimeframe returns tasks grouped by timeframe: today, this week, and rest
// - today: tasks scheduled for today
// - this week: tasks scheduled for tomorrow through Sunday of current week
// - rest: unscheduled tasks, past-dated tasks, and tasks scheduled beyond this week
func (s *TaskService) GetTasksByTimeframe() (today, thisWeek, rest []Task, err error) {
	s.logger.Debug("GetTasksByTimeframe", "operation", "start")

	tasks, err := s.GetAllTasks()
	if err != nil {
		s.logger.Error("GetTasksByTimeframe", "error", err, "operation", "load_tasks")
		return nil, nil, nil, fmt.Errorf("failed to load tasks: %w", err)
	}

	todayDate := time.Now().Format("2006-01-02")
	endOfWeek := getEndOfWeek()

	for _, task := range tasks {
		if task.ScheduledDate == nil {
			rest = append(rest, task)
			continue
		}

		scheduledDate := *task.ScheduledDate
		if scheduledDate == todayDate {
			today = append(today, task)
		} else if scheduledDate > todayDate && scheduledDate <= endOfWeek {
			thisWeek = append(thisWeek, task)
		} else {
			rest = append(rest, task)
		}
	}

	s.logger.Debug("GetTasksByTimeframe", "operation", "complete", "today", len(today), "this_week", len(thisWeek), "rest", len(rest))
	return today, thisWeek, rest, nil
}

// getEndOfWeek returns the date string for the end of the current week (Sunday)
func getEndOfWeek() string {
	now := time.Now()
	weekday := now.Weekday()
	daysToSunday := int(time.Sunday - weekday)
	if daysToSunday <= 0 {
		daysToSunday += 7
	}
	return now.AddDate(0, 0, daysToSunday).Format("2006-01-02")
}
