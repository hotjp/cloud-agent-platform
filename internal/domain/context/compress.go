package context

import (
	"fmt"
	"sort"
)

// ----------------------------------------------------------------------------
// CompressionResult
// ----------------------------------------------------------------------------

// CompressionResult contains the result of a context compression operation.
type CompressionResult struct {
	// OriginalTokens is the token count before compression.
	OriginalTokens int
	// CompressedTokens is the token count after compression.
	CompressedTokens int
	// TokensRemoved is the number of tokens removed.
	TokensRemoved int
	// ItemsRemoved is the count of individual items removed by type.
	ItemsRemoved CompressionStats
	// BudgetAfter is the remaining budget after compression.
	BudgetAfter int
	// BudgetUsed is the percentage of budget used after compression.
	BudgetUsed float64
}

// CompressionStats tracks how many items of each type were removed.
type CompressionStats struct {
	Messages    int
	FileStates   int
	Constraints int
}

// CompressionSummary returns a human-readable summary of the compression.
func (r *CompressionResult) CompressionSummary() string {
	return fmt.Sprintf(
		"compressed %d tokens (%d removed), items: %d msgs, %d files, %d constraints, budget used: %.1f%%",
		r.CompressedTokens, r.TokensRemoved,
		r.ItemsRemoved.Messages, r.ItemsRemoved.FileStates, r.ItemsRemoved.Constraints,
		r.BudgetUsed*100,
	)
}

// ----------------------------------------------------------------------------
// CompressionStrategy
// ----------------------------------------------------------------------------

// CompressionStrategy defines how to prioritize and compress context.
type CompressionStrategy string

const (
	// StrategyAggressive removes low-priority items first, preserving key information.
	StrategyAggressive CompressionStrategy = "aggressive"
	// StrategyModerate balances preservation of information with budget constraints.
	StrategyModerate CompressionStrategy = "moderate"
	// StrategyConservative preserves most information, only removing clearly expendable items.
	StrategyConservative CompressionStrategy = "conservative"
)

// String returns the string representation of CompressionStrategy.
func (s CompressionStrategy) String() string { return string(s) }

// IsValid reports whether s is a known CompressionStrategy.
func (s CompressionStrategy) IsValid() bool {
	switch s {
	case StrategyAggressive, StrategyModerate, StrategyConservative:
		return true
	}
	return false
}

// ----------------------------------------------------------------------------
// Compressor
// ----------------------------------------------------------------------------

// Compressor provides context compression functionality.
// It reduces the context to fit within the token budget while preserving
// critical information (hard constraints, recent messages, modified files).
type Compressor struct {
	// strategy determines how aggressively to compress.
	strategy CompressionStrategy
	// preserveRecentMessages is the number of recent messages to always preserve.
	preserveRecentMessages int
	// preserveRecentFiles is the number of recent file states to always preserve.
	preserveRecentFiles int
}

// NewCompressor creates a new Compressor with the given strategy.
func NewCompressor(strategy CompressionStrategy) *Compressor {
	preserveRecent := 10
	preserveRecentFiles := 5

	switch strategy {
	case StrategyAggressive:
		preserveRecent = 5
		preserveRecentFiles = 3
	case StrategyModerate:
		preserveRecent = 10
		preserveRecentFiles = 5
	case StrategyConservative:
		preserveRecent = 20
		preserveRecentFiles = 10
	}

	return &Compressor{
		strategy:              strategy,
		preserveRecentMessages: preserveRecent,
		preserveRecentFiles:   preserveRecentFiles,
	}
}

// DefaultCompressor creates a Compressor with the default (moderate) strategy.
func DefaultCompressor() *Compressor {
	return NewCompressor(StrategyModerate)
}

// Compress compresses the given TaskContext to fit within its token budget.
// Returns a CompressionResult describing what was changed.
// The original context is modified in place.
func (c *Compressor) Compress(tc *TaskContext) (*CompressionResult, error) {
	if tc == nil {
		return nil, fmt.Errorf("cannot compress nil context")
	}

	result := &CompressionResult{
		OriginalTokens: tc.TotalTokens(),
		BudgetAfter:    tc.RemainingBudget(),
	}

	if !tc.IsBudgetExceeded() {
		result.CompressedTokens = result.OriginalTokens
		result.BudgetUsed = float64(result.OriginalTokens) / float64(tc.TokenBudget)
		return result, nil
	}

	// Step 1: Compress messages (highest impact on token count)
	c.compressMessages(tc, result)

	// Step 2: Compress file states
	c.compressFileStates(tc, result)

	// Step 3: Compress constraints (preserve hard constraints)
	c.compressConstraints(tc, result)

	// Recalculate and verify
	tc.RecalculateTokens()
	result.CompressedTokens = tc.TotalTokens()
	result.TokensRemoved = result.OriginalTokens - result.CompressedTokens
	result.BudgetAfter = tc.TokenBudget - result.CompressedTokens
	result.BudgetUsed = float64(result.CompressedTokens) / float64(tc.TokenBudget)

	return result, nil
}

// compressMessages removes low-priority or old messages to reduce token count.
func (c *Compressor) compressMessages(tc *TaskContext, result *CompressionResult) {
	if len(tc.Messages) <= c.preserveRecentMessages {
		return
	}

	// Sort messages by priority (lower = higher priority to keep)
	// but always preserve the most recent messages
	sorted := make([]*ConversationMessage, len(tc.Messages))
	copy(sorted, tc.Messages)

	sort.Slice(sorted, func(i, j int) bool {
		// First, by priority (lower priority number = higher priority to keep)
		if sorted[i].Priority != sorted[j].Priority {
			return sorted[i].Priority < sorted[j].Priority
		}
		// Then by timestamp (newer = higher priority to keep)
		return sorted[i].Timestamp.After(sorted[j].Timestamp)
	})

	// Identify messages to remove (lowest priority, oldest first)
	// but always keep the most recent c.preserveRecentMessages
	removeCount := len(tc.Messages) - c.preserveRecentMessages
	if removeCount <= 0 {
		return
	}

	// Build set of IDs to remove
	toRemove := make(map[string]bool)
	removedCount := 0

	// First pass: remove oldest, lowest priority messages
	for _, m := range sorted {
		if removedCount >= removeCount {
			break
		}
		// Skip system messages (high value)
		if m.Role == MessageRoleSystem {
			continue
		}
		// Skip user messages with high priority
		if m.Role == MessageRoleUser && m.Priority < 5 {
			continue
		}
		toRemove[m.ID] = true
		removedCount++
		result.ItemsRemoved.Messages++
	}

	// Build new messages slice, preserving order
	var newMessages []*ConversationMessage
	for _, m := range tc.Messages {
		if !toRemove[m.ID] {
			newMessages = append(newMessages, m)
		}
	}

	tc.Messages = newMessages
	tc.RecalculateTokens()
}

// compressFileStates removes low-priority or less important file states.
func (c *Compressor) compressFileStates(tc *TaskContext, result *CompressionResult) {
	if len(tc.FileStates) <= c.preserveRecentFiles {
		return
	}

	// Sort file states by priority and recency
	sorted := make([]*FileState, len(tc.FileStates))
	copy(sorted, tc.FileStates)

	sort.Slice(sorted, func(i, j int) bool {
		// First by priority (lower = higher priority to keep)
		if sorted[i].Priority != sorted[j].Priority {
			return sorted[i].Priority < sorted[j].Priority
		}
		// Then by modified time (newer = higher priority)
		return sorted[i].ModifiedAt.After(sorted[j].ModifiedAt)
	})

	// Always keep deleted files (they're small and important)
	// and recently modified files
	removeCount := len(tc.FileStates) - c.preserveRecentFiles
	if removeCount <= 0 {
		return
	}

	toRemove := make(map[string]bool)
	removedCount := 0

	for _, f := range sorted {
		if removedCount >= removeCount {
			break
		}
		// Never remove deleted files
		if f.ChangeType == FileChangeDeleted {
			continue
		}
		// Never remove created files (they're the output)
		if f.ChangeType == FileChangeCreated {
			continue
		}
		toRemove[f.ID] = true
		removedCount++
		result.ItemsRemoved.FileStates++
	}

	var newFileStates []*FileState
	for _, f := range tc.FileStates {
		if !toRemove[f.ID] {
			newFileStates = append(newFileStates, f)
		}
	}

	tc.FileStates = newFileStates
}

// compressConstraints removes low-priority soft constraints.
// Hard (NON_NEGOTIABLE) constraints are NEVER removed.
func (c *Compressor) compressConstraints(tc *TaskContext, result *CompressionResult) {
	// Count current soft constraints
	softCount := 0
	for _, c := range tc.Constraints {
		if !c.IsHard() {
			softCount++
		}
	}

	if softCount == 0 {
		return
	}

	// Sort soft constraints by priority
	var softConstraints []*Constraint
	var hardConstraints []*Constraint

	for _, constraint := range tc.Constraints {
		if constraint.IsHard() {
			hardConstraints = append(hardConstraints, constraint)
		} else {
			softConstraints = append(softConstraints, constraint)
		}
	}

	sort.Slice(softConstraints, func(i, j int) bool {
		if softConstraints[i].Priority != softConstraints[j].Priority {
			return softConstraints[i].Priority < softConstraints[j].Priority
		}
		return softConstraints[i].CreatedAt.Before(softConstraints[j].CreatedAt)
	})

	// Remove lowest priority soft constraints first
	// Strategy determines how many to remove
	removeFraction := 0.3 // default 30%
	switch c.strategy {
	case StrategyAggressive:
		removeFraction = 0.5
	case StrategyModerate:
		removeFraction = 0.3
	case StrategyConservative:
		removeFraction = 0.1
	}

	removeCount := int(float64(len(softConstraints)) * removeFraction)
	if removeCount > len(softConstraints) {
		removeCount = len(softConstraints)
	}

	// Remove the lowest priority soft constraints
	var newConstraints []*Constraint
	newConstraints = append(newConstraints, hardConstraints...)
	newConstraints = append(newConstraints, softConstraints[removeCount:]...)

	result.ItemsRemoved.Constraints = len(tc.Constraints) - len(newConstraints)
	tc.Constraints = newConstraints
}

// CompressToBudget compresses the context until it fits within the target budget.
// This may require multiple compression passes.
func (c *Compressor) CompressToBudget(tc *TaskContext, targetBudget int) (*CompressionResult, error) {
	if tc == nil {
		return nil, fmt.Errorf("cannot compress nil context")
	}

	if targetBudget <= 0 {
		targetBudget = tc.TokenBudget
	}

	result, err := c.Compress(tc)
	if err != nil {
		return nil, err
	}

	// If still over budget, increase aggression
	iterations := 0
	maxIterations := 5

	for tc.TotalTokens() > targetBudget && iterations < maxIterations {
		// Increase removal fraction
		c2 := NewCompressor(StrategyAggressive)
		c2.preserveRecentMessages = c.preserveRecentMessages / 2
		c2.preserveRecentFiles = c.preserveRecentFiles / 2

		result2, err := c2.Compress(tc)
		if err != nil {
			return nil, err
		}

		// Accumulate stats
		result.TokensRemoved += result2.TokensRemoved
		result.ItemsRemoved.Messages += result2.ItemsRemoved.Messages
		result.ItemsRemoved.FileStates += result2.ItemsRemoved.FileStates
		result.ItemsRemoved.Constraints += result2.ItemsRemoved.Constraints

		iterations++
	}

	// Final recalculation
	tc.RecalculateTokens()
	result.CompressedTokens = tc.TotalTokens()
	result.BudgetAfter = tc.TokenBudget - result.CompressedTokens
	result.BudgetUsed = float64(result.CompressedTokens) / float64(tc.TokenBudget)

	return result, nil
}

// ----------------------------------------------------------------------------
// ContextWindow
// ----------------------------------------------------------------------------

// ContextWindow tracks token usage and provides warnings when approaching budget limits.
type ContextWindow struct {
	// budget is the maximum token budget.
	budget int
	// warningThreshold is the percentage at which to start warning (0.0-1.0).
	warningThreshold float64
	// criticalThreshold is the percentage at which compression is recommended.
	criticalThreshold float64
}

// NewContextWindow creates a new ContextWindow with the given budget.
// Default thresholds: warning at 80%, critical at 95%.
func NewContextWindow(budget int) *ContextWindow {
	return &ContextWindow{
		budget:           budget,
		warningThreshold: 0.80,
		criticalThreshold: 0.95,
	}
}

// SetWarningThreshold sets the threshold for warning messages.
func (cw *ContextWindow) SetWarningThreshold(threshold float64) *ContextWindow {
	cw.warningThreshold = threshold
	return cw
}

// SetCriticalThreshold sets the threshold for critical compression recommendations.
func (cw *ContextWindow) SetCriticalThreshold(threshold float64) *ContextWindow {
	cw.criticalThreshold = threshold
	return cw
}

// Budget returns the total token budget.
func (cw *ContextWindow) Budget() int {
	return cw.budget
}

// UsageLevel returns the current usage level as a fraction (0.0-1.0+).
func (cw *ContextWindow) UsageLevel(tokens int) float64 {
	if cw.budget == 0 {
		return 1.0
	}
	return float64(tokens) / float64(cw.budget)
}

// Status returns the current window status based on token usage.
func (cw *ContextWindow) Status(tokens int) WindowStatus {
	level := cw.UsageLevel(tokens)

	switch {
	case level >= cw.criticalThreshold:
		return WindowStatusCritical
	case level >= cw.warningThreshold:
		return WindowStatusWarning
	default:
		return WindowStatusOK
	}
}

// WindowStatus represents the current state of the context window.
type WindowStatus string

const (
	WindowStatusOK        WindowStatus = "ok"
	WindowStatusWarning   WindowStatus = "warning"
	WindowStatusCritical  WindowStatus = "critical"
)

// String returns the string representation of WindowStatus.
func (s WindowStatus) String() string { return string(s) }

// ShouldCompress reports whether compression is recommended at the given token count.
func (cw *ContextWindow) ShouldCompress(tokens int) bool {
	return cw.UsageLevel(tokens) >= cw.criticalThreshold
}

// Remaining returns the remaining budget at the given token count.
func (cw *ContextWindow) Remaining(tokens int) int {
	remaining := cw.budget - tokens
	if remaining < 0 {
		return 0
	}
	return remaining
}

// EstimateForAdd estimates whether adding tokens would trigger warning/critical status.
func (cw *ContextWindow) EstimateForAdd(currentTokens, additionalTokens int) WindowStatus {
	return cw.Status(currentTokens + additionalTokens)
}

// ----------------------------------------------------------------------------
// ContextBuilder
// ----------------------------------------------------------------------------

// ContextBuilder provides a fluent API for building TaskContext.
type ContextBuilder struct {
	tc *TaskContext
}

// NewContextBuilder creates a new ContextBuilder for the given task.
func NewContextBuilder(taskID, goal string) *ContextBuilder {
	return &ContextBuilder{
		tc: NewTaskContext(taskID, goal),
	}
}

// WithBudget sets the token budget.
func (b *ContextBuilder) WithBudget(budget int) *ContextBuilder {
	b.tc.TokenBudget = budget
	return b
}

// WithConstraint adds a constraint.
func (b *ContextBuilder) WithConstraint(c *Constraint) *ContextBuilder {
	b.tc.AddConstraint(c)
	return b
}

// WithConstraints adds multiple constraints.
func (b *ContextBuilder) WithConstraints(constraints []*Constraint) *ContextBuilder {
	for _, c := range constraints {
		b.tc.AddConstraint(c)
	}
	return b
}

// WithMessage adds a conversation message.
func (b *ContextBuilder) WithMessage(m *ConversationMessage) *ContextBuilder {
	b.tc.AddMessage(m)
	return b
}

// WithFileState adds a file state.
func (b *ContextBuilder) WithFileState(fs *FileState) *ContextBuilder {
	b.tc.AddFileState(fs)
	return b
}

// Build returns the constructed TaskContext.
func (b *ContextBuilder) Build() *TaskContext {
	b.tc.RecalculateTokens()
	return b.tc
}
