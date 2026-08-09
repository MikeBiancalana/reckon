package components

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TodoBrowser must satisfy Prompt[[]TodoItem] like every other component in
// this package, so it can be hosted by RunPrompt without a bespoke adapter.
var _ Prompt[[]TodoItem] = (*TodoBrowser)(nil)

// TestTodoBrowser_ConformanceCompiles gives the var block above a runnable
// test to report; the assertion is the compile-time check itself.
func TestTodoBrowser_ConformanceCompiles(t *testing.T) {}

// fakeMarkDoneCall records one MarkDoneFunc invocation: the cursor position
// it was called with, plus the Kind/Ref of the item that sat at that
// position at call time. Recording Kind/Ref (not just position) lets tests
// prove the component reached the *intended row*, not merely "some" row --
// mirroring fakeToggler.calls (checklist_runner_test.go) but carrying the
// extra identity fields a heterogeneous list needs.
type fakeMarkDoneCall struct {
	position int
	kind     string
	ref      string
}

// fakeMarkDoner stands in for the real CLI-layer closure
// (makeMarkDoneFunc): it removes the acted-on item from its own in-memory
// copy and returns the shrunk slice, mirroring MarkDoneFunc's one-directional
// "remove on action" contract (todo_browser.go has no toggle-back path).
type fakeMarkDoner struct {
	items []TodoItem
	calls []fakeMarkDoneCall
	err   error
}

func newFakeMarkDoner(items []TodoItem) *fakeMarkDoner {
	cp := make([]TodoItem, len(items))
	copy(cp, items)
	return &fakeMarkDoner{items: cp}
}

func (f *fakeMarkDoner) markDone(position int) ([]TodoItem, error) {
	f.calls = append(f.calls, fakeMarkDoneCall{
		position: position,
		kind:     f.items[position].Kind,
		ref:      f.items[position].Ref,
	})
	if f.err != nil {
		return nil, f.err
	}
	f.items = append(f.items[:position], f.items[position+1:]...)
	out := make([]TodoItem, len(f.items))
	copy(out, f.items)
	return out, nil
}

// runTodoBrowser drives browser through RunPrompt with the given raw key
// bytes (one KeyMsg per byte, via the package's singleByteReader --
// checklist_runner_test.go), bounded by runPromptForTest's timeout so a
// wrong or missing quit key fails the test instead of hanging it.
func runTodoBrowser(t *testing.T, browser *TodoBrowser, keys []byte) ([]TodoItem, bool, error) {
	t.Helper()
	return runPromptForTest(t, func() ([]TodoItem, bool, error) {
		return RunPrompt[[]TodoItem](browser,
			tea.WithInput(singleByteReader{bytes.NewReader(keys)}),
			tea.WithOutput(io.Discard),
		)
	})
}

// TestTodoBrowser_FreshNavigateAndMarkDone proves the cursor position at the
// time Space is pressed -- not just the fact of a call -- is what reaches the
// callback: one down-move then Space must call the fake with position 1, not 0.
func TestTodoBrowser_FreshNavigateAndMarkDone(t *testing.T) {
	items := []TodoItem{
		{Kind: "durable", Ref: "a", Title: "one"},
		{Kind: "durable", Ref: "b", Title: "two"},
		{Kind: "durable", Ref: "c", Title: "three"},
	}
	fake := newFakeMarkDoner(items)

	browser := NewTodoBrowser("Todos")
	browser.Show(items, fake.markDone)

	_, ok, err := runTodoBrowser(t, browser, []byte{'j', ' ', 'q'})
	require.NoError(t, err)
	assert.False(t, ok, "quitting via q must not be reported as a finished session")

	require.Len(t, fake.calls, 1)
	assert.Equal(t, 1, fake.calls[0].position)
	assert.Equal(t, "b", fake.calls[0].ref)
}

// TestTodoBrowser_ListShrinksAndCursorClamps mirrors AC §3 "Cursor clamp
// after a toggle shrinks the list": marking done the item under the cursor
// on a 3-item list removes it, and a cursor sitting at the now-invalid last
// index must clamp to the new last index rather than go out of range.
func TestTodoBrowser_ListShrinksAndCursorClamps(t *testing.T) {
	items := []TodoItem{
		{Kind: "durable", Ref: "a", Title: "Task A"},
		{Kind: "durable", Ref: "b", Title: "Task B"},
		{Kind: "durable", Ref: "c", Title: "Task C"},
	}
	fake := newFakeMarkDoner(items)

	browser := NewTodoBrowser("Todos")
	browser.Show(items, fake.markDone)

	_, ok, err := runTodoBrowser(t, browser, []byte{'j', 'j', ' ', 'q'})
	require.NoError(t, err)
	assert.False(t, ok)

	require.Len(t, fake.calls, 1)
	assert.Equal(t, 2, fake.calls[0].position, "mark-done should act on the item under the cursor (index 2)")

	result := browser.Result()
	require.Len(t, result, 2, "the acted-on item must be removed from the session, not merely flipped")

	view := browser.View()
	if !strings.Contains(view, "> [ ] Task B") {
		t.Errorf("cursor should clamp to the new last item (Task B) after the list shrank, got view:\n%s", view)
	}
	if strings.Contains(view, "> [ ] Task A") {
		t.Errorf("cursor should not remain on Task A after clamping, got view:\n%s", view)
	}
}

// TestTodoBrowser_LastItemMarkDoneEmptyTransition mirrors AC §3 "List becomes
// empty mid-session": marking done the only remaining item must transition
// cleanly to the (no items) render, with no index-out-of-range panic and a
// clean quit afterward.
func TestTodoBrowser_LastItemMarkDoneEmptyTransition(t *testing.T) {
	items := []TodoItem{{Kind: "durable", Ref: "a", Title: "Only task"}}
	fake := newFakeMarkDoner(items)

	browser := NewTodoBrowser("Todos")
	browser.Show(items, fake.markDone)

	_, ok, err := runTodoBrowser(t, browser, []byte{' ', 'q'})
	require.NoError(t, err)
	assert.False(t, ok)

	require.Len(t, fake.calls, 1)
	assert.Empty(t, browser.Result())
	assert.Contains(t, browser.View(), "(no items)")
}

// TestTodoBrowser_HeterogeneousCursorToRefDispatch proves the cursor
// position reaching the callback resolves unambiguously to the intended
// mixed-kind row: navigating onto an ephemeral row among durable rows and
// marking it done must record that row's own Kind/Ref, not the position of a
// neighboring durable row (AC §3 "Mixed-kind list ordering stability").
func TestTodoBrowser_HeterogeneousCursorToRefDispatch(t *testing.T) {
	items := []TodoItem{
		{Kind: "durable", Ref: "01JDURABLEONE00000000000", Title: "Durable one"},
		{Kind: "ephemeral", Ref: "2", Title: "Ephemeral two"},
		{Kind: "durable", Ref: "01JDURABLETHREE000000000", Title: "Durable three"},
	}
	fake := newFakeMarkDoner(items)

	browser := NewTodoBrowser("Todos")
	browser.Show(items, fake.markDone)

	_, ok, err := runTodoBrowser(t, browser, []byte{'j', ' ', 'q'})
	require.NoError(t, err)
	assert.False(t, ok)

	require.Len(t, fake.calls, 1)
	call := fake.calls[0]
	assert.Equal(t, 1, call.position)
	assert.Equal(t, "ephemeral", call.kind, "cursor landed on the ephemeral row, dispatch must not guess durable")
	assert.Equal(t, "2", call.ref)
}

// TestTodoBrowser_CursorClampDown proves three down-moves on a 3-item list
// stop at the last index instead of wrapping back to 0.
func TestTodoBrowser_CursorClampDown(t *testing.T) {
	items := []TodoItem{
		{Kind: "durable", Ref: "a", Title: "one"},
		{Kind: "durable", Ref: "b", Title: "two"},
		{Kind: "durable", Ref: "c", Title: "three"},
	}
	fake := newFakeMarkDoner(items)

	browser := NewTodoBrowser("Todos")
	browser.Show(items, fake.markDone)

	_, ok, err := runTodoBrowser(t, browser, []byte{'j', 'j', 'j', ' ', 'q'})
	require.NoError(t, err)
	assert.False(t, ok)
	require.Len(t, fake.calls, 1)
	assert.Equal(t, 2, fake.calls[0].position, "cursor should clamp at the last item, not wrap to 0")
}

// TestTodoBrowser_CursorClampUp proves up-moves from the initial cursor
// position (0) never go negative.
func TestTodoBrowser_CursorClampUp(t *testing.T) {
	items := []TodoItem{
		{Kind: "durable", Ref: "a", Title: "one"},
		{Kind: "durable", Ref: "b", Title: "two"},
		{Kind: "durable", Ref: "c", Title: "three"},
	}
	fake := newFakeMarkDoner(items)

	browser := NewTodoBrowser("Todos")
	browser.Show(items, fake.markDone)

	_, ok, err := runTodoBrowser(t, browser, []byte{'k', 'k', ' ', 'q'})
	require.NoError(t, err)
	assert.False(t, ok)
	require.Len(t, fake.calls, 1)
	assert.Equal(t, 0, fake.calls[0].position)
}

// TestTodoBrowser_EmptyListNoOps: navigation and mark-done keys on a 0-item
// list are no-ops (no callback invocation, no panic); quitting still exits
// cleanly.
func TestTodoBrowser_EmptyListNoOps(t *testing.T) {
	fake := newFakeMarkDoner(nil)

	browser := NewTodoBrowser("Todos")
	browser.Show(nil, fake.markDone)

	result, ok, err := runTodoBrowser(t, browser, []byte{'j', 'k', ' ', 'q'})
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Empty(t, result)
	assert.Empty(t, fake.calls, "navigation/mark-done on an empty list must never call the callback")
	assert.Contains(t, browser.View(), "(no items)")
}

// TestTodoBrowser_MidSessionError: a mark-done call that fails must not
// surface through RunPrompt's own error return (which would misreport a user
// quit as a program failure) -- it is recorded on the browser and read
// separately via Err() once RunPrompt returns, exactly like ChecklistRunner.
func TestTodoBrowser_MidSessionError(t *testing.T) {
	items := []TodoItem{
		{Kind: "durable", Ref: "a", Title: "one"},
		{Kind: "durable", Ref: "b", Title: "two"},
	}
	fake := newFakeMarkDoner(items)
	wantErr := errors.New("boom: persistence failed")
	fake.err = wantErr

	browser := NewTodoBrowser("Todos")
	browser.Show(items, fake.markDone)

	result, ok, err := runTodoBrowser(t, browser, []byte{' '})
	require.NoError(t, err, "RunPrompt itself must not surface a mid-session mark-done error")
	assert.False(t, ok)
	assert.Empty(t, result)
	require.Len(t, fake.calls, 1)
	require.ErrorIs(t, browser.Err(), wantErr)

	// No quit key was pressed, so canceled must come solely from the
	// r.err != nil clause in Done(), not from a separate quit flag.
	_, canceled := browser.Done()
	assert.True(t, canceled, "a mid-session error alone must report canceled")
}

// testTodoBrowserQuit is shared by the three quit-key cases below: each key,
// pressed alone, must end the session cleanly with no error and without ever
// invoking the mark-done callback.
func testTodoBrowserQuit(t *testing.T, key byte) {
	t.Helper()
	items := []TodoItem{
		{Kind: "durable", Ref: "a", Title: "one"},
		{Kind: "durable", Ref: "b", Title: "two"},
	}
	fake := newFakeMarkDoner(items)

	browser := NewTodoBrowser("Todos")
	browser.Show(items, fake.markDone)

	result, ok, err := runTodoBrowser(t, browser, []byte{key})
	require.NoError(t, err, "quitting must never surface an error")
	assert.False(t, ok)
	assert.Empty(t, result)
	assert.Empty(t, fake.calls, "the mark-done callback must never fire on a quit key")
}

func TestTodoBrowser_QuitQ(t *testing.T) {
	testTodoBrowserQuit(t, 'q')
}

func TestTodoBrowser_QuitEsc(t *testing.T) {
	testTodoBrowserQuit(t, 0x1b)
}

// TestTodoBrowser_QuitCtrlC proves Ctrl+C is handled in Update (setting
// canceled) rather than falling through to bubbletea's default kill path --
// if it fell through, Program.Run would return ErrProgramKilled and this
// would fail on the require.NoError below instead of just ok=false.
func TestTodoBrowser_QuitCtrlC(t *testing.T) {
	testTodoBrowserQuit(t, 0x03)
}

// TestTodoBrowser_EmptyTitleRendersFallback mirrors AC §3 "Todo with no
// title / malformed node": an item reaching the component with an empty
// Title must never render a blank/invisible cursor row.
func TestTodoBrowser_EmptyTitleRendersFallback(t *testing.T) {
	items := []TodoItem{{Kind: "durable", Ref: "a", Title: ""}}
	fake := newFakeMarkDoner(items)

	browser := NewTodoBrowser("Todos")
	browser.Show(items, fake.markDone)

	view := browser.View()
	lines := strings.Split(view, "\n")
	require.GreaterOrEqual(t, len(lines), 2, "expected a header line plus at least one item row, got:\n%s", view)

	row := strings.TrimSpace(lines[1])
	assert.NotEqual(t, "> [ ]", row, "empty-Title item rendered as a blank/invisible row, got view:\n%s", view)
}
