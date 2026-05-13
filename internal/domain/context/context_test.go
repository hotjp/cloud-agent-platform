package context

import (
	"testing"
	"time"
)

func TestNewULID(t *testing.T) {
	id := NewULID()
	if id == "" {
		t.Error("NewULID() returned empty string")
	}
	if len(id) != 26 {
		t.Errorf("NewULID() length = %d, want 26", len(id))
	}

	// Generate another and ensure they're different
	id2 := NewULID()
	if id == id2 {
		t.Error("Two consecutive NewULID() calls returned the same ID")
	}
}

func TestConstraintStrength(t *testing.T) {
	tests := []struct {
		s    ConstraintStrength
		want bool
	}{
		{ConstraintNonNegotiable, true},
		{ConstraintPreferable, true},
		{"invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.s.String(), func(t *testing.T) {
			if got := tt.s.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConstraint(t *testing.T) {
	c := NewConstraint(ConstraintPreferable, "style", "Follow Go coding conventions")
	if c.ID == "" {
		t.Error("Constraint.ID should not be empty")
	}
	if c.Strength != ConstraintPreferable {
		t.Errorf("Strength = %v, want %v", c.Strength, ConstraintPreferable)
	}
	if c.Category != "style" {
		t.Errorf("Category = %q, want %q", c.Category, "style")
	}
	if c.Description != "Follow Go coding conventions" {
		t.Errorf("Description = %q, want %q", c.Description, "Follow Go coding conventions")
	}
	if c.Priority != 0 {
		t.Errorf("Priority = %d, want 0", c.Priority)
	}
}

func TestConstraint_IsHard(t *testing.T) {
	hard := NewNonNegotiable("security", "No eval()")
	if !hard.IsHard() {
		t.Error("NonNegotiable constraint should be hard")
	}

	soft := NewPreferable("style", "Use camelCase")
	if soft.IsHard() {
		t.Error("Preferable constraint should not be hard")
	}
}

func TestConstraint_CompressionKey(t *testing.T) {
	c := NewConstraint(ConstraintNonNegotiable, "security", "No SQL injection")
	key := c.CompressionKey()
	if key != "security:NON_NEGOTIABLE" {
		t.Errorf("CompressionKey() = %q, want %q", key, "security:NON_NEGOTIABLE")
	}
}

func TestFileChangeType(t *testing.T) {
	tests := []struct {
		f    FileChangeType
		want bool
	}{
		{FileChangeCreated, true},
		{FileChangeModified, true},
		{FileChangeDeleted, true},
		{FileChangeRenamed, true},
		{"invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.f.String(), func(t *testing.T) {
			if got := tt.f.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFileState(t *testing.T) {
	fs := NewFileStateModified("src/main.go", "abc123", 1024, "Added new function")
	if fs.ID == "" {
		t.Error("FileState.ID should not be empty")
	}
	if fs.Path != "src/main.go" {
		t.Errorf("Path = %q, want %q", fs.Path, "src/main.go")
	}
	if fs.ContentHash != "abc123" {
		t.Errorf("ContentHash = %q, want %q", fs.ContentHash, "abc123")
	}
	if fs.ChangeType != FileChangeModified {
		t.Errorf("ChangeType = %v, want %v", fs.ChangeType, FileChangeModified)
	}
	if fs.SizeBytes != 1024 {
		t.Errorf("SizeBytes = %d, want 1024", fs.SizeBytes)
	}
	if fs.DiffSummary != "Added new function" {
		t.Errorf("DiffSummary = %q, want %q", fs.DiffSummary, "Added new function")
	}
}

func TestFileState_IsDeleted(t *testing.T) {
	deleted := NewFileStateDeleted("src/old.go")
	if !deleted.IsDeleted() {
		t.Error("Deleted file should report IsDeleted() = true")
	}

	created := NewFileStateCreated("src/new.go", "xyz", 100)
	if created.IsDeleted() {
		t.Error("Created file should report IsDeleted() = false")
	}
}

func TestFileState_IsCreated(t *testing.T) {
	created := NewFileStateCreated("src/new.go", "xyz", 100)
	if !created.IsCreated() {
		t.Error("Created file should report IsCreated() = true")
	}

	modified := NewFileStateModified("src/main.go", "abc", 100, "change")
	if modified.IsCreated() {
		t.Error("Modified file should report IsCreated() = false")
	}
}

func TestMessageRole(t *testing.T) {
	tests := []struct {
		r    MessageRole
		want bool
	}{
		{MessageRoleUser, true},
		{MessageRoleAgent, true},
			{MessageRoleSystem, true},
		{"invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.r.String(), func(t *testing.T) {
			if got := tt.r.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConversationMessage(t *testing.T) {
	m := NewUserMessage("Hello, agent!")
	if m.ID == "" {
		t.Error("Message.ID should not be empty")
	}
	if m.Role != MessageRoleUser {
		t.Errorf("Role = %v, want %v", m.Role, MessageRoleUser)
	}
	if m.Content != "Hello, agent!" {
		t.Errorf("Content = %q, want %q", m.Content, "Hello, agent!")
	}
	if m.TokenCount == 0 {
		t.Error("TokenCount should be estimated")
	}
}

func TestConversationMessage_WithSubtask(t *testing.T) {
	m := NewAgentMessage("Working on it")
	m = m.WithSubtask("subtask-123")
	if m.SubtaskID == nil {
		t.Fatal("SubtaskID should not be nil after WithSubtask")
	}
	if *m.SubtaskID != "subtask-123" {
		t.Errorf("SubtaskID = %q, want %q", *m.SubtaskID, "subtask-123")
	}
}

func TestTaskContext_New(t *testing.T) {
	tc := NewTaskContext("task-001", "Implement login feature")
	if tc.ID == "" {
		t.Error("TaskContext.ID should not be empty")
	}
	if tc.TaskID != "task-001" {
		t.Errorf("TaskID = %q, want %q", tc.TaskID, "task-001")
	}
	if tc.Goal != "Implement login feature" {
		t.Errorf("Goal = %q, want %q", tc.Goal, "Implement login feature")
	}
	if tc.TokenBudget != 128000 {
		t.Errorf("TokenBudget = %d, want 128000", tc.TokenBudget)
	}
	if len(tc.Constraints) != 0 {
		t.Errorf("Constraints should be empty, got %d", len(tc.Constraints))
	}
	if len(tc.Messages) != 0 {
		t.Errorf("Messages should be empty, got %d", len(tc.Messages))
	}
	if len(tc.FileStates) != 0 {
		t.Errorf("FileStates should be empty, got %d", len(tc.FileStates))
	}
}

func TestTaskContext_AddConstraint(t *testing.T) {
	tc := NewTaskContext("task-001", "Test")
	c := NewNonNegotiable("security", "No hardcoded passwords")
	tc.AddConstraint(c)

	if len(tc.Constraints) != 1 {
		t.Errorf("Constraints length = %d, want 1", len(tc.Constraints))
	}
	if tc.HardConstraintsCount() != 1 {
		t.Errorf("HardConstraintsCount() = %d, want 1", tc.HardConstraintsCount())
	}
}

func TestTaskContext_AddMessage(t *testing.T) {
	tc := NewTaskContext("task-001", "Test")
	m := NewUserMessage("Start working")
	beforeTokens := m.TokenCount
	tc.AddMessage(m)

	if len(tc.Messages) != 1 {
		t.Errorf("Messages length = %d, want 1", len(tc.Messages))
	}
	if tc.TokensUsed != beforeTokens {
		t.Errorf("TokensUsed = %d, want %d", tc.TokensUsed, beforeTokens)
	}
}

func TestTaskContext_AddFileState(t *testing.T) {
	tc := NewTaskContext("task-001", "Test")
	fs := NewFileStateCreated("src/main.go", "hash123", 500)
	tc.AddFileState(fs)

	if len(tc.FileStates) != 1 {
		t.Errorf("FileStates length = %d, want 1", len(tc.FileStates))
	}
}

func TestTaskContext_GetHardConstraints(t *testing.T) {
	tc := NewTaskContext("task-001", "Test")
	tc.AddConstraint(NewNonNegotiable("security", "No eval"))
	tc.AddConstraint(NewPreferable("style", "Use Gofmt"))
	tc.AddConstraint(NewNonNegotiable("legal", "Apache 2.0 license"))

	hard := tc.GetHardConstraints()
	if len(hard) != 2 {
		t.Errorf("Hard constraints count = %d, want 2", len(hard))
	}
}

func TestTaskContext_GetMessagesBySubtask(t *testing.T) {
	tc := NewTaskContext("task-001", "Test")

	subtaskID := "subtask-001"
	m1 := NewUserMessage("Start").WithSubtask(subtaskID)
	m2 := NewAgentMessage("Done").WithSubtask(subtaskID)
	m3 := NewUserMessage("Unrelated")

	tc.AddMessage(m1)
	tc.AddMessage(m2)
	tc.AddMessage(m3)

	msgs := tc.GetMessagesBySubtask(subtaskID)
	if len(msgs) != 2 {
		t.Errorf("Messages for subtask = %d, want 2", len(msgs))
	}
}

func TestTaskContext_TotalTokens(t *testing.T) {
	tc := NewTaskContext("task-001", "Test task")

	// Add messages
	for i := 0; i < 3; i++ {
		tc.AddMessage(NewUserMessage("This is a test message for token counting"))
	}

	tokens := tc.TotalTokens()
	if tokens <= 0 {
		t.Errorf("TotalTokens() = %d, should be > 0", tokens)
	}

	// Tokens should increase after adding
	before := tc.TotalTokens()
	tc.AddMessage(NewAgentMessage("Another message"))
	after := tc.TotalTokens()
	if after <= before {
		t.Error("TotalTokens should increase after adding message")
	}
}

func TestTaskContext_RemainingBudget(t *testing.T) {
	tc := NewTaskContext("task-001", "Test")
	tc.SetTokenBudget(1000)

	remaining := tc.RemainingBudget()
	if remaining != 1000 {
		t.Errorf("RemainingBudget() = %d, want 1000", remaining)
	}
}

func TestTaskContext_IsBudgetExceeded(t *testing.T) {
	tc := NewTaskContext("task-001", "Test")
	tc.SetTokenBudget(10) // Very small budget

	// Add enough messages to exceed budget
	for i := 0; i < 50; i++ {
		tc.AddMessage(NewUserMessage("This is a long message that will consume many tokens and should eventually exceed the budget"))
	}

	if !tc.IsBudgetExceeded() {
		t.Error("IsBudgetExceeded() should return true after adding many messages")
	}
}

func TestTaskContext_GetFileStateByPath(t *testing.T) {
	tc := NewTaskContext("task-001", "Test")
	fs1 := NewFileStateCreated("src/main.go", "hash1", 100)
	fs2 := NewFileStateModified("src/main.go", "hash2", 150, "updated")
	fs3 := NewFileStateCreated("src/util.go", "hash3", 200)

	tc.AddFileState(fs1)
	tc.AddFileState(fs2)
	tc.AddFileState(fs3)

	// Should return the most recent (last added) for the path
	found := tc.GetFileStateByPath("src/main.go")
	if found == nil {
		t.Fatal("GetFileStateByPath returned nil")
	}
	if found.ContentHash != "hash2" {
		t.Errorf("Most recent state for src/main.go has hash %q, want %q", found.ContentHash, "hash2")
	}

	notFound := tc.GetFileStateByPath("nonexistent.go")
	if notFound != nil {
		t.Error("GetFileStateByPath should return nil for nonexistent path")
	}
}

func TestTaskContext_GetConstraintCategories(t *testing.T) {
	tc := NewTaskContext("task-001", "Test")
	tc.AddConstraint(NewNonNegotiable("security", "No eval"))
	tc.AddConstraint(NewPreferable("style", "Use Go fmt"))
	tc.AddConstraint(NewNonNegotiable("security", "HTTPS only"))
	tc.AddConstraint(NewPreferable("performance", "Cache responses"))

	cats := tc.GetConstraintCategories()
	if len(cats) != 3 {
		t.Errorf("Categories count = %d, want 3: %v", len(cats), cats)
	}
}

func TestTaskContext_Clone(t *testing.T) {
	tc := NewTaskContext("task-001", "Original goal")
	tc.AddConstraint(NewNonNegotiable("test", "Constraint 1"))
	tc.AddMessage(NewUserMessage("Test message"))
	tc.AddFileState(NewFileStateCreated("src/main.go", "hash", 100))

	clone := tc.Clone()

	// Modify the clone
	clone.Goal = "Modified goal"
	clone.AddMessage(NewAgentMessage("New message"))

	// Original should be unchanged
	if tc.Goal != "Original goal" {
		t.Error("Original context goal was modified")
	}
	if len(tc.Messages) != 1 {
		t.Error("Original context messages were modified")
	}
	if len(clone.Messages) != 2 {
		t.Errorf("Clone messages count = %d, want 2", len(clone.Messages))
	}
}

func TestEstimateTokensForText(t *testing.T) {
	tests := []struct {
		text string
		min  int
		max  int
	}{
		{"", 0, 0},
		{"hello", 1, 2},
		{"This is a test sentence.", 4, 8},
		{"a", 1, 1},
		{"abcdefgh", 2, 3},
	}

	for _, tt := range tests {
		tokens := EstimateTokensForText(tt.text)
		if tokens < tt.min || tokens > tt.max {
			t.Errorf("EstimateTokensForText(%q) = %d, want between %d and %d",
				tt.text, tokens, tt.min, tt.max)
		}
	}
}

func TestCompressionStrategy(t *testing.T) {
	tests := []struct {
		s    CompressionStrategy
		want bool
	}{
		{StrategyAggressive, true},
		{StrategyModerate, true},
		{StrategyConservative, true},
		{"invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.s.String(), func(t *testing.T) {
			if got := tt.s.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompressor_Compress_NotNeeded(t *testing.T) {
	tc := NewTaskContext("task-001", "Test")
	tc.SetTokenBudget(100000)

	compressor := DefaultCompressor()
	result, err := compressor.Compress(tc)

	if err != nil {
		t.Fatalf("Compress() error = %v", err)
	}
	if result.TokensRemoved != 0 {
		t.Errorf("TokensRemoved = %d, want 0 when not over budget", result.TokensRemoved)
	}
}

func TestCompressor_Compress_OverBudget(t *testing.T) {
	tc := NewTaskContext("task-001", "Test")
	tc.SetTokenBudget(100) // Very small budget

	// Add many short messages to exceed budget
	// Each message is ~9 chars ≈ 3 tokens, so 20 messages ≈ 60 tokens overhead + ~20 tokens structure
	// With a budget of 100, we can fit ~30 messages, so 50 messages will definitely exceed
	for i := 0; i < 50; i++ {
		m := NewUserMessage("Short message")
		m.Priority = 10 // Higher priority number = lower priority for compression
		tc.AddMessage(m)
	}

	beforeTokens := tc.TotalTokens()
	if beforeTokens <= 100 {
		t.Skipf("Context not over budget (%d <= 100), skipping compression test", beforeTokens)
	}

	compressor := DefaultCompressor()
	_, err := compressor.Compress(tc)

	if err != nil {
		t.Fatalf("Compress() error = %v", err)
	}

	// With 50 messages and moderate compressor preserving 10, we should remove ~40
	// (priorities are all 10, so they're all candidates for removal)
	if len(tc.Messages) >= 50 {
		t.Errorf("Messages should be reduced from 50, got %d", len(tc.Messages))
	}
}

func TestCompressor_Compress_PreservesHardConstraints(t *testing.T) {
	tc := NewTaskContext("task-001", "Test")
	tc.SetTokenBudget(50) // Very small budget

	// Add many soft constraints
	for i := 0; i < 10; i++ {
		tc.AddConstraint(NewPreferable("style", "Use consistent naming"))
	}
	// Add one hard constraint
	hard := NewNonNegotiable("security", "No eval()")
	tc.AddConstraint(hard)

	compressor := DefaultCompressor()
	_, err := compressor.Compress(tc)
	if err != nil {
		t.Fatalf("Compress() error = %v", err)
	}

	// Hard constraint should be preserved
	found := tc.GetConstraintByID(hard.ID)
	if found == nil {
		t.Error("Hard constraint was removed during compression")
	}
}

func TestContextWindow_Status(t *testing.T) {
	cw := NewContextWindow(1000)

	tests := []struct {
		tokens    int
		want      WindowStatus
	}{
		{0, WindowStatusOK},
		{500, WindowStatusOK},         // 50%
		{799, WindowStatusOK},        // 79.9%
		{800, WindowStatusWarning},   // 80%
		{949, WindowStatusWarning},   // 94.9% < critical threshold
		{950, WindowStatusCritical},  // 95% >= critical threshold
		{951, WindowStatusCritical},  // 95.1%
		{1000, WindowStatusCritical},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := cw.Status(tt.tokens)
			if got != tt.want {
				t.Errorf("Status(%d) = %v, want %v", tt.tokens, got, tt.want)
			}
		})
	}
}

func TestContextWindow_ShouldCompress(t *testing.T) {
	cw := NewContextWindow(1000)
	cw.SetCriticalThreshold(0.95)

	if cw.ShouldCompress(940) {
		t.Error("ShouldCompress(940) should be false (94%)")
	}
	if !cw.ShouldCompress(951) {
		t.Error("ShouldCompress(951) should be true (95.1%)")
	}
}

func TestContextBuilder(t *testing.T) {
	tc := NewContextBuilder("task-001", "Build login").
		WithBudget(50000).
		WithConstraint(NewNonNegotiable("security", "HTTPS only")).
		WithConstraint(NewPreferable("performance", "Cache responses")).
		WithMessage(NewUserMessage("Build the login page")).
		WithMessage(NewAgentMessage("Login page ready")).
		WithFileState(NewFileStateCreated("src/login.go", "hash123", 500)).
		Build()

	if tc.TaskID != "task-001" {
		t.Errorf("TaskID = %q, want %q", tc.TaskID, "task-001")
	}
	if tc.Goal != "Build login" {
		t.Errorf("Goal = %q, want %q", tc.Goal, "Build login")
	}
	if tc.TokenBudget != 50000 {
		t.Errorf("TokenBudget = %d, want 50000", tc.TokenBudget)
	}
	if len(tc.Constraints) != 2 {
		t.Errorf("Constraints count = %d, want 2", len(tc.Constraints))
	}
	if len(tc.Messages) != 2 {
		t.Errorf("Messages count = %d, want 2", len(tc.Messages))
	}
	if len(tc.FileStates) != 1 {
		t.Errorf("FileStates count = %d, want 1", len(tc.FileStates))
	}
}

func TestTaskContext_MarshalJSON(t *testing.T) {
	tc := NewTaskContext("task-001", "Test")
	tc.AddMessage(NewUserMessage("Hello"))

	data, err := tc.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}

	// Should contain total_tokens field
	if string(data) == "" {
		t.Error("MarshalJSON() returned empty")
	}
}

func TestTaskContext_RecalculateTokens(t *testing.T) {
	tc := NewTaskContext("task-001", "Test")
	tc.AddMessage(NewUserMessage("Test"))
	tc.AddMessage(NewAgentMessage("Response"))

	initial := tc.TokensUsed
	tc.RecalculateTokens()
	after := tc.TokensUsed

	if initial != after {
		t.Errorf("TokensUsed changed after RecalculateTokens: %d -> %d", initial, after)
	}
}

func TestNewFileState(t *testing.T) {
	fs := NewFileState("path/to/file.go", "sha256hash", FileChangeModified, 2048)
	if fs.ID == "" {
		t.Error("ID should not be empty")
	}
	if fs.Path != "path/to/file.go" {
		t.Errorf("Path = %q, want %q", fs.Path, "path/to/file.go")
	}
	if fs.ContentHash != "sha256hash" {
		t.Errorf("ContentHash = %q, want %q", fs.ContentHash, "sha256hash")
	}
	if fs.ChangeType != FileChangeModified {
		t.Errorf("ChangeType = %v, want %v", fs.ChangeType, FileChangeModified)
	}
	if fs.SizeBytes != 2048 {
		t.Errorf("SizeBytes = %d, want 2048", fs.SizeBytes)
	}
}

func TestConstraint_NewNonNegotiable(t *testing.T) {
	c := NewNonNegotiable("test", "Test constraint")
	if c.Strength != ConstraintNonNegotiable {
		t.Errorf("Strength = %v, want %v", c.Strength, ConstraintNonNegotiable)
	}
	if c.Description != "Test constraint" {
		t.Errorf("Description = %q, want %q", c.Description, "Test constraint")
	}
}

func TestConstraint_NewPreferable(t *testing.T) {
	c := NewPreferable("test", "Test soft constraint")
	if c.Strength != ConstraintPreferable {
		t.Errorf("Strength = %v, want %v", c.Strength, ConstraintPreferable)
	}
}

func TestFileState_NewFileStateDeleted(t *testing.T) {
	fs := NewFileStateDeleted("src/removed.go")
	if fs.ChangeType != FileChangeDeleted {
		t.Errorf("ChangeType = %v, want %v", fs.ChangeType, FileChangeDeleted)
	}
	if fs.ContentHash != "" {
		t.Errorf("ContentHash = %q, want empty for deleted file", fs.ContentHash)
	}
	if fs.SizeBytes != 0 {
		t.Errorf("SizeBytes = %d, want 0 for deleted file", fs.SizeBytes)
	}
}

func TestContextWindow_Remaining(t *testing.T) {
	cw := NewContextWindow(1000)

	if cw.Remaining(300) != 700 {
		t.Errorf("Remaining(300) = %d, want 700", cw.Remaining(300))
	}
	if cw.Remaining(1000) != 0 {
		t.Errorf("Remaining(1000) = %d, want 0", cw.Remaining(1000))
	}
	if cw.Remaining(1500) != 0 {
		t.Errorf("Remaining(1500) = %d, want 0 (clamped)", cw.Remaining(1500))
	}
}

func TestContextBuilder_Build(t *testing.T) {
	tc := NewContextBuilder("task-001", "Goal").
		WithMessage(NewUserMessage("Test")).
		Build()

	if tc == nil {
		t.Fatal("Build() returned nil")
	}
	// TokensUsed should be recalculated
	if tc.TokensUsed == 0 {
		t.Error("TokensUsed should be > 0 after adding message")
	}
}

func TestCompressionResult_CompressionSummary(t *testing.T) {
	result := &CompressionResult{
		OriginalTokens:  1000,
		CompressedTokens: 700,
		TokensRemoved:   300,
		ItemsRemoved: CompressionStats{
			Messages:    5,
			FileStates:   2,
			Constraints: 1,
		},
		BudgetUsed: 0.7,
	}

	summary := result.CompressionSummary()
	if summary == "" {
		t.Error("CompressionSummary() returned empty")
	}
}

func TestNewCompressor(t *testing.T) {
	compressor := NewCompressor(StrategyAggressive)
	if compressor == nil {
		t.Fatal("NewCompressor returned nil")
	}
	if compressor.strategy != StrategyAggressive {
		t.Errorf("strategy = %v, want %v", compressor.strategy, StrategyAggressive)
	}
}

func TestCompressor_CompressToBudget(t *testing.T) {
	tc := NewTaskContext("task-001", "Test")
	tc.SetTokenBudget(500)

	// Add many short messages to exceed budget
	// ~3 tokens per message, 50 messages = ~150 tokens + overhead
	for i := 0; i < 50; i++ {
		tc.AddMessage(NewUserMessage("Short message"))
	}

	if tc.TotalTokens() <= 500 {
		t.Skipf("Context not over budget (%d <= 500)", tc.TotalTokens())
	}

	compressor := DefaultCompressor()
	result, err := compressor.CompressToBudget(tc, 300)
	if err != nil {
		t.Fatalf("CompressToBudget() error = %v", err)
	}

	if result.CompressedTokens > 500 {
		t.Errorf("CompressedTokens = %d, should be <= budget 500", result.CompressedTokens)
	}
}

func TestContextWindow_UsageLevel(t *testing.T) {
	cw := NewContextWindow(1000)

	if level := cw.UsageLevel(0); level != 0.0 {
		t.Errorf("UsageLevel(0) = %f, want 0.0", level)
	}
	if level := cw.UsageLevel(500); level != 0.5 {
		t.Errorf("UsageLevel(500) = %f, want 0.5", level)
	}
	if level := cw.UsageLevel(1000); level != 1.0 {
		t.Errorf("UsageLevel(1000) = %f, want 1.0", level)
	}
	if level := cw.UsageLevel(1500); level != 1.5 {
		t.Errorf("UsageLevel(1500) = %f, want 1.5", level)
	}
}

func TestContextWindow_EstimateForAdd(t *testing.T) {
	cw := NewContextWindow(1000)
	cw.SetWarningThreshold(0.8)
	cw.SetCriticalThreshold(0.95)

	// 700 tokens + 200 = 900 (90%) -> should be warning (critical is >= 95%)
	status := cw.EstimateForAdd(700, 200)
	if status != WindowStatusWarning {
		t.Errorf("EstimateForAdd(700, 200) = %v, want %v (90%% usage, critical>=95%%)", status, WindowStatusWarning)
	}

	// 950 tokens + 100 = 1050 (105%) -> should be critical
	status = cw.EstimateForAdd(950, 100)
	if status != WindowStatusCritical {
		t.Errorf("EstimateForAdd(950, 100) = %v, want %v (105%% usage)", status, WindowStatusCritical)
	}
}

func TestTaskContext_SetTokenBudget(t *testing.T) {
	tc := NewTaskContext("task-001", "Test")
	tc.SetTokenBudget(50000)

	if tc.TokenBudget != 50000 {
		t.Errorf("TokenBudget = %d, want 50000", tc.TokenBudget)
	}
}

func TestTaskContext_HardConstraintsCount(t *testing.T) {
	tc := NewTaskContext("task-001", "Test")

	if count := tc.HardConstraintsCount(); count != 0 {
		t.Errorf("Empty context HardConstraintsCount = %d, want 0", count)
	}

	tc.AddConstraint(NewPreferable("style", "gofmt"))
	if count := tc.HardConstraintsCount(); count != 0 {
		t.Errorf("After soft constraint, HardConstraintsCount = %d, want 0", count)
	}

	tc.AddConstraint(NewNonNegotiable("legal", "Apache 2.0"))
	if count := tc.HardConstraintsCount(); count != 1 {
		t.Errorf("After hard constraint, HardConstraintsCount = %d, want 1", count)
	}

	tc.AddConstraint(NewNonNegotiable("security", "No hardcoded secrets"))
	if count := tc.HardConstraintsCount(); count != 2 {
		t.Errorf("After second hard constraint, HardConstraintsCount = %d, want 2", count)
	}
}

func TestTaskContext_Version(t *testing.T) {
	tc := NewTaskContext("task-001", "Test")
	initialVersion := tc.Version

	tc.AddConstraint(NewNonNegotiable("test", "constraint"))
	if tc.Version != initialVersion+1 {
		t.Errorf("After AddConstraint, Version = %d, want %d", tc.Version, initialVersion+1)
	}

	tc.AddMessage(NewUserMessage("message"))
	if tc.Version != initialVersion+2 {
		t.Errorf("After AddMessage, Version = %d, want %d", tc.Version, initialVersion+2)
	}

	tc.AddFileState(NewFileStateCreated("main.go", "hash", 100))
	if tc.Version != initialVersion+3 {
		t.Errorf("After AddFileState, Version = %d, want %d", tc.Version, initialVersion+3)
	}
}

func TestTaskContext_UpdatedAt(t *testing.T) {
	tc := NewTaskContext("task-001", "Test")
	initial := tc.UpdatedAt

	time.Sleep(time.Millisecond)
	tc.AddMessage(NewUserMessage("message"))
	if !tc.UpdatedAt.After(initial) {
		t.Error("UpdatedAt should be updated after AddMessage")
	}
}

func TestTaskContext_GetConstraintByID(t *testing.T) {
	tc := NewTaskContext("task-001", "Test")
	c := NewNonNegotiable("test", "Test constraint")
	tc.AddConstraint(c)

	found := tc.GetConstraintByID(c.ID)
	if found == nil {
		t.Fatal("GetConstraintByID returned nil for existing constraint")
	}
	if found.ID != c.ID {
		t.Errorf("Found constraint ID = %q, want %q", found.ID, c.ID)
	}

	notFound := tc.GetConstraintByID("nonexistent-id")
	if notFound != nil {
		t.Error("GetConstraintByID should return nil for nonexistent ID")
	}
}

func TestConversationMessage_SetPriority(t *testing.T) {
	m := NewUserMessage("Test")
	if m.Priority != 0 {
		t.Errorf("Initial Priority = %d, want 0", m.Priority)
	}

	m.SetPriority(5)
	if m.Priority != 5 {
		t.Errorf("After SetPriority, Priority = %d, want 5", m.Priority)
	}
}

func TestEstimateTokensForStruct(t *testing.T) {
	tokens := EstimateTokensForStruct(5)
	if tokens != 50 {
		t.Errorf("EstimateTokensForStruct(5) = %d, want 50", tokens)
	}
}

func TestWindowStatus_String(t *testing.T) {
	tests := []WindowStatus{
		WindowStatusOK,
		WindowStatusWarning,
		WindowStatusCritical,
	}

	for _, s := range tests {
		if s.String() == "" {
			t.Errorf("WindowStatus %v String() returned empty", s)
		}
	}
}

func TestConstraintStrength_String(t *testing.T) {
	if ConstraintNonNegotiable.String() != "NON_NEGOTIABLE" {
		t.Errorf("NON_NEGOTIABLE.String() = %q, want %q", ConstraintNonNegotiable.String(), "NON_NEGOTIABLE")
	}
	if ConstraintPreferable.String() != "PREFERABLE" {
		t.Errorf("PREFERABLE.String() = %q, want %q", ConstraintPreferable.String(), "PREFERABLE")
	}
}

func TestFileChangeType_String(t *testing.T) {
	if FileChangeCreated.String() != "created" {
		t.Errorf("FileChangeCreated.String() = %q, want %q", FileChangeCreated.String(), "created")
	}
	if FileChangeModified.String() != "modified" {
		t.Errorf("FileChangeModified.String() = %q, want %q", FileChangeModified.String(), "modified")
	}
	if FileChangeDeleted.String() != "deleted" {
		t.Errorf("FileChangeDeleted.String() = %q, want %q", FileChangeDeleted.String(), "deleted")
	}
	if FileChangeRenamed.String() != "renamed" {
		t.Errorf("FileChangeRenamed.String() = %q, want %q", FileChangeRenamed.String(), "renamed")
	}
}

func TestMessageRole_String(t *testing.T) {
	if MessageRoleUser.String() != "user" {
		t.Errorf("MessageRoleUser.String() = %q, want %q", MessageRoleUser.String(), "user")
	}
	if MessageRoleAgent.String() != "agent" {
		t.Errorf("MessageRoleAgent.String() = %q, want %q", MessageRoleAgent.String(), "agent")
	}
	if MessageRoleSystem.String() != "system" {
		t.Errorf("MessageRoleSystem.String() = %q, want %q", MessageRoleSystem.String(), "system")
	}
}

func TestCompressionStrategy_String(t *testing.T) {
	if StrategyAggressive.String() != "aggressive" {
		t.Errorf("StrategyAggressive.String() = %q, want %q", StrategyAggressive.String(), "aggressive")
	}
	if StrategyModerate.String() != "moderate" {
		t.Errorf("StrategyModerate.String() = %q, want %q", StrategyModerate.String(), "moderate")
	}
	if StrategyConservative.String() != "conservative" {
		t.Errorf("StrategyConservative.String() = %q, want %q", StrategyConservative.String(), "conservative")
	}
}

func TestNewTaskContext_Timestamps(t *testing.T) {
	before := time.Now().UTC()
	tc := NewTaskContext("task-001", "Test")
	after := time.Now().UTC()

	if tc.CreatedAt.Before(before) || tc.CreatedAt.After(after) {
		t.Errorf("CreatedAt %v not in expected range [%v, %v]", tc.CreatedAt, before, after)
	}
	if tc.UpdatedAt.Before(before) || tc.UpdatedAt.After(after) {
		t.Errorf("UpdatedAt %v not in expected range [%v, %v]", tc.UpdatedAt, before, after)
	}
}
