package tui

import (
	"testing"

	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/MikeBiancalana/reckon/internal/tui/components"
	tea "github.com/charmbracelet/bubbletea"
)

// TestHandleWindowSize tests the window resize handler
func TestHandleWindowSize(t *testing.T) {
	t.Run("sets width and height", func(t *testing.T) {
		m := &Model{}
		msg := tea.WindowSizeMsg{Width: 120, Height: 40}

		updatedModel, _ := m.handleWindowSize(msg)
		model := updatedModel.(*Model)

		if model.width != 120 {
			t.Errorf("expected width 120, got %d", model.width)
		}
		if model.height != 40 {
			t.Errorf("expected height 40, got %d", model.height)
		}
	})

	t.Run("sets terminalTooSmall when below minimum", func(t *testing.T) {
		m := &Model{}
		msg := tea.WindowSizeMsg{Width: 60, Height: 20}

		updatedModel, _ := m.handleWindowSize(msg)
		model := updatedModel.(*Model)

		if !model.terminalTooSmall {
			t.Error("expected terminalTooSmall to be true")
		}
	})

	t.Run("clears terminalTooSmall when above minimum", func(t *testing.T) {
		m := &Model{terminalTooSmall: true}
		msg := tea.WindowSizeMsg{Width: 120, Height: 40}

		updatedModel, _ := m.handleWindowSize(msg)
		model := updatedModel.(*Model)

		if model.terminalTooSmall {
			t.Error("expected terminalTooSmall to be false")
		}
	})
}

// TestHandleJournalLoaded tests the journal loaded handler
func TestHandleJournalLoaded(t *testing.T) {
	t.Run("sets current journal", func(t *testing.T) {
		m := &Model{}
		j := journal.Journal{
			Date: "2026-01-19",
			Intentions: []journal.Intention{
				{ID: "1", Text: "Test intention"},
			},
		}
		msg := journalLoadedMsg{journal: j}

		updatedModel, _ := m.handleJournalLoaded(msg)
		model := updatedModel.(*Model)

		if model.currentJournal == nil {
			t.Fatal("expected currentJournal to be set")
		}
		if model.currentJournal.Date != "2026-01-19" {
			t.Errorf("expected date 2026-01-19, got %s", model.currentJournal.Date)
		}
	})

	t.Run("creates intention list when nil", func(t *testing.T) {
		m := &Model{}
		j := journal.Journal{
			Intentions: []journal.Intention{
				{ID: "1", Text: "Test intention"},
			},
		}
		msg := journalLoadedMsg{journal: j}

		updatedModel, _ := m.handleJournalLoaded(msg)
		model := updatedModel.(*Model)

		if model.intentionList == nil {
			t.Error("expected intentionList to be created")
		}
	})

	t.Run("updates existing intention list", func(t *testing.T) {
		m := &Model{
			intentionList: components.NewIntentionList([]journal.Intention{}),
		}
		j := journal.Journal{
			Intentions: []journal.Intention{
				{ID: "1", Text: "Test intention"},
			},
		}
		msg := journalLoadedMsg{journal: j}

		updatedModel, _ := m.handleJournalLoaded(msg)
		model := updatedModel.(*Model)

		if model.intentionList == nil {
			t.Error("expected intentionList to remain set")
		}
	})
}

// TestHandleJournalUpdated tests the journal updated handler
func TestHandleJournalUpdated(t *testing.T) {
	t.Run("returns loadJournal command", func(t *testing.T) {
		m := &Model{
			service:     &journal.Service{},
			currentDate: "2026-01-19",
		}
		msg := journalUpdatedMsg{}

		_, cmd := m.handleJournalUpdated(msg)

		if cmd == nil {
			t.Error("expected command to be returned")
		}
	})
}

// TestHandleTasksLoaded tests the tasks loaded handler
func TestHandleTasksLoaded(t *testing.T) {
	t.Run("creates task list when nil", func(t *testing.T) {
		m := &Model{}
		tasks := []journal.Task{
			{ID: "1", Text: "Test task", Status: journal.TaskOpen},
		}
		msg := tasksLoadedMsg{tasks: tasks}

		updatedModel, _ := m.handleTasksLoaded(msg)
		model := updatedModel.(*Model)

		if model.taskList == nil {
			t.Error("expected taskList to be created")
		}
	})

	t.Run("updates existing task list", func(t *testing.T) {
		m := &Model{
			taskList: components.NewTaskList([]journal.Task{}),
		}
		tasks := []journal.Task{
			{ID: "1", Text: "Test task", Status: journal.TaskOpen},
		}
		msg := tasksLoadedMsg{tasks: tasks}

		updatedModel, _ := m.handleTasksLoaded(msg)
		model := updatedModel.(*Model)

		if model.taskList == nil {
			t.Error("expected taskList to remain set")
		}
	})
}

// TestHandleTaskToggle tests the task toggle handler
func TestHandleTaskToggle(t *testing.T) {
	t.Run("returns nil when taskService is nil", func(t *testing.T) {
		m := &Model{}
		msg := components.TaskToggleMsg{TaskID: "1"}

		_, cmd := m.handleTaskToggle(msg)

		if cmd != nil {
			t.Error("expected no command when taskService is nil")
		}
	})

	t.Run("returns command when taskService exists", func(t *testing.T) {
		m := &Model{
			taskService: &journal.TaskService{},
		}
		msg := components.TaskToggleMsg{TaskID: "1"}

		_, cmd := m.handleTaskToggle(msg)

		if cmd == nil {
			t.Error("expected command when taskService exists")
		}
	})
}

// TestHandleTaskSelectionChanged tests the task selection changed handler
func TestHandleTaskSelectionChanged(t *testing.T) {
	t.Run("updates notes for selected task", func(t *testing.T) {
		m := &Model{
			taskList:  components.NewTaskList([]journal.Task{}),
			notesPane: components.NewNotesPane(),
		}
		msg := components.TaskSelectionChangedMsg{}

		updatedModel, _ := m.handleTaskSelectionChanged(msg)
		model := updatedModel.(*Model)

		// Just verify it doesn't panic and returns model
		if model == nil {
			t.Error("expected model to be returned")
		}
	})
}

// TestHandleError tests the error handler
func TestHandleError(t *testing.T) {
	t.Run("stores error in model", func(t *testing.T) {
		m := &Model{}
		testErr := errMsg{err: &testError{"test error"}}
		msg := testErr

		updatedModel, _ := m.handleError(msg)
		model := updatedModel.(*Model)

		if model.lastError == nil {
			t.Fatal("expected error to be stored")
		}
		if model.lastError.Error() != "test error" {
			t.Errorf("expected error 'test error', got %v", model.lastError)
		}
	})
}

// testError is a simple error type for testing
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

// TestHandleLogNoteAdd tests the log note add handler
func TestHandleLogNoteAdd(t *testing.T) {
	t.Run("sets mode and focuses text entry", func(t *testing.T) {
		m := &Model{
			textEntryBar: components.NewTextEntryBar(),
			statusBar:    components.NewStatusBar(),
		}
		msg := components.LogNoteAddMsg{LogEntryID: "log123"}

		updatedModel, cmd := m.handleLogNoteAdd(msg)
		model := updatedModel.(*Model)

		if model.noteLogEntryID != "log123" {
			t.Errorf("expected noteLogEntryID to be log123, got %s", model.noteLogEntryID)
		}
		if cmd == nil {
			t.Error("expected focus command to be returned")
		}
	})
}

// TestHandleLogNoteDelete tests the log note delete handler
func TestHandleLogNoteDelete(t *testing.T) {
	t.Run("enters confirm mode", func(t *testing.T) {
		m := &Model{}
		msg := components.LogNoteDeleteMsg{
			LogEntryID: "log123",
			NoteID:     "note456",
		}

		updatedModel, _ := m.handleLogNoteDelete(msg)
		model := updatedModel.(*Model)

		if !model.confirmMode {
			t.Error("expected confirmMode to be true")
		}
		if model.confirmItemType != "log_note" {
			t.Errorf("expected confirmItemType to be log_note, got %s", model.confirmItemType)
		}
		if model.confirmItemID != "note456" {
			t.Errorf("expected confirmItemID to be note456, got %s", model.confirmItemID)
		}
		if model.confirmLogEntryID != "log123" {
			t.Errorf("expected confirmLogEntryID to be log123, got %s", model.confirmLogEntryID)
		}
	})
}

// TestHandleTaskNoteDelete tests the task note delete handler
func TestHandleTaskNoteDelete(t *testing.T) {
	t.Run("enters confirm mode", func(t *testing.T) {
		m := &Model{}
		msg := components.TaskNoteDeleteMsg{
			TaskID: "task123",
			NoteID: "note456",
		}

		updatedModel, _ := m.handleTaskNoteDelete(msg)
		model := updatedModel.(*Model)

		if !model.confirmMode {
			t.Error("expected confirmMode to be true")
		}
		if model.confirmItemType != "task_note" {
			t.Errorf("expected confirmItemType to be task_note, got %s", model.confirmItemType)
		}
		if model.confirmItemID != "note456" {
			t.Errorf("expected confirmItemID to be note456, got %s", model.confirmItemID)
		}
		if model.confirmLogEntryID != "task123" {
			t.Errorf("expected confirmLogEntryID to be task123, got %s", model.confirmLogEntryID)
		}
	})
}
