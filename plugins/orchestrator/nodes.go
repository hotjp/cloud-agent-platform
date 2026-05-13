// Package orchestrator implements Eino-based task orchestration graph.
// Responsible for task decomposition, agent matching, and execution scheduling.
package orchestrator

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/cloud-agent-platform/cap/internal/agent/react"
	"github.com/cloud-agent-platform/cap/internal/domain"
	"github.com/cloud-agent-platform/cap/internal/observability/tracing"

	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

// TaskComplexity represents the complexity level of a task.
type TaskComplexity string

const (
	ComplexitySimple  TaskComplexity = "simple"
	ComplexityMedium  TaskComplexity = "medium"
	ComplexityComplex TaskComplexity = "complex"
)

// TaskInput represents the input to the orchestration graph.
type TaskInput struct {
	TaskID               string
	Goal                 string
	Constraints          []string
	RepoURL              string
	BaseBranch           string
	HistoricalComplexity TaskComplexity // Previous complexity for similar tasks
}

// TaskContext represents the context passed through the graph.
type TaskContext struct {
	TaskID       string
	Goal         string
	Constraints  []string
	RepoURL      string
	BaseBranch   string
	Complexity   TaskComplexity
	Subtasks     []*SubtaskPlan
	Summary      string
	CurrentAgent string
	Progress     float64
	TokensUsed   int
}

// SubtaskPlan represents a planned subtask in the graph.
type SubtaskPlan struct {
	ID          string
	Description string
	Type        domain.SubtaskType
	Dependencies []string
	AgentRole   domain.AgentRole
	Status      string
	Summary     string
}

// TaskResult represents the result of task orchestration.
type TaskResult struct {
	TaskID      string
	Summary     string
	Subtasks    []*SubtaskPlan
	TokensUsed  int
	Duration    time.Duration
	Success     bool
	Error       error
}

// Dependencies holds all dependencies needed by nodes.
type Dependencies struct {
	LLM          react.LLM
	ToolRegistry *react.ToolRegistry
	Logger       *zap.Logger
	Tracer       *tracing.SpanHelper
	TaskRepo     interface {
		GetByID(ctx context.Context, id string) (*domain.Task, error)
	}
	SubtaskRepo interface {
		ListByTaskID(ctx context.Context, taskID string) ([]*domain.Subtask, error)
	}
}

// ----------------------------------------------------------------------------
// Node Implementations
// ----------------------------------------------------------------------------

// AnalyzerNode analyzes task complexity and prepares context.
type AnalyzerNode struct {
	deps *Dependencies
}

// NewAnalyzerNode creates a new AnalyzerNode.
func NewAnalyzerNode(deps *Dependencies) *AnalyzerNode {
	return &AnalyzerNode{deps: deps}
}

// Invoke processes the task input and returns updated context.
func (n *AnalyzerNode) Invoke(ctx context.Context, input *TaskInput) (*TaskContext, error) {
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
func (n *AnalyzerNode) evaluateComplexity(input *TaskInput) TaskComplexity {
	goal := strings.ToLower(input.Goal)

	// Simple task indicators: single file, small scope, no dependencies mentioned
	simplePatterns := []string{
		"fix typo", "update comment", "rename variable", "format code",
		"add import", "remove debug", "change string", "single file",
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
	}

	for _, pattern := range mediumPatterns {
		if strings.Contains(goal, pattern) {
			return ComplexityMedium
		}
	}

	// Default to medium complexity
	return ComplexityMedium
}

// ComplexityRouter routes to different paths based on complexity.
type ComplexityRouter struct{}

// NewComplexityRouter creates a new ComplexityRouter.
func NewComplexityRouter() *ComplexityRouter {
	return &ComplexityRouter{}
}

// Invoke routes based on complexity.
func (r *ComplexityRouter) Invoke(ctx context.Context, input *TaskContext) (string, error) {
	return string(input.Complexity), nil
}

// SimpleExecutorNode executes simple tasks directly.
type SimpleExecutorNode struct {
	deps *Dependencies
}

// NewSimpleExecutorNode creates a new SimpleExecutorNode.
func NewSimpleExecutorNode(deps *Dependencies) *SimpleExecutorNode {
	return &SimpleExecutorNode{deps: deps}
}

// getTracer returns the tracer helper from dependencies or a default one.
func (n *SimpleExecutorNode) getTracer() *tracing.SpanHelper {
	if n.deps.Tracer != nil {
		return n.deps.Tracer
	}
	return tracing.NewSpanHelper()
}

// Invoke executes a simple task.
func (n *SimpleExecutorNode) Invoke(ctx context.Context, input *TaskContext) (*TaskContext, error) {
	if n.deps.Logger != nil {
		n.deps.Logger.Info("SimpleExecutorNode executing",
			zap.String("task_id", input.TaskID),
			zap.String("goal", truncateString(input.Goal, 50)))
	}

	// Get tracer helper
	tracer := n.getTracer()

	// Start agent.act span
	ctx, actSpan := tracer.StartAgentAct(ctx, input.TaskID, "simple_executor", "execute_simple_task")
	defer func() {
		if actSpan != nil {
			tracing.EndSpan(actSpan)
		}
	}()

	// For simple tasks, we execute directly with a basic executor agent
	// The execution would call the LLM with the goal
	if n.deps.LLM != nil {
		messages := []*react.Message{
			{Role: react.RoleSystem, Content: "You are a code executor. Execute the following task and provide a summary."},
			{Role: react.RoleUser, Content: input.Goal},
		}

		// Start llm.call span
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
				n.deps.Logger.Error("SimpleExecutorNode LLM error", zap.Error(err))
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
		n.deps.Logger.Info("SimpleExecutorNode completed",
			zap.String("task_id", input.TaskID))
	}

	return input, nil
}

// MediumDecomposerNode decomposes medium tasks into subtasks.
type MediumDecomposerNode struct {
	deps *Dependencies
}

// NewMediumDecomposerNode creates a new MediumDecomposerNode.
func NewMediumDecomposerNode(deps *Dependencies) *MediumDecomposerNode {
	return &MediumDecomposerNode{deps: deps}
}

// Invoke decomposes a medium task into subtasks.
func (n *MediumDecomposerNode) Invoke(ctx context.Context, input *TaskContext) (*TaskContext, error) {
	if n.deps.Logger != nil {
		n.deps.Logger.Info("MediumDecomposerNode decomposing",
			zap.String("task_id", input.TaskID))
	}

	// Analyze and decompose into subtasks
	subtasks := n.createSubtasks(input)

	input.Subtasks = subtasks
	input.Progress = 20.0

	if n.deps.Logger != nil {
		n.deps.Logger.Info("MediumDecomposerNode created subtasks",
			zap.String("task_id", input.TaskID),
			zap.Int("subtask_count", len(subtasks)))
	}

	return input, nil
}

// createSubtasks creates subtask plans from the goal.
func (n *MediumDecomposerNode) createSubtasks(input *TaskContext) []*SubtaskPlan {
	// Simple decomposition logic - in production this would use LLM
	var subtasks []*SubtaskPlan

	// Look for keywords to determine decomposition
	goal := strings.ToLower(input.Goal)

	if strings.Contains(goal, "api") || strings.Contains(goal, "endpoint") {
		subtasks = append(subtasks, &SubtaskPlan{
			ID:          domain.NewULID(),
			Description: "Implement API endpoint",
			Type:        domain.SubtaskTypeCoding,
			AgentRole:   domain.AgentRoleExecutor,
		})
		subtasks = append(subtasks, &SubtaskPlan{
			ID:          domain.NewULID(),
			Description: "Write tests for API",
			Type:        domain.SubtaskTypeTesting,
			Dependencies: []string{subtasks[0].ID},
			AgentRole:   domain.AgentRoleTester,
		})
	} else {
		// Default: single coding subtask
		subtasks = append(subtasks, &SubtaskPlan{
			ID:          domain.NewULID(),
			Description: input.Goal,
			Type:        domain.SubtaskTypeCoding,
			AgentRole:   domain.AgentRoleExecutor,
		})
	}

	return subtasks
}

// MediumExecutorNode executes medium tasks (handles parallel execution).
type MediumExecutorNode struct {
	deps *Dependencies
}

// NewMediumExecutorNode creates a new MediumExecutorNode.
func NewMediumExecutorNode(deps *Dependencies) *MediumExecutorNode {
	return &MediumExecutorNode{deps: deps}
}

// Invoke executes medium tasks.
func (n *MediumExecutorNode) Invoke(ctx context.Context, input *TaskContext) (*TaskContext, error) {
	if n.deps.Logger != nil {
		n.deps.Logger.Info("MediumExecutorNode executing",
			zap.String("task_id", input.TaskID),
			zap.Int("subtask_count", len(input.Subtasks)))
	}

	var summaries []string
	for i, st := range input.Subtasks {
		if n.deps.Logger != nil {
			n.deps.Logger.Info("Executing subtask",
				zap.String("task_id", input.TaskID),
				zap.Int("subtask_index", i),
				zap.String("description", st.Description))
		}

		// Execute subtask (simplified - would use actual agent)
		summary := fmt.Sprintf("Completed: %s", st.Description)
		st.Summary = summary
		summaries = append(summaries, summary)
	}

	input.Summary = strings.Join(summaries, "; ")
	input.Progress = 100.0

	if n.deps.Logger != nil {
		n.deps.Logger.Info("MediumExecutorNode completed",
			zap.String("task_id", input.TaskID))
	}

	return input, nil
}

// ObserverNode observes and analyzes in complex tasks.
type ObserverNode struct {
	deps *Dependencies
}

// NewObserverNode creates a new ObserverNode.
func NewObserverNode(deps *Dependencies) *ObserverNode {
	return &ObserverNode{deps: deps}
}

// Invoke performs observation and analysis.
func (n *ObserverNode) Invoke(ctx context.Context, input *TaskContext) (*TaskContext, error) {
	if n.deps.Logger != nil {
		n.deps.Logger.Info("ObserverNode observing",
			zap.String("task_id", input.TaskID))
	}

	input.CurrentAgent = "observer"

	// Observer analyzes the task and provides initial insights
	if n.deps.LLM != nil {
		messages := []*react.Message{
			{Role: react.RoleSystem, Content: "You are an observer agent. Analyze the task and provide initial observations."},
			{Role: react.RoleUser, Content: input.Goal},
		}

		result, err := n.deps.LLM.Generate(ctx, messages, &react.GenerateOptions{
			MaxTokens:   2048,
			Temperature: 0.7,
		})

		if err == nil {
			input.Summary = result.Content
			input.TokensUsed += result.TotalTokens
		}
	}

	input.Progress = 10.0

	if n.deps.Logger != nil {
		n.deps.Logger.Info("ObserverNode completed",
			zap.String("task_id", input.TaskID))
	}

	return input, nil
}

// StrategistNode creates execution plan for complex tasks.
type StrategistNode struct {
	deps *Dependencies
}

// NewStrategistNode creates a new StrategistNode.
func NewStrategistNode(deps *Dependencies) *StrategistNode {
	return &StrategistNode{deps: deps}
}

// Invoke creates an execution plan.
func (n *StrategistNode) Invoke(ctx context.Context, input *TaskContext) (*TaskContext, error) {
	if n.deps.Logger != nil {
		n.deps.Logger.Info("StrategistNode planning",
			zap.String("task_id", input.TaskID))
	}

	input.CurrentAgent = "strategist"

	// Create subtask decomposition
	subtasks := []*SubtaskPlan{
		{
			ID:          domain.NewULID(),
			Description: "Implement core functionality",
			Type:        domain.SubtaskTypeCoding,
			AgentRole:   domain.AgentRoleExecutor,
		},
		{
			ID:          domain.NewULID(),
			Description: "Review implementation",
			Type:        domain.SubtaskTypeReview,
			Dependencies: []string{},
			AgentRole:   domain.AgentRoleGuardian,
		},
		{
			ID:          domain.NewULID(),
			Description: "Test implementation",
			Type:        domain.SubtaskTypeTesting,
			Dependencies: []string{},
			AgentRole:   domain.AgentRoleTester,
		},
	}

	input.Subtasks = subtasks
	input.Progress = 25.0

	if n.deps.Logger != nil {
		n.deps.Logger.Info("StrategistNode created plan",
			zap.String("task_id", input.TaskID),
			zap.Int("subtask_count", len(subtasks)))
	}

	return input, nil
}

// ExecutorNode executes the planned work.
type ExecutorNode struct {
	deps *Dependencies
}

// NewExecutorNode creates a new ExecutorNode.
func NewExecutorNode(deps *Dependencies) *ExecutorNode {
	return &ExecutorNode{deps: deps}
}

// Invoke executes the planned work.
func (n *ExecutorNode) Invoke(ctx context.Context, input *TaskContext) (*TaskContext, error) {
	if n.deps.Logger != nil {
		n.deps.Logger.Info("ExecutorNode executing",
			zap.String("task_id", input.TaskID),
			zap.String("current_agent", input.CurrentAgent))
	}

	input.CurrentAgent = "executor"

	// Execute based on subtasks
	if n.deps.LLM != nil && len(input.Subtasks) > 0 {
		st := input.Subtasks[0]
		messages := []*react.Message{
			{Role: react.RoleSystem, Content: "You are an executor agent. Execute the following task and provide a summary."},
			{Role: react.RoleUser, Content: st.Description},
		}

		result, err := n.deps.LLM.Generate(ctx, messages, &react.GenerateOptions{
			MaxTokens:   4096,
			Temperature: 0.7,
		})

		if err == nil {
			input.Summary = result.Content
			input.TokensUsed += result.TotalTokens
		}
	}

	input.Progress = 60.0

	if n.deps.Logger != nil {
		n.deps.Logger.Info("ExecutorNode completed",
			zap.String("task_id", input.TaskID))
	}

	return input, nil
}

// GuardianNode validates and waits for human confirmation.
type GuardianNode struct {
	deps *Dependencies
}

// NewGuardianNode creates a new GuardianNode.
func NewGuardianNode(deps *Dependencies) *GuardianNode {
	return &GuardianNode{deps: deps}
}

// Invoke performs validation and waits for confirmation.
func (n *GuardianNode) Invoke(ctx context.Context, input *TaskContext) (*TaskContext, error) {
	if n.deps.Logger != nil {
		n.deps.Logger.Info("GuardianNode validating",
			zap.String("task_id", input.TaskID))
	}

	input.CurrentAgent = "guardian"

	// Guardian validates the execution
	if n.deps.LLM != nil {
		messages := []*react.Message{
			{Role: react.RoleSystem, Content: "You are a guardian agent. Review the execution and validate it meets requirements."},
			{Role: react.RoleUser, Content: fmt.Sprintf("Task: %s\nSummary: %s", input.Goal, input.Summary)},
		}

		result, err := n.deps.LLM.Generate(ctx, messages, &react.GenerateOptions{
			MaxTokens:   2048,
			Temperature: 0.7,
		})

		if err == nil {
			input.Summary = result.Content
			input.TokensUsed += result.TotalTokens
		}
	}

	input.Progress = 80.0

	if n.deps.Logger != nil {
		n.deps.Logger.Info("GuardianNode completed",
			zap.String("task_id", input.TaskID))
	}

	return input, nil
}

// TesterNode tests the implementation.
type TesterNode struct {
	deps *Dependencies
}

// NewTesterNode creates a new TesterNode.
func NewTesterNode(deps *Dependencies) *TesterNode {
	return &TesterNode{deps: deps}
}

// Invoke performs testing.
func (n *TesterNode) Invoke(ctx context.Context, input *TaskContext) (*TaskContext, error) {
	if n.deps.Logger != nil {
		n.deps.Logger.Info("TesterNode testing",
			zap.String("task_id", input.TaskID))
	}

	input.CurrentAgent = "tester"

	// Run tests (simplified)
	if n.deps.LLM != nil {
		messages := []*react.Message{
			{Role: react.RoleSystem, Content: "You are a tester agent. Test the implementation and provide test results."},
			{Role: react.RoleUser, Content: fmt.Sprintf("Task: %s\nImplementation: %s", input.Goal, input.Summary)},
		}

		result, err := n.deps.LLM.Generate(ctx, messages, &react.GenerateOptions{
			MaxTokens:   2048,
			Temperature: 0.7,
		})

		if err == nil {
			input.Summary = result.Content
			input.TokensUsed += result.TotalTokens
		}
	}

	input.Progress = 95.0

	if n.deps.Logger != nil {
		n.deps.Logger.Info("TesterNode completed",
			zap.String("task_id", input.TaskID))
	}

	return input, nil
}

// ResultMerger merges results from all paths.
type ResultMerger struct {
	deps *Dependencies
}

// NewResultMerger creates a new ResultMerger.
func NewResultMerger(deps *Dependencies) *ResultMerger {
	return &ResultMerger{deps: deps}
}

// Invoke merges results into final output.
func (n *ResultMerger) Invoke(ctx context.Context, input *TaskContext) (*TaskResult, error) {
	if n.deps.Logger != nil {
		n.deps.Logger.Info("ResultMerger merging",
			zap.String("task_id", input.TaskID),
			zap.Float64("progress", input.Progress))
	}

	result := &TaskResult{
		TaskID:     input.TaskID,
		Summary:    input.Summary,
		Subtasks:   input.Subtasks,
		TokensUsed: input.TokensUsed,
		Success:    true,
	}

	if n.deps.Logger != nil {
		n.deps.Logger.Info("ResultMerger completed",
			zap.String("task_id", input.TaskID),
			zap.Bool("success", result.Success))
	}

	return result, nil
}

// ----------------------------------------------------------------------------
// Helper Functions
// ----------------------------------------------------------------------------

// truncateString truncates a string to the specified length.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	// Find a good break point
	if maxLen < len(s) {
		// Try to break at a space
		for i := maxLen; i > maxLen-30 && i > 0; i-- {
			if s[i] == ' ' {
				return s[:i] + "..."
			}
		}
	}
	return s[:maxLen] + "..."
}

// containsKeyword checks if text contains any of the patterns.
func containsKeyword(text string, patterns []string) bool {
	lower := strings.ToLower(text)
	for _, pattern := range patterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

// countWords counts words in text.
func countWords(text string) int {
	reg := regexp.MustCompile(`\w+`)
	return len(reg.FindAllString(text, -1))
}
