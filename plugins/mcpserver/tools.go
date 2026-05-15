// Package mcpserver implements MCP Server with 9 Tools and 4 Resources.
// Exposes platform capabilities via Model Context Protocol.
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloud-agent-platform/cap/internal/mcp"
	"go.uber.org/zap"
)

// ToolHandler is a function that handles a tool call.
type ToolHandler func(ctx context.Context, params json.RawMessage) (*mcp.ToolCallResult, error)

// Tool represents an MCP tool definition with its handler.
type Tool struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	Handler     ToolHandler
}

// Tools returns all 9 MCP tools.
func Tools() []Tool {
	return []Tool{
		TaskSubmitTool(),
		TaskStatusTool(),
		TaskListTool(),
		TaskCancelTool(),
		TaskDecideTool(),
		TaskDiffTool(),
		TaskWaitTool(),
		AgentTemplatesTool(),
		PlatformStatusTool(),
	}
}

// TaskSubmitTool creates the task_submit tool.
// POST /api/v1/tasks
func TaskSubmitTool() Tool {
	return Tool{
		Name:        "task_submit",
		Description: "提交一个开发任务到云端 Agent 平台。平台会自动拆解任务、分配合适的 Agent 执行，完成后将代码改动推送到 Git 分支。",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"required": ["goal"],
			"properties": {
				"goal": {
					"type": "string",
					"description": "任务目标，一句话说清楚要做什么"
				},
				"repository": {
					"type": "object",
					"description": "Git 仓库信息",
					"properties": {
						"url": {"type": "string", "description": "Git 仓库 HTTPS 地址"},
						"branch": {"type": "string", "description": "基于哪个分支，默认 main"},
						"resultBranch": {"type": "string", "description": "结果写入哪个分支，默认自动生成"}
					}
				},
				"constraints": {
					"type": "array",
					"items": {"type": "string"},
					"description": "约束条件"
				},
				"verificationCriteria": {
					"type": "array",
					"items": {"type": "string"},
					"description": "验收标准"
				},
				"priority": {
					"type": "integer",
					"minimum": 0,
					"maximum": 9,
					"description": "优先级 0-9，默认 5"
				},
				"timeout": {
					"type": "integer",
					"description": "超时时间（秒），默认 1800（30分钟）"
				},
				"agentHint": {
					"type": "object",
					"description": "建议的 Agent 配置",
					"properties": {
						"templates": {
							"type": "array",
							"items": {"type": "string"},
							"description": "建议使用的 Agent 模板 ID"
						},
						"model": {"type": "string", "description": "建议使用的模型"},
						"maxAgents": {"type": "integer", "description": "最大并发 Agent 数"}
					}
				},
				"tags": {
					"type": "array",
					"items": {"type": "string"},
					"description": "任务标签"
				}
			}
		}`),
	}
}

// TaskStatusTool creates the task_status tool.
// GET /api/v1/tasks/{taskId}
func TaskStatusTool() Tool {
	return Tool{
		Name:        "task_status",
		Description: "查询任务状态。返回当前进度、子任务状态、Agent 产出摘要。任务完成后返回 Git 分支和改动文件列表。",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"required": ["taskId"],
			"properties": {
				"taskId": {"type": "string", "description": "任务 ID"},
				"includeLog": {"type": "boolean", "description": "是否包含最新 20 条 Agent 日志，默认 false"},
				"includeDiff": {"type": "boolean", "description": "是否包含 diff 摘要（仅已完成任务），默认 false"}
			}
		}`),
	}
}

// TaskListTool creates the task_list tool.
// GET /api/v1/tasks
func TaskListTool() Tool {
	return Tool{
		Name:        "task_list",
		Description: "列出最近的任务。可按状态、标签过滤。默认返回最近 20 个。",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"status": {
					"type": "string",
					"enum": ["pending", "dispatched", "running", "reviewing", "confirming", "completed", "failed", "cancelled"],
					"description": "按状态过滤"
				},
				"tags": {
					"type": "array",
					"items": {"type": "string"},
					"description": "按标签过滤"
				},
				"limit": {
					"type": "integer",
					"description": "返回数量，默认 20，最大 100"
				}
			}
		}`),
	}
}

// TaskCancelTool creates the task_cancel tool.
// POST /api/v1/tasks/{taskId}/cancel
func TaskCancelTool() Tool {
	return Tool{
		Name:        "task_cancel",
		Description: "取消任务。正在运行的 Agent 会被终止。",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"required": ["taskId", "reason"],
			"properties": {
				"taskId": {"type": "string", "description": "任务 ID"},
				"reason": {"type": "string", "description": "取消原因"}
			}
		}`),
	}
}

// TaskDecideTool creates the task_decide tool for approving/rejecting subtask decisions.
// POST /api/v1/tasks/{taskId}/subtasks/{subtaskId}/decision
func TaskDecideTool() Tool {
	return Tool{
		Name:        "task_decide",
		Description: "对 Agent 的高风险操作进行决策（批准或拒绝）。",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"required": ["taskId", "subtaskId", "decision"],
			"properties": {
				"taskId": {"type": "string", "description": "任务 ID"},
				"subtaskId": {"type": "string", "description": "子任务 ID"},
				"decision": {
					"type": "string",
					"enum": ["approve", "reject"],
					"description": "决策：approve（批准）或 reject（拒绝）"
				},
				"feedback": {"type": "string", "description": "反馈信息（拒绝时必填）"}
			}
		}`),
	}
}

// TaskDiffTool creates the task_diff tool for getting diff information.
// GET /api/v1/tasks/{taskId}/diff
func TaskDiffTool() Tool {
	return Tool{
		Name:        "task_diff",
		Description: "获取任务的代码改动（diff）。仅对已完成任务可用。",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"required": ["taskId"],
			"properties": {
				"taskId": {"type": "string", "description": "任务 ID"},
				"baseBranch": {"type": "string", "description": "对比的基准分支"},
				"includeUntracked": {"type": "boolean", "description": "是否包含未跟踪的文件，默认 true"}
			}
		}`),
	}
}

// TaskWaitTool creates the task_wait tool for polling task completion.
// GET /api/v1/tasks/{taskId} with polling until terminal state.
func TaskWaitTool() Tool {
	return Tool{
		Name:        "task_wait",
		Description: "等待任务完成。轮询任务状态直到达到终态（completed/failed/cancelled）或超时。",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"required": ["taskId"],
			"properties": {
				"taskId": {"type": "string", "description": "任务 ID"},
				"timeout": {
					"type": "integer",
					"description": "超时时间（秒），默认 3600（1小时）"
				},
				"pollInterval": {
					"type": "integer",
					"description": "轮询间隔（秒），默认 5"
				}
			}
		}`),
	}
}

// AgentTemplatesTool creates the agent_templates tool.
// GET /api/v1/agent-templates
func AgentTemplatesTool() Tool {
	return Tool{
		Name:        "agent_templates",
		Description: "列出平台可用的 Agent 模板及其能力。提交任务时可通过 agentHint 指定模板。",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"capability": {
					"type": "string",
					"enum": ["analysis", "coding", "review", "testing", "research"],
					"description": "按能力过滤"
				}
			}
		}`),
	}
}

// PlatformStatusTool creates the platform_status tool.
// GET /api/v1/platform/status
func PlatformStatusTool() Tool {
	return Tool{
		Name:        "platform_status",
		Description: "获取平台的实时状态，包括 Worker 池、队列、模型状态。",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {}
		}`),
	}
}

// ValidateToolParams validates parameters against tool schema.
func ValidateToolParams(toolName string, params json.RawMessage) error {
	tools := Tools()
	var schema json.RawMessage
	for _, tool := range tools {
		if tool.Name == toolName {
			schema = tool.InputSchema
			break
		}
	}
	if schema == nil {
		return fmt.Errorf("unknown tool: %s", toolName)
	}

	if params == nil || string(params) == "null" {
		// Check if required fields exist
		var schemaMap map[string]any
		if err := json.Unmarshal(schema, &schemaMap); err != nil {
			return err
		}
		if required, ok := schemaMap["required"]; ok {
			if reqArr, ok := required.([]any); ok && len(reqArr) > 0 {
				return fmt.Errorf("missing required parameters")
			}
		}
		return nil
	}

	// Basic JSON schema validation
	var paramsMap map[string]any
	if err := json.Unmarshal(params, &paramsMap); err != nil {
		return fmt.Errorf("invalid JSON in parameters: %w", err)
	}

	// Validate required fields
	var schemaMap map[string]any
	if err := json.Unmarshal(schema, &schemaMap); err != nil {
		return err
	}

	if required, ok := schemaMap["required"].([]any); ok {
		for _, r := range required {
			if rStr, ok := r.(string); ok {
				if _, exists := paramsMap[rStr]; !exists {
					return fmt.Errorf("missing required parameter: %s", rStr)
				}
			}
		}
	}

	// Validate enum fields
	if props, ok := schemaMap["properties"].(map[string]any); ok {
		for field, prop := range props {
			if propMap, ok := prop.(map[string]any); ok {
				if enumVals, ok := propMap["enum"].([]any); ok && len(enumVals) > 0 {
					if val, exists := paramsMap[field]; exists {
						valid := false
						for _, e := range enumVals {
							if e == val {
								valid = true
								break
							}
						}
						if !valid {
							return fmt.Errorf("invalid value for %s: must be one of %v", field, enumVals)
						}
					}
				}
			}
		}
	}

	return nil
}

// TaskExecutor is the interface for executing platform API calls.
type TaskExecutor interface {
	SubmitTask(ctx context.Context, req mcp.TaskSubmitRequest) (*mcp.TaskSubmitResponse, error)
	GetTask(ctx context.Context, taskID string) (*mcp.TaskStatusResponse, error)
	ListTasks(ctx context.Context, status string, tags []string, limit int) (*mcp.TaskListResponse, error)
	CancelTask(ctx context.Context, taskID, reason string) (*mcp.CancelTaskResponse, error)
	DecideTask(ctx context.Context, taskID, subtaskID, decision, feedback string) (*mcp.DecideResponse, error)
	GetDiff(ctx context.Context, taskID string) (*mcp.DiffResponse, error)
	ListAgentTemplates(ctx context.Context) ([]mcp.AgentTemplateResponse, error)
	GetPlatformStatus(ctx context.Context) (*mcp.PlatformStatusResponse, error)
}

// PlatformCaller implements tool handlers using REST API calls.
type PlatformCaller struct {
	client *mcp.PlatformClient
	logger *zap.Logger
}

// NewPlatformCaller creates a new PlatformCaller.
func NewPlatformCaller(client *mcp.PlatformClient, logger *zap.Logger) *PlatformCaller {
	return &PlatformCaller{
		client: client,
		logger: logger,
	}
}

// HandleTaskSubmit handles task_submit tool.
func (p *PlatformCaller) HandleTaskSubmit(ctx context.Context, params json.RawMessage) (*mcp.ToolCallResult, error) {
	var input struct {
		Goal                 string               `json:"goal"`
		Repository           *mcp.RepositoryInput `json:"repository,omitempty"`
		Constraints          []string             `json:"constraints,omitempty"`
		VerificationCriteria []string             `json:"verificationCriteria,omitempty"`
		Priority             int                  `json:"priority,omitempty"`
		Timeout              int                  `json:"timeout,omitempty"`
		AgentHint            *mcp.AgentHintInput  `json:"agentHint,omitempty"`
		Tags                 []string             `json:"tags,omitempty"`
	}

	if err := json.Unmarshal(params, &input); err != nil {
		return &mcp.ToolCallResult{
			Content: []mcp.ContentBlock{{Type: "text", Text: fmt.Sprintf("Failed to parse parameters: %v", err)}},
			IsError: true,
		}, nil
	}

	req := mcp.TaskSubmitRequest{
		Goal:                 input.Goal,
		Repository:           input.Repository,
		Constraints:          input.Constraints,
		VerificationCriteria: input.VerificationCriteria,
		Priority:             input.Priority,
		Timeout:              input.Timeout,
		AgentHint:            input.AgentHint,
		Tags:                 input.Tags,
	}

	result, err := p.client.SubmitTask(ctx, req)
	if err != nil {
		p.logger.Error("task_submit failed",
			zap.String("layer", "MCP"),
			zap.Error(err),
		)
		return &mcp.ToolCallResult{
			Content: []mcp.ContentBlock{{Type: "text", Text: fmt.Sprintf("Failed to submit task: %v", err)}},
			IsError: true,
		}, nil
	}

	output, _ := json.Marshal(result)
	return &mcp.ToolCallResult{
		Content: []mcp.ContentBlock{{Type: "text", Text: string(output)}},
	}, nil
}

// HandleTaskStatus handles task_status tool.
func (p *PlatformCaller) HandleTaskStatus(ctx context.Context, params json.RawMessage) (*mcp.ToolCallResult, error) {
	var input struct {
		TaskID      string `json:"taskId"`
		IncludeLog  bool   `json:"includeLog,omitempty"`
		IncludeDiff bool   `json:"includeDiff,omitempty"`
	}

	if err := json.Unmarshal(params, &input); err != nil {
		return &mcp.ToolCallResult{
			Content: []mcp.ContentBlock{{Type: "text", Text: fmt.Sprintf("Failed to parse parameters: %v", err)}},
			IsError: true,
		}, nil
	}

	result, err := p.client.GetTask(ctx, input.TaskID)
	if err != nil {
		p.logger.Error("task_status failed",
			zap.String("layer", "MCP"),
			zap.String("task_id", input.TaskID),
			zap.Error(err),
		)
		return &mcp.ToolCallResult{
			Content: []mcp.ContentBlock{{Type: "text", Text: fmt.Sprintf("Failed to get task status: %v", err)}},
			IsError: true,
		}, nil
	}

	output, _ := json.Marshal(result)
	return &mcp.ToolCallResult{
		Content: []mcp.ContentBlock{{Type: "text", Text: string(output)}},
	}, nil
}

// HandleTaskList handles task_list tool.
func (p *PlatformCaller) HandleTaskList(ctx context.Context, params json.RawMessage) (*mcp.ToolCallResult, error) {
	var input struct {
		Status string   `json:"status,omitempty"`
		Tags   []string `json:"tags,omitempty"`
		Limit  int      `json:"limit,omitempty"`
	}

	if err := json.Unmarshal(params, &input); err != nil {
		return &mcp.ToolCallResult{
			Content: []mcp.ContentBlock{{Type: "text", Text: fmt.Sprintf("Failed to parse parameters: %v", err)}},
			IsError: true,
		}, nil
	}

	if input.Limit == 0 {
		input.Limit = 20
	}

	result, err := p.client.ListTasks(ctx, input.Status, input.Tags, input.Limit)
	if err != nil {
		p.logger.Error("task_list failed",
			zap.String("layer", "MCP"),
			zap.Error(err),
		)
		return &mcp.ToolCallResult{
			Content: []mcp.ContentBlock{{Type: "text", Text: fmt.Sprintf("Failed to list tasks: %v", err)}},
			IsError: true,
		}, nil
	}

	output, _ := json.Marshal(result)
	return &mcp.ToolCallResult{
		Content: []mcp.ContentBlock{{Type: "text", Text: string(output)}},
	}, nil
}

// HandleTaskCancel handles task_cancel tool.
func (p *PlatformCaller) HandleTaskCancel(ctx context.Context, params json.RawMessage) (*mcp.ToolCallResult, error) {
	var input struct {
		TaskID string `json:"taskId"`
		Reason string `json:"reason"`
	}

	if err := json.Unmarshal(params, &input); err != nil {
		return &mcp.ToolCallResult{
			Content: []mcp.ContentBlock{{Type: "text", Text: fmt.Sprintf("Failed to parse parameters: %v", err)}},
			IsError: true,
		}, nil
	}

	result, err := p.client.CancelTask(ctx, input.TaskID, input.Reason)
	if err != nil {
		p.logger.Error("task_cancel failed",
			zap.String("layer", "MCP"),
			zap.String("task_id", input.TaskID),
			zap.Error(err),
		)
		return &mcp.ToolCallResult{
			Content: []mcp.ContentBlock{{Type: "text", Text: fmt.Sprintf("Failed to cancel task: %v", err)}},
			IsError: true,
		}, nil
	}

	output, _ := json.Marshal(result)
	return &mcp.ToolCallResult{
		Content: []mcp.ContentBlock{{Type: "text", Text: string(output)}},
	}, nil
}

// HandleTaskDecide handles task_decide tool.
func (p *PlatformCaller) HandleTaskDecide(ctx context.Context, params json.RawMessage) (*mcp.ToolCallResult, error) {
	var input struct {
		TaskID    string `json:"taskId"`
		SubtaskID string `json:"subtaskId"`
		Decision  string `json:"decision"`
		Feedback  string `json:"feedback,omitempty"`
	}

	if err := json.Unmarshal(params, &input); err != nil {
		return &mcp.ToolCallResult{
			Content: []mcp.ContentBlock{{Type: "text", Text: fmt.Sprintf("Failed to parse parameters: %v", err)}},
			IsError: true,
		}, nil
	}

	result, err := p.client.DecideTask(ctx, input.TaskID, input.SubtaskID, input.Decision, input.Feedback)
	if err != nil {
		p.logger.Error("task_decide failed",
			zap.String("layer", "MCP"),
			zap.String("task_id", input.TaskID),
			zap.String("subtask_id", input.SubtaskID),
			zap.Error(err),
		)
		return &mcp.ToolCallResult{
			Content: []mcp.ContentBlock{{Type: "text", Text: fmt.Sprintf("Failed to decide task: %v", err)}},
			IsError: true,
		}, nil
	}

	output, _ := json.Marshal(result)
	return &mcp.ToolCallResult{
		Content: []mcp.ContentBlock{{Type: "text", Text: string(output)}},
	}, nil
}

// HandleTaskDiff handles task_diff tool.
func (p *PlatformCaller) HandleTaskDiff(ctx context.Context, params json.RawMessage) (*mcp.ToolCallResult, error) {
	var input struct {
		TaskID           string `json:"taskId"`
		BaseBranch       string `json:"baseBranch,omitempty"`
		IncludeUntracked bool   `json:"includeUntracked,omitempty"`
	}

	if err := json.Unmarshal(params, &input); err != nil {
		return &mcp.ToolCallResult{
			Content: []mcp.ContentBlock{{Type: "text", Text: fmt.Sprintf("Failed to parse parameters: %v", err)}},
			IsError: true,
		}, nil
	}

	result, err := p.client.GetDiff(ctx, input.TaskID)
	if err != nil {
		p.logger.Error("task_diff failed",
			zap.String("layer", "MCP"),
			zap.String("task_id", input.TaskID),
			zap.Error(err),
		)
		return &mcp.ToolCallResult{
			Content: []mcp.ContentBlock{{Type: "text", Text: fmt.Sprintf("Failed to get task diff: %v", err)}},
			IsError: true,
		}, nil
	}

	output, _ := json.Marshal(result)
	return &mcp.ToolCallResult{
		Content: []mcp.ContentBlock{{Type: "text", Text: string(output)}},
	}, nil
}

// HandleTaskWait handles task_wait tool with polling.
func (p *PlatformCaller) HandleTaskWait(ctx context.Context, params json.RawMessage) (*mcp.ToolCallResult, error) {
	var input struct {
		TaskID       string `json:"taskId"`
		Timeout      int    `json:"timeout,omitempty"`
		PollInterval int    `json:"pollInterval,omitempty"`
	}

	if err := json.Unmarshal(params, &input); err != nil {
		return &mcp.ToolCallResult{
			Content: []mcp.ContentBlock{{Type: "text", Text: fmt.Sprintf("Failed to parse parameters: %v", err)}},
			IsError: true,
		}, nil
	}

	timeout := time.Duration(input.Timeout)
	if timeout == 0 {
		timeout = 3600 * time.Second // default 1 hour
	}

	pollInterval := time.Duration(input.PollInterval)
	if pollInterval == 0 {
		pollInterval = 5 * time.Second // default 5 seconds
	}

	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return &mcp.ToolCallResult{
				Content: []mcp.ContentBlock{{Type: "text", Text: "Task wait cancelled"}},
				IsError: true,
			}, nil
		default:
		}

		result, err := p.client.GetTask(ctx, input.TaskID)
		if err != nil {
			p.logger.Error("task_wait polling failed",
				zap.String("layer", "MCP"),
				zap.String("task_id", input.TaskID),
				zap.Error(err),
			)
			time.Sleep(pollInterval)
			continue
		}

		// Check if task reached terminal state
		if result.Status == "completed" || result.Status == "failed" || result.Status == "cancelled" {
			output, _ := json.Marshal(result)
			return &mcp.ToolCallResult{
				Content: []mcp.ContentBlock{{Type: "text", Text: string(output)}},
			}, nil
		}

		p.logger.Debug("task_wait polling",
			zap.String("layer", "MCP"),
			zap.String("task_id", input.TaskID),
			zap.String("status", result.Status),
		)

		time.Sleep(pollInterval)
	}

	// Timeout reached
	return &mcp.ToolCallResult{
		Content: []mcp.ContentBlock{{Type: "text", Text: fmt.Sprintf("Task wait timeout after %v", timeout)}},
		IsError: true,
	}, nil
}

// HandleAgentTemplates handles agent_templates tool.
func (p *PlatformCaller) HandleAgentTemplates(ctx context.Context, params json.RawMessage) (*mcp.ToolCallResult, error) {
	var input struct {
		Capability string `json:"capability,omitempty"`
	}

	if err := json.Unmarshal(params, &input); err != nil {
		return &mcp.ToolCallResult{
			Content: []mcp.ContentBlock{{Type: "text", Text: fmt.Sprintf("Failed to parse parameters: %v", err)}},
			IsError: true,
		}, nil
	}

	result, err := p.client.ListAgentTemplates(ctx)
	if err != nil {
		p.logger.Error("agent_templates failed",
			zap.String("layer", "MCP"),
			zap.Error(err),
		)
		return &mcp.ToolCallResult{
			Content: []mcp.ContentBlock{{Type: "text", Text: fmt.Sprintf("Failed to list agent templates: %v", err)}},
			IsError: true,
		}, nil
	}

	// Filter by capability if specified
	if input.Capability != "" {
		filtered := make([]mcp.AgentTemplateResponse, 0)
		for _, t := range result {
			if capVal, ok := t.Capabilities[input.Capability]; ok && capVal > 0 {
				filtered = append(filtered, t)
			}
		}
		result = filtered
	}

	output, _ := json.Marshal(result)
	return &mcp.ToolCallResult{
		Content: []mcp.ContentBlock{{Type: "text", Text: string(output)}},
	}, nil
}

// HandlePlatformStatus handles platform_status tool.
func (p *PlatformCaller) HandlePlatformStatus(ctx context.Context, params json.RawMessage) (*mcp.ToolCallResult, error) {
	result, err := p.client.GetPlatformStatus(ctx)
	if err != nil {
		p.logger.Error("platform_status failed",
			zap.String("layer", "MCP"),
			zap.Error(err),
		)
		return &mcp.ToolCallResult{
			Content: []mcp.ContentBlock{{Type: "text", Text: fmt.Sprintf("Failed to get platform status: %v", err)}},
			IsError: true,
		}, nil
	}

	output, _ := json.Marshal(result)
	return &mcp.ToolCallResult{
		Content: []mcp.ContentBlock{{Type: "text", Text: string(output)}},
	}, nil
}
