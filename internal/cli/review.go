package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var reviewCmd = &cobra.Command{
	Use:   "review",
	Short: "Review stale tasks",
	Long:  "Review tasks that haven't been updated recently to keep your task list clean.",
}

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

var reviewInteractiveCmd = &cobra.Command{
	Use:   "interactive",
	Short: "Start interactive review mode (not yet supported in unified system)",
	Long:  "Interactively review stale tasks with quick actions.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !quietFlag {
			fmt.Println("Note: Interactive task review is not yet implemented in the unified task system.")
			fmt.Println("This feature will be added in a future update.")
		}
		return nil
	},
}

func init() {
	reviewCmd.AddCommand(reviewListCmd)
	reviewCmd.AddCommand(reviewInteractiveCmd)
}

func GetReviewCommand() *cobra.Command {
	return reviewCmd
}
