package task

import (
	"database/sql"
	"fmt"

	"github.com/MikeBiancalana/reckon/internal/storage"
)

// Repository handles task database operations
type Repository struct {
	db *storage.Database
}

// NewRepository creates a new task repository
func NewRepository(db *storage.Database) *Repository {
	return &Repository{db: db}
}

// SaveTask saves or updates a task in the database
func (r *Repository) SaveTask(task *Task) error {
	tx, err := r.db.BeginTx()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert or replace task
	_, err = tx.Exec(`
		INSERT OR REPLACE INTO phase2_tasks (id, title, status, created, file_path)
		VALUES (?, ?, ?, ?, ?)
	`, task.ID, task.Title, task.Status, task.Created, task.FilePath)
	if err != nil {
		return fmt.Errorf("failed to save task: %w", err)
	}

	// Delete existing tags
	_, err = tx.Exec("DELETE FROM phase2_task_tags WHERE task_id = ?", task.ID)
	if err != nil {
		return fmt.Errorf("failed to delete old tags: %w", err)
	}

	// Insert tags
	for _, tag := range task.Tags {
		_, err = tx.Exec(`
			INSERT INTO phase2_task_tags (task_id, tag)
			VALUES (?, ?)
		`, task.ID, tag)
		if err != nil {
			return fmt.Errorf("failed to save tag: %w", err)
		}
	}

	// Delete existing log entries
	_, err = tx.Exec("DELETE FROM phase2_task_log_entries WHERE task_id = ?", task.ID)
	if err != nil {
		return fmt.Errorf("failed to delete old log entries: %w", err)
	}

	// Insert log entries
	for _, entry := range task.LogEntries {
		_, err = tx.Exec(`
			INSERT INTO phase2_task_log_entries (id, task_id, date, timestamp, content)
			VALUES (?, ?, ?, ?, ?)
		`, entry.ID, task.ID, entry.Date, entry.Timestamp.Format("2006-01-02 15:04:05"), entry.Content)
		if err != nil {
			return fmt.Errorf("failed to save log entry: %w", err)
		}
	}

	return tx.Commit()
}

// GetTaskByID retrieves a task by ID
func (r *Repository) GetTaskByID(id string) (*Task, error) {
	var task Task
	err := r.db.DB().QueryRow(`
		SELECT id, title, status, created, file_path
		FROM phase2_tasks
		WHERE id = ?
	`, id).Scan(&task.ID, &task.Title, &task.Status, &task.Created, &task.FilePath)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("task not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	// Load tags
	rows, err := r.db.DB().Query(`
		SELECT tag FROM phase2_task_tags WHERE task_id = ?
	`, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get tags: %w", err)
	}
	defer rows.Close()

	task.Tags = make([]string, 0)
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, fmt.Errorf("failed to scan tag: %w", err)
		}
		task.Tags = append(task.Tags, tag)
	}

	return &task, nil
}

// ListTasks retrieves tasks filtered by status and tags
func (r *Repository) ListTasks(status *Status, tags []string) ([]Task, error) {
	query := "SELECT id, title, status, created, file_path FROM phase2_tasks WHERE 1=1"
	args := make([]interface{}, 0)

	if status != nil {
		query += " AND status = ?"
		args = append(args, *status)
	}

	if len(tags) > 0 {
		// Tasks must have all specified tags
		query += fmt.Sprintf(` AND id IN (
			SELECT task_id FROM phase2_task_tags
			WHERE tag IN (%s)
			GROUP BY task_id
			HAVING COUNT(DISTINCT tag) = ?
		)`, placeholders(len(tags)))

		for _, tag := range tags {
			args = append(args, tag)
		}
		args = append(args, len(tags))
	}

	query += " ORDER BY created DESC"

	rows, err := r.db.DB().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list tasks: %w", err)
	}
	defer rows.Close()

	tasks := make([]Task, 0)
	for rows.Next() {
		var task Task
		if err := rows.Scan(&task.ID, &task.Title, &task.Status, &task.Created, &task.FilePath); err != nil {
			return nil, fmt.Errorf("failed to scan task: %w", err)
		}

		// Load tags for each task
		tagRows, err := r.db.DB().Query("SELECT tag FROM phase2_task_tags WHERE task_id = ?", task.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get tags: %w", err)
		}

		task.Tags = make([]string, 0)
		for tagRows.Next() {
			var tag string
			if err := tagRows.Scan(&tag); err != nil {
				tagRows.Close()
				return nil, fmt.Errorf("failed to scan tag: %w", err)
			}
			task.Tags = append(task.Tags, tag)
		}
		tagRows.Close()

		tasks = append(tasks, task)
	}

	return tasks, nil
}

// DeleteTask deletes a task from the database
func (r *Repository) DeleteTask(id string) error {
	_, err := r.db.DB().Exec("DELETE FROM phase2_tasks WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete task: %w", err)
	}
	return nil
}

// placeholders generates SQL placeholders like "?,?,?"
func placeholders(n int) string {
	if n == 0 {
		return ""
	}
	result := "?"
	for i := 1; i < n; i++ {
		result += ",?"
	}
	return result
}
