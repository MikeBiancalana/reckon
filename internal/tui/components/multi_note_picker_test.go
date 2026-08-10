package components

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Compile-time conformance assertion: MultiNotePicker must satisfy
// Prompt[[]string] via its own Update method.
var _ Prompt[[]string] = (*MultiNotePicker)(nil)

// TestMultiNotePicker_ConformanceCompiles exists so the conformance
// assertion above has a runnable test function to report.
func TestMultiNotePicker_ConformanceCompiles(t *testing.T) {}

func multiNotePickerRows() []IndexRow {
	return []IndexRow{
		{ID: "note-1", Title: "OAuth Flow Patterns", Props: map[string]string{"slug": "oauth-flow-patterns"}},
		{ID: "note-2", Title: "Second Note", Props: map[string]string{"slug": "second-note"}},
	}
}

func TestNewMultiNotePicker(t *testing.T) {
	mp := NewMultiNotePicker("Link notes")
	if mp == nil {
		t.Fatal("expected NewMultiNotePicker to return non-nil")
	}
	if mp.visible {
		t.Error("expected initial state to be hidden")
	}
}

func TestMultiNotePicker_ShowResetsSelection(t *testing.T) {
	mp := NewMultiNotePicker("Link notes")
	mp.Show(multiNotePickerRows())

	if !mp.visible {
		t.Error("expected visible=true after Show")
	}
	if len(mp.notes) != 2 {
		t.Errorf("notes = %d, want 2", len(mp.notes))
	}
	if got := mp.SelectedSlugs(); len(got) != 0 {
		t.Errorf("SelectedSlugs() = %v, want empty right after Show", got)
	}
}

// TestMultiNotePicker_SpaceTogglesSelectionEnterConfirms (T15): Space on two
// distinct rows toggles both into the selection set; Enter then confirms
// with both slugs present in Result().
func TestMultiNotePicker_SpaceTogglesSelectionEnterConfirms(t *testing.T) {
	mp := NewMultiNotePicker("Link notes")
	mp.Show(multiNotePickerRows())

	mp.Update(tea.KeyMsg{Type: tea.KeySpace}) // toggle row 0 (cursor starts at 0)
	mp.Update(tea.KeyMsg{Type: tea.KeyDown})
	mp.Update(tea.KeyMsg{Type: tea.KeySpace}) // toggle row 1
	mp.Update(tea.KeyMsg{Type: tea.KeyEnter})

	finished, canceled := mp.Done()
	if !finished || canceled {
		t.Fatalf("Done() = (%v, %v), want (true, false) after two Space toggles + Enter", finished, canceled)
	}

	got := mp.Result()
	want := map[string]bool{"oauth-flow-patterns": true, "second-note": true}
	if len(got) != 2 {
		t.Fatalf("Result() = %v, want 2 slugs", got)
	}
	for _, slug := range got {
		if !want[slug] {
			t.Errorf("unexpected slug %q in Result() %v", slug, got)
		}
	}
}

// TestMultiNotePicker_EmptySelectionEnterConfirms (gap G5): Enter with no
// Space toggles at all still confirms (finished=true), with Result() an
// empty (not nil-panicking) slice -- links are optional, so "skip" must be
// reachable without selecting anything.
func TestMultiNotePicker_EmptySelectionEnterConfirms(t *testing.T) {
	mp := NewMultiNotePicker("Link notes")
	mp.Show(multiNotePickerRows())

	mp.Update(tea.KeyMsg{Type: tea.KeyEnter})

	finished, canceled := mp.Done()
	if !finished || canceled {
		t.Fatalf("Done() = (%v, %v), want (true, false) after Enter with no selections", finished, canceled)
	}
	if got := mp.Result(); len(got) != 0 {
		t.Errorf("Result() = %v, want empty", got)
	}
}

func TestMultiNotePicker_EscCancels(t *testing.T) {
	mp := NewMultiNotePicker("Link notes")
	mp.Show(multiNotePickerRows())

	mp.Update(tea.KeyMsg{Type: tea.KeySpace})
	mp.Update(tea.KeyMsg{Type: tea.KeyEsc})

	finished, canceled := mp.Done()
	if finished || !canceled {
		t.Fatalf("Done() = (%v, %v), want (false, true) after Esc", finished, canceled)
	}
}

// TestMultiNotePicker_SpaceTwiceOnSameRowDeselects: Space is a toggle, not a
// one-way set -- pressing it twice on the same row must leave that row
// unselected again.
func TestMultiNotePicker_SpaceTwiceOnSameRowDeselects(t *testing.T) {
	mp := NewMultiNotePicker("Link notes")
	mp.Show(multiNotePickerRows())

	mp.Update(tea.KeyMsg{Type: tea.KeySpace})
	mp.Update(tea.KeyMsg{Type: tea.KeySpace})
	mp.Update(tea.KeyMsg{Type: tea.KeyEnter})

	finished, _ := mp.Done()
	if !finished {
		t.Fatal("expected Enter to confirm")
	}
	if got := mp.Result(); len(got) != 0 {
		t.Errorf("Result() = %v, want empty after toggling the same row on then off", got)
	}
}
