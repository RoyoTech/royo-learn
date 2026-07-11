package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"agent-royo-learn/internal/domain"
)

// SavePreview inserts a publication preview.
func SavePreview(ctx context.Context, tx *sql.Tx, p *domain.PublicationPreview) error {
	planJSON := marshalAny(p.Plan)
	requiresApproval := boolToInt(p.RequiresApproval)

	var invalidatedAt *string
	if p.InvalidatedAt != nil {
		s := p.InvalidatedAt.Format(time.RFC3339)
		invalidatedAt = &s
	}

	_, err := tx.ExecContext(ctx, `
		INSERT INTO publication_previews (id, learning_id, plan_json, preview_hash, risk, requires_approval, created_at, invalidated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`,
		string(p.ID),
		string(p.LearningID),
		planJSON,
		p.PreviewHash,
		string(p.Risk),
		requiresApproval,
		p.CreatedAt.Format(time.RFC3339),
		invalidatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return domain.NewConflictError(domain.ErrPublicationConflict, "preview already exists: "+string(p.ID))
		}
		return fmt.Errorf("SavePreview: %w", err)
	}
	return nil
}

// GetPreview retrieves a preview by ID.
func GetPreview(ctx context.Context, tx *sql.Tx, id domain.PreviewID) (*domain.PublicationPreview, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT id, learning_id, plan_json, preview_hash, risk, requires_approval, created_at, invalidated_at
		FROM publication_previews WHERE id = ?
	`, string(id))
	return scanPreview(row)
}

// GetPreviewByHash retrieves a preview by its hash.
func GetPreviewByHash(ctx context.Context, tx *sql.Tx, hash string) (*domain.PublicationPreview, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT id, learning_id, plan_json, preview_hash, risk, requires_approval, created_at, invalidated_at
		FROM publication_previews WHERE preview_hash = ?
	`, hash)
	return scanPreview(row)
}

// InvalidatePreview marks a preview as invalidated.
func InvalidatePreview(ctx context.Context, tx *sql.Tx, id domain.PreviewID) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := tx.ExecContext(ctx, `
		UPDATE publication_previews SET invalidated_at = ? WHERE id = ?
	`, now, string(id))
	if err != nil {
		return fmt.Errorf("InvalidatePreview: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return domain.NewNotFoundError(domain.ErrPreviewNotFound, "preview: "+string(id))
	}
	return nil
}

// scanPreview scans a row into a PublicationPreview.
func scanPreview(row interface{ Scan(...interface{}) error }) (*domain.PublicationPreview, error) {
	p := &domain.PublicationPreview{}
	var (
		planJSON, createdAt string
		invalidatedAt       *string
		requiresApprovalInt int
	)

	err := row.Scan(
		(*string)(&p.ID),
		(*string)(&p.LearningID),
		&planJSON,
		&p.PreviewHash,
		(*string)(&p.Risk),
		&requiresApprovalInt,
		&createdAt,
		&invalidatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, domain.NewNotFoundError(domain.ErrPreviewNotFound, "preview")
	}
	if err != nil {
		return nil, fmt.Errorf("scanPreview: %w", err)
	}

	p.Plan = unmarshalPublicationPlan(planJSON)
	p.RequiresApproval = requiresApprovalInt != 0
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	if invalidatedAt != nil {
		t, _ := time.Parse(time.RFC3339, *invalidatedAt)
		p.InvalidatedAt = &t
	}

	return p, nil
}

// unmarshalPublicationPlan parses a PublicationPlan from JSON.
func unmarshalPublicationPlan(raw string) domain.PublicationPlan {
	if raw == "" || raw == "{}" {
		return domain.PublicationPlan{}
	}
	var plan domain.PublicationPlan
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		return domain.PublicationPlan{}
	}
	return plan
}
