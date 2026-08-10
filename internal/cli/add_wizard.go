package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// addWantsTUI reports whether a bare `rk add` invocation should open the
// interactive quick-capture prompt instead of the classic flag-driven path:
// only on a real TTY, only with no positional args, and only when none of
// the add input flags (--author/--at/-m/--edit, mirrors resetAddFlags's own
// list, add.go:60) has been Changed. --no-input is deliberately NOT
// consulted here -- see todoAddWantsTUI's doc comment for the identical
// rationale.
//
// NOT YET IMPLEMENTED: returns false unconditionally.
func addWantsTUI(cmd *cobra.Command, args []string) bool {
	return false
}

// runAddWizard drives a single TextPrompt (Required=false) through
// components.RunPrompt -- not a Wizard, since there's only one field, the
// captured line -- and, on submit, calls resolveAuthor/effectiveLogDate/
// resolveAtTime/appendLogEntry exactly as runAddE's own tail, with
// body = wizardAddBody(capturedLine).
//
// NOT YET IMPLEMENTED: not wired from add.go's RunE yet, and this stub does
// not construct or run a real Prompt.
func runAddWizard(cmd *cobra.Command) error {
	return fmt.Errorf("add: interactive prompt not implemented")
}

// wizardAddBody applies rk add's quick-capture convergence formula: the
// captured line, trimmed -- mirrors the positional-args branch of
// assembleBody (requireSubject=false, body_entry.go:65-66), with no
// subject/body split.
//
// NOT YET IMPLEMENTED: returns capture unchanged (untrimmed), so a test
// asserting the trim actually happened fails on padded input.
func wizardAddBody(capture string) string {
	return capture
}
