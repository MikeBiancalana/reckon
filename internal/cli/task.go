package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/MikeBiancalana/reckon/internal/task"
	"github.com/spf13/cobra"
)

var (
	taskStatusFlag string
	taskTagsFlag   []string
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

		if len(tasks) == 0 {
			fmt.Println("No tasks found")
			return nil
		}

		fmt.Printf("Found %d task(s):\n\n", len(tasks))
		for _, t := range tasks {
			fmt.Printf("[%s] %s\n", t.Status, t.Title)
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
		taskID := args[0]

		// Use global taskService
		if taskService == nil {
			return fmt.Errorf("task service not initialized")
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
	Use:   "log [task-id] [message | -]",
	Short: "Append a log entry to a task",
	Long:  `Append a log entry to a task. Use - to read message from stdin, or provide message as arguments. Runs interactively if only task-id given.`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID := args[0]
		var message string

		if len(args) == 1 {
			// Interactive mode: read message from stdin
			fmt.Fprintf(os.Stderr, "Enter task log message (Ctrl+D to finish):\n")
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("failed to read from stdin: %w", err)
			}
			message = strings.TrimSpace(string(data))
			if message == "" {
				return fmt.Errorf("no message provided")
			}
		} else if len(args) == 2 && args[1] == "-" {
			// Read from stdin
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("failed to read from stdin: %w", err)
			}
			message = strings.TrimSpace(string(data))
			if message == "" {
				return fmt.Errorf("no message provided via stdin")
			}
		} else {
			message = strings.Join(args[1:], " ")
		}

		// Use global taskService
		if taskService == nil {
			return fmt.Errorf("task service not initialized")
		}

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
		taskID := args[0]

		// Use global taskService
		if taskService == nil {
			return fmt.Errorf("task service not initialized")
		}

		// Update status
		if err := taskService.UpdateStatus(taskID, task.StatusDone); err != nil {
			return fmt.Errorf("failed to mark task as done: %w", err)
		}

		fmt.Printf("✓ Marked task %s as done\n", taskID)

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

	// Flags
	taskNewCmd.Flags().StringSliceVar(&taskTagsFlag, "tags", []string{}, "Task tags (comma-separated)")
	taskListCmd.Flags().StringVar(&taskStatusFlag, "status", "", "Filter by status (active, done, waiting, someday)")
	taskListCmd.Flags().StringSliceVar(&taskTagsFlag, "tag", []string{}, "Filter by tags")
}

// GetTaskCommand returns the task command
func GetTaskCommand() *cobra.Command {
	return taskCmd
}
