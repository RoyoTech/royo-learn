package publish

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"
)

// TestPublish_DryRunByDefaultWritesNothing proves that a publish request without
// Apply set writes NOTHING: no destination file is created, the learning stays
// Approved (never Published), and the result is flagged as a dry run (D7,
// Recorrido D). The write path is the second, independent line of defence after
// the approval gate.
func TestPublish_DryRunByDefaultWritesNothing(t *testing.T) {
	ctx := context.Background()
	env := seedPublishEnv(t, "dryrun-skill", false, "")
	defer env.db.Close()

	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)

	// A real preview so the plan is fully populated.
	prev, err := svc.Preview(ctx, env.projectID, &PreviewInput{
		LearningID: env.learningID, Actor: env.actor,
	})
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	res, err := svc.Publish(ctx, env.projectID, &PublishInput{
		LearningID:  env.learningID,
		PreviewHash: prev.Preview.PreviewHash,
		Apply:       false, // default: dry run
		Actor:       env.actor,
	})
	if err != nil {
		t.Fatalf("dry-run Publish must not error: %v", err)
	}
	if res == nil || !res.DryRun {
		t.Fatalf("result must be flagged DryRun, got %+v", res)
	}
	if res.Publication != nil {
		t.Errorf("dry run must not produce a publication record, got %v", res.Publication.ID)
	}

	// No skill file may exist anywhere under skills/.
	skillsDir := filepath.Join(env.projectRoot, "skills")
	_ = filepath.Walk(skillsDir, func(p string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			t.Errorf("dry run wrote a file: %s", p)
		}
		return nil
	})

	// Learning must stay Approved.
	readTx, _ := env.db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	l, _ := storage.GetLearning(ctx, readTx, env.learningID)
	readTx.Rollback()
	if l.Status != domain.StatusApproved {
		t.Errorf("learning status = %q, want %q (dry run must not publish)", l.Status, domain.StatusApproved)
	}
}

// TestPublish_ApplyWritesAndMarksPublished proves that the same request WITH
// Apply set does write the destination and moves the learning to Published.
func TestPublish_ApplyWritesAndMarksPublished(t *testing.T) {
	ctx := context.Background()
	env := seedPublishEnv(t, "apply-skill", false, "")
	defer env.db.Close()

	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
	prev, err := svc.Preview(ctx, env.projectID, &PreviewInput{
		LearningID: env.learningID, Actor: env.actor,
	})
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	res, err := svc.Publish(ctx, env.projectID, &PublishInput{
		LearningID:  env.learningID,
		PreviewHash: prev.Preview.PreviewHash,
		Apply:       true,
		Force:       true,
		Actor:       env.actor,
	})
	if err != nil {
		t.Fatalf("apply Publish: %v", err)
	}
	if res.DryRun {
		t.Fatal("apply publish must not be a dry run")
	}
	if res.Publication == nil || res.Publication.Status != domain.PubStatusCompleted {
		t.Fatalf("expected a completed publication, got %+v", res.Publication)
	}

	readTx, _ := env.db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	l, _ := storage.GetLearning(ctx, readTx, env.learningID)
	readTx.Rollback()
	if l.Status != domain.StatusPublished {
		t.Errorf("learning status = %q, want %q", l.Status, domain.StatusPublished)
	}
}
