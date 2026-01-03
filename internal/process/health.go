package process

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"wrkmn/internal/config"
)

// HealthChecker performs health checks on processes
type HealthChecker struct {
	httpClient *http.Client
}

// NewHealthChecker creates a new health checker
func NewHealthChecker() *HealthChecker {
	return &HealthChecker{
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// CheckResult represents the result of a health check
type CheckResult struct {
	Healthy bool
	Message string
	Latency time.Duration
}

// Check performs a health check based on the configuration
func (h *HealthChecker) Check(ctx context.Context, hc *config.HealthCheck) CheckResult {
	start := time.Now()

	var result CheckResult
	switch hc.Type {
	case config.HealthCheckHTTP:
		result = h.checkHTTP(ctx, hc)
	case config.HealthCheckTCP:
		result = h.checkTCP(ctx, hc)
	case config.HealthCheckExec:
		result = h.checkExec(ctx, hc)
	default:
		result = CheckResult{
			Healthy: false,
			Message: fmt.Sprintf("unknown health check type: %s", hc.Type),
		}
	}

	result.Latency = time.Since(start)
	return result
}

// checkHTTP performs an HTTP health check
func (h *HealthChecker) checkHTTP(ctx context.Context, hc *config.HealthCheck) CheckResult {
	target := hc.Target
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		target = "http://" + target
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return CheckResult{
			Healthy: false,
			Message: fmt.Sprintf("creating request: %v", err),
		}
	}

	// Use custom timeout if specified
	client := h.httpClient
	if hc.Timeout.Duration > 0 {
		client = &http.Client{
			Timeout: hc.Timeout.Duration,
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return CheckResult{
			Healthy: false,
			Message: fmt.Sprintf("request failed: %v", err),
		}
	}
	defer resp.Body.Close()

	// Check for expected status codes (2xx or 3xx by default)
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return CheckResult{
			Healthy: true,
			Message: fmt.Sprintf("HTTP %d", resp.StatusCode),
		}
	}

	return CheckResult{
		Healthy: false,
		Message: fmt.Sprintf("HTTP %d", resp.StatusCode),
	}
}

// checkTCP performs a TCP connection health check
func (h *HealthChecker) checkTCP(ctx context.Context, hc *config.HealthCheck) CheckResult {
	target := hc.Target

	// Ensure target has host:port format
	if !strings.Contains(target, ":") {
		return CheckResult{
			Healthy: false,
			Message: "TCP target must be in host:port format",
		}
	}

	timeout := hc.Timeout.Duration
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	dialer := &net.Dialer{
		Timeout: timeout,
	}

	conn, err := dialer.DialContext(ctx, "tcp", target)
	if err != nil {
		return CheckResult{
			Healthy: false,
			Message: fmt.Sprintf("connection failed: %v", err),
		}
	}
	defer conn.Close()

	return CheckResult{
		Healthy: true,
		Message: "TCP connection successful",
	}
}

// checkExec performs a command execution health check
// The Target field contains the command to execute for exec health checks
func (h *HealthChecker) checkExec(ctx context.Context, hc *config.HealthCheck) CheckResult {
	if hc.Target == "" {
		return CheckResult{
			Healthy: false,
			Message: "exec health check requires a command in target field",
		}
	}

	timeout := hc.Timeout.Duration
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Split command into parts (Target contains the command for exec checks)
	parts := strings.Fields(hc.Target)
	if len(parts) == 0 {
		return CheckResult{
			Healthy: false,
			Message: "empty command",
		}
	}

	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return CheckResult{
			Healthy: false,
			Message: fmt.Sprintf("command failed: %v (output: %s)", err, strings.TrimSpace(string(output))),
		}
	}

	return CheckResult{
		Healthy: true,
		Message: "command succeeded",
	}
}

// HealthMonitor continuously monitors health for a process
type HealthMonitor struct {
	name        string
	config      *config.HealthCheck
	checker     *HealthChecker
	graph       *DependencyGraph
	stopCh      chan struct{}
	failCount   int
	startTime   time.Time
	inStartup   bool
}

// NewHealthMonitor creates a new health monitor for a process
func NewHealthMonitor(name string, hc *config.HealthCheck, graph *DependencyGraph) *HealthMonitor {
	return &HealthMonitor{
		name:      name,
		config:    hc,
		checker:   NewHealthChecker(),
		graph:     graph,
		stopCh:    make(chan struct{}),
		startTime: time.Now(),
		inStartup: true,
	}
}

// Start begins monitoring the process health
func (m *HealthMonitor) Start(ctx context.Context) {
	m.graph.UpdateHealth(m.name, HealthChecking)

	interval := m.config.Interval.Duration
	if interval == 0 {
		interval = config.DefaultHealthCheckInterval
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.doCheck(ctx)
		}
	}
}

// doCheck performs a single health check and updates state
func (m *HealthMonitor) doCheck(ctx context.Context) {
	// Check if still in startup period
	if m.inStartup && m.config.StartPeriod.Duration > 0 {
		if time.Since(m.startTime) < m.config.StartPeriod.Duration {
			// During startup period, don't mark as unhealthy
			return
		}
		m.inStartup = false
	}

	result := m.checker.Check(ctx, m.config)

	if result.Healthy {
		m.failCount = 0
		m.graph.UpdateHealth(m.name, HealthHealthy)
	} else {
		m.failCount++
		retries := m.config.Retries
		if retries == 0 {
			retries = config.DefaultHealthCheckRetries
		}

		if m.failCount >= retries {
			m.graph.UpdateHealth(m.name, HealthUnhealthy)
		}
	}
}

// Stop stops the health monitor
func (m *HealthMonitor) Stop() {
	close(m.stopCh)
}

// WaitForHealthy blocks until the process becomes healthy or timeout
func (m *HealthMonitor) WaitForHealthy(ctx context.Context, timeout time.Duration) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timeout waiting for %s to become healthy", m.name)
		case <-ticker.C:
			node := m.graph.GetNode(m.name)
			if node != nil && node.Health == HealthHealthy {
				return nil
			}
		}
	}
}
