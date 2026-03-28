package tui

// PaneDimensions holds calculated dimensions for all panes in the TUI layout
type PaneDimensions struct {
	// Left pane (Logs)
	LogsWidth  int
	LogsHeight int

	// Center pane (Tasks)
	TasksWidth  int
	TasksHeight int

	// Center pane (Notes)
	NotesWidth  int
	NotesHeight int

	// Bottom bars
	TextEntryHeight int // Fixed: 3 lines
	SummaryHeight   int // Fixed: 1 line
	StatusHeight    int // Fixed: 1 line
}

// CalculatePaneDimensions computes pane sizes based on terminal dimensions.
// It implements a 50-50 horizontal split for the main panes (Logs and Tasks).
// Tasks always get the full available height; the notes pane is no longer displayed.
func CalculatePaneDimensions(termWidth, termHeight int) PaneDimensions {
	dims := PaneDimensions{
		TextEntryHeight: 3,
		SummaryHeight:   1,
		StatusHeight:    1,
	}

	// Calculate available height (terminal height minus fixed bottom bars)
	availableHeight := termHeight - dims.TextEntryHeight - dims.SummaryHeight - dims.StatusHeight
	if availableHeight < 0 {
		availableHeight = 0
	}

	// All main panes share the same available height
	dims.LogsHeight = availableHeight

	// Tasks get full height; notes pane is no longer shown
	dims.TasksHeight = availableHeight
	dims.NotesHeight = 0

	// Calculate horizontal widths with 50-50 split
	// Logs gets half, Tasks gets the remainder (handles odd widths correctly)
	dims.LogsWidth = termWidth / 2
	dims.TasksWidth = termWidth - dims.LogsWidth
	dims.NotesWidth = dims.TasksWidth

	// Ensure non-negative widths
	if dims.LogsWidth < 0 {
		dims.LogsWidth = 0
	}
	if dims.TasksWidth < 0 {
		dims.TasksWidth = 0
	}

	return dims
}
