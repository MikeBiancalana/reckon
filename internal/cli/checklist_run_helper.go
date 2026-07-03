package cli

import (
	"fmt"
	"strings"

	"github.com/MikeBiancalana/reckon/internal/checklist"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	checklistHintStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	checklistCursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
)

// checklistRunModel is the inline Bubble Tea model driving a single active
// checklist run. It follows the same non-alt-screen, synchronous-service-call
// pattern as taskPickerModel and logMultilineModel.
type checklistRunModel struct {
	service   *checklist.Service
	run       *checklist.Run
	cursor    int
	completed bool
	abandoned bool
	canceled  bool
	err       error
}

func (m checklistRunModel) Init() tea.Cmd {
	return nil
}

func (m checklistRunModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch keyMsg.String() {
	case "ctrl+c", "q", "esc":
		m.canceled = true
		return m, tea.Quit

	case "a":
		abandoned, err := m.service.AbandonRun(m.run.TemplateID)
		if err != nil {
			m.err = err
			return m, tea.Quit
		}
		m.run = abandoned
		m.abandoned = true
		return m, tea.Quit

	case "j", "down":
		if len(m.run.Items) > 0 && m.cursor < len(m.run.Items)-1 {
			m.cursor++
		}
		return m, nil

	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil

	case " ", "enter":
		if len(m.run.Items) == 0 {
			return m, nil
		}
		if err := m.service.CheckItem(m.run.ID, m.cursor); err != nil {
			m.err = err
			return m, tea.Quit
		}
		updated, err := m.service.GetRunStatus(m.run.ID)
		if err != nil {
			m.err = err
			return m, tea.Quit
		}
		m.run = updated
		if updated.Status == checklist.RunStatusCompleted {
			m.completed = true
			return m, tea.Quit
		}
		return m, nil
	}

	return m, nil
}

func (m checklistRunModel) View() string {
	if m.canceled || m.abandoned || m.err != nil {
		return ""
	}

	lines := []string{m.run.TemplateName}

	if len(m.run.Items) == 0 {
		lines = append(lines, "(no items)")
	} else {
		for i, item := range m.run.Items {
			mark := "[ ]"
			if item.Checked {
				mark = "[x]"
			}
			prefix := "  "
			if i == m.cursor {
				prefix = checklistCursorStyle.Render(">") + " "
			}
			lines = append(lines, fmt.Sprintf("%s%s %d. %s", prefix, mark, i+1, item.Text))
		}
	}

	if m.completed {
		// Bubble Tea's standard renderer erases the very last rendered line
		// when the program exits (so the shell prompt doesn't land mid-line).
		// Append a trailing blank line so that erasure claims empty space
		// instead of the completion message, keeping it visible in
		// scrollback after the TUI quits.
		lines = append(lines, "✓ Complete!", "")
	} else {
		lines = append(lines, checklistHintStyle.Render("j/k: move  space/enter: toggle  a: abandon  q: quit"))
	}

	return strings.Join(lines, "\n")
}

// resolveChecklistRun resumes the active run for a template if one exists, or
// starts a fresh run otherwise (including after the previous run was
// abandoned or completed). It fails fast if the template doesn't exist.
func resolveChecklistRun(svc *checklist.Service, nameOrID string) (run *checklist.Run, resumed bool, err error) {
	run, err = svc.GetActiveRun(nameOrID)
	if err == nil {
		return run, true, nil
	}

	run, err = svc.StartRun(nameOrID)
	if err != nil {
		return nil, false, err
	}
	return run, false, nil
}

// runChecklistTUI launches the inline checklist run TUI for the given run and
// returns the final model so the caller can branch on its terminal state
// (completed/abandoned/canceled/err) to decide what to print.
func runChecklistTUI(svc *checklist.Service, run *checklist.Run) (checklistRunModel, error) {
	m := checklistRunModel{service: svc, run: run}
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return checklistRunModel{}, fmt.Errorf("interactive checklist failed: %w", err)
	}
	result, ok := finalModel.(checklistRunModel)
	if !ok {
		return checklistRunModel{}, fmt.Errorf("unexpected model type returned")
	}
	return result, nil
}
