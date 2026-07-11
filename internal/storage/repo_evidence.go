package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"agent-royo-learn/internal/domain"
)

// SaveEvidence inserts a new evidence record.
func SaveEvidence(ctx context.Context, tx *sql.Tx, e *domain.Evidence) error {
	commandJSON := marshalAny(e.Command)
	_, err := tx.ExecContext(ctx, `
		INSERT INTO evidence (id, learning_id, kind, uri, summary, sha256, command_json, exit_code, redacted, size_bytes, collected_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		string(e.ID),
		string(e.LearningID),
		string(e.Kind),
		e.URI,
		e.Summary,
		e.SHA256,
		commandJSON,
		e.ExitCode,
		boolToInt(e.Redacted),
		e.SizeBytes,
		e.CollectedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("SaveEvidence: %w", err)
	}
	return nil
}

// GetEvidence retrieves an evidence record by ID.
func GetEvidence(ctx context.Context, tx *sql.Tx, id domain.EvidenceID) (*domain.Evidence, error) {
	e := &domain.Evidence{}
	var collectedAt, cmdJSON string
	err := tx.QueryRowContext(ctx, `
		SELECT id, learning_id, kind, uri, summary, sha256, command_json, exit_code, redacted, size_bytes, collected_at
		FROM evidence WHERE id = ?
	`, string(id)).Scan(
		(*string)(&e.ID),
		(*string)(&e.LearningID),
		(*string)(&e.Kind),
		&e.URI,
		&e.Summary,
		&e.SHA256,
		&cmdJSON,
		&e.ExitCode,
		&e.Redacted,
		&e.SizeBytes,
		&collectedAt,
	)
	if err == sql.ErrNoRows {
		return nil, domain.NewNotFoundError(domain.ErrLearningNotFound, "evidence")
	}
	if err != nil {
		return nil, fmt.Errorf("GetEvidence: %w", err)
	}
	e.Command = unmarshalStringSlice(cmdJSON)
	e.CollectedAt, _ = time.Parse(time.RFC3339, collectedAt)
	return e, nil
}

// ListEvidenceByLearning returns all evidence for a learning.
func ListEvidenceByLearning(ctx context.Context, tx *sql.Tx, learningID domain.LearningID) ([]*domain.Evidence, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, learning_id, kind, uri, summary, sha256, command_json, exit_code, redacted, size_bytes, collected_at
		FROM evidence WHERE learning_id = ?
		ORDER BY collected_at DESC
	`, string(learningID))
	if err != nil {
		return nil, fmt.Errorf("ListEvidenceByLearning: %w", err)
	}
	defer rows.Close()

	var out []*domain.Evidence
	for rows.Next() {
		e := &domain.Evidence{}
		var collectedAt, cmdJSON string
		if err := rows.Scan(
			(*string)(&e.ID),
			(*string)(&e.LearningID),
			(*string)(&e.Kind),
			&e.URI,
			&e.Summary,
			&e.SHA256,
			&cmdJSON,
			&e.ExitCode,
			&e.Redacted,
			&e.SizeBytes,
			&collectedAt,
		); err != nil {
			return nil, fmt.Errorf("ListEvidenceByLearning scan: %w", err)
		}
		e.Command = unmarshalStringSlice(cmdJSON)
		e.CollectedAt, _ = time.Parse(time.RFC3339, collectedAt)
		out = append(out, e)
	}
	return out, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
