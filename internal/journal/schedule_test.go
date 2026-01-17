package journal

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MikeBiancalana/reckon/internal/storage"
)

func setupTestService(t *testing.T) (*Service, string) {
	t.Helper()

	// Create temporary directory for test
	tmpDir, err := os.MkdirTemp("", "reckon-schedule-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Set up config to point to our test directory
	// This is needed because FileStore uses config.JournalDir()
	origDataDir := os.Getenv("RECKON_DATA_DIR")
	os.Setenv("RECKON_DATA_DIR", tmpDir)
	t.Cleanup(func() {
		if origDataDir == "" {
			os.Unsetenv("RECKON_DATA_DIR")
		} else {
			os.Setenv("RECKON_DATA_DIR", origDataDir)
		}
	})

	// Create database
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := storage.NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Create file store
	fileStore := storage.NewFileStore()

	// Create service
	repo := NewRepository(db, nil)
	service := NewService(repo, fileStore, nil)

	return service, tmpDir
}

func TestAddScheduleItem_WithTime(t *testing.T) {
	service, tmpDir := setupTestService(t)
	defer os.RemoveAll(tmpDir)

	// Create a journal
	date := "2024-01-15"
	j := NewJournal(date)

	// Add schedule item with time
	err := service.AddScheduleItem(j, "09:00", "Morning standup")
	if err != nil {
		t.Fatalf("AddScheduleItem failed: %v", err)
	}

	// Verify item was added
	if len(j.ScheduleItems) != 1 {
		t.Fatalf("Expected 1 schedule item, got %d", len(j.ScheduleItems))
	}

	item := j.ScheduleItems[0]
	if item.Content != "Morning standup" {
		t.Errorf("Expected content 'Morning standup', got '%s'", item.Content)
	}
	if item.Position != 0 {
		t.Errorf("Expected position 0, got %d", item.Position)
	}
	if item.ID == "" {
		t.Error("Expected non-empty ID")
	}

	// Verify time was parsed correctly
	expectedTime, _ := time.Parse("2006-01-02 15:04", "2024-01-15 09:00")
	if !item.Time.Equal(expectedTime) {
		t.Errorf("Expected time %v, got %v", expectedTime, item.Time)
	}

	// Verify item was persisted to database
	dbJournal, err := service.repo.GetJournalByDate(date)
	if err != nil {
		t.Fatalf("Failed to get journal from database: %v", err)
	}
	if len(dbJournal.ScheduleItems) != 1 {
		t.Fatalf("Expected 1 schedule item in database, got %d", len(dbJournal.ScheduleItems))
	}
	if dbJournal.ScheduleItems[0].Content != "Morning standup" {
		t.Errorf("Expected content 'Morning standup' in database, got '%s'", dbJournal.ScheduleItems[0].Content)
	}
}

func TestAddScheduleItem_WithoutTime(t *testing.T) {
	service, tmpDir := setupTestService(t)
	defer os.RemoveAll(tmpDir)

	// Create a journal
	date := "2024-01-15"
	j := NewJournal(date)

	// Add schedule item without time
	err := service.AddScheduleItem(j, "", "Review PR")
	if err != nil {
		t.Fatalf("AddScheduleItem failed: %v", err)
	}

	// Verify item was added
	if len(j.ScheduleItems) != 1 {
		t.Fatalf("Expected 1 schedule item, got %d", len(j.ScheduleItems))
	}

	item := j.ScheduleItems[0]
	if item.Content != "Review PR" {
		t.Errorf("Expected content 'Review PR', got '%s'", item.Content)
	}
	if !item.Time.IsZero() {
		t.Errorf("Expected zero time, got %v", item.Time)
	}
}

func TestAddScheduleItem_InvalidTime(t *testing.T) {
	service, tmpDir := setupTestService(t)
	defer os.RemoveAll(tmpDir)

	// Create a journal
	date := "2024-01-15"
	j := NewJournal(date)

	// Try to add schedule item with invalid time
	err := service.AddScheduleItem(j, "25:00", "Invalid time")
	if err == nil {
		t.Error("Expected error for invalid time, got nil")
	}
}

func TestAddScheduleItem_MultipleItems(t *testing.T) {
	service, tmpDir := setupTestService(t)
	defer os.RemoveAll(tmpDir)

	// Create a journal
	date := "2024-01-15"
	j := NewJournal(date)

	// Add multiple schedule items
	items := []struct {
		time    string
		content string
	}{
		{"09:00", "Morning standup"},
		{"10:00", "Team meeting"},
		{"", "Review PRs"},
		{"14:00", "1:1 with manager"},
	}

	for _, item := range items {
		err := service.AddScheduleItem(j, item.time, item.content)
		if err != nil {
			t.Fatalf("AddScheduleItem failed: %v", err)
		}
	}

	// Verify all items were added
	if len(j.ScheduleItems) != 4 {
		t.Fatalf("Expected 4 schedule items, got %d", len(j.ScheduleItems))
	}

	// Verify positions are correct
	for i, item := range j.ScheduleItems {
		if item.Position != i {
			t.Errorf("Expected position %d for item %d, got %d", i, i, item.Position)
		}
	}
}

func TestDeleteScheduleItem_Success(t *testing.T) {
	service, tmpDir := setupTestService(t)
	defer os.RemoveAll(tmpDir)

	// Create a journal with schedule items
	date := "2024-01-15"
	j := NewJournal(date)

	service.AddScheduleItem(j, "09:00", "Morning standup")
	service.AddScheduleItem(j, "10:00", "Team meeting")
	service.AddScheduleItem(j, "14:00", "1:1 with manager")

	// Delete the middle item
	itemID := j.ScheduleItems[1].ID
	err := service.DeleteScheduleItem(j, itemID)
	if err != nil {
		t.Fatalf("DeleteScheduleItem failed: %v", err)
	}

	// Verify item was deleted
	if len(j.ScheduleItems) != 2 {
		t.Fatalf("Expected 2 schedule items, got %d", len(j.ScheduleItems))
	}

	// Verify remaining items
	if j.ScheduleItems[0].Content != "Morning standup" {
		t.Errorf("Expected first item 'Morning standup', got '%s'", j.ScheduleItems[0].Content)
	}
	if j.ScheduleItems[1].Content != "1:1 with manager" {
		t.Errorf("Expected second item '1:1 with manager', got '%s'", j.ScheduleItems[1].Content)
	}

	// Verify positions were re-indexed
	if j.ScheduleItems[0].Position != 0 {
		t.Errorf("Expected position 0 for first item, got %d", j.ScheduleItems[0].Position)
	}
	if j.ScheduleItems[1].Position != 1 {
		t.Errorf("Expected position 1 for second item, got %d", j.ScheduleItems[1].Position)
	}

	// Verify deletion was persisted to database
	dbJournal, err := service.repo.GetJournalByDate(date)
	if err != nil {
		t.Fatalf("Failed to get journal from database: %v", err)
	}
	if len(dbJournal.ScheduleItems) != 2 {
		t.Fatalf("Expected 2 schedule items in database, got %d", len(dbJournal.ScheduleItems))
	}
}

func TestDeleteScheduleItem_NotFound(t *testing.T) {
	service, tmpDir := setupTestService(t)
	defer os.RemoveAll(tmpDir)

	// Create a journal with schedule items
	date := "2024-01-15"
	j := NewJournal(date)

	service.AddScheduleItem(j, "09:00", "Morning standup")

	// Try to delete non-existent item
	err := service.DeleteScheduleItem(j, "nonexistent-id")
	if err == nil {
		t.Error("Expected error for non-existent item, got nil")
	}
	if err.Error() != "schedule item not found: nonexistent-id" {
		t.Errorf("Expected specific error message, got: %v", err)
	}
}

func TestGetByDate_LoadsScheduleItemsFromDatabase(t *testing.T) {
	service, tmpDir := setupTestService(t)
	defer os.RemoveAll(tmpDir)

	// Create a journal with schedule items
	date := "2024-01-15"
	j := NewJournal(date)

	service.AddScheduleItem(j, "09:00", "Morning standup")
	service.AddScheduleItem(j, "10:00", "Team meeting")

	// Load journal again
	loadedJournal, err := service.GetByDate(date)
	if err != nil {
		t.Fatalf("GetByDate failed: %v", err)
	}

	// Verify schedule items were loaded from database
	if len(loadedJournal.ScheduleItems) != 2 {
		t.Fatalf("Expected 2 schedule items, got %d", len(loadedJournal.ScheduleItems))
	}

	if loadedJournal.ScheduleItems[0].Content != "Morning standup" {
		t.Errorf("Expected first item 'Morning standup', got '%s'", loadedJournal.ScheduleItems[0].Content)
	}
	if loadedJournal.ScheduleItems[1].Content != "Team meeting" {
		t.Errorf("Expected second item 'Team meeting', got '%s'", loadedJournal.ScheduleItems[1].Content)
	}
}

func TestScheduleItems_Persistence(t *testing.T) {
	service, tmpDir := setupTestService(t)
	defer os.RemoveAll(tmpDir)

	// Create a journal with schedule items
	date := "2024-01-15"
	j := NewJournal(date)

	// Add items
	service.AddScheduleItem(j, "09:00", "Morning standup")
	service.AddScheduleItem(j, "", "Review PRs")
	service.AddScheduleItem(j, "14:00", "1:1 with manager")

	// Load from database
	dbJournal, err := service.repo.GetJournalByDate(date)
	if err != nil {
		t.Fatalf("Failed to get journal from database: %v", err)
	}

	// Verify all items were persisted
	if len(dbJournal.ScheduleItems) != 3 {
		t.Fatalf("Expected 3 schedule items in database, got %d", len(dbJournal.ScheduleItems))
	}

	// Verify items with times
	if !dbJournal.ScheduleItems[0].Time.IsZero() {
		expectedTime, _ := time.Parse("2006-01-02 15:04", "2024-01-15 09:00")
		if !dbJournal.ScheduleItems[0].Time.Equal(expectedTime) {
			t.Errorf("Expected time %v, got %v", expectedTime, dbJournal.ScheduleItems[0].Time)
		}
	}

	// Verify items without times
	if !dbJournal.ScheduleItems[1].Time.IsZero() {
		t.Errorf("Expected zero time for item without time, got %v", dbJournal.ScheduleItems[1].Time)
	}

	// Verify content
	if dbJournal.ScheduleItems[0].Content != "Morning standup" {
		t.Errorf("Expected content 'Morning standup', got '%s'", dbJournal.ScheduleItems[0].Content)
	}
	if dbJournal.ScheduleItems[1].Content != "Review PRs" {
		t.Errorf("Expected content 'Review PRs', got '%s'", dbJournal.ScheduleItems[1].Content)
	}
	if dbJournal.ScheduleItems[2].Content != "1:1 with manager" {
		t.Errorf("Expected content '1:1 with manager', got '%s'", dbJournal.ScheduleItems[2].Content)
	}
}

func TestScheduleItems_Integration(t *testing.T) {
	service, tmpDir := setupTestService(t)
	defer os.RemoveAll(tmpDir)

	date := "2024-01-15"

	// Test 1: Add schedule items
	j := NewJournal(date)
	service.AddScheduleItem(j, "09:00", "Morning standup")
	service.AddScheduleItem(j, "10:30", "Code review")
	service.AddScheduleItem(j, "", "Lunch")

	// Test 2: Load journal and verify items
	loadedJournal, err := service.GetByDate(date)
	if err != nil {
		t.Fatalf("GetByDate failed: %v", err)
	}
	if len(loadedJournal.ScheduleItems) != 3 {
		t.Fatalf("Expected 3 schedule items after load, got %d", len(loadedJournal.ScheduleItems))
	}

	// Test 3: Delete an item
	itemID := loadedJournal.ScheduleItems[1].ID
	err = service.DeleteScheduleItem(loadedJournal, itemID)
	if err != nil {
		t.Fatalf("DeleteScheduleItem failed: %v", err)
	}

	// Test 4: Load again and verify deletion
	reloadedJournal, err := service.GetByDate(date)
	if err != nil {
		t.Fatalf("GetByDate failed after delete: %v", err)
	}
	if len(reloadedJournal.ScheduleItems) != 2 {
		t.Fatalf("Expected 2 schedule items after delete, got %d", len(reloadedJournal.ScheduleItems))
	}

	// Test 5: Verify correct items remain
	if reloadedJournal.ScheduleItems[0].Content != "Morning standup" {
		t.Errorf("Expected first item 'Morning standup', got '%s'", reloadedJournal.ScheduleItems[0].Content)
	}
	if reloadedJournal.ScheduleItems[1].Content != "Lunch" {
		t.Errorf("Expected second item 'Lunch', got '%s'", reloadedJournal.ScheduleItems[1].Content)
	}

	// Test 6: Add another item
	err = service.AddScheduleItem(reloadedJournal, "15:00", "Sprint planning")
	if err != nil {
		t.Fatalf("AddScheduleItem failed: %v", err)
	}

	// Test 7: Final verification
	finalJournal, err := service.GetByDate(date)
	if err != nil {
		t.Fatalf("GetByDate failed for final check: %v", err)
	}
	if len(finalJournal.ScheduleItems) != 3 {
		t.Fatalf("Expected 3 schedule items in final check, got %d", len(finalJournal.ScheduleItems))
	}
}
