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

// StrategistPromptTemplate is the prompt template for the Strategist agent.
const StrategistPromptTemplate = `You are a STRATEGIST agent. Your role is to develop modification strategies and design implementation plans.

RESPONSIBILITIES:
- Create detailed execution plans based on analysis
- Break down complex tasks into manageable subtasks
- Identify dependencies between subtasks
- Estimate complexity and potential risks
- Design implementation approach and sequence

CAPABILITIES:
- Analysis: Level 5 (Expert)
- Coding: Level 4 (Advanced)

OUTPUT FORMAT:
Return your strategy in this structure:
{
  "summary": "<brief summary of the strategy>",
  "artifacts": [
    {"type": "plan", "content": "<detailed execution plan>", "lang": "markdown"}
  ],
  "suggestions": ["<risk 1>", "<risk 2>", "<mitigation 1>"]
}

Focus on actionable, sequential plans. Identify potential failure points.`

// StrategistAgent develops execution strategies for complex tasks.
// It receives Observer's analysis and creates a detailed implementation plan.
type StrategistAgent struct {
	base *BaseAgent
}

// NewStrategistAgent creates a new StrategistAgent.
func NewStrategistAgent(llm react.LLM, toolRegistry *react.ToolRegistry, logger *zap.Logger) *StrategistAgent {
	return &StrategistAgent{
		base: NewBaseAgent(llm, toolRegistry, logger),
	}
}

// Run executes the Strategist agent with the given input.
func (a *StrategistAgent) Run(ctx context.Context, input *AgentInput) (*AgentOutput, error) {
	if a.base.Logger != nil {
		a.base.Logger.Info("StrategistAgent starting",
			zap.String("task_id", input.TaskID),
			zap.String("goal", truncateForLog(input.Goal)))
	}

	// Start tracing span
	ctx, span := a.base.Tracer.StartAgentAct(ctx, input.TaskID, "strategist", "plan")
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
			a.base.Logger.Error("StrategistAgent LLM call failed", zap.Error(err))
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
				Type:    "plan",
				Content: summary,
				Lang:    "markdown",
			},
		},
		Suggestions: a.extractRisks(summary),
		Status:      AgentStatusSuccess,
	}

	if a.base.Logger != nil {
		a.base.Logger.Info("StrategistAgent completed",
			zap.String("task_id", input.TaskID))
	}

	return output, nil
}

// buildPrompt constructs the full prompt for the Strategist agent.
func (a *StrategistAgent) buildPrompt(input *AgentInput) string {
	// Include observer analysis if available
	observerAnalysis := ""
	if input.Context != nil {
		if analysis, ok := input.Context["observer_analysis"].(string); ok {
			observerAnalysis = fmt.Sprintf("\n\nOBSERVER ANALYSIS:\n%s", analysis)
		}
	}

	return fmt.Sprintf(`%s%s

TASK CONTEXT:
Task ID: %s
Goal: %s
Constraints: %v
Repo URL: %s
Base Branch: %s`,
		StrategistPromptTemplate,
		observerAnalysis,
		input.TaskID,
		input.Goal,
		input.Constraints,
		input.RepoURL,
		input.BaseBranch,
	)
}

// extractRisks extracts risks and mitigations from the strategy.
func (a *StrategistAgent) extractRisks(content string) []string {
	// Simple extraction - in production this would parse structured output
	var risks []string
	lines := splitLines(content)
	inRiskSection := false
	for _, line := range lines {
		if containsLower(line, "risk") || containsLower(line, "mitigation") {
			inRiskSection = true
		}
		if inRiskSection && (containsLower(line, "-") || containsLower(line, "*")) {
			risks = append(risks, trimLine(line))
		}
	}
	return risks
}

// Verify StrategistAgent implements the expected interface.
var _ = interface{}((*StrategistAgent)(nil))