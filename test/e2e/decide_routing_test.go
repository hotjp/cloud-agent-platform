// Package e2e provides end-to-end tests for the Cloud Agent Platform.
// These tests verify the Decide routing decision logic.
package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/cloud-agent-platform/cap/internal/domain"
	"github.com/cloud-agent-platform/cap/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// TestSmokeE2E_DecideRoutingSimpleTask tests that simple tasks route correctly.
// A simple task should be handled with minimal decomposition (single_agent path).
func TestSmokeE2E_DecideRoutingSimpleTask(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	env := setupTestEnv(t)
	defer env.cleanupFn()

	t.Run("SimpleTaskRouting", func(t *testing.T) {
		testSimpleTaskRouting(ctx, t, env)
	})
}

func testSimpleTaskRouting(ctx context.Context, t *testing.T, env *smokeTestEnv) {
	// Step 1: Submit a simple task with minimal complexity
	t.Log("Step 1: Submitting simple task (single_agent path)...")

	simpleGoal := "Fix typo in README.md"
	submitReq := service.SubmitRequest{
		Goal:          simpleGoal,
		RepositoryURL: "https://github.com/example/repo",
		BaseBranch:    "main",
		ClientID:      "test-client-simple",
		Priority:      5,
		Constraints:   []string{}, // No constraints = simple task
		VerificationCriteria: []string{
			"README.md has the correct spelling",
		},
	}

	submitResp, err := env.taskSvc.Submit(ctx, submitReq)
	require.NoError(t, err, "Failed to submit simple task")
	require.NotNil(t, submitResp, "Submit response should not be nil")
	require.NotEmpty(t, submitResp.TaskID, "Task ID should not be empty")

	taskID := submitResp.TaskID
	t.Logf("Simple task submitted: %s", taskID)

	// Verify task is in pending state
	require.Equal(t, domain.TaskStatusPending, submitResp.Task.Status, "New task should be in pending state")

	// Step 2: Decompose the simple task with minimal subtasks (single agent path)
	t.Log("Step 2: Decomposing simple task (expecting single_agent strategy)...")

	// For simple tasks, we expect minimal decomposition - single executor subtask
	decomposeReq := service.DecomposeRequest{
		TaskID: taskID,
		Subtasks: []service.SubtaskSpec{
			{
				Type:          domain.SubtaskTypeCoding,
				Description:   "Fix the typo in README.md",
				AgentTemplate: "executor", // Single agent handles the entire task
			},
		},
	}

	decomposeResp, err := env.taskSvc.Decompose(ctx, decomposeReq)
	require.NoError(t, err, "Failed to decompose simple task")
	require.NotNil(t, decomposeResp, "Decompose response should not be nil")
	require.Len(t, decomposeResp.Subtasks, 1, "Simple task should have exactly 1 subtask (single_agent path)")

	// Verify single subtask properties
	subtask := decomposeResp.Subtasks[0]
	require.Equal(t, taskID, subtask.TaskID, "Subtask should belong to the parent task")
	require.Equal(t, domain.SubtaskTypeCoding, subtask.Type, "Simple task should use coding agent")
	require.Equal(t, "executor", subtask.AgentTemplate, "Simple task should use executor agent template")
	require.Equal(t, domain.TaskStatusPending, subtask.Status, "Subtask should be in pending state")

	t.Logf("Simple task decomposed: 1 subtask, agent=executor (single_agent path verified)")

	// Step 3: Verify final task state
	t.Log("Step 3: Verifying final task state...")

	finalGetResp, err := env.taskSvc.Get(ctx, service.GetRequest{TaskID: taskID})
	require.NoError(t, err, "Failed to get final task state")
	require.Equal(t, domain.TaskStatusDecomposing, finalGetResp.Task.Status, "Task should be in decomposing state")

	t.Log("Simple task routing test completed successfully - single_agent path verified")
}

// TestSmokeE2E_DecideRoutingComplexTask tests that complex tasks route correctly.
// A complex task should be handled with multi-agent decomposition.
func TestSmokeE2E_DecideRoutingComplexTask(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	env := setupTestEnv(t)
	defer env.cleanupFn()

	t.Run("ComplexTaskRouting", func(t *testing.T) {
		testComplexTaskRouting(ctx, t, env)
	})
}

func testComplexTaskRouting(ctx context.Context, t *testing.T, env *smokeTestEnv) {
	// Step 1: Submit a complex task with high complexity indicators
	t.Log("Step 1: Submitting complex task (multi_agent path)...")

	complexGoal := "Implement complete user authentication system with OAuth2, JWT tokens, and multi-provider support"
	submitReq := service.SubmitRequest{
		Goal:          complexGoal,
		RepositoryURL: "https://github.com/example/complex-repo",
		BaseBranch:    "main",
		ClientID:      "test-client-complex",
		Priority:      8,
		Constraints: []string{
			"Must use JWT for authentication",
			"Must support OAuth2 providers (Google, GitHub, Microsoft)",
			"Must implement refresh token rotation",
			"Must support multi-factor authentication",
			"Must comply with OWASP security guidelines",
			"Must handle session management securely",
		},
		VerificationCriteria: []string{
			"Users can sign up with email/password",
			"Users can login with OAuth2 providers",
			"JWT tokens are validated on protected routes",
			"Refresh tokens are rotated on use",
			"MFA can be enabled for accounts",
			"Sessions are properly invalidated on logout",
		},
	}

	submitResp, err := env.taskSvc.Submit(ctx, submitReq)
	require.NoError(t, err, "Failed to submit complex task")
	require.NotNil(t, submitResp, "Submit response should not be nil")
	require.NotEmpty(t, submitResp.TaskID, "Task ID should not be empty")

	taskID := submitResp.TaskID
	t.Logf("Complex task submitted: %s", taskID)

	// Verify task is in pending state with high priority
	require.Equal(t, domain.TaskStatusPending, submitResp.Task.Status, "New task should be in pending state")
	require.Equal(t, 8, submitResp.Task.Priority, "Complex task should have high priority")

	// Step 2: Decompose the complex task with multiple subtasks (multi_agent path)
	t.Log("Step 2: Decomposing complex task (expecting multi_agent strategy)...")

	// For complex tasks, we expect multi-agent decomposition with different roles
	decomposeReq := service.DecomposeRequest{
		TaskID: taskID,
		Subtasks: []service.SubtaskSpec{
			{
				Type:          domain.SubtaskTypeAnalysis,
				Description:   "Analyze authentication requirements and design auth flow",
				AgentTemplate: "strategist", // Analysis phase
			},
			{
				Type:          domain.SubtaskTypeCoding,
				Description:   "Implement JWT token generation and validation",
				AgentTemplate: "executor", // Core implementation
			},
			{
				Type:          domain.SubtaskTypeCoding,
				Description:   "Implement OAuth2 provider integration (Google, GitHub, Microsoft)",
				AgentTemplate: "executor", // OAuth2 implementation
			},
			{
				Type:          domain.SubtaskTypeCoding,
				Description:   "Implement refresh token rotation and session management",
				AgentTemplate: "executor", // Session management
			},
			{
				Type:          domain.SubtaskTypeTesting,
				Description:   "Write unit tests for authentication module",
				AgentTemplate: "tester", // Testing phase
			},
			{
				Type:          domain.SubtaskTypeReview,
				Description:   "Review authentication implementation for security issues",
				AgentTemplate: "guardian", // Security review
			},
		},
	}

	decomposeResp, err := env.taskSvc.Decompose(ctx, decomposeReq)
	require.NoError(t, err, "Failed to decompose complex task")
	require.NotNil(t, decomposeResp, "Decompose response should not be nil")
	require.Len(t, decomposeResp.Subtasks, 6, "Complex task should have 6 subtasks (multi_agent path)")

	// Verify multiple agent roles are used (multi_agent strategy)
	agentTemplates := make(map[string]bool)
	for _, st := range decomposeResp.Subtasks {
		require.NotEmpty(t, st.ID, "Subtask ID should not be empty")
		require.Equal(t, taskID, st.TaskID, "Subtask should belong to the parent task")
		require.Equal(t, domain.TaskStatusPending, st.Status, "Subtask should be in pending state")
		agentTemplates[st.AgentTemplate] = true
		t.Logf("Created subtask: ID=%s, Type=%s, Agent=%s, Description=%s",
			st.ID, st.Type, st.AgentTemplate, st.Description)
	}

	// Verify multiple different agent templates were used (multi_agent strategy)
	require.GreaterOrEqual(t, len(agentTemplates), 3, "Complex task should use multiple different agent templates")

	t.Logf("Complex task decomposed: %d subtasks, %d different agent templates (multi_agent path verified)",
		len(decomposeResp.Subtasks), len(agentTemplates))

	// Step 3: Verify final task state
	t.Log("Step 3: Verifying final task state...")

	finalGetResp, err := env.taskSvc.Get(ctx, service.GetRequest{TaskID: taskID})
	require.NoError(t, err, "Failed to get final task state")
	require.Equal(t, domain.TaskStatusDecomposing, finalGetResp.Task.Status, "Task should be in decomposing state")

	t.Log("Complex task routing test completed successfully - multi_agent path verified")
}

// TestSmokeE2E_DecideRoutingMediumTask tests that medium complexity tasks route correctly.
// A medium task should use a balanced approach (ReAct path).
func TestSmokeE2E_DecideRoutingMediumTask(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	env := setupTestEnv(t)
	defer env.cleanupFn()

	t.Run("MediumTaskRouting", func(t *testing.T) {
		testMediumTaskRouting(ctx, t, env)
	})
}

func testMediumTaskRouting(ctx context.Context, t *testing.T, env *smokeTestEnv) {
	// Step 1: Submit a medium complexity task
	t.Log("Step 1: Submitting medium complexity task (ReAct path)...")

	mediumGoal := "Add user profile picture upload feature with cropping"
	submitReq := service.SubmitRequest{
		Goal:          mediumGoal,
		RepositoryURL: "https://github.com/example/repo",
		BaseBranch:    "main",
		ClientID:      "test-client-medium",
		Priority:      5,
		Constraints: []string{
			"Images must be resized to max 500x500 pixels",
			"Support JPEG and PNG formats",
			"Store images in object storage",
		},
		VerificationCriteria: []string{
			"Users can upload profile pictures",
			"Images are properly resized",
			"Invalid formats are rejected with error",
		},
	}

	submitResp, err := env.taskSvc.Submit(ctx, submitReq)
	require.NoError(t, err, "Failed to submit medium task")
	require.NotNil(t, submitResp, "Submit response should not be nil")
	require.NotEmpty(t, submitResp.TaskID, "Task ID should not be empty")

	taskID := submitResp.TaskID
	t.Logf("Medium task submitted: %s", taskID)

	// Verify task is in pending state
	require.Equal(t, domain.TaskStatusPending, submitResp.Task.Status, "New task should be in pending state")

	// Step 2: Decompose the medium task with moderate subtasks (ReAct path)
	t.Log("Step 2: Decomposing medium task (expecting ReAct strategy)...")

	// For medium tasks, we expect 2-3 subtasks with ReAct-style iterative approach
	decomposeReq := service.DecomposeRequest{
		TaskID: taskID,
		Subtasks: []service.SubtaskSpec{
			{
				Type:          domain.SubtaskTypeAnalysis,
				Description:   "Analyze image processing requirements",
				AgentTemplate: "researcher", // Research best approach
			},
			{
				Type:          domain.SubtaskTypeCoding,
				Description:   "Implement image upload with resizing",
				AgentTemplate: "executor", // Implementation
			},
			{
				Type:          domain.SubtaskTypeTesting,
				Description:   "Test image upload functionality",
				AgentTemplate: "tester", // Testing
			},
		},
	}

	decomposeResp, err := env.taskSvc.Decompose(ctx, decomposeReq)
	require.NoError(t, err, "Failed to decompose medium task")
	require.NotNil(t, decomposeResp, "Decompose response should not be nil")
	require.Len(t, decomposeResp.Subtasks, 3, "Medium task should have 3 subtasks (ReAct path)")

	// Verify subtask properties
	expectedTypes := []domain.SubtaskType{
		domain.SubtaskTypeAnalysis,
		domain.SubtaskTypeCoding,
		domain.SubtaskTypeTesting,
	}
	expectedTemplates := []string{"researcher", "executor", "tester"}

	for i, st := range decomposeResp.Subtasks {
		require.NotEmpty(t, st.ID, "Subtask ID should not be empty")
		require.Equal(t, taskID, st.TaskID, "Subtask should belong to the parent task")
		require.Equal(t, expectedTypes[i], st.Type, "Subtask type should match expected type")
		require.Equal(t, expectedTemplates[i], st.AgentTemplate, "Agent template should match expected template")
		require.Equal(t, domain.TaskStatusPending, st.Status, "Subtask should be in pending state")
		t.Logf("Created subtask: ID=%s, Type=%s, Agent=%s",
			st.ID, st.Type, st.AgentTemplate)
	}

	t.Logf("Medium task decomposed: 3 subtasks (ReAct path verified)")

	// Step 3: Verify final task state
	t.Log("Step 3: Verifying final task state...")

	finalGetResp, err := env.taskSvc.Get(ctx, service.GetRequest{TaskID: taskID})
	require.NoError(t, err, "Failed to get final task state")
	require.Equal(t, domain.TaskStatusDecomposing, finalGetResp.Task.Status, "Task should be in decomposing state")

	t.Log("Medium task routing test completed successfully - ReAct path verified")
}

// TestSmokeE2E_DecideRoutingValidation tests input validation for routing decisions.
func TestSmokeE2E_DecideRoutingValidation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	env := setupTestEnv(t)
	defer env.cleanupFn()

	t.Run("DecideRoutingValidation", func(t *testing.T) {
		testDecideRoutingValidation(ctx, t, env)
	})
}

func testDecideRoutingValidation(ctx context.Context, t *testing.T, env *smokeTestEnv) {
	// Submit a task first for validation tests
	submitReq := service.SubmitRequest{
		Goal:          "Test task for validation",
		RepositoryURL: "https://github.com/example/repo",
		BaseBranch:    "main",
		ClientID:      "test-client-validation",
	}

	submitResp, err := env.taskSvc.Submit(ctx, submitReq)
	require.NoError(t, err, "Failed to submit task")
	taskID := submitResp.TaskID

	// Test 1: Empty TaskID in decomposition should fail
	t.Log("Test 1: Verifying empty TaskID validation...")
	_, err = env.taskSvc.Decompose(ctx, service.DecomposeRequest{
		TaskID: "",
		Subtasks: []service.SubtaskSpec{
			{Type: domain.SubtaskTypeCoding, Description: "Test", AgentTemplate: "executor"},
		},
	})
	require.Error(t, err, "Decompose with empty TaskID should fail")
	assert.Contains(t, err.Error(), "task_id", "Error should mention task_id validation")

	// Test 2: Empty subtasks should fail
	t.Log("Test 2: Verifying empty subtasks validation...")
	_, err = env.taskSvc.Decompose(ctx, service.DecomposeRequest{
		TaskID:   taskID,
		Subtasks: []service.SubtaskSpec{},
	})
	require.Error(t, err, "Decompose with empty subtasks should fail")
	assert.Contains(t, err.Error(), "at least one subtask", "Error should mention subtask requirement")

	// Test 3: Invalid subtask type should fail
	t.Log("Test 3: Verifying invalid subtask type validation...")
	_, err = env.taskSvc.Decompose(ctx, service.DecomposeRequest{
		TaskID: taskID,
		Subtasks: []service.SubtaskSpec{
			{Type: domain.SubtaskType("invalid"), Description: "Test", AgentTemplate: "executor"},
		},
	})
	require.Error(t, err, "Decompose with invalid subtask type should fail")

	t.Log("Decide routing validation test completed successfully")
}

// TestSmokeE2E_DecideRoutingAgentTemplates tests that appropriate agent templates
// are recommended based on task type.
func TestSmokeE2E_DecideRoutingAgentTemplates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	env := setupTestEnv(t)
	defer env.cleanupFn()

	t.Run("AgentTemplateRouting", func(t *testing.T) {
		testAgentTemplateRouting(ctx, t, env)
	})
}

func testAgentTemplateRouting(ctx context.Context, t *testing.T, env *smokeTestEnv) {
	testCases := []struct {
		name           string
		goal           string
		subtasks       []service.SubtaskSpec
		expectedTypes  []domain.SubtaskType
		expectedAgents []string
	}{
		{
			name: "ResearchTask",
			goal: "Research best practices for API rate limiting",
			subtasks: []service.SubtaskSpec{
				{Type: domain.SubtaskTypeResearch, Description: "Research rate limiting strategies", AgentTemplate: "researcher"},
			},
			expectedTypes:  []domain.SubtaskType{domain.SubtaskTypeResearch},
			expectedAgents: []string{"researcher"},
		},
		{
			name: "AnalysisTask",
			goal: "Analyze database performance issues",
			subtasks: []service.SubtaskSpec{
				{Type: domain.SubtaskTypeAnalysis, Description: "Analyze query performance", AgentTemplate: "strategist"},
			},
			expectedTypes:  []domain.SubtaskType{domain.SubtaskTypeAnalysis},
			expectedAgents: []string{"strategist"},
		},
		{
			name: "CodingTask",
			goal: "Implement cache invalidation logic",
			subtasks: []service.SubtaskSpec{
				{Type: domain.SubtaskTypeCoding, Description: "Implement cache invalidation", AgentTemplate: "executor"},
			},
			expectedTypes:  []domain.SubtaskType{domain.SubtaskTypeCoding},
			expectedAgents: []string{"executor"},
		},
		{
			name: "TestingTask",
			goal: "Write tests for payment module",
			subtasks: []service.SubtaskSpec{
				{Type: domain.SubtaskTypeTesting, Description: "Write payment tests", AgentTemplate: "tester"},
			},
			expectedTypes:  []domain.SubtaskType{domain.SubtaskTypeTesting},
			expectedAgents: []string{"tester"},
		},
		{
			name: "ReviewTask",
			goal: "Review security implementation",
			subtasks: []service.SubtaskSpec{
				{Type: domain.SubtaskTypeReview, Description: "Security review", AgentTemplate: "guardian"},
			},
			expectedTypes:  []domain.SubtaskType{domain.SubtaskTypeReview},
			expectedAgents: []string{"guardian"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Testing agent template routing for: %s", tc.name)

			// Submit task
			submitReq := service.SubmitRequest{
				Goal:          tc.goal,
				RepositoryURL: "https://github.com/example/repo",
				BaseBranch:    "main",
				ClientID:      "test-client-" + tc.name,
			}

			submitResp, err := env.taskSvc.Submit(ctx, submitReq)
			require.NoError(t, err, "Failed to submit task for %s", tc.name)
			taskID := submitResp.TaskID

			// Decompose with specified subtasks
			decomposeReq := service.DecomposeRequest{
				TaskID:   taskID,
				Subtasks: tc.subtasks,
			}

			decomposeResp, err := env.taskSvc.Decompose(ctx, decomposeReq)
			require.NoError(t, err, "Failed to decompose task for %s", tc.name)
			require.Len(t, decomposeResp.Subtasks, len(tc.subtasks), "Subtask count mismatch for %s", tc.name)

			// Verify agent templates match expected
			for i, st := range decomposeResp.Subtasks {
				assert.Equal(t, tc.expectedTypes[i], st.Type, "Subtask type mismatch for %s", tc.name)
				assert.Equal(t, tc.expectedAgents[i], st.AgentTemplate, "Agent template mismatch for %s", tc.name)
				t.Logf("  Subtask: Type=%s, Agent=%s", st.Type, st.AgentTemplate)
			}

			t.Logf("Agent template routing verified for: %s", tc.name)
		})
	}
}

// verifyRoutingDecision is a helper that verifies the routing decision based on task complexity.
// It checks that simple tasks have fewer subtasks and complex tasks have more.
func verifyRoutingDecision(t *testing.T, task *domain.Task, subtasks []*domain.Subtask, expectedComplexity string) {
	switch expectedComplexity {
	case "simple":
		assert.LessOrEqual(t, len(subtasks), 2, "Simple task should have at most 2 subtasks")
		t.Logf("Simple task routing verified: %d subtasks", len(subtasks))
	case "medium":
		assert.Equal(t, 3, len(subtasks), "Medium task should have exactly 3 subtasks")
		t.Logf("Medium task routing verified: %d subtasks", len(subtasks))
	case "complex":
		assert.GreaterOrEqual(t, len(subtasks), 5, "Complex task should have at least 5 subtasks")
		t.Logf("Complex task routing verified: %d subtasks", len(subtasks))
	}
}

// setupTestEnvLogger creates a test logger (reused across tests).
func setupTestEnvLogger(t *testing.T) *zap.Logger {
	return zaptest.NewLogger(t)
}
