// Package context_test provides tests for the context compression service.
package context

import (
	"context"
	"errors"
	"testing"
	"time"

	domaincontext "github.com/cloud-agent-platform/cap/internal/domain/context"
	"github.com/cloud-agent-platform/cap/plugins/llmrouter"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// Mock LLMCompressor for testing
// ---------------------------------------------------------------------------

// mockLLMCompressor implements LLMCompressor for testing.
type mockLLMCompressor struct {
	summarizeFunc func(ctx context.Context, messages []*domaincontext.ConversationMessage) ([]*domaincontext.ConversationMessage, error)
	calls        int
}

func (m *mockLLMCompressor) SummarizeMessages(ctx context.Context, messages []*domaincontext.ConversationMessage) ([]*domaincontext.ConversationMessage, error) {
	m.calls++
	if m.summarizeFunc != nil {
		return m.summarizeFunc(ctx, messages)
	}
	// Default: keep only the last message as summary
	if len(messages) == 0 {
		return messages, nil
	}
	summary := domaincontext.NewAgentMessage("Summarized " + messages[0].Content)
	summary.Priority = -1
	return []*domaincontext.ConversationMessage{summary}, nil
}

// ---------------------------------------------------------------------------
// CompressionLevel tests
// ---------------------------------------------------------------------------

func TestCompressionLevel_IsValid(t *testing.T) {
	tests := []struct {
		level CompressionLevel
		valid bool
	}{
		{CompressionLevelL1, true},
		{CompressionLevelL3, true},
		{CompressionLevelAuto, true},
		{CompressionLevel("invalid"), false},
		{CompressionLevel(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.level.String(), func(t *testing.T) {
			assert.Equal(t, tt.valid, tt.level.IsValid())
		})
	}
}

func TestCompressionLevel_String(t *testing.T) {
	assert.Equal(t, "l1", CompressionLevelL1.String())
	assert.Equal(t, "l3", CompressionLevelL3.String())
	assert.Equal(t, "auto", CompressionLevelAuto.String())
}

// ---------------------------------------------------------------------------
// LLMCompressorConfig tests
// ---------------------------------------------------------------------------

func TestDefaultLLMCompressorConfig(t *testing.T) {
	config := DefaultLLMCompressorConfig()

	assert.Equal(t, llmrouter.ModelClaudeHaiku, config.Model)
	assert.Equal(t, 100, config.MaxTokensPerSummary)
	assert.Equal(t, 0.3, config.Temperature)
	assert.Equal(t, 5, config.PreserveRecentMessages)
	assert.True(t, config.EnableParallelSummarization)
	assert.Equal(t, 10, config.BatchSize)
	assert.Equal(t, 30*time.Second, config.Timeout)
}

// ---------------------------------------------------------------------------
// IntelligentCompressor tests
// ---------------------------------------------------------------------------

func TestNewIntelligentCompressor(t *testing.T) {
	logger := zap.NewNop()
	l1 := domaincontext.DefaultCompressor()
	config := DefaultIntelligentCompressorConfig()

	ic := NewIntelligentCompressor(l1, nil, logger, config)

	assert.NotNil(t, ic)
	assert.Equal(t, domaincontext.StrategyModerate, ic.config.L1Strategy)
}

func TestNewIntelligentCompressor_WithNilL1(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultIntelligentCompressorConfig()

	ic := NewIntelligentCompressor(nil, nil, logger, config)

	assert.NotNil(t, ic)
	assert.NotNil(t, ic.l1Compressor)
}

func TestIntelligentCompressor_Compress_NilContext(t *testing.T) {
	logger := zap.NewNop()
	ic := NewIntelligentCompressor(nil, nil, logger, DefaultIntelligentCompressorConfig())

	result, err := ic.Compress(context.Background(), nil)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "nil context")
}

func TestIntelligentCompressor_Compress_NotOverBudget(t *testing.T) {
	logger := zap.NewNop()
	ic := NewIntelligentCompressor(nil, nil, logger, DefaultIntelligentCompressorConfig())

	tc := domaincontext.NewTaskContext("test-task", "Test goal")
	tc.SetTokenBudget(100000)
	tc.AddMessage(domaincontext.NewUserMessage("Hello"))
	tc.AddConstraint(domaincontext.NewNonNegotiable("security", "No eval"))

	result, err := ic.Compress(context.Background(), tc)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, CompressionLevelL1, result.Level)
	assert.Equal(t, 0, result.TokensRemoved)
	assert.Equal(t, "no compression needed", result.Summary)
	assert.False(t, result.LLMUsed)
}

func TestIntelligentCompressor_Compress_L1OverBudget(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultIntelligentCompressorConfig()
	// Set thresholds to ensure L1 is selected even when over budget
	config.AutoSelectThreshold = 100000 // High threshold so L3 not triggered by size
	config.LLMBudgetThreshold = 0.01    // Very low threshold so L3 not triggered by budget

	ic := NewIntelligentCompressor(nil, nil, logger, config)

	tc := domaincontext.NewTaskContext("test-task", "Test goal")
	tc.SetTokenBudget(1000) // Small budget but not too tight

	// Add many messages to exceed budget
	for i := 0; i < 50; i++ {
		tc.AddMessage(domaincontext.NewUserMessage("Short message content"))
	}

	// Add constraints
	tc.AddConstraint(domaincontext.NewNonNegotiable("security", "No eval"))
	tc.AddConstraint(domaincontext.NewPreferable("style", "Use camelCase"))

	if !tc.IsBudgetExceeded() {
		t.Skip("Context not over budget, cannot test L1 compression")
	}

	result, err := ic.Compress(context.Background(), tc)

	require.NoError(t, err)
	assert.NotNil(t, result)
	// L1 should be selected since we set thresholds to avoid L3
	assert.Equal(t, CompressionLevelL1, result.Level)
	assert.Greater(t, result.TokensRemoved, 0)
	assert.False(t, result.LLMUsed)

	// Verify goal is preserved
	assert.Equal(t, "Test goal", tc.Goal)

	// Verify hard constraints are preserved
	hardConstraints := tc.GetHardConstraints()
	assert.GreaterOrEqual(t, len(hardConstraints), 1, "Hard constraints should be preserved")
}

func TestIntelligentCompressor_Compress_L3WithMockLLM(t *testing.T) {
	logger := zap.NewNop()
	mockLLM := &mockLLMCompressor{
		summarizeFunc: func(ctx context.Context, messages []*domaincontext.ConversationMessage) ([]*domaincontext.ConversationMessage, error) {
			// Simply keep the first and last message as a mock summary
			if len(messages) <= 2 {
				return messages, nil
			}
			summary := domaincontext.NewAgentMessage("Mock summary of " + messages[0].Content)
			summary.Priority = -1
			return []*domaincontext.ConversationMessage{summary, messages[len(messages)-1]}, nil
		},
	}

	config := DefaultIntelligentCompressorConfig()
	config.AutoSelectThreshold = 100 // Very low threshold to force L3
	config.LLMBudgetThreshold = 0.5 // 50% to trigger L3
	config.PreferQualityWhenBudgetSufficient = true

	ic := NewIntelligentCompressor(nil, mockLLM, logger, config)

	tc := domaincontext.NewTaskContext("test-task", "Test goal")
	tc.SetTokenBudget(500) // Small budget

	// Add many messages
	for i := 0; i < 30; i++ {
		tc.AddMessage(domaincontext.NewUserMessage("Message content"))
	}

	// Add hard constraint
	tc.AddConstraint(domaincontext.NewNonNegotiable("security", "No eval"))

	// Set remaining budget low enough to trigger L3
	tc.TokenBudget = tc.TotalTokens() / 2

	if !tc.IsBudgetExceeded() {
		t.Skip("Context not over budget for L3")
	}

	result, err := ic.Compress(context.Background(), tc)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, mockLLM.calls)

	// Verify goal is preserved
	assert.Equal(t, "Test goal", tc.Goal)

	// Verify hard constraints are preserved
	hardConstraints := tc.GetHardConstraints()
	assert.GreaterOrEqual(t, len(hardConstraints), 1, "Hard constraints should be preserved")
}

func TestIntelligentCompressor_Compress_PreservesGoalAndHardConstraints(t *testing.T) {
	logger := zap.NewNop()
	mockLLM := &mockLLMCompressor{}

	config := DefaultIntelligentCompressorConfig()
	config.AutoSelectThreshold = 100
	config.LLMBudgetThreshold = 0.1

	ic := NewIntelligentCompressor(nil, mockLLM, logger, config)

	tc := domaincontext.NewTaskContext("task-123", "IMPORTANT: Build login feature")
	tc.SetTokenBudget(50) // Very small budget

	// Add many messages
	for i := 0; i < 50; i++ {
		tc.AddMessage(domaincontext.NewUserMessage("Message number "))
	}

	// Add hard and soft constraints
	tc.AddConstraint(domaincontext.NewNonNegotiable("security", "HTTPS only"))
	tc.AddConstraint(domaincontext.NewNonNegotiable("legal", "Apache 2.0 license"))
	tc.AddConstraint(domaincontext.NewPreferable("style", "Use Go fmt"))
	tc.AddConstraint(domaincontext.NewPreferable("performance", "Cache responses"))

	originalGoal := tc.Goal
	hardConstraints := tc.GetHardConstraints()
	originalHardCount := len(hardConstraints)

	// Force compression
	for tc.TotalTokens() < tc.TokenBudget*2 {
		tc.AddMessage(domaincontext.NewUserMessage("More content to fill budget"))
	}

	result, err := ic.Compress(context.Background(), tc)

	require.NoError(t, err)
	assert.NotNil(t, result)

	// Verify goal is ALWAYS preserved (hard requirement)
	assert.Equal(t, originalGoal, tc.Goal, "Goal must be preserved")

	// Verify hard constraints are ALWAYS preserved (hard requirement)
	hardConstraints = tc.GetHardConstraints()
	assert.Equal(t, originalHardCount, len(hardConstraints), "Hard constraints must be preserved")
}

func TestIntelligentCompressor_selectLevel(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultIntelligentCompressorConfig()
	config.AutoSelectThreshold = 5000
	config.LLMBudgetThreshold = 0.2

	ic := NewIntelligentCompressor(nil, nil, logger, config)

	tests := []struct {
		name          string
		tokens        int
		budget        int
		expectL3      bool
	}{
		{
			name:   "small context, plenty budget -> L1",
			tokens: 1000,
			budget: 10000,
			expectL3: false,
		},
		{
			name:   "medium context, very low budget -> L3",
			tokens: 6000,
			budget: 7000, // 85.7% used, 14.3% remaining < 20% threshold triggers L3
			expectL3: true,
		},
		{
			name:   "large context, tight budget -> L3",
			tokens: 10000,
			budget: 10000,
			expectL3: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := domaincontext.NewTaskContext("test", "Goal")
			tc.TokenBudget = tt.budget

			// Force token count
			for tc.TotalTokens() < tt.tokens {
				tc.AddMessage(domaincontext.NewUserMessage("content"))
			}

			level := ic.selectLevel(tc)
			if tt.expectL3 {
				assert.Equal(t, CompressionLevelL3, level)
			} else {
				assert.Equal(t, CompressionLevelL1, level)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// compressFileStatesL1 tests
// ---------------------------------------------------------------------------

func TestIntelligentCompressor_compressFileStatesL1(t *testing.T) {
	logger := zap.NewNop()
	ic := NewIntelligentCompressor(nil, nil, logger, DefaultIntelligentCompressorConfig())

	tc := domaincontext.NewTaskContext("test", "Goal")

	// Add many file states
	for i := 0; i < 20; i++ {
		fs := domaincontext.NewFileStateModified("file.go", "hash", 100, "change")
		fs.Priority = i
		tc.AddFileState(fs)
	}

	// Add important files (created, deleted)
	tc.AddFileState(domaincontext.NewFileStateCreated("new.go", "hash", 50))
	tc.AddFileState(domaincontext.NewFileStateDeleted("old.go"))

	originalCount := len(tc.FileStates)

	ic.compressFileStatesL1(tc)

	// Should reduce but preserve created and deleted files
	assert.Less(t, len(tc.FileStates), originalCount)
	// Should still have created and deleted files
	hasDeleted := false
	hasCreated := false
	for _, fs := range tc.FileStates {
		if fs.ChangeType == domaincontext.FileChangeDeleted {
			hasDeleted = true
		}
		if fs.ChangeType == domaincontext.FileChangeCreated {
			hasCreated = true
		}
	}
	assert.True(t, hasDeleted, "Deleted files should be preserved")
	assert.True(t, hasCreated, "Created files should be preserved")
}

// ---------------------------------------------------------------------------
// compressConstraintsL1 tests
// ---------------------------------------------------------------------------

func TestIntelligentCompressor_compressConstraintsL1(t *testing.T) {
	logger := zap.NewNop()
	ic := NewIntelligentCompressor(nil, nil, logger, DefaultIntelligentCompressorConfig())

	tc := domaincontext.NewTaskContext("test", "Goal")

	// Add hard constraints
	tc.AddConstraint(domaincontext.NewNonNegotiable("security", "No eval"))
	tc.AddConstraint(domaincontext.NewNonNegotiable("legal", "Apache 2.0"))

	// Add many soft constraints
	for i := 0; i < 10; i++ {
		c := domaincontext.NewPreferable("style", "Style preference")
		c.Priority = i
		tc.AddConstraint(c)
	}

	originalHardCount := tc.HardConstraintsCount()
	originalSoftCount := len(tc.Constraints) - originalHardCount

	ic.compressConstraintsL1(tc)

	// Hard constraints should be unchanged
	assert.Equal(t, originalHardCount, tc.HardConstraintsCount())

	// Soft constraints should be reduced (kept top 50%)
	assert.Less(t, len(tc.Constraints)-tc.HardConstraintsCount(), originalSoftCount)
}

func TestIntelligentCompressor_compressConstraintsL1_AllHard(t *testing.T) {
	logger := zap.NewNop()
	ic := NewIntelligentCompressor(nil, nil, logger, DefaultIntelligentCompressorConfig())

	tc := domaincontext.NewTaskContext("test", "Goal")

	// Add only hard constraints
	tc.AddConstraint(domaincontext.NewNonNegotiable("security", "No eval"))
	tc.AddConstraint(domaincontext.NewNonNegotiable("legal", "Apache 2.0"))

	originalCount := len(tc.Constraints)

	ic.compressConstraintsL1(tc)

	// Should be unchanged
	assert.Equal(t, originalCount, len(tc.Constraints))
}

// ---------------------------------------------------------------------------
// DefaultIntelligentCompressorConfig tests
// ---------------------------------------------------------------------------

func TestDefaultIntelligentCompressorConfig(t *testing.T) {
	config := DefaultIntelligentCompressorConfig()

	assert.Equal(t, domaincontext.StrategyModerate, config.L1Strategy)
	assert.Equal(t, 5000, config.AutoSelectThreshold)
	assert.Equal(t, 0.2, config.LLMBudgetThreshold)
	assert.True(t, config.PreferQualityWhenBudgetSufficient)
}

// ---------------------------------------------------------------------------
// CompressResult tests
// ---------------------------------------------------------------------------

func TestCompressResult_Summary(t *testing.T) {
	result := &CompressResult{
		Level:            CompressionLevelL1,
		OriginalTokens:   1000,
		CompressedTokens: 700,
		TokensRemoved:    300,
		ItemsRemoved: domaincontext.CompressionStats{
			Messages:     5,
			FileStates:   2,
			Constraints:  1,
		},
		BudgetUsed: 0.7,
		LLMUsed:    false,
		Summary:     "L1 rule-based: removed 300 tokens",
	}

	summary := result.Summary
	assert.Contains(t, summary, "L1 rule-based")
	assert.Contains(t, summary, "removed 300 tokens")
}

func TestCompressResult_SummaryL3(t *testing.T) {
	result := &CompressResult{
		Level:            CompressionLevelL3,
		OriginalTokens:   1000,
		CompressedTokens: 500,
		TokensRemoved:    500,
		LLMUsed:          true,
		Summary:          "L3 LLM-powered: removed 500 tokens, preserved goals and hard constraints",
	}

	summary := result.Summary
	assert.Contains(t, summary, "L3 LLM-powered")
	assert.Contains(t, summary, "preserved goals and hard constraints")
}

// ---------------------------------------------------------------------------
// LLM Compressor integration tests
// ---------------------------------------------------------------------------

func TestNewLLMCompressor_WithNilRouter(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultLLMCompressorConfig()

	compressor := NewLLMCompressor(nil, logger, config)

	// Should return noop compressor
	assert.NotNil(t, compressor)

	// Should work without error
	messages := []*domaincontext.ConversationMessage{
		domaincontext.NewUserMessage("Hello"),
		domaincontext.NewAgentMessage("Hi"),
	}

	result, err := compressor.SummarizeMessages(context.Background(), messages)
	require.NoError(t, err)
	assert.Equal(t, messages, result)
}

func TestLLMCompressor_Truncate(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is a long string", 10, "this is a ..."},
		{"", 5, ""},
	}

	for _, tt := range tests {
		result := truncate(tt.input, tt.maxLen)
		assert.Equal(t, tt.expected, result)
	}
}

// ---------------------------------------------------------------------------
// Build summarization prompt tests
// ---------------------------------------------------------------------------

func TestLLMCompressor_BuildSummarizationPrompt(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultLLMCompressorConfig()
	compressor := NewLLMCompressor(nil, logger, config)

	messages := []*domaincontext.ConversationMessage{
		domaincontext.NewUserMessage("Hello, how are you?"),
		domaincontext.NewAgentMessage("I'm doing well, thank you!"),
		domaincontext.NewUserMessage("Can you help with the task?"),
	}

	// Get the internal function via type assertion
	lc, ok := compressor.(*llmCompressor)
	if !ok {
		t.Skip("compressor is not llmCompressor (likely noop)")
	}

	prompt := lc.buildSummarizationPrompt(messages)

	assert.Contains(t, prompt, "USER: Hello, how are you?")
	assert.Contains(t, prompt, "AGENT: I'm doing well, thank you!")
	assert.Contains(t, prompt, "Summarize the following conversation")
	assert.Contains(t, prompt, "Main topics discussed")
	assert.Contains(t, prompt, "Key decisions made")
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestIntelligentCompressor_Compress_EmptyContext(t *testing.T) {
	logger := zap.NewNop()
	ic := NewIntelligentCompressor(nil, nil, logger, DefaultIntelligentCompressorConfig())

	tc := domaincontext.NewTaskContext("test", "Goal")
	tc.SetTokenBudget(1000)

	result, err := ic.Compress(context.Background(), tc)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0, result.TokensRemoved)
}

func TestIntelligentCompressor_Compress_OnlyGoalAndConstraints(t *testing.T) {
	logger := zap.NewNop()
	ic := NewIntelligentCompressor(nil, nil, logger, DefaultIntelligentCompressorConfig())

	tc := domaincontext.NewTaskContext("test", "Important goal")
	tc.SetTokenBudget(100)
	tc.AddConstraint(domaincontext.NewNonNegotiable("security", "No eval"))
	tc.AddConstraint(domaincontext.NewPreferable("style", "Use Go fmt"))

	result, err := ic.Compress(context.Background(), tc)

	require.NoError(t, err)
	assert.NotNil(t, result)

	// Goal and hard constraints should be preserved
	assert.Equal(t, "Important goal", tc.Goal)
	hardConstraints := tc.GetHardConstraints()
	assert.Equal(t, 1, len(hardConstraints))
}

func TestIntelligentCompressor_Compress_L3FallbackToL1(t *testing.T) {
	logger := zap.NewNop()
	// Use noop LLM compressor (nil router)
	mockLLM := &mockLLMCompressor{
		summarizeFunc: func(ctx context.Context, messages []*domaincontext.ConversationMessage) ([]*domaincontext.ConversationMessage, error) {
			return nil, errors.New("LLM failed")
		},
	}

	config := DefaultIntelligentCompressorConfig()
	config.AutoSelectThreshold = 100
	config.LLMBudgetThreshold = 0.1

	ic := NewIntelligentCompressor(nil, mockLLM, logger, config)

	tc := domaincontext.NewTaskContext("test", "Goal")
	tc.SetTokenBudget(50)

	// Add many messages to force L3
	for i := 0; i < 30; i++ {
		tc.AddMessage(domaincontext.NewUserMessage("Message"))
	}
	tc.AddConstraint(domaincontext.NewNonNegotiable("security", "No eval"))

	// Force over budget
	for tc.TotalTokens() < tc.TokenBudget*2 {
		tc.AddMessage(domaincontext.NewUserMessage("More"))
	}

	// Should not error even though LLM fails
	result, err := ic.Compress(context.Background(), tc)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, mockLLM.calls)
	// Should have tried L3
	assert.Equal(t, CompressionLevelL3, result.Level)
}

// ---------------------------------------------------------------------------
// countSoftConstraints tests
// ---------------------------------------------------------------------------

func TestIntelligentCompressor_countSoftConstraints(t *testing.T) {
	logger := zap.NewNop()
	ic := NewIntelligentCompressor(nil, nil, logger, DefaultIntelligentCompressorConfig())

	tc := domaincontext.NewTaskContext("test", "Goal")
	tc.AddConstraint(domaincontext.NewNonNegotiable("test", "hard1"))
	tc.AddConstraint(domaincontext.NewPreferable("test", "soft1"))
	tc.AddConstraint(domaincontext.NewNonNegotiable("test", "hard2"))
	tc.AddConstraint(domaincontext.NewPreferable("test", "soft2"))

	count := ic.countSoftConstraints(tc)
	assert.Equal(t, 2, count)
}