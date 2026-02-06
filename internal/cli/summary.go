package cli

import (
	"encoding/json"
	"fmt"
	"os"
	stdtime "time"

	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/MikeBiancalana/reckon/internal/time"
	"github.com/spf13/cobra"
)

var weekFlag bool
var summaryJsonFlag bool

var summaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Show time tracking summary",
	Long:  `Show time tracking summary for today or the past week.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if weekFlag {
			return showWeekSummary()
		}
		return showTodaySummary()
	},
}

func showTodaySummary() error {
	effectiveDate, err := getEffectiveDate()
	if err != nil {
		return err
	}

	j, err := journalService.GetByDate(effectiveDate)
	if err != nil {
		return fmt.Errorf("error getting journal for %s: %w", effectiveDate, err)
	}

	summary := time.CalculateDaySummary(j)

	if summaryJsonFlag {
		if err := json.NewEncoder(os.Stdout).Encode(summary); err != nil {
			return fmt.Errorf("failed to encode summary as JSON: %w", err)
		}
		return nil
	}

	if dateFlag != "" {
		fmt.Printf("Time Summary for %s:\n", effectiveDate)
	} else {
		fmt.Println("Time Summary for Today:")
	}
	fmt.Println("")
	fmt.Printf("  Meetings:   %s\n", summary.MeetingsFormatted())
	fmt.Printf("  Tasks:      %s\n", summary.TasksFormatted())
	fmt.Printf("  Breaks:     %s\n", summary.BreaksFormatted())
	fmt.Printf("  Untracked:  %s\n", summary.UntrackedFormatted())
	fmt.Println("")
	fmt.Printf("  Total Tracked: %s\n", summary.TotalTrackedFormatted())

	return nil
}

func showWeekSummary() error {
	effectiveDate, err := getEffectiveDate()
	if err != nil {
		return err
	}

	journals := make(map[string]*journal.Journal)
	start := stdtime.Now()
	if dateFlag != "" {
		start, err = stdtime.Parse("2006-01-02", effectiveDate)
		if err != nil {
			return fmt.Errorf("invalid effective date: %w", err)
		}
	}

	for i := 6; i >= 0; i-- {
		date := start.AddDate(0, 0, -i).Format("2006-01-02")
		j, err := journalService.GetByDate(date)
		if err != nil {
			continue
		}
		journals[date] = j
	}

	summary := time.CalculateWeekSummary(journals)

	if summaryJsonFlag {
		if err := json.NewEncoder(os.Stdout).Encode(summary); err != nil {
			return fmt.Errorf("failed to encode summary as JSON: %w", err)
		}
		return nil
	}

	if dateFlag != "" {
		fmt.Printf("Time Summary for the Week of %s:\n", effectiveDate)
	} else {
		fmt.Println("Time Summary for the Past Week:")
	}
	fmt.Println("")
	fmt.Printf("  Meetings:   %s\n", summary.MeetingsFormatted())
	fmt.Printf("  Tasks:      %s\n", summary.TasksFormatted())
	fmt.Printf("  Breaks:     %s\n", summary.BreaksFormatted())
	fmt.Printf("  Untracked:  %s\n", summary.UntrackedFormatted())
	fmt.Println("")
	fmt.Printf("  Total Tracked: %s\n", summary.TotalTrackedFormatted())

	return nil
}

func init() {
	summaryCmd.Flags().BoolVarP(&weekFlag, "week", "w", false, "Show summary for the past week")
	summaryCmd.Flags().BoolVar(&summaryJsonFlag, "json", false, "Output as JSON")
	RootCmd.AddCommand(summaryCmd)
}
