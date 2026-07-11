package publish

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Writer provides atomic file write operations.
type Writer struct {
	projectRoot string
}

// NewWriter creates a new atomic Writer.
func NewWriter(projectRoot string) *Writer {
	return &Writer{projectRoot: projectRoot}
}

// WriteFile writes content to a file atomically using temp file + rename.
func (w *Writer) WriteFile(targetPath string, content []byte, perm os.FileMode) error {
	fullPath := filepath.Join(w.projectRoot, targetPath)

	// Ensure parent directory exists.
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("WriteFile: mkdir: %w", err)
	}

	// Write to a temp file in the same directory to ensure atomic rename.
	tmpFile, err := os.CreateTemp(dir, ".royo-learn-write-*.tmp")
	if err != nil {
		return fmt.Errorf("WriteFile: create temp: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Ensure cleanup on error.
	written := false
	defer func() {
		if !written {
			os.Remove(tmpPath)
		}
	}()

	if _, err := tmpFile.Write(content); err != nil {
		tmpFile.Close()
		return fmt.Errorf("WriteFile: write temp: %w", err)
	}

	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return fmt.Errorf("WriteFile: sync temp: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("WriteFile: close temp: %w", err)
	}

	// Atomic rename.
	if err := os.Rename(tmpPath, fullPath); err != nil {
		return fmt.Errorf("WriteFile: rename: %w", err)
	}

	written = true
	return nil
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
	return fmt.Sprintf("%x", sum)
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
