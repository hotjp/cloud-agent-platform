// Package mcp implements the MCP (Model Context Protocol) server for the Cloud Agent Platform.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"
)

// ToolExecutor handles the execution of MCP tools.
type ToolExecutor struct {
	client *PlatformClient
	logger *zap.Logger
}

// NewToolExecutor creates a new tool executor.
func NewToolExecutor(client *PlatformClient, logger *zap.Logger) *ToolExecutor {
	return &ToolExecutor{
		client: client,
		logger: logger,
	}
}

// Execute runs a tool with the given parameters.
func (e *ToolExecutor) Execute(ctx context.Context, name string, params json.RawMessage) (*ToolCallResult, error) {
	e.logger.Info("executing MCP tool",
		zap.String("tool", name),
		zap.String("layer", "MCP"),
	)

	// Validate parameters
	if err := ValidateToolParams(name, params); err != nil {
		return &ToolCallResult{
			Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Invalid parameters: %v", err)}},
			IsError: true,
		}, nil
	}

	switch name {
	case "task_submit":
		return e.submitTask(ctx, params)
	case "task_status":
		return e.getTaskStatus(ctx, params)
	case "task_list":
		return e.listTasks(ctx, params)
	case "task_cancel":
		return e.cancelTask(ctx, params)
	case "context_approve":
		return e.approveContext(ctx, params)
	case "context_reject":
		return e.rejectContext(ctx, params)
	case "agent_list":
		return e.listAgents(ctx, params)
	case "session_list":
		return e.listSessions(ctx)
	default:
		return &ToolCallResult{
			Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Unknown tool: %s", name)}},
			IsError: true,
		}, nil
	}
}

// submitTask handles task_submit tool.
func (e *ToolExecutor) submitTask(ctx context.Context, params json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		Goal                 string             `json:"goal"`
		Repository          *RepositoryInput   `json:"repository,omitempty"`
		Constraints         []string           `json:"constraints,omitempty"`
		VerificationCriteria []string           `json:"verificationCriteria,omitempty"`
		Priority            int                `json:"priority,omitempty"`
		Timeout             int                `json:"timeout,omitempty"`
		AgentHint           *AgentHintInput    `json:"agentHint,omitempty"`
		Tags                []string           `json:"tags,omitempty"`
	}

	if err := json.Unmarshal(params, &input); err != nil {
		return &ToolCallResult{
			Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Failed to parse parameters: %v", err)}},
			IsError: true,
		}, nil
	}

	req := TaskSubmitRequest{
		Goal:                  input.Goal,
		Repository:            input.Repository,
		Constraints:          input.Constraints,
		VerificationCriteria: input.VerificationCriteria,
		Priority:             input.Priority,
		Timeout:              input.Timeout,
		AgentHint:            input.AgentHint,
		Tags:                 input.Tags,
	}

	result, err := e.client.SubmitTask(ctx, req)
	if err != nil {
		e.logger.Error("task_submit failed",
			zap.String("layer", "MCP"),
			zap.Error(err),
		)
		return &ToolCallResult{
			Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Failed to submit task: %v", err)}},
			IsError: true,
		}, nil
	}

	output, _ := json.Marshal(result)
	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: string(output)}},
	}, nil
}

// getTaskStatus handles task_status tool.
func (e *ToolExecutor) getTaskStatus(ctx context.Context, params json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		TaskID      string `json:"taskId"`
		IncludeLog  bool   `json:"includeLog,omitempty"`
		IncludeDiff bool   `json:"includeDiff,omitempty"`
	}

	if err := json.Unmarshal(params, &input); err != nil {
		return &ToolCallResult{
			Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Failed to parse parameters: %v", err)}},
			IsError: true,
		}, nil
	}

	result, err := e.client.GetTask(ctx, input.TaskID)
	if err != nil {
		e.logger.Error("task_status failed",
			zap.String("layer", "MCP"),
			zap.String("task_id", input.TaskID),
			zap.Error(err),
		)
		return &ToolCallResult{
			Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Failed to get task status: %v", err)}},
			IsError: true,
		}, nil
	}

	output, _ := json.Marshal(result)
	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: string(output)}},
	}, nil
}

// listTasks handles task_list tool.
func (e *ToolExecutor) listTasks(ctx context.Context, params json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		Status string   `json:"status,omitempty"`
		Tags   []string `json:"tags,omitempty"`
		Limit  int      `json:"limit,omitempty"`
	}

	if err := json.Unmarshal(params, &input); err != nil {
		return &ToolCallResult{
			Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Failed to parse parameters: %v", err)}},
			IsError: true,
		}, nil
	}

	if input.Limit == 0 {
		input.Limit = 20
	}

	result, err := e.client.ListTasks(ctx, input.Status, input.Tags, input.Limit)
	if err != nil {
		e.logger.Error("task_list failed",
			zap.String("layer", "MCP"),
			zap.Error(err),
		)
		return &ToolCallResult{
			Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Failed to list tasks: %v", err)}},
			IsError: true,
		}, nil
	}

	output, _ := json.Marshal(result)
	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: string(output)}},
	}, nil
}

// cancelTask handles task_cancel tool.
func (e *ToolExecutor) cancelTask(ctx context.Context, params json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		TaskID string `json:"taskId"`
		Reason string `json:"reason"`
	}

	if err := json.Unmarshal(params, &input); err != nil {
		return &ToolCallResult{
			Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Failed to parse parameters: %v", err)}},
			IsError: true,
		}, nil
	}

	result, err := e.client.CancelTask(ctx, input.TaskID, input.Reason)
	if err != nil {
		e.logger.Error("task_cancel failed",
			zap.String("layer", "MCP"),
			zap.String("task_id", input.TaskID),
			zap.Error(err),
		)
		return &ToolCallResult{
			Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Failed to cancel task: %v", err)}},
			IsError: true,
		}, nil
	}

	output, _ := json.Marshal(result)
	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: string(output)}},
	}, nil
}

// approveContext handles context_approve tool.
func (e *ToolExecutor) approveContext(ctx context.Context, params json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		TaskID    string `json:"taskId"`
		SubtaskID string `json:"subtaskId"`
		Feedback  string `json:"feedback,omitempty"`
	}

	if err := json.Unmarshal(params, &input); err != nil {
		return &ToolCallResult{
			Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Failed to parse parameters: %v", err)}},
			IsError: true,
		}, nil
	}

	result, err := e.client.DecideTask(ctx, input.TaskID, input.SubtaskID, "approve", input.Feedback)
	if err != nil {
		e.logger.Error("context_approve failed",
			zap.String("layer", "MCP"),
			zap.String("task_id", input.TaskID),
			zap.String("subtask_id", input.SubtaskID),
			zap.Error(err),
		)
		return &ToolCallResult{
			Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Failed to approve context: %v", err)}},
			IsError: true,
		}, nil
	}

	output, _ := json.Marshal(result)
	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: string(output)}},
	}, nil
}

// rejectContext handles context_reject tool.
func (e *ToolExecutor) rejectContext(ctx context.Context, params json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		TaskID    string `json:"taskId"`
		SubtaskID string `json:"subtaskId"`
		Feedback  string `json:"feedback"`
	}

	if err := json.Unmarshal(params, &input); err != nil {
		return &ToolCallResult{
			Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Failed to parse parameters: %v", err)}},
			IsError: true,
		}, nil
	}

	result, err := e.client.DecideTask(ctx, input.TaskID, input.SubtaskID, "reject", input.Feedback)
	if err != nil {
		e.logger.Error("context_reject failed",
			zap.String("layer", "MCP"),
			zap.String("task_id", input.TaskID),
			zap.String("subtask_id", input.SubtaskID),
			zap.Error(err),
		)
		return &ToolCallResult{
			Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Failed to reject context: %v", err)}},
			IsError: true,
		}, nil
	}

	output, _ := json.Marshal(result)
	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: string(output)}},
	}, nil
}

// listAgents handles agent_list tool.
func (e *ToolExecutor) listAgents(ctx context.Context, params json.RawMessage) (*ToolCallResult, error) {
	var input struct {
		Capability string `json:"capability,omitempty"`
	}

	if err := json.Unmarshal(params, &input); err != nil {
		return &ToolCallResult{
			Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Failed to parse parameters: %v", err)}},
			IsError: true,
		}, nil
	}

	result, err := e.client.ListAgentTemplates(ctx)
	if err != nil {
		e.logger.Error("agent_list failed",
			zap.String("layer", "MCP"),
			zap.Error(err),
		)
		return &ToolCallResult{
			Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Failed to list agents: %v", err)}},
			IsError: true,
		}, nil
	}

	// Filter by capability if specified
	if input.Capability != "" {
		filtered := make([]AgentTemplateResponse, 0)
		for _, t := range result {
			if capVal, ok := t.Capabilities[input.Capability]; ok && capVal > 0 {
				filtered = append(filtered, t)
			}
		}
		result = filtered
	}

	output, _ := json.Marshal(result)
	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: string(output)}},
	}, nil
}

// listSessions handles session_list tool.
func (e *ToolExecutor) listSessions(ctx context.Context) (*ToolCallResult, error) {
	result, err := e.client.ListSessions(ctx)
	if err != nil {
		e.logger.Error("session_list failed",
			zap.String("layer", "MCP"),
			zap.Error(err),
		)
		return &ToolCallResult{
			Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("Failed to list sessions: %v", err)}},
			IsError: true,
		}, nil
	}

	output, _ := json.Marshal(result)
	return &ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: string(output)}},
	}, nil
}
