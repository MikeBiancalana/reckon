package components

import (
	"github.com/MikeBiancalana/reckon/internal/time"
	"github.com/charmbracelet/lipgloss"
)

var (
	summaryStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Padding(0, 1)
	summaryLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	summaryValueStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
)

type SummaryView struct {
	summary *time.TimeSummary
	width   int
	visible bool
}

func NewSummaryView() *SummaryView {
	return &SummaryView{
		summary: nil,
		width:   0,
		visible: false,
	}
}

func (sv *SummaryView) View() string {
	if !sv.visible || sv.summary == nil {
		return ""
	}

	s := sv.summary
	content := summaryLabelStyle.Render("Meetings: ") + summaryValueStyle.Render(s.MeetingsFormatted()) + "  " +
		summaryLabelStyle.Render("Tasks: ") + summaryValueStyle.Render(s.TasksFormatted()) + "  " +
		summaryLabelStyle.Render("Breaks: ") + summaryValueStyle.Render(s.BreaksFormatted()) + "  " +
		summaryLabelStyle.Render("Untracked: ") + summaryValueStyle.Render(s.UntrackedFormatted())

	return summaryStyle.Render(content)
}

func (sv *SummaryView) SetSummary(summary *time.TimeSummary) {
	sv.summary = summary
}

func (sv *SummaryView) SetWidth(width int) {
	sv.width = width
}

func (sv *SummaryView) Toggle() {
	sv.visible = !sv.visible
}

func (sv *SummaryView) IsVisible() bool {
	return sv.visible
}

func (sv *SummaryView) SetVisible(visible bool) {
	sv.visible = visible
}
