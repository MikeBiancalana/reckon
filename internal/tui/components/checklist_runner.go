package components

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// ChecklistItem is the display row ChecklistRunner renders and mutates via
// ToggleFunc. It carries only what the TUI needs to draw a checkbox line --
// this package must never import internal/checklist.
type ChecklistItem struct {
	Text    string
	Checked bool
}

// ToggleFunc toggles the item at position (0-based, matching the slice
// index -- Service-fetched runs keep Items[i].Position == i) and returns the
// refreshed item list plus whether the run just auto-completed. The CLI
// layer wires this to Service.CheckItem followed by Service.GetRunStatus;
// ChecklistRunner calls it synchronously on every toggle keypress and never
// interprets position beyond passing the cursor through.
type ToggleFunc func(position int) (items []ChecklistItem, completed bool, err error)

// ChecklistRunner is a single-screen Prompt[[]ChecklistItem]: move a cursor
// over a checklist, toggle items live through an injected callback, and
// auto-quit once the callback reports completion.
type ChecklistRunner struct {
	title     string
	items     []ChecklistItem
	cursor    int
	onToggle  ToggleFunc
	completed bool
	canceled  bool
	err       error
}

// NewChecklistRunner creates a runner with the given header title (the
// template name). Show primes the run state before use.
func NewChecklistRunner(title string) *ChecklistRunner {
	return &ChecklistRunner{title: title}
}

// Show primes/resets the runner with a run's items and the callback that
// persists a toggle. Mirrors TaskPicker.Show as the reset point for
// per-session state.
func (r *ChecklistRunner) Show(items []ChecklistItem, onToggle ToggleFunc) {
	r.items = items
	r.cursor = 0
	r.onToggle = onToggle
	r.completed = false
	r.canceled = false
	r.err = nil
}

// Init satisfies Prompt[[]ChecklistItem]; priming already happened in Show.
func (r *ChecklistRunner) Init() tea.Cmd { return nil }

// Result returns the current items. Only meaningful once Done() reports
// finished.
func (r *ChecklistRunner) Result() []ChecklistItem { return r.items }

// Done reports whether the session has reached a terminal state: finished
// on auto-completion, canceled on a quit key or a mid-session toggle error
// (the error itself is read separately via Err()).
func (r *ChecklistRunner) Done() (finished, canceled bool) {
	return r.completed, r.canceled || r.err != nil
}

// Err returns a mid-session ToggleFunc error, if one occurred. Not part of
// Prompt[T]: RunPrompt's own error return is reserved for host/guard
// failures, so a toggle failure is surfaced here instead, read by the CLI
// after RunPrompt returns.
func (r *ChecklistRunner) Err() error { return r.err }

// Update handles keystrokes: q/esc/ctrl+c cancel; j/down and k/up move the
// cursor with no wraparound; space/enter toggle the item under the cursor.
// Navigation and toggle are no-ops on an empty checklist.
func (r *ChecklistRunner) Update(msg tea.Msg) (Prompt[[]ChecklistItem], tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return r, nil
	}

	switch keyMsg.String() {
	case "q", "esc", "ctrl+c":
		r.canceled = true
	case "j", "down":
		if len(r.items) > 0 && r.cursor < len(r.items)-1 {
			r.cursor++
		}
	case "k", "up":
		if len(r.items) > 0 && r.cursor > 0 {
			r.cursor--
		}
	case " ", "enter":
		if len(r.items) == 0 {
			break
		}
		pos := r.cursor
		items, completed, err := r.onToggle(pos)
		if err != nil {
			r.err = err
			break
		}
		r.items = items
		r.completed = completed
	}

	return r, nil
}

// View renders the header, one line per item, and a footer that is either
// the completion banner, an error line, or the help line -- never blank.
// Lines are collected into a slice and joined with strings.Join rather than
// concatenated with unconditional "\n" separators, so an absent optional
// line never leaves a phantom blank row.
func (r *ChecklistRunner) View() string {
	checked := 0
	for _, item := range r.items {
		if item.Checked {
			checked++
		}
	}

	lines := []string{fmt.Sprintf("%s  [%d/%d]", r.title, checked, len(r.items))}

	if len(r.items) == 0 {
		lines = append(lines, "(no items)")
	}
	for i, item := range r.items {
		prefix := "  "
		if i == r.cursor {
			prefix = "> "
		}
		mark := " "
		if item.Checked {
			mark = "x"
		}
		lines = append(lines, fmt.Sprintf("%s[%s] %s", prefix, mark, item.Text))
	}

	switch {
	case r.completed:
		// Trailing blank element renders as a trailing newline so
		// bubbletea's non-alt-screen renderer doesn't erase this final
		// frame on exit.
		lines = append(lines, "✓ Complete!", "")
	case r.err != nil:
		lines = append(lines, fmt.Sprintf("error: %v", r.err))
	default:
		lines = append(lines, "j/k: move  space/enter: toggle  q: quit")
	}

	return strings.Join(lines, "\n")
}
