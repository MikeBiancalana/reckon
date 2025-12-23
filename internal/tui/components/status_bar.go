package components

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	statusBarStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Background(lipgloss.Color("236")).
		Padding(0, 1)

	dateStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("255")).
		Background(lipgloss.Color("236")).
		Bold(true)
)

// StatusBar represents the status bar component
type StatusBar struct {
	width       int
	currentDate string
}

// NewStatusBar creates a new status bar
func NewStatusBar() *StatusBar {
	return &StatusBar{}
}

// SetWidth sets the width of the status bar
func (sb *StatusBar) SetWidth(width int) {
	sb.width = width
}

// SetDate sets the current date being viewed
func (sb *StatusBar) SetDate(date string) {
	sb.currentDate = date
}

// View renders the status bar
func (sb *StatusBar) View() string {
	// Format the date display
	dateDisplay := sb.formatDate()
	hints := "q:quit tab:switch i:intention w:win L:log h/l:nav t:today ?:help enter:toggle"

	// Calculate available space
	dateLen := lipgloss.Width(dateDisplay)
	hintsLen := len(hints)
	totalLen := dateLen + hintsLen + 3 // +3 for spacing

	// If we have enough space, show date and hints on same line
	if totalLen <= sb.width {
		// Use padding to space them out
		spacer := strings.Repeat(" ", sb.width-dateLen-hintsLen-2) // -2 for padding
		content := dateDisplay + spacer + hints
		return statusBarStyle.Width(sb.width).Render(content)
	}

	// Not enough space, truncate hints
	availableForHints := sb.width - dateLen - 5
	if availableForHints > 0 && hintsLen > availableForHints {
		hints = hints[:availableForHints] + "..."
	}

	spacer := strings.Repeat(" ", sb.width-dateLen-len(hints)-2)
	content := dateDisplay + spacer + hints
	return statusBarStyle.Width(sb.width).Render(content)
}

// formatDate formats the current date for display
func (sb *StatusBar) formatDate() string {
	if sb.currentDate == "" {
		return ""
	}

	// Check if it's today
	today := time.Now().Format("2006-01-02")
	if sb.currentDate == today {
		return dateStyle.Render("Today")
	}

	// Parse and format the date
	date, err := time.Parse("2006-01-02", sb.currentDate)
	if err != nil {
		return dateStyle.Render(sb.currentDate)
	}

	// Show in a friendly format: "Mon, Jan 2, 2006"
	formatted := date.Format("Mon, Jan 2, 2006")
	return dateStyle.Render(formatted)
}
