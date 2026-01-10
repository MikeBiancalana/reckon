package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/MikeBiancalana/reckon/internal/task"
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
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		title := strings.Join(args, " ")

		// Use global taskService
		if taskService == nil {
			return fmt.Errorf("task service not initialized")
		}

		// Create task
		t, err := taskService.Create(title, taskTagsFlag)
		if err != nil {
			return fmt.Errorf("failed to create task: %w", err)
		}

		fmt.Printf("✓ Created task: %s\n", t.ID)
		fmt.Printf("  Title: %s\n", t.Title)
		if len(t.Tags) > 0 {
			fmt.Printf("  Tags: %s\n", strings.Join(t.Tags, ", "))
		}

		return nil
	},
}

// taskListCmd lists tasks
var taskListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tasks",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Use global taskService
		if taskService == nil {
			return fmt.Errorf("task service not initialized")
		}

		// Parse status filter
		var statusFilter *task.Status
		if taskStatusFlag != "" {
			s := task.Status(taskStatusFlag)
			statusFilter = &s
		}

		// List tasks
		tasks, err := taskService.List(statusFilter, taskTagsFlag)
		if err != nil {
			return fmt.Errorf("failed to list tasks: %w", err)
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
			// Compact output: number status title
			for i, t := range tasks {
				fmt.Printf("%d %s %s\n", i+1, t.Status, t.Title)
			}
			return nil
		}

		// Default verbose output with sequential numbers
		fmt.Printf("Found %d task(s):\n\n", len(tasks))
		for i, t := range tasks {
			fmt.Printf("[%d] [%s] %s\n", i+1, t.Status, t.Title)
			fmt.Printf("  ID: %s\n", t.ID)
			fmt.Printf("  Created: %s\n", t.Created)
			if len(t.Tags) > 0 {
				fmt.Printf("  Tags: %s\n", strings.Join(t.Tags, ", "))
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
		// Use global taskService
		if taskService == nil {
			return fmt.Errorf("task service not initialized")
		}

		// Resolve task ID (supports numeric indices)
		taskID, err := resolveTaskID(args[0], taskService)
		if err != nil {
			return err
		}

		// Get task
		t, err := taskService.GetByID(taskID)
		if err != nil {
			return fmt.Errorf("failed to get task: %w", err)
		}

		// Output task as markdown
		content := task.WriteTask(t)
		fmt.Print(content)

		return nil
	},
}

// taskLogCmd appends a log entry to a task
var taskLogCmd = &cobra.Command{
	Use:   "log [task-id] [message]",
	Short: "Append a log entry to a task",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Use global taskService
		if taskService == nil {
			return fmt.Errorf("task service not initialized")
		}

		// Resolve task ID (supports numeric indices)
		taskID, err := resolveTaskID(args[0], taskService)
		if err != nil {
			return err
		}

		message := strings.Join(args[1:], " ")

		// Append log
		if err := taskService.AppendLog(taskID, message); err != nil {
			return fmt.Errorf("failed to append log: %w", err)
		}

		fmt.Printf("✓ Logged to task %s\n", taskID)
		fmt.Printf("  Also added to today's journal\n")

		return nil
	},
}

// taskDoneCmd marks a task as done
var taskDoneCmd = &cobra.Command{
	Use:   "done [task-id]",
	Short: "Mark a task as done",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Use global taskService
		if taskService == nil {
			return fmt.Errorf("task service not initialized")
		}

		// Resolve task ID (supports numeric indices)
		taskID, err := resolveTaskID(args[0], taskService)
		if err != nil {
			return err
		}

		// Update status
		if err := taskService.UpdateStatus(taskID, task.StatusDone); err != nil {
			return fmt.Errorf("failed to mark task as done: %w", err)
		}

		fmt.Printf("✓ Marked task %s as done\n", taskID)

		return nil
	},
}

// taskEditCmd edits a task's details
var taskEditCmd = &cobra.Command{
	Use:   "edit [task-id]",
	Short: "Edit task details",
	Long:  `Edit task title, description, and tags. Use flags to specify what to edit.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Use global taskService
		if taskService == nil {
			return fmt.Errorf("task service not initialized")
		}

		// Resolve task ID (supports numeric indices)
		taskID, err := resolveTaskID(args[0], taskService)
		if err != nil {
			return err
		}

		// Check if any edit flags were provided
		if taskEditTitleFlag == "" && taskEditDescriptionFlag == "" && len(taskEditTagsFlag) == 0 {
			return fmt.Errorf("no changes specified. Use --title, --description, or --tags flags")
		}

		// Prepare update parameters
		var title *string
		var description *string
		var tags []string

		if taskEditTitleFlag != "" {
			title = &taskEditTitleFlag
		}
		if taskEditDescriptionFlag != "" {
			description = &taskEditDescriptionFlag
		}
		if len(taskEditTagsFlag) > 0 {
			tags = taskEditTagsFlag
		}

		// Update task
		if err := taskService.Update(taskID, title, description, tags); err != nil {
			return fmt.Errorf("failed to update task: %w", err)
		}

		fmt.Printf("✓ Updated task %s\n", taskID)
		return nil
	},
}

// taskNoteCmd adds a note to a task
var taskNoteCmd = &cobra.Command{
	Use:   "note [task-id] [note-text]",
	Short: "Add a note to a task",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Use global taskService
		if taskService == nil {
			return fmt.Errorf("task service not initialized")
		}

		// Resolve task ID (supports numeric indices)
		taskID, err := resolveTaskID(args[0], taskService)
		if err != nil {
			return err
		}

		noteText := strings.Join(args[1:], " ")

		// Add note (using AppendLog which adds to task's log)
		if err := taskService.AppendLog(taskID, noteText); err != nil {
			return fmt.Errorf("failed to add note: %w", err)
		}

		fmt.Printf("✓ Added note to task %s\n", taskID)
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
	taskCmd.AddCommand(taskNoteCmd)

	// Flags
	taskNewCmd.Flags().StringSliceVar(&taskTagsFlag, "tags", []string{}, "Task tags (comma-separated)")
	taskListCmd.Flags().StringVar(&taskStatusFlag, "status", "", "Filter by status (active, done, waiting, someday)")
	taskListCmd.Flags().StringSliceVar(&taskTagsFlag, "tag", []string{}, "Filter by tags")
	taskListCmd.Flags().BoolVar(&taskCompactFlag, "compact", false, "Show compact output")
	taskListCmd.Flags().BoolVar(&taskJsonFlag, "json", false, "Output as JSON")
	taskEditCmd.Flags().StringVar(&taskEditTitleFlag, "title", "", "New task title")
	taskEditCmd.Flags().StringVarP(&taskEditDescriptionFlag, "description", "d", "", "New task description")
	taskEditCmd.Flags().StringSliceVar(&taskEditTagsFlag, "tags", []string{}, "New task tags (comma-separated)")
}

// resolveTaskID resolves a task identifier (numeric index or string ID) to a task ID
func resolveTaskID(identifier string, svc *task.Service) (string, error) {
	// Try to parse as number (1-based index)
	if index, err := strconv.Atoi(identifier); err == nil && index > 0 {
		// Get all tasks (unfiltered) to find by index
		tasks, err := svc.List(nil, nil)
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
