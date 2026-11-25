package worker

import (
	"time"

	"wrkmn/internal/config"
)

type Worker struct {
	Name        string            `json:"name"`
	Path        string            `json:"path"`
	PackageJSON *PackageJSON      `json:"package_json"`
	Scripts     map[string]string `json:"scripts"`
	Port        int               `json:"port"`
	Status      WorkerStatus      `json:"status"`
	LastUpdated time.Time         `json:"last_updated"`

	// IsCfProject marks a directory with a cloudflare.config.* file.
	IsCfProject bool `json:"is_cf_project,omitempty"`

	// Manual marks a process configured with `autostart: false`: start-all,
	// dependency resolution, and the restart policy never launch it — only an
	// explicit start does. Zero value (false) means normal autostart, so
	// discovered workers need no special handling.
	Manual bool `json:"manual,omitempty"`

	// Modes a worker's cloudflare.config.ts function accepts (cf dev --mode),
	// e.g. "development", "production".
	Modes []string `json:"modes,omitempty"`

	// SelectedMode is the currently selected cf dev --mode for remote
	// operations (logs, analytics).
	SelectedMode string `json:"selected_mode,omitempty"`

	// DevMode is the runtime-selected dev mode (local / remote-bindings).
	// Empty means remote-bindings (see EffectiveDevMode).
	DevMode config.DevMode `json:"dev_mode,omitempty"`

	// AccountID for remote operations. Precedence: compose account_id > Build
	// Output > CLOUDFLARE_ACCOUNT_ID fallback inside the tail client.
	AccountID string `json:"account_id,omitempty"`

	// DevServerURL and DebugURL are discovered at runtime from the cf dev
	// startup banner (see internal/cfproject.ParseBannerLine). Written by the
	// manager's log reader; same unsynchronized-field pattern as Status
	// (parked architectural debt).
	DevServerURL string `json:"dev_server_url,omitempty"`
	DebugURL     string `json:"debug_url,omitempty"`
}

type PackageJSON struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	Scripts     map[string]string `json:"scripts"`
}

type WorkerStatus string

const (
	StatusStopped  WorkerStatus = "stopped"
	StatusRunning  WorkerStatus = "running"
	StatusFailed   WorkerStatus = "failed"
	StatusStarting WorkerStatus = "starting"
)

func (w *Worker) GetDevPort() int {
	if w.Port > 0 {
		return w.Port
	}
	return 8787 // Default Wrangler port
}

func (w *Worker) HasScript(script string) bool {
	_, exists := w.Scripts[script]
	return exists
}

// HasModes returns true if the worker has cf dev modes configured
func (w *Worker) HasModes() bool {
	return len(w.Modes) > 0
}

// EffectiveDevMode returns the selected dev mode, defaulting to remote-bindings.
func (w *Worker) EffectiveDevMode() config.DevMode {
	if w.DevMode == "" {
		return config.DevModeRemoteBindings
	}
	return w.DevMode
}

// SupportsDebugging reports whether breakpoint debugging is available. Both
// cf dev modes run code locally, so this is always true.
func (w *Worker) SupportsDebugging() bool {
	return true
}

// IsCloudflareWorker returns true if this worker is a detected cf project.
func (w *Worker) IsCloudflareWorker() bool {
	return w.IsCfProject
}
