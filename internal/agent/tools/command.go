// Package tools implements the tool set available to agents.
package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/cloud-agent-platform/cap/internal/domain"
	"go.uber.org/zap"
)

// ----------------------------------------------------------------------------
// ExecCommand Tool
// ----------------------------------------------------------------------------

// ExecCommand executes a shell command with timeout and security restrictions.
type ExecCommand struct {
	toolBase
	workDir         string
	timeout         time.Duration
	maxOutputSize   int
	allowedCommands []string // whitelist of allowed commands
	blockedPatterns []string // patterns to block in commands
	logger          *zap.Logger
}

const execCommandSchema = `{
	"type": "object",
	"properties": {
		"command": {
			"type": "string",
			"description": "The shell command to execute"
		},
		"timeout": {
			"type": "integer",
			"description": "Maximum execution time in seconds (default: 60, max: 300)"
		}
	},
	"required": ["command"]
}`

// NewExecCommand creates a new ExecCommand tool.
func NewExecCommand(workDir string, timeout time.Duration, maxOutputSize int, allowedCommands, blockedPatterns []string, logger *zap.Logger) *ExecCommand {
	if logger == nil {
		logger, _ = zap.NewProduction()
	}
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	if timeout > 5*time.Minute {
		timeout = 5 * time.Minute
	}
	if maxOutputSize == 0 {
		maxOutputSize = 1024 * 1024 // 1MB default
	}
	return &ExecCommand{
		toolBase: toolBase{
			name:        "ExecCommand",
			description: "Executes a shell command with timeout and security restrictions",
			inputSchema: execCommandSchema,
		},
		workDir:         workDir,
		timeout:         timeout,
		maxOutputSize:   maxOutputSize,
		allowedCommands: allowedCommands,
		blockedPatterns: blockedPatterns,
		logger:          logger,
	}
}

// Execute runs the command.
func (t *ExecCommand) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	start := time.Now()
	command, ok := input["command"].(string)
	if !ok || command == "" {
		return &ToolResult{Success: false, Error: "command is required and must be a string"}, nil
	}

	// Apply timeout
	timeout := t.timeout
	if to, ok := input["timeout"].(float64); ok {
		timeout = time.Duration(to) * time.Second
		if timeout > 5*time.Minute {
			timeout = 5 * time.Minute
		}
		if timeout <= 0 {
			timeout = t.timeout
		}
	}

	// Security: validate command
	if err := t.validateCommand(command); err != nil {
		return &ToolResult{
			Success: false,
			Error:   domain.NewL4ToolDeniedError(t.Name(), err.Error()).Error(),
			Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
		}, nil
	}

	// Set up context with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Determine working directory
	workDir := t.workDir
	if workDir == "" {
		if wd, err := os.Getwd(); err == nil {
			workDir = wd
		}
	}

	// Build the command
	var cmd *exec.Cmd
	shell := "/bin/sh"
	shellArg := "-c"

	// Parse command properly
	parts := parseCommand(command)
	if len(parts) > 0 {
		// Check if it's a direct path to executable
		if filepath.IsAbs(parts[0]) || strings.Contains(parts[0], "/") {
			if filepath.IsAbs(parts[0]) {
				cmd = exec.CommandContext(cmdCtx, parts[0], parts[1:]...)
			} else {
				cmd = exec.CommandContext(cmdCtx, parts[0], parts[1:]...)
			}
		} else {
			// Use shell
			cmd = exec.CommandContext(cmdCtx, shell, shellArg, command)
		}
	} else {
		cmd = exec.CommandContext(cmdCtx, shell, shellArg, command)
	}

	cmd.Dir = workDir
	cmd.Env = os.Environ()

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	t.logger.Info("executing command",
		zap.String("command", command),
		zap.String("workdir", workDir),
		zap.Duration("timeout", timeout),
	)

	// Run the command
	err := cmd.Run()

	// Get exit code
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else if errors.Is(err, context.DeadlineExceeded) {
			return &ToolResult{
				Success: false,
				Error:   domain.NewL2InvalidOperationError("exec", "command timed out").Error(),
				Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
			}, nil
		} else if errors.Is(err, context.Canceled) {
			return &ToolResult{
				Success: false,
				Error:   domain.NewL2InvalidOperationError("exec", "command cancelled").Error(),
				Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
			}, nil
		} else {
			exitCode = -1
		}
	}

	// Truncate output if needed
	stdoutStr := t.truncateOutput(stdout.String())
	stderrStr := t.truncateOutput(stderr.String())

	result := CommandResult{
		Command:  command,
		ExitCode: exitCode,
		Stdout:   stdoutStr,
		Stderr:   stderrStr,
		Duration:  time.Since(start).String(),
	}

	if exitCode == 0 && err == nil {
		return &ToolResult{
			Success: true,
			Output:  result,
			Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
		}, nil
	}

	return &ToolResult{
		Success: false,
		Output:  result,
		Error:   fmt.Sprintf("command exited with code %d", exitCode),
		Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
	}, nil
}

// validateCommand checks if the command is allowed.
func (t *ExecCommand) validateCommand(command string) error {
	// Block dangerous patterns
	dangerousPatterns := []string{
		`rm\s+-rf\s+/`,                    // recursive root delete
		`rm\s+-rf\s+/home`,               // delete home dirs
		`>\s*/dev/sda`,                   // write to device
		`mkfs\.`,                         // format filesystem
		`dd\s+if=`,                       // direct device copy
		`fork\s+bomb`,                    // fork bomb
		`:\(){ :|:& };:`,                 // bash fork bomb
		`curl.*\|.*sh`,                   // pipe to shell download
		`wget.*\|.*sh`,                   // pipe to shell download
		`;\s*sh\s*$`,                     // end with shell
		`>\s*~`,                          // overwrite home
		`chmod\s+-R\s+777\s+/`,           // chmod 777 root
		`dchmod\s+000`,                   // chmod 000
		`/etc/passwd`,                    // modify passwd
		`/etc/shadow`,                    // access shadow
		`>\s+/etc/`,                      // write to etc
		`mv\s+/\s+`,                      // move root
	}

	for _, pattern := range dangerousPatterns {
		matched, _ := regexp.MatchString(pattern, command)
		if matched {
			return fmt.Errorf("command contains blocked pattern: %s", pattern)
		}
	}

	// Check blocked patterns
	for _, pattern := range t.blockedPatterns {
		matched, _ := regexp.MatchString(pattern, command)
		if matched {
			return fmt.Errorf("command contains blocked pattern: %s", pattern)
		}
	}

	// If allowed commands list is set, check against it
	if len(t.allowedCommands) > 0 {
		parts := parseCommand(command)
		if len(parts) == 0 {
			return errors.New("empty command")
		}
		cmdName := parts[0]
		// Find the base command
		if idx := strings.LastIndex(cmdName, "/"); idx >= 0 {
			cmdName = cmdName[idx+1:]
		}

		allowed := false
		for _, allowedCmd := range t.allowedCommands {
			if allowedCmd == cmdName {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("command '%s' not in allowed list", cmdName)
		}
	}

	// Block commands that try to break out of workDir
	if t.workDir != "" {
		absWorkDir, _ := filepath.Abs(t.workDir)
		// Check for path traversal
		if strings.Contains(command, "../") || strings.HasPrefix(command, "/") {
			// Allow absolute paths only if they're within workDir
			re := regexp.MustCompile(`/(?:home|usr|etc|var|sys|proc|dev|sbin|bin|root)(\s|$)`)
			if re.MatchString(command) && !strings.Contains(command, absWorkDir) {
				return errors.New("command references paths outside work directory")
			}
		}
	}

	return nil
}

// parseCommand parses a shell command into parts.
func parseCommand(command string) []string {
	var parts []string
	var current bytes.Buffer
	inQuote := false
	quoteChar := byte(0)

	for i := 0; i < len(command); i++ {
		c := command[i]

		if !inQuote && (c == '"' || c == '\'') {
			inQuote = true
			quoteChar = c
			continue
		}

		if inQuote && c == quoteChar {
			inQuote = false
			continue
		}

		if !inQuote && c == ' ' {
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
			continue
		}

		current.WriteByte(c)
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

// truncateOutput truncates output if it exceeds maxOutputSize.
func (t *ExecCommand) truncateOutput(output string) string {
	if len(output) > t.maxOutputSize {
		return output[:t.maxOutputSize] + fmt.Sprintf("\n... [truncated, total %d bytes]", len(output))
	}
	return output
}

// CommandResult represents the result of a command execution.
type CommandResult struct {
	Command  string `json:"command"`
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Duration string `json:"duration"`
}

// Verify tool interface
var _ Tool = (*ExecCommand)(nil)

// KillProcess kills a process and its children.
func KillProcess(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	// Get the process group ID
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		return cmd.Process.Kill()
	}
	// Kill the process group
	return syscall.Kill(-pgid, syscall.SIGTERM)
}
