package cli

import (
	"fmt"
	"strings"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/index"
	"github.com/MikeBiancalana/reckon/internal/models"
	"github.com/MikeBiancalana/reckon/internal/tui/components"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
// sub-flow capture (agenda d/D/p arg entry) steals key events from the
// focused pane's own normal-mode handling until it completes or is
// cancelled.
type tuiInputMode int

const (
	inputModeNormal tuiInputMode = iota
	inputModeSubFlow
)

// tuiSubFlowKind identifies which agenda actuator arg sub-flow
// (tui_keyboard.go) is currently capturing input, when inputMode is
// inputModeSubFlow.
type tuiSubFlowKind int

const (
	subFlowNone tuiSubFlowKind = iota
	subFlowAgendaDefer
	subFlowAgendaDeadline
	subFlowAgendaPriority
)

// tuiModel is the top-level bubbletea model for `rk tui`: a persistent
// 4-pane porcelain over the index (reads) and the unexported CLI verbs
// (writes). All orchestration lives in package cli because the verbs it
// calls are unexported.
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

	// subFlow/subFlowRef track the in-progress agenda actuator arg capture
	// (d/D/p); datePicker and textEntry are the two widgets those sub-flows
	// drive.
	subFlow    tuiSubFlowKind
	subFlowRef string
	datePicker *components.DatePicker
	textEntry  *components.TextEntryBar

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
	return tea.Batch(m.loadAgendaCmd(), m.loadTodosCmd(), m.loadLogCmd(), m.loadNotesListCmd())
}

// Update is the flat msg.(type) dispatcher for every message tuiModel
// handles: window resize (tui_layout.go), key input (tui_keyboard.go), the
// 4 panes' async load results, mutation completion, and errors.
func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m, m.handleWindowSize(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)

	case agendaLoadedMsg:
		items := msg.items
		if items == nil {
			items = []agendaItem{}
		}
		m.agenda.items = items
		m.agenda.reselect()
		return m, nil

	case todosLoadedMsg:
		items := msg.items
		if items == nil {
			items = []todoListItem{}
		}
		m.todos.items = items
		m.todos.reselect()
		return m, nil

	case logLoadedMsg:
		m.log.view.UpdateLogEntries(msg.entries)
		return m, nil

	case notesListLoadedMsg:
		m.notes.notes = msg.notes
		m.notes.picker.Show(msg.notes)
		return m, nil

	case components.NotePickerSelectMsg:
		return m, m.selectNoteCmd(msg.NoteSlug)

	case notesLinksLoadedMsg:
		m.notes.links.UpdateLinks(msg.noteID, msg.outgoing, msg.backlinks)
		m.notes.links.SetFocused(true)
		m.notes.mode = notesShowInspect
		return m, nil

	case components.LinkSelectedMsg:
		if msg.NoteID == "" {
			// Unresolved link: nothing to navigate to.
			return m, nil
		}
		m.notes.links.SetLoading(msg.NoteID, true)
		return m, m.loadNotesLinksCmd(msg.NoteID)

	case mutationDoneMsg:
		return m, m.reloadCmdFor(msg.kind)

	case errMsg:
		m.lastErr = msg.err
		return m, nil
	}
	return m, nil
}

// View renders the modal-state branch (agenda actuator arg sub-flow) or
// falls through to the 4-pane layout.
func (m *tuiModel) View() string {
	if m.inputMode == inputModeSubFlow {
		switch m.subFlow {
		case subFlowAgendaDefer, subFlowAgendaDeadline:
			return m.datePicker.View()
		case subFlowAgendaPriority:
			return m.textEntry.View()
		}
	}

	agendaBox := renderPaneBox("Agenda", m.focus == focusAgenda, m.agenda.width, m.agenda.height, renderAgendaBody(m.agenda))
	todosBox := renderPaneBox("Todos", m.focus == focusTodos, m.todos.width, m.todos.height, renderTodosBody(m.todos))
	logBox := renderPaneBox("Log", m.focus == focusLog, m.log.width, m.log.height, m.log.view.View())
	var notesBody string
	if m.notes.mode == notesShowBrowse {
		notesBody = m.notes.picker.View()
	} else {
		notesBody = m.notes.links.View()
	}
	notesBox := renderPaneBox("Notes", m.focus == focusNotes, m.notes.width, m.notes.height, notesBody)

	left := lipgloss.JoinVertical(lipgloss.Left, agendaBox, todosBox)
	right := lipgloss.JoinVertical(lipgloss.Left, logBox, notesBox)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	if m.lastErr != nil {
		return body + "\n" + tuiErrStyle.Render("error: "+m.lastErr.Error())
	}
	return body
}

// ─────────────────────────────────────────────────────────────────────────────
// Load cmd builders (Init + reload-after-mutation): each closure captures
// only *sql.DB/*index.Index and any plain values it needs, never a pointer
// into pane state, so a stale/in-flight load can never write back over
// newer data (async-closure-capture pitfall, docs/REVIEW_PATTERNS.md).
// ─────────────────────────────────────────────────────────────────────────────

func (m *tuiModel) loadAgendaCmd() tea.Cmd {
	db := m.ix.DB()
	today := todoNow().Format("2006-01-02")
	return func() tea.Msg {
		items, warnings, err := buildAgenda(db, today)
		if err != nil {
			return errMsg{err: err}
		}
		return agendaLoadedMsg{items: items, warnings: warnings}
	}
}

func (m *tuiModel) loadTodosCmd() tea.Cmd {
	db := m.ix.DB()
	return func() tea.Msg {
		durItems, err := listDurableTodos(db, false, "")
		if err != nil {
			return errMsg{err: err}
		}
		ephItems, err := listEphemeralTodos(db, false)
		if err != nil {
			return errMsg{err: err}
		}
		items := append(append([]todoListItem{}, durItems...), ephItems...)
		return todosLoadedMsg{items: items}
	}
}

func (m *tuiModel) loadLogCmd() tea.Cmd {
	db := m.ix.DB()
	return func() tea.Msg {
		entries, err := loadLogEntries(db)
		if err != nil {
			return errMsg{err: err}
		}
		return logLoadedMsg{entries: entries}
	}
}

func (m *tuiModel) loadNotesListCmd() tea.Cmd {
	db := m.ix.DB()
	return func() tea.Msg {
		notes, err := listNotes(db)
		if err != nil {
			return errMsg{err: err}
		}
		return notesListLoadedMsg{notes: notes}
	}
}

func (m *tuiModel) loadNotesLinksCmd(noteID string) tea.Cmd {
	db := m.ix.DB()
	return func() tea.Msg {
		outgoing, backlinks, err := loadNotesPaneLinks(db, noteID)
		if err != nil {
			return errMsg{err: err}
		}
		return notesLinksLoadedMsg{noteID: noteID, outgoing: outgoing, backlinks: backlinks}
	}
}

// selectNoteCmd resolves a NotePickerSelectMsg's slug to a note id and fires
// the link-graph load for it.
func (m *tuiModel) selectNoteCmd(slug string) tea.Cmd {
	db := m.ix.DB()
	return func() tea.Msg {
		id, err := resolveNoteIDBySlug(db, slug)
		if err != nil {
			return errMsg{err: err}
		}
		if id == "" {
			return errMsg{err: fmt.Errorf("tui: notes pane: no note found matching slug %q", slug)}
		}
		outgoing, backlinks, err := loadNotesPaneLinks(db, id)
		if err != nil {
			return errMsg{err: err}
		}
		return notesLinksLoadedMsg{noteID: id, outgoing: outgoing, backlinks: backlinks}
	}
}

// reloadCmdFor fires the reload cmd for the pane a mutation just wrote to.
// Only "agenda" is wired today (the actuator sub-flow, tui_keyboard.go); the
// string key lets a future todos/log/notes creation flow reuse this same
// dispatch (see loadTodosCmd/loadLogCmd/loadNotesListCmd).
func (m *tuiModel) reloadCmdFor(kind string) tea.Cmd {
	switch kind {
	case "agenda":
		return m.loadAgendaCmd()
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// View rendering (plain, no-frills: each pane is a bordered box, the
// focused one highlighted).
// ─────────────────────────────────────────────────────────────────────────────

var (
	tuiPaneStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)

	tuiFocusedPaneStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("39")).
				Padding(0, 1)

	tuiPaneTitleStyle = lipgloss.NewStyle().Bold(true)

	tuiErrStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

// renderPaneBox wraps body in a titled, bordered box sized to width/height,
// styled to indicate whether it currently has focus. lipgloss's Width/Height
// set the CONTENT box (before padding/border are added on top), so the
// requested outer width/height must have the style's own decoration
// (1-cell border each side, plus this style's Padding(0,1) horizontally)
// subtracted first, or the rendered box comes out larger than the caller
// asked for.
func renderPaneBox(title string, focused bool, width, height int, body string) string {
	style := tuiPaneStyle
	if focused {
		style = tuiFocusedPaneStyle
	}
	const (
		borderH = 2 // 1 cell left + right
		borderV = 2 // 1 cell top + bottom
		padH    = 2 // Padding(0, 1): 1 cell left + right
	)
	innerW, innerH := width-borderH-padH, height-borderV
	if innerW < 0 {
		innerW = 0
	}
	if innerH < 1 {
		innerH = 1
	}
	content := tuiPaneTitleStyle.Render(title) + "\n" + body
	return style.Width(innerW).Height(innerH).Render(content)
}

// renderAgendaBody renders the agenda pane's hand-rolled row list.
func renderAgendaBody(p *agendaPane) string {
	if len(p.items) == 0 {
		return "today: nothing due"
	}
	var b strings.Builder
	for i, it := range p.items {
		cursor := "  "
		if i == p.selected {
			cursor = "> "
		}
		marker := ""
		if it.ReadOnly {
			marker = " [read-only]"
		}
		fmt.Fprintf(&b, "%s[%s]%s %s\n", cursor, it.State, marker, it.Title)
	}
	return b.String()
}

// renderTodosBody renders the todos pane's hand-rolled, subject-only row
// list: the item's Title (or Body as fallback), never the full node body.
func renderTodosBody(p *todosPane) string {
	if len(p.items) == 0 {
		return "todo: no items"
	}
	var b strings.Builder
	for i, it := range p.items {
		cursor := "  "
		if i == p.selected {
			cursor = "> "
		}
		text := it.Title
		if text == "" {
			text = it.Body
		}
		fmt.Fprintf(&b, "%s%s\n", cursor, text)
	}
	return b.String()
}
