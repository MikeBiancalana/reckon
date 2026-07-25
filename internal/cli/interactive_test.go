package cli

import (
	"bytes"
	"strings"
	"testing"
)

// TestPromptGuard_NonTTYErrors (acceptance-criteria.md §4 scenario 11): with
// the isInteractive seam (mirrors todoNow at todo.go:33) stubbed to report a
// non-TTY, promptGuard returns an error naming --no-input.
func TestPromptGuard_NonTTYErrors(t *testing.T) {
	prevInteractive := isInteractive
	prevNoInput := noInputFlag
	t.Cleanup(func() {
		isInteractive = prevInteractive
		noInputFlag = prevNoInput
	})

	isInteractive = func() bool { return false }
	noInputFlag = false

	err := promptGuard()
	if err == nil {
		t.Fatal("expected promptGuard to return an error when stdin is not a TTY")
	}
	if !strings.Contains(err.Error(), "--no-input") {
		t.Errorf("expected error to mention --no-input, got: %v", err)
	}
}

// TestPromptGuard_NoInputFlagBeatsTTY (scenario 12): --no-input fires the
// same usage error even when isInteractive reports a real TTY — the flag
// beats TTY detection.
func TestPromptGuard_NoInputFlagBeatsTTY(t *testing.T) {
	prevInteractive := isInteractive
	prevNoInput := noInputFlag
	t.Cleanup(func() {
		isInteractive = prevInteractive
		noInputFlag = prevNoInput
	})

	isInteractive = func() bool { return true }
	noInputFlag = true

	err := promptGuard()
	if err == nil {
		t.Fatal("expected promptGuard to return an error when --no-input is set, even on a TTY")
	}
	if !strings.Contains(err.Error(), "--no-input") {
		t.Errorf("expected error to mention --no-input, got: %v", err)
	}
}

// TestPromptGuard_NotInvokedForNonPromptingVerb (scenario 13): --no-input
// passed to a verb that never calls RunPrompt/Wizard (rk --help) completes
// normally. The guard lives inside RunPrompt/Wizard.Run itself (plan.md /
// acceptance-criteria.md §2.7), not on RootCmd.PersistentPreRunE, so there is
// no verb-wide flag check to fire spuriously — and root.go's own
// PersistentPreRunE comment already documents that --help bypasses it
// entirely, so this also exercises pure flag registration/parsing.
func TestPromptGuard_NotInvokedForNonPromptingVerb(t *testing.T) {
	var buf bytes.Buffer
	RootCmd.SetOut(&buf)
	RootCmd.SetErr(&buf)
	RootCmd.SetArgs([]string{"--no-input", "--help"})
	t.Cleanup(func() {
		RootCmd.SetArgs(nil)
		RootCmd.SetOut(nil)
		RootCmd.SetErr(nil)
		noInputFlag = false
	})

	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("expected --no-input on a non-prompting verb (--help) to succeed, got: %v", err)
	}
}
