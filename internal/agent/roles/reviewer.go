// Package roles defines the six core Agent roles for the Cloud Agent Platform.
package roles

import (
	"context"
	"fmt"
	"time"

	"github.com/cloud-agent-platform/cap/internal/agent/react"
	"go.uber.org/zap"
)

// ReviewerConfig holds configuration for the Reviewer role.
type ReviewerConfig struct {
	// Timeout is the timeout for review.
	Timeout time.Duration
	// StrictMode enables strict review criteria.
	StrictMode bool
	// RequireTestCoverage requires test coverage checks.
	RequireTestCoverage bool
}

// DefaultReviewerConfig returns the default Reviewer configuration.
func DefaultReviewerConfig() *ReviewerConfig {
	return &ReviewerConfig{
		Timeout:             60 * time.Second,
		StrictMode:          false,
		RequireTestCoverage: true,
	}
}

// ReviewerResult represents the result of a review.
type ReviewerResult struct {
	// TaskID is the ULID of the task being reviewed.
	TaskID string `json:"task_id"`
	// ReviewResult is the overall review outcome.
	ReviewResult ReviewOutcome `json:"review_result"`
	// Findings contains the identified issues.
	Findings []*Finding `json:"findings"`
	// Assessment contains overall assessments.
	Assessment *Assessment `json:"assessment"`
	// RevisionRequired indicates if revision is needed.
	RevisionRequired bool `json:"revision_required"`
	// RevisionNotes contain notes for revision.
	RevisionNotes string `json:"revision_notes"`
	// Duration is how long the review took.
	Duration time.Duration `json:"duration"`
	// Error records any error that occurred.
	Error error `json:"error,omitempty"`
}

// ReviewOutcome represents the review outcome.
type ReviewOutcome string

const (
	ReviewOutcomeApproved       ReviewOutcome = "approved"
	ReviewOutcomeNeedsRevision ReviewOutcome = "needs_revision"
	ReviewOutcomeRejected      ReviewOutcome = "rejected"
)

// Finding represents a single review finding.
type Finding struct {
	// Description describes the issue.
	Description string `json:"description"`
	// Location is where the issue was found.
	Location string `json:"location"`
	// Severity is the issue severity.
	Severity FindingSeverity `json:"severity"`
	// Suggestion is how to fix the issue.
	Suggestion string `json:"suggestion"`
}

// FindingSeverity represents the severity of a finding.
type FindingSeverity string

const (
	SeverityCritical FindingSeverity = "critical"
	SeverityMajor    FindingSeverity = "major"
	SeverityMinor    FindingSeverity = "minor"
)

// Assessment contains overall assessments.
type Assessment struct {
	// Correctness is the correctness assessment.
	Correctness string `json:"correctness"`
	// CodeQuality is the code quality assessment.
	CodeQuality string `json:"code_quality"`
	// TestCoverage is the test coverage assessment.
	TestCoverage string `json:"test_coverage"`
	// Documentation is the documentation assessment.
	Documentation string `json:"documentation"`
}

// NewReviewerAgent creates a new Reviewer agent.
func NewReviewerAgent(llm react.LLM, tools *react.ToolRegistry, logger *zap.Logger) (*react.Agent, error) {
	config := react.DefaultConfig()
	config.SystemPrompt = RoleDefinitions[RoleReviewer].BuildPrompt()
	config.MaxIterations = 10
	config.Timeout = 60 * time.Second
	return react.NewAgent(llm, tools, config, logger)
}

// Review executes the review workflow.
func Review(ctx context.Context, agent *react.Agent, taskID string, deliverables string, requirements string) (*ReviewerResult, error) {
	start := time.Now()
	result := &ReviewerResult{
		TaskID: taskID,
		Findings: make([]*Finding, 0),
	}

	// Execute the ReAct agent
	actResult, err := agent.Run(ctx, fmt.Sprintf("Review this deliverable:\n\nTask ID: %s\n\nDeliverable:\n%s\n\nRequirements:\n%s", taskID, deliverables, requirements))
	if err != nil {
		result.Error = err
		result.Duration = time.Since(start)
		return result, err
	}

	// Parse review result
	parsed := parseReview(actResult.Answer)
	result.ReviewResult = parsed.Outcome
	result.Findings = parsed.Findings
	result.Assessment = parsed.Assessment
	result.RevisionRequired = parsed.Outcome == ReviewOutcomeNeedsRevision
	result.RevisionNotes = parsed.Notes
	result.Duration = time.Since(start)

	return result, nil
}

// parsedReview holds parsed review data.
type parsedReviewData struct {
	Outcome   ReviewOutcome
	Findings  []*Finding
	Assessment *Assessment
	Notes     string
}

// parseReview parses the review result from the agent's answer.
func parseReview(answer string) *parsedReviewData {
	data := &parsedReviewData{
		Findings:  make([]*Finding, 0),
		Assessment: &Assessment{},
		Outcome: ReviewOutcomeApproved,
	}

	lines := splitLines(answer)
	currentFinding := (*Finding)(nil)

	for i, line := range lines {
		lower := toLower(line)

		switch {
		case contains(lower, "approved"):
			data.Outcome = ReviewOutcomeApproved
		case contains(lower, "needs revision") || contains(lower, "needs_revision"):
			data.Outcome = ReviewOutcomeNeedsRevision
		case contains(lower, "rejected"):
			data.Outcome = ReviewOutcomeRejected
		case contains(lower, "finding"):
			if currentFinding != nil {
				data.Findings = append(data.Findings, currentFinding)
			}
			currentFinding = &Finding{}
		case contains(lower, "severity"):
			if currentFinding != nil && i+1 < len(lines) {
				currentFinding.Severity = parseSeverity(cleanLine(lines[i+1]))
			}
		case contains(lower, "location"):
			if currentFinding != nil && i+1 < len(lines) {
				currentFinding.Location = cleanLine(lines[i+1])
			}
		case contains(lower, "suggestion"):
			if currentFinding != nil && i+1 < len(lines) {
				currentFinding.Suggestion = cleanLine(lines[i+1])
			}
		case contains(lower, "correctness"):
			if i+1 < len(lines) {
				data.Assessment.Correctness = cleanLine(lines[i+1])
			}
		case contains(lower, "code quality"):
			if i+1 < len(lines) {
				data.Assessment.CodeQuality = cleanLine(lines[i+1])
			}
		case contains(lower, "test coverage"):
			if i+1 < len(lines) {
				data.Assessment.TestCoverage = cleanLine(lines[i+1])
			}
		case contains(lower, "documentation"):
			if i+1 < len(lines) {
				data.Assessment.Documentation = cleanLine(lines[i+1])
			}
		case contains(lower, "revision note"):
			if i+1 < len(lines) {
				data.Notes = cleanLine(lines[i+1])
			}
		}

		// Capture description if in finding context
		if currentFinding != nil && currentFinding.Description == "" && !contains(lower, "finding") && !contains(lower, "severity") && !contains(lower, "location") && !contains(lower, "suggestion") {
			if len(lines) > i && i > 0 {
				prevLower := toLower(lines[i-1])
				if contains(prevLower, "finding") || contains(prevLower, "-") || contains(prevLower, "*") {
					currentFinding.Description = cleanLine(line)
				}
			}
		}
	}

	if currentFinding != nil {
		data.Findings = append(data.Findings, currentFinding)
	}

	return data
}

// parseSeverity parses a severity level from a string.
func parseSeverity(s string) FindingSeverity {
	s = toLower(s)
	switch {
	case contains(s, "critical"):
		return SeverityCritical
	case contains(s, "major"):
		return SeverityMajor
	case contains(s, "minor"):
		return SeverityMinor
	default:
		return SeverityMinor
	}
}

// GetTaskID returns the task ID.
func (r *ReviewerResult) GetTaskID() string { return r.TaskID }

// GetDuration returns the review duration.
func (r *ReviewerResult) GetDuration() time.Duration { return r.Duration }

// GetError returns any error that occurred.
func (r *ReviewerResult) GetError() error { return r.Error }
