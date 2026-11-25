package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"wrkmn/internal/config"
	"wrkmn/internal/worker"
)

func autostartTestModel(t *testing.T) (TableModel, string, string) {
	t.Helper()
	dir := t.TempDir()
	autoMarker := filepath.Join(dir, "auto-ran")
	manualMarker := filepath.Join(dir, "manual-ran")
	off := false
	cfg := &config.Config{
		BaseDir: dir,
		Processes: map[string]config.Process{
			"api": {
				Command: "sh",
				Args:    []string{"-c", "echo ran >> " + autoMarker + "; sleep 30"},
				Type:    config.ProcessTypeService,
			},
			"funnel": {
				Command:   "sh",
				Args:      []string{"-c", "echo ran >> " + manualMarker + "; sleep 30"},
				Type:      config.ProcessTypeService,
				Autostart: &off,
			},
		},
	}
	workers := []*worker.Worker{
		{Name: "api", Status: worker.StatusStopped, Scripts: map[string]string{"_config": "sh"}},
		{Name: "funnel", Status: worker.StatusStopped, Manual: true, Scripts: map[string]string{"_config": "sh"}},
	}
	m := NewTableModel(workers, cfg)
	return m, autoMarker, manualMarker
}

func TestStartAllSkipsManualRow(t *testing.T) {
	m, autoMarker, manualMarker := autostartTestModel(t)
	defer m.GetManager().StopAll()

	m2, cmd := m.startAllWorkers()
	if cmd == nil {
		t.Fatal("startAllWorkers returned no command")
	}
	msg := cmd()
	started, ok := msg.(allProcessesStartedMsg)
	if !ok {
		t.Fatalf("msg = %T, want allProcessesStartedMsg", msg)
	}
	// No dependent on the manual row here, so start-all has nothing to warn about.
	if started.warn != "" {
		t.Fatalf("unexpected warning: %q", started.warn)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(autoMarker); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := os.Stat(autoMarker); err != nil {
		t.Fatal("autostart process did not start")
	}
	if _, err := os.Stat(manualMarker); err == nil {
		t.Fatal("manual process was started by start-all")
	}
	for _, w := range m2.workers {
		if w.Name == "funnel" && w.Status != worker.StatusStopped {
			t.Fatalf("manual row status = %s, want stopped (never marked starting)", w.Status)
		}
	}
}

func TestSpaceTogglesRow(t *testing.T) {
	m, autoMarker, _ := autostartTestModel(t)
	defer m.GetManager().StopAll()

	space := tea.KeyMsg{Type: tea.KeySpace}

	// Cursor starts at row 0; order of m.workers is as constructed: api first.
	model, cmd := m.handleTableKeys(space)
	if cmd == nil {
		t.Fatal("space on a stopped row should produce a start command")
	}
	msg := cmd()
	startMsg, ok := msg.(processStartedMsg)
	if !ok || startMsg.err != nil {
		t.Fatalf("toggle start = %T %v", msg, msg)
	}
	m2 := model.(TableModel)
	model2, _ := m2.Update(msg)
	m3 := model2.(TableModel)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(autoMarker); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, exists := m3.GetManager().GetProcess("api"); !exists {
		t.Fatal("toggle did not start the process")
	}

	model3, cmd2 := m3.handleTableKeys(space)
	if cmd2 == nil {
		t.Fatal("space on a running row should produce a stop command")
	}
	msg2 := cmd2()
	if stopMsg, ok := msg2.(processStoppedMsg); !ok || stopMsg.err != nil {
		t.Fatalf("toggle stop = %T %v", msg2, msg2)
	}
	m4 := model3.(TableModel)
	if _, exists := m4.GetManager().GetProcess("api"); exists {
		t.Fatal("toggle did not stop the process")
	}
}

func TestManualStatusCellRendersManual(t *testing.T) {
	m, _, _ := autostartTestModel(t)
	var funnel *worker.Worker
	for _, w := range m.workers {
		if w.Name == "funnel" {
			funnel = w
		}
	}
	row := m.renderDataRow(funnel, 1, 20, 12, 8, 10, 20)
	if !strings.Contains(row, "manual") {
		t.Fatalf("manual row should render 'manual', got: %q", row)
	}
	// Once running it reads like any other row.
	funnel.Status = worker.StatusRunning
	row = m.renderDataRow(funnel, 1, 20, 12, 8, 10, 20)
	if strings.Contains(row, "manual") {
		t.Fatalf("running manual row should not read 'manual': %q", row)
	}
}
