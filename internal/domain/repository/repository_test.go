// Package repository provides mock implementations and tests for repository interfaces.
// This package is part of L2-Domain layer and has ZERO external dependencies.
package repository

import (
	"context"
	"testing"
	"time"

	"github.com/cloud-agent-platform/cap/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ----------------------------------------------------------------------------
// Mock Implementations for compile-time interface verification
// ----------------------------------------------------------------------------

// MockTaskRepository is a mock implementation of domain.TaskRepository.
type MockTaskRepository struct {
	CreateFunc           func(ctx context.Context, task *domain.Task) (*domain.Task, error)
	GetByIDFunc          func(ctx context.Context, id string) (*domain.Task, error)
	UpdateFunc           func(ctx context.Context, task *domain.Task) (*domain.Task, error)
	DeleteFunc           func(ctx context.Context, id string) error
	ListByStatusFunc     func(ctx context.Context, status domain.TaskStatus, limit, offset int) ([]*domain.Task, int, error)
	ListByClientIDFunc   func(ctx context.Context, clientID string, limit, offset int) ([]*domain.Task, int, error)
	ListFunc             func(ctx context.Context, limit, offset int) ([]*domain.Task, int, error)
	UpdateStatusFunc     func(ctx context.Context, id string, status domain.TaskStatus, expectedVersion int64) (*domain.Task, error)
	CountByStatusFunc    func(ctx context.Context, status domain.TaskStatus) (int, error)
}

func (m *MockTaskRepository) Create(ctx context.Context, task *domain.Task) (*domain.Task, error) {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, task)
	}
	return task, nil
}

func (m *MockTaskRepository) GetByID(ctx context.Context, id string) (*domain.Task, error) {
	if m.GetByIDFunc != nil {
		return m.GetByIDFunc(ctx, id)
	}
	return nil, nil
}

func (m *MockTaskRepository) Update(ctx context.Context, task *domain.Task) (*domain.Task, error) {
	if m.UpdateFunc != nil {
		return m.UpdateFunc(ctx, task)
	}
	return task, nil
}

func (m *MockTaskRepository) Delete(ctx context.Context, id string) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, id)
	}
	return nil
}

func (m *MockTaskRepository) ListByStatus(ctx context.Context, status domain.TaskStatus, limit, offset int) ([]*domain.Task, int, error) {
	if m.ListByStatusFunc != nil {
		return m.ListByStatusFunc(ctx, status, limit, offset)
	}
	return nil, 0, nil
}

func (m *MockTaskRepository) ListByClientID(ctx context.Context, clientID string, limit, offset int) ([]*domain.Task, int, error) {
	if m.ListByClientIDFunc != nil {
		return m.ListByClientIDFunc(ctx, clientID, limit, offset)
	}
	return nil, 0, nil
}

func (m *MockTaskRepository) List(ctx context.Context, limit, offset int) ([]*domain.Task, int, error) {
	if m.ListFunc != nil {
		return m.ListFunc(ctx, limit, offset)
	}
	return nil, 0, nil
}

func (m *MockTaskRepository) UpdateStatus(ctx context.Context, id string, status domain.TaskStatus, expectedVersion int64) (*domain.Task, error) {
	if m.UpdateStatusFunc != nil {
		return m.UpdateStatusFunc(ctx, id, status, expectedVersion)
	}
	return nil, nil
}

func (m *MockTaskRepository) CountByStatus(ctx context.Context, status domain.TaskStatus) (int, error) {
	if m.CountByStatusFunc != nil {
		return m.CountByStatusFunc(ctx, status)
	}
	return 0, nil
}

// MockSubtaskRepository is a mock implementation of domain.SubtaskRepository.
type MockSubtaskRepository struct {
	CreateFunc      func(ctx context.Context, subtask *domain.Subtask) (*domain.Subtask, error)
	GetByIDFunc     func(ctx context.Context, id string) (*domain.Subtask, error)
	UpdateFunc      func(ctx context.Context, subtask *domain.Subtask) (*domain.Subtask, error)
	DeleteFunc      func(ctx context.Context, id string) error
	ListByTaskIDFunc func(ctx context.Context, taskID string) ([]*domain.Subtask, error)
	UpdateStatusFunc func(ctx context.Context, id string, status domain.TaskStatus, expectedVersion int64) (*domain.Subtask, error)
	ListByStatusFunc func(ctx context.Context, status domain.TaskStatus, limit, offset int) ([]*domain.Subtask, int, error)
}

func (m *MockSubtaskRepository) Create(ctx context.Context, subtask *domain.Subtask) (*domain.Subtask, error) {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, subtask)
	}
	return subtask, nil
}

func (m *MockSubtaskRepository) GetByID(ctx context.Context, id string) (*domain.Subtask, error) {
	if m.GetByIDFunc != nil {
		return m.GetByIDFunc(ctx, id)
	}
	return nil, nil
}

func (m *MockSubtaskRepository) Update(ctx context.Context, subtask *domain.Subtask) (*domain.Subtask, error) {
	if m.UpdateFunc != nil {
		return m.UpdateFunc(ctx, subtask)
	}
	return subtask, nil
}

func (m *MockSubtaskRepository) Delete(ctx context.Context, id string) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, id)
	}
	return nil
}

func (m *MockSubtaskRepository) ListByTaskID(ctx context.Context, taskID string) ([]*domain.Subtask, error) {
	if m.ListByTaskIDFunc != nil {
		return m.ListByTaskIDFunc(ctx, taskID)
	}
	return nil, nil
}

func (m *MockSubtaskRepository) UpdateStatus(ctx context.Context, id string, status domain.TaskStatus, expectedVersion int64) (*domain.Subtask, error) {
	if m.UpdateStatusFunc != nil {
		return m.UpdateStatusFunc(ctx, id, status, expectedVersion)
	}
	return nil, nil
}

func (m *MockSubtaskRepository) ListByStatus(ctx context.Context, status domain.TaskStatus, limit, offset int) ([]*domain.Subtask, int, error) {
	if m.ListByStatusFunc != nil {
		return m.ListByStatusFunc(ctx, status, limit, offset)
	}
	return nil, 0, nil
}

// MockAuditLogRepository is a mock implementation of domain.AuditLogRepository.
type MockAuditLogRepository struct {
	CreateFunc          func(ctx context.Context, log *domain.AuditLog) (*domain.AuditLog, error)
	ListByTaskIDFunc    func(ctx context.Context, taskID string, limit, offset int) ([]*domain.AuditLog, int, error)
	ListBySubtaskIDFunc func(ctx context.Context, subtaskID string, limit, offset int) ([]*domain.AuditLog, int, error)
}

func (m *MockAuditLogRepository) Create(ctx context.Context, log *domain.AuditLog) (*domain.AuditLog, error) {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, log)
	}
	return log, nil
}

func (m *MockAuditLogRepository) ListByTaskID(ctx context.Context, taskID string, limit, offset int) ([]*domain.AuditLog, int, error) {
	if m.ListByTaskIDFunc != nil {
		return m.ListByTaskIDFunc(ctx, taskID, limit, offset)
	}
	return nil, 0, nil
}

func (m *MockAuditLogRepository) ListBySubtaskID(ctx context.Context, subtaskID string, limit, offset int) ([]*domain.AuditLog, int, error) {
	if m.ListBySubtaskIDFunc != nil {
		return m.ListBySubtaskIDFunc(ctx, subtaskID, limit, offset)
	}
	return nil, 0, nil
}

// MockOutboxRepository is a mock implementation of domain.OutboxRepository.
type MockOutboxRepository struct {
	WriteFunc            func(ctx context.Context, tx interface{}, event *domain.DomainEvent) error
	ReadPendingFunc      func(ctx context.Context, limit int) ([]*domain.OutboxEvent, error)
	MarkPublishedFunc    func(ctx context.Context, id string) error
	MarkFailedFunc       func(ctx context.Context, id string, lastError string) error
	DeleteOldPublishedFunc func(ctx context.Context, olderThan int64) (int64, error)
}

func (m *MockOutboxRepository) Write(ctx context.Context, tx interface{}, event *domain.DomainEvent) error {
	if m.WriteFunc != nil {
		return m.WriteFunc(ctx, tx, event)
	}
	return nil
}

func (m *MockOutboxRepository) ReadPending(ctx context.Context, limit int) ([]*domain.OutboxEvent, error) {
	if m.ReadPendingFunc != nil {
		return m.ReadPendingFunc(ctx, limit)
	}
	return nil, nil
}

func (m *MockOutboxRepository) MarkPublished(ctx context.Context, id string) error {
	if m.MarkPublishedFunc != nil {
		return m.MarkPublishedFunc(ctx, id)
	}
	return nil
}

func (m *MockOutboxRepository) MarkFailed(ctx context.Context, id string, lastError string) error {
	if m.MarkFailedFunc != nil {
		return m.MarkFailedFunc(ctx, id, lastError)
	}
	return nil
}

func (m *MockOutboxRepository) DeleteOldPublished(ctx context.Context, olderThan int64) (int64, error) {
	if m.DeleteOldPublishedFunc != nil {
		return m.DeleteOldPublishedFunc(ctx, olderThan)
	}
	return 0, nil
}

// MockObjectStorage is a mock implementation of domain.ObjectStorage.
type MockObjectStorage struct {
	UploadFunc             func(ctx context.Context, key string, data []byte, contentType string) error
	DownloadFunc           func(ctx context.Context, key string) ([]byte, error)
	GeneratePresignedURLFunc func(ctx context.Context, key string, expiry time.Duration) (string, error)
	DeleteFunc             func(ctx context.Context, key string) error
	ListExpiredObjectsFunc func(ctx context.Context, olderThan time.Duration) ([]string, error)
	ExistsFunc             func(ctx context.Context, key string) (bool, error)
	BucketNameFunc         func() string
}

func (m *MockObjectStorage) Upload(ctx context.Context, key string, data []byte, contentType string) error {
	if m.UploadFunc != nil {
		return m.UploadFunc(ctx, key, data, contentType)
	}
	return nil
}

func (m *MockObjectStorage) Download(ctx context.Context, key string) ([]byte, error) {
	if m.DownloadFunc != nil {
		return m.DownloadFunc(ctx, key)
	}
	return nil, nil
}

func (m *MockObjectStorage) GeneratePresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	if m.GeneratePresignedURLFunc != nil {
		return m.GeneratePresignedURLFunc(ctx, key, expiry)
	}
	return "", nil
}

func (m *MockObjectStorage) Delete(ctx context.Context, key string) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, key)
	}
	return nil
}

func (m *MockObjectStorage) ListExpiredObjects(ctx context.Context, olderThan time.Duration) ([]string, error) {
	if m.ListExpiredObjectsFunc != nil {
		return m.ListExpiredObjectsFunc(ctx, olderThan)
	}
	return nil, nil
}

func (m *MockObjectStorage) Exists(ctx context.Context, key string) (bool, error) {
	if m.ExistsFunc != nil {
		return m.ExistsFunc(ctx, key)
	}
	return false, nil
}

func (m *MockObjectStorage) BucketName() string {
	if m.BucketNameFunc != nil {
		return m.BucketNameFunc()
	}
	return ""
}

// ----------------------------------------------------------------------------
// Compile-time interface verification
// These tests verify that the mock implementations satisfy the interfaces.
// ----------------------------------------------------------------------------

// TestTaskRepositoryInterface verifies MockTaskRepository implements domain.TaskRepository.
func TestTaskRepositoryInterface(t *testing.T) {
	var _ domain.TaskRepository = (*MockTaskRepository)(nil)
}

// TestSubtaskRepositoryInterface verifies MockSubtaskRepository implements domain.SubtaskRepository.
func TestSubtaskRepositoryInterface(t *testing.T) {
	var _ domain.SubtaskRepository = (*MockSubtaskRepository)(nil)
}

// TestAuditLogRepositoryInterface verifies MockAuditLogRepository implements domain.AuditLogRepository.
func TestAuditLogRepositoryInterface(t *testing.T) {
	var _ domain.AuditLogRepository = (*MockAuditLogRepository)(nil)
}

// TestOutboxRepositoryInterface verifies MockOutboxRepository implements domain.OutboxRepository.
func TestOutboxRepositoryInterface(t *testing.T) {
	var _ domain.OutboxRepository = (*MockOutboxRepository)(nil)
}

// TestObjectStorageInterface verifies MockObjectStorage implements domain.ObjectStorage.
func TestObjectStorageInterface(t *testing.T) {
	var _ domain.ObjectStorage = (*MockObjectStorage)(nil)
}

// ----------------------------------------------------------------------------
// TaskRepository method signature tests
// ----------------------------------------------------------------------------

// TestTaskRepositoryCreate verifies the Create method signature.
func TestTaskRepositoryCreate(t *testing.T) {
	mock := &MockTaskRepository{
		CreateFunc: func(ctx context.Context, task *domain.Task) (*domain.Task, error) {
			require.NotNil(t, ctx)
			require.NotNil(t, task)
			assert.Equal(t, "test-task-id", task.ID)
			return task, nil
		},
	}

	task := domain.NewTask("test-task-id", "Test Goal", "https://github.com/test/repo", "main", "client-123")
	result, err := mock.Create(context.Background(), task)

	require.NoError(t, err)
	assert.Equal(t, "test-task-id", result.ID)
	assert.Equal(t, "Test Goal", result.Goal)
}

// TestTaskRepositoryGetByID verifies the GetByID method signature.
func TestTaskRepositoryGetByID(t *testing.T) {
	mock := &MockTaskRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*domain.Task, error) {
			require.NotNil(t, ctx)
			assert.Equal(t, "test-task-id", id)
			return domain.NewTask(id, "Retrieved Goal", "https://github.com/test/repo", "main", "client-123"), nil
		},
	}

	result, err := mock.GetByID(context.Background(), "test-task-id")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "test-task-id", result.ID)
}

// TestTaskRepositoryUpdate verifies the Update method signature.
func TestTaskRepositoryUpdate(t *testing.T) {
	mock := &MockTaskRepository{
		UpdateFunc: func(ctx context.Context, task *domain.Task) (*domain.Task, error) {
			require.NotNil(t, ctx)
			require.NotNil(t, task)
			task.Progress = 50.0
			return task, nil
		},
	}

	task := domain.NewTask("test-task-id", "Test Goal", "https://github.com/test/repo", "main", "client-123")
	result, err := mock.Update(context.Background(), task)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 50.0, result.Progress)
}

// TestTaskRepositoryDelete verifies the Delete method signature.
func TestTaskRepositoryDelete(t *testing.T) {
	deleteCalled := false
	mock := &MockTaskRepository{
		DeleteFunc: func(ctx context.Context, id string) error {
			require.NotNil(t, ctx)
			assert.Equal(t, "test-task-id", id)
			deleteCalled = true
			return nil
		},
	}

	err := mock.Delete(context.Background(), "test-task-id")

	require.NoError(t, err)
	assert.True(t, deleteCalled)
}

// TestTaskRepositoryListByStatus verifies the ListByStatus method signature.
func TestTaskRepositoryListByStatus(t *testing.T) {
	mock := &MockTaskRepository{
		ListByStatusFunc: func(ctx context.Context, status domain.TaskStatus, limit, offset int) ([]*domain.Task, int, error) {
			require.NotNil(t, ctx)
			assert.Equal(t, domain.TaskStatusPending, status)
			assert.Equal(t, 10, limit)
			assert.Equal(t, 0, offset)
			tasks := []*domain.Task{
				domain.NewTask("task-1", "Goal 1", "https://github.com/test/repo", "main", "client-123"),
				domain.NewTask("task-2", "Goal 2", "https://github.com/test/repo", "main", "client-123"),
			}
			return tasks, 2, nil
		},
	}

	tasks, total, err := mock.ListByStatus(context.Background(), domain.TaskStatusPending, 10, 0)

	require.NoError(t, err)
	assert.Equal(t, 2, len(tasks))
	assert.Equal(t, 2, total)
}

// TestTaskRepositoryCountByStatus verifies the CountByStatus method signature.
func TestTaskRepositoryCountByStatus(t *testing.T) {
	mock := &MockTaskRepository{
		CountByStatusFunc: func(ctx context.Context, status domain.TaskStatus) (int, error) {
			require.NotNil(t, ctx)
			assert.Equal(t, domain.TaskStatusRunning, status)
			return 5, nil
		},
	}

	count, err := mock.CountByStatus(context.Background(), domain.TaskStatusRunning)

	require.NoError(t, err)
	assert.Equal(t, 5, count)
}

// TestTaskRepositoryUpdateStatus verifies the UpdateStatus method signature.
func TestTaskRepositoryUpdateStatus(t *testing.T) {
	mock := &MockTaskRepository{
		UpdateStatusFunc: func(ctx context.Context, id string, status domain.TaskStatus, expectedVersion int64) (*domain.Task, error) {
			require.NotNil(t, ctx)
			assert.Equal(t, "test-task-id", id)
			assert.Equal(t, domain.TaskStatusRunning, status)
			assert.Equal(t, int64(1), expectedVersion)
			task := domain.NewTask(id, "Test", "https://github.com/test/repo", "main", "client-123")
			task.Status = status
			return task, nil
		},
	}

	result, err := mock.UpdateStatus(context.Background(), "test-task-id", domain.TaskStatusRunning, 1)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, domain.TaskStatusRunning, result.Status)
}

// ----------------------------------------------------------------------------
// SubtaskRepository method signature tests
// ----------------------------------------------------------------------------

// TestSubtaskRepositoryCreate verifies the Create method signature.
func TestSubtaskRepositoryCreate(t *testing.T) {
	mock := &MockSubtaskRepository{
		CreateFunc: func(ctx context.Context, subtask *domain.Subtask) (*domain.Subtask, error) {
			require.NotNil(t, ctx)
			require.NotNil(t, subtask)
			assert.Equal(t, "test-subtask-id", subtask.ID)
			return subtask, nil
		},
	}

	subtask := domain.NewSubtask("test-subtask-id", "parent-task-id", domain.SubtaskTypeCoding, "Write code", "executor")
	result, err := mock.Create(context.Background(), subtask)

	require.NoError(t, err)
	assert.Equal(t, "test-subtask-id", result.ID)
}

// TestSubtaskRepositoryGetByID verifies the GetByID method signature.
func TestSubtaskRepositoryGetByID(t *testing.T) {
	mock := &MockSubtaskRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*domain.Subtask, error) {
			require.NotNil(t, ctx)
			assert.Equal(t, "test-subtask-id", id)
			return domain.NewSubtask(id, "parent-task-id", domain.SubtaskTypeCoding, "Write code", "executor"), nil
		},
	}

	result, err := mock.GetByID(context.Background(), "test-subtask-id")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "test-subtask-id", result.ID)
}

// TestSubtaskRepositoryListByTaskID verifies the ListByTaskID method signature.
func TestSubtaskRepositoryListByTaskID(t *testing.T) {
	mock := &MockSubtaskRepository{
		ListByTaskIDFunc: func(ctx context.Context, taskID string) ([]*domain.Subtask, error) {
			require.NotNil(t, ctx)
			assert.Equal(t, "parent-task-id", taskID)
			subtasks := []*domain.Subtask{
				domain.NewSubtask("subtask-1", taskID, domain.SubtaskTypeCoding, "Code", "executor"),
				domain.NewSubtask("subtask-2", taskID, domain.SubtaskTypeReview, "Review", "reviewer"),
			}
			return subtasks, nil
		},
	}

	subtasks, err := mock.ListByTaskID(context.Background(), "parent-task-id")

	require.NoError(t, err)
	assert.Equal(t, 2, len(subtasks))
}

// TestSubtaskRepositoryUpdateStatus verifies the UpdateStatus method signature.
func TestSubtaskRepositoryUpdateStatus(t *testing.T) {
	mock := &MockSubtaskRepository{
		UpdateStatusFunc: func(ctx context.Context, id string, status domain.TaskStatus, expectedVersion int64) (*domain.Subtask, error) {
			require.NotNil(t, ctx)
			assert.Equal(t, "test-subtask-id", id)
			assert.Equal(t, domain.TaskStatusRunning, status)
			assert.Equal(t, int64(1), expectedVersion)
			subtask := domain.NewSubtask(id, "parent-task-id", domain.SubtaskTypeCoding, "Code", "executor")
			subtask.Status = status
			return subtask, nil
		},
	}

	result, err := mock.UpdateStatus(context.Background(), "test-subtask-id", domain.TaskStatusRunning, 1)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, domain.TaskStatusRunning, result.Status)
}

// ----------------------------------------------------------------------------
// AuditLogRepository method signature tests
// ----------------------------------------------------------------------------

// TestAuditLogRepositoryCreate verifies the Create method signature.
func TestAuditLogRepositoryCreate(t *testing.T) {
	mock := &MockAuditLogRepository{
		CreateFunc: func(ctx context.Context, log *domain.AuditLog) (*domain.AuditLog, error) {
			require.NotNil(t, ctx)
			require.NotNil(t, log)
			assert.Equal(t, "test-log-id", log.ID)
			return log, nil
		},
	}

	log := domain.NewAuditLog("test-log-id", "task-123", "TaskCreated", "Task was created", "info")
	result, err := mock.Create(context.Background(), log)

	require.NoError(t, err)
	assert.Equal(t, "test-log-id", result.ID)
}

// TestAuditLogRepositoryListByTaskID verifies the ListByTaskID method signature.
func TestAuditLogRepositoryListByTaskID(t *testing.T) {
	mock := &MockAuditLogRepository{
		ListByTaskIDFunc: func(ctx context.Context, taskID string, limit, offset int) ([]*domain.AuditLog, int, error) {
			require.NotNil(t, ctx)
			assert.Equal(t, "task-123", taskID)
			assert.Equal(t, 20, limit)
			assert.Equal(t, 0, offset)
			logs := []*domain.AuditLog{
				domain.NewAuditLog("log-1", taskID, "TaskStarted", "Task started", "info"),
				domain.NewAuditLog("log-2", taskID, "TaskCompleted", "Task completed", "info"),
			}
			return logs, 2, nil
		},
	}

	logs, total, err := mock.ListByTaskID(context.Background(), "task-123", 20, 0)

	require.NoError(t, err)
	assert.Equal(t, 2, len(logs))
	assert.Equal(t, 2, total)
}

// TestAuditLogRepositoryListBySubtaskID verifies the ListBySubtaskID method signature.
func TestAuditLogRepositoryListBySubtaskID(t *testing.T) {
	mock := &MockAuditLogRepository{
		ListBySubtaskIDFunc: func(ctx context.Context, subtaskID string, limit, offset int) ([]*domain.AuditLog, int, error) {
			require.NotNil(t, ctx)
			assert.Equal(t, "subtask-456", subtaskID)
			assert.Equal(t, 20, limit)
			assert.Equal(t, 10, offset)
			logs := []*domain.AuditLog{
				domain.NewAuditLog("log-3", "task-123", "SubtaskStarted", "Subtask started", "info").
					WithSubtask(subtaskID),
			}
			return logs, 1, nil
		},
	}

	logs, total, err := mock.ListBySubtaskID(context.Background(), "subtask-456", 20, 10)

	require.NoError(t, err)
	assert.Equal(t, 1, len(logs))
	assert.Equal(t, 1, total)
}

// ----------------------------------------------------------------------------
// OutboxRepository method signature tests
// ----------------------------------------------------------------------------

// TestOutboxRepositoryWrite verifies the Write method signature.
func TestOutboxRepositoryWrite(t *testing.T) {
	mock := &MockOutboxRepository{
		WriteFunc: func(ctx context.Context, tx interface{}, event *domain.DomainEvent) error {
			require.NotNil(t, ctx)
			require.NotNil(t, event)
			assert.NotEmpty(t, event.EventID) // Event ID is auto-generated ULID
			assert.Equal(t, "Task", event.AggregateType)
			assert.Equal(t, "task-123", event.AggregateID)
			assert.Equal(t, "TaskCreatedV1", event.EventType)
			return nil
		},
	}

	event, err := domain.NewDomainEvent("Task", "task-123", "TaskCreatedV1", []byte(`{}`), 1)
	require.NoError(t, err)
	err = mock.Write(context.Background(), nil, event)

	require.NoError(t, err)
}

// TestOutboxRepositoryReadPending verifies the ReadPending method signature.
func TestOutboxRepositoryReadPending(t *testing.T) {
	mock := &MockOutboxRepository{
		ReadPendingFunc: func(ctx context.Context, limit int) ([]*domain.OutboxEvent, error) {
			require.NotNil(t, ctx)
			assert.Equal(t, 100, limit)
			events := []*domain.OutboxEvent{
				{ID: "event-1", EventType: "TaskCreatedV1"},
				{ID: "event-2", EventType: "SubtaskCreatedV1"},
			}
			return events, nil
		},
	}

	events, err := mock.ReadPending(context.Background(), 100)

	require.NoError(t, err)
	assert.Equal(t, 2, len(events))
}

// TestOutboxRepositoryMarkPublished verifies the MarkPublished method signature.
func TestOutboxRepositoryMarkPublished(t *testing.T) {
	mock := &MockOutboxRepository{
		MarkPublishedFunc: func(ctx context.Context, id string) error {
			require.NotNil(t, ctx)
			assert.Equal(t, "event-1", id)
			return nil
		},
	}

	err := mock.MarkPublished(context.Background(), "event-1")

	require.NoError(t, err)
}

// TestOutboxRepositoryMarkFailed verifies the MarkFailed method signature.
func TestOutboxRepositoryMarkFailed(t *testing.T) {
	mock := &MockOutboxRepository{
		MarkFailedFunc: func(ctx context.Context, id string, lastError string) error {
			require.NotNil(t, ctx)
			assert.Equal(t, "event-1", id)
			assert.Contains(t, lastError, "connection timeout")
			return nil
		},
	}

	err := mock.MarkFailed(context.Background(), "event-1", "connection timeout")

	require.NoError(t, err)
}

// TestOutboxRepositoryDeleteOldPublished verifies the DeleteOldPublished method signature.
func TestOutboxRepositoryDeleteOldPublished(t *testing.T) {
	mock := &MockOutboxRepository{
		DeleteOldPublishedFunc: func(ctx context.Context, olderThan int64) (int64, error) {
			require.NotNil(t, ctx)
			assert.Equal(t, int64(1700000000000000000), olderThan)
			return 42, nil
		},
	}

	deleted, err := mock.DeleteOldPublished(context.Background(), 1700000000000000000)

	require.NoError(t, err)
	assert.Equal(t, int64(42), deleted)
}

// ----------------------------------------------------------------------------
// ObjectStorage method signature tests
// ----------------------------------------------------------------------------

// TestObjectStorageUpload verifies the Upload method signature.
func TestObjectStorageUpload(t *testing.T) {
	mock := &MockObjectStorage{
		UploadFunc: func(ctx context.Context, key string, data []byte, contentType string) error {
			require.NotNil(t, ctx)
			assert.Equal(t, "artifacts/task-123/diff.patch", key)
			assert.Equal(t, []byte("patch content"), data)
			assert.Equal(t, "text/plain", contentType)
			return nil
		},
	}

	err := mock.Upload(context.Background(), "artifacts/task-123/diff.patch", []byte("patch content"), "text/plain")

	require.NoError(t, err)
}

// TestObjectStorageDownload verifies the Download method signature.
func TestObjectStorageDownload(t *testing.T) {
	mock := &MockObjectStorage{
		DownloadFunc: func(ctx context.Context, key string) ([]byte, error) {
			require.NotNil(t, ctx)
			assert.Equal(t, "artifacts/task-123/diff.patch", key)
			return []byte("patch content"), nil
		},
	}

	data, err := mock.Download(context.Background(), "artifacts/task-123/diff.patch")

	require.NoError(t, err)
	assert.Equal(t, []byte("patch content"), data)
}

// TestObjectStorageGeneratePresignedURL verifies the GeneratePresignedURL method signature.
func TestObjectStorageGeneratePresignedURL(t *testing.T) {
	mock := &MockObjectStorage{
		GeneratePresignedURLFunc: func(ctx context.Context, key string, expiry time.Duration) (string, error) {
			require.NotNil(t, ctx)
			assert.Equal(t, "artifacts/task-123/diff.patch", key)
			assert.Equal(t, 1*time.Hour, expiry)
			return "https://minio.example.com/bucket/artifacts/task-123/diff.patch?signature=xxx", nil
		},
	}

	url, err := mock.GeneratePresignedURL(context.Background(), "artifacts/task-123/diff.patch", 1*time.Hour)

	require.NoError(t, err)
	assert.Contains(t, url, "signature=xxx")
}

// TestObjectStorageBucketName verifies the BucketName method signature.
func TestObjectStorageBucketName(t *testing.T) {
	mock := &MockObjectStorage{
		BucketNameFunc: func() string {
			return "cap-artifacts"
		},
	}

	bucket := mock.BucketName()

	assert.Equal(t, "cap-artifacts", bucket)
}

// TestObjectStorageExists verifies the Exists method signature.
func TestObjectStorageExists(t *testing.T) {
	mock := &MockObjectStorage{
		ExistsFunc: func(ctx context.Context, key string) (bool, error) {
			require.NotNil(t, ctx)
			assert.Equal(t, "artifacts/task-123/diff.patch", key)
			return true, nil
		},
	}

	exists, err := mock.Exists(context.Background(), "artifacts/task-123/diff.patch")

	require.NoError(t, err)
	assert.True(t, exists)
}

// TestObjectStorageDelete verifies the Delete method signature.
func TestObjectStorageDelete(t *testing.T) {
	mock := &MockObjectStorage{
		DeleteFunc: func(ctx context.Context, key string) error {
			require.NotNil(t, ctx)
			assert.Equal(t, "artifacts/task-123/diff.patch", key)
			return nil
		},
	}

	err := mock.Delete(context.Background(), "artifacts/task-123/diff.patch")

	require.NoError(t, err)
}

// TestObjectStorageListExpiredObjects verifies the ListExpiredObjects method signature.
func TestObjectStorageListExpiredObjects(t *testing.T) {
	mock := &MockObjectStorage{
		ListExpiredObjectsFunc: func(ctx context.Context, olderThan time.Duration) ([]string, error) {
			require.NotNil(t, ctx)
			assert.Equal(t, 90*24*time.Hour, olderThan)
			return []string{
				"artifacts/task-001/old-diff.patch",
				"artifacts/task-002/old-report.txt",
			}, nil
		},
	}

	keys, err := mock.ListExpiredObjects(context.Background(), 90*24*time.Hour)

	require.NoError(t, err)
	assert.Equal(t, 2, len(keys))
}
