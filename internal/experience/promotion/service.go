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
	"log/slog"

	"agent-royo-learn/internal/capture"
	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/evidence"
	"agent-royo-learn/internal/experience/patterns"
	"agent-royo-learn/internal/storage"
)

// patternsFacade is the minimal surface the promotion Service uses
// from patterns. It is unexported on purpose: production wiring keeps
// the *patterns.Service signature, while tests can inject a stub that
// simulates the race window between the lookup pre-insert and the
// CAS UPDATE inside PromoteAtomic without standing up a concurrent
// transaction.
type patternsFacade interface {
	Get(ctx context.Context, id domain.ExperiencePatternID) (*patterns.ExperiencePattern, error)
	LookupPromotionState(ctx context.Context, id domain.ExperiencePatternID) (patterns.PatternStatus, *domain.LearningID, error)
	LookupPromotionAuditID(ctx context.Context, id domain.ExperiencePatternID) (domain.AuditEventID, bool, error)
	PromoteAtomic(
		ctx context.Context,
		patternID domain.ExperiencePatternID,
		learningID domain.LearningID,
		normalizedHash string,
		actor domain.Actor,
		redactionReport evidence.RedactionReport,
		note string,
		promotionFingerprint string,
	) (domain.AuditEventID, error)
}

// Service is the production PromotionService implementation.
type Service struct {
	capture  *capture.Service
	patterns patternsFacade
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

// newServiceWithPatterns is the test-only constructor that lets a
// test inject a stub patternsFacade. Production wiring uses
// NewService; the unexported helper exists so the slice 7.3 race
// test can simulate the lookup-vs-CAS-update window without
// standing up a concurrent transaction.
func newServiceWithPatterns(c *capture.Service, p patternsFacade, db *storage.DB) (*Service, error) {
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

// Promote is the slice 7.3 idempotent implementation. It validates the
// input, runs the lookup pre-insert, and either short-circuits with
// the existing learning (was_new=false), surfaces the typed error for
// the orphan / not-eligible cases, or runs the two-phase transactional
// pipeline (Capture + PromoteAtomic) for the happy path.
//
// The two phases are idempotent: Phase 1 keys Capture on
// "promotion:" + pattern.Fingerprint so a retry collapses onto the
// existing Learning; Phase 2's CAS guard on the pattern revision
// rejects a stale caller with ErrPatternInsufficientSources. The
// lookup pre-insert catches the well-known "already promoted" case
// BEFORE Capture runs, so the second call does not even attempt to
// insert a new learning or audit row.
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

	// Lookup pre-insert (slice 7.3): the lookup returns only
	// (status, proposed_learning_id) so the Service can short-circuit
	// on the already-promoted case without fetching the rest of the
	// pattern. The lookup is the beginning of the idempotency guard.
	status, existingLearningID, err := s.patterns.LookupPromotionState(ctx, input.PatternID)
	if err != nil {
		if patterns.ErrorIs(err, patterns.ErrPatternNotFound) {
			return nil, ErrPromotionPatternNotFound
		}
		return nil, err
	}

	// Idempotent return: the pattern is already promoted and the
	// proposed_learning_id is populated. We return the existing
	// learning without inserting a new one and without stamping a
	// second audit row. The AuditID is recovered from the audit_events
	// table so the observability envelope for the idempotent return
	// still carries the original audit row (a defensive read; the
	// (AuditEventID, false, nil) tuple degrades to AuditID="" if no
	// row exists, which is the right behaviour for a freshly-created
	// orphan-promoted state).
	if status == patterns.PatternPromoted && existingLearningID != nil {
		auditID, _, _ := s.patterns.LookupPromotionAuditID(ctx, input.PatternID)
		return &PromotionResult{
			PatternID:        input.PatternID,
			LearningID:       *existingLearningID,
			WasNew:           false,
			AuditID:          auditID,
			RedactionSummary: RedactionSummary{},
		}, nil
	}

	// Orphan promoted state: status='promoted' but
	// proposed_learning_id is NULL. The app invariant says promoted
	// implies proposed_learning_id IS NOT NULL, so this is an
	// inconsistent state — the lookup cannot be trusted and the
	// Service must not attempt to fabricate a learning. Surface the
	// slice 7.2 typed error and log a warning so an operator can
	// investigate the orphan.
	if status == patterns.PatternPromoted && existingLearningID == nil {
		slog.Default().Warn("promotion: orphan promoted state with null learning_id",
			"pattern_id", string(input.PatternID))
		return nil, ErrPromotionAlreadyPromoted
	}

	// Status guard. Only PatternQualified is eligible; any other
	// status (observed, dismissed, stale) surfaces
	// ErrPromotionNotEligible before any DB write.
	switch status {
	case patterns.PatternQualified:
		// Fall through to the transactional pipeline.
	default:
		return nil, ErrPromotionNotEligible
	}

	// Fetch the full pattern: the lookup returned only status +
	// proposed_learning_id, but Capture + ToPromotionFields need
	// Fingerprint, Title, Summary, etc. The Get is a cheap read on
	// the same connection and is run only on the non-idempotent path,
	// so the cost is paid once per first-time promotion.
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
