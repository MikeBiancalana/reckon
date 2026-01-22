package journal

import (
	"time"

	"github.com/rs/xid"
)

// IntentionStatus represents the status of an intention
type IntentionStatus string

const (
	IntentionOpen    IntentionStatus = "open"
	IntentionDone    IntentionStatus = "done"
	IntentionCarried IntentionStatus = "carried"
)

// TaskStatus represents the status of a task
type TaskStatus string

const (
	TaskOpen TaskStatus = "open"
	TaskDone TaskStatus = "done"
)

// EntryType represents the type of a log entry
type EntryType string

const (
	EntryTypeLog     EntryType = "log"
	EntryTypeMeeting EntryType = "meeting"
	EntryTypeBreak   EntryType = "break"
)

// Intention represents a daily intention/task
type Intention struct {
	ID          string          `json:"id"`
	Text        string          `json:"text"`
	Status      IntentionStatus `json:"status"`
	CarriedFrom string          `json:"carried_from,omitempty"` // date from which this was carried
	Position    int             `json:"position"`
}

// NewIntention creates a new intention with a generated ID
func NewIntention(text string, position int) *Intention {
	return &Intention{
		ID:       xid.New().String(),
		Text:     text,
		Status:   IntentionOpen,
		Position: position,
	}
}

// NewCarriedIntention creates a new intention carried from a previous day
func NewCarriedIntention(text string, carriedFrom string, position int) *Intention {
	return &Intention{
		ID:          xid.New().String(),
		Text:        text,
		Status:      IntentionCarried,
		CarriedFrom: carriedFrom,
		Position:    position,
	}
}

// LogEntry represents a timestamped log entry
type LogEntry struct {
	ID              string    `json:"id"`
	Timestamp       time.Time `json:"timestamp"`
	Content         string    `json:"content"`
	TaskID          string    `json:"task_id,omitempty"`
	EntryType       EntryType `json:"entry_type"`
	DurationMinutes int       `json:"duration_minutes,omitempty"`
	Notes           []LogNote `json:"notes"`
	Position        int       `json:"position"`
}

// NewLogEntry creates a new log entry with a generated ID
func NewLogEntry(timestamp time.Time, content string, entryType EntryType, position int) *LogEntry {
	return &LogEntry{
		ID:        xid.New().String(),
		Timestamp: timestamp,
		Content:   content,
		EntryType: entryType,
		Notes:     make([]LogNote, 0),
		Position:  position,
	}
}

// Win represents a daily win/accomplishment
type Win struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Position int    `json:"position"`
}

// NewWin creates a new win with a generated ID
func NewWin(text string, position int) *Win {
	return &Win{
		ID:       xid.New().String(),
		Text:     text,
		Position: position,
	}
}

// Task represents a global task
type Task struct {
	ID            string     `json:"id"`
	Text          string     `json:"text"`
	Status        TaskStatus `json:"status"`
	Tags          []string   `json:"tags"`
	Notes         []TaskNote `json:"notes"`
	Position      int        `json:"position"`
	CreatedAt     time.Time  `json:"created_at"`
	ScheduledDate *string    `json:"scheduled_date,omitempty"` // YYYY-MM-DD format, nil if unscheduled
	DeadlineDate  *string    `json:"deadline_date,omitempty"`  // YYYY-MM-DD format, nil if no deadline
}

// NewTask creates a new task with a generated ID
func NewTask(text string, tags []string, position int) *Task {
	return &Task{
		ID:        xid.New().String(),
		Text:      text,
		Status:    TaskOpen,
		Tags:      tags,
		Notes:     make([]TaskNote, 0),
		Position:  position,
		CreatedAt: time.Now(),
	}
}

// TaskNote represents a note attached to a task
type TaskNote struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Position int    `json:"position"`
}

// NewTaskNote creates a new task note with a generated ID
func NewTaskNote(text string, position int) *TaskNote {
	return &TaskNote{
		ID:       xid.New().String(),
		Text:     text,
		Position: position,
	}
}

// LogNote represents a note attached to a log entry
type LogNote struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Position int    `json:"position"`
	// Future: NoteSlug string for zettelkasten
}

// NewLogNote creates a new log note with a generated XID.
// Deprecated: This function generates XID-format IDs which are inconsistent with
// the parser's position-based ID format. Service layer methods (AddLogNote, etc.)
// now generate position-based IDs directly. This function is retained only for
// backward compatibility with existing tests.
func NewLogNote(text string, position int) *LogNote {
	return &LogNote{
		ID:       xid.New().String(),
		Text:     text,
		Position: position,
	}
}

// ScheduleItem represents a scheduled item in a journal
type ScheduleItem struct {
	ID       string    `json:"id"`
	Time     time.Time `json:"time"`
	Content  string    `json:"content"`
	Position int       `json:"position"`
}

// NewScheduleItem creates a new schedule item with a generated ID
func NewScheduleItem(time time.Time, content string, position int) *ScheduleItem {
	return &ScheduleItem{
		ID:       xid.New().String(),
		Time:     time,
		Content:  content,
		Position: position,
	}
}

// Journal represents a daily journal entry
type Journal struct {
	Date          string         `json:"date"` // YYYY-MM-DD format
	Intentions    []Intention    `json:"intentions"`
	Wins          []Win          `json:"wins"`
	LogEntries    []LogEntry     `json:"log_entries"`
	ScheduleItems []ScheduleItem `json:"schedule_items"`
	FilePath      string         `json:"file_path"`
	LastModified  time.Time      `json:"last_modified"`
}

// NewJournal creates a new empty journal for the given date
func NewJournal(date string) *Journal {
	return &Journal{
		Date:          date,
		Intentions:    make([]Intention, 0),
		Wins:          make([]Win, 0),
		LogEntries:    make([]LogEntry, 0),
		ScheduleItems: make([]ScheduleItem, 0),
	}
}
