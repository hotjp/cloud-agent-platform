// Package llmrouter implements LLM multi-model routing (Claude/GLM adaptive).
package llmrouter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuiltInTemplates(t *testing.T) {
	t.Run("task_decompose template exists", func(t *testing.T) {
		tmpl, ok := BuiltInTemplates[TemplateTaskDecompose]
		assert.True(t, ok)
		assert.NotEmpty(t, tmpl.SystemTmpl)
		assert.NotEmpty(t, tmpl.UserTmpl)
	})

	t.Run("code_review template exists", func(t *testing.T) {
		tmpl, ok := BuiltInTemplates[TemplateCodeReview]
		assert.True(t, ok)
		assert.NotEmpty(t, tmpl.SystemTmpl)
	})

	t.Run("code_fix template exists", func(t *testing.T) {
		tmpl, ok := BuiltInTemplates[TemplateCodeFix]
		assert.True(t, ok)
		assert.NotEmpty(t, tmpl.SystemTmpl)
	})

	t.Run("test_gen template exists", func(t *testing.T) {
		tmpl, ok := BuiltInTemplates[TemplateTestGen]
		assert.True(t, ok)
		assert.NotEmpty(t, tmpl.SystemTmpl)
	})
}

func TestGetTemplate(t *testing.T) {
	t.Run("returns built-in template", func(t *testing.T) {
		tmpl, ok := GetTemplate(TemplateTaskDecompose)
		assert.True(t, ok)
		assert.Equal(t, TemplateTaskDecompose, tmpl.Name)
	})

	t.Run("returns false for unknown template", func(t *testing.T) {
		_, ok := GetTemplate(TemplateType("unknown"))
		assert.False(t, ok)
	})
}

func TestRegisterTemplate(t *testing.T) {
	custom := PromptTemplate{
		Name:       TemplateType("custom_test"),
		SystemTmpl: "Custom system: {{.Goal}}",
		UserTmpl:   "Custom user: {{.Context}}",
		Desc:       "Custom test template",
	}

	RegisterTemplate(custom)

	tmpl, ok := GetTemplate(TemplateType("custom_test"))
	assert.True(t, ok)
	assert.Equal(t, "Custom system: {{.Goal}}", tmpl.SystemTmpl)
}

func TestRenderTemplate(t *testing.T) {
	t.Run("renders task_decompose with all vars", func(t *testing.T) {
		vars := PromptVars(
			"Goal", "Implement user authentication",
			"Context", "Using JWT tokens with Redis session store",
			"Constraints", "Must support refresh tokens",
			"VerificationCriteria", "Login/logout works correctly",
		)

		system, user, err := RenderTemplate(TemplateTaskDecompose, vars)
		assert.NoError(t, err)
		assert.Contains(t, system, "Goal: Implement user authentication")
		assert.Contains(t, system, "Context: Using JWT tokens with Redis session store")
		assert.Contains(t, system, "Constraints: Must support refresh tokens")
		assert.Contains(t, system, "Verification Criteria: Login/logout works correctly")
		assert.Contains(t, user, "Implement user authentication")
	})

	t.Run("renders code_review with partial vars", func(t *testing.T) {
		vars := PromptVars(
			"Goal", "Focus on SQL injection",
			"Context", "SELECT * FROM users WHERE id = ?",
		)

		_, user, err := RenderTemplate(TemplateCodeReview, vars)
		assert.NoError(t, err)
		// Goal appears in user prompt, not system prompt
		assert.Contains(t, user, "Focus on SQL injection")
		assert.Contains(t, user, "SELECT * FROM users")
	})

	t.Run("renders code_fix with context", func(t *testing.T) {
		vars := PromptVars(
			"Goal", "Fix nil pointer dereference",
			"Context", "panic: runtime error: invalid memory address or nil pointer dereference",
			"Constraints", "Cannot change function signature",
			"VerificationCriteria", "Function returns empty slice, not panics",
		)

		system, user, err := RenderTemplate(TemplateCodeFix, vars)
		assert.NoError(t, err)
		assert.Contains(t, system, "nil pointer dereference")
		assert.Contains(t, system, "Cannot change function signature")
		assert.Contains(t, user, "Fix nil pointer dereference")
		assert.Contains(t, user, "panic: runtime error")
	})

	t.Run("renders test_gen template", func(t *testing.T) {
		vars := PromptVars(
			"Goal", "Test the CalculateTotal function",
			"Context", "func CalculateTotal(items []Item) float64",
			"Constraints", "Use table-driven tests",
			"VerificationCriteria", "All test cases pass",
		)

		system, user, err := RenderTemplate(TemplateTestGen, vars)
		assert.NoError(t, err)
		assert.Contains(t, system, "table-driven tests")
		assert.Contains(t, user, "CalculateTotal")
	})

	t.Run("returns error for unknown template", func(t *testing.T) {
		_, _, err := RenderTemplate(TemplateType("nonexistent"), nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "template not found")
	})
}

func TestRenderString(t *testing.T) {
	t.Run("renders simple variables", func(t *testing.T) {
		tmpl := "Hello {{.Name}}, your order {{.OrderID}} is ready."
		vars := PromptVars("Name", "Alice", "OrderID", "12345")

		result, err := renderString(tmpl, vars)
		assert.NoError(t, err)
		assert.Equal(t, "Hello Alice, your order 12345 is ready.", result)
	})

	t.Run("renders conditional blocks with value", func(t *testing.T) {
		tmpl := "Start{{if .Goal}} Goal: {{.Goal}}{{end}} End"
		vars := PromptVars("Goal", "Test goal")

		result, err := renderString(tmpl, vars)
		assert.NoError(t, err)
		assert.Equal(t, "Start Goal: Test goal End", result)
	})

	t.Run("removes conditional blocks without value", func(t *testing.T) {
		tmpl := "Start{{if .Goal}} Goal: {{.Goal}}{{end}} End"
		vars := PromptVars("Other", "Value")

		result, err := renderString(tmpl, vars)
		assert.NoError(t, err)
		assert.Equal(t, "Start End", result)
	})

	t.Run("handles empty value in conditional", func(t *testing.T) {
		tmpl := "Start{{if .Goal}} Goal: {{.Goal}}{{end}} End"
		vars := PromptVars("Goal", "")

		result, err := renderString(tmpl, vars)
		assert.NoError(t, err)
		assert.Equal(t, "Start End", result)
	})

	t.Run("renders multiple conditionals", func(t *testing.T) {
		tmpl := "{{if .A}}A: {{.A}}{{end}}{{if .B}}B: {{.B}}{{end}}"
		vars := PromptVars("A", "valueA", "B", "valueB")

		result, err := renderString(tmpl, vars)
		assert.NoError(t, err)
		assert.Equal(t, "A: valueAB: valueB", result)
	})

	t.Run("preserves text outside conditionals", func(t *testing.T) {
		tmpl := "Prefix{{if .X}}Middle{{.X}}End{{end}}Suffix"
		vars := PromptVars("X", " - inserted - ")

		result, err := renderString(tmpl, vars)
		assert.NoError(t, err)
		assert.Equal(t, "PrefixMiddle - inserted - EndSuffix", result)
	})
}

func TestOptimizePrompt(t *testing.T) {
	t.Run("adds format instruction by default", func(t *testing.T) {
		prompt := "Explain this code"
		result := OptimizePrompt(prompt)
		assert.Contains(t, result, "Format your response clearly")
		assert.Contains(t, result, "code blocks")
	})

	t.Run("does not duplicate format instruction", func(t *testing.T) {
		// First call adds the instruction
		prompt := "Explain this code"
		result1 := OptimizePrompt(prompt)
		assert.Equal(t, 1, countSubstring(result1, "Format your response clearly"))

		// Second call should not add duplicate
		result2 := OptimizePrompt(result1)
		assert.Equal(t, 1, countSubstring(result2, "Format your response clearly"))
	})

	t.Run("respects WithFormatInstruction false", func(t *testing.T) {
		prompt := "Simple prompt"
		result := OptimizePrompt(prompt, WithFormatInstruction(false))
		assert.NotContains(t, result, "Format your response clearly")
	})

	t.Run("adds length warning for long prompts", func(t *testing.T) {
		longPrompt := ""
		for i := 0; i < 500; i++ {
			longPrompt += "word "
		}
		result := OptimizePrompt(longPrompt, WithMaxLength(1000))
		assert.Contains(t, result, "Keep your response under")
	})

	t.Run("skips length warning for short prompts", func(t *testing.T) {
		prompt := "Short prompt"
		result := OptimizePrompt(prompt, WithMaxLength(1000))
		assert.NotContains(t, result, "Keep your response")
	})

	t.Run("respects WithLengthWarning false", func(t *testing.T) {
		longPrompt := ""
		for i := 0; i < 500; i++ {
			longPrompt += "word "
		}
		result := OptimizePrompt(longPrompt, WithLengthWarning(false))
		assert.NotContains(t, result, "Keep your response")
	})
}

func countSubstring(s, substr string) int {
	count := 0
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			count++
		}
	}
	return count
}

func TestPromptVars(t *testing.T) {
	t.Run("creates vars from pairs", func(t *testing.T) {
		vars := PromptVars("Key1", "Value1", "Key2", "Value2")
		assert.Len(t, vars, 2)
		assert.Equal(t, "Key1", vars[0].Key)
		assert.Equal(t, "Value1", vars[0].Value)
		assert.Equal(t, "Key2", vars[1].Key)
		assert.Equal(t, "Value2", vars[1].Value)
	})

	t.Run("handles odd number of args", func(t *testing.T) {
		vars := PromptVars("Key1", "Value1", "KeyOnly")
		assert.Len(t, vars, 1)
	})

	t.Run("returns empty for no args", func(t *testing.T) {
		vars := PromptVars()
		assert.Len(t, vars, 0)
	})
}

func TestListTemplates(t *testing.T) {
	templates := ListTemplates()
	assert.NotEmpty(t, templates)
	assert.Contains(t, templates, TemplateTaskDecompose)
	assert.Contains(t, templates, TemplateCodeReview)
	assert.Contains(t, templates, TemplateCodeFix)
	assert.Contains(t, templates, TemplateTestGen)
}

func TestTemplateVar(t *testing.T) {
	t.Run("creates template var", func(t *testing.T) {
		v := TemplateVar{Key: "test", Value: "value"}
		assert.Equal(t, "test", v.Key)
		assert.Equal(t, "value", v.Value)
	})
}

func TestPromptTemplate(t *testing.T) {
	t.Run("creates prompt template", func(t *testing.T) {
		tmpl := PromptTemplate{
			Name:       TemplateTaskDecompose,
			SystemTmpl: "System prompt",
			UserTmpl:   "User prompt",
			Desc:       "Test template",
		}
		assert.Equal(t, TemplateTaskDecompose, tmpl.Name)
		assert.Equal(t, "System prompt", tmpl.SystemTmpl)
		assert.Equal(t, "User prompt", tmpl.UserTmpl)
		assert.Equal(t, "Test template", tmpl.Desc)
	})
}
