package migrate

import (
	"testing"
	"time"
)

func TestGenerateSlug(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello World", "hello-world"},
		{"test task", "test-task"},
		{"Multiple   Spaces", "multiple-spaces"},
		{"Special!@#$%Characters", "special-characters"},
		{"UPPERCASE", "uppercase"},
		{"  leading and trailing  ", "leading-and-trailing"},
		{"VeryLongTitleThatExceedsFiftyCharactersShouldBeTruncatedXYZ", "verylongtitlethatexceedsfiftycharactersshouldbetru"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := GenerateSlug(tt.input)
			if result != tt.expected {
				t.Errorf("GenerateSlug(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGenerateSlugEmpty(t *testing.T) {
	result := GenerateSlug("")
	if result != "untitled" {
		t.Errorf("GenerateSlug('') = %q, want %q", result, "untitled")
	}
}

func TestGenerateSlugSpecialChars(t *testing.T) {
	result := GenerateSlug("Task: Fix Bug/Major Issue")
	expected := "task-fix-bug-major-issue"
	if result != expected {
		t.Errorf("GenerateSlug() = %q, want %q", result, expected)
	}
}

func TestLogEntrySlug(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello World", "hello-world"},
		{"Working on API", "working-on-api"},
		{"Multiple words here", "multiple-words-here"},
		{"One", "one"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := LogEntrySlug(tt.input)
			if result != tt.expected {
				t.Errorf("LogEntrySlug(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestLogEntrySlugEmpty(t *testing.T) {
	result := LogEntrySlug("")
	if result != "untitled" {
		t.Errorf("LogEntrySlug('') = %q, want %q", result, "untitled")
	}
}

func TestTaskFilename(t *testing.T) {
	createdAt := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	task := TaskMigrationInfo{
		ID:        "test-id",
		Title:     "Test Task",
		CreatedAt: createdAt,
	}

	result := TaskFilename(task)
	expected := "2026-01-15-test-task.md"
	if result != expected {
		t.Errorf("TaskFilename() = %q, want %q", result, expected)
	}
}

func TestLogEntryFilename(t *testing.T) {
	timestamp := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	info := LogEntryMigrationInfo{
		ID:        "test-id",
		Content:   "Daily standup meeting",
		Timestamp: timestamp,
	}

	result := LogEntryFilename(info)
	expected := "20260115-daily-standup-meeting.md"
	if result != expected {
		t.Errorf("LogEntryFilename() = %q, want %q", result, expected)
	}
}

func TestIsNewTaskFormat(t *testing.T) {
	tests := []struct {
		filename string
		expected bool
	}{
		{"2026-01-15-test-task.md", true},
		{"2025-12-24-fix-bug.md", true},
		{"2024-01-01-new-years-day.md", true},
		{"abc123.md", false},
		{"old-format.md", false},
		{"2026-01.md", false},
		{"not-enough.md", false},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := isNewTaskFormat(tt.filename)
			if result != tt.expected {
				t.Errorf("isNewTaskFormat(%q) = %v, want %v", tt.filename, result, tt.expected)
			}
		})
	}
}
