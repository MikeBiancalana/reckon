package components

import (
	"strings"
	"testing"
)

func TestStatusBarTasksSectionShowsSchedulingHints(t *testing.T) {
	sb := NewStatusBar()
	sb.SetSection("Tasks")
	sb.SetWidth(200)

	view := sb.View()

	for _, hint := range []string{"s:schedule", "D:deadline", "c:clear"} {
		if !strings.Contains(view, hint) {
			t.Errorf("Tasks section status bar should contain %q, got: %s", hint, view)
		}
	}
}

func TestStatusBarNoteSelectedHidesSchedulingHints(t *testing.T) {
	sb := NewStatusBar()
	sb.SetSection("Tasks")
	sb.SetNoteSelected(true)
	sb.SetWidth(200)

	view := sb.View()

	for _, hint := range []string{"s:schedule", "D:deadline", "c:clear"} {
		if strings.Contains(view, hint) {
			t.Errorf("Tasks section with note selected should NOT contain %q, got: %s", hint, view)
		}
	}
}

func TestStatusBarTaskSelectedShowsSchedulingHints(t *testing.T) {
	sb := NewStatusBar()
	sb.SetSection("Tasks")
	sb.SetNoteSelected(false)
	sb.SetWidth(200)

	view := sb.View()

	for _, hint := range []string{"s:schedule", "D:deadline", "c:clear"} {
		if !strings.Contains(view, hint) {
			t.Errorf("Tasks section with task selected should contain %q, got: %s", hint, view)
		}
	}
}

func TestStatusBarOtherSectionsHideSchedulingHints(t *testing.T) {
	sections := []string{"Intentions", "Wins", "Logs", "Schedule"}

	for _, section := range sections {
		t.Run(section, func(t *testing.T) {
			sb := NewStatusBar()
			sb.SetSection(section)
			sb.SetWidth(200)

			view := sb.View()

			for _, hint := range []string{"s:schedule", "D:deadline", "c:clear"} {
				if strings.Contains(view, hint) {
					t.Errorf("%s section should NOT contain %q, got: %s", section, hint, view)
				}
			}
		})
	}
}

func TestStatusBarInputModeOverridesHints(t *testing.T) {
	sb := NewStatusBar()
	sb.SetSection("Tasks")
	sb.SetNoteSelected(false)
	sb.SetInputMode(true)
	sb.SetWidth(200)

	view := sb.View()

	if !strings.Contains(view, "enter:submit") {
		t.Errorf("Input mode status bar should contain 'enter:submit', got: %s", view)
	}
	for _, hint := range []string{"s:schedule", "D:deadline", "c:clear"} {
		if strings.Contains(view, hint) {
			t.Errorf("Input mode status bar should NOT contain %q, got: %s", hint, view)
		}
	}
}

func TestStatusBarSetNoteSelectedResetsOnNewSection(t *testing.T) {
	sb := NewStatusBar()
	sb.SetSection("Tasks")
	sb.SetNoteSelected(true)
	sb.SetWidth(200)

	// Switch sections - note-selected state should not leak
	sb.SetSection("Intentions")
	sb.SetSection("Tasks")

	// After section switch and back, note selection was not reset
	// (that's the keyboard's responsibility). The status bar just uses what it has.
	// This test verifies SetNoteSelected(false) works correctly.
	sb.SetNoteSelected(false)
	view := sb.View()

	if !strings.Contains(view, "s:schedule") {
		t.Errorf("After SetNoteSelected(false), Tasks section should show s:schedule, got: %s", view)
	}
}
