// Package scheduler - adapter to bridge scheduler to orchestrator's WorkerExecutor interface.
package scheduler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloud-agent-platform/cap/internal/domain"
	"github.com/cloud-agent-platform/cap/internal/domain/worker"
	"github.com/cloud-agent-platform/cap/internal/gitcontainer"
	"github.com/cloud-agent-platform/cap/internal/orchestrator"
)

// OrchestratorAdapter wraps a Scheduler to implement orchestrator.WorkerExecutor.
type OrchestratorAdapter struct {
	sched   Scheduler
	config  AdapterConfig
	gitMgr  *gitcontainer.Manager
}

// AdapterConfig holds LLM and image configuration.
type AdapterConfig struct {
	WorkerImage string
	LLMAPIURL   string
	LLMAPIKey   string
	LLMModel    string
	CAPAPIURL   string // Platform API URL for artifact reporting
}

// DefaultAdapterConfig returns sensible defaults.
func DefaultAdapterConfig() AdapterConfig {
	return AdapterConfig{
		WorkerImage: "cap-worker:latest",
		LLMAPIURL:   "https://api.minimaxi.com/v1/chat/completions",
		LLMAPIKey:   "", // Must be configured
		LLMModel:    "MiniMax-M2.7-highspeed",
		CAPAPIURL:   "http://host.docker.internal:18080",
	}
}

// NewOrchestratorAdapter creates a WorkerExecutor backed by the scheduler.
func NewOrchestratorAdapter(sched Scheduler, config AdapterConfig, gitMgr *gitcontainer.Manager) *OrchestratorAdapter {
	return &OrchestratorAdapter{sched: sched, config: config, gitMgr: gitMgr}
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
	env["CAP_API_URL"] = a.config.CAPAPIURL

	// Proxy config
	proxy := "http://host.docker.internal:7890"
	env["HTTP_PROXY"] = proxy
	env["HTTPS_PROXY"] = proxy
	env["http_proxy"] = proxy
	env["https_proxy"] = proxy
	env["no_proxy"] = "localhost,127.0.0.1,0.0.0.0"
	env["NO_PROXY"] = "localhost,127.0.0.1,0.0.0.0"

	// Defaults
	if env["TASK_ID"] == "" {
		env["TASK_ID"] = taskID
	}
	if env["RESULT_BRANCH"] == "" || strings.HasPrefix(env["RESULT_BRANCH"], env["BASE_BRANCH"]+"/") {
		env["RESULT_BRANCH"] = fmt.Sprintf("cap-agent/%s", taskID)
	}
	if env["BASE_BRANCH"] == "" {
		env["BASE_BRANCH"] = "main"
	}

	// ─── Git Container mode ──────────────────────────────────────────────────
	// If we have a Git container manager, create/lookup the project's Git container
	// and mount its volume into the worker.
	if a.gitMgr != nil && env["REPO_URL"] != "" {
		repoURL := env["REPO_URL"]
		branch := env["BASE_BRANCH"]

		gc, err := a.gitMgr.Ensure(ctx, repoURL, branch)
		if err != nil {
			// Git container failed — fall back to standalone mode
			fmt.Printf("[adapter] git container failed, falling back to standalone: %v\n", err)
		} else {
			// Tell worker to use Git Container mode
			env["GIT_CONTAINER_API"] = fmt.Sprintf("http://host.docker.internal:%d", gc.APIPort)
			env["PROJECT_ID"] = gc.ProjectID

			// Create worker with the Git container's volume mounted
			spec := ContainerSpec{
				Image:          a.config.WorkerImage,
				WorkingDir:     "/workspace",
				Timeout:        30 * time.Minute,
				Env:            env,
				VolumeHostPath: gc.VolumeDir, // Mount Git container's volume
			}
			if opts.Timeout > 0 {
				spec.Timeout = opts.Timeout
			}

			cmd := ExecSpec{
				Cmd:     []string{"/usr/local/bin/entrypoint.sh"},
				Timeout: spec.Timeout,
				Env:     env,
			}

			result, err := a.sched.Run(ctx, spec, cmd)
			if err != nil {
				return &orchestrator.AgentResult{
					Summary:           fmt.Sprintf("execution failed: %v", err),
					ExecutionDuration: 0,
					Error:             err,
				}, err
			}

			return a.buildResult(result, opts), nil
		}
	}

	// ─── Standalone mode (legacy / no Git container) ─────────────────────────
	spec := ContainerSpec{
		Image:      a.config.WorkerImage,
		WorkingDir: "/workspace",
		Timeout:    30 * time.Minute,
		Env:        env,
	}
	if opts.Timeout > 0 {
		spec.Timeout = opts.Timeout
	}

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
			Summary:           fmt.Sprintf("execution failed: %v", err),
			ExecutionDuration: duration,
			Error:             err,
		}, err
	}

	return a.buildResult(result, opts), nil
}

func (a *OrchestratorAdapter) buildResult(result ExecResult, opts worker.ExecOptions) *orchestrator.AgentResult {
	agentResult := &orchestrator.AgentResult{
		Summary:           string(result.Stdout),
		TokensUsed:        0,
		ExecutionDuration: result.FinishedAt.Sub(result.StartedAt),
	}

	if result.ExitCode != 0 {
		agentResult.Error = fmt.Errorf("agent exited with code %d: %s", result.ExitCode, string(result.Stderr))
	}

	if opts.GitOptions != nil && opts.GitOptions.DoGitCommit && result.ExitCode == 0 {
		agentResult.Artifacts = []domain.ArtifactRef{
			{Type: "git_branch", URL: opts.GitOptions.ResultBranch},
		}
	}

	return agentResult
}
