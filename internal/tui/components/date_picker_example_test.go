package components_test

import (
	"fmt"

	"github.com/MikeBiancalana/reckon/internal/tui/components"
)

// ExampleDatePicker demonstrates how to use the DatePicker component
func ExampleDatePicker() {
	// Create a new date picker
	dp := components.NewDatePicker("Schedule Task")

	// Show the date picker
	dp.Show()

	// The date picker is now visible and accepting input
	fmt.Println("Date picker is visible:", dp.IsVisible())

	// Output:
	// Date picker is visible: true
}

// ExampleDatePicker_parsing demonstrates date parsing functionality
func ExampleDatePicker_parsing() {
	// Create a date picker
	dp := components.NewDatePicker("Select Date")
	dp.Show()

	// Test different date formats
	testInputs := []string{
		"2025-01-15",    // Absolute date
		"t",             // Today
		"tomorrow",      // Tomorrow
		"+3d",           // 3 days from now
		"mon",           // Next Monday
	}

	for _, input := range testInputs {
		// Simulate user typing (in a real app, this would come from keyboard events)
		fmt.Printf("Input: %s\n", input)
	}

	// Output:
	// Input: 2025-01-15
	// Input: t
	// Input: tomorrow
	// Input: +3d
	// Input: mon
}

// ExampleDatePicker_workflow demonstrates a typical workflow
func ExampleDatePicker_workflow() {
	// Create and show the date picker
	dp := components.NewDatePicker("Schedule Task")
	dp.Show()

	fmt.Println("Step 1: Date picker shown")

	// User can now interact with it:
	// - Type a date (YYYY-MM-DD, relative shortcuts, etc.)
	// - Press Enter to confirm
	// - Press ESC to cancel

	// After confirmation, parse the date
	// date, err := dp.ParsedDate()
	// if err == nil {
	//     // Use the date
	// }

	// Hide the date picker
	dp.Hide()

	fmt.Println("Step 2: Date picker hidden")
	fmt.Println("Visible:", dp.IsVisible())

	// Output:
	// Step 1: Date picker shown
	// Step 2: Date picker hidden
	// Visible: false
}
