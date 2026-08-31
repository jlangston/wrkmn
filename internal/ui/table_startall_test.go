package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"wrkmn/internal/config"
	"wrkmn/internal/worker"
)

func TestStartAllRespectsDependencies(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "order")
	cfg := &config.Config{
		BaseDir: dir,
		Processes: map[string]config.Process{
			"db": {
				Command: "sh",
				Args:    []string{"-c", "echo db >> " + marker + "; sleep 30"},
				Type:    config.ProcessTypeService,
			},
			"app": {
				Command: "sh",
				Args:    []string{"-c", "echo app >> " + marker + "; sleep 30"},
				Type:    config.ProcessTypeService,
				DependsOn: []config.Dependency{
					{Process: "db", Condition: config.ConditionStarted},
				},
			},
		},
	}
	workers := []*worker.Worker{
		{Name: "db", Status: worker.StatusStopped, Scripts: map[string]string{"_config": "sh"}},
		{Name: "app", Status: worker.StatusStopped, Scripts: map[string]string{"_config": "sh"}},
	}
	m := NewTableModel(workers, cfg)
	defer m.GetManager().StopAll()

	m2, cmd := m.startAllWorkers()
	if cmd == nil {
		t.Fatal("startAllWorkers returned no command")
	}
	msg := cmd() // runs the scheduler synchronously
	if _, ok := msg.(allProcessesStartedMsg); !ok {
		t.Fatalf("msg = %T, want allProcessesStartedMsg", msg)
	}
	_ = m2

	deadline := time.Now().Add(5 * time.Second)
	var order []string
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(marker)
		order = strings.Fields(string(data))
		if len(order) >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(order) != 2 || order[0] != "db" || order[1] != "app" {
		t.Fatalf("start order = %v, want [db app]", order)
	}
}
