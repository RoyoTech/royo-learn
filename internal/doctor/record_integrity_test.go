package doctor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"agent-royo-learn/internal/coherence"
	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"
	"agent-royo-learn/internal/testutil"

	"github.com/google/uuid"
)

// TestRecordIntegrity_DetectsAndClears proves the doctor check FAILS on a
// SQLite<->Markdown divergence and PASSES once rebuild-index (coherence.Repair)
// reconciles it — the §4.7 "doctor detects, rebuild-index repairs" contract at
// the doctor surface.
func TestRecordIntegrity_DetectsAndClears(t *testing.T) {
	dir, err := os.MkdirTemp("", "doctor-ri-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	path := filepath.Join(dir, "royo-learn.db")
	db, err := storage.Open(path)
	if err != nil {
		os.RemoveAll(dir)
		t.Fatalf("Open: %v", err)
	}
	if err := storage.Migrate(db); err != nil {
		db.Close()
		os.RemoveAll(dir)
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		_ = testutil.RemoveAllWithRetry(dir)
	})

	now := time.Now().UTC().Truncate(time.Second)
	proj := &domain.Project{
		ID: domain.ProjectID(uuid.Must(uuid.NewV7()).String()), ProjectKey: "ri", DisplayName: "ri",
		CanonicalPath: dir, Fingerprint: "fp", CreatedAt: now, UpdatedAt: now,
	}
	ctx := context.Background()
	tx, _ := db.DB.BeginTx(ctx, nil)
	if err := storage.SaveProject(ctx, tx, proj); err != nil {
		tx.Rollback()
		t.Fatalf("SaveProject: %v", err)
	}
	tx.Commit()

	l := &domain.Learning{
		ID: domain.LearningID(uuid.Must(uuid.NewV7()).String()), ProjectID: proj.ID,
		Status: domain.StatusCaptured, Type: domain.TypeProcedure, Title: "t", Context: "c",
		Observation: "o", ReusableLesson: "les", ScopeGuess: domain.ScopeProject,
		Confidence: domain.ConfidenceMedium, EvidenceLevel: domain.EvidenceModerate,
		ProposedDestination: domain.DestProject, Fingerprint: "f", NormalizedHash: "h",
		Actor: domain.Actor{Kind: "agent", Name: "seed"}, Revision: 1, CreatedAt: now, UpdatedAt: now,
	}
	wtx, _ := db.DB.BeginTx(ctx, nil)
	if err := storage.SaveLearning(ctx, wtx, l); err != nil {
		wtx.Rollback()
		t.Fatalf("SaveLearning: %v", err)
	}
	wtx.Commit()

	recordsDir := filepath.Join(dir, "records")

	// A learning in SQLite with no record on disk is a divergence.
	runner := NewRunner(
		WithProjectRoot(dir),
		WithStore(db, proj.ID, recordsDir),
	)
	defer runner.Close()

	check, err := runner.RunCheck(ctx, "record-integrity")
	if err != nil {
		t.Fatalf("RunCheck: %v", err)
	}
	if check.Status != StatusFail {
		t.Fatalf("record-integrity before repair = %q, want fail: %s", check.Status, check.Message)
	}

	// rebuild-index repairs it.
	if _, err := coherence.Repair(ctx, db, proj.ID, recordsDir); err != nil {
		t.Fatalf("Repair: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(recordsDir, string(l.ID)+".md")); statErr != nil {
		t.Fatalf("record not materialized: %v", statErr)
	}

	check, err = runner.RunCheck(ctx, "record-integrity")
	if err != nil {
		t.Fatalf("RunCheck after repair: %v", err)
	}
	if check.Status != StatusPass {
		t.Fatalf("record-integrity after repair = %q, want pass: %s", check.Status, check.Message)
	}
}
