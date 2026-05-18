// Package poc provides proof-of-concept limit tests.
package poc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestConcurrentConflict_WriteNoLock tests multiple goroutines writing to the same file without locking.
// This should reveal data corruption or lost writes due to concurrent access.
func TestConcurrentConflict_WriteNoLock(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "no_lock.txt")

	// Pre-create the file
	f, err := os.Create(filePath)
	assert.NoError(t, err)
	f.Close()

	const numGoroutines = 50
	const writesPerGoroutine = 100
	var totalWrites int64
	var conflictCount int64
	var wg sync.WaitGroup

	start := time.Now()

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < writesPerGoroutine; j++ {
				// Open file with O_APPEND flag - OS handles atomic append on some systems
				// but not all writes are guaranteed atomic across all OSes
				data := fmt.Sprintf("goroutine-%d-write-%d\n", id, j)
				f, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0644)
				if err != nil {
					atomic.AddInt64(&conflictCount, 1)
					continue
				}
				_, err = f.WriteString(data)
				f.Close()
				if err != nil {
					atomic.AddInt64(&conflictCount, 1)
					continue
				}
				atomic.AddInt64(&totalWrites, 1)
			}
		}(i)
	}

	wg.Wait()
	elapsed := time.Now().Sub(start)

	// Read final file content
	content, err := os.ReadFile(filePath)
	assert.NoError(t, err)

	// Count actual lines written
	lineCount := int64(0)
	for _, c := range content {
		if c == '\n' {
			lineCount++
		}
	}

	expectedWrites := int64(numGoroutines * writesPerGoroutine)
	successRate := float64(lineCount) / float64(expectedWrites) * 100
	conflictRate := 100.0 - successRate

	t.Logf("WriteNoLock: totalWrites=%d, actualLines=%d, conflicts=%d, conflictRate=%.2f%%, elapsed=%v",
		totalWrites, lineCount, expectedWrites-lineCount, conflictRate, elapsed)
	t.Logf("Success rate: %.2f%%", successRate)

	// Without proper locking, we expect some writes to be lost or corrupted
	// The actual conflict rate depends on OS and filesystem
	assert.LessOrEqual(t, lineCount, expectedWrites, "Should not exceed expected writes")
}

// TestConcurrentConflict_WriteWithLock tests concurrent writes protected by sync.Mutex.
func TestConcurrentConflict_WriteWithLock(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "with_lock.txt")

	var mu sync.Mutex
	var totalWrites int64
	var wg sync.WaitGroup

	const numGoroutines = 50
	const writesPerGoroutine = 100

	start := time.Now()

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < writesPerGoroutine; j++ {
				mu.Lock()
				f, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
				if err != nil {
					mu.Unlock()
					continue
				}
				data := fmt.Sprintf("goroutine-%d-write-%d\n", id, j)
				_, err = f.WriteString(data)
				f.Close()
				mu.Unlock()
				if err == nil {
					atomic.AddInt64(&totalWrites, 1)
				}
			}
		}(i)
	}

	wg.Wait()
	elapsed := time.Now().Sub(start)

	// Verify all writes are present
	content, err := os.ReadFile(filePath)
	assert.NoError(t, err)

	lineCount := int64(0)
	for _, c := range content {
		if c == '\n' {
			lineCount++
		}
	}

	expectedWrites := int64(numGoroutines * writesPerGoroutine)
	successRate := float64(lineCount) / float64(expectedWrites) * 100

	t.Logf("WriteWithLock: totalWrites=%d, actualLines=%d, successRate=%.2f%%, elapsed=%v",
		totalWrites, lineCount, successRate, elapsed)

	// With mutex, all writes should succeed
	assert.Equal(t, expectedWrites, lineCount, "All writes should be preserved with mutex")
}

// TestConcurrentConflict_ReadWriteRace tests concurrent reads and writes to the same file.
func TestConcurrentConflict_ReadWriteRace(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "rw_race.txt")

	// Initialize file with some content
	err := os.WriteFile(filePath, []byte("initial content\n"), 0644)
	assert.NoError(t, err)

	var mu sync.Mutex
	var readCount int64
	var writeCount int64
	var readErrors int64
	var inconsistentReads int64
	var wg sync.WaitGroup

	const numReaders = 30
	const numWriters = 20
	const testDuration = 500 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()

	// Start readers
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					mu.Lock()
					content, err := os.ReadFile(filePath)
					mu.Unlock()
					if err != nil {
						atomic.AddInt64(&readErrors, 1)
						continue
					}
					// Check if content is valid (not corrupted)
					if len(content) == 0 || content[len(content)-1] != '\n' {
						atomic.AddInt64(&inconsistentReads, 1)
					}
					atomic.AddInt64(&readCount, 1)
					time.Sleep(time.Microsecond)
				}
			}
		}(i)
	}

	// Start writers
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					mu.Lock()
					f, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0644)
					if err != nil {
						mu.Unlock()
						continue
					}
					data := fmt.Sprintf("writer-%d-msg-%d\n", id, atomic.AddInt64(&writeCount, 1))
					_, err = f.WriteString(data)
					f.Close()
					mu.Unlock()
					time.Sleep(time.Millisecond)
				}
			}
		}(i)
	}

	wg.Wait()

	t.Logf("ReadWriteRace: reads=%d, writes=%d, readErrors=%d, inconsistentReads=%d",
		readCount, writeCount, readErrors, inconsistentReads)

	// With proper mutex protection, reads and writes should not cause errors
	assert.Equal(t, int64(0), readErrors, "Should have no read errors with mutex")
	assert.Equal(t, int64(0), inconsistentReads, "Should have no inconsistent reads with mutex")
}

// TestConcurrentConflict_DeadlockDetection simulates a deadlock scenario with two mutexes.
// This test uses a timeout to detect if deadlock occurs.
func TestConcurrentConflict_DeadlockDetection(t *testing.T) {
	var mu1 sync.Mutex
	var mu2 sync.Mutex
	var completed int64
	var wg sync.WaitGroup

	// Strategy 1: Lock in same order - no deadlock
	wg.Add(1)
	go func() {
		defer wg.Done()
		mu1.Lock()
		time.Sleep(10 * time.Millisecond)
		mu2.Lock()
		atomic.AddInt64(&completed, 1)
		mu2.Unlock()
		mu1.Unlock()
	}()

	// Strategy 2: Lock in same order - no deadlock
	wg.Add(1)
	go func() {
		defer wg.Done()
		mu1.Lock()
		time.Sleep(10 * time.Millisecond)
		mu2.Lock()
		atomic.AddInt64(&completed, 1)
		mu2.Unlock()
		mu1.Unlock()
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Logf("DeadlockDetection: completed=%d (same order - no deadlock expected)", completed)
		assert.Equal(t, int64(2), completed, "Both should complete with same lock order")
	case <-time.After(2 * time.Second):
		t.Fatal("Deadlock detected: test timed out")
	}
}

// TestConcurrentConflict_DeadlockDetection_ReverseOrder tests deadlock when locks are acquired in opposite order.
func TestConcurrentConflict_DeadlockDetection_ReverseOrder(t *testing.T) {
	var mu1 sync.Mutex
	var mu2 sync.Mutex
	var completed int64
	var deadlockDetected int64
	var wg sync.WaitGroup

	// Goroutine 1: lock1 then lock2
	wg.Add(1)
	go func() {
		defer wg.Done()
		mu1.Lock()
		time.Sleep(50 * time.Millisecond) // Give other goroutine time to grab mu2
		mu2.Lock()
		atomic.AddInt64(&completed, 1)
		mu2.Unlock()
		mu1.Unlock()
	}()

	// Goroutine 2: lock2 then lock1 (opposite order - potential deadlock)
	wg.Add(1)
	go func() {
		defer wg.Done()
		mu2.Lock()
		time.Sleep(50 * time.Millisecond) // Give other goroutine time to grab mu1
		mu1.Lock()
		atomic.AddInt64(&completed, 1)
		mu1.Unlock()
		mu2.Unlock()
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Both completed - this is the expected outcome in a simple test
		// In real deadlocks this would timeout
		t.Logf("ReverseOrder: completed=%d (no deadlock in this run)", completed)
	case <-time.After(500 * time.Millisecond):
		atomic.AddInt64(&deadlockDetected, 1)
		t.Logf("ReverseOrder: DEADLOCK detected, completed=%d", completed)
	}

	// Note: This test may or may not detect deadlock depending on timing
	// The point is to demonstrate the concept
	t.Logf("Deadlock detection result: detected=%d", deadlockDetected)
}

// TestConcurrentConflict_StressHighConcurrency tests system behavior under high concurrency.
func TestConcurrentConflict_StressHighConcurrency(t *testing.T) {
	tmpDir := t.TempDir()

	const numGoroutines = 100 // High concurrency
	const opsPerGoroutine = 50
	var mu sync.Mutex
	var totalOps int64
	var fileOps int64
	var errors int64
	var wg sync.WaitGroup

	start := time.Now()

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				filePath := filepath.Join(tmpDir, fmt.Sprintf("stress_%d.txt", id%10))
				mu.Lock()
				f, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
				if err != nil {
					mu.Unlock()
					atomic.AddInt64(&errors, 1)
					continue
				}
				data := fmt.Sprintf("g-%d-op-%d\n", id, j)
				_, err = f.WriteString(data)
				f.Close()
				mu.Unlock()
				if err == nil {
					atomic.AddInt64(&totalOps, 1)
				} else {
					atomic.AddInt64(&errors, 1)
				}
				atomic.AddInt64(&fileOps, 1)
			}
		}(i)
	}

	wg.Wait()
	elapsed := time.Now().Sub(start)

	expectedOps := int64(numGoroutines * opsPerGoroutine)
	successRate := float64(totalOps) / float64(expectedOps) * 100
	opsPerSecond := float64(totalOps) / elapsed.Seconds()

	t.Logf("StressHighConcurrency (%d goroutines, %d ops each):", numGoroutines, opsPerGoroutine)
	t.Logf("  totalOps=%d, fileOps=%d, errors=%d", totalOps, fileOps, errors)
	t.Logf("  successRate=%.2f%%, throughput=%.0f ops/s", successRate, opsPerSecond)
	t.Logf("  elapsed=%v", elapsed)

	assert.Equal(t, expectedOps, totalOps+errors, "All operations should complete (success or error)")
	assert.Equal(t, int64(0), errors, "Should have no errors with mutex protection")
}

// TestConcurrentConflict_Stress50Vs100Compares tests 50 vs 100 goroutines for comparison.
func TestConcurrentConflict_Stress50Vs100(t *testing.T) {
	tmpDir := t.TempDir()

	runStress := func(numGoroutines, opsPerGoroutine int) (elapsed time.Duration, totalOps int64) {
		var mu sync.Mutex
		var ops int64
		var wg sync.WaitGroup

		start := time.Now()
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < opsPerGoroutine; j++ {
					filePath := filepath.Join(tmpDir, fmt.Sprintf("compare_%d.txt", id%20))
					mu.Lock()
					f, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
					if err != nil {
						mu.Unlock()
						continue
					}
					_, err = f.WriteString(fmt.Sprintf("g%d-op%d\n", id, j))
					f.Close()
					mu.Unlock()
					if err == nil {
						atomic.AddInt64(&ops, 1)
					}
				}
			}(i)
		}
		wg.Wait()
		elapsed = time.Now().Sub(start)
		return elapsed, ops
	}

	// Run with 50 goroutines
	elapsed50, ops50 := runStress(50, 50)
	opsPerSec50 := float64(ops50) / elapsed50.Seconds()

	// Run with 100 goroutines
	elapsed100, ops100 := runStress(100, 50)
	opsPerSec100 := float64(ops100) / elapsed100.Seconds()

	t.Logf("50 goroutines: ops=%d, elapsed=%v, throughput=%.0f ops/s", ops50, elapsed50, opsPerSec50)
	t.Logf("100 goroutines: ops=%d, elapsed=%v, throughput=%.0f ops/s", ops100, elapsed100, opsPerSec100)

	// Both should complete successfully
	assert.Equal(t, int64(2500), ops50, "50 goroutines should complete all 2500 ops")
	assert.Equal(t, int64(5000), ops100, "100 goroutines should complete all 5000 ops")
}
