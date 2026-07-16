package publish

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"agent-royo-learn/internal/domain"

	"github.com/google/uuid"
)

// TargetIdentity is the expected destination state for one CAS mutation.
type TargetIdentity struct {
	Exists bool
	Hash   string
	Mode   *uint32
}

// Writer provides rooted, atomic compare-and-swap file operations.
type Writer struct {
	projectRoot       string
	beforeDestructive func(string) error
}

// NewWriter creates a new atomic Writer.
func NewWriter(projectRoot string) *Writer { return &Writer{projectRoot: projectRoot} }

// WriteFile preserves the historical API while routing it through CAS against a
// snapshot captured immediately before the mutation.
func (w *Writer) WriteFile(targetPath string, content []byte, perm os.FileMode) error {
	manager := NewBackupManager(w.projectRoot, filepath.Join(w.projectRoot, ".royo-learn", "backups"))
	snapshot, err := manager.SnapshotFile(targetPath)
	if err != nil {
		return fmt.Errorf("WriteFile: snapshot: %w", err)
	}
	expected := TargetIdentity{Exists: snapshot.Exists, Hash: snapshot.Hash}
	if snapshot.Exists {
		mode := uint32(snapshot.Mode)
		expected.Mode = &mode
	}
	return w.WriteFileCAS(targetPath, content, perm, expected)
}

// WriteFileCAS atomically replaces exactly the expected target. Existing
// content is first moved aside and verified; absent targets use an exclusive
// hard-link placement so a concurrent creation is never overwritten.
func (w *Writer) WriteFileCAS(targetPath string, content []byte, perm os.FileMode, expected TargetIdentity) error {
	fullPath, err := secureRelativePath(w.projectRoot, targetPath, "target", true)
	if err != nil {
		return fmt.Errorf("WriteFileCAS: %w", err)
	}
	dir := filepath.Dir(fullPath)
	tmp, err := os.CreateTemp(dir, ".royo-learn-write-*.tmp")
	if err != nil {
		return fmt.Errorf("WriteFileCAS: create temp: %w", err)
	}
	tmpPath := tmp.Name()
	placed := false
	defer func() {
		if !placed {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("WriteFileCAS: chmod temp: %w", err)
	}
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("WriteFileCAS: write temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("WriteFileCAS: sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("WriteFileCAS: close temp: %w", err)
	}
	if w.beforeDestructive != nil {
		if err := w.beforeDestructive(fullPath); err != nil {
			return fmt.Errorf("WriteFileCAS: final-boundary hook: %w", err)
		}
	}
	if _, err := secureRelativePath(w.projectRoot, targetPath, "target", false); err != nil {
		return fmt.Errorf("WriteFileCAS: final path validation: %w", err)
	}

	if !expected.Exists {
		if err := os.Link(tmpPath, fullPath); err != nil {
			if _, statErr := os.Lstat(fullPath); statErr == nil {
				return targetChanged("target appeared before atomic create", targetPath, "")
			}
			return fmt.Errorf("WriteFileCAS: exclusive placement: %w", err)
		}
		placed = true
		if err := os.Remove(tmpPath); err != nil {
			return fmt.Errorf("WriteFileCAS: remove temp link: %w", err)
		}
		if _, err := syncParentDirectoryRequired(fullPath); err != nil {
			return fmt.Errorf("WriteFileCAS: %w", err)
		}
		return nil
	}

	quarantine := filepath.Join(dir, ".royo-learn-old-"+uuid.NewString())
	if err := os.Rename(fullPath, quarantine); err != nil {
		if os.IsNotExist(err) {
			return targetChanged("target disappeared before replacement", targetPath, "")
		}
		return fmt.Errorf("WriteFileCAS: move current target aside: %w", err)
	}
	current, inspectErr := inspectMovedRegularFile(quarantine)
	if inspectErr != nil || current.Hash != expected.Hash || expected.Mode != nil && uint32(current.Mode) != *expected.Mode {
		restoreErr := restoreMovedFile(quarantine, fullPath)
		if restoreErr != nil {
			return targetChanged("target changed and could not be returned to its path", targetPath, quarantine)
		}
		return targetChanged("target changed at final replacement boundary", targetPath, "")
	}
	if err := os.Link(tmpPath, fullPath); err != nil {
		preserved := quarantine
		if restoreErr := restoreMovedFile(quarantine, fullPath); restoreErr == nil {
			preserved = ""
		}
		return targetChanged("target appeared during atomic replacement", targetPath, preserved)
	}
	placed = true
	if err := os.Remove(tmpPath); err != nil {
		return fmt.Errorf("WriteFileCAS: remove temp link: %w", err)
	}
	if _, err := syncParentDirectoryRequired(fullPath); err != nil {
		return fmt.Errorf("WriteFileCAS: %w", err)
	}
	if err := os.Remove(quarantine); err != nil {
		return fmt.Errorf("WriteFileCAS: remove prior target %s: %w", quarantine, err)
	}
	if _, err := syncParentDirectoryRequired(fullPath); err != nil {
		return fmt.Errorf("WriteFileCAS: %w", err)
	}
	return nil
}

// RemoveFileCAS removes exactly the expected regular file without deleting a
// replacement that appears at the final boundary.
func (w *Writer) RemoveFileCAS(targetPath string, expected TargetIdentity) error {
	fullPath, err := secureRelativePath(w.projectRoot, targetPath, "target", false)
	if err != nil {
		return fmt.Errorf("RemoveFileCAS: %w", err)
	}
	if w.beforeDestructive != nil {
		if err := w.beforeDestructive(fullPath); err != nil {
			return fmt.Errorf("RemoveFileCAS: final-boundary hook: %w", err)
		}
	}
	if _, err := secureRelativePath(w.projectRoot, targetPath, "target", false); err != nil {
		return fmt.Errorf("RemoveFileCAS: final path validation: %w", err)
	}
	quarantine := filepath.Join(filepath.Dir(fullPath), ".royo-learn-delete-"+uuid.NewString())
	if err := os.Rename(fullPath, quarantine); err != nil {
		if os.IsNotExist(err) {
			return targetChanged("target disappeared before deletion", targetPath, "")
		}
		return fmt.Errorf("RemoveFileCAS: move target aside: %w", err)
	}
	current, inspectErr := inspectMovedRegularFile(quarantine)
	if inspectErr != nil || !expected.Exists || current.Hash != expected.Hash || expected.Mode != nil && uint32(current.Mode) != *expected.Mode {
		if restoreErr := restoreMovedFile(quarantine, fullPath); restoreErr != nil {
			return targetChanged("target changed and was preserved outside its path", targetPath, quarantine)
		}
		return targetChanged("target changed at final delete boundary", targetPath, "")
	}
	if err := os.Remove(quarantine); err != nil {
		return fmt.Errorf("RemoveFileCAS: remove verified target: %w", err)
	}
	if _, err := syncParentDirectoryRequired(fullPath); err != nil {
		return fmt.Errorf("RemoveFileCAS: %w", err)
	}
	return nil
}

func inspectMovedRegularFile(path string) (*FileSnapshot, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("moved target is not a regular non-symlink file")
	}
	content, err := readFileOnce(path)
	if err != nil {
		return nil, err
	}
	return &FileSnapshot{Exists: true, Content: content, Hash: HashContent(content), Mode: info.Mode()}, nil
}

func restoreMovedFile(movedPath, targetPath string) error {
	if err := os.Link(movedPath, targetPath); err != nil {
		return fmt.Errorf("original preserved at %s: %w", movedPath, err)
	}
	if err := os.Remove(movedPath); err != nil {
		return fmt.Errorf("remove preserved link %s: %w", movedPath, err)
	}
	return nil
}

func targetChanged(message, path, preserved string) error {
	details := map[string]any{"path": path}
	if preserved != "" {
		details["preserved_path"] = preserved
	}
	return &domain.ConflictError{DomainError: &domain.DomainError{
		Code:        domain.ErrTargetChanged,
		Message:     "target changed: " + message + ": " + path,
		Recoverable: true,
		Details:     details,
		NextAction:  "inspect the preserved destination and regenerate the preview before retrying",
	}}
}

// HashFile computes the SHA-256 hash of a file's content.
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("HashFile: open: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("HashFile: read: %w", err)
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// HashContent computes the SHA-256 hash of byte content.
func HashContent(content []byte) string {
	sum := sha256.Sum256(content)
	return fmt.Sprintf("%x", sum[:])
}

// ContentChanged returns true if the target file's hash differs from the given hash.
func ContentChanged(path string, expectedHash string) (bool, error) {
	currentHash, err := HashFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return expectedHash != "", nil
		}
		return false, err
	}
	return currentHash != expectedHash, nil
}
