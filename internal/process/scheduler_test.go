package process

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"wrkmn/internal/config"
)

func TestStartEventType_String(t *testing.T) {
	tests := []struct {
		eventType StartEventType
		expected  string
	}{
		{EventStarting, "starting"},
		{EventStarted, "started"},
		{EventHealthy, "healthy"},
		{EventFailed, "failed"},
		{EventWaiting, "waiting"},
		{StartEventType(99), "unknown"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.eventType.String())
	}
}

func TestNewScheduler(t *testing.T) {
	processes := map[string]config.Process{
		"api": {Command: "npm"},
	}
	graph := NewDependencyGraph(processes)

	starter := func(ctx context.Context, name string, proc *config.Process) (*Process, error) {
		return nil, nil
	}

	scheduler := NewScheduler(graph, starter)

	assert.NotNil(t, scheduler)
	assert.Equal(t, graph, scheduler.graph)
	assert.NotNil(t, scheduler.events)
	assert.NotNil(t, scheduler.healthMonitors)
}

func TestScheduler_GroupByLevels(t *testing.T) {
	t.Run("all independent", func(t *testing.T) {
		processes := map[string]config.Process{
			"a": {Command: "a"},
			"b": {Command: "b"},
			"c": {Command: "c"},
		}
		graph := NewDependencyGraph(processes)
		scheduler := NewScheduler(graph, nil)

		order, _ := graph.GetStartOrder()
		levels := scheduler.groupByLevels(order)

		// All should be at level 0
		assert.Len(t, levels, 1)
		assert.Len(t, levels[0], 3)
	})

	t.Run("linear chain", func(t *testing.T) {
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
			"c": {Command: "c"},
		}
		graph := NewDependencyGraph(processes)
		scheduler := NewScheduler(graph, nil)

		order, _ := graph.GetStartOrder()
		levels := scheduler.groupByLevels(order)

		// Should be 3 levels: c, b, a
		assert.Len(t, levels, 3)
		assert.Contains(t, levels[0], "c")
		assert.Contains(t, levels[1], "b")
		assert.Contains(t, levels[2], "a")
	})

	t.Run("diamond pattern", func(t *testing.T) {
		processes := map[string]config.Process{
			"top": {Command: "top"},
			"left": {
				Command: "left",
				DependsOn: []config.Dependency{
					{Process: "top", Condition: config.ConditionStarted},
				},
			},
			"right": {
				Command: "right",
				DependsOn: []config.Dependency{
					{Process: "top", Condition: config.ConditionStarted},
				},
			},
			"bottom": {
				Command: "bottom",
				DependsOn: []config.Dependency{
					{Process: "left", Condition: config.ConditionStarted},
					{Process: "right", Condition: config.ConditionStarted},
				},
			},
		}
		graph := NewDependencyGraph(processes)
		scheduler := NewScheduler(graph, nil)

		order, _ := graph.GetStartOrder()
		levels := scheduler.groupByLevels(order)

		// Level 0: top
		// Level 1: left, right
		// Level 2: bottom
		assert.Len(t, levels, 3)
		assert.Contains(t, levels[0], "top")
		assert.Len(t, levels[1], 2)
		assert.Contains(t, levels[2], "bottom")
	})
}

func TestScheduler_StartAll(t *testing.T) {
	t.Run("starts processes in order", func(t *testing.T) {
		processes := map[string]config.Process{
			"api": {
				Command: "npm",
				DependsOn: []config.Dependency{
					{Process: "db", Condition: config.ConditionStarted},
				},
			},
			"db": {Command: "postgres"},
		}
		graph := NewDependencyGraph(processes)

		var startOrder []string
		starter := func(ctx context.Context, name string, proc *config.Process) (*Process, error) {
			startOrder = append(startOrder, name)
			return &Process{}, nil
		}

		scheduler := NewScheduler(graph, starter)

		err := scheduler.StartAll(context.Background())
		require.NoError(t, err)

		// db should start before api
		require.Len(t, startOrder, 2)
		assert.Equal(t, "db", startOrder[0])
		assert.Equal(t, "api", startOrder[1])
	})

	t.Run("handles start error", func(t *testing.T) {
		processes := map[string]config.Process{
			"api": {Command: "npm"},
		}
		graph := NewDependencyGraph(processes)

		starter := func(ctx context.Context, name string, proc *config.Process) (*Process, error) {
			return nil, errors.New("start failed")
		}

		scheduler := NewScheduler(graph, starter)

		err := scheduler.StartAll(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "starting api")
	})
}

func TestScheduler_WaitForDependency(t *testing.T) {
	t.Run("returns immediately when dependency running", func(t *testing.T) {
		processes := map[string]config.Process{
			"api": {Command: "npm"},
			"db":  {Command: "postgres"},
		}
		graph := NewDependencyGraph(processes)
		graph.UpdateState("db", StateRunning)

		scheduler := NewScheduler(graph, nil)

		dep := config.Dependency{
			Process:   "db",
			Condition: config.ConditionStarted,
		}

		err := scheduler.waitForDependency(context.Background(), dep)
		assert.NoError(t, err)
	})

	t.Run("waits for dependency to become running", func(t *testing.T) {
		processes := map[string]config.Process{
			"api": {Command: "npm"},
			"db":  {Command: "postgres"},
		}
		graph := NewDependencyGraph(processes)

		scheduler := NewScheduler(graph, nil)

		// Start a goroutine to update state after a delay
		go func() {
			time.Sleep(50 * time.Millisecond)
			graph.UpdateState("db", StateRunning)
		}()

		dep := config.Dependency{
			Process:   "db",
			Condition: config.ConditionStarted,
		}

		err := scheduler.waitForDependency(context.Background(), dep)
		assert.NoError(t, err)
	})

	t.Run("returns error when dependency fails for healthy condition", func(t *testing.T) {
		processes := map[string]config.Process{
			"api": {Command: "npm"},
			"db":  {Command: "postgres"},
		}
		graph := NewDependencyGraph(processes)
		graph.UpdateState("db", StateFailed)

		scheduler := NewScheduler(graph, nil)

		dep := config.Dependency{
			Process:   "db",
			Condition: config.ConditionHealthy, // Use healthy condition to test failure path
		}

		err := scheduler.waitForDependency(context.Background(), dep)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "dependency db failed")
	})

	t.Run("returns error for unknown dependency", func(t *testing.T) {
		processes := map[string]config.Process{
			"api": {Command: "npm"},
		}
		graph := NewDependencyGraph(processes)

		scheduler := NewScheduler(graph, nil)

		dep := config.Dependency{
			Process:   "unknown",
			Condition: config.ConditionStarted,
		}

		err := scheduler.waitForDependency(context.Background(), dep)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown dependency")
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		processes := map[string]config.Process{
			"api": {Command: "npm"},
			"db":  {Command: "postgres"},
		}
		graph := NewDependencyGraph(processes)

		scheduler := NewScheduler(graph, nil)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		dep := config.Dependency{
			Process:   "db",
			Condition: config.ConditionStarted,
		}

		err := scheduler.waitForDependency(ctx, dep)
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("returns error when dependency stopped for started condition", func(t *testing.T) {
		processes := map[string]config.Process{
			"api": {Command: "npm"},
			"db":  {Command: "postgres"},
		}
		graph := NewDependencyGraph(processes)
		graph.UpdateState("db", StateStopped)

		scheduler := NewScheduler(graph, nil)

		dep := config.Dependency{
			Process:   "db",
			Condition: config.ConditionStarted,
		}

		start := time.Now()
		err := scheduler.waitForDependency(context.Background(), dep)
		elapsed := time.Since(start)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "dependency db stopped")
		assert.Less(t, elapsed, time.Second, "should fail fast rather than polling to the 5-minute timeout")
	})

	t.Run("returns error when dependency stopped for healthy condition", func(t *testing.T) {
		processes := map[string]config.Process{
			"api": {Command: "npm"},
			"db":  {Command: "postgres"},
		}
		graph := NewDependencyGraph(processes)
		graph.UpdateState("db", StateStopped)

		scheduler := NewScheduler(graph, nil)

		dep := config.Dependency{
			Process:   "db",
			Condition: config.ConditionHealthy,
		}

		start := time.Now()
		err := scheduler.waitForDependency(context.Background(), dep)
		elapsed := time.Since(start)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "dependency db stopped")
		assert.Less(t, elapsed, time.Second, "should fail fast rather than polling to the 5-minute timeout")
	})

	t.Run("returns error when dependency stopped for completed condition", func(t *testing.T) {
		processes := map[string]config.Process{
			"app":   {Command: "npm"},
			"setup": {Command: "sh", Type: config.ProcessTypeOneshot},
		}
		graph := NewDependencyGraph(processes)
		graph.UpdateState("setup", StateStopped)

		scheduler := NewScheduler(graph, nil)

		dep := config.Dependency{
			Process:   "setup",
			Condition: config.ConditionCompleted,
		}

		start := time.Now()
		err := scheduler.waitForDependency(context.Background(), dep)
		elapsed := time.Since(start)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "dependency setup stopped")
		assert.Less(t, elapsed, time.Second, "should fail fast rather than polling to the 5-minute timeout")
	})

	t.Run("waits for healthy condition", func(t *testing.T) {
		processes := map[string]config.Process{
			"api": {Command: "npm"},
			"db":  {Command: "postgres"},
		}
		graph := NewDependencyGraph(processes)
		graph.UpdateState("db", StateRunning) // Running but not healthy

		scheduler := NewScheduler(graph, nil)

		// Start a goroutine to update health after a delay
		go func() {
			time.Sleep(50 * time.Millisecond)
			graph.UpdateHealth("db", HealthHealthy)
		}()

		dep := config.Dependency{
			Process:   "db",
			Condition: config.ConditionHealthy,
		}

		err := scheduler.waitForDependency(context.Background(), dep)
		assert.NoError(t, err)
	})
}

func TestScheduler_GetAllDependents(t *testing.T) {
	t.Run("returns direct dependents", func(t *testing.T) {
		processes := map[string]config.Process{
			"db": {Command: "postgres"},
			"api": {
				Command: "npm",
				DependsOn: []config.Dependency{
					{Process: "db", Condition: config.ConditionStarted},
				},
			},
			"worker": {
				Command: "node",
				DependsOn: []config.Dependency{
					{Process: "db", Condition: config.ConditionStarted},
				},
			},
		}
		graph := NewDependencyGraph(processes)
		scheduler := NewScheduler(graph, nil)

		dependents := scheduler.getAllDependents("db")
		assert.Len(t, dependents, 2)
		assert.Contains(t, dependents, "api")
		assert.Contains(t, dependents, "worker")
	})

	t.Run("returns transitive dependents", func(t *testing.T) {
		processes := map[string]config.Process{
			"db": {Command: "postgres"},
			"cache": {
				Command: "redis",
				DependsOn: []config.Dependency{
					{Process: "db", Condition: config.ConditionStarted},
				},
			},
			"api": {
				Command: "npm",
				DependsOn: []config.Dependency{
					{Process: "cache", Condition: config.ConditionStarted},
				},
			},
		}
		graph := NewDependencyGraph(processes)
		scheduler := NewScheduler(graph, nil)

		dependents := scheduler.getAllDependents("db")
		assert.Len(t, dependents, 2)
		assert.Contains(t, dependents, "cache")
		assert.Contains(t, dependents, "api")
	})

	t.Run("returns empty for leaf nodes", func(t *testing.T) {
		processes := map[string]config.Process{
			"db": {Command: "postgres"},
			"api": {
				Command: "npm",
				DependsOn: []config.Dependency{
					{Process: "db", Condition: config.ConditionStarted},
				},
			},
		}
		graph := NewDependencyGraph(processes)
		scheduler := NewScheduler(graph, nil)

		dependents := scheduler.getAllDependents("api")
		assert.Empty(t, dependents)
	})
}

func TestScheduler_Events(t *testing.T) {
	processes := map[string]config.Process{
		"api": {Command: "npm"},
	}
	graph := NewDependencyGraph(processes)

	starter := func(ctx context.Context, name string, proc *config.Process) (*Process, error) {
		return &Process{}, nil
	}

	scheduler := NewScheduler(graph, starter)

	// Start in background
	go func() {
		scheduler.StartAll(context.Background())
	}()

	// Should receive events
	select {
	case event := <-scheduler.Events():
		assert.Equal(t, "api", event.Name)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestScheduler_SendEvent(t *testing.T) {
	processes := map[string]config.Process{}
	graph := NewDependencyGraph(processes)
	scheduler := NewScheduler(graph, nil)

	// Send event
	scheduler.sendEvent(StartEvent{
		Name:    "test",
		Type:    EventStarting,
		Message: "test message",
	})

	// Receive event
	select {
	case event := <-scheduler.Events():
		assert.Equal(t, "test", event.Name)
		assert.Equal(t, EventStarting, event.Type)
		assert.Equal(t, "test message", event.Message)
	default:
		t.Fatal("no event received")
	}
}

func TestProcessStatus(t *testing.T) {
	ps := ProcessStatus{
		Name:   "api",
		State:  StateRunning,
		Health: HealthHealthy,
		PID:    12345,
		Uptime: 5 * time.Minute,
	}

	assert.Equal(t, "api", ps.Name)
	assert.Equal(t, StateRunning, ps.State)
	assert.Equal(t, HealthHealthy, ps.Health)
	assert.Equal(t, 12345, ps.PID)
	assert.Equal(t, 5*time.Minute, ps.Uptime)
}

func readOrder(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return strings.Fields(string(data))
}

func TestOneshotCompletionSatisfiesDependent(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "order")
	processes := map[string]config.Process{
		"setup": {
			Command: "sh",
			Args:    []string{"-c", "sleep 0.2; echo setup >> " + marker},
			Type:    config.ProcessTypeOneshot,
		},
		"app": {
			Command: "sh",
			Args:    []string{"-c", "echo app >> " + marker + "; sleep 30"},
			Type:    config.ProcessTypeService,
			DependsOn: []config.Dependency{
				{Process: "setup", Condition: config.ConditionCompleted},
			},
		},
	}
	graph := NewDependencyGraph(processes)
	m := NewManager()
	defer m.StopAll()
	sched := NewScheduler(graph, func(ctx context.Context, name string, proc *config.Process) (*Process, error) {
		return m.StartProcess(ctx, name, proc, nil)
	})

	if err := sched.StartAll(context.Background()); err != nil {
		t.Fatalf("StartAll: %v", err)
	}

	// "app" starting is async from the scheduler's perspective (no health
	// check to wait on), so its own "echo app" may not have landed in the
	// marker file the instant StartAll returns. Poll briefly rather than
	// racing a bare read.
	var order []string
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		order = readOrder(t, marker)
		if len(order) >= 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(order) != 2 || order[0] != "setup" || order[1] != "app" {
		t.Fatalf("order = %v, want [setup app]", order)
	}
	if st := graph.GetNode("setup").State; st != StateCompleted {
		t.Fatalf("setup state = %v, want StateCompleted", st)
	}
}

func TestOneshotFailureFailsDependent(t *testing.T) {
	processes := map[string]config.Process{
		"setup": {
			Command: "sh",
			Args:    []string{"-c", "exit 7"},
			Type:    config.ProcessTypeOneshot,
		},
		"app": {
			Command: "sleep",
			Args:    []string{"30"},
			Type:    config.ProcessTypeService,
			DependsOn: []config.Dependency{
				{Process: "setup", Condition: config.ConditionCompleted},
			},
		},
	}
	graph := NewDependencyGraph(processes)
	m := NewManager()
	defer m.StopAll()
	sched := NewScheduler(graph, func(ctx context.Context, name string, proc *config.Process) (*Process, error) {
		return m.StartProcess(ctx, name, proc, nil)
	})

	if err := sched.StartAll(context.Background()); err == nil {
		t.Fatal("StartAll should fail when a oneshot dependency exits non-zero")
	}
	if st := graph.GetNode("setup").State; st != StateFailed {
		t.Fatalf("setup state = %v, want StateFailed", st)
	}
	if _, exists := m.GetProcess("app"); exists {
		t.Fatal("app should not have started after its dependency failed")
	}
}
