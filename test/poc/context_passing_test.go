// Package poc provides proof-of-concept limit tests.
package poc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestContextPassing_BaseOverhead measures the base overhead of context passing.
func TestContextPassing_BaseOverhead(t *testing.T) {
	ctx := context.Background()

	r := measureLatencies("context-background", 10000, func() error {
		_ = ctx.Value("key")
		return nil
	})
	r.Log(t)
	assert.Less(t, r.P99, 1*time.Microsecond, "ctx.Value P99 should be < 1μs")
}

// TestContextPassing_WithValueOverhead measures overhead of context with values.
func TestContextPassing_WithValueOverhead(t *testing.T) {
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		ctx = context.WithValue(ctx, fmt.Sprintf("key_%d", i), fmt.Sprintf("value_%d", i))
	}

	r := measureLatencies("context-with-20-values", 10000, func() error {
		_ = ctx.Value("key_19")
		return nil
	})
	r.Log(t)
	assert.Less(t, r.P99, 10*time.Microsecond, "ctx.Value(20 keys) P99 should be < 10μs")
}

// TestContextPassing_JSONMarshalCost measures JSON marshal/unmarshal cost for context data.
func TestContextPassing_JSONMarshalCost(t *testing.T) {
	type TaskContext struct {
		Goal        string   `json:"goal"`
		Files       []string `json:"files"`
		Constraints []string `json:"constraints"`
		History     []string `json:"history"`
	}

	tc := TaskContext{
		Goal:        "Fix the authentication bug",
		Files:       make([]string, 50),
		Constraints: make([]string, 10),
		History:     make([]string, 100),
	}
	for i := range tc.Files {
		tc.Files[i] = fmt.Sprintf("src/module%d/file%d.go", i/10, i)
	}
	for i := range tc.Constraints {
		tc.Constraints[i] = fmt.Sprintf("constraint_%d", i)
	}
	for i := range tc.History {
		tc.History[i] = fmt.Sprintf("step_%d: did something", i)
	}

	r := measureLatencies("json-marshal-context", 1000, func() error {
		_, err := json.Marshal(tc)
		return err
	})
	r.Log(t)
	assert.Less(t, r.P99, 1*time.Millisecond, "JSON marshal P99 should be < 1ms")

	r2 := measureLatencies("json-unmarshal-context", 1000, func() error {
		data, _ := json.Marshal(tc)
		var result TaskContext
		return json.Unmarshal(data, &result)
	})
	r2.Log(t)
	assert.Less(t, r2.P99, 2*time.Millisecond, "JSON round-trip P99 should be < 2ms")
}

// TestContextPassing_LargeContext measures cost of passing large context strings.
func TestContextPassing_LargeContext(t *testing.T) {
	sizes := []struct {
		name string
		size int
	}{
		{"1KB", 1024},
		{"10KB", 10 * 1024},
		{"100KB", 100 * 1024},
		{"1MB", 1024 * 1024},
	}

	for _, s := range sizes {
		t.Run(s.name, func(t *testing.T) {
			data := strings.Repeat("x", s.size)
			r := measureLatencies("pass-"+s.name, 100, func() error {
				ctx := context.WithValue(context.Background(), "data", data)
				_ = ctx.Value("data")
				return nil
			})
			r.Log(t)
		})
	}
}

// TestContextPassing_ConcurrentContexts measures cost of concurrent context operations.
func TestContextPassing_ConcurrentContexts(t *testing.T) {
	var wg sync.WaitGroup
	results := make(chan time.Duration, 50)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			start := time.Now()
			for j := 0; j < 100; j++ {
				ctx := context.WithValue(context.Background(), "agent", idx)
				ctx = context.WithValue(ctx, "task", j)
				_ = ctx.Value("agent")
				_ = ctx.Value("task")
			}
			results <- time.Since(start)
		}(i)
	}

	wg.Wait()
	close(results)

	var maxDur time.Duration
	for d := range results {
		if d > maxDur {
			maxDur = d
		}
	}
	t.Logf("50 concurrent agents (100 ops each): max=%v", maxDur)
	assert.Less(t, maxDur, 1*time.Second, "Should complete in < 1s")
}

// TestContextPassing_CancelPropagation measures cancel propagation speed.
func TestContextPassing_CancelPropagation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	r := measureLatencies("cancel-check", 10000, func() error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	})
	r.Log(t)
	assert.Less(t, r.P99, 1*time.Microsecond, "cancel check P99 should be < 1μs")

	// Measure propagation after cancel
	cancel()
	start := time.Now()
	<-ctx.Done()
	propagationTime := time.Since(start)
	t.Logf("Cancel propagation: %v", propagationTime)
	assert.Less(t, propagationTime, 1*time.Millisecond, "Cancel should propagate in < 1ms")
}

// TestContextPassing_TimeoutContext measures timeout context overhead.
func TestContextPassing_TimeoutContext(t *testing.T) {
	r := measureLatencies("with-timeout-create", 10000, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		cancel()
		_ = ctx
		return nil
	})
	r.Log(t)
	assert.Less(t, r.P99, 10*time.Microsecond, "WithTimeout creation P99 should be < 10μs")
}
