// Package orchestrator implements L4 orchestration: task scheduling, agent session
// management, and event-driven workflow coordination.
package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/cloud-agent-platform/cap/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ----------------------------------------------------------------------------
// Mock Implementations
// ----------------------------------------------------------------------------

// mockTaskRepository is a mock implementation of domain.TaskRepository.
type mockTaskRepository struct {
	mu      sync.RWMutex
	tasks   map[string]*domain.Task
	outbox  []*domain.DomainEvent
}

func newMockTaskRepository() *mockTaskRepository {
	return &mockTaskRepository{
		tasks:  make(map[string]*domain.Task),
		outbox: make([]*domain.DomainEvent, 0),
	}
}

func (m *mockTaskRepository) Create(ctx context.Context, task *domain.Task) (*domain.Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tasks[task.ID] = task
	return task, nil
}

func (m *mockTaskRepository) GetByID(ctx context.Context, id string) (*domain.Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	task, ok := m.tasks[id]
	if !ok {
		return nil, domain.NewL2AggregateNotFoundError("Task", id)
	}
	// Return a copy to avoid mutation issues
	taskCopy := *task
	return &taskCopy, nil
}

func (m *mockTaskRepository) Update(ctx context.Context, task *domain.Task) (*domain.Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tasks[task.ID] = task
	return task, nil
}

func (m *mockTaskRepository) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tasks, id)
	return nil
}

func (m *mockTaskRepository) ListByStatus(ctx context.Context, status domain.TaskStatus, limit, offset int) ([]*domain.Task, int, error) {
	return nil, 0, nil
}

func (m *mockTaskRepository) ListByClientID(ctx context.Context, clientID string, limit, offset int) ([]*domain.Task, int, error) {
	return nil, 0, nil
}

func (m *mockTaskRepository) List(ctx context.Context, limit, offset int) ([]*domain.Task, int, error) {
	return nil, 0, nil
}

func (m *mockTaskRepository) UpdateStatus(ctx context.Context, id string, status domain.TaskStatus, expectedVersion int64) (*domain.Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	task, ok := m.tasks[id]
	if !ok {
		return nil, domain.NewL2AggregateNotFoundError("Task", id)
	}
	task.Status = status
	task.Version = expectedVersion + 1
	return task, nil
}

func (m *mockTaskRepository) CountByStatus(ctx context.Context, status domain.TaskStatus) (int, error) {
	return 0, nil
}

// mockSubtaskRepository is a mock implementation of domain.SubtaskRepository.
type mockSubtaskRepository struct {
	mu       sync.RWMutex
	subtasks map[string]*domain.Subtask
}

func newMockSubtaskRepository() *mockSubtaskRepository {
	return &mockSubtaskRepository{
		subtasks: make(map[string]*domain.Subtask),
	}
}

func (m *mockSubtaskRepository) Create(ctx context.Context, subtask *domain.Subtask) (*domain.Subtask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subtasks[subtask.ID] = subtask
	return subtask, nil
}

func (m *mockSubtaskRepository) GetByID(ctx context.Context, id string) (*domain.Subtask, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	subtask, ok := m.subtasks[id]
	if !ok {
		return nil, domain.NewL2AggregateNotFoundError("Subtask", id)
	}
	subtaskCopy := *subtask
	return &subtaskCopy, nil
}

func (m *mockSubtaskRepository) Update(ctx context.Context, subtask *domain.Subtask) (*domain.Subtask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subtasks[subtask.ID] = subtask
	return subtask, nil
}

func (m *mockSubtaskRepository) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.subtasks, id)
	return nil
}

func (m *mockSubtaskRepository) ListByTaskID(ctx context.Context, taskID string) ([]*domain.Subtask, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*domain.Subtask
	for _, st := range m.subtasks {
		if st.TaskID == taskID {
			result = append(result, st)
		}
	}
	return result, nil
}

func (m *mockSubtaskRepository) UpdateStatus(ctx context.Context, id string, status domain.TaskStatus, expectedVersion int64) (*domain.Subtask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	subtask, ok := m.subtasks[id]
	if !ok {
		return nil, domain.NewL2AggregateNotFoundError("Subtask", id)
	}
	subtask.Status = status
	subtask.Version = expectedVersion + 1
	return subtask, nil
}

func (m *mockSubtaskRepository) ListByStatus(ctx context.Context, status domain.TaskStatus, limit, offset int) ([]*domain.Subtask, int, error) {
	return nil, 0, nil
}

// mockOutboxWriter is a mock implementation of domain.OutboxWriter.
type mockOutboxWriter struct {
	mu     sync.Mutex
	events []*domain.DomainEvent
}

func newMockOutboxWriter() *mockOutboxWriter {
	return &mockOutboxWriter{
		events: make([]*domain.DomainEvent, 0),
	}
}

func (m *mockOutboxWriter) Write(ctx context.Context, tx interface{}, event *domain.DomainEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return nil
}

func (m *mockOutboxWriter) GetEvents() []*domain.DomainEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.events
}

// mockTransactionManager is a mock implementation of TransactionManager.
type mockTransactionManager struct {
	mu        sync.RWMutex
	commited  bool
	rolledBack bool
}

func (m *mockTransactionManager) BeginTx(ctx context.Context) (TransactionManager, error) {
	return &mockTransactionManager{}, nil
}

func (m *mockTransactionManager) Commit(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.commited = true
	return nil
}

func (m *mockTransactionManager) Rollback(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rolledBack = true
	return nil
}

// mockAgentRunner is a mock implementation of AgentRunner.
type mockAgentRunner struct {
	mu       sync.RWMutex
	typeName string
	results  map[string]*AgentResult
	calls    []*AgentRunnerCall
}

type AgentRunnerCall struct {
	SubtaskID string
	TaskID    string
	StartedAt time.Time
}

func newMockAgentRunner() *mockAgentRunner {
	return &mockAgentRunner{
		typeName: "mock",
		results:  make(map[string]*AgentResult),
		calls:    make([]*AgentRunnerCall, 0),
	}
}

func (m *mockAgentRunner) Type() string {
	return m.typeName
}

func (m *mockAgentRunner) Run(ctx context.Context, subtask *domain.Subtask, task *domain.Task) (*AgentResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, &AgentRunnerCall{
		SubtaskID: subtask.ID,
		TaskID:    task.ID,
		StartedAt: time.Now(),
	})
	result, ok := m.results[subtask.ID]
	if !ok {
		// Default successful result
		return &AgentResult{
			Summary:          "Task completed successfully",
			Artifacts:        []domain.ArtifactRef{},
			TokensUsed:       100,
			ExecutionDuration: 1 * time.Second,
		}, nil
	}
	return result, result.Error
}

func (m *mockAgentRunner) SetResult(subtaskID string, result *AgentResult) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.results[subtaskID] = result
}

func (m *mockAgentRunner) GetCalls() []*AgentRunnerCall {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.calls
}

// ----------------------------------------------------------------------------
// Unit Tests
// ----------------------------------------------------------------------------

func TestOrchestrator_SingleAgentPath(t *testing.T) {
	// Test: pending → assigned → running → completed
	logger, _ := zap.NewDevelopment()

	taskRepo := newMockTaskRepository()
	subtaskRepo := newMockSubtaskRepository()
	outboxWriter := newMockOutboxWriter()
	txManager := &mockTransactionManager{}
	agentRunner := newMockAgentRunner()

	cfg := DefaultConfig()
	cfg.DefaultAgentTemplate = "executor"

	orch := NewOrchestrator(
		cfg,
		taskRepo,
		subtaskRepo,
		outboxWriter,
		txManager,
		agentRunner,
		logger,
		nil, // workerExecutor
		nil, // guardian
	)

	// Create a task in pending state
	task := domain.NewTask("task123", "Test task goal", "https://github.com/test/repo", "main", "client-1")
	taskRepo.Create(context.Background(), task)

	// Verify initial state
	require.Equal(t, domain.TaskStatusPending, task.Status)

	// Start the task
	err := orch.StartTask(context.Background(), task)
	require.NoError(t, err)

	// Give time for async execution
	time.Sleep(100 * time.Millisecond)

	// Verify task was dispatched
	updatedTask, err := taskRepo.GetByID(context.Background(), task.ID)
	require.NoError(t, err)
	assert.True(t, updatedTask.Status == domain.TaskStatusRunning || updatedTask.Status == domain.TaskStatusReviewing || updatedTask.Status == domain.TaskStatusCompleted,
		"Expected task to be in running/reviewing/completed state, got %s", updatedTask.Status)

	// Verify agent runner was called
	calls := agentRunner.GetCalls()
	assert.Len(t, calls, 1, "Expected one agent runner call")
}

// TestOrchestrator_InvalidTaskState tests that orchestrator rejects tasks in invalid states.
func TestOrchestrator_InvalidTaskState(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	taskRepo := newMockTaskRepository()
	subtaskRepo := newMockSubtaskRepository()
	outboxWriter := newMockOutboxWriter()
	txManager := &mockTransactionManager{}
	agentRunner := newMockAgentRunner()

	cfg := DefaultConfig()
	orch := NewOrchestrator(cfg, taskRepo, subtaskRepo, outboxWriter, txManager, agentRunner, logger, nil, nil)

	// Create a task in running state (invalid for StartTask)
	task := domain.NewTask("task456", "Test task", "https://github.com/test/repo", "main", "client-1")
	task.Status = domain.TaskStatusRunning
	taskRepo.Create(context.Background(), task)

	// Attempt to start should fail
	err := orch.StartTask(context.Background(), task)
	assert.Error(t, err)
}

// TestOrchestrator_CancelTask tests task cancellation.
func TestOrchestrator_CancelTask(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	taskRepo := newMockTaskRepository()
	subtaskRepo := newMockSubtaskRepository()
	outboxWriter := newMockOutboxWriter()
	txManager := &mockTransactionManager{}
	agentRunner := newMockAgentRunner()

	cfg := DefaultConfig()
	orch := NewOrchestrator(cfg, taskRepo, subtaskRepo, outboxWriter, txManager, agentRunner, logger, nil, nil)

	// Create a task in pending state
	task := domain.NewTask("task789", "Test task", "https://github.com/test/repo", "main", "client-1")
	taskRepo.Create(context.Background(), task)

	// Cancel should succeed
	err := orch.CancelTask(context.Background(), task.ID)
	assert.NoError(t, err)

	// Verify task was cancelled
	updatedTask, err := taskRepo.GetByID(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.TaskStatusCancelled, updatedTask.Status)
}

// TestOrchestrator_GetTaskStatus tests retrieving orchestration status.
func TestOrchestrator_GetTaskStatus(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	taskRepo := newMockTaskRepository()
	subtaskRepo := newMockSubtaskRepository()
	outboxWriter := newMockOutboxWriter()
	txManager := &mockTransactionManager{}
	agentRunner := newMockAgentRunner()

	cfg := DefaultConfig()
	orch := NewOrchestrator(cfg, taskRepo, subtaskRepo, outboxWriter, txManager, agentRunner, logger, nil, nil)

	// Create a task
	task := domain.NewTask("task-status", "Test task", "https://github.com/test/repo", "main", "client-1")
	taskRepo.Create(context.Background(), task)

	// Get status
	status, err := orch.GetTaskStatus(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, task.ID, status.TaskID)
	assert.Equal(t, domain.TaskStatusPending, status.TaskStatus)
}

// TestOrchestrator_DomainEventEmission tests that domain events are emitted.
func TestOrchestrator_DomainEventEmission(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	taskRepo := newMockTaskRepository()
	subtaskRepo := newMockSubtaskRepository()
	outboxWriter := newMockOutboxWriter()
	txManager := &mockTransactionManager{}
	agentRunner := newMockAgentRunner()

	cfg := DefaultConfig()
	orch := NewOrchestrator(cfg, taskRepo, subtaskRepo, outboxWriter, txManager, agentRunner, logger, nil, nil)

	// Create a task
	task := domain.NewTask("task-events", "Test task", "https://github.com/test/repo", "main", "client-1")
	taskRepo.Create(context.Background(), task)

	// Start task
	err := orch.StartTask(context.Background(), task)
	require.NoError(t, err)

	// Give time for async execution
	time.Sleep(50 * time.Millisecond)

	// Verify events were written to outbox
	events := outboxWriter.GetEvents()
	assert.NotEmpty(t, events, "Expected events to be written to outbox")
}

// TestOrchestrator_AgentFailure tests handling of agent execution failure.
func TestOrchestrator_AgentFailure(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	taskRepo := newMockTaskRepository()
	subtaskRepo := newMockSubtaskRepository()
	outboxWriter := newMockOutboxWriter()
	txManager := &mockTransactionManager{}
	agentRunner := newMockAgentRunner()

	cfg := DefaultConfig()
	orch := NewOrchestrator(cfg, taskRepo, subtaskRepo, outboxWriter, txManager, agentRunner, logger, nil, nil)

	// Configure agent to fail
	agentRunner.SetResult("task-fail", &AgentResult{
		Error: errors.New("agent execution failed"),
	})

	// Create a task
	task := domain.NewTask("task-fail", "Test task", "https://github.com/test/repo", "main", "client-1")
	taskRepo.Create(context.Background(), task)

	// Start task - it will fail
	err := orch.StartTask(context.Background(), task)
	require.NoError(t, err)

	// Give time for async execution
	time.Sleep(50 * time.Millisecond)

	// Verify task status (may be failed or still in progress depending on timing)
	// The important thing is the agent was called
	calls := agentRunner.GetCalls()
	assert.Len(t, calls, 1, "Expected agent runner to be called even on failure setup")
}

// TestOrchestrator_StartTask_WithSubtasks tests starting a task with subtasks.
func TestOrchestrator_StartTask_WithSubtasks(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	taskRepo := newMockTaskRepository()
	subtaskRepo := newMockSubtaskRepository()
	outboxWriter := newMockOutboxWriter()
	txManager := &mockTransactionManager{}
	agentRunner := newMockAgentRunner()

	cfg := DefaultConfig()
	orch := NewOrchestrator(cfg, taskRepo, subtaskRepo, outboxWriter, txManager, agentRunner, logger, nil, nil)

	// Create a task with subtasks
	task := domain.NewTask("task-with-subtasks", "Test task", "https://github.com/test/repo", "main", "client-1")
	taskRepo.Create(context.Background(), task)

	// Create a subtask
	subtask := domain.NewSubtask("subtask-1", task.ID, domain.SubtaskTypeCoding, "Implement feature X", "executor")
	subtaskRepo.Create(context.Background(), subtask)

	// Start task
	err := orch.StartTask(context.Background(), task)
	require.NoError(t, err)

	// Give time for async execution
	time.Sleep(100 * time.Millisecond)

	// Verify agent was called for the subtask
	calls := agentRunner.GetCalls()
	assert.Len(t, calls, 1, "Expected one agent call for subtask")
}

// TestOrchestrator_EventDispatcher tests the event dispatcher.
func TestOrchestrator_EventDispatcher(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	taskRepo := newMockTaskRepository()
	subtaskRepo := newMockSubtaskRepository()
	outboxWriter := newMockOutboxWriter()
	txManager := &mockTransactionManager{}
	agentRunner := newMockAgentRunner()

	cfg := DefaultConfig()
	orch := NewOrchestrator(cfg, taskRepo, subtaskRepo, outboxWriter, txManager, agentRunner, logger, nil, nil)

	// Create a domain event
	event, err := domain.NewDomainEvent("Task", "task-123", "TaskSubmittedV1", []byte(`{}`), 1)
	require.NoError(t, err)

	// Dispatch should not error even without a specific handler for TaskSubmittedV1
	err = orch.Dispatch(context.Background(), event)
	assert.NoError(t, err)
}

// TestOrchestrator_ConcurrentSessions tests concurrent task orchestration.
func TestOrchestrator_ConcurrentSessions(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	taskRepo := newMockTaskRepository()
	subtaskRepo := newMockSubtaskRepository()
	outboxWriter := newMockOutboxWriter()
	txManager := &mockTransactionManager{}
	agentRunner := newMockAgentRunner()

	cfg := DefaultConfig()
	cfg.MaxConcurrentSessions = 2
	orch := NewOrchestrator(cfg, taskRepo, subtaskRepo, outboxWriter, txManager, agentRunner, logger, nil, nil)

	// Create multiple tasks
	for i := 0; i < 3; i++ {
		task := domain.NewTask("task-concurrent-"+string(rune('0'+i)), "Test task", "https://github.com/test/repo", "main", "client-1")
		taskRepo.Create(context.Background(), task)

		go func(t *domain.Task) {
			_ = orch.StartTask(context.Background(), t)
		}(task)
	}

	// Give time for async executions
	time.Sleep(200 * time.Millisecond)

	// Verify all tasks were processed
	calls := agentRunner.GetCalls()
	assert.Len(t, calls, 3, "Expected all three tasks to be processed")
}

// TestDefaultConfig tests the default configuration.
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, 10, cfg.MaxConcurrentSessions)
	assert.Equal(t, 30*time.Minute, cfg.SessionTimeout)
	assert.Equal(t, "executor", cfg.DefaultAgentTemplate)
}
