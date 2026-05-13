// Package tools implements the tool set available to agents.
package tools

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloud-agent-platform/cap/internal/domain"
	"github.com/cloud-agent-platform/cap/plugins/gitclient"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/diff"
	"go.uber.org/zap"
)

// ----------------------------------------------------------------------------
// GitClone Tool
// ----------------------------------------------------------------------------

// GitClone clones a git repository.
type GitClone struct {
	toolBase
	workDir string
	auth    *gitclient.AuthMethod
	logger  *zap.Logger
}

const gitCloneSchema = `{
	"type": "object",
	"properties": {
		"url": {
			"type": "string",
			"description": "The URL of the git repository to clone"
		},
		"path": {
			"type": "string",
			"description": "The directory path to clone into (relative to work directory, default: 'cloned_repo')"
		},
		"branch": {
			"type": "string",
			"description": "The branch to checkout (optional, defaults to default branch)"
		}
	},
	"required": ["url"]
}`

// NewGitClone creates a new GitClone tool.
func NewGitClone(workDir string, auth *gitclient.AuthMethod, logger *zap.Logger) *GitClone {
	if logger == nil {
		logger, _ = zap.NewProduction()
	}
	return &GitClone{
		toolBase: toolBase{
			name:        "GitClone",
			description: "Clones a git repository to a local directory",
			inputSchema: gitCloneSchema,
		},
		workDir: workDir,
		auth:    auth,
		logger:  logger,
	}
}

// Execute clones the repository.
func (t *GitClone) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	start := time.Now()
	url, ok := input["url"].(string)
	if !ok || url == "" {
		return &ToolResult{Success: false, Error: "url is required and must be a string"}, nil
	}

	path := "cloned_repo"
	if p, ok := input["path"].(string); ok && p != "" {
		path = p
	}

	branch := ""
	if b, ok := input["branch"].(string); ok {
		branch = b
	}

	// Resolve target directory
	targetDir := path
	if t.workDir != "" {
		targetDir = filepath.Join(t.workDir, path)
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(targetDir), 0755); err != nil && !errors.Is(err, os.ErrExist) {
		return &ToolResult{
			Success: false,
			Error:   domain.NewL2InvalidOperationError("clone", err.Error()).Error(),
			Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
		}, nil
	}

	client := gitclient.New(t.logger)
	if t.auth != nil {
		client.SetAuth(t.auth)
	}

	err := client.Clone(ctx, gitclient.CloneOptions{
		URL:       url,
		Directory: targetDir,
		Branch:    branch,
	})
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   domain.NewL4GitOpError("clone", url, err).Error(),
			Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
		}, nil
	}

	return &ToolResult{
		Success: true,
		Output: map[string]any{
			"url":      url,
			"directory": targetDir,
			"branch":    branch,
		},
		Meta: ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
	}, nil
}

// ----------------------------------------------------------------------------
// GitCommit Tool
// ----------------------------------------------------------------------------

// GitCommit commits changes in a git repository.
type GitCommit struct {
	toolBase
	workDir string
	logger  *zap.Logger
}

const gitCommitSchema = `{
	"type": "object",
	"properties": {
		"path": {
			"type": "string",
			"description": "The path to the git repository (relative to work directory)"
		},
		"message": {
			"type": "string",
			"description": "The commit message"
		}
	},
	"required": ["path", "message"]
}`

// NewGitCommit creates a new GitCommit tool.
func NewGitCommit(workDir string, logger *zap.Logger) *GitCommit {
	if logger == nil {
		logger, _ = zap.NewProduction()
	}
	return &GitCommit{
		toolBase: toolBase{
			name:        "GitCommit",
			description: "Commits changes to a git repository",
			inputSchema: gitCommitSchema,
		},
		workDir: workDir,
		logger:  logger,
	}
}

// Execute commits the changes.
func (t *GitCommit) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	start := time.Now()
	path, ok := input["path"].(string)
	if !ok || path == "" {
		return &ToolResult{Success: false, Error: "path is required and must be a string"}, nil
	}

	message, ok := input["message"].(string)
	if !ok || message == "" {
		return &ToolResult{Success: false, Error: "message is required and must be a string"}, nil
	}

	// Resolve path safely
	repoPath, err := t.resolvePath(path)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, nil
	}

	client := gitclient.New(t.logger)
	if err := client.Open(ctx, repoPath); err != nil {
		return &ToolResult{
			Success: false,
			Error:   domain.NewL4GitOpError("open", repoPath, err).Error(),
			Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
		}, nil
	}

	commitHash, err := client.Commit(ctx, message)
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   domain.NewL4GitOpError("commit", repoPath, err).Error(),
			Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
		}, nil
	}

	if commitHash == "" {
		return &ToolResult{
			Success: true,
			Output:  map[string]any{"message": "nothing to commit"},
			Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
		}, nil
	}

	return &ToolResult{
		Success: true,
		Output:  map[string]any{"commit_hash": commitHash, "message": message},
		Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
	}, nil
}

func (t *GitCommit) resolvePath(path string) (string, error) {
	if t.workDir == "" {
		return filepath.Abs(path)
	}
	fullPath := filepath.Join(t.workDir, path)
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", errors.New("invalid path")
	}
	if !strings.HasPrefix(absPath, t.workDir) {
		return "", errors.New("access denied: path outside work directory")
	}
	return absPath, nil
}

// ----------------------------------------------------------------------------
// GitPush Tool
// ----------------------------------------------------------------------------

// GitPush pushes changes to a remote repository.
type GitPush struct {
	toolBase
	workDir string
	auth    *gitclient.AuthMethod
	logger  *zap.Logger
}

const gitPushSchema = `{
	"type": "object",
	"properties": {
		"path": {
			"type": "string",
			"description": "The path to the git repository (relative to work directory)"
		},
		"branch": {
			"type": "string",
			"description": "The branch to push to (optional, defaults to current branch)"
		}
	},
	"required": ["path"]
}`

// NewGitPush creates a new GitPush tool.
func NewGitPush(workDir string, auth *gitclient.AuthMethod, logger *zap.Logger) *GitPush {
	if logger == nil {
		logger, _ = zap.NewProduction()
	}
	return &GitPush{
		toolBase: toolBase{
			name:        "GitPush",
			description: "Pushes committed changes to a remote git repository",
			inputSchema: gitPushSchema,
		},
		workDir: workDir,
		auth:    auth,
		logger:  logger,
	}
}

// Execute pushes to the remote.
func (t *GitPush) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	start := time.Now()
	path, ok := input["path"].(string)
	if !ok || path == "" {
		return &ToolResult{Success: false, Error: "path is required and must be a string"}, nil
	}

	branchArg := ""
	if b, ok := input["branch"].(string); ok && b != "" {
		branchArg = b
	}

	// Resolve path safely
	repoPath, err := t.resolvePath(path)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, nil
	}

	client := gitclient.New(t.logger)
	if t.auth != nil {
		client.SetAuth(t.auth)
	}

	if err := client.Open(ctx, repoPath); err != nil {
		return &ToolResult{
			Success: false,
			Error:   domain.NewL4GitOpError("open", repoPath, err).Error(),
			Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
		}, nil
	}

	var branch string
	if branchArg != "" {
		branch = branchArg
		if err := client.PushBranch(ctx, branchArg); err != nil {
			return &ToolResult{
				Success: false,
				Error:   domain.NewL4GitOpError("push", repoPath, err).Error(),
				Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
			}, nil
		}
	} else {
		branch, _ = client.GetCurrentBranch()
		if err := client.Push(ctx); err != nil {
			return &ToolResult{
				Success: false,
				Error:   domain.NewL4GitOpError("push", repoPath, err).Error(),
				Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
			}, nil
		}
	}

	return &ToolResult{
		Success: true,
		Output:  map[string]any{"branch": branch, "message": "pushed successfully"},
		Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
	}, nil
}

func (t *GitPush) resolvePath(path string) (string, error) {
	if t.workDir == "" {
		return filepath.Abs(path)
	}
	fullPath := filepath.Join(t.workDir, path)
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", errors.New("invalid path")
	}
	if !strings.HasPrefix(absPath, t.workDir) {
		return "", errors.New("access denied: path outside work directory")
	}
	return absPath, nil
}

// ----------------------------------------------------------------------------
// GitDiff Tool
// ----------------------------------------------------------------------------

// GitDiff shows changes between commits, branches, or working tree.
type GitDiff struct {
	toolBase
	workDir string
	logger  *zap.Logger
}

const gitDiffSchema = `{
	"type": "object",
	"properties": {
		"path": {
			"type": "string",
			"description": "The path to the git repository (relative to work directory)"
		},
		"commit": {
			"type": "string",
			"description": "Show changes from this commit (optional, shows working tree changes if not specified)"
		},
		"to_commit": {
			"type": "string",
			"description": "Compare from commit to this commit (optional)"
		},
		"staged": {
			"type": "boolean",
			"description": "Show staged diff (changes in the index vs HEAD). Default false."
		}
	},
	"required": ["path"]
}`

// NewGitDiff creates a new GitDiff tool.
func NewGitDiff(workDir string, logger *zap.Logger) *GitDiff {
	if logger == nil {
		logger, _ = zap.NewProduction()
	}
	return &GitDiff{
		toolBase: toolBase{
			name:        "GitDiff",
			description: "Shows changes between commits, branches, or the working tree",
			inputSchema: gitDiffSchema,
		},
		workDir: workDir,
		logger:  logger,
	}
}

// Execute shows the diff.
func (t *GitDiff) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	start := time.Now()
	path, ok := input["path"].(string)
	if !ok || path == "" {
		return &ToolResult{Success: false, Error: "path is required and must be a string"}, nil
	}

	commit := ""
	if c, ok := input["commit"].(string); ok {
		commit = c
	}

	toCommit := ""
	if tc, ok := input["to_commit"].(string); ok {
		toCommit = tc
	}

	staged := false
	if s, ok := input["staged"].(bool); ok {
		staged = s
	}

	// Resolve path safely
	repoPath, err := t.resolvePath(path)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, nil
	}

	client := gitclient.New(t.logger)
	if err := client.Open(ctx, repoPath); err != nil {
		return &ToolResult{
			Success: false,
			Error:   domain.NewL4GitOpError("open", repoPath, err).Error(),
			Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
		}, nil
	}

	var diffOutput string
	if staged {
		diffOutput, err = t.diffStaged(client)
	} else if commit != "" && toCommit != "" {
		diffOutput, err = t.diffCommits(client, commit, toCommit)
	} else if commit != "" {
		diffOutput, err = t.diffCommit(client, commit)
	} else {
		diffOutput, err = t.diffWorktree(client)
	}

	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   domain.NewL4GitOpError("diff", repoPath, err).Error(),
			Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
		}, nil
	}

	return &ToolResult{
		Success: true,
		Output:  map[string]any{"diff": diffOutput},
		Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
	}, nil
}

func (t *GitDiff) diffCommits(client *gitclient.GitClient, from, to string) (string, error) {
	fromHash := plumbing.NewHash(from)
	toHash := plumbing.NewHash(to)

	fromCommit, err := client.GetRepository().CommitObject(fromHash)
	if err != nil {
		return "", err
	}

	toCommit, err := client.GetRepository().CommitObject(toHash)
	if err != nil {
		return "", err
	}

	// Get patch
	fromTree, err := fromCommit.Tree()
	if err != nil {
		return "", err
	}
	toTree, err := toCommit.Tree()
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	patch, err := fromTree.Patch(toTree)
	if err != nil {
		return "", err
	}

	// Use FilePatches() method
	for _, fp := range patch.FilePatches() {
		from, to := fp.Files()
		if from == nil && to == nil {
			continue
		}
		var path string
		if from != nil {
			path = from.Path()
		} else {
			path = to.Path()
		}

		if fp.IsBinary() {
			buf.WriteString(path + ": binary file\n")
			continue
		}

		buf.WriteString(path + ":\n")
		for _, ch := range fp.Chunks() {
			switch ch.Type() {
			case diff.Add:
				buf.WriteString("+")
			case diff.Delete:
				buf.WriteString("-")
			default:
				buf.WriteString(" ")
			}
			buf.WriteString(ch.Content())
		}
		buf.WriteString("\n")
	}

	return buf.String(), nil
}

func (t *GitDiff) diffCommit(client *gitclient.GitClient, commitHash string) (string, error) {
	hash := plumbing.NewHash(commitHash)
	commit, err := client.GetRepository().CommitObject(hash)
	if err != nil {
		return "", err
	}

	commitTree, err := commit.Tree()
	if err != nil {
		return "", err
	}

	head, err := client.GetRepository().Head()
	if err != nil {
		return "", err
	}

	headCommit, err := client.GetRepository().CommitObject(head.Hash())
	if err != nil {
		return "", err
	}

	headTree, err := headCommit.Tree()
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	patch, err := headTree.Patch(commitTree)
	if err != nil {
		return "", err
	}

	// Use FilePatches() method
	for _, fp := range patch.FilePatches() {
		from, to := fp.Files()
		if from == nil && to == nil {
			continue
		}
		var path string
		if from != nil {
			path = from.Path()
		} else {
			path = to.Path()
		}

		if fp.IsBinary() {
			buf.WriteString(path + ": binary file\n")
			continue
		}

		buf.WriteString(path + ":\n")
		for _, ch := range fp.Chunks() {
			switch ch.Type() {
			case diff.Add:
				buf.WriteString("+")
			case diff.Delete:
				buf.WriteString("-")
			default:
				buf.WriteString(" ")
			}
			buf.WriteString(ch.Content())
		}
		buf.WriteString("\n")
	}

	return buf.String(), nil
}

func (t *GitDiff) diffWorktree(client *gitclient.GitClient) (string, error) {
	workTree, err := client.GetWorktree()
	if err != nil {
		return "", err
	}

	status, err := workTree.Status()
	if err != nil {
		return "", err
	}

	if status.IsClean() {
		return "working tree is clean", nil
	}

	var buf bytes.Buffer
	for path, s := range status {
		if s.Worktree != 0 || s.Staging != 0 {
			var changeType string
			switch {
			case s.Staging == git.Unmodified && s.Worktree == git.Deleted:
				changeType = "deleted"
			case s.Staging == git.Untracked:
				changeType = "untracked"
			case s.Staging == git.Added:
				changeType = "new file"
			case s.Staging == git.Modified:
				changeType = "modified"
			case s.Worktree == git.Modified:
				changeType = "modified"
			case s.Worktree == git.Added:
				changeType = "new file"
			default:
				changeType = "changed"
			}
			buf.WriteString(path + ": " + changeType + "\n")
		}
	}

	return buf.String(), nil
}

func (t *GitDiff) diffStaged(client *gitclient.GitClient) (string, error) {
	diffOutput, err := client.DiffStaged(context.Background())
	if err != nil {
		return "", err
	}
	if diffOutput == "" {
		return "no staged changes", nil
	}
	return diffOutput, nil
}

func (t *GitDiff) resolvePath(path string) (string, error) {
	if t.workDir == "" {
		return filepath.Abs(path)
	}
	fullPath := filepath.Join(t.workDir, path)
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", errors.New("invalid path")
	}
	if !strings.HasPrefix(absPath, t.workDir) {
		return "", errors.New("access denied: path outside work directory")
	}
	return absPath, nil
}

// ----------------------------------------------------------------------------
// GitStatus Tool (helper)
// ----------------------------------------------------------------------------

// GitStatus shows the status of a git repository.
type GitStatus struct {
	toolBase
	workDir string
	logger  *zap.Logger
}

const gitStatusSchema = `{
	"type": "object",
	"properties": {
		"path": {
			"type": "string",
			"description": "The path to the git repository (relative to work directory)"
		}
	},
	"required": ["path"]
}`

// NewGitStatus creates a new GitStatus tool.
func NewGitStatus(workDir string, logger *zap.Logger) *GitStatus {
	if logger == nil {
		logger, _ = zap.NewProduction()
	}
	return &GitStatus{
		toolBase: toolBase{
			name:        "GitStatus",
			description: "Shows the status of a git repository",
			inputSchema: gitStatusSchema,
		},
		workDir: workDir,
		logger:  logger,
	}
}

// Execute shows the status with categorized file changes.
func (t *GitStatus) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	start := time.Now()
	path, ok := input["path"].(string)
	if !ok || path == "" {
		return &ToolResult{Success: false, Error: "path is required and must be a string"}, nil
	}

	// Resolve path safely
	repoPath, err := t.resolvePath(path)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, nil
	}

	client := gitclient.New(t.logger)
	if err := client.Open(ctx, repoPath); err != nil {
		return &ToolResult{
			Success: false,
			Error:   domain.NewL4GitOpError("open", repoPath, err).Error(),
			Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
		}, nil
	}

	status, err := client.Status(ctx)
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   domain.NewL4GitOpError("status", repoPath, err).Error(),
			Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
		}, nil
	}

	// Build categorized output
	var stagedFiles, modifiedFiles, untrackedFiles []map[string]any
	for _, f := range status.Staged {
		stagedFiles = append(stagedFiles, map[string]any{
			"path":     f.Path,
			"staging":  f.Staging,
			"worktree": f.Worktree,
		})
	}
	for _, f := range status.Modified {
		modifiedFiles = append(modifiedFiles, map[string]any{
			"path":     f.Path,
			"staging":  f.Staging,
			"worktree": f.Worktree,
		})
	}
	for _, f := range status.Untracked {
		untrackedFiles = append(untrackedFiles, map[string]any{
			"path": f.Path,
		})
	}

	return &ToolResult{
		Success: true,
		Output: map[string]any{
			"branch":     status.Branch,
			"is_clean":   status.IsClean,
			"staged":     stagedFiles,
			"modified":   modifiedFiles,
			"untracked": untrackedFiles,
		},
		Meta: ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
	}, nil
}

func (t *GitStatus) resolvePath(path string) (string, error) {
	if t.workDir == "" {
		return filepath.Abs(path)
	}
	fullPath := filepath.Join(t.workDir, path)
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", errors.New("invalid path")
	}
	if !strings.HasPrefix(absPath, t.workDir) {
		return "", errors.New("access denied: path outside work directory")
	}
	return absPath, nil
}

// Verify tool interfaces
var (
	_ Tool = (*GitClone)(nil)
	_ Tool = (*GitCommit)(nil)
	_ Tool = (*GitPush)(nil)
	_ Tool = (*GitDiff)(nil)
	_ Tool = (*GitStatus)(nil)
)
