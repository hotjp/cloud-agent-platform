package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloud-agent-platform/cap/internal/agent/react"
	"github.com/go-git/go-git/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

func TestReadFile(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "tools_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create test file
	testFile := filepath.Join(tmpDir, "test.txt")
	err = os.WriteFile(testFile, []byte("Hello, World!"), 0644)
	require.NoError(t, err)

	tool := NewReadFile(tmpDir, 10*1024*1024)
	ctx := context.Background()

	t.Run("read existing file", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{"path": "test.txt"})
		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Equal(t, "Hello, World!", result.Output)
	})

	t.Run("read with offset and limit", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{
			"path":   "test.txt",
			"offset": 0,
			"limit":  5,
		})
		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Equal(t, "Hello", result.Output)
	})

	t.Run("read non-existent file", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{"path": "nonexistent.txt"})
		require.NoError(t, err)
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "no such file")
	})

	t.Run("path traversal blocked", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{"path": "../etc/passwd"})
		require.NoError(t, err)
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "access denied")
	})

	t.Run("missing path", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{})
		require.NoError(t, err)
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "path is required")
	})

	t.Run("file too large", func(t *testing.T) {
		smallTool := NewReadFile(tmpDir, 10) // 10 byte limit
		largeFile := filepath.Join(tmpDir, "large.txt")
		os.WriteFile(largeFile, []byte("This file content is much longer than 10 bytes"), 0644)

		result, err := smallTool.Execute(ctx, map[string]any{"path": "large.txt"})
		require.NoError(t, err)
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "exceeds maximum")
	})
}

func TestWriteFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tools_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	tool := NewWriteFile(tmpDir, 1024*1024) // 1MB limit
	ctx := context.Background()

	t.Run("write new file", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{
			"path":    "newfile.txt",
			"content": "Test content",
		})
		require.NoError(t, err)
		assert.True(t, result.Success)

		// Verify file was written
		data, err := os.ReadFile(filepath.Join(tmpDir, "newfile.txt"))
		require.NoError(t, err)
		assert.Equal(t, "Test content", string(data))
	})

	t.Run("append to existing file", func(t *testing.T) {
		// First write
		_, _ = tool.Execute(ctx, map[string]any{
			"path":    "append.txt",
			"content": "Line 1",
		})
		// Then append
		result, err := tool.Execute(ctx, map[string]any{
			"path":    "append.txt",
			"content": "\nLine 2",
			"append":  true,
		})
		require.NoError(t, err)
		assert.True(t, result.Success)

		// Verify content
		data, err := os.ReadFile(filepath.Join(tmpDir, "append.txt"))
		require.NoError(t, err)
		assert.Equal(t, "Line 1\nLine 2", string(data))
	})

	t.Run("path traversal blocked", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{
			"path":    "../evil.txt",
			"content": "evil",
		})
		require.NoError(t, err)
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "access denied")
	})

	t.Run("content too large", func(t *testing.T) {
		smallTool := NewWriteFile(tmpDir, 10) // 10 byte limit
		result, err := smallTool.Execute(ctx, map[string]any{
			"path":    "large.txt",
			"content": "This content is way too large",
		})
		require.NoError(t, err)
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "exceeds maximum")
	})
}

func TestListFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tools_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create test structure
	os.MkdirAll(filepath.Join(tmpDir, "subdir"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("content"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file2.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "subdir", "file3.txt"), []byte("content"), 0644)

	tool := NewListFiles(tmpDir)
	ctx := context.Background()

	t.Run("list root directory", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{"path": "."})
		require.NoError(t, err)
		assert.True(t, result.Success)

		entries, ok := result.Output.([]FileEntry)
		assert.True(t, ok)
		assert.Len(t, entries, 3) // file1.txt, file2.go, subdir
	})

	t.Run("filter with pattern", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{
			"path":    ".",
			"pattern": "*.txt",
		})
		require.NoError(t, err)
		assert.True(t, result.Success)

		entries, ok := result.Output.([]FileEntry)
		assert.True(t, ok)
		assert.Len(t, entries, 1)
		assert.Equal(t, "file1.txt", entries[0].Name)
	})

	t.Run("recursive listing", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{
			"path":      ".",
			"recursive": true,
		})
		require.NoError(t, err)
		assert.True(t, result.Success)

		entries, ok := result.Output.([]FileEntry)
		assert.True(t, ok)
		assert.Len(t, entries, 4) // all files including subdir
	})
}

func TestDeleteFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tools_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create test files
	os.WriteFile(filepath.Join(tmpDir, "to_delete.txt"), []byte("delete me"), 0644)
	os.MkdirAll(filepath.Join(tmpDir, "to_delete_dir"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "to_delete_dir", "file.txt"), []byte("content"), 0644)

	tool := NewDeleteFile(tmpDir)
	ctx := context.Background()

	t.Run("delete file", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{"path": "to_delete.txt"})
		require.NoError(t, err)
		assert.True(t, result.Success)

		// Verify file is gone
		_, err = os.Stat(filepath.Join(tmpDir, "to_delete.txt"))
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("delete directory recursively", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{
			"path":     "to_delete_dir",
			"recursive": true,
		})
		require.NoError(t, err)
		assert.True(t, result.Success)
	})

	t.Run("delete non-existent file", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{"path": "nonexistent.txt"})
		require.NoError(t, err)
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "not found")
	})

	t.Run("delete directory without recursive fails", func(t *testing.T) {
		os.MkdirAll(filepath.Join(tmpDir, "testdir"), 0755)
		result, err := tool.Execute(ctx, map[string]any{"path": "testdir"})
		require.NoError(t, err)
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "recursive")
	})
}

func TestSearchFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tools_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create test files
	os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("Hello World\nFoo Bar"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("Hello Universe\nBar Baz"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file3.go"), []byte("package main\nfunc main()"), 0644)

	tool := NewSearchFiles(tmpDir, 10*1024*1024)
	ctx := context.Background()

	t.Run("search in all files", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{"query": "Hello"})
		require.NoError(t, err)
		assert.True(t, result.Success)

		results, ok := result.Output.([]SearchResult)
		assert.True(t, ok)
		assert.Len(t, results, 2)
	})

	t.Run("case insensitive search", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{
			"query":         "hello",
			"case_sensitive": false,
		})
		require.NoError(t, err)
		assert.True(t, result.Success)

		results, ok := result.Output.([]SearchResult)
		assert.True(t, ok)
		assert.Len(t, results, 2)
	})

	t.Run("filter by file pattern", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{
			"query":        "Hello",
			"file_pattern": "*.go",
		})
		require.NoError(t, err)
		assert.True(t, result.Success)

		results, ok := result.Output.([]SearchResult)
		assert.True(t, ok)
		assert.Len(t, results, 0) // No "Hello" in .go files
	})

	t.Run("regex search", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{
			"query":     "Hello.*World",
			"use_regex": true,
		})
		require.NoError(t, err)
		assert.True(t, result.Success)

		results, ok := result.Output.([]SearchResult)
		assert.True(t, ok)
		assert.GreaterOrEqual(t, len(results), 1)
	})

	t.Run("regex with case insensitive", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{
			"query":         "hello.*world",
			"use_regex":     true,
			"case_sensitive": false,
		})
		require.NoError(t, err)
		assert.True(t, result.Success)
	})

	t.Run("invalid regex", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{
			"query":     "[invalid",
			"use_regex": true,
		})
		require.NoError(t, err)
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "invalid regex")
	})
}

func TestEditFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tools_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create test file
	testFile := filepath.Join(tmpDir, "test.txt")
	err = os.WriteFile(testFile, []byte("Hello World\nFoo Bar\nHello Again"), 0644)
	require.NoError(t, err)

	tool := NewEditFile(tmpDir, 10*1024*1024)
	ctx := context.Background()

	t.Run("basic edit", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{
			"path":       "test.txt",
			"old_string": "Hello World",
			"new_string": "Hello Universe",
		})
		require.NoError(t, err)
		assert.True(t, result.Success)

		output := result.Output.(map[string]any)
		assert.Equal(t, float64(1), output["replacements"])

		// Verify content changed
		data, err := os.ReadFile(testFile)
		require.NoError(t, err)
		assert.Contains(t, string(data), "Hello Universe")
		assert.NotContains(t, string(data), "Hello World\n")
	})

	t.Run("edit with multiple occurrences", func(t *testing.T) {
		// Reset file
		os.WriteFile(testFile, []byte("Hello World\nFoo Bar\nHello Again"), 0644)

		result, err := tool.Execute(ctx, map[string]any{
			"path":       "test.txt",
			"old_string": "Hello",
			"new_string": "Hi",
		})
		require.NoError(t, err)
		assert.True(t, result.Success)

		output := result.Output.(map[string]any)
		assert.Equal(t, float64(2), output["replacements"])

		// Verify content changed
		data, err := os.ReadFile(testFile)
		require.NoError(t, err)
		assert.Contains(t, string(data), "Hi World")
		assert.Contains(t, string(data), "Hi Again")
	})

	t.Run("old_string not found", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{
			"path":       "test.txt",
			"old_string": "Not Found",
			"new_string": "Replaced",
		})
		require.NoError(t, err)
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "not found")
	})

	t.Run("path traversal blocked", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{
			"path":       "../etc/passwd",
			"old_string": "root",
			"new_string": "hacker",
		})
		require.NoError(t, err)
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "access denied")
	})

	t.Run("missing old_string", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{
			"path":       "test.txt",
			"new_string": "replaced",
		})
		require.NoError(t, err)
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "old_string is required")
	})

	t.Run("file too large", func(t *testing.T) {
		smallTool := NewEditFile(tmpDir, 10)
		// Reset file to large content
		os.WriteFile(testFile, []byte("This is a long content file that exceeds 10 bytes"), 0644)

		result, err := smallTool.Execute(ctx, map[string]any{
			"path":       "test.txt",
			"old_string": "long",
			"new_string": "short",
		})
		require.NoError(t, err)
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "exceeds maximum")
	})
}

func TestToolResult(t *testing.T) {
	t.Run("success result", func(t *testing.T) {
		result := NewSuccessResult("test output")
		assert.True(t, result.Success)
		assert.Equal(t, "test output", result.Output)
		assert.Empty(t, result.Error)
	})

	t.Run("error result", func(t *testing.T) {
		err := os.ErrNotExist
		result := NewErrorResult(err)
		assert.False(t, result.Success)
		assert.Nil(t, result.Output)
		assert.Equal(t, "file does not exist", result.Error)
	})

	t.Run("format for LLM success", func(t *testing.T) {
		result := NewSuccessResult("output")
		formatted := result.FormatForLLM()
		assert.Contains(t, formatted, "TOOL SUCCESS")
		assert.Contains(t, formatted, "output")
	})

	t.Run("format for LLM error", func(t *testing.T) {
		result := NewErrorResult(os.ErrPermission)
		formatted := result.FormatForLLM()
		assert.Contains(t, formatted, "TOOL ERROR")
		assert.Contains(t, formatted, "permission denied")
	})
}

func TestExecCommand(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tools_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	logger, _ := zap.NewDevelopment()
	tool := NewExecCommand(tmpDir, 0, 0, nil, nil, logger)
	ctx := context.Background()

	t.Run("echo command", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{"command": "echo 'Hello World'"})
		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Equal(t, 0, result.Output.(CommandResult).ExitCode)
		assert.Contains(t, result.Output.(CommandResult).Stdout, "Hello World")
	})

	t.Run("command with timeout", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{
			"command": "sleep 0.5 && echo done",
			"timeout": 5,
		})
		require.NoError(t, err)
		assert.True(t, result.Success)
	})

	t.Run("command with path", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{"command": "pwd"})
		require.NoError(t, err)
		assert.True(t, result.Success)
	})

	t.Run("blocked dangerous pattern", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{"command": "rm -rf /tmp/some_dir"})
		require.NoError(t, err)
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "blocked pattern")
	})

	t.Run("exit code on failure", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{"command": "exit 42"})
		require.NoError(t, err)
		assert.False(t, result.Success)
		assert.Equal(t, 42, result.Output.(CommandResult).ExitCode)
	})

	t.Run("blocked command not in allowed list", func(t *testing.T) {
		restrictedTool := NewExecCommand(tmpDir, 0, 0, []string{"ls", "cat"}, nil, logger)
		result, err := restrictedTool.Execute(ctx, map[string]any{"command": "echo hello"})
		require.NoError(t, err)
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "not in allowed list")
	})
}

func TestLLMCall(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// Create mock LLM
	mockLLM := &mockLLM{
		response: "This is a test response",
	}

	tool := NewLLMCall(mockLLM, logger)
	ctx := context.Background()

	t.Run("basic call", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{"prompt": "Say hello"})
		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Equal(t, "This is a test response", result.Output.(LLMResult).Content)
	})

	t.Run("with system prompt", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{
			"prompt": "Say hello",
			"system": "You are a helpful assistant.",
		})
		require.NoError(t, err)
		assert.True(t, result.Success)
	})

	t.Run("with options", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{
			"prompt":      "Say hello",
			"temperature":  0.5,
			"max_tokens":  100,
			"model":       "gpt-4",
		})
		require.NoError(t, err)
		assert.True(t, result.Success)
	})

	t.Run("missing prompt", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]any{})
		require.NoError(t, err)
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "prompt is required")
	})
}

// mockLLM for testing
type mockLLM struct {
	response string
	err      error
}

func (m *mockLLM) Generate(ctx context.Context, messages []*react.Message, opts *react.GenerateOptions) (*react.GenerateResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &react.GenerateResult{
		Content:      m.response,
		StopReason:   "stop",
		TotalTokens:  10,
		PromptTokens: 5,
	}, nil
}

func (m *mockLLM) GenerateStream(ctx context.Context, messages []*react.Message, opts *react.GenerateOptions, callback func(chunk string) error) (*react.GenerateResult, error) {
	return m.Generate(ctx, messages, opts)
}

func TestToolAdapter(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "adapter_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	err = os.WriteFile(testFile, []byte("Hello, Adapter!"), 0644)
	require.NoError(t, err)

	t.Run("adapter wraps ReadFile correctly", func(t *testing.T) {
		readTool := NewReadFile(tmpDir, 10*1024*1024)
		adapter := NewToolAdapter(readTool)

		// Verify adapter satisfies react.Tool interface
		var _ react.Tool = adapter

		// Verify adapter metadata
		assert.Equal(t, "ReadFile", adapter.Name())
		assert.Contains(t, adapter.Description(), "Reads the contents")

		ctx := context.Background()
		result, err := adapter.Execute(ctx, map[string]any{"path": "test.txt"})
		require.NoError(t, err)
		assert.Equal(t, "Hello, Adapter!", result)
	})

	t.Run("adapter handles tool errors", func(t *testing.T) {
		readTool := NewReadFile(tmpDir, 10*1024*1024)
		adapter := NewToolAdapter(readTool)

		ctx := context.Background()
		result, err := adapter.Execute(ctx, map[string]any{"path": "nonexistent.txt"})
		require.Error(t, err) // react.Tool returns error on failure
		assert.Contains(t, err.Error(), "no such file")
		assert.Nil(t, result)
	})

	t.Run("adapter wraps WriteFile correctly", func(t *testing.T) {
		writeTool := NewWriteFile(tmpDir, 1024*1024)
		adapter := NewToolAdapter(writeTool)

		ctx := context.Background()
		result, err := adapter.Execute(ctx, map[string]any{
			"path":    "adapter_test.txt",
			"content": "Written via adapter",
		})
		require.NoError(t, err)
		assert.NotNil(t, result)

		// Verify file was actually written
		data, err := os.ReadFile(filepath.Join(tmpDir, "adapter_test.txt"))
		require.NoError(t, err)
		assert.Equal(t, "Written via adapter", string(data))
	})

	t.Run("adapter wraps ListFiles correctly", func(t *testing.T) {
		listTool := NewListFiles(tmpDir)
		adapter := NewToolAdapter(listTool)

		ctx := context.Background()
		result, err := adapter.Execute(ctx, map[string]any{"path": "."})
		require.NoError(t, err)

		// Result should be a slice of FileEntry
		entries, ok := result.([]FileEntry)
		assert.True(t, ok)
		assert.GreaterOrEqual(t, len(entries), 1)
	})

	t.Run("path traversal check helper", func(t *testing.T) {
		// Test the IsPathTraversal helper
		assert.True(t, IsPathTraversal("/workdir", "../etc/passwd"))
		assert.True(t, IsPathTraversal("/workdir", "/etc/passwd"))
		assert.False(t, IsPathTraversal("/workdir", "subdir/file.txt"))
		assert.False(t, IsPathTraversal("/workdir", "file.txt"))
	})
}

func TestToolRegistryAdapter(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "registry_adapter_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create test file
	testFile := filepath.Join(tmpDir, "test.txt")
	err = os.WriteFile(testFile, []byte("Test content"), 0644)
	require.NoError(t, err)

	t.Run("registry adapter registers and retrieves tools", func(t *testing.T) {
		reactRegistry := react.NewToolRegistry()
		adapter := NewToolRegistryAdapter(reactRegistry)

		// Register a tool
		readTool := NewReadFile(tmpDir, 10*1024*1024)
		err := adapter.Register(readTool)
		require.NoError(t, err)

		assert.Equal(t, 1, adapter.Len())

		// Get the tool
		tool := adapter.Get("ReadFile")
		assert.NotNil(t, tool)

		// Execute through adapter
		ctx := context.Background()
		result, err := tool.Execute(ctx, map[string]any{"path": "test.txt"})
		require.NoError(t, err)
		assert.Equal(t, "Test content", result)
	})

	t.Run("registry adapter lists all tools", func(t *testing.T) {
		reactRegistry := react.NewToolRegistry()
		adapter := NewToolRegistryAdapter(reactRegistry)

		// Register multiple tools
		adapter.Register(NewReadFile(tmpDir, 10*1024*1024))
		adapter.Register(NewWriteFile(tmpDir, 1024*1024))
		adapter.Register(NewListFiles(tmpDir))

		assert.Equal(t, 3, adapter.Len())

		tools := adapter.List()
		assert.Len(t, tools, 3)
	})
}

func TestGitStatus(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "git_status_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Initialize a git repo
	repo, err := git.PlainInit(tmpDir, false)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)

	logger := zaptest.NewLogger(t)

	t.Run("status of clean repo", func(t *testing.T) {
		tool := NewGitStatus("", logger)
		ctx := context.Background()

		result, err := tool.Execute(ctx, map[string]any{"path": tmpDir})
		require.NoError(t, err)
		assert.True(t, result.Success)

		output := result.Output.(map[string]any)
		assert.True(t, output["is_clean"].(bool))
		assert.Empty(t, output["staged"])
		assert.Empty(t, output["modified"])
		assert.Empty(t, output["untracked"])
	})

	t.Run("status with modified file", func(t *testing.T) {
		// Create a file and commit it first
		testFile := filepath.Join(tmpDir, "test.txt")
		err = os.WriteFile(testFile, []byte("hello world"), 0644)
		require.NoError(t, err)

		_, err = wt.Add("test.txt")
		require.NoError(t, err)
		_, err = wt.Commit("initial commit", &git.CommitOptions{})
		require.NoError(t, err)

		// Now modify the file (not staged)
		err = os.WriteFile(testFile, []byte("modified content"), 0644)
		require.NoError(t, err)

		tool := NewGitStatus("", logger)
		ctx := context.Background()

		result, err := tool.Execute(ctx, map[string]any{"path": tmpDir})
		require.NoError(t, err)
		assert.True(t, result.Success)

		output := result.Output.(map[string]any)
		assert.False(t, output["is_clean"].(bool))
		modified, ok := output["modified"].([]map[string]any)
		assert.True(t, ok)
		assert.Len(t, modified, 1)
		assert.Equal(t, "test.txt", modified[0]["path"])
	})

	t.Run("status with staged file", func(t *testing.T) {
		// Create and commit a file
		testFile := filepath.Join(tmpDir, "test2.txt")
		err = os.WriteFile(testFile, []byte("original"), 0644)
		require.NoError(t, err)

		_, err = wt.Add("test2.txt")
		require.NoError(t, err)
		_, err = wt.Commit("add test2", &git.CommitOptions{})
		require.NoError(t, err)

		// Modify and stage the file
		err = os.WriteFile(testFile, []byte("staged changes"), 0644)
		require.NoError(t, err)
		_, err = wt.Add("test2.txt")
		require.NoError(t, err)

		tool := NewGitStatus("", logger)
		ctx := context.Background()

		result, err := tool.Execute(ctx, map[string]any{"path": tmpDir})
		require.NoError(t, err)
		assert.True(t, result.Success)

		output := result.Output.(map[string]any)
		assert.False(t, output["is_clean"].(bool))
		staged, ok := output["staged"].([]map[string]any)
		assert.True(t, ok)
		assert.GreaterOrEqual(t, len(staged), 1)
	})

	t.Run("status with untracked file", func(t *testing.T) {
		// Create a new untracked file
		newFile := filepath.Join(tmpDir, "untracked.txt")
		err = os.WriteFile(newFile, []byte("new content"), 0644)
		require.NoError(t, err)

		tool := NewGitStatus("", logger)
		ctx := context.Background()

		result, err := tool.Execute(ctx, map[string]any{"path": tmpDir})
		require.NoError(t, err)
		assert.True(t, result.Success)

		output := result.Output.(map[string]any)
		assert.False(t, output["is_clean"].(bool))
		untracked, ok := output["untracked"].([]map[string]any)
		assert.True(t, ok)
		assert.GreaterOrEqual(t, len(untracked), 1)
	})

	t.Run("missing path", func(t *testing.T) {
		tool := NewGitStatus("", logger)
		ctx := context.Background()

		result, err := tool.Execute(ctx, map[string]any{})
		require.NoError(t, err)
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "path is required")
	})

	t.Run("invalid path", func(t *testing.T) {
		tool := NewGitStatus("", logger)
		ctx := context.Background()

		result, err := tool.Execute(ctx, map[string]any{"path": "/nonexistent/path"})
		require.NoError(t, err)
		assert.False(t, result.Success)
	})
}

func TestGitDiff(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "git_diff_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Initialize a git repo
	repo, err := git.PlainInit(tmpDir, false)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)

	logger := zaptest.NewLogger(t)

	t.Run("diff of clean repo", func(t *testing.T) {
		tool := NewGitDiff("", logger)
		ctx := context.Background()

		result, err := tool.Execute(ctx, map[string]any{"path": tmpDir})
		require.NoError(t, err)
		assert.True(t, result.Success)

		output := result.Output.(map[string]any)
		assert.Contains(t, output["diff"], "clean")
	})

	t.Run("diff with unstaged changes", func(t *testing.T) {
		// Create a file and commit it
		testFile := filepath.Join(tmpDir, "test.txt")
		err = os.WriteFile(testFile, []byte("hello world"), 0644)
		require.NoError(t, err)

		_, err = wt.Add("test.txt")
		require.NoError(t, err)
		_, err = wt.Commit("initial commit", &git.CommitOptions{})
		require.NoError(t, err)

		// Modify the file
		err = os.WriteFile(testFile, []byte("hello world modified"), 0644)
		require.NoError(t, err)

		tool := NewGitDiff("", logger)
		ctx := context.Background()

		result, err := tool.Execute(ctx, map[string]any{"path": tmpDir})
		require.NoError(t, err)
		assert.True(t, result.Success)

		output := result.Output.(map[string]any)
		diffOutput, ok := output["diff"].(string)
		assert.True(t, ok)
		assert.Contains(t, diffOutput, "test.txt")
	})

	t.Run("diff with staged changes", func(t *testing.T) {
		// Create a file and commit it
		testFile := filepath.Join(tmpDir, "test2.txt")
		err = os.WriteFile(testFile, []byte("original"), 0644)
		require.NoError(t, err)

		_, err = wt.Add("test2.txt")
		require.NoError(t, err)
		_, err = wt.Commit("initial commit", &git.CommitOptions{})
		require.NoError(t, err)

		// Modify and stage the file
		err = os.WriteFile(testFile, []byte("staged content"), 0644)
		require.NoError(t, err)
		_, err = wt.Add("test2.txt")
		require.NoError(t, err)

		tool := NewGitDiff("", logger)
		ctx := context.Background()

		result, err := tool.Execute(ctx, map[string]any{"path": tmpDir, "staged": true})
		require.NoError(t, err)
		assert.True(t, result.Success)

		output := result.Output.(map[string]any)
		diffOutput, ok := output["diff"].(string)
		assert.True(t, ok)
		assert.NotContains(t, diffOutput, "no staged changes")
	})

	t.Run("diff between commits", func(t *testing.T) {
		// Create two commits
		testFile := filepath.Join(tmpDir, "test3.txt")
		err = os.WriteFile(testFile, []byte("v1"), 0644)
		require.NoError(t, err)

		_, err = wt.Add("test3.txt")
		require.NoError(t, err)
		commit1, err := wt.Commit("v1 commit", &git.CommitOptions{})
		require.NoError(t, err)

		// Make another commit
		err = os.WriteFile(testFile, []byte("v2"), 0644)
		require.NoError(t, err)
		_, err = wt.Add("test3.txt")
		require.NoError(t, err)
		_, err = wt.Commit("v2 commit", &git.CommitOptions{})
		require.NoError(t, err)

		tool := NewGitDiff("", logger)
		ctx := context.Background()

		result, err := tool.Execute(ctx, map[string]any{
			"path":      tmpDir,
			"commit":    commit1.String(),
			"to_commit": "HEAD",
		})
		require.NoError(t, err)
		assert.True(t, result.Success)

		output := result.Output.(map[string]any)
		diffOutput, ok := output["diff"].(string)
		assert.True(t, ok)
		assert.Contains(t, diffOutput, "test3.txt")
	})

	t.Run("missing path", func(t *testing.T) {
		tool := NewGitDiff("", logger)
		ctx := context.Background()

		result, err := tool.Execute(ctx, map[string]any{})
		require.NoError(t, err)
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "path is required")
	})
}

func TestGitCommit(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "git_commit_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Initialize a git repo
	_, err = git.PlainInit(tmpDir, false)
	require.NoError(t, err)

	logger := zaptest.NewLogger(t)

	t.Run("commit with changes", func(t *testing.T) {
		// Create a file
		testFile := filepath.Join(tmpDir, "test.txt")
		err = os.WriteFile(testFile, []byte("hello world"), 0644)
		require.NoError(t, err)

		tool := NewGitCommit("", logger)
		ctx := context.Background()

		result, err := tool.Execute(ctx, map[string]any{
			"path":    tmpDir,
			"message": "add test file",
		})
		require.NoError(t, err)
		assert.True(t, result.Success)

		output := result.Output.(map[string]any)
		assert.NotEmpty(t, output["commit_hash"])
	})

	t.Run("commit with empty message fails", func(t *testing.T) {
		tool := NewGitCommit("", logger)
		ctx := context.Background()

		result, err := tool.Execute(ctx, map[string]any{
			"path":    tmpDir,
			"message": "",
		})
		require.NoError(t, err)
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "message is required")
	})

	t.Run("missing path", func(t *testing.T) {
		tool := NewGitCommit("", logger)
		ctx := context.Background()

		result, err := tool.Execute(ctx, map[string]any{"message": "test"})
		require.NoError(t, err)
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "path is required")
	})
}

func TestGitPush(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "git_push_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Initialize a git repo
	repo, err := git.PlainInit(tmpDir, false)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)

	// Create initial commit on a non-main branch
	_, err = wt.Commit("initial", &git.CommitOptions{})
	require.NoError(t, err)

	// Create a feature branch
	err = wt.Checkout(&git.CheckoutOptions{Branch: "refs/heads/feature/test"})
	require.NoError(t, err)

	logger := zaptest.NewLogger(t)

	t.Run("push without remote fails gracefully", func(t *testing.T) {
		tool := NewGitPush("", nil, logger)
		ctx := context.Background()

		result, err := tool.Execute(ctx, map[string]any{"path": tmpDir})
		require.NoError(t, err)
		// Push without remote should fail, but not crash
		assert.False(t, result.Success)
	})

	t.Run("push to protected branch blocked", func(t *testing.T) {
		// Checkout main branch
		err = wt.Checkout(&git.CheckoutOptions{Branch: "refs/heads/main"})
		require.NoError(t, err)

		// Create a commit on main
		testFile := filepath.Join(tmpDir, "main.txt")
		err = os.WriteFile(testFile, []byte("main content"), 0644)
		require.NoError(t, err)

		_, err = wt.Add("main.txt")
		require.NoError(t, err)
		_, err = wt.Commit("main commit", &git.CommitOptions{})
		require.NoError(t, err)

		tool := NewGitPush("", nil, logger)
		ctx := context.Background()

		result, err := tool.Execute(ctx, map[string]any{"path": tmpDir})
		require.NoError(t, err)
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "protected")
	})

	t.Run("missing path", func(t *testing.T) {
		tool := NewGitPush("", nil, logger)
		ctx := context.Background()

		result, err := tool.Execute(ctx, map[string]any{})
		require.NoError(t, err)
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "path is required")
	})
}

func TestGitClone(t *testing.T) {
	// Create a temp dir to act as "remote"
	remoteDir, err := os.MkdirTemp("", "git_remote")
	require.NoError(t, err)
	defer os.RemoveAll(remoteDir)

	// Init remote repo
	remoteRepo, err := git.PlainInit(remoteDir, false)
	require.NoError(t, err)
	remoteWt, err := remoteRepo.Worktree()
	require.NoError(t, err)

	// Add a file to remote
	remoteFile := filepath.Join(remoteDir, "README.md")
	err = os.WriteFile(remoteFile, []byte("# Test\n"), 0644)
	require.NoError(t, err)
	_, err = remoteWt.Add("README.md")
	require.NoError(t, err)
	_, err = remoteWt.Commit("initial commit", &git.CommitOptions{})
	require.NoError(t, err)

	// Clone dir
	cloneDir, err := os.MkdirTemp("", "git_clone_dest")
	require.NoError(t, err)
	defer os.RemoveAll(cloneDir)

	logger := zaptest.NewLogger(t)

	t.Run("clone repository", func(t *testing.T) {
		tool := NewGitClone("", nil, logger)
		ctx := context.Background()

		result, err := tool.Execute(ctx, map[string]any{
			"url": "file://" + remoteDir,
			"path": cloneDir,
		})
		require.NoError(t, err)
		assert.True(t, result.Success)

		output := result.Output.(map[string]any)
		assert.Equal(t, "file://"+remoteDir, output["url"])
	})

	t.Run("clone missing url", func(t *testing.T) {
		tool := NewGitClone("", nil, logger)
		ctx := context.Background()

		result, err := tool.Execute(ctx, map[string]any{"path": cloneDir})
		require.NoError(t, err)
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "url is required")
	})
}
