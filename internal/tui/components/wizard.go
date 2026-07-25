package components

import tea "github.com/charmbracelet/bubbletea"

// wizardStep is the type-erased form of a Prompt[T] a Wizard step wraps.
// Steps are heterogeneous (each has its own T), so they cannot be stored in
// one []Prompt[T] slice for a single T -- this non-generic interface is
// what the Wizard actually holds and drives.
type wizardStep interface {
	Init() tea.Cmd
	Update(tea.Msg) (wizardStep, tea.Cmd)
	View() string
	Done() (finished, canceled bool)
	resultAny() any
	key() string
}

// erasedStep adapts one Prompt[T] into a wizardStep, remembering the key
// its result is filed under in the Wizard's shared result map.
type erasedStep[T any] struct {
	stepKey string
	p       Prompt[T]
}

func (s *erasedStep[T]) Init() tea.Cmd { return s.p.Init() }

func (s *erasedStep[T]) Update(msg tea.Msg) (wizardStep, tea.Cmd) {
	next, cmd := s.p.Update(msg)
	s.p = next
	return s, cmd
}

func (s *erasedStep[T]) View() string { return s.p.View() }

func (s *erasedStep[T]) Done() (finished, canceled bool) { return s.p.Done() }

func (s *erasedStep[T]) resultAny() any { return s.p.Result() }

func (s *erasedStep[T]) key() string { return s.stepKey }

// StepFactory builds one wizard step given the shared result map
// accumulated so far. It is the type-erased form Wizard stores; construct
// one via Step[T].
type StepFactory func(prior map[string]any) wizardStep

// Step wraps a typed factory func(prior) Prompt[T] into a StepFactory,
// erasing T so heterogeneous steps can share one []StepFactory. The
// factory closure is the priming point: every component primes its state
// via Show(...), not Init() (which bubbletea calls with no arguments), so a
// factory calls Show() itself and may read prior[...] to pre-fill state.
func Step[T any](key string, factory func(prior map[string]any) Prompt[T]) StepFactory {
	return func(prior map[string]any) wizardStep {
		return &erasedStep[T]{stepKey: key, p: factory(prior)}
	}
}

// Wizard chains heterogeneous Prompt[T] steps into one flow, collecting
// each step's result under its key in a shared map. It is itself a
// Prompt[map[string]any], so Wizard.Run drives it through the same
// RunPrompt host every single component uses -- exactly one tea.Program is
// ever opened.
type Wizard struct {
	factories []StepFactory
	results   map[string]any
	index     int
	active    wizardStep
	finished  bool
	canceled  bool
}

// NewWizard constructs a Wizard over steps. No step is mounted until Init().
func NewWizard(steps ...StepFactory) *Wizard {
	return &Wizard{
		factories: steps,
		results:   make(map[string]any),
	}
}

// Init mounts step 0 and returns its Init() cmd.
func (w *Wizard) Init() tea.Cmd {
	if len(w.factories) == 0 {
		w.finished = true
		return nil
	}
	w.index = 0
	w.active = w.factories[0](w.results)
	if w.active == nil {
		w.canceled = true
		return nil
	}
	return w.active.Init()
}

// Update delegates to the active step, then reacts to its terminal state:
// finished files the step's result under its key and advances (completing
// the whole Wizard if it was the last step); canceled at step 0 aborts the
// whole flow; canceled at step >0 re-mounts the prior step from its own
// factory, re-priming from the unchanged shared result map -- ESC-back
// reuses each step's own cancel signal rather than a new keybinding.
//
// On any transition (advance or back), the newly-mounted step's Init() is
// called explicitly and its cmd returned: bubbletea only calls the outer
// Wizard's own Init() once at Program start, it never re-fires for an inner
// step swapped in later.
func (w *Wizard) Update(msg tea.Msg) (Prompt[map[string]any], tea.Cmd) {
	if w.active == nil {
		return w, nil
	}

	next, cmd := w.active.Update(msg)
	w.active = next

	finished, canceled := w.active.Done()
	switch {
	case finished:
		w.results[w.active.key()] = w.active.resultAny()
		if w.index == len(w.factories)-1 {
			w.finished = true
			w.active = nil
			return w, nil
		}
		w.index++
		w.active = w.factories[w.index](w.results)
		if w.active == nil {
			w.finished = true
			return w, nil
		}
		return w, w.active.Init()

	case canceled:
		if w.index == 0 {
			w.canceled = true
			w.active = nil
			return w, nil
		}
		w.index--
		w.active = w.factories[w.index](w.results)
		if w.active == nil {
			w.canceled = true
			return w, nil
		}
		return w, w.active.Init()
	}

	return w, cmd
}

// View renders the active step, or an empty string if the flow has ended.
func (w *Wizard) View() string {
	if w.active == nil {
		return ""
	}
	return w.active.View()
}

// Result returns the shared map of every finished step's result, keyed by
// the name passed to Step[T].
func (w *Wizard) Result() map[string]any { return w.results }

// Done reports whether the whole flow has ended: finished once every step
// has submitted, canceled once step 0's own cancel signal fires.
func (w *Wizard) Done() (finished, canceled bool) { return w.finished, w.canceled }

// Run drives the Wizard through RunPrompt, the same single-Program host
// every other Prompt[T] uses.
func (w *Wizard) Run(opts ...tea.ProgramOption) (map[string]any, bool, error) {
	return RunPrompt[map[string]any](w, opts...)
}
