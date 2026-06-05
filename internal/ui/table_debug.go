package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"wrkmn/internal/debug"
)

// inspectorWaitTimeout bounds how long we wait for a worker's V8 inspector
// target to appear after entering debug mode.
const inspectorWaitTimeout = 10 * time.Second

// debugConnectedMsg reports the result of attaching to a worker's inspector.
type debugConnectedMsg struct {
	worker string
	err    error
}

// debugEventMsg carries one CDP event from the active debug session.
type debugEventMsg struct {
	event debug.DebugEvent
}

// debugClosedMsg signals that the debug session ended (disconnect or worker exit).
type debugClosedMsg struct{}

// enterDebug attaches the in-TUI debugger to the selected worker. It validates
// the worker is a running Cloudflare worker with a known inspector port, builds
// the debug session/components, and kicks off an async connect.
func (m TableModel) enterDebug() (TableModel, tea.Cmd) {
	if m.cursor >= len(m.workers) {
		return m, nil
	}
	w := m.workers[m.cursor]

	if !w.IsCloudflareWorker() {
		m.statusMsg = "Debug: not a Cloudflare worker"
		return m, nil
	}
	if w.InspectorPort == 0 {
		m.statusMsg = "Debug: no inspector_port configured for " + w.Name
		return m, nil
	}
	if _, running := m.manager.GetProcess(w.Name); !running {
		m.statusMsg = "Debug: start " + w.Name + " first"
		return m, nil
	}

	// Build the debug session and its components.
	session := debug.NewSession()
	stepper := debug.NewStepController(session)
	evaluator := debug.NewEvaluator(session)
	breakpoints := debug.NewBreakpointManager(session)
	source := debug.NewSourceManager(session, w.Path)

	view := NewDebugView(session, stepper, evaluator, breakpoints, source)
	view.SetWorkerName(w.Name)
	view.SetSize(m.width, m.height)

	m.debugSession = session
	m.debugView = view
	m.statusMsg = "Debug: attaching to " + w.Name + "..."

	name := w.Name
	port := w.InspectorPort

	return m, func() tea.Msg {
		disc := debug.NewInspectorDiscovery(port)
		target, err := disc.WaitForTarget(inspectorWaitTimeout)
		if err != nil {
			return debugConnectedMsg{worker: name, err: err}
		}
		if err := session.Connect(target); err != nil {
			return debugConnectedMsg{worker: name, err: err}
		}
		return debugConnectedMsg{worker: name, err: nil}
	}
}

// onDebugConnected transitions into the debug view on success, or tears down
// the half-built session and surfaces the error on failure.
func (m TableModel) onDebugConnected(msg debugConnectedMsg) (TableModel, tea.Cmd) {
	if msg.err != nil {
		if m.debugSession != nil {
			m.debugSession.Disconnect()
		}
		m.debugSession = nil
		m.debugView = nil
		m.statusMsg = fmt.Sprintf("Debug: could not attach to %s: %v", msg.worker, msg.err)
		return m, nil
	}

	m.currentView = ViewDebug
	m.statusMsg = ""
	if m.debugView != nil {
		m.debugView.AddConsoleInfo("Attached to " + msg.worker + " inspector")
	}
	return m, listenDebugEvents(m.debugSession)
}

// onDebugEvent forwards a CDP event to the debug view and re-arms the listener.
func (m TableModel) onDebugEvent(msg debugEventMsg) (TableModel, tea.Cmd) {
	if m.debugView == nil || m.debugSession == nil {
		return m, nil
	}

	var viewCmd tea.Cmd
	switch msg.event.Type {
	case debug.EventPaused:
		if pe, ok := msg.event.Data.(debug.PausedEvent); ok {
			viewCmd = m.debugView.Update(pe)
		}
	case debug.EventResumed:
		viewCmd = m.debugView.Update(debug.ResumedEvent{})
	case debug.EventScriptParsed:
		if sc, ok := msg.event.Data.(*debug.Script); ok {
			viewCmd = m.debugView.Update(debug.ScriptParsedEvent{ScriptID: sc.ID, URL: sc.URL})
		}
	case debug.EventConsole:
		if text, isErr := formatConsoleEvent(msg.event.Data); text != "" {
			if isErr {
				m.debugView.AddConsoleError(text)
			} else {
				m.debugView.AddConsoleOutput(text)
			}
		}
	case debug.EventException:
		if text := formatExceptionEvent(msg.event.Data); text != "" {
			m.debugView.AddConsoleError(text)
		}
	}

	return m, tea.Batch(viewCmd, listenDebugEvents(m.debugSession))
}

// handleDebugViewKeys routes key input while in the debug view. Esc leaves the
// debugger, except when the console (VI) panel is focused so its own Esc
// handling (insert → normal mode) keeps working — tab away from it to exit.
func (m TableModel) handleDebugViewKeys(msg tea.KeyMsg) (TableModel, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m = m.exitDebug()
		m.manager.StopAll()
		return m, tea.Quit
	case "esc":
		if m.debugView == nil || m.debugView.GetActivePanel() != PanelConsole {
			return m.exitDebug(), nil
		}
	}

	if m.debugView == nil {
		return m, nil
	}
	cmd := m.debugView.Update(msg)
	return m, cmd
}

// exitDebug disconnects the session and returns to the dashboard.
func (m TableModel) exitDebug() TableModel {
	if m.debugSession != nil {
		m.debugSession.Disconnect()
	}
	m.debugSession = nil
	m.debugView = nil
	m.currentView = ViewDashboard
	return m
}

// listenDebugEvents blocks on the session's event stream, delivering one event
// per command. It unblocks via Done() when the session disconnects so the
// command resolves instead of leaking.
func listenDebugEvents(s *debug.Session) tea.Cmd {
	if s == nil {
		return nil
	}
	return func() tea.Msg {
		select {
		case ev, ok := <-s.Events():
			if !ok {
				return debugClosedMsg{}
			}
			return debugEventMsg{event: ev}
		case <-s.Done():
			return debugClosedMsg{}
		}
	}
}

// formatConsoleEvent renders a Runtime.consoleAPICalled payload into a line of
// text, returning whether it should be styled as an error/warning.
func formatConsoleEvent(data any) (string, bool) {
	params, ok := data.(map[string]any)
	if !ok {
		return "", false
	}

	typ, _ := params["type"].(string)
	var parts []string
	if args, ok := params["args"].([]any); ok {
		for _, a := range args {
			am, ok := a.(map[string]any)
			if !ok {
				continue
			}
			if v, ok := am["value"]; ok {
				parts = append(parts, fmt.Sprintf("%v", v))
			} else if d, ok := am["description"].(string); ok {
				parts = append(parts, d)
			}
		}
	}

	return strings.Join(parts, " "), typ == "error" || typ == "warning" || typ == "assert"
}

// formatExceptionEvent renders a Runtime.exceptionThrown payload into a line.
func formatExceptionEvent(data any) string {
	params, ok := data.(map[string]any)
	if !ok {
		return ""
	}
	if details, ok := params["exceptionDetails"].(map[string]any); ok {
		if text, ok := details["text"].(string); ok {
			if exc, ok := details["exception"].(map[string]any); ok {
				if desc, ok := exc["description"].(string); ok {
					return text + " " + desc
				}
			}
			return text
		}
	}
	return "Uncaught exception"
}
