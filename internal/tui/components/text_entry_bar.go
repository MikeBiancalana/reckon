package components

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// EntryMode represents the current input mode of the text entry bar
type EntryMode string

const (
	ModeInactive  EntryMode = ""
	ModeTask      EntryMode = "task"
	ModeIntention EntryMode = "intention"
	ModeWin       EntryMode = "win"
	ModeLog       EntryMode = "log"
	ModeNote      EntryMode = "note"
	ModeLogNote   EntryMode = "log_note"
)

var (
	activeStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("39")).
			Padding(0, 1)

	inactiveStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1).
			Foreground(lipgloss.Color("240"))
)

// TextEntryBar is a Bubble Tea component for text input with different modes
type TextEntryBar struct {
	textInput textinput.Model
	mode      EntryMode
	width     int
}

// NewTextEntryBar creates a new TextEntryBar component
func NewTextEntryBar() *TextEntryBar {
	ti := textinput.New()
	ti.Placeholder = ""
	ti.CharLimit = 500

	return &TextEntryBar{
		textInput: ti,
		mode:      ModeInactive,
		width:     80,
	}
}

// Update handles Bubble Tea messages
func (teb *TextEntryBar) Update(msg tea.Msg) (*TextEntryBar, tea.Cmd) {
	// Only process messages when focused
	if !teb.textInput.Focused() {
		return teb, nil
	}

	// Handle ESC key to blur
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.Type == tea.KeyEsc {
			teb.Blur()
			return teb, nil
		}
	}

	// Delegate other messages to textInput
	var cmd tea.Cmd
	teb.textInput, cmd = teb.textInput.Update(msg)
	return teb, cmd
}

// View renders the text entry bar
func (teb *TextEntryBar) View() string {
	var content string

	if teb.mode == ModeInactive {
		// Inactive state
		prompt := "Press t (task), i (intention), w (win), L (log), or n (note) to add entry"
		content = "> " + prompt
		return inactiveStyle.Width(teb.width - 2).Render(content)
	}

	// Active state - show mode-specific prompt
	prompt := teb.getPromptForMode()
	content = "> " + prompt + teb.textInput.View()

	return activeStyle.Width(teb.width - 2).Render(content)
}

// getPromptForMode returns the prompt string for the current mode
func (teb *TextEntryBar) getPromptForMode() string {
	switch teb.mode {
	case ModeTask:
		return "Add task: "
	case ModeIntention:
		return "Add intention: "
	case ModeWin:
		return "Add win: "
	case ModeLog:
		return "Add log entry: "
	case ModeNote:
		return "Add note: "
	case ModeLogNote:
		return "Add note: "
	default:
		return ""
	}
}

// SetWidth sets the width of the text entry bar
func (teb *TextEntryBar) SetWidth(width int) {
	teb.width = width

	// Update textInput width to account for prompt and styling
	promptLen := len("> " + teb.getPromptForMode())
	// Account for border (2) and padding (2)
	availableWidth := width - promptLen - 4
	if availableWidth < 10 {
		availableWidth = 10
	}
	teb.textInput.Width = availableWidth
}

// SetMode sets the current entry mode
func (teb *TextEntryBar) SetMode(mode EntryMode) {
	teb.mode = mode

	// Update textInput width when mode changes (different prompt lengths)
	if teb.width > 0 {
		teb.SetWidth(teb.width)
	}

	// Update placeholder based on mode
	if mode == ModeInactive {
		teb.textInput.Placeholder = ""
	} else {
		teb.textInput.Placeholder = "Type here..."
	}
}

// GetMode returns the current entry mode
func (teb *TextEntryBar) GetMode() EntryMode {
	return teb.mode
}

// GetValue returns the current input value
func (teb *TextEntryBar) GetValue() string {
	return teb.textInput.Value()
}

// Clear resets the input value
func (teb *TextEntryBar) Clear() {
	teb.textInput.SetValue("")
}

// Focus focuses the text input
func (teb *TextEntryBar) Focus() tea.Cmd {
	return teb.textInput.Focus()
}

// Blur removes focus from the text input
func (teb *TextEntryBar) Blur() {
	teb.textInput.Blur()
}

// IsFocused returns whether the text input is focused
func (teb *TextEntryBar) IsFocused() bool {
	return teb.textInput.Focused()
}
