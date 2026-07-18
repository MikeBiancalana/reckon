// Package cli — tests for the persistent 4-pane `rk tui` model (tui.go,
// tui_model.go, tui_layout.go, tui_keyboard.go, tui_panes.go, tui_read.go).
//
// These drive the model/pane/keyboard layer directly via typed
// tea.Msg/tea.Cmd values and direct calls to tuiModel.handleKey /
// tuiModel.Update, mirroring this package's existing convention of testing
// Bubble Tea components through direct Update() calls rather than a
// pty/teatest harness. Read-side tests build a real vault + *index.Index and
// call the read helpers/verb functions directly; write-flow tests call the
// same verb functions (addDurableTodo, dispatchTodayAct, appendLogEntry,
// createNote) the keyboard layer is meant to invoke, then assert the model
// picks the mutation up through its own typed message pipeline — the
// keyboard layer itself has no concrete text-entry-sub-flow keybinding yet,
// so those two tests (add-todo, add-log) exercise the verb call plus the
// model's reload path rather than a specific, still-undefined keypress
// sequence. Agenda actuator keys (t/d/D/p/x/i/c) are not ambiguous — they are
// the same single-letter vocabulary `rk today act` already uses — so agenda
// tests drive tuiModel.handleKey with those literal keys.
//
// Harness reuse (already defined elsewhere in this package, not redefined
// here): setupQueryVault, writeTestNode, resetCLIFlags, writeTodoFixture,
// writeRecurringTodo, pinTodoNow, dayOffset, mustWriteFile, mustReadFile,
// containsID, writeEphemeralContainer, checklistLine, dayLogPath,
// parseLogDayFile, utcToday, snapshotVaultFiles, vaultSnapshotsEqual,
// runTodo, mustDecodeJSON.
package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/index"
	"github.com/MikeBiancalana/reckon/internal/node"
	"github.com/MikeBiancalana/reckon/internal/tui/components"
	tea "github.com/charmbracelet/bubbletea"
)

// ─────────────────────────────────────────────────────────────────────────────
// Harness helpers
// ─────────────────────────────────────────────────────────────────────────────

// newTUITestModel opens a real *index.Index over vault (reconciled once) and
// builds a *tuiModel over it, registering index cleanup. Callers needing the
// index directly (to call read helpers or verb functions) get it back too.
func newTUITestModel(t *testing.T, vault string) (*tuiModel, *index.Index) {
	t.Helper()
	cfg, err := config.LoadWithOverrides(vault, "")
	if err != nil {
		t.Fatalf("config.LoadWithOverrides: %v", err)
	}
	ix, err := index.Open(cfg)
	if err != nil {
		t.Fatalf("index.Open: %v", err)
	}
	t.Cleanup(func() { ix.Close() })
	if _, err := ix.Reconcile(); err != nil {
		t.Fatalf("index.Reconcile: %v", err)
	}
	return newTUIModel(ix, cfg), ix
}

// drainTUICmd runs cmd (and, recursively, every sub-command of any
// tea.BatchMsg it returns) to completion, collecting every resulting
// tea.Msg in execution order. A nil cmd or a cmd returning a nil msg yields
// no messages.
func drainTUICmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if msg == nil {
		return nil
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, drainTUICmd(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

// applyTUIMsg sends msg through m.Update, type-asserts the result back to
// *tuiModel (failing the test if Update ever returns some other tea.Model),
// and recursively applies any follow-up messages produced by the returned
// tea.Cmd.
func applyTUIMsg(t *testing.T, m *tuiModel, msg tea.Msg) *tuiModel {
	t.Helper()
	newModel, cmd := m.Update(msg)
	tm, ok := newModel.(*tuiModel)
	if !ok {
		t.Fatalf("Update(%T) returned a %T, want *tuiModel", msg, newModel)
	}
	for _, follow := range drainTUICmd(cmd) {
		tm = applyTUIMsg(t, tm, follow)
	}
	return tm
}

// containsTodoText reports whether items has a durable item whose Title, or
// an ephemeral item whose Body, equals text.
func containsTodoText(items []todoListItem, text string) bool {
	for _, it := range items {
		if it.Title == text || it.Body == text {
			return true
		}
	}
	return false
}

// findAgendaItem returns the first item in items with the given ID.
func findAgendaItem(items []agendaItem, id string) (agendaItem, bool) {
	for _, it := range items {
		if it.ID == id {
			return it, true
		}
	}
	return agendaItem{}, false
}

func agendaContainsID(items []agendaItem, id string) bool {
	_, ok := findAgendaItem(items, id)
	return ok
}

// agendaIndexOf returns id's index within items, failing the test if absent.
func agendaIndexOf(t *testing.T, items []agendaItem, id string) int {
	t.Helper()
	for i, it := range items {
		if it.ID == id {
			return i
		}
	}
	t.Fatalf("agendaIndexOf: %s not found in %+v", id, items)
	return -1
}

// ─────────────────────────────────────────────────────────────────────────────
// Reads: todos pane (scenario 1)
// ─────────────────────────────────────────────────────────────────────────────

// TestTodosPaneLoad: an open durable todo, a done durable todo, and an
// unchecked ephemeral item all exist; once the model applies a
// todosLoadedMsg built from the same read helpers `rk todo list` uses, the
// pane's items must include the open durable item (by its derived subject
// title) and the ephemeral item, and must exclude the done item — the same
// filtering `rk todo list`'s default (open-only) view applies.
func TestTodosPaneLoad(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	openID := node.Mint()
	writeTodoFixture(t, vault, openID, "open", "", "Write the quarterly report.\n\nExtra body detail that must not appear as the pane's display text.")
	doneID := node.Mint()
	writeTodoFixture(t, vault, doneID, "done", "", "Already finished.")
	writeEphemeralContainer(t, vault, node.Mint(), checklistLine(false, "Buy milk"))

	m, ix := newTUITestModel(t, vault)

	if cmd := m.Init(); cmd == nil {
		t.Errorf("tuiModel.Init() returned a nil tea.Cmd, want a batch of the panes' initial load cmds")
	}

	durItems, err := listDurableTodos(ix.DB(), false, "")
	if err != nil {
		t.Fatalf("listDurableTodos: %v", err)
	}
	ephItems, err := listEphemeralTodos(ix.DB(), false)
	if err != nil {
		t.Fatalf("listEphemeralTodos: %v", err)
	}
	if !containsTodoText(durItems, "Write the quarterly report.") {
		t.Fatalf("precondition: listDurableTodos missing the open todo: %+v", durItems)
	}
	if containsID(durItems, doneID) {
		t.Fatalf("precondition: listDurableTodos (open-only) unexpectedly includes the done todo")
	}
	all := append(append([]todoListItem{}, durItems...), ephItems...)

	m2 := applyTUIMsg(t, m, todosLoadedMsg{items: all})

	if !containsTodoText(m2.todos.items, "Write the quarterly report.") {
		t.Errorf("todos pane after todosLoadedMsg: items = %+v, want the open durable todo's subject title included", m2.todos.items)
	}
	if !containsTodoText(m2.todos.items, "Buy milk") {
		t.Errorf("todos pane after todosLoadedMsg: items = %+v, want the ephemeral item included", m2.todos.items)
	}
	if containsID(m2.todos.items, doneID) {
		t.Errorf("todos pane after todosLoadedMsg unexpectedly includes the done (non-open) todo")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Reads: log pane (scenario 2)
// ─────────────────────────────────────────────────────────────────────────────

// TestLoadLogEntries: a day file with 3 entries indexes as 3 distinct
// log-entry nodes; loadLogEntries must return exactly those 3 rows (not the
// raw day-file body as one blob), each carrying non-empty content.
func TestLoadLogEntries(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	logDir := filepath.Join(vault, "log")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}
	day := "2026-07-10"

	var wantIDs []string
	for _, hhmm := range []string{"08:00", "09:30", "17:45"} {
		res, err := appendLogEntry(logDir, day, hhmm, "tester", "did something at "+hhmm)
		if err != nil {
			t.Fatalf("appendLogEntry(%s): %v", hhmm, err)
		}
		wantIDs = append(wantIDs, res.ID)
	}

	_, ix := newTUITestModel(t, vault)

	entries, err := loadLogEntries(ix.DB())
	if err != nil {
		t.Fatalf("loadLogEntries: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("loadLogEntries: got %d entries, want 3 (one per ## block in the day file): %+v", len(entries), entries)
	}
	got := map[string]bool{}
	for _, e := range entries {
		got[e.ID] = true
		if e.Content == "" {
			t.Errorf("entry %s: empty Content, want the entry's body text", e.ID)
		}
	}
	for _, id := range wantIDs {
		if !got[id] {
			t.Errorf("loadLogEntries missing entry %s: got ids %v", id, got)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Reads: notes pane (scenario 3)
// ─────────────────────────────────────────────────────────────────────────────

// TestLoadNotesPaneLinks: a note with 2 outgoing wikilinks and 1 backlink
// must round-trip through loadNotesPaneLinks with matching counts, and the
// backlink's source note must be resolved (a nil SourceNote panics the
// rendering component that consumes this data).
func TestLoadNotesPaneLinks(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	notesDir := filepath.Join(vault, "notes")
	mainRes, err := createNote(notesDir, noteCreateParams{
		Title: "Main Note", Slug: "main-note", Type: "note", Author: "tester",
		Body: "See [[other-one]] and [[other-two]].\n",
	})
	if err != nil {
		t.Fatalf("createNote(main): %v", err)
	}
	if _, err := createNote(notesDir, noteCreateParams{Title: "Other One", Slug: "other-one", Type: "note", Author: "tester"}); err != nil {
		t.Fatalf("createNote(other-one): %v", err)
	}
	if _, err := createNote(notesDir, noteCreateParams{Title: "Other Two", Slug: "other-two", Type: "note", Author: "tester"}); err != nil {
		t.Fatalf("createNote(other-two): %v", err)
	}
	if _, err := createNote(notesDir, noteCreateParams{
		Title: "Linker Note", Slug: "linker-note", Type: "note", Author: "tester",
		Body: "Back to [[main-note]].\n",
	}); err != nil {
		t.Fatalf("createNote(linker): %v", err)
	}

	_, ix := newTUITestModel(t, vault)

	outgoing, backlinks, err := loadNotesPaneLinks(ix.DB(), mainRes.ID)
	if err != nil {
		t.Fatalf("loadNotesPaneLinks: %v", err)
	}
	if len(outgoing) != 2 {
		t.Errorf("loadNotesPaneLinks(main): outgoing = %+v, want 2", outgoing)
	}
	if len(backlinks) != 1 {
		t.Fatalf("loadNotesPaneLinks(main): backlinks = %+v, want 1", backlinks)
	}
	bl := backlinks[0]
	if bl.NoteLink.SourceNote == nil {
		t.Fatalf("backlink missing a resolved SourceNote (a nil pointer here panics the rendering component)")
	}
	if bl.NoteLink.SourceNote.Slug != "linker-note" {
		t.Errorf("backlink SourceNote.Slug = %q, want %q", bl.NoteLink.SourceNote.Slug, "linker-note")
	}
}

// TestNotesComposition: selecting a note in the browse (picker) view must
// switch the composite notes pane into inspect mode and load that note's
// link graph into the notes-pane component.
func TestNotesComposition(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	notesDir := filepath.Join(vault, "notes")
	mainRes, err := createNote(notesDir, noteCreateParams{
		Title: "Main Note", Slug: "main-note", Type: "note", Author: "tester",
		Body: "See [[other-one]].\n",
	})
	if err != nil {
		t.Fatalf("createNote(main): %v", err)
	}
	if _, err := createNote(notesDir, noteCreateParams{Title: "Other One", Slug: "other-one", Type: "note", Author: "tester"}); err != nil {
		t.Fatalf("createNote(other-one): %v", err)
	}

	m, _ := newTUITestModel(t, vault)
	if m.notes.mode != notesShowBrowse {
		t.Fatalf("precondition: notes pane must start in browse mode, got %v", m.notes.mode)
	}

	m2 := applyTUIMsg(t, m, components.NotePickerSelectMsg{NoteSlug: "main-note"})

	if m2.notes.mode != notesShowInspect {
		t.Errorf("notes pane mode after selecting a note = %v, want inspect mode", m2.notes.mode)
	}
	view := m2.notes.links.View()
	if !strings.Contains(view, "Linked Notes") {
		t.Errorf("notes pane (inspect) View() = %q, want the loaded link graph for %s rendered, not the pre-selection placeholder", view, mainRes.ID)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Reads: agenda pane (scenario 4)
// ─────────────────────────────────────────────────────────────────────────────

// TestAgendaPaneLoad: a native todo scheduled today and an external
// work-ticket with deadline today must both appear in the agenda pane after
// load, with only the work-ticket row flagged read-only.
func TestAgendaPaneLoad(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-10")
	today := "2026-07-10"

	nativeID := node.Mint()
	writeTodoFixture(t, vault, nativeID, "open", today, "Ship the report.")

	extID := node.Mint()
	writeTestNode(t, vault, "work/"+extID+".md", extID, "work-ticket", "External ticket.",
		"state: open", "deadline: "+today, "source: jira", "source-url: https://example.com/T-1")

	m, ix := newTUITestModel(t, vault)

	items, warnings, err := buildAgenda(ix.DB(), today)
	if err != nil {
		t.Fatalf("buildAgenda: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("precondition: unexpected buildAgenda warnings: %v", warnings)
	}
	if len(items) != 2 {
		t.Fatalf("precondition: buildAgenda returned %d items, want 2: %+v", len(items), items)
	}

	m2 := applyTUIMsg(t, m, agendaLoadedMsg{items: items, warnings: warnings})

	native, ok := findAgendaItem(m2.agenda.items, nativeID)
	if !ok {
		t.Fatalf("agenda pane after load: native todo %s missing: %+v", nativeID, m2.agenda.items)
	}
	if native.ReadOnly {
		t.Errorf("native todo %s must not be flagged ReadOnly", nativeID)
	}
	ext, ok := findAgendaItem(m2.agenda.items, extID)
	if !ok {
		t.Fatalf("agenda pane after load: external work-ticket %s missing: %+v", extID, m2.agenda.items)
	}
	if !ext.ReadOnly {
		t.Errorf("external work-ticket %s must be flagged ReadOnly", extID)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Writes: add todo (scenario 5)
// ─────────────────────────────────────────────────────────────────────────────

// TestAddTodoFlow: submitting "buy milk" must call addDurableTodo (a real
// vault file appears with state: open), and after the resulting index
// reconcile, the model's next todos-pane render must include it.
func TestAddTodoFlow(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	m, ix := newTUITestModel(t, vault)

	if err := os.MkdirAll(filepath.Join(vault, "todos"), 0o755); err != nil {
		t.Fatalf("mkdir todos dir: %v", err)
	}
	addRes, err := addDurableTodo(filepath.Join(vault, "todos"), "tester", "buy milk", "", "", "", "")
	if err != nil {
		t.Fatalf("addDurableTodo: %v", err)
	}
	path := filepath.Join(vault, "todos", addRes.ID+".md")
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("addDurableTodo did not create %s: %v", path, statErr)
	}
	if !strings.Contains(mustReadFile(t, path), "state: open") {
		t.Errorf("new todo file missing state: open")
	}

	if _, err := ix.Reconcile(); err != nil {
		t.Fatalf("index.Reconcile after addDurableTodo: %v", err)
	}
	durItems, err := listDurableTodos(ix.DB(), false, "")
	if err != nil {
		t.Fatalf("listDurableTodos: %v", err)
	}
	if !containsID(durItems, addRes.ID) {
		t.Fatalf("precondition: listDurableTodos after reconcile does not include the new todo %s: %+v", addRes.ID, durItems)
	}

	m2 := applyTUIMsg(t, m, todosLoadedMsg{items: durItems})
	if !containsID(m2.todos.items, addRes.ID) {
		t.Errorf("todos pane after reconcile+reload: items = %+v, want the new todo %s included", m2.todos.items, addRes.ID)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Writes: agenda actuator (scenario 6)
// ─────────────────────────────────────────────────────────────────────────────

// TestAgendaActDone: pressing 'x' on a selected, non-recurring native agenda
// row must run completeDurableTodoNode's plain path and flip state->done in
// the file.
func TestAgendaActDone(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-10")
	today := "2026-07-10"

	id := node.Mint()
	path, src := writeTodoFixture(t, vault, id, "open", today, "Ship the report.")

	m, ix := newTUITestModel(t, vault)
	items, _, err := buildAgenda(ix.DB(), today)
	if err != nil {
		t.Fatalf("buildAgenda: %v", err)
	}
	m.agenda.items = items
	m.agenda.selected = agendaIndexOf(t, items, id)

	newModel, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m2, ok := newModel.(*tuiModel)
	if !ok {
		t.Fatalf("handleKey(x) returned a %T, want *tuiModel", newModel)
	}
	for _, follow := range drainTUICmd(cmd) {
		m2 = applyTUIMsg(t, m2, follow)
	}
	_ = m2

	got := mustReadFile(t, path)
	want := strings.Replace(src, "state: open", "state: done", 1)
	if got != want {
		t.Errorf("agenda 'x' on a native todo did not flip state->done\n--- want ---\n%q\n--- got (unchanged) ---\n%q", want, got)
	}
}

// TestAgendaActDoneRecurring: pressing 'x' on a repeat: agenda row must take
// the recurrence branch — the scheduled cursor advances and state stays
// open, it must not force state->done the way a plain completion would.
func TestAgendaActDoneRecurring(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-05")

	id := node.Mint()
	path, src := writeRecurringTodo(t, vault, id, "2026-07-05", "+7d", "Water plants")

	m, ix := newTUITestModel(t, vault)
	items, _, err := buildAgenda(ix.DB(), "2026-07-05")
	if err != nil {
		t.Fatalf("buildAgenda: %v", err)
	}
	m.agenda.items = items
	m.agenda.selected = agendaIndexOf(t, items, id)

	newModel, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m2, ok := newModel.(*tuiModel)
	if !ok {
		t.Fatalf("handleKey(x) returned a %T, want *tuiModel", newModel)
	}
	for _, follow := range drainTUICmd(cmd) {
		m2 = applyTUIMsg(t, m2, follow)
	}

	got := mustReadFile(t, path)
	if got == src {
		t.Errorf("agenda 'x' on a repeat: todo did not advance the scheduled cursor; file unchanged:\n%q", got)
	}
	if !strings.Contains(got, "state: open") {
		t.Errorf("recurring todo's state must stay open after completion, got:\n%q", got)
	}
	if idx := m2.agenda.selected; idx >= 0 && idx < len(m2.agenda.items) {
		if m2.agenda.items[idx].State != "open" {
			t.Errorf("agenda pane's own reloaded row must show state=open for a recurring completion, got %+v", m2.agenda.items[idx])
		}
	}
}

// TestAgendaActDoneEmitsLogEntry: completing an agenda row via 'x' with no
// explicit toggle must default to logging on — a did-linked log entry
// appears in today's day file, the same default `rk today act x` uses.
func TestAgendaActDoneEmitsLogEntry(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-10")
	today := "2026-07-10"

	id := node.Mint()
	writeTodoFixture(t, vault, id, "open", today, "Ship the report.")

	m, ix := newTUITestModel(t, vault)
	items, _, err := buildAgenda(ix.DB(), today)
	if err != nil {
		t.Fatalf("buildAgenda: %v", err)
	}
	m.agenda.items = items
	m.agenda.selected = agendaIndexOf(t, items, id)

	newModel, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m2, ok := newModel.(*tuiModel)
	if !ok {
		t.Fatalf("handleKey(x) returned a %T, want *tuiModel", newModel)
	}
	for _, follow := range drainTUICmd(cmd) {
		m2 = applyTUIMsg(t, m2, follow)
	}
	_ = m2

	if _, statErr := os.Stat(dayLogPath(vault, today)); statErr != nil {
		t.Fatalf("agenda 'x' (default logging on) did not create a did-linked log entry: log day file missing: %v", statErr)
	}
	entries := parseLogDayFile(t, vault, today)[1:]
	if len(entries) != 1 {
		t.Fatalf("want exactly 1 log entry from the default-logging completion, got %d", len(entries))
	}
	var foundDid bool
	for _, l := range entries[0].Links {
		if l.Rel == "did" && l.To == id {
			foundDid = true
		}
	}
	if !foundDid {
		t.Errorf("log entry missing Link{Rel:did, To:%s}: %+v", id, entries[0].Links)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Writes: add log entry (scenario 7)
// ─────────────────────────────────────────────────────────────────────────────

// TestAddLogFlow: submitting a log entry must call appendLogEntry and write
// the same entry block bytes the plain capture path produces, and the log
// pane must reflect the new entry once the model applies the load result.
func TestAddLogFlow(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	m, _ := newTUITestModel(t, vault)

	logDir := filepath.Join(vault, "log")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}
	day := utcToday()
	res, err := appendLogEntry(logDir, day, "09:15", "tester", "wrote the tui tests")
	if err != nil {
		t.Fatalf("appendLogEntry: %v", err)
	}

	wantBlock := node.RenderLogEntry("09:15", "tester", res.ID, "wrote the tui tests")
	got := mustReadFile(t, dayLogPath(vault, day))
	if !strings.Contains(got, wantBlock) {
		t.Fatalf("appendLogEntry did not write the expected entry block\n--- want substring ---\n%q\n--- got file ---\n%q", wantBlock, got)
	}

	entry := components.LogEntryRow{ID: res.ID, Timestamp: time.Now().UTC(), Content: "wrote the tui tests", EntryType: "note"}
	m2 := applyTUIMsg(t, m, logLoadedMsg{entries: []components.LogEntryRow{entry}})

	selected := m2.log.view.SelectedLogEntry()
	if selected == nil || selected.ID != res.ID {
		t.Errorf("log pane after logLoadedMsg: SelectedLogEntry() = %+v, want the new entry %s reflected", selected, res.ID)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Writes: agenda read-only guard (scenario 8)
// ─────────────────────────────────────────────────────────────────────────────

// TestAgendaReadOnlyGuard: any actuator key pressed on a selected external
// (work-ticket) agenda row must be rejected with the same read-only error
// `rk today act` gives, and must leave the vault untouched.
func TestAgendaReadOnlyGuard(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-10")
	today := "2026-07-10"

	extID := node.Mint()
	writeTestNode(t, vault, "work/"+extID+".md", extID, "work-ticket", "Fix the external ticket.",
		"state: open", "scheduled: "+today, "source: jira", "source-url: https://example.com/T-9")

	m, ix := newTUITestModel(t, vault)
	items, _, err := buildAgenda(ix.DB(), today)
	if err != nil {
		t.Fatalf("buildAgenda: %v", err)
	}
	m.agenda.items = items
	m.agenda.selected = agendaIndexOf(t, items, extID)
	if !m.agenda.items[m.agenda.selected].ReadOnly {
		t.Fatalf("precondition: work-ticket row must be ReadOnly")
	}

	wantMsg := "is read-only (external work ticket); use rk today open instead"
	for _, key := range []string{"x", "t", "i", "c"} {
		before := snapshotVaultFiles(t, vault)

		newModel, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		m2, ok := newModel.(*tuiModel)
		if !ok {
			t.Fatalf("handleKey(%q) returned a %T, want *tuiModel", key, newModel)
		}
		for _, follow := range drainTUICmd(cmd) {
			m2 = applyTUIMsg(t, m2, follow)
		}

		if m2.lastErr == nil || !strings.Contains(m2.lastErr.Error(), wantMsg) {
			t.Errorf("handleKey(%q) on a read-only row: lastErr = %v, want an error containing %q", key, m2.lastErr, wantMsg)
		}
		after := snapshotVaultFiles(t, vault)
		if !vaultSnapshotsEqual(before, after) {
			t.Errorf("handleKey(%q) on a read-only row mutated a vault file", key)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Index durability (scenarios 9, 10)
// ─────────────────────────────────────────────────────────────────────────────

// TestDurabilityAfterRebuild: a create+complete+log sequence (the same verbs
// the model's write flows call) must survive a full index rebuild with no
// warnings, and the rebuilt index must show the completed state plus both
// log entries through the model's own log read helper.
func TestDurabilityAfterRebuild(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-10")
	today := "2026-07-10"

	if err := os.MkdirAll(filepath.Join(vault, "todos"), 0o755); err != nil {
		t.Fatalf("mkdir todos dir: %v", err)
	}
	addRes, err := addDurableTodo(filepath.Join(vault, "todos"), "tester", "Ship the report.", today, "", "", "")
	if err != nil {
		t.Fatalf("addDurableTodo: %v", err)
	}

	if _, err := dispatchTodayAct(vault, addRes.ID, "x", "", false); err != nil {
		t.Fatalf("dispatchTodayAct x: %v", err)
	}

	logDir := filepath.Join(vault, "log")
	if _, err := appendLogEntry(logDir, today, "16:00", "tester", "wrapped up for the day"); err != nil {
		t.Fatalf("appendLogEntry: %v", err)
	}

	cfg, err := config.LoadWithOverrides(vault, "")
	if err != nil {
		t.Fatalf("config.LoadWithOverrides: %v", err)
	}
	ix, err := index.Open(cfg)
	if err != nil {
		t.Fatalf("index.Open: %v", err)
	}
	t.Cleanup(func() { ix.Close() })

	stats, err := ix.Rebuild()
	if err != nil {
		t.Fatalf("index.Rebuild: %v", err)
	}
	if len(stats.Warnings) != 0 {
		t.Errorf("rebuild produced warnings attributable to these writes: %+v", stats.Warnings)
	}

	var state string
	if err := ix.DB().QueryRow("SELECT value FROM node_props WHERE id = ? AND key = 'state'", addRes.ID).Scan(&state); err != nil {
		t.Fatalf("query rebuilt state: %v", err)
	}
	if state != "done" {
		t.Errorf("rebuilt index state = %q, want %q", state, "done")
	}

	entries, err := loadLogEntries(ix.DB())
	if err != nil {
		t.Fatalf("loadLogEntries: %v", err)
	}
	if len(entries) < 2 {
		t.Errorf("loadLogEntries after rebuild: got %d entries, want at least 2 (the did-entry from completing %s, plus the independently-appended entry)", len(entries), addRes.ID)
	}
}

// TestMutationVisibleToFreshCLI: a todo created via addDurableTodo must be a
// real vault file change, visible to a separate `rk todo list --json`
// invocation — not state that only exists inside a running TUI process.
func TestMutationVisibleToFreshCLI(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	if err := os.MkdirAll(filepath.Join(vault, "todos"), 0o755); err != nil {
		t.Fatalf("mkdir todos dir: %v", err)
	}
	addRes, err := addDurableTodo(filepath.Join(vault, "todos"), "tester", "Buy milk", "", "", "", "")
	if err != nil {
		t.Fatalf("addDurableTodo: %v", err)
	}

	stdout, stderr, err := runTodo(t, vault, "list", "--json")
	if err != nil {
		t.Fatalf("rk todo list --json: %v\nstderr: %s", err, stderr)
	}
	var res todoListResult
	mustDecodeJSON(t, stdout, &res)
	if !containsID(res.Items, addRes.ID) {
		t.Errorf("rk todo list --json does not list the vault-created todo %s: %+v", addRes.ID, res.Items)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Layout (scenario 12)
// ─────────────────────────────────────────────────────────────────────────────

// TestResizeClampsDimensions: a WindowSizeMsg must update the model's own
// width/height, propagate positive, non-negative dimensions to all 4 panes,
// and calcPaneDims itself must clamp negative input to 0 rather than
// propagating negative pane dimensions.
func TestResizeClampsDimensions(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	m, _ := newTUITestModel(t, vault)

	m.handleWindowSize(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.handleWindowSize(tea.WindowSizeMsg{Width: 80, Height: 24})

	if m.width != 80 || m.height != 24 {
		t.Errorf("model dims after WindowSizeMsg{80,24} = (%d,%d), want (80,24)", m.width, m.height)
	}

	dims := calcPaneDims(80, 24)
	for name, wh := range map[string][2]int{
		"agenda": {dims.agendaWidth, dims.agendaHeight},
		"todos":  {dims.todosWidth, dims.todosHeight},
		"log":    {dims.logWidth, dims.logHeight},
		"notes":  {dims.notesWidth, dims.notesHeight},
	} {
		if wh[0] < 0 || wh[1] < 0 {
			t.Errorf("calcPaneDims(80,24).%s = %v, has a negative dimension", name, wh)
		}
		if wh[0] == 0 || wh[1] == 0 {
			t.Errorf("calcPaneDims(80,24).%s = %v, want a positive width/height for a normal-sized terminal", name, wh)
		}
	}

	clamped := calcPaneDims(-10, -5)
	for name, wh := range map[string][2]int{
		"agenda": {clamped.agendaWidth, clamped.agendaHeight},
		"todos":  {clamped.todosWidth, clamped.todosHeight},
		"log":    {clamped.logWidth, clamped.logHeight},
		"notes":  {clamped.notesWidth, clamped.notesHeight},
	} {
		if wh[0] < 0 || wh[1] < 0 {
			t.Errorf("calcPaneDims(-10,-5).%s = %v, must clamp negative dims to 0", name, wh)
		}
	}

	if m.agenda.width <= 0 || m.agenda.height <= 0 {
		t.Errorf("agenda pane SetSize not propagated by handleWindowSize: width=%d height=%d", m.agenda.width, m.agenda.height)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Keyboard (scenario 13)
// ─────────────────────────────────────────────────────────────────────────────

// TestFocusCycle: Tab from the todos pane must advance focus to the next of
// the 4 fixed panes in a stable order, and 4 successive Tabs from the first
// pane must return focus to the first pane.
func TestFocusCycle(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	m, _ := newTUITestModel(t, vault)

	m.focus = focusTodos
	newModel, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	m2, ok := newModel.(*tuiModel)
	if !ok {
		t.Fatalf("handleKey(Tab) returned a %T, want *tuiModel", newModel)
	}
	if m2.focus != focusLog {
		t.Errorf("focus after Tab from focusTodos = %v, want focusLog", m2.focus)
	}

	m3, _ := newTUITestModel(t, vault)
	m3.focus = focusAgenda
	cur := tea.Model(m3)
	for i := 0; i < 4; i++ {
		var nm tea.Model
		cm, ok := cur.(*tuiModel)
		if !ok {
			t.Fatalf("iteration %d: cur is a %T, want *tuiModel", i, cur)
		}
		nm, _ = cm.handleKey(tea.KeyMsg{Type: tea.KeyTab})
		cur = nm
	}
	final, ok := cur.(*tuiModel)
	if !ok {
		t.Fatalf("handleKey(Tab) returned a %T, want *tuiModel", cur)
	}
	if final.focus != focusAgenda {
		t.Errorf("after 4 Tabs starting at focusAgenda, focus = %v, want focusAgenda (full cycle back to start)", final.focus)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Edge cases
// ─────────────────────────────────────────────────────────────────────────────

// TestEmptyVaultFriendlyEmptyStates: an empty vault must leave every pane in
// a ready-to-render "loaded, zero items" state (a non-nil empty slice, or an
// explicitly shown-but-empty picker) rather than the pane's pre-load zero
// value, so a renderer can tell "never loaded" apart from "loaded, nothing
// here."
func TestEmptyVaultFriendlyEmptyStates(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	m, ix := newTUITestModel(t, vault)

	durItems, err := listDurableTodos(ix.DB(), false, "")
	if err != nil {
		t.Fatalf("listDurableTodos: %v", err)
	}
	ephItems, err := listEphemeralTodos(ix.DB(), false)
	if err != nil {
		t.Fatalf("listEphemeralTodos: %v", err)
	}
	all := append(append([]todoListItem{}, durItems...), ephItems...)
	m2 := applyTUIMsg(t, m, todosLoadedMsg{items: all})
	if m2.todos.items == nil {
		t.Errorf("todos pane on an empty vault: items is nil after todosLoadedMsg, want a non-nil empty slice")
	}

	items, warnings, err := buildAgenda(ix.DB(), "2026-07-10")
	if err != nil {
		t.Fatalf("buildAgenda: %v", err)
	}
	m3 := applyTUIMsg(t, m2, agendaLoadedMsg{items: items, warnings: warnings})
	if m3.agenda.items == nil {
		t.Errorf("agenda pane on an empty vault: items is nil after agendaLoadedMsg, want a non-nil empty slice")
	}

	notes, err := listNotes(ix.DB())
	if err != nil {
		t.Fatalf("listNotes: %v", err)
	}
	m4 := applyTUIMsg(t, m3, notesListLoadedMsg{notes: notes})
	if !m4.notes.picker.IsVisible() {
		t.Errorf("notes pane on an empty vault: picker not shown after notesListLoadedMsg, want an empty-but-shown filterable list")
	}
}

// TestAgendaPaneLoad_SkipsMalformedDateWithWarning: a row with an
// unparseable scheduled date must be skipped (with a warning), while a
// sibling valid row still loads into the pane — one bad row must not sink
// the whole agenda.
func TestAgendaPaneLoad_SkipsMalformedDateWithWarning(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	pinTodoNow(t, "2026-07-10")
	today := "2026-07-10"
	yesterday := dayOffset(t, today, -1)

	badID := node.Mint()
	writeTestNode(t, vault, "todos/"+badID+".md", badID, "todo", "Malformed date todo.",
		"state: open", "scheduled: not-a-date")
	goodID := node.Mint()
	writeTodoFixture(t, vault, goodID, "open", yesterday, "Valid overdue todo.")

	m, ix := newTUITestModel(t, vault)
	items, warnings, err := buildAgenda(ix.DB(), today)
	if err != nil {
		t.Fatalf("buildAgenda: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatalf("precondition: buildAgenda produced no warning for the malformed scheduled date")
	}
	if agendaContainsID(items, badID) {
		t.Fatalf("precondition: buildAgenda must skip the malformed-date row")
	}
	if !agendaContainsID(items, goodID) {
		t.Fatalf("precondition: buildAgenda must still surface the sibling valid row")
	}

	m2 := applyTUIMsg(t, m, agendaLoadedMsg{items: items, warnings: warnings})
	if !agendaContainsID(m2.agenda.items, goodID) {
		t.Errorf("agenda pane after load (sibling row had a malformed date) missing the valid row %s: %+v", goodID, m2.agenda.items)
	}
}

// TestLogPaneSetSizeTruncatesLongContent: a very long single-line log entry
// must be truncated to fit the pane's configured width once SetSize has been
// called — it must not expand the rendered line past the pane's bounds.
func TestLogPaneSetSizeTruncatesLongContent(t *testing.T) {
	p := newLogPane()
	long := strings.Repeat("x", 500)
	p.view.UpdateLogEntries([]components.LogEntryRow{
		{ID: "01LONGENTRY", Timestamp: time.Now(), Content: long, EntryType: "note"},
	})

	p.SetSize(40, 10)

	rendered := p.view.View()
	for _, line := range strings.Split(rendered, "\n") {
		if len(line) > 60 {
			t.Fatalf("logPane.SetSize(40, 10) did not propagate width to the underlying log view: rendered line is %d chars wide, want it truncated to fit a 40-wide pane\nline: %q", len(line), line)
		}
	}
}

// TestErrMsgSurfacesCRLFRejection: a verb call refused for CRLF line endings
// must surface onto the model's error state when delivered as an errMsg, not
// be silently dropped or worked around; the offending file must stay
// untouched.
func TestErrMsgSurfacesCRLFRejection(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	logDir := filepath.Join(vault, "log")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}
	day := "2026-07-10"
	crlfPath := dayLogPath(vault, day)
	if err := os.WriteFile(crlfPath, []byte("---\r\nid: 01CRLFDAYFILE\r\ntype: log-day\r\n---\r\n# 2026-07-10\r\n"), 0o644); err != nil {
		t.Fatalf("write CRLF fixture: %v", err)
	}

	_, verbErr := appendLogEntry(logDir, day, "10:00", "tester", "should be refused")
	if verbErr == nil {
		t.Fatalf("precondition: appendLogEntry on a CRLF day file must fail")
	}
	if !strings.Contains(verbErr.Error(), "CRLF") {
		t.Fatalf("precondition: appendLogEntry error = %v, want it to mention CRLF", verbErr)
	}

	m, _ := newTUITestModel(t, vault)
	m2 := applyTUIMsg(t, m, errMsg{err: verbErr})

	if m2.lastErr == nil || !strings.Contains(m2.lastErr.Error(), "CRLF") {
		t.Errorf("Update(errMsg) did not surface the CRLF-refusal error: lastErr = %v", m2.lastErr)
	}
	got := mustReadFile(t, crlfPath)
	if !strings.Contains(got, "\r\n") {
		t.Errorf("CRLF file must remain untouched (refused, not worked around)")
	}
}
