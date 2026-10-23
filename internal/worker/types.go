package worker

import (
	"time"

	"wrkmn/internal/config"
	"wrkmn/internal/wrangler"
)

type Worker struct {
	Name        string            `json:"name"`
	Path        string            `json:"path"`
	PackageJSON *PackageJSON      `json:"package_json"`
	Scripts     map[string]string `json:"scripts"`
	Port        int               `json:"port"`
	// InspectorPort is the V8 inspector / debugger port (wrangler --inspector-port).
	// Distinct from Port (the HTTP serving port). 0 if not configured.
	InspectorPort int          `json:"inspector_port,omitempty"`
	Status        WorkerStatus `json:"status"`
	LastUpdated   time.Time    `json:"last_updated"`

	// Wrangler configuration (parsed from wrangler.toml)
	WranglerConfig *wrangler.Config `json:"-"`

	// Modes a worker's cloudflare.config.ts function accepts (cf dev --mode),
	// e.g. "development", "production".
	Modes []string `json:"modes,omitempty"`

	// SelectedMode is the currently selected cf dev --mode for remote
	// operations (logs, analytics).
	SelectedMode string `json:"selected_mode,omitempty"`

	// DevMode is the runtime-selected dev mode (local / remote-bindings / remote).
	// Empty means remote-bindings (see EffectiveDevMode).
	DevMode config.DevMode `json:"dev_mode,omitempty"`

	// AccountID from wrangler.toml (root level)
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

// GetCurrentAccountID returns the account ID for remote operations.
func (w *Worker) GetCurrentAccountID() string {
	return w.AccountID
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

// IsCloudflareWorker returns true if this worker has a wrangler.toml configuration
func (w *Worker) IsCloudflareWorker() bool {
	return w.WranglerConfig != nil
}
