package cli

import (
	"fmt"
	stdtime "time"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/MikeBiancalana/reckon/internal/storage"
	"github.com/MikeBiancalana/reckon/internal/time"
	"github.com/spf13/cobra"
)

var weekFlag bool

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
	dbPath, err := config.DatabasePath()
	if err != nil {
		return fmt.Errorf("error getting database path: %w", err)
	}

	db, err := storage.NewDatabase(dbPath)
	if err != nil {
		return fmt.Errorf("error opening database: %w", err)
	}

	repo := journal.NewRepository(db)
	fileStore := storage.NewFileStore()
	service := journal.NewService(repo, fileStore)

	j, err := service.GetToday()
	if err != nil {
		return fmt.Errorf("error getting today's journal: %w", err)
	}

	summary := time.CalculateDaySummary(j)

	fmt.Println("Time Summary for Today:")
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
	dbPath, err := config.DatabasePath()
	if err != nil {
		return fmt.Errorf("error getting database path: %w", err)
	}

	db, err := storage.NewDatabase(dbPath)
	if err != nil {
		return fmt.Errorf("error opening database: %w", err)
	}

	repo := journal.NewRepository(db)
	fileStore := storage.NewFileStore()
	service := journal.NewService(repo, fileStore)

	journals := make(map[string]*journal.Journal)
	today := stdtime.Now()

	for i := 6; i >= 0; i-- {
		date := today.AddDate(0, 0, -i).Format("2006-01-02")
		j, err := service.GetByDate(date)
		if err != nil {
			continue
		}
		journals[date] = j
	}

	summary := time.CalculateWeekSummary(journals)

	fmt.Println("Time Summary for the Past Week:")
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
	RootCmd.AddCommand(summaryCmd)
}
