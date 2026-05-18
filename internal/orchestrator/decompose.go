// Package orchestrator implements L4 orchestration: task scheduling, agent session
// management, and event-driven workflow coordination.
package orchestrator

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/cloud-agent-platform/cap/internal/domain"
)

// ----------------------------------------------------------------------------
// Decomposition Strategy
// ----------------------------------------------------------------------------

// DecompositionStrategy defines how to decompose a complex task.
type DecompositionStrategy string

const (
	// StrategyByFile decomposes by individual files.
	StrategyByFile DecompositionStrategy = "by_file"
	// StrategyByModule decomposes by modules/packages.
	StrategyByModule DecompositionStrategy = "by_module"
	// StrategyByFeature decomposes by features/use cases.
	StrategyByFeature DecompositionStrategy = "by_feature"
	// StrategyAuto automatically determines the best strategy.
	StrategyAuto DecompositionStrategy = "auto"
)

// ----------------------------------------------------------------------------
// Decompose Options
// ----------------------------------------------------------------------------

// DecomposeOptions controls task decomposition behavior.
type DecomposeOptions struct {
	// Strategy is the decomposition strategy to use.
	Strategy DecompositionStrategy
	// MaxSubtasks limits the maximum number of subtasks (0 = unlimited).
	MaxSubtasks int
	// IncludeTests includes test files in decomposition.
	IncludeTests bool
	// ParallelThreshold minimum subtasks before enabling parallel execution.
	ParallelThreshold int
}

// DefaultDecomposeOptions returns default decomposition options.
func DefaultDecomposeOptions() DecomposeOptions {
	return DecomposeOptions{
		Strategy:         StrategyAuto,
		MaxSubtasks:       20,
		IncludeTests:      true,
		ParallelThreshold: 3,
	}
}

// ----------------------------------------------------------------------------
// Decomposition Result
// ----------------------------------------------------------------------------

// DecompositionResult contains the result of task decomposition.
type DecompositionResult struct {
	// Subtasks is the list of decomposed subtasks.
	Subtasks []*domain.Subtask
	// Strategy used for decomposition.
	Strategy DecompositionStrategy
	// ExecutionOrder is the recommended execution order (topologically sorted).
	ExecutionOrder []string
	// PriorityMap maps subtask ID to priority (0-9).
	PriorityMap map[string]int
	// DependencyGraph describes the dependency relationships.
	DependencyGraph map[string][]string
}

// ----------------------------------------------------------------------------
// Task Decomposer
// ----------------------------------------------------------------------------

// TaskDecomposer handles task decomposition into subtasks.
type TaskDecomposer struct {
	subtaskRepo domain.SubtaskRepository
	logger      interface {
		Info(msg string, fields ...interface{})
		Warn(msg string, fields ...interface{})
		Error(msg string, fields ...interface{})
	}
}

// NewTaskDecomposer creates a new TaskDecomposer.
func NewTaskDecomposer(subtaskRepo domain.SubtaskRepository) *TaskDecomposer {
	return &TaskDecomposer{
		subtaskRepo: subtaskRepo,
	}
}

// DecomposeTask decomposes a complex task into subtasks.
// It analyzes the task goal and creates appropriate subtasks with dependencies.
func (d *TaskDecomposer) DecomposeTask(ctx context.Context, task *domain.Task, opts DecomposeOptions) (*DecompositionResult, error) {
	if task == nil {
		return nil, fmt.Errorf("task is nil")
	}

	// Determine strategy
	strategy := d.determineStrategy(task, opts)

	// Parse task goal to extract components
	components := d.parseTaskGoal(task.Goal)

	// Generate subtasks based on strategy
	subtasks := d.generateSubtasks(task, components, strategy, opts)

	// Limit subtasks if needed
	if opts.MaxSubtasks > 0 && len(subtasks) > opts.MaxSubtasks {
		subtasks = subtasks[:opts.MaxSubtasks]
	}

	// Analyze dependencies between subtasks
	dependencyGraph := d.analyzeDependencies(subtasks, components)

	// Assign priorities based on dependencies
	priorityMap := d.assignPriorities(subtasks, dependencyGraph)

	// Compute execution order (topological sort)
	executionOrder := d.computeExecutionOrder(subtasks, dependencyGraph)

	result := &DecompositionResult{
		Subtasks:        subtasks,
		Strategy:        strategy,
		ExecutionOrder:  executionOrder,
		PriorityMap:     priorityMap,
		DependencyGraph: dependencyGraph,
	}

	return result, nil
}

// determineStrategy selects the best decomposition strategy.
func (d *TaskDecomposer) determineStrategy(task *domain.Task, opts DecomposeOptions) DecompositionStrategy {
	if opts.Strategy != StrategyAuto {
		return opts.Strategy
	}

	goal := strings.ToLower(task.Goal)

	// Heuristics for strategy selection
	if strings.Contains(goal, "file") || strings.Contains(goal, "modify") || strings.Contains(goal, "update") {
		return StrategyByFile
	}
	if strings.Contains(goal, "module") || strings.Contains(goal, "package") || strings.Contains(goal, "service") {
		return StrategyByModule
	}
	if strings.Contains(goal, "feature") || strings.Contains(goal, "implement") || strings.Contains(goal, "add") {
		return StrategyByFeature
	}

	return StrategyByFeature
}

// parseTaskGoal extracts meaningful components from the task goal.
func (d *TaskDecomposer) parseTaskGoal(goal string) []TaskComponent {
	var components []TaskComponent

	// Split by common delimiters
	segments := splitGoalSegments(goal)

	for i, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}

		component := TaskComponent{
			ID:          fmt.Sprintf("component-%d", i+1),
			Description: segment,
			Type:        classifyComponent(segment),
		}
		components = append(components, component)
	}

	return components
}

// splitGoalSegments splits the goal into logical segments.
func splitGoalSegments(goal string) []string {
	// Remove common prefixes
	goal = strings.TrimPrefix(goal, "I need to ")
	goal = strings.TrimPrefix(goal, "Please ")
	goal = strings.TrimPrefix(goal, "Could you ")

	// Split by conjunctions and punctuation
	delimiters := []string{" and ", " then ", ". ", "; ", "\n"}

	segments := []string{goal}
	for _, delim := range delimiters {
		var newSegments []string
		for _, seg := range segments {
			newSegments = append(newSegments, splitAndKeep(seg, delim)...)
		}
		if len(newSegments) > 0 {
			segments = newSegments
		}
	}

	if len(segments) == 0 {
		segments = []string{goal}
	}

	// Final pass with primary delimiter
	var result []string
	for _, seg := range segments {
		parts := strings.Split(seg, " and ")
		result = append(result, parts...)
	}

	return result
}

func splitAndKeep(s, delim string) []string {
	parts := strings.Split(s, delim)
	var result []string
	for _, p := range parts {
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// classifyComponent determines the type of a task component.
func classifyComponent(segment string) domain.SubtaskType {
	segment = strings.ToLower(segment)

	if strings.Contains(segment, "test") || strings.Contains(segment, "verify") || strings.Contains(segment, "check") {
		return domain.SubtaskTypeTesting
	}
	if strings.Contains(segment, "review") || strings.Contains(segment, "refactor") || strings.Contains(segment, "improve") {
		return domain.SubtaskTypeReview
	}
	if strings.Contains(segment, "analyze") || strings.Contains(segment, "investigate") || strings.Contains(segment, "explore") {
		return domain.SubtaskTypeAnalysis
	}
	if strings.Contains(segment, "research") || strings.Contains(segment, "find") || strings.Contains(segment, "search") {
		return domain.SubtaskTypeResearch
	}

	return domain.SubtaskTypeCoding
}

// generateSubtasks creates subtasks based on the selected strategy.
func (d *TaskDecomposer) generateSubtasks(task *domain.Task, components []TaskComponent, strategy DecompositionStrategy, opts DecomposeOptions) []*domain.Subtask {
	var subtasks []*domain.Subtask

	switch strategy {
	case StrategyByFile:
		subtasks = d.generateFileBasedSubtasks(task, components)
	case StrategyByModule:
		subtasks = d.generateModuleBasedSubtasks(task, components)
	case StrategyByFeature:
		subtasks = d.generateFeatureBasedSubtasks(task, components)
	default:
		subtasks = d.generateFeatureBasedSubtasks(task, components)
	}

	// Assign agent templates based on component types
	for _, st := range subtasks {
		if st.AgentTemplate == "" {
			st.AgentTemplate = d.defaultAgentTemplate(st.Type)
		}
	}

	return subtasks
}

// generateFileBasedSubtasks decomposes by individual files.
func (d *TaskDecomposer) generateFileBasedSubtasks(task *domain.Task, components []TaskComponent) []*domain.Subtask {
	var subtasks []*domain.Subtask

	for i, comp := range components {
		subtask := domain.NewSubtask(
			domain.NewULID(),
			task.ID,
			comp.Type,
			fmt.Sprintf("Work on file: %s", comp.Description),
			"",
		)
		subtask.Dependencies = []string{}
		subtasks = append(subtasks, subtask)

		_ = i // component index may be used for ordering
	}

	return subtasks
}

// generateModuleBasedSubtasks decomposes by modules/packages.
func (d *TaskDecomposer) generateModuleBasedSubtasks(task *domain.Task, components []TaskComponent) []*domain.Subtask {
	var subtasks []*domain.Subtask

	// Group components by module
	moduleMap := make(map[string][]TaskComponent)
	for _, comp := range components {
		moduleName := extractModuleName(comp.Description)
		moduleMap[moduleName] = append(moduleMap[moduleName], comp)
	}

	moduleNames := sortedKeys(moduleMap)
	for _, moduleName := range moduleNames {
		comps := moduleMap[moduleName]
		if len(comps) == 0 {
			continue
		}

		// Create one subtask per module
		description := fmt.Sprintf("Work on module %s: %s", moduleName, comps[0].Description)
		subtask := domain.NewSubtask(
			domain.NewULID(),
			task.ID,
			comps[0].Type,
			description,
			"",
		)
		subtasks = append(subtasks, subtask)
	}

	return subtasks
}

// generateFeatureBasedSubtasks decomposes by features/use cases.
func (d *TaskDecomposer) generateFeatureBasedSubtasks(task *domain.Task, components []TaskComponent) []*domain.Subtask {
	var subtasks []*domain.Subtask

	for _, comp := range components {
		subtask := domain.NewSubtask(
			domain.NewULID(),
			task.ID,
			comp.Type,
			comp.Description,
			"",
		)
		subtasks = append(subtasks, subtask)
	}

	return subtasks
}

// extractModuleName extracts module/package name from a description.
func extractModuleName(description string) string {
	// Common patterns for module names
	patterns := []string{
		"module ", "package ", "service ", "component ",
	}

	lower := strings.ToLower(description)
	for _, pattern := range patterns {
		if idx := strings.Index(lower, pattern); idx >= 0 {
			end := strings.IndexAny(description[idx+len(pattern):], " ,;.\n")
			if end > 0 {
				return description[idx+len(pattern) : idx+len(pattern)+end]
			}
			return description[idx+len(pattern):]
		}
	}

	// Default: first word
	parts := strings.Fields(description)
	if len(parts) > 0 {
		return parts[0]
	}
	return "unknown"
}

func sortedKeys(m map[string][]TaskComponent) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// defaultAgentTemplate returns the default agent template for a subtask type.
func (d *TaskDecomposer) defaultAgentTemplate(subType domain.SubtaskType) string {
	switch subType {
	case domain.SubtaskTypeTesting:
		return "tester"
	case domain.SubtaskTypeReview:
		return "reviewer"
	case domain.SubtaskTypeAnalysis:
		return "analyst"
	case domain.SubtaskTypeResearch:
		return "researcher"
	default:
		return "executor"
	}
}

// analyzeDependencies identifies dependencies between subtasks.
func (d *TaskDecomposer) analyzeDependencies(subtasks []*domain.Subtask, components []TaskComponent) map[string][]string {
	graph := make(map[string][]string)

	// Initialize empty dependency lists
	for _, st := range subtasks {
		graph[st.ID] = []string{}
	}

	// Analyze dependencies based on component ordering and types
	for i, st := range subtasks {
		// Dependencies on earlier components of certain types
		for j := 0; j < i; j++ {
			prev := subtasks[j]

			// Research/Analysis should complete before Coding
			if (st.Type == domain.SubtaskTypeCoding || st.Type == domain.SubtaskTypeTesting) &&
				(prev.Type == domain.SubtaskTypeResearch || prev.Type == domain.SubtaskTypeAnalysis) {
				graph[st.ID] = append(graph[st.ID], prev.ID)
				continue
			}

			// Coding should complete before Review
			if st.Type == domain.SubtaskTypeReview && prev.Type == domain.SubtaskTypeCoding {
				graph[st.ID] = append(graph[st.ID], prev.ID)
				continue
			}

			// Testing depends on Coding
			if st.Type == domain.SubtaskTypeTesting && prev.Type == domain.SubtaskTypeCoding {
				graph[st.ID] = append(graph[st.ID], prev.ID)
			}
		}
	}

	// Remove duplicate dependencies
	for id := range graph {
		graph[id] = uniqueStrings(graph[id])
	}

	return graph
}

// uniqueStrings removes duplicate strings from a slice.
func uniqueStrings(ss []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(ss))
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

// assignPriorities assigns priorities based on dependencies.
// Subtasks with no dependencies get higher priority (can execute first).
func (d *TaskDecomposer) assignPriorities(subtasks []*domain.Subtask, graph map[string][]string) map[string]int {
	priorityMap := make(map[string]int)

	// Count incoming edges (dependencies) for each subtask
	inDegree := make(map[string]int)
	for _, st := range subtasks {
		inDegree[st.ID] = 0
	}
	for _, deps := range graph {
		for _, dep := range deps {
			inDegree[dep]++
		}
	}

	// Assign priority based on "distance" from root (0 dependency = high priority)
	// Use reverse topological level: nodes with no incoming edges get highest priority
	visited := make(map[string]bool)
	var assignLevel func(id string, level int)
	assignLevel = func(id string, level int) {
		if visited[id] {
			return
		}
		visited[id] = true

		// Priority is inverse of level (level 0 = priority 9)
		priority := 9 - level
		if priority < 1 {
			priority = 1
		}
		priorityMap[id] = priority

		// Process dependents
		for _, st := range subtasks {
			for _, dep := range graph[st.ID] {
				if dep == id {
					assignLevel(st.ID, level+1)
				}
			}
		}
	}

	// Start with nodes that have no dependencies
	for _, st := range subtasks {
		if inDegree[st.ID] == 0 {
			assignLevel(st.ID, 0)
		}
	}

	// Handle any remaining unvisited nodes (cycles, etc.)
	for _, st := range subtasks {
		if !visited[st.ID] {
			assignLevel(st.ID, 5) // Default mid priority
		}
	}

	return priorityMap
}

// computeExecutionOrder returns subtask IDs in topologically sorted order.
func (d *TaskDecomposer) computeExecutionOrder(subtasks []*domain.Subtask, graph map[string][]string) []string {
	if len(subtasks) == 0 {
		return nil
	}

	// Build adjacency list and in-degree count
	adjList := make(map[string][]string)
	inDegree := make(map[string]int)

	for _, st := range subtasks {
		adjList[st.ID] = []string{}
		inDegree[st.ID] = 0
	}

	for id, deps := range graph {
		for _, dep := range deps {
			adjList[dep] = append(adjList[dep], id)
			inDegree[id]++
		}
	}

	// Kahn's algorithm for topological sort
	var queue []string
	for _, st := range subtasks {
		if inDegree[st.ID] == 0 {
			queue = append(queue, st.ID)
		}
	}

	var result []string
	for len(queue) > 0 {
		// Sort queue for deterministic output
		sort.Strings(queue)
		current := queue[0]
		queue = queue[1:]

		result = append(result, current)

		for _, neighbor := range adjList[current] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	// If result doesn't include all nodes, there's a cycle
	// Fall back to simple order
	if len(result) < len(subtasks) {
		result = make([]string, 0, len(subtasks))
		for _, st := range subtasks {
			result = append(result, st.ID)
		}
	}

	return result
}

// SaveSubtasks persists subtasks to the repository.
func (d *TaskDecomposer) SaveSubtasks(ctx context.Context, subtasks []*domain.Subtask) error {
	for _, st := range subtasks {
		if _, err := d.subtaskRepo.Create(ctx, st); err != nil {
			return fmt.Errorf("failed to save subtask %s: %w", st.ID, err)
		}
	}
	return nil
}

// TaskComponent represents a parsed component of a task goal.
type TaskComponent struct {
	ID          string
	Description string
	Type        domain.SubtaskType
}