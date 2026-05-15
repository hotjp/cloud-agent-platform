// Package e2e provides end-to-end tests for the Cloud Agent Platform MCP server.
package e2e

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cloud-agent-platform/cap/internal/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// mockPlatformAPI is a mock HTTP server that implements the platform API endpoints.
type mockPlatformAPI struct {
	tasks          map[string]*mockTask
	agentTemplates []mcp.AgentTemplateResponse
	sessions       []mcp.Session
	t              *testing.T
}

type mockTask struct {
	TaskID       string
	Status       string
	Goal         string
	Priority     int
	ResultBranch string
	Progress     int
	Subtasks     []mockSubtask
}

type mockSubtask struct {
	SubtaskID     string
	Type          string
	AgentTemplate string
	Status        string
}

func newMockPlatformAPI(t *testing.T) *mockPlatformAPI {
	return &mockPlatformAPI{
		tasks:          make(map[string]*mockTask),
		agentTemplates: newMockAgentTemplates(),
		sessions:       []mcp.Session{},
		t:              t,
	}
}

func newMockAgentTemplates() []mcp.AgentTemplateResponse {
	return []mcp.AgentTemplateResponse{
		{
			TemplateID:        "executor",
			Name:              "Executor Agent",
			Description:       "Executes coding tasks with high precision",
			Capabilities:      map[string]int{"coding": 9, "testing": 7},
			AvailableModels:   []string{"claude-3-5-sonnet", "claude-3-opus"},
			MaxConcurrent:     3,
			AvgCompletionTime: 300,
			SuccessRate:       0.95,
		},
		{
			TemplateID:        "strategist",
			Name:              "Strategist Agent",
			Description:       "Analyzes requirements and creates execution plans",
			Capabilities:      map[string]int{"analysis": 9, "research": 8},
			AvailableModels:   []string{"claude-3-5-sonnet", "claude-3-haiku"},
			MaxConcurrent:     1,
			AvgCompletionTime: 120,
			SuccessRate:       0.98,
		},
		{
			TemplateID:        "tester",
			Name:              "Tester Agent",
			Description:       "Writes comprehensive tests and validates functionality",
			Capabilities:      map[string]int{"testing": 9, "review": 6},
			AvailableModels:   []string{"claude-3-5-sonnet"},
			MaxConcurrent:     2,
			AvgCompletionTime: 240,
			SuccessRate:       0.92,
		},
		{
			TemplateID:        "guardian",
			Name:              "Guardian Agent",
			Description:       "Reviews code for security and quality issues",
			Capabilities:      map[string]int{"review": 9, "analysis": 7},
			AvailableModels:   []string{"claude-3-opus"},
			MaxConcurrent:     2,
			AvgCompletionTime: 180,
			SuccessRate:       0.97,
		},
	}
}

func (m *mockPlatformAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.t.Logf("Mock API: %s %s", r.Method, r.URL.Path)

	w.Header().Set("Content-Type", "application/json")

	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/tasks":
		m.handleTaskSubmit(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/agent-templates":
		m.handleAgentTemplates(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/sessions":
		m.handleSessions(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/tasks":
		m.handleListTasks(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/tasks/") && strings.HasSuffix(r.URL.Path, "/cancel"):
		m.handleTaskCancel(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/tasks/") && strings.HasSuffix(r.URL.Path, "/decompose"):
		m.handleTaskDecompose(w, r)
	case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/subtasks/") && strings.HasSuffix(r.URL.Path, "/decision"):
		m.handleSubtaskDecision(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/tasks/"):
		m.handleGetTask(w, r)
	default:
		resp := mcp.APIResponse{
			OK: false,
			Error: &mcp.APIError{
				Code:    "NOT_FOUND",
				Message: "Endpoint not found: " + r.URL.Path,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}
}

func (m *mockPlatformAPI) handleTaskSubmit(w http.ResponseWriter, r *http.Request) {
	var req mcp.TaskSubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		m.sendError(w, "INVALID_REQUEST", "Invalid request body")
		return
	}

	taskID := "task_" + generateID()
	m.tasks[taskID] = &mockTask{
		TaskID:       taskID,
		Status:       "pending",
		Goal:         req.Goal,
		Priority:     req.Priority,
		ResultBranch: "feature/" + taskID[:8],
		Progress:     0,
		Subtasks:     []mockSubtask{},
	}

	resp := mcp.APIResponse{
		OK: true,
		Data: json.RawMessage(`{
			"taskId": "` + taskID + `",
			"status": "pending",
			"resultBranch": "` + "feature/" + taskID[:8] + `",
			"createdAt": "2026-05-15T10:00:00Z",
			"estimatedDuration": 1800
		}`),
	}
	json.NewEncoder(w).Encode(resp)
}

func (m *mockPlatformAPI) handleGetTask(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Path[len("/api/v1/tasks/"):]
	task, ok := m.tasks[taskID]
	if !ok {
		m.sendError(w, "NOT_FOUND", "Task not found: "+taskID)
		return
	}

	subtasks := make([]mcp.SubtaskStatus, len(task.Subtasks))
	for i, st := range task.Subtasks {
		subtasks[i] = mcp.SubtaskStatus{
			SubtaskID:     st.SubtaskID,
			Type:          st.Type,
			AgentTemplate: st.AgentTemplate,
			Status:        st.Status,
		}
	}

	respData := map[string]interface{}{
		"taskId":       task.TaskID,
		"status":       task.Status,
		"goal":         task.Goal,
		"priority":     task.Priority,
		"resultBranch": task.ResultBranch,
		"progress":     task.Progress,
		"subtasks":     subtasks,
		"createdAt":    "2026-05-15T10:00:00Z",
	}
	respDataJSON, _ := json.Marshal(respData)

	resp := mcp.APIResponse{
		OK:   true,
		Data: respDataJSON,
	}
	json.NewEncoder(w).Encode(resp)
}

func (m *mockPlatformAPI) handleListTasks(w http.ResponseWriter, r *http.Request) {
	tasks := make([]mcp.TaskStatusResponse, 0, len(m.tasks))
	for _, t := range m.tasks {
		tasks = append(tasks, mcp.TaskStatusResponse{
			TaskID:   t.TaskID,
			Status:   t.Status,
			Goal:     t.Goal,
			Priority: t.Priority,
		})
	}

	respData := map[string]interface{}{
		"tasks":    tasks,
		"total":    len(tasks),
		"page":     1,
		"pageSize": 20,
	}
	respDataJSON, _ := json.Marshal(respData)

	resp := mcp.APIResponse{
		OK:   true,
		Data: respDataJSON,
	}
	json.NewEncoder(w).Encode(resp)
}

func (m *mockPlatformAPI) handleAgentTemplates(w http.ResponseWriter, r *http.Request) {
	respData, _ := json.Marshal(m.agentTemplates)
	resp := mcp.APIResponse{
		OK:   true,
		Data: respData,
	}
	json.NewEncoder(w).Encode(resp)
}

func (m *mockPlatformAPI) handleSessions(w http.ResponseWriter, r *http.Request) {
	respData, _ := json.Marshal(mcp.SessionResponse{
		Sessions: m.sessions,
		Total:    len(m.sessions),
	})
	resp := mcp.APIResponse{
		OK:   true,
		Data: respData,
	}
	json.NewEncoder(w).Encode(resp)
}

func (m *mockPlatformAPI) handleTaskCancel(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Path[len("/api/v1/tasks/"):]
	taskID = taskID[:len(taskID)-len("/cancel")]
	task, ok := m.tasks[taskID]
	if !ok {
		m.sendError(w, "NOT_FOUND", "Task not found: "+taskID)
		return
	}

	var req mcp.CancelTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		m.sendError(w, "INVALID_REQUEST", "Invalid request body")
		return
	}

	task.Status = "cancelled"
	resp := mcp.APIResponse{
		OK: true,
		Data: json.RawMessage(fmt.Sprintf(`{
			"taskId": "%s",
			"status": "cancelled",
			"terminatedAgents": []
		}`, taskID)),
	}
	json.NewEncoder(w).Encode(resp)
}

func (m *mockPlatformAPI) handleTaskDecompose(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Path[len("/api/v1/tasks/"):]
	taskID = taskID[:len(taskID)-len("/decompose")]
	task, ok := m.tasks[taskID]
	if !ok {
		m.sendError(w, "NOT_FOUND", "Task not found: "+taskID)
		return
	}

	subtaskIDs := []string{}
	for i := range 3 {
		subtaskID := fmt.Sprintf("subtask_%s_%d", taskID[:8], i)
		subtaskIDs = append(subtaskIDs, subtaskID)
		task.Subtasks = append(task.Subtasks, mockSubtask{
			SubtaskID:     subtaskID,
			Type:          "agent",
			AgentTemplate: "executor",
			Status:        "pending",
		})
	}

	resp := mcp.APIResponse{
		OK: true,
		Data: json.RawMessage(fmt.Sprintf(`{
			"taskId": "%s",
			"subtasks": %s
		}`, taskID, fmt.Sprintf(`["%s","%s","%s"]`, subtaskIDs[0], subtaskIDs[1], subtaskIDs[2]))),
	}
	json.NewEncoder(w).Encode(resp)
}

func (m *mockPlatformAPI) handleSubtaskDecision(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	parts := strings.Split(path, "/")
	// Path: /api/v1/tasks/{taskID}/subtasks/{subtaskID}/decision
	// Index:  0    1    2     3         4           5      6           7
	if len(parts) < 8 {
		m.sendError(w, "INVALID_REQUEST", "Invalid path format")
		return
	}
	taskID := parts[4]
	subtaskID := parts[6]

	var req mcp.DecideRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		m.sendError(w, "INVALID_REQUEST", "Invalid request body")
		return
	}

	task, ok := m.tasks[taskID]
	if !ok {
		m.sendError(w, "NOT_FOUND", "Task not found: "+taskID)
		return
	}

	found := false
	for i := range task.Subtasks {
		if task.Subtasks[i].SubtaskID == subtaskID {
			if req.Decision == "approve" {
				task.Subtasks[i].Status = "approved"
			} else {
				task.Subtasks[i].Status = "rejected"
			}
			found = true
			break
		}
	}

	if !found {
		m.sendError(w, "NOT_FOUND", "Subtask not found: "+subtaskID)
		return
	}

	resp := mcp.APIResponse{
		OK: true,
		Data: json.RawMessage(fmt.Sprintf(`{
			"taskId": "%s",
			"subtaskId": "%s",
			"status": "%s"
		}`, taskID, subtaskID, req.Decision)),
	}
	json.NewEncoder(w).Encode(resp)
}

func (m *mockPlatformAPI) sendError(w http.ResponseWriter, code, message string) {
	resp := mcp.APIResponse{
		OK: false,
		Error: &mcp.APIError{
			Code:    code,
			Message: message,
		},
	}
	json.NewEncoder(w).Encode(resp)
}

// generateID generates a simple unique ID for testing.
func generateID() string {
	return fmt.Sprintf("%d%07d", time.Now().UnixNano(), rand.Intn(10000000))
}

// mcpTestEnv holds the test environment for MCP E2E tests.
type mcpTestEnv struct {
	t          *testing.T
	logger     *zap.Logger
	mockServer *httptest.Server
	mcpServer  *mcp.Server
	executor   *mcp.ToolExecutor
	client     *mcp.PlatformClient
}

// setupMCPTestEnv creates a test environment with mock platform API and MCP server.
func setupMCPTestEnv(t *testing.T) *mcpTestEnv {
	logger := zaptest.NewLogger(t)

	// Create mock platform API server
	mockAPI := newMockPlatformAPI(t)
	mockServer := httptest.NewServer(mockAPI)
	t.Cleanup(mockServer.Close)

	// Create platform client pointing to mock server
	client := mcp.NewPlatformClient(mockServer.URL, "test-token", logger)

	// Create tool executor with the client
	executor := mcp.NewToolExecutor(client, logger)

	// Initialize MCP registry and executor
	mcp.InitRegistry(logger)
	mcp.InitExecutor(client, logger)

	// Create MCP server
	server := mcp.NewServer(executor, logger)

	return &mcpTestEnv{
		t:          t,
		logger:     logger,
		mockServer: mockServer,
		mcpServer:  server,
		executor:   executor,
		client:     client,
	}
}

// sendJSONRPCRequest sends a JSON-RPC request to the MCP server and returns the response.
func (env *mcpTestEnv) sendJSONRPCRequest(method string, params interface{}) *mcp.JSONRPCResponse {
	req := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
	}

	if params != nil {
		paramsJSON, err := json.Marshal(params)
		require.NoError(env.t, err)
		req.Params = paramsJSON
	}

	reqJSON, err := json.Marshal(req)
	require.NoError(env.t, err)

	resp, err := env.mcpServer.HandleMessage(reqJSON)
	require.NoError(env.t, err)

	return resp
}

// TestMCP_ToolsList tests that tools/list returns the 9 MCP tools.
func TestMCP_ToolsList(t *testing.T) {
	env := setupMCPTestEnv(t)

	resp := env.sendJSONRPCRequest("tools/list", nil)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Result)
	require.Nil(t, resp.Error, "tools/list should not return an error: %s", resp.Error)

	var result mcp.ToolsListResult
	err := json.Unmarshal(resp.Result, &result)
	require.NoError(t, err)

	// Verify we have 9 tools registered
	require.Len(t, result.Tools, 9, "Expected 9 MCP tools to be registered")

	// Verify tool names
	toolNames := make(map[string]bool)
	for _, tool := range result.Tools {
		toolNames[tool.Name] = true
		t.Logf("Tool: %s - %s", tool.Name, tool.Description)
	}

	expectedTools := []string{
		"task_submit",
		"task_status",
		"task_list",
		"task_cancel",
		"task_decompose",
		"context_approve",
		"context_reject",
		"agent_list",
		"session_list",
	}

	for _, name := range expectedTools {
		assert.True(t, toolNames[name], "Tool %s should be registered", name)
	}
}

// TestMCP_TaskSubmit tests the task_submit tool.
func TestMCP_TaskSubmit(t *testing.T) {
	env := setupMCPTestEnv(t)

	params := map[string]interface{}{
		"goal":       "Implement user authentication feature",
		"priority":    5,
		"tags":       []string{"auth", "security"},
		"constraints": []string{"Must use JWT", "Must support OAuth2"},
	}

	resp := env.sendJSONRPCRequest("tools/call", map[string]interface{}{
		"name":      "task_submit",
		"arguments": params,
	})

	require.NotNil(t, resp)
	require.Nil(t, resp.Error, "task_submit should not return an error: %s", resp.Error)
	require.NotNil(t, resp.Result)

	var result mcp.ToolCallResult
	err := json.Unmarshal(resp.Result, &result)
	require.NoError(t, err)

	// Verify result structure
	require.Len(t, result.Content, 1, "Expected 1 content block")
	require.Equal(t, "text", result.Content[0].Type, "Content type should be text")
	require.False(t, result.IsError, "Result should not be an error")

	t.Logf("Task submit result: %s", result.Content[0].Text)

	// Verify the response contains a taskId
	var taskResp mcp.TaskSubmitResponse
	err = json.Unmarshal([]byte(result.Content[0].Text), &taskResp)
	require.NoError(t, err)
	require.NotEmpty(t, taskResp.TaskID, "Task ID should not be empty")
	require.Equal(t, "pending", taskResp.Status, "Task should be in pending state")
}

// TestMCP_TaskStatus tests the task_status tool.
func TestMCP_TaskStatus(t *testing.T) {
	env := setupMCPTestEnv(t)

	// First submit a task
	submitResp := env.sendJSONRPCRequest("tools/call", map[string]interface{}{
		"name": "task_submit",
		"arguments": map[string]interface{}{
			"goal":     "Test task for status check",
			"priority": 3,
		},
	})
	require.NotNil(t, submitResp)

	var submitResult mcp.ToolCallResult
	err := json.Unmarshal(submitResp.Result, &submitResult)
	require.NoError(t, err)

	var taskResp mcp.TaskSubmitResponse
	err = json.Unmarshal([]byte(submitResult.Content[0].Text), &taskResp)
	require.NoError(t, err)
	taskID := taskResp.TaskID

	// Now check task status
	statusResp := env.sendJSONRPCRequest("tools/call", map[string]interface{}{
		"name": "task_status",
		"arguments": map[string]interface{}{
			"taskId": taskID,
		},
	})

	require.NotNil(t, statusResp)
	require.Nil(t, statusResp.Error, "task_status should not return an error: %s", statusResp.Error)
	require.NotNil(t, statusResp.Result)

	var statusResult mcp.ToolCallResult
	err = json.Unmarshal(statusResp.Result, &statusResult)
	require.NoError(t, err)

	require.Len(t, statusResult.Content, 1)
	require.Equal(t, "text", statusResult.Content[0].Type)

	var statusData mcp.TaskStatusResponse
	err = json.Unmarshal([]byte(statusResult.Content[0].Text), &statusData)
	require.NoError(t, err)
	require.Equal(t, taskID, statusData.TaskID)
	require.Equal(t, "pending", statusData.Status)

	t.Logf("Task status: %s", statusData.Status)
}

// TestMCP_TaskList tests the task_list tool.
func TestMCP_TaskList(t *testing.T) {
	env := setupMCPTestEnv(t)

	// Submit a few tasks first
	for i := 0; i < 3; i++ {
		resp := env.sendJSONRPCRequest("tools/call", map[string]interface{}{
			"name": "task_submit",
			"arguments": map[string]interface{}{
				"goal":     "Test task " + string(rune('A'+i)),
				"priority": i + 1,
			},
		})
		require.NotNil(t, resp)
	}

	// Now list tasks
	resp := env.sendJSONRPCRequest("tools/call", map[string]interface{}{
		"name": "task_list",
		"arguments": map[string]interface{}{
			"limit": 10,
		},
	})

	require.NotNil(t, resp)
	require.Nil(t, resp.Error, "task_list should not return an error: %s", resp.Error)
	require.NotNil(t, resp.Result)

	var result mcp.ToolCallResult
	err := json.Unmarshal(resp.Result, &result)
	require.NoError(t, err)

	require.Len(t, result.Content, 1)
	require.Equal(t, "text", result.Content[0].Type)

	var listData mcp.TaskListResponse
	err = json.Unmarshal([]byte(result.Content[0].Text), &listData)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(listData.Tasks), 3, "Should have at least 3 tasks")

	t.Logf("Listed %d tasks", len(listData.Tasks))
}

// TestMCP_AgentList tests the agent_list tool.
func TestMCP_AgentList(t *testing.T) {
	env := setupMCPTestEnv(t)

	resp := env.sendJSONRPCRequest("tools/call", map[string]interface{}{
		"name":      "agent_list",
		"arguments": map[string]interface{}{},
	})

	require.NotNil(t, resp)
	require.Nil(t, resp.Error, "agent_list should not return an error: %s", resp.Error)
	require.NotNil(t, resp.Result)

	var result mcp.ToolCallResult
	err := json.Unmarshal(resp.Result, &result)
	require.NoError(t, err)

	require.Len(t, result.Content, 1)
	require.Equal(t, "text", result.Content[0].Type)

	var agents []mcp.AgentTemplateResponse
	err = json.Unmarshal([]byte(result.Content[0].Text), &agents)
	require.NoError(t, err)
	require.Len(t, agents, 4, "Should have 4 agent templates")

	// Verify expected agents
	agentMap := make(map[string]bool)
	for _, a := range agents {
		agentMap[a.TemplateID] = true
		t.Logf("Agent: %s (%s) - capabilities: %v", a.TemplateID, a.Name, a.Capabilities)
	}

	assert.True(t, agentMap["executor"])
	assert.True(t, agentMap["strategist"])
	assert.True(t, agentMap["tester"])
	assert.True(t, agentMap["guardian"])
}

// TestMCP_Initialize tests the MCP initialize handshake.
func TestMCP_Initialize(t *testing.T) {
	env := setupMCPTestEnv(t)

	params := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "test-client",
			"version": "1.0.0",
		},
	}

	resp := env.sendJSONRPCRequest("initialize", params)
	require.NotNil(t, resp)
	require.Nil(t, resp.Error)
	require.NotNil(t, resp.Result)

	var result mcp.InitializeResult
	err := json.Unmarshal(resp.Result, &result)
	require.NoError(t, err)

	require.Equal(t, "2024-11-05", result.ProtocolVersion)
	require.NotNil(t, result.Capabilities.Tools, "Server should advertise tools capability")
	require.Equal(t, "cloud-agent-platform", result.ServerInfo.Name)
	require.Equal(t, "1.0.0", result.ServerInfo.Version)

	t.Logf("Initialized MCP server: %s v%s", result.ServerInfo.Name, result.ServerInfo.Version)
}

// TestMCP_InvalidToolCall tests that calling a non-existent tool returns an error.
func TestMCP_InvalidToolCall(t *testing.T) {
	env := setupMCPTestEnv(t)

	resp := env.sendJSONRPCRequest("tools/call", map[string]interface{}{
		"name":      "non_existent_tool",
		"arguments": map[string]interface{}{},
	})

	require.NotNil(t, resp)
	// Server returns IsError results as successful JSON-RPC responses
	require.NotNil(t, resp.Result, "Should return a result")
	var result mcp.ToolCallResult
	err := json.Unmarshal(resp.Result, &result)
	require.NoError(t, err)
	require.True(t, result.IsError, "Result should have IsError=true for non-existent tool")
}

// TestMCP_InvalidParams tests that invalid parameters return an error.
func TestMCP_InvalidParams(t *testing.T) {
	env := setupMCPTestEnv(t)

	// task_submit requires 'goal' parameter
	resp := env.sendJSONRPCRequest("tools/call", map[string]interface{}{
		"name": "task_submit",
		"arguments": map[string]interface{}{
			// Missing required 'goal' parameter
			"priority": 5,
		},
	})

	require.NotNil(t, resp)
	// Server returns IsError results as successful JSON-RPC responses
	require.NotNil(t, resp.Result, "Should return a result")
	var result mcp.ToolCallResult
	err := json.Unmarshal(resp.Result, &result)
	require.NoError(t, err)
	require.True(t, result.IsError, "Result should have IsError=true for missing required parameters")
}

// TestMCP_SessionList tests the session_list tool.
func TestMCP_SessionList(t *testing.T) {
	env := setupMCPTestEnv(t)

	resp := env.sendJSONRPCRequest("tools/call", map[string]interface{}{
		"name":      "session_list",
		"arguments": map[string]interface{}{},
	})

	require.NotNil(t, resp)
	require.Nil(t, resp.Error, "session_list should not return an error: %s", resp.Error)
	require.NotNil(t, resp.Result)

	var result mcp.ToolCallResult
	err := json.Unmarshal(resp.Result, &result)
	require.NoError(t, err)

	require.Len(t, result.Content, 1)
	require.Equal(t, "text", result.Content[0].Type)

	var sessions mcp.SessionResponse
	err = json.Unmarshal([]byte(result.Content[0].Text), &sessions)
	require.NoError(t, err)
	require.NotNil(t, sessions.Sessions, "Sessions should not be nil")

	t.Logf("Session list: total=%d", sessions.Total)
}

// TestMCP_TaskCancel tests the task_cancel tool.
func TestMCP_TaskCancel(t *testing.T) {
	env := setupMCPTestEnv(t)

	// First submit a task
	submitResp := env.sendJSONRPCRequest("tools/call", map[string]interface{}{
		"name": "task_submit",
		"arguments": map[string]interface{}{
			"goal":     "Task to cancel",
			"priority": 3,
		},
	})
	require.NotNil(t, submitResp)

	var submitResult mcp.ToolCallResult
	err := json.Unmarshal(submitResp.Result, &submitResult)
	require.NoError(t, err)

	var taskResp mcp.TaskSubmitResponse
	err = json.Unmarshal([]byte(submitResult.Content[0].Text), &taskResp)
	require.NoError(t, err)
	taskID := taskResp.TaskID

	// Now cancel the task
	cancelResp := env.sendJSONRPCRequest("tools/call", map[string]interface{}{
		"name": "task_cancel",
		"arguments": map[string]interface{}{
			"taskId": taskID,
			"reason": "No longer needed",
		},
	})

	require.NotNil(t, cancelResp)
	require.Nil(t, cancelResp.Error, "task_cancel should not return an error: %s", cancelResp.Error)
	require.NotNil(t, cancelResp.Result)

	var cancelResult mcp.ToolCallResult
	err = json.Unmarshal(cancelResp.Result, &cancelResult)
	require.NoError(t, err)

	require.Len(t, cancelResult.Content, 1)
	require.Equal(t, "text", cancelResult.Content[0].Type)

	var cancelData mcp.CancelTaskResponse
	err = json.Unmarshal([]byte(cancelResult.Content[0].Text), &cancelData)
	require.NoError(t, err)
	require.Equal(t, taskID, cancelData.TaskID)
	require.Equal(t, "cancelled", cancelData.Status)

	t.Logf("Task %s cancelled successfully", taskID)
}

// TestMCP_TaskDecompose tests the task_decompose tool.
func TestMCP_TaskDecompose(t *testing.T) {
	env := setupMCPTestEnv(t)

	// First submit a task
	submitResp := env.sendJSONRPCRequest("tools/call", map[string]interface{}{
		"name": "task_submit",
		"arguments": map[string]interface{}{
			"goal":     "Complex task to decompose",
			"priority": 5,
		},
	})
	require.NotNil(t, submitResp)

	var submitResult mcp.ToolCallResult
	err := json.Unmarshal(submitResp.Result, &submitResult)
	require.NoError(t, err)

	var taskResp mcp.TaskSubmitResponse
	err = json.Unmarshal([]byte(submitResult.Content[0].Text), &taskResp)
	require.NoError(t, err)
	taskID := taskResp.TaskID

	// Now decompose the task
	decomposeResp := env.sendJSONRPCRequest("tools/call", map[string]interface{}{
		"name": "task_decompose",
		"arguments": map[string]interface{}{
			"taskId": taskID,
		},
	})

	require.NotNil(t, decomposeResp)
	require.Nil(t, decomposeResp.Error, "task_decompose should not return an error: %s", decomposeResp.Error)
	require.NotNil(t, decomposeResp.Result)

	var decomposeResult mcp.ToolCallResult
	err = json.Unmarshal(decomposeResp.Result, &decomposeResult)
	require.NoError(t, err)

	require.Len(t, decomposeResult.Content, 1)
	require.Equal(t, "text", decomposeResult.Content[0].Type)

	var decomposeData mcp.DecomposeTaskResponse
	err = json.Unmarshal([]byte(decomposeResult.Content[0].Text), &decomposeData)
	require.NoError(t, err)
	require.Equal(t, taskID, decomposeData.TaskID)
	require.Len(t, decomposeData.Subtasks, 3, "Should decompose into 3 subtasks")

	t.Logf("Task %s decomposed into %d subtasks", taskID, len(decomposeData.Subtasks))
}

// TestMCP_ContextApprove tests the context_approve tool.
func TestMCP_ContextApprove(t *testing.T) {
	env := setupMCPTestEnv(t)

	// First submit a task
	submitResp := env.sendJSONRPCRequest("tools/call", map[string]interface{}{
		"name": "task_submit",
		"arguments": map[string]interface{}{
			"goal":     "Task for context approval",
			"priority": 5,
		},
	})
	require.NotNil(t, submitResp)

	var submitResult mcp.ToolCallResult
	err := json.Unmarshal(submitResp.Result, &submitResult)
	require.NoError(t, err)

	var taskResp mcp.TaskSubmitResponse
	err = json.Unmarshal([]byte(submitResult.Content[0].Text), &taskResp)
	require.NoError(t, err)
	taskID := taskResp.TaskID

	// Decompose to get subtasks
	decomposeResp := env.sendJSONRPCRequest("tools/call", map[string]interface{}{
		"name": "task_decompose",
		"arguments": map[string]interface{}{
			"taskId": taskID,
		},
	})
	require.NotNil(t, decomposeResp)

	var decompResult mcp.ToolCallResult
	err = json.Unmarshal(decomposeResp.Result, &decompResult)
	require.NoError(t, err)

	var decompData mcp.DecomposeTaskResponse
	err = json.Unmarshal([]byte(decompResult.Content[0].Text), &decompData)
	require.NoError(t, err)
	subtaskID := decompData.Subtasks[0]

	// Approve the subtask
	approveResp := env.sendJSONRPCRequest("tools/call", map[string]interface{}{
		"name": "context_approve",
		"arguments": map[string]interface{}{
			"taskId":    taskID,
			"subtaskId": subtaskID,
			"feedback":  "Looks good, proceed",
		},
	})

	require.NotNil(t, approveResp)
	require.Nil(t, approveResp.Error, "context_approve should not return an error: %s", approveResp.Error)
	require.NotNil(t, approveResp.Result)

	var approveResult mcp.ToolCallResult
	err = json.Unmarshal(approveResp.Result, &approveResult)
	require.NoError(t, err)

	require.Len(t, approveResult.Content, 1)
	require.Equal(t, "text", approveResult.Content[0].Type)

	var decideData mcp.DecideResponse
	err = json.Unmarshal([]byte(approveResult.Content[0].Text), &decideData)
	require.NoError(t, err)
	require.Equal(t, taskID, decideData.TaskID)
	require.Equal(t, subtaskID, decideData.SubtaskID)

	t.Logf("Subtask %s approved for task %s", subtaskID, taskID)
}

// TestMCP_ContextReject tests the context_reject tool.
func TestMCP_ContextReject(t *testing.T) {
	env := setupMCPTestEnv(t)

	// First submit a task
	submitResp := env.sendJSONRPCRequest("tools/call", map[string]interface{}{
		"name": "task_submit",
		"arguments": map[string]interface{}{
			"goal":     "Task for context rejection",
			"priority": 5,
		},
	})
	require.NotNil(t, submitResp)

	var submitResult mcp.ToolCallResult
	err := json.Unmarshal(submitResp.Result, &submitResult)
	require.NoError(t, err)

	var taskResp mcp.TaskSubmitResponse
	err = json.Unmarshal([]byte(submitResult.Content[0].Text), &taskResp)
	require.NoError(t, err)
	taskID := taskResp.TaskID

	// Decompose to get subtasks
	decomposeResp := env.sendJSONRPCRequest("tools/call", map[string]interface{}{
		"name": "task_decompose",
		"arguments": map[string]interface{}{
			"taskId": taskID,
		},
	})
	require.NotNil(t, decomposeResp)

	var decompResult mcp.ToolCallResult
	err = json.Unmarshal(decomposeResp.Result, &decompResult)
	require.NoError(t, err)

	var decompData mcp.DecomposeTaskResponse
	err = json.Unmarshal([]byte(decompResult.Content[0].Text), &decompData)
	require.NoError(t, err)
	subtaskID := decompData.Subtasks[1]

	// Reject the subtask
	rejectResp := env.sendJSONRPCRequest("tools/call", map[string]interface{}{
		"name": "context_reject",
		"arguments": map[string]interface{}{
			"taskId":    taskID,
			"subtaskId": subtaskID,
			"feedback":  "Needs revision",
		},
	})

	require.NotNil(t, rejectResp)
	require.Nil(t, rejectResp.Error, "context_reject should not return an error: %s", rejectResp.Error)
	require.NotNil(t, rejectResp.Result)

	var rejectResult mcp.ToolCallResult
	err = json.Unmarshal(rejectResp.Result, &rejectResult)
	require.NoError(t, err)

	require.Len(t, rejectResult.Content, 1)
	require.Equal(t, "text", rejectResult.Content[0].Type)

	var decideData mcp.DecideResponse
	err = json.Unmarshal([]byte(rejectResult.Content[0].Text), &decideData)
	require.NoError(t, err)
	require.Equal(t, taskID, decideData.TaskID)
	require.Equal(t, subtaskID, decideData.SubtaskID)

	t.Logf("Subtask %s rejected for task %s", subtaskID, taskID)
}

// TestMCP_FullWorkflow tests the complete MCP tool workflow: tools/list, task_submit, task_status, task_list, task_cancel.
func TestMCP_FullWorkflow(t *testing.T) {
	env := setupMCPTestEnv(t)

	// Step 1: List tools and verify all 9 are registered
	t.Log("Step 1: Verifying tools/list returns 9 tools")
	listResp := env.sendJSONRPCRequest("tools/list", nil)
	require.NotNil(t, listResp)
	require.Nil(t, listResp.Error)

	var listResult mcp.ToolsListResult
	err := json.Unmarshal(listResp.Result, &listResult)
	require.NoError(t, err)
	require.Len(t, listResult.Tools, 9, "Expected 9 MCP tools")

	// Step 2: Submit a task
	t.Log("Step 2: Submitting a task")
	submitResp := env.sendJSONRPCRequest("tools/call", map[string]interface{}{
		"name": "task_submit",
		"arguments": map[string]interface{}{
			"goal":       "Implement user authentication",
			"priority":   8,
			"tags":       []string{"auth", "security"},
			"constraints": []string{"Must use JWT"},
		},
	})
	require.NotNil(t, submitResp)
	require.Nil(t, submitResp.Error)

	var submitResult mcp.ToolCallResult
	err = json.Unmarshal(submitResp.Result, &submitResult)
	require.NoError(t, err)

	var taskResp mcp.TaskSubmitResponse
	err = json.Unmarshal([]byte(submitResult.Content[0].Text), &taskResp)
	require.NoError(t, err)
	require.NotEmpty(t, taskResp.TaskID)
	taskID := taskResp.TaskID
	t.Logf("   Submitted task: %s", taskID)

	// Step 3: Check task status
	t.Log("Step 3: Checking task status")
	statusResp := env.sendJSONRPCRequest("tools/call", map[string]interface{}{
		"name": "task_status",
		"arguments": map[string]interface{}{
			"taskId": taskID,
		},
	})
	require.NotNil(t, statusResp)
	require.Nil(t, statusResp.Error)

	var statusResult mcp.ToolCallResult
	err = json.Unmarshal(statusResp.Result, &statusResult)
	require.NoError(t, err)

	var statusData mcp.TaskStatusResponse
	err = json.Unmarshal([]byte(statusResult.Content[0].Text), &statusData)
	require.NoError(t, err)
	require.Equal(t, taskID, statusData.TaskID)
	require.Equal(t, "pending", statusData.Status)
	t.Logf("   Task status: %s", statusData.Status)

	// Step 4: List tasks
	t.Log("Step 4: Listing tasks")
	listTasksResp := env.sendJSONRPCRequest("tools/call", map[string]interface{}{
		"name": "task_list",
		"arguments": map[string]interface{}{
			"limit": 10,
		},
	})
	require.NotNil(t, listTasksResp)
	require.Nil(t, listTasksResp.Error)

	var listTasksResult mcp.ToolCallResult
	err = json.Unmarshal(listTasksResp.Result, &listTasksResult)
	require.NoError(t, err)

	var listTasksData mcp.TaskListResponse
	err = json.Unmarshal([]byte(listTasksResult.Content[0].Text), &listTasksData)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(listTasksData.Tasks), 1)
	t.Logf("   Found %d tasks", len(listTasksData.Tasks))

	// Step 5: Cancel the task
	t.Log("Step 5: Cancelling task")
	cancelResp := env.sendJSONRPCRequest("tools/call", map[string]interface{}{
		"name": "task_cancel",
		"arguments": map[string]interface{}{
			"taskId": taskID,
			"reason": "Test complete",
		},
	})
	require.NotNil(t, cancelResp)
	require.Nil(t, cancelResp.Error)

	var cancelResult mcp.ToolCallResult
	err = json.Unmarshal(cancelResp.Result, &cancelResult)
	require.NoError(t, err)

	var cancelData mcp.CancelTaskResponse
	err = json.Unmarshal([]byte(cancelResult.Content[0].Text), &cancelData)
	require.NoError(t, err)
	require.Equal(t, taskID, cancelData.TaskID)
	require.Equal(t, "cancelled", cancelData.Status)
	t.Logf("   Task cancelled: %s", cancelData.Status)

	t.Log("Full MCP workflow completed successfully")
}

// TestMCP_AgentList_Filter tests the agent_list tool with capability filter.
func TestMCP_AgentList_Filter(t *testing.T) {
	env := setupMCPTestEnv(t)

	// Filter by 'coding' capability
	resp := env.sendJSONRPCRequest("tools/call", map[string]interface{}{
		"name": "agent_list",
		"arguments": map[string]interface{}{
			"capability": "coding",
		},
	})

	require.NotNil(t, resp)
	require.Nil(t, resp.Error, "agent_list should not return an error: %s", resp.Error)
	require.NotNil(t, resp.Result)

	var result mcp.ToolCallResult
	err := json.Unmarshal(resp.Result, &result)
	require.NoError(t, err)

	require.Len(t, result.Content, 1)
	require.Equal(t, "text", result.Content[0].Type)

	var agents []mcp.AgentTemplateResponse
	err = json.Unmarshal([]byte(result.Content[0].Text), &agents)
	require.NoError(t, err)
	// Only executor and strategist should have 'coding' capability based on test data
	for _, a := range agents {
		capVal, ok := a.Capabilities["coding"]
		require.True(t, ok && capVal > 0, "Agent %s should have coding capability", a.TemplateID)
	}

	t.Logf("Filtered agent list by 'coding': %d agents", len(agents))
}