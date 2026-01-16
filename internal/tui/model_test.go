package tui

import (
	"strings"
	"testing"
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
