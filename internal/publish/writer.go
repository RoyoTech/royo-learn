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

// WriteFileCAS atomically replaces exactly the expected target inside an
// os.Root capability. Existing targets are never moved away first: the final
// rename is one atomic replacement, so failure or process loss leaves either
// the original or the complete replacement at the destination.
func (w *Writer) WriteFileCAS(targetPath string, content []byte, perm os.FileMode, expected TargetIdentity) error {
	fullPath, err := secureRelativePath(w.projectRoot, targetPath, "target", true)
	if err != nil {
		return fmt.Errorf("WriteFileCAS: %w", err)
	}
	relative, err := cleanRootName(targetPath, "target")
	if err != nil {
		return fmt.Errorf("WriteFileCAS: %w", err)
	}
	root, err := openRootNoFollow(w.projectRoot)
	if err != nil {
		return fmt.Errorf("WriteFileCAS: open root: %w", err)
	}
	defer root.Close()
	if err := rejectRootSymlinks(root, filepath.Dir(relative), false); err != nil {
		return fmt.Errorf("WriteFileCAS: parent: %w", err)
	}
	tmpPath := filepath.Join(filepath.Dir(relative), ".royo-learn-write-"+uuid.NewString()+".tmp")
	tmp, err := root.OpenFile(tmpPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm.Perm())
	if err != nil {
		return fmt.Errorf("WriteFileCAS: create temp: %w", err)
	}
	placed := false
	defer func() {
		if !placed {
			_ = root.Remove(tmpPath)
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
	current, inspectErr := inspectRootRegularFile(root, relative)
	if !expected.Exists {
		if inspectErr == nil {
			return targetChanged("target appeared before atomic create", targetPath, "")
		}
		if !os.IsNotExist(inspectErr) {
			return fmt.Errorf("WriteFileCAS: inspect target: %w", inspectErr)
		}
		// Link is used only for exclusive creation. Existing-file replacement,
		// the crash-sensitive path, uses atomic Rename below.
		if err := root.Link(tmpPath, relative); err != nil {
			if _, statErr := root.Lstat(relative); statErr == nil {
				return targetChanged("target appeared before atomic create", targetPath, "")
			}
			return fmt.Errorf("WriteFileCAS: exclusive placement: %w", err)
		}
		if err := root.Remove(tmpPath); err != nil {
			return fmt.Errorf("WriteFileCAS: remove temp link: %w", err)
		}
		placed = true
		if _, err := syncParentDirectoryRequired(fullPath); err != nil {
			return fmt.Errorf("WriteFileCAS: %w", err)
		}
		return nil
	}
	if inspectErr != nil {
		if os.IsNotExist(inspectErr) {
			return targetChanged("target disappeared before replacement", targetPath, "")
		}
		return fmt.Errorf("WriteFileCAS: inspect target: %w", inspectErr)
	}
	if current.Hash != expected.Hash || expected.Mode != nil && fileModeIdentity(current.Mode) != fileModeIdentity(os.FileMode(*expected.Mode)) {
		return targetChanged("target changed at final replacement boundary", targetPath, "")
	}
	if err := root.Rename(tmpPath, relative); err != nil {
		return fmt.Errorf("WriteFileCAS: atomic replacement: %w", err)
	}
	placed = true
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
	relative, err := cleanRootName(targetPath, "target")
	if err != nil {
		return fmt.Errorf("RemoveFileCAS: %w", err)
	}
	root, err := openRootNoFollow(w.projectRoot)
	if err != nil {
		return fmt.Errorf("RemoveFileCAS: open root: %w", err)
	}
	defer root.Close()
	current, inspectErr := inspectRootRegularFile(root, relative)
	if inspectErr != nil {
		if os.IsNotExist(inspectErr) {
			return targetChanged("target disappeared before deletion", targetPath, "")
		}
		return fmt.Errorf("RemoveFileCAS: inspect target: %w", inspectErr)
	}
	if !expected.Exists || current.Hash != expected.Hash || expected.Mode != nil && fileModeIdentity(current.Mode) != fileModeIdentity(os.FileMode(*expected.Mode)) {
		return targetChanged("target changed at final delete boundary", targetPath, "")
	}
	if err := root.Remove(relative); err != nil {
		return fmt.Errorf("RemoveFileCAS: remove verified target: %w", err)
	}
	if _, err := syncParentDirectoryRequired(fullPath); err != nil {
		return fmt.Errorf("RemoveFileCAS: %w", err)
	}
	return nil
}

func inspectRootRegularFile(root *os.Root, relative string) (*FileSnapshot, error) {
	info, err := root.Lstat(relative)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("target is not a regular non-symlink file")
	}
	f, err := root.Open(relative)
	if err != nil {
		return nil, err
	}
	content, readErr := io.ReadAll(f)
	opened, statErr := f.Stat()
	closeErr := f.Close()
	if readErr != nil {
		return nil, readErr
	}
	if statErr != nil {
		return nil, statErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if !os.SameFile(info, opened) {
		return nil, fmt.Errorf("target changed while opened")
	}
	return &FileSnapshot{Exists: true, Content: content, Hash: HashContent(content), Mode: opened.Mode()}, nil
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
