// Package orchestrator implements Eino-based task orchestration graph tests.
package orchestrator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// =============================================================================
// ComplexityAnalyzer Tests
// =============================================================================

func TestComplexityAnalyzer_Analyze_SimpleTasks(t *testing.T) {
	analyzer := NewComplexityAnalyzer()

	tests := []struct {
		name     string
		goal     string
		expected string
	}{
		{
			name:     "fix typo",
			goal:     "fix typo in README",
			expected: "simple",
		},
		{
			name:     "update comment",
			goal:     "update comment in main.go",
			expected: "simple",
		},
		{
			name:     "rename variable",
			goal:     "rename variable foo to bar",
			expected: "simple",
		},
		{
			name:     "fix bug",
			goal:     "fix bug in the code",
			expected: "simple",
		},
		{
			name:     "add import",
			goal:     "add import for fmt package",
			expected: "simple",
		},
		{
			name:     "remove debug",
			goal:     "remove debug print statement",
			expected: "simple",
		},
		{
			name:     "single file change",
			goal:     "change string value in config.yaml",
			expected: "simple",
		},
		{
			name:     "quick fix",
			goal:     "apply a quick fix for the issue",
			expected: "simple",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := &TaskInput{
				TaskID: "test-task",
				Goal:   tt.goal,
			}
			result := analyzer.Analyze(context.Background(), input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestComplexityAnalyzer_Analyze_MediumTasks(t *testing.T) {
	analyzer := NewComplexityAnalyzer()

	tests := []struct {
		name     string
		goal     string
		expected string
	}{
		{
			name:     "implement feature",
			goal:     "implement user login feature with JWT",
			expected: "medium",
		},
		{
			name:     "add functionality",
			goal:     "add password reset functionality to the auth module",
			expected: "medium",
		},
		{
			name:     "create module",
			goal:     "create payment processing module",
			expected: "medium",
		},
		{
			name:     "build component",
			goal:     "build user profile component with avatar upload",
			expected: "medium",
		},
		{
			name:     "multiple files",
			goal:     "update database queries across multiple files",
			expected: "medium",
		},
		{
			name:     "api endpoint",
			goal:     "add new REST API endpoint for user preferences",
			expected: "medium",
		},
		{
			name:     "add feature",
			goal:     "add new feature to the dashboard",
			expected: "medium",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := &TaskInput{
				TaskID: "test-task",
				Goal:   tt.goal,
			}
			result := analyzer.Analyze(context.Background(), input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestComplexityAnalyzer_Analyze_ComplexTasks(t *testing.T) {
	analyzer := NewComplexityAnalyzer()

	// Note: Complex tasks require > 2000 tokens OR specific complex keywords
	// For testing complex keywords without high token count, we use the keyword match
	tests := []struct {
		name     string
		goal     string
		expected string
	}{
		{
			name:     "architecture refactor - keyword match",
			goal:     "refactor architecture to use microservices design pattern across distributed systems",
			expected: "complex",
		},
		{
			name:     "design pattern - keyword match",
			goal:     "implement observer design pattern for event handling in the system",
			expected: "complex",
		},
		{
			name:     "performance optimization - keyword match",
			goal:     "performance optimization for distributed caching system with multi-module architecture",
			expected: "complex",
		},
		{
			name:     "security audit - keyword match",
			goal:     "security audit for authentication system with code review",
			expected: "complex",
		},
		{
			name:     "system design - keyword match",
			goal:     "system design for new design system architecture",
			expected: "complex",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := &TaskInput{
				TaskID: "test-task",
				Goal:   tt.goal,
			}
			result := analyzer.Analyze(context.Background(), input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestComplexityAnalyzer_Analyze_DefaultComplexity(t *testing.T) {
	analyzer := NewComplexityAnalyzer()

	// Test with ambiguous input that doesn't match any specific pattern
	input := &TaskInput{
		TaskID: "test-task",
		Goal:   "do something with the codebase",
	}
	result := analyzer.Analyze(context.Background(), input)
	// Should default to medium
	assert.Equal(t, "medium", result)
}

func TestComplexityAnalyzer_AnalyzeWithComplexity(t *testing.T) {
	analyzer := NewComplexityAnalyzer()

	tests := []struct {
		complexity TaskComplexity
		expected   string
	}{
		{ComplexitySimple, "simple"},
		{ComplexityMedium, "medium"},
		{ComplexityComplex, "complex"},
	}

	for _, tt := range tests {
		t.Run(string(tt.complexity), func(t *testing.T) {
			result := analyzer.AnalyzeWithComplexity(context.Background(), tt.complexity)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestComplexityAnalyzer_WithConfig(t *testing.T) {
	cfg := &RouterConfig{
		Rules: []RoutingRule{
			{
				Name:      "custom_simple",
				MaxTokens: 100,
				Keywords:  []string{"quick", "small"},
			},
		},
		DefaultComplexity: ComplexityMedium,
		TokenEstimateFunc: estimateTokens,
	}

	analyzer := NewComplexityAnalyzerWithConfig(cfg)

	input := &TaskInput{
		TaskID: "test-task",
		Goal:   "quick change",
	}
	result := analyzer.Analyze(context.Background(), input)
	assert.Equal(t, "custom_simple", result)
}

func TestComplexityAnalyzer_UpdateRule(t *testing.T) {
	analyzer := NewComplexityAnalyzer()

	// Update the simple rule to include a new keyword
	analyzer.UpdateRule("simple", RoutingRule{
		Name:      "simple",
		MaxTokens: 500,
		Keywords:  []string{"fix typo", "tiny"},
	})

	input := &TaskInput{
		TaskID: "test-task",
		Goal:   "make a tiny adjustment",
	}
	result := analyzer.Analyze(context.Background(), input)
	assert.Equal(t, "simple", result)
}

// =============================================================================
// TaskGraph Tests
// =============================================================================

func TestNewTaskGraph(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	deps := &Dependencies{
		Logger: logger,
	}
	cfg := DefaultGraphConfig()

	tg, err := NewTaskGraph(deps, cfg)
	require.NoError(t, err)
	assert.NotNil(t, tg)
	assert.NotNil(t, tg.graph)
}

func TestNewTaskGraph_NilDeps(t *testing.T) {
	_, err := NewTaskGraph(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dependencies cannot be nil")
}

func TestTaskGraph_Compile(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	deps := &Dependencies{
		Logger: logger,
	}

	tg, err := NewTaskGraph(deps, nil)
	require.NoError(t, err)

	ctx := context.Background()
	runnable, err := tg.Compile(ctx)
	require.NoError(t, err)
	assert.NotNil(t, runnable)
}

func TestTaskGraph_GetConfig(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	deps := &Dependencies{
		Logger: logger,
	}
	cfg := DefaultGraphConfig()

	tg, err := NewTaskGraph(deps, cfg)
	require.NoError(t, err)

	gotCfg := tg.GetConfig()
	assert.NotNil(t, gotCfg)
	assert.Equal(t, cfg, gotCfg)
}

func TestTaskGraph_GetAnalyzer(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	deps := &Dependencies{
		Logger: logger,
	}

	tg, err := NewTaskGraph(deps, nil)
	require.NoError(t, err)

	analyzer := tg.GetAnalyzer()
	assert.NotNil(t, analyzer)
}

// =============================================================================
// NodeRegistry Tests
// =============================================================================

func TestNodeRegistry_RegisterAndGet(t *testing.T) {
	registry := NewNodeRegistry()

	factory := func(deps *Dependencies) (interface{}, error) {
		return NewAnalyzerNode(deps), nil
	}
	registry.RegisterNode("analyzer", factory)

	gotFactory, ok := registry.GetNode("analyzer")
	assert.True(t, ok)
	assert.NotNil(t, gotFactory)
}

func TestNodeRegistry_GetNode_NotFound(t *testing.T) {
	registry := NewNodeRegistry()

	_, ok := registry.GetNode("nonexistent")
	assert.False(t, ok)
}

func TestNodeRegistry_ListNodes(t *testing.T) {
	registry := NewNodeRegistry()

	registry.RegisterNode("node1", func(deps *Dependencies) (interface{}, error) { return nil, nil })
	registry.RegisterNode("node2", func(deps *Dependencies) (interface{}, error) { return nil, nil })

	nodes := registry.ListNodes()
	assert.Len(t, nodes, 2)
	assert.Contains(t, nodes, "node1")
	assert.Contains(t, nodes, "node2")
}

// =============================================================================
// RouterConfig Tests
// =============================================================================

func TestDefaultRouterConfig(t *testing.T) {
	cfg := DefaultRouterConfig()
	assert.NotNil(t, cfg)
	assert.NotEmpty(t, cfg.Rules)
	assert.Equal(t, ComplexityMedium, cfg.DefaultComplexity)
	assert.NotNil(t, cfg.TokenEstimateFunc)
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		text     string
		expected int
	}{
		{"hello", 1},
		{"hello world", 2},
		{"The quick brown fox jumps over the lazy dog", 12},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			result := estimateTokens(tt.text)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// =============================================================================
// Integration-style Tests (3-path routing)
// =============================================================================

func TestTaskGraph_ThreePathRouting(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	deps := &Dependencies{
		Logger: logger,
	}

	tg, err := NewTaskGraph(deps, nil)
	require.NoError(t, err)

	analyzer := tg.GetAnalyzer()
	require.NotNil(t, analyzer)

	tests := []struct {
		name     string
		goal     string
		expected string
	}{
		// Simple path tests
		{
			name:     "simple_fix_typo",
			goal:     "fix typo",
			expected: "simple",
		},
		{
			name:     "simple_update_comment",
			goal:     "update comment",
			expected: "simple",
		},
		{
			name:     "simple_fix_bug",
			goal:     "fix bug in authentication",
			expected: "simple",
		},

		// Medium path tests
		{
			name:     "medium_implement_feature",
			goal:     "implement feature",
			expected: "medium",
		},
		{
			name:     "medium_create_module",
			goal:     "create module",
			expected: "medium",
		},
		{
			name:     "medium_api_endpoint",
			goal:     "add api endpoint",
			expected: "medium",
		},

		// Complex path tests (using goals with complex keywords)
		{
			name:     "complex_refactor_architecture",
			goal:     "refactor architecture to use microservices design pattern",
			expected: "complex",
		},
		{
			name:     "complex_design_pattern",
			goal:     "implement observer design pattern",
			expected: "complex",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := &TaskInput{
				TaskID: "test-task",
				Goal:   tt.goal,
			}
			result := analyzer.Analyze(context.Background(), input)
			assert.Equal(t, tt.expected, result, "Goal: %s", tt.goal)
		})
	}
}
