// Package gateway implements L5-Gateway layer: protocol adaptation, middleware chain,
// and request routing via connect-go.
package gateway

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/cloud-agent-platform/cap/internal/domain"
	"github.com/cloud-agent-platform/cap/internal/mcp"
	"github.com/cloud-agent-platform/cap/internal/service"
	"go.uber.org/zap"
)

// RESTAdapter handles REST API requests and adapts them to the service layer.
type RESTAdapter struct {
	svc    *service.TaskService
	logger *zap.Logger
}

// NewRESTAdapter creates a new RESTAdapter.
func NewRESTAdapter(svc *service.TaskService, logger *zap.Logger) *RESTAdapter {
	return &RESTAdapter{
		svc:    svc,
		logger: logger,
	}
}

// writeJSON writes a JSON response with the standard wrapper format.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := mcp.APIResponse{
		OK:   status >= 200 && status < 300,
		Data: mustMarshal(data),
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		// If we fail to encode, we can't do much - the response is already started
		return
	}
}

// writeError writes an error response in the standard format.
func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := mcp.APIResponse{
		OK: false,
		Error: &mcp.APIError{
			Code:    code,
			Message: message,
		},
	}
	json.NewEncoder(w).Encode(resp)
}

// mustMarshal marshals data to JSON or returns nil on error.
func mustMarshal(data any) json.RawMessage {
	b, err := json.Marshal(data)
	if err != nil {
		return nil
	}
	return b
}

// mapServiceError maps service/domain errors to HTTP status codes and error codes.
func mapServiceError(err error) (int, string, string) {
	if err == nil {
		return http.StatusOK, "", ""
	}

	var appErr *domain.AppError
	if ok := mapErrorToCode(err, &appErr); ok {
		switch appErr.Code {
		case domain.CodeL1DBConnect, domain.CodeL1RedisConnect:
			return http.StatusInternalServerError, "L1DBConnect", appErr.Message
		case domain.CodeL2AggregateNotFound, domain.CodeL4TaskNotFound, domain.CodeL4SubtaskNotFound:
			return http.StatusNotFound, string(appErr.Code), appErr.Message
		case domain.CodeL2InvalidStateTransition, domain.CodeL4TaskStateInvalid:
			return http.StatusConflict, string(appErr.Code), appErr.Message
		case domain.CodeL2OptimisticLock:
			return http.StatusConflict, string(appErr.Code), appErr.Message
		case domain.CodeL3AuthnFailed, domain.CodeL3TokenExpired:
			return http.StatusUnauthorized, string(appErr.Code), appErr.Message
		case domain.CodeL3AuthzDenied:
			return http.StatusForbidden, string(appErr.Code), appErr.Message
		case domain.CodeL3RateLimited:
			return http.StatusTooManyRequests, string(appErr.Code), appErr.Message
		case domain.CodeL5InvalidRequest:
			return http.StatusBadRequest, string(appErr.Code), appErr.Message
		default:
			return http.StatusInternalServerError, string(appErr.Code), appErr.Message
		}
	}

	return http.StatusInternalServerError, "InternalError", err.Error()
}

// mapErrorToCode maps an error to an AppError if possible.
func mapErrorToCode(err error, appErr **domain.AppError) bool {
	if err == nil {
		return false
	}
	var ae *domain.AppError
	if ok := errors.As(err, &ae); ok {
		*appErr = ae
		return true
	}
	return false
}

// SubmitTask handles POST /api/v1/tasks.
func (a *RESTAdapter) SubmitTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "POST required")
		return
	}

	ctx := r.Context()
	uc, err := extractUserContext(ctx)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Unauthenticated", "user context not found")
		return
	}

	var req mcp.TaskSubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "InvalidRequest", "invalid JSON body")
		return
	}

	svcReq := service.SubmitRequest{
		Goal:                 req.Goal,
		RepositoryURL:        getRepoURLFromInput(req.Repository),
		BaseBranch:           getRepoBranchFromInput(req.Repository),
		Constraints:          req.Constraints,
		VerificationCriteria: req.VerificationCriteria,
		Priority:             req.Priority,
		Tags:                 req.Tags,
		ClientID:             uc.clientID,
	}

	if req.AgentHint != nil {
		svcReq.AgentHint = &domain.AgentHint{
			Templates: req.AgentHint.Templates,
			Model:     req.AgentHint.Model,
			MaxAgents: req.AgentHint.MaxAgents,
		}
	}

	resp, err := a.svc.Submit(ctx, svcReq)
	if err != nil {
		a.logger.Error("REST SubmitTask failed",
			zap.String("layer", "L5"),
			zap.String("user_id", uc.userID),
			zap.Error(err),
		)
		status, code, msg := mapServiceError(err)
		writeError(w, status, code, msg)
		return
	}

	result := mcp.TaskSubmitResponse{
		TaskID:       resp.Task.ID,
		Status:       mapDomainStatusToString(resp.Task.Status),
		ResultBranch: resp.Task.ResultBranch,
		CreatedAt:    resp.Task.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	writeJSON(w, http.StatusOK, result)
}

// GetTask handles GET /api/v1/tasks/:id.
func (a *RESTAdapter) GetTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "GET required")
		return
	}

	ctx := r.Context()
	uc, err := extractUserContext(ctx)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Unauthenticated", "user context not found")
		return
	}

	taskID := extractPathParam(r.URL.Path, "/api/v1/tasks/")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "InvalidRequest", "task_id is required")
		return
	}

	svcReq := service.GetRequest{
		TaskID: taskID,
	}

	resp, err := a.svc.Get(ctx, svcReq)
	if err != nil {
		a.logger.Error("REST GetTask failed",
			zap.String("layer", "L5"),
			zap.String("user_id", uc.userID),
			zap.String("task_id", taskID),
			zap.Error(err),
		)
		status, code, msg := mapServiceError(err)
		writeError(w, status, code, msg)
		return
	}

	result := mapTaskToResponse(resp.Task)
	writeJSON(w, http.StatusOK, result)
}

// ListTasks handles GET /api/v1/tasks.
func (a *RESTAdapter) ListTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "GET required")
		return
	}

	ctx := r.Context()
	uc, err := extractUserContext(ctx)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Unauthenticated", "user context not found")
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	svcReq := service.ListRequest{
		Limit:    pageSize,
		Offset:   (page - 1) * pageSize,
		ClientID: uc.clientID,
	}

	if status := r.URL.Query().Get("status"); status != "" {
		s := domain.TaskStatus(status)
		svcReq.Status = &s
	}

	resp, err := a.svc.List(ctx, svcReq)
	if err != nil {
		a.logger.Error("REST ListTasks failed",
			zap.String("layer", "L5"),
			zap.String("user_id", uc.userID),
			zap.Error(err),
		)
		status, code, msg := mapServiceError(err)
		writeError(w, status, code, msg)
		return
	}

	tasks := make([]mcp.TaskStatusResponse, len(resp.Tasks))
	for i, t := range resp.Tasks {
		tasks[i] = mapTaskToResponse(t)
	}

	result := mcp.TaskListResponse{
		Tasks:    tasks,
		Total:    resp.Total,
		Page:     page,
		PageSize: pageSize,
	}
	writeJSON(w, http.StatusOK, result)
}

// CancelTask handles POST /api/v1/tasks/:id/cancel.
func (a *RESTAdapter) CancelTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "POST required")
		return
	}

	ctx := r.Context()
	uc, err := extractUserContext(ctx)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Unauthenticated", "user context not found")
		return
	}

	taskID := extractPathParam(r.URL.Path, "/api/v1/tasks/")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "InvalidRequest", "task_id is required")
		return
	}

	var req mcp.CancelTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Allow empty body
		req = mcp.CancelTaskRequest{}
	}

	svcReq := service.CancelRequest{
		TaskID: taskID,
		Reason: req.Reason,
	}

	resp, err := a.svc.Cancel(ctx, svcReq)
	if err != nil {
		a.logger.Error("REST CancelTask failed",
			zap.String("layer", "L5"),
			zap.String("user_id", uc.userID),
			zap.String("task_id", taskID),
			zap.Error(err),
		)
		status, code, msg := mapServiceError(err)
		writeError(w, status, code, msg)
		return
	}

	result := mcp.CancelTaskResponse{
		TaskID: resp.Task.ID,
		Status: mapDomainStatusToString(resp.Task.Status),
	}
	writeJSON(w, http.StatusOK, result)
}

// DecideTask handles POST /api/v1/tasks/:taskId/subtasks/:subtaskId/decision.
func (a *RESTAdapter) DecideTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "POST required")
		return
	}

	ctx := r.Context()
	uc, err := extractUserContext(ctx)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Unauthenticated", "user context not found")
		return
	}

	// Extract task ID and subtask ID from path
	// Path: /api/v1/tasks/{taskId}/subtasks/{subtaskId}/decision
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/tasks/"), "/")
	if len(parts) < 4 || parts[2] != "subtasks" {
		writeError(w, http.StatusBadRequest, "InvalidRequest", "invalid path format")
		return
	}
	taskID := parts[0]
	subtaskID := parts[3]

	var req mcp.DecideRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "InvalidRequest", "invalid JSON body")
		return
	}

	var mods map[string]string
	if req.Modifications != "" {
		mods = map[string]string{"raw": req.Modifications}
	}

	svcReq := service.DecideRequest{
		TaskID:        taskID,
		SubtaskID:     subtaskID,
		Feedback:      req.Feedback,
		Modifications: mods,
	}

	// Map decision string to service.Decision
	switch strings.ToLower(req.Decision) {
	case "approve":
		svcReq.Decision = service.DecisionApprove
	case "reject":
		svcReq.Decision = service.DecisionReject
	case "modify":
		svcReq.Decision = service.DecisionModify
	default:
		writeError(w, http.StatusBadRequest, "InvalidRequest", "decision must be approve, reject, or modify")
		return
	}

	resp, err := a.svc.Decide(ctx, svcReq)
	if err != nil {
		a.logger.Error("REST DecideTask failed",
			zap.String("layer", "L5"),
			zap.String("user_id", uc.userID),
			zap.String("task_id", taskID),
			zap.String("subtask_id", subtaskID),
			zap.Error(err),
		)
		status, code, msg := mapServiceError(err)
		writeError(w, status, code, msg)
		return
	}

	result := mcp.DecideResponse{
		TaskID:    resp.TaskID,
		SubtaskID: resp.SubtaskID,
		Status:    resp.Status,
	}
	writeJSON(w, http.StatusOK, result)
}

// ListAgents handles GET /api/v1/agent-templates.
func (a *RESTAdapter) ListAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "GET required")
		return
	}

	result := []mcp.AgentTemplateResponse{
		{
			TemplateID:        "observer",
			Name:              "Observer",
			Description:       "观察者，监控任务进度",
			Capabilities:      map[string]int{"monitoring": 10, "reporting": 8},
			AvailableModels:   []string{"gpt-4o", "claude-3-5-sonnet"},
			MaxConcurrent:     5,
			AvgCompletionTime: 30,
			SuccessRate:       0.95,
		},
		{
			TemplateID:        "strategist",
			Name:              "Strategist",
			Description:       "策略师，分析复杂度",
			Capabilities:      map[string]int{"analysis": 10, "planning": 9},
			AvailableModels:   []string{"gpt-4o", "claude-3-5-sonnet"},
			MaxConcurrent:     3,
			AvgCompletionTime: 45,
			SuccessRate:       0.92,
		},
		{
			TemplateID:        "executor",
			Name:              "Executor",
			Description:       "执行者，编码实现",
			Capabilities:      map[string]int{"coding": 10, "debugging": 7},
			AvailableModels:   []string{"gpt-4o", "claude-3-5-sonnet"},
			MaxConcurrent:     8,
			AvgCompletionTime: 120,
			SuccessRate:       0.88,
		},
		{
			TemplateID:        "guardian",
			Name:              "Guardian",
			Description:       "守卫，风险评估",
			Capabilities:      map[string]int{"security": 10, "review": 8},
			AvailableModels:   []string{"gpt-4o", "claude-3-5-sonnet"},
			MaxConcurrent:     4,
			AvgCompletionTime: 40,
			SuccessRate:       0.94,
		},
		{
			TemplateID:        "tester",
			Name:              "Tester",
			Description:       "测试者，验证结果",
			Capabilities:      map[string]int{"testing": 10, "validation": 9},
			AvailableModels:   []string{"gpt-4o", "claude-3-5-sonnet"},
			MaxConcurrent:     6,
			AvgCompletionTime: 60,
			SuccessRate:       0.90,
		},
		{
			TemplateID:        "researcher",
			Name:              "Researcher",
			Description:       "研究者，信息收集",
			Capabilities:      map[string]int{"research": 10, "summarization": 8},
			AvailableModels:   []string{"gpt-4o", "claude-3-5-sonnet"},
			MaxConcurrent:     5,
			AvgCompletionTime: 50,
			SuccessRate:       0.93,
		},
	}
	writeJSON(w, http.StatusOK, result)
}

// ListSessions handles GET /api/v1/sessions.
func (a *RESTAdapter) ListSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "GET required")
		return
	}

	// Placeholder - sessions not yet implemented, returns empty list
	result := mcp.SessionResponse{
		Sessions: []mcp.Session{},
		Total:    0,
	}
	writeJSON(w, http.StatusOK, result)
}

// PlatformStatus handles GET /api/v1/platform/status.
func (a *RESTAdapter) PlatformStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "GET required")
		return
	}

	result := mcp.PlatformStatusResponse{}
	result.Pool.Total = 0
	result.Pool.Idle = 0
	result.Pool.Busy = 0
	result.Pool.MaxCapacity = 10
	result.Queue.Pending = 0
	result.Queue.AvgWaitTime = 0.0
	result.Models = []mcp.ModelStatus{}
	result.Uptime = 0
	writeJSON(w, http.StatusOK, result)
}

// GetTaskDiff handles GET /api/v1/tasks/:id/diff.
func (a *RESTAdapter) GetTaskDiff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "GET required")
		return
	}

	ctx := r.Context()
	uc, err := extractUserContext(ctx)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Unauthenticated", "user context not found")
		return
	}

	taskID := extractPathParam(r.URL.Path, "/api/v1/tasks/")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "InvalidRequest", "task_id is required")
		return
	}

	// GetDiff is not implemented in service layer yet
	// Return a placeholder response indicating the feature is not yet available
	a.logger.Info("REST GetTaskDiff called but not yet implemented",
		zap.String("layer", "L5"),
		zap.String("user_id", uc.userID),
		zap.String("task_id", taskID),
	)

	// Return a clear message that this feature is pending service layer implementation
	result := mcp.DiffResponse{
		TaskID: taskID,
		Diff:   "",
		Stats:  nil,
	}
	resp := struct {
		OK      bool                `json:"ok"`
		Data    mcp.DiffResponse   `json:"data"`
		Message string              `json:"message,omitempty"`
	}{
		OK:      true,
		Data:    result,
		Message: "GetTaskDiff not yet implemented in service layer",
	}
	writeJSON(w, http.StatusOK, resp)
}

// DecomposeTask handles POST /api/v1/tasks/:id/decompose.
func (a *RESTAdapter) DecomposeTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "POST required")
		return
	}

	ctx := r.Context()
	uc, err := extractUserContext(ctx)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Unauthenticated", "user context not found")
		return
	}

	taskID := extractPathParam(r.URL.Path, "/api/v1/tasks/")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "InvalidRequest", "task_id is required")
		return
	}

	var body struct {
		Subtasks []struct {
			Type          string   `json:"type"`
			Description   string   `json:"description"`
			AgentTemplate string   `json:"agent_template"`
			Dependencies  []string `json:"dependencies"`
		} `json:"subtasks"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "InvalidRequest", "invalid JSON body")
		return
	}

	if len(body.Subtasks) == 0 {
		writeError(w, http.StatusBadRequest, "InvalidRequest", "at least one subtask is required")
		return
	}

	subtasks := make([]service.SubtaskSpec, len(body.Subtasks))
	for i, st := range body.Subtasks {
		subtasks[i] = service.SubtaskSpec{
			Type:          domain.SubtaskType(st.Type),
			Description:   st.Description,
			AgentTemplate: st.AgentTemplate,
			Dependencies:  st.Dependencies,
		}
	}

	svcReq := service.DecomposeRequest{
		TaskID:   taskID,
		Subtasks: subtasks,
	}

	resp, err := a.svc.Decompose(ctx, svcReq)
	if err != nil {
		a.logger.Error("REST DecomposeTask failed",
			zap.String("layer", "L5"),
			zap.String("user_id", uc.userID),
			zap.String("task_id", taskID),
			zap.Error(err),
		)
		status, code, msg := mapServiceError(err)
		writeError(w, status, code, msg)
		return
	}

	// Build response
	subtaskIDs := make([]string, len(resp.Subtasks))
	for i, st := range resp.Subtasks {
		subtaskIDs[i] = st.ID
	}

	result := mcp.DecomposeTaskResponse{
		TaskID:   resp.Task.ID,
		Subtasks: subtaskIDs,
	}
	writeJSON(w, http.StatusOK, result)
}

// GetGuardianCheck handles GET /api/v1/tasks/:id/guardian.
func (a *RESTAdapter) GetGuardianCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "GET required")
		return
	}

	ctx := r.Context()
	uc, err := extractUserContext(ctx)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Unauthenticated", "user context not found")
		return
	}

	taskID := extractPathParam(r.URL.Path, "/api/v1/tasks/")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "InvalidRequest", "task_id is required")
		return
	}

	// Get the task to retrieve guardian check info
	svcReq := service.GetRequest{TaskID: taskID}
	resp, err := a.svc.Get(ctx, svcReq)
	if err != nil {
		a.logger.Error("REST GetGuardianCheck failed",
			zap.String("layer", "L5"),
			zap.String("user_id", uc.userID),
			zap.String("task_id", taskID),
			zap.Error(err),
		)
		status, code, msg := mapServiceError(err)
		writeError(w, status, code, msg)
		return
	}

	task := resp.Task

	// Build guardian check response based on task state
	var approvalStatus string
	if task.Status == domain.TaskStatusConfirming {
		approvalStatus = "pending"
	} else if task.Status == domain.TaskStatusReviewing {
		approvalStatus = "approved"
	}

	// Determine risk level based on estimated cost
	riskLevel := "low"
	if task.EstimatedCost > 100 {
		riskLevel = "high"
	} else if task.EstimatedCost > 10 {
		riskLevel = "medium"
	}

	result := struct {
		TaskID          string  `json:"taskId"`
		RequireApproval bool    `json:"requireApproval"`
		RiskLevel       string  `json:"riskLevel"`
		EstimatedCost   float64 `json:"estimatedCost"`
		ApprovalStatus  string  `json:"approvalStatus"`
	}{
		TaskID:          task.ID,
		RequireApproval: task.EstimatedCost > 1.0,
		RiskLevel:       riskLevel,
		EstimatedCost:   task.EstimatedCost,
		ApprovalStatus:  approvalStatus,
	}
	writeJSON(w, http.StatusOK, result)
}

// ListAgentsForTask handles GET /api/v1/tasks/:id/agents.
func (a *RESTAdapter) ListAgentsForTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "GET required")
		return
	}

	ctx := r.Context()
	uc, err := extractUserContext(ctx)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Unauthenticated", "user context not found")
		return
	}

	taskID := extractPathParam(r.URL.Path, "/api/v1/tasks/")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "InvalidRequest", "task_id is required")
		return
	}

	// Get the task to check if it exists and get its subtasks
	svcReq := service.GetRequest{TaskID: taskID}
	_, err = a.svc.Get(ctx, svcReq)
	if err != nil {
		a.logger.Error("REST ListAgentsForTask failed",
			zap.String("layer", "L5"),
			zap.String("user_id", uc.userID),
			zap.String("task_id", taskID),
			zap.Error(err),
		)
		status, code, msg := mapServiceError(err)
		writeError(w, status, code, msg)
		return
	}

	// Agents are represented as subtasks in the system
	// For now, return empty list since agent tracking is not yet fully implemented
	// The subtask repository would need to be queried for agents
	result := struct {
		TaskID string        `json:"taskId"`
		Agents []interface{} `json:"agents"`
		Total  int           `json:"total"`
	}{
		TaskID: taskID,
		Agents: []interface{}{},
		Total:  0,
	}
	writeJSON(w, http.StatusOK, result)
}

// handleTasks dispatches requests to /api/v1/tasks (GET=list, POST=submit).
func (a *RESTAdapter) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.ListTasks(w, r)
	case http.MethodPost:
		a.SubmitTask(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "GET or POST required")
	}
}

// handleTaskOperations dispatches requests to /api/v1/tasks/{id}/* operations.
func (a *RESTAdapter) handleTaskOperations(w http.ResponseWriter, r *http.Request) {
	// Path after /api/v1/tasks/ is like:
	// {id}                  -> GetTask
	// {id}/cancel           -> CancelTask
	// {id}/decompose        -> DecomposeTask
	// {id}/diff             -> GetTaskDiff
	// {id}/guardian         -> GetGuardianCheck
	// {id}/agents           -> ListAgentsForTask
	// {id}/subtasks/{subtaskId}/decision -> DecideTask

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/tasks/")
	if path == "" {
		writeError(w, http.StatusBadRequest, "InvalidRequest", "task_id is required")
		return
	}

	// Check if this is a subtask decision path
	if strings.HasPrefix(path, "subtasks/") {
		// This shouldn't happen with proper path structure
		writeError(w, http.StatusBadRequest, "InvalidRequest", "invalid path format")
		return
	}

	parts := strings.Split(path, "/")

	// {id} alone - GetTask
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			a.GetTask(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "GET required")
		}
		return
	}

	// {id}/subtasks/{subtaskId}/decision
	if len(parts) >= 4 && parts[1] == "subtasks" && parts[3] == "decision" {
		a.DecideTask(w, r)
		return
	}

	// {id}/cancel, {id}/decompose, {id}/diff, {id}/guardian, {id}/agents
	if len(parts) == 2 {
		switch parts[1] {
		case "cancel":
			a.CancelTask(w, r)
		case "decompose":
			a.DecomposeTask(w, r)
		case "diff":
			a.GetTaskDiff(w, r)
		case "guardian":
			a.GetGuardianCheck(w, r)
		case "agents":
			a.ListAgentsForTask(w, r)
		default:
			writeError(w, http.StatusNotFound, "NotFound", "unknown operation")
		}
		return
	}

	writeError(w, http.StatusBadRequest, "InvalidRequest", "invalid path format")
}

// Helper functions

func getRepoURLFromInput(repo *mcp.RepositoryInput) string {
	if repo == nil {
		return ""
	}
	return repo.URL
}

func getRepoBranchFromInput(repo *mcp.RepositoryInput) string {
	if repo == nil {
		return ""
	}
	return repo.Branch
}

func mapDomainStatusToString(status domain.TaskStatus) string {
	switch status {
	case domain.TaskStatusPending:
		return "PENDING"
	case domain.TaskStatusDecomposing:
		return "DECOMPOSING"
	case domain.TaskStatusDispatched:
		return "DISPATCHED"
	case domain.TaskStatusRunning:
		return "RUNNING"
	case domain.TaskStatusReviewing:
		return "REVIEWING"
	case domain.TaskStatusConfirming:
		return "CONFIRMING"
	case domain.TaskStatusCompleted:
		return "COMPLETED"
	case domain.TaskStatusFailed:
		return "FAILED"
	case domain.TaskStatusCancelled:
		return "CANCELLED"
	default:
		return "UNSPECIFIED"
	}
}

func mapTaskToResponse(task *domain.Task) mcp.TaskStatusResponse {
	if task == nil {
		return mcp.TaskStatusResponse{}
	}

	var startedAt, completedAt string
	if task.StartedAt != nil {
		startedAt = task.StartedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	if task.CompletedAt != nil {
		completedAt = task.CompletedAt.Format("2006-01-02T15:04:05Z07:00")
	}

	return mcp.TaskStatusResponse{
		TaskID:       task.ID,
		Status:       mapDomainStatusToString(task.Status),
		Goal:         task.Goal,
		Priority:     task.Priority,
		ResultBranch: task.ResultBranch,
		Progress:     int(task.Progress),
		CreatedAt:    task.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		StartedAt:    startedAt,
		CompletedAt:  completedAt,
	}
}

// extractPathParam extracts the path parameter after the given prefix.
// For example, prefix "/api/v1/tasks/" and path "/api/v1/tasks/123" returns "123".
func extractPathParam(path, prefix string) string {
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	param := strings.TrimPrefix(path, prefix)
	// Remove trailing slash and anything after
	if idx := strings.Index(param, "/"); idx != -1 {
		param = param[:idx]
	}
	return param
}