package cli

import (
	"strconv"

	"github.com/MikeBiancalana/reckon/internal/models"
	"github.com/MikeBiancalana/reckon/internal/tui/components"
)

// clampDim clamps a pane dimension to >= 0 (edge case: resize below the
// layout's minimum, or a negative WindowSizeMsg value).
func clampDim(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

// agendaPane is the hand-rolled list wrapper over []agendaItem (today.go's
// buildAgenda output): ReadOnly/Title/State/Priority/dates, plus the
// actuator sub-flow (tui_keyboard.go).
type agendaPane struct {
	items    []agendaItem
	selected int
	// selectedID tracks the currently-selected row's ID independent of its
	// slice position, so a reload after a mutation can re-find the same row
	// even if items reordered (index-only selection identity is a pitfall:
	// docs/REVIEW_PATTERNS.md).
	selectedID string
	width      int
	height     int
}

func newAgendaPane() *agendaPane {
	return &agendaPane{}
}

// SetSize resizes the pane's viewport.
func (p *agendaPane) SetSize(width, height int) {
	p.width = clampDim(width)
	p.height = clampDim(height)
}

func (p *agendaPane) moveDown() {
	if p.selected < len(p.items)-1 {
		p.selected++
		p.selectedID = p.items[p.selected].ID
	}
}

func (p *agendaPane) moveUp() {
	if p.selected > 0 {
		p.selected--
		p.selectedID = p.items[p.selected].ID
	}
}

// selectedItem returns the currently-selected row, or false if the pane has
// no items or the cursor is out of range.
func (p *agendaPane) selectedItem() (agendaItem, bool) {
	if p.selected < 0 || p.selected >= len(p.items) {
		return agendaItem{}, false
	}
	return p.items[p.selected], true
}

// reselect re-derives p.selected from p.selectedID after a reload: the same
// row (by ID) if it's still present, else clamps the old index into range.
func (p *agendaPane) reselect() {
	if len(p.items) == 0 {
		p.selected = 0
		return
	}
	if p.selectedID != "" {
		for i, it := range p.items {
			if it.ID == p.selectedID {
				p.selected = i
				return
			}
		}
	}
	if p.selected < 0 {
		p.selected = 0
	}
	if p.selected >= len(p.items) {
		p.selected = len(p.items) - 1
	}
	p.selectedID = p.items[p.selected].ID
}

// todosPane is the hand-rolled, subject-only list wrapper over
// []todoListItem (todo.go's listDurableTodos/listEphemeralTodos output;
// AC#4: Title only, no body).
type todosPane struct {
	items      []todoListItem
	selected   int
	selectedID string
	width      int
	height     int
}

func newTodosPane() *todosPane {
	return &todosPane{}
}

// SetSize resizes the pane's viewport.
func (p *todosPane) SetSize(width, height int) {
	p.width = clampDim(width)
	p.height = clampDim(height)
}

func (p *todosPane) moveDown() {
	if p.selected < len(p.items)-1 {
		p.selected++
		p.selectedID = todoItemKey(p.items[p.selected])
	}
}

func (p *todosPane) moveUp() {
	if p.selected > 0 {
		p.selected--
		p.selectedID = todoItemKey(p.items[p.selected])
	}
}

// reselect mirrors agendaPane.reselect: keeps the same row selected across a
// reload, identified by todoItemKey rather than slice index.
func (p *todosPane) reselect() {
	if len(p.items) == 0 {
		p.selected = 0
		return
	}
	if p.selectedID != "" {
		for i, it := range p.items {
			if todoItemKey(it) == p.selectedID {
				p.selected = i
				return
			}
		}
	}
	if p.selected < 0 {
		p.selected = 0
	}
	if p.selected >= len(p.items) {
		p.selected = len(p.items) - 1
	}
	p.selectedID = todoItemKey(p.items[p.selected])
}

// todoItemKey is a todoListItem's selection identity: a durable item's ULID
// is stable across reloads; an ephemeral item has no ID, so its container
// path + line number stands in.
func todoItemKey(it todoListItem) string {
	if it.Kind == "durable" {
		return "d:" + it.ID
	}
	return "e:" + it.Container + "#" + strconv.Itoa(it.Line)
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
	p.width = clampDim(width)
	p.height = clampDim(height)
	p.view.SetSize(p.width, p.height)
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

	// notes is the last list loaded via listNotes, kept so browse mode can
	// re-Show the picker (Esc from inspect) without a fresh read.
	notes []*models.Note
}

func newNotesPane() *notesPane {
	p := &notesPane{
		picker: components.NewNotePicker("Notes"),
		links:  components.NewNotesPane(),
	}
	// This picker is always mounted inline as part of the notes pane's
	// region, never as a self-contained modal popup, so it must not draw
	// its own border on top of the pane's frame.
	p.picker.SetEmbedded(true)
	return p
}

// SetSize resizes the pane's viewport, propagating to both the picker
// (browse) and the links inspector (inspect) since either may be visible.
func (p *notesPane) SetSize(width, height int) {
	p.width = clampDim(width)
	p.height = clampDim(height)
	p.picker.SetWidth(p.width)
	p.picker.SetHeight(p.height)
	p.links.SetSize(p.width, p.height)
}
