package worker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
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

	return worker, nil
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
