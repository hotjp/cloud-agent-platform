// Package tracing provides OpenTelemetry distributed tracing with OTLP export
// and probability sampling for the Cloud Agent Platform.
package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Span names following the Cloud-Agent-Platform.md specification.
// All span names are lowercase with dots.
const (
	// Task lifecycle spans
	SpanTaskSubmit    = "task.submit"
	SpanTaskDecompose = "task.decompose"
	SpanTaskExecute   = "task.execute"
	SpanTaskComplete  = "task.complete"
	SpanTaskCancel    = "task.cancel"

	// Subtask spans
	SpanSubtaskExecute = "subtask.execute"

	// Agent spans
	SpanAgentThink = "agent.think"
	SpanAgentAct   = "agent.act"

	// LLM spans
	SpanLLMCall = "llm.call"

	// Outbox spans
	SpanOutboxPublish = "outbox.publish"

	// Storage spans
	SpanStorageQuery  = "storage.query"
	SpanStorageInsert = "storage.insert"
	SpanStorageUpdate = "storage.update"

	// Gateway spans
	SpanGatewayHTTP = "gateway.http"
)

// Attribute keys for spans.
const (
	AttrTaskID          = "task.id"
	AttrTaskGoal        = "task.goal"
	AttrTaskStatus      = "task.status"
	AttrSubtaskID       = "subtask.id"
	AttrSubtaskType     = "subtask.type"
	AttrAgentTemplate   = "agent.template"
	AttrAgentRole       = "agent.role"
	AttrLLMModel        = "llm.model"
	AttrLLMTokens       = "llm.tokens"
	AttrLLMDuration     = "llm.duration_ms"
	AttrToolName        = "tool.name"
	AttrToolDuration    = "tool.duration_ms"
	AttrLayer           = "layer"
	AttrRepositoryURL   = "repository.url"
	AttrBranch          = "branch"
	AttrErrorCode       = "error.code"
	AttrErrorMessage    = "error.message"
	AttrOutboxEventType = "outbox.event_type"
	AttrOutboxEventID   = "outbox.event_id"
	AttrDBOperation     = "db.operation"
	AttrDBTable         = "db.table"
	AttrDBRowsAffected  = "db.rows_affected"
	AttrHTTPMethod      = "http.method"
	AttrHTTPPath        = "http.path"
	AttrHTTPStatusCode  = "http.status_code"
	AttrClientID        = "client.id"
	AttrUserID          = "user.id"
	AttrComplexity      = "task.complexity"
	AttrSubtaskCount    = "task.subtask_count"
)

// SpanHelper provides helper methods for creating and ending spans.
type SpanHelper struct {
	tracer trace.Tracer
}

// NewSpanHelper creates a new SpanHelper with the global tracer.
func NewSpanHelper() *SpanHelper {
	return &SpanHelper{tracer: otel.Tracer("github.com/cloud-agent-platform/cap")}
}

// StartTaskSubmit starts a task.submit span.
func (sh *SpanHelper) StartTaskSubmit(ctx context.Context, taskID, goal, clientID string) (context.Context, trace.Span) {
	return sh.tracer.Start(ctx, SpanTaskSubmit,
		trace.WithAttributes(
			attribute.String(AttrTaskID, taskID),
			attribute.String(AttrTaskGoal, truncateString(goal, 200)),
			attribute.String(AttrClientID, clientID),
			attribute.String(AttrLayer, "L4"),
		),
		trace.WithSpanKind(trace.SpanKindServer),
	)
}

// StartTaskDecompose starts a task.decompose span.
func (sh *SpanHelper) StartTaskDecompose(ctx context.Context, taskID string, subtaskCount int) (context.Context, trace.Span) {
	return sh.tracer.Start(ctx, SpanTaskDecompose,
		trace.WithAttributes(
			attribute.String(AttrTaskID, taskID),
			attribute.Int(AttrSubtaskCount, subtaskCount),
			attribute.String(AttrLayer, "L4"),
		),
		trace.WithSpanKind(trace.SpanKindServer),
	)
}

// StartTaskExecute starts a task.execute span.
func (sh *SpanHelper) StartTaskExecute(ctx context.Context, taskID, complexity string) (context.Context, trace.Span) {
	return sh.tracer.Start(ctx, SpanTaskExecute,
		trace.WithAttributes(
			attribute.String(AttrTaskID, taskID),
			attribute.String(AttrComplexity, complexity),
			attribute.String(AttrLayer, "L4"),
		),
		trace.WithSpanKind(trace.SpanKindServer),
	)
}

// StartTaskComplete starts a task.complete span.
func (sh *SpanHelper) StartTaskComplete(ctx context.Context, taskID string, success bool, tokensUsed int) (context.Context, trace.Span) {
	return sh.tracer.Start(ctx, SpanTaskComplete,
		trace.WithAttributes(
			attribute.String(AttrTaskID, taskID),
			attribute.Bool("task.success", success),
			attribute.Int(AttrLLMTokens, tokensUsed),
			attribute.String(AttrLayer, "L4"),
		),
		trace.WithSpanKind(trace.SpanKindServer),
	)
}

// StartSubtaskExecute starts a subtask.execute span.
func (sh *SpanHelper) StartSubtaskExecute(ctx context.Context, taskID, subtaskID, subtaskType, agentTemplate string) (context.Context, trace.Span) {
	return sh.tracer.Start(ctx, SpanSubtaskExecute,
		trace.WithAttributes(
			attribute.String(AttrTaskID, taskID),
			attribute.String(AttrSubtaskID, subtaskID),
			attribute.String(AttrSubtaskType, subtaskType),
			attribute.String(AttrAgentTemplate, agentTemplate),
			attribute.String(AttrLayer, "L4"),
		),
		trace.WithSpanKind(trace.SpanKindClient),
	)
}

// StartAgentThink starts an agent.think span.
func (sh *SpanHelper) StartAgentThink(ctx context.Context, taskID, agentTemplate, promptPreview string) (context.Context, trace.Span) {
	return sh.tracer.Start(ctx, SpanAgentThink,
		trace.WithAttributes(
			attribute.String(AttrTaskID, taskID),
			attribute.String(AttrAgentTemplate, agentTemplate),
			attribute.String("prompt.preview", truncateString(promptPreview, 100)),
			attribute.String(AttrLayer, "L4"),
		),
		trace.WithSpanKind(trace.SpanKindClient),
	)
}

// StartAgentAct starts an agent.act span.
func (sh *SpanHelper) StartAgentAct(ctx context.Context, taskID, agentTemplate, action string) (context.Context, trace.Span) {
	return sh.tracer.Start(ctx, SpanAgentAct,
		trace.WithAttributes(
			attribute.String(AttrTaskID, taskID),
			attribute.String(AttrAgentTemplate, agentTemplate),
			attribute.String("action", truncateString(action, 100)),
			attribute.String(AttrLayer, "L4"),
		),
		trace.WithSpanKind(trace.SpanKindClient),
	)
}

// StartLLMCall starts an llm.call span.
func (sh *SpanHelper) StartLLMCall(ctx context.Context, taskID, model string, estimatedTokens int) (context.Context, trace.Span) {
	return sh.tracer.Start(ctx, SpanLLMCall,
		trace.WithAttributes(
			attribute.String(AttrTaskID, taskID),
			attribute.String(AttrLLMModel, model),
			attribute.Int(AttrLLMTokens, estimatedTokens),
			attribute.String(AttrLayer, "L4"),
		),
		trace.WithSpanKind(trace.SpanKindClient),
	)
}

// StartOutboxPublish starts an outbox.publish span.
func (sh *SpanHelper) StartOutboxPublish(ctx context.Context, eventType, eventID string) (context.Context, trace.Span) {
	return sh.tracer.Start(ctx, SpanOutboxPublish,
		trace.WithAttributes(
			attribute.String(AttrOutboxEventType, eventType),
			attribute.String(AttrOutboxEventID, eventID),
			attribute.String(AttrLayer, "L1"),
		),
		trace.WithSpanKind(trace.SpanKindProducer),
	)
}

// StartStorageQuery starts a storage.query span.
func (sh *SpanHelper) StartStorageQuery(ctx context.Context, table, operation string) (context.Context, trace.Span) {
	return sh.tracer.Start(ctx, SpanStorageQuery,
		trace.WithAttributes(
			attribute.String(AttrDBTable, table),
			attribute.String(AttrDBOperation, operation),
			attribute.String(AttrLayer, "L1"),
		),
		trace.WithSpanKind(trace.SpanKindClient),
	)
}

// EndSpan ends a span with success status.
func EndSpan(span trace.Span) {
	span.SetStatus(codes.Ok, "")
	span.End()
}

// EndSpanWithError ends a span with error status.
func EndSpanWithError(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}

// RecordSpanEvent records an event on the span.
func RecordSpanEvent(span trace.Span, name string, attrs ...attribute.KeyValue) {
	span.AddEvent(name, trace.WithAttributes(attrs...))
}

// truncateString truncates a string to the specified max length.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen < len(s) {
		for i := maxLen; i > maxLen-30 && i > 0; i-- {
			if s[i] == ' ' {
				return s[:i] + "..."
			}
		}
	}
	return s[:maxLen] + "..."
}
