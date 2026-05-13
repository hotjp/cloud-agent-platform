// Package domain implements L2-Domain layer: domain entities, state machines,
// event collection (Outbox), and business invariants.
// This layer has ZERO external dependencies - pure Go structs + standard library.
package domain

import (
	"context"
	"time"
)

// ----------------------------------------------------------------------------
// Repository Interfaces
//
// These interfaces define the contract for data access.
// They are implemented by the L1-Storage layer (ent).
// Core layer defines interfaces; plugin layer implements.
// ----------------------------------------------------------------------------

// TaskRepository defines the interface for Task aggregate persistence.
// Implemented by L1-Storage (ent), used by L4-Service.
type TaskRepository interface {
	// Create creates a new Task and returns the created entity.
	Create(ctx context.Context, task *Task) (*Task, error)

	// GetByID retrieves a Task by its ID.
	// Returns ErrL2AggregateNotFound if the task does not exist.
	GetByID(ctx context.Context, id string) (*Task, error)

	// Update updates an existing Task and returns the updated entity.
	// Returns ErrL2AggregateNotFound if the task does not exist.
	// Returns ErrL2OptimisticLock if the version doesn't match.
	Update(ctx context.Context, task *Task) (*Task, error)

	// Delete deletes a Task by its ID.
	// Returns ErrL2AggregateNotFound if the task does not exist.
	Delete(ctx context.Context, id string) error

	// ListByStatus returns all Tasks with the given status.
	ListByStatus(ctx context.Context, status TaskStatus, limit, offset int) ([]*Task, int, error)

	// ListByClientID returns all Tasks for a given client.
	ListByClientID(ctx context.Context, clientID string, limit, offset int) ([]*Task, int, error)

	// List returns all Tasks with pagination.
	List(ctx context.Context, limit, offset int) ([]*Task, int, error)

	// UpdateStatus updates only the status and version of a Task.
	// This is optimized for state machine transitions.
	UpdateStatus(ctx context.Context, id string, status TaskStatus, expectedVersion int64) (*Task, error)

	// CountByStatus returns the count of Tasks with the given status.
	CountByStatus(ctx context.Context, status TaskStatus) (int, error)
}

// SubtaskRepository defines the interface for Subtask aggregate persistence.
// Implemented by L1-Storage (ent), used by L4-Service.
type SubtaskRepository interface {
	// Create creates a new Subtask and returns the created entity.
	Create(ctx context.Context, subtask *Subtask) (*Subtask, error)

	// GetByID retrieves a Subtask by its ID.
	// Returns ErrL2AggregateNotFound if the subtask does not exist.
	GetByID(ctx context.Context, id string) (*Subtask, error)

	// Update updates an existing Subtask and returns the updated entity.
	// Returns ErrL2AggregateNotFound if the subtask does not exist.
	// Returns ErrL2OptimisticLock if the version doesn't match.
	Update(ctx context.Context, subtask *Subtask) (*Subtask, error)

	// Delete deletes a Subtask by its ID.
	// Returns ErrL2AggregateNotFound if the subtask does not exist.
	Delete(ctx context.Context, id string) error

	// ListByTaskID returns all Subtasks for a given Task.
	ListByTaskID(ctx context.Context, taskID string) ([]*Subtask, error)

	// UpdateStatus updates only the status and version of a Subtask.
	// This is optimized for state machine transitions.
	UpdateStatus(ctx context.Context, id string, status TaskStatus, expectedVersion int64) (*Subtask, error)

	// ListByStatus returns all Subtasks with the given status.
	ListByStatus(ctx context.Context, status TaskStatus, limit, offset int) ([]*Subtask, int, error)
}

// AuditLogRepository defines the interface for audit log persistence.
// Implemented by L1-Storage (ent), used by L4-Service.
type AuditLogRepository interface {
	// Create creates a new AuditLog entry and returns the created entity.
	Create(ctx context.Context, log *AuditLog) (*AuditLog, error)

	// ListByTaskID returns all AuditLogs for a given Task.
	ListByTaskID(ctx context.Context, taskID string, limit, offset int) ([]*AuditLog, int, error)

	// ListBySubtaskID returns all AuditLogs for a given Subtask.
	ListBySubtaskID(ctx context.Context, subtaskID string, limit, offset int) ([]*AuditLog, int, error)
}

// OutboxRepository defines the interface for outbox event persistence.
// This is separate from OutboxWriter (which writes within a transaction).
// Implemented by L1-Storage (ent).
type OutboxRepository interface {
	// Write writes a domain event to the outbox table within the given transaction.
	// This is typically called by the OutboxWriter implementation.
	Write(ctx context.Context, tx interface{}, event *DomainEvent) error

	// ReadPending reads pending (unpublished) events up to the given limit.
	// Returns events sorted by created_at ascending.
	// Implements the relay process reading side.
	ReadPending(ctx context.Context, limit int) ([]*OutboxEvent, error)

	// MarkPublished marks an event as published by setting its status.
	MarkPublished(ctx context.Context, id string) error

	// MarkFailed marks an event as failed and increments retry_count.
	MarkFailed(ctx context.Context, id string, lastError string) error

	// DeleteOldPublished deletes published events older than the given cutoff time.
	// This is for cleanup operations.
	DeleteOldPublished(ctx context.Context, olderThan int64) (int64, error)
}

// OutboxEvent represents an event in the outbox table.
type OutboxEvent struct {
	ID            string
	AggregateType string
	AggregateID   string
	EventType     string
	Payload       []byte
	OccurredAt    int64
	Status        string // "pending", "published", "failed"
	RetryCount    int
	LastError     string
	CreatedAt     int64
}

// ObjectStorage defines the interface for cold object storage (MinIO/S3-compatible).
// Implemented by L1-Storage (infra), used by L4-Service.
type ObjectStorage interface {
	// Upload uploads an object to cold storage and returns the object key.
	// The content is stored with 90-day TTL.
	Upload(ctx context.Context, key string, data []byte, contentType string) error

	// Download downloads an object from cold storage by key.
	// Returns ErrL1ObjectNotFound if the object does not exist.
	Download(ctx context.Context, key string) ([]byte, error)

	// GeneratePresignedURL generates a presigned URL for direct object access.
	// The URL expires after 1 hour.
	// Returns ErrL1ObjectNotFound if the object does not exist.
	GeneratePresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error)

	// Delete deletes an object from cold storage.
	Delete(ctx context.Context, key string) error

	// ListExpiredObjects returns keys of objects older than the given TTL.
	// Used by the cleanup worker to identify expired objects.
	ListExpiredObjects(ctx context.Context, olderThan time.Duration) ([]string, error)

	// Exists checks if an object exists in cold storage.
	Exists(ctx context.Context, key string) (bool, error)

	// BucketName returns the configured bucket name.
	BucketName() string
}

// Verify interface implementations at compile time.
var (
	_ TaskRepository     = (TaskRepository)(nil)
	_ SubtaskRepository  = (SubtaskRepository)(nil)
	_ AuditLogRepository = (AuditLogRepository)(nil)
	_ OutboxRepository   = (OutboxRepository)(nil)
	_ ObjectStorage      = (ObjectStorage)(nil)
)