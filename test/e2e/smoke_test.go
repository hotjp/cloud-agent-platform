// Package e2e provides end-to-end smoke tests for the Cloud Agent Platform.
// These tests verify the full task lifecycle: submit -> decompose -> execute -> verify.
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

// smokeTestEnv holds the test environment for E2E smoke tests.
type smokeTestEnv struct {
	t            *testing.T
	logger       *zap.Logger
	taskSvc      *service.TaskService
	taskRepo     *inMemoryTaskRepo
	subtaskRepo  *inMemorySubtaskRepo
	outboxWriter *inMemoryOutboxWriter
	txManager    *inMemoryTxManager
	cleanupFn    func()
}

// setupTestEnv creates a test environment with in-memory mocks.
func setupTestEnv(t *testing.T) *smokeTestEnv {
	logger := zaptest.NewLogger(t)

	// Create in-memory repositories
	taskRepo := newInMemoryTaskRepo()
	subtaskRepo := newInMemorySubtaskRepo()
	outboxWriter := newInMemoryOutboxWriter()
	txManager := newInMemoryTxManager()

	// Create TaskService
	taskSvc := service.NewTaskService(service.TaskServiceInput{
		TaskRepo:     taskRepo,
		SubtaskRepo:  subtaskRepo,
		OutboxWriter: outboxWriter,
		Storage:      txManager,
		Logger:       logger,
	})

	return &smokeTestEnv{
		t:            t,
		logger:       logger,
		taskSvc:      taskSvc,
		taskRepo:     taskRepo,
		subtaskRepo:  subtaskRepo,
		outboxWriter: outboxWriter,
		txManager:    txManager,
		cleanupFn:    func() {},
	}
}

// TestMain sets up the test environment.
func TestMain(m *testing.M) {
	m.Run()
}

// TestSmokeE2E_FullTaskLifecycle tests the complete task lifecycle:
// 1. Submit a task
// 2. Verify task is created in pending state
// 3. Decompose the task into subtasks
// 4. Verify subtasks are created
// 5. Verify task status transitions to decomposing
func TestSmokeE2E_FullTaskLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	env := setupTestEnv(t)

	t.Run("FullTaskLifecycle", func(t *testing.T) {
		testFullTaskLifecycle(ctx, t, env)
	})
}

func testFullTaskLifecycle(ctx context.Context, t *testing.T, env *smokeTestEnv) {
	// Step 1: Submit a new task
	t.Log("Step 1: Submitting new task...")

	submitReq := service.SubmitRequest{
		Goal:          "Implement user authentication feature",
		RepositoryURL: "https://github.com/example/test-repo",
		BaseBranch:    "main",
		ClientID:      "test-client-001",
		Priority:      5,
		Constraints: []string{
			"Must use JWT for authentication",
			"Must support OAuth2 providers",
		},
		VerificationCriteria: []string{
			"Users can sign up with email",
			"Users can login with email/password",
			"JWT tokens are validated on protected routes",
		},
	}

	submitResp, err := env.taskSvc.Submit(ctx, submitReq)
	require.NoError(t, err, "Failed to submit task")
	require.NotNil(t, submitResp, "Submit response should not be nil")
	require.NotEmpty(t, submitResp.TaskID, "Task ID should not be empty")

	taskID := submitResp.TaskID
	t.Logf("Task submitted successfully: %s", taskID)

	// Verify task was created with correct initial state
	require.Equal(t, domain.TaskStatusPending, submitResp.Task.Status, "New task should be in pending state")
	require.Equal(t, "Implement user authentication feature", submitResp.Task.Goal)
	require.Equal(t, "test-client-001", submitResp.Task.ClientID)
	require.Equal(t, "main", submitResp.Task.BaseBranch)

	// Step 2: Verify task can be retrieved
	t.Log("Step 2: Verifying task retrieval...")

	getResp, err := env.taskSvc.Get(ctx, service.GetRequest{TaskID: taskID})
	require.NoError(t, err, "Failed to get task")
	require.Equal(t, taskID, getResp.Task.ID, "Retrieved task ID should match")
	require.Equal(t, domain.TaskStatusPending, getResp.Task.Status, "Task should still be in pending state")

	// Step 3: Decompose the task into subtasks
	t.Log("Step 3: Decomposing task into subtasks...")

	decomposeReq := service.DecomposeRequest{
		TaskID: taskID,
		Subtasks: []service.SubtaskSpec{
			{
				Type:          domain.SubtaskTypeAnalysis,
				Description:   "Analyze authentication requirements and design auth flow",
				AgentTemplate: "strategist",
			},
			{
				Type:          domain.SubtaskTypeCoding,
				Description:   "Implement JWT token generation and validation",
				AgentTemplate: "executor",
			},
			{
				Type:          domain.SubtaskTypeTesting,
				Description:   "Write unit tests for authentication module",
				AgentTemplate: "tester",
			},
			{
				Type:          domain.SubtaskTypeReview,
				Description:   "Review authentication implementation",
				AgentTemplate: "guardian",
			},
		},
	}

	decomposeResp, err := env.taskSvc.Decompose(ctx, decomposeReq)
	require.NoError(t, err, "Failed to decompose task")
	require.NotNil(t, decomposeResp, "Decompose response should not be nil")
	require.Len(t, decomposeResp.Subtasks, 4, "Should create exactly 4 subtasks")

	// Verify task status transitioned to decomposing
	require.Equal(t, domain.TaskStatusDecomposing, decomposeResp.Task.Status, "Task should be in decomposing state after decomposition")

	// Verify subtask properties
	expectedSubtaskTypes := []domain.SubtaskType{
		domain.SubtaskTypeAnalysis,
		domain.SubtaskTypeCoding,
		domain.SubtaskTypeTesting,
		domain.SubtaskTypeReview,
	}

	for i, subtask := range decomposeResp.Subtasks {
		require.NotEmpty(t, subtask.ID, "Subtask ID should not be empty")
		require.Equal(t, taskID, subtask.TaskID, "Subtask should belong to the parent task")
		require.Equal(t, expectedSubtaskTypes[i], subtask.Type, "Subtask type should match expected type")
		require.Equal(t, domain.TaskStatusPending, subtask.Status, "Subtask should be in pending state initially")
		t.Logf("Created subtask: %s (type: %s, description: %s)", subtask.ID, subtask.Type, subtask.Description)
	}

	// Step 4: Verify all subtasks can be retrieved
	t.Log("Step 4: Verifying subtask retrieval...")

	subtasks, err := env.subtaskRepo.ListByTaskID(ctx, taskID)
	require.NoError(t, err, "Failed to list subtasks")
	require.Len(t, subtasks, 4, "Should have 4 subtasks")

	// Step 5: Verify task is still in decomposing state (not automatically transitioning)
	t.Log("Step 5: Verifying final task state...")

	finalGetResp, err := env.taskSvc.Get(ctx, service.GetRequest{TaskID: taskID})
	require.NoError(t, err, "Failed to get final task state")
	require.Equal(t, domain.TaskStatusDecomposing, finalGetResp.Task.Status, "Task should remain in decomposing state")
	require.GreaterOrEqual(t, finalGetResp.Task.Version, int64(2), "Task version should be incremented after decomposition")

	t.Log("E2E smoke test completed successfully!")
}

// TestSmokeE2E_TaskLifecycleWithCancellation tests task lifecycle with cancellation.
func TestSmokeE2E_TaskLifecycleWithCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	env := setupTestEnv(t)

	t.Run("TaskLifecycleWithCancellation", func(t *testing.T) {
		testTaskLifecycleWithCancellation(ctx, t, env)
	})
}

func testTaskLifecycleWithCancellation(ctx context.Context, t *testing.T, env *smokeTestEnv) {
	t.Log("Submitting task for cancellation test...")

	submitReq := service.SubmitRequest{
		Goal:          "Task that will be cancelled",
		RepositoryURL: "https://github.com/example/repo",
		BaseBranch:    "develop",
		ClientID:      "test-client-002",
	}

	submitResp, err := env.taskSvc.Submit(ctx, submitReq)
	require.NoError(t, err, "Failed to submit task")

	taskID := submitResp.TaskID
	require.Equal(t, domain.TaskStatusPending, submitResp.Task.Status)

	// Cancel the task
	t.Log("Cancelling task...")

	cancelResp, err := env.taskSvc.Cancel(ctx, service.CancelRequest{
		TaskID: taskID,
		Reason: "Test cancellation",
	})
	require.NoError(t, err, "Failed to cancel task")
	require.Equal(t, domain.TaskStatusCancelled, cancelResp.Task.Status)

	// Verify task is cancelled
	getResp, err := env.taskSvc.Get(ctx, service.GetRequest{TaskID: taskID})
	require.NoError(t, err, "Failed to get cancelled task")
	require.Equal(t, domain.TaskStatusCancelled, getResp.Task.Status)

	t.Log("Cancellation test completed successfully!")
}

// TestSmokeE2E_SubmitValidation tests input validation during task submission.
func TestSmokeE2E_SubmitValidation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	env := setupTestEnv(t)

	t.Run("SubmitValidation", func(t *testing.T) {
		testSubmitValidation(ctx, t, env)
	})
}

func testSubmitValidation(ctx context.Context, t *testing.T, env *smokeTestEnv) {
	testCases := []struct {
		name    string
		req     service.SubmitRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "Empty goal",
			req: service.SubmitRequest{
				Goal:          "",
				RepositoryURL: "https://github.com/example/repo",
				BaseBranch:    "main",
				ClientID:      "client1",
			},
			wantErr: true,
			errMsg:  "goal",
		},
		{
			name: "Empty repository URL",
			req: service.SubmitRequest{
				Goal:          "Valid goal",
				RepositoryURL: "",
				BaseBranch:    "main",
				ClientID:      "client1",
			},
			wantErr: true,
			errMsg:  "repository_url",
		},
		{
			name: "Empty base branch",
			req: service.SubmitRequest{
				Goal:          "Valid goal",
				RepositoryURL: "https://github.com/example/repo",
				BaseBranch:    "",
				ClientID:      "client1",
			},
			wantErr: true,
			errMsg:  "base_branch",
		},
		{
			name: "Empty client ID",
			req: service.SubmitRequest{
				Goal:          "Valid goal",
				RepositoryURL: "https://github.com/example/repo",
				BaseBranch:    "main",
				ClientID:      "",
			},
			wantErr: true,
			errMsg:  "client_id",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := env.taskSvc.Submit(ctx, tc.req)

			if tc.wantErr {
				require.Error(t, err, "Expected error for: %s", tc.name)
				assert.Contains(t, err.Error(), tc.errMsg, "Error should contain validation field")
				assert.Nil(t, resp, "Response should be nil on validation error")
			} else {
				require.NoError(t, err, "Should not error for: %s", tc.name)
			}
		})
	}
}

// TestSmokeE2E_DecomposeValidation tests input validation during task decomposition.
func TestSmokeE2E_DecomposeValidation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	env := setupTestEnv(t)

	t.Run("DecomposeValidation", func(t *testing.T) {
		testDecomposeValidation(ctx, t, env)
	})
}

func testDecomposeValidation(ctx context.Context, t *testing.T, env *smokeTestEnv) {
	submitReq := service.SubmitRequest{
		Goal:          "Task for decompose validation",
		RepositoryURL: "https://github.com/example/repo",
		BaseBranch:    "main",
		ClientID:      "test-client",
	}

	submitResp, err := env.taskSvc.Submit(ctx, submitReq)
	require.NoError(t, err)

	taskID := submitResp.TaskID

	testCases := []struct {
		name    string
		req     service.DecomposeRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "Empty task ID",
			req: service.DecomposeRequest{
				TaskID: "",
				Subtasks: []service.SubtaskSpec{
					{Type: domain.SubtaskTypeCoding, Description: "Test", AgentTemplate: "executor"},
				},
			},
			wantErr: true,
			errMsg:  "task_id",
		},
		{
			name: "Empty subtasks",
			req: service.DecomposeRequest{
				TaskID:   taskID,
				Subtasks: []service.SubtaskSpec{},
			},
			wantErr: true,
			errMsg:  "at least one subtask",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := env.taskSvc.Decompose(ctx, tc.req)

			if tc.wantErr {
				require.Error(t, err, "Expected error for: %s", tc.name)
				assert.Contains(t, err.Error(), tc.errMsg, "Error should contain validation message")
				assert.Nil(t, resp, "Response should be nil on validation error")
			} else {
				require.NoError(t, err, "Should not error for: %s", tc.name)
			}
		})
	}
}

// TestSmokeE2E_StateTransitions tests the task state transitions:
// pending → decomposing → dispatched → running → reviewing → completed
func TestSmokeE2E_StateTransitions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	env := setupTestEnv(t)

	t.Run("StateTransitions", func(t *testing.T) {
		testSmokeStateTransitions(ctx, t, env)
	})
}

func testSmokeStateTransitions(ctx context.Context, t *testing.T, env *smokeTestEnv) {
	// Step 1: Submit a new task and verify it's in pending state
	t.Log("Step 1: Submitting new task (should be pending)...")

	submitReq := service.SubmitRequest{
		Goal:          "Implement state transition test",
		RepositoryURL: "https://github.com/example/test-repo",
		BaseBranch:    "main",
		ClientID:      "test-client-state",
		Priority:      5,
	}

	submitResp, err := env.taskSvc.Submit(ctx, submitReq)
	require.NoError(t, err, "Failed to submit task")
	require.Equal(t, domain.TaskStatusPending, submitResp.Task.Status, "New task should be in pending state")

	taskID := submitResp.TaskID
	version := submitResp.Task.Version
	t.Logf("Task submitted: %s, status=%s, version=%d", taskID, submitResp.Task.Status, version)

	// Step 2: Decompose the task and verify it transitions to decomposing
	t.Log("Step 2: Decomposing task (pending → decomposing)...")

	decomposeReq := service.DecomposeRequest{
		TaskID: taskID,
		Subtasks: []service.SubtaskSpec{
			{
				Type:          domain.SubtaskTypeCoding,
				Description:   "Implement feature",
				AgentTemplate: "executor",
			},
		},
	}

	decomposeResp, err := env.taskSvc.Decompose(ctx, decomposeReq)
	require.NoError(t, err, "Failed to decompose task")
	require.Equal(t, domain.TaskStatusDecomposing, decomposeResp.Task.Status, "Task should be in decomposing state after decomposition")
	require.Greater(t, decomposeResp.Task.Version, version, "Version should increment after decomposition")
	version = decomposeResp.Task.Version
	t.Logf("Task decomposed: %s, status=%s, version=%d", taskID, decomposeResp.Task.Status, version)

	// Step 3: Transition to dispatched using state machine (decomposing → dispatched)
	t.Log("Step 3: Transitioning to dispatched (decomposing → dispatched)...")

	task, err := env.taskRepo.GetByID(ctx, taskID)
	require.NoError(t, err, "Failed to get task for state transition")

	err = task.TransitionTo("DecompositionComplete")
	require.NoError(t, err, "Transition from decomposing to dispatched should succeed")

	// Update via repository to persist
	task, err = env.taskRepo.Update(ctx, task)
	require.NoError(t, err, "Failed to persist dispatched state")
	require.Equal(t, domain.TaskStatusDispatched, task.Status, "Task should be dispatched")
	require.Greater(t, task.Version, version, "Version should increment after transition")
	version = task.Version
	t.Logf("Task dispatched: %s, status=%s, version=%d", taskID, task.Status, version)

	// Step 4: Transition to running (dispatched → running)
	t.Log("Step 4: Transitioning to running (dispatched → running)...")

	err = task.TransitionTo("StartExecution")
	require.NoError(t, err, "Transition from dispatched to running should succeed")

	task, err = env.taskRepo.Update(ctx, task)
	require.NoError(t, err, "Failed to persist running state")
	require.Equal(t, domain.TaskStatusRunning, task.Status, "Task should be running")
	require.Greater(t, task.Version, version, "Version should increment after transition")
	version = task.Version
	t.Logf("Task running: %s, status=%s, version=%d", taskID, task.Status, version)

	// Step 5: Transition to reviewing (running → reviewing)
	t.Log("Step 5: Transitioning to reviewing (running → reviewing)...")

	err = task.TransitionTo("CompleteExecution")
	require.NoError(t, err, "Transition from running to reviewing should succeed")

	task, err = env.taskRepo.Update(ctx, task)
	require.NoError(t, err, "Failed to persist reviewing state")
	require.Equal(t, domain.TaskStatusReviewing, task.Status, "Task should be reviewing")
	require.Greater(t, task.Version, version, "Version should increment after transition")
	version = task.Version
	t.Logf("Task reviewing: %s, status=%s, version=%d", taskID, task.Status, version)

	// Step 6: Transition to completed (reviewing → completed)
	t.Log("Step 6: Transitioning to completed (reviewing → completed)...")

	err = task.MarkCompleted()
	require.NoError(t, err, "Transition from reviewing to completed should succeed")

	task, err = env.taskRepo.Update(ctx, task)
	require.NoError(t, err, "Failed to persist completed state")
	require.Equal(t, domain.TaskStatusCompleted, task.Status, "Task should be completed")
	require.Greater(t, task.Version, version, "Version should increment after transition")
	require.NotNil(t, task.CompletedAt, "CompletedAt should be set")
	t.Logf("Task completed: %s, status=%s, version=%d, completedAt=%s", taskID, task.Status, task.Version, task.CompletedAt)

	// Step 7: Verify task is in completed state via service Get
	t.Log("Step 7: Verifying final state via service.Get...")

	getResp, err := env.taskSvc.Get(ctx, service.GetRequest{TaskID: taskID})
	require.NoError(t, err, "Failed to get completed task")
	require.Equal(t, domain.TaskStatusCompleted, getResp.Task.Status, "Task should still be completed")
	require.NotNil(t, getResp.Task.CompletedAt, "CompletedAt should be set in retrieved task")

	t.Log("State transition E2E test completed successfully!")
}

// TestSmokeE2E_InvalidStateTransitions tests that invalid state transitions are rejected
func TestSmokeE2E_InvalidStateTransitions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	env := setupTestEnv(t)

	t.Run("InvalidTransitions", func(t *testing.T) {
		testInvalidStateTransitions(ctx, t, env)
	})
}

func testInvalidStateTransitions(ctx context.Context, t *testing.T, env *smokeTestEnv) {
	submitReq := service.SubmitRequest{
		Goal:          "Test invalid transitions",
		RepositoryURL: "https://github.com/example/repo",
		BaseBranch:    "main",
		ClientID:      "test-client-invalid",
	}

	submitResp, err := env.taskSvc.Submit(ctx, submitReq)
	require.NoError(t, err, "Failed to submit task")
	taskID := submitResp.TaskID
	t.Logf("Task submitted: %s, status=%s", taskID, submitResp.Task.Status)

	task, err := env.taskRepo.GetByID(ctx, taskID)
	require.NoError(t, err)

	// Step 1: Cannot go directly from pending to running (invalid)
	t.Log("Testing: pending → running should fail")
	err = task.TransitionTo("StartExecution")
	require.Error(t, err, "Transition from pending to running should fail")
	require.Equal(t, domain.TaskStatusPending, task.Status, "Status should remain pending after failed transition")

	// Step 2: pending → decomposing is valid
	t.Log("Testing: pending → decomposing (valid)")
	err = task.TransitionTo("StartDecomposition")
	require.NoError(t, err, "Transition from pending to decomposing should succeed")
	task, err = env.taskRepo.Update(ctx, task)
	require.NoError(t, err)
	require.Equal(t, domain.TaskStatusDecomposing, task.Status)

	// Step 3: Cannot go from decomposing directly to running (must go through dispatched)
	t.Log("Testing: decomposing → running should fail")
	err = task.TransitionTo("StartExecution")
	require.Error(t, err, "Transition from decomposing to running should fail")
	require.Equal(t, domain.TaskStatusDecomposing, task.Status, "Status should remain decomposing")

	// Step 4: decomposing → dispatched is valid
	t.Log("Testing: decomposing → dispatched (valid)")
	err = task.TransitionTo("DecompositionComplete")
	require.NoError(t, err)
	task, err = env.taskRepo.Update(ctx, task)
	require.NoError(t, err)
	require.Equal(t, domain.TaskStatusDispatched, task.Status)

	// Step 5: dispatched → running is valid
	t.Log("Testing: dispatched → running (valid)")
	err = task.TransitionTo("StartExecution")
	require.NoError(t, err)
	task, err = env.taskRepo.Update(ctx, task)
	require.NoError(t, err)
	require.Equal(t, domain.TaskStatusRunning, task.Status)

	// Step 6: Cannot go from running directly to completed (must go through reviewing)
	t.Log("Testing: running → completed should fail")
	err = task.TransitionTo("ReviewPassed")
	require.Error(t, err, "Transition from running to completed should fail")
	require.Equal(t, domain.TaskStatusRunning, task.Status, "Status should remain running")

	// Step 7: running → reviewing is valid
	t.Log("Testing: running → reviewing (valid)")
	err = task.TransitionTo("CompleteExecution")
	require.NoError(t, err)
	task, err = env.taskRepo.Update(ctx, task)
	require.NoError(t, err)
	require.Equal(t, domain.TaskStatusReviewing, task.Status)

	// Step 8: reviewing → completed is valid
	t.Log("Testing: reviewing → completed (valid)")
	err = task.TransitionTo("ReviewPassed")
	require.NoError(t, err)
	task, err = env.taskRepo.Update(ctx, task)
	require.NoError(t, err)
	require.Equal(t, domain.TaskStatusCompleted, task.Status)

	t.Log("Invalid state transition test completed successfully!")
}

// TestSmokeE2E_ConcurrentSubmit tests concurrent task submissions.
func TestSmokeE2E_ConcurrentSubmit(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	env := setupTestEnv(t)

	t.Run("ConcurrentSubmit", func(t *testing.T) {
		testConcurrentSubmit(ctx, t, env)
	})
}

func testConcurrentSubmit(ctx context.Context, t *testing.T, env *smokeTestEnv) {
	numTasks := 10
	t.Logf("Submitting %d tasks concurrently...", numTasks)

	submitChan := make(chan *service.SubmitResponse, numTasks)
	errChan := make(chan error, numTasks)

	for i := 0; i < numTasks; i++ {
		go func(idx int) {
			req := service.SubmitRequest{
				Goal:          "Concurrent task",
				RepositoryURL: "https://github.com/example/repo",
				BaseBranch:    "main",
				ClientID:      "test-client",
			}
			resp, err := env.taskSvc.Submit(ctx, req)
			if err != nil {
				errChan <- err
				return
			}
			submitChan <- resp
		}(i)
	}

	var successCount, errorCount int
	for i := 0; i < numTasks; i++ {
		select {
		case resp := <-submitChan:
			require.NotNil(t, resp, "Submit response should not be nil")
			require.NotEmpty(t, resp.TaskID, "Task ID should not be empty")
			successCount++
			t.Logf("Task %d submitted: %s", i+1, resp.TaskID)
		case err := <-errChan:
			require.NoError(t, err, "Submit should not error concurrently")
			errorCount++
		case <-ctx.Done():
			t.Fatal("Test timed out")
		}
	}

	assert.Equal(t, numTasks, successCount, "All tasks should be submitted successfully")
	assert.Equal(t, 0, errorCount, "No tasks should fail")

	t.Logf("Successfully submitted %d tasks concurrently", successCount)
}
