package components

import (
	"fmt"
	"strings"

	"github.com/MikeBiancalana/reckon/internal/task"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	pickerTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("12"))

	pickerSelectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("15")).
				Background(lipgloss.Color("4")).
				Bold(true)

	pickerNormalStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("7"))

	pickerDimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))
)

// TaskPicker is a fuzzy-searchable task picker
type TaskPicker struct {
	tasks         []task.Task
	filteredTasks []task.Task
	searchInput   textinput.Model
	selectedIndex int
	width         int
	height        int
}

// NewTaskPicker creates a new task picker
func NewTaskPicker(tasks []task.Task) *TaskPicker {
	ti := textinput.New()
	ti.Placeholder = "Search tasks..."
	ti.Focus()
	ti.CharLimit = 100

	picker := &TaskPicker{
		tasks:         tasks,
		filteredTasks: tasks,
		searchInput:   ti,
		selectedIndex: 0,
	}

	return picker
}

// Update handles messages
func (p *TaskPicker) Update(msg tea.Msg) (*TaskPicker, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if p.selectedIndex > 0 {
				p.selectedIndex--
			}
			return p, nil

		case "down", "j":
			if p.selectedIndex < len(p.filteredTasks)-1 {
				p.selectedIndex++
			}
			return p, nil

		default:
			// Update search input
			p.searchInput, cmd = p.searchInput.Update(msg)
			p.filterTasks()

			// Reset selection if needed
			if p.selectedIndex >= len(p.filteredTasks) {
				p.selectedIndex = 0
			}
			return p, cmd
		}

	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height
	}

	return p, nil
}

// filterTasks filters tasks based on search query
func (p *TaskPicker) filterTasks() {
	query := strings.ToLower(p.searchInput.Value())

	if query == "" {
		p.filteredTasks = p.tasks
		return
	}

	filtered := make([]task.Task, 0)
	for _, t := range p.tasks {
		// Search in title
		if strings.Contains(strings.ToLower(t.Title), query) {
			filtered = append(filtered, t)
			continue
		}

		// Search in tags
		for _, tag := range t.Tags {
			if strings.Contains(strings.ToLower(tag), query) {
				filtered = append(filtered, t)
				break
			}
		}
	}

	p.filteredTasks = filtered
}

// View renders the task picker
func (p *TaskPicker) View() string {
	var sb strings.Builder

	// Title
	sb.WriteString(pickerTitleStyle.Render("Select Task"))
	sb.WriteString("\n\n")

	// Search input
	sb.WriteString(p.searchInput.View())
	sb.WriteString("\n\n")

	// Task list
	if len(p.filteredTasks) == 0 {
		sb.WriteString(pickerDimStyle.Render("No tasks found"))
	} else {
		// Show up to 10 tasks
		maxDisplay := 10
		if len(p.filteredTasks) < maxDisplay {
			maxDisplay = len(p.filteredTasks)
		}

		for i := 0; i < maxDisplay; i++ {
			t := p.filteredTasks[i]

			var line string
			if i == p.selectedIndex {
				line = pickerSelectedStyle.Render(fmt.Sprintf("▶ %s", t.Title))
			} else {
				line = pickerNormalStyle.Render(fmt.Sprintf("  %s", t.Title))
			}

			// Add tags
			if len(t.Tags) > 0 {
				tagsStr := pickerDimStyle.Render(fmt.Sprintf(" [%s]", strings.Join(t.Tags, ", ")))
				line += tagsStr
			}

			sb.WriteString(line)
			sb.WriteString("\n")
		}

		// Show count if more tasks available
		if len(p.filteredTasks) > maxDisplay {
			sb.WriteString(pickerDimStyle.Render(fmt.Sprintf("\n... and %d more", len(p.filteredTasks)-maxDisplay)))
		}
	}

	sb.WriteString("\n\n")
	sb.WriteString(pickerDimStyle.Render("↑/↓: navigate • enter: select • esc: cancel"))

	return sb.String()
}

// SelectedTask returns the currently selected task
func (p *TaskPicker) SelectedTask() *task.Task {
	if p.selectedIndex < 0 || p.selectedIndex >= len(p.filteredTasks) {
		return nil
	}
	return &p.filteredTasks[p.selectedIndex]
}

// SetSize sets the picker dimensions
func (p *TaskPicker) SetSize(width, height int) {
	p.width = width
	p.height = height
	p.searchInput.Width = width - 4
}
