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

	// Available environments from wrangler.toml (e.g., "production", "staging")
	Environments []string `json:"environments,omitempty"`

	// Currently selected environment for remote operations (logs, analytics)
	SelectedEnv string `json:"selected_env,omitempty"`

	// DevMode is the runtime-selected dev mode (local / remote-bindings / remote).
	// Empty means remote-bindings (see EffectiveDevMode).
	DevMode config.DevMode `json:"dev_mode,omitempty"`

	// AccountID from wrangler.toml (root level)
	AccountID string `json:"account_id,omitempty"`
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

// GetAccountIDForEnv returns the account ID for the given environment.
// Falls back to the root account ID if the environment doesn't have a specific one.
func (w *Worker) GetAccountIDForEnv(env string) string {
	if w.WranglerConfig == nil {
		return w.AccountID
	}
	return w.WranglerConfig.GetAccountID(env)
}

// GetCurrentAccountID returns the account ID for the currently selected environment
func (w *Worker) GetCurrentAccountID() string {
	return w.GetAccountIDForEnv(w.SelectedEnv)
}

// HasEnvironments returns true if the worker has multiple environments configured
func (w *Worker) HasEnvironments() bool {
	return len(w.Environments) > 1
}

// EffectiveDevMode returns the selected dev mode, defaulting to remote-bindings.
func (w *Worker) EffectiveDevMode() config.DevMode {
	if w.DevMode == "" {
		return config.DevModeRemoteBindings
	}
	return w.DevMode
}

// SupportsDebugging reports whether breakpoint debugging is available in the
// current dev mode. Full-remote runs on the edge, so the local inspector cannot attach.
func (w *Worker) SupportsDebugging() bool {
	return w.EffectiveDevMode() != config.DevModeRemote
}

// IsCloudflareWorker returns true if this worker has a wrangler.toml configuration
func (w *Worker) IsCloudflareWorker() bool {
	return w.WranglerConfig != nil
}
