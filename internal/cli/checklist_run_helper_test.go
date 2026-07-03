package cli

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MikeBiancalana/reckon/internal/checklist"
	"github.com/MikeBiancalana/reckon/internal/storage"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupChecklistRunTestService creates a real temp-SQLite *checklist.Service,
// following the same fixture style as checklist.setupChecklistTestService but
// built from this package's public constructors since it lives across the
// package boundary.
func setupChecklistRunTestService(t *testing.T) *checklist.Service {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	db, err := storage.NewDatabase(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	repo := checklist.NewRepository(db)
	return checklist.NewService(repo)
}

// startTestRun creates a template with the given items and starts a run for it.
func startTestRun(t *testing.T, svc *checklist.Service, name string, items []string) *checklist.Run {
	t.Helper()
	_, err := svc.CreateTemplate(name, items)
	require.NoError(t, err)
	run, err := svc.StartRun(name)
	require.NoError(t, err)
	return run
}

// keyRune builds a tea.KeyMsg for a single-rune key press (e.g. "j", "k", "a", "q").
func keyRune(r string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(r)}
}

// asChecklistRunModel type-asserts the result of Update, following the
// bare-assertion-guard pitfall called out in the plan.
func asChecklistRunModel(t *testing.T, model tea.Model) checklistRunModel {
	t.Helper()
	result, ok := model.(checklistRunModel)
	require.True(t, ok, "expected checklistRunModel, got %T", model)
	return result
}

// --- Navigation ---

func TestChecklistRunModelNavigateDown(t *testing.T) {
	svc := setupChecklistRunTestService(t)
	run := startTestRun(t, svc, "morning", []string{"A", "B", "C"})

	m := checklistRunModel{service: svc, run: run, cursor: 0}

	updated, _ := m.Update(keyRune("j"))
	result := asChecklistRunModel(t, updated)
	assert.Equal(t, 1, result.cursor)

	updated, _ = result.Update(tea.KeyMsg{Type: tea.KeyDown})
	result = asChecklistRunModel(t, updated)
	assert.Equal(t, 2, result.cursor)

	// Clamps at the last item.
	updated, _ = result.Update(keyRune("j"))
	result = asChecklistRunModel(t, updated)
	assert.Equal(t, 2, result.cursor)

	updated, _ = result.Update(tea.KeyMsg{Type: tea.KeyDown})
	result = asChecklistRunModel(t, updated)
	assert.Equal(t, 2, result.cursor)
}

func TestChecklistRunModelNavigateUp(t *testing.T) {
	svc := setupChecklistRunTestService(t)
	run := startTestRun(t, svc, "morning", []string{"A", "B", "C"})

	m := checklistRunModel{service: svc, run: run, cursor: 2}

	updated, _ := m.Update(keyRune("k"))
	result := asChecklistRunModel(t, updated)
	assert.Equal(t, 1, result.cursor)

	updated, _ = result.Update(tea.KeyMsg{Type: tea.KeyUp})
	result = asChecklistRunModel(t, updated)
	assert.Equal(t, 0, result.cursor)

	// Clamps at the first item.
	updated, _ = result.Update(keyRune("k"))
	result = asChecklistRunModel(t, updated)
	assert.Equal(t, 0, result.cursor)

	updated, _ = result.Update(tea.KeyMsg{Type: tea.KeyUp})
	result = asChecklistRunModel(t, updated)
	assert.Equal(t, 0, result.cursor)
}

// --- Toggling ---

func TestChecklistRunModelToggleUncheckedViaSpace(t *testing.T) {
	svc := setupChecklistRunTestService(t)
	run := startTestRun(t, svc, "morning", []string{"A", "B"})

	m := checklistRunModel{service: svc, run: run, cursor: 0}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	result := asChecklistRunModel(t, updated)

	assert.False(t, result.completed)

	status, err := svc.GetRunStatus(run.ID)
	require.NoError(t, err)
	assert.True(t, status.Items[0].Checked)
	assert.Equal(t, checklist.RunStatusActive, status.Status)
}

func TestChecklistRunModelToggleUncheckedViaEnter(t *testing.T) {
	svc := setupChecklistRunTestService(t)
	run := startTestRun(t, svc, "morning", []string{"A", "B"})

	m := checklistRunModel{service: svc, run: run, cursor: 1}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	result := asChecklistRunModel(t, updated)

	assert.False(t, result.completed)

	status, err := svc.GetRunStatus(run.ID)
	require.NoError(t, err)
	assert.True(t, status.Items[1].Checked)
	assert.False(t, status.Items[0].Checked)
	assert.Equal(t, checklist.RunStatusActive, status.Status)
}

func TestChecklistRunModelToggleCheckedOff(t *testing.T) {
	svc := setupChecklistRunTestService(t)
	run := startTestRun(t, svc, "morning", []string{"A", "B"})

	require.NoError(t, svc.CheckItem(run.ID, 0))
	checkedRun, err := svc.GetRunStatus(run.ID)
	require.NoError(t, err)
	require.True(t, checkedRun.Items[0].Checked)

	m := checklistRunModel{service: svc, run: checkedRun, cursor: 0}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	asChecklistRunModel(t, updated)

	status, err := svc.GetRunStatus(run.ID)
	require.NoError(t, err)
	assert.False(t, status.Items[0].Checked)
	assert.Nil(t, status.Items[0].CheckedAt)
}

// --- Auto-complete ---

func TestChecklistRunModelAutoComplete(t *testing.T) {
	svc := setupChecklistRunTestService(t)
	run := startTestRun(t, svc, "morning", []string{"A", "B"})

	require.NoError(t, svc.CheckItem(run.ID, 0))
	partial, err := svc.GetRunStatus(run.ID)
	require.NoError(t, err)
	require.Equal(t, checklist.RunStatusActive, partial.Status)

	m := checklistRunModel{service: svc, run: partial, cursor: 1}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	result := asChecklistRunModel(t, updated)

	assert.True(t, result.completed)
	assert.False(t, result.abandoned)
	assert.False(t, result.canceled)
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())

	final, err := svc.GetRunStatus(run.ID)
	require.NoError(t, err)
	assert.Equal(t, checklist.RunStatusCompleted, final.Status)
}

// --- Abandon ---

func TestChecklistRunModelAbandon(t *testing.T) {
	svc := setupChecklistRunTestService(t)
	run := startTestRun(t, svc, "morning", []string{"A", "B"})

	require.NoError(t, svc.CheckItem(run.ID, 0))
	partial, err := svc.GetRunStatus(run.ID)
	require.NoError(t, err)

	m := checklistRunModel{service: svc, run: partial, cursor: 0}
	updated, cmd := m.Update(keyRune("a"))
	result := asChecklistRunModel(t, updated)

	assert.True(t, result.abandoned)
	assert.False(t, result.completed)
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())

	_, err = svc.GetActiveRun("morning")
	assert.Error(t, err)

	active, err := svc.ListRuns(false)
	require.NoError(t, err)
	assert.Len(t, active, 0)

	abandonedStatus, err := svc.GetRunStatus(run.ID)
	require.NoError(t, err)
	assert.Equal(t, checklist.RunStatusAbandoned, abandonedStatus.Status)
}

// --- Quit (q / esc / ctrl+c) ---

// runChecklistRunModelQuitCase exercises a quit keypress and asserts the
// shared "canceled, run unaffected" behavior. It is not itself a test
// function (it takes more than a *testing.T), so `go test` will not try to
// run it directly.
func runChecklistRunModelQuitCase(t *testing.T, msg tea.KeyMsg) {
	t.Helper()
	svc := setupChecklistRunTestService(t)
	run := startTestRun(t, svc, "morning", []string{"A", "B"})

	require.NoError(t, svc.CheckItem(run.ID, 0))
	partial, err := svc.GetRunStatus(run.ID)
	require.NoError(t, err)

	m := checklistRunModel{service: svc, run: partial, cursor: 0}
	updated, cmd := m.Update(msg)
	result := asChecklistRunModel(t, updated)

	assert.True(t, result.canceled)
	assert.False(t, result.abandoned)
	assert.False(t, result.completed)
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())

	status, err := svc.GetRunStatus(run.ID)
	require.NoError(t, err)
	assert.Equal(t, checklist.RunStatusActive, status.Status)
	assert.True(t, status.Items[0].Checked)
	assert.False(t, status.Items[1].Checked)
}

func TestChecklistRunModelQuitQ(t *testing.T) {
	runChecklistRunModelQuitCase(t, keyRune("q"))
}

func TestChecklistRunModelQuitEsc(t *testing.T) {
	runChecklistRunModelQuitCase(t, tea.KeyMsg{Type: tea.KeyEsc})
}

func TestChecklistRunModelQuitCtrlC(t *testing.T) {
	runChecklistRunModelQuitCase(t, tea.KeyMsg{Type: tea.KeyCtrlC})
}

// --- View ---

func TestChecklistRunModelViewActive(t *testing.T) {
	svc := setupChecklistRunTestService(t)
	run := startTestRun(t, svc, "morning", []string{"Check email", "Review calendar"})

	m := checklistRunModel{service: svc, run: run, cursor: 0}
	view := m.View()

	assert.NotEmpty(t, view)
	assert.Contains(t, view, "morning")
	assert.Contains(t, view, "[ ]")

	lines := strings.Split(view, "\n")
	// name line + one line per item + one hint line, nothing more.
	assert.Len(t, lines, len(run.Items)+2)
}

func TestChecklistRunModelViewCompleted(t *testing.T) {
	svc := setupChecklistRunTestService(t)
	run := startTestRun(t, svc, "morning", []string{"Check email"})

	require.NoError(t, svc.CheckItem(run.ID, 0))
	completedRun, err := svc.GetRunStatus(run.ID)
	require.NoError(t, err)

	m := checklistRunModel{service: svc, run: completedRun, cursor: 0, completed: true}
	view := m.View()

	assert.NotEmpty(t, view)
	assert.Contains(t, view, "morning")
	assert.Contains(t, view, "Complete")
}

func TestChecklistRunModelViewCanceled(t *testing.T) {
	svc := setupChecklistRunTestService(t)
	run := startTestRun(t, svc, "morning", []string{"Check email"})

	m := checklistRunModel{service: svc, run: run, cursor: 0, canceled: true}
	assert.Equal(t, "", m.View())
}

func TestChecklistRunModelViewAbandoned(t *testing.T) {
	svc := setupChecklistRunTestService(t)
	run := startTestRun(t, svc, "morning", []string{"Check email"})

	m := checklistRunModel{service: svc, run: run, cursor: 0, abandoned: true}
	assert.Equal(t, "", m.View())
}

func TestChecklistRunModelViewErr(t *testing.T) {
	svc := setupChecklistRunTestService(t)
	run := startTestRun(t, svc, "morning", []string{"Check email"})

	m := checklistRunModel{service: svc, run: run, cursor: 0, err: fmt.Errorf("boom")}
	assert.Equal(t, "", m.View())
}

// --- Empty checklist (0 items) ---

func TestChecklistRunModelEmptyChecklist(t *testing.T) {
	svc := setupChecklistRunTestService(t)
	run := startTestRun(t, svc, "empty", []string{})
	require.Len(t, run.Items, 0)

	m := checklistRunModel{service: svc, run: run, cursor: 0}

	view := m.View()
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "(no items)")

	// space is a no-op: no completion, no crash, no service error.
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	result := asChecklistRunModel(t, updated)
	assert.Nil(t, cmd)
	assert.False(t, result.completed)
	assert.False(t, result.abandoned)
	assert.False(t, result.canceled)
	assert.Nil(t, result.err)

	// enter is a no-op.
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	result = asChecklistRunModel(t, updated)
	assert.Nil(t, cmd)
	assert.False(t, result.completed)

	// j/k are no-ops: cursor stays at 0 with nothing to select.
	updated, _ = m.Update(keyRune("j"))
	result = asChecklistRunModel(t, updated)
	assert.Equal(t, 0, result.cursor)

	updated, _ = m.Update(keyRune("k"))
	result = asChecklistRunModel(t, updated)
	assert.Equal(t, 0, result.cursor)

	// q quits without abandoning.
	updated, cmd = m.Update(keyRune("q"))
	result = asChecklistRunModel(t, updated)
	assert.True(t, result.canceled)
	assert.False(t, result.abandoned)
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())

	// a abandons and exits (exercised last since it mutates the DB).
	updated, cmd = m.Update(keyRune("a"))
	result = asChecklistRunModel(t, updated)
	assert.True(t, result.abandoned)
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())

	_, err := svc.GetActiveRun("empty")
	assert.Error(t, err)
}
