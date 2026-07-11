package domain

import (
	"fmt"
	"time"
)

// ValidTransitions defines the allowed state machine transitions.
var ValidTransitions = map[LearningStatus][]LearningStatus{
	StatusCaptured:      {StatusNeedsEvidence, StatusApproved, StatusRejected},
	StatusNeedsEvidence: {StatusCaptured, StatusApproved, StatusRejected},
	StatusApproved:      {StatusPublished, StatusRejected, StatusArchived, StatusMerged},
	StatusPublished:     {StatusSuperseded, StatusArchived},
	StatusSuperseded:    {StatusArchived},
	StatusMerged:        {StatusPublished, StatusArchived},
	// rejected and archived are terminal.
	StatusRejected: {},
	StatusArchived: {},
}

// CanTransition reports whether a transition from current to target is allowed.
func CanTransition(from, target LearningStatus) bool {
	if from == target {
		return false
	}
	targets, ok := ValidTransitions[from]
	if !ok {
		return false
	}
	for _, t := range targets {
		if t == target {
			return true
		}
	}
	return false
}

// MustTransition validates and applies a state transition on learning.
// On success it updates the learning's Status, Actor, Revision, and UpdatedAt.
func MustTransition(learning *Learning, actor Actor, newStatus LearningStatus) error {
	if learning == nil {
		return NewValidationError(ErrInvalidArgument, "learning is nil")
	}

	if !CanTransition(learning.Status, newStatus) {
		return NewValidationError(ErrInvalidTransition,
			fmt.Sprintf("cannot transition from %q to %q", learning.Status, newStatus))
	}

	learning.Status = newStatus
	learning.Actor = actor
	learning.Revision++
	learning.UpdatedAt = time.Now().UTC()
	return nil
}
