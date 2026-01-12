package process

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"wrkmn/internal/config"
)

func TestNewHealthChecker(t *testing.T) {
	hc := NewHealthChecker()
	assert.NotNil(t, hc)
	assert.NotNil(t, hc.httpClient)
}

func TestHealthChecker_CheckHTTP(t *testing.T) {
	t.Run("healthy on 200", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		hc := NewHealthChecker()
		cfg := &config.HealthCheck{
			Type:   config.HealthCheckHTTP,
			Target: server.URL,
		}

		result := hc.Check(context.Background(), cfg)
		assert.True(t, result.Healthy)
		assert.Contains(t, result.Message, "HTTP 200")
		assert.Greater(t, result.Latency, time.Duration(0))
	})

	t.Run("healthy on 3xx", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusFound)
		}))
		defer server.Close()

		hc := NewHealthChecker()
		cfg := &config.HealthCheck{
			Type:   config.HealthCheckHTTP,
			Target: server.URL,
		}

		result := hc.Check(context.Background(), cfg)
		assert.True(t, result.Healthy)
	})

	t.Run("unhealthy on 4xx", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		hc := NewHealthChecker()
		cfg := &config.HealthCheck{
			Type:   config.HealthCheckHTTP,
			Target: server.URL,
		}

		result := hc.Check(context.Background(), cfg)
		assert.False(t, result.Healthy)
		assert.Contains(t, result.Message, "HTTP 404")
	})

	t.Run("unhealthy on 5xx", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		hc := NewHealthChecker()
		cfg := &config.HealthCheck{
			Type:   config.HealthCheckHTTP,
			Target: server.URL,
		}

		result := hc.Check(context.Background(), cfg)
		assert.False(t, result.Healthy)
		assert.Contains(t, result.Message, "HTTP 500")
	})

	t.Run("unhealthy on connection refused", func(t *testing.T) {
		hc := NewHealthChecker()
		cfg := &config.HealthCheck{
			Type:    config.HealthCheckHTTP,
			Target:  "http://localhost:59999",
			Timeout: config.Duration{Duration: 500 * time.Millisecond},
		}

		result := hc.Check(context.Background(), cfg)
		assert.False(t, result.Healthy)
		assert.Contains(t, result.Message, "request failed")
	})

	t.Run("adds http prefix", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		hc := NewHealthChecker()
		// Remove http:// prefix
		target := server.URL[7:] // Remove "http://"
		cfg := &config.HealthCheck{
			Type:   config.HealthCheckHTTP,
			Target: target,
		}

		result := hc.Check(context.Background(), cfg)
		assert.True(t, result.Healthy)
	})

	t.Run("respects custom timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		hc := NewHealthChecker()
		cfg := &config.HealthCheck{
			Type:    config.HealthCheckHTTP,
			Target:  server.URL,
			Timeout: config.Duration{Duration: 50 * time.Millisecond},
		}

		result := hc.Check(context.Background(), cfg)
		assert.False(t, result.Healthy)
		assert.Contains(t, result.Message, "request failed")
	})
}

func TestHealthChecker_CheckTCP(t *testing.T) {
	t.Run("healthy on successful connection", func(t *testing.T) {
		// Start a TCP server
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer listener.Close()

		go func() {
			conn, _ := listener.Accept()
			if conn != nil {
				conn.Close()
			}
		}()

		hc := NewHealthChecker()
		cfg := &config.HealthCheck{
			Type:   config.HealthCheckTCP,
			Target: listener.Addr().String(),
		}

		result := hc.Check(context.Background(), cfg)
		assert.True(t, result.Healthy)
		assert.Contains(t, result.Message, "TCP connection successful")
	})

	t.Run("unhealthy on connection refused", func(t *testing.T) {
		hc := NewHealthChecker()
		cfg := &config.HealthCheck{
			Type:    config.HealthCheckTCP,
			Target:  "127.0.0.1:59999",
			Timeout: config.Duration{Duration: 500 * time.Millisecond},
		}

		result := hc.Check(context.Background(), cfg)
		assert.False(t, result.Healthy)
		assert.Contains(t, result.Message, "connection failed")
	})

	t.Run("rejects invalid format", func(t *testing.T) {
		hc := NewHealthChecker()
		cfg := &config.HealthCheck{
			Type:   config.HealthCheckTCP,
			Target: "localhost", // Missing port
		}

		result := hc.Check(context.Background(), cfg)
		assert.False(t, result.Healthy)
		assert.Contains(t, result.Message, "host:port format")
	})
}

func TestHealthChecker_CheckExec(t *testing.T) {
	t.Run("healthy on exit 0", func(t *testing.T) {
		hc := NewHealthChecker()
		cfg := &config.HealthCheck{
			Type:   config.HealthCheckExec,
			Target: "true", // Unix command that always exits 0
		}

		result := hc.Check(context.Background(), cfg)
		assert.True(t, result.Healthy)
		assert.Contains(t, result.Message, "command succeeded")
	})

	t.Run("unhealthy on non-zero exit", func(t *testing.T) {
		hc := NewHealthChecker()
		cfg := &config.HealthCheck{
			Type:   config.HealthCheckExec,
			Target: "false", // Unix command that always exits 1
		}

		result := hc.Check(context.Background(), cfg)
		assert.False(t, result.Healthy)
		assert.Contains(t, result.Message, "command failed")
	})

	t.Run("rejects empty command", func(t *testing.T) {
		hc := NewHealthChecker()
		cfg := &config.HealthCheck{
			Type:   config.HealthCheckExec,
			Target: "",
		}

		result := hc.Check(context.Background(), cfg)
		assert.False(t, result.Healthy)
		assert.Contains(t, result.Message, "requires a command")
	})

	t.Run("handles command with arguments", func(t *testing.T) {
		hc := NewHealthChecker()
		cfg := &config.HealthCheck{
			Type:   config.HealthCheckExec,
			Target: "echo hello",
		}

		result := hc.Check(context.Background(), cfg)
		assert.True(t, result.Healthy)
	})
}

func TestHealthChecker_UnknownType(t *testing.T) {
	hc := NewHealthChecker()
	cfg := &config.HealthCheck{
		Type:   "unknown",
		Target: "test",
	}

	result := hc.Check(context.Background(), cfg)
	assert.False(t, result.Healthy)
	assert.Contains(t, result.Message, "unknown health check type")
}

func TestNewHealthMonitor(t *testing.T) {
	processes := map[string]config.Process{
		"api": {Command: "npm"},
	}
	graph := NewDependencyGraph(processes)

	hc := &config.HealthCheck{
		Type:   config.HealthCheckHTTP,
		Target: "http://localhost:3000",
	}

	monitor := NewHealthMonitor("api", hc, graph)

	assert.Equal(t, "api", monitor.name)
	assert.Equal(t, hc, monitor.config)
	assert.NotNil(t, monitor.checker)
	assert.NotNil(t, monitor.graph)
	assert.True(t, monitor.inStartup)
}

func TestHealthMonitor_DoCheck(t *testing.T) {
	t.Run("updates health on successful check", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		processes := map[string]config.Process{
			"api": {Command: "npm"},
		}
		graph := NewDependencyGraph(processes)

		hc := &config.HealthCheck{
			Type:   config.HealthCheckHTTP,
			Target: server.URL,
		}

		monitor := NewHealthMonitor("api", hc, graph)
		monitor.inStartup = false // Skip startup period

		monitor.doCheck(context.Background())

		assert.Equal(t, HealthHealthy, graph.GetNode("api").Health)
		assert.Equal(t, int32(0), monitor.failCount.Load())
	})

	t.Run("increments fail count on failed check", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		processes := map[string]config.Process{
			"api": {Command: "npm"},
		}
		graph := NewDependencyGraph(processes)

		hc := &config.HealthCheck{
			Type:    config.HealthCheckHTTP,
			Target:  server.URL,
			Retries: 3,
		}

		monitor := NewHealthMonitor("api", hc, graph)
		monitor.inStartup = false

		// First failure
		monitor.doCheck(context.Background())
		assert.Equal(t, int32(1), monitor.failCount.Load())
		assert.NotEqual(t, HealthUnhealthy, graph.GetNode("api").Health)

		// Second failure
		monitor.doCheck(context.Background())
		assert.Equal(t, int32(2), monitor.failCount.Load())
		assert.NotEqual(t, HealthUnhealthy, graph.GetNode("api").Health)

		// Third failure - should mark unhealthy
		monitor.doCheck(context.Background())
		assert.Equal(t, int32(3), monitor.failCount.Load())
		assert.Equal(t, HealthUnhealthy, graph.GetNode("api").Health)
	})

	t.Run("resets fail count on recovery", func(t *testing.T) {
		callCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			if callCount < 3 {
				w.WriteHeader(http.StatusInternalServerError)
			} else {
				w.WriteHeader(http.StatusOK)
			}
		}))
		defer server.Close()

		processes := map[string]config.Process{
			"api": {Command: "npm"},
		}
		graph := NewDependencyGraph(processes)

		hc := &config.HealthCheck{
			Type:   config.HealthCheckHTTP,
			Target: server.URL,
		}

		monitor := NewHealthMonitor("api", hc, graph)
		monitor.inStartup = false

		// Two failures
		monitor.doCheck(context.Background())
		monitor.doCheck(context.Background())
		assert.Equal(t, int32(2), monitor.failCount.Load())

		// Recovery
		monitor.doCheck(context.Background())
		assert.Equal(t, int32(0), monitor.failCount.Load())
		assert.Equal(t, HealthHealthy, graph.GetNode("api").Health)
	})

	t.Run("skips check during startup period", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		processes := map[string]config.Process{
			"api": {Command: "npm"},
		}
		graph := NewDependencyGraph(processes)

		hc := &config.HealthCheck{
			Type:        config.HealthCheckHTTP,
			Target:      server.URL,
			StartPeriod: config.Duration{Duration: 1 * time.Hour},
		}

		monitor := NewHealthMonitor("api", hc, graph)
		// inStartup is true by default

		monitor.doCheck(context.Background())

		// Should not increment fail count during startup
		assert.Equal(t, int32(0), monitor.failCount.Load())
	})
}

func TestHealthMonitor_WaitForHealthy(t *testing.T) {
	t.Run("returns immediately if already healthy", func(t *testing.T) {
		processes := map[string]config.Process{
			"api": {Command: "npm"},
		}
		graph := NewDependencyGraph(processes)
		graph.UpdateHealth("api", HealthHealthy)

		hc := &config.HealthCheck{
			Type:   config.HealthCheckHTTP,
			Target: "http://localhost:3000",
		}

		monitor := NewHealthMonitor("api", hc, graph)

		err := monitor.WaitForHealthy(context.Background(), 1*time.Second)
		assert.NoError(t, err)
	})

	t.Run("returns error on timeout", func(t *testing.T) {
		processes := map[string]config.Process{
			"api": {Command: "npm"},
		}
		graph := NewDependencyGraph(processes)
		graph.UpdateHealth("api", HealthChecking)

		hc := &config.HealthCheck{
			Type:   config.HealthCheckHTTP,
			Target: "http://localhost:3000",
		}

		monitor := NewHealthMonitor("api", hc, graph)

		err := monitor.WaitForHealthy(context.Background(), 100*time.Millisecond)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "timeout")
	})

	t.Run("returns error on context cancel", func(t *testing.T) {
		processes := map[string]config.Process{
			"api": {Command: "npm"},
		}
		graph := NewDependencyGraph(processes)
		graph.UpdateHealth("api", HealthChecking)

		hc := &config.HealthCheck{
			Type:   config.HealthCheckHTTP,
			Target: "http://localhost:3000",
		}

		monitor := NewHealthMonitor("api", hc, graph)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err := monitor.WaitForHealthy(ctx, 1*time.Second)
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})
}

func TestCheckResult(t *testing.T) {
	result := CheckResult{
		Healthy: true,
		Message: "OK",
		Latency: 50 * time.Millisecond,
	}

	assert.True(t, result.Healthy)
	assert.Equal(t, "OK", result.Message)
	assert.Equal(t, 50*time.Millisecond, result.Latency)
}

// Bug fix tests

func TestHealthMonitor_Stop_MultipleCalls(t *testing.T) {
	// Test that Stop() can be called multiple times without panicking
	// This was a bug fix - previously it would panic on double close
	processes := map[string]config.Process{
		"api": {Command: "npm"},
	}
	graph := NewDependencyGraph(processes)

	hc := &config.HealthCheck{
		Type:   config.HealthCheckHTTP,
		Target: "http://localhost:3000",
	}

	monitor := NewHealthMonitor("api", hc, graph)

	// Should not panic on multiple calls
	assert.NotPanics(t, func() {
		monitor.Stop()
		monitor.Stop()
		monitor.Stop()
	})
}

func TestHealthMonitor_FailCount_ThreadSafe(t *testing.T) {
	// Test that failCount operations are thread-safe
	// This was a bug fix - failCount is now atomic
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Alternate between success and failure
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	processes := map[string]config.Process{
		"api": {Command: "npm"},
	}
	graph := NewDependencyGraph(processes)

	hc := &config.HealthCheck{
		Type:    config.HealthCheckHTTP,
		Target:  server.URL,
		Retries: 100, // High retry count so we don't trigger unhealthy
	}

	monitor := NewHealthMonitor("api", hc, graph)
	monitor.inStartup = false

	// Run multiple concurrent doCheck calls
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				monitor.doCheck(context.Background())
			}
		}()
	}

	// Should not panic or have race conditions
	assert.NotPanics(t, func() {
		wg.Wait()
	})

	// Fail count should be consistent (100 total checks)
	assert.Equal(t, int32(100), monitor.failCount.Load())
}
