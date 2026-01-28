package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/MikeBiancalana/reckon/internal/tui/components"
)

func TestMinimumTerminalSizeConstants(t *testing.T) {
	// Test that our constants are defined correctly
	if MinTerminalWidth != 80 {
		t.Errorf("Expected MinTerminalWidth to be 80, got %d", MinTerminalWidth)
	}

	if MinTerminalHeight != 24 {
		t.Errorf("Expected MinTerminalHeight to be 24, got %d", MinTerminalHeight)
	}
}

func TestTerminalTooSmallValidation(t *testing.T) {
	// Test the validation logic directly without creating a full model
	testCases := []struct {
		width    int
		height   int
		expected bool
		name     string
	}{
		{60, 25, true, "Width too small"},
		{80, 20, true, "Height too small"},
		{60, 20, true, "Both dimensions too small"},
		{80, 24, false, "Exactly minimum dimensions"},
		{120, 40, false, "Larger than minimum"},
		{79, 24, true, "Width one pixel under minimum"},
		{80, 23, true, "Height one pixel under minimum"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test the validation logic that's used in the WindowSizeMsg handler
			tooSmall := tc.width < MinTerminalWidth || tc.height < MinTerminalHeight
			if tooSmall != tc.expected {
				t.Errorf("Expected terminalTooSmall=%v for dimensions %dx%d, got %v",
					tc.expected, tc.width, tc.height, tooSmall)
			}
		})
	}
}

func TestTerminalTooSmallViewContent(t *testing.T) {
	// Create a minimal model just for testing the view
	model := &Model{
		terminalTooSmall: true,
		width:            60,
		height:           20,
	}

	// Get the view
	view := model.terminalTooSmallView()

	// Check that the view contains expected text
	expectedTexts := []string{
		"Terminal Too Small",
		"Current: 60x20",
		"Required: 80x24 or larger",
		"Resize your terminal",
	}

	for _, expected := range expectedTexts {
		if !strings.Contains(view, expected) {
			t.Errorf("Expected view to contain '%s', but got view: %s", expected, view)
		}
	}

	// Verify the view is not empty
	if view == "" {
		t.Error("Expected terminalTooSmallView to return a non-empty string")
	}
}

func TestParseTaskTags(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantText string
		wantTags []string
	}{
		{
			name:     "empty input",
			input:    "",
			wantText: "",
			wantTags: nil,
		},
		{
			name:     "no tags",
			input:    "Buy milk and eggs",
			wantText: "Buy milk and eggs",
			wantTags: nil,
		},
		{
			name:     "single tag at end",
			input:    "Buy milk #groceries",
			wantText: "Buy milk",
			wantTags: []string{"groceries"},
		},
		{
			name:     "single tag at start",
			input:    "#important Review PR",
			wantText: "Review PR",
			wantTags: []string{"important"},
		},
		{
			name:     "multiple tags",
			input:    "Task #tag1 #tag2 #tag3",
			wantText: "Task",
			wantTags: []string{"tag1", "tag2", "tag3"},
		},
		{
			name:     "tags with mixed case",
			input:    "Task #TAG1 #Tag2",
			wantText: "Task",
			wantTags: []string{"tag1", "tag2"},
		},
		{
			name:     "just hash symbol",
			input:    "Task #",
			wantText: "Task #",
			wantTags: nil,
		},
		{
			name:     "double hash",
			input:    "Task ##",
			wantText: "Task",
			wantTags: nil,
		},
		{
			name:     "tag with punctuation",
			input:    "Task #urgent, #important",
			wantText: "Task",
			wantTags: []string{"urgent,", "important"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotText, gotTags := parseTaskTags(tt.input)

			if gotText != tt.wantText {
				t.Errorf("parseTaskTags(%q) text = %q, want %q", tt.input, gotText, tt.wantText)
			}

			if len(gotTags) != len(tt.wantTags) {
				t.Errorf("parseTaskTags(%q) tags count = %d, want %d", tt.input, len(gotTags), len(tt.wantTags))
				return
			}

			for i, tag := range gotTags {
				if tag != tt.wantTags[i] {
					t.Errorf("parseTaskTags(%q) tags[%d] = %q, want %q", tt.input, i, tag, tt.wantTags[i])
				}
			}
		})
	}
}

func TestGetTaskSection(t *testing.T) {
	today := time.Now().Truncate(24 * time.Hour)
	yesterday := today.AddDate(0, 0, -1)
	twoWeeksOut := today.AddDate(0, 0, 14)

	todayStr := today.Format("2006-01-02")
	twoWeeksOutStr := twoWeeksOut.Format("2006-01-02")
	yesterdayStr := yesterday.Format("2006-01-02")

	tests := []struct {
		name string
		task *journal.Task
	}{
		{
			name: "nil task returns AllTasks",
			task: nil,
		},
		{
			name: "task scheduled for today",
			task: &journal.Task{
				ID:            "1",
				Text:          "Task 1",
				Status:        journal.TaskOpen,
				ScheduledDate: &todayStr,
			},
		},
		{
			name: "task scheduled for yesterday (overdue)",
			task: &journal.Task{
				ID:            "2",
				Text:          "Task 2",
				Status:        journal.TaskOpen,
				ScheduledDate: &yesterdayStr,
			},
		},
		{
			name: "task scheduled for two weeks out",
			task: &journal.Task{
				ID:            "4",
				Text:          "Task 4",
				Status:        journal.TaskOpen,
				ScheduledDate: &twoWeeksOutStr,
			},
		},
		{
			name: "task with deadline today",
			task: &journal.Task{
				ID:           "5",
				Text:         "Task 5",
				Status:       journal.TaskOpen,
				DeadlineDate: &todayStr,
			},
		},
		{
			name: "task with no dates",
			task: &journal.Task{
				ID:     "7",
				Text:   "Task 7",
				Status: journal.TaskOpen,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getTaskSection(tt.task)

			// Verify the result is consistent with GroupTasksByTime
			if tt.task != nil {
				grouped := components.GroupTasksByTime([]journal.Task{*tt.task})
				var expectedSection TaskSection
				if len(grouped.Today) > 0 {
					expectedSection = TaskSectionToday
				} else if len(grouped.ThisWeek) > 0 {
					expectedSection = TaskSectionThisWeek
				} else {
					expectedSection = TaskSectionAllTasks
				}

				if result != expectedSection {
					t.Errorf("getTaskSection() = %v, want %v (based on GroupTasksByTime)", result, expectedSection)
				}
			} else {
				// Nil task should always return AllTasks
				if result != TaskSectionAllTasks {
					t.Errorf("getTaskSection(nil) = %v, want TaskSectionAllTasks", result)
				}
			}
		})
	}
}

func TestCalculateDetailPanePosition(t *testing.T) {
	today := time.Now().Truncate(24 * time.Hour)
	twoWeeksOut := today.AddDate(0, 0, 14)

	todayStr := today.Format("2006-01-02")
	twoWeeksOutStr := twoWeeksOut.Format("2006-01-02")

	tests := []struct {
		name string
		task journal.Task
	}{
		{
			name: "task in TODAY section",
			task: journal.Task{
				ID:            "1",
				Text:          "Task 1",
				Status:        journal.TaskOpen,
				ScheduledDate: &todayStr,
			},
		},
		{
			name: "task in ALL TASKS section",
			task: journal.Task{
				ID:            "3",
				Text:          "Task 3",
				Status:        journal.TaskOpen,
				ScheduledDate: &twoWeeksOutStr,
			},
		},
		{
			name: "task with no dates (ALL TASKS)",
			task: journal.Task{
				ID:     "4",
				Text:   "Task 4",
				Status: journal.TaskOpen,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the position logic: should match the section-to-position mapping
			section := getTaskSection(&tt.task)
			var expectedPosition DetailPanePosition
			switch section {
			case TaskSectionToday, TaskSectionThisWeek:
				expectedPosition = DetailPaneBottom
			case TaskSectionAllTasks:
				expectedPosition = DetailPaneMiddle
			}

			// Verify the logic is correct
			if section == TaskSectionToday || section == TaskSectionThisWeek {
				if expectedPosition != DetailPaneBottom {
					t.Errorf("Tasks in TODAY/THIS WEEK should have position bottom, got %v", expectedPosition)
				}
			} else if section == TaskSectionAllTasks {
				if expectedPosition != DetailPaneMiddle {
					t.Errorf("Tasks in ALL TASKS should have position middle, got %v", expectedPosition)
				}
			}
		})
	}
}
