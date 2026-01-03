//go:build integration

package process_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"wrkmn/internal/config"
	"wrkmn/internal/process"
)

// =============================================================================
// Process Lifecycle Tests
// =============================================================================

func TestProcessLifecycle_StartStop(t *testing.T) {
	skipIfNoDocker(t)

	t.Run("start and stop simple process", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		manager := process.NewManager()
		cfg := &config.Process{
			Command: "sleep",
			Args:    []string{"60"},
		}

		proc, err := manager.StartProcess(ctx, "sleeper", cfg)
		require.NoError(t, err)
		require.NotNil(t, proc)

		// Verify process is running
		assert.True(t, proc.IsRunning())
		assert.Greater(t, proc.Pid(), 0)

		// Stop and verify
		err = manager.StopProcess("sleeper")
		require.NoError(t, err)

		// Wait for process to terminate
		time.Sleep(300 * time.Millisecond)
		assert.False(t, proc.IsRunning())
	})

	t.Run("start process with environment variables", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		manager := process.NewManager()
		cfg := &config.Process{
			Command: "sh",
			Args:    []string{"-c", "echo $TEST_VAR && sleep 5"},
			Environment: map[string]string{
				"TEST_VAR": "hello_integration_test",
			},
		}

		proc, err := manager.StartProcess(ctx, "env-test", cfg)
		require.NoError(t, err)

		// Wait for output
		time.Sleep(1 * time.Second)

		// Check logs contain env var value
		logs := proc.LogBuffer.GetLines()
		found := false
		for _, line := range logs {
			if strings.Contains(line, "hello_integration_test") {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected to find 'hello_integration_test' in logs: %v", logs)

		manager.StopProcess("env-test")
	})

	t.Run("start process with working directory", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		tmpDir := t.TempDir()

		manager := process.NewManager()
		cfg := &config.Process{
			Command:    "pwd",
			WorkingDir: tmpDir,
		}

		proc, err := manager.StartProcess(ctx, "pwd-test", cfg)
		require.NoError(t, err)

		// Wait for output
		time.Sleep(500 * time.Millisecond)

		logs := proc.LogBuffer.GetLines()
		found := false
		for _, line := range logs {
			if strings.Contains(line, tmpDir) {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected working directory in output: %v", logs)
	})
}

func TestProcessLifecycle_Restart(t *testing.T) {
	skipIfNoDocker(t)

	t.Run("restart process gets new PID", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		manager := process.NewManager()
		cfg := &config.Process{
			Command: "sh",
			Args:    []string{"-c", "echo 'started' && sleep 60"},
		}

		proc, err := manager.StartProcess(ctx, "restart-test", cfg)
		require.NoError(t, err)

		time.Sleep(500 * time.Millisecond)
		initialPid := proc.Pid()
		require.Greater(t, initialPid, 0)

		// Stop and restart
		manager.StopProcess("restart-test")
		time.Sleep(300 * time.Millisecond)

		proc2, err := manager.StartProcess(ctx, "restart-test", cfg)
		require.NoError(t, err)

		time.Sleep(500 * time.Millisecond)

		// New PID
		assert.NotEqual(t, initialPid, proc2.Pid())
		assert.True(t, proc2.IsRunning())

		manager.StopProcess("restart-test")
	})
}

func TestProcessLifecycle_StopAll(t *testing.T) {
	skipIfNoDocker(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	manager := process.NewManager()

	// Start multiple processes
	for i := 0; i < 3; i++ {
		cfg := &config.Process{
			Command: "sleep",
			Args:    []string{"60"},
		}
		_, err := manager.StartProcess(ctx, fmt.Sprintf("proc-%d", i), cfg)
		require.NoError(t, err)
	}

	// Verify all running
	procs := manager.GetAllProcesses()
	assert.Len(t, procs, 3)

	// Stop all
	manager.StopAll()

	time.Sleep(500 * time.Millisecond)

	// Verify all stopped
	procs = manager.GetAllProcesses()
	assert.Empty(t, procs)
}

// =============================================================================
// Health Check Integration Tests
// =============================================================================

func TestHealthCheck_TCP_PostgreSQL(t *testing.T) {
	skipIfNoDocker(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Start PostgreSQL container
	pg := StartPostgres(ctx, t)
	defer pg.Cleanup(ctx)

	t.Run("healthy when postgres is running", func(t *testing.T) {
		hc := process.NewHealthChecker()
		cfg := &config.HealthCheck{
			Type:    config.HealthCheckTCP,
			Target:  pg.Address(),
			Timeout: config.Duration{Duration: 5 * time.Second},
		}

		result := hc.Check(ctx, cfg)
		assert.True(t, result.Healthy, "Expected healthy, got: %s", result.Message)
		assert.Contains(t, result.Message, "TCP connection successful")
	})

	t.Run("unhealthy when port doesn't exist", func(t *testing.T) {
		hc := process.NewHealthChecker()
		cfg := &config.HealthCheck{
			Type:    config.HealthCheckTCP,
			Target:  "127.0.0.1:59999", // Non-existent port
			Timeout: config.Duration{Duration: 1 * time.Second},
		}

		result := hc.Check(ctx, cfg)
		assert.False(t, result.Healthy)
	})
}

func TestHealthCheck_HTTP_RealServer(t *testing.T) {
	skipIfNoDocker(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("healthy on 200 response", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		})
		server := StartSimpleHTTPServer(t, handler)
		defer server.Shutdown(ctx)

		hc := process.NewHealthChecker()
		cfg := &config.HealthCheck{
			Type:    config.HealthCheckHTTP,
			Target:  server.URL + "/health",
			Timeout: config.Duration{Duration: 5 * time.Second},
		}

		result := hc.Check(ctx, cfg)
		assert.True(t, result.Healthy, "Expected healthy, got: %s", result.Message)
	})

	t.Run("unhealthy on 500 response", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
		server := StartSimpleHTTPServer(t, handler)
		defer server.Shutdown(ctx)

		hc := process.NewHealthChecker()
		cfg := &config.HealthCheck{
			Type:    config.HealthCheckHTTP,
			Target:  server.URL + "/health",
			Timeout: config.Duration{Duration: 5 * time.Second},
		}

		result := hc.Check(ctx, cfg)
		assert.False(t, result.Healthy)
	})

	t.Run("healthy on redirect (3xx)", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/health" {
				http.Redirect(w, r, "/actual-health", http.StatusTemporaryRedirect)
				return
			}
			w.WriteHeader(http.StatusOK)
		})
		server := StartSimpleHTTPServer(t, handler)
		defer server.Shutdown(ctx)

		hc := process.NewHealthChecker()
		cfg := &config.HealthCheck{
			Type:    config.HealthCheckHTTP,
			Target:  server.URL + "/health",
			Timeout: config.Duration{Duration: 5 * time.Second},
		}

		result := hc.Check(ctx, cfg)
		// Redirects should be followed and result in healthy
		assert.True(t, result.Healthy, "Expected healthy after redirect, got: %s", result.Message)
	})
}

func TestHealthCheck_Exec_RealCommand(t *testing.T) {
	skipIfNoDocker(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("healthy on exit 0", func(t *testing.T) {
		hc := process.NewHealthChecker()
		cfg := &config.HealthCheck{
			Type:    config.HealthCheckExec,
			Target:  "true", // Always exits 0
			Timeout: config.Duration{Duration: 5 * time.Second},
		}

		result := hc.Check(ctx, cfg)
		assert.True(t, result.Healthy, "Expected healthy, got: %s", result.Message)
	})

	t.Run("unhealthy on exit 1", func(t *testing.T) {
		hc := process.NewHealthChecker()
		cfg := &config.HealthCheck{
			Type:    config.HealthCheckExec,
			Target:  "false", // Always exits 1
			Timeout: config.Duration{Duration: 5 * time.Second},
		}

		result := hc.Check(ctx, cfg)
		assert.False(t, result.Healthy)
	})

	t.Run("unhealthy on command not found", func(t *testing.T) {
		hc := process.NewHealthChecker()
		cfg := &config.HealthCheck{
			Type:    config.HealthCheckExec,
			Target:  "nonexistent-command-xyz-123",
			Timeout: config.Duration{Duration: 5 * time.Second},
		}

		result := hc.Check(ctx, cfg)
		assert.False(t, result.Healthy)
	})
}

// =============================================================================
// Log Capture Tests
// =============================================================================

func TestLogCapture_StdoutStderr(t *testing.T) {
	skipIfNoDocker(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("captures stdout and stderr", func(t *testing.T) {
		manager := process.NewManager()
		cfg := &config.Process{
			Command: "sh",
			Args:    []string{"-c", "echo 'stdout_message' && echo 'stderr_message' >&2 && sleep 5"},
		}

		proc, err := manager.StartProcess(ctx, "log-test", cfg)
		require.NoError(t, err)

		// Wait for output
		time.Sleep(1 * time.Second)

		logs := proc.LogBuffer.GetLines()

		hasStdout := false
		hasStderr := false
		for _, line := range logs {
			if strings.Contains(line, "stdout_message") && strings.Contains(line, "STDOUT") {
				hasStdout = true
			}
			if strings.Contains(line, "stderr_message") && strings.Contains(line, "STDERR") {
				hasStderr = true
			}
		}

		assert.True(t, hasStdout, "Expected stdout capture in: %v", logs)
		assert.True(t, hasStderr, "Expected stderr capture in: %v", logs)

		manager.StopProcess("log-test")
	})
}

func TestLogCapture_CircularBuffer(t *testing.T) {
	skipIfNoDocker(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("buffer wraps at capacity", func(t *testing.T) {
		manager := process.NewManager()
		cfg := &config.Process{
			Command: "sh",
			Args:    []string{"-c", "for i in $(seq 1 1500); do echo \"line_$i\"; done && sleep 5"},
		}

		proc, err := manager.StartProcess(ctx, "buffer-test", cfg)
		require.NoError(t, err)

		// Wait for output
		time.Sleep(3 * time.Second)

		logs := proc.LogBuffer.GetLines()

		// Buffer capacity is 1000, so we should have at most 1000 lines
		assert.LessOrEqual(t, len(logs), 1000, "Buffer should wrap at 1000 lines")

		// Should have some lines (not empty)
		assert.Greater(t, len(logs), 0, "Should have captured some lines")

		manager.StopProcess("buffer-test")
	})
}

func TestLogCapture_ContinuousOutput(t *testing.T) {
	skipIfNoDocker(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("captures continuous output", func(t *testing.T) {
		manager := process.NewManager()
		cfg := &config.Process{
			Command: "sh",
			Args:    []string{"-c", "while true; do date; sleep 0.2; done"},
		}

		proc, err := manager.StartProcess(ctx, "continuous-test", cfg)
		require.NoError(t, err)

		// Check logs grow over time
		time.Sleep(1 * time.Second)
		logs1 := proc.LogBuffer.GetLines()
		initialCount := len(logs1)

		time.Sleep(1 * time.Second)
		logs2 := proc.LogBuffer.GetLines()
		finalCount := len(logs2)

		assert.Greater(t, finalCount, initialCount, "Expected more logs over time")

		manager.StopProcess("continuous-test")
	})
}

// =============================================================================
// Dependency Ordering Tests
// =============================================================================

func TestDependencyOrdering_SimpleChain(t *testing.T) {
	skipIfNoDocker(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	t.Run("processes start in dependency order", func(t *testing.T) {
		// Start postgres container for health check
		pg := StartPostgres(ctx, t)
		defer pg.Cleanup(ctx)

		processes := map[string]config.Process{
			"db": {
				Command: "echo",
				Args:    []string{"db started"},
				HealthCheck: &config.HealthCheck{
					Type:     config.HealthCheckTCP,
					Target:   pg.Address(),
					Interval: config.Duration{Duration: 500 * time.Millisecond},
					Retries:  3,
				},
			},
			"api": {
				Command: "echo",
				Args:    []string{"api started"},
				DependsOn: []config.Dependency{
					{Process: "db", Condition: config.ConditionStarted},
				},
			},
		}

		graph := process.NewDependencyGraph(processes)
		manager := process.NewManager()

		var startOrder []string
		starter := func(ctx context.Context, name string, proc *config.Process) (*process.Process, error) {
			startOrder = append(startOrder, name)
			return manager.StartProcess(ctx, name, proc)
		}

		scheduler := process.NewScheduler(graph, starter)

		err := scheduler.StartAll(ctx)
		require.NoError(t, err)

		// db should start before api
		require.Len(t, startOrder, 2)
		assert.Equal(t, "db", startOrder[0])
		assert.Equal(t, "api", startOrder[1])

		manager.StopAll()
	})
}

func TestDependencyOrdering_DiamondPattern(t *testing.T) {
	skipIfNoDocker(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	t.Run("diamond dependency resolves correctly", func(t *testing.T) {
		processes := map[string]config.Process{
			"base": {
				Command:      "sleep",
				Args:         []string{"60"},
				StartupDelay: config.Duration{Duration: 100 * time.Millisecond},
			},
			"left": {
				Command:      "sleep",
				Args:         []string{"60"},
				StartupDelay: config.Duration{Duration: 100 * time.Millisecond},
				DependsOn: []config.Dependency{
					{Process: "base", Condition: config.ConditionStarted},
				},
			},
			"right": {
				Command:      "sleep",
				Args:         []string{"60"},
				StartupDelay: config.Duration{Duration: 100 * time.Millisecond},
				DependsOn: []config.Dependency{
					{Process: "base", Condition: config.ConditionStarted},
				},
			},
			"top": {
				Command:      "sleep",
				Args:         []string{"60"},
				StartupDelay: config.Duration{Duration: 100 * time.Millisecond},
				DependsOn: []config.Dependency{
					{Process: "left", Condition: config.ConditionStarted},
					{Process: "right", Condition: config.ConditionStarted},
				},
			},
		}

		graph := process.NewDependencyGraph(processes)
		manager := process.NewManager()

		var startOrder []string
		starter := func(ctx context.Context, name string, proc *config.Process) (*process.Process, error) {
			startOrder = append(startOrder, name)
			return manager.StartProcess(ctx, name, proc)
		}

		scheduler := process.NewScheduler(graph, starter)

		err := scheduler.StartAll(ctx)
		require.NoError(t, err)

		// base must be first, top must be last
		require.Len(t, startOrder, 4)
		assert.Equal(t, "base", startOrder[0])
		assert.Equal(t, "top", startOrder[3])

		// left and right should be in the middle (order between them doesn't matter)
		middle := startOrder[1:3]
		assert.Contains(t, middle, "left")
		assert.Contains(t, middle, "right")

		manager.StopAll()
	})
}
