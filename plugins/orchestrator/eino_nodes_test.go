// Package orchestrator implements Eino-based task orchestration graph tests.
package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cloud-agent-platform/cap/internal/agent/react"
	"github.com/cloud-agent-platform/cap/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// =============================================================================
// Test Fixtures
// =============================================================================

func newTestLogger(t *testing.T) *zap.Logger {
	logger, _ := zap.NewDevelopment()
	t.Cleanup(func() { logger.Sync() })
	return logger
}

func newTestDependencies(t *testing.T) *Dependencies {
	return &Dependencies{
		Logger: newTestLogger(t),
	}
}

func newTestTaskContext() *TaskContext {
	return &TaskContext{
		TaskID:      "test-task-001",
		Goal:        "implement user login feature",
		Constraints: []string{"must use JWT", "support refresh tokens"},
		RepoURL:     "https://github.com/test/repo",
		BaseBranch:  "main",
		Complexity:  ComplexityMedium,
	}
}

// MockLLM is a mock LLM implementation for testing.
type MockLLM struct {
	GenerateFunc func(ctx context.Context, messages []*react.Message, opts *react.GenerateOptions) (*react.GenerateResult, error)
}

func (m *MockLLM) Generate(ctx context.Context, messages []*react.Message, opts *react.GenerateOptions) (*react.GenerateResult, error) {
	if m.GenerateFunc != nil {
		return m.GenerateFunc(ctx, messages, opts)
	}
	return &react.GenerateResult{
		Content:     "Mock response",
		TotalTokens: 100,
	}, nil
}

func (m *MockLLM) GenerateStream(ctx context.Context, messages []*react.Message, opts *react.GenerateOptions, callback func(chunk string) error) (*react.GenerateResult, error) {
	return m.Generate(ctx, messages, opts)
}

// =============================================================================
// EinoAnalyzerNode Tests
// =============================================================================

func TestEinoAnalyzerNode_Name(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoAnalyzerNode(deps)
	assert.Equal(t, "analyzer", node.Name())
}

func TestEinoAnalyzerNode_Description(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoAnalyzerNode(deps)
	assert.Contains(t, node.Description(), "Analyzes task")
}

func TestEinoAnalyzerNode_Execute_SimpleTasks(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoAnalyzerNode(deps)

	tests := []struct {
		name     string
		goal     string
		expected TaskComplexity
	}{
		{"fix typo", "fix typo in README", ComplexitySimple},
		{"update comment", "update comment in main.go", ComplexitySimple},
		{"rename variable", "rename variable foo to bar", ComplexitySimple},
		{"fix bug", "fix bug in the code", ComplexitySimple},
		{"quick fix", "apply a quick fix", ComplexitySimple},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := &TaskContext{
				TaskID: "test-task",
				Goal:   tt.goal,
			}
			result, err := node.Execute(context.Background(), input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result.Complexity)
		})
	}
}

func TestEinoAnalyzerNode_Execute_MediumTasks(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoAnalyzerNode(deps)

	tests := []struct {
		name     string
		goal     string
		expected TaskComplexity
	}{
		{"implement feature", "implement user login feature", ComplexityMedium},
		{"add functionality", "add password reset functionality", ComplexityMedium},
		{"create module", "create payment processing module", ComplexityMedium},
		{"build component", "build user profile component", ComplexityMedium},
		{"api endpoint", "add new REST API endpoint", ComplexityMedium},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := &TaskContext{
				TaskID: "test-task",
				Goal:   tt.goal,
			}
			result, err := node.Execute(context.Background(), input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result.Complexity)
		})
	}
}

func TestEinoAnalyzerNode_Execute_ComplexTasks(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoAnalyzerNode(deps)

	tests := []struct {
		name     string
		goal     string
		expected TaskComplexity
	}{
		{"architecture refactor", "refactor architecture to microservices", ComplexityComplex},
		{"design pattern", "implement observer design pattern", ComplexityComplex},
		{"performance optimization", "performance optimization for distributed caching", ComplexityComplex},
		{"security audit", "security audit for authentication system", ComplexityComplex},
		{"system design", "system design for new architecture", ComplexityComplex},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := &TaskContext{
				TaskID: "test-task",
				Goal:   tt.goal,
			}
			result, err := node.Execute(context.Background(), input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result.Complexity)
		})
	}
}

func TestEinoAnalyzerNode_Execute_NilInput(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoAnalyzerNode(deps)

	_, err := node.Execute(context.Background(), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "input is nil")
}

func TestEinoAnalyzerNode_Execute_PreservesFields(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoAnalyzerNode(deps)

	input := &TaskContext{
		TaskID:      "test-task-123",
		Goal:        "implement feature",
		Constraints: []string{"constraint1", "constraint2"},
		RepoURL:     "https://github.com/test/repo",
		BaseBranch:  "main",
	}

	result, err := node.Execute(context.Background(), input)
	require.NoError(t, err)

	assert.Equal(t, input.TaskID, result.TaskID)
	assert.Equal(t, input.Goal, result.Goal)
	assert.Equal(t, input.Constraints, result.Constraints)
	assert.Equal(t, input.RepoURL, result.RepoURL)
	assert.Equal(t, input.BaseBranch, result.BaseBranch)
}

// =============================================================================
// EinoRouterNode Tests
// =============================================================================

func TestEinoRouterNode_Name(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoRouterNode(deps)
	assert.Equal(t, "router", node.Name())
}

func TestEinoRouterNode_Description(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoRouterNode(deps)
	assert.Contains(t, node.Description(), "Routes")
}

func TestEinoRouterNode_Execute_SimpleComplexity(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoRouterNode(deps)

	input := &TaskContext{
		TaskID:    "test-task",
		Complexity: ComplexitySimple,
	}

	result, err := node.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, ComplexitySimple, result.Complexity)
}

func TestEinoRouterNode_Execute_MediumComplexity(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoRouterNode(deps)

	input := &TaskContext{
		TaskID:    "test-task",
		Complexity: ComplexityMedium,
	}

	result, err := node.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, ComplexityMedium, result.Complexity)
}

func TestEinoRouterNode_Execute_ComplexComplexity(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoRouterNode(deps)

	input := &TaskContext{
		TaskID:    "test-task",
		Complexity: ComplexityComplex,
	}

	result, err := node.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, ComplexityComplex, result.Complexity)
}

func TestEinoRouterNode_Execute_NilInput(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoRouterNode(deps)

	_, err := node.Execute(context.Background(), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "input is nil")
}

func TestEinoRouterNode_GetRoute(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoRouterNode(deps)

	tests := []struct {
		complexity TaskComplexity
		expected  string
	}{
		{ComplexitySimple, "simple"},
		{ComplexityMedium, "medium"},
		{ComplexityComplex, "complex"},
	}

	for _, tt := range tests {
		t.Run(string(tt.complexity), func(t *testing.T) {
			input := &TaskContext{Complexity: tt.complexity}
			route, err := node.GetRoute(context.Background(), input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, route)
		})
	}
}

// =============================================================================
// EinoSimpleExecNode Tests
// =============================================================================

func TestEinoSimpleExecNode_Name(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoSimpleExecNode(deps)
	assert.Equal(t, "simple_exec", node.Name())
}

func TestEinoSimpleExecNode_Description(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoSimpleExecNode(deps)
	assert.Contains(t, node.Description(), "Executes simple tasks")
}

func TestEinoSimpleExecNode_Execute_WithLLM(t *testing.T) {
	deps := newTestDependencies(t)
	mockLLM := &MockLLM{
		GenerateFunc: func(ctx context.Context, messages []*react.Message, opts *react.GenerateOptions) (*react.GenerateResult, error) {
			assert.NotEmpty(t, messages)
			assert.Contains(t, messages[1].Content, "fix typo")
			return &react.GenerateResult{
				Content:     "Fixed the typo in README.md",
				TotalTokens: 50,
			}, nil
		},
	}
	deps.LLM = mockLLM

	node := NewEinoSimpleExecNode(deps)
	input := &TaskContext{
		TaskID: "test-task",
		Goal:   "fix typo in README",
	}

	result, err := node.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, 100.0, result.Progress)
	assert.Equal(t, 50, result.TokensUsed)
}

func TestEinoSimpleExecNode_Execute_WithoutLLM(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoSimpleExecNode(deps)

	input := &TaskContext{
		TaskID: "test-task",
		Goal:   "fix typo",
	}

	result, err := node.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, 100.0, result.Progress)
}

func TestEinoSimpleExecNode_Execute_NilInput(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoSimpleExecNode(deps)

	_, err := node.Execute(context.Background(), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "input is nil")
}

func TestEinoSimpleExecNode_Execute_LLMError(t *testing.T) {
	deps := newTestDependencies(t)
	expectedErr := errors.New("LLM error")
	mockLLM := &MockLLM{
		GenerateFunc: func(ctx context.Context, messages []*react.Message, opts *react.GenerateOptions) (*react.GenerateResult, error) {
			return nil, expectedErr
		},
	}
	deps.LLM = mockLLM

	node := NewEinoSimpleExecNode(deps)
	input := &TaskContext{
		TaskID: "test-task",
		Goal:   "fix typo",
	}

	_, err := node.Execute(context.Background(), input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "LLM error")
}

// =============================================================================
// EinoReActNode Tests
// =============================================================================

func TestEinoReActNode_Name(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoReActNode(deps)
	assert.Equal(t, "react", node.Name())
}

func TestEinoReActNode_Description(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoReActNode(deps)
	assert.Contains(t, node.Description(), "ReAct loop")
}

func TestEinoReActNode_Execute_WithLLM(t *testing.T) {
	deps := newTestDependencies(t)
	mockLLM := &MockLLM{
		GenerateFunc: func(ctx context.Context, messages []*react.Message, opts *react.GenerateOptions) (*react.GenerateResult, error) {
			return &react.GenerateResult{
				Content:     "Thought: I need to implement this feature\nAction: final_answer\nActionArgs: Task completed successfully",
				TotalTokens: 100,
			}, nil
		},
	}
	deps.LLM = mockLLM

	node := NewEinoReActNode(deps)
	input := &TaskContext{
		TaskID: "test-task",
		Goal:   "implement feature",
	}

	result, err := node.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, 100.0, result.Progress)
	assert.Equal(t, 100, result.TokensUsed)
}

func TestEinoReActNode_Execute_WithoutLLM(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoReActNode(deps)

	input := &TaskContext{
		TaskID: "test-task",
		Goal:   "implement feature",
	}

	result, err := node.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, 100.0, result.Progress)
}

func TestEinoReActNode_Execute_NilInput(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoReActNode(deps)

	_, err := node.Execute(context.Background(), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "input is nil")
}

func TestEinoReActNode_Execute_MaxIterations(t *testing.T) {
	deps := newTestDependencies(t)
	callCount := 0
	mockLLM := &MockLLM{
		GenerateFunc: func(ctx context.Context, messages []*react.Message, opts *react.GenerateOptions) (*react.GenerateResult, error) {
			callCount++
			return &react.GenerateResult{
				Content:     "Thought: continuing analysis...",
				TotalTokens: 50,
			}, nil
		},
	}
	deps.LLM = mockLLM

	node := NewEinoReActNodeWithConfig(deps, 3, 0)
	input := &TaskContext{
		TaskID: "test-task",
		Goal:   "implement feature",
	}

	result, err := node.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, 100.0, result.Progress)
	assert.Equal(t, 3, callCount)
}

func TestEinoReActNode_Execute_LLMError(t *testing.T) {
	deps := newTestDependencies(t)
	expectedErr := errors.New("LLM connection failed")
	mockLLM := &MockLLM{
		GenerateFunc: func(ctx context.Context, messages []*react.Message, opts *react.GenerateOptions) (*react.GenerateResult, error) {
			return nil, expectedErr
		},
	}
	deps.LLM = mockLLM

	node := NewEinoReActNode(deps)
	input := &TaskContext{
		TaskID: "test-task",
		Goal:   "implement feature",
	}

	_, err := node.Execute(context.Background(), input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "LLM connection failed")
}

func TestEinoReActNode_WithConfig(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoReActNodeWithConfig(deps, 5, 10*time.Minute)

	assert.Equal(t, "react", node.Name())
	assert.Equal(t, 5, node.maxIterations)
	assert.Equal(t, 10*time.Minute, node.maxExecutionTime)
}

// =============================================================================
// EinoDecomposerNode Tests
// =============================================================================

func TestEinoDecomposerNode_Name(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoDecomposerNode(deps)
	assert.Equal(t, "decomposer", node.Name())
}

func TestEinoDecomposerNode_Description(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoDecomposerNode(deps)
	assert.Contains(t, node.Description(), "Decomposes")
}

func TestEinoDecomposerNode_Execute_APIFocus(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoDecomposerNode(deps)

	input := &TaskContext{
		TaskID: "test-task",
		Goal:   "implement REST API endpoint for user management",
	}

	result, err := node.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.NotEmpty(t, result.Subtasks)
	assert.GreaterOrEqual(t, len(result.Subtasks), 2)

	hasAnalysis := false
	for _, st := range result.Subtasks {
		if st.Type == domain.SubtaskTypeAnalysis {
			hasAnalysis = true
			break
		}
	}
	assert.True(t, hasAnalysis)
}

func TestEinoDecomposerNode_Execute_ModuleFocus(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoDecomposerNode(deps)

	input := &TaskContext{
		TaskID: "test-task",
		Goal:   "create payment processing module",
	}

	result, err := node.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.NotEmpty(t, result.Subtasks)
	assert.GreaterOrEqual(t, len(result.Subtasks), 3)

	hasCoding := false
	hasReview := false
	for _, st := range result.Subtasks {
		if st.Type == domain.SubtaskTypeCoding {
			hasCoding = true
		}
		if st.Type == domain.SubtaskTypeReview {
			hasReview = true
		}
	}
	assert.True(t, hasCoding)
	assert.True(t, hasReview)
}

func TestEinoDecomposerNode_Execute_DatabaseFocus(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoDecomposerNode(deps)

	input := &TaskContext{
		TaskID: "test-task",
		Goal:   "implement database migration for user schema",
	}

	result, err := node.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.NotEmpty(t, result.Subtasks)

	hasResearch := false
	for _, st := range result.Subtasks {
		if st.Type == domain.SubtaskTypeResearch {
			hasResearch = true
			break
		}
	}
	assert.True(t, hasResearch)
}

func TestEinoDecomposerNode_Execute_DefaultDecomposition(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoDecomposerNode(deps)

	input := &TaskContext{
		TaskID: "test-task",
		Goal:   "refactor the entire authentication system",
	}

	result, err := node.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.NotEmpty(t, result.Subtasks)
	assert.GreaterOrEqual(t, len(result.Subtasks), 4)

	typeCount := make(map[domain.SubtaskType]int)
	for _, st := range result.Subtasks {
		typeCount[st.Type]++
	}
	assert.GreaterOrEqual(t, typeCount[domain.SubtaskTypeResearch], 1)
	assert.GreaterOrEqual(t, typeCount[domain.SubtaskTypeAnalysis], 1)
	assert.GreaterOrEqual(t, typeCount[domain.SubtaskTypeCoding], 1)
}

func TestEinoDecomposerNode_Execute_NilInput(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoDecomposerNode(deps)

	_, err := node.Execute(context.Background(), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "input is nil")
}

func TestEinoDecomposerNode_Execute_ProgressUpdated(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoDecomposerNode(deps)

	input := &TaskContext{
		TaskID: "test-task",
		Goal:   "implement feature",
	}

	result, err := node.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, 20.0, result.Progress)
}

func TestEinoDecomposerNode_SubtaskDependencies(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoDecomposerNode(deps)

	input := &TaskContext{
		TaskID: "test-task",
		Goal:   "create new API module",
	}

	result, err := node.Execute(context.Background(), input)
	require.NoError(t, err)

	for i, st := range result.Subtasks {
		if len(st.Dependencies) > 0 && i > 0 {
			for _, depID := range st.Dependencies {
				found := false
				for j := 0; j < i; j++ {
					if result.Subtasks[j].ID == depID {
						found = true
						break
					}
				}
				assert.True(t, found, "Dependency %s should reference an earlier subtask", depID)
			}
		}
	}
}

// =============================================================================
// EinoAggregatorNode Tests
// =============================================================================

func TestEinoAggregatorNode_Name(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoAggregatorNode(deps)
	assert.Equal(t, "aggregator", node.Name())
}

func TestEinoAggregatorNode_Description(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoAggregatorNode(deps)
	assert.Contains(t, node.Description(), "Aggregates")
}

func TestEinoAggregatorNode_Execute_WithSubtasks(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoAggregatorNode(deps)

	input := &TaskContext{
		TaskID:   "test-task",
		Goal:     "implement feature",
		Progress: 80.0,
		Subtasks: []*SubtaskPlan{
			{
				ID:          "subtask-1",
				Description: "Design API",
				Type:        domain.SubtaskTypeAnalysis,
				Status:      "completed",
				Summary:     "API design completed",
			},
			{
				ID:          "subtask-2",
				Description: "Implement API",
				Type:        domain.SubtaskTypeCoding,
				Status:      "completed",
				Summary:     "API implementation completed",
			},
		},
	}

	result, err := node.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, result.Summary, "Task completed: implement feature")
	assert.Contains(t, result.Summary, "Subtasks (2)")
	assert.Contains(t, result.Summary, "Design API")
	assert.Contains(t, result.Summary, "Implement API")
}

func TestEinoAggregatorNode_Execute_WithoutSubtasks(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoAggregatorNode(deps)

	input := &TaskContext{
		TaskID:   "test-task",
		Goal:     "simple task",
		Summary:  "Simple task result",
		Progress: 100.0,
	}

	result, err := node.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, result.Summary, "Task completed: simple task")
	assert.Contains(t, result.Summary, "Simple task result")
}

func TestEinoAggregatorNode_Execute_NilInput(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoAggregatorNode(deps)

	_, err := node.Execute(context.Background(), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "input is nil")
}

func TestEinoAggregatorNode_Execute_EmptySubtasks(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoAggregatorNode(deps)

	input := &TaskContext{
		TaskID:   "test-task",
		Goal:     "task without subtasks",
		Progress: 100.0,
	}

	result, err := node.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, result.Summary, "Task completed: task without subtasks")
}

func TestEinoAggregatorNode_Execute_SubtaskWithEmptySummary(t *testing.T) {
	deps := newTestDependencies(t)
	node := NewEinoAggregatorNode(deps)

	input := &TaskContext{
		TaskID:   "test-task",
		Goal:     "task",
		Progress: 100.0,
		Subtasks: []*SubtaskPlan{
			{
				ID:          "subtask-1",
				Description: "Task without summary",
				Status:      "completed",
				Summary:     "",
			},
		},
	}

	result, err := node.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, result.Summary, "Task without summary")
	assert.NotContains(t, result.Summary, "Summary:")
}

// =============================================================================
// Node Interface Compliance Tests
// =============================================================================

func TestAllEinoNodesImplementNodeInterface(t *testing.T) {
	deps := newTestDependencies(t)

	nodes := []Node{
		NewEinoAnalyzerNode(deps),
		NewEinoRouterNode(deps),
		NewEinoSimpleExecNode(deps),
		NewEinoReActNode(deps),
		NewEinoDecomposerNode(deps),
		NewEinoAggregatorNode(deps),
	}

	for _, node := range nodes {
		t.Run(node.Name(), func(t *testing.T) {
			assert.NotEmpty(t, node.Name())
			assert.NotEmpty(t, node.Description())

			input := &TaskContext{
				TaskID:    "test-task",
				Goal:      "test goal",
				Complexity: ComplexityMedium,
			}
			result, err := node.Execute(context.Background(), input)
			require.NoError(t, err)
			assert.NotNil(t, result)
		})
	}
}

// =============================================================================
// Integration Tests
// =============================================================================

func TestEinoAnalyzerRouterIntegration(t *testing.T) {
	deps := newTestDependencies(t)

	analyzer := NewEinoAnalyzerNode(deps)
	router := NewEinoRouterNode(deps)

	tests := []struct {
		goal          string
		expectedRoute string
	}{
		{"fix typo in code", "simple"},
		{"implement user login feature", "medium"},
		{"refactor architecture to microservices", "complex"},
	}

	for _, tt := range tests {
		t.Run(tt.goal, func(t *testing.T) {
			input := &TaskContext{TaskID: "test", Goal: tt.goal}
			ctxOut, err := analyzer.Execute(context.Background(), input)
			require.NoError(t, err)

			route, err := router.GetRoute(context.Background(), ctxOut)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedRoute, route)
		})
	}
}

func TestEinoSimpleExecNode_ProgressCompletion(t *testing.T) {
	deps := newTestDependencies(t)
	mockLLM := &MockLLM{
		GenerateFunc: func(ctx context.Context, messages []*react.Message, opts *react.GenerateOptions) (*react.GenerateResult, error) {
			return &react.GenerateResult{
				Content:     "Completed",
				TotalTokens: 10,
			}, nil
		},
	}
	deps.LLM = mockLLM

	node := NewEinoSimpleExecNode(deps)
	input := &TaskContext{
		TaskID: "test-task",
		Goal:   "simple task",
	}

	result, err := node.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, 100.0, result.Progress)
}