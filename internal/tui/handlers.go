package tui

import (
	stdtime "time"

	"github.com/MikeBiancalana/reckon/internal/logger"
	"github.com/MikeBiancalana/reckon/internal/time"
	"github.com/MikeBiancalana/reckon/internal/tui/components"
	tea "github.com/charmbracelet/bubbletea"
)

// Message Handlers
//
// These methods handle specific message types, keeping the main Update()
// function clean and focused. Each handler follows the pattern:
//
//   func (m *Model) handle<MessageType>(msg <MessageType>) (tea.Model, tea.Cmd)
//
// This makes handlers testable in isolation and easy to understand.

// handleWindowSize handles terminal resize events
func (m *Model) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height

	// Check if terminal meets minimum dimensions
	m.terminalTooSmall = msg.Width < MinTerminalWidth || msg.Height < MinTerminalHeight

	if m.statusBar != nil {
		m.statusBar.SetWidth(msg.Width)
	}

	// logView sizing is handled in renderNewLayout via CalculatePaneDimensions

	return m, nil
}

// handleJournalLoaded handles the journal loaded message
func (m *Model) handleJournalLoaded(msg journalLoadedMsg) (tea.Model, tea.Cmd) {
	logger.Debug("tui: handling journalLoadedMsg",
		"date", msg.journal.Date,
		"intentions", len(msg.journal.Intentions),
		"wins", len(msg.journal.Wins),
		"logs", len(msg.journal.LogEntries))

	m.currentJournal = &msg.journal

	// Update or create LogView
	if m.logView == nil {
		m.logView = components.NewLogView(msg.journal.LogEntries)
	} else {
		m.logView.UpdateLogEntries(msg.journal.LogEntries)
	}

	// Calculate time summary
	daySummary := time.CalculateDaySummary(&msg.journal)
	if m.summaryView != nil {
		m.summaryView.SetSummary(&daySummary)
	}

	return m, nil
}

// handleJournalUpdated handles journal update notifications
func (m *Model) handleJournalUpdated(msg journalUpdatedMsg) (tea.Model, tea.Cmd) {
	// Reset confirmation state
	m.confirmMode = false
	m.confirmItemType = ""
	m.confirmItemID = ""
	m.confirmLogEntryID = ""

	// Reload journal after update
	return m, m.loadJournal()
}

// handleTasksLoaded handles tasks loaded message
func (m *Model) handleTasksLoaded(msg tasksLoadedMsg) (tea.Model, tea.Cmd) {
	logger.Debug("tui: handling tasksLoadedMsg", "taskCount", len(msg.tasks))

	m.tasks = SortTasksByPriority(msg.tasks, stdtime.Now())
	m.selectedIndex = clampIndex(m.selectedIndex, len(m.tasks))

	// Clear success message after 2 seconds
	if m.successMessage != "" {
		return m, tea.Tick(2*stdtime.Second, func(t stdtime.Time) tea.Msg {
			return clearSuccessMsg{}
		})
	}

	return m, nil
}

// handleTaskToggle handles task toggle events from component
func (m *Model) handleTaskToggle(msg components.TaskToggleMsg) (tea.Model, tea.Cmd) {
	logger.Debug("tui: handling TaskToggleMsg", "taskID", msg.TaskID)

	if m.taskService != nil {
		return m, m.toggleTask(msg.TaskID)
	}

	return m, nil
}

// handleTaskToggled handles task toggled confirmation
func (m *Model) handleTaskToggled(msg taskToggledMsg) (tea.Model, tea.Cmd) {
	// Task toggled successfully, reload tasks
	return m, m.loadTasks()
}

// handleTaskAdded handles task added confirmation
func (m *Model) handleTaskAdded(msg taskAddedMsg) (tea.Model, tea.Cmd) {
	// Task added successfully, reload tasks
	return m, m.loadTasks()
}

// handleNoteAdded handles note added confirmation
func (m *Model) handleNoteAdded(msg noteAddedMsg) (tea.Model, tea.Cmd) {
	// Note added successfully, reload tasks
	return m, m.loadTasks()
}

// handleLogNoteAdd handles log note add initiation
func (m *Model) handleLogNoteAdd(msg components.LogNoteAddMsg) (tea.Model, tea.Cmd) {
	// Start adding a log note
	m.noteLogEntryID = msg.LogEntryID
	m.textEntryBar.SetMode(components.ModeLogNote)
	m.textEntryBar.Clear()

	if m.statusBar != nil {
		m.statusBar.SetInputMode(true)
	}

	return m, m.textEntryBar.Focus()
}

// handleLogNoteDelete handles log note delete confirmation request
func (m *Model) handleLogNoteDelete(msg components.LogNoteDeleteMsg) (tea.Model, tea.Cmd) {
	logger.Debug("tui: entering confirm mode for log note deletion",
		"logEntryID", msg.LogEntryID,
		"noteID", msg.NoteID)

	m.confirmMode = true
	m.confirmItemType = "log_note"
	m.confirmItemID = msg.NoteID
	m.confirmLogEntryID = msg.LogEntryID

	return m, nil
}

// handleTaskNoteDelete handles task note delete confirmation request
func (m *Model) handleTaskNoteDelete(msg components.TaskNoteDeleteMsg) (tea.Model, tea.Cmd) {
	logger.Debug("tui: entering confirm mode for task note deletion",
		"taskID", msg.TaskID,
		"noteID", msg.NoteID)

	m.confirmMode = true
	m.confirmItemType = "task_note"
	m.confirmItemID = msg.NoteID
	m.confirmLogEntryID = msg.TaskID // Reuse this field for task ID

	return m, nil
}

// handleLogNoteAdded handles log note added confirmation
func (m *Model) handleLogNoteAdded(msg logNoteAddedMsg) (tea.Model, tea.Cmd) {
	// Log note added successfully, reload journal
	return m, m.loadJournal()
}

// handleLogNoteDeleted handles log note deleted confirmation
func (m *Model) handleLogNoteDeleted(msg logNoteDeletedMsg) (tea.Model, tea.Cmd) {
	// Reset confirmation state
	m.confirmMode = false
	m.confirmItemType = ""
	m.confirmItemID = ""
	m.confirmLogEntryID = ""

	// Log note deleted successfully, reload journal
	return m, m.loadJournal()
}

// handleTaskNoteDeleted handles task note deleted confirmation
func (m *Model) handleTaskNoteDeleted(msg taskNoteDeletedMsg) (tea.Model, tea.Cmd) {
	// Reset confirmation state
	m.confirmMode = false
	m.confirmItemType = ""
	m.confirmItemID = ""
	m.confirmLogEntryID = ""

	// Task note deleted successfully, reload tasks
	return m, m.loadTasks()
}

// handleFileChanged handles file change notifications from watcher
func (m *Model) handleFileChanged(msg fileChangedMsg) (tea.Model, tea.Cmd) {
	// Reload journal if the changed file is for the current date
	var cmd tea.Cmd
	if msg.date == m.currentDate {
		cmd = tea.Batch(m.loadJournal(), m.waitForFileChange())
	} else {
		cmd = m.waitForFileChange()
	}
	return m, cmd
}

// handleError handles error messages
func (m *Model) handleError(msg errMsg) (tea.Model, tea.Cmd) {
	// Store error for display
	m.lastError = msg.err
	return m, nil
}

// handleTaskScheduled handles task scheduled confirmation
func (m *Model) handleTaskScheduled(msg taskScheduledMsg) (tea.Model, tea.Cmd) {
	// Show success message
	parsedDate, _ := components.ParseRelativeDate(msg.date)
	friendlyDate := parsedDate.Format("Jan 2")
	m.successMessage = "✓ Task scheduled for " + friendlyDate
	// Reload tasks to reflect the change
	return m, m.loadTasks()
}

// handleTaskDeadlineSet handles task deadline set confirmation
func (m *Model) handleTaskDeadlineSet(msg taskDeadlineSetMsg) (tea.Model, tea.Cmd) {
	// Show success message
	parsedDate, _ := components.ParseRelativeDate(msg.date)
	friendlyDate := parsedDate.Format("Jan 2")
	m.successMessage = "✓ Deadline set to " + friendlyDate
	// Reload tasks to reflect the change
	return m, m.loadTasks()
}

// handleTaskDateCleared handles task date cleared confirmation
func (m *Model) handleTaskDateCleared(msg taskDateClearedMsg) (tea.Model, tea.Cmd) {
	// Show success message
	switch msg.clearedType {
	case "schedule":
		m.successMessage = "✓ Schedule cleared"
	case "deadline":
		m.successMessage = "✓ Deadline cleared"
	case "both":
		m.successMessage = "✓ Schedule and deadline cleared"
	}
	// Reload tasks to reflect the change
	return m, m.loadTasks()
}
