package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var winJsonFlag bool

var winCmd = &cobra.Command{
	Use:   "win",
	Short: "Manage daily wins",
	Long:  "Add, list, and delete daily wins and accomplishments.",
}

var winAddCmd = &cobra.Command{
	Use:   "add [text]",
	Short: "Add a new win",
	Long: `Add a new win to today's journal (or the date specified with --date).

Examples:
  rk win add "Fixed the login bug"
  rk win add "Completed project documentation" --date 2024-01-15`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		text := strings.Join(args, " ")

		effectiveDate, err := getEffectiveDate()
		if err != nil {
			return err
		}

		j, err := service.GetByDate(effectiveDate)
		if err != nil {
			return fmt.Errorf("failed to get journal for %s: %w", effectiveDate, err)
		}

		if err := service.AddWin(j, text); err != nil {
			return fmt.Errorf("failed to add win: %w", err)
		}

		fmt.Printf("✓ Added win: %s\n", text)
		return nil
	},
}

var winListCmd = &cobra.Command{
	Use:   "list",
	Short: "List wins",
	Long: `List all wins for today (or the date specified with --date).

Use --json to output as JSON array.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		effectiveDate, err := getEffectiveDate()
		if err != nil {
			return err
		}

		j, err := service.GetByDate(effectiveDate)
		if err != nil {
			return fmt.Errorf("failed to get journal for %s: %w", effectiveDate, err)
		}

		wins := j.Wins

		if winJsonFlag {
			if err := json.NewEncoder(os.Stdout).Encode(wins); err != nil {
				return fmt.Errorf("failed to encode wins as JSON: %w", err)
			}
			return nil
		}

		if len(wins) == 0 {
			fmt.Println("No wins found")
			return nil
		}

		fmt.Printf("Wins for %s:\n\n", effectiveDate)

		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.TabIndent)
		fmt.Fprintln(tw, "ID\tTEXT")
		for _, w := range wins {
			fmt.Fprintf(tw, "%.8s\t%s\n", w.ID, w.Text)
		}
		tw.Flush()

		return nil
	},
}

var winDeleteCmd = &cobra.Command{
	Use:   "delete [win-id]",
	Short: "Delete a win by ID",
	Long: `Delete a win by its ID.

Examples:
  rk win delete abc12345
  rk win delete abc12345 --date 2024-01-15`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		winID := args[0]

		effectiveDate, err := getEffectiveDate()
		if err != nil {
			return err
		}

		j, err := service.GetByDate(effectiveDate)
		if err != nil {
			return fmt.Errorf("failed to get journal for %s: %w", effectiveDate, err)
		}

		if err := service.DeleteWin(j, winID); err != nil {
			return fmt.Errorf("failed to delete win: %w", err)
		}

		fmt.Printf("✓ Deleted win %s\n", winID)
		return nil
	},
}

func init() {
	winCmd.AddCommand(winAddCmd)
	winCmd.AddCommand(winListCmd)
	winCmd.AddCommand(winDeleteCmd)

	winListCmd.Flags().BoolVar(&winJsonFlag, "json", false, "Output as JSON")
}

func GetWinCommand() *cobra.Command {
	return winCmd
}
