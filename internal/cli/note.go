package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var noteCmd = &cobra.Command{
	Use:   "note",
	Short: "[DEPRECATED] Manage standalone notes - use 'rk log add' instead",
	Long: `[DEPRECATED] This command will be removed in a future version.

Please use 'rk log add' instead, which provides the same functionality
with additional interactive mode support.

The 'rk note' command was confusing because it creates journal log entries,
not zettelkasten notes (which are managed via 'rk notes').`,
}

var noteNewCmd = &cobra.Command{
	Use:   "new [text]",
	Short: "[DEPRECATED] Create a new standalone note - use 'rk log add' instead",
	Long: `[DEPRECATED] This command will be removed in a future version.

Please use 'rk log add [message]' instead, which provides the same functionality
with additional interactive mode support.

The 'rk note new' command was confusing because it creates journal log entries,
not zettelkasten notes (which are managed via 'rk notes').`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Display deprecation warning
		fmt.Fprintf(os.Stderr, "\n⚠️  WARNING: 'rk note new' is DEPRECATED and will be removed in a future version.\n")
		fmt.Fprintf(os.Stderr, "   Please use 'rk log add' instead for the same functionality.\n")
		fmt.Fprintf(os.Stderr, "   Example: rk log add %s\n\n", strings.Join(args, " "))

		var noteText string

		if len(args) == 0 {
			return fmt.Errorf("note text cannot be empty")
		}
		noteText = strings.TrimSpace(strings.Join(args, " "))

		if noteText == "" {
			return fmt.Errorf("note text cannot be empty")
		}

		effectiveDate, err := getEffectiveDate()
		if err != nil {
			return err
		}

		j, err := service.GetByDate(effectiveDate)
		if err != nil {
			return fmt.Errorf("failed to get journal for %s: %w", effectiveDate, err)
		}

		if err := service.AppendLog(j, noteText); err != nil {
			return fmt.Errorf("failed to add note: %w", err)
		}

		if !quietFlag {
			fmt.Printf("✓ Added note: %s\n", noteText)
		}
		return nil
	},
}

func init() {
	noteCmd.AddCommand(noteNewCmd)
}

func GetNoteCommand() *cobra.Command {
	return noteCmd
}
