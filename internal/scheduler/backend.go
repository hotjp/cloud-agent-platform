// Package scheduler - Backend interface bridges scheduler to actual container runtimes.
package scheduler

import "context"

// Backend is the abstraction over container runtimes (Docker, CubeSandbox, etc.).
// The scheduler owns the lifecycle policy; the backend owns the runtime operations.
type Backend interface {
	// Create creates a new container and returns its ID.
	Create(ctx context.Context, spec ContainerSpec) (string, error)

	// Exec runs a command in an existing container.
	Exec(ctx context.Context, containerID string, spec ExecSpec) (ExecResult, error)

	// Destroy stops and removes a container.
	Destroy(ctx context.Context, containerID string) error
}
