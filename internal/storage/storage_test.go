package storage

import (
	"context"
	"testing"
)

func TestStorageNew(t *testing.T) {
	s := New()
	if s == nil {
		t.Error("expected non-nil Storage")
	}
}

func TestStorageConnect(t *testing.T) {
	s := New()
	err := s.Connect(context.Background())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStorageClose(t *testing.T) {
	s := New()
	err := s.Close(context.Background())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
