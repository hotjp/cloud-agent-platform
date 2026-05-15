// Package e2e provides end-to-end integration tests for the Cloud Agent Platform.
// These tests verify the complete flow: MCP submit -> HTTP API -> TaskService -> Orchestrator -> Mock Agent execution.
// Uses httptest for the HTTP layer and in-memory mocks for storage.
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cloud-agent-platform/cap/internal/domain"
	"github.com/cloud-agent-platform/cap/internal/mcp"
	"github.com/cloud-agent-platform/cap/internal/orchestrator"
	"github.com/cloud-agent-platform/cap/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// ----------------------------------------------------------------------------
// Integration Test HTTP Server
// ----------------------------------------------------------------------------

// integrationHTTPServer is an httptest.Server that handles the REST API endpoints
// expected by the MCP PlatformClient, delegating to the actual TaskService.
type integrationHTTPServer struct {
	taskSvc  *service.TaskService
	orch     *orchestrator.OrchestratorImpl
	taskRepo *inMemoryTaskRepo
}

// newIntegrationHTTPServer creates an httptest.Server with REST API handlers.
func newIntegrationHTTPServer(t *testing.T, taskSvc *service.TaskService, orch *orchestrator.OrchestratorImpl, taskRepo *inMemoryTaskRepo) *httptest.Server {
	server := &integrationHTTPServer{
		taskSvc:  taskSvc,
		orch:     orch,
		taskRepo: taskRepo,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/tasks", server.handleTasks)
	mux.HandleFunc("/api/v1/tasks/", server.handleTaskByID)
	mux.HandleFunc("/api/v1/agent-templates", server.handleAgentTemplates)
	mux.HandleFunc("/api/v1/sessions", server.handleSessions)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	return httptest.NewServer(mux)
}

func (s *integrationHTTPServer) serveJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (s *integrationHTTPServer) serveError(w http.ResponseWriter, code, message string) {
	resp := mcp.APIResponse{
		OK: false,
		Error: &mcp.APIError{
			Code:    code,
			Message: message,
		},
	}
	s.serveJSON(w, resp)
}

func (s *integrationHTTPServer) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handleTaskSubmit(w, r)
	case http.MethodGet:
		s.handleTaskList(w, r)
	default:
		s.serveError(w, "METHOD_NOT_ALLOWED", "Only GET and POST are supported")
	}
}

func (s *integrationHTTPServer) handleTaskList(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters for pagination
	limit := 20
	offset := 0

	if err := r.ParseForm(); err == nil {
		if pageSizeStr := r.Form.Get("pageSize"); pageSizeStr != "" {
			fmt.Sscanf(pageSizeStr, "%d", &limit)
		}
		if page := r.Form.Get("page"); page != "" {
			pageNum := 0
			fmt.Sscanf(page, "%d", &pageNum)
			offset = (pageNum - 1) * limit
		}
	}

	resp, err := s.taskSvc.List(context.Background(), service.ListRequest{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		s.serveError(w, "LIST_FAILED", err.Error())
		return
	}

	// Convert to mcp.TaskStatusResponse
	tasks := make([]mcp.TaskStatusResponse, len(resp.Tasks))
	for i, task := range resp.Tasks {
		tasks[i] = mcp.TaskStatusResponse{
			TaskID:       task.ID,
			Status:       string(task.Status),
			Goal:         task.Goal,
			Priority:     task.Priority,
			ResultBranch: task.ResultBranch,
			Progress:     int(task.Progress),
			CreatedAt:    task.CreatedAt.Format(time.RFC3339),
		}
	}

	result := mcp.TaskListResponse{
		Tasks:    tasks,
		Total:    resp.Total,
		Page:     1,
		PageSize: limit,
	}

	apiResp := mcp.APIResponse{
		OK:   true,
		Data: must(json.Marshal(result)),
	}
	s.serveJSON(w, apiResp)
}

func (s *integrationHTTPServer) handleTaskByID(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if strings.HasSuffix(path, "/cancel") {
		s.handleTaskCancel(w, r, path)
		return
	}
	if strings.HasSuffix(path, "/decompose") {
		s.handleTaskDecompose(w, r, path)
		return
	}

	// GET /api/v1/tasks/:id
	taskID := strings.TrimPrefix(path, "/api/v1/tasks/")
	s.handleGetTask(w, r, taskID)
}

func (s *integrationHTTPServer) handleTaskSubmit(w http.ResponseWriter, r *http.Request) {
	var req mcp.TaskSubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.serveError(w, "INVALID_REQUEST", "Invalid request body")
		return
	}

	// Convert to service.SubmitRequest
	submitReq := service.SubmitRequest{
		Goal:          req.Goal,
		RepositoryURL: "",
		BaseBranch:    "main",
		ClientID:      "mcp-client",
		Priority:      req.Priority,
		Constraints:   req.Constraints,
		VerificationCriteria: req.VerificationCriteria,
		Tags:         req.Tags,
	}
	if req.Repository != nil {
		submitReq.RepositoryURL = req.Repository.URL
		submitReq.BaseBranch = req.Repository.Branch
	}

	resp, err := s.taskSvc.Submit(context.Background(), submitReq)
	if err != nil {
		s.serveError(w, "TASK_SUBMIT_FAILED", err.Error())
		return
	}

	// Build response matching mcp.TaskSubmitResponse
	result := mcp.TaskSubmitResponse{
		TaskID:       resp.TaskID,
		Status:       string(resp.Task.Status),
		ResultBranch: resp.Task.ResultBranch,
		CreatedAt:    resp.Task.CreatedAt.Format(time.RFC3339),
	}

	apiResp := mcp.APIResponse{
		OK:   true,
		Data: must(json.Marshal(result)),
	}
	s.serveJSON(w, apiResp)
}

func (s *integrationHTTPServer) handleGetTask(w http.ResponseWriter, r *http.Request, taskID string) {
	resp, err := s.taskSvc.Get(context.Background(), service.GetRequest{TaskID: taskID})
	if err != nil {
		s.serveError(w, "NOT_FOUND", "Task not found: "+taskID)
		return
	}

	task := resp.Task
	result := mcp.TaskStatusResponse{
		TaskID:       task.ID,
		Status:       string(task.Status),
		Goal:         task.Goal,
		Priority:     task.Priority,
		ResultBranch: task.ResultBranch,
		Progress:     int(task.Progress),
		CreatedAt:    task.CreatedAt.Format(time.RFC3339),
	}

	apiResp := mcp.APIResponse{
		OK:   true,
		Data: must(json.Marshal(result)),
	}
	s.serveJSON(w, apiResp)
}

func (s *integrationHTTPServer) handleTaskCancel(w http.ResponseWriter, r *http.Request, path string) {
	taskID := strings.TrimPrefix(path, "/api/v1/tasks/")
	taskID = strings.TrimSuffix(taskID, "/cancel")

	var req mcp.CancelTaskRequest
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&req)
	}

	resp, err := s.taskSvc.Cancel(context.Background(), service.CancelRequest{
		TaskID: taskID,
		Reason: req.Reason,
	})
	if err != nil {
		s.serveError(w, "CANCEL_FAILED", err.Error())
		return
	}

	result := mcp.CancelTaskResponse{
		TaskID:           resp.Task.ID,
		Status:           string(resp.Task.Status),
		TerminatedAgents: []string{},
	}

	apiResp := mcp.APIResponse{
		OK:   true,
		Data: must(json.Marshal(result)),
	}
	s.serveJSON(w, apiResp)
}

func (s *integrationHTTPServer) handleTaskDecompose(w http.ResponseWriter, r *http.Request, path string) {
	taskID := strings.TrimPrefix(path, "/api/v1/tasks/")
	taskID = strings.TrimSuffix(taskID, "/decompose")

	// For integration test, auto-decompose with a single subtask
	decomposeReq := service.DecomposeRequest{
		TaskID: taskID,
		Subtasks: []service.SubtaskSpec{
			{
				Type:          domain.SubtaskTypeCoding,
				Description:   "Auto-decomposed for integration test",
				AgentTemplate: "executor",
			},
		},
	}

	decomposeResp, err := s.taskSvc.Decompose(context.Background(), decomposeReq)
	if err != nil {
		s.serveError(w, "DECOMPOSE_FAILED", err.Error())
		return
	}

	subtaskIDs := make([]string, len(decomposeResp.Subtasks))
	for i, st := range decomposeResp.Subtasks {
		subtaskIDs[i] = st.ID
	}

	result := mcp.DecomposeTaskResponse{
		TaskID:   taskID,
		Subtasks: subtaskIDs,
	}

	apiResp := mcp.APIResponse{
		OK:   true,
		Data: must(json.Marshal(result)),
	}
	s.serveJSON(w, apiResp)
}

func (s *integrationHTTPServer) handleAgentTemplates(w http.ResponseWriter, r *http.Request) {
	templates := []mcp.AgentTemplateResponse{
		{TemplateID: "executor", Name: "Executor Agent", Capabilities: map[string]int{"coding": 9}},
		{TemplateID: "strategist", Name: "Strategist Agent", Capabilities: map[string]int{"analysis": 9}},
		{TemplateID: "tester", Name: "Tester Agent", Capabilities: map[string]int{"testing": 9}},
		{TemplateID: "guardian", Name: "Guardian Agent", Capabilities: map[string]int{"review": 9}},
	}

	apiResp := mcp.APIResponse{
		OK:   true,
		Data: must(json.Marshal(templates)),
	}
	s.serveJSON(w, apiResp)
}

func (s *integrationHTTPServer) handleSessions(w http.ResponseWriter, r *http.Request) {
	result := mcp.SessionResponse{
		Sessions: []mcp.Session{},
		Total:    0,
	}

	apiResp := mcp.APIResponse{
		OK:   true,
		Data: must(json.Marshal(result)),
	}
	s.serveJSON(w, apiResp)
}

func must(data []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return data
}

// ----------------------------------------------------------------------------
// Integration Test Environment
// ----------------------------------------------------------------------------

// integrationTestEnv holds the test environment for integration tests.
type integrationTestEnv struct {
	t            *testing.T
	logger       *zap.Logger
	httpServer   *httptest.Server
	mcpServer    *mcp.Server
	platformClient *mcp.PlatformClient
	taskSvc      *service.TaskService
	orchestrator *orchestrator.OrchestratorImpl
	taskRepo     *inMemoryTaskRepo
	subtaskRepo  *inMemorySubtaskRepo
	mockWorker   *mockWorkerExecutor
	mockGuardian *mockGuardian
	cleanupFn    func()
}

// setupIntegrationEnv creates a test environment with HTTP server, MCP server, and real TaskService+Orchestrator.
func setupIntegrationEnv(t *testing.T) *integrationTestEnv {
	logger := zaptest.NewLogger(t)

	// Create in-memory repositories (same as full_pipeline_test.go)
	taskRepo := newInMemoryTaskRepo()
	subtaskRepo := newInMemorySubtaskRepo()
	outboxWriter := newInMemoryOutboxWriter()
	txManager := newInMemoryTxManager()

	// Create TaskService
	taskSvc := service.NewTaskService(service.TaskServiceInput{
		TaskRepo:     taskRepo,
		SubtaskRepo:  subtaskRepo,
		OutboxWriter: outboxWriter,
		Storage:      txManager,
		Logger:       logger,
	})

	// Create mock worker executor
	mockWorker := newMockWorkerExecutor()

	// Create mock guardian (auto-approve for testing)
	mockGuardian := newMockGuardian()

	// Create orchestrator with mocks
	orch := orchestrator.NewOrchestrator(
		orchestrator.Config{
			MaxConcurrentSessions: 5,
			SessionTimeout:       30 * time.Minute,
			DefaultAgentTemplate: "executor",
			GuardianEnabled:      true,
		},
		taskRepo,
		subtaskRepo,
		outboxWriter,
		newOrchestratorTxManager(txManager),
		nil, // agentRunner
		logger,
		mockWorker,
		mockGuardian,
	)

	// Create HTTP test server with REST API handlers
	httpServer := newIntegrationHTTPServer(t, taskSvc, orch, taskRepo)

	// Create platform client pointing to test server
	platformClient := mcp.NewPlatformClient(httpServer.URL, "test-token", logger)

	// Create MCP server
	executor := mcp.NewToolExecutor(platformClient, logger)
	mcpServer := mcp.NewServer(executor, logger)

	return &integrationTestEnv{
		t:            t,
		logger:       logger,
		httpServer:   httpServer,
		mcpServer:    mcpServer,
		platformClient: platformClient,
		taskSvc:      taskSvc,
		orchestrator: orch,
		taskRepo:     taskRepo,
		subtaskRepo:  subtaskRepo,
		mockWorker:   mockWorker,
		mockGuardian: mockGuardian,
		cleanupFn: func() {
			httpServer.Close()
		},
	}
}

// sendMCPPRequest sends a JSON-RPC request to the MCP server and returns the response.
func (env *integrationTestEnv) sendMCPRequest(method string, params interface{}) *mcp.JSONRPCResponse {
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

// ----------------------------------------------------------------------------
// Integration Tests
// ----------------------------------------------------------------------------

// TestIntegration_MCPSubmitToAgentComplete tests the complete flow from MCP submit to agent execution completion.
func TestIntegration_MCPSubmitToAgentComplete(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	env := setupIntegrationEnv(t)
	defer env.cleanupFn()

	t.Run("MCPSubmitToAgentComplete", func(t *testing.T) {
		testMCPSubmitToAgentComplete(ctx, t, env)
	})
}

func testMCPSubmitToAgentComplete(ctx context.Context, t *testing.T, env *integrationTestEnv) {
	// Step 1: Initialize MCP connection
	t.Log("Step 1: Initializing MCP connection...")
	initResp := env.sendMCPRequest("initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "integration-test",
			"version": "1.0.0",
		},
	})
	require.NotNil(t, initResp, "Initialize should return a response")
	require.Nil(t, initResp.Error, "Initialize should not return an error")

	var initResult mcp.InitializeResult
	err := json.Unmarshal(initResp.Result, &initResult)
	require.NoError(t, err, "Failed to unmarshal initialize result")
	assert.Equal(t, "cloud-agent-platform", initResult.ServerInfo.Name)
	t.Logf("MCP initialized: %s v%s", initResult.ServerInfo.Name, initResult.ServerInfo.Version)

	// Step 2: Submit task via MCP tool
	t.Log("Step 2: Submitting task via MCP task_submit tool...")

	submitParams := map[string]interface{}{
		"goal":    "Implement user authentication feature",
		"priority": 5,
		"repository": map[string]interface{}{
			"url":    "https://github.com/example/test-repo",
			"branch": "main",
		},
		"constraints": []string{"Must use JWT for authentication"},
		"tags":        []string{"auth", "integration-test"},
	}

	submitResp := env.sendMCPRequest("tools/call", map[string]interface{}{
		"name":      "task_submit",
		"arguments": submitParams,
	})
	require.NotNil(t, submitResp, "task_submit should return a response")
	require.Nil(t, submitResp.Error, "task_submit should not return an error: %s", submitResp.Error)

	var submitResult mcp.ToolCallResult
	err = json.Unmarshal(submitResp.Result, &submitResult)
	require.NoError(t, err, "Failed to unmarshal submit result")
	require.False(t, submitResult.IsError, "Submit result should not be an error")
	require.Len(t, submitResult.Content, 1, "Submit result should have 1 content block")

	var taskSubmitResp mcp.TaskSubmitResponse
	err = json.Unmarshal([]byte(submitResult.Content[0].Text), &taskSubmitResp)
	require.NoError(t, err, "Failed to unmarshal task submit response")
	require.NotEmpty(t, taskSubmitResp.TaskID, "Task ID should not be empty")
	require.Equal(t, "pending", taskSubmitResp.Status, "Initial status should be pending")

	taskID := taskSubmitResp.TaskID
	t.Logf("Task submitted via MCP: %s, status: %s", taskID, taskSubmitResp.Status)

	// Step 3: Decompose task via MCP tool
	t.Log("Step 3: Decomposing task via MCP task_decompose tool...")

	decomposeResp := env.sendMCPRequest("tools/call", map[string]interface{}{
		"name": "task_decompose",
		"arguments": map[string]interface{}{
			"taskId": taskID,
		},
	})
	require.NotNil(t, decomposeResp, "task_decompose should return a response")
	require.Nil(t, decomposeResp.Error, "task_decompose should not return an error")

	var decompResult mcp.ToolCallResult
	err = json.Unmarshal(decomposeResp.Result, &decompResult)
	require.NoError(t, err, "Failed to unmarshal decompose result")

	var decompData mcp.DecomposeTaskResponse
	err = json.Unmarshal([]byte(decompResult.Content[0].Text), &decompData)
	require.NoError(t, err, "Failed to unmarshal decompose data")
	require.Equal(t, taskID, decompData.TaskID)
	require.Len(t, decompData.Subtasks, 1, "Should have 1 subtask")

	subtaskID := decompData.Subtasks[0]
	t.Logf("Task decomposed: %s, subtask: %s", taskID, subtaskID)

	// Step 4: Start orchestration by calling orchestrator.StartTask
	t.Log("Step 4: Starting orchestration...")

	task, err := env.taskRepo.GetByID(ctx, taskID)
	require.NoError(t, err, "Failed to get task from repository")

	err = env.orchestrator.StartTask(ctx, task)
	require.NoError(t, err, "StartTask should succeed")

	// Step 5: Wait for task to complete
	t.Log("Step 5: Waiting for task to complete...")

	maxWait := 10 * time.Second
	pollInterval := 100 * time.Millisecond
	deadline := time.Now().Add(maxWait)

	var finalTask *domain.Task
	for time.Now().Before(deadline) {
		finalTask, err = env.taskRepo.GetByID(ctx, taskID)
		require.NoError(t, err, "Failed to get task during polling")

		t.Logf("Task polling: %s, status=%s, version=%d", taskID, finalTask.Status, finalTask.Version)

		if finalTask.Status == domain.TaskStatusCompleted {
			t.Logf("Task %s reached completed state", taskID)
			break
		}

		if finalTask.Status == domain.TaskStatusFailed {
			t.Fatal("Task failed during execution")
		}

		select {
		case <-ctx.Done():
			t.Fatal("Context cancelled during polling")
		case <-time.After(pollInterval):
		}
	}

	// Verify final state
	require.Equal(t, domain.TaskStatusCompleted, finalTask.Status, "Task should be completed")
	require.NotNil(t, finalTask.CompletedAt, "CompletedAt should be set")

	// Give async goroutines time to finish
	time.Sleep(200 * time.Millisecond)

	// Step 6: Verify mock worker was called
	executedIDs := env.mockWorker.GetExecutedIDs()
	require.Len(t, executedIDs, 1, "Mock worker should have been called for 1 subtask")
	require.Equal(t, subtaskID, executedIDs[0], "Mock worker should have executed the subtask")

	// Step 7: Verify task status via MCP
	t.Log("Step 7: Verifying final status via MCP task_status tool...")

	statusResp := env.sendMCPRequest("tools/call", map[string]interface{}{
		"name": "task_status",
		"arguments": map[string]interface{}{
			"taskId": taskID,
		},
	})
	require.NotNil(t, statusResp, "task_status should return a response")
	require.Nil(t, statusResp.Error, "task_status should not return an error")

	var statusResult mcp.ToolCallResult
	err = json.Unmarshal(statusResp.Result, &statusResult)
	require.NoError(t, err, "Failed to unmarshal status result")

	var statusData mcp.TaskStatusResponse
	err = json.Unmarshal([]byte(statusResult.Content[0].Text), &statusData)
	require.NoError(t, err, "Failed to unmarshal status data")
	require.Equal(t, taskID, statusData.TaskID)
	require.Equal(t, "completed", statusData.Status, "Status should be completed")

	t.Log("Integration test completed successfully!")
}

// TestIntegration_MCPSimpleTaskFlow tests a simple task through MCP with single-agent routing.
func TestIntegration_MCPSimpleTaskFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	env := setupIntegrationEnv(t)
	defer env.cleanupFn()

	t.Run("MCPSimpleTaskFlow", func(t *testing.T) {
		testMCPSimpleTaskFlow(ctx, t, env)
	})
}

func testMCPSimpleTaskFlow(ctx context.Context, t *testing.T, env *integrationTestEnv) {
	// Step 1: Submit a simple task via MCP
	t.Log("Step 1: Submitting simple task via MCP...")

	submitResp := env.sendMCPRequest("tools/call", map[string]interface{}{
		"name": "task_submit",
		"arguments": map[string]interface{}{
			"goal":     "Fix typo in README.md",
			"priority": 3,
			"repository": map[string]interface{}{
				"url":    "https://github.com/example/repo",
				"branch": "main",
			},
		},
	})
	require.NotNil(t, submitResp)
	require.Nil(t, submitResp.Error)

	var submitResult mcp.ToolCallResult
	err := json.Unmarshal(submitResp.Result, &submitResult)
	require.NoError(t, err)

	var taskResp mcp.TaskSubmitResponse
	err = json.Unmarshal([]byte(submitResult.Content[0].Text), &taskResp)
	require.NoError(t, err)
	require.Equal(t, "pending", taskResp.Status)

	taskID := taskResp.TaskID
	t.Logf("Simple task submitted: %s", taskID)

	// Step 2: Decompose with single subtask (simple routing)
	t.Log("Step 2: Decomposing simple task...")

	decomposeResp := env.sendMCPRequest("tools/call", map[string]interface{}{
		"name": "task_decompose",
		"arguments": map[string]interface{}{
			"taskId": taskID,
		},
	})
	require.NotNil(t, decomposeResp)
	require.Nil(t, decomposeResp.Error)

	var decompResult mcp.ToolCallResult
	err = json.Unmarshal(decomposeResp.Result, &decompResult)
	require.NoError(t, err)

	var decompData mcp.DecomposeTaskResponse
	err = json.Unmarshal([]byte(decompResult.Content[0].Text), &decompData)
	require.NoError(t, err)
	require.Len(t, decompData.Subtasks, 1, "Simple task should have 1 subtask")

	t.Logf("Simple task decomposed into 1 subtask")

	// Step 3: Start orchestration
	t.Log("Step 3: Starting orchestration...")

	task, err := env.taskRepo.GetByID(ctx, taskID)
	require.NoError(t, err)

	err = env.orchestrator.StartTask(ctx, task)
	require.NoError(t, err)

	// Step 4: Wait for completion
	t.Log("Step 4: Waiting for completion...")

	maxWait := 10 * time.Second
	pollInterval := 100 * time.Millisecond
	deadline := time.Now().Add(maxWait)

	for time.Now().Before(deadline) {
		task, err = env.taskRepo.GetByID(ctx, taskID)
		require.NoError(t, err)

		if task.Status == domain.TaskStatusCompleted {
			break
		}

		if task.Status == domain.TaskStatusFailed {
			t.Fatal("Simple task failed")
		}

		select {
		case <-ctx.Done():
			t.Fatal("Context cancelled")
		case <-time.After(pollInterval):
		}
	}

	require.Equal(t, domain.TaskStatusCompleted, task.Status, "Simple task should complete")
	time.Sleep(200 * time.Millisecond)

	t.Log("Simple task flow completed successfully!")
}

// TestIntegration_MCPComplexTaskFlow tests a complex task through MCP with multi-agent routing.
func TestIntegration_MCPComplexTaskFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	env := setupIntegrationEnv(t)
	defer env.cleanupFn()

	t.Run("MCPComplexTaskFlow", func(t *testing.T) {
		testMCPComplexTaskFlow(ctx, t, env)
	})
}

func testMCPComplexTaskFlow(ctx context.Context, t *testing.T, env *integrationTestEnv) {
	// Submit a complex task
	t.Log("Step 1: Submitting complex task via MCP...")

	submitResp := env.sendMCPRequest("tools/call", map[string]interface{}{
		"name": "task_submit",
		"arguments": map[string]interface{}{
			"goal":       "Implement complete user authentication system with OAuth2, JWT, and MFA",
			"priority":   8,
			"repository": map[string]interface{}{
				"url":    "https://github.com/example/complex-repo",
				"branch": "main",
			},
			"constraints": []string{
				"Must use JWT for authentication",
				"Must support OAuth2 providers",
			},
		},
	})
	require.NotNil(t, submitResp)
	require.Nil(t, submitResp.Error)

	var submitResult mcp.ToolCallResult
	err := json.Unmarshal(submitResp.Result, &submitResult)
	require.NoError(t, err)

	var taskResp mcp.TaskSubmitResponse
	err = json.Unmarshal([]byte(submitResult.Content[0].Text), &taskResp)
	require.NoError(t, err)
	require.Equal(t, "pending", taskResp.Status)

	taskID := taskResp.TaskID
	t.Logf("Complex task submitted: %s", taskID)

	// Decompose - for complex tasks, multiple subtasks would be created
	// In our test environment, decompose creates a single subtask
	t.Log("Step 2: Decomposing complex task...")

	decomposeResp := env.sendMCPRequest("tools/call", map[string]interface{}{
		"name": "task_decompose",
		"arguments": map[string]interface{}{
			"taskId": taskID,
		},
	})
	require.NotNil(t, decomposeResp)
	require.Nil(t, decomposeResp.Error)

	var decompResult mcp.ToolCallResult
	err = json.Unmarshal(decomposeResp.Result, &decompResult)
	require.NoError(t, err)

	var decompData mcp.DecomposeTaskResponse
	err = json.Unmarshal([]byte(decompResult.Content[0].Text), &decompData)
	require.NoError(t, err)
	t.Logf("Complex task decomposed into %d subtasks", len(decompData.Subtasks))

	// Start orchestration
	t.Log("Step 3: Starting orchestration...")

	task, err := env.taskRepo.GetByID(ctx, taskID)
	require.NoError(t, err)

	err = env.orchestrator.StartTask(ctx, task)
	require.NoError(t, err)

	// Wait for completion
	t.Log("Step 4: Waiting for completion...")

	maxWait := 10 * time.Second
	pollInterval := 100 * time.Millisecond
	deadline := time.Now().Add(maxWait)

	for time.Now().Before(deadline) {
		task, err = env.taskRepo.GetByID(ctx, taskID)
		require.NoError(t, err)

		if task.Status == domain.TaskStatusCompleted {
			break
		}

		if task.Status == domain.TaskStatusFailed {
			t.Fatal("Complex task failed")
		}

		select {
		case <-ctx.Done():
			t.Fatal("Context cancelled")
		case <-time.After(pollInterval):
		}
	}

	require.Equal(t, domain.TaskStatusCompleted, task.Status, "Complex task should complete")
	time.Sleep(200 * time.Millisecond)

	t.Log("Complex task flow completed successfully!")
}

// TestIntegration_MCPTaskListAndStatus tests task list and status via MCP.
func TestIntegration_MCPTaskListAndStatus(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	env := setupIntegrationEnv(t)
	defer env.cleanupFn()

	t.Run("MCPTaskListAndStatus", func(t *testing.T) {
		testMCPTaskListAndStatus(ctx, t, env)
	})
}

func testMCPTaskListAndStatus(ctx context.Context, t *testing.T, env *integrationTestEnv) {
	// Submit multiple tasks
	t.Log("Step 1: Submitting multiple tasks...")

	taskIDs := make([]string, 3)
	for i := 0; i < 3; i++ {
		submitResp := env.sendMCPRequest("tools/call", map[string]interface{}{
			"name": "task_submit",
			"arguments": map[string]interface{}{
				"goal":     fmt.Sprintf("Test task %d", i+1),
				"priority": i + 1,
				"repository": map[string]interface{}{
					"url":    "https://github.com/example/repo",
					"branch": "main",
				},
			},
		})
		require.NotNil(t, submitResp)
		require.Nil(t, submitResp.Error)

		var submitResult mcp.ToolCallResult
		err := json.Unmarshal(submitResp.Result, &submitResult)
		require.NoError(t, err)

		var taskResp mcp.TaskSubmitResponse
		err = json.Unmarshal([]byte(submitResult.Content[0].Text), &taskResp)
		require.NoError(t, err)
		taskIDs[i] = taskResp.TaskID
		t.Logf("Submitted task %d: %s", i+1, taskIDs[i])
	}

	// List tasks via MCP
	t.Log("Step 2: Listing tasks via MCP...")

	listResp := env.sendMCPRequest("tools/call", map[string]interface{}{
		"name": "task_list",
		"arguments": map[string]interface{}{
			"limit": 10,
		},
	})
	require.NotNil(t, listResp)
	require.Nil(t, listResp.Error)

	var listResult mcp.ToolCallResult
	err := json.Unmarshal(listResp.Result, &listResult)
	require.NoError(t, err)

	var listData mcp.TaskListResponse
	err = json.Unmarshal([]byte(listResult.Content[0].Text), &listData)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(listData.Tasks), 3, "Should have at least 3 tasks")

	// Verify all submitted tasks are in the list
	foundIDs := make(map[string]bool)
	for _, task := range listData.Tasks {
		foundIDs[task.TaskID] = true
		t.Logf("Found task in list: %s, status: %s", task.TaskID, task.Status)
	}

	for _, taskID := range taskIDs {
		require.True(t, foundIDs[taskID], "Task %s should be in list", taskID)
	}

	// Check status of each task via MCP
	t.Log("Step 3: Checking status of each task via MCP...")

	for i, taskID := range taskIDs {
		statusResp := env.sendMCPRequest("tools/call", map[string]interface{}{
			"name": "task_status",
			"arguments": map[string]interface{}{
				"taskId": taskID,
			},
		})
		require.NotNil(t, statusResp)
		require.Nil(t, statusResp.Error)

		var statusResult mcp.ToolCallResult
		err := json.Unmarshal(statusResp.Result, &statusResult)
		require.NoError(t, err)

		var statusData mcp.TaskStatusResponse
		err = json.Unmarshal([]byte(statusResult.Content[0].Text), &statusData)
		require.NoError(t, err)
		require.Equal(t, taskID, statusData.TaskID)
		require.Equal(t, "pending", statusData.Status, "Task %d should be pending", i+1)
	}

	t.Log("Task list and status test completed successfully!")
}

// TestIntegration_MCPAgentList tests agent_list tool via MCP.
func TestIntegration_MCPAgentList(t *testing.T) {
	env := setupIntegrationEnv(t)
	defer env.cleanupFn()

	t.Run("MCPAgentList", func(t *testing.T) {
		testMCPAgentList(t, env)
	})
}

func testMCPAgentList(t *testing.T, env *integrationTestEnv) {
	// List available agents
	t.Log("Listing agents via MCP...")

	resp := env.sendMCPRequest("tools/call", map[string]interface{}{
		"name":      "agent_list",
		"arguments": map[string]interface{}{},
	})
	require.NotNil(t, resp)
	require.Nil(t, resp.Error)

	var result mcp.ToolCallResult
	err := json.Unmarshal(resp.Result, &result)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var agents []mcp.AgentTemplateResponse
	err = json.Unmarshal([]byte(result.Content[0].Text), &agents)
	require.NoError(t, err)
	require.Len(t, agents, 4, "Should have 4 agent templates")

	// Verify expected agents
	agentMap := make(map[string]bool)
	for _, a := range agents {
		agentMap[a.TemplateID] = true
		t.Logf("Agent: %s (%s)", a.TemplateID, a.Name)
	}

	assert.True(t, agentMap["executor"])
	assert.True(t, agentMap["strategist"])
	assert.True(t, agentMap["tester"])
	assert.True(t, agentMap["guardian"])

	t.Log("Agent list test completed successfully!")
}

// TestIntegration_MCPToolsList tests that all MCP tools are registered.
func TestIntegration_MCPToolsList(t *testing.T) {
	env := setupIntegrationEnv(t)
	defer env.cleanupFn()

	t.Run("MCPToolsList", func(t *testing.T) {
		testMCPToolsList(t, env)
	})
}

func testMCPToolsList(t *testing.T, env *integrationTestEnv) {
	// List tools
	resp := env.sendMCPRequest("tools/list", nil)
	require.NotNil(t, resp)
	require.Nil(t, resp.Error)

	var result mcp.ToolsListResult
	err := json.Unmarshal(resp.Result, &result)
	require.NoError(t, err)
	require.Len(t, result.Tools, 9, "Expected 9 MCP tools")

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

	t.Log("Tools list test completed successfully!")
}

// TestIntegration_MCPCancelTask tests task cancellation via MCP.
func TestIntegration_MCPCancelTask(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	env := setupIntegrationEnv(t)
	defer env.cleanupFn()

	t.Run("MCPCancelTask", func(t *testing.T) {
		testMCPCancelTask(ctx, t, env)
	})
}

func testMCPCancelTask(ctx context.Context, t *testing.T, env *integrationTestEnv) {
	// Submit a task
	t.Log("Step 1: Submitting task...")

	submitResp := env.sendMCPRequest("tools/call", map[string]interface{}{
		"name": "task_submit",
		"arguments": map[string]interface{}{
			"goal":     "Task to cancel",
			"priority": 5,
			"repository": map[string]interface{}{
				"url":    "https://github.com/example/repo",
				"branch": "main",
			},
		},
	})
	require.NotNil(t, submitResp)
	require.Nil(t, submitResp.Error)

	var submitResult mcp.ToolCallResult
	err := json.Unmarshal(submitResp.Result, &submitResult)
	require.NoError(t, err)

	var taskResp mcp.TaskSubmitResponse
	err = json.Unmarshal([]byte(submitResult.Content[0].Text), &taskResp)
	require.NoError(t, err)

	taskID := taskResp.TaskID
	t.Logf("Task submitted: %s", taskID)

	// Cancel the task via MCP
	t.Log("Step 2: Cancelling task via MCP...")

	cancelResp := env.sendMCPRequest("tools/call", map[string]interface{}{
		"name": "task_cancel",
		"arguments": map[string]interface{}{
			"taskId": taskID,
			"reason": "Integration test cancellation",
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

	// Verify via task_status
	statusResp := env.sendMCPRequest("tools/call", map[string]interface{}{
		"name": "task_status",
		"arguments": map[string]interface{}{
			"taskId": taskID,
		},
	})
	require.NotNil(t, statusResp)

	var statusResult mcp.ToolCallResult
	err = json.Unmarshal(statusResp.Result, &statusResult)
	require.NoError(t, err)

	var statusData mcp.TaskStatusResponse
	err = json.Unmarshal([]byte(statusResult.Content[0].Text), &statusData)
	require.NoError(t, err)
	require.Equal(t, taskID, statusData.TaskID)
	require.Equal(t, "cancelled", statusData.Status)

	t.Log("Cancel task test completed successfully!")
}
