package debug

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPortAllocator_Allocate(t *testing.T) {
	t.Run("allocates sequential ports", func(t *testing.T) {
		allocator := NewPortAllocator(9229)

		port1 := allocator.Allocate("worker1")
		port2 := allocator.Allocate("worker2")
		port3 := allocator.Allocate("worker3")

		assert.Equal(t, 9229, port1)
		assert.Equal(t, 9230, port2)
		assert.Equal(t, 9231, port3)
	})

	t.Run("returns same port for same name", func(t *testing.T) {
		allocator := NewPortAllocator(9229)

		port1 := allocator.Allocate("worker1")
		port2 := allocator.Allocate("worker1")

		assert.Equal(t, port1, port2)
	})
}

func TestPortAllocator_GetPort(t *testing.T) {
	allocator := NewPortAllocator(9229)

	// Not allocated yet
	_, exists := allocator.GetPort("worker1")
	assert.False(t, exists)

	// After allocation
	allocator.Allocate("worker1")
	port, exists := allocator.GetPort("worker1")
	assert.True(t, exists)
	assert.Equal(t, 9229, port)
}

func TestPortAllocator_Release(t *testing.T) {
	allocator := NewPortAllocator(9229)

	allocator.Allocate("worker1")
	_, exists := allocator.GetPort("worker1")
	assert.True(t, exists)

	allocator.Release("worker1")
	_, exists = allocator.GetPort("worker1")
	assert.False(t, exists)
}

func TestPortAllocator_GetAll(t *testing.T) {
	allocator := NewPortAllocator(9229)

	allocator.Allocate("worker1")
	allocator.Allocate("worker2")

	all := allocator.GetAll()
	assert.Equal(t, 2, len(all))
	assert.Equal(t, 9229, all["worker1"])
	assert.Equal(t, 9230, all["worker2"])
}

// Bug fix tests

func TestPortAllocator_ThreadSafe(t *testing.T) {
	// Test that PortAllocator is thread-safe
	// This was a bug fix - added mutex protection
	allocator := NewPortAllocator(9229)

	var wg sync.WaitGroup
	numWorkers := 100

	// Concurrent allocations
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			name := "worker" + string(rune('A'+id%26)) + string(rune('0'+id/26))
			allocator.Allocate(name)
		}(i)
	}

	// Should not panic or race
	assert.NotPanics(t, func() {
		wg.Wait()
	})

	// Verify all ports are allocated
	all := allocator.GetAll()
	assert.Equal(t, numWorkers, len(all))
}

func TestPortAllocator_ConcurrentReadWrite(t *testing.T) {
	// Test concurrent reads and writes
	allocator := NewPortAllocator(9229)

	// Pre-allocate some workers
	for i := 0; i < 10; i++ {
		allocator.Allocate("worker" + string(rune('0'+i)))
	}

	var wg sync.WaitGroup

	// Concurrent reads
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				allocator.GetAll()
				allocator.GetPort("worker0")
			}
		}()
	}

	// Concurrent writes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				name := "new" + string(rune('A'+id)) + string(rune('0'+j))
				allocator.Allocate(name)
			}
		}(i)
	}

	// Should not panic or race
	assert.NotPanics(t, func() {
		wg.Wait()
	})
}
