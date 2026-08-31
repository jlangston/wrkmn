package ui

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wrkmn/internal/config"
	"wrkmn/internal/worker"
)

// TestStartAllCancelUnblocksPromptly covers Finding 2: cancelling the model's
// stored startAllCancel must unblock startAllWorkers's returned cmd promptly,
// even while it's stuck waiting on a dependency's health check that will
// never pass, and must prevent any dependent gated on that health from ever
// starting.
func TestStartAllCancelUnblocksPromptly(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "order")

	// Reserve a free port, then release it immediately — nothing listens on
	// it, so "db"'s TCP health check can never succeed and its wait blocks
	// until the ctx is cancelled.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	cfg := &config.Config{
		BaseDir: dir,
		Processes: map[string]config.Process{
			"db": {
				Command: "sh",
				Args:    []string{"-c", "sleep 30"},
				Type:    config.ProcessTypeService,
				HealthCheck: &config.HealthCheck{
					Type:     config.HealthCheckTCP,
					Target:   addr,
					Interval: config.Duration{Duration: 50 * time.Millisecond},
					Retries:  100000, // effectively never gives up on its own
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

	m2, cmd := m.startAllWorkers()
	if cmd == nil {
		t.Fatal("startAllWorkers returned no command")
	}
	if m2.startAllCancel == nil {
		t.Fatal("startAllCancel was not stored on the returned model")
	}

	done := make(chan struct{})
	go func() {
		cmd() // runs the scheduler; blocks until cancelled or db becomes healthy
		close(done)
	}()

	// Give the scheduler time to start "db" and get stuck waiting on its
	// (unsatisfiable) health check.
	time.Sleep(100 * time.Millisecond)
	m2.startAllCancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("cmd() did not return within 5s of cancellation")
	}

	// "app" is gated on db's health, which never came — it must never have
	// started.
	if data, err := os.ReadFile(marker); err == nil {
		t.Fatalf("app should not have started, marker contents = %q", string(data))
	} else if !os.IsNotExist(err) {
		t.Fatalf("unexpected error reading marker: %v", err)
	}
}
