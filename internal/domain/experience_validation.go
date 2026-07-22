package domain

import (
	"fmt"
	"strings"
)

const (
	// Experience metadata is bounded independently of the aggregate payload so
	// identifiers and audit fields cannot become unbounded sink inputs.
	MaxExperienceIDBytes             = 256
	MaxExperiencePathBytes           = 4096
	MaxExperienceMetadataBytes       = 256
	MaxExperienceSourceInstanceBytes = 256
	MaxExperienceCursorBytes         = 65536
	MaxExperienceDigestBytes         = 128
	MaxExperienceSummaryBytes        = 8192
	MaxExperienceJSONBytes           = 65536
	MaxExperienceErrorCodeBytes      = 128
	MaxExperienceErrorMessageBytes   = 1024
)

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
	if err := validateExperienceField("session id", string(s.ID), MaxExperienceIDBytes); err != nil {
		return err
	}
	if s.ProjectID == "" {
		return NewValidationError(ErrInvalidArgument, "experience session project_id is required")
	}
	if err := validateExperienceField("session project_id", string(s.ProjectID), MaxExperienceIDBytes); err != nil {
		return err
	}
	if s.ExternalSessionID == "" {
		return NewValidationError(ErrInvalidArgument, "experience session external_session_id is required")
	}
	if err := validateExperienceField("session external_session_id", s.ExternalSessionID, MaxExperienceIDBytes); err != nil {
		return err
	}
	if !isValidExperienceSource(s.Source) {
		return NewValidationError(ErrInvalidArgument, "invalid experience source")
	}
	if err := validateExperienceField("session metadata hash", s.MetadataSHA256, MaxExperienceDigestBytes); err != nil {
		return err
	}
	return ValidateTranscriptLocator(s.Locator)
}

// ValidateTranscriptLocator checks a locator's kind and required fields. It does
// not resolve or read the path: root confinement is enforced by the caller
// against user-configured roots, never by the repository.
func ValidateTranscriptLocator(l TranscriptLocator) error {
	if err := validateExperienceField("locator kind", l.Kind, MaxExperienceMetadataBytes); err != nil {
		return err
	}
	if err := validateExperienceField("locator path", l.Path, MaxExperiencePathBytes); err != nil {
		return err
	}
	if err := validateExperienceField("locator session id", l.SessionID, MaxExperienceIDBytes); err != nil {
		return err
	}
	if err := validateExperienceField("locator turn id", l.TurnID, MaxExperienceIDBytes); err != nil {
		return err
	}
	if err := validateExperienceField("locator source hash", l.SourceHash, MaxExperienceDigestBytes); err != nil {
		return err
	}
	if l.Kind == "api" {
		return NewValidationError(ErrExperienceSchemaUnsupported,
			"remote locator kind \"api\" is not supported in v1")
	}
	if !localLocatorKinds[l.Kind] {
		return NewValidationError(ErrExperienceLocatorInvalid, "invalid locator kind")
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
	if err := validateExperienceField("turn id", string(t.ID), MaxExperienceIDBytes); err != nil {
		return err
	}
	if t.SessionID == "" {
		return NewValidationError(ErrInvalidArgument, "experience turn session_id is required")
	}
	if err := validateExperienceField("turn session_id", string(t.SessionID), MaxExperienceIDBytes); err != nil {
		return err
	}
	if t.ExternalTurnID == "" {
		return NewValidationError(ErrInvalidArgument, "experience turn external_turn_id is required")
	}
	if err := validateExperienceField("turn external_turn_id", t.ExternalTurnID, MaxExperienceIDBytes); err != nil {
		return err
	}
	if t.Sequence < 0 {
		return NewValidationError(ErrInvalidArgument, "experience turn sequence must be non-negative")
	}
	if !isValidTurnStatus(t.Status) {
		return NewValidationError(ErrInvalidArgument,
			fmt.Sprintf("invalid turn status: %q", t.Status))
	}
	for field, value := range map[string]string{
		"turn fingerprint":       t.Fingerprint,
		"turn user digest":       t.UserDigest,
		"turn assistant digest":  t.AssistantDigest,
		"turn tool calls digest": t.ToolCallsDigest,
		"turn source revision":   t.SourceRevision,
	} {
		if err := validateExperienceField(field, value, MaxExperienceDigestBytes); err != nil {
			return err
		}
	}
	if err := validateExperienceField("turn safe summary", t.SafeSummary, MaxExperienceSummaryBytes); err != nil {
		return err
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
	if err := validateExperienceField("event id", string(e.ID), MaxExperienceIDBytes); err != nil {
		return err
	}
	if err := validateExperienceField("event project_id", string(e.ProjectID), MaxExperienceIDBytes); err != nil {
		return err
	}
	if e.TurnID == "" {
		return NewValidationError(ErrInvalidArgument, "experience event turn_id is required (provenance)")
	}
	if err := validateExperienceField("event turn_id", string(e.TurnID), MaxExperienceIDBytes); err != nil {
		return err
	}
	if !isValidExperienceEventKind(e.Kind) {
		return NewValidationError(ErrInvalidArgument, "invalid experience event kind")
	}
	if e.Summary == "" {
		return NewValidationError(ErrInvalidArgument, "experience event summary is required")
	}
	for field, value := range map[string]string{
		"event summary":        e.Summary,
		"event observation":    e.Observation,
		"event outcome":        e.Outcome,
		"event fingerprint":    e.Fingerprint,
		"event evidence JSON":  e.EvidenceJSON,
		"detector name":        e.Detector.Name,
		"detector version":     e.Detector.Version,
		"detector model":       e.Detector.Model,
		"detector prompt hash": e.Detector.PromptHash,
	} {
		limit := MaxExperienceSummaryBytes
		if strings.HasSuffix(field, "JSON") {
			limit = MaxExperienceJSONBytes
		}
		if strings.Contains(field, "fingerprint") || strings.Contains(field, "hash") {
			limit = MaxExperienceDigestBytes
		}
		if err := validateExperienceField(field, value, limit); err != nil {
			return err
		}
	}
	if !isValidDetectorKind(e.Detector.Kind) {
		return NewValidationError(ErrInvalidArgument,
			fmt.Sprintf("invalid detector kind: %q", e.Detector.Kind))
	}
	if !isValidConfidence(e.Confidence) {
		return NewValidationError(ErrInvalidArgument,
			fmt.Sprintf("invalid confidence: %q", e.Confidence))
	}
	if e.Detector.Kind == "host_llm" && e.Confidence == ConfidenceHigh {
		return NewValidationError(ErrInvalidArgument,
			"host_llm events cannot use high confidence")
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

func isValidDetectorKind(kind string) bool {
	switch kind {
	case "deterministic", "host_llm":
		return true
	}
	return false
}

// ValidateExperienceActor bounds actor metadata used by experience audit rows.
func ValidateExperienceActor(actor Actor) error {
	for field, value := range map[string]string{
		"actor kind":       actor.Kind,
		"actor name":       actor.Name,
		"actor model":      actor.Model,
		"actor session id": actor.SessionID,
	} {
		if err := validateExperienceField(field, value, MaxExperienceMetadataBytes); err != nil {
			return err
		}
	}
	return nil
}

func validateExperienceField(field, value string, max int) error {
	if len(value) > max {
		return NewValidationError(ErrExperiencePayloadTooLarge, fmt.Sprintf("%s exceeds the permitted byte limit", field))
	}
	return nil
}

// IsValidExperienceSource reports whether s is a known experience source. Public
// interfaces validate their input against this rather than defining their own
// list.
func IsValidExperienceSource(s ExperienceSource) bool { return isValidExperienceSource(s) }
