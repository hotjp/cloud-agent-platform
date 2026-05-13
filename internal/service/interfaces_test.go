package service

import (
	"context"
	"testing"
)

func TestServiceNew(t *testing.T) {
	svc := New()
	if svc == nil {
		t.Error("expected non-nil Service")
	}
}

func TestServiceInitialize(t *testing.T) {
	svc := New()
	err := svc.Initialize(context.Background())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestServiceShutdown(t *testing.T) {
	svc := New()
	err := svc.Shutdown(context.Background())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
