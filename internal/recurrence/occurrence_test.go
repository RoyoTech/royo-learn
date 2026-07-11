package recurrence

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"

	"github.com/google/uuid"
)

func setupRecurrenceDB(t *testing.T) (*storage.DB, *domain.Project) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "recurrence.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	proj := &domain.Project{
		ID:            domain.ProjectID(uuid.Must(uuid.NewV7()).String()),
		ProjectKey:    "rec-test",
		DisplayName:   "Recurrence Test",
		CanonicalPath: "/tmp/rec-test",
		GitRemote:     "",
		Fingerprint:   "fp-rec",
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	ctx := context.Background()
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	if err := storage.SaveProject(ctx, tx, proj); err != nil {
		tx.Rollback()
		t.Fatalf("SaveProject: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return db, proj
}

func newTestLearning() *domain.Learning {
	return &domain.Learning{
		ID:             domain.LearningID(uuid.Must(uuid.NewV7()).String()),
		Title:          "Test bug pattern",
		Context:        "Test context",
		Observation:    "Observed crash on startup",
		ReusableLesson: "Always validate config before loading",
		Status:         domain.StatusCaptured,
		Type:           domain.TypeProcedure,
		ScopeGuess:     domain.ScopeProject,
		Confidence:     domain.ConfidenceHigh,
		EvidenceLevel:  domain.EvidenceStrong,
		Fingerprint:    "fp-001",
		NormalizedHash: "nh-001",
		Actor:          domain.Actor{Kind: "agent", Name: "test"},
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
}

func saveLearningInDB(t *testing.T, db *storage.DB, learning *domain.Learning, projID domain.ProjectID) {
	t.Helper()
	ctx := context.Background()
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer tx.Rollback()
	learning.ProjectID = projID
	if err := storage.SaveLearning(ctx, tx, learning); err != nil {
		t.Fatalf("SaveLearning: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

func TestRecordRecurrence_CreatesRecord(t *testing.T) {
	db, proj := setupRecurrenceDB(t)
	ctx := context.Background()
	learning := newTestLearning()
	saveLearningInDB(t, db, learning, proj.ID)

	rec, err := RecordRecurrence(ctx, db, proj.ID, learning)
	if err != nil {
		t.Fatalf("RecordRecurrence: %v", err)
	}
	if rec == nil {
		t.Fatal("expected non-nil recurrence record")
	}
	if rec.LearningID != learning.ID {
		t.Fatalf("LearningID = %q, want %q", rec.LearningID, learning.ID)
	}
	if rec.ProjectID != proj.ID {
		t.Fatalf("ProjectID = %q, want %q", rec.ProjectID, proj.ID)
	}
	expectedFP := RecurrenceFingerprint(learning)
	if rec.RecurrenceFingerprint != expectedFP {
		t.Fatalf("RecurrenceFingerprint = %q, want %q", rec.RecurrenceFingerprint, expectedFP)
	}
}

func TestRecordRecurrence_TwiceIncrementsCount(t *testing.T) {
	db, proj := setupRecurrenceDB(t)
	ctx := context.Background()
	l1 := newTestLearning()
	saveLearningInDB(t, db, l1, proj.ID)

	// Record first occurrence.
	_, err := RecordRecurrence(ctx, db, proj.ID, l1)
	if err != nil {
		t.Fatalf("RecordRecurrence #1: %v", err)
	}

	// Second learning with same content should produce same fingerprint.
	l2 := newTestLearning()
	l2.ID = domain.LearningID(uuid.Must(uuid.NewV7()).String())
	l2.Title = l1.Title
	l2.Observation = l1.Observation
	l2.ReusableLesson = l1.ReusableLesson
	saveLearningInDB(t, db, l2, proj.ID)

	rec2, err := RecordRecurrence(ctx, db, proj.ID, l2)
	if err != nil {
		t.Fatalf("RecordRecurrence #2: %v", err)
	}
	if rec2 == nil {
		t.Fatal("expected non-nil second record")
	}

	// Verify count increased.
	tx, err := db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer tx.Rollback()

	fp := RecurrenceFingerprint(l1)
	count, err := storage.CountRecurrences(ctx, tx, proj.ID, fp)
	if err != nil {
		t.Fatalf("CountRecurrences: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
}

func TestListRecurrencesForLearning_ReturnsRecords(t *testing.T) {
	db, proj := setupRecurrenceDB(t)
	ctx := context.Background()
	learning := newTestLearning()
	saveLearningInDB(t, db, learning, proj.ID)

	_, err := RecordRecurrence(ctx, db, proj.ID, learning)
	if err != nil {
		t.Fatalf("RecordRecurrence: %v", err)
	}

	records, err := ListRecurrencesForLearning(ctx, db, learning.ID, 10)
	if err != nil {
		t.Fatalf("ListRecurrencesForLearning: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("expected at least one recurrence record")
	}
	if records[0].LearningID != learning.ID {
		t.Fatalf("LearningID = %q, want %q", records[0].LearningID, learning.ID)
	}
}
