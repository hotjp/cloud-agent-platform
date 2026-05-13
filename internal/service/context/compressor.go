// Package context implements L4-Service context compression with L1 (rule-based) and L3 (LLM intelligent) modes.
// L1 compression: fast, no LLM calls, suitable for high-frequency scenarios.
// L3 compression: higher quality summaries via LLM, has cost and latency.
package context

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	domaincontext "github.com/cloud-agent-platform/cap/internal/domain/context"
	"github.com/cloud-agent-platform/cap/plugins/llmrouter"

	"go.uber.org/zap"
)

// CompressionLevel defines the compression level to use.
type CompressionLevel string

const (
	// CompressionLevelL1 uses rule-based compression (fast, no LLM).
	CompressionLevelL1 CompressionLevel = "l1"
	// CompressionLevelL3 uses LLM intelligent compression (high quality, has cost).
	CompressionLevelL3 CompressionLevel = "l3"
	// CompressionLevelAuto automatically selects based on context size and budget.
	CompressionLevelAuto CompressionLevel = "auto"
)

// String returns the string representation of CompressionLevel.
func (c CompressionLevel) String() string { return string(c) }

// IsValid reports whether c is a known CompressionLevel.
func (c CompressionLevel) IsValid() bool {
	switch c {
	case CompressionLevelL1, CompressionLevelL3, CompressionLevelAuto:
		return true
	}
	return false
}

// LLMCompressorConfig holds configuration for L3 LLM compression.
type LLMCompressorConfig struct {
	// Model is the LLM model to use for summarization.
	Model llmrouter.ModelName
	// MaxTokensPerSummary is the maximum tokens for each message summary.
	MaxTokensPerSummary int
	// Temperature controls randomness in LLM responses.
	Temperature float64
	// PreserveRecentMessages is the number of recent messages to preserve verbatim.
	PreserveRecentMessages int
	// EnableParallelSummarization enables parallel LLM calls for multiple message batches.
	EnableParallelSummarization bool
	// BatchSize is the number of messages to summarize in parallel.
	BatchSize int
	// Timeout is the timeout for LLM calls.
	Timeout time.Duration
}

// DefaultLLMCompressorConfig returns the default LLM compression configuration.
func DefaultLLMCompressorConfig() LLMCompressorConfig {
	return LLMCompressorConfig{
		Model:                    llmrouter.ModelClaudeHaiku,
		MaxTokensPerSummary:       100,
		Temperature:              0.3,
		PreserveRecentMessages:    5,
		EnableParallelSummarization: true,
		BatchSize:                10,
		Timeout:                  30 * time.Second,
	}
}

// LLMCompressor is the interface for LLM-based compression.
type LLMCompressor interface {
	// SummarizeMessages uses LLM to summarize conversation messages.
	SummarizeMessages(ctx context.Context, messages []*domaincontext.ConversationMessage) ([]*domaincontext.ConversationMessage, error)
}

// llmCompressor implements LLMCompressor using the llmrouter.
type llmCompressor struct {
	router *llmrouter.Router
	config LLMCompressorConfig
	logger *zap.Logger
}

// NewLLMCompressor creates a new LLM compressor.
func NewLLMCompressor(router *llmrouter.Router, logger *zap.Logger, config LLMCompressorConfig) LLMCompressor {
	if router == nil {
		return &noopLLMCompressor{}
	}
	if config.MaxTokensPerSummary == 0 {
		config = DefaultLLMCompressorConfig()
	}
	return &llmCompressor{
		router: router,
		config: config,
		logger: logger,
	}
}

// SummarizeMessages uses LLM to summarize conversation messages.
func (c *llmCompressor) SummarizeMessages(ctx context.Context, messages []*domaincontext.ConversationMessage) ([]*domaincontext.ConversationMessage, error) {
	if len(messages) == 0 {
		return messages, nil
	}

	// Always preserve recent messages verbatim
	if len(messages) <= c.config.PreserveRecentMessages {
		return messages, nil
	}

	// Separate recent messages (preserve) from older messages (potentially summarize)
	recentCount := c.config.PreserveRecentMessages
	olderMessages := messages[:len(messages)-recentCount]
	recentMessages := messages[len(messages)-recentCount:]

	// Build summarization prompt
	prompt := c.buildSummarizationPrompt(olderMessages)

	// Call LLM for summarization
	req := &llmrouter.LLMRequest{
		TaskType:    llmrouter.TaskTypeSummarize,
		Model:       c.config.Model,
		Prompt:      prompt,
		System:      "You are a context compression assistant. Summarize the conversation concisely while preserving key information, decisions, and constraints.",
		MaxTokens:   c.config.MaxTokensPerSummary * len(olderMessages) / c.config.BatchSize,
		Temperature: c.config.Temperature,
	}

	// Apply timeout
	if c.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.config.Timeout)
		defer cancel()
	}

	resp, err := c.router.Complete(ctx, req)
	if err != nil {
		c.logger.Warn("LLM summarization failed, falling back to rule-based compression",
			zap.Error(err),
			zap.Int("message_count", len(olderMessages)))
		return c.fallbackRuleBased(olderMessages), nil
	}

	// Create summary message
	summaryMsg := domaincontext.NewAgentMessage(resp.Content)
	summaryMsg.Priority = -1 // High priority to preserve

	// Combine: summarized older + preserved recent
	result := []*domaincontext.ConversationMessage{summaryMsg}
	result = append(result, recentMessages...)

	c.logger.Info("LLM summarization completed",
		zap.Int("original_messages", len(olderMessages)),
		zap.Int("summary_messages", 1),
		zap.String("model", string(resp.Model)),
		zap.Int("tokens_used", resp.TokensUsed))

	return result, nil
}

// buildSummarizationPrompt creates a prompt for summarizing messages.
func (c *llmCompressor) buildSummarizationPrompt(messages []*domaincontext.ConversationMessage) string {
	var sb strings.Builder
	sb.WriteString("Summarize the following conversation concisely. Preserve key information:\n\n")

	for i, msg := range messages {
		role := strings.ToUpper(string(msg.Role))
		sb.WriteString(fmt.Sprintf("[%d] %s: %s\n", i+1, role, msg.Content))
	}

	sb.WriteString("\nProvide a concise summary that captures:")
	sb.WriteString("\n- Main topics discussed")
	sb.WriteString("\n- Key decisions made")
	sb.WriteString("\n- Open questions or pending items")
	sb.WriteString("\n- Any constraints or requirements mentioned")

	return sb.String()
}

// fallbackRuleBased provides rule-based summarization when LLM fails.
func (c *llmCompressor) fallbackRuleBased(messages []*domaincontext.ConversationMessage) []*domaincontext.ConversationMessage {
	if len(messages) == 0 {
		return messages
	}

	// Keep only the most important messages (highest priority or system messages)
	var kept []*domaincontext.ConversationMessage
	var summarized []string

	for _, msg := range messages {
		if msg.Role == domaincontext.MessageRoleSystem || msg.Priority < 5 {
			kept = append(kept, msg)
		} else {
			summarized = append(summarized, fmt.Sprintf("[%s] %s", msg.Role, truncate(msg.Content, 50)))
		}
	}

	// Add a summary message if we filtered some
	if len(summarized) > 0 {
		summary := domaincontext.NewAgentMessage(
			fmt.Sprintf("Previous context: %d messages summarized. Topics: %s",
				len(summarized), strings.Join(summarized, "; ")))
		summary.Priority = -1
		kept = append([]*domaincontext.ConversationMessage{summary}, kept...)
	}

	return kept
}

// truncate shortens a string to maxLen characters.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// noopLLMCompressor is a no-op implementation when no LLM router is available.
type noopLLMCompressor struct{}

func (c *noopLLMCompressor) SummarizeMessages(ctx context.Context, messages []*domaincontext.ConversationMessage) ([]*domaincontext.ConversationMessage, error) {
	return messages, nil
}

// ---------------------------------------------------------------------------
// IntelligentCompressor - L4-Service layer compression engine
// ---------------------------------------------------------------------------

// IntelligentCompressorConfig holds configuration for the intelligent compressor.
type IntelligentCompressorConfig struct {
	// L1Strategy is the rule-based compression strategy.
	L1Strategy domaincontext.CompressionStrategy
	// LLMConfig is the LLM compression configuration.
	LLMConfig LLMCompressorConfig
	// AutoSelectThreshold is the token count at which to auto-select compression level.
	// Below this: use L1 (fast). Above this: consider L3 (quality).
	AutoSelectThreshold int
	// LLMBudgetThreshold is the remaining budget percentage below which to use L3.
	// E.g., 0.2 means use L3 when remaining budget is less than 20%.
	LLMBudgetThreshold float64
	// PreferQualityWhenBudgetSufficient prefer L3 when budget allows.
	PreferQualityWhenBudgetSufficient bool
}

// DefaultIntelligentCompressorConfig returns the default configuration.
func DefaultIntelligentCompressorConfig() IntelligentCompressorConfig {
	return IntelligentCompressorConfig{
		L1Strategy:             domaincontext.StrategyModerate,
		LLMConfig:              DefaultLLMCompressorConfig(),
		AutoSelectThreshold:    5000,  // Use L3 when context > 5k tokens
		LLMBudgetThreshold:     0.2,  // Use L3 when < 20% budget remains
		PreferQualityWhenBudgetSufficient: true,
	}
}

// IntelligentCompressor provides automatic compression with L1/L3 strategy selection.
type IntelligentCompressor struct {
	l1Compressor *domaincontext.Compressor
	llmCompressor LLMCompressor
	config       IntelligentCompressorConfig
	logger       *zap.Logger
}

// NewIntelligentCompressor creates a new intelligent compressor.
func NewIntelligentCompressor(
	l1Compressor *domaincontext.Compressor,
	llmCompressor LLMCompressor,
	logger *zap.Logger,
	config IntelligentCompressorConfig,
) *IntelligentCompressor {
	if l1Compressor == nil {
		l1Compressor = domaincontext.DefaultCompressor()
	}
	return &IntelligentCompressor{
		l1Compressor:  l1Compressor,
		llmCompressor: llmCompressor,
		config:        config,
		logger:        logger,
	}
}

// CompressResult holds the result of intelligent compression.
type CompressResult struct {
	// Level used for compression.
	Level CompressionLevel
	// OriginalTokens before compression.
	OriginalTokens int
	// CompressedTokens after compression.
	CompressedTokens int
	// TokensRemoved number of tokens removed.
	TokensRemoved int
	// ItemsRemoved statistics.
	ItemsRemoved domaincontext.CompressionStats
	// BudgetAfter remaining budget.
	BudgetAfter int
	// BudgetUsed percentage of budget used.
	BudgetUsed float64
	// LLMUsed indicates if LLM was called.
	LLMUsed bool
	// Summary human-readable summary.
	Summary string
}

// Compress compresses the context using the appropriate level.
func (c *IntelligentCompressor) Compress(ctx context.Context, tc *domaincontext.TaskContext) (*CompressResult, error) {
	if tc == nil {
		return nil, fmt.Errorf("cannot compress nil context")
	}

	result := &CompressResult{
		OriginalTokens: tc.TotalTokens(),
		BudgetAfter:    tc.RemainingBudget(),
	}

	// If not over budget, no compression needed
	if !tc.IsBudgetExceeded() {
		result.CompressedTokens = result.OriginalTokens
		result.BudgetUsed = float64(result.OriginalTokens) / float64(tc.TokenBudget)
		result.Level = CompressionLevelL1
		result.Summary = "no compression needed"
		return result, nil
	}

	// Select compression level
	level := c.selectLevel(tc)
	result.Level = level

	c.logger.Info("compressing context",
		zap.String("layer", "L4"),
		zap.String("task_id", tc.TaskID),
		zap.String("level", string(level)),
		zap.Int("original_tokens", result.OriginalTokens))

	switch level {
	case CompressionLevelL1:
		return c.compressL1(ctx, tc, result)
	case CompressionLevelL3:
		return c.compressL3(ctx, tc, result)
	default:
		// Fallback to L1
		return c.compressL1(ctx, tc, result)
	}
}

// selectLevel automatically selects the compression level.
func (c *IntelligentCompressor) selectLevel(tc *domaincontext.TaskContext) CompressionLevel {
	tokens := tc.TotalTokens()
	budgetUsed := float64(tokens) / float64(tc.TokenBudget)
	remainingBudget := 1.0 - budgetUsed

	// L3 is more expensive but produces better results
	// Use L3 when:
	// 1. Context is large enough that L1 might lose important info
	// 2. Budget is tight and we need quality over speed
	// 3. User prefers quality and budget allows

	// If context is very large (> threshold) and budget is tight, use L3
	if tokens > c.config.AutoSelectThreshold && remainingBudget < c.config.LLMBudgetThreshold {
		return CompressionLevelL3
	}

	// If budget is very tight and we want quality, use L3
	if remainingBudget < c.config.LLMBudgetThreshold/2 && c.config.PreferQualityWhenBudgetSufficient {
		return CompressionLevelL3
	}

	// Default to L1 (fast, rule-based)
	return CompressionLevelL1
}

// compressL1 performs rule-based compression (fast, no LLM).
func (c *IntelligentCompressor) compressL1(ctx context.Context, tc *domaincontext.TaskContext, result *CompressResult) (*CompressResult, error) {
	// L1 compression: preserve goals and constraints, compress messages and file states
	domainResult, err := c.l1Compressor.Compress(tc)
	if err != nil {
		return nil, fmt.Errorf("L1 compression failed: %w", err)
	}

	result.CompressedTokens = domainResult.CompressedTokens
	result.TokensRemoved = domainResult.TokensRemoved
	result.ItemsRemoved = domainResult.ItemsRemoved
	result.BudgetAfter = domainResult.BudgetAfter
	result.BudgetUsed = domainResult.BudgetUsed
	result.LLMUsed = false
	result.Summary = fmt.Sprintf("L1 rule-based: removed %d tokens (%d msgs, %d files, %d soft constraints)",
		result.TokensRemoved,
		result.ItemsRemoved.Messages,
		result.ItemsRemoved.FileStates,
		result.ItemsRemoved.Constraints)

	c.logger.Info("L1 compression completed",
		zap.Int("tokens_removed", result.TokensRemoved),
		zap.Float64("budget_used", result.BudgetUsed))

	return result, nil
}

// compressL3 performs LLM intelligent compression (high quality, has cost).
func (c *IntelligentCompressor) compressL3(ctx context.Context, tc *domaincontext.TaskContext, result *CompressResult) (*CompressResult, error) {
	// L3 compression strategy:
	// 1. Preserve goals (NEVER compress)
	// 2. Preserve hard constraints (NEVER compress)
	// 3. Summarize old messages using LLM
	// 4. Compress file states using rules
	// 5. Soft constraints: keep high-priority, summarize/remove low-priority

	originalTokens := tc.TotalTokens()
	originalMsgCount := len(tc.Messages)
	originalFileCount := len(tc.FileStates)
	originalSoftConstraints := len(tc.Constraints) - tc.HardConstraintsCount()

	// Step 1: Summarize messages using LLM (preserving recent)
	var err error
	if c.llmCompressor != nil && len(tc.Messages) > c.config.LLMConfig.PreserveRecentMessages {
		tc.Messages, err = c.llmCompressor.SummarizeMessages(ctx, tc.Messages)
		if err != nil {
			c.logger.Warn("LLM message summarization failed, using L1 fallback",
				zap.Error(err))
			// Fallback to L1 for messages
			c.l1Compressor.Compress(tc)
		}
	}

	// Step 2: Compress file states using L1 rules (preserve recent and important)
	c.compressFileStatesL1(tc)

	// Step 3: Compress soft constraints using rules (preserve hard)
	c.compressConstraintsL1(tc)

	// Recalculate tokens
	tc.RecalculateTokens()

	result.CompressedTokens = tc.TotalTokens()
	result.TokensRemoved = originalTokens - result.CompressedTokens
	result.ItemsRemoved.Messages = originalMsgCount - len(tc.Messages)
	result.ItemsRemoved.FileStates = originalFileCount - len(tc.FileStates)
	result.ItemsRemoved.Constraints = originalSoftConstraints - c.countSoftConstraints(tc)
	result.BudgetAfter = tc.TokenBudget - result.CompressedTokens
	result.BudgetUsed = float64(result.CompressedTokens) / float64(tc.TokenBudget)
	result.LLMUsed = true
	result.Summary = fmt.Sprintf("L3 LLM-powered: removed %d tokens, preserved goals and hard constraints",
		result.TokensRemoved)

	c.logger.Info("L3 compression completed",
		zap.Int("tokens_removed", result.TokensRemoved),
		zap.Float64("budget_used", result.BudgetUsed),
		zap.Bool("llm_used", result.LLMUsed))

	return result, nil
}

// compressFileStatesL1 compresses file states using rules (preserve recent/important).
func (c *IntelligentCompressor) compressFileStatesL1(tc *domaincontext.TaskContext) {
	if len(tc.FileStates) <= 5 {
		return
	}

	// Sort by priority and recency
	sorted := make([]*domaincontext.FileState, len(tc.FileStates))
	copy(sorted, tc.FileStates)

	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Priority != sorted[j].Priority {
			return sorted[i].Priority < sorted[j].Priority
		}
		return sorted[i].ModifiedAt.After(sorted[j].ModifiedAt)
	})

	// Keep: recent, created, deleted, modified
	// Remove: old unmodified files with low priority
	var kept []*domaincontext.FileState
	removeCount := 0

	for _, f := range sorted {
		keep := true
		// Never remove created, deleted, or recently modified
		if f.ChangeType == domaincontext.FileChangeDeleted ||
			f.ChangeType == domaincontext.FileChangeCreated ||
			f.Priority < 5 {
			keep = true
		} else if len(kept) > 5 && f.Priority >= 10 {
			// Remove low-priority old files beyond the first 5
			keep = false
			removeCount++
		}
		if keep {
			kept = append(kept, f)
		}
	}

	// Preserve order by path (most recent for each path)
	byPath := make(map[string]*domaincontext.FileState)
	for _, f := range kept {
		byPath[f.Path] = f
	}

	var final []*domaincontext.FileState
	seen := make(map[string]bool)
	// Go through original order, keeping most recent of each path
	for _, f := range tc.FileStates {
		if rf, ok := byPath[f.Path]; ok && !seen[f.Path] {
			final = append(final, rf)
			seen[f.Path] = true
		}
	}

	tc.FileStates = final
}

// compressConstraintsL1 compresses soft constraints using rules (preserve hard).
func (c *IntelligentCompressor) compressConstraintsL1(tc *domaincontext.TaskContext) {
	var hardConstraints []*domaincontext.Constraint
	var softConstraints []*domaincontext.Constraint

	for _, c := range tc.Constraints {
		if c.IsHard() {
			hardConstraints = append(hardConstraints, c)
		} else {
			softConstraints = append(softConstraints, c)
		}
	}

	if len(softConstraints) == 0 {
		return
	}

	// Sort soft constraints by priority
	sort.Slice(softConstraints, func(i, j int) bool {
		if softConstraints[i].Priority != softConstraints[j].Priority {
			return softConstraints[i].Priority < softConstraints[j].Priority
		}
		return softConstraints[i].CreatedAt.Before(softConstraints[j].CreatedAt)
	})

	// Keep top 50% of soft constraints by priority
	keepCount := len(softConstraints) / 2
	if keepCount < 1 {
		keepCount = 1
	}

	// Rebuild constraint list: all hard + top soft
	tc.Constraints = append(hardConstraints, softConstraints[:keepCount]...)
}

// countSoftConstraints counts soft (non-hard) constraints.
func (c *IntelligentCompressor) countSoftConstraints(tc *domaincontext.TaskContext) int {
	count := 0
	for _, c := range tc.Constraints {
		if !c.IsHard() {
			count++
		}
	}
	return count
}

// Verify interface implementation.
var _ LLMCompressor = (*llmCompressor)(nil)
var _ LLMCompressor = (*noopLLMCompressor)(nil)