package process

import (
	"fmt"
	"sync"

	"wrkmn/internal/config"
)

// ProcessState represents the current state of a process
type ProcessState int

const (
	StatePending ProcessState = iota
	StateStarting
	StateRunning
	StateCompleted // a oneshot that exited 0
	StateStopped
	StateFailed
)

func (s ProcessState) String() string {
	switch s {
	case StatePending:
		return "pending"
	case StateStarting:
		return "starting"
	case StateRunning:
		return "running"
	case StateCompleted:
		return "completed"
	case StateStopped:
		return "stopped"
	case StateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// HealthState represents the health status of a process
type HealthState int

const (
	HealthUnknown HealthState = iota
	HealthChecking
	HealthHealthy
	HealthUnhealthy
)

func (h HealthState) String() string {
	switch h {
	case HealthUnknown:
		return "unknown"
	case HealthChecking:
		return "checking"
	case HealthHealthy:
		return "healthy"
	case HealthUnhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}

// ProcessNode represents a process in the dependency graph
type ProcessNode struct {
	Name    string
	Config  *config.Process
	State   ProcessState
	Health  HealthState
	Process *Process // The actual running process, if any
}

// DependencyGraph represents process dependencies as a DAG
type DependencyGraph struct {
	nodes    map[string]*ProcessNode
	edges    map[string][]string // process -> depends on
	reversed map[string][]string // process -> depended by (reverse edges)
	mu       sync.RWMutex
}

// NewDependencyGraph creates a new dependency graph from config
func NewDependencyGraph(processes map[string]config.Process) *DependencyGraph {
	g := &DependencyGraph{
		nodes:    make(map[string]*ProcessNode),
		edges:    make(map[string][]string),
		reversed: make(map[string][]string),
	}

	for name, proc := range processes {
		p := proc // copy for pointer
		g.nodes[name] = &ProcessNode{
			Name:   name,
			Config: &p,
			State:  StatePending,
			Health: HealthUnknown,
		}

		for _, dep := range proc.DependsOn {
			g.edges[name] = append(g.edges[name], dep.Process)
			g.reversed[dep.Process] = append(g.reversed[dep.Process], name)
		}
	}

	return g
}

// GetNode returns the node for a process
func (g *DependencyGraph) GetNode(name string) *ProcessNode {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.nodes[name]
}

// GetAllNodes returns all nodes
func (g *DependencyGraph) GetAllNodes() []*ProcessNode {
	g.mu.RLock()
	defer g.mu.RUnlock()

	nodes := make([]*ProcessNode, 0, len(g.nodes))
	for _, node := range g.nodes {
		nodes = append(nodes, node)
	}
	return nodes
}

// UpdateState updates the state of a process
func (g *DependencyGraph) UpdateState(name string, state ProcessState) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if node, exists := g.nodes[name]; exists {
		node.State = state
	}
}

// UpdateHealth updates the health state of a process
func (g *DependencyGraph) UpdateHealth(name string, health HealthState) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if node, exists := g.nodes[name]; exists {
		node.Health = health
	}
}

// SetProcess associates a running process with a node
func (g *DependencyGraph) SetProcess(name string, proc *Process) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if node, exists := g.nodes[name]; exists {
		node.Process = proc
	}
}

// GetStartOrder returns processes in topological order (dependencies first)
func (g *DependencyGraph) GetStartOrder() ([]string, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Kahn's algorithm for topological sort
	inDegree := make(map[string]int)
	for name := range g.nodes {
		inDegree[name] = len(g.edges[name])
	}

	// Find all nodes with no dependencies
	var queue []string
	for name, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, name)
		}
	}

	var order []string
	for len(queue) > 0 {
		// Pop from queue
		current := queue[0]
		queue = queue[1:]
		order = append(order, current)

		// Reduce in-degree for all dependents
		for _, dependent := range g.reversed[current] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	// Check for cycles (if not all nodes are in order)
	if len(order) != len(g.nodes) {
		return nil, fmt.Errorf("circular dependency detected")
	}

	return order, nil
}

// GetReadyToStart returns processes whose dependencies are satisfied
func (g *DependencyGraph) GetReadyToStart() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var ready []string
	for name, node := range g.nodes {
		if node.State != StatePending {
			continue
		}
		if g.dependenciesSatisfied(name) {
			ready = append(ready, name)
		}
	}
	return ready
}

// dependenciesSatisfied checks if all dependencies for a process are met
// Must be called with lock held
func (g *DependencyGraph) dependenciesSatisfied(name string) bool {
	node := g.nodes[name]
	if node == nil || node.Config == nil {
		return true
	}

	for _, dep := range node.Config.DependsOn {
		depNode := g.nodes[dep.Process]
		if depNode == nil {
			return false
		}

		switch dep.Condition {
		case config.ConditionStarted:
			if depNode.State != StateRunning && depNode.State != StateCompleted {
				return false
			}
		case config.ConditionHealthy:
			if depNode.Health != HealthHealthy {
				return false
			}
		case config.ConditionCompleted:
			if depNode.State != StateCompleted {
				return false
			}
		}
	}
	return true
}

// GetDependents returns all processes that depend on the given process
func (g *DependencyGraph) GetDependents(name string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.reversed[name]
}

// GetDependencies returns all processes that the given process depends on
func (g *DependencyGraph) GetDependencies(name string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.edges[name]
}

// GetStopOrder returns processes in reverse topological order (dependents first)
func (g *DependencyGraph) GetStopOrder() ([]string, error) {
	order, err := g.GetStartOrder()
	if err != nil {
		return nil, err
	}

	// Reverse the order
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}

	return order, nil
}
