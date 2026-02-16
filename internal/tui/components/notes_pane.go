package components

import (
	"fmt"
	"strings"

	"github.com/MikeBiancalana/reckon/internal/models"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	notePaneStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	focusedNotePaneStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("11"))

	notePaneTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("205")).
				Bold(true)

	focusedNotePaneTitleStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("11")).
					Bold(true)

	sectionHeaderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("205")).
				Bold(true)

	dimmedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	unresolvedLinkStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240")).
				Italic(true)
)

// LinkDisplayItem represents a link for display in the notes pane
type LinkDisplayItem struct {
	NoteLink    models.NoteLink
	DisplayText string // Title if available, otherwise slug
	IsResolved  bool   // Whether the target/source note exists
}

// LinkSelectedMsg is emitted when user presses enter on a link
type LinkSelectedMsg struct {
	NoteSlug string
	NoteID   string
}

// NotesPane represents the notes component
type NotesPane struct {
	// Data
	outgoingLinks []LinkDisplayItem
	backlinks     []LinkDisplayItem

	// UI state
	focused            bool
	cursor             int // Global cursor position across both sections
	outgoingCollapsed  bool
	backlinksCollapsed bool

	// Dimensions
	width  int
	height int

	// Context
	currentNoteID string
	loading       bool
	errorMsg      string

	// Scroll offset for viewport
	scrollOffset int
}

func NewNotesPane() *NotesPane {
	return &NotesPane{
		outgoingLinks:      []LinkDisplayItem{},
		backlinks:          []LinkDisplayItem{},
		focused:            false,
		cursor:             0,
		outgoingCollapsed:  false,
		backlinksCollapsed: false,
		width:              0,
		height:             0,
		currentNoteID:      "",
		loading:            false,
		errorMsg:           "",
		scrollOffset:       0,
	}
}

// Update handles messages for the notes pane
func (np *NotesPane) Update(msg tea.Msg) (*NotesPane, tea.Cmd) {
	if !np.focused {
		return np, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			np.moveCursorDown()
			np.adjustScrollOffset()
		case "k", "up":
			np.moveCursorUp()
			np.adjustScrollOffset()
		case "g":
			np.cursor = 0
			np.scrollOffset = 0
		case "G":
			np.cursor = np.maxCursor()
			np.adjustScrollOffset()
		case "enter":
			return np, np.selectCurrentLink()
		case " ":
			np.toggleSectionAtCursor()
		}
	}

	return np, nil
}

// View renders the notes pane
func (np *NotesPane) View() string {
	if np.currentNoteID == "" {
		return np.renderEmptyState("Select a note to see its links")
	}

	if np.loading {
		return np.renderEmptyState("Loading links...")
	}

	if len(np.outgoingLinks) == 0 && len(np.backlinks) == 0 {
		return np.renderEmptyState("No linked notes found")
	}

	var sb strings.Builder
	sb.WriteString(sectionHeaderStyle.Render("Linked Notes"))
	sb.WriteString("\n\n")

	currentLine := 0

	// Render Outgoing Links section
	currentLine += np.renderSection(&sb, "Outgoing Links", np.outgoingLinks, np.outgoingCollapsed, currentLine, false)

	// Render Backlinks section
	np.renderSection(&sb, "Backlinks", np.backlinks, np.backlinksCollapsed, currentLine, true)

	// Apply scroll offset and viewport clipping
	content := sb.String()
	lines := strings.Split(content, "\n")
	// Apply scroll offset
	if np.scrollOffset > 0 && np.scrollOffset < len(lines) {
		lines = lines[np.scrollOffset:]
	}
	visibleLines := np.height - 2 // title + padding
	if visibleLines > 0 && len(lines) > visibleLines {
		lines = lines[:visibleLines]
	}
	return strings.Join(lines, "\n")
}

// renderEmptyState renders a centered empty state message
func (np *NotesPane) renderEmptyState(message string) string {
	style := dimmedStyle
	if np.focused {
		style = focusedNotePaneStyle
	}
	return style.Render(message)
}

// renderSection renders a collapsible section with links
func (np *NotesPane) renderSection(sb *strings.Builder, title string, links []LinkDisplayItem, collapsed bool, startLine int, isBacklink bool) int {
	indicator := CollapseIndicatorExpanded
	if collapsed {
		indicator = CollapseIndicatorCollapsed
	}

	header := fmt.Sprintf("%s%s (%d)", indicator, title, len(links))

	// Check if cursor is on header
	if startLine == np.cursor && np.focused {
		header = SelectedStyle.Render(header)
	} else {
		header = sectionHeaderStyle.Render(header)
	}

	sb.WriteString(header)
	sb.WriteString("\n")

	linesRendered := 1

	if !collapsed {
		if len(links) == 0 {
			emptyMsg := fmt.Sprintf("  No %s", strings.ToLower(title))
			sb.WriteString(dimmedStyle.Render(emptyMsg))
			sb.WriteString("\n")
			linesRendered++
		} else {
			for i, link := range links {
				lineIndex := startLine + 1 + i
				linkText := np.formatLinkItem(link, isBacklink)

				// Apply cursor highlight
				if lineIndex == np.cursor && np.focused {
					linkText = SelectedStyle.Render(linkText)
				} else if !link.IsResolved {
					linkText = unresolvedLinkStyle.Render(linkText)
				} else {
					linkText = notePaneStyle.Render(linkText)
				}

				sb.WriteString(linkText)
				sb.WriteString("\n")
				linesRendered++
			}
		}
	}

	return linesRendered
}

// formatLinkItem formats a link for display
func (np *NotesPane) formatLinkItem(link LinkDisplayItem, isBacklink bool) string {
	prefix := "  "
	slug := link.NoteLink.TargetSlug

	// For backlinks, show the source slug instead of target
	if isBacklink && link.NoteLink.SourceNote != nil {
		slug = link.NoteLink.SourceNote.Slug
	}

	if link.IsResolved {
		// Show title with slug in brackets
		return fmt.Sprintf("%s[[%s]] %s", prefix, slug, link.DisplayText)
	} else {
		// Unresolved link - just show slug
		return fmt.Sprintf("%s[[%s]] (unresolved)", prefix, slug)
	}
}

// SetSize sets the size of the pane
func (np *NotesPane) SetSize(width, height int) {
	np.width = width
	np.height = height
	np.adjustScrollOffset()
}

// SetFocused sets whether this component is focused
func (np *NotesPane) SetFocused(focused bool) {
	np.focused = focused
}

// UpdateLinks updates the pane with outgoing links and backlinks
func (np *NotesPane) UpdateLinks(noteID string, outgoing []LinkDisplayItem, backlinks []LinkDisplayItem) {
	// Ignore stale updates
	if np.currentNoteID != "" && noteID != np.currentNoteID {
		return
	}

	np.currentNoteID = noteID
	np.outgoingLinks = outgoing
	np.backlinks = backlinks
	np.loading = false
	np.cursor = 0
	np.scrollOffset = 0
}

// SetLoading sets the loading state for a note
func (np *NotesPane) SetLoading(noteID string, loading bool) {
	// Reset collapse state when switching to a different note
	if noteID != np.currentNoteID {
		np.outgoingCollapsed = false
		np.backlinksCollapsed = false
	}
	np.currentNoteID = noteID
	np.loading = loading
}

// moveCursorDown moves the cursor to the next valid position
func (np *NotesPane) moveCursorDown() {
	maxCursor := np.maxCursor()
	if np.cursor >= maxCursor {
		return
	}
	np.cursor++

	// Skip over items in collapsed sections
	for np.cursor <= maxCursor && np.isLineInCollapsedSection(np.cursor) {
		np.cursor++
	}

	if np.cursor > maxCursor {
		np.cursor = maxCursor
	}
}

// moveCursorUp moves the cursor to the previous valid position
func (np *NotesPane) moveCursorUp() {
	if np.cursor <= 0 {
		return
	}
	np.cursor--

	// Skip over items in collapsed sections
	for np.cursor >= 0 && np.isLineInCollapsedSection(np.cursor) {
		np.cursor--
	}

	if np.cursor < 0 {
		np.cursor = 0
	}
}

// maxCursor returns the maximum valid cursor position
func (np *NotesPane) maxCursor() int {
	total := 0

	// Outgoing section header
	total++ // header line
	if !np.outgoingCollapsed {
		if len(np.outgoingLinks) == 0 {
			total++ // "No outgoing links" line
		} else {
			total += len(np.outgoingLinks)
		}
	}

	// Backlinks section header
	total++ // header line
	if !np.backlinksCollapsed {
		if len(np.backlinks) == 0 {
			total++ // "No backlinks" line
		} else {
			total += len(np.backlinks)
		}
	}

	return total - 1 // Convert count to index
}

// isLineInCollapsedSection checks if a line index is within a collapsed section
func (np *NotesPane) isLineInCollapsedSection(lineIndex int) bool {
	currentLine := 0

	// Outgoing section
	outgoingHeaderLine := currentLine
	currentLine++

	if np.outgoingCollapsed {
		// Lines after outgoing header until backlinks header are collapsed
		outgoingEndLine := currentLine + len(np.outgoingLinks)
		if lineIndex > outgoingHeaderLine && lineIndex < outgoingEndLine {
			return true
		}
	} else {
		if len(np.outgoingLinks) == 0 {
			currentLine++ // empty line
		} else {
			currentLine += len(np.outgoingLinks)
		}
	}

	// Backlinks section
	backlinksHeaderLine := currentLine
	currentLine++

	if np.backlinksCollapsed {
		backlinksEndLine := currentLine + len(np.backlinks)
		if lineIndex > backlinksHeaderLine && lineIndex < backlinksEndLine {
			return true
		}
	}

	return false
}

// toggleSectionAtCursor toggles the collapse state of the section at cursor
func (np *NotesPane) toggleSectionAtCursor() {
	currentLine := 0

	// Check if cursor is on outgoing header
	if np.cursor == currentLine {
		np.outgoingCollapsed = !np.outgoingCollapsed
		return
	}

	currentLine++ // outgoing header
	if !np.outgoingCollapsed {
		if len(np.outgoingLinks) == 0 {
			currentLine++
		} else {
			currentLine += len(np.outgoingLinks)
		}
	}

	// Check if cursor is on backlinks header
	if np.cursor == currentLine {
		np.backlinksCollapsed = !np.backlinksCollapsed
		return
	}
}

// selectCurrentLink returns a command that emits LinkSelectedMsg
func (np *NotesPane) selectCurrentLink() tea.Cmd {
	link := np.getLinkAtCursor()
	if link == nil {
		return nil
	}

	return func() tea.Msg {
		return LinkSelectedMsg{
			NoteSlug: link.NoteLink.TargetSlug,
			NoteID:   link.NoteLink.TargetNoteID,
		}
	}
}

// getLinkAtCursor returns the link at the current cursor position, or nil
func (np *NotesPane) getLinkAtCursor() *LinkDisplayItem {
	currentLine := 0

	// Skip outgoing header
	currentLine++

	// Check outgoing links
	if !np.outgoingCollapsed {
		if len(np.outgoingLinks) > 0 {
			for i := range np.outgoingLinks {
				if currentLine == np.cursor {
					return &np.outgoingLinks[i]
				}
				currentLine++
			}
		} else {
			currentLine++ // empty line
		}
	}

	// Skip backlinks header
	currentLine++

	// Check backlinks
	if !np.backlinksCollapsed {
		if len(np.backlinks) > 0 {
			for i := range np.backlinks {
				if currentLine == np.cursor {
					// For backlinks, we want to navigate to the source note
					bl := &np.backlinks[i]
					// Create a copy with swapped source/target for navigation
					return &LinkDisplayItem{
						NoteLink: models.NoteLink{
							TargetSlug:   bl.NoteLink.SourceNote.Slug,
							TargetNoteID: bl.NoteLink.SourceNoteID,
						},
						DisplayText: bl.DisplayText,
						IsResolved:  bl.IsResolved,
					}
				}
				currentLine++
			}
		}
	}

	return nil
}

// adjustScrollOffset adjusts scroll offset to keep cursor in view
func (np *NotesPane) adjustScrollOffset() {
	if np.height <= 0 {
		return
	}

	visibleLines := np.height - 2 // Account for title and padding

	// Cursor is below viewport
	if np.cursor >= np.scrollOffset+visibleLines {
		np.scrollOffset = np.cursor - visibleLines + 1
	}

	// Cursor is above viewport
	if np.cursor < np.scrollOffset {
		np.scrollOffset = np.cursor
	}

	// Ensure scroll offset is not negative
	if np.scrollOffset < 0 {
		np.scrollOffset = 0
	}
}
