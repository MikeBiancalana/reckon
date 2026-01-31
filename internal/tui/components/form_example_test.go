package components_test

import (
	"fmt"
	"strings"

	"github.com/MikeBiancalana/reckon/internal/tui/components"
	tea "github.com/charmbracelet/bubbletea"
)

// ExampleForm demonstrates basic usage of the Form component
func ExampleForm() {
	// Create a new form
	form := components.NewForm("Create Task")

	// Add fields
	form.AddField(components.FormField{
		Label:       "Task Name",
		Key:         "name",
		Type:        components.FieldTypeText,
		Required:    true,
		Placeholder: "Enter task name",
	}).AddField(components.FormField{
		Label:       "Description",
		Key:         "description",
		Type:        components.FieldTypeText,
		Required:    false,
		Placeholder: "Optional description",
	}).AddField(components.FormField{
		Label:       "Due Date",
		Key:         "due_date",
		Type:        components.FieldTypeDate,
		Required:    false,
		Placeholder: "t, tm, +3d, mon, 2026-12-31",
	})

	// Show the form
	form.Show()

	// The form is now ready to be used in a Bubble Tea Update loop
	fmt.Println("Form created successfully")
	// Output: Form created successfully
}

// ExampleForm_withCustomValidator demonstrates using a custom validator
func ExampleForm_withCustomValidator() {
	form := components.NewForm("Create User")

	// Add a field with custom validation
	form.AddField(components.FormField{
		Label:    "Email",
		Key:      "email",
		Type:     components.FieldTypeText,
		Required: true,
		Validator: func(value string) error {
			if !strings.Contains(value, "@") {
				return fmt.Errorf("invalid email format")
			}
			return nil
		},
	})

	form.Show()
	fmt.Println("Form with validator created")
	// Output: Form with validator created
}

// ExampleForm_integration demonstrates integration in a Bubble Tea model
func ExampleForm_integration() {
	type model struct {
		form *components.Form
	}

	// Initialize model with form
	m := model{
		form: components.NewForm("Add Entry"),
	}

	m.form.AddField(components.FormField{
		Label:    "Content",
		Key:      "content",
		Type:     components.FieldTypeText,
		Required: true,
	})

	// Show the form
	m.form.Show()

	// In the Update function, handle form messages:
	handleUpdate := func(msg tea.Msg) {
		switch msg := msg.(type) {
		case components.FormSubmitMsg:
			// Form was submitted successfully
			values := msg.Result.Values
			fmt.Printf("Content: %s\n", values["content"])

		case components.FormCancelMsg:
			// Form was cancelled
			fmt.Println("Form cancelled")
		}
	}

	// Simulate form submission
	handleUpdate(components.FormSubmitMsg{
		Result: components.FormResult{
			Values: map[string]string{
				"content": "Test entry",
			},
		},
	})

	// Output: Content: Test entry
}

// ExampleForm_dateField demonstrates using date fields
func ExampleForm_dateField() {
	form := components.NewForm("Schedule Task")

	form.AddField(components.FormField{
		Label:       "Task Name",
		Key:         "name",
		Type:        components.FieldTypeText,
		Required:    true,
		Placeholder: "Enter task name",
	}).AddField(components.FormField{
		Label:       "Due Date",
		Key:         "due_date",
		Type:        components.FieldTypeDate,
		Required:    false,
		Placeholder: "t, tm, +3d, mon, 2026-12-31",
	})

	form.Show()

	// Set values programmatically
	form.SetValues(map[string]string{
		"name":     "Complete project",
		"due_date": "tm",
	})

	// Parse the date value
	date, err := form.ParsedDateValue("due_date")
	if err == nil && !date.IsZero() {
		fmt.Println("Date field parsed successfully")
	}

	// Output: Date field parsed successfully
}

// ExampleForm_multipleFields demonstrates a form with multiple field types
func ExampleForm_multipleFields() {
	form := components.NewForm("Task Details")

	form.AddField(components.FormField{
		Label:       "Title",
		Key:         "title",
		Type:        components.FieldTypeText,
		Required:    true,
		Placeholder: "Task title",
	}).AddField(components.FormField{
		Label:       "Tags",
		Key:         "tags",
		Type:        components.FieldTypeText,
		Required:    false,
		Placeholder: "#tag1 #tag2",
	}).AddField(components.FormField{
		Label:       "Due Date",
		Key:         "due_date",
		Type:        components.FieldTypeDate,
		Required:    false,
		Placeholder: "Optional due date",
	}).AddField(components.FormField{
		Label:       "Notes",
		Key:         "notes",
		Type:        components.FieldTypeText,
		Required:    false,
		Placeholder: "Additional notes",
	})

	form.Show()
	fmt.Println("Multi-field form created")
	// Output: Multi-field form created
}
