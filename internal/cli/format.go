package cli

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/MikeBiancalana/reckon/internal/models"
)

type OutputFormat string

const (
	FormatJSON OutputFormat = "json"
	FormatTSV  OutputFormat = "tsv"
	FormatCSV  OutputFormat = "csv"
)

func parseFormat(s string) (OutputFormat, error) {
	switch strings.ToLower(s) {
	case string(FormatJSON):
		return FormatJSON, nil
	case string(FormatTSV):
		return FormatTSV, nil
	case string(FormatCSV):
		return FormatCSV, nil
	default:
		return "", fmt.Errorf("unsupported format: %s (supported: json, tsv, csv)", s)
	}
}

func csvQuote(field string) string {
	return fmt.Sprintf("\"%s\"", strings.ReplaceAll(field, "\"", "\"\""))
}

func csvEscapeField(field string) string {
	if strings.HasPrefix(field, "=") ||
		strings.HasPrefix(field, "+") ||
		strings.HasPrefix(field, "-") ||
		strings.HasPrefix(field, "@") {
		return "'" + field
	}
	return field
}

func formatTasksJSON(tasks []journal.Task) error {
	return json.NewEncoder(os.Stdout).Encode(tasks)
}

func formatTasksTSV(tasks []journal.Task) error {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', tabwriter.TabIndent)
	if _, err := fmt.Fprintln(tw, "ID\tSTATUS\tCREATED\tTAGS\tTITLE"); err != nil {
		return err
	}
	for _, t := range tasks {
		tags := "-"
		if len(t.Tags) > 0 {
			tags = strings.Join(t.Tags, ", ")
		}
		if _, err := fmt.Fprintf(tw, "%.8s\t%s\t%s\t%s\t%s\n", t.ID, t.Status, t.CreatedAt.Format("2006-01-02"), tags, csvEscapeField(t.Text)); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func formatTasksCSV(tasks []journal.Task) error {
	w := csv.NewWriter(os.Stdout)
	if err := w.Write([]string{"ID", "STATUS", "CREATED", "TAGS", "TITLE"}); err != nil {
		return err
	}
	for _, t := range tasks {
		tags := ""
		if len(t.Tags) > 0 {
			tags = strings.Join(t.Tags, ", ")
		}
		record := []string{t.ID, string(t.Status), t.CreatedAt.Format("2006-01-02"), tags, t.Text}
		if err := w.Write(record); err != nil {
			return err
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
	if _, err := fmt.Fprintln(tw, "ID\tTIME\tCONTENT"); err != nil {
		return err
	}
	for _, item := range items {
		timeStr := "-"
		if !item.Time.IsZero() {
			timeStr = item.Time.Format("15:04")
		}
		if _, err := fmt.Fprintf(tw, "%.8s\t%s\t%s\n", item.ID, timeStr, csvEscapeField(item.Content)); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func formatScheduleItemsCSV(items []journal.ScheduleItem) error {
	w := csv.NewWriter(os.Stdout)
	if err := w.Write([]string{"ID", "TIME", "CONTENT"}); err != nil {
		return err
	}
	for _, item := range items {
		timeStr := ""
		if !item.Time.IsZero() {
			timeStr = item.Time.Format("15:04")
		}
		record := []string{item.ID, timeStr, item.Content}
		if err := w.Write(record); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func formatWinsJSON(wins []journal.Win) error {
	return json.NewEncoder(os.Stdout).Encode(wins)
}

func formatWinsTSV(wins []journal.Win) error {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.TabIndent)
	if _, err := fmt.Fprintln(tw, "ID\tTEXT"); err != nil {
		return err
	}
	for _, w := range wins {
		if _, err := fmt.Fprintf(tw, "%.8s\t%s\n", w.ID, csvEscapeField(w.Text)); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func formatWinsCSV(wins []journal.Win) error {
	w := csv.NewWriter(os.Stdout)
	if err := w.Write([]string{"ID", "TEXT"}); err != nil {
		return err
	}
	for _, win := range wins {
		record := []string{win.ID, win.Text}
		if err := w.Write(record); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func formatJournalsJSON(journals []*journal.Journal) error {
	return json.NewEncoder(os.Stdout).Encode(journals)
}

func formatJournalTSV(j *journal.Journal) error {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.TabIndent)
	if _, err := fmt.Fprintf(tw, "DATE\t%s\n", j.Date); err != nil {
		return err
	}
	return tw.Flush()
}

func formatJournalsTSV(journals []*journal.Journal) error {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.TabIndent)
	if _, err := fmt.Fprintln(tw, "DATE"); err != nil {
		return err
	}
	for _, j := range journals {
		if _, err := fmt.Fprintf(tw, "%s\n", j.Date); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func formatJournalsCSV(journals []*journal.Journal) error {
	w := csv.NewWriter(os.Stdout)
	if err := w.Write([]string{"DATE"}); err != nil {
		return err
	}
	for _, j := range journals {
		record := []string{j.Date}
		if err := w.Write(record); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func formatNotesJSON(notes []*models.Note) error {
	return json.NewEncoder(os.Stdout).Encode(notes)
}

func formatNotesCSV(notes []*models.Note) error {
	w := csv.NewWriter(os.Stdout)
	if err := w.Write([]string{"title", "slug", "created", "updated", "tags"}); err != nil {
		return err
	}
	for _, n := range notes {
		tags := ""
		if len(n.Tags) > 0 {
			tags = strings.Join(n.Tags, ", ")
		}
		record := []string{
			n.Title,
			n.Slug,
			n.CreatedAt.Format("2006-01-02"),
			n.UpdatedAt.Format("2006-01-02"),
			tags,
		}
		if err := w.Write(record); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}
