package domain

import (
	"errors"
	"fmt"
	"testing"
)

func TestAppError_Error(t *testing.T) {
	t.Run("without cause", func(t *testing.T) {
		e := &AppError{Code: "L2200", Message: "illegal state transition", Layer: LayerDomain}
		got := e.Error()
		want := "L2200: illegal state transition"
		if got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
	})

	t.Run("with cause", func(t *testing.T) {
		cause := errors.New("connection reset")
		e := &AppError{Code: "L1001", Message: "database connection failed", Layer: LayerStorage, Cause: cause}
		got := e.Error()
		want := "L1001: database connection failed (caused by: connection reset)"
		if got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
	})

	t.Run("nil error", func(t *testing.T) {
		var e *AppError
		got := e.Error()
		if got != "" {
			t.Errorf("Error() on nil = %q, want empty string", got)
		}
	})
}

func TestAppError_Unwrap(t *testing.T) {
	t.Run("returns cause", func(t *testing.T) {
		cause := errors.New("underlying error")
		e := &AppError{Code: "L4600", Message: "task not found", Layer: LayerService, Cause: cause}
		if e.Unwrap() != cause {
			t.Errorf("Unwrap() = %v, want %v", e.Unwrap(), cause)
		}
	})

	t.Run("nil cause returns nil", func(t *testing.T) {
		e := &AppError{Code: "L2200", Message: "test", Layer: LayerDomain}
		if e.Unwrap() != nil {
			t.Errorf("Unwrap() = %v, want nil", e.Unwrap())
		}
	})
}

func TestAppError_Is(t *testing.T) {
	t.Run("same code is equal", func(t *testing.T) {
		e1 := &AppError{Code: "L2200", Message: "msg1", Layer: LayerDomain}
		e2 := &AppError{Code: "L2200", Message: "msg2", Layer: LayerDomain}
		if !e1.Is(e2) {
			t.Errorf("Is(e2) with same code = false, want true")
		}
	})

	t.Run("different code is not equal", func(t *testing.T) {
		e1 := &AppError{Code: "L2200", Message: "msg", Layer: LayerDomain}
		e2 := &AppError{Code: "L2201", Message: "msg", Layer: LayerDomain}
		if e1.Is(e2) {
			t.Errorf("Is(e2) with different code = true, want false")
		}
	})

	t.Run("matches via errors.Is", func(t *testing.T) {
		e := NewL2InvalidStateTransitionError("Task", TaskStatusPending, TaskStatusRunning)
		target := &AppError{Code: CodeL2InvalidStateTransition}
		if !errors.Is(e, target) {
			t.Errorf("errors.Is(e, target) = false, want true")
		}
	})

	t.Run("non-AppError target checks cause chain", func(t *testing.T) {
		cause := errors.New("some error")
		e := &AppError{Code: "L1001", Message: "db fail", Layer: LayerStorage, Cause: cause}
		// errors.Is follows the Unwrap chain, so it finds cause
		if !errors.Is(e, cause) {
			t.Errorf("errors.Is(e, cause) = false, want true (standard Is follows Unwrap chain)")
		}
	})
}

func TestAppError_As(t *testing.T) {
	t.Run("finds AppError in chain", func(t *testing.T) {
		inner := &AppError{Code: "L1001", Message: "inner", Layer: LayerStorage}
		e := &AppError{Code: "L4600", Message: "outer", Layer: LayerService, Cause: inner}
		var target *AppError
		// errors.As traverses the chain and finds the FIRST *AppError: inner (L1001)
		if !e.As(&target) {
			t.Fatalf("As(&target) = false, want true")
		}
		if target.Code != "L1001" {
			t.Errorf("target.Code = %q, want %q (first AppError in chain)", target.Code, "L1001")
		}
	})

	t.Run("copies AppError fields", func(t *testing.T) {
		e := &AppError{
			Code:    "L2201",
			Message: "business rule violated",
			Layer:   LayerDomain,
			Details: map[string]any{"rule": "max_retries"},
		}
		var target *AppError
		if !e.As(&target) {
			t.Fatalf("As(&target) = false, want true")
		}
		if target.Code != "L2201" || target.Message != "business rule violated" || target.Layer != LayerDomain {
			t.Errorf("target = %+v, want Code=L2201, Message=business rule violated, Layer=L2", target)
		}
		// Details is a map, verify it was shallow-copied (same reference is fine for our use case)
		if target.Details["rule"] != "max_retries" {
			t.Errorf("target.Details[rule] = %v, want %v", target.Details["rule"], "max_retries")
		}
	})

	t.Run("delegates to cause chain", func(t *testing.T) {
		// When cause is a plain error (not *AppError), As should still return e itself
		// since e is a valid *AppError in the chain.
		cause := errors.New("network error")
		e := &AppError{Code: "L1001", Message: "db fail", Layer: LayerStorage, Cause: cause}
		var target *AppError
		if !e.As(&target) {
			t.Fatalf("As(&target) = false, want true")
		}
		// e itself is found (no *AppError in cause chain beyond e)
		if target.Code != "L1001" {
			t.Errorf("target.Code = %q, want L1001", target.Code)
		}
	})
}

func TestAppError_WithDetails(t *testing.T) {
	t.Run("merges details", func(t *testing.T) {
		e := &AppError{
			Code:    "L2200",
			Message: "state transition failed",
			Layer:   LayerDomain,
			Details: map[string]any{"from": "pending"},
		}
		e2 := e.WithDetails(map[string]any{"to": "running"})
		if e2 == e {
			t.Errorf("WithDetails should return a new error")
		}
		if e2.Details["from"] != "pending" {
			t.Errorf("e2.Details[from] = %v, want pending", e2.Details["from"])
		}
		if e2.Details["to"] != "running" {
			t.Errorf("e2.Details[to] = %v, want running", e2.Details["to"])
		}
		// original unchanged
		if e.Details["to"] != nil {
			t.Errorf("original Details[to] = %v, want nil", e.Details["to"])
		}
	})

	t.Run("overwrites existing key", func(t *testing.T) {
		e := &AppError{
			Code:    "L2200",
			Message: "state transition failed",
			Layer:   LayerDomain,
			Details: map[string]any{"attempt": 1},
		}
		e2 := e.WithDetails(map[string]any{"attempt": 2})
		if e2.Details["attempt"] != 2 {
			t.Errorf("e2.Details[attempt] = %v, want 2", e2.Details["attempt"])
		}
	})

	t.Run("nil details returns identical copy", func(t *testing.T) {
		e := &AppError{Code: "L2200", Message: "test", Layer: LayerDomain}
		e2 := e.WithDetails(nil)
		if e2 != e {
			t.Errorf("WithDetails(nil) should return same instance")
		}
	})
}

func TestAppError_WithCause(t *testing.T) {
	cause := errors.New("original error")
	e := NewL4TaskNotFoundError("task_abc")
	e2 := e.WithCause(cause)

	if e2 == e {
		t.Errorf("WithCause should return a new error")
	}
	if e2.Cause != cause {
		t.Errorf("e2.Cause = %v, want %v", e2.Cause, cause)
	}
	if e2.Code != e.Code || e2.Message != e.Message || e2.Layer != e.Layer {
		t.Errorf("e2 fields changed unexpectedly")
	}
	// original unchanged
	if e.Cause != nil {
		t.Errorf("original cause = %v, want nil", e.Cause)
	}
}

func TestCodeIs(t *testing.T) {
	e := NewL2InvalidStateTransitionError("Task", TaskStatusPending, TaskStatusRunning)
	if !CodeIs(e, CodeL2InvalidStateTransition) {
		t.Errorf("CodeIs(e, L2200) = false, want true")
	}
	if CodeIs(e, CodeL2BusinessRuleViolated) {
		t.Errorf("CodeIs(e, L2201) = true, want false")
	}
}

func TestCodeAs(t *testing.T) {
	e := NewL4TaskNotFoundError("task_xyz")
	found := CodeAs(e, CodeL4TaskNotFound)
	if found == nil {
		t.Fatalf("CodeAs(e, L4600) = nil, want non-nil")
	}
	if found.Code != CodeL4TaskNotFound {
		t.Errorf("found.Code = %q, want %q", found.Code, CodeL4TaskNotFound)
	}

	notFound := CodeAs(e, CodeL2InvalidStateTransition)
	if notFound != nil {
		t.Errorf("CodeAs(e, L2200) = %v, want nil", notFound)
	}
}

func TestLayerConstants(t *testing.T) {
	if LayerStorage != "L1" {
		t.Errorf("LayerStorage = %q, want L1", LayerStorage)
	}
	if LayerDomain != "L2" {
		t.Errorf("LayerDomain = %q, want L2", LayerDomain)
	}
	if LayerAuthz != "L3" {
		t.Errorf("LayerAuthz = %q, want L3", LayerAuthz)
	}
	if LayerService != "L4" {
		t.Errorf("LayerService = %q, want L4", LayerService)
	}
	if LayerGateway != "L5" {
		t.Errorf("LayerGateway = %q, want L5", LayerGateway)
	}
}

// ----------------------------------------------------------------------------
// Constructor tests
// ----------------------------------------------------------------------------

func TestL1Constructors(t *testing.T) {
	t.Run("NewL1DBConnectError", func(t *testing.T) {
		cause := errors.New("connection refused")
		e := NewL1DBConnectError(cause)
		checkAppError(t, e, CodeL1DBConnect, "database connection failed", LayerStorage, cause)
	})

	t.Run("NewL1RecordNotFoundError", func(t *testing.T) {
		e := NewL1RecordNotFoundError("Task", "task_abc")
		checkAppError(t, e, CodeL1RecordNotFound, "Task record not found: task_abc", LayerStorage, nil)
		if e.Details["entity"] != "Task" || e.Details["id"] != "task_abc" {
			t.Errorf("Details = %v, want entity=Task, id=task_abc", e.Details)
		}
	})

	t.Run("NewL1UniqueConstraintError", func(t *testing.T) {
		e := NewL1UniqueConstraintError("client_id", "cli_xyz")
		checkAppError(t, e, CodeL1UniqueConstraint, "unique constraint violated on field: client_id", LayerStorage, nil)
	})

	t.Run("NewL1RedisLockError", func(t *testing.T) {
		e := NewL1RedisLockError("cap:lock:task_abc", nil)
		checkAppError(t, e, CodeL1RedisLock, "distributed lock acquisition failed: cap:lock:task_abc", LayerStorage, nil)
		if e.Details["lock_key"] != "cap:lock:task_abc" {
			t.Errorf("Details[lock_key] = %v", e.Details["lock_key"])
		}
	})
}

func TestL2Constructors(t *testing.T) {
	t.Run("NewL2InvalidStateTransitionError", func(t *testing.T) {
		e := NewL2InvalidStateTransitionError("Task", TaskStatusPending, TaskStatusRunning)
		checkAppError(t, e, CodeL2InvalidStateTransition, "illegal state transition for Task: pending -> running", LayerDomain, nil)
		if e.Details["from_state"] != "pending" || e.Details["to_state"] != "running" {
			t.Errorf("Details = %v", e.Details)
		}
	})

	t.Run("NewL2BusinessRuleViolatedError", func(t *testing.T) {
		e := NewL2BusinessRuleViolatedError("max_retries", map[string]any{"retry": 5, "max": 3})
		checkAppError(t, e, CodeL2BusinessRuleViolated, "business rule violated: max_retries", LayerDomain, nil)
	})

	t.Run("NewL2OptimisticLockError", func(t *testing.T) {
		e := NewL2OptimisticLockError("Task", "task_abc", 3, 4)
		checkAppError(t, e, CodeL2OptimisticLock, "optimistic lock conflict for Task task_abc: expected version 3, found 4", LayerDomain, nil)
	})

	t.Run("NewL2EventSerializationError", func(t *testing.T) {
		cause := errors.New("json: unsupported type")
		e := NewL2EventSerializationError("TaskSubmittedV1", cause)
		checkAppError(t, e, CodeL2EventSerialization, "domain event serialization failed: TaskSubmittedV1", LayerDomain, cause)
	})

	t.Run("NewL2ContextExceededError", func(t *testing.T) {
		e := NewL2ContextExceededError(50000, 52341)
		checkAppError(t, e, CodeL2ContextExceeded, "context token budget exceeded: 52341 used, budget 50000", LayerDomain, nil)
	})
}

func TestL3Constructors(t *testing.T) {
	t.Run("NewL3AuthnFailedError", func(t *testing.T) {
		e := NewL3AuthnFailedError("JWT signature invalid")
		checkAppError(t, e, "L1400", "authentication failed: JWT signature invalid", LayerAuthz, nil)
	})

	t.Run("NewL3AuthzDeniedError", func(t *testing.T) {
		e := NewL3AuthzDeniedError("task:cancel")
		checkAppError(t, e, "L1401", "authorization denied: task:cancel", LayerAuthz, nil)
	})

	t.Run("NewL3RateLimitedError", func(t *testing.T) {
		e := NewL3RateLimitedError("client_ip:10/s", 30)
		checkAppError(t, e, "L1402", "rate limit exceeded: client_ip:10/s", LayerAuthz, nil)
		if e.Details["retry_after_seconds"] != 30 {
			t.Errorf("Details[retry_after_seconds] = %v", e.Details["retry_after_seconds"])
		}
	})

	t.Run("NewL3InsufficientQuotaError", func(t *testing.T) {
		e := NewL3InsufficientQuotaError("monthly_tasks", 150, 100)
		checkAppError(t, e, "L1406", "quota exceeded: monthly_tasks (150 / 100)", LayerAuthz, nil)
	})
}

func TestL4Constructors(t *testing.T) {
	t.Run("NewL4TaskNotFoundError", func(t *testing.T) {
		e := NewL4TaskNotFoundError("task_xyz")
		checkAppError(t, e, "L1600", "task not found: task_xyz", LayerService, nil)
		if e.Details["task_id"] != "task_xyz" {
			t.Errorf("Details[task_id] = %v", e.Details["task_id"])
		}
	})

	t.Run("NewL4TaskStateInvalidError", func(t *testing.T) {
		e := NewL4TaskStateInvalidError("task_abc", TaskStatusCompleted, []TaskStatus{TaskStatusPending, TaskStatusRunning})
		checkAppError(t, e, "L1601", "task task_abc is in state completed, expected one of [pending running]", LayerService, nil)
	})

	t.Run("NewL4AgentUnavailableError", func(t *testing.T) {
		e := NewL4AgentUnavailableError(AgentRoleGuardian)
		checkAppError(t, e, "L1603", "no available agent for role: guardian", LayerService, nil)
	})

	t.Run("NewL4OrchestrationFailedError", func(t *testing.T) {
		cause := errors.New("graph node panic")
		e := NewL4OrchestrationFailedError("executor node crashed", cause)
		checkAppError(t, e, "L1604", "orchestration failed: executor node crashed", LayerService, cause)
	})

	t.Run("NewL4LLMInvokeError", func(t *testing.T) {
		cause := errors.New("request timeout")
		e := NewL4LLMInvokeError("claude-sonnet-4-20250514", cause)
		checkAppError(t, e, "L1609", "LLM invocation failed (model: claude-sonnet-4-20250514)", LayerService, cause)
	})

	t.Run("NewL4ConfirmationTimeoutError", func(t *testing.T) {
		e := NewL4ConfirmationTimeoutError("task_abc", 300)
		checkAppError(t, e, "L1614", "user confirmation timeout for task task_abc after 300 seconds", LayerService, nil)
	})

	t.Run("NewL4AgentMatchError", func(t *testing.T) {
		e := NewL4AgentMatchError(SubtaskTypeTesting)
		checkAppError(t, e, "L1616", "no matching agent template found for subtask type: testing", LayerService, nil)
	})
}

func TestL5Constructors(t *testing.T) {
	t.Run("NewL5InvalidRequestError", func(t *testing.T) {
		e := NewL5InvalidRequestError("goal", "must be non-empty")
		checkAppError(t, e, "L1800", "invalid request: goal (must be non-empty)", LayerGateway, nil)
	})

	t.Run("NewL5RouteNotFoundError", func(t *testing.T) {
		e := NewL5RouteNotFoundError("/api/v1/tasks/invalid")
		checkAppError(t, e, "L1801", "route not found: /api/v1/tasks/invalid", LayerGateway, nil)
	})

	t.Run("NewL5WSConnectError", func(t *testing.T) {
		cause := errors.New("connection refused")
		e := NewL5WSConnectError(cause)
		checkAppError(t, e, "L1802", "WebSocket connection failed", LayerGateway, cause)
	})

	t.Run("NewL5PayloadTooLargeError", func(t *testing.T) {
		e := NewL5PayloadTooLargeError(15_000_000, 10_000_000)
		checkAppError(t, e, "L1807", "request payload too large: 15000000 bytes (limit: 10000000)", LayerGateway, nil)
	})

	t.Run("NewL5TimeoutError", func(t *testing.T) {
		e := NewL5TimeoutError("TaskService.SubmitTask")
		checkAppError(t, e, "L1808", "request timeout during: TaskService.SubmitTask", LayerGateway, nil)
	})
}

// checkAppError is a test helper that verifies the fields of an AppError.
func checkAppError(t *testing.T, e *AppError, code, msg string, layer Layer, cause error) {
	t.Helper()
	if e == nil {
		t.Fatal("AppError is nil")
	}
	if e.Code != code {
		t.Errorf("Code = %q, want %q", e.Code, code)
	}
	if e.Message != msg {
		t.Errorf("Message = %q, want %q", e.Message, msg)
	}
	if e.Layer != layer {
		t.Errorf("Layer = %q, want %q", e.Layer, layer)
	}
	if cause == nil {
		if e.Cause != nil {
			t.Errorf("Cause = %v, want nil", e.Cause)
		}
	} else {
		if e.Cause != cause {
			t.Errorf("Cause = %v, want %v", e.Cause, cause)
		}
	}
}

func TestErrorCodeRanges(t *testing.T) {
	// Verify all error codes fall within their designated ranges per the spec.
	// Code format: L{layer}{4-digit} where 4-digit = layer*1000 + 3-digit sequence.
	cases := []struct {
		code  string
		layer Layer
		min   int
		max   int
	}{
		{CodeL1DBConnect, LayerStorage, 1001, 1199},
		{CodeL1RecordNotFound, LayerStorage, 1001, 1199},
		{CodeL1RedisLock, LayerStorage, 1001, 1199},
		{CodeL2InvalidStateTransition, LayerDomain, 1200, 1399},
		{CodeL2BusinessRuleViolated, LayerDomain, 1200, 1399},
		{CodeL2OptimisticLock, LayerDomain, 1200, 1399},
		{CodeL3AuthnFailed, LayerAuthz, 1400, 1599},
		{CodeL3RateLimited, LayerAuthz, 1400, 1599},
		{CodeL4TaskNotFound, LayerService, 1600, 1799},
		{CodeL4OrchestrationFailed, LayerService, 1600, 1799},
		{CodeL5InvalidRequest, LayerGateway, 1800, 1999},
		{CodeL5Timeout, LayerGateway, 1800, 1999},
	}

	for _, c := range cases {
		t.Run(c.code, func(t *testing.T) {
			var n int
			if _, err := fmt.Sscanf(c.code[1:], "%d", &n); err != nil {
				t.Fatalf("failed to parse code %q: %v", c.code, err)
			}
			if n < c.min || n > c.max {
				t.Errorf("code %s (parsed as %d) not in range [%d, %d] for layer %s",
					c.code, n, c.min, c.max, c.layer)
			}
		})
	}
}

func TestErrorCodeUniqueness(t *testing.T) {
	// Ensure all error code constants are unique.
	codes := []string{
		// L1
		CodeL1DBConnect, CodeL1DBQuery, CodeL1DBTx, CodeL1RecordNotFound,
		CodeL1UniqueConstraint, CodeL1ForeignKey, CodeL1OutboxPoll, CodeL1OutboxForward,
		CodeL1RedisConnect, CodeL1RedisOp, CodeL1RedisLock, CodeL1MinIOOp, CodeL1Migration,
		// L2
		CodeL2InvalidStateTransition, CodeL2BusinessRuleViolated, CodeL2OptimisticLock,
		CodeL2InvariantBroken, CodeL2EventSerialization, CodeL2ParamValidation,
		CodeL2AggregateNotFound, CodeL2ContextExceeded,
		// L3
		CodeL3AuthnFailed, CodeL3AuthzDenied, CodeL3RateLimited, CodeL3AuthzSvcDown,
		CodeL3InvalidAPIKey, CodeL3TokenExpired, CodeL3InsufficientQuota,
		// L4
		CodeL4TaskNotFound, CodeL4TaskStateInvalid, CodeL4SubtaskNotFound,
		CodeL4AgentUnavailable, CodeL4OrchestrationFailed, CodeL4PluginInvoke,
		CodeL4ContextBudget, CodeL4WorkerAcquire, CodeL4LLMRoute, CodeL4LLMInvoke,
		CodeL4SandboxCreate, CodeL4SandboxDestroy, CodeL4GitOp, CodeL4ToolDenied,
		CodeL4ConfirmationTimeout, CodeL4DecomposeFailed, CodeL4AgentMatch,
		// L5
		CodeL5InvalidRequest, CodeL5RouteNotFound, CodeL5WSConnect, CodeL5WSMessage,
		CodeL5ProtocolError, CodeL5InternalError, CodeL5ServiceUnavailable,
		CodeL5PayloadTooLarge, CodeL5Timeout,
	}

	seen := make(map[string]bool)
	for _, code := range codes {
		if seen[code] {
			t.Errorf("duplicate error code: %s", code)
		}
		seen[code] = true
	}
}

func TestAllLayerConstantsUnique(t *testing.T) {
	// Verify each layer only defines codes for its range.
	// Code format: L{layer}{4-digit} = layer*1000 + 3-digit sequence.
	l1codes := []string{CodeL1DBConnect, CodeL1DBQuery, CodeL1DBTx, CodeL1RecordNotFound,
		CodeL1UniqueConstraint, CodeL1ForeignKey, CodeL1OutboxPoll, CodeL1OutboxForward,
		CodeL1RedisConnect, CodeL1RedisOp, CodeL1RedisLock, CodeL1MinIOOp, CodeL1Migration}
	for _, c := range l1codes {
		n := parseCodeNum(t, c)
		if n < 1001 || n > 1199 {
			t.Errorf("L1 code %s has number %d, want [1001,1199]", c, n)
		}
	}

	l2codes := []string{CodeL2InvalidStateTransition, CodeL2BusinessRuleViolated, CodeL2OptimisticLock,
		CodeL2InvariantBroken, CodeL2EventSerialization, CodeL2ParamValidation,
		CodeL2AggregateNotFound, CodeL2ContextExceeded}
	for _, c := range l2codes {
		n := parseCodeNum(t, c)
		if n < 1200 || n > 1399 {
			t.Errorf("L2 code %s has number %d, want [1200,1399]", c, n)
		}
	}

	l3codes := []string{CodeL3AuthnFailed, CodeL3AuthzDenied, CodeL3RateLimited, CodeL3AuthzSvcDown,
		CodeL3InvalidAPIKey, CodeL3TokenExpired, CodeL3InsufficientQuota}
	for _, c := range l3codes {
		n := parseCodeNum(t, c)
		if n < 1400 || n > 1599 {
			t.Errorf("L3 code %s has number %d, want [1400,1599]", c, n)
		}
	}

	l4codes := []string{CodeL4TaskNotFound, CodeL4TaskStateInvalid, CodeL4SubtaskNotFound,
		CodeL4AgentUnavailable, CodeL4OrchestrationFailed, CodeL4PluginInvoke,
		CodeL4ContextBudget, CodeL4WorkerAcquire, CodeL4LLMRoute, CodeL4LLMInvoke,
		CodeL4SandboxCreate, CodeL4SandboxDestroy, CodeL4GitOp, CodeL4ToolDenied,
		CodeL4ConfirmationTimeout, CodeL4DecomposeFailed, CodeL4AgentMatch}
	for _, c := range l4codes {
		n := parseCodeNum(t, c)
		if n < 1600 || n > 1799 {
			t.Errorf("L4 code %s has number %d, want [1600,1799]", c, n)
		}
	}

	l5codes := []string{CodeL5InvalidRequest, CodeL5RouteNotFound, CodeL5WSConnect, CodeL5WSMessage,
		CodeL5ProtocolError, CodeL5InternalError, CodeL5ServiceUnavailable,
		CodeL5PayloadTooLarge, CodeL5Timeout}
	for _, c := range l5codes {
		n := parseCodeNum(t, c)
		if n < 1800 || n > 1999 {
			t.Errorf("L5 code %s has number %d, want [1800,1999]", c, n)
		}
	}
}

func parseCodeNum(t *testing.T, code string) int {
	t.Helper()
	var n int
	if _, err := fmt.Sscanf(code[1:], "%d", &n); err != nil {
		t.Fatalf("failed to parse code %q: %v", code, err)
	}
	return n
}
