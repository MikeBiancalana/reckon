package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var logCmd = &cobra.Command{
	Use:   "log [message | -]",
	Short: "Append a log entry to today's journal",
	Long:  `Appends a timestamped log entry to today's journal. Use - to read from stdin, or run without arguments for interactive input.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var message string

		if len(args) == 0 {
			// Interactive mode: prompt user for input
			fmt.Fprint(os.Stderr, "Enter log message: ")
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("failed to read input: %w", err)
			}
			message = strings.TrimSpace(string(data))
			if message == "" {
				return fmt.Errorf("no message provided")
			}
		} else if args[0] == "-" {
			// Read from stdin
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("failed to read from stdin: %w", err)
			}
			message = strings.TrimSpace(string(data))
			if message == "" {
				return fmt.Errorf("no message provided via stdin")
			}
		} else {
			message = args[0]
		}

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
