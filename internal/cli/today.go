package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var todayFormatFlag string

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

		if todayFormatFlag != "" {
			format, err := parseFormat(todayFormatFlag)
			if err != nil {
				return err
			}
			j, err := service.GetByDate(today)
			if err != nil {
				return fmt.Errorf("failed to get journal for %s: %w", today, err)
			}
			switch format {
			case FormatJSON:
				if err := formatJournalJSON(j); err != nil {
					return fmt.Errorf("failed to format journal as JSON: %w", err)
				}
			case FormatTSV:
				if err := formatJournalTSV(j); err != nil {
					return fmt.Errorf("failed to format journal as TSV: %w", err)
				}
			case FormatCSV:
				if err := formatJournalCSV(j); err != nil {
					return fmt.Errorf("failed to format journal as CSV: %w", err)
				}
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
	todayCmd.Flags().StringVar(&todayFormatFlag, "format", "", "Output format (json, tsv, csv)")
}
