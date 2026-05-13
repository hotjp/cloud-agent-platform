package gateway

import (
	"context"
	"testing"
)

func TestGatewayNew(t *testing.T) {
	g := New()
	if g == nil {
		t.Error("expected non-nil Gateway")
	}
}

func TestGatewayStart(t *testing.T) {
	g := New()
	err := g.Start(context.Background())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGatewayStop(t *testing.T) {
	g := New()
	err := g.Stop(context.Background())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
