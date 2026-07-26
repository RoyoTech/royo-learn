// Service is the production PromotionService. It wires the
// capture.Service (Hito 1) to the patterns.Service (Hito 6) and
// owns the atomic pipeline that turns a qualified ExperiencePattern
// into a persistent Learning plus the audit row.
//
// Slice 7.0 ships the constructor and the typed-error surface only.
// Slice 7.1 adds the redaction pipeline (Promote calls
// evidence.RedactPromotionFields + evidence.PromotionFingerprint
// before Capture).
// Slice 7.2 (this file) replaces the slice 7.0 stub with the
// two-phase idempotent pipeline:
//
//	Phase 1 — capture.Service.Capture produces the Learning and
//	          stamps the capture audit row, idempotent on
//	          "promotion:" + pattern.Fingerprint.
//	Phase 2 — patterns.Service.PromoteAtomic performs the CAS UPDATE
//	          on experience_patterns (status='promoted',
//	          proposed_learning_id) and inserts the
//	          experience_pattern_promoted audit row, all in one
//	          SQLite transaction.
//
// The two phases are NOT a single SQL transaction by design. A
// failure between phases leaves the Learning persisted without the
// pattern being marked promoted; that state is observable via the
// capture audit row and recoverable via the idempotent retry (the
// second call hits the dedup branch in Capture and writes the
// promotion audit row, completing the bridge). The trade-off keeps
// Hito 1's capture pipeline untouched and lets the promotion
// redaction pipeline stay pure and testable. The details live in
// docs/23-PATTERN-MINING.md §8 RF-E08.

package promotion

import (
	"context"
	"fmt"

	"agent-royo-learn/internal/capture"
	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/evidence"
	"agent-royo-learn/internal/experience/patterns"
	"agent-royo-learn/internal/storage"
)

// Service is the production PromotionService implementation.
type Service struct {
	capture  *capture.Service
	patterns *patterns.Service
	db       *storage.DB
}

// NewService wires a PromotionService that uses the supplied capture
// and patterns services plus the shared SQLite handle. All three
// arguments are required; nil values fail fast at construction with a
// typed validation error so a misconfigured CLI cannot silently drop
// promotions on the floor.
func NewService(c *capture.Service, p *patterns.Service, db *storage.DB) (*Service, error) {
	if c == nil {
		return nil, ErrPromotionInvalidArgument
	}
	if p == nil {
		return nil, ErrPromotionInvalidArgument
	}
	if db == nil {
		return nil, ErrPromotionInvalidArgument
	}
	return &Service{capture: c, patterns: p, db: db}, nil
}

// Promote is the slice 7.2 transactional implementation. It validates
// the input, looks up the pattern, runs the redaction pipeline, hands
// the redacted bag to capture.Service, and finally stamps the
// promotion audit row through patterns.Service.PromoteAtomic.
//
// The two phases are idempotent: Phase 1 keys Capture on
// "promotion:" + pattern.Fingerprint so a retry collapses onto the
// existing Learning; Phase 2's CAS guard on the pattern revision
// rejects a stale caller with ErrPatternInsufficientSources and the
// short-circuit on status='promoted' returns
// ErrPromotionAlreadyPromoted before any work happens.
func (s *Service) Promote(ctx context.Context, projectID domain.ProjectID, input *PromotionInput) (*PromotionResult, error) {
	if s == nil {
		return nil, ErrPromotionInvalidArgument
	}
	if err := input.Validate(); err != nil {
		// Multi-wrap so callers can compare with either sentinel via
		// errors.Is. ErrPromotionInvalidArgument is the canonical
		// promotion-side code; the inner error keeps the underlying
		// ErrInvalidArgument / ErrExperiencePayloadTooLarge visible
		// for diagnostics.
		return nil, fmt.Errorf("%w: %w", ErrPromotionInvalidArgument, err)
	}
	if projectID == "" {
		return nil, ErrPromotionInvalidArgument
	}

	// Look up the pattern. Slice 6.4's patterns.Service.Get returns
	// patterns.ErrPatternNotFound when the id is unknown; we rewrap
	// the typed error so the CLI/MCP layer keeps a single source of
	// truth for the canonical code (docs/17-ERROR-CODES.md).
	current, err := s.patterns.Get(ctx, input.PatternID)
	if err != nil {
		if patterns.ErrorIs(err, patterns.ErrPatternNotFound) {
			return nil, ErrPromotionPatternNotFound
		}
		return nil, err
	}
	if current == nil {
		return nil, ErrPromotionPatternNotFound
	}

	// Status guard. Only PatternQualified is eligible; a second
	// Promote on the same pattern sees PatternPromoted and returns
	// the typed terminal-state error so the CLI can render a stable
	// envelope. Any other status (observed, dismissed, stale)
	// surfaces ErrPromotionNotEligible before any DB write.
	switch current.Status {
	case patterns.PatternPromoted:
		return nil, ErrPromotionAlreadyPromoted
	case patterns.PatternQualified:
		// Fall through to the transactional pipeline.
	default:
		return nil, ErrPromotionNotEligible
	}

	// Build the deterministic PromotionFields bag from the pattern,
	// redact every free-text field in place, and compute the
	// "what Promotion saw" fingerprint. The fingerprint is stable
	// across calls with the same pattern, so the audit row stays
	// reproducible byte-for-byte. Redaction runs BEFORE Capture so
	// no secret the pattern happened to inherit can reach the
	// persisted Learning.
	fields := current.ToPromotionFields()
	redactionReport := evidence.RedactPromotionFields(&fields)
	promotionFingerprint := evidence.PromotionFingerprint(fields)

	// Phase 1: Capture. The idempotency key ties the persisted
	// Learning to the pattern fingerprint so a retry collapses onto
	// the existing row (capture.Service.Capture's dedup branch). The
	// normalized hash Capture returns is the payload_sha256 the
	// promotion audit row carries; reusing it (rather than
	// recomputing) keeps the audit evidence aligned with the
	// persisted Learning.
	captureInput := &capture.CaptureInput{
		Title:          fields.Title,
		Context:        fields.Context,
		Observation:    fields.Observation,
		Lesson:         lessonForCapture(fields),
		Type:           domain.TypeProcedure,
		Scope:          domain.ScopeProject,
		Destination:    domain.DestProject,
		Confidence:     domain.ConfidenceMedium,
		EvidenceLevel:  domain.EvidenceInsufficient,
		Recommended:    fields.Recommended,
		Limits:         fields.Limits,
		RetrievalTerms: fields.RetrievalTerms,
		Actor:          input.Actor,
		IdempotencyKey: "promotion:" + current.Fingerprint,
	}
	captureResult, err := s.capture.Capture(ctx, projectID, captureInput)
	if err != nil {
		return nil, fmt.Errorf("promotion: capture: %w", err)
	}
	if captureResult == nil {
		return nil, fmt.Errorf("promotion: capture returned nil result without error")
	}

	// Phase 2: CAS UPDATE on experience_patterns plus the
	// experience_pattern_promoted audit row, in a single SQLite
	// transaction. PromoteAtomic is the only surface that mutates the
	// pattern's status to PatternPromoted, so the invariant
	// "promoted <=> status='promoted' AND proposed_learning_id IS NOT
	// NULL" is enforced in one place.
	auditID, err := s.patterns.PromoteAtomic(
		ctx,
		current.ID,
		captureResult.LearningID,
		captureResult.NormalizedHash,
		input.Actor,
		redactionReport,
		input.Note,
		promotionFingerprint,
	)
	if err != nil {
		// PromoteAtomic's typed errors surface as-is: callers can
		// compare with patterns.ErrPatternInsufficientSources when
		// the CAS races with another transition. No extra wrapping
		// here keeps the error chain honest.
		return nil, err
	}

	return &PromotionResult{
		PatternID:  current.ID,
		LearningID: captureResult.LearningID,
		WasNew:     captureResult.New,
		AuditID:    auditID,
		RedactionSummary: RedactionSummary{
			AnyRedacted:    redactionReport.AnyRedacted,
			RedactedFields: redactionReport.RedactedFields,
		},
	}, nil
}

// lessonForCapture picks the value the capture pipeline will store as
// the Learning's ReusableLesson. The promotion mapping leaves
// PromotionFields.ReusableLesson empty by design (the redaction
// report's redacted_fields stays meaningful); capture.Service
// requires a non-empty Lesson, so we fall back to the redacted Title
// when no explicit lesson was derived. The fallback is applied AFTER
// redaction so it inherits the same scrubbing the title received.
func lessonForCapture(fields evidence.PromotionFields) string {
	if fields.ReusableLesson != "" {
		return fields.ReusableLesson
	}
	return fields.Title
}
