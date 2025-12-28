package tui_test

import (
	"fmt"

	"github.com/MikeBiancalana/reckon/internal/tui"
)

// ExampleCalculatePaneDimensions demonstrates basic usage of the layout manager
func ExampleCalculatePaneDimensions() {
	// Calculate dimensions for a standard 120x30 terminal
	dims := tui.CalculatePaneDimensions(120, 30)

	fmt.Printf("Terminal: 120x30\n")
	fmt.Printf("Left pane (Logs): %dx%d\n", dims.LogsWidth, dims.LogsHeight)
	fmt.Printf("Center pane (Tasks): %dx%d\n", dims.TasksWidth, dims.TasksHeight)
	fmt.Printf("Right pane (total): %dx%d\n", dims.RightWidth, dims.RightHeight)
	fmt.Printf("  Schedule: height %d\n", dims.ScheduleHeight)
	fmt.Printf("  Intentions: height %d\n", dims.IntentionsHeight)
	fmt.Printf("  Wins: height %d\n", dims.WinsHeight)
	fmt.Printf("Text entry bar: height %d\n", dims.TextEntryHeight)
	fmt.Printf("Status bar: height %d\n", dims.StatusHeight)

	// Output:
	// Terminal: 120x30
	// Left pane (Logs): 48x26
	// Center pane (Tasks): 48x26
	// Right pane (total): 24x26
	//   Schedule: height 7
	//   Intentions: height 9
	//   Wins: height 10
	// Text entry bar: height 3
	// Status bar: height 1
}

// ExampleCalculatePaneDimensions_minimumTerminal demonstrates layout with minimum terminal size
func ExampleCalculatePaneDimensions_minimumTerminal() {
	// Calculate dimensions for minimum 80x24 terminal
	dims := tui.CalculatePaneDimensions(80, 24)

	fmt.Printf("Terminal: 80x24 (minimum size)\n")
	fmt.Printf("Left pane (Logs): %dx%d\n", dims.LogsWidth, dims.LogsHeight)
	fmt.Printf("Center pane (Tasks): %dx%d\n", dims.TasksWidth, dims.TasksHeight)
	fmt.Printf("Right pane (total): %dx%d\n", dims.RightWidth, dims.RightHeight)

	// Output:
	// Terminal: 80x24 (minimum size)
	// Left pane (Logs): 32x20
	// Center pane (Tasks): 32x20
	// Right pane (total): 16x20
}

// ExampleCalculatePaneDimensions_largeTerminal demonstrates layout with a large terminal
func ExampleCalculatePaneDimensions_largeTerminal() {
	// Calculate dimensions for a large 200x50 terminal
	dims := tui.CalculatePaneDimensions(200, 50)

	// Verify the 40-40-18 split
	logsPercent := float64(dims.LogsWidth) / 200.0 * 100
	tasksPercent := float64(dims.TasksWidth) / 200.0 * 100
	rightPercent := float64(dims.RightWidth) / 200.0 * 100

	fmt.Printf("Terminal: 200x50\n")
	fmt.Printf("Logs width: %.0f%%\n", logsPercent)
	fmt.Printf("Tasks width: %.0f%%\n", tasksPercent)
	fmt.Printf("Right sidebar width: %.0f%%\n", rightPercent)
	fmt.Printf("Available height for content: %d\n", dims.LogsHeight)

	// Output:
	// Terminal: 200x50
	// Logs width: 40%
	// Tasks width: 40%
	// Right sidebar width: 20%
	// Available height for content: 46
}
