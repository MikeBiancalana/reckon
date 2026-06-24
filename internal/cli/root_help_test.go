package cli

import (
	"bytes"
	"strings"
	"testing"
)

// TestRootHelp_ListsSubcommands (T-1 / AC-1): rk --help must exit 0, print
// "Usage:", and list at minimum the planned v1 stub subcommand verbs.
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
	// AC-1: all planned v1 stub subcommand verbs must appear in the help output.
	for _, verb := range []string{"add", "log", "todo", "note", "query", "today", "index"} {
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
