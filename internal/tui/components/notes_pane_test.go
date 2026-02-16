package components

import (
	"testing"
	"time"

	"github.com/MikeBiancalana/reckon/internal/models"
	tea "github.com/charmbracelet/bubbletea"
)

// TestNewNotesPane verifies the initial state of a new NotesPane
func TestNewNotesPane(t *testing.T) {
	np := NewNotesPane()

	if np == nil {
		t.Fatal("NewNotesPane returned nil")
	}

	if np.focused {
		t.Error("new NotesPane should not be focused initially")
	}

	if np.outgoingLinks != nil && len(np.outgoingLinks) > 0 {
		t.Error("new NotesPane should have no outgoing links initially")
	}

	if np.backlinks != nil && len(np.backlinks) > 0 {
		t.Error("new NotesPane should have no backlinks initially")
	}

	if np.currentNoteID != "" {
		t.Error("new NotesPane should have no current note ID initially")
	}

	if np.loading {
		t.Error("new NotesPane should not be in loading state initially")
	}

	if np.cursor != 0 {
		t.Error("new NotesPane cursor should be 0 initially")
	}

	if np.outgoingCollapsed {
		t.Error("outgoing section should not be collapsed initially")
	}

	if np.backlinksCollapsed {
		t.Error("backlinks section should not be collapsed initially")
	}
}

// TestNotesPane_UpdateLinks verifies updating the pane with outgoing and backlinks
func TestNotesPane_UpdateLinks(t *testing.T) {
	np := NewNotesPane()

	outgoing := []LinkDisplayItem{
		{
			NoteLink: models.NoteLink{
				ID:           "link1",
				SourceNoteID: "note1",
				TargetSlug:   "target-note",
				TargetNoteID: "note2",
				LinkType:     models.LinkTypeReference,
				CreatedAt:    time.Now(),
				TargetNote: &models.Note{
					ID:    "note2",
					Title: "Target Note",
					Slug:  "target-note",
				},
			},
			DisplayText: "Target Note",
			IsResolved:  true,
		},
	}

	backlinks := []LinkDisplayItem{
		{
			NoteLink: models.NoteLink{
				ID:           "link2",
				SourceNoteID: "note3",
				TargetSlug:   "current-note",
				TargetNoteID: "note1",
				LinkType:     models.LinkTypeReference,
				CreatedAt:    time.Now(),
				SourceNote: &models.Note{
					ID:    "note3",
					Title: "Source Note",
					Slug:  "source-note",
				},
			},
			DisplayText: "Source Note",
			IsResolved:  true,
		},
	}

	np.UpdateLinks("note1", outgoing, backlinks)

	if len(np.outgoingLinks) != 1 {
		t.Errorf("expected 1 outgoing link, got %d", len(np.outgoingLinks))
	}

	if len(np.backlinks) != 1 {
		t.Errorf("expected 1 backlink, got %d", len(np.backlinks))
	}

	if np.currentNoteID != "note1" {
		t.Errorf("expected currentNoteID to be 'note1', got '%s'", np.currentNoteID)
	}

	if np.loading {
		t.Error("loading should be false after UpdateLinks")
	}

	if np.cursor != 0 {
		t.Error("cursor should be reset to 0 after UpdateLinks")
	}
}

// TestNotesPane_View_EmptyState tests empty state rendering
func TestNotesPane_View_EmptyState(t *testing.T) {
	tests := []struct {
		name          string
		currentNoteID string
		loading       bool
		wantContains  string
	}{
		{
			name:          "no note selected",
			currentNoteID: "",
			loading:       false,
			wantContains:  "Select a note",
		},
		{
			name:          "loading state",
			currentNoteID: "note1",
			loading:       true,
			wantContains:  "Loading",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			np := NewNotesPane()
			np.currentNoteID = tt.currentNoteID
			np.loading = tt.loading

			view := np.View()

			if view == "" {
				t.Error("View should not return empty string")
			}

			// Note: This is a basic check - actual implementation may differ
			// The test will fail initially until View is implemented
		})
	}
}

// TestNotesPane_View_NoLinks tests rendering when note has no links
func TestNotesPane_View_NoLinks(t *testing.T) {
	np := NewNotesPane()
	np.UpdateLinks("note1", []LinkDisplayItem{}, []LinkDisplayItem{})

	view := np.View()

	if view == "" {
		t.Error("View should render even with no links")
	}

	// Should show "No outgoing links" or similar message
	// Test will fail until implemented
}

// TestNotesPane_View_WithLinks tests rendering with both outgoing and backlinks
func TestNotesPane_View_WithLinks(t *testing.T) {
	np := NewNotesPane()

	outgoing := []LinkDisplayItem{
		{
			NoteLink: models.NoteLink{
				TargetSlug: "target-1",
				TargetNote: &models.Note{Title: "Target One", Slug: "target-1"},
			},
			DisplayText: "Target One",
			IsResolved:  true,
		},
		{
			NoteLink: models.NoteLink{
				TargetSlug: "target-2",
			},
			DisplayText: "target-2",
			IsResolved:  false,
		},
	}

	backlinks := []LinkDisplayItem{
		{
			NoteLink: models.NoteLink{
				TargetSlug: "current",
				SourceNote: &models.Note{Title: "Source One", Slug: "source-1"},
			},
			DisplayText: "Source One",
			IsResolved:  true,
		},
	}

	np.UpdateLinks("note1", outgoing, backlinks)
	view := np.View()

	if view == "" {
		t.Error("View should render with links")
	}

	// View should contain section headers and link text
	// Test will fail until rendering is implemented
}

// TestNotesPane_View_OnlyOutgoing tests rendering with only outgoing links
func TestNotesPane_View_OnlyOutgoing(t *testing.T) {
	np := NewNotesPane()

	outgoing := []LinkDisplayItem{
		{
			NoteLink:    models.NoteLink{TargetSlug: "target"},
			DisplayText: "Target Link",
			IsResolved:  true,
		},
	}

	np.UpdateLinks("note1", outgoing, []LinkDisplayItem{})

	view := np.View()

	if view == "" {
		t.Error("View should render with only outgoing links")
	}
}

// TestNotesPane_View_OnlyBacklinks tests rendering with only backlinks
func TestNotesPane_View_OnlyBacklinks(t *testing.T) {
	np := NewNotesPane()

	backlinks := []LinkDisplayItem{
		{
			NoteLink:    models.NoteLink{TargetSlug: "current"},
			DisplayText: "Source Link",
			IsResolved:  true,
		},
	}

	np.UpdateLinks("note1", []LinkDisplayItem{}, backlinks)

	view := np.View()

	if view == "" {
		t.Error("View should render with only backlinks")
	}
}

// TestNotesPane_Navigation_JK tests j/k navigation
func TestNotesPane_Navigation_JK(t *testing.T) {
	np := NewNotesPane()
	np.focused = true

	outgoing := []LinkDisplayItem{
		{NoteLink: models.NoteLink{TargetSlug: "link1"}, DisplayText: "Link 1", IsResolved: true},
		{NoteLink: models.NoteLink{TargetSlug: "link2"}, DisplayText: "Link 2", IsResolved: true},
	}

	backlinks := []LinkDisplayItem{
		{NoteLink: models.NoteLink{TargetSlug: "current"}, DisplayText: "Backlink 1", IsResolved: true},
	}

	np.UpdateLinks("note1", outgoing, backlinks)

	t.Run("j moves cursor down", func(t *testing.T) {
		initialCursor := np.cursor
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}
		np, _ = np.Update(msg)

		if np.cursor <= initialCursor {
			t.Errorf("j should move cursor down, was %d, now %d", initialCursor, np.cursor)
		}
	})

	t.Run("k moves cursor up", func(t *testing.T) {
		np.cursor = 1 // Set to non-zero position
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")}
		np, _ = np.Update(msg)

		if np.cursor != 0 {
			t.Errorf("k should move cursor up to 0, got %d", np.cursor)
		}
	})

	t.Run("cursor respects bounds", func(t *testing.T) {
		// Try to move up from 0
		np.cursor = 0
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")}
		np, _ = np.Update(msg)

		if np.cursor < 0 {
			t.Error("cursor should not go below 0")
		}

		// Try to move down past max
		maxAttempts := 100
		for i := 0; i < maxAttempts; i++ {
			msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}
			np, _ = np.Update(msg)
		}

		// Cursor should stay within valid range
		// (exact max depends on section count and collapse state)
	})
}

// TestNotesPane_Navigation_GG tests g/G jump navigation
func TestNotesPane_Navigation_GG(t *testing.T) {
	np := NewNotesPane()
	np.focused = true

	outgoing := []LinkDisplayItem{
		{NoteLink: models.NoteLink{TargetSlug: "link1"}, DisplayText: "Link 1", IsResolved: true},
		{NoteLink: models.NoteLink{TargetSlug: "link2"}, DisplayText: "Link 2", IsResolved: true},
		{NoteLink: models.NoteLink{TargetSlug: "link3"}, DisplayText: "Link 3", IsResolved: true},
	}

	backlinks := []LinkDisplayItem{
		{NoteLink: models.NoteLink{TargetSlug: "current"}, DisplayText: "Backlink 1", IsResolved: true},
		{NoteLink: models.NoteLink{TargetSlug: "current"}, DisplayText: "Backlink 2", IsResolved: true},
	}

	np.UpdateLinks("note1", outgoing, backlinks)

	t.Run("g jumps to first item", func(t *testing.T) {
		np.cursor = 5 // Set to middle position
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")}
		np, _ = np.Update(msg)

		if np.cursor != 0 {
			t.Errorf("g should jump to first item (0), got %d", np.cursor)
		}
	})

	t.Run("G jumps to last item", func(t *testing.T) {
		np.cursor = 0
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")}
		np, _ = np.Update(msg)

		// Cursor should be at the last valid position
		// (exact position depends on section headers + items)
		if np.cursor == 0 {
			t.Error("G should jump to last item, cursor is still 0")
		}
	})
}

// TestNotesPane_Navigation_Enter tests enter key emitting LinkSelectedMsg
func TestNotesPane_Navigation_Enter(t *testing.T) {
	np := NewNotesPane()
	np.focused = true

	outgoing := []LinkDisplayItem{
		{
			NoteLink: models.NoteLink{
				TargetSlug:   "target-note",
				TargetNoteID: "note2",
			},
			DisplayText: "Target Note",
			IsResolved:  true,
		},
	}

	np.UpdateLinks("note1", outgoing, []LinkDisplayItem{})

	// Position cursor on the link (not the header)
	np.cursor = 1 // Assuming header is at 0

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := np.Update(msg)

	if cmd == nil {
		t.Fatal("enter should return a command")
	}

	result := cmd()
	linkMsg, ok := result.(LinkSelectedMsg)
	if !ok {
		t.Fatalf("expected LinkSelectedMsg, got %T", result)
	}

	if linkMsg.NoteSlug != "target-note" {
		t.Errorf("expected slug 'target-note', got '%s'", linkMsg.NoteSlug)
	}

	if linkMsg.NoteID != "note2" {
		t.Errorf("expected note ID 'note2', got '%s'", linkMsg.NoteID)
	}
}

// TestNotesPane_Navigation_Tab tests tab key toggling section collapse
func TestNotesPane_Navigation_Space(t *testing.T) {
	np := NewNotesPane()
	np.focused = true

	outgoing := []LinkDisplayItem{
		{NoteLink: models.NoteLink{TargetSlug: "link1"}, DisplayText: "Link 1", IsResolved: true},
		{NoteLink: models.NoteLink{TargetSlug: "link2"}, DisplayText: "Link 2", IsResolved: true},
	}

	backlinks := []LinkDisplayItem{
		{NoteLink: models.NoteLink{TargetSlug: "current"}, DisplayText: "Backlink 1", IsResolved: true},
	}

	np.UpdateLinks("note1", outgoing, backlinks)

	t.Run("space on outgoing section header toggles collapse", func(t *testing.T) {
		// Position cursor on outgoing section header (index 0)
		np.cursor = 0
		initialCollapsed := np.outgoingCollapsed

		msg := tea.KeyMsg{Type: tea.KeySpace}
		np, _ = np.Update(msg)

		if np.outgoingCollapsed == initialCollapsed {
			t.Error("space should toggle outgoing section collapse state")
		}

		// Toggle again
		np, _ = np.Update(msg)

		if np.outgoingCollapsed != initialCollapsed {
			t.Error("space again should restore original collapse state")
		}
	})

	t.Run("space on backlinks section header toggles collapse", func(t *testing.T) {
		// Position cursor on backlinks section header
		// (exact index depends on whether outgoing is collapsed and how many items)
		// For now, test the mechanism exists
		np.cursor = 3 // Approximate position for backlinks header
		initialCollapsed := np.backlinksCollapsed

		msg := tea.KeyMsg{Type: tea.KeySpace}
		np, _ = np.Update(msg)

		if np.backlinksCollapsed == initialCollapsed {
			t.Error("space should toggle backlinks section collapse state")
		}
	})
}

// TestNotesPane_CollapsedSection tests navigation skips collapsed sections
func TestNotesPane_CollapsedSection(t *testing.T) {
	np := NewNotesPane()
	np.focused = true

	outgoing := []LinkDisplayItem{
		{NoteLink: models.NoteLink{TargetSlug: "link1"}, DisplayText: "Link 1", IsResolved: true},
		{NoteLink: models.NoteLink{TargetSlug: "link2"}, DisplayText: "Link 2", IsResolved: true},
	}

	backlinks := []LinkDisplayItem{
		{NoteLink: models.NoteLink{TargetSlug: "current"}, DisplayText: "Backlink 1", IsResolved: true},
	}

	np.UpdateLinks("note1", outgoing, backlinks)

	// Collapse the outgoing section
	np.outgoingCollapsed = true

	// Try to navigate into collapsed section
	// Should skip over collapsed items
	// Test will verify this behavior once implemented
}

// TestNotesPane_SetFocused tests focus state changes
func TestNotesPane_SetFocused(t *testing.T) {
	np := NewNotesPane()

	t.Run("SetFocused(true) sets focused", func(t *testing.T) {
		np.SetFocused(true)

		if !np.focused {
			t.Error("SetFocused(true) should set focused to true")
		}
	})

	t.Run("SetFocused(false) unsets focused", func(t *testing.T) {
		np.SetFocused(false)

		if np.focused {
			t.Error("SetFocused(false) should set focused to false")
		}
	})

	t.Run("unfocused pane does not respond to keys", func(t *testing.T) {
		np.SetFocused(false)
		np.cursor = 0

		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}
		np, _ = np.Update(msg)

		if np.cursor != 0 {
			t.Error("unfocused pane should not respond to navigation keys")
		}
	})
}

// TestNotesPane_SetSize tests size setting
func TestNotesPane_SetSize(t *testing.T) {
	np := NewNotesPane()

	// Should not panic
	np.SetSize(80, 24)

	if np.width != 80 {
		t.Errorf("expected width 80, got %d", np.width)
	}

	if np.height != 24 {
		t.Errorf("expected height 24, got %d", np.height)
	}
}

// TestNotesPane_UnresolvedLink tests unresolved link display
func TestNotesPane_UnresolvedLink(t *testing.T) {
	np := NewNotesPane()

	outgoing := []LinkDisplayItem{
		{
			NoteLink: models.NoteLink{
				TargetSlug:   "unresolved-note",
				TargetNoteID: "", // Unresolved
			},
			DisplayText: "unresolved-note",
			IsResolved:  false,
		},
	}

	np.UpdateLinks("note1", outgoing, []LinkDisplayItem{})

	view := np.View()

	if view == "" {
		t.Error("View should render unresolved links")
	}

	// Unresolved links should be visually distinct (dimmed, marked, etc.)
	// Test will verify once rendering is implemented
}

// TestNotesPane_Resize tests terminal resize handling
func TestNotesPane_Resize(t *testing.T) {
	np := NewNotesPane()

	outgoing := []LinkDisplayItem{
		{NoteLink: models.NoteLink{TargetSlug: "link1"}, DisplayText: "Link 1", IsResolved: true},
	}

	np.UpdateLinks("note1", outgoing, []LinkDisplayItem{})

	// Set initial size
	np.SetSize(80, 24)

	// Cursor at some position
	np.cursor = 2

	// Resize to smaller
	np.SetSize(40, 12)

	// Cursor should be adjusted if it's out of bounds for new viewport
	// View should still render correctly
	view := np.View()

	if view == "" {
		t.Error("View should render after resize")
	}
}

// TestNotesPane_LoadingState tests loading state display
func TestNotesPane_LoadingState(t *testing.T) {
	np := NewNotesPane()

	np.SetLoading("note1", true)

	if !np.loading {
		t.Error("SetLoading(true) should set loading state")
	}

	if np.currentNoteID != "note1" {
		t.Errorf("expected currentNoteID 'note1', got '%s'", np.currentNoteID)
	}

	view := np.View()

	if view == "" {
		t.Error("View should render loading state")
	}

	// Clear loading state
	np.SetLoading("note1", false)

	if np.loading {
		t.Error("SetLoading(false) should clear loading state")
	}
}

// TestNotesPane_StaleDataHandling tests ignoring stale link updates
func TestNotesPane_StaleDataHandling(t *testing.T) {
	np := NewNotesPane()

	// Set current note
	np.UpdateLinks("note1", []LinkDisplayItem{
		{NoteLink: models.NoteLink{TargetSlug: "link1"}, DisplayText: "Link 1", IsResolved: true},
	}, []LinkDisplayItem{})

	// User rapidly switches to note2
	np.currentNoteID = "note2"

	// Stale update for note1 arrives
	staleOutgoing := []LinkDisplayItem{
		{NoteLink: models.NoteLink{TargetSlug: "stale"}, DisplayText: "Stale Link", IsResolved: true},
	}

	// UpdateLinks should check if noteID matches currentNoteID
	// and ignore stale updates
	np.UpdateLinks("note1", staleOutgoing, []LinkDisplayItem{})

	// If implementation correctly handles stale data,
	// the links should not be updated since currentNoteID is now "note2"
	// Test will verify this once implemented
}

// TestNotesPane_CircularLinks tests display of circular links
func TestNotesPane_CircularLinks(t *testing.T) {
	np := NewNotesPane()

	// Note A links to Note B, and Note B appears in backlinks
	// (meaning Note B links back to Note A)
	outgoing := []LinkDisplayItem{
		{
			NoteLink: models.NoteLink{
				TargetSlug:   "note-b",
				TargetNoteID: "note-b-id",
			},
			DisplayText: "Note B",
			IsResolved:  true,
		},
	}

	backlinks := []LinkDisplayItem{
		{
			NoteLink: models.NoteLink{
				SourceNoteID: "note-b-id",
				TargetSlug:   "note-a",
			},
			DisplayText: "Note B",
			IsResolved:  true,
		},
	}

	np.UpdateLinks("note-a-id", outgoing, backlinks)

	// Both sections should render the same note
	view := np.View()

	if view == "" {
		t.Error("View should render circular links")
	}

	// Verify both sections are present
	if len(np.outgoingLinks) != 1 {
		t.Error("expected 1 outgoing link")
	}

	if len(np.backlinks) != 1 {
		t.Error("expected 1 backlink")
	}
}

// TestNotesPane_LongTitles tests truncation of long note titles
func TestNotesPane_LongTitles(t *testing.T) {
	np := NewNotesPane()

	longTitle := "This is a very long note title that exceeds the normal width of the pane and should be truncated with ellipsis to fit"

	outgoing := []LinkDisplayItem{
		{
			NoteLink: models.NoteLink{
				TargetSlug: "long-note",
				TargetNote: &models.Note{Title: longTitle, Slug: "long-note"},
			},
			DisplayText: longTitle,
			IsResolved:  true,
		},
	}

	np.UpdateLinks("note1", outgoing, []LinkDisplayItem{})
	np.SetSize(40, 24) // Narrow width

	view := np.View()

	if view == "" {
		t.Error("View should render with long titles")
	}

	// View should truncate or wrap long titles
	// Test will verify once rendering is implemented
}

// TestNotesPane_EmptySections tests rendering when one section is empty
func TestNotesPane_EmptySections(t *testing.T) {
	np := NewNotesPane()

	t.Run("empty outgoing, non-empty backlinks", func(t *testing.T) {
		backlinks := []LinkDisplayItem{
			{NoteLink: models.NoteLink{TargetSlug: "current"}, DisplayText: "Backlink", IsResolved: true},
		}

		np.UpdateLinks("note1", []LinkDisplayItem{}, backlinks)

		view := np.View()

		if view == "" {
			t.Error("View should render with empty outgoing section")
		}

		// Should show "No outgoing links" but still render backlinks section
	})

	t.Run("empty backlinks, non-empty outgoing", func(t *testing.T) {
		outgoing := []LinkDisplayItem{
			{NoteLink: models.NoteLink{TargetSlug: "target"}, DisplayText: "Target", IsResolved: true},
		}

		np.UpdateLinks("note1", outgoing, []LinkDisplayItem{})

		view := np.View()

		if view == "" {
			t.Error("View should render with empty backlinks section")
		}

		// Should show "No backlinks" but still render outgoing section
	})
}

// TestNotesPane_CollapsedStatePersistence tests collapse state persists across updates
func TestNotesPane_CollapsedStatePersistence(t *testing.T) {
	np := NewNotesPane()

	outgoing := []LinkDisplayItem{
		{NoteLink: models.NoteLink{TargetSlug: "link1"}, DisplayText: "Link 1", IsResolved: true},
	}

	np.UpdateLinks("note1", outgoing, []LinkDisplayItem{})

	// Collapse outgoing section
	np.outgoingCollapsed = true

	// Reload same note (links update)
	newOutgoing := []LinkDisplayItem{
		{NoteLink: models.NoteLink{TargetSlug: "link1"}, DisplayText: "Link 1", IsResolved: true},
		{NoteLink: models.NoteLink{TargetSlug: "link2"}, DisplayText: "Link 2", IsResolved: true},
	}

	np.UpdateLinks("note1", newOutgoing, []LinkDisplayItem{})

	// Collapsed state should persist for same note
	if !np.outgoingCollapsed {
		t.Error("collapsed state should persist when reloading same note")
	}

	// Switch to different note (must call SetLoading first to update currentNoteID)
	np.SetLoading("note2", true)
	np.UpdateLinks("note2", outgoing, []LinkDisplayItem{})

	// Collapsed state should reset for new note
	if np.outgoingCollapsed {
		t.Error("collapsed state should reset when switching to different note")
	}
}

// TestLinkDisplayItem_Construction tests LinkDisplayItem helper construction
func TestLinkDisplayItem_Construction(t *testing.T) {
	t.Run("resolved link with target note", func(t *testing.T) {
		noteLink := models.NoteLink{
			ID:           "link1",
			SourceNoteID: "source",
			TargetSlug:   "target-slug",
			TargetNoteID: "target-id",
			TargetNote: &models.Note{
				ID:    "target-id",
				Title: "Target Title",
				Slug:  "target-slug",
			},
		}

		item := LinkDisplayItem{
			NoteLink:    noteLink,
			DisplayText: noteLink.TargetNote.Title,
			IsResolved:  noteLink.TargetNoteID != "",
		}

		if !item.IsResolved {
			t.Error("link with TargetNoteID should be resolved")
		}

		if item.DisplayText != "Target Title" {
			t.Errorf("expected display text 'Target Title', got '%s'", item.DisplayText)
		}
	})

	t.Run("unresolved link without target note", func(t *testing.T) {
		noteLink := models.NoteLink{
			ID:           "link1",
			SourceNoteID: "source",
			TargetSlug:   "unresolved-slug",
			TargetNoteID: "",
		}

		item := LinkDisplayItem{
			NoteLink:    noteLink,
			DisplayText: noteLink.TargetSlug,
			IsResolved:  noteLink.TargetNoteID != "",
		}

		if item.IsResolved {
			t.Error("link without TargetNoteID should be unresolved")
		}

		if item.DisplayText != "unresolved-slug" {
			t.Errorf("expected display text 'unresolved-slug', got '%s'", item.DisplayText)
		}
	})
}

// TestLinkSelectedMsg_Creation tests LinkSelectedMsg message creation
func TestLinkSelectedMsg_Creation(t *testing.T) {
	msg := LinkSelectedMsg{
		NoteSlug: "test-slug",
		NoteID:   "test-id",
	}

	if msg.NoteSlug != "test-slug" {
		t.Errorf("expected slug 'test-slug', got '%s'", msg.NoteSlug)
	}

	if msg.NoteID != "test-id" {
		t.Errorf("expected ID 'test-id', got '%s'", msg.NoteID)
	}
}

// TestNotesPane_ArrowKeyNavigation tests arrow key navigation (alternative to j/k)
func TestNotesPane_ArrowKeyNavigation(t *testing.T) {
	np := NewNotesPane()
	np.focused = true

	outgoing := []LinkDisplayItem{
		{NoteLink: models.NoteLink{TargetSlug: "link1"}, DisplayText: "Link 1", IsResolved: true},
		{NoteLink: models.NoteLink{TargetSlug: "link2"}, DisplayText: "Link 2", IsResolved: true},
	}

	np.UpdateLinks("note1", outgoing, []LinkDisplayItem{})

	t.Run("down arrow moves cursor down", func(t *testing.T) {
		initialCursor := np.cursor
		msg := tea.KeyMsg{Type: tea.KeyDown}
		np, _ = np.Update(msg)

		if np.cursor <= initialCursor {
			t.Error("down arrow should move cursor down")
		}
	})

	t.Run("up arrow moves cursor up", func(t *testing.T) {
		np.cursor = 1
		msg := tea.KeyMsg{Type: tea.KeyUp}
		np, _ = np.Update(msg)

		if np.cursor != 0 {
			t.Error("up arrow should move cursor up")
		}
	})
}

// TestNotesPane_ViewportScrolling tests viewport scrolling for many links
func TestNotesPane_ViewportScrolling(t *testing.T) {
	np := NewNotesPane()

	// Create many links to test scrolling
	outgoing := make([]LinkDisplayItem, 50)
	for i := 0; i < 50; i++ {
		outgoing[i] = LinkDisplayItem{
			NoteLink:    models.NoteLink{TargetSlug: "link-" + string(rune(i))},
			DisplayText: "Link " + string(rune(i)),
			IsResolved:  true,
		}
	}

	np.UpdateLinks("note1", outgoing, []LinkDisplayItem{})
	np.SetSize(80, 10) // Small height to force scrolling

	// Navigate to bottom
	for i := 0; i < 60; i++ {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}
		np, _ = np.Update(msg)
	}

	view := np.View()

	if view == "" {
		t.Error("View should render with scrolling")
	}

	// Viewport should scroll to show current cursor position
	// Test will verify scroll offset calculation once implemented
}
