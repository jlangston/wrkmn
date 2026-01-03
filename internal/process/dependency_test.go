package process

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"wrkmn/internal/config"
)

func TestProcessState_String(t *testing.T) {
	tests := []struct {
		state    ProcessState
		expected string
	}{
		{StatePending, "pending"},
		{StateStarting, "starting"},
		{StateRunning, "running"},
		{StateStopped, "stopped"},
		{StateFailed, "failed"},
		{ProcessState(99), "unknown"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.state.String())
	}
}

func TestHealthState_String(t *testing.T) {
	tests := []struct {
		state    HealthState
		expected string
	}{
		{HealthUnknown, "unknown"},
		{HealthChecking, "checking"},
		{HealthHealthy, "healthy"},
		{HealthUnhealthy, "unhealthy"},
		{HealthState(99), "unknown"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.state.String())
	}
}

func TestNewDependencyGraph(t *testing.T) {
	processes := map[string]config.Process{
		"api": {
			Command: "npm",
			DependsOn: []config.Dependency{
				{Process: "db", Condition: config.ConditionHealthy},
			},
		},
		"db": {
			Command: "postgres",
		},
	}

	g := NewDependencyGraph(processes)

	assert.NotNil(t, g.GetNode("api"))
	assert.NotNil(t, g.GetNode("db"))
	assert.Nil(t, g.GetNode("unknown"))

	// Check edges
	assert.Contains(t, g.edges["api"], "db")
	assert.Contains(t, g.reversed["db"], "api")
}

func TestDependencyGraph_GetAllNodes(t *testing.T) {
	processes := map[string]config.Process{
		"a": {Command: "a"},
		"b": {Command: "b"},
		"c": {Command: "c"},
	}

	g := NewDependencyGraph(processes)
	nodes := g.GetAllNodes()

	assert.Len(t, nodes, 3)
}

func TestDependencyGraph_UpdateState(t *testing.T) {
	processes := map[string]config.Process{
		"api": {Command: "npm"},
	}

	g := NewDependencyGraph(processes)
	assert.Equal(t, StatePending, g.GetNode("api").State)

	g.UpdateState("api", StateRunning)
	assert.Equal(t, StateRunning, g.GetNode("api").State)

	// Unknown process should not panic
	g.UpdateState("unknown", StateRunning)
}

func TestDependencyGraph_UpdateHealth(t *testing.T) {
	processes := map[string]config.Process{
		"api": {Command: "npm"},
	}

	g := NewDependencyGraph(processes)
	assert.Equal(t, HealthUnknown, g.GetNode("api").Health)

	g.UpdateHealth("api", HealthHealthy)
	assert.Equal(t, HealthHealthy, g.GetNode("api").Health)

	// Unknown process should not panic
	g.UpdateHealth("unknown", HealthHealthy)
}

func TestDependencyGraph_GetStartOrder_Linear(t *testing.T) {
	// Linear: c -> b -> a (a depends on b, b depends on c)
	processes := map[string]config.Process{
		"a": {
			Command: "a",
			DependsOn: []config.Dependency{
				{Process: "b", Condition: config.ConditionStarted},
			},
		},
		"b": {
			Command: "b",
			DependsOn: []config.Dependency{
				{Process: "c", Condition: config.ConditionStarted},
			},
		},
		"c": {
			Command: "c",
		},
	}

	g := NewDependencyGraph(processes)
	order, err := g.GetStartOrder()

	require.NoError(t, err)
	require.Len(t, order, 3)

	// c must come before b, b must come before a
	cIdx := indexOf(order, "c")
	bIdx := indexOf(order, "b")
	aIdx := indexOf(order, "a")

	assert.Less(t, cIdx, bIdx)
	assert.Less(t, bIdx, aIdx)
}

func TestDependencyGraph_GetStartOrder_Diamond(t *testing.T) {
	// Diamond: d depends on b,c; b and c depend on a
	processes := map[string]config.Process{
		"a": {Command: "a"},
		"b": {
			Command: "b",
			DependsOn: []config.Dependency{
				{Process: "a", Condition: config.ConditionStarted},
			},
		},
		"c": {
			Command: "c",
			DependsOn: []config.Dependency{
				{Process: "a", Condition: config.ConditionStarted},
			},
		},
		"d": {
			Command: "d",
			DependsOn: []config.Dependency{
				{Process: "b", Condition: config.ConditionStarted},
				{Process: "c", Condition: config.ConditionStarted},
			},
		},
	}

	g := NewDependencyGraph(processes)
	order, err := g.GetStartOrder()

	require.NoError(t, err)
	require.Len(t, order, 4)

	aIdx := indexOf(order, "a")
	bIdx := indexOf(order, "b")
	cIdx := indexOf(order, "c")
	dIdx := indexOf(order, "d")

	// a must come first
	assert.Less(t, aIdx, bIdx)
	assert.Less(t, aIdx, cIdx)
	// d must come last
	assert.Less(t, bIdx, dIdx)
	assert.Less(t, cIdx, dIdx)
}

func TestDependencyGraph_GetStartOrder_Isolated(t *testing.T) {
	// No dependencies - all can run in parallel
	processes := map[string]config.Process{
		"a": {Command: "a"},
		"b": {Command: "b"},
		"c": {Command: "c"},
	}

	g := NewDependencyGraph(processes)
	order, err := g.GetStartOrder()

	require.NoError(t, err)
	assert.Len(t, order, 3)
	assert.Contains(t, order, "a")
	assert.Contains(t, order, "b")
	assert.Contains(t, order, "c")
}

func TestDependencyGraph_GetStopOrder(t *testing.T) {
	// Linear: a -> b (a depends on b)
	processes := map[string]config.Process{
		"a": {
			Command: "a",
			DependsOn: []config.Dependency{
				{Process: "b", Condition: config.ConditionStarted},
			},
		},
		"b": {
			Command: "b",
		},
	}

	g := NewDependencyGraph(processes)
	order, err := g.GetStopOrder()

	require.NoError(t, err)
	require.Len(t, order, 2)

	// Stop order is reverse of start order
	// a should stop before b
	assert.Equal(t, "a", order[0])
	assert.Equal(t, "b", order[1])
}

func TestDependencyGraph_GetReadyToStart(t *testing.T) {
	processes := map[string]config.Process{
		"api": {
			Command: "npm",
			DependsOn: []config.Dependency{
				{Process: "db", Condition: config.ConditionHealthy},
			},
		},
		"db": {
			Command: "postgres",
		},
	}

	g := NewDependencyGraph(processes)

	// Initially, only db should be ready (api depends on db being healthy)
	ready := g.GetReadyToStart()
	assert.Contains(t, ready, "db")
	assert.NotContains(t, ready, "api")

	// After db is running, api still not ready (needs healthy)
	g.UpdateState("db", StateRunning)
	ready = g.GetReadyToStart()
	assert.NotContains(t, ready, "api")

	// After db is healthy, api is ready
	g.UpdateHealth("db", HealthHealthy)
	ready = g.GetReadyToStart()
	assert.Contains(t, ready, "api")

	// After api starts, neither is ready
	g.UpdateState("api", StateRunning)
	ready = g.GetReadyToStart()
	assert.Empty(t, ready)
}

func TestDependencyGraph_GetDependents(t *testing.T) {
	processes := map[string]config.Process{
		"api": {
			Command: "npm",
			DependsOn: []config.Dependency{
				{Process: "db", Condition: config.ConditionStarted},
			},
		},
		"worker": {
			Command: "wrangler",
			DependsOn: []config.Dependency{
				{Process: "db", Condition: config.ConditionStarted},
			},
		},
		"db": {
			Command: "postgres",
		},
	}

	g := NewDependencyGraph(processes)
	dependents := g.GetDependents("db")

	assert.Len(t, dependents, 2)
	assert.Contains(t, dependents, "api")
	assert.Contains(t, dependents, "worker")
}

func TestDependencyGraph_GetDependencies(t *testing.T) {
	processes := map[string]config.Process{
		"api": {
			Command: "npm",
			DependsOn: []config.Dependency{
				{Process: "db", Condition: config.ConditionStarted},
				{Process: "cache", Condition: config.ConditionHealthy},
			},
		},
		"db":    {Command: "postgres"},
		"cache": {Command: "redis"},
	}

	g := NewDependencyGraph(processes)
	deps := g.GetDependencies("api")

	assert.Len(t, deps, 2)
	assert.Contains(t, deps, "db")
	assert.Contains(t, deps, "cache")
}

func TestDependencyGraph_DependenciesSatisfied_Started(t *testing.T) {
	processes := map[string]config.Process{
		"api": {
			Command: "npm",
			DependsOn: []config.Dependency{
				{Process: "db", Condition: config.ConditionStarted},
			},
		},
		"db": {Command: "postgres"},
	}

	g := NewDependencyGraph(processes)

	// Initially not satisfied
	ready := g.GetReadyToStart()
	assert.NotContains(t, ready, "api")

	// StateStarting is not enough
	g.UpdateState("db", StateStarting)
	ready = g.GetReadyToStart()
	assert.NotContains(t, ready, "api")

	// StateRunning satisfies process_started
	g.UpdateState("db", StateRunning)
	ready = g.GetReadyToStart()
	assert.Contains(t, ready, "api")
}

func TestDependencyGraph_DependenciesSatisfied_Healthy(t *testing.T) {
	processes := map[string]config.Process{
		"api": {
			Command: "npm",
			DependsOn: []config.Dependency{
				{Process: "db", Condition: config.ConditionHealthy},
			},
		},
		"db": {Command: "postgres"},
	}

	g := NewDependencyGraph(processes)

	// Running but not healthy is not enough
	g.UpdateState("db", StateRunning)
	g.UpdateHealth("db", HealthChecking)
	ready := g.GetReadyToStart()
	assert.NotContains(t, ready, "api")

	// Unhealthy is not enough
	g.UpdateHealth("db", HealthUnhealthy)
	ready = g.GetReadyToStart()
	assert.NotContains(t, ready, "api")

	// Healthy satisfies process_healthy
	g.UpdateHealth("db", HealthHealthy)
	ready = g.GetReadyToStart()
	assert.Contains(t, ready, "api")
}

// Helper function
func indexOf(slice []string, item string) int {
	for i, s := range slice {
		if s == item {
			return i
		}
	}
	return -1
}
