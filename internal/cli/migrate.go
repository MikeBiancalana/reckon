package cli

import "github.com/spf13/cobra"

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migration commands",
	Long:  `Migration commands: move data from earlier reckon generations onto the current vault-native format.`,
	// Args+Run (not RunE) make cobra reject an unrecognized child ("rk
	// migrate bogus") instead of silently falling through to help: without
	// Run/RunE the command isn't Runnable and cobra always treats it as a
	// help request, before Args ever gets a chance to run.
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}
