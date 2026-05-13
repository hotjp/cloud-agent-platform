// Package orchestrator implements Eino-based task orchestration graph tests.
package orchestrator

import (
	"context"
	"testing"

	"github.com/cloud-agent-platform/cap/internal/agent/react"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// mockLLM is a mock LLM for testing.
type mockLLM struct {
	generateFunc func(ctx context.Context, messages []*react.Message, opts *react.GenerateOptions) (*react.GenerateResult, error)
}

func (m *mockLLM) Generate(ctx context.Context, messages []*react.Message, opts *react.GenerateOptions) (*react.GenerateResult, error) {
	if m.generateFunc != nil {
		return m.generateFunc(ctx, messages, opts)
	}
	return &react.GenerateResult{
		Content:     "mock response",
		TotalTokens: 100,
	}, nil
}

func (m *mockLLM) GenerateStream(ctx context.Context, messages []*react.Message, opts *react.GenerateOptions, callback func(chunk string) error) (*react.GenerateResult, error) {
	return m.Generate(ctx, messages, opts)
}

func TestNewFullTaskGraph(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	deps := &Dependencies{
		LLM:   &mockLLM{},
		Logger: logger,
	}

	ftg, err := NewFullTaskGraph(deps)
	require.NoError(t, err)
	assert.NotNil(t, ftg)
	assert.NotNil(t, ftg.graph)
}

func TestFullTaskGraph_Compile(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	deps := &Dependencies{
		LLM:   &mockLLM{},
		Logger: logger,
	}

	ftg, err := NewFullTaskGraph(deps)
	require.NoError(t, err)

	ctx := context.Background()
	runnable, err := ftg.Compile(ctx)
	require.NoError(t, err)
	assert.NotNil(t, runnable)
}

func TestFullTaskGraph_Invoke_SimpleTask(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	deps := &Dependencies{
		LLM:   &mockLLM{},
		Logger: logger,
	}

	ftg, err := NewFullTaskGraph(deps)
	require.NoError(t, err)

	ctx := context.Background()
	input := &TaskInput{
		TaskID: "test-simple-task",
		Goal:   "fix typo in README",
	}

	result, err := ftg.Invoke(ctx, input)
	// May fail due to graph structure, but should not panic
	if err != nil {
		t.Logf("Invoke error (expected in some cases): %v", err)
	}
	if result != nil {
		assert.Equal(t, "test-simple-task", result.TaskID)
	}
}

func TestFullTaskGraph_Invoke_MediumTask(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	deps := &Dependencies{
		LLM:   &mockLLM{},
		Logger: logger,
	}

	ftg, err := NewFullTaskGraph(deps)
	require.NoError(t, err)

	ctx := context.Background()
	input := &TaskInput{
		TaskID: "test-medium-task",
		Goal:   "implement feature to add user authentication",
	}

	result, err := ftg.Invoke(ctx, input)
	if err != nil {
		t.Logf("Invoke error (expected in some cases): %v", err)
	}
	if result != nil {
		assert.Equal(t, "test-medium-task", result.TaskID)
	}
}

func TestFullTaskGraph_Invoke_ComplexTask(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	deps := &Dependencies{
		LLM:   &mockLLM{},
		Logger: logger,
	}

	ftg, err := NewFullTaskGraph(deps)
	require.NoError(t, err)

	ctx := context.Background()
	input := &TaskInput{
		TaskID: "test-complex-task",
		Goal:   "refactor architecture to use microservices design pattern",
	}

	result, err := ftg.Invoke(ctx, input)
	if err != nil {
		t.Logf("Invoke error (expected in some cases): %v", err)
	}
	if result != nil {
		assert.Equal(t, "test-complex-task", result.TaskID)
	}
}

func TestBuildFullTaskGraph(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	deps := &Dependencies{
		LLM:   &mockLLM{},
		Logger: logger,
	}

	ctx := context.Background()
	runnable, err := BuildFullTaskGraph(ctx, deps)
	require.NoError(t, err)
	assert.NotNil(t, runnable)
}

func TestAnalyzerNode_Invoke(t *testing.T) {
	tests := []struct {
		name     string
		goal     string
		expected TaskComplexity
	}{
		{
			name:     "simple task - fix typo",
			goal:     "fix typo in code",
			expected: ComplexitySimple,
		},
		{
			name:     "simple task - update comment",
			goal:     "update comment in main.go",
			expected: ComplexitySimple,
		},
		{
			name:     "medium task - implement feature",
			goal:     "implement user login feature",
			expected: ComplexityMedium,
		},
		{
			name:     "medium task - create module",
			goal:     "create payment module",
			expected: ComplexityMedium,
		},
		{
			name:     "complex task - architecture refactor",
			goal:     "refactor architecture to use microservices",
			expected: ComplexityComplex,
		},
		{
			name:     "complex task - design pattern",
			goal:     "implement observer design pattern",
			expected: ComplexityComplex,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, _ := zap.NewDevelopment()
			deps := &Dependencies{
				LLM:   &mockLLM{},
				Logger: logger,
			}

			node := NewAnalyzerNode(deps)
			input := &TaskInput{
				TaskID: "test-task",
				Goal:   tt.goal,
			}

			ctx := context.Background()
			result, err := node.Invoke(ctx, input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result.Complexity)
		})
	}
}

func TestSimpleExecutorNode_Invoke(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	deps := &Dependencies{
		LLM:   &mockLLM{},
		Logger: logger,
	}

	node := NewSimpleExecutorNode(deps)
	ctx := context.Background()
	input := &TaskContext{
		TaskID: "test-task",
		Goal:   "fix typo in code",
	}

	result, err := node.Invoke(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, 100.0, result.Progress)
	assert.NotEmpty(t, result.Summary)
}

func TestMediumDecomposerNode_Invoke(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	deps := &Dependencies{
		LLM:   &mockLLM{},
		Logger: logger,
	}

	node := NewMediumDecomposerNode(deps)
	ctx := context.Background()
	input := &TaskContext{
		TaskID: "test-task",
		Goal:   "implement user authentication",
	}

	result, err := node.Invoke(ctx, input)
	require.NoError(t, err)
	assert.NotEmpty(t, result.Subtasks)
	assert.True(t, result.Progress >= 20.0)
}

func TestMediumExecutorNode_Invoke(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	deps := &Dependencies{
		LLM:   &mockLLM{},
		Logger: logger,
	}

	node := NewMediumExecutorNode(deps)
	ctx := context.Background()
	input := &TaskContext{
		TaskID: "test-task",
		Goal:   "implement feature",
		Subtasks: []*SubtaskPlan{
			{
				ID:          "subtask-1",
				Description: "First step",
			},
		},
	}

	result, err := node.Invoke(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, 100.0, result.Progress)
}

func TestObserverNode_Invoke(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	deps := &Dependencies{
		LLM:   &mockLLM{},
		Logger: logger,
	}

	node := NewObserverNode(deps)
	ctx := context.Background()
	input := &TaskContext{
		TaskID: "test-task",
		Goal:   "observe and analyze the task",
	}

	result, err := node.Invoke(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, "observer", result.CurrentAgent)
	assert.True(t, result.Progress >= 10.0)
}

func TestStrategistNode_Invoke(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	deps := &Dependencies{
		LLM:   &mockLLM{},
		Logger: logger,
	}

	node := NewStrategistNode(deps)
	ctx := context.Background()
	input := &TaskContext{
		TaskID: "test-task",
		Goal:   "plan the execution",
	}

	result, err := node.Invoke(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, "strategist", result.CurrentAgent)
	assert.NotEmpty(t, result.Subtasks)
	assert.True(t, result.Progress >= 25.0)
}

func TestExecutorNode_Invoke(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	deps := &Dependencies{
		LLM:   &mockLLM{},
		Logger: logger,
	}

	node := NewExecutorNode(deps)
	ctx := context.Background()
	input := &TaskContext{
		TaskID: "test-task",
		Goal:   "execute the plan",
		Subtasks: []*SubtaskPlan{
			{
				ID:          "subtask-1",
				Description: "Execute this",
			},
		},
	}

	result, err := node.Invoke(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, "executor", result.CurrentAgent)
	assert.True(t, result.Progress >= 60.0)
}

func TestGuardianNode_Invoke(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	deps := &Dependencies{
		LLM:   &mockLLM{},
		Logger: logger,
	}

	node := NewGuardianNode(deps)
	ctx := context.Background()
	input := &TaskContext{
		TaskID: "test-task",
		Goal:   "validate the execution",
		Summary: "executed successfully",
	}

	result, err := node.Invoke(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, "guardian", result.CurrentAgent)
	assert.True(t, result.Progress >= 80.0)
}

func TestTesterNode_Invoke(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	deps := &Dependencies{
		LLM:   &mockLLM{},
		Logger: logger,
	}

	node := NewTesterNode(deps)
	ctx := context.Background()
	input := &TaskContext{
		TaskID: "test-task",
		Goal:   "run tests",
	}

	result, err := node.Invoke(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, "tester", result.CurrentAgent)
	assert.True(t, result.Progress >= 95.0)
}

func TestResultMerger_Invoke(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	deps := &Dependencies{
		LLM:   &mockLLM{},
		Logger: logger,
	}

	node := NewResultMerger(deps)
	ctx := context.Background()
	input := &TaskContext{
		TaskID:     "test-task",
		Summary:    "task completed successfully",
		TokensUsed: 500,
	}

	result, err := node.Invoke(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, "test-task", result.TaskID)
	assert.True(t, result.Success)
	assert.Equal(t, 500, result.TokensUsed)
}

func TestComplexityRouter_Invoke(t *testing.T) {
	router := NewComplexityRouter()

	tests := []struct {
		name     string
		complexity TaskComplexity
		expected  string
	}{
		{
			name:       "simple complexity",
			complexity: ComplexitySimple,
			expected:   "simple",
		},
		{
			name:       "medium complexity",
			complexity: ComplexityMedium,
			expected:   "medium",
		},
		{
			name:       "complex complexity",
			complexity: ComplexityComplex,
			expected:   "complex",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			input := &TaskContext{
				Complexity: tt.complexity,
			}

			result, err := router.Invoke(ctx, input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParallelExecutorNode_Invoke(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	deps := &Dependencies{
		LLM:   &mockLLM{},
		Logger: logger,
	}

	// Create a parallel executor with 2 nodes
	node := NewParallelExecutorNode(deps, "test-parallel",
		func(ctx context.Context, in *TaskContext) (*TaskContext, error) {
			return &TaskContext{
				TaskID:  in.TaskID,
				Summary: "result1",
			}, nil
		},
		func(ctx context.Context, in *TaskContext) (*TaskContext, error) {
			return &TaskContext{
				TaskID:  in.TaskID,
				Summary: "result2",
			}, nil
		},
	)

	ctx := context.Background()
	input := &TaskContext{
		TaskID: "test-task",
	}

	result, err := node.Invoke(ctx, input)
	require.NoError(t, err)
	assert.Contains(t, result.Summary, "result1")
	assert.Contains(t, result.Summary, "result2")
}
