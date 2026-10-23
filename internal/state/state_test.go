package state

import (
	"os"
	"path/filepath"
	"testing"

	"wrkmn/internal/config"
	"wrkmn/internal/worker"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	workers := []*worker.Worker{
		{Name: "api", DevMode: config.DevModeLocal, SelectedMode: "staging"},
		{Name: "web"}, // nothing selected — should not be persisted
	}
	if err := Save(dir, workers); err != nil {
		t.Fatalf("save: %v", err)
	}
	states, err := Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := states["api"]; got.DevMode != "local" || got.SelectedMode != "staging" {
		t.Fatalf("api state = %+v", got)
	}
	if _, ok := states["web"]; ok {
		t.Fatal("worker with no selections should not be persisted")
	}
}

func TestLoadMigratesLegacySelectedEnv(t *testing.T) {
	dir := t.TempDir()
	legacy := `{"workers":{"api":{"dev_mode":"remote","selected_env":"staging"}}}`
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	states, err := Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if states["api"].SelectedMode != "staging" {
		t.Fatalf("legacy selected_env not migrated: %+v", states["api"])
	}
	// dev_mode "remote" survives Load but must be dropped by Apply (Task 2 allowlist).
	workers := []*worker.Worker{{Name: "api"}}
	Apply(states, workers)
	if workers[0].DevMode != "" || workers[0].SelectedMode != "staging" {
		t.Fatalf("Apply = DevMode %q SelectedMode %q", workers[0].DevMode, workers[0].SelectedMode)
	}
}

func TestLoadMissingFileIsEmpty(t *testing.T) {
	states, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(states) != 0 {
		t.Fatalf("states = %v, want empty", states)
	}
}

func TestApplyOverridesDefaultsAndIgnoresBogusModes(t *testing.T) {
	workers := []*worker.Worker{
		{Name: "api", DevMode: config.DevModeRemoteBindings},
		{Name: "web"},
	}
	Apply(map[string]WorkerState{
		"api": {DevMode: "local", SelectedMode: "staging"},
		"web": {DevMode: "remote"}, // removed mode — must be ignored
	}, workers)
	if workers[0].DevMode != config.DevModeLocal || workers[0].SelectedMode != "staging" {
		t.Fatalf("api = %+v, want saved selections applied", workers[0])
	}
	if workers[1].DevMode != "" {
		t.Fatalf("web.DevMode = %q, bogus saved mode must be ignored", workers[1].DevMode)
	}
}

func TestLoadCorruptFileIsError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("{nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("expected error for corrupt state file")
	}
}
