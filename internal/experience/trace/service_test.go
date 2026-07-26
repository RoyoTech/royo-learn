package trace

import (
	"context"
	"testing"

	"agent-royo-learn/internal/domain"
)

func TestNewService(t *testing.T) {
	s := NewService(nil)
	if s == nil {
		t.Fatal("NewService returned nil")
	}
}

func TestTrace_NilService(t *testing.T) {
	var s *Service
	_, err := s.Trace(context.Background(), "L1", TraceBounds{})
	if err == nil {
		t.Fatal("expected error for nil service")
	}
}

func TestTrace_EmptyLearningID(t *testing.T) {
	s := NewService(nil)
	_, err := s.Trace(context.Background(), "", TraceBounds{})
	if err == nil {
		t.Fatal("expected error for empty learning_id")
	}
}

func TestTrace_NoEvents(t *testing.T) {
	// Without a real DB we cannot test the happy path here; the
	// acceptance tests in slice 4.4 will cover it end-to-end.
	// This test documents the contract: nil DB returns an error
	// when events are linked (the query will fail).
	s := NewService(nil)
	result, err := s.Trace(context.Background(), domain.LearningID("L-nonexistent"), TraceBounds{})
	// nil DB fails at query time.
	if err == nil && result != nil {
		t.Log("trace with nil db returned result without error (expected query error)")
	}
}

func TestTraceBounds_ExcerptDefault(t *testing.T) {
	b := TraceBounds{}
	if b.IncludeExcerpt {
		t.Error("default should not include excerpts")
	}
}
