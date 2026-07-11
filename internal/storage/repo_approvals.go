package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"agent-royo-learn/internal/domain"
)

// SaveApproval inserts an approval record.
func SaveApproval(ctx context.Context, tx *sql.Tx, a *domain.Approval) error {
	var expiresAt *string
	if a.ExpiresAt != nil {
		s := a.ExpiresAt.Format(time.RFC3339)
		expiresAt = &s
	}
	var revokedAt *string
	if a.RevokedAt != nil {
		s := a.RevokedAt.Format(time.RFC3339)
		revokedAt = &s
	}

	_, err := tx.ExecContext(ctx, `
		INSERT INTO approvals (id, learning_id, preview_hash, approved_by, reason, approval_evidence, created_at, expires_at, revoked_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		string(a.ID),
		string(a.LearningID),
		a.PreviewHash,
		a.ApprovedBy,
		a.Reason,
		a.ApprovalEvidence,
		a.CreatedAt.Format(time.RFC3339),
		expiresAt,
		revokedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return domain.NewConflictError(domain.ErrPublicationConflict, "approval already exists: "+string(a.ID))
		}
		return fmt.Errorf("SaveApproval: %w", err)
	}
	return nil
}

// GetApproval retrieves an approval by ID.
func GetApproval(ctx context.Context, tx *sql.Tx, id domain.ApprovalID) (*domain.Approval, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT id, learning_id, preview_hash, approved_by, reason, approval_evidence, created_at, expires_at, revoked_at
		FROM approvals WHERE id = ?
	`, string(id))
	return scanApproval(row)
}

// GetApprovalByHash retrieves the latest non-revoked approval for a preview hash.
func GetApprovalByHash(ctx context.Context, tx *sql.Tx, hash string) (*domain.Approval, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT id, learning_id, preview_hash, approved_by, reason, approval_evidence, created_at, expires_at, revoked_at
		FROM approvals WHERE preview_hash = ? AND revoked_at IS NULL
		ORDER BY created_at DESC LIMIT 1
	`, hash)
	return scanApproval(row)
}

// RevokeApproval marks an approval as revoked.
func RevokeApproval(ctx context.Context, tx *sql.Tx, id domain.ApprovalID) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := tx.ExecContext(ctx, `
		UPDATE approvals SET revoked_at = ? WHERE id = ?
	`, now, string(id))
	if err != nil {
		return fmt.Errorf("RevokeApproval: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return domain.NewNotFoundError(domain.ErrApprovalInvalid, "approval: "+string(id))
	}
	return nil
}

// scanApproval scans a row into an Approval.
func scanApproval(row interface{ Scan(...interface{}) error }) (*domain.Approval, error) {
	a := &domain.Approval{}
	var createdAt string
	var expiresAt, revokedAt *string

	err := row.Scan(
		(*string)(&a.ID),
		(*string)(&a.LearningID),
		&a.PreviewHash,
		&a.ApprovedBy,
		&a.Reason,
		&a.ApprovalEvidence,
		&createdAt,
		&expiresAt,
		&revokedAt,
	)
	if err == sql.ErrNoRows {
		return nil, domain.NewNotFoundError(domain.ErrApprovalInvalid, "approval")
	}
	if err != nil {
		return nil, fmt.Errorf("scanApproval: %w", err)
	}

	a.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	if expiresAt != nil {
		t, _ := time.Parse(time.RFC3339, *expiresAt)
		a.ExpiresAt = &t
	}
	if revokedAt != nil {
		t, _ := time.Parse(time.RFC3339, *revokedAt)
		a.RevokedAt = &t
	}

	return a, nil
}

// unmarshalTargetEntries parses a JSON array of TargetEntry.
func unmarshalTargetEntries(raw string) []domain.TargetEntry {
	if raw == "" || raw == "[]" {
		return nil
	}
	var out []domain.TargetEntry
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

// unmarshalRollbackEntries parses a JSON array of RollbackEntry.
func unmarshalRollbackEntries(raw string) []domain.RollbackEntry {
	if raw == "" || raw == "[]" {
		return nil
	}
	var out []domain.RollbackEntry
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

// intToBoolPtr converts 0/1 from SQLite to *bool.
func intToBoolPtr(v *int) *bool {
	if v == nil {
		return nil
	}
	b := *v != 0
	return &b
}
