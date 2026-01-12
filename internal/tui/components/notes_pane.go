package components

import (
	"fmt"
	"io"

	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	notePaneStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	focusedNotePaneStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("11"))

	notePaneTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("205")).
				Bold(true)

	focusedNotePaneTitleStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("11")).
					Bold(true)
)

// NoteItem represents a note in the list
type NoteItem struct {
	note journal.TaskNote
}

func (n NoteItem) FilterValue() string { return n.note.Text }

// NoteDelegate handles rendering of note items
type NoteDelegate struct{}

func (d NoteDelegate) Height() int                               { return 1 }
func (d NoteDelegate) Spacing() int                              { return 0 }
func (d NoteDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return nil }

func (d NoteDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	ni, ok := listItem.(NoteItem)
	if !ok {
		return
	}

	text := ni.note.Text

	if index == m.Index() {
		text = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true).Render("▶ " + text)
	} else {
		text = notePaneStyle.Render(text)
	}

	fmt.Fprintf(w, "%s", text)
}

// NotesPane represents the notes component
type NotesPane struct {
	list    list.Model
	focused bool
	taskID  string
}

func NewNotesPane() *NotesPane {
	l := list.New([]list.Item{}, NoteDelegate{}, 0, 0)
	l.Title = "Notes"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = notePaneTitleStyle

	return &NotesPane{list: l}
}

// Update handles messages for the notes pane
func (np *NotesPane) Update(msg tea.Msg) (*NotesPane, tea.Cmd) {
	var cmd tea.Cmd
	np.list, cmd = np.list.Update(msg)
	return np, cmd
}

// View renders the notes pane
func (np *NotesPane) View() string {
	if len(np.list.Items()) == 0 {
		if np.taskID == "" {
			return "\nNo task selected"
		}
		return "\nNo notes for this task"
	}
	return np.list.View()
}

// SetSize sets the size of the list
func (np *NotesPane) SetSize(width, height int) {
	np.list.SetSize(width, height)
}

// SetFocused sets whether this component is focused
func (np *NotesPane) SetFocused(focused bool) {
	np.focused = focused
	if focused {
		np.list.Styles.Title = focusedNotePaneTitleStyle
	} else {
		np.list.Styles.Title = notePaneTitleStyle
	}
}

// UpdateNotes updates the list with notes for a specific task
func (np *NotesPane) UpdateNotes(taskID string, notes []journal.TaskNote) {
	np.taskID = taskID
	items := make([]list.Item, len(notes))
	for i, note := range notes {
		items[i] = NoteItem{note}
	}
	np.list.SetItems(items)
}
