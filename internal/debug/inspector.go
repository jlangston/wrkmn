package debug

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// InspectorTarget represents a V8 inspector target
type InspectorTarget struct {
	Description          string `json:"description"`
	DevtoolsFrontendURL  string `json:"devtoolsFrontendUrl"`
	ID                   string `json:"id"`
	Title                string `json:"title"`
	Type                 string `json:"type"`
	URL                  string `json:"url"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

// InspectorDiscovery handles discovery of V8 inspector targets
type InspectorDiscovery struct {
	client  *http.Client
	baseURL string
}

// NewInspectorDiscovery creates a new inspector discovery
func NewInspectorDiscovery(port int) *InspectorDiscovery {
	return &InspectorDiscovery{
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		baseURL: fmt.Sprintf("http://localhost:%d", port),
	}
}

// Discover returns all available inspector targets
func (d *InspectorDiscovery) Discover() ([]InspectorTarget, error) {
	resp, err := d.client.Get(d.baseURL + "/json")
	if err != nil {
		return nil, fmt.Errorf("failed to connect to inspector: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("inspector returned status %d", resp.StatusCode)
	}

	var targets []InspectorTarget
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return nil, fmt.Errorf("failed to decode targets: %w", err)
	}

	return targets, nil
}

// DiscoverFirst returns the first available target
func (d *InspectorDiscovery) DiscoverFirst() (*InspectorTarget, error) {
	targets, err := d.Discover()
	if err != nil {
		return nil, err
	}

	if len(targets) == 0 {
		return nil, fmt.Errorf("no inspector targets available")
	}

	return &targets[0], nil
}

// GetVersion returns the V8 version info
func (d *InspectorDiscovery) GetVersion() (map[string]string, error) {
	resp, err := d.client.Get(d.baseURL + "/json/version")
	if err != nil {
		return nil, fmt.Errorf("failed to get version: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("version endpoint returned status %d", resp.StatusCode)
	}

	var version map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&version); err != nil {
		return nil, fmt.Errorf("failed to decode version: %w", err)
	}

	return version, nil
}

// WaitForTarget waits for a target to become available
func (d *InspectorDiscovery) WaitForTarget(timeout time.Duration) (*InspectorTarget, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		target, err := d.DiscoverFirst()
		if err == nil {
			return target, nil
		}

		time.Sleep(100 * time.Millisecond)
	}

	return nil, fmt.Errorf("timeout waiting for inspector target")
}

// IsAvailable checks if the inspector is available
func (d *InspectorDiscovery) IsAvailable() bool {
	_, err := d.GetVersion()
	return err == nil
}

// PortAllocator manages inspector port allocation
type PortAllocator struct {
	basePort  int
	allocated map[string]int // process name -> port
}

// NewPortAllocator creates a new port allocator
func NewPortAllocator(basePort int) *PortAllocator {
	return &PortAllocator{
		basePort:  basePort,
		allocated: make(map[string]int),
	}
}

// Allocate allocates a port for a process
func (a *PortAllocator) Allocate(name string) int {
	if port, ok := a.allocated[name]; ok {
		return port
	}

	port := a.basePort + len(a.allocated)
	a.allocated[name] = port
	return port
}

// GetPort returns the allocated port for a process
func (a *PortAllocator) GetPort(name string) (int, bool) {
	port, ok := a.allocated[name]
	return port, ok
}

// Release releases the port for a process
func (a *PortAllocator) Release(name string) {
	delete(a.allocated, name)
}

// GetAll returns all allocated ports
func (a *PortAllocator) GetAll() map[string]int {
	result := make(map[string]int)
	for k, v := range a.allocated {
		result[k] = v
	}
	return result
}
