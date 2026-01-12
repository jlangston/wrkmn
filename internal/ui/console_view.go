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
	completer *debug.Completer

	// VI mode input
	input *vi.Model

	// Autocomplete
	autocomplete *AutocompletePopup

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
		session:      session,
		evaluator:    evaluator,
		completer:    debug.NewCompleter(session, evaluator),
		input:        input,
		autocomplete: NewAutocompletePopup(),
		entries:      make([]ConsoleEntry, 0),
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
		// Handle autocomplete popup first
		if v.autocomplete.Visible() {
			handled, selected := v.autocomplete.Update(msg)
			if handled {
				if selected != nil {
					v.applyCompletion(selected)
				}
				return nil
			}
		}

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

		// Handle Tab for autocomplete in insert mode
		if msg.Type == tea.KeyTab && v.input.Mode() == vi.ModeInsert {
			v.triggerCompletion()
			return nil
		}

		// Hide autocomplete on typing
		if v.autocomplete.Visible() && msg.Type != tea.KeyTab {
			v.autocomplete.Hide()
		}

		// Pass to VI input
		result := v.input.HandleKey(msg)

		switch result.Action {
		case vi.ActionSubmit:
			v.autocomplete.Hide()
			return v.executeCommand()
		case vi.ActionHistoryPrev, vi.ActionHistoryNext:
			// History navigation already handled by VI model
		}
	}

	return nil
}

// triggerCompletion fetches and displays completions
func (v *ConsoleView) triggerCompletion() {
	input := v.input.Value()
	cursorPos := v.input.Cursor()

	completions, err := v.completer.GetCompletions(input, cursorPos)
	if err != nil || len(completions) == 0 {
		v.autocomplete.Hide()
		return
	}

	// If only one completion, apply it directly
	if len(completions) == 1 {
		v.applyCompletion(&completions[0])
		return
	}

	// Show popup above the input line
	// Position is relative to the input area
	x := len(v.input.Value()[:v.input.Cursor()]) + 2 // Account for prompt
	y := 0                                           // Will be positioned in View

	v.autocomplete.Show(completions, x, y)
}

// applyCompletion inserts the selected completion
func (v *ConsoleView) applyCompletion(comp *debug.Completion) {
	input := v.input.Value()
	cursorPos := v.input.Cursor()

	// Find the prefix to replace
	prefix := getCompletionPrefix(input, cursorPos)
	prefixStart := cursorPos - len(prefix)

	// Build new input
	before := input[:prefixStart]
	after := ""
	if cursorPos < len(input) {
		after = input[cursorPos:]
	}

	newInput := before + comp.Text + after
	newCursor := prefixStart + len(comp.Text)

	v.input.SetValue(newInput)
	// Move cursor to end of inserted text
	v.input.SetValue(newInput) // SetValue already puts cursor at end

	// We need to position cursor correctly
	// The VI model's SetValue puts cursor at end, so adjust if needed
	_ = newCursor

	v.autocomplete.Hide()
}

// getCompletionPrefix extracts the prefix being completed
func getCompletionPrefix(input string, cursorPos int) string {
	if cursorPos > len(input) {
		cursorPos = len(input)
	}

	text := input[:cursorPos]

	// Find the start of the current identifier
	start := cursorPos
	for i := cursorPos - 1; i >= 0; i-- {
		r := rune(text[i])
		if isCompletionChar(r) {
			start = i
		} else if r == '.' {
			// Include just the part after the dot
			start = i + 1
			break
		} else {
			break
		}
	}

	return text[start:cursorPos]
}

// isCompletionChar returns true if the rune can be part of an identifier
func isCompletionChar(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '_' || r == '$'
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
		v.AddInfo("Autocomplete:")
		v.AddInfo("  Tab          - Trigger/cycle completions")
		v.AddInfo("  Enter        - Accept completion")
		v.AddInfo("  Esc          - Dismiss completions")
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

	containerView := v.renderContainer(header, output, separator, inputArea)

	// Overlay autocomplete popup if visible
	if v.autocomplete.Visible() {
		containerView = v.overlayAutocomplete(containerView)
	}

	return containerView
}

// overlayAutocomplete renders the autocomplete popup over the console
func (v *ConsoleView) overlayAutocomplete(base string) string {
	popup := v.autocomplete.View()
	if popup == "" {
		return base
	}

	baseLines := strings.Split(base, "\n")
	popupLines := strings.Split(popup, "\n")

	// Safety check: if popup has no lines, return base
	if len(popupLines) == 0 {
		return base
	}

	// Position popup above the input line (last few lines of base)
	// The input is near the bottom, so place popup just above it
	popupY := len(baseLines) - len(popupLines) - 3 // 3 lines from bottom (input, separator, padding)
	if popupY < 2 {
		popupY = 2
	}

	popupX, _ := v.autocomplete.GetPosition()
	// Calculate popup width safely
	popupWidth := lipgloss.Width(popupLines[0])
	maxX := v.width - popupWidth - 4
	if maxX < 2 {
		maxX = 2
	}
	popupX = min(popupX+2, maxX)
	if popupX < 2 {
		popupX = 2
	}

	// Overlay popup onto base
	for i, popupLine := range popupLines {
		lineIdx := popupY + i
		if lineIdx < 0 || lineIdx >= len(baseLines) {
			continue
		}

		baseLine := baseLines[lineIdx]
		baseRunes := []rune(baseLine)

		// Ensure base line is long enough
		for len(baseRunes) < popupX {
			baseRunes = append(baseRunes, ' ')
		}

		// Replace characters with popup content
		popupRunes := []rune(popupLine)
		for j, r := range popupRunes {
			pos := popupX + j
			if pos < len(baseRunes) {
				baseRunes[pos] = r
			} else {
				baseRunes = append(baseRunes, r)
			}
		}

		baseLines[lineIdx] = string(baseRunes)
	}

	return strings.Join(baseLines, "\n")
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
