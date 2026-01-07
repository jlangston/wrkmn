package ui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"wrkmn/internal/theme"
	"wrkmn/internal/worker"
)

// ScriptPopup displays available scripts from a worker's package.json
type ScriptPopup struct {
	visible bool
	worker  *worker.Worker
	scripts []string // Sorted script names
	cursor  int
	scroll  int
	width   int
	height  int

	// Running script state
	running    bool
	cmd        *exec.Cmd
	cancel     context.CancelFunc
	output     []string
	outputMu   sync.RWMutex
	outputDone chan struct{}
}

// NewScriptPopup creates a new script popup
func NewScriptPopup() *ScriptPopup {
	return &ScriptPopup{
		output: make([]string, 0, 100),
	}
}

// Show displays the popup for the given worker
func (s *ScriptPopup) Show(w *worker.Worker) {
	s.visible = true
	s.worker = w
	s.cursor = 0
	s.scroll = 0
	s.running = false
	s.output = s.output[:0]

	// Extract and sort script names
	s.scripts = make([]string, 0, len(w.Scripts))
	for name := range w.Scripts {
		// Skip internal markers
		if name == "_config" {
			continue
		}
		s.scripts = append(s.scripts, name)
	}
	sort.Strings(s.scripts)
}

// Hide closes the popup
func (s *ScriptPopup) Hide() {
	s.visible = false
	s.StopScript()
}

// Visible returns whether the popup is visible
func (s *ScriptPopup) Visible() bool {
	return s.visible
}

// SetSize updates the popup dimensions
func (s *ScriptPopup) SetSize(width, height int) {
	s.width = width
	s.height = height
}

// Update handles key events for the popup
func (s *ScriptPopup) Update(msg tea.Msg) tea.Cmd {
	if !s.visible {
		return nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			s.Hide()
			return nil

		case "ctrl+c":
			if s.running {
				s.StopScript()
				return nil
			}
			s.Hide()
			return nil

		case "j", "down":
			if s.running {
				// Scroll output
				s.scroll++
			} else if s.cursor < len(s.scripts)-1 {
				s.cursor++
			}

		case "k", "up":
			if s.running {
				// Scroll output
				if s.scroll > 0 {
					s.scroll--
				}
			} else if s.cursor > 0 {
				s.cursor--
			}

		case "g":
			if s.running {
				s.scroll = 0
			} else {
				s.cursor = 0
			}

		case "G":
			if s.running {
				s.outputMu.RLock()
				s.scroll = max(0, len(s.output)-10)
				s.outputMu.RUnlock()
			} else {
				s.cursor = len(s.scripts) - 1
			}

		case "enter":
			if !s.running && len(s.scripts) > 0 {
				return s.RunScript(s.scripts[s.cursor])
			}
		}

	case scriptOutputMsg:
		s.outputMu.Lock()
		s.output = append(s.output, string(msg))
		// Auto-scroll to bottom
		if len(s.output) > 10 {
			s.scroll = len(s.output) - 10
		}
		s.outputMu.Unlock()
		return s.waitForOutput()

	case scriptDoneMsg:
		s.running = false
		s.outputMu.Lock()
		if msg.err != nil {
			s.output = append(s.output, fmt.Sprintf("[Exit: %v]", msg.err))
		} else {
			s.output = append(s.output, "[Done]")
		}
		s.outputMu.Unlock()
	}

	return nil
}

// scriptOutputMsg represents a line of output from the running script
type scriptOutputMsg string

// scriptDoneMsg indicates the script has finished
type scriptDoneMsg struct {
	err error
}

// RunScript starts a script execution
func (s *ScriptPopup) RunScript(scriptName string) tea.Cmd {
	if s.worker == nil {
		return nil
	}

	s.running = true
	s.output = s.output[:0]
	s.scroll = 0
	s.outputDone = make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	// Use pnpm/npm/bun based on what's available
	s.cmd = exec.CommandContext(ctx, "pnpm", "run", scriptName)
	s.cmd.Dir = s.worker.Path
	s.cmd.Env = os.Environ()
	s.cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Pgid:    0,
	}

	stdout, err := s.cmd.StdoutPipe()
	if err != nil {
		s.output = append(s.output, fmt.Sprintf("Error: %v", err))
		s.running = false
		return nil
	}

	stderr, err := s.cmd.StderrPipe()
	if err != nil {
		s.output = append(s.output, fmt.Sprintf("Error: %v", err))
		s.running = false
		return nil
	}

	if err := s.cmd.Start(); err != nil {
		s.output = append(s.output, fmt.Sprintf("Error: %v", err))
		s.running = false
		return nil
	}

	s.output = append(s.output, fmt.Sprintf("Running: pnpm run %s", scriptName))
	s.output = append(s.output, "")

	// Read output in background
	go s.readOutput(stdout)
	go s.readOutput(stderr)

	// Wait for command to complete
	go func() {
		err := s.cmd.Wait()
		close(s.outputDone)
		// Send done message (will be picked up by waitForOutput returning nil)
		_ = err // Error is captured in the done channel
	}()

	return s.waitForOutput()
}

func (s *ScriptPopup) readOutput(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		s.outputMu.Lock()
		s.output = append(s.output, scanner.Text())
		s.outputMu.Unlock()
	}
}

func (s *ScriptPopup) waitForOutput() tea.Cmd {
	return func() tea.Msg {
		// Small delay to batch output
		time.Sleep(50 * time.Millisecond)

		select {
		case <-s.outputDone:
			var err error
			if s.cmd != nil && s.cmd.ProcessState != nil {
				if !s.cmd.ProcessState.Success() {
					err = fmt.Errorf("exit code %d", s.cmd.ProcessState.ExitCode())
				}
			}
			return scriptDoneMsg{err: err}
		default:
			// Return a tick to keep reading
			return scriptOutputMsg("")
		}
	}
}

// StopScript stops the currently running script
func (s *ScriptPopup) StopScript() {
	if s.cmd != nil && s.cmd.Process != nil {
		// Kill process group
		pgid, err := syscall.Getpgid(s.cmd.Process.Pid)
		if err == nil {
			syscall.Kill(-pgid, syscall.SIGTERM)
			time.Sleep(100 * time.Millisecond)
			syscall.Kill(-pgid, syscall.SIGKILL)
		}
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.running = false
}

// View renders the popup
func (s *ScriptPopup) View() string {
	if !s.visible {
		return ""
	}

	t := theme.Current

	// Calculate dimensions
	overlayWidth := min(80, s.width-4)
	overlayHeight := min(30, s.height-4)
	contentHeight := overlayHeight - 6 // Account for borders, header, footer

	var content string
	var footer string

	if s.running {
		// Show output
		content = s.renderOutput(overlayWidth-4, contentHeight)
		footer = lipgloss.NewStyle().
			Foreground(t.Secondary).
			Render("Ctrl+C: stop • j/k: scroll • q: close")
	} else {
		// Show script list
		content = s.renderScriptList(overlayWidth-4, contentHeight)
		footer = lipgloss.NewStyle().
			Foreground(t.Secondary).
			Render("Enter: run • j/k: navigate • q: close")
	}

	// Header
	workerName := ""
	if s.worker != nil {
		workerName = s.worker.Name
	}
	header := lipgloss.NewStyle().
		Foreground(t.Primary).
		Bold(true).
		Render(fmt.Sprintf("Scripts • %s", workerName))

	// Overlay style
	overlayStyle := lipgloss.NewStyle().
		Width(overlayWidth).
		Height(overlayHeight).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderFocused).
		Padding(1, 2).
		Background(t.Background)

	body := lipgloss.JoinVertical(lipgloss.Left,
		header,
		"",
		content,
		"",
		footer,
	)

	return overlayStyle.Render(body)
}

func (s *ScriptPopup) renderScriptList(width, height int) string {
	if len(s.scripts) == 0 {
		return lipgloss.NewStyle().
			Foreground(theme.Current.Secondary).
			Render("No scripts found in package.json")
	}

	t := theme.Current
	var lines []string

	selectedStyle := lipgloss.NewStyle().
		Background(t.Primary).
		Foreground(t.Background).
		Bold(true).
		Width(width)

	normalStyle := lipgloss.NewStyle().
		Foreground(t.Foreground).
		Width(width)

	for i, script := range s.scripts {
		line := fmt.Sprintf("  %s", script)
		if i == s.cursor {
			line = fmt.Sprintf("> %s", script)
			lines = append(lines, selectedStyle.Render(line))
		} else {
			lines = append(lines, normalStyle.Render(line))
		}

		if len(lines) >= height {
			break
		}
	}

	return strings.Join(lines, "\n")
}

func (s *ScriptPopup) renderOutput(width, height int) string {
	s.outputMu.RLock()
	defer s.outputMu.RUnlock()

	if len(s.output) == 0 {
		return lipgloss.NewStyle().
			Foreground(theme.Current.Secondary).
			Render("Waiting for output...")
	}

	// Apply scroll
	start := s.scroll
	if start >= len(s.output) {
		start = max(0, len(s.output)-1)
	}
	end := start + height
	if end > len(s.output) {
		end = len(s.output)
	}

	visible := s.output[start:end]

	// Truncate long lines
	var lines []string
	for _, line := range visible {
		if len(line) > width {
			line = line[:width-3] + "..."
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

