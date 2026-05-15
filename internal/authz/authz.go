// Package authz implements L3-Authz layer: permission checks (RBAC),
// rate limiting (sentinel-go), and identity verification.
package authz

import (
	"context"
	"time"

	"connectrpc.com/connect"
)

// Config holds the configuration for the authz service.
type Config struct {
	JWTSecret    string
	APIKeyHeader string
	CacheTTL     time.Duration
}

// InterceptorConfig holds the configuration for the authz interceptor.
type InterceptorConfig struct {
	Authz        *Authz
	SkipPaths    map[string]bool
	APIKeyHeader string
}

// Authz handles authorization and rate limiting.
type Authz struct {
	cfg Config
}

// New creates a new Authz instance.
func New() *Authz {
	return &Authz{}
}

// NewWithConfig creates a new Authz instance with configuration.
func NewWithConfig(cfg Config) *Authz {
	return &Authz{cfg: cfg}
}

// CheckPermission checks if the request has permission to access the resource.
func (a *Authz) CheckPermission(ctx context.Context, subject, action, resource string) error {
	// TODO: Implement permission check
	return nil
}

// ValidateToken validates the JWT token and returns claims.
func (a *Authz) ValidateToken(ctx context.Context, token string) (map[string]interface{}, error) {
	// TODO: Implement token validation
	return nil, nil
}

// Interceptor provides authz interceptors for connect-go.
// This is a minimal implementation that satisfies the connect.Interceptor interface.
type Interceptor struct {
	cfg InterceptorConfig
}

// NewInterceptor creates a new authz interceptor.
func NewInterceptor(cfg InterceptorConfig, logger interface{}) *Interceptor {
	return &Interceptor{cfg: cfg}
}

// WrapUnary implements connect.Interceptor.
func (i *Interceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return next
}
