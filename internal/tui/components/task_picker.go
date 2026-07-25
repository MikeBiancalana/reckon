package components

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
)

var (
	taskPickerBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("39")).
				Padding(1, 2)

	taskPickerTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("39"))

	taskPickerItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))

	taskPickerSelectedItemStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("39")).
					Bold(true)

	taskPickerDescStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245"))

	taskPickerHelpStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240"))
)

// TaskPickerSelectMsg is sent when a task is selected
type TaskPickerSelectMsg struct {
	TaskID string
}

// TaskPickerCancelMsg is sent when the task picker is cancelled
type TaskPickerCancelMsg struct{}

// taskPickerItem implements list.Item for the task picker
type taskPickerItem struct {
	row IndexRow
}

func (i taskPickerItem) FilterValue() string {
	return i.row.Title
}

func (i taskPickerItem) Title() string {
	return i.row.Title
}

func (i taskPickerItem) Description() string {
	var parts []string

	// Add schedule if present
	if v := i.row.Props["scheduled"]; v != "" {
		parts = append(parts, "Scheduled: "+v)
	}

	// Add deadline if present
	if v := i.row.Props["deadline"]; v != "" {
		parts = append(parts, "Deadline: "+v)
	}

	return strings.Join(parts, " | ")
}

// taskPickerDelegate handles rendering of task picker items
type taskPickerDelegate struct{}

func (d taskPickerDelegate) Height() int  { return 2 }
func (d taskPickerDelegate) Spacing() int { return 1 }
func (d taskPickerDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return nil
}

func (d taskPickerDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	item, ok := listItem.(taskPickerItem)
	if !ok {
		return
	}

	title := item.Title()
	desc := item.Description()

	isSelected := index == m.Index()

	// Render title
	titleStyle := taskPickerItemStyle
	if isSelected {
		titleStyle = taskPickerSelectedItemStyle
		title = "> " + title
	} else {
		title = "  " + title
	}

	fmt.Fprint(w, titleStyle.Render(title))
	if desc != "" {
		fmt.Fprint(w, "\n  "+taskPickerDescStyle.Render(desc))
	}
}

// TaskPicker is a reusable fuzzy finder for selecting tasks
type TaskPicker struct {
	list         list.Model
	title        string
	visible      bool
	tasks        []IndexRow
	selectedTask *IndexRow
	canceled     bool
	width        int
}

// fuzzyFilter implements fuzzy matching for task picker items
func fuzzyFilter(term string, targets []string) []list.Rank {
	if term == "" {
		return nil
	}

	matches := fuzzy.Find(term, targets)
	ranks := make([]list.Rank, len(matches))

	for i, match := range matches {
		ranks[i] = list.Rank{
			Index:          match.Index,
			MatchedIndexes: match.MatchedIndexes,
		}
	}

	return ranks
}

// NewTaskPicker creates a new task picker component
func NewTaskPicker(title string) *TaskPicker {
	delegate := taskPickerDelegate{}
	l := list.New([]list.Item{}, delegate, 0, 0)
	l.Title = title
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.FilterInput.Prompt = "Filter: "
	l.Styles.Title = taskPickerTitleStyle
	l.SetShowHelp(false)

	// Configure fuzzy matching filter
	l.Filter = fuzzyFilter

	return &TaskPicker{
		list:    l,
		title:   title,
		visible: false,
		width:   80,
	}
}

// Show displays the task picker with the given rows
func (tp *TaskPicker) Show(rows []IndexRow) tea.Cmd {
	tp.visible = true
	tp.tasks = rows
	tp.selectedTask = nil
	tp.canceled = false

	// Convert rows to list items
	items := make([]list.Item, len(rows))
	for i, row := range rows {
		items[i] = taskPickerItem{row: row}
	}

	tp.list.SetItems(items)
	tp.list.ResetFilter()
	return nil
}

// Hide hides the task picker. It does not touch selectedTask -- Show() is
// the reset point for selection state.
func (tp *TaskPicker) Hide() {
	tp.visible = false
}

// IsVisible returns whether the task picker is visible
func (tp *TaskPicker) IsVisible() bool {
	return tp.visible
}

// GetSelectedTaskID returns the ID of the selected task, or empty string if none
func (tp *TaskPicker) GetSelectedTaskID() string {
	if tp.selectedTask == nil {
		return ""
	}
	return tp.selectedTask.ID
}

// SetWidth sets the width of the task picker
func (tp *TaskPicker) SetWidth(width int) {
	tp.width = width
	listWidth := width - 10
	listHeight := 15
	if listWidth < 40 {
		listWidth = 40
	}
	tp.list.SetSize(listWidth, listHeight)
}

// Init satisfies Prompt[string]. Priming (Show) already happened before a
// TaskPicker is handed to RunPrompt/Wizard, so there is nothing to do here.
func (tp *TaskPicker) Init() tea.Cmd { return nil }

// Result returns the selected task's ID. Only meaningful once Done()
// reports finished.
func (tp *TaskPicker) Result() string { return tp.GetSelectedTaskID() }

// Done reports whether Update has reached a terminal state.
func (tp *TaskPicker) Done() (finished, canceled bool) {
	return tp.selectedTask != nil, tp.canceled
}

// Update handles Bubble Tea messages
func (tp *TaskPicker) Update(msg tea.Msg) (Prompt[string], tea.Cmd) {
	if !tp.visible {
		return tp, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc:
			tp.Hide()
			tp.canceled = true
			return tp, func() tea.Msg {
				return TaskPickerCancelMsg{}
			}

		case tea.KeyEnter:
			// Get selected item
			selectedItem := tp.list.SelectedItem()
			if selectedItem == nil {
				return tp, nil
			}

			item, ok := selectedItem.(taskPickerItem)
			if !ok {
				return tp, nil
			}

			tp.selectedTask = &item.row
			tp.Hide()

			return tp, func() tea.Msg {
				return TaskPickerSelectMsg{
					TaskID: item.row.ID,
				}
			}
		}
	}

	// Update the list
	var cmd tea.Cmd
	tp.list, cmd = tp.list.Update(msg)

	return tp, cmd
}

// View renders the task picker
func (tp *TaskPicker) View() string {
	if !tp.visible {
		return ""
	}

	var content strings.Builder

	// List view
	content.WriteString(tp.list.View())
	content.WriteString("\n\n")

	// Help text
	helpText := "ENTER: select  ESC: cancel  /: filter"
	content.WriteString(taskPickerHelpStyle.Render(helpText))

	// Wrap in box
	return taskPickerBoxStyle.Render(content.String())
}
