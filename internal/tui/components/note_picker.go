package components

import (
	"fmt"
	"io"
	"strings"

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
	row IndexRow
}

func (i notePickerItem) FilterValue() string {
	// Allow filtering by both title and slug
	return i.row.Title + " " + i.row.Props["slug"]
}

func (i notePickerItem) Title() string {
	return i.row.Title
}

func (i notePickerItem) Description() string {
	var parts []string

	// Add slug
	if slug := i.row.Props["slug"]; slug != "" {
		parts = append(parts, "slug: "+slug)
	}

	// Add tags if present
	if tags := i.row.Props["tags"]; tags != "" {
		parts = append(parts, "tags: "+tags)
	}

	// Add created date
	if created := i.row.Props["created"]; created != "" {
		parts = append(parts, "created: "+created)
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
	notes        []IndexRow
	selectedNote *IndexRow
	canceled     bool
	width        int
	height       int

	// embedded is true when the picker is mounted inline as part of a larger
	// pane region rather than shown as a self-contained modal popup. View()
	// skips notePickerBoxStyle's own border/padding wrap when true, so the
	// picker doesn't double up against its host's own frame.
	embedded bool
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

// Show displays the note picker with the given rows
func (np *NotePicker) Show(rows []IndexRow) tea.Cmd {
	np.visible = true
	np.notes = rows
	np.selectedNote = nil
	np.canceled = false

	// Convert rows to list items
	items := make([]list.Item, len(rows))
	for i, row := range rows {
		items[i] = notePickerItem{row: row}
	}

	np.list.SetItems(items)
	np.list.ResetFilter()
	return nil
}

// Hide hides the note picker. It does not touch selectedNote -- Show() is
// the reset point for selection state, so Update can set selectedNote
// around a Hide() call in either order.
func (np *NotePicker) Hide() {
	np.visible = false
}

// IsVisible returns whether the note picker is visible
func (np *NotePicker) IsVisible() bool {
	return np.visible
}

// IsFiltering reports whether the picker's internal list is mid-filter-entry
// (the user has pressed "/" and is typing into the filter input). A host
// composing this picker inline (e.g. the notes pane's browse mode) must
// check this before treating a plain letter key as its own shortcut, or it
// steals keystrokes the filter input should have received.
func (np *NotePicker) IsFiltering() bool {
	return np.list.FilterState() == list.Filtering
}

// GetSelectedNoteSlug returns the slug of the selected note, or empty string
// if none. The slug is read from Props["slug"], not ID -- IndexRow.ID has no
// fixed relationship to a note's slug.
func (np *NotePicker) GetSelectedNoteSlug() string {
	if np.selectedNote == nil {
		return ""
	}
	return np.selectedNote.Props["slug"]
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

// SetHeight sets the height of the note picker's internal list. The
// constructor (list.New(items, delegate, 0, 0)) never sets a height, so
// before this method existed the list always rendered at height 0 — fine for
// the self-sized modal-popup usage (SetWidth's own hardcoded listHeight
// covers that), but wrong for mounting the picker inline in a fixed-height
// pane region.
func (np *NotePicker) SetHeight(h int) {
	np.height = h
	np.list.SetHeight(h)
}

// SetEmbedded sets whether the picker is mounted inline within a host pane
// (true) or rendered as a self-contained modal popup (false, the default).
// View() skips notePickerBoxStyle's own border/padding wrap when embedded,
// so the picker doesn't double up against its host's own frame.
func (np *NotePicker) SetEmbedded(embedded bool) {
	np.embedded = embedded
}

// Init satisfies Prompt[string]. Priming (Show) already happened before a
// NotePicker is handed to RunPrompt/Wizard, so there is nothing to do here.
func (np *NotePicker) Init() tea.Cmd { return nil }

// Result returns the selected note's slug. Only meaningful once Done()
// reports finished.
func (np *NotePicker) Result() string { return np.GetSelectedNoteSlug() }

// Done reports whether Update has reached a terminal state.
func (np *NotePicker) Done() (finished, canceled bool) {
	return np.selectedNote != nil, np.canceled
}

// Update handles Bubble Tea messages
func (np *NotePicker) Update(msg tea.Msg) (Prompt[string], tea.Cmd) {
	if !np.visible {
		return np, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc:
			np.Hide()
			np.canceled = true
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

			np.selectedNote = &item.row
			np.Hide()

			return np, func() tea.Msg {
				return NotePickerSelectMsg{
					NoteSlug: item.row.Props["slug"],
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

	if np.embedded {
		return content.String()
	}

	// Wrap in box
	return notePickerBoxStyle.Render(content.String())
}
