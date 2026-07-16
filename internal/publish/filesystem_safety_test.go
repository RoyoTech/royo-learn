package publish

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBackupSnapshotUsesOneOriginalRead(t *testing.T) {
	root := t.TempDir()
	relative := filepath.Join("content", "target.txt")
	target := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("create target directory: %v", err)
	}
	if err := os.WriteFile(target, []byte("original bytes"), 0o440); err != nil {
		t.Fatalf("write original: %v", err)
	}

	manager := NewBackupManager(root, filepath.Join(root, ".royo-learn", "backups"))
	snapshot, err := manager.SnapshotFile(relative)
	if err != nil {
		t.Fatalf("SnapshotFile: %v", err)
	}
	if err := os.WriteFile(target, []byte("later path content"), 0o660); err != nil {
		t.Fatalf("replace target after snapshot: %v", err)
	}

	entry, err := manager.BackupSnapshot(snapshot)
	if err != nil {
		t.Fatalf("BackupSnapshot: %v", err)
	}
	backup, err := os.ReadFile(entry.BackupPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(backup) != "original bytes" {
		t.Fatalf("backup = %q, want the captured original bytes", backup)
	}
	if entry.OriginalHash != HashContent([]byte("original bytes")) {
		t.Fatalf("original hash = %q, want snapshot hash", entry.OriginalHash)
	}
	if entry.Checksum != entry.OriginalHash {
		t.Fatalf("backup checksum = %q, want original hash %q", entry.Checksum, entry.OriginalHash)
	}
	if entry.OriginalMode == nil || os.FileMode(*entry.OriginalMode).Perm() != 0o440 {
		t.Fatalf("original mode = %v, want 0440", entry.OriginalMode)
	}
}

func TestBackupManagerRejectsSymlinkedBackupRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	backupRoot := filepath.Join(root, ".royo-learn", "backups")
	if err := os.MkdirAll(filepath.Dir(backupRoot), 0o755); err != nil {
		t.Fatalf("create store directory: %v", err)
	}
	if err := os.Symlink(outside, backupRoot); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink unavailable: %v", err)
		}
		t.Fatalf("create backup-root symlink: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "target.txt"), []byte("original"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}

	manager := NewBackupManager(root, backupRoot)
	_, err := manager.BackupFile("target.txt")
	if err == nil || !strings.Contains(err.Error(), "backup root") {
		t.Fatalf("BackupFile error = %v, want backup-root rejection", err)
	}
	entries, readErr := os.ReadDir(outside)
	if readErr != nil {
		t.Fatalf("read outside directory: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("backup escaped through symlink: %v", entries)
	}
}

func TestWriterCASPreservesFinalBoundaryReplacement(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("baseline"), 0o644); err != nil {
		t.Fatalf("write baseline: %v", err)
	}

	writer := NewWriter(root)
	writer.beforeDestructive = func(string) error {
		return os.WriteFile(target, []byte("user edit at boundary"), 0o644)
	}
	err := writer.WriteFileCAS("target.txt", []byte("published"), 0o644, TargetIdentity{
		Exists: true,
		Hash:   HashContent([]byte("baseline")),
	})
	if err == nil || !strings.Contains(err.Error(), "target changed") {
		t.Fatalf("WriteFileCAS error = %v, want target-changed conflict", err)
	}
	got, readErr := os.ReadFile(target)
	if readErr != nil || string(got) != "user edit at boundary" {
		t.Fatalf("boundary edit was lost: content=%q error=%v", got, readErr)
	}
}

func TestWriterCASPreservesConcurrentCreationForAbsentTarget(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	writer := NewWriter(root)
	writer.beforeDestructive = func(string) error {
		return os.WriteFile(target, []byte("user-created"), 0o644)
	}

	err := writer.WriteFileCAS("target.txt", []byte("published"), 0o644, TargetIdentity{Exists: false})
	if err == nil || !strings.Contains(err.Error(), "target changed") {
		t.Fatalf("WriteFileCAS error = %v, want target-changed conflict", err)
	}
	got, readErr := os.ReadFile(target)
	if readErr != nil || string(got) != "user-created" {
		t.Fatalf("concurrent creation was lost: content=%q error=%v", got, readErr)
	}
}

func TestWriterCASDeletePreservesFinalBoundaryReplacement(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("published"), 0o644); err != nil {
		t.Fatalf("write published target: %v", err)
	}

	writer := NewWriter(root)
	writer.beforeDestructive = func(string) error {
		return os.WriteFile(target, []byte("user edit at delete boundary"), 0o644)
	}
	err := writer.RemoveFileCAS("target.txt", TargetIdentity{
		Exists: true,
		Hash:   HashContent([]byte("published")),
	})
	if err == nil || !strings.Contains(err.Error(), "target changed") {
		t.Fatalf("RemoveFileCAS error = %v, want target-changed conflict", err)
	}
	got, readErr := os.ReadFile(target)
	if readErr != nil || string(got) != "user edit at delete boundary" {
		t.Fatalf("boundary edit was deleted: content=%q error=%v", got, readErr)
	}
}

func TestWriterRejectsTraversalAndSymlinkTargets(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "project")
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("create project root: %v", err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("create outside root: %v", err)
	}
	writer := NewWriter(root)

	for _, path := range []string{filepath.Join("..", "outside", "target.txt"), `C:\outside\target.txt`, `\\?\C:\outside\target.txt`} {
		if err := writer.WriteFileCAS(path, []byte("bad"), 0o644, TargetIdentity{Exists: false}); err == nil {
			t.Errorf("WriteFileCAS(%q) succeeded, want unsafe-path error", path)
		}
	}

	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		if runtime.GOOS == "windows" {
			return
		}
		t.Fatalf("create target symlink: %v", err)
	}
	if err := writer.WriteFileCAS(filepath.Join("link", "target.txt"), []byte("bad"), 0o644, TargetIdentity{Exists: false}); err == nil {
		t.Fatal("WriteFileCAS followed a symlinked target parent")
	}
}
