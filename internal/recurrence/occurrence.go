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

// OccurrenceInput carries the explicit occurrence-report detail (plan 4.4).
type OccurrenceInput struct {
	// Summary overrides the recurrence summary; empty falls back to the title.
	Summary string
	// Fingerprint overrides the derived recurrence fingerprint; empty derives it.
	Fingerprint    string
	Outcome        string
	Retrieved      bool
	SkillActivated bool
	Evidence       string
	Actor          domain.Actor
	// IdempotencyKey applies the D5 technical-retry guard: the same key on a
	// retry returns the existing record without creating a second one.
	IdempotencyKey string
}

// RecordOccurrence records an explicit occurrence of a learning's pattern with
// the detail the plan 4.4 enumerates, and applies D5 idempotency. It returns the
// record and whether it was newly created (false means a technical retry matched
// an existing record by idempotency key).
func RecordOccurrence(ctx context.Context, db *storage.DB, projectID domain.ProjectID, learning *domain.Learning, in OccurrenceInput) (*domain.RecurrenceRecord, bool, error) {
	if learning == nil {
		return nil, false, fmt.Errorf("recurrence: cannot record occurrence for nil learning")
	}
	fp := in.Fingerprint
	if fp == "" {
		fp = RecurrenceFingerprint(learning)
	}
	if fp == "" {
		return nil, false, fmt.Errorf("recurrence: cannot derive a fingerprint for the learning")
	}

	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("RecordOccurrence: begin tx: %w", err)
	}
	defer tx.Rollback()

	// D5: the same idempotency key is a technical retry, not a new occurrence.
	if in.IdempotencyKey != "" {
		existing, findErr := storage.FindRecurrenceByIdempotencyKey(ctx, tx, projectID, in.IdempotencyKey)
		if findErr != nil {
			return nil, false, fmt.Errorf("RecordOccurrence: %w", findErr)
		}
		if existing != nil {
			return existing, false, nil
		}
	}

	summary := in.Summary
	if summary == "" {
		summary = learning.Title
	}

	rec := &domain.RecurrenceRecord{
		ID:                    domain.RecurrenceRecordID(uuid.Must(uuid.NewV7()).String()),
		RecurrenceFingerprint: fp,
		LearningID:            learning.ID,
		ProjectID:             projectID,
		Summary:               summary,
		OccurredAt:            time.Now().UTC(),
		Outcome:               in.Outcome,
		Retrieved:             in.Retrieved,
		SkillActivated:        in.SkillActivated,
		Evidence:              in.Evidence,
		ActorKind:             in.Actor.Kind,
		ActorName:             in.Actor.Name,
	}
	if in.IdempotencyKey != "" {
		key := in.IdempotencyKey
		rec.IdempotencyKey = &key
	}

	if err := storage.SaveRecurrenceRecord(ctx, tx, rec); err != nil {
		return nil, false, fmt.Errorf("RecordOccurrence: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("RecordOccurrence: commit: %w", err)
	}
	return rec, true, nil
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
