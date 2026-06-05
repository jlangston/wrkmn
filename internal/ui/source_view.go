package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"wrkmn/internal/debug"
	"wrkmn/internal/theme"
)

// SourceView displays source code with breakpoint gutter
type SourceView struct {
	source      *debug.SourceManager
	session     *debug.Session
	breakpoints *debug.BreakpointManager

	// Current state
	scriptID     string
	scriptURL    string
	lines        []debug.SourceLine
	currentLine  int // 0-indexed line of current execution
	scrollOffset int
	cursorLine   int // Line under cursor for navigation

	// Dimensions
	width  int
	height int

	// Focus
	focused bool
}

// NewSourceView creates a new source view
func NewSourceView(session *debug.Session, source *debug.SourceManager, bp *debug.BreakpointManager) *SourceView {
	return &SourceView{
		session:     session,
		source:      source,
		breakpoints: bp,
		cursorLine:  0,
	}
}

// SetSize sets the view dimensions
func (v *SourceView) SetSize(width, height int) {
	v.width = width
	v.height = height
}

// SetFocus sets whether the view is focused
func (v *SourceView) SetFocus(focused bool) {
	v.focused = focused
}

// LoadScript loads a script for display
func (v *SourceView) LoadScript(scriptID, scriptURL string) error {
	v.scriptID = scriptID
	v.scriptURL = scriptURL

	// Get all source lines
	source, err := v.source.GetSource(scriptID)
	if err != nil {
		return err
	}

	rawLines := strings.Split(source, "\n")
	v.lines = make([]debug.SourceLine, len(rawLines))
	for i, content := range rawLines {
		v.lines[i] = debug.SourceLine{
			Number:  i + 1,
			Content: content,
		}
	}

	v.updateBreakpoints()
	return nil
}

// SetCurrentLine sets the current execution line
func (v *SourceView) SetCurrentLine(line int) {
	v.currentLine = line
	v.cursorLine = line

	// Scroll to show current line
	v.scrollToLine(line)
}

// scrollToLine scrolls to ensure a line is visible
func (v *SourceView) scrollToLine(line int) {
	visibleLines := v.height - 2 // Account for borders
	if visibleLines < 1 {
		visibleLines = 1
	}

	// Keep some context around the current line
	margin := visibleLines / 4
	if margin < 2 {
		margin = 2
	}

	if line < v.scrollOffset+margin {
		v.scrollOffset = line - margin
	} else if line >= v.scrollOffset+visibleLines-margin {
		v.scrollOffset = line - visibleLines + margin + 1
	}

	if v.scrollOffset < 0 {
		v.scrollOffset = 0
	}
}

// updateBreakpoints updates breakpoint markers for all lines
func (v *SourceView) updateBreakpoints() {
	bpMap := v.source.GetBreakpointsForFile(v.scriptURL)
	for i := range v.lines {
		v.lines[i].HasBreakpoint = bpMap[i]
	}
}

// Update handles key events
func (v *SourceView) Update(msg tea.Msg) tea.Cmd {
	if !v.focused {
		return nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if v.cursorLine < len(v.lines)-1 {
				v.cursorLine++
				v.scrollToLine(v.cursorLine)
			}
		case "k", "up":
			if v.cursorLine > 0 {
				v.cursorLine--
				v.scrollToLine(v.cursorLine)
			}
		case "g":
			v.cursorLine = 0
			v.scrollOffset = 0
		case "G":
			v.cursorLine = len(v.lines) - 1
			v.scrollToLine(v.cursorLine)
		case "ctrl+d":
			v.cursorLine += v.height / 2
			if v.cursorLine >= len(v.lines) {
				v.cursorLine = len(v.lines) - 1
			}
			v.scrollToLine(v.cursorLine)
		case "ctrl+u":
			v.cursorLine -= v.height / 2
			if v.cursorLine < 0 {
				v.cursorLine = 0
			}
			v.scrollToLine(v.cursorLine)
		case "f9", "b":
			// Toggle breakpoint at cursor line (Bubble Tea reports F9 as "f9")
			return v.toggleBreakpoint()
		}
	}

	return nil
}

// toggleBreakpoint toggles a breakpoint at the cursor line
func (v *SourceView) toggleBreakpoint() tea.Cmd {
	if v.scriptURL == "" {
		return nil
	}

	// Check if there's already a breakpoint
	bps := v.breakpoints.GetBreakpointsForScript(v.scriptURL)
	for _, bp := range bps {
		if bp.LineNumber == v.cursorLine {
			// Remove existing breakpoint
			v.breakpoints.RemoveBreakpoint(bp.ID)
			v.updateBreakpoints()
			return nil
		}
	}

	// Add new breakpoint
	_, err := v.breakpoints.SetBreakpoint(v.scriptURL, v.cursorLine, 0, "")
	if err != nil {
		// Could return an error message
		return nil
	}

	v.updateBreakpoints()
	return nil
}

// View renders the source view
func (v *SourceView) View() string {
	t := theme.Current
	styles := theme.NewStyles(t)

	if len(v.lines) == 0 {
		return v.renderEmpty()
	}

	// Calculate visible lines
	visibleLines := v.height - 2
	if visibleLines < 1 {
		visibleLines = 1
	}

	endLine := v.scrollOffset + visibleLines
	if endLine > len(v.lines) {
		endLine = len(v.lines)
	}

	// Styles
	gutterWidth := 6
	gutterStyle := lipgloss.NewStyle().
		Width(gutterWidth).
		Align(lipgloss.Right).
		Foreground(t.LineNumber)

	activeGutterStyle := gutterStyle.Copy().
		Foreground(t.LineNumberActive).
		Bold(true)

	breakpointStyle := lipgloss.NewStyle().
		Foreground(t.BreakpointActive).
		Bold(true)

	currentLineStyle := lipgloss.NewStyle().
		Background(t.CurrentLine).
		Width(v.width - gutterWidth - 4)

	cursorLineStyle := lipgloss.NewStyle().
		Background(t.BackgroundLight)

	codeStyle := lipgloss.NewStyle().
		Foreground(t.Foreground)

	// Render lines
	var renderedLines []string
	for i := v.scrollOffset; i < endLine; i++ {
		line := v.lines[i]

		// Gutter: breakpoint marker or line number
		var gutter string
		if line.HasBreakpoint {
			gutter = breakpointStyle.Render(" ● ")
		} else {
			gutter = "   "
		}

		// Line number
		var lineNum string
		if i == v.currentLine {
			lineNum = activeGutterStyle.Render(fmt.Sprintf("%d", line.Number))
		} else {
			lineNum = gutterStyle.Render(fmt.Sprintf("%d", line.Number))
		}

		// Code content
		content := line.Content
		if len(content) > v.width-gutterWidth-6 {
			content = content[:v.width-gutterWidth-9] + "..."
		}

		var codePart string
		if i == v.currentLine {
			// Current execution line
			codePart = currentLineStyle.Render("→ " + content)
		} else if i == v.cursorLine && v.focused {
			// Cursor line (when navigating)
			codePart = cursorLineStyle.Render("  " + content)
		} else {
			codePart = codeStyle.Render("  " + content)
		}

		renderedLines = append(renderedLines, gutter+lineNum+" "+codePart)
	}

	// Join lines
	content := strings.Join(renderedLines, "\n")

	// Container
	borderColor := t.Border
	if v.focused {
		borderColor = t.BorderFocused
	}

	container := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(v.width).
		Height(v.height)

	// Header
	header := styles.Title.Render(v.scriptURL)
	if v.scriptURL == "" {
		header = styles.Muted.Render("No source loaded")
	}

	return container.Render(lipgloss.JoinVertical(lipgloss.Left, header, content))
}

// renderEmpty renders an empty state
func (v *SourceView) renderEmpty() string {
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
		Align(lipgloss.Center, lipgloss.Center)

	return container.Render("No source loaded\n\nStart debugging to view source code")
}

// GetCurrentLine returns the current cursor line
func (v *SourceView) GetCurrentLine() int {
	return v.cursorLine
}

// GetScriptID returns the current script ID
func (v *SourceView) GetScriptID() string {
	return v.scriptID
}
