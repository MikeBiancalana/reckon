package cli

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/MikeBiancalana/reckon/internal/tui/components"
)

// taskNoteStage represents the current stage in the note entry flow
type taskNoteStage int

const (
	taskNoteStagePickTask taskNoteStage = iota
	taskNoteStageEnterNote
	taskNoteStageDone
)

// taskNoteModel is the Bubble Tea model for the task note entry workflow
type taskNoteModel struct {
	stage      taskNoteStage
	picker     *components.TaskPicker
	editor     *components.TextEditor
	taskID     string
	noteText   string
	canceled   bool
	err        error
}

func (m taskNoteModel) Init() tea.Cmd {
	return nil
}

func (m taskNoteModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Allow ctrl+c to quit at any stage
		if msg.String() == "ctrl+c" {
			m.canceled = true
			return m, tea.Quit
		}

	case components.TaskPickerSelectMsg:
		// User selected a task, move to note entry stage
		m.taskID = msg.TaskID
		m.stage = taskNoteStageEnterNote
		return m, m.editor.Show()

	case components.TaskPickerCancelMsg:
		// User cancelled task picker
		m.canceled = true
		return m, tea.Quit

	case components.TextEditorSubmitMsg:
		// User submitted note text
		m.noteText = msg.Text
		m.stage = taskNoteStageDone
		return m, tea.Quit

	case components.TextEditorCancelMsg:
		// User cancelled text editor
		m.canceled = true
		return m, tea.Quit
	}

	// Update the current component based on stage
	var cmd tea.Cmd
	switch m.stage {
	case taskNoteStagePickTask:
		m.picker, cmd = m.picker.Update(msg)
	case taskNoteStageEnterNote:
		m.editor, cmd = m.editor.Update(msg)
	}

	return m, cmd
}

func (m taskNoteModel) View() string {
	switch m.stage {
	case taskNoteStagePickTask:
		return m.picker.View()
	case taskNoteStageEnterNote:
		return m.editor.View()
	default:
		return ""
	}
}

// PickTaskAndEnterNote launches an interactive workflow for selecting a task
// and entering a note. Returns the task ID, note text, whether it was canceled,
// and any error.
func PickTaskAndEnterNote(tasks []journal.Task) (taskID string, noteText string, canceled bool, err error) {
	if len(tasks) == 0 {
		return "", "", false, fmt.Errorf("no tasks available")
	}

	// Create picker
	picker := components.NewTaskPicker("Select Task for Note")
	picker.Show(tasks)

	// Create editor
	editor := components.NewTextEditor("Enter Note")
	editor.SetSize(80, 15)

	// Create model
	m := taskNoteModel{
		stage:  taskNoteStagePickTask,
		picker: picker,
		editor: editor,
	}

	// Run the program
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return "", "", false, fmt.Errorf("failed to run task note workflow: %w", err)
	}

	// Extract result
	result, ok := finalModel.(taskNoteModel)
	if !ok {
		return "", "", false, fmt.Errorf("unexpected model type returned")
	}

	if result.err != nil {
		return "", "", false, result.err
	}

	return result.taskID, result.noteText, result.canceled, nil
}

// PickOpenTaskAndEnterNote is a convenience function that filters tasks to only
// open tasks and launches the note entry workflow.
func PickOpenTaskAndEnterNote(allTasks []journal.Task) (taskID string, noteText string, canceled bool, err error) {
	// Filter to only open tasks
	openTasks := make([]journal.Task, 0, len(allTasks))
	for _, t := range allTasks {
		if t.Status == journal.TaskOpen {
			openTasks = append(openTasks, t)
		}
	}

	if len(openTasks) == 0 {
		return "", "", false, fmt.Errorf("no open tasks found")
	}

	return PickTaskAndEnterNote(openTasks)
}
