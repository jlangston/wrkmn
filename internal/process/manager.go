package process

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"

	"wrkmn/internal/config"
	"wrkmn/internal/worker"
)

// buildWranglerPassthrough returns the args to append after a `bun run <script>`
// (or configured command) invocation. They are placed behind a single `--`
// separator so they reach the underlying `wrangler dev` rather than bun/npm.
//
// Mode mapping:
//
//	local           -> --inspector-port <p> --local   (debugger on)
//	remote-bindings -> --inspector-port <p>            (debugger on; honors `remote: true` bindings)
//	remote          -> --remote                        (code on edge; no inspector, debugger off)
//
// `--env <env>` is appended for any mode when env is non-empty. Returns nil when
// there are no flags to add (so callers can append unconditionally).
func buildWranglerPassthrough(mode config.DevMode, inspectorPort int, env string) []string {
	if mode == "" {
		mode = config.DevModeRemoteBindings
	}

	var flags []string
	if mode != config.DevModeRemote && inspectorPort > 0 {
		flags = append(flags, "--inspector-port", strconv.Itoa(inspectorPort))
	}
	switch mode {
	case config.DevModeLocal:
		flags = append(flags, "--local")
	case config.DevModeRemote:
		flags = append(flags, "--remote")
	}
	if env != "" && env != "production" {
		flags = append(flags, "--env", env)
	}
	if len(flags) == 0 {
		return nil
	}
	return append([]string{"--"}, flags...)
}

type Manager struct {
	processes map[string]*Process
	mu        sync.RWMutex
}

// ProcessRunState describes whether a process is alive and, if not, how it ended.
type ProcessRunState int

const (
	RunStateRunning ProcessRunState = iota
	RunStateExited  // exited on its own
	RunStateKilled  // we cancelled/stopped it
)

type Process struct {
	Name      string          // Process name (from config or worker name)
	Worker    *worker.Worker  // nil for non-worker processes
	Config    *config.Process // nil for legacy worker-only processes
	Script    string          // package.json script for StartWorker-launched processes ("" otherwise)
	Cmd       *exec.Cmd
	Cancel    context.CancelFunc
	LogBuffer *CircularBuffer
	Started   time.Time
	mu        sync.RWMutex

	// Exit tracking (guarded by mu)
	runState ProcessRunState
	exitCode int
	exitErr  error
	finished time.Time
	killed   bool // true if we explicitly requested to stop this process
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

// RunState reports whether the process is alive and, if not, how it ended.
func (p *Process) RunState() ProcessRunState {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.runState
}

// ExitCode returns the recorded exit code (0 until the process exits; -1 for
// exits with no extractable code).
func (p *Process) ExitCode() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.exitCode
}

// ExitErr returns the error from cmd.Wait for a natural exit, if any.
func (p *Process) ExitErr() error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.exitErr
}

// Finished returns when the process exited (zero while running).
func (p *Process) Finished() time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.finished
}

// recordExit stores the outcome of cmd.Wait. It reads the killed flag
// (set by markKilled before external signals) to determine if this was
// an externally requested stop rather than a natural exit. Returns true
// if the process was externally killed.
func (p *Process) recordExit(waitErr error) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.finished = time.Now()
	killed := p.killed
	if killed {
		p.runState = RunStateKilled
		return true
	}
	p.runState = RunStateExited
	p.exitErr = waitErr
	switch e := waitErr.(type) {
	case nil:
		p.exitCode = 0
	case *exec.ExitError:
		p.exitCode = e.ExitCode()
	default:
		p.exitCode = -1
	}
	return false
}

// markKilled marks this process as explicitly killed/stopped externally.
// Must be called before sending signals to ensure superviseProcess sees the killed flag.
func (p *Process) markKilled() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.killed = true
}

// Stop stops the process
func (p *Process) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.Cmd == nil || p.Cmd.Process == nil {
		return nil
	}

	// Mark that we're explicitly stopping this process
	p.killed = true

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

// superviseProcess waits for the process to end and records its exit state.
// For natural exits, it reflects the outcome on the worker's status.
// Externally requested stops (StopWorker/StopProcess/StopAll) set the killed flag
// before signaling, so recordExit correctly distinguishes natural vs forced exits.
func (m *Manager) superviseProcess(p *Process) {
	waitErr := p.Cmd.Wait()
	killed := p.recordExit(waitErr)
	if killed {
		return
	}

	if p.Worker != nil {
		if p.ExitCode() == 0 {
			p.Worker.Status = worker.StatusStopped
		} else {
			p.Worker.Status = worker.StatusFailed
		}
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

	if existing, exists := m.processes[w.Name]; exists {
		if existing.RunState() == RunStateRunning {
			return fmt.Errorf("worker %s is already running", w.Name)
		}
		delete(m.processes, w.Name) // dead entry from a previous run — replace it
	}

	_, exists := w.Scripts[script]
	if !exists {
		return fmt.Errorf("script %s not found for worker %s", script, w.Name)
	}

	ctx, cancel := context.WithCancel(context.Background())

	runArgs := []string{"run", script}
	runArgs = append(runArgs, buildWranglerPassthrough(w.DevMode, w.InspectorPort, w.SelectedEnv)...)
	cmd := exec.CommandContext(ctx, "bun", runArgs...)
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
		Name:      w.Name,
		Worker:    w,
		Script:    script,
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

	go m.superviseProcess(process)

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

	// Mark killed before sending signals so superviseProcess sees the killed flag
	process.markKilled()

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

	// Mark all processes as killed before sending signals so superviseProcess sees the killed flag
	for _, process := range processes {
		process.markKilled()
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
		// Only update worker status if we explicitly killed this process (marked as killed before signals).
		// If it exited naturally, superviseProcess already set the appropriate status.
		if process.Worker != nil {
			process.mu.RLock()
			wasKilled := process.killed
			process.mu.RUnlock()
			if wasKilled {
				process.Worker.Status = worker.StatusStopped
			}
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
func (m *Manager) StartProcess(ctx context.Context, name string, cfg *config.Process, w *worker.Worker) (*Process, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, exists := m.processes[name]; exists {
		if existing.RunState() == RunStateRunning {
			return nil, fmt.Errorf("process %s is already running", name)
		}
		delete(m.processes, name) // dead entry from a previous run — replace it
	}

	ctx, cancel := context.WithCancel(ctx)

	// Build command with args. For worker processes, forward wrangler passthrough
	// flags (inspector port, dev mode, env) so wrangler dev receives them.
	args := append([]string{}, cfg.Args...)
	if cfg.Type == config.ProcessTypeWorker {
		mode := cfg.DevMode
		inspectorPort := cfg.InspectorPort
		env := ""
		if w != nil {
			if w.DevMode != "" {
				mode = w.DevMode
			}
			env = w.SelectedEnv
			if w.InspectorPort > 0 {
				inspectorPort = w.InspectorPort
			}
		}
		args = append(args, buildWranglerPassthrough(mode, inspectorPort, env)...)
	}
	var cmd *exec.Cmd
	if len(args) > 0 {
		cmd = exec.CommandContext(ctx, cfg.Command, args...)
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
		Worker:    w,
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

	go m.superviseProcess(process)

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
