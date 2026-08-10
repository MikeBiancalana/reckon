package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/output"
	"github.com/MikeBiancalana/reckon/internal/tui/components"
	"github.com/spf13/cobra"
)

// addWantsTUI reports whether a bare `rk add` invocation should open the
// interactive quick-capture prompt instead of the classic flag-driven path:
// only on a real TTY, only with no positional args, and only when none of
// the add input flags (--author/--at/-m/--edit, mirrors resetAddFlags's own
// list, add.go:60) has been Changed. --no-input is deliberately NOT
// consulted here -- see todoAddWantsTUI's doc comment for the identical
// rationale.
func addWantsTUI(cmd *cobra.Command, args []string) bool {
	if !isInteractive() {
		return false
	}
	if len(args) > 0 {
		return false
	}
	for _, name := range []string{"author", "at", "message", "edit"} {
		if cmd.Flags().Changed(name) {
			return false
		}
	}
	return true
}

// runAddWizard drives a single TextPrompt (Required=false) through
// components.RunPrompt -- not a Wizard, since there's only one field, the
// captured line -- and, on submit, calls resolveAuthor/effectiveLogDate/
// resolveAtTime/appendLogEntry exactly as runAddE's own tail, with
// body = wizardAddBody(capturedLine).
func runAddWizard(cmd *cobra.Command) error {
	tp := components.NewTextPrompt("Quick capture", false)
	tp.Show()

	capture, ok, err := components.RunPrompt[string](tp)
	if err != nil {
		return fmt.Errorf("add: %w", err)
	}
	if !ok {
		return nil
	}

	author := resolveAuthor("")
	if embeddedHeaderRe.MatchString(author) {
		return fmt.Errorf(`add: author must not contain a line starting with "## " (would be mis-split as a new entry)`)
	}

	body := wizardAddBody(capture)
	if body == "" {
		return fmt.Errorf("add: empty body text")
	}
	if embeddedHeaderRe.MatchString(body) {
		return fmt.Errorf(`add: body must not contain a line starting with "## " (would be mis-split as a new entry)`)
	}

	mode, err := output.ModeFromFlags(jsonFlag, ndjsonFlag)
	if err != nil {
		return err
	}

	day, err := effectiveLogDate()
	if err != nil {
		return fmt.Errorf("add: %w", err)
	}

	hhmm, err := resolveAtTime("")
	if err != nil {
		return fmt.Errorf("add: %w", err)
	}

	cfg, err := config.LoadWithOverrides(vaultFlag, "")
	if err != nil {
		return fmt.Errorf("add: load config: %w", err)
	}

	logDir := filepath.Join(cfg.VaultDir, "log")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("add: create log dir: %w", err)
	}

	res, err := appendLogEntry(logDir, day, hhmm, author, body)
	if err != nil {
		return err
	}

	if !(mode == output.Pretty && quietFlag) {
		if err := output.New(cmd.OutOrStdout(), mode).Print(res); err != nil {
			return err
		}
	}
	return nil
}

// wizardAddBody applies rk add's quick-capture convergence formula: the
// captured line, trimmed -- mirrors the positional-args branch of
// assembleBody (requireSubject=false, body_entry.go:65-66), with no
// subject/body split.
func wizardAddBody(capture string) string {
	return strings.TrimSpace(capture)
}
