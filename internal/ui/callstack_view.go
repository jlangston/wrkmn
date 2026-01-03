package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"wrkmn/internal/debug"
	"wrkmn/internal/theme"
)

// CallStackView displays the call stack
type CallStackView struct {
	session *debug.Session
	stepper *debug.StepController

	// Current state
	frames       []debug.CallFrame
	selectedIdx  int
	scrollOffset int

	// Dimensions
	width  int
	height int

	// Focus
	focused bool

	// Callback when frame is selected
	onFrameSelect func(frame debug.CallFrame)
}

// NewCallStackView creates a new call stack view
func NewCallStackView(session *debug.Session, stepper *debug.StepController) *CallStackView {
	return &CallStackView{
		session: session,
		stepper: stepper,
	}
}

// SetSize sets the view dimensions
func (v *CallStackView) SetSize(width, height int) {
	v.width = width
	v.height = height
}

// SetFocus sets whether the view is focused
func (v *CallStackView) SetFocus(focused bool) {
	v.focused = focused
}

// SetOnFrameSelect sets the callback for frame selection
func (v *CallStackView) SetOnFrameSelect(fn func(debug.CallFrame)) {
	v.onFrameSelect = fn
}

// Refresh updates the call stack from the session
func (v *CallStackView) Refresh() {
	v.frames = v.stepper.GetCallStack()
	if v.selectedIdx >= len(v.frames) {
		v.selectedIdx = 0
	}
}

// Update handles key events
func (v *CallStackView) Update(msg tea.Msg) tea.Cmd {
	if !v.focused {
		return nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if v.selectedIdx < len(v.frames)-1 {
				v.selectedIdx++
				v.ensureVisible()
			}
		case "k", "up":
			if v.selectedIdx > 0 {
				v.selectedIdx--
				v.ensureVisible()
			}
		case "enter":
			if v.selectedIdx < len(v.frames) && v.onFrameSelect != nil {
				v.onFrameSelect(v.frames[v.selectedIdx])
			}
		case "g":
			v.selectedIdx = 0
			v.scrollOffset = 0
		case "G":
			v.selectedIdx = len(v.frames) - 1
			v.ensureVisible()
		}
	}

	return nil
}

// ensureVisible ensures the selected frame is visible
func (v *CallStackView) ensureVisible() {
	visibleLines := v.height - 4 // Account for borders and header
	if visibleLines < 1 {
		visibleLines = 1
	}

	if v.selectedIdx < v.scrollOffset {
		v.scrollOffset = v.selectedIdx
	} else if v.selectedIdx >= v.scrollOffset+visibleLines {
		v.scrollOffset = v.selectedIdx - visibleLines + 1
	}
}

// View renders the call stack view
func (v *CallStackView) View() string {
	t := theme.Current
	styles := theme.NewStyles(t)

	// Header
	header := styles.Title.Render("Call Stack")

	if len(v.frames) == 0 {
		return v.renderContainer(header, styles.Muted.Render("Not paused"))
	}

	// Calculate visible frames
	visibleLines := v.height - 4
	if visibleLines < 1 {
		visibleLines = 1
	}

	endIdx := v.scrollOffset + visibleLines
	if endIdx > len(v.frames) {
		endIdx = len(v.frames)
	}

	// Styles
	selectedStyle := lipgloss.NewStyle().
		Background(t.Selection).
		Foreground(lipgloss.Color("230")).
		Bold(true).
		Width(v.width - 4)

	normalStyle := lipgloss.NewStyle().
		Foreground(t.Foreground).
		Width(v.width - 4)

	funcStyle := lipgloss.NewStyle().
		Foreground(t.Primary)

	locationStyle := lipgloss.NewStyle().
		Foreground(t.ForegroundMuted)

	arrowStyle := lipgloss.NewStyle().
		Foreground(t.CurrentExecution).
		Bold(true)

	// Render frames
	var lines []string
	for i := v.scrollOffset; i < endIdx; i++ {
		frame := v.frames[i]

		// Format function name
		funcName := frame.FunctionName
		if funcName == "" {
			funcName = "(anonymous)"
		}

		// Format location
		location := ""
		if frame.URL != "" {
			fileName := filepath.Base(frame.URL)
			location = fmt.Sprintf("%s:%d", fileName, frame.Location.LineNumber+1)
		}

		// Build line content
		var content string
		if i == 0 {
			// Current frame gets arrow
			content = arrowStyle.Render("▶ ") + funcStyle.Render(funcName)
		} else {
			content = "  " + funcStyle.Render(funcName)
		}

		if location != "" {
			content += " " + locationStyle.Render(location)
		}

		// Apply selection style
		if i == v.selectedIdx && v.focused {
			content = selectedStyle.Render(content)
		} else {
			content = normalStyle.Render(content)
		}

		lines = append(lines, content)
	}

	content := strings.Join(lines, "\n")
	return v.renderContainer(header, content)
}

// renderContainer renders the view container
func (v *CallStackView) renderContainer(header, content string) string {
	t := theme.Current

	borderColor := t.Border
	if v.focused {
		borderColor = t.BorderFocused
	}

	container := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(v.width).
		Height(v.height).
		Padding(0, 1)

	return container.Render(lipgloss.JoinVertical(lipgloss.Left, header, "", content))
}

// GetSelectedFrame returns the currently selected frame
func (v *CallStackView) GetSelectedFrame() *debug.CallFrame {
	if v.selectedIdx < len(v.frames) {
		return &v.frames[v.selectedIdx]
	}
	return nil
}

// GetSelectedIndex returns the selected index
func (v *CallStackView) GetSelectedIndex() int {
	return v.selectedIdx
}

// SetSelectedIndex sets the selected index
func (v *CallStackView) SetSelectedIndex(idx int) {
	if idx >= 0 && idx < len(v.frames) {
		v.selectedIdx = idx
		v.ensureVisible()
	}
}
