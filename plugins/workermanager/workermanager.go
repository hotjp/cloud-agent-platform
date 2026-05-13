// Package workermanager implements Worker lifecycle management.
// Supports both Docker and CubeSandbox backends.
package workermanager

// WorkerManager handles Worker lifecycle (create/destroy/health-check).
type WorkerManager struct {
	// TODO: Add worker pool and backend implementations
}

// New creates a new WorkerManager instance.
func New() *WorkerManager {
	return &WorkerManager{}
}
