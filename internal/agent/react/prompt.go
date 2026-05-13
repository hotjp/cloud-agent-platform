// Package react implements the ReAct (Reasoning + Acting) agent pattern.
package react

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
)

// ----------------------------------------------------------------------------
// Prompt Templates
// ----------------------------------------------------------------------------

// DefaultSystemPrompt is the default system prompt for the ReAct agent.
const DefaultSystemPrompt = `You are a helpful AI assistant that uses a structured reasoning approach.

You have access to the following tools:
{{TOOLS}}

When solving a problem:
1. THINK: Analyze the problem and plan your approach
2. ACT: Use a tool to gather information or perform an action
3. OBSERVE: Review the result of your action
4. REPEAT until you have enough information to provide a final answer
5. FINAL_ANSWER: Provide your final response using the final_answer tool

Important guidelines:
- Always think step by step before taking action
- Use tools when you need external information or to perform actions
- After using a tool, always review the result before deciding next steps
- Provide a final answer once you have sufficient information
- If a tool fails, try an alternative approach or acknowledge the limitation

Format your responses as:
THOUGHT: <your reasoning>
ACTION: <tool_name>(<json_arguments>)
`

// DefaultFewShotExamples contains few-shot examples for the ReAct agent.
// Note: Examples use placeholder JSON to avoid raw string literal conflicts.
const DefaultFewShotExamples = `Example 1:
---
User: What is the current weather in Tokyo?

Thought: The user wants to know the weather in Tokyo. I should use a weather tool to get this information.

Action: weather(tool_args)

Observation: {"temperature": 22, "condition": "Partly Cloudy", "humidity": 65}

Thought: I now have the weather information for Tokyo. I can provide this to the user.

Final Answer: The current weather in Tokyo is partly cloudy with a temperature of 22C and humidity of 65%.
---

Example 2:
---
User: Calculate the sum of all even numbers from 1 to 100.

Thought: I need to calculate the sum of all even numbers from 1 to 100. Even numbers in this range are 2, 4, 6, ..., 100. This is an arithmetic series with first term 2, last term 100, and 50 terms. The sum = n/2 * (first + last) = 50/2 * (2 + 100) = 25 * 102 = 2550.

Action: calculator(tool_args)

Observation: {"result": 2550}

Thought: The calculator confirmed the result. 2550 is the sum of all even numbers from 1 to 100.

Final Answer: The sum of all even numbers from 1 to 100 is 2,550.
---

Example 3:
---
User: Search for recent news about AI

Thought: The user wants recent news about AI. I'll use a search tool to find this information.

Action: search(tool_args)

Observation: {"results": [{"title": "New AI Model Breaks Records", "url": "..."}, {"title": "AI Ethics Guidelines Released", "url": "..."}]}

Thought: I found several recent AI news articles. Let me provide a summary of the findings.

Final Answer: Here are recent AI news highlights:
1. New AI Model Breaks Records - A new AI model has achieved state-of-the-art results...
2. AI Ethics Guidelines Released - Major organizations have published...
`

// ----------------------------------------------------------------------------
// PromptBuilder
// ----------------------------------------------------------------------------

// PromptBuilder builds prompts for the ReAct agent.
type PromptBuilder struct {
	systemPrompt   string
	fewShotExamples string
	tools          []Tool
	template       *template.Template
}

// NewPromptBuilder creates a new PromptBuilder.
func NewPromptBuilder() *PromptBuilder {
	return &PromptBuilder{
		systemPrompt:   DefaultSystemPrompt,
		fewShotExamples: DefaultFewShotExamples,
		template:       nil,
	}
}

// WithSystemPrompt sets the system prompt.
func (b *PromptBuilder) WithSystemPrompt(prompt string) *PromptBuilder {
	b.systemPrompt = prompt
	return b
}

// WithFewShotExamples sets the few-shot examples.
func (b *PromptBuilder) WithFewShotExamples(examples string) *PromptBuilder {
	b.fewShotExamples = examples
	return b
}

// WithTools sets the tools for the prompt.
func (b *PromptBuilder) WithTools(tools []Tool) *PromptBuilder {
	b.tools = tools
	return b
}

// Build builds the system prompt with tools substituted.
func (b *PromptBuilder) Build() (string, error) {
	// Build tools section
	toolsSection, err := b.buildToolsSection()
	if err != nil {
		return "", err
	}

	// Execute template with tools substituted
	tmpl := b.systemPrompt
	if b.template != nil {
		tmpl = b.systemPrompt
	}

	result := strings.Replace(tmpl, "{{TOOLS}}", toolsSection, 1)

	// Add few-shot examples if present
	if b.fewShotExamples != "" {
		result += "\n" + b.fewShotExamples
	}

	return result, nil
}

// buildToolsSection builds the tools section of the system prompt.
func (b *PromptBuilder) buildToolsSection() (string, error) {
	if len(b.tools) == 0 {
		return "(No tools available)", nil
	}

	var buf bytes.Buffer
	buf.WriteString("\n")
	for _, tool := range b.tools {
		buf.WriteString(fmt.Sprintf("## %s\n", tool.Name()))
		buf.WriteString(fmt.Sprintf("Description: %s\n", tool.Description()))
		buf.WriteString(fmt.Sprintf("Input: %s\n\n", tool.InputSchema()))
	}
	return buf.String(), nil
}

// BuildMessages builds the full message list for the LLM.
func (b *PromptBuilder) BuildMessages(ctx context.Context, userInput string, history []*Message) ([]*Message, error) {
	systemPrompt, err := b.Build()
	if err != nil {
		return nil, err
	}

	messages := make([]*Message, 0, 2+len(history))

	// Add system message
	messages = append(messages, &Message{
		Role:    RoleSystem,
		Content: systemPrompt,
	})

	// Add history messages
	messages = append(messages, history...)

	// Add current user input
	messages = append(messages, &Message{
		Role:    RoleUser,
		Content: userInput,
	})

	return messages, nil
}

// ----------------------------------------------------------------------------
// Prompt Parser
// ----------------------------------------------------------------------------

// ParseResponse parses an LLM response to extract thought, action, and answer.
// The LLM is expected to respond in a specific format with markers.
type ResponseParser struct{}

// NewResponseParser creates a new ResponseParser.
func NewResponseParser() *ResponseParser {
	return &ResponseParser{}
}

// ParseResult represents the parsed components of an LLM response.
type ParseResult struct {
	HasThought    bool
	Thought       string
	HasAction     bool
	ActionName    string
	ActionArgs    map[string]any
	ActionRaw     string
	HasFinalAnswer bool
	FinalAnswer   string
	Raw           string
}

// Parse attempts to parse a response for thought, action, and final answer.
func (p *ResponseParser) Parse(response string) *ParseResult {
	result := &ParseResult{Raw: response}

	// Look for THOUGHT marker
	if thought := extractBetween(response, "THOUGHT:", "\n"); thought != "" {
		result.HasThought = true
		result.Thought = strings.TrimSpace(thought)
	}

	// Look for ACTION marker
	if actionBlock := extractBetween(response, "ACTION:", "\n"); actionBlock != "" {
		result.HasAction = true
		result.ActionRaw = strings.TrimSpace(actionBlock)

		// Try to parse action name and arguments
		name, args, err := parseActionCall(actionBlock)
		if err == nil {
			result.ActionName = name
			result.ActionArgs = args
		}
	}

	// Look for Final Answer marker
	if finalAnswer := extractBetween(response, "FINAL_ANSWER:", ""); finalAnswer != "" {
		result.HasFinalAnswer = true
		result.FinalAnswer = strings.TrimSpace(finalAnswer)
	} else if finalAnswer := extractBetween(response, "Final Answer:", ""); finalAnswer != "" {
		result.HasFinalAnswer = true
		result.FinalAnswer = strings.TrimSpace(finalAnswer)
	}

	return result
}

// extractBetween extracts text between two markers.
func extractBetween(s, start, end string) string {
	if start == "" {
		// Extract from beginning
		if end == "" {
			return s
		}
		idx := strings.Index(s, end)
		if idx == -1 {
			return ""
		}
		return s[:idx]
	}

	startIdx := strings.Index(s, start)
	if startIdx == -1 {
		return ""
	}
	startIdx += len(start)

	if end == "" {
		return s[startIdx:]
	}

	endIdx := strings.Index(s[startIdx:], end)
	if endIdx == -1 {
		return ""
	}
	return s[startIdx : startIdx+endIdx]
}

// parseActionCall parses an action call like "tool_name({"arg": "value"})".
func parseActionCall(action string) (string, map[string]any, error) {
	action = strings.TrimSpace(action)

	// Find the opening paren
	openIdx := strings.Index(action, "(")
	if openIdx == -1 {
		return "", nil, fmt.Errorf("no opening paren in action: %s", action)
	}

	name := strings.TrimSpace(action[:openIdx])
	if name == "" {
		return "", nil, fmt.Errorf("empty tool name in action: %s", action)
	}

	// Extract JSON arguments
	argsStr := action[openIdx+1:]
	closeIdx := strings.LastIndex(argsStr, ")")
	if closeIdx == -1 {
		return "", nil, fmt.Errorf("no closing paren in action: %s", action)
	}
	argsStr = argsStr[:closeIdx]

	if argsStr == "" {
		return name, make(map[string]any), nil
	}

	// Try to parse as JSON
	var args map[string]any
	if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
		// Try with braces if missing
		if !strings.HasPrefix(argsStr, "{") {
			argsStr = "{" + argsStr + "}"
			if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
				return "", nil, fmt.Errorf("failed to parse action args: %w", err)
			}
		}
	}

	return name, args, nil
}