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
// interactive quick-capture prompt instead of the classic flag-driven path.
// See wantsWizardTUI (todo_add_wizard.go) for the shared shape and the
// --no-input rationale. The flag list is resetAddFlags's own list
// (--author/--at/-m/--edit) plus --date, the global persistent flag
// effectiveLogDate reads (--date isn't one of resetAddFlags's own flags,
// since it isn't registered on addCmd itself, but effectiveLogDate() still
// consults it, so a caller who set it wants the classic path).
func addWantsTUI(cmd *cobra.Command, args []string) bool {
	return wantsWizardTUI(cmd, args, []string{"author", "at", "message", "edit", "date"})
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
// assembleBody (requireSubject=false), with no subject/body split.
func wizardAddBody(capture string) string {
	return strings.TrimSpace(capture)
}
