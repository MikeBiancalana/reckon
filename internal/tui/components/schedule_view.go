package components

import (
	"sort"
	"strings"

	"github.com/MikeBiancalana/reckon/internal/journal"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	scheduleTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("39")).
				Bold(true)

	itemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	emptyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Italic(true)
)

// ScheduleView is a Bubble Tea component for displaying schedule items
type ScheduleView struct {
	items  []journal.ScheduleItem
	width  int
	height int
}

// NewScheduleView creates a new ScheduleView component
func NewScheduleView(items []journal.ScheduleItem) *ScheduleView {
	return &ScheduleView{
		items:  items,
		width:  20,
		height: 10,
	}
}

// Update handles Bubble Tea messages
func (sv *ScheduleView) Update(msg tea.Msg) (*ScheduleView, tea.Cmd) {
	// ScheduleView is read-only initially, no message handling needed
	return sv, nil
}

// View renders the schedule view
func (sv *ScheduleView) View() string {
	if sv.width <= 0 || sv.height <= 0 {
		return ""
	}

	var sb strings.Builder

	// Title
	title := scheduleTitleStyle.Render("Schedule")
	sb.WriteString(title)
	sb.WriteString("\n\n")

	if len(sv.items) == 0 {
		emptyMsg := emptyStyle.Render("No schedule items")
		sb.WriteString(emptyMsg)
		return sb.String()
	}

	// Sort items by time (items without time go to end)
	sortedItems := sv.getSortedItems()

	// Render each item
	for _, item := range sortedItems {
		line := sv.renderItem(item)
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	return sb.String()
}

// SetSize updates the dimensions of the schedule view
func (sv *ScheduleView) SetSize(width, height int) {
	sv.width = width
	sv.height = height
}

// UpdateSchedule updates the schedule items
func (sv *ScheduleView) UpdateSchedule(items []journal.ScheduleItem) {
	sv.items = items
}

// getSortedItems returns items sorted by time, with untimed items at the end
func (sv *ScheduleView) getSortedItems() []journal.ScheduleItem {
	// Create a copy to avoid modifying original
	sorted := make([]journal.ScheduleItem, len(sv.items))
	copy(sorted, sv.items)

	// Sort by time (zero time items go to end)
	sort.Slice(sorted, func(i, j int) bool {
		iHasTime := !sorted[i].Time.IsZero()
		jHasTime := !sorted[j].Time.IsZero()

		if iHasTime && jHasTime {
			return sorted[i].Time.Before(sorted[j].Time)
		}
		if iHasTime && !jHasTime {
			return true // timed items come before untimed
		}
		if !iHasTime && jHasTime {
			return false // untimed items come after timed
		}
		// Both untimed - maintain original order (position)
		return sorted[i].Position < sorted[j].Position
	})

	return sorted
}

// renderItem renders a single schedule item
func (sv *ScheduleView) renderItem(item journal.ScheduleItem) string {
	if sv.width <= 2 {
		return "" // No space to render
	}

	var line string
	maxContentWidth := sv.width - 2 // Account for "- " prefix and margin

	if !item.Time.IsZero() {
		// Format with time: "- HH:MM Content"
		timeStr := item.Time.Format("15:04")
		prefix := timeStr + " "
		if len(prefix) > maxContentWidth {
			// If time alone is too long, truncate time
			prefix = prefix[:maxContentWidth-3] + "..."
			line = "- " + prefix
		} else {
			content := item.Content
			availableWidth := maxContentWidth - len(prefix)
			if availableWidth > 0 && len(content) > availableWidth {
				content = content[:availableWidth-3] + "..."
			}
			line = "- " + prefix + content
		}
	} else {
		// Format without time: "- Content"
		content := item.Content
		if len(content) > maxContentWidth {
			content = content[:maxContentWidth-3] + "..."
		}
		line = "- " + content
	}

	return itemStyle.Render(line)
}
