package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"wrkmn/internal/debug"
	"wrkmn/internal/theme"
)

// DebugPanel represents a focusable panel in the debug view
type DebugPanel int

const (
	PanelSource DebugPanel = iota
	PanelCallStack
	PanelVariables
	PanelConsole
)

// DebugView coordinates the debug mode layout
type DebugView struct {
	// Debug components
	session     *debug.Session
	stepper     *debug.StepController
	evaluator   *debug.Evaluator
	breakpoints *debug.BreakpointManager
	source      *debug.SourceManager

	// UI components
	sourceView    *SourceView
	callStackView *CallStackView
	variablesView *VariablesView
	consoleView   *ConsoleView

	// Layout state
	activePanel DebugPanel
	width       int
	height      int

	// Status
	isPaused   bool
	statusMsg  string
	workerName string
}

// NewDebugView creates a new debug view
func NewDebugView(
	session *debug.Session,
	stepper *debug.StepController,
	evaluator *debug.Evaluator,
	breakpoints *debug.BreakpointManager,
	source *debug.SourceManager,
) *DebugView {
	v := &DebugView{
		session:     session,
		stepper:     stepper,
		evaluator:   evaluator,
		breakpoints: breakpoints,
		source:      source,
		activePanel: PanelSource,
	}

	// Create sub-views
	v.sourceView = NewSourceView(session, source, breakpoints)
	v.callStackView = NewCallStackView(session, stepper)
	v.variablesView = NewVariablesView(session, evaluator)
	v.consoleView = NewConsoleView(session, evaluator)

	// Set up callbacks
	v.callStackView.SetOnFrameSelect(func(frame debug.CallFrame) {
		v.onFrameSelected(frame)
	})

	// Initial focus
	v.updateFocus()

	return v
}

// SetSize sets the view dimensions
func (v *DebugView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.updateLayout()
}

// SetWorkerName sets the worker name for display
func (v *DebugView) SetWorkerName(name string) {
	v.workerName = name
}

// updateLayout recalculates sub-view sizes
func (v *DebugView) updateLayout() {
	// Layout:
	// ┌─────────────────────────────┬──────────────┐
	// │                             │  Call Stack  │
	// │         Source              ├──────────────┤
	// │   (with breakpoint gutter)  │  Variables   │
	// ├─────────────────────────────┴──────────────┤
	// │         Console / REPL (VI mode)           │
	// └────────────────────────────────────────────┘

	// Account for borders and status bar
	contentWidth := v.width - 2
	contentHeight := v.height - 3 // Status bar at bottom

	// Split horizontally: source (70%) | right panel (30%)
	sourceWidth := contentWidth * 70 / 100
	rightWidth := contentWidth - sourceWidth

	// Split vertically: top panels (70%) | console (30%)
	topHeight := contentHeight * 70 / 100
	consoleHeight := contentHeight - topHeight

	// Right panel split: call stack (40%) | variables (60%)
	callStackHeight := topHeight * 40 / 100
	variablesHeight := topHeight - callStackHeight

	// Apply sizes
	v.sourceView.SetSize(sourceWidth, topHeight)
	v.callStackView.SetSize(rightWidth, callStackHeight)
	v.variablesView.SetSize(rightWidth, variablesHeight)
	v.consoleView.SetSize(contentWidth, consoleHeight)
}

// updateFocus updates focus state for all panels
func (v *DebugView) updateFocus() {
	v.sourceView.SetFocus(v.activePanel == PanelSource)
	v.callStackView.SetFocus(v.activePanel == PanelCallStack)
	v.variablesView.SetFocus(v.activePanel == PanelVariables)
	v.consoleView.SetFocus(v.activePanel == PanelConsole)
}

// NextPanel moves focus to the next panel
func (v *DebugView) NextPanel() {
	v.activePanel = (v.activePanel + 1) % 4
	v.updateFocus()
}

// PrevPanel moves focus to the previous panel
func (v *DebugView) PrevPanel() {
	v.activePanel = (v.activePanel + 3) % 4 // +3 is same as -1 mod 4
	v.updateFocus()
}

// FocusPanel focuses a specific panel
func (v *DebugView) FocusPanel(panel DebugPanel) {
	v.activePanel = panel
	v.updateFocus()
}

// Update handles messages
func (v *DebugView) Update(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle global debug keys first
		// Bubble Tea reports function keys lowercase ("f5", "f11", ...). Shifted
		// function keys are not reliably delivered across terminals, so step-out
		// also accepts shift+f11 where available.
		switch msg.String() {
		case "tab":
			v.NextPanel()
			return nil
		case "shift+tab":
			v.PrevPanel()
			return nil
		case "f5":
			return v.handleContinue()
		case "f9":
			// Toggle breakpoint (handled by source view if focused)
			if v.activePanel == PanelSource {
				return v.sourceView.Update(msg)
			}
		case "f10":
			return v.handleStepOver()
		case "f11":
			return v.handleStepInto()
		case "shift+f11", "f12":
			return v.handleStepOut()
		}

		// Pass to active panel
		switch v.activePanel {
		case PanelSource:
			cmd := v.sourceView.Update(msg)
			cmds = append(cmds, cmd)
		case PanelCallStack:
			cmd := v.callStackView.Update(msg)
			cmds = append(cmds, cmd)
		case PanelVariables:
			cmd := v.variablesView.Update(msg)
			cmds = append(cmds, cmd)
		case PanelConsole:
			cmd := v.consoleView.Update(msg)
			cmds = append(cmds, cmd)
		}

	case debug.PausedEvent:
		v.handlePaused(msg)
	case debug.ResumedEvent:
		v.handleResumed()
	case debug.ScriptParsedEvent:
		v.handleScriptParsed(msg)
	}

	return tea.Batch(cmds...)
}

// handleContinue resumes execution
func (v *DebugView) handleContinue() tea.Cmd {
	if !v.isPaused {
		return nil
	}

	err := v.stepper.Resume()
	if err != nil {
		v.consoleView.AddError("Failed to resume: " + err.Error())
	}
	return nil
}

// handleStepOver steps over the current line
func (v *DebugView) handleStepOver() tea.Cmd {
	if !v.isPaused {
		return nil
	}

	err := v.stepper.StepOver()
	if err != nil {
		v.consoleView.AddError("Step over failed: " + err.Error())
	}
	return nil
}

// handleStepInto steps into a function call
func (v *DebugView) handleStepInto() tea.Cmd {
	if !v.isPaused {
		return nil
	}

	err := v.stepper.StepInto()
	if err != nil {
		v.consoleView.AddError("Step into failed: " + err.Error())
	}
	return nil
}

// handleStepOut steps out of the current function
func (v *DebugView) handleStepOut() tea.Cmd {
	if !v.isPaused {
		return nil
	}

	err := v.stepper.StepOut()
	if err != nil {
		v.consoleView.AddError("Step out failed: " + err.Error())
	}
	return nil
}

// handlePaused handles when execution is paused
func (v *DebugView) handlePaused(event debug.PausedEvent) {
	v.isPaused = true

	// Update status
	switch event.Reason {
	case "breakpoint":
		v.statusMsg = "Paused on breakpoint"
	case "exception":
		v.statusMsg = "Paused on exception"
	case "step":
		v.statusMsg = "Stepped"
	default:
		v.statusMsg = "Paused"
	}

	// Refresh views
	v.callStackView.Refresh()
	v.variablesView.Refresh()

	// Load source for current location
	if len(event.CallFrames) > 0 {
		frame := event.CallFrames[0]
		if frame.URL != "" {
			v.sourceView.LoadScript(frame.Location.ScriptID, frame.URL)
			v.sourceView.SetCurrentLine(frame.Location.LineNumber)
		}
	}

	// Add info to console
	v.consoleView.AddInfo(v.statusMsg)
}

// handleResumed handles when execution resumes
func (v *DebugView) handleResumed() {
	v.isPaused = false
	v.statusMsg = "Running"
	v.consoleView.AddInfo("Resumed execution")
}

// handleScriptParsed handles when a new script is loaded
func (v *DebugView) handleScriptParsed(_ debug.ScriptParsedEvent) {
	// Could refresh breakpoints or load source
}

// onFrameSelected handles when a call frame is selected
func (v *DebugView) onFrameSelected(frame debug.CallFrame) {
	// Update source view to show the selected frame
	if frame.URL != "" {
		v.sourceView.LoadScript(frame.Location.ScriptID, frame.URL)
		v.sourceView.SetCurrentLine(frame.Location.LineNumber)
	}

	// Update variables for the selected frame
	v.variablesView.RefreshForFrame(frame)
}

// View renders the debug view
func (v *DebugView) View() string {
	// Render sub-views
	sourceContent := v.sourceView.View()
	callStackContent := v.callStackView.View()
	variablesContent := v.variablesView.View()
	consoleContent := v.consoleView.View()

	// Build right panel (call stack + variables)
	rightPanel := lipgloss.JoinVertical(
		lipgloss.Left,
		callStackContent,
		variablesContent,
	)

	// Build top section (source + right panel)
	topSection := lipgloss.JoinHorizontal(
		lipgloss.Top,
		sourceContent,
		rightPanel,
	)

	// Build main content
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		topSection,
		consoleContent,
	)

	// Build status bar
	statusBar := v.renderStatusBar()

	// Combine everything
	return lipgloss.JoinVertical(
		lipgloss.Left,
		content,
		statusBar,
	)
}

// renderStatusBar renders the debug status bar
func (v *DebugView) renderStatusBar() string {
	t := theme.Current

	statusStyle := lipgloss.NewStyle().
		Background(t.BackgroundLight).
		Foreground(t.Foreground).
		Padding(0, 1).
		Width(v.width)

	// Left side: worker name and status
	var leftParts []string

	if v.workerName != "" {
		workerStyle := lipgloss.NewStyle().
			Foreground(t.Primary).
			Bold(true)
		leftParts = append(leftParts, workerStyle.Render(v.workerName))
	}

	if v.isPaused {
		pausedStyle := lipgloss.NewStyle().
			Background(t.Warning).
			Foreground(lipgloss.Color("230")).
			Padding(0, 1).
			Bold(true)
		leftParts = append(leftParts, pausedStyle.Render("PAUSED"))
	} else {
		runningStyle := lipgloss.NewStyle().
			Background(t.Success).
			Foreground(lipgloss.Color("230")).
			Padding(0, 1).
			Bold(true)
		leftParts = append(leftParts, runningStyle.Render("RUNNING"))
	}

	if v.statusMsg != "" {
		msgStyle := lipgloss.NewStyle().
			Foreground(t.ForegroundMuted)
		leftParts = append(leftParts, msgStyle.Render(v.statusMsg))
	}

	left := strings.Join(leftParts, "  ")

	// Right side: keybindings
	keyStyle := lipgloss.NewStyle().
		Foreground(t.Secondary)
	mutedStyle := lipgloss.NewStyle().
		Foreground(t.ForegroundMuted)

	rightParts := []string{
		keyStyle.Render("[F5]") + mutedStyle.Render("Continue"),
		keyStyle.Render("[F9]") + mutedStyle.Render("Breakpoint"),
		keyStyle.Render("[F10]") + mutedStyle.Render("Over"),
		keyStyle.Render("[F11]") + mutedStyle.Render("Into"),
		keyStyle.Render("[F12]") + mutedStyle.Render("Out"),
		keyStyle.Render("[Tab]") + mutedStyle.Render("Panel"),
		keyStyle.Render("[Esc]") + mutedStyle.Render("Exit"),
	}
	right := strings.Join(rightParts, " ")

	// Calculate padding to right-align
	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	padding := v.width - leftWidth - rightWidth - 4
	if padding < 1 {
		padding = 1
	}

	return statusStyle.Render(left + strings.Repeat(" ", padding) + right)
}

// GetActivePanel returns the active panel
func (v *DebugView) GetActivePanel() DebugPanel {
	return v.activePanel
}

// IsPaused returns whether execution is paused
func (v *DebugView) IsPaused() bool {
	return v.isPaused
}

// AddConsoleOutput adds output to the console
func (v *DebugView) AddConsoleOutput(content string) {
	v.consoleView.AddOutput(content)
}

// AddConsoleError adds an error to the console
func (v *DebugView) AddConsoleError(content string) {
	v.consoleView.AddError(content)
}

// AddConsoleInfo adds info to the console
func (v *DebugView) AddConsoleInfo(content string) {
	v.consoleView.AddInfo(content)
}

// ClearConsole clears the console
func (v *DebugView) ClearConsole() {
	v.consoleView.Clear()
}

// LoadSource loads source for a script
func (v *DebugView) LoadSource(scriptID, scriptURL string) error {
	return v.sourceView.LoadScript(scriptID, scriptURL)
}

// Refresh refreshes all sub-views
func (v *DebugView) Refresh() {
	v.callStackView.Refresh()
	v.variablesView.Refresh()
}
