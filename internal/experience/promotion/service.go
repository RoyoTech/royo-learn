// Service is the production PromotionService. It wires the
// capture.Service (Hito 1) to the patterns.Service (Hito 6) and
// owns the atomic pipeline that turns a qualified ExperiencePattern
// into a persistent Learning plus the audit row.
//
// Slice 7.0 ships the constructor and the typed-error surface only.
// Slices 7.1–7.4 add the redaction pipeline (7.1), the atomic
// transactional promotion (7.2), the idempotency guard (7.3), and
// the CLI/MCP integration (7.4).
//
// The Service MUST never write a Learning directly; the actual
// write goes through capture.Service so the redaction, hash, and
// audit invariants of Hito 1 stay intact. Promotion is the bridge,
// not a side effect.

package promotion

import (
	"context"

	"agent-royo-learn/internal/capture"
	"agent-royo-learn/internal/domain"
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

// Promote is the slice 7.0 stub. It validates the input, looks up
// the pattern, and surfaces the typed errors the contract tests pin.
// The happy path is intentionally out of scope here and lands in
// slice 7.2 with the redaction pipeline and the transactional
// commit.
func (s *Service) Promote(ctx context.Context, projectID domain.ProjectID, input *PromotionInput) (*PromotionResult, error) {
	if s == nil {
		return nil, ErrPromotionInvalidArgument
	}
	if err := input.Validate(); err != nil {
		return nil, err
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

	// Slice 7.0 only implements the typed-error surface for the
	// status transitions. The terminal branch (PatternPromoted) and
	// the eligible branch (PatternQualified) are wired up; everything
	// else is rejected with ErrPromotionNotEligible.
	switch current.Status {
	case patterns.PatternPromoted:
		return nil, ErrPromotionAlreadyPromoted
	case patterns.PatternQualified:
		// Happy path lands in slice 7.2. Surface a typed error so the
		// contract test table can pin the status transition without
		// having to wait for the redaction pipeline.
		return nil, ErrPromotionNotImplemented
	default:
		return nil, ErrPromotionNotEligible
	}
}
