package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"wrkmn/internal/process"
	"wrkmn/internal/worker"
)

type TableModel struct {
	workers     []*worker.Worker
	manager     *process.Manager
	cursor      int
	width       int
	height      int
	currentView ViewMode
	logView     *LogView
}

var (
	headerCellStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("62")).
			Align(lipgloss.Center).
			Padding(0, 1)

	selectedCellStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("62")).
				Foreground(lipgloss.Color("15")).
				Bold(true).
				Padding(0, 1)

	evenCellStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("235")).
			Foreground(lipgloss.Color("15")).
			Padding(0, 1)

	oddCellStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			Padding(0, 1)

	borderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	statusStyles = map[worker.WorkerStatus]lipgloss.Style{
		worker.StatusRunning:  lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true),
		worker.StatusStopped:  lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
		worker.StatusFailed:   lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true),
		worker.StatusStarting: lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true),
	}
)

func NewTableModel(workers []*worker.Worker) TableModel {
	return TableModel{
		workers:     workers,
		manager:     process.NewManager(),
		currentView: ViewDashboard,
		logView:     NewLogView(),
	}
}

func (m TableModel) Init() tea.Cmd {
	return tea.Batch(
		m.tickCmd(),
	)
}

func (m TableModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.logView.SetSize(msg.Width, msg.Height)
		// Force a redraw after resize
		if m.currentView == ViewLogs {
			return m, m.logView.TickCmd()
		}

	case UpdateMsg:
		return m, m.tickCmd()

	case tea.KeyMsg:
		if m.currentView == ViewLogs {
			return m.handleLogsViewKeys(msg)
		}
		return m.handleTableKeys(msg)
	}

	if m.currentView == ViewLogs {
		var cmd tea.Cmd
		m.logView, cmd = m.logView.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m TableModel) handleTableKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("q", "ctrl+c"))):
		// Ensure all processes are properly terminated
		m.manager.StopAll()
		return m, tea.Quit

	case key.Matches(msg, key.NewBinding(key.WithKeys("j", "down"))):
		if m.cursor < len(m.workers)-1 {
			m.cursor++
		}

	case key.Matches(msg, key.NewBinding(key.WithKeys("k", "up"))):
		if m.cursor > 0 {
			m.cursor--
		}

	case key.Matches(msg, key.NewBinding(key.WithKeys("d"))):
		return m.startWorker("dev"), nil

	// case key.Matches(msg, key.NewBinding(key.WithKeys("s"))):
	// 	return m.startWorker("start"), nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("x"))):
		return m.stopWorker(), nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("a"))):
		return m.startAllWorkers(), nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("z"))):
		return m.stopAllWorkers(), nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("l", "enter"))):
		return m.showLogs()

	case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
		return m, nil
	}

	return m, nil
}

func (m TableModel) handleLogsViewKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("q", "esc"))):
		m.currentView = ViewDashboard
		return m, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c"))):
		// Ensure all processes are properly terminated
		m.manager.StopAll()
		return m, tea.Quit
	}

	var cmd tea.Cmd
	m.logView, cmd = m.logView.Update(msg)
	return m, cmd
}

func (m TableModel) startWorker(script string) TableModel {
	if m.cursor < len(m.workers) {
		w := m.workers[m.cursor]
		if err := m.manager.StartWorker(w, script); err != nil {
			w.Status = worker.StatusFailed
		}
	}
	return m
}

func (m TableModel) stopWorker() TableModel {
	if m.cursor < len(m.workers) {
		w := m.workers[m.cursor]
		m.manager.StopWorker(w.Name)
	}
	return m
}

func (m TableModel) startAllWorkers() TableModel {
	for _, w := range m.workers {
		if w.Status != worker.StatusRunning {
			script := "dev"
			if !w.HasScript("dev") && w.HasScript("start") {
				script = "start"
			}
			if w.HasScript(script) {
				m.manager.StartWorker(w, script)
			}
		}
	}
	return m
}

func (m TableModel) stopAllWorkers() TableModel {
	m.manager.StopAll()
	return m
}

func (m TableModel) showLogs() (TableModel, tea.Cmd) {
	if m.cursor < len(m.workers) {
		w := m.workers[m.cursor]
		if process, exists := m.manager.GetProcess(w.Name); exists {
			m.logView.SetProcess(process)
			m.currentView = ViewLogs
			return m, m.logView.TickCmd()
		}
	}
	return m, nil
}

func (m TableModel) View() string {
	if m.currentView == ViewLogs {
		return m.logView.View()
	}

	return m.renderTable()
}

func (m TableModel) renderTable() string {
	if len(m.workers) == 0 {
		return "No workers found"
	}

	nameWidth := 40
	statusWidth := 20
	portWidth := 8
	uptimeWidth := 12
	// Calculate table dimensions
	tableWidth := m.width // Account for main container padding
	if tableWidth < 80 {
		tableWidth = 80 // Minimum table width
	}

	// Calculate path width (remaining space)
	pathWidth := tableWidth - nameWidth - statusWidth - portWidth - uptimeWidth - 7 // 7 for borders and separators
	if pathWidth < 15 {
		pathWidth = 15
	}

	if pathWidth < 20 {
		pathWidth = 20
	}

	// Build the complete table
	table := m.buildTable(m.workers, nameWidth, statusWidth, portWidth, uptimeWidth, pathWidth)

	helpText := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Padding(0, 1).
		Render("j/k: navigate • d: dev • x: stop • a: start all • z: stop all • l: logs • q: quit")

	// Add main container with border
	mainContent := lipgloss.JoinVertical(
		lipgloss.Left,
		table,
		"",
		helpText,
	)

	containerStyle := lipgloss.NewStyle().
		MaxWidth(m.width).
		MaxHeight(m.height)

	return containerStyle.Render(mainContent)
}

func (m TableModel) buildTable(workers []*worker.Worker, nameWidth, statusWidth, portWidth, uptimeWidth, pathWidth int) string {
	var tableRows []string

	// Top border
	topBorder := m.renderTableBorder("top", nameWidth, statusWidth, portWidth, uptimeWidth, pathWidth)
	tableRows = append(tableRows, topBorder)

	// Header row
	headerRow := m.renderHeaderRow(nameWidth, statusWidth, portWidth, uptimeWidth, pathWidth)
	tableRows = append(tableRows, headerRow)

	// Header separator
	headerSep := m.renderTableBorder("middle", nameWidth, statusWidth, portWidth, uptimeWidth, pathWidth)
	tableRows = append(tableRows, headerSep)

	// Data rows
	for i, worker := range workers {
		dataRow := m.renderDataRow(worker, i, nameWidth, statusWidth, portWidth, uptimeWidth, pathWidth)
		tableRows = append(tableRows, dataRow)
	}

	// Bottom border
	bottomBorder := m.renderTableBorder("bottom", nameWidth, statusWidth, portWidth, uptimeWidth, pathWidth)
	tableRows = append(tableRows, bottomBorder)

	return lipgloss.JoinVertical(lipgloss.Left, tableRows...)
}

func (m TableModel) renderTableBorder(position string, nameWidth, statusWidth, portWidth, uptimeWidth, pathWidth int) string {
	var left, right, horizontal, cross string

	switch position {
	case "top":
		left, right, horizontal, cross = "┌", "┐", "─", "┬"
	case "middle":
		left, right, horizontal, cross = "├", "┤", "─", "┼"
	case "bottom":
		left, right, horizontal, cross = "└", "┘", "─", "┴"
	}

	border := left +
		strings.Repeat(horizontal, nameWidth) + cross +
		strings.Repeat(horizontal, statusWidth) + cross +
		strings.Repeat(horizontal, portWidth) + cross +
		strings.Repeat(horizontal, uptimeWidth) + cross +
		strings.Repeat(horizontal, pathWidth) +
		right

	return borderStyle.Render(border)
}

func (m TableModel) renderHeaderRow(nameWidth, statusWidth, portWidth, uptimeWidth, pathWidth int) string {
	// Truncate header text if needed to fit column widths
	nameText := m.truncateText("NAME", nameWidth-2)
	name := headerCellStyle.Width(nameWidth).Align(lipgloss.Center).Render(nameText)

	statusText := m.truncateText("STATUS", statusWidth-2)
	status := headerCellStyle.Width(statusWidth).Align(lipgloss.Center).Render(statusText)

	portText := m.truncateText("PORT", portWidth-2)
	port := headerCellStyle.Width(portWidth).Align(lipgloss.Center).Render(portText)

	uptimeText := m.truncateText("UPTIME", uptimeWidth-2)
	uptime := headerCellStyle.Width(uptimeWidth).Align(lipgloss.Center).Render(uptimeText)

	pathText := m.truncateText("PATH", pathWidth-2)
	path := headerCellStyle.Width(pathWidth).Align(lipgloss.Center).Render(pathText)

	row := borderStyle.Render("│") + name + borderStyle.Render("│") + status + borderStyle.Render("│") + port + borderStyle.Render("│") + uptime + borderStyle.Render("│") + path + borderStyle.Render("│")
	return row
}

func (m TableModel) renderDataRow(w *worker.Worker, index int, nameWidth, statusWidth, portWidth, uptimeWidth, pathWidth int) string {
	var cellStyle lipgloss.Style
	if index == m.cursor {
		cellStyle = selectedCellStyle
	} else if index%2 == 0 {
		cellStyle = evenCellStyle
	} else {
		cellStyle = oddCellStyle
	}

	// Prepare cell contents with proper truncation for all columns
	nameText := m.truncateText(w.Name, nameWidth-5)
	name := cellStyle.Width(nameWidth).Align(lipgloss.Left).Render(" " + nameText)

	statusText := string(w.Status)
	if statusStyle, ok := statusStyles[w.Status]; ok {
		statusText = statusStyle.Render(statusText)
	}
	// Truncate status text if needed
	statusText = m.truncateText(statusText, statusWidth-2)
	status := cellStyle.Width(statusWidth).Align(lipgloss.Center).Render(statusText)

	portText := "-"
	if w.Port > 0 {
		portText = fmt.Sprintf("%d", w.Port)
	}
	// Truncate port text if needed
	portText = m.truncateText(portText, portWidth-2)
	port := cellStyle.Width(portWidth).Align(lipgloss.Center).Render(portText)

	uptimeText := "-"
	if process, exists := m.manager.GetProcess(w.Name); exists {
		duration := time.Since(process.Started)
		uptimeText = m.formatDuration(duration)
	}
	// Truncate uptime text if needed
	uptimeText = m.truncateText(uptimeText, uptimeWidth-2)
	uptime := cellStyle.Width(uptimeWidth).Align(lipgloss.Center).Render(uptimeText)

	pathText := m.truncateText(w.Path, pathWidth-10)
	path := cellStyle.Width(pathWidth).Align(lipgloss.Left).Render(" " + pathText)

	row := borderStyle.Render("│") + name + borderStyle.Render("│") + status + borderStyle.Render("│") + port + borderStyle.Render("│") + uptime + borderStyle.Render("│") + path + borderStyle.Render("│")
	return row
}

func (m TableModel) truncateText(text string, maxWidth int) string {
	if len(text) <= maxWidth {
		return text
	}
	if maxWidth <= 3 {
		return "..."
	}
	return text[:maxWidth-3] + "..."
}

func (m TableModel) formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

type UpdateMsg struct{}

func (m TableModel) tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return UpdateMsg{}
	})
}

func (m TableModel) GetManager() *process.Manager {
	return m.manager
}
