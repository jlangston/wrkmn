package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"wrkmn/internal/config"
	"wrkmn/internal/debug"
	"wrkmn/internal/process"
	"wrkmn/internal/state"
	"wrkmn/internal/worker"
)

type TableModel struct {
	workers        []*worker.Worker
	manager        *process.Manager
	config         *config.Config
	cursor         int
	width          int
	height         int
	currentView    ViewMode
	logView        *LogView
	scriptPopup    *ScriptPopup
	remoteLogsView *RemoteLogsView
	envSelector    *EnvSelector
	debugView      *DebugView
	debugSession   *debug.Session
	statusMsg      string
	modeSelector   *ModeSelector
	confirm        confirmDialog
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

func NewTableModel(workers []*worker.Worker, cfg *config.Config) TableModel {
	return TableModel{
		workers:        workers,
		manager:        process.NewManager(),
		config:         cfg,
		currentView:    ViewDashboard,
		logView:        NewLogView(),
		scriptPopup:    NewScriptPopup(),
		remoteLogsView: NewRemoteLogsView(),
		envSelector:    NewEnvSelector(),
		modeSelector:   NewModeSelector(),
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
		m.scriptPopup.SetSize(msg.Width, msg.Height)
		m.remoteLogsView.SetSize(msg.Width, msg.Height)
		m.envSelector.SetSize(msg.Width, msg.Height)
		m.modeSelector.SetSize(msg.Width, msg.Height)
		if m.debugView != nil {
			m.debugView.SetSize(msg.Width, msg.Height)
		}
		// Force a redraw after resize
		if m.currentView == ViewLogs {
			return m, m.logView.TickCmd()
		}

	case UpdateMsg:
		return m, m.tickCmd()

	case processStartedMsg:
		// Update worker status based on result
		for _, w := range m.workers {
			if w.Name == msg.workerName {
				if msg.err != nil {
					w.Status = worker.StatusFailed
				} else {
					w.Status = worker.StatusRunning
				}
				break
			}
		}
		return m, nil

	case processStoppedMsg:
		// Update worker status
		for _, w := range m.workers {
			if w.Name == msg.workerName {
				w.Status = worker.StatusStopped
				break
			}
		}
		return m, nil

	case allProcessesStartedMsg:
		// All processes started, nothing else to do
		return m, nil

	case allProcessesStoppedMsg:
		// All processes stopped, update all worker statuses
		for _, w := range m.workers {
			w.Status = worker.StatusStopped
		}
		return m, nil

	case debugConnectedMsg:
		return m.onDebugConnected(msg)

	case debugEventMsg:
		return m.onDebugEvent(msg)

	case debugClosedMsg:
		// Session ended (disconnect or worker exit) — return to the dashboard.
		if m.currentView == ViewDebug {
			return m.exitDebug(), nil
		}
		return m, nil

	case tea.KeyMsg:
		m.statusMsg = "" // clear transient status on any keypress
		// Confirm dialog takes priority
		if m.confirm.Visible() {
			switch msg.String() {
			case "y", "Y":
				onYes := m.confirm.onYes
				m.confirm.Hide()
				if onYes != nil {
					return onYes(m)
				}
				return m, nil
			case "n", "N", "esc", "q":
				m.confirm.Hide()
				return m, nil
			}
			return m, nil
		}

		// Mode selector
		if m.modeSelector.Visible() {
			cmd := m.modeSelector.Update(msg)
			if !m.modeSelector.Visible() {
				m.saveState() // selector closed — persist any selection
			}
			return m, cmd
		}

		// Handle env selector first if visible
		if m.envSelector.Visible() {
			cmd := m.envSelector.Update(msg)
			if !m.envSelector.Visible() {
				m.saveState() // selector closed — persist any selection
			}
			return m, cmd
		}

		// Handle script popup if visible
		if m.scriptPopup.Visible() {
			cmd := m.scriptPopup.Update(msg)
			return m, cmd
		}

		if m.currentView == ViewLogs {
			return m.handleLogsViewKeys(msg)
		}
		if m.currentView == ViewRemoteLogs {
			return m.handleRemoteLogsViewKeys(msg)
		}
		if m.currentView == ViewDebug {
			return m.handleDebugViewKeys(msg)
		}
		return m.handleTableKeys(msg)
	}

	// Handle script popup messages (output, done)
	if m.scriptPopup.Visible() {
		cmd := m.scriptPopup.Update(msg)
		return m, cmd
	}

	// Handle remote logs messages
	if m.currentView == ViewRemoteLogs {
		cmd := m.remoteLogsView.Update(msg)
		return m, cmd
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
		return m.startWorkerGuarded("dev")

	case key.Matches(msg, key.NewBinding(key.WithKeys("D"))):
		if m.cursor < len(m.workers) && !m.workers[m.cursor].SupportsDebugging() {
			m.statusMsg = "Debugging unavailable in full-remote mode (code runs on Cloudflare's edge)"
			return m, nil
		}
		return m.enterDebug()

	case key.Matches(msg, key.NewBinding(key.WithKeys("s"))):
		return m.showScriptPopup()

	case key.Matches(msg, key.NewBinding(key.WithKeys("x"))):
		return m.stopWorker()

	case key.Matches(msg, key.NewBinding(key.WithKeys("a"))):
		return m.startAllWorkersGuarded()

	case key.Matches(msg, key.NewBinding(key.WithKeys("z"))):
		return m.stopAllWorkers()

	case key.Matches(msg, key.NewBinding(key.WithKeys("l", "enter"))):
		return m.showLogs()

	case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
		return m.showRemoteLogs()

	case key.Matches(msg, key.NewBinding(key.WithKeys("e"))):
		return m.showEnvSelector()

	case key.Matches(msg, key.NewBinding(key.WithKeys("m"))):
		return m.showModeSelector()
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

func (m TableModel) handleRemoteLogsViewKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("q", "esc"))):
		m.remoteLogsView.Close()
		m.currentView = ViewDashboard
		return m, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c"))):
		m.remoteLogsView.Close()
		m.manager.StopAll()
		return m, tea.Quit
	}

	cmd := m.remoteLogsView.Update(msg)
	return m, cmd
}

func (m TableModel) startWorker(script string) (TableModel, tea.Cmd) {
	if m.cursor >= len(m.workers) {
		return m, nil
	}

	w := m.workers[m.cursor]
	workerName := w.Name

	// Set status to starting
	w.Status = worker.StatusStarting

	// Check if this is a config-based process (has "_config" script marker)
	if _, isConfig := w.Scripts["_config"]; isConfig && m.config != nil {
		if proc, exists := m.config.Processes[w.Name]; exists {
			procCopy := proc // Copy for closure

			// Resolve env files and merge with inline environment. An
			// explicitly configured env file that's missing fails the start.
			resolvedEnv, envErr := config.ResolveEnvFiles(&procCopy, m.config.Defaults, m.config.BaseDir)
			if envErr != nil {
				w.Status = worker.StatusFailed
				m.statusMsg = fmt.Sprintf("%s: %v", workerName, envErr)
				return m, nil
			}
			if len(resolvedEnv) > 0 {
				procCopy.Environment = resolvedEnv
			}

			return m, func() tea.Msg {
				_, err := m.manager.StartProcess(context.Background(), workerName, &procCopy, w)
				return processStartedMsg{workerName: workerName, err: err}
			}
		}
	}

	// Regular discovered worker with scripts
	return m, func() tea.Msg {
		err := m.manager.StartWorker(w, script)
		return processStartedMsg{workerName: workerName, err: err}
	}
}

func (m TableModel) stopWorker() (TableModel, tea.Cmd) {
	if m.cursor >= len(m.workers) {
		return m, nil
	}

	w := m.workers[m.cursor]
	workerName := w.Name
	isConfig := false
	if _, ok := w.Scripts["_config"]; ok {
		isConfig = true
	}

	return m, func() tea.Msg {
		var err error
		if isConfig {
			err = m.manager.StopProcess(workerName)
		} else {
			err = m.manager.StopWorker(workerName)
		}
		return processStoppedMsg{workerName: workerName, err: err}
	}
}

func (m TableModel) startAllWorkers() (TableModel, tea.Cmd) {
	// Discovered workers (no config entry) start directly; config-based
	// processes go through the scheduler so depends_on ordering, health
	// checks, and oneshot completion hold.
	type discoveredStart struct {
		w      *worker.Worker
		script string
	}

	var discovered []discoveredStart
	configProcs := make(map[string]config.Process)
	workerByName := make(map[string]*worker.Worker)

	for _, w := range m.workers {
		workerByName[w.Name] = w

		if _, isConfig := w.Scripts["_config"]; isConfig && m.config != nil {
			if proc, exists := m.config.Processes[w.Name]; exists {
				configProcs[w.Name] = proc
				if w.Status != worker.StatusRunning {
					w.Status = worker.StatusStarting
				}
				continue
			}
		}

		if w.Status == worker.StatusRunning {
			continue
		}
		script := "dev"
		if !w.HasScript("dev") && w.HasScript("start") {
			script = "start"
		}
		if w.HasScript(script) {
			w.Status = worker.StatusStarting
			discovered = append(discovered, discoveredStart{w: w, script: script})
		}
	}

	if len(discovered) == 0 && len(configProcs) == 0 {
		return m, nil
	}

	graph := process.NewDependencyGraph(configProcs)
	// Pre-mark nodes that are already running so the scheduler skips them but
	// dependents still see them as started/healthy.
	for name := range configProcs {
		if w := workerByName[name]; w != nil && w.Status == worker.StatusRunning {
			graph.UpdateState(name, process.StateRunning)
			graph.UpdateHealth(name, process.HealthHealthy)
			if p, exists := m.manager.GetProcess(name); exists {
				graph.SetProcess(name, p)
			}
		}
	}

	manager := m.manager
	cfg := m.config
	sched := process.NewScheduler(graph, func(ctx context.Context, name string, proc *config.Process) (*process.Process, error) {
		procCopy := *proc
		resolvedEnv, err := config.ResolveEnvFiles(&procCopy, cfg.Defaults, cfg.BaseDir)
		if err != nil {
			return nil, err
		}
		if len(resolvedEnv) > 0 {
			procCopy.Environment = resolvedEnv
		}
		// Use a fresh background context here, not the scheduler's orchestration
		// ctx: StartProcess runs the process under exec.CommandContext on the ctx
		// it's given, and the process's lifetime is owned by the manager, not by
		// start-all — it must keep running after orchestration completes and its
		// ctx is cancelled below.
		return manager.StartProcess(context.Background(), name, &procCopy, workerByName[name])
	})

	// Reflect scheduler events in worker statuses; the 1s tick re-renders.
	eventsDone := make(chan struct{})
	go func() {
		defer close(eventsDone)
		for ev := range sched.Events() {
			w := workerByName[ev.Name]
			if w == nil {
				continue
			}
			switch ev.Type {
			case process.EventStarting, process.EventWaiting:
				w.Status = worker.StatusStarting
			case process.EventStarted, process.EventHealthy:
				w.Status = worker.StatusRunning
			case process.EventCompleted:
				w.Status = worker.StatusStopped // oneshot finished successfully
			case process.EventFailed:
				w.Status = worker.StatusFailed
			}
		}
	}()

	return m, func() tea.Msg {
		for _, ds := range discovered {
			manager.StartWorker(ds.w, ds.script)
		}
		if len(configProcs) > 0 {
			// Health monitors (and other startup-scoped waits) are gated on this
			// ctx — they exist only to hold up dependents during start-all.
			// Nothing consumes graph health after orchestration finishes (the
			// graph itself is discarded), so cancel it here to stop those
			// goroutines instead of leaking one health monitor per health-checked
			// process on every start-all cycle.
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			sched.StartAll(ctx)
		}
		sched.CloseEvents()
		<-eventsDone
		return allProcessesStartedMsg{}
	}
}

func (m TableModel) stopAllWorkers() (TableModel, tea.Cmd) {
	return m, func() tea.Msg {
		m.manager.StopAll()
		return allProcessesStoppedMsg{}
	}
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

func (m TableModel) showScriptPopup() (TableModel, tea.Cmd) {
	if m.cursor < len(m.workers) {
		w := m.workers[m.cursor]
		m.scriptPopup.Show(w)
	}
	return m, nil
}

func (m TableModel) showRemoteLogs() (TableModel, tea.Cmd) {
	if m.cursor < len(m.workers) {
		w := m.workers[m.cursor]

		// Only show for Cloudflare Workers with wrangler config
		if !w.IsCloudflareWorker() {
			return m, nil
		}

		m.currentView = ViewRemoteLogs
		cmd := m.remoteLogsView.Init(w)
		return m, cmd
	}
	return m, nil
}

func (m TableModel) showEnvSelector() (TableModel, tea.Cmd) {
	if m.cursor < len(m.workers) {
		w := m.workers[m.cursor]

		// Only show for workers with multiple environments
		if w.HasEnvironments() {
			m.envSelector.Show(w)
		}
	}
	return m, nil
}

func (m TableModel) showModeSelector() (TableModel, tea.Cmd) {
	if m.cursor < len(m.workers) {
		m.modeSelector.Show(m.workers[m.cursor])
	}
	return m, nil
}

// startWorkerGuarded confirms before starting a full-remote worker, otherwise starts immediately.
func (m TableModel) startWorkerGuarded(script string) (TableModel, tea.Cmd) {
	if m.cursor >= len(m.workers) {
		return m, nil
	}
	w := m.workers[m.cursor]
	if w.EffectiveDevMode() == config.DevModeRemote {
		m.confirm.Show(
			fmt.Sprintf("Start %q in FULL-REMOTE mode? Code runs on Cloudflare's edge and bindings hit real resources. Debugging is unavailable.", w.Name),
			func(mm TableModel) (TableModel, tea.Cmd) { return mm.startWorker(script) },
		)
		return m, nil
	}
	return m.startWorker(script)
}

// startAllWorkersGuarded shows one confirmation listing every worker that would start in full-remote mode.
func (m TableModel) startAllWorkersGuarded() (TableModel, tea.Cmd) {
	var remote []string
	for _, w := range m.workers {
		if w.Status == worker.StatusRunning {
			continue
		}
		if w.EffectiveDevMode() == config.DevModeRemote {
			remote = append(remote, w.Name)
		}
	}
	if len(remote) > 0 {
		m.confirm.Show(
			fmt.Sprintf("These workers will start in FULL-REMOTE mode (code on edge, real resources, no debugging): %s. Continue?", strings.Join(remote, ", ")),
			func(mm TableModel) (TableModel, tea.Cmd) { return mm.startAllWorkers() },
		)
		return m, nil
	}
	return m.startAllWorkers()
}

func (m TableModel) View() string {
	if m.currentView == ViewLogs {
		return m.logView.View()
	}

	if m.currentView == ViewRemoteLogs {
		return m.remoteLogsView.View()
	}

	if m.currentView == ViewDebug && m.debugView != nil {
		return m.debugView.View()
	}

	tableView := m.renderTable()

	// Overlay env selector if visible
	if m.envSelector.Visible() {
		popupView := m.envSelector.View()
		return m.overlayPopup(tableView, popupView)
	}

	// Overlay script popup if visible
	if m.scriptPopup.Visible() {
		popupView := m.scriptPopup.View()
		// Center the popup on screen
		return m.overlayPopup(tableView, popupView)
	}

	if m.modeSelector.Visible() {
		popupView := m.modeSelector.View()
		return m.overlayPopup(tableView, popupView)
	}

	if m.confirm.Visible() {
		popupView := m.confirm.View(m.width)
		return m.overlayPopup(tableView, popupView)
	}

	return tableView
}

// overlayPopup centers a popup over the background
func (m TableModel) overlayPopup(background, popup string) string {
	// Split background into lines
	bgLines := strings.Split(background, "\n")

	// Get popup dimensions using visual width (not byte length)
	popupLines := strings.Split(popup, "\n")
	popupHeight := len(popupLines)
	popupWidth := lipgloss.Width(popup) // Use visual width for centering

	// Calculate center position
	startY := (m.height - popupHeight) / 2
	startX := (m.width - popupWidth) / 2
	if startY < 0 {
		startY = 0
	}
	if startX < 0 {
		startX = 0
	}

	// Overlay popup onto background
	for i, popupLine := range popupLines {
		bgY := startY + i
		if bgY >= 0 && bgY < len(bgLines) {
			bgLine := bgLines[bgY]
			bgLineWidth := lipgloss.Width(bgLine)
			popupLineWidth := lipgloss.Width(popupLine)

			// Build the new line using strings.Builder for efficiency
			var newLine strings.Builder

			// Add left portion (spaces for padding)
			if startX > bgLineWidth {
				// Need more padding than we have background
				newLine.WriteString(bgLine)
				newLine.WriteString(strings.Repeat(" ", startX-bgLineWidth))
			} else if startX > 0 {
				// Take prefix from background (approximate - may not be exact due to ANSI)
				newLine.WriteString(strings.Repeat(" ", startX))
			}

			// Add popup line
			newLine.WriteString(popupLine)

			// Add right portion if background extends beyond popup
			rightStart := startX + popupLineWidth
			if rightStart < bgLineWidth {
				// Approximate by adding spaces
				remaining := bgLineWidth - rightStart
				newLine.WriteString(strings.Repeat(" ", remaining))
			}

			bgLines[bgY] = newLine.String()
		}
	}

	return strings.Join(bgLines, "\n")
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
		Render("j/k: navigate • d: dev • D: debug • m: mode • s: scripts • r: remote logs • e: env • x: stop • a: all • l: logs • q: quit")

	// Add main container with border
	rows := []string{table, "", helpText}
	if m.statusMsg != "" {
		statusLine := lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")).
			Padding(0, 1).
			Render(m.statusMsg)
		rows = append(rows, statusLine)
	}
	mainContent := lipgloss.JoinVertical(lipgloss.Left, rows...)

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

// processStartedMsg is sent when a process start operation completes
type processStartedMsg struct {
	workerName string
	err        error
}

// processStoppedMsg is sent when a process stop operation completes
type processStoppedMsg struct {
	workerName string
	err        error
}

// allProcessesStartedMsg is sent when all processes have been started
type allProcessesStartedMsg struct{}

// allProcessesStoppedMsg is sent when all processes have been stopped
type allProcessesStoppedMsg struct{}

func (m TableModel) tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return UpdateMsg{}
	})
}

func (m TableModel) GetManager() *process.Manager {
	return m.manager
}

// saveState persists runtime selections. Best-effort: the TUI must not break
// because the state file can't be written.
func (m TableModel) saveState() {
	if m.config == nil {
		return
	}
	_ = state.Save(m.config.BaseDir, m.workers)
}
