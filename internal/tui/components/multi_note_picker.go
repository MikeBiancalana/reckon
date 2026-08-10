package components

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// MultiNotePicker is a Prompt[[]string], mirroring NotePicker's list+delegate
// but multi-select: Space toggles the highlighted row's membership in the
// selection set, Enter confirms the whole set (possibly empty), Esc cancels.
// Result() is the selected notes' slugs (Props["slug"], not IndexRow.ID --
// same convention as NotePicker).
//
// STUB: Update/View are not yet implemented. Update currently returns mp
// unchanged with a nil cmd on every message, so mp never reaches a terminal
// state -- see TextPrompt's doc comment for the same rationale.
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

// Update handles Bubble Tea messages.
//
// NOT YET IMPLEMENTED: the real body must mirror NotePicker.Update with a
// Space-toggle branch added before the list update falls through, and Enter
// confirming (finished=true) regardless of whether the selection set is
// empty -- but is stubbed here to compile without prematurely passing any
// behavioral test.
func (mp *MultiNotePicker) Update(msg tea.Msg) (Prompt[[]string], tea.Cmd) {
	return mp, nil
}

// View renders the picker.
//
// NOT YET IMPLEMENTED.
func (mp *MultiNotePicker) View() string {
	return ""
}
