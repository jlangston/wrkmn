package ui

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
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

// TestStartAllHealthCheckGatesDependentAndStopsMonitor exercises the
// process_healthy path (a health-checked process gating a dependent) and
// confirms the health-monitor goroutine spawned for it doesn't leak past
// start-all's orchestration window.
func TestStartAllHealthCheckGatesDependentAndStopsMonitor(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "order")

	// A real TCP listener stands in for "db"'s health endpoint — the health
	// monitor dials this directly, so we don't need db's own script to open a
	// port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	cfg := &config.Config{
		BaseDir: dir,
		Processes: map[string]config.Process{
			"db": {
				Command: "sh",
				Args:    []string{"-c", "echo db >> " + marker + "; sleep 30"},
				Type:    config.ProcessTypeService,
				HealthCheck: &config.HealthCheck{
					Type:     config.HealthCheckTCP,
					Target:   ln.Addr().String(),
					Interval: config.Duration{Duration: 50 * time.Millisecond},
					Retries:  3,
				},
			},
			"app": {
				Command: "sh",
				Args:    []string{"-c", "echo app >> " + marker + "; sleep 30"},
				Type:    config.ProcessTypeService,
				DependsOn: []config.Dependency{
					{Process: "db", Condition: config.ConditionHealthy},
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

	baseline := runtime.NumGoroutine()

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

	// Stop the processes now so the health monitor's context cancellation (in
	// startAllWorkers, once StartAll returns) is the only thing keeping it
	// running down.
	m.GetManager().StopAll()

	// The health-monitor goroutine (and everything else start-all spun up for
	// this run) should wind down once orchestration finishes and its context
	// is cancelled — confirm the goroutine count settles back near baseline
	// instead of leaking one monitor per start-all cycle.
	deadline = time.Now().Add(2 * time.Second)
	var finalCount int
	for time.Now().Before(deadline) {
		finalCount = runtime.NumGoroutine()
		if finalCount <= baseline+2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if finalCount > baseline+2 {
		t.Fatalf("goroutine count = %d, want <= baseline(%d)+2", finalCount, baseline)
	}
}
