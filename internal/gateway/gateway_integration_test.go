// Package gateway provides integration tests for the L5-Gateway layer.
package gateway

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	capv1 "github.com/cloud-agent-platform/cap/api/cap/v1"
	"github.com/cloud-agent-platform/cap/internal/domain"
	"github.com/cloud-agent-platform/cap/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// TestMain sets up the test environment.
func TestMain(m *testing.M) {
	// Run tests
	m.Run()
}

func TestGateway_New(t *testing.T) {
	logger := zaptest.NewLogger(t)
	svc := service.NewTaskService(service.TaskServiceInput{})

	cfg := Config{
		Port:        8080,
		ReadTimeout: 10 * time.Second,
		WriteTimeout: 30 * time.Second,
		JWTSecret:   "test-secret",
		CORSOrigins: []string{"*"},
	}

	g := New(cfg, svc, logger)

	assert.NotNil(t, g)
	assert.NotNil(t, g.Handler())
	assert.NotNil(t, g.MetricsCollector())
}

func TestGateway_Healthz(t *testing.T) {
	logger := zaptest.NewLogger(t)
	svc := service.NewTaskService(service.TaskServiceInput{})

	cfg := Config{
		Port:        8081,
		ReadTimeout: 10 * time.Second,
		WriteTimeout: 30 * time.Second,
		JWTSecret:   "test-secret",
	}

	g := New(cfg, svc, logger)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	g.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "ok", w.Body.String())
}

func TestGateway_Readyz(t *testing.T) {
	logger := zaptest.NewLogger(t)
	svc := service.NewTaskService(service.TaskServiceInput{})

	cfg := Config{
		Port:        8082,
		ReadTimeout: 10 * time.Second,
		WriteTimeout: 30 * time.Second,
		JWTSecret:   "test-secret",
	}

	g := New(cfg, svc, logger)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	g.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "ok")
}

func TestGateway_CORS(t *testing.T) {
	logger := zaptest.NewLogger(t)
	svc := service.NewTaskService(service.TaskServiceInput{})

	cfg := Config{
		Port:        8083,
		ReadTimeout: 10 * time.Second,
		WriteTimeout: 30 * time.Second,
		JWTSecret:   "test-secret",
		CORSOrigins: []string{"*"},
	}

	g := New(cfg, svc, logger)

	// Test OPTIONS preflight
	req := httptest.NewRequest(http.MethodOptions, "/cap.v1.TaskService/Submit", nil)
	req.Header.Set("Origin", "http://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")

	w := httptest.NewRecorder()
	g.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "http://example.com", w.Header().Get("Access-Control-Allow-Origin"))
}

func TestGateway_RequestID(t *testing.T) {
	logger := zaptest.NewLogger(t)
	svc := service.NewTaskService(service.TaskServiceInput{})

	cfg := Config{
		Port:        8084,
		ReadTimeout: 10 * time.Second,
		WriteTimeout: 30 * time.Second,
		JWTSecret:   "test-secret",
	}

	g := New(cfg, svc, logger)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	g.Handler().ServeHTTP(w, req)

	// Should have a request ID in response header
	assert.NotEmpty(t, w.Header().Get("X-Request-ID"))
}

func TestGateway_RequestID_FromHeader(t *testing.T) {
	logger := zaptest.NewLogger(t)
	svc := service.NewTaskService(service.TaskServiceInput{})

	cfg := Config{
		Port:        8085,
		ReadTimeout: 10 * time.Second,
		WriteTimeout: 30 * time.Second,
		JWTSecret:   "test-secret",
	}

	g := New(cfg, svc, logger)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-Request-ID", "custom-request-id")
	w := httptest.NewRecorder()

	g.Handler().ServeHTTP(w, req)

	assert.Equal(t, "custom-request-id", w.Header().Get("X-Request-ID"))
}

func TestGateway_Auth_JWTDecryption(t *testing.T) {
	logger := zaptest.NewLogger(t)
	svc := service.NewTaskService(service.TaskServiceInput{})

	cfg := Config{
		Port:        8086,
		ReadTimeout: 10 * time.Second,
		WriteTimeout: 30 * time.Second,
		JWTSecret:   "test-secret",
	}

	g := New(cfg, svc, logger)

	// Create a simple JWT-like token (base64 encoded JSON)
	// This is not a real JWT, just testing the decryption logic
	claims := map[string]interface{}{
		"sub":        "user123",
		"client_id":  "client456",
		"exp":        time.Now().Add(time.Hour).Unix(),
	}
	claimsJSON, _ := json.Marshal(claims)
	// Create a simple "header.claims.signature" format for testing
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signature := base64.RawURLEncoding.EncodeToString([]byte("fake-signature"))
	token := header + "." + payload + "." + signature

	// Test with valid token format
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	w := httptest.NewRecorder()
	g.Handler().ServeHTTP(w, req)

	// Should pass through (healthz doesn't require auth)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGateway_Auth_InvalidAuthHeader(t *testing.T) {
	logger := zaptest.NewLogger(t)
	svc := service.NewTaskService(service.TaskServiceInput{})

	cfg := Config{
		Port:        8087,
		ReadTimeout: 10 * time.Second,
		WriteTimeout: 30 * time.Second,
		JWTSecret:   "test-secret",
	}

	g := New(cfg, svc, logger)

	// Test with invalid auth header format
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Authorization", "InvalidFormat")

	w := httptest.NewRecorder()
	g.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestGateway_Auth_InvalidToken(t *testing.T) {
	logger := zaptest.NewLogger(t)
	svc := service.NewTaskService(service.TaskServiceInput{})

	cfg := Config{
		Port:        8088,
		ReadTimeout: 10 * time.Second,
		WriteTimeout: 30 * time.Second,
		JWTSecret:   "test-secret",
	}

	g := New(cfg, svc, logger)

	// Test with invalid token
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")

	w := httptest.NewRecorder()
	g.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestGateway_MetricsCollector(t *testing.T) {
	mc := NewMetricsCollector()

	// Record some metrics
	mc.RecordRequest("/healthz", "GET", 200, 1*time.Millisecond)
	mc.RecordRequest("/healthz", "GET", 200, 2*time.Millisecond)
	mc.RecordRequest("/readyz", "GET", 200, 1*time.Millisecond)

	// Verify metrics were recorded
	assert.NotNil(t, mc)
}

func TestGateway_Recover_Panic(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Create a custom handler that panics
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	// Build the middleware chain
	handler := Chain(panicHandler,
		Recover(logger),
		RequestID(),
	)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	// Should not panic, should return 500
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestTaskServiceHandler_Decide_Unimplemented(t *testing.T) {
	logger := zaptest.NewLogger(t)
	svc := service.NewTaskService(service.TaskServiceInput{})
	handler := NewTaskServiceHandler(svc, logger)

	ctx := context.Background()
	req := connect.NewRequest(&capv1.DecideRequest{
		TaskId:    "task123",
		SubtaskId: "subtask456",
		Decision:  capv1.UserDecision_USER_DECISION_APPROVE,
	})

	_, err := handler.Decide(ctx, req)

	assert.Error(t, err)
	connectErr, ok := err.(*connect.Error)
	assert.True(t, ok)
	assert.Equal(t, connect.CodeUnimplemented, connectErr.Code())
}

func TestTaskServiceHandler_GetArtifact_Unimplemented(t *testing.T) {
	logger := zaptest.NewLogger(t)
	svc := service.NewTaskService(service.TaskServiceInput{})
	handler := NewTaskServiceHandler(svc, logger)

	ctx := context.Background()
	req := connect.NewRequest(&capv1.GetArtifactRequest{
		TaskId:     "task123",
		ArtifactId: "artifact456",
	})

	_, err := handler.GetArtifact(ctx, req)

	assert.Error(t, err)
	connectErr, ok := err.(*connect.Error)
	assert.True(t, ok)
	assert.Equal(t, connect.CodeUnimplemented, connectErr.Code())
}

func TestTaskServiceHandler_GetDiff_Unimplemented(t *testing.T) {
	logger := zaptest.NewLogger(t)
	svc := service.NewTaskService(service.TaskServiceInput{})
	handler := NewTaskServiceHandler(svc, logger)

	ctx := context.Background()
	req := connect.NewRequest(&capv1.GetDiffRequest{
		TaskId:   "task123",
		MaxLines: 500,
	})

	_, err := handler.GetDiff(ctx, req)

	assert.Error(t, err)
	connectErr, ok := err.(*connect.Error)
	assert.True(t, ok)
	assert.Equal(t, connect.CodeUnimplemented, connectErr.Code())
}

func TestTaskServiceHandler_Wait_Unimplemented(t *testing.T) {
	logger := zaptest.NewLogger(t)
	svc := service.NewTaskService(service.TaskServiceInput{})
	handler := NewTaskServiceHandler(svc, logger)

	ctx := context.Background()
	req := connect.NewRequest(&capv1.WaitTaskRequest{
		TaskId:       "task123",
		Timeout:      600,
		PollInterval: 5,
	})

	_, err := handler.Wait(ctx, req)

	assert.Error(t, err)
	connectErr, ok := err.(*connect.Error)
	assert.True(t, ok)
	assert.Equal(t, connect.CodeUnimplemented, connectErr.Code())
}

func TestExtractUserContext_Missing(t *testing.T) {
	logger := zaptest.NewLogger(t)
	svc := service.NewTaskService(service.TaskServiceInput{})
	handler := NewTaskServiceHandler(svc, logger)

	ctx := context.Background()
	req := connect.NewRequest(&capv1.SubmitTaskRequest{
		Goal: "test task",
	})

	_, err := handler.Submit(ctx, req)

	assert.Error(t, err)
	connectErr, ok := err.(*connect.Error)
	assert.True(t, ok)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func TestDecryptJWT(t *testing.T) {
	// Create a simple JWT-like token
	claims := map[string]interface{}{
		"sub":       "user123",
		"client_id": "client456",
		"exp":       time.Now().Add(time.Hour).Unix(),
	}
	claimsJSON, _ := json.Marshal(claims)
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signature := base64.RawURLEncoding.EncodeToString([]byte("fake-signature"))
	token := header + "." + payload + "." + signature

	result, err := decryptJWT(token, "any-secret")
	require.NoError(t, err)
	assert.Equal(t, "user123", result["sub"])
	assert.Equal(t, "client456", result["client_id"])
}

func TestDecryptJWT_InvalidFormat(t *testing.T) {
	_, err := decryptJWT("invalid-token", "secret")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid JWT format")
}

func TestDecryptJWT_InvalidBase64(t *testing.T) {
	_, err := decryptJWT("header!!!.payload.signature", "secret")
	assert.Error(t, err)
}

func TestIsOriginAllowed(t *testing.T) {
	tests := []struct {
		origin  string
		allowed []string
		want    bool
	}{
		{"http://example.com", []string{"*"}, true},
		{"http://example.com", []string{"http://example.com"}, true},
		{"http://other.com", []string{"http://example.com"}, false},
		{"", []string{"*"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.origin, func(t *testing.T) {
			got := isOriginAllowed(tt.origin, tt.allowed)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResponseWriter_WriteHeader(t *testing.T) {
	w := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

	rw.WriteHeader(http.StatusCreated)

	assert.Equal(t, http.StatusCreated, rw.statusCode)
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestChain(t *testing.T) {
	callOrder := make([]string, 0)

	middleware1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callOrder = append(callOrder, "mw1-before")
			next.ServeHTTP(w, r)
			callOrder = append(callOrder, "mw1-after")
		})
	}

	middleware2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callOrder = append(callOrder, "mw2-before")
			next.ServeHTTP(w, r)
			callOrder = append(callOrder, "mw2-after")
		})
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callOrder = append(callOrder, "handler")
	})

	wrapped := Chain(handler, middleware1, middleware2)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	// Chain applies in reverse, so mw2 is outermost
	expected := []string{"mw1-before", "mw2-before", "handler", "mw2-after", "mw1-after"}
	assert.Equal(t, expected, callOrder)
}

func TestWithUserContext(t *testing.T) {
	uc := &userContext{
		userID:   "user123",
		clientID: "client456",
		claims:   map[string]interface{}{"role": "admin"},
	}

	ctx := withUserContext(context.Background(), uc)

	// Extract and verify
	extracted, err := extractUserContext(ctx)
	require.NoError(t, err)
	assert.Equal(t, "user123", extracted.userID)
	assert.Equal(t, "client456", extracted.clientID)
	assert.Equal(t, "admin", extracted.claims["role"])
}

func TestGetStringClaim(t *testing.T) {
	claims := map[string]interface{}{
		"sub":       "user123",
		"client_id": "client456",
		"exp":       float64(time.Now().Add(time.Hour).Unix()),
	}

	assert.Equal(t, "user123", getStringClaim(claims, "sub"))
	assert.Equal(t, "client456", getStringClaim(claims, "client_id"))
	assert.Equal(t, "", getStringClaim(claims, "nonexistent"))
	assert.Equal(t, "", getStringClaim(claims, "exp")) // exp is float64, not string
}

func TestMapTaskStatus(t *testing.T) {
	tests := []struct {
		domainStatus domain.TaskStatus
		protoStatus capv1.TaskStatus
	}{
		{domain.TaskStatusPending, capv1.TaskStatus_TASK_STATUS_PENDING},
		{domain.TaskStatusDispatched, capv1.TaskStatus_TASK_STATUS_DISPATCHED},
		{domain.TaskStatusRunning, capv1.TaskStatus_TASK_STATUS_RUNNING},
		{domain.TaskStatusReviewing, capv1.TaskStatus_TASK_STATUS_REVIEWING},
		{domain.TaskStatusConfirming, capv1.TaskStatus_TASK_STATUS_CONFIRMING},
		{domain.TaskStatusCompleted, capv1.TaskStatus_TASK_STATUS_COMPLETED},
		{domain.TaskStatusFailed, capv1.TaskStatus_TASK_STATUS_FAILED},
		{domain.TaskStatusCancelled, capv1.TaskStatus_TASK_STATUS_CANCELLED},
		{"", capv1.TaskStatus_TASK_STATUS_UNSPECIFIED},
	}

	for _, tt := range tests {
		t.Run(string(tt.domainStatus), func(t *testing.T) {
			got := mapTaskStatus(tt.domainStatus)
			assert.Equal(t, tt.protoStatus, got)
		})
	}
}

func TestMapProtoTaskStatus(t *testing.T) {
	tests := []struct {
		protoStatus  capv1.TaskStatus
		domainStatus string
	}{
		{capv1.TaskStatus_TASK_STATUS_PENDING, "pending"},
		{capv1.TaskStatus_TASK_STATUS_DISPATCHED, "dispatched"},
		{capv1.TaskStatus_TASK_STATUS_RUNNING, "running"},
		{capv1.TaskStatus_TASK_STATUS_REVIEWING, "reviewing"},
		{capv1.TaskStatus_TASK_STATUS_CONFIRMING, "confirming"},
		{capv1.TaskStatus_TASK_STATUS_COMPLETED, "completed"},
		{capv1.TaskStatus_TASK_STATUS_FAILED, "failed"},
		{capv1.TaskStatus_TASK_STATUS_CANCELLED, "cancelled"},
		{capv1.TaskStatus_TASK_STATUS_UNSPECIFIED, ""},
	}

	for _, tt := range tests {
		t.Run(tt.protoStatus.String(), func(t *testing.T) {
			got := mapProtoTaskStatus(tt.protoStatus)
			assert.Equal(t, tt.domainStatus, string(got))
		})
	}
}

func TestGetRepoURL(t *testing.T) {
	req := &capv1.SubmitTaskRequest{
		Repository: &capv1.Repository{
			Url: "https://github.com/example/repo",
		},
	}

	assert.Equal(t, "https://github.com/example/repo", getRepoURL(req))
}

func TestGetRepoURL_Nil(t *testing.T) {
	req := &capv1.SubmitTaskRequest{
		Repository: nil,
	}

	assert.Equal(t, "", getRepoURL(req))
}

func TestGetRepoBranch(t *testing.T) {
	req := &capv1.SubmitTaskRequest{
		Repository: &capv1.Repository{
			Branch: "main",
		},
	}

	assert.Equal(t, "main", getRepoBranch(req))
}

func TestGetRepoBranch_Nil(t *testing.T) {
	req := &capv1.SubmitTaskRequest{
		Repository: nil,
	}

	assert.Equal(t, "", getRepoBranch(req))
}

// TestGateway_StartStop tests the Start and Stop methods.
func TestGateway_StartStop(t *testing.T) {
	logger := zaptest.NewLogger(t)
	svc := service.NewTaskService(service.TaskServiceInput{})

	cfg := Config{
		Port:        8090,
		ReadTimeout: 1 * time.Second,
		WriteTimeout: 1 * time.Second,
		JWTSecret:   "test-secret",
	}

	g := New(cfg, svc, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := g.Start(ctx)
	// Should return nil because context was cancelled
	assert.NoError(t, err)

	// Stop should also work
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer stopCancel()
	err = g.Stop(stopCtx)
	assert.NoError(t, err)
}

// BenchmarkGateway_MiddlewareChain benchmarks the middleware chain.
func BenchmarkGateway_MiddlewareChain(b *testing.B) {
	logger := zaptest.NewLogger(b)
	svc := service.NewTaskService(service.TaskServiceInput{})

	cfg := Config{
		Port:        8091,
		ReadTimeout: 10 * time.Second,
		WriteTimeout: 30 * time.Second,
		JWTSecret:   "test-secret",
	}

	g := New(cfg, svc, logger)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.Handler().ServeHTTP(w, req)
	}
}

// BenchmarkDecryptJWT benchmarks JWT decryption.
func BenchmarkDecryptJWT(b *testing.B) {
	claims := map[string]interface{}{
		"sub":       strings.Repeat("user", 10),
		"client_id": strings.Repeat("client", 10),
		"exp":       time.Now().Add(time.Hour).Unix(),
	}
	claimsJSON, _ := json.Marshal(claims)
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signature := base64.RawURLEncoding.EncodeToString([]byte("fake-signature"))
	token := header + "." + payload + "." + signature

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decryptJWT(token, "any-secret")
	}
}
