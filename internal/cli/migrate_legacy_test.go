// Tests for `rk migrate legacy` (internal/cli/migrate_legacy.go), the CLI
// surface over internal/textmigrate's one-shot legacy-data importer.
// `migrate legacy` is a child of the `migrate` parent command; it wires
// --dry-run/--verify/--source to a textmigrate.Importer and drives the real
// RootCmd.Execute() dispatch end-to-end. The top-level `rk import` verb no
// longer exists.
//
// Harness: reuses setupQueryVault/resetCLIFlags from query_test.go, mirroring
// adopt_test.go's real-Execute() style.
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// runImport executes `rk migrate legacy --vault <vault> [args...]` through
// RootCmd and returns stdout plus the command's error.
func runImport(t *testing.T, vault string, args ...string) (stdout string, err error) {
	t.Helper()
	var buf bytes.Buffer
	RootCmd.SetOut(&buf)
	RootCmd.SetErr(&buf)
	RootCmd.SetArgs(append([]string{"migrate", "legacy", "--vault", vault}, args...))
	err = RootCmd.Execute()
	return buf.String(), err
}

// Given a fixture legacy data root with one gen-1 task file, when `rk
// migrate legacy` runs against it, then it succeeds and one todos/<ULID>.md
// file exists under the vault.
func TestMigrateLegacyCmd_RealRun_CreatesTodoFile(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	source := t.TempDir()
	writeLegacyFixtureTask(t, source, "legacy-abc123", "Buy milk")

	if _, err := runImport(t, vault, "--source", source); err != nil {
		t.Fatalf("rk migrate legacy: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(vault, "todos"))
	if err != nil {
		t.Fatalf("read todos dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("todos dir has %d entries, want 1", len(entries))
	}
}

// Given the same fixture, `rk migrate legacy --dry-run` succeeds without
// writing any file under the vault.
func TestMigrateLegacyCmd_DryRun_SucceedsWritesNoFiles(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	source := t.TempDir()
	writeLegacyFixtureTask(t, source, "legacy-abc123", "Buy milk")

	if _, err := runImport(t, vault, "--source", source, "--dry-run"); err != nil {
		t.Fatalf("rk migrate legacy --dry-run: %v", err)
	}

	if _, err := os.Stat(filepath.Join(vault, "todos")); !os.IsNotExist(err) {
		t.Fatalf("--dry-run wrote todos/ (stat err: %v)", err)
	}
}

// Given a completed real migration, `rk migrate legacy --verify` succeeds.
func TestMigrateLegacyCmd_Verify_SucceedsAfterRealRun(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	source := t.TempDir()
	writeLegacyFixtureTask(t, source, "legacy-abc123", "Buy milk")

	if _, err := runImport(t, vault, "--source", source); err != nil {
		t.Fatalf("rk migrate legacy: %v", err)
	}
	resetCLIFlags()

	if _, err := runImport(t, vault, "--source", source, "--verify"); err != nil {
		t.Fatalf("rk migrate legacy --verify: %v", err)
	}
}

// Given --source pointing at a fixture legacy root distinct from the
// default legacy data directory, `rk migrate legacy` migrates from the
// flagged source, not the default.
func TestMigrateLegacyCmd_SourceFlagOverridesDefault(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	// Point the default legacy data dir somewhere with nothing in it, and
	// the fixture (real) source somewhere else with one task.
	emptyDefault := t.TempDir()
	t.Setenv("RECKON_DATA_DIR", emptyDefault)

	source := t.TempDir()
	writeLegacyFixtureTask(t, source, "legacy-abc123", "Buy milk")

	if _, err := runImport(t, vault, "--source", source); err != nil {
		t.Fatalf("rk migrate legacy --source: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(vault, "todos"))
	if err != nil {
		t.Fatalf("read todos dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("todos dir has %d entries, want 1 (import must have read --source, not the empty default)", len(entries))
	}
}

// Given the same fixture as the real-run scenario above, when the run's
// stdout is captured, then it contains the summary line's exact counts.
// Only the summary line is asserted (never the whole multi-line output):
// the per-record lines that follow embed a freshly-minted ULID and are
// non-deterministic across runs.
func TestMigrateLegacyCmd_OutputSummary(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	source := t.TempDir()
	writeLegacyFixtureTask(t, source, "legacy-abc123", "Buy milk")

	out, err := runImport(t, vault, "--source", source)
	if err != nil {
		t.Fatalf("rk migrate legacy: %v", err)
	}
	want := "import: 1 created, 0 skipped, 0 errored"
	if !strings.Contains(out, want) {
		t.Errorf("stdout = %q, want it to contain %q", out, want)
	}
}

// Given both --dry-run and --verify, `rk migrate legacy` still rejects the
// combination with the same mutual-exclusivity error text as today's `rk
// import` (the "import:" prefix is preserved verbatim; it is not renamed to
// describe the new command tree).
func TestMigrateLegacy_MutualExclusivity(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	source := t.TempDir()
	writeLegacyFixtureTask(t, source, "legacy-abc123", "Buy milk")

	_, err := runImport(t, vault, "--source", source, "--dry-run", "--verify")
	if err == nil {
		t.Fatal("expected mutual-exclusivity error for --dry-run --verify, got nil")
	}
	want := "import: --dry-run and --verify are mutually exclusive"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), want)
	}
}

// TestMigrateSubcommandSurface asserts the two-level `migrate legacy`
// structure: `migrate` is registered on RootCmd, has no RunE of its own
// (mirrors noteCmd), and its only child is `legacy`. Located by name via
// RootCmd.Commands(), never by referencing a production var directly.
func TestMigrateSubcommandSurface(t *testing.T) {
	var migrateCommand *cobra.Command
	for _, cmd := range RootCmd.Commands() {
		if cmd.Name() == "migrate" {
			migrateCommand = cmd
			break
		}
	}
	if migrateCommand == nil {
		t.Fatal("migrate command not found on RootCmd")
	}
	if migrateCommand.RunE != nil {
		t.Error("migrate command must have no RunE (bare `rk migrate` should print help, not run)")
	}

	names := make(map[string]bool)
	for _, cmd := range migrateCommand.Commands() {
		names[cmd.Name()] = true
	}
	if len(names) != 1 || !names["legacy"] {
		t.Errorf("migrate subcommands = %v, want exactly {\"legacy\"}", names)
	}
}

// TestImportVerbRemoved asserts the top-level `rk import` verb no longer
// resolves: RootCmd.Execute() with args ["import"] must fail with cobra's
// unknown-command error, not run any migration.
func TestImportVerbRemoved(t *testing.T) {
	// Isolate: for as long as `import` remains registered, executing it for
	// real must not touch the caller's actual home-directory vault or data
	// dir.
	t.Setenv("RECKON_VAULT", t.TempDir())
	t.Setenv("RECKON_DATA_DIR", t.TempDir())
	t.Setenv("RECKON_CACHE", t.TempDir())

	var buf bytes.Buffer
	RootCmd.SetOut(&buf)
	RootCmd.SetErr(&buf)
	RootCmd.SetArgs([]string{"import"})
	t.Cleanup(resetCLIFlags)

	err := RootCmd.Execute()
	if err == nil {
		t.Fatal("expected `rk import` to be removed (unknown command), got nil error")
	}
	want := `unknown command "import" for "rk"`
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), want)
	}
}

// TestMigrateBareInvocation asserts `rk migrate` with no subcommand prints
// help and exits 0 — the parent command has no RunE, same as `rk note`
// alone today.
func TestMigrateBareInvocation(t *testing.T) {
	t.Setenv("RECKON_VAULT", t.TempDir())
	t.Setenv("RECKON_DATA_DIR", t.TempDir())

	var buf bytes.Buffer
	RootCmd.SetOut(&buf)
	RootCmd.SetErr(&buf)
	RootCmd.SetArgs([]string{"migrate"})
	t.Cleanup(resetCLIFlags)

	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("rk migrate (bare): expected nil error, got: %v", err)
	}
}

// TestMigrateUnknownSubcommand asserts `rk migrate bogus` fails with
// cobra's unknown-command error scoped to the `migrate` command path.
func TestMigrateUnknownSubcommand(t *testing.T) {
	t.Setenv("RECKON_VAULT", t.TempDir())
	t.Setenv("RECKON_DATA_DIR", t.TempDir())

	var buf bytes.Buffer
	RootCmd.SetOut(&buf)
	RootCmd.SetErr(&buf)
	RootCmd.SetArgs([]string{"migrate", "bogus"})
	t.Cleanup(resetCLIFlags)

	err := RootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for `rk migrate bogus`, got nil")
	}
	want := `unknown command "bogus" for "rk migrate"`
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), want)
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
