package cli

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/MikeBiancalana/reckon/internal/journal"
)

type OutputFormat string

const (
	FormatJSON OutputFormat = "json"
	FormatTSV  OutputFormat = "tsv"
	FormatCSV  OutputFormat = "csv"
)

func parseFormat(s string) (OutputFormat, error) {
	switch strings.ToLower(s) {
	case "json":
		return FormatJSON, nil
	case "tsv":
		return FormatTSV, nil
	case "csv":
		return FormatCSV, nil
	default:
		return "", fmt.Errorf("unsupported format: %s (supported: json, tsv, csv)", s)
	}
}

func formatTasksJSON(tasks []journal.Task) error {
	return json.NewEncoder(os.Stdout).Encode(tasks)
}

func formatTasksTSV(tasks []journal.Task) error {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', tabwriter.TabIndent)
	fmt.Fprintln(tw, "ID\tSTATUS\tCREATED\tTAGS\tTITLE")
	for _, t := range tasks {
		tags := "-"
		if len(t.Tags) > 0 {
			tags = strings.Join(t.Tags, ", ")
		}
		fmt.Fprintf(tw, "%.8s\t%s\t%s\t%s\t%s\n", t.ID, t.Status, t.CreatedAt.Format("2006-01-02"), tags, t.Text)
	}
	tw.Flush()
	return nil
}

func formatTasksCSV(tasks []journal.Task) error {
	w := csv.NewWriter(os.Stdout)
	w.Write([]string{"ID", "STATUS", "CREATED", "TAGS", "TITLE"})
	for _, t := range tasks {
		tags := ""
		if len(t.Tags) > 0 {
			tags = strings.Join(t.Tags, ",")
		}
		record := []string{t.ID, string(t.Status), t.CreatedAt.Format("2006-01-02"), tags, t.Text}
		if err := w.Write(record); err != nil {
			return fmt.Errorf("failed to write CSV record: %w", err)
		}
	}
	w.Flush()
	return w.Error()
}

func formatScheduleItemsJSON(items []journal.ScheduleItem) error {
	return json.NewEncoder(os.Stdout).Encode(items)
}

func formatScheduleItemsTSV(items []journal.ScheduleItem) error {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', tabwriter.TabIndent)
	fmt.Fprintln(tw, "ID\tTIME\tCONTENT")
	for _, item := range items {
		timeStr := ""
		if !item.Time.IsZero() {
			timeStr = item.Time.Format("15:04")
		}
		fmt.Fprintf(tw, "%.8s\t%s\t%s\n", item.ID, timeStr, item.Content)
	}
	tw.Flush()
	return nil
}

func formatScheduleItemsCSV(items []journal.ScheduleItem) error {
	w := csv.NewWriter(os.Stdout)
	w.Write([]string{"ID", "TIME", "CONTENT"})
	for _, item := range items {
		timeStr := ""
		if !item.Time.IsZero() {
			timeStr = item.Time.Format("15:04")
		}
		record := []string{item.ID, timeStr, item.Content}
		if err := w.Write(record); err != nil {
			return fmt.Errorf("failed to write CSV record: %w", err)
		}
	}
	w.Flush()
	return w.Error()
}

func formatWinsJSON(wins []journal.Win) error {
	return json.NewEncoder(os.Stdout).Encode(wins)
}

func formatWinsTSV(wins []journal.Win) error {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', tabwriter.TabIndent)
	fmt.Fprintln(tw, "ID\tTEXT")
	for _, w := range wins {
		fmt.Fprintf(tw, "%.8s\t%s\n", w.ID, w.Text)
	}
	tw.Flush()
	return nil
}

func formatWinsCSV(wins []journal.Win) error {
	w := csv.NewWriter(os.Stdout)
	w.Write([]string{"ID", "TEXT"})
	for _, win := range wins {
		record := []string{win.ID, win.Text}
		if err := w.Write(record); err != nil {
			return fmt.Errorf("failed to write CSV record: %w", err)
		}
	}
	w.Flush()
	return w.Error()
}

func formatJournalJSON(j *journal.Journal) error {
	return json.NewEncoder(os.Stdout).Encode(j)
}

func formatJournalTSV(j *journal.Journal) error {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', tabwriter.TabIndent)
	fmt.Fprintln(tw, "DATE\tINTENTIONS\tWINS\tLOG_ENTRIES\tSCHEDULE_ITEMS")
	fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%d\n", j.Date, len(j.Intentions), len(j.Wins), len(j.LogEntries), len(j.ScheduleItems))
	tw.Flush()
	return nil
}

func formatJournalCSV(j *journal.Journal) error {
	w := csv.NewWriter(os.Stdout)
	w.Write([]string{"DATE", "INTENTIONS", "WINS", "LOG_ENTRIES", "SCHEDULE_ITEMS"})
	record := []string{j.Date, fmt.Sprintf("%d", len(j.Intentions)), fmt.Sprintf("%d", len(j.Wins)), fmt.Sprintf("%d", len(j.LogEntries)), fmt.Sprintf("%d", len(j.ScheduleItems))}
	if err := w.Write(record); err != nil {
		return fmt.Errorf("failed to write CSV record: %w", err)
	}
	w.Flush()
	return w.Error()
}

func formatJournalsJSON(journals []*journal.Journal) error {
	return json.NewEncoder(os.Stdout).Encode(journals)
}

func formatJournalsTSV(journals []*journal.Journal) error {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', tabwriter.TabIndent)
	fmt.Fprintln(tw, "DATE\tINTENTIONS\tWINS\tLOG_ENTRIES\tSCHEDULE_ITEMS")
	for _, j := range journals {
		fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%d\n", j.Date, len(j.Intentions), len(j.Wins), len(j.LogEntries), len(j.ScheduleItems))
	}
	tw.Flush()
	return nil
}

func formatJournalsCSV(journals []*journal.Journal) error {
	w := csv.NewWriter(os.Stdout)
	w.Write([]string{"DATE", "INTENTIONS", "WINS", "LOG_ENTRIES", "SCHEDULE_ITEMS"})
	for _, j := range journals {
		record := []string{j.Date, fmt.Sprintf("%d", len(j.Intentions)), fmt.Sprintf("%d", len(j.Wins)), fmt.Sprintf("%d", len(j.LogEntries)), fmt.Sprintf("%d", len(j.ScheduleItems))}
		if err := w.Write(record); err != nil {
			return fmt.Errorf("failed to write CSV record: %w", err)
		}
	}
	w.Flush()
	return w.Error()
}
