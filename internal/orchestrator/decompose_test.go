// Package orchestrator implements L4 orchestration: task scheduling, agent session
// management, and event-driven workflow coordination.
package orchestrator

import (
	"context"
	"sync"
	"testing"

	"github.com/cloud-agent-platform/cap/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskDecomposer_DecomposeTask(t *testing.T) {
	subtaskRepo := newMockSubtaskRepository()
	decomposer := NewTaskDecomposer(subtaskRepo)

	task := domain.NewTask("task-decomp-1", "Implement login feature and add tests", "https://github.com/test/repo", "main", "client-1")

	result, err := decomposer.DecomposeTask(context.Background(), task, DefaultDecomposeOptions())
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Greater(t, len(result.Subtasks), 0)
	assert.NotEmpty(t, result.Strategy)
	assert.NotNil(t, result.PriorityMap)
	assert.NotNil(t, result.DependencyGraph)
}

func TestTaskDecomposer_DecomposeTaskNil(t *testing.T) {
	decomposer := NewTaskDecomposer(nil)

	result, err := decomposer.DecomposeTask(context.Background(), nil, DefaultDecomposeOptions())
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestTaskDecomposer_StrategySelection(t *testing.T) {
	tests := []struct {
		name     string
		goal     string
		expected DecompositionStrategy
	}{
		{
			name:     "file strategy for modify goal",
			goal:     "Modify the user file to add validation",
			expected: StrategyByFile,
		},
		{
			name:     "module strategy for service goal",
			goal:     "Refactor the auth module",
			expected: StrategyByModule,
		},
		{
			name:     "feature strategy for implement goal",
			goal:     "Implement user registration feature",
			expected: StrategyByFeature,
		},
	}

	decomposer := NewTaskDecomposer(nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := domain.NewTask("task-"+tt.name, tt.goal, "https://github.com/test/repo", "main", "client-1")
			opts := DefaultDecomposeOptions()
			opts.Strategy = StrategyAuto

			// Use the internal determineStrategy method via decomposition
			result, err := decomposer.DecomposeTask(context.Background(), task, opts)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result.Strategy)
		})
	}
}

func TestTaskDecomposer_PriorityAssignment(t *testing.T) {
	decomposer := NewTaskDecomposer(nil)

	task := domain.NewTask("task-priority", "Research architecture, then implement API, then write tests, then review", "https://github.com/test/repo", "main", "client-1")

	result, err := decomposer.DecomposeTask(context.Background(), task, DefaultDecomposeOptions())
	require.NoError(t, err)

	// Verify that all subtasks have priorities assigned
	for _, st := range result.Subtasks {
		pri, exists := result.PriorityMap[st.ID]
		assert.True(t, exists, "Subtask %s should have priority", st.ID)
		assert.Greater(t, pri, 0, "Priority should be positive")
		assert.LessOrEqual(t, pri, 9, "Priority should be at most 9")
	}
}

func TestTaskDecomposer_DependencyAnalysis(t *testing.T) {
	decomposer := NewTaskDecomposer(nil)

	task := domain.NewTask("task-deps", "Analyze requirements, implement feature, write tests, review code", "https://github.com/test/repo", "main", "client-1")

	result, err := decomposer.DecomposeTask(context.Background(), task, DefaultDecomposeOptions())
	require.NoError(t, err)

	// Verify dependency graph exists for all subtasks
	for _, st := range result.Subtasks {
		_, exists := result.DependencyGraph[st.ID]
		assert.True(t, exists, "Subtask %s should have dependency graph entry", st.ID)
	}

	// Verify execution order is valid
	assert.Equal(t, len(result.Subtasks), len(result.ExecutionOrder), "Execution order should include all subtasks")
}

func TestTaskDecomposer_ExecutionOrder(t *testing.T) {
	decomposer := NewTaskDecomposer(nil)

	task := domain.NewTask("task-order", "Step 1: analyze, Step 2: implement, Step 3: test, Step 4: review", "https://github.com/test/repo", "main", "client-1")

	result, err := decomposer.DecomposeTask(context.Background(), task, DefaultDecomposeOptions())
	require.NoError(t, err)

	// Verify execution order respects dependencies
	pos := make(map[string]int)
	for i, id := range result.ExecutionOrder {
		pos[id] = i
	}

	for id, deps := range result.DependencyGraph {
		for _, dep := range deps {
			assert.Less(t, pos[dep], pos[id], "Dependency %s should come before dependent %s", dep, id)
		}
	}
}

func TestTaskDecomposer_MaxSubtasks(t *testing.T) {
	decomposer := NewTaskDecomposer(nil)

	// Create a task with many components
	task := domain.NewTask("task-many", "Item1, Item2, Item3, Item4, Item5, Item6, Item7, Item8, Item9, Item10, Item11, Item12", "https://github.com/test/repo", "main", "client-1")

	opts := DefaultDecomposeOptions()
	opts.MaxSubtasks = 5

	result, err := decomposer.DecomposeTask(context.Background(), task, opts)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(result.Subtasks), 5, "Should limit subtasks to MaxSubtasks")
}

func TestTaskDecomposer_ComponentClassification(t *testing.T) {
	tests := []struct {
		segment  string
		expected domain.SubtaskType
	}{
		{"write tests for login", domain.SubtaskTypeTesting},
		{"verify the results", domain.SubtaskTypeTesting},
		{"review the code", domain.SubtaskTypeReview},
		{"analyze the requirements", domain.SubtaskTypeAnalysis},
		{"research best practices", domain.SubtaskTypeResearch},
		{"implement the feature", domain.SubtaskTypeCoding},
	}

	for _, tt := range tests {
		t.Run(tt.segment, func(t *testing.T) {
			result := classifyComponent(tt.segment)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTaskDecomposer_SaveSubtasks(t *testing.T) {
	subtaskRepo := newMockSubtaskRepository()
	decomposer := NewTaskDecomposer(subtaskRepo)

	task := domain.NewTask("task-save", "Implement feature A and feature B", "https://github.com/test/repo", "main", "client-1")

	result, err := decomposer.DecomposeTask(context.Background(), task, DefaultDecomposeOptions())
	require.NoError(t, err)

	err = decomposer.SaveSubtasks(context.Background(), result.Subtasks)
	require.NoError(t, err)

	// Verify subtasks were saved
	saved, err := subtaskRepo.ListByTaskID(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, len(result.Subtasks), len(saved))
}

func TestUniqueStrings(t *testing.T) {
	tests := []struct {
		input    []string
		expected []string
	}{
		{[]string{"a", "b", "a", "c"}, []string{"a", "b", "c"}},
		{[]string{"x", "x", "x"}, []string{"x"}},
		{[]string{}, []string{}},
		{[]string{"only"}, []string{"only"}},
	}

	for _, tt := range tests {
		result := uniqueStrings(tt.input)
		assert.ElementsMatch(t, tt.expected, result)
	}
}

func TestSplitGoalSegments(t *testing.T) {
	tests := []struct {
		goal     string
		expected int // minimum number of segments
	}{
		{"Implement login and add tests", 2},
		{"Step 1: analyze. Step 2: implement.", 2},
		{"Simple task", 1},
	}

	for _, tt := range tests {
		result := splitGoalSegments(tt.goal)
		assert.GreaterOrEqual(t, len(result), tt.expected)
	}
}

func TestExtractModuleName(t *testing.T) {
	tests := []struct {
		desc     string
		expected string
	}{
		{"work on module auth", "auth"},
		{"update package user", "user"},
		{"modify service api", "api"},
		{"change component header", "header"},
		{"regular description", "regular"},
	}

	for _, tt := range tests {
		result := extractModuleName(tt.desc)
		assert.Equal(t, tt.expected, result)
	}
}

func TestDefaultDecomposeOptions(t *testing.T) {
	opts := DefaultDecomposeOptions()

	assert.Equal(t, StrategyAuto, opts.Strategy)
	assert.Equal(t, 20, opts.MaxSubtasks)
	assert.True(t, opts.IncludeTests)
	assert.Equal(t, 3, opts.ParallelThreshold)
}

// Integration test with mock repository
func TestTaskDecomposer_Integration(t *testing.T) {
	taskRepo := newMockTaskRepository()
	subtaskRepo := newMockSubtaskRepository()

	decomposer := NewTaskDecomposer(subtaskRepo)

	// Create a complex task
	task := domain.NewTask("task-integration", "Research best practices, implement authentication module, add unit tests, perform code review", "https://github.com/test/repo", "main", "client-1")
	taskRepo.Create(context.Background(), task)

	// Decompose the task
	result, err := decomposer.DecomposeTask(context.Background(), task, DefaultDecomposeOptions())
	require.NoError(t, err)
	require.NotEmpty(t, result.Subtasks)

	// Save subtasks
	err = decomposer.SaveSubtasks(context.Background(), result.Subtasks)
	require.NoError(t, err)

	// Create orchestrator and start task (skip actual orchestrator test since it requires a real logger)
	// Just verify decomposition worked
	assert.Greater(t, len(result.Subtasks), 0)
	assert.Equal(t, len(result.Subtasks), len(result.ExecutionOrder))
}

// Concurrent decomposition test
func TestTaskDecomposer_Concurrent(t *testing.T) {
	decomposer := NewTaskDecomposer(nil)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			task := domain.NewTask("task-concurrent-"+string(rune('0'+id)), "Feature work", "https://github.com/test/repo", "main", "client-1")
			_, err := decomposer.DecomposeTask(context.Background(), task, DefaultDecomposeOptions())
			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()
}