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
	"agent-royo-learn/internal/storage"
)

func TestPublishPersistsReachableAttemptBeforeFirstWrite(t *testing.T) {
	env := seedPublishEnv(t, "attempt/SKILL.md", true, "original\n")
	defer env.db.Close()
	target := filepath.Join(env.projectRoot, "skills", "attempt", "SKILL.md")
	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
	svc.faults = &FaultHooks{
		AfterAttemptPersisted: func(domain.PublicationID) error {
			return errors.New("simulated interruption before first target write")
		},
	}

	_, err := svc.Publish(context.Background(), env.projectID, &PublishInput{
		LearningID: env.learningID, PreviewHash: env.previewHash,
		Apply: true, Force: true, Actor: env.actor,
	})
	publicationID := recoveryIDFromError(t, err)
	pub := loadPublicationByID(t, env.db, publicationID)
	if pub.Status != domain.PubStatusInProgress {
		t.Fatalf("attempt status = %q, want in_progress", pub.Status)
	}
	if len(pub.Rollback) != 1 {
		t.Fatalf("rollback metadata count = %d, want 1", len(pub.Rollback))
	}
	assertCompleteRecoveryEntry(t, pub.Rollback[0])
	got, readErr := os.ReadFile(target)
	if readErr != nil || string(got) != "original\n" {
		t.Fatalf("target changed before first write: content=%q error=%v", got, readErr)
	}
}

func TestCrashAfterFirstWriteIsRecoverableByAdvertisedID(t *testing.T) {
	env := seedPublishEnv(t, "crash/SKILL.md", true, "original\n")
	defer env.db.Close()
	target := filepath.Join(env.projectRoot, "skills", "crash", "SKILL.md")
	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
	svc.faults = &FaultHooks{
		AfterTargetWrite: func(index int, _ string) error {
			if index == 0 {
				return errors.New("simulated process loss")
			}
			return nil
		},
	}

	_, err := svc.Publish(context.Background(), env.projectID, &PublishInput{
		LearningID: env.learningID, PreviewHash: env.previewHash,
		Apply: true, Force: true, Actor: env.actor,
	})
	publicationID := recoveryIDFromError(t, err)
	if got, readErr := os.ReadFile(target); readErr != nil || string(got) == "original\n" {
		t.Fatalf("crash seam did not occur after the write: content=%q error=%v", got, readErr)
	}

	svc.faults = nil
	if err := svc.Rollback(context.Background(), env.projectID, &RollbackPublicationInput{
		PublicationID: publicationID,
		Actor:         env.actor,
	}); err != nil {
		t.Fatalf("rollback by advertised recovery id: %v", err)
	}
	got, readErr := os.ReadFile(target)
	if readErr != nil || string(got) != "original\n" {
		t.Fatalf("recovery did not restore original: content=%q error=%v", got, readErr)
	}
	if pub := loadPublicationByID(t, env.db, publicationID); pub.Status != domain.PubStatusRolledback {
		t.Fatalf("publication status = %q, want rolled_back", pub.Status)
	}
}

func TestRollbackRetryConvergesAfterProgressPersistenceFailure(t *testing.T) {
	env := seedPublishEnv(t, "progress/SKILL.md", true, "original\n")
	defer env.db.Close()
	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
	publicationID := publishForRecovery(t, svc, env)
	fired := false
	svc.faults = &FaultHooks{
		BeforeRollbackProgress: func(index int) error {
			if index == 0 && !fired {
				fired = true
				return errors.New("simulated progress update failure")
			}
			return nil
		},
	}

	if err := svc.Rollback(context.Background(), env.projectID, &RollbackPublicationInput{PublicationID: publicationID, Actor: env.actor}); err == nil {
		t.Fatal("first rollback returned nil despite progress persistence failure")
	}
	target := filepath.Join(env.projectRoot, "skills", "progress", "SKILL.md")
	if got, _ := os.ReadFile(target); string(got) != "original\n" {
		t.Fatalf("filesystem was not restored before injected DB failure: %q", got)
	}

	svc.faults = nil
	if err := svc.Rollback(context.Background(), env.projectID, &RollbackPublicationInput{PublicationID: publicationID, Actor: env.actor}); err != nil {
		t.Fatalf("convergent retry: %v", err)
	}
	if pub := loadPublicationByID(t, env.db, publicationID); pub.Status != domain.PubStatusRolledback || pub.Rollback[0].RecoveryState != domain.RecoveryRestored {
		t.Fatalf("retry did not converge: status=%q recovery=%q", pub.Status, pub.Rollback[0].RecoveryState)
	}
}

func TestRollbackRetryConvergesAfterFinalDatabaseFailure(t *testing.T) {
	env := seedPublishEnv(t, "final-db/SKILL.md", true, "original\n")
	defer env.db.Close()
	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
	publicationID := publishForRecovery(t, svc, env)
	svc.faults = &FaultHooks{BeforeRollbackCommit: func() error { return errors.New("simulated final commit failure") }}

	err := svc.Rollback(context.Background(), env.projectID, &RollbackPublicationInput{PublicationID: publicationID, Actor: env.actor})
	var domainErr *domain.DomainError
	if !errors.As(err, &domainErr) || domainErr.Details["database_state"] != "rollback_pending" {
		t.Fatalf("rollback error = %v, want actionable rollback_pending state", err)
	}
	target := filepath.Join(env.projectRoot, "skills", "final-db", "SKILL.md")
	if got, _ := os.ReadFile(target); string(got) != "original\n" {
		t.Fatalf("filesystem was not restored before final DB failure: %q", got)
	}

	svc.faults = nil
	if err := svc.Rollback(context.Background(), env.projectID, &RollbackPublicationInput{PublicationID: publicationID, Actor: env.actor}); err != nil {
		t.Fatalf("retry after final DB failure: %v", err)
	}
	if pub := loadPublicationByID(t, env.db, publicationID); pub.Status != domain.PubStatusRolledback {
		t.Fatalf("publication status = %q, want rolled_back", pub.Status)
	}
}

func TestRollbackMalformedOrLegacyMetadataFailsClosed(t *testing.T) {
	for _, tt := range []struct {
		name string
		raw  string
	}{
		{name: "malformed", raw: "{"},
		{name: "legacy empty", raw: "[]"},
		{name: "legacy incomplete", raw: `[{"path":"skills/legacy/SKILL.md","backup":"","success":true}]`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			env := seedPublishEnv(t, "legacy/SKILL.md", true, "original\n")
			defer env.db.Close()
			svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
			publicationID := publishForRecovery(t, svc, env)
			target := filepath.Join(env.projectRoot, "skills", "legacy", "SKILL.md")
			published, readErr := os.ReadFile(target)
			if readErr != nil {
				t.Fatalf("read published target: %v", readErr)
			}
			if _, err := env.db.DB.Exec(`UPDATE publications SET rollback_json = ? WHERE id = ?`, tt.raw, string(publicationID)); err != nil {
				t.Fatalf("inject legacy metadata: %v", err)
			}

			err := svc.Rollback(context.Background(), env.projectID, &RollbackPublicationInput{PublicationID: publicationID, Actor: env.actor})
			var domainErr *domain.DomainError
			if !errors.As(err, &domainErr) {
				t.Fatalf("Rollback error = %v, want domain error", err)
			}
			artifact, _ := domainErr.Details["recovery_artifact"].(string)
			if artifact == "" {
				t.Fatalf("legacy metadata error lacks recovery artifact: %+v", domainErr.Details)
			}
			if _, err := os.Stat(artifact); err != nil {
				t.Fatalf("recovery artifact is not readable: %v", err)
			}
			if got, _ := os.ReadFile(target); string(got) != string(published) {
				t.Fatalf("legacy rollback changed destination: got %q want %q", got, published)
			}
			var raw string
			if err := env.db.DB.QueryRow(`SELECT rollback_json FROM publications WHERE id = ?`, string(publicationID)).Scan(&raw); err != nil {
				t.Fatalf("read raw rollback metadata: %v", err)
			}
			if raw != tt.raw {
				t.Fatalf("rollback metadata was overwritten: got %q want %q", raw, tt.raw)
			}
		})
	}
}

func TestRollbackConflictPreservesTargetAndEmitsPatch(t *testing.T) {
	env := seedPublishEnv(t, "conflict/SKILL.md", true, "original\n")
	defer env.db.Close()
	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
	publicationID := publishForRecovery(t, svc, env)
	target := filepath.Join(env.projectRoot, "skills", "conflict", "SKILL.md")
	if err := os.WriteFile(target, []byte("user edit\n"), 0o644); err != nil {
		t.Fatalf("write user edit: %v", err)
	}

	err := svc.Rollback(context.Background(), env.projectID, &RollbackPublicationInput{PublicationID: publicationID, Actor: env.actor})
	var domainErr *domain.DomainError
	if !errors.As(err, &domainErr) {
		t.Fatalf("Rollback error = %v, want domain error", err)
	}
	if domainErr.Details["path"] != filepath.Join("skills", "conflict", "SKILL.md") {
		t.Fatalf("conflict path = %v", domainErr.Details["path"])
	}
	if reason, _ := domainErr.Details["reason"].(string); !strings.Contains(reason, "conflict") {
		t.Fatalf("conflict reason = %q", reason)
	}
	artifact, _ := domainErr.Details["recovery_artifact"].(string)
	patch, readErr := os.ReadFile(artifact)
	if readErr != nil || !strings.Contains(string(patch), "user edit") || !strings.Contains(string(patch), "original") {
		t.Fatalf("reversal patch is not actionable: path=%q content=%q error=%v", artifact, patch, readErr)
	}
	if got, _ := os.ReadFile(target); string(got) != "user edit\n" {
		t.Fatalf("conflicting target was overwritten: %q", got)
	}
}

func publishForRecovery(t *testing.T, svc *Service, env *publishTestEnv) domain.PublicationID {
	t.Helper()
	result, err := svc.Publish(context.Background(), env.projectID, &PublishInput{
		LearningID: env.learningID, PreviewHash: env.previewHash,
		Apply: true, Force: true, Actor: env.actor,
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	return result.Publication.ID
}

func recoveryIDFromError(t *testing.T, err error) domain.PublicationID {
	t.Helper()
	var domainErr *domain.DomainError
	if !errors.As(err, &domainErr) {
		t.Fatalf("error = %v, want domain recovery error", err)
	}
	value, _ := domainErr.Details["recovery_id"].(string)
	if value == "" {
		t.Fatalf("error does not advertise recovery_id: %+v", domainErr.Details)
	}
	return domain.PublicationID(value)
}

func loadPublicationByID(t *testing.T, db *storage.DB, publicationID domain.PublicationID) *domain.Publication {
	t.Helper()
	tx, err := db.DB.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("begin read transaction: %v", err)
	}
	defer tx.Rollback()
	pub, err := storage.GetPublication(context.Background(), tx, publicationID)
	if err != nil {
		t.Fatalf("GetPublication(%s): %v", publicationID, err)
	}
	return pub
}

func assertCompleteRecoveryEntry(t *testing.T, entry domain.RollbackEntry) {
	t.Helper()
	if entry.Path == "" || entry.OriginalExisted == nil || entry.OriginalMode == nil || entry.OriginalSHA256 == "" || entry.Backup == "" || entry.BackupSHA256 == "" || entry.ExpectedPublishedHash == "" {
		t.Fatalf("incomplete recovery entry: %+v", entry)
	}
}
