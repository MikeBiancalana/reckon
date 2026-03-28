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
	SectionLogs  Section = iota // 0
	SectionTasks                // 1
	SectionCount                // Keep this last to get the count
)

const (
	SectionNameLogs  = "Logs"
	SectionNameTasks = "Tasks"
)

// sectionName returns the display name for a section
func sectionName(s Section) string {
	switch s {
	case SectionLogs:
		return SectionNameLogs
	case SectionTasks:
		return SectionNameTasks
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
	logView      *components.LogView
	textEntryBar *components.TextEntryBar
	statusBar    *components.StatusBar

	// Main layout components
	summaryView *components.SummaryView

	// Simple task list state (replaces taskList component)
	tasks            []journal.Task
	selectedIndex    int
	taskScrollOffset int

	// Date picker state
	datePicker           *components.DatePicker
	datePickerVisible    bool
	datePickerMode       string // "schedule" or "deadline"
	datePickerTargetTask string // ID of task being scheduled/deadline-set

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

// NewModel creates a new TUI model
func NewModel(service *journal.Service) *Model {
	// Create watcher
	watcher, err := sync.NewWatcher(service)
	if err != nil {
		// Log error but continue without watcher
		watcher = nil
	}

	sb := components.NewStatusBar()
	sb.SetSection(SectionNameLogs)
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

	if m.helpMode {
		return m.helpView()
	}

	// Use multi-section split view when journal is loaded
	if m.currentJournal != nil {
		return m.renderNewLayout()
	}

	// Minimal fallback when journal not yet loaded
	return "Loading..."
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

// renderNewLayout renders the 50-50 layout: Logs | Tasks
func (m *Model) renderNewLayout() string {
	dims := CalculatePaneDimensions(m.width, m.height)

	// Size log view accounting for borders
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

	// Center and box Logs pane
	logsBox := m.getBorderStyle(SectionLogs).Render(
		centerView(logsInnerWidth, logsInnerHeight, logsView),
	)

	// New rendering: task list + detail area stacked vertically
	detailHeight := tasksInnerHeight / 3
	if detailHeight < 3 {
		detailHeight = 3
	}
	taskListHeight := tasksInnerHeight - detailHeight - 1 // -1 for separator
	if taskListHeight < 1 {
		taskListHeight = 1
	}

	taskListContent := m.renderTaskList(tasksInnerWidth, taskListHeight)
	separatorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	separator := separatorStyle.Render(strings.Repeat("─", tasksInnerWidth))
	detailContent := m.renderDetailArea(tasksInnerWidth, detailHeight)

	centerContent := strings.Join([]string{taskListContent, separator, detailContent}, "\n")
	tasksCentered := centerView(tasksInnerWidth, tasksInnerHeight, centerContent)
	tasksBox := m.getBorderStyle(SectionTasks).Render(tasksCentered)

	// Join main panes horizontally
	content := lipgloss.JoinHorizontal(
		lipgloss.Top,
		logsBox,
		tasksBox,
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

	parts := []string{content}
	if textEntry != "" {
		parts = append(parts, textEntry)
	}
	if successMsg != "" {
		parts = append(parts, successMsg)
	}
	if summary != "" {
		parts = append(parts, summary)
	}
	if status != "" {
		parts = append(parts, status)
	}
	mainView := strings.Join(parts, "\n")

	// Overlay date picker if visible
	if m.datePickerVisible && m.datePicker != nil {
		overlay := m.datePicker.View()
		// Center the date picker overlay using Place
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlay)
	}

	return mainView
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
  e          Edit selected item (Tasks/Logs sections)
  space      Toggle task completion (Tasks section)
  enter      Expand log entry (Logs section)
  d          Delete selected item (with confirmation)
  s          Schedule task (Tasks section)
  D          Set deadline (Tasks section)

Text Entry:
  enter      Submit entry
  esc        Cancel entry
  any key    Type character

General:
  q, ctrl+c  Quit
  ?          Toggle help

Sections:
  - Logs: Activity log with timestamps
  - Tasks: Todo list with notes

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

// clampIndex returns idx clamped to [0, length-1], or -1 if length == 0.
func clampIndex(idx, length int) int {
	if length == 0 {
		return -1
	}
	if idx < 0 {
		return 0
	}
	if idx >= length {
		return length - 1
	}
	return idx
}

// selectedTask returns the currently selected task, or nil if nothing is selected.
func (m *Model) selectedTask() *journal.Task {
	if m.selectedIndex < 0 || m.selectedIndex >= len(m.tasks) {
		return nil
	}
	return &m.tasks[m.selectedIndex]
}

// renderTaskList renders the task list using m.tasks and m.selectedIndex.
// It applies scroll-follow-cursor logic to keep the selected task visible.
func (m *Model) renderTaskList(width, height int) string {
	if len(m.tasks) == 0 {
		noTasksStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))
		return noTasksStyle.Render("No tasks - press t to add one")
	}

	// Adjust scroll offset so selected item is visible
	if m.selectedIndex >= 0 {
		if m.selectedIndex < m.taskScrollOffset {
			m.taskScrollOffset = m.selectedIndex
		}
		if m.selectedIndex >= m.taskScrollOffset+height {
			m.taskScrollOffset = m.selectedIndex - height + 1
		}
	}

	taskNormalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39"))
	taskDoneStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Strikethrough(true)

	var lines []string
	end := m.taskScrollOffset + height
	if end > len(m.tasks) {
		end = len(m.tasks)
	}
	for i := m.taskScrollOffset; i < end; i++ {
		task := m.tasks[i]
		checkbox := "[ ]"
		style := taskNormalStyle
		if task.Status == journal.TaskDone {
			checkbox = "[x]"
			style = taskDoneStyle
		}

		line := fmt.Sprintf("%s %s", checkbox, task.Text)
		if len(task.Tags) > 0 {
			line += fmt.Sprintf(" [%s]", strings.Join(task.Tags, " "))
		}
		if task.Status != journal.TaskDone {
			dateInfo := components.FormatDateInfo(task)
			if dateInfo != "" {
				line += "  " + dateInfo
				style = components.GetDateStyle(task)
			}
		}

		if i == m.selectedIndex {
			line = components.SelectedStyle.Render(line)
		} else {
			line = style.Render(line)
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// renderDetailArea renders the notes for the currently selected task.
func (m *Model) renderDetailArea(width, height int) string {
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Bold(true)

	task := m.selectedTask()
	if task == nil {
		return titleStyle.Render("━━ NOTES ━━") + "\n" +
			lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("  No task selected")
	}

	header := titleStyle.Render("━━ NOTES ━━")
	if len(task.Notes) == 0 {
		return header + "\n" +
			lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("  No notes")
	}

	noteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	var lines []string
	lines = append(lines, header)
	for _, note := range task.Notes {
		lines = append(lines, noteStyle.Render("  - "+note.Text))
	}

	// Clamp to height
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}
