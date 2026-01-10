package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/spf13/cobra"
)

var (
	taskStatusFlag          string
	taskTagsFlag            []string
	taskCompactFlag         bool
	taskJsonFlag            bool
	taskEditTitleFlag       string
	taskEditDescriptionFlag string
	taskEditTagsFlag        []string
)

// taskCmd represents the task command
var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Manage multi-day tasks",
	Long:  "Create and manage tasks that span multiple days with their own log history.",
}

// taskNewCmd creates a new task
var taskNewCmd = &cobra.Command{
	Use:   "new [title]",
	Short: "Create a new task",
	RunE: func(cmd *cobra.Command, args []string) error {
		title := strings.Join(args, " ")

		// Use global journalTaskService
		if journalTaskService == nil {
			return fmt.Errorf("task service not initialized")
		}

		if title == "" {
			return fmt.Errorf("task title is required")
		}

		// Create task
		err := journalTaskService.AddTask(title)
		if err != nil {
			return fmt.Errorf("failed to create task: %w", err)
		}

		fmt.Printf("✓ Created task: %s\n", title)
		if len(taskTagsFlag) > 0 {
			fmt.Printf("  Note: Tags are not yet supported in the unified task system\n")
		}

		return nil
	},
}

// taskListCmd lists tasks
var taskListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tasks",
	Long:  "List tasks. Supported status filters: open, active, done",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Use global journalTaskService
		if journalTaskService == nil {
			return fmt.Errorf("task service not initialized")
		}

		// Validate status filter if provided
		if taskStatusFlag != "" {
			validStatuses := map[string]bool{"open": true, "active": true, "done": true}
			if !validStatuses[strings.ToLower(taskStatusFlag)] {
				return fmt.Errorf("unsupported status value: %s (supported: open, active, done)", taskStatusFlag)
			}
		}

		// List all tasks
		tasks, err := journalTaskService.GetAllTasks()
		if err != nil {
			return fmt.Errorf("failed to list tasks: %w", err)
		}

		// Apply status filter if provided
		if taskStatusFlag != "" {
			filtered := make([]journal.Task, 0)
			for _, t := range tasks {
				statusMatch := false
				switch strings.ToLower(taskStatusFlag) {
				case "open", "active":
					statusMatch = t.Status == journal.TaskOpen
				case "done":
					statusMatch = t.Status == journal.TaskDone
				}
				if statusMatch {
					filtered = append(filtered, t)
				}
			}
			tasks = filtered
		}

		// Note: Tag filtering not yet supported in journal task system
		if len(taskTagsFlag) > 0 {
			fmt.Println("Note: Tag filtering is not yet supported in the unified task system")
		}

		if taskJsonFlag {
			// JSON output
			if err := json.NewEncoder(os.Stdout).Encode(tasks); err != nil {
				return fmt.Errorf("failed to encode tasks as JSON: %w", err)
			}
			return nil
		}

		if len(tasks) == 0 {
			fmt.Println("No tasks found")
			return nil
		}

		if taskCompactFlag {
			// Compact output: number status text
			for i, t := range tasks {
				fmt.Printf("%d %s %s\n", i+1, t.Status, t.Text)
			}
			return nil
		}

		// Default verbose output with sequential numbers
		fmt.Printf("Found %d task(s):\n\n", len(tasks))
		for i, t := range tasks {
			fmt.Printf("[%d] [%s] %s\n", i+1, t.Status, t.Text)
			fmt.Printf("  ID: %s\n", t.ID)
			fmt.Printf("  Created: %s\n", t.CreatedAt.Format("2006-01-02"))
			if len(t.Notes) > 0 {
				fmt.Printf("  Notes: %d\n", len(t.Notes))
			}
			fmt.Println()
		}

		return nil
	},
}

// taskShowCmd shows a task
var taskShowCmd = &cobra.Command{
	Use:   "show [task-id]",
	Short: "Show task details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Use global journalTaskService
		if journalTaskService == nil {
			return fmt.Errorf("task service not initialized")
		}

		// Resolve task ID (supports numeric indices)
		taskID, err := resolveJournalTaskID(args[0], journalTaskService)
		if err != nil {
			return err
		}

		// Get all tasks and find the one we want
		tasks, err := journalTaskService.GetAllTasks()
		if err != nil {
			return fmt.Errorf("failed to get tasks: %w", err)
		}

		var foundTask *journal.Task
		for _, t := range tasks {
			if t.ID == taskID {
				foundTask = &t
				break
			}
		}

		if foundTask == nil {
			return fmt.Errorf("task not found: %s", taskID)
		}

		// Output task details
		fmt.Printf("Task: %s\n", foundTask.Text)
		fmt.Printf("ID: %s\n", foundTask.ID)
		fmt.Printf("Status: %s\n", foundTask.Status)
		fmt.Printf("Created: %s\n", foundTask.CreatedAt.Format("2006-01-02"))
		if len(foundTask.Notes) > 0 {
			fmt.Printf("\nNotes:\n")
			for _, note := range foundTask.Notes {
				fmt.Printf("  - %s\n", note.Text)
			}
		}

		return nil
	},
}

// taskLogCmd appends a log entry to a task (now adds as a note)
var taskLogCmd = &cobra.Command{
	Use:   "log [task-id] [message]",
	Short: "Append a note to a task",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Use global journalTaskService
		if journalTaskService == nil {
			return fmt.Errorf("task service not initialized")
		}

		// Resolve task ID (supports numeric indices)
		taskID, err := resolveJournalTaskID(args[0], journalTaskService)
		if err != nil {
			return err
		}

		message := strings.Join(args[1:], " ")

		// Add note to task
		if err := journalTaskService.AddTaskNote(taskID, message); err != nil {
			return fmt.Errorf("failed to add note: %w", err)
		}

		fmt.Printf("✓ Added note to task %s\n", taskID)

		return nil
	},
}

// taskDoneCmd marks a task as done
var taskDoneCmd = &cobra.Command{
	Use:   "done [task-id]",
	Short: "Mark a task as done (toggle status)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Use global journalTaskService
		if journalTaskService == nil {
			return fmt.Errorf("task service not initialized")
		}

		// Resolve task ID (supports numeric indices)
		taskID, err := resolveJournalTaskID(args[0], journalTaskService)
		if err != nil {
			return err
		}

		// Toggle task status
		if err := journalTaskService.ToggleTask(taskID); err != nil {
			return fmt.Errorf("failed to toggle task: %w", err)
		}

		fmt.Printf("✓ Toggled task %s status\n", taskID)

		return nil
	},
}

// taskEditCmd edits a task's details
var taskEditCmd = &cobra.Command{
	Use:   "edit [task-id]",
	Short: "Edit task details",
	Long:  `Edit task title using --title flag.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Use global journalTaskService
		if journalTaskService == nil {
			return fmt.Errorf("task service not initialized")
		}

		// Check if any flags were provided
		if taskEditTitleFlag == "" {
			return fmt.Errorf("no edit flags provided. Use --title to update the task title")
		}

		// Resolve task ID (supports numeric indices)
		taskID, err := resolveJournalTaskID(args[0], journalTaskService)
		if err != nil {
			return err
		}

		// Update the task
		if err := journalTaskService.UpdateTask(taskID, taskEditTitleFlag, nil); err != nil {
			return fmt.Errorf("failed to update task: %w", err)
		}

		fmt.Printf("✓ Updated task %s\n", taskID)
		if taskEditTitleFlag != "" {
			fmt.Printf("  New title: %s\n", taskEditTitleFlag)
		}
		if len(taskEditTagsFlag) > 0 {
			fmt.Println("  Note: Tags are not yet fully supported in the unified task system")
		}

		return nil
	},
}

func init() {
	// Add subcommands
	taskCmd.AddCommand(taskNewCmd)
	taskCmd.AddCommand(taskListCmd)
	taskCmd.AddCommand(taskShowCmd)
	taskCmd.AddCommand(taskLogCmd)
	taskCmd.AddCommand(taskDoneCmd)
	taskCmd.AddCommand(taskEditCmd)

	// Flags
	taskNewCmd.Flags().StringSliceVar(&taskTagsFlag, "tags", []string{}, "Task tags (comma-separated)")
	taskListCmd.Flags().StringVar(&taskStatusFlag, "status", "", "Filter by status (open, active, done)")
	taskListCmd.Flags().StringSliceVar(&taskTagsFlag, "tag", []string{}, "Filter by tags")
	taskListCmd.Flags().BoolVar(&taskCompactFlag, "compact", false, "Show compact output")
	taskListCmd.Flags().BoolVar(&taskJsonFlag, "json", false, "Output as JSON")
	taskEditCmd.Flags().StringVar(&taskEditTitleFlag, "title", "", "New task title")
	taskEditCmd.Flags().StringVar(&taskEditDescriptionFlag, "description", "", "New task description")
	taskEditCmd.Flags().StringSliceVar(&taskEditTagsFlag, "tags", []string{}, "New task tags (comma-separated)")
}

// resolveJournalTaskID resolves a task identifier (numeric index or string ID) to a task ID
func resolveJournalTaskID(identifier string, svc *journal.TaskService) (string, error) {
	// Try to parse as number (1-based index)
	if index, err := strconv.Atoi(identifier); err == nil && index > 0 {
		// Get all tasks (unfiltered) to find by index
		tasks, err := svc.GetAllTasks()
		if err != nil {
			return "", fmt.Errorf("failed to list tasks: %w", err)
		}
		if index > len(tasks) {
			return "", fmt.Errorf("task index %d out of range (found %d tasks)", index, len(tasks))
		}
		return tasks[index-1].ID, nil
	}
	// Otherwise, treat as direct task ID
	return identifier, nil
}

// GetTaskCommand returns the task command
func GetTaskCommand() *cobra.Command {
	return taskCmd
}
