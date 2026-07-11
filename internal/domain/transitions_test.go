package domain

import (
	"testing"
)

func TestValidTransitions(t *testing.T) {
	t.Parallel()

	// captured can go to needs_evidence, approved, rejected
	assertCanTransition(t, StatusCaptured, StatusNeedsEvidence)
	assertCanTransition(t, StatusCaptured, StatusApproved)
	assertCanTransition(t, StatusCaptured, StatusRejected)

	// needs_evidence can go to captured, approved, rejected
	assertCanTransition(t, StatusNeedsEvidence, StatusCaptured)
	assertCanTransition(t, StatusNeedsEvidence, StatusApproved)
	assertCanTransition(t, StatusNeedsEvidence, StatusRejected)

	// approved can go to published, rejected, archived, merged
	assertCanTransition(t, StatusApproved, StatusPublished)
	assertCanTransition(t, StatusApproved, StatusRejected)
	assertCanTransition(t, StatusApproved, StatusArchived)
	assertCanTransition(t, StatusApproved, StatusMerged)

	// published can go to superseded, archived
	assertCanTransition(t, StatusPublished, StatusSuperseded)
	assertCanTransition(t, StatusPublished, StatusArchived)

	// superseded can go to archived
	assertCanTransition(t, StatusSuperseded, StatusArchived)

	// merged can go to published, archived
	assertCanTransition(t, StatusMerged, StatusPublished)
	assertCanTransition(t, StatusMerged, StatusArchived)
}

func TestInvalidTransitions(t *testing.T) {
	t.Parallel()

	assertCannotTransition(t, StatusCaptured, StatusPublished)
	assertCannotTransition(t, StatusCaptured, StatusSuperseded)
	assertCannotTransition(t, StatusCaptured, StatusArchived)
	assertCannotTransition(t, StatusCaptured, StatusMerged)

	assertCannotTransition(t, StatusRejected, StatusApproved)
	assertCannotTransition(t, StatusRejected, StatusPublished)

	assertCannotTransition(t, StatusPublished, StatusCaptured)
	assertCannotTransition(t, StatusPublished, StatusApproved)
	assertCannotTransition(t, StatusPublished, StatusRejected)

	assertCannotTransition(t, StatusArchived, StatusCaptured)
	assertCannotTransition(t, StatusArchived, StatusApproved)
	assertCannotTransition(t, StatusArchived, StatusPublished)

	assertCannotTransition(t, StatusSuperseded, StatusPublished)
	assertCannotTransition(t, StatusSuperseded, StatusApproved)

	assertCannotTransition(t, StatusMerged, StatusCaptured)
	assertCannotTransition(t, StatusMerged, StatusRejected)
}

func TestCannotTransitionSameStatus(t *testing.T) {
	t.Parallel()

	assertCannotTransition(t, StatusCaptured, StatusCaptured)
	assertCannotTransition(t, StatusApproved, StatusApproved)
	assertCannotTransition(t, StatusPublished, StatusPublished)
}

func TestMustTransition(t *testing.T) {
	t.Parallel()

	actor := Actor{Kind: "agent", Name: "test", Model: "test-model", SessionID: "sess-001"}

	learning := &Learning{
		ID:       "learn-001",
		Status:   StatusCaptured,
		Revision: 1,
	}

	// Valid transition.
	err := MustTransition(learning, actor, StatusNeedsEvidence)
	if err != nil {
		t.Fatalf("MustTransition: expected no error, got %v", err)
	}
	if learning.Status != StatusNeedsEvidence {
		t.Errorf("Status = %q after MustTransition, want %q", learning.Status, StatusNeedsEvidence)
	}
	if learning.UpdatedAt.IsZero() {
		t.Error("UpdatedAt was not set after MustTransition")
	}
	if learning.Actor.Name != actor.Name {
		t.Errorf("Actor.Name = %q, want %q", learning.Actor.Name, actor.Name)
	}
	if learning.Revision != 2 {
		t.Errorf("Revision = %d, want 2", learning.Revision)
	}
}

func TestMustTransitionInvalid(t *testing.T) {
	t.Parallel()

	actor := Actor{Kind: "agent", Name: "test", Model: "test-model", SessionID: "sess-001"}

	learning := &Learning{
		ID:       "learn-001",
		Status:   StatusCaptured,
		Revision: 1,
	}

	err := MustTransition(learning, actor, StatusPublished)
	if err == nil {
		t.Fatal("MustTransition: expected error for invalid transition, got nil")
	}
	// MustTransition returns *ValidationError which wraps *DomainError.
	if _, ok := err.(*ValidationError); !ok {
		t.Errorf("expected *ValidationError, got %T", err)
	}
}

func TestMustTransitionNilLearning(t *testing.T) {
	t.Parallel()

	err := MustTransition(nil, Actor{}, StatusCaptured)
	if err == nil {
		t.Fatal("expected error for nil learning")
	}
}

// --- helpers ---

func assertCanTransition(t *testing.T, from, to LearningStatus) {
	t.Helper()
	if !CanTransition(from, to) {
		t.Errorf("CanTransition(%q, %q) = false, want true", from, to)
	}
}

func assertCannotTransition(t *testing.T, from, to LearningStatus) {
	t.Helper()
	if CanTransition(from, to) {
		t.Errorf("CanTransition(%q, %q) = true, want false", from, to)
	}
}

// isDomainError reports whether err (or an error in its chain) is a *DomainError.
func isDomainError(err error, target **DomainError) bool {
	if err == nil {
		return false
	}
	if de, ok := err.(*DomainError); ok {
		if target != nil {
			*target = de
		}
		return true
	}
	return false
}
