// Package gateway implements L5-Gateway layer: protocol adaptation, middleware chain,
// and request routing via connect-go.
package gateway

import (
	"context"
	"errors"
	"time"

	"connectrpc.com/connect"
	v1 "github.com/cloud-agent-platform/cap/api/cap/v1"
	"github.com/cloud-agent-platform/cap/api/cap/v1/capv1connect"
	"github.com/cloud-agent-platform/cap/internal/domain"
	"github.com/cloud-agent-platform/cap/internal/service"
	"go.uber.org/zap"
)

// TaskServiceHandler implements the connect-go TaskServiceHandler interface.
// It handles protocol adaptation between the connect-go protocol and the service layer.
type TaskServiceHandler struct {
	svc    *service.TaskService
	logger *zap.Logger
}

// NewTaskServiceHandler creates a new TaskServiceHandler.
func NewTaskServiceHandler(svc *service.TaskService, logger *zap.Logger) *TaskServiceHandler {
	return &TaskServiceHandler{
		svc:    svc,
		logger: logger,
	}
}

// Ensure TaskServiceHandler implements connect-go handler interface.
var _ capv1connect.TaskServiceHandler = (*TaskServiceHandler)(nil)

// userContext holds user information extracted from JWT.
type userContext struct {
	userID   string
	clientID string
	claims   map[string]interface{}
}

// userContextKey is the context key for user context.
type userContextKey struct{}

// extractUserContext extracts user context from the given context.
// The JWT has already been decrypted by the auth middleware; verification is done by L3.
func extractUserContext(ctx context.Context) (*userContext, error) {
	uc, ok := ctx.Value(userContextKey{}).(*userContext)
	if !ok || uc == nil {
		return nil, errors.New("user context not found in context")
	}
	return uc, nil
}

// withUserContext adds user context to the context.
func withUserContext(ctx context.Context, uc *userContext) context.Context {
	return context.WithValue(ctx, userContextKey{}, uc)
}

// mapError maps domain errors to connect-go errors.
func mapError(err error) error {
	if err == nil {
		return nil
	}

	var appErr *domain.AppError
	if errors.As(err, &appErr) {
		switch appErr.Code {
		case domain.CodeL1DBConnect, domain.CodeL1RedisConnect:
			return connect.NewError(connect.CodeInternal, err)
		case domain.CodeL2AggregateNotFound, domain.CodeL4TaskNotFound:
			return connect.NewError(connect.CodeNotFound, err)
		case domain.CodeL2InvalidStateTransition, domain.CodeL4TaskStateInvalid:
			return connect.NewError(connect.CodeFailedPrecondition, err)
		case domain.CodeL2OptimisticLock:
			return connect.NewError(connect.CodeAborted, err)
		case domain.CodeL3AuthnFailed, domain.CodeL3TokenExpired:
			return connect.NewError(connect.CodeUnauthenticated, err)
		case domain.CodeL3AuthzDenied:
			return connect.NewError(connect.CodePermissionDenied, err)
		case domain.CodeL3RateLimited:
			return connect.NewError(connect.CodeResourceExhausted, err)
		case domain.CodeL5InvalidRequest:
			return connect.NewError(connect.CodeInvalidArgument, err)
		default:
			return connect.NewError(connect.CodeInternal, err)
		}
	}

	return connect.NewError(connect.CodeInternal, err)
}

// Submit handles the Submit RPC.
func (h *TaskServiceHandler) Submit(ctx context.Context, req *connect.Request[v1.SubmitTaskRequest]) (*connect.Response[v1.SubmitTaskResponse], error) {
	uc, err := extractUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	svcReq := service.SubmitRequest{
		Goal:                    req.Msg.Goal,
		RepositoryURL:           getRepoURL(req.Msg),
		BaseBranch:              getRepoBranch(req.Msg),
		Constraints:             req.Msg.Constraints,
		VerificationCriteria:    req.Msg.VerificationCriteria,
		Priority:                int(req.Msg.Priority),
		Tags:                    req.Msg.Tags,
		ClientID:                uc.clientID,
	}

	if hint := req.Msg.AgentHint; hint != nil {
		svcReq.AgentHint = &domain.AgentHint{
			Templates: hint.Templates,
			Model:     hint.Model,
			MaxAgents: int(hint.MaxAgents),
		}
	}

	resp, err := h.svc.Submit(ctx, svcReq)
	if err != nil {
		h.logger.Error("Submit failed",
			zap.String("layer", "L5"),
			zap.String("user_id", uc.userID),
			zap.Error(err),
		)
		return nil, mapError(err)
	}

	return connect.NewResponse(&v1.SubmitTaskResponse{
		TaskId:     resp.TaskID,
		Status:     mapTaskStatus(resp.Task.Status),
		ResultBranch: resp.Task.ResultBranch,
		CreatedAt:  resp.Task.CreatedAt.Format(time.RFC3339),
	}), nil
}

// Get handles the Get RPC.
func (h *TaskServiceHandler) Get(ctx context.Context, req *connect.Request[v1.GetTaskRequest]) (*connect.Response[v1.GetTaskResponse], error) {
	uc, err := extractUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	svcReq := service.GetRequest{
		TaskID: req.Msg.TaskId,
	}

	resp, err := h.svc.Get(ctx, svcReq)
	if err != nil {
		h.logger.Error("Get failed",
			zap.String("layer", "L5"),
			zap.String("user_id", uc.userID),
			zap.String("task_id", req.Msg.TaskId),
			zap.Error(err),
		)
		return nil, mapError(err)
	}

	return connect.NewResponse(&v1.GetTaskResponse{
		Task: mapTask(resp.Task),
	}), nil
}

// List handles the List RPC.
func (h *TaskServiceHandler) List(ctx context.Context, req *connect.Request[v1.ListTasksRequest]) (*connect.Response[v1.ListTasksResponse], error) {
	uc, err := extractUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	svcReq := service.ListRequest{
		Limit:    int(req.Msg.PageSize),
		Offset:   int((req.Msg.Page - 1) * req.Msg.PageSize),
		ClientID: uc.clientID,
	}

	if req.Msg.Status != v1.TaskStatus_TASK_STATUS_UNSPECIFIED {
		status := mapProtoTaskStatus(req.Msg.Status)
		svcReq.Status = &status
	}

	resp, err := h.svc.List(ctx, svcReq)
	if err != nil {
		h.logger.Error("List failed",
			zap.String("layer", "L5"),
			zap.String("user_id", uc.userID),
			zap.Error(err),
		)
		return nil, mapError(err)
	}

	tasks := make([]*v1.Task, len(resp.Tasks))
	for i, t := range resp.Tasks {
		tasks[i] = mapTask(t)
	}

	return connect.NewResponse(&v1.ListTasksResponse{
		Tasks:    tasks,
		Total:    int32(resp.Total),
		Page:     int32(req.Msg.Page),
		PageSize: int32(resp.Limit),
	}), nil
}

// Cancel handles the Cancel RPC.
func (h *TaskServiceHandler) Cancel(ctx context.Context, req *connect.Request[v1.CancelTaskRequest]) (*connect.Response[v1.CancelTaskResponse], error) {
	uc, err := extractUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	svcReq := service.CancelRequest{
		TaskID: req.Msg.TaskId,
		Reason: req.Msg.Reason,
	}

	resp, err := h.svc.Cancel(ctx, svcReq)
	if err != nil {
		h.logger.Error("Cancel failed",
			zap.String("layer", "L5"),
			zap.String("user_id", uc.userID),
			zap.String("task_id", req.Msg.TaskId),
			zap.Error(err),
		)
		return nil, mapError(err)
	}

	return connect.NewResponse(&v1.CancelTaskResponse{
		TaskId: resp.Task.ID,
		Status: mapTaskStatus(resp.Task.Status),
	}), nil
}

// Decide handles the Decide RPC.
// Returns Unimplemented as the service layer does not yet implement this method.
func (h *TaskServiceHandler) Decide(ctx context.Context, req *connect.Request[v1.DecideRequest]) (*connect.Response[v1.DecideResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("Decide not implemented in service layer"))
}

// GetArtifact handles the GetArtifact RPC.
// Returns Unimplemented as the service layer does not yet implement this method.
func (h *TaskServiceHandler) GetArtifact(ctx context.Context, req *connect.Request[v1.GetArtifactRequest]) (*connect.Response[v1.GetArtifactResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("GetArtifact not implemented in service layer"))
}

// GetDiff handles the GetDiff RPC.
// Returns Unimplemented as the service layer does not yet implement this method.
func (h *TaskServiceHandler) GetDiff(ctx context.Context, req *connect.Request[v1.GetDiffRequest]) (*connect.Response[v1.GetDiffResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("GetDiff not implemented in service layer"))
}

// Wait handles the Wait RPC.
// Returns Unimplemented as the service layer does not yet implement this method.
func (h *TaskServiceHandler) Wait(ctx context.Context, req *connect.Request[v1.WaitTaskRequest]) (*connect.Response[v1.WaitTaskResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("Wait not implemented in service layer"))
}

// Helper functions for mapping between proto and domain types.

func getRepoURL(req *v1.SubmitTaskRequest) string {
	if req.Repository != nil {
		return req.Repository.Url
	}
	return ""
}

func getRepoBranch(req *v1.SubmitTaskRequest) string {
	if req.Repository != nil {
		return req.Repository.Branch
	}
	return ""
}

func mapTaskStatus(status domain.TaskStatus) v1.TaskStatus {
	switch status {
	case domain.TaskStatusPending:
		return v1.TaskStatus_TASK_STATUS_PENDING
	case domain.TaskStatusDispatched:
		return v1.TaskStatus_TASK_STATUS_DISPATCHED
	case domain.TaskStatusRunning:
		return v1.TaskStatus_TASK_STATUS_RUNNING
	case domain.TaskStatusReviewing:
		return v1.TaskStatus_TASK_STATUS_REVIEWING
	case domain.TaskStatusConfirming:
		return v1.TaskStatus_TASK_STATUS_CONFIRMING
	case domain.TaskStatusCompleted:
		return v1.TaskStatus_TASK_STATUS_COMPLETED
	case domain.TaskStatusFailed:
		return v1.TaskStatus_TASK_STATUS_FAILED
	case domain.TaskStatusCancelled:
		return v1.TaskStatus_TASK_STATUS_CANCELLED
	default:
		return v1.TaskStatus_TASK_STATUS_UNSPECIFIED
	}
}

func mapProtoTaskStatus(status v1.TaskStatus) domain.TaskStatus {
	switch status {
	case v1.TaskStatus_TASK_STATUS_PENDING:
		return domain.TaskStatusPending
	case v1.TaskStatus_TASK_STATUS_DISPATCHED:
		return domain.TaskStatusDispatched
	case v1.TaskStatus_TASK_STATUS_RUNNING:
		return domain.TaskStatusRunning
	case v1.TaskStatus_TASK_STATUS_REVIEWING:
		return domain.TaskStatusReviewing
	case v1.TaskStatus_TASK_STATUS_CONFIRMING:
		return domain.TaskStatusConfirming
	case v1.TaskStatus_TASK_STATUS_COMPLETED:
		return domain.TaskStatusCompleted
	case v1.TaskStatus_TASK_STATUS_FAILED:
		return domain.TaskStatusFailed
	case v1.TaskStatus_TASK_STATUS_CANCELLED:
		return domain.TaskStatusCancelled
	default:
		return domain.TaskStatus("")
	}
}

func mapTask(task *domain.Task) *v1.Task {
	if task == nil {
		return nil
	}

	var startedAt, completedAt string
	if task.StartedAt != nil {
		startedAt = task.StartedAt.Format(time.RFC3339)
	}
	if task.CompletedAt != nil {
		completedAt = task.CompletedAt.Format(time.RFC3339)
	}

	return &v1.Task{
		TaskId:       task.ID,
		Status:       mapTaskStatus(task.Status),
		Goal:         task.Goal,
		Priority:     int32(task.Priority),
		ResultBranch: task.ResultBranch,
		Progress:     int32(task.Progress),
		CreatedAt:    task.CreatedAt.Format(time.RFC3339),
		StartedAt:    startedAt,
		CompletedAt:  completedAt,
	}
}
