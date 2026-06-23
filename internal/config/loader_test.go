package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindConfigFile(t *testing.T) {
	t.Run("finds worker-compose.yaml", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "worker-compose.yaml")
		err := os.WriteFile(configPath, []byte("version: '1.0'"), 0644)
		require.NoError(t, err)

		found := FindConfigFile(dir)
		assert.Equal(t, configPath, found)
	})

	t.Run("finds worker-compose.yml", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "worker-compose.yml")
		err := os.WriteFile(configPath, []byte("version: '1.0'"), 0644)
		require.NoError(t, err)

		found := FindConfigFile(dir)
		assert.Equal(t, configPath, found)
	})

	t.Run("prefers yaml over yml", func(t *testing.T) {
		dir := t.TempDir()
		yamlPath := filepath.Join(dir, "worker-compose.yaml")
		ymlPath := filepath.Join(dir, "worker-compose.yml")
		require.NoError(t, os.WriteFile(yamlPath, []byte("version: '1.0'"), 0644))
		require.NoError(t, os.WriteFile(ymlPath, []byte("version: '1.0'"), 0644))

		found := FindConfigFile(dir)
		assert.Equal(t, yamlPath, found)
	})

	t.Run("finds dotfile config", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, ".worker-compose.yaml")
		err := os.WriteFile(configPath, []byte("version: '1.0'"), 0644)
		require.NoError(t, err)

		found := FindConfigFile(dir)
		assert.Equal(t, configPath, found)
	})

	t.Run("returns empty when not found", func(t *testing.T) {
		dir := t.TempDir()
		found := FindConfigFile(dir)
		assert.Equal(t, "", found)
	})
}

func TestLoader_Load(t *testing.T) {
	t.Run("loads valid config", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "worker-compose.yaml")
		configContent := `
version: "1.0"
processes:
  api:
    command: npm
    args: [run, dev]
`
		require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

		loader := NewLoader(configPath)
		cfg, err := loader.Load()

		require.NoError(t, err)
		assert.Equal(t, "1.0", cfg.Version)
		assert.Contains(t, cfg.Processes, "api")
	})

	t.Run("returns default when file not found", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "nonexistent.yaml")

		loader := NewLoader(configPath)
		cfg, err := loader.Load()

		require.NoError(t, err)
		assert.Equal(t, "1.0", cfg.Version)
		assert.True(t, cfg.Discovery.Enabled)
	})

	t.Run("returns error for invalid YAML", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "worker-compose.yaml")
		require.NoError(t, os.WriteFile(configPath, []byte("invalid: yaml: content:"), 0644))

		loader := NewLoader(configPath)
		_, err := loader.Load()

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "parsing config")
	})
}

func TestLoader_Validate(t *testing.T) {
	t.Run("rejects health_check with startup_delay", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "worker-compose.yaml")
		configContent := `
version: "1.0"
processes:
  api:
    command: npm
    health_check:
      type: http
      target: http://localhost:3000
    startup_delay: 5s
`
		require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

		loader := NewLoader(configPath)
		_, err := loader.Load()

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot specify both health_check and startup_delay")
	})

	t.Run("rejects invalid health_check type", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "worker-compose.yaml")
		configContent := `
version: "1.0"
processes:
  api:
    command: npm
    health_check:
      type: invalid
      target: http://localhost:3000
`
		require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

		loader := NewLoader(configPath)
		_, err := loader.Load()

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid health_check.type")
	})

	t.Run("rejects missing health_check type", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "worker-compose.yaml")
		configContent := `
version: "1.0"
processes:
  api:
    command: npm
    health_check:
      target: http://localhost:3000
`
		require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

		loader := NewLoader(configPath)
		_, err := loader.Load()

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "health_check.type is required")
	})

	t.Run("rejects invalid dependency condition", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "worker-compose.yaml")
		configContent := `
version: "1.0"
processes:
  api:
    command: npm
    depends_on:
      - process: db
        condition: invalid_condition
  db:
    command: docker
`
		require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

		loader := NewLoader(configPath)
		_, err := loader.Load()

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid depends_on")
	})

	t.Run("rejects missing dependency condition", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "worker-compose.yaml")
		configContent := `
version: "1.0"
processes:
  api:
    command: npm
    depends_on:
      - process: db
  db:
    command: docker
`
		require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

		loader := NewLoader(configPath)
		_, err := loader.Load()

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "condition is required")
	})

	t.Run("rejects unknown dependency", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "worker-compose.yaml")
		configContent := `
version: "1.0"
processes:
  api:
    command: npm
    depends_on:
      - process: unknown
        condition: process_started
`
		require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

		loader := NewLoader(configPath)
		_, err := loader.Load()

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "depends on unknown process")
	})
}

func TestLoader_ValidateDependencyGraph(t *testing.T) {
	t.Run("accepts valid dependency chain", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "worker-compose.yaml")
		configContent := `
version: "1.0"
processes:
  api:
    command: npm
    depends_on:
      - process: cache
        condition: process_healthy
  cache:
    command: redis
    depends_on:
      - process: db
        condition: process_started
  db:
    command: postgres
`
		require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

		loader := NewLoader(configPath)
		cfg, err := loader.Load()

		require.NoError(t, err)
		assert.Len(t, cfg.Processes, 3)
	})

	t.Run("rejects self-referential dependency", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "worker-compose.yaml")
		configContent := `
version: "1.0"
processes:
  api:
    command: npm
    depends_on:
      - process: api
        condition: process_started
`
		require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

		loader := NewLoader(configPath)
		_, err := loader.Load()

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "circular dependency")
	})

	t.Run("rejects circular dependency", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "worker-compose.yaml")
		configContent := `
version: "1.0"
processes:
  a:
    command: a
    depends_on:
      - process: b
        condition: process_started
  b:
    command: b
    depends_on:
      - process: c
        condition: process_started
  c:
    command: c
    depends_on:
      - process: a
        condition: process_started
`
		require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

		loader := NewLoader(configPath)
		_, err := loader.Load()

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "circular dependency")
	})
}

func TestLoader_ApplyDefaults(t *testing.T) {
	t.Run("applies health check defaults", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "worker-compose.yaml")
		configContent := `
version: "1.0"
processes:
  api:
    command: npm
    health_check:
      type: http
      target: http://localhost:3000
`
		require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

		loader := NewLoader(configPath)
		cfg, err := loader.Load()

		require.NoError(t, err)
		hc := cfg.Processes["api"].HealthCheck
		assert.Equal(t, DefaultHealthCheckInterval, hc.Interval.Duration)
		assert.Equal(t, DefaultHealthCheckTimeout, hc.Timeout.Duration)
		assert.Equal(t, DefaultHealthCheckRetries, hc.Retries)
	})

	t.Run("applies restart defaults", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "worker-compose.yaml")
		configContent := `
version: "1.0"
processes:
  api:
    command: npm
    restart:
      enabled: true
`
		require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

		loader := NewLoader(configPath)
		cfg, err := loader.Load()

		require.NoError(t, err)
		restart := cfg.Processes["api"].Restart
		assert.Equal(t, 3, restart.MaxTries)
		assert.Equal(t, time.Second, restart.Delay.Duration)
	})

	t.Run("sets process type based on inspector port", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "worker-compose.yaml")
		configContent := `
version: "1.0"
processes:
  worker:
    command: wrangler
    inspector_port: 9229
  service:
    command: node
`
		require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

		loader := NewLoader(configPath)
		cfg, err := loader.Load()

		require.NoError(t, err)
		assert.Equal(t, ProcessTypeWorker, cfg.Processes["worker"].Type)
		assert.Equal(t, ProcessTypeService, cfg.Processes["service"].Type)
	})

	t.Run("applies discovery defaults", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "worker-compose.yaml")
		configContent := `
version: "1.0"
discovery:
  enabled: true
`
		require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

		loader := NewLoader(configPath)
		cfg, err := loader.Load()

		require.NoError(t, err)
		assert.Contains(t, cfg.Discovery.ScriptNames, "dev")
		assert.Contains(t, cfg.Discovery.ScriptNames, "start")
	})
}

func TestLoader_ResolveWorkingDirs(t *testing.T) {
	t.Run("resolves relative working dirs", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "worker-compose.yaml")
		configContent := `
version: "1.0"
processes:
  api:
    command: npm
    working_dir: ./api
`
		require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

		loader := NewLoader(configPath)
		cfg, err := loader.Load()

		require.NoError(t, err)
		expected := filepath.Join(dir, "api")
		assert.Equal(t, expected, cfg.Processes["api"].WorkingDir)
	})

	t.Run("preserves absolute working dirs", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "worker-compose.yaml")
		configContent := `
version: "1.0"
processes:
  api:
    command: npm
    working_dir: /absolute/path
`
		require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

		loader := NewLoader(configPath)
		cfg, err := loader.Load()

		require.NoError(t, err)
		assert.Equal(t, "/absolute/path", cfg.Processes["api"].WorkingDir)
	})

	t.Run("resolves discovery paths", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "worker-compose.yaml")
		configContent := `
version: "1.0"
discovery:
  enabled: true
  paths:
    - ./workers
    - /absolute/workers
`
		require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

		loader := NewLoader(configPath)
		cfg, err := loader.Load()

		require.NoError(t, err)
		assert.Equal(t, filepath.Join(dir, "workers"), cfg.Discovery.Paths[0])
		assert.Equal(t, "/absolute/workers", cfg.Discovery.Paths[1])
	})
}

func TestLoadOrDefault(t *testing.T) {
	t.Run("loads config when present", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "worker-compose.yaml")
		configContent := `
version: "2.0"
processes:
  test:
    command: echo
`
		require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

		cfg, err := LoadOrDefault(dir)

		require.NoError(t, err)
		assert.Equal(t, "2.0", cfg.Version)
	})

	t.Run("returns defaults when no config", func(t *testing.T) {
		dir := t.TempDir()

		cfg, err := LoadOrDefault(dir)

		require.NoError(t, err)
		assert.Equal(t, "1.0", cfg.Version)
		assert.True(t, cfg.Discovery.Enabled)
	})
}

func writeTempConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "worker-compose.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestDevModeDefaultsToRemoteBindingsForWorkers(t *testing.T) {
	path := writeTempConfig(t, `
version: "1.0"
processes:
  api:
    command: bun
    args: [run, dev]
    type: worker
    inspector_port: 9229
`)
	cfg, err := NewLoader(path).Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := cfg.Processes["api"].DevMode; got != DevModeRemoteBindings {
		t.Fatalf("DevMode = %q, want %q", got, DevModeRemoteBindings)
	}
}

func TestDevModeExplicitValuePreserved(t *testing.T) {
	path := writeTempConfig(t, `
version: "1.0"
processes:
  api:
    command: bun
    args: [run, dev]
    type: worker
    dev_mode: local
`)
	cfg, err := NewLoader(path).Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := cfg.Processes["api"].DevMode; got != DevModeLocal {
		t.Fatalf("DevMode = %q, want %q", got, DevModeLocal)
	}
}

func TestDevModeInvalidRejected(t *testing.T) {
	path := writeTempConfig(t, `
version: "1.0"
processes:
  api:
    command: bun
    args: [run, dev]
    type: worker
    dev_mode: bogus
`)
	if _, err := NewLoader(path).Load(); err == nil {
		t.Fatal("expected error for invalid dev_mode, got nil")
	}
}
