// Package react implements the ReAct (Reasoning + Acting) agent pattern.
package react

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ----------------------------------------------------------------------------
// ReAct Agent
// ----------------------------------------------------------------------------

// Agent implements the ReAct (Reasoning + Acting) agent pattern.
// It uses an LLM to generate thought-action-observation cycles until
// a final answer is produced or max iterations are reached.
type Agent struct {
	config      *Config
	llm         LLM
	tools       *ToolRegistry
	promptBuilder *PromptBuilder
	logger      *zap.Logger
}

// NewAgent creates a new ReAct agent.
func NewAgent(llm LLM, tools *ToolRegistry, config *Config, logger *zap.Logger) (*Agent, error) {
	if llm == nil {
		return nil, fmt.Errorf("llm is required")
	}
	if tools == nil {
		tools = NewToolRegistry()
	}
	if config == nil {
		config = DefaultConfig()
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	if logger == nil {
		logger, _ = zap.NewProduction()
		defer logger.Sync()
	}

	// Build prompt with tools
	promptBuilder := NewPromptBuilder().
		WithTools(tools.List()).
		WithSystemPrompt(config.SystemPrompt)

	return &Agent{
		config:        config,
		llm:           llm,
		tools:         tools,
		promptBuilder: promptBuilder,
		logger:        logger,
	}, nil
}

// Run executes the ReAct agent with the given user input.
// It blocks until a final answer is produced or the context is cancelled.
func (a *Agent) Run(ctx context.Context, userInput string) (*Result, error) {
	return a.run(ctx, userInput, nil)
}

// RunWithHistory executes the ReAct agent with conversation history.
// This allows for multi-turn conversations.
func (a *Agent) RunWithHistory(ctx context.Context, userInput string, history []*Message) (*Result, error) {
	return a.run(ctx, userInput, history)
}

// run is the internal implementation of the ReAct loop.
func (a *Agent) run(ctx context.Context, userInput string, history []*Message) (*Result, error) {
	startTime := time.Now()

	// Validate input
	if strings.TrimSpace(userInput) == "" && (history == nil || len(history) == 0) {
		return &Result{
			StopReason: StopReasonContextEmpty,
			Error:      fmt.Errorf("user input is empty"),
		}, nil
	}

	// Initialize conversation messages
	messages, err := a.promptBuilder.BuildMessages(ctx, userInput, history)
	if err != nil {
		return nil, fmt.Errorf("failed to build messages: %w", err)
	}

	// Initialize result tracking
	result := &Result{
		Steps: make([]*Step, 0),
	}
	a.logger.Info("Starting ReAct agent",
		zap.String("user_input", truncate(userInput, 100)),
		zap.Int("max_iterations", a.config.MaxIterations),
		zap.Duration("max_execution_time", a.config.MaxExecutionTime))

	// Main ReAct loop
	for iteration := 0; iteration < a.config.MaxIterations; iteration++ {
		// Check for context cancellation or overall timeout
		select {
		case <-ctx.Done():
			result.StopReason = StopReasonCancelled
			result.SetExecutionTime(time.Since(startTime))
			return result, ctx.Err()
		default:
		}

		// Check overall execution timeout
		if a.config.MaxExecutionTime > 0 {
			elapsed := time.Since(startTime)
			if elapsed >= a.config.MaxExecutionTime {
				result.StopReason = StopReasonTimeout
				result.SetExecutionTime(elapsed)
				err := fmt.Errorf("max execution time exceeded: %v", a.config.MaxExecutionTime)
				result.SetError(err)
				a.logger.Warn("ReAct agent timeout",
					zap.Int("iteration", iteration),
					zap.Duration("elapsed", elapsed),
					zap.Duration("max_execution_time", a.config.MaxExecutionTime))
				return result, err
			}
		}

		// Call LLM
		opts := &GenerateOptions{
			MaxTokens:   a.config.MaxTokens,
			Temperature: a.config.Temperature,
			StopSequences: []string{"OBSERVATION:", "Observation:"},
		}

		var llmResult *GenerateResult
		var llmErr error

		// Create a timeout context for the LLM call
		llmCtx, cancel := context.WithTimeout(ctx, a.config.Timeout)
		llmResult, llmErr = a.llm.Generate(llmCtx, messages, opts)
		cancel()

		if llmErr != nil {
			a.logger.Error("LLM call failed",
				zap.Int("iteration", iteration),
				zap.Error(llmErr))

			step := NewStep(StepTypeError, fmt.Sprintf("LLM call failed: %v", llmErr))
			result.Steps = append(result.Steps, step)
			result.StopReason = StopReasonError
			result.SetError(llmErr)
			a.callOnError(fmt.Errorf("LLM call failed: %w", llmErr), step)
			return result, llmErr
		}

		// Track token usage
		result.AddTokens(llmResult.TotalTokens)

		// Parse the LLM response
		parser := NewResponseParser()
		parsed := parser.Parse(llmResult.Content)

		// Build accumulated context for next iteration
		accumulated := a.buildAccumulatedContext(messages, llmResult.Content)

		// Process the response
		if parsed.HasFinalAnswer {
			// If there's also a thought, process it first
			if parsed.HasThought {
				step := NewStep(StepTypeThought, parsed.Thought).
					WithAccumulated(accumulated)
				result.Steps = append(result.Steps, step)
				a.emitStep(step)
				a.callOnIterationEnd(iteration, step)
			}

			// Final answer found
			step := NewStep(StepTypeFinalAnswer, parsed.FinalAnswer).
				WithAccumulated(accumulated)
			result.Steps = append(result.Steps, step)
			a.emitStep(step)

			result.Answer = parsed.FinalAnswer
			result.StopReason = StopReasonFinalAnswer
			result.Iterations = iteration + 1
			result.SetExecutionTime(time.Since(startTime))
			a.logger.Info("ReAct agent completed",
				zap.Int("iterations", result.Iterations),
				zap.Duration("execution_time", result.ExecutionTime))
			return result, nil
		}

		if parsed.HasAction {
			// Execute the tool
			toolCallID := newULID()
			step := NewStep(StepTypeAction, parsed.ActionRaw).
				WithToolInfo(parsed.ActionName, parsed.ActionArgs).
				WithAccumulated(accumulated)
			result.Steps = append(result.Steps, step)
			a.emitStep(step)

			// Get the tool
			tool := a.tools.Get(parsed.ActionName)
			if tool == nil {
				err = fmt.Errorf("unknown tool: %s", parsed.ActionName)
				step := NewStep(StepTypeError, err.Error()).
					WithToolInfo(parsed.ActionName, parsed.ActionArgs).
					WithToolResult(nil)
				result.Steps = append(result.Steps, step)
				result.StopReason = StopReasonError
				result.SetError(err)
				a.callOnError(err, step)
				return result, err
			}

			// Execute the tool with timeout
			toolCtx, cancel := context.WithTimeout(ctx, a.config.Timeout)
			toolResult, toolErr := tool.Execute(toolCtx, parsed.ActionArgs)
			cancel()

			// Check for final answer tool
			if finalAns, ok := toolResult.(*FinalAnswer); ok {
				step := NewStep(StepTypeFinalAnswer, finalAns.Answer).
					WithToolInfo(parsed.ActionName, parsed.ActionArgs).
					WithToolResult(finalAns).
					WithAccumulated(accumulated)
				result.Steps = append(result.Steps, step)
				a.emitStep(step)

				result.Answer = finalAns.Answer
				result.StopReason = StopReasonFinalAnswer
				result.Iterations = iteration + 1
				result.SetExecutionTime(time.Since(startTime))
				return result, nil
			}

			// Format tool result for LLM
			toolResultMsg := &ToolResult{
				ID:     toolCallID,
				Name:   parsed.ActionName,
				Result: toolResult,
			}
			if toolErr != nil {
				toolResultMsg.Error = toolErr.Error()
			}

			// Add assistant message with tool call
			messages = append(messages, &Message{
				Role:    RoleAssistant,
				Content: llmResult.Content,
				ToolUse: &ToolUse{
					ID:   toolCallID,
					Name: parsed.ActionName,
					Args: parsed.ActionArgs,
				},
			})

			// Add tool result message
			observationContent := toolResultMsg.FormatForLLM()
			messages = append(messages, &Message{
				Role:    RoleTool,
				Content: observationContent,
				Name:    parsed.ActionName,
			})

			// Record observation step
			observationStep := NewStep(StepTypeObservation, observationContent).
				WithToolInfo(parsed.ActionName, parsed.ActionArgs).
				WithToolResult(toolResult)
			if toolErr != nil {
				observationStep.WithError(toolErr.Error())
			}
			result.Steps = append(result.Steps, observationStep)
			a.emitStep(observationStep)

			if toolErr != nil {
				a.logger.Warn("Tool execution failed",
					zap.String("tool", parsed.ActionName),
					zap.Error(toolErr))
			}

			// Notify iteration end
			a.callOnIterationEnd(iteration, observationStep)
			continue
		}

		// No final answer and no action - this might be a thought or an error
		// Add the response to messages and continue
		messages = append(messages, &Message{
			Role:    RoleAssistant,
			Content: llmResult.Content,
		})

		if parsed.HasThought {
			step := NewStep(StepTypeThought, parsed.Thought).
				WithAccumulated(accumulated)
			result.Steps = append(result.Steps, step)
			a.emitStep(step)
			a.callOnIterationEnd(iteration, step)
		} else {
			// Unparseable response - ask for clarification
			step := NewStep(StepTypeThought, llmResult.Content).
				WithAccumulated(accumulated)
			result.Steps = append(result.Steps, step)
			a.emitStep(step)
			a.callOnIterationEnd(iteration, step)
		}
	}

	// Max iterations reached
	result.StopReason = StopReasonMaxIterations
	result.Iterations = a.config.MaxIterations
	result.SetExecutionTime(time.Since(startTime))
	result.SetError(fmt.Errorf("max iterations (%d) reached without final answer", a.config.MaxIterations))
	a.logger.Warn("ReAct agent max iterations reached",
		zap.Int("iterations", a.config.MaxIterations))

	return result, nil
}

// buildAccumulatedContext builds the accumulated context for the next LLM call.
func (a *Agent) buildAccumulatedContext(messages []*Message, newContent string) string {
	var sb strings.Builder
	for i, msg := range messages {
		if i > 0 {
			sb.WriteString("\n")
		}
		switch msg.Role {
		case RoleSystem:
			sb.WriteString("[SYSTEM PROMPT]")
		case RoleUser:
			sb.WriteString(fmt.Sprintf("[USER]: %s", msg.Content))
		case RoleAssistant:
			sb.WriteString(fmt.Sprintf("[ASSISTANT]: %s", msg.Content))
		case RoleTool:
			sb.WriteString(fmt.Sprintf("[TOOL %s]: %s", msg.Name, msg.Content))
		}
	}
	sb.WriteString(fmt.Sprintf("\n[ASSISTANT]: %s", newContent))
	return sb.String()
}

// emitStep emits a step if streaming is enabled.
func (a *Agent) emitStep(step *Step) {
	if a.config.EnableStreaming && a.config.StreamingCallback != nil {
		a.config.StreamingCallback(context.Background(), step)
	}
}

// callOnIterationEnd calls the OnIterationEnd callback if configured.
func (a *Agent) callOnIterationEnd(iteration int, step *Step) {
	if a.config.OnIterationEnd != nil {
		a.config.OnIterationEnd(context.Background(), iteration, step)
	}
}

// callOnError calls the OnError callback if configured.
func (a *Agent) callOnError(err error, step *Step) {
	if a.config.OnError != nil {
		a.config.OnError(context.Background(), err, step)
	}
}

// truncate truncates a string to the specified length.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// Verify Agent implements the AgentRunner interface.
var _ AgentRunner = (*Agent)(nil)

// AgentRunner is the interface for running agents, implemented by *Agent.
type AgentRunner interface {
	Run(ctx context.Context, userInput string) (*Result, error)
	RunWithHistory(ctx context.Context, userInput string, history []*Message) (*Result, error)
}

// ----------------------------------------------------------------------------
// Retry Configuration
// ----------------------------------------------------------------------------

// RetryConfig holds retry configuration for LLM calls.
type RetryConfig struct {
	MaxRetries    int
	InitialDelay  time.Duration
	MaxDelay      time.Duration
	BackoffFactor float64
}

// DefaultRetryConfig returns the default retry configuration.
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxRetries:    3,
		InitialDelay:  1 * time.Second,
		MaxDelay:      30 * time.Second,
		BackoffFactor: 2.0,
	}
}

// WithRetry executes a function with retry logic.
func WithRetry(ctx context.Context, config *RetryConfig, fn func() error) error {
	if config == nil {
		config = DefaultRetryConfig()
	}

	var lastErr error
	delay := config.InitialDelay

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}

			delay = time.Duration(float64(delay) * config.BackoffFactor)
			if delay > config.MaxDelay {
				delay = config.MaxDelay
			}
		}

		if err := fn(); err != nil {
			lastErr = err
			continue
		}
		return nil
	}

	return lastErr
}

// ----------------------------------------------------------------------------
// Synchronous Execution Helper
// ----------------------------------------------------------------------------

// RunSync runs the agent synchronously and returns when complete.
func RunSync(agent *Agent, userInput string) (*Result, error) {
	return agent.Run(context.Background(), userInput)
}

// RunWithHistorySync runs the agent with history synchronously.
func RunWithHistorySync(agent *Agent, userInput string, history []*Message) (*Result, error) {
	return agent.RunWithHistory(context.Background(), userInput, history)
}

// ----------------------------------------------------------------------------
// Concurrent Agent Pool
// ----------------------------------------------------------------------------

// AgentPool manages a pool of ReAct agents for parallel execution.
type AgentPool struct {
	agents []*Agent
	mu     sync.Mutex
}

// NewAgentPool creates a new AgentPool.
func NewAgentPool() *AgentPool {
	return &AgentPool{
		agents: make([]*Agent, 0),
	}
}

// Add adds an agent to the pool.
func (p *AgentPool) Add(agent *Agent) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.agents = append(p.agents, agent)
}

// RunAll runs all agents in the pool with the same input.
func (p *AgentPool) RunAll(ctx context.Context, userInput string) ([]*Result, []error) {
	p.mu.Lock()
	count := len(p.agents)
	if count == 0 {
		p.mu.Unlock()
		return nil, []error{fmt.Errorf("no agents in pool")}
	}
	p.mu.Unlock()

	results := make([]*Result, count)
	errors := make([]error, count)
	var wg sync.WaitGroup
	wg.Add(count)

	for i, agent := range p.agents {
		go func(idx int, ag *Agent) {
			defer wg.Done()
			result, err := ag.Run(ctx, userInput)
			results[idx] = result
			errors[idx] = err
		}(i, agent)
	}

	wg.Wait()
	return results, errors
}