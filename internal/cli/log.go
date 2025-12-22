package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var logCmd = &cobra.Command{
	Use:   "log [message]",
	Short: "Append a log entry to today's journal",
	Long:  `Appends a timestamped log entry to today's journal.`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		message := strings.Join(args, " ")

		j, err := service.GetToday()
		if err != nil {
			return fmt.Errorf("failed to get today's journal: %w", err)
		}

		if err := service.AppendLog(j, message); err != nil {
			return fmt.Errorf("failed to append log: %w", err)
		}

		fmt.Printf("✓ Logged: %s\n", message)
		return nil
	},
}
