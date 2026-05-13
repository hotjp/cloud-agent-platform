// Package orchestrator implements Eino-based task orchestration graph.
package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/cloud-agent-platform/cap/internal/agent/react"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// mockLLM is a mock LLM for testing (declared in full_graph_test.go).
// This file reuses the same mockLLM type.

func TestObserverAgent_NewObserverAgent(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	llm := &mockLLM{}
	tools := react.NewToolRegistry()

	agent := NewObserverAgent(llm, tools, logger)
	require.NotNil(t, agent)
	assert.NotNil(t, agent.base)
	assert.Equal(t, llm, agent.base.LLM)
}

func TestObserverAgent_Run(t *testing.T) {
	tests := []struct {
		name          string
		input         *AgentInput
		mockGenerate  func(ctx context.Context, messages []*react.Message, opts *react.GenerateOptions) (*react.GenerateResult, error)
		expectedErr   bool
		expectedStatus AgentStatus
	}{
		{
			name: "successful observation",
			input: &AgentInput{
				TaskID:      "test-task-1",
				Goal:        "Analyze the codebase structure",
				Constraints: []string{"constraint1"},
				RepoURL:     "https://github.com/test/repo",
				BaseBranch:  "main",
			},
			mockGenerate: func(ctx context.Context, messages []*react.Message, opts *react.GenerateOptions) (*react.GenerateResult, error) {
				return &react.GenerateResult{
					Content:     "Analysis complete. Found 5 key files.",
					TotalTokens: 150,
				}, nil
			},
			expectedErr:   false,
			expectedStatus: AgentStatusSuccess,
		},
		{
			name: "LLM error",
			input: &AgentInput{
				TaskID: "test-task-2",
				Goal:   "Analyze something",
			},
			mockGenerate: func(ctx context.Context, messages []*react.Message, opts *react.GenerateOptions) (*react.GenerateResult, error) {
				return nil, errors.New("LLM error")
			},
			expectedErr:   true,
			expectedStatus: AgentStatusFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, _ := zap.NewDevelopment()
			llm := &mockLLM{generateFunc: tt.mockGenerate}
			tools := react.NewToolRegistry()

			agent := NewObserverAgent(llm, tools, logger)
			output, err := agent.Run(context.Background(), tt.input)

			if tt.expectedErr {
				assert.Error(t, err)
				// When LLM fails, output may still be returned with failed status
				if output != nil {
					assert.Equal(t, AgentStatusFailed, output.Status)
				}
			} else {
				assert.NoError(t, err)
				require.NotNil(t, output)
				assert.Equal(t, tt.expectedStatus, output.Status)
				assert.NotEmpty(t, output.Summary)
			}
		})
	}
}

func TestStrategistAgent_NewStrategistAgent(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	llm := &mockLLM{}
	tools := react.NewToolRegistry()

	agent := NewStrategistAgent(llm, tools, logger)
	require.NotNil(t, agent)
	assert.NotNil(t, agent.base)
}

func TestStrategistAgent_Run(t *testing.T) {
	tests := []struct {
		name          string
		input         *AgentInput
		mockGenerate  func(ctx context.Context, messages []*react.Message, opts *react.GenerateOptions) (*react.GenerateResult, error)
		expectedErr   bool
		expectedStatus AgentStatus
	}{
		{
			name: "successful planning",
			input: &AgentInput{
				TaskID: "test-task-1",
				Goal:   "Implement new feature",
				Context: map[string]any{
					"observer_analysis": "Found key dependencies",
				},
			},
			mockGenerate: func(ctx context.Context, messages []*react.Message, opts *react.GenerateOptions) (*react.GenerateResult, error) {
				return &react.GenerateResult{
					Content:     "Plan: Step 1, Step 2, Step 3",
					TotalTokens: 200,
				}, nil
			},
			expectedErr:   false,
			expectedStatus: AgentStatusSuccess,
		},
		{
			name: "LLM error",
			input: &AgentInput{
				TaskID: "test-task-2",
				Goal:   "Plan something",
			},
			mockGenerate: func(ctx context.Context, messages []*react.Message, opts *react.GenerateOptions) (*react.GenerateResult, error) {
				return nil, errors.New("LLM error")
			},
			expectedErr:   true,
			expectedStatus: AgentStatusFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, _ := zap.NewDevelopment()
			llm := &mockLLM{generateFunc: tt.mockGenerate}
			tools := react.NewToolRegistry()

			agent := NewStrategistAgent(llm, tools, logger)
			output, err := agent.Run(context.Background(), tt.input)

			if tt.expectedErr {
				assert.Error(t, err)
				// When LLM fails, output may still be returned with failed status
				if output != nil {
					assert.Equal(t, AgentStatusFailed, output.Status)
				}
			} else {
				assert.NoError(t, err)
				require.NotNil(t, output)
				assert.Equal(t, tt.expectedStatus, output.Status)
			}
		})
	}
}

func TestExecutorAgent_NewExecutorAgent(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	llm := &mockLLM{}
	tools := react.NewToolRegistry()

	agent := NewExecutorAgent(llm, tools, logger)
	require.NotNil(t, agent)
	assert.NotNil(t, agent.base)
}

func TestExecutorAgent_Run(t *testing.T) {
	tests := []struct {
		name          string
		input         *AgentInput
		mockGenerate  func(ctx context.Context, messages []*react.Message, opts *react.GenerateOptions) (*react.GenerateResult, error)
		expectedErr   bool
		expectedStatus AgentStatus
	}{
		{
			name: "successful execution",
			input: &AgentInput{
				TaskID: "test-task-1",
				Goal:   "Implement feature X",
				Context: map[string]any{
					"strategist_plan": "Step 1: Create file, Step 2: Add tests",
				},
			},
			mockGenerate: func(ctx context.Context, messages []*react.Message, opts *react.GenerateOptions) (*react.GenerateResult, error) {
				return &react.GenerateResult{
					Content:     "Path: src/feature.go\nCode implementation complete",
					TotalTokens: 300,
				}, nil
			},
			expectedErr:   false,
			expectedStatus: AgentStatusSuccess,
		},
		{
			name: "LLM error",
			input: &AgentInput{
				TaskID: "test-task-2",
				Goal:   "Execute something",
			},
			mockGenerate: func(ctx context.Context, messages []*react.Message, opts *react.GenerateOptions) (*react.GenerateResult, error) {
				return nil, errors.New("LLM error")
			},
			expectedErr:   true,
			expectedStatus: AgentStatusFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, _ := zap.NewDevelopment()
			llm := &mockLLM{generateFunc: tt.mockGenerate}
			tools := react.NewToolRegistry()

			agent := NewExecutorAgent(llm, tools, logger)
			output, err := agent.Run(context.Background(), tt.input)

			if tt.expectedErr {
				assert.Error(t, err)
				// When LLM fails, output may still be returned with failed status
				if output != nil {
					assert.Equal(t, AgentStatusFailed, output.Status)
				}
			} else {
				assert.NoError(t, err)
				require.NotNil(t, output)
				assert.Equal(t, tt.expectedStatus, output.Status)
			}
		})
	}
}

func TestBaseAgent_NewBaseAgent(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	llm := &mockLLM{}
	tools := react.NewToolRegistry()

	base := NewBaseAgent(llm, tools, logger)
	require.NotNil(t, base)
	assert.Equal(t, llm, base.LLM)
	assert.Equal(t, tools, base.ToolRegistry)
	assert.Equal(t, logger, base.Logger)
	assert.Equal(t, "claude-sonnet", base.Model)
	assert.Equal(t, 0.7, base.Temperature)
	assert.Equal(t, 4096, base.MaxTokens)
}

func TestAgentOutput_Artifacts(t *testing.T) {
	output := &AgentOutput{
		Summary: "Test summary",
		Artifacts: []Artifact{
			{Type: "file", Content: "content", Path: "path/file.go"},
			{Type: "code", Content: "code", Lang: "go"},
		},
		Status: AgentStatusSuccess,
	}

	assert.Equal(t, "Test summary", output.Summary)
	assert.Len(t, output.Artifacts, 2)
	assert.Equal(t, "file", output.Artifacts[0].Type)
	assert.Equal(t, "code", output.Artifacts[1].Type)
}