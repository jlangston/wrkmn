package worker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"wrkmn/internal/wrangler"
)

type Discoverer struct {
	portRegex *regexp.Regexp
}

func NewDiscoverer() *Discoverer {
	return &Discoverer{
		portRegex: regexp.MustCompile(`--port\s+(\d+)`),
	}
}

func (d *Discoverer) DiscoverWorkers(rootPath string) ([]*Worker, error) {
	var workers []*Worker

	entries, err := os.ReadDir(rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", rootPath, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			workerPath := filepath.Join(rootPath, entry.Name())
			packageJSONPath := filepath.Join(workerPath, "package.json")

			if _, err := os.Stat(packageJSONPath); err == nil {
				worker, err := d.parseWorkerFromPackageJSON(packageJSONPath)
				if err != nil {
					fmt.Printf("Warning: Failed to parse %s: %v\n", packageJSONPath, err)
					continue
				}

				if worker != nil {
					workers = append(workers, worker)
				}
			}
		}
	}

	return workers, nil
}

func (d *Discoverer) parseWorkerFromPackageJSON(packageJSONPath string) (*Worker, error) {
	data, err := os.ReadFile(packageJSONPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read package.json: %w", err)
	}

	var pkg PackageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("failed to parse package.json: %w", err)
	}

	if !d.hasWranglerScripts(pkg.Scripts) {
		return nil, nil
	}

	workerDir := filepath.Dir(packageJSONPath)
	workerName := filepath.Base(workerDir)

	if pkg.Name != "" {
		workerName = pkg.Name
	}

	port := d.extractPortFromDevScript(pkg.Scripts["dev"])

	worker := &Worker{
		Name:        workerName,
		Path:        workerDir,
		PackageJSON: &pkg,
		Scripts:     pkg.Scripts,
		Port:        port,
		Status:      StatusStopped,
	}

	// Parse wrangler.toml if present
	d.parseWranglerConfig(worker)

	return worker, nil
}

// parseWranglerConfig loads and parses wrangler.toml for a worker
func (d *Discoverer) parseWranglerConfig(w *Worker) {
	if !wrangler.Exists(w.Path) {
		return
	}

	config, err := wrangler.LoadFromDirectory(w.Path)
	if err != nil {
		// Log warning but continue - worker can still function without wrangler config
		fmt.Printf("Warning: Failed to parse wrangler.toml for %s: %v\n", w.Name, err)
		return
	}

	w.WranglerConfig = config

	// Use name from wrangler.toml if available
	if config.Name != "" {
		w.Name = config.Name
	}

	// Set account ID from root config
	w.AccountID = config.AccountID

	// Extract and sort environments
	w.Environments = config.GetEnvironments()
	sort.Strings(w.Environments)

	// Default to "production" (the root config)
	w.SelectedEnv = "production"
}

func (d *Discoverer) hasWranglerScripts(scripts map[string]string) bool {
	for _, script := range scripts {
		if strings.Contains(script, "wrangler") {
			return true
		}
	}
	return false
}

func (d *Discoverer) extractPortFromDevScript(devScript string) int {
	if devScript == "" {
		return 0
	}

	matches := d.portRegex.FindStringSubmatch(devScript)
	if len(matches) > 1 {
		if port, err := strconv.Atoi(matches[1]); err == nil {
			return port
		}
	}

	return 0
}
