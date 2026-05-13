// Package react implements the ReAct (Reasoning + Acting) agent pattern.
package react

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// ----------------------------------------------------------------------------
// Tool Interface
// ----------------------------------------------------------------------------

// Tool defines the interface for tools that the ReAct agent can invoke.
// Each tool has a name, description, input schema, and an execute function.
type Tool interface {
	// Name returns the unique name of the tool.
	Name() string

	// Description returns a human-readable description of the tool.
	Description() string

	// InputSchema returns the JSON schema for the tool's input parameters.
	InputSchema() string

	// Execute executes the tool with the given input parameters.
	// It returns the result or an error if execution failed.
	Execute(ctx context.Context, input map[string]any) (any, error)
}

// ToolFunc is an adapter that allows using a function as a Tool.
// It is useful for simple tools that don't need state.
type ToolFunc struct {
	name        string
	description string
	inputSchema string
	fn          func(ctx context.Context, input map[string]any) (any, error)
}

// NewToolFunc creates a new ToolFunc with the given metadata and function.
func NewToolFunc(name, description, inputSchema string, fn func(ctx context.Context, input map[string]any) (any, error)) *ToolFunc {
	return &ToolFunc{
		name:        name,
		description: description,
		inputSchema: inputSchema,
		fn:          fn,
	}
}

// Name returns the tool name.
func (t *ToolFunc) Name() string { return t.name }

// Description returns the tool description.
func (t *ToolFunc) Description() string { return t.description }

// InputSchema returns the JSON schema for the tool's input.
func (t *ToolFunc) InputSchema() string { return t.inputSchema }

// Execute runs the wrapped function.
func (t *ToolFunc) Execute(ctx context.Context, input map[string]any) (any, error) {
	if t.fn == nil {
		return nil, fmt.Errorf("tool %s: function is nil", t.name)
	}
	return t.fn(ctx, input)
}

// Verify Tool interface is satisfied.
var _ Tool = (*ToolFunc)(nil)

// ----------------------------------------------------------------------------
// Tool Registry
// ----------------------------------------------------------------------------

// ToolRegistry manages available tools for the ReAct agent.
// It provides thread-safe registration and lookup of tools.
type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewToolRegistry creates a new ToolRegistry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry.
// If a tool with the same name already exists, it will be replaced.
func (r *ToolRegistry) Register(tool Tool) error {
	if tool == nil {
		return fmt.Errorf("cannot register nil tool")
	}
	if tool.Name() == "" {
		return fmt.Errorf("cannot register tool with empty name")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name()] = tool
	return nil
}

// RegisterFunc registers a function as a tool.
func (r *ToolRegistry) RegisterFunc(name, description, inputSchema string, fn func(ctx context.Context, input map[string]any) (any, error)) error {
	return r.Register(NewToolFunc(name, description, inputSchema, fn))
}

// Get retrieves a tool by name.
// Returns nil if the tool is not found.
func (r *ToolRegistry) Get(name string) Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.tools[name]
}

// List returns a list of all registered tools.
func (r *ToolRegistry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tools := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}
	return tools
}

// MustGet retrieves a tool by name and panics if not found.
func (r *ToolRegistry) MustGet(name string) Tool {
	tool := r.Get(name)
	if tool == nil {
		panic(fmt.Sprintf("tool not found: %s", name))
	}
	return tool
}

// Len returns the number of registered tools.
func (r *ToolRegistry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

// Clear removes all registered tools.
func (r *ToolRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools = make(map[string]Tool)
}

// Verify ToolRegistry satisfies ToolProvider interface.
var _ ToolProvider = (*ToolRegistry)(nil)

// ToolProvider is an interface for objects that provide tools to the ReAct agent.
type ToolProvider interface {
	// GetTool retrieves a tool by name.
	Get(name string) Tool
	// ListTools returns all available tools.
	List() []Tool
}

// ----------------------------------------------------------------------------
// Tool Result Serialization
// ----------------------------------------------------------------------------

// ToolResult represents the result of a tool execution for LLM consumption.
type ToolResult struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Result any    `json:"result"`
	Error  string `json:"error,omitempty"`
}

// NewToolResult creates a new ToolResult.
func NewToolResult(id, name string, result any, err error) *ToolResult {
	tr := &ToolResult{
		ID:     id,
		Name:   name,
		Result: result,
	}
	if err != nil {
		tr.Error = err.Error()
	}
	return tr
}

// FormatForLLM formats the tool result as a string for inclusion in the LLM prompt.
func (tr *ToolResult) FormatForLLM() string {
	if tr.Error != "" {
		return fmt.Sprintf("[TOOL=%s ID=%s ERROR=%s]", tr.Name, tr.ID, tr.Error)
	}
	return fmt.Sprintf("[TOOL=%s ID=%s RESULT=%s]", tr.Name, tr.ID, formatResult(tr.Result))
}

// formatResult formats a tool result for display.
func formatResult(v any) string {
	if v == nil {
		return "null"
	}
	switch val := v.(type) {
	case string:
		return val
	case error:
		return val.Error()
	default:
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprintf("%v", val)
		}
		return string(b)
	}
}

// ----------------------------------------------------------------------------
// Built-in Tool: Final Answer
// ----------------------------------------------------------------------------

// FinalAnswerTool is a special built-in tool that signals the agent has finished.
// The ReAct loop detects calls to this tool and returns the result.
type FinalAnswerTool struct{}

// NewFinalAnswerTool creates a new FinalAnswerTool.
func NewFinalAnswerTool() *FinalAnswerTool {
	return &FinalAnswerTool{}
}

// Name returns "final_answer".
func (t *FinalAnswerTool) Name() string {
	return "final_answer"
}

// Description returns "Use this tool to provide the final answer to the user's question.".
func (t *FinalAnswerTool) Description() string {
	return "Use this tool to provide the final answer to the user's question."
}

// InputSchema returns the JSON schema for the final_answer tool.
func (t *FinalAnswerTool) InputSchema() string {
	return `{"type":"object","properties":{"answer":{"type":"string","description":"The final answer to provide to the user"}},"required":["answer"]}`
}

// Execute stores the answer in a special result that signals completion.
func (t *FinalAnswerTool) Execute(ctx context.Context, input map[string]any) (any, error) {
	answer, ok := input["answer"].(string)
	if !ok {
		return nil, fmt.Errorf("final_answer: answer must be a string")
	}
	// Return a special marker to signal final answer
	return &FinalAnswer{Answer: answer}, nil
}

// FinalAnswer is a special result that signals the agent has completed.
type FinalAnswer struct {
	Answer string `json:"answer"`
}

// IsFinalAnswer checks if a result is a FinalAnswer.
func IsFinalAnswer(v any) bool {
	_, ok := v.(*FinalAnswer)
	return ok
}

// Verify Tool interface is satisfied.
var _ Tool = (*FinalAnswerTool)(nil)