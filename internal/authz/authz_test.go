package authz

import (
	"context"
	"testing"
)

func TestAuthzNew(t *testing.T) {
	a := New()
	if a == nil {
		t.Error("expected non-nil Authz")
	}
}

func TestAuthzCheckPermission(t *testing.T) {
	a := New()
	err := a.CheckPermission(context.Background(), "user", "read", "task")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAuthzValidateToken(t *testing.T) {
	a := New()
	claims, err := a.ValidateToken(context.Background(), "token")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if claims != nil {
		t.Error("expected nil claims for empty implementation")
	}
}
