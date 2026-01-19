package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var reviewCmd = &cobra.Command{
	Use:   "review",
	Short: "Review stale tasks",
	Long:  "Review tasks that haven't been updated recently to keep your task list clean.",
}

// reviewListCmd lists stale tasks without interactive mode
var reviewListCmd = &cobra.Command{
	Use:   "list",
	Short: "List stale tasks (not yet supported in unified system)",
	Long:  "List tasks that haven't been updated in 7+ days.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !quietFlag {
			fmt.Println("Note: Task review functionality is not yet implemented in the unified task system.")
			fmt.Println("This feature will be added in a future update.")
		}
		return nil
	},
}

// reviewInteractiveCmd starts interactive review mode
var reviewInteractiveCmd = &cobra.Command{
	Use:   "interactive",
	Short: "Start interactive review mode (not yet supported in unified system)",
	Long:  "Interactively review stale tasks with quick actions.",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check if we're in a TTY
		if !isTerminal(os.Stdout) {
			return fmt.Errorf("interactive mode requires a terminal. Use 'rk review list' for non-interactive output")
		}

		if !quietFlag {
			fmt.Println("Note: Interactive task review is not yet implemented in the unified task system.")
			fmt.Println("This feature will be added in a future update.")
		}
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
	// Simple check - in a real implementation, you'd use unix.Isatty
	return true // For now, assume it's a terminal
}
