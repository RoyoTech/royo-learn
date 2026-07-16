package publish

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/record"
	"agent-royo-learn/internal/storage"
)

func TestPublishAndRollbackMaterializeTruthAndReturnToApproved(t *testing.T) {
	env := seedPublishEnv(t, "materialized/SKILL.md", true, "original\n")
	defer env.db.Close()
	recordsDir := filepath.Join(env.projectRoot, ".royo-learn", "records")
	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir, recordsDir)

	publicationID := publishForRecovery(t, svc, env)
	published := loadMaterializedLearning(t, env)
	if published.Status != domain.StatusPublished {
		t.Fatalf("learning status after publish = %q, want published", published.Status)
	}
	assertMaterializedHash(t, recordsDir, published)

	if err := svc.Rollback(context.Background(), env.projectID, &RollbackPublicationInput{PublicationID: publicationID, Actor: env.actor}); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	approved := loadMaterializedLearning(t, env)
	if approved.Status != domain.StatusApproved {
		t.Fatalf("learning status after rollback = %q, want approved", approved.Status)
	}
	assertMaterializedHash(t, recordsDir, approved)
	if domain.CanTransition(domain.StatusPublished, domain.StatusApproved) {
		t.Fatal("published -> approved must not be exposed through curation ValidTransitions")
	}
}

func TestRollbackConflictLeavesLearningPublished(t *testing.T) {
	env := seedPublishEnv(t, "materialization-conflict/SKILL.md", true, "original\n")
	defer env.db.Close()
	recordsDir := filepath.Join(env.projectRoot, ".royo-learn", "records")
	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir, recordsDir)
	publicationID := publishForRecovery(t, svc, env)
	target := filepath.Join(env.projectRoot, "skills", "materialization-conflict", "SKILL.md")
	if err := os.WriteFile(target, []byte("user conflict\n"), 0o644); err != nil {
		t.Fatalf("write conflict: %v", err)
	}

	if err := svc.Rollback(context.Background(), env.projectID, &RollbackPublicationInput{PublicationID: publicationID, Actor: env.actor}); err == nil {
		t.Fatal("Rollback returned nil for a destination conflict")
	}
	if learning := loadMaterializedLearning(t, env); learning.Status != domain.StatusPublished {
		t.Fatalf("failed rollback changed learning status to %q, want published", learning.Status)
	}
}

func TestPublishMaterializationFailureReportsCommittedState(t *testing.T) {
	env := seedPublishEnv(t, "materialization-failure/SKILL.md", false, "")
	defer env.db.Close()
	recordsDir := filepath.Join(env.projectRoot, ".royo-learn", "records")
	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir, recordsDir)
	materializeErr := errors.New("materializer unavailable")
	svc.faults = &FaultHooks{BeforeMaterialize: func() error { return materializeErr }}

	_, err := svc.Publish(context.Background(), env.projectID, &PublishInput{
		LearningID: env.learningID, PreviewHash: env.previewHash,
		Apply: true, Force: true, Actor: env.actor,
	})
	assertCommittedError(t, err, domain.PubStatusCompleted)
	if !errors.Is(err, materializeErr) {
		t.Fatalf("committed error lost materialization cause: %v", err)
	}
	if learning := loadMaterializedLearning(t, env); learning.Status != domain.StatusPublished {
		t.Fatalf("committed publish reported learning status %q, want published", learning.Status)
	}
}

func TestPublishPreservesMaterializationAndTerminalAuditFailures(t *testing.T) {
	env := seedPublishEnv(t, "combined-failure/SKILL.md", false, "")
	defer env.db.Close()
	recordsDir := filepath.Join(env.projectRoot, ".royo-learn", "records")
	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir, recordsDir)
	materializeErr := errors.New("materialization failed")
	auditErr := errors.New("terminal journal failed")
	svc.faults = &FaultHooks{
		BeforeMaterialize:     func() error { return materializeErr },
		BeforeTerminalJournal: func() error { return auditErr },
	}

	_, err := svc.Publish(context.Background(), env.projectID, &PublishInput{
		LearningID: env.learningID, PreviewHash: env.previewHash,
		Apply: true, Force: true, Actor: env.actor,
	})
	assertCommittedError(t, err, domain.PubStatusCompleted)
	if !errors.Is(err, materializeErr) || !errors.Is(err, auditErr) {
		t.Fatalf("combined committed error lost a cause: %v", err)
	}
}

func TestRollbackMaterializationFailureReportsCommittedApprovedState(t *testing.T) {
	env := seedPublishEnv(t, "rollback-materialization/SKILL.md", true, "original\n")
	defer env.db.Close()
	recordsDir := filepath.Join(env.projectRoot, ".royo-learn", "records")
	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir, recordsDir)
	publicationID := publishForRecovery(t, svc, env)
	materializeErr := errors.New("rollback materialization failed")
	svc.faults = &FaultHooks{BeforeMaterialize: func() error { return materializeErr }}

	err := svc.Rollback(context.Background(), env.projectID, &RollbackPublicationInput{PublicationID: publicationID, Actor: env.actor})
	assertCommittedError(t, err, domain.PubStatusRolledback)
	if learning := loadMaterializedLearning(t, env); learning.Status != domain.StatusApproved {
		t.Fatalf("committed rollback learning status = %q, want approved", learning.Status)
	}
}

func assertMaterializedHash(t *testing.T, recordsDir string, learning *domain.Learning) {
	t.Helper()
	hash, found, err := record.ReadRecordHash(filepath.Join(recordsDir, string(learning.ID)+".md"))
	if err != nil {
		t.Fatalf("ReadRecordHash: %v", err)
	}
	if !found || hash != record.RecordHash(learning) {
		t.Fatalf("record hash = %q found=%v, want %q", hash, found, record.RecordHash(learning))
	}
}

func loadMaterializedLearning(t *testing.T, env *publishTestEnv) *domain.Learning {
	t.Helper()
	tx, err := env.db.DB.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("begin learning read transaction: %v", err)
	}
	defer tx.Rollback()
	learning, err := storage.GetLearning(context.Background(), tx, env.learningID)
	if err != nil {
		t.Fatalf("GetLearning: %v", err)
	}
	return learning
}

func assertCommittedError(t *testing.T, err error, status domain.PublicationStatus) {
	t.Helper()
	var domainErr *domain.DomainError
	if !errors.As(err, &domainErr) {
		t.Fatalf("error = %v, want committed domain error", err)
	}
	if domainErr.Details["committed"] != true || domainErr.Details["status"] != string(status) {
		t.Fatalf("committed details = %+v, want status %q", domainErr.Details, status)
	}
	if !strings.Contains(strings.ToLower(domainErr.NextAction), "do not retry") {
		t.Fatalf("next action allows blind retry: %q", domainErr.NextAction)
	}
}
