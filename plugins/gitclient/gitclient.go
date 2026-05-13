// Package gitclient implements Git operations using go-git.
// Provides clone/commit/push without requiring git binary in Worker.
package gitclient

// GitClient handles Git operations.
type GitClient struct {
	// TODO: Add go-git client
}

// New creates a new GitClient instance.
func New() *GitClient {
	return &GitClient{}
}
