package publish

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// BackupManager creates and restores file backups.
type BackupManager struct {
	backupDir   string
	projectRoot string
}

// NewBackupManager creates a new BackupManager.
func NewBackupManager(projectRoot, backupDir string) *BackupManager {
	return &BackupManager{
		backupDir:   backupDir,
		projectRoot: projectRoot,
	}
}

// BackupEntry describes a single backup.
type BackupEntry struct {
	OriginalPath string
	BackupPath   string
	Checksum     string
	Timestamp    time.Time
}

// BackupFile creates a timestamped backup of a file. Returns the backup entry.
func (b *BackupManager) BackupFile(relativePath string) (*BackupEntry, error) {
	srcPath := filepath.Join(b.projectRoot, relativePath)

	// Create backup directory if needed.
	if err := os.MkdirAll(b.backupDir, 0o755); err != nil {
		return nil, fmt.Errorf("BackupFile: mkdir backup: %w", err)
	}

	// Generate backup filename with timestamp.
	ts := time.Now().UTC().Format("20060102T150405")
	backupName := filepath.Base(relativePath) + "." + ts + ".bak"
	backupPath := filepath.Join(b.backupDir, backupName)

	// Copy file.
	src, err := os.Open(srcPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &BackupEntry{
				OriginalPath: relativePath,
				BackupPath:   backupPath,
				Checksum:     "",
				Timestamp:    time.Now().UTC(),
			}, nil
		}
		return nil, fmt.Errorf("BackupFile: open source: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(backupPath)
	if err != nil {
		return nil, fmt.Errorf("BackupFile: create backup: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return nil, fmt.Errorf("BackupFile: copy: %w", err)
	}

	// Compute checksum of backup.
	checksum, err := HashFile(backupPath)
	if err != nil {
		return nil, fmt.Errorf("BackupFile: checksum: %w", err)
	}

	return &BackupEntry{
		OriginalPath: relativePath,
		BackupPath:   backupPath,
		Checksum:     checksum,
		Timestamp:    time.Now().UTC(),
	}, nil
}

// RestoreFile restores a file from its backup. Returns error if restoration fails.
func (b *BackupManager) RestoreFile(entry BackupEntry) error {
	dstPath := filepath.Join(b.projectRoot, entry.OriginalPath)

	if entry.BackupPath == "" || entry.Checksum == "" {
		// File didn't exist before publish — just remove it.
		if err := os.Remove(dstPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("RestoreFile: remove new file: %w", err)
		}
		return nil
	}

	// Verify backup exists before attempting restore.
	if _, err := os.Stat(entry.BackupPath); os.IsNotExist(err) {
		return fmt.Errorf("RestoreFile: backup file not found: %s", entry.BackupPath)
	}

	// Copy backup to original location.
	src, err := os.Open(entry.BackupPath)
	if err != nil {
		return fmt.Errorf("RestoreFile: open backup: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("RestoreFile: create restore target: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("RestoreFile: copy: %w", err)
	}

	return nil
}

// RestoreAll restores all files from their backups. On failure, it reports
// which files failed but continues trying to restore the rest.
func (b *BackupManager) RestoreAll(entries []BackupEntry) []RestoreResult {
	var results []RestoreResult
	for _, entry := range entries {
		err := b.RestoreFile(entry)
		results = append(results, RestoreResult{
			Path:    entry.OriginalPath,
			Backup:  entry.BackupPath,
			Success: err == nil,
			Error:   err,
		})
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
