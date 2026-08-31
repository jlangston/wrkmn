package process

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"wrkmn/internal/config"
)

// StartEvent represents a process lifecycle event
type StartEvent struct {
	Name    string
	Type    StartEventType
	Message string
	Error   error
}

// StartEventType categorizes startup events
type StartEventType int

const (
	EventStarting StartEventType = iota
	EventStarted
	EventHealthy
	EventFailed
	EventWaiting
	EventCompleted
)

func (t StartEventType) String() string {
	switch t {
	case EventStarting:
		return "starting"
	case EventStarted:
		return "started"
	case EventHealthy:
		return "healthy"
	case EventFailed:
		return "failed"
	case EventWaiting:
		return "waiting"
	case EventCompleted:
		return "completed"
	default:
		return "unknown"
	}
}

// ProcessStarter is a function that starts a process and returns when it's running
type ProcessStarter func(ctx context.Context, name string, proc *config.Process) (*Process, error)

// Scheduler orchestrates process startup based on dependencies
type Scheduler struct {
	graph          *DependencyGraph
	starter        ProcessStarter
	healthMonitors map[string]*HealthMonitor
	events         chan StartEvent
	mu             sync.Mutex
}

// NewScheduler creates a new process scheduler
func NewScheduler(graph *DependencyGraph, starter ProcessStarter) *Scheduler {
	return &Scheduler{
		graph:          graph,
		starter:        starter,
		healthMonitors: make(map[string]*HealthMonitor),
		events:         make(chan StartEvent, 100),
	}
}

// Events returns the channel for receiving startup events
func (s *Scheduler) Events() <-chan StartEvent {
	return s.events
}

// CloseEvents closes the event channel so range-consumers terminate. Call only
// after StartAll has returned and no further scheduler operations will run.
func (s *Scheduler) CloseEvents() {
	close(s.events)
}

// StartAll starts all processes in dependency order
func (s *Scheduler) StartAll(ctx context.Context) error {
	// Get topological order
	order, err := s.graph.GetStartOrder()
	if err != nil {
		return fmt.Errorf("calculating start order: %w", err)
	}

	// Group processes by their dependency level for parallel startup
	levels := s.groupByLevels(order)

	for _, level := range levels {
		if err := s.startLevel(ctx, level); err != nil {
			return err
		}
	}

	return nil
}

// groupByLevels groups processes by dependency level for parallel startup
func (s *Scheduler) groupByLevels(order []string) [][]string {
	// Calculate levels (processes with no deps = level 0, etc.)
	levels := make(map[string]int)

	for _, name := range order {
		deps := s.graph.GetDependencies(name)
		if len(deps) == 0 {
			levels[name] = 0
		} else {
			maxLevel := 0
			for _, dep := range deps {
				if l, ok := levels[dep]; ok && l >= maxLevel {
					maxLevel = l + 1
				}
			}
			levels[name] = maxLevel
		}
	}

	// Group by level
	maxLevel := 0
	for _, l := range levels {
		if l > maxLevel {
			maxLevel = l
		}
	}

	result := make([][]string, maxLevel+1)
	for name, l := range levels {
		result[l] = append(result[l], name)
	}

	return result
}

// startLevel starts all processes in a level concurrently
func (s *Scheduler) startLevel(ctx context.Context, names []string) error {
	var wg sync.WaitGroup
	errCh := make(chan error, len(names))

	for _, name := range names {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			if err := s.startProcess(ctx, n); err != nil {
				errCh <- fmt.Errorf("starting %s: %w", n, err)
			}
		}(name)
	}

	wg.Wait()
	close(errCh)

	// Collect errors
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return errs[0] // Return first error
	}

	return nil
}

// startProcess starts a single process, waiting for dependencies first
func (s *Scheduler) startProcess(ctx context.Context, name string) error {
	node := s.graph.GetNode(name)
	if node == nil {
		return fmt.Errorf("unknown process: %s", name)
	}

	// Already running or completed (e.g. started individually from the UI
	// before "start all") — nothing to do.
	if node.State == StateRunning || node.State == StateCompleted {
		return nil
	}

	// Wait for dependencies
	if err := s.waitForDependencies(ctx, name); err != nil {
		s.sendEvent(StartEvent{
			Name:    name,
			Type:    EventFailed,
			Message: "dependency failed",
			Error:   err,
		})
		return err
	}

	// Start the process
	s.graph.UpdateState(name, StateStarting)
	s.sendEvent(StartEvent{
		Name:    name,
		Type:    EventStarting,
		Message: fmt.Sprintf("starting %s", node.Config.Command),
	})

	proc, err := s.starter(ctx, name, node.Config)
	if err != nil {
		s.graph.UpdateState(name, StateFailed)
		s.sendEvent(StartEvent{
			Name:    name,
			Type:    EventFailed,
			Message: "failed to start",
			Error:   err,
		})
		return err
	}

	s.graph.SetProcess(name, proc)
	s.graph.UpdateState(name, StateRunning)
	s.sendEvent(StartEvent{
		Name:    name,
		Type:    EventStarted,
		Message: "process started",
	})

	// One-shot processes: wait for the command to finish. Exit 0 completes the
	// node (satisfying process_started and process_completed); anything else
	// fails it and dependents fail fast.
	if node.Config.Type == config.ProcessTypeOneshot {
		return s.waitForCompletion(ctx, name, proc)
	}

	// Handle startup delay if specified
	if node.Config.StartupDelay.Duration > 0 {
		time.Sleep(node.Config.StartupDelay.Duration)
		s.graph.UpdateHealth(name, HealthHealthy)
		s.sendEvent(StartEvent{
			Name:    name,
			Type:    EventHealthy,
			Message: fmt.Sprintf("ready after %s delay", node.Config.StartupDelay.Duration),
		})
		return nil
	}

	// Start health monitoring if configured
	if node.Config.HealthCheck != nil {
		monitor := NewHealthMonitor(name, node.Config.HealthCheck, s.graph)
		s.mu.Lock()
		s.healthMonitors[name] = monitor
		s.mu.Unlock()

		go monitor.Start(ctx)

		// Wait for initial healthy state
		timeout := 60 * time.Second
		if node.Config.HealthCheck.StartPeriod.Duration > 0 {
			timeout = node.Config.HealthCheck.StartPeriod.Duration + 30*time.Second
		}

		if err := monitor.WaitForHealthy(ctx, timeout); err != nil {
			s.sendEvent(StartEvent{
				Name:    name,
				Type:    EventFailed,
				Message: "health check failed",
				Error:   err,
			})
			return err
		}

		s.sendEvent(StartEvent{
			Name:    name,
			Type:    EventHealthy,
			Message: "health check passed",
		})
	} else {
		// No health check, mark as healthy immediately
		s.graph.UpdateHealth(name, HealthHealthy)
	}

	return nil
}

// waitForDependencies waits for all dependencies of a process to be satisfied
func (s *Scheduler) waitForDependencies(ctx context.Context, name string) error {
	node := s.graph.GetNode(name)
	if node == nil || node.Config == nil {
		return nil
	}

	for _, dep := range node.Config.DependsOn {
		s.sendEvent(StartEvent{
			Name:    name,
			Type:    EventWaiting,
			Message: fmt.Sprintf("waiting for %s (%s)", dep.Process, dep.Condition),
		})

		if err := s.waitForDependency(ctx, dep); err != nil {
			return fmt.Errorf("waiting for %s: %w", dep.Process, err)
		}
	}

	return nil
}

// waitForDependency waits for a single dependency to be satisfied
func (s *Scheduler) waitForDependency(ctx context.Context, dep config.Dependency) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	// Use Timer instead of time.After to allow proper cleanup
	timer := time.NewTimer(5 * time.Minute)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return fmt.Errorf("timeout waiting for %s", dep.Process)
		case <-ticker.C:
			depNode := s.graph.GetNode(dep.Process)
			if depNode == nil {
				return fmt.Errorf("unknown dependency: %s", dep.Process)
			}

			switch dep.Condition {
			case config.ConditionStarted:
				if depNode.State == StateRunning || depNode.State == StateCompleted {
					return nil
				}
				if depNode.State == StateFailed {
					return fmt.Errorf("dependency %s failed", dep.Process)
				}
				if depNode.State == StateStopped {
					return fmt.Errorf("dependency %s stopped", dep.Process)
				}
			case config.ConditionHealthy:
				if depNode.Health == HealthHealthy {
					return nil
				}
				if depNode.State == StateFailed {
					return fmt.Errorf("dependency %s failed", dep.Process)
				}
				if depNode.State == StateStopped {
					return fmt.Errorf("dependency %s stopped", dep.Process)
				}
			case config.ConditionCompleted:
				if depNode.State == StateCompleted {
					return nil
				}
				if depNode.State == StateFailed {
					return fmt.Errorf("dependency %s failed", dep.Process)
				}
				if depNode.State == StateStopped {
					return fmt.Errorf("dependency %s stopped", dep.Process)
				}
			}
		}
	}
}

// waitForCompletion polls a oneshot process until it exits, with the same
// bounded pattern as waitForDependency.
func (s *Scheduler) waitForCompletion(ctx context.Context, name string, proc *Process) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(5 * time.Minute)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			s.graph.UpdateState(name, StateFailed)
			s.sendEvent(StartEvent{Name: name, Type: EventFailed, Message: "timed out waiting for oneshot to finish"})
			return fmt.Errorf("timeout waiting for %s to complete", name)
		case <-ticker.C:
			switch proc.RunState() {
			case RunStateRunning:
				// still going
			case RunStateKilled:
				s.graph.UpdateState(name, StateStopped)
				return fmt.Errorf("%s was stopped before completing", name)
			case RunStateExited:
				if code := proc.ExitCode(); code != 0 {
					s.graph.UpdateState(name, StateFailed)
					s.sendEvent(StartEvent{Name: name, Type: EventFailed, Message: fmt.Sprintf("exited with code %d", code)})
					return fmt.Errorf("%s exited with code %d", name, code)
				}
				s.graph.UpdateState(name, StateCompleted)
				s.graph.UpdateHealth(name, HealthHealthy)
				s.sendEvent(StartEvent{Name: name, Type: EventCompleted, Message: "completed"})
				return nil
			}
		}
	}
}

// sendEvent sends an event to the events channel
func (s *Scheduler) sendEvent(event StartEvent) {
	select {
	case s.events <- event:
	default:
		// Channel full, log warning about dropped event
		log.Printf("warning: scheduler event channel full, dropping event: %s %s", event.Name, event.Type)
	}
}

// StopAll stops all processes in reverse dependency order
func (s *Scheduler) StopAll(ctx context.Context) error {
	// Stop health monitors first
	s.mu.Lock()
	for _, monitor := range s.healthMonitors {
		monitor.Stop()
	}
	s.healthMonitors = make(map[string]*HealthMonitor)
	s.mu.Unlock()

	// Get reverse order
	order, err := s.graph.GetStopOrder()
	if err != nil {
		return err
	}

	for _, name := range order {
		node := s.graph.GetNode(name)
		if node != nil && node.Process != nil {
			if err := node.Process.Stop(); err != nil {
				// Log error but continue stopping others
				s.sendEvent(StartEvent{
					Name:    name,
					Type:    EventFailed,
					Message: "failed to stop",
					Error:   err,
				})
			}
			s.graph.UpdateState(name, StateStopped)
		}
	}

	return nil
}

// RestartProcess restarts a single process and its dependents
func (s *Scheduler) RestartProcess(ctx context.Context, name string) error {
	// Get all dependents that need to be stopped first
	dependents := s.getAllDependents(name)

	// Stop in reverse order (dependents first)
	for i := len(dependents) - 1; i >= 0; i-- {
		depName := dependents[i]
		node := s.graph.GetNode(depName)
		if node != nil && node.Process != nil {
			node.Process.Stop()
			s.graph.UpdateState(depName, StateStopped)
		}
	}

	// Stop the target process
	node := s.graph.GetNode(name)
	if node != nil && node.Process != nil {
		node.Process.Stop()
	}
	s.graph.UpdateState(name, StatePending)
	s.graph.UpdateHealth(name, HealthUnknown)

	// Restart the target process
	if err := s.startProcess(ctx, name); err != nil {
		return err
	}

	// Restart dependents in order
	for _, depName := range dependents {
		s.graph.UpdateState(depName, StatePending)
		s.graph.UpdateHealth(depName, HealthUnknown)
		if err := s.startProcess(ctx, depName); err != nil {
			return fmt.Errorf("restarting dependent %s: %w", depName, err)
		}
	}

	return nil
}

// getAllDependents returns all processes that depend on the given process (transitively)
func (s *Scheduler) getAllDependents(name string) []string {
	visited := make(map[string]bool)
	var result []string

	var visit func(n string)
	visit = func(n string) {
		for _, dep := range s.graph.GetDependents(n) {
			if !visited[dep] {
				visited[dep] = true
				result = append(result, dep)
				visit(dep)
			}
		}
	}

	visit(name)
	return result
}

// GetProcessStatus returns the current status of all processes
func (s *Scheduler) GetProcessStatus() map[string]ProcessStatus {
	status := make(map[string]ProcessStatus)

	for _, node := range s.graph.GetAllNodes() {
		ps := ProcessStatus{
			Name:   node.Name,
			State:  node.State,
			Health: node.Health,
		}

		if node.Process != nil {
			ps.PID = node.Process.Pid()
			ps.Uptime = time.Since(node.Process.StartTime())
		}

		status[node.Name] = ps
	}

	return status
}

// ProcessStatus represents the current status of a process
type ProcessStatus struct {
	Name   string
	State  ProcessState
	Health HealthState
	PID    int
	Uptime time.Duration
}
