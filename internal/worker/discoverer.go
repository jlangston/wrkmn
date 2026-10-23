package worker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"wrkmn/internal/cfproject"
)

type Discoverer struct{}

func NewDiscoverer() *Discoverer { return &Discoverer{} }

// DiscoverWorkers scans rootPath's immediate subdirectories. A directory with
// a cloudflare.config.* file is a worker; a directory with only a Wrangler
// config is returned in unmigrated so callers can tell the user to run
// `cf migrate` there.
func (d *Discoverer) DiscoverWorkers(rootPath string) (workers []*Worker, unmigrated []string, err error) {
	entries, err := os.ReadDir(rootPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read directory %s: %w", rootPath, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(rootPath, entry.Name())
		if _, ok := cfproject.Detect(dir); ok {
			workers = append(workers, buildWorker(dir))
			continue
		}
		if cfproject.DetectUnmigrated(dir) {
			unmigrated = append(unmigrated, dir)
		}
	}
	return workers, unmigrated, nil
}

// buildWorker assembles a Worker for a detected cf project directory.
// Name precedence: Build Output > package.json name > directory name.
func buildWorker(dir string) *Worker {
	w := &Worker{
		Name:        filepath.Base(dir),
		Path:        dir,
		Status:      StatusStopped,
		IsCfProject: true,
		Scripts:     map[string]string{},
	}

	if data, err := os.ReadFile(filepath.Join(dir, "package.json")); err == nil {
		var pkg PackageJSON
		if json.Unmarshal(data, &pkg) == nil {
			if pkg.Name != "" {
				w.Name = pkg.Name
			}
			if pkg.Scripts != nil {
				w.Scripts = pkg.Scripts
			}
			w.PackageJSON = &pkg
		}
	}

	if bo, err := cfproject.ReadBuildOutput(dir); err == nil && bo != nil {
		if bo.WorkerName != "" {
			w.Name = bo.WorkerName
		}
		if bo.AccountID != "" {
			w.AccountID = bo.AccountID
		}
	}

	return w
}
