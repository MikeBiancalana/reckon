package cli

import tea "github.com/charmbracelet/bubbletea"

// handleKey is the keyboard priority-chain dispatcher: sub-flow-input-active
// > text-entry-active > focused-pane-normal > global (Tab focus-cycle across
// the 4 fixed panes, quit, help). Also hosts the agenda actuator sub-flow
// state machine (plan.md "Agenda actuator sub-flow", AC §2.11): read-only
// guard first, then no-arg keys (t/x/i/c) dispatch immediately while arg
// keys (d/D/p) open an input sub-flow before dispatching.
func (m *tuiModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// TODO(reckon-fnqs.8): implement
	return m, nil
}
