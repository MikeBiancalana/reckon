package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var weekJsonFlag bool

var weekCmd = &cobra.Command{
	Use:   "week",
	Short: "Output the last 7 days of journals to stdout",
	Long:  `Outputs the last 7 days of journal content to stdout (useful for weekly reviews).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if service == nil {
			return fmt.Errorf("journal service not initialized")
		}

		if weekJsonFlag {
			journals, err := service.GetWeekJournals()
			if err != nil {
				return fmt.Errorf("failed to get week's journals: %w", err)
			}
			if err := json.NewEncoder(os.Stdout).Encode(journals); err != nil {
				return fmt.Errorf("failed to encode journals as JSON: %w", err)
			}
			return nil
		}

		content, err := service.GetWeekContent()
		if err != nil {
			return fmt.Errorf("failed to get week's journals: %w", err)
		}

		fmt.Print(content)
		return nil
	},
}

func init() {
	weekCmd.Flags().BoolVar(&weekJsonFlag, "json", false, "Output as JSON")
}
