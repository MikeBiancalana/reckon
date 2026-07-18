package cli

import (
	"github.com/MikeBiancalana/reckon/internal/tui/components"
)

// agendaPane is the hand-rolled list wrapper over []agendaItem (today.go's
// buildAgenda output): ReadOnly/Title/State/Priority/dates, plus the
// actuator sub-flow (tui_keyboard.go).
type agendaPane struct {
	items    []agendaItem
	selected int
	width    int
	height   int
}

func newAgendaPane() *agendaPane {
	return &agendaPane{}
}

// SetSize resizes the pane's viewport.
func (p *agendaPane) SetSize(width, height int) {
	// TODO(reckon-fnqs.8): implement
}

// todosPane is the hand-rolled, subject-only list wrapper over
// []todoListItem (todo.go's listDurableTodos/listEphemeralTodos output;
// AC#4: Title only, no body).
type todosPane struct {
	items    []todoListItem
	selected int
	width    int
	height   int
}

func newTodosPane() *todosPane {
	return &todosPane{}
}

// SetSize resizes the pane's viewport.
func (p *todosPane) SetSize(width, height int) {
	// TODO(reckon-fnqs.8): implement
}

// logPane wraps the decoupled components.LogView.
type logPane struct {
	view   *components.LogView
	width  int
	height int
}

func newLogPane() *logPane {
	return &logPane{view: components.NewLogView(nil)}
}

// SetSize resizes the pane's viewport.
func (p *logPane) SetSize(width, height int) {
	// TODO(reckon-fnqs.8): implement
}

// notesPaneShowMode is the composite notes pane's internal mode (Decision:
// Notes-pane composition, plan.md): browse shows components.NotePicker,
// inspect shows components.NotesPane. Independent of top-level pane focus.
type notesPaneShowMode int

const (
	notesShowBrowse notesPaneShowMode = iota
	notesShowInspect
)

// notesPane is the composite wrapper owning focus at the top level: a
// components.NotePicker (browse) plus a components.NotesPane (inspect) over
// the same pane region.
type notesPane struct {
	picker *components.NotePicker
	links  *components.NotesPane
	mode   notesPaneShowMode
	width  int
	height int
}

func newNotesPane() *notesPane {
	return &notesPane{
		picker: components.NewNotePicker("Notes"),
		links:  components.NewNotesPane(),
	}
}

// SetSize resizes the pane's viewport.
func (p *notesPane) SetSize(width, height int) {
	// TODO(reckon-fnqs.8): implement
}
