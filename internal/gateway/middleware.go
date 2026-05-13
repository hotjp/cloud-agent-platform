// Package gateway implements L5-Gateway layer: protocol adaptation, middleware chain,
// and request routing via connect-go.
package gateway

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Middleware is a function that wraps an http.Handler.
type Middleware func(http.Handler) http.Handler

// Chain applies middlewares in order: Recover → RequestID → Metrics → Logging → CORS → Auth.
func Chain(handler http.Handler, mw ...Middleware) http.Handler {
	// Apply in reverse order since middleware wraps from outside in
	for i := len(mw) - 1; i >= 0; i-- {
		handler = mw[i](handler)
	}
	return handler
}

// Recover middleware recovers from panics and returns 500.
func Recover(logger *zap.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					logger.Error("panic recovered",
						zap.String("layer", "L5"),
						zap.Any("error", err),
						zap.String("request_id", getRequestID(r.Context())),
						zap.String("method", r.Method),
						zap.String("path", r.URL.Path),
					)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					json.NewEncoder(w).Encode(map[string]string{
						"error": "internal server error",
					})
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// requestIDKey is the context key for request ID.
type requestIDKey struct{}

// getRequestID gets the request ID from context.
func getRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey{}).(string); ok {
		return id
	}
	return ""
}

// RequestID middleware generates and attaches a unique request ID to each request.
func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" {
				requestID = uuid.New().String()
			}

			ctx := context.WithValue(r.Context(), requestIDKey{}, requestID)
			w.Header().Set("X-Request-ID", requestID)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// metricsKey is the context key for request start time.
type metricsKey struct{}

// Metrics middleware records request metrics.
func Metrics(metrics *MetricsCollector) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ctx := context.WithValue(r.Context(), metricsKey{}, start)

			// Wrap response writer to capture status code
			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(wrapped, r.WithContext(ctx))

			duration := time.Since(start)
			metrics.RecordRequest(r.URL.Path, r.Method, wrapped.statusCode, duration)
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// MetricsCollector collects HTTP metrics.
type MetricsCollector struct {
	requestsTotal   map[string]int
	requestsTotalMu sync.Mutex // Using simple approach for now
}

// NewMetricsCollector creates a new metrics collector.
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		requestsTotal: make(map[string]int),
	}
}

// RecordRequest records a request.
func (m *MetricsCollector) RecordRequest(path, method string, status int, duration time.Duration) {
	m.requestsTotalMu.Lock()
	defer m.requestsTotalMu.Unlock()
	key := fmt.Sprintf("%s_%s_%d", method, path, status)
	m.requestsTotal[key]++
}

// Logging middleware logs request details.
func Logging(logger *zap.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			requestID := getRequestID(r.Context())

			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(wrapped, r.WithContext(r.Context()))

			duration := time.Since(start)
			logger.Info("request completed",
				zap.String("layer", "L5"),
				zap.String("request_id", requestID),
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", wrapped.statusCode),
				zap.Duration("duration", duration),
				zap.String("remote_addr", r.RemoteAddr),
			)
		})
	}
}

// CORS middleware handles CORS preflight requests and adds CORS headers.
func CORS(allowedOrigins []string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && isOriginAllowed(origin, allowedOrigins) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
				w.Header().Set("Access-Control-Max-Age", "86400")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func isOriginAllowed(origin string, allowed []string) bool {
	if origin == "" {
		return false
	}
	for _, o := range allowed {
		if o == "*" || o == origin {
			return true
		}
	}
	return false
}

// AuthConfig holds JWT auth configuration.
type AuthConfig struct {
	JWTSecret string // JWT secret for decryption (verification done by L3)
}

// Auth middleware decrypts JWT and extracts user context.
// Note: This only decrypts the JWT, verification is done by L3-Authz.
func Auth(cfg AuthConfig) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				// No auth header, continue without user context
				next.ServeHTTP(w, r)
				return
			}

			// Extract Bearer token
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{
					"error": "invalid authorization header format",
				})
				return
			}

			token := parts[1]
			claims, err := decryptJWT(token, cfg.JWTSecret)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{
					"error": "invalid token",
				})
				return
			}

			// Extract user info from claims
			uc := &userContext{
				userID:   getStringClaim(claims, "sub"),
				clientID: getStringClaim(claims, "client_id"),
				claims:   claims,
			}

			ctx := withUserContext(r.Context(), uc)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// decryptJWT decrypts a JWT token without verification.
// This is used by the gateway to extract claims for logging and routing.
// Actual verification is done by L3-Authz.
func decryptJWT(token string, secret string) (map[string]interface{}, error) {
	// JWT format: header.payload.signature
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format")
	}

	// Decode payload (second part)
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode payload: %w", err)
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse claims: %w", err)
	}

	return claims, nil
}

// getStringClaim extracts a string claim from the claims map.
func getStringClaim(claims map[string]interface{}, key string) string {
	if v, ok := claims[key].(string); ok {
		return v
	}
	return ""
}

// ResponseWriter interface for metrics collection.
type ResponseWriter interface {
	WriteHeader(int)
}

// HandlerOption is a functional option for Gateway.
type HandlerOption func(*Gateway)

// WithAuthConfig sets the auth configuration.
func WithAuthConfig(cfg AuthConfig) HandlerOption {
	return func(g *Gateway) {
		g.authConfig = cfg
	}
}

// WithCORSOrigins sets the allowed CORS origins.
func WithCORSOrigins(origins []string) HandlerOption {
	return func(g *Gateway) {
		g.corsOrigins = origins
	}
}