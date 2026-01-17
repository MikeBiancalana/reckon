package journal_test

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/MikeBiancalana/reckon/internal/storage"
)

// This example demonstrates how to use TaskService to manage tasks
func Example_taskService() {
	// Set up temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "task-service-example-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Configure to use temp directory
	os.Setenv("RECKON_DATA_DIR", tmpDir)
	defer os.Unsetenv("RECKON_DATA_DIR")

	// Create database
	db, err := storage.NewDatabase(filepath.Join(tmpDir, "reckon.db"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Create repository and file store
	repo := journal.NewTaskRepository(db, nil)
	store := storage.NewFileStore()

	// Create service
	service := journal.NewTaskService(repo, store, nil)

	// Add some tasks
	if err := service.AddTask("Complete project documentation", []string{}); err != nil {
		log.Fatal(err)
	}

	if err := service.AddTask("Review pull requests", []string{}); err != nil {
		log.Fatal(err)
	}

	// Get all tasks
	tasks, err := service.GetAllTasks()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Created %d tasks\n", len(tasks))

	// Add a note to the first task
	if err := service.AddTaskNote(tasks[0].ID, "Focus on API documentation"); err != nil {
		log.Fatal(err)
	}

	// Toggle the second task to done
	if err := service.ToggleTask(tasks[1].ID); err != nil {
		log.Fatal(err)
	}

	// Get updated tasks
	tasks, err = service.GetAllTasks()
	if err != nil {
		log.Fatal(err)
	}

	// Display tasks
	for _, task := range tasks {
		status := "[ ]"
		if task.Status == journal.TaskDone {
			status = "[x]"
		}
		fmt.Printf("%s %s\n", status, task.Text)
		for _, note := range task.Notes {
			fmt.Printf("  - %s\n", note.Text)
		}
	}

	// Output:
	// Created 2 tasks
	// [ ] Complete project documentation
	//   - Focus on API documentation
	// [x] Review pull requests
}
