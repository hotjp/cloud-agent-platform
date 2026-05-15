// Package tools implements the tool set available to agents.
package tools

import (
	"context"
	"time"

	"github.com/cloud-agent-platform/cap/internal/agent/react"
	"github.com/cloud-agent-platform/cap/internal/domain"
	"go.uber.org/zap"
)

// ----------------------------------------------------------------------------
// LLMCall Tool
// ----------------------------------------------------------------------------

// LLMCall invokes an LLM to generate text.
type LLMCall struct {
	toolBase
	llm    react.LLM
	logger *zap.Logger
}

const llmCallSchema = `{
	"type": "object",
	"properties": {
		"prompt": {
			"type": "string",
			"description": "The prompt to send to the LLM"
		},
		"system": {
			"type": "string",
			"description": "Optional system message to set context"
		},
		"model": {
			"type": "string",
			"description": "The model to use (optional, uses default if not specified)"
		},
		"temperature": {
			"type": "number",
			"description": "Temperature for generation (0.0-2.0, default: 0.7)"
		},
		"max_tokens": {
			"type": "integer",
			"description": "Maximum tokens to generate (default: 4096)"
		}
	},
	"required": ["prompt"]
}`

// NewLLMCall creates a new LLMCall tool.
func NewLLMCall(llm react.LLM, logger *zap.Logger) *LLMCall {
	if logger == nil {
		logger, _ = zap.NewProduction()
	}
	return &LLMCall{
		toolBase: toolBase{
			name:        "LLMCall",
			description: "Calls a Language Model to generate text based on a prompt",
			inputSchema: llmCallSchema,
		},
		llm:    llm,
		logger: logger,
	}
}

// Execute calls the LLM.
func (t *LLMCall) Execute(ctx context.Context, input map[string]any) (*ToolResult, error) {
	start := time.Now()

	prompt, ok := input["prompt"].(string)
	if !ok || prompt == "" {
		return &ToolResult{Success: false, Error: "prompt is required and must be a string"}, nil
	}

	system := ""
	if s, ok := input["system"].(string); ok {
		system = s
	}

	model := ""
	if m, ok := input["model"].(string); ok {
		model = m
	}

	temperature := 0.7
	if temp, ok := input["temperature"].(float64); ok {
		temperature = temp
	}

	maxTokens := 4096
	if mt, ok := input["max_tokens"].(float64); ok {
		maxTokens = int(mt)
	}

	// Build messages
	messages := make([]*react.Message, 0, 2)
	if system != "" {
		messages = append(messages, &react.Message{
			Role:    react.RoleSystem,
			Content: system,
		})
	}
	messages = append(messages, &react.Message{
		Role:    react.RoleUser,
		Content: prompt,
	})

	opts := &react.GenerateOptions{
		Model:       model,
		Temperature: temperature,
		MaxTokens:   maxTokens,
	}

	t.logger.Info("calling LLM",
		zap.String("model", model),
		zap.Int("max_tokens", maxTokens),
		zap.Float64("temperature", temperature),
	)

	result, err := t.llm.Generate(ctx, messages, opts)
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   domain.NewL4LLMInvokeError(model, err).Error(),
			Meta:    ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
		}, nil
	}

	return &ToolResult{
		Success: true,
		Output: LLMResult{
			Content:      result.Content,
			StopReason:   result.StopReason,
			TotalTokens:  result.TotalTokens,
			PromptTokens: result.PromptTokens,
		},
		Meta: ResultMeta{DurationMS: time.Since(start).Milliseconds(), ToolName: t.Name()},
	}, nil
}

// LLMResult represents the result of an LLM call.
type LLMResult struct {
	Content      string `json:"content"`
	StopReason   string `json:"stop_reason"`
	TotalTokens  int    `json:"total_tokens"`
	PromptTokens int    `json:"prompt_tokens"`
}

// Verify tool interface
var _ Tool = (*LLMCall)(nil)
