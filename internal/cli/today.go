package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var todayJsonFlag bool

var todayCmd = &cobra.Command{
	Use:   "today",
	Short: "Output today's journal to stdout",
	Long:  `Outputs today's journal content to stdout (useful for piping to LLMs).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if service == nil {
			return fmt.Errorf("journal service not initialized")
		}

		today, err := getEffectiveDate()
		if err != nil {
			return err
		}

		if todayJsonFlag {
			j, err := service.GetByDate(today)
			if err != nil {
				return fmt.Errorf("failed to get journal for %s: %w", today, err)
			}
			if err := json.NewEncoder(os.Stdout).Encode(j); err != nil {
				return fmt.Errorf("failed to encode journal as JSON: %w", err)
			}
			return nil
		}

		content, err := service.GetJournalContent(today)
		if err != nil {
			return fmt.Errorf("failed to get journal for %s: %w", today, err)
		}

		fmt.Print(content)
		return nil
	},
}

func init() {
	todayCmd.Flags().BoolVar(&todayJsonFlag, "json", false, "Output as JSON")
}
