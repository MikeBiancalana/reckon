package cli

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/MikeBiancalana/reckon/internal/tui/components"
	"github.com/sahilm/fuzzy"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const (
	idWidth       = 8
	statusWidth   = 10
	createdWidth  = 10
	tagsWidth     = 15
	minTitleWidth = 20
)

// setTaskDates is a helper function to set schedule and deadline dates on a task.
// It takes the task ID and optional schedule/deadline strings in relative date format.
func setTaskDates(taskID, scheduleStr, deadlineStr string) error {
	// Set schedule if provided
	if scheduleStr != "" {
		parsedDate, err := components.ParseRelativeDate(scheduleStr)
		if err != nil {
			return fmt.Errorf("invalid schedule date: %w", err)
		}
		formattedDate := components.FormatDate(parsedDate)
		if err := journalTaskService.ScheduleTask(taskID, formattedDate); err != nil {
			return fmt.Errorf("failed to set schedule: %w", err)
		}
	}

	// Set deadline if provided
	if deadlineStr != "" {
		parsedDate, err := components.ParseRelativeDate(deadlineStr)
		if err != nil {
			return fmt.Errorf("invalid deadline date: %w", err)
		}
		formattedDate := components.FormatDate(parsedDate)
		if err := journalTaskService.SetTaskDeadline(taskID, formattedDate); err != nil {
			return fmt.Errorf("failed to set deadline: %w", err)
		}
	}

	return nil
}

var (
	taskStatusFlag       string
	taskTagsFlag         []string
	taskCompactFlag      bool
	taskVerboseFlag      bool
	taskFormatFlag       string
	taskEditTitleFlag    string
	taskEditTagsFlag     []string
	taskMatchFlag        string
	taskScheduledFlag    string
	taskDeadlineFlag     string
	taskOverdueFlag      bool
	taskGroupedFlag      bool
	taskNewScheduleFlag  string
	taskNewDeadlineFlag  string
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

		// Check if we should launch the interactive form
		// Launch form if: no title AND no flags are set
		hasFlags := len(taskTagsFlag) > 0 || taskNewScheduleFlag != "" || taskNewDeadlineFlag != ""
		if title == "" && !hasFlags {
			return launchTaskNewForm()
		}

		// Otherwise, use CLI arguments
		if title == "" {
			return fmt.Errorf("task title is required")
		}

		// Create task
		taskID, err := journalTaskService.AddTask(title, taskTagsFlag)
		if err != nil {
			return fmt.Errorf("failed to create task: %w", err)
		}

		// Set schedule and deadline if provided
		if err := setTaskDates(taskID, taskNewScheduleFlag, taskNewDeadlineFlag); err != nil {
			return err
		}

		if !quietFlag {
			fmt.Printf("✓ Created task: %s\n", title)
			if len(taskTagsFlag) > 0 {
				fmt.Printf("  Tags: %s\n", strings.Join(taskTagsFlag, ", "))
			}
			if taskNewScheduleFlag != "" {
				fmt.Printf("  Scheduled: %s\n", taskNewScheduleFlag)
			}
			if taskNewDeadlineFlag != "" {
				fmt.Printf("  Deadline: %s\n", taskNewDeadlineFlag)
			}
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

		// Initialize date helpers for filtering
		todayDate := time.Now().Format("2006-01-02")

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

		// Apply scheduled date filter if provided
		if taskScheduledFlag != "" {
			todayDate := time.Now().Format("2006-01-02")
			endOfWeek := getEndOfWeek()

			filtered := make([]journal.Task, 0)
			for _, t := range tasks {
				if t.ScheduledDate == nil {
					continue
				}
				scheduledDate := *t.ScheduledDate

				switch taskScheduledFlag {
				case "today":
					if scheduledDate == todayDate {
						filtered = append(filtered, t)
					}
				case "this-week":
					if scheduledDate >= todayDate && scheduledDate <= endOfWeek {
						filtered = append(filtered, t)
					}
				default:
					// Treat as specific date
					if scheduledDate == taskScheduledFlag {
						filtered = append(filtered, t)
					}
				}
			}
			tasks = filtered
		}

		// Apply deadline filter if provided
		if taskDeadlineFlag != "" {
			todayDate := time.Now().Format("2006-01-02")
			endOfWeek := getEndOfWeek()

			filtered := make([]journal.Task, 0)
			for _, t := range tasks {
				if t.DeadlineDate == nil {
					continue
				}
				deadlineDate := *t.DeadlineDate

				switch taskDeadlineFlag {
				case "today":
					if deadlineDate == todayDate {
						filtered = append(filtered, t)
					}
				case "this-week":
					if deadlineDate >= todayDate && deadlineDate <= endOfWeek {
						filtered = append(filtered, t)
					}
				default:
					if deadlineDate == taskDeadlineFlag {
						filtered = append(filtered, t)
					}
				}
			}
			tasks = filtered
		}

		// Apply overdue filter if provided
		if taskOverdueFlag {
			filtered := make([]journal.Task, 0)
			for _, t := range tasks {
				if t.DeadlineDate != nil && *t.DeadlineDate < todayDate && t.Status != journal.TaskDone {
					filtered = append(filtered, t)
				}
			}
			tasks = filtered
		}

		// Apply grouped output if requested
		if taskGroupedFlag {
			return listTasksGrouped(tasks)
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
			if !quietFlag {
				fmt.Println("No tasks found")
			}
			return nil
		}

		if taskVerboseFlag {
			// Verbose output with sequential numbers
			if !quietFlag {
				fmt.Printf("Found %d task(s):\n\n", len(tasks))
			}
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

		if !quietFlag {
			fmt.Printf("✓ Added note to task %s\n", taskID)
		}

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

		if !quietFlag {
			fmt.Printf("✓ Toggled task %s status\n", taskID)
		}

		return nil
	},
}

// taskEditCmd edits a task's details
var taskEditCmd = &cobra.Command{
	Use:   "edit [task-id|--match <pattern>]",
	Short: "Edit task details",
	Long: `Edit a task's title using the --title flag and tags with --tags flag.

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

		if !quietFlag {
			fmt.Printf("✓ Updated task %s\n", taskID)
			if taskEditTitleFlag != "" {
				fmt.Printf("  New title: %s\n", taskEditTitleFlag)
			}
			if len(taskEditTagsFlag) > 0 {
				fmt.Printf("  Note: Tags are not yet fully supported in the unified task system\n")
			}
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

		if !quietFlag {
			fmt.Printf("✓ Added note to task %s\n", taskID)
		}
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

// taskScheduleCmd sets a task's scheduled date
var taskScheduleCmd = &cobra.Command{
	Use:   "schedule [task-id|--match <pattern>] <date>",
	Short: "Set a task's scheduled date",
	Long: `Set a task's scheduled date.

Use task index (1, 2, 3...) or exact task ID.
Or use --match to fuzzy-match by task title.

Supported date formats:
  - YYYY-MM-DD (e.g., 2025-01-15)
  - today/t (today)
  - tomorrow/tm (tomorrow)
  - mon, tue, wed, thu, fri, sat, sun (next occurrence)
  - +3d (3 days from now)
  - +2w (2 weeks from now)

Use --clear to remove the scheduled date.

Examples:
  rk task schedule 1 2025-01-15
  rk task schedule abc123 tomorrow
  rk task schedule --match auth +3d
  rk task schedule 1 --clear`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tmpMatch := taskMatchFlag
		taskMatchFlag = ""

		clearFlag, err := cmd.Flags().GetBool("clear")
		if err != nil {
			return fmt.Errorf("internal error: %w", err)
		}

		if journalTaskService == nil {
			return fmt.Errorf("task service not initialized")
		}

		var taskID, dateStr string

		if clearFlag {
			if len(args) > 1 {
				return fmt.Errorf("too many arguments; use --match for task matching or provide only task ID")
			}
			if tmpMatch != "" && len(args) == 1 {
				return fmt.Errorf("cannot use both task-id and --match with --clear")
			}
			if tmpMatch == "" && len(args) == 0 {
				return fmt.Errorf("missing task identifier; use task index, ID, or --match <pattern>")
			}
			if len(args) == 1 {
				taskID = args[0]
			}
		} else {
			if len(args) < 2 {
				return fmt.Errorf("missing date; provide a date or use --clear")
			}
			if len(args) > 2 {
				return fmt.Errorf("too many arguments; use quotes for dates with spaces")
			}

			if tmpMatch != "" && len(args) > 1 {
				return fmt.Errorf("cannot use both task-id and --match; use one or the other")
			}
			if tmpMatch == "" && len(args) < 2 {
				return fmt.Errorf("missing task identifier; use task index, ID, or --match <pattern>")
			}

			if tmpMatch != "" {
				taskID = tmpMatch
				dateStr = args[0]
			} else {
				taskID = args[0]
				dateStr = args[1]
			}
		}

		if tmpMatch != "" {
			taskID = tmpMatch
		}

		if taskID == "" {
			return fmt.Errorf("missing task identifier; use task index, ID, or --match <pattern>")
		}

		resolvedTaskID, err := resolveJournalTaskID(taskID, "", journalTaskService)
		if err != nil {
			return err
		}

		if clearFlag {
			if err := journalTaskService.ClearTaskSchedule(resolvedTaskID); err != nil {
				return fmt.Errorf("failed to clear schedule: %w", err)
			}
			if !quietFlag {
				fmt.Printf("✓ Cleared scheduled date for task %s\n", resolvedTaskID)
			}
			return nil
		}

		parsedDate, err := components.ParseRelativeDate(dateStr)
		if err != nil {
			return fmt.Errorf("invalid date: %w", err)
		}
		formattedDate := components.FormatDate(parsedDate)

		if err := journalTaskService.ScheduleTask(resolvedTaskID, formattedDate); err != nil {
			return fmt.Errorf("failed to set schedule: %w", err)
		}

		if !quietFlag {
			desc := components.GetDateDescription(parsedDate)
			fmt.Printf("✓ Set scheduled date for task %s to %s (%s)\n", resolvedTaskID, formattedDate, desc)
		}

		return nil
	},
}

// taskDeadlineCmd sets a task's deadline
var taskDeadlineCmd = &cobra.Command{
	Use:   "deadline [task-id|--match <pattern>] <date>",
	Short: "Set a task's deadline",
	Long: `Set a task's deadline.

Use task index (1, 2, 3...) or exact task ID.
Or use --match to fuzzy-match by task title.

Supported date formats:
  - YYYY-MM-DD (e.g., 2025-01-15)
  - today/t (today)
  - tomorrow/tm (tomorrow)
  - mon, tue, wed, thu, fri, sat, sun (next occurrence)
  - +3d (3 days from now)
  - +2w (2 weeks from now)

Use --clear to remove the deadline.

Examples:
  rk task deadline 1 2025-01-15
  rk task deadline abc123 +3d
  rk task deadline --match auth next fri
  rk task deadline 1 --clear`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tmpMatch := taskMatchFlag
		taskMatchFlag = ""

		clearFlag, err := cmd.Flags().GetBool("clear")
		if err != nil {
			return fmt.Errorf("internal error: %w", err)
		}

		if journalTaskService == nil {
			return fmt.Errorf("task service not initialized")
		}

		var taskID, dateStr string

		if clearFlag {
			if len(args) > 1 {
				return fmt.Errorf("too many arguments; use --match for task matching or provide only task ID")
			}
			if tmpMatch != "" && len(args) == 1 {
				return fmt.Errorf("cannot use both task-id and --match with --clear")
			}
			if tmpMatch == "" && len(args) == 0 {
				return fmt.Errorf("missing task identifier; use task index, ID, or --match <pattern>")
			}
			if len(args) == 1 {
				taskID = args[0]
			}
		} else {
			if len(args) < 2 {
				return fmt.Errorf("missing date; provide a date or use --clear")
			}
			if len(args) > 2 {
				return fmt.Errorf("too many arguments; use quotes for dates with spaces")
			}

			if tmpMatch != "" && len(args) > 1 {
				return fmt.Errorf("cannot use both task-id and --match; use one or the other")
			}
			if tmpMatch == "" && len(args) < 2 {
				return fmt.Errorf("missing task identifier; use task index, ID, or --match <pattern>")
			}

			if tmpMatch != "" {
				taskID = tmpMatch
				dateStr = args[0]
			} else {
				taskID = args[0]
				dateStr = args[1]
			}
		}

		if tmpMatch != "" {
			taskID = tmpMatch
		}

		if taskID == "" {
			return fmt.Errorf("missing task identifier; use task index, ID, or --match <pattern>")
		}

		resolvedTaskID, err := resolveJournalTaskID(taskID, "", journalTaskService)
		if err != nil {
			return err
		}

		if clearFlag {
			if err := journalTaskService.ClearTaskDeadline(resolvedTaskID); err != nil {
				return fmt.Errorf("failed to clear deadline: %w", err)
			}
			if !quietFlag {
				fmt.Printf("✓ Cleared deadline for task %s\n", resolvedTaskID)
			}
			return nil
		}

		parsedDate, err := components.ParseRelativeDate(dateStr)
		if err != nil {
			return fmt.Errorf("invalid date: %w", err)
		}
		formattedDate := components.FormatDate(parsedDate)

		if err := journalTaskService.SetTaskDeadline(resolvedTaskID, formattedDate); err != nil {
			return fmt.Errorf("failed to set deadline: %w", err)
		}

		if !quietFlag {
			desc := components.GetDateDescription(parsedDate)
			fmt.Printf("✓ Set deadline for task %s to %s (%s)\n", resolvedTaskID, formattedDate, desc)
		}

		return nil
	},
}

// taskNewFormModel is the Bubble Tea model for the task creation form
type taskNewFormModel struct {
	form   *components.Form
	result *components.FormResult
	quit   bool
}

func (m taskNewFormModel) Init() tea.Cmd {
	return m.form.Show()
}

func (m taskNewFormModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			m.quit = true
			return m, tea.Quit
		}

	case components.FormSubmitMsg:
		m.result = &msg.Result
		m.quit = true
		return m, tea.Quit

	case components.FormCancelMsg:
		m.quit = true
		return m, tea.Quit
	}

	// Update form
	var cmd tea.Cmd
	m.form, cmd = m.form.Update(msg)
	return m, cmd
}

func (m taskNewFormModel) View() string {
	if m.quit {
		return ""
	}
	return m.form.View()
}

// launchTaskNewForm launches the interactive form for creating a new task
func launchTaskNewForm() error {
	// Create form
	form := components.NewForm("Create New Task")

	form.AddField(components.FormField{
		Label:       "Title",
		Key:         "title",
		Type:        components.FieldTypeText,
		Required:    true,
		Placeholder: "Enter task title",
	}).AddField(components.FormField{
		Label:       "Tags",
		Key:         "tags",
		Type:        components.FieldTypeText,
		Required:    false,
		Placeholder: "tag1, tag2, tag3",
	}).AddField(components.FormField{
		Label:       "Schedule",
		Key:         "schedule",
		Type:        components.FieldTypeDate,
		Required:    false,
		Placeholder: "t, tm, +3d, mon, 2026-01-31",
	}).AddField(components.FormField{
		Label:       "Deadline",
		Key:         "deadline",
		Type:        components.FieldTypeDate,
		Required:    false,
		Placeholder: "t, tm, +3d, mon, 2026-01-31",
	})

	// Create Bubble Tea program with form
	initialModel := taskNewFormModel{
		form: form,
	}

	p := tea.NewProgram(initialModel)

	// Run the program
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("failed to run form: %w", err)
	}

	m := finalModel.(taskNewFormModel)
	if m.result == nil {
		// Form was cancelled
		return nil
	}

	// Process form result
	return createTaskFromForm(m.result)
}

// createTaskFromForm creates a task from the form result
func createTaskFromForm(result *components.FormResult) error {
	title := strings.TrimSpace(result.Values["title"])
	if title == "" {
		return fmt.Errorf("task title is required")
	}

	// Parse tags from comma-separated string
	var tags []string
	if tagsStr := strings.TrimSpace(result.Values["tags"]); tagsStr != "" {
		for _, tag := range strings.Split(tagsStr, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				tags = append(tags, tag)
			}
		}
	}

	// Create the task
	taskID, err := journalTaskService.AddTask(title, tags)
	if err != nil {
		return fmt.Errorf("failed to create task: %w", err)
	}

	// Set schedule and deadline if provided
	scheduleStr := strings.TrimSpace(result.Values["schedule"])
	deadlineStr := strings.TrimSpace(result.Values["deadline"])
	if err := setTaskDates(taskID, scheduleStr, deadlineStr); err != nil {
		return err
	}

	// Print success message
	if !quietFlag {
		fmt.Printf("✓ Created task: %s\n", title)
		if len(tags) > 0 {
			fmt.Printf("  Tags: %s\n", strings.Join(tags, ", "))
		}
		if scheduleStr != "" {
			// Date parsing errors are safely ignored here because the date was already
			// validated during form submission, so ParseRelativeDate cannot fail
			parsedDate, _ := components.ParseRelativeDate(scheduleStr)
			desc := components.GetDateDescription(parsedDate)
			fmt.Printf("  Scheduled: %s (%s)\n", components.FormatDate(parsedDate), desc)
		}
		if deadlineStr != "" {
			// Date parsing errors are safely ignored here because the date was already
			// validated during form submission, so ParseRelativeDate cannot fail
			parsedDate, _ := components.ParseRelativeDate(deadlineStr)
			desc := components.GetDateDescription(parsedDate)
			fmt.Printf("  Deadline: %s (%s)\n", components.FormatDate(parsedDate), desc)
		}
	}

	return nil
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
	taskCmd.AddCommand(taskScheduleCmd)
	taskCmd.AddCommand(taskDeadlineCmd)

	// Flags
	taskNewCmd.Flags().StringSliceVar(&taskTagsFlag, "tags", []string{}, "Task tags (comma-separated)")
	taskNewCmd.Flags().StringVar(&taskNewScheduleFlag, "schedule", "", "Schedule date (t, tm, +3d, mon, YYYY-MM-DD)")
	taskNewCmd.Flags().StringVar(&taskNewDeadlineFlag, "deadline", "", "Deadline date (t, tm, +3d, mon, YYYY-MM-DD)")
	taskListCmd.Flags().StringVar(&taskStatusFlag, "status", "", "Filter by status (open, active, done)")
	taskListCmd.Flags().StringSliceVar(&taskTagsFlag, "tag", []string{}, "Filter by tags")
	taskListCmd.Flags().BoolVar(&taskCompactFlag, "compact", false, "Show compact single-line output")
	taskListCmd.Flags().BoolVarP(&taskVerboseFlag, "verbose", "v", false, "Show verbose multi-line output")
	taskListCmd.Flags().StringVar(&taskFormatFlag, "format", "", "Output format (json, tsv, csv)")
	taskListCmd.Flags().StringVar(&taskScheduledFlag, "scheduled", "", "Filter by scheduled date (today, this-week, YYYY-MM-DD)")
	taskListCmd.Flags().StringVar(&taskDeadlineFlag, "deadline", "", "Filter by deadline (today, this-week)")
	taskListCmd.Flags().BoolVar(&taskOverdueFlag, "overdue", false, "Show only overdue tasks")
	taskListCmd.Flags().BoolVar(&taskGroupedFlag, "grouped", false, "Group tasks by timeframe (TODAY, THIS WEEK, ALL TASKS)")
	taskEditCmd.Flags().StringVar(&taskEditTitleFlag, "title", "", "New task title")
	taskEditCmd.Flags().StringSliceVar(&taskEditTagsFlag, "tags", []string{}, "New task tags (comma-separated)")

	// Commands that support --match flag for fuzzy title matching
	taskShowCmd.Flags().StringVar(&taskMatchFlag, "match", "", "Fuzzy match task by title (alternative to task-id)")
	taskLogCmd.Flags().StringVar(&taskMatchFlag, "match", "", "Fuzzy match task by title (alternative to task-id)")
	taskDoneCmd.Flags().StringVar(&taskMatchFlag, "match", "", "Fuzzy match task by title (alternative to task-id)")
	taskEditCmd.Flags().StringVar(&taskMatchFlag, "match", "", "Fuzzy match task by title (alternative to task-id)")
	taskNoteCmd.Flags().StringVar(&taskMatchFlag, "match", "", "Fuzzy match task by title (alternative to task-id)")
	taskDeleteCmd.Flags().StringVar(&taskMatchFlag, "match", "", "Fuzzy match task by title (alternative to task-id)")
	taskScheduleCmd.Flags().StringVar(&taskMatchFlag, "match", "", "Fuzzy match task by title (alternative to task-id)")
	taskScheduleCmd.Flags().Bool("clear", false, "Clear the scheduled date")
	taskDeadlineCmd.Flags().StringVar(&taskMatchFlag, "match", "", "Fuzzy match task by title (alternative to task-id)")
	taskDeadlineCmd.Flags().Bool("clear", false, "Clear the deadline")

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

// getEndOfWeek returns the date string for the end of the current week (Sunday)
func getEndOfWeek() string {
	now := time.Now()
	weekday := now.Weekday()
	daysToSunday := int(time.Sunday - weekday)
	if daysToSunday <= 0 {
		daysToSunday += 7
	}
	return now.AddDate(0, 0, daysToSunday).Format("2006-01-02")
}

// listTasksGrouped outputs tasks grouped by timeframe: TODAY, THIS WEEK, and ALL TASKS
func listTasksGrouped(tasks []Task) error {
	todayDate := time.Now().Format("2006-01-02")
	endOfWeek := getEndOfWeek()

	var today, thisWeek, allTasks []Task
	for _, t := range tasks {
		if t.ScheduledDate == nil {
			allTasks = append(allTasks, t)
			continue
		}

		scheduledDate := *t.ScheduledDate
		if scheduledDate == todayDate {
			today = append(today, t)
		} else if scheduledDate >= todayDate && scheduledDate <= endOfWeek {
			thisWeek = append(thisWeek, t)
		} else {
			allTasks = append(allTasks, t)
		}
	}

	// Print each group
	if len(today) > 0 {
		fmt.Println("=== TODAY ===")
		for i, t := range today {
			fmt.Printf("[%d] [%s] %s", i+1, t.Status, t.Text)
			if t.DeadlineDate != nil {
				fmt.Printf(" [deadline: %s]", *t.DeadlineDate)
			}
			fmt.Println()
		}
		fmt.Println()
	}

	if len(thisWeek) > 0 {
		fmt.Println("=== THIS WEEK ===")
		for i, t := range thisWeek {
			fmt.Printf("[%d] [%s] %s", i+1, t.Status, t.Text)
			if t.ScheduledDate != nil {
				fmt.Printf(" [scheduled: %s]", *t.ScheduledDate)
			}
			if t.DeadlineDate != nil {
				fmt.Printf(" [deadline: %s]", *t.DeadlineDate)
			}
			fmt.Println()
		}
		fmt.Println()
	}

	if len(allTasks) > 0 {
		fmt.Println("=== ALL TASKS ===")
		for i, t := range allTasks {
			fmt.Printf("[%d] [%s] %s", i+1, t.Status, t.Text)
			if t.ScheduledDate != nil {
				fmt.Printf(" [scheduled: %s]", *t.ScheduledDate)
			}
			if t.DeadlineDate != nil {
				fmt.Printf(" [deadline: %s]", *t.DeadlineDate)
			}
			fmt.Println()
		}
	}

	return nil
}

// Task is a local alias for journal.Task to use in listTasksGrouped
type Task = journal.Task
