package tui

import (
	"fmt"

	"github.com/MikeBiancalana/reckon/internal/logger"
	"github.com/MikeBiancalana/reckon/internal/tui/components"
	tea "github.com/charmbracelet/bubbletea"
)

// Keyboard Handlers
//
// These methods handle keyboard input organized by mode and context.
// The main handleKeyPress dispatcher routes to specific handlers based
// on the current state (text entry mode, confirm mode, normal mode).

// handleKeyPress is the main keyboard input dispatcher
func (m *Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle date picker mode first (highest priority after text entry)
	if m.datePickerVisible && m.datePicker != nil {
		return m.handleDatePickerKeys(msg)
	}

	// Handle text entry bar mode (highest priority)
	if m.textEntryBar != nil && m.textEntryBar.IsFocused() {
		return m.handleTextEntryKeys(msg)
	}

	// Handle clear date submenu mode
	if m.clearDateMode {
		return m.handleClearDateKeys(msg)
	}

	// Handle confirmation mode
	if m.confirmMode {
		return m.handleConfirmKeys(msg)
	}

	// Normal mode keyboard handling
	return m.handleNormalModeKeys(msg)
}

// handleTextEntryKeys handles keyboard input when text entry bar is focused
func (m *Model) handleTextEntryKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		// Submit text entry
		cmd := m.submitTextEntry()

		// Reset text entry bar state
		m.textEntryBar.Clear()
		m.textEntryBar.Blur()
		m.textEntryBar.SetMode(components.ModeInactive)
		m.noteTaskID = ""
		m.noteLogEntryID = ""
		m.editItemID = ""
		m.editItemType = ""

		if m.statusBar != nil {
			m.statusBar.SetInputMode(false)
		}

		return m, cmd

	case "esc":
		// Cancel text entry
		m.textEntryBar.Clear()
		m.textEntryBar.Blur()
		m.textEntryBar.SetMode(components.ModeInactive)
		m.noteTaskID = ""
		m.noteLogEntryID = ""
		m.editItemID = ""
		m.editItemType = ""

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

// handleConfirmKeys handles keyboard input in confirmation mode
func (m *Model) handleConfirmKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		// Confirm deletion
		return m, m.deleteItem()

	case "n", "N", "esc":
		// Cancel deletion
		logger.Debug("tui: cancelled deletion in confirm mode", "itemType", m.confirmItemType)
		m.confirmMode = false
		m.confirmItemType = ""
		m.confirmItemID = ""
		m.confirmLogEntryID = ""
		return m, nil
	}

	// Ignore other keys in confirm mode
	return m, nil
}

// handleNormalModeKeys handles keyboard input in normal mode
func (m *Model) handleNormalModeKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle special key types first
	switch msg.Type {
	case tea.KeyCtrlC:
		return m.handleQuit()

	case tea.KeySpace:
		return m.handleSpaceKey(msg)

	case tea.KeyEnter:
		return m.handleEnterKey(msg)
	}

	// Handle key strings (runes)
	return m.handleKeyString(msg)
}

// handleQuit handles quit operations
func (m *Model) handleQuit() (tea.Model, tea.Cmd) {
	// Stop watcher on quit
	if m.watcher != nil {
		m.watcher.Stop()
	}
	return m, tea.Quit
}

// handleSpaceKey handles space key in different contexts
func (m *Model) handleSpaceKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle space key for tasks (toggle completion)
	if m.focusedSection == SectionTasks && m.taskList != nil {
		var cmd tea.Cmd
		m.taskList, cmd = m.taskList.Update(msg)
		return m, cmd
	}
	return m, nil
}

// handleEnterKey handles enter key in different contexts
func (m *Model) handleEnterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle enter key for toggling intentions
	if m.focusedSection == SectionIntentions && m.intentionList != nil {
		intention := m.intentionList.SelectedIntention()
		if intention != nil {
			return m, m.toggleIntention(intention.ID)
		}
	}

	// Handle enter key for tasks (expand/collapse)
	if m.focusedSection == SectionTasks && m.taskList != nil {
		var cmd tea.Cmd
		m.taskList, cmd = m.taskList.Update(msg)
		return m, cmd
	}

	// Handle enter key for logs (expand/collapse)
	if m.focusedSection == SectionLogs && m.logView != nil {
		var cmd tea.Cmd
		m.logView, cmd = m.logView.Update(msg)
		return m, cmd
	}

	return m, nil
}

// handleKeyString handles string-based key input
func (m *Model) handleKeyString(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m.handleQuit()

	case "N":
		return m.handleToggleNotesPane()

	case "tab":
		return m.handleNextSection()

	case "shift+tab":
		return m.handlePrevSection()

	case "h", "left":
		return m, m.prevDay()

	case "l", "right":
		return m, m.nextDay()

	case "T":
		return m, m.jumpToToday()

	case "t":
		return m.handleAddTask()

	case "n":
		return m.handleAddNote(msg)

	case "?":
		return m.handleToggleHelp()

	case "s":
		// Context-sensitive: schedule task in Tasks section, toggle summary elsewhere
		if m.focusedSection == SectionTasks {
			return m.handleScheduleTask()
		}
		return m.handleToggleSummary()

	case "i":
		return m.handleAddIntention()

	case "w":
		return m.handleAddWin()

	case "L":
		return m.handleAddLog()

	case "d":
		return m.handleDelete(msg)

	case "e":
		return m.handleEdit(msg)

	case "D":
		return m.handleSetDeadline()

	case "c":
		return m.handleClearDate()

	default:
		return m.handleComponentKeys(msg)
	}
}

// handleToggleNotesPane toggles notes pane visibility
func (m *Model) handleToggleNotesPane() (tea.Model, tea.Cmd) {
	m.notesPaneVisible = !m.notesPaneVisible
	logger.Debug("tui: toggled notes pane visibility", "visible", m.notesPaneVisible)
	return m, nil
}

// handleNextSection cycles to next section
func (m *Model) handleNextSection() (tea.Model, tea.Cmd) {
	oldSection := m.focusedSection
	m.focusedSection = (m.focusedSection + 1) % SectionCount
	logger.Debug("tui: section changed",
		"oldSection", sectionName(oldSection),
		"newSection", sectionName(m.focusedSection))

	if m.statusBar != nil {
		m.statusBar.SetSection(sectionName(m.focusedSection))
	}

	return m, nil
}

// handlePrevSection cycles to previous section
func (m *Model) handlePrevSection() (tea.Model, tea.Cmd) {
	m.focusedSection = (m.focusedSection + SectionCount - 1) % SectionCount

	if m.statusBar != nil {
		m.statusBar.SetSection(sectionName(m.focusedSection))
	}

	return m, nil
}

// handleAddTask initiates task creation
func (m *Model) handleAddTask() (tea.Model, tea.Cmd) {
	if m.textEntryBar != nil {
		m.textEntryBar.SetMode(components.ModeTask)
		m.textEntryBar.Clear()

		if m.statusBar != nil {
			m.statusBar.SetInputMode(true)
		}

		return m, m.textEntryBar.Focus()
	}
	return m, nil
}

// handleAddNote initiates note creation for selected task or log
func (m *Model) handleAddNote(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Add note to selected task in Tasks section
	if m.focusedSection == SectionTasks && m.taskList != nil {
		selectedTask := m.taskList.SelectedTask()
		if selectedTask != nil {
			m.noteTaskID = selectedTask.ID
			m.textEntryBar.SetMode(components.ModeNote)
			m.textEntryBar.Clear()

			if m.statusBar != nil {
				m.statusBar.SetInputMode(true)
			}

			return m, m.textEntryBar.Focus()
		}
	}

	// Add note to selected log entry in Logs section
	if m.focusedSection == SectionLogs && m.logView != nil {
		// Delegate to log view which will return LogNoteAddMsg if appropriate
		var cmd tea.Cmd
		m.logView, cmd = m.logView.Update(msg)
		return m, cmd
	}

	return m, nil
}

// handleToggleHelp toggles help mode
func (m *Model) handleToggleHelp() (tea.Model, tea.Cmd) {
	m.helpMode = !m.helpMode
	logger.Debug("tui: toggled help mode", "helpMode", m.helpMode)
	return m, nil
}

// handleToggleSummary toggles summary view
func (m *Model) handleToggleSummary() (tea.Model, tea.Cmd) {
	if m.summaryView != nil {
		m.summaryView.Toggle()
	}
	return m, nil
}

// handleAddIntention initiates intention creation
func (m *Model) handleAddIntention() (tea.Model, tea.Cmd) {
	if m.textEntryBar != nil {
		m.textEntryBar.SetMode(components.ModeIntention)
		m.textEntryBar.Clear()

		if m.statusBar != nil {
			m.statusBar.SetInputMode(true)
		}

		return m, m.textEntryBar.Focus()
	}
	return m, nil
}

// handleAddWin initiates win creation
func (m *Model) handleAddWin() (tea.Model, tea.Cmd) {
	if m.textEntryBar != nil {
		m.textEntryBar.SetMode(components.ModeWin)
		m.textEntryBar.Clear()

		if m.statusBar != nil {
			m.statusBar.SetInputMode(true)
		}

		return m, m.textEntryBar.Focus()
	}
	return m, nil
}

// handleAddLog initiates log entry creation
func (m *Model) handleAddLog() (tea.Model, tea.Cmd) {
	if m.textEntryBar != nil {
		m.textEntryBar.SetMode(components.ModeLog)
		m.textEntryBar.Clear()

		if m.statusBar != nil {
			m.statusBar.SetInputMode(true)
		}

		return m, m.textEntryBar.Focus()
	}
	return m, nil
}

// handleDelete initiates deletion with confirmation
func (m *Model) handleDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.confirmMode {
		// Already in confirm mode, ignore
		return m, nil
	}

	switch m.focusedSection {
	case SectionIntentions:
		return m.handleDeleteIntention()

	case SectionWins:
		return m.handleDeleteWin()

	case SectionLogs:
		return m.handleDeleteLog(msg)

	case SectionTasks:
		return m.handleDeleteTask(msg)
	}

	return m, nil
}

// handleDeleteIntention initiates intention deletion
func (m *Model) handleDeleteIntention() (tea.Model, tea.Cmd) {
	if m.intentionList != nil {
		intention := m.intentionList.SelectedIntention()
		if intention != nil {
			logger.Debug("tui: entering confirm mode for intention deletion", "intentionID", intention.ID)
			m.confirmMode = true
			m.confirmItemType = "intention"
			m.confirmItemID = intention.ID
		}
	}
	return m, nil
}

// handleDeleteWin initiates win deletion
func (m *Model) handleDeleteWin() (tea.Model, tea.Cmd) {
	if m.winsView != nil {
		win := m.winsView.SelectedWin()
		if win != nil {
			logger.Debug("tui: entering confirm mode for win deletion", "winID", win.ID)
			m.confirmMode = true
			m.confirmItemType = "win"
			m.confirmItemID = win.ID
		}
	}
	return m, nil
}

// handleDeleteLog initiates log entry or note deletion
func (m *Model) handleDeleteLog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.logView != nil {
		// Check if a note is selected
		if m.logView.IsSelectedItemNote() {
			// Delegate to log view to get the LogNoteDeleteMsg
			var cmd tea.Cmd
			m.logView, cmd = m.logView.Update(msg)
			return m, cmd
		}

		// Otherwise, it's a log entry
		entry := m.logView.SelectedLogEntry()
		if entry != nil {
			logger.Debug("tui: entering confirm mode for log entry deletion", "logEntryID", entry.ID)
			m.confirmMode = true
			m.confirmItemType = "log"
			m.confirmItemID = entry.ID
		}
	}
	return m, nil
}

// handleDeleteTask initiates task or note deletion
func (m *Model) handleDeleteTask(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.taskList != nil {
		// Check if a note is selected
		if m.taskList.IsSelectedItemNote() {
			// Delegate to task list to get the TaskNoteDeleteMsg
			var cmd tea.Cmd
			m.taskList, cmd = m.taskList.Update(msg)
			return m, cmd
		}

		// Otherwise, it's a task
		task := m.taskList.SelectedTask()
		if task != nil {
			logger.Debug("tui: entering confirm mode for task deletion", "taskID", task.ID)
			m.confirmMode = true
			m.confirmItemType = "task"
			m.confirmItemID = task.ID
		}
	}
	return m, nil
}

// handleEdit initiates editing of selected item
func (m *Model) handleEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.confirmMode {
		// Already in confirm mode, ignore
		return m, nil
	}

	switch m.focusedSection {
	case SectionTasks:
		return m.handleEditTask()

	case SectionIntentions:
		return m.handleEditIntention()

	case SectionWins:
		return m.handleEditWin()

	case SectionLogs:
		return m.handleEditLog()
	}

	return m, nil
}

// handleEditTask initiates task editing
func (m *Model) handleEditTask() (tea.Model, tea.Cmd) {
	if m.taskList != nil {
		selectedTask := m.taskList.SelectedTask()
		if selectedTask != nil && !m.taskList.IsSelectedItemNote() {
			// Edit task (only main tasks, not notes)
			m.editItemID = selectedTask.ID
			m.editItemType = "task"
			m.editItemTags = selectedTask.Tags // Preserve tags
			m.textEntryBar.SetMode(components.ModeEditTask)
			m.textEntryBar.SetValue(selectedTask.Text)

			if m.statusBar != nil {
				m.statusBar.SetInputMode(true)
			}

			return m, m.textEntryBar.Focus()
		}
	}
	return m, nil
}

// handleEditIntention initiates intention editing
func (m *Model) handleEditIntention() (tea.Model, tea.Cmd) {
	if m.intentionList != nil {
		selectedIntention := m.intentionList.SelectedIntention()
		if selectedIntention != nil {
			m.editItemID = selectedIntention.ID
			m.editItemType = "intention"
			m.textEntryBar.SetMode(components.ModeEditIntention)
			m.textEntryBar.SetValue(selectedIntention.Text)

			if m.statusBar != nil {
				m.statusBar.SetInputMode(true)
			}

			return m, m.textEntryBar.Focus()
		}
	}
	return m, nil
}

// handleEditWin initiates win editing
func (m *Model) handleEditWin() (tea.Model, tea.Cmd) {
	if m.winsView != nil {
		selectedWin := m.winsView.SelectedWin()
		if selectedWin != nil {
			m.editItemID = selectedWin.ID
			m.editItemType = "win"
			m.textEntryBar.SetMode(components.ModeEditWin)
			m.textEntryBar.SetValue(selectedWin.Text)

			if m.statusBar != nil {
				m.statusBar.SetInputMode(true)
			}

			return m, m.textEntryBar.Focus()
		}
	}
	return m, nil
}

// handleEditLog initiates log entry editing
func (m *Model) handleEditLog() (tea.Model, tea.Cmd) {
	if m.logView != nil {
		selectedLogEntry := m.logView.SelectedLogEntry()
		if selectedLogEntry != nil && !m.logView.IsSelectedItemNote() {
			// Edit log entry (only main entries, not notes)
			m.editItemID = selectedLogEntry.ID
			m.editItemType = "log"
			m.textEntryBar.SetMode(components.ModeEditLog)
			m.textEntryBar.SetValue(selectedLogEntry.Content)

			if m.statusBar != nil {
				m.statusBar.SetInputMode(true)
			}

			return m, m.textEntryBar.Focus()
		}
	}
	return m, nil
}

// handleComponentKeys delegates key handling to focused component
func (m *Model) handleComponentKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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

	return m, nil
}

// handleScheduleTask initiates task scheduling
func (m *Model) handleScheduleTask() (tea.Model, tea.Cmd) {
	// Only handle when Tasks section is focused
	if m.focusedSection != SectionTasks || m.taskList == nil {
		return m, nil
	}

	// Get selected task
	selectedTask := m.taskList.SelectedTask()
	if selectedTask == nil || m.taskList.IsSelectedItemNote() {
		return m, nil
	}

	// Show date picker in schedule mode
	m.datePickerMode = "schedule"
	m.datePickerTargetTask = selectedTask.ID
	m.datePickerVisible = true
	if m.datePicker != nil {
		m.datePicker = components.NewDatePicker("Schedule Task")
		return m, m.datePicker.Show()
	}

	return m, nil
}

// handleSetDeadline initiates deadline setting
func (m *Model) handleSetDeadline() (tea.Model, tea.Cmd) {
	// Only handle when Tasks section is focused
	if m.focusedSection != SectionTasks || m.taskList == nil {
		return m, nil
	}

	// Get selected task
	selectedTask := m.taskList.SelectedTask()
	if selectedTask == nil || m.taskList.IsSelectedItemNote() {
		return m, nil
	}

	// Show date picker in deadline mode
	m.datePickerMode = "deadline"
	m.datePickerTargetTask = selectedTask.ID
	m.datePickerVisible = true
	if m.datePicker != nil {
		m.datePicker = components.NewDatePicker("Set Deadline")
		return m, m.datePicker.Show()
	}

	return m, nil
}

// handleClearDate initiates clear date submenu
func (m *Model) handleClearDate() (tea.Model, tea.Cmd) {
	// Only handle when Tasks section is focused
	if m.focusedSection != SectionTasks || m.taskList == nil {
		return m, nil
	}

	// Get selected task
	selectedTask := m.taskList.SelectedTask()
	if selectedTask == nil || m.taskList.IsSelectedItemNote() {
		return m, nil
	}

	// Show clear date submenu
	m.clearDateMode = true
	m.clearDateTargetTask = selectedTask.ID
	return m, nil
}

// handleDatePickerKeys handles keyboard input when date picker is visible
func (m *Model) handleDatePickerKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		// Cancel date picker
		m.datePickerVisible = false
		m.datePickerMode = ""
		m.datePickerTargetTask = ""
		if m.datePicker != nil {
			m.datePicker.Hide()
		}
		return m, nil

	case tea.KeyEnter:
		// Submit date picker
		if m.datePicker == nil {
			return m, nil
		}

		dateValue := m.datePicker.GetValue()
		if dateValue == "" {
			return m, nil
		}

		// Parse the date
		parsedDate, err := components.ParseRelativeDate(dateValue)
		if err != nil {
			return m, nil
		}

		// Format as YYYY-MM-DD
		dateStr := parsedDate.Format("2006-01-02")

		// Hide date picker
		m.datePickerVisible = false
		m.datePicker.Hide()

		// Execute the appropriate command based on mode
		capturedMode := m.datePickerMode
		capturedTaskID := m.datePickerTargetTask
		capturedTaskService := m.taskService
		capturedDate := dateStr

		// Reset state
		m.datePickerMode = ""
		m.datePickerTargetTask = ""

		return m, func() tea.Msg {
			if capturedTaskService == nil {
				return errMsg{err: fmt.Errorf("task service not available")}
			}

			var err error
			if capturedMode == "schedule" {
				err = capturedTaskService.ScheduleTask(capturedTaskID, capturedDate)
				if err != nil {
					return errMsg{err: err}
				}
				return taskScheduledMsg{date: capturedDate}
			} else if capturedMode == "deadline" {
				err = capturedTaskService.SetTaskDeadline(capturedTaskID, capturedDate)
				if err != nil {
					return errMsg{err: err}
				}
				return taskDeadlineSetMsg{date: capturedDate}
			}

			return errMsg{err: fmt.Errorf("unknown date picker mode")}
		}
	}

	// Delegate to date picker component
	var cmd tea.Cmd
	if m.datePicker != nil {
		m.datePicker, cmd = m.datePicker.Update(msg)
	}
	return m, cmd
}

// handleClearDateKeys handles keyboard input in clear date submenu
func (m *Model) handleClearDateKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "s", "S":
		// Clear schedule
		capturedTaskID := m.clearDateTargetTask
		capturedTaskService := m.taskService

		m.clearDateMode = false
		m.clearDateTargetTask = ""

		return m, func() tea.Msg {
			if capturedTaskService == nil {
				return errMsg{err: fmt.Errorf("task service not available")}
			}

			err := capturedTaskService.ClearTaskSchedule(capturedTaskID)
			if err != nil {
				return errMsg{err: err}
			}
			return taskDateClearedMsg{clearedType: "schedule"}
		}

	case "d", "D":
		// Clear deadline
		capturedTaskID := m.clearDateTargetTask
		capturedTaskService := m.taskService

		m.clearDateMode = false
		m.clearDateTargetTask = ""

		return m, func() tea.Msg {
			if capturedTaskService == nil {
				return errMsg{err: fmt.Errorf("task service not available")}
			}

			err := capturedTaskService.ClearTaskDeadline(capturedTaskID)
			if err != nil {
				return errMsg{err: err}
			}
			return taskDateClearedMsg{clearedType: "deadline"}
		}

	case "b", "B":
		// Clear both
		capturedTaskID := m.clearDateTargetTask
		capturedTaskService := m.taskService

		m.clearDateMode = false
		m.clearDateTargetTask = ""

		return m, func() tea.Msg {
			if capturedTaskService == nil {
				return errMsg{err: fmt.Errorf("task service not available")}
			}

			err := capturedTaskService.ClearTaskSchedule(capturedTaskID)
			if err != nil {
				return errMsg{err: err}
			}
			err = capturedTaskService.ClearTaskDeadline(capturedTaskID)
			if err != nil {
				return errMsg{err: err}
			}
			return taskDateClearedMsg{clearedType: "both"}
		}

	case "esc":
		// Cancel
		m.clearDateMode = false
		m.clearDateTargetTask = ""
		return m, nil
	}

	return m, nil
}
