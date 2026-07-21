package domain

import "fmt"

// Validation for the experience-discovery domain (Hito 1). Mirrors the style of
// Validate/ValidateEvidence: required fields, then enum checks, with typed
// ValidationErrors. See docs/21-EXPERIENCE-DOMAIN.md.

// v1 accepts only local locator kinds. Remote schemes are out of scope for the
// first version (docs/21 §TranscriptLocator).
var localLocatorKinds = map[string]bool{
	"sqlite": true, "jsonl": true, "rollout": true, "file": true,
}

// ValidateExperienceSession checks a session for required fields and enums.
func ValidateExperienceSession(s *ExperienceSession) error {
	if s == nil {
		return NewValidationError(ErrInvalidArgument, "experience session is nil")
	}
	if s.ID == "" {
		return NewValidationError(ErrInvalidArgument, "experience session id is required")
	}
	if s.ProjectID == "" {
		return NewValidationError(ErrInvalidArgument, "experience session project_id is required")
	}
	if s.ExternalSessionID == "" {
		return NewValidationError(ErrInvalidArgument, "experience session external_session_id is required")
	}
	if !isValidExperienceSource(s.Source) {
		return NewValidationError(ErrInvalidArgument,
			fmt.Sprintf("invalid experience source: %q", s.Source))
	}
	return ValidateTranscriptLocator(s.Locator)
}

// ValidateTranscriptLocator checks a locator's kind and required fields. It does
// not resolve or read the path: root confinement is enforced by the caller
// against user-configured roots, never by the repository.
func ValidateTranscriptLocator(l TranscriptLocator) error {
	if l.Kind == "api" {
		return NewValidationError(ErrExperienceSchemaUnsupported,
			"remote locator kind \"api\" is not supported in v1")
	}
	if !localLocatorKinds[l.Kind] {
		return NewValidationError(ErrExperienceLocatorInvalid,
			fmt.Sprintf("invalid locator kind: %q", l.Kind))
	}
	if l.Path == "" {
		return NewValidationError(ErrExperienceLocatorInvalid, "locator path is required")
	}
	return nil
}

// ValidateExperienceTurn checks a turn for required fields and enums.
func ValidateExperienceTurn(t *ExperienceTurn) error {
	if t == nil {
		return NewValidationError(ErrInvalidArgument, "experience turn is nil")
	}
	if t.ID == "" {
		return NewValidationError(ErrInvalidArgument, "experience turn id is required")
	}
	if t.SessionID == "" {
		return NewValidationError(ErrInvalidArgument, "experience turn session_id is required")
	}
	if t.ExternalTurnID == "" {
		return NewValidationError(ErrInvalidArgument, "experience turn external_turn_id is required")
	}
	if t.Sequence < 0 {
		return NewValidationError(ErrInvalidArgument, "experience turn sequence must be non-negative")
	}
	if !isValidTurnStatus(t.Status) {
		return NewValidationError(ErrInvalidArgument,
			fmt.Sprintf("invalid turn status: %q", t.Status))
	}
	return nil
}

// ValidateExperienceEvent checks an event for required fields and enums. An
// event without a TurnID is rejected: provenance is mandatory.
func ValidateExperienceEvent(e *ExperienceEvent) error {
	if e == nil {
		return NewValidationError(ErrInvalidArgument, "experience event is nil")
	}
	if e.ID == "" {
		return NewValidationError(ErrInvalidArgument, "experience event id is required")
	}
	if e.ProjectID == "" {
		return NewValidationError(ErrInvalidArgument, "experience event project_id is required")
	}
	if e.TurnID == "" {
		return NewValidationError(ErrInvalidArgument, "experience event turn_id is required (provenance)")
	}
	if !isValidExperienceEventKind(e.Kind) {
		return NewValidationError(ErrInvalidArgument,
			fmt.Sprintf("invalid experience event kind: %q", e.Kind))
	}
	if e.Summary == "" {
		return NewValidationError(ErrInvalidArgument, "experience event summary is required")
	}
	if !isValidConfidence(e.Confidence) {
		return NewValidationError(ErrInvalidArgument,
			fmt.Sprintf("invalid confidence: %q", e.Confidence))
	}
	return nil
}

// --- private validators ---

func isValidExperienceSource(s ExperienceSource) bool {
	switch s {
	case SourceOpenCode, SourceClaudeCode, SourceCodex, SourcePi, SourceManual:
		return true
	}
	return false
}

func isValidTurnStatus(s TurnStatus) bool {
	switch s {
	case TurnObserved, TurnIncomplete, TurnStable, TurnIngested, TurnSuperseded, TurnFailed:
		return true
	}
	return false
}

func isValidExperienceEventKind(k ExperienceEventKind) bool {
	switch k {
	case EventUserCorrection, EventCommandFailure, EventTestFailure, EventTestSuccess,
		EventSuccessfulProcedure, EventRetryCorrected, EventToolLimitation,
		EventArchitectureDecision, EventPreference, EventUnknown:
		return true
	}
	return false
}

// IsValidExperienceSource reports whether s is a known experience source. Public
// interfaces validate their input against this rather than defining their own
// list.
func IsValidExperienceSource(s ExperienceSource) bool { return isValidExperienceSource(s) }
