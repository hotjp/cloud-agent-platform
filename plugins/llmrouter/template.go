// Package llmrouter implements LLM multi-model routing (Claude/GLM adaptive).
package llmrouter

import (
	"fmt"
	"strings"
	"sync"
)

// TemplateType represents the type of prompt template.
type TemplateType string

const (
	TemplateTaskDecompose TemplateType = "task_decompose"
	TemplateCodeReview    TemplateType = "code_review"
	TemplateCodeFix      TemplateType = "code_fix"
	TemplateTestGen      TemplateType = "test_gen"
)

// TemplateVar represents a variable in a prompt template.
type TemplateVar struct {
	Key   string
	Value string
}

// PromptTemplate represents a system prompt template with variable injection.
type PromptTemplate struct {
	Name       TemplateType
	SystemTmpl string
	UserTmpl   string
	Desc       string
}

// BuiltInTemplates contains all built-in prompt templates.
var BuiltInTemplates = map[TemplateType]PromptTemplate{
	TemplateTaskDecompose: {
		Name: TemplateTaskDecompose,
		Desc: "Task decomposition template for breaking down complex tasks",
		SystemTmpl: `You are an expert task decomposition assistant.
Break down the given task into smaller, actionable subtasks.

{{if .Goal}}Goal: {{.Goal}}
{{end}}{{if .Context}}Context: {{.Context}}
{{end}}{{if .Constraints}}Constraints: {{.Constraints}}
{{end}}{{if .VerificationCriteria}}Verification Criteria: {{.VerificationCriteria}}
{{end}}

Follow these principles:
1. Each subtask should be atomic and independently verifiable
2. Include acceptance criteria for each subtask
3. Consider dependencies between subtasks
4. Prioritize critical path items`,
		UserTmpl: `Decompose the following task into subtasks:

{{.Goal}}

{{if .Context}}Additional Context:
{{.Context}}
{{end}}`,
	},
	TemplateCodeReview: {
		Name: TemplateCodeReview,
		Desc: "Code review template for analyzing code quality and security",
		SystemTmpl: `You are an expert code reviewer specializing in security, performance, and best practices.

{{if .Constraints}}Review Constraints: {{.Constraints}}
{{end}}

Provide reviews covering:
1. Security vulnerabilities (injection, auth issues, data exposure)
2. Performance concerns (N+1 queries, missing indexes, inefficient algorithms)
3. Error handling completeness
4. Code readability and maintainability
5. Edge case coverage`,
		UserTmpl: `Review the following code:

{{.Context}}

{{if .Goal}}Specific Focus: {{.Goal}}
{{end}}`,
	},
	TemplateCodeFix: {
		Name: TemplateCodeFix,
		Desc: "Code fix template for bug fixing with context",
		SystemTmpl: `You are an expert bug fixer. Analyze the problem, identify root cause, and provide a fix.

{{if .Context}}Code Context:
{{.Context}}
{{end}}{{if .Constraints}}Constraints:
{{.Constraints}}
{{end}}{{if .VerificationCriteria}}Expected Behavior:
{{.VerificationCriteria}}
{{end}}

Follow this process:
1. Identify the root cause
2. Explain why the bug occurs
3. Provide the minimal fix
4. Suggest preventive measures`,
		UserTmpl: `Bug Description:
{{.Goal}}

Error/Symptom:
{{.Context}}`,
	},
	TemplateTestGen: {
		Name: TemplateTestGen,
		Desc: "Test generation template for creating comprehensive test cases",
		SystemTmpl: `You are an expert test engineer. Generate comprehensive test cases.

{{if .Constraints}}Test Constraints: {{.Constraints}}
{{end}}

Coverage requirements:
1. Happy path cases
2. Edge cases and boundary conditions
3. Error handling paths
4. Performance considerations
5. Security test cases if applicable`,
		UserTmpl: `Generate tests for:
{{.Goal}}

Code under test:
{{.Context}}

{{if .VerificationCriteria}}Verification: {{.VerificationCriteria}}
{{end}}`,
	},
}

// templateEngine handles template rendering.
type templateEngine struct {
	mu     sync.RWMutex
	custom map[TemplateType]PromptTemplate
}

// globalTemplateEngine is the global template engine instance.
var globalTemplateEngine = &templateEngine{
	custom: make(map[TemplateType]PromptTemplate),
}

// GetTemplate returns a template by type (builtin or custom).
func GetTemplate(tmplType TemplateType) (PromptTemplate, bool) {
	// Check custom templates first
	globalTemplateEngine.mu.RLock()
	if tmpl, ok := globalTemplateEngine.custom[tmplType]; ok {
		globalTemplateEngine.mu.RUnlock()
		return tmpl, true
	}
	globalTemplateEngine.mu.RUnlock()

	// Check built-in templates
	if tmpl, ok := BuiltInTemplates[tmplType]; ok {
		return tmpl, true
	}
	return PromptTemplate{}, false
}

// RegisterTemplate registers a custom template.
func RegisterTemplate(tmpl PromptTemplate) {
	globalTemplateEngine.mu.Lock()
	defer globalTemplateEngine.mu.Unlock()
	globalTemplateEngine.custom[tmpl.Name] = tmpl
}

// RenderTemplate renders a template with the given variables.
func RenderTemplate(tmplType TemplateType, vars []TemplateVar) (systemPrompt, userPrompt string, err error) {
	tmpl, ok := GetTemplate(tmplType)
	if !ok {
		return "", "", fmt.Errorf("template not found: %s", tmplType)
	}

	systemPrompt, err = renderString(tmpl.SystemTmpl, vars)
	if err != nil {
		return "", "", fmt.Errorf("render system template: %w", err)
	}

	userPrompt, err = renderString(tmpl.UserTmpl, vars)
	if err != nil {
		return "", "", fmt.Errorf("render user template: %w", err)
	}

	return systemPrompt, userPrompt, nil
}

// renderString renders a template string with variables.
func renderString(tmpl string, vars []TemplateVar) (string, error) {
	result := tmpl

	// Build variable map for lookup
	varMap := make(map[string]string)
	for _, v := range vars {
		varMap[v.Key] = v.Value
	}

	// Process conditional blocks: {{if .Key}}...{{end}}
	// This handles {{if .Goal}}content{{end}} where content only appears if Goal is set
	for {
		idx := strings.Index(result, "{{if .")
		if idx == -1 {
			break
		}

		// Find the end of the if block
		endIdx := strings.Index(result[idx:], "{{end}}")
		if endIdx == -1 {
			break
		}
		endIdx += idx + len("{{end}}")

		// Extract the condition key
		condStart := idx + len("{{if .")
		condEnd := strings.Index(result[condStart:], "}}")
		if condEnd == -1 {
			break
		}
		condKey := result[condStart : condStart+condEnd]

		// Check if variable is set
		val, exists := varMap[condKey]

		// Extract the content between {{if .Key}} and {{end}}
		contentStart := condStart + condEnd + len("}}")
		content := result[contentStart : endIdx-len("{{end}}")]

		// Replace the conditional block
		if exists && val != "" {
			// Render content with the variable value
			rendered := strings.Replace(content, "{{."+condKey+"}}", val, -1)
			result = result[:idx] + rendered + result[endIdx:]
		} else {
			// Remove the entire block
			result = result[:idx] + result[endIdx:]
		}
	}

	// Process simple variable replacements: {{.Key}}
	for key, val := range varMap {
		result = strings.Replace(result, "{{."+key+"}}", val, -1)
	}

	return result, nil
}

// OptimizePrompt applies prompt optimizations.
func OptimizePrompt(prompt string, opts ...PromptOption) string {
	cfg := &promptConfig{
		addFormatInstruction: true,
		addLengthWarning:     true,
		maxLength:            4000,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	result := prompt

	// Add format instruction if enabled
	if cfg.addFormatInstruction {
		result = addFormatInstruction(result)
	}

	// Add length warning if enabled
	if cfg.addLengthWarning && len(result) > cfg.maxLength {
		result = addLengthWarning(result, cfg.maxLength)
	}

	return result
}

// promptConfig holds configuration for prompt optimization.
type promptConfig struct {
	addFormatInstruction bool
	addLengthWarning     bool
	maxLength            int
}

// PromptOption is a function that modifies promptConfig.
type PromptOption func(*promptConfig)

// WithFormatInstruction enables or disables format instruction addition.
func WithFormatInstruction(enabled bool) PromptOption {
	return func(c *promptConfig) {
		c.addFormatInstruction = enabled
	}
}

// WithLengthWarning enables or disables length warning addition.
func WithLengthWarning(enabled bool) PromptOption {
	return func(c *promptConfig) {
		c.addLengthWarning = enabled
	}
}

// WithMaxLength sets the maximum length before adding a warning.
func WithMaxLength(max int) PromptOption {
	return func(c *promptConfig) {
		c.maxLength = max
	}
}

// addFormatInstruction adds formatting instructions to the prompt.
func addFormatInstruction(prompt string) string {
	instr := "\n\nFormat your response clearly. Use code blocks for code. Use bullet points for lists."
	if !strings.Contains(prompt, "code blocks") && !strings.Contains(prompt, "Format your response") {
		return prompt + instr
	}
	return prompt
}

// addLengthWarning adds a warning about response length.
func addLengthWarning(prompt string, maxLen int) string {
	warning := fmt.Sprintf("\n\n[Note: Keep your response under %d characters for clarity.]", maxLen/4)
	return prompt + warning
}

// PromptVars creates a slice of TemplateVar from key-value pairs.
func PromptVars(pairs ...string) []TemplateVar {
	vars := make([]TemplateVar, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		if i+1 < len(pairs) {
			vars = append(vars, TemplateVar{Key: pairs[i], Value: pairs[i+1]})
		}
	}
	return vars
}

// ListTemplates returns all available template types.
func ListTemplates() []TemplateType {
	types := make([]TemplateType, 0, len(BuiltInTemplates)+len(globalTemplateEngine.custom))
	for t := range BuiltInTemplates {
		types = append(types, t)
	}
	globalTemplateEngine.mu.RLock()
	for t := range globalTemplateEngine.custom {
		types = append(types, t)
	}
	globalTemplateEngine.mu.RUnlock()
	return types
}
