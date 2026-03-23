package cli

import (
	"fmt"
	"strings"

	"github.com/MikeBiancalana/reckon/internal/tui/components"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var logHintStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

// logMultilineModel handles the Bubble Tea model for interactive multiline log input.
type logMultilineModel struct {
	editor   *components.TextEditor
	message  string
	canceled bool
	width    int
	height   int
}

func (m logMultilineModel) Init() tea.Cmd {
	return m.editor.Show()
}

func (m logMultilineModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.canceled = true
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.editor.SetSize(msg.Width, msg.Height)
		return m, nil

	case components.TextEditorSubmitMsg:
		m.message = joinLogLines(msg.Text)
		return m, tea.Quit

	case components.TextEditorCancelMsg:
		m.canceled = true
		return m, tea.Quit
	}

	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(msg)
	return m, cmd
}

func (m logMultilineModel) View() string {
	if m.message != "" || m.canceled {
		return ""
	}
	editorView := m.editor.View()
	if editorView == "" {
		return ""
	}
	hint := logHintStyle.Render("Syntax: [meeting:name]  [task:id]  30m  [break]")
	return editorView + "\n" + hint
}

// joinLogLines joins multiline editor content into a single log line.
func joinLogLines(text string) string {
	return strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
}

// runLogMultilineEditor launches the multiline editor and returns the entered message.
func runLogMultilineEditor() (message string, canceled bool, err error) {
	editor := components.NewTextEditor("Add Log Entry")
	editor.SetSize(80, 15)
	m := logMultilineModel{
		editor: editor,
		width:  80,
		height: 24,
	}
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return "", false, fmt.Errorf("interactive input failed: %w", err)
	}
	result, ok := finalModel.(logMultilineModel)
	if !ok {
		return "", false, fmt.Errorf("unexpected model type returned")
	}
	return result.message, result.canceled, nil
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
			// Interactive mode - multiline editor
			var canceled bool
			var interactiveErr error
			message, canceled, interactiveErr = runLogMultilineEditor()
			if interactiveErr != nil {
				return interactiveErr
			}
			if canceled {
				return nil
			}
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
