// Package state persists per-worker runtime selections (dev mode, selected
// env) across wrkmn sessions in a .wrkmn-state.json next to the config file.
// The file is a convenience cache: losing it only loses UI selections.
package state

import (
	"encoding/json"
	"os"
	"path/filepath"

	"wrkmn/internal/config"
	"wrkmn/internal/worker"
)

// FileName is the state file created in the project's base directory. Add it
// to the project's .gitignore.
const FileName = ".wrkmn-state.json"

// WorkerState holds the runtime selections worth keeping for one worker.
type WorkerState struct {
	DevMode     string `json:"dev_mode,omitempty"`
	SelectedEnv string `json:"selected_env,omitempty"`
}

type fileFormat struct {
	Workers map[string]WorkerState `json:"workers"`
}

// Load reads saved selections. A missing file yields an empty map, not an error.
func Load(baseDir string) (map[string]WorkerState, error) {
	data, err := os.ReadFile(filepath.Join(baseDir, FileName))
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]WorkerState{}, nil
		}
		return nil, err
	}
	var f fileFormat
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	if f.Workers == nil {
		f.Workers = map[string]WorkerState{}
	}
	return f.Workers, nil
}

// Save writes the selections of every worker that has any. Atomic via
// write-to-temp + rename.
func Save(baseDir string, workers []*worker.Worker) error {
	f := fileFormat{Workers: map[string]WorkerState{}}
	for _, w := range workers {
		if w.DevMode == "" && w.SelectedEnv == "" {
			continue
		}
		f.Workers[w.Name] = WorkerState{
			DevMode:     string(w.DevMode),
			SelectedEnv: w.SelectedEnv,
		}
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(baseDir, FileName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Apply overlays saved selections onto workers. Saved values win over config
// defaults — they are the user's most recent explicit choice. Unknown dev
// modes (hand-edited or stale files) are ignored.
func Apply(states map[string]WorkerState, workers []*worker.Worker) {
	for _, w := range workers {
		st, ok := states[w.Name]
		if !ok {
			continue
		}
		switch config.DevMode(st.DevMode) {
		case config.DevModeLocal, config.DevModeRemoteBindings:
			w.DevMode = config.DevMode(st.DevMode)
		}
		if st.SelectedEnv != "" {
			w.SelectedEnv = st.SelectedEnv
		}
	}
}
