package components

import (
	"fmt"
	"io"
	"strings"

	"github.com/MikeBiancalana/reckon/internal/models"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
)

var (
	notePickerBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("39")).
				Padding(1, 2)

	notePickerTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("39"))

	notePickerItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))

	notePickerSelectedItemStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("39")).
					Bold(true)

	notePickerDescStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245"))

	notePickerHelpStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240"))
)

// NotePickerSelectMsg is sent when a note is selected
type NotePickerSelectMsg struct {
	NoteSlug string
}

// NotePickerCancelMsg is sent when the note picker is cancelled
type NotePickerCancelMsg struct{}

// notePickerItem implements list.Item for the note picker
type notePickerItem struct {
	note *models.Note
}

func (i notePickerItem) FilterValue() string {
	// Allow filtering by both title and slug
	return i.note.Title + " " + i.note.Slug
}

func (i notePickerItem) Title() string {
	return i.note.Title
}

func (i notePickerItem) Description() string {
	var parts []string

	// Add slug
	parts = append(parts, "slug: "+i.note.Slug)

	// Add tags if present
	if len(i.note.Tags) > 0 {
		parts = append(parts, "tags: "+strings.Join(i.note.Tags, ", "))
	}

	// Add created date
	if !i.note.CreatedAt.IsZero() {
		parts = append(parts, "created: "+i.note.CreatedAt.Format("2006-01-02"))
	}

	return strings.Join(parts, " | ")
}

// notePickerDelegate handles rendering of note picker items
type notePickerDelegate struct{}

func (d notePickerDelegate) Height() int  { return 2 }
func (d notePickerDelegate) Spacing() int { return 1 }
func (d notePickerDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return nil
}

func (d notePickerDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	item, ok := listItem.(notePickerItem)
	if !ok {
		return
	}

	title := item.Title()
	desc := item.Description()

	isSelected := index == m.Index()

	// Render title
	titleStyle := notePickerItemStyle
	if isSelected {
		titleStyle = notePickerSelectedItemStyle
		title = "> " + title
	} else {
		title = "  " + title
	}

	fmt.Fprint(w, titleStyle.Render(title))
	if desc != "" {
		fmt.Fprint(w, "\n  "+notePickerDescStyle.Render(desc))
	}
}

// NotePicker is a reusable fuzzy finder for selecting notes
type NotePicker struct {
	list         list.Model
	title        string
	visible      bool
	notes        []*models.Note
	selectedNote *models.Note
	width        int
}

// notePickerFuzzyFilter implements fuzzy matching for note picker items
func notePickerFuzzyFilter(term string, targets []string) []list.Rank {
	if term == "" {
		return nil
	}

	matches := fuzzy.Find(term, targets)
	ranks := make([]list.Rank, len(matches))

	for i, match := range matches {
		ranks[i] = list.Rank{
			Index:          match.Index,
			MatchedIndexes: match.MatchedIndexes,
		}
	}

	return ranks
}

// NewNotePicker creates a new note picker component
func NewNotePicker(title string) *NotePicker {
	delegate := notePickerDelegate{}
	l := list.New([]list.Item{}, delegate, 0, 0)
	l.Title = title
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.FilterInput.Prompt = "Filter: "
	l.Styles.Title = notePickerTitleStyle
	l.SetShowHelp(false)

	// Configure fuzzy matching filter
	l.Filter = notePickerFuzzyFilter

	return &NotePicker{
		list:    l,
		title:   title,
		visible: false,
		width:   80,
	}
}

// Show displays the note picker with the given notes
func (np *NotePicker) Show(notes []*models.Note) tea.Cmd {
	np.visible = true
	np.notes = notes
	np.selectedNote = nil

	// Convert notes to list items
	items := make([]list.Item, len(notes))
	for i, note := range notes {
		items[i] = notePickerItem{note: note}
	}

	np.list.SetItems(items)
	np.list.ResetFilter()
	return nil
}

// Hide hides the note picker
func (np *NotePicker) Hide() {
	np.visible = false
	np.selectedNote = nil
}

// IsVisible returns whether the note picker is visible
func (np *NotePicker) IsVisible() bool {
	return np.visible
}

// GetSelectedNoteSlug returns the slug of the selected note, or empty string if none
func (np *NotePicker) GetSelectedNoteSlug() string {
	if np.selectedNote == nil {
		return ""
	}
	return np.selectedNote.Slug
}

// SetWidth sets the width of the note picker
func (np *NotePicker) SetWidth(width int) {
	np.width = width
	listWidth := width - 10
	listHeight := 15
	if listWidth < 40 {
		listWidth = 40
	}
	np.list.SetSize(listWidth, listHeight)
}

// Update handles Bubble Tea messages
func (np *NotePicker) Update(msg tea.Msg) (*NotePicker, tea.Cmd) {
	if !np.visible {
		return np, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc:
			np.Hide()
			return np, func() tea.Msg {
				return NotePickerCancelMsg{}
			}

		case tea.KeyEnter:
			// Get selected item
			selectedItem := np.list.SelectedItem()
			if selectedItem == nil {
				return np, nil
			}

			item, ok := selectedItem.(notePickerItem)
			if !ok {
				return np, nil
			}

			np.selectedNote = item.note
			np.Hide()

			return np, func() tea.Msg {
				return NotePickerSelectMsg{
					NoteSlug: item.note.Slug,
				}
			}
		}
	}

	// Update the list
	var cmd tea.Cmd
	np.list, cmd = np.list.Update(msg)

	return np, cmd
}

// View renders the note picker
func (np *NotePicker) View() string {
	if !np.visible {
		return ""
	}

	var content strings.Builder

	// List view
	content.WriteString(np.list.View())
	content.WriteString("\n\n")

	// Help text
	helpText := "ENTER: select  ESC: cancel  /: filter"
	content.WriteString(notePickerHelpStyle.Render(helpText))

	// Wrap in box
	return notePickerBoxStyle.Render(content.String())
}
