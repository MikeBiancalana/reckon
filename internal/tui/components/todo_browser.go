package components

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// TodoItem is the display row TodoBrowser renders and removes via
// MarkDoneFunc. Kind/Ref are opaque identity carried through for the CLI
// layer's dispatch closure -- this package must never import
// internal/index/internal/node, so it never interprets them itself.
type TodoItem struct {
	Kind  string // "durable" | "ephemeral" -- opaque to this package
	Ref   string // durable ULID, or ephemeral 1-based line index as a string -- opaque to this package
	Title string // render text (never empty; fallbacks applied at build time)
	Done  bool   // rendered checkbox state
}

// MarkDoneFunc marks the item at position (0-based, matching the slice
// index) done and returns the refreshed, shrunk item list. Unlike
// ChecklistRunner's ToggleFunc, this is one-directional -- a marked-done
// item is removed from the list, not flipped in place -- and carries no
// aggregate "browsing complete" signal.
type MarkDoneFunc func(position int) (remaining []TodoItem, err error)

// TodoBrowser is a single-screen Prompt[[]TodoItem]: move a cursor over a
// list of open todos and mark items done live through an injected callback.
// There is no terminal "finished" state -- every mark-done already persisted
// per-keypress, so the only way out is a quit key or a mid-session error.
type TodoBrowser struct {
	title      string
	items      []TodoItem
	cursor     int
	onMarkDone MarkDoneFunc
	canceled   bool
	err        error
}

// NewTodoBrowser creates a browser with the given header title. Show primes
// the session state before use.
func NewTodoBrowser(title string) *TodoBrowser {
	return &TodoBrowser{title: title}
}

// Show primes/resets the browser with the open items and the callback that
// persists a mark-done. Mirrors ChecklistRunner.Show as the reset point for
// per-session state.
func (r *TodoBrowser) Show(items []TodoItem, onMarkDone MarkDoneFunc) {
	r.items = items
	r.cursor = 0
	r.onMarkDone = onMarkDone
	r.canceled = false
	r.err = nil
}

// Init satisfies Prompt[[]TodoItem]; priming already happened in Show.
func (r *TodoBrowser) Init() tea.Cmd { return nil }

// Result returns the current (possibly shrunk) item list.
func (r *TodoBrowser) Result() []TodoItem { return r.items }

// Done reports whether the session has reached a terminal state. finished is
// always false -- there is no auto-completion state, unlike ChecklistRunner
// -- so the session only ends canceled: on a quit key, or on a mid-session
// mark-done error (the error itself is read separately via Err()).
func (r *TodoBrowser) Done() (finished, canceled bool) {
	return false, r.canceled || r.err != nil
}

// Err returns a mid-session MarkDoneFunc error, if one occurred. Not part of
// Prompt[T]: RunPrompt's own error return is reserved for host/guard
// failures, so a mark-done failure is surfaced here instead, read by the CLI
// after RunPrompt returns.
func (r *TodoBrowser) Err() error { return r.err }

// Update handles Bubble Tea messages.
func (r *TodoBrowser) Update(msg tea.Msg) (Prompt[[]TodoItem], tea.Cmd) {
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
		items, err := r.onMarkDone(pos)
		if err != nil {
			r.err = err
			break
		}
		r.items = items
		// Removal is always at the cursor position, so clamping the index
		// lands on the sensible next item without any ID-based re-location.
		if len(r.items) == 0 {
			r.cursor = 0
		} else if r.cursor > len(r.items)-1 {
			r.cursor = len(r.items) - 1
		}
	}

	return r, nil
}

// View renders the header, one line per item, and a footer that is either an
// error line or the help line -- never blank. Lines are collected into a
// slice and joined with strings.Join rather than concatenated with
// unconditional "\n" separators, so an absent optional line never leaves a
// phantom blank row.
func (r *TodoBrowser) View() string {
	lines := []string{fmt.Sprintf("%s  (%d)", r.title, len(r.items))}

	if len(r.items) == 0 {
		lines = append(lines, "(no items)")
	}
	for i, item := range r.items {
		prefix := "  "
		if i == r.cursor {
			prefix = "> "
		}
		mark := " "
		if item.Done {
			mark = "x"
		}
		title := item.Title
		if title == "" {
			title = "(untitled)"
		}
		lines = append(lines, fmt.Sprintf("%s[%s] %s", prefix, mark, title))
	}

	if r.err != nil {
		lines = append(lines, fmt.Sprintf("error: %v", r.err))
	} else {
		lines = append(lines, "j/k: move  space/enter: mark done  q: quit")
	}

	return strings.Join(lines, "\n")
}
