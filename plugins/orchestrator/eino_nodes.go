// Package orchestrator implements Eino-based task orchestration graph.
// Responsible for task decomposition, agent matching, and execution scheduling.
package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloud-agent-platform/cap/internal/agent/react"
	"github.com/cloud-agent-platform/cap/internal/domain"
	"github.com/cloud-agent-platform/cap/internal/observability/tracing"

	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

// ----------------------------------------------------------------------------
// Unified Node Interface
// ----------------------------------------------------------------------------

// Node is the unified interface for all Eino graph nodes.
// Every node must implement Execute, Name, and Description.
type Node interface {
	// Execute processes the input and returns the output.
	// It receives context and input data, returns result or error.
	Execute(ctx context.Context, input *TaskContext) (*TaskContext, error)
	// Name returns the node's unique identifier.
	Name() string
	// Description returns a human-readable description of the node.
	Description() string
}

// NodeBase provides common functionality for all nodes.
type NodeBase struct {
	name        string
	description string
}

// Name implements Node.
func (n *NodeBase) Name() string { return n.name }

// Description implements Node.
func (n *NodeBase) Description() string { return n.description }

// ----------------------------------------------------------------------------
// AnalyzerNode
// Analyzes task description and extracts complexity metrics.
// ----------------------------------------------------------------------------

// EinoAnalyzerNode analyzes task complexity and prepares context.
// This is the unified Eino node implementation.
type EinoAnalyzerNode struct {
	*NodeBase
	deps *Dependencies
}

// NewEinoAnalyzerNode creates a new EinoAnalyzerNode.
func NewEinoAnalyzerNode(deps *Dependencies) *EinoAnalyzerNode {
	return &EinoAnalyzerNode{
		NodeBase: &NodeBase{
			name:        "analyzer",
			description: "Analyzes task description and extracts complexity metrics",
		},
		deps: deps,
	}
}

// Execute implements Node.
// It analyzes the task input and determines complexity level.
func (n *EinoAnalyzerNode) Execute(ctx context.Context, input *TaskContext) (*TaskContext, error) {
	if input == nil {
		return nil, fmt.Errorf("EinoAnalyzerNode: input is nil")
	}

	if n.deps.Logger != nil {
		n.deps.Logger.Info("AnalyzerNode processing task",
			zap.String("task_id", input.TaskID),
			zap.String("goal", truncateString(input.Goal, 100)))
	}

	tc := &TaskContext{
		TaskID:      input.TaskID,
		Goal:        input.Goal,
		Constraints: input.Constraints,
		RepoURL:     input.RepoURL,
		BaseBranch:  input.BaseBranch,
		Complexity:  n.evaluateComplexity(input),
	}

	if n.deps.Logger != nil {
		n.deps.Logger.Info("AnalyzerNode determined complexity",
			zap.String("task_id", input.TaskID),
			zap.String("complexity", string(tc.Complexity)))
	}

	return tc, nil
}

// evaluateComplexity determines task complexity based on goal analysis.
func (n *EinoAnalyzerNode) evaluateComplexity(input *TaskContext) TaskComplexity {
	goal := strings.ToLower(input.Goal)

	// Simple task indicators: single file, small scope, no dependencies mentioned
	simplePatterns := []string{
		"fix typo", "update comment", "rename variable", "format code",
		"add import", "remove debug", "change string", "single file",
		"fix bug", "hotfix", "quick fix",
	}

	for _, pattern := range simplePatterns {
		if strings.Contains(goal, pattern) {
			return ComplexitySimple
		}
	}

	// Check for complex task indicators: architecture, multiple roles, patterns
	complexPatterns := []string{
		"architecture", "refactor", "design pattern", "multi-module",
		"distributed", "microservice", "performance optimization",
		"security audit", "code review", "design system",
		"rearchitect", "redesign", "system design", "microservices",
	}

	for _, pattern := range complexPatterns {
		if strings.Contains(goal, pattern) {
			return ComplexityComplex
		}
	}

	// Check for medium indicators: multiple files, modules, features
	mediumPatterns := []string{
		"implement feature", "add functionality", "create module",
		"build component", "develop feature", "multiple files",
		"api endpoint", "rest api", "database migration",
		"add feature", "new feature", "create feature",
	}

	for _, pattern := range mediumPatterns {
		if strings.Contains(goal, pattern) {
			return ComplexityMedium
		}
	}

	// Default to medium complexity
	return ComplexityMedium
}

// Verify EinoAnalyzerNode implements Node.
var _ Node = (*EinoAnalyzerNode)(nil)

// ----------------------------------------------------------------------------
// RouterNode
// Routes to 3 paths (simple/medium/complex) based on AnalyzerNode output.
// ----------------------------------------------------------------------------

// EinoRouterNode performs 3-way routing based on complexity level.
type EinoRouterNode struct {
	*NodeBase
	deps *Dependencies
}

// NewEinoRouterNode creates a new EinoRouterNode.
func NewEinoRouterNode(deps *Dependencies) *EinoRouterNode {
	return &EinoRouterNode{
		NodeBase: &NodeBase{
			name:        "router",
			description: "Routes to simple/medium/complex execution path based on task complexity",
		},
		deps: deps,
	}
}

// Execute implements Node.
// It examines the complexity field and returns the appropriate path name.
func (n *EinoRouterNode) Execute(ctx context.Context, input *TaskContext) (*TaskContext, error) {
	if input == nil {
		return nil, fmt.Errorf("EinoRouterNode: input is nil")
	}

	if n.deps.Logger != nil {
		n.deps.Logger.Info("RouterNode routing",
			zap.String("task_id", input.TaskID),
			zap.String("complexity", string(input.Complexity)))
	}

	// The routing is determined by the complexity field
	// This node passes through the context, but logs the routing decision
	// The actual routing happens in the graph branch

	return input, nil
}

// GetRoute returns the route name based on complexity.
// This is a helper method for external callers.
func (n *EinoRouterNode) GetRoute(ctx context.Context, input *TaskContext) (string, error) {
	if input == nil {
		return "", fmt.Errorf("EinoRouterNode: input is nil")
	}

	switch input.Complexity {
	case ComplexitySimple:
		return "simple", nil
	case ComplexityMedium:
		return "medium", nil
	case ComplexityComplex:
		return "complex", nil
	default:
		return "simple", nil
	}
}

// Verify EinoRouterNode implements Node.
var _ Node = (*EinoRouterNode)(nil)

// ----------------------------------------------------------------------------
// SimpleExecNode
// Executes simple tasks directly without decomposition.
// ----------------------------------------------------------------------------

// EinoSimpleExecNode executes simple tasks directly.
type EinoSimpleExecNode struct {
	*NodeBase
	deps *Dependencies
}

// NewEinoSimpleExecNode creates a new EinoSimpleExecNode.
func NewEinoSimpleExecNode(deps *Dependencies) *EinoSimpleExecNode {
	return &EinoSimpleExecNode{
		NodeBase: &NodeBase{
			name:        "simple_exec",
			description: "Executes simple tasks directly without decomposition",
		},
		deps: deps,
	}
}

// getTracer returns the tracer helper from dependencies or a default one.
func (n *EinoSimpleExecNode) getTracer() *tracing.SpanHelper {
	if n.deps.Tracer != nil {
		return n.deps.Tracer
	}
	return tracing.NewSpanHelper()
}

// Execute implements Node.
// For simple tasks, execution is done directly with the LLM.
func (n *EinoSimpleExecNode) Execute(ctx context.Context, input *TaskContext) (*TaskContext, error) {
	if input == nil {
		return nil, fmt.Errorf("EinoSimpleExecNode: input is nil")
	}

	if n.deps.Logger != nil {
		n.deps.Logger.Info("EinoSimpleExecNode executing",
			zap.String("task_id", input.TaskID),
			zap.String("goal", truncateString(input.Goal, 50)))
	}

	tracer := n.getTracer()

	ctx, actSpan := tracer.StartAgentAct(ctx, input.TaskID, "simple_exec", "execute_simple_task")
	defer func() {
		if actSpan != nil {
			tracing.EndSpan(actSpan)
		}
	}()

	if n.deps.LLM != nil {
		messages := []*react.Message{
			{Role: react.RoleSystem, Content: "You are a code executor. Execute the following task and provide a summary."},
			{Role: react.RoleUser, Content: input.Goal},
		}

		ctx, llmSpan := tracer.StartLLMCall(ctx, input.TaskID, "claude-sonnet", len(input.Goal))
		defer func() {
			if llmSpan != nil {
				tracing.EndSpan(llmSpan)
			}
		}()

		result, err := n.deps.LLM.Generate(ctx, messages, &react.GenerateOptions{
			MaxTokens:   4096,
			Temperature: 0.7,
		})

		if err != nil {
			if n.deps.Logger != nil {
				n.deps.Logger.Error("EinoSimpleExecNode LLM error", zap.Error(err))
			}
			tracing.EndSpanWithError(llmSpan, err)
			tracing.EndSpanWithError(actSpan, err)
			return input, err
		}

		input.Summary = result.Content
		input.TokensUsed += result.TotalTokens
		llmSpan.SetAttributes(attribute.Int(tracing.AttrLLMTokens, result.TotalTokens))
	}

	input.Progress = 100.0

	if n.deps.Logger != nil {
		n.deps.Logger.Info("EinoSimpleExecNode completed",
			zap.String("task_id", input.TaskID))
	}

	return input, nil
}

// Verify EinoSimpleExecNode implements Node.
var _ Node = (*EinoSimpleExecNode)(nil)

// ----------------------------------------------------------------------------
// ReActNode
// ReAct loop execution for medium complexity tasks.
// ----------------------------------------------------------------------------

// EinoReActNode wraps the ReAct agent for medium task execution.
type EinoReActNode struct {
	*NodeBase
	deps           *Dependencies
	maxIterations  int
	maxExecutionTime time.Duration
}

// NewEinoReActNode creates a new EinoReActNode.
func NewEinoReActNode(deps *Dependencies) *EinoReActNode {
	return &EinoReActNode{
		NodeBase: &NodeBase{
			name:        "react",
			description: "ReAct loop execution for medium complexity tasks",
		},
		deps:            deps,
		maxIterations:   10,
		maxExecutionTime: 5 * time.Minute,
	}
}

// NewEinoReActNodeWithConfig creates a new EinoReActNode with custom configuration.
func NewEinoReActNodeWithConfig(deps *Dependencies, maxIterations int, maxExecutionTime time.Duration) *EinoReActNode {
	return &EinoReActNode{
		NodeBase: &NodeBase{
			name:        "react",
			description: "ReAct loop execution for medium complexity tasks",
		},
		deps:            deps,
		maxIterations:   maxIterations,
		maxExecutionTime: maxExecutionTime,
	}
}

// Execute implements Node.
// It runs the ReAct loop for medium complexity tasks.
func (n *EinoReActNode) Execute(ctx context.Context, input *TaskContext) (*TaskContext, error) {
	if input == nil {
		return nil, fmt.Errorf("EinoReActNode: input is nil")
	}

	if n.deps.Logger != nil {
		n.deps.Logger.Info("EinoReActNode starting ReAct loop",
			zap.String("task_id", input.TaskID),
			zap.Int("max_iterations", n.maxIterations),
			zap.Duration("max_execution_time", n.maxExecutionTime))
	}

	tracer := n.getTracer()

	ctx, actSpan := tracer.StartAgentAct(ctx, input.TaskID, "react", "react_loop")
	defer func() {
		if actSpan != nil {
			tracing.EndSpan(actSpan)
		}
	}()

	if n.deps.LLM == nil {
		if n.deps.Logger != nil {
			n.deps.Logger.Warn("ReActNode: LLM not available, skipping execution")
		}
		input.Progress = 100.0
		return input, nil
	}

	// Build the ReAct prompt
	systemPrompt := `You are a ReAct agent. For the given task:
1. Think about what needs to be done
2. Take actions using available tools when needed
3. Observe the results of your actions
4. Continue until you have a final answer

Format your responses as:
Thought: <your reasoning>
Action: <tool_name> if taking action, or "final_answer" if done
ActionArgs: <arguments to the tool, or final answer>
Observation: <result of action> (if action was taken)`

	messages := []*react.Message{
		{Role: react.RoleSystem, Content: systemPrompt},
		{Role: react.RoleUser, Content: input.Goal},
	}

	// Run the ReAct loop manually since we need intermediate steps
	var totalTokens int
	for iteration := 0; iteration < n.maxIterations; iteration++ {
		select {
		case <-ctx.Done():
			if n.deps.Logger != nil {
				n.deps.Logger.Info("ReActNode context cancelled",
					zap.String("task_id", input.TaskID),
					zap.Int("iteration", iteration))
			}
			return input, ctx.Err()
		default:
		}

		// Check execution timeout
		if n.maxExecutionTime > 0 {
			// This is a simplified check - in production you'd track actual elapsed time
		}

		result, err := n.deps.LLM.Generate(ctx, messages, &react.GenerateOptions{
			MaxTokens:   4096,
			Temperature: 0.7,
		})

		if err != nil {
			if n.deps.Logger != nil {
				n.deps.Logger.Error("ReActNode LLM error",
					zap.Error(err),
					zap.Int("iteration", iteration))
			}
			tracing.EndSpanWithError(actSpan, err)
			return input, err
		}

		totalTokens += result.TotalTokens
		content := result.Content

		// Parse the response to check if we have a final answer
		if strings.Contains(content, "final_answer") || strings.Contains(content, "Final Answer") {
			// Extract the final answer
			lines := strings.Split(content, "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "Action:") && strings.Contains(line, "final_answer") {
					// Found final answer marker
					input.Summary = content
					input.TokensUsed = totalTokens
					input.Progress = 100.0
					if n.deps.Logger != nil {
						n.deps.Logger.Info("ReActNode completed with final answer",
							zap.String("task_id", input.TaskID),
							zap.Int("iterations", iteration+1),
							zap.Int("total_tokens", totalTokens))
					}
					return input, nil
				}
			}
		}

		// Add assistant message
		messages = append(messages, &react.Message{
			Role:    react.RoleAssistant,
			Content: content,
		})

		// In a full implementation, we would parse actions and execute them
		// For now, we simulate the loop and produce a result
		if iteration == n.maxIterations-1 {
			input.Summary = content
			input.TokensUsed = totalTokens
			input.Progress = 100.0
		}
	}

	if n.deps.Logger != nil {
		n.deps.Logger.Info("ReActNode max iterations reached",
			zap.String("task_id", input.TaskID),
			zap.Int("max_iterations", n.maxIterations))
	}

	return input, nil
}

// getTracer returns the tracer helper from dependencies or a default one.
func (n *EinoReActNode) getTracer() *tracing.SpanHelper {
	if n.deps.Tracer != nil {
		return n.deps.Tracer
	}
	return tracing.NewSpanHelper()
}

// Verify EinoReActNode implements Node.
var _ Node = (*EinoReActNode)(nil)

// ----------------------------------------------------------------------------
// DecomposerNode
// Decomposes complex tasks into subtasks.
// ----------------------------------------------------------------------------

// EinoDecomposerNode decomposes complex tasks into subtask plans.
type EinoDecomposerNode struct {
	*NodeBase
	deps *Dependencies
}

// NewEinoDecomposerNode creates a new EinoDecomposerNode.
func NewEinoDecomposerNode(deps *Dependencies) *EinoDecomposerNode {
	return &EinoDecomposerNode{
		NodeBase: &NodeBase{
			name:        "decomposer",
			description: "Decomposes complex tasks into subtask plans",
		},
		deps: deps,
	}
}

// Execute implements Node.
// It decomposes the task into subtasks based on the goal.
func (n *EinoDecomposerNode) Execute(ctx context.Context, input *TaskContext) (*TaskContext, error) {
	if input == nil {
		return nil, fmt.Errorf("EinoDecomposerNode: input is nil")
	}

	if n.deps.Logger != nil {
		n.deps.Logger.Info("EinoDecomposerNode decomposing task",
			zap.String("task_id", input.TaskID))
	}

	tracer := n.getTracer()

	ctx, decomposeSpan := tracer.StartTaskDecompose(ctx, input.TaskID, 0)
	defer func() {
		if decomposeSpan != nil {
			tracing.EndSpan(decomposeSpan)
		}
	}()

	// Analyze and decompose into subtasks
	subtasks := n.createSubtasks(input)

	input.Subtasks = subtasks
	input.Progress = 20.0

	decomposeSpan.SetAttributes(attribute.Int(tracing.AttrSubtaskCount, len(subtasks)))

	if n.deps.Logger != nil {
		n.deps.Logger.Info("EinoDecomposerNode created subtasks",
			zap.String("task_id", input.TaskID),
			zap.Int("subtask_count", len(subtasks)))
	}

	return input, nil
}

// createSubtasks creates subtask plans from the goal.
func (n *EinoDecomposerNode) createSubtasks(input *TaskContext) []*SubtaskPlan {
	var subtasks []*SubtaskPlan

	goal := strings.ToLower(input.Goal)

	// Analyze goal to determine decomposition strategy
	if strings.Contains(goal, "api") || strings.Contains(goal, "endpoint") {
		// API-focused decomposition
		subtasks = append(subtasks, &SubtaskPlan{
			ID:          domain.NewULID(),
			Description: "Design API specification",
			Type:        domain.SubtaskTypeAnalysis,
			AgentRole:   domain.AgentRoleStrategist,
		})
		subtasks = append(subtasks, &SubtaskPlan{
			ID:          domain.NewULID(),
			Description: "Implement API endpoint",
			Type:        domain.SubtaskTypeCoding,
			Dependencies: []string{subtasks[0].ID},
			AgentRole:   domain.AgentRoleExecutor,
		})
		subtasks = append(subtasks, &SubtaskPlan{
			ID:          domain.NewULID(),
			Description: "Write tests for API",
			Type:        domain.SubtaskTypeTesting,
			Dependencies: []string{subtasks[1].ID},
			AgentRole:   domain.AgentRoleTester,
		})
	} else if strings.Contains(goal, "module") || strings.Contains(goal, "component") {
		// Module/component decomposition
		subtasks = append(subtasks, &SubtaskPlan{
			ID:          domain.NewULID(),
			Description: "Design module architecture",
			Type:        domain.SubtaskTypeAnalysis,
			AgentRole:   domain.AgentRoleStrategist,
		})
		subtasks = append(subtasks, &SubtaskPlan{
			ID:          domain.NewULID(),
			Description: "Implement core module",
			Type:        domain.SubtaskTypeCoding,
			Dependencies: []string{subtasks[0].ID},
			AgentRole:   domain.AgentRoleExecutor,
		})
		subtasks = append(subtasks, &SubtaskPlan{
			ID:          domain.NewULID(),
			Description: "Review implementation",
			Type:        domain.SubtaskTypeReview,
			Dependencies: []string{subtasks[1].ID},
			AgentRole:   domain.AgentRoleGuardian,
		})
		subtasks = append(subtasks, &SubtaskPlan{
			ID:          domain.NewULID(),
			Description: "Test module",
			Type:        domain.SubtaskTypeTesting,
			Dependencies: []string{subtasks[2].ID},
			AgentRole:   domain.AgentRoleTester,
		})
	} else if strings.Contains(goal, "database") || strings.Contains(goal, "migration") {
		// Database-focused decomposition
		subtasks = append(subtasks, &SubtaskPlan{
			ID:          domain.NewULID(),
			Description: "Research database requirements and best practices",
			Type:        domain.SubtaskTypeResearch,
			AgentRole:   domain.AgentRoleResearcher,
		})
		subtasks = append(subtasks, &SubtaskPlan{
			ID:          domain.NewULID(),
			Description: "Design database schema",
			Type:        domain.SubtaskTypeAnalysis,
			Dependencies: []string{subtasks[0].ID},
			AgentRole:   domain.AgentRoleStrategist,
		})
		subtasks = append(subtasks, &SubtaskPlan{
			ID:          domain.NewULID(),
			Description: "Implement database migration",
			Type:        domain.SubtaskTypeCoding,
			Dependencies: []string{subtasks[1].ID},
			AgentRole:   domain.AgentRoleExecutor,
		})
		subtasks = append(subtasks, &SubtaskPlan{
			ID:          domain.NewULID(),
			Description: "Verify data integrity",
			Type:        domain.SubtaskTypeTesting,
			Dependencies: []string{subtasks[2].ID},
			AgentRole:   domain.AgentRoleTester,
		})
	} else {
		// Default: basic decomposition for complex tasks
		subtasks = append(subtasks, &SubtaskPlan{
			ID:          domain.NewULID(),
			Description: "Research and analyze requirements",
			Type:        domain.SubtaskTypeResearch,
			AgentRole:   domain.AgentRoleResearcher,
		})
		subtasks = append(subtasks, &SubtaskPlan{
			ID:          domain.NewULID(),
			Description: "Design solution",
			Type:        domain.SubtaskTypeAnalysis,
			Dependencies: []string{subtasks[0].ID},
			AgentRole:   domain.AgentRoleStrategist,
		})
		subtasks = append(subtasks, &SubtaskPlan{
			ID:          domain.NewULID(),
			Description: "Implement solution",
			Type:        domain.SubtaskTypeCoding,
			Dependencies: []string{subtasks[1].ID},
			AgentRole:   domain.AgentRoleExecutor,
		})
		subtasks = append(subtasks, &SubtaskPlan{
			ID:          domain.NewULID(),
			Description: "Review implementation",
			Type:        domain.SubtaskTypeReview,
			Dependencies: []string{subtasks[2].ID},
			AgentRole:   domain.AgentRoleGuardian,
		})
		subtasks = append(subtasks, &SubtaskPlan{
			ID:          domain.NewULID(),
			Description: "Test implementation",
			Type:        domain.SubtaskTypeTesting,
			Dependencies: []string{subtasks[3].ID},
			AgentRole:   domain.AgentRoleTester,
		})
	}

	return subtasks
}

// getTracer returns the tracer helper from dependencies or a default one.
func (n *EinoDecomposerNode) getTracer() *tracing.SpanHelper {
	if n.deps.Tracer != nil {
		return n.deps.Tracer
	}
	return tracing.NewSpanHelper()
}

// Verify EinoDecomposerNode implements Node.
var _ Node = (*EinoDecomposerNode)(nil)

// ----------------------------------------------------------------------------
// EinoAggregatorNode
// Aggregates subtask results into final output.
// ----------------------------------------------------------------------------

// EinoAggregatorNode aggregates subtask results into final task result.
type EinoAggregatorNode struct {
	*NodeBase
	deps *Dependencies
}

// NewEinoAggregatorNode creates a new EinoAggregatorNode.
func NewEinoAggregatorNode(deps *Dependencies) *EinoAggregatorNode {
	return &EinoAggregatorNode{
		NodeBase: &NodeBase{
			name:        "aggregator",
			description: "Aggregates subtask results into final output",
		},
		deps: deps,
	}
}

// Execute implements Node.
// It merges subtask results into the final task result.
func (n *EinoAggregatorNode) Execute(ctx context.Context, input *TaskContext) (*TaskContext, error) {
	if input == nil {
		return nil, fmt.Errorf("EinoAggregatorNode: input is nil")
	}

	if n.deps.Logger != nil {
		n.deps.Logger.Info("EinoAggregatorNode aggregating results",
			zap.String("task_id", input.TaskID),
			zap.Float64("progress", input.Progress))
	}

	// Aggregate subtask summaries
	var aggregatedSummary strings.Builder
	aggregatedSummary.WriteString(fmt.Sprintf("Task completed: %s\n\n", input.Goal))

	if len(input.Subtasks) > 0 {
		aggregatedSummary.WriteString(fmt.Sprintf("Subtasks (%d):\n", len(input.Subtasks)))
		for i, st := range input.Subtasks {
			status := st.Status
			if status == "" {
				status = "completed"
			}
			aggregatedSummary.WriteString(fmt.Sprintf("  %d. [%s] %s", i+1, status, st.Description))
			if st.Summary != "" {
				aggregatedSummary.WriteString(fmt.Sprintf("\n     Summary: %s", st.Summary))
			}
			aggregatedSummary.WriteString("\n")
		}
	}

	if input.Summary != "" {
		aggregatedSummary.WriteString(fmt.Sprintf("\nFinal Result:\n%s", input.Summary))
	}

	input.Summary = aggregatedSummary.String()

	if n.deps.Logger != nil {
		n.deps.Logger.Info("EinoAggregatorNode completed aggregation",
			zap.String("task_id", input.TaskID),
			zap.Int("subtask_count", len(input.Subtasks)))
	}

	return input, nil
}

// Verify EinoAggregatorNode implements Node.
var _ Node = (*EinoAggregatorNode)(nil)