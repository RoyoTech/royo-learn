package publish

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"agent-royo-learn/internal/domain"
	"agent-royo-learn/internal/storage"
)

func TestApprovalValidationAndLifecycleEdges(t *testing.T) {
	env := seedPublishEnv(t, "approval-edges/SKILL.md", false, "")
	ctx := context.Background()
	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)

	invalid := []*ApproveInput{
		nil,
		{ApprovedBy: "reviewer"},
		{PreviewHash: env.previewHash},
		{LearningID: "wrong-learning", PreviewHash: env.previewHash, ApprovedBy: "reviewer"},
	}
	for index, input := range invalid {
		if _, err := svc.Approve(ctx, env.projectID, input); err == nil {
			t.Fatalf("invalid approval %d unexpectedly succeeded", index)
		}
	}

	if approval, err := svc.CheckApproval(ctx, env.previewHash); err != nil || approval.ID != *env.approvalID {
		t.Fatalf("CheckApproval valid = %#v, %v", approval, err)
	}
	if _, err := svc.CheckApproval(ctx, "missing-preview-hash"); err == nil {
		t.Fatal("missing approval unexpectedly succeeded")
	}

	past := utcNowPublish().Add(-time.Minute)
	explicit, err := svc.Approve(ctx, env.projectID, &ApproveInput{
		LearningID: env.learningID, PreviewHash: env.previewHash, ApprovedBy: "explicit-expiry", ExpiresAt: &past,
	})
	if err != nil || explicit.ExpiresAt == nil || !explicit.ExpiresAt.Equal(past) {
		t.Fatalf("explicit expiry approval = %#v, %v", explicit, err)
	}
	relative, err := svc.Approve(ctx, env.projectID, &ApproveInput{
		LearningID: env.learningID, PreviewHash: env.previewHash, ApprovedBy: "relative-expiry", ExpiresIn: 60,
	})
	if err != nil || relative.ExpiresAt == nil || !relative.ExpiresAt.After(utcNowPublish()) {
		t.Fatalf("relative expiry approval = %#v, %v", relative, err)
	}

	if _, err := env.db.DB.ExecContext(ctx, `UPDATE approvals SET expires_at = ? WHERE preview_hash = ?`, past.Format(time.RFC3339), env.previewHash); err != nil {
		t.Fatalf("expire approvals: %v", err)
	}
	if _, err := svc.CheckApproval(ctx, env.previewHash); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired CheckApproval error = %v", err)
	}

	var previewID domain.PreviewID
	if err := storage.WithTx(ctx, env.db, func(tx *sql.Tx) error {
		preview, err := storage.GetPreviewByHash(ctx, tx, env.previewHash)
		if err != nil {
			return err
		}
		previewID = preview.ID
		return storage.InvalidatePreview(ctx, tx, preview.ID)
	}); err != nil {
		t.Fatalf("invalidate preview: %v", err)
	}
	if previewID == "" {
		t.Fatal("preview ID was not loaded")
	}
	if _, err := svc.Approve(ctx, env.projectID, &ApproveInput{PreviewHash: env.previewHash, ApprovedBy: "reviewer"}); err == nil {
		t.Fatal("invalidated preview approval unexpectedly succeeded")
	}

	if err := env.db.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}
	if _, err := svc.CheckApproval(ctx, env.previewHash); err == nil || !strings.Contains(err.Error(), "begin tx") {
		t.Fatalf("closed database CheckApproval error = %v", err)
	}
}

func TestRestoreEntryValidationEdges(t *testing.T) {
	existed := true
	absent := false
	mode := uint32(0o644)
	valid := BackupEntry{
		OriginalPath:          "skills/test/SKILL.md",
		BackupPath:            "backup.bak",
		Checksum:              "checksum",
		OriginalExisted:       &existed,
		OriginalHash:          "original",
		OriginalMode:          &mode,
		ExpectedPublishedHash: "published",
		ExpectedPublishedMode: &mode,
	}
	if err := validateRestoreEntry(valid); err != nil {
		t.Fatalf("valid restore entry: %v", err)
	}
	validAbsent := BackupEntry{
		OriginalPath:          "skills/new/SKILL.md",
		OriginalExisted:       &absent,
		ExpectedPublishedHash: "published",
		ExpectedPublishedMode: &mode,
	}
	if err := validateRestoreEntry(validAbsent); err != nil {
		t.Fatalf("valid absent restore entry: %v", err)
	}

	cases := []struct {
		name  string
		entry BackupEntry
	}{
		{"missing path", func() BackupEntry { e := valid; e.OriginalPath = ""; return e }()},
		{"missing existence", func() BackupEntry { e := valid; e.OriginalExisted = nil; return e }()},
		{"missing published hash", func() BackupEntry { e := valid; e.ExpectedPublishedHash = ""; return e }()},
		{"missing published mode", func() BackupEntry { e := valid; e.ExpectedPublishedMode = nil; return e }()},
		{"contradictory absent", func() BackupEntry { e := validAbsent; e.BackupPath = "unexpected"; return e }()},
		{"missing backup", func() BackupEntry { e := valid; e.BackupPath = ""; return e }()},
		{"missing checksum", func() BackupEntry { e := valid; e.Checksum = ""; return e }()},
		{"missing original hash", func() BackupEntry { e := valid; e.OriginalHash = ""; return e }()},
		{"missing original mode", func() BackupEntry { e := valid; e.OriginalMode = nil; return e }()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateRestoreEntry(tc.entry); err == nil {
				t.Fatal("invalid restore entry unexpectedly passed")
			}
		})
	}
}

func TestPublicationLockOwnershipAndConflictEdges(t *testing.T) {
	root := t.TempDir()
	actor := domain.Actor{Kind: "agent"}
	lock, err := acquirePublicationLock(root, "publish", actor)
	if err != nil {
		t.Fatalf("acquirePublicationLock: %v", err)
	}
	if lock.owner.Owner != actor.Kind {
		t.Fatalf("lock owner = %q, want %q", lock.owner.Owner, actor.Kind)
	}
	if _, err := acquirePublicationLock(root, "rollback", domain.Actor{Name: "other"}); err == nil {
		t.Fatal("second publication lock unexpectedly succeeded")
	}

	lockPath := filepath.Join(root, filepath.FromSlash(publicationLockPath))
	changed := `{"token":"other","owner":"stale-owner","operation":"rollback","pid":1,"acquired_at":"2000-01-01T00:00:00Z"}`
	if err := os.WriteFile(lockPath, []byte(changed), 0o600); err != nil {
		t.Fatalf("replace lock owner: %v", err)
	}
	if err := lock.Release(); err == nil || !strings.Contains(err.Error(), "ownership changed") {
		t.Fatalf("changed-owner release error = %v", err)
	}
	if err := os.Remove(lockPath); err != nil {
		t.Fatalf("remove changed lock: %v", err)
	}

	clean, err := acquirePublicationLock(root, "publish", domain.Actor{Name: "owner"})
	if err != nil {
		t.Fatalf("reacquirePublicationLock: %v", err)
	}
	if err := clean.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	var nilLock *publicationLock
	if err := nilLock.Release(); err != nil {
		t.Fatalf("nil Release: %v", err)
	}
}

func TestJournalReadAndValidationEdges(t *testing.T) {
	root := t.TempDir()
	journalDir := filepath.Join(root, ".royo-learn")
	journal, err := NewJournal(root, journalDir)
	if err != nil {
		t.Fatalf("NewJournal: %v", err)
	}
	if entries, err := journal.ReadAll(); err != nil || entries != nil {
		t.Fatalf("empty ReadAll = %#v, %v", entries, err)
	}
	if err := journal.Append(JournalEntry{PublicationID: "pub", RollbackStatus: "started"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if status, err := journal.LatestStatus("pub"); err != nil || status != "started" {
		t.Fatalf("LatestStatus = %q, %v", status, err)
	}
	if status, err := journal.LatestStatus("missing"); err != nil || status != "" {
		t.Fatalf("missing LatestStatus = %q, %v", status, err)
	}

	journalPath := filepath.Join(journalDir, "publish-journal.jsonl")
	if err := os.WriteFile(journalPath, []byte("{}\nnot-json\n"), 0o600); err != nil {
		t.Fatalf("write corrupt journal: %v", err)
	}
	if _, err := journal.ReadAll(); err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("corrupt ReadAll error = %v", err)
	}
	if _, err := NewJournal(root, root); err == nil {
		t.Fatal("project root accepted as journal directory")
	}
	if _, err := NewJournal(root, filepath.Join(filepath.Dir(root), "outside-journal")); err == nil {
		t.Fatal("outside journal directory accepted")
	}
}

func TestFilesystemAndWriterSafetyEdges(t *testing.T) {
	root := t.TempDir()
	if _, err := secureRelativePath("", "file", "test", false); err == nil {
		t.Fatal("empty root accepted")
	}
	for _, path := range []string{"", ".", "..", filepath.Join("..", "escape"), filepath.Join(root, "absolute")} {
		if _, err := cleanRootName(path, "test"); err == nil {
			t.Fatalf("unsafe root name %q accepted", path)
		}
	}
	full, err := secureRelativePath(root, filepath.Join("nested", "file.txt"), "test", true)
	if err != nil || full != filepath.Join(root, "nested", "file.txt") {
		t.Fatalf("secureRelativePath = %q, %v", full, err)
	}
	if _, err := secureAbsoluteWithin(root, "", "test"); err == nil {
		t.Fatal("empty absolute path accepted")
	}
	if _, err := secureAbsoluteWithin(root, root, "test"); err == nil {
		t.Fatal("root itself accepted as child path")
	}
	if _, err := secureAbsoluteWithin(root, filepath.Join(root, "nested", "file.txt"), "test"); err != nil {
		t.Fatalf("inside absolute path rejected: %v", err)
	}
	if err := rejectSymlinkComponents(filepath.Join(root, "missing", "file"), true); err != nil {
		t.Fatalf("allowed missing component rejected: %v", err)
	}
	if err := rejectSymlinkComponents(filepath.Join(root, "missing", "file"), false); err == nil {
		t.Fatal("missing component accepted when disallowed")
	}

	writer := NewWriter(root)
	if err := writer.WriteFileCAS("../escape", []byte("x"), 0o600, TargetIdentity{}); err == nil {
		t.Fatal("writer accepted traversal")
	}
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := writer.WriteFileCAS("target.txt", []byte("replacement"), 0o600, TargetIdentity{}); err == nil {
		t.Fatal("writer replaced target expected to be absent")
	}
	mode := fileModeIdentity(0o600)
	missing := TargetIdentity{Exists: true, Hash: HashContent([]byte("missing")), Mode: &mode}
	if err := writer.WriteFileCAS("missing.txt", []byte("replacement"), 0o600, missing); err == nil {
		t.Fatal("writer replaced missing expected target")
	}
	writer.beforeDestructive = func(string) error { return errors.New("injected boundary failure") }
	expected := TargetIdentity{Exists: true, Hash: HashContent([]byte("original")), Mode: &mode}
	if err := writer.WriteFileCAS("target.txt", []byte("replacement"), 0o600, expected); err == nil || !strings.Contains(err.Error(), "boundary") {
		t.Fatalf("WriteFileCAS hook error = %v", err)
	}
	if err := writer.RemoveFileCAS("target.txt", expected); err == nil || !strings.Contains(err.Error(), "boundary") {
		t.Fatalf("RemoveFileCAS hook error = %v", err)
	}
	writer.beforeDestructive = nil
	if err := writer.RemoveFileCAS("missing.txt", missing); err == nil {
		t.Fatal("RemoveFileCAS accepted missing target")
	}
	wrong := TargetIdentity{Exists: true, Hash: "wrong", Mode: &mode}
	if err := writer.RemoveFileCAS("target.txt", wrong); err == nil {
		t.Fatal("RemoveFileCAS accepted wrong identity")
	}

	if runtime.GOOS == "windows" {
		if supported, err := syncParentDirectoryRequired(target); err != nil || supported {
			t.Fatalf("Windows directory sync = %v, %v", supported, err)
		}
	} else {
		if supported, err := syncParentDirectoryRequired(target); err != nil || !supported {
			t.Fatalf("directory sync = %v, %v", supported, err)
		}
		if _, err := syncParentDirectory(filepath.Join(root, "missing", "file")); err == nil {
			t.Fatal("syncParentDirectory accepted missing parent")
		}
	}
}

func TestCheckDirtyWorktreeMatchesOnlyPublicationTargets(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable")
	}
	root := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
		}
	}
	runGit("init")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "Test")
	tracked := filepath.Join(root, "tracked.md")
	if err := os.WriteFile(tracked, []byte("clean"), 0o600); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	runGit("add", "tracked.md")
	runGit("commit", "-m", "initial")
	if err := os.WriteFile(tracked, []byte("dirty"), 0o600); err != nil {
		t.Fatalf("modify tracked file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "unrelated.md"), []byte("unrelated"), 0o600); err != nil {
		t.Fatalf("write unrelated file: %v", err)
	}
	result, err := CheckDirtyWorktree(root, []TargetResolution{{Root: ".", Path: "tracked.md"}})
	if err != nil {
		t.Fatalf("CheckDirtyWorktree: %v", err)
	}
	if !result.IsDirty || len(result.DirtyFiles) != 1 || result.DirtyFiles[0] != "tracked.md" || !strings.Contains(result.Reason, "tracked.md") {
		t.Fatalf("dirty result = %#v", result)
	}

	invalid := t.TempDir()
	if err := os.Mkdir(filepath.Join(invalid, ".git"), 0o700); err != nil {
		t.Fatalf("create invalid .git: %v", err)
	}
	if _, err := CheckDirtyWorktree(invalid, nil); err == nil {
		t.Fatal("invalid git repository unexpectedly passed dirty check")
	}
}

func TestBackupManagerRejectsUnsafeAndInconsistentState(t *testing.T) {
	root := t.TempDir()
	backupDir := filepath.Join(root, ".royo-learn", "backups")
	manager := NewBackupManager(root, backupDir)

	if _, err := manager.SnapshotFile("../escape"); err == nil {
		t.Fatal("SnapshotFile accepted traversal")
	}
	if err := os.Mkdir(filepath.Join(root, "directory-target"), 0o700); err != nil {
		t.Fatalf("create directory target: %v", err)
	}
	if _, err := manager.SnapshotFile("directory-target"); err == nil {
		t.Fatal("SnapshotFile accepted directory target")
	}
	if _, err := manager.BackupFile("directory-target"); err == nil {
		t.Fatal("BackupFile accepted directory target")
	}
	if _, err := manager.BackupSnapshot(nil); err == nil {
		t.Fatal("BackupSnapshot accepted nil snapshot")
	}
	if _, err := manager.BackupSnapshot(&FileSnapshot{RelativePath: "../escape"}); err == nil {
		t.Fatal("BackupSnapshot accepted traversal")
	}
	outsideManager := NewBackupManager(root, filepath.Join(filepath.Dir(root), "outside-backups"))
	if _, err := outsideManager.BackupSnapshot(&FileSnapshot{RelativePath: "missing"}); err == nil {
		t.Fatal("BackupSnapshot accepted outside backup root")
	}
	backupFile := filepath.Join(root, "backup-file")
	if err := os.WriteFile(backupFile, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write backup root file: %v", err)
	}
	fileManager := NewBackupManager(root, backupFile)
	if _, err := fileManager.BackupSnapshot(&FileSnapshot{RelativePath: "missing"}); err == nil {
		t.Fatal("BackupSnapshot accepted file as backup root")
	}

	if got := sanitizeBackupPrefix("bad name!?\u2603"); got != "bad_name___" {
		t.Fatalf("sanitizeBackupPrefix = %q", got)
	}
	if got := sanitizeBackupPrefix("."); got != "target" {
		t.Fatalf("sanitizeBackupPrefix dot = %q", got)
	}
	if _, err := readVerifiedBackup(filepath.Join(root, "missing.bak"), "hash"); err == nil {
		t.Fatal("readVerifiedBackup accepted missing file")
	}
	verifiedPath := filepath.Join(root, "verified.bak")
	if err := os.WriteFile(verifiedPath, []byte("backup"), 0o600); err != nil {
		t.Fatalf("write verified backup: %v", err)
	}
	if _, err := readVerifiedBackup(verifiedPath, "wrong"); err == nil {
		t.Fatal("readVerifiedBackup accepted checksum mismatch")
	}
	if content, err := readVerifiedBackup(verifiedPath, HashContent([]byte("backup"))); err != nil || string(content) != "backup" {
		t.Fatalf("readVerifiedBackup = %q, %v", content, err)
	}

	if err := manager.RestoreFile(BackupEntry{}); err == nil {
		t.Fatal("RestoreFile accepted empty metadata")
	}
	publishedPath := filepath.Join(root, "published.txt")
	if err := os.WriteFile(publishedPath, []byte("published"), 0o600); err != nil {
		t.Fatalf("write published target: %v", err)
	}
	existed := true
	mode := fileModeIdentity(0o600)
	entry := BackupEntry{
		OriginalPath:          "published.txt",
		BackupPath:            filepath.Join(filepath.Dir(root), "outside.bak"),
		Checksum:              "checksum",
		OriginalExisted:       &existed,
		OriginalHash:          HashContent([]byte("original")),
		OriginalMode:          &mode,
		ExpectedPublishedHash: HashContent([]byte("published")),
		ExpectedPublishedMode: &mode,
	}
	if err := manager.RestoreFile(entry); err == nil || !strings.Contains(err.Error(), "backup path") {
		t.Fatalf("outside backup RestoreFile error = %v", err)
	}
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		t.Fatalf("create backup directory: %v", err)
	}
	insideBackup := filepath.Join(backupDir, "original.bak")
	if err := os.WriteFile(insideBackup, []byte("different-original"), 0o600); err != nil {
		t.Fatalf("write inside backup: %v", err)
	}
	entry.BackupPath = insideBackup
	entry.Checksum = HashContent([]byte("different-original"))
	if err := manager.RestoreFile(entry); err == nil || !strings.Contains(err.Error(), "original hash") {
		t.Fatalf("wrong original RestoreFile error = %v", err)
	}
}

func TestPreviewAndPublishRejectInvalidLifecycleState(t *testing.T) {
	ctx := context.Background()
	env := seedPublishEnv(t, "lifecycle/SKILL.md", false, "")
	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)

	if _, err := svc.Preview(ctx, env.projectID, nil); err == nil {
		t.Fatal("Preview accepted nil input")
	}
	if _, err := svc.Preview(ctx, env.projectID, &PreviewInput{LearningID: "missing"}); err == nil || !strings.Contains(err.Error(), "get learning") {
		t.Fatalf("missing learning Preview error = %v", err)
	}
	repeated, err := svc.Preview(ctx, env.projectID, &PreviewInput{LearningID: env.learningID, Actor: env.actor})
	if err != nil || repeated.Preview.PreviewHash != env.previewHash {
		t.Fatalf("idempotent Preview = %#v, %v", repeated, err)
	}
	if _, err := svc.loadProject(ctx, "missing-project"); err == nil {
		t.Fatal("loadProject accepted missing project")
	}
	if err := os.Mkdir(filepath.Join(env.projectRoot, "AGENTS.md"), 0o700); err != nil {
		t.Fatalf("create AGENTS.md directory: %v", err)
	}
	if _, err := svc.needAgentsHook("test"); err == nil {
		t.Fatal("needAgentsHook accepted unreadable AGENTS.md target")
	}

	if _, err := svc.Publish(ctx, env.projectID, nil); err == nil {
		t.Fatal("Publish accepted nil input")
	}
	if _, err := svc.Publish(ctx, env.projectID, &PublishInput{LearningID: env.learningID}); err == nil {
		t.Fatal("Publish accepted empty preview hash")
	}
	if _, err := svc.Publish(ctx, env.projectID, &PublishInput{LearningID: "missing", PreviewHash: "missing"}); err == nil || !strings.Contains(err.Error(), "get learning") {
		t.Fatalf("missing learning Publish error = %v", err)
	}
	if _, err := svc.Publish(ctx, env.projectID, &PublishInput{LearningID: env.learningID, PreviewHash: "missing"}); err == nil || !strings.Contains(err.Error(), "get preview") {
		t.Fatalf("missing preview Publish error = %v", err)
	}
	sensitiveEnv := seedPublishEnv(t, "approval-mismatch/SKILL.md", true, "# Existing\n")
	sensitiveService := NewService(sensitiveEnv.db, sensitiveEnv.projectRoot, sensitiveEnv.backupDir, sensitiveEnv.journalDir)
	wrongApproval := domain.ApprovalID("wrong")
	if _, err := sensitiveService.Publish(ctx, sensitiveEnv.projectID, &PublishInput{
		LearningID: sensitiveEnv.learningID, PreviewHash: sensitiveEnv.previewHash, ApprovalID: &wrongApproval,
	}); err == nil || !strings.Contains(err.Error(), "approval_id") {
		t.Fatalf("wrong approval Publish error = %v", err)
	}

	lock, err := acquirePublicationLock(env.projectRoot, "test", env.actor)
	if err != nil {
		t.Fatalf("acquire test lock: %v", err)
	}
	_, publishErr := svc.Publish(ctx, env.projectID, &PublishInput{
		Apply: true, LearningID: env.learningID, PreviewHash: env.previewHash, ApprovalID: env.approvalID, Force: true, Actor: env.actor,
	})
	if publishErr == nil || !strings.Contains(publishErr.Error(), "lock") {
		t.Fatalf("locked Publish error = %v", publishErr)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("release test lock: %v", err)
	}

	if err := svc.Rollback(ctx, env.projectID, nil); err == nil {
		t.Fatal("Rollback accepted nil input")
	}
	if err := svc.Rollback(ctx, env.projectID, &RollbackPublicationInput{PublicationID: "missing", Actor: env.actor}); err == nil || !strings.Contains(err.Error(), "get publication") {
		t.Fatalf("missing publication Rollback error = %v", err)
	}
}

func TestPreviewIntegrityAndTargetVerificationEdges(t *testing.T) {
	learningID := domain.LearningID("learning")
	content := "content"
	target := domain.PublicationPlanTarget{
		Root: "skills", Path: "test/SKILL.md", Operation: domain.OpCreate,
		Content: content, PosteriorHash: HashContent([]byte(content)),
	}
	valid := &domain.PublicationPreview{
		LearningID: learningID,
		Plan: domain.PublicationPlan{
			LearningID: learningID, PolicySignature: "policy", Targets: []domain.PublicationPlanTarget{target},
		},
	}
	valid.PreviewHash = previewHash(learningID, valid.Plan.Targets, valid.Plan.PolicySignature)
	if err := validatePreviewIntegrity(valid, learningID); err != nil {
		t.Fatalf("valid preview integrity: %v", err)
	}

	cases := []struct {
		name    string
		preview *domain.PublicationPreview
	}{
		{"nil", nil},
		{"incomplete plan", &domain.PublicationPreview{LearningID: learningID, Plan: domain.PublicationPlan{LearningID: learningID}}},
		{"incomplete target", func() *domain.PublicationPreview {
			p := *valid
			p.Plan = valid.Plan
			p.Plan.Targets = []domain.PublicationPlanTarget{{Root: "skills"}}
			return &p
		}()},
		{"duplicate target", func() *domain.PublicationPreview {
			p := *valid
			p.Plan = valid.Plan
			p.Plan.Targets = []domain.PublicationPlanTarget{target, target}
			return &p
		}()},
		{"wrong posterior", func() *domain.PublicationPreview {
			p := *valid
			p.Plan = valid.Plan
			bad := target
			bad.PosteriorHash = "wrong"
			p.Plan.Targets = []domain.PublicationPlanTarget{bad}
			return &p
		}()},
		{"wrong plan hash", func() *domain.PublicationPreview { p := *valid; p.PreviewHash = "wrong"; return &p }()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validatePreviewIntegrity(tc.preview, learningID); err == nil {
				t.Fatal("invalid preview unexpectedly passed")
			}
		})
	}

	resolved := []TargetResolution{{Root: target.Root, Path: target.Path, Operation: target.Operation}}
	if contents, err := validatePlannedTargets(valid, resolved); err != nil || len(contents) != 1 || contents[0] != content {
		t.Fatalf("validatePlannedTargets = %#v, %v", contents, err)
	}
	if _, err := validatePlannedTargets(valid, nil); err == nil {
		t.Fatal("validatePlannedTargets accepted missing resolved target")
	}
	wrongOperation := resolved
	wrongOperation[0].Operation = domain.OpReplace
	if _, err := validatePlannedTargets(valid, wrongOperation); err == nil {
		t.Fatal("validatePlannedTargets accepted changed operation")
	}

	root := t.TempDir()
	svc := NewService(nil, root, filepath.Join(root, "backups"), filepath.Join(root, ".royo-learn"))
	if err := svc.validateCurrentHashes(valid, []TargetResolution{{Root: "other", Path: "ignored"}}); err != nil {
		t.Fatalf("unrecorded target hash validation: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, target.Root, target.Path), 0o700); err != nil {
		t.Fatalf("create directory target: %v", err)
	}
	if err := svc.validateCurrentHashes(valid, resolved); err == nil || !strings.Contains(err.Error(), "inspect destination") {
		t.Fatalf("directory target hash validation error = %v", err)
	}

	agentsPath := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(agentsPath, []byte("# Existing\n"), 0o600); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
	composed, err := svc.composeTargetContent(TargetResolution{Root: ".", Path: "AGENTS.md", Exists: true, IsManaged: true}, "", learningID, &TargetContext{ProjectKey: "project"})
	if err != nil || !strings.Contains(composed, "project") {
		t.Fatalf("compose AGENTS.md = %q, %v", composed, err)
	}
	if _, err := svc.composeTargetContent(TargetResolution{Root: ".", Path: "missing.md", Exists: true, IsManaged: true}, "", learningID, nil); err == nil {
		t.Fatal("composeTargetContent accepted missing existing target")
	}

	verificationRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(verificationRoot, "mismatch.txt"), []byte("actual"), 0o600); err != nil {
		t.Fatalf("write mismatch target: %v", err)
	}
	if err := os.Mkdir(filepath.Join(verificationRoot, "directory"), 0o700); err != nil {
		t.Fatalf("create verification directory: %v", err)
	}
	results := verifyTargets(verificationRoot, []domain.TargetEntry{
		{Root: ".", Path: "no-expected.txt"},
		{Root: ".", Path: "missing.txt"},
		{Root: ".", Path: "directory"},
		{Root: ".", Path: "mismatch.txt"},
	}, map[string]string{
		"missing.txt":  "expected",
		"directory":    "expected",
		"mismatch.txt": "expected",
	})
	for index, result := range results {
		if result.Pass {
			t.Fatalf("verification result %d unexpectedly passed: %#v", index, result)
		}
	}
	if changed, err := hashChanged(filepath.Join(verificationRoot, "missing.txt"), "missing.txt", map[string]string{"missing.txt": "hash"}); err == nil || changed {
		t.Fatalf("hashChanged missing = %v, %v", changed, err)
	}
}

func TestRecoveryEntryPointsSurfaceUnavailableState(t *testing.T) {
	ctx := context.Background()
	env := seedPublishEnv(t, "recovery-entry/SKILL.md", false, "")
	invalidJournal := NewService(env.db, env.projectRoot, env.backupDir, env.projectRoot)
	if _, err := invalidJournal.RecoverablePublications(ctx); err == nil {
		t.Fatal("RecoverablePublications accepted project root as journal directory")
	}

	closedEnv := seedPublishEnv(t, "closed-recovery/SKILL.md", false, "")
	closedService := NewService(closedEnv.db, closedEnv.projectRoot, closedEnv.backupDir, closedEnv.journalDir)
	if err := closedEnv.db.Close(); err != nil {
		t.Fatalf("close recovery database: %v", err)
	}
	if _, err := closedService.Preview(ctx, closedEnv.projectID, &PreviewInput{LearningID: closedEnv.learningID}); err == nil || !strings.Contains(err.Error(), "begin tx") {
		t.Fatalf("closed Preview error = %v", err)
	}
	if _, err := closedService.loadProject(ctx, closedEnv.projectID); err == nil || !strings.Contains(err.Error(), "begin tx") {
		t.Fatalf("closed loadProject error = %v", err)
	}
	if _, err := closedService.Approve(ctx, closedEnv.projectID, &ApproveInput{PreviewHash: closedEnv.previewHash, ApprovedBy: "reviewer"}); err == nil || !strings.Contains(err.Error(), "begin tx") {
		t.Fatalf("closed Approve error = %v", err)
	}
	if _, err := closedService.RecoverablePublications(ctx); err == nil || !strings.Contains(err.Error(), "begin") {
		t.Fatalf("closed RecoverablePublications error = %v", err)
	}
	if err := closedService.Rollback(ctx, closedEnv.projectID, &RollbackPublicationInput{PublicationID: "publication", Actor: closedEnv.actor}); err == nil || !strings.Contains(err.Error(), "begin tx") {
		t.Fatalf("closed Rollback error = %v", err)
	}
}

func TestPublishAndPreviewStateTransitionEdges(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid learning status", func(t *testing.T) {
		env := seedPublishEnv(t, "invalid-status/SKILL.md", false, "")
		svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
		if _, err := env.db.DB.ExecContext(ctx, `UPDATE learnings SET status = ? WHERE id = ?`, string(domain.StatusCaptured), string(env.learningID)); err != nil {
			t.Fatalf("update learning status: %v", err)
		}
		if _, err := svc.Preview(ctx, env.projectID, &PreviewInput{LearningID: env.learningID}); err == nil || !strings.Contains(err.Error(), "must be approved") {
			t.Fatalf("invalid-status Preview error = %v", err)
		}
		if _, err := svc.Publish(ctx, env.projectID, &PublishInput{LearningID: env.learningID, PreviewHash: env.previewHash}); err == nil || !strings.Contains(err.Error(), "must be approved") {
			t.Fatalf("invalid-status Publish error = %v", err)
		}
	})

	t.Run("missing curation", func(t *testing.T) {
		env := seedPublishEnv(t, "missing-curation/SKILL.md", false, "")
		svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
		if _, err := env.db.DB.ExecContext(ctx, `DELETE FROM curations WHERE learning_id = ?`, string(env.learningID)); err != nil {
			t.Fatalf("delete curations: %v", err)
		}
		if _, err := svc.Preview(ctx, env.projectID, &PreviewInput{LearningID: env.learningID}); err == nil || !strings.Contains(err.Error(), "no curation") {
			t.Fatalf("missing-curation Preview error = %v", err)
		}
		if _, err := svc.Publish(ctx, env.projectID, &PublishInput{LearningID: env.learningID, PreviewHash: env.previewHash}); err == nil || !strings.Contains(err.Error(), "no curation") {
			t.Fatalf("missing-curation Publish error = %v", err)
		}
	})

	t.Run("invalidated preview", func(t *testing.T) {
		env := seedPublishEnv(t, "invalidated/SKILL.md", false, "")
		svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
		if _, err := env.db.DB.ExecContext(ctx, `UPDATE publication_previews SET invalidated_at = ? WHERE preview_hash = ?`, utcNowPublish().Format(time.RFC3339), env.previewHash); err != nil {
			t.Fatalf("invalidate preview: %v", err)
		}
		if _, err := svc.Publish(ctx, env.projectID, &PublishInput{LearningID: env.learningID, PreviewHash: env.previewHash}); err == nil || !strings.Contains(err.Error(), "invalidated") {
			t.Fatalf("invalidated Publish error = %v", err)
		}
	})

	t.Run("policy changed", func(t *testing.T) {
		env := seedPublishEnv(t, "policy-changed/SKILL.md", false, "")
		svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
		if _, err := env.db.DB.ExecContext(ctx, `UPDATE curations SET decision = ?, destination_json = ? WHERE learning_id = ?`,
			string(domain.CurationApproveSharedKnowledge), `{"type":"shared","root":"shared","path":"knowledge.md","required":true}`, string(env.learningID)); err != nil {
			t.Fatalf("change curation policy: %v", err)
		}
		if _, err := svc.Publish(ctx, env.projectID, &PublishInput{LearningID: env.learningID, PreviewHash: env.previewHash}); err == nil || !strings.Contains(err.Error(), "policy changed") {
			t.Fatalf("policy-changed Publish error = %v", err)
		}
	})

	t.Run("published state without matching publication", func(t *testing.T) {
		env := seedPublishEnv(t, "reconcile-missing/SKILL.md", false, "")
		svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
		if _, err := env.db.DB.ExecContext(ctx, `UPDATE learnings SET status = ? WHERE id = ?`, string(domain.StatusPublished), string(env.learningID)); err != nil {
			t.Fatalf("mark learning published: %v", err)
		}
		input := &PublishInput{Apply: true, LearningID: env.learningID, PreviewHash: env.previewHash, Actor: env.actor}
		if _, err := svc.Publish(ctx, env.projectID, input); err == nil || !strings.Contains(err.Error(), "no matching completed publication") {
			t.Fatalf("missing reconciliation Publish error = %v", err)
		}

		lock, err := acquirePublicationLock(env.projectRoot, "test", env.actor)
		if err != nil {
			t.Fatalf("acquire reconciliation lock: %v", err)
		}
		if _, err := svc.Publish(ctx, env.projectID, input); err == nil || !strings.Contains(err.Error(), "lock") {
			t.Fatalf("locked reconciliation Publish error = %v", err)
		}
		if err := lock.Release(); err != nil {
			t.Fatalf("release reconciliation lock: %v", err)
		}
	})
}

func TestFilesystemJournalAndWriterTypeSafety(t *testing.T) {
	root := t.TempDir()
	rootFile := filepath.Join(root, "not-a-root")
	if err := os.WriteFile(rootFile, []byte("file"), 0o600); err != nil {
		t.Fatalf("write root file: %v", err)
	}
	if _, err := openRootNoFollow(rootFile); err == nil {
		t.Fatal("openRootNoFollow accepted a file")
	}
	if _, err := openRootNoFollow(filepath.Join(root, "missing-root")); err == nil {
		t.Fatal("openRootNoFollow accepted a missing root")
	}
	if err := NewWriter(rootFile).WriteFile("target", []byte("content"), 0o600); err == nil {
		t.Fatal("WriteFile accepted a file as project root")
	}

	directory := filepath.Join(root, "target-directory")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatalf("create target directory: %v", err)
	}
	writer := NewWriter(root)
	mode := fileModeIdentity(0o700)
	expected := TargetIdentity{Exists: true, Hash: "hash", Mode: &mode}
	if err := writer.WriteFileCAS("target-directory", []byte("content"), 0o600, expected); err == nil || !strings.Contains(err.Error(), "inspect target") {
		t.Fatalf("directory WriteFileCAS error = %v", err)
	}
	if err := writer.RemoveFileCAS("../escape", expected); err == nil {
		t.Fatal("RemoveFileCAS accepted traversal")
	}
	if err := writer.RemoveFileCAS("target-directory", expected); err == nil || !strings.Contains(err.Error(), "inspect target") {
		t.Fatalf("directory RemoveFileCAS error = %v", err)
	}
	if changed, err := ContentChanged(filepath.Join(root, "missing"), "hash"); err != nil || !changed {
		t.Fatalf("ContentChanged missing = %v, %v", changed, err)
	}
	if changed, err := ContentChanged(directory, "hash"); err == nil || changed {
		t.Fatalf("ContentChanged directory = %v, %v", changed, err)
	}
	conflict := targetChanged("changed", "target", "preserved")
	domainErr, ok := domain.AsDomainError(conflict)
	if !ok || domainErr.Details["preserved_path"] != "preserved" {
		t.Fatalf("targetChanged details = %#v, %v", domainErr, ok)
	}

	journalDir := filepath.Join(root, "journal")
	journal, err := NewJournal(root, journalDir)
	if err != nil {
		t.Fatalf("NewJournal: %v", err)
	}
	journalPath := filepath.Join(journalDir, "publish-journal.jsonl")
	if err := os.Mkdir(journalPath, 0o700); err != nil {
		t.Fatalf("create journal directory target: %v", err)
	}
	if _, err := journal.ReadAll(); err == nil || !strings.Contains(err.Error(), "not a regular") {
		t.Fatalf("directory journal ReadAll error = %v", err)
	}
	if err := os.RemoveAll(root); err != nil {
		t.Fatalf("remove journal root: %v", err)
	}
	if err := journal.Append(JournalEntry{}); err == nil || !strings.Contains(err.Error(), "open root") {
		t.Fatalf("removed-root Append error = %v", err)
	}
}

func TestSymlinkSafetyBranchesWhenSupported(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("outside"), 0o600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	parentLink := filepath.Join(root, "parent-link")
	if err := os.Symlink(outside, parentLink); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := secureRelativePath(root, filepath.Join("parent-link", "file"), "target", false); err == nil {
		t.Fatal("secureRelativePath accepted symlink parent")
	}
	if err := rejectSymlinkComponents(filepath.Join(parentLink, "file"), true); err == nil {
		t.Fatal("rejectSymlinkComponents accepted symlink parent")
	}
	targetLink := filepath.Join(root, "target-link")
	if err := os.Symlink(outsideFile, targetLink); err != nil {
		t.Fatalf("create target symlink: %v", err)
	}
	if _, err := secureRelativePath(root, "target-link", "target", false); err == nil {
		t.Fatal("secureRelativePath accepted target symlink")
	}
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	defer rootHandle.Close()
	if err := rejectRootSymlinks(rootHandle, "parent-link", false); err == nil {
		t.Fatal("rejectRootSymlinks accepted symlink")
	}
	if err := rejectRootSymlinks(rootHandle, "missing", false); err == nil {
		t.Fatal("rejectRootSymlinks accepted missing component")
	}
	if _, err := inspectRootRegularFile(rootHandle, "target-link"); err == nil {
		t.Fatal("inspectRootRegularFile accepted symlink")
	}

	journalLink := filepath.Join(root, "journal-link")
	if err := os.Symlink(outside, journalLink); err != nil {
		t.Fatalf("create journal symlink: %v", err)
	}
	if _, err := NewJournal(root, journalLink); err == nil {
		t.Fatal("NewJournal accepted symlink directory")
	}
	backupLink := filepath.Join(root, "backup-link")
	if err := os.Symlink(outside, backupLink); err != nil {
		t.Fatalf("create backup symlink: %v", err)
	}
	if _, err := secureBackupRoot(root, backupLink); err == nil {
		t.Fatal("secureBackupRoot accepted symlink directory")
	}
}

func TestSkillDiscoveryAndFormattingEdges(t *testing.T) {
	root := t.TempDir()
	if _, err := DiscoverChildSkills("\x00", "project"); err == nil {
		t.Fatal("DiscoverChildSkills accepted invalid project root")
	}
	skillsDir := filepath.Join(root, SkillsDir)
	for _, name := range []string{"project-zeta", "project-alpha"} {
		dir := filepath.Join(skillsDir, name)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("create child skill: %v", err)
		}
		description := "Trigger: " + strings.Repeat(name+" ", 20)
		content := "---\nname: " + name + "\ndescription: \"" + description + "\"\n---\n"
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o600); err != nil {
			t.Fatalf("write child skill: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "plain-file"), []byte("ignored"), 0o600); err != nil {
		t.Fatalf("write plain skills file: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(skillsDir, "project-missing"), 0o700); err != nil {
		t.Fatalf("create missing child: %v", err)
	}
	entries, err := DiscoverChildSkills(root, "project")
	if err != nil || len(entries) != 2 || entries[0].SkillName != "project-alpha" || len(entries[0].Description) <= 120 {
		t.Fatalf("DiscoverChildSkills = %#v, %v", entries, err)
	}

	if _, err := ParseFrontmatter("no frontmatter"); err == nil {
		t.Fatal("ParseFrontmatter accepted missing opening marker")
	}
	if _, err := ParseFrontmatter("---\nname: test"); err == nil {
		t.Fatal("ParseFrontmatter accepted missing closing marker")
	}
	if got := extractDescription("invalid"); got != "" {
		t.Fatalf("extractDescription invalid = %q", got)
	}
	longDescription := strings.Repeat("description ", 20)
	if got := extractTrigger("name", longDescription); len(got) <= 120 || !strings.HasSuffix(got, "…") {
		t.Fatalf("extractTrigger long fallback = %q", got)
	}

	parsed := parseSkillSections("## Rule before headers\n<!-- royo-learn:learning-id id-1 -->\nfirst line\nsecond line\n")
	if len(parsed) != 1 || parsed[0].Rule != "first line\nsecond line" {
		t.Fatalf("parseSkillSections rule fallback = %#v", parsed)
	}
	if got := InsertManagedBlock("existing text", "new block"); !strings.HasPrefix(got, "existing text\n") {
		t.Fatalf("InsertManagedBlock no-newline = %q", got)
	}
	if got := DiffSummary("unchanged", "file.md"); got != "No changes to file.md" {
		t.Fatalf("DiffSummary no changes = %q", got)
	}
	if !requiresSensitiveApproval([]TargetResolution{{Root: ".", Path: "AGENTS.md"}}) {
		t.Fatal("AGENTS.md target did not require sensitive approval")
	}

	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("existing"), 0o600); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
	targets, err := ResolveSkillPublishTargets(root, &domain.Destination{Path: "project-area"}, "project", true)
	if err != nil || targets.AgentsRef == nil || targets.AgentsRef.Operation != domain.OpReplaceManagedBlock {
		t.Fatalf("ResolveSkillPublishTargets existing AGENTS = %#v, %v", targets, err)
	}
}

func TestJournalAndLockMissingOrCorruptState(t *testing.T) {
	root := t.TempDir()
	journalDir := filepath.Join(root, ".royo-learn")
	journal, err := NewJournal(root, journalDir)
	if err != nil {
		t.Fatalf("NewJournal: %v", err)
	}
	journalPath := filepath.Join(journalDir, "publish-journal.jsonl")
	if err := os.WriteFile(journalPath, []byte("{}\n\n{}\n"), 0o600); err != nil {
		t.Fatalf("write blank journal line: %v", err)
	}
	if entries, err := journal.ReadAll(); err != nil || len(entries) != 2 {
		t.Fatalf("blank-line ReadAll = %#v, %v", entries, err)
	}
	if err := os.WriteFile(journalPath, []byte("invalid"), 0o600); err != nil {
		t.Fatalf("write invalid journal: %v", err)
	}
	if _, err := journal.LatestStatus("publication"); err == nil {
		t.Fatal("LatestStatus accepted corrupt journal")
	}

	lock, err := acquirePublicationLock(root, "publish", domain.Actor{Name: "owner"})
	if err != nil {
		t.Fatalf("acquirePublicationLock: %v", err)
	}
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(publicationLockPath))); err != nil {
		t.Fatalf("remove lock file: %v", err)
	}
	if err := lock.Release(); err == nil || !strings.Contains(err.Error(), "read publication lock") {
		t.Fatalf("missing lock Release error = %v", err)
	}
	if _, err := acquirePublicationLock(filepath.Join(root, "missing-root"), "publish", domain.Actor{}); err == nil {
		t.Fatal("acquirePublicationLock accepted missing root")
	}
}

func TestRollbackAndReconciliationErrorEdges(t *testing.T) {
	ctx := context.Background()
	env := seedPublishEnv(t, "rollback-edges/SKILL.md", false, "")
	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
	result, err := svc.Publish(ctx, env.projectID, &PublishInput{
		Apply: true, LearningID: env.learningID, PreviewHash: env.previewHash, ApprovalID: env.approvalID, Force: true, Actor: env.actor,
	})
	if err != nil {
		t.Fatalf("Publish fixture: %v", err)
	}

	lock, err := acquirePublicationLock(env.projectRoot, "test", env.actor)
	if err != nil {
		t.Fatalf("acquire rollback lock: %v", err)
	}
	if err := svc.Rollback(ctx, env.projectID, &RollbackPublicationInput{PublicationID: result.Publication.ID, Actor: env.actor}); err == nil || !strings.Contains(err.Error(), "lock") {
		t.Fatalf("locked Rollback error = %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("release rollback lock: %v", err)
	}

	invalidJournalService := NewService(env.db, env.projectRoot, env.backupDir, env.projectRoot)
	if err := invalidJournalService.Rollback(ctx, env.projectID, &RollbackPublicationInput{PublicationID: result.Publication.ID, Actor: env.actor}); err == nil || !strings.Contains(err.Error(), "create journal") {
		t.Fatalf("invalid journal Rollback error = %v", err)
	}

	corruptRoot := t.TempDir()
	corruptJournal, err := NewJournal(corruptRoot, filepath.Join(corruptRoot, ".royo-learn"))
	if err != nil {
		t.Fatalf("NewJournal corrupt fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(corruptRoot, ".royo-learn", "publish-journal.jsonl"), []byte("invalid"), 0o600); err != nil {
		t.Fatalf("write corrupt reconciliation journal: %v", err)
	}
	if _, err := svc.reconcileRolledBack(ctx, corruptJournal, result.Publication); err == nil {
		t.Fatal("reconcileRolledBack accepted corrupt journal")
	}
}

func TestPreviewTargetResolutionFailureEdges(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid explicit area", func(t *testing.T) {
		env := seedPublishEnv(t, "invalid-area/SKILL.md", false, "")
		svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
		destination := `{"type":"skill","root":"skills","path":"invalid-area/SKILL.md","required":true,"area":"!!!"}`
		if _, err := env.db.DB.ExecContext(ctx, `UPDATE curations SET destination_json = ? WHERE learning_id = ?`, destination, string(env.learningID)); err != nil {
			t.Fatalf("update invalid area: %v", err)
		}
		if _, err := svc.Preview(ctx, env.projectID, &PreviewInput{LearningID: env.learningID}); err == nil {
			t.Fatal("Preview accepted invalid explicit area")
		}
	})

	t.Run("escaping destination", func(t *testing.T) {
		env := seedPublishEnv(t, "escape/SKILL.md", false, "")
		svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
		destination := `{"type":"skill","root":"skills","path":"../../outside","required":true}`
		if _, err := env.db.DB.ExecContext(ctx, `UPDATE curations SET destination_json = ? WHERE learning_id = ?`, destination, string(env.learningID)); err != nil {
			t.Fatalf("update escaping destination: %v", err)
		}
		if _, err := svc.Preview(ctx, env.projectID, &PreviewInput{LearningID: env.learningID}); err == nil || !strings.Contains(err.Error(), "resolve target") {
			t.Fatalf("escaping destination Preview error = %v", err)
		}
	})

	t.Run("directory destination", func(t *testing.T) {
		env := seedPublishEnv(t, "directory-target/SKILL.md", false, "")
		svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)
		fullPath := filepath.Join(env.projectRoot, "skills", "directory-target", "SKILL.md")
		if err := os.MkdirAll(fullPath, 0o700); err != nil {
			t.Fatalf("create directory destination: %v", err)
		}
		if _, err := svc.Preview(ctx, env.projectID, &PreviewInput{LearningID: env.learningID}); err == nil || !strings.Contains(err.Error(), "snapshot") {
			t.Fatalf("directory destination Preview error = %v", err)
		}
	})

	root := t.TempDir()
	if err := validateTargetPath(root, TargetResolution{Root: "..", Path: "outside"}); err == nil {
		t.Fatal("validateTargetPath accepted project escape")
	}
	if err := validateTargetPath(root, TargetResolution{Root: "skills", Path: filepath.Join("..", "outside")}); err == nil {
		t.Fatal("validateTargetPath accepted destination-root escape")
	}
	if risk := evaluateRisk(&domain.Learning{}, &domain.Curation{Destination: &domain.Destination{Type: "unknown"}}); risk != domain.RiskLow {
		t.Fatalf("unknown destination risk = %q", risk)
	}
	if risk := evaluateRisk(&domain.Learning{}, &domain.Curation{Destination: &domain.Destination{Type: domain.DestSkill}}); risk != domain.RiskLow {
		t.Fatalf("optional skill destination risk = %q", risk)
	}

	agentsPath := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(agentsPath, []byte("existing"), 0o600); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
	targets, err := ResolveTarget(root, &domain.Curation{Destination: &domain.Destination{Type: domain.DestAgentsRule, Root: ".", Path: "AGENTS.md"}}, nil)
	if err != nil || len(targets) != 1 || targets[0].Operation != domain.OpReplaceManagedBlock {
		t.Fatalf("existing AGENTS ResolveTarget = %#v, %v", targets, err)
	}
}

func TestJournalSymlinkPathWhenSupported(t *testing.T) {
	root := t.TempDir()
	journalDir := filepath.Join(root, ".royo-learn")
	journal, err := NewJournal(root, journalDir)
	if err != nil {
		t.Fatalf("NewJournal: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatalf("write outside journal: %v", err)
	}
	journalPath := filepath.Join(journalDir, "publish-journal.jsonl")
	if err := os.Symlink(outside, journalPath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := journal.Append(JournalEntry{}); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlink journal Append error = %v", err)
	}
	if _, err := journal.ReadAll(); err == nil || !strings.Contains(err.Error(), "non-symlink") {
		t.Fatalf("symlink journal ReadAll error = %v", err)
	}
}

func TestJournalRejectsForbiddenAndNonDirectoryRoots(t *testing.T) {
	root := t.TempDir()
	if _, err := NewJournal(root, filepath.Join(root, "bad\x00root")); err == nil {
		t.Fatal("NewJournal accepted NUL path")
	}
	journalFile := filepath.Join(root, "journal-file")
	if err := os.WriteFile(journalFile, []byte("file"), 0o600); err != nil {
		t.Fatalf("write journal root file: %v", err)
	}
	if _, err := NewJournal(root, journalFile); err == nil {
		t.Fatal("NewJournal accepted file as journal directory")
	}
}

func TestReconciliationAndRecoveryArtifactErrorDetails(t *testing.T) {
	ctx := context.Background()
	env := seedPublishEnv(t, "reconcile-errors/SKILL.md", false, "")
	svc := NewService(env.db, env.projectRoot, env.backupDir, env.journalDir)

	wrongPublication := &domain.Publication{
		ID: domain.PublicationID("wrong-publication"), LearningID: env.learningID,
		PreviewHash: "different-preview", Status: domain.PubStatusCompleted, StartedAt: utcNowPublish(),
	}
	if err := storage.WithTx(ctx, env.db, func(tx *sql.Tx) error {
		return storage.SavePublication(ctx, tx, wrongPublication)
	}); err != nil {
		t.Fatalf("save non-matching publication: %v", err)
	}
	if _, err := env.db.DB.ExecContext(ctx, `UPDATE learnings SET status = ? WHERE id = ?`, string(domain.StatusPublished), string(env.learningID)); err != nil {
		t.Fatalf("mark learning published: %v", err)
	}
	if _, err := svc.Publish(ctx, env.projectID, &PublishInput{Apply: true, LearningID: env.learningID, PreviewHash: env.previewHash, Actor: env.actor}); err == nil || !strings.Contains(err.Error(), "no matching completed publication") {
		t.Fatalf("non-matching reconciliation error = %v", err)
	}

	journal, err := NewJournal(env.projectRoot, env.journalDir)
	if err != nil {
		t.Fatalf("NewJournal: %v", err)
	}
	if _, err := svc.reconcileRolledBack(ctx, journal, &domain.Publication{ID: "missing-learning-publication", LearningID: "missing-learning"}); err == nil {
		t.Fatal("reconcileRolledBack accepted missing learning")
	}

	closedEnv := seedPublishEnv(t, "closed-reconcile/SKILL.md", false, "")
	closedService := NewService(closedEnv.db, closedEnv.projectRoot, closedEnv.backupDir, closedEnv.journalDir)
	if err := closedEnv.db.Close(); err != nil {
		t.Fatalf("close reconciliation database: %v", err)
	}
	if _, err := closedService.reconcilePublished(ctx, &PublishInput{Actor: closedEnv.actor}, &domain.Learning{ID: closedEnv.learningID}); err == nil {
		t.Fatal("reconcilePublished accepted closed database")
	}
	closedJournal, err := NewJournal(closedEnv.projectRoot, closedEnv.journalDir)
	if err != nil {
		t.Fatalf("NewJournal closed fixture: %v", err)
	}
	if _, err := closedService.reconcileRolledBack(ctx, closedJournal, &domain.Publication{ID: "publication", LearningID: closedEnv.learningID}); err == nil {
		t.Fatal("reconcileRolledBack accepted closed database")
	}

	failure := rollbackFailureError("publication", []recoveryFailure{{Path: "target", Reason: "conflict"}}, errors.New("journal unavailable"))
	domainErr, ok := domain.AsDomainError(failure)
	if !ok || domainErr.Details["audit_reason"] == nil {
		t.Fatalf("rollbackFailureError details = %#v, %v", domainErr, ok)
	}

	badRoot := filepath.Join(t.TempDir(), "missing-root")
	badService := NewService(nil, badRoot, filepath.Join(badRoot, "backups"), filepath.Join(badRoot, ".royo-learn"))
	legacy := badService.legacyRecoveryError("legacy/id", "raw metadata", "invalid metadata")
	legacyErr, ok := domain.AsDomainError(legacy)
	if !ok || !strings.Contains(legacyErr.Details["reason"].(string), "artifact creation failed") {
		t.Fatalf("legacyRecoveryError details = %#v, %v", legacyErr, ok)
	}
	if _, err := badService.writeRecoveryArtifact("publication", "suffix", "content"); err == nil {
		t.Fatal("writeRecoveryArtifact accepted missing root")
	}
	if _, err := svc.writeConflictArtifact("publication", 0, domain.RollbackEntry{Path: "../escape"}, errors.New("conflict")); err == nil {
		t.Fatal("writeConflictArtifact accepted unsafe target")
	}
}
