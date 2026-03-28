package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/MikeBiancalana/reckon/internal/journal"
	rtime "github.com/MikeBiancalana/reckon/internal/time"
	"github.com/MikeBiancalana/reckon/internal/tui/components"
)

func TestMinimumTerminalSizeConstants(t *testing.T) {
	// Test that our constants are defined correctly
	if MinTerminalWidth != 80 {
		t.Errorf("Expected MinTerminalWidth to be 80, got %d", MinTerminalWidth)
	}

	if MinTerminalHeight != 24 {
		t.Errorf("Expected MinTerminalHeight to be 24, got %d", MinTerminalHeight)
	}
}

func TestTerminalTooSmallValidation(t *testing.T) {
	// Test the validation logic directly without creating a full model
	testCases := []struct {
		width    int
		height   int
		expected bool
		name     string
	}{
		{60, 25, true, "Width too small"},
		{80, 20, true, "Height too small"},
		{60, 20, true, "Both dimensions too small"},
		{80, 24, false, "Exactly minimum dimensions"},
		{120, 40, false, "Larger than minimum"},
		{79, 24, true, "Width one pixel under minimum"},
		{80, 23, true, "Height one pixel under minimum"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test the validation logic that's used in the WindowSizeMsg handler
			tooSmall := tc.width < MinTerminalWidth || tc.height < MinTerminalHeight
			if tooSmall != tc.expected {
				t.Errorf("Expected terminalTooSmall=%v for dimensions %dx%d, got %v",
					tc.expected, tc.width, tc.height, tooSmall)
			}
		})
	}
}

func TestTerminalTooSmallViewContent(t *testing.T) {
	// Create a minimal model just for testing the view
	model := &Model{
		terminalTooSmall: true,
		width:            60,
		height:           20,
	}

	// Get the view
	view := model.terminalTooSmallView()

	// Check that the view contains expected text
	expectedTexts := []string{
		"Terminal Too Small",
		"Current: 60x20",
		"Required: 80x24 or larger",
		"Resize your terminal",
	}

	for _, expected := range expectedTexts {
		if !strings.Contains(view, expected) {
			t.Errorf("Expected view to contain '%s', but got view: %s", expected, view)
		}
	}

	// Verify the view is not empty
	if view == "" {
		t.Error("Expected terminalTooSmallView to return a non-empty string")
	}
}

func TestParseTaskTags(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantText string
		wantTags []string
	}{
		{
			name:     "empty input",
			input:    "",
			wantText: "",
			wantTags: nil,
		},
		{
			name:     "no tags",
			input:    "Buy milk and eggs",
			wantText: "Buy milk and eggs",
			wantTags: nil,
		},
		{
			name:     "single tag at end",
			input:    "Buy milk #groceries",
			wantText: "Buy milk",
			wantTags: []string{"groceries"},
		},
		{
			name:     "single tag at start",
			input:    "#important Review PR",
			wantText: "Review PR",
			wantTags: []string{"important"},
		},
		{
			name:     "multiple tags",
			input:    "Task #tag1 #tag2 #tag3",
			wantText: "Task",
			wantTags: []string{"tag1", "tag2", "tag3"},
		},
		{
			name:     "tags with mixed case",
			input:    "Task #TAG1 #Tag2",
			wantText: "Task",
			wantTags: []string{"tag1", "tag2"},
		},
		{
			name:     "just hash symbol",
			input:    "Task #",
			wantText: "Task #",
			wantTags: nil,
		},
		{
			name:     "double hash",
			input:    "Task ##",
			wantText: "Task",
			wantTags: nil,
		},
		{
			name:     "tag with punctuation",
			input:    "Task #urgent, #important",
			wantText: "Task",
			wantTags: []string{"urgent,", "important"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotText, gotTags := parseTaskTags(tt.input)

			if gotText != tt.wantText {
				t.Errorf("parseTaskTags(%q) text = %q, want %q", tt.input, gotText, tt.wantText)
			}

			if len(gotTags) != len(tt.wantTags) {
				t.Errorf("parseTaskTags(%q) tags count = %d, want %d", tt.input, len(gotTags), len(tt.wantTags))
				return
			}

			for i, tag := range gotTags {
				if tag != tt.wantTags[i] {
					t.Errorf("parseTaskTags(%q) tags[%d] = %q, want %q", tt.input, i, tag, tt.wantTags[i])
				}
			}
		})
	}
}

// newMinimalModelForRender constructs a Model with just enough components initialized
// to make renderNewLayout() produce meaningful output without panicking.
func newMinimalModelForRender(termWidth, termHeight int) *Model {
	sb := components.NewStatusBar()
	sb.SetWidth(termWidth)

	return &Model{
		width:        termWidth,
		height:       termHeight,
		logView:      components.NewLogView([]journal.LogEntry{}),
		textEntryBar: components.NewTextEntryBar(),
		statusBar:    sb,
		summaryView:  components.NewSummaryView(),
	}
}

// TestSectionCount_TwoSections asserts that the Section enum has exactly 2 sections
// (SectionLogs, SectionTasks) after removing SectionNotes.
func TestSectionCount_TwoSections(t *testing.T) {
	if SectionCount != 2 {
		t.Errorf("Expected SectionCount=2 (Logs, Tasks), got %d", int(SectionCount))
	}
}

// TestRenderNewLayout_LineCount verifies that the rendered view does not exceed
// the terminal height when successMsg and summary are both empty (the common case).
func TestRenderNewLayout_LineCount(t *testing.T) {
	tests := []struct {
		name           string
		termWidth      int
		termHeight     int
		successMessage string
		summaryVisible bool
		maxExtraLines  int
	}{
		{
			name:           "common case: no success, summary hidden",
			termWidth:      120,
			termHeight:     50,
			successMessage: "",
			summaryVisible: false,
			maxExtraLines:  0,
		},
		{
			name:           "summary visible, no success",
			termWidth:      120,
			termHeight:     50,
			successMessage: "",
			summaryVisible: true,
			maxExtraLines:  0,
		},
		{
			name:           "success message present, summary hidden",
			termWidth:      120,
			termHeight:     50,
			successMessage: "Task added!",
			summaryVisible: false,
			maxExtraLines:  1,
		},
		{
			name:           "both success and summary present",
			termWidth:      120,
			termHeight:     50,
			successMessage: "Win recorded!",
			summaryVisible: true,
			maxExtraLines:  1,
		},
		{
			name:           "minimum terminal dimensions",
			termWidth:      80,
			termHeight:     24,
			successMessage: "",
			summaryVisible: false,
			maxExtraLines:  0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newMinimalModelForRender(tc.termWidth, tc.termHeight)
			m.successMessage = tc.successMessage

			if tc.summaryVisible {
				m.summaryView.SetVisible(true)
				m.summaryView.SetSummary(&rtime.TimeSummary{Meetings: 30, Tasks: 60})
			}

			// Provide a minimal journal so renderNewLayout is triggered
			m.currentJournal = &journal.Journal{}

			view := m.renderNewLayout()

			// Count rendered lines. strings.Split on "\n" gives len-1 separators.
			lineCount := len(strings.Split(view, "\n"))

			allowedMax := tc.termHeight + tc.maxExtraLines
			if lineCount > allowedMax {
				t.Errorf(
					"rendered view has %d lines, want ≤ %d (termHeight=%d, maxExtraLines=%d)\n"+
						"This indicates the unconditional newline join bug is present.",
					lineCount, allowedMax, tc.termHeight, tc.maxExtraLines,
				)
			}
		})
	}
}

// TestRenderNewLayout_NoSpuriousBlankLine verifies that when successMsg is empty
// there is no blank line between the text-entry box and the status bar.
func TestRenderNewLayout_NoSpuriousBlankLine(t *testing.T) {
	t.Run("no blank line between text entry and status when successMsg empty", func(t *testing.T) {
		m := newMinimalModelForRender(120, 50)
		m.currentJournal = &journal.Journal{}

		view := m.renderNewLayout()

		if strings.Contains(view, "\n\n") {
			t.Errorf(
				"rendered view contains consecutive newlines (spurious blank line).\n"+
					"This indicates the unconditional newline join bug is present.\n"+
					"Relevant tail of view:\n%s",
				lastNLines(view, 10),
			)
		}
	})

	t.Run("no blank line when both successMsg and summary are empty", func(t *testing.T) {
		m := newMinimalModelForRender(80, 24)
		m.currentJournal = &journal.Journal{}
		m.successMessage = ""

		view := m.renderNewLayout()

		if strings.Contains(view, "\n\n") {
			t.Errorf(
				"rendered view contains consecutive newlines (spurious blank line) at minimum terminal size.\n"+
					"Relevant tail of view:\n%s",
				lastNLines(view, 10),
			)
		}
	})
}

// lastNLines returns the last n lines of s for diagnostic output.
func lastNLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

func TestHelpViewContainsSchedulingShortcuts(t *testing.T) {
	model := &Model{}

	view := model.helpView()

	expectedEntries := []string{
		"s          Schedule task",
		"D          Set deadline",
	}

	for _, entry := range expectedEntries {
		if !strings.Contains(view, entry) {
			t.Errorf("helpView() should contain %q for scheduling shortcuts, got:\n%s", entry, view)
		}
	}
}

func TestHelpViewContainsExistingShortcuts(t *testing.T) {
	model := &Model{}

	view := model.helpView()

	// Ensure existing shortcuts are not broken
	existingEntries := []string{
		"h, ←",
		"l, →",
		"tab",
		"j, k",
		"q, ctrl+c",
		"?",
	}

	for _, entry := range existingEntries {
		if !strings.Contains(view, entry) {
			t.Errorf("helpView() should contain existing shortcut %q, got:\n%s", entry, view)
		}
	}
}

// --- Tests for new task list rendering (reckon-obed) ---

func makeTestTask(id, text string, status journal.TaskStatus) journal.Task {
	return journal.Task{
		ID:        id,
		Text:      text,
		Status:    status,
		CreatedAt: time.Now(),
	}
}

func TestRenderTaskList_Empty(t *testing.T) {
	m := &Model{
		tasks:         []journal.Task{},
		selectedIndex: -1,
	}
	result := m.renderTaskList(80, 20)
	if result == "" {
		t.Error("renderTaskList with empty tasks should return non-empty placeholder string")
	}
	if !strings.Contains(result, "No tasks") {
		t.Errorf("renderTaskList with empty tasks should contain 'No tasks', got: %q", result)
	}
}

func TestRenderTaskList_SelectedHighlighted(t *testing.T) {
	tasks := []journal.Task{
		makeTestTask("t1", "First task", journal.TaskOpen),
		makeTestTask("t2", "Second task", journal.TaskOpen),
		makeTestTask("t3", "Third task", journal.TaskOpen),
	}
	m := &Model{
		tasks:         tasks,
		selectedIndex: 1,
	}
	result := m.renderTaskList(80, 10)

	// The selected task's text should appear in the output
	if !strings.Contains(result, "Second task") {
		t.Errorf("renderTaskList should contain selected task text, got: %q", result)
	}
	// All tasks should appear
	if !strings.Contains(result, "First task") {
		t.Errorf("renderTaskList should contain all tasks, got: %q", result)
	}
}

func TestRenderTaskList_ScrollOffset(t *testing.T) {
	// Create 20 tasks
	var tasks []journal.Task
	for i := 0; i < 20; i++ {
		tasks = append(tasks, makeTestTask(
			fmt.Sprintf("t%d", i),
			fmt.Sprintf("Task %d", i),
			journal.TaskOpen,
		))
	}
	m := &Model{
		tasks:         tasks,
		selectedIndex: 15, // Beyond visible area with height=5
	}
	result := m.renderTaskList(80, 5)

	// Task 15 should be visible
	if !strings.Contains(result, "Task 15") {
		t.Errorf("renderTaskList should scroll to show selected task 15, got: %q", result)
	}
	// Task 0 should NOT be visible (scrolled away)
	if strings.Contains(result, "Task 0\n") || strings.HasPrefix(result, "[ ] Task 0") {
		t.Errorf("renderTaskList should have scrolled past task 0, got: %q", result)
	}
}

func TestRenderDetailArea_NoTasks(t *testing.T) {
	m := &Model{
		tasks:         []journal.Task{},
		selectedIndex: -1,
	}
	result := m.renderDetailArea(80, 10)
	if result == "" {
		t.Error("renderDetailArea should return non-empty string")
	}
	// Should show some kind of "no selection" message
	if !strings.Contains(result, "No task") && !strings.Contains(result, "NOTES") {
		t.Errorf("renderDetailArea with no tasks should indicate no selection, got: %q", result)
	}
}

func TestRenderDetailArea_TaskWithNotes(t *testing.T) {
	task := journal.Task{
		ID:   "t1",
		Text: "My task",
		Notes: []journal.TaskNote{
			{ID: "n1", Text: "First note"},
			{ID: "n2", Text: "Second note"},
		},
		Status:    journal.TaskOpen,
		CreatedAt: time.Now(),
	}
	m := &Model{
		tasks:         []journal.Task{task},
		selectedIndex: 0,
	}
	result := m.renderDetailArea(80, 10)

	if !strings.Contains(result, "First note") {
		t.Errorf("renderDetailArea should show task notes, got: %q", result)
	}
	if !strings.Contains(result, "Second note") {
		t.Errorf("renderDetailArea should show all notes, got: %q", result)
	}
}

func TestRenderDetailArea_TaskWithoutNotes(t *testing.T) {
	task := makeTestTask("t1", "My task", journal.TaskOpen)
	m := &Model{
		tasks:         []journal.Task{task},
		selectedIndex: 0,
	}
	result := m.renderDetailArea(80, 10)

	if !strings.Contains(result, "No notes") {
		t.Errorf("renderDetailArea should show 'No notes' placeholder, got: %q", result)
	}
}

func TestClampIndex(t *testing.T) {
	tests := []struct {
		idx, length, want int
	}{
		{0, 0, -1},
		{5, 0, -1},
		{-1, 3, 0},
		{0, 3, 0},
		{2, 3, 2},
		{3, 3, 2},
		{10, 3, 2},
	}
	for _, tt := range tests {
		got := clampIndex(tt.idx, tt.length)
		if got != tt.want {
			t.Errorf("clampIndex(%d, %d) = %d, want %d", tt.idx, tt.length, got, tt.want)
		}
	}
}
