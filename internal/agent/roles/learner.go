// Package roles defines the six core Agent roles for the Cloud Agent Platform.
package roles

import (
	"context"
	"fmt"
	"time"

	"github.com/cloud-agent-platform/cap/internal/agent/react"
	"go.uber.org/zap"
)

// LearnerConfig holds configuration for the Learner role.
type LearnerConfig struct {
	// Timeout is the timeout for learning.
	Timeout time.Duration
	// MaxPatterns is the maximum number of patterns to extract.
	MaxPatterns int
	// ConfidenceThreshold is the minimum confidence for insights.
	ConfidenceThreshold float64
}

// DefaultLearnerConfig returns the default Learner configuration.
func DefaultLearnerConfig() *LearnerConfig {
	return &LearnerConfig{
		Timeout:             60 * time.Second,
		MaxPatterns:         10,
		ConfidenceThreshold: 0.7,
	}
}

// LearnerResult represents the result of learning.
type LearnerResult struct {
	// TaskID is the ULID of the task being learned from.
	TaskID string `json:"task_id"`
	// Lessons contains the lessons learned.
	Lessons []*Lesson `json:"lessons"`
	// Improvements contains improvement suggestions.
	Improvements []*Improvement `json:"improvements"`
	// KnowledgeUpdates contains knowledge base updates.
	KnowledgeUpdates []string `json:"knowledge_updates"`
	// Duration is how long the learning took.
	Duration time.Duration `json:"duration"`
	// Error records any error that occurred.
	Error error `json:"error,omitempty"`
}

// Lesson represents a learned lesson.
type Lesson struct {
	// Pattern describes the pattern identified.
	Pattern string `json:"pattern"`
	// Context describes when this pattern applies.
	Context string `json:"context"`
	// Recommendation is how to apply this lesson.
	Recommendation string `json:"recommendation"`
	// Confidence is the confidence level (0-1).
	Confidence float64 `json:"confidence"`
}

// Improvement represents an improvement suggestion.
type Improvement struct {
	// Category is the improvement category.
	Category ImprovementCategory `json:"category"`
	// Description describes the improvement.
	Description string `json:"description"`
	// Impact is the expected impact.
	Impact string `json:"impact"`
}

// ImprovementCategory categorizes improvements.
type ImprovementCategory string

const (
	ImprovementProcess   ImprovementCategory = "process"
	ImprovementTooling   ImprovementCategory = "tooling"
	ImprovementDocs      ImprovementCategory = "documentation"
	ImprovementTeam      ImprovementCategory = "team"
)

// NewLearnerAgent creates a new Learner agent.
func NewLearnerAgent(llm react.LLM, tools *react.ToolRegistry, logger *zap.Logger) (*react.Agent, error) {
	config := react.DefaultConfig()
	config.SystemPrompt = RoleDefinitions[RoleLearner].BuildPrompt()
	config.MaxIterations = 10
	config.Timeout = 60 * time.Second
	return react.NewAgent(llm, tools, config, logger)
}

// Learn executes the learning workflow.
func Learn(ctx context.Context, agent *react.Agent, taskID string, executionHistory string, previousResults string) (*LearnerResult, error) {
	start := time.Now()
	result := &LearnerResult{
		TaskID: taskID,
		Lessons: make([]*Lesson, 0),
		Improvements: make([]*Improvement, 0),
		KnowledgeUpdates: make([]string, 0),
	}

	// Execute the ReAct agent
	actResult, err := agent.Run(ctx, fmt.Sprintf("Learn from this execution:\n\nTask ID: %s\n\nExecution History:\n%s\n\nPrevious Results:\n%s", taskID, executionHistory, previousResults))
	if err != nil {
		result.Error = err
		result.Duration = time.Since(start)
		return result, err
	}

	// Parse learning result
	parsed := parseLearning(actResult.Answer)
	result.Lessons = parsed.Lessons
	result.Improvements = parsed.Improvements
	result.KnowledgeUpdates = parsed.KnowledgeUpdates
	result.Duration = time.Since(start)

	return result, nil
}

// parsedLearning holds parsed learning data.
type parsedLearningData struct {
	Lessons         []*Lesson
	Improvements   []*Improvement
	KnowledgeUpdates []string
}

// parseLearning parses the learning result from the agent's answer.
func parseLearning(answer string) *parsedLearningData {
	data := &parsedLearningData{
		Lessons:           make([]*Lesson, 0),
		Improvements:     make([]*Improvement, 0),
		KnowledgeUpdates: make([]string, 0),
	}

	lines := splitLines(answer)
	currentLesson := (*Lesson)(nil)
	currentImprovement := (*Improvement)(nil)

	for i, line := range lines {
		lower := toLower(line)

		switch {
		case contains(lower, "pattern"):
			if currentLesson != nil {
				data.Lessons = append(data.Lessons, currentLesson)
			}
			currentLesson = &Lesson{}
		case contains(lower, "improvement") && !contains(lower, "improvement suggestion"):
			if currentImprovement != nil {
				data.Improvements = append(data.Improvements, currentImprovement)
			}
			currentImprovement = &Improvement{}
		case contains(lower, "knowledge update") || contains(lower, "key insight"):
			if i+1 < len(lines) {
				data.KnowledgeUpdates = append(data.KnowledgeUpdates, cleanLine(lines[i+1]))
			}
		case currentLesson != nil:
			if contains(lower, "context") && i+1 < len(lines) {
				currentLesson.Context = cleanLine(lines[i+1])
			} else if contains(lower, "recommendation") && i+1 < len(lines) {
				currentLesson.Recommendation = cleanLine(lines[i+1])
			} else if contains(lower, "confidence") && i+1 < len(lines) {
				// Parse confidence if needed
				currentLesson.Confidence = 0.8 // Default
			}
		case currentImprovement != nil:
			if contains(lower, "process") {
				currentImprovement.Category = ImprovementProcess
			} else if contains(lower, "tool") {
				currentImprovement.Category = ImprovementTooling
			} else if contains(lower, "document") {
				currentImprovement.Category = ImprovementDocs
			} else if contains(lower, "description") && i+1 < len(lines) {
				currentImprovement.Description = cleanLine(lines[i+1])
			} else if contains(lower, "impact") && i+1 < len(lines) {
				currentImprovement.Impact = cleanLine(lines[i+1])
			}
		}
	}

	if currentLesson != nil {
		data.Lessons = append(data.Lessons, currentLesson)
	}
	if currentImprovement != nil {
		data.Improvements = append(data.Improvements, currentImprovement)
	}

	return data
}

// GetTaskID returns the task ID.
func (r *LearnerResult) GetTaskID() string { return r.TaskID }

// GetDuration returns the learning duration.
func (r *LearnerResult) GetDuration() time.Duration { return r.Duration }

// GetError returns any error that occurred.
func (r *LearnerResult) GetError() error { return r.Error }
