package components

import (
	"bytes"
	"errors"
	"io"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ChecklistRunner must satisfy Prompt[[]ChecklistItem] like every other
// component in this package, so it can be hosted by RunPrompt without a
// bespoke adapter.
var _ Prompt[[]ChecklistItem] = (*ChecklistRunner)(nil)

// TestChecklistRunner_ConformanceCompiles gives the var block above a
// runnable test to report; the assertion is the compile-time check itself.
func TestChecklistRunner_ConformanceCompiles(t *testing.T) {}

// fakeToggler stands in for the real Service.CheckItem/GetRunStatus round
// trip: it records every position it is called with (so cursor movement can
// be proven to reach the right item, not just "some" item) and mutates its
// own in-memory copy of the items, mirroring how the real callback re-fetches
// and returns the updated run state.
type fakeToggler struct {
	items []ChecklistItem
	calls []int
	err   error
	// completeOn is the zero-based call index (into calls) that reports
	// completed=true. -1 means never report completion.
	completeOn int
}

func newFakeToggler(items []ChecklistItem) *fakeToggler {
	cp := make([]ChecklistItem, len(items))
	copy(cp, items)
	return &fakeToggler{items: cp, completeOn: -1}
}

func (f *fakeToggler) toggle(position int) ([]ChecklistItem, bool, error) {
	f.calls = append(f.calls, position)
	if f.err != nil {
		return nil, false, f.err
	}
	f.items[position].Checked = !f.items[position].Checked
	out := make([]ChecklistItem, len(f.items))
	copy(out, f.items)
	completed := f.completeOn >= 0 && len(f.calls)-1 == f.completeOn
	return out, completed, nil
}

// singleByteReader delivers at most one byte per Read call. bubbletea's
// input decoder parses everything returned by a single Read as one input
// chunk, coalescing consecutive plain runes (e.g. "jjj") into one multi-rune
// KeyMsg instead of three separate keypresses. Repeated identical
// navigation keys need to register as distinct events, so tests route their
// scripted bytes through this reader rather than handing bytes.Reader to
// tea.WithInput directly.
type singleByteReader struct {
	r io.Reader
}

func (s singleByteReader) Read(p []byte) (int, error) {
	if len(p) > 1 {
		p = p[:1]
	}
	return s.r.Read(p)
}

// runChecklistRunner drives runner through RunPrompt with the given raw key
// bytes (one KeyMsg per byte, see singleByteReader), bounded by
// runPromptForTest's timeout so a wrong or missing quit key fails the test
// instead of hanging it.
func runChecklistRunner(t *testing.T, runner *ChecklistRunner, keys []byte) ([]ChecklistItem, bool, error) {
	t.Helper()
	return runPromptForTest(t, func() ([]ChecklistItem, bool, error) {
		return RunPrompt[[]ChecklistItem](runner,
			tea.WithInput(singleByteReader{bytes.NewReader(keys)}),
			tea.WithOutput(io.Discard),
		)
	})
}

// TestChecklistRunner_FreshRunNavigateAndToggle proves the cursor position at
// the time Space is pressed -- not just the fact of a toggle -- is what
// reaches the callback: one down-move then Space must call the fake with
// position 1, not 0.
func TestChecklistRunner_FreshRunNavigateAndToggle(t *testing.T) {
	items := []ChecklistItem{{Text: "one"}, {Text: "two"}, {Text: "three"}}
	fake := newFakeToggler(items)

	runner := NewChecklistRunner("Morning")
	runner.Show(items, fake.toggle)

	_, ok, err := runChecklistRunner(t, runner, []byte{'j', ' ', 'q'})
	require.NoError(t, err)
	assert.False(t, ok, "quitting via q must not be reported as a finished session")

	require.Equal(t, []int{1}, fake.calls)
	result := runner.Result()
	require.Len(t, result, 3)
	assert.False(t, result[0].Checked)
	assert.True(t, result[1].Checked)
	assert.False(t, result[2].Checked)
}

// TestChecklistRunner_CursorClampDown proves three down-moves on a 3-item
// list stop at the last index instead of wrapping back to 0.
func TestChecklistRunner_CursorClampDown(t *testing.T) {
	items := []ChecklistItem{{Text: "one"}, {Text: "two"}, {Text: "three"}}
	fake := newFakeToggler(items)

	runner := NewChecklistRunner("Morning")
	runner.Show(items, fake.toggle)

	_, ok, err := runChecklistRunner(t, runner, []byte{'j', 'j', 'j', ' ', 'q'})
	require.NoError(t, err)
	assert.False(t, ok)
	require.Equal(t, []int{2}, fake.calls, "cursor should clamp at the last item, not wrap to 0")
}

// TestChecklistRunner_CursorClampUp proves up-moves from the initial cursor
// position (0) never go negative.
func TestChecklistRunner_CursorClampUp(t *testing.T) {
	items := []ChecklistItem{{Text: "one"}, {Text: "two"}, {Text: "three"}}
	fake := newFakeToggler(items)

	runner := NewChecklistRunner("Morning")
	runner.Show(items, fake.toggle)

	_, ok, err := runChecklistRunner(t, runner, []byte{'k', 'k', ' ', 'q'})
	require.NoError(t, err)
	assert.False(t, ok)
	require.Equal(t, []int{0}, fake.calls)
}

// TestChecklistRunner_ToggleOn checks an unchecked item and asserts both the
// callback wiring and the rendered checkbox mark.
func TestChecklistRunner_ToggleOn(t *testing.T) {
	items := []ChecklistItem{{Text: "buy milk"}, {Text: "walk dog"}}
	fake := newFakeToggler(items)

	runner := NewChecklistRunner("Errands")
	runner.Show(items, fake.toggle)

	_, ok, err := runChecklistRunner(t, runner, []byte{' ', 'q'})
	require.NoError(t, err)
	assert.False(t, ok)
	require.Equal(t, []int{0}, fake.calls)
	assert.True(t, runner.Result()[0].Checked)
	assert.Contains(t, runner.View(), "[x] buy milk")
}

// TestChecklistRunner_ToggleOff checks a pre-checked item back off.
func TestChecklistRunner_ToggleOff(t *testing.T) {
	items := []ChecklistItem{{Text: "buy milk", Checked: true}, {Text: "walk dog"}}
	fake := newFakeToggler(items)

	runner := NewChecklistRunner("Errands")
	runner.Show(items, fake.toggle)

	_, ok, err := runChecklistRunner(t, runner, []byte{' ', 'q'})
	require.NoError(t, err)
	assert.False(t, ok)
	require.Equal(t, []int{0}, fake.calls)
	assert.False(t, runner.Result()[0].Checked)
	assert.Contains(t, runner.View(), "[ ] buy milk")
}

// TestChecklistRunner_AutoCompleteExits: checking the last remaining item
// where the callback reports completed=true ends the session with no
// further quit key required, and the final Result() is all-checked.
func TestChecklistRunner_AutoCompleteExits(t *testing.T) {
	items := []ChecklistItem{
		{Text: "one", Checked: true},
		{Text: "two", Checked: true},
		{Text: "three"},
	}
	fake := newFakeToggler(items)
	fake.completeOn = 0 // the single toggle this test drives completes the run

	runner := NewChecklistRunner("Morning")
	runner.Show(items, fake.toggle)

	result, ok, err := runChecklistRunner(t, runner, []byte{'j', 'j', ' '})
	require.NoError(t, err)
	assert.True(t, ok, "auto-completion should end the session as finished, not canceled")
	require.Equal(t, []int{2}, fake.calls)

	require.Len(t, result, 3)
	for i, item := range result {
		assert.True(t, item.Checked, "item %d should be checked on a completed run", i)
	}
	assert.Contains(t, runner.View(), "✓ Complete!")
}

// testChecklistRunnerQuit is shared by the three quit-key cases below: each
// key, pressed alone, must end the session cleanly with no error and without
// ever invoking the toggle callback.
func testChecklistRunnerQuit(t *testing.T, key byte) {
	t.Helper()
	items := []ChecklistItem{{Text: "one"}, {Text: "two"}}
	fake := newFakeToggler(items)

	runner := NewChecklistRunner("Morning")
	runner.Show(items, fake.toggle)

	result, ok, err := runChecklistRunner(t, runner, []byte{key})
	require.NoError(t, err, "quitting must never surface an error")
	assert.False(t, ok)
	assert.Empty(t, result)
	assert.Empty(t, fake.calls, "the toggle callback must never fire on a quit key")
}

func TestChecklistRunner_QuitQ(t *testing.T) {
	testChecklistRunnerQuit(t, 'q')
}

func TestChecklistRunner_QuitEsc(t *testing.T) {
	testChecklistRunnerQuit(t, 0x1b)
}

// TestChecklistRunner_QuitCtrlC proves Ctrl+C is handled in Update (setting
// canceled) rather than falling through to bubbletea's default kill path --
// if it fell through, Program.Run would return ErrProgramKilled and this
// would fail on the require.NoError below instead of just ok=false.
func TestChecklistRunner_QuitCtrlC(t *testing.T) {
	testChecklistRunnerQuit(t, 0x03)
}

// TestChecklistRunner_EmptyChecklist: navigation and toggle keys on a
// 0-item checklist are no-ops (no callback invocation, no panic); quitting
// still exits cleanly.
func TestChecklistRunner_EmptyChecklist(t *testing.T) {
	fake := newFakeToggler(nil)

	runner := NewChecklistRunner("Empty")
	runner.Show(nil, fake.toggle)

	result, ok, err := runChecklistRunner(t, runner, []byte{'j', 'k', ' ', 'q'})
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Empty(t, result)
	assert.Empty(t, fake.calls, "navigation/toggle on an empty checklist must never call the toggle callback")
	assert.Contains(t, runner.View(), "(no items)")
}

// TestChecklistRunner_MidSessionError: a toggle that fails must not surface
// through RunPrompt's own error return (which would misreport a user quit as
// a program failure) -- it is recorded on the runner and read separately via
// Err() once RunPrompt returns.
func TestChecklistRunner_MidSessionError(t *testing.T) {
	items := []ChecklistItem{{Text: "one"}, {Text: "two"}}
	fake := newFakeToggler(items)
	wantErr := errors.New("boom: persistence failed")
	fake.err = wantErr

	runner := NewChecklistRunner("Morning")
	runner.Show(items, fake.toggle)

	result, ok, err := runChecklistRunner(t, runner, []byte{' '})
	require.NoError(t, err, "RunPrompt itself must not surface a mid-session toggle error")
	assert.False(t, ok)
	assert.Empty(t, result)
	require.Equal(t, []int{0}, fake.calls)
	require.ErrorIs(t, runner.Err(), wantErr)
}
