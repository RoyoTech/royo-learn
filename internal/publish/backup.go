package publish

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BackupManager creates and restores file backups rooted inside a project.
type BackupManager struct {
	backupDir         string
	projectRoot       string
	beforeDestructive func(string) error
}

// NewBackupManager creates a new BackupManager.
func NewBackupManager(projectRoot, backupDir string) *BackupManager {
	return &BackupManager{backupDir: backupDir, projectRoot: projectRoot}
}

// FileSnapshot is one opened/read view of a target. Its bytes, hash and mode
// are the only baseline used to create rollback metadata and backup content.
type FileSnapshot struct {
	RelativePath string
	Exists       bool
	Content      []byte
	Hash         string
	Mode         os.FileMode
}

// BackupEntry describes a single backup and the identity needed for safe CAS restore.
type BackupEntry struct {
	OriginalPath          string
	BackupPath            string
	Checksum              string
	OriginalHash          string
	OriginalMode          *uint32
	OriginalExisted       *bool
	ExpectedPublishedHash string
	DirectorySynced       bool
	Timestamp             time.Time
}

// SnapshotFile captures one stable view of a regular target without following symlinks.
func (b *BackupManager) SnapshotFile(relativePath string) (*FileSnapshot, error) {
	fullPath, err := secureRelativePath(b.projectRoot, relativePath, "target", false)
	if err != nil {
		return nil, fmt.Errorf("SnapshotFile: validate target path: %w", err)
	}
	lstat, err := os.Lstat(fullPath)
	if os.IsNotExist(err) {
		return &FileSnapshot{RelativePath: filepath.Clean(relativePath)}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("SnapshotFile: lstat: %w", err)
	}
	if lstat.Mode()&os.ModeSymlink != 0 || !lstat.Mode().IsRegular() {
		return nil, fmt.Errorf("SnapshotFile: target must be a regular non-symlink file: %s", relativePath)
	}

	f, err := os.Open(fullPath)
	if err != nil {
		return nil, fmt.Errorf("SnapshotFile: open: %w", err)
	}
	content, readErr := io.ReadAll(f)
	fstat, statErr := f.Stat()
	closeErr := f.Close()
	if readErr != nil {
		return nil, fmt.Errorf("SnapshotFile: read: %w", readErr)
	}
	if statErr != nil {
		return nil, fmt.Errorf("SnapshotFile: stat opened target: %w", statErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("SnapshotFile: close: %w", closeErr)
	}
	if !os.SameFile(lstat, fstat) {
		return nil, fmt.Errorf("SnapshotFile: target changed while it was opened: %s", relativePath)
	}
	return &FileSnapshot{
		RelativePath: filepath.Clean(relativePath),
		Exists:       true,
		Content:      content,
		Hash:         HashContent(content),
		Mode:         fstat.Mode(),
	}, nil
}

// BackupFile captures and backs up a target. Publish uses SnapshotFile and
// BackupSnapshot separately so the same snapshot can also drive its CAS write.
func (b *BackupManager) BackupFile(relativePath string) (*BackupEntry, error) {
	snapshot, err := b.SnapshotFile(relativePath)
	if err != nil {
		return nil, err
	}
	return b.BackupSnapshot(snapshot)
}

// BackupSnapshot writes a collision-resistant backup from captured bytes. It
// never reopens the original path and verifies the closed backup in one read.
func (b *BackupManager) BackupSnapshot(snapshot *FileSnapshot) (*BackupEntry, error) {
	if snapshot == nil || snapshot.RelativePath == "" {
		return nil, fmt.Errorf("BackupSnapshot: snapshot and relative path are required")
	}
	if _, err := secureRelativePath(b.projectRoot, snapshot.RelativePath, "target", false); err != nil {
		return nil, fmt.Errorf("BackupSnapshot: validate original path: %w", err)
	}
	backupRoot, err := secureBackupRoot(b.projectRoot, b.backupDir)
	if err != nil {
		return nil, fmt.Errorf("BackupSnapshot: backup root: %w", err)
	}

	existed := snapshot.Exists
	entry := &BackupEntry{
		OriginalPath:    snapshot.RelativePath,
		OriginalExisted: &existed,
		OriginalHash:    snapshot.Hash,
		Timestamp:       time.Now().UTC(),
	}
	if !snapshot.Exists {
		return entry, nil
	}
	mode := uint32(snapshot.Mode)
	entry.OriginalMode = &mode

	prefix := sanitizeBackupPrefix(filepath.Base(snapshot.RelativePath)) + "-"
	dst, err := os.CreateTemp(backupRoot, prefix+"*.bak")
	if err != nil {
		return nil, fmt.Errorf("BackupSnapshot: create backup: %w", err)
	}
	backupPath := dst.Name()
	complete := false
	defer func() {
		if !complete {
			_ = os.Remove(backupPath)
		}
	}()
	if err := dst.Chmod(0o600); err != nil {
		_ = dst.Close()
		return nil, fmt.Errorf("BackupSnapshot: chmod backup: %w", err)
	}
	if _, err := dst.Write(snapshot.Content); err != nil {
		_ = dst.Close()
		return nil, fmt.Errorf("BackupSnapshot: write backup: %w", err)
	}
	if err := dst.Sync(); err != nil {
		_ = dst.Close()
		return nil, fmt.Errorf("BackupSnapshot: sync backup: %w", err)
	}
	if err := dst.Close(); err != nil {
		return nil, fmt.Errorf("BackupSnapshot: close backup: %w", err)
	}
	directorySynced, err := syncParentDirectoryRequired(backupPath)
	if err != nil {
		return nil, fmt.Errorf("BackupSnapshot: %w", err)
	}
	verified, err := readFileOnce(backupPath)
	if err != nil {
		return nil, fmt.Errorf("BackupSnapshot: verify backup: %w", err)
	}
	if !bytes.Equal(verified, snapshot.Content) {
		return nil, fmt.Errorf("BackupSnapshot: backup bytes do not match captured original")
	}

	entry.BackupPath = backupPath
	entry.Checksum = HashContent(verified)
	entry.DirectorySynced = directorySynced
	complete = true
	return entry, nil
}

func secureBackupRoot(projectRoot, backupDir string) (string, error) {
	absProject, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", err
	}
	absBackup, err := filepath.Abs(backupDir)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absProject, absBackup)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("backup root %q must be inside project root %q", backupDir, projectRoot)
	}
	if unsafeRootForm(absBackup) {
		return "", fmt.Errorf("backup root uses a forbidden path form: %s", backupDir)
	}
	if info, statErr := os.Lstat(absBackup); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("backup root must not be a symlink: %s", backupDir)
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return "", statErr
	}
	if err := os.MkdirAll(absBackup, 0o700); err != nil {
		return "", err
	}
	if err := rejectSymlinkComponents(absBackup, false); err != nil {
		return "", err
	}
	return absBackup, nil
}

func sanitizeBackupPrefix(name string) string {
	name = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '_'
	}, name)
	if name == "" || name == "." {
		return "target"
	}
	return name
}

// RestoreFile restores one target only when its current identity matches the
// published hash. Already-restored targets are successful idempotent retries.
func (b *BackupManager) RestoreFile(entry BackupEntry) error {
	if err := validateRestoreEntry(entry); err != nil {
		return fmt.Errorf("RestoreFile: %w", err)
	}
	current, err := b.SnapshotFile(entry.OriginalPath)
	if err != nil {
		return fmt.Errorf("RestoreFile: inspect destination: %w", err)
	}

	writer := NewWriter(b.projectRoot)
	writer.beforeDestructive = b.beforeDestructive
	if *entry.OriginalExisted {
		if current.Exists && current.Hash == entry.OriginalHash {
			return nil
		}
		if !current.Exists || current.Hash != entry.ExpectedPublishedHash {
			return fmt.Errorf("RestoreFile: destination content conflict for %s", entry.OriginalPath)
		}
		backupPath, err := secureAbsoluteWithin(b.backupDir, entry.BackupPath, "backup")
		if err != nil {
			return fmt.Errorf("RestoreFile: validate backup path: %w", err)
		}
		content, err := readVerifiedBackup(backupPath, entry.Checksum)
		if err != nil {
			return fmt.Errorf("RestoreFile: %w", err)
		}
		if HashContent(content) != entry.OriginalHash {
			return fmt.Errorf("RestoreFile: backup does not match original hash")
		}
		return writer.WriteFileCAS(entry.OriginalPath, content, os.FileMode(*entry.OriginalMode), TargetIdentity{
			Exists: true,
			Hash:   entry.ExpectedPublishedHash,
		})
	}

	if !current.Exists {
		return nil
	}
	if current.Hash != entry.ExpectedPublishedHash {
		return fmt.Errorf("RestoreFile: destination content conflict for %s", entry.OriginalPath)
	}
	return writer.RemoveFileCAS(entry.OriginalPath, TargetIdentity{Exists: true, Hash: entry.ExpectedPublishedHash})
}

func validateRestoreEntry(entry BackupEntry) error {
	if entry.OriginalPath == "" {
		return fmt.Errorf("original path is required")
	}
	if entry.OriginalExisted == nil {
		return fmt.Errorf("original existence metadata is required")
	}
	if entry.ExpectedPublishedHash == "" {
		return fmt.Errorf("expected published hash is required")
	}
	if !*entry.OriginalExisted {
		if entry.BackupPath != "" || entry.Checksum != "" || entry.OriginalHash != "" || entry.OriginalMode != nil {
			return fmt.Errorf("original-absent metadata is contradictory")
		}
		return nil
	}
	if entry.BackupPath == "" {
		return fmt.Errorf("backup path is required")
	}
	if entry.Checksum == "" {
		return fmt.Errorf("backup checksum is required")
	}
	if entry.OriginalHash == "" {
		return fmt.Errorf("original hash is required")
	}
	if entry.OriginalMode == nil {
		return fmt.Errorf("original mode is required")
	}
	return nil
}

func readVerifiedBackup(path, expectedChecksum string) ([]byte, error) {
	content, err := readFileOnce(path)
	if err != nil {
		return nil, fmt.Errorf("read backup: %w", err)
	}
	if HashContent(content) != expectedChecksum {
		return nil, fmt.Errorf("backup checksum mismatch")
	}
	return content, nil
}

func readFileOnce(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	content, readErr := io.ReadAll(f)
	closeErr := f.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return content, nil
}

// RestoreAll restores all files and reports every result.
func (b *BackupManager) RestoreAll(entries []BackupEntry) []RestoreResult {
	results := make([]RestoreResult, 0, len(entries))
	for _, entry := range entries {
		err := b.RestoreFile(entry)
		results = append(results, RestoreResult{Path: entry.OriginalPath, Backup: entry.BackupPath, Success: err == nil, Error: err})
	}
	return results
}

// RestoreResult records the outcome of a single file restoration.
type RestoreResult struct {
	Path    string
	Backup  string
	Success bool
	Error   error
}
