// Package e2e provides end-to-end tests for the Cloud Agent Platform MCP integration.
package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cloud-agent-platform/cap/internal/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// mockPlatformHandler simulates the Cloud Agent Platform REST API.
type mockPlatformHandler struct {
	tasks       map[string]*mcp.TaskStatusResponse
	taskOrder   []string
	agentList   []mcp.AgentTemplateResponse
calledMethods []string
}

func newMockPlatformHandler() *mockPlatformHandler {
	return &mockPlatformHandler{
		tasks:       make(map[string]*mcp.TaskStatusResponse),
		taskOrder:   []string{},
		agentList: []mcp.AgentTemplateResponse{
			{
				TemplateID:        "coding-agent",
				Name:              "Coding Agent",
				Description:       "General purpose coding agent",
				Capabilities:      map[string]int{"coding": 9, "review": 7, "testing": 6},
				AvailableModels:   []string{"claude-4", "claude-3-5"},
				MaxConcurrent:     3,
				AvgCompletionTime: 300,
				SuccessRate:       0.92,
			},
			{
				TemplateID:        "review-agent",
				Name:              "Review Agent",
				Description:       "Code review specialist",
				Capabilities:      map[string]int{"review": 9, "coding": 4},
				AvailableModels:   []string{"claude-4"},
				MaxConcurrent:     5,
				AvgCompletionTime: 120,
				SuccessRate:       0.95,
			},
		},
		calledMethods: []string{},
	}
}

func (h *mockPlatformHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.calledMethods = append(h.calledMethods, r.Method+" "+r.URL.Path)
	w.Header().Set("Content-Type", "application/json")

	var resp mcp.APIResponse
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/tasks":
		// Task submit
		var req mcp.TaskSubmitRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			resp = mcp.APIResponse{OK: false, Error: &mcp.APIError{Code: "INVALID_REQUEST", Message: err.Error()}}
		} else {
			taskID := "task_" + generateID()
			h.tasks[taskID] = &mcp.TaskStatusResponse{
				TaskID:       taskID,
				Status:       "pending",
				Goal:         req.Goal,
				Priority:     req.Priority,
				ResultBranch: "feature/" + taskID,
				Progress:     0,
				CreatedAt:    "2026-05-15T10:00:00Z",
			}
			h.taskOrder = append(h.taskOrder, taskID)
			resp = mcp.APIResponse{OK: true, Data: marshalJSON(mcp.TaskSubmitResponse{
				TaskID:       taskID,
				Status:       "pending",
				ResultBranch: "feature/" + taskID,
				CreatedAt:    "2026-05-15T10:00:00Z",
			})}
		}

	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/tasks":
		// Task list
		tasks := make([]mcp.TaskStatusResponse, 0, len(h.taskOrder))
		for _, id := range h.taskOrder {
			tasks = append(tasks, *h.tasks[id])
		}
		resp = mcp.APIResponse{OK: true, Data: marshalJSON(mcp.TaskListResponse{
			Tasks:    tasks,
			Total:    len(tasks),
			Page:     1,
			PageSize: 20,
		})}

	case r.Method == http.MethodGet && len(r.URL.Path) > 14 && r.URL.Path[:14] == "/api/v1/tasks/":
		// Task status (GET /api/v1/tasks/:taskId)
		taskID := r.URL.Path[14:]
		if task, ok := h.tasks[taskID]; ok {
			resp = mcp.APIResponse{OK: true, Data: marshalJSON(task)}
		} else {
			resp = mcp.APIResponse{OK: false, Error: &mcp.APIError{Code: "NOT_FOUND", Message: "Task not found"}}
		}

	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/agent-templates":
		// Agent templates
		resp = mcp.APIResponse{OK: true, Data: marshalJSON(h.agentList)}

	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/platform/status":
		resp = mcp.APIResponse{OK: true, Data: marshalJSON(mcp.PlatformStatusResponse{
			Pool: struct {
				Total       int `json:"total"`
				Idle        int `json:"idle"`
				Busy        int `json:"busy"`
				MaxCapacity int `json:"maxCapacity"`
			}{Total: 10, Idle: 7, Busy: 3, MaxCapacity: 20},
			Queue: struct {
				Pending    int     `json:"pending"`
				AvgWaitTime float64 `json:"avgWaitTime"`
			}{Pending: 2, AvgWaitTime: 1.5},
			Models: []mcp.ModelStatus{
				{ModelID: "claude-4", Available: true, AvgLatency: 800},
			},
			Uptime: 3600,
		})}

	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/sessions":
		resp = mcp.APIResponse{OK: true, Data: marshalJSON(mcp.SessionResponse{
			Sessions: []mcp.Session{
				{SessionID: "sess_001", ClientID: "client_001", Status: "active", CreatedAt: "2026-05-15T09:00:00Z", UpdatedAt: "2026-05-15T10:00:00Z"},
			},
			Total: 1,
		})}

	default:
		resp = mcp.APIResponse{OK: false, Error: &mcp.APIError{Code: "NOT_FOUND", Message: "Endpoint not found"}}
	}

	_ = json.NewEncoder(w).Encode(resp)
}

func marshalJSON(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}

func generateID() string {
	return "e2e_test_12345"
}

// TestMCPEndToEnd tests the complete MCP protocol flow simulating Claude Code as MCP Client.
func TestMCPEndToEnd(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// Step 1: Set up mock platform API server
	mockServer := httptest.NewServer(newMockPlatformHandler())
	defer mockServer.Close()

	// Step 2: Create platform client connected to mock server
	platformClient := mcp.NewPlatformClient(mockServer.URL, "test-token", logger)

	// Step 3: Create MCP server with tool executor
	executor := mcp.NewToolExecutor(platformClient, logger)
	server := mcp.NewServer(executor, logger)

	// Step 4: Simulate Claude Code (MCP Client) interaction

	// 4a. Send initialize request
	initReq := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "initialize",
		Params:  marshalJSON(mcp.InitializeParams{ProtocolVersion: "2024-11-05", ClientInfo: mcp.ClientInfo{Name: "Claude Code", Version: "1.0"}}),
		ID:      marshalJSON(1),
	}
	initData, _ := json.Marshal(initReq)
	resp, err := server.HandleMessage(initData)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Empty(t, resp.Error)

	var initResult mcp.InitializeResult
	err = json.Unmarshal(resp.Result, &initResult)
	require.NoError(t, err)
	assert.Equal(t, "2024-11-05", initResult.ProtocolVersion)
	assert.NotNil(t, initResult.Capabilities.Tools)
	assert.Equal(t, "cloud-agent-platform", initResult.ServerInfo.Name)

	// 4b. Send notifications/initialized (client done initializing)
	notifReq := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	notifData, _ := json.Marshal(notifReq)
	_, err = server.HandleMessage(notifData)
	require.NoError(t, err)

	// 4c. tools/list - discover available tools
	toolsListReq := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/list",
		ID:      marshalJSON(2),
	}
	toolsListData, _ := json.Marshal(toolsListReq)
	resp, err = server.HandleMessage(toolsListData)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Empty(t, resp.Error)

	var toolsResult mcp.ToolsListResult
	err = json.Unmarshal(resp.Result, &toolsResult)
	require.NoError(t, err)
	assert.NotEmpty(t, toolsResult.Tools, "should return tools")

	// Find the tools we need
	toolNames := make(map[string]bool)
	for _, tool := range toolsResult.Tools {
		toolNames[tool.Name] = true
	}
	assert.True(t, toolNames["task_submit"], "task_submit tool should be available")
	assert.True(t, toolNames["task_status"], "task_status tool should be available")
	assert.True(t, toolNames["task_list"], "task_list tool should be available")
	assert.True(t, toolNames["agent_list"] || toolNames["agent_templates"], "agent tool should be available")

	// 4d. task_submit - submit a task
	submitReq := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params: marshalJSON(mcp.ToolCallParams{
			Name: "task_submit",
			Arguments: marshalJSON(map[string]any{
				"goal":      "Implement user authentication",
				"priority":  7,
				"tags":      []string{"auth", "security"},
				"repository": map[string]string{"url": "https://github.com/example/repo", "branch": "main"},
			}),
		}),
		ID: marshalJSON(3),
	}
	submitData, _ := json.Marshal(submitReq)
	resp, err = server.HandleMessage(submitData)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Empty(t, resp.Error)

	var submitResult mcp.ToolCallResult
	err = json.Unmarshal(resp.Result, &submitResult)
	require.NoError(t, err)
	assert.False(t, submitResult.IsError, "task_submit should not be an error")
	require.NotEmpty(t, submitResult.Content)

	var submitResp mcp.TaskSubmitResponse
	err = json.Unmarshal([]byte(submitResult.Content[0].Text), &submitResp)
	require.NoError(t, err)
	assert.NotEmpty(t, submitResp.TaskID)
	assert.Equal(t, "pending", submitResp.Status)

	// 4e. task_status - query task status
	statusReq := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params: marshalJSON(mcp.ToolCallParams{
			Name:      "task_status",
			Arguments: marshalJSON(map[string]any{"taskId": submitResp.TaskID}),
		}),
		ID: marshalJSON(4),
	}
	statusData, _ := json.Marshal(statusReq)
	resp, err = server.HandleMessage(statusData)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Empty(t, resp.Error)

	var statusResult mcp.ToolCallResult
	err = json.Unmarshal(resp.Result, &statusResult)
	require.NoError(t, err)
	assert.False(t, statusResult.IsError)

	var statusResp mcp.TaskStatusResponse
	err = json.Unmarshal([]byte(statusResult.Content[0].Text), &statusResp)
	require.NoError(t, err)
	assert.Equal(t, submitResp.TaskID, statusResp.TaskID)

	// 4f. task_list - list all tasks
	listReq := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params: marshalJSON(mcp.ToolCallParams{
			Name:      "task_list",
			Arguments: marshalJSON(map[string]any{"limit": 10}),
		}),
		ID: marshalJSON(5),
	}
	listData, _ := json.Marshal(listReq)
	resp, err = server.HandleMessage(listData)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Empty(t, resp.Error)

	var listResult mcp.ToolCallResult
	err = json.Unmarshal(resp.Result, &listResult)
	require.NoError(t, err)
	assert.False(t, listResult.IsError)

	var listResp mcp.TaskListResponse
	err = json.Unmarshal([]byte(listResult.Content[0].Text), &listResp)
	require.NoError(t, err)
	assert.NotEmpty(t, listResp.Tasks)
	assert.GreaterOrEqual(t, listResp.Total, 1)

	// 4g. agent_list / agent_templates - list agents
	agentReq := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params: marshalJSON(mcp.ToolCallParams{
			Name:      "agent_templates",
			Arguments: marshalJSON(map[string]any{}),
		}),
		ID: marshalJSON(6),
	}
	agentData, _ := json.Marshal(agentReq)
	resp, err = server.HandleMessage(agentData)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Empty(t, resp.Error)

	var agentResult mcp.ToolCallResult
	err = json.Unmarshal(resp.Result, &agentResult)
	require.NoError(t, err)
	assert.False(t, agentResult.IsError)

	var agentResp []mcp.AgentTemplateResponse
	err = json.Unmarshal([]byte(agentResult.Content[0].Text), &agentResp)
	require.NoError(t, err)
	assert.NotEmpty(t, agentResp)

	// 4h. session_list - list sessions
	sessionReq := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params: marshalJSON(mcp.ToolCallParams{
			Name:      "session_list",
			Arguments: marshalJSON(map[string]any{}),
		}),
		ID: marshalJSON(7),
	}
	sessionData, _ := json.Marshal(sessionReq)
	resp, err = server.HandleMessage(sessionData)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Empty(t, resp.Error)

	var sessionResult mcp.ToolCallResult
	err = json.Unmarshal(resp.Result, &sessionResult)
	require.NoError(t, err)
	assert.False(t, sessionResult.IsError)

	var sessionResp mcp.SessionResponse
	err = json.Unmarshal([]byte(sessionResult.Content[0].Text), &sessionResp)
	require.NoError(t, err)
	assert.NotEmpty(t, sessionResp.Sessions)

	t.Log("MCP End-to-End test passed: Claude Code successfully interacted with platform via MCP protocol")
}

// TestMCPProtocolErrors tests MCP protocol error handling.
func TestMCPProtocolErrors(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	mockServer := httptest.NewServer(newMockPlatformHandler())
	defer mockServer.Close()

	platformClient := mcp.NewPlatformClient(mockServer.URL, "test-token", logger)
	executor := mcp.NewToolExecutor(platformClient, logger)
	server := mcp.NewServer(executor, logger)

	// Test invalid JSON
	invalidReq := []byte(`{invalid json}`)
	resp, err := server.HandleMessage(invalidReq)
	// HandleMessage returns both error response and error for parse errors
	require.NotNil(t, resp)
	require.NotNil(t, resp.Error)
	assert.Equal(t, int64(mcp.CodeParseError), resp.Error.Code)

	// Test unknown method
	unknownReq := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "unknown/method",
		ID:      marshalJSON(1),
	}
	unknownData, _ := json.Marshal(unknownReq)
	resp, err = server.HandleMessage(unknownData)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Error)
	assert.Equal(t, int64(mcp.CodeMethodNotFound), resp.Error.Code)

	// Test tools/call with empty tool name
	emptyToolReq := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params: marshalJSON(mcp.ToolCallParams{
			Name:      "",
			Arguments: marshalJSON(map[string]any{}),
		}),
		ID: marshalJSON(2),
	}
	emptyToolData, _ := json.Marshal(emptyToolReq)
	resp, err = server.HandleMessage(emptyToolData)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Error)
	assert.Equal(t, int64(mcp.CodeInvalidParams), resp.Error.Code)
}

// TestMCPEndToEnd_WithMCPClient tests using the full MCP client flow.
func TestMCPEndToEnd_WithMCPClient(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	mockServer := httptest.NewServer(newMockPlatformHandler())
	defer mockServer.Close()

	// Create MCP client connected to the mock server
	client := mcp.NewPlatformClient(mockServer.URL, "test-token", logger)

	// Test that client can call platform API directly
	ctx := context.Background()

	// Submit a task
	submitResp, err := client.SubmitTask(ctx, mcp.TaskSubmitRequest{
		Goal:     "Test MCP client submission",
		Priority: 5,
		Tags:     []string{"test", "mcp"},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, submitResp.TaskID)

	// Get task status
	statusResp, err := client.GetTask(ctx, submitResp.TaskID)
	require.NoError(t, err)
	assert.Equal(t, submitResp.TaskID, statusResp.TaskID)

	// List tasks
	listResp, err := client.ListTasks(ctx, "", nil, 20)
	require.NoError(t, err)
	assert.NotEmpty(t, listResp.Tasks)

	// List agent templates
	agentResp, err := client.ListAgentTemplates(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, agentResp)

	// Get platform status
	status, err := client.GetPlatformStatus(ctx)
	require.NoError(t, err)
	assert.Equal(t, 10, status.Pool.Total)

	// List sessions
	sessionResp, err := client.ListSessions(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, sessionResp.Sessions)
}

// TestMCPResourceLifecycle tests MCP resources through the protocol.
func TestMCPResourceLifecycle(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	mockServer := httptest.NewServer(newMockPlatformHandler())
	defer mockServer.Close()

	platformClient := mcp.NewPlatformClient(mockServer.URL, "test-token", logger)
	executor := mcp.NewToolExecutor(platformClient, logger)
	server := mcp.NewServer(executor, logger)

	// Initialize first
	initReq := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "initialize",
		Params:  marshalJSON(mcp.InitializeParams{ProtocolVersion: "2024-11-05", ClientInfo: mcp.ClientInfo{Name: "Test", Version: "1.0"}}),
		ID:      marshalJSON(1),
	}
	initData, _ := json.Marshal(initReq)
	resp, err := server.HandleMessage(initData)
	require.NoError(t, err)
	require.Nil(t, resp.Error)

	// resources/list
	resListReq := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "resources/list",
		ID:      marshalJSON(2),
	}
	resListData, _ := json.Marshal(resListReq)
	resp, err = server.HandleMessage(resListData)
	require.NoError(t, err)
	require.Nil(t, resp.Error)

	var resourcesResult mcp.ResourcesListResult
	err = json.Unmarshal(resp.Result, &resourcesResult)
	require.NoError(t, err)
	// Resources may be empty if registry is not initialized, which is fine for this test
	assert.NotNil(t, resourcesResult.Resources)
}

// BenchmarkMCPEndToEnd benchmarks the MCP full chain performance.
func BenchmarkMCPEndToEnd(b *testing.B) {
	logger, _ := zap.NewDevelopment()

	mockServer := httptest.NewServer(newMockPlatformHandler())
	defer mockServer.Close()

	platformClient := mcp.NewPlatformClient(mockServer.URL, "test-token", logger)
	executor := mcp.NewToolExecutor(platformClient, logger)
	server := mcp.NewServer(executor, logger)

	// Initialize
	initReq := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "initialize",
		Params:  marshalJSON(mcp.InitializeParams{ProtocolVersion: "2024-11-05", ClientInfo: mcp.ClientInfo{Name: "Benchmark", Version: "1.0"}}),
		ID:      marshalJSON(1),
	}
	initData, _ := json.Marshal(initReq)
	_, _ = server.HandleMessage(initData)

	// Notification
	notifReq := mcp.JSONRPCRequest{JSONRPC: "2.0", Method: "notifications/initialized"}
	notifData, _ := json.Marshal(notifReq)
	_, _ = server.HandleMessage(notifData)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// tools/list
		toolsReq := mcp.JSONRPCRequest{JSONRPC: "2.0", Method: "tools/list", ID: marshalJSON(int64(i))}
		toolsData, _ := json.Marshal(toolsReq)
		resp, _ := server.HandleMessage(toolsData)
		if resp != nil && resp.Result != nil {
			var toolsResult mcp.ToolsListResult
			_ = json.Unmarshal(resp.Result, &toolsResult)
		}
	}
}
