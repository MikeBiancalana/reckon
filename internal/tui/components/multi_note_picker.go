package components

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// MultiNotePickerConfirmMsg is sent when the selection is confirmed with Enter.
type MultiNotePickerConfirmMsg struct {
	Slugs []string
}

// MultiNotePickerCancelMsg is sent when the picker is cancelled with Esc.
type MultiNotePickerCancelMsg struct{}

// MultiNotePicker is a Prompt[[]string], mirroring NotePicker's list+delegate
// but multi-select: Space toggles the highlighted row's membership in the
// selection set, Enter confirms the whole set (possibly empty), Esc cancels.
// Result() is the selected notes' slugs (Props["slug"], not IndexRow.ID --
// same convention as NotePicker).
type MultiNotePicker struct {
	list     list.Model
	title    string
	visible  bool
	notes    []IndexRow
	selected map[string]bool // keyed by Props["slug"]

	confirmed bool
	canceled  bool
}

// NewMultiNotePicker creates a new multi-select note picker component.
func NewMultiNotePicker(title string) *MultiNotePicker {
	l := list.New([]list.Item{}, notePickerDelegate{}, 0, 0)
	l.Title = title
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.FilterInput.Prompt = "Filter: "
	l.Styles.Title = notePickerTitleStyle
	l.SetShowHelp(false)
	l.Filter = notePickerFuzzyFilter

	return &MultiNotePicker{
		list:     l,
		title:    title,
		selected: make(map[string]bool),
	}
}

// Show displays the picker with the given rows.
func (mp *MultiNotePicker) Show(rows []IndexRow) tea.Cmd {
	mp.visible = true
	mp.notes = rows
	mp.selected = make(map[string]bool)
	mp.confirmed = false
	mp.canceled = false

	items := make([]list.Item, len(rows))
	for i, row := range rows {
		items[i] = notePickerItem{row: row}
	}
	mp.list.SetItems(items)
	mp.list.ResetFilter()
	return nil
}

// Hide hides the picker. It does not touch the selection set -- Show() is
// the reset point for selection state.
func (mp *MultiNotePicker) Hide() {
	mp.visible = false
}

// IsVisible returns whether the picker is visible.
func (mp *MultiNotePicker) IsVisible() bool {
	return mp.visible
}

// SelectedSlugs returns the currently toggled-on slugs, in no particular
// guaranteed order. Empty (never nil-vs-empty distinguished) when nothing is
// selected.
func (mp *MultiNotePicker) SelectedSlugs() []string {
	slugs := make([]string, 0, len(mp.selected))
	for slug, on := range mp.selected {
		if on {
			slugs = append(slugs, slug)
		}
	}
	return slugs
}

// Init satisfies Prompt[[]string]. Priming (Show) already happened before a
// MultiNotePicker is handed to RunPrompt/Wizard, so there is nothing to do
// here.
func (mp *MultiNotePicker) Init() tea.Cmd { return nil }

// Result returns the confirmed selection. Only meaningful once Done()
// reports finished.
func (mp *MultiNotePicker) Result() []string { return mp.SelectedSlugs() }

// Done reports whether Update has reached a terminal state.
func (mp *MultiNotePicker) Done() (finished, canceled bool) { return mp.confirmed, mp.canceled }

// Update handles Bubble Tea messages. Mirrors NotePicker.Update's Esc/Enter
// shape with a Space-toggle branch added ahead of the list fallthrough:
// Space toggles the highlighted row's slug in the selection set (a no-op
// while the filter input is active, so a literal space can still be typed
// into a filter query); Enter confirms unconditionally, empty selection
// included -- links are optional.
func (mp *MultiNotePicker) Update(msg tea.Msg) (Prompt[[]string], tea.Cmd) {
	if !mp.visible {
		return mp, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc:
			mp.Hide()
			mp.canceled = true
			return mp, func() tea.Msg {
				return MultiNotePickerCancelMsg{}
			}

		case tea.KeyEnter:
			mp.confirmed = true
			slugs := mp.SelectedSlugs()
			mp.Hide()
			return mp, func() tea.Msg {
				return MultiNotePickerConfirmMsg{Slugs: slugs}
			}

		case tea.KeySpace:
			if mp.list.FilterState() != list.Filtering {
				if item, ok := mp.list.SelectedItem().(notePickerItem); ok {
					slug := item.row.Props["slug"]
					if mp.selected[slug] {
						delete(mp.selected, slug)
					} else {
						mp.selected[slug] = true
					}
				}
				return mp, nil
			}
		}
	}

	var cmd tea.Cmd
	mp.list, cmd = mp.list.Update(msg)
	return mp, cmd
}

// View renders the picker.
func (mp *MultiNotePicker) View() string {
	if !mp.visible {
		return ""
	}

	var content strings.Builder
	content.WriteString(mp.list.View())
	content.WriteString("\n\n")

	selected := mp.SelectedSlugs()
	sort.Strings(selected)
	if len(selected) > 0 {
		content.WriteString(notePickerDescStyle.Render("Selected: " + strings.Join(selected, ", ")))
	} else {
		content.WriteString(notePickerDescStyle.Render("Selected: (none)"))
	}
	content.WriteString("\n")

	helpText := "SPACE: toggle  ENTER: confirm  ESC: cancel  /: filter"
	content.WriteString(notePickerHelpStyle.Render(helpText))

	return notePickerBoxStyle.Render(content.String())
}
