package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MikeBiancalana/reckon/internal/tui/components"
	tea "github.com/charmbracelet/bubbletea"
)

// handleKey is the keyboard priority-chain dispatcher: sub-flow-input-active
// > focused-pane-normal > global (Tab focus-cycle across the 4 fixed panes,
// quit). Also hosts the agenda actuator sub-flow state machine: read-only
// guard first, then no-arg keys (t/x/i/c) dispatch immediately while arg
// keys (d/D/p) open an input sub-flow before dispatching. The todos/log/notes
// creation flows (addDurableTodo, appendLogEntry, createNote) reuse the same
// text-entry sub-flow shape via their own "n" key in each pane's handler
// below.
func (m *tuiModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.inputMode == inputModeSubFlow {
		return m.handleSubFlowKey(msg)
	}

	// Global keys (no pane currently binds these, so ordering against the
	// focused-pane handlers below is a non-issue in practice).
	switch msg.Type {
	case tea.KeyTab:
		m.focus = nextFocus(m.focus)
		return m, nil
	case tea.KeyCtrlC:
		return m, tea.Quit
	}
	if msg.String() == "q" {
		return m, tea.Quit
	}

	switch m.focus {
	case focusAgenda:
		return m.handleAgendaKey(msg)
	case focusTodos:
		return m.handleTodosKey(msg)
	case focusLog:
		return m.handleLogKey(msg)
	case focusNotes:
		return m.handleNotesKey(msg)
	}
	return m, nil
}

// nextFocus advances to the next of the 4 fixed panes, in the stable order
// agenda -> todos -> log -> notes -> agenda (test scenario 13).
func nextFocus(f tuiFocus) tuiFocus {
	return (f + 1) % 4
}

// ─────────────────────────────────────────────────────────────────────────────
// Agenda pane: navigation + actuator dispatch.
// ─────────────────────────────────────────────────────────────────────────────

func (m *tuiModel) handleAgendaKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		m.agenda.moveDown()
		return m, nil
	case "k", "up":
		m.agenda.moveUp()
		return m, nil
	case "t", "d", "D", "p", "x", "i", "c":
		return m.dispatchAgendaActuator(msg.String())
	}
	return m, nil
}

// dispatchAgendaActuator implements the agenda actuator sub-flow: the
// read-only guard runs first and unconditionally (before any file touch or
// sub-flow is opened), then no-arg keys dispatch immediately while d/D/p
// open an input sub-flow that dispatches on completion.
func (m *tuiModel) dispatchAgendaActuator(key string) (tea.Model, tea.Cmd) {
	m.lastErr = nil

	item, ok := m.agenda.selectedItem()
	if !ok {
		return m, nil
	}

	// Gate on the loaded row's ReadOnly flag directly -- dispatchTodayAct
	// itself has no such guard (that lives in runTodayActE's cobra handler,
	// today.go:348-354), so calling it on a work-ticket ref would instead
	// fall through to loadNativeTodoForEdit's unrelated "no todo found"
	// error.
	if item.ReadOnly {
		m.lastErr = fmt.Errorf("%s is read-only (external work ticket); use rk today open instead", item.ID)
		return m, nil
	}

	m.agenda.selectedID = item.ID

	switch key {
	case "d":
		return m, m.startDateSubFlow(subFlowAgendaDefer, item.ID)
	case "D":
		return m, m.startDateSubFlow(subFlowAgendaDeadline, item.ID)
	case "p":
		return m, m.startPrioritySubFlow(item.ID)
	default: // t, x, i, c: no argument, dispatch immediately
		return m, m.actuateCmd(item.ID, key, "")
	}
}

// actuateCmd calls dispatchTodayAct (the same function `rk today act`
// calls) and reconciles the index on success, emitting mutationDoneMsg so
// the model re-fires the agenda reload. noLog is always false here, matching
// today.go:570-571's CLI default (--no-log defaults off) -- the
// complete-as-logging behavior that's the point of the agenda pane's 'x'
// key existing at all.
func (m *tuiModel) actuateCmd(ref, key, arg string) tea.Cmd {
	vaultDir := m.vaultDir
	ix := m.ix
	return func() tea.Msg {
		if _, err := dispatchTodayAct(vaultDir, ref, key, arg, false); err != nil {
			return errMsg{err: err}
		}
		if _, err := ix.Reconcile(); err != nil {
			return errMsg{err: err}
		}
		return mutationDoneMsg{kind: "agenda"}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Todos pane: navigation plus "n" (new) to add a durable todo.
// ─────────────────────────────────────────────────────────────────────────────

func (m *tuiModel) handleTodosKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		m.todos.moveDown()
	case "k", "up":
		m.todos.moveUp()
	case "n":
		return m, m.startAddTodoSubFlow()
	}
	return m, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Log pane: navigation (delegated to components.LogView) plus "n" (new) to
// append a log entry.
// ─────────────────────────────────────────────────────────────────────────────

func (m *tuiModel) handleLogKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "n" {
		return m, m.startAddLogSubFlow()
	}
	var cmd tea.Cmd
	m.log.view, cmd = m.log.view.Update(msg)
	return m, cmd
}

// ─────────────────────────────────────────────────────────────────────────────
// Notes pane: browse (NotePicker) / inspect (NotesPane). "n" (new) in browse
// mode creates a note, unless the picker is mid-filter-typing (its own "n"
// keystrokes must reach the filter input, not be stolen as a shortcut).
// ─────────────────────────────────────────────────────────────────────────────

func (m *tuiModel) handleNotesKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.notes.mode == notesShowBrowse {
		if msg.String() == "n" && !m.notes.picker.IsFiltering() {
			return m, m.startNewNoteSubFlow()
		}
		var cmd tea.Cmd
		m.notes.picker, cmd = m.notes.picker.Update(msg)
		return m, cmd
	}

	// inspect mode
	if msg.Type == tea.KeyEsc {
		m.notes.mode = notesShowBrowse
		m.notes.links.SetFocused(false)
		m.notes.picker.Show(m.notes.notes)
		return m, nil
	}
	var cmd tea.Cmd
	m.notes.links, cmd = m.notes.links.Update(msg)
	return m, cmd
}

// ─────────────────────────────────────────────────────────────────────────────
// Agenda actuator arg sub-flows: d/D (date) and p (priority letter).
// ─────────────────────────────────────────────────────────────────────────────

// startDateSubFlow opens the date-picker sub-flow for the "d"/"D" actuator
// keys, targeting ref.
func (m *tuiModel) startDateSubFlow(kind tuiSubFlowKind, ref string) tea.Cmd {
	m.subFlow = kind
	m.subFlowRef = ref
	m.inputMode = inputModeSubFlow
	return m.datePicker.Show()
}

// startPrioritySubFlow opens a short A/B/C text capture for the "p"
// actuator key, targeting ref.
func (m *tuiModel) startPrioritySubFlow(ref string) tea.Cmd {
	m.subFlow = subFlowAgendaPriority
	m.subFlowRef = ref
	m.inputMode = inputModeSubFlow
	m.textEntry.SetMode(components.ModeTask)
	m.textEntry.Clear()
	return m.textEntry.Focus()
}

// startAddTodoSubFlow opens the todos pane's "n" (new) text-entry sub-flow.
func (m *tuiModel) startAddTodoSubFlow() tea.Cmd {
	m.subFlow = subFlowAddTodo
	m.subFlowRef = ""
	m.inputMode = inputModeSubFlow
	m.textEntry.SetMode(components.ModeTask)
	m.textEntry.Clear()
	return m.textEntry.Focus()
}

// startAddLogSubFlow opens the log pane's "n" (new) text-entry sub-flow.
func (m *tuiModel) startAddLogSubFlow() tea.Cmd {
	m.subFlow = subFlowAddLog
	m.subFlowRef = ""
	m.inputMode = inputModeSubFlow
	m.textEntry.SetMode(components.ModeLog)
	m.textEntry.Clear()
	return m.textEntry.Focus()
}

// startNewNoteSubFlow opens the notes pane's "n" (new) text-entry sub-flow.
// v1 captures only a title (TextEntryBar is single-line): createNote's Slug
// is derived from it, and Body stays empty, matching the same
// minimal-fields shape the todos/log creation flows use.
func (m *tuiModel) startNewNoteSubFlow() tea.Cmd {
	m.subFlow = subFlowNewNote
	m.subFlowRef = ""
	m.inputMode = inputModeSubFlow
	m.textEntry.SetMode(components.ModeNote)
	m.textEntry.Clear()
	return m.textEntry.Focus()
}

// cancelSubFlow resets all sub-flow state back to normal-mode, hiding
// whichever widget was active.
func (m *tuiModel) cancelSubFlow() {
	m.subFlow = subFlowNone
	m.subFlowRef = ""
	m.inputMode = inputModeNormal
	m.datePicker.Hide()
	m.textEntry.Blur()
	m.textEntry.SetMode(components.ModeInactive)
}

// handleSubFlowKey routes keys to whichever structured sub-flow (agenda arg
// capture, or a pane's add/create text capture) is active.
func (m *tuiModel) handleSubFlowKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.subFlow {
	case subFlowAgendaDefer, subFlowAgendaDeadline:
		return m.handleDateSubFlowKey(msg)
	case subFlowAgendaPriority:
		return m.handlePrioritySubFlowKey(msg)
	case subFlowAddTodo:
		return m.handleAddTodoSubFlowKey(msg)
	case subFlowAddLog:
		return m.handleAddLogSubFlowKey(msg)
	case subFlowNewNote:
		return m.handleNewNoteSubFlowKey(msg)
	}
	m.cancelSubFlow()
	return m, nil
}

// handleDateSubFlowKey finalizes the d/D actuator sub-flow. The picker's
// own ParseRelativeDate-backed preview is UI-only: on submit we resolve to a
// concrete date and emit that literal YYYY-MM-DD as the actuator arg, never
// the shorthand -- resolveDeferDate/parseSchedDate (today.go) accept that
// literal directly, sidestepping any CLI-vs-TUI parser-syntax divergence
// entirely.
func (m *tuiModel) handleDateSubFlowKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.cancelSubFlow()
		return m, nil
	case tea.KeyEnter:
		date, err := m.datePicker.ParsedDate()
		if err != nil || date.IsZero() {
			return m, nil // datePicker already renders its own inline error
		}
		key := "d"
		if m.subFlow == subFlowAgendaDeadline {
			key = "D"
		}
		ref := m.subFlowRef
		m.cancelSubFlow()
		return m, m.actuateCmd(ref, key, date.Format("2006-01-02"))
	}
	var cmd tea.Cmd
	m.datePicker, cmd = m.datePicker.Update(msg)
	return m, cmd
}

// handlePrioritySubFlowKey finalizes the p actuator sub-flow: validation
// happens here (before actuateCmd/actPriority) so an invalid letter never
// even reaches the verb call.
func (m *tuiModel) handlePrioritySubFlowKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.cancelSubFlow()
		return m, nil
	case tea.KeyEnter:
		val := strings.ToUpper(strings.TrimSpace(m.textEntry.GetValue()))
		ref := m.subFlowRef
		m.cancelSubFlow()
		if val != "A" && val != "B" && val != "C" {
			m.lastErr = fmt.Errorf("today act: priority: invalid value %q (want A, B, or C)", val)
			return m, nil
		}
		return m, m.actuateCmd(ref, "p", val)
	}
	var cmd tea.Cmd
	m.textEntry, cmd = m.textEntry.Update(msg)
	return m, cmd
}

// ─────────────────────────────────────────────────────────────────────────────
// Creation sub-flows: todos pane "n" (add todo), log pane "n" (add log),
// notes pane "n" in browse mode (create note). Each finalizes the same way
// the priority sub-flow does: Esc cancels with no verb call; Enter reads
// m.textEntry's value, cancels the sub-flow, and (if the trimmed value is
// non-empty) dispatches the pane's mutation cmd. An empty submission is a
// silent no-op, matching the CLI's own "empty body text" rejection for
// `rk todo add`/`rk add` without duplicating its exact error text.
// ─────────────────────────────────────────────────────────────────────────────

// handleAddTodoSubFlowKey finalizes the todos pane's "n" sub-flow.
func (m *tuiModel) handleAddTodoSubFlowKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.cancelSubFlow()
		return m, nil
	case tea.KeyEnter:
		body := strings.TrimSpace(m.textEntry.GetValue())
		m.cancelSubFlow()
		if body == "" {
			return m, nil
		}
		return m, m.addTodoCmd(body)
	}
	var cmd tea.Cmd
	m.textEntry, cmd = m.textEntry.Update(msg)
	return m, cmd
}

// handleAddLogSubFlowKey finalizes the log pane's "n" sub-flow.
func (m *tuiModel) handleAddLogSubFlowKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.cancelSubFlow()
		return m, nil
	case tea.KeyEnter:
		body := strings.TrimSpace(m.textEntry.GetValue())
		m.cancelSubFlow()
		if body == "" {
			return m, nil
		}
		return m, m.addLogCmd(body)
	}
	var cmd tea.Cmd
	m.textEntry, cmd = m.textEntry.Update(msg)
	return m, cmd
}

// handleNewNoteSubFlowKey finalizes the notes pane's "n" sub-flow.
func (m *tuiModel) handleNewNoteSubFlowKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.cancelSubFlow()
		return m, nil
	case tea.KeyEnter:
		title := strings.TrimSpace(m.textEntry.GetValue())
		m.cancelSubFlow()
		if title == "" {
			return m, nil
		}
		return m, m.createNoteCmd(title)
	}
	var cmd tea.Cmd
	m.textEntry, cmd = m.textEntry.Update(msg)
	return m, cmd
}

// ─────────────────────────────────────────────────────────────────────────────
// Creation mutation cmds: mirror actuateCmd's shape (verb call -> Reconcile
// -> mutationDoneMsg), each capturing only plain values, never a pointer
// into pane state (async-closure-capture pitfall, docs/REVIEW_PATTERNS.md).
// ─────────────────────────────────────────────────────────────────────────────

// addTodoCmd calls addDurableTodo (the same verb `rk todo add` calls) with
// only a body -- no scheduled/deadline/depends/repeat, v1's minimal add
// flow -- and reconciles the index on success.
func (m *tuiModel) addTodoCmd(body string) tea.Cmd {
	vaultDir := m.vaultDir
	ix := m.ix
	author := resolveAuthor("")
	return func() tea.Msg {
		todosDir := filepath.Join(vaultDir, "todos")
		if err := os.MkdirAll(todosDir, 0o755); err != nil {
			return errMsg{err: fmt.Errorf("tui: add todo: create todos dir: %w", err)}
		}
		if _, err := addDurableTodo(todosDir, author, body, "", "", "", ""); err != nil {
			return errMsg{err: err}
		}
		if _, err := ix.Reconcile(); err != nil {
			return errMsg{err: err}
		}
		return mutationDoneMsg{kind: "todos"}
	}
}

// addLogCmd calls appendLogEntry (the same verb `rk add` calls) for the
// current day/time and reconciles the index on success.
func (m *tuiModel) addLogCmd(body string) tea.Cmd {
	vaultDir := m.vaultDir
	ix := m.ix
	author := resolveAuthor("")
	return func() tea.Msg {
		logDir := filepath.Join(vaultDir, "log")
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			return errMsg{err: fmt.Errorf("tui: add log: create log dir: %w", err)}
		}
		day := time.Now().UTC().Format("2006-01-02")
		hhmm := time.Now().UTC().Format("15:04")
		if _, err := appendLogEntry(logDir, day, hhmm, author, body); err != nil {
			return errMsg{err: err}
		}
		if _, err := ix.Reconcile(); err != nil {
			return errMsg{err: err}
		}
		return mutationDoneMsg{kind: "log"}
	}
}

// createNoteCmd calls createNote (the same verb `rk note create` calls) with
// only a title -- the slug is self-minted from it via slugify, matching
// createNote's own aliasing convention, and Body stays empty (v1's minimal
// create flow; TextEntryBar is single-line) -- and reconciles the index on
// success.
func (m *tuiModel) createNoteCmd(title string) tea.Cmd {
	vaultDir := m.vaultDir
	ix := m.ix
	author := resolveAuthor("")
	return func() tea.Msg {
		notesDir := filepath.Join(vaultDir, "notes")
		slug := slugify(title)
		if err := validateSlug(slug); err != nil {
			return errMsg{err: fmt.Errorf("tui: create note: %w", err)}
		}
		if _, err := createNote(notesDir, noteCreateParams{
			Title:  title,
			Slug:   slug,
			Type:   "note",
			Author: author,
		}); err != nil {
			return errMsg{err: err}
		}
		if _, err := ix.Reconcile(); err != nil {
			return errMsg{err: err}
		}
		return mutationDoneMsg{kind: "notes"}
	}
}
