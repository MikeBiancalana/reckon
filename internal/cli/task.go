package cli

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/sahilm/fuzzy"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/MikeBiancalana/reckon/internal/journal"
)

const (
	idWidth       = 8
	statusWidth   = 10
	createdWidth  = 10
	tagsWidth     = 15
	minTitleWidth = 20
)

var (
	taskStatusFlag          string
	taskTagsFlag            []string
	taskCompactFlag         bool
	taskVerboseFlag         bool
	taskFormatFlag          string
	taskEditTitleFlag       string
	taskEditDescriptionFlag string
	taskEditTagsFlag        []string
	taskMatchFlag           string
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
		err := journalTaskService.AddTask(title, taskTagsFlag)
		if err != nil {
			return fmt.Errorf("failed to create task: %w", err)
		}

		fmt.Printf("✓ Created task: %s\n", title)
		if len(taskTagsFlag) > 0 {
			fmt.Printf("  Tags: %s\n", strings.Join(taskTagsFlag, ", "))
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

		// Validate mutually exclusive flags
		if taskCompactFlag && taskVerboseFlag {
			return fmt.Errorf("cannot use both --compact and --verbose flags")
		}

		// List all tasks
		tasks, err := journalTaskService.GetAllTasks()
		if err != nil {
			return fmt.Errorf("failed to list tasks: %w", err)
		}

		// Apply status filter if provided
		if taskStatusFlag != "" {
			statusLower := strings.ToLower(taskStatusFlag)
			// Validate status value
			switch statusLower {
			case "open", "active", "done":
				// Valid status, continue with filtering
			default:
				return fmt.Errorf("unsupported status value: %s (supported: open, active, done)", taskStatusFlag)
			}

			filtered := make([]journal.Task, 0)
			for _, t := range tasks {
				statusMatch := false
				switch statusLower {
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

		// Filter by tags if specified
		if len(taskTagsFlag) > 0 {
			filtered := make([]journal.Task, 0)
			for _, t := range tasks {
				// Check if task has all specified tags (AND logic)
				hasAllTags := true
				for _, filterTag := range taskTagsFlag {
					found := false
					for _, taskTag := range t.Tags {
						if strings.EqualFold(taskTag, filterTag) {
							found = true
							break
						}
					}
					if !found {
						hasAllTags = false
						break
					}
				}
				if hasAllTags {
					filtered = append(filtered, t)
				}
			}
			tasks = filtered
		}

		if taskFormatFlag != "" {
			format, err := parseFormat(taskFormatFlag)
			if err != nil {
				return err
			}
			switch format {
			case FormatJSON:
				return formatTasksJSON(tasks)
			case FormatTSV:
				return formatTasksTSV(tasks)
			case FormatCSV:
				return formatTasksCSV(tasks)
			}
			return nil
		}

		if len(tasks) == 0 {
			fmt.Println("No tasks found")
			return nil
		}

		if taskVerboseFlag {
			// Verbose output with sequential numbers
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
		}

		if taskCompactFlag {
			// Compact output: number status text
			for i, t := range tasks {
				fmt.Printf("%d %s %s\n", i+1, t.Status, t.Text)
			}
			return nil
		}

		if taskVerboseFlag {
			// Verbose output with sequential numbers
			fmt.Printf("Found %d task(s):\n\n", len(tasks))
			for i, t := range tasks {
				fmt.Printf("[%d] [%s] %s\n", i+1, t.Status, t.Text)
				fmt.Printf("  ID: %s\n", t.ID)
				fmt.Printf("  Created: %s\n", t.CreatedAt.Format("2006-01-02"))
				if len(t.Tags) > 0 {
					fmt.Printf("  Tags: %s\n", strings.Join(t.Tags, ", "))
				}
				if len(t.Notes) > 0 {
					fmt.Printf("  Notes: %d\n", len(t.Notes))
				}
				fmt.Println()
			}
			return nil
		}

		// Default tabular output
		// Get terminal width for dynamic title truncation
		width, _, err := term.GetSize(int(os.Stdout.Fd()))
		if err != nil {
			width = 80 // fallback
		}
		availableTitleWidth := width - (idWidth + statusWidth + createdWidth + tagsWidth + 20) // estimate padding
		if availableTitleWidth < minTitleWidth {
			availableTitleWidth = minTitleWidth
		}

		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', tabwriter.TabIndent)
		fmt.Fprintln(tw, "ID\tSTATUS\tCREATED\tTAGS\tTITLE")
		for _, t := range tasks {
			tags := "-"
			if len(t.Tags) > 0 {
				tags = strings.Join(t.Tags, ", ")
			}
			title := t.Text
			// Truncate title based on available width
			if len(title) > availableTitleWidth {
				title = title[:availableTitleWidth-3] + "..."
			}
			fmt.Fprintf(tw, "%.8s\t%s\t%s\t%s\t%s\n", t.ID, t.Status, t.CreatedAt.Format("2006-01-02"), tags, title)
		}
		tw.Flush()

		return nil
	},
}

// taskShowCmd shows a task
var taskShowCmd = &cobra.Command{
	Use:   "show [task-id|--match <pattern>]",
	Short: "Show task details",
	Long: `Show task details.

Use task index (1, 2, 3...) or exact task ID.
Or use --match to fuzzy-match by task title.

Examples:
  rk task show 1
  rk task show abc123
  rk task show --match auth`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tmpMatch := taskMatchFlag
		taskMatchFlag = ""

		// Use global journalTaskService
		if journalTaskService == nil {
			return fmt.Errorf("task service not initialized")
		}

		// Validate mutually exclusive options
		if tmpMatch != "" && len(args) > 0 {
			return fmt.Errorf("cannot use both task-id and --match; use one or the other")
		}
		if tmpMatch == "" && len(args) == 0 {
			return fmt.Errorf("missing task identifier; use task index, ID, or --match <pattern>")
		}

		// Resolve task ID (supports numeric indices, IDs, or --match for fuzzy matching)
		taskID, err := resolveJournalTaskID(args[0], tmpMatch, journalTaskService)
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
	Use:   "log [task-id|--match <pattern>] [message]",
	Short: "Append a note to a task",
	Long: `Append a note to a task.

Use task index (1, 2, 3...) or exact task ID.
Or use --match to fuzzy-match by task title.

Examples:
  rk task log 1 "Started working on this"
  rk task log abc123 "Progress update"
  rk task log --match auth "Authentication implemented"`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		tmpMatch := taskMatchFlag
		taskMatchFlag = ""

		// Use global journalTaskService
		if journalTaskService == nil {
			return fmt.Errorf("task service not initialized")
		}

		// Validate mutually exclusive options
		if tmpMatch != "" && len(args) > 0 {
			return fmt.Errorf("cannot use both task-id and --match; use one or the other")
		}
		if tmpMatch == "" && len(args) == 0 {
			return fmt.Errorf("missing task identifier; use task index, ID, or --match <pattern>")
		}

		// Resolve task ID (supports numeric indices, IDs, or --match for fuzzy matching)
		taskID, err := resolveJournalTaskID(args[0], tmpMatch, journalTaskService)
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
	Use:   "done [task-id|--match <pattern>]",
	Short: "Mark a task as done (toggle status)",
	Long: `Mark a task as done (toggle status).

Use task index (1, 2, 3...) or exact task ID.
Or use --match to fuzzy-match by task title.
Or use --stdin to read task IDs from stdin (one per line).

Examples:
  rk task done 1
  rk task done abc123
  rk task done --match auth
  echo -e "abc123\ndef456" | rk task done --stdin`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tmpMatch := taskMatchFlag
		taskMatchFlag = ""

		stdinFlag, _ := cmd.Flags().GetBool("stdin")

		// Use global journalTaskService
		if journalTaskService == nil {
			return fmt.Errorf("task service not initialized")
		}

		// Validate mutually exclusive options
		if tmpMatch != "" && len(args) > 0 {
			return fmt.Errorf("cannot use both task-id and --match; use one or the other")
		}
		if tmpMatch != "" && len(args) == 0 && !stdinFlag {
			return fmt.Errorf("missing task identifier; use task index, ID, --match <pattern>, or --stdin")
		}
		if tmpMatch == "" && len(args) == 0 && !stdinFlag {
			return fmt.Errorf("missing task identifier; use task index, ID, --match <pattern>, or --stdin")
		}

		// Handle stdin input
		if stdinFlag {
			ids, err := readStdinIDs()
			if err != nil {
				return err
			}
			// Process each ID
			for _, taskID := range ids {
				if err := journalTaskService.ToggleTask(taskID); err != nil {
					return fmt.Errorf("failed to toggle task %s: %w", taskID, err)
				}
				fmt.Printf("✓ Toggled task %s status\n", taskID)
			}
			return nil
		}

		// Resolve task ID (supports numeric indices, IDs, or --match for fuzzy matching)
		taskID, err := resolveJournalTaskID(args[0], tmpMatch, journalTaskService)
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
	Use:   "edit [task-id|--match <pattern>]",
	Short: "Edit task details",
	Long: `Edit a task's title using the --title flag. Tags support will be added in a future update.

Use task index (1, 2, 3...) or exact task ID.
Or use --match to fuzzy-match by task title.
Or use --stdin to read task IDs from stdin (one per line).

Examples:
  rk task edit 1 --title "New Title"
  rk task edit abc123 --title "New Title"
  rk task edit --match auth --title "Authentication Feature"
  echo -e "abc123\ndef456" | rk task edit --stdin --title "New Title"`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tmpMatch := taskMatchFlag
		taskMatchFlag = ""

		stdinFlag, _ := cmd.Flags().GetBool("stdin")

		// Use global journalTaskService
		if journalTaskService == nil {
			return fmt.Errorf("task service not initialized")
		}

		// Validate mutually exclusive options
		if tmpMatch != "" && len(args) > 0 {
			return fmt.Errorf("cannot use both task-id and --match; use one or the other")
		}
		if tmpMatch == "" && len(args) == 0 && !stdinFlag {
			return fmt.Errorf("missing task identifier; use task index, ID, --match <pattern>, or --stdin")
		}

		// Handle stdin input
		if stdinFlag {
			ids, err := readStdinIDs()
			if err != nil {
				return err
			}
			// Process each ID
			for _, taskID := range ids {
				if err := journalTaskService.UpdateTask(taskID, taskEditTitleFlag, taskEditTagsFlag); err != nil {
					return fmt.Errorf("failed to update task %s: %w", taskID, err)
				}
				fmt.Printf("✓ Updated task %s\n", taskID)
			}
			return nil
		}

		// Check if any flags were provided
		if taskEditTitleFlag == "" && len(taskEditTagsFlag) == 0 {
			return fmt.Errorf("no changes specified. Use --title to update the task title")
		}

		// Resolve task ID (supports numeric indices, IDs, or --match for fuzzy matching)
		taskID, err := resolveJournalTaskID(args[0], tmpMatch, journalTaskService)
		if err != nil {
			return err
		}

		// Update task
		if err := journalTaskService.UpdateTask(taskID, taskEditTitleFlag, taskEditTagsFlag); err != nil {
			return fmt.Errorf("failed to update task: %w", err)
		}

		fmt.Printf("✓ Updated task %s\n", taskID)
		if taskEditTitleFlag != "" {
			fmt.Printf("  New title: %s\n", taskEditTitleFlag)
		}
		if len(taskEditTagsFlag) > 0 {
			fmt.Printf("  Note: Tags are not yet fully supported in the unified task system\n")
		}

		return nil
	},
}

// taskNoteCmd adds a note to a task
var taskNoteCmd = &cobra.Command{
	Use:   "note [task-id|--match <pattern>] [note-text]",
	Short: "Add a note to a task",
	Long: `Add a note to a task.

Use task index (1, 2, 3...) or exact task ID.
Or use --match to fuzzy-match by task title.

Examples:
  rk task note 1 "Some note"
  rk task note abc123 "Another note"
  rk task note --match auth "Auth-related note"`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		tmpMatch := taskMatchFlag
		taskMatchFlag = ""

		// Use global journalTaskService
		if journalTaskService == nil {
			return fmt.Errorf("task service not initialized")
		}

		// Validate mutually exclusive options
		if tmpMatch != "" && len(args) > 0 {
			return fmt.Errorf("cannot use both task-id and --match; use one or the other")
		}
		if tmpMatch == "" && len(args) == 0 {
			return fmt.Errorf("missing task identifier; use task index, ID, or --match <pattern>")
		}

		// Resolve task ID (supports numeric indices, IDs, or --match for fuzzy matching)
		taskID, err := resolveJournalTaskID(args[0], tmpMatch, journalTaskService)
		if err != nil {
			return err
		}

		noteText := strings.Join(args[1:], " ")

		// Add note (using AddTaskNote which adds to task's log)
		if err := journalTaskService.AddTaskNote(taskID, noteText); err != nil {
			return fmt.Errorf("failed to add note: %w", err)
		}

		fmt.Printf("✓ Added note to task %s\n", taskID)
		return nil
	},
}

// taskDeleteCmd deletes a task
var taskDeleteCmd = &cobra.Command{
	Use:   "delete [task-id|--match <pattern>]",
	Short: "Delete a task",
	Long: `Delete a task.

Use task index (1, 2, 3...) or exact task ID.
Or use --match to fuzzy-match by task title.
Or use --stdin to read task IDs from stdin (one per line).

Examples:
  rk task delete 1
  rk task delete abc123
  rk task delete --match auth
  echo -e "abc123\ndef456" | rk task delete --stdin`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tmpMatch := taskMatchFlag
		taskMatchFlag = ""

		stdinFlag, _ := cmd.Flags().GetBool("stdin")

		// Use global journalTaskService
		if journalTaskService == nil {
			return fmt.Errorf("task service not initialized")
		}

		// Validate mutually exclusive options
		if tmpMatch != "" && len(args) > 0 {
			return fmt.Errorf("cannot use both task-id and --match; use one or the other")
		}
		if tmpMatch == "" && len(args) == 0 && !stdinFlag {
			return fmt.Errorf("missing task identifier; use task index, ID, --match <pattern>, or --stdin")
		}

		// Handle stdin input (skip confirmation for batch deletes)
		if stdinFlag {
			ids, err := readStdinIDs()
			if err != nil {
				return err
			}
			// Process each ID
			for _, taskID := range ids {
				if err := journalTaskService.DeleteTask(taskID); err != nil {
					return fmt.Errorf("failed to delete task %s: %w", taskID, err)
				}
				fmt.Printf("✓ Deleted task %s\n", taskID)
			}
			return nil
		}

		// Resolve task ID (supports numeric indices, IDs, or --match for fuzzy matching)
		taskID, err := resolveJournalTaskID(args[0], tmpMatch, journalTaskService)
		if err != nil {
			return err
		}

		// Get task details for confirmation
		tasks, err := journalTaskService.GetAllTasks()
		if err != nil {
			return fmt.Errorf("failed to get tasks: %w", err)
		}

		var taskToDelete *journal.Task
		for _, t := range tasks {
			if t.ID == taskID {
				taskToDelete = &t
				break
			}
		}

		if taskToDelete == nil {
			return fmt.Errorf("task not found: %s", taskID)
		}

		// Confirmation prompt
		fmt.Printf("Delete task '%s'? (y/n): ", taskToDelete.Text)
		var response string
		fmt.Scanln(&response)
		response = strings.ToLower(strings.TrimSpace(response))
		if response != "y" && response != "yes" {
			fmt.Println("Deletion cancelled.")
			return nil
		}

		// Delete task
		if err := journalTaskService.DeleteTask(taskID); err != nil {
			return fmt.Errorf("failed to delete task: %w", err)
		}

		fmt.Printf("✓ Deleted task %s\n", taskID)

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
	taskCmd.AddCommand(taskDeleteCmd)

	// Flags
	taskNewCmd.Flags().StringSliceVar(&taskTagsFlag, "tags", []string{}, "Task tags (comma-separated)")
	taskListCmd.Flags().StringVar(&taskStatusFlag, "status", "", "Filter by status (open, active, done)")
	taskListCmd.Flags().StringSliceVar(&taskTagsFlag, "tag", []string{}, "Filter by tags")
	taskListCmd.Flags().BoolVar(&taskCompactFlag, "compact", false, "Show compact single-line output")
	taskListCmd.Flags().BoolVarP(&taskVerboseFlag, "verbose", "v", false, "Show verbose multi-line output")
	taskListCmd.Flags().StringVar(&taskFormatFlag, "format", "", "Output format (json, tsv, csv)")
	taskEditCmd.Flags().StringVar(&taskEditTitleFlag, "title", "", "New task title")
	taskEditCmd.Flags().StringVarP(&taskEditDescriptionFlag, "description", "d", "", "New task description")
	taskEditCmd.Flags().StringSliceVar(&taskEditTagsFlag, "tags", []string{}, "New task tags (comma-separated)")

	// Commands that support --match flag for fuzzy title matching
	taskShowCmd.Flags().StringVar(&taskMatchFlag, "match", "", "Fuzzy match task by title (alternative to task-id)")
	taskLogCmd.Flags().StringVar(&taskMatchFlag, "match", "", "Fuzzy match task by title (alternative to task-id)")
	taskDoneCmd.Flags().StringVar(&taskMatchFlag, "match", "", "Fuzzy match task by title (alternative to task-id)")
	taskEditCmd.Flags().StringVar(&taskMatchFlag, "match", "", "Fuzzy match task by title (alternative to task-id)")
	taskNoteCmd.Flags().StringVar(&taskMatchFlag, "match", "", "Fuzzy match task by title (alternative to task-id)")
	taskDeleteCmd.Flags().StringVar(&taskMatchFlag, "match", "", "Fuzzy match task by title (alternative to task-id)")

	// Commands that support --stdin flag for batch operations
	taskDoneCmd.Flags().Bool("stdin", false, "Read task IDs from stdin (one per line)")
	taskEditCmd.Flags().Bool("stdin", false, "Read task IDs from stdin (one per line)")
	taskDeleteCmd.Flags().Bool("stdin", false, "Read task IDs from stdin (one per line)")
}

// resolveJournalTaskID resolves a task identifier (numeric index, exact ID, or fuzzy-matched title) to a task ID
func resolveJournalTaskID(identifier string, matchPattern string, svc *journal.TaskService) (string, error) {
	if matchPattern != "" {
		return resolveJournalTaskIDByMatch(matchPattern, svc)
	}
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

// resolveJournalTaskIDByMatch finds a task by fuzzy-matching its title against the given pattern
func resolveJournalTaskIDByMatch(pattern string, svc *journal.TaskService) (string, error) {
	tasks, err := svc.GetAllTasks()
	if err != nil {
		return "", fmt.Errorf("failed to list tasks: %w", err)
	}

	if len(tasks) == 0 {
		return "", fmt.Errorf("no tasks found")
	}

	// Build a list of task titles for fuzzy matching
	names := make([]string, len(tasks))
	for i, t := range tasks {
		names[i] = t.Text
	}

	// Perform fuzzy matching
	matches := fuzzy.Find(pattern, names)

	if len(matches) == 0 {
		return "", fmt.Errorf("no tasks match pattern: %s", pattern)
	}

	if len(matches) > 1 {
		// Multiple matches - show candidates and return error
		candidates := make([]string, 0, len(matches))
		for _, m := range matches {
			candidates = append(candidates, fmt.Sprintf("  - %s", tasks[m.Index].Text))
		}
		return "", fmt.Errorf("multiple tasks match pattern %q:\n%s\nUse a more specific pattern", pattern, strings.Join(candidates, "\n"))
	}

	// Single match found
	matchedTask := tasks[matches[0].Index]
	return matchedTask.ID, nil
}

// GetTaskCommand returns the task command
func GetTaskCommand() *cobra.Command {
	return taskCmd
}

const maxStdinSize = 64 * 1024

// readStdinIDs reads task IDs from stdin, one per line.
// Empty lines and whitespace-only lines are ignored.
// Returns an error if reading fails or if no valid IDs are found.
// Uses a size limit to prevent DoS via memory exhaustion.
func readStdinIDs() ([]string, error) {
	lr := &io.LimitedReader{R: os.Stdin, N: maxStdinSize}
	data, err := io.ReadAll(lr)
	if err != nil {
		return nil, fmt.Errorf("failed to read from stdin: %w", err)
	}
	if lr.N == 0 {
		return nil, fmt.Errorf("input too large (max %d bytes)", maxStdinSize)
	}

	var ids []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			ids = append(ids, line)
		}
	}

	if len(ids) == 0 {
		return nil, fmt.Errorf("no task IDs provided via stdin")
	}

	return ids, nil
}
