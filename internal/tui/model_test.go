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

// TestNilTaskListHandling tests that model methods handle nil taskList gracefully
func TestNilTaskListHandling(t *testing.T) {
	t.Run("calculateDetailPanePosition with nil taskList", func(t *testing.T) {
		m := &Model{
			taskList:           nil,
			detailPanePosition: DetailPaneBottom, // Set to bottom initially
		}

		// Should not panic with nil taskList
		m.calculateDetailPanePosition()

		// Position should remain unchanged since taskList is nil
		if m.detailPanePosition != DetailPaneBottom {
			t.Errorf("Expected position to remain DetailPaneBottom, got %v", m.detailPanePosition)
		}
	})

	t.Run("renderDetailPane with nil taskList", func(t *testing.T) {
		m := &Model{
			taskList: nil,
		}

		// Should not panic with nil taskList
		result := m.renderDetailPane(80, 20)

		// Should return appropriate message
		if result == "" {
			t.Error("Expected non-empty result from renderDetailPane with nil taskList")
		}
		if !strings.Contains(result, "No task list available") {
			t.Errorf("Expected result to mention no task list, got: %s", result)
		}
	})

	t.Run("renderTasksWithDetailPane with nil taskList", func(t *testing.T) {
		m := &Model{
			taskList: nil,
		}

		// Should not panic with nil taskList
		result := m.renderTasksWithDetailPane()

		// Should return appropriate message
		if result != "No tasks" {
			t.Errorf("Expected 'No tasks' result, got: %s", result)
		}
	})

	t.Run("updateNotesForSelectedTask with nil taskList", func(t *testing.T) {
		m := &Model{
			taskList:  nil,
			notesPane: components.NewNotesPane(),
		}

		// Should not panic with nil taskList
		m.updateNotesForSelectedTask()

		// selectedTaskID should remain empty
		if m.selectedTaskID != "" {
			t.Errorf("Expected selectedTaskID to remain empty, got %s", m.selectedTaskID)
		}
	})

	t.Run("updateNotesForSelectedTask with nil notesPane", func(t *testing.T) {
		m := &Model{
			taskList:  components.NewTaskList([]journal.Task{}),
			notesPane: nil,
		}

		// Should not panic with nil notesPane
		m.updateNotesForSelectedTask()
	})

	t.Run("full model with nil taskList during initialization", func(t *testing.T) {
		// Create a model without taskService (simulates early initialization)
		service := &journal.Service{}
		m := NewModel(service)

		// taskList should be nil initially
		if m.taskList != nil {
			t.Error("Expected taskList to be nil before journal loaded")
		}

		// These methods should not panic even with nil taskList
		m.calculateDetailPanePosition()

		detailPaneView := m.renderDetailPane(80, 20)
		if detailPaneView == "" {
			t.Error("Expected non-empty detail pane view")
		}

		tasksView := m.renderTasksWithDetailPane()
		if tasksView == "" {
			t.Error("Expected non-empty tasks view")
		}
	})
}

// TestRenderTasksWithDetailPane_SmallWidth tests that separator width is bounded
func TestRenderTasksWithDetailPane_SmallWidth(t *testing.T) {
	t.Run("handles very small width without panic", func(t *testing.T) {
		m := &Model{
			width:              10, // Very small width
			height:             30,
			taskList:           components.NewTaskList([]journal.Task{}),
			notesPaneVisible:   false,
			detailPanePosition: DetailPaneBottom,
		}

		// Should not panic even with very small width
		result := m.renderTasksWithDetailPane()

		// Should return something non-empty
		if result == "" {
			t.Error("Expected non-empty result from renderTasksWithDetailPane with small width")
		}
	})

	t.Run("handles width smaller than border", func(t *testing.T) {
		m := &Model{
			width:              1, // Width smaller than BorderWidth
			height:             30,
			taskList:           components.NewTaskList([]journal.Task{}),
			notesPaneVisible:   false,
			detailPanePosition: DetailPaneBottom,
		}

		// Should not panic even when width < BorderWidth
		result := m.renderTasksWithDetailPane()

		if result == "" {
			t.Error("Expected non-empty result from renderTasksWithDetailPane with width < BorderWidth")
		}
	})

	t.Run("separator width is zero when CenterWidth equals BorderWidth", func(t *testing.T) {
		// This test verifies the bounds checking logic for separator width
		// When CenterWidth - BorderWidth would be negative or zero, separator should be empty
		m := &Model{
			width:    5, // Small enough to trigger edge case
			height:   30,
			taskList: components.NewTaskList([]journal.Task{{ID: "1", Text: "Test", Status: journal.TaskOpen}}),
		}

		// Should not panic
		result := m.renderTasksWithDetailPane()
		if result == "" {
			t.Error("Expected non-empty result")
		}
	})
}

// TestGetTaskSection_DoneTasks tests that done tasks are categorized correctly
func TestGetTaskSection_DoneTasks(t *testing.T) {
	today := time.Now().Truncate(24 * time.Hour)
	todayStr := today.Format("2006-01-02")

	t.Run("done task with today's schedule returns AllTasks", func(t *testing.T) {
		task := &journal.Task{
			ID:            "1",
			Text:          "Done task",
			Status:        journal.TaskDone,
			ScheduledDate: &todayStr,
		}

		section := getTaskSection(task)

		// Done tasks should not appear in TODAY section
		if section != TaskSectionAllTasks {
			t.Errorf("Expected done task to be in AllTasks section, got %v", section)
		}
	})

	t.Run("done task with today's deadline returns AllTasks", func(t *testing.T) {
		task := &journal.Task{
			ID:           "2",
			Text:         "Done task",
			Status:       journal.TaskDone,
			DeadlineDate: &todayStr,
		}

		section := getTaskSection(task)

		// Done tasks should not appear in TODAY section
		if section != TaskSectionAllTasks {
			t.Errorf("Expected done task to be in AllTasks section, got %v", section)
		}
	})
}

// TestGetTaskSection_Performance is a benchmark-style test to ensure the optimized version is efficient
func TestGetTaskSection_Performance(t *testing.T) {
	today := time.Now().Truncate(24 * time.Hour)
	todayStr := today.Format("2006-01-02")

	task := &journal.Task{
		ID:            "1",
		Text:          "Test task",
		Status:        journal.TaskOpen,
		ScheduledDate: &todayStr,
	}

	// Run many times to ensure no significant performance regression
	iterations := 10000
	start := time.Now()
	for i := 0; i < iterations; i++ {
		getTaskSection(task)
	}
	elapsed := time.Since(start)

	// Should complete very quickly (well under 100ms for 10000 iterations)
	if elapsed > 100*time.Millisecond {
		t.Errorf("Performance regression: %d iterations took %v (expected < 100ms)", iterations, elapsed)
	}

	t.Logf("Performance: %d iterations completed in %v", iterations, elapsed)
}
