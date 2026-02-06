package cli

import (
	"fmt"

	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/spf13/cobra"
)

var todayFormatFlag string

var todayCmd = &cobra.Command{
	Use:   "today",
	Short: "Output today's journal to stdout",
	Long:  `Outputs today's journal content to stdout (useful for piping to LLMs).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if journalService == nil {
			return fmt.Errorf("journal service not initialized")
		}

		today, err := getEffectiveDate()
		if err != nil {
			return err
		}

		if todayFormatFlag != "" {
			format, err := parseFormat(todayFormatFlag)
			if err != nil {
				return err
			}
			switch format {
			case FormatJSON:
				j, err := journalService.GetByDate(today)
				if err != nil {
					return fmt.Errorf("failed to get journal for %s: %w", today, err)
				}
				return formatJournalsJSON([]*journal.Journal{j})
			case FormatTSV:
				j, err := journalService.GetByDate(today)
				if err != nil {
					return fmt.Errorf("failed to get journal for %s: %w", today, err)
				}
				return formatJournalTSV(j)
			case FormatCSV:
				j, err := journalService.GetByDate(today)
				if err != nil {
					return fmt.Errorf("failed to get journal for %s: %w", today, err)
				}
				return formatJournalsCSV([]*journal.Journal{j})
			}
			return nil
		}

		content, err := journalService.GetJournalContent(today)
		if err != nil {
			return fmt.Errorf("failed to get journal for %s: %w", today, err)
		}

		fmt.Print(content)
		return nil
	},
}

func init() {
	todayCmd.Flags().StringVar(&todayFormatFlag, "format", "", "Output format (json, tsv, csv)")
}
