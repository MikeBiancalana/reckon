// TDD red tests for `rk import` (internal/cli/import.go), the CLI surface
// over internal/textmigrate's one-shot legacy-data importer. import.go
// registers the command and wires --dry-run/--verify/--source to a
// textmigrate.Importer, but every scenario below drives the real
// RootCmd.Execute() dispatch end-to-end and IS EXPECTED TO FAIL until
// internal/textmigrate's Importer.Run/Verify are implemented (currently
// compilation-only stubs that always return a "not implemented" error).
//
// Harness: reuses setupQueryVault/resetCLIFlags from query_test.go, mirroring
// adopt_test.go's real-Execute() style.
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// runImport executes `rk import --vault <vault> [args...]` through RootCmd
// and returns stdout plus the command's error.
func runImport(t *testing.T, vault string, args ...string) (stdout string, err error) {
	t.Helper()
	var buf bytes.Buffer
	RootCmd.SetOut(&buf)
	RootCmd.SetErr(&buf)
	RootCmd.SetArgs(append([]string{"import", "--vault", vault}, args...))
	err = RootCmd.Execute()
	return buf.String(), err
}

// Given a fixture legacy data root with one gen-1 task file, when `rk
// import` runs against it, then it succeeds and one todos/<ULID>.md file
// exists under the vault.
func TestImportCmd_RealRun_CreatesTodoFile(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	source := t.TempDir()
	writeLegacyFixtureTask(t, source, "legacy-abc123", "Buy milk")

	if _, err := runImport(t, vault, "--source", source); err != nil {
		t.Fatalf("rk import: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(vault, "todos"))
	if err != nil {
		t.Fatalf("read todos dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("todos dir has %d entries, want 1", len(entries))
	}
}

// Given the same fixture, `rk import --dry-run` succeeds without writing
// any file under the vault.
func TestImportCmd_DryRun_SucceedsWritesNoFiles(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	source := t.TempDir()
	writeLegacyFixtureTask(t, source, "legacy-abc123", "Buy milk")

	if _, err := runImport(t, vault, "--source", source, "--dry-run"); err != nil {
		t.Fatalf("rk import --dry-run: %v", err)
	}

	if _, err := os.Stat(filepath.Join(vault, "todos")); !os.IsNotExist(err) {
		t.Fatalf("--dry-run wrote todos/ (stat err: %v)", err)
	}
}

// Given a completed real import, `rk import --verify` succeeds.
func TestImportCmd_Verify_SucceedsAfterRealRun(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	source := t.TempDir()
	writeLegacyFixtureTask(t, source, "legacy-abc123", "Buy milk")

	if _, err := runImport(t, vault, "--source", source); err != nil {
		t.Fatalf("rk import: %v", err)
	}
	resetCLIFlags()

	if _, err := runImport(t, vault, "--source", source, "--verify"); err != nil {
		t.Fatalf("rk import --verify: %v", err)
	}
}

// Given --source pointing at a fixture legacy root distinct from the
// default legacy data directory, `rk import` migrates from the flagged
// source, not the default.
func TestImportCmd_SourceFlagOverridesDefault(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	// Point the default legacy data dir somewhere with nothing in it, and
	// the fixture (real) source somewhere else with one task.
	emptyDefault := t.TempDir()
	t.Setenv("RECKON_DATA_DIR", emptyDefault)

	source := t.TempDir()
	writeLegacyFixtureTask(t, source, "legacy-abc123", "Buy milk")

	if _, err := runImport(t, vault, "--source", source); err != nil {
		t.Fatalf("rk import --source: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(vault, "todos"))
	if err != nil {
		t.Fatalf("read todos dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("todos dir has %d entries, want 1 (import must have read --source, not the empty default)", len(entries))
	}
}

// writeLegacyFixtureTask hand-renders one legacy gen-1 task file under
// source/tasks/, matching the frontmatter keys internal/journal's real
// reader expects (id, title, created, status).
func writeLegacyFixtureTask(t *testing.T, source, id, title string) {
	t.Helper()
	dir := filepath.Join(source, "tasks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	content := "---\nid: " + id + "\ntitle: " + title + "\ncreated: 2026-01-05\nstatus: open\n---\n\n## Description\n\n\n"
	path := filepath.Join(dir, "task1.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
