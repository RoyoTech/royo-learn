package storage

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
)

func TestRecurrenceRepositoryRoundTripAndQueries(t *testing.T) {
	db, project := setupTestDB(t)
	ctx := context.Background()
	learning := newTestLearning(project.ID)
	key := "retry-key"
	newer := &domain.RecurrenceRecord{
		ID:                    domain.RecurrenceRecordID(newUUID()),
		RecurrenceFingerprint: "same-pattern",
		LearningID:            learning.ID,
		ProjectID:             project.ID,
		Summary:               "newer occurrence",
		OccurredAt:            utcNow(),
		Outcome:               "fixed",
		Retrieved:             true,
		SkillActivated:        true,
		Evidence:              "test evidence",
		ActorKind:             "agent",
		ActorName:             "storage-test",
		IdempotencyKey:        &key,
	}
	older := &domain.RecurrenceRecord{
		ID:                    domain.RecurrenceRecordID(newUUID()),
		RecurrenceFingerprint: newer.RecurrenceFingerprint,
		LearningID:            learning.ID,
		ProjectID:             project.ID,
		Summary:               "older occurrence",
		OccurredAt:            newer.OccurredAt.Add(-time.Hour),
	}

	if err := WithTx(ctx, db, func(tx *sql.Tx) error {
		if err := SaveLearning(ctx, tx, learning); err != nil {
			return err
		}
		if err := SaveRecurrenceRecord(ctx, tx, older); err != nil {
			return err
		}
		return SaveRecurrenceRecord(ctx, tx, newer)
	}); err != nil {
		t.Fatalf("seed recurrence records: %v", err)
	}

	if err := WithTx(ctx, db, func(tx *sql.Tx) error {
		missing, err := FindRecurrenceByIdempotencyKey(ctx, tx, project.ID, "")
		if err != nil || missing != nil {
			t.Fatalf("empty idempotency key = %#v, %v; want nil, nil", missing, err)
		}
		missing, err = FindRecurrenceByIdempotencyKey(ctx, tx, project.ID, "missing")
		if err != nil || missing != nil {
			t.Fatalf("missing idempotency key = %#v, %v; want nil, nil", missing, err)
		}
		found, err := FindRecurrenceByIdempotencyKey(ctx, tx, project.ID, key)
		if err != nil {
			t.Fatalf("FindRecurrenceByIdempotencyKey: %v", err)
		}
		if found.ID != newer.ID || found.Outcome != newer.Outcome || !found.Retrieved || !found.SkillActivated || found.IdempotencyKey == nil || *found.IdempotencyKey != key {
			t.Fatalf("found recurrence = %#v, want full newer record", found)
		}

		byFingerprint, err := ListRecurrenceRecords(ctx, tx, project.ID, newer.RecurrenceFingerprint, 0)
		if err != nil || len(byFingerprint) != 2 || byFingerprint[0].ID != newer.ID {
			t.Fatalf("ListRecurrenceRecords = %#v, %v", byFingerprint, err)
		}
		limited, err := ListRecurrenceRecords(ctx, tx, project.ID, newer.RecurrenceFingerprint, 1)
		if err != nil || len(limited) != 1 || limited[0].ID != newer.ID {
			t.Fatalf("limited ListRecurrenceRecords = %#v, %v", limited, err)
		}
		count, err := CountRecurrences(ctx, tx, project.ID, newer.RecurrenceFingerprint)
		if err != nil || count != 2 {
			t.Fatalf("CountRecurrences = %d, %v; want 2", count, err)
		}
		byLearning, err := ListRecurrencesByLearning(ctx, tx, learning.ID, 0)
		if err != nil || len(byLearning) != 2 || byLearning[0].ID != newer.ID {
			t.Fatalf("ListRecurrencesByLearning = %#v, %v", byLearning, err)
		}
		all, err := ListAllRecurrences(ctx, tx, project.ID, 0)
		if err != nil || len(all) != 2 || all[0].ID != newer.ID {
			t.Fatalf("ListAllRecurrences = %#v, %v", all, err)
		}
		return nil
	}); err != nil {
		t.Fatalf("query recurrence records: %v", err)
	}
}

func TestSaveRecurrenceRecordRejectsDuplicateIdempotencyKey(t *testing.T) {
	db, project := setupTestDB(t)
	ctx := context.Background()
	learning := newTestLearning(project.ID)
	key := "duplicate-key"
	record := &domain.RecurrenceRecord{
		ID:                    domain.RecurrenceRecordID(newUUID()),
		RecurrenceFingerprint: "pattern",
		LearningID:            learning.ID,
		ProjectID:             project.ID,
		Summary:               "first",
		OccurredAt:            utcNow(),
		IdempotencyKey:        &key,
	}

	err := WithTx(ctx, db, func(tx *sql.Tx) error {
		if err := SaveLearning(ctx, tx, learning); err != nil {
			return err
		}
		if err := SaveRecurrenceRecord(ctx, tx, record); err != nil {
			return err
		}
		duplicate := *record
		duplicate.ID = domain.RecurrenceRecordID(newUUID())
		return SaveRecurrenceRecord(ctx, tx, &duplicate)
	})
	if err == nil {
		t.Fatal("duplicate idempotency key unexpectedly succeeded")
	}
}
