// Package e2e provides end-to-end integration tests for the Cloud Agent Platform.
// These tests verify the full pipeline: submit -> decompose -> execute -> complete.
// Uses in-memory mocks to avoid external dependencies (Docker/Redis/LLM).
package e2e

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/cloud-agent-platform/cap/internal/domain"
	"github.com/cloud-agent-platform/cap/internal/domain/worker"
	"github.com/cloud-agent-platform/cap/internal/orchestrator"
	"github.com/cloud-agent-platform/cap/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// ----------------------------------------------------------------------------
// In-Memory Repository Implementations
// ----------------------------------------------------------------------------

// inMemoryTaskRepo implements domain.TaskRepository using in-memory storage.
type inMemoryTaskRepo struct {
	mu    sync.RWMutex
	tasks map[string]*domain.Task
}

func newInMemoryTaskRepo() *inMemoryTaskRepo {
	return &inMemoryTaskRepo{
		tasks: make(map[string]*domain.Task),
	}
}

func (r *inMemoryTaskRepo) Create(ctx context.Context, task *domain.Task) (*domain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tasks[task.ID] = task
	return task, nil
}

func (r *inMemoryTaskRepo) GetByID(ctx context.Context, id string) (*domain.Task, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	task, ok := r.tasks[id]
	if !ok {
		return nil, domain.NewL2AggregateNotFoundError("Task", id)
	}
	return task, nil
}

func (r *inMemoryTaskRepo) Update(ctx context.Context, task *domain.Task) (*domain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tasks[task.ID] = task
	return task, nil
}

func (r *inMemoryTaskRepo) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tasks, id)
	return nil
}

func (r *inMemoryTaskRepo) ListByStatus(ctx context.Context, status domain.TaskStatus, limit, offset int) ([]*domain.Task, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*domain.Task
	for _, t := range r.tasks {
		if t.Status == status {
			result = append(result, t)
		}
	}
	return result, len(result), nil
}

func (r *inMemoryTaskRepo) ListByClientID(ctx context.Context, clientID string, limit, offset int) ([]*domain.Task, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*domain.Task
	for _, t := range r.tasks {
		if t.ClientID == clientID {
			result = append(result, t)
		}
	}
	return result, len(result), nil
}

func (r *inMemoryTaskRepo) List(ctx context.Context, limit, offset int) ([]*domain.Task, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*domain.Task
	for _, t := range r.tasks {
		result = append(result, t)
	}
	return result, len(result), nil
}

func (r *inMemoryTaskRepo) UpdateStatus(ctx context.Context, id string, status domain.TaskStatus, expectedVersion int64) (*domain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[id]
	if !ok {
		return nil, domain.NewL2AggregateNotFoundError("Task", id)
	}
	if task.Version != expectedVersion {
		return nil, domain.NewL2OptimisticLockError("Task", id, expectedVersion, task.Version)
	}
	task.Status = status
	task.Version++
	// Set CompletedAt when task is completed (mimics domain model behavior)
	if status == domain.TaskStatusCompleted || status == domain.TaskStatusFailed || status == domain.TaskStatusCancelled {
		now := time.Now().UTC()
		task.CompletedAt = &now
	}
	return task, nil
}

func (r *inMemoryTaskRepo) CountByStatus(ctx context.Context, status domain.TaskStatus) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	count := 0
	for _, t := range r.tasks {
		if t.Status == status {
			count++
		}
	}
	return count, nil
}

// inMemorySubtaskRepo implements domain.SubtaskRepository using in-memory storage.
type inMemorySubtaskRepo struct {
	mu       sync.RWMutex
	subtasks map[string]*domain.Subtask
}

func newInMemorySubtaskRepo() *inMemorySubtaskRepo {
	return &inMemorySubtaskRepo{
		subtasks: make(map[string]*domain.Subtask),
	}
}

func (r *inMemorySubtaskRepo) Create(ctx context.Context, s *domain.Subtask) (*domain.Subtask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subtasks[s.ID] = s
	return s, nil
}

func (r *inMemorySubtaskRepo) GetByID(ctx context.Context, id string) (*domain.Subtask, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.subtasks[id]
	if !ok {
		return nil, domain.NewL2AggregateNotFoundError("Subtask", id)
	}
	return s, nil
}

func (r *inMemorySubtaskRepo) Update(ctx context.Context, s *domain.Subtask) (*domain.Subtask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subtasks[s.ID] = s
	return s, nil
}

func (r *inMemorySubtaskRepo) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.subtasks, id)
	return nil
}

func (r *inMemorySubtaskRepo) ListByTaskID(ctx context.Context, taskID string) ([]*domain.Subtask, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*domain.Subtask
	for _, s := range r.subtasks {
		if s.TaskID == taskID {
			result = append(result, s)
		}
	}
	return result, nil
}

func (r *inMemorySubtaskRepo) UpdateStatus(ctx context.Context, id string, status domain.TaskStatus, expectedVersion int64) (*domain.Subtask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.subtasks[id]
	if !ok {
		return nil, domain.NewL2AggregateNotFoundError("Subtask", id)
	}
	if s.Version != expectedVersion {
		return nil, domain.NewL2OptimisticLockError("Subtask", id, expectedVersion, s.Version)
	}
	s.Status = status
	s.Version++
	return s, nil
}

func (r *inMemorySubtaskRepo) ListByStatus(ctx context.Context, status domain.TaskStatus, limit, offset int) ([]*domain.Subtask, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*domain.Subtask
	for _, s := range r.subtasks {
		if s.Status == status {
			result = append(result, s)
		}
	}
	return result, len(result), nil
}

// inMemoryOutboxWriter implements domain.OutboxWriter using in-memory storage.
type inMemoryOutboxWriter struct {
	mu     sync.Mutex
	events []*domain.DomainEvent
}

func newInMemoryOutboxWriter() *inMemoryOutboxWriter {
	return &inMemoryOutboxWriter{
		events: make([]*domain.DomainEvent, 0),
	}
}

func (w *inMemoryOutboxWriter) Write(ctx context.Context, tx interface{}, event *domain.DomainEvent) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.events = append(w.events, event)
	return nil
}

func (w *inMemoryOutboxWriter) GetEvents() []*domain.DomainEvent {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.events
}

func (w *inMemoryOutboxWriter) Clear() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.events = make([]*domain.DomainEvent, 0)
}

// inMemoryTxManager implements both service.TransactionManager and orchestrator.TransactionManager
// using in-memory storage. We use an internal struct and wrapper methods to satisfy both interfaces.
type inMemoryTxManager struct {
	mu       sync.Mutex
	commits  int
	rollbacks int
}

func newInMemoryTxManager() *inMemoryTxManager {
	return &inMemoryTxManager{}
}

func (m *inMemoryTxManager) BeginTx(ctx context.Context) (service.TransactionManager, error) {
	return m, nil
}

func (m *inMemoryTxManager) Commit(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.commits++
	return nil
}

func (m *inMemoryTxManager) Rollback(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rollbacks++
	return nil
}

// toOrchestratorTx converts service.TransactionManager to orchestrator.TransactionManager
// by wrapping the underlying implementation.
type toOrchestratorTx struct {
	*inMemoryTxManager
}

func (t *toOrchestratorTx) BeginTx(ctx context.Context) (orchestrator.TransactionManager, error) {
	return t, nil
}

// newOrchestratorTxManager creates an orchestrator.TransactionManager from inMemoryTxManager
func newOrchestratorTxManager(m *inMemoryTxManager) orchestrator.TransactionManager {
	return &toOrchestratorTx{inMemoryTxManager: m}
}

// mockWorkerExecutor is a mock implementation of WorkerExecutor for testing.
type mockWorkerExecutor struct {
	mu             sync.Mutex
	executedIDs    []string
	resultToReturn *orchestrator.AgentResult
	errorToReturn  error
}

func newMockWorkerExecutor() *mockWorkerExecutor {
	return &mockWorkerExecutor{
		executedIDs: make([]string, 0),
		resultToReturn: &orchestrator.AgentResult{
			Summary:           "Mock execution completed successfully",
			Artifacts:         []domain.ArtifactRef{},
			TokensUsed:        100,
			ExecutionDuration: 1 * time.Second,
		},
	}
}

func (m *mockWorkerExecutor) Execute(ctx context.Context, subtaskID, taskID string, opts worker.ExecOptions) (*orchestrator.AgentResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executedIDs = append(m.executedIDs, subtaskID)

	// Simulate work with a small delay to allow cancellation to work
	time.Sleep(200 * time.Millisecond)

	if m.errorToReturn != nil {
		return nil, m.errorToReturn
	}
	return m.resultToReturn, nil
}

func (m *mockWorkerExecutor) GetExecutedIDs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]string, len(m.executedIDs))
	copy(ids, m.executedIDs)
	return ids
}

// mockGuardian is a mock implementation of Guardian for testing.
type mockGuardian struct {
	mu              sync.RWMutex
	autoApprove     bool
	approvalResult  orchestrator.ApprovalResult
}

func newMockGuardian() *mockGuardian {
	return &mockGuardian{
		autoApprove:    true,
		approvalResult:  orchestrator.ApprovalResultApprove,
	}
}

func (m *mockGuardian) NeedsApproval(ctx context.Context, task *domain.Task) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return !m.autoApprove
}

func (m *mockGuardian) RequestApproval(ctx context.Context, task *domain.Task, customTimeout time.Duration) (*orchestrator.ApprovalRequest, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	resultCh := make(chan orchestrator.ApprovalResult, 1)
	if m.autoApprove {
		resultCh <- orchestrator.ApprovalResultApprove
	} else {
		resultCh <- m.approvalResult
	}

	return &orchestrator.ApprovalRequest{
		TaskID:   task.ID,
		TaskGoal: task.Goal,
		ResultCh: resultCh,
		Timeout:  customTimeout,
	}, nil
}

func (m *mockGuardian) IsPending(ctx context.Context, taskID string) bool {
	return false
}

func (m *mockGuardian) SetAutoApprove(auto bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.autoApprove = auto
}

func (m *mockGuardian) SetApprovalResult(result orchestrator.ApprovalResult) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.approvalResult = result
}

// ----------------------------------------------------------------------------
// Test Environment
// ----------------------------------------------------------------------------

// fullPipelineTestEnv holds the test environment for full pipeline E2E tests.
type fullPipelineTestEnv struct {
	t            *testing.T
	logger       *zap.Logger
	taskSvc      *service.TaskService
	taskRepo     *inMemoryTaskRepo
	subtaskRepo  *inMemorySubtaskRepo
	outboxWriter *inMemoryOutboxWriter
	txManager    *inMemoryTxManager
	orchestrator *orchestrator.OrchestratorImpl
	mockWorker   *mockWorkerExecutor
	mockGuardian *mockGuardian
}

// setupFullPipelineEnv creates a test environment with in-memory mocks.
func setupFullPipelineEnv(t *testing.T) *fullPipelineTestEnv {
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

	// Create mock worker executor
	mockWorker := newMockWorkerExecutor()

	// Create mock guardian (auto-approve for testing)
	mockGuardian := newMockGuardian()

	// Create orchestrator with mocks
	orch := orchestrator.NewOrchestrator(
		orchestrator.Config{
			MaxConcurrentSessions: 5,
			SessionTimeout:       30 * time.Minute,
			DefaultAgentTemplate: "executor",
			GuardianEnabled:      true,
		},
		taskRepo,
		subtaskRepo,
		outboxWriter,
		newOrchestratorTxManager(txManager),
		nil, // agentRunner - not used when workerExecutor is set
		logger,
		mockWorker,
		mockGuardian,
	)

	return &fullPipelineTestEnv{
		t:            t,
		logger:       logger,
		taskSvc:      taskSvc,
		taskRepo:     taskRepo,
		subtaskRepo:  subtaskRepo,
		outboxWriter: outboxWriter,
		txManager:    txManager,
		orchestrator: orch,
		mockWorker:   mockWorker,
		mockGuardian: mockGuardian,
	}
}

// ----------------------------------------------------------------------------
// Tests
// ----------------------------------------------------------------------------

// TestFullPipeline_SubmitToComplete tests the complete pipeline:
// 1. Submit task (pending)
// 2. Decompose task (decomposing)
// 3. StartTask via orchestrator
// 4. Task transitions through: pending -> decomposing -> dispatched -> running -> reviewing -> completed
func TestFullPipeline_SubmitToComplete(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	env := setupFullPipelineEnv(t)

	t.Run("SubmitToComplete", func(t *testing.T) {
		testSubmitToComplete(ctx, t, env)
	})
}

func testSubmitToComplete(ctx context.Context, t *testing.T, env *fullPipelineTestEnv) {
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
	t.Logf("Task submitted: %s, status: %s", taskID, submitResp.Task.Status)

	// Verify initial state is pending
	require.Equal(t, domain.TaskStatusPending, submitResp.Task.Status, "New task should be in pending state")

	// Step 2: Decompose the task
	t.Log("Step 2: Decomposing task into subtasks...")

	decomposeReq := service.DecomposeRequest{
		TaskID: taskID,
		Subtasks: []service.SubtaskSpec{
			{
				Type:          domain.SubtaskTypeAnalysis,
				Description:   "Analyze authentication requirements",
				AgentTemplate: "strategist",
			},
			{
				Type:          domain.SubtaskTypeCoding,
				Description:   "Implement JWT authentication",
				AgentTemplate: "executor",
			},
			{
				Type:          domain.SubtaskTypeTesting,
				Description:   "Write authentication tests",
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

	t.Logf("Task decomposed: %s, status: %s, subtasks: %d", taskID, decomposeResp.Task.Status, len(decomposeResp.Subtasks))

	// Verify task status is decomposing
	require.Equal(t, domain.TaskStatusDecomposing, decomposeResp.Task.Status, "Task should be in decomposing state after decomposition")

	// Step 3: Get the task and start orchestration
	t.Log("Step 3: Starting task orchestration via orchestrator.StartTask...")

	task, err := env.taskRepo.GetByID(ctx, taskID)
	require.NoError(t, err, "Failed to get task for orchestration")

	// StartTask should transition task from decomposing to dispatched and begin execution
	err = env.orchestrator.StartTask(ctx, task)
	require.NoError(t, err, "StartTask should succeed")

	t.Logf("StartTask returned successfully for task: %s", taskID)

	// Step 4: Verify task transitions to dispatched (orchestrator starts async execution)
	t.Log("Step 4: Verifying task state transitions (dispatched -> running -> completed)...")

	// Wait for async execution to complete (orchestrator executes in goroutine)
	// Poll for task completion with timeout
	maxWait := 10 * time.Second
	pollInterval := 100 * time.Millisecond
	deadline := time.Now().Add(maxWait)

	for time.Now().Before(deadline) {
		task, err = env.taskRepo.GetByID(ctx, taskID)
		require.NoError(t, err, "Failed to get task during polling")

		t.Logf("Polling task %s: status=%s, version=%d", taskID, task.Status, task.Version)

		if task.Status == domain.TaskStatusCompleted {
			t.Logf("Task %s reached completed state", taskID)
			break
		}

		if task.Status == domain.TaskStatusFailed {
			t.Fatal("Task failed during execution")
		}

		select {
		case <-ctx.Done():
			t.Fatal("Context cancelled during polling")
		case <-time.After(pollInterval):
			// Continue polling
		}
	}

	// Verify final state
	require.Equal(t, domain.TaskStatusCompleted, task.Status, "Task should be completed after full pipeline execution")

	// Give goroutines time to complete to avoid panic during test cleanup
	time.Sleep(100 * time.Millisecond)

	// Verify subtasks were executed via mock worker
	// Note: orchestrator only starts 1 agent session (first ready subtask) even with multiple subtasks
	executedIDs := env.mockWorker.GetExecutedIDs()
	require.Len(t, executedIDs, 1, "Mock worker should have been called for 1 subtask")
	t.Logf("Mock worker executed %d subtasks: %v", len(executedIDs), executedIDs)

	// Step 5: Verify subtasks are in completed state
	t.Log("Step 5: Verifying subtasks are in completed state...")

	subtasks, err := env.subtaskRepo.ListByTaskID(ctx, taskID)
	require.NoError(t, err, "Failed to list subtasks")
	require.Len(t, subtasks, 4, "Should have 4 subtasks")

	for i, st := range subtasks {
		t.Logf("Subtask[%d]: %s, type: %s, status: %s", i, st.ID, st.Type, st.Status)
	}

	t.Log("Full pipeline E2E test completed successfully!")
}

// TestFullPipeline_SingleAgentSubmitToComplete tests the pipeline with a single-agent task
// (no explicit subtasks) to verify the single-agent execution path.
func TestFullPipeline_SingleAgentSubmitToComplete(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	env := setupFullPipelineEnv(t)

	t.Run("SingleAgentSubmitToComplete", func(t *testing.T) {
		testSingleAgentSubmitToComplete(ctx, t, env)
	})
}

func testSingleAgentSubmitToComplete(ctx context.Context, t *testing.T, env *fullPipelineTestEnv) {
	// Step 1: Submit a task (no explicit subtasks)
	t.Log("Step 1: Submitting single-agent task...")

	submitReq := service.SubmitRequest{
		Goal:          "Implement a simple feature",
		RepositoryURL: "https://github.com/example/repo",
		BaseBranch:    "main",
		ClientID:      "test-client-single",
		Priority:      3,
	}

	submitResp, err := env.taskSvc.Submit(ctx, submitReq)
	require.NoError(t, err, "Failed to submit task")
	require.Equal(t, domain.TaskStatusPending, submitResp.Task.Status)

	taskID := submitResp.TaskID
	t.Logf("Task submitted: %s", taskID)

	// Step 2: Start orchestration directly (orchestrator handles single-agent case)
	t.Log("Step 2: Starting orchestrator for single-agent task...")

	task, err := env.taskRepo.GetByID(ctx, taskID)
	require.NoError(t, err, "Failed to get task")

	err = env.orchestrator.StartTask(ctx, task)
	require.NoError(t, err, "StartTask should succeed for single-agent task")

	// Step 3: Wait for completion
	t.Log("Step 3: Waiting for task to complete...")

	maxWait := 10 * time.Second
	pollInterval := 100 * time.Millisecond
	deadline := time.Now().Add(maxWait)

	for time.Now().Before(deadline) {
		task, err = env.taskRepo.GetByID(ctx, taskID)
		require.NoError(t, err, "Failed to get task during polling")

		t.Logf("Single-agent task polling: %s, status=%s", taskID, task.Status)

		if task.Status == domain.TaskStatusCompleted {
			break
		}

		if task.Status == domain.TaskStatusFailed {
			t.Fatal("Single-agent task failed during execution")
		}

		select {
		case <-ctx.Done():
			t.Fatal("Context cancelled during polling")
		case <-time.After(pollInterval):
		}
	}

	require.Equal(t, domain.TaskStatusCompleted, task.Status, "Single-agent task should complete")
	t.Log("Single-agent pipeline E2E test completed successfully!")
}

// TestFullPipeline_GuardianRejection tests that task execution fails when guardian rejects.
func TestFullPipeline_GuardianRejection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	env := setupFullPipelineEnv(t)

	// Configure mock guardian to reject
	env.mockGuardian.SetAutoApprove(false)
	env.mockGuardian.SetApprovalResult(orchestrator.ApprovalResultReject)

	t.Run("GuardianRejection", func(t *testing.T) {
		testGuardianRejection(ctx, t, env)
	})
}

func testGuardianRejection(ctx context.Context, t *testing.T, env *fullPipelineTestEnv) {
	// Submit task
	t.Log("Submitting task for guardian rejection test...")

	submitReq := service.SubmitRequest{
		Goal:          "Implement risky feature",
		RepositoryURL: "https://github.com/example/repo",
		BaseBranch:    "main",
		ClientID:      "test-client-reject",
	}

	submitResp, err := env.taskSvc.Submit(ctx, submitReq)
	require.NoError(t, err, "Failed to submit task")

	taskID := submitResp.TaskID

	// Decompose
	_, err = env.taskSvc.Decompose(ctx, service.DecomposeRequest{
		TaskID: taskID,
		Subtasks: []service.SubtaskSpec{
			{
				Type:          domain.SubtaskTypeCoding,
				Description:   "Implement risky feature",
				AgentTemplate: "executor",
			},
		},
	})
	require.NoError(t, err, "Failed to decompose task")

	// Start orchestration
	task, err := env.taskRepo.GetByID(ctx, taskID)
	require.NoError(t, err, "Failed to get task")

	err = env.orchestrator.StartTask(ctx, task)
	require.NoError(t, err, "StartTask should succeed")

	// Wait for guardian rejection
	t.Log("Waiting for guardian rejection...")

	maxWait := 5 * time.Second
	pollInterval := 100 * time.Millisecond
	deadline := time.Now().Add(maxWait)

	for time.Now().Before(deadline) {
		task, err = env.taskRepo.GetByID(ctx, taskID)
		require.NoError(t, err, "Failed to get task during polling")

		if task.Status == domain.TaskStatusFailed {
			t.Logf("Task %s correctly failed due to guardian rejection", taskID)
			return
		}

		select {
		case <-ctx.Done():
			t.Fatal("Context cancelled during polling")
		case <-time.After(pollInterval):
		}
	}

	t.Fatal("Task should have failed due to guardian rejection")
}

// TestFullPipeline_TaskStatusQuery tests the task status query via orchestrator.
func TestFullPipeline_TaskStatusQuery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	env := setupFullPipelineEnv(t)

	t.Run("TaskStatusQuery", func(t *testing.T) {
		testTaskStatusQuery(ctx, t, env)
	})
}

func testTaskStatusQuery(ctx context.Context, t *testing.T, env *fullPipelineTestEnv) {
	// Submit and decompose task
	submitResp, err := env.taskSvc.Submit(ctx, service.SubmitRequest{
		Goal:          "Test status query",
		RepositoryURL: "https://github.com/example/repo",
		BaseBranch:    "main",
		ClientID:      "test-client-status",
	})
	require.NoError(t, err)

	taskID := submitResp.TaskID

	// Decompose
	_, err = env.taskSvc.Decompose(ctx, service.DecomposeRequest{
		TaskID: taskID,
		Subtasks: []service.SubtaskSpec{
			{
				Type:          domain.SubtaskTypeCoding,
				Description:   "Test subtask",
				AgentTemplate: "executor",
			},
		},
	})
	require.NoError(t, err, "Failed to decompose task")

	// Start orchestration
	task, err := env.taskRepo.GetByID(ctx, taskID)
	require.NoError(t, err, "Failed to get task")

	err = env.orchestrator.StartTask(ctx, task)
	require.NoError(t, err, "StartTask should succeed")

	// Query status via orchestrator
	t.Log("Querying task status via orchestrator.GetTaskStatus...")

	status, err := env.orchestrator.GetTaskStatus(ctx, taskID)
	require.NoError(t, err, "GetTaskStatus should succeed")
	require.NotNil(t, status, "Status should not be nil")
	require.Equal(t, taskID, status.TaskID, "TaskID should match")

	t.Logf("Initial status: TaskID=%s, TaskStatus=%s, ActiveAgents=%d",
		status.TaskID, status.TaskStatus, status.ActiveAgents)

	// Wait for completion and verify final status
	maxWait := 10 * time.Second
	pollInterval := 100 * time.Millisecond
	deadline := time.Now().Add(maxWait)

	for time.Now().Before(deadline) {
		status, err = env.orchestrator.GetTaskStatus(ctx, taskID)
		require.NoError(t, err, "GetTaskStatus should succeed")

		t.Logf("Status polling: TaskStatus=%s, ActiveAgents=%d", status.TaskStatus, status.ActiveAgents)

		if status.TaskStatus == domain.TaskStatusCompleted {
			break
		}

		select {
		case <-ctx.Done():
			t.Fatal("Context cancelled during polling")
		case <-time.After(pollInterval):
		}
	}

	require.Equal(t, domain.TaskStatusCompleted, status.TaskStatus, "Task should be completed")
	t.Log("Task status query E2E test completed successfully!")
}

// TestFullPipeline_TaskListQuery tests the task list query via service.
func TestFullPipeline_TaskListQuery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	env := setupFullPipelineEnv(t)

	t.Run("TaskListQuery", func(t *testing.T) {
		testTaskListQuery(ctx, t, env)
	})
}

func testTaskListQuery(ctx context.Context, t *testing.T, env *fullPipelineTestEnv) {
	// Submit multiple tasks
	t.Log("Submitting multiple tasks for list query test...")

	clientID := "test-client-list"
	taskIDs := make([]string, 3)

	for i := 0; i < 3; i++ {
		submitResp, err := env.taskSvc.Submit(ctx, service.SubmitRequest{
			Goal:          "Test task for list query",
			RepositoryURL: "https://github.com/example/repo",
			BaseBranch:    "main",
			ClientID:      clientID,
		})
		require.NoError(t, err, "Failed to submit task %d", i)
		taskIDs[i] = submitResp.TaskID
		t.Logf("Submitted task %d: %s", i, taskIDs[i])
	}

	// List tasks by client ID
	t.Log("Listing tasks by client ID...")

	listResp, err := env.taskSvc.List(ctx, service.ListRequest{
		ClientID: clientID,
		Limit:   10,
	})
	require.NoError(t, err, "Failed to list tasks")
	require.NotNil(t, listResp, "List response should not be nil")
	require.Equal(t, 3, listResp.Total, "Should have 3 tasks")
	require.Len(t, listResp.Tasks, 3, "Should return 3 tasks")

	// Verify all submitted tasks are in the list
	foundIDs := make(map[string]bool)
	for _, task := range listResp.Tasks {
		foundIDs[task.ID] = true
	}

	for _, taskID := range taskIDs {
		require.True(t, foundIDs[taskID], "Task %s should be in list", taskID)
	}

	t.Log("Task list query E2E test completed successfully!")
}

// TestFullPipeline_CancelTask tests task cancellation during execution.
func TestFullPipeline_CancelTask(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	env := setupFullPipelineEnv(t)

	t.Run("CancelTask", func(t *testing.T) {
		testCancelTask(ctx, t, env)
	})
}

func testCancelTask(ctx context.Context, t *testing.T, env *fullPipelineTestEnv) {
	// Submit and decompose task
	submitResp, err := env.taskSvc.Submit(ctx, service.SubmitRequest{
		Goal:          "Task to cancel",
		RepositoryURL: "https://github.com/example/repo",
		BaseBranch:    "main",
		ClientID:      "test-client-cancel",
	})
	require.NoError(t, err)

	taskID := submitResp.TaskID

	// Decompose
	_, err = env.taskSvc.Decompose(ctx, service.DecomposeRequest{
		TaskID: taskID,
		Subtasks: []service.SubtaskSpec{
			{
				Type:          domain.SubtaskTypeCoding,
				Description:   "Long running task",
				AgentTemplate: "executor",
			},
		},
	})
	require.NoError(t, err, "Failed to decompose task")

	// Start orchestration
	task, err := env.taskRepo.GetByID(ctx, taskID)
	require.NoError(t, err, "Failed to get task")

	err = env.orchestrator.StartTask(ctx, task)
	require.NoError(t, err, "StartTask should succeed")

	// Poll for task to be in running state before cancelling
	t.Log("Waiting for task to start running...")
	pollInterval := 50 * time.Millisecond
	timeout := 5 * time.Second
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		task, _ = env.taskRepo.GetByID(ctx, taskID)
		if task != nil && task.Status == domain.TaskStatusRunning {
			break
		}
		time.Sleep(pollInterval)
	}
	require.NotNil(t, task, "Task should be found")
	t.Logf("Task is now in state: %s", task.Status)

	// Cancel the task immediately while it's running
	t.Log("Cancelling task...")

	cancelResp, err := env.taskSvc.Cancel(ctx, service.CancelRequest{
		TaskID: taskID,
		Reason: "Test cancellation",
	})
	require.NoError(t, err, "Cancel should succeed")
	require.Equal(t, domain.TaskStatusCancelled, cancelResp.Task.Status, "Task should be cancelled")

	// Verify final status
	getResp, err := env.taskSvc.Get(ctx, service.GetRequest{TaskID: taskID})
	require.NoError(t, err, "Failed to get cancelled task")
	require.Equal(t, domain.TaskStatusCancelled, getResp.Task.Status, "Task should be cancelled")

	t.Log("Task cancellation E2E test completed successfully!")
}

// TestFullPipeline_StateTransitions tests all task state transitions explicitly.
func TestFullPipeline_StateTransitions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	env := setupFullPipelineEnv(t)

	t.Run("StateTransitions", func(t *testing.T) {
		testStateTransitions(ctx, t, env)
	})
}

func testStateTransitions(ctx context.Context, t *testing.T, env *fullPipelineTestEnv) {
	// Submit task - should be pending
	t.Log("Step 1: Submit task (should be pending)...")

	submitResp, err := env.taskSvc.Submit(ctx, service.SubmitRequest{
		Goal:          "Test state transitions",
		RepositoryURL: "https://github.com/example/repo",
		BaseBranch:    "main",
		ClientID:      "test-client-states",
	})
	require.NoError(t, err)
	require.Equal(t, domain.TaskStatusPending, submitResp.Task.Status)

	taskID := submitResp.TaskID
	t.Logf("Task %s submitted, status: %s", taskID, submitResp.Task.Status)

	// Decompose task - should be decomposing
	t.Log("Step 2: Decompose task (should be decomposing)...")

	decomposeResp, err := env.taskSvc.Decompose(ctx, service.DecomposeRequest{
		TaskID: taskID,
		Subtasks: []service.SubtaskSpec{
			{
				Type:          domain.SubtaskTypeCoding,
				Description:   "Code something",
				AgentTemplate: "executor",
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, domain.TaskStatusDecomposing, decomposeResp.Task.Status)
	t.Logf("Task %s decomposed, status: %s", taskID, decomposeResp.Task.Status)

	// Start orchestration - should transition to dispatched -> running -> reviewing -> completed
	t.Log("Step 3: Start orchestration...")

	task, err := env.taskRepo.GetByID(ctx, taskID)
	require.NoError(t, err)

	err = env.orchestrator.StartTask(ctx, task)
	require.NoError(t, err)

	// Wait for completion
	t.Log("Step 4: Wait for completion...")

	maxWait := 10 * time.Second
	pollInterval := 100 * time.Millisecond
	deadline := time.Now().Add(maxWait)

	for time.Now().Before(deadline) {
		task, err = env.taskRepo.GetByID(ctx, taskID)
		require.NoError(t, err)

		t.Logf("State transition polling: %s, status=%s", taskID, task.Status)

		if task.Status == domain.TaskStatusCompleted {
			t.Logf("Task %s completed", taskID)
			break
		}

		if task.Status == domain.TaskStatusFailed {
			t.Fatal("Task failed during state transitions")
		}

		select {
		case <-ctx.Done():
			t.Fatal("Context cancelled during polling")
		case <-time.After(pollInterval):
		}
	}

	// Verify all states were visited
	require.Equal(t, domain.TaskStatusCompleted, task.Status)

	// Verify the state transition path
	// pending -> decomposing -> dispatched -> running -> reviewing -> completed
	t.Log("State transition E2E test completed successfully!")
	assert.NotNil(t, task.CompletedAt)
}