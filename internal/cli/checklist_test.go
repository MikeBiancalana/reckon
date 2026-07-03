package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MikeBiancalana/reckon/internal/checklist"
	"github.com/MikeBiancalana/reckon/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupChecklistCLITestService points the package-global checklistService at
// a fresh temp-SQLite database for the duration of a single test, restoring
// it (and quietFlag) afterward. This mirrors how other internal/cli tests
// (e.g. notes_integration_test.go) set package-global services directly.
func setupChecklistCLITestService(t *testing.T) *checklist.Service {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	db, err := storage.NewDatabase(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	repo := checklist.NewRepository(db)
	svc := checklist.NewService(repo)

	checklistService = svc
	quietFlag = false
	t.Cleanup(func() {
		checklistService = nil
		quietFlag = false
	})

	return svc
}

// captureStdout redirects os.Stdout for the duration of fn and returns
// everything written to it. Needed because checklist commands print via
// fmt.Printf/fmt.Println directly to os.Stdout rather than cmd.OutOrStdout().
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()

	require.NoError(t, w.Close())
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(out)
}

// --- rk cl abandon ---

func TestChecklistAbandonCmd_ActiveRun(t *testing.T) {
	svc := setupChecklistCLITestService(t)

	_, err := svc.CreateTemplate("morning", []string{"Step 1", "Step 2"})
	require.NoError(t, err)
	run, err := svc.StartRun("morning")
	require.NoError(t, err)

	var cmdErr error
	output := captureStdout(t, func() {
		cmd := checklistAbandonCmd()
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		cmd.SetArgs([]string{"morning"})
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	assert.Contains(t, output, "Abandoned")
	assert.Contains(t, output, "morning")

	status, err := svc.GetRunStatus(run.ID)
	require.NoError(t, err)
	assert.Equal(t, checklist.RunStatusAbandoned, status.Status)

	_, err = svc.GetActiveRun("morning")
	assert.Error(t, err)
}

func TestChecklistAbandonCmd_Quiet(t *testing.T) {
	svc := setupChecklistCLITestService(t)

	_, err := svc.CreateTemplate("morning", []string{"Step 1"})
	require.NoError(t, err)
	run, err := svc.StartRun("morning")
	require.NoError(t, err)

	quietFlag = true

	var cmdErr error
	output := captureStdout(t, func() {
		cmd := checklistAbandonCmd()
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		cmd.SetArgs([]string{"morning"})
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	assert.Equal(t, run.ID, strings.TrimSpace(output))
}

func TestChecklistAbandonCmd_NoActiveRun(t *testing.T) {
	svc := setupChecklistCLITestService(t)

	_, err := svc.CreateTemplate("morning", []string{"Step 1"})
	require.NoError(t, err)

	cmd := checklistAbandonCmd()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs([]string{"morning"})
	err = cmd.Execute()
	assert.Error(t, err)
}

func TestChecklistAbandonCmd_TemplateNotFound(t *testing.T) {
	setupChecklistCLITestService(t)

	cmd := checklistAbandonCmd()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs([]string{"ghost"})
	err := cmd.Execute()
	assert.Error(t, err)
}

// --- resolveChecklistRun ---

func TestResolveChecklistRun_Resumes(t *testing.T) {
	svc := setupChecklistCLITestService(t)

	_, err := svc.CreateTemplate("morning", []string{"Step 1", "Step 2"})
	require.NoError(t, err)
	started, err := svc.StartRun("morning")
	require.NoError(t, err)
	require.NoError(t, svc.CheckItem(started.ID, 0))

	run, resumed, err := resolveChecklistRun(svc, "morning")
	require.NoError(t, err)
	assert.True(t, resumed)
	assert.Equal(t, started.ID, run.ID)
	assert.True(t, run.Items[0].Checked)
}

func TestResolveChecklistRun_StartsFreshAfterAbandon(t *testing.T) {
	svc := setupChecklistCLITestService(t)

	_, err := svc.CreateTemplate("morning", []string{"Step 1"})
	require.NoError(t, err)
	started, err := svc.StartRun("morning")
	require.NoError(t, err)
	_, err = svc.AbandonRun("morning")
	require.NoError(t, err)

	run, resumed, err := resolveChecklistRun(svc, "morning")
	require.NoError(t, err)
	assert.False(t, resumed)
	assert.NotEqual(t, started.ID, run.ID)
	assert.Equal(t, checklist.RunStatusActive, run.Status)
	assert.False(t, run.Items[0].Checked)
}

func TestResolveChecklistRun_StartsFreshAfterCompletion(t *testing.T) {
	svc := setupChecklistCLITestService(t)

	_, err := svc.CreateTemplate("morning", []string{"Step 1"})
	require.NoError(t, err)
	started, err := svc.StartRun("morning")
	require.NoError(t, err)
	// Checking the only item auto-completes the run.
	require.NoError(t, svc.CheckItem(started.ID, 0))

	run, resumed, err := resolveChecklistRun(svc, "morning")
	require.NoError(t, err)
	assert.False(t, resumed)
	assert.NotEqual(t, started.ID, run.ID)
	assert.Equal(t, checklist.RunStatusActive, run.Status)
	assert.False(t, run.Items[0].Checked)
}

func TestResolveChecklistRun_TemplateNotFound(t *testing.T) {
	svc := setupChecklistCLITestService(t)

	_, _, err := resolveChecklistRun(svc, "ghost")
	assert.Error(t, err)
}
