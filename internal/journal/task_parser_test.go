package journal

import (
	"strings"
	"testing"
)

func TestParseTasksFile(t *testing.T) {
	t.Run("parse empty content", func(t *testing.T) {
		content := `# Tasks

`
		tasks, err := ParseTasksFile(content)
		if err != nil {
			t.Fatalf("ParseTasksFile failed: %v", err)
		}
		if len(tasks) != 0 {
			t.Errorf("Expected 0 tasks, got %d", len(tasks))
		}
	})

	t.Run("parse single open task without notes", func(t *testing.T) {
		content := `# Tasks

- [ ] task-123 Buy groceries
`
		tasks, err := ParseTasksFile(content)
		if err != nil {
			t.Fatalf("ParseTasksFile failed: %v", err)
		}
		if len(tasks) != 1 {
			t.Fatalf("Expected 1 task, got %d", len(tasks))
		}
		task := tasks[0]
		if task.ID != "task-123" {
			t.Errorf("Expected task ID 'task-123', got '%s'", task.ID)
		}
		if task.Text != "Buy groceries" {
			t.Errorf("Expected task text 'Buy groceries', got '%s'", task.Text)
		}
		if task.Status != TaskOpen {
			t.Errorf("Expected task status 'open', got '%s'", task.Status)
		}
		if len(task.Notes) != 0 {
			t.Errorf("Expected 0 notes, got %d", len(task.Notes))
		}
		if task.Position != 0 {
			t.Errorf("Expected position 0, got %d", task.Position)
		}
	})

	t.Run("parse single completed task without notes", func(t *testing.T) {
		content := `# Tasks

- [x] task-456 Complete the project
`
		tasks, err := ParseTasksFile(content)
		if err != nil {
			t.Fatalf("ParseTasksFile failed: %v", err)
		}
		if len(tasks) != 1 {
			t.Fatalf("Expected 1 task, got %d", len(tasks))
		}
		task := tasks[0]
		if task.ID != "task-456" {
			t.Errorf("Expected task ID 'task-456', got '%s'", task.ID)
		}
		if task.Text != "Complete the project" {
			t.Errorf("Expected task text 'Complete the project', got '%s'", task.Text)
		}
		if task.Status != TaskDone {
			t.Errorf("Expected task status 'done', got '%s'", task.Status)
		}
	})

	t.Run("parse task with single note", func(t *testing.T) {
		content := `# Tasks

- [ ] task-789 Write documentation
  - note-001 Remember to include examples
`
		tasks, err := ParseTasksFile(content)
		if err != nil {
			t.Fatalf("ParseTasksFile failed: %v", err)
		}
		if len(tasks) != 1 {
			t.Fatalf("Expected 1 task, got %d", len(tasks))
		}
		task := tasks[0]
		if len(task.Notes) != 1 {
			t.Fatalf("Expected 1 note, got %d", len(task.Notes))
		}
		note := task.Notes[0]
		if note.ID != "note-001" {
			t.Errorf("Expected note ID 'note-001', got '%s'", note.ID)
		}
		if note.Text != "Remember to include examples" {
			t.Errorf("Expected note text 'Remember to include examples', got '%s'", note.Text)
		}
		if note.Position != 0 {
			t.Errorf("Expected note position 0, got %d", note.Position)
		}
	})

	t.Run("parse task with multiple notes", func(t *testing.T) {
		content := `# Tasks

- [ ] task-abc Review code
  - note-001 Check for edge cases
  - note-002 Run all tests
  - note-003 Update changelog
`
		tasks, err := ParseTasksFile(content)
		if err != nil {
			t.Fatalf("ParseTasksFile failed: %v", err)
		}
		if len(tasks) != 1 {
			t.Fatalf("Expected 1 task, got %d", len(tasks))
		}
		task := tasks[0]
		if len(task.Notes) != 3 {
			t.Fatalf("Expected 3 notes, got %d", len(task.Notes))
		}
		expectedNotes := []struct {
			id   string
			text string
			pos  int
		}{
			{"note-001", "Check for edge cases", 0},
			{"note-002", "Run all tests", 1},
			{"note-003", "Update changelog", 2},
		}
		for i, expected := range expectedNotes {
			note := task.Notes[i]
			if note.ID != expected.id {
				t.Errorf("Expected note %d ID '%s', got '%s'", i, expected.id, note.ID)
			}
			if note.Text != expected.text {
				t.Errorf("Expected note %d text '%s', got '%s'", i, expected.text, note.Text)
			}
			if note.Position != expected.pos {
				t.Errorf("Expected note %d position %d, got %d", i, expected.pos, note.Position)
			}
		}
	})

	t.Run("parse multiple tasks with mixed statuses and notes", func(t *testing.T) {
		content := `# Tasks

- [ ] task-001 First task
  - note-001 First note
- [x] task-002 Second task completed
- [ ] task-003 Third task
  - note-002 Another note
  - note-003 Yet another note
`
		tasks, err := ParseTasksFile(content)
		if err != nil {
			t.Fatalf("ParseTasksFile failed: %v", err)
		}
		if len(tasks) != 3 {
			t.Fatalf("Expected 3 tasks, got %d", len(tasks))
		}

		// Check first task
		if tasks[0].ID != "task-001" {
			t.Errorf("Expected task 0 ID 'task-001', got '%s'", tasks[0].ID)
		}
		if tasks[0].Status != TaskOpen {
			t.Errorf("Expected task 0 status 'open', got '%s'", tasks[0].Status)
		}
		if len(tasks[0].Notes) != 1 {
			t.Errorf("Expected task 0 to have 1 note, got %d", len(tasks[0].Notes))
		}
		if tasks[0].Position != 0 {
			t.Errorf("Expected task 0 position 0, got %d", tasks[0].Position)
		}

		// Check second task
		if tasks[1].ID != "task-002" {
			t.Errorf("Expected task 1 ID 'task-002', got '%s'", tasks[1].ID)
		}
		if tasks[1].Status != TaskDone {
			t.Errorf("Expected task 1 status 'done', got '%s'", tasks[1].Status)
		}
		if len(tasks[1].Notes) != 0 {
			t.Errorf("Expected task 1 to have 0 notes, got %d", len(tasks[1].Notes))
		}
		if tasks[1].Position != 1 {
			t.Errorf("Expected task 1 position 1, got %d", tasks[1].Position)
		}

		// Check third task
		if tasks[2].ID != "task-003" {
			t.Errorf("Expected task 2 ID 'task-003', got '%s'", tasks[2].ID)
		}
		if tasks[2].Status != TaskOpen {
			t.Errorf("Expected task 2 status 'open', got '%s'", tasks[2].Status)
		}
		if len(tasks[2].Notes) != 2 {
			t.Errorf("Expected task 2 to have 2 notes, got %d", len(tasks[2].Notes))
		}
		if tasks[2].Position != 2 {
			t.Errorf("Expected task 2 position 2, got %d", tasks[2].Position)
		}
	})

	t.Run("ignore empty lines and extra whitespace", func(t *testing.T) {
		content := `# Tasks


- [ ] task-001 Task with spacing
  - note-001 Note with spacing


- [x] task-002 Another task

`
		tasks, err := ParseTasksFile(content)
		if err != nil {
			t.Fatalf("ParseTasksFile failed: %v", err)
		}
		if len(tasks) != 2 {
			t.Fatalf("Expected 2 tasks, got %d", len(tasks))
		}
	})

	t.Run("handle tasks without ID prefix", func(t *testing.T) {
		content := `# Tasks

- [ ] Just task text without ID
`
		tasks, err := ParseTasksFile(content)
		if err != nil {
			t.Fatalf("ParseTasksFile failed: %v", err)
		}
		if len(tasks) != 1 {
			t.Fatalf("Expected 1 task, got %d", len(tasks))
		}
		// The ID should be extracted or the entire text used
		if tasks[0].Text != "Just task text without ID" {
			t.Errorf("Expected text 'Just task text without ID', got '%s'", tasks[0].Text)
		}
		// ID should be auto-generated
		if tasks[0].ID == "" {
			t.Errorf("Expected auto-generated ID, got empty string")
		}
	})

	t.Run("handle notes without ID prefix", func(t *testing.T) {
		content := `# Tasks

- [ ] task-001 Task
  - Note without ID prefix
`
		tasks, err := ParseTasksFile(content)
		if err != nil {
			t.Fatalf("ParseTasksFile failed: %v", err)
		}
		if len(tasks) != 1 {
			t.Fatalf("Expected 1 task, got %d", len(tasks))
		}
		if len(tasks[0].Notes) != 1 {
			t.Fatalf("Expected 1 note, got %d", len(tasks[0].Notes))
		}
		if tasks[0].Notes[0].Text != "Note without ID prefix" {
			t.Errorf("Expected note text 'Note without ID prefix', got '%s'", tasks[0].Notes[0].Text)
		}
		// ID should be auto-generated
		if tasks[0].Notes[0].ID == "" {
			t.Errorf("Expected auto-generated ID, got empty string")
		}
	})

	t.Run("ignore orphaned notes without parent task", func(t *testing.T) {
		content := `# Tasks

  - orphan-note This note has no parent task
- [ ] task-001 Valid task
`
		tasks, err := ParseTasksFile(content)
		if err != nil {
			t.Fatalf("ParseTasksFile failed: %v", err)
		}
		if len(tasks) != 1 {
			t.Fatalf("Expected 1 task (orphaned note should be ignored), got %d", len(tasks))
		}
		if tasks[0].ID != "task-001" {
			t.Errorf("Expected task ID 'task-001', got '%s'", tasks[0].ID)
		}
		if len(tasks[0].Notes) != 0 {
			t.Errorf("Expected 0 notes, got %d", len(tasks[0].Notes))
		}
	})

	t.Run("handle empty content gracefully", func(t *testing.T) {
		tasks, err := ParseTasksFile("")
		if err != nil {
			t.Fatalf("ParseTasksFile failed: %v", err)
		}
		if len(tasks) != 0 {
			t.Errorf("Expected 0 tasks for empty content, got %d", len(tasks))
		}
	})

	t.Run("handle whitespace-only content gracefully", func(t *testing.T) {
		tasks, err := ParseTasksFile("   \n\n   \n")
		if err != nil {
			t.Fatalf("ParseTasksFile failed: %v", err)
		}
		if len(tasks) != 0 {
			t.Errorf("Expected 0 tasks for whitespace-only content, got %d", len(tasks))
		}
	})

	t.Run("accept flexible checkbox spacing", func(t *testing.T) {
		content := `# Tasks

- [] task-001 No space in checkbox
- [  ] task-002 Multiple spaces in checkbox
`
		tasks, err := ParseTasksFile(content)
		if err != nil {
			t.Fatalf("ParseTasksFile failed: %v", err)
		}
		if len(tasks) != 2 {
			t.Fatalf("Expected 2 tasks, got %d", len(tasks))
		}
		if tasks[0].ID != "task-001" {
			t.Errorf("Expected task ID 'task-001', got '%s'", tasks[0].ID)
		}
		if tasks[1].ID != "task-002" {
			t.Errorf("Expected task ID 'task-002', got '%s'", tasks[1].ID)
		}
	})

	t.Run("accept uppercase X in done tasks", func(t *testing.T) {
		content := `# Tasks

- [X] task-001 Task with uppercase X
- [x] task-002 Task with lowercase x
`
		tasks, err := ParseTasksFile(content)
		if err != nil {
			t.Fatalf("ParseTasksFile failed: %v", err)
		}
		if len(tasks) != 2 {
			t.Fatalf("Expected 2 tasks, got %d", len(tasks))
		}
		if tasks[0].Status != TaskDone {
			t.Errorf("Expected task 0 to be done, got status '%s'", tasks[0].Status)
		}
		if tasks[1].Status != TaskDone {
			t.Errorf("Expected task 1 to be done, got status '%s'", tasks[1].Status)
		}
	})

	t.Run("validate ID extraction patterns", func(t *testing.T) {
		content := `# Tasks

- [ ] ab-123 This ID should be accepted (2+ chars before hyphen)
- [ ] task-001 This ID should be accepted
- [ ] a-1 This has only 1 char before hyphen - ID should be auto-generated
`
		tasks, err := ParseTasksFile(content)
		if err != nil {
			t.Fatalf("ParseTasksFile failed: %v", err)
		}
		if len(tasks) != 3 {
			t.Fatalf("Expected 3 tasks, got %d", len(tasks))
		}
		// First task should have ID extracted (2 chars before hyphen)
		if tasks[0].ID != "ab-123" {
			t.Errorf("Expected task ID 'ab-123', got '%s'", tasks[0].ID)
		}
		if tasks[0].Text != "This ID should be accepted (2+ chars before hyphen)" {
			t.Errorf("Expected correct text extraction, got '%s'", tasks[0].Text)
		}
		// Second task should have ID extracted
		if tasks[1].ID != "task-001" {
			t.Errorf("Expected task ID 'task-001', got '%s'", tasks[1].ID)
		}
		// Third task should have auto-generated ID (single char before hyphen not accepted)
		if tasks[2].ID == "a-1" {
			t.Errorf("Expected auto-generated ID for single-char prefix, got '%s'", tasks[2].ID)
		}
		if tasks[2].Text != "a-1 This has only 1 char before hyphen - ID should be auto-generated" {
			t.Errorf("Expected full text when ID not extracted, got '%s'", tasks[2].Text)
		}
	})

	t.Run("set CreatedAt field when parsing", func(t *testing.T) {
		content := `# Tasks

- [ ] task-001 Test task
`
		tasks, err := ParseTasksFile(content)
		if err != nil {
			t.Fatalf("ParseTasksFile failed: %v", err)
		}
		if len(tasks) != 1 {
			t.Fatalf("Expected 1 task, got %d", len(tasks))
		}
		if tasks[0].CreatedAt.IsZero() {
			t.Errorf("Expected CreatedAt to be set, got zero time")
		}
	})
}

func TestWriteTasksFile(t *testing.T) {
	t.Run("write empty tasks list", func(t *testing.T) {
		tasks := []Task{}
		content := WriteTasksFile(tasks)
		expected := `# Tasks

`
		if content != expected {
			t.Errorf("Expected:\n%s\nGot:\n%s", expected, content)
		}
	})

	t.Run("write nil tasks list gracefully", func(t *testing.T) {
		content := WriteTasksFile(nil)
		expected := `# Tasks

`
		if content != expected {
			t.Errorf("Expected:\n%s\nGot:\n%s", expected, content)
		}
	})

	t.Run("write single task without notes", func(t *testing.T) {
		task := Task{
			ID:       "task-123",
			Text:     "Buy groceries",
			Status:   TaskOpen,
			Notes:    []TaskNote{},
			Position: 0,
		}
		tasks := []Task{task}
		content := WriteTasksFile(tasks)
		expected := `# Tasks

- [ ] task-123 Buy groceries
`
		if content != expected {
			t.Errorf("Expected:\n%s\nGot:\n%s", expected, content)
		}
	})

	t.Run("write completed task without notes", func(t *testing.T) {
		task := Task{
			ID:       "task-456",
			Text:     "Complete the project",
			Status:   TaskDone,
			Notes:    []TaskNote{},
			Position: 0,
		}
		tasks := []Task{task}
		content := WriteTasksFile(tasks)
		expected := `# Tasks

- [x] task-456 Complete the project
`
		if content != expected {
			t.Errorf("Expected:\n%s\nGot:\n%s", expected, content)
		}
	})

	t.Run("write task with single note", func(t *testing.T) {
		task := Task{
			ID:     "task-789",
			Text:   "Write documentation",
			Status: TaskOpen,
			Notes: []TaskNote{
				{ID: "note-001", Text: "Remember to include examples", Position: 0},
			},
			Position: 0,
		}
		tasks := []Task{task}
		content := WriteTasksFile(tasks)
		expected := `# Tasks

- [ ] task-789 Write documentation
  - note-001 Remember to include examples
`
		if content != expected {
			t.Errorf("Expected:\n%s\nGot:\n%s", expected, content)
		}
	})

	t.Run("write task with multiple notes", func(t *testing.T) {
		task := Task{
			ID:     "task-abc",
			Text:   "Review code",
			Status: TaskOpen,
			Notes: []TaskNote{
				{ID: "note-001", Text: "Check for edge cases", Position: 0},
				{ID: "note-002", Text: "Run all tests", Position: 1},
				{ID: "note-003", Text: "Update changelog", Position: 2},
			},
			Position: 0,
		}
		tasks := []Task{task}
		content := WriteTasksFile(tasks)
		expected := `# Tasks

- [ ] task-abc Review code
  - note-001 Check for edge cases
  - note-002 Run all tests
  - note-003 Update changelog
`
		if content != expected {
			t.Errorf("Expected:\n%s\nGot:\n%s", expected, content)
		}
	})

	t.Run("write multiple tasks with mixed statuses", func(t *testing.T) {
		tasks := []Task{
			{
				ID:       "task-001",
				Text:     "First task",
				Status:   TaskOpen,
				Notes:    []TaskNote{{ID: "note-001", Text: "First note", Position: 0}},
				Position: 0,
			},
			{
				ID:       "task-002",
				Text:     "Second task completed",
				Status:   TaskDone,
				Notes:    []TaskNote{},
				Position: 1,
			},
			{
				ID:     "task-003",
				Text:   "Third task",
				Status: TaskOpen,
				Notes: []TaskNote{
					{ID: "note-002", Text: "Another note", Position: 0},
					{ID: "note-003", Text: "Yet another note", Position: 1},
				},
				Position: 2,
			},
		}
		content := WriteTasksFile(tasks)
		expected := `# Tasks

- [ ] task-001 First task
  - note-001 First note
- [x] task-002 Second task completed
- [ ] task-003 Third task
  - note-002 Another note
  - note-003 Yet another note
`
		if content != expected {
			t.Errorf("Expected:\n%s\nGot:\n%s", expected, content)
		}
	})

	t.Run("sort tasks by position when writing", func(t *testing.T) {
		tasks := []Task{
			{ID: "task-003", Text: "Third", Status: TaskOpen, Notes: []TaskNote{}, Position: 2},
			{ID: "task-001", Text: "First", Status: TaskOpen, Notes: []TaskNote{}, Position: 0},
			{ID: "task-002", Text: "Second", Status: TaskOpen, Notes: []TaskNote{}, Position: 1},
		}
		content := WriteTasksFile(tasks)
		lines := strings.Split(strings.TrimSpace(content), "\n")
		if !strings.Contains(lines[2], "task-001") {
			t.Errorf("Expected task-001 at position 0, got: %s", lines[2])
		}
		if !strings.Contains(lines[3], "task-002") {
			t.Errorf("Expected task-002 at position 1, got: %s", lines[3])
		}
		if !strings.Contains(lines[4], "task-003") {
			t.Errorf("Expected task-003 at position 2, got: %s", lines[4])
		}
	})

	t.Run("sort notes by position when writing", func(t *testing.T) {
		task := Task{
			ID:     "task-001",
			Text:   "Task with unsorted notes",
			Status: TaskOpen,
			Notes: []TaskNote{
				{ID: "note-003", Text: "Third note", Position: 2},
				{ID: "note-001", Text: "First note", Position: 0},
				{ID: "note-002", Text: "Second note", Position: 1},
			},
			Position: 0,
		}
		tasks := []Task{task}
		content := WriteTasksFile(tasks)
		lines := strings.Split(strings.TrimSpace(content), "\n")
		if !strings.Contains(lines[3], "note-001") {
			t.Errorf("Expected note-001 at position 0, got: %s", lines[3])
		}
		if !strings.Contains(lines[4], "note-002") {
			t.Errorf("Expected note-002 at position 1, got: %s", lines[4])
		}
		if !strings.Contains(lines[5], "note-003") {
			t.Errorf("Expected note-003 at position 2, got: %s", lines[5])
		}
	})
}

func TestRoundTripParsing(t *testing.T) {
	t.Run("parse and write produces same output", func(t *testing.T) {
		original := `# Tasks

- [ ] task-001 First task
  - note-001 First note
- [x] task-002 Second task completed
- [ ] task-003 Third task
  - note-002 Another note
  - note-003 Yet another note
`
		tasks, err := ParseTasksFile(original)
		if err != nil {
			t.Fatalf("ParseTasksFile failed: %v", err)
		}
		written := WriteTasksFile(tasks)
		if written != original {
			t.Errorf("Round trip failed.\nExpected:\n%s\nGot:\n%s", original, written)
		}
	})
}
