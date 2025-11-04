package worker

import "time"

type Worker struct {
	Name        string            `json:"name"`
	Path        string            `json:"path"`
	PackageJSON *PackageJSON      `json:"package_json"`
	Scripts     map[string]string `json:"scripts"`
	Port        int               `json:"port"`
	Status      WorkerStatus      `json:"status"`
	LastUpdated time.Time         `json:"last_updated"`
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
