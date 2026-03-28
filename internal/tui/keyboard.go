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
	if m.focusedSection == SectionTasks {
		task := m.selectedTask()
		if task == nil {
			return m, nil
		}
		capturedID := task.ID
		return m, func() tea.Msg {
			return components.TaskToggleMsg{TaskID: capturedID}
		}
	}
	return m, nil
}

// handleEnterKey handles enter key in different contexts
func (m *Model) handleEnterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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

	default:
		return m.handleComponentKeys(msg)
	}
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
		m.statusBar.SetNoteSelected(false)
	}

	return m, nil
}

// handlePrevSection cycles to previous section
func (m *Model) handlePrevSection() (tea.Model, tea.Cmd) {
	m.focusedSection = (m.focusedSection + SectionCount - 1) % SectionCount

	if m.statusBar != nil {
		m.statusBar.SetSection(sectionName(m.focusedSection))
		m.statusBar.SetNoteSelected(false)
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
	if m.focusedSection == SectionTasks {
		task := m.selectedTask()
		if task != nil {
			m.noteTaskID = task.ID
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
	case SectionLogs:
		return m.handleDeleteLog(msg)
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

// handleEdit initiates editing of selected item
func (m *Model) handleEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.confirmMode {
		// Already in confirm mode, ignore
		return m, nil
	}

	switch m.focusedSection {
	case SectionTasks:
		return m.handleEditTask()

	case SectionLogs:
		return m.handleEditLog()
	}

	return m, nil
}

// handleEditTask initiates task editing
func (m *Model) handleEditTask() (tea.Model, tea.Cmd) {
	selectedTask := m.selectedTask()
	if selectedTask != nil {
		m.editItemID = selectedTask.ID
		m.editItemType = "task"
		m.editItemTags = selectedTask.Tags
		m.textEntryBar.SetMode(components.ModeEditTask)
		m.textEntryBar.SetValue(selectedTask.Text)
		if m.statusBar != nil {
			m.statusBar.SetInputMode(true)
		}
		return m, m.textEntryBar.Focus()
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
	case SectionLogs:
		if m.logView != nil {
			var cmd tea.Cmd
			m.logView, cmd = m.logView.Update(msg)
			return m, cmd
		}

	case SectionTasks:
		switch msg.String() {
		case "j", "down":
			m.selectedIndex = clampIndex(m.selectedIndex+1, len(m.tasks))
		case "k", "up":
			if m.selectedIndex > 0 {
				m.selectedIndex--
			}
		}
		return m, nil

	}

	return m, nil
}

// handleScheduleTask initiates task scheduling
func (m *Model) handleScheduleTask() (tea.Model, tea.Cmd) {
	if m.focusedSection != SectionTasks {
		return m, nil
	}

	selectedTask := m.selectedTask()
	if selectedTask == nil {
		m.successMessage = "No task selected"
		return m, func() tea.Msg { return clearSuccessMsg{} }
	}

	m.datePickerMode = "schedule"
	m.datePickerTargetTask = selectedTask.ID
	m.datePickerVisible = true
	m.datePicker = components.NewDatePicker("Schedule Task")
	return m, m.datePicker.Show()
}

// handleSetDeadline initiates deadline setting
func (m *Model) handleSetDeadline() (tea.Model, tea.Cmd) {
	if m.focusedSection != SectionTasks {
		return m, nil
	}

	selectedTask := m.selectedTask()
	if selectedTask == nil {
		m.successMessage = "No task selected"
		return m, func() tea.Msg { return clearSuccessMsg{} }
	}

	m.datePickerMode = "deadline"
	m.datePickerTargetTask = selectedTask.ID
	m.datePickerVisible = true
	m.datePicker = components.NewDatePicker("Set Deadline")
	return m, m.datePicker.Show()
}

// closeDatePicker cleans up date picker state
func (m *Model) closeDatePicker() {
	m.datePickerVisible = false
	m.datePickerMode = ""
	m.datePickerTargetTask = ""
	if m.datePicker != nil {
		m.datePicker.Hide()
	}
}

// handleDatePickerKeys handles keyboard input when date picker is visible
func (m *Model) handleDatePickerKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		// Cancel date picker
		m.closeDatePicker()
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

		// Execute the appropriate command based on mode
		capturedMode := m.datePickerMode
		capturedTaskID := m.datePickerTargetTask
		capturedTaskService := m.taskService
		capturedDate := dateStr

		// Reset state
		m.closeDatePicker()

		return m, func() tea.Msg {
			if capturedTaskService == nil {
				return errMsg{err: fmt.Errorf("task service not available for setting date")}
			}

			var err error
			if capturedMode == "schedule" {
				err = capturedTaskService.ScheduleTask(capturedTaskID, capturedDate)
				if err != nil {
					return errMsg{err: fmt.Errorf("failed to schedule task: %w", err)}
				}
				return taskScheduledMsg{date: capturedDate}
			} else if capturedMode == "deadline" {
				err = capturedTaskService.SetTaskDeadline(capturedTaskID, capturedDate)
				if err != nil {
					return errMsg{err: fmt.Errorf("failed to set deadline: %w", err)}
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
