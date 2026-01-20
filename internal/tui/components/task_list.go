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
)

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
		text = SelectedStyle.Render(text)
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

type TimeGroupedTasks struct {
	Today    []journal.Task
	ThisWeek []journal.Task
	AllTasks []journal.Task
}

func parseDate(dateStr *string) (time.Time, bool) {
	if dateStr == nil || *dateStr == "" {
		return time.Time{}, false
	}
	t, err := time.Parse("2006-01-02", *dateStr)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func GroupTasksByTime(tasks []journal.Task) TimeGroupedTasks {
	today := time.Now().Truncate(24 * time.Hour)
	now := time.Now()
	weekday := now.Weekday()
	if weekday == time.Sunday {
		weekday = 7
	}
	weekStart := today.AddDate(0, 0, -int(weekday-time.Monday))
	weekEnd := weekStart.AddDate(0, 0, 7)

	var grouped TimeGroupedTasks
	for _, task := range tasks {
		if task.Status == journal.TaskDone {
			continue
		}
		isToday := false
		isThisWeek := false

		if scheduledDate, ok := parseDate(task.ScheduledDate); ok {
			if scheduledDate.Equal(today) || scheduledDate.Before(today) {
				isToday = true
			} else if (scheduledDate.After(weekStart) || scheduledDate.Equal(weekStart)) && scheduledDate.Before(weekEnd) {
				isThisWeek = true
			}
		}
		if deadlineDate, ok := parseDate(task.DeadlineDate); ok {
			if deadlineDate.Before(today) || deadlineDate.Equal(today) {
				isToday = true
			} else if (deadlineDate.After(weekStart) || deadlineDate.Equal(weekStart)) && deadlineDate.Before(weekEnd) {
				isThisWeek = true
			}
		}

		if isToday {
			grouped.Today = append(grouped.Today, task)
		} else if isThisWeek {
			grouped.ThisWeek = append(grouped.ThisWeek, task)
		} else {
			grouped.AllTasks = append(grouped.AllTasks, task)
		}
	}
	return grouped
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

type TimeGroupedTaskList struct {
	list         list.Model
	groupedTasks TimeGroupedTasks
	sectionIndex int
	collapsedMap map[string]bool
	focused      bool
	delegate     TaskDelegate
}

func NewTimeGroupedTaskList(tasks []journal.Task) *TimeGroupedTaskList {
	groupedTasks := GroupTasksByTime(tasks)
	collapsedMap := make(map[string]bool)
	tgl := &TimeGroupedTaskList{
		groupedTasks: groupedTasks,
		sectionIndex: 0,
		collapsedMap: collapsedMap,
	}
	tgl.updateListForSection()
	return tgl
}

func (tgl *TimeGroupedTaskList) updateListForSection() {
	var items []list.Item
	switch tgl.sectionIndex {
	case 0:
		items = buildTaskItems(tgl.groupedTasks.Today, tgl.collapsedMap)
	case 1:
		items = buildTaskItems(tgl.groupedTasks.ThisWeek, tgl.collapsedMap)
	case 2:
		items = buildTaskItems(tgl.groupedTasks.AllTasks, tgl.collapsedMap)
	}
	tgl.delegate = TaskDelegate{collapsedMap: tgl.collapsedMap}
	l := list.New(items, tgl.delegate, 0, 0)
	l.Title = "Tasks"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = taskListTitleStyle
	tgl.list = l
}

func (tgl *TimeGroupedTaskList) currentSectionTasks() []journal.Task {
	switch tgl.sectionIndex {
	case 0:
		return tgl.groupedTasks.Today
	case 1:
		return tgl.groupedTasks.ThisWeek
	case 2:
		return tgl.groupedTasks.AllTasks
	}
	return nil
}

func (tgl *TimeGroupedTaskList) currentSectionLength() int {
	tasks := tgl.currentSectionTasks()
	if tasks == nil {
		return 0
	}
	return len(tasks)
}

func (tgl *TimeGroupedTaskList) findNextNonEmptySection(startIdx, direction int) int {
	sections := []func() []journal.Task{
		func() []journal.Task { return tgl.groupedTasks.Today },
		func() []journal.Task { return tgl.groupedTasks.ThisWeek },
		func() []journal.Task { return tgl.groupedTasks.AllTasks },
	}
	for i := 1; i <= 3; i++ {
		nextIdx := (startIdx + direction*i + 3) % 3
		if len(sections[nextIdx]()) > 0 {
			return nextIdx
		}
	}
	return -1
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
			currentLen := tgl.currentSectionLength()
			if currentLen == 0 {
				nextIdx := tgl.findNextNonEmptySection(tgl.sectionIndex, 1)
				if nextIdx != -1 {
					tgl.sectionIndex = nextIdx
					tgl.updateListForSection()
				}
				return tgl, nil
			}
			if tgl.list.Index() >= currentLen-1 {
				nextIdx := tgl.findNextNonEmptySection(tgl.sectionIndex, 1)
				if nextIdx != -1 {
					tgl.sectionIndex = nextIdx
					tgl.updateListForSection()
					tgl.list.Select(0)
				}
			} else {
				var cmd tea.Cmd
				tgl.list, cmd = tgl.list.Update(msg)
				return tgl, cmd
			}
		case "k", "up":
			currentLen := tgl.currentSectionLength()
			if currentLen == 0 {
				prevIdx := tgl.findNextNonEmptySection(tgl.sectionIndex, -1)
				if prevIdx != -1 {
					tgl.sectionIndex = prevIdx
					tgl.updateListForSection()
					tgl.list.Select(len(tgl.currentSectionTasks()) - 1)
				}
				return tgl, nil
			}
			if tgl.list.Index() <= 0 {
				prevIdx := tgl.findNextNonEmptySection(tgl.sectionIndex, -1)
				if prevIdx != -1 {
					tgl.sectionIndex = prevIdx
					tgl.updateListForSection()
					tgl.list.Select(len(tgl.currentSectionTasks()) - 1)
				}
			} else {
				var cmd tea.Cmd
				tgl.list, cmd = tgl.list.Update(msg)
				return tgl, cmd
			}
		case " ":
			selectedItem := tgl.list.SelectedItem()
			if selectedItem == nil {
				return tgl, nil
			}
			taskItem, ok := selectedItem.(TaskItem)
			if !ok || taskItem.isNote {
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
			if !ok || taskItem.isNote || len(taskItem.task.Notes) == 0 {
				return tgl, nil
			}
			tgl.collapsedMap[taskItem.task.ID] = !tgl.collapsedMap[taskItem.task.ID]
			tgl.updateListForSection()
			tgl.list.Select(0)
		}
	}
	var cmd tea.Cmd
	tgl.list, cmd = tgl.list.Update(msg)
	return tgl, cmd
}

func (tgl *TimeGroupedTaskList) View() string {
	var sb strings.Builder

	sectionNames := []string{"TODAY", "THIS WEEK", "ALL TASKS"}
	sectionCounts := []int{tgl.groupedTasks.TodayCount(), tgl.groupedTasks.ThisWeekCount(), tgl.groupedTasks.AllTasksCount()}
	sectionTasks := [][]journal.Task{tgl.groupedTasks.Today, tgl.groupedTasks.ThisWeek, tgl.groupedTasks.AllTasks}

	for i, name := range sectionNames {
		count := sectionCounts[i]
		tasks := sectionTasks[i]

		if i == tgl.sectionIndex {
			sb.WriteString(fmt.Sprintf("\n━━ %s (%d) ━━\n", name, count))
			if len(tasks) == 0 {
				sb.WriteString("  No tasks\n")
			}
			sb.WriteString(tgl.list.View())
		} else {
			sb.WriteString(fmt.Sprintf("\n━━ %s (%d) ━━\n", name, count))
			if len(tasks) == 0 {
				sb.WriteString("  No tasks\n")
			} else {
				for _, task := range tasks {
					if task.Status == journal.TaskOpen {
						sb.WriteString(fmt.Sprintf("[ ] %s\n", task.Text))
					}
				}
			}
		}
	}

	return sb.String()
}

func (tgl *TimeGroupedTaskList) SetFocused(focused bool) {
	tgl.focused = focused
	if focused {
		tgl.list.Styles.Title = focusedTaskListTitleStyle
	} else {
		tgl.list.Styles.Title = taskListTitleStyle
	}
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
	tasks := tgl.currentSectionTasks()
	for i := range tasks {
		if tasks[i].ID == taskItem.task.ID {
			return &tasks[i]
		}
	}
	return nil
}

func (tgl *TimeGroupedTaskList) UpdateTasks(tasks []journal.Task) {
	selectedItem := tgl.list.SelectedItem()
	var selectedTaskID string
	if selectedItem != nil {
		if taskItem, ok := selectedItem.(TaskItem); ok {
			selectedTaskID = taskItem.task.ID
		}
	}

	tgl.groupedTasks = GroupTasksByTime(tasks)

	if selectedTaskID != "" {
		for i, section := range [][]journal.Task{tgl.groupedTasks.Today, tgl.groupedTasks.ThisWeek, tgl.groupedTasks.AllTasks} {
			for _, task := range section {
				if task.ID == selectedTaskID {
					tgl.sectionIndex = i
					break
				}
			}
		}
	}

	tgl.updateListForSection()

	if selectedTaskID != "" {
		items := tgl.list.Items()
		for i, item := range items {
			if taskItem, ok := item.(TaskItem); ok && !taskItem.isNote && taskItem.task.ID == selectedTaskID {
				tgl.list.Select(i)
				break
			}
		}
	}
}
