// Package mcp implements the MCP (Model Context Protocol) server for the Cloud Agent Platform.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// PlatformClient communicates with the Cloud Agent Platform REST API.
type PlatformClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
	logger     *zap.Logger
}

// NewPlatformClient creates a new platform API client.
func NewPlatformClient(baseURL, token string, logger *zap.Logger) *PlatformClient {
	return &PlatformClient{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

// API Response types

// APIResponse is the standard API response wrapper.
type APIResponse struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data,omitempty"`
	Error    *APIError       `json:"error,omitempty"`
	RequestID string         `json:"requestId,omitempty"`
}

// APIError represents an API error.
type APIError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Detail  map[string]any `json:"detail,omitempty"`
}

// TaskSubmitRequest matches the REST API SubmitTaskRequest.
type TaskSubmitRequest struct {
	Goal                  string              `json:"goal"`
	Repository            *RepositoryInput    `json:"repository,omitempty"`
	Constraints           []string            `json:"constraints,omitempty"`
	VerificationCriteria  []string            `json:"verificationCriteria,omitempty"`
	Priority              int                 `json:"priority,omitempty"`
	Timeout               int                 `json:"timeout,omitempty"`
	AgentHint             *AgentHintInput     `json:"agentHint,omitempty"`
	Tags                  []string            `json:"tags,omitempty"`
}

// RepositoryInput for task submission.
type RepositoryInput struct {
	URL           string `json:"url"`
	Branch        string `json:"branch"`
	ResultBranch  string `json:"resultBranch,omitempty"`
}

// AgentHintInput for task submission.
type AgentHintInput struct {
	Templates []string `json:"templates,omitempty"`
	Model     string   `json:"model,omitempty"`
	MaxAgents int      `json:"maxAgents,omitempty"`
}

// TaskSubmitResponse matches SubmitTaskResponse.
type TaskSubmitResponse struct {
	TaskID           string `json:"taskId"`
	Status           string `json:"status"`
	ResultBranch     string `json:"resultBranch"`
	CreatedAt        string `json:"createdAt"`
	EstimatedDuration int   `json:"estimatedDuration,omitempty"`
}

// TaskStatusResponse matches TaskStatus from the contract.
type TaskStatusResponse struct {
	TaskID       string             `json:"taskId"`
	Status       string             `json:"status"`
	Goal         string             `json:"goal"`
	Priority     int                `json:"priority"`
	ResultBranch string             `json:"resultBranch"`
	Progress     int                `json:"progress"`
	CurrentPhase string             `json:"currentPhase,omitempty"`
	Subtasks     []SubtaskStatus    `json:"subtasks,omitempty"`
	CreatedAt    string             `json:"createdAt"`
	StartedAt    string             `json:"startedAt,omitempty"`
	CompletedAt  string             `json:"completedAt,omitempty"`
	Result       *TaskResultResponse `json:"result,omitempty"`
	Stats        *TaskStats         `json:"stats,omitempty"`
}

// SubtaskStatus matches SubTaskStatus from the contract.
type SubtaskStatus struct {
	SubtaskID      string   `json:"subtaskId"`
	Type           string   `json:"type"`
	AgentTemplate  string   `json:"agentTemplate"`
	Status         string   `json:"status"`
	Summary        *string  `json:"summary,omitempty"`
	Artifacts      []ArtifactRef `json:"artifacts,omitempty"`
	StartedAt      string   `json:"startedAt,omitempty"`
	CompletedAt    string   `json:"completedAt,omitempty"`
}

// ArtifactRef matches ArtifactRef from the contract.
type ArtifactRef struct {
	ArtifactID string `json:"artifactId"`
	Type       string `json:"type"`
	Summary    string `json:"summary"`
	URL        string `json:"url"`
	Size       int64  `json:"size"`
}

// TaskResultResponse matches TaskResult from the contract.
type TaskResultResponse struct {
	GitCommit   string        `json:"gitCommit"`
	GitBranch   string        `json:"gitBranch"`
	Summary     string        `json:"summary"`
	Changes     []FileChange   `json:"changes,omitempty"`
	TestResults *TestResult   `json:"testResults,omitempty"`
	QualityScore *int         `json:"qualityScore,omitempty"`
}

// FileChange matches FileChange from the contract.
type FileChange struct {
	Path     string `json:"path"`
	Action   string `json:"action"`
	Additions int   `json:"additions"`
	Deletions int   `json:"deletions"`
}

// TestResult matches TestResult from the contract.
type TestResult struct {
	Total     int     `json:"total"`
	Passed    int     `json:"passed"`
	Failed    int     `json:"failed"`
	Coverage  *float64 `json:"coverage,omitempty"`
}

// TaskStats matches stats from the contract.
type TaskStats struct {
	AgentsUsed    int     `json:"agentsUsed"`
	TokensUsed    int     `json:"tokensUsed"`
	EstimatedCost float64 `json:"estimatedCost"`
}

// TaskListResponse matches TaskListResponse from the contract.
type TaskListResponse struct {
	Tasks    []TaskStatusResponse `json:"tasks"`
	Total    int                 `json:"total"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"pageSize"`
}

// CancelTaskRequest matches CancelTaskRequest from the contract.
type CancelTaskRequest struct {
	Reason          string `json:"reason"`
	TerminateAgents bool   `json:"terminateAgents,omitempty"`
}

// CancelTaskResponse matches CancelTaskResponse from the contract.
type CancelTaskResponse struct {
	TaskID           string        `json:"taskId"`
	Status           string        `json:"status"`
	TerminatedAgents []string      `json:"terminatedAgents,omitempty"`
	PartialResult    *ArtifactRef  `json:"partialResult,omitempty"`
}

// DecideRequest matches UserDecision from the contract.
type DecideRequest struct {
	Decision    string `json:"decision"`
	Feedback    string `json:"feedback,omitempty"`
	Modifications string `json:"modifications,omitempty"`
}

// DecideResponse is the response for decide.
type DecideResponse struct {
	TaskID    string `json:"taskId"`
	SubtaskID string `json:"subtaskId"`
	Status    string `json:"status"`
}

// AgentTemplateResponse matches AgentTemplate from the contract.
type AgentTemplateResponse struct {
	TemplateID       string            `json:"templateId"`
	Name             string            `json:"name"`
	Description      string            `json:"description"`
	Capabilities     map[string]int    `json:"capabilities"`
	AvailableModels  []string          `json:"availableModels"`
	MaxConcurrent    int               `json:"maxConcurrent"`
	AvgCompletionTime int              `json:"avgCompletionTime"`
	SuccessRate      float64           `json:"successRate"`
}

// PlatformStatusResponse matches PlatformStatus from the contract.
type PlatformStatusResponse struct {
	Pool struct {
		Total      int `json:"total"`
		Idle       int `json:"idle"`
		Busy       int `json:"busy"`
		MaxCapacity int `json:"maxCapacity"`
	} `json:"pool"`
	Queue struct {
		Pending    int     `json:"pending"`
		AvgWaitTime float64 `json:"avgWaitTime"`
	} `json:"queue"`
	Models []ModelStatus `json:"models"`
	Uptime int          `json:"uptime"`
}

// ModelStatus matches the model status from the contract.
type ModelStatus struct {
	ModelID    string `json:"modelId"`
	Available  bool   `json:"available"`
	AvgLatency int    `json:"avgLatency"`
}

// SessionResponse represents session info (for session_list tool).
type SessionResponse struct {
	Sessions []Session `json:"sessions"`
	Total    int       `json:"total"`
}

// Session represents a session.
type Session struct {
	SessionID string `json:"sessionId"`
	ClientID  string `json:"clientId"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// DecomposeTaskResponse represents the result of task decomposition.
type DecomposeTaskResponse struct {
	TaskID     string   `json:"taskId"`
	Subtasks   []string `json:"subtasks"`
}

// do performs an HTTP request to the platform API.
func (c *PlatformClient) do(ctx context.Context, method, path string, body any) (*APIResponse, error) {
	var reqBody []byte
	var err error
	if body != nil {
		reqBody, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
	}

	url := c.baseURL + path
	c.logger.Debug("API request",
		zap.String("method", method),
		zap.String("url", url),
		zap.String("layer", "MCP"),
	)

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	var apiResp APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &apiResp, nil
}

// SubmitTask calls POST /tasks.
func (c *PlatformClient) SubmitTask(ctx context.Context, req TaskSubmitRequest) (*TaskSubmitResponse, error) {
	apiResp, err := c.do(ctx, http.MethodPost, "/api/v1/tasks", req)
	if err != nil {
		return nil, err
	}

	if !apiResp.OK {
		return nil, fmt.Errorf("API error: %s - %s", apiResp.Error.Code, apiResp.Error.Message)
	}

	var result TaskSubmitResponse
	if err := json.Unmarshal(apiResp.Data, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &result, nil
}

// GetTask calls GET /tasks/:taskId.
func (c *PlatformClient) GetTask(ctx context.Context, taskID string) (*TaskStatusResponse, error) {
	apiResp, err := c.do(ctx, http.MethodGet, "/api/v1/tasks/"+taskID, nil)
	if err != nil {
		return nil, err
	}

	if !apiResp.OK {
		return nil, fmt.Errorf("API error: %s - %s", apiResp.Error.Code, apiResp.Error.Message)
	}

	var result TaskStatusResponse
	if err := json.Unmarshal(apiResp.Data, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &result, nil
}

// ListTasks calls GET /tasks.
func (c *PlatformClient) ListTasks(ctx context.Context, status string, tags []string, limit int) (*TaskListResponse, error) {
	path := "/api/v1/tasks?page=1"
	if limit > 0 {
		path += fmt.Sprintf("&pageSize=%d", limit)
	}
	if status != "" {
		path += "&status=" + status
	}
	for _, tag := range tags {
		path += "&tag=" + tag
	}

	apiResp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	if !apiResp.OK {
		return nil, fmt.Errorf("API error: %s - %s", apiResp.Error.Code, apiResp.Error.Message)
	}

	var result TaskListResponse
	if err := json.Unmarshal(apiResp.Data, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &result, nil
}

// CancelTask calls POST /tasks/:taskId/cancel.
func (c *PlatformClient) CancelTask(ctx context.Context, taskID string, reason string) (*CancelTaskResponse, error) {
	req := CancelTaskRequest{
		Reason:          reason,
		TerminateAgents: true,
	}

	apiResp, err := c.do(ctx, http.MethodPost, "/api/v1/tasks/"+taskID+"/cancel", req)
	if err != nil {
		return nil, err
	}

	if !apiResp.OK {
		return nil, fmt.Errorf("API error: %s - %s", apiResp.Error.Code, apiResp.Error.Message)
	}

	var result CancelTaskResponse
	if err := json.Unmarshal(apiResp.Data, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &result, nil
}

// DecideTask calls POST /tasks/:taskId/subtasks/:subtaskId/decision.
func (c *PlatformClient) DecideTask(ctx context.Context, taskID, subtaskID, decision, feedback string) (*DecideResponse, error) {
	req := DecideRequest{
		Decision: decision,
		Feedback: feedback,
	}

	path := fmt.Sprintf("/api/v1/tasks/%s/subtasks/%s/decision", taskID, subtaskID)
	apiResp, err := c.do(ctx, http.MethodPost, path, req)
	if err != nil {
		return nil, err
	}

	if !apiResp.OK {
		return nil, fmt.Errorf("API error: %s - %s", apiResp.Error.Code, apiResp.Error.Message)
	}

	var result DecideResponse
	if err := json.Unmarshal(apiResp.Data, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &result, nil
}

// ListAgentTemplates calls GET /agent-templates.
func (c *PlatformClient) ListAgentTemplates(ctx context.Context) ([]AgentTemplateResponse, error) {
	apiResp, err := c.do(ctx, http.MethodGet, "/api/v1/agent-templates", nil)
	if err != nil {
		return nil, err
	}

	if !apiResp.OK {
		return nil, fmt.Errorf("API error: %s - %s", apiResp.Error.Code, apiResp.Error.Message)
	}

	var result []AgentTemplateResponse
	if err := json.Unmarshal(apiResp.Data, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return result, nil
}

// GetPlatformStatus calls GET /platform/status.
func (c *PlatformClient) GetPlatformStatus(ctx context.Context) (*PlatformStatusResponse, error) {
	apiResp, err := c.do(ctx, http.MethodGet, "/api/v1/platform/status", nil)
	if err != nil {
		return nil, err
	}

	if !apiResp.OK {
		return nil, fmt.Errorf("API error: %s - %s", apiResp.Error.Code, apiResp.Error.Message)
	}

	var result PlatformStatusResponse
	if err := json.Unmarshal(apiResp.Data, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &result, nil
}

// ListSessions calls GET /sessions (placeholder - not in contract, implementing for session_list tool).
func (c *PlatformClient) ListSessions(ctx context.Context) (*SessionResponse, error) {
	apiResp, err := c.do(ctx, http.MethodGet, "/api/v1/sessions", nil)
	if err != nil {
		return nil, err
	}

	if !apiResp.OK {
		return nil, fmt.Errorf("API error: %s - %s", apiResp.Error.Code, apiResp.Error.Message)
	}

	var result SessionResponse
	if err := json.Unmarshal(apiResp.Data, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &result, nil
}

// DecomposeTask calls POST /tasks/:taskId/decompose (placeholder).
func (c *PlatformClient) DecomposeTask(ctx context.Context, taskID string) (*DecomposeTaskResponse, error) {
	path := "/api/v1/tasks/" + taskID + "/decompose"
	apiResp, err := c.do(ctx, http.MethodPost, path, nil)
	if err != nil {
		return nil, err
	}

	if !apiResp.OK {
		return nil, fmt.Errorf("API error: %s - %s", apiResp.Error.Code, apiResp.Error.Message)
	}

	var result DecomposeTaskResponse
	if err := json.Unmarshal(apiResp.Data, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &result, nil
}
