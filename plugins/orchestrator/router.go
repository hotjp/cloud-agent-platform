// Package orchestrator implements Eino-based task orchestration graph.
// Responsible for task decomposition, agent matching, and execution scheduling.
package orchestrator

import (
	"context"
	"regexp"
	"strings"
)

// RoutingRule defines a single routing rule condition.
type RoutingRule struct {
	// Name is the name of the target path (e.g., "simple", "medium", "complex").
	Name string
	// MinTokens is the minimum estimated token count to match this rule.
	MinTokens int
	// MaxTokens is the maximum estimated token count to match this rule (0 = unlimited).
	MaxTokens int
	// Keywords that indicate this complexity level (OR logic within a rule).
	Keywords []string
	// Patterns are regex patterns that indicate this complexity level.
	Patterns []*regexp.Regexp
	// HistoricalComplexity if set, tasks with this historical complexity will match.
	HistoricalComplexity TaskComplexity
}

// RouterConfig holds configuration for the ComplexityAnalyzer.
type RouterConfig struct {
	// Rules defines the routing rules in order of precedence.
	Rules []RoutingRule
	// DefaultComplexity is the complexity used when no rules match.
	DefaultComplexity TaskComplexity
	// TokenEstimateFunc estimates tokens from goal text.
	TokenEstimateFunc func(goal string) int
}

// DefaultRouterConfig returns the default router configuration.
func DefaultRouterConfig() *RouterConfig {
	return &RouterConfig{
		Rules: []RoutingRule{
			// Complex: architecture, refactoring, multi-module (evaluated FIRST due to higher priority)
			{
				Name:      "complex",
				Keywords: []string{
					"architecture", "refactor", "design pattern", "multi-module",
					"distributed", "microservice", "performance optimization",
					"security audit", "code review", "design system",
					"rearchitect", "redesign", "system design", "microservices",
				},
			},
			// Medium: multiple files, modules, features
			{
				Name:      "medium",
				Keywords: []string{
					"implement feature", "add functionality", "create module",
					"build component", "develop feature", "multiple files",
					"api endpoint", "rest api", "database migration",
					"add feature", "new feature", "create feature",
				},
			},
			// Simple: single file changes, small scope
			{
				Name:      "simple",
				Keywords: []string{
					"fix typo", "update comment", "rename variable", "format code",
					"add import", "remove debug", "change string", "single file",
					"fix bug", "hotfix", "quick fix",
				},
			},
		},
		DefaultComplexity: ComplexityMedium,
		TokenEstimateFunc: estimateTokens,
	}
}

// estimateTokens estimates the token count for a given text.
// This is a rough estimation based on word count (1 token ≈ 0.75 words).
func estimateTokens(text string) int {
	words := len(regexp.MustCompile(`\w+`).FindAllString(text, -1))
	return int(float64(words) / 0.75)
}

// ComplexityAnalyzer routes tasks based on configurable complexity rules.
// It analyzes task input and determines the appropriate complexity level.
type ComplexityAnalyzer struct {
	cfg *RouterConfig
}

// NewComplexityAnalyzer creates a new ComplexityAnalyzer with default configuration.
func NewComplexityAnalyzer() *ComplexityAnalyzer {
	return &ComplexityAnalyzer{cfg: DefaultRouterConfig()}
}

// NewComplexityAnalyzerWithConfig creates a new ComplexityAnalyzer with custom configuration.
func NewComplexityAnalyzerWithConfig(cfg *RouterConfig) *ComplexityAnalyzer {
	if cfg == nil {
		cfg = DefaultRouterConfig()
	}
	if cfg.TokenEstimateFunc == nil {
		cfg.TokenEstimateFunc = estimateTokens
	}
	return &ComplexityAnalyzer{cfg: cfg}
}

// Analyze determines the routing path based on task input.
func (r *ComplexityAnalyzer) Analyze(ctx context.Context, input *TaskInput) string {
	goal := strings.ToLower(input.Goal)
	estimatedTokens := r.cfg.TokenEstimateFunc(input.Goal)

	// Try each rule in order
	for _, rule := range r.cfg.Rules {
		if r.matchesRule(goal, estimatedTokens, input, &rule) {
			return rule.Name
		}
	}

	// Default fallback
	return string(r.cfg.DefaultComplexity)
}

// matchesRule checks if an input matches a specific routing rule.
func (r *ComplexityAnalyzer) matchesRule(goal string, tokens int, input *TaskInput, rule *RoutingRule) bool {
	// Check token range
	if rule.MinTokens > 0 && tokens < rule.MinTokens {
		return false
	}
	if rule.MaxTokens > 0 && tokens > rule.MaxTokens {
		return false
	}

	// Check historical complexity
	if rule.HistoricalComplexity != "" && input.HistoricalComplexity != "" {
		if rule.HistoricalComplexity != input.HistoricalComplexity {
			return false
		}
	}

	// Check keywords (OR logic - any keyword match)
	hasKeyword := len(rule.Keywords) == 0
	for _, kw := range rule.Keywords {
		if strings.Contains(goal, strings.ToLower(kw)) {
			hasKeyword = true
			break
		}
	}
	if !hasKeyword {
		return false
	}

	// Check patterns (OR logic - any pattern match)
	if len(rule.Patterns) > 0 {
		hasPattern := false
		for _, pattern := range rule.Patterns {
			if pattern.MatchString(goal) {
				hasPattern = true
				break
			}
		}
		if !hasPattern {
			return false
		}
	}

	return true
}

// AnalyzeWithComplexity routes based on pre-determined complexity level.
func (r *ComplexityAnalyzer) AnalyzeWithComplexity(ctx context.Context, complexity TaskComplexity) string {
	switch complexity {
	case ComplexitySimple:
		return "simple"
	case ComplexityMedium:
		return "medium"
	case ComplexityComplex:
		return "complex"
	default:
		return "medium"
	}
}

// GetConfig returns the analyzer configuration.
func (r *ComplexityAnalyzer) GetConfig() *RouterConfig {
	return r.cfg
}

// UpdateRule updates a routing rule by name.
func (r *ComplexityAnalyzer) UpdateRule(name string, rule RoutingRule) {
	for i, cfg := range r.cfg.Rules {
		if cfg.Name == name {
			r.cfg.Rules[i] = rule
			return
		}
	}
	// Rule not found, append it
	r.cfg.Rules = append(r.cfg.Rules, rule)
}
