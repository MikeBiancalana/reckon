package journal

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/MikeBiancalana/reckon/internal/logger"
	"github.com/MikeBiancalana/reckon/internal/perf"
	"github.com/MikeBiancalana/reckon/internal/storage"
)

// TaskRepository handles database operations for tasks
type TaskRepository struct {
	db *storage.Database
}

// NewTaskRepository creates a new task repository
func NewTaskRepository(db *storage.Database) *TaskRepository {
	return &TaskRepository{db: db}
}

// SaveTask saves a task and its notes to the database
// Uses a transaction to ensure atomicity
func (r *TaskRepository) SaveTask(task *Task) error {
	logger.Debug("SaveTask", "task_id", task.ID)

	tx, err := r.db.BeginTx()
	if err != nil {
		logger.Error("SaveTask", "error", err, "task_id", task.ID, "operation", "begin_transaction")
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Marshal tags to JSON
	tagsJSON, _ := json.Marshal(task.Tags)

	// Insert or replace task
	_, err = tx.Exec(`
		INSERT OR REPLACE INTO tasks (id, text, status, tags, position, created_at, scheduled_date, deadline_date)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, task.ID, task.Text, task.Status, string(tagsJSON), task.Position, task.CreatedAt.Unix(), task.ScheduledDate, task.DeadlineDate)
	if err != nil {
		logger.Error("SaveTask", "error", err, "task_id", task.ID, "operation", "insert_task")
		return fmt.Errorf("failed to save task: %w", err)
	}

	// Delete existing notes for this task
	_, err = tx.Exec("DELETE FROM task_notes WHERE task_id = ?", task.ID)
	if err != nil {
		logger.Error("SaveTask", "error", err, "task_id", task.ID, "operation", "delete_old_notes")
		return fmt.Errorf("failed to delete old notes: %w", err)
	}

	// Insert new notes
	for _, note := range task.Notes {
		_, err = tx.Exec(`
			INSERT INTO task_notes (id, task_id, text, position)
			VALUES (?, ?, ?, ?)
		`, note.ID, task.ID, note.Text, note.Position)
		if err != nil {
			logger.Error("SaveTask", "error", err, "task_id", task.ID, "note_id", note.ID, "operation", "insert_note")
			return fmt.Errorf("failed to save note: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		logger.Error("SaveTask", "error", err, "task_id", task.ID, "operation", "commit_transaction")
		return err
	}

	return nil
}

// SaveTasks saves multiple tasks in a single transaction
// This is a bulk operation for efficiency
func (r *TaskRepository) SaveTasks(tasks []Task) error {
	logger.Info("SaveTasks", "task_count", len(tasks))

	tx, err := r.db.BeginTx()
	if err != nil {
		logger.Error("SaveTasks", "error", err, "operation", "begin_transaction")
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	for _, task := range tasks {
		// Marshal tags to JSON
		tagsJSON, _ := json.Marshal(task.Tags)

		// Insert or replace task
		_, err = tx.Exec(`
			INSERT OR REPLACE INTO tasks (id, text, status, tags, position, created_at, scheduled_date, deadline_date)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, task.ID, task.Text, task.Status, string(tagsJSON), task.Position, task.CreatedAt.Unix(), task.ScheduledDate, task.DeadlineDate)
		if err != nil {
			logger.Error("SaveTasks", "error", err, "task_id", task.ID)
			return fmt.Errorf("failed to save task %s: %w", task.ID, err)
		}

		// Delete existing notes for this task
		_, err = tx.Exec("DELETE FROM task_notes WHERE task_id = ?", task.ID)
		if err != nil {
			logger.Error("SaveTasks", "error", err, "task_id", task.ID, "operation", "delete_notes")
			return fmt.Errorf("failed to delete old notes for task %s: %w", task.ID, err)
		}

		// Insert new notes
		for _, note := range task.Notes {
			_, err = tx.Exec(`
				INSERT INTO task_notes (id, task_id, text, position)
				VALUES (?, ?, ?, ?)
			`, note.ID, task.ID, note.Text, note.Position)
			if err != nil {
				logger.Error("SaveTasks", "error", err, "task_id", task.ID, "note_id", note.ID)
				return fmt.Errorf("failed to save note %s for task %s: %w", note.ID, task.ID, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		logger.Error("SaveTasks", "error", err, "operation", "commit_transaction")
		return err
	}
	return nil
}

// GetAllTasks retrieves all tasks with their notes
// Tasks and notes are sorted by position
func (r *TaskRepository) GetAllTasks() ([]Task, error) {
	timer := perf.NewTimer("TaskRepository.GetAllTasks", nil, 50)
	defer timer.Stop()

	logger.Debug("GetAllTasks", "operation", "start")

	// Use LEFT JOIN to get tasks, notes, and tags in a single query
	rows, err := r.db.DB().Query(`
		SELECT
			t.id, t.text, t.status, t.position, t.created_at, t.tags, t.scheduled_date, t.deadline_date,
			n.id, n.text, n.position
		FROM tasks t
		LEFT JOIN task_notes n ON t.id = n.task_id
		ORDER BY t.position, n.position
	`)
	if err != nil {
		logger.Error("GetAllTasks", "error", err, "operation", "query_tasks")
		return nil, fmt.Errorf("failed to query tasks: %w", err)
	}
	defer rows.Close()

	tasksMap := make(map[string]*Task)
	taskOrder := make([]string, 0)

	for rows.Next() {
		var taskID, taskText, tagsJSON string
		var taskStatus TaskStatus
		var taskPosition int
		var taskCreatedAtUnix int64
		var scheduledDate, deadlineDate sql.NullString
		var noteID, noteText sql.NullString
		var notePosition sql.NullInt64

		err := rows.Scan(
			&taskID, &taskText, &taskStatus, &taskPosition, &taskCreatedAtUnix, &tagsJSON, &scheduledDate, &deadlineDate,
			&noteID, &noteText, &notePosition,
		)
		if err != nil {
			logger.Error("GetAllTasks", "error", err, "operation", "scan_row", "task_id", taskID)
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Get or create task
		task, exists := tasksMap[taskID]
		if !exists {
			var tags []string
			if tagsJSON != "" {
				json.Unmarshal([]byte(tagsJSON), &tags)
			}
			task = &Task{
				ID:            taskID,
				Text:          taskText,
				Status:        taskStatus,
				Position:      taskPosition,
				CreatedAt:     unixToTime(taskCreatedAtUnix),
				Tags:          tags,
				Notes:         make([]TaskNote, 0),
				ScheduledDate: nil,
				DeadlineDate:  nil,
			}
			if scheduledDate.Valid {
				task.ScheduledDate = &scheduledDate.String
			}
			if deadlineDate.Valid {
				task.DeadlineDate = &deadlineDate.String
			}
			tasksMap[taskID] = task
			taskOrder = append(taskOrder, taskID)
		}

		// Add note if it exists
		if noteID.Valid {
			note := TaskNote{
				ID:       noteID.String,
				Text:     noteText.String,
				Position: int(notePosition.Int64),
			}
			task.Notes = append(task.Notes, note)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating tasks: %w", err)
	}

	// Convert map to slice in the correct order
	tasks := make([]Task, 0, len(taskOrder))
	for _, id := range taskOrder {
		tasks = append(tasks, *tasksMap[id])
	}

	logger.Debug("GetAllTasks", "operation", "complete", "total_tasks", len(tasks))
	return tasks, nil
}

// GetTaskByID retrieves a single task by ID with its notes
func (r *TaskRepository) GetTaskByID(id string) (*Task, error) {
	var task Task
	var createdAtUnix int64
	var scheduledDate, deadlineDate sql.NullString

	err := r.db.DB().QueryRow(`
		SELECT id, text, status, position, created_at, scheduled_date, deadline_date
		FROM tasks
		WHERE id = ?
	`, id).Scan(&task.ID, &task.Text, &task.Status, &task.Position, &createdAtUnix, &scheduledDate, &deadlineDate)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("task not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	task.CreatedAt = unixToTime(createdAtUnix)
	if scheduledDate.Valid {
		task.ScheduledDate = &scheduledDate.String
	}
	if deadlineDate.Valid {
		task.DeadlineDate = &deadlineDate.String
	}

	// Load notes for this task
	notes, err := r.loadTaskNotes(id)
	if err != nil {
		return nil, err
	}
	task.Notes = notes

	return &task, nil
}

// DeleteTask deletes a task and its notes from the database
// Notes are automatically deleted by CASCADE
func (r *TaskRepository) DeleteTask(id string) error {
	logger.Info("DeleteTask", "task_id", id)

	_, err := r.db.DB().Exec("DELETE FROM tasks WHERE id = ?", id)
	if err != nil {
		logger.Error("DeleteTask", "error", err, "task_id", id)
		return fmt.Errorf("failed to delete task: %w", err)
	}
	return nil
}

// DeleteTaskNote deletes a specific note from the database
func (r *TaskRepository) DeleteTaskNote(noteID string) error {
	_, err := r.db.DB().Exec("DELETE FROM task_notes WHERE id = ?", noteID)
	if err != nil {
		return fmt.Errorf("failed to delete note: %w", err)
	}
	return nil
}

// loadTaskNotes is a helper function to load notes for a task
// Notes are sorted by position
func (r *TaskRepository) loadTaskNotes(taskID string) ([]TaskNote, error) {
	rows, err := r.db.DB().Query(`
		SELECT id, text, position
		FROM task_notes
		WHERE task_id = ?
		ORDER BY position
	`, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to query notes: %w", err)
	}
	defer rows.Close()

	notes := make([]TaskNote, 0)
	for rows.Next() {
		var note TaskNote
		err := rows.Scan(&note.ID, &note.Text, &note.Position)
		if err != nil {
			return nil, fmt.Errorf("failed to scan note: %w", err)
		}
		notes = append(notes, note)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating notes: %w", err)
	}

	return notes, nil
}

// unixToTime converts a Unix timestamp to time.Time
func unixToTime(unix int64) time.Time {
	return time.Unix(unix, 0)
}

// FindStaleTasks finds tasks that haven't had any log entry activity in the specified number of days
// Uses a single query with LEFT JOIN to avoid N+1 queries
// Excludes done tasks
func (r *TaskRepository) FindStaleTasks(days int) ([]Task, error) {
	timer := perf.NewTimer("TaskRepository.FindStaleTasks", nil, 50)
	defer timer.Stop()

	logger.Debug("FindStaleTasks", "operation", "start", "days", days)

	rows, err := r.db.DB().Query(`
		SELECT
			t.id, t.text, t.status, t.position, t.created_at, t.tags, t.scheduled_date, t.deadline_date,
			COALESCE(MAX(le.timestamp), '') as last_activity
		FROM tasks t
		LEFT JOIN log_entries le ON t.id = le.task_id
		WHERE t.status = 'open'
		GROUP BY t.id
		HAVING MAX(le.timestamp) IS NULL OR MAX(le.timestamp) < datetime('now', '-' || ? || ' days')
		ORDER BY t.position
	`, days)
	if err != nil {
		logger.Error("FindStaleTasks", "error", err, "operation", "query")
		return nil, fmt.Errorf("failed to query stale tasks: %w", err)
	}
	defer rows.Close()

	tasksMap := make(map[string]*Task)
	taskOrder := make([]string, 0)

	for rows.Next() {
		var taskID, taskText, tagsJSON string
		var taskStatus TaskStatus
		var taskPosition int
		var taskCreatedAtUnix int64
		var scheduledDate, deadlineDate sql.NullString

		err := rows.Scan(
			&taskID, &taskText, &taskStatus, &taskPosition, &taskCreatedAtUnix, &tagsJSON, &scheduledDate, &deadlineDate,
		)
		if err != nil {
			logger.Error("FindStaleTasks", "error", err, "operation", "scan", "task_id", taskID)
			return nil, fmt.Errorf("failed to scan task: %w", err)
		}

		if _, exists := tasksMap[taskID]; exists {
			continue
		}

		var tags []string
		if tagsJSON != "" {
			json.Unmarshal([]byte(tagsJSON), &tags)
		}

		task := &Task{
			ID:            taskID,
			Text:          taskText,
			Status:        taskStatus,
			Position:      taskPosition,
			CreatedAt:     unixToTime(taskCreatedAtUnix),
			Tags:          tags,
			Notes:         make([]TaskNote, 0),
			ScheduledDate: nil,
			DeadlineDate:  nil,
		}
		if scheduledDate.Valid {
			task.ScheduledDate = &scheduledDate.String
		}
		if deadlineDate.Valid {
			task.DeadlineDate = &deadlineDate.String
		}
		tasksMap[taskID] = task
		taskOrder = append(taskOrder, taskID)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating stale tasks: %w", err)
	}

	tasks := make([]Task, 0, len(taskOrder))
	for _, id := range taskOrder {
		tasks = append(tasks, *tasksMap[id])
	}

	logger.Debug("FindStaleTasks", "operation", "complete", "stale_count", len(tasks))
	return tasks, nil
}
