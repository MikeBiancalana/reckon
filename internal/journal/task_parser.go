package journal

import (
	"bufio"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/rs/xid"
	"gopkg.in/yaml.v3"
)

var (
	// Task patterns - flexible spacing in checkboxes, accept both [x] and [X]
	taskOpenRe = regexp.MustCompile(`^-\s+\[\s*\]\s+(.+)$`)
	taskDoneRe = regexp.MustCompile(`^-\s+\[(?i:x)\]\s+(.+)$`)

	// Note pattern - matches lines with 2-space indentation
	taskNoteRe = regexp.MustCompile(`^\s{2}-\s+(.+)$`)

	// ID token pattern - matches individual ID tokens
	// Supports two formats:
	// 1. Hyphenated IDs: prefix-suffix where prefix has 2+ chars (e.g., task-123, note-001)
	// 2. XID format: 15+ alphanumeric characters (e.g., d58mbq96rjumohmic4dg which is 20 chars)
	idTokenRe = regexp.MustCompile(`^([a-zA-Z][a-zA-Z0-9_]+-[a-zA-Z0-9_-]+|[a-zA-Z0-9]{15,})$`)

	// Frontmatter pattern - matches the opening/closing ---
	frontmatterRe = regexp.MustCompile(`^---\s*$`)
)

var ErrNoFrontmatter = errors.New("no frontmatter found")

type TaskFileFrontmatter struct {
	ID        string   `yaml:"id"`
	Title     string   `yaml:"title"`
	Created   string   `yaml:"created"`
	Status    string   `yaml:"status"`
	Tags      []string `yaml:"tags,omitempty"`
	Scheduled *string  `yaml:"scheduled,omitempty"`
	Deadline  *string  `yaml:"deadline,omitempty"`
}

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
// Handles multiple IDs before the actual text content
// Returns the first ID and the remaining text, or empty ID and original text if no ID found
func extractID(text string) (id string, remaining string) {
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return "", text
	}

	i := 0
	for i < len(parts) {
		if idTokenRe.MatchString(parts[i]) {
			if id == "" {
				id = parts[i]
			}
			i++
		} else {
			break
		}
	}

	remaining = strings.Join(parts[i:], " ")
	return id, remaining
}

func parseTaskFileFrontmatter(frontmatterContent string) (*TaskFileFrontmatter, error) {
	var fm TaskFileFrontmatter
	if err := yaml.Unmarshal([]byte(frontmatterContent), &fm); err != nil {
		return nil, fmt.Errorf("error parsing frontmatter: %w", err)
	}

	if fm.Scheduled != nil && !isValidDate(*fm.Scheduled) {
		return nil, fmt.Errorf("invalid scheduled date format: %s (expected YYYY-MM-DD)", *fm.Scheduled)
	}
	if fm.Deadline != nil && !isValidDate(*fm.Deadline) {
		return nil, fmt.Errorf("invalid deadline date format: %s (expected YYYY-MM-DD)", *fm.Deadline)
	}

	return &fm, nil
}

func isValidDate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

func newTaskFromFrontmatter(fm *TaskFileFrontmatter, notes []TaskNote) *Task {
	createdAt := time.Now()
	if fm.Created != "" {
		if parsed, err := time.Parse("2006-01-02", fm.Created); err == nil {
			createdAt = parsed
		}
	}

	status := TaskOpen
	if fm.Status == "done" {
		status = TaskDone
	}

	return &Task{
		ID:            fm.ID,
		Text:          fm.Title,
		Status:        status,
		Tags:          fm.Tags,
		Notes:         notes,
		Position:      0,
		CreatedAt:     createdAt,
		ScheduledDate: fm.Scheduled,
		DeadlineDate:  fm.Deadline,
	}
}

func newTaskFromLine(line string, status TaskStatus, position int) *Task {
	line = strings.TrimSpace(line)
	taskID, text := extractID(line)
	if taskID == "" {
		taskID = xid.New().String()
	}

	return &Task{
		ID:        taskID,
		Text:      text,
		Status:    status,
		Notes:     make([]TaskNote, 0),
		Position:  position,
		CreatedAt: time.Now(),
	}
}

func parseTaskFileContent(content string) (*Task, error) {
	lines := strings.Split(content, "\n")

	var fmStart, fmEnd int = -1, -1
	for i, line := range lines {
		if frontmatterRe.MatchString(line) {
			if fmStart == -1 {
				fmStart = i
			} else {
				fmEnd = i
				break
			}
		}
	}

	if fmStart == -1 || fmEnd == -1 {
		return nil, nil
	}

	frontmatterContent := strings.Join(lines[fmStart+1:fmEnd], "\n")
	fm, err := parseTaskFileFrontmatter(frontmatterContent)
	if err != nil {
		return nil, err
	}

	bodyStart := fmEnd + 1
	body := strings.Join(lines[bodyStart:], "\n")
	notes := parseNotesFromBody(body)

	return newTaskFromFrontmatter(fm, notes), nil
}

// ParseTaskFile parses a task file with YAML frontmatter and returns a Task.
// Returns (nil, nil) if content is empty.
// Returns (nil, ErrNoFrontmatter) if content has no frontmatter.
func ParseTaskFile(content string) (*Task, error) {
	if strings.TrimSpace(content) == "" {
		return nil, nil
	}

	task, err := parseTaskFileContent(content)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, ErrNoFrontmatter
	}
	return task, nil
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
