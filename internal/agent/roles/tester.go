// Package roles defines the six core Agent roles for the Cloud Agent Platform.
package roles

import (
	"context"
	"fmt"
	"time"

	"github.com/cloud-agent-platform/cap/internal/agent/react"
	"go.uber.org/zap"
)

// TesterConfig holds configuration for the Tester role.
type TesterConfig struct {
	// Timeout is the timeout for testing.
	Timeout time.Duration
	// RequireMinimumCoverage requires minimum coverage threshold.
	RequireMinimumCoverage bool
	// MinimumCoverage is the minimum required coverage percentage.
	MinimumCoverage float64
}

// DefaultTesterConfig returns the default Tester configuration.
func DefaultTesterConfig() *TesterConfig {
	return &TesterConfig{
		Timeout:                120 * time.Second,
		RequireMinimumCoverage: true,
		MinimumCoverage:       80.0,
	}
}

// TesterResult represents the result of testing.
type TesterResult struct {
	// TaskID is the ULID of the task being tested.
	TaskID string `json:"task_id"`
	// TotalTests is the total number of tests.
	TotalTests int `json:"total_tests"`
	// PassedTests is the number of passed tests.
	PassedTests int `json:"passed_tests"`
	// FailedTests is the number of failed tests.
	FailedTests int `json:"failed_tests"`
	// SkippedTests is the number of skipped tests.
	SkippedTests int `json:"skipped_tests"`
	// LineCoverage is the line coverage percentage.
	LineCoverage float64 `json:"line_coverage"`
	// BranchCoverage is the branch coverage percentage.
	BranchCoverage float64 `json:"branch_coverage"`
	// CriticalPathsCovered indicates if critical paths are covered.
	CriticalPathsCovered bool `json:"critical_paths_covered"`
	// Correctness is the correctness assessment.
	Correctness string `json:"correctness"`
	// Completeness is the completeness assessment.
	Completeness string `json:"completeness"`
	// IssuesFound contains issues found.
	IssuesFound []string `json:"issues_found"`
	// Recommendations contains recommendations.
	Recommendations []string `json:"recommendations"`
	// Duration is how long the testing took.
	Duration time.Duration `json:"duration"`
	// Error records any error that occurred.
	Error error `json:"error,omitempty"`
}

// NewTesterAgent creates a new Tester agent.
func NewTesterAgent(llm react.LLM, tools *react.ToolRegistry, logger *zap.Logger) (*react.Agent, error) {
	config := react.DefaultConfig()
	config.SystemPrompt = RoleDefinitions[RoleTester].BuildPrompt()
	config.MaxIterations = 15
	config.Timeout = 120 * time.Second
	return react.NewAgent(llm, tools, config, logger)
}

// Test executes the testing workflow.
func Test(ctx context.Context, agent *react.Agent, taskID string, deliverables string, qualityStandards string) (*TesterResult, error) {
	start := time.Now()
	result := &TesterResult{
		TaskID:           taskID,
		IssuesFound:      make([]string, 0),
		Recommendations: make([]string, 0),
	}

	// Execute the ReAct agent
	actResult, err := agent.Run(ctx, fmt.Sprintf("Test and validate this deliverable:\n\nTask ID: %s\n\nDeliverable:\n%s\n\nQuality Standards:\n%s", taskID, deliverables, qualityStandards))
	if err != nil {
		result.Error = err
		result.Duration = time.Since(start)
		return result, err
	}

	// Parse test result
	parsed := parseTestResult(actResult.Answer)
	result.TotalTests = parsed.TotalTests
	result.PassedTests = parsed.PassedTests
	result.FailedTests = parsed.FailedTests
	result.SkippedTests = parsed.SkippedTests
	result.LineCoverage = parsed.LineCoverage
	result.BranchCoverage = parsed.BranchCoverage
	result.CriticalPathsCovered = parsed.CriticalPathsCovered
	result.Correctness = parsed.Correctness
	result.Completeness = parsed.Completeness
	result.IssuesFound = parsed.IssuesFound
	result.Recommendations = parsed.Recommendations
	result.Duration = time.Since(start)

	return result, nil
}

// parsedTestResult holds parsed test result data.
type parsedTestResultData struct {
	TotalTests           int
	PassedTests          int
	FailedTests          int
	SkippedTests         int
	LineCoverage         float64
	BranchCoverage       float64
	CriticalPathsCovered bool
	Correctness          string
	Completeness         string
	IssuesFound          []string
	Recommendations      []string
}

// parseTestResult parses the test result from the agent's answer.
func parseTestResult(answer string) *parsedTestResultData {
	data := &parsedTestResultData{
		IssuesFound:      make([]string, 0),
		Recommendations:  make([]string, 0),
		LineCoverage:     0,
		BranchCoverage:   0,
		CriticalPathsCovered: true,
	}

	lines := splitLines(answer)
	inIssues := false
	inRecommendations := false

	for i, line := range lines {
		lower := toLower(line)

		switch {
		case contains(lower, "total test"):
			if i+1 < len(lines) {
				data.TotalTests = parseIntFromLine(lines[i+1])
			}
		case contains(lower, "passed"):
			if contains(lower, "skip") {
				// Skip line
			} else if i+1 < len(lines) {
				data.PassedTests = parseIntFromLine(lines[i+1])
			}
		case contains(lower, "failed"):
			if i+1 < len(lines) {
				data.FailedTests = parseIntFromLine(lines[i+1])
			}
		case contains(lower, "skipped"):
			if i+1 < len(lines) {
				data.SkippedTests = parseIntFromLine(lines[i+1])
			}
		case contains(lower, "line coverage"):
			if i+1 < len(lines) {
				data.LineCoverage = parseFloatFromLine(lines[i+1])
			}
		case contains(lower, "branch coverage"):
			if i+1 < len(lines) {
				data.BranchCoverage = parseFloatFromLine(lines[i+1])
			}
		case contains(lower, "critical path"):
			if i+1 < len(lines) {
				data.CriticalPathsCovered = !contains(toLower(lines[i+1]), "not")
			}
		case contains(lower, "correctness"):
			if i+1 < len(lines) {
				data.Correctness = cleanLine(lines[i+1])
			}
		case contains(lower, "completeness"):
			if i+1 < len(lines) {
				data.Completeness = cleanLine(lines[i+1])
			}
		case contains(lower, "issue"):
			inIssues = true
			inRecommendations = false
		case contains(lower, "recommendation"):
			inRecommendations = true
			inIssues = false
		case inIssues:
			if contains(lower, "-") || contains(lower, "*") || contains(lower, "must") {
				data.IssuesFound = append(data.IssuesFound, cleanLine(line))
			} else if !contains(lower, "issue") && !contains(lower, ":") && len(line) > 0 && line[0] != '\n' {
				// Continue collecting issue description
				if len(data.IssuesFound) > 0 {
					data.IssuesFound[len(data.IssuesFound)-1] += " " + cleanLine(line)
				}
			}
		case inRecommendations:
			if contains(lower, "-") || contains(lower, "*") || contains(lower, "should") {
				data.Recommendations = append(data.Recommendations, cleanLine(line))
			}
		}
	}

	return data
}

// parseIntFromLine parses an integer from a line of text.
func parseIntFromLine(line string) int {
	line = cleanLine(line)
	var num int
	for _, c := range line {
		if c >= '0' && c <= '9' {
			num = num*10 + int(c-'0')
		}
	}
	return num
}

// parseFloatFromLine parses a float from a line of text.
func parseFloatFromLine(line string) float64 {
	line = cleanLine(line)
	var num float64
	var decimal float64 = 1
	inDecimal := false

	for _, c := range line {
		if c >= '0' && c <= '9' {
			if inDecimal {
				decimal /= 10
				num += float64(c-'0') * decimal
			} else {
				num = num*10 + float64(c-'0')
			}
		} else if c == '.' {
			inDecimal = true
			decimal = 1
		}
	}
	return num
}

// GetTaskID returns the task ID.
func (r *TesterResult) GetTaskID() string { return r.TaskID }

// GetDuration returns the testing duration.
func (r *TesterResult) GetDuration() time.Duration { return r.Duration }

// GetError returns any error that occurred.
func (r *TesterResult) GetError() error { return r.Error }
