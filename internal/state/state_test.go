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
		{Name: "api", DevMode: config.DevModeLocal, SelectedEnv: "staging"},
		{Name: "web"}, // nothing selected — should not be persisted
	}
	if err := Save(dir, workers); err != nil {
		t.Fatalf("save: %v", err)
	}
	states, err := Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := states["api"]; got.DevMode != "local" || got.SelectedEnv != "staging" {
		t.Fatalf("api state = %+v", got)
	}
	if _, ok := states["web"]; ok {
		t.Fatal("worker with no selections should not be persisted")
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
		"api": {DevMode: "local", SelectedEnv: "staging"},
		"web": {DevMode: "remote"}, // removed mode — must be ignored
	}, workers)
	if workers[0].DevMode != config.DevModeLocal || workers[0].SelectedEnv != "staging" {
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
