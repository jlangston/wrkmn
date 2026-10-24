package process

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"wrkmn/internal/config"
	"wrkmn/internal/worker"
)

func TestCircularBuffer_ThreadSafe(t *testing.T) {
	// Test that CircularBuffer is thread-safe for concurrent writes and reads
	cb := NewCircularBuffer(100)

	var wg sync.WaitGroup
	numWriters := 10
	numReaders := 5
	writesPerWriter := 100

	// Concurrent writers
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < writesPerWriter; j++ {
				cb.Write("test line from writer")
			}
		}(i)
	}

	// Concurrent readers
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				cb.GetLines()
				time.Sleep(time.Microsecond)
			}
		}()
	}

	// Should not panic or race
	assert.NotPanics(t, func() {
		wg.Wait()
	})
}

func TestCircularBuffer_Capacity(t *testing.T) {
	cb := NewCircularBuffer(5)

	// Write 3 lines (under capacity)
	cb.Write("line1")
	cb.Write("line2")
	cb.Write("line3")

	lines := cb.GetLines()
	assert.Len(t, lines, 3)
	assert.Equal(t, "line1", lines[0])

	// Write 5 more lines (over capacity)
	cb.Write("line4")
	cb.Write("line5")
	cb.Write("line6")
	cb.Write("line7")
	cb.Write("line8")

	lines = cb.GetLines()
	assert.Len(t, lines, 5)
	// Should have the latest 5 lines in order
	assert.Equal(t, "line4", lines[0])
	assert.Equal(t, "line8", lines[4])
}

func TestManager_ProcessMapThreadSafe(t *testing.T) {
	// Test that Manager's process map access is thread-safe
	m := NewManager()

	var wg sync.WaitGroup

	// Concurrent reads
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				m.GetRunningProcesses()
				m.GetAllProcesses()
				m.GetProcess("test")
			}
		}()
	}

	// Should not panic
	assert.NotPanics(t, func() {
		wg.Wait()
	})
}

func TestProcess_MethodsThreadSafe(t *testing.T) {
	// Test that Process methods are thread-safe
	p := &Process{
		Name:    "test",
		Started: time.Now(),
	}

	var wg sync.WaitGroup

	// Concurrent reads
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				p.Pid()
				p.StartTime()
				p.IsRunning()
			}
		}()
	}

	// Should not panic
	assert.NotPanics(t, func() {
		wg.Wait()
	})
}

// Test that StopAll properly collects processes before map modification
// This is a regression test for the bug where force-kill never executed
// because the map was cleared before the second loop
func TestManager_StopAll_ProcessCollection(t *testing.T) {
	m := NewManager()

	// The StopAll function requires Cmd.Process to be non-nil
	// So we test with an empty map to verify the map clearing logic
	assert.NotPanics(t, func() {
		m.StopAll()
	})

	// Map should be empty
	assert.Equal(t, 0, len(m.GetRunningProcesses()))
}

// Test that StopAll handles nil Cmd gracefully
func TestManager_StopAll_NilCmd(t *testing.T) {
	m := NewManager()

	// Create processes with nil Cmd (simulating processes that failed to start)
	m.mu.Lock()
	for i := 0; i < 3; i++ {
		name := "process-" + string(rune('a'+i))
		m.processes[name] = &Process{
			Name:      name,
			LogBuffer: NewCircularBuffer(10),
			Cancel:    func() {}, // No-op cancel
			// Cmd is nil
		}
	}
	m.mu.Unlock()

	// StopAll should handle nil Cmd gracefully (after fix)
	assert.NotPanics(t, func() {
		m.StopAll()
	})

	// Map should be empty after StopAll
	assert.Equal(t, 0, len(m.GetRunningProcesses()))
}

func TestCircularBuffer_WrapAround(t *testing.T) {
	cb := NewCircularBuffer(3)

	// Fill buffer
	cb.Write("a")
	cb.Write("b")
	cb.Write("c")

	// Buffer is now full, next write wraps
	cb.Write("d")

	lines := cb.GetLines()
	assert.Len(t, lines, 3)
	assert.Equal(t, []string{"b", "c", "d"}, lines)

	// More wraps
	cb.Write("e")
	cb.Write("f")

	lines = cb.GetLines()
	assert.Equal(t, []string{"d", "e", "f"}, lines)
}

func TestCircularBuffer_Empty(t *testing.T) {
	cb := NewCircularBuffer(10)

	lines := cb.GetLines()
	assert.Empty(t, lines)
}

func TestNewManager(t *testing.T) {
	m := NewManager()
	assert.NotNil(t, m)
	assert.NotNil(t, m.processes)
	assert.Empty(t, m.processes)
}

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
	defer m.StopAll()
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
	defer m.StopAll()
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
	defer m.StopAll()
	if err := m.StopProcess("sleeper"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	waitForExit(t, p, 5*time.Second)
	if p.RunState() != RunStateKilled {
		t.Fatalf("RunState = %v, want RunStateKilled", p.RunState())
	}
}

func TestCrashedProcessMarksExited(t *testing.T) {
	m := NewManager()
	defer m.StopAll()
	cfg := &config.Process{Command: "sh", Args: []string{"-c", "exit 2"}, Type: config.ProcessTypeService}
	p, err := m.StartProcess(context.Background(), "crashy", cfg, nil)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForExit(t, p, 5*time.Second)
	// Verify that superviseProcess recorded the exit code (indicates non-zero exit).
	if p.RunState() != RunStateExited {
		t.Fatalf("RunState = %v, want RunStateExited", p.RunState())
	}
	if p.ExitCode() != 2 {
		t.Fatalf("ExitCode = %d, want 2", p.ExitCode())
	}
}

func TestDeadEntryReplacedOnRestart(t *testing.T) {
	m := NewManager()
	cfg := &config.Process{Command: "sh", Args: []string{"-c", "exit 1"}, Type: config.ProcessTypeService}
	p, err := m.StartProcess(context.Background(), "flappy", cfg, nil)
	if err != nil {
		t.Fatalf("first start: %v", err)
	}
	defer m.StopAll()
	waitForExit(t, p, 5*time.Second)
	// A dead entry must not block a fresh start.
	p2, err := m.StartProcess(context.Background(), "flappy", cfg, nil)
	if err != nil {
		t.Fatalf("second start: %v", err)
	}
	waitForExit(t, p2, 5*time.Second)
}

func TestStopAllMarksKilled(t *testing.T) {
	m := NewManager()
	cfg := &config.Process{Command: "sleep", Args: []string{"30"}, Type: config.ProcessTypeService}
	p, err := m.StartProcess(context.Background(), "sleeper", cfg, nil)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	// Stop all processes (not using StopProcess, testing StopAll path)
	m.StopAll()
	waitForExit(t, p, 5*time.Second)
	if p.RunState() != RunStateKilled {
		t.Fatalf("RunState = %v, want RunStateKilled", p.RunState())
	}
}

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

func TestManualStopDuringRestartDelayPreventsRestart(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "runs")
	cfg := &config.Process{
		Command: "sh",
		Args:    []string{"-c", "echo run >> " + marker + "; exit 1"},
		Type:    config.ProcessTypeService,
		Restart: &config.RestartPolicy{Enabled: true, MaxTries: 3, Delay: config.Duration{Duration: 500 * time.Millisecond}},
	}
	m := NewManager()
	p, err := m.StartProcess(context.Background(), "flappy", cfg, nil)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer m.StopAll()

	// Process dies naturally; the dead entry stays in m.processes and
	// maybeRestart begins its Delay sleep.
	waitForExit(t, p, 5*time.Second)

	// Stop it promptly, well within the 500ms restart delay.
	if err := m.StopProcess("flappy"); err != nil {
		t.Fatalf("stop: %v", err)
	}

	// Sleep past the restart delay and confirm no restart fired.
	time.Sleep(800 * time.Millisecond)
	if _, exists := m.GetProcess("flappy"); exists {
		t.Fatal("process was restarted after a manual stop during the restart delay")
	}
	if got := countLines(t, marker); got != 1 {
		t.Fatalf("runs = %d, want exactly 1 (no restart after manual stop)", got)
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

func TestBannerParsingPopulatesWorkerURLs(t *testing.T) {
	m := NewManager()
	w := &worker.Worker{Name: "bannered", Status: worker.StatusStarting}
	cfg := &config.Process{
		Command: "sh",
		Args: []string{"-c",
			`printf '  \xe2\x9e\x9c  Local:   http://localhost:8123/\n  \xe2\x9e\x9c  Debug:   http://localhost:8123/__debug\n'; sleep 30`},
		Type: config.ProcessTypeWorker,
	}
	p, err := m.StartProcess(context.Background(), "bannered", cfg, w)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer m.StopAll()

	// Poll the process's LogBuffer rather than w.Port/w.DebugURL directly.
	// applyBannerLine writes those Worker fields unsynchronized (the same
	// accepted pattern as Status — see its doc comment), so busy-polling them
	// from this goroutine races the log-reader goroutine under -race.
	// LogBuffer.Write/GetLines are mutex-guarded, and the writer calls
	// applyBannerLine(w, raw) before buffer.Write(line) on each line, so
	// observing the Debug line here via the buffer's lock establishes a real
	// happens-before edge for w's fields too — safe to read once this
	// resolves.
	deadline := time.Now().Add(5 * time.Second)
	var sawDebugLine bool
	for time.Now().Before(deadline) {
		if strings.Contains(strings.Join(p.LogBuffer.GetLines(), "\n"), "Debug:") {
			sawDebugLine = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !sawDebugLine {
		t.Fatalf("banner lines never logged: lines=%v", p.LogBuffer.GetLines())
	}
	if w.DebugURL == "" || w.Port != 8123 {
		t.Fatalf("banner not parsed: Port=%d DevServerURL=%q DebugURL=%q", w.Port, w.DevServerURL, w.DebugURL)
	}
}

// TestStaleWorkerFieldsClearedOnRestart is a regression test: a previous
// run's DevServerURL/DebugURL/Port must not survive into a new run. Ports
// get reassigned across restarts (vite auto-increments), so a dangling
// DebugURL from the old run could point debugger attach at a different
// worker's inspector entirely.
func TestStaleWorkerFieldsClearedOnRestart(t *testing.T) {
	m := NewManager()
	w := &worker.Worker{Name: "restarty", Status: worker.StatusStarting}
	bannerCfg := &config.Process{
		Command: "sh",
		Args: []string{"-c",
			`printf '  \xe2\x9e\x9c  Local:   http://localhost:8124/\n  \xe2\x9e\x9c  Debug:   http://localhost:8124/__debug\n'; sleep 30`},
		Type: config.ProcessTypeWorker,
	}
	p, err := m.StartProcess(context.Background(), "restarty", bannerCfg, w)
	if err != nil {
		t.Fatalf("first start: %v", err)
	}

	// Poll the mutex-guarded LogBuffer rather than busy-reading w's
	// unsynchronized fields directly — see TestBannerParsingPopulatesWorkerURLs
	// for why that establishes a real happens-before edge for w's fields too.
	deadline := time.Now().Add(5 * time.Second)
	var sawDebugLine bool
	for time.Now().Before(deadline) {
		if strings.Contains(strings.Join(p.LogBuffer.GetLines(), "\n"), "Debug:") {
			sawDebugLine = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !sawDebugLine {
		t.Fatalf("banner lines never logged: lines=%v", p.LogBuffer.GetLines())
	}
	if w.DebugURL == "" || w.DevServerURL == "" || w.Port != 8124 {
		t.Fatalf("banner not parsed before restart: Port=%d DevServerURL=%q DebugURL=%q", w.Port, w.DevServerURL, w.DebugURL)
	}

	if err := m.StopProcess("restarty"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	waitForExit(t, p, 5*time.Second)

	// Start a new process for the same worker that prints no banner at all.
	// The fields are cleared synchronously inside StartProcess, before
	// cmd.Start() launches the new command, so a read right here — in the
	// same goroutine, immediately after StartProcess returns — is
	// deterministic: nothing will repopulate them since this run never
	// prints a banner line.
	quietCfg := &config.Process{Command: "sleep", Args: []string{"30"}, Type: config.ProcessTypeWorker}
	if _, err := m.StartProcess(context.Background(), "restarty", quietCfg, w); err != nil {
		t.Fatalf("second start: %v", err)
	}
	defer m.StopAll()

	if w.DevServerURL != "" || w.DebugURL != "" || w.Port != 0 {
		t.Fatalf("stale fields survived restart: Port=%d DevServerURL=%q DebugURL=%q", w.Port, w.DevServerURL, w.DebugURL)
	}
}
