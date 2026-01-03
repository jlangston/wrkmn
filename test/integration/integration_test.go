//go:build integration

package integration_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"wrkmn/internal/config"
	"wrkmn/internal/process"
)

// TestFullE2E_ConfigToProcessLifecycle tests the complete flow from YAML config to process lifecycle
func TestFullE2E_ConfigToProcessLifecycle(t *testing.T) {
	if os.Getenv("SKIP_INTEGRATION") == "true" {
		t.Skip("Skipping integration test (SKIP_INTEGRATION=true)")
	}

	t.Run("load config and start processes in dependency order", func(t *testing.T) {
		// Create a temporary config file
		configContent := `
version: "1.0"

processes:
  database:
    command: sleep
    args: [10]
    type: service
    startup_delay: 100ms

  api:
    command: sleep
    args: [10]
    type: service
    depends_on:
      - process: database
        condition: process_started

  frontend:
    command: sleep
    args: [10]
    type: service
    depends_on:
      - process: api
        condition: process_started

discovery:
  enabled: false
`
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "worker-compose.yaml")
		err := os.WriteFile(configPath, []byte(configContent), 0644)
		require.NoError(t, err)

		// Load configuration
		loader := config.NewLoader(configPath)
		cfg, err := loader.Load()
		require.NoError(t, err)

		// Verify config was loaded correctly
		assert.Equal(t, "1.0", cfg.Version)
		assert.Len(t, cfg.Processes, 3)
		assert.Contains(t, cfg.Processes, "database")
		assert.Contains(t, cfg.Processes, "api")
		assert.Contains(t, cfg.Processes, "frontend")

		// Create manager and dependency graph
		manager := process.NewManager()
		defer manager.StopAll()

		graph := process.NewDependencyGraph(cfg.Processes)

		// Create a starter function that uses the manager
		starter := func(ctx context.Context, name string, proc *config.Process) (*process.Process, error) {
			return manager.StartProcess(ctx, name, proc)
		}

		scheduler := process.NewScheduler(graph, starter)

		// Subscribe to events
		events := scheduler.Events()
		eventLog := make([]string, 0)
		done := make(chan struct{})

		go func() {
			for event := range events {
				eventLog = append(eventLog, event.Name)
			}
			close(done)
		}()

		// Start all processes via scheduler
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err = scheduler.StartAll(ctx)
		require.NoError(t, err)

		// Wait a bit for events to be processed
		time.Sleep(500 * time.Millisecond)

		// Verify processes are running
		allProcs := manager.GetAllProcesses()
		assert.Len(t, allProcs, 3)

		for name, proc := range allProcs {
			assert.True(t, proc.IsRunning(), "process %s should be running", name)
		}

		// Verify events were received (at least the process names)
		assert.Contains(t, eventLog, "database")
		assert.Contains(t, eventLog, "api")
		assert.Contains(t, eventLog, "frontend")

		// Verify database started before api (dependency order)
		dbIdx := indexOf(eventLog, "database")
		apiIdx := indexOf(eventLog, "api")
		frontendIdx := indexOf(eventLog, "frontend")

		assert.True(t, dbIdx < apiIdx, "database should start before api")
		assert.True(t, apiIdx < frontendIdx, "api should start before frontend")

		// Stop all and verify
		manager.StopAll()
		time.Sleep(300 * time.Millisecond)

		allProcs = manager.GetAllProcesses()
		assert.Empty(t, allProcs, "all processes should be stopped")
	})
}

// TestFullE2E_ConfigWithHealthChecks tests config with health check configuration
func TestFullE2E_ConfigWithHealthChecks(t *testing.T) {
	if os.Getenv("SKIP_INTEGRATION") == "true" {
		t.Skip("Skipping integration test (SKIP_INTEGRATION=true)")
	}

	t.Run("load config with health check settings", func(t *testing.T) {
		configContent := `
version: "1.0"

processes:
  web-server:
    command: sleep
    args: [5]
    type: service
    health_check:
      type: http
      target: http://localhost:8080/health
      interval: 1s
      timeout: 500ms
      retries: 3
      start_period: 2s

discovery:
  enabled: false
`
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "worker-compose.yaml")
		err := os.WriteFile(configPath, []byte(configContent), 0644)
		require.NoError(t, err)

		// Load configuration
		loader := config.NewLoader(configPath)
		cfg, err := loader.Load()
		require.NoError(t, err)

		// Verify health check config was loaded
		webServer := cfg.Processes["web-server"]
		require.NotNil(t, webServer.HealthCheck)
		assert.Equal(t, config.HealthCheckHTTP, webServer.HealthCheck.Type)
		assert.Equal(t, "http://localhost:8080/health", webServer.HealthCheck.Target)
		assert.Equal(t, time.Second, webServer.HealthCheck.Interval.Duration)
		assert.Equal(t, 500*time.Millisecond, webServer.HealthCheck.Timeout.Duration)
		assert.Equal(t, 3, webServer.HealthCheck.Retries)
		assert.Equal(t, 2*time.Second, webServer.HealthCheck.StartPeriod.Duration)
	})
}

// TestFullE2E_ConfigWithEnvironment tests environment variable handling
func TestFullE2E_ConfigWithEnvironment(t *testing.T) {
	if os.Getenv("SKIP_INTEGRATION") == "true" {
		t.Skip("Skipping integration test (SKIP_INTEGRATION=true)")
	}

	t.Run("processes inherit default environment", func(t *testing.T) {
		configContent := `
version: "1.0"

defaults:
  environment:
    GLOBAL_VAR: global_value

processes:
  service-a:
    command: echo
    args: [hello]
    environment:
      LOCAL_VAR: local_value

  service-b:
    command: echo
    args: [world]

discovery:
  enabled: false
`
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "worker-compose.yaml")
		err := os.WriteFile(configPath, []byte(configContent), 0644)
		require.NoError(t, err)

		// Load configuration
		loader := config.NewLoader(configPath)
		cfg, err := loader.Load()
		require.NoError(t, err)

		// service-a should have both global and local env vars
		serviceA := cfg.Processes["service-a"]
		assert.Equal(t, "global_value", serviceA.Environment["GLOBAL_VAR"])
		assert.Equal(t, "local_value", serviceA.Environment["LOCAL_VAR"])

		// service-b should inherit global env vars
		serviceB := cfg.Processes["service-b"]
		assert.Equal(t, "global_value", serviceB.Environment["GLOBAL_VAR"])
	})
}

// TestFullE2E_ConfigValidation tests config validation
func TestFullE2E_ConfigValidation(t *testing.T) {
	if os.Getenv("SKIP_INTEGRATION") == "true" {
		t.Skip("Skipping integration test (SKIP_INTEGRATION=true)")
	}

	t.Run("detects circular dependencies", func(t *testing.T) {
		configContent := `
version: "1.0"

processes:
  a:
    command: echo
    args: [a]
    depends_on:
      - process: b
        condition: process_started
  b:
    command: echo
    args: [b]
    depends_on:
      - process: c
        condition: process_started
  c:
    command: echo
    args: [c]
    depends_on:
      - process: a
        condition: process_started

discovery:
  enabled: false
`
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "worker-compose.yaml")
		err := os.WriteFile(configPath, []byte(configContent), 0644)
		require.NoError(t, err)

		// Load should fail with circular dependency error
		loader := config.NewLoader(configPath)
		_, err = loader.Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "circular dependency")
	})

	t.Run("detects unknown dependency", func(t *testing.T) {
		configContent := `
version: "1.0"

processes:
  service:
    command: echo
    args: [test]
    depends_on:
      - process: nonexistent
        condition: process_started

discovery:
  enabled: false
`
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "worker-compose.yaml")
		err := os.WriteFile(configPath, []byte(configContent), 0644)
		require.NoError(t, err)

		loader := config.NewLoader(configPath)
		_, err = loader.Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown process")
	})

	t.Run("detects invalid health check type", func(t *testing.T) {
		configContent := `
version: "1.0"

processes:
  service:
    command: echo
    args: [test]
    health_check:
      type: invalid
      target: localhost:8080

discovery:
  enabled: false
`
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "worker-compose.yaml")
		err := os.WriteFile(configPath, []byte(configContent), 0644)
		require.NoError(t, err)

		loader := config.NewLoader(configPath)
		_, err = loader.Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid health_check.type")
	})
}

// TestFullE2E_SchedulerEvents tests that scheduler emits correct events
func TestFullE2E_SchedulerEvents(t *testing.T) {
	if os.Getenv("SKIP_INTEGRATION") == "true" {
		t.Skip("Skipping integration test (SKIP_INTEGRATION=true)")
	}

	t.Run("emits started and starting events", func(t *testing.T) {
		configContent := `
version: "1.0"

processes:
  worker:
    command: sleep
    args: [5]
    type: service

discovery:
  enabled: false
`
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "worker-compose.yaml")
		err := os.WriteFile(configPath, []byte(configContent), 0644)
		require.NoError(t, err)

		loader := config.NewLoader(configPath)
		cfg, err := loader.Load()
		require.NoError(t, err)

		manager := process.NewManager()
		defer manager.StopAll()

		graph := process.NewDependencyGraph(cfg.Processes)
		starter := func(ctx context.Context, name string, proc *config.Process) (*process.Process, error) {
			return manager.StartProcess(ctx, name, proc)
		}

		scheduler := process.NewScheduler(graph, starter)
		events := scheduler.Events()

		var receivedEvents []process.StartEvent
		done := make(chan struct{})

		go func() {
			for event := range events {
				receivedEvents = append(receivedEvents, event)
			}
			close(done)
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = scheduler.StartAll(ctx)
		require.NoError(t, err)

		time.Sleep(300 * time.Millisecond)

		// Should have starting and started events
		var hasStarting, hasStarted bool
		for _, e := range receivedEvents {
			if e.Name == "worker" && e.Type == process.EventStarting {
				hasStarting = true
			}
			if e.Name == "worker" && e.Type == process.EventStarted {
				hasStarted = true
			}
		}

		assert.True(t, hasStarting, "should have starting event")
		assert.True(t, hasStarted, "should have started event")
	})
}

// indexOf finds the first occurrence of needle in haystack
func indexOf(haystack []string, needle string) int {
	for i, s := range haystack {
		if s == needle {
			return i
		}
	}
	return -1
}
