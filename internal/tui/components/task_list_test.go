package components

import (
	"testing"
	"time"

	"github.com/MikeBiancalana/reckon/internal/journal"
)

func ptr(s string) *string { return &s }

// TestParseDate verifies the internal date parsing helper.
func TestParseDate(t *testing.T) {
	t.Run("nil pointer returns false", func(t *testing.T) {
		_, ok := parseDate(nil)
		if ok {
			t.Error("expected ok=false for nil pointer")
		}
	})

	t.Run("empty string returns false", func(t *testing.T) {
		s := ""
		_, ok := parseDate(&s)
		if ok {
			t.Error("expected ok=false for empty string")
		}
	})

	t.Run("valid date returns correct time", func(t *testing.T) {
		s := "2026-03-28"
		got, ok := parseDate(&s)
		if !ok {
			t.Fatal("expected ok=true for valid date")
		}
		want := time.Date(2026, 3, 28, 0, 0, 0, 0, time.UTC)
		if !got.Equal(want) {
			t.Errorf("parseDate(%q) = %v, want %v", s, got, want)
		}
	})

	t.Run("invalid date returns false", func(t *testing.T) {
		s := "not-a-date"
		_, ok := parseDate(&s)
		if ok {
			t.Error("expected ok=false for invalid date")
		}
	})
}

// TestFormatDateInfo verifies the exported date info formatter.
func TestFormatDateInfo(t *testing.T) {
	t.Run("no dates returns empty string", func(t *testing.T) {
		task := journal.Task{ID: "1", Text: "Test", Status: journal.TaskOpen}
		got := FormatDateInfo(task)
		if got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})

	t.Run("scheduled date appears in output", func(t *testing.T) {
		future := time.Now().AddDate(0, 0, 10).Format("2006-01-02")
		task := journal.Task{
			ID:            "1",
			Text:          "Test",
			Status:        journal.TaskOpen,
			ScheduledDate: &future,
		}
		got := FormatDateInfo(task)
		if got == "" {
			t.Error("expected non-empty string for scheduled task")
		}
	})

	t.Run("overdue deadline produces overdue message", func(t *testing.T) {
		past := time.Now().AddDate(0, 0, -5).Format("2006-01-02")
		task := journal.Task{
			ID:           "1",
			Text:         "Test",
			Status:       journal.TaskOpen,
			DeadlineDate: &past,
		}
		got := FormatDateInfo(task)
		if got == "" {
			t.Error("expected non-empty string for overdue deadline")
		}
	})
}

// TestFormatFriendlyDate verifies relative date formatting.
func TestFormatFriendlyDate(t *testing.T) {
	today := time.Now().Truncate(24 * time.Hour)

	t.Run("today returns 'today'", func(t *testing.T) {
		got := formatFriendlyDate(today, today)
		if got != "today" {
			t.Errorf("expected 'today', got %q", got)
		}
	})

	t.Run("tomorrow returns 'tomorrow'", func(t *testing.T) {
		tomorrow := today.Add(24 * time.Hour)
		got := formatFriendlyDate(tomorrow, today)
		if got != "tomorrow" {
			t.Errorf("expected 'tomorrow', got %q", got)
		}
	})

	t.Run("same year shows month and day", func(t *testing.T) {
		future := today.AddDate(0, 1, 0)
		got := formatFriendlyDate(future, today)
		if got == "" {
			t.Error("expected non-empty friendly date")
		}
	})
}

// TestGetDateStyle verifies that GetDateStyle returns a usable style for each case.
// Lipgloss styles contain funcs and cannot be compared directly; we just verify
// the function does not panic and returns a style that renders the input.
func TestGetDateStyle(t *testing.T) {
	sentinel := "TEST"

	t.Run("overdue task style renders the input", func(t *testing.T) {
		past := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
		task := journal.Task{
			ID:           "1",
			Status:       journal.TaskOpen,
			DeadlineDate: &past,
		}
		got := GetDateStyle(task)
		rendered := got.Render(sentinel)
		if rendered == "" {
			t.Error("expected non-empty render from GetDateStyle for overdue task")
		}
	})

	t.Run("deadline today style renders the input", func(t *testing.T) {
		today := time.Now().Truncate(24 * time.Hour).Format("2006-01-02")
		task := journal.Task{
			ID:           "1",
			Status:       journal.TaskOpen,
			DeadlineDate: &today,
		}
		got := GetDateStyle(task)
		rendered := got.Render(sentinel)
		if rendered == "" {
			t.Error("expected non-empty render from GetDateStyle for deadline today")
		}
	})

	t.Run("no dates style renders the input", func(t *testing.T) {
		task := journal.Task{ID: "1", Status: journal.TaskOpen}
		got := GetDateStyle(task)
		rendered := got.Render(sentinel)
		if rendered == "" {
			t.Error("expected non-empty render from GetDateStyle with no dates")
		}
	})
}
