package tui

// PaneDimensions holds calculated dimensions for all panes in the TUI layout
type PaneDimensions struct {
	// Left pane (Logs)
	LogsWidth  int
	LogsHeight int

	// Center pane (Tasks)
	TasksWidth  int
	TasksHeight int

	// Right sidebar (total)
	RightWidth  int
	RightHeight int

	// Right sidebar (stacked components)
	ScheduleHeight   int // ~30% of right height
	IntentionsHeight int // ~35% of right height
	WinsHeight       int // ~35% of right height

	// Bottom bars
	TextEntryHeight int // Fixed: 3 lines
	StatusHeight    int // Fixed: 1 line
}

// CalculatePaneDimensions computes pane sizes based on terminal dimensions.
// It implements a 40-40-18 horizontal split for the main panes, with the right
// sidebar further divided vertically into 30-35-35 for Schedule, Intentions, and Wins.
func CalculatePaneDimensions(termWidth, termHeight int) PaneDimensions {
	dims := PaneDimensions{
		TextEntryHeight: 3,
		StatusHeight:    1,
	}

	// Calculate available height (terminal height minus fixed bottom bars)
	availableHeight := termHeight - dims.TextEntryHeight - dims.StatusHeight
	if availableHeight < 0 {
		availableHeight = 0
	}

	// All main panes share the same available height
	dims.LogsHeight = availableHeight
	dims.TasksHeight = availableHeight
	dims.RightHeight = availableHeight

	// Calculate horizontal widths with 40-40-18 split
	// Use integer arithmetic to ensure sum equals termWidth
	dims.LogsWidth = int(float64(termWidth) * 0.40)
	dims.TasksWidth = int(float64(termWidth) * 0.40)
	// Remaining width goes to right pane (ensures sum = termWidth)
	dims.RightWidth = termWidth - dims.LogsWidth - dims.TasksWidth

	// Ensure non-negative widths
	if dims.LogsWidth < 0 {
		dims.LogsWidth = 0
	}
	if dims.TasksWidth < 0 {
		dims.TasksWidth = 0
	}
	if dims.RightWidth < 0 {
		dims.RightWidth = 0
	}

	// Calculate right sidebar vertical split (30-35-35)
	dims.ScheduleHeight = int(float64(dims.RightHeight) * 0.30)
	dims.IntentionsHeight = int(float64(dims.RightHeight) * 0.35)
	// Remaining height goes to Wins (ensures sum = RightHeight)
	dims.WinsHeight = dims.RightHeight - dims.ScheduleHeight - dims.IntentionsHeight

	// Ensure non-negative heights for right sidebar components
	if dims.ScheduleHeight < 0 {
		dims.ScheduleHeight = 0
	}
	if dims.IntentionsHeight < 0 {
		dims.IntentionsHeight = 0
	}
	if dims.WinsHeight < 0 {
		dims.WinsHeight = 0
	}

	return dims
}
