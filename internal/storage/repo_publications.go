package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"agent-royo-learn/internal/domain"
)

// PublicationMetadataError reports persisted publication JSON that cannot be
// decoded safely. Raw is retained so recovery callers can create an actionable
// artifact without mutating or erasing the stored evidence.
type PublicationMetadataError struct {
	PublicationID domain.PublicationID
	Field         string
	Raw           string
	Err           error
}

func (e *PublicationMetadataError) Error() string {
	return fmt.Sprintf("publication %s has malformed %s: %v", e.PublicationID, e.Field, e.Err)
}

func (e *PublicationMetadataError) Unwrap() error { return e.Err }

// SavePublication inserts a publication record.
func SavePublication(ctx context.Context, tx *sql.Tx, p *domain.Publication) error {
	targetsJSON := marshalAny(p.Targets)
	verificationJSON := marshalAny(p.Verification)
	rollbackJSON := marshalAny(p.Rollback)

	var completedAt *string
	if p.CompletedAt != nil {
		s := p.CompletedAt.Format(time.RFC3339)
		completedAt = &s
	}

	var approvalID *string
	if p.ApprovalID != nil {
		s := string(*p.ApprovalID)
		approvalID = &s
	}

	_, err := tx.ExecContext(ctx, `
		INSERT INTO publications (id, learning_id, preview_hash, approval_id, targets_json, verification_json, rollback_json, status, started_at, completed_at, error_code, error_message)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		string(p.ID),
		string(p.LearningID),
		p.PreviewHash,
		approvalID,
		targetsJSON,
		verificationJSON,
		rollbackJSON,
		string(p.Status),
		p.StartedAt.Format(time.RFC3339),
		completedAt,
		p.ErrorCode,
		p.ErrorMessage,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return domain.NewConflictError(domain.ErrPublicationConflict, "publication already exists: "+string(p.ID))
		}
		return fmt.Errorf("SavePublication: %w", err)
	}
	return nil
}

// GetPublication retrieves a publication by ID.
func GetPublication(ctx context.Context, tx *sql.Tx, id domain.PublicationID) (*domain.Publication, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT id, learning_id, preview_hash, approval_id, targets_json, verification_json, rollback_json, status, started_at, completed_at, error_code, error_message
		FROM publications WHERE id = ?
	`, string(id))
	return scanPublication(row)
}

// UpdatePublication updates a publication's status and results.
func UpdatePublication(ctx context.Context, tx *sql.Tx, p *domain.Publication) error {
	targetsJSON := marshalAny(p.Targets)
	verificationJSON := marshalAny(p.Verification)
	rollbackJSON := marshalAny(p.Rollback)

	var completedAt *string
	if p.CompletedAt != nil {
		s := p.CompletedAt.Format(time.RFC3339)
		completedAt = &s
	}

	_, err := tx.ExecContext(ctx, `
		UPDATE publications SET
			status = ?, targets_json = ?, verification_json = ?, rollback_json = ?,
			completed_at = ?, error_code = ?, error_message = ?
		WHERE id = ?
	`,
		string(p.Status),
		targetsJSON,
		verificationJSON,
		rollbackJSON,
		completedAt,
		p.ErrorCode,
		p.ErrorMessage,
		string(p.ID),
	)
	if err != nil {
		return fmt.Errorf("UpdatePublication: %w", err)
	}
	return nil
}

// ListPublicationsByLearning returns all publications for a learning.
func ListPublicationsByLearning(ctx context.Context, tx *sql.Tx, learningID domain.LearningID) ([]*domain.Publication, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, learning_id, preview_hash, approval_id, targets_json, verification_json, rollback_json, status, started_at, completed_at, error_code, error_message
		FROM publications WHERE learning_id = ?
		ORDER BY started_at DESC
	`, string(learningID))
	if err != nil {
		return nil, fmt.Errorf("ListPublicationsByLearning: %w", err)
	}
	defer rows.Close()

	var out []*domain.Publication
	for rows.Next() {
		p, err := scanPublicationFromRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// scanPublication scans a row into a Publication.
func scanPublication(row interface{ Scan(...interface{}) error }) (*domain.Publication, error) {
	p := &domain.Publication{}
	var (
		startedAt, targetsJSON, verificationJSON, rollbackJSON string
		completedAt                                            *string
		approvalID                                             *string
	)

	err := row.Scan(
		(*string)(&p.ID),
		(*string)(&p.LearningID),
		&p.PreviewHash,
		&approvalID,
		&targetsJSON,
		&verificationJSON,
		&rollbackJSON,
		(*string)(&p.Status),
		&startedAt,
		&completedAt,
		&p.ErrorCode,
		&p.ErrorMessage,
	)
	if err == sql.ErrNoRows {
		return nil, domain.NewNotFoundError(domain.ErrLearningNotFound, "publication")
	}
	if err != nil {
		return nil, fmt.Errorf("scanPublication: %w", err)
	}

	p.Targets = unmarshalTargetEntries(targetsJSON)
	p.Verification = unmarshalValidationResults(verificationJSON)
	if err := decodeRollbackEntries(p, rollbackJSON); err != nil {
		return nil, err
	}
	p.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
	if completedAt != nil {
		t, _ := time.Parse(time.RFC3339, *completedAt)
		p.CompletedAt = &t
	}
	if approvalID != nil {
		id := domain.ApprovalID(*approvalID)
		p.ApprovalID = &id
	}

	return p, nil
}

// scanPublicationFromRows scans a publication from sql.Rows.
func scanPublicationFromRows(rows interface{ Scan(...interface{}) error }) (*domain.Publication, error) {
	p := &domain.Publication{}
	var (
		startedAt, targetsJSON, verificationJSON, rollbackJSON string
		completedAt                                            *string
		approvalID                                             *string
	)

	err := rows.Scan(
		(*string)(&p.ID),
		(*string)(&p.LearningID),
		&p.PreviewHash,
		&approvalID,
		&targetsJSON,
		&verificationJSON,
		&rollbackJSON,
		(*string)(&p.Status),
		&startedAt,
		&completedAt,
		&p.ErrorCode,
		&p.ErrorMessage,
	)
	if err != nil {
		return nil, fmt.Errorf("scanPublicationFromRows: %w", err)
	}

	p.Targets = unmarshalTargetEntries(targetsJSON)
	p.Verification = unmarshalValidationResults(verificationJSON)
	if err := decodeRollbackEntries(p, rollbackJSON); err != nil {
		return nil, err
	}
	p.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
	if completedAt != nil {
		t, _ := time.Parse(time.RFC3339, *completedAt)
		p.CompletedAt = &t
	}
	if approvalID != nil {
		id := domain.ApprovalID(*approvalID)
		p.ApprovalID = &id
	}

	return p, nil
}

func decodeRollbackEntries(p *domain.Publication, raw string) error {
	var entries []domain.RollbackEntry
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return &PublicationMetadataError{
			PublicationID: p.ID,
			Field:         "rollback_json",
			Raw:           raw,
			Err:           err,
		}
	}
	p.Rollback = entries
	return nil
}

// jsonNullString returns the string representation of a nullable JSON value.
func jsonNullString(v interface{}) string {
	if v == nil {
		return "null"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(b)
}
