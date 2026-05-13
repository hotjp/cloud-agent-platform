// Package react implements the ReAct (Reasoning + Acting) agent pattern.
package react

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
)

// ----------------------------------------------------------------------------
// Streaming Support
// ----------------------------------------------------------------------------

// StepHandler is a function that handles ReAct steps as they are generated.
type StepHandler func(ctx context.Context, step *Step) error

// Streamer provides streaming support for ReAct agent execution.
type Streamer struct {
	handler StepHandler
	mu      sync.Mutex
}

// NewStreamer creates a new Streamer with the given handler.
func NewStreamer(handler StepHandler) *Streamer {
	return &Streamer{handler: handler}
}

// Emit emits a step to the handler.
func (s *Streamer) Emit(ctx context.Context, step *Step) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.handler(ctx, step)
}

// EmitAsync emits a step asynchronously.
func (s *Streamer) EmitAsync(ctx context.Context, step *Step) {
	go func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.handler(ctx, step)
	}()
}

// StreamResult streams the execution result step by step.
type StreamResult struct {
	mu         sync.Mutex
	steps      []*Step
	answer     string
	stopReason StopReason
	totalTokens int
	iterations int
	err        error
	complete   bool
}

// NewStreamResult creates a new StreamResult.
func NewStreamResult() *StreamResult {
	return &StreamResult{
		steps:      make([]*Step, 0),
		stopReason: StopReasonCancelled,
	}
}

// AddStep adds a step to the result.
func (r *StreamResult) AddStep(step *Step) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.steps = append(r.steps, step)
}

// SetAnswer sets the final answer.
func (r *StreamResult) SetAnswer(answer string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.answer = answer
}

// SetStopReason sets the stop reason.
func (r *StreamResult) SetStopReason(reason StopReason) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopReason = reason
}

// SetError sets the error.
func (r *StreamResult) SetError(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.err = err
}

// SetComplete marks the result as complete.
func (r *StreamResult) SetComplete() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.complete = true
}

// GetResult returns the final Result.
func (r *StreamResult) GetResult() *Result {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := &Result{
		Answer:          r.answer,
		Steps:           r.steps,
		TotalSteps:      len(r.steps),
		TotalTokensUsed: r.totalTokens,
		Iterations:      r.iterations,
		StopReason:      r.stopReason,
		Error:           r.err,
	}
	return result
}

// IsComplete returns whether the result is complete.
func (r *StreamResult) IsComplete() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.complete
}

// AddTokens adds token usage.
func (r *StreamResult) AddTokens(tokens int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.totalTokens += tokens
}

// SetIterations sets the iteration count.
func (r *StreamResult) SetIterations(count int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.iterations = count
}

// ----------------------------------------------------------------------------
// SSE Streaming
// ----------------------------------------------------------------------------

// SSEWriter writes Server-Sent Events for streaming.
type SSEWriter struct {
	writer io.Writer
	mu     sync.Mutex
}

// NewSSEWriter creates a new SSEWriter.
func NewSSEWriter(w io.Writer) *SSEWriter {
	return &SSEWriter{writer: w}
}

// Write writes a Server-Sent Event.
func (w *SSEWriter) Write(event string, data any) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Write event line
	if _, err := fmt.Fprintf(w.writer, "event: %s\n", event); err != nil {
		return err
	}

	// Write data as JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(w.writer, "data: %s\n\n", jsonData); err != nil {
		return err
	}

	// Flush
	if f, ok := w.writer.(interface{ Flush() error }); ok {
		return f.Flush()
	}
	return nil
}

// WriteStep writes a step event.
func (w *SSEWriter) WriteStep(step *Step) error {
	return w.Write("step", step)
}

// WriteError writes an error event.
func (w *SSEWriter) WriteError(err error) error {
	return w.Write("error", map[string]string{"error": err.Error()})
}

// WriteDone writes a done event.
func (w *SSEWriter) WriteDone(result *Result) error {
	return w.Write("done", result)
}

// ----------------------------------------------------------------------------
// Step Aggregator
// ----------------------------------------------------------------------------

// StepAggregator aggregates steps into a summary.
type StepAggregator struct {
	steps []*Step
}

// NewStepAggregator creates a new StepAggregator.
func NewStepAggregator() *StepAggregator {
	return &StepAggregator{
		steps: make([]*Step, 0),
	}
}

// Add adds a step to the aggregator.
func (a *StepAggregator) Add(step *Step) {
	a.steps = append(a.steps, step)
}

// Summarize returns a summary of the steps.
func (a *StepAggregator) Summarize() string {
	var sb strings.Builder
	sb.WriteString("## Execution Summary\n\n")
	sb.WriteString(fmt.Sprintf("Total Steps: %d\n\n", len(a.steps)))

	thoughts := 0
	actions := 0
	observations := 0
	for _, step := range a.steps {
		switch step.Type {
		case StepTypeThought:
			thoughts++
		case StepTypeAction:
			actions++
		case StepTypeObservation:
			observations++
		}
	}

	sb.WriteString(fmt.Sprintf("- Thoughts: %d\n", thoughts))
	sb.WriteString(fmt.Sprintf("- Actions: %d\n", actions))
	sb.WriteString(fmt.Sprintf("- Observations: %d\n\n", observations))

	return sb.String()
}

// String implements fmt.Stringer.
func (a *StepAggregator) String() string {
	return a.Summarize()
}