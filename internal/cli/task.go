package cli

import (
	"fmt"
	"strings"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/MikeBiancalana/reckon/internal/storage"
	"github.com/MikeBiancalana/reckon/internal/task"
	"github.com/charmbracelet/huh"
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
	RunE: func(cmd *cobra.Command, args []string) error {
		// Initialize services
		svc, err := initTaskService()
		if err != nil {
			return err
		}

		var t *task.Task
		if len(args) == 0 {
			// Interactive mode
			t, err = runInteractiveTaskForm(svc)
		} else {
			// Non-interactive mode
			title := strings.Join(args, " ")
			t, err = svc.Create(title, taskTagsFlag)
		}
		if err != nil {
			return fmt.Errorf("failed to create task: %w", err)
		}

		fmt.Printf("✓ Created task: %s\n", t.ID)
		fmt.Printf("  Title: %s\n", t.Title)
		if t.Description != "" {
			fmt.Printf("  Description: %s\n", strings.ReplaceAll(t.Description, "\n", "\n    "))
		}
		if len(t.Tags) > 0 {
			fmt.Printf("  Tags: %s\n", strings.Join(t.Tags, ", "))
		}

		return nil
	},
}

// runInteractiveTaskForm runs an interactive form to create a task
func runInteractiveTaskForm(svc *task.Service) (*task.Task, error) {
	var title string
	var description string
	var tags string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Task title").
				Value(&title).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("title is required")
					}
					return nil
				}),
			huh.NewText().
				Title("Description (optional, press Enter on empty line to finish)").
				Value(&description),
			huh.NewInput().
				Title("Tags (optional, comma-separated)").
				Value(&tags),
		),
	)

	err := form.Run()
	if err != nil {
		return nil, fmt.Errorf("form cancelled: %w", err)
	}

	// Parse tags
	var tagList []string
	if tags != "" {
		for _, tag := range strings.Split(tags, ",") {
			trimmed := strings.TrimSpace(tag)
			if trimmed != "" {
				tagList = append(tagList, trimmed)
			}
		}
	}

	return svc.CreateWithDescription(title, description, tagList)
}

// taskListCmd lists tasks
var taskListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tasks",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Initialize services
		svc, err := initTaskService()
		if err != nil {
			return err
		}

		// Parse status filter
		var statusFilter *task.Status
		if taskStatusFlag != "" {
			s := task.Status(taskStatusFlag)
			statusFilter = &s
		}

		// List tasks
		tasks, err := svc.List(statusFilter, taskTagsFlag)
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

		// Initialize services
		svc, err := initTaskService()
		if err != nil {
			return err
		}

		// Get task
		t, err := svc.GetByID(taskID)
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
		taskID := args[0]
		message := strings.Join(args[1:], " ")

		// Initialize services
		svc, err := initTaskService()
		if err != nil {
			return err
		}

		// Append log
		if err := svc.AppendLog(taskID, message); err != nil {
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

		// Initialize services
		svc, err := initTaskService()
		if err != nil {
			return err
		}

		// Update status
		if err := svc.UpdateStatus(taskID, task.StatusDone); err != nil {
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

// initTaskService initializes the task service with all dependencies
func initTaskService() (*task.Service, error) {
	// Get database path
	dbPath, err := config.DatabasePath()
	if err != nil {
		return nil, fmt.Errorf("failed to get database path: %w", err)
	}

	// Open database
	db, err := storage.NewDatabase(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create repositories
	taskRepo := task.NewRepository(db)
	journalRepo := journal.NewRepository(db)

	// Create file store
	fileStore := storage.NewFileStore()

	// Create services
	journalSvc := journal.NewService(journalRepo, fileStore)
	taskSvc := task.NewService(taskRepo, journalSvc)

	return taskSvc, nil
}

// GetTaskCommand returns the task command
func GetTaskCommand() *cobra.Command {
	return taskCmd
}
