package tui

import (
	"testing"
)

// TestCalculatePaneDimensions_StandardSizes tests common terminal sizes
func TestCalculatePaneDimensions_StandardSizes(t *testing.T) {
	tests := []struct {
		name       string
		termWidth  int
		termHeight int
	}{
		{"Standard terminal 80x24", 80, 24},
		{"Common terminal 120x30", 120, 30},
		{"Large terminal 160x40", 160, 40},
		{"Wide terminal 200x50", 200, 50},
		{"HD terminal 1920x1080", 1920, 1080},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dims := CalculatePaneDimensions(tt.termWidth, tt.termHeight, false)

			// Verify basic properties
			if dims.TextEntryHeight != 3 {
				t.Errorf("Expected TextEntryHeight=3, got %d", dims.TextEntryHeight)
			}
			if dims.SummaryHeight != 1 {
				t.Errorf("Expected SummaryHeight=1, got %d", dims.SummaryHeight)
			}
			if dims.StatusHeight != 1 {
				t.Errorf("Expected StatusHeight=1, got %d", dims.StatusHeight)
			}

			// Verify all widths are positive
			if dims.LogsWidth <= 0 || dims.TasksWidth <= 0 {
				t.Errorf("All widths must be positive: Logs=%d, Tasks=%d",
					dims.LogsWidth, dims.TasksWidth)
			}

			// Verify all heights are positive
			if dims.LogsHeight <= 0 || dims.TasksHeight <= 0 {
				t.Errorf("All pane heights must be positive: Logs=%d, Tasks=%d",
					dims.LogsHeight, dims.TasksHeight)
			}
		})
	}
}

// TestCalculatePaneDimensions_WidthDistribution tests the 50-50 split
func TestCalculatePaneDimensions_WidthDistribution(t *testing.T) {
	tests := []struct {
		name      string
		termWidth int
	}{
		{"Width 100", 100},
		{"Width 120", 120},
		{"Width 160", 160},
		{"Width 200", 200},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dims := CalculatePaneDimensions(tt.termWidth, 30, false)

			// Calculate percentages
			logsPercent := float64(dims.LogsWidth) / float64(tt.termWidth)
			tasksPercent := float64(dims.TasksWidth) / float64(tt.termWidth)

			// Verify 50-50 split (allow 2% tolerance for rounding)
			tolerance := 0.02
			if logsPercent < 0.50-tolerance || logsPercent > 0.50+tolerance {
				t.Errorf("Expected LogsWidth ~50%%, got %.2f%%", logsPercent*100)
			}
			if tasksPercent < 0.50-tolerance || tasksPercent > 0.50+tolerance {
				t.Errorf("Expected TasksWidth ~50%%, got %.2f%%", tasksPercent*100)
			}

			// Verify sum equals terminal width
			totalWidth := dims.LogsWidth + dims.TasksWidth
			if totalWidth != tt.termWidth {
				t.Errorf("Sum of widths (%d) does not equal terminal width (%d)",
					totalWidth, tt.termWidth)
			}
		})
	}
}

// TestCalculatePaneDimensions_HeightDistribution tests height calculations
func TestCalculatePaneDimensions_HeightDistribution(t *testing.T) {
	tests := []struct {
		name       string
		termHeight int
	}{
		{"Height 24", 24},
		{"Height 30", 30},
		{"Height 40", 40},
		{"Height 50", 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dims := CalculatePaneDimensions(120, tt.termHeight, false)

			// Available height should be termHeight - text entry (3) - summary (1) - status (1)
			expectedAvailableHeight := tt.termHeight - 5
			if dims.LogsHeight != expectedAvailableHeight {
				t.Errorf("Expected LogsHeight=%d, got %d", expectedAvailableHeight, dims.LogsHeight)
			}
			if dims.TasksHeight != expectedAvailableHeight {
				t.Errorf("Expected TasksHeight=%d, got %d", expectedAvailableHeight, dims.TasksHeight)
			}

			// Verify pane heights are consistent
			if dims.LogsHeight != dims.TasksHeight {
				t.Errorf("All pane heights should be equal: Logs=%d, Tasks=%d",
					dims.LogsHeight, dims.TasksHeight)
			}
		})
	}
}

// TestCalculatePaneDimensions_MinimumSizes tests edge cases with small terminals
func TestCalculatePaneDimensions_MinimumSizes(t *testing.T) {
	tests := []struct {
		name       string
		termWidth  int
		termHeight int
	}{
		{"Minimum 80x24", 80, 24},
		{"Very small 10x10", 10, 10},
		{"Tiny 5x5", 5, 5},
		{"Zero height", 80, 0},
		{"Zero width", 0, 24},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dims := CalculatePaneDimensions(tt.termWidth, tt.termHeight, false)

			// All dimensions should be non-negative
			if dims.LogsWidth < 0 {
				t.Errorf("LogsWidth should be non-negative, got %d", dims.LogsWidth)
			}
			if dims.TasksWidth < 0 {
				t.Errorf("TasksWidth should be non-negative, got %d", dims.TasksWidth)
			}
			if dims.LogsHeight < 0 {
				t.Errorf("LogsHeight should be non-negative, got %d", dims.LogsHeight)
			}
			if dims.TasksHeight < 0 {
				t.Errorf("TasksHeight should be non-negative, got %d", dims.TasksHeight)
			}

			// Verify sum of widths equals terminal width (if width > 0)
			if tt.termWidth > 0 {
				totalWidth := dims.LogsWidth + dims.TasksWidth
				if totalWidth != tt.termWidth {
					t.Errorf("Sum of widths (%d) does not equal terminal width (%d)",
						totalWidth, tt.termWidth)
				}
			}

			// Fixed heights should always be correct
			if dims.TextEntryHeight != 3 {
				t.Errorf("Expected TextEntryHeight=3, got %d", dims.TextEntryHeight)
			}
			if dims.SummaryHeight != 1 {
				t.Errorf("Expected SummaryHeight=1, got %d", dims.SummaryHeight)
			}
			if dims.StatusHeight != 1 {
				t.Errorf("Expected StatusHeight=1, got %d", dims.StatusHeight)
			}
		})
	}
}

// TestCalculatePaneDimensions_SpecificDimensions tests exact calculations
func TestCalculatePaneDimensions_SpecificDimensions(t *testing.T) {
	// Test with width=100, height=30
	dims := CalculatePaneDimensions(100, 30, false)

	// Width calculations: 50% of 100 = 50 each
	if dims.LogsWidth != 50 {
		t.Errorf("Expected LogsWidth=50, got %d", dims.LogsWidth)
	}
	if dims.TasksWidth != 50 {
		t.Errorf("Expected TasksWidth=50, got %d", dims.TasksWidth)
	}

	// Height calculations: 30 - 3 - 1 - 1 = 25
	expectedHeight := 25
	if dims.LogsHeight != expectedHeight {
		t.Errorf("Expected LogsHeight=%d, got %d", expectedHeight, dims.LogsHeight)
	}
	if dims.TasksHeight != expectedHeight {
		t.Errorf("Expected TasksHeight=%d, got %d", expectedHeight, dims.TasksHeight)
	}
}

// TestCalculatePaneDimensions_WidthRounding tests that widths sum correctly
func TestCalculatePaneDimensions_WidthRounding(t *testing.T) {
	// Test various widths that might cause rounding issues
	widths := []int{81, 82, 83, 97, 98, 99, 101, 102, 103, 119, 121, 123}

	for _, width := range widths {
		t.Run("Width_"+string(rune(width+'0')), func(t *testing.T) {
			dims := CalculatePaneDimensions(width, 30, false)

			totalWidth := dims.LogsWidth + dims.TasksWidth
			if totalWidth != width {
				t.Errorf("Width=%d: Sum of widths (%d) does not equal terminal width",
					width, totalWidth)
			}
		})
	}
}

// TestCalculatePaneDimensions_Consistency tests that multiple calls produce same results
func TestCalculatePaneDimensions_Consistency(t *testing.T) {
	termWidth, termHeight := 120, 30

	dims1 := CalculatePaneDimensions(termWidth, termHeight, false)
	dims2 := CalculatePaneDimensions(termWidth, termHeight, false)

	if dims1 != dims2 {
		t.Errorf("Multiple calls with same parameters should produce identical results")
	}
}

// TestCalculatePaneDimensions_FiftyFiftySplit verifies the 50-50 horizontal split
func TestCalculatePaneDimensions_FiftyFiftySplit(t *testing.T) {
	t.Run("even width 120: each pane gets 60", func(t *testing.T) {
		dims := CalculatePaneDimensions(120, 30, false)

		if dims.LogsWidth != 60 {
			t.Errorf("Expected LogsWidth=60, got %d", dims.LogsWidth)
		}
		if dims.TasksWidth != 60 {
			t.Errorf("Expected TasksWidth=60, got %d", dims.TasksWidth)
		}
		totalWidth := dims.LogsWidth + dims.TasksWidth
		if totalWidth != 120 {
			t.Errorf("Expected LogsWidth+TasksWidth=120, got %d", totalWidth)
		}
	})

	t.Run("odd width 121: logs gets 60, tasks gets 61", func(t *testing.T) {
		dims := CalculatePaneDimensions(121, 30, false)

		if dims.LogsWidth != 60 {
			t.Errorf("Expected LogsWidth=60, got %d", dims.LogsWidth)
		}
		if dims.TasksWidth != 61 {
			t.Errorf("Expected TasksWidth=61, got %d", dims.TasksWidth)
		}
		totalWidth := dims.LogsWidth + dims.TasksWidth
		if totalWidth != 121 {
			t.Errorf("Expected LogsWidth+TasksWidth=121, got %d", totalWidth)
		}
	})

	t.Run("widths always sum to termWidth", func(t *testing.T) {
		for _, w := range []int{80, 100, 120, 121, 160, 200} {
			dims := CalculatePaneDimensions(w, 30, false)
			total := dims.LogsWidth + dims.TasksWidth
			if total != w {
				t.Errorf("termWidth=%d: LogsWidth+TasksWidth=%d, want %d", w, total, w)
			}
		}
	})
}

// TestCalculateTaskSectionDimensions_NoDetailPane tests layout without detail pane
func TestCalculateTaskSectionDimensions_NoDetailPane(t *testing.T) {
	termWidth, termHeight := 120, 30

	dims := CalculateTaskSectionDimensions(termWidth, termHeight, DetailPaneBottom, false)

	// Center width should be 50% of terminal width
	expectedWidth := termWidth / 2
	if dims.CenterWidth != expectedWidth {
		t.Errorf("Expected CenterWidth=%d, got %d", expectedWidth, dims.CenterWidth)
	}

	// All three sections should be visible and roughly equal
	if dims.TodayHeight == 0 {
		t.Error("TodayHeight should not be 0 when detail pane is hidden")
	}
	if dims.ThisWeekHeight == 0 {
		t.Error("ThisWeekHeight should not be 0 when detail pane is hidden")
	}
	if dims.AllTasksHeight == 0 {
		t.Error("AllTasksHeight should not be 0 when detail pane is hidden")
	}

	// Verify sections add up to available height (minus separators)
	totalHeight := dims.TodayHeight + dims.ThisWeekHeight + dims.AllTasksHeight
	const sectionSeparators = 2
	expectedAvailableHeight := termHeight - 3 - 1 - 1 - sectionSeparators // text entry, status, summary, separators
	if totalHeight != expectedAvailableHeight {
		t.Errorf("Expected total section height=%d, got %d", expectedAvailableHeight, totalHeight)
	}
}

// TestCalculateTaskSectionDimensions_DetailPaneBottom tests layout with detail pane at bottom
func TestCalculateTaskSectionDimensions_DetailPaneBottom(t *testing.T) {
	termWidth, termHeight := 120, 30

	dims := CalculateTaskSectionDimensions(termWidth, termHeight, DetailPaneBottom, true)

	// ALL TASKS should be replaced by detail pane
	if dims.AllTasksHeight != 0 {
		t.Errorf("AllTasksHeight should be 0 when detail pane is at bottom, got %d", dims.AllTasksHeight)
	}

	// Detail height should be non-zero
	if dims.DetailHeight == 0 {
		t.Error("DetailHeight should not be 0 when detail pane is visible")
	}

	// TODAY and THIS WEEK should be visible
	if dims.TodayHeight == 0 {
		t.Error("TodayHeight should not be 0")
	}
	if dims.ThisWeekHeight == 0 {
		t.Error("ThisWeekHeight should not be 0")
	}

	// Verify total height
	totalHeight := dims.TodayHeight + dims.ThisWeekHeight + dims.DetailHeight
	const sectionSeparators = 2
	expectedAvailableHeight := termHeight - 3 - 1 - 1 - sectionSeparators
	if totalHeight != expectedAvailableHeight {
		t.Errorf("Expected total height=%d, got %d", expectedAvailableHeight, totalHeight)
	}
}

// TestCalculateTaskSectionDimensions_DetailPaneMiddle tests layout with detail pane in middle
func TestCalculateTaskSectionDimensions_DetailPaneMiddle(t *testing.T) {
	termWidth, termHeight := 120, 30

	dims := CalculateTaskSectionDimensions(termWidth, termHeight, DetailPaneMiddle, true)

	// THIS WEEK should be replaced by detail pane
	if dims.ThisWeekHeight != 0 {
		t.Errorf("ThisWeekHeight should be 0 when detail pane is in middle, got %d", dims.ThisWeekHeight)
	}

	// Detail height should be non-zero
	if dims.DetailHeight == 0 {
		t.Error("DetailHeight should not be 0 when detail pane is visible")
	}

	// TODAY and ALL TASKS should be visible
	if dims.TodayHeight == 0 {
		t.Error("TodayHeight should not be 0")
	}
	if dims.AllTasksHeight == 0 {
		t.Error("AllTasksHeight should not be 0")
	}

	// Verify total height
	totalHeight := dims.TodayHeight + dims.DetailHeight + dims.AllTasksHeight
	const sectionSeparators = 2
	expectedAvailableHeight := termHeight - 3 - 1 - 1 - sectionSeparators
	if totalHeight != expectedAvailableHeight {
		t.Errorf("Expected total height=%d, got %d", expectedAvailableHeight, totalHeight)
	}
}

// TestCalculateTaskSectionDimensions_VariousSizes tests different terminal sizes
func TestCalculateTaskSectionDimensions_VariousSizes(t *testing.T) {
	tests := []struct {
		name       string
		termWidth  int
		termHeight int
	}{
		{"Standard 80x24", 80, 24},
		{"Common 120x30", 120, 30},
		{"Large 160x40", 160, 40},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test all combinations of position and visibility
			positions := []DetailPanePosition{DetailPaneBottom, DetailPaneMiddle}
			visibilities := []bool{true, false}

			for _, pos := range positions {
				for _, visible := range visibilities {
					dims := CalculateTaskSectionDimensions(tt.termWidth, tt.termHeight, pos, visible)

					// All dimensions should be non-negative
					if dims.CenterWidth < 0 {
						t.Errorf("CenterWidth should be non-negative, got %d", dims.CenterWidth)
					}
					if dims.TodayHeight < 0 {
						t.Errorf("TodayHeight should be non-negative, got %d", dims.TodayHeight)
					}
					if dims.ThisWeekHeight < 0 {
						t.Errorf("ThisWeekHeight should be non-negative, got %d", dims.ThisWeekHeight)
					}
					if dims.AllTasksHeight < 0 {
						t.Errorf("AllTasksHeight should be non-negative, got %d", dims.AllTasksHeight)
					}
					if dims.DetailHeight < 0 {
						t.Errorf("DetailHeight should be non-negative, got %d", dims.DetailHeight)
					}
				}
			}
		})
	}
}
