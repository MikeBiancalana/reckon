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
// It implements a 40-40-18 horizontal split for the main panes, with the center
// Tasks pane split vertically 50-50 for Tasks and Notes (when notes visible), and the right
// sidebar further divided vertically into 30-35-35 for Schedule, Intentions, and Wins.
func CalculatePaneDimensions(termWidth, termHeight int, notesPaneVisible bool) PaneDimensions {
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
	dims.RightHeight = availableHeight

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

	// Calculate horizontal widths with 40-40-18 split
	// Use integer arithmetic to ensure sum equals termWidth
	dims.LogsWidth = int(float64(termWidth) * 0.40)
	dims.TasksWidth = int(float64(termWidth) * 0.40)
	dims.NotesWidth = dims.TasksWidth
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

	// Calculate center column width (40% of terminal width)
	dims.CenterWidth = int(float64(termWidth) * 0.40)

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
		// Detail pane visible: replace one section
		switch detailPanePosition {
		case DetailPaneBottom:
			// Detail pane replaces ALL TASKS (bottom)
			// TODAY and THIS WEEK split the available height (50-50)
			topHalfHeight := effectiveHeight / 2
			dims.TodayHeight = topHalfHeight / 2
			dims.ThisWeekHeight = topHalfHeight - dims.TodayHeight
			dims.AllTasksHeight = 0
			dims.DetailHeight = effectiveHeight - topHalfHeight
		case DetailPaneMiddle:
			// Detail pane replaces THIS WEEK (middle)
			// TODAY and ALL TASKS split the available height (50-50)
			topHalfHeight := effectiveHeight / 2
			dims.TodayHeight = topHalfHeight
			dims.ThisWeekHeight = 0
			dims.AllTasksHeight = effectiveHeight - topHalfHeight
			dims.DetailHeight = topHalfHeight
		}
	}

	return dims
}
