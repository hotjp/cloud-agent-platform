// Package orchestrator implements Eino-based task orchestration graph.
// Responsible for task decomposition, agent matching, and execution scheduling.
package orchestrator

import (
	"context"
	"fmt"

	"github.com/cloud-agent-platform/cap/internal/agent/react"
	"github.com/cloud-agent-platform/cap/internal/observability/tracing"

	"go.uber.org/zap"
)

// AgentInput represents the input to an agent.
type AgentInput struct {
	TaskID      string
	Goal        string
	Constraints []string
	RepoURL     string
	BaseBranch  string
	Context     map[string]any
}

// AgentOutput represents the output from an agent.
type AgentOutput struct {
	Summary     string         `json:"summary"`
	Artifacts   []Artifact     `json:"artifacts"`
	Suggestions []string       `json:"suggestions"`
	Status      AgentStatus    `json:"status"`
	Error       error          `json:"error,omitempty"`
}

// Artifact represents a code artifact produced by an agent.
type Artifact struct {
	Type    string `json:"type"`    // "file", "code", "analysis"
	Content string `json:"content"`
	Path    string `json:"path,omitempty"`
	Lang    string `json:"lang,omitempty"`
}

// AgentStatus represents the status of an agent execution.
type AgentStatus string

const (
	AgentStatusSuccess AgentStatus = "success"
	AgentStatusFailed AgentStatus = "failed"
	AgentStatusPartial AgentStatus = "partial"
)

// BaseAgent provides common functionality for all agents.
type BaseAgent struct {
	LLM          react.LLM
	ToolRegistry *react.ToolRegistry
	Logger       *zap.Logger
	Tracer       *tracing.SpanHelper
	Model        string
	Temperature  float64
	MaxTokens    int
}

// NewBaseAgent creates a new BaseAgent with default settings.
func NewBaseAgent(llm react.LLM, toolRegistry *react.ToolRegistry, logger *zap.Logger) *BaseAgent {
	return &BaseAgent{
		LLM:          llm,
		ToolRegistry: toolRegistry,
		Logger:       logger,
		Tracer:       tracing.NewSpanHelper(),
		Model:        "claude-sonnet",
		Temperature:  0.7,
		MaxTokens:    4096,
	}
}

// callLLM invokes the LLM with the given prompt and options.
func (b *BaseAgent) callLLM(ctx context.Context, prompt string) (string, error) {
	if b.LLM == nil {
		return "", fmt.Errorf("LLM not configured")
	}

	messages := []*react.Message{
		{Role: react.RoleSystem, Content: prompt},
	}

	result, err := b.LLM.Generate(ctx, messages, &react.GenerateOptions{
		MaxTokens:   b.MaxTokens,
		Temperature: b.Temperature,
	})
	if err != nil {
		return "", fmt.Errorf("LLM call failed: %w", err)
	}

	return result.Content, nil
}

// ObserverPromptTemplate is the prompt template for the Observer agent.
const ObserverPromptTemplate = `You are an OBSERVER agent. Your role is to analyze code structure, identify dependencies, and assess impact.

RESPONSIBILITIES:
- Analyze the codebase structure and organization
- Identify key dependencies and relationships
- Assess potential impact areas of changes
- Provide insights on code quality and patterns
- Research relevant context for the task

CAPABILITIES:
- Analysis: Level 5 (Expert)
- Research: Level 4 (Advanced)

OUTPUT FORMAT:
Return your analysis in this structure:
{
  "summary": "<brief summary of findings>",
  "artifacts": [
    {"type": "analysis", "content": "<detailed analysis>", "lang": "markdown"}
  ],
  "suggestions": ["<suggestion 1>", "<suggestion 2>"]
}

Be thorough but concise. Focus on actionable insights.`

// ObserverAgent observes and analyzes code structure for complex tasks.
// It receives the full goal context and provides initial analysis.
type ObserverAgent struct {
	base *BaseAgent
}

// NewObserverAgent creates a new ObserverAgent.
func NewObserverAgent(llm react.LLM, toolRegistry *react.ToolRegistry, logger *zap.Logger) *ObserverAgent {
	return &ObserverAgent{
		base: NewBaseAgent(llm, toolRegistry, logger),
	}
}

// Run executes the Observer agent with the given input.
func (a *ObserverAgent) Run(ctx context.Context, input *AgentInput) (*AgentOutput, error) {
	if a.base.Logger != nil {
		a.base.Logger.Info("ObserverAgent starting",
			zap.String("task_id", input.TaskID),
			zap.String("goal", truncateForLog(input.Goal)))
	}

	// Start tracing span
	ctx, span := a.base.Tracer.StartAgentAct(ctx, input.TaskID, "observer", "analyze")
	defer func() {
		if span != nil {
			tracing.EndSpan(span)
		}
	}()

	// Build the prompt with input context
	prompt := a.buildPrompt(input)

	// Call LLM
	summary, err := a.base.callLLM(ctx, prompt)
	if err != nil {
		if a.base.Logger != nil {
			a.base.Logger.Error("ObserverAgent LLM call failed", zap.Error(err))
		}
		return &AgentOutput{
			Status: AgentStatusFailed,
			Error:  err,
		}, err
	}

	output := &AgentOutput{
		Summary: summary,
		Artifacts: []Artifact{
			{
				Type:    "analysis",
				Content: summary,
				Lang:    "markdown",
			},
		},
		Suggestions: a.extractSuggestions(summary),
		Status:      AgentStatusSuccess,
	}

	if a.base.Logger != nil {
		a.base.Logger.Info("ObserverAgent completed",
			zap.String("task_id", input.TaskID))
	}

	return output, nil
}

// buildPrompt constructs the full prompt for the Observer agent.
func (a *ObserverAgent) buildPrompt(input *AgentInput) string {
	return fmt.Sprintf(`%s

TASK CONTEXT:
Task ID: %s
Goal: %s
Constraints: %v
Repo URL: %s
Base Branch: %s`,
		ObserverPromptTemplate,
		input.TaskID,
		input.Goal,
		input.Constraints,
		input.RepoURL,
		input.BaseBranch,
	)
}

// extractSuggestions extracts suggestions from the LLM response.
func (a *ObserverAgent) extractSuggestions(content string) []string {
	// Simple extraction - in production this would parse structured output
	var suggestions []string
	lines := splitLines(content)
	for _, line := range lines {
		if containsLower(line, "suggestion") || containsLower(line, "recommend") {
			suggestions = append(suggestions, trimLine(line))
		}
	}
	return suggestions
}

// truncateForLog truncates a string for logging purposes.
func truncateForLog(s string) string {
	if len(s) > 100 {
		return s[:100] + "..."
	}
	return s
}

// splitLines splits a string into lines.
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// containsLower checks if s contains substr (case-insensitive).
func containsLower(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	sLower := toLower(s)
	substrLower := toLower(substr)
	for i := 0; i <= len(sLower)-len(substrLower); i++ {
		if sLower[i:i+len(substrLower)] == substrLower {
			return true
		}
	}
	return false
}

// toLower converts a string to lowercase.
func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		result[i] = c
	}
	return string(result)
}

// trimLine removes common prefixes and whitespace from a line.
func trimLine(s string) string {
	s = trimSpace(s)
	prefixes := []string{"- ", "* ", "> ", "1. ", "2. ", "3. "}
	for _, p := range prefixes {
		if len(s) >= len(p) && s[:len(p)] == p {
			s = s[len(p):]
		}
	}
	return trimSpace(s)
}

// trimSpace removes leading and trailing whitespace.
func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

// Verify ObserverAgent implements the expected interface.
var _ = interface{}((*ObserverAgent)(nil))