// Package tools implements the tool set available to agents.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/cloud-agent-platform/cap/internal/domain"
)

// ----------------------------------------------------------------------------
// ReadFile Tool
// ----------------------------------------------------------------------------

// ReadFile reads the contents of a file.
type ReadFile struct {
	toolBase
	workDir     string // allowed base directory
	maxFileSize int64  // maximum file size in bytes
}

const readFileSchema = `{
	"type": "object",
	"properties": {
		"path": {
			"type": "string",
			"description": "The path to the file to read (relative to work directory)"
		},
		"offset": {
			"type": "integer",
			"description": "Byte offset to start reading from (default: 0)"
		},
		"limit": {
			"type": "integer",
			"description": "Maximum number of bytes to read (default: no limit)"
		}
	},
	"required": ["path"]
}`

// NewReadFile creates a new ReadFile tool.
func NewReadFile(workDir string, maxFileSize int64) *ReadFile {
	if maxFileSize == 0 {
		maxFileSize = 10 * 1024 * 1024 // default 10MB
	}
	return &ReadFile{
		toolBase: toolBase{
			name:        "ReadFile",
			description: "Reads the contents of a file from the filesystem",
			inputSchema: readFileSchema,
		},
		workDir:     workDir,
		maxFileSize: maxFileSize,
	}
}

// Execute reads the file contents.
func (t *ReadFile) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	start := time.Now()
	path, ok := input["path"].(string)
	if !ok || path == "" {
		return &ToolResult{Success: false, Error: "path is required and must be a string"}, nil
	}

	// Resolve path safely
	fullPath, err := t.resolvePath(path)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, nil
	}

	// Check file size before reading
	info, err := os.Stat(fullPath)
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   domain.NewL2InvalidOperationError("read", err.Error()).Error(),
			Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
		}, nil
	}

	if info.Size() > t.maxFileSize {
		return &ToolResult{
			Success: false,
			Error:   domain.NewL2InvalidOperationError("read", "file size exceeds maximum allowed").Error(),
			Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
		}, nil
	}

	// Read file
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   domain.NewL2InvalidOperationError("read", err.Error()).Error(),
			Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
		}, nil
	}

	// Handle offset and limit
	offset := 0
	if o, ok := input["offset"].(float64); ok {
		offset = int(o)
	} else if o, ok := input["offset"].(int); ok {
		offset = o
	}
	limit := len(data)
	if l, ok := input["limit"].(float64); ok {
		limit = int(l)
	} else if l, ok := input["limit"].(int); ok {
		limit = l
	}

	if offset >= len(data) {
		return &ToolResult{
			Success: true,
			Output:  "",
			Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
		}, nil
	}

	end := offset + limit
	if end > len(data) {
		end = len(data)
	}

	return &ToolResult{
		Success: true,
		Output:  string(data[offset:end]),
		Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
	}, nil
}

func (t *ReadFile) resolvePath(path string) (string, error) {
	if t.workDir == "" {
		return filepath.Abs(path)
	}
	fullPath := filepath.Join(t.workDir, path)
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", errors.New("invalid path")
	}
	// Security: ensure path is within workDir
	if !strings.HasPrefix(absPath, t.workDir) {
		return "", errors.New("access denied: path outside work directory")
	}
	return absPath, nil
}

// ----------------------------------------------------------------------------
// WriteFile Tool
// ----------------------------------------------------------------------------

// WriteFile writes content to a file.
type WriteFile struct {
	toolBase
	workDir     string
	maxFileSize int64 // maximum file size in bytes
}

const writeFileSchema = `{
	"type": "object",
	"properties": {
		"path": {
			"type": "string",
			"description": "The path to the file to write (relative to work directory)"
		},
		"content": {
			"type": "string",
			"description": "The content to write to the file"
		},
		"append": {
			"type": "boolean",
			"description": "Whether to append to existing file (default: false, overwrites)"
		}
	},
	"required": ["path", "content"]
}`

// NewWriteFile creates a new WriteFile tool.
func NewWriteFile(workDir string, maxFileSize int64) *WriteFile {
	if maxFileSize == 0 {
		maxFileSize = 10 * 1024 * 1024 // default 10MB
	}
	return &WriteFile{
		toolBase: toolBase{
			name:        "WriteFile",
			description: "Writes content to a file in the filesystem",
			inputSchema: writeFileSchema,
		},
		workDir:     workDir,
		maxFileSize: maxFileSize,
	}
}

// Execute writes content to the file.
func (t *WriteFile) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	start := time.Now()
	path, ok := input["path"].(string)
	if !ok || path == "" {
		return &ToolResult{Success: false, Error: "path is required and must be a string"}, nil
	}
	content, ok := input["content"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "content is required and must be a string"}, nil
	}

	// Check size limit
	if int64(len(content)) > t.maxFileSize {
		return &ToolResult{
			Success: false,
			Error:   domain.NewL2InvalidOperationError("write", "file size exceeds maximum allowed").Error(),
			Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
		}, nil
	}

	// Resolve path safely
	fullPath, err := t.resolvePath(path)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, nil
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return &ToolResult{
			Success: false,
			Error:   domain.NewL2InvalidOperationError("write", err.Error()).Error(),
			Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
		}, nil
	}

	// Determine write mode
	append := false
	if a, ok := input["append"].(bool); ok {
		append = a
	}

	var flags int
	if append {
		flags = os.O_APPEND | os.O_CREATE | os.O_WRONLY
	} else {
		flags = os.O_TRUNC | os.O_CREATE | os.O_WRONLY
	}

	file, err := os.OpenFile(fullPath, flags, 0644)
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   domain.NewL2InvalidOperationError("write", err.Error()).Error(),
			Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
		}, nil
	}
	defer file.Close()

	if _, err := file.WriteString(content); err != nil {
		return &ToolResult{
			Success: false,
			Error:   domain.NewL2InvalidOperationError("write", err.Error()).Error(),
			Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
		}, nil
	}

	return &ToolResult{
		Success: true,
		Output:  map[string]any{"path": fullPath, "bytes_written": len(content)},
		Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
	}, nil
}

func (t *WriteFile) resolvePath(path string) (string, error) {
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
// ListFiles Tool
// ----------------------------------------------------------------------------

// ListFiles lists files in a directory.
type ListFiles struct {
	toolBase
	workDir string
}

const listFilesSchema = `{
	"type": "object",
	"properties": {
		"path": {
			"type": "string",
			"description": "The directory path to list (relative to work directory, default: '.')"
		},
		"recursive": {
			"type": "boolean",
			"description": "Whether to list recursively (default: false)"
		},
		"pattern": {
			"type": "string",
			"description": "Glob pattern to filter files (e.g., '*.go', '**/*.txt')"
		}
	},
	"required": []
}`

// NewListFiles creates a new ListFiles tool.
func NewListFiles(workDir string) *ListFiles {
	return &ListFiles{
		toolBase: toolBase{
			name:        "ListFiles",
			description: "Lists files and directories in a given directory",
			inputSchema: listFilesSchema,
		},
		workDir: workDir,
	}
}

// Execute lists the files.
func (t *ListFiles) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	start := time.Now()
	path := "."
	if p, ok := input["path"].(string); ok {
		path = p
	}

	recursive := false
	if r, ok := input["recursive"].(bool); ok {
		recursive = r
	}

	pattern := ""
	if p, ok := input["pattern"].(string); ok {
		pattern = p
	}

	// Resolve path safely
	fullPath, err := t.resolvePath(path)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, nil
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   domain.NewL2InvalidOperationError("list", err.Error()).Error(),
			Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
		}, nil
	}

	var results []FileEntry
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		relPath, _ := filepath.Rel(t.workDir, filepath.Join(fullPath, entry.Name()))
		fe := FileEntry{
			Name:    entry.Name(),
			Path:    relPath,
			IsDir:   entry.IsDir(),
			Size:    info.Size(),
			Mode:    info.Mode().String(),
			ModTime: info.ModTime().Format(time.RFC3339),
		}

		// Apply pattern filter
		if pattern != "" {
			matched, _ := filepath.Match(pattern, entry.Name())
			if !matched {
				continue
			}
		}

		results = append(results, fe)
	}

	// Handle recursive
	if recursive {
		// For recursive, we need to gather all files from subdirectories
		// but avoid re-adding files that were already in the base directory
		results = t.gatherRecursive(fullPath, pattern, results)
	}

	return &ToolResult{
		Success: true,
		Output:  results,
		Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
	}, nil
}

func (t *ListFiles) gatherRecursive(basePath, pattern string, existing []FileEntry) []FileEntry {
	results := existing
	if results == nil {
		results = []FileEntry{}
	}

	// Track paths we've already added to avoid duplicates
	addedPaths := make(map[string]bool)
	for _, fe := range results {
		addedPaths[fe.Path] = true
	}

	_ = filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		// Skip the base path itself
		if path == basePath {
			return nil
		}

		relPath, _ := filepath.Rel(t.workDir, path)

		// Calculate depth: number of path separators from basePath
		depth := strings.Count(relPath, string(filepath.Separator)) + 1

		if info.IsDir() {
			// For directories, just return nil to continue walking
			return nil
		}

		// Skip direct children (depth=1) - they're already in existing
		if depth == 1 {
			return nil
		}

		// Skip if already added
		if addedPaths[relPath] {
			return nil
		}

		if pattern != "" {
			matched, _ := filepath.Match(pattern, info.Name())
			if !matched {
				return nil
			}
		}

		results = append(results, FileEntry{
			Name:    info.Name(),
			Path:    relPath,
			IsDir:   false,
			Size:    info.Size(),
			Mode:    info.Mode().String(),
			ModTime: info.ModTime().Format(time.RFC3339),
		})
		addedPaths[relPath] = true
		return nil
	})

	return results
}

func (t *ListFiles) resolvePath(path string) (string, error) {
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

// FileEntry represents a file or directory entry.
type FileEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	ModTime string `json:"mod_time"`
}

// MarshalJSON for FileEntry
func (f FileEntry) MarshalJSON() ([]byte, error) {
	type Alias FileEntry
	return json.Marshal(&struct {
		Alias
	}{
		Alias: Alias(f),
	})
}

// ----------------------------------------------------------------------------
// DeleteFile Tool
// ----------------------------------------------------------------------------

// DeleteFile deletes a file or directory.
type DeleteFile struct {
	toolBase
	workDir string
}

const deleteFileSchema = `{
	"type": "object",
	"properties": {
		"path": {
			"type": "string",
			"description": "The path to the file or directory to delete"
		},
		"recursive": {
			"type": "boolean",
			"description": "Whether to delete directories recursively (default: false)"
		}
	},
	"required": ["path"]
}`

// NewDeleteFile creates a new DeleteFile tool.
func NewDeleteFile(workDir string) *DeleteFile {
	return &DeleteFile{
		toolBase: toolBase{
			name:        "DeleteFile",
			description: "Deletes a file or directory from the filesystem",
			inputSchema: deleteFileSchema,
		},
		workDir: workDir,
	}
}

// Execute deletes the file or directory.
func (t *DeleteFile) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	start := time.Now()
	path, ok := input["path"].(string)
	if !ok || path == "" {
		return &ToolResult{Success: false, Error: "path is required and must be a string"}, nil
	}

	recursive := false
	if r, ok := input["recursive"].(bool); ok {
		recursive = r
	}

	// Resolve path safely
	fullPath, err := t.resolvePath(path)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, nil
	}

	// Check if path exists
	info, err := os.Stat(fullPath)
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   domain.NewL2NotFoundError("file", fullPath).Error(),
			Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
		}, nil
	}

	if info.IsDir() && !recursive {
		return &ToolResult{
			Success: false,
			Error:   domain.NewL2InvalidOperationError("delete", "cannot delete directory without recursive=true").Error(),
			Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
		}, nil
	}

	if info.IsDir() {
		err = os.RemoveAll(fullPath)
	} else {
		err = os.Remove(fullPath)
	}

	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   domain.NewL2InvalidOperationError("delete", err.Error()).Error(),
			Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
		}, nil
	}

	return &ToolResult{
		Success: true,
		Output:  map[string]any{"deleted": fullPath},
		Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
	}, nil
}

func (t *DeleteFile) resolvePath(path string) (string, error) {
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
// SearchFiles Tool
// ----------------------------------------------------------------------------

// SearchFiles searches for files matching criteria.
type SearchFiles struct {
	toolBase
	workDir      string
	maxFileSize  int64 // maximum file size to search (in bytes)
}

const searchFilesSchema = `{
	"type": "object",
	"properties": {
		"path": {
			"type": "string",
			"description": "The directory path to search in (relative to work directory, default: '.')"
		},
		"query": {
			"type": "string",
			"description": "Text or regex pattern to search for in file contents"
		},
		"file_pattern": {
			"type": "string",
			"description": "Glob pattern for files to search (e.g., '*.go', '*.md')"
		},
		"case_sensitive": {
			"type": "boolean",
			"description": "Whether the search is case sensitive (default: false)"
		},
		"use_regex": {
			"type": "boolean",
			"description": "Whether to treat query as regex pattern (default: false)"
		},
		"max_results": {
			"type": "integer",
			"description": "Maximum number of results to return (default: 100)"
		}
	},
	"required": ["query"]
}`

// NewSearchFiles creates a new SearchFiles tool.
func NewSearchFiles(workDir string, maxFileSize int64) *SearchFiles {
	if maxFileSize == 0 {
		maxFileSize = 10 * 1024 * 1024 // default 10MB
	}
	return &SearchFiles{
		toolBase: toolBase{
			name:        "SearchFiles",
			description: "Searches for text within files matching the given criteria",
			inputSchema: searchFilesSchema,
		},
		workDir:     workDir,
		maxFileSize: maxFileSize,
	}
}

// Execute searches for the query in files.
func (t *SearchFiles) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	start := time.Now()
	query, ok := input["query"].(string)
	if !ok || query == "" {
		return &ToolResult{Success: false, Error: "query is required and must be a string"}, nil
	}

	path := "."
	if p, ok := input["path"].(string); ok {
		path = p
	}

	filePattern := ""
	if fp, ok := input["file_pattern"].(string); ok {
		filePattern = fp
	}

	caseSensitive := false
	if cs, ok := input["case_sensitive"].(bool); ok {
		caseSensitive = cs
	}

	useRegex := false
	if ur, ok := input["use_regex"].(bool); ok {
		useRegex = ur
	}

	maxResults := 100
	if mr, ok := input["max_results"].(float64); ok {
		maxResults = int(mr)
	}

	// Resolve path safely
	fullPath, err := t.resolvePath(path)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, nil
	}

	results, err := t.search(fullPath, query, filePattern, caseSensitive, useRegex, maxResults)
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   domain.NewL2InvalidOperationError("search", err.Error()).Error(),
			Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
		}, nil
	}

	return &ToolResult{
		Success: true,
		Output:  results,
		Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
	}, nil
}

func (t *SearchFiles) search(basePath, query, filePattern string, caseSensitive, useRegex bool, maxResults int) ([]SearchResult, error) {
	var results []SearchResult

	// Compile regex if needed
	var regex *regexp.Regexp
	var regexErr error
	if useRegex {
		regex, regexErr = regexp.Compile(query)
		if regexErr != nil {
			return nil, fmt.Errorf("invalid regex pattern: %w", regexErr)
		}
	}

	err := filepath.Walk(basePath, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}

		relPath, _ := filepath.Rel(t.workDir, filePath)

		// Apply file pattern filter
		if filePattern != "" {
			matched, _ := filepath.Match(filePattern, info.Name())
			if !matched {
				return nil
			}
		}

		// Check file size limit
		if info.Size() > t.maxFileSize {
			return nil
		}

		// Read and search file content
		content, err := os.ReadFile(filePath)
		if err != nil {
			return nil
		}

		text := string(content)
		if !caseSensitive && !useRegex {
			text = strings.ToLower(text)
			query = strings.ToLower(query)
		}

		lines := strings.Split(text, "\n")
		for lineNum, line := range lines {
			var matched bool
			if useRegex && regex != nil {
				matched = regex.MatchString(line)
			} else {
				matched = strings.Contains(line, query)
			}
			if matched {
				if len(results) >= maxResults {
					return filepath.SkipDir // Early exit
				}
				results = append(results, SearchResult{
					Path:    relPath,
					Line:    lineNum + 1,
					Content: strings.TrimSpace(line),
				})
			}
		}
		return nil
	})

	if err != nil && err != filepath.SkipDir {
		return results, err
	}
	return results, nil
}

func (t *SearchFiles) resolvePath(path string) (string, error) {
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

// SearchResult represents a search match.
type SearchResult struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Content string `json:"content"`
}

// ----------------------------------------------------------------------------
// EditFile Tool
// ----------------------------------------------------------------------------

// EditFile performs precise string replacement in a file.
type EditFile struct {
	toolBase
	workDir     string
	maxFileSize int64
}

const editFileSchema = `{
	"type": "object",
	"properties": {
		"path": {
			"type": "string",
			"description": "The path to the file to edit (relative to work directory)"
		},
		"old_string": {
			"type": "string",
			"description": "The exact string to replace (must match exactly including whitespace)"
		},
		"new_string": {
			"type": "string",
			"description": "The replacement string"
		}
	},
	"required": ["path", "old_string", "new_string"]
}`

// NewEditFile creates a new EditFile tool.
func NewEditFile(workDir string, maxFileSize int64) *EditFile {
	if maxFileSize == 0 {
		maxFileSize = 10 * 1024 * 1024 // default 10MB
	}
	return &EditFile{
		toolBase: toolBase{
			name:        "EditFile",
			description: "Performs precise string replacement in a file (old_string → new_string)",
			inputSchema: editFileSchema,
		},
		workDir:     workDir,
		maxFileSize: maxFileSize,
	}
}

// Execute performs the string replacement.
func (t *EditFile) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	start := time.Now()
	path, ok := input["path"].(string)
	if !ok || path == "" {
		return &ToolResult{Success: false, Error: "path is required and must be a string"}, nil
	}

	oldString, ok := input["old_string"].(string)
	if !ok || oldString == "" {
		return &ToolResult{Success: false, Error: "old_string is required and must be a string"}, nil
	}

	newString, ok := input["new_string"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "new_string is required and must be a string"}, nil
	}

	// Resolve path safely
	fullPath, err := t.resolvePath(path)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, nil
	}

	// Check file size before reading
	info, err := os.Stat(fullPath)
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   domain.NewL2InvalidOperationError("edit", err.Error()).Error(),
			Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
		}, nil
	}

	if info.Size() > t.maxFileSize {
		return &ToolResult{
			Success: false,
			Error:   domain.NewL2InvalidOperationError("edit", "file size exceeds maximum allowed").Error(),
			Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
		}, nil
	}

	// Read file content
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   domain.NewL2InvalidOperationError("edit", err.Error()).Error(),
			Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
		}, nil
	}

	text := string(content)

	// Find and replace
	if !strings.Contains(text, oldString) {
		return &ToolResult{
			Success: false,
			Error:   domain.NewL2InvalidOperationError("edit", "old_string not found in file").Error(),
			Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
		}, nil
	}

	// Count occurrences before replacing
	occurrences := strings.Count(text, oldString)

	// Perform replacement
	newContent := strings.ReplaceAll(text, oldString, newString)

	// Write back
	if err := os.WriteFile(fullPath, []byte(newContent), 0644); err != nil {
		return &ToolResult{
			Success: false,
			Error:   domain.NewL2InvalidOperationError("edit", err.Error()).Error(),
			Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
		}, nil
	}

	return &ToolResult{
		Success: true,
		Output: map[string]any{
			"path":        fullPath,
			"replacements": float64(occurrences),
		},
		Meta: ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
	}, nil
}

func (t *EditFile) resolvePath(path string) (string, error) {
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

// Verify tool interface
var _ Tool = (*EditFile)(nil)