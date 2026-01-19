package components

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/MikeBiancalana/reckon/internal/logger"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	taskStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39"))

	taskDoneStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Strikethrough(true)

	noteStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	taskListTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("205")).
				Bold(true)

	focusedTaskListTitleStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("11")).
					Bold(true)

	sectionHeaderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("14")).
				Bold(true).
				Border(lipgloss.RoundedBorder()).
				Padding(0, 1)

	sectionHeaderFocusedStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("11")).
					Bold(true).
					Border(lipgloss.RoundedBorder()).
					BorderForeground(lipgloss.Color("11")).
					Padding(0, 1)
)

type TimeGroupedTasks struct {
	Today    []journal.Task
	ThisWeek []journal.Task
	AllTasks []journal.Task
}

func (t TimeGroupedTasks) IsEmpty() bool {
	return len(t.Today) == 0 && len(t.ThisWeek) == 0 && len(t.AllTasks) == 0
}

func (t TimeGroupedTasks) TodayCount() int {
	count := 0
	for _, task := range t.Today {
		if task.Status == journal.TaskOpen {
			count++
		}
	}
	return count
}

func (t TimeGroupedTasks) ThisWeekCount() int {
	count := 0
	for _, task := range t.ThisWeek {
		if task.Status == journal.TaskOpen {
			count++
		}
	}
	return count
}

func (t TimeGroupedTasks) AllTasksCount() int {
	count := 0
	for _, task := range t.AllTasks {
		if task.Status == journal.TaskOpen {
			count++
		}
	}
	return count
}

func GroupTasksByTime(tasks []journal.Task) TimeGroupedTasks {
	now := time.Now()
	today := now.Truncate(24 * time.Hour)
	weekStart := today.AddDate(0, 0, -int(now.Weekday()))
	weekEnd := weekStart.AddDate(0, 0, 7)

	result := TimeGroupedTasks{}

	for _, task := range tasks {
		if task.Status == journal.TaskDone {
			continue
		}

		isToday := false
		isThisWeek := false

		if task.ScheduledDate != nil {
			scheduled, err := time.Parse("2006-01-02", *task.ScheduledDate)
			if err == nil {
				scheduled = scheduled.Truncate(24 * time.Hour)
				if scheduled.Equal(today) {
					isToday = true
				} else if (scheduled.Equal(weekStart) || scheduled.After(weekStart)) && scheduled.Before(weekEnd) {
					isThisWeek = true
				}
			}
		}

		if task.DeadlineDate != nil {
			deadline, err := time.Parse("2006-01-02", *task.DeadlineDate)
			if err == nil {
				deadline = deadline.Truncate(24 * time.Hour)
				if deadline.Before(today) {
					isToday = true
				} else if deadline.Equal(today) {
					isToday = true
				} else if (deadline.Equal(weekStart) || deadline.After(weekStart)) && deadline.Before(weekEnd) {
					isThisWeek = true
				}
			}
		}

		if isToday {
			result.Today = append(result.Today, task)
		} else if isThisWeek {
			result.ThisWeek = append(result.ThisWeek, task)
		} else {
			result.AllTasks = append(result.AllTasks, task)
		}
	}

	return result
}

// TaskToggleMsg is sent when a task's status is toggled
type TaskToggleMsg struct {
	TaskID string
}

// TaskSelectionChangedMsg is sent when the task selection changes
type TaskSelectionChangedMsg struct {
	TaskID string
}

// TaskNoteDeleteMsg is sent when a task note should be deleted
type TaskNoteDeleteMsg struct {
	TaskID string
	NoteID string
}

// TaskItem represents an item in the task list (either a task or a note)
type TaskItem struct {
	task   journal.Task
	isNote bool
	noteID string
	taskID string // parent task ID for notes
}

func (t TaskItem) FilterValue() string {
	if t.isNote {
		return ""
	}
	return t.task.Text
}

// TaskDelegate handles rendering of task items
type TaskDelegate struct {
	collapsedMap map[string]bool // taskID -> isCollapsed
}

func (d TaskDelegate) Height() int                               { return 1 }
func (d TaskDelegate) Spacing() int                              { return 0 }
func (d TaskDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return nil }

func (d TaskDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	item, ok := listItem.(TaskItem)
	if !ok {
		return
	}

	var text string
	var style lipgloss.Style

	if item.isNote {
		// Render note with 2-space indent
		text = fmt.Sprintf("  - %s", findNoteText(item.task.Notes, item.noteID))
		style = noteStyle
	} else {
		// Render task with checkbox
		checkbox := "[ ]"
		style = taskStyle
		if item.task.Status == journal.TaskDone {
			checkbox = "[x]"
			style = taskDoneStyle
		}

		// Add expand/collapse indicator if task has notes
		indicator := ""
		if len(item.task.Notes) > 0 {
			if d.collapsedMap[item.task.ID] {
				indicator = CollapseIndicatorCollapsed
			} else {
				indicator = CollapseIndicatorExpanded
			}
		}

		text = fmt.Sprintf("%s%s %s", indicator, checkbox, item.task.Text)

		if len(item.task.Tags) > 0 {
			tagStr := fmt.Sprintf(" [%s]", strings.Join(item.task.Tags, " "))
			text = text + tagStr
		}
	}

	// Highlight selected item
	if index == m.Index() {
		text = SelectedStyle.Render(CollapseIndicatorCollapsed + text)
	} else {
		text = style.Render(text)
	}

	fmt.Fprintf(w, "%s", text)
}

// findNoteText finds the text of a note by ID
func findNoteText(notes []journal.TaskNote, noteID string) string {
	for _, note := range notes {
		if note.ID == noteID {
			return note.Text
		}
	}
	return ""
}

// TaskList represents the tasks component
type TaskList struct {
	list               list.Model
	collapsedMap       map[string]bool
	tasks              []journal.Task // keep track of original tasks for state management
	focused            bool
	lastSelectedTaskID string // Track previous selection to detect changes
}

// NewTaskList creates a new task list component
func NewTaskList(tasks []journal.Task) *TaskList {
	collapsedMap := make(map[string]bool)
	items := buildTaskItems(tasks, collapsedMap)

	delegate := TaskDelegate{collapsedMap: collapsedMap}
	l := list.New(items, delegate, 0, 0)
	l.Title = "Tasks"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = taskListTitleStyle

	// Determine initial selection
	initialTaskID := ""
	if len(items) > 0 {
		if taskItem, ok := items[0].(TaskItem); ok && !taskItem.isNote {
			initialTaskID = taskItem.task.ID
		}
	}

	return &TaskList{
		list:               l,
		collapsedMap:       collapsedMap,
		tasks:              tasks,
		lastSelectedTaskID: initialTaskID,
	}
}

// buildTaskItems converts tasks into list items, respecting collapsed state
func buildTaskItems(tasks []journal.Task, collapsedMap map[string]bool) []list.Item {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("task_list: panic in buildTaskItems", "error", r, slog.String("stack", fmt.Sprintf("%v", r)))
		}
	}()

	items := make([]list.Item, 0)

	for _, task := range tasks {
		if task.ID == "" {
			logger.Warn("task_list: skipping task with empty ID")
			continue
		}

		items = append(items, TaskItem{
			task:   task,
			isNote: false,
		})

		if !collapsedMap[task.ID] && len(task.Notes) > 0 {
			for _, note := range task.Notes {
				if note.ID == "" {
					logger.Warn("task_list: skipping note with empty ID", "taskID", task.ID)
					continue
				}
				items = append(items, TaskItem{
					task:   task,
					isNote: true,
					noteID: note.ID,
					taskID: task.ID,
				})
			}
		}
	}

	return items
}

// Update handles messages for the task list
func (tl *TaskList) Update(msg tea.Msg) (*TaskList, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if !tl.focused {
			var cmd tea.Cmd
			tl.list, cmd = tl.list.Update(msg)
			return tl, cmd
		}
		switch msg.String() {
		case " ":
			selectedItem := tl.list.SelectedItem()
			if selectedItem == nil {
				logger.Warn("task_list: cannot toggle - no item selected")
				return tl, nil
			}
			taskItem, ok := selectedItem.(TaskItem)
			if !ok {
				logger.Error("task_list: failed to cast selected item to TaskItem")
				return tl, nil
			}
			if taskItem.isNote {
				logger.Warn("task_list: cannot toggle note completion", "noteID", taskItem.noteID)
				return tl, nil
			}
			return tl, func() tea.Msg {
				return TaskToggleMsg{TaskID: taskItem.task.ID}
			}

		case "enter":
			selectedItem := tl.list.SelectedItem()
			if selectedItem == nil {
				logger.Warn("task_list: cannot expand/collapse - no item selected")
				return tl, nil
			}
			taskItem, ok := selectedItem.(TaskItem)
			if !ok {
				logger.Error("task_list: failed to cast selected item to TaskItem")
				return tl, nil
			}
			if taskItem.isNote {
				logger.Warn("task_list: cannot expand/collapse note", "noteID", taskItem.noteID)
				return tl, nil
			}
			if len(taskItem.task.Notes) == 0 {
				logger.Debug("task_list: no notes to expand/collapse", "taskID", taskItem.task.ID)
				return tl, nil
			}

			tl.collapsedMap[taskItem.task.ID] = !tl.collapsedMap[taskItem.task.ID]
			items := buildTaskItems(tl.tasks, tl.collapsedMap)
			tl.list.SetItems(items)

			delegate := TaskDelegate{collapsedMap: tl.collapsedMap}
			tl.list.SetDelegate(delegate)

			currentIndex := tl.list.Index()
			if currentIndex < len(items) {
				currentItem, ok := items[currentIndex].(TaskItem)
				if ok && currentItem.isNote && tl.collapsedMap[taskItem.task.ID] {
					for i, item := range items {
						if ti, ok := item.(TaskItem); ok && !ti.isNote && ti.task.ID == taskItem.task.ID {
							tl.list.Select(i)
							break
						}
					}
				}
			}
			return tl, nil

		case "d":
			selectedItem := tl.list.SelectedItem()
			if selectedItem == nil {
				return tl, nil
			}
			taskItem, ok := selectedItem.(TaskItem)
			if ok {
				if taskItem.isNote {
					return tl, func() tea.Msg {
						return TaskNoteDeleteMsg{
							TaskID: taskItem.taskID,
							NoteID: taskItem.noteID,
						}
					}
				}
			}
			return tl, nil
		}
	}

	var cmd tea.Cmd
	tl.list, cmd = tl.list.Update(msg)

	// Only send selection changed message if selection actually changed
	selectedItem := tl.list.SelectedItem()
	var currentTaskID string
	if selectedItem != nil {
		taskItem, ok := selectedItem.(TaskItem)
		if ok && !taskItem.isNote {
			currentTaskID = taskItem.task.ID
		}
	}

	// Compare with previous selection
	if currentTaskID != "" && currentTaskID != tl.lastSelectedTaskID {
		tl.lastSelectedTaskID = currentTaskID
		cmd = tea.Batch(cmd, func() tea.Msg {
			return TaskSelectionChangedMsg{TaskID: currentTaskID}
		})
	}

	return tl, cmd
}

// View renders the task list
func (tl *TaskList) View() string {
	if len(tl.list.Items()) == 0 {
		return "Tasks\n\nNo tasks yet - press t to add one"
	}
	return tl.list.View()
}

// SetSize sets the size of the list
func (tl *TaskList) SetSize(width, height int) {
	tl.list.SetSize(width, height)
}

// SetFocused sets whether this component is focused
func (tl *TaskList) SetFocused(focused bool) {
	tl.focused = focused
	if focused {
		tl.list.Styles.Title = focusedTaskListTitleStyle
	} else {
		tl.list.Styles.Title = taskListTitleStyle
	}
}

// SelectedTask returns the currently selected task
func (tl *TaskList) SelectedTask() *journal.Task {
	item := tl.list.SelectedItem()
	if item == nil {
		return nil
	}
	taskItem, ok := item.(TaskItem)
	if !ok {
		return nil
	}

	// Find and return the task from our tasks slice
	for i := range tl.tasks {
		if tl.tasks[i].ID == taskItem.task.ID {
			return &tl.tasks[i]
		}
	}
	return nil
}

// IsSelectedItemNote returns true if the currently selected item is a note
func (tl *TaskList) IsSelectedItemNote() bool {
	item := tl.list.SelectedItem()
	if item == nil {
		return false
	}
	taskItem, ok := item.(TaskItem)
	if !ok {
		return false
	}
	return taskItem.isNote
}

// UpdateTasks updates the list with new tasks
func (tl *TaskList) UpdateTasks(tasks []journal.Task) {
	// Preserve cursor position by finding the currently selected task ID
	selectedItem := tl.list.SelectedItem()
	var selectedTaskID string
	if selectedItem != nil {
		if taskItem, ok := selectedItem.(TaskItem); ok {
			selectedTaskID = taskItem.task.ID
		}
	}

	tl.tasks = tasks
	items := buildTaskItems(tasks, tl.collapsedMap)
	tl.list.SetItems(items)

	// Restore cursor to the previously selected task
	if selectedTaskID != "" {
		for i, item := range tl.list.Items() {
			if taskItem, ok := item.(TaskItem); ok && !taskItem.isNote && taskItem.task.ID == selectedTaskID {
				tl.list.Select(i)
				break
			}
		}
	}

	// Update delegate with current collapsed map
	delegate := TaskDelegate{collapsedMap: tl.collapsedMap}
	tl.list.SetDelegate(delegate)

	// Update lastSelectedTaskID to match restored cursor position
	selectedItem = tl.list.SelectedItem()
	if selectedItem != nil {
		if taskItem, ok := selectedItem.(TaskItem); ok && !taskItem.isNote {
			tl.lastSelectedTaskID = taskItem.task.ID
		}
	} else {
		tl.lastSelectedTaskID = ""
	}
}

// GetTasks returns the current tasks
func (tl *TaskList) GetTasks() []journal.Task {
	return tl.tasks
}

type TimeGroupedTaskList struct {
	taskList     *TaskList
	groupedTasks TimeGroupedTasks
	focused      bool
	sectionIndex int
	list         list.Model
}

type SectionType int

const (
	SectionToday SectionType = iota
	SectionThisWeek
	SectionAllTasks
)

func NewTimeGroupedTaskList(tasks []journal.Task) *TimeGroupedTaskList {
	grouped := GroupTasksByTime(tasks)

	tgl := &TimeGroupedTaskList{
		groupedTasks: grouped,
		sectionIndex: 0,
	}

	tgl.updateListForSection()

	return tgl
}

func (tgl *TimeGroupedTaskList) updateListForSection() {
	var tasks []journal.Task
	var sectionTitle string

	switch tgl.sectionIndex {
	case 0:
		tasks = tgl.groupedTasks.Today
		sectionTitle = "TODAY"
	case 1:
		tasks = tgl.groupedTasks.ThisWeek
		sectionTitle = "THIS WEEK"
	case 2:
		tasks = tgl.groupedTasks.AllTasks
		sectionTitle = "ALL TASKS"
	}

	collapsedMap := make(map[string]bool)
	items := buildTaskItems(tasks, collapsedMap)

	delegate := TaskDelegate{collapsedMap: collapsedMap}
	l := list.New(items, delegate, 0, 0)
	l.Title = sectionTitle
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = taskListTitleStyle

	tgl.list = l
}

func (tgl *TimeGroupedTaskList) View() string {
	if tgl.groupedTasks.IsEmpty() {
		return "Tasks\n\nNo tasks yet - press t to add one"
	}

	var sb strings.Builder

	todayCount := tgl.groupedTasks.TodayCount()
	thisWeekCount := tgl.groupedTasks.ThisWeekCount()
	allTasksCount := tgl.groupedTasks.AllTasksCount()

	todayTitle := fmt.Sprintf(" TODAY (%d) ", todayCount)
	weekTitle := fmt.Sprintf(" THIS WEEK (%d) ", thisWeekCount)
	allTitle := fmt.Sprintf(" ALL TASKS (%d) ", allTasksCount)

	if tgl.sectionIndex == 0 {
		sb.WriteString(sectionHeaderFocusedStyle.Render(todayTitle))
	} else {
		sb.WriteString(sectionHeaderStyle.Render(todayTitle))
	}
	sb.WriteString("\n")

	if len(tgl.groupedTasks.Today) > 0 || tgl.sectionIndex == 0 {
		if tgl.sectionIndex == 0 {
			sb.WriteString(tgl.list.View())
		} else {
			for _, task := range tgl.groupedTasks.Today {
				if task.Status == journal.TaskOpen {
					sb.WriteString(fmt.Sprintf("[ ] %s\n", task.Text))
				}
			}
			if len(tgl.groupedTasks.Today) == 0 {
				sb.WriteString("  No tasks\n")
			}
		}
	}

	sb.WriteString("\n")

	if tgl.sectionIndex == 1 {
		sb.WriteString(sectionHeaderFocusedStyle.Render(weekTitle))
	} else {
		sb.WriteString(sectionHeaderStyle.Render(weekTitle))
	}
	sb.WriteString("\n")

	if len(tgl.groupedTasks.ThisWeek) > 0 || tgl.sectionIndex == 1 {
		if tgl.sectionIndex == 1 {
			sb.WriteString(tgl.list.View())
		} else {
			for _, task := range tgl.groupedTasks.ThisWeek {
				if task.Status == journal.TaskOpen {
					sb.WriteString(fmt.Sprintf("[ ] %s\n", task.Text))
				}
			}
			if len(tgl.groupedTasks.ThisWeek) == 0 {
				sb.WriteString("  No tasks\n")
			}
		}
	}

	sb.WriteString("\n")

	if tgl.sectionIndex == 2 {
		sb.WriteString(sectionHeaderFocusedStyle.Render(allTitle))
	} else {
		sb.WriteString(sectionHeaderStyle.Render(allTitle))
	}
	sb.WriteString("\n")

	if tgl.sectionIndex == 2 {
		sb.WriteString(tgl.list.View())
	} else {
		for _, task := range tgl.groupedTasks.AllTasks {
			if task.Status == journal.TaskOpen {
				sb.WriteString(fmt.Sprintf("[ ] %s\n", task.Text))
			}
		}
		if len(tgl.groupedTasks.AllTasks) == 0 {
			sb.WriteString("  No tasks\n")
		}
	}

	return sb.String()
}

func (tgl *TimeGroupedTaskList) SetSize(width, height int) {
	tgl.list.SetSize(width, height)
}

func (tgl *TimeGroupedTaskList) SetFocused(focused bool) {
	tgl.focused = focused
	if focused {
		tgl.list.Styles.Title = focusedTaskListTitleStyle
	} else {
		tgl.list.Styles.Title = taskListTitleStyle
	}
}

func (tgl *TimeGroupedTaskList) Update(msg tea.Msg) (*TimeGroupedTaskList, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if !tgl.focused {
			var cmd tea.Cmd
			tgl.list, cmd = tgl.list.Update(msg)
			return tgl, cmd
		}

		switch msg.String() {
		case "j", "down":
			currentListLen := tgl.currentSectionLength()
			if tgl.list.Index() >= currentListLen-1 {
				if tgl.sectionIndex < 2 {
					tgl.sectionIndex++
					tgl.updateListForSection()
					return tgl, nil
				}
			}
			var cmd tea.Cmd
			tgl.list, cmd = tgl.list.Update(msg)
			return tgl, cmd

		case "k", "up":
			if tgl.list.Index() <= 0 {
				if tgl.sectionIndex > 0 {
					tgl.sectionIndex--
					tgl.updateListForSection()
					tgl.list.Select(tgl.currentSectionLength() - 1)
					return tgl, nil
				}
			}
			var cmd tea.Cmd
			tgl.list, cmd = tgl.list.Update(msg)
			return tgl, cmd

		case " ":
			selectedItem := tgl.list.SelectedItem()
			if selectedItem == nil {
				return tgl, nil
			}
			taskItem, ok := selectedItem.(TaskItem)
			if !ok {
				return tgl, nil
			}
			if taskItem.isNote {
				return tgl, nil
			}
			return tgl, func() tea.Msg {
				return TaskToggleMsg{TaskID: taskItem.task.ID}
			}

		case "enter":
			selectedItem := tgl.list.SelectedItem()
			if selectedItem == nil {
				return tgl, nil
			}
			taskItem, ok := selectedItem.(TaskItem)
			if !ok {
				return tgl, nil
			}
			if taskItem.isNote || len(taskItem.task.Notes) == 0 {
				return tgl, nil
			}

			taskList := NewTaskList(tgl.getAllTasks())
			taskList.SetFocused(true)
			taskList.collapsedMap[taskItem.task.ID] = !taskList.collapsedMap[taskItem.task.ID]
			items := buildTaskItems(taskList.tasks, taskList.collapsedMap)
			taskList.list.SetItems(items)

			for i, item := range items {
				if ti, ok := item.(TaskItem); ok && !ti.isNote && ti.task.ID == taskItem.task.ID {
					taskList.list.Select(i)
					break
				}
			}

			delegate := TaskDelegate{collapsedMap: taskList.collapsedMap}
			taskList.list.SetDelegate(delegate)

			tgl.taskList = taskList
			return tgl, nil
		}
	}

	var cmd tea.Cmd
	tgl.list, cmd = tgl.list.Update(msg)
	return tgl, cmd
}

func (tgl *TimeGroupedTaskList) currentSectionLength() int {
	switch tgl.sectionIndex {
	case 0:
		return len(tgl.groupedTasks.Today)
	case 1:
		return len(tgl.groupedTasks.ThisWeek)
	case 2:
		return len(tgl.groupedTasks.AllTasks)
	}
	return 0
}

func (tgl *TimeGroupedTaskList) getAllTasks() []journal.Task {
	var all []journal.Task
	all = append(all, tgl.groupedTasks.Today...)
	all = append(all, tgl.groupedTasks.ThisWeek...)
	all = append(all, tgl.groupedTasks.AllTasks...)
	return all
}

func (tgl *TimeGroupedTaskList) SelectedTask() *journal.Task {
	item := tgl.list.SelectedItem()
	if item == nil {
		return nil
	}
	taskItem, ok := item.(TaskItem)
	if !ok {
		return nil
	}

	allTasks := tgl.getAllTasks()
	for i := range allTasks {
		if allTasks[i].ID == taskItem.task.ID {
			return &allTasks[i]
		}
	}
	return nil
}

func (tgl *TimeGroupedTaskList) IsSelectedItemNote() bool {
	item := tgl.list.SelectedItem()
	if item == nil {
		return false
	}
	taskItem, ok := item.(TaskItem)
	if !ok {
		return false
	}
	return taskItem.isNote
}

func (tgl *TimeGroupedTaskList) UpdateTasks(tasks []journal.Task) {
	tgl.groupedTasks = GroupTasksByTime(tasks)
	if tgl.sectionIndex >= 3 {
		tgl.sectionIndex = 0
	}
	tgl.updateListForSection()
}

func (tgl *TimeGroupedTaskList) GetTasks() []journal.Task {
	return tgl.getAllTasks()
}
