package tui

import (
	"fmt"
	"time"

	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/MikeBiancalana/reckon/internal/sync"
	"github.com/MikeBiancalana/reckon/internal/task"
	"github.com/MikeBiancalana/reckon/internal/tui/components"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Section represents different sections of the journal
type Section int

const (
	SectionIntentions Section = iota
	SectionWins
	SectionLogs
	SectionCount // Keep this last to get the count
)

const (
	SectionNameIntentions = "Intentions"
	SectionNameWins       = "Wins"
	SectionNameLogs       = "Logs"
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
	taskService    *task.Service
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
	textInput     textinput.Model
	statusBar     *components.StatusBar

	// New components for 40-40-18 layout
	taskList     *components.TaskList
	scheduleView *components.ScheduleView

	// Task components (legacy Phase 2)
	currentTask  *task.Task
	taskView     *components.TaskView
	taskPicker   *components.TaskPicker
	activePane   Pane
	showingTasks bool // Two-pane mode enabled

	// State for input modes
	inputMode        bool
	inputType        string // "intention", "win", "log", "task", "task_log"
	helpMode         bool
	taskPickerMode   bool
	taskCreationMode bool
	lastError        error

	// Terminal size validation
	terminalTooSmall bool
}

// NewModel creates a new TUI model
func NewModel(service *journal.Service) *Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = ""
	ti.CharLimit = 200
	ti.Width = 50

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
		taskService:    nil, // Will be set via SetTaskService
		watcher:        watcher,
		currentDate:    time.Now().Format("2006-01-02"),
		focusedSection: SectionIntentions,
		textInput:      ti,
		statusBar:      sb,
		activePane:     PaneJournal,
		showingTasks:   false,
	}
}

// SetTaskService sets the task service for task management features
func (m *Model) SetTaskService(taskService *task.Service) {
	m.taskService = taskService
}

// Init initializes the model
func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.loadJournal()}

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
		m.taskList = components.NewTaskList([]journal.Task{}) // Start with empty, will be loaded separately

		return m, nil

	case journalUpdatedMsg:
		// Reset input state after successful submission
		if m.inputMode {
			m.inputMode = false
			m.textInput.SetValue("")
			m.textInput.Blur()
			if m.statusBar != nil {
				m.statusBar.SetInputMode(false)
			}
		}
		return m, m.loadJournal()

	case tasksLoadedMsg:
		// Tasks loaded, show task picker
		m.taskPicker = components.NewTaskPicker(msg.tasks)
		m.taskPicker.SetSize(m.width/2, m.height-4)
		m.taskPickerMode = true
		return m, nil

	case taskLoadedMsg:
		// Task loaded, switch to two-pane mode
		if m.inputMode {
			m.inputMode = false
			m.textInput.SetValue("")
			m.textInput.Blur()
			if m.statusBar != nil {
				m.statusBar.SetInputMode(false)
			}
		}
		m.currentTask = &msg.task
		m.taskView = components.NewTaskView(&msg.task)
		m.showingTasks = true
		m.activePane = PaneTask
		m.taskPickerMode = false
		return m, nil

	case taskUpdatedMsg:
		// Task updated, reload task and journal
		cmds := []tea.Cmd{
			m.loadTask(msg.taskID),
			m.loadJournal(),
		}
		if m.inputMode {
			m.inputMode = false
			m.textInput.SetValue("")
			m.textInput.Blur()
			if m.statusBar != nil {
				m.statusBar.SetInputMode(false)
			}
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
		// Reset input state and store error for display
		if m.inputMode {
			m.inputMode = false
			m.textInput.SetValue("")
			m.textInput.Blur()
			if m.statusBar != nil {
				m.statusBar.SetInputMode(false)
			}
		}
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

		// Handle input mode
		if m.inputMode {
			switch msg.String() {
			case "enter":
				// Submit input
				return m, m.submitInput()
			case "esc":
				// Cancel input
				m.inputMode = false
				m.textInput.SetValue("")
				m.textInput.Blur()
				if m.statusBar != nil {
					m.statusBar.SetInputMode(false)
				}
				return m, nil
			default:
				// Delegate to textinput for editing
				var cmd tea.Cmd
				m.textInput, cmd = m.textInput.Update(msg)
				return m, cmd
			}
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
			// Open task picker
			if m.taskService != nil {
				return m, m.openTaskPicker()
			}
			return m, nil
		case "ctrl+n":
			// Create new task
			if m.taskService != nil {
				m.inputMode = true
				m.inputType = "task"
				m.textInput.Prompt = "New task title: "
				m.textInput.Placeholder = "Enter task title"
				m.textInput.SetValue("")
				m.textInput.Focus()
				if m.statusBar != nil {
					m.statusBar.SetInputMode(true)
				}
				return m, textinput.Blink
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
		case "t":
			return m, m.jumpToToday()
		case "?":
			m.helpMode = !m.helpMode
			return m, nil
		case "i":
			// Add intention
			m.inputMode = true
			m.inputType = "intention"
			m.textInput.Prompt = "Add intention: "
			m.textInput.Placeholder = "What do you intend to accomplish?"
			m.textInput.SetValue("")
			m.textInput.Focus()
			if m.statusBar != nil {
				m.statusBar.SetInputMode(true)
			}
			return m, textinput.Blink
		case "w":
			// Add win
			m.inputMode = true
			m.inputType = "win"
			m.textInput.Prompt = "Add win: "
			m.textInput.Placeholder = "What did you accomplish?"
			m.textInput.SetValue("")
			m.textInput.Focus()
			if m.statusBar != nil {
				m.statusBar.SetInputMode(true)
			}
			return m, textinput.Blink
		case "L":
			// Add log - if in task pane and task is loaded, log to task
			if m.showingTasks && m.activePane == PaneTask && m.currentTask != nil {
				m.inputMode = true
				m.inputType = "task_log"
				m.textInput.Prompt = "Log to task: "
				m.textInput.Placeholder = "What did you do?"
				m.textInput.SetValue("")
				m.textInput.Focus()
				if m.statusBar != nil {
					m.statusBar.SetInputMode(true)
				}
				return m, textinput.Blink
			}
			// Otherwise log to journal
			m.inputMode = true
			m.inputType = "log"
			m.textInput.Prompt = "Add log entry: "
			m.textInput.Placeholder = "What did you do?"
			m.textInput.SetValue("")
			m.textInput.Focus()
			if m.statusBar != nil {
				m.statusBar.SetInputMode(true)
			}
			return m, textinput.Blink
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

	if m.inputMode {
		view := m.textInput.View() + "\n\n(Enter to submit, Esc to cancel)"
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

// renderNewLayout renders the 40-40-18 layout: Logs | Tasks | Schedule/Intentions/Wins
func (m *Model) renderNewLayout() string {
	dims := CalculatePaneDimensions(m.width, m.height)

	// Size components
	if m.taskList != nil {
		m.taskList.SetSize(dims.TasksWidth, dims.TasksHeight)
	}
	if m.scheduleView != nil {
		m.scheduleView.SetSize(dims.RightWidth, dims.ScheduleHeight)
	}
	if m.intentionList != nil {
		m.intentionList.SetSize(dims.RightWidth, dims.IntentionsHeight)
	}
	if m.winsView != nil {
		m.winsView.SetSize(dims.RightWidth, dims.WinsHeight)
	}
	if m.logView != nil {
		m.logView.SetSize(dims.LogsWidth, dims.LogsHeight)
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

	// Stack right sidebar vertically
	rightSidebar := ""
	if m.scheduleView != nil && m.intentionList != nil && m.winsView != nil {
		scheduleView := m.scheduleView.View()
		intentionsView := m.intentionList.View()
		winsView := m.winsView.View()

		rightSidebar = lipgloss.JoinVertical(
			lipgloss.Top,
			scheduleView,
			intentionsView,
			winsView,
		)
	}

	// Join main panes horizontally
	borderStyle := lipgloss.NewStyle().BorderRight(true).BorderStyle(lipgloss.NormalBorder())
	content := lipgloss.JoinHorizontal(
		lipgloss.Top,
		borderStyle.Render(logsView),
		borderStyle.Render(tasksView),
		rightSidebar,
	)

	// Add status bar
	status := ""
	if m.statusBar != nil {
		m.statusBar.SetDate(m.currentDate)
		status = m.statusBar.View()
	}

	return content + "\n" + status
}

// helpView renders the help overlay
func (m *Model) helpView() string {
	helpText := `Help - Key Bindings:

Navigation:
  h, ←       Previous day
  l, →       Next day
  t          Jump to today
  tab        Next section / Switch panes (in two-pane mode)
  shift+tab  Previous section

Actions:
  i          Add intention
  w          Add win
  L          Add log entry (or log to task if in task pane)
  enter      Toggle intention (in intentions section)

Task Management:
  ctrl+t     Open task picker
  ctrl+n     Create new task
  ctrl+w     Close task (exit two-pane mode)

Input Mode:
  enter      Submit
  esc        Cancel
  backspace  Delete character
  any key    Add character

General:
  q, ctrl+c  Quit
  ?          Toggle help

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
	date, _ := time.Parse("2006-01-02", m.currentDate)
	newDate := date.AddDate(0, 0, -1).Format("2006-01-02")
	m.currentDate = newDate
	return m.loadJournal()
}

func (m *Model) nextDay() tea.Cmd {
	date, _ := time.Parse("2006-01-02", m.currentDate)
	today := time.Now().Format("2006-01-02")
	newDate := date.AddDate(0, 0, 1).Format("2006-01-02")

	// Don't go beyond today
	if newDate > today {
		return nil
	}

	m.currentDate = newDate
	return m.loadJournal()
}

func (m *Model) jumpToToday() tea.Cmd {
	m.currentDate = time.Now().Format("2006-01-02")
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

func (m *Model) submitInput() tea.Cmd {
	inputText := m.textInput.Value()
	if inputText == "" {
		return nil
	}

	return func() tea.Msg {
		var err error
		switch m.inputType {
		case "intention":
			err = m.service.AddIntention(m.currentJournal, inputText)
		case "win":
			err = m.service.AddWin(m.currentJournal, inputText)
		case "log":
			err = m.service.AppendLog(m.currentJournal, inputText)
		case "task":
			// Create new task
			if m.taskService != nil {
				t, err := m.taskService.Create(inputText, []string{})
				if err != nil {
					return errMsg{err}
				}
				return taskLoadedMsg{task: *t}
			}
		case "task_log":
			// Log to current task
			if m.taskService != nil && m.currentTask != nil {
				err = m.taskService.AppendLog(m.currentTask.ID, inputText)
				if err != nil {
					return errMsg{err}
				}
				// Reload both task and journal
				return taskUpdatedMsg{taskID: m.currentTask.ID}
			}
		}

		if err != nil {
			return errMsg{err}
		}
		return journalUpdatedMsg{}
	}
}

// openTaskPicker loads active tasks and opens the task picker
func (m *Model) openTaskPicker() tea.Cmd {
	return func() tea.Msg {
		if m.taskService == nil {
			return errMsg{fmt.Errorf("task service not available")}
		}

		// Get active tasks
		status := task.StatusActive
		tasks, err := m.taskService.List(&status, []string{})
		if err != nil {
			return errMsg{err}
		}

		return tasksLoadedMsg{tasks: tasks}
	}
}

// loadTask loads a task by ID
func (m *Model) loadTask(taskID string) tea.Cmd {
	return func() tea.Msg {
		if m.taskService == nil {
			return errMsg{fmt.Errorf("task service not available")}
		}

		t, err := m.taskService.GetByID(taskID)
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

type tasksLoadedMsg struct {
	tasks []task.Task
}

type taskLoadedMsg struct {
	task task.Task
}

type taskUpdatedMsg struct {
	taskID string
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
