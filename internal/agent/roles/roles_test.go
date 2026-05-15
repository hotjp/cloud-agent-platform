package roles

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/cloud-agent-platform/cap/internal/agent/react"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// mockLLM is a mock LLM for testing.
type mockLLM struct {
	generateFunc func(ctx context.Context, messages []*react.Message, opts *react.GenerateOptions) (*react.GenerateResult, error)
}

func (m *mockLLM) Generate(ctx context.Context, messages []*react.Message, opts *react.GenerateOptions) (*react.GenerateResult, error) {
	if m.generateFunc != nil {
		return m.generateFunc(ctx, messages, opts)
	}
	return &react.GenerateResult{
		Content:     "Final Answer: Test completed",
		StopReason: "final_answer",
	}, nil
}

func (m *mockLLM) GenerateStream(ctx context.Context, messages []*react.Message, opts *react.GenerateOptions, callback func(chunk string) error) (*react.GenerateResult, error) {
	return m.Generate(ctx, messages, opts)
}

func TestRoleDefinitions(t *testing.T) {
	tests := []struct {
		name     string
		roleType RoleType
	}{
		{"Observer role", RoleObserver},
		{"Strategist role", RoleStrategist},
		{"Executor role", RoleExecutor},
		{"Reviewer role", RoleReviewer},
		{"Learner role", RoleLearner},
		{"Coordinator role", RoleCoordinator},
		{"Guardian role", RoleGuardian},
		{"Tester role", RoleTester},
		{"Researcher role", RoleResearcher},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def, ok := RoleDefinitions[tt.roleType]
			require.True(t, ok, "Role definition should exist for %s", tt.roleType)

			assert.NotEmpty(t, def.Name())
			assert.NotEmpty(t, def.Description())
			assert.NotEmpty(t, def.SystemPrompt())
			assert.NotEmpty(t, def.OutputFormat())
			assert.Equal(t, tt.roleType, def.RoleType())
		})
	}
}

func TestRoleDefinition_BuildPrompt(t *testing.T) {
	def := RoleDefinitions[RoleObserver]
	prompt := def.BuildPrompt()

	assert.NotEmpty(t, prompt)
	assert.Contains(t, prompt, def.SystemPrompt())
	assert.Contains(t, prompt, "AVAILABLE TOOLS")
}

func TestNewRoleAgent(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	llm := &mockLLM{}
	tools := react.NewToolRegistry()

	tests := []struct {
		name     string
		roleType RoleType
		wantErr  bool
	}{
		{"Observer agent creation", RoleObserver, false},
		{"Strategist agent creation", RoleStrategist, false},
		{"Executor agent creation", RoleExecutor, false},
		{"Reviewer agent creation", RoleReviewer, false},
		{"Learner agent creation", RoleLearner, false},
		{"Coordinator agent creation", RoleCoordinator, false},
		{"Guardian agent creation", RoleGuardian, false},
		{"Tester agent creation", RoleTester, false},
		{"Researcher agent creation", RoleResearcher, false},
		{"Unknown role", RoleType("unknown"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent, err := NewRoleAgent(tt.roleType, llm, tools, logger)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, agent)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, agent)
			}
		})
	}
}

func TestObserverResult(t *testing.T) {
	result := &ObserverResult{
		TaskID: "test-task-id",
		Observations: &Observations{
			KeyContext:      "Test context",
			Constraints:     []string{"constraint1"},
			Requirements:    []string{"req1"},
			SuccessCriteria: []string{"criteria1"},
		},
		Duration: 100 * time.Millisecond,
	}

	assert.Equal(t, "test-task-id", result.GetTaskID())
	assert.Equal(t, 100*time.Millisecond, result.GetDuration())
	assert.Nil(t, result.GetError())
}

func TestStrategistResult(t *testing.T) {
	result := &StrategistResult{
		TaskID: "test-task-id",
		Strategy: &Strategy{
			Phases: []*Phase{
				{
					Name:        "Phase 1",
					Subtask:     "Subtask 1",
					AgentRole:   RoleExecutor,
					Dependencies: []string{},
					Priority:    1,
				},
			},
			Risks: []*Risk{
				{
					Description: "Risk 1",
					Mitigation:  "Mitigation 1",
					Severity:    "medium",
				},
			},
			ExecutionOrder: "Sequential",
		},
		Duration: 200 * time.Millisecond,
	}

	assert.Equal(t, "test-task-id", result.GetTaskID())
	assert.Equal(t, 200*time.Millisecond, result.GetDuration())
	assert.Len(t, result.Strategy.Phases, 1)
	assert.Len(t, result.Strategy.Risks, 1)
}

func TestExecutorResult(t *testing.T) {
	result := &ExecutorResult{
		TaskID:   "test-task-id",
		Subtask:  "Test subtask",
		Status:   ExecutorStatusSuccess,
		Output:   "Test output",
		Duration: 300 * time.Millisecond,
	}

	assert.Equal(t, "test-task-id", result.GetTaskID())
	assert.Equal(t, 300*time.Millisecond, result.GetDuration())
	assert.Equal(t, ExecutorStatusSuccess, result.Status)
}

func TestReviewerResult(t *testing.T) {
	result := &ReviewerResult{
		TaskID:            "test-task-id",
		ReviewResult:      ReviewOutcomeApproved,
		Findings:          []*Finding{},
		RevisionRequired:  false,
		Duration:          150 * time.Millisecond,
	}

	assert.Equal(t, "test-task-id", result.GetTaskID())
	assert.Equal(t, 150*time.Millisecond, result.GetDuration())
	assert.Equal(t, ReviewOutcomeApproved, result.ReviewResult)
	assert.False(t, result.RevisionRequired)
}

func TestLearnerResult(t *testing.T) {
	result := &LearnerResult{
		TaskID: "test-task-id",
		Lessons: []*Lesson{
			{
				Pattern:        "Pattern 1",
				Context:        "Context 1",
				Recommendation: "Recommendation 1",
				Confidence:     0.9,
			},
		},
		Improvements: []*Improvement{
			{
				Category:    ImprovementProcess,
				Description: "Improve process",
				Impact:      "High",
			},
		},
		Duration: 250 * time.Millisecond,
	}

	assert.Equal(t, "test-task-id", result.GetTaskID())
	assert.Equal(t, 250*time.Millisecond, result.GetDuration())
	assert.Len(t, result.Lessons, 1)
	assert.Len(t, result.Improvements, 1)
}

func TestCoordinatorResult(t *testing.T) {
	result := &CoordinatorResult{
		TaskID: "test-task-id",
		Status: WorkflowStatusRunning,
		Assignments: []*Assignment{
			{
				AgentID: "agent-1",
				Role:    RoleExecutor,
				Task:    "Task 1",
				Status:  AssignmentStatusRunning,
			},
		},
		Completed: []string{"Task 1"},
		Pending:  []string{"Task 2"},
		Duration: 400 * time.Millisecond,
	}

	assert.Equal(t, "test-task-id", result.GetTaskID())
	assert.Equal(t, 400*time.Millisecond, result.GetDuration())
	assert.Equal(t, WorkflowStatusRunning, result.Status)
	assert.Len(t, result.Assignments, 1)
}

func TestContextHelpers(t *testing.T) {
	ctx := context.Background()

	// Test WithRole and GetRole
	ctx = WithRole(ctx, RoleExecutor)
	role, ok := GetRole(ctx)
	assert.True(t, ok)
	assert.Equal(t, RoleExecutor, role)

	// Test WithTaskID and GetTaskID
	ctx = WithTaskID(ctx, "task-123")
	taskID, ok := GetTaskID(ctx)
	assert.True(t, ok)
	assert.Equal(t, "task-123", taskID)
}

func TestNewULID(t *testing.T) {
	ulid1 := NewULID()

	assert.NotEmpty(t, ulid1)
	assert.Len(t, ulid1, 26) // ULID is 26 characters
}

func TestRoleTypeString(t *testing.T) {
	assert.Equal(t, "observer", RoleObserver.String())
	assert.Equal(t, "strategist", RoleStrategist.String())
	assert.Equal(t, "executor", RoleExecutor.String())
	assert.Equal(t, "reviewer", RoleReviewer.String())
	assert.Equal(t, "learner", RoleLearner.String())
	assert.Equal(t, "coordinator", RoleCoordinator.String())
}

func TestDefaultConfigs(t *testing.T) {
	// Observer config
	obsCfg := DefaultObserverConfig()
	assert.NotNil(t, obsCfg)
	assert.Equal(t, 10000, obsCfg.MaxContextSize)
	assert.Equal(t, 30*time.Second, obsCfg.Timeout)

	// Strategist config
	stratCfg := DefaultStrategistConfig()
	assert.NotNil(t, stratCfg)
	assert.Equal(t, 10, stratCfg.MaxSubtasks)
	assert.Equal(t, 60*time.Second, stratCfg.Timeout)

	// Executor config
	execCfg := DefaultExecutorConfig()
	assert.NotNil(t, execCfg)
	assert.Equal(t, 120*time.Second, execCfg.Timeout)
	assert.Equal(t, 2, execCfg.MaxRetries)

	// Reviewer config
	revCfg := DefaultReviewerConfig()
	assert.NotNil(t, revCfg)
	assert.Equal(t, 60*time.Second, revCfg.Timeout)
	assert.True(t, revCfg.RequireTestCoverage)

	// Learner config
	lrnCfg := DefaultLearnerConfig()
	assert.NotNil(t, lrnCfg)
	assert.Equal(t, 10, lrnCfg.MaxPatterns)
	assert.Equal(t, 0.7, lrnCfg.ConfidenceThreshold)

	// Coordinator config
	coordCfg := DefaultCoordinatorConfig()
	assert.NotNil(t, coordCfg)
	assert.Equal(t, 180*time.Second, coordCfg.Timeout)
	assert.Equal(t, 3, coordCfg.MaxParallelAgents)
	assert.True(t, coordCfg.EnableAutoRetry)

	// Guardian config
	guardCfg := DefaultGuardianConfig()
	assert.NotNil(t, guardCfg)
	assert.Equal(t, 60*time.Second, guardCfg.Timeout)
	assert.True(t, guardCfg.RequireHumanApproval)
	assert.Equal(t, RiskLevelHigh, guardCfg.HighRiskThreshold)

	// Tester config
	testCfg := DefaultTesterConfig()
	assert.NotNil(t, testCfg)
	assert.Equal(t, 120*time.Second, testCfg.Timeout)
	assert.True(t, testCfg.RequireMinimumCoverage)
	assert.Equal(t, 80.0, testCfg.MinimumCoverage)

	// Researcher config
	researchCfg := DefaultResearcherConfig()
	assert.NotNil(t, researchCfg)
	assert.Equal(t, 90*time.Second, researchCfg.Timeout)
	assert.Equal(t, 10, researchCfg.MaxSources)
	assert.Equal(t, 0.7, researchCfg.MinConfidence)
}

func TestHelperFunctions(t *testing.T) {
	// Test toLower
	assert.Equal(t, "hello world", toLower("Hello World"))
	assert.Equal(t, "hello world", toLower("HeLLo WoRLD"))

	// Test cleanLine
	assert.Equal(t, "item", cleanLine("- item"))
	assert.Equal(t, "item", cleanLine("* item"))
	assert.Equal(t, "item", cleanLine("> item"))
	assert.Equal(t, "item", cleanLine("  item  "))

	// Test contains
	assert.True(t, contains("hello world", "world"))
	assert.True(t, contains("hello world", "hello"))
	assert.False(t, contains("hello world", "foo"))
	assert.True(t, contains("hello world", ""))
	assert.True(t, contains("", ""))

	// Test splitLines
	assert.Equal(t, []string{"a", "b", "c"}, splitLines("a\nb\nc"))
	assert.Equal(t, []string{"single"}, splitLines("single"))
	assert.Nil(t, splitLines(""))
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"empty string", "", 0},
		{"single line", "hello", 1},
		{"two lines", "hello\nworld", 2},
		{"three lines", "line1\nline2\nline3", 3},
		{"trailing newline", "hello\n", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitLines(tt.input)
			assert.Equal(t, tt.expected, len(result))
		})
	}
}

func TestParseAgentRole(t *testing.T) {
	tests := []struct {
		input    string
		expected RoleType
	}{
		{"observer", RoleObserver},
		{"OBSERVER", RoleObserver},
		{"Observer Agent", RoleObserver},
		{"strategist", RoleStrategist},
		{"executor", RoleExecutor},
		{"reviewer", RoleReviewer},
		{"learner", RoleLearner},
		{"coordinator", RoleCoordinator},
		{"unknown", RoleExecutor}, // defaults to executor
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseAgentRole(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ----------------------------------------------------------------------------
// RoleRegistry Tests
// ----------------------------------------------------------------------------

func TestRoleRegistry_NewRoleRegistry(t *testing.T) {
	r := NewRoleRegistry()
	assert.NotNil(t, r)
	assert.Equal(t, 0, r.Len())
}

func TestRoleRegistry_Register(t *testing.T) {
	r := NewRoleRegistry()
	def := RoleDefinition{
		type_: RoleObserver,
		name:  "Observer",
	}

	err := r.Register(def)
	assert.NoError(t, err)
	assert.Equal(t, 1, r.Len())
}

func TestRoleRegistry_RegisterDuplicate(t *testing.T) {
	r := NewRoleRegistry()
	def := RoleDefinition{
		type_: RoleObserver,
		name:  "Observer",
	}

	err := r.Register(def)
	assert.NoError(t, err)

	// Registering again should replace
	def.name = "Observer v2"
	err = r.Register(def)
	assert.NoError(t, err)
	assert.Equal(t, 1, r.Len())

	// Verify the name was updated
	updatedDef, _ := r.Get(RoleObserver)
	assert.Equal(t, "Observer v2", updatedDef.name)
}

func TestRoleRegistry_RegisterEmptyType(t *testing.T) {
	r := NewRoleRegistry()
	def := RoleDefinition{
		name: "Test",
	}

	err := r.Register(def)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty type")
}

func TestRoleRegistry_Get(t *testing.T) {
	r := NewRoleRegistry()
	def := RoleDefinition{
		type_:        RoleStrategist,
		name:        "Strategist",
		description: "Test description",
	}

	_ = r.Register(def)

	got, ok := r.Get(RoleStrategist)
	assert.True(t, ok)
	assert.Equal(t, "Strategist", got.name)
	assert.Equal(t, "Test description", got.description)
}

func TestRoleRegistry_GetNotFound(t *testing.T) {
	r := NewRoleRegistry()
	_, ok := r.Get(RoleObserver)
	assert.False(t, ok)
}

func TestRoleRegistry_GetAgentRole(t *testing.T) {
	r := NewRoleRegistry()
	def := RoleDefinition{
		type_:        RoleExecutor,
		name:        "Executor",
		description: "Test executor",
	}
	_ = r.Register(def)

	agentRole, ok := r.GetAgentRole(RoleExecutor)
	assert.True(t, ok)
	assert.Equal(t, "Executor", agentRole.Name())
	assert.Equal(t, RoleExecutor, agentRole.RoleType())
}

func TestRoleRegistry_List(t *testing.T) {
	r := NewRoleRegistry()
	_ = r.Register(RoleDefinition{type_: RoleObserver, name: "Observer"})
	_ = r.Register(RoleDefinition{type_: RoleStrategist, name: "Strategist"})
	_ = r.Register(RoleDefinition{type_: RoleExecutor, name: "Executor"})

	types := r.List()
	assert.Len(t, types, 3)
	assert.Contains(t, types, RoleObserver)
	assert.Contains(t, types, RoleStrategist)
	assert.Contains(t, types, RoleExecutor)
}

func TestRoleRegistry_ListRoles(t *testing.T) {
	r := NewRoleRegistry()
	_ = r.Register(RoleDefinition{type_: RoleReviewer, name: "Reviewer"})
	_ = r.Register(RoleDefinition{type_: RoleLearner, name: "Learner"})

	defs := r.ListRoles()
	assert.Len(t, defs, 2)
}

func TestRoleRegistry_MustGet(t *testing.T) {
	r := NewRoleRegistry()
	_ = r.Register(RoleDefinition{type_: RoleCoordinator, name: "Coordinator"})

	def := r.MustGet(RoleCoordinator)
	assert.Equal(t, "Coordinator", def.name)
}

func TestRoleRegistry_MustGetPanic(t *testing.T) {
	r := NewRoleRegistry()

	assert.Panics(t, func() {
		r.MustGet(RoleObserver)
	})
}

func TestRoleRegistry_Clear(t *testing.T) {
	r := NewRoleRegistry()
	_ = r.Register(RoleDefinition{type_: RoleObserver, name: "Observer"})
	assert.Equal(t, 1, r.Len())

	r.Clear()
	assert.Equal(t, 0, r.Len())
}

func TestRoleRegistry_DefaultRoleRegistry(t *testing.T) {
	r := DefaultRoleRegistry()

	// Should have all 9 core roles
	assert.Equal(t, 9, r.Len())

	// Verify all roles are present
	_, ok := r.Get(RoleObserver)
	assert.True(t, ok)
	_, ok = r.Get(RoleStrategist)
	assert.True(t, ok)
	_, ok = r.Get(RoleExecutor)
	assert.True(t, ok)
	_, ok = r.Get(RoleReviewer)
	assert.True(t, ok)
	_, ok = r.Get(RoleLearner)
	assert.True(t, ok)
	_, ok = r.Get(RoleCoordinator)
	assert.True(t, ok)
	_, ok = r.Get(RoleGuardian)
	assert.True(t, ok)
	_, ok = r.Get(RoleTester)
	assert.True(t, ok)
	_, ok = r.Get(RoleResearcher)
	assert.True(t, ok)
}

// ----------------------------------------------------------------------------
// AgentRole Interface Compliance Tests
// ----------------------------------------------------------------------------

func TestAgentRole_AllRolesImplementInterface(t *testing.T) {
	roles := []RoleType{
		RoleObserver,
		RoleStrategist,
		RoleExecutor,
		RoleReviewer,
		RoleLearner,
		RoleCoordinator,
		RoleGuardian,
		RoleTester,
		RoleResearcher,
	}

	for _, roleType := range roles {
		t.Run(string(roleType), func(t *testing.T) {
			def, ok := RoleDefinitions[roleType]
			require.True(t, ok, "Role definition should exist for %s", roleType)

			// Test that it implements AgentRole
			var agentRole AgentRole = def
			assert.NotNil(t, agentRole)

			// Test all interface methods
			assert.NotEmpty(t, agentRole.Name())
			assert.NotEmpty(t, agentRole.Description())
			assert.NotEmpty(t, agentRole.SystemPrompt())
			assert.NotEmpty(t, agentRole.OutputFormat())
			assert.NotNil(t, agentRole.AvailableTools())
			assert.NotNil(t, agentRole.Constraints())
			assert.NotNil(t, agentRole.PromptTemplate())
			assert.Equal(t, roleType, agentRole.RoleType())
		})
	}
}

func TestAgentRole_PromptTemplate(t *testing.T) {
	for roleType, def := range RoleDefinitions {
		t.Run(string(roleType), func(t *testing.T) {
			pt := def.PromptTemplate()

			// Verify template contains expected fields
			assert.NotEmpty(t, pt.SystemPrompt)
			assert.Equal(t, def.systemPrompt, pt.SystemPrompt)
			assert.Equal(t, def.outputFormat, pt.OutputFormat)
			assert.Equal(t, def.availableTools, pt.AvailableTools)
			assert.Equal(t, def.constraints, pt.Constraints)
		})
	}
}

func TestAgentRole_BuildPrompt(t *testing.T) {
	for roleType, def := range RoleDefinitions {
		t.Run(string(roleType), func(t *testing.T) {
			prompt := def.BuildPrompt()

			assert.NotEmpty(t, prompt)
			assert.Contains(t, prompt, def.systemPrompt)
			assert.Contains(t, prompt, "AVAILABLE TOOLS")

			// Each role should have at least one tool
			if len(def.availableTools) > 0 {
				for _, tool := range def.availableTools {
					assert.Contains(t, prompt, tool)
				}
			}
		})
	}
}

func TestAgentRole_NewAgent(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	llm := &mockLLM{}
	tools := react.NewToolRegistry()

	for roleType, def := range RoleDefinitions {
		t.Run(string(roleType), func(t *testing.T) {
			agent, err := def.NewAgent(llm, tools, logger)
			assert.NoError(t, err)
			assert.NotNil(t, agent)
		})
	}
}

// ----------------------------------------------------------------------------
// PromptTemplate Tests
// ----------------------------------------------------------------------------

func TestPromptTemplate_BuildPrompt_Empty(t *testing.T) {
	pt := PromptTemplate{
		SystemPrompt: "Test prompt",
	}

	prompt := pt.BuildPrompt()
	assert.Contains(t, prompt, "Test prompt")
	assert.NotContains(t, prompt, "FEW-SHOT")
	assert.NotContains(t, prompt, "AVAILABLE TOOLS")
}

func TestPromptTemplate_BuildPrompt_WithFewShot(t *testing.T) {
	pt := PromptTemplate{
		SystemPrompt: "Test prompt",
		FewShotExamples: []FewShotExample{
			{
				Input:    "What is 2+2?",
				Output:   "4",
				Reasoning: "Basic addition",
			},
		},
	}

	prompt := pt.BuildPrompt()
	assert.Contains(t, prompt, "FEW-SHOT EXAMPLES")
	assert.Contains(t, prompt, "Example 1:")
	assert.Contains(t, prompt, "What is 2+2?")
	assert.Contains(t, prompt, "Basic addition")
	assert.Contains(t, prompt, "4")
}

func TestPromptTemplate_BuildPrompt_WithTools(t *testing.T) {
	pt := PromptTemplate{
		SystemPrompt:   "Test prompt",
		AvailableTools: []string{"tool1", "tool2"},
	}

	prompt := pt.BuildPrompt()
	assert.Contains(t, prompt, "AVAILABLE TOOLS")
	assert.Contains(t, prompt, "tool1")
	assert.Contains(t, prompt, "tool2")
}

func TestPromptTemplate_BuildPrompt_WithOutputFormat(t *testing.T) {
	pt := PromptTemplate{
		SystemPrompt: "Test prompt",
		OutputFormat: "JSON format",
	}

	prompt := pt.BuildPrompt()
	assert.Contains(t, prompt, "OUTPUT FORMAT:")
	assert.Contains(t, prompt, "JSON format")
}

// ----------------------------------------------------------------------------
// FewShotExample Tests
// ----------------------------------------------------------------------------

func TestFewShotExample(t *testing.T) {
	ex := FewShotExample{
		Input:    "Test input",
		Output:   "Test output",
		Reasoning: "Test reasoning",
	}

	assert.Equal(t, "Test input", ex.Input)
	assert.Equal(t, "Test output", ex.Output)
	assert.Equal(t, "Test reasoning", ex.Reasoning)
}

// ----------------------------------------------------------------------------
// ExecuteFunc Tests
// ----------------------------------------------------------------------------

func TestRoleDefinition_ExecuteFunc(t *testing.T) {
	def := RoleDefinition{
		type_: RoleObserver,
		name:  "Observer",
		executeFunc: func(ctx context.Context, taskID string, input string) (RoleResultProvider, error) {
			return &ObserverResult{
				TaskID: taskID,
			}, nil
		},
	}

	result, err := def.Execute(context.Background(), "task-123", "test input")
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "task-123", result.GetTaskID())
}

func TestRoleDefinition_ExecuteFunc_NotSet(t *testing.T) {
	def := RoleDefinition{
		type_: RoleObserver,
		name:  "Observer",
	}

	_, err := def.Execute(context.Background(), "task-123", "test input")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "execute function not set")
}

// ----------------------------------------------------------------------------
// RoleResultProvider Verification
// ----------------------------------------------------------------------------

func TestAllResultTypesImplementRoleResultProvider(t *testing.T) {
	results := []RoleResultProvider{
		&ObserverResult{TaskID: "test"},
		&StrategistResult{TaskID: "test"},
		&ExecutorResult{TaskID: "test"},
		&ReviewerResult{TaskID: "test"},
		&LearnerResult{TaskID: "test"},
		&CoordinatorResult{TaskID: "test"},
		&GuardianResult{TaskID: "test"},
		&TesterResult{TaskID: "test"},
		&ResearcherResult{TaskID: "test"},
	}

	for i, result := range results {
		t.Run(fmt.Sprintf("Result_%d", i), func(t *testing.T) {
			assert.NotNil(t, result.GetTaskID())
			assert.Equal(t, "test", result.GetTaskID())
			assert.Nil(t, result.GetError())
		})
	}
}

// ----------------------------------------------------------------------------
// Guardian Result Tests
// ----------------------------------------------------------------------------

func TestGuardianResult(t *testing.T) {
	result := &GuardianResult{
		TaskID:           "test-task-id",
		Operation:        "deploy_service",
		RiskLevel:       RiskLevelHigh,
		Concerns:        []string{"Data exposure risk", "Permission scope too broad"},
		CompliancePassed: true,
		ComplianceIssues: []string{},
		Decision:        ApprovalRequiresHumanApproval,
		Rationale:       "High risk operation requires human review",
		Conditions:      []string{"Must have security team approval", "Implement rate limiting"},
		Duration:        150 * time.Millisecond,
	}

	assert.Equal(t, "test-task-id", result.GetTaskID())
	assert.Equal(t, 150*time.Millisecond, result.GetDuration())
	assert.Equal(t, RiskLevelHigh, result.RiskLevel)
	assert.Equal(t, ApprovalRequiresHumanApproval, result.Decision)
	assert.Len(t, result.Concerns, 2)
	assert.Len(t, result.Conditions, 2)
}

func TestGuardianResult_GetError(t *testing.T) {
	result := &GuardianResult{
		TaskID: "test-task-id",
		Error:  fmt.Errorf("security check failed"),
	}

	assert.Equal(t, "test-task-id", result.GetTaskID())
	assert.NotNil(t, result.GetError())
	assert.Contains(t, result.GetError().Error(), "security check failed")
}

func TestParseRiskLevel(t *testing.T) {
	assert.Equal(t, RiskLevelCritical, parseRiskLevel("critical"))
	assert.Equal(t, RiskLevelHigh, parseRiskLevel("high risk"))
	assert.Equal(t, RiskLevelMedium, parseRiskLevel("medium"))
	assert.Equal(t, RiskLevelLow, parseRiskLevel("low"))
	assert.Equal(t, RiskLevelLow, parseRiskLevel("unknown"))
}

func TestParseApprovalDecision(t *testing.T) {
	assert.Equal(t, ApprovalDenied, parseApprovalDecision("denied"))
	assert.Equal(t, ApprovalRequiresHumanApproval, parseApprovalDecision("requires human approval"))
	assert.Equal(t, ApprovalApproved, parseApprovalDecision("approved"))
	assert.Equal(t, ApprovalRequiresHumanApproval, parseApprovalDecision("unknown"))
}

// ----------------------------------------------------------------------------
// Tester Result Tests
// ----------------------------------------------------------------------------

func TestTesterResult(t *testing.T) {
	result := &TesterResult{
		TaskID:             "test-task-id",
		TotalTests:         100,
		PassedTests:        95,
		FailedTests:        3,
		SkippedTests:       2,
		LineCoverage:       85.5,
		BranchCoverage:     72.3,
		CriticalPathsCovered: true,
		Correctness:        "Good - minor issues found",
		Completeness:       "Complete - all requirements covered",
		IssuesFound:        []string{"Missing error handling in auth module"},
		Recommendations:    []string{"Add unit tests for edge cases"},
		Duration:           500 * time.Millisecond,
	}

	assert.Equal(t, "test-task-id", result.GetTaskID())
	assert.Equal(t, 500*time.Millisecond, result.GetDuration())
	assert.Equal(t, 100, result.TotalTests)
	assert.Equal(t, 95, result.PassedTests)
	assert.Equal(t, 3, result.FailedTests)
	assert.Equal(t, 85.5, result.LineCoverage)
	assert.True(t, result.CriticalPathsCovered)
	assert.Len(t, result.IssuesFound, 1)
	assert.Len(t, result.Recommendations, 1)
}

func TestTesterResult_GetError(t *testing.T) {
	result := &TesterResult{
		TaskID: "test-task-id",
		Error:  fmt.Errorf("test execution failed"),
	}

	assert.Equal(t, "test-task-id", result.GetTaskID())
	assert.NotNil(t, result.GetError())
	assert.Contains(t, result.GetError().Error(), "test execution failed")
}

func TestParseIntFromLine(t *testing.T) {
	assert.Equal(t, 42, parseIntFromLine("Passed: 42 tests"))
	assert.Equal(t, 100, parseIntFromLine("Total: 100"))
	assert.Equal(t, 0, parseIntFromLine("No numbers here"))
}

func TestParseFloatFromLine(t *testing.T) {
	assert.InDelta(t, 85.5, parseFloatFromLine("Coverage: 85.5%"), 0.1)
	assert.InDelta(t, 72.3, parseFloatFromLine("Branch: 72.3"), 0.1)
	assert.InDelta(t, 100.0, parseFloatFromLine("100%"), 0.1)
}

// ----------------------------------------------------------------------------
// Researcher Result Tests
// ----------------------------------------------------------------------------

func TestResearcherResult(t *testing.T) {
	result := &ResearcherResult{
		TaskID:   "test-task-id",
		Topic:    "microservices architecture",
		Summary:  "Microservices offer better scalability but increase complexity",
		Sources: []Source{
			{Index: 1, Description: "Martin Fowler'sMicroservices Guide", URL: "https://martinfowler.com"},
		},
		AlternativesAnalysis: []*Alternative{
			{
				Name:        "Monolith",
				Description: "Traditional monolithic architecture",
				Pros:        []string{"Simpler deployment", "Easier testing"},
				Cons:        []string{"Harder to scale", "Tightly coupled"},
			},
		},
		KnowledgeGaps:   []string{"Long-term maintenance costs"},
		Recommendations: []string{"Start with modular monolith, migrate to microservices when needed"},
		Duration:        300 * time.Millisecond,
	}

	assert.Equal(t, "test-task-id", result.GetTaskID())
	assert.Equal(t, 300*time.Millisecond, result.GetDuration())
	assert.Equal(t, "microservices architecture", result.Topic)
	assert.Len(t, result.Sources, 1)
	assert.Len(t, result.AlternativesAnalysis, 1)
	assert.Equal(t, "Monolith", result.AlternativesAnalysis[0].Name)
	assert.Len(t, result.KnowledgeGaps, 1)
	assert.Len(t, result.Recommendations, 1)
}

func TestResearcherResult_GetError(t *testing.T) {
	result := &ResearcherResult{
		TaskID: "test-task-id",
		Error:  fmt.Errorf("research timeout"),
	}

	assert.Equal(t, "test-task-id", result.GetTaskID())
	assert.NotNil(t, result.GetError())
	assert.Contains(t, result.GetError().Error(), "research timeout")
}

func TestSource(t *testing.T) {
	source := Source{
		Index:       1,
		Description: "Test Source",
		URL:         "https://example.com",
	}

	assert.Equal(t, 1, source.Index)
	assert.Equal(t, "Test Source", source.Description)
	assert.Equal(t, "https://example.com", source.URL)
}

func TestAlternative(t *testing.T) {
	alt := &Alternative{
		Name:        "Option A",
		Description: "First option",
		Pros:        []string{"Pro 1", "Pro 2"},
		Cons:        []string{"Con 1"},
	}

	assert.Equal(t, "Option A", alt.Name)
	assert.Equal(t, "First option", alt.Description)
	assert.Len(t, alt.Pros, 2)
	assert.Len(t, alt.Cons, 1)
}
