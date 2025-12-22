package tui

import (
	"fmt"
	"time"

	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/MikeBiancalana/reckon/internal/tui/components"
	tea "github.com/charmbracelet/bubbletea"
)

// Section represents different sections of the journal
type Section int

const (
	SectionIntentions Section = iota
	SectionWins
	SectionLogs
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
	logInput      tea.Model
	statusBar     *components.StatusBar

	// State for input modes
	inputMode bool
	inputType string // "intention", "win", "log"
	inputText string
}

// NewModel creates a new TUI model
func NewModel(service *journal.Service) *Model {
	return &Model{
		service:        service,
		currentDate:    time.Now().Format("2006-01-02"),
		focusedSection: SectionIntentions,
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
		return m, m.loadJournal()

	case errMsg:
		// Handle error - for now just ignore
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
				m.inputText = ""
				return m, nil
			case "backspace":
				if len(m.inputText) > 0 {
					m.inputText = m.inputText[:len(m.inputText)-1]
				}
				return m, nil
			default:
				// Add character to input
				if len(msg.String()) == 1 {
					m.inputText += msg.String()
				}
				return m, nil
			}
		}

		// Normal mode
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			m.focusedSection = (m.focusedSection + 1) % 3
			return m, nil
		case "shift+tab":
			m.focusedSection = (m.focusedSection + 2) % 3
			return m, nil
		case "h", "left":
			return m, m.prevDay()
		case "l", "right":
			return m, m.nextDay()
		case "t":
			return m, m.jumpToToday()
		case "?":
			// Help mode - TODO
			return m, nil
		case "i":
			// Add intention
			m.inputMode = true
			m.inputType = "intention"
			m.inputText = ""
			return m, nil
		case "w":
			// Add win
			m.inputMode = true
			m.inputType = "win"
			m.inputText = ""
			return m, nil
		case "L":
			// Add log
			m.inputMode = true
			m.inputType = "log"
			m.inputText = ""
			return m, nil
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
		prompt := fmt.Sprintf("Add %s: %s", m.inputType, m.inputText)
		return prompt + "\n\n(Enter to submit, Esc to cancel)"
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
		status = m.statusBar.View()
	}

	return content + "\n" + status
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
	if m.inputText == "" {
		m.inputMode = false
		return nil
	}

	return func() tea.Msg {
		var err error
		switch m.inputType {
		case "intention":
			err = m.service.AddIntention(m.currentJournal, m.inputText)
		case "win":
			err = m.service.AddWin(m.currentJournal, m.inputText)
		case "log":
			err = m.service.AppendLog(m.currentJournal, m.inputText)
		}

		m.inputMode = false
		m.inputText = ""

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
