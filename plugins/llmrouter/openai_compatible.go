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

// OpenAICompatibleProvider implements LLMProvider for OpenAI-compatible APIs.
// Supports Deepseek, Qwen, GLM (with compatible endpoint), and other REST-based LLMs.
type OpenAICompatibleProvider struct {
	name       ModelName
	apiKey     string
	endpoint   string
	timeout    time.Duration
	httpClient *http.Client
	logger     *zap.Logger
	stats      ProviderStats
	mu         sync.RWMutex
	// roundRobinIndex is used for load balancing across multiple endpoints
	roundRobinIndex int
	endpoints       []string
}

// OpenAIProviderConfig holds additional configuration for OpenAI-compatible providers.
type OpenAIProviderConfig struct {
	// Endpoints is a list of endpoints for load balancing (optional).
	Endpoints []string
	// APIVersion is the API version (e.g., "v1").
	APIVersion string
	// SupportsReasoning indicates if the provider supports reasoning models.
	SupportsReasoning bool
}

// NewOpenAICompatibleProvider creates a new OpenAI-compatible provider.
func NewOpenAICompatibleProvider(name ModelName, apiKey, endpoint string, timeout time.Duration, logger *zap.Logger) *OpenAICompatibleProvider {
	if logger == nil {
		logger = zap.NewNop()
	}
	endpoints := []string{endpoint}
	return &OpenAICompatibleProvider{
		name:       name,
		apiKey:     apiKey,
		endpoint:   endpoint,
		timeout:    timeout,
		httpClient: &http.Client{Timeout: timeout},
		logger:     logger,
		endpoints:  endpoints,
	}
}

// NewOpenAICompatibleProviderWithEndpoints creates a new provider with multiple endpoints for load balancing.
func NewOpenAICompatibleProviderWithEndpoints(name ModelName, apiKey string, endpoints []string, timeout time.Duration, logger *zap.Logger) *OpenAICompatibleProvider {
	if logger == nil {
		logger = zap.NewNop()
	}
	if len(endpoints) == 0 {
		endpoints = []string{""}
	}
	return &OpenAICompatibleProvider{
		name:       name,
		apiKey:     apiKey,
		endpoint:   endpoints[0],
		timeout:    timeout,
		httpClient: &http.Client{Timeout: timeout},
		logger:     logger,
		endpoints:  endpoints,
	}
}

// Name returns the provider name.
func (p *OpenAICompatibleProvider) Name() ModelName {
	return p.name
}

// Stats returns the provider statistics.
func (p *OpenAICompatibleProvider) Stats() *ProviderStats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	stats := p.stats
	return &stats
}

// getNextEndpoint returns the next endpoint in round-robin fashion.
func (p *OpenAICompatibleProvider) getNextEndpoint() string {
	if len(p.endpoints) <= 1 {
		return p.endpoint
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	ep := p.endpoints[p.roundRobinIndex%len(p.endpoints)]
	p.roundRobinIndex++
	return ep
}

// Complete generates a complete response.
func (p *OpenAICompatibleProvider) Complete(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
	start := time.Now()
	endpoint := p.getNextEndpoint()

	// Build OpenAI-compatible request payload
	payload := openAIRequestPayload{
		Model: string(p.name),
		Messages: []openAIMessage{
			{
				Role:    "user",
				Content: req.Prompt,
			},
		},
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}

	if req.System != "" {
		payload.Messages = append([]openAIMessage{
			{Role: "system", Content: req.System},
		}, payload.Messages...)
	}

	if len(req.StopWords) > 0 {
		payload.Stop = req.StopWords
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, &RetryableError{Err: fmt.Errorf("marshal request: %w", err), Retryable: false}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		p.recordError(err)
		return nil, &RetryableError{Err: fmt.Errorf("create request: %w", err), Retryable: false}
	}

	p.setHeaders(httpReq)

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

	var openAIResp openAIResponse
	if err := json.Unmarshal(respBody, &openAIResp); err != nil {
		p.recordError(err)
		return nil, &RetryableError{Err: fmt.Errorf("parse response: %w", err), Retryable: true}
	}

	latency := time.Since(start).Milliseconds()
	tokensUsed := int64(openAIResp.Usage.TotalTokens)

	p.recordSuccess(tokensUsed, latency)

	content := openAIResp.Choices[0].Message.Content
	return &LLMResponse{
		Content:    content,
		Model:      p.name,
		TokensUsed: int(tokensUsed),
		LatencyMs:  latency,
	}, nil
}

// Stream generates a streaming response.
func (p *OpenAICompatibleProvider) Stream(ctx context.Context, req *LLMRequest, handler func(*StreamChunk) error) error {
	endpoint := p.getNextEndpoint()

	payload := openAIRequestPayload{
		Model: string(p.name),
		Messages: []openAIMessage{
			{
				Role:    "user",
				Content: req.Prompt,
			},
		},
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Stream:      true,
	}

	if req.System != "" {
		payload.Messages = append([]openAIMessage{
			{Role: "system", Content: req.System},
		}, payload.Messages...)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return &RetryableError{Err: fmt.Errorf("marshal request: %w", err), Retryable: false}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return &RetryableError{Err: fmt.Errorf("create request: %w", err), Retryable: false}
	}

	p.setHeaders(httpReq)
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Cache-Control", "no-cache")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return &RetryableError{Err: fmt.Errorf("execute request: %w", err), Retryable: true}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return &RetryableError{Err: fmt.Errorf("API error: status=%d, body=%s", resp.StatusCode, string(respBody)), Retryable: resp.StatusCode >= 500}
	}

	// Simple SSE parsing for streaming responses
	buf := make([]byte, 4096)
	reader := resp.Body
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			text := string(buf[:n])
			// Parse SSE format: data: {...}\n\n
			lines := strings.Split(text, "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "data: ") {
					data := strings.TrimPrefix(line, "data: ")
					if data == "[DONE]" {
						handler(&StreamChunk{Done: true})
						return nil
					}
					var chunk openAIDeltaChunk
					if json.Unmarshal([]byte(data), &chunk) == nil && len(chunk.Choices) > 0 {
						if err := handler(&StreamChunk{
							Content: chunk.Choices[0].Delta.Content,
							Done:    false,
						}); err != nil {
							return err
						}
					}
				}
			}
		}
		if err != nil {
			break
		}
	}

	return nil
}

// Embed generates embeddings for the given text.
func (p *OpenAICompatibleProvider) Embed(ctx context.Context, req *EmbedRequest) (*EmbedResponse, error) {
	start := time.Now()
	endpoint := p.getNextEndpoint()

	payload := map[string]any{
		"model": string(p.name),
		"input": req.Text,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, &RetryableError{Err: fmt.Errorf("marshal request: %w", err), Retryable: false}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, &RetryableError{Err: fmt.Errorf("create request: %w", err), Retryable: false}
	}

	p.setHeaders(httpReq)

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

	var embedResp openAIEmbeddingResponse
	if err := json.Unmarshal(respBody, &embedResp); err != nil {
		p.recordError(err)
		return nil, &RetryableError{Err: fmt.Errorf("parse response: %w", err), Retryable: true}
	}

	_ = time.Since(start).Milliseconds()

	return &EmbedResponse{
		Embedding: embedResp.Data[0].Embedding,
		Model:     p.name,
	}, nil
}

// setHeaders sets common headers for OpenAI-compatible APIs.
func (p *OpenAICompatibleProvider) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.apiKey))
}

// recordSuccess records a successful request.
func (p *OpenAICompatibleProvider) recordSuccess(tokens, latencyMs int64) {
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
func (p *OpenAICompatibleProvider) recordError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stats.TotalRequests++
	p.stats.FailedRequests++
	p.stats.LastError = err.Error()
	p.stats.LastErrorTime = time.Now()
}

// openAIRequestPayload is the request payload for OpenAI-compatible APIs.
type openAIRequestPayload struct {
	Model       string           `json:"model"`
	Messages    []openAIMessage  `json:"messages"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Temperature float64          `json:"temperature,omitempty"`
	Stop        []string         `json:"stop,omitempty"`
	Stream      bool             `json:"stream,omitempty"`
}

// openAIMessage is a message in OpenAI API format.
type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIResponse is the response from OpenAI-compatible APIs.
type openAIResponse struct {
	ID      string           `json:"id"`
	Object  string           `json:"object"`
	Created int64            `json:"created"`
	Model   string           `json:"model"`
	Choices []openAIChoice   `json:"choices"`
	Usage   openAIUsage      `json:"usage"`
}

// openAIDeltaChunk is a streaming delta chunk.
type openAIDeltaChunk struct {
	Choices []openAIDelta `json:"choices"`
}

// openAIDelta is a delta in streaming response.
type openAIDelta struct {
	Index        int          `json:"index"`
	Delta        openAIMessage `json:"delta"`
	FinishReason string       `json:"finish_reason,omitempty"`
}

// openAIChoice is a choice in OpenAI response.
type openAIChoice struct {
	Index        int           `json:"index"`
	Message      openAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason,omitempty"`
}

// openAIUsage is the token usage of an OpenAI response.
type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// openAIEmbeddingResponse is the response for embedding requests.
type openAIEmbeddingResponse struct {
	Object string         `json:"object"`
	Data   []openAIEmbedding `json:"data"`
	Model  string          `json:"model"`
	Usage  openAIUsage     `json:"usage"`
}

// openAIEmbedding is an embedding result.
type openAIEmbedding struct {
	Object string    `json:"object"`
	Index  int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}

// Ensure OpenAICompatibleProvider implements LLMProvider.
var _ LLMProvider = (*OpenAICompatibleProvider)(nil)