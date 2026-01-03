package worker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDiscoverer(t *testing.T) {
	d := NewDiscoverer()
	assert.NotNil(t, d)
	assert.NotNil(t, d.portRegex)
}

func TestDiscoverer_DiscoverWorkers(t *testing.T) {
	t.Run("discovers wrangler worker", func(t *testing.T) {
		dir := t.TempDir()
		workerDir := filepath.Join(dir, "my-worker")
		require.NoError(t, os.Mkdir(workerDir, 0755))

		packageJSON := `{
			"name": "my-worker",
			"version": "1.0.0",
			"scripts": {
				"dev": "wrangler dev --port 8788",
				"deploy": "wrangler deploy"
			}
		}`
		require.NoError(t, os.WriteFile(filepath.Join(workerDir, "package.json"), []byte(packageJSON), 0644))

		d := NewDiscoverer()
		workers, err := d.DiscoverWorkers(dir)

		require.NoError(t, err)
		require.Len(t, workers, 1)
		assert.Equal(t, "my-worker", workers[0].Name)
		assert.Equal(t, workerDir, workers[0].Path)
		assert.Equal(t, 8788, workers[0].Port)
		assert.Equal(t, StatusStopped, workers[0].Status)
	})

	t.Run("skips non-wrangler projects", func(t *testing.T) {
		dir := t.TempDir()
		projectDir := filepath.Join(dir, "regular-project")
		require.NoError(t, os.Mkdir(projectDir, 0755))

		packageJSON := `{
			"name": "regular-project",
			"scripts": {
				"dev": "node server.js",
				"test": "jest"
			}
		}`
		require.NoError(t, os.WriteFile(filepath.Join(projectDir, "package.json"), []byte(packageJSON), 0644))

		d := NewDiscoverer()
		workers, err := d.DiscoverWorkers(dir)

		require.NoError(t, err)
		assert.Empty(t, workers)
	})

	t.Run("discovers multiple workers", func(t *testing.T) {
		dir := t.TempDir()

		// Worker 1
		worker1Dir := filepath.Join(dir, "worker-1")
		require.NoError(t, os.Mkdir(worker1Dir, 0755))
		pkg1 := `{"name": "worker-1", "scripts": {"dev": "wrangler dev"}}`
		require.NoError(t, os.WriteFile(filepath.Join(worker1Dir, "package.json"), []byte(pkg1), 0644))

		// Worker 2
		worker2Dir := filepath.Join(dir, "worker-2")
		require.NoError(t, os.Mkdir(worker2Dir, 0755))
		pkg2 := `{"name": "worker-2", "scripts": {"dev": "wrangler dev"}}`
		require.NoError(t, os.WriteFile(filepath.Join(worker2Dir, "package.json"), []byte(pkg2), 0644))

		d := NewDiscoverer()
		workers, err := d.DiscoverWorkers(dir)

		require.NoError(t, err)
		assert.Len(t, workers, 2)
	})

	t.Run("skips directories without package.json", func(t *testing.T) {
		dir := t.TempDir()
		emptyDir := filepath.Join(dir, "empty-dir")
		require.NoError(t, os.Mkdir(emptyDir, 0755))

		d := NewDiscoverer()
		workers, err := d.DiscoverWorkers(dir)

		require.NoError(t, err)
		assert.Empty(t, workers)
	})

	t.Run("handles non-existent directory", func(t *testing.T) {
		d := NewDiscoverer()
		_, err := d.DiscoverWorkers("/non/existent/path")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read directory")
	})

	t.Run("uses directory name when package name missing", func(t *testing.T) {
		dir := t.TempDir()
		workerDir := filepath.Join(dir, "unnamed-worker")
		require.NoError(t, os.Mkdir(workerDir, 0755))

		packageJSON := `{
			"scripts": {
				"dev": "wrangler dev"
			}
		}`
		require.NoError(t, os.WriteFile(filepath.Join(workerDir, "package.json"), []byte(packageJSON), 0644))

		d := NewDiscoverer()
		workers, err := d.DiscoverWorkers(dir)

		require.NoError(t, err)
		require.Len(t, workers, 1)
		assert.Equal(t, "unnamed-worker", workers[0].Name)
	})

	t.Run("continues on invalid package.json", func(t *testing.T) {
		dir := t.TempDir()

		// Invalid JSON
		invalidDir := filepath.Join(dir, "invalid")
		require.NoError(t, os.Mkdir(invalidDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(invalidDir, "package.json"), []byte("invalid json"), 0644))

		// Valid worker
		validDir := filepath.Join(dir, "valid")
		require.NoError(t, os.Mkdir(validDir, 0755))
		validPkg := `{"name": "valid", "scripts": {"dev": "wrangler dev"}}`
		require.NoError(t, os.WriteFile(filepath.Join(validDir, "package.json"), []byte(validPkg), 0644))

		d := NewDiscoverer()
		workers, err := d.DiscoverWorkers(dir)

		require.NoError(t, err)
		assert.Len(t, workers, 1)
		assert.Equal(t, "valid", workers[0].Name)
	})
}

func TestDiscoverer_HasWranglerScripts(t *testing.T) {
	d := NewDiscoverer()

	tests := []struct {
		name     string
		scripts  map[string]string
		expected bool
	}{
		{
			"wrangler dev",
			map[string]string{"dev": "wrangler dev"},
			true,
		},
		{
			"wrangler deploy",
			map[string]string{"deploy": "wrangler deploy"},
			true,
		},
		{
			"npx wrangler",
			map[string]string{"dev": "npx wrangler dev"},
			true,
		},
		{
			"no wrangler",
			map[string]string{"dev": "node server.js"},
			false,
		},
		{
			"empty scripts",
			map[string]string{},
			false,
		},
		{
			"nil scripts",
			nil,
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.hasWranglerScripts(tt.scripts)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDiscoverer_ExtractPortFromDevScript(t *testing.T) {
	d := NewDiscoverer()

	tests := []struct {
		name     string
		script   string
		expected int
	}{
		{"port with space", "wrangler dev --port 8788", 8788},
		{"port with multiple spaces", "wrangler dev --port  9000", 9000},
		{"port at end", "wrangler dev --port 3000", 3000},
		{"port with other flags", "wrangler dev --local --port 4000 --persist", 4000},
		{"no port flag", "wrangler dev", 0},
		{"empty script", "", 0},
		{"port flag without value", "wrangler dev --port", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.extractPortFromDevScript(tt.script)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWorker_GetDevPort(t *testing.T) {
	tests := []struct {
		name     string
		port     int
		expected int
	}{
		{"custom port", 8788, 8788},
		{"default port", 0, 8787},
		{"negative port uses default", -1, 8787},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &Worker{Port: tt.port}
			if tt.port < 0 {
				w.Port = 0 // negative is treated as 0
			}
			assert.Equal(t, tt.expected, w.GetDevPort())
		})
	}
}

func TestWorker_HasScript(t *testing.T) {
	w := &Worker{
		Scripts: map[string]string{
			"dev":    "wrangler dev",
			"deploy": "wrangler deploy",
		},
	}

	assert.True(t, w.HasScript("dev"))
	assert.True(t, w.HasScript("deploy"))
	assert.False(t, w.HasScript("test"))
	assert.False(t, w.HasScript(""))
}

func TestWorkerStatus_Constants(t *testing.T) {
	assert.Equal(t, WorkerStatus("stopped"), StatusStopped)
	assert.Equal(t, WorkerStatus("running"), StatusRunning)
	assert.Equal(t, WorkerStatus("failed"), StatusFailed)
	assert.Equal(t, WorkerStatus("starting"), StatusStarting)
}
