package cli

import (
	"fmt"
	"os"

	"github.com/MikeBiancalana/reckon/internal/task"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var reviewCmd = &cobra.Command{
	Use:   "review",
	Short: "Review stale tasks",
	Long:  "Review tasks that haven't been updated recently to keep your task list clean.",
}

// reviewListCmd lists stale tasks without interactive mode
var reviewListCmd = &cobra.Command{
	Use:   "list",
	Short: "List stale tasks",
	Long:  "List tasks that haven't been updated in 7+ days.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if taskService == nil {
			return fmt.Errorf("task service not initialized")
		}

		staleTasks, err := taskService.FindStaleTasks(7)
		if err != nil {
			return fmt.Errorf("failed to find stale tasks: %w", err)
		}

		if len(staleTasks) == 0 {
			fmt.Println("No stale tasks found")
			return nil
		}

		fmt.Printf("Found %d stale task(s):\n\n", len(staleTasks))
		for i, t := range staleTasks {
			fmt.Printf("[%d] [%s] %s\n", i+1, t.Status, t.Title)
			fmt.Printf("  ID: %s\n", t.ID)
			fmt.Printf("  Created: %s\n", t.Created)
			if len(t.Tags) > 0 {
				fmt.Printf("  Tags: %s\n", t.Tags)
			}
			fmt.Println()
		}

		return nil
	},
}

// reviewInteractiveCmd starts interactive review mode
var reviewInteractiveCmd = &cobra.Command{
	Use:   "interactive",
	Short: "Start interactive review mode",
	Long:  "Interactively review stale tasks with quick actions.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if taskService == nil {
			return fmt.Errorf("task service not initialized")
		}

		// Check if we're in a TTY
		if !isTerminal(os.Stdin) {
			return fmt.Errorf("interactive mode requires a terminal. Use 'rk review list' for non-interactive output")
		}

		staleTasks, err := taskService.FindStaleTasks(7)
		if err != nil {
			return fmt.Errorf("failed to find stale tasks: %w", err)
		}

		if len(staleTasks) == 0 {
			fmt.Println("No stale tasks found to review")
			return nil
		}

		fmt.Printf("Found %d stale task(s) to review\n", len(staleTasks))
		fmt.Println("Starting interactive review mode...")
		fmt.Println("Commands: d=done, s=someday, w=waiting, n=note, x=delete, space=skip, q=quit")
		fmt.Println()

		reviewed := 0
		done := 0
		deferred := 0
		deleted := 0

		for i, t := range staleTasks {
			fmt.Printf("\n[%d/%d] %s\n", i+1, len(staleTasks), t.Title)
			fmt.Printf("Status: %s | Created: %s", t.Status, t.Created)
			if len(t.Tags) > 0 {
				fmt.Printf(" | Tags: %s", t.Tags)
			}
			fmt.Println()
			fmt.Print("Action (d/s/w/n/x/space/q): ")

			var action string
			fmt.Scanln(&action)

			switch action {
			case "d":
				if err := taskService.UpdateStatus(t.ID, task.StatusDone); err != nil {
					fmt.Printf("Error marking task done: %v\n", err)
				} else {
					fmt.Printf("✓ Marked as done\n")
					done++
				}
			case "s":
				if err := taskService.UpdateStatus(t.ID, task.StatusSomeday); err != nil {
					fmt.Printf("Error moving to someday: %v\n", err)
				} else {
					fmt.Printf("✓ Moved to someday\n")
					deferred++
				}
			case "w":
				fmt.Print("Reason for waiting: ")
				var reason string
				fmt.Scanln(&reason)
				if err := taskService.UpdateStatus(t.ID, task.StatusWaiting); err != nil {
					fmt.Printf("Error marking as waiting: %v\n", err)
				} else {
					fmt.Printf("✓ Marked as waiting: %s\n", reason)
					deferred++
				}
			case "n":
				fmt.Print("Note to add: ")
				var note string
				fmt.Scanln(&note)
				if err := taskService.AppendLog(t.ID, note); err != nil {
					fmt.Printf("Error adding note: %v\n", err)
				} else {
					fmt.Printf("✓ Added note\n")
				}
				continue // Don't count as reviewed
			case "x":
				// For now, just mark as done (deletion could be dangerous in interactive mode)
				if err := taskService.UpdateStatus(t.ID, task.StatusDone); err != nil {
					fmt.Printf("Error marking task done: %v\n", err)
				} else {
					fmt.Printf("✓ Marked as done (delete not implemented in interactive mode)\n")
					done++
				}
			case "space", "":
				fmt.Printf("Skipped\n")
				continue
			case "q":
				fmt.Printf("Review cancelled\n")
				break
			default:
				fmt.Printf("Unknown action: %s\n", action)
				continue
			}

			reviewed++
		}

		fmt.Printf("\nReview complete: %d reviewed, %d done, %d deferred, %d deleted\n", reviewed, done, deferred, deleted)
		return nil
	},
}

func init() {
	// Add subcommands
	reviewCmd.AddCommand(reviewListCmd)
	reviewCmd.AddCommand(reviewInteractiveCmd)
}

// GetReviewCommand returns the review command
func GetReviewCommand() *cobra.Command {
	return reviewCmd
}

// isTerminal checks if the given file descriptor is a terminal
func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}
