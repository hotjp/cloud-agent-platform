// Package poc provides proof-of-concept limit tests.
package poc

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestConcurrency_SemaphoreLimit tests semaphore-based concurrency control.
func TestConcurrency_SemaphoreLimit(t *testing.T) {
	const maxConcurrent = 5
	sem := make(chan struct{}, maxConcurrent)
	var active int64
	var maxActive int64
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			current := atomic.AddInt64(&active, 1)
			for {
				old := atomic.LoadInt64(&maxActive)
				if current <= old || atomic.CompareAndSwapInt64(&maxActive, old, current) {
					break
				}
			}
			time.Sleep(50 * time.Millisecond)
			atomic.AddInt64(&active, -1)
			<-sem
		}()
	}

	wg.Wait()
	t.Logf("Max concurrent with semaphore(%d): %d", maxConcurrent, maxActive)
	assert.LessOrEqual(t, maxActive, int64(maxConcurrent), "Should never exceed semaphore limit")
}

// TestConcurrency_RateLimiter tests rate limiting.
func TestConcurrency_RateLimiter(t *testing.T) {
	const rate = 100 // ops/sec
	interval := time.Second / time.Duration(rate)
	var count int64

	start := time.Now()
	for i := 0; i < 50; i++ {
		atomic.AddInt64(&count, 1)
		time.Sleep(interval)
	}
	elapsed := time.Since(start)

	t.Logf("Rate limiter: %d ops in %v (%.0f ops/s)", count, elapsed, float64(count)/elapsed.Seconds())
	assert.LessOrEqual(t, count, int64(50))
}

// TestConcurrency_50AgentsSimultaneous tests 50 concurrent agents.
func TestConcurrency_50AgentsSimultaneous(t *testing.T) {
	var wg sync.WaitGroup
	var successCount int64
	var timeoutCount int64

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				atomic.AddInt64(&timeoutCount, 1)
				return
			default:
				// Simulate agent work
				time.Sleep(time.Duration(10+idx%20) * time.Millisecond)
				atomic.AddInt64(&successCount, 1)
			}
		}(i)
	}

	wg.Wait()
	t.Logf("50 agents: success=%d timeout=%d", successCount, timeoutCount)
	assert.Equal(t, int64(50), successCount, "All 50 agents should succeed")
}

// TestConcurrency_WorkerPool tests worker pool pattern.
func TestConcurrency_WorkerPool(t *testing.T) {
	const numWorkers = 10
	const numTasks = 100

	tasks := make(chan int, numTasks)
	var processed int64

	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range tasks {
				time.Sleep(time.Millisecond) // simulate work
				atomic.AddInt64(&processed, 1)
			}
		}()
	}

	start := time.Now()
	for i := 0; i < numTasks; i++ {
		tasks <- i
	}
	close(tasks)
	wg.Wait()
	elapsed := time.Since(start)

	t.Logf("Worker pool (%d workers, %d tasks): %v, %.0f tasks/s",
		numWorkers, numTasks, elapsed, float64(numTasks)/elapsed.Seconds())
	assert.Equal(t, int64(numTasks), processed)
}

// TestConcurrency_MutexContention tests mutex contention under high load.
func TestConcurrency_MutexContention(t *testing.T) {
	var mu sync.Mutex
	var counter int64
	var wg sync.WaitGroup

	start := time.Now()
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				mu.Lock()
				counter++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	t.Logf("Mutex contention (100k ops): %v, %.0f ops/s", elapsed, float64(100000)/elapsed.Seconds())
	assert.Equal(t, int64(100000), counter)
}

// TestConcurrency_ConcurrentMapAccess tests concurrent map access with sync.Map.
func TestConcurrency_ConcurrentMapAccess(t *testing.T) {
	var m sync.Map
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				key := fmt.Sprintf("key_%d", j%20)
				m.Store(key, idx*100+j)
				_, _ = m.Load(key)
			}
		}(i)
	}

	wg.Wait()

	count := 0
	m.Range(func(_, _ any) bool {
		count++
		return true
	})
	t.Logf("Concurrent map: %d unique keys after 5000 ops", count)
	assert.Equal(t, 20, count, "Should have 20 unique keys")
}
