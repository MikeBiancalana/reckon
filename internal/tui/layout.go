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
// It implements a 50-50 horizontal split for the main panes (Logs and Tasks),
// with the Tasks pane split vertically 50-50 for Tasks and Notes (when notes visible).
func CalculatePaneDimensions(termWidth, termHeight int, notesPaneVisible bool) PaneDimensions {
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

	// Conditionally split Tasks/Notes based on visibility
	if notesPaneVisible {
		// Account for separator line between Tasks and Notes (1 line)
		const separatorHeight = 1
		// Split Tasks/Notes vertically, accounting for separator
		effectiveTasksNotesHeight := availableHeight - separatorHeight
		dims.TasksHeight = effectiveTasksNotesHeight / 2
		dims.NotesHeight = effectiveTasksNotesHeight - dims.TasksHeight
	} else {
		// Notes pane hidden, tasks get full height
		dims.TasksHeight = availableHeight
		dims.NotesHeight = 0
	}

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

// TaskSectionDimensions holds dimensions for the three-section task view
type TaskSectionDimensions struct {
	// Center column width (same for all sections)
	CenterWidth int

	// Section heights
	TodayHeight    int // Top section
	ThisWeekHeight int // Middle section
	AllTasksHeight int // Bottom section

	// Detail pane dimensions (replaces one of the sections)
	DetailHeight int
}

// CalculateTaskSectionDimensions computes dimensions for the three-section task view
// with detail pane positioning. The center column is divided into three vertical sections:
// TODAY (top), THIS WEEK (middle), ALL TASKS (bottom).
// When detail pane is visible, it replaces one of these sections based on detailPanePosition.
func CalculateTaskSectionDimensions(termWidth, termHeight int, detailPanePosition DetailPanePosition, detailPaneVisible bool) TaskSectionDimensions {
	dims := TaskSectionDimensions{}

	// Calculate center column width (50% of terminal width)
	dims.CenterWidth = termWidth / 2

	// Calculate available height (terminal height minus fixed bottom bars: 3 + 1 + 1 for text entry, summary, status)
	textEntryHeight := 3
	statusHeight := 1
	summaryHeight := 1
	availableHeight := termHeight - textEntryHeight - statusHeight - summaryHeight
	if availableHeight < 0 {
		availableHeight = 0
	}

	// Account for section separators (2 lines between 3 sections)
	const sectionSeparators = 2
	effectiveHeight := availableHeight - sectionSeparators
	if effectiveHeight < 0 {
		effectiveHeight = 0
	}

	if !detailPaneVisible {
		// No detail pane: split evenly among three sections (33% each)
		dims.TodayHeight = effectiveHeight / 3
		dims.ThisWeekHeight = effectiveHeight / 3
		dims.AllTasksHeight = effectiveHeight - dims.TodayHeight - dims.ThisWeekHeight
	} else {
		// Detail pane visible: one of three sections is replaced by detail pane
		// The other two sections are shown
		switch detailPanePosition {
		case DetailPaneBottom:
			// Detail pane replaces ALL TASKS (bottom)
			// TODAY and THIS WEEK are shown, detail pane is shown
			// Split: TODAY (25%), THIS WEEK (25%), DETAIL (50%)
			topQuarterHeight := effectiveHeight / 4
			dims.TodayHeight = topQuarterHeight
			dims.ThisWeekHeight = topQuarterHeight
			dims.AllTasksHeight = 0
			dims.DetailHeight = effectiveHeight - dims.TodayHeight - dims.ThisWeekHeight
		case DetailPaneMiddle:
			// Detail pane replaces THIS WEEK (middle)
			// TODAY and ALL TASKS are shown, detail pane is shown
			// Split: TODAY (25%), DETAIL (50%), ALL TASKS (25%)
			topQuarterHeight := effectiveHeight / 4
			dims.TodayHeight = topQuarterHeight
			dims.ThisWeekHeight = 0
			dims.DetailHeight = effectiveHeight / 2
			dims.AllTasksHeight = effectiveHeight - dims.TodayHeight - dims.DetailHeight
		}
	}

	return dims
}
