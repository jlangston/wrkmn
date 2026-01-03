package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"wrkmn/internal/debug"
	"wrkmn/internal/theme"
	"wrkmn/internal/vi"
)

// ConsoleEntry represents an entry in the console
type ConsoleEntry struct {
	Type    string // "input", "output", "error", "info"
	Content string
}

// ConsoleView provides a REPL console with VI mode
type ConsoleView struct {
	session   *debug.Session
	evaluator *debug.Evaluator

	// VI mode input
	input *vi.Model

	// Console history
	entries      []ConsoleEntry
	scrollOffset int

	// Dimensions
	width  int
	height int

	// Focus
	focused bool
}

// NewConsoleView creates a new console view
func NewConsoleView(session *debug.Session, evaluator *debug.Evaluator) *ConsoleView {
	input := vi.NewModel()
	input.SetPrompt("> ")

	return &ConsoleView{
		session:   session,
		evaluator: evaluator,
		input:     input,
		entries:   make([]ConsoleEntry, 0),
	}
}

// SetSize sets the view dimensions
func (v *ConsoleView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.input.SetWidth(width - 4)
}

// SetFocus sets whether the view is focused
func (v *ConsoleView) SetFocus(focused bool) {
	v.focused = focused
}

// AddEntry adds an entry to the console
func (v *ConsoleView) AddEntry(entryType, content string) {
	v.entries = append(v.entries, ConsoleEntry{
		Type:    entryType,
		Content: content,
	})

	// Auto-scroll to bottom
	v.scrollToBottom()
}

// AddOutput adds output to the console
func (v *ConsoleView) AddOutput(content string) {
	v.AddEntry("output", content)
}

// AddError adds an error to the console
func (v *ConsoleView) AddError(content string) {
	v.AddEntry("error", content)
}

// AddInfo adds info to the console
func (v *ConsoleView) AddInfo(content string) {
	v.AddEntry("info", content)
}

// scrollToBottom scrolls to show the latest entries
func (v *ConsoleView) scrollToBottom() {
	outputHeight := v.height - 5 // Account for input and borders
	if outputHeight < 1 {
		outputHeight = 1
	}

	if len(v.entries) > outputHeight {
		v.scrollOffset = len(v.entries) - outputHeight
	}
}

// Clear clears the console
func (v *ConsoleView) Clear() {
	v.entries = nil
	v.scrollOffset = 0
}

// Update handles messages
func (v *ConsoleView) Update(msg tea.Msg) tea.Cmd {
	if !v.focused {
		return nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle scroll keys in output area
		switch msg.String() {
		case "ctrl+l":
			v.Clear()
			return nil
		case "pageup":
			v.scrollOffset -= v.height / 2
			if v.scrollOffset < 0 {
				v.scrollOffset = 0
			}
			return nil
		case "pagedown":
			v.scrollOffset += v.height / 2
			v.scrollToBottom()
			return nil
		}

		// Pass to VI input
		result := v.input.HandleKey(msg)

		switch result.Action {
		case vi.ActionSubmit:
			return v.executeCommand()
		case vi.ActionHistoryPrev, vi.ActionHistoryNext:
			// History navigation already handled by VI model
		}
	}

	return nil
}

// executeCommand executes the current input
func (v *ConsoleView) executeCommand() tea.Cmd {
	expression := v.input.Value()
	if expression == "" {
		return nil
	}

	// Add input to history
	v.input.AddHistory(expression)

	// Add to console
	v.AddEntry("input", "> "+expression)

	// Clear input
	v.input.Reset()

	// Handle special commands
	if strings.HasPrefix(expression, ":") {
		return v.handleCommand(expression[1:])
	}

	// Evaluate expression
	result, err := v.evaluator.Evaluate(expression)
	if err != nil {
		v.AddError(err.Error())
		return nil
	}

	// Format and display result
	output := debug.FormatResult(result)
	if result.ExceptionDetails != nil {
		v.AddError(output)
	} else {
		v.AddOutput(output)
	}

	return nil
}

// handleCommand handles special console commands
func (v *ConsoleView) handleCommand(cmd string) tea.Cmd {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return nil
	}

	switch parts[0] {
	case "clear", "c":
		v.Clear()
	case "help", "h", "?":
		v.AddInfo("Console Commands:")
		v.AddInfo("  :clear, :c   - Clear console")
		v.AddInfo("  :help, :h    - Show help")
		v.AddInfo("")
		v.AddInfo("VI Mode:")
		v.AddInfo("  i, a, A      - Enter insert mode")
		v.AddInfo("  Esc          - Normal mode")
		v.AddInfo("  h, l         - Move cursor")
		v.AddInfo("  w, b         - Word navigation")
		v.AddInfo("  Enter        - Execute")
	default:
		v.AddError(fmt.Sprintf("Unknown command: %s", parts[0]))
	}

	return nil
}

// View renders the console view
func (v *ConsoleView) View() string {
	t := theme.Current
	styles := theme.NewStyles(t)

	// Header
	header := styles.Title.Render("Console")

	// Render output area
	output := v.renderOutput()

	// Render input with mode indicator
	modeStyle := v.getModeStyle()
	modeIndicator := modeStyle.Render(v.input.Mode().String())

	inputLine := v.input.View()

	// Input area with mode indicator
	inputArea := lipgloss.JoinHorizontal(
		lipgloss.Center,
		inputLine,
		"  ",
		modeIndicator,
	)

	// Separator
	separator := lipgloss.NewStyle().
		Foreground(t.Border).
		Render(strings.Repeat("─", v.width-4))

	return v.renderContainer(header, output, separator, inputArea)
}

// renderOutput renders the output area
func (v *ConsoleView) renderOutput() string {
	t := theme.Current

	outputHeight := v.height - 6
	if outputHeight < 1 {
		outputHeight = 1
	}

	// Styles for different entry types
	inputStyle := lipgloss.NewStyle().
		Foreground(t.Primary)

	outputStyle := lipgloss.NewStyle().
		Foreground(t.Foreground)

	errorStyle := lipgloss.NewStyle().
		Foreground(t.Error)

	infoStyle := lipgloss.NewStyle().
		Foreground(t.ForegroundMuted).
		Italic(true)

	// Calculate visible entries
	endIdx := v.scrollOffset + outputHeight
	if endIdx > len(v.entries) {
		endIdx = len(v.entries)
	}

	startIdx := v.scrollOffset
	if startIdx < 0 {
		startIdx = 0
	}

	// Render entries
	var lines []string
	for i := startIdx; i < endIdx; i++ {
		entry := v.entries[i]
		var line string

		switch entry.Type {
		case "input":
			line = inputStyle.Render(entry.Content)
		case "output":
			line = outputStyle.Render(entry.Content)
		case "error":
			line = errorStyle.Render(entry.Content)
		case "info":
			line = infoStyle.Render(entry.Content)
		default:
			line = entry.Content
		}

		// Truncate long lines
		if len(line) > v.width-6 {
			line = line[:v.width-9] + "..."
		}

		lines = append(lines, line)
	}

	// Pad to fill output area
	for len(lines) < outputHeight {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

// getModeStyle returns the style for the current VI mode
func (v *ConsoleView) getModeStyle() lipgloss.Style {
	t := theme.Current

	switch v.input.Mode() {
	case vi.ModeInsert:
		return lipgloss.NewStyle().
			Background(t.ViInsert).
			Foreground(lipgloss.Color("230")).
			Bold(true).
			Padding(0, 1)
	case vi.ModeReplace:
		return lipgloss.NewStyle().
			Background(t.ViReplace).
			Foreground(lipgloss.Color("230")).
			Bold(true).
			Padding(0, 1)
	default:
		return lipgloss.NewStyle().
			Background(t.ViNormal).
			Foreground(lipgloss.Color("230")).
			Bold(true).
			Padding(0, 1)
	}
}

// renderContainer renders the view container
func (v *ConsoleView) renderContainer(header, output, separator, input string) string {
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

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		"",
		output,
		separator,
		input,
	)

	return container.Render(content)
}

// GetInput returns the VI input model
func (v *ConsoleView) GetInput() *vi.Model {
	return v.input
}
