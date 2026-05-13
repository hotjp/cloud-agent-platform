// Package domain implements L2-Domain layer: domain entities, state machines,
// event collection (Outbox), and business invariants.
// This layer has ZERO external dependencies - pure Go structs + standard library.
package domain

import (
	"errors"
	"fmt"
)

// ----------------------------------------------------------------------------
// Layer identifiers
// ----------------------------------------------------------------------------

// Layer represents the architectural layer that produced an error.
// See Cloud-Agent-Platform.md §十 for the full error code specification.
type Layer string

const (
	LayerStorage Layer = "L1" // L1-Storage: persistence (PostgreSQL, Redis, MinIO)
	LayerDomain  Layer = "L2" // L2-Domain: business logic, state machines
	LayerAuthz  Layer = "L3" // L3-Authz: authentication, authorization, rate limiting
	LayerService Layer = "L4" // L4-Service: orchestration, coordination
	LayerGateway Layer = "L5" // L5-Gateway: protocol adaptation, routing
)

// ----------------------------------------------------------------------------
// Error codes
// ----------------------------------------------------------------------------

// L1-Storage error codes [001, 199].
const (
	CodeL1DBConnect        = "L1001" // database connection failed
	CodeL1DBQuery          = "L1002" // database query failed
	CodeL1DBTx             = "L1003" // database transaction failed
	CodeL1RecordNotFound   = "L1004" // record does not exist
	CodeL1UniqueConstraint = "L1005" // unique constraint violation
	CodeL1ForeignKey       = "L1006" // foreign key constraint violation
	CodeL1OutboxPoll       = "L1007" // outbox polling failed
	CodeL1OutboxForward    = "L1008" // outbox event forward failed
	CodeL1RedisConnect     = "L1009" // Redis connection failed
	CodeL1RedisOp          = "L1010" // Redis operation failed
	CodeL1RedisLock        = "L1011" // distributed lock acquisition failed
	CodeL1MinIOOp          = "L1012" // MinIO operation failed
	CodeL1Migration        = "L1013" // database migration failed
)

// L2-Domain error codes [200, 399].
const (
	CodeL2InvalidStateTransition = "L1200" // illegal state machine transition
	CodeL2BusinessRuleViolated   = "L1201" // business rule validation failed
	CodeL2OptimisticLock         = "L1202" // aggregate version conflict (optimistic lock)
	CodeL2InvariantBroken        = "L1203" // domain invariant violated
	CodeL2EventSerialization    = "L1204" // domain event serialization failed
	CodeL2ParamValidation       = "L1205" // parameter validation failed
	CodeL2AggregateNotFound     = "L1206" // aggregate root not found
	CodeL2ContextExceeded       = "L1207" // context token budget exceeded
	CodeL2InvalidInput         = "L1208" // invalid input parameter
	CodeL2InvalidState         = "L1209" // invalid state for operation
	CodeL2NotFound             = "L1210" // resource not found
	CodeL2Unauthorized         = "L1211" // unauthorized access
	CodeL2OperationFailed      = "L1212" // operation failed
	CodeL2InvalidOperation     = "L1213" // operation not allowed
)

// L3-Authz error codes [400, 599].
const (
	CodeL3AuthnFailed    = "L1400" // authentication failed (JWT invalid/expired)
	CodeL3AuthzDenied    = "L1401" // authorization denied (permission check failed)
	CodeL3RateLimited    = "L1402" // rate limit exceeded
	CodeL3AuthzSvcDown   = "L1403" // authorization service unavailable
	CodeL3InvalidAPIKey  = "L1404" // API key invalid or revoked
	CodeL3TokenExpired   = "L1405" // JWT token expired
	CodeL3InsufficientQuota = "L1406" // quota exceeded
)

// L4-Service error codes [600, 799].
const (
	CodeL4TaskNotFound       = "L1600" // task does not exist
	CodeL4TaskStateInvalid   = "L1601" // task is not in a valid state for this operation
	CodeL4SubtaskNotFound    = "L1602" // subtask does not exist
	CodeL4AgentUnavailable   = "L1603" // no available agent for the required role
	CodeL4OrchestrationFailed = "L1604" // orchestration graph execution failed
	CodeL4PluginInvoke       = "L1605" // plugin invocation failed
	CodeL4ContextBudget      = "L1606" // context exceeds token budget
	CodeL4WorkerAcquire       = "L1607" // failed to acquire worker from pool
	CodeL4LLMRoute           = "L1608" // LLM routing failed
	CodeL4LLMInvoke          = "L1609" // LLM invocation failed
	CodeL4SandboxCreate      = "L1610" // sandbox creation failed
	CodeL4SandboxDestroy     = "L1611" // sandbox destruction failed
	CodeL4GitOp              = "L1612" // git operation failed
	CodeL4ToolDenied         = "L1613" // tool execution denied by security policy
	CodeL4ConfirmationTimeout = "L1614" // user confirmation timeout
	CodeL4DecomposeFailed    = "L1615" // task decomposition failed
	CodeL4AgentMatch         = "L1616" // no matching agent template found
)

// L5-Gateway error codes [800, 999].
const (
	CodeL5InvalidRequest   = "L1800" // request parameter parsing/validation failed
	CodeL5RouteNotFound    = "L1801" // RPC route does not exist
	CodeL5WSConnect        = "L1802" // WebSocket connection failed
	CodeL5WSMessage        = "L1803" // WebSocket message send/receive failed
	CodeL5ProtocolError    = "L1804" // protocol adaptation error
	CodeL5InternalError    = "L1805" // internal gateway error
	CodeL5ServiceUnavailable = "L1806" // upstream service unavailable
	CodeL5PayloadTooLarge   = "L1807" // request payload exceeds limit
	CodeL5Timeout           = "L1808" // request timeout
)

// ----------------------------------------------------------------------------
// AppError
// ----------------------------------------------------------------------------

// AppError is the canonical error type used across all five layers.
// It carries the error code, human-readable message, layer of origin,
// optional key-value details, and an underlying cause.
//
// Code is compatible with errors.Is/As for idiomatic Go error handling.
// All public functions in the domain package return AppError or errors
// that wrap it.
type AppError struct {
	Code    string            // error code, e.g. "L2200"
	Message string            // human-readable message
	Layer   Layer             // architectural layer of origin
	Details map[string]any   // structured context (optional)
	Cause   error             // underlying cause (optional, nil when no cause)
}

// Error implements the error interface.
func (e *AppError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause != nil {
		return e.Code + ": " + e.Message + " (caused by: " + e.Cause.Error() + ")"
	}
	return e.Code + ": " + e.Message
}

// Unwrap returns the wrapped cause so errors.Is/As work with standard library
// and third-party error wrapping (e.g., fmt.Errorf, os.LookupError).
func (e *AppError) Unwrap() error {
	return e.Cause
}

// Is reports whether target is the same error. Uses code-based equality
// so that any AppError with the same code is considered equal.
func (e *AppError) Is(target error) bool {
	if t, ok := target.(*AppError); ok {
		return e.Code == t.Code
	}
	return false
}

// As finds the first AppError in the error chain and copies it into target.
// When target is **AppError (the case when called from errors.As or e.As(&target)
// where target is *AppError), it walks the chain via Unwrap() starting from e's
// cause, returning the first *AppError found and true. If no *AppError is found
// in the cause chain, it returns e itself.
//
// This allows errors.As(e, &target) to find the first *AppError in the cause
// chain rather than e itself (which errors.As already handles via direct type
// assignability check).
//
// For non-*AppError targets, it delegates to errors.As to support third-party
// error types in the cause chain.
func (e *AppError) As(target any) bool {
	if t, ok := target.(**AppError); ok {
		// Walk the cause chain starting from e.Cause (not e itself),
		// following Unwrap() until we find a *AppError.
		for err := e.Cause; err != nil; {
			if appErr, ok := err.(*AppError); ok {
				*t = appErr
				return true
			}
			if uw, ok := err.(interface{ Unwrap() error }); ok {
				err = uw.Unwrap()
			} else {
				break
			}
		}
		// No AppError in cause chain; e itself is a valid AppError.
		*t = e
		return true
	}
	// Delegate to standard errors.As for third-party error types.
	if e.Cause != nil {
		return errors.As(e.Cause, target)
	}
	return false
}

// WithDetails returns a copy of e with the given key-value pairs merged into
// e.Details. This is useful for adding call-site context without losing the
// original error.
func (e *AppError) WithDetails(details map[string]any) *AppError {
	if details == nil {
		return e
	}
	merged := make(map[string]any, len(e.Details)+len(details))
	for k, v := range e.Details {
		merged[k] = v
	}
	for k, v := range details {
		merged[k] = v
	}
	return &AppError{
		Code:    e.Code,
		Message: e.Message,
		Layer:   e.Layer,
		Details: merged,
		Cause:   e.Cause,
	}
}

// WithCause returns a copy of e wrapped with the given cause error.
func (e *AppError) WithCause(cause error) *AppError {
	return &AppError{
		Code:    e.Code,
		Message: e.Message,
		Layer:   e.Layer,
		Details: e.Details,
		Cause:   cause,
	}
}

// CodeIs is a convenience helper that checks whether err (or any error in its
// chain) has the given code. Equivalent to errors.Is(err, &AppError{Code: code}).
func CodeIs(err error, code string) bool {
	return errors.Is(err, &AppError{Code: code})
}

// CodeAs is a convenience helper that finds an AppError with the given code
// in err's chain. Returns nil if not found.
func CodeAs(err error, code string) *AppError {
	var appErr *AppError
	if errors.As(err, &appErr) && appErr.Code == code {
		return appErr
	}
	return nil
}

// ----------------------------------------------------------------------------
// Sentinel Errors
// These are used for errors.Is/As checks across the codebase.
// ----------------------------------------------------------------------------

// ErrInvalidInput indicates an invalid input parameter.
var ErrInvalidInput = errors.New("invalid input")

// ErrInvalidState indicates the entity is in an invalid state for this operation.
var ErrInvalidState = errors.New("invalid state")

// ErrNotFound indicates a required resource was not found.
var ErrNotFound = errors.New("not found")

// ErrUnauthorized indicates the operation is not authorized.
var ErrUnauthorized = errors.New("unauthorized")

// ErrOperationFailed indicates a general operation failure.
var ErrOperationFailed = errors.New("operation failed")

// ErrInvalidOperation indicates the operation is not allowed.
var ErrInvalidOperation = errors.New("operation not allowed")

// ----------------------------------------------------------------------------
// L1-Storage constructors
// These are placed here because the domain package owns the shared error type,
// but they are thin wrappers that use only the standard library.
// Callers in the storage layer should import this package.
// ----------------------------------------------------------------------------

// NewL1DBConnectError creates an error for a failed database connection.
func NewL1DBConnectError(cause error) *AppError {
	return &AppError{Code: CodeL1DBConnect, Message: "database connection failed", Layer: LayerStorage, Cause: cause}
}

// NewL1DBQueryError creates an error for a failed database query.
func NewL1DBQueryError(cause error) *AppError {
	return &AppError{Code: CodeL1DBQuery, Message: "database query failed", Layer: LayerStorage, Cause: cause}
}

// NewL1DBTxError creates an error for a failed database transaction.
func NewL1DBTxError(cause error) *AppError {
	return &AppError{Code: CodeL1DBTx, Message: "database transaction failed", Layer: LayerStorage, Cause: cause}
}

// NewL1RecordNotFoundError creates an error when a required record does not exist.
func NewL1RecordNotFoundError(entity, id string) *AppError {
	return &AppError{
		Code:    CodeL1RecordNotFound,
		Message: fmt.Sprintf("%s record not found: %s", entity, id),
		Layer:   LayerStorage,
		Details: map[string]any{"entity": entity, "id": id},
	}
}

// NewL1UniqueConstraintError creates an error for a unique constraint violation.
func NewL1UniqueConstraintError(field, value string) *AppError {
	return &AppError{
		Code:    CodeL1UniqueConstraint,
		Message: fmt.Sprintf("unique constraint violated on field: %s", field),
		Layer:   LayerStorage,
		Details: map[string]any{"field": field, "value": value},
	}
}

// NewL1ForeignKeyError creates an error for a foreign key constraint violation.
func NewL1ForeignKeyError(field, value string) *AppError {
	return &AppError{
		Code:    CodeL1ForeignKey,
		Message: fmt.Sprintf("foreign key constraint violated: %s", field),
		Layer:   LayerStorage,
		Details: map[string]any{"field": field, "value": value},
	}
}

// NewL1OutboxPollError creates an error when outbox polling fails.
func NewL1OutboxPollError(cause error) *AppError {
	return &AppError{Code: CodeL1OutboxPoll, Message: "outbox polling failed", Layer: LayerStorage, Cause: cause}
}

// NewL1OutboxForwardError creates an error when outbox event forwarding fails.
func NewL1OutboxForwardError(eventID string, cause error) *AppError {
	return &AppError{
		Code:    CodeL1OutboxForward,
		Message: fmt.Sprintf("outbox event forward failed: %s", eventID),
		Layer:   LayerStorage,
		Details: map[string]any{"event_id": eventID},
		Cause:   cause,
	}
}

// NewL1RedisConnectError creates an error for a failed Redis connection.
func NewL1RedisConnectError(cause error) *AppError {
	return &AppError{Code: CodeL1RedisConnect, Message: "Redis connection failed", Layer: LayerStorage, Cause: cause}
}

// NewL1RedisOpError creates an error for a failed Redis operation.
func NewL1RedisOpError(op string, cause error) *AppError {
	return &AppError{
		Code:    CodeL1RedisOp,
		Message: fmt.Sprintf("Redis %s operation failed", op),
		Layer:   LayerStorage,
		Details: map[string]any{"operation": op},
		Cause:   cause,
	}
}

// NewL1RedisLockError creates an error when a distributed lock cannot be acquired.
func NewL1RedisLockError(key string, cause error) *AppError {
	return &AppError{
		Code:    CodeL1RedisLock,
		Message: fmt.Sprintf("distributed lock acquisition failed: %s", key),
		Layer:   LayerStorage,
		Details: map[string]any{"lock_key": key},
		Cause:   cause,
	}
}

// NewL1MinIOOpError creates an error for a failed MinIO operation.
func NewL1MinIOOpError(op string, cause error) *AppError {
	return &AppError{
		Code:    CodeL1MinIOOp,
		Message: fmt.Sprintf("MinIO %s operation failed", op),
		Layer:   LayerStorage,
		Details: map[string]any{"operation": op},
		Cause:   cause,
	}
}

// NewL1MigrationError creates an error for a failed database migration.
func NewL1MigrationError(cause error) *AppError {
	return &AppError{Code: CodeL1Migration, Message: "database migration failed", Layer: LayerStorage, Cause: cause}
}

// ----------------------------------------------------------------------------
// L2-Domain constructors (zero external dependencies)
// These use only errors.New and string operations.
// ----------------------------------------------------------------------------

// NewL2InvalidStateTransitionError creates an error for an illegal state transition.
func NewL2InvalidStateTransitionError(entity string, from, to TaskStatus) *AppError {
	return &AppError{
		Code:    CodeL2InvalidStateTransition,
		Message: fmt.Sprintf("illegal state transition for %s: %s -> %s", entity, from, to),
		Layer:   LayerDomain,
		Details: map[string]any{"entity": entity, "from_state": string(from), "to_state": string(to)},
	}
}

// NewL2BusinessRuleViolatedError creates an error when a business rule check fails.
func NewL2BusinessRuleViolatedError(rule string, details map[string]any) *AppError {
	return &AppError{
		Code:    CodeL2BusinessRuleViolated,
		Message: fmt.Sprintf("business rule violated: %s", rule),
		Layer:   LayerDomain,
		Details: details,
	}
}

// NewL2OptimisticLockError creates an error for an aggregate version conflict.
func NewL2OptimisticLockError(entity, id string, expected, actual int64) *AppError {
	return &AppError{
		Code:    CodeL2OptimisticLock,
		Message: fmt.Sprintf("optimistic lock conflict for %s %s: expected version %d, found %d", entity, id, expected, actual),
		Layer:   LayerDomain,
		Details: map[string]any{"entity": entity, "id": id, "expected_version": expected, "actual_version": actual},
	}
}

// NewL2InvariantBrokenError creates an error when a domain invariant is violated.
func NewL2InvariantBrokenError(invariant string, details map[string]any) *AppError {
	return &AppError{
		Code:    CodeL2InvariantBroken,
		Message: fmt.Sprintf("domain invariant broken: %s", invariant),
		Layer:   LayerDomain,
		Details: details,
	}
}

// NewL2EventSerializationError creates an error when domain event serialization fails.
func NewL2EventSerializationError(eventType string, cause error) *AppError {
	return &AppError{
		Code:    CodeL2EventSerialization,
		Message: fmt.Sprintf("domain event serialization failed: %s", eventType),
		Layer:   LayerDomain,
		Details: map[string]any{"event_type": eventType},
		Cause:   cause,
	}
}

// NewL2ParamValidationError creates an error for a domain-level parameter validation failure.
func NewL2ParamValidationError(field, reason string) *AppError {
	return &AppError{
		Code:    CodeL2ParamValidation,
		Message: fmt.Sprintf("parameter validation failed for %s: %s", field, reason),
		Layer:   LayerDomain,
		Details: map[string]any{"field": field, "reason": reason},
	}
}

// NewL2AggregateNotFoundError creates an error when an aggregate root is not found.
func NewL2AggregateNotFoundError(aggregateType, id string) *AppError {
	return &AppError{
		Code:    CodeL2AggregateNotFound,
		Message: fmt.Sprintf("%s aggregate not found: %s", aggregateType, id),
		Layer:   LayerDomain,
		Details: map[string]any{"aggregate_type": aggregateType, "id": id},
	}
}

// NewL2ContextExceededError creates an error when the context token budget is exceeded.
func NewL2ContextExceededError(budget, used int) *AppError {
	return &AppError{
		Code:    CodeL2ContextExceeded,
		Message: fmt.Sprintf("context token budget exceeded: %d used, budget %d", used, budget),
		Layer:   LayerDomain,
		Details: map[string]any{"budget": budget, "used": used},
	}
}

// NewL2InvalidInputError creates an error for an invalid input parameter.
func NewL2InvalidInputError(field, reason string) *AppError {
	return &AppError{
		Code:    CodeL2InvalidInput,
		Message: fmt.Sprintf("invalid input: %s (%s)", field, reason),
		Layer:   LayerDomain,
		Details: map[string]any{"field": field, "reason": reason},
	}
}

// NewL2InvalidStateError creates an error when an entity is in an invalid state.
func NewL2InvalidStateError(entity, state, reason string) *AppError {
	return &AppError{
		Code:    CodeL2InvalidState,
		Message: fmt.Sprintf("invalid state for %s: %s (%s)", entity, state, reason),
		Layer:   LayerDomain,
		Details: map[string]any{"entity": entity, "state": state, "reason": reason},
	}
}

// NewL2NotFoundError creates an error when a resource is not found.
func NewL2NotFoundError(resourceType, id string) *AppError {
	return &AppError{
		Code:    CodeL2NotFound,
		Message: fmt.Sprintf("%s not found: %s", resourceType, id),
		Layer:   LayerDomain,
		Details: map[string]any{"type": resourceType, "id": id},
	}
}

// NewL2UnauthorizedError creates an error for unauthorized access.
func NewL2UnauthorizedError(reason string) *AppError {
	return &AppError{
		Code:    CodeL2Unauthorized,
		Message: fmt.Sprintf("unauthorized: %s", reason),
		Layer:   LayerDomain,
		Details: map[string]any{"reason": reason},
	}
}

// NewL2OperationFailedError creates an error for a general operation failure.
func NewL2OperationFailedError(op string, cause error) *AppError {
	return &AppError{
		Code:    CodeL2OperationFailed,
		Message: fmt.Sprintf("operation failed: %s", op),
		Layer:   LayerDomain,
		Details: map[string]any{"operation": op},
		Cause:   cause,
	}
}

// NewL2InvalidOperationError creates an error when an operation is not allowed.
func NewL2InvalidOperationError(op, reason string) *AppError {
	return &AppError{
		Code:    CodeL2InvalidOperation,
		Message: fmt.Sprintf("operation not allowed: %s (%s)", op, reason),
		Layer:   LayerDomain,
		Details: map[string]any{"operation": op, "reason": reason},
	}
}

// ----------------------------------------------------------------------------
// L3-Authz constructors
// ----------------------------------------------------------------------------

// NewL3AuthnFailedError creates an error for an authentication failure.
func NewL3AuthnFailedError(reason string) *AppError {
	return &AppError{
		Code:    CodeL3AuthnFailed,
		Message: fmt.Sprintf("authentication failed: %s", reason),
		Layer:   LayerAuthz,
		Details: map[string]any{"reason": reason},
	}
}

// NewL3AuthzDeniedError creates an error for an authorization denial.
func NewL3AuthzDeniedError(permission string) *AppError {
	return &AppError{
		Code:    CodeL3AuthzDenied,
		Message: fmt.Sprintf("authorization denied: %s", permission),
		Layer:   LayerAuthz,
		Details: map[string]any{"permission": permission},
	}
}

// NewL3RateLimitedError creates an error when rate limit is exceeded.
func NewL3RateLimitedError(limitType string, retryAfter int) *AppError {
	return &AppError{
		Code:    CodeL3RateLimited,
		Message: fmt.Sprintf("rate limit exceeded: %s", limitType),
		Layer:   LayerAuthz,
		Details: map[string]any{"limit_type": limitType, "retry_after_seconds": retryAfter},
	}
}

// NewL3AuthzSvcDownError creates an error when the authorization service is unavailable.
func NewL3AuthzSvcDownError(cause error) *AppError {
	return &AppError{Code: CodeL3AuthzSvcDown, Message: "authorization service unavailable", Layer: LayerAuthz, Cause: cause}
}

// NewL3InvalidAPIKeyError creates an error for an invalid or revoked API key.
func NewL3InvalidAPIKeyError() *AppError {
	return &AppError{Code: CodeL3InvalidAPIKey, Message: "API key invalid or revoked", Layer: LayerAuthz}
}

// NewL3TokenExpiredError creates an error for an expired JWT token.
func NewL3TokenExpiredError() *AppError {
	return &AppError{Code: CodeL3TokenExpired, Message: "JWT token expired", Layer: LayerAuthz}
}

// NewL3InsufficientQuotaError creates an error when a quota is exceeded.
func NewL3InsufficientQuotaError(quotaType string, used, limit int) *AppError {
	return &AppError{
		Code:    CodeL3InsufficientQuota,
		Message: fmt.Sprintf("quota exceeded: %s (%d / %d)", quotaType, used, limit),
		Layer:   LayerAuthz,
		Details: map[string]any{"quota_type": quotaType, "used": used, "limit": limit},
	}
}

// ----------------------------------------------------------------------------
// L4-Service constructors
// ----------------------------------------------------------------------------

// NewL4TaskNotFoundError creates an error when a task does not exist.
func NewL4TaskNotFoundError(taskID string) *AppError {
	return &AppError{
		Code:    CodeL4TaskNotFound,
		Message: fmt.Sprintf("task not found: %s", taskID),
		Layer:   LayerService,
		Details: map[string]any{"task_id": taskID},
	}
}

// NewL4TaskStateInvalidError creates an error when a task is not in a valid state.
func NewL4TaskStateInvalidError(taskID string, currentStatus TaskStatus, expected []TaskStatus) *AppError {
	return &AppError{
		Code:    CodeL4TaskStateInvalid,
		Message: fmt.Sprintf("task %s is in state %s, expected one of %v", taskID, currentStatus, expected),
		Layer:   LayerService,
		Details: map[string]any{
			"task_id":         taskID,
			"current_status":  string(currentStatus),
			"expected_states":  expected,
		},
	}
}

// NewL4SubtaskNotFoundError creates an error when a subtask does not exist.
func NewL4SubtaskNotFoundError(subtaskID string) *AppError {
	return &AppError{
		Code:    CodeL4SubtaskNotFound,
		Message: fmt.Sprintf("subtask not found: %s", subtaskID),
		Layer:   LayerService,
		Details: map[string]any{"subtask_id": subtaskID},
	}
}

// NewL4AgentUnavailableError creates an error when no agent is available.
func NewL4AgentUnavailableError(role AgentRole) *AppError {
	return &AppError{
		Code:    CodeL4AgentUnavailable,
		Message: fmt.Sprintf("no available agent for role: %s", role),
		Layer:   LayerService,
		Details: map[string]any{"role": string(role)},
	}
}

// NewL4OrchestrationFailedError creates an error when orchestration fails.
func NewL4OrchestrationFailedError(reason string, cause error) *AppError {
	return &AppError{
		Code:    CodeL4OrchestrationFailed,
		Message: fmt.Sprintf("orchestration failed: %s", reason),
		Layer:   LayerService,
		Details: map[string]any{"reason": reason},
		Cause:   cause,
	}
}

// NewL4PluginInvokeError creates an error when a plugin invocation fails.
func NewL4PluginInvokeError(plugin, method string, cause error) *AppError {
	return &AppError{
		Code:    CodeL4PluginInvoke,
		Message: fmt.Sprintf("plugin %s.%s invocation failed", plugin, method),
		Layer:   LayerService,
		Details: map[string]any{"plugin": plugin, "method": method},
		Cause:   cause,
	}
}

// NewL4ContextBudgetError creates an error when context exceeds token budget.
func NewL4ContextBudgetError(taskID string, budget, used int) *AppError {
	return &AppError{
		Code:    CodeL4ContextBudget,
		Message: fmt.Sprintf("task %s context exceeds token budget: %d > %d", taskID, used, budget),
		Layer:   LayerService,
		Details: map[string]any{"task_id": taskID, "budget": budget, "used": used},
	}
}

// NewL4WorkerAcquireError creates an error when a worker cannot be acquired.
func NewL4WorkerAcquireError(cause error) *AppError {
	return &AppError{Code: CodeL4WorkerAcquire, Message: "failed to acquire worker from pool", Layer: LayerService, Cause: cause}
}

// NewL4LLMRouteError creates an error when LLM routing fails.
func NewL4LLMRouteError(taskType SubtaskType, reason string) *AppError {
	return &AppError{
		Code:    CodeL4LLMRoute,
		Message: fmt.Sprintf("LLM routing failed for task type %s: %s", taskType, reason),
		Layer:   LayerService,
		Details: map[string]any{"task_type": string(taskType), "reason": reason},
	}
}

// NewL4LLMInvokeError creates an error when LLM invocation fails.
func NewL4LLMInvokeError(model string, cause error) *AppError {
	return &AppError{
		Code:    CodeL4LLMInvoke,
		Message: fmt.Sprintf("LLM invocation failed (model: %s)", model),
		Layer:   LayerService,
		Details: map[string]any{"model": model},
		Cause:   cause,
	}
}

// NewL4SandboxCreateError creates an error when sandbox creation fails.
func NewL4SandboxCreateError(backend string, cause error) *AppError {
	return &AppError{
		Code:    CodeL4SandboxCreate,
		Message: fmt.Sprintf("sandbox creation failed (backend: %s)", backend),
		Layer:   LayerService,
		Details: map[string]any{"backend": backend},
		Cause:   cause,
	}
}

// NewL4SandboxDestroyError creates an error when sandbox destruction fails.
func NewL4SandboxDestroyError(workerID string, cause error) *AppError {
	return &AppError{
		Code:    CodeL4SandboxDestroy,
		Message: fmt.Sprintf("sandbox destruction failed (worker: %s)", workerID),
		Layer:   LayerService,
		Details: map[string]any{"worker_id": workerID},
		Cause:   cause,
	}
}

// NewL4GitOpError creates an error for a git operation failure.
func NewL4GitOpError(op, repo string, cause error) *AppError {
	return &AppError{
		Code:    CodeL4GitOp,
		Message: fmt.Sprintf("git %s failed for %s", op, repo),
		Layer:   LayerService,
		Details: map[string]any{"operation": op, "repository": repo},
		Cause:   cause,
	}
}

// NewL4ToolDeniedError creates an error when tool execution is denied.
func NewL4ToolDeniedError(toolName, reason string) *AppError {
	return &AppError{
		Code:    CodeL4ToolDenied,
		Message: fmt.Sprintf("tool execution denied: %s (%s)", toolName, reason),
		Layer:   LayerService,
		Details: map[string]any{"tool_name": toolName, "reason": reason},
	}
}

// NewL4ConfirmationTimeoutError creates an error when user confirmation times out.
func NewL4ConfirmationTimeoutError(taskID string, timeoutSeconds int) *AppError {
	return &AppError{
		Code:    CodeL4ConfirmationTimeout,
		Message: fmt.Sprintf("user confirmation timeout for task %s after %d seconds", taskID, timeoutSeconds),
		Layer:   LayerService,
		Details: map[string]any{"task_id": taskID, "timeout_seconds": timeoutSeconds},
	}
}

// NewL4DecomposeFailedError creates an error when task decomposition fails.
func NewL4DecomposeFailedError(taskID string, reason string, cause error) *AppError {
	return &AppError{
		Code:    CodeL4DecomposeFailed,
		Message: fmt.Sprintf("task decomposition failed for %s: %s", taskID, reason),
		Layer:   LayerService,
		Details: map[string]any{"task_id": taskID, "reason": reason},
		Cause:   cause,
	}
}

// NewL4AgentMatchError creates an error when no matching agent template is found.
func NewL4AgentMatchError(subtaskType SubtaskType) *AppError {
	return &AppError{
		Code:    CodeL4AgentMatch,
		Message: fmt.Sprintf("no matching agent template found for subtask type: %s", subtaskType),
		Layer:   LayerService,
		Details: map[string]any{"subtask_type": string(subtaskType)},
	}
}

// ----------------------------------------------------------------------------
// L5-Gateway constructors
// ----------------------------------------------------------------------------

// NewL5InvalidRequestError creates an error for an invalid request.
func NewL5InvalidRequestError(field, reason string) *AppError {
	return &AppError{
		Code:    CodeL5InvalidRequest,
		Message: fmt.Sprintf("invalid request: %s (%s)", field, reason),
		Layer:   LayerGateway,
		Details: map[string]any{"field": field, "reason": reason},
	}
}

// NewL5RouteNotFoundError creates an error when an RPC route does not exist.
func NewL5RouteNotFoundError(method string) *AppError {
	return &AppError{
		Code:    CodeL5RouteNotFound,
		Message: fmt.Sprintf("route not found: %s", method),
		Layer:   LayerGateway,
		Details: map[string]any{"method": method},
	}
}

// NewL5WSConnectError creates an error when a WebSocket connection fails.
func NewL5WSConnectError(cause error) *AppError {
	return &AppError{Code: CodeL5WSConnect, Message: "WebSocket connection failed", Layer: LayerGateway, Cause: cause}
}

// NewL5WSMessageError creates an error when a WebSocket message operation fails.
func NewL5WSMessageError(room, action string, cause error) *AppError {
	return &AppError{
		Code:    CodeL5WSMessage,
		Message: fmt.Sprintf("WebSocket %s failed for room %s", action, room),
		Layer:   LayerGateway,
		Details: map[string]any{"room": room, "action": action},
		Cause:   cause,
	}
}

// NewL5ProtocolError creates an error for a protocol adaptation failure.
func NewL5ProtocolError(detail string, cause error) *AppError {
	return &AppError{
		Code:    CodeL5ProtocolError,
		Message: fmt.Sprintf("protocol adaptation error: %s", detail),
		Layer:   LayerGateway,
		Details: map[string]any{"detail": detail},
		Cause:   cause,
	}
}

// NewL5InternalError creates an error for an internal gateway error.
func NewL5InternalError(cause error) *AppError {
	return &AppError{Code: CodeL5InternalError, Message: "internal gateway error", Layer: LayerGateway, Cause: cause}
}

// NewL5ServiceUnavailableError creates an error when an upstream service is unavailable.
func NewL5ServiceUnavailableError(service string, cause error) *AppError {
	return &AppError{
		Code:    CodeL5ServiceUnavailable,
		Message: fmt.Sprintf("upstream service unavailable: %s", service),
		Layer:   LayerGateway,
		Details: map[string]any{"service": service},
		Cause:   cause,
	}
}

// NewL5PayloadTooLargeError creates an error when the request payload exceeds the limit.
func NewL5PayloadTooLargeError(size, limit int64) *AppError {
	return &AppError{
		Code:    CodeL5PayloadTooLarge,
		Message: fmt.Sprintf("request payload too large: %d bytes (limit: %d)", size, limit),
		Layer:   LayerGateway,
		Details: map[string]any{"size": size, "limit": limit},
	}
}

// NewL5TimeoutError creates an error for a request timeout.
func NewL5TimeoutError(operation string) *AppError {
	return &AppError{
		Code:    CodeL5Timeout,
		Message: fmt.Sprintf("request timeout during: %s", operation),
		Layer:   LayerGateway,
		Details: map[string]any{"operation": operation},
	}
}
