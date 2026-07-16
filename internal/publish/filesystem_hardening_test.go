package publish

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/record"
)

func TestWriterDoesNotCreateThroughSymlinkedAncestor(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink unavailable: %v", err)
		}
		t.Fatalf("create symlink: %v", err)
	}

	err := NewWriter(root).WriteFileCAS(filepath.Join("linked", "created", "target.txt"), []byte("bad"), 0o644, TargetIdentity{Exists: false})
	if err == nil {
		t.Fatal("WriteFileCAS followed a symlinked ancestor")
	}
	if _, statErr := os.Stat(filepath.Join(outside, "created")); !os.IsNotExist(statErr) {
		t.Fatalf("writer created a directory outside root: %v", statErr)
	}
}

func TestRecordMaterializerDoesNotCreateThroughSymlinkedAncestor(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "store")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink unavailable: %v", err)
		}
		t.Fatalf("create symlink: %v", err)
	}
	learning := &domain.Learning{ID: "learning-1", Title: "safe", CreatedAt: time.Now().UTC()}
	err := record.WriteRecord(filepath.Join(root, "store", "records"), learning)
	if err == nil {
		t.Fatal("WriteRecord followed a symlinked ancestor")
	}
	if _, statErr := os.Stat(filepath.Join(outside, "records")); !os.IsNotExist(statErr) {
		t.Fatalf("record materializer created a directory outside root: %v", statErr)
	}
}

func TestRestoreCASIncludesPublishedAndOriginalModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose Unix permission identity")
	}
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("original"), 0o640); err != nil {
		t.Fatalf("write original: %v", err)
	}
	manager := NewBackupManager(root, filepath.Join(root, ".royo-learn", "backups"))
	entry, err := manager.BackupFile("target.txt")
	if err != nil {
		t.Fatalf("BackupFile: %v", err)
	}
	if err := os.WriteFile(target, []byte("published"), 0o644); err != nil {
		t.Fatalf("write published: %v", err)
	}
	entry.ExpectedPublishedHash = HashContent([]byte("published"))
	publishedMode := uint32(0o644)
	entry.ExpectedPublishedMode = &publishedMode
	if err := os.Chmod(target, 0o600); err != nil {
		t.Fatalf("change published mode: %v", err)
	}
	if err := manager.RestoreFile(*entry); err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("mode-only published drift error = %v, want conflict", err)
	}
	if got, _ := os.ReadFile(target); string(got) != "published" {
		t.Fatalf("mode conflict overwrote target: %q", got)
	}

	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatalf("write idempotent candidate: %v", err)
	}
	if err := manager.RestoreFile(*entry); err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("wrong original mode error = %v, want conflict", err)
	}
}

func TestConcurrentPublishIsRejectedByProjectLock(t *testing.T) {
	env := seedPublishEnv(t, "locked/SKILL.md", false, "")
	defer env.db.Close()
	entered := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	first := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
	first.faults = &FaultHooks{AfterAttemptPersisted: func(domain.PublicationID) error {
		close(entered)
		<-release
		return errors.New("stop first publication")
	}}
	go func() {
		_, err := first.Publish(context.Background(), env.projectID, &PublishInput{
			LearningID: env.learningID, PreviewHash: env.previewHash, ApprovalID: env.approvalID,
			Apply: true, Force: true, Actor: env.actor,
		})
		firstDone <- err
	}()
	<-entered

	second := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
	_, err := second.Publish(context.Background(), env.projectID, &PublishInput{
		LearningID: env.learningID, PreviewHash: env.previewHash, ApprovalID: env.approvalID,
		Apply: true, Force: true, Actor: env.actor,
	})
	close(release)
	<-firstDone
	assertDomainCode(t, err, domain.ErrPublicationConflict)
}

func TestPublicationLockReportsExistingOwner(t *testing.T) {
	env := seedPublishEnv(t, "owner/SKILL.md", false, "")
	defer env.db.Close()
	store := filepath.Join(env.projectRoot, ".royo-learn")
	if err := os.MkdirAll(store, 0o700); err != nil {
		t.Fatalf("create store: %v", err)
	}
	lockPath := filepath.Join(store, "publication.lock")
	if err := os.WriteFile(lockPath, []byte(`{"owner":"other-process","operation":"rollback"}`), 0o600); err != nil {
		t.Fatalf("create lock: %v", err)
	}

	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
	_, err := svc.Publish(context.Background(), env.projectID, &PublishInput{
		LearningID: env.learningID, PreviewHash: env.previewHash, ApprovalID: env.approvalID,
		Apply: true, Force: true, Actor: env.actor,
	})
	domainErr, ok := domain.AsDomainError(err)
	if !ok || domainErr.Code != domain.ErrPublicationConflict || !strings.Contains(domainErr.Message, "other-process") {
		t.Fatalf("lock error = %v, want owner-attributed publication conflict", err)
	}
}
