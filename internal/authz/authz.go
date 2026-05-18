// Package authz implements L3-Authz layer: permission checks (RBAC),
// rate limiting (sentinel-go), and identity verification.
package authz

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

// Role represents a user role in the system.
type Role string

const (
	RoleAdmin  Role = "admin"
	RoleUser   Role = "user"
	RoleAgent  Role = "agent"
)

// Resource represents a protected resource.
type Resource string

const (
	ResourceTask      Resource = "task"
	ResourceSession   Resource = "session"
	ResourceAgent     Resource = "agent"
	ResourceTemplate  Resource = "template"
	ResourceContext   Resource = "context"
	ResourceAuditLog  Resource = "auditlog"
	ResourceConfig    Resource = "config"
)

// Action represents an operation on a resource.
type Action string

const (
	ActionRead   Action = "read"
	ActionWrite  Action = "write"
	ActionDelete Action = "delete"
	ActionExec   Action = "exec"
	ActionAdmin  Action = "admin"
)

// Claims represents JWT claims structure.
type Claims struct {
	jwt.RegisteredClaims
	Role      Role              `json:"role,omitempty"`
	APIKeyID  string            `json:"api_key_id,omitempty"`
	Scope     string            `json:"scope,omitempty"`
	RefreshAt int64             `json:"refresh_at,omitempty"`
}

// AuthzConfig holds the configuration for the authz service.
type AuthzConfig struct {
	// JWTSecret is the secret key for HS256 signing
	JWTSecret string
	// RS256Keys holds RSA public keys for RS256 verification (key ID -> public key)
	RS256Keys map[string]*rsa.PublicKey
	// RS256PrivateKeys holds RSA private keys for RS256 signing (key ID -> private key)
	RS256PrivateKeys map[string]*rsa.PrivateKey
	// APIKeys holds valid API keys (key hash -> metadata)
	APIKeys map[string]APIKeyMeta
	// APIKeyHeader is the header name for API key authentication
	APIKeyHeader string
	// CacheTTL is the TTL for permission cache
	CacheTTL time.Duration
	// TokenExpiry is the JWT token expiry duration
	TokenExpiry time.Duration
	// RefreshExpiry is the refresh token expiry duration
	RefreshExpiry time.Duration
}

// APIKeyMeta holds metadata for an API key.
type APIKeyMeta struct {
	KeyID    string    `json:"key_id"`
	Secret   string    `json:"secret"` // hashed secret
	Role     Role      `json:"role"`
	Scopes   []string  `json:"scopes"`
	IssuedAt time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// InterceptorConfig holds the configuration for the authz interceptor.
type InterceptorConfig struct {
	Authz        *Authz
	SkipPaths    map[string]bool
	APIKeyHeader string
}

// Authz handles authorization and rate limiting.
type Authz struct {
	cfg        AuthzConfig
	logger     *zap.Logger
	permCache  map[string]bool
}

// New creates a new Authz instance with default config.
func New() *Authz {
	return &Authz{
		cfg: AuthzConfig{
			APIKeyHeader: "X-API-Key",
			CacheTTL:     5 * time.Minute,
			TokenExpiry:  1 * time.Hour,
			RefreshExpiry: 7 * 24 * time.Hour,
			RS256Keys:    make(map[string]*rsa.PublicKey),
			APIKeys:      make(map[string]APIKeyMeta),
		},
		logger:    zap.NewNop(),
		permCache: make(map[string]bool),
	}
}

// NewWithConfig creates a new Authz instance with configuration.
func NewWithConfig(cfg AuthzConfig, logger *zap.Logger) *Authz {
	if logger == nil {
		logger = zap.NewNop()
	}
	if cfg.RS256Keys == nil {
		cfg.RS256Keys = make(map[string]*rsa.PublicKey)
	}
	if cfg.RS256PrivateKeys == nil {
		cfg.RS256PrivateKeys = make(map[string]*rsa.PrivateKey)
	}
	if cfg.APIKeys == nil {
		cfg.APIKeys = make(map[string]APIKeyMeta)
	}
	return &Authz{
		cfg:        cfg,
		logger:     logger,
		permCache:  make(map[string]bool),
	}
}

// AddRS256Key adds an RSA public key for RS256 token verification.
func (a *Authz) AddRS256Key(keyID string, key *rsa.PublicKey) {
	a.cfg.RS256Keys[keyID] = key
}

// AddRS256PrivateKey adds an RSA private key for RS256 token signing.
func (a *Authz) AddRS256PrivateKey(keyID string, key *rsa.PrivateKey) {
	a.cfg.RS256PrivateKeys[keyID] = key
	// Also add the public key for verification
	a.cfg.RS256Keys[keyID] = &key.PublicKey
}

// AddAPIKey adds an API key for authentication.
func (a *Authz) AddAPIKey(meta APIKeyMeta) {
	hash := HashAPIKey(meta.KeyID + ":" + meta.Secret)
	a.cfg.APIKeys[hash] = meta
}

// HashAPIKey returns a SHA256 hash of the API key for storage.
func HashAPIKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

// GenerateToken generates a new JWT token for a subject with the given role.
func (a *Authz) GenerateToken(subject string, role Role, scopes []string) (string, string, error) {
	now := time.Now()
	expiresAt := now.Add(a.cfg.TokenExpiry)
	refreshAt := now.Add(a.cfg.RefreshExpiry).Unix()

	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "cloud-agent-platform",
			Audience:  jwt.ClaimStrings{"cap"},
		},
		Role:      role,
		Scope:     strings.Join(scopes, " "),
		RefreshAt: refreshAt,
	}

	// Use HS256 if no RS256 keys configured
	if len(a.cfg.RS256Keys) == 0 && a.cfg.JWTSecret != "" {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		signed, err := token.SignedString([]byte(a.cfg.JWTSecret))
		if err != nil {
			return "", "", fmt.Errorf("sign token: %w", err)
		}
		return signed, "", nil
	}

	// Use RS256 with first available private key
	for keyID, privKey := range a.cfg.RS256PrivateKeys {
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		token.Header["kid"] = keyID
		signed, err := token.SignedString(privKey)
		if err != nil {
			return "", "", fmt.Errorf("sign token with RS256: %w", err)
		}
		return signed, "", nil
	}

	return "", "", errors.New("no signing key available (HS256 secret or RS256 private key required)")
}

// GenerateRefreshToken generates a refresh token.
func (a *Authz) GenerateRefreshToken(subject string, role Role) (string, error) {
	now := time.Now()
	expiresAt := now.Add(a.cfg.RefreshExpiry)

	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "cloud-agent-platform",
			Audience:  jwt.ClaimStrings{"cap-refresh"},
		},
		Role: role,
	}

	// Use HS256 if no RS256 private keys configured
	if len(a.cfg.RS256PrivateKeys) == 0 && a.cfg.JWTSecret != "" {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		signed, err := token.SignedString([]byte(a.cfg.JWTSecret))
		if err != nil {
			return "", fmt.Errorf("sign refresh token: %w", err)
		}
		return signed, nil
	}

	// Use RS256 with first available private key
	for keyID, privKey := range a.cfg.RS256PrivateKeys {
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		token.Header["kid"] = keyID
		signed, err := token.SignedString(privKey)
		if err != nil {
			return "", fmt.Errorf("sign refresh token with RS256: %w", err)
		}
		return signed, nil
	}

	return "", errors.New("no signing key available for refresh token")
}

// ValidateToken validates the JWT token and returns claims.
func (a *Authz) ValidateToken(ctx context.Context, tokenString string) (*Claims, error) {
	// Parse token without validation first to get the key ID
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	token, _, err := parser.ParseUnverified(tokenString, &Claims{})
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	// Determine signing method and key
	var key interface{}
	signingMethod := token.Method.Alg()

	switch signingMethod {
	case "HS256":
		if a.cfg.JWTSecret == "" {
			return nil, errors.New("HS256 secret not configured")
		}
		key = []byte(a.cfg.JWTSecret)
	case "RS256":
		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, errors.New("RS256 token missing kid header")
		}
		var exists bool
		key, exists = a.cfg.RS256Keys[kid]
		if !exists {
			return nil, fmt.Errorf("unknown RSA key id: %s", kid)
		}
	default:
		return nil, fmt.Errorf("unsupported signing method: %s", signingMethod)
	}

	// Validate token
	validatedToken, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		return key, nil
	}, jwt.WithIssuer("cloud-agent-platform"), jwt.WithAudience("cap"))

	if err != nil {
		return nil, fmt.Errorf("validate token: %w", err)
	}

	validatedClaims, ok := validatedToken.Claims.(*Claims)
	if !ok || !validatedToken.Valid {
		return nil, errors.New("invalid token")
	}

	return validatedClaims, nil
}

// ValidateRefreshToken validates a refresh token and returns claims.
func (a *Authz) ValidateRefreshToken(ctx context.Context, tokenString string) (*Claims, error) {
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	token, _, err := parser.ParseUnverified(tokenString, &Claims{})
	if err != nil {
		return nil, fmt.Errorf("parse refresh token: %w", err)
	}

	var key interface{}
	signingMethod := token.Method.Alg()

	switch signingMethod {
	case "HS256":
		if a.cfg.JWTSecret == "" {
			return nil, errors.New("HS256 secret not configured")
		}
		key = []byte(a.cfg.JWTSecret)
	case "RS256":
		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, errors.New("RS256 token missing kid header")
		}
		var exists bool
		key, exists = a.cfg.RS256Keys[kid]
		if !exists {
			return nil, fmt.Errorf("unknown RSA key id: %s", kid)
		}
	default:
		return nil, fmt.Errorf("unsupported signing method: %s", signingMethod)
	}

	validatedToken, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		return key, nil
	}, jwt.WithIssuer("cloud-agent-platform"), jwt.WithAudience("cap-refresh"))

	if err != nil {
		return nil, fmt.Errorf("validate refresh token: %w", err)
	}

	validatedClaims, ok := validatedToken.Claims.(*Claims)
	if !ok || !validatedToken.Valid {
		return nil, errors.New("invalid refresh token")
	}

	return validatedClaims, nil
}

// ValidateAPIKey validates an API key and returns its metadata.
func (a *Authz) ValidateAPIKey(ctx context.Context, keyID, secret string) (*APIKeyMeta, error) {
	hash := HashAPIKey(keyID + ":" + secret)
	meta, exists := a.cfg.APIKeys[hash]
	if !exists {
		return nil, errors.New("invalid API key")
	}

	if meta.ExpiresAt.Before(time.Now()) {
		return nil, errors.New("API key expired")
	}

	return &meta, nil
}

// CheckPermission checks if the request has permission to access the resource.
func (a *Authz) CheckPermission(ctx context.Context, subject, role Role, action Action, resource Resource) error {
	// Admin role has all permissions
	if role == RoleAdmin {
		return nil
	}

	// Build permission cache key
	cacheKey := fmt.Sprintf("%s:%s:%s:%s", subject, role, action, resource)

	// Check permission cache
	if a.cfg.CacheTTL > 0 {
		if allowed, exists := a.permCache[cacheKey]; exists && allowed {
			return nil
		}
	}

	// Define role-based permissions
	perms := a.getRolePermissions(role)

	allowed := false
	for _, perm := range perms {
		if perm.Action == action && perm.Resource == resource {
			allowed = true
			break
		}
	}

	if !allowed {
		return fmt.Errorf("permission denied: %s cannot perform %s on %s", role, action, resource)
	}

	// Cache the result
	if a.cfg.CacheTTL > 0 {
		a.permCache[cacheKey] = true
	}

	return nil
}

// Permission represents a single permission rule.
type Permission struct {
	Action   Action
	Resource Resource
}

// getRolePermissions returns the permissions for a given role.
func (a *Authz) getRolePermissions(role Role) []Permission {
	switch role {
	case RoleAdmin:
		return []Permission{
			{Action: ActionRead, Resource: ResourceTask},
			{Action: ActionWrite, Resource: ResourceTask},
			{Action: ActionDelete, Resource: ResourceTask},
			{Action: ActionExec, Resource: ResourceTask},
			{Action: ActionRead, Resource: ResourceSession},
			{Action: ActionWrite, Resource: ResourceSession},
			{Action: ActionDelete, Resource: ResourceSession},
			{Action: ActionRead, Resource: ResourceAgent},
			{Action: ActionWrite, Resource: ResourceAgent},
			{Action: ActionDelete, Resource: ResourceAgent},
			{Action: ActionRead, Resource: ResourceTemplate},
			{Action: ActionWrite, Resource: ResourceTemplate},
			{Action: ActionDelete, Resource: ResourceTemplate},
			{Action: ActionRead, Resource: ResourceContext},
			{Action: ActionWrite, Resource: ResourceContext},
			{Action: ActionDelete, Resource: ResourceContext},
			{Action: ActionRead, Resource: ResourceAuditLog},
			{Action: ActionAdmin, Resource: ResourceConfig},
		}
	case RoleUser:
		return []Permission{
			{Action: ActionRead, Resource: ResourceTask},
			{Action: ActionWrite, Resource: ResourceTask},
			{Action: ActionRead, Resource: ResourceSession},
			{Action: ActionWrite, Resource: ResourceSession},
			{Action: ActionRead, Resource: ResourceAgent},
			{Action: ActionRead, Resource: ResourceTemplate},
			{Action: ActionRead, Resource: ResourceContext},
			{Action: ActionWrite, Resource: ResourceContext},
		}
	case RoleAgent:
		return []Permission{
			{Action: ActionRead, Resource: ResourceTask},
			{Action: ActionWrite, Resource: ResourceTask},
			{Action: ActionExec, Resource: ResourceTask},
			{Action: ActionRead, Resource: ResourceSession},
			{Action: ActionWrite, Resource: ResourceSession},
			{Action: ActionRead, Resource: ResourceContext},
			{Action: ActionWrite, Resource: ResourceContext},
		}
	default:
		return nil
	}
}

// HasScope checks if the claims have a specific scope.
func (c *Claims) HasScope(scope string) bool {
	if c.Scope == "" {
		return false
	}
	scopes := strings.Split(c.Scope, " ")
	for _, s := range scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// ShouldRefresh checks if the token should be refreshed.
func (c *Claims) ShouldRefresh() bool {
	if c.RefreshAt == 0 {
		return false
	}
	return time.Now().Unix() >= c.RefreshAt
}

// Interceptor provides authz interceptors for connect-go.
type Interceptor struct {
	cfg        InterceptorConfig
	logger     *zap.Logger
	skipPaths  map[string]bool
}

// NewInterceptor creates a new authz interceptor.
func NewInterceptor(cfg InterceptorConfig, logger *zap.Logger) *Interceptor {
	if logger == nil {
		logger = zap.NewNop()
	}
	skipPaths := make(map[string]bool)
	for path := range cfg.SkipPaths {
		skipPaths[path] = true
	}
	return &Interceptor{
		cfg:       cfg,
		logger:    logger,
		skipPaths: skipPaths,
	}
}

// WrapUnary implements connect.Interceptor.
func (i *Interceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		// Get the full method name
		procedure := req.Spec().Procedure
		// Format: /service/method
		if i.skipPaths[procedure] {
			return next(ctx, req)
		}

		// Extract token from metadata
		md := req.Header()
		var claims *Claims
		var err error

		// Try API key first
		apiKeyHeader := i.cfg.APIKeyHeader
		if apiKeyHeader == "" {
			apiKeyHeader = "X-API-Key"
		}

		if apiKey := md.Get(apiKeyHeader); apiKey != "" {
			claims, err = i.validateAPIKeyAuth(ctx, apiKey)
			if err != nil {
				i.logger.Debug("API key auth failed", zap.Error(err))
				return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("invalid API key: %w", err))
			}
		} else if authHeader := md.Get("Authorization"); authHeader != "" {
			// Try Bearer token
			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token == authHeader {
				return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("missing Bearer prefix"))
			}
			claims, err = i.cfg.Authz.ValidateToken(ctx, token)
			if err != nil {
				i.logger.Debug("JWT auth failed", zap.Error(err))
				return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("invalid token: %w", err))
			}
		} else {
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("missing authentication"))
		}

		// Check if token needs refresh
		if claims.ShouldRefresh() {
			i.logger.Info("token should be refreshed", zap.String("subject", claims.Subject))
		}

		// Store claims in context for later use
		ctx = ContextWithClaims(ctx, claims)

		return next(ctx, req)
	}
}

// WrapStreamingClient implements connect.Interceptor.
func (i *Interceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

// WrapStreamingHandler implements connect.Interceptor.
func (i *Interceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}

// validateAPIKeyAuth validates API key authentication.
func (i *Interceptor) validateAPIKeyAuth(ctx context.Context, apiKey string) (*Claims, error) {
	// API key format: keyID:secret
	parts := strings.SplitN(apiKey, ":", 2)
	if len(parts) != 2 {
		return nil, errors.New("invalid API key format")
	}

	meta, err := i.cfg.Authz.ValidateAPIKey(ctx, parts[0], parts[1])
	if err != nil {
		return nil, err
	}

	return &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: meta.KeyID,
		},
		Role:    meta.Role,
		Scope:   strings.Join(meta.Scopes, " "),
	}, nil
}

// ConnectRPCInterceptor returns a connect.UnaryInterceptorFunc for use with connect-go.
func (a *Authz) ConnectRPCInterceptor(logger *zap.Logger) connect.UnaryInterceptorFunc {
	interceptor := NewInterceptor(InterceptorConfig{
		Authz:        a,
		SkipPaths:    make(map[string]bool),
		APIKeyHeader: a.cfg.APIKeyHeader,
	}, logger)
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return interceptor.WrapUnary(next)
	}
}

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

const claimsContextKey contextKey = "authz.claims"

// ContextWithClaims stores claims in the context.
func ContextWithClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, claimsContextKey, claims)
}

// ClaimsFromContext retrieves claims from the context.
func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	claims, ok := ctx.Value(claimsContextKey).(*Claims)
	return claims, ok
}
