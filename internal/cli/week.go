package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/spf13/cobra"
)

var jsonFlag bool

var weekCmd = &cobra.Command{
	Use:   "week",
	Short: "Output the last 7 days of journals to stdout",
	Long:  `Outputs the last 7 days of journal content to stdout (useful for weekly reviews).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if service == nil {
			return fmt.Errorf("journal service not initialized")
		}

		if jsonFlag {
			var journals []*journal.Journal
			var err error
			if dateFlag != "" {
				journals, err = service.GetWeekJournalsFromDate(dateFlag)
			} else {
				journals, err = service.GetWeekJournals()
			}
			if err != nil {
				return fmt.Errorf("failed to get week's journals: %w", err)
			}
			if err := json.NewEncoder(os.Stdout).Encode(journals); err != nil {
				return fmt.Errorf("failed to encode journals as JSON: %w", err)
			}
			return nil
		}

		var content string
		var err error
		if dateFlag != "" {
			content, err = service.GetWeekContentFromDate(dateFlag)
		} else {
			content, err = service.GetWeekContent()
		}
		if err != nil {
			return fmt.Errorf("failed to get week's journals: %w", err)
		}

		fmt.Print(content)
		return nil
	},
}

func init() {
	weekCmd.Flags().BoolVar(&jsonFlag, "json", false, "Output as JSON")
}
