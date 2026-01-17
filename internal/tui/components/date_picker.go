package components

import (
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	datePickerBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("39")).
				Padding(1, 2)

	datePickerTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("39"))

	datePickerPreviewStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("40")).
				Italic(true)

	datePickerErrorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("196")).
				Italic(true)

	datePickerHelpStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240"))
)

// DatePicker is a TUI component for selecting dates
type DatePicker struct {
	textInput textinput.Model
	visible   bool
	title     string
	error     string
	preview   string
	width     int
}

// NewDatePicker creates a new date picker component
func NewDatePicker(title string) *DatePicker {
	ti := textinput.New()
	ti.Placeholder = "YYYY-MM-DD, t, tm, mon-sun, +3d, +2w"
	ti.CharLimit = 100
	ti.Width = 30

	return &DatePicker{
		textInput: ti,
		visible:   false,
		title:     title,
		width:     40,
	}
}

// Show displays the date picker and focuses the input
func (dp *DatePicker) Show() tea.Cmd {
	dp.visible = true
	dp.error = ""
	dp.preview = ""
	dp.textInput.SetValue("")
	return dp.textInput.Focus()
}

// Hide hides the date picker
func (dp *DatePicker) Hide() {
	dp.visible = false
	dp.textInput.Blur()
	dp.error = ""
	dp.preview = ""
	dp.textInput.SetValue("")
}

// IsVisible returns whether the date picker is visible
func (dp *DatePicker) IsVisible() bool {
	return dp.visible
}

// GetValue returns the current input value
func (dp *DatePicker) GetValue() string {
	return dp.textInput.Value()
}

// SetWidth sets the width of the date picker
func (dp *DatePicker) SetWidth(width int) {
	dp.width = width
}

// Update handles Bubble Tea messages
func (dp *DatePicker) Update(msg tea.Msg) (*DatePicker, tea.Cmd) {
	if !dp.visible {
		return dp, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc:
			dp.Hide()
			return dp, nil
		case tea.KeyEnter:
			// Validate and return if valid
			input := dp.textInput.Value()
			if input == "" {
				dp.error = "Please enter a date"
				return dp, nil
			}

			// Try to parse the date
			_, err := ParseRelativeDate(input)
			if err != nil {
				dp.error = "Invalid date: " + err.Error()
				return dp, nil
			}

			// Valid date - will be handled by parent
			return dp, nil
		}
	}

	// Update text input and refresh preview
	var cmd tea.Cmd
	dp.textInput, cmd = dp.textInput.Update(msg)

	// Update preview based on current input
	dp.updatePreview()

	return dp, cmd
}

// updatePreview updates the preview and error messages based on current input
func (dp *DatePicker) updatePreview() {
	input := dp.textInput.Value()

	if input == "" {
		dp.error = ""
		dp.preview = ""
		return
	}

	// Try to parse the date
	date, err := ParseRelativeDate(input)
	if err != nil {
		dp.error = err.Error()
		dp.preview = ""
		return
	}

	// Valid date - show preview
	dp.error = ""
	description := GetDateDescription(date)
	dp.preview = FormatDate(date) + " (" + description + ")"
}

// View renders the date picker
func (dp *DatePicker) View() string {
	if !dp.visible {
		return ""
	}

	// Build the content
	var content string

	// Title
	content += datePickerTitleStyle.Render(dp.title) + "\n\n"

	// Input field
	content += "Date: " + dp.textInput.View() + "\n"

	// Preview or error
	if dp.error != "" {
		content += datePickerErrorStyle.Render("✗ "+dp.error) + "\n"
	} else if dp.preview != "" {
		content += datePickerPreviewStyle.Render("→ "+dp.preview) + "\n"
	} else {
		content += "\n"
	}

	// Help text
	content += "\n"
	content += datePickerHelpStyle.Render("ESC: cancel  ENTER: confirm")

	// Wrap in box
	return datePickerBoxStyle.Render(content)
}

// ParsedDate returns the parsed date from the current input
// Returns error if input is invalid or empty
func (dp *DatePicker) ParsedDate() (time.Time, error) {
	input := dp.textInput.Value()
	if input == "" {
		return time.Time{}, nil
	}
	return ParseRelativeDate(input)
}
