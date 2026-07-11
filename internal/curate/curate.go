package curate

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"agent-royo-learn/internal/capture"
	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"

	"github.com/google/uuid"
)

// Service provides curation and relation operations.
type Service struct {
	db         *storage.DB
	recordsDir string
}

// NewService creates a new curate Service.
func NewService(db *storage.DB, recordsDir string) *Service {
	return &Service{db: db, recordsDir: recordsDir}
}

// ---------------------------------------------------------------------------
// Curate
// ---------------------------------------------------------------------------

// CurateInput is the input for a curation action.
type CurateInput struct {
	LearningID domain.LearningID
	Decision   domain.CurationDecision
	Rationale  string
	Actor      domain.Actor
}

// CurateResult is the output of a curation action.
type CurateResult struct {
	CurationID domain.CurationID
	LearningID domain.LearningID
	NewStatus  domain.LearningStatus
}

// Curate evaluates a learning and applies a curation decision.
func (s *Service) Curate(ctx context.Context, projectID domain.ProjectID, input *CurateInput) (*CurateResult, error) {
	if input == nil {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "curate: input is nil")
	}
	if input.LearningID == "" {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "curate: learning_id is required")
	}
	if input.Decision == "" {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "curate: decision is required")
	}

	// Map decision to target status.
	targetStatus, err := decisionToStatus(input.Decision)
	if err != nil {
		return nil, err
	}

	// Load the learning inside a read transaction.
	tx, err := s.db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("curate: begin read tx: %w", err)
	}
	learning, err := storage.GetLearning(ctx, tx, input.LearningID)
	tx.Rollback()
	if err != nil {
		return nil, fmt.Errorf("curate: get learning: %w", err)
	}
	if learning == nil {
		return nil, domain.NewNotFoundError(domain.ErrLearningNotFound, "learning: "+string(input.LearningID))
	}

	// Verify project ownership.
	if learning.ProjectID != projectID {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument,
			fmt.Sprintf("curate: learning %q belongs to project %q, not %q",
				input.LearningID, learning.ProjectID, projectID))
	}

	// If approving, check evidence threshold.
	if isApprovalDecision(input.Decision) {
		if err := s.checkEvidenceThreshold(ctx, learning); err != nil {
			return nil, err
		}
	}

	// Validate transition.
	if !domain.CanTransition(learning.Status, targetStatus) {
		return nil, domain.NewValidationError(domain.ErrInvalidTransition,
			fmt.Sprintf("cannot transition from %q to %q", learning.Status, targetStatus))
	}

	now := time.Now().UTC()
	curation := &domain.Curation{
		ID:         domain.CurationID(uuid.Must(uuid.NewV7()).String()),
		LearningID: input.LearningID,
		Decision:   input.Decision,
		Rationale:  input.Rationale,
		Actor:      input.Actor,
		CreatedAt:  now,
	}

	// Apply transition and save in a write transaction.
	if err := storage.WithTx(ctx, s.db, func(wtx *sql.Tx) error {
		// Save curation record.
		if err := storage.SaveCuration(ctx, wtx, curation); err != nil {
			return fmt.Errorf("curate: save curation: %w", err)
		}

		// Apply state transition.
		if err := domain.MustTransition(learning, input.Actor, targetStatus); err != nil {
			return fmt.Errorf("curate: transition: %w", err)
		}

		// Persist updated learning.
		if err := storage.UpdateLearning(ctx, wtx, learning); err != nil {
			return fmt.Errorf("curate: update learning: %w", err)
		}

		return nil
	}); err != nil {
		return nil, err
	}

	// Record audit event.
	auditEvt := &domain.AuditEvent{
		ID:         domain.AuditEventID(uuid.Must(uuid.NewV7()).String()),
		OccurredAt: now,
		Actor:      input.Actor,
		Operation:  "curate",
		EntityType: "learning",
		EntityID:   string(input.LearningID),
		NewState:   stringPtr(string(targetStatus)),
		Result:     "success",
		Details: map[string]any{
			"decision":        string(input.Decision),
			"rationale":       input.Rationale,
			"curation_id":     string(curation.ID),
			"previous_status": string(learning.Status),
		},
	}
	if err := storage.RecordEvent(ctx, s.db.DB, auditEvt); err != nil {
		return nil, fmt.Errorf("curate: record audit: %w", err)
	}

	// Update Markdown record.
	if err := capture.WriteRecord(s.recordsDir, learning); err != nil {
		return nil, fmt.Errorf("curate: write record: %w", err)
	}

	return &CurateResult{
		CurationID: curation.ID,
		LearningID: learning.ID,
		NewStatus:  learning.Status,
	}, nil
}

// checkEvidenceThreshold verifies the learning meets minimum evidence requirements.
func (s *Service) checkEvidenceThreshold(ctx context.Context, learning *domain.Learning) error {
	// Check learning's own evidence level.
	if learning.EvidenceLevel == domain.EvidenceWeak || learning.EvidenceLevel == domain.EvidenceInsufficient {
		return domain.NewValidationError(domain.ErrEvidenceMissing,
			fmt.Sprintf("cannot approve learning %q: evidence level is %q (minimum: moderate)",
				learning.ID, learning.EvidenceLevel))
	}

	// Check that at least one evidence record exists.
	tx, err := s.db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("curate: begin evidence check tx: %w", err)
	}
	defer tx.Rollback()

	evidence, err := storage.ListEvidenceByLearning(ctx, tx, learning.ID)
	if err != nil {
		return fmt.Errorf("curate: list evidence: %w", err)
	}
	if len(evidence) == 0 {
		return domain.NewValidationError(domain.ErrEvidenceMissing,
			fmt.Sprintf("cannot approve learning %q: no evidence records attached", learning.ID))
	}

	return nil
}

// decisionToStatus maps a curation decision to the target learning status.
func decisionToStatus(d domain.CurationDecision) (domain.LearningStatus, error) {
	switch d {
	case domain.CurationReject:
		return domain.StatusRejected, nil
	case domain.CurationNeedsEvidence:
		return domain.StatusNeedsEvidence, nil
	case domain.CurationMerge:
		return domain.StatusMerged, nil
	case domain.CurationApproveProjectKnowledge,
		domain.CurationApproveSharedKnowledge,
		domain.CurationApproveNewSkill,
		domain.CurationApproveSkillUpdate,
		domain.CurationApproveAgentsRule,
		domain.CurationApproveTest:
		return domain.StatusApproved, nil
	default:
		return "", domain.NewValidationError(domain.ErrInvalidArgument,
			fmt.Sprintf("unknown curation decision: %q", d))
	}
}

// isApprovalDecision returns true if the decision is an approval type.
func isApprovalDecision(d domain.CurationDecision) bool {
	switch d {
	case domain.CurationApproveProjectKnowledge,
		domain.CurationApproveSharedKnowledge,
		domain.CurationApproveNewSkill,
		domain.CurationApproveSkillUpdate,
		domain.CurationApproveAgentsRule,
		domain.CurationApproveTest:
		return true
	}
	return false
}

func stringPtr(s string) *string {
	return &s
}
