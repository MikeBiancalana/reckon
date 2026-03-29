package components

import (
	"strings"
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

	t.Run("valid date returns correct time in local zone", func(t *testing.T) {
		s := "2026-03-28"
		got, ok := parseDate(&s)
		if !ok {
			t.Fatal("expected ok=true for valid date")
		}
		want := time.Date(2026, 3, 28, 0, 0, 0, 0, time.Local)
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

// localTestToday returns local midnight, consistent with localToday() in production code.
func localTestToday() time.Time {
	now := time.Now()
	y, m, d := now.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, now.Location())
}

// TestFormatFriendlyDate verifies relative date formatting.
func TestFormatFriendlyDate(t *testing.T) {
	today := localTestToday()

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

// TestFormatDateInfoFormat verifies the exact format strings produced by FormatDateInfo.
// These tests document the display contract for the TUI task list date indicators.
func TestFormatDateInfoFormat(t *testing.T) {
	today := localTestToday()

	t.Run("scheduled future uses calendar emoji prefix", func(t *testing.T) {
		future := today.AddDate(0, 0, 10).Format("2006-01-02")
		task := journal.Task{ID: "1", Status: journal.TaskOpen, ScheduledDate: &future}
		got := FormatDateInfo(task)
		if !strings.HasPrefix(got, "📅 ") {
			t.Errorf("scheduled date should start with '📅 ', got %q", got)
		}
	})

	t.Run("overdue deadline uses red emoji prefix", func(t *testing.T) {
		past := today.AddDate(0, 0, -3).Format("2006-01-02")
		task := journal.Task{ID: "1", Status: journal.TaskOpen, DeadlineDate: &past}
		got := FormatDateInfo(task)
		if !strings.HasPrefix(got, "🔴 overdue (due ") {
			t.Errorf("overdue deadline should start with '🔴 overdue (due ', got %q", got)
		}
		if !strings.HasSuffix(got, ")") {
			t.Errorf("overdue deadline should end with ')', got %q", got)
		}
	})

	t.Run("deadline today shows 'due today' with yellow emoji", func(t *testing.T) {
		todayStr := today.Format("2006-01-02")
		task := journal.Task{ID: "1", Status: journal.TaskOpen, DeadlineDate: &todayStr}
		got := FormatDateInfo(task)
		if got != "due today 🟡" {
			t.Errorf("deadline today should be %q, got %q", "due today 🟡", got)
		}
	})

	t.Run("deadline tomorrow shows 'due tomorrow' with yellow emoji", func(t *testing.T) {
		tomorrow := today.AddDate(0, 0, 1).Format("2006-01-02")
		task := journal.Task{ID: "1", Status: journal.TaskOpen, DeadlineDate: &tomorrow}
		got := FormatDateInfo(task)
		if got != "due tomorrow 🟡" {
			t.Errorf("deadline tomorrow should be %q, got %q", "due tomorrow 🟡", got)
		}
	})

	t.Run("deadline in 2 days shows date with yellow emoji", func(t *testing.T) {
		in2Days := today.AddDate(0, 0, 2).Format("2006-01-02")
		task := journal.Task{ID: "1", Status: journal.TaskOpen, DeadlineDate: &in2Days}
		got := FormatDateInfo(task)
		if !strings.HasPrefix(got, "due ") {
			t.Errorf("deadline due soon should start with 'due ', got %q", got)
		}
		if !strings.HasSuffix(got, " 🟡") {
			t.Errorf("deadline due soon should end with ' 🟡', got %q", got)
		}
	})

	t.Run("deadline far future shows date without urgency emoji", func(t *testing.T) {
		future := today.AddDate(0, 0, 10).Format("2006-01-02")
		task := journal.Task{ID: "1", Status: journal.TaskOpen, DeadlineDate: &future}
		got := FormatDateInfo(task)
		if !strings.HasPrefix(got, "due ") {
			t.Errorf("far future deadline should start with 'due ', got %q", got)
		}
		if strings.Contains(got, "🟡") || strings.Contains(got, "🔴") {
			t.Errorf("far future deadline should not have urgency emoji, got %q", got)
		}
	})

	t.Run("both scheduled and deadline joined with two spaces", func(t *testing.T) {
		scheduled := today.AddDate(0, 0, 5).Format("2006-01-02")
		deadline := today.AddDate(0, 0, 10).Format("2006-01-02")
		task := journal.Task{
			ID:            "1",
			Status:        journal.TaskOpen,
			ScheduledDate: &scheduled,
			DeadlineDate:  &deadline,
		}
		got := FormatDateInfo(task)
		if !strings.HasPrefix(got, "📅 ") {
			t.Errorf("combined output should start with scheduled indicator, got %q", got)
		}
		if !strings.Contains(got, "  ") {
			t.Errorf("combined output should have double-space separator, got %q", got)
		}
		if !strings.Contains(got, "due ") {
			t.Errorf("combined output should contain deadline, got %q", got)
		}
	})

	t.Run("done task still formats dates (filtering is caller's responsibility)", func(t *testing.T) {
		future := today.AddDate(0, 0, 10).Format("2006-01-02")
		task := journal.Task{ID: "1", Status: journal.TaskDone, ScheduledDate: &future}
		got := FormatDateInfo(task)
		// FormatDateInfo does not filter by status — model.go skips done tasks
		if got == "" {
			t.Error("FormatDateInfo should format dates regardless of task status")
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
		today := localTestToday().Format("2006-01-02")
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
