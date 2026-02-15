package components_test

import (
	"fmt"

	"github.com/MikeBiancalana/reckon/internal/tui/components"
	tea "github.com/charmbracelet/bubbletea"
)

// ExampleTextEditor_integration demonstrates the basic usage of the TextEditor component
// in a Bubble Tea application.
func ExampleTextEditor_integration() {
	// This example demonstrates:
	// 1. Create a text editor
	// 2. Show the editor to the user
	// 3. Handle TextEditorSubmitMsg and TextEditorCancelMsg in Update()

	editor := components.NewTextEditor("Enter Note")
	editor.Show()

	// Set some text
	editor.SetText("This is a note")

	// Simulate submitting with Ctrl+D
	editor, cmd := editor.Update(tea.KeyMsg{Type: tea.KeyCtrlD})

	// The submit message is returned
	msg := cmd()
	if submitMsg, ok := msg.(components.TextEditorSubmitMsg); ok {
		fmt.Printf("Note text: %s\n", submitMsg.Text)
		fmt.Printf("Editor visible: %t\n", editor.IsVisible())
	}

	// Output:
	// Note text: This is a note
	// Editor visible: false
}

// ExampleTextEditor_withTaskNote demonstrates using the text editor for task notes
func ExampleTextEditor_withTaskNote() {
	// This example shows a typical workflow:
	// 1. User selects a task (task picker - not shown here)
	// 2. User enters note text with the text editor
	// 3. Note is saved to the task

	editor := components.NewTextEditor("Add Note to Task")
	editor.Show()
	editor.SetSize(80, 15)

	// Set note text
	editor.SetText("Implemented authentication feature")

	// Simulate submitting
	editor, cmd := editor.Update(tea.KeyMsg{Type: tea.KeyCtrlD})

	// Handle the message
	msg := cmd()
	if submitMsg, ok := msg.(components.TextEditorSubmitMsg); ok {
		taskID := "task-123"
		noteText := submitMsg.Text

		fmt.Printf("Task ID: %s\n", taskID)
		fmt.Printf("Note: %s\n", noteText)
		fmt.Printf("Saved: true\n")
	}

	// Output:
	// Task ID: task-123
	// Note: Implemented authentication feature
	// Saved: true
}
