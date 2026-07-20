package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestRootHelp_ListsSubcommands (T-1 / AC-1): rk --help must exit 0, print
// "Usage:", and list the surviving v1 subcommand verbs.
//
// Superseded by TestRootCommandSurface for absence checks: this test only
// asserts presence via substring match, which cannot express "verb X is
// gone" (dying verb names survive as substrings of surviving flags/help
// text — see TestRootCommandSurface).
//
// NOTE: RootCmd is a package-level global; each test restores SetArgs/SetOut/SetErr
// in a t.Cleanup to prevent cross-test leakage.
func TestRootHelp_ListsSubcommands(t *testing.T) {
	var buf bytes.Buffer
	RootCmd.SetOut(&buf)
	RootCmd.SetErr(&buf)
	RootCmd.SetArgs([]string{"--help"})
	t.Cleanup(func() {
		RootCmd.SetArgs(nil)
		RootCmd.SetOut(nil)
		RootCmd.SetErr(nil)
	})

	err := RootCmd.Execute()
	if err != nil {
		t.Fatalf("RootCmd.Execute() --help: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Usage:") {
		t.Errorf("--help output does not contain 'Usage:'\noutput:\n%s", out)
	}
	// AC-1: all surviving v1 subcommand verbs must appear in the help output.
	for _, verb := range []string{"add", "adopt", "migrate", "index", "note", "query", "today", "todo"} {
		if !strings.Contains(out, verb) {
			t.Errorf("--help output missing verb %q\noutput:\n%s", verb, out)
		}
	}
}

// TestAddHelp (T-2 / AC-1): rk add --help must exit 0 and must not print
// "unknown command" (the stub command must be registered with its own usage text).
func TestAddHelp(t *testing.T) {
	var buf bytes.Buffer
	RootCmd.SetOut(&buf)
	RootCmd.SetErr(&buf)
	RootCmd.SetArgs([]string{"add", "--help"})
	t.Cleanup(func() {
		RootCmd.SetArgs(nil)
		RootCmd.SetOut(nil)
		RootCmd.SetErr(nil)
	})

	err := RootCmd.Execute()
	if err != nil {
		t.Fatalf("RootCmd.Execute() 'add --help': %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "unknown command") {
		t.Errorf("'add --help' output contains 'unknown command' — stub must be registered:\n%s", out)
	}
}

// TestRootHelp_MissingVaultOK (T-14 / EC-1): when RECKON_VAULT points to a
// non-existent path, rk --help must still exit 0. Vault absence must not break
// informational subcommands (the vault is stat-ed lazily by data commands only).
func TestRootHelp_MissingVaultOK(t *testing.T) {
	t.Setenv("RECKON_VAULT", "/nonexistent/path/that/does/not/exist/__reckon_test__")

	var buf bytes.Buffer
	RootCmd.SetOut(&buf)
	RootCmd.SetErr(&buf)
	RootCmd.SetArgs([]string{"--help"})
	t.Cleanup(func() {
		RootCmd.SetArgs(nil)
		RootCmd.SetOut(nil)
		RootCmd.SetErr(nil)
	})

	err := RootCmd.Execute()
	if err != nil {
		t.Fatalf("--help with missing vault: expected nil error, got: %v", err)
	}
}

// TestRootCommandSurface asserts the v0 DB-primary verb surface is retired.
// Assert over RootCmd.Commands() *names*, not rendered help text —
// dying verb names occur as substrings of surviving output (e.g. "log" in
// the --log-file/--log-level flags, "task" in RootCmd.Long, "notes" in
// noteCmd's Short/Long), so a substring-match assertion of absence would
// false-fail forever. Do not call RootCmd.Execute() here: cobra lazily
// registers help/completion inside Execute(), and RootCmd is a package-level
// global shared across this test binary, so an exact-set snapshot would be
// order-dependent on whichever tests already ran. Two-directional
// containment sidesteps both problems and needs no total-count assertion.
func TestRootCommandSurface(t *testing.T) {
	names := make(map[string]bool)
	for _, cmd := range RootCmd.Commands() {
		names[cmd.Name()] = true
	}

	survivors := []string{"add", "adopt", "migrate", "index", "note", "query", "today", "todo", "tui"}
	for _, verb := range survivors {
		if !names[verb] {
			t.Errorf("expected verb %q to be registered", verb)
		}
	}

	// tui was revived (reckon-fnqs.8) as a porcelain over the index + verbs;
	// it is no longer a dying verb (moved to survivors above).
	dying := []string{
		"log", "notes", "week", "rebuild", "review", "schedule", "task",
		"win", "checklist", "import", "summary",
	}
	for _, verb := range dying {
		if names[verb] {
			t.Errorf("dying verb %q is still registered", verb)
		}
	}
}

// TestNoteSubcommandSurface asserts rk note keeps only its v1 children.
// Same reasoning as TestRootCommandSurface — assert over noteCmd.Commands()
// names, not "rk note --help" text, since "notes"/"new"/"list" can appear
// as substrings of legitimate Short/Long descriptions.
func TestNoteSubcommandSurface(t *testing.T) {
	var noteCommand *cobra.Command
	for _, cmd := range RootCmd.Commands() {
		if cmd.Name() == "note" {
			noteCommand = cmd
			break
		}
	}
	if noteCommand == nil {
		t.Fatal("note command not found on RootCmd")
	}

	names := make(map[string]bool)
	for _, cmd := range noteCommand.Commands() {
		names[cmd.Name()] = true
	}

	survivors := []string{"create", "show", "rename", "index"}
	for _, verb := range survivors {
		if !names[verb] {
			t.Errorf("expected note subcommand %q to be registered", verb)
		}
	}

	dying := []string{"new", "list"}
	for _, verb := range dying {
		if names[verb] {
			t.Errorf("dying note subcommand %q is still registered", verb)
		}
	}
}
