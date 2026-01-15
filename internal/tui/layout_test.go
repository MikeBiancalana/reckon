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
			if dims.StatusHeight != 1 {
				t.Errorf("Expected StatusHeight=1, got %d", dims.StatusHeight)
			}

			// Verify all widths are positive
			if dims.LogsWidth <= 0 || dims.TasksWidth <= 0 || dims.RightWidth <= 0 {
				t.Errorf("All widths must be positive: Logs=%d, Tasks=%d, Right=%d",
					dims.LogsWidth, dims.TasksWidth, dims.RightWidth)
			}

			// Verify all heights are positive
			if dims.LogsHeight <= 0 || dims.TasksHeight <= 0 || dims.RightHeight <= 0 {
				t.Errorf("All pane heights must be positive: Logs=%d, Tasks=%d, Right=%d",
					dims.LogsHeight, dims.TasksHeight, dims.RightHeight)
			}

			// Verify right sidebar component heights are positive
			if dims.ScheduleHeight <= 0 || dims.IntentionsHeight <= 0 || dims.WinsHeight <= 0 {
				t.Errorf("All right sidebar heights must be positive: Schedule=%d, Intentions=%d, Wins=%d",
					dims.ScheduleHeight, dims.IntentionsHeight, dims.WinsHeight)
			}
		})
	}
}

// TestCalculatePaneDimensions_WidthDistribution tests the 40-40-18 split
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
			rightPercent := float64(dims.RightWidth) / float64(tt.termWidth)

			// Verify 40-40-18 split (allow 2% tolerance for rounding)
			tolerance := 0.02
			if logsPercent < 0.40-tolerance || logsPercent > 0.40+tolerance {
				t.Errorf("Expected LogsWidth ~40%%, got %.2f%%", logsPercent*100)
			}
			if tasksPercent < 0.40-tolerance || tasksPercent > 0.40+tolerance {
				t.Errorf("Expected TasksWidth ~40%%, got %.2f%%", tasksPercent*100)
			}
			if rightPercent < 0.18-tolerance || rightPercent > 0.20+tolerance {
				t.Errorf("Expected RightWidth ~18-20%%, got %.2f%%", rightPercent*100)
			}

			// Verify sum equals terminal width
			totalWidth := dims.LogsWidth + dims.TasksWidth + dims.RightWidth
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

			// Available height should be termHeight - text entry (3) - status (1)
			expectedAvailableHeight := tt.termHeight - 4
			if dims.LogsHeight != expectedAvailableHeight {
				t.Errorf("Expected LogsHeight=%d, got %d", expectedAvailableHeight, dims.LogsHeight)
			}
			if dims.TasksHeight != expectedAvailableHeight {
				t.Errorf("Expected TasksHeight=%d, got %d", expectedAvailableHeight, dims.TasksHeight)
			}
			if dims.RightHeight != expectedAvailableHeight {
				t.Errorf("Expected RightHeight=%d, got %d", expectedAvailableHeight, dims.RightHeight)
			}

			// Verify pane heights are consistent
			if dims.LogsHeight != dims.TasksHeight || dims.TasksHeight != dims.RightHeight {
				t.Errorf("All pane heights should be equal: Logs=%d, Tasks=%d, Right=%d",
					dims.LogsHeight, dims.TasksHeight, dims.RightHeight)
			}
		})
	}
}

// TestCalculatePaneDimensions_RightSidebarSplit tests the 30-35-35 vertical split
func TestCalculatePaneDimensions_RightSidebarSplit(t *testing.T) {
	tests := []struct {
		name       string
		termHeight int
	}{
		{"Height 24", 24},
		{"Height 30", 30},
		{"Height 40", 40},
		{"Height 50", 50},
		{"Height 100", 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dims := CalculatePaneDimensions(120, tt.termHeight, false)

			// Calculate percentages
			schedulePercent := float64(dims.ScheduleHeight) / float64(dims.RightHeight)
			intentionsPercent := float64(dims.IntentionsHeight) / float64(dims.RightHeight)
			winsPercent := float64(dims.WinsHeight) / float64(dims.RightHeight)

			// Verify 30-35-35 split (allow 5% tolerance for rounding with small heights)
			tolerance := 0.05
			if schedulePercent < 0.30-tolerance || schedulePercent > 0.30+tolerance {
				t.Errorf("Expected ScheduleHeight ~30%%, got %.2f%%", schedulePercent*100)
			}
			if intentionsPercent < 0.35-tolerance || intentionsPercent > 0.35+tolerance {
				t.Errorf("Expected IntentionsHeight ~35%%, got %.2f%%", intentionsPercent*100)
			}
			if winsPercent < 0.35-tolerance || winsPercent > 0.35+tolerance {
				t.Errorf("Expected WinsHeight ~35%%, got %.2f%%", winsPercent*100)
			}

			// Verify sum equals right pane height
			totalRightHeight := dims.ScheduleHeight + dims.IntentionsHeight + dims.WinsHeight
			if totalRightHeight != dims.RightHeight {
				t.Errorf("Sum of right sidebar heights (%d) does not equal RightHeight (%d)",
					totalRightHeight, dims.RightHeight)
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
			if dims.RightWidth < 0 {
				t.Errorf("RightWidth should be non-negative, got %d", dims.RightWidth)
			}
			if dims.LogsHeight < 0 {
				t.Errorf("LogsHeight should be non-negative, got %d", dims.LogsHeight)
			}
			if dims.TasksHeight < 0 {
				t.Errorf("TasksHeight should be non-negative, got %d", dims.TasksHeight)
			}
			if dims.RightHeight < 0 {
				t.Errorf("RightHeight should be non-negative, got %d", dims.RightHeight)
			}
			if dims.ScheduleHeight < 0 {
				t.Errorf("ScheduleHeight should be non-negative, got %d", dims.ScheduleHeight)
			}
			if dims.IntentionsHeight < 0 {
				t.Errorf("IntentionsHeight should be non-negative, got %d", dims.IntentionsHeight)
			}
			if dims.WinsHeight < 0 {
				t.Errorf("WinsHeight should be non-negative, got %d", dims.WinsHeight)
			}

			// Verify sum of widths equals terminal width (if width > 0)
			if tt.termWidth > 0 {
				totalWidth := dims.LogsWidth + dims.TasksWidth + dims.RightWidth
				if totalWidth != tt.termWidth {
					t.Errorf("Sum of widths (%d) does not equal terminal width (%d)",
						totalWidth, tt.termWidth)
				}
			}

			// Fixed heights should always be correct
			if dims.TextEntryHeight != 3 {
				t.Errorf("Expected TextEntryHeight=3, got %d", dims.TextEntryHeight)
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

	// Width calculations: 40% = 40, 40% = 40, remaining = 20
	if dims.LogsWidth != 40 {
		t.Errorf("Expected LogsWidth=40, got %d", dims.LogsWidth)
	}
	if dims.TasksWidth != 40 {
		t.Errorf("Expected TasksWidth=40, got %d", dims.TasksWidth)
	}
	if dims.RightWidth != 20 {
		t.Errorf("Expected RightWidth=20, got %d", dims.RightWidth)
	}

	// Height calculations: 30 - 3 - 1 = 26
	expectedHeight := 26
	if dims.LogsHeight != expectedHeight {
		t.Errorf("Expected LogsHeight=%d, got %d", expectedHeight, dims.LogsHeight)
	}
	if dims.TasksHeight != expectedHeight {
		t.Errorf("Expected TasksHeight=%d, got %d", expectedHeight, dims.TasksHeight)
	}
	if dims.RightHeight != expectedHeight {
		t.Errorf("Expected RightHeight=%d, got %d", expectedHeight, dims.RightHeight)
	}

	// Right sidebar: 30% of 26 = 7.8 ≈ 8, 35% of 26 = 9.1 ≈ 9, remaining = 9
	// (actual values may vary slightly due to rounding strategy)
	totalRightHeight := dims.ScheduleHeight + dims.IntentionsHeight + dims.WinsHeight
	if totalRightHeight != expectedHeight {
		t.Errorf("Expected sum of right sidebar heights=%d, got %d", expectedHeight, totalRightHeight)
	}
}

// TestCalculatePaneDimensions_WidthRounding tests that widths sum correctly
func TestCalculatePaneDimensions_WidthRounding(t *testing.T) {
	// Test various widths that might cause rounding issues
	widths := []int{81, 82, 83, 97, 98, 99, 101, 102, 103, 119, 121, 123}

	for _, width := range widths {
		t.Run("Width_"+string(rune(width+'0')), func(t *testing.T) {
			dims := CalculatePaneDimensions(width, 30, false)

			totalWidth := dims.LogsWidth + dims.TasksWidth + dims.RightWidth
			if totalWidth != width {
				t.Errorf("Width=%d: Sum of widths (%d) does not equal terminal width",
					width, totalWidth)
			}
		})
	}
}

// TestCalculatePaneDimensions_HeightRounding tests that right sidebar heights sum correctly
func TestCalculatePaneDimensions_HeightRounding(t *testing.T) {
	// Test various heights that might cause rounding issues
	heights := []int{24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 40, 50, 60}

	for _, height := range heights {
		t.Run("Height_"+string(rune(height+'0')), func(t *testing.T) {
			dims := CalculatePaneDimensions(120, height, false)

			totalRightHeight := dims.ScheduleHeight + dims.IntentionsHeight + dims.WinsHeight
			if totalRightHeight != dims.RightHeight {
				t.Errorf("Height=%d: Sum of right sidebar heights (%d) does not equal RightHeight (%d)",
					height, totalRightHeight, dims.RightHeight)
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
