package service

import (
	"context"
	"errors"
	"testing"

	"github.com/cloud-agent-platform/cap/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// mockTaskRepository is a mock implementation of domain.TaskRepository.
type mockTaskRepository struct {
	createFunc          func(ctx context.Context, task *domain.Task) (*domain.Task, error)
	getByIDFunc         func(ctx context.Context, id string) (*domain.Task, error)
	updateFunc          func(ctx context.Context, task *domain.Task) (*domain.Task, error)
	updateStatusFunc    func(ctx context.Context, id string, status domain.TaskStatus, expectedVersion int64) (*domain.Task, error)
	listByStatusFunc    func(ctx context.Context, status domain.TaskStatus, limit, offset int) ([]*domain.Task, int, error)
	listByClientIDFunc  func(ctx context.Context, clientID string, limit, offset int) ([]*domain.Task, int, error)
	listFunc            func(ctx context.Context, limit, offset int) ([]*domain.Task, int, error)
	countByStatusFunc   func(ctx context.Context, status domain.TaskStatus) (int, error)
	deleteFunc          func(ctx context.Context, id string) error
}

func (m *mockTaskRepository) Create(ctx context.Context, task *domain.Task) (*domain.Task, error) {
	if m.createFunc != nil {
		return m.createFunc(ctx, task)
	}
	return task, nil
}

func (m *mockTaskRepository) GetByID(ctx context.Context, id string) (*domain.Task, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, id)
	}
	return nil, domain.NewL2AggregateNotFoundError("Task", id)
}

func (m *mockTaskRepository) Update(ctx context.Context, task *domain.Task) (*domain.Task, error) {
	if m.updateFunc != nil {
		return m.updateFunc(ctx, task)
	}
	return task, nil
}

func (m *mockTaskRepository) Delete(ctx context.Context, id string) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, id)
	}
	return nil
}

func (m *mockTaskRepository) UpdateStatus(ctx context.Context, id string, status domain.TaskStatus, expectedVersion int64) (*domain.Task, error) {
	if m.updateStatusFunc != nil {
		return m.updateStatusFunc(ctx, id, status, expectedVersion)
	}
	return nil, nil
}

func (m *mockTaskRepository) ListByStatus(ctx context.Context, status domain.TaskStatus, limit, offset int) ([]*domain.Task, int, error) {
	if m.listByStatusFunc != nil {
		return m.listByStatusFunc(ctx, status, limit, offset)
	}
	return nil, 0, nil
}

func (m *mockTaskRepository) ListByClientID(ctx context.Context, clientID string, limit, offset int) ([]*domain.Task, int, error) {
	if m.listByClientIDFunc != nil {
		return m.listByClientIDFunc(ctx, clientID, limit, offset)
	}
	return nil, 0, nil
}

func (m *mockTaskRepository) List(ctx context.Context, limit, offset int) ([]*domain.Task, int, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, limit, offset)
	}
	return nil, 0, nil
}

func (m *mockTaskRepository) CountByStatus(ctx context.Context, status domain.TaskStatus) (int, error) {
	if m.countByStatusFunc != nil {
		return m.countByStatusFunc(ctx, status)
	}
	return 0, nil
}

// mockOutboxWriter is a mock implementation of domain.OutboxWriter.
type mockOutboxWriter struct {
	writeFunc func(ctx context.Context, tx interface{}, event *domain.DomainEvent) error
}

func (m *mockOutboxWriter) Write(ctx context.Context, tx interface{}, event *domain.DomainEvent) error {
	if m.writeFunc != nil {
		return m.writeFunc(ctx, tx, event)
	}
	return nil
}

// mockTransactionManager is a mock implementation of TransactionManager.
type mockTransactionManager struct {
	commitFunc   func(ctx context.Context) error
	rollbackFunc func(ctx context.Context) error
	beginTxFunc  func(ctx context.Context) (TransactionManager, error)
}

func (m *mockTransactionManager) BeginTx(ctx context.Context) (TransactionManager, error) {
	if m.beginTxFunc != nil {
		return m.beginTxFunc(ctx)
	}
	return m, nil
}

func (m *mockTransactionManager) Commit(ctx context.Context) error {
	if m.commitFunc != nil {
		return m.commitFunc(ctx)
	}
	return nil
}

func (m *mockTransactionManager) Rollback(ctx context.Context) error {
	if m.rollbackFunc != nil {
		return m.rollbackFunc(ctx)
	}
	return nil
}

// mockSubtaskRepository is a mock implementation of domain.SubtaskRepository.
type mockSubtaskRepository struct {
	createFunc       func(ctx context.Context, subtask *domain.Subtask) (*domain.Subtask, error)
	getByIDFunc     func(ctx context.Context, id string) (*domain.Subtask, error)
	updateFunc      func(ctx context.Context, subtask *domain.Subtask) (*domain.Subtask, error)
	deleteFunc      func(ctx context.Context, id string) error
	listByTaskIDFunc func(ctx context.Context, taskID string) ([]*domain.Subtask, error)
	updateStatusFunc func(ctx context.Context, id string, status domain.TaskStatus, expectedVersion int64) (*domain.Subtask, error)
	listByStatusFunc func(ctx context.Context, status domain.TaskStatus, limit, offset int) ([]*domain.Subtask, int, error)
}

func (m *mockSubtaskRepository) Create(ctx context.Context, subtask *domain.Subtask) (*domain.Subtask, error) {
	if m.createFunc != nil {
		return m.createFunc(ctx, subtask)
	}
	return subtask, nil
}

func (m *mockSubtaskRepository) GetByID(ctx context.Context, id string) (*domain.Subtask, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, id)
	}
	return nil, domain.NewL2AggregateNotFoundError("Subtask", id)
}

func (m *mockSubtaskRepository) Update(ctx context.Context, subtask *domain.Subtask) (*domain.Subtask, error) {
	if m.updateFunc != nil {
		return m.updateFunc(ctx, subtask)
	}
	return subtask, nil
}

func (m *mockSubtaskRepository) Delete(ctx context.Context, id string) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, id)
	}
	return nil
}

func (m *mockSubtaskRepository) ListByTaskID(ctx context.Context, taskID string) ([]*domain.Subtask, error) {
	if m.listByTaskIDFunc != nil {
		return m.listByTaskIDFunc(ctx, taskID)
	}
	return nil, nil
}

func (m *mockSubtaskRepository) UpdateStatus(ctx context.Context, id string, status domain.TaskStatus, expectedVersion int64) (*domain.Subtask, error) {
	if m.updateStatusFunc != nil {
		return m.updateStatusFunc(ctx, id, status, expectedVersion)
	}
	return nil, nil
}

func (m *mockSubtaskRepository) ListByStatus(ctx context.Context, status domain.TaskStatus, limit, offset int) ([]*domain.Subtask, int, error) {
	if m.listByStatusFunc != nil {
		return m.listByStatusFunc(ctx, status, limit, offset)
	}
	return nil, 0, nil
}

func newTestTaskService(repo *mockTaskRepository, subtaskRepo *mockSubtaskRepository, outbox *mockOutboxWriter, tx *mockTransactionManager) *TaskService {
	logger, _ := zap.NewDevelopment()
	return NewTaskService(TaskServiceInput{
		TaskRepo:     repo,
		SubtaskRepo:  subtaskRepo,
		OutboxWriter: outbox,
		Storage:      tx,
		Logger:       logger,
	})
}

func TestTaskService_Submit(t *testing.T) {
	t.Run("should submit task successfully", func(t *testing.T) {
		repo := &mockTaskRepository{
			createFunc: func(ctx context.Context, task *domain.Task) (*domain.Task, error) {
				return task, nil
			},
		}
		outbox := &mockOutboxWriter{
			writeFunc: func(ctx context.Context, tx interface{}, event *domain.DomainEvent) error {
				return nil
			},
		}
		tx := &mockTransactionManager{
			commitFunc: func(ctx context.Context) error { return nil },
		}

		svc := newTestTaskService(repo, &mockSubtaskRepository{}, outbox, tx)

		req := SubmitRequest{
			Goal:          "Test task",
			RepositoryURL: "https://github.com/test/repo",
			BaseBranch:    "main",
			ClientID:      "client-123",
		}

		resp, err := svc.Submit(context.Background(), req)

		require.NoError(t, err)
		assert.NotEmpty(t, resp.TaskID)
		assert.Equal(t, "Test task", resp.Task.Goal)
		assert.Equal(t, "https://github.com/test/repo", resp.Task.RepositoryURL)
		assert.Equal(t, "main", resp.Task.BaseBranch)
		assert.Equal(t, "client-123", resp.Task.ClientID)
		assert.Equal(t, domain.TaskStatusPending, resp.Task.Status)
	})

	t.Run("should return error when goal is empty", func(t *testing.T) {
		svc := newTestTaskService(&mockTaskRepository{}, &mockSubtaskRepository{}, &mockOutboxWriter{}, &mockTransactionManager{})

		req := SubmitRequest{
			Goal:          "",
			RepositoryURL: "https://github.com/test/repo",
			BaseBranch:    "main",
			ClientID:      "client-123",
		}

		resp, err := svc.Submit(context.Background(), req)

		assert.Nil(t, resp)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "goal")
	})

	t.Run("should return error when repository_url is empty", func(t *testing.T) {
		svc := newTestTaskService(&mockTaskRepository{}, &mockSubtaskRepository{}, &mockOutboxWriter{}, &mockTransactionManager{})

		req := SubmitRequest{
			Goal:          "Test task",
			RepositoryURL: "",
			BaseBranch:    "main",
			ClientID:      "client-123",
		}

		resp, err := svc.Submit(context.Background(), req)

		assert.Nil(t, resp)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "repository_url")
	})

	t.Run("should return error when base_branch is empty", func(t *testing.T) {
		svc := newTestTaskService(&mockTaskRepository{}, &mockSubtaskRepository{}, &mockOutboxWriter{}, &mockTransactionManager{})

		req := SubmitRequest{
			Goal:          "Test task",
			RepositoryURL: "https://github.com/test/repo",
			BaseBranch:    "",
			ClientID:      "client-123",
		}

		resp, err := svc.Submit(context.Background(), req)

		assert.Nil(t, resp)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "base_branch")
	})

	t.Run("should return error when client_id is empty", func(t *testing.T) {
		svc := newTestTaskService(&mockTaskRepository{}, &mockSubtaskRepository{}, &mockOutboxWriter{}, &mockTransactionManager{})

		req := SubmitRequest{
			Goal:          "Test task",
			RepositoryURL: "https://github.com/test/repo",
			BaseBranch:    "main",
			ClientID:      "",
		}

		resp, err := svc.Submit(context.Background(), req)

		assert.Nil(t, resp)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "client_id")
	})

	t.Run("should rollback on repository error", func(t *testing.T) {
		repo := &mockTaskRepository{
			createFunc: func(ctx context.Context, task *domain.Task) (*domain.Task, error) {
				return nil, errors.New("database error")
			},
		}
		outbox := &mockOutboxWriter{}
		tx := &mockTransactionManager{
			rollbackFunc: func(ctx context.Context) error { return nil },
		}

		svc := newTestTaskService(repo, &mockSubtaskRepository{}, outbox, tx)

		req := SubmitRequest{
			Goal:          "Test task",
			RepositoryURL: "https://github.com/test/repo",
			BaseBranch:    "main",
			ClientID:      "client-123",
		}

		resp, err := svc.Submit(context.Background(), req)

		assert.Nil(t, resp)
		assert.Error(t, err)
	})

	t.Run("should rollback on outbox write error", func(t *testing.T) {
		repo := &mockTaskRepository{
			createFunc: func(ctx context.Context, task *domain.Task) (*domain.Task, error) {
				return task, nil
			},
		}
		outbox := &mockOutboxWriter{
			writeFunc: func(ctx context.Context, tx interface{}, event *domain.DomainEvent) error {
				return errors.New("outbox error")
			},
		}
		tx := &mockTransactionManager{
			rollbackFunc: func(ctx context.Context) error { return nil },
		}

		svc := newTestTaskService(repo, &mockSubtaskRepository{}, outbox, tx)

		req := SubmitRequest{
			Goal:          "Test task",
			RepositoryURL: "https://github.com/test/repo",
			BaseBranch:    "main",
			ClientID:      "client-123",
		}

		resp, err := svc.Submit(context.Background(), req)

		assert.Nil(t, resp)
		assert.Error(t, err)
	})
}

func TestTaskService_Get(t *testing.T) {
	t.Run("should get task successfully", func(t *testing.T) {
		expectedTask := &domain.Task{
			AggregateRoot: domain.AggregateRoot{
				Entity: domain.Entity{
					ID:      "task-123",
					Version: 1,
				},
			},
			Goal:          "Test task",
			Status:        domain.TaskStatusPending,
			RepositoryURL: "https://github.com/test/repo",
		}

		repo := &mockTaskRepository{
			getByIDFunc: func(ctx context.Context, id string) (*domain.Task, error) {
				if id == "task-123" {
					return expectedTask, nil
				}
				return nil, domain.NewL2AggregateNotFoundError("Task", id)
			},
		}

		svc := newTestTaskService(repo, &mockSubtaskRepository{}, &mockOutboxWriter{}, &mockTransactionManager{})

		resp, err := svc.Get(context.Background(), GetRequest{TaskID: "task-123"})

		require.NoError(t, err)
		assert.Equal(t, "task-123", resp.Task.ID)
		assert.Equal(t, "Test task", resp.Task.Goal)
	})

	t.Run("should return error when task_id is empty", func(t *testing.T) {
		svc := newTestTaskService(&mockTaskRepository{}, &mockSubtaskRepository{}, &mockOutboxWriter{}, &mockTransactionManager{})

		resp, err := svc.Get(context.Background(), GetRequest{TaskID: ""})

		assert.Nil(t, resp)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "task_id")
	})

	t.Run("should return L4TaskNotFound when task does not exist", func(t *testing.T) {
		repo := &mockTaskRepository{
			getByIDFunc: func(ctx context.Context, id string) (*domain.Task, error) {
				return nil, domain.NewL2AggregateNotFoundError("Task", id)
			},
		}

		svc := newTestTaskService(repo, &mockSubtaskRepository{}, &mockOutboxWriter{}, &mockTransactionManager{})

		resp, err := svc.Get(context.Background(), GetRequest{TaskID: "non-existent"})

		assert.Nil(t, resp)
		assert.Error(t, err)
		assert.True(t, domain.CodeIs(err, domain.CodeL4TaskNotFound))
	})
}

func TestTaskService_List(t *testing.T) {
	t.Run("should list tasks with default pagination", func(t *testing.T) {
		tasks := []*domain.Task{
			{Goal: "Task 1", Status: domain.TaskStatusPending},
			{Goal: "Task 2", Status: domain.TaskStatusPending},
		}

		repo := &mockTaskRepository{
			listFunc: func(ctx context.Context, limit, offset int) ([]*domain.Task, int, error) {
				return tasks, 2, nil
			},
		}

		svc := newTestTaskService(repo, &mockSubtaskRepository{}, &mockOutboxWriter{}, &mockTransactionManager{})

		resp, err := svc.List(context.Background(), ListRequest{Limit: 0, Offset: 0})

		require.NoError(t, err)
		assert.Len(t, resp.Tasks, 2)
		assert.Equal(t, 2, resp.Total)
		assert.Equal(t, 20, resp.Limit) // default limit
	})

	t.Run("should list tasks by status", func(t *testing.T) {
		tasks := []*domain.Task{
			{Goal: "Failed Task", Status: domain.TaskStatusFailed},
		}

		repo := &mockTaskRepository{
			listByStatusFunc: func(ctx context.Context, status domain.TaskStatus, limit, offset int) ([]*domain.Task, int, error) {
				if status == domain.TaskStatusFailed {
					return tasks, 1, nil
				}
				return nil, 0, nil
			},
		}

		svc := newTestTaskService(repo, &mockSubtaskRepository{}, &mockOutboxWriter{}, &mockTransactionManager{})

		status := domain.TaskStatusFailed
		resp, err := svc.List(context.Background(), ListRequest{Status: &status, Limit: 10, Offset: 0})

		require.NoError(t, err)
		assert.Len(t, resp.Tasks, 1)
		assert.Equal(t, 1, resp.Total)
	})

	t.Run("should list tasks by client_id", func(t *testing.T) {
		tasks := []*domain.Task{
			{Goal: "Client Task", Status: domain.TaskStatusPending, ClientID: "client-123"},
		}

		repo := &mockTaskRepository{
			listByClientIDFunc: func(ctx context.Context, clientID string, limit, offset int) ([]*domain.Task, int, error) {
				if clientID == "client-123" {
					return tasks, 1, nil
				}
				return nil, 0, nil
			},
		}

		svc := newTestTaskService(repo, &mockSubtaskRepository{}, &mockOutboxWriter{}, &mockTransactionManager{})

		resp, err := svc.List(context.Background(), ListRequest{ClientID: "client-123", Limit: 10, Offset: 0})

		require.NoError(t, err)
		assert.Len(t, resp.Tasks, 1)
		assert.Equal(t, 1, resp.Total)
	})

	t.Run("should cap limit at 100", func(t *testing.T) {
		repo := &mockTaskRepository{
			listFunc: func(ctx context.Context, limit, offset int) ([]*domain.Task, int, error) {
				assert.Equal(t, 100, limit) // should be capped
				return nil, 0, nil
			},
		}

		svc := newTestTaskService(repo, &mockSubtaskRepository{}, &mockOutboxWriter{}, &mockTransactionManager{})

		_, err := svc.List(context.Background(), ListRequest{Limit: 500, Offset: 0})

		require.NoError(t, err)
	})
}

func TestTaskService_Cancel(t *testing.T) {
	t.Run("should cancel pending task successfully", func(t *testing.T) {
		pendingTask := &domain.Task{
			AggregateRoot: domain.AggregateRoot{
				Entity: domain.Entity{
					ID:      "task-123",
					Version: 1,
				},
			},
			Goal:   "Task to cancel",
			Status: domain.TaskStatusPending,
		}

		repo := &mockTaskRepository{
			getByIDFunc: func(ctx context.Context, id string) (*domain.Task, error) {
				return pendingTask, nil
			},
			updateStatusFunc: func(ctx context.Context, id string, status domain.TaskStatus, expectedVersion int64) (*domain.Task, error) {
				pendingTask.Status = status
				return pendingTask, nil
			},
		}
		outbox := &mockOutboxWriter{
			writeFunc: func(ctx context.Context, tx interface{}, event *domain.DomainEvent) error {
				return nil
			},
		}
		tx := &mockTransactionManager{
			commitFunc: func(ctx context.Context) error { return nil },
		}

		svc := newTestTaskService(repo, &mockSubtaskRepository{}, outbox, tx)

		resp, err := svc.Cancel(context.Background(), CancelRequest{TaskID: "task-123", Reason: "User requested"})

		require.NoError(t, err)
		assert.Equal(t, domain.TaskStatusCancelled, resp.Task.Status)
	})

	t.Run("should cancel running task successfully", func(t *testing.T) {
		runningTask := &domain.Task{
			AggregateRoot: domain.AggregateRoot{
				Entity: domain.Entity{
					ID:      "task-456",
					Version: 2,
				},
			},
			Goal:   "Running task",
			Status: domain.TaskStatusRunning,
		}

		repo := &mockTaskRepository{
			getByIDFunc: func(ctx context.Context, id string) (*domain.Task, error) {
				return runningTask, nil
			},
			updateStatusFunc: func(ctx context.Context, id string, status domain.TaskStatus, expectedVersion int64) (*domain.Task, error) {
				runningTask.Status = status
				return runningTask, nil
			},
		}
		outbox := &mockOutboxWriter{
			writeFunc: func(ctx context.Context, tx interface{}, event *domain.DomainEvent) error {
				return nil
			},
		}
		tx := &mockTransactionManager{
			commitFunc: func(ctx context.Context) error { return nil },
		}

		svc := newTestTaskService(repo, &mockSubtaskRepository{}, outbox, tx)

		resp, err := svc.Cancel(context.Background(), CancelRequest{TaskID: "task-456", Reason: "User requested"})

		require.NoError(t, err)
		assert.Equal(t, domain.TaskStatusCancelled, resp.Task.Status)
	})

	t.Run("should return error when task_id is empty", func(t *testing.T) {
		svc := newTestTaskService(&mockTaskRepository{}, &mockSubtaskRepository{}, &mockOutboxWriter{}, &mockTransactionManager{})

		resp, err := svc.Cancel(context.Background(), CancelRequest{TaskID: ""})

		assert.Nil(t, resp)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "task_id")
	})

	t.Run("should return L4TaskNotFound when task does not exist", func(t *testing.T) {
		repo := &mockTaskRepository{
			getByIDFunc: func(ctx context.Context, id string) (*domain.Task, error) {
				return nil, domain.NewL2AggregateNotFoundError("Task", id)
			},
		}

		svc := newTestTaskService(repo, &mockSubtaskRepository{}, &mockOutboxWriter{}, &mockTransactionManager{})

		resp, err := svc.Cancel(context.Background(), CancelRequest{TaskID: "non-existent"})

		assert.Nil(t, resp)
		assert.Error(t, err)
		assert.True(t, domain.CodeIs(err, domain.CodeL4TaskNotFound))
	})

	t.Run("should return L4TaskStateInvalid when task is already completed", func(t *testing.T) {
		completedTask := &domain.Task{
			AggregateRoot: domain.AggregateRoot{
				Entity: domain.Entity{
					ID:      "task-789",
					Version: 1,
				},
			},
			Goal:   "Completed task",
			Status: domain.TaskStatusCompleted,
		}

		repo := &mockTaskRepository{
			getByIDFunc: func(ctx context.Context, id string) (*domain.Task, error) {
				return completedTask, nil
			},
		}

		svc := newTestTaskService(repo, &mockSubtaskRepository{}, &mockOutboxWriter{}, &mockTransactionManager{})

		resp, err := svc.Cancel(context.Background(), CancelRequest{TaskID: "task-789"})

		assert.Nil(t, resp)
		assert.Error(t, err)
		assert.True(t, domain.CodeIs(err, domain.CodeL4TaskStateInvalid))
	})

	t.Run("should rollback on update status error", func(t *testing.T) {
		pendingTask := &domain.Task{
			AggregateRoot: domain.AggregateRoot{
				Entity: domain.Entity{
					ID:      "task-123",
					Version: 1,
				},
			},
			Goal:   "Task to cancel",
			Status: domain.TaskStatusPending,
		}

		repo := &mockTaskRepository{
			getByIDFunc: func(ctx context.Context, id string) (*domain.Task, error) {
				return pendingTask, nil
			},
			updateStatusFunc: func(ctx context.Context, id string, status domain.TaskStatus, expectedVersion int64) (*domain.Task, error) {
				return nil, errors.New("update error")
			},
		}
		outbox := &mockOutboxWriter{}
		tx := &mockTransactionManager{
			rollbackFunc: func(ctx context.Context) error { return nil },
		}

		svc := newTestTaskService(repo, &mockSubtaskRepository{}, outbox, tx)

		resp, err := svc.Cancel(context.Background(), CancelRequest{TaskID: "task-123"})

		assert.Nil(t, resp)
		assert.Error(t, err)
	})
}

func TestTaskService_Retry(t *testing.T) {
	t.Run("should retry failed task successfully", func(t *testing.T) {
		failedTask := &domain.Task{
			AggregateRoot: domain.AggregateRoot{
				Entity: domain.Entity{
					ID:      "task-123",
					Version: 3,
				},
			},
			Goal:   "Failed task",
			Status: domain.TaskStatusFailed,
		}

		updatedTask := &domain.Task{
			AggregateRoot: domain.AggregateRoot{
				Entity: domain.Entity{
					ID:      "task-123",
					Version: 4,
				},
			},
			Goal:   "Failed task",
			Status: domain.TaskStatusPending,
		}

		repo := &mockTaskRepository{
			getByIDFunc: func(ctx context.Context, id string) (*domain.Task, error) {
				return failedTask, nil
			},
			updateFunc: func(ctx context.Context, task *domain.Task) (*domain.Task, error) {
				return updatedTask, nil
			},
		}
		outbox := &mockOutboxWriter{
			writeFunc: func(ctx context.Context, tx interface{}, event *domain.DomainEvent) error {
				return nil
			},
		}
		tx := &mockTransactionManager{
			commitFunc: func(ctx context.Context) error { return nil },
		}

		svc := newTestTaskService(repo, &mockSubtaskRepository{}, outbox, tx)

		resp, err := svc.Retry(context.Background(), RetryRequest{TaskID: "task-123"})

		require.NoError(t, err)
		assert.Equal(t, domain.TaskStatusPending, resp.Task.Status)
	})

	t.Run("should return error when task_id is empty", func(t *testing.T) {
		svc := newTestTaskService(&mockTaskRepository{}, &mockSubtaskRepository{}, &mockOutboxWriter{}, &mockTransactionManager{})

		resp, err := svc.Retry(context.Background(), RetryRequest{TaskID: ""})

		assert.Nil(t, resp)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "task_id")
	})

	t.Run("should return L4TaskNotFound when task does not exist", func(t *testing.T) {
		repo := &mockTaskRepository{
			getByIDFunc: func(ctx context.Context, id string) (*domain.Task, error) {
				return nil, domain.NewL2AggregateNotFoundError("Task", id)
			},
		}

		svc := newTestTaskService(repo, &mockSubtaskRepository{}, &mockOutboxWriter{}, &mockTransactionManager{})

		resp, err := svc.Retry(context.Background(), RetryRequest{TaskID: "non-existent"})

		assert.Nil(t, resp)
		assert.Error(t, err)
		assert.True(t, domain.CodeIs(err, domain.CodeL4TaskNotFound))
	})

	t.Run("should return L4TaskStateInvalid when task is not failed", func(t *testing.T) {
		pendingTask := &domain.Task{
			AggregateRoot: domain.AggregateRoot{
				Entity: domain.Entity{
					ID:      "task-123",
					Version: 1,
				},
			},
			Goal:   "Pending task",
			Status: domain.TaskStatusPending,
		}

		repo := &mockTaskRepository{
			getByIDFunc: func(ctx context.Context, id string) (*domain.Task, error) {
				return pendingTask, nil
			},
		}

		svc := newTestTaskService(repo, &mockSubtaskRepository{}, &mockOutboxWriter{}, &mockTransactionManager{})

		resp, err := svc.Retry(context.Background(), RetryRequest{TaskID: "task-123"})

		assert.Nil(t, resp)
		assert.Error(t, err)
		assert.True(t, domain.CodeIs(err, domain.CodeL4TaskStateInvalid))
	})

	t.Run("should rollback on update error", func(t *testing.T) {
		failedTask := &domain.Task{
			AggregateRoot: domain.AggregateRoot{
				Entity: domain.Entity{
					ID:      "task-123",
					Version: 3,
				},
			},
			Goal:   "Failed task",
			Status: domain.TaskStatusFailed,
		}

		repo := &mockTaskRepository{
			getByIDFunc: func(ctx context.Context, id string) (*domain.Task, error) {
				return failedTask, nil
			},
			updateFunc: func(ctx context.Context, task *domain.Task) (*domain.Task, error) {
				return nil, errors.New("update error")
			},
		}
		outbox := &mockOutboxWriter{}
		tx := &mockTransactionManager{
			rollbackFunc: func(ctx context.Context) error { return nil },
		}

		svc := newTestTaskService(repo, &mockSubtaskRepository{}, outbox, tx)

		resp, err := svc.Retry(context.Background(), RetryRequest{TaskID: "task-123"})

		assert.Nil(t, resp)
		assert.Error(t, err)
	})
}

func TestTaskService_Decompose(t *testing.T) {
	t.Run("should decompose task successfully", func(t *testing.T) {
		pendingTask := &domain.Task{
			AggregateRoot: domain.AggregateRoot{
				Entity: domain.Entity{
					ID:      "task-123",
					Version: 1,
				},
			},
			Goal:   "Task to decompose",
			Status: domain.TaskStatusPending,
		}

		repo := &mockTaskRepository{
			getByIDFunc: func(ctx context.Context, id string) (*domain.Task, error) {
				return pendingTask, nil
			},
			updateStatusFunc: func(ctx context.Context, id string, status domain.TaskStatus, expectedVersion int64) (*domain.Task, error) {
				pendingTask.Status = status
				pendingTask.Version = expectedVersion + 1
				return pendingTask, nil
			},
		}
		subtaskRepo := &mockSubtaskRepository{
			createFunc: func(ctx context.Context, subtask *domain.Subtask) (*domain.Subtask, error) {
				return subtask, nil
			},
		}
		outbox := &mockOutboxWriter{
			writeFunc: func(ctx context.Context, tx interface{}, event *domain.DomainEvent) error {
				return nil
			},
		}
		tx := &mockTransactionManager{
			commitFunc: func(ctx context.Context) error { return nil },
		}

		svc := newTestTaskService(repo, subtaskRepo, outbox, tx)

		resp, err := svc.Decompose(context.Background(), DecomposeRequest{
			TaskID: "task-123",
			Subtasks: []SubtaskSpec{
				{Type: domain.SubtaskTypeCoding, Description: "Implement feature", AgentTemplate: "executor"},
				{Type: domain.SubtaskTypeReview, Description: "Review code", AgentTemplate: "reviewer"},
			},
		})

		require.NoError(t, err)
		assert.Equal(t, domain.TaskStatusDecomposing, resp.Task.Status)
		assert.Len(t, resp.Subtasks, 2)
	})

	t.Run("should return error when task_id is empty", func(t *testing.T) {
		svc := newTestTaskService(&mockTaskRepository{}, &mockSubtaskRepository{}, &mockOutboxWriter{}, &mockTransactionManager{})

		resp, err := svc.Decompose(context.Background(), DecomposeRequest{
			TaskID: "",
			Subtasks: []SubtaskSpec{
				{Type: domain.SubtaskTypeCoding, Description: "Implement feature", AgentTemplate: "executor"},
			},
		})

		assert.Nil(t, resp)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "task_id")
	})

	t.Run("should return error when no subtasks provided", func(t *testing.T) {
		svc := newTestTaskService(&mockTaskRepository{}, &mockSubtaskRepository{}, &mockOutboxWriter{}, &mockTransactionManager{})

		resp, err := svc.Decompose(context.Background(), DecomposeRequest{
			TaskID:   "task-123",
			Subtasks: []SubtaskSpec{},
		})

		assert.Nil(t, resp)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "at least one subtask")
	})

	t.Run("should return L4TaskNotFound when task does not exist", func(t *testing.T) {
		repo := &mockTaskRepository{
			getByIDFunc: func(ctx context.Context, id string) (*domain.Task, error) {
				return nil, domain.NewL2AggregateNotFoundError("Task", id)
			},
		}

		svc := newTestTaskService(repo, &mockSubtaskRepository{}, &mockOutboxWriter{}, &mockTransactionManager{})

		resp, err := svc.Decompose(context.Background(), DecomposeRequest{
			TaskID: "non-existent",
			Subtasks: []SubtaskSpec{
				{Type: domain.SubtaskTypeCoding, Description: "Implement feature", AgentTemplate: "executor"},
			},
		})

		assert.Nil(t, resp)
		assert.Error(t, err)
		assert.True(t, domain.CodeIs(err, domain.CodeL4TaskNotFound))
	})

	t.Run("should return L4TaskStateInvalid when task is not in pending state", func(t *testing.T) {
		runningTask := &domain.Task{
			AggregateRoot: domain.AggregateRoot{
				Entity: domain.Entity{
					ID:      "task-456",
					Version: 2,
				},
			},
			Goal:   "Running task",
			Status: domain.TaskStatusRunning,
		}

		repo := &mockTaskRepository{
			getByIDFunc: func(ctx context.Context, id string) (*domain.Task, error) {
				return runningTask, nil
			},
		}

		svc := newTestTaskService(repo, &mockSubtaskRepository{}, &mockOutboxWriter{}, &mockTransactionManager{})

		resp, err := svc.Decompose(context.Background(), DecomposeRequest{
			TaskID: "task-456",
			Subtasks: []SubtaskSpec{
				{Type: domain.SubtaskTypeCoding, Description: "Implement feature", AgentTemplate: "executor"},
			},
		})

		assert.Nil(t, resp)
		assert.Error(t, err)
		assert.True(t, domain.CodeIs(err, domain.CodeL4TaskStateInvalid))
	})

	t.Run("should return error on subtask creation failure", func(t *testing.T) {
		pendingTask := &domain.Task{
			AggregateRoot: domain.AggregateRoot{
				Entity: domain.Entity{
					ID:      "task-123",
					Version: 1,
				},
			},
			Goal:   "Task to decompose",
			Status: domain.TaskStatusPending,
		}

		repo := &mockTaskRepository{
			getByIDFunc: func(ctx context.Context, id string) (*domain.Task, error) {
				return pendingTask, nil
			},
		}
		subtaskRepo := &mockSubtaskRepository{
			createFunc: func(ctx context.Context, subtask *domain.Subtask) (*domain.Subtask, error) {
				return nil, errors.New("subtask creation failed")
			},
		}
		outbox := &mockOutboxWriter{}
		tx := &mockTransactionManager{
			rollbackFunc: func(ctx context.Context) error { return nil },
		}

		svc := newTestTaskService(repo, subtaskRepo, outbox, tx)

		resp, err := svc.Decompose(context.Background(), DecomposeRequest{
			TaskID: "task-123",
			Subtasks: []SubtaskSpec{
				{Type: domain.SubtaskTypeCoding, Description: "Implement feature", AgentTemplate: "executor"},
			},
		})

		assert.Nil(t, resp)
		assert.Error(t, err)
	})

	t.Run("should return error on update status failure", func(t *testing.T) {
		pendingTask := &domain.Task{
			AggregateRoot: domain.AggregateRoot{
				Entity: domain.Entity{
					ID:      "task-123",
					Version: 1,
				},
			},
			Goal:   "Task to decompose",
			Status: domain.TaskStatusPending,
		}

		repo := &mockTaskRepository{
			getByIDFunc: func(ctx context.Context, id string) (*domain.Task, error) {
				return pendingTask, nil
			},
			updateStatusFunc: func(ctx context.Context, id string, status domain.TaskStatus, expectedVersion int64) (*domain.Task, error) {
				return nil, errors.New("update status failed")
			},
		}
		subtaskRepo := &mockSubtaskRepository{
			createFunc: func(ctx context.Context, subtask *domain.Subtask) (*domain.Subtask, error) {
				return subtask, nil
			},
		}
		outbox := &mockOutboxWriter{}
		tx := &mockTransactionManager{
			rollbackFunc: func(ctx context.Context) error { return nil },
		}

		svc := newTestTaskService(repo, subtaskRepo, outbox, tx)

		resp, err := svc.Decompose(context.Background(), DecomposeRequest{
			TaskID: "task-123",
			Subtasks: []SubtaskSpec{
				{Type: domain.SubtaskTypeCoding, Description: "Implement feature", AgentTemplate: "executor"},
			},
		})

		assert.Nil(t, resp)
		assert.Error(t, err)
	})
}
