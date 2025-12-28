package journal

import (
	"bufio"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/rs/xid"
)

var (
	// Task patterns - flexible spacing in checkboxes, accept both [x] and [X]
	taskOpenRe = regexp.MustCompile(`^-\s+\[\s*\]\s+(.+)$`)
	taskDoneRe = regexp.MustCompile(`^-\s+\[(?i:x)\]\s+(.+)$`)

	// Note pattern - matches lines with 2-space indentation
	taskNoteRe = regexp.MustCompile(`^\s{2}-\s+(.+)$`)

	// ID extraction pattern - matches IDs with hyphens (like task-123 or note-001)
	// Requires at least 2 characters before hyphen and 1 after to avoid matching dates or single chars
	// Note: This does NOT support TaskArchived status - archived tasks are not supported in markdown format
	idRe = regexp.MustCompile(`^([a-zA-Z][a-zA-Z0-9_]+-[a-zA-Z0-9_-]+)\s+(.+)$`)
)

// ParseTasksFile parses the tasks.md file content and returns a slice of tasks
func ParseTasksFile(content string) ([]Task, error) {
	// Validate input
	if strings.TrimSpace(content) == "" {
		return []Task{}, nil
	}

	tasks := make([]Task, 0)
	scanner := bufio.NewScanner(strings.NewReader(content))

	var currentTask *Task
	taskPos := 0
	notePos := 0

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and header
		if trimmed == "" || trimmed == "# Tasks" {
			continue
		}

		// Check if it's a task note (2-space indentation)
		if match := taskNoteRe.FindStringSubmatch(line); match != nil {
			if currentTask != nil {
				noteText := strings.TrimSpace(match[1])
				noteID, text := extractID(noteText)
				if noteID == "" {
					noteID = xid.New().String()
				}

				note := TaskNote{
					ID:       noteID,
					Text:     text,
					Position: notePos,
				}
				currentTask.Notes = append(currentTask.Notes, note)
				notePos++
			}
			continue
		}

		// Check if it's an open task
		if match := taskOpenRe.FindStringSubmatch(trimmed); match != nil {
			currentTask = finalizePreviousTask(currentTask, &tasks, &taskPos)
			currentTask = createTask(match[1], TaskOpen, taskPos)
			notePos = 0
			continue
		}

		// Check if it's a done task
		if match := taskDoneRe.FindStringSubmatch(trimmed); match != nil {
			currentTask = finalizePreviousTask(currentTask, &tasks, &taskPos)
			currentTask = createTask(match[1], TaskDone, taskPos)
			notePos = 0
			continue
		}
	}

	// Don't forget the last task
	if currentTask != nil {
		tasks = append(tasks, *currentTask)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error scanning tasks file: %w", err)
	}

	return tasks, nil
}

// createTask creates a new task from the given text and status
// Sets CreatedAt to current time (metadata, not stored in markdown)
func createTask(taskText string, status TaskStatus, position int) *Task {
	taskText = strings.TrimSpace(taskText)
	taskID, text := extractID(taskText)
	if taskID == "" {
		taskID = xid.New().String()
	}

	return &Task{
		ID:        taskID,
		Text:      text,
		Status:    status,
		Notes:     make([]TaskNote, 0),
		Position:  position,
		CreatedAt: time.Now(), // CreatedAt is metadata, not stored in markdown
	}
}

// finalizePreviousTask adds the current task to the list if it exists
// and increments the task position counter
func finalizePreviousTask(currentTask *Task, tasks *[]Task, taskPos *int) *Task {
	if currentTask != nil {
		*tasks = append(*tasks, *currentTask)
		(*taskPos)++
	}
	return nil
}

// extractID extracts an ID from the beginning of text if present
// Returns the ID and the remaining text, or empty ID and original text if no ID found
func extractID(text string) (id string, remaining string) {
	if match := idRe.FindStringSubmatch(text); match != nil {
		return match[1], strings.TrimSpace(match[2])
	}
	return "", text
}

// WriteTasksFile serializes tasks to markdown format
func WriteTasksFile(tasks []Task) string {
	// Validate input - handle nil or empty gracefully
	if tasks == nil {
		tasks = []Task{}
	}

	var sb strings.Builder

	// Write header
	sb.WriteString("# Tasks\n\n")

	// Sort tasks by position
	sortedTasks := make([]Task, len(tasks))
	copy(sortedTasks, tasks)
	sort.Slice(sortedTasks, func(i, j int) bool {
		return sortedTasks[i].Position < sortedTasks[j].Position
	})

	// Write each task
	for _, task := range sortedTasks {
		// Write task line with checkbox
		// Note: TaskArchived status is not supported in markdown format
		checkbox := "[ ]"
		if task.Status == TaskDone {
			checkbox = "[x]"
		}
		sb.WriteString(fmt.Sprintf("- %s %s %s\n", checkbox, task.ID, task.Text))

		// Sort and write notes
		sortedNotes := make([]TaskNote, len(task.Notes))
		copy(sortedNotes, task.Notes)
		sort.Slice(sortedNotes, func(i, j int) bool {
			return sortedNotes[i].Position < sortedNotes[j].Position
		})

		for _, note := range sortedNotes {
			sb.WriteString(fmt.Sprintf("  - %s %s\n", note.ID, note.Text))
		}
	}

	return sb.String()
}
