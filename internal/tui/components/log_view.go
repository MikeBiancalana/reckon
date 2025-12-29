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
)

// LogEntryItem represents a log entry in the list
type LogEntryItem struct {
	entry journal.LogEntry
}

func (l LogEntryItem) FilterValue() string { return l.entry.Content }

// LogDelegate handles rendering of log entry items
type LogDelegate struct{}

func (d LogDelegate) Height() int                               { return 1 }
func (d LogDelegate) Spacing() int                              { return 0 }
func (d LogDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return nil }

func (d LogDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	le, ok := listItem.(LogEntryItem)
	if !ok {
		return
	}

	timeStr := le.entry.Timestamp.Format("15:04")
	var icon string
	switch le.entry.EntryType {
	case journal.EntryTypeMeeting:
		icon = "📅"
	case journal.EntryTypeBreak:
		icon = "☕"
	default:
		icon = "📝"
	}

	text := fmt.Sprintf("%s %s: %s", timeStr, icon, le.entry.Content)

	if index == m.Index() {
		text = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true).Render("▶ " + text)
	} else {
		text = logStyle.Render(text)
	}

	fmt.Fprintf(w, "%s", text)
}

// LogView represents the log entries component
type LogView struct {
	list list.Model
}

func NewLogView(logEntries []journal.LogEntry) *LogView {
	items := make([]list.Item, len(logEntries))
	for i, entry := range logEntries {
		items[i] = LogEntryItem{entry}
	}

	l := list.New(items, LogDelegate{}, 0, 0)
	l.Title = "Log Entries"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = logStyle

	return &LogView{list: l}
}

// Update handles messages for the log view
func (lv *LogView) Update(msg tea.Msg) (*LogView, tea.Cmd) {
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

// UpdateLogEntries updates the list with new log entries
func (lv *LogView) UpdateLogEntries(logEntries []journal.LogEntry) {
	items := make([]list.Item, len(logEntries))
	for i, entry := range logEntries {
		items[i] = LogEntryItem{entry}
	}
	lv.list.SetItems(items)
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
