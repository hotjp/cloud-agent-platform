// Package context implements L2-Domain layer: TaskContext, FileState, Constraint,
// ContextWindow management, and context compression.
// This package has ZERO external dependencies - pure Go structs + standard library.
package context

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/oklog/ulid/v2"
)

// ----------------------------------------------------------------------------
// ID generation
// ----------------------------------------------------------------------------

// randEntropy wraps crypto/rand.Reader so it satisfies ulid.Entropy.
type randEntropy struct{}

func (randEntropy) Read(p []byte) (n int, err error) {
	return rand.Read(p)
}

// NewULID generates a new ULID string using crypto/rand as the entropy source.
func NewULID() string {
	entropy := ulid.Monotonic(randEntropy{}, 0)
	return ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
}

// ----------------------------------------------------------------------------
// Constraint strength
// ----------------------------------------------------------------------------

// ConstraintStrength represents the enforceability level of a constraint.
type ConstraintStrength string

const (
	// ConstraintNonNegotiable is a hard constraint that MUST be respected.
	// Violating this constraint should fail the task.
	ConstraintNonNegotiable ConstraintStrength = "NON_NEGOTIABLE"
	// ConstraintPreferable is a soft constraint that SHOULD be respected.
	// Violating this is logged but does not fail the task.
	ConstraintPreferable ConstraintStrength = "PREFERABLE"
)

// String returns the string representation of ConstraintStrength.
func (s ConstraintStrength) String() string { return string(s) }

// IsValid reports whether s is a known ConstraintStrength.
func (s ConstraintStrength) IsValid() bool {
	switch s {
	case ConstraintNonNegotiable, ConstraintPreferable:
		return true
	}
	return false
}

// ----------------------------------------------------------------------------
// File change type
// ----------------------------------------------------------------------------

// FileChangeType represents the type of change made to a file.
type FileChangeType string

const (
	FileChangeCreated  FileChangeType = "created"
	FileChangeModified FileChangeType = "modified"
	FileChangeDeleted  FileChangeType = "deleted"
	FileChangeRenamed  FileChangeType = "renamed"
)

// String returns the string representation of FileChangeType.
func (f FileChangeType) String() string { return string(f) }

// IsValid reports whether f is a known FileChangeType.
func (f FileChangeType) IsValid() bool {
	switch f {
	case FileChangeCreated, FileChangeModified, FileChangeDeleted, FileChangeRenamed:
		return true
	}
	return false
}

// ----------------------------------------------------------------------------
// Constraint
// ----------------------------------------------------------------------------

// Constraint represents a task constraint with enforceability level.
// Constraints guide agent behavior and are validated during execution.
type Constraint struct {
	// ID is the unique identifier for this constraint.
	ID string
	// Strength determines whether this is a hard or soft constraint.
	Strength ConstraintStrength
	// Category groups constraints (e.g., "language", "style", "performance", "security").
	Category string
	// Description is the human-readable constraint text.
	Description string
	// CreatedAt is when the constraint was added.
	CreatedAt time.Time
	// Priority is used during context compression (lower = higher priority to keep).
	Priority int
}

// NewConstraint creates a new Constraint with a generated ULID.
func NewConstraint(strength ConstraintStrength, category, description string) *Constraint {
	return &Constraint{
		ID:          NewULID(),
		Strength:    strength,
		Category:    category,
		Description: description,
		CreatedAt:   time.Now().UTC(),
		Priority:    0,
	}
}

// NewNonNegotiable creates a hard (NON_NEGOTIABLE) constraint.
func NewNonNegotiable(category, description string) *Constraint {
	c := NewConstraint(ConstraintNonNegotiable, category, description)
	return c
}

// NewPreferable creates a soft (PREFERABLE) constraint.
func NewPreferable(category, description string) *Constraint {
	c := NewConstraint(ConstraintPreferable, category, description)
	return c
}

// IsHard reports whether this is a hard constraint.
func (c *Constraint) IsHard() bool {
	return c.Strength == ConstraintNonNegotiable
}

// CompressionKey returns a key used for grouping constraints during compression.
func (c *Constraint) CompressionKey() string {
	return fmt.Sprintf("%s:%s", c.Category, c.Strength)
}

// ----------------------------------------------------------------------------
// FileState
// ----------------------------------------------------------------------------

// FileState represents the state of a file at a point in time.
// It is used to track which files are relevant to the task and how they have changed.
type FileState struct {
	// ID is the unique identifier for this file state entry.
	ID string
	// Path is the file path relative to the repository root.
	Path string
	// ContentHash is the SHA-256 hash of the file content.
	ContentHash string
	// ModifiedAt is when the file was last modified.
	ModifiedAt time.Time
	// ChangeType describes how this file was changed in the current session.
	ChangeType FileChangeType
	// Priority is used during context compression (lower = higher priority to keep).
	Priority int
	// SizeBytes is the file size in bytes (0 for deleted files).
	SizeBytes int64
	// DiffSummary is a short summary of the changes (for modified files).
	// This is used instead of full content to save tokens.
	DiffSummary string
}

// NewFileState creates a new FileState with a generated ULID.
func NewFileState(path, contentHash string, changeType FileChangeType, sizeBytes int64) *FileState {
	return &FileState{
		ID:          NewULID(),
		Path:        path,
		ContentHash: contentHash,
		ModifiedAt:  time.Now().UTC(),
		ChangeType:  changeType,
		Priority:    0,
		SizeBytes:   sizeBytes,
		DiffSummary: "",
	}
}

// NewFileStateCreated creates a FileState for a newly created file.
func NewFileStateCreated(path, contentHash string, sizeBytes int64) *FileState {
	return NewFileState(path, contentHash, FileChangeCreated, sizeBytes)
}

// NewFileStateModified creates a FileState for a modified file.
func NewFileStateModified(path, contentHash string, sizeBytes int64, diffSummary string) *FileState {
	fs := NewFileState(path, contentHash, FileChangeModified, sizeBytes)
	fs.DiffSummary = diffSummary
	return fs
}

// NewFileStateDeleted creates a FileState for a deleted file.
func NewFileStateDeleted(path string) *FileState {
	return NewFileState(path, "", FileChangeDeleted, 0)
}

// IsDeleted reports whether this file was deleted.
func (f *FileState) IsDeleted() bool {
	return f.ChangeType == FileChangeDeleted
}

// IsCreated reports whether this file was newly created.
func (f *FileState) IsCreated() bool {
	return f.ChangeType == FileChangeCreated
}

// ----------------------------------------------------------------------------
// Message role
// ----------------------------------------------------------------------------

// MessageRole represents the sender of a message in the conversation history.
type MessageRole string

const (
	MessageRoleUser    MessageRole = "user"
	MessageRoleAgent   MessageRole = "agent"
	MessageRoleSystem  MessageRole = "system"
)

// String returns the string representation of MessageRole.
func (r MessageRole) String() string { return string(r) }

// IsValid reports whether r is a known MessageRole.
func (r MessageRole) IsValid() bool {
	switch r {
	case MessageRoleUser, MessageRoleAgent, MessageRoleSystem:
		return true
	}
	return false
}

// ----------------------------------------------------------------------------
// ConversationMessage
// ----------------------------------------------------------------------------

// ConversationMessage represents a single message in the task conversation history.
type ConversationMessage struct {
	// ID is the unique identifier for this message.
	ID string
	// Role is the sender of the message.
	Role MessageRole
	// Content is the message text.
	Content string
	// Timestamp is when the message was sent.
	Timestamp time.Time
	// TokenCount is the estimated token count for this message.
	// This is set by the ContextWindow when adding the message.
	TokenCount int
	// Priority is used during context compression (lower = higher priority to keep).
	Priority int
	// SubtaskID is the subtask this message belongs to (optional).
	SubtaskID *string
}

// NewConversationMessage creates a new ConversationMessage with a generated ULID.
func NewConversationMessage(role MessageRole, content string) *ConversationMessage {
	return &ConversationMessage{
		ID:          NewULID(),
		Role:        role,
		Content:     content,
		Timestamp:   time.Now().UTC(),
		TokenCount:  estimateTokens(content),
		Priority:    0,
	}
}

// NewUserMessage creates a new user message.
func NewUserMessage(content string) *ConversationMessage {
	return NewConversationMessage(MessageRoleUser, content)
}

// NewAgentMessage creates a new agent message.
func NewAgentMessage(content string) *ConversationMessage {
	return NewConversationMessage(MessageRoleAgent, content)
}

// NewSystemMessage creates a new system message.
func NewSystemMessage(content string) *ConversationMessage {
	return NewConversationMessage(MessageRoleSystem, content)
}

// WithSubtask associates this message with a subtask.
func (m *ConversationMessage) WithSubtask(subtaskID string) *ConversationMessage {
	m.SubtaskID = &subtaskID
	return m
}

// SetPriority sets the compression priority for this message.
func (m *ConversationMessage) SetPriority(priority int) *ConversationMessage {
	m.Priority = priority
	return m
}

// ----------------------------------------------------------------------------
// TaskContext
// ----------------------------------------------------------------------------

// TaskContext represents the full execution context for a task.
// It aggregates the task description, constraints, conversation history,
// and file state - everything an agent needs to execute the task.
type TaskContext struct {
	// ID is the unique identifier for this task context.
	ID string
	// TaskID is the parent task ID this context belongs to.
	TaskID string
	// Goal is the task objective/description.
	Goal string
	// Constraints are the task constraints (hard and soft).
	Constraints []*Constraint
	// Messages is the chronological conversation history.
	Messages []*ConversationMessage
	// FileStates tracks the state of relevant files.
	FileStates []*FileState
	// TokenBudget is the maximum tokens allowed for this context.
	TokenBudget int
	// TokensUsed is the current estimated token count.
	TokensUsed int
	// CreatedAt is when this context was created.
	CreatedAt time.Time
	// UpdatedAt is when this context was last modified.
	UpdatedAt time.Time
	// Version is the context version for optimistic locking.
	Version int
}

// NewTaskContext creates a new TaskContext with a generated ULID.
// Default token budget is 128k tokens (128000).
func NewTaskContext(taskID, goal string) *TaskContext {
	now := time.Now().UTC()
	return &TaskContext{
		ID:           NewULID(),
		TaskID:       taskID,
		Goal:         goal,
		Constraints:  []*Constraint{},
		Messages:     []*ConversationMessage{},
		FileStates:   []*FileState{},
		TokenBudget:  128000,
		TokensUsed:   0,
		CreatedAt:    now,
		UpdatedAt:    now,
		Version:      1,
	}
}

// SetTokenBudget sets the token budget for this context.
func (tc *TaskContext) SetTokenBudget(budget int) *TaskContext {
	tc.TokenBudget = budget
	return tc
}

// AddConstraint adds a constraint to the context.
func (tc *TaskContext) AddConstraint(c *Constraint) {
	tc.Constraints = append(tc.Constraints, c)
	tc.UpdatedAt = time.Now().UTC()
	tc.Version++
}

// AddMessage adds a conversation message to the context.
func (tc *TaskContext) AddMessage(m *ConversationMessage) {
	tc.Messages = append(tc.Messages, m)
	tc.TokensUsed += m.TokenCount
	tc.UpdatedAt = time.Now().UTC()
	tc.Version++
}

// AddFileState adds a file state entry to the context.
func (tc *TaskContext) AddFileState(fs *FileState) {
	tc.FileStates = append(tc.FileStates, fs)
	tc.UpdatedAt = time.Now().UTC()
	tc.Version++
}

// GetConstraintByID returns a constraint by its ID.
func (tc *TaskContext) GetConstraintByID(id string) *Constraint {
	for _, c := range tc.Constraints {
		if c.ID == id {
			return c
		}
	}
	return nil
}

// GetFileStateByPath returns the most recent file state for a path.
func (tc *TaskContext) GetFileStateByPath(path string) *FileState {
	for i := len(tc.FileStates) - 1; i >= 0; i-- {
		if tc.FileStates[i].Path == path {
			return tc.FileStates[i]
		}
	}
	return nil
}

// GetHardConstraints returns all hard (NON_NEGOTIABLE) constraints.
func (tc *TaskContext) GetHardConstraints() []*Constraint {
	var result []*Constraint
	for _, c := range tc.Constraints {
		if c.IsHard() {
			result = append(result, c)
		}
	}
	return result
}

// GetMessagesBySubtask returns all messages associated with a subtask.
func (tc *TaskContext) GetMessagesBySubtask(subtaskID string) []*ConversationMessage {
	var result []*ConversationMessage
	for _, m := range tc.Messages {
		if m.SubtaskID != nil && *m.SubtaskID == subtaskID {
			result = append(result, m)
		}
	}
	return result
}

// HardConstraintsCount returns the number of hard constraints.
func (tc *TaskContext) HardConstraintsCount() int {
	count := 0
	for _, c := range tc.Constraints {
		if c.IsHard() {
			count++
		}
	}
	return count
}

// TotalTokens returns the current estimated token count.
// This includes constraints, messages, and file metadata.
func (tc *TaskContext) TotalTokens() int {
	tokens := tc.TokensUsed
	// Estimate tokens for constraints
	for _, c := range tc.Constraints {
		tokens += estimateTokens(c.Description)
		tokens += 20 // overhead for constraint structure
	}
	// Estimate tokens for file states
	for _, f := range tc.FileStates {
		tokens += estimateTokens(f.Path)
		tokens += estimateTokens(f.DiffSummary)
		tokens += 30 // overhead for file state structure
	}
	return tokens
}

// RemainingBudget returns the remaining token budget.
func (tc *TaskContext) RemainingBudget() int {
	return tc.TokenBudget - tc.TotalTokens()
}

// IsBudgetExceeded reports whether the context has exceeded its token budget.
func (tc *TaskContext) IsBudgetExceeded() bool {
	return tc.TotalTokens() > tc.TokenBudget
}

// RecalculateTokens recomputes the token count from all components.
// Call this after deserialization or manual modifications.
func (tc *TaskContext) RecalculateTokens() {
	tc.TokensUsed = 0
	for _, m := range tc.Messages {
		tc.TokensUsed += m.TokenCount
	}
}

// GetConstraintCategories returns all unique constraint categories.
func (tc *TaskContext) GetConstraintCategories() []string {
	seen := make(map[string]bool)
	var categories []string
	for _, c := range tc.Constraints {
		if !seen[c.Category] {
			seen[c.Category] = true
			categories = append(categories, c.Category)
		}
	}
	sort.Strings(categories)
	return categories
}

// Clone creates a deep copy of the TaskContext.
func (tc *TaskContext) Clone() *TaskContext {
	// Deep copy constraints
	constraints := make([]*Constraint, len(tc.Constraints))
	for i, c := range tc.Constraints {
		constraints[i] = &Constraint{
			ID:          c.ID,
			Strength:    c.Strength,
			Category:    c.Category,
			Description: c.Description,
			CreatedAt:   c.CreatedAt,
			Priority:    c.Priority,
		}
	}

	// Deep copy messages
	messages := make([]*ConversationMessage, len(tc.Messages))
	for i, m := range tc.Messages {
		var subtaskID *string
		if m.SubtaskID != nil {
			sid := *m.SubtaskID
			subtaskID = &sid
		}
		messages[i] = &ConversationMessage{
			ID:          m.ID,
			Role:        m.Role,
			Content:     m.Content,
			Timestamp:   m.Timestamp,
			TokenCount:  m.TokenCount,
			Priority:    m.Priority,
			SubtaskID:   subtaskID,
		}
	}

	// Deep copy file states
	fileStates := make([]*FileState, len(tc.FileStates))
	for i, f := range tc.FileStates {
		fileStates[i] = &FileState{
			ID:          f.ID,
			Path:        f.Path,
			ContentHash: f.ContentHash,
			ModifiedAt:  f.ModifiedAt,
			ChangeType:  f.ChangeType,
			Priority:    f.Priority,
			SizeBytes:   f.SizeBytes,
			DiffSummary: f.DiffSummary,
		}
	}

	return &TaskContext{
		ID:           tc.ID,
		TaskID:       tc.TaskID,
		Goal:         tc.Goal,
		Constraints:  constraints,
		Messages:     messages,
		FileStates:   fileStates,
		TokenBudget:  tc.TokenBudget,
		TokensUsed:   tc.TokensUsed,
		CreatedAt:    tc.CreatedAt,
		UpdatedAt:    tc.UpdatedAt,
		Version:      tc.Version,
	}
}

// MarshalJSON implements json.Marshaler for TaskContext.
func (tc *TaskContext) MarshalJSON() ([]byte, error) {
	type Alias TaskContext
	return json.Marshal(&struct {
		*Alias
		TotalTokens_ int `json:"total_tokens"`
	}{
		Alias:        (*Alias)(tc),
		TotalTokens_: tc.TotalTokens(),
	})
}

// ----------------------------------------------------------------------------
// Token estimation utilities
// ----------------------------------------------------------------------------

// estimateTokens provides a rough token count estimate.
// This uses a simple approximation: ~4 characters per token for English text.
// For production, this should be replaced with a proper tokenizer
// (e.g., tiktoken or similar) when this package is used in L4-Service.
func estimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	// Rough approximation: 4 characters per token
	return (len(text) + 3) / 4
}

// EstimateTokensForText estimates tokens for a given text.
func EstimateTokensForText(text string) int {
	return estimateTokens(text)
}

// EstimateTokensForStruct estimates the token overhead for a struct.
func EstimateTokensForStruct(fieldCount int) int {
	// Rough overhead: ~10 tokens per field for JSON struct
	return fieldCount * 10
}
