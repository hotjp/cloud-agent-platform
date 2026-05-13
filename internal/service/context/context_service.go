// Package context implements L4-Service context propagation service.
// Provides full/summary/delta transmission modes for TaskContext.
package context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	domaincontext "github.com/cloud-agent-platform/cap/internal/domain/context"
	"github.com/cloud-agent-platform/cap/internal/infra/cache"

	"go.uber.org/zap"
)

// PropagationMode defines how context is transmitted to agents.
type PropagationMode string

const (
	// ModeFull transmits complete context (for short tasks).
	ModeFull PropagationMode = "full"
	// ModeSummary transmits compressed summary (for medium tasks).
	ModeSummary PropagationMode = "summary"
	// ModeDelta transmits incremental changes (for long tasks).
	ModeDelta PropagationMode = "delta"
	// ModeAuto automatically selects mode based on context size and task complexity.
	ModeAuto PropagationMode = "auto"
)

// String returns the string representation of PropagationMode.
func (m PropagationMode) String() string { return string(m) }

// IsValid reports whether m is a known PropagationMode.
func (m PropagationMode) IsValid() bool {
	switch m {
	case ModeFull, ModeSummary, ModeDelta, ModeAuto:
		return true
	}
	return false
}

// ModeSelectionThresholds defines thresholds for automatic mode selection.
type ModeSelectionThresholds struct {
	// FullToSummaryTokens is the token count at which to switch from full to summary.
	FullToSummaryTokens int
	// SummaryToDeltaTokens is the token count at which to switch from summary to delta.
	SummaryToDeltaTokens int
	// ShortTaskMessages is the message count below which a task is considered short.
	ShortTaskMessages int
	// LongTaskMessages is the message count above which a task is considered long.
	LongTaskMessages int
}

// DefaultThresholds returns the default mode selection thresholds.
func DefaultThresholds() ModeSelectionThresholds {
	return ModeSelectionThresholds{
		FullToSummaryTokens: 32000,  // 32k tokens (~25% of 128k budget)
		SummaryToDeltaTokens: 96000, // 96k tokens (~75% of 128k budget)
		ShortTaskMessages:    20,
		LongTaskMessages:     200,
	}
}

// ContextProvider is the interface for retrieving TaskContext.
type ContextProvider interface {
	// GetTaskContext retrieves the current TaskContext for a task.
	GetTaskContext(ctx context.Context, taskID string) (*domaincontext.TaskContext, error)
}

// ContextCache provides hot-layer caching for TaskContext.
type ContextCache interface {
	// Get retrieves a TaskContext from cache by taskID.
	Get(ctx context.Context, taskID string) (*domaincontext.TaskContext, error)
	// Set stores a TaskContext in cache with TTL.
	Set(ctx context.Context, taskCtx *domaincontext.TaskContext, ttl time.Duration) error
	// Delete removes a TaskContext from cache.
	Delete(ctx context.Context, taskID string) error
}

// PropagationInput holds the input for context propagation.
type PropagationInput struct {
	TaskID        string
	SubtaskID     string
	Mode          PropagationMode
	Thresholds    ModeSelectionThresholds
	Compressor    *domaincontext.Compressor
}

// PropagationOutput holds the result of context propagation.
type PropagationOutput struct {
	TaskContext   *domaincontext.TaskContext
	Mode          PropagationMode
	Tokens        int
	BudgetUsed    float64
	CompressionRatio float64
	IsCached      bool
	Summary       string
}

// ContextService handles context propagation with full/summary/delta modes.
type ContextService struct {
	cache        ContextCache
	logger       *zap.Logger
	thresholds   ModeSelectionThresholds
	defaultCompressor *domaincontext.Compressor
}

// ContextServiceInput holds dependencies for ContextService.
type ContextServiceInput struct {
	Cache     ContextCache
	Logger    *zap.Logger
	Thresholds *ModeSelectionThresholds
}

// NewContextService creates a new ContextService.
func NewContextService(in ContextServiceInput) *ContextService {
	thresholds := DefaultThresholds()
	if in.Thresholds != nil {
		thresholds = *in.Thresholds
	}

	compressor := domaincontext.DefaultCompressor()

	return &ContextService{
		cache:              in.Cache,
		logger:             in.Logger,
		thresholds:         thresholds,
		defaultCompressor: compressor,
	}
}

// Propagate propagates context using the specified or auto-selected mode.
func (s *ContextService) Propagate(ctx context.Context, input PropagationInput) (*PropagationOutput, error) {
	if input.TaskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}

	// Get the source context
	srcCtx, isCached, err := s.getSourceContext(ctx, input.TaskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get source context: %w", err)
	}

	// Auto-select mode if needed
	mode := input.Mode
	if mode == ModeAuto {
		mode = s.selectMode(srcCtx, input)
	}

	// Build result
	output := &PropagationOutput{
		Mode:     mode,
		IsCached: isCached,
	}

	// Select compression strategy based on mode
	switch mode {
	case ModeFull:
		output.TaskContext = s.propagateFull(srcCtx)
	case ModeSummary:
		output.TaskContext, err = s.propagateSummary(srcCtx, input)
		if err != nil {
			return nil, fmt.Errorf("summary propagation failed: %w", err)
		}
	case ModeDelta:
		output.TaskContext, err = s.propagateDelta(ctx, srcCtx, input.TaskID)
		if err != nil {
			return nil, fmt.Errorf("delta propagation failed: %w", err)
		}
	default:
		return nil, fmt.Errorf("unknown propagation mode: %s", mode)
	}

	// Calculate metrics
	output.Tokens = output.TaskContext.TotalTokens()
	output.BudgetUsed = float64(output.Tokens) / float64(output.TaskContext.TokenBudget)
	if srcCtx.TotalTokens() > 0 {
		output.CompressionRatio = float64(output.Tokens) / float64(srcCtx.TotalTokens())
	}
	output.Summary = s.buildSummary(mode, output)

	s.logger.Info("context propagated",
		zap.String("layer", "L4"),
		zap.String("task_id", input.TaskID),
		zap.String("mode", string(mode)),
		zap.Int("tokens", output.Tokens),
		zap.Float64("budget_used", output.BudgetUsed),
		zap.Bool("is_cached", output.IsCached),
	)

	return output, nil
}

// propagateFull returns a complete copy of the context.
func (s *ContextService) propagateFull(src *domaincontext.TaskContext) *domaincontext.TaskContext {
	return src.Clone()
}

// propagateSummary compresses the context using the compressor.
func (s *ContextService) propagateSummary(src *domaincontext.TaskContext, input PropagationInput) (*domaincontext.TaskContext, error) {
	// Clone to avoid modifying the source
	clone := src.Clone()

	// Use provided compressor or default
	compressor := input.Compressor
	if compressor == nil {
		compressor = s.defaultCompressor
	}

	// Compress to fit within 75% of budget
	targetBudget := int(float64(src.TokenBudget) * 0.75)
	_, err := compressor.CompressToBudget(clone, targetBudget)
	if err != nil {
		return nil, fmt.Errorf("compression failed: %w", err)
	}

	return clone, nil
}

// propagateDelta extracts only the changes since last propagation.
func (s *ContextService) propagateDelta(ctx context.Context, src *domaincontext.TaskContext, taskID string) (*domaincontext.TaskContext, error) {
	// Try to get cached version
	cached, err := s.cache.Get(ctx, s.deltaCacheKey(taskID))
	if err != nil && !errors.Is(err, cache.ErrCacheMiss) {
		s.logger.Warn("failed to get cached delta context",
			zap.String("task_id", taskID),
			zap.Error(err),
		)
	}

	// Build delta context
	delta := s.buildDeltaContext(src, cached)

	// Update cache with new baseline
	if s.cache != nil {
		if cacheErr := s.cache.Set(ctx, delta, 24*time.Hour); cacheErr != nil {
			s.logger.Warn("failed to cache delta context",
				zap.String("task_id", taskID),
				zap.Error(cacheErr),
			)
		}
	}

	return delta, nil
}

// buildDeltaContext builds a new context containing only the changes.
func (s *ContextService) buildDeltaContext(src, prev *domaincontext.TaskContext) *domaincontext.TaskContext {
	delta := domaincontext.NewTaskContext(src.TaskID, src.Goal)
	delta.TokenBudget = src.TokenBudget

	if prev == nil {
		// No previous context - return full context as "delta"
		return src.Clone()
	}

	// Build sets of previous message IDs for efficient lookup
	prevMessageIDs := make(map[string]bool)
	for _, msg := range prev.Messages {
		prevMessageIDs[msg.ID] = true
	}

	// Only include messages that are new (not in previous)
	for _, msg := range src.Messages {
		if !prevMessageIDs[msg.ID] {
			delta.AddMessage(msg)
		}
	}

	// Only include constraints that are new (not in previous)
	prevConstraintIDs := make(map[string]bool)
	for _, c := range prev.Constraints {
		prevConstraintIDs[c.ID] = true
	}
	for _, c := range src.Constraints {
		if !prevConstraintIDs[c.ID] {
			delta.AddConstraint(c)
		}
	}

	// Only include file states that are new or changed since base version
	prevFileMap := make(map[string]*domaincontext.FileState)
	for _, f := range prev.FileStates {
		prevFileMap[f.Path] = f
	}
	for _, f := range src.FileStates {
		prev, exists := prevFileMap[f.Path]
		if !exists || prev.ContentHash != f.ContentHash {
			delta.AddFileState(f)
		}
	}

	// Preserve hard constraints from previous context
	for _, c := range prev.GetHardConstraints() {
		delta.AddConstraint(c)
	}

	delta.Version = src.Version

	return delta
}

// deltaCacheKey returns the cache key for delta context.
func (s *ContextService) deltaCacheKey(taskID string) string {
	return fmt.Sprintf("context:delta:%s", taskID)
}

// selectMode automatically selects the appropriate propagation mode.
func (s *ContextService) selectMode(ctx *domaincontext.TaskContext, input PropagationInput) PropagationMode {
	tokens := ctx.TotalTokens()
	messageCount := len(ctx.Messages)

	// Check token-based thresholds
	if tokens >= s.thresholds.SummaryToDeltaTokens || messageCount >= s.thresholds.LongTaskMessages {
		return ModeDelta
	}
	if tokens >= s.thresholds.FullToSummaryTokens || messageCount >= s.thresholds.ShortTaskMessages {
		return ModeSummary
	}

	return ModeFull
}

// buildSummary returns a human-readable summary of the propagation.
func (s *ContextService) buildSummary(mode PropagationMode, output *PropagationOutput) string {
	return fmt.Sprintf(
		"mode=%s tokens=%d budget_used=%.1f%% compression=%.2fx",
		mode, output.Tokens, output.BudgetUsed*100, output.CompressionRatio,
	)
}

// getSourceContext retrieves the source context for propagation.
// Uses cache if available, otherwise returns an empty context.
// Returns (context, isCached, error).
func (s *ContextService) getSourceContext(ctx context.Context, taskID string) (*domaincontext.TaskContext, bool, error) {
	// Try cache first
	if s.cache != nil {
		cached, err := s.cache.Get(ctx, taskID)
		if err == nil && cached != nil {
			return cached, true, nil
		}
	}

	// Return a new empty context - actual data should come from the caller
	return domaincontext.NewTaskContext(taskID, ""), false, nil
}

// CacheContext stores the context in cache.
func (s *ContextService) CacheContext(ctx context.Context, taskCtx *domaincontext.TaskContext, ttl time.Duration) error {
	if s.cache == nil {
		return nil
	}
	return s.cache.Set(ctx, taskCtx, ttl)
}

// InvalidateCache removes the cached context for a task.
func (s *ContextService) InvalidateCache(ctx context.Context, taskID string) error {
	if s.cache == nil {
		return nil
	}
	return s.cache.Delete(ctx, taskID)
}

// ContextSnapshot represents a point-in-time snapshot of context for delta tracking.
type ContextSnapshot struct {
	TaskID       string
	Version      int
	Tokens       int
	MessagesHash string
	FilesHash    string
	SnapshotAt   time.Time
}

// NewContextSnapshot creates a new snapshot from a TaskContext.
func NewContextSnapshot(tc *domaincontext.TaskContext) *ContextSnapshot {
	return &ContextSnapshot{
		TaskID:       tc.TaskID,
		Version:      tc.Version,
		Tokens:       tc.TotalTokens(),
		MessagesHash: hashMessages(tc.Messages),
		FilesHash:    hashFiles(tc.FileStates),
		SnapshotAt:   time.Now().UTC(),
	}
}

// hashMessages computes a hash of message IDs for change detection.
func hashMessages(messages []*domaincontext.ConversationMessage) string {
	if len(messages) == 0 {
		return ""
	}
	// Simple hash based on last message ID count
	return fmt.Sprintf("%d-%s", len(messages), messages[len(messages)-1].ID)
}

// hashFiles computes a hash of file states for change detection.
func hashFiles(files []*domaincontext.FileState) string {
	if len(files) == 0 {
		return ""
	}
	// Simple hash based on last file path
	return fmt.Sprintf("%d-%s", len(files), files[len(files)-1].Path)
}

// PropagationStats holds statistics about context propagation.
type PropagationStats struct {
	Mode          PropagationMode
	SourceTokens  int
	OutputTokens  int
	Cached        bool
	CompressionRatio float64
	SelectionReason string
}

// SelectModeForTask analyzes a task and returns the recommended propagation mode.
func (s *ContextService) SelectModeForTask(ctx context.Context, taskID string, complexity int) (PropagationMode, string, error) {
	tc, _, err := s.getSourceContext(ctx, taskID)
	if err != nil {
		return ModeFull, "", err
	}

	tokens := tc.TotalTokens()
	messages := len(tc.Messages)

	// Use complexity hint if provided
	if complexity > 0 {
		if complexity >= 8 {
			return ModeDelta, "high complexity task", nil
		}
		if complexity >= 5 {
			return ModeSummary, "medium complexity task", nil
		}
	}

	// Token-based selection
	if tokens >= s.thresholds.SummaryToDeltaTokens {
		return ModeDelta, fmt.Sprintf("token count %d exceeds threshold %d", tokens, s.thresholds.SummaryToDeltaTokens), nil
	}
	if tokens >= s.thresholds.FullToSummaryTokens {
		return ModeSummary, fmt.Sprintf("token count %d exceeds threshold %d", tokens, s.thresholds.FullToSummaryTokens), nil
	}

	// Message count-based selection
	if messages >= s.thresholds.LongTaskMessages {
		return ModeDelta, fmt.Sprintf("message count %d exceeds threshold %d", messages, s.thresholds.LongTaskMessages), nil
	}
	if messages >= s.thresholds.ShortTaskMessages {
		return ModeSummary, fmt.Sprintf("message count %d exceeds threshold %d", messages, s.thresholds.ShortTaskMessages), nil
	}

	return ModeFull, "short task, full context appropriate", nil
}

// MarshalJSON implements json.Marshaler for PropagationOutput.
func (p *PropagationOutput) MarshalJSON() ([]byte, error) {
	type Alias PropagationOutput
	return json.Marshal(&struct {
		*Alias
		Mode string `json:"mode"`
	}{
		Alias: (*Alias)(p),
		Mode:  p.Mode.String(),
	})
}