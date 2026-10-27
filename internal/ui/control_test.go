package ui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"wrkmn/internal/config"
	"wrkmn/internal/control"
	"wrkmn/internal/worker"
)

func controlFixture(t *testing.T) TableModel {
	t.Helper()
	cfg := &config.Config{
		BaseDir: t.TempDir(),
		Processes: map[string]config.Process{
			"db":  {Command: "sh", Args: []string{"-c", "echo db-up; sleep 30"}, Type: config.ProcessTypeService},
			"app": {Command: "sh", Args: []string{"-c", "echo app-up; sleep 30"}, Type: config.ProcessTypeWorker, DevMode: config.DevModeLocal, Modes: []string{"development", "production"}},
		},
	}
	workers := []*worker.Worker{
		{Name: "db", Path: "./db", Status: worker.StatusStopped, Scripts: map[string]string{"_config": "sh"}},
		{Name: "app", Path: "./app", Status: worker.StatusStopped, DevMode: config.DevModeLocal, Modes: []string{"development", "production"}, SelectedMode: "production", Scripts: map[string]string{"_config": "sh"}},
	}
	m := NewTableModel(workers, cfg)
	t.Cleanup(m.GetManager().StopAll)
	return m
}

// drive runs one Update and, if it yields a command, executes it and feeds the
// resulting message back — the way the Bubble Tea runtime would. Returns the
// settled model.
func drive(t *testing.T, m TableModel, msg tea.Msg) TableModel {
	t.Helper()
	model, cmd := m.Update(msg)
	m = model.(TableModel)
	for cmd != nil {
		next := cmd()
		if next == nil {
			break
		}
		if _, isBatch := next.(tea.BatchMsg); isBatch {
			break // tick batches etc. — not what we're driving
		}
		model, cmd = m.Update(next)
		m = model.(TableModel)
	}
	return m
}

func sendControl(t *testing.T, m TableModel, req control.Request) (TableModel, control.Response) {
	t.Helper()
	reply := make(chan control.Response, 1)
	m = drive(t, m, ControlRequestMsg{Request: req, Reply: reply})
	select {
	case resp := <-reply:
		return m, resp
	case <-time.After(5 * time.Second):
		t.Fatalf("no reply for %+v", req)
	}
	return m, control.Response{}
}

// TestControlStartMatchesDKey is the parity test: a ControlRequestMsg{start}
// must land in the same handler the `d` key uses (startWorkerAt), differing
// only in how the row is chosen — by name instead of by cursor.
func TestControlStartMatchesDKey(t *testing.T) {
	// Keyboard path: cursor on row 1 ("app"), press d.
	viaKey := controlFixture(t)
	viaKey.cursor = 1
	viaKey = drive(t, viaKey, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	keyProc, ok := viaKey.manager.GetProcess("app")
	if !ok {
		t.Fatal("d key did not start app")
	}

	// Control path: cursor left on row 0, start "app" by name.
	viaCtl := controlFixture(t)
	viaCtl.cursor = 0
	viaCtl, resp := sendControl(t, viaCtl, control.Request{Cmd: control.CmdStart, Name: "app"})
	if !resp.OK {
		t.Fatalf("control start failed: %s", resp.Error)
	}
	if viaCtl.cursor != 0 {
		t.Fatalf("control start moved the cursor to %d", viaCtl.cursor)
	}
	ctlProc, ok := viaCtl.manager.GetProcess("app")
	if !ok {
		t.Fatal("control start did not start app")
	}
	if db, _ := viaCtl.manager.GetProcess("db"); db != nil {
		t.Fatal("control start touched the cursor row instead of the named one")
	}

	// Same handler ⇒ same launch: identical argv, dir and worker status.
	if strings.Join(keyProc.Cmd.Args, " ") != strings.Join(ctlProc.Cmd.Args, " ") {
		t.Fatalf("argv differs: key=%v ctl=%v", keyProc.Cmd.Args, ctlProc.Cmd.Args)
	}
	if viaKey.workers[1].Status != viaCtl.workers[1].Status || viaCtl.workers[1].Status != worker.StatusRunning {
		t.Fatalf("status differs: key=%s ctl=%s", viaKey.workers[1].Status, viaCtl.workers[1].Status)
	}
	var res control.ActionResult
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		t.Fatal(err)
	}
	if res.Name != "app" || res.Status != "running" {
		t.Fatalf("result = %+v", res)
	}
}

func TestControlStartReportsLaunchFailure(t *testing.T) {
	m := controlFixture(t)
	m.config.Processes["app"] = config.Process{Command: "/definitely/not/a/binary", Type: config.ProcessTypeService}
	_, resp := sendControl(t, m, control.Request{Cmd: control.CmdStart, Name: "app"})
	if resp.OK {
		t.Fatal("start of a missing binary must report ok=false")
	}
}

func TestControlUnknownNameIsAnError(t *testing.T) {
	m := controlFixture(t)
	_, resp := sendControl(t, m, control.Request{Cmd: control.CmdStart, Name: "ghost"})
	if resp.OK || !strings.Contains(resp.Error, "ghost") {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestControlStopAndRestart(t *testing.T) {
	m := controlFixture(t)
	m, resp := sendControl(t, m, control.Request{Cmd: control.CmdStart, Name: "db"})
	if !resp.OK {
		t.Fatal(resp.Error)
	}
	first, _ := m.manager.GetProcess("db")

	m, resp = sendControl(t, m, control.Request{Cmd: control.CmdRestart, Name: "db"})
	if !resp.OK {
		t.Fatalf("restart: %s", resp.Error)
	}
	second, ok := m.manager.GetProcess("db")
	if !ok || second == first || second.Pid() == first.Pid() {
		t.Fatal("restart did not produce a fresh process")
	}
	if m.workers[0].Status != worker.StatusRunning {
		t.Fatalf("status after restart = %s", m.workers[0].Status)
	}

	m, resp = sendControl(t, m, control.Request{Cmd: control.CmdStop, Name: "db"})
	if !resp.OK {
		t.Fatalf("stop: %s", resp.Error)
	}
	if _, ok := m.manager.GetProcess("db"); ok {
		t.Fatal("stop left the process registered")
	}
	if m.workers[0].Status != worker.StatusStopped {
		t.Fatalf("status after stop = %s", m.workers[0].Status)
	}

	// Stopping a stopped process surfaces the manager's error (the key path
	// swallows it; the agent wants to know).
	_, resp = sendControl(t, m, control.Request{Cmd: control.CmdStop, Name: "db"})
	if resp.OK {
		t.Fatal("stop of a stopped process should be ok=false")
	}

	// Restart of a stopped process still starts it.
	m, resp = sendControl(t, m, control.Request{Cmd: control.CmdRestart, Name: "db"})
	if !resp.OK {
		t.Fatalf("restart of stopped: %s", resp.Error)
	}
	if _, ok := m.manager.GetProcess("db"); !ok {
		t.Fatal("restart of a stopped process did not start it")
	}
}

func TestControlStartAllAndStopAllUseKeyHandlers(t *testing.T) {
	m := controlFixture(t)
	m, resp := sendControl(t, m, control.Request{Cmd: control.CmdStartAll})
	if !resp.OK {
		t.Fatalf("start-all: %s", resp.Error)
	}
	var status control.StatusResult
	if err := json.Unmarshal(resp.Result, &status); err != nil {
		t.Fatal(err)
	}
	if len(status.Processes) != 2 {
		t.Fatalf("start-all result rows = %d", len(status.Processes))
	}
	for _, row := range status.Processes {
		if row.Status != "running" {
			t.Fatalf("%s status after start-all = %s", row.Name, row.Status)
		}
	}
	if m.startAllCancel == nil {
		t.Fatal("start-all did not go through startAllWorkers (no cancel func stored)")
	}

	m, resp = sendControl(t, m, control.Request{Cmd: control.CmdStopAll})
	if !resp.OK {
		t.Fatalf("stop-all: %s", resp.Error)
	}
	if err := json.Unmarshal(resp.Result, &status); err != nil {
		t.Fatal(err)
	}
	for _, row := range status.Processes {
		if row.Status != "stopped" {
			t.Fatalf("%s status after stop-all = %s", row.Name, row.Status)
		}
	}
	if len(m.manager.GetRunningProcesses()) != 0 {
		t.Fatal("stop-all left processes running")
	}
}

func TestControlStatusRows(t *testing.T) {
	m := controlFixture(t)
	m, _ = sendControl(t, m, control.Request{Cmd: control.CmdStart, Name: "app"})
	_, resp := sendControl(t, m, control.Request{Cmd: control.CmdStatus})
	if !resp.OK {
		t.Fatal(resp.Error)
	}
	var status control.StatusResult
	if err := json.Unmarshal(resp.Result, &status); err != nil {
		t.Fatal(err)
	}
	byName := map[string]control.StatusRow{}
	for _, r := range status.Processes {
		byName[r.Name] = r
	}
	app := byName["app"]
	if app.Status != "running" || app.Type != "worker" || app.Path != "./app" {
		t.Fatalf("app row = %+v", app)
	}
	if app.DevMode != "local" || app.SelectedMode != "production" {
		t.Fatalf("app modes = %+v", app)
	}
	if app.PID == 0 {
		t.Fatalf("running row should carry a pid: %+v", app)
	}
	db := byName["db"]
	if db.Status != "stopped" || db.Type != "service" || db.DevMode != "" || db.SelectedMode != "" {
		t.Fatalf("db row = %+v", db)
	}
}

func TestControlLogsTail(t *testing.T) {
	m := controlFixture(t)
	m, _ = sendControl(t, m, control.Request{Cmd: control.CmdStart, Name: "db"})
	p, _ := m.manager.GetProcess("db")
	for i := 0; i < 10; i++ {
		p.LogBuffer.Write("line")
	}
	_, resp := sendControl(t, m, control.Request{Cmd: control.CmdLogs, Name: "db", Tail: 3})
	if !resp.OK {
		t.Fatal(resp.Error)
	}
	var logs control.LogsResult
	if err := json.Unmarshal(resp.Result, &logs); err != nil {
		t.Fatal(err)
	}
	if len(logs.Lines) != 3 {
		t.Fatalf("tail 3 returned %d lines", len(logs.Lines))
	}
	_, resp = sendControl(t, m, control.Request{Cmd: control.CmdLogs, Name: "app"})
	if resp.OK {
		t.Fatal("logs for a never-started process should be ok=false")
	}
}

func TestControlWorksOutsideDashboardView(t *testing.T) {
	m := controlFixture(t)
	m.currentView = ViewLogs
	m, resp := sendControl(t, m, control.Request{Cmd: control.CmdStart, Name: "db"})
	if !resp.OK {
		t.Fatalf("start from logs view: %s", resp.Error)
	}
	if m.currentView != ViewLogs {
		t.Fatal("control must not change the user's view")
	}
}

// If a key path raises a y/N confirm dialog, the control path answers yes.
func TestControlAutoConfirms(t *testing.T) {
	m := controlFixture(t)
	ran := false
	m.confirm.Show("sure?", func(m TableModel) (TableModel, tea.Cmd) {
		ran = true
		return m, nil
	})
	m = m.autoConfirm()
	if !ran || m.confirm.Visible() {
		t.Fatal("autoConfirm must run onYes and hide the dialog")
	}
}
