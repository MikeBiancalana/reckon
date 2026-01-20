package components

import (
	"github.com/charmbracelet/lipgloss"
)

const (
	CollapseIndicatorCollapsed = "▶ "
	CollapseIndicatorExpanded  = "▼ "
)

var SelectedStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("11")).
	Bold(true)
