package tui

import (
	"fmt"
	"strings"
	stdtime "time"

	"github.com/MikeBiancalana/reckon/internal/journal"
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
	SectionIntentions Section = iota
	SectionWins
	SectionLogs
	SectionTasks
	SectionSchedule
	SectionCount // Keep this last to get the count
)

const (
	SectionNameIntentions = "Intentions"
	SectionNameWins       = "Wins"
	SectionNameLogs       = "Logs"
	SectionNameTasks      = "Tasks"
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

// Model represents the main TUI state
type Model struct {
	service        *journal.Service
	taskService    *journal.TaskService
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

	// State for modes
	helpMode          bool
	taskCreationMode  bool
	confirmMode       bool
	confirmItemType   string // "intention", "win", "log", "log_note"
	confirmItemID     string
	confirmLogEntryID string // For log_note deletion, stores the log entry ID
	editItemID        string // ID of item being edited
	editItemType      string // "task", "intention", "win", "log"
	noteTaskID        string // ID of task being noted
	noteLogEntryID    string // ID of log entry being noted
	lastError         error

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
		m.notesPane.UpdateNotes("", []journal.TaskNote{})
		return
	}
	if selectedTask.ID == m.selectedTaskID {
		return // No change
	}
	m.selectedTaskID = selectedTask.ID
	m.notesPane.UpdateNotes(selectedTask.ID, selectedTask.Notes)
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
	}
}

// SetJournalTaskService sets the journal task service
func (m *Model) SetJournalTaskService(taskService *journal.TaskService) {
	m.taskService = taskService
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

// renderNewLayout renders the 40-40-18 layout: Logs | Tasks | Schedule/Intentions/Wins
func (m *Model) renderNewLayout() string {
	dims := CalculatePaneDimensions(m.width, m.height, m.notesPaneVisible)

	// Border overhead: 2 chars width (left + right), 2 chars height (top + bottom)
	borderWidth := 2
	borderHeight := 2

	// Size components accounting for borders
	if m.taskList != nil {
		m.taskList.SetSize(dims.TasksWidth-borderWidth, dims.TasksHeight-borderHeight)
		m.taskList.SetFocused(m.focusedSection == SectionTasks)
	}
	if m.notesPane != nil {
		m.notesPane.SetSize(dims.NotesWidth-borderWidth, dims.NotesHeight-borderHeight)
		// Notes pane is not focusable, so no SetFocused
	}
	if m.scheduleView != nil {
		m.scheduleView.SetSize(dims.RightWidth-borderWidth, dims.ScheduleHeight-borderHeight)
		m.scheduleView.SetFocused(m.focusedSection == SectionSchedule)
	}
	if m.intentionList != nil {
		m.intentionList.SetSize(dims.RightWidth-borderWidth, dims.IntentionsHeight-borderHeight)
		m.intentionList.SetFocused(m.focusedSection == SectionIntentions)
	}
	if m.winsView != nil {
		m.winsView.SetSize(dims.RightWidth-borderWidth, dims.WinsHeight-borderHeight)
		m.winsView.SetFocused(m.focusedSection == SectionWins)
	}
	if m.logView != nil {
		m.logView.SetSize(dims.LogsWidth-borderWidth, dims.LogsHeight-borderHeight)
		m.logView.SetFocused(m.focusedSection == SectionLogs)
	}

	// Get pane views
	logsView := ""
	if m.logView != nil {
		logsView = m.logView.View()
	}

	tasksView := ""
	if m.taskList != nil {
		tasksView = m.taskList.View()
	}

	notesView := ""
	if m.notesPane != nil {
		notesView = m.notesPane.View()
	}

	// Calculate inner dimensions for centering
	logsInnerWidth := dims.LogsWidth - borderWidth
	logsInnerHeight := dims.LogsHeight - borderHeight
	tasksInnerWidth := dims.TasksWidth - borderWidth
	tasksInnerHeight := dims.TasksHeight - borderHeight
	notesInnerWidth := dims.NotesWidth - borderWidth
	notesInnerHeight := dims.NotesHeight - borderHeight
	rightInnerWidth := dims.RightWidth - borderWidth

	// Center and box Logs pane
	logsBox := m.getBorderStyle(SectionLogs).Render(
		centerView(logsInnerWidth, logsInnerHeight, logsView),
	)

	// Center and box Tasks pane (conditionally split vertically with notes)
	tasksCentered := centerView(tasksInnerWidth, tasksInnerHeight, tasksView)

	var centerContent string
	if m.notesPaneVisible {
		// Show notes pane with separator
		notesCentered := centerView(notesInnerWidth, notesInnerHeight, notesView)

		// Create separator with proper width matching the actual rendered width
		separatorWidth := tasksInnerWidth
		if separatorWidth > notesInnerWidth {
			separatorWidth = notesInnerWidth
		}
		separator := lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Width(separatorWidth).
			Align(lipgloss.Center).
			Render(strings.Repeat("─", separatorWidth))

		centerContent = lipgloss.JoinVertical(lipgloss.Left, tasksCentered, separator, notesCentered)
	} else {
		// Notes pane hidden, show only tasks
		centerContent = tasksCentered
	}

	tasksBox := m.getBorderStyle(SectionTasks).Render(centerContent)

	// Build right sidebar with centered, boxed components
	rightSidebar := m.buildRightSidebar(dims, rightInnerWidth, borderHeight)

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

	return content + "\n" + textEntry + "\n" + summary + "\n" + status
}

// buildRightSidebar constructs the vertically stacked right sidebar
func (m *Model) buildRightSidebar(dims PaneDimensions, rightInnerWidth, borderHeight int) string {
	if m.scheduleView == nil || m.intentionList == nil || m.winsView == nil {
		return ""
	}

	scheduleView := centerView(rightInnerWidth, dims.ScheduleHeight-borderHeight, m.scheduleView.View())
	intentionsView := centerView(rightInnerWidth, dims.IntentionsHeight-borderHeight, m.intentionList.View())
	winsView := centerView(rightInnerWidth, dims.WinsHeight-borderHeight, m.winsView.View())

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
  - Schedule: Upcoming items for the day
  - Intentions: 1-3 focus tasks for today
  - Wins: Daily accomplishments

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
