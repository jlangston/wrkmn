package ui

import (
	"testing"

	"wrkmn/internal/config"
	"wrkmn/internal/state"
	"wrkmn/internal/worker"
)

// TestSaveStateMasksConfigDerivedDevMode verifies that saveState does not
// freeze a config-derived DevMode into the state file (Finding 1): a worker
// whose DevMode still equals its config entry must not appear with that
// DevMode in the saved file, while a worker whose DevMode genuinely diverges
// from config must be saved.
func TestSaveStateMasksConfigDerivedDevMode(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		BaseDir: dir,
		Processes: map[string]config.Process{
			"api": {Command: "sh", DevMode: config.DevModeRemoteBindings},
			"web": {Command: "sh", DevMode: config.DevModeRemoteBindings},
		},
	}
	workers := []*worker.Worker{
		// "api" was seeded from config (cmd/root.go behavior) and never
		// changed by the user — must not be persisted.
		{Name: "api", Status: worker.StatusStopped, DevMode: config.DevModeRemoteBindings, Scripts: map[string]string{"_config": "sh"}},
		// "web" was seeded from config but the user switched it — must be
		// persisted as a genuine divergence.
		{Name: "web", Status: worker.StatusStopped, DevMode: config.DevModeLocal, Scripts: map[string]string{"_config": "sh"}},
	}
	m := NewTableModel(workers, cfg)

	m.saveState()

	saved, err := state.Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got, ok := saved["api"]; ok {
		t.Fatalf("api should not be saved (config-derived DevMode), got %+v", got)
	}
	got, ok := saved["web"]
	if !ok {
		t.Fatal("web should be saved (diverges from config)")
	}
	if got.DevMode != string(config.DevModeLocal) {
		t.Fatalf("web.DevMode = %q, want %q", got.DevMode, config.DevModeLocal)
	}

	// The in-memory workers must be untouched by the masking (only the copies
	// passed to Save are masked).
	if workers[0].DevMode != config.DevModeRemoteBindings {
		t.Fatalf("workers[0].DevMode mutated: %q", workers[0].DevMode)
	}
}
