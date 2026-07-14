package cli

import "github.com/spf13/cobra"

var noteCmd = &cobra.Command{
	Use:   "note",
	Short: "Manage notes",
	Long:  `Manage notes: create, show, rename, and (re)index notes/**/index.md catalogs.`,
}

func GetNoteCommand() *cobra.Command {
	return noteCmd
}
