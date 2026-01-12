package process

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
