package tui

import (
	"fmt"
	stdtime "time"

	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/MikeBiancalana/reckon/internal/sync"
	"github.com/MikeBiancalana/reckon/internal/task"
	"github.com/MikeBiancalana/reckon/internal/time"
	"github.com/MikeBiancalana/reckon/internal/tui/components"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Section represents different sections of the journal
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

// Pane represents which pane is active in two-pane mode
type Pane int

const (
	PaneJournal Pane = iota
	PaneTask
)

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

	// Cached data
	tasks []journal.Task

	// Legacy Phase 2 task components (kept for backward compatibility)
	legacyTaskService *task.Service
	currentTask       *task.Task
	taskView          *components.TaskView
	taskPicker        *components.TaskPicker
	activePane        Pane
	showingTasks      bool // Two-pane mode enabled

	// State for modes
	helpMode         bool
	taskPickerMode   bool
	taskCreationMode bool
	confirmMode      bool
	confirmItemType  string // "intention", "win", "log"
	confirmItemID    string
	editItemID       string // ID of item being edited
	lastError        error

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
	sb.SetSection(SectionNameIntentions)
	sb.SetInputMode(false)

	return &Model{
		service:           service,
		taskService:       nil, // Will be set via SetTaskService
		legacyTaskService: nil, // Will be set via SetTaskService
		watcher:           watcher,
		currentDate:       stdtime.Now().Format("2006-01-02"),
		focusedSection:    SectionIntentions,
		textEntryBar:      components.NewTextEntryBar(),
		statusBar:         sb,
		activePane:        PaneJournal,
		showingTasks:      false,
		summaryView:       components.NewSummaryView(),
	}
}

// SetTaskService sets the legacy task service for task management features (backward compatibility)
func (m *Model) SetTaskService(taskService *task.Service) {
	m.legacyTaskService = taskService
}

// SetJournalTaskService sets the new journal task service
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

// loadJournal loads the journal for the current date
func (m *Model) loadJournal() tea.Cmd {
	return func() tea.Msg {
		j, err := m.service.GetByDate(m.currentDate)
		if err != nil {
			return errMsg{err}
		}
		return journalLoadedMsg{*j}
	}
}

// Update handles messages and updates the model
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Check if terminal meets minimum dimensions
		m.terminalTooSmall = msg.Width < MinTerminalWidth || msg.Height < MinTerminalHeight

		if m.statusBar != nil {
			m.statusBar.SetWidth(msg.Width)
		}

		// Only calculate pane dimensions if terminal is large enough
		if !m.terminalTooSmall {
			// Calculate pane dimensions
			paneWidthIntentions := int(float64(msg.Width) * 0.25)
			paneWidthWins := paneWidthIntentions
			paneWidthLogs := msg.Width - 2*paneWidthIntentions
			paneHeight := msg.Height - 2
			if m.intentionList != nil {
				m.intentionList.SetSize(paneWidthIntentions, paneHeight)
			}
			if m.winsView != nil {
				m.winsView.SetSize(paneWidthWins, paneHeight)
			}
			if m.logView != nil {
				m.logView.SetSize(paneWidthLogs, paneHeight)
			}
		}
		return m, nil

	case journalLoadedMsg:
		m.currentJournal = &msg.journal
		m.intentionList = components.NewIntentionList(msg.journal.Intentions)
		m.winsView = components.NewWinsView(msg.journal.Wins)
		m.logView = components.NewLogView(msg.journal.LogEntries)

		// Initialize new components for 40-40-18 layout
		m.scheduleView = components.NewScheduleView(msg.journal.ScheduleItems)
		// Initialize taskList with cached tasks (if available) or empty
		if m.taskList == nil {
			m.taskList = components.NewTaskList(m.tasks)
		} else {
			m.taskList.UpdateTasks(m.tasks)
		}

		// Calculate time summary
		daySummary := time.CalculateDaySummary(&msg.journal)
		if m.summaryView != nil {
			m.summaryView.SetSummary(&daySummary)
		}

		return m, nil

	case journalUpdatedMsg:
		// Reload journal after update
		return m, m.loadJournal()

	case tasksLoadedMsg:
		// New journal tasks loaded, update task list
		m.tasks = msg.tasks
		if m.taskList != nil {
			m.taskList.UpdateTasks(msg.tasks)
		} else {
			m.taskList = components.NewTaskList(msg.tasks)
		}
		return m, nil

	case components.TaskToggleMsg:
		// Task toggled, update in service
		if m.taskService != nil {
			return m, m.toggleTask(msg.TaskID)
		}
		return m, nil

	case taskToggledMsg:
		// Task toggled successfully, reload tasks
		return m, m.loadTasks()

	case taskAddedMsg:
		// Task added successfully, reload tasks
		return m, m.loadTasks()

	case noteAddedMsg:
		// Note added successfully, reload tasks
		return m, m.loadTasks()

	case legacyTasksLoadedMsg:
		// Legacy tasks loaded, show task picker
		m.taskPicker = components.NewTaskPicker(msg.tasks)
		m.taskPicker.SetSize(m.width/2, m.height-4)
		m.taskPickerMode = true
		return m, nil

	case taskLoadedMsg:
		// Task loaded, switch to two-pane mode (legacy)
		m.currentTask = &msg.task
		m.taskView = components.NewTaskView(&msg.task)
		m.showingTasks = true
		m.activePane = PaneTask
		m.taskPickerMode = false
		return m, nil

	case taskUpdatedMsg:
		// Task updated, reload task and journal (legacy)
		cmds := []tea.Cmd{
			m.loadTask(msg.taskID),
			m.loadJournal(),
		}
		return m, tea.Batch(cmds...)

	case fileChangedMsg:
		// Reload journal if the changed file is for the current date
		var cmd tea.Cmd
		if msg.date == m.currentDate {
			cmd = tea.Batch(m.loadJournal(), m.waitForFileChange())
		} else {
			cmd = m.waitForFileChange()
		}
		return m, cmd

	case errMsg:
		// Store error for display
		m.lastError = msg.err
		return m, nil

	case tea.KeyMsg:
		// Handle task picker mode first
		if m.taskPickerMode {
			switch msg.String() {
			case "enter":
				// Select task
				if m.taskPicker != nil {
					selectedTask := m.taskPicker.SelectedTask()
					if selectedTask != nil {
						return m, m.loadTask(selectedTask.ID)
					}
				}
				m.taskPickerMode = false
				return m, nil
			case "esc":
				// Cancel task picker
				m.taskPickerMode = false
				return m, nil
			default:
				// Delegate to task picker
				if m.taskPicker != nil {
					var cmd tea.Cmd
					m.taskPicker, cmd = m.taskPicker.Update(msg)
					return m, cmd
				}
			}
		}

		// Handle text entry bar mode
		if m.textEntryBar != nil && m.textEntryBar.IsFocused() {
			switch msg.String() {
			case "enter":
				// Submit text entry
				cmd := m.submitTextEntry()
				// Reset text entry bar
				m.textEntryBar.Clear()
				m.textEntryBar.Blur()
				m.textEntryBar.SetMode(components.ModeInactive)
				if m.statusBar != nil {
					m.statusBar.SetInputMode(false)
				}
				return m, cmd
			case "esc":
				// Cancel text entry
				m.textEntryBar.Clear()
				m.textEntryBar.Blur()
				m.textEntryBar.SetMode(components.ModeInactive)
				if m.statusBar != nil {
					m.statusBar.SetInputMode(false)
				}
				return m, nil
			default:
				// Delegate to text entry bar
				var cmd tea.Cmd
				m.textEntryBar, cmd = m.textEntryBar.Update(msg)
				return m, cmd
			}
		}

		// Handle confirmation mode
		if m.confirmMode {
			switch msg.String() {
			case "y", "Y":
				// Confirm deletion
				return m, m.deleteItem()
			case "n", "N", "esc":
				// Cancel deletion
				m.confirmMode = false
				m.confirmItemType = ""
				m.confirmItemID = ""
				return m, nil
			}
			// Ignore other keys in confirm mode
			return m, nil
		}

		// Normal mode
		switch msg.String() {
		case "q", "ctrl+c":
			// Stop watcher on quit
			if m.watcher != nil {
				m.watcher.Stop()
			}
			return m, tea.Quit
		case "tab":
			// In two-pane mode, switch between panes
			if m.showingTasks {
				if m.activePane == PaneJournal {
					m.activePane = PaneTask
				} else {
					m.activePane = PaneJournal
				}
				return m, nil
			}
			// Otherwise cycle sections
			m.focusedSection = (m.focusedSection + 1) % SectionCount
			if m.statusBar != nil {
				m.statusBar.SetSection(sectionName(m.focusedSection))
			}
			return m, nil
		case "shift+tab":
			m.focusedSection = (m.focusedSection + SectionCount - 1) % SectionCount
			if m.statusBar != nil {
				m.statusBar.SetSection(sectionName(m.focusedSection))
			}
			return m, nil
		case "ctrl+t":
			// Open task picker (legacy)
			if m.legacyTaskService != nil {
				return m, m.openTaskPicker()
			}
			return m, nil
		case "ctrl+n":
			// Create new task (legacy)
			if m.legacyTaskService != nil {
				m.textEntryBar.SetMode(components.ModeTask)
				m.textEntryBar.Clear()
				if m.statusBar != nil {
					m.statusBar.SetInputMode(true)
				}
				return m, m.textEntryBar.Focus()
			}
			return m, nil
		case "ctrl+w":
			// Close current task (exit two-pane mode)
			if m.showingTasks {
				m.showingTasks = false
				m.currentTask = nil
				m.taskView = nil
				m.activePane = PaneJournal
			}
			return m, nil
		case "h", "left":
			return m, m.prevDay()
		case "l", "right":
			return m, m.nextDay()
		case "T":
			// Jump to today (uppercase T)
			return m, m.jumpToToday()
		case "t":
			// Add task
			if m.textEntryBar != nil {
				m.textEntryBar.SetMode(components.ModeTask)
				m.textEntryBar.Clear()
				if m.statusBar != nil {
					m.statusBar.SetInputMode(true)
				}
				return m, m.textEntryBar.Focus()
			}
			return m, nil
		case "n":
			// Add note to task (if a task is selected)
			// For now, we'll handle this similar to tasks
			// TODO: Implement proper note-to-task workflow
			return m, nil
		case "?":
			m.helpMode = !m.helpMode
			return m, nil
		case "s":
			// Toggle time summary view
			if m.summaryView != nil {
				m.summaryView.Toggle()
			}
			return m, nil
		case "i":
			// Add intention
			if m.textEntryBar != nil {
				m.textEntryBar.SetMode(components.ModeIntention)
				m.textEntryBar.Clear()
				if m.statusBar != nil {
					m.statusBar.SetInputMode(true)
				}
				return m, m.textEntryBar.Focus()
			}
			return m, nil
		case "w":
			// Add win
			if m.textEntryBar != nil {
				m.textEntryBar.SetMode(components.ModeWin)
				m.textEntryBar.Clear()
				if m.statusBar != nil {
					m.statusBar.SetInputMode(true)
				}
				return m, m.textEntryBar.Focus()
			}
			return m, nil
		case "L":
			// Add log
			if m.textEntryBar != nil {
				m.textEntryBar.SetMode(components.ModeLog)
				m.textEntryBar.Clear()
				if m.statusBar != nil {
					m.statusBar.SetInputMode(true)
				}
				return m, m.textEntryBar.Focus()
			}
			return m, nil
		case "d":
			// Delete selected item with confirmation
			if m.confirmMode {
				// Already in confirm mode, ignore
				return m, nil
			}
			switch m.focusedSection {
			case SectionIntentions:
				if m.intentionList != nil {
					intention := m.intentionList.SelectedIntention()
					if intention != nil {
						m.confirmMode = true
						m.confirmItemType = "intention"
						m.confirmItemID = intention.ID
					}
				}
			case SectionWins:
				if m.winsView != nil {
					win := m.winsView.SelectedWin()
					if win != nil {
						m.confirmMode = true
						m.confirmItemType = "win"
						m.confirmItemID = win.ID
					}
				}
			case SectionLogs:
				if m.logView != nil {
					entry := m.logView.SelectedLogEntry()
					if entry != nil {
						m.confirmMode = true
						m.confirmItemType = "log"
						m.confirmItemID = entry.ID
					}
				}
			}
			return m, nil
		case "e":
			// TODO: Edit functionality needs to be refactored to use TextEntryBar
			// For now, edit is disabled
			return m, nil
		case "enter":
			// Handle enter key for toggling intentions
			if m.focusedSection == SectionIntentions && m.intentionList != nil {
				intention := m.intentionList.SelectedIntention()
				if intention != nil {
					return m, m.toggleIntention(intention.ID)
				}
			}
		default:
			// Delegate to focused component
			switch m.focusedSection {
			case SectionIntentions:
				if m.intentionList != nil {
					var cmd tea.Cmd
					m.intentionList, cmd = m.intentionList.Update(msg)
					return m, cmd
				}
			case SectionWins:
				if m.winsView != nil {
					var cmd tea.Cmd
					m.winsView, cmd = m.winsView.Update(msg)
					return m, cmd
				}
			case SectionLogs:
				if m.logView != nil {
					var cmd tea.Cmd
					m.logView, cmd = m.logView.Update(msg)
					return m, cmd
				}
			case SectionTasks:
				if m.taskList != nil {
					var cmd tea.Cmd
					m.taskList, cmd = m.taskList.Update(msg)
					return m, cmd
				}
			case SectionSchedule:
				// ScheduleView doesn't have interactive elements yet
				// So we don't delegate to it
			}
		}
	}

	return m, nil
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

	// Handle task picker overlay
	if m.taskPickerMode && m.taskPicker != nil {
		// Render task picker as overlay
		pickerView := m.taskPicker.View()

		// Center it on screen
		pickerStyle := lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("12")).
			Padding(1, 2).
			Width(m.width / 2)

		return lipgloss.Place(
			m.width,
			m.height,
			lipgloss.Center,
			lipgloss.Center,
			pickerStyle.Render(pickerView),
		)
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

	// Check if new 40-40-18 layout components are available
	if m.taskList != nil && m.scheduleView != nil {
		return m.renderNewLayout()
	}

	var content string
	paneHeight := m.height - 2

	// Two-pane mode: Journal (left) + Task (right)
	if m.showingTasks && m.taskView != nil {
		// 50/50 split
		journalWidth := m.width / 2
		taskWidth := m.width - journalWidth

		// Render journal pane (simplified - just log view)
		if m.logView != nil {
			m.logView.SetSize(journalWidth-1, paneHeight)
		}
		journalView := ""
		if m.logView != nil {
			journalView = m.logView.View()
		}

		// Render task pane
		m.taskView.SetSize(taskWidth-1, paneHeight)
		taskView := m.taskView.View()

		// Add focus indicator
		journalStyle := lipgloss.NewStyle().BorderRight(true).BorderStyle(lipgloss.NormalBorder())
		if m.activePane == PaneJournal {
			journalStyle = journalStyle.BorderForeground(lipgloss.Color("12"))
		} else {
			journalStyle = journalStyle.BorderForeground(lipgloss.Color("8"))
		}

		content = lipgloss.JoinHorizontal(
			lipgloss.Top,
			journalStyle.Render(journalView),
			taskView,
		)
	} else {
		// Original three-pane mode
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
	}

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
		style = style.BorderForeground(lipgloss.Color("39")) // blue color for focus
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
	dims := CalculatePaneDimensions(m.width, m.height)

	// Border overhead: 2 chars width (left + right), 2 chars height (top + bottom)
	borderWidth := 2
	borderHeight := 2

	// Size components accounting for borders
	if m.taskList != nil {
		m.taskList.SetSize(dims.TasksWidth-borderWidth, dims.TasksHeight-borderHeight)
	}
	if m.scheduleView != nil {
		m.scheduleView.SetSize(dims.RightWidth-borderWidth, dims.ScheduleHeight-borderHeight)
	}
	if m.intentionList != nil {
		m.intentionList.SetSize(dims.RightWidth-borderWidth, dims.IntentionsHeight-borderHeight)
	}
	if m.winsView != nil {
		m.winsView.SetSize(dims.RightWidth-borderWidth, dims.WinsHeight-borderHeight)
	}
	if m.logView != nil {
		m.logView.SetSize(dims.LogsWidth-borderWidth, dims.LogsHeight-borderHeight)
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

	// Calculate inner dimensions for centering
	logsInnerWidth := dims.LogsWidth - borderWidth
	logsInnerHeight := dims.LogsHeight - borderHeight
	tasksInnerWidth := dims.TasksWidth - borderWidth
	tasksInnerHeight := dims.TasksHeight - borderHeight
	rightInnerWidth := dims.RightWidth - borderWidth

	// Center and box Logs pane
	logsBox := m.getBorderStyle(SectionLogs).Render(
		centerView(logsInnerWidth, logsInnerHeight, logsView),
	)

	// Center and box Tasks pane
	tasksBox := m.getBorderStyle(SectionTasks).Render(
		centerView(tasksInnerWidth, tasksInnerHeight, tasksView),
	)

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
  space      Toggle task completion (Tasks section)
  enter      Toggle intention / Expand task (Intentions/Tasks section)
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

// Helper functions for navigation
func (m *Model) prevDay() tea.Cmd {
	date, err := stdtime.Parse("2006-01-02", m.currentDate)
	if err != nil {
		// If current date is corrupted, fall back to today
		m.currentDate = stdtime.Now().Format("2006-01-02")
		return m.loadJournal()
	}
	newDate := date.AddDate(0, 0, -1).Format("2006-01-02")
	m.currentDate = newDate
	return m.loadJournal()
}

func (m *Model) nextDay() tea.Cmd {
	date, err := stdtime.Parse("2006-01-02", m.currentDate)
	if err != nil {
		// If current date is corrupted, fall back to today
		m.currentDate = stdtime.Now().Format("2006-01-02")
		return m.loadJournal()
	}
	today := stdtime.Now().Format("2006-01-02")
	newDate := date.AddDate(0, 0, 1).Format("2006-01-02")

	// Don't go beyond today
	if newDate > today {
		return nil
	}

	m.currentDate = newDate
	return m.loadJournal()
}

func (m *Model) jumpToToday() tea.Cmd {
	m.currentDate = stdtime.Now().Format("2006-01-02")
	return m.loadJournal()
}

func (m *Model) toggleIntention(intentionID string) tea.Cmd {
	return func() tea.Msg {
		err := m.service.ToggleIntention(m.currentJournal, intentionID)
		if err != nil {
			return errMsg{err}
		}
		return journalUpdatedMsg{}
	}
}

// submitInput is a legacy function for backward compatibility with old task mode
// This should eventually be removed when legacy task functionality is fully deprecated
func (m *Model) submitInput() tea.Cmd {
	// This function is no longer used with the new text entry bar
	// It's kept for backward compatibility with legacy task mode only
	return func() tea.Msg {
		return errMsg{fmt.Errorf("submitInput is deprecated - use submitTextEntry instead")}
	}
}

// deleteItem deletes the item in confirmation mode
func (m *Model) deleteItem() tea.Cmd {
	return func() tea.Msg {
		var err error
		switch m.confirmItemType {
		case "intention":
			err = m.service.DeleteIntention(m.currentJournal, m.confirmItemID)
		case "win":
			err = m.service.DeleteWin(m.currentJournal, m.confirmItemID)
		case "log":
			err = m.service.DeleteLogEntry(m.currentJournal, m.confirmItemID)
		}

		// Reset confirmation state
		m.confirmMode = false
		m.confirmItemType = ""
		m.confirmItemID = ""

		if err != nil {
			return errMsg{err}
		}
		return journalUpdatedMsg{}
	}
}

// openTaskPicker loads active tasks and opens the task picker (legacy)
func (m *Model) openTaskPicker() tea.Cmd {
	return func() tea.Msg {
		if m.legacyTaskService == nil {
			return errMsg{fmt.Errorf("task service not available")}
		}

		// Get active tasks
		status := task.StatusActive
		tasks, err := m.legacyTaskService.List(&status, []string{})
		if err != nil {
			return errMsg{err}
		}

		return legacyTasksLoadedMsg{tasks: tasks}
	}
}

// loadTask loads a task by ID (legacy)
func (m *Model) loadTask(taskID string) tea.Cmd {
	return func() tea.Msg {
		if m.legacyTaskService == nil {
			return errMsg{fmt.Errorf("task service not available")}
		}

		t, err := m.legacyTaskService.GetByID(taskID)
		if err != nil {
			return errMsg{err}
		}

		return taskLoadedMsg{task: *t}
	}
}

// Messages
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

// Legacy task messages for backward compatibility
type legacyTasksLoadedMsg struct {
	tasks []task.Task
}

type taskLoadedMsg struct {
	task task.Task
}

type taskUpdatedMsg struct {
	taskID string
}

// New journal task messages
type tasksLoadedMsg struct {
	tasks []journal.Task
}

type taskToggledMsg struct{}

type taskAddedMsg struct{}

type noteAddedMsg struct{}

// loadTasks loads all tasks from the task service
func (m *Model) loadTasks() tea.Cmd {
	return func() tea.Msg {
		if m.taskService == nil {
			return errMsg{fmt.Errorf("task service not available")}
		}

		tasks, err := m.taskService.GetAllTasks()
		if err != nil {
			return errMsg{err}
		}

		return tasksLoadedMsg{tasks: tasks}
	}
}

// toggleTask toggles a task's status
func (m *Model) toggleTask(taskID string) tea.Cmd {
	return func() tea.Msg {
		if m.taskService == nil {
			return errMsg{fmt.Errorf("task service not available")}
		}

		err := m.taskService.ToggleTask(taskID)
		if err != nil {
			return errMsg{err}
		}

		return taskToggledMsg{}
	}
}

// submitTextEntry submits the text entry based on the current mode
func (m *Model) submitTextEntry() tea.Cmd {
	if m.textEntryBar == nil {
		return nil
	}

	inputText := m.textEntryBar.GetValue()
	if inputText == "" {
		return nil
	}

	mode := m.textEntryBar.GetMode()

	return func() tea.Msg {
		var err error

		switch mode {
		case components.ModeTask:
			// Add task
			if m.taskService != nil {
				err = m.taskService.AddTask(inputText)
				if err != nil {
					return errMsg{err}
				}
				// Reload tasks
				tasks, errGetTasks := m.taskService.GetAllTasks()
				if errGetTasks != nil {
					return errMsg{errGetTasks}
				}
				return tasksLoadedMsg{tasks: tasks}
			}
			return errMsg{fmt.Errorf("task service not available")}

		case components.ModeIntention:
			err = m.service.AddIntention(m.currentJournal, inputText)

		case components.ModeWin:
			err = m.service.AddWin(m.currentJournal, inputText)

		case components.ModeLog:
			err = m.service.AppendLog(m.currentJournal, inputText)

		default:
			return errMsg{fmt.Errorf("unknown entry mode")}
		}

		if err != nil {
			return errMsg{err}
		}
		return journalUpdatedMsg{}
	}
}

// waitForFileChange waits for file change events from the watcher
func (m *Model) waitForFileChange() tea.Cmd {
	if m.watcher == nil {
		return nil
	}

	return func() tea.Msg {
		event := <-m.watcher.Changes()
		return fileChangedMsg{date: event.Date}
	}
}
