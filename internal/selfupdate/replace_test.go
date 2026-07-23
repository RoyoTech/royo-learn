package selfupdate

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"agent-royo-learn/internal/testutil"
)

func writeFileOrFatal(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFileOrFatal(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestReplaceUnixStyle(t *testing.T) {
	dir := testutil.TempDir(t)
	target := filepath.Join(dir, "royo-learn")
	writeFileOrFatal(t, target, []byte("old binary"))

	stagingDir := testutil.TempDir(t) // deliberately a different directory
	newBinary := filepath.Join(stagingDir, "royo-learn.new")
	writeFileOrFatal(t, newBinary, []byte("new binary"))

	if err := Replace(target, newBinary, false); err != nil {
		t.Fatalf("Replace returned error: %v", err)
	}

	if got := string(readFileOrFatal(t, target)); got != "new binary" {
		t.Fatalf("target content = %q, want %q", got, "new binary")
	}
	if _, err := os.Stat(target + oldBinarySuffix); !os.IsNotExist(err) {
		t.Fatalf("unix-style replace must not leave a %s file, stat err = %v", oldBinarySuffix, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(target)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Fatalf("replaced binary is not executable: mode %v", info.Mode())
		}
	}
}

func TestReplaceWindowsStyle(t *testing.T) {
	dir := testutil.TempDir(t)
	target := filepath.Join(dir, "royo-learn.exe")
	writeFileOrFatal(t, target, []byte("old binary"))

	stagingDir := testutil.TempDir(t)
	newBinary := filepath.Join(stagingDir, "royo-learn.exe.new")
	writeFileOrFatal(t, newBinary, []byte("new binary"))

	if err := Replace(target, newBinary, true); err != nil {
		t.Fatalf("Replace returned error: %v", err)
	}

	if got := string(readFileOrFatal(t, target)); got != "new binary" {
		t.Fatalf("target content = %q, want %q", got, "new binary")
	}
	// The previous binary is parked next to the target so a running
	// executable can be swapped out; it is removed on the next run.
	if got := string(readFileOrFatal(t, target+oldBinarySuffix)); got != "old binary" {
		t.Fatalf("%s content = %q, want %q", oldBinarySuffix, got, "old binary")
	}
	// The staged copy is consumed by the same-directory swap.
	if _, err := os.Stat(target + newBinarySuffix); !os.IsNotExist(err) {
		t.Fatalf("staged %s file must not remain after success, stat err = %v", newBinarySuffix, err)
	}
	// The update lock is released on success.
	if _, err := os.Stat(target + updateLockSuffix); !os.IsNotExist(err) {
		t.Fatalf("lock file must be removed after success, stat err = %v", err)
	}
}

func TestReplaceFailsWhenLockHeld(t *testing.T) {
	dir := testutil.TempDir(t)
	target := filepath.Join(dir, "royo-learn.exe")
	writeFileOrFatal(t, target, []byte("old binary"))
	lockPath := target + updateLockSuffix
	writeFileOrFatal(t, lockPath, []byte(""))

	stagingDir := testutil.TempDir(t)
	newBinary := filepath.Join(stagingDir, "royo-learn.exe.new")
	writeFileOrFatal(t, newBinary, []byte("new binary"))

	err := Replace(target, newBinary, true)
	if err == nil {
		t.Fatal("Replace expected error while lock is held, got nil")
	}
	if !strings.Contains(err.Error(), lockPath) {
		t.Fatalf("error %q should name the lock file %s", err, lockPath)
	}
	if !strings.Contains(err.Error(), "remove it") {
		t.Fatalf("error %q should tell the user to remove the lock if no other update is running", err)
	}
	if got := string(readFileOrFatal(t, target)); got != "old binary" {
		t.Fatalf("target content = %q, want untouched %q", got, "old binary")
	}
	// A pre-existing lock is never removed by the losing run.
	if _, statErr := os.Stat(lockPath); statErr != nil {
		t.Fatalf("pre-existing lock file must remain, stat err = %v", statErr)
	}
}

func TestReplaceReleasesLockAfterSuccessfulUnixRun(t *testing.T) {
	dir := testutil.TempDir(t)
	target := filepath.Join(dir, "royo-learn")
	writeFileOrFatal(t, target, []byte("old binary"))

	stagingDir := testutil.TempDir(t)
	newBinary := filepath.Join(stagingDir, "royo-learn.new")
	writeFileOrFatal(t, newBinary, []byte("new binary"))

	if err := Replace(target, newBinary, false); err != nil {
		t.Fatalf("Replace returned error: %v", err)
	}
	if _, err := os.Stat(target + updateLockSuffix); !os.IsNotExist(err) {
		t.Fatalf("lock file must be removed after success, stat err = %v", err)
	}
}

func TestReplaceWindowsStyleStagingFailureLeavesBinaryUntouched(t *testing.T) {
	dir := testutil.TempDir(t)
	target := filepath.Join(dir, "royo-learn.exe")
	writeFileOrFatal(t, target, []byte("old binary"))

	stagingDir := testutil.TempDir(t)
	newBinary := filepath.Join(stagingDir, "royo-learn.exe.new")
	writeFileOrFatal(t, newBinary, []byte("new binary"))

	// Occupy the staged path with a directory so staging fails before the
	// current binary is touched.
	if err := os.Mkdir(target+newBinarySuffix, 0o755); err != nil {
		t.Fatal(err)
	}

	err := Replace(target, newBinary, true)
	if err == nil {
		t.Fatal("Replace expected error when staging is blocked, got nil")
	}
	if got := string(readFileOrFatal(t, target)); got != "old binary" {
		t.Fatalf("target content = %q, want untouched %q", got, "old binary")
	}
	if _, statErr := os.Stat(target + updateLockSuffix); !os.IsNotExist(statErr) {
		t.Fatalf("lock file must be removed after failure, stat err = %v", statErr)
	}
}

func TestReplaceWindowsStyleOverwritesStaleOld(t *testing.T) {
	dir := testutil.TempDir(t)
	target := filepath.Join(dir, "royo-learn.exe")
	writeFileOrFatal(t, target, []byte("old binary"))
	writeFileOrFatal(t, target+oldBinarySuffix, []byte("stale leftover"))

	stagingDir := testutil.TempDir(t)
	newBinary := filepath.Join(stagingDir, "royo-learn.exe.new")
	writeFileOrFatal(t, newBinary, []byte("new binary"))

	if err := Replace(target, newBinary, true); err != nil {
		t.Fatalf("Replace returned error: %v", err)
	}
	if got := string(readFileOrFatal(t, target)); got != "new binary" {
		t.Fatalf("target content = %q, want %q", got, "new binary")
	}
	if got := string(readFileOrFatal(t, target+oldBinarySuffix)); got != "old binary" {
		t.Fatalf("%s content = %q, want the just-replaced binary", oldBinarySuffix, got)
	}
}

func TestCleanupOldBinaryRemovesLeftover(t *testing.T) {
	dir := testutil.TempDir(t)
	target := filepath.Join(dir, "royo-learn.exe")
	writeFileOrFatal(t, target, []byte("current"))
	writeFileOrFatal(t, target+oldBinarySuffix, []byte("leftover"))

	CleanupOldBinary(target)

	if _, err := os.Stat(target + oldBinarySuffix); !os.IsNotExist(err) {
		t.Fatalf("leftover %s file still present, stat err = %v", oldBinarySuffix, err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("current binary must remain, stat err = %v", err)
	}
}

func TestCleanupOldBinaryNoLeftoverIsNoop(t *testing.T) {
	dir := testutil.TempDir(t)
	target := filepath.Join(dir, "royo-learn.exe")
	writeFileOrFatal(t, target, []byte("current"))

	CleanupOldBinary(target) // must not panic or touch the binary

	if got := string(readFileOrFatal(t, target)); got != "current" {
		t.Fatalf("current binary changed: %q", got)
	}
}
