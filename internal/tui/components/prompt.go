package components

import tea "github.com/charmbracelet/bubbletea"

// Prompt is the shape a single interactive TUI component satisfies to be
// hostable by RunPrompt: each concrete component's own Update returns
// itself as Prompt[T] directly (no shared adapter type sits between them),
// so var _ Prompt[T] = (*Concrete)(nil) holds per component.
type Prompt[T any] interface {
	Init() tea.Cmd
	Update(tea.Msg) (Prompt[T], tea.Cmd)
	View() string

	// Result returns the committed value. Only meaningful once Done()
	// reports finished; callers must not read it beforehand.
	Result() T

	// Done reports whether the prompt has reached a terminal state: at
	// most one of finished/canceled is ever true at a time.
	Done() (finished, canceled bool)
}

// IndexRow is the display row both TaskPicker and NotePicker consume,
// decoupling the picker layer from any specific domain type. Props carries
// whatever per-picker display fields the caller assembles (e.g.
// "scheduled"/"deadline" for TaskPicker, "slug" for NotePicker).
type IndexRow struct {
	ID    string
	Title string
	Type  string
	Props map[string]string
}

// PromptGuard is checked by RunPrompt before it ever opens a tea.Program.
// nil (the default) means "allow" -- component unit tests need no setup.
// internal/cli wires this once at process entry so a non-interactive
// terminal or --no-input is caught before any RunPrompt/Wizard.Run call,
// without this package importing internal/cli.
var PromptGuard func() error

// runPromptHost adapts a Prompt[T] into the tea.Model RunPrompt drives: it
// delegates every message to p, then checks p.Done() -- finished captures
// Result() and quits, canceled quits leaving the zero value.
type runPromptHost[T any] struct {
	p      Prompt[T]
	result T
	ok     bool
}

func (h *runPromptHost[T]) Init() tea.Cmd {
	return h.p.Init()
}

func (h *runPromptHost[T]) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := h.p.Update(msg)
	h.p = next

	if finished, canceled := h.p.Done(); finished || canceled {
		if finished {
			h.result = h.p.Result()
			h.ok = true
		}
		return h, tea.Quit
	}

	return h, cmd
}

func (h *runPromptHost[T]) View() string {
	return h.p.View()
}

// RunPrompt drives p through the single tea.Program this function opens,
// blocking until p reports Done(). opts is empty in production; tests pass
// tea.WithInput/tea.WithOutput to inject a scripted keystroke reader in
// place of the real (non-TTY-safe-under-test) stdin -- without that seam, a
// test driving a Program would hang or fail setting raw mode on a non-TTY.
func RunPrompt[T any](p Prompt[T], opts ...tea.ProgramOption) (result T, ok bool, err error) {
	if PromptGuard != nil {
		if guardErr := PromptGuard(); guardErr != nil {
			var zero T
			return zero, false, guardErr
		}
	}

	host := &runPromptHost[T]{p: p}
	finalModel, err := tea.NewProgram(host, opts...).Run()
	if err != nil {
		var zero T
		return zero, false, err
	}

	final := finalModel.(*runPromptHost[T])
	if !final.ok {
		var zero T
		return zero, false, nil
	}
	return final.result, true, nil
}
