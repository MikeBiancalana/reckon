package task

import (
	"time"

	"github.com/rs/xid"
)

// Status represents the status of a task
type Status string

const (
	StatusActive  Status = "active"
	StatusDone    Status = "done"
	StatusWaiting Status = "waiting"
	StatusSomeday Status = "someday"
)

// LogEntry represents a log entry for a task
type LogEntry struct {
	ID        string    `json:"id"`
	Date      string    `json:"date"`      // YYYY-MM-DD
	Timestamp time.Time `json:"timestamp"` // Full timestamp
	Content   string    `json:"content"`
}

// NewLogEntry creates a new log entry
func NewLogEntry(timestamp time.Time, content string) *LogEntry {
	return &LogEntry{
		ID:        xid.New().String(),
		Date:      timestamp.Format("2006-01-02"),
		Timestamp: timestamp,
		Content:   content,
	}
}

// Task represents a multi-day task
type Task struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Status      Status     `json:"status"`
	Created     string     `json:"created"` // YYYY-MM-DD format
	Tags        []string   `json:"tags"`
	Description string     `json:"description"`
	LogEntries  []LogEntry `json:"log_entries"`
	FilePath    string     `json:"file_path"`
}

// NewTask creates a new task
func NewTask(title string, tags []string) *Task {
	return &Task{
		ID:          xid.New().String(),
		Title:       title,
		Status:      StatusActive,
		Created:     time.Now().Format("2006-01-02"),
		Tags:        tags,
		Description: "",
		LogEntries:  make([]LogEntry, 0),
	}
}

// AppendLog adds a log entry to the task
func (t *Task) AppendLog(entry LogEntry) {
	t.LogEntries = append(t.LogEntries, entry)
}

// SetStatus updates the task status
func (t *Task) SetStatus(status Status) {
	t.Status = status
}
