package publish

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/record"
	"agent-royo-learn/internal/storage"
)

// TestRollback_SuccessRevokesPublishedStatus pins the D18 rule at the unit
// level: a successful rollback returns the learning to `approved` and rewrites
// its derived record, because the published content no longer exists.
func TestRollback_SuccessRevokesPublishedStatus(t *testing.T) {
	env := seedPublishEnv(t, "rb-state/SKILL.md", true, "# Original\n")
	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir, env.recordsDir)
	ctx := context.Background()

	pubID := publishForRollback(t, svc, env)

	if err := svc.Rollback(ctx, env.projectID, &RollbackPublicationInput{
		PublicationID: pubID,
		Actor:         env.actor,
	}); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	learning := loadLearning(t, env)
	if learning.Status != domain.StatusApproved {
		t.Fatalf("learning status after successful rollback = %q, want %q",
			learning.Status, domain.StatusApproved)
	}

	// The derived record must follow the truth, not lag behind it (D18 rule 5).
	stored, found, err := record.ReadRecordHash(filepath.Join(env.recordsDir, string(env.learningID)+".md"))
	if err != nil {
		t.Fatalf("ReadRecordHash: %v", err)
	}
	if !found {
		t.Fatal("rollback did not materialize the Markdown record")
	}
	if stored != record.RecordHash(learning) {
		t.Fatal("the Markdown record still disagrees with SQLite after rollback")
	}
}

// TestRollback_FailureLeavesLearningUntouched is the other half of D18 rule 1.
// If the content could NOT be restored it may well still be published, so
// stamping `approved` over it would trade one false state for another. The
// learning stays as it is and the publication is marked failed.
func TestRollback_FailureLeavesLearningUntouched(t *testing.T) {
	env := seedPublishEnv(t, "rb-fail/SKILL.md", true, "# Original\n")
	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir, env.recordsDir)
	ctx := context.Background()

	pubID := publishForRollback(t, svc, env)

	// Make the restore fail portably: replace the published file with a
	// directory of the same name, so writing the backup back over it cannot
	// succeed on any OS.
	target := filepath.Join(env.projectRoot, "skills", "rb-fail", "SKILL.md")
	if err := os.Remove(target); err != nil {
		t.Fatalf("remove target: %v", err)
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("mkdir over target: %v", err)
	}

	if err := svc.Rollback(ctx, env.projectID, &RollbackPublicationInput{
		PublicationID: pubID,
		Actor:         env.actor,
	}); err != nil {
		t.Fatalf("Rollback returned an error: %v", err)
	}

	if learning := loadLearning(t, env); learning.Status != domain.StatusPublished {
		t.Fatalf("a FAILED rollback changed the learning status to %q; "+
			"the content was never restored, so `published` is still the truth",
			learning.Status)
	}

	pub := loadPublication(t, env, pubID)
	if pub.Status != domain.PubStatusFailed {
		t.Fatalf("publication status after failed rollback = %q, want %q",
			pub.Status, domain.PubStatusFailed)
	}
}

// publishForRollback publishes the seeded learning and returns the publication
// id, failing the test if the publication itself did not complete.
func publishForRollback(t *testing.T, svc *Service, env *publishTestEnv) domain.PublicationID {
	t.Helper()
	result, err := svc.Publish(context.Background(), env.projectID, &PublishInput{
		Apply:       true,
		LearningID:  env.learningID,
		PreviewHash: env.previewHash,
		Force:       true,
		Actor:       env.actor,
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if result.Publication.Status != domain.PubStatusCompleted {
		t.Fatalf("publication status = %q, want completed", result.Publication.Status)
	}
	return result.Publication.ID
}

func loadLearning(t *testing.T, env *publishTestEnv) *domain.Learning {
	t.Helper()
	tx, err := env.db.DB.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()
	learning, err := storage.GetLearning(context.Background(), tx, env.learningID)
	if err != nil {
		t.Fatalf("GetLearning: %v", err)
	}
	if learning == nil {
		t.Fatal("GetLearning: learning is gone")
	}
	return learning
}

func loadPublication(t *testing.T, env *publishTestEnv, pubID domain.PublicationID) *domain.Publication {
	t.Helper()
	tx, err := env.db.DB.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()
	pub, err := storage.GetPublication(context.Background(), tx, pubID)
	if err != nil {
		t.Fatalf("GetPublication: %v", err)
	}
	return pub
}
