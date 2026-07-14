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

// newBinarySuffix is appended to the target path for the staged copy of
// the verified new binary. Staging into the target's own directory makes
// the final swap a same-directory rename instead of a potentially slow
// cross-device copy.
const newBinarySuffix = ".new"

// updateLockSuffix is appended to the target path for the lock file that
// prevents two concurrent self-update runs from racing on the shared
// .old backup.
const updateLockSuffix = ".update-lock"

// Replace atomically (or best-effort on Windows) replaces the file at
// targetPath with newPath. isWindows toggles the platform-specific
// strategy: on Unix it uses an atomic rename into the same directory;
// on Windows it parks the current binary as .old and swaps a staged
// copy in with a same-directory rename.
//
// A lock file at targetPath + ".update-lock" guards the whole operation;
// it is removed on every exit path.
func Replace(targetPath, newPath string, isWindows bool) error {
	unlock, err := acquireUpdateLock(targetPath)
	if err != nil {
		return err
	}
	defer unlock()

	if isWindows {
		return replaceWindows(targetPath, newPath)
	}
	return replaceUnix(targetPath, newPath)
}

// acquireUpdateLock creates the exclusive lock file for targetPath and
// returns the function that releases it. When the lock already exists it
// returns a readable error naming the file so the user can recover from
// a crashed previous run.
func acquireUpdateLock(targetPath string) (func(), error) {
	lockPath := targetPath + updateLockSuffix
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("another self-update appears to be in progress (lock file %s exists) — remove it if no other update is running", lockPath)
		}
		return nil, fmt.Errorf("replace: create lock file %s: %w", lockPath, err)
	}
	_ = f.Close()
	return func() { _ = os.Remove(lockPath) }, nil
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

// replaceWindows stages the new binary next to the target, parks the
// current binary as <target>.old, and swaps the staged copy in with a
// same-directory rename. The window in which no binary exists at
// targetPath is a single rename, never a copy, and a failed swap rolls
// the old binary back.
func replaceWindows(targetPath, newPath string) error {
	stagedPath := targetPath + newBinarySuffix
	backupPath := targetPath + oldBinarySuffix

	// Stage the verified new binary in the target's own directory so
	// the final swap cannot degrade into a cross-device copy.
	if err := stageFile(newPath, stagedPath); err != nil {
		return fmt.Errorf("replace: stage new binary: %w", err)
	}

	// Remove a stale .old from a previous run.
	if err := os.Remove(backupPath); err != nil && !os.IsNotExist(err) {
		_ = os.Remove(stagedPath)
		return fmt.Errorf("replace: remove stale %s: %w", oldBinarySuffix, err)
	}

	// Park the current binary. It is OK if the file is locked (e.g. the
	// running process); Windows allows renaming open .exe files.
	if err := os.Rename(targetPath, backupPath); err != nil {
		_ = os.Remove(stagedPath)
		return fmt.Errorf("replace: rename current to %s: %w", oldBinarySuffix, err)
	}

	if err := os.Rename(stagedPath, targetPath); err != nil {
		// Roll back: restore the old binary and drop the staged copy.
		_ = os.Rename(backupPath, targetPath)
		_ = os.Remove(stagedPath)
		return fmt.Errorf("replace: swap staged binary in: %w", err)
	}
	return nil
}

// stageFile copies src to stagedPath with mode 0755 and syncs it to disk
// so the subsequent rename swaps in fully written bytes.
func stageFile(src, stagedPath string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(stagedPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(stagedPath)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		_ = os.Remove(stagedPath)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(stagedPath)
		return err
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
