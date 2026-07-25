package components

import (
	"bytes"
	"io"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests assume the public surface sketched in plan.md's "Wizard:
// shared-result-map + ESC-back" section:
//
//	func NewWizard(steps ...StepFactory) *Wizard
//	func Step[T any](key string, factory func(prior map[string]any) Prompt[T]) StepFactory
//	func (w *Wizard) Run(opts ...tea.ProgramOption) (map[string]any, bool, error)
//
// Only Wizard, wizardStep, StepFactory, and Step[T] are named by the plan;
// NewWizard's variadic-constructor shape is chosen to match this package's
// existing NewForm/NewDatePicker/NewTaskPicker/... convention. Factories
// construct-and-configure a component (calling Show(), mirroring how every
// component already primes state — plan.md §2.5) rather than relying on
// Init()'s argument-less signature.

// TestWizard_ThreeStepsCollectAllResults (scenario 7): a Wizard chaining
// DatePicker -> Form -> TextEditor (3 distinct Prompt[T]s), driven end to
// end through the WithInput/WithOutput seam, collects all 3 steps' results
// under distinct keys and reports overall success. This also stands in for
// scenario 6 (reframed in plan.md: "Wizard opens no Program of its own" is
// proven by this test succeeding through exactly one Wizard.Run call, not a
// separate tea.NewProgram grep).
func TestWizard_ThreeStepsCollectAllResults(t *testing.T) {
	dateStr := futureDate()

	w := NewWizard(
		Step("when", func(prior map[string]any) Prompt[time.Time] {
			dp := NewDatePicker("When")
			dp.Show()
			return dp
		}),
		Step("details", func(prior map[string]any) Prompt[FormResult] {
			f := NewForm("Details")
			f.AddField(FormField{Label: "Title", Key: "title", Type: FieldTypeText, Required: true})
			f.Show()
			return f
		}),
		Step("notes", func(prior map[string]any) Prompt[string] {
			te := NewTextEditor("Notes")
			te.Show()
			return te
		}),
	)

	var keys bytes.Buffer
	keys.WriteString(dateStr)
	keys.WriteByte('\r') // submit "when"
	keys.WriteString("My Title")
	keys.WriteByte('\r') // submit "details"
	keys.WriteString("Some notes")
	keys.WriteByte(0x04) // Ctrl+D submits "notes"

	result, ok, err := runPromptForTest(t, func() (map[string]any, bool, error) {
		return w.Run(tea.WithInput(bytes.NewReader(keys.Bytes())), tea.WithOutput(io.Discard))
	})

	require.NoError(t, err)
	assert.True(t, ok)
	require.Len(t, result, 3)

	gotDate, isTime := result["when"].(time.Time)
	require.True(t, isTime, "expected result[\"when\"] to be time.Time, got %T", result["when"])
	assert.Equal(t, dateStr, gotDate.Format("2006-01-02"))

	gotForm, isForm := result["details"].(FormResult)
	require.True(t, isForm, "expected result[\"details\"] to be FormResult, got %T", result["details"])
	assert.Equal(t, "My Title", gotForm.Values["title"])

	gotNotes, isString := result["notes"].(string)
	require.True(t, isString, "expected result[\"notes\"] to be string, got %T", result["notes"])
	assert.Equal(t, "Some notes", gotNotes)
}

// TestWizard_EscMidFlowStepsBackKeepsResult (scenario 8): on step 2 of 3
// with step 1 already submitted, Esc steps back to step 1; that step's
// factory re-primes from the (unchanged) shared result map, so re-
// submitting WITHOUT retyping proves the prior value survived the round
// trip. If the factory failed to re-prime from prior, DatePicker's Update
// would reject the empty input on Enter (no submit signal — date_picker.go's
// existing "Please enter a date" branch), and the flow would never reach
// "details" a second time: a specific, legible failure rather than a silent
// pass.
func TestWizard_EscMidFlowStepsBackKeepsResult(t *testing.T) {
	dateStr := futureDate()

	w := NewWizard(
		Step("when", func(prior map[string]any) Prompt[time.Time] {
			dp := NewDatePicker("When")
			dp.Show()
			if v, ok := prior["when"].(time.Time); ok {
				dp.textInput.SetValue(v.Format("2006-01-02"))
			}
			return dp
		}),
		Step("details", func(prior map[string]any) Prompt[FormResult] {
			f := NewForm("Details")
			f.AddField(FormField{Label: "Title", Key: "title", Type: FieldTypeText, Required: true})
			f.Show()
			return f
		}),
		Step("notes", func(prior map[string]any) Prompt[string] {
			te := NewTextEditor("Notes")
			te.Show()
			return te
		}),
	)

	var keys bytes.Buffer
	keys.WriteString(dateStr)
	keys.WriteByte('\r') // submit "when" (step 1 of 3)
	keys.WriteByte(0x1b) // Esc at "details" (step 2 of 3) -> back to "when"
	keys.WriteByte('\r') // re-submit "when" without retyping (proves re-prime)
	keys.WriteString("Ada")
	keys.WriteByte('\r') // submit "details"
	keys.WriteString("Some notes")
	keys.WriteByte(0x04) // Ctrl+D submits "notes", completing the flow

	result, ok, err := runPromptForTest(t, func() (map[string]any, bool, error) {
		return w.Run(tea.WithInput(bytes.NewReader(keys.Bytes())), tea.WithOutput(io.Discard))
	})

	require.NoError(t, err)
	assert.True(t, ok)

	gotDate, isTime := result["when"].(time.Time)
	require.True(t, isTime, "expected result[\"when\"] to be time.Time, got %T", result["when"])
	assert.Equal(t, dateStr, gotDate.Format("2006-01-02"))

	gotForm, isForm := result["details"].(FormResult)
	require.True(t, isForm, "expected result[\"details\"] to be FormResult, got %T", result["details"])
	assert.Equal(t, "Ada", gotForm.Values["title"])

	gotNotes, isString := result["notes"].(string)
	require.True(t, isString, "expected result[\"notes\"] to be string, got %T", result["notes"])
	assert.Equal(t, "Some notes", gotNotes)
}

// TestWizard_EscAtStep0AbortsFlow (scenario 9): Esc at the first step
// aborts the whole flow — the Wizard-level run reports not-ok, and no
// step's result is committed anywhere.
func TestWizard_EscAtStep0AbortsFlow(t *testing.T) {
	w := NewWizard(
		Step("when", func(prior map[string]any) Prompt[time.Time] {
			dp := NewDatePicker("When")
			dp.Show()
			return dp
		}),
		Step("details", func(prior map[string]any) Prompt[FormResult] {
			f := NewForm("Details")
			f.AddField(FormField{Label: "Title", Key: "title", Type: FieldTypeText, Required: true})
			f.Show()
			return f
		}),
	)

	result, ok, err := runPromptForTest(t, func() (map[string]any, bool, error) {
		return w.Run(
			tea.WithInput(bytes.NewReader([]byte{0x1b})), // Esc at step 0
			tea.WithOutput(io.Discard),
		)
	})

	require.NoError(t, err)
	assert.False(t, ok)
	assert.Empty(t, result)
}

// TestWizard_FactoryStepSeesPriorResult (scenario 10): step 2's factory
// closure reads step 1's just-submitted result at mount time and pre-fills
// its own state. Submitting step 2 WITHOUT typing anything new proves the
// pre-fill happened at construction (closure-capture, plan.md's "Wizard"
// section / REVIEW_PATTERNS.md:117-144), not from user input.
func TestWizard_FactoryStepSeesPriorResult(t *testing.T) {
	w := NewWizard(
		Step("who", func(prior map[string]any) Prompt[FormResult] {
			f := NewForm("Who")
			f.AddField(FormField{Label: "Name", Key: "name", Type: FieldTypeText, Required: true})
			f.Show()
			return f
		}),
		Step("greeting", func(prior map[string]any) Prompt[string] {
			te := NewTextEditor("Greeting")
			te.Show()
			if who, ok := prior["who"].(FormResult); ok {
				te.SetText("Hello, " + who.Values["name"])
			}
			return te
		}),
	)

	var keys bytes.Buffer
	keys.WriteString("Ada")
	keys.WriteByte('\r') // submit "who"
	keys.WriteByte(0x04) // Ctrl+D submits "greeting" with no retyping

	result, ok, err := runPromptForTest(t, func() (map[string]any, bool, error) {
		return w.Run(tea.WithInput(bytes.NewReader(keys.Bytes())), tea.WithOutput(io.Discard))
	})

	require.NoError(t, err)
	assert.True(t, ok)

	got, isString := result["greeting"].(string)
	require.True(t, isString, "expected result[\"greeting\"] to be string, got %T", result["greeting"])
	assert.Equal(t, "Hello, Ada", got)
}
