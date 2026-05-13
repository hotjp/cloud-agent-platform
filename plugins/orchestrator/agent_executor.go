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

// ExecutorPromptTemplate is the prompt template for the Executor agent.
const ExecutorPromptTemplate = `You are an EXECUTOR agent. Your role is to execute specific code writing and modifications.

RESPONSIBILITIES:
- Implement code changes according to specifications
- Follow established patterns and conventions
- Handle errors gracefully with informative messages
- Report completion status with results
- Ensure code quality and adherence to standards

CAPABILITIES:
- Coding: Level 5 (Expert)

OUTPUT FORMAT:
Return your execution results in this structure:
{
  "summary": "<brief summary of changes made>",
  "artifacts": [
    {"type": "file", "content": "<file content or diff>", "path": "relative/path.go"},
    {"type": "code", "content": "<code snippet>", "lang": "go"}
  ],
  "suggestions": ["<next step 1>", "<next step 2>"]
}

Be precise. Only modify what is specified. Report any blockers clearly.`

// ExecutorAgent executes specific code writing and modifications.
// It receives the strategy from Strategist and performs the actual implementation.
type ExecutorAgent struct {
	base *BaseAgent
}

// NewExecutorAgent creates a new ExecutorAgent.
func NewExecutorAgent(llm react.LLM, toolRegistry *react.ToolRegistry, logger *zap.Logger) *ExecutorAgent {
	return &ExecutorAgent{
		base: NewBaseAgent(llm, toolRegistry, logger),
	}
}

// Run executes the Executor agent with the given input.
func (a *ExecutorAgent) Run(ctx context.Context, input *AgentInput) (*AgentOutput, error) {
	if a.base.Logger != nil {
		a.base.Logger.Info("ExecutorAgent starting",
			zap.String("task_id", input.TaskID),
			zap.String("goal", truncateForLog(input.Goal)))
	}

	// Start tracing span
	ctx, span := a.base.Tracer.StartAgentAct(ctx, input.TaskID, "executor", "execute")
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
			a.base.Logger.Error("ExecutorAgent LLM call failed", zap.Error(err))
		}
		return &AgentOutput{
			Status: AgentStatusFailed,
			Error:  err,
		}, err
	}

	// Extract artifacts (code changes) from the response
	artifacts := a.extractArtifacts(summary)

	output := &AgentOutput{
		Summary:     summary,
		Artifacts:   artifacts,
		Suggestions: a.extractNextSteps(summary),
		Status:      AgentStatusSuccess,
	}

	if a.base.Logger != nil {
		a.base.Logger.Info("ExecutorAgent completed",
			zap.String("task_id", input.TaskID),
			zap.Int("artifacts_count", len(artifacts)))
	}

	return output, nil
}

// buildPrompt constructs the full prompt for the Executor agent.
func (a *ExecutorAgent) buildPrompt(input *AgentInput) string {
	// Include strategist plan if available
	strategistPlan := ""
	if input.Context != nil {
		if plan, ok := input.Context["strategist_plan"].(string); ok {
			strategistPlan = fmt.Sprintf("\n\nSTRATEGIST PLAN:\n%s", plan)
		}
	}

	return fmt.Sprintf(`%s%s

TASK CONTEXT:
Task ID: %s
Goal: %s
Constraints: %v
Repo URL: %s
Base Branch: %s`,
		ExecutorPromptTemplate,
		strategistPlan,
		input.TaskID,
		input.Goal,
		input.Constraints,
		input.RepoURL,
		input.BaseBranch,
	)
}

// extractArtifacts extracts code artifacts from the execution response.
func (a *ExecutorAgent) extractArtifacts(content string) []Artifact {
	var artifacts []Artifact
	lines := splitLines(content)

	var currentArtifact *Artifact
	for _, line := range lines {
		if containsLower(line, "path:") || containsLower(line, "file:") {
			if currentArtifact != nil {
				artifacts = append(artifacts, *currentArtifact)
			}
			path := trimLine(line)
			path = stripPrefix(path, "path:")
			path = stripPrefix(path, "file:")
			currentArtifact = &Artifact{
				Type:    "file",
				Content: "",
				Path:    trimSpace(path),
			}
		} else if containsLower(line, "```") {
			if currentArtifact != nil && currentArtifact.Content != "" {
				artifacts = append(artifacts, *currentArtifact)
				currentArtifact = nil
			} else if currentArtifact == nil {
				currentArtifact = &Artifact{
					Type:    "code",
					Content: "",
				}
			}
		} else if currentArtifact != nil {
			if currentArtifact.Content != "" {
				currentArtifact.Content += "\n"
			}
			currentArtifact.Content += line
		}
	}

	if currentArtifact != nil {
		artifacts = append(artifacts, *currentArtifact)
	}

	// If no artifacts found, create one with the full content
	if len(artifacts) == 0 {
		artifacts = append(artifacts, Artifact{
			Type:    "code",
			Content: content,
		})
	}

	return artifacts
}

// extractNextSteps extracts suggested next steps from the response.
func (a *ExecutorAgent) extractNextSteps(content string) []string {
	var steps []string
	lines := splitLines(content)
	inNextSection := false
	for _, line := range lines {
		if containsLower(line, "next") || containsLower(line, "follow") || containsLower(line, "then") {
			inNextSection = true
		}
		if inNextSection && (containsLower(line, "-") || containsLower(line, "*") || containsLower(line, "1.")) {
			steps = append(steps, trimLine(line))
		}
	}
	return steps
}

// stripPrefix removes a prefix from a string if present.
func stripPrefix(s, prefix string) string {
	if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):]
	}
	return s
}

// Verify ExecutorAgent implements the expected interface.
var _ = interface{}((*ExecutorAgent)(nil))