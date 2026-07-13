package selfupdate

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// oldBinarySuffix is appended to the current executable on Windows when
// the running binary cannot be overwritten in-place. The file is removed
// at the start of the next self-update run.
const oldBinarySuffix = ".old"

// Replace atomically (or best-effort on Windows) replaces the file at
// targetPath with newPath. isWindows toggles the platform-specific
// strategy: on Unix it uses an atomic rename into the same directory;
// on Windows it renames the current binary to .old and moves the new
// binary in its place.
func Replace(targetPath, newPath string, isWindows bool) error {
	if isWindows {
		return replaceWindows(targetPath, newPath)
	}
	return replaceUnix(targetPath, newPath)
}

// replaceUnix writes to a temp file in the same directory and uses
// os.Rename to atomically swap the executable, preserving 0755.
func replaceUnix(targetPath, newPath string) error {
	dir := filepath.Dir(targetPath)
	tmp, err := os.CreateTemp(dir, filepath.Base(targetPath)+".tmp-*")
	if err != nil {
		return fmt.Errorf("replace: create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	newFile, err := os.Open(newPath)
	if err != nil {
		tmp.Close()
		return fmt.Errorf("replace: open new binary: %w", err)
	}
	defer newFile.Close()

	if _, err := io.Copy(tmp, newFile); err != nil {
		tmp.Close()
		return fmt.Errorf("replace: copy new binary: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("replace: close temp: %w", err)
	}

	if err := os.Chmod(tmpName, 0o755); err != nil {
		return fmt.Errorf("replace: chmod temp: %w", err)
	}

	if err := os.Rename(tmpName, targetPath); err != nil {
		return fmt.Errorf("replace: rename: %w", err)
	}
	return nil
}

// replaceWindows parks the current binary as <target>.old and moves the
// new binary to the original location.
func replaceWindows(targetPath, newPath string) error {
	backupPath := targetPath + oldBinarySuffix

	// Remove a stale .old from a previous run.
	if _, err := os.Stat(backupPath); err == nil {
		if err := os.Remove(backupPath); err != nil {
			return fmt.Errorf("replace: remove stale %s: %w", oldBinarySuffix, err)
		}
	}

	// Park the current binary. It is OK if the file is locked (e.g. the
	// running process); Windows allows renaming open .exe files.
	if err := os.Rename(targetPath, backupPath); err != nil {
		return fmt.Errorf("replace: rename current to %s: %w", oldBinarySuffix, err)
	}

	if err := moveFile(newPath, targetPath); err != nil {
		// Best-effort rollback: put the old binary back.
		_ = os.Rename(backupPath, targetPath)
		return fmt.Errorf("replace: move new binary: %w", err)
	}
	return nil
}

// moveFile first attempts os.Rename; if it fails with a cross-device
// error it falls back to copy + delete.
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	// os.Rename failed — likely cross-device; fall back to copy + delete.
	in, openErr := os.Open(src)
	if openErr != nil {
		return fmt.Errorf("moveFile: open source after failed rename: %w (rename: %v)", openErr, os.Rename(src, dst))
	}
	defer in.Close()

	out, createErr := os.Create(dst)
	if createErr != nil {
		return createErr
	}
	defer out.Close()

	if _, copyErr := io.Copy(out, in); copyErr != nil {
		out.Close()
		os.Remove(dst)
		return copyErr
	}
	out.Close()
	in.Close()
	if removeErr := os.Remove(src); removeErr != nil {
		return fmt.Errorf("moveFile: remove source after copy: %w", removeErr)
	}
	return nil
}

// CleanupOldBinary removes the <target>.old file left over from a
// previous Windows-style replacement. It is a no-op when the file does
// not exist.
func CleanupOldBinary(targetPath string) {
	backupPath := targetPath + oldBinarySuffix
	_ = os.Remove(backupPath)
}
