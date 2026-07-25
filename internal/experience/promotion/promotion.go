// Package promotion implements the promotion bridge that turns a
// qualified ExperiencePattern (Hito 6) into a persistent Learning via
// the capture.Service (Hito 1). Promotion is the only path that turns
// observed experience into reusable knowledge; it is the boundary the
// rest of the system trusts.
//
// Slice 7.0 ships the contract:
//
//   - PromotionInput, PromotionResult, RedactionSummary value types.
//   - PromotionService interface with the Promote operation.
//   - SourceKind enum (v1: pattern_mining only).
//   - Typed errors (ErrPromotionNotFound, ErrPromotionNotEligible,
//     ErrPromotionAlreadyPromoted, ErrPromotionInvalidArgument).
//   - Stubs for the Service and Repository surfaces.
//
// Slices 7.1–7.4 add the redaction pipeline, the atomic transactional
// promotion, the idempotency guard, and the CLI/MCP surface.
//
// The package never writes a Learning directly. Promotion is the bridge
// to capture.Service, not a side effect. See docs/20-EXPERIENCE-INGESTION-PRD.md
// §6 RF-E08 and docs/23-PATTERN-MINING.md §8.

package promotion

import (
	"context"
	"errors"
	"fmt"

	"agent-royo-learn/internal/domain"
)

// SourceKind identifies how a promotion was triggered. The v1 surface
// only knows pattern_mining; future triggers (curated lessons, manual
// capture) must extend this enum and the supporting Repository.
type SourceKind string

const (
	// SourcePatternMining is the source kind for promotions triggered
	// by the pattern-mining pipeline (Hito 6 → Hito 7).
	SourcePatternMining SourceKind = "pattern_mining"
)

// IsValidSourceKind reports whether the supplied kind is a closed enum
// value. The check is intentionally conservative: an unknown kind is
// rejected at construction, not at first use.
func IsValidSourceKind(k SourceKind) bool {
	switch k {
	case SourcePatternMining:
		return true
	}
	return false
}

// PromotionInput is the input for a PromotionService.Promote call.
//
// PatternID identifies the qualified pattern that will be promoted;
// Actor names the operator (admin-only in MCP). Note is an optional
// reviewer comment bounded to MaxPromotionNoteBytes so the audit trail
// cannot grow unbounded.
type PromotionInput struct {
	PatternID domain.ExperiencePatternID
	Actor     domain.Actor
	Note      string
}

// MaxPromotionNoteBytes bounds the reviewer note carried in the audit
// row. The same constant is reused by the dismissal flow (see
// patterns.MaxDismissalNoteBytes) so the audit sink is bounded the
// same way regardless of which closed-flow produced the row.
const MaxPromotionNoteBytes = 1024

// Validate enforces the documented invariants. Failing fast at the
// caller keeps the typed errors table tight.
func (in *PromotionInput) Validate() error {
	if in == nil {
		return domain.NewValidationError(domain.ErrInvalidArgument, "promotion: input is nil")
	}
	if in.PatternID == "" {
		return domain.NewValidationError(domain.ErrInvalidArgument, "promotion: pattern id is required")
	}
	if in.Actor.Kind == "" || in.Actor.Name == "" {
		return domain.NewValidationError(domain.ErrInvalidArgument, "promotion: actor is required")
	}
	if len(in.Note) > MaxPromotionNoteBytes {
		return domain.NewValidationError(domain.ErrExperiencePayloadTooLarge,
			"promotion: note exceeds the permitted byte limit")
	}
	return nil
}

// RedactionSummary is the structured response the promotion audit row
// stores. It mirrors the fields captured by evidence.AnyRedacted so
// reviewers can confirm whether sensitive text was scrubbed before
// the Learning was written.
type RedactionSummary struct {
	AnyRedacted    bool     `json:"any_redacted"`
	RedactedFields []string `json:"redacted_fields,omitempty"`
}

// PromotionResult is the output of a successful Promote. PatternID is
// echoed for caller convenience; LearningID is the new or existing
// learning; WasNew is false when the call was a no-op idempotent
// retry; AuditID identifies the append-only audit row; RedactionSummary
// records what was scrubbed before persistence.
type PromotionResult struct {
	PatternID        domain.ExperiencePatternID `json:"pattern_id"`
	LearningID       domain.LearningID          `json:"learning_id"`
	WasNew           bool                       `json:"was_new"`
	AuditID          domain.AuditEventID        `json:"audit_id"`
	RedactionSummary RedactionSummary           `json:"redaction_summary"`
}

// PromotionStatus is reserved as a hook for future revisions. v1 ships
// only the strict check that the pattern is in the PatternQualified
// state; the Service does the runtime check and returns the
// corresponding typed error. Adding a new eligible status requires
// editing the Service switch statement and the docs/23 spec.

// PromotionService is the contract higher layers (CLI, MCP) call into.
// Implementations own the transactional pipeline that turns a
// qualified pattern into a learning, audit row, and pattern status
// transition.
type PromotionService interface {
	Promote(ctx context.Context, projectID domain.ProjectID, input *PromotionInput) (*PromotionResult, error)
}

// --- Typed errors -------------------------------------------------
//
// Per docs/23 §8 the package surfaces these typed errors. They are
// exposed as package-level variables so callers can compare with
// errors.Is; they also carry the canonical domain code so the CLI
// and MCP layers can render stable error envelopes without
// re-classifying them.

var (
	// ErrPromotionPatternNotFound is returned when the supplied
	// pattern id does not match any persisted pattern.
	ErrPromotionPatternNotFound = domain.NewNotFoundError(domain.ErrPatternNotFound, "promotion: experience pattern")

	// ErrPromotionNotEligible is returned when the pattern is in a
	// status that does not permit promotion (anything other than
	// PatternQualified).
	ErrPromotionNotEligible = domain.NewValidationError(domain.ErrPatternNotQualified,
		"promotion: pattern is not qualified")

	// ErrPromotionAlreadyPromoted is returned when the pattern is
	// already in the promoted state. Promotion is terminal; a second
	// call returns the existing LearningID via WasNew=false.
	ErrPromotionAlreadyPromoted = domain.NewConflictError(domain.ErrPatternAlreadyPromoted,
		"promotion: pattern is already promoted")

	// ErrPromotionInvalidArgument is a convenience alias for the
	// ErrInvalidArgument validation errors the Promote constructor
	// rejects. Surfaced as a typed error so callers can compare with
	// errors.Is without depending on the canonical domain code.
	ErrPromotionInvalidArgument = domain.NewValidationError(domain.ErrInvalidArgument,
		"promotion: invalid argument")

	// ErrPromotionNotImplemented is returned by the slice 7.0 stub
	// when the pattern is eligible but the atomic pipeline has not
	// landed yet. It is removed by slice 7.2; the contract tests pin
	// it so callers learn the typed error surface even before the
	// happy path is wired up.
	ErrPromotionNotImplemented = domain.NewValidationError(domain.ErrPromotionNotImplemented,
		"promotion: happy path not implemented in slice 7.0")
)

// errorIs exposes the package-level typed errors so callers and
// tests can compare with errors.Is without depending on the
// variable identity. Kept as a function (not a constant) so adding
// new errors in slices 7.1–7.4 only requires extending this single
// source of truth.
func errorIs(err, target error) bool {
	return errors.Is(err, target)
}

// ErrorIs is the public lookup, mirroring the patterns.ErrorIs
// pattern. It exists so callers and tests can compare with errors.Is
// without taking a hard dependency on the package-level variables.
func ErrorIs(err, target error) bool {
	return errorIs(err, target)
}

// formatPromotionContext is the small helper used by the audit row to
// describe the source. It is intentionally simple so the audit row
// stays grep-friendly.
func formatPromotionContext(patternID domain.ExperiencePatternID, source SourceKind, note string) string {
	if note == "" {
		return fmt.Sprintf("pattern=%s source=%s", patternID, source)
	}
	return fmt.Sprintf("pattern=%s source=%s note=%s", patternID, source, note)
}
