package journal

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/MikeBiancalana/reckon/internal/storage"
)

// Example demonstrates the schedule items functionality
func Example_scheduleItems() {
	// Set up a temporary environment
	tmpDir, _ := os.MkdirTemp("", "reckon-example-*")
	defer os.RemoveAll(tmpDir)

	os.Setenv("RECKON_DATA_DIR", tmpDir)
	defer os.Unsetenv("RECKON_DATA_DIR")

	// Create database and service
	dbPath := filepath.Join(tmpDir, "example.db")
	db, _ := storage.NewDatabase(dbPath)
	fileStore := storage.NewFileStore()
	repo := NewRepository(db)
	service := NewService(repo, fileStore)

	// Create a journal for today
	date := time.Now().Format("2006-01-02")
	journal := NewJournal(date)

	// Add schedule items
	service.AddScheduleItem(journal, "09:00", "Morning standup")
	service.AddScheduleItem(journal, "10:30", "Code review session")
	service.AddScheduleItem(journal, "", "Review PRs")
	service.AddScheduleItem(journal, "14:00", "1:1 with manager")

	// Display schedule items
	fmt.Println("Schedule Items:")
	for _, item := range journal.ScheduleItems {
		if !item.Time.IsZero() {
			fmt.Printf("- %s %s\n", item.Time.Format("15:04"), item.Content)
		} else {
			fmt.Printf("- %s\n", item.Content)
		}
	}

	// Delete the code review item
	itemID := journal.ScheduleItems[1].ID
	service.DeleteScheduleItem(journal, itemID)

	fmt.Println("\nAfter deleting code review:")
	for _, item := range journal.ScheduleItems {
		if !item.Time.IsZero() {
			fmt.Printf("- %s %s\n", item.Time.Format("15:04"), item.Content)
		} else {
			fmt.Printf("- %s\n", item.Content)
		}
	}

	// Load from database
	loadedJournal, _ := service.GetByDate(date)
	fmt.Printf("\nSchedule items loaded from database: %d\n", len(loadedJournal.ScheduleItems))

	// Output:
	// Schedule Items:
	// - 09:00 Morning standup
	// - 10:30 Code review session
	// - Review PRs
	// - 14:00 1:1 with manager
	//
	// After deleting code review:
	// - 09:00 Morning standup
	// - Review PRs
	// - 14:00 1:1 with manager
	//
	// Schedule items loaded from database: 3
}
