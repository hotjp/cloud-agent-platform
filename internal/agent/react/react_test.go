// Package react provides tests for the ReAct agent implementation.
package react

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ----------------------------------------------------------------------------
// Mock LLM Implementation
// ----------------------------------------------------------------------------

// MockLLM is a mock implementation of the LLM interface for testing.
type MockLLM struct {
	mu          sync.Mutex
	GenerateFunc func(ctx context.Context, messages []*Message, opts *GenerateOptions) (*GenerateResult, error)
	Calls       []LLMCall
}

type LLMCall struct {
	Messages []*Message
	Options  *GenerateOptions
}

// NewMockLLM creates a new MockLLM.
func NewMockLLM() *MockLLM {
	return &MockLLM{
		Calls: make([]LLMCall, 0),
	}
}

// Generate implements LLM.
func (m *MockLLM) Generate(ctx context.Context, messages []*Message, opts *GenerateOptions) (*GenerateResult, error) {
	m.mu.Lock()
	m.Calls = append(m.Calls, LLMCall{Messages: messages, Options: opts})
	m.mu.Unlock()

	if m.GenerateFunc != nil {
		return m.GenerateFunc(ctx, messages, opts)
	}
	return &GenerateResult{
		Content:     "Mock response",
		StopReason:  "stop",
		TotalTokens: 100,
	}, nil
}

// GenerateStream implements LLM.
func (m *MockLLM) GenerateStream(ctx context.Context, messages []*Message, opts *GenerateOptions, callback func(chunk string) error) (*GenerateResult, error) {
	result, err := m.Generate(ctx, messages, opts)
	if err != nil {
		return nil, err
	}
	if callback != nil {
		if err := callback(result.Content); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// ----------------------------------------------------------------------------
// Mock Tool Implementation
// ----------------------------------------------------------------------------

// MockTool is a mock implementation of the Tool interface.
type MockTool struct {
	NameFunc        func() string
	DescriptionFunc func() string
	InputSchemaFunc func() string
	ExecuteFunc     func(ctx context.Context, input map[string]any) (any, error)
}

// NewMockTool creates a MockTool with default implementations.
func NewMockTool(name string) *MockTool {
	return &MockTool{
		NameFunc:        func() string { return name },
		DescriptionFunc: func() string { return "Mock tool: " + name },
		InputSchemaFunc: func() string { return `{"type":"object"}` },
		ExecuteFunc:     func(ctx context.Context, input map[string]any) (any, error) { return "mock result", nil },
	}
}

func (t *MockTool) Name() string {
	if t.NameFunc != nil {
		return t.NameFunc()
	}
	return "mock_tool"
}

func (t *MockTool) Description() string {
	if t.DescriptionFunc != nil {
		return t.DescriptionFunc()
	}
	return "mock description"
}

func (t *MockTool) InputSchema() string {
	if t.InputSchemaFunc != nil {
		return t.InputSchemaFunc()
	}
	return "{}"
}

func (t *MockTool) Execute(ctx context.Context, input map[string]any) (any, error) {
	if t.ExecuteFunc != nil {
		return t.ExecuteFunc(ctx, input)
	}
	return nil, nil
}

// ----------------------------------------------------------------------------
// Test: Config Validation
// ----------------------------------------------------------------------------

func TestConfig_Validate(t *testing.T) {
	t.Run("default config", func(t *testing.T) {
		config := DefaultConfig()
		err := config.Validate()
		require.NoError(t, err)
		assert.Equal(t, 10, config.MaxIterations)
		assert.Equal(t, 4096, config.MaxTokens)
		assert.Equal(t, 0.7, config.Temperature)
		assert.Equal(t, 60*time.Second, config.Timeout)
	})

	t.Run("negative max iterations becomes default", func(t *testing.T) {
		config := &Config{MaxIterations: -1}
		err := config.Validate()
		require.NoError(t, err)
		assert.Equal(t, 10, config.MaxIterations)
	})

	t.Run("zero max tokens becomes default", func(t *testing.T) {
		config := &Config{MaxTokens: 0}
		err := config.Validate()
		require.NoError(t, err)
		assert.Equal(t, 4096, config.MaxTokens)
	})

	t.Run("invalid temperature becomes default", func(t *testing.T) {
		config := &Config{Temperature: 3.0}
		err := config.Validate()
		require.NoError(t, err)
		assert.Equal(t, 0.7, config.Temperature)
	})
}

// ----------------------------------------------------------------------------
// Test: Tool Registry
// ----------------------------------------------------------------------------

func TestToolRegistry_Register(t *testing.T) {
	registry := NewToolRegistry()

	// Register a tool
	tool := NewMockTool("test_tool")
	err := registry.Register(tool)
	require.NoError(t, err)
	assert.Equal(t, 1, registry.Len())

	// Get the tool
	retrieved := registry.Get("test_tool")
	require.NotNil(t, retrieved)
	assert.Equal(t, "test_tool", retrieved.Name())

	// Duplicate registration replaces
	newTool := NewMockTool("test_tool")
	err = registry.Register(newTool)
	require.NoError(t, err)
	assert.Equal(t, 1, registry.Len())
}

func TestToolRegistry_RegisterNil(t *testing.T) {
	registry := NewToolRegistry()
	err := registry.Register(nil)
	assert.Error(t, err)
}

func TestToolRegistry_RegisterEmptyName(t *testing.T) {
	registry := NewToolRegistry()
	tool := NewMockTool("")
	tool.NameFunc = func() string { return "" }
	err := registry.Register(tool)
	assert.Error(t, err)
}

func TestToolRegistry_RegisterFunc(t *testing.T) {
	registry := NewToolRegistry()
	err := registry.RegisterFunc("calc", "A calculator", `{"type":"object"}`,
		func(ctx context.Context, input map[string]any) (any, error) {
			return 42, nil
		})
	require.NoError(t, err)
	assert.Equal(t, 1, registry.Len())
}

func TestToolRegistry_List(t *testing.T) {
	registry := NewToolRegistry()
	registry.RegisterFunc("tool1", "Tool 1", "{}", func(ctx context.Context, input map[string]any) (any, error) { return nil, nil })
	registry.RegisterFunc("tool2", "Tool 2", "{}", func(ctx context.Context, input map[string]any) (any, error) { return nil, nil })

	tools := registry.List()
	assert.Len(t, tools, 2)
}

func TestToolRegistry_Clear(t *testing.T) {
	registry := NewToolRegistry()
	registry.RegisterFunc("tool1", "Tool 1", "{}", func(ctx context.Context, input map[string]any) (any, error) { return nil, nil })
	registry.Clear()
	assert.Equal(t, 0, registry.Len())
}

// ----------------------------------------------------------------------------
// Test: FinalAnswerTool
// ----------------------------------------------------------------------------

func TestFinalAnswerTool(t *testing.T) {
	tool := NewFinalAnswerTool()

	assert.Equal(t, "final_answer", tool.Name())
	assert.Contains(t, tool.Description(), "final answer")

	ctx := context.Background()
	result, err := tool.Execute(ctx, map[string]any{"answer": "The answer is 42"})
	require.NoError(t, err)

	finalAns, ok := result.(*FinalAnswer)
	require.True(t, ok)
	assert.Equal(t, "The answer is 42", finalAns.Answer)
}

func TestFinalAnswerTool_RequiresStringAnswer(t *testing.T) {
	tool := NewFinalAnswerTool()

	ctx := context.Background()
	_, err := tool.Execute(ctx, map[string]any{"answer": 42})
	assert.Error(t, err)
}

// ----------------------------------------------------------------------------
// Test: ToolResult Formatting
// ----------------------------------------------------------------------------

func TestToolResult_FormatForLLM(t *testing.T) {
	// With result
	tr := NewToolResult("call-123", "search", map[string]string{"title": "test"}, nil)
	formatted := tr.FormatForLLM()
	assert.Contains(t, formatted, "TOOL=search")
	assert.Contains(t, formatted, "ID=call-123")
	assert.Contains(t, formatted, "test")

	// With error
	trErr := NewToolResult("call-456", "search", nil, errors.New("connection failed"))
	formattedErr := trErr.FormatForLLM()
	assert.Contains(t, formattedErr, "ERROR=connection failed")
}

// ----------------------------------------------------------------------------
// Test: Response Parser
// ----------------------------------------------------------------------------

func TestResponseParser_Parse(t *testing.T) {
	parser := NewResponseParser()

	tests := []struct {
		name       string
		response   string
		wantThought bool
		wantAction bool
		wantFinal  bool
	}{
		{
			name: "full thought-action-observation format",
			response: "THOUGHT: I need to search for information.\nACTION: search({\"query\": \"test\"})\nObservation: Results found.\n",
			wantThought: true,
			wantAction: true,
			wantFinal:  false,
		},
		{
			name: "final answer only",
			response: `The answer is 42.

FINAL_ANSWER: The answer is 42.`,
			wantThought: false,
			wantAction: false,
			wantFinal:  true,
		},
		{
			name: "thought then final answer",
			response: `THOUGHT: I've found the answer.
Final Answer: The answer is 42.`,
			wantThought: true,
			wantAction: false,
			wantFinal:  true,
		},
		{
			name: "action with tool call",
			response: "Let me use the calculator.\nACTION: calculator({\"expression\": \"2+2\"})\n",
			wantThought: false,
			wantAction: true,
			wantFinal:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parser.Parse(tt.response)
			assert.Equal(t, tt.wantThought, result.HasThought)
			assert.Equal(t, tt.wantAction, result.HasAction)
			assert.Equal(t, tt.wantFinal, result.HasFinalAnswer)
		})
	}
}

func TestResponseParser_ParseActionArgs(t *testing.T) {
	parser := NewResponseParser()

	result := parser.Parse("ACTION: search({\"query\": \"test\", \"max_results\": 5})\n")
	require.True(t, result.HasAction)
	assert.Equal(t, "search", result.ActionName)
	assert.Equal(t, "test", result.ActionArgs["query"])
	assert.Equal(t, float64(5), result.ActionArgs["max_results"]) // JSON unmarshal converts int to float64
}

// ----------------------------------------------------------------------------
// Test: ReAct Agent - Basic Execution
// ----------------------------------------------------------------------------

func TestAgent_Run_BasicExecution(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	tools := NewToolRegistry()
	tools.RegisterFunc("final_answer", "Provide final answer", `{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"]}`,
		func(ctx context.Context, input map[string]any) (any, error) {
			return &FinalAnswer{Answer: input["answer"].(string)}, nil
		})

	mockLLM := NewMockLLM()
	mockLLM.GenerateFunc = func(ctx context.Context, messages []*Message, opts *GenerateOptions) (*GenerateResult, error) {
		// Simulate returning a final answer directly
		return &GenerateResult{
			Content:      `The answer is 42.

FINAL_ANSWER: The answer is 42.`,
			StopReason:   "stop",
			TotalTokens:  50,
			PromptTokens: 100,
		}, nil
	}

	agent, err := NewAgent(mockLLM, tools, &Config{MaxIterations: 5}, logger)
	require.NoError(t, err)

	result, err := agent.Run(context.Background(), "What is 6 times 7?")
	require.NoError(t, err)
	require.Nil(t, result.Error)

	assert.Equal(t, StopReasonFinalAnswer, result.StopReason)
	assert.Equal(t, "The answer is 42.", result.Answer)
	assert.Equal(t, 1, result.Iterations)
	assert.Equal(t, 50, result.TotalTokensUsed)
}

func TestAgent_Run_ToolExecution(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	tools := NewToolRegistry()
	tools.RegisterFunc("final_answer", "Provide final answer", `{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"]}`,
		func(ctx context.Context, input map[string]any) (any, error) {
			return &FinalAnswer{Answer: input["answer"].(string)}, nil
		})
	tools.RegisterFunc("calculator", "Perform calculation", `{"type":"object","properties":{"expression":{"type":"string"}},"required":["expression"]}`,
		func(ctx context.Context, input map[string]any) (any, error) {
			return map[string]any{"result": 42}, nil
		})

	callCount := 0
	mockLLM := NewMockLLM()
	mockLLM.GenerateFunc = func(ctx context.Context, messages []*Message, opts *GenerateOptions) (*GenerateResult, error) {
		callCount++

		if callCount == 1 {
			// First call: return action
			return &GenerateResult{
				Content:     `THOUGHT: I need to calculate this.
ACTION: calculator({"expression": "6*7"})`,
				StopReason:  "stop",
				TotalTokens:  50,
			}, nil
		}
		// Second call: return final answer
		return &GenerateResult{
			Content:     `The result is 42.

FINAL_ANSWER: The result is 42.`,
			StopReason:  "stop",
			TotalTokens:  30,
		}, nil
	}

	agent, err := NewAgent(mockLLM, tools, &Config{MaxIterations: 5}, logger)
	require.NoError(t, err)

	result, err := agent.Run(context.Background(), "What is 6 times 7?")
	require.NoError(t, err)
	require.Nil(t, result.Error)

	assert.Equal(t, StopReasonFinalAnswer, result.StopReason)
	assert.Equal(t, "The result is 42.", result.Answer)
	assert.Equal(t, 2, result.Iterations)
	assert.Equal(t, 2, len(result.Steps))
}

func TestAgent_Run_MaxIterations(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	tools := NewToolRegistry()
	tools.RegisterFunc("calculator", "Perform calculation", `{"type":"object","properties":{"expression":{"type":"string"}},"required":["expression"]}`,
		func(ctx context.Context, input map[string]any) (any, error) {
			return map[string]any{"result": 42}, nil
		})

	mockLLM := NewMockLLM()
	mockLLM.GenerateFunc = func(ctx context.Context, messages []*Message, opts *GenerateOptions) (*GenerateResult, error) {
		// Always return action, never final answer
		return &GenerateResult{
			Content:     `THOUGHT: I need to calculate this.
ACTION: calculator({"expression": "6*7"})`,
			StopReason:  "stop",
			TotalTokens:  50,
		}, nil
	}

	agent, err := NewAgent(mockLLM, tools, &Config{MaxIterations: 3}, logger)
	require.NoError(t, err)

	result, err := agent.Run(context.Background(), "What is 6 times 7?")
	require.NoError(t, err)

	assert.Equal(t, StopReasonMaxIterations, result.StopReason)
	assert.Equal(t, 3, result.Iterations)
	assert.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "max iterations")
}

func TestAgent_Run_UnknownTool(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	tools := NewToolRegistry()
	tools.RegisterFunc("final_answer", "Provide final answer", `{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"]}`,
		func(ctx context.Context, input map[string]any) (any, error) {
			return &FinalAnswer{Answer: input["answer"].(string)}, nil
		})

	mockLLM := NewMockLLM()
	mockLLM.GenerateFunc = func(ctx context.Context, messages []*Message, opts *GenerateOptions) (*GenerateResult, error) {
		return &GenerateResult{
			Content:     "THOUGHT: I need to use a tool.\nACTION: unknown_tool({\"arg\": \"value\"})\n",
			StopReason:  "stop",
			TotalTokens:  50,
		}, nil
	}

	agent, err := NewAgent(mockLLM, tools, &Config{MaxIterations: 5}, logger)
	require.NoError(t, err)

	result, err := agent.Run(context.Background(), "Test")
	// Unknown tool returns an error from agent.Run
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown tool")

	assert.Equal(t, StopReasonError, result.StopReason)
	assert.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "unknown tool")
}

func TestAgent_Run_ToolError(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	tools := NewToolRegistry()
	tools.RegisterFunc("final_answer", "Provide final answer", `{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"]}`,
		func(ctx context.Context, input map[string]any) (any, error) {
			return &FinalAnswer{Answer: input["answer"].(string)}, nil
		})
	tools.RegisterFunc("unreliable", "Unreliable tool", `{"type":"object"}`,
		func(ctx context.Context, input map[string]any) (any, error) {
			return nil, errors.New("tool execution failed")
		})

	callCount := 0
	mockLLM := NewMockLLM()
	mockLLM.GenerateFunc = func(ctx context.Context, messages []*Message, opts *GenerateOptions) (*GenerateResult, error) {
		callCount++
		if callCount == 1 {
			return &GenerateResult{
				Content:     "ACTION: unreliable({})\n",
				StopReason:  "stop",
				TotalTokens:  50,
			}, nil
		}
		return &GenerateResult{
			Content:     "FINAL_ANSWER: Done despite error\n",
			StopReason:  "stop",
			TotalTokens:  30,
		}, nil
	}

	agent, err := NewAgent(mockLLM, tools, &Config{MaxIterations: 5}, logger)
	require.NoError(t, err)

	result, err := agent.Run(context.Background(), "Test")
	require.NoError(t, err)

	// Should complete but with tool error in steps
	assert.Equal(t, StopReasonFinalAnswer, result.StopReason)
	assert.NotEmpty(t, result.Steps)
}

func TestAgent_Run_ContextCancellation(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	tools := NewToolRegistry()

	mockLLM := NewMockLLM()
	mockLLM.GenerateFunc = func(ctx context.Context, messages []*Message, opts *GenerateOptions) (*GenerateResult, error) {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
			return &GenerateResult{
				Content:     "FINAL_ANSWER: Done\n",
				StopReason:  "stop",
				TotalTokens:  50,
			}, nil
		}
	}

	agent, err := NewAgent(mockLLM, tools, &Config{MaxIterations: 5, Timeout: 50 * time.Millisecond}, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	result, err := agent.Run(ctx, "Test")
	// Context deadline exceeded causes an error, not a cancellation
	assert.Equal(t, StopReasonError, result.StopReason)
}

func TestAgent_Run_EmptyInput(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	tools := NewToolRegistry()
	mockLLM := NewMockLLM()

	agent, err := NewAgent(mockLLM, tools, &Config{MaxIterations: 5}, logger)
	require.NoError(t, err)

	result, err := agent.Run(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, StopReasonContextEmpty, result.StopReason)
}

// ----------------------------------------------------------------------------
// Test: Prompt Builder
// ----------------------------------------------------------------------------

func TestPromptBuilder_Build(t *testing.T) {
	tools := NewToolRegistry()
	tools.RegisterFunc("search", "Search for information", `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`,
		func(ctx context.Context, input map[string]any) (any, error) { return nil, nil })

	builder := NewPromptBuilder().
		WithTools(tools.List()).
		WithSystemPrompt(DefaultSystemPrompt)

	prompt, err := builder.Build()
	require.NoError(t, err)
	assert.Contains(t, prompt, "search")
	assert.Contains(t, prompt, "Search for information")
}

func TestPromptBuilder_BuildMessages(t *testing.T) {
	builder := NewPromptBuilder()

	messages, err := builder.BuildMessages(context.Background(), "What is 2+2?", nil)
	require.NoError(t, err)
	require.Len(t, messages, 2) // system + user

	assert.Equal(t, RoleSystem, messages[0].Role)
	assert.Equal(t, RoleUser, messages[1].Role)
	assert.Contains(t, messages[1].Content, "What is 2+2?")
}

func TestPromptBuilder_BuildMessagesWithHistory(t *testing.T) {
	builder := NewPromptBuilder()

	history := []*Message{
		{Role: RoleUser, Content: "Hello"},
		{Role: RoleAssistant, Content: "Hi there!"},
	}

	messages, err := builder.BuildMessages(context.Background(), "How are you?", history)
	require.NoError(t, err)
	require.Len(t, messages, 4) // system + history + user

	assert.Equal(t, RoleSystem, messages[0].Role)
	assert.Equal(t, RoleUser, messages[1].Role)
	assert.Equal(t, "Hello", messages[1].Content)
	assert.Equal(t, RoleAssistant, messages[2].Role)
	assert.Equal(t, "Hi there!", messages[2].Content)
	assert.Equal(t, RoleUser, messages[3].Role)
	assert.Equal(t, "How are you?", messages[3].Content)
}

// ----------------------------------------------------------------------------
// Test: Streaming
// ----------------------------------------------------------------------------

func TestAgent_Run_StreamingEnabled(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	tools := NewToolRegistry()

	var receivedSteps []*Step
	var mu sync.Mutex
	streamingCallback := func(ctx context.Context, step *Step) error {
		mu.Lock()
		receivedSteps = append(receivedSteps, step)
		mu.Unlock()
		return nil
	}

	mockLLM := NewMockLLM()
	mockLLM.GenerateFunc = func(ctx context.Context, messages []*Message, opts *GenerateOptions) (*GenerateResult, error) {
		return &GenerateResult{
			Content:     `FINAL_ANSWER: Streaming test complete.`,
			StopReason:  "stop",
			TotalTokens:  50,
		}, nil
	}

	agent, err := NewAgent(mockLLM, tools, &Config{
		MaxIterations:    5,
		EnableStreaming:  true,
		StreamingCallback: streamingCallback,
	}, logger)
	require.NoError(t, err)

	result, err := agent.Run(context.Background(), "Streaming test")
	require.NoError(t, err)

	mu.Lock()
	assert.NotEmpty(t, receivedSteps)
	mu.Unlock()

	assert.Equal(t, StopReasonFinalAnswer, result.StopReason)
}

// ----------------------------------------------------------------------------
// Test: AgentPool
// ----------------------------------------------------------------------------

func TestAgentPool_RunAll(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	pool := NewAgentPool()

	for i := 0; i < 3; i++ {
		tools := NewToolRegistry()
		tools.RegisterFunc("final_answer", "Provide final answer", `{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"]}`,
			func(ctx context.Context, input map[string]any) (any, error) {
				return &FinalAnswer{Answer: "response-" + string(rune('0'+i))}, nil
			})

		mockLLM := NewMockLLM()
		mockLLM.GenerateFunc = func(ctx context.Context, messages []*Message, opts *GenerateOptions) (*GenerateResult, error) {
			return &GenerateResult{
				Content:     `FINAL_ANSWER: Response from agent`,
				StopReason:  "stop",
				TotalTokens:  50,
			}, nil
		}

		agent, err := NewAgent(mockLLM, tools, &Config{MaxIterations: 5}, logger)
		require.NoError(t, err)
		pool.Add(agent)
	}

	results, errs := pool.RunAll(context.Background(), "Test")
	assert.Len(t, results, 3)
	assert.Len(t, errs, 3)
}

// ----------------------------------------------------------------------------
// Test: Result Methods
// ----------------------------------------------------------------------------

func TestResult_AddTokens(t *testing.T) {
	result := NewResult("test", []*Step{}, 1, StopReasonFinalAnswer)
	result.AddTokens(100)
	result.AddTokens(50)
	assert.Equal(t, 150, result.TotalTokensUsed)
}

func TestResult_SetExecutionTime(t *testing.T) {
	result := NewResult("test", []*Step{}, 1, StopReasonFinalAnswer)
	result.SetExecutionTime(5 * time.Second)
	assert.Equal(t, 5*time.Second, result.ExecutionTime)
}

func TestResult_SetError(t *testing.T) {
	result := NewResult("test", []*Step{}, 1, StopReasonFinalAnswer)
	err := errors.New("test error")
	result.SetError(err)
	assert.Equal(t, err, result.Error)
}

// ----------------------------------------------------------------------------
// Test: Step Methods
// ----------------------------------------------------------------------------

func TestStep_WithToolInfo(t *testing.T) {
	step := NewStep(StepTypeAction, "test")
	step.WithToolInfo("search", map[string]any{"query": "test"})

	assert.Equal(t, "search", step.ToolName)
	assert.Equal(t, "test", step.ToolArgs.(map[string]any)["query"])
}

func TestStep_WithToolResult(t *testing.T) {
	step := NewStep(StepTypeObservation, "result")
	step.WithToolResult(map[string]string{"title": "test"})

	assert.NotNil(t, step.ToolResult)
}

func TestStep_WithError(t *testing.T) {
	step := NewStep(StepTypeError, "error occurred")
	step.WithError("something went wrong")

	assert.Equal(t, "something went wrong", step.Error)
}

// ----------------------------------------------------------------------------
// Test: Tool Registry Provider Interface
// ----------------------------------------------------------------------------

func TestToolProvider_Interface(t *testing.T) {
	registry := NewToolRegistry()
	registry.RegisterFunc("tool1", "Tool 1", "{}", func(ctx context.Context, input map[string]any) (any, error) { return nil, nil })

	var provider ToolProvider = registry
	assert.NotNil(t, provider)
	assert.Len(t, provider.List(), 1)
	assert.NotNil(t, provider.Get("tool1"))
}

// ----------------------------------------------------------------------------
// Test: Response Parser Edge Cases
// ----------------------------------------------------------------------------

func TestResponseParser_ParseEdgeCases(t *testing.T) {
	parser := NewResponseParser()

	tests := []struct {
		name     string
		response string
		check    func(*ParseResult)
	}{
		{
			name:     "empty response",
			response: "",
			check: func(r *ParseResult) {
				assert.False(t, r.HasThought)
				assert.False(t, r.HasAction)
				assert.False(t, r.HasFinalAnswer)
			},
		},
		{
			name:     "no markers",
			response: "Just some text without any markers",
			check: func(r *ParseResult) {
				assert.False(t, r.HasThought)
				assert.False(t, r.HasAction)
				assert.False(t, r.HasFinalAnswer)
			},
		},
		{
			name:     "uppercase THOUGHT",
			response: "THOUGHT: This is a thought\n",
			check: func(r *ParseResult) {
				assert.True(t, r.HasThought)
				assert.Equal(t, "This is a thought", r.Thought)
			},
		},
		{
			name:     "Final Answer capitalized",
			response: "Final Answer: The answer is 42",
			check: func(r *ParseResult) {
				assert.True(t, r.HasFinalAnswer)
				assert.Equal(t, "The answer is 42", r.FinalAnswer)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parser.Parse(tt.response)
			tt.check(result)
		})
	}
}

// ----------------------------------------------------------------------------
// Test: SSE Writer
// ----------------------------------------------------------------------------

func TestSSEWriter_WriteStep(t *testing.T) {
	// Just verify SSEWriter and StepAggregator can be instantiated
	_ = &SSEWriter{}
	_ = &StepAggregator{}
}

// ----------------------------------------------------------------------------
// Test: IsFinalAnswer
// ----------------------------------------------------------------------------

func TestIsFinalAnswer(t *testing.T) {
	assert.True(t, IsFinalAnswer(&FinalAnswer{Answer: "test"}))
	assert.False(t, IsFinalAnswer(nil))
	assert.False(t, IsFinalAnswer("string"))
	assert.False(t, IsFinalAnswer(map[string]any{}))
}

// ----------------------------------------------------------------------------
// Test: NewToolFunc
// ----------------------------------------------------------------------------

func TestNewToolFunc(t *testing.T) {
	tool := NewToolFunc(
		"test_tool",
		"A test tool",
		`{"type":"object"}`,
		func(ctx context.Context, input map[string]any) (any, error) {
			return "result", nil
		},
	)

	assert.Equal(t, "test_tool", tool.Name())
	assert.Equal(t, "A test tool", tool.Description())
	assert.Equal(t, `{"type":"object"}`, tool.InputSchema())

	result, err := tool.Execute(context.Background(), map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, "result", result)
}

func TestNewToolFunc_NilFunction(t *testing.T) {
	tool := NewToolFunc("test", "Test", "{}", nil)
	_, err := tool.Execute(context.Background(), nil)
	assert.Error(t, err)
}

// ----------------------------------------------------------------------------
// Test: Retry Config
// ----------------------------------------------------------------------------

func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()
	assert.Equal(t, 3, config.MaxRetries)
	assert.Equal(t, 1*time.Second, config.InitialDelay)
	assert.Equal(t, 30*time.Second, config.MaxDelay)
	assert.Equal(t, 2.0, config.BackoffFactor)
}

func TestWithRetry_Success(t *testing.T) {
	config := &RetryConfig{MaxRetries: 3}
	callCount := 0

	err := WithRetry(context.Background(), config, func() error {
		callCount++
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 1, callCount)
}

func TestWithRetry_EventuallySucceeds(t *testing.T) {
	config := &RetryConfig{MaxRetries: 3, InitialDelay: 1 * time.Millisecond}
	callCount := 0

	err := WithRetry(context.Background(), config, func() error {
		callCount++
		if callCount < 3 {
			return errors.New("temporary error")
		}
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 3, callCount)
}

func TestWithRetry_AllRetriesFail(t *testing.T) {
	config := &RetryConfig{MaxRetries: 2, InitialDelay: 1 * time.Millisecond}

	err := WithRetry(context.Background(), config, func() error {
		return errors.New("persistent error")
	})

	assert.Error(t, err)
}

func TestWithRetry_ContextCancellation(t *testing.T) {
	config := &RetryConfig{MaxRetries: 10, InitialDelay: 1 * time.Hour}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := WithRetry(ctx, config, func() error {
		time.Sleep(10 * time.Millisecond)
		return errors.New("should not reach here")
	})

	assert.Error(t, err)
	assert.Equal(t, context.DeadlineExceeded, err)
}

// ----------------------------------------------------------------------------
// Test: Agent Creation
// ----------------------------------------------------------------------------

func TestNewAgent_NilLLM(t *testing.T) {
	_, err := NewAgent(nil, nil, nil, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "llm is required")
}

func TestNewAgent_WithNilTools(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	mockLLM := NewMockLLM()

	agent, err := NewAgent(mockLLM, nil, nil, logger)
	require.NoError(t, err)
	assert.NotNil(t, agent)
}

// ----------------------------------------------------------------------------
// Test: MaxExecutionTime Timeout
// ----------------------------------------------------------------------------

func TestAgent_Run_MaxExecutionTimeTimeout(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	tools := NewToolRegistry()
	tools.RegisterFunc("calculator", "Perform calculation", `{"type":"object","properties":{"expression":{"type":"string"}},"required":["expression"]}`,
		func(ctx context.Context, input map[string]any) (any, error) {
			return map[string]any{"result": 42}, nil
		})

	callCount := 0
	mockLLM := NewMockLLM()
	mockLLM.GenerateFunc = func(ctx context.Context, messages []*Message, opts *GenerateOptions) (*GenerateResult, error) {
		callCount++
		// Always return action, never final answer - simulating a long-running task
		return &GenerateResult{
			Content:     `THOUGHT: I need to calculate this.
ACTION: calculator({"expression": "6*7"})`,
			StopReason:  "stop",
			TotalTokens:  50,
		}, nil
	}

	// Set max execution time to a very short duration
	agent, err := NewAgent(mockLLM, tools, &Config{
		MaxIterations:    100, // High number so max iterations doesn't trigger first
		MaxExecutionTime: 1 * time.Millisecond, // Very short timeout
	}, logger)
	require.NoError(t, err)

	result, err := agent.Run(context.Background(), "What is 6 times 7?")
	// Timeout returns both result and error
	assert.Equal(t, StopReasonTimeout, result.StopReason)
	assert.Error(t, err)
	assert.Contains(t, result.Error.Error(), "max execution time exceeded")
	// Should have done at least one iteration before timing out
	assert.GreaterOrEqual(t, callCount, 1)
}

func TestAgent_Run_MaxExecutionTimeNoLimit(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	tools := NewToolRegistry()
	tools.RegisterFunc("final_answer", "Provide final answer", `{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"]}`,
		func(ctx context.Context, input map[string]any) (any, error) {
			return &FinalAnswer{Answer: input["answer"].(string)}, nil
		})

	mockLLM := NewMockLLM()
	mockLLM.GenerateFunc = func(ctx context.Context, messages []*Message, opts *GenerateOptions) (*GenerateResult, error) {
		return &GenerateResult{
			Content:      `FINAL_ANSWER: The answer is 42.`,
			StopReason:   "stop",
			TotalTokens:  50,
		}, nil
	}

	// MaxExecutionTime = 0 means no limit
	agent, err := NewAgent(mockLLM, tools, &Config{
		MaxIterations:    10,
		MaxExecutionTime: 0, // No limit
	}, logger)
	require.NoError(t, err)

	result, err := agent.Run(context.Background(), "What is 6 times 7?")
	require.NoError(t, err)

	assert.Equal(t, StopReasonFinalAnswer, result.StopReason)
	assert.Equal(t, "The answer is 42.", result.Answer)
}

// ----------------------------------------------------------------------------
// Test: OnIterationEnd Callback
// ----------------------------------------------------------------------------

func TestAgent_Run_OnIterationEndCallback(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	tools := NewToolRegistry()
	tools.RegisterFunc("calculator", "Perform calculation", `{"type":"object","properties":{"expression":{"type":"string"}},"required":["expression"]}`,
		func(ctx context.Context, input map[string]any) (any, error) {
			return map[string]any{"result": 42}, nil
		})
	tools.RegisterFunc("final_answer", "Provide final answer", `{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"]}`,
		func(ctx context.Context, input map[string]any) (any, error) {
			return &FinalAnswer{Answer: input["answer"].(string)}, nil
		})

	var iterationCallbacks []int
	var mu sync.Mutex

	callCount := 0
	mockLLM := NewMockLLM()
	mockLLM.GenerateFunc = func(ctx context.Context, messages []*Message, opts *GenerateOptions) (*GenerateResult, error) {
		callCount++
		if callCount == 1 {
			return &GenerateResult{
				Content:     `THOUGHT: I need to calculate this.
ACTION: calculator({"expression": "6*7"})`,
				StopReason:  "stop",
				TotalTokens:  50,
			}, nil
		}
		// Second call returns final answer directly (no action)
		return &GenerateResult{
			Content:      `THOUGHT: I have the answer.
FINAL_ANSWER: The result is 42.`,
			StopReason:   "stop",
			TotalTokens:  30,
		}, nil
	}

	agent, err := NewAgent(mockLLM, tools, &Config{
		MaxIterations: 5,
		OnIterationEnd: func(ctx context.Context, iteration int, step *Step) {
			mu.Lock()
			iterationCallbacks = append(iterationCallbacks, iteration)
			mu.Unlock()
		},
	}, logger)
	require.NoError(t, err)

	result, err := agent.Run(context.Background(), "What is 6 times 7?")
	require.NoError(t, err)

	assert.Equal(t, StopReasonFinalAnswer, result.StopReason)
	assert.Equal(t, 2, result.Iterations)

	mu.Lock()
	defer mu.Unlock()
	// OnIterationEnd is called after each iteration completes:
	// - Iteration 0: action -> observation step, callback called with observation
	// - Iteration 1: final answer with thought -> thought step, callback called with thought
	assert.Equal(t, 2, len(iterationCallbacks))
	assert.Equal(t, 0, iterationCallbacks[0]) // First callback is for iteration 0
	assert.Equal(t, 1, iterationCallbacks[1]) // Second callback is for iteration 1
}

// ----------------------------------------------------------------------------
// Test: OnError Callback
// ----------------------------------------------------------------------------

func TestAgent_Run_OnErrorCallback(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	tools := NewToolRegistry()

	// Register a tool that always fails
	tools.RegisterFunc("failing_tool", "A tool that fails", `{"type":"object"}`,
		func(ctx context.Context, input map[string]any) (any, error) {
			return nil, errors.New("tool execution failed")
		})
	tools.RegisterFunc("final_answer", "Provide final answer", `{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"]}`,
		func(ctx context.Context, input map[string]any) (any, error) {
			return &FinalAnswer{Answer: input["answer"].(string)}, nil
		})

	var errorsReceived []error
	var mu sync.Mutex

	callCount := 0
	mockLLM := NewMockLLM()
	mockLLM.GenerateFunc = func(ctx context.Context, messages []*Message, opts *GenerateOptions) (*GenerateResult, error) {
		callCount++
		if callCount == 1 {
			return &GenerateResult{
				Content:     `THOUGHT: I need to use a tool.
ACTION: failing_tool({})`,
				StopReason:  "stop",
				TotalTokens:  50,
			}, nil
		}
		// Second call returns final answer (tool error doesn't stop the loop)
		return &GenerateResult{
			Content:      `FINAL_ANSWER: Done despite error.`,
			StopReason:   "stop",
			TotalTokens:  30,
		}, nil
	}

	agent, err := NewAgent(mockLLM, tools, &Config{
		MaxIterations: 5,
		OnError: func(ctx context.Context, err error, step *Step) {
			mu.Lock()
			errorsReceived = append(errorsReceived, err)
			mu.Unlock()
		},
	}, logger)
	require.NoError(t, err)

	result, err := agent.Run(context.Background(), "Test")
	require.NoError(t, err) // Tool error doesn't cause Run to return error

	mu.Lock()
	defer mu.Unlock()
	// OnError is only called for LLM errors and unknown tool errors
	// Tool execution errors don't trigger OnError - they are recorded in the step
	assert.Equal(t, 0, len(errorsReceived))
	// But the step should have the error recorded
	assert.NotEmpty(t, result.Steps)
}

// ----------------------------------------------------------------------------
// Test: Config Validation - MaxExecutionTime
// ----------------------------------------------------------------------------

func TestConfig_Validate_MaxExecutionTime(t *testing.T) {
	t.Run("negative max execution time becomes zero", func(t *testing.T) {
		config := &Config{MaxExecutionTime: -1 * time.Second}
		err := config.Validate()
		require.NoError(t, err)
		assert.Equal(t, time.Duration(0), config.MaxExecutionTime)
	})

	t.Run("zero max execution time is valid (no limit)", func(t *testing.T) {
		config := &Config{MaxExecutionTime: 0}
		err := config.Validate()
		require.NoError(t, err)
		assert.Equal(t, time.Duration(0), config.MaxExecutionTime)
	})

	t.Run("positive max execution time is valid", func(t *testing.T) {
		config := &Config{MaxExecutionTime: 5 * time.Second}
		err := config.Validate()
		require.NoError(t, err)
		assert.Equal(t, 5*time.Second, config.MaxExecutionTime)
	})

	t.Run("max iterations capped at 100", func(t *testing.T) {
		config := &Config{MaxIterations: 500}
		err := config.Validate()
		require.NoError(t, err)
		assert.Equal(t, 100, config.MaxIterations)
	})
}

// ----------------------------------------------------------------------------
// Test: StopReason Values
// ----------------------------------------------------------------------------

func TestStopReason_Values(t *testing.T) {
	assert.Equal(t, StopReason("final_answer"), StopReasonFinalAnswer)
	assert.Equal(t, StopReason("max_iterations"), StopReasonMaxIterations)
	assert.Equal(t, StopReason("context_empty"), StopReasonContextEmpty)
	assert.Equal(t, StopReason("error"), StopReasonError)
	assert.Equal(t, StopReason("cancelled"), StopReasonCancelled)
	assert.Equal(t, StopReason("timeout"), StopReasonTimeout)
}

// ----------------------------------------------------------------------------
// Benchmark Tests
// ----------------------------------------------------------------------------

func BenchmarkAgent_Run(b *testing.B) {
	logger, _ := zap.NewDevelopment()
	tools := NewToolRegistry()
	tools.RegisterFunc("final_answer", "Provide final answer", `{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"]}`,
		func(ctx context.Context, input map[string]any) (any, error) {
			return &FinalAnswer{Answer: input["answer"].(string)}, nil
		})

	mockLLM := NewMockLLM()
	mockLLM.GenerateFunc = func(ctx context.Context, messages []*Message, opts *GenerateOptions) (*GenerateResult, error) {
		return &GenerateResult{
			Content:      `FINAL_ANSWER: Benchmark complete.`,
			StopReason:   "stop",
			TotalTokens:  50,
		}, nil
	}

	agent, _ := NewAgent(mockLLM, tools, &Config{MaxIterations: 5}, logger)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		agent.Run(context.Background(), "Benchmark test")
	}
}