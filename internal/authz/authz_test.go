package authz

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestAuthzNew(t *testing.T) {
	a := New()
	require.NotNil(t, a)
	assert.Equal(t, "X-API-Key", a.cfg.APIKeyHeader)
	assert.Equal(t, 5*time.Minute, a.cfg.CacheTTL)
	assert.Equal(t, 1*time.Hour, a.cfg.TokenExpiry)
	assert.Equal(t, 7*24*time.Hour, a.cfg.RefreshExpiry)
}

func TestAuthzNewWithConfig(t *testing.T) {
	logger := zap.NewNop()
	cfg := AuthzConfig{
		JWTSecret:    "test-secret",
		APIKeyHeader: "X-Custom-Key",
		CacheTTL:     10 * time.Minute,
		TokenExpiry:  2 * time.Hour,
	}
	a := NewWithConfig(cfg, logger)
	require.NotNil(t, a)
	assert.Equal(t, "test-secret", a.cfg.JWTSecret)
	assert.Equal(t, "X-Custom-Key", a.cfg.APIKeyHeader)
}

func TestAuthzGenerateAndValidateToken(t *testing.T) {
	a := NewWithConfig(AuthzConfig{
		JWTSecret:    "test-secret-key-min-32-chars!!",
		TokenExpiry:  1 * time.Hour,
		RefreshExpiry: 7 * 24 * time.Hour,
	}, nil)

	// Generate token
	token, _, err := a.GenerateToken("user123", RoleUser, []string{"task:read", "task:write"})
	require.NoError(t, err)
	require.NotEmpty(t, token)

	// Validate token
	claims, err := a.ValidateToken(context.Background(), token)
	require.NoError(t, err)
	require.NotNil(t, claims)

	assert.Equal(t, "user123", claims.Subject)
	assert.Equal(t, RoleUser, claims.Role)
	assert.True(t, claims.HasScope("task:read"))
	assert.True(t, claims.HasScope("task:write"))
	assert.False(t, claims.HasScope("task:delete"))
}

func TestAuthzValidateToken_Expired(t *testing.T) {
	a := NewWithConfig(AuthzConfig{
		JWTSecret:   "test-secret-key-min-32-chars!!",
		TokenExpiry: -1 * time.Hour, // expired
	}, nil)

	token, _, err := a.GenerateToken("user123", RoleUser, nil)
	require.NoError(t, err)

	_, err = a.ValidateToken(context.Background(), token)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "validate token")
}

func TestAuthzValidateToken_Invalid(t *testing.T) {
	a := NewWithConfig(AuthzConfig{
		JWTSecret: "test-secret-key-min-32-chars!!",
	}, nil)

	_, err := a.ValidateToken(context.Background(), "invalid-token")
	assert.Error(t, err)
}

func TestAuthzValidateToken_WrongSecret(t *testing.T) {
	a1 := NewWithConfig(AuthzConfig{
		JWTSecret:   "secret-one-min-32-characters!!!",
		TokenExpiry: 1 * time.Hour,
	}, nil)

	token, _, err := a1.GenerateToken("user123", RoleUser, nil)
	require.NoError(t, err)

	a2 := NewWithConfig(AuthzConfig{
		JWTSecret:   "different-secret-min-32-chars!!!",
		TokenExpiry: 1 * time.Hour,
	}, nil)

	_, err = a2.ValidateToken(context.Background(), token)
	assert.Error(t, err)
}

func TestAuthzRS256(t *testing.T) {
	// Generate RSA key pair
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	a := NewWithConfig(AuthzConfig{
		TokenExpiry:  1 * time.Hour,
		RefreshExpiry: 7 * 24 * time.Hour,
	}, nil)
	a.AddRS256PrivateKey("test-key", privateKey)

	// Generate token with RS256
	token, _, err := a.GenerateToken("userRS256", RoleAdmin, nil)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	// Validate token
	claims, err := a.ValidateToken(context.Background(), token)
	require.NoError(t, err)
	assert.Equal(t, "userRS256", claims.Subject)
	assert.Equal(t, RoleAdmin, claims.Role)
}

func TestAuthzGenerateRefreshToken(t *testing.T) {
	a := NewWithConfig(AuthzConfig{
		JWTSecret:    "test-secret-key-min-32-chars!!",
		RefreshExpiry: 7 * 24 * time.Hour,
	}, nil)

	refreshToken, err := a.GenerateRefreshToken("user123", RoleUser)
	require.NoError(t, err)
	require.NotEmpty(t, refreshToken)

	claims, err := a.ValidateRefreshToken(context.Background(), refreshToken)
	require.NoError(t, err)
	assert.Equal(t, "user123", claims.Subject)
	assert.Equal(t, RoleUser, claims.Role)
}

func TestAuthzCheckPermission(t *testing.T) {
	a := NewWithConfig(AuthzConfig{
		JWTSecret: "test-secret-key-min-32-chars!!",
		CacheTTL:  5 * time.Minute,
	}, nil)

	tests := []struct {
		name     string
		role     Role
		action   Action
		resource Resource
		wantErr  bool
	}{
		// Admin can do everything
		{"admin read task", RoleAdmin, ActionRead, ResourceTask, false},
		{"admin write task", RoleAdmin, ActionWrite, ResourceTask, false},
		{"admin delete task", RoleAdmin, ActionDelete, ResourceTask, false},
		{"admin admin config", RoleAdmin, ActionAdmin, ResourceConfig, false},

		// User permissions
		{"user read task", RoleUser, ActionRead, ResourceTask, false},
		{"user write task", RoleUser, ActionWrite, ResourceTask, false},
		{"user delete task", RoleUser, ActionDelete, ResourceTask, true},
		{"user admin config", RoleUser, ActionAdmin, ResourceConfig, true},
		{"user read agent", RoleUser, ActionRead, ResourceAgent, false},

		// Agent permissions
		{"agent read task", RoleAgent, ActionRead, ResourceTask, false},
		{"agent write task", RoleAgent, ActionWrite, ResourceTask, false},
		{"agent exec task", RoleAgent, ActionExec, ResourceTask, false},
		{"agent delete task", RoleAgent, ActionDelete, ResourceTask, true},
		{"agent admin config", RoleAgent, ActionAdmin, ResourceConfig, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := a.CheckPermission(context.Background(), "subject", tt.role, tt.action, tt.resource)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAuthzAPIKey(t *testing.T) {
	logger := zap.NewNop()
	a := NewWithConfig(AuthzConfig{
		JWTSecret:   "test-secret-key-min-32-chars!!",
		APIKeyHeader: "X-API-Key",
	}, logger)

	// Add API key
	a.AddAPIKey(APIKeyMeta{
		KeyID:     "key1",
		Secret:    "secret1",
		Role:      RoleUser,
		Scopes:    []string{"task:read", "task:write"},
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	// Validate valid API key
	meta, err := a.ValidateAPIKey(context.Background(), "key1", "secret1")
	require.NoError(t, err)
	assert.Equal(t, RoleUser, meta.Role)

	// Invalid secret
	_, err = a.ValidateAPIKey(context.Background(), "key1", "wrong-secret")
	assert.Error(t, err)

	// Expired API key
	a.AddAPIKey(APIKeyMeta{
		KeyID:     "expired-key",
		Secret:    "secret",
		Role:      RoleUser,
		IssuedAt:  time.Now().Add(-48 * time.Hour),
		ExpiresAt: time.Now().Add(-24 * time.Hour),
	})
	_, err = a.ValidateAPIKey(context.Background(), "expired-key", "secret")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expired")

	// Non-existent key
	_, err = a.ValidateAPIKey(context.Background(), "nonexistent", "secret")
	assert.Error(t, err)
}

func TestAuthzHashAPIKey(t *testing.T) {
	hash1 := HashAPIKey("key1:secret1")
	hash2 := HashAPIKey("key1:secret1")
	hash3 := HashAPIKey("key1:secret2")

	assert.Equal(t, hash1, hash2)
	assert.NotEqual(t, hash1, hash3)
	assert.Len(t, hash1, 64) // SHA256 produces 64 hex characters
}

func TestClaimsHasScope(t *testing.T) {
	claims := &Claims{Scope: "task:read task:write session:read"}

	assert.True(t, claims.HasScope("task:read"))
	assert.True(t, claims.HasScope("task:write"))
	assert.True(t, claims.HasScope("session:read"))
	assert.False(t, claims.HasScope("task:delete"))
	assert.False(t, claims.HasScope(""))
}

func TestClaimsShouldRefresh(t *testing.T) {
	// Token that should be refreshed
	futureRefreshAt := time.Now().Add(1 * time.Hour).Unix()
	claimsFuture := &Claims{RefreshAt: futureRefreshAt}
	assert.False(t, claimsFuture.ShouldRefresh())

	// Token that needs refresh
	pastRefreshAt := time.Now().Add(-1 * time.Hour).Unix()
	claimsPast := &Claims{RefreshAt: pastRefreshAt}
	assert.True(t, claimsPast.ShouldRefresh())

	// Token with no refresh time
	claimsNoRefresh := &Claims{RefreshAt: 0}
	assert.False(t, claimsNoRefresh.ShouldRefresh())
}

func TestContextWithClaims(t *testing.T) {
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: "user123"},
		Role:             RoleUser,
	}

	ctx := ContextWithClaims(context.Background(), claims)

	retrieved, ok := ClaimsFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, "user123", retrieved.Subject)
	assert.Equal(t, RoleUser, retrieved.Role)

	// Empty context
	_, ok = ClaimsFromContext(context.Background())
	assert.False(t, ok)
}

func TestInterceptor(t *testing.T) {
	logger := zap.NewNop()
	authz := NewWithConfig(AuthzConfig{
		JWTSecret:    "test-secret-key-min-32-chars!!",
		APIKeyHeader: "X-API-Key",
		TokenExpiry:  1 * time.Hour,
	}, logger)

	interceptor := NewInterceptor(InterceptorConfig{
		Authz:        authz,
		SkipPaths:    map[string]bool{"/health": true},
		APIKeyHeader: "X-API-Key",
	}, logger)

	assert.NotNil(t, interceptor)
}

// Test that Interceptor implements connect.Interceptor
func TestInterceptorImplementsInterface(t *testing.T) {
	var _ connect.Interceptor = (*Interceptor)(nil)
}
