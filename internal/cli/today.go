package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var todayCmd = &cobra.Command{
	Use:   "today",
	Short: "Output today's journal to stdout",
	Long:  `Outputs today's journal content to stdout (useful for piping to LLMs).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		today := time.Now().Format("2006-01-02")

		content, err := service.GetJournalContent(today)
		if err != nil {
			return fmt.Errorf("failed to get today's journal: %w", err)
		}

		fmt.Print(content)
		return nil
	},
}
