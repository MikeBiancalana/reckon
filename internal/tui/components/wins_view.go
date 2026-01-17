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
	winStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("46")).
			Bold(true)

	focusedWinStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")).
			Bold(true)
)

// WinItem represents a win in the list
type WinItem struct {
	win journal.Win
}

func (w WinItem) FilterValue() string { return w.win.Text }

// WinDelegate handles rendering of win items
type WinDelegate struct{}

func (d WinDelegate) Height() int                               { return 1 }
func (d WinDelegate) Spacing() int                              { return 0 }
func (d WinDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return nil }

func (d WinDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	wi, ok := listItem.(WinItem)
	if !ok {
		return
	}

	text := fmt.Sprintf("🏆 %s", wi.win.Text)

	if index == m.Index() {
		text = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true).Render("▶ " + text)
	} else {
		text = winStyle.Render(text)
	}

	fmt.Fprintf(w, "%s", text)
}

// WinsView represents the wins component
type WinsView struct {
	list    list.Model
	focused bool
}

func NewWinsView(wins []journal.Win) *WinsView {
	items := make([]list.Item, len(wins))
	for i, win := range wins {
		items[i] = WinItem{win}
	}

	l := list.New(items, WinDelegate{}, 0, 0)
	l.Title = "Wins"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = winStyle

	return &WinsView{list: l}
}

// Update handles messages for the wins view
func (wv *WinsView) Update(msg tea.Msg) (*WinsView, tea.Cmd) {
	var cmd tea.Cmd
	wv.list, cmd = wv.list.Update(msg)
	return wv, cmd
}

// View renders the wins view
func (wv *WinsView) View() string {
	if len(wv.list.Items()) == 0 {
		return "Wins\n\nNo wins yet - press w to add one"
	}
	return wv.list.View()
}

// SetSize sets the size of the list
func (wv *WinsView) SetSize(width, height int) {
	wv.list.SetSize(width, height)
}

// SetFocused sets whether this component is focused
func (wv *WinsView) SetFocused(focused bool) {
	wv.focused = focused
	if focused {
		wv.list.Styles.Title = focusedWinStyle
	} else {
		wv.list.Styles.Title = winStyle
	}
}

// UpdateWins updates the list with new wins
func (wv *WinsView) UpdateWins(wins []journal.Win) {
	// Preserve cursor position by finding the currently selected win ID
	selectedItem := wv.list.SelectedItem()
	var selectedWinID string
	if selectedItem != nil {
		if winItem, ok := selectedItem.(WinItem); ok {
			selectedWinID = winItem.win.ID
		}
	}

	items := make([]list.Item, len(wins))
	for i, win := range wins {
		items[i] = WinItem{win}
	}
	wv.list.SetItems(items)

	// Restore cursor to the previously selected win
	if selectedWinID != "" {
		for i, item := range items {
			if winItem, ok := item.(WinItem); ok && winItem.win.ID == selectedWinID {
				wv.list.Select(i)
				break
			}
		}
	}
}

// SelectedWin returns the currently selected win
func (wv *WinsView) SelectedWin() *journal.Win {
	item := wv.list.SelectedItem()
	if item == nil {
		return nil
	}
	winItem, ok := item.(WinItem)
	if !ok {
		return nil
	}
	// Create a copy and return pointer to it
	win := winItem.win
	return &win
}
