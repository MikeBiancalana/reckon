package checklist

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/MikeBiancalana/reckon/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupChecklistTestService(t *testing.T) *Service {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	db, err := storage.NewDatabase(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	repo := NewRepository(db)
	return NewService(repo)
}

func TestCreateTemplate(t *testing.T) {
	svc := setupChecklistTestService(t)

	tpl, err := svc.CreateTemplate("morning", []string{"Check email", "Review calendar"})
	require.NoError(t, err)
	assert.NotEmpty(t, tpl.ID)
	assert.Equal(t, "morning", tpl.Name)
	assert.Len(t, tpl.Items, 2)
	assert.Equal(t, "Check email", tpl.Items[0].Text)
	assert.Equal(t, 0, tpl.Items[0].Position)
	assert.Equal(t, "Review calendar", tpl.Items[1].Text)
	assert.Equal(t, 1, tpl.Items[1].Position)
}

func TestCreateTemplateDuplicateName(t *testing.T) {
	svc := setupChecklistTestService(t)

	_, err := svc.CreateTemplate("morning", []string{"Step 1"})
	require.NoError(t, err)

	_, err = svc.CreateTemplate("morning", []string{"Step 2"})
	assert.Error(t, err)
}

func TestGetTemplate(t *testing.T) {
	svc := setupChecklistTestService(t)

	tpl, err := svc.CreateTemplate("standup", []string{"Blockers?", "Progress?"})
	require.NoError(t, err)

	// Get by name
	got, err := svc.GetTemplate("standup")
	require.NoError(t, err)
	assert.Equal(t, tpl.ID, got.ID)
	assert.Len(t, got.Items, 2)

	// Get by ID
	gotByID, err := svc.GetTemplate(tpl.ID)
	require.NoError(t, err)
	assert.Equal(t, tpl.ID, gotByID.ID)
}

func TestGetTemplateNotFound(t *testing.T) {
	svc := setupChecklistTestService(t)
	_, err := svc.GetTemplate("nonexistent")
	assert.Error(t, err)
}

func TestListTemplates(t *testing.T) {
	svc := setupChecklistTestService(t)

	_, err := svc.CreateTemplate("morning", []string{"A"})
	require.NoError(t, err)
	_, err = svc.CreateTemplate("evening", []string{"B", "C"})
	require.NoError(t, err)

	templates, err := svc.ListTemplates()
	require.NoError(t, err)
	assert.Len(t, templates, 2)
}

func TestDeleteTemplate(t *testing.T) {
	svc := setupChecklistTestService(t)

	tpl, err := svc.CreateTemplate("to-delete", []string{"step"})
	require.NoError(t, err)

	err = svc.DeleteTemplate(tpl.Name)
	require.NoError(t, err)

	_, err = svc.GetTemplate(tpl.Name)
	assert.Error(t, err)
}

func TestAddTemplateItem(t *testing.T) {
	svc := setupChecklistTestService(t)

	_, err := svc.CreateTemplate("morning", []string{"Step 1"})
	require.NoError(t, err)

	err = svc.AddTemplateItem("morning", "Step 2")
	require.NoError(t, err)

	tpl, err := svc.GetTemplate("morning")
	require.NoError(t, err)
	assert.Len(t, tpl.Items, 2)
	assert.Equal(t, "Step 2", tpl.Items[1].Text)
	assert.Equal(t, 1, tpl.Items[1].Position)
}

func TestRemoveTemplateItem(t *testing.T) {
	svc := setupChecklistTestService(t)

	_, err := svc.CreateTemplate("morning", []string{"Step A", "Step B", "Step C"})
	require.NoError(t, err)

	// Remove position 1 (0-based: second item "Step B")
	err = svc.RemoveTemplateItem("morning", 1)
	require.NoError(t, err)

	tpl, err := svc.GetTemplate("morning")
	require.NoError(t, err)
	assert.Len(t, tpl.Items, 2)
	assert.Equal(t, "Step A", tpl.Items[0].Text)
	assert.Equal(t, "Step C", tpl.Items[1].Text)
	// Positions should be compacted
	assert.Equal(t, 0, tpl.Items[0].Position)
	assert.Equal(t, 1, tpl.Items[1].Position)
}

func TestStartRun(t *testing.T) {
	svc := setupChecklistTestService(t)

	_, err := svc.CreateTemplate("morning", []string{"Email", "Calendar"})
	require.NoError(t, err)

	run, err := svc.StartRun("morning")
	require.NoError(t, err)
	assert.NotEmpty(t, run.ID)
	assert.Equal(t, RunStatusActive, run.Status)
	assert.Len(t, run.Items, 2)
	assert.False(t, run.Items[0].Checked)
	assert.False(t, run.Items[1].Checked)
}

func TestStartRunAlreadyActive(t *testing.T) {
	svc := setupChecklistTestService(t)

	_, err := svc.CreateTemplate("morning", []string{"Step 1"})
	require.NoError(t, err)

	_, err = svc.StartRun("morning")
	require.NoError(t, err)

	// Second start should fail - active run exists
	_, err = svc.StartRun("morning")
	assert.Error(t, err)
}

func TestGetActiveRun(t *testing.T) {
	svc := setupChecklistTestService(t)

	_, err := svc.CreateTemplate("morning", []string{"Step 1"})
	require.NoError(t, err)

	run, err := svc.StartRun("morning")
	require.NoError(t, err)

	activeRun, err := svc.GetActiveRun("morning")
	require.NoError(t, err)
	assert.Equal(t, run.ID, activeRun.ID)
}

func TestCheckItem(t *testing.T) {
	svc := setupChecklistTestService(t)

	_, err := svc.CreateTemplate("morning", []string{"Step 1", "Step 2"})
	require.NoError(t, err)

	run, err := svc.StartRun("morning")
	require.NoError(t, err)

	// Check item at position 0 (1-based: 1)
	err = svc.CheckItem(run.ID, 0)
	require.NoError(t, err)

	updated, err := svc.GetRunStatus(run.ID)
	require.NoError(t, err)
	assert.True(t, updated.Items[0].Checked)
	assert.NotNil(t, updated.Items[0].CheckedAt)
	assert.False(t, updated.Items[1].Checked)
	assert.Equal(t, RunStatusActive, updated.Status)
}

func TestCheckAllItemsCompletesRun(t *testing.T) {
	svc := setupChecklistTestService(t)

	_, err := svc.CreateTemplate("morning", []string{"Step 1", "Step 2"})
	require.NoError(t, err)

	run, err := svc.StartRun("morning")
	require.NoError(t, err)

	err = svc.CheckItem(run.ID, 0)
	require.NoError(t, err)
	err = svc.CheckItem(run.ID, 1)
	require.NoError(t, err)

	completed, err := svc.GetRunStatus(run.ID)
	require.NoError(t, err)
	assert.Equal(t, RunStatusCompleted, completed.Status)
	assert.NotNil(t, completed.CompletedAt)
}

func TestCheckItemUncheck(t *testing.T) {
	svc := setupChecklistTestService(t)

	_, err := svc.CreateTemplate("morning", []string{"Step 1"})
	require.NoError(t, err)

	run, err := svc.StartRun("morning")
	require.NoError(t, err)

	err = svc.CheckItem(run.ID, 0)
	require.NoError(t, err)

	updated, err := svc.GetRunStatus(run.ID)
	require.NoError(t, err)
	assert.True(t, updated.Items[0].Checked)

	// Uncheck: toggle again
	err = svc.CheckItem(run.ID, 0)
	require.NoError(t, err)

	updated, err = svc.GetRunStatus(run.ID)
	require.NoError(t, err)
	assert.False(t, updated.Items[0].Checked)
	assert.Nil(t, updated.Items[0].CheckedAt)
}

func TestResetRun(t *testing.T) {
	svc := setupChecklistTestService(t)

	_, err := svc.CreateTemplate("morning", []string{"Step 1", "Step 2"})
	require.NoError(t, err)

	run, err := svc.StartRun("morning")
	require.NoError(t, err)
	// Only check first item — run stays active (not auto-completed)
	err = svc.CheckItem(run.ID, 0)
	require.NoError(t, err)

	newRun, err := svc.ResetRun("morning")
	require.NoError(t, err)
	assert.NotEqual(t, run.ID, newRun.ID)
	assert.Equal(t, RunStatusActive, newRun.Status)
	assert.False(t, newRun.Items[0].Checked)

	// Old run should be abandoned
	oldRun, err := svc.GetRunStatus(run.ID)
	require.NoError(t, err)
	assert.Equal(t, RunStatusAbandoned, oldRun.Status)
}

func TestResetRunNoActive(t *testing.T) {
	svc := setupChecklistTestService(t)

	_, err := svc.CreateTemplate("morning", []string{"Step 1"})
	require.NoError(t, err)

	// Reset with no active run just starts a new one
	run, err := svc.ResetRun("morning")
	require.NoError(t, err)
	assert.NotEmpty(t, run.ID)
	assert.Equal(t, RunStatusActive, run.Status)
}

func TestListRuns(t *testing.T) {
	svc := setupChecklistTestService(t)

	_, err := svc.CreateTemplate("morning", []string{"Step 1", "Step 2"})
	require.NoError(t, err)

	run, err := svc.StartRun("morning")
	require.NoError(t, err)

	// Check all to complete
	err = svc.CheckItem(run.ID, 0)
	require.NoError(t, err)
	err = svc.CheckItem(run.ID, 1)
	require.NoError(t, err)

	// Start another run
	_, err = svc.StartRun("morning")
	require.NoError(t, err)

	all, err := svc.ListRuns(true)
	require.NoError(t, err)
	assert.Len(t, all, 2)

	active, err := svc.ListRuns(false)
	require.NoError(t, err)
	assert.Len(t, active, 1)
	assert.Equal(t, RunStatusActive, active[0].Status)
}

func TestRunItemTimestamps(t *testing.T) {
	svc := setupChecklistTestService(t)

	_, err := svc.CreateTemplate("morning", []string{"Step 1"})
	require.NoError(t, err)

	before := time.Now().Add(-time.Second)
	run, err := svc.StartRun("morning")
	require.NoError(t, err)

	err = svc.CheckItem(run.ID, 0)
	require.NoError(t, err)
	after := time.Now().Add(time.Second)

	updated, err := svc.GetRunStatus(run.ID)
	require.NoError(t, err)
	require.NotNil(t, updated.Items[0].CheckedAt)
	assert.True(t, updated.Items[0].CheckedAt.After(before))
	assert.True(t, updated.Items[0].CheckedAt.Before(after))
}
