// Package context_test provides tests for the context propagation service.
package context

import (
	"context"
	"errors"
	"testing"
	"time"

	domaincontext "github.com/cloud-agent-platform/cap/internal/domain/context"
	"github.com/cloud-agent-platform/cap/internal/infra/cache"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// mockContextCache implements ContextCache for testing.
type mockContextCache struct {
	data map[string]*domaincontext.TaskContext
}

func newMockContextCache() *mockContextCache {
	return &mockContextCache{
		data: make(map[string]*domaincontext.TaskContext),
	}
}

func (m *mockContextCache) Get(ctx context.Context, taskID string) (*domaincontext.TaskContext, error) {
	if tc, ok := m.data[taskID]; ok {
		return tc, nil
	}
	return nil, cache.ErrCacheMiss
}

func (m *mockContextCache) Set(ctx context.Context, taskCtx *domaincontext.TaskContext, ttl time.Duration) error {
	m.data[taskCtx.TaskID] = taskCtx
	return nil
}

func (m *mockContextCache) Delete(ctx context.Context, taskID string) error {
	delete(m.data, taskID)
	return nil
}

func (m *mockContextCache) reset() {
	m.data = make(map[string]*domaincontext.TaskContext)
}

func TestPropagationMode_IsValid(t *testing.T) {
	tests := []struct {
		mode  PropagationMode
		valid bool
	}{
		{ModeFull, true},
		{ModeSummary, true},
		{ModeDelta, true},
		{ModeAuto, true},
		{PropagationMode("invalid"), false},
		{PropagationMode(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.mode.String(), func(t *testing.T) {
			assert.Equal(t, tt.valid, tt.mode.IsValid())
		})
	}
}

func TestDefaultThresholds(t *testing.T) {
	thresholds := DefaultThresholds()

	assert.Equal(t, 32000, thresholds.FullToSummaryTokens)
	assert.Equal(t, 96000, thresholds.SummaryToDeltaTokens)
	assert.Equal(t, 20, thresholds.ShortTaskMessages)
	assert.Equal(t, 200, thresholds.LongTaskMessages)
}

func TestContextService_Propagate_FullMode(t *testing.T) {
	logger := zap.NewNop()
	cache := newMockContextCache()

	svc := NewContextService(ContextServiceInput{
		Cache:  cache,
		Logger: logger,
	})

	ctx := context.Background()
	taskID := "task-full-test"

	// Create source context
	srcCtx := domaincontext.NewTaskContext(taskID, "Test goal")
	srcCtx.AddMessage(domaincontext.NewUserMessage("Hello"))
	srcCtx.AddMessage(domaincontext.NewAgentMessage("Hi there"))
	srcCtx.AddConstraint(domaincontext.NewNonNegotiable("security", "No eval"))
	srcCtx.AddFileState(domaincontext.NewFileStateModified("main.go", "abc123", 1000, "added feature"))

	// Cache the source
	require.NoError(t, cache.Set(ctx, srcCtx, time.Hour))

	input := PropagationInput{
		TaskID: taskID,
		Mode:   ModeFull,
	}

	output, err := svc.Propagate(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, output)

	assert.Equal(t, ModeFull, output.Mode)
	assert.NotNil(t, output.TaskContext)
	assert.Equal(t, srcCtx.Goal, output.TaskContext.Goal)
	assert.Equal(t, 2, len(output.TaskContext.Messages))
	assert.Equal(t, 1, len(output.TaskContext.Constraints))
	assert.Equal(t, 1, len(output.TaskContext.FileStates))
	assert.True(t, output.IsCached)
}

func TestContextService_Propagate_SummaryMode(t *testing.T) {
	logger := zap.NewNop()
	cache := newMockContextCache()

	svc := NewContextService(ContextServiceInput{
		Cache:  cache,
		Logger: logger,
	})

	ctx := context.Background()
	taskID := "task-summary-test"

	// Create source context with many messages to trigger compression
	srcCtx := domaincontext.NewTaskContext(taskID, "Test goal with many messages")
	srcCtx.TokenBudget = 1000 // Small budget for testing

	// Add many short messages with varying priorities
	for i := 0; i < 100; i++ {
		msg := domaincontext.NewUserMessage("Short message")
		msg.Priority = i % 10 // Vary priority so compression removes some
		srcCtx.AddMessage(msg)
	}

	// Add constraints
	for i := 0; i < 20; i++ {
		srcCtx.AddConstraint(domaincontext.NewPreferable("style", "constraint"))
	}

	// Cache the source
	require.NoError(t, cache.Set(ctx, srcCtx, time.Hour))

	input := PropagationInput{
		TaskID: taskID,
		Mode:   ModeSummary,
	}

	output, err := svc.Propagate(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, output)

	assert.Equal(t, ModeSummary, output.Mode)
	assert.NotNil(t, output.TaskContext)

	// Summary mode should produce a result
	assert.Greater(t, output.Tokens, 0)

	// Budget used should be reasonable
	assert.Less(t, output.BudgetUsed, 1.5) // Should not exceed 150% of budget
}

func TestContextService_Propagate_DeltaMode(t *testing.T) {
	logger := zap.NewNop()
	cache := newMockContextCache()

	svc := NewContextService(ContextServiceInput{
		Cache:  cache,
		Logger: logger,
	})

	ctx := context.Background()
	taskID := "task-delta-test"

	// Create initial context and cache it
	initialCtx := domaincontext.NewTaskContext(taskID, "Initial context")
	initialCtx.AddMessage(domaincontext.NewUserMessage("Initial message"))
	initialCtx.AddConstraint(domaincontext.NewNonNegotiable("security", "No eval"))
	initialCtx.Version = 1

	// Cache initial state
	cache.Set(ctx, initialCtx, time.Hour)

	// Create updated context with new messages
	updatedCtx := domaincontext.NewTaskContext(taskID, "Updated context")
	updatedCtx.Version = 2

	// Keep old message
	for _, msg := range initialCtx.Messages {
		updatedCtx.AddMessage(msg)
	}

	// Add new messages
	updatedCtx.AddMessage(domaincontext.NewAgentMessage("Response to initial"))
	updatedCtx.AddMessage(domaincontext.NewUserMessage("Follow up question"))

	// Add new constraint
	updatedCtx.AddConstraint(domaincontext.NewNonNegotiable("security", "No eval"))
	updatedCtx.AddConstraint(domaincontext.NewPreferable("style", "New style preference"))

	// Cache updated
	require.NoError(t, cache.Set(ctx, updatedCtx, time.Hour))

	input := PropagationInput{
		TaskID: taskID,
		Mode:   ModeDelta,
	}

	output, err := svc.Propagate(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, output)

	assert.Equal(t, ModeDelta, output.Mode)
	assert.NotNil(t, output.TaskContext)

	// Delta should include new messages
	assert.GreaterOrEqual(t, len(output.TaskContext.Messages), 1)

	// Should preserve hard constraints
	hardConstraints := output.TaskContext.GetHardConstraints()
	assert.GreaterOrEqual(t, len(hardConstraints), 1)
}

func TestContextService_Propagate_AutoMode(t *testing.T) {
	logger := zap.NewNop()
	cache := newMockContextCache()

	thresholds := ModeSelectionThresholds{
		FullToSummaryTokens:  1000,
		SummaryToDeltaTokens: 5000,
		ShortTaskMessages:   10,
		LongTaskMessages:   50,
	}

	svc := NewContextService(ContextServiceInput{
		Cache:      cache,
		Logger:     logger,
		Thresholds: &thresholds,
	})

	ctx := context.Background()

	tests := []struct {
		name            string
		messageCount    int
		tokenBudget     int
		expectedMode    PropagationMode
	}{
		{
			name:         "short task uses full mode",
			messageCount: 5,
			tokenBudget:  10000,
			expectedMode: ModeFull,
		},
		{
			name:         "medium task uses summary mode",
			messageCount: 15,
			tokenBudget:  10000,
			expectedMode: ModeSummary,
		},
		{
			name:         "long task uses delta mode",
			messageCount: 60,
			tokenBudget:  10000,
			expectedMode: ModeDelta,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache.reset()
			taskID := "task-auto-" + tt.name

			srcCtx := domaincontext.NewTaskContext(taskID, "Test")
			srcCtx.TokenBudget = tt.tokenBudget

			for i := 0; i < tt.messageCount; i++ {
				srcCtx.AddMessage(domaincontext.NewUserMessage("Message content"))
			}

			require.NoError(t, cache.Set(ctx, srcCtx, time.Hour))

			input := PropagationInput{
				TaskID: taskID,
				Mode:   ModeAuto,
			}

			output, err := svc.Propagate(ctx, input)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedMode, output.Mode)
		})
	}
}

func TestContextService_SelectModeForTask(t *testing.T) {
	logger := zap.NewNop()
	cache := newMockContextCache()

	svc := NewContextService(ContextServiceInput{
		Cache:  cache,
		Logger: logger,
	})

	ctx := context.Background()
	taskID := "task-select-test"

	// Create context with specific characteristics
	srcCtx := domaincontext.NewTaskContext(taskID, "Test")
	srcCtx.TokenBudget = 128000

	// Add enough messages to trigger summary mode
	for i := 0; i < 30; i++ {
		srcCtx.AddMessage(domaincontext.NewUserMessage("Message"))
	}

	require.NoError(t, cache.Set(ctx, srcCtx, time.Hour))

	// Test without complexity hint
	mode, reason, err := svc.SelectModeForTask(ctx, taskID, 0)
	require.NoError(t, err)
	assert.NotEmpty(t, reason)
	t.Logf("Mode: %s, Reason: %s", mode, reason)

	// Test with high complexity hint
	mode, reason, err = svc.SelectModeForTask(ctx, taskID, 9)
	require.NoError(t, err)
	assert.Equal(t, ModeDelta, mode)
	assert.Contains(t, reason, "complexity")
}

func TestContextService_CacheContext(t *testing.T) {
	logger := zap.NewNop()
	mockCache := newMockContextCache()

	svc := NewContextService(ContextServiceInput{
		Cache:  mockCache,
		Logger: logger,
	})

	ctx := context.Background()
	tc := domaincontext.NewTaskContext("task-cache-test", "Goal")

	err := svc.CacheContext(ctx, tc, time.Hour)
	require.NoError(t, err)

	// Verify it's in cache
	cached, err := mockCache.Get(ctx, "task-cache-test")
	require.NoError(t, err)
	assert.Equal(t, "Goal", cached.Goal)
}

func TestContextService_InvalidateCache(t *testing.T) {
	logger := zap.NewNop()
	mockCache := newMockContextCache()

	svc := NewContextService(ContextServiceInput{
		Cache:  mockCache,
		Logger: logger,
	})

	ctx := context.Background()
	taskID := "task-invalidate-test"

	// Add to cache
	tc := domaincontext.NewTaskContext(taskID, "Goal")
	require.NoError(t, mockCache.Set(ctx, tc, time.Hour))

	// Verify it's there
	_, err := mockCache.Get(ctx, taskID)
	require.NoError(t, err)

	// Invalidate
	err = svc.InvalidateCache(ctx, taskID)
	require.NoError(t, err)

	// Verify it's gone
	_, err = mockCache.Get(ctx, taskID)
	assert.True(t, errors.Is(err, cache.ErrCacheMiss))
}

func TestContextService_Propagate_MissingTaskID(t *testing.T) {
	logger := zap.NewNop()
	cache := newMockContextCache()

	svc := NewContextService(ContextServiceInput{
		Cache:  cache,
		Logger: logger,
	})

	ctx := context.Background()

	input := PropagationInput{
		TaskID: "",
		Mode:   ModeFull,
	}

	output, err := svc.Propagate(ctx, input)
	assert.Error(t, err)
	assert.Nil(t, output)
	assert.Contains(t, err.Error(), "task_id")
}

func TestContextService_Propagate_CacheMiss(t *testing.T) {
	logger := zap.NewNop()
	cache := newMockContextCache()

	svc := NewContextService(ContextServiceInput{
		Cache:  cache,
		Logger: logger,
	})

	ctx := context.Background()

	// Don't add anything to cache
	input := PropagationInput{
		TaskID: "nonexistent-task",
		Mode:   ModeFull,
	}

	// Should not error, just return empty context
	output, err := svc.Propagate(ctx, input)
	require.NoError(t, err)
	assert.NotNil(t, output)
	assert.Equal(t, ModeFull, output.Mode)
	assert.False(t, output.IsCached)
}

func TestNewContextSnapshot(t *testing.T) {
	tc := domaincontext.NewTaskContext("test-task", "Test goal")
	tc.AddMessage(domaincontext.NewUserMessage("Hello"))
	tc.AddMessage(domaincontext.NewAgentMessage("Hi"))
	tc.AddFileState(domaincontext.NewFileStateModified("main.go", "abc", 100, ""))

	snapshot := NewContextSnapshot(tc)

	assert.Equal(t, "test-task", snapshot.TaskID)
	assert.Greater(t, snapshot.Tokens, 0) // Tokens reflects total tokens in context
	assert.NotEmpty(t, snapshot.MessagesHash)
	assert.NotEmpty(t, snapshot.FilesHash)
	assert.False(t, snapshot.SnapshotAt.IsZero())
}

func TestPropagationOutput_MarshalJSON(t *testing.T) {
	output := &PropagationOutput{
		Mode:             ModeSummary,
		Tokens:           5000,
		BudgetUsed:       0.5,
		CompressionRatio: 0.75,
		IsCached:         true,
	}

	data, err := output.MarshalJSON()
	require.NoError(t, err)
	assert.Contains(t, string(data), `"mode":"summary"`)
	assert.Contains(t, string(data), `"Tokens":5000`) // Note: capital T
}

func TestBuildSummary(t *testing.T) {
	logger := zap.NewNop()
	cache := newMockContextCache()

	svc := NewContextService(ContextServiceInput{
		Cache:  cache,
		Logger: logger,
	})

	output := &PropagationOutput{
		Mode:             ModeFull,
		Tokens:           1000,
		BudgetUsed:       0.5,
		CompressionRatio: 1.0,
	}

	summary := svc.buildSummary(ModeFull, output)
	assert.Contains(t, summary, "mode=full")
	assert.Contains(t, summary, "tokens=1000")
	assert.Contains(t, summary, "budget_used=")
}

func TestDeltaCacheKey(t *testing.T) {
	logger := zap.NewNop()
	cache := newMockContextCache()

	svc := NewContextService(ContextServiceInput{
		Cache:  cache,
		Logger: logger,
	})

	key := svc.deltaCacheKey("test-task-123")
	assert.Equal(t, "context:delta:test-task-123", key)
}

func TestHashMessages(t *testing.T) {
	messages := []*domaincontext.ConversationMessage{
		{ID: "msg-1"},
		{ID: "msg-2"},
	}

	hash := hashMessages(messages)
	assert.Contains(t, hash, "2")
	assert.Contains(t, hash, "msg-2")
}

func TestHashFiles(t *testing.T) {
	files := []*domaincontext.FileState{
		{Path: "main.go"},
		{Path: "utils.go"},
	}

	hash := hashFiles(files)
	assert.Contains(t, hash, "2")
	assert.Contains(t, hash, "utils.go")
}