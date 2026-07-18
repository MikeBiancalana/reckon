package components

import (
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/MikeBiancalana/reckon/internal/logger"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	logStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("99")).
			Bold(true)

	focusedLogStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")).
			Bold(true)
)

// LogEntryRow is the flat, journal-free shape LogView renders: one row per
// index `log-entry` node (internal/node/logparser.go), decoupled from
// internal/journal.LogEntry. Log entries have no nested-notes source or verb
// in v1, so the Notes/LogNote machinery this component used to carry
// (findLogNoteText, LogNoteAddMsg/LogNoteDeleteMsg, the n/d note keys,
// SelectedLogNote, IsSelectedItemNote, note-collapse state) is dropped, not
// decoupled.
type LogEntryRow struct {
	ID        string
	Timestamp time.Time
	Content   string
	EntryType string
}

// LogEntryItem represents a log entry in the list
type LogEntryItem struct {
	entry LogEntryRow
}

func (l LogEntryItem) FilterValue() string { return l.entry.Content }

// LogDelegate handles rendering of log entry items
type LogDelegate struct {
	width int
}

func (d LogDelegate) Height() int                               { return 1 }
func (d LogDelegate) Spacing() int                              { return 0 }
func (d LogDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return nil }

func (d LogDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	item, ok := listItem.(LogEntryItem)
	if !ok {
		return
	}

	// Render log entry with icon
	timeStr := item.entry.Timestamp.Format("15:04")
	var icon string
	switch item.entry.EntryType {
	case "meeting":
		icon = "📅"
	case "break":
		icon = "☕"
	default:
		icon = "📝"
	}

	text := fmt.Sprintf("%s %s: %s", timeStr, icon, item.entry.Content)

	// Truncate to available width to prevent pane from expanding horizontally
	if d.width > 0 {
		text = lipgloss.NewStyle().MaxWidth(d.width).Render(text)
	}

	// Highlight selected item
	if index == m.Index() {
		text = SelectedStyle.Render(text)
	} else {
		text = logStyle.Render(text)
	}

	fmt.Fprintf(w, "%s", text)
}

// buildLogItems converts log entries into list items.
func buildLogItems(logEntries []LogEntryRow) []list.Item {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("log_view: panic in buildLogItems", "error", r, slog.String("stack", fmt.Sprintf("%v", r)))
		}
	}()

	items := make([]list.Item, 0)

	for _, entry := range logEntries {
		if entry.ID == "" {
			logger.Warn("log_view: skipping log entry with empty ID")
			continue
		}

		items = append(items, LogEntryItem{entry: entry})
	}

	return items
}

// LogView represents the log entries component
type LogView struct {
	list       list.Model
	logEntries []LogEntryRow // keep track of original log entries for state management
	focused    bool
	width      int
}

func NewLogView(logEntries []LogEntryRow) *LogView {
	items := buildLogItems(logEntries)

	delegate := LogDelegate{}
	l := list.New(items, delegate, 0, 0)
	l.Title = "Log Entries"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = logStyle

	return &LogView{
		list:       l,
		logEntries: logEntries,
	}
}

// Update handles messages for the log view
func (lv *LogView) Update(msg tea.Msg) (*LogView, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if !lv.focused {
			var cmd tea.Cmd
			lv.list, cmd = lv.list.Update(msg)
			return lv, cmd
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
	lv.width = width
	lv.list.SetSize(width, height)
	lv.list.SetDelegate(LogDelegate{width: lv.width})
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
func (lv *LogView) UpdateLogEntries(logEntries []LogEntryRow) {
	// Preserve cursor position by finding the currently selected log entry ID
	selectedItem := lv.list.SelectedItem()
	var selectedLogEntryID string
	if selectedItem != nil {
		if logItem, ok := selectedItem.(LogEntryItem); ok {
			selectedLogEntryID = logItem.entry.ID
		}
	}

	lv.logEntries = logEntries
	items := buildLogItems(logEntries)
	lv.list.SetItems(items)

	// Restore cursor to the previously selected log entry
	if selectedLogEntryID != "" {
		for i, item := range items {
			if logItem, ok := item.(LogEntryItem); ok && logItem.entry.ID == selectedLogEntryID {
				lv.list.Select(i)
				break
			}
		}
	}

	lv.list.SetDelegate(LogDelegate{width: lv.width})
}

// SelectedLogEntry returns the currently selected log entry
func (lv *LogView) SelectedLogEntry() *LogEntryRow {
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
