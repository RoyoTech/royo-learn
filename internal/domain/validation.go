package domain

import (
	"fmt"
)

// Validate checks a Learning for required fields and constraints.
func Validate(learning *Learning) error {
	if learning == nil {
		return NewValidationError(ErrInvalidArgument, "learning is nil")
	}

	if learning.ID == "" {
		return NewValidationError(ErrInvalidArgument, "learning id is required")
	}
	if learning.ProjectID == "" {
		return NewValidationError(ErrInvalidArgument, "project id is required")
	}
	if learning.Title == "" {
		return NewValidationError(ErrInvalidArgument, "title is required")
	}
	if learning.Context == "" {
		return NewValidationError(ErrInvalidArgument, "context is required")
	}
	if learning.Observation == "" {
		return NewValidationError(ErrInvalidArgument, "observation is required")
	}
	if learning.ReusableLesson == "" {
		return NewValidationError(ErrInvalidArgument, "reusable_lesson is required")
	}

	// Enums must be valid.
	if !isValidStatus(learning.Status) {
		return NewValidationError(ErrInvalidArgument,
			fmt.Sprintf("invalid status: %q", learning.Status))
	}
	if !isValidType(learning.Type) {
		return NewValidationError(ErrInvalidArgument,
			fmt.Sprintf("invalid type: %q", learning.Type))
	}
	if !isValidScope(learning.ScopeGuess) {
		return NewValidationError(ErrInvalidArgument,
			fmt.Sprintf("invalid scope: %q", learning.ScopeGuess))
	}
	if !isValidConfidence(learning.Confidence) {
		return NewValidationError(ErrInvalidArgument,
			fmt.Sprintf("invalid confidence: %q", learning.Confidence))
	}
	if !isValidEvidenceLevel(learning.EvidenceLevel) {
		return NewValidationError(ErrInvalidArgument,
			fmt.Sprintf("invalid evidence_level: %q", learning.EvidenceLevel))
	}
	if !isValidDestination(learning.ProposedDestination) {
		return NewValidationError(ErrInvalidArgument,
			fmt.Sprintf("invalid proposed_destination: %q", learning.ProposedDestination))
	}

	// Preference type cannot be published as shared/agents rule.
	if learning.Type == TypePreference {
		if learning.ProposedDestination == DestShared ||
			learning.ProposedDestination == DestAgentsRule {
			return NewValidationError(ErrInvalidArgument,
				"learning type 'preference' cannot be published as shared or agents_rule without explicit user decision")
		}
	}

	return nil
}

// ValidateEvidence checks an Evidence record for required fields.
func ValidateEvidence(e *Evidence) error {
	if e == nil {
		return NewValidationError(ErrInvalidArgument, "evidence is nil")
	}
	if e.ID == "" {
		return NewValidationError(ErrInvalidArgument, "evidence id is required")
	}
	if e.LearningID == "" {
		return NewValidationError(ErrInvalidArgument, "evidence learning_id is required")
	}
	if e.Kind == "" {
		return NewValidationError(ErrInvalidArgument, "evidence kind is required")
	}
	if !isValidEvidenceKind(e.Kind) {
		return NewValidationError(ErrInvalidArgument,
			fmt.Sprintf("invalid evidence kind: %q", e.Kind))
	}
	if e.Summary == "" {
		return NewValidationError(ErrInvalidArgument, "evidence summary is required")
	}
	return nil
}

// --- private validators ---

func isValidStatus(s LearningStatus) bool {
	switch s {
	case StatusCaptured, StatusNeedsEvidence, StatusApproved, StatusRejected,
		StatusMerged, StatusPublished, StatusSuperseded, StatusArchived:
		return true
	}
	return false
}

func isValidType(t LearningType) bool {
	switch t {
	case TypeProcedure, TypePrevention, TypeDiagnostic, TypeTooling,
		TypeArchitecture, TypeQuality, TypeSecurity, TypeHypothesis, TypePreference:
		return true
	}
	return false
}

func isValidScope(s Scope) bool {
	switch s {
	case ScopeProject, ScopeShared, ScopePersonal, ScopeUnknown:
		return true
	}
	return false
}

func isValidConfidence(c Confidence) bool {
	switch c {
	case ConfidenceHigh, ConfidenceMedium, ConfidenceLow:
		return true
	}
	return false
}

func isValidEvidenceLevel(el EvidenceLevel) bool {
	switch el {
	case EvidenceStrong, EvidenceModerate, EvidenceWeak, EvidenceInsufficient:
		return true
	}
	return false
}

// IsValidEvidenceLevel reports whether el is a known evidence level. Public
// interfaces validate their input against this rather than defining their own
// list.
func IsValidEvidenceLevel(el EvidenceLevel) bool { return isValidEvidenceLevel(el) }

// IsValidEvidenceKind reports whether k is a known evidence kind. Public
// interfaces validate their input against this rather than defining their own
// list.
func IsValidEvidenceKind(k EvidenceKind) bool { return isValidEvidenceKind(k) }

func isValidDestination(d DestinationType) bool {
	switch d {
	case DestNone, DestProject, DestShared, DestSkill, DestAgentsRule:
		return true
	}
	return false
}

func isValidEvidenceKind(k EvidenceKind) bool {
	switch k {
	case KindFile, KindGitDiff, KindGitCommit, KindCommand, KindTest,
		KindEngramObservation, KindIssue, KindPullRequest, KindText, KindExternalReference:
		return true
	}
	return false
}
