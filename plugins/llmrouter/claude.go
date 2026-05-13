// Package llmrouter implements LLM multi-model routing (Claude/GLM adaptive).
package llmrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ClaudeProvider implements LLMProvider for Claude models.
type ClaudeProvider struct {
	name       ModelName
	apiKey     string
	endpoint   string
	timeout    time.Duration
	httpClient *http.Client
	logger     *zap.Logger
	stats      ProviderStats
	mu         sync.RWMutex
}

// NewClaudeProvider creates a new Claude provider.
func NewClaudeProvider(name ModelName, apiKey, endpoint string, timeout time.Duration, logger *zap.Logger) *ClaudeProvider {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ClaudeProvider{
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
func (p *ClaudeProvider) Name() ModelName {
	return p.name
}

// Stats returns the provider statistics.
func (p *ClaudeProvider) Stats() *ProviderStats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	stats := p.stats
	return &stats
}

// Complete generates a complete response.
func (p *ClaudeProvider) Complete(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
	start := time.Now()

	// Build request payload
	payload := claudeRequestPayload{
		Model: string(p.name),
		Messages: []claudeMessage{
			{
				Role:    "user",
				Content: req.Prompt,
			},
		},
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}

	if req.System != "" {
		payload.Messages = append([]claudeMessage{
			{Role: "assistant", Content: req.System},
		}, payload.Messages...)
	}

	if len(req.StopWords) > 0 {
		payload.StopSequences = req.StopWords
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
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("anthropic-dangerous-direct-browser-access", "true")

	// Execute request
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		p.recordError(err)
		return nil, &RetryableError{Err: fmt.Errorf("execute request: %w", err), Retryable: true}
	}
	defer resp.Body.Close()

	// Read response
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		p.recordError(err)
		return nil, &RetryableError{Err: fmt.Errorf("read response: %w", err), Retryable: true}
	}

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("API error: status=%d, body=%s", resp.StatusCode, string(body))
		p.recordError(err)
		// Retry on 5xx errors
		retryable := resp.StatusCode >= 500
		return nil, &RetryableError{Err: err, Retryable: retryable}
	}

	// Parse response
	var claudeResp claudeResponse
	if err := json.Unmarshal(body, &claudeResp); err != nil {
		p.recordError(err)
		return nil, &RetryableError{Err: fmt.Errorf("parse response: %w", err), Retryable: true}
	}

	latency := time.Since(start).Milliseconds()
	tokensUsed := int64(claudeResp.Usage.InputTokens + claudeResp.Usage.OutputTokens)

	p.recordSuccess(tokensUsed, latency)

	return &LLMResponse{
		Content:    claudeResp.Content[0].Text,
		Model:      p.name,
		TokensUsed: int(tokensUsed),
		LatencyMs:  latency,
	}, nil
}

// Stream generates a streaming response.
func (p *ClaudeProvider) Stream(ctx context.Context, req *LLMRequest, handler func(*StreamChunk) error) error {
	p.logger.Warn("streaming not fully implemented", zap.String("provider", string(p.name)))
	return nil
}

// Embed generates embeddings for the given text.
func (p *ClaudeProvider) Embed(ctx context.Context, req *EmbedRequest) (*EmbedResponse, error) {
	start := time.Now()

	payload := map[string]any{
		"model": string(req.Text),
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
	httpReq.Header.Set("x-api-key", p.apiKey)

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
func (p *ClaudeProvider) recordSuccess(tokens, latencyMs int64) {
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
func (p *ClaudeProvider) recordError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stats.TotalRequests++
	p.stats.FailedRequests++
	p.stats.LastError = err.Error()
	p.stats.LastErrorTime = time.Now()
}

// claudeRequestPayload is the request payload for Claude API.
type claudeRequestPayload struct {
	Model          string          `json:"model"`
	Messages       []claudeMessage `json:"messages"`
	MaxTokens      int             `json:"max_tokens"`
	Temperature    float64         `json:"temperature,omitempty"`
	StopSequences  []string        `json:"stop_sequences,omitempty"`
	System         string          `json:"system,omitempty"`
}

// claudeMessage is a message in Claude API format.
type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// claudeResponse is the response from Claude API.
type claudeResponse struct {
	ID      string           `json:"id"`
	Type    string           `json:"type"`
	Role    string           `json:"role"`
	Content []claudeContent  `json:"content"`
	Model   string           `json:"model"`
	Usage   claudeUsage      `json:"usage"`
	Stop    string           `json:"stop_reason,omitempty"`
}

// claudeContent is the content of a Claude response.
type claudeContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// claudeUsage is the token usage of a Claude response.
type claudeUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Ensure ClaudeProvider implements LLMProvider.
var _ LLMProvider = (*ClaudeProvider)(nil)

// TruncatePrompt truncates prompt to fit within token limit.
func TruncatePrompt(prompt string, maxTokens int) string {
	// Rough estimation: 1 token ≈ 4 characters
	maxChars := maxTokens * 4
	if len(prompt) <= maxChars {
		return prompt
	}
	return prompt[:maxChars] + "..."
}

// ParseModelName parses a model name string to ModelName.
func ParseModelName(name string) ModelName {
	name = strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.Contains(name, "claude-sonnet"):
		return ModelClaudeSonnet
	case strings.Contains(name, "claude-haiku"):
		return ModelClaudeHaiku
	case strings.Contains(name, "glm-5.1-air"):
		return ModelGLM5Air
	case strings.Contains(name, "glm"):
		return ModelGLM5
	default:
		return ModelClaudeSonnet
	}
}
