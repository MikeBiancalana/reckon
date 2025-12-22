package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var weekCmd = &cobra.Command{
	Use:   "week",
	Short: "Output the last 7 days of journals to stdout",
	Long:  `Outputs the last 7 days of journal content to stdout (useful for weekly reviews).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		content, err := service.GetWeekContent()
		if err != nil {
			return fmt.Errorf("failed to get week's journals: %w", err)
		}

		fmt.Print(content)
		return nil
	},
}
