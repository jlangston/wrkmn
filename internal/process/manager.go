package process

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"wrkmn/internal/config"
	"wrkmn/internal/worker"
)

type Manager struct {
	processes map[string]*Process
	mu        sync.RWMutex
}

type Process struct {
	Name      string         // Process name (from config or worker name)
	Worker    *worker.Worker // nil for non-worker processes
	Config    *config.Process // nil for legacy worker-only processes
	Cmd       *exec.Cmd
	Cancel    context.CancelFunc
	LogBuffer *CircularBuffer
	Started   time.Time
	mu        sync.RWMutex
}

// Pid returns the process ID
func (p *Process) Pid() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.Cmd != nil && p.Cmd.Process != nil {
		return p.Cmd.Process.Pid
	}
	return 0
}

// StartTime returns when the process was started
func (p *Process) StartTime() time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.Started
}

// Stop stops the process
func (p *Process) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.Cmd == nil || p.Cmd.Process == nil {
		return nil
	}

	// Kill the entire process group to prevent orphaned processes
	pgid, err := syscall.Getpgid(p.Cmd.Process.Pid)
	if err == nil {
		syscall.Kill(-pgid, syscall.SIGTERM)
		time.Sleep(100 * time.Millisecond)
		syscall.Kill(-pgid, syscall.SIGKILL)
	}

	if p.Cancel != nil {
		p.Cancel()
	}

	return nil
}

// IsRunning returns true if the process is still running
func (p *Process) IsRunning() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.Cmd == nil || p.Cmd.Process == nil {
		return false
	}
	// Check if process exists
	err := p.Cmd.Process.Signal(syscall.Signal(0))
	return err == nil
}

type CircularBuffer struct {
	lines    []string
	capacity int
	index    int
	full     bool
	mu       sync.RWMutex
}

func NewManager() *Manager {
	return &Manager{
		processes: make(map[string]*Process),
	}
}

func NewCircularBuffer(capacity int) *CircularBuffer {
	return &CircularBuffer{
		lines:    make([]string, capacity),
		capacity: capacity,
	}
}

func (cb *CircularBuffer) Write(line string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.lines[cb.index] = line
	cb.index = (cb.index + 1) % cb.capacity
	if cb.index == 0 {
		cb.full = true
	}
}

func (cb *CircularBuffer) GetLines() []string {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	if !cb.full {
		return cb.lines[:cb.index]
	}

	result := make([]string, cb.capacity)
	copy(result, cb.lines[cb.index:])
	copy(result[cb.capacity-cb.index:], cb.lines[:cb.index])
	return result
}

func (m *Manager) StartWorker(w *worker.Worker, script string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.processes[w.Name]; exists {
		return fmt.Errorf("worker %s is already running", w.Name)
	}

	_, exists := w.Scripts[script]
	if !exists {
		return fmt.Errorf("script %s not found for worker %s", script, w.Name)
	}

	ctx, cancel := context.WithCancel(context.Background())

	cmd := exec.CommandContext(ctx, "bun", "run", script)
	cmd.Dir = w.Path
	// Set process group to handle signals properly
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Pgid:    0,
	}

	logBuffer := NewCircularBuffer(1000)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdout.Close() // Close stdout pipe to prevent leak
		cancel()
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	process := &Process{
		Worker:    w,
		Cmd:       cmd,
		Cancel:    cancel,
		LogBuffer: logBuffer,
		Started:   time.Now(),
	}

	m.processes[w.Name] = process

	if err := cmd.Start(); err != nil {
		delete(m.processes, w.Name)
		cancel()
		return fmt.Errorf("failed to start worker: %w", err)
	}

	go m.readLogs(stdout, logBuffer, "STDOUT")
	go m.readLogs(stderr, logBuffer, "STDERR")

	// Use a goroutine that can be interrupted by context cancellation
	go func(ctx context.Context, name string) {
		done := make(chan struct{})
		go func() {
			cmd.Wait()
			close(done)
		}()

		select {
		case <-done:
			m.mu.Lock()
			if proc, exists := m.processes[name]; exists && proc.Worker != nil {
				proc.Worker.Status = worker.StatusStopped
			}
			m.mu.Unlock()
		case <-ctx.Done():
			// Context cancelled, process is being stopped externally
			// No need to update status as StopWorker handles it
		}
	}(ctx, w.Name)

	w.Status = worker.StatusRunning
	return nil
}

func (m *Manager) StopWorker(workerName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	process, exists := m.processes[workerName]
	if !exists {
		return fmt.Errorf("worker %s is not running", workerName)
	}

	// Kill the entire process group to prevent orphaned processes
	if process.Cmd.Process != nil {
		pgid, err := syscall.Getpgid(process.Cmd.Process.Pid)
		if err == nil {
			// Kill the entire process group
			syscall.Kill(-pgid, syscall.SIGTERM)
			// Give processes time to gracefully shut down
			time.Sleep(100 * time.Millisecond)
			// Force kill if still running
			syscall.Kill(-pgid, syscall.SIGKILL)
		}
	}

	process.Cancel()
	delete(m.processes, workerName)
	if process.Worker != nil {
		process.Worker.Status = worker.StatusStopped
	}

	return nil
}

func (m *Manager) GetProcess(workerName string) (*Process, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	process, exists := m.processes[workerName]
	return process, exists
}

func (m *Manager) GetRunningProcesses() map[string]*Process {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*Process)
	for name, process := range m.processes {
		result[name] = process
	}
	return result
}

func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Collect all processes before modifying the map
	processes := make([]*Process, 0, len(m.processes))
	for _, process := range m.processes {
		processes = append(processes, process)
	}

	// Send SIGTERM to all process groups
	for _, process := range processes {
		if process.Cmd != nil && process.Cmd.Process != nil {
			pgid, err := syscall.Getpgid(process.Cmd.Process.Pid)
			if err == nil {
				syscall.Kill(-pgid, syscall.SIGTERM)
			}
		}
	}

	// Give processes time to gracefully shut down
	time.Sleep(200 * time.Millisecond)

	// Force kill any remaining processes
	for _, process := range processes {
		if process.Cmd != nil && process.Cmd.Process != nil {
			pgid, err := syscall.Getpgid(process.Cmd.Process.Pid)
			if err == nil {
				syscall.Kill(-pgid, syscall.SIGKILL)
			}
		}
		if process.Cancel != nil {
			process.Cancel()
		}
		if process.Worker != nil {
			process.Worker.Status = worker.StatusStopped
		}
	}

	// Clear the map
	m.processes = make(map[string]*Process)
}

func (m *Manager) readLogs(reader io.Reader, buffer *CircularBuffer, prefix string) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := fmt.Sprintf("[%s] %s: %s", time.Now().Format("15:04:05"), prefix, scanner.Text())
		buffer.Write(line)
	}
}

// StartProcess starts a generic process from config
func (m *Manager) StartProcess(ctx context.Context, name string, cfg *config.Process) (*Process, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.processes[name]; exists {
		return nil, fmt.Errorf("process %s is already running", name)
	}

	ctx, cancel := context.WithCancel(ctx)

	// Build command with args
	var cmd *exec.Cmd
	if len(cfg.Args) > 0 {
		cmd = exec.CommandContext(ctx, cfg.Command, cfg.Args...)
	} else {
		cmd = exec.CommandContext(ctx, cfg.Command)
	}

	// Set working directory
	if cfg.WorkingDir != "" {
		cmd.Dir = cfg.WorkingDir
	}

	// Set environment variables (inherit current env and add custom vars)
	if len(cfg.Environment) > 0 {
		cmd.Env = os.Environ() // Start with current environment
		for k, v := range cfg.Environment {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}
	// If no custom env vars, cmd.Env stays nil which inherits parent env

	// Set process group to handle signals properly
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Pgid:    0,
	}

	logBuffer := NewCircularBuffer(1000)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdout.Close() // Close stdout pipe to prevent leak
		cancel()
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	process := &Process{
		Name:      name,
		Config:    cfg,
		Cmd:       cmd,
		Cancel:    cancel,
		LogBuffer: logBuffer,
		Started:   time.Now(),
	}

	m.processes[name] = process

	if err := cmd.Start(); err != nil {
		delete(m.processes, name)
		cancel()
		return nil, fmt.Errorf("failed to start process: %w", err)
	}

	go m.readLogs(stdout, logBuffer, "STDOUT")
	go m.readLogs(stderr, logBuffer, "STDERR")

	// Use a goroutine that can be interrupted by context cancellation
	go func(ctx context.Context, processName string) {
		done := make(chan struct{})
		go func() {
			cmd.Wait()
			close(done)
		}()

		select {
		case <-done:
			// Process exited naturally
			m.mu.Lock()
			// Mark process as stopped when it exits
			m.mu.Unlock()
		case <-ctx.Done():
			// Context cancelled, process is being stopped externally
		}
	}(ctx, name)

	return process, nil
}

// StopProcess stops a process by name
func (m *Manager) StopProcess(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	process, exists := m.processes[name]
	if !exists {
		return fmt.Errorf("process %s is not running", name)
	}

	if err := process.Stop(); err != nil {
		return err
	}

	delete(m.processes, name)
	return nil
}

// GetAllProcesses returns all managed processes
func (m *Manager) GetAllProcesses() map[string]*Process {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*Process)
	for name, process := range m.processes {
		result[name] = process
	}
	return result
}
