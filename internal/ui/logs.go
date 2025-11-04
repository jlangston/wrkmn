package ui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cf-worker-manager/internal/process"
)

type LogView struct {
	viewport viewport.Model
	process  *process.Process
	width    int
	height   int
}

type LogUpdateMsg struct{}

func NewLogView() *LogView {
	vp := viewport.New(0, 0)
	vp.Style = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62"))

	return &LogView{
		viewport: vp,
	}
}

func (lv *LogView) SetSize(width, height int) {
	lv.width = width
	lv.height = height
	// Account for borders and title/help text
	lv.viewport.Width = width
	lv.viewport.Height = height - 8
	if lv.viewport.Width < 10 {
		lv.viewport.Width = 10
	}
	if lv.viewport.Height < 5 {
		lv.viewport.Height = 5
	}
}

func (lv *LogView) SetProcess(proc *process.Process) {
	lv.process = proc
	lv.updateLogs()
}

func (lv *LogView) updateLogs() {
	if lv.process == nil {
		return
	}

	lines := lv.process.LogBuffer.GetLines()
	content := strings.Join(lines, "\n")
	lv.viewport.SetContent(content)
	lv.viewport.GotoBottom()
}

func (lv *LogView) Update(msg tea.Msg) (*LogView, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case LogUpdateMsg:
		lv.updateLogs()
		return lv, lv.tickCmd()
	case tea.WindowSizeMsg:
		lv.SetSize(msg.Width, msg.Height)
		lv.updateLogs() // Refresh logs after resize
	}

	lv.viewport, cmd = lv.viewport.Update(msg)
	return lv, cmd
}

func (lv *LogView) tickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond*500, func(time.Time) tea.Msg {
		return LogUpdateMsg{}
	})
}

func (lv *LogView) TickCmd() tea.Cmd {
	return lv.tickCmd()
}

func (lv *LogView) View() string {
	if lv.process == nil {
		return "No process selected"
	}

	title := lipgloss.NewStyle().
		Background(lipgloss.Color("62")).
		Foreground(lipgloss.Color("230")).
		Padding(0, 1).
		Width(lv.width).
		Align(lipgloss.Center).
		Render("📄 Logs: " + lv.process.Worker.Name)

	helpText := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Align(lipgloss.Center).
		Width(lv.width).
		Render("↑/↓: scroll • PgUp/PgDn: page • Home/End: top/bottom • q/esc: back • ctrl+c: quit")

	// Create bordered log container
	logContainer := lipgloss.NewStyle().
		Width(lv.width).
		Height(lv.height).
		Render(lipgloss.JoinVertical(
			lipgloss.Left,
			title,
			lv.viewport.View(),
			"",
			helpText,
		))

	return logContainer
}
