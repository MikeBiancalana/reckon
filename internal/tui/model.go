package tui

import (
	"fmt"
	"strings"
	stdtime "time"

	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/MikeBiancalana/reckon/internal/service"
	"github.com/MikeBiancalana/reckon/internal/sync"
	"github.com/MikeBiancalana/reckon/internal/tui/components"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// DetailPanePosition represents where the detail pane should be displayed
type DetailPanePosition int

const (
	DetailPaneBottom DetailPanePosition = iota // Detail pane replaces bottom section (ALL TASKS)
	DetailPaneMiddle                           // Detail pane replaces middle section (THIS WEEK)
)

// Section represents different sections of the journal
//
// Async Closure Capture Pattern
// ==============================
// When using tea.Cmd (async functions), Go closures capture variables by REFERENCE,
// not by value. This is a common source of bugs in event-driven code where model
// state may change between when the closure is created and when it executes.
//
// WRONG (buggy):
//
//	func (m *Model) submitTextEntry() tea.Cmd {
//	    inputText := m.textEntryBar.GetValue()
//	    mode := m.textEntryBar.GetMode()
//	    return func() tea.Msg {
//	        // BUG: m.currentJournal may have changed by the time this runs!
//	        err := m.service.AddWin(m.currentJournal, inputText)
//	        return errMsg{err}
//	    }
//	}
//
// CORRECT (captured values):
//
//	func (m *Model) submitTextEntry() tea.Cmd {
//	    inputText := m.textEntryBar.GetValue()
//	    mode := m.textEntryBar.GetMode()
//	    capturedJournal := m.currentJournal  // Capture BEFORE creating closure
//	    return func() tea.Msg {
//	        // Safe: capturedJournal preserves the value at capture time
//	        err := m.service.AddWin(capturedJournal, inputText)
//	        return errMsg{err}
//	    }
//	}
//
// Key principles:
// 1. Capture all model values you need BEFORE returning the closure
// 2. Use descriptive variable names: capturedXxx for clarity
// 3. Function parameters are captured at the point of definition
type Section int

const (
	SectionIntentions Section = iota
	SectionWins
	SectionLogs
	SectionTasks
	SectionNotes
	SectionSchedule
	SectionCount // Keep this last to get the count
)

const (
	SectionNameIntentions = "Intentions"
	SectionNameWins       = "Wins"
	SectionNameLogs       = "Logs"
	SectionNameTasks      = "Tasks"
	SectionNameNotes      = "Notes"
	SectionNameSchedule   = "Schedule"
)

// sectionName returns the display name for a section
func sectionName(s Section) string {
	switch s {
	case SectionIntentions:
		return SectionNameIntentions
	case SectionWins:
		return SectionNameWins
	case SectionLogs:
		return SectionNameLogs
	case SectionTasks:
		return SectionNameTasks
	case SectionNotes:
		return SectionNameNotes
	case SectionSchedule:
		return SectionNameSchedule
	default:
		return "Unknown"
	}
}

// Minimum terminal dimensions
const (
	MinTerminalWidth  = 80
	MinTerminalHeight = 24
)

// Border dimensions for lipgloss boxes
const (
	BorderWidth  = 2 // Left + right border (1 char each)
	BorderHeight = 2 // Top + bottom border (1 char each)
)

// Model represents the main TUI state
type Model struct {
	service        *journal.Service
	taskService    *journal.TaskService
	notesService   *service.NotesService
	watcher        *sync.Watcher
	currentDate    string
	currentJournal *journal.Journal
	focusedSection Section
	width          int
	height         int

	// Components
	intentionList *components.IntentionList
	winsView      *components.WinsView
	logView       *components.LogView
	textEntryBar  *components.TextEntryBar
	statusBar     *components.StatusBar

	// New components for 40-40-18 layout
	taskList     *components.TaskList
	scheduleView *components.ScheduleView
	summaryView  *components.SummaryView
	notesPane    *components.NotesPane

	// Notes pane state
	selectedTaskID   string
	notesPaneVisible bool

	// Detail pane positioning
	detailPanePosition DetailPanePosition

	// Date picker state
	datePicker           *components.DatePicker
	datePickerVisible    bool
	datePickerMode       string // "schedule" or "deadline"
	datePickerTargetTask string // ID of task being scheduled/deadline-set

	// Clear date submenu state
	clearDateMode       bool
	clearDateTargetTask string // ID of task for clear submenu

	// State for modes
	helpMode          bool
	taskCreationMode  bool
	confirmMode       bool
	confirmItemType   string // "intention", "win", "log", "log_note"
	confirmItemID     string
	confirmLogEntryID string   // For log_note deletion, stores the log entry ID
	editItemID        string   // ID of item being edited
	editItemType      string   // "task", "intention", "win", "log"
	editItemTags      []string // Original tags of task being edited (for preservation)
	noteTaskID        string   // ID of task being noted
	noteLogEntryID    string   // ID of log entry being noted
	lastError         error
	successMessage    string // Temporary success message

	// Terminal size validation
	terminalTooSmall bool
}

// updateNotesForSelectedTask updates the notes pane with notes for the currently selected task
func (m *Model) updateNotesForSelectedTask() {
	if m.taskList == nil || m.notesPane == nil {
		return
	}
	selectedTask := m.taskList.SelectedTask()
	if selectedTask == nil {
		m.selectedTaskID = ""
		return
	}
	if selectedTask.ID == m.selectedTaskID {
		return // No change
	}
	m.selectedTaskID = selectedTask.ID
}

// updateLinksForSelectedItem loads links for the currently selected item
// This is a stub for now - full implementation will come when tasks are linked to notes
func (m *Model) updateLinksForSelectedItem() tea.Cmd {
	// For now, this is a stub since tasks don't yet have associated zettelkasten notes
	// Once reckon-edr implements note navigation, this will be fully wired up
	// The infrastructure is ready - just need the task<->note association

	if m.notesService == nil || m.notesPane == nil {
		return nil
	}

	// TODO: Get note ID from current context (task, note picker, etc.)
	// For now, return nil since tasks don't have associated notes yet
	// When implemented, this would be:
	// return m.loadLinksForNote(noteID)

	return nil
}

// TaskSection represents which time-based section a task belongs to
type TaskSection int

const (
	TaskSectionToday    TaskSection = iota // Task is scheduled/due today or overdue
	TaskSectionThisWeek                    // Task is scheduled/due this week (but not today)
	TaskSectionAllTasks                    // All other tasks
)

// getTaskSection determines which section a task belongs to based on its schedule and deadline
// This function mirrors the logic from components.GroupTasksByTime but is optimized for single tasks
// to avoid creating a single-element slice and iterating through it.
func getTaskSection(task *journal.Task) TaskSection {
	if task == nil {
		return TaskSectionAllTasks
	}

	// Skip done tasks (they don't appear in any section)
	if task.Status == journal.TaskDone {
		return TaskSectionAllTasks
	}

	// Calculate time boundaries
	today := stdtime.Now().Truncate(24 * stdtime.Hour)
	now := stdtime.Now()
	weekday := now.Weekday()
	if weekday == stdtime.Sunday {
		weekday = 7
	}
	weekStart := today.AddDate(0, 0, -int(weekday-stdtime.Monday))
	weekEnd := weekStart.AddDate(0, 0, 7)

	isToday := false
	isThisWeek := false

	// Check scheduled date
	if task.ScheduledDate != nil && *task.ScheduledDate != "" {
		if scheduledDate, err := stdtime.Parse("2006-01-02", *task.ScheduledDate); err == nil {
			if scheduledDate.Equal(today) || scheduledDate.Before(today) {
				isToday = true
			} else if (scheduledDate.After(weekStart) || scheduledDate.Equal(weekStart)) && scheduledDate.Before(weekEnd) {
				isThisWeek = true
			}
		}
	}

	// Check deadline date
	if task.DeadlineDate != nil && *task.DeadlineDate != "" {
		if deadlineDate, err := stdtime.Parse("2006-01-02", *task.DeadlineDate); err == nil {
			if deadlineDate.Before(today) || deadlineDate.Equal(today) {
				isToday = true
			} else if (deadlineDate.After(weekStart) || deadlineDate.Equal(weekStart)) && deadlineDate.Before(weekEnd) {
				isThisWeek = true
			}
		}
	}

	if isToday {
		return TaskSectionToday
	}
	if isThisWeek {
		return TaskSectionThisWeek
	}
	return TaskSectionAllTasks
}

// calculateDetailPanePosition determines where to show the detail pane based on the selected task's section
// Rules:
// - If task is in TODAY or THIS WEEK: show detail pane at bottom (replacing ALL TASKS)
// - If task is in ALL TASKS: show detail pane in middle (replacing THIS WEEK)
func (m *Model) calculateDetailPanePosition() {
	if m.taskList == nil {
		return
	}

	selectedTask := m.taskList.SelectedTask()
	section := getTaskSection(selectedTask)

	switch section {
	case TaskSectionToday, TaskSectionThisWeek:
		m.detailPanePosition = DetailPaneBottom
	case TaskSectionAllTasks:
		m.detailPanePosition = DetailPaneMiddle
	}
}

// renderTaskSection renders a task section with a header
func (m *Model) renderTaskSection(title string, tasks []journal.Task, width, height int) string {
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Bold(true)

	header := titleStyle.Render(fmt.Sprintf("━━ %s (%d) ━━", title, len(tasks)))

	if len(tasks) == 0 {
		noTasksStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Padding(1, 2)
		content := noTasksStyle.Render("No tasks")
		return header + "\n" + content
	}

	// Define task styles
	taskStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39"))
	taskDoneStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Strikethrough(true)

	// Get selected task ID for highlighting.
	// Note: We must manually apply selection styling in renderTaskSection because
	// this function bypasses the TaskList delegate which normally handles selection
	// rendering. The delegate is only used for the main task list view, not for
	// embedded task sections in the daily view.
	var selectedTaskID string
	if m.taskList != nil {
		selectedTask := m.taskList.SelectedTask()
		if selectedTask != nil {
			selectedTaskID = selectedTask.ID
		}
	}

	// Render tasks
	var taskLines []string
	for _, task := range tasks {
		checkbox := "[ ]"
		style := taskStyle
		if task.Status == journal.TaskDone {
			checkbox = "[x]"
			style = taskDoneStyle
		}

		line := fmt.Sprintf("%s %s", checkbox, task.Text)
		if len(task.Tags) > 0 {
			line = line + fmt.Sprintf(" [%s]", strings.Join(task.Tags, " "))
		}

		// Apply selection highlighting if this task is selected.
		// Selection style takes precedence over task status styling (e.g., strikethrough
		// for done tasks) to ensure the selected item is always clearly visible.
		if task.ID == selectedTaskID {
			line = components.SelectedStyle.Render(line)
		} else {
			line = style.Render(line)
		}

		taskLines = append(taskLines, line)
	}

	content := strings.Join(taskLines, "\n")

	// Ensure content fits within height
	contentHeight := height - 1 // Reserve 1 line for header
	if contentHeight < 0 {
		contentHeight = 0
	}

	return header + "\n" + content
}

// renderDetailPane renders a placeholder for the task detail pane
func (m *Model) renderDetailPane(width, height int) string {
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Bold(true)

	if m.taskList == nil {
		return titleStyle.Render("━━ TASK DETAILS ━━") + "\n\nNo task list available"
	}

	selectedTask := m.taskList.SelectedTask()
	if selectedTask == nil {
		return titleStyle.Render("━━ TASK DETAILS ━━") + "\n\nNo task selected"
	}

	header := titleStyle.Render("━━ TASK DETAILS ━━")

	// Render basic task info as placeholder
	placeholderStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Padding(1, 2)

	placeholder := placeholderStyle.Render(
		fmt.Sprintf("Task: %s\n\nDetail pane positioning framework is ready.\nActual task details component (reckon-egt) not yet implemented.",
			selectedTask.Text),
	)

	return header + "\n" + placeholder
}

// NewModel creates a new TUI model
func NewModel(service *journal.Service) *Model {
	// Create watcher
	watcher, err := sync.NewWatcher(service)
	if err != nil {
		// Log error but continue without watcher
		watcher = nil
	}

	sb := components.NewStatusBar()
	sb.SetSection(SectionNameIntentions)
	sb.SetInputMode(false)

	return &Model{
		service:        service,
		taskService:    nil, // Will be set via SetJournalTaskService
		watcher:        watcher,
		currentDate:    stdtime.Now().Format("2006-01-02"),
		focusedSection: SectionLogs,
		textEntryBar:   components.NewTextEntryBar(),
		statusBar:      sb,
		summaryView:    components.NewSummaryView(),
		notesPane:      components.NewNotesPane(),
		datePicker:     components.NewDatePicker(""),
	}
}

// SetJournalTaskService sets the journal task service
func (m *Model) SetJournalTaskService(taskService *journal.TaskService) {
	m.taskService = taskService
}

// SetNotesService sets the notes service
func (m *Model) SetNotesService(svc *service.NotesService) {
	m.notesService = svc
}

// Init initializes the model
func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.loadJournal()}

	// Load tasks if taskService is available
	if m.taskService != nil {
		cmds = append(cmds, m.loadTasks())
	} else {
		// Debug: taskService is nil
	}

	// Start watcher
	if m.watcher != nil {
		if err := m.watcher.Start(); err == nil {
			cmds = append(cmds, m.waitForFileChange())
		}
	}

	return tea.Batch(cmds...)
}

// Update handles messages and updates the model
// This function is now a simple dispatcher that routes messages to
// dedicated handler methods organized in handlers.go and keyboard.go
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleWindowSize(msg)

	case journalLoadedMsg:
		return m.handleJournalLoaded(msg)

	case journalUpdatedMsg:
		return m.handleJournalUpdated(msg)

	case tasksLoadedMsg:
		return m.handleTasksLoaded(msg)

	case components.TaskToggleMsg:
		return m.handleTaskToggle(msg)

	case components.TaskSelectionChangedMsg:
		return m.handleTaskSelectionChanged(msg)

	case taskToggledMsg:
		return m.handleTaskToggled(msg)

	case taskAddedMsg:
		return m.handleTaskAdded(msg)

	case noteAddedMsg:
		return m.handleNoteAdded(msg)

	case components.LogNoteAddMsg:
		return m.handleLogNoteAdd(msg)

	case components.LogNoteDeleteMsg:
		return m.handleLogNoteDelete(msg)

	case components.TaskNoteDeleteMsg:
		return m.handleTaskNoteDelete(msg)

	case logNoteAddedMsg:
		return m.handleLogNoteAdded(msg)

	case logNoteDeletedMsg:
		return m.handleLogNoteDeleted(msg)

	case taskNoteDeletedMsg:
		return m.handleTaskNoteDeleted(msg)

	case taskScheduledMsg:
		return m.handleTaskScheduled(msg)

	case taskDeadlineSetMsg:
		return m.handleTaskDeadlineSet(msg)

	case taskDateClearedMsg:
		return m.handleTaskDateCleared(msg)

	case clearSuccessMsg:
		m.successMessage = ""
		return m, nil

	case linksLoadedMsg:
		return m.handleLinksLoaded(msg)

	case components.LinkSelectedMsg:
		return m.handleLinkSelected(msg)

	case fileChangedMsg:
		return m.handleFileChanged(msg)

	case errMsg:
		return m.handleError(msg)

	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	default:
		return m, nil
	}
}

// View renders the TUI
func (m *Model) View() string {
	// Handle terminal too small case
	if m.terminalTooSmall {
		return m.terminalTooSmallView()
	}

	if m.currentJournal == nil {
		return "Loading..."
	}

	if m.confirmMode {
		var itemType string
		switch m.confirmItemType {
		case "intention":
			itemType = "intention"
		case "win":
			itemType = "win"
		case "log":
			itemType = "log entry"
		case "log_note":
			itemType = "log note"
		case "task":
			itemType = "task"
		case "task_note":
			itemType = "task note"
		}
		view := fmt.Sprintf("Delete this %s? (y/n)", itemType)
		if m.lastError != nil {
			view += "\n\nError: " + m.lastError.Error()
		}
		return view
	}

	if m.clearDateMode {
		view := "Clear: [S]chedule [D]eadline [B]oth ESC:cancel"
		return view
	}

	if m.helpMode {
		return m.helpView()
	}

	// Use multi-section split view when components are available and terminal is wide enough
	if m.taskList != nil && m.scheduleView != nil && m.width >= MinTerminalWidth {
		return m.renderNewLayout()
	}

	// Three-pane mode (fallback for narrow terminals)
	var content string
	paneHeight := m.height - 2
	paneWidthIntentions := int(float64(m.width) * 0.25)
	paneWidthWins := paneWidthIntentions
	paneWidthLogs := m.width - 2*paneWidthIntentions

	// Size components to panes
	if m.intentionList != nil {
		m.intentionList.SetSize(paneWidthIntentions, paneHeight)
	}
	if m.winsView != nil {
		m.winsView.SetSize(paneWidthWins, paneHeight)
	}
	if m.logView != nil {
		m.logView.SetSize(paneWidthLogs, paneHeight)
	}

	// Get pane views
	intentionsView := ""
	if m.intentionList != nil {
		intentionsView = m.intentionList.View()
	}
	winsView := ""
	if m.winsView != nil {
		winsView = m.winsView.View()
	}
	logsView := ""
	if m.logView != nil {
		logsView = m.logView.View()
	}

	// Join panes with borders
	borderStyle := lipgloss.NewStyle().BorderRight(true).BorderStyle(lipgloss.NormalBorder())
	content = lipgloss.JoinHorizontal(
		lipgloss.Top,
		borderStyle.Render(intentionsView),
		borderStyle.Render(winsView),
		logsView,
	)

	status := ""
	if m.statusBar != nil {
		m.statusBar.SetDate(m.currentDate)
		status = m.statusBar.View()
	}

	return content + "\n" + status
}

// centerView places a view within given dimensions (left-aligned, top-aligned)
func centerView(width, height int, view string) string {
	return lipgloss.Place(width, height, lipgloss.Left, lipgloss.Top, view)
}

// getBorderStyle returns a border style with focus color if the section is focused
func (m *Model) getBorderStyle(section Section) lipgloss.Style {
	style := lipgloss.NewStyle().Border(lipgloss.NormalBorder())
	if m.focusedSection == section {
		style = style.BorderForeground(lipgloss.Color("11")) // bright yellow color for focus
	}
	return style
}

// isRightPaneFocused returns true if any right pane section is focused
func (m *Model) isRightPaneFocused() bool {
	return m.focusedSection == SectionIntentions ||
		m.focusedSection == SectionWins ||
		m.focusedSection == SectionSchedule
}

// renderTasksWithDetailPane renders the center column with three task sections and optional detail pane
func (m *Model) renderTasksWithDetailPane() string {
	if m.taskList == nil {
		return "No tasks"
	}

	// Get grouped tasks
	allTasks := m.taskList.GetTasks()
	grouped := components.GroupTasksByTime(allTasks)

	// Calculate dimensions for task sections
	sectionDims := CalculateTaskSectionDimensions(m.width, m.height, m.detailPanePosition, m.notesPaneVisible)

	// Build sections based on detail pane position
	var sections []string

	if !m.notesPaneVisible {
		// No detail pane: show all three sections
		if sectionDims.TodayHeight > 0 {
			todayView := m.renderTaskSection("TODAY", grouped.Today, sectionDims.CenterWidth-BorderWidth, sectionDims.TodayHeight-BorderHeight)
			sections = append(sections, todayView)
		}
		if sectionDims.ThisWeekHeight > 0 {
			thisWeekView := m.renderTaskSection("THIS WEEK", grouped.ThisWeek, sectionDims.CenterWidth-BorderWidth, sectionDims.ThisWeekHeight-BorderHeight)
			sections = append(sections, thisWeekView)
		}
		if sectionDims.AllTasksHeight > 0 {
			allTasksView := m.renderTaskSection("ALL TASKS", grouped.AllTasks, sectionDims.CenterWidth-BorderWidth, sectionDims.AllTasksHeight-BorderHeight)
			sections = append(sections, allTasksView)
		}
	} else {
		// Detail pane visible: show sections based on position
		switch m.detailPanePosition {
		case DetailPaneBottom:
			// Show TODAY and THIS WEEK, detail pane at bottom
			if sectionDims.TodayHeight > 0 {
				todayView := m.renderTaskSection("TODAY", grouped.Today, sectionDims.CenterWidth-BorderWidth, sectionDims.TodayHeight-BorderHeight)
				sections = append(sections, todayView)
			}
			if sectionDims.ThisWeekHeight > 0 {
				thisWeekView := m.renderTaskSection("THIS WEEK", grouped.ThisWeek, sectionDims.CenterWidth-BorderWidth, sectionDims.ThisWeekHeight-BorderHeight)
				sections = append(sections, thisWeekView)
			}
			if sectionDims.DetailHeight > 0 {
				detailView := m.renderDetailPane(sectionDims.CenterWidth-BorderWidth, sectionDims.DetailHeight-BorderHeight)
				sections = append(sections, detailView)
			}
		case DetailPaneMiddle:
			// Show TODAY, detail pane in middle, ALL TASKS at bottom
			if sectionDims.TodayHeight > 0 {
				todayView := m.renderTaskSection("TODAY", grouped.Today, sectionDims.CenterWidth-BorderWidth, sectionDims.TodayHeight-BorderHeight)
				sections = append(sections, todayView)
			}
			if sectionDims.DetailHeight > 0 {
				detailView := m.renderDetailPane(sectionDims.CenterWidth-BorderWidth, sectionDims.DetailHeight-BorderHeight)
				sections = append(sections, detailView)
			}
			if sectionDims.AllTasksHeight > 0 {
				allTasksView := m.renderTaskSection("ALL TASKS", grouped.AllTasks, sectionDims.CenterWidth-BorderWidth, sectionDims.AllTasksHeight-BorderHeight)
				sections = append(sections, allTasksView)
			}
		}
	}

	// Join sections with separators
	if len(sections) == 0 {
		return "No tasks"
	}

	// Calculate separator width with bounds checking to prevent negative widths
	separatorWidth := sectionDims.CenterWidth - BorderWidth
	if separatorWidth < 0 {
		separatorWidth = 0
	}

	separatorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))
	separator := separatorStyle.Render(strings.Repeat("─", separatorWidth))

	return strings.Join(sections, "\n"+separator+"\n")
}

// renderNewLayout renders the 40-40-18 layout: Logs | Tasks | Schedule/Intentions/Wins
func (m *Model) renderNewLayout() string {
	dims := CalculatePaneDimensions(m.width, m.height, m.notesPaneVisible)

	// Size components accounting for borders
	if m.taskList != nil {
		m.taskList.SetSize(dims.TasksWidth-BorderWidth, dims.TasksHeight-BorderHeight)
		m.taskList.SetFocused(m.focusedSection == SectionTasks)
	}
	if m.notesPane != nil {
		m.notesPane.SetSize(dims.NotesWidth-BorderWidth, dims.NotesHeight-BorderHeight)
		m.notesPane.SetFocused(m.focusedSection == SectionNotes)
	}
	if m.scheduleView != nil {
		m.scheduleView.SetSize(dims.RightWidth-BorderWidth, dims.ScheduleHeight-BorderHeight)
		m.scheduleView.SetFocused(m.focusedSection == SectionSchedule)
	}
	if m.intentionList != nil {
		m.intentionList.SetSize(dims.RightWidth-BorderWidth, dims.IntentionsHeight-BorderHeight)
		m.intentionList.SetFocused(m.focusedSection == SectionIntentions)
	}
	if m.winsView != nil {
		m.winsView.SetSize(dims.RightWidth-BorderWidth, dims.WinsHeight-BorderHeight)
		m.winsView.SetFocused(m.focusedSection == SectionWins)
	}
	if m.logView != nil {
		m.logView.SetSize(dims.LogsWidth-BorderWidth, dims.LogsHeight-BorderHeight)
		m.logView.SetFocused(m.focusedSection == SectionLogs)
	}

	// Get pane views
	logsView := ""
	if m.logView != nil {
		logsView = m.logView.View()
	}

	// Calculate inner dimensions for centering
	logsInnerWidth := dims.LogsWidth - BorderWidth
	logsInnerHeight := dims.LogsHeight - BorderHeight
	tasksInnerWidth := dims.TasksWidth - BorderWidth
	tasksInnerHeight := dims.TasksHeight - BorderHeight
	rightInnerWidth := dims.RightWidth - BorderWidth

	// Center and box Logs pane
	logsBox := m.getBorderStyle(SectionLogs).Render(
		centerView(logsInnerWidth, logsInnerHeight, logsView),
	)

	// Render center column with three-section task view and detail pane
	centerContent := ""
	if m.taskList != nil {
		centerContent = m.renderTasksWithDetailPane()
	}

	// Center and box the center column
	tasksCentered := centerView(tasksInnerWidth, tasksInnerHeight, centerContent)
	tasksBox := m.getBorderStyle(SectionTasks).Render(tasksCentered)

	// Build right sidebar with centered, boxed components
	rightSidebar := m.buildRightSidebar(dims, rightInnerWidth, BorderHeight)

	// Join main panes horizontally
	content := lipgloss.JoinHorizontal(
		lipgloss.Top,
		logsBox,
		tasksBox,
		rightSidebar,
	)

	// Add text entry bar
	textEntry := ""
	if m.textEntryBar != nil {
		m.textEntryBar.SetWidth(m.width)
		textEntry = m.textEntryBar.View()
	}

	// Add success message if present
	successMsg := ""
	if m.successMessage != "" {
		successStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("40")).
			Padding(0, 1)
		successMsg = successStyle.Render(m.successMessage)
	}

	// Add summary view
	summary := ""
	if m.summaryView != nil {
		m.summaryView.SetWidth(m.width)
		summary = m.summaryView.View()
	}

	// Add status bar
	status := ""
	if m.statusBar != nil {
		m.statusBar.SetDate(m.currentDate)
		status = m.statusBar.View()
	}

	mainView := content + "\n" + textEntry + "\n" + successMsg + "\n" + summary + "\n" + status

	// Overlay date picker if visible
	if m.datePickerVisible && m.datePicker != nil {
		overlay := m.datePicker.View()
		// Center the date picker overlay using Place
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlay)
	}

	return mainView
}

// buildRightSidebar constructs the vertically stacked right sidebar
func (m *Model) buildRightSidebar(dims PaneDimensions, rightInnerWidth, _ int) string {
	if m.scheduleView == nil || m.intentionList == nil || m.winsView == nil {
		return ""
	}

	scheduleView := centerView(rightInnerWidth, dims.ScheduleHeight-BorderHeight, m.scheduleView.View())
	intentionsView := centerView(rightInnerWidth, dims.IntentionsHeight-BorderHeight, m.intentionList.View())
	winsView := centerView(rightInnerWidth, dims.WinsHeight-BorderHeight, m.winsView.View())

	scheduleBox := m.getBorderStyle(SectionSchedule).Render(scheduleView)
	intentionsBox := m.getBorderStyle(SectionIntentions).Render(intentionsView)
	winsBox := m.getBorderStyle(SectionWins).Render(winsView)

	return lipgloss.JoinVertical(lipgloss.Top, scheduleBox, intentionsBox, winsBox)
}

// helpView renders the help overlay
func (m *Model) helpView() string {
	helpText := `Help - Key Bindings:

Navigation:
  h, ←       Previous day
  l, →       Next day
  T          Jump to today
  tab        Next section
  shift+tab  Previous section
  j, k       Navigate within section

 Actions:
   t          Add task
   i          Add intention
   w          Add win
   L          Add log entry
   n          Add note (Tasks/Logs section)
   e          Edit selected item (Tasks/Intentions/Wins/Logs sections)
    space      Toggle task completion (Tasks section)
    enter/space Toggle intention / Expand task (Intentions/Tasks section) / Expand log entry (Logs section)
    space      Expand log entry (Logs section)
   d          Delete selected item (with confirmation)

Text Entry:
  enter      Submit entry
  esc        Cancel entry
  any key    Type character

General:
  q, ctrl+c  Quit
  ?          Toggle help

Sections:
  - Logs: Activity log with stdtimestamps
  - Tasks: General todo list with collapsible notes
  - Notes: Linked notes and backlinks (wiki-style navigation)
  - Schedule: Upcoming items for the day
  - Intentions: 1-3 focus tasks for today
  - Wins: Daily accomplishments

Notes Section (when focused):
  - j/k: Navigate links
  - enter: Follow link
  - tab: Toggle section collapse
  - g/G: Jump to top/bottom

Press ? to exit help.`

	status := ""
	if m.statusBar != nil {
		status = m.statusBar.View()
	}

	return helpText + "\n\n" + status
}

// terminalTooSmallView renders the message when terminal is too small
func (m *Model) terminalTooSmallView() string {
	title := "Terminal Too Small"
	currentSize := fmt.Sprintf("Current: %dx%d", m.width, m.height)
	requiredSize := fmt.Sprintf("Required: %dx%d or larger", MinTerminalWidth, MinTerminalHeight)

	style := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Align(lipgloss.Center, lipgloss.Center)

	content := fmt.Sprintf(
		"%s\n\n%s\n\n%s\n\nResize your terminal and restart Reckon.",
		title,
		currentSize,
		requiredSize,
	)

	return style.Render(content)
}

// Message type definitions
type journalLoadedMsg struct {
	journal journal.Journal
}

type journalUpdatedMsg struct{}

type fileChangedMsg struct {
	date string
}

type errMsg struct {
	err error
}

// New journal task messages
// New journal task messages
type tasksLoadedMsg struct {
	tasks []journal.Task
}

type taskToggledMsg struct{}

type taskAddedMsg struct{}

type noteAddedMsg struct{}

type logNoteAddedMsg struct{}

type logNoteDeletedMsg struct{}

type TaskNoteDeleteMsg struct {
	TaskID string
	NoteID string
}

type taskNoteDeletedMsg struct{}

type taskScheduledMsg struct {
	date string
}

type taskDeadlineSetMsg struct {
	date string
}

type taskDateClearedMsg struct {
	clearedType string // "schedule", "deadline", or "both"
}

type clearSuccessMsg struct{}

type linksLoadedMsg struct {
	noteID    string
	outgoing  []components.LinkDisplayItem
	backlinks []components.LinkDisplayItem
}
