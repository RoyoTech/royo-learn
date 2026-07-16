package publish

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/record"
	"agent-royo-learn/internal/storage"

	"github.com/google/uuid"
)

func TestRollbackRejectsOlderOverlappingPublication(t *testing.T) {
	env := seedPublishEnv(t, "ownership/SKILL.md", true, "original\n")
	defer env.db.Close()
	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
	olderID := publishForRecovery(t, svc, env)
	older := loadPublicationByID(t, env.db, olderID)
	newer := *older
	newer.ID = domain.PublicationID(uuid.Must(uuid.NewV7()).String())
	newer.StartedAt = older.StartedAt.Add(time.Second)
	newer.Rollback = append([]domain.RollbackEntry(nil), older.Rollback...)
	if err := storage.WithTx(context.Background(), env.db, func(tx *sql.Tx) error {
		return storage.SavePublication(context.Background(), tx, &newer)
	}); err != nil {
		t.Fatalf("save newer publication: %v", err)
	}

	err := svc.Rollback(context.Background(), env.projectID, &RollbackPublicationInput{PublicationID: olderID, Actor: env.actor})
	assertDomainCode(t, err, domain.ErrPublicationConflict)
}

func TestInterruptedPublicationIsDiscoverableWithoutKnownID(t *testing.T) {
	env := seedPublishEnv(t, "discover/SKILL.md", false, "")
	defer env.db.Close()
	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
	svc.faults = &FaultHooks{AfterTargetWrite: func(int, string) error { return errors.New("simulated crash") }}
	_, publishErr := svc.Publish(context.Background(), env.projectID, &PublishInput{
		LearningID: env.learningID, PreviewHash: env.previewHash, ApprovalID: env.approvalID,
		Apply: true, Force: true, Actor: env.actor,
	})
	publicationID := recoveryIDFromError(t, publishErr)

	candidates, err := svc.RecoverablePublications(context.Background())
	if err != nil {
		t.Fatalf("RecoverablePublications: %v", err)
	}
	if len(candidates) != 1 || candidates[0].PublicationID != publicationID || candidates[0].JournalStatus != "attempting" {
		t.Fatalf("recovery candidates = %+v, want %s attempting", candidates, publicationID)
	}
}

func TestPublishRetryReconcilesCommittedMaterializationFailure(t *testing.T) {
	env := seedPublishEnv(t, "publish-reconcile/SKILL.md", false, "")
	defer env.db.Close()
	recordsDir := filepath.Join(env.projectRoot, ".royo-learn", "records")
	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir, recordsDir)
	svc.faults = &FaultHooks{BeforeMaterialize: func() error { return errors.New("record unavailable") }}
	_, err := svc.Publish(context.Background(), env.projectID, &PublishInput{
		LearningID: env.learningID, PreviewHash: env.previewHash, ApprovalID: env.approvalID,
		Apply: true, Force: true, Actor: env.actor,
	})
	assertCommittedError(t, err, domain.PubStatusCompleted)
	svc.faults = nil

	result, err := svc.Publish(context.Background(), env.projectID, &PublishInput{
		LearningID: env.learningID, PreviewHash: env.previewHash, ApprovalID: env.approvalID,
		Apply: true, Force: true, Actor: env.actor,
	})
	if err != nil || result == nil || result.Publication == nil {
		t.Fatalf("reconciliation retry result=%+v error=%v", result, err)
	}
	learning := loadMaterializedLearning(t, env)
	if _, found, readErr := record.ReadRecordHash(filepath.Join(recordsDir, string(learning.ID)+".md")); readErr != nil || !found {
		t.Fatalf("reconciled record missing: found=%v error=%v", found, readErr)
	}
}

func TestRollbackRetryReconcilesThenSecondRollbackConflicts(t *testing.T) {
	env := seedPublishEnv(t, "rollback-reconcile/SKILL.md", true, "original\n")
	defer env.db.Close()
	recordsDir := filepath.Join(env.projectRoot, ".royo-learn", "records")
	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir, recordsDir)
	publicationID := publishForRecovery(t, svc, env)
	svc.faults = &FaultHooks{BeforeMaterialize: func() error { return errors.New("record unavailable") }}
	err := svc.Rollback(context.Background(), env.projectID, &RollbackPublicationInput{PublicationID: publicationID, Actor: env.actor})
	assertCommittedError(t, err, domain.PubStatusRolledback)
	svc.faults = nil
	if err := svc.Rollback(context.Background(), env.projectID, &RollbackPublicationInput{PublicationID: publicationID, Actor: env.actor}); err != nil {
		t.Fatalf("reconciliation retry: %v", err)
	}
	err = svc.Rollback(context.Background(), env.projectID, &RollbackPublicationInput{PublicationID: publicationID, Actor: env.actor})
	assertDomainCode(t, err, domain.ErrPublicationConflict)
}

func TestSuccessfulRollbackAuditsActorInStateTransaction(t *testing.T) {
	env := seedPublishEnv(t, "audit/SKILL.md", true, "original\n")
	defer env.db.Close()
	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
	publicationID := publishForRecovery(t, svc, env)
	actor := domain.Actor{Kind: "human", Name: "release-maintainer", SessionID: "audit-session"}
	if err := svc.Rollback(context.Background(), env.projectID, &RollbackPublicationInput{PublicationID: publicationID, Actor: actor}); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	events, err := storage.ListEvents(context.Background(), env.db.DB, storage.AuditEventFilter{
		EntityType: "publication", EntityID: string(publicationID), Operation: "publication.rollback",
	})
	if err != nil || len(events) != 1 || events[0].Actor != actor {
		t.Fatalf("rollback audit events=%+v error=%v", events, err)
	}
}

func TestMissingBackupDoesNotCreateDeletionArtifact(t *testing.T) {
	env := seedPublishEnv(t, "missing-backup/SKILL.md", true, "original\n")
	defer env.db.Close()
	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
	publicationID := publishForRecovery(t, svc, env)
	pub := loadPublicationByID(t, env.db, publicationID)
	if err := os.Remove(pub.Rollback[0].Backup); err != nil {
		t.Fatalf("remove backup: %v", err)
	}
	err := svc.Rollback(context.Background(), env.projectID, &RollbackPublicationInput{PublicationID: publicationID, Actor: env.actor})
	domainErr, ok := domain.AsDomainError(err)
	if !ok || domainErr.Code != domain.ErrRollbackFailed {
		t.Fatalf("rollback error = %v", err)
	}
	if artifact, _ := domainErr.Details["recovery_artifact"].(string); artifact != "" {
		t.Fatalf("missing backup generated misleading artifact %q", artifact)
	}
	if reason, _ := domainErr.Details["reason"].(string); !strings.Contains(reason, "backup") {
		t.Fatalf("rollback reason %q does not expose backup failure", reason)
	}
}

func TestRecoveryArtifactIsRegeneratedFromCurrentState(t *testing.T) {
	env := seedPublishEnv(t, "fresh-artifact/SKILL.md", true, "original\n")
	defer env.db.Close()
	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
	publicationID := publishForRecovery(t, svc, env)
	target := filepath.Join(env.projectRoot, "skills", "fresh-artifact", "SKILL.md")
	if err := os.WriteFile(target, []byte("first user edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = svc.Rollback(context.Background(), env.projectID, &RollbackPublicationInput{PublicationID: publicationID, Actor: env.actor})
	if err := os.WriteFile(target, []byte("second user edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := svc.Rollback(context.Background(), env.projectID, &RollbackPublicationInput{PublicationID: publicationID, Actor: env.actor})
	domainErr, _ := domain.AsDomainError(err)
	artifact, _ := domainErr.Details["recovery_artifact"].(string)
	content, readErr := os.ReadFile(artifact)
	if readErr != nil || !strings.Contains(string(content), "second user edit") {
		t.Fatalf("artifact was stale: path=%q content=%q error=%v", artifact, content, readErr)
	}
}

func TestRecoveryArtifactWriteFailureIsSurfaced(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission failure is not deterministic on Windows")
	}
	env := seedPublishEnv(t, "artifact-write/SKILL.md", true, "original\n")
	defer env.db.Close()
	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
	publicationID := publishForRecovery(t, svc, env)
	target := filepath.Join(env.projectRoot, "skills", "artifact-write", "SKILL.md")
	if err := os.WriteFile(target, []byte("user edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	recoveryDir := filepath.Join(env.projectRoot, ".royo-learn", "recovery")
	if err := os.MkdirAll(recoveryDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(recoveryDir, 0o700) })
	err := svc.Rollback(context.Background(), env.projectID, &RollbackPublicationInput{PublicationID: publicationID, Actor: env.actor})
	domainErr, _ := domain.AsDomainError(err)
	if reason, _ := domainErr.Details["reason"].(string); !strings.Contains(reason, "artifact") {
		t.Fatalf("artifact write failure was suppressed: %v", err)
	}
}
