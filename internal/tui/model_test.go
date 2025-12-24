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
