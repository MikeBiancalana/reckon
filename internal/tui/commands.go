package tui

import (
	"fmt"
	"strings"
	stdtime "time"

	"github.com/MikeBiancalana/reckon/internal/logger"
	"github.com/MikeBiancalana/reckon/internal/perf"
	"github.com/MikeBiancalana/reckon/internal/tui/components"
	tea "github.com/charmbracelet/bubbletea"
)

// Command Builders
//
// These methods create tea.Cmd functions for async operations.
// They follow the async closure capture pattern to avoid bugs
// where model state changes between closure creation and execution.
//
// Key principle: Capture all needed values BEFORE returning the closure.

// prevDay navigates to the previous day
func (m *Model) prevDay() tea.Cmd {
	date, err := stdtime.Parse("2006-01-02", m.currentDate)
	if err != nil {
		// If current date is corrupted, fall back to today
		m.currentDate = stdtime.Now().Format("2006-01-02")
		return m.loadJournal()
	}

	oldDate := m.currentDate
	newDate := date.AddDate(0, 0, -1).Format("2006-01-02")
	m.currentDate = newDate

	logger.Debug("tui: navigating to previous day", "oldDate", oldDate, "newDate", newDate)
	return m.loadJournal()
}

// nextDay navigates to the next day
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

	oldDate := m.currentDate
	m.currentDate = newDate

	logger.Debug("tui: navigating to next day", "oldDate", oldDate, "newDate", newDate)
	return m.loadJournal()
}

// jumpToToday navigates to today's journal
func (m *Model) jumpToToday() tea.Cmd {
	m.currentDate = stdtime.Now().Format("2006-01-02")
	return m.loadJournal()
}

// loadJournal loads the journal for the current date
// Note: m.currentDate is accessed safely because its value is captured at
// the time of closure creation. For strings and other immutable types,
// this pattern is safe because the closure executes before any state change.
func (m *Model) loadJournal() tea.Cmd {
	return func() tea.Msg {
		timer := perf.NewTimer("tui.loadJournal", nil, 100)
		defer timer.Stop()

		logger.Debug("tui: loading journal", "date", m.currentDate)
		j, err := m.service.GetByDate(m.currentDate)
		if err != nil {
			logger.Debug("tui: failed to load journal", "date", m.currentDate, "error", err)
			return errMsg{err}
		}
		logger.Debug("tui: journal loaded successfully", "date", m.currentDate)
		return journalLoadedMsg{*j}
	}
}

// loadTasks loads all tasks from the task service
func (m *Model) loadTasks() tea.Cmd {
	return func() tea.Msg {
		timer := perf.NewTimer("tui.loadTasks", nil, 100)
		defer timer.Stop()

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

// toggleTask toggles a task's completion status
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

// toggleIntention toggles an intention's completion status
func (m *Model) toggleIntention(intentionID string) tea.Cmd {
	capturedJournal := m.currentJournal
	return func() tea.Msg {
		logger.Debug("tui: toggling intention", "intentionID", intentionID)
		err := m.service.ToggleIntention(capturedJournal, intentionID)
		if err != nil {
			logger.Debug("tui: failed to toggle intention", "intentionID", intentionID, "error", err)
			return errMsg{err}
		}
		logger.Debug("tui: intention toggled successfully", "intentionID", intentionID)
		return journalUpdatedMsg{}
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

		case "task":
			if m.taskService != nil {
				err = m.taskService.DeleteTask(m.confirmItemID)
			} else {
				err = fmt.Errorf("task service not available")
			}

		case "log_note":
			// Delete log note using both log entry ID and note ID
			err = m.service.DeleteLogNote(m.currentJournal, m.confirmLogEntryID, m.confirmItemID)
			if err != nil {
				// Reset confirmation state
				m.confirmMode = false
				m.confirmItemType = ""
				m.confirmItemID = ""
				m.confirmLogEntryID = ""
				return errMsg{err}
			}
			// Reset confirmation state
			m.confirmMode = false
			m.confirmItemType = ""
			m.confirmItemID = ""
			m.confirmLogEntryID = ""
			return logNoteDeletedMsg{}

		case "task_note":
			// Delete task note using both task ID and note ID
			if m.taskService != nil {
				err = m.taskService.DeleteTaskNote(m.confirmLogEntryID, m.confirmItemID)
				if err != nil {
					// Reset confirmation state
					m.confirmMode = false
					m.confirmItemType = ""
					m.confirmItemID = ""
					m.confirmLogEntryID = ""
					return errMsg{err}
				}
				// Reset confirmation state
				m.confirmMode = false
				m.confirmItemType = ""
				m.confirmItemID = ""
				m.confirmLogEntryID = ""
				return taskNoteDeletedMsg{}
			}
			return errMsg{fmt.Errorf("task service not available")}
		}

		// Reset confirmation state
		m.confirmMode = false
		m.confirmItemType = ""
		m.confirmItemID = ""
		m.confirmLogEntryID = ""

		if err != nil {
			return errMsg{err}
		}
		return journalUpdatedMsg{}
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

	// CRITICAL: Capture ALL values we need BEFORE creating the async function.
	// Go closures capture variables by REFERENCE, not by value.
	// If we don't capture them here, the values may be reset by the key handler
	// before the async function runs.
	//
	// Example of the bug this prevents:
	//   // WRONG - captures m.noteLogEntryID by reference
	//   return func() tea.Msg {
	//       err := m.service.AddLogNote(m.currentJournal, m.noteLogEntryID, inputText)
	//       // If key handler runs before this, m.noteLogEntryID could be reset!
	//   }
	//
	//   // CORRECT - captures value at this point in time
	//   capturedLogEntryID := m.noteLogEntryID
	//   return func() tea.Msg {
	//       err := m.service.AddLogNote(m.currentJournal, capturedLogEntryID, inputText)
	//   }
	capturedLogEntryID := m.noteLogEntryID
	capturedTaskID := m.noteTaskID
	capturedEditID := m.editItemID
	capturedEditType := m.editItemType
	capturedCurrentJournal := m.currentJournal
	capturedMode := mode
	capturedTaskService := m.taskService

	return func() tea.Msg {
		var err error

		switch capturedMode {
		case components.ModeTask:
			// Add task
			if capturedTaskService != nil {
				taskText, tags := parseTaskTags(inputText)
				err = capturedTaskService.AddTask(taskText, tags)
				if err != nil {
					return errMsg{err}
				}
				// Reload tasks
				tasks, errGetTasks := capturedTaskService.GetAllTasks()
				if errGetTasks != nil {
					return errMsg{errGetTasks}
				}
				return tasksLoadedMsg{tasks: tasks}
			}
			return errMsg{fmt.Errorf("task service not available")}

		case components.ModeIntention:
			err = m.service.AddIntention(capturedCurrentJournal, inputText)

		case components.ModeWin:
			err = m.service.AddWin(capturedCurrentJournal, inputText)

		case components.ModeLog:
			err = m.service.AppendLog(capturedCurrentJournal, inputText)

		case components.ModeNote:
			// Add note to the selected task
			if capturedTaskService != nil && capturedTaskID != "" {
				err = capturedTaskService.AddTaskNote(capturedTaskID, inputText)
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
			return errMsg{fmt.Errorf("task service not available or no task selected")}

		case components.ModeLogNote:
			// Add note to the selected log entry
			if capturedLogEntryID != "" {
				err = m.service.AddLogNote(capturedCurrentJournal, capturedLogEntryID, inputText)
				if err != nil {
					return errMsg{err}
				}
				return logNoteAddedMsg{}
			}
			return errMsg{fmt.Errorf("no log entry selected")}

		case components.ModeEditTask:
			// Edit task
			if capturedTaskService != nil && capturedEditID != "" && capturedEditType == "task" {
				err = capturedTaskService.UpdateTask(capturedEditID, inputText, []string{}) // TODO: preserve existing tags
				if err != nil {
					return errMsg{err}
				}
				// Reload tasks
				tasks, errGetTasks := capturedTaskService.GetAllTasks()
				if errGetTasks != nil {
					return errMsg{errGetTasks}
				}
				return tasksLoadedMsg{tasks: tasks}
			}
			return errMsg{fmt.Errorf("task service not available or no task selected for editing")}

		case components.ModeEditIntention:
			// Edit intention
			if capturedEditID != "" && capturedEditType == "intention" {
				err = m.service.UpdateIntention(capturedCurrentJournal, capturedEditID, inputText)
			}

		case components.ModeEditWin:
			// Edit win
			if capturedEditID != "" && capturedEditType == "win" {
				err = m.service.UpdateWin(capturedCurrentJournal, capturedEditID, inputText)
			}

		case components.ModeEditLog:
			// Edit log entry
			if capturedEditID != "" && capturedEditType == "log" {
				err = m.service.UpdateLogEntry(capturedCurrentJournal, capturedEditID, inputText)
			}

		default:
			return errMsg{fmt.Errorf("unknown entry mode")}
		}

		if err != nil {
			return errMsg{err}
		}
		return journalUpdatedMsg{}
	}
}

// waitForFileChange waits for file change events from the watcher.
// This is a non-blocking async command - it returns immediately and the
// closure waits for the watcher channel to signal changes.
func (m *Model) waitForFileChange() tea.Cmd {
	if m.watcher == nil {
		return nil
	}

	return func() tea.Msg {
		event := <-m.watcher.Changes()
		return fileChangedMsg{date: event.Date}
	}
}

// parseTaskTags extracts tags (words starting with #) from input text
func parseTaskTags(input string) (string, []string) {
	var tags []string
	words := strings.Fields(input)
	var filteredWords []string

	for _, word := range words {
		if strings.HasPrefix(word, "#") && len(word) > 1 {
			tag := strings.TrimLeft(strings.ToLower(word), "#")
			if tag != "" {
				tags = append(tags, tag)
			}
		} else {
			filteredWords = append(filteredWords, word)
		}
	}

	return strings.Join(filteredWords, " "), tags
}
