// Package mcp implements the MCP (Model Context Protocol) server for the Cloud Agent Platform.
package mcp

import (
	"encoding/json"
	"fmt"
)

// toolSchemas defines the input schemas for all 9 MCP tools.
// These schemas must match the interface contract exactly.
var toolSchemas = map[string]json.RawMessage{
	"task_submit": json.RawMessage(`{
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
				"description": "约束条件，如 ['禁止 mock 数据', '最小改动原则']"
			},
			"verificationCriteria": {
				"type": "array",
				"items": {"type": "string"},
				"description": "验收标准，如 ['所有测试通过', '类型检查无错误']"
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
				"description": "任务标签，用于分类和检索"
			}
		}
	}`),

	"task_status": json.RawMessage(`{
		"type": "object",
		"required": ["taskId"],
		"properties": {
			"taskId": {"type": "string", "description": "任务 ID"},
			"includeLog": {"type": "boolean", "description": "是否包含最新 20 条 Agent 日志，默认 false"},
			"includeDiff": {"type": "boolean", "description": "是否包含 diff 摘要（仅已完成任务），默认 false"}
		}
	}`),

	"task_list": json.RawMessage(`{
		"type": "object",
		"properties": {
			"status": {
				"type": "string",
				"enum": ["pending", "running", "completed", "failed", "cancelled"],
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

	"task_cancel": json.RawMessage(`{
		"type": "object",
		"required": ["taskId", "reason"],
		"properties": {
			"taskId": {"type": "string", "description": "任务 ID"},
			"reason": {"type": "string", "description": "取消原因"}
		}
	}`),

	"task_decompose": json.RawMessage(`{
		"type": "object",
		"required": ["taskId"],
		"properties": {
			"taskId": {"type": "string", "description": "任务 ID"}
		}
	}`),

	"context_approve": json.RawMessage(`{
		"type": "object",
		"required": ["taskId", "subtaskId"],
		"properties": {
			"taskId": {"type": "string", "description": "任务 ID"},
			"subtaskId": {"type": "string", "description": "子任务 ID"},
			"feedback": {"type": "string", "description": "可选的反馈信息"}
		}
	}`),

	"context_reject": json.RawMessage(`{
		"type": "object",
		"required": ["taskId", "subtaskId", "feedback"],
		"properties": {
			"taskId": {"type": "string", "description": "任务 ID"},
			"subtaskId": {"type": "string", "description": "子任务 ID"},
			"feedback": {"type": "string", "description": "拒绝原因（必填）"}
		}
	}`),

	"agent_list": json.RawMessage(`{
		"type": "object",
		"properties": {
			"capability": {
				"type": "string",
				"enum": ["analysis", "coding", "review", "testing", "research"],
				"description": "按能力过滤"
			}
		}
	}`),

	"session_list": json.RawMessage(`{
		"type": "object",
		"properties": {}
	}`),
}

// toolDescriptions provides human-readable descriptions for all 9 tools.
var toolDescriptions = map[string]string{
	"task_submit":     "提交一个开发任务到云端 Agent 平台。平台会自动拆解任务、分配合适的 Agent 执行，完成后将代码改动推送到 Git 分支。",
	"task_status":     "查询任务状态。返回当前进度、子任务状态、Agent 产出摘要。任务完成后返回 Git 分支和改动文件列表。",
	"task_list":       "列出最近的任务。可按状态、标签过滤。默认返回最近 20 个。",
	"task_cancel":     "取消任务。正在运行的 Agent 会被终止。",
	"task_decompose":  "对任务进行拆解，返回子任务列表。",
	"context_approve":  "批准 Agent 的高风险操作。当 Agent 检测到高风险改动时，会暂停等待确认。",
	"context_reject":   "拒绝 Agent 的高风险操作。",
	"agent_list":      "列出平台可用的 Agent 模板及其能力。提交任务时可通过 agentHint 指定模板。",
	"session_list":    "列出当前会话列表。",
}

// GetToolDefinitions returns all MCP tool definitions.
func GetToolDefinitions() []Tool {
	tools := make([]Tool, 0, len(toolSchemas))
	for name, schema := range toolSchemas {
		tools = append(tools, Tool{
			Name:        name,
			Description: toolDescriptions[name],
			InputSchema: schema,
		})
	}
	return tools
}

// ValidateToolParams validates the parameters for a tool call.
func ValidateToolParams(toolName string, params json.RawMessage) error {
	schema, ok := toolSchemas[toolName]
	if !ok {
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
				return fmt.Errorf("missing required parameters: %v", reqArr)
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
