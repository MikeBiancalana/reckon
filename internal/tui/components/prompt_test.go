package components

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time conformance assertions (acceptance-criteria.md §4 scenario 1):
// each of the 5 named components must satisfy Prompt[T] for its own result
// type via its own concrete Update method (Fork A option (a) — plan.md's
// "Summary of approach" section resolves the (a)/(b) fork this way because
// only (a) makes this exact assertion shape compile per component).
var (
	_ Prompt[FormResult] = (*Form)(nil)
	_ Prompt[string]     = (*TextEditor)(nil)
	_ Prompt[time.Time]  = (*DatePicker)(nil)
	_ Prompt[string]     = (*TaskPicker)(nil)
	_ Prompt[string]     = (*NotePicker)(nil)
)

// TestPrompt_ConformanceCompiles exists so scenario 1 has a runnable test
// function to report; the actual assertion is the package-level var block
// above, which only compiles once every component's Update signature
// satisfies Prompt[T] for its own result type.
func TestPrompt_ConformanceCompiles(t *testing.T) {}

// runPromptForTest drives a RunPrompt/Wizard.Run-shaped call in a goroutine
// bounded by a generous timeout. Without this, a wrong keystroke sequence in
// a test (or a host that never reaches Done()) would block on the real
// tea.Program's input loop and hang `go test` indefinitely — precisely the
// hang class this ticket's guard exists to prevent, resurfacing on the test
// side if driving the WithInput/WithOutput seam goes wrong.
func runPromptForTest[T any](t *testing.T, fn func() (T, bool, error)) (T, bool, error) {
	t.Helper()

	type res struct {
		val T
		ok  bool
		err error
	}
	done := make(chan res, 1)
	go func() {
		val, ok, err := fn()
		done <- res{val, ok, err}
	}()

	select {
	case r := <-done:
		return r.val, r.ok, r.err
	case <-time.After(5 * time.Second):
		t.Fatal("prompt run did not complete within timeout — check the driven keystroke sequence")
		var zero T
		return zero, false, nil
	}
}

// TestRunPrompt_FormSubmitReturnsValues (scenario 4): a Form primed with all
// required fields filled validly, driven through the WithInput/WithOutput
// I/O seam (plan.md's "RunPrompt[T] host" section) with a single Enter
// keystroke, submits — RunPrompt returns the matching values.
func TestRunPrompt_FormSubmitReturnsValues(t *testing.T) {
	form := NewForm("Test Form")
	form.AddField(FormField{Label: "Name", Key: "name", Type: FieldTypeText, Required: true})
	form.Show()
	form.SetValues(map[string]string{"name": "Ada Lovelace"})

	result, ok, err := runPromptForTest(t, func() (FormResult, bool, error) {
		return RunPrompt[FormResult](form,
			tea.WithInput(bytes.NewReader([]byte{'\r'})), // Enter
			tea.WithOutput(io.Discard),
		)
	})

	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "Ada Lovelace", result.Values["name"])
}

// TestRunPrompt_EscReturnsZeroNotOK (scenario 5, reframed per plan.md's
// "Test scenarios" section: a behavioral zero/ok assertion rather than a
// Result()-never-invoked spy, since the concrete type has no such hook).
// Esc, driven through the same seam, yields the zero value and ok=false.
func TestRunPrompt_EscReturnsZeroNotOK(t *testing.T) {
	dp := NewDatePicker("Test")
	dp.Show()

	result, ok, err := runPromptForTest(t, func() (time.Time, bool, error) {
		return RunPrompt[time.Time](dp,
			tea.WithInput(bytes.NewReader([]byte{0x1b})), // lone Esc
			tea.WithOutput(io.Discard),
		)
	})

	require.NoError(t, err)
	assert.False(t, ok)
	assert.True(t, result.IsZero(), "expected zero-value time.Time on cancellation, got %v", result)
}

// TestRunPrompt_GuardBlocksBeforeProgram: PromptGuard (the nil-by-default
// hook wired from internal/cli — plan.md's "TTY guard" section) must
// short-circuit RunPrompt before tea.NewProgram ever runs. Bounded by
// runPromptForTest's timeout so a guard that forgets to short-circuit fails
// fast instead of hanging on stdin/the injected reader.
func TestRunPrompt_GuardBlocksBeforeProgram(t *testing.T) {
	prevGuard := PromptGuard
	t.Cleanup(func() { PromptGuard = prevGuard })

	guardErr := errors.New("no-input: pass --no-input or run from an interactive terminal")
	PromptGuard = func() error { return guardErr }

	dp := NewDatePicker("Test")
	dp.Show()

	result, ok, err := runPromptForTest(t, func() (time.Time, bool, error) {
		return RunPrompt[time.Time](dp,
			tea.WithInput(bytes.NewReader(nil)),
			tea.WithOutput(io.Discard),
		)
	})

	require.ErrorIs(t, err, guardErr)
	assert.False(t, ok)
	assert.True(t, result.IsZero())
}
