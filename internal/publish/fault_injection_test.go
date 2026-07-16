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

// faultyWriter wraps the real atomic writer and can fail or corrupt a chosen
// write call, counting calls in order. It is the injectable write seam used to
// prove the compensation path runs through the real service code (Recorrido D).
type faultyWriter struct {
	real      *Writer
	failAt    int // 1-based index of the write call to fail; 0 = never
	corruptAt int // 1-based index of the write call to corrupt (writes different bytes but succeeds); 0 = never
	calls     int
}

func (w *faultyWriter) WriteFileCAS(path string, content []byte, perm os.FileMode, expected TargetIdentity) error {
	w.calls++
	if w.failAt == w.calls {
		return errors.New("injected write failure at call for " + path)
	}
	if w.corruptAt == w.calls {
		return w.real.WriteFileCAS(path, append([]byte("CORRUPTED "), content...), perm, expected)
	}
	return w.real.WriteFileCAS(path, content, perm, expected)
}

// assertApproved asserts a learning is still Approved (never falsely published).
func assertApproved(t *testing.T, env *publishTestEnv) {
	t.Helper()
	ctx := context.Background()
	readTx, _ := env.db.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	l, _ := storage.GetLearning(ctx, readTx, env.learningID)
	readTx.Rollback()
	if l.Status != domain.StatusApproved {
		t.Errorf("learning status = %q, want %q — a late failure produced a FALSE published", l.Status, domain.StatusApproved)
	}
}

// assertNoSkillFiles asserts nothing remains under skills/ (all new writes were
// rolled back byte-for-byte, i.e. removed).
func assertNoSkillFiles(t *testing.T, env *publishTestEnv) {
	t.Helper()
	_ = filepath.Walk(filepath.Join(env.projectRoot, "skills"), func(p string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			t.Errorf("a modified file survived a failure: %s", p)
		}
		return nil
	})
}

// --- Fault 1: writing the FIRST file fails -----------------------------------

func TestFault_FirstFileWriteFails(t *testing.T) {
	ctx := context.Background()
	env := seedPublishEnv(t, "fault1/SKILL.md", false, "")
	defer env.db.Close()

	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
	svc.writer = &faultyWriter{real: NewWriter(env.projectRoot), failAt: 1}

	_, err := svc.Publish(ctx, env.projectID, &PublishInput{
		LearningID: env.learningID, PreviewHash: env.previewHash,
		Apply: true, Force: true, Actor: env.actor,
	})
	if err == nil {
		t.Fatal("expected a write failure error")
	}
	assertNoSkillFiles(t, env)
	assertApproved(t, env)
}

// --- Fault 2: writing the SECOND file fails ----------------------------------

func TestFault_SecondFileWriteFails(t *testing.T) {
	ctx := context.Background()
	// A path without a directory triggers the multi-target skill resolution
	// (child skill + index skill), so there are two files to write.
	env := seedPublishEnv(t, "fault2", false, "")
	defer env.db.Close()

	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
	fw := &faultyWriter{real: NewWriter(env.projectRoot), failAt: 2}
	svc.writer = fw

	_, err := svc.Publish(ctx, env.projectID, &PublishInput{
		LearningID: env.learningID, PreviewHash: env.previewHash,
		Apply: true, Force: true, Actor: env.actor,
	})
	if err == nil {
		t.Fatal("expected a write failure on the second file")
	}
	if fw.calls < 2 {
		t.Fatalf("expected at least 2 write calls (multi-target), got %d", fw.calls)
	}
	// The FIRST file was written then must have been rolled back (removed).
	assertNoSkillFiles(t, env)
	assertApproved(t, env)
}

// --- Fault 3: the verification step fails ------------------------------------

func TestFault_VerificationFails(t *testing.T) {
	ctx := context.Background()
	const original = "original skill body\nline two\n"
	env := seedPublishEnv(t, "fault3/SKILL.md", true, original)
	defer env.db.Close()
	target := filepath.Join(env.projectRoot, "skills", "fault3", "SKILL.md")

	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
	// Corrupt the write so the on-disk content no longer matches the expected
	// hash: verification must catch it and roll back.
	svc.writer = &faultyWriter{real: NewWriter(env.projectRoot), corruptAt: 1}

	_, err := svc.Publish(ctx, env.projectID, &PublishInput{
		LearningID: env.learningID, PreviewHash: env.previewHash,
		Apply: true, Force: true, Actor: env.actor,
	})
	if err == nil {
		t.Fatal("expected verification to fail")
	}
	got, _ := os.ReadFile(target)
	if !strings.HasPrefix(string(got), "CORRUPTED ") {
		t.Errorf("conflicting destination should be preserved for recovery, got: %q", string(got))
	}
	var domainErr *domain.DomainError
	if !errors.As(err, &domainErr) || domainErr.Code != domain.ErrRollbackFailed {
		t.Fatalf("verification conflict error = %v, want rollback_failed", err)
	}
	assertApproved(t, env)
}

// --- Fault 4: the journal (attempt record) fails -----------------------------

func TestFault_JournalAttemptFails(t *testing.T) {
	ctx := context.Background()
	const original = "original body\n"
	env := seedPublishEnv(t, "fault4/SKILL.md", true, original)
	defer env.db.Close()
	target := filepath.Join(env.projectRoot, "skills", "fault4", "SKILL.md")

	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
	svc.faults = &FaultHooks{
		BeforeJournalAttempt: func() error { return errors.New("injected journal failure") },
	}

	_, err := svc.Publish(ctx, env.projectID, &PublishInput{
		LearningID: env.learningID, PreviewHash: env.previewHash,
		Apply: true, Force: true, Actor: env.actor,
	})
	if err == nil {
		t.Fatal("expected a journal failure error")
	}
	if !strings.Contains(err.Error(), "journal") {
		t.Errorf("error should mention journal, got: %v", err)
	}
	// The attempt journal fails BEFORE any write, so the file is untouched.
	got, _ := os.ReadFile(target)
	if string(got) != original {
		t.Errorf("file changed despite a pre-write journal failure: %q", string(got))
	}
	assertApproved(t, env)
}

// --- Fault 5: the final SQLite update fails ----------------------------------

func TestFault_FinalSQLiteUpdateFails(t *testing.T) {
	ctx := context.Background()
	const original = "original body\nkeep me\n"
	env := seedPublishEnv(t, "fault5/SKILL.md", true, original)
	defer env.db.Close()
	target := filepath.Join(env.projectRoot, "skills", "fault5", "SKILL.md")

	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
	svc.faults = &FaultHooks{
		BeforeDBCommit: func() error { return errors.New("injected DB commit failure") },
	}

	_, err := svc.Publish(ctx, env.projectID, &PublishInput{
		LearningID: env.learningID, PreviewHash: env.previewHash,
		Apply: true, Force: true, Actor: env.actor,
	})
	if err == nil {
		t.Fatal("expected a DB commit failure error")
	}
	// Files must be rolled back byte-for-byte even though the write and verify
	// already succeeded.
	got, _ := os.ReadFile(target)
	if string(got) != original {
		t.Errorf("file not restored after DB commit failure.\n got: %q\nwant: %q", string(got), original)
	}
	assertApproved(t, env)
}

// --- Fault 6: the rollback itself fails --------------------------------------

func TestFault_RollbackItselfFailsEmitsRecoveryInstruction(t *testing.T) {
	ctx := context.Background()
	const original = "original body\n"
	env := seedPublishEnv(t, "fault6/SKILL.md", true, original)
	defer env.db.Close()

	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
	// Corrupt the write so verification fails, then force the compensating
	// rollback to fail too: the system must surface a recovery instruction.
	svc.writer = &faultyWriter{real: NewWriter(env.projectRoot), corruptAt: 1}
	svc.faults = &FaultHooks{
		FailRollback: func() error { return errors.New("backup volume unavailable") },
	}

	_, err := svc.Publish(ctx, env.projectID, &PublishInput{
		LearningID: env.learningID, PreviewHash: env.previewHash,
		Apply: true, Force: true, Actor: env.actor,
	})
	if err == nil {
		t.Fatal("expected a rollback failure error")
	}
	var de *domain.DomainError
	if !errors.As(err, &de) {
		t.Fatalf("expected a structured domain error, got %T: %v", err, err)
	}
	if de.Code != domain.ErrRollbackFailed {
		t.Errorf("error code = %q, want %q", de.Code, domain.ErrRollbackFailed)
	}
	if de.NextAction == "" || !strings.Contains(strings.ToLower(de.NextAction), "restore") {
		t.Errorf("a recovery instruction must be emitted, got NextAction=%q", de.NextAction)
	}
	if de.Details == nil || de.Details["journal_id"] == nil {
		t.Errorf("recovery details must reference the journal for manual recovery, got %v", de.Details)
	}
	// Still not published.
	assertApproved(t, env)
}

// --- Fault 7: a destination modified AFTER the preview was taken --------------

func TestFault_DestinationModifiedAfterPreviewIsRefused(t *testing.T) {
	ctx := context.Background()
	const original = "original body before preview\n"
	env := seedPublishEnv(t, "fault7/SKILL.md", true, original)
	defer env.db.Close()
	target := filepath.Join(env.projectRoot, "skills", "fault7", "SKILL.md")

	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)

	// Generate a REAL preview so its plan records the prior hash of the file.
	prev, err := svc.Preview(ctx, env.projectID, &PreviewInput{
		LearningID: env.learningID, Actor: env.actor,
	})
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	// Someone edits the destination after the preview was taken.
	const tampered = "someone edited this out of band\n"
	if err := os.WriteFile(target, []byte(tampered), 0o644); err != nil {
		t.Fatalf("tamper: %v", err)
	}

	_, err = svc.Publish(ctx, env.projectID, &PublishInput{
		LearningID: env.learningID, PreviewHash: prev.Preview.PreviewHash,
		Apply: true, Force: true, Actor: env.actor,
	})
	if err == nil {
		t.Fatal("publish must refuse a destination that changed after the preview")
	}
	var ce *domain.ConflictError
	if !errors.As(err, &ce) || ce.Code != domain.ErrTargetChanged {
		t.Fatalf("expected a target_changed refusal, got %v", err)
	}
	// The publish never touched the file; the out-of-band edit is intact.
	got, _ := os.ReadFile(target)
	if string(got) != tampered {
		t.Errorf("publish must not write when it refuses; file = %q", string(got))
	}
	assertApproved(t, env)
}
