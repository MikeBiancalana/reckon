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
	Use:   "log",
	Short: "Manage journal log entries",
	Long:  `Manage journal log entries - add, list, note, and delete log entries.`,
}

var logAddCmd = &cobra.Command{
	Use:   "add [message]",
	Short: "Append a log entry to the journal",
	Long: `Appends a timestamped log entry to the journal.
Supports interactive mode when no message is provided.`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		var message string

		if len(args) == 0 {
			// Interactive mode - uses TUI, so reconfigure logger
			// Note: The logger has already been initialized in PersistentPreRunE,
			// but this is a lightweight TUI that doesn't need full redirection
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

		effectiveDate, err := getEffectiveDate()
		if err != nil {
			return err
		}

		j, err := journalService.GetByDate(effectiveDate)
		if err != nil {
			return fmt.Errorf("failed to get journal for %s: %w", effectiveDate, err)
		}

		if err := journalService.AppendLog(j, message); err != nil {
			return fmt.Errorf("failed to append log: %w", err)
		}

		if !quietFlag {
			fmt.Printf("✓ Logged: %s\n", message)
		}
		return nil
	},
}

var logNoteCmd = &cobra.Command{
	Use:   "note <log-id> <text>",
	Short: "Add a note to a log entry",
	Long: `Adds a note to an existing log entry.
The log-id can be found using 'rk log list'.`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		logID := args[0]
		noteText := strings.TrimSpace(strings.Join(args[1:], " "))

		effectiveDate, err := getEffectiveDate()
		if err != nil {
			return err
		}

		j, err := journalService.GetByDate(effectiveDate)
		if err != nil {
			return fmt.Errorf("failed to get journal for %s: %w", effectiveDate, err)
		}

		if err := journalService.AddLogNote(j, logID, noteText); err != nil {
			return fmt.Errorf("failed to add note: %w", err)
		}

		fmt.Printf("✓ Note added to log entry %s\n", logID)
		return nil
	},
}

var logDeleteCmd = &cobra.Command{
	Use:   "delete <log-id>",
	Short: "Delete a log entry",
	Long: `Deletes a log entry from the journal.
The log-id can be found using 'rk log list'.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		logID := args[0]

		effectiveDate, err := getEffectiveDate()
		if err != nil {
			return err
		}

		j, err := journalService.GetByDate(effectiveDate)
		if err != nil {
			return fmt.Errorf("failed to get journal for %s: %w", effectiveDate, err)
		}

		if err := journalService.DeleteLogEntry(j, logID); err != nil {
			return fmt.Errorf("failed to delete log entry: %w", err)
		}

		fmt.Printf("✓ Deleted log entry %s\n", logID)
		return nil
	},
}

func init() {
	logCmd.AddCommand(logAddCmd)
	logCmd.AddCommand(logNoteCmd)
	logCmd.AddCommand(logDeleteCmd)
}

func GetLogCommand() *cobra.Command {
	return logCmd
}
