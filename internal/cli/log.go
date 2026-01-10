package cli

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

// logInputModel handles the Bubble Tea model for interactive log input.
type logInputModel struct {
	textInput textinput.Model
	done      bool
	cancelled bool
}

func initialLogModel() logInputModel {
	ti := textinput.New()
	ti.Placeholder = "Type your log entry..."
	ti.Focus()
	return logInputModel{
		textInput: ti,
		done:      false,
		cancelled: false,
	}
}

func (m logInputModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m logInputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			m.done = true
			return m, tea.Quit
		case tea.KeyEsc:
			m.cancelled = true
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m logInputModel) View() string {
	if m.done || m.cancelled {
		return ""
	}
	return fmt.Sprintf("Add log entry: %s\n\nEnter to submit, Esc to cancel", m.textInput.View())
}

var logCmd = &cobra.Command{
	Use:   "log [message]",
	Short: "Append a log entry to today's journal",
	Long:  `Appends a timestamped log entry to today's journal.`,
	Args:  cobra.RangeArgs(0, -1), // Accepts 0 or more arguments
	RunE: func(cmd *cobra.Command, args []string) error {
		var message string

		if len(args) == 0 {
			// Interactive mode
			p := tea.NewProgram(initialLogModel())
			model, err := p.Run()
			if err != nil {
				return fmt.Errorf("interactive input failed: %w", err)
			}
			m := model.(logInputModel)
			if m.cancelled {
				return nil
			}
			message = strings.TrimSpace(m.textInput.Value())
		} else {
			message = strings.TrimSpace(strings.Join(args, " "))
		}

		if message == "" {
			return fmt.Errorf("log message cannot be empty")
		}

		j, err := service.GetToday()
		if err != nil {
			return fmt.Errorf("failed to get today's journal: %w", err)
		}

		if err := service.AppendLog(j, message); err != nil {
			return fmt.Errorf("failed to append log: %w", err)
		}

		fmt.Printf("✓ Logged: %s\n", message)
		return nil
	},
}
