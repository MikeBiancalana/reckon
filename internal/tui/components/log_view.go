package components

import (
	"fmt"
	"io"

	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	logStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("99")).
			Bold(true)

	logNoteStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	focusedLogStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")).
			Bold(true)
)

// LogEntryItem represents a log entry or note in the list
type LogEntryItem struct {
	entry      journal.LogEntry
	isNote     bool
	noteID     string
	logEntryID string // parent log entry ID for notes
}

func (l LogEntryItem) FilterValue() string { return l.entry.Content }

// findLogNoteText finds the text of a log note by ID
func findLogNoteText(notes []journal.LogNote, noteID string) string {
	for _, note := range notes {
		if note.ID == noteID {
			return note.Text
		}
	}
	return ""
}

// LogDelegate handles rendering of log entry items
type LogDelegate struct {
	collapsedMap map[string]bool // logEntryID -> isCollapsed
}

func (d LogDelegate) Height() int                               { return 1 }
func (d LogDelegate) Spacing() int                              { return 0 }
func (d LogDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return nil }

func (d LogDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	item, ok := listItem.(LogEntryItem)
	if !ok {
		return
	}

	var text string
	var style lipgloss.Style

	if item.isNote {
		// Render note with 2-space indent
		text = fmt.Sprintf("  - %s", findLogNoteText(item.entry.Notes, item.noteID))
		style = logNoteStyle
	} else {
		// Render log entry with icon
		timeStr := item.entry.Timestamp.Format("15:04")
		var icon string
		switch item.entry.EntryType {
		case journal.EntryTypeMeeting:
			icon = "📅"
		case journal.EntryTypeBreak:
			icon = "☕"
		default:
			icon = "📝"
		}

		// Add expand/collapse indicator if entry has notes
		indicator := ""
		if len(item.entry.Notes) > 0 {
			if d.collapsedMap[item.entry.ID] {
				indicator = "▶ "
			} else {
				indicator = "▼ "
			}
		}

		text = fmt.Sprintf("%s%s %s: %s", indicator, timeStr, icon, item.entry.Content)
		style = logStyle
	}

	// Highlight selected item
	if index == m.Index() {
		text = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true).Render("▶ " + text)
	} else {
		text = style.Render(text)
	}

	fmt.Fprintf(w, "%s", text)
}

// buildLogItems converts log entries into list items, respecting collapsed state
func buildLogItems(logEntries []journal.LogEntry, collapsedMap map[string]bool) []list.Item {
	items := make([]list.Item, 0)

	for _, entry := range logEntries {
		// Add the log entry itself
		items = append(items, LogEntryItem{
			entry:      entry,
			isNote:     false,
			logEntryID: entry.ID,
		})

		// Add notes if entry is not collapsed
		if !collapsedMap[entry.ID] && len(entry.Notes) > 0 {
			for _, note := range entry.Notes {
				items = append(items, LogEntryItem{
					entry:      entry,
					isNote:     true,
					noteID:     note.ID,
					logEntryID: entry.ID,
				})
			}
		}
	}

	return items
}

// LogView represents the log entries component
type LogView struct {
	list         list.Model
	collapsedMap map[string]bool
	logEntries   []journal.LogEntry // keep track of original log entries for state management
	focused      bool
}

func NewLogView(logEntries []journal.LogEntry) *LogView {
	collapsedMap := make(map[string]bool)
	items := buildLogItems(logEntries, collapsedMap)

	delegate := LogDelegate{collapsedMap: collapsedMap}
	l := list.New(items, delegate, 0, 0)
	l.Title = "Log Entries"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = logStyle

	return &LogView{
		list:         l,
		collapsedMap: collapsedMap,
		logEntries:   logEntries,
	}
}

// LogNoteAddMsg is sent when a log note should be added
type LogNoteAddMsg struct {
	LogEntryID string
}

// LogNoteDeleteMsg is sent when a log note should be deleted
type LogNoteDeleteMsg struct {
	LogEntryID string
	NoteID     string
}

// Update handles messages for the log view
func (lv *LogView) Update(msg tea.Msg) (*LogView, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "n":
			// Add note to selected log entry
			selectedItem := lv.list.SelectedItem()
			if selectedItem != nil {
				logItem, ok := selectedItem.(LogEntryItem)
				if ok && !logItem.isNote {
					// Return a message to add a note to this log entry
					return lv, func() tea.Msg {
						return LogNoteAddMsg{LogEntryID: logItem.entry.ID}
					}
				}
			}
			return lv, nil
		case "d":
			// Delete selected note
			selectedItem := lv.list.SelectedItem()
			if selectedItem != nil {
				logItem, ok := selectedItem.(LogEntryItem)
				if ok && logItem.isNote {
					// Return a message to delete this note
					return lv, func() tea.Msg {
						return LogNoteDeleteMsg{
							LogEntryID: logItem.logEntryID,
							NoteID:     logItem.noteID,
						}
					}
				}
			}
			return lv, nil
		case "enter", " ":
			// Toggle expand/collapse
			selectedItem := lv.list.SelectedItem()
			if selectedItem != nil {
				logItem, ok := selectedItem.(LogEntryItem)
				if ok && !logItem.isNote && len(logItem.entry.Notes) > 0 {
					// Toggle collapsed state
					lv.collapsedMap[logItem.entry.ID] = !lv.collapsedMap[logItem.entry.ID]

					// Rebuild items with new collapsed state
					items := buildLogItems(lv.logEntries, lv.collapsedMap)
					lv.list.SetItems(items)

					// Update delegate with new collapsed map
					delegate := LogDelegate{collapsedMap: lv.collapsedMap}
					lv.list.SetDelegate(delegate)

					// If collapsing and cursor was on a note, move back to log entry
					currentIndex := lv.list.Index()
					if currentIndex < len(items) {
						currentItem, ok := items[currentIndex].(LogEntryItem)
						if ok && currentItem.isNote && lv.collapsedMap[logItem.entry.ID] {
							// Find the log entry item index
							for i, item := range items {
								if li, ok := item.(LogEntryItem); ok && !li.isNote && li.entry.ID == logItem.entry.ID {
									lv.list.Select(i)
									break
								}
							}
						}
					}
				}
			}
			return lv, nil
		}
	}

	var cmd tea.Cmd
	lv.list, cmd = lv.list.Update(msg)
	return lv, cmd
}

// View renders the log view
func (lv *LogView) View() string {
	if len(lv.list.Items()) == 0 {
		return "Log Entries\n\nNo log entries yet - press L to add one"
	}
	return lv.list.View()
}

// SetSize sets the size of the list
func (lv *LogView) SetSize(width, height int) {
	lv.list.SetSize(width, height)
}

// SetFocused sets whether this component is focused
func (lv *LogView) SetFocused(focused bool) {
	lv.focused = focused
	if focused {
		lv.list.Styles.Title = focusedLogStyle
	} else {
		lv.list.Styles.Title = logStyle
	}
}

// UpdateLogEntries updates the list with new log entries
func (lv *LogView) UpdateLogEntries(logEntries []journal.LogEntry) {
	lv.logEntries = logEntries
	items := buildLogItems(logEntries, lv.collapsedMap)
	lv.list.SetItems(items)

	// Update delegate with current collapsed map
	delegate := LogDelegate{collapsedMap: lv.collapsedMap}
	lv.list.SetDelegate(delegate)
}

// SelectedLogEntry returns the currently selected log entry
func (lv *LogView) SelectedLogEntry() *journal.LogEntry {
	item := lv.list.SelectedItem()
	if item == nil {
		return nil
	}
	logItem, ok := item.(LogEntryItem)
	if !ok {
		return nil
	}
	// Create a copy and return pointer to it
	entry := logItem.entry
	return &entry
}

// IsSelectedItemNote returns true if the currently selected item is a note
func (lv *LogView) IsSelectedItemNote() bool {
	item := lv.list.SelectedItem()
	if item == nil {
		return false
	}
	logItem, ok := item.(LogEntryItem)
	if !ok {
		return false
	}
	return logItem.isNote
}
