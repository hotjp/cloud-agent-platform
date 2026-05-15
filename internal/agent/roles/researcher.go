// Package roles defines the six core Agent roles for the Cloud Agent Platform.
package roles

import (
	"context"
	"fmt"
	"time"

	"github.com/cloud-agent-platform/cap/internal/agent/react"
	"go.uber.org/zap"
)

// ResearcherConfig holds configuration for the Researcher role.
type ResearcherConfig struct {
	// Timeout is the timeout for research.
	Timeout time.Duration
	// MaxSources is the maximum number of sources to gather.
	MaxSources int
	// MinConfidence is the minimum confidence threshold for recommendations.
	MinConfidence float64
}

// DefaultResearcherConfig returns the default Researcher configuration.
func DefaultResearcherConfig() *ResearcherConfig {
	return &ResearcherConfig{
		Timeout:       90 * time.Second,
		MaxSources:    10,
		MinConfidence: 0.7,
	}
}

// ResearcherResult represents the result of research.
type ResearcherResult struct {
	// TaskID is the ULID of the task being researched.
	TaskID string `json:"task_id"`
	// Topic is the research topic.
	Topic string `json:"topic"`
	// Summary is a concise summary of findings.
	Summary string `json:"summary"`
	// Sources contains the sources gathered.
	Sources []Source `json:"sources"`
	// AlternativesAnalysis contains analysis of alternatives.
	AlternativesAnalysis []*Alternative `json:"alternatives_analysis"`
	// KnowledgeGaps contains areas needing more research.
	KnowledgeGaps []string `json:"knowledge_gaps"`
	// Recommendations contains evidence-backed recommendations.
	Recommendations []string `json:"recommendations"`
	// Duration is how long the research took.
	Duration time.Duration `json:"duration"`
	// Error records any error that occurred.
	Error error `json:"error,omitempty"`
}

// Source represents a research source.
type Source struct {
	// Index is the source index number.
	Index int `json:"index"`
	// Description describes the source.
	Description string `json:"description"`
	// URL is the source URL if applicable.
	URL string `json:"url,omitempty"`
}

// Alternative represents an alternative solution analysis.
type Alternative struct {
	// Name is the alternative name.
	Name string `json:"name"`
	// Description describes the alternative.
	Description string `json:"description"`
	// Pros contains the pros of the alternative.
	Pros []string `json:"pros"`
	// Cons contains the cons of the alternative.
	Cons []string `json:"cons"`
}

// NewResearcherAgent creates a new Researcher agent.
func NewResearcherAgent(llm react.LLM, tools *react.ToolRegistry, logger *zap.Logger) (*react.Agent, error) {
	config := react.DefaultConfig()
	config.SystemPrompt = RoleDefinitions[RoleResearcher].BuildPrompt()
	config.MaxIterations = 12
	config.Timeout = 90 * time.Second
	return react.NewAgent(llm, tools, config, logger)
}

// Research executes the research workflow.
func Research(ctx context.Context, agent *react.Agent, taskID string, topic string, existingKnowledge string) (*ResearcherResult, error) {
	start := time.Now()
	result := &ResearcherResult{
		TaskID:          taskID,
		Topic:           topic,
		Sources:         make([]Source, 0),
		AlternativesAnalysis: make([]*Alternative, 0),
		KnowledgeGaps:   make([]string, 0),
		Recommendations: make([]string, 0),
	}

	// Execute the ReAct agent
	actResult, err := agent.Run(ctx, fmt.Sprintf("Research this topic and provide findings:\n\nTask ID: %s\n\nTopic:\n%s\n\nExisting Knowledge:\n%s", taskID, topic, existingKnowledge))
	if err != nil {
		result.Error = err
		result.Duration = time.Since(start)
		return result, err
	}

	// Parse research result
	parsed := parseResearchResult(actResult.Answer)
	result.Summary = parsed.Summary
	result.Sources = parsed.Sources
	result.AlternativesAnalysis = parsed.AlternativesAnalysis
	result.KnowledgeGaps = parsed.KnowledgeGaps
	result.Recommendations = parsed.Recommendations
	result.Duration = time.Since(start)

	return result, nil
}

// parsedResearchResult holds parsed research result data.
type parsedResearchResultData struct {
	Summary             string
	Sources             []Source
	AlternativesAnalysis []*Alternative
	KnowledgeGaps       []string
	Recommendations     []string
}

// parseResearchResult parses the research result from the agent's answer.
func parseResearchResult(answer string) *parsedResearchResultData {
	data := &parsedResearchResultData{
		Sources:             make([]Source, 0),
		AlternativesAnalysis: make([]*Alternative, 0),
		KnowledgeGaps:       make([]string, 0),
		Recommendations:     make([]string, 0),
	}

	lines := splitLines(answer)
	inSources := false
	inPros := false
	inCons := false
	inKnowledgeGaps := false
	inRecommendations := false
	currentAlternative := (*Alternative)(nil)
	sourceIndex := 0

	for i, line := range lines {
		lower := toLower(line)

		switch {
		case contains(lower, "summary"):
			if i+1 < len(lines) {
				data.Summary = cleanLine(lines[i+1])
			}
		case contains(lower, "source"):
			inSources = true
			inKnowledgeGaps = false
			inRecommendations = false
			sourceIndex++
		case contains(lower, "alternative"):
			if currentAlternative != nil {
				data.AlternativesAnalysis = append(data.AlternativesAnalysis, currentAlternative)
			}
			inSources = false
			inKnowledgeGaps = false
			inRecommendations = false
			inPros = false
			inCons = false
			currentAlternative = &Alternative{}
			if i+1 < len(lines) {
				currentAlternative.Name = cleanLine(lines[i+1])
			}
		case contains(lower, "pro"):
			if currentAlternative != nil {
				inPros = true
				inCons = false
			}
		case contains(lower, "con"):
			if currentAlternative != nil {
				inCons = true
				inPros = false
			}
		case contains(lower, "knowledge gap"):
			inKnowledgeGaps = true
			inSources = false
			inRecommendations = false
		case contains(lower, "recommendation"):
			inRecommendations = true
			inSources = false
			inKnowledgeGaps = false
		case inSources:
			if contains(lower, "-") || contains(lower, "*") || contains(lower, "[") {
				source := Source{Index: sourceIndex}
				desc := cleanLine(line)
				// Check if URL is present
				if contains(desc, "http") {
					for j := len(desc) - 1; j >= 0; j-- {
						if desc[j] == ')' || desc[j] == ']' {
							if j+1 < len(desc) && (desc[j+1] == ' ' || contains(desc[j+1:], "http") == false) {
								break
							}
						}
					}
				}
				source.Description = desc
				data.Sources = append(data.Sources, source)
			}
		case inPros:
			if currentAlternative != nil {
				if contains(lower, "-") || contains(lower, "*") {
					currentAlternative.Pros = append(currentAlternative.Pros, cleanLine(line))
				} else if !contains(lower, "pro") && len(line) > 0 && line[0] != '\n' {
					if len(currentAlternative.Pros) > 0 {
						currentAlternative.Pros[len(currentAlternative.Pros)-1] += " " + cleanLine(line)
					}
				}
			}
		case inCons:
			if currentAlternative != nil {
				if contains(lower, "-") || contains(lower, "*") {
					currentAlternative.Cons = append(currentAlternative.Cons, cleanLine(line))
				} else if !contains(lower, "con") && len(line) > 0 && line[0] != '\n' {
					if len(currentAlternative.Cons) > 0 {
						currentAlternative.Cons[len(currentAlternative.Cons)-1] += " " + cleanLine(line)
					}
				}
			}
		case inKnowledgeGaps:
			if contains(lower, "-") || contains(lower, "*") {
				data.KnowledgeGaps = append(data.KnowledgeGaps, cleanLine(line))
			}
		case inRecommendations:
			if contains(lower, "-") || contains(lower, "*") || contains(lower, "should") {
				data.Recommendations = append(data.Recommendations, cleanLine(line))
			}
		}
	}

	if currentAlternative != nil {
		data.AlternativesAnalysis = append(data.AlternativesAnalysis, currentAlternative)
	}

	return data
}

// GetTaskID returns the task ID.
func (r *ResearcherResult) GetTaskID() string { return r.TaskID }

// GetDuration returns the research duration.
func (r *ResearcherResult) GetDuration() time.Duration { return r.Duration }

// GetError returns any error that occurred.
func (r *ResearcherResult) GetError() error { return r.Error }
