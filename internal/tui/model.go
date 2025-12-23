package tui

import (
	"fmt"
	"time"

	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/MikeBiancalana/reckon/internal/tui/components"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// Section represents different sections of the journal
type Section int

const (
	SectionIntentions Section = iota
	SectionWins
	SectionLogs
	SectionCount // Keep this last to get the count
)

// Model represents the main TUI state
type Model struct {
	service        *journal.Service
	currentDate    string
	currentJournal *journal.Journal
	focusedSection Section
	width          int
	height         int

	// Components
	intentionList *components.IntentionList
	winsView      *components.WinsView
	logView       *components.LogView
	textInput     textinput.Model
	statusBar     *components.StatusBar

	// State for input modes
	inputMode bool
	inputType string // "intention", "win", "log"
	helpMode  bool
	lastError error
}

// NewModel creates a new TUI model
func NewModel(service *journal.Service) *Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = ""
	ti.CharLimit = 200
	ti.Width = 50

	return &Model{
		service:        service,
		currentDate:    time.Now().Format("2006-01-02"),
		focusedSection: SectionIntentions,
		textInput:      ti,
		statusBar:      components.NewStatusBar(),
	}
}

// Init initializes the model
func (m *Model) Init() tea.Cmd {
	return m.loadJournal()
}

// loadJournal loads the journal for the current date
func (m *Model) loadJournal() tea.Cmd {
	return func() tea.Msg {
		j, err := m.service.GetByDate(m.currentDate)
		if err != nil {
			return errMsg{err}
		}
		return journalLoadedMsg{*j}
	}
}

// Update handles messages and updates the model
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.statusBar != nil {
			m.statusBar.SetWidth(msg.Width)
		}
		if m.intentionList != nil {
			m.intentionList.SetSize(msg.Width, msg.Height-2) // Leave space for status bar
		}
		if m.winsView != nil {
			m.winsView.SetSize(msg.Width, msg.Height-2) // Leave space for status bar
		}
		if m.logView != nil {
			m.logView.SetSize(msg.Width, msg.Height-2) // Leave space for status bar
		}
		return m, nil

	case journalLoadedMsg:
		m.currentJournal = &msg.journal
		m.intentionList = components.NewIntentionList(msg.journal.Intentions)
		m.winsView = components.NewWinsView(msg.journal.Wins)
		m.logView = components.NewLogView(msg.journal.LogEntries)
		return m, nil

	case journalUpdatedMsg:
		// Reset input state after successful submission
		if m.inputMode {
			m.inputMode = false
			m.textInput.SetValue("")
			m.textInput.Blur()
		}
		return m, m.loadJournal()

	case errMsg:
		// Reset input state and store error for display
		if m.inputMode {
			m.inputMode = false
			m.textInput.SetValue("")
			m.textInput.Blur()
		}
		m.lastError = msg.err
		return m, nil

	case tea.KeyMsg:
		// Handle input mode first
		if m.inputMode {
			switch msg.String() {
			case "enter":
				// Submit input
				return m, m.submitInput()
			case "esc":
				// Cancel input
				m.inputMode = false
				m.textInput.SetValue("")
				m.textInput.Blur()
				return m, nil
			default:
				// Delegate to textinput for editing
				var cmd tea.Cmd
				m.textInput, cmd = m.textInput.Update(msg)
				return m, cmd
			}
		}

		// Normal mode
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			m.focusedSection = (m.focusedSection + 1) % SectionCount
			return m, nil
		case "shift+tab":
			m.focusedSection = (m.focusedSection + SectionCount - 1) % SectionCount
			return m, nil
		case "h", "left":
			return m, m.prevDay()
		case "l", "right":
			return m, m.nextDay()
		case "t":
			return m, m.jumpToToday()
		case "?":
			m.helpMode = !m.helpMode
			return m, nil
		case "i":
			// Add intention
			m.inputMode = true
			m.inputType = "intention"
			m.textInput.Prompt = "Add intention: "
			m.textInput.Placeholder = "What do you intend to accomplish?"
			m.textInput.SetValue("")
			m.textInput.Focus()
			return m, textinput.Blink
		case "w":
			// Add win
			m.inputMode = true
			m.inputType = "win"
			m.textInput.Prompt = "Add win: "
			m.textInput.Placeholder = "What did you accomplish?"
			m.textInput.SetValue("")
			m.textInput.Focus()
			return m, textinput.Blink
		case "L":
			// Add log
			m.inputMode = true
			m.inputType = "log"
			m.textInput.Prompt = "Add log entry: "
			m.textInput.Placeholder = "What did you do?"
			m.textInput.SetValue("")
			m.textInput.Focus()
			return m, textinput.Blink
		case "enter":
			// Handle enter key for toggling intentions
			if m.focusedSection == SectionIntentions && m.intentionList != nil {
				intention := m.intentionList.SelectedIntention()
				if intention != nil {
					return m, m.toggleIntention(intention.ID)
				}
			}
		default:
			// Delegate to focused component
			switch m.focusedSection {
			case SectionIntentions:
				if m.intentionList != nil {
					var cmd tea.Cmd
					m.intentionList, cmd = m.intentionList.Update(msg)
					return m, cmd
				}
			case SectionWins:
				if m.winsView != nil {
					var cmd tea.Cmd
					m.winsView, cmd = m.winsView.Update(msg)
					return m, cmd
				}
			case SectionLogs:
				if m.logView != nil {
					var cmd tea.Cmd
					m.logView, cmd = m.logView.Update(msg)
					return m, cmd
				}
			}
		}
	}

	return m, nil
}

// View renders the TUI
func (m *Model) View() string {
	if m.currentJournal == nil {
		return "Loading..."
	}

	if m.inputMode {
		view := m.textInput.View() + "\n\n(Enter to submit, Esc to cancel)"
		if m.lastError != nil {
			view += "\n\nError: " + m.lastError.Error()
		}
		return view
	}

	if m.helpMode {
		return m.helpView()
	}

	var content string

	switch m.focusedSection {
	case SectionIntentions:
		if m.intentionList != nil {
			content = m.intentionList.View()
		}
	case SectionWins:
		if m.winsView != nil {
			content = m.winsView.View()
		}
	case SectionLogs:
		if m.logView != nil {
			content = m.logView.View()
		}
	}

	status := ""
	if m.statusBar != nil {
		m.statusBar.SetDate(m.currentDate)
		status = m.statusBar.View()
	}

	// Display error if present
	if m.lastError != nil {
		errorMsg := fmt.Sprintf("Error: %s", m.lastError.Error())
		content += "\n\n" + errorMsg
	}

	return content + "\n" + status
}

// helpView renders the help overlay
func (m *Model) helpView() string {
	helpText := `Help - Key Bindings:

Navigation:
  h, ←       Previous day
  l, →       Next day
  t          Jump to today
  tab        Next section
  shift+tab  Previous section

Actions:
  i          Add intention
  w          Add win
  L          Add log entry
  enter      Toggle intention (in intentions section)

Input Mode:
  enter      Submit
  esc        Cancel
  backspace  Delete character
  any key    Add character

General:
  q, ctrl+c  Quit
  ?          Toggle help

Press ? to exit help.`

	status := ""
	if m.statusBar != nil {
		status = m.statusBar.View()
	}

	return helpText + "\n\n" + status
}

// Helper functions for navigation
func (m *Model) prevDay() tea.Cmd {
	date, _ := time.Parse("2006-01-02", m.currentDate)
	newDate := date.AddDate(0, 0, -1).Format("2006-01-02")
	m.currentDate = newDate
	return m.loadJournal()
}

func (m *Model) nextDay() tea.Cmd {
	date, _ := time.Parse("2006-01-02", m.currentDate)
	today := time.Now().Format("2006-01-02")
	newDate := date.AddDate(0, 0, 1).Format("2006-01-02")

	// Don't go beyond today
	if newDate > today {
		return nil
	}

	m.currentDate = newDate
	return m.loadJournal()
}

func (m *Model) jumpToToday() tea.Cmd {
	m.currentDate = time.Now().Format("2006-01-02")
	return m.loadJournal()
}

func (m *Model) toggleIntention(intentionID string) tea.Cmd {
	return func() tea.Msg {
		err := m.service.ToggleIntention(m.currentJournal, intentionID)
		if err != nil {
			return errMsg{err}
		}
		return journalUpdatedMsg{}
	}
}

func (m *Model) submitInput() tea.Cmd {
	inputText := m.textInput.Value()
	if inputText == "" {
		return nil
	}

	return func() tea.Msg {
		var err error
		switch m.inputType {
		case "intention":
			err = m.service.AddIntention(m.currentJournal, inputText)
		case "win":
			err = m.service.AddWin(m.currentJournal, inputText)
		case "log":
			err = m.service.AppendLog(m.currentJournal, inputText)
		}

		if err != nil {
			return errMsg{err}
		}
		return journalUpdatedMsg{}
	}
}

// Messages
type journalLoadedMsg struct {
	journal journal.Journal
}

type journalUpdatedMsg struct{}

type errMsg struct {
	err error
}
