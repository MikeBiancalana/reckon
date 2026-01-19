package tui

import (
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
}

// handleJournalLoaded handles the journal loaded message
func (m *Model) handleJournalLoaded(msg journalLoadedMsg) (tea.Model, tea.Cmd) {
	logger.Debug("tui: handling journalLoadedMsg",
		"date", msg.journal.Date,
		"intentions", len(msg.journal.Intentions),
		"wins", len(msg.journal.Wins),
		"logs", len(msg.journal.LogEntries))

	m.currentJournal = &msg.journal

	// Update or create IntentionList
	if m.intentionList == nil {
		m.intentionList = components.NewIntentionList(msg.journal.Intentions)
	} else {
		m.intentionList.UpdateIntentions(msg.journal.Intentions)
	}

	// Update or create WinsView
	if m.winsView == nil {
		m.winsView = components.NewWinsView(msg.journal.Wins)
	} else {
		m.winsView.UpdateWins(msg.journal.Wins)
	}

	// Update or create LogView
	if m.logView == nil {
		m.logView = components.NewLogView(msg.journal.LogEntries)
	} else {
		m.logView.UpdateLogEntries(msg.journal.LogEntries)
	}

	// Update or create ScheduleView
	if m.scheduleView == nil {
		m.scheduleView = components.NewScheduleView(msg.journal.ScheduleItems)
	} else {
		m.scheduleView.UpdateSchedule(msg.journal.ScheduleItems)
	}

	// Initialize taskList. Tasks are loaded via the separate tasksLoadedMsg handler.
	if m.taskList == nil {
		m.taskList = components.NewTaskList(nil)
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
	// Reload journal after update
	return m, m.loadJournal()
}

// handleTasksLoaded handles tasks loaded message
func (m *Model) handleTasksLoaded(msg tasksLoadedMsg) (tea.Model, tea.Cmd) {
	logger.Debug("tui: handling tasksLoadedMsg", "taskCount", len(msg.tasks))

	// Update or create task list
	if m.taskList != nil {
		m.taskList.UpdateTasks(msg.tasks)
	} else {
		m.taskList = components.NewTaskList(msg.tasks)
	}

	m.updateNotesForSelectedTask()
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

// handleTaskSelectionChanged handles task selection change events
func (m *Model) handleTaskSelectionChanged(msg components.TaskSelectionChangedMsg) (tea.Model, tea.Cmd) {
	m.updateNotesForSelectedTask()
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
	// Log note deleted successfully, reload journal
	return m, m.loadJournal()
}

// handleTaskNoteDeleted handles task note deleted confirmation
func (m *Model) handleTaskNoteDeleted(msg taskNoteDeletedMsg) (tea.Model, tea.Cmd) {
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
