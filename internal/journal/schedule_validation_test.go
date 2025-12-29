package journal

import (
	"os"
	"testing"
)

func TestAddScheduleItem_EmptyContent(t *testing.T) {
	service, tmpDir := setupTestService(t)
	defer os.RemoveAll(tmpDir)

	date := "2024-01-15"
	j := NewJournal(date)

	// Test empty content
	err := service.AddScheduleItem(j, "09:00", "")
	if err == nil {
		t.Error("Expected error for empty content, got nil")
	}
	if err.Error() != "schedule item content cannot be empty" {
		t.Errorf("Expected specific error message, got: %v", err)
	}

	// Verify no item was added
	if len(j.ScheduleItems) != 0 {
		t.Errorf("Expected 0 schedule items, got %d", len(j.ScheduleItems))
	}
}

func TestAddScheduleItem_WhitespaceOnlyContent(t *testing.T) {
	service, tmpDir := setupTestService(t)
	defer os.RemoveAll(tmpDir)

	date := "2024-01-15"
	j := NewJournal(date)

	// Test whitespace-only content
	err := service.AddScheduleItem(j, "09:00", "   \t\n  ")
	if err == nil {
		t.Error("Expected error for whitespace-only content, got nil")
	}
	if err.Error() != "schedule item content cannot be empty" {
		t.Errorf("Expected specific error message, got: %v", err)
	}

	// Verify no item was added
	if len(j.ScheduleItems) != 0 {
		t.Errorf("Expected 0 schedule items, got %d", len(j.ScheduleItems))
	}
}

func TestAddScheduleItem_ContentWithLeadingTrailingWhitespace(t *testing.T) {
	service, tmpDir := setupTestService(t)
	defer os.RemoveAll(tmpDir)

	date := "2024-01-15"
	j := NewJournal(date)

	// Test content with leading/trailing whitespace
	err := service.AddScheduleItem(j, "09:00", "  Meeting with team  ")
	if err != nil {
		t.Fatalf("AddScheduleItem failed: %v", err)
	}

	// Verify item was added with trimmed content
	if len(j.ScheduleItems) != 1 {
		t.Fatalf("Expected 1 schedule item, got %d", len(j.ScheduleItems))
	}

	if j.ScheduleItems[0].Content != "Meeting with team" {
		t.Errorf("Expected trimmed content 'Meeting with team', got '%s'", j.ScheduleItems[0].Content)
	}
}
