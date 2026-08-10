package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

// runEditor is the testable seam for shelling out to $EDITOR: production
// wires the subprocess to the real TTY (os.Stdin/Stdout/Stderr, not
// cmd.InOrStdin/OutOrStdout) so an interactive editor works normally; tests
// stub this var to write canned content, or return a canned error, without
// spawning a process.
var runEditor = func(editor, path string) error {
	c := exec.Command(editor, path)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

// isStdinDash reports whether args is exactly the single-element slice ["-"]
// -- the stdin-read sentinel. Two or more args, even if one is "-", do not
// trigger it (shared by assembleBody's dispatch and the ephemeral guard in
// runTodoAddE).
func isStdinDash(args []string) bool {
	return len(args) == 1 && args[0] == "-"
}

// assembleBody resolves the one active body-entry source among positional
// args, repeatable -m/--message values, --edit, and stdin via a lone "-" arg,
// and returns the fully-trimmed, multi-paragraph body text with no trailing
// newline. Combining more than one source is an error. Supplying none
// returns ("", nil), leaving the "body is empty" decision to the caller's
// existing empty-body guard.
//
// requireSubject gates a bespoke check on the -m path only: messages[0],
// individually trimmed, must be non-empty (rk todo add's subject rule). It
// does not apply to edit/stdin/positional, whose "first line non-empty"
// requirement is already implied by the caller's post-assembly empty-body
// check once TrimSpace has run.
//
// cmd.InOrStdin() is read only when the stdin source is the sole active one
// -- reading it unconditionally would block forever on a TTY for every other
// invocation shape.
func assembleBody(cmd *cobra.Command, args, messages []string, edit, requireSubject bool) (string, error) {
	hasStdin := isStdinDash(args)
	hasPositional := len(args) > 0 && !hasStdin
	hasMessages := len(messages) > 0

	active := 0
	for _, b := range []bool{hasStdin, hasPositional, hasMessages, edit} {
		if b {
			active++
		}
	}
	if active > 1 {
		return "", errors.New("choose one entry method (-m, --edit, stdin '-', or positional text)")
	}

	switch {
	case hasPositional:
		return strings.TrimSpace(strings.Join(args, " ")), nil

	case hasMessages:
		if requireSubject && strings.TrimSpace(messages[0]) == "" {
			return "", errors.New("subject (first -m) must not be empty")
		}
		trimmed := make([]string, len(messages))
		for i, m := range messages {
			trimmed[i] = strings.TrimSpace(m)
		}
		return strings.TrimSpace(strings.Join(trimmed, "\n\n")), nil

	case hasStdin:
		b, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		return strings.TrimSpace(string(b)), nil

	case edit:
		editor := strings.TrimSpace(os.Getenv("EDITOR"))
		if editor == "" {
			return "", errors.New("--edit requires $EDITOR to be set")
		}

		tmp, err := os.CreateTemp("", "rk-edit-*.md")
		if err != nil {
			return "", fmt.Errorf("create temp file: %w", err)
		}
		tmpPath := tmp.Name()
		defer os.Remove(tmpPath)
		if err := tmp.Close(); err != nil {
			return "", fmt.Errorf("close temp file: %w", err)
		}

		if err := runEditor(editor, tmpPath); err != nil {
			return "", fmt.Errorf("editor exited with an error: %w", err)
		}

		content, err := os.ReadFile(tmpPath)
		if err != nil {
			return "", fmt.Errorf("read edited file: %w", err)
		}
		return strings.TrimSpace(string(content)), nil

	default:
		return "", nil
	}
}

// joinSubjectBody applies the wizard-only subject/body convergence formula:
// a two-value counterpart to assembleBody's -m path, used by todo-add's and
// note-create's wizard conversion functions ONLY -- assembleBody itself
// (and its N-message join, body_entry.go:72-76) is untouched.
//
// The formula to replicate exactly: strings.TrimSpace(subject), then, if
// strings.TrimSpace(body) is non-empty, append "\n\n"+that trimmed body.
//
// NOT YET IMPLEMENTED: returns "" unconditionally so callers compile without
// this stub prematurely passing any convergence test.
func joinSubjectBody(subject, body string) string {
	return ""
}
