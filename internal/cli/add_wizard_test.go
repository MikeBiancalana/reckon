// Tests for `rk add`'s quick-capture wizard dispatch. Unlike todo add/note
// create, `rk add`'s wizard is a single TextPrompt driven through
// components.RunPrompt directly -- not a Wizard -- so there is no
// map[string]any result map or Wizard-composition test analogous to the
// other two verbs.
package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ─────────────────────────────────────────────────────────────────────────────
// Dispatch predicate: addWantsTUI's flag-set gating.
// ─────────────────────────────────────────────────────────────────────────────

// TestAddWantsTUI_TrueOnBareInteractive: bare invocation on a reported real
// TTY must route to the wizard.
func TestAddWantsTUI_TrueOnBareInteractive(t *testing.T) {
	prevInteractive := isInteractive
	t.Cleanup(func() {
		isInteractive = prevInteractive
		resetAddFlags(addCmd)
	})
	isInteractive = func() bool { return true }

	if !addWantsTUI(addCmd, nil) {
		t.Error("addWantsTUI(bare, TTY) = false, want true")
	}
}

// TestAddWantsTUI_FalseWhenArgsPresent: any positional arg routes classic
// regardless of TTY.
func TestAddWantsTUI_FalseWhenArgsPresent(t *testing.T) {
	prevInteractive := isInteractive
	t.Cleanup(func() { isInteractive = prevInteractive })
	isInteractive = func() bool { return true }

	if addWantsTUI(addCmd, []string{"hello"}) {
		t.Error("addWantsTUI(args present) = true, want false")
	}
}

// TestAddWantsTUI_FalseWhenNonInteractive: a reported non-TTY routes classic
// even with zero args.
func TestAddWantsTUI_FalseWhenNonInteractive(t *testing.T) {
	prevInteractive := isInteractive
	t.Cleanup(func() { isInteractive = prevInteractive })
	isInteractive = func() bool { return false }

	if addWantsTUI(addCmd, nil) {
		t.Error("addWantsTUI(non-interactive) = true, want false")
	}
}

// TestAddWantsTUI_FalseWhenInputFlagChanged: each of resetAddFlags's own
// input-affecting flag names (author/at/message/edit, add.go:60),
// individually Changed, routes classic even on a reported TTY with zero
// args.
func TestAddWantsTUI_FalseWhenInputFlagChanged(t *testing.T) {
	prevInteractive := isInteractive
	t.Cleanup(func() { isInteractive = prevInteractive })
	isInteractive = func() bool { return true }

	cases := []struct{ name, value string }{
		{"author", "ada"},
		{"at", "09:15"},
		{"message", "hello"},
		{"edit", "true"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() { resetAddFlags(addCmd) })
			mustSetFlag(t, addCmd, tc.name, tc.value)
			if addWantsTUI(addCmd, nil) {
				t.Errorf("addWantsTUI = true with --%s Changed, want false", tc.name)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// CLI dispatch integration, at the RootCmd.Execute level.
// ─────────────────────────────────────────────────────────────────────────────

// TestAddCmd_AtFlagStaysClassicNoANSI: --at always routes classic, never
// opening the TUI.
func TestAddCmd_AtFlagStaysClassicNoANSI(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	prevInteractive := isInteractive
	t.Cleanup(func() { isInteractive = prevInteractive })
	isInteractive = func() bool { return true }

	out, stderr, err := runAdd(t, vault, "hello", "--at", "14:30")
	if err != nil {
		t.Fatalf("rk add --at: %v\nstderr: %s", err, stderr)
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("--at invocation must never open the TUI, got %q", out)
	}
}

// TestAddCmd_InteractiveNoInputReachesPromptGuard: a reported TTY with
// --no-input must error via components.PromptGuard's message, proving the
// guard is reached rather than bypassed.
func TestAddCmd_InteractiveNoInputReachesPromptGuard(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	prevInteractive := isInteractive
	prevNoInput := noInputFlag
	t.Cleanup(func() {
		isInteractive = prevInteractive
		noInputFlag = prevNoInput
	})
	isInteractive = func() bool { return true }

	_, stderr, err := runAdd(t, vault, "--no-input")
	if err == nil {
		t.Fatal("expected an error opening the prompt on a reported TTY with --no-input, got nil")
	}
	combined := strings.ToLower(err.Error() + stderr)
	if !strings.Contains(combined, "--no-input") && !strings.Contains(combined, "terminal") {
		t.Errorf("expected the guard's --no-input/terminal error once dispatch is wired, got err=%v stderr=%q", err, stderr)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Pure conversion: wizardAddBody.
// ─────────────────────────────────────────────────────────────────────────────

// TestWizardAddBody_TrimsWhitespace: the captured line is trimmed, matching
// requireSubject=false's convergence formula (no subject/body split for
// this verb).
func TestWizardAddBody_TrimsWhitespace(t *testing.T) {
	got := wizardAddBody("  quick note  ")
	if got != "quick note" {
		t.Errorf("wizardAddBody(...) = %q, want %q", got, "quick note")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Convergence: flag path vs. wizard-conversion-fed appendLogEntry.
// ─────────────────────────────────────────────────────────────────────────────

// TestAddWizard_FileConvergence_FlagVsWizard: the classic flag-driven path
// and the wizard conversion path, given the same logical capture text, must
// produce entries with identical Body/Author fields -- timestamp-dependent
// fields (id/time, which mint independently per invocation) are
// deliberately not compared.
func TestAddWizard_FileConvergence_FlagVsWizard(t *testing.T) {
	flagVault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	if _, stderr, err := runAdd(t, flagVault, "quick note"); err != nil {
		t.Fatalf("flag-path rk add: %v\nstderr: %s", err, stderr)
	}
	today := utcToday()
	flagEntries := parseLogDayFile(t, flagVault, today)[1:]
	if len(flagEntries) != 1 {
		t.Fatalf("flag path: want 1 entry, got %d", len(flagEntries))
	}
	resetCLIFlags()

	// Wizard path: synthetic capture -> wizardAddBody -> appendLogEntry
	// directly (the same converge-point function the classic path calls).
	body := wizardAddBody("  quick note  ")
	author := resolveAuthor("")
	hhmm, err := resolveAtTime("")
	if err != nil {
		t.Fatalf("resolveAtTime: %v", err)
	}
	day, err := effectiveLogDate()
	if err != nil {
		t.Fatalf("effectiveLogDate: %v", err)
	}

	wizVault, _ := setupQueryVault(t)
	wizLogDir := filepath.Join(wizVault, "log")
	if err := os.MkdirAll(wizLogDir, 0o755); err != nil {
		t.Fatalf("mkdir wizard log dir: %v", err)
	}
	if _, err := appendLogEntry(wizLogDir, day, hhmm, author, body); err != nil {
		t.Fatalf("appendLogEntry (wizard path): %v", err)
	}
	wizEntries := parseLogDayFile(t, wizVault, day)[1:]
	if len(wizEntries) != 1 {
		t.Fatalf("wizard path: want 1 entry, got %d", len(wizEntries))
	}

	if wizEntries[0].Body != flagEntries[0].Body {
		t.Errorf("Body = %q, want %q", wizEntries[0].Body, flagEntries[0].Body)
	}
	if wizEntries[0].Author != flagEntries[0].Author {
		t.Errorf("Author = %q, want %q", wizEntries[0].Author, flagEntries[0].Author)
	}
}
