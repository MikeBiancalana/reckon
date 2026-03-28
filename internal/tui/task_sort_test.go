package tui

import (
	"testing"
	stdtime "time"

	"github.com/MikeBiancalana/reckon/internal/journal"
)

// fixedMonday is a Monday at noon used as a stable "now" for tests.
// 2026-03-23 is a Monday.
var fixedMonday = stdtime.Date(2026, 3, 23, 12, 0, 0, 0, stdtime.UTC)

func strPtr(s string) *string { return &s }

func makeTask(id, text string, status journal.TaskStatus, createdAt stdtime.Time, deadline, scheduled *string) journal.Task {
	return journal.Task{
		ID:            id,
		Text:          text,
		Status:        status,
		CreatedAt:     createdAt,
		DeadlineDate:  deadline,
		ScheduledDate: scheduled,
	}
}

func TestSortTasksByPriority_Empty(t *testing.T) {
	result := SortTasksByPriority(nil, fixedMonday)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d tasks", len(result))
	}

	result = SortTasksByPriority([]journal.Task{}, fixedMonday)
	if len(result) != 0 {
		t.Errorf("expected empty result for empty slice, got %d tasks", len(result))
	}
}

func TestSortTasksByPriority_DoneTasksExcluded(t *testing.T) {
	tasks := []journal.Task{
		makeTask("done1", "Done task", journal.TaskDone, fixedMonday, nil, nil),
		makeTask("open1", "Open task", journal.TaskOpen, fixedMonday, nil, nil),
		makeTask("done2", "Another done", journal.TaskDone, fixedMonday, nil, nil),
	}

	result := SortTasksByPriority(tasks, fixedMonday)
	if len(result) != 1 {
		t.Fatalf("expected 1 task (done tasks excluded), got %d", len(result))
	}
	if result[0].ID != "open1" {
		t.Errorf("expected open1, got %s", result[0].ID)
	}
}

func TestSortTasksByPriority_AllDoneReturnsEmpty(t *testing.T) {
	tasks := []journal.Task{
		makeTask("d1", "Done", journal.TaskDone, fixedMonday, nil, nil),
	}
	result := SortTasksByPriority(tasks, fixedMonday)
	if len(result) != 0 {
		t.Errorf("expected empty, got %d", len(result))
	}
}

func TestSortTasksByPriority_Bucket0_OverdueDeadline(t *testing.T) {
	yesterday := fixedMonday.AddDate(0, 0, -1).Format("2006-01-02")
	nextWeek := fixedMonday.AddDate(0, 0, 7).Format("2006-01-02")

	tasks := []journal.Task{
		makeTask("future", "Future deadline", journal.TaskOpen, fixedMonday.Add(-2*stdtime.Hour), strPtr(nextWeek), nil),
		makeTask("overdue", "Overdue deadline", journal.TaskOpen, fixedMonday.Add(-1*stdtime.Hour), strPtr(yesterday), nil),
	}

	result := SortTasksByPriority(tasks, fixedMonday)
	if len(result) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(result))
	}
	if result[0].ID != "overdue" {
		t.Errorf("expected overdue first (bucket 0), got %s", result[0].ID)
	}
}

func TestSortTasksByPriority_Bucket1_DueToday(t *testing.T) {
	today := fixedMonday.Format("2006-01-02")
	nextWeek := fixedMonday.AddDate(0, 0, 7).Format("2006-01-02")

	tasks := []journal.Task{
		makeTask("future", "Future", journal.TaskOpen, fixedMonday.Add(-2*stdtime.Hour), strPtr(nextWeek), nil),
		makeTask("today", "Due today", journal.TaskOpen, fixedMonday.Add(-1*stdtime.Hour), strPtr(today), nil),
	}

	result := SortTasksByPriority(tasks, fixedMonday)
	if result[0].ID != "today" {
		t.Errorf("expected due-today first (bucket 1), got %s", result[0].ID)
	}
}

func TestSortTasksByPriority_Bucket1_ScheduledPast(t *testing.T) {
	yesterday := fixedMonday.AddDate(0, 0, -1).Format("2006-01-02")
	nextWeek := fixedMonday.AddDate(0, 0, 7).Format("2006-01-02")

	tasks := []journal.Task{
		makeTask("future", "Future scheduled", journal.TaskOpen, fixedMonday.Add(-2*stdtime.Hour), nil, strPtr(nextWeek)),
		makeTask("past-sched", "Past scheduled", journal.TaskOpen, fixedMonday.Add(-1*stdtime.Hour), nil, strPtr(yesterday)),
	}

	result := SortTasksByPriority(tasks, fixedMonday)
	if result[0].ID != "past-sched" {
		t.Errorf("expected past-scheduled first (bucket 1), got %s", result[0].ID)
	}
}

func TestSortTasksByPriority_Bucket1_ScheduledToday(t *testing.T) {
	today := fixedMonday.Format("2006-01-02")
	nextWeek := fixedMonday.AddDate(0, 0, 7).Format("2006-01-02")

	tasks := []journal.Task{
		makeTask("future", "Future scheduled", journal.TaskOpen, fixedMonday.Add(-2*stdtime.Hour), nil, strPtr(nextWeek)),
		makeTask("today-sched", "Scheduled today", journal.TaskOpen, fixedMonday.Add(-1*stdtime.Hour), nil, strPtr(today)),
	}

	result := SortTasksByPriority(tasks, fixedMonday)
	if result[0].ID != "today-sched" {
		t.Errorf("expected today-scheduled first (bucket 1), got %s", result[0].ID)
	}
}

func TestSortTasksByPriority_Bucket2_DeadlineThisWeek(t *testing.T) {
	// fixedMonday is 2026-03-23 (Monday). This week = Mon–Sun.
	wednesday := fixedMonday.AddDate(0, 0, 2).Format("2006-01-02") // same week
	nextWeek := fixedMonday.AddDate(0, 0, 7).Format("2006-01-02")

	tasks := []journal.Task{
		makeTask("next", "Next week", journal.TaskOpen, fixedMonday.Add(-2*stdtime.Hour), strPtr(nextWeek), nil),
		makeTask("week", "This week", journal.TaskOpen, fixedMonday.Add(-1*stdtime.Hour), strPtr(wednesday), nil),
	}

	result := SortTasksByPriority(tasks, fixedMonday)
	if result[0].ID != "week" {
		t.Errorf("expected this-week deadline first (bucket 2), got %s", result[0].ID)
	}
}

func TestSortTasksByPriority_Bucket2_ScheduledThisWeek(t *testing.T) {
	friday := fixedMonday.AddDate(0, 0, 4).Format("2006-01-02") // same week
	nextWeek := fixedMonday.AddDate(0, 0, 7).Format("2006-01-02")

	tasks := []journal.Task{
		makeTask("next", "Next week", journal.TaskOpen, fixedMonday.Add(-2*stdtime.Hour), nil, strPtr(nextWeek)),
		makeTask("week", "This week scheduled", journal.TaskOpen, fixedMonday.Add(-1*stdtime.Hour), nil, strPtr(friday)),
	}

	result := SortTasksByPriority(tasks, fixedMonday)
	if result[0].ID != "week" {
		t.Errorf("expected this-week-scheduled first (bucket 2), got %s", result[0].ID)
	}
}

func TestSortTasksByPriority_Bucket3_NoDates(t *testing.T) {
	t1 := fixedMonday.Add(-3 * stdtime.Hour)
	t2 := fixedMonday.Add(-2 * stdtime.Hour)
	t3 := fixedMonday.Add(-1 * stdtime.Hour)

	tasks := []journal.Task{
		makeTask("c", "Task C", journal.TaskOpen, t3, nil, nil),
		makeTask("a", "Task A", journal.TaskOpen, t1, nil, nil),
		makeTask("b", "Task B", journal.TaskOpen, t2, nil, nil),
	}

	result := SortTasksByPriority(tasks, fixedMonday)
	if len(result) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(result))
	}
	// Within bucket 3, sorted by CreatedAt ascending
	if result[0].ID != "a" || result[1].ID != "b" || result[2].ID != "c" {
		t.Errorf("expected a,b,c order within bucket 3; got %s,%s,%s",
			result[0].ID, result[1].ID, result[2].ID)
	}
}

func TestSortTasksByPriority_StableWithinBucket(t *testing.T) {
	// Same created-at but different IDs — order should be stable (insertion order)
	same := fixedMonday.Add(-1 * stdtime.Hour)

	tasks := []journal.Task{
		makeTask("x", "X", journal.TaskOpen, same, nil, nil),
		makeTask("y", "Y", journal.TaskOpen, same, nil, nil),
		makeTask("z", "Z", journal.TaskOpen, same, nil, nil),
	}

	result := SortTasksByPriority(tasks, fixedMonday)
	if result[0].ID != "x" || result[1].ID != "y" || result[2].ID != "z" {
		t.Errorf("expected stable order x,y,z; got %s,%s,%s",
			result[0].ID, result[1].ID, result[2].ID)
	}
}

func TestSortTasksByPriority_FullBucketOrder(t *testing.T) {
	// Verify correct ordering across all four buckets
	yesterday := fixedMonday.AddDate(0, 0, -1).Format("2006-01-02")
	today := fixedMonday.Format("2006-01-02")
	wednesday := fixedMonday.AddDate(0, 0, 2).Format("2006-01-02")

	t1 := fixedMonday.Add(-4 * stdtime.Hour)
	t2 := fixedMonday.Add(-3 * stdtime.Hour)
	t3 := fixedMonday.Add(-2 * stdtime.Hour)
	t4 := fixedMonday.Add(-1 * stdtime.Hour)

	tasks := []journal.Task{
		makeTask("bucket3", "No dates", journal.TaskOpen, t4, nil, nil),
		makeTask("bucket1", "Due today", journal.TaskOpen, t2, strPtr(today), nil),
		makeTask("bucket0", "Overdue", journal.TaskOpen, t1, strPtr(yesterday), nil),
		makeTask("bucket2", "This week", journal.TaskOpen, t3, strPtr(wednesday), nil),
	}

	result := SortTasksByPriority(tasks, fixedMonday)
	if len(result) != 4 {
		t.Fatalf("expected 4 tasks, got %d", len(result))
	}
	expected := []string{"bucket0", "bucket1", "bucket2", "bucket3"}
	for i, id := range expected {
		if result[i].ID != id {
			t.Errorf("position %d: expected %s, got %s", i, id, result[i].ID)
		}
	}
}

func TestSortTasksByPriority_BothDeadlineAndSchedule(t *testing.T) {
	// Task with a this-week scheduled date but overdue deadline → bucket 0
	yesterday := fixedMonday.AddDate(0, 0, -1).Format("2006-01-02")
	wednesday := fixedMonday.AddDate(0, 0, 2).Format("2006-01-02")

	tasks := []journal.Task{
		makeTask("t1", "Overdue deadline + this-week scheduled", journal.TaskOpen, fixedMonday,
			strPtr(yesterday), strPtr(wednesday)),
		makeTask("t2", "No dates", journal.TaskOpen, fixedMonday.Add(-stdtime.Hour), nil, nil),
	}

	result := SortTasksByPriority(tasks, fixedMonday)
	if result[0].ID != "t1" {
		t.Errorf("expected overdue deadline task first (bucket 0), got %s", result[0].ID)
	}
}
