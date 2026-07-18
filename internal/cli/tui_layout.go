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
// panes' minimum layout).
func calcPaneDims(w, h int) paneDims {
	// TODO(reckon-fnqs.8): implement
	return paneDims{}
}

// handleWindowSize recomputes pane dimensions for a tea.WindowSizeMsg and
// propagates them via each pane wrapper's SetSize.
func (m *tuiModel) handleWindowSize(msg tea.WindowSizeMsg) tea.Cmd {
	// TODO(reckon-fnqs.8): implement
	return nil
}
