package cli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
)

// TestFindExternal_Found (IR-1): a tempdir containing an executable 'rk-foo'
// prepended to PATH must cause findExternal("foo") to return the full path.
func TestFindExternal_Found(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "rk-foo")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho hello\n"), 0755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// Prepend dir so rk-foo is discoverable before system binaries.
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	got, err := findExternal("foo")
	if err != nil {
		t.Fatalf("findExternal(%q): unexpected error: %v", "foo", err)
	}
	if got != bin {
		t.Errorf("findExternal(%q) = %q, want %q", "foo", got, bin)
	}
}

// TestFindExternal_NotFound (EC-2 / IR-1): when no rk-<name> exists on PATH,
// findExternal must return errExtNotFound (not a generic OS error).
func TestFindExternal_NotFound(t *testing.T) {
	dir := t.TempDir()
	// Override PATH to a directory that has no binaries at all.
	t.Setenv("PATH", dir)

	_, err := findExternal("nosuchcommand")
	if err == nil {
		t.Fatal("findExternal: expected error for missing rk-nosuchcommand, got nil")
	}
	if !errors.Is(err, errExtNotFound) {
		t.Errorf("findExternal (not found): error = %v; want errors.Is(err, errExtNotFound)", err)
	}
}

// TestFindExternal_NotExecutable (EC-3 / IR-1): when rk-foo exists on PATH but its
// mode is 0644 (not executable), findExternal must return a non-nil error that is
// NOT errExtNotFound — this is the "found but not executable" case distinct from
// "not found at all" (EC-2).
func TestFindExternal_NotExecutable(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "rk-foo")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho hello\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	_, err := findExternal("foo")
	if err == nil {
		t.Fatal("findExternal: expected error for non-executable rk-foo, got nil")
	}
	if errors.Is(err, errExtNotFound) {
		t.Errorf("findExternal (not executable): error must NOT be errExtNotFound (EC-3); got %v", err)
	}
}

// TestFirstNonFlag (IR-1): firstNonFlag must return the first positional argument,
// correctly skipping flag names and their consumed values in all standard forms.
//
// The FlagSet mirrors the planned root persistent flags that firstNonFlag must
// understand: --vault (string, consumes next arg), --json (bool), --ndjson (bool),
// -q/--quiet (bool). Using a local FlagSet makes the test self-contained and
// independent of cobra's lazy persistent-flag-merge timing.
func TestFirstNonFlag(t *testing.T) {
	fs := pflag.NewFlagSet("rk", pflag.ContinueOnError)
	fs.String("vault", "", "vault dir")         // non-bool: next arg is its value
	fs.Bool("json", false, "json output")        // bool: never consumes a following arg
	fs.Bool("ndjson", false, "ndjson output")    // bool
	fs.BoolP("quiet", "q", false, "quiet mode") // bool with shorthand -q

	tests := []struct {
		name string
		args []string
		want string
	}{
		// T-plan: {"--vault","/x","foo"} → "foo"  (value-taking flag skips two tokens)
		{"vault value then verb", []string{"--vault", "/x", "foo"}, "foo"},
		// T-plan: {"--json","foo"} → "foo"  (bool flag skips only itself)
		{"bool flag then verb", []string{"--json", "foo"}, "foo"},
		// T-plan: {"foo"} → "foo"  (bare verb — no flags)
		{"bare verb", []string{"foo"}, "foo"},
		// T-plan: {"--vault=/x","foo"} → "foo"  (--key=value form, one token)
		{"vault equals form", []string{"--vault=/x", "foo"}, "foo"},
		// T-plan: {"-q","foo"} → "foo"  (shorthand bool flag)
		{"shorthand quiet flag", []string{"-q", "foo"}, "foo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := firstNonFlag(tt.args, fs)
			if got != tt.want {
				t.Errorf("firstNonFlag(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}
