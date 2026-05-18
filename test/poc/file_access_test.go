// Package poc provides proof-of-concept limit tests.
package poc

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// latencyResult records latency statistics.
type latencyResult struct {
	Name    string
	Count   int
	P50     time.Duration
	P95     time.Duration
	P99     time.Duration
	Average time.Duration
	Total   time.Duration
	Min     time.Duration
	Max     time.Duration
}

func measureLatencies(name string, runs int, fn func() error) latencyResult {
	durations := make([]time.Duration, runs)
	start := time.Now()
	for i := 0; i < runs; i++ {
		t0 := time.Now()
		_ = fn()
		durations[i] = time.Since(t0)
	}
	total := time.Since(start)

	// Sort for percentiles
	for i := 0; i < len(durations); i++ {
		for j := i + 1; j < len(durations); j++ {
			if durations[j] < durations[i] {
				durations[i], durations[j] = durations[j], durations[i]
			}
		}
	}

	var sum time.Duration
	min := durations[0]
	max := durations[0]
	for _, d := range durations {
		sum += d
		if d < min {
			min = d
		}
		if d > max {
			max = d
		}
	}

	return latencyResult{
		Name:    name,
		Count:   runs,
		P50:     durations[runs*50/100],
		P95:     durations[runs*95/100],
		P99:     durations[runs*99/100],
		Average: sum / time.Duration(runs),
		Total:   total,
		Min:     min,
		Max:     max,
	}
}

func (r latencyResult) Log(t *testing.T) {
	t.Logf("%-20s count=%5d avg=%10v min=%10v p50=%10v p95=%10v p99=%10v max=%10v total=%10v",
		r.Name, r.Count, r.Average, r.Min, r.P50, r.P95, r.P99, r.Max, r.Total)
}

func (r latencyResult) String() string {
	return fmt.Sprintf("%-20s count=%5d avg=%10v min=%10v p50=%10v p95=%10v p99=%10v max=%10v total=%10v",
		r.Name, r.Count, r.Average, r.Min, r.P50, r.P95, r.P99, r.Max, r.Total)
}

func makeTempDir(t *testing.T) string {
	dir, err := os.MkdirTemp("", "poc-file-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func writeFile(dir, name string, size int) error {
	data := make([]byte, size)
	for i := range data {
		data[i] = byte('a' + i%26)
	}
	return os.WriteFile(filepath.Join(dir, name), data, 0644)
}

// TestFileAccess_ReadLatency measures read latency for various file sizes.
func TestFileAccess_ReadLatency(t *testing.T) {
	dir := makeTempDir(t)

	sizes := []struct {
		name string
		size int
	}{
		{"1KB", 1024},
		{"1MB", 1024 * 1024},
		{"10MB", 10 * 1024 * 1024},
		{"100MB", 100 * 1024 * 1024},
	}

	t.Log("\n" + formatTableHeader())
	for _, tc := range sizes {
		fname := fmt.Sprintf("read_%s.bin", tc.name)
		require.NoError(t, writeFile(dir, fname, tc.size), "failed to create %s file", tc.name)
		fpath := filepath.Join(dir, fname)

		r := measureLatencies(tc.name+"-read", 100, func() error {
			_, err := os.ReadFile(fpath)
			return err
		})
		r.Log(t)
	}
}

// TestFileAccess_WriteLatency measures write latency for various file sizes.
func TestFileAccess_WriteLatency(t *testing.T) {
	dir := makeTempDir(t)

	sizes := []struct {
		name string
		size int
	}{
		{"1KB", 1024},
		{"1MB", 1024 * 1024},
		{"10MB", 10 * 1024 * 1024},
		{"100MB", 100 * 1024 * 1024},
	}

	t.Log("\n" + formatTableHeader())
	for _, tc := range sizes {
		filename := filepath.Join(dir, fmt.Sprintf("write_%s.bin", tc.name))

		// Pre-allocate data to avoid measuring allocation time
		data := make([]byte, tc.size)
		for i := range data {
			data[i] = byte('a' + i%26)
		}

		r := measureLatencies(tc.name+"-write", 100, func() error {
			return os.WriteFile(filename, data, 0644)
		})
		r.Log(t)
	}
}

// TestFileAccess_ConcurrentRead tests N goroutines reading files concurrently.
func TestFileAccess_ConcurrentRead(t *testing.T) {
	dir := makeTempDir(t)

	// Create test files
	fileSizes := []int{1024, 1024 * 1024}
	files := make([]string, len(fileSizes))
	for i, size := range fileSizes {
		fname := fmt.Sprintf("concurrent_read_%d.bin", i)
		require.NoError(t, writeFile(dir, fname, size))
		files[i] = filepath.Join(dir, fname)
	}

	concurrencyLevels := []int{1, 2, 4, 8, 16, 32}

	for _, numGoroutines := range concurrencyLevels {
		t.Run(fmt.Sprintf("%d_goroutines", numGoroutines), func(t *testing.T) {
			var wg sync.WaitGroup
			readCount := int64(0)
			readsPerGoroutine := 100

			start := time.Now()
			for i := 0; i < numGoroutines; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					file := files[idx%len(files)]
					for j := 0; j < readsPerGoroutine; j++ {
						_, _ = os.ReadFile(file)
						atomic.AddInt64(&readCount, 1)
					}
				}(i)
			}
			wg.Wait()
			elapsed := time.Since(start)

			totalReads := int64(numGoroutines * readsPerGoroutine)
			avgLatency := elapsed / time.Duration(totalReads)
			t.Logf("%2d goroutines: %6d reads in %v (avg %v/read)",
				numGoroutines, totalReads, elapsed, avgLatency)
		})
	}
}

// TestFileAccess_ConcurrentWrite tests concurrent writes with and without locking.
func TestFileAccess_ConcurrentWrite(t *testing.T) {
	dir := makeTempDir(t)
	bytesToWrite := 1024 // 1KB per write

	t.Run("without_lock", func(t *testing.T) {
		path := filepath.Join(dir, "concurrent_no_lock.bin")
		data := make([]byte, bytesToWrite)
		for i := range data {
			data[i] = byte('a' + i%26)
		}

		numGoroutines := 10
		writesPerGoroutine := 100

		start := time.Now()
		var wg sync.WaitGroup
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				for j := 0; j < writesPerGoroutine; j++ {
					filename := fmt.Sprintf("%s_%d_%d", path, idx, j)
					_ = os.WriteFile(filename, data, 0644)
				}
			}(i)
		}
		wg.Wait()
		elapsed := time.Since(start)

		totalWrites := numGoroutines * writesPerGoroutine
		_ = totalWrites // may not be used if optimized away
		t.Logf("Without lock: %d writes in %v (%v/write)",
			numGoroutines*writesPerGoroutine, elapsed, elapsed/time.Duration(numGoroutines*writesPerGoroutine))
	})

	t.Run("with_lock", func(t *testing.T) {
		path := filepath.Join(dir, "concurrent_with_lock.bin")
		var mu sync.Mutex
		data := make([]byte, bytesToWrite)
		for i := range data {
			data[i] = byte('a' + i%26)
		}

		numGoroutines := 10
		writesPerGoroutine := 100
		var writeCount int64

		start := time.Now()
		var wg sync.WaitGroup
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				for j := 0; j < writesPerGoroutine; j++ {
					mu.Lock()
					_ = os.WriteFile(path, data, 0644)
					mu.Unlock()
					atomic.AddInt64(&writeCount, 1)
				}
			}(i)
		}
		wg.Wait()
		elapsed := time.Since(start)

		t.Logf("With lock: %d writes in %v (%v/write)",
			writeCount, elapsed, elapsed/time.Duration(writeCount))
	})

	t.Run("to_same_file_parallel", func(t *testing.T) {
		// Multiple goroutines write to different files in parallel (no contention)
		numGoroutines := 10
		writesPerGoroutine := 100
		data := make([]byte, bytesToWrite)
		for i := range data {
			data[i] = byte('a' + i%26)
		}

		start := time.Now()
		var wg sync.WaitGroup
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				for j := 0; j < writesPerGoroutine; j++ {
					path := filepath.Join(dir, fmt.Sprintf("parallel_write_%d_%d.bin", idx, j))
					_ = os.WriteFile(path, data, 0644)
				}
			}(i)
		}
		wg.Wait()
		elapsed := time.Since(start)

		totalWrites := numGoroutines * writesPerGoroutine
		t.Logf("Parallel (different files): %d writes in %v (%v/write)",
			totalWrites, elapsed, elapsed/time.Duration(totalWrites))
	})
}

// TestFileAccess_BatchVsSingle compares batch vs single file operations.
func TestFileAccess_BatchVsSingle(t *testing.T) {
	dir := makeTempDir(t)
	fileCount := 50
	fileSize := 1024

	// Create test files
	for i := 0; i < fileCount; i++ {
		require.NoError(t, writeFile(dir, fmt.Sprintf("batch_%d.txt", i), fileSize))
	}

	// Single: sequential reads
	start := time.Now()
	for i := 0; i < fileCount; i++ {
		_, _ = os.ReadFile(filepath.Join(dir, fmt.Sprintf("batch_%d.txt", i)))
	}
	singleTotal := time.Since(start)

	// Batch: parallel reads using goroutines
	start = time.Now()
	var wg sync.WaitGroup
	for i := 0; i < fileCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, _ = os.ReadFile(filepath.Join(dir, fmt.Sprintf("batch_%d.txt", idx)))
		}(i)
	}
	wg.Wait()
	batchTotal := time.Since(start)

	// Single writes
	start = time.Now()
	data := make([]byte, fileSize)
	for i := range data {
		data[i] = byte('x')
	}
	for i := 0; i < fileCount; i++ {
		_ = os.WriteFile(filepath.Join(dir, fmt.Sprintf("single_write_%d.txt", i)), data, 0644)
	}
	singleWriteTotal := time.Since(start)

	// Batch writes (parallel)
	start = time.Now()
	for i := 0; i < fileCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_ = os.WriteFile(filepath.Join(dir, fmt.Sprintf("batch_write_%d.txt", idx)), data, 0644)
		}(i)
	}
	wg.Wait()
	batchWriteTotal := time.Since(start)

	t.Logf("\n=== Batch vs Single Comparison ===")
	t.Logf("Read:  single=%v, batch(parallel)=%v, speedup=%.2fx",
		singleTotal, batchTotal, float64(singleTotal)/float64(batchTotal))
	t.Logf("Write: single=%v, batch(parallel)=%v, speedup=%.2fx",
		singleWriteTotal, batchWriteTotal, float64(singleWriteTotal)/float64(batchWriteTotal))

	// Batch should be faster due to parallelism
	assert.Less(t, batchTotal, singleTotal, "batch read should be faster than single")
}

// formatTableHeader returns a formatted table header for latency results.
func formatTableHeader() string {
	return fmt.Sprintf("%-20s %7s %10s %10s %10s %10s %10s %10s %10s",
		"Operation", "Count", "Avg", "Min", "P50", "P95", "P99", "Max", "Total")
}

// TestFileAccess_CPUCornerCases tests file access with various CPU scheduler conditions.
func TestFileAccess_CPUCornerCases(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CPU corner case tests in short mode")
	}

	dir := makeTempDir(t)
	require.NoError(t, writeFile(dir, "cpu_test.bin", 1024*1024))

	// Test with GOMAXPROCS = 1 (stress scheduler)
	oldProcs := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(oldProcs)

	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = os.ReadFile(filepath.Join(dir, "cpu_test.bin"))
		}()
	}
	wg.Wait()
	t.Logf("GOMAXPROCS=1: 100 reads in %v", time.Since(start))

	// Test with GOMAXPROCS = numCPU (max parallelism)
	runtime.GOMAXPROCS(runtime.NumCPU())
	start = time.Now()
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = os.ReadFile(filepath.Join(dir, "cpu_test.bin"))
		}()
	}
	wg.Wait()
	t.Logf("GOMAXPROCS=%d: 100 reads in %v", runtime.NumCPU(), time.Since(start))
}
