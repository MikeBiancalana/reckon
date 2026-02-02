package cli

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/MikeBiancalana/reckon/internal/models"
	"github.com/MikeBiancalana/reckon/internal/tui/components"
)

// notePickerModel is a simple Bubble Tea model for the note picker
type notePickerModel struct {
	picker   *components.NotePicker
	noteSlug string
	canceled bool
}

func (m notePickerModel) Init() tea.Cmd {
	return nil
}

func (m notePickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Allow ctrl+c to quit
		if msg.String() == "ctrl+c" {
			m.canceled = true
			return m, tea.Quit
		}

	case components.NotePickerSelectMsg:
		// User selected a note
		m.noteSlug = msg.NoteSlug
		return m, tea.Quit

	case components.NotePickerCancelMsg:
		// User cancelled with ESC
		m.canceled = true
		return m, tea.Quit
	}

	// Update the picker
	var cmd tea.Cmd
	m.picker, cmd = m.picker.Update(msg)
	return m, cmd
}

func (m notePickerModel) View() string {
	return m.picker.View()
}

// PickNote launches an interactive note picker and returns the selected note slug.
// Returns the note slug, whether it was canceled, and any error.
// This is a helper function for use in CLI commands that need note selection.
func PickNote(notes []*models.Note, title string) (noteSlug string, canceled bool, err error) {
	if len(notes) == 0 {
		return "", false, fmt.Errorf("no notes available")
	}

	// Create picker
	picker := components.NewNotePicker(title)
	picker.Show(notes)

	// Create model
	m := notePickerModel{
		picker: picker,
	}

	// Run the program
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return "", false, fmt.Errorf("failed to run note picker: %w", err)
	}

	// Extract result with panic protection
	result, ok := finalModel.(notePickerModel)
	if !ok {
		return "", false, fmt.Errorf("unexpected model type returned from picker")
	}
	return result.noteSlug, result.canceled, nil
}
