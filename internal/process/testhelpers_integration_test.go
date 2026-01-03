//go:build integration

package process_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// skipIfNoDocker skips the test if Docker is not available
func skipIfNoDocker(t *testing.T) {
	t.Helper()
	if os.Getenv("SKIP_INTEGRATION") == "true" {
		t.Skip("Skipping integration test (SKIP_INTEGRATION=true)")
	}

	cmd := exec.Command("docker", "info")
	if err := cmd.Run(); err != nil {
		t.Skip("Docker not available, skipping integration test")
	}
}

// PostgresContainer wraps a testcontainers postgres instance
type PostgresContainer struct {
	Container testcontainers.Container
	Host      string
	Port      string
	DSN       string
}

// StartPostgres starts a PostgreSQL container for testing
func StartPostgres(ctx context.Context, t *testing.T) *PostgresContainer {
	t.Helper()

	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err)

	host, err := pgContainer.Host(ctx)
	require.NoError(t, err)

	port, err := pgContainer.MappedPort(ctx, "5432")
	require.NoError(t, err)

	return &PostgresContainer{
		Container: pgContainer,
		Host:      host,
		Port:      port.Port(),
		DSN:       fmt.Sprintf("postgres://testuser:testpass@%s:%s/testdb?sslmode=disable", host, port.Port()),
	}
}

// Cleanup stops and removes the container
func (p *PostgresContainer) Cleanup(ctx context.Context) error {
	if p.Container != nil {
		return p.Container.Terminate(ctx)
	}
	return nil
}

// Address returns host:port for TCP health checks
func (p *PostgresContainer) Address() string {
	return fmt.Sprintf("%s:%s", p.Host, p.Port)
}

// SimpleHTTPServer creates a simple HTTP server for health check testing
type SimpleHTTPServer struct {
	Server   *http.Server
	URL      string
	Port     int
	listener net.Listener
}

// StartSimpleHTTPServer starts an HTTP server on a random port
func StartSimpleHTTPServer(t *testing.T, handler http.Handler) *SimpleHTTPServer {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	port := listener.Addr().(*net.TCPAddr).Port

	server := &http.Server{Handler: handler}

	go func() {
		server.Serve(listener)
	}()

	return &SimpleHTTPServer{
		Server:   server,
		URL:      fmt.Sprintf("http://127.0.0.1:%d", port),
		Port:     port,
		listener: listener,
	}
}

// Shutdown stops the HTTP server
func (s *SimpleHTTPServer) Shutdown(ctx context.Context) error {
	return s.Server.Shutdown(ctx)
}

// waitForTCP waits for a TCP port to be available
func waitForTCP(t *testing.T, address string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("TCP port %s not available after %v", address, timeout)
}

// waitForHTTP waits for an HTTP endpoint to return 2xx
func waitForHTTP(t *testing.T, url string, timeout time.Duration) {
	t.Helper()

	client := &http.Client{Timeout: 1 * time.Second}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 400 {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("HTTP endpoint %s not healthy after %v", url, timeout)
}

// createTempConfigFile creates a temporary config file for testing
func createTempConfigFile(t *testing.T, content string) string {
	t.Helper()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "worker-compose.yaml")

	err := os.WriteFile(configPath, []byte(content), 0644)
	require.NoError(t, err)

	return configPath
}
