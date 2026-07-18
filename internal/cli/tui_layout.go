package cli

import tea "github.com/charmbracelet/bubbletea"

// paneDims is the computed width/height for each of the 4 fixed panes,
// derived from the terminal's total width/height by calcPaneDims.
type paneDims struct {
	agendaWidth, agendaHeight int
	todosWidth, todosHeight   int
	logWidth, logHeight       int
	notesWidth, notesHeight   int
}

// calcPaneDims computes each pane's width/height from the terminal's total
// w/h, clamping negative dimensions to 0 (edge case: a resize below the
// panes' minimum layout). Fixed 2x2 grid: agenda/todos share the left
// column, log/notes share the right column.
func calcPaneDims(w, h int) paneDims {
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	leftW := w / 2
	rightW := w - leftW
	topH := h / 2
	bottomH := h - topH

	return paneDims{
		agendaWidth: leftW, agendaHeight: topH,
		todosWidth: leftW, todosHeight: bottomH,
		logWidth: rightW, logHeight: topH,
		notesWidth: rightW, notesHeight: bottomH,
	}
}

// handleWindowSize recomputes pane dimensions for a tea.WindowSizeMsg and
// propagates them via each pane wrapper's SetSize.
func (m *tuiModel) handleWindowSize(msg tea.WindowSizeMsg) tea.Cmd {
	w, h := msg.Width, msg.Height
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	m.width = w
	m.height = h

	dims := calcPaneDims(w, h)
	m.agenda.SetSize(dims.agendaWidth, dims.agendaHeight)
	m.todos.SetSize(dims.todosWidth, dims.todosHeight)
	m.log.SetSize(dims.logWidth, dims.logHeight)
	m.notes.SetSize(dims.notesWidth, dims.notesHeight)
	return nil
}
