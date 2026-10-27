package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"wrkmn/internal/config"
	"wrkmn/internal/control"
	"wrkmn/internal/worker"
)

// ControlRequestMsg carries one `wrkmn ctl` request into the model. The
// control socket server sends it with p.Send and blocks on Reply; the model
// answers from inside Update, after routing the request through the same
// handler the equivalent keybinding uses (d/x/a/z). The Reply channel is
// buffered, so a server that has already given up never blocks the model.
type ControlRequestMsg struct {
	Request control.Request
	Reply   chan<- control.Response
}

// controlDoneMsg wraps the completion message of an asynchronous handler
// (processStartedMsg, allProcessesStoppedMsg, ...) so the reply is built
// from the model *after* that message has been applied — the same ordering
// the keyboard path sees. `next`, when set, re-dispatches a follow-up
// request on the same reply channel (restart = stop, then start).
type controlDoneMsg struct {
	inner tea.Msg
	req   control.Request
	reply chan<- control.Response
	next  *control.Request
}

func sendReply(reply chan<- control.Response, resp control.Response) {
	if reply == nil {
		return
	}
	select {
	case reply <- resp:
	default:
	}
}

// handleControl is the Update case for ControlRequestMsg.
func (m TableModel) handleControl(msg ControlRequestMsg) (TableModel, tea.Cmd) {
	req := msg.Request
	if err := req.Validate(); err != nil {
		sendReply(msg.Reply, control.Errorf("%v", err))
		return m, nil
	}

	switch req.Cmd {
	case control.CmdStatus:
		sendReply(msg.Reply, control.NewResponse(m.statusResult()))
		return m, nil

	case control.CmdLogs:
		idx := m.indexOfWorker(req.Name)
		if idx < 0 {
			sendReply(msg.Reply, control.Errorf("no process named %q", req.Name))
			return m, nil
		}
		p, ok := m.manager.GetProcess(req.Name)
		if !ok {
			sendReply(msg.Reply, control.Errorf("%s has not been started; no logs captured", req.Name))
			return m, nil
		}
		lines := p.LogBuffer.GetLines()
		if n := req.TailOrDefault(); len(lines) > n {
			lines = lines[len(lines)-n:]
		}
		sendReply(msg.Reply, control.NewResponse(control.LogsResult{Name: req.Name, Lines: lines}))
		return m, nil

	case control.CmdStart, control.CmdStop, control.CmdRestart:
		idx := m.indexOfWorker(req.Name)
		if idx < 0 {
			sendReply(msg.Reply, control.Errorf("no process named %q", req.Name))
			return m, nil
		}
		var cmd tea.Cmd
		var next *control.Request
		switch req.Cmd {
		case control.CmdStart:
			m, cmd = m.startWorkerAt(idx) // same body as the `d` key
		case control.CmdStop:
			m, cmd = m.stopWorkerAt(idx) // same body as the `x` key
		case control.CmdRestart:
			// x then d, chained through controlDoneMsg.next so the start only
			// begins once the stop's completion message has been applied.
			m, cmd = m.stopWorkerAt(idx)
			next = &control.Request{Cmd: control.CmdStart, Name: req.Name}
		}
		m = m.autoConfirm()
		if cmd == nil {
			// The handler bailed synchronously (e.g. env-file resolution
			// failed and set statusMsg); report what it left behind.
			if next != nil {
				return m.handleControl(ControlRequestMsg{Request: *next, Reply: msg.Reply})
			}
			sendReply(msg.Reply, m.actionResponse(req, nil))
			return m, nil
		}
		return m, wrapControl(cmd, req, msg.Reply, next)

	case control.CmdStartAll:
		var cmd tea.Cmd
		m, cmd = m.startAllWorkers() // same body as the `a` key
		m = m.autoConfirm()
		if cmd == nil {
			sendReply(msg.Reply, control.NewResponse(m.statusResult()))
			return m, nil
		}
		return m, wrapControl(cmd, req, msg.Reply, nil)

	case control.CmdStopAll:
		var cmd tea.Cmd
		m, cmd = m.stopAllWorkers() // same body as the `z` key
		m = m.autoConfirm()
		if cmd == nil {
			sendReply(msg.Reply, control.NewResponse(m.statusResult()))
			return m, nil
		}
		return m, wrapControl(cmd, req, msg.Reply, nil)
	}

	sendReply(msg.Reply, control.Errorf("unknown cmd %q", req.Cmd))
	return m, nil
}

// wrapControl runs the handler's command and tags its completion message so
// Update can reply once the message has been applied.
func wrapControl(cmd tea.Cmd, req control.Request, reply chan<- control.Response, next *control.Request) tea.Cmd {
	return func() tea.Msg {
		return controlDoneMsg{inner: cmd(), req: req, reply: reply, next: next}
	}
}

// handleControlDone is the Update case for controlDoneMsg: apply the inner
// completion message exactly as the keyboard path would, then answer (or
// chain the next request).
func (m TableModel) handleControlDone(msg controlDoneMsg) (TableModel, tea.Cmd) {
	var cmd tea.Cmd
	if msg.inner != nil {
		var model tea.Model
		model, cmd = m.Update(msg.inner)
		m = model.(TableModel)
	}
	if msg.next != nil {
		// A restart's stop half may legitimately fail ("not running"); the
		// start half still proceeds, as a user pressing x then d would.
		var nextModel TableModel
		var nextCmd tea.Cmd
		nextModel, nextCmd = m.handleControl(ControlRequestMsg{Request: *msg.next, Reply: msg.reply})
		return nextModel, tea.Batch(cmd, nextCmd)
	}
	switch msg.req.Cmd {
	case control.CmdStartAll, control.CmdStopAll:
		sendReply(msg.reply, control.NewResponse(m.statusResult()))
	default:
		sendReply(msg.reply, m.actionResponse(msg.req, msg.inner))
	}
	return m, cmd
}

// actionResponse builds the start/stop/restart reply from the applied
// completion message and the worker's resulting status.
func (m TableModel) actionResponse(req control.Request, inner tea.Msg) control.Response {
	switch done := inner.(type) {
	case processStartedMsg:
		if done.err != nil {
			return control.Errorf("start %s: %v", req.Name, done.err)
		}
	case processStoppedMsg:
		if done.err != nil {
			return control.Errorf("stop %s: %v", req.Name, done.err)
		}
	}
	idx := m.indexOfWorker(req.Name)
	if idx < 0 {
		return control.Errorf("no process named %q", req.Name)
	}
	w := m.workers[idx]
	if inner == nil && w.Status == worker.StatusFailed {
		if m.statusMsg != "" {
			return control.Errorf("%s", m.statusMsg)
		}
		return control.Errorf("%s failed to start", req.Name)
	}
	return control.NewResponse(control.ActionResult{Name: w.Name, Status: string(w.Status)})
}

// autoConfirm answers "yes" to any y/N confirm dialog a handler raised, since
// a ctl caller has no keyboard to answer it with.
func (m TableModel) autoConfirm() TableModel {
	if !m.confirm.Visible() {
		return m
	}
	onYes := m.confirm.onYes
	m.confirm.Hide()
	if onYes != nil {
		m, _ = onYes(m)
	}
	return m
}

func (m TableModel) indexOfWorker(name string) int {
	for i, w := range m.workers {
		if w.Name == name {
			return i
		}
	}
	return -1
}

// statusResult snapshots every row the table renders.
func (m TableModel) statusResult() control.StatusResult {
	rows := make([]control.StatusRow, 0, len(m.workers))
	for _, w := range m.workers {
		row := control.StatusRow{
			Name:   w.Name,
			Status: string(w.Status),
			Port:   w.Port,
			Path:   w.Path,
			Type:   m.processType(w),
		}
		if p, exists := m.manager.GetProcess(w.Name); exists {
			row.UptimeSeconds = int64(time.Since(p.Started).Seconds())
			if p.IsRunning() {
				row.PID = p.Pid()
			}
		}
		if row.Type == string(config.ProcessTypeWorker) {
			row.DevMode = string(w.EffectiveDevMode())
			row.SelectedMode = w.SelectedMode
			row.DevServerURL = w.DevServerURL
		}
		rows = append(rows, row)
	}
	return control.StatusResult{Processes: rows}
}

// processType mirrors how the table/manager treat a row: config processes
// carry their configured type (service when unset); discovered workers are
// always cf workers.
func (m TableModel) processType(w *worker.Worker) string {
	if _, isConfig := w.Scripts["_config"]; isConfig && m.config != nil {
		if proc, exists := m.config.Processes[w.Name]; exists {
			if proc.Type == "" {
				return string(config.ProcessTypeService)
			}
			return string(proc.Type)
		}
	}
	return string(config.ProcessTypeWorker)
}
