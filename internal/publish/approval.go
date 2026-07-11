package publish

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"

	"github.com/google/uuid"
)

// Approve records an approval for a publication preview.
func (s *Service) Approve(ctx context.Context, projectID domain.ProjectID, input *ApproveInput) (*domain.Approval, error) {
	if input == nil {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "approval input is nil")
	}
	if input.PreviewHash == "" {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "preview_hash is required")
	}
	if input.ApprovedBy == "" {
		return nil, domain.NewValidationError(domain.ErrInvalidArgument, "approved_by is required")
	}

	// Verify the preview exists and is not invalidated.
	readTx, err := s.db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("Approve: begin tx: %w", err)
	}
	preview, err := storage.GetPreviewByHash(ctx, readTx, input.PreviewHash)
	readTx.Rollback()
	if err != nil {
		return nil, fmt.Errorf("Approve: get preview: %w", err)
	}
	if preview.InvalidatedAt != nil {
		return nil, domain.NewValidationError(domain.ErrPreviewHashMismatch,
			"preview has been invalidated — regenerate before approving")
	}

	// If a learning ID is provided explicitly, verify it matches the preview.
	if input.LearningID != "" && input.LearningID != preview.LearningID {
		return nil, domain.NewValidationError(domain.ErrApprovalInvalid,
			"learning_id does not match the preview's learning")
	}

	now := utcNowPublish()

	// Calculate expiry if requested.
	var expiresAt *time.Time
	if input.ExpiresIn > 0 {
		t := now.Add(time.Duration(input.ExpiresIn) * time.Second)
		expiresAt = &t
	}

	approval := &domain.Approval{
		ID:               domain.ApprovalID(uuid.Must(uuid.NewV7()).String()),
		LearningID:       preview.LearningID,
		PreviewHash:      input.PreviewHash,
		ApprovedBy:       input.ApprovedBy,
		Reason:           input.Reason,
		ApprovalEvidence: input.ApprovalEvidence,
		CreatedAt:        now,
		ExpiresAt:        expiresAt,
	}

	// Persist approval.
	if err := storage.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		return storage.SaveApproval(ctx, tx, approval)
	}); err != nil {
		return nil, fmt.Errorf("Approve: save: %w", err)
	}

	return approval, nil
}

// CheckApproval verifies that a valid non-expired, non-revoked approval exists
// for the given preview hash. Returns the approval if valid, or a detailed error.
func (s *Service) CheckApproval(ctx context.Context, previewHash string) (*domain.Approval, error) {
	readTx, err := s.db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("CheckApproval: begin tx: %w", err)
	}
	defer readTx.Rollback()

	approval, err := storage.GetApprovalByHash(ctx, readTx, previewHash)
	if err != nil {
		return nil, domain.NewValidationError(domain.ErrApprovalRequired,
			"no valid approval found for this preview hash — approval is required before publishing")
	}
	if approval == nil {
		return nil, domain.NewValidationError(domain.ErrApprovalRequired,
			"no approval found for preview")
	}

	// Check expiry.
	if approval.ExpiresAt != nil && approval.ExpiresAt.Before(utcNowPublish()) {
		return nil, domain.NewValidationError(domain.ErrApprovalExpired,
			fmt.Sprintf("approval expired at %s", approval.ExpiresAt.Format(time.RFC3339)))
	}

	// Check revocation.
	if approval.RevokedAt != nil {
		return nil, domain.NewValidationError(domain.ErrApprovalInvalid,
			"approval has been revoked")
	}

	return approval, nil
}
