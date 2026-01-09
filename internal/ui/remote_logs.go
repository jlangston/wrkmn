package ui

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"wrkmn/internal/cloudflare"
	"wrkmn/internal/theme"
	"wrkmn/internal/worker"
)

// RemoteLogsView displays streaming logs from a deployed Cloudflare Worker
type RemoteLogsView struct {
	worker  *worker.Worker
	client  *cloudflare.Client
	session *cloudflare.TailSession

	// UI state
	width   int
	height  int
	scroll  int
	filter  cloudflare.TailFilter
	showFilter bool

	// Messages
	messages   []cloudflare.TailMessage
	messagesMu sync.RWMutex
	maxMessages int

	// Status
	connected bool
	error     string
	ctx       context.Context
	cancel    context.CancelFunc

	// Environment selector
	envSelector *EnvSelector
}

// NewRemoteLogsView creates a new remote logs view
func NewRemoteLogsView() *RemoteLogsView {
	return &RemoteLogsView{
		messages:    make([]cloudflare.TailMessage, 0, 1000),
		maxMessages: 1000,
		envSelector: NewEnvSelector(),
	}
}

// Init initializes the view with a worker and starts tailing
func (r *RemoteLogsView) Init(w *worker.Worker) tea.Cmd {
	r.worker = w
	r.messages = r.messages[:0]
	r.scroll = 0
	r.error = ""
	r.connected = false

	// Set up environment selector callback
	r.envSelector.SetOnSelect(func(worker *worker.Worker, env string) {
		// Reconnect with new environment
		r.reconnect()
	})

	return r.connect()
}

// SetSize updates the view dimensions
func (r *RemoteLogsView) SetSize(width, height int) {
	r.width = width
	r.height = height
	r.envSelector.SetSize(width, height)
}

// connect establishes a connection to the tail WebSocket
func (r *RemoteLogsView) connect() tea.Cmd {
	return func() tea.Msg {
		// Check if API token is available
		if !cloudflare.IsConfigured() {
			return remoteLogsErrorMsg{err: cloudflare.ErrNoAPIToken}
		}

		// Create client
		client, err := cloudflare.NewClient()
		if err != nil {
			return remoteLogsErrorMsg{err: err}
		}
		r.client = client

		// Get account ID for current environment
		accountID := r.worker.GetCurrentAccountID()
		if accountID == "" {
			return remoteLogsErrorMsg{err: cloudflare.ErrNoAccountID}
		}

		// Create and start tail session
		r.ctx, r.cancel = context.WithCancel(context.Background())
		r.session = cloudflare.NewTailSession(client, accountID, r.worker.Name, r.worker.SelectedEnv)
		r.session.SetFilter(r.filter)

		if err := r.session.Start(r.ctx); err != nil {
			return remoteLogsErrorMsg{err: err}
		}

		return remoteLogsConnectedMsg{}
	}
}

// reconnect stops the current session and reconnects
func (r *RemoteLogsView) reconnect() {
	r.disconnect()
	r.messages = r.messages[:0]
	r.error = ""
	// Note: reconnection will happen via Init or separate command
}

// disconnect stops the tail session
func (r *RemoteLogsView) disconnect() {
	if r.cancel != nil {
		r.cancel()
	}
	if r.session != nil {
		_ = r.session.Stop(context.Background())
		r.session = nil
	}
	r.connected = false
}

// Close cleans up resources
func (r *RemoteLogsView) Close() {
	r.disconnect()
}

// Update handles messages for the remote logs view
func (r *RemoteLogsView) Update(msg tea.Msg) tea.Cmd {
	// Handle environment selector if visible
	if r.envSelector.Visible() {
		cmd := r.envSelector.Update(msg)
		return cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			r.scrollDown()

		case "k", "up":
			r.scrollUp()

		case "g":
			r.scroll = 0

		case "G":
			r.messagesMu.RLock()
			r.scroll = max(0, len(r.messages)-r.visibleLines())
			r.messagesMu.RUnlock()

		case "e":
			// Show environment selector
			if r.worker != nil && r.worker.HasEnvironments() {
				r.envSelector.Show(r.worker)
			}

		case "f":
			// Toggle filter panel
			r.showFilter = !r.showFilter

		case "s":
			// Cycle status filter
			r.cycleStatusFilter()

		case "c":
			// Clear messages
			r.messagesMu.Lock()
			r.messages = r.messages[:0]
			r.scroll = 0
			r.messagesMu.Unlock()

		case "r":
			// Reconnect
			return r.connect()
		}

	case remoteLogsConnectedMsg:
		r.connected = true
		r.error = ""
		return r.listenForMessages()

	case remoteLogsErrorMsg:
		r.connected = false
		r.error = msg.err.Error()

	case remoteLogsMessageMsg:
		r.messagesMu.Lock()
		r.messages = append(r.messages, msg.msg)
		// Trim old messages if over limit
		if len(r.messages) > r.maxMessages {
			r.messages = r.messages[len(r.messages)-r.maxMessages:]
		}
		// Auto-scroll if at bottom
		if r.scroll >= len(r.messages)-r.visibleLines()-2 {
			r.scroll = max(0, len(r.messages)-r.visibleLines())
		}
		r.messagesMu.Unlock()
		return r.listenForMessages()

	case remoteLogsDisconnectedMsg:
		r.connected = false
		if msg.err != nil {
			r.error = msg.err.Error()
		}
	}

	return nil
}

// listenForMessages returns a command that listens for new messages
func (r *RemoteLogsView) listenForMessages() tea.Cmd {
	if r.session == nil {
		return nil
	}

	return func() tea.Msg {
		select {
		case msg, ok := <-r.session.Messages():
			if !ok {
				return remoteLogsDisconnectedMsg{}
			}
			return remoteLogsMessageMsg{msg: msg}

		case err := <-r.session.Errors():
			return remoteLogsDisconnectedMsg{err: err}

		case <-time.After(100 * time.Millisecond):
			// Keep polling
			return remoteLogsTickMsg{}
		}
	}
}

func (r *RemoteLogsView) visibleLines() int {
	return r.height - 8 // Account for header, filters, footer
}

func (r *RemoteLogsView) scrollUp() {
	if r.scroll > 0 {
		r.scroll--
	}
}

func (r *RemoteLogsView) scrollDown() {
	r.messagesMu.RLock()
	maxScroll := max(0, len(r.messages)-r.visibleLines())
	r.messagesMu.RUnlock()
	if r.scroll < maxScroll {
		r.scroll++
	}
}

func (r *RemoteLogsView) cycleStatusFilter() {
	// Cycle through: all -> ok -> error -> all
	if len(r.filter.Status) == 0 {
		r.filter.Status = []string{"ok"}
	} else if len(r.filter.Status) == 1 && r.filter.Status[0] == "ok" {
		r.filter.Status = []string{"error"}
	} else {
		r.filter.Status = nil
	}

	if r.session != nil {
		r.session.SetFilter(r.filter)
	}
}

// View renders the remote logs view
func (r *RemoteLogsView) View() string {
	t := theme.Current

	// Header with worker name and environment
	headerStyle := lipgloss.NewStyle().
		Foreground(t.Primary).
		Bold(true)

	envDisplay := r.worker.SelectedEnv
	if r.worker.HasEnvironments() {
		envDisplay += " ▾"
	}

	statusIndicator := "○"
	statusColor := t.Muted
	if r.connected {
		statusIndicator = "●"
		statusColor = t.Success
	}

	header := lipgloss.JoinHorizontal(lipgloss.Left,
		headerStyle.Render(fmt.Sprintf("Remote Logs: %s ", r.worker.Name)),
		lipgloss.NewStyle().Foreground(t.Secondary).Render(fmt.Sprintf("[%s]", envDisplay)),
		"  ",
		lipgloss.NewStyle().Foreground(statusColor).Render(statusIndicator),
		lipgloss.NewStyle().Foreground(t.Secondary).Render(fmt.Sprintf(" %s", r.connectionStatus())),
	)

	// Filter bar (if shown)
	var filterBar string
	if r.showFilter {
		filterBar = r.renderFilterBar()
	}

	// Main content - log messages
	content := r.renderMessages()

	// Footer with keybindings
	footer := r.renderFooter()

	// Assemble view
	parts := []string{header}
	if filterBar != "" {
		parts = append(parts, filterBar)
	}
	parts = append(parts, "", content, "", footer)

	view := lipgloss.JoinVertical(lipgloss.Left, parts...)

	// Overlay environment selector if visible
	if r.envSelector.Visible() {
		selectorView := r.envSelector.View()
		// Center the selector overlay
		return placeOverlay(r.width, r.height, view, selectorView)
	}

	return view
}

func (r *RemoteLogsView) connectionStatus() string {
	if r.error != "" {
		return "Error"
	}
	if r.connected {
		return "Connected"
	}
	return "Disconnected"
}

func (r *RemoteLogsView) renderFilterBar() string {
	t := theme.Current

	filterStyle := lipgloss.NewStyle().
		Foreground(t.Secondary)

	activeFilterStyle := lipgloss.NewStyle().
		Foreground(t.Primary).
		Bold(true)

	var filters []string

	// Status filter
	statusLabel := "status:all"
	if len(r.filter.Status) > 0 {
		statusLabel = fmt.Sprintf("status:%s", strings.Join(r.filter.Status, ","))
		filters = append(filters, activeFilterStyle.Render(statusLabel))
	} else {
		filters = append(filters, filterStyle.Render(statusLabel))
	}

	// Search filter
	if r.filter.Search != "" {
		filters = append(filters, activeFilterStyle.Render(fmt.Sprintf("search:\"%s\"", r.filter.Search)))
	}

	return lipgloss.NewStyle().
		Foreground(t.Secondary).
		Render("Filters: " + strings.Join(filters, "  "))
}

func (r *RemoteLogsView) renderMessages() string {
	t := theme.Current

	if r.error != "" {
		return lipgloss.NewStyle().
			Foreground(t.Error).
			Render(fmt.Sprintf("Error: %s\n\nPress 'r' to retry", r.error))
	}

	r.messagesMu.RLock()
	defer r.messagesMu.RUnlock()

	if len(r.messages) == 0 {
		return lipgloss.NewStyle().
			Foreground(t.Secondary).
			Render("Waiting for logs...")
	}

	// Calculate visible window
	visibleLines := r.visibleLines()
	start := r.scroll
	end := start + visibleLines
	if end > len(r.messages) {
		end = len(r.messages)
	}

	var lines []string
	for _, msg := range r.messages[start:end] {
		lines = append(lines, r.formatMessage(msg))
	}

	return strings.Join(lines, "\n")
}

func (r *RemoteLogsView) formatMessage(msg cloudflare.TailMessage) string {
	t := theme.Current

	// Timestamp
	ts := msg.Timestamp.Format("15:04:05")
	tsStyle := lipgloss.NewStyle().Foreground(t.Secondary)

	// Outcome badge
	var outcomeStyle lipgloss.Style
	var outcomeText string
	switch msg.Outcome {
	case "ok":
		outcomeStyle = lipgloss.NewStyle().Foreground(t.Success).Bold(true)
		outcomeText = "OK"
	case "exception":
		outcomeStyle = lipgloss.NewStyle().Foreground(t.Error).Bold(true)
		outcomeText = "ERROR"
	case "exceededCpu":
		outcomeStyle = lipgloss.NewStyle().Foreground(t.Warning).Bold(true)
		outcomeText = "CPU"
	default:
		outcomeStyle = lipgloss.NewStyle().Foreground(t.Muted)
		outcomeText = msg.Outcome
	}

	// Request info
	var method, url string
	if msg.Event.Request != nil {
		method = msg.Event.Request.Method
		url = msg.Event.Request.URL
		// Truncate long URLs
		if len(url) > 60 {
			url = url[:57] + "..."
		}
	}

	methodStyle := lipgloss.NewStyle().Foreground(t.Primary).Bold(true)
	urlStyle := lipgloss.NewStyle().Foreground(t.Foreground)

	// CPU time
	cpuTime := ""
	if msg.Diagnostics.CPUTime > 0 {
		cpuTime = lipgloss.NewStyle().
			Foreground(t.Secondary).
			Render(fmt.Sprintf("(%dms)", msg.Diagnostics.CPUTime/1000))
	}

	// Main line
	mainLine := fmt.Sprintf("%s [%s] %s %s %s",
		tsStyle.Render(ts),
		outcomeStyle.Render(outcomeText),
		methodStyle.Render(method),
		urlStyle.Render(url),
		cpuTime,
	)

	lines := []string{mainLine}

	// Add exceptions if any
	if len(msg.Exceptions) > 0 {
		excStyle := lipgloss.NewStyle().Foreground(t.Error)
		for _, exc := range msg.Exceptions {
			lines = append(lines, excStyle.Render(fmt.Sprintf("  Exception: %s: %s", exc.Name, exc.Message)))
		}
	}

	// Add logs if any
	if len(msg.Logs) > 0 {
		for _, log := range msg.Logs {
			logStyle := r.logLevelStyle(log.Level)
			logMsg := strings.Join(log.Message, " ")
			if len(logMsg) > 100 {
				logMsg = logMsg[:97] + "..."
			}
			lines = append(lines, fmt.Sprintf("  %s %s",
				logStyle.Render(fmt.Sprintf("[%s]", log.Level)),
				logMsg,
			))
		}
	}

	return strings.Join(lines, "\n")
}

func (r *RemoteLogsView) logLevelStyle(level string) lipgloss.Style {
	t := theme.Current

	switch level {
	case "error":
		return lipgloss.NewStyle().Foreground(t.Error)
	case "warn":
		return lipgloss.NewStyle().Foreground(t.Warning)
	case "info":
		return lipgloss.NewStyle().Foreground(t.Primary)
	case "debug":
		return lipgloss.NewStyle().Foreground(t.Muted)
	default:
		return lipgloss.NewStyle().Foreground(t.Secondary)
	}
}

func (r *RemoteLogsView) renderFooter() string {
	t := theme.Current

	keyStyle := lipgloss.NewStyle().Foreground(t.Primary).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(t.Secondary)
	sepStyle := lipgloss.NewStyle().Foreground(t.Muted)

	bindings := []string{
		keyStyle.Render("e") + descStyle.Render(":env"),
		keyStyle.Render("f") + descStyle.Render(":filter"),
		keyStyle.Render("s") + descStyle.Render(":status"),
		keyStyle.Render("c") + descStyle.Render(":clear"),
		keyStyle.Render("r") + descStyle.Render(":reconnect"),
		keyStyle.Render("Esc") + descStyle.Render(":back"),
	}

	return strings.Join(bindings, sepStyle.Render(" • "))
}

// placeOverlay centers an overlay on top of a base view
func placeOverlay(width, height int, base, overlay string) string {
	// Simple centering - just place overlay content after base
	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")

	// Calculate centering
	startY := (height - len(overlayLines)) / 2
	startX := (width - lipgloss.Width(overlayLines[0])) / 2

	if startY < 0 {
		startY = 0
	}
	if startX < 0 {
		startX = 0
	}

	// Overlay the content
	for i, line := range overlayLines {
		y := startY + i
		if y < len(baseLines) {
			// Pad with spaces and insert overlay
			baseLine := baseLines[y]
			if startX < len(baseLine) {
				paddedLine := strings.Repeat(" ", startX) + line
				if len(paddedLine) < width {
					paddedLine += strings.Repeat(" ", width-len(paddedLine))
				}
				baseLines[y] = paddedLine
			} else {
				baseLines[y] = strings.Repeat(" ", startX) + line
			}
		}
	}

	return strings.Join(baseLines, "\n")
}

// Message types for remote logs
type remoteLogsConnectedMsg struct{}
type remoteLogsDisconnectedMsg struct {
	err error
}
type remoteLogsErrorMsg struct {
	err error
}
type remoteLogsMessageMsg struct {
	msg cloudflare.TailMessage
}
type remoteLogsTickMsg struct{}
