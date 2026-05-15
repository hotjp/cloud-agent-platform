// Package scheduler - adapter to bridge scheduler to orchestrator's WorkerExecutor interface.
package scheduler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloud-agent-platform/cap/internal/domain"
	"github.com/cloud-agent-platform/cap/internal/domain/worker"
	"github.com/cloud-agent-platform/cap/internal/orchestrator"
)

// OrchestratorAdapter wraps a Scheduler to implement orchestrator.WorkerExecutor.
type OrchestratorAdapter struct {
	sched  Scheduler
	config AdapterConfig
}

// AdapterConfig holds LLM and image configuration.
type AdapterConfig struct {
	WorkerImage string
	LLMAPIURL   string
	LLMAPIKey   string
	LLMModel    string
}

// DefaultAdapterConfig returns sensible defaults.
func DefaultAdapterConfig() AdapterConfig {
	return AdapterConfig{
		WorkerImage: "cap-worker:latest",
		LLMAPIURL:   "https://open.bigmodel.cn/api/paas/v4/chat/completions",
		LLMAPIKey:   "", // Must be configured
		LLMModel:    "glm-4-flash",
	}
}

// NewOrchestratorAdapter creates a WorkerExecutor backed by the scheduler.
func NewOrchestratorAdapter(sched Scheduler, config AdapterConfig) *OrchestratorAdapter {
	return &OrchestratorAdapter{sched: sched, config: config}
}

// Execute implements orchestrator.WorkerExecutor.
func (a *OrchestratorAdapter) Execute(ctx context.Context, subtaskID, taskID string, opts worker.ExecOptions) (*orchestrator.AgentResult, error) {
	// Build env vars: merge task context from orchestrator + LLM config
	env := make(map[string]string)
	for k, v := range opts.Envvars {
		env[k] = v
	}
	env["LLM_API_URL"] = a.config.LLMAPIURL
	env["LLM_API_KEY"] = a.config.LLMAPIKey
	env["LLM_MODEL"] = a.config.LLMModel

	// Proxy config for container (Docker Desktop uses host.docker.internal)
	// Docker Desktop auto-injects HTTP_PROXY=http://127.0.0.1:7890 which doesn't work inside containers.
	// We override all proxy vars to use host.docker.internal.
	// Also override no_proxy to remove host.docker.internal from the exclusion list.
	proxy := "http://host.docker.internal:7890"
	env["HTTP_PROXY"] = proxy
	env["HTTPS_PROXY"] = proxy
	env["http_proxy"] = proxy
	env["https_proxy"] = proxy
	env["no_proxy"] = "localhost,127.0.0.1,0.0.0.0"
	env["NO_PROXY"] = "localhost,127.0.0.1,0.0.0.0"

	// Defaults if not provided by orchestrator
	if env["TASK_ID"] == "" {
		env["TASK_ID"] = taskID
	}
	if env["RESULT_BRANCH"] == "" || strings.HasPrefix(env["RESULT_BRANCH"], env["BASE_BRANCH"]+"/") {
		env["RESULT_BRANCH"] = fmt.Sprintf("cap-agent/%s", taskID)
	}
	if env["BASE_BRANCH"] == "" {
		env["BASE_BRANCH"] = "main"
	}

	spec := ContainerSpec{
		Image:      a.config.WorkerImage,
		WorkingDir: "/workspace",
		Timeout:    30 * time.Minute,
		Env:        env,
	}
	if opts.Timeout > 0 {
		spec.Timeout = opts.Timeout
	}

	// Execute the cap-worker entrypoint inside the container
	cmd := ExecSpec{
		Cmd:     []string{"/usr/local/bin/entrypoint.sh"},
		Timeout: spec.Timeout,
		Env:     env,
	}

	result, err := a.sched.Run(ctx, spec, cmd)
	if err != nil {
		duration := time.Duration(0)
		if !result.FinishedAt.IsZero() && !result.StartedAt.IsZero() {
			duration = result.FinishedAt.Sub(result.StartedAt)
		}
		return &orchestrator.AgentResult{
			Summary:          fmt.Sprintf("execution failed: %v", err),
			ExecutionDuration: duration,
			Error:            err,
		}, err
	}

	agentResult := &orchestrator.AgentResult{
		Summary:          string(result.Stdout),
		TokensUsed:       0,
		ExecutionDuration: result.FinishedAt.Sub(result.StartedAt),
	}

	if result.ExitCode != 0 {
		agentResult.Error = fmt.Errorf("agent exited with code %d: %s", result.ExitCode, string(result.Stderr))
	}

	// Attach artifacts
	if opts.GitOptions != nil && opts.GitOptions.DoGitCommit && result.ExitCode == 0 {
		agentResult.Artifacts = []domain.ArtifactRef{
			{Type: "git_branch", URL: opts.GitOptions.ResultBranch},
		}
	}

	return agentResult, nil
}
