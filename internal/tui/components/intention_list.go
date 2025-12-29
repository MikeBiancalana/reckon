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
	intentionStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("205")).
			Bold(true)

	doneStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Strikethrough(true)

	openStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39"))

	carriedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214"))
)

// IntentionItem represents an intention in the list
type IntentionItem struct {
	intention journal.Intention
}

func (i IntentionItem) FilterValue() string { return i.intention.Text }

// IntentionDelegate handles rendering of intention items
type IntentionDelegate struct{}

func (d IntentionDelegate) Height() int                               { return 1 }
func (d IntentionDelegate) Spacing() int                              { return 0 }
func (d IntentionDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return nil }

func (d IntentionDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(IntentionItem)
	if !ok {
		return
	}

	var style lipgloss.Style
	var status string

	switch i.intention.Status {
	case journal.IntentionDone:
		style = doneStyle
		status = "✓"
	case journal.IntentionCarried:
		style = carriedStyle
		status = "→"
	default:
		style = openStyle
		status = "○"
	}

	text := fmt.Sprintf("%s %s", status, i.intention.Text)
	if i.intention.Status == journal.IntentionCarried && i.intention.CarriedFrom != "" {
		text += fmt.Sprintf(" (from %s)", i.intention.CarriedFrom)
	}

	if index == m.Index() {
		text = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true).Render("▶ " + text)
	} else {
		text = style.Render(text)
	}

	fmt.Fprintf(w, "%s", text)
}

// IntentionList represents the intentions component
type IntentionList struct {
	list list.Model
}

func NewIntentionList(intentions []journal.Intention) *IntentionList {
	items := make([]list.Item, len(intentions))
	for i, intention := range intentions {
		items[i] = IntentionItem{intention}
	}

	l := list.New(items, IntentionDelegate{}, 0, 0)
	l.Title = "Intentions"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = intentionStyle

	return &IntentionList{list: l}
}

// Update handles messages for the intention list
func (il *IntentionList) Update(msg tea.Msg) (*IntentionList, tea.Cmd) {
	var cmd tea.Cmd
	il.list, cmd = il.list.Update(msg)
	return il, cmd
}

// View renders the intention list
func (il *IntentionList) View() string {
	if len(il.list.Items()) == 0 {
		return "Intentions\n\nNo intentions yet - press i to add one"
	}
	return il.list.View()
}

// SetSize sets the size of the list
func (il *IntentionList) SetSize(width, height int) {
	il.list.SetSize(width, height)
}

// SelectedIntention returns the currently selected intention
func (il *IntentionList) SelectedIntention() *journal.Intention {
	item := il.list.SelectedItem()
	if item == nil {
		return nil
	}
	intentionItem, ok := item.(IntentionItem)
	if !ok {
		return nil
	}
	// Create a copy and return pointer to it
	intention := intentionItem.intention
	return &intention
}

// UpdateIntentions updates the list with new intentions
func (il *IntentionList) UpdateIntentions(intentions []journal.Intention) {
	items := make([]list.Item, len(intentions))
	for i, intention := range intentions {
		items[i] = IntentionItem{intention}
	}
	il.list.SetItems(items)
}
