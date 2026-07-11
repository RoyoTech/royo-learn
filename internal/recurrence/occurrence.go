package recurrence

import (
	"context"
	"fmt"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"

	"github.com/google/uuid"
)

// RecordRecurrence records a recurrence event for a learning. It computes
// the recurrence fingerprint and creates a RecurrenceRecord. If a record
// with the same fingerprint and learning already exists within the same
// transaction, it is idempotent for that period.
func RecordRecurrence(ctx context.Context, db *storage.DB, projectID domain.ProjectID, learning *domain.Learning) (*domain.RecurrenceRecord, error) {
	fp := RecurrenceFingerprint(learning)
	if fp == "" {
		return nil, fmt.Errorf("recurrence: cannot record recurrence for nil learning")
	}

	rec := &domain.RecurrenceRecord{
		ID:                    domain.RecurrenceRecordID(uuid.Must(uuid.NewV7()).String()),
		RecurrenceFingerprint: fp,
		LearningID:            learning.ID,
		ProjectID:             projectID,
		Summary:               learning.Title,
		OccurredAt:            time.Now().UTC(),
	}

	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("RecordRecurrence: begin tx: %w", err)
	}
	defer tx.Rollback()

	if err := storage.SaveRecurrenceRecord(ctx, tx, rec); err != nil {
		return nil, fmt.Errorf("RecordRecurrence: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("RecordRecurrence: commit: %w", err)
	}

	return rec, nil
}

// ListRecurrencesForLearning returns recurrence records for a specific
// learning, ordered by occurred_at DESC.
func ListRecurrencesForLearning(ctx context.Context, db *storage.DB, learningID domain.LearningID, limit int) ([]*domain.RecurrenceRecord, error) {
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("ListRecurrencesForLearning: begin tx: %w", err)
	}
	defer tx.Rollback()

	records, err := storage.ListRecurrencesByLearning(ctx, tx, learningID, limit)
	if err != nil {
		return nil, fmt.Errorf("ListRecurrencesForLearning: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("ListRecurrencesForLearning: commit: %w", err)
	}

	return records, nil
}

// ListAllRecurrences returns all recurrence records for a project.
func ListAllRecurrences(ctx context.Context, db *storage.DB, projectID domain.ProjectID, limit int) ([]*domain.RecurrenceRecord, error) {
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("ListAllRecurrences: begin tx: %w", err)
	}
	defer tx.Rollback()

	records, err := storage.ListAllRecurrences(ctx, tx, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("ListAllRecurrences: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("ListAllRecurrences: commit: %w", err)
	}

	return records, nil
}
