package cli

import (
	"fmt"
	"os"

	"github.com/MikeBiancalana/reckon/internal/tui/components"
)

// isInteractive reports whether both stdin and stdout are attached to a
// real terminal. Bubbletea's read loop hangs indefinitely on piped or
// redirected stdin, so stdin is the fatal check; stdout is checked too
// since rendering TUI escape sequences into a non-terminal stream produces
// garbled output. A package-level var (mirrors todoNow, todo.go) so tests
// can stub it -- os.Stat can't be faked directly.
var isInteractive = func() bool {
	return isCharDevice(os.Stdin) && isCharDevice(os.Stdout)
}

func isCharDevice(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// promptGuard is wired into components.PromptGuard (init below) so the
// check runs inside RunPrompt/Wizard.Run itself, not on every command via
// RootCmd.PersistentPreRunE -- most verbs never prompt and shouldn't pay
// for or be blocked by a check that doesn't apply to them.
func promptGuard() error {
	if noInputFlag || !isInteractive() {
		return fmt.Errorf("cannot show an interactive prompt: pass --no-input or run from an interactive terminal")
	}
	return nil
}

func init() {
	components.PromptGuard = promptGuard
}
