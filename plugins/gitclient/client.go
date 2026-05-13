// Package gitclient implements Git operations using go-git.
// Provides clone/commit/push without requiring git binary in Worker.
package gitclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"go.uber.org/zap"

	"github.com/cloud-agent-platform/cap/internal/domain"
)

// ProtectedBranches contains branches that are not allowed to be pushed to.
var ProtectedBranches = []string{"main", "master"}

// ErrPushToProtectedBranch is returned when attempting to push to a protected branch.
var ErrPushToProtectedBranch = errors.New("push to protected branch is forbidden")

// ErrForcePushDisabled is returned when force push is attempted.
var ErrForcePushDisabled = errors.New("force push is disabled")

// ErrNoRepository is returned when no repository has been opened.
var ErrNoRepository = errors.New("no repository opened")

// ErrInvalidURL is returned when the repository URL is invalid.
var ErrInvalidURL = errors.New("invalid repository URL")

// GitClient handles Git operations using go-git.
type GitClient struct {
	repo   *git.Repository
	auth   transport.AuthMethod
	logger *zap.Logger
}

// CloneOptions contains options for Clone operation.
type CloneOptions struct {
	// URL is the repository URL to clone.
	URL string
	// Directory is the target directory for clone.
	Directory string
	// Branch is the branch to checkout (optional, defaults to default branch).
	Branch string
	// Auth is optional authentication (nil for public repos).
	Auth *AuthMethod
}

// AuthMethod represents Git authentication credentials.
type AuthMethod struct {
	// Username for basic auth or token auth.
	Username string
	// Password for basic auth or token auth (can be personal access token).
	Password string
	// SSHKeyPath is the path to SSH private key (alternative to password auth).
	SSHKeyPath string
}

// toAuth converts AuthMethod to go-git transport.AuthMethod.
func (a *AuthMethod) toAuth() transport.AuthMethod {
	if a == nil {
		return nil
	}
	if a.Username != "" || a.Password != "" {
		return &githttp.BasicAuth{
			Username: a.Username,
			Password: a.Password,
		}
	}
	return nil
}

// Clone clones a repository to the specified directory.
func (c *GitClient) Clone(ctx context.Context, opts CloneOptions) error {
	c.logger.Info("cloning repository",
		zap.String("url", opts.URL),
		zap.String("directory", opts.Directory),
		zap.String("branch", opts.Branch),
	)

	if opts.URL == "" {
		return domain.NewL2InvalidInputError("URL", "is required")
	}

	// Clean and validate directory path
	if opts.Directory == "" {
		return domain.NewL2InvalidInputError("directory", "is required")
	}

	// Determine branch to checkout
	branch := plumbing.HEAD
	if opts.Branch != "" {
		branch = plumbing.NewBranchReferenceName(opts.Branch)
	}

	// Clone options
	cloneOpts := &git.CloneOptions{
		URL:          opts.URL,
		Depth:        0, // Full clone
		ReferenceName: branch,
		SingleBranch: false,
	}

	// Set up authentication if provided
	if opts.Auth != nil {
		if authMethod := opts.Auth.toAuth(); authMethod != nil {
			cloneOpts.Auth = authMethod
		}
	}

	// Perform clone
	repo, err := git.PlainCloneContext(ctx, opts.Directory, false, cloneOpts)
	if err != nil {
		c.logger.Error("failed to clone repository",
			zap.String("url", opts.URL),
			zap.Error(err),
		)
		return domain.NewL4GitOpError("clone", opts.URL, err)
	}

	c.repo = repo
	c.logger.Info("repository cloned successfully",
		zap.String("directory", opts.Directory),
	)
	return nil
}

// Open opens an existing local repository.
func (c *GitClient) Open(ctx context.Context, dir string) error {
	c.logger.Info("opening repository",
		zap.String("directory", dir),
	)

	if dir == "" {
		return domain.NewL2InvalidInputError("directory", "is required")
	}

	repo, err := git.PlainOpen(dir)
	if err != nil {
		c.logger.Error("failed to open repository",
			zap.String("directory", dir),
			zap.Error(err),
		)
		return domain.NewL4GitOpError("open", dir, err)
	}

	c.repo = repo
	return nil
}

// Commit commits changes with the given message.
// Returns the commit hash on success.
func (c *GitClient) Commit(ctx context.Context, message string) (string, error) {
	c.logger.Info("creating commit",
		zap.String("message", message),
	)

	if c.repo == nil {
		return "", domain.NewL2InvalidStateError("GitClient", "no repository", ErrNoRepository.Error())
	}

	if message == "" {
		return "", domain.NewL2InvalidInputError("message", "is required")
	}

	// Get the working tree
	workTree, err := c.repo.Worktree()
	if err != nil {
		c.logger.Error("failed to get worktree", zap.Error(err))
		return "", domain.NewL4GitOpError("worktree", "", err)
	}

	// Get status to see what changed
	status, err := workTree.Status()
	if err != nil {
		c.logger.Error("failed to get status", zap.Error(err))
		return "", domain.NewL4GitOpError("status", "", err)
	}

	// Check if there are changes to commit
	if status.IsClean() {
		c.logger.Info("nothing to commit")
		return "", nil
	}

	// Add all changed files
	for path := range status {
		_, err := workTree.Add(path)
		if err != nil {
			c.logger.Error("failed to add file",
				zap.String("path", path),
				zap.Error(err),
			)
			return "", domain.NewL4GitOpError("add", path, err)
		}
	}

	// Create commit
	commit, err := workTree.Commit(message, &git.CommitOptions{})
	if err != nil {
		c.logger.Error("failed to create commit", zap.Error(err))
		return "", domain.NewL4GitOpError("commit", "", err)
	}

	commitHash := commit.String()
	c.logger.Info("commit created successfully",
		zap.String("hash", commitHash),
	)
	return commitHash, nil
}

// Push pushes changes to the remote repository.
// Security: prevents push to protected branches (main/master) and force push.
func (c *GitClient) Push(ctx context.Context) error {
	c.logger.Info("pushing changes")

	if c.repo == nil {
		return domain.NewL2InvalidStateError("GitClient", "no repository", ErrNoRepository.Error())
	}

	// Get current branch to check if it's protected
	head, err := c.repo.Head()
	if err != nil {
		c.logger.Error("failed to get HEAD", zap.Error(err))
		return domain.NewL4GitOpError("head", "", err)
	}

	branchName := head.Name().Short()
	c.logger.Info("current branch",
		zap.String("branch", branchName),
	)

	// Check if pushing to a protected branch
	if isProtectedBranch(branchName) {
		c.logger.Warn("push to protected branch blocked",
			zap.String("branch", branchName),
		)
		return domain.NewL2InvalidOperationError("push", "cannot push to protected branch "+branchName)
	}

	// Push options - force push is disabled by default
	pushOpts := &git.PushOptions{
		RemoteName: "origin",
		// Force: false - force push is disabled by default
	}

	// Set up authentication if available
	if c.auth != nil {
		pushOpts.Auth = c.auth
	}

	// Perform push
	err = c.repo.PushContext(ctx, pushOpts)
	if err != nil {
		c.logger.Error("failed to push",
			zap.String("branch", branchName),
			zap.Error(err),
		)
		return domain.NewL4GitOpError("push", branchName, err)
	}

	c.logger.Info("push successful",
		zap.String("branch", branchName),
	)
	return nil
}

// PushBranch pushes a specific branch to the remote repository.
// Security: prevents push to protected branches (main/master) and force push.
func (c *GitClient) PushBranch(ctx context.Context, branchName string) error {
	c.logger.Info("pushing branch",
		zap.String("branch", branchName),
	)

	if c.repo == nil {
		return domain.NewL2InvalidStateError("GitClient", "no repository", ErrNoRepository.Error())
	}

	// Check if pushing to a protected branch
	if isProtectedBranch(branchName) {
		c.logger.Warn("push to protected branch blocked",
			zap.String("branch", branchName),
		)
		return domain.NewL2InvalidOperationError("push", "cannot push to protected branch "+branchName)
	}

	// Push options - force push is disabled by default
	pushOpts := &git.PushOptions{
		RemoteName: "origin",
		RefSpecs: []config.RefSpec{
			config.RefSpec("refs/heads/" + branchName + ":refs/heads/" + branchName),
		},
		// Force: false - force push is disabled by default
	}

	// Set up authentication if available
	if c.auth != nil {
		pushOpts.Auth = c.auth
	}

	// Perform push
	err := c.repo.PushContext(ctx, pushOpts)
	if err != nil {
		c.logger.Error("failed to push branch",
			zap.String("branch", branchName),
			zap.Error(err),
		)
		return domain.NewL4GitOpError("push", branchName, err)
	}

	c.logger.Info("branch push successful",
		zap.String("branch", branchName),
	)
	return nil
}

// SetAuth sets authentication for subsequent operations.
func (c *GitClient) SetAuth(auth *AuthMethod) {
	c.auth = auth.toAuth()
}

// GetHead returns the current HEAD reference.
func (c *GitClient) GetHead() (*plumbing.Reference, error) {
	if c.repo == nil {
		return nil, ErrNoRepository
	}
	return c.repo.Head()
}

// GetCurrentBranch returns the current branch name.
func (c *GitClient) GetCurrentBranch() (string, error) {
	head, err := c.GetHead()
	if err != nil {
		return "", err
	}
	return head.Name().Short(), nil
}

// isProtectedBranch checks if the branch name is protected.
func isProtectedBranch(branch string) bool {
	branch = strings.ToLower(branch)
	for _, protected := range ProtectedBranches {
		if branch == strings.ToLower(protected) {
			return true
		}
	}
	return false
}

// New creates a new GitClient instance.
func New(logger *zap.Logger) *GitClient {
	return &GitClient{
		logger: logger,
	}
}

// NewWithAuth creates a new GitClient with authentication.
func NewWithAuth(logger *zap.Logger, auth *AuthMethod) *GitClient {
	return &GitClient{
		logger: logger,
		auth:   auth.toAuth(),
	}
}

// CloneToTemp clones a repository to a temporary directory and returns the path.
// The caller is responsible for cleaning up the directory.
func CloneToTemp(ctx context.Context, url string, auth *AuthMethod, logger *zap.Logger) (string, *GitClient, error) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "git-clone-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp dir: %w", err)
	}

	client := New(logger)
	if auth != nil {
		client.SetAuth(auth)
	}

	err = client.Clone(ctx, CloneOptions{
		URL:      url,
		Directory: tempDir,
	})
	if err != nil {
		// Clean up on failure
		os.RemoveAll(tempDir)
		return "", nil, err
	}

	return tempDir, client, nil
}

// CreateBranch creates a new branch from the current HEAD.
func (c *GitClient) CreateBranch(ctx context.Context, name string) error {
	if c.repo == nil {
		return domain.NewL2InvalidStateError("GitClient", "no repository", ErrNoRepository.Error())
	}

	workTree, err := c.repo.Worktree()
	if err != nil {
		return domain.NewL4GitOpError("worktree", "", err)
	}

	err = workTree.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(name),
		Create: true,
	})
	if err != nil {
		return domain.NewL4GitOpError("create-branch", name, err)
	}

	c.logger.Info("branch created",
		zap.String("branch", name),
	)
	return nil
}

// CheckoutBranch checks out an existing branch.
func (c *GitClient) CheckoutBranch(ctx context.Context, name string) error {
	if c.repo == nil {
		return domain.NewL2InvalidStateError("GitClient", "no repository", ErrNoRepository.Error())
	}

	workTree, err := c.repo.Worktree()
	if err != nil {
		return domain.NewL4GitOpError("worktree", "", err)
	}

	err = workTree.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(name),
	})
	if err != nil {
		return domain.NewL4GitOpError("checkout-branch", name, err)
	}

	c.logger.Info("checked out branch",
		zap.String("branch", name),
	)
	return nil
}

// GetWorktree returns the repository worktree.
func (c *GitClient) GetWorktree() (*git.Worktree, error) {
	if c.repo == nil {
		return nil, ErrNoRepository
	}
	return c.repo.Worktree()
}

// GetRepository returns the underlying git.Repository.
func (c *GitClient) GetRepository() *git.Repository {
	return c.repo
}

// RepositoryPath returns the filesystem path of the repository.
func (c *GitClient) RepositoryPath() (string, error) {
	if c.repo == nil {
		return "", ErrNoRepository
	}
	// Get the working tree's filesystem path
	wt, err := c.repo.Worktree()
	if err != nil {
		return "", err
	}
	return wt.Filesystem.Root(), nil
}

// WriteFile writes a file to the working tree.
func (c *GitClient) WriteFile(path string, content []byte) error {
	wt, err := c.GetWorktree()
	if err != nil {
		return err
	}

	fullPath := filepath.Join(wt.Filesystem.Root(), path)
	return os.WriteFile(fullPath, content, 0644)
}

// FileStatus represents the status of a single file in the repository.
type FileStatus struct {
	Path      string `json:"path"`
	Staging   string `json:"staging,omitempty"`   // staged status: added, modified, deleted, etc.
	Worktree  string `json:"worktree,omitempty"`  // worktree status: modified, deleted, untracked, etc.
	IsNew     bool   `json:"is_new"`
	IsModified bool  `json:"is_modified"`
	IsDeleted  bool   `json:"is_deleted"`
	IsRenamed  bool   `json:"is_renamed"`
	IsCopied   bool   `json:"is_copied"`
	IsUntracked bool  `json:"is_untracked"`
}

// RepositoryStatus holds the categorized status of all files in the repository.
type RepositoryStatus struct {
	Branch    string                 `json:"branch"`
	IsClean   bool                   `json:"is_clean"`
	Staged    []FileStatus           `json:"staged,omitempty"`    // files in staging area
	Modified  []FileStatus           `json:"modified,omitempty"`  // modified files in worktree
	Untracked []FileStatus           `json:"untracked,omitempty"`  // untracked files
	All       map[string]FileStatus `json:"all,omitempty"`        // all files by path
}

// Status returns the categorized status of the repository.
func (c *GitClient) Status(ctx context.Context) (*RepositoryStatus, error) {
	if c.repo == nil {
		return nil, domain.NewL2InvalidStateError("GitClient", "no repository", ErrNoRepository.Error())
	}

	workTree, err := c.repo.Worktree()
	if err != nil {
		return nil, domain.NewL4GitOpError("worktree", "", err)
	}

	status, err := workTree.Status()
	if err != nil {
		return nil, domain.NewL4GitOpError("status", "", err)
	}

	branch, _ := c.GetCurrentBranch()

	result := &RepositoryStatus{
		Branch: branch,
		All:    make(map[string]FileStatus),
	}

	for path, fileStatus := range status {
		fs := FileStatus{Path: path}

		// Determine staging status
		switch fileStatus.Staging {
		case git.Added:
			fs.Staging = "added"
			fs.IsNew = true
		case git.Modified:
			fs.Staging = "modified"
			fs.IsModified = true
		case git.Deleted:
			fs.Staging = "deleted"
			fs.IsDeleted = true
		case git.Renamed:
			fs.Staging = "renamed"
			fs.IsRenamed = true
		case git.Copied:
			fs.Staging = "copied"
			fs.IsCopied = true
		case git.Untracked:
			fs.Staging = "untracked"
			fs.IsUntracked = true
		}

		// Determine worktree status
		switch fileStatus.Worktree {
		case git.Added:
			fs.Worktree = "added"
			fs.IsNew = true
		case git.Modified:
			fs.Worktree = "modified"
			fs.IsModified = true
		case git.Deleted:
			fs.Worktree = "deleted"
			fs.IsDeleted = true
		case git.Renamed:
			fs.Worktree = "renamed"
			fs.IsRenamed = true
		case git.Copied:
			fs.Worktree = "copied"
			fs.IsCopied = true
		case git.Untracked:
			fs.Worktree = "untracked"
			fs.IsUntracked = true
		}

		result.All[path] = fs

		// Categorize: staged files (anything in staging area that's not clean)
		if fileStatus.Staging != git.Unmodified {
			result.Staged = append(result.Staged, fs)
		}

		// Untracked files (only in worktree, not staged)
		if fileStatus.Staging == git.Untracked {
			result.Untracked = append(result.Untracked, fs)
		} else if fileStatus.Staging == git.Unmodified && fileStatus.Worktree != git.Unmodified && fileStatus.Worktree != 0 {
			// Modified in worktree but not staged
			result.Modified = append(result.Modified, fs)
		}
	}

	result.IsClean = status.IsClean()

	c.logger.Debug("repository status",
		zap.String("branch", branch),
		zap.Bool("is_clean", result.IsClean),
		zap.Int("staged", len(result.Staged)),
		zap.Int("modified", len(result.Modified)),
		zap.Int("untracked", len(result.Untracked)),
	)

	return result, nil
}

// DiffStaged returns the diff of staged changes (files in the index).
func (c *GitClient) DiffStaged(ctx context.Context) (string, error) {
	if c.repo == nil {
		return "", domain.NewL2InvalidStateError("GitClient", "no repository", ErrNoRepository.Error())
	}

	repo := c.repo

	// Get HEAD commit for comparison
	head, err := repo.Head()
	if err != nil {
		return "", domain.NewL4GitOpError("head", "", err)
	}

	headCommit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return "", domain.NewL4GitOpError("commit-object", "", err)
	}

	headTree, err := headCommit.Tree()
	if err != nil {
		return "", domain.NewL4GitOpError("tree", "", err)
	}

	// Get index
	index, err := repo.Storer.Index()
	if err != nil {
		return "", domain.NewL4GitOpError("index", "", err)
	}

	// Get worktree for reading file contents
	workTree, err := repo.Worktree()
	if err != nil {
		return "", domain.NewL4GitOpError("worktree", "", err)
	}

	worktreeRoot := workTree.Filesystem.Root()

	var buf bytes.Buffer

	// Iterate through index entries and compare with HEAD tree
	for _, entry := range index.Entries {
		// Get the entry in HEAD tree for this path
		headEntry, err := headTree.FindEntry(entry.Name)
		if err == nil && headEntry.Hash == entry.Hash {
			// Hash matches - no staged change for this file
			continue
		}

		// File differs - generate diff
		path := entry.Name

		// Read the staged content from worktree (after git add)
		stagedContent, err := os.ReadFile(filepath.Join(worktreeRoot, path))
		if err != nil {
			// If we can't read the file (e.g., deleted), skip
			continue
		}

		// Get HEAD content if exists
		var headContent string
		if headEntry != nil {
			headFile, err := headCommit.File(path)
			if err == nil {
				headContent, _ = headFile.Contents()
			}
		}

		// Generate unified diff format
		buf.WriteString(path + ":\n")
		generateUnifiedDiff(&buf, headContent, string(stagedContent), path)
		buf.WriteString("\n")
	}

	result := buf.String()
	if result == "" {
		return "no staged changes", nil
	}
	return result, nil
}

// generateUnifiedDiff generates a unified diff string.
func generateUnifiedDiff(buf *bytes.Buffer, oldContent, newContent, path string) {
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	// Simple line-by-line diff
	maxLen := len(oldLines)
	if len(newLines) > maxLen {
		maxLen = len(newLines)
	}

	for i := 0; i < maxLen; i++ {
		var oldLine, newLine string
		if i < len(oldLines) {
			oldLine = oldLines[i]
		}
		if i < len(newLines) {
			newLine = newLines[i]
		}

		if oldLine == newLine {
			buf.WriteString(" " + oldLine + "\n")
		} else {
			if oldLine != "" || (i < len(oldLines) && oldLine == "") {
				buf.WriteString("-" + oldLine + "\n")
			}
			if newLine != "" || (i < len(newLines) && newLine == "") {
				buf.WriteString("+" + newLine + "\n")
			}
		}
	}
}
