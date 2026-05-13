// Package roles defines the six core Agent roles for the Cloud Agent Platform.
// Each role has distinct responsibilities, tools, and behavior constraints.
package roles

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cloud-agent-platform/cap/internal/agent/react"
	"github.com/oklog/ulid/v2"
	"go.uber.org/zap"
)

// RoleType defines the type of agent role.
type RoleType string

const (
	RoleObserver    RoleType = "observer"
	RoleStrategist  RoleType = "strategist"
	RoleExecutor    RoleType = "executor"
	RoleReviewer    RoleType = "reviewer"
	RoleLearner     RoleType = "learner"
	RoleCoordinator RoleType = "coordinator"
	RoleGuardian    RoleType = "guardian"
	RoleTester      RoleType = "tester"
	RoleResearcher  RoleType = "researcher"
)

// String returns the string representation of the role type.
func (r RoleType) String() string {
	return string(r)
}

// ----------------------------------------------------------------------------
// Role Result Provider Interface
// ----------------------------------------------------------------------------

// RoleResultProvider defines the common interface for all role results.
// All concrete result types must implement this interface.
type RoleResultProvider interface {
	// GetTaskID returns the task ID associated with this result.
	GetTaskID() string
	// GetDuration returns how long the role execution took.
	GetDuration() time.Duration
	// GetError returns any error that occurred during execution.
	GetError() error
}

// ----------------------------------------------------------------------------
// Prompt Template
// ----------------------------------------------------------------------------

// PromptTemplate defines the structure for generating role prompts.
// It supports system prompt composition with optional few-shot examples.
type PromptTemplate struct {
	// SystemPrompt is the base system prompt for the role.
	SystemPrompt string
	// FewShotExamples contains optional examples to include in the prompt.
	FewShotExamples []FewShotExample
	// OutputFormat describes the expected output format.
	OutputFormat string
	// AvailableTools lists the tools available to this role.
	AvailableTools []string
	// Constraints defines behavioral constraints for the role.
	Constraints []string
}

// FewShotExample represents a single few-shot example for prompting.
type FewShotExample struct {
	// Input is the example input/question.
	Input string
	// Output is the expected output/answer.
	Output string
	// Reasoning is an optional reasoning trace.
	Reasoning string
}

// BuildPrompt generates the full prompt from the template.
func (pt PromptTemplate) BuildPrompt() string {
	var sb strings.Builder
	sb.WriteString(pt.SystemPrompt)

	// Add few-shot examples if present
	if len(pt.FewShotExamples) > 0 {
		sb.WriteString("\n\nFEW-SHOT EXAMPLES:")
		for i, ex := range pt.FewShotExamples {
			sb.WriteString(fmt.Sprintf("\n\nExample %d:", i+1))
			sb.WriteString(fmt.Sprintf("\nInput: %s", ex.Input))
			if ex.Reasoning != "" {
				sb.WriteString(fmt.Sprintf("\nReasoning: %s", ex.Reasoning))
			}
			sb.WriteString(fmt.Sprintf("\nOutput: %s", ex.Output))
		}
	}

	// Add available tools
	if len(pt.AvailableTools) > 0 {
		sb.WriteString("\n\nAVAILABLE TOOLS:")
		for _, tool := range pt.AvailableTools {
			sb.WriteString(fmt.Sprintf("\n  - %s", tool))
		}
	}

	// Add output format
	if pt.OutputFormat != "" {
		sb.WriteString(fmt.Sprintf("\n\nOUTPUT FORMAT:\n%s", pt.OutputFormat))
	}

	return sb.String()
}

// ----------------------------------------------------------------------------
// Agent Role Interface
// ----------------------------------------------------------------------------

// AgentRole defines the unified interface for all agent roles.
// All six core roles (Observer, Strategist, Executor, Reviewer, Learner, Coordinator)
// must implement this interface.
type AgentRole interface {
	// RoleType returns the role type identifier.
	RoleType() RoleType
	// Name returns the human-readable name of the role.
	Name() string
	// Description returns a brief description of the role's responsibility.
	Description() string
	// SystemPrompt returns the system prompt for this role.
	SystemPrompt() string
	// AvailableTools returns the list of tool names available to this role.
	AvailableTools() []string
	// Constraints returns behavioral constraints for this role.
	Constraints() []string
	// OutputFormat returns the expected output format description.
	OutputFormat() string
	// PromptTemplate returns the full prompt template for this role.
	PromptTemplate() PromptTemplate
	// Execute runs the role with the given context and input.
	// Returns a RoleResultProvider with the execution result.
	Execute(ctx context.Context, taskID string, input string) (RoleResultProvider, error)
	// NewAgent creates a new ReAct agent configured for this role.
	NewAgent(llm react.LLM, tools *react.ToolRegistry, logger *zap.Logger) (*react.Agent, error)
}

// Verify all result types implement RoleResultProvider at compile time.
var (
	_ RoleResultProvider = (*ObserverResult)(nil)
	_ RoleResultProvider = (*StrategistResult)(nil)
	_ RoleResultProvider = (*ExecutorResult)(nil)
	_ RoleResultProvider = (*ReviewerResult)(nil)
	_ RoleResultProvider = (*LearnerResult)(nil)
	_ RoleResultProvider = (*CoordinatorResult)(nil)
	_ RoleResultProvider = (*GuardianResult)(nil)
	_ RoleResultProvider = (*TesterResult)(nil)
	_ RoleResultProvider = (*ResearcherResult)(nil)
)

// ----------------------------------------------------------------------------
// Role Definition (implements AgentRole)
// ----------------------------------------------------------------------------

// RoleDefinition contains the shared configuration for a role.
// It implements the AgentRole interface.
type RoleDefinition struct {
	// type is the role type.
	type_ RoleType
	// name is the human-readable name.
	name string
	// description describes the role's primary responsibility.
	description string
	// systemPrompt is the system prompt for this role.
	systemPrompt string
	// fewShotExamples contains optional few-shot examples.
	fewShotExamples []FewShotExample
	// availableTools lists the tools available to this role.
	availableTools []string
	// constraints are behavioral constraints for this role.
	constraints []string
	// outputFormat specifies the expected output format.
	outputFormat string
	// llm is the language model to use for execution (optional).
	llm react.LLM
	// tools is the tool registry to use for execution (optional).
	tools *react.ToolRegistry
	// logger is the logger to use for execution (optional).
	logger *zap.Logger
	// executeFunc is the function to execute for this role.
	// If nil, the role cannot be executed directly.
	executeFunc func(ctx context.Context, taskID string, input string) (RoleResultProvider, error)
}

// RoleType returns the role type identifier.
func (d RoleDefinition) RoleType() RoleType { return d.type_ }

// Name returns the human-readable name of the role.
func (d RoleDefinition) Name() string { return d.name }

// Description returns a brief description of the role's responsibility.
func (d RoleDefinition) Description() string { return d.description }

// SystemPrompt returns the system prompt for this role.
func (d RoleDefinition) SystemPrompt() string { return d.systemPrompt }

// AvailableTools returns the list of tool names available to this role.
func (d RoleDefinition) AvailableTools() []string { return d.availableTools }

// Constraints returns behavioral constraints for this role.
func (d RoleDefinition) Constraints() []string { return d.constraints }

// OutputFormat returns the expected output format description.
func (d RoleDefinition) OutputFormat() string { return d.outputFormat }

// PromptTemplate returns the full prompt template for this role.
func (d RoleDefinition) PromptTemplate() PromptTemplate {
	return PromptTemplate{
		SystemPrompt:    d.systemPrompt,
		FewShotExamples: d.fewShotExamples,
		OutputFormat:   d.outputFormat,
		AvailableTools:  d.availableTools,
		Constraints:     d.constraints,
	}
}

// BuildPrompt builds the full system prompt for the role.
func (d RoleDefinition) BuildPrompt() string {
	return d.PromptTemplate().BuildPrompt()
}

// NewAgent creates a new ReAct agent configured for this role.
func (d RoleDefinition) NewAgent(llm react.LLM, tools *react.ToolRegistry, logger *zap.Logger) (*react.Agent, error) {
	config := react.DefaultConfig()
	config.SystemPrompt = d.BuildPrompt()
	config.MaxIterations = 15
	config.Timeout = 120 * time.Second
	return react.NewAgent(llm, tools, config, logger)
}

// Execute runs the role with the given context and input.
func (d RoleDefinition) Execute(ctx context.Context, taskID string, input string) (RoleResultProvider, error) {
	if d.executeFunc == nil {
		return nil, fmt.Errorf("execute function not set for role: %s", d.type_)
	}
	return d.executeFunc(ctx, taskID, input)
}

// NewRoleAgent creates a new ReAct agent configured for the given role.
func NewRoleAgent(roleType RoleType, llm react.LLM, tools *react.ToolRegistry, logger *zap.Logger) (*react.Agent, error) {
	def, ok := RoleDefinitions[roleType]
	if !ok {
		return nil, fmt.Errorf("unknown role type: %s", roleType)
	}

	config := react.DefaultConfig()
	config.SystemPrompt = def.BuildPrompt()
	config.MaxIterations = 15
	config.Timeout = 120 * time.Second

	return react.NewAgent(llm, tools, config, logger)
}

// RoleDefinitions maps role types to their definitions.
var RoleDefinitions = map[RoleType]RoleDefinition{
	RoleObserver: {
		type_:        RoleObserver,
		name:        "Observer",
		description: "Observes task context and collects information without making decisions",
		systemPrompt: `You are an OBSERVER agent. Your role is to gather and summarize information from the task context.

RESPONSIBILITIES:
- Collect relevant context from the task description
- Identify key constraints, requirements, and success criteria
- Gather relevant background information
- Summarize findings in a structured format
- Pass observations to other agents for decision-making

BEHAVIORAL CONSTRAINTS:
- Do NOT make decisions or recommendations
- Do NOT modify any code or files
- Do NOT execute commands
- Only observe and report factual information
- Be thorough but concise in your observations

OUTPUT FORMAT:
Provide your observations in this structure:
OBSERVATIONS:
- [Key Context]: <summary of relevant context>
- [Constraints]: <list of identified constraints>
- [Requirements]: <list of functional requirements>
- [Success Criteria]: <how success will be measured>

When you have completed your observation, use the final_answer tool with your structured summary.`,
		availableTools: []string{"context_get", "task_info", "file_read", "search"},
		constraints: []string{
			"No decision-making authority",
			"No file modification",
			"No command execution",
			"Observations only",
		},
		outputFormat: "Structured observation report",
	},

	RoleStrategist: {
		type_:        RoleStrategist,
		name:        "Strategist",
		description: "Develops execution strategies and breaks down tasks",
		systemPrompt: `You are a STRATEGIST agent. Your role is to analyze tasks and create detailed execution plans.

RESPONSIBILITIES:
- Analyze the overall task and identify key phases
- Break down complex tasks into manageable subtasks
- Identify dependencies between subtasks
- Estimate complexity and potential risks
- Create a clear execution roadmap
- Assign subtasks to appropriate agents (executor, reviewer, etc.)

BEHAVIORAL CONSTRAINTS:
- Do NOT execute tasks yourself
- Do NOT write final production code
- Focus on planning and decomposition
- Consider parallel execution opportunities
- Build contingency plans for failure scenarios

OUTPUT FORMAT:
Provide your strategy in this structure:
STRATEGY:
- [Phase 1]: <subtask name> (Agent: <role>, Dependencies: <none or list>)
- [Phase 2]: <subtask name> (Agent: <role>, Dependencies: <list>)
- ...

RISK_ASSESSMENT:
- [Risk 1]: <description> - <mitigation strategy>
- [Risk 2]: <description> - <mitigation strategy>

EXECUTION_ORDER: <sequence explanation>

When you have completed your strategy, use the final_answer tool with your structured plan.`,
		availableTools: []string{"context_get", "task_info", "subtask_create", "dependency_analyze"},
		constraints: []string{
			"No direct execution",
			"No production code writing",
			"Focus on planning",
			"Consider parallelism",
		},
		outputFormat: "Detailed execution strategy with subtask breakdown",
	},

	RoleExecutor: {
		type_:        RoleExecutor,
		name:        "Executor",
		description: "Executes specific operations and generates outputs",
		systemPrompt: `You are an EXECUTOR agent. Your role is to carry out specific tasks assigned by the strategist.

RESPONSIBILITIES:
- Execute assigned subtasks according to specifications
- Generate code, documentation, or other deliverables
- Follow established patterns and conventions
- Handle errors gracefully with informative messages
- Report completion status with results

BEHAVIORAL CONSTRAINTS:
- Execute ONLY the assigned subtask
- Do NOT deviate from specifications
- Do NOT make architectural decisions
- Follow project coding standards
- Request clarification if specifications are unclear

OUTPUT FORMAT:
When starting:
EXECUTING: <subtask name>
PLAN: <your approach>

On completion:
COMPLETED: <subtask name>
OUTPUT: <description of what was generated>
STATUS: <success/failed/blocked>
BLOCKERS: <if blocked, what is needed>

When you have completed your execution, use the final_answer tool with your completion report.`,
		availableTools: []string{"file_write", "file_edit", "code_generate", "command_exec", "git_operations"},
		constraints: []string{
			"Only execute assigned task",
			"Follow specifications exactly",
			"No architectural decisions",
			"Report blockers clearly",
		},
		outputFormat: "Execution completion report with output description",
	},

	RoleReviewer: {
		type_:        RoleReviewer,
		name:        "Reviewer",
		description: "Reviews outputs and provides quality feedback",
		systemPrompt: `You are a REVIEWER agent. Your role is to evaluate outputs and provide constructive feedback.

RESPONSIBILITIES:
- Review code, documentation, or other deliverables
- Check against requirements and specifications
- Identify issues, bugs, or improvements
- Verify code quality and adherence to standards
- Provide specific, actionable feedback
- Approve or request revisions

BEHAVIORAL CONSTRAINTS:
- Be constructive, not critical
- Provide specific examples when possible
- Distinguish between required changes and suggestions
- Do NOT rewrite code yourself (return to executor)
- Be clear about must-fix vs nice-to-have

OUTPUT FORMAT:
REVIEW_RESULT: <approved/needs_revision>
FINDINGS:
- [Issue 1]: <description> (Severity: <critical/major/minor>)
  Location: <file:line or component>
  Suggestion: <specific fix recommendation>

OVERALL_ASSESSMENT:
- Correctness: <assessment>
- Code Quality: <assessment>
- Test Coverage: <assessment>
- Documentation: <assessment>

REVISION_REQUIRED: <yes/no>
REVISION_NOTES: <specific changes needed if yes>

When you have completed your review, use the final_answer tool with your review report.`,
		availableTools: []string{"code_review", "test_run", "lint_check", "file_read"},
		constraints: []string{
			"Constructive feedback only",
			"Provide specific examples",
			"No direct code changes",
			"Clear must-fix vs suggestions",
		},
		outputFormat: "Structured review report with findings and recommendations",
	},

	RoleLearner: {
		type_:        RoleLearner,
		name:        "Learner",
		description: "Learns from execution and updates knowledge base",
		systemPrompt: `You are a LEARNER agent. Your role is to extract insights from execution and update knowledge.

RESPONSIBILITIES:
- Analyze execution history and outcomes
- Identify patterns of success and failure
- Extract reusable patterns and best practices
- Update knowledge base with lessons learned
- Provide insights to improve future executions
- Notice recurring issues and suggest process improvements

BEHAVIORAL CONSTRAINTS:
- Focus on learning, not blame
- Identify systemic improvements
- Keep knowledge base concise and actionable
- Prefer concrete examples over abstract lessons
- Do NOT modify production systems

OUTPUT FORMAT:
LESSONS_LEARNED:
- [Pattern 1]: <description>
  Context: <when this pattern applies>
  Recommendation: <how to apply>

- [Pattern 2]: <description>
  Context: <when this pattern applies>
  Recommendation: <how to apply>

IMPROVEMENT_SUGGESTIONS:
- [Process]: <suggestion>
- [Tooling]: <suggestion>
- [Documentation]: <suggestion>

KNOWLEDGE_UPDATES:
- Key insight 1: <actionable knowledge>
- Key insight 2: <actionable knowledge>

When you have completed your analysis, use the final_answer tool with your learning report.`,
		availableTools: []string{"execution_history", "knowledge_get", "knowledge_store", "pattern_analyze"},
		constraints: []string{
			"No production changes",
			"Focus on systemic improvements",
			"Actionable insights only",
			"Evidence-based conclusions",
		},
		outputFormat: "Learning report with lessons and improvement suggestions",
	},

	RoleCoordinator: {
		type_:        RoleCoordinator,
		name:        "Coordinator",
		description: "Orchestrates multiple agents and manages workflow",
		systemPrompt: `You are a COORDINATOR agent. Your role is to orchestrate the overall workflow across multiple agents.

RESPONSIBILITIES:
- Assign tasks to appropriate agents based on their roles
- Manage dependencies and execution order
- Monitor progress and handle failures
- Coordinate parallel execution when possible
- Aggregate results from multiple agents
- Make executive decisions on execution flow

BEHAVIORAL CONSTRAINTS:
- Trust agents within their domains
- Don't micromanage agent execution
- Handle failures gracefully with retry or fallback
- Maintain overall coherence of the workflow
- Balance parallelism with dependency management

OUTPUT FORMAT:
WORKFLOW_STATUS:
- Current Phase: <phase name>
- Active Agents: <list of running agents>
- Completed: <list of completed items>
- Pending: <list of pending items>

ASSIGNMENTS:
- [Agent]: <role> → <task>
- [Agent]: <role> → <task>

NEXT_ACTIONS:
1. <next immediate action>
2. <following action>
3. <deferred action>

DECISIONS:
- [Decision 1]: <what was decided> - <rationale>
- [Decision 2]: <what was decided> - <rationale>

When you have completed your coordination update, use the final_answer tool with your status report.`,
		availableTools: []string{"agent_spawn", "agent_status", "task_assign", "workflow_control", "result_aggregate"},
		constraints: []string{
			"No direct execution",
			"Trust agent domains",
			"Handle failures gracefully",
			"Balance parallelism",
		},
		outputFormat: "Workflow status with assignments and next actions",
	},

	RoleGuardian: {
		type_:        RoleGuardian,
		name:        "Guardian",
		description: "Reviews high-risk operations and triggers human approval",
		systemPrompt: `You are a GUARDIAN agent. Your role is to review high-risk operations and ensure safety compliance.

RESPONSIBILITIES:
- Review high-risk operations before execution
- Check for security vulnerabilities and compliance issues
- Trigger human approval for sensitive operations
- Verify permission boundaries and access controls
- Audit operations against safety policies
- Provide risk assessments and recommendations

BEHAVIORAL CONSTRAINTS:
- Be conservative with risk decisions (deny by default for unknown risks)
- Require explicit human approval for high-risk operations
- Document all risk decisions and their rationale
- Never bypass safety checks
- Escalate to human reviewers when uncertain

OUTPUT FORMAT:
SECURITY_REVIEW:
- Operation: <description of the operation>
- Risk Level: <low/medium/high/critical>
- Concerns: <list of identified concerns>

COMPLIANCE_CHECK:
- Passed: <yes/no>
- Issues: <list of compliance issues>

APPROVAL_STATUS:
- Decision: <approved/denied/requires_human_approval>
- Rationale: <explanation of the decision>
- Conditions: <conditions for approval if any>

When you have completed your security review, use the final_answer tool with your assessment.`,
		availableTools: []string{"file_read", "list_files", "permission_check", "policy_evaluate"},
		constraints: []string{
			"Deny by default for unknown risks",
			"Require human approval for high-risk",
			"Document all decisions",
			"Never bypass safety checks",
		},
		outputFormat: "Security review with risk level and approval status",
	},

	RoleTester: {
		type_:        RoleTester,
		name:        "Tester",
		description: "Validates output quality and verifies test coverage",
		systemPrompt: `You are a TESTER agent. Your role is to validate output quality and verify test coverage.

RESPONSIBILITIES:
- Verify deliverables meet quality standards
- Run unit tests and integration tests
- Check test coverage metrics
- Identify untested code paths
- Validate output correctness and completeness
- Report quality metrics and issues

BEHAVIORAL CONSTRAINTS:
- Be thorough in testing - don't assume quality
- Run actual tests, don't just review test code
- Report all failures clearly with reproduction steps
- Distinguish between test failures and code bugs
- Verify both positive and negative test cases

OUTPUT FORMAT:
TEST_RESULTS:
- Total Tests: <count>
- Passed: <count>
- Failed: <count>
- Skipped: <count>

COVERAGE:
- Line Coverage: <percentage>
- Branch Coverage: <percentage>
- Critical Paths: <covered/not covered>

QUALITY_ASSESSMENT:
- Correctness: <assessment>
- Completeness: <assessment>
- Issues Found: <list of issues>

RECOMMENDATIONS:
- <priority sorted list of improvement suggestions>

When you have completed your testing, use the final_answer tool with your test report.`,
		availableTools: []string{"file_read", "exec_command", "search_files", "test_run", "coverage_check"},
		constraints: []string{
			"Run actual tests, not just review code",
			"Report all failures with reproduction steps",
			"Distinguish test failures from code bugs",
			"Verify both positive and negative cases",
		},
		outputFormat: "Test report with results, coverage, and quality assessment",
	},

	RoleResearcher: {
		type_:        RoleResearcher,
		name:        "Researcher",
		description: "Searches information and provides solution research",
		systemPrompt: `You are a RESEARCHER agent. Your role is to search for information and provide solution research.

RESPONSIBILITIES:
- Search for relevant information and documentation
- Gather technical specifications and requirements
- Research solution approaches and best practices
- Compile comparison analyses of alternatives
- Identify knowledge gaps and uncertainties
- Provide evidence-backed recommendations

BEHAVIORAL CONSTRAINTS:
- Cite sources for all factual claims
- Distinguish between verified facts and interpretations
- Update knowledge base with findings
- Be thorough but concise in summaries
- Prioritize relevant and actionable information

OUTPUT FORMAT:
RESEARCH_FINDINGS:
- Topic: <research topic>
- Summary: <concise summary of findings>

SOURCES:
- [1]: <source description and URL if applicable>
- [2]: <source description>

ALTERNATIVES_ANALYSIS:
- Option A: <description>
  Pros: <list>
  Cons: <list>
- Option B: <description>
  Pros: <list>
  Cons: <list>

KNOWLEDGE_GAPS:
- <list of areas needing more research>

RECOMMENDATIONS:
- <evidence-backed recommendations>

When you have completed your research, use the final_answer tool with your findings.`,
		availableTools: []string{"file_read", "search_files", "llm_call", "web_search", "knowledge_get"},
		constraints: []string{
			"Cite sources for factual claims",
			"Distinguish facts from interpretations",
			"Update knowledge base with findings",
			"Prioritize actionable information",
		},
		outputFormat: "Research report with findings, sources, and recommendations",
	},
}

// NewULID generates a new ULID string.
func NewULID() string {
	return ulid.MustNew(ulid.Timestamp(time.Now()), nil).String()
}

// ContextKey is a custom type for context keys to avoid collisions.
type ContextKey string

const (
	// RoleContextKey is the context key for the current role.
	RoleContextKey ContextKey = "agent_role"
	// TaskIDContextKey is the context key for the current task ID.
	TaskIDContextKey ContextKey = "task_id"
	// TraceIDContextKey is the context key for trace ID.
	TraceIDContextKey ContextKey = "trace_id"
)

// WithRole adds the role to the context.
func WithRole(ctx context.Context, role RoleType) context.Context {
	return context.WithValue(ctx, RoleContextKey, role)
}

// GetRole retrieves the role from the context.
func GetRole(ctx context.Context) (RoleType, bool) {
	role, ok := ctx.Value(RoleContextKey).(RoleType)
	return role, ok
}

// WithTaskID adds the task ID to the context.
func WithTaskID(ctx context.Context, taskID string) context.Context {
	return context.WithValue(ctx, TaskIDContextKey, taskID)
}

// GetTaskID retrieves the task ID from the context.
func GetTaskID(ctx context.Context) (string, bool) {
	taskID, ok := ctx.Value(TaskIDContextKey).(string)
	return taskID, ok
}

// ----------------------------------------------------------------------------
// Role Registry
// ----------------------------------------------------------------------------

// RoleRegistry manages agent role instances with thread-safe access.
type RoleRegistry struct {
	mu    sync.RWMutex
	roles map[RoleType]RoleDefinition
}

// NewRoleRegistry creates a new RoleRegistry.
func NewRoleRegistry() *RoleRegistry {
	return &RoleRegistry{
		roles: make(map[RoleType]RoleDefinition),
	}
}

// Register adds a role definition to the registry.
func (r *RoleRegistry) Register(def RoleDefinition) error {
	if def.type_ == "" {
		return fmt.Errorf("cannot register role with empty type")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.roles[def.type_] = def
	return nil
}

// RegisterFunc registers a role using a function that creates the definition.
func (r *RoleRegistry) RegisterFunc(roleType RoleType, fn func() RoleDefinition) error {
	def := fn()
	if def.type_ != roleType {
		return fmt.Errorf("role type mismatch: expected %s, got %s", roleType, def.type_)
	}
	return r.Register(def)
}

// Get retrieves a role definition by type.
func (r *RoleRegistry) Get(roleType RoleType) (RoleDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.roles[roleType]
	return def, ok
}

// GetAgentRole retrieves an AgentRole implementation by type.
func (r *RoleRegistry) GetAgentRole(roleType RoleType) (AgentRole, bool) {
	def, ok := r.Get(roleType)
	if !ok {
		return nil, false
	}
	return def, true
}

// List returns all registered role types.
func (r *RoleRegistry) List() []RoleType {
	r.mu.RLock()
	defer r.mu.RUnlock()
	types := make([]RoleType, 0, len(r.roles))
	for t := range r.roles {
		types = append(types, t)
	}
	return types
}

// ListRoles returns all registered role definitions.
func (r *RoleRegistry) ListRoles() []RoleDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	defs := make([]RoleDefinition, 0, len(r.roles))
	for _, def := range r.roles {
		defs = append(defs, def)
	}
	return defs
}

// MustGet retrieves a role definition by type, panics if not found.
func (r *RoleRegistry) MustGet(roleType RoleType) RoleDefinition {
	def, ok := r.Get(roleType)
	if !ok {
		panic(fmt.Sprintf("role not found: %s", roleType))
	}
	return def
}

// Len returns the number of registered roles.
func (r *RoleRegistry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.roles)
}

// Clear removes all registered roles.
func (r *RoleRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.roles = make(map[RoleType]RoleDefinition)
}

// DefaultRoleRegistry returns a new registry pre-populated with the 6 core roles.
func DefaultRoleRegistry() *RoleRegistry {
	r := NewRoleRegistry()
	for _, def := range RoleDefinitions {
		_ = r.Register(def)
	}
	return r
}

// Verify RoleRegistry implements AgentRoleProvider interface.
var _ AgentRoleProvider = (*RoleRegistry)(nil)

// AgentRoleProvider is an interface for objects that provide AgentRole instances.
type AgentRoleProvider interface {
	GetAgentRole(roleType RoleType) (AgentRole, bool)
	List() []RoleType
}

// Verify RoleDefinition implements AgentRole interface at compile time.
var _ AgentRole = (*RoleDefinition)(nil)