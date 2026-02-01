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
func (m *Model) loadJournal() tea.Cmd {
	// Capture values before creating closure
	capturedDate := m.currentDate
	capturedService := m.service

	return func() tea.Msg {
		timer := perf.NewTimer("tui.loadJournal", nil, 100)
		defer timer.Stop()

		logger.Debug("tui: loading journal", "date", capturedDate)
		j, err := capturedService.GetByDate(capturedDate)
		if err != nil {
			logger.Debug("tui: failed to load journal", "date", capturedDate, "error", err)
			return errMsg{err}
		}
		logger.Debug("tui: journal loaded successfully", "date", capturedDate)
		return journalLoadedMsg{*j}
	}
}

// loadTasks loads all tasks from the task service
func (m *Model) loadTasks() tea.Cmd {
	// Capture values before creating closure
	capturedTaskService := m.taskService

	return func() tea.Msg {
		timer := perf.NewTimer("tui.loadTasks", nil, 100)
		defer timer.Stop()

		if capturedTaskService == nil {
			return errMsg{fmt.Errorf("task service not available")}
		}

		tasks, err := capturedTaskService.GetAllTasks()
		if err != nil {
			return errMsg{err}
		}

		return tasksLoadedMsg{tasks: tasks}
	}
}

// toggleTask toggles a task's completion status
func (m *Model) toggleTask(taskID string) tea.Cmd {
	// Capture values before creating closure
	capturedTaskService := m.taskService

	return func() tea.Msg {
		if capturedTaskService == nil {
			return errMsg{fmt.Errorf("task service not available")}
		}

		err := capturedTaskService.ToggleTask(taskID)
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
	// Capture all values before creating closure
	capturedService := m.service
	capturedTaskService := m.taskService
	capturedJournal := m.currentJournal
	capturedItemType := m.confirmItemType
	capturedItemID := m.confirmItemID
	capturedLogEntryID := m.confirmLogEntryID

	return func() tea.Msg {
		var err error
		switch capturedItemType {
		case "intention":
			err = capturedService.DeleteIntention(capturedJournal, capturedItemID)

		case "win":
			err = capturedService.DeleteWin(capturedJournal, capturedItemID)

		case "log":
			err = capturedService.DeleteLogEntry(capturedJournal, capturedItemID)

		case "task":
			if capturedTaskService != nil {
				err = capturedTaskService.DeleteTask(capturedItemID)
			} else {
				err = fmt.Errorf("task service not available")
			}

		case "log_note":
			// Delete log note using both log entry ID and note ID
			err = capturedService.DeleteLogNote(capturedJournal, capturedLogEntryID, capturedItemID)
			if err != nil {
				return errMsg{err}
			}
			return logNoteDeletedMsg{}

		case "task_note":
			// Delete task note using both task ID and note ID
			if capturedTaskService != nil {
				err = capturedTaskService.DeleteTaskNote(capturedLogEntryID, capturedItemID)
				if err != nil {
					return errMsg{err}
				}
				return taskNoteDeletedMsg{}
			}
			return errMsg{fmt.Errorf("task service not available")}
		}

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
	capturedEditTags := m.editItemTags
	capturedCurrentJournal := m.currentJournal
	capturedMode := mode
	capturedTaskService := m.taskService
	capturedService := m.service

	return func() tea.Msg {
		var err error

		switch capturedMode {
		case components.ModeTask:
			// Add task
			if capturedTaskService != nil {
				taskText, tags := parseTaskTags(inputText)
				_, err = capturedTaskService.AddTask(taskText, tags)
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
			err = capturedService.AddIntention(capturedCurrentJournal, inputText)
			if err != nil {
				return errMsg{err}
			}
			return journalUpdatedMsg{}

		case components.ModeWin:
			err = capturedService.AddWin(capturedCurrentJournal, inputText)
			if err != nil {
				return errMsg{err}
			}
			return journalUpdatedMsg{}

		case components.ModeLog:
			err = capturedService.AppendLog(capturedCurrentJournal, inputText)
			if err != nil {
				return errMsg{err}
			}
			return journalUpdatedMsg{}

		case components.ModeNote:
			// Add note to the selected task
			if capturedTaskService != nil && capturedTaskID != "" {
				err = capturedTaskService.AddTaskNote(capturedTaskID, inputText)
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
			return errMsg{fmt.Errorf("task service not available or no task selected")}

		case components.ModeLogNote:
			// Add note to the selected log entry
			if capturedLogEntryID != "" {
				err = capturedService.AddLogNote(capturedCurrentJournal, capturedLogEntryID, inputText)
				if err != nil {
					return errMsg{err}
				}
				return logNoteAddedMsg{}
			}
			return errMsg{fmt.Errorf("no log entry selected")}

		case components.ModeEditTask:
			// Edit task
			if capturedTaskService != nil && capturedEditID != "" && capturedEditType == "task" {
				err = capturedTaskService.UpdateTask(capturedEditID, inputText, capturedEditTags)
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
				err = capturedService.UpdateIntention(capturedCurrentJournal, capturedEditID, inputText)
				if err != nil {
					return errMsg{err}
				}
				return journalUpdatedMsg{}
			}
			return errMsg{fmt.Errorf("no intention selected for editing")}

		case components.ModeEditWin:
			// Edit win
			if capturedEditID != "" && capturedEditType == "win" {
				err = capturedService.UpdateWin(capturedCurrentJournal, capturedEditID, inputText)
				if err != nil {
					return errMsg{err}
				}
				return journalUpdatedMsg{}
			}
			return errMsg{fmt.Errorf("no win selected for editing")}

		case components.ModeEditLog:
			// Edit log entry
			if capturedEditID != "" && capturedEditType == "log" {
				err = capturedService.UpdateLogEntry(capturedCurrentJournal, capturedEditID, inputText)
				if err != nil {
					return errMsg{err}
				}
				return journalUpdatedMsg{}
			}
			return errMsg{fmt.Errorf("no log entry selected for editing")}

		default:
			return errMsg{fmt.Errorf("unknown entry mode")}
		}
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
