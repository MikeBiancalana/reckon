package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var rebuildCmd = &cobra.Command{
	Use:   "rebuild",
	Short: "Rebuild the SQLite database from markdown files",
	Long:  `Deletes and regenerates the SQLite database by scanning all markdown journal files.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !quietFlag {
			fmt.Println("Rebuilding database from markdown files...")
		}

		if err := journalService.Rebuild(); err != nil {
			return fmt.Errorf("failed to rebuild database: %w", err)
		}

		if !quietFlag {
			fmt.Println("✓ Database rebuilt successfully")
		}
		return nil
	},
}
