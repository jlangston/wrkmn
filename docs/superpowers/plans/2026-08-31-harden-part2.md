# Hardening Part 2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Finish the approved 2026-05-18 hardening spec (exit tracking, oneshot, validate, pre-flight) plus implement the documented-but-missing restart policy, wire dependency-ordered startup into the TUI, and persist runtime selections.

**Architecture:** Exit state (`RunState`/`ExitCode`) is recorded on `process.Process` by a shared supervise goroutine, which also drives restart policy. The existing (currently unused) `Scheduler` becomes the TUI's "start all" path so `depends_on` actually holds, and gains `oneshot`/`process_completed` semantics on top of exit tracking. New leaf packages `internal/preflight` (checks) and `internal/state` (runtime-selection persistence) are consumed by `cmd` and the TUI.

**Tech Stack:** Go, Bubble Tea / Lipgloss, Cobra, `go test` (standard library only).

**Spec:** `docs/superpowers/specs/2026-05-18-harden-multiservice-design.md` (Components 2–5). Deviations from that spec, agreed in chat 2026-08-31:
- Component 1's env loading already exists as `internal/config/envfile.go` (not the planned `internal/env` package). We keep it there and add the spec's severity rules (explicit missing = error, defaulted missing = skip). Provenance needs no extra flag: `applyDefaults` never copies `defaults.env_file` into processes, so `defaults.EnvFile` vs `proc.EnvFile` *is* the provenance.
- Component 5's TUI consumption is a terminal-level gate in `cmd/root.go` before the TUI launches (errors exit; warnings pause for Enter; `--skip-preflight` bypasses), not an in-TUI summary screen. Same guarantee, far less UI code.
- Additions beyond the spec (approved in chat): restart policy implementation, scheduler wiring into the `a` key, runtime-selection persistence, docs housekeeping.

## Global Constraints

- Go standard library only for new code; no new module dependencies.
- All work on branch `harden-part2` (created in Task 1 from `main`).
- Never `git add -A`; stage only the files each task lists. Do NOT commit `.claude/settings.local.json`.
- Existing public signatures stay stable except where a task explicitly changes them.
- `sh` is assumed on PATH in tests (used by existing `internal/process` tests already).
- Every process-spawning test must clean up (defer `m.StopAll()` or stop started processes) so `go test` never leaks children.

---

### Task 1: Branch + docs housekeeping

**Files:**
- Modify: `docs/superpowers/plans/2026-06-22-dev-mode-remote.md` (check off completed checkboxes)
- Commit (already written, untracked): `docs/superpowers/specs/2026-06-18-dev-mode-remote-design.md`, `docs/superpowers/plans/2026-06-22-dev-mode-remote.md`, `docs/superpowers/specs/2026-05-18-harden-multiservice-design.md` (if untracked), this plan file

**Interfaces:**
- Consumes: nothing
- Produces: branch `harden-part2`; committed design docs

- [ ] **Step 1: Create the branch**

```bash
cd /home/jefflangston/dev/wrkmn
git checkout -b harden-part2
```

- [ ] **Step 2: Check off the completed dev-mode plan**

The dev-mode plan's work all shipped (commits `3f0e0a4`..`b5fc2ca`). Mark every checkbox done:

```bash
cd /home/jefflangston/dev/wrkmn
sed -i 's/^- \[ \]/- [x]/' docs/superpowers/plans/2026-06-22-dev-mode-remote.md
```

- [ ] **Step 3: Commit the docs**

```bash
cd /home/jefflangston/dev/wrkmn
git add docs/superpowers/specs/ docs/superpowers/plans/
git commit -m "docs: commit dev-mode spec/plan (shipped) and harden-part2 plan"
```

---

### Task 2: Exit tracking on `process.Process`

**Files:**
- Modify: `internal/process/manager.go`
- Test: `internal/process/manager_test.go` (append)

**Interfaces:**
- Consumes: existing `Process` struct, `StartWorker`, `StartProcess`.
- Produces: `type ProcessRunState int` with `RunStateRunning`/`RunStateExited`/`RunStateKilled`; methods `(p *Process) RunState() ProcessRunState`, `ExitCode() int`, `ExitErr() error`, `Finished() time.Time`; `Process.Name` now also set by `StartWorker`; `(m *Manager) superviseProcess(ctx, p)` goroutine (unexported); dead map entries are replaced on restart.

- [ ] **Step 1: Write the failing tests**

Append to `internal/process/manager_test.go` (match its existing imports; add `"context"`, `"testing"`, `"time"`, `"wrkmn/internal/config"`, `"wrkmn/internal/worker"` as needed):

```go
// waitForExit polls until the process leaves RunStateRunning or the timeout hits.
func waitForExit(t *testing.T, p *Process, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if p.RunState() != RunStateRunning {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("process still running after %s", timeout)
}

func TestExitCodeCapturedOnCrash(t *testing.T) {
	m := NewManager()
	cfg := &config.Process{Command: "sh", Args: []string{"-c", "exit 3"}, Type: config.ProcessTypeService}
	p, err := m.StartProcess(context.Background(), "boom", cfg, nil)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForExit(t, p, 5*time.Second)
	if p.RunState() != RunStateExited {
		t.Fatalf("RunState = %v, want RunStateExited", p.RunState())
	}
	if p.ExitCode() != 3 {
		t.Fatalf("ExitCode = %d, want 3", p.ExitCode())
	}
	if p.Finished().IsZero() {
		t.Fatal("Finished not recorded")
	}
}

func TestExitCodeZeroOnCleanExit(t *testing.T) {
	m := NewManager()
	cfg := &config.Process{Command: "true", Type: config.ProcessTypeService}
	p, err := m.StartProcess(context.Background(), "ok", cfg, nil)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForExit(t, p, 5*time.Second)
	if p.RunState() != RunStateExited || p.ExitCode() != 0 {
		t.Fatalf("got (%v, %d), want (RunStateExited, 0)", p.RunState(), p.ExitCode())
	}
}

func TestStoppedProcessMarkedKilled(t *testing.T) {
	m := NewManager()
	cfg := &config.Process{Command: "sleep", Args: []string{"30"}, Type: config.ProcessTypeService}
	p, err := m.StartProcess(context.Background(), "sleeper", cfg, nil)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := m.StopProcess("sleeper"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	waitForExit(t, p, 5*time.Second)
	if p.RunState() != RunStateKilled {
		t.Fatalf("RunState = %v, want RunStateKilled", p.RunState())
	}
}

func TestCrashedWorkerStatusFailed(t *testing.T) {
	m := NewManager()
	w := &worker.Worker{Name: "crashy", Status: worker.StatusStarting}
	cfg := &config.Process{Command: "sh", Args: []string{"-c", "exit 2"}, Type: config.ProcessTypeService}
	p, err := m.StartProcess(context.Background(), "crashy", cfg, w)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForExit(t, p, 5*time.Second)
	// status write happens right after recordExit; give it a moment
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && w.Status != worker.StatusFailed {
		time.Sleep(10 * time.Millisecond)
	}
	if w.Status != worker.StatusFailed {
		t.Fatalf("worker status = %s, want failed", w.Status)
	}
}

func TestDeadEntryReplacedOnRestart(t *testing.T) {
	m := NewManager()
	cfg := &config.Process{Command: "sh", Args: []string{"-c", "exit 1"}, Type: config.ProcessTypeService}
	p, err := m.StartProcess(context.Background(), "flappy", cfg, nil)
	if err != nil {
		t.Fatalf("first start: %v", err)
	}
	waitForExit(t, p, 5*time.Second)
	// A dead entry must not block a fresh start.
	p2, err := m.StartProcess(context.Background(), "flappy", cfg, nil)
	if err != nil {
		t.Fatalf("second start: %v", err)
	}
	waitForExit(t, p2, 5*time.Second)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/jefflangston/dev/wrkmn && go test ./internal/process/ -run "TestExitCode|TestStoppedProcess|TestCrashedWorker|TestDeadEntry" -v`
Expected: compile error (`RunState` undefined).

- [ ] **Step 3: Add run-state fields, accessors, and recordExit**

In `internal/process/manager.go`, after the `Process` struct (line 60-69), extend it and add the type. The struct becomes:

```go
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
}
```

Add after the `StartTime` method:

```go
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

// recordExit stores the outcome of cmd.Wait. killed marks an externally
// requested stop (context cancelled) rather than a natural exit.
func (p *Process) recordExit(waitErr error, killed bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.finished = time.Now()
	if killed {
		p.runState = RunStateKilled
		return
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
}
```

- [ ] **Step 4: Replace both Wait goroutines with a shared supervisor**

Add to `manager.go`:

```go
// superviseProcess waits for the process to end, records its exit state, and
// reflects the outcome on the worker's status. Externally requested stops
// (StopWorker/StopProcess/StopAll) manage status themselves.
func (m *Manager) superviseProcess(ctx context.Context, p *Process) {
	waitErr := p.Cmd.Wait()
	killed := ctx.Err() != nil
	p.recordExit(waitErr, killed)
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
```

In `StartWorker`, replace the whole `go func(ctx context.Context, name string) { ... }(ctx, w.Name)` block (lines 229-248) with:

```go
	go m.superviseProcess(ctx, process)
```

In `StartProcess`, replace the whole `go func(ctx context.Context, processName string) { ... }(ctx, name)` block (lines 447-464) with:

```go
	go m.superviseProcess(ctx, process)
```

In `StartWorker`, also set `Name` in the `Process` literal (line 210):

```go
	process := &Process{
		Name:      w.Name,
		Worker:    w,
		Script:    script,
		Cmd:       cmd,
		Cancel:    cancel,
		LogBuffer: logBuffer,
		Started:   time.Now(),
	}
```

- [ ] **Step 5: Replace stale dead entries instead of refusing to start**

In `StartWorker`, replace the existing check (lines 174-176):

```go
	if existing, exists := m.processes[w.Name]; exists {
		if existing.RunState() == RunStateRunning {
			return fmt.Errorf("worker %s is already running", w.Name)
		}
		delete(m.processes, w.Name) // dead entry from a previous run — replace it
	}
```

In `StartProcess`, replace the existing check (lines 360-362):

```go
	if existing, exists := m.processes[name]; exists {
		if existing.RunState() == RunStateRunning {
			return nil, fmt.Errorf("process %s is already running", name)
		}
		delete(m.processes, name) // dead entry from a previous run — replace it
	}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd /home/jefflangston/dev/wrkmn && go build ./... && go test ./internal/process/ -v`
Expected: build succeeds; new tests PASS; all existing process tests still PASS (some existing tests may rely on the old "already running" behavior for dead processes — if one fails, read it: the new replace-dead-entry behavior is the intended one, update the test's expectation).

- [ ] **Step 7: Commit**

```bash
cd /home/jefflangston/dev/wrkmn
git add internal/process/manager.go internal/process/manager_test.go
git commit -m "process: record exit state (RunState/ExitCode) and replace dead map entries"
```

---

### Task 3: Restart policy (config + manager)

**Files:**
- Modify: `internal/config/loader.go` (`applyDefaults`, ~line 257)
- Modify: `internal/process/manager.go`
- Test: `internal/config/loader_test.go` (append), `internal/process/manager_test.go` (append)

**Interfaces:**
- Consumes: Task 2's `RunState`/`ExitCode`/`superviseProcess`, `Process.Name`, dead-entry replacement.
- Produces: `defaults.restart` inherited by processes; `Manager.restartTries map[string]int` (unexported); auto-restart of crashed config-based `worker`/`service` processes.

- [ ] **Step 1: Write the failing config test**

Append to `internal/config/loader_test.go` (it already has `writeTempConfig`):

```go
func TestRestartPolicyInheritedFromDefaults(t *testing.T) {
	path := writeTempConfig(t, `
version: "1.0"
processes:
  api:
    command: bun
    args: [run, dev]
    type: service
defaults:
  restart:
    enabled: true
`)
	cfg, err := NewLoader(path).Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	r := cfg.Processes["api"].Restart
	if r == nil || !r.Enabled {
		t.Fatalf("Restart = %+v, want inherited enabled policy", r)
	}
	if r.MaxTries != 3 {
		t.Fatalf("MaxTries = %d, want defaulted 3", r.MaxTries)
	}
}

func TestExplicitRestartPolicyNotOverridden(t *testing.T) {
	path := writeTempConfig(t, `
version: "1.0"
processes:
  api:
    command: bun
    type: service
    restart:
      enabled: false
defaults:
  restart:
    enabled: true
`)
	cfg, err := NewLoader(path).Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Processes["api"].Restart.Enabled {
		t.Fatal("explicit restart.enabled=false was overridden by defaults")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/jefflangston/dev/wrkmn && go test ./internal/config/ -run TestRestartPolicy -v && go test ./internal/config/ -run TestExplicitRestart -v`
Expected: FAIL (`Restart` is nil — defaults not inherited).

- [ ] **Step 3: Inherit `defaults.restart` in applyDefaults**

In `internal/config/loader.go`, inside `applyDefaults`, immediately BEFORE the existing `// Apply restart policy defaults` block (~line 257), add:

```go
		// Inherit restart policy from defaults when the process doesn't set one
		if proc.Restart == nil && cfg.Defaults.Restart != nil {
			policy := *cfg.Defaults.Restart
			proc.Restart = &policy
		}
```

(The existing block below it then fills MaxTries/Delay defaults for the inherited copy too.)

- [ ] **Step 4: Run config tests to verify they pass**

Run: `cd /home/jefflangston/dev/wrkmn && go test ./internal/config/ -v`
Expected: PASS.

- [ ] **Step 5: Write the failing manager tests**

Append to `internal/process/manager_test.go` (add `"os"`, `"path/filepath"`, `"strings"` imports as needed):

```go
func countLines(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return len(strings.Fields(string(data)))
}

func TestRestartPolicyRestartsCrashedProcess(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "runs")
	cfg := &config.Process{
		Command: "sh",
		Args:    []string{"-c", "echo run >> " + marker + "; exit 1"},
		Type:    config.ProcessTypeService,
		Restart: &config.RestartPolicy{Enabled: true, MaxTries: 2, Delay: config.Duration{Duration: 20 * time.Millisecond}},
	}
	m := NewManager()
	if _, err := m.StartProcess(context.Background(), "flappy", cfg, nil); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer m.StopAll()

	// initial run + 2 restarts = 3 runs
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && countLines(t, marker) < 3 {
		time.Sleep(20 * time.Millisecond)
	}
	if got := countLines(t, marker); got != 3 {
		t.Fatalf("runs = %d, want 3 (initial + 2 restarts)", got)
	}
	// give it a chance to over-restart, then confirm it didn't
	time.Sleep(300 * time.Millisecond)
	if got := countLines(t, marker); got != 3 {
		t.Fatalf("runs = %d after settle, want exactly 3", got)
	}
}

func TestKilledProcessNotRestarted(t *testing.T) {
	cfg := &config.Process{
		Command: "sleep",
		Args:    []string{"30"},
		Type:    config.ProcessTypeService,
		Restart: &config.RestartPolicy{Enabled: true, MaxTries: 3, Delay: config.Duration{Duration: 10 * time.Millisecond}},
	}
	m := NewManager()
	p, err := m.StartProcess(context.Background(), "sleeper", cfg, nil)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := m.StopProcess("sleeper"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	waitForExit(t, p, 5*time.Second)
	time.Sleep(200 * time.Millisecond)
	if _, exists := m.GetProcess("sleeper"); exists {
		t.Fatal("killed process was restarted")
	}
}
```

- [ ] **Step 6: Run tests to verify they fail**

Run: `cd /home/jefflangston/dev/wrkmn && go test ./internal/process/ -run "TestRestartPolicy|TestKilledProcess" -v`
Expected: `TestRestartPolicyRestartsCrashedProcess` FAILs (runs = 1, no restarts happen). `TestKilledProcessNotRestarted` may already pass.

- [ ] **Step 7: Implement restart in the manager**

In `internal/process/manager.go`:

Add to `Manager` and `NewManager`:

```go
type Manager struct {
	processes    map[string]*Process
	restartTries map[string]int
	mu           sync.RWMutex
}

func NewManager() *Manager {
	return &Manager{
		processes:    make(map[string]*Process),
		restartTries: make(map[string]int),
	}
}
```

Add near the top of the file:

```go
// restartHealthyAfter: a run at least this long counts as a successful run,
// resetting the crash-loop counter.
const restartHealthyAfter = 10 * time.Second
```

At the END of `superviseProcess` (after the worker-status update), add:

```go
	m.maybeRestart(p)
```

Add the method:

```go
// maybeRestart re-launches a config-based worker/service whose restart policy
// allows it. Manual stops never reach here (killed processes return earlier in
// superviseProcess), and a manual stop also clears the try counter.
func (m *Manager) maybeRestart(p *Process) {
	cfg := p.Config
	if cfg == nil || cfg.Restart == nil || !cfg.Restart.Enabled {
		return
	}
	if cfg.Type != config.ProcessTypeWorker && cfg.Type != config.ProcessTypeService {
		return
	}

	m.mu.Lock()
	if time.Since(p.Started) >= restartHealthyAfter {
		m.restartTries[p.Name] = 0
	}
	tries := m.restartTries[p.Name]
	if tries >= cfg.Restart.MaxTries {
		m.mu.Unlock()
		return
	}
	m.restartTries[p.Name] = tries + 1
	m.mu.Unlock()

	time.Sleep(cfg.Restart.Delay.Duration)

	// A manual stop while we slept removes the map entry — don't resurrect.
	m.mu.RLock()
	_, stillTracked := m.processes[p.Name]
	m.mu.RUnlock()
	if !stillTracked {
		return
	}

	if _, err := m.StartProcess(context.Background(), p.Name, cfg, p.Worker); err == nil && p.Worker != nil {
		p.Worker.Status = worker.StatusRunning
	}
}
```

Clear counters on manual stops: in `StopWorker` add `delete(m.restartTries, workerName)` next to the `delete(m.processes, workerName)`; in `StopProcess` add `delete(m.restartTries, name)` next to its delete; in `StopAll` add `m.restartTries = make(map[string]int)` next to `m.processes = make(map[string]*Process)`.

- [ ] **Step 8: Run tests to verify they pass**

Run: `cd /home/jefflangston/dev/wrkmn && go build ./... && go test ./internal/process/ -v`
Expected: PASS (all, including existing suite — restart only fires when `Restart.Enabled`, which no existing test sets).

- [ ] **Step 9: Commit**

```bash
cd /home/jefflangston/dev/wrkmn
git add internal/config/loader.go internal/config/loader_test.go internal/process/manager.go internal/process/manager_test.go
git commit -m "process: implement restart policy (inherited from defaults, crash-loop capped)"
```

---

### Task 4: Env-file severity by provenance

**Files:**
- Modify: `internal/config/envfile.go`
- Modify: `internal/ui/table.go` (`startWorker` ~line 337, `startAllWorkers` ~line 403)
- Test: `internal/config/envfile_test.go` (append)

**Interfaces:**
- Consumes: existing `ParseEnvFile`, `ResolveEnvFiles`, `ProcessDefaults`.
- Produces: `type EnvFileRef struct { Path string; FromDefault bool }`; `func ResolveEnvFilePaths(proc *Process, defaults ProcessDefaults, baseDir string) []EnvFileRef` (used by Task 7's preflight); `ResolveEnvFiles` now returns an error when an explicitly configured env file is missing/unreadable (defaulted files stay best-effort).

- [ ] **Step 1: Write the failing tests**

Append to `internal/config/envfile_test.go` (match existing imports; add `"os"`, `"path/filepath"`, `"testing"` if missing):

```go
func TestResolveEnvFilesExplicitMissingIsError(t *testing.T) {
	dir := t.TempDir()
	proc := &Process{WorkingDir: dir, EnvFile: "nope.env"}
	if _, err := ResolveEnvFiles(proc, ProcessDefaults{}, dir); err == nil {
		t.Fatal("expected error for missing explicit env_file, got nil")
	}
}

func TestResolveEnvFilesExplicitListMissingIsError(t *testing.T) {
	dir := t.TempDir()
	proc := &Process{WorkingDir: dir, EnvFiles: []string{"nope.env"}}
	if _, err := ResolveEnvFiles(proc, ProcessDefaults{}, dir); err == nil {
		t.Fatal("expected error for missing explicit env_files entry, got nil")
	}
}

func TestResolveEnvFilesDefaultMissingIsSkipped(t *testing.T) {
	dir := t.TempDir()
	proc := &Process{WorkingDir: dir, Environment: map[string]string{"A": "1"}}
	env, err := ResolveEnvFiles(proc, ProcessDefaults{EnvFile: "absent.dev.vars"}, dir)
	if err != nil {
		t.Fatalf("defaulted missing env file must be skipped, got error: %v", err)
	}
	if env["A"] != "1" {
		t.Fatalf("env = %v, want inline environment preserved", env)
	}
}

func TestResolveEnvFilePathsProvenance(t *testing.T) {
	dir := t.TempDir()
	proc := &Process{WorkingDir: dir, EnvFile: ".dev.vars"}
	refs := ResolveEnvFilePaths(proc, ProcessDefaults{EnvFile: "shared.env"}, dir)
	if len(refs) != 2 {
		t.Fatalf("refs = %d, want 2", len(refs))
	}
	if !refs[0].FromDefault || refs[0].Path != filepath.Join(dir, "shared.env") {
		t.Fatalf("refs[0] = %+v, want defaults ref first", refs[0])
	}
	if refs[1].FromDefault || refs[1].Path != filepath.Join(dir, ".dev.vars") {
		t.Fatalf("refs[1] = %+v, want explicit ref", refs[1])
	}
}
```

(If `os` ends up unused after writing, drop it from imports.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/jefflangston/dev/wrkmn && go test ./internal/config/ -run TestResolveEnvFile -v`
Expected: compile error (`EnvFileRef` undefined) and/or FAILs (no error returned for missing explicit file).

- [ ] **Step 3: Rework envfile.go**

Replace `ResolveEnvFiles` in `internal/config/envfile.go` (lines 62-131) with (add `"fmt"` to the imports):

```go
// EnvFileRef is a resolved env-file path plus its provenance. Files inherited
// from defaults are best-effort; files a process asked for explicitly must exist.
type EnvFileRef struct {
	Path        string
	FromDefault bool
}

// ResolveEnvFilePaths returns the env files that apply to a process in merge
// order (lowest priority first). Relative paths resolve against baseDir for
// defaults.env_file and against the process working dir otherwise.
func ResolveEnvFilePaths(proc *Process, defaults ProcessDefaults, baseDir string) []EnvFileRef {
	workingDir := proc.WorkingDir
	if workingDir == "" {
		workingDir = "."
	}
	if !filepath.IsAbs(workingDir) {
		workingDir = filepath.Join(baseDir, workingDir)
	}

	var refs []EnvFileRef
	if defaults.EnvFile != "" {
		path := defaults.EnvFile
		if !filepath.IsAbs(path) {
			path = filepath.Join(baseDir, path)
		}
		refs = append(refs, EnvFileRef{Path: path, FromDefault: true})
	}
	for _, file := range proc.EnvFiles {
		path := file
		if !filepath.IsAbs(path) {
			path = filepath.Join(workingDir, file)
		}
		refs = append(refs, EnvFileRef{Path: path})
	}
	if proc.EnvFile != "" {
		path := proc.EnvFile
		if !filepath.IsAbs(path) {
			path = filepath.Join(workingDir, path)
		}
		refs = append(refs, EnvFileRef{Path: path})
	}
	return refs
}

// ResolveEnvFiles loads and merges environment files for a process.
// Merge priority (lowest to highest):
//  1. defaults.env_file — best-effort: a missing file is skipped (it may not
//     apply to every process)
//  2. defaults.environment
//  3. process.env_files (in array order) — missing file is an error
//  4. process.env_file — missing file is an error
//  5. process.environment
func ResolveEnvFiles(proc *Process, defaults ProcessDefaults, baseDir string) (map[string]string, error) {
	env := make(map[string]string)

	var explicit []EnvFileRef
	for _, ref := range ResolveEnvFilePaths(proc, defaults, baseDir) {
		if ref.FromDefault {
			if vars, err := ParseEnvFile(ref.Path); err == nil {
				for k, v := range vars {
					env[k] = v
				}
			}
			continue
		}
		explicit = append(explicit, ref)
	}

	for k, v := range defaults.Environment {
		env[k] = v
	}

	for _, ref := range explicit {
		vars, err := ParseEnvFile(ref.Path)
		if err != nil {
			return nil, fmt.Errorf("env file %s: %w", ref.Path, err)
		}
		for k, v := range vars {
			env[k] = v
		}
	}

	for k, v := range proc.Environment {
		env[k] = v
	}

	return env, nil
}
```

- [ ] **Step 4: Surface the error at both UI call sites**

In `internal/ui/table.go` `startWorker`, replace (lines 336-340):

```go
			// Resolve env files and merge with inline environment
			resolvedEnv, _ := config.ResolveEnvFiles(&procCopy, m.config.Defaults, m.config.BaseDir)
			if len(resolvedEnv) > 0 {
				procCopy.Environment = resolvedEnv
			}
```

with:

```go
			// Resolve env files and merge with inline environment. An
			// explicitly configured env file that's missing fails the start.
			resolvedEnv, envErr := config.ResolveEnvFiles(&procCopy, m.config.Defaults, m.config.BaseDir)
			if envErr != nil {
				w.Status = worker.StatusFailed
				m.statusMsg = fmt.Sprintf("%s: %v", workerName, envErr)
				return m, nil
			}
			if len(resolvedEnv) > 0 {
				procCopy.Environment = resolvedEnv
			}
```

In `startAllWorkers`, replace (lines 402-406):

```go
				// Resolve env files and merge with inline environment
				resolvedEnv, _ := config.ResolveEnvFiles(&procCopy, m.config.Defaults, m.config.BaseDir)
				if len(resolvedEnv) > 0 {
					procCopy.Environment = resolvedEnv
				}
```

with:

```go
				// Resolve env files; a missing explicit env file fails this
				// worker's start but doesn't block the others.
				resolvedEnv, envErr := config.ResolveEnvFiles(&procCopy, m.config.Defaults, m.config.BaseDir)
				if envErr != nil {
					w.Status = worker.StatusFailed
					m.statusMsg = fmt.Sprintf("%s: %v", w.Name, envErr)
					continue
				}
				if len(resolvedEnv) > 0 {
					procCopy.Environment = resolvedEnv
				}
```

(Note: Task 6 rewrites `startAllWorkers` and moves this into the scheduler starter; apply here anyway so this task stands alone.)

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /home/jefflangston/dev/wrkmn && go build ./... && go test ./internal/config/ ./internal/ui/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd /home/jefflangston/dev/wrkmn
git add internal/config/envfile.go internal/config/envfile_test.go internal/ui/table.go
git commit -m "config: explicit env files must exist; surface resolution errors in the TUI"
```

---

### Task 5: `oneshot` type + `process_completed` condition

**Files:**
- Modify: `internal/config/types.go`, `internal/config/loader.go`
- Modify: `internal/process/dependency.go`, `internal/process/scheduler.go`
- Test: `internal/config/loader_test.go` (append), `internal/process/scheduler_test.go` (append)

**Interfaces:**
- Consumes: Task 2's `RunState()`/`ExitCode()`.
- Produces: `config.ProcessTypeOneshot`, `config.ConditionCompleted`; `process.StateCompleted` (inserted between `StateRunning` and `StateStopped`); `process.EventCompleted`; scheduler waits for oneshot exit; `waitForDependency`/`dependenciesSatisfied` handle the new condition.

- [ ] **Step 1: Write the failing config validation tests**

Append to `internal/config/loader_test.go`:

```go
func TestUnknownProcessTypeRejected(t *testing.T) {
	path := writeTempConfig(t, `
version: "1.0"
processes:
  api:
    command: bun
    type: bogus
`)
	if _, err := NewLoader(path).Load(); err == nil {
		t.Fatal("expected error for unknown type, got nil")
	}
}

func TestOneshotWithHealthCheckRejected(t *testing.T) {
	path := writeTempConfig(t, `
version: "1.0"
processes:
  setup:
    command: sh
    type: oneshot
    health_check:
      type: tcp
      target: localhost:5432
`)
	if _, err := NewLoader(path).Load(); err == nil {
		t.Fatal("expected error for oneshot with health_check, got nil")
	}
}

func TestProcessCompletedRequiresOneshotTarget(t *testing.T) {
	path := writeTempConfig(t, `
version: "1.0"
processes:
  db:
    command: docker
    type: service
  app:
    command: bun
    type: worker
    depends_on:
      - process: db
        condition: process_completed
`)
	if _, err := NewLoader(path).Load(); err == nil {
		t.Fatal("expected error for process_completed on non-oneshot target, got nil")
	}
}

func TestOneshotWithCompletedDependentAccepted(t *testing.T) {
	path := writeTempConfig(t, `
version: "1.0"
processes:
  setup:
    command: sh
    args: [-c, "true"]
    type: oneshot
  app:
    command: bun
    type: worker
    depends_on:
      - process: setup
        condition: process_completed
`)
	if _, err := NewLoader(path).Load(); err != nil {
		t.Fatalf("valid oneshot config rejected: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/jefflangston/dev/wrkmn && go test ./internal/config/ -run "TestUnknownProcessType|TestOneshot|TestProcessCompleted" -v`
Expected: FAIL (unknown type currently accepted; `process_completed` currently rejected as invalid condition — so the "Accepted" test fails too).

- [ ] **Step 3: Add the config constants**

In `internal/config/types.go`, extend the `ProcessType` const block:

```go
const (
	ProcessTypeWorker  ProcessType = "worker"
	ProcessTypeService ProcessType = "service"
	// ProcessTypeOneshot runs once; exit 0 marks it completed, anything else failed.
	ProcessTypeOneshot ProcessType = "oneshot"
)
```

Extend the `DependencyCondition` const block:

```go
const (
	ConditionStarted   DependencyCondition = "process_started"
	ConditionHealthy   DependencyCondition = "process_healthy"
	// ConditionCompleted is satisfied when a oneshot dependency exits 0.
	ConditionCompleted DependencyCondition = "process_completed"
)
```

- [ ] **Step 4: Add loader validation**

In `internal/config/loader.go` `validate`:

Add a new block before the dev_mode validation (~line 152):

```go
	// Validate process types
	for name, proc := range cfg.Processes {
		switch proc.Type {
		case "", ProcessTypeWorker, ProcessTypeService, ProcessTypeOneshot:
			// Valid (empty defaults later)
		default:
			return fmt.Errorf("process %q: invalid type %q (must be worker, service, or oneshot)", name, proc.Type)
		}
		if proc.Type == ProcessTypeOneshot && proc.HealthCheck != nil {
			return fmt.Errorf("process %q: oneshot processes cannot have a health_check (they exit when done)", name)
		}
	}
```

In the dependency-conditions loop (~line 134), change the switch and add the target check so the loop body becomes:

```go
		for _, dep := range proc.DependsOn {
			switch dep.Condition {
			case ConditionStarted, ConditionHealthy, ConditionCompleted:
				// Valid
			case "":
				return fmt.Errorf("process %q: depends_on[%s].condition is required", name, dep.Process)
			default:
				return fmt.Errorf("process %q: invalid depends_on[%s].condition %q (must be process_started, process_healthy, or process_completed)", name, dep.Process, dep.Condition)
			}

			// Check that dependency exists
			target, exists := cfg.Processes[dep.Process]
			if !exists {
				return fmt.Errorf("process %q: depends on unknown process %q", name, dep.Process)
			}
			if dep.Condition == ConditionCompleted && target.Type != ProcessTypeOneshot {
				return fmt.Errorf("process %q: process_completed requires %q to be type oneshot", name, dep.Process)
			}
		}
```

- [ ] **Step 5: Run config tests to verify they pass**

Run: `cd /home/jefflangston/dev/wrkmn && go test ./internal/config/ -v`
Expected: PASS.

- [ ] **Step 6: Write the failing scheduler tests**

Append to `internal/process/scheduler_test.go` (add imports `"os"`, `"path/filepath"`, `"strings"` as needed; `config`, `context`, `time`, `testing` are already there):

```go
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

	order := readOrder(t, marker)
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
```

- [ ] **Step 7: Run tests to verify they fail**

Run: `cd /home/jefflangston/dev/wrkmn && go test ./internal/process/ -run TestOneshot -v`
Expected: compile error (`StateCompleted` undefined).

- [ ] **Step 8: Add StateCompleted and fix dependency-state checks**

In `internal/process/dependency.go`, change the `ProcessState` consts (insert `StateCompleted` after `StateRunning` — note this renumbers `StateStopped`/`StateFailed`, which is safe because nothing persists these ints):

```go
const (
	StatePending ProcessState = iota
	StateStarting
	StateRunning
	StateCompleted // a oneshot that exited 0
	StateStopped
	StateFailed
)
```

Add to its `String()` switch: `case StateCompleted: return "completed"`.

In `dependenciesSatisfied`, replace the switch with explicit-state checks (the old `<`/`>=` comparisons wrongly treated Stopped/Failed as "started"):

```go
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
```

In `internal/process/scheduler.go` `waitForDependency`, replace the condition switch:

```go
			switch dep.Condition {
			case config.ConditionStarted:
				if depNode.State == StateRunning || depNode.State == StateCompleted {
					return nil
				}
				if depNode.State == StateFailed {
					return fmt.Errorf("dependency %s failed", dep.Process)
				}
			case config.ConditionHealthy:
				if depNode.Health == HealthHealthy {
					return nil
				}
				if depNode.State == StateFailed {
					return fmt.Errorf("dependency %s failed", dep.Process)
				}
			case config.ConditionCompleted:
				if depNode.State == StateCompleted {
					return nil
				}
				if depNode.State == StateFailed {
					return fmt.Errorf("dependency %s failed", dep.Process)
				}
			}
```

- [ ] **Step 9: Oneshot handling in the scheduler**

In `internal/process/scheduler.go`, add `EventCompleted` to the event consts and `String()`:

```go
const (
	EventStarting StartEventType = iota
	EventStarted
	EventHealthy
	EventFailed
	EventWaiting
	EventCompleted
)
```

and in `String()`: `case EventCompleted: return "completed"`.

In `startProcess`, immediately after the `EventStarted` send (~line 207), add:

```go
	// One-shot processes: wait for the command to finish. Exit 0 completes the
	// node (satisfying process_started and process_completed); anything else
	// fails it and dependents fail fast.
	if node.Config.Type == config.ProcessTypeOneshot {
		return s.waitForCompletion(ctx, name, proc)
	}
```

Add the method:

```go
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
```

- [ ] **Step 10: Run tests to verify they pass**

Run: `cd /home/jefflangston/dev/wrkmn && go build ./... && go test ./internal/process/ ./internal/config/ -v`
Expected: PASS, including the pre-existing dependency/scheduler tests (if one asserted the old numeric state ordering or `>= StateRunning` behavior, update it to the explicit-state semantics — the new behavior is intended).

- [ ] **Step 11: Commit**

```bash
cd /home/jefflangston/dev/wrkmn
git add internal/config/types.go internal/config/loader.go internal/config/loader_test.go internal/process/dependency.go internal/process/scheduler.go internal/process/scheduler_test.go
git commit -m "process: add oneshot type and process_completed dependency condition"
```

---

### Task 6: Wire the scheduler into the TUI's "start all"

**Files:**
- Modify: `internal/process/scheduler.go` (idempotency guard + CloseEvents)
- Modify: `internal/ui/table.go` (`startAllWorkers`, ~line 379)
- Test: `internal/ui/table_startall_test.go` (create)

**Interfaces:**
- Consumes: `process.NewDependencyGraph`, `process.NewScheduler`, `Scheduler.StartAll/Events`, Task 4's `ResolveEnvFiles` error, Task 5's states/events.
- Produces: `(s *Scheduler) CloseEvents()`; `startProcess` skips already-running/completed nodes; the `a` key honors `depends_on` for config processes (discovered workers keep starting directly).

- [ ] **Step 1: Write the failing test**

Create `internal/ui/table_startall_test.go`:

```go
package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"wrkmn/internal/config"
	"wrkmn/internal/worker"
)

func TestStartAllRespectsDependencies(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "order")
	cfg := &config.Config{
		BaseDir: dir,
		Processes: map[string]config.Process{
			"db": {
				Command: "sh",
				Args:    []string{"-c", "echo db >> " + marker + "; sleep 30"},
				Type:    config.ProcessTypeService,
			},
			"app": {
				Command: "sh",
				Args:    []string{"-c", "echo app >> " + marker + "; sleep 30"},
				Type:    config.ProcessTypeService,
				DependsOn: []config.Dependency{
					{Process: "db", Condition: config.ConditionStarted},
				},
			},
		},
	}
	workers := []*worker.Worker{
		{Name: "db", Status: worker.StatusStopped, Scripts: map[string]string{"_config": "sh"}},
		{Name: "app", Status: worker.StatusStopped, Scripts: map[string]string{"_config": "sh"}},
	}
	m := NewTableModel(workers, cfg)
	defer m.GetManager().StopAll()

	m2, cmd := m.startAllWorkers()
	if cmd == nil {
		t.Fatal("startAllWorkers returned no command")
	}
	msg := cmd() // runs the scheduler synchronously
	if _, ok := msg.(allProcessesStartedMsg); !ok {
		t.Fatalf("msg = %T, want allProcessesStartedMsg", msg)
	}
	_ = m2

	deadline := time.Now().Add(5 * time.Second)
	var order []string
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(marker)
		order = strings.Fields(string(data))
		if len(order) >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(order) != 2 || order[0] != "db" || order[1] != "app" {
		t.Fatalf("start order = %v, want [db app]", order)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/jefflangston/dev/wrkmn && go test ./internal/ui/ -run TestStartAllRespects -v`
Expected: FAIL — with today's code both processes start simultaneously, so the order is nondeterministic; the test may flake-pass. Run it with `-count=5`: at least one run should produce `[app db]`. (If it passes 5/5, proceed anyway — the deterministic guarantee is what the implementation adds.)

- [ ] **Step 3: Scheduler additions**

In `internal/process/scheduler.go`:

At the top of `startProcess` (after the `node == nil` check), add:

```go
	// Already running or completed (e.g. started individually from the UI
	// before "start all") — nothing to do.
	if node.State == StateRunning || node.State == StateCompleted {
		return nil
	}
```

Add after the `Events()` method:

```go
// CloseEvents closes the event channel so range-consumers terminate. Call only
// after StartAll has returned and no further scheduler operations will run.
func (s *Scheduler) CloseEvents() {
	close(s.events)
}
```

- [ ] **Step 4: Rewrite `startAllWorkers`**

In `internal/ui/table.go`, replace the whole `startAllWorkers` function with:

```go
func (m TableModel) startAllWorkers() (TableModel, tea.Cmd) {
	// Discovered workers (no config entry) start directly; config-based
	// processes go through the scheduler so depends_on ordering, health
	// checks, and oneshot completion hold.
	type discoveredStart struct {
		w      *worker.Worker
		script string
	}

	var discovered []discoveredStart
	configProcs := make(map[string]config.Process)
	workerByName := make(map[string]*worker.Worker)

	for _, w := range m.workers {
		workerByName[w.Name] = w

		if _, isConfig := w.Scripts["_config"]; isConfig && m.config != nil {
			if proc, exists := m.config.Processes[w.Name]; exists {
				configProcs[w.Name] = proc
				if w.Status != worker.StatusRunning {
					w.Status = worker.StatusStarting
				}
				continue
			}
		}

		if w.Status == worker.StatusRunning {
			continue
		}
		script := "dev"
		if !w.HasScript("dev") && w.HasScript("start") {
			script = "start"
		}
		if w.HasScript(script) {
			w.Status = worker.StatusStarting
			discovered = append(discovered, discoveredStart{w: w, script: script})
		}
	}

	if len(discovered) == 0 && len(configProcs) == 0 {
		return m, nil
	}

	graph := process.NewDependencyGraph(configProcs)
	// Pre-mark nodes that are already running so the scheduler skips them but
	// dependents still see them as started/healthy.
	for name := range configProcs {
		if w := workerByName[name]; w != nil && w.Status == worker.StatusRunning {
			graph.UpdateState(name, process.StateRunning)
			graph.UpdateHealth(name, process.HealthHealthy)
			if p, exists := m.manager.GetProcess(name); exists {
				graph.SetProcess(name, p)
			}
		}
	}

	manager := m.manager
	cfg := m.config
	sched := process.NewScheduler(graph, func(ctx context.Context, name string, proc *config.Process) (*process.Process, error) {
		procCopy := *proc
		resolvedEnv, err := config.ResolveEnvFiles(&procCopy, cfg.Defaults, cfg.BaseDir)
		if err != nil {
			return nil, err
		}
		if len(resolvedEnv) > 0 {
			procCopy.Environment = resolvedEnv
		}
		return manager.StartProcess(ctx, name, &procCopy, workerByName[name])
	})

	// Reflect scheduler events in worker statuses; the 1s tick re-renders.
	go func() {
		for ev := range sched.Events() {
			w := workerByName[ev.Name]
			if w == nil {
				continue
			}
			switch ev.Type {
			case process.EventStarting, process.EventWaiting:
				w.Status = worker.StatusStarting
			case process.EventStarted, process.EventHealthy:
				w.Status = worker.StatusRunning
			case process.EventCompleted:
				w.Status = worker.StatusStopped // oneshot finished successfully
			case process.EventFailed:
				w.Status = worker.StatusFailed
			}
		}
	}()

	return m, func() tea.Msg {
		for _, ds := range discovered {
			manager.StartWorker(ds.w, ds.script)
		}
		if len(configProcs) > 0 {
			sched.StartAll(context.Background())
		}
		sched.CloseEvents()
		return allProcessesStartedMsg{}
	}
}
```

(This supersedes Task 4's `startAllWorkers` env-error edit — env resolution now lives in the starter closure, where an error fails that node and its dependents via the scheduler. The `fmt` usage from Task 4 remains in `startWorker`.)

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /home/jefflangston/dev/wrkmn && go build ./... && go test ./internal/ui/ -run TestStartAllRespects -count=5 -v && go test ./...`
Expected: 5/5 PASS in order `[db app]`; full suite PASS.

- [ ] **Step 6: Commit**

```bash
cd /home/jefflangston/dev/wrkmn
git add internal/process/scheduler.go internal/ui/table.go internal/ui/table_startall_test.go
git commit -m "ui: start-all goes through the dependency scheduler for config processes"
```

---

### Task 7: Pre-flight checks package

**Files:**
- Create: `internal/preflight/preflight.go`
- Test: `internal/preflight/preflight_test.go` (create)

**Interfaces:**
- Consumes: `config.Config`, Task 4's `config.ResolveEnvFilePaths`.
- Produces: `type Severity int` (`SeverityWarning`, `SeverityError`, `String()`); `type Issue struct { Severity Severity; Process string; Message string }`; `func Check(cfg *config.Config) []Issue`; `func HasErrors(issues []Issue) bool`. Consumed by Tasks 8 and 9.

- [ ] **Step 1: Write the failing tests**

Create `internal/preflight/preflight_test.go`:

```go
package preflight

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wrkmn/internal/config"
)

func issueMessages(issues []Issue) string {
	var b strings.Builder
	for _, i := range issues {
		fmt.Fprintf(&b, "[%s] %s: %s\n", i.Severity, i.Process, i.Message)
	}
	return b.String()
}

func findIssue(issues []Issue, process string, sev Severity, substr string) bool {
	for _, i := range issues {
		if i.Process == process && i.Severity == sev && strings.Contains(i.Message, substr) {
			return true
		}
	}
	return false
}

func TestMissingCommandIsError(t *testing.T) {
	cfg := &config.Config{BaseDir: t.TempDir(), Processes: map[string]config.Process{
		"api": {Command: "definitely-not-a-real-binary-xyz", Type: config.ProcessTypeService},
	}}
	issues := Check(cfg)
	if !findIssue(issues, "api", SeverityError, "not found on PATH") {
		t.Fatalf("missing command not reported:\n%s", issueMessages(issues))
	}
}

func TestBadWorkingDirIsError(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{BaseDir: dir, Processes: map[string]config.Process{
		"api": {Command: "sh", WorkingDir: filepath.Join(dir, "nope"), Type: config.ProcessTypeService},
	}}
	issues := Check(cfg)
	if !findIssue(issues, "api", SeverityError, "not a directory") {
		t.Fatalf("bad working_dir not reported:\n%s", issueMessages(issues))
	}
}

func TestEnvFileSeverityByProvenance(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		BaseDir:  dir,
		Defaults: config.ProcessDefaults{EnvFile: "shared-missing.env"},
		Processes: map[string]config.Process{
			"api": {Command: "sh", WorkingDir: dir, EnvFile: "explicit-missing.env", Type: config.ProcessTypeService},
		},
	}
	issues := Check(cfg)
	if !findIssue(issues, "api", SeverityError, "explicit-missing.env") {
		t.Fatalf("explicit missing env file should be an error:\n%s", issueMessages(issues))
	}
	if !findIssue(issues, "api", SeverityWarning, "shared-missing.env") {
		t.Fatalf("defaulted missing env file should be a warning:\n%s", issueMessages(issues))
	}
}

func TestDuplicateInspectorPortsIsError(t *testing.T) {
	cfg := &config.Config{BaseDir: t.TempDir(), Processes: map[string]config.Process{
		"a": {Command: "sh", InspectorPort: 9329, Type: config.ProcessTypeWorker},
		"b": {Command: "sh", InspectorPort: 9329, Type: config.ProcessTypeWorker},
	}}
	issues := Check(cfg)
	if !findIssue(issues, "", SeverityError, "9329") {
		t.Fatalf("duplicate inspector port not reported:\n%s", issueMessages(issues))
	}
}

func TestOccupiedInspectorPortIsWarning(t *testing.T) {
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	cfg := &config.Config{BaseDir: t.TempDir(), Processes: map[string]config.Process{
		"api": {Command: "sh", InspectorPort: port, Type: config.ProcessTypeWorker},
	}}
	issues := Check(cfg)
	if !findIssue(issues, "api", SeverityWarning, "already in use") {
		t.Fatalf("occupied inspector port not reported:\n%s", issueMessages(issues))
	}
}

func TestCleanConfigHasNoIssues(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".dev.vars"), []byte("A=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{BaseDir: dir, Processes: map[string]config.Process{
		"api": {Command: "sh", WorkingDir: dir, EnvFile: ".dev.vars", Type: config.ProcessTypeService},
	}}
	if issues := Check(cfg); len(issues) != 0 {
		t.Fatalf("clean config produced issues:\n%s", issueMessages(issues))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/jefflangston/dev/wrkmn && go test ./internal/preflight/ -v`
Expected: compile error (package doesn't exist).

- [ ] **Step 3: Implement the package**

Create `internal/preflight/preflight.go`:

```go
// Package preflight inspects a wrkmn configuration and the local environment
// for problems that would otherwise surface as cryptic child-process failures.
// It never starts anything.
package preflight

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"sort"
	"time"

	"wrkmn/internal/config"
)

// Severity ranks an issue: warnings are survivable, errors are not.
type Severity int

const (
	SeverityWarning Severity = iota
	SeverityError
)

func (s Severity) String() string {
	if s == SeverityError {
		return "error"
	}
	return "warning"
}

// Issue is a single pre-flight finding. Process is "" for config-level issues.
type Issue struct {
	Severity Severity
	Process  string
	Message  string
}

const dialTimeout = 250 * time.Millisecond

// Check inspects the configuration and environment. Deterministic order
// (processes sorted by name) so reports are stable.
func Check(cfg *config.Config) []Issue {
	var issues []Issue

	names := make([]string, 0, len(cfg.Processes))
	for name := range cfg.Processes {
		names = append(names, name)
	}
	sort.Strings(names)

	inspectorPorts := make(map[int][]string)

	for _, name := range names {
		proc := cfg.Processes[name]

		if _, err := exec.LookPath(proc.Command); err != nil {
			issues = append(issues, Issue{SeverityError, name, fmt.Sprintf("command %q not found on PATH", proc.Command)})
		}

		if proc.WorkingDir != "" {
			if info, err := os.Stat(proc.WorkingDir); err != nil || !info.IsDir() {
				issues = append(issues, Issue{SeverityError, name, fmt.Sprintf("working_dir %q is not a directory", proc.WorkingDir)})
			}
		}

		for _, ref := range config.ResolveEnvFilePaths(&proc, cfg.Defaults, cfg.BaseDir) {
			if _, err := os.Stat(ref.Path); err != nil {
				if ref.FromDefault {
					issues = append(issues, Issue{SeverityWarning, name, fmt.Sprintf("default env file %q not found (skipped at start)", ref.Path)})
				} else {
					issues = append(issues, Issue{SeverityError, name, fmt.Sprintf("env file %q not found", ref.Path)})
				}
			}
		}

		if proc.InspectorPort > 0 {
			inspectorPorts[proc.InspectorPort] = append(inspectorPorts[proc.InspectorPort], name)
			addr := fmt.Sprintf("localhost:%d", proc.InspectorPort)
			if conn, err := net.DialTimeout("tcp", addr, dialTimeout); err == nil {
				conn.Close()
				issues = append(issues, Issue{SeverityWarning, name, fmt.Sprintf("inspector port %d is already in use", proc.InspectorPort)})
			}
		}
	}

	ports := make([]int, 0, len(inspectorPorts))
	for port := range inspectorPorts {
		ports = append(ports, port)
	}
	sort.Ints(ports)
	for _, port := range ports {
		if procs := inspectorPorts[port]; len(procs) > 1 {
			sort.Strings(procs)
			issues = append(issues, Issue{SeverityError, "", fmt.Sprintf("inspector port %d used by multiple processes: %v", port, procs)})
		}
	}

	return issues
}

// HasErrors reports whether any issue is error severity.
func HasErrors(issues []Issue) bool {
	for _, issue := range issues {
		if issue.Severity == SeverityError {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/jefflangston/dev/wrkmn && go test ./internal/preflight/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/jefflangston/dev/wrkmn
git add internal/preflight/
git commit -m "preflight: add environment checks (commands, dirs, env files, inspector ports)"
```

---

### Task 8: `wrkmn validate` command

**Files:**
- Create: `cmd/validate.go`
- Modify: `cmd/root.go` (`init`, ~line 72: register command)
- Test: `cmd/validate_test.go` (create)

**Interfaces:**
- Consumes: `config.LoadOrDefault`/`NewLoader`, `preflight.Check`/`HasErrors`.
- Produces: `wrkmn validate [directory] [-c config]` subcommand; testable `runValidate(dir, cfgPath string, out io.Writer) int`.

- [ ] **Step 1: Write the failing tests**

Create `cmd/validate_test.go`:

```go
package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "worker-compose.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestValidateCleanConfig(t *testing.T) {
	dir := writeConfig(t, `
version: "1.0"
processes:
  api:
    command: sh
    args: [-c, "true"]
    type: service
`)
	var out strings.Builder
	if code := runValidate(dir, "", &out); code != 0 {
		t.Fatalf("exit code = %d, want 0; output:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "config valid") {
		t.Fatalf("output missing 'config valid':\n%s", out.String())
	}
}

func TestValidateInvalidConfigExitsNonzero(t *testing.T) {
	dir := writeConfig(t, `
version: "1.0"
processes:
  api:
    command: sh
    dev_mode: bogus
`)
	var out strings.Builder
	if code := runValidate(dir, "", &out); code != 1 {
		t.Fatalf("exit code = %d, want 1; output:\n%s", code, out.String())
	}
}

func TestValidatePreflightErrorExitsNonzero(t *testing.T) {
	dir := writeConfig(t, `
version: "1.0"
processes:
  api:
    command: definitely-not-a-real-binary-xyz
    type: service
`)
	var out strings.Builder
	if code := runValidate(dir, "", &out); code != 1 {
		t.Fatalf("exit code = %d, want 1; output:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "not found on PATH") {
		t.Fatalf("output missing preflight finding:\n%s", out.String())
	}
}

func TestValidateWarningsStillExitZero(t *testing.T) {
	dir := writeConfig(t, `
version: "1.0"
processes:
  api:
    command: sh
    args: [-c, "true"]
    type: service
defaults:
  env_file: missing-shared.env
`)
	var out strings.Builder
	if code := runValidate(dir, "", &out); code != 0 {
		t.Fatalf("exit code = %d, want 0 (warnings don't fail); output:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "missing-shared.env") {
		t.Fatalf("warning not printed:\n%s", out.String())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/jefflangston/dev/wrkmn && go test ./cmd/ -v`
Expected: compile error (`runValidate` undefined).

- [ ] **Step 3: Implement the command**

Create `cmd/validate.go`:

```go
package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"wrkmn/internal/config"
	"wrkmn/internal/preflight"
)

// validateCmd checks config + environment without starting any process.
var validateCmd = &cobra.Command{
	Use:   "validate [directory]",
	Short: "Validate configuration and environment without starting anything",
	Long: `Loads worker-compose.yaml, validates it (types, dependencies, cycles,
dev modes), and runs pre-flight checks: commands on PATH, working
directories, env files, inspector ports.

Exit code 0 when there are no errors (warnings allowed), 1 otherwise.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		dir := "."
		if len(args) > 0 {
			dir = args[0]
		}
		os.Exit(runValidate(dir, cfgFile, os.Stdout))
	},
}

// runValidate loads the config, runs pre-flight checks, writes a report, and
// returns the process exit code.
func runValidate(dir, cfgPath string, out io.Writer) int {
	var cfg *config.Config
	var err error
	if cfgPath != "" {
		cfg, err = config.NewLoader(cfgPath).Load()
	} else {
		cfg, err = config.LoadOrDefault(dir)
	}
	if err != nil {
		fmt.Fprintf(out, "✗ config: %v\n", err)
		return 1
	}

	fmt.Fprintf(out, "✓ config valid (%d processes)\n", len(cfg.Processes))

	issues := preflight.Check(cfg)
	for _, issue := range issues {
		scope := issue.Process
		if scope == "" {
			scope = "config"
		}
		marker := "⚠"
		if issue.Severity == preflight.SeverityError {
			marker = "✗"
		}
		fmt.Fprintf(out, "%s %s: %s\n", marker, scope, issue.Message)
	}

	if preflight.HasErrors(issues) {
		return 1
	}
	if len(issues) == 0 {
		fmt.Fprintln(out, "✓ pre-flight checks passed")
	}
	return 0
}
```

In `cmd/root.go` `init()`, add alongside the other `AddCommand` calls:

```go
	rootCmd.AddCommand(validateCmd)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/jefflangston/dev/wrkmn && go build ./... && go test ./cmd/ -v`
Expected: PASS.

- [ ] **Step 5: Smoke test**

Run: `cd /home/jefflangston/dev/wrkmn && go build -o wrkmn . && ./wrkmn validate . ; echo "exit=$?"`
Expected: readable report, sensible exit code for this repo (no config file → default config, 0 processes, exit 0).

- [ ] **Step 6: Commit**

```bash
cd /home/jefflangston/dev/wrkmn
git add cmd/validate.go cmd/validate_test.go cmd/root.go
git commit -m "cmd: add 'wrkmn validate' non-interactive config + pre-flight check"
```

---

### Task 9: Pre-flight gate before the TUI starts

**Files:**
- Modify: `cmd/root.go` (`runApp` after config load ~line 111; flags in `init`)

**Interfaces:**
- Consumes: `preflight.Check`/`HasErrors`.
- Produces: `--skip-preflight` flag; errors block TUI launch (exit 1); warnings pause for Enter.

- [ ] **Step 1: Add the flag**

In `cmd/root.go` var block, add:

```go
	skipPreflight bool
```

In `init()`, next to the `no-discovery` flag:

```go
	rootCmd.Flags().BoolVar(&skipPreflight, "skip-preflight", false, "skip pre-flight environment checks")
```

- [ ] **Step 2: Add the gate**

In `runApp`, immediately after `config.ApplyUIConfig(cfg)` (~line 111), add (import `wrkmn/internal/preflight` and `bufio`):

```go
	// Pre-flight: surface environment problems before any process starts.
	// Errors block; warnings pause so they're seen before the alt-screen TUI
	// hides them.
	if !skipPreflight {
		issues := preflight.Check(cfg)
		if len(issues) > 0 {
			for _, issue := range issues {
				scope := issue.Process
				if scope == "" {
					scope = "config"
				}
				fmt.Fprintf(os.Stderr, "[%s] %s: %s\n", issue.Severity, scope, issue.Message)
			}
			if preflight.HasErrors(issues) {
				fmt.Fprintln(os.Stderr, "\nPre-flight errors — fix them, or run 'wrkmn validate' for the full report. (--skip-preflight to bypass)")
				os.Exit(1)
			}
			fmt.Fprint(os.Stderr, "\nWarnings above — press Enter to continue, Ctrl+C to abort... ")
			bufio.NewReader(os.Stdin).ReadString('\n')
		}
	}
```

- [ ] **Step 3: Build and smoke test**

Run: `cd /home/jefflangston/dev/wrkmn && go build -o wrkmn . && go test ./cmd/`
Then create a broken config in the scratchpad and verify the gate:

```bash
mkdir -p /tmp/claude-1000/-home-jefflangston-dev-wrkmn/c5a98c24-a166-45bb-8a58-82d68f5ae28f/scratchpad/gate-test
cat > /tmp/claude-1000/-home-jefflangston-dev-wrkmn/c5a98c24-a166-45bb-8a58-82d68f5ae28f/scratchpad/gate-test/worker-compose.yaml <<'EOF'
version: "1.0"
processes:
  api:
    command: definitely-not-a-real-binary-xyz
    type: service
EOF
./wrkmn /tmp/claude-1000/-home-jefflangston-dev-wrkmn/c5a98c24-a166-45bb-8a58-82d68f5ae28f/scratchpad/gate-test ; echo "exit=$?"
```

Expected: prints the `[error] api: command ... not found on PATH` line and exits 1 without launching the TUI.

- [ ] **Step 4: Commit**

```bash
cd /home/jefflangston/dev/wrkmn
git add cmd/root.go
git commit -m "cmd: gate TUI startup on pre-flight checks (--skip-preflight to bypass)"
```

---

### Task 10: Persist runtime selections (dev mode + env)

**Files:**
- Create: `internal/state/state.go`
- Test: `internal/state/state_test.go` (create)
- Modify: `cmd/root.go` (apply after workers built, ~line 155)
- Modify: `internal/ui/table.go` (save hooks in `Update`, ~lines 180-190; new `saveState` helper)
- Modify: `.gitignore` (create or append)

**Interfaces:**
- Consumes: `worker.Worker` (`DevMode`, `SelectedEnv`), `config.DevMode` constants.
- Produces: `state.Load(baseDir) (map[string]WorkerState, error)`, `state.Save(baseDir string, workers []*worker.Worker) error`, `state.Apply(states map[string]WorkerState, workers []*worker.Worker)`, `state.FileName = ".wrkmn-state.json"`.

- [ ] **Step 1: Write the failing tests**

Create `internal/state/state_test.go`:

```go
package state

import (
	"os"
	"path/filepath"
	"testing"

	"wrkmn/internal/config"
	"wrkmn/internal/worker"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	workers := []*worker.Worker{
		{Name: "api", DevMode: config.DevModeLocal, SelectedEnv: "staging"},
		{Name: "web"}, // nothing selected — should not be persisted
	}
	if err := Save(dir, workers); err != nil {
		t.Fatalf("save: %v", err)
	}
	states, err := Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := states["api"]; got.DevMode != "local" || got.SelectedEnv != "staging" {
		t.Fatalf("api state = %+v", got)
	}
	if _, ok := states["web"]; ok {
		t.Fatal("worker with no selections should not be persisted")
	}
}

func TestLoadMissingFileIsEmpty(t *testing.T) {
	states, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(states) != 0 {
		t.Fatalf("states = %v, want empty", states)
	}
}

func TestApplyOverridesDefaultsAndIgnoresBogusModes(t *testing.T) {
	workers := []*worker.Worker{
		{Name: "api", DevMode: config.DevModeRemoteBindings},
		{Name: "web"},
	}
	Apply(map[string]WorkerState{
		"api": {DevMode: "local", SelectedEnv: "staging"},
		"web": {DevMode: "not-a-mode"},
	}, workers)
	if workers[0].DevMode != config.DevModeLocal || workers[0].SelectedEnv != "staging" {
		t.Fatalf("api = %+v, want saved selections applied", workers[0])
	}
	if workers[1].DevMode != "" {
		t.Fatalf("web.DevMode = %q, bogus saved mode must be ignored", workers[1].DevMode)
	}
}

func TestLoadCorruptFileIsError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("{nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("expected error for corrupt state file")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/jefflangston/dev/wrkmn && go test ./internal/state/ -v`
Expected: compile error (package doesn't exist).

- [ ] **Step 3: Implement the package**

Create `internal/state/state.go`:

```go
// Package state persists per-worker runtime selections (dev mode, selected
// env) across wrkmn sessions in a .wrkmn-state.json next to the config file.
// The file is a convenience cache: losing it only loses UI selections.
package state

import (
	"encoding/json"
	"os"
	"path/filepath"

	"wrkmn/internal/config"
	"wrkmn/internal/worker"
)

// FileName is the state file created in the project's base directory. Add it
// to the project's .gitignore.
const FileName = ".wrkmn-state.json"

// WorkerState holds the runtime selections worth keeping for one worker.
type WorkerState struct {
	DevMode     string `json:"dev_mode,omitempty"`
	SelectedEnv string `json:"selected_env,omitempty"`
}

type fileFormat struct {
	Workers map[string]WorkerState `json:"workers"`
}

// Load reads saved selections. A missing file yields an empty map, not an error.
func Load(baseDir string) (map[string]WorkerState, error) {
	data, err := os.ReadFile(filepath.Join(baseDir, FileName))
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]WorkerState{}, nil
		}
		return nil, err
	}
	var f fileFormat
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	if f.Workers == nil {
		f.Workers = map[string]WorkerState{}
	}
	return f.Workers, nil
}

// Save writes the selections of every worker that has any. Atomic via
// write-to-temp + rename.
func Save(baseDir string, workers []*worker.Worker) error {
	f := fileFormat{Workers: map[string]WorkerState{}}
	for _, w := range workers {
		if w.DevMode == "" && w.SelectedEnv == "" {
			continue
		}
		f.Workers[w.Name] = WorkerState{
			DevMode:     string(w.DevMode),
			SelectedEnv: w.SelectedEnv,
		}
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(baseDir, FileName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Apply overlays saved selections onto workers. Saved values win over config
// defaults — they are the user's most recent explicit choice. Unknown dev
// modes (hand-edited or stale files) are ignored.
func Apply(states map[string]WorkerState, workers []*worker.Worker) {
	for _, w := range workers {
		st, ok := states[w.Name]
		if !ok {
			continue
		}
		switch config.DevMode(st.DevMode) {
		case config.DevModeLocal, config.DevModeRemoteBindings, config.DevModeRemote:
			w.DevMode = config.DevMode(st.DevMode)
		}
		if st.SelectedEnv != "" {
			w.SelectedEnv = st.SelectedEnv
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/jefflangston/dev/wrkmn && go test ./internal/state/ -v`
Expected: PASS.

- [ ] **Step 5: Load in cmd/root.go, save in the TUI**

In `cmd/root.go` `runApp`, after the discovery block finishes building `workers` (right before the `if len(workers) == 0` check), add (import `wrkmn/internal/state`):

```go
	// Restore runtime selections (dev mode, selected env) from prior sessions.
	if saved, err := state.Load(cfg.BaseDir); err == nil {
		state.Apply(saved, workers)
	}
```

In `internal/ui/table.go` `Update`, replace the two selector dispatch blocks:

```go
			// Mode selector
			if m.modeSelector.Visible() {
				cmd := m.modeSelector.Update(msg)
				return m, cmd
			}

			// Handle env selector first if visible
			if m.envSelector.Visible() {
				cmd := m.envSelector.Update(msg)
				return m, cmd
			}
```

with:

```go
			// Mode selector
			if m.modeSelector.Visible() {
				cmd := m.modeSelector.Update(msg)
				if !m.modeSelector.Visible() {
					m.saveState() // selector closed — persist any selection
				}
				return m, cmd
			}

			// Handle env selector first if visible
			if m.envSelector.Visible() {
				cmd := m.envSelector.Update(msg)
				if !m.envSelector.Visible() {
					m.saveState() // selector closed — persist any selection
				}
				return m, cmd
			}
```

Add the helper near `GetManager` (import `wrkmn/internal/state`):

```go
// saveState persists runtime selections. Best-effort: the TUI must not break
// because the state file can't be written.
func (m TableModel) saveState() {
	if m.config == nil {
		return
	}
	_ = state.Save(m.config.BaseDir, m.workers)
}
```

- [ ] **Step 6: Ignore the state file in this repo**

`.wrkmn-state.json` lands in whatever project wrkmn runs against — including this repo during smoke tests:

```bash
cd /home/jefflangston/dev/wrkmn
grep -qxF '.wrkmn-state.json' .gitignore 2>/dev/null || echo '.wrkmn-state.json' >> .gitignore
```

- [ ] **Step 7: Build, test, commit**

Run: `cd /home/jefflangston/dev/wrkmn && go build ./... && go test ./...`
Expected: PASS.

```bash
cd /home/jefflangston/dev/wrkmn
git add internal/state/ cmd/root.go internal/ui/table.go .gitignore
git commit -m "state: persist per-worker dev mode and env selections across sessions"
```

---

### Task 11: Documentation + final verification

**Files:**
- Modify: `README.md`, `CLAUDE.md`

**Interfaces:**
- Consumes: everything above.
- Produces: docs matching actual behavior.

- [ ] **Step 1: Update README.md**

Make these edits (adapt to the README's existing structure):

1. In the Dev Modes / Process Types area, add a **Process Types** row/entry for `oneshot`: "runs once; exit 0 marks it completed; dependents can wait on it with `condition: process_completed`".
2. Document restart policy under configuration:

```markdown
### Restart policy

```yaml
defaults:
  restart:
    enabled: true     # restart worker/service processes that crash
    max_tries: 3      # consecutive crash restarts before giving up (default 3)
    delay: 1s         # wait between restarts (default 1s)
```

Applies to config-defined `worker`/`service` processes. A run lasting 10s+
resets the crash counter; manual stops never trigger a restart. `oneshot`
processes are never restarted.
```

3. Document dependency conditions including `process_completed` (waits for a `oneshot` to exit 0).
4. Add a **Validation** section:

```markdown
## Validation

```bash
wrkmn validate [directory]   # check config + environment, start nothing
```

Checks: YAML validity, dependency cycles, process types, dev modes, commands
on PATH, working directories, env files (explicit = error, inherited default =
warning), inspector port conflicts. Exit 0 unless errors.

The TUI runs the same checks at startup: errors block, warnings pause for
Enter. Bypass with `--skip-preflight`.
```

5. Add a note that runtime selections persist:

```markdown
Dev mode (`m`) and environment (`e`) selections persist across sessions in
`.wrkmn-state.json` next to your config — add it to your project's .gitignore.
```

6. Add `--skip-preflight` to the CLI flags table.

- [ ] **Step 2: Update CLAUDE.md**

1. **Process Types** section: add `oneshot` line.
2. **Dependency Conditions**: add `process_completed - Wait for a oneshot to exit 0`.
3. CLI flags table: add `--skip-preflight`.
4. Package structure tree: add `internal/preflight/` ("environment pre-flight checks") and `internal/state/` ("persisted runtime selections"), and `cmd/validate.go`.
5. Data Flow: note that "start all" routes config processes through the Scheduler, and that pre-flight runs before the TUI starts.
6. Key Concepts: brief restart-policy paragraph mirroring the README's.

- [ ] **Step 3: Full verification**

Run: `cd /home/jefflangston/dev/wrkmn && gofmt -l . && go vet ./... && go build ./... && go test ./...`
Expected: gofmt prints nothing; vet, build, and all tests clean.

- [ ] **Step 4: Commit**

```bash
cd /home/jefflangston/dev/wrkmn
git add README.md CLAUDE.md
git commit -m "docs: document oneshot, restart policy, validate, pre-flight, and state persistence"
```

---

## Self-review notes

- **Spec coverage:** Component 1 severity rules → Task 4 (provenance via `defaults.EnvFile` never being copied into processes — verified in `applyDefaults`); Component 2 → Task 2; Component 3 → Task 5 (+ Task 6 makes it reachable from the TUI); Component 4 → Task 8; Component 5 → Tasks 7 + 9 (terminal gate deviation documented in header). Additions: restart (Task 3), scheduler wiring (Task 6), state persistence (Task 10), housekeeping (Task 1), docs (Task 11).
- **Type consistency:** `ProcessRunState`/`RunStateRunning|Exited|Killed` (Task 2) consumed by Tasks 3 and 5 with identical names. `EnvFileRef`/`ResolveEnvFilePaths` (Task 4) consumed by Task 7. `StateCompleted`/`EventCompleted` (Task 5) consumed by Task 6. `Severity`/`Issue`/`Check`/`HasErrors` (Task 7) consumed by Tasks 8-9. `WorkerState`/`Load`/`Save`/`Apply`/`FileName` (Task 10) consistent across state package, cmd, and ui.
- **Ordering:** Task 3 avoids referencing `ProcessTypeOneshot` (allowlists worker/service instead), so Tasks 3 and 5 are order-independent beyond both needing Task 2. Task 6 needs Tasks 4 and 5. Tasks 8-9 need Task 7.
- **Risks:** `StateCompleted` insertion renumbers `StateStopped`/`StateFailed` (safe — never persisted; existing tests compared via constants). `startAllWorkers` rewrite touches the same lines as Task 4's interim edit; Task 6 explicitly supersedes it. The event-consumer goroutine terminates via `CloseEvents()` after `StartAll` returns.
