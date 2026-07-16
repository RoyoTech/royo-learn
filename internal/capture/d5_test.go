package capture

import (
	"context"
	"database/sql"
	"testing"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"
	"agent-royo-learn/internal/testutil"
)

// countRecurrences returns the number of recurrence records for a project.
func countRecurrences(t *testing.T, db *storage.DB, projectID domain.ProjectID) int {
	t.Helper()
	ctx := context.Background()
	tx, err := db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer tx.Rollback()
	records, err := storage.ListAllRecurrences(ctx, tx, projectID, 100)
	if err != nil {
		t.Fatalf("ListAllRecurrences: %v", err)
	}
	return len(records)
}

// TestCapture_D5ThreeCases proves the three D5 capture semantics exactly:
//
//	same idempotency_key     -> technical retry: no new learning, no new recurrence
//	different key + same hash -> equivalent event: reuse learning, record recurrence
//	no key + same hash        -> conservative dedup: no automatic recurrence
func TestCapture_D5ThreeCases(t *testing.T) {
	base := func() *CaptureInput {
		return &CaptureInput{
			Title:       "D5 pattern",
			Context:     "Same content across events",
			Observation: "The same failure recurred",
			Lesson:      "Guard the config before load",
			Type:        domain.TypeProcedure,
			Scope:       domain.ScopeProject,
			Actor:       domain.Actor{Kind: "agent", Name: "test-agent"},
		}
	}

	t.Run("same_key_is_technical_retry", func(t *testing.T) {
		db, proj := setupCaptureDB(t)
		svc := NewService(db, testutil.TempDir(t))
		ctx := context.Background()

		in := base()
		in.IdempotencyKey = "key-A"

		first, err := svc.Capture(ctx, proj.ID, in)
		if err != nil {
			t.Fatalf("Capture #1: %v", err)
		}
		if !first.New {
			t.Fatal("first capture should be New")
		}

		retry, err := svc.Capture(ctx, proj.ID, in)
		if err != nil {
			t.Fatalf("Capture retry: %v", err)
		}
		if retry.New {
			t.Fatal("same-key retry should not be New")
		}
		if retry.RecurrenceRecorded {
			t.Fatal("same-key retry must not record a recurrence")
		}
		if retry.LearningID != first.LearningID {
			t.Fatalf("retry LearningID = %q, want %q", retry.LearningID, first.LearningID)
		}
		if got := countRecurrences(t, db, proj.ID); got != 0 {
			t.Fatalf("recurrence count = %d, want 0 (technical retry recorded a recurrence)", got)
		}
	})

	t.Run("different_key_same_hash_is_equivalent_event", func(t *testing.T) {
		db, proj := setupCaptureDB(t)
		svc := NewService(db, testutil.TempDir(t))
		ctx := context.Background()

		in1 := base()
		in1.IdempotencyKey = "key-A"
		first, err := svc.Capture(ctx, proj.ID, in1)
		if err != nil {
			t.Fatalf("Capture #1: %v", err)
		}

		in2 := base()
		in2.IdempotencyKey = "key-B"
		equivalent, err := svc.Capture(ctx, proj.ID, in2)
		if err != nil {
			t.Fatalf("Capture equivalent: %v", err)
		}
		if equivalent.New {
			t.Fatal("equivalent event must reuse the learning (New should be false)")
		}
		if equivalent.LearningID != first.LearningID {
			t.Fatalf("equivalent LearningID = %q, want %q", equivalent.LearningID, first.LearningID)
		}
		if !equivalent.RecurrenceRecorded {
			t.Fatal("equivalent event must record a recurrence")
		}
		if got := countRecurrences(t, db, proj.ID); got != 1 {
			t.Fatalf("recurrence count = %d, want 1 (equivalent event did not record)", got)
		}

		// Retrying the equivalent event with the same key must be idempotent.
		retry, err := svc.Capture(ctx, proj.ID, in2)
		if err != nil {
			t.Fatalf("Capture equivalent retry: %v", err)
		}
		if retry.RecurrenceRecorded {
			t.Fatal("retry of an equivalent event must not record a second recurrence")
		}
		if got := countRecurrences(t, db, proj.ID); got != 1 {
			t.Fatalf("after retry recurrence count = %d, want 1 (D5 duplicated a recurrence)", got)
		}
	})

	t.Run("no_key_same_hash_is_conservative_dedup", func(t *testing.T) {
		db, proj := setupCaptureDB(t)
		svc := NewService(db, testutil.TempDir(t))
		ctx := context.Background()

		first, err := svc.Capture(ctx, proj.ID, base())
		if err != nil {
			t.Fatalf("Capture #1: %v", err)
		}

		dup, err := svc.Capture(ctx, proj.ID, base())
		if err != nil {
			t.Fatalf("Capture dup: %v", err)
		}
		if dup.New {
			t.Fatal("keyless duplicate must dedup (New should be false)")
		}
		if dup.LearningID != first.LearningID {
			t.Fatalf("dup LearningID = %q, want %q", dup.LearningID, first.LearningID)
		}
		if dup.RecurrenceRecorded {
			t.Fatal("keyless duplicate must not record an automatic recurrence")
		}
		if got := countRecurrences(t, db, proj.ID); got != 0 {
			t.Fatalf("recurrence count = %d, want 0 (conservative dedup recorded a recurrence)", got)
		}
	})
}
