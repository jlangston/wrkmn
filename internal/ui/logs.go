package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"wrkmn/internal/clipboard"
	"wrkmn/internal/process"
)

// LogMode represents the current mode of the log viewer
type LogMode int

const (
	LogModeNormal LogMode = iota
	LogModeVisual
)

func (m LogMode) String() string {
	switch m {
	case LogModeVisual:
		return "VISUAL"
	default:
		return "NORMAL"
	}
}

type LogView struct {
	viewport     viewport.Model
	process      *process.Process
	width        int
	height       int
	lines        []string      // Cached log lines
	follow       bool          // Auto-scroll to bottom (tail mode)
	mode         LogMode       // Current mode
	cursorLine   int           // Current cursor line (0-indexed)
	visualStart  int           // Start of visual selection
	yankedText   string        // Last yanked text
	yankMessage  string        // Feedback message for yank
	yankMsgTimer int           // Countdown for yank message display
}

type LogUpdateMsg struct{}

func NewLogView() *LogView {
	vp := viewport.New(0, 0)
	vp.Style = lipgloss.NewStyle()

	return &LogView{
		viewport: vp,
		follow:   true, // Start with follow mode enabled
		mode:     LogModeNormal,
	}
}

func (lv *LogView) SetSize(width, height int) {
	lv.width = width
	lv.height = height
	// Account for title, mode line, and help text
	lv.viewport.Width = width
	lv.viewport.Height = height - 6
	if lv.viewport.Width < 10 {
		lv.viewport.Width = 10
	}
	if lv.viewport.Height < 5 {
		lv.viewport.Height = 5
	}
}

func (lv *LogView) SetProcess(proc *process.Process) {
	lv.process = proc
	lv.cursorLine = 0
	lv.visualStart = 0
	lv.mode = LogModeNormal
	lv.follow = true
	lv.updateLogs()
	// Position cursor at the last line
	if len(lv.lines) > 0 {
		lv.cursorLine = len(lv.lines) - 1
	}
}

func (lv *LogView) updateLogs() {
	if lv.process == nil {
		return
	}

	lv.lines = lv.process.LogBuffer.GetLines()
	lv.renderContent()

	// Auto-scroll if in follow mode
	if lv.follow {
		lv.viewport.GotoBottom()
		if len(lv.lines) > 0 {
			lv.cursorLine = len(lv.lines) - 1
		}
	}
}

func (lv *LogView) renderContent() {
	if len(lv.lines) == 0 {
		lv.viewport.SetContent("No logs yet...")
		return
	}

	var rendered []string
	for i, line := range lv.lines {
		prefix := "  "
		lineStyle := lipgloss.NewStyle()

		// Check if this line is selected in visual mode
		if lv.mode == LogModeVisual {
			start, end := lv.getSelectionRange()
			if i >= start && i <= end {
				lineStyle = lineStyle.Background(lipgloss.Color("62")).Foreground(lipgloss.Color("230"))
				prefix = "█ "
			}
		} else if i == lv.cursorLine {
			// Highlight current cursor line in normal mode
			lineStyle = lineStyle.Background(lipgloss.Color("236"))
			prefix = "▸ "
		}

		// Truncate long lines to viewport width
		displayLine := line
		maxLen := lv.viewport.Width - 4
		if maxLen > 0 && len(displayLine) > maxLen {
			displayLine = displayLine[:maxLen-1] + "…"
		}

		rendered = append(rendered, lineStyle.Render(prefix+displayLine))
	}

	lv.viewport.SetContent(strings.Join(rendered, "\n"))
}

func (lv *LogView) getSelectionRange() (int, int) {
	start, end := lv.visualStart, lv.cursorLine
	if start > end {
		start, end = end, start
	}
	return start, end
}

func (lv *LogView) getSelectedText() string {
	if len(lv.lines) == 0 {
		return ""
	}
	start, end := lv.getSelectionRange()
	if start < 0 {
		start = 0
	}
	if end >= len(lv.lines) {
		end = len(lv.lines) - 1
	}
	return strings.Join(lv.lines[start:end+1], "\n")
}

func (lv *LogView) Update(msg tea.Msg) (*LogView, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case LogUpdateMsg:
		lv.updateLogs()
		// Decrement yank message timer
		if lv.yankMsgTimer > 0 {
			lv.yankMsgTimer--
			if lv.yankMsgTimer == 0 {
				lv.yankMessage = ""
			}
		}
		return lv, lv.tickCmd()

	case tea.WindowSizeMsg:
		lv.SetSize(msg.Width, msg.Height)
		lv.renderContent()

	case tea.KeyMsg:
		return lv.handleKey(msg)
	}

	lv.viewport, cmd = lv.viewport.Update(msg)
	return lv, cmd
}

func (lv *LogView) handleKey(msg tea.KeyMsg) (*LogView, tea.Cmd) {
	// Handle mode-specific keys
	switch lv.mode {
	case LogModeVisual:
		return lv.handleVisualKey(msg)
	default:
		return lv.handleNormalKey(msg)
	}
}

func (lv *LogView) handleNormalKey(msg tea.KeyMsg) (*LogView, tea.Cmd) {
	switch msg.String() {
	// Navigation
	case "j", "down":
		lv.moveCursor(1)
		lv.follow = false
	case "k", "up":
		lv.moveCursor(-1)
		lv.follow = false
	case "ctrl+d":
		lv.moveCursor(lv.viewport.Height / 2)
		lv.follow = false
	case "ctrl+u":
		lv.moveCursor(-lv.viewport.Height / 2)
		lv.follow = false
	case "g":
		// Go to top (gg would need state tracking, using single g for simplicity)
		lv.cursorLine = 0
		lv.follow = false
		lv.scrollToCursor()
	case "G":
		// Go to bottom
		if len(lv.lines) > 0 {
			lv.cursorLine = len(lv.lines) - 1
		}
		lv.follow = true
		lv.scrollToCursor()
	case "home":
		lv.cursorLine = 0
		lv.follow = false
		lv.scrollToCursor()
	case "end":
		if len(lv.lines) > 0 {
			lv.cursorLine = len(lv.lines) - 1
		}
		lv.follow = true
		lv.scrollToCursor()

	// Follow mode toggle
	case "f", "F":
		lv.follow = !lv.follow
		if lv.follow && len(lv.lines) > 0 {
			lv.cursorLine = len(lv.lines) - 1
			lv.viewport.GotoBottom()
		}

	// Enter visual mode
	case "v", "V":
		lv.mode = LogModeVisual
		lv.visualStart = lv.cursorLine

	// Yank current line
	case "y":
		if len(lv.lines) > 0 && lv.cursorLine >= 0 && lv.cursorLine < len(lv.lines) {
			lv.yankText(lv.lines[lv.cursorLine])
		}

	// Page navigation (pass to viewport)
	case "pgup":
		lv.follow = false
		lv.viewport.ViewUp()
		lv.updateCursorFromViewport()
	case "pgdown":
		lv.viewport.ViewDown()
		lv.updateCursorFromViewport()
		// Check if at bottom to re-enable follow
		if lv.viewport.AtBottom() {
			lv.follow = true
		}
	}

	lv.renderContent()
	return lv, nil
}

func (lv *LogView) handleVisualKey(msg tea.KeyMsg) (*LogView, tea.Cmd) {
	switch msg.String() {
	// Navigation (extends selection)
	case "j", "down":
		lv.moveCursor(1)
	case "k", "up":
		lv.moveCursor(-1)
	case "ctrl+d":
		lv.moveCursor(lv.viewport.Height / 2)
	case "ctrl+u":
		lv.moveCursor(-lv.viewport.Height / 2)
	case "g":
		lv.cursorLine = 0
		lv.scrollToCursor()
	case "G":
		if len(lv.lines) > 0 {
			lv.cursorLine = len(lv.lines) - 1
		}
		lv.scrollToCursor()

	// Yank selection
	case "y":
		text := lv.getSelectedText()
		lv.yankText(text)
		lv.mode = LogModeNormal

	// Exit visual mode
	case "esc", "v", "V":
		lv.mode = LogModeNormal
	}

	lv.renderContent()
	return lv, nil
}

func (lv *LogView) moveCursor(delta int) {
	lv.cursorLine += delta
	if lv.cursorLine < 0 {
		lv.cursorLine = 0
	}
	if len(lv.lines) > 0 && lv.cursorLine >= len(lv.lines) {
		lv.cursorLine = len(lv.lines) - 1
	}
	lv.scrollToCursor()
}

func (lv *LogView) scrollToCursor() {
	// Calculate which line is at the top of the viewport
	topLine := lv.viewport.YOffset
	bottomLine := topLine + lv.viewport.Height - 1

	if lv.cursorLine < topLine {
		// Cursor above viewport, scroll up
		lv.viewport.SetYOffset(lv.cursorLine)
	} else if lv.cursorLine > bottomLine {
		// Cursor below viewport, scroll down
		lv.viewport.SetYOffset(lv.cursorLine - lv.viewport.Height + 1)
	}
}

func (lv *LogView) updateCursorFromViewport() {
	// After viewport scroll, update cursor to be within visible range
	topLine := lv.viewport.YOffset
	bottomLine := topLine + lv.viewport.Height - 1

	if lv.cursorLine < topLine {
		lv.cursorLine = topLine
	} else if lv.cursorLine > bottomLine {
		lv.cursorLine = bottomLine
	}
}

func (lv *LogView) yankText(text string) {
	lv.yankedText = text
	lineCount := strings.Count(text, "\n") + 1

	// Copy to the system clipboard via OSC 52 (works over SSH/tmux). This is
	// fire-and-forget: the terminal does not acknowledge it, so there's no
	// failure to report.
	clipboard.Copy(text)
	lv.yankMessage = fmt.Sprintf("Yanked %d line(s) to clipboard", lineCount)
	lv.yankMsgTimer = 4 // Show message for ~2 seconds (4 ticks at 500ms)
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

	// Get process name (handle nil Worker)
	processName := lv.process.Name
	if lv.process.Worker != nil {
		processName = lv.process.Worker.Name
	}
	if processName == "" {
		processName = "unknown"
	}

	// Title bar
	title := lipgloss.NewStyle().
		Background(lipgloss.Color("62")).
		Foreground(lipgloss.Color("230")).
		Padding(0, 1).
		Width(lv.width).
		Align(lipgloss.Center).
		Render("Logs: " + processName)

	// Mode/status line
	modeStyle := lipgloss.NewStyle().Bold(true)
	if lv.mode == LogModeVisual {
		modeStyle = modeStyle.Foreground(lipgloss.Color("212")) // Pink for visual
	} else {
		modeStyle = modeStyle.Foreground(lipgloss.Color("75")) // Blue for normal
	}

	followIndicator := ""
	if lv.follow {
		followIndicator = lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")).
			Bold(true).
			Render(" [FOLLOW]")
	}

	lineInfo := ""
	if len(lv.lines) > 0 {
		lineInfo = fmt.Sprintf(" %d/%d", lv.cursorLine+1, len(lv.lines))
	}

	yankMsg := ""
	if lv.yankMessage != "" {
		yankMsg = lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")).
			Render(" │ " + lv.yankMessage)
	}

	statusLine := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Width(lv.width).
		Render(modeStyle.Render("-- "+lv.mode.String()+" --") + followIndicator + lineInfo + yankMsg)

	// Help text based on mode
	var helpText string
	if lv.mode == LogModeVisual {
		helpText = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Align(lipgloss.Center).
			Width(lv.width).
			Render("j/k: extend selection • y: yank • esc: cancel • q: back")
	} else {
		helpText = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Align(lipgloss.Center).
			Width(lv.width).
			Render("j/k: navigate • v: visual mode • y: yank line • f: toggle follow • g/G: top/bottom • q/esc: back")
	}

	// Create log container
	logContainer := lipgloss.NewStyle().
		Width(lv.width).
		Height(lv.height).
		Render(lipgloss.JoinVertical(
			lipgloss.Left,
			title,
			statusLine,
			lv.viewport.View(),
			"",
			helpText,
		))

	return logContainer
}
