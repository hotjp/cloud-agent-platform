// Package llmrouter implements LLM multi-model routing (Claude/GLM adaptive).
package llmrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
)

// GLMProvider implements LLMProvider for GLM models.
type GLMProvider struct {
	name       ModelName
	apiKey     string
	endpoint   string
	timeout    time.Duration
	httpClient *http.Client
	logger     *zap.Logger
	stats      ProviderStats
	mu         sync.RWMutex
}

// NewGLMProvider creates a new GLM provider.
func NewGLMProvider(name ModelName, apiKey, endpoint string, timeout time.Duration, logger *zap.Logger) *GLMProvider {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &GLMProvider{
		name:     name,
		apiKey:   apiKey,
		endpoint: endpoint,
		timeout:  timeout,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		logger: logger,
	}
}

// Name returns the provider name.
func (p *GLMProvider) Name() ModelName {
	return p.name
}

// Stats returns the provider statistics.
func (p *GLMProvider) Stats() *ProviderStats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	stats := p.stats
	return &stats
}

// Complete generates a complete response.
func (p *GLMProvider) Complete(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
	start := time.Now()

	// Build request payload for GLM API
	payload := glmRequestPayload{
		Model: string(p.name),
		Messages: []glmMessage{
			{
				Role:    "user",
				Content: req.Prompt,
			},
		},
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}

	if req.System != "" {
		payload.Messages = append([]glmMessage{
			{Role: "system", Content: req.System},
		}, payload.Messages...)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, &RetryableError{Err: fmt.Errorf("marshal request: %w", err), Retryable: false}
	}

	// Create request
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, &RetryableError{Err: fmt.Errorf("create request: %w", err), Retryable: false}
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.apiKey))

	// Execute request
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		p.recordError(err)
		return nil, &RetryableError{Err: fmt.Errorf("execute request: %w", err), Retryable: true}
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		p.recordError(err)
		return nil, &RetryableError{Err: fmt.Errorf("read response: %w", err), Retryable: true}
	}

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("API error: status=%d, body=%s", resp.StatusCode, string(respBody))
		p.recordError(err)
		// Retry on 5xx errors
		return nil, &RetryableError{Err: err, Retryable: resp.StatusCode >= 500}
	}

	// Parse response
	var glmResp glmResponse
	if err := json.Unmarshal(respBody, &glmResp); err != nil {
		p.recordError(err)
		return nil, &RetryableError{Err: fmt.Errorf("parse response: %w", err), Retryable: true}
	}

	latency := time.Since(start).Milliseconds()
	tokensUsed := int64(glmResp.Usage.TotalTokens)

	p.recordSuccess(tokensUsed, latency)

	return &LLMResponse{
		Content:    glmResp.Choices[0].Message.Content,
		Model:      p.name,
		TokensUsed: int(tokensUsed),
		LatencyMs:  latency,
	}, nil
}

// Stream generates a streaming response.
func (p *GLMProvider) Stream(ctx context.Context, req *LLMRequest, handler func(*StreamChunk) error) error {
	p.logger.Warn("streaming not fully implemented", zap.String("provider", string(p.name)))
	return nil
}

// Embed generates embeddings for the given text.
func (p *GLMProvider) Embed(ctx context.Context, req *EmbedRequest) (*EmbedResponse, error) {
	start := time.Now()

	payload := map[string]any{
		"model": string(p.name),
		"input": req.Text,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, &RetryableError{Err: fmt.Errorf("marshal request: %w", err), Retryable: false}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, &RetryableError{Err: fmt.Errorf("create request: %w", err), Retryable: false}
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.apiKey))

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		p.recordError(err)
		return nil, &RetryableError{Err: fmt.Errorf("execute request: %w", err), Retryable: true}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		p.recordError(err)
		return nil, &RetryableError{Err: fmt.Errorf("read response: %w", err), Retryable: true}
	}

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("API error: status=%d, body=%s", resp.StatusCode, string(respBody))
		p.recordError(err)
		return nil, &RetryableError{Err: err, Retryable: resp.StatusCode >= 500}
	}

	_ = time.Since(start).Milliseconds()

	return &EmbedResponse{
		Model: p.name,
	}, nil
}

// recordSuccess records a successful request.
func (p *GLMProvider) recordSuccess(tokens, latencyMs int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stats.TotalRequests++
	p.stats.SuccessRequests++
	p.stats.TotalTokens += tokens
	if p.stats.TotalRequests > 0 {
		p.stats.AvgLatencyMs = (p.stats.AvgLatencyMs*(p.stats.TotalRequests-1) + latencyMs) / p.stats.TotalRequests
	} else {
		p.stats.AvgLatencyMs = latencyMs
	}
}

// recordError records a failed request.
func (p *GLMProvider) recordError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stats.TotalRequests++
	p.stats.FailedRequests++
	p.stats.LastError = err.Error()
	p.stats.LastErrorTime = time.Now()
}

// glmRequestPayload is the request payload for GLM API.
type glmRequestPayload struct {
	Model       string      `json:"model"`
	Messages    []glmMessage `json:"messages"`
	MaxTokens   int         `json:"max_tokens"`
	Temperature float64     `json:"temperature,omitempty"`
	Stop        []string    `json:"stop,omitempty"`
}

// glmMessage is a message in GLM API format.
type glmMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// glmResponse is the response from GLM API.
type glmResponse struct {
	ID      string        `json:"id"`
	Model   string        `json:"model"`
	Choices []glmChoice   `json:"choices"`
	Usage   glmUsage      `json:"usage"`
}

// glmChoice is a choice in GLM response.
type glmChoice struct {
	Index        int        `json:"index"`
	Message      glmMessage `json:"message"`
	FinishReason string     `json:"finish_reason"`
}

// glmUsage is the token usage of a GLM response.
type glmUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Ensure GLMProvider implements LLMProvider.
var _ LLMProvider = (*GLMProvider)(nil)
