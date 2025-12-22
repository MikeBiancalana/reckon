package components

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	statusBarStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Background(lipgloss.Color("236")).
		Padding(0, 1)
)

// StatusBar represents the status bar component
type StatusBar struct {
	width int
}

// NewStatusBar creates a new status bar
func NewStatusBar() *StatusBar {
	return &StatusBar{}
}

// SetWidth sets the width of the status bar
func (sb *StatusBar) SetWidth(width int) {
	sb.width = width
}

// View renders the status bar
func (sb *StatusBar) View() string {
	hints := "q:quit tab:switch i:intention w:win L:log h/l:nav t:today ?:help enter:toggle"

	// Truncate if too long
	if len(hints) > sb.width-2 {
		hints = hints[:sb.width-5] + "..."
	}

	return statusBarStyle.Width(sb.width).Render(hints)
}
