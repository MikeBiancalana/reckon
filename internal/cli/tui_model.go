package cli

import (
	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/index"
	"github.com/MikeBiancalana/reckon/internal/models"
	"github.com/MikeBiancalana/reckon/internal/tui/components"
	tea "github.com/charmbracelet/bubbletea"
)

// tuiFocus identifies which of the 4 fixed panes currently has keyboard
// focus (tui_keyboard.go's global Tab focus-cycle).
type tuiFocus int

const (
	focusAgenda tuiFocus = iota
	focusTodos
	focusLog
	focusNotes
)

// tuiInputMode gates the keyboard priority chain (tui_keyboard.go): a
// sub-flow or text-entry capture steals key events from the focused pane's
// own normal-mode handling until it completes or is cancelled.
type tuiInputMode int

const (
	inputModeNormal tuiInputMode = iota
	inputModeTextEntry
	inputModeSubFlow
)

// tuiModel is the top-level bubbletea model for `rk tui`: a persistent
// 4-pane porcelain over the index (reads) and the unexported CLI verbs
// (writes). All orchestration lives in package cli because the verbs it
// calls are unexported (Decision: Package placement, plan.md).
type tuiModel struct {
	ix       *index.Index
	cfg      *config.Config
	vaultDir string

	agenda *agendaPane
	todos  *todosPane
	log    *logPane
	notes  *notesPane

	focus     tuiFocus
	inputMode tuiInputMode

	width  int
	height int

	lastErr error
}

// ─────────────────────────────────────────────────────────────────────────────
// Async result msg types
// ─────────────────────────────────────────────────────────────────────────────

// agendaLoadedMsg carries buildAgenda's result (today.go).
type agendaLoadedMsg struct {
	items    []agendaItem
	warnings []string
}

// todosLoadedMsg carries listDurableTodos+listEphemeralTodos's result
// (todo.go).
type todosLoadedMsg struct {
	items []todoListItem
}

// logLoadedMsg carries loadLogEntries's result (tui_read.go).
type logLoadedMsg struct {
	entries []components.LogEntryRow
}

// notesListLoadedMsg carries listNotes's result (tui_read.go).
type notesListLoadedMsg struct {
	notes []*models.Note
}

// notesLinksLoadedMsg carries loadNotesPaneLinks's result (tui_read.go) for
// one note.
type notesLinksLoadedMsg struct {
	noteID    string
	outgoing  []components.LinkDisplayItem
	backlinks []components.LinkDisplayItem
}

// mutationDoneMsg signals a verb call (addDurableTodo, dispatchTodayAct,
// appendLogEntry, createNote) completed and the index was reconciled; the
// model responds by re-firing the affected pane's load cmd.
type mutationDoneMsg struct {
	kind string
}

// errMsg carries an error from any async load or mutation cmd.
type errMsg struct {
	err error
}

// ─────────────────────────────────────────────────────────────────────────────
// tea.Model
// ─────────────────────────────────────────────────────────────────────────────

// Init batches the 4 initial pane load cmds.
func (m *tuiModel) Init() tea.Cmd {
	// TODO(reckon-fnqs.8): implement
	return nil
}

// Update is the flat msg.(type) dispatcher for every message tuiModel
// handles: window resize (tui_layout.go), key input (tui_keyboard.go), the
// 4 panes' async load results, mutation completion, and errors.
func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// TODO(reckon-fnqs.8): implement
	return m, nil
}

// View renders the modal-state branch (sub-flow/text-entry overlays) or
// falls through to the 4-pane layout.
func (m *tuiModel) View() string {
	// TODO(reckon-fnqs.8): implement
	return ""
}
