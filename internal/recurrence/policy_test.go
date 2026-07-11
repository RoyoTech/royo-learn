package recurrence

import (
	"context"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"

	"github.com/google/uuid"
)

func TestCheckNeedsReview_NoOccurrences(t *testing.T) {
	db, proj := setupRecurrenceDB(t)
	ctx := context.Background()
	learning := newTestLearning()
	saveLearningInDB(t, db, learning, proj.ID)

	status, err := CheckNeedsReview(ctx, db, proj.ID, learning)
	if err != nil {
		t.Fatalf("CheckNeedsReview: %v", err)
	}
	// A learning with no recurrence records should not need review yet.
	if status.NeedsReview {
		t.Fatal("expected NeedsReview=false for learning with no recurrences")
	}
}

func TestCheckNeedsReview_RecentRecurrence(t *testing.T) {
	db, proj := setupRecurrenceDB(t)
	ctx := context.Background()
	learning := newTestLearning()
	saveLearningInDB(t, db, learning, proj.ID)

	_, err := RecordRecurrence(ctx, db, proj.ID, learning)
	if err != nil {
		t.Fatalf("RecordRecurrence: %v", err)
	}

	status, err := CheckNeedsReview(ctx, db, proj.ID, learning)
	if err != nil {
		t.Fatalf("CheckNeedsReview: %v", err)
	}
	if status.NeedsReview {
		t.Fatal("expected NeedsReview=false for recently recurred learning")
	}
}

func TestCheckNeedsReview_StaleRecurrence(t *testing.T) {
	db, proj := setupRecurrenceDB(t)
	ctx := context.Background()

	// Create a learning that was observed long ago.
	learning := newTestLearning()
	learning.Title = "Old pattern"
	learning.Observation = "Old observation"
	learning.ReusableLesson = "Old lesson"
	saveLearningInDB(t, db, learning, proj.ID)

	// Manually insert a recurrence record with a timestamp 100 days ago.
	oldTime := time.Now().UTC().AddDate(0, 0, -100)
	saveRecurrenceAtTime(t, db, learning, proj.ID, oldTime)

	status, err := CheckNeedsReview(ctx, db, proj.ID, learning)
	if err != nil {
		t.Fatalf("CheckNeedsReview: %v", err)
	}
	if !status.NeedsReview {
		t.Fatalf("expected NeedsReview=true for learning stale >90 days, got false. reason=%q", status.Reason)
	}
}

func TestCheckNeedsReview_NilLearning(t *testing.T) {
	db, proj := setupRecurrenceDB(t)
	ctx := context.Background()

	status, err := CheckNeedsReview(ctx, db, proj.ID, nil)
	if err != nil {
		t.Fatalf("CheckNeedsReview: %v", err)
	}
	if status.NeedsReview {
		t.Fatal("expected NeedsReview=false for nil learning")
	}
}

func saveRecurrenceAtTime(t *testing.T, db *storage.DB, learning *domain.Learning, projID domain.ProjectID, at time.Time) {
	t.Helper()
	ctx := context.Background()
	fp := RecurrenceFingerprint(learning)
	rec := &domain.RecurrenceRecord{
		ID:                    domain.RecurrenceRecordID(uuid.Must(uuid.NewV7()).String()),
		RecurrenceFingerprint: fp,
		LearningID:            learning.ID,
		ProjectID:             projID,
		Summary:               learning.Title,
		OccurredAt:            at,
	}
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer tx.Rollback()
	if err := storage.SaveRecurrenceRecord(ctx, tx, rec); err != nil {
		t.Fatalf("SaveRecurrenceRecord: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}
