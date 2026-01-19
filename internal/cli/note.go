package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var noteCmd = &cobra.Command{
	Use:   "note",
	Short: "Manage standalone notes",
	Long:  `Manage standalone notes - create, list, and delete notes.`,
}

var noteNewCmd = &cobra.Command{
	Use:   "new [text]",
	Short: "Create a new standalone note",
	Long: `Creates a new standalone note.
The note will be added to the journal as a log entry.`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
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
