package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"agent-royo-learn/internal/domain"
)

// SaveRecurrenceRecord inserts a recurrence record.
func SaveRecurrenceRecord(ctx context.Context, tx *sql.Tx, rec *domain.RecurrenceRecord) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO recurrence_records (id, recurrence_fingerprint, learning_id, project_id, summary, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`,
		string(rec.ID),
		rec.RecurrenceFingerprint,
		string(rec.LearningID),
		string(rec.ProjectID),
		rec.Summary,
		rec.OccurredAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("SaveRecurrenceRecord: %w", err)
	}
	return nil
}

// ListRecurrenceRecords returns recurrence records for a given fingerprint,
// ordered by occurred_at DESC.
func ListRecurrenceRecords(ctx context.Context, tx *sql.Tx, projectID domain.ProjectID, fingerprint string, limit int) ([]*domain.RecurrenceRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, recurrence_fingerprint, learning_id, project_id, summary, occurred_at
		FROM recurrence_records
		WHERE project_id = ? AND recurrence_fingerprint = ?
		ORDER BY occurred_at DESC
		LIMIT ?
	`, string(projectID), fingerprint, limit)
	if err != nil {
		return nil, fmt.Errorf("ListRecurrenceRecords: %w", err)
	}
	defer rows.Close()

	var out []*domain.RecurrenceRecord
	for rows.Next() {
		rec := &domain.RecurrenceRecord{}
		var occurredAt string
		if err := rows.Scan(
			(*string)(&rec.ID),
			&rec.RecurrenceFingerprint,
			(*string)(&rec.LearningID),
			(*string)(&rec.ProjectID),
			&rec.Summary,
			&occurredAt,
		); err != nil {
			return nil, fmt.Errorf("ListRecurrenceRecords scan: %w", err)
		}
		rec.OccurredAt, _ = time.Parse(time.RFC3339Nano, occurredAt)
		out = append(out, rec)
	}
	return out, rows.Err()
}

// CountRecurrences returns the total number of recurrence records for a given
// fingerprint in the project.
func CountRecurrences(ctx context.Context, tx *sql.Tx, projectID domain.ProjectID, fingerprint string) (int, error) {
	var count int
	err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM recurrence_records
		WHERE project_id = ? AND recurrence_fingerprint = ?
	`, string(projectID), fingerprint).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("CountRecurrences: %w", err)
	}
	return count, nil
}

// ListRecurrencesByLearning returns recurrence records for a specific learning,
// ordered by occurred_at DESC.
func ListRecurrencesByLearning(ctx context.Context, tx *sql.Tx, learningID domain.LearningID, limit int) ([]*domain.RecurrenceRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, recurrence_fingerprint, learning_id, project_id, summary, occurred_at
		FROM recurrence_records
		WHERE learning_id = ?
		ORDER BY occurred_at DESC
		LIMIT ?
	`, string(learningID), limit)
	if err != nil {
		return nil, fmt.Errorf("ListRecurrencesByLearning: %w", err)
	}
	defer rows.Close()

	var out []*domain.RecurrenceRecord
	for rows.Next() {
		rec := &domain.RecurrenceRecord{}
		var occurredAt string
		if err := rows.Scan(
			(*string)(&rec.ID),
			&rec.RecurrenceFingerprint,
			(*string)(&rec.LearningID),
			(*string)(&rec.ProjectID),
			&rec.Summary,
			&occurredAt,
		); err != nil {
			return nil, fmt.Errorf("ListRecurrencesByLearning scan: %w", err)
		}
		rec.OccurredAt, _ = time.Parse(time.RFC3339Nano, occurredAt)
		out = append(out, rec)
	}
	return out, rows.Err()
}

// ListAllRecurrences returns all recurrence records for a project, ordered by
// occurred_at DESC.
func ListAllRecurrences(ctx context.Context, tx *sql.Tx, projectID domain.ProjectID, limit int) ([]*domain.RecurrenceRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, recurrence_fingerprint, learning_id, project_id, summary, occurred_at
		FROM recurrence_records
		WHERE project_id = ?
		ORDER BY occurred_at DESC
		LIMIT ?
	`, string(projectID), limit)
	if err != nil {
		return nil, fmt.Errorf("ListAllRecurrences: %w", err)
	}
	defer rows.Close()

	var out []*domain.RecurrenceRecord
	for rows.Next() {
		rec := &domain.RecurrenceRecord{}
		var occurredAt string
		if err := rows.Scan(
			(*string)(&rec.ID),
			&rec.RecurrenceFingerprint,
			(*string)(&rec.LearningID),
			(*string)(&rec.ProjectID),
			&rec.Summary,
			&occurredAt,
		); err != nil {
			return nil, fmt.Errorf("ListAllRecurrences scan: %w", err)
		}
		rec.OccurredAt, _ = time.Parse(time.RFC3339Nano, occurredAt)
		out = append(out, rec)
	}
	return out, rows.Err()
}
