package tui

import (
	"sort"
	stdtime "time"

	"github.com/MikeBiancalana/reckon/internal/journal"
)

// SortTasksByPriority returns open tasks sorted by urgency.
//
// Buckets (lower = more urgent):
//
//	0: overdue deadline (deadline < today)
//	1: due today (deadline == today) OR scheduled today or past (scheduled <= today)
//	2: due/scheduled this week
//	3: everything else
//
// Done tasks are excluded. Tasks within a bucket are stable-sorted by CreatedAt ascending.
// now is the reference time used for date calculations (use time.Now() in production).
func SortTasksByPriority(tasks []journal.Task, now stdtime.Time) []journal.Task {
	today := now.Truncate(24 * stdtime.Hour)
	weekday := now.Weekday()
	if weekday == stdtime.Sunday {
		weekday = 7
	}
	weekStart := today.AddDate(0, 0, -int(weekday-stdtime.Monday))
	weekEnd := weekStart.AddDate(0, 0, 7)

	type indexed struct {
		task   journal.Task
		bucket int
	}

	var items []indexed
	for _, task := range tasks {
		if task.Status == journal.TaskDone {
			continue
		}
		items = append(items, indexed{task: task, bucket: taskBucket(task, today, weekStart, weekEnd)})
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].bucket != items[j].bucket {
			return items[i].bucket < items[j].bucket
		}
		return items[i].task.CreatedAt.Before(items[j].task.CreatedAt)
	})

	result := make([]journal.Task, len(items))
	for i, item := range items {
		result[i] = item.task
	}
	return result
}

// taskBucket returns the urgency bucket (0–3) for a task.
func taskBucket(task journal.Task, today, weekStart, weekEnd stdtime.Time) int {
	// Bucket 0: overdue deadline
	if d, ok := parseTaskDate(task.DeadlineDate); ok {
		if d.Before(today) {
			return 0
		}
	}

	// Bucket 1: due today OR scheduled today/past
	if d, ok := parseTaskDate(task.DeadlineDate); ok {
		if d.Equal(today) {
			return 1
		}
	}
	if s, ok := parseTaskDate(task.ScheduledDate); ok {
		if s.Equal(today) || s.Before(today) {
			return 1
		}
	}

	// Bucket 2: due/scheduled this week
	if d, ok := parseTaskDate(task.DeadlineDate); ok {
		if inWeek(d, weekStart, weekEnd) {
			return 2
		}
	}
	if s, ok := parseTaskDate(task.ScheduledDate); ok {
		if inWeek(s, weekStart, weekEnd) {
			return 2
		}
	}

	return 3
}

func inWeek(t, weekStart, weekEnd stdtime.Time) bool {
	return (t.After(weekStart) || t.Equal(weekStart)) && t.Before(weekEnd)
}

// parseTaskDate parses a YYYY-MM-DD date string pointer.
func parseTaskDate(dateStr *string) (stdtime.Time, bool) {
	if dateStr == nil || *dateStr == "" {
		return stdtime.Time{}, false
	}
	t, err := stdtime.Parse("2006-01-02", *dateStr)
	if err != nil {
		return stdtime.Time{}, false
	}
	return t, true
}
